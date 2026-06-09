package twitter

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/auth"
)

// User-Context OAuth 2.0 Authorization-Code-with-PKCE (S256) endpoints and
// parameters for the Twitter/X connector (BUG-056-002 design A.2, LOCKED).
const (
	// twitterAuthorizeEndpoint is the browser consent endpoint the operator
	// opens during authorize-begin.
	twitterAuthorizeEndpoint = "https://twitter.com/i/oauth2/authorize"
	// twitterTokenEndpoint is the confidential-client token endpoint hit during
	// authorize-finalize (code exchange) and, in Scope C, refresh.
	twitterTokenEndpoint = "https://api.twitter.com/2/oauth2/token"
	// twitterOAuthStateTTL bounds the lifetime of a persisted PKCE state row
	// (mirrors drive_oauth_states' 15-minute window).
	twitterOAuthStateTTL = 15 * time.Minute
	// stateTokenBytes is the number of cryptographically-random bytes behind the
	// CSRF state token (base64url-nopad encoded → 43 chars).
	stateTokenBytes = 32
)

// DefaultOwnerUserID is the owner_user_id the authorize CLI persists tokens
// under when the operator does not pass --user-id. The Twitter connector is a
// single-operator surface today; the composite (owner_user_id, connector_id)
// primary key leaves room for multiple accounts later (operators pass
// --user-id to scope a non-default account) without DDL churn. Scope C reads
// the user-context token under the same owner, so the default MUST stay stable.
const DefaultOwnerUserID = "default"

// twitterOAuthScopes returns the exact, LOCKED scope set the User-Context flow
// requests (BUG-056-002 design A.2). The order is fixed so the authorize URL is
// deterministic and assertable. offline.access is REQUIRED to receive a refresh
// token; the per-endpoint scopes gate /2/users/me (users.read),
// /2/users/:id/bookmarks (bookmark.read), and /2/users/:id/liked_tweets
// (like.read); tweet.read covers the tweet payloads on every endpoint.
func twitterOAuthScopes() []string {
	return []string{
		"offline.access",
		"tweet.read",
		"users.read",
		"bookmark.read",
		"like.read",
	}
}

// TwitterOAuthConfig carries the operator-configured OAuth 2.0 confidential
// client credentials threaded from SST (config.TwitterOAuth*; design A.8). No
// hidden defaults: an empty ClientID / RedirectURL makes authorize-begin fail
// loud, and an empty at-rest key (see NewAuthorizeService) refuses to persist
// tokens (smackerel-no-defaults).
type TwitterOAuthConfig struct {
	ClientID           string
	ClientSecret       string
	RedirectURL        string
	HTTPTimeoutSeconds int
}

// oauthFlowStore is the persistence surface the authorize flow depends on. The
// concrete *oauthStore (oauth_store.go) satisfies it against Postgres; tests
// use an in-memory fake so the begin/finalize/status orchestration is
// CI-runnable without a database (the at-rest crypto + DB round-trip are
// covered separately by the oauth_store tests + the integration migration pass).
type oauthFlowStore interface {
	SaveState(ctx context.Context, st pkceState) error
	ConsumeState(ctx context.Context, stateToken string) (pkceState, error)
	SaveTokens(ctx context.Context, owner string, t *auth.Token) error
	HasValidUserContext(ctx context.Context, owner string) (bool, error)
}

// newTwitterOAuthProvider builds the PKCE-capable confidential-client OAuth2
// provider with Twitter's LOCKED authorize/token endpoints and HTTP Basic auth
// at the token endpoint (design A.1/A.2). It reuses the additive
// auth.GenericOAuth2 methods delivered in Scope A; the shared OAuth2Provider
// interface is untouched.
func newTwitterOAuthProvider(cfg TwitterOAuthConfig) *auth.GenericOAuth2 {
	return auth.NewGenericOAuth2("twitter", auth.OAuth2Config{
		ClientID:               cfg.ClientID,
		ClientSecret:           cfg.ClientSecret,
		RedirectURL:            cfg.RedirectURL,
		AuthEndpoint:           twitterAuthorizeEndpoint,
		TokenEndpoint:          twitterTokenEndpoint,
		HTTPTimeoutSeconds:     cfg.HTTPTimeoutSeconds,
		TokenEndpointAuthStyle: "basic",
	})
}

// AuthorizeBeginResult is the operator-facing output of authorize-begin: the
// browser authorize URL and the state token the operator echoes back to
// authorize-finalize. Neither the code_verifier nor the client secret is ever
// surfaced here.
type AuthorizeBeginResult struct {
	AuthURL string
	State   string
}

// AuthorizeService orchestrates the operator CLI authorize flow
// (connector twitter authorize-begin|finalize|status). It is intentionally a
// thin coordinator over the Scope A primitives (PKCE pair generation, the
// encrypted store, the confidential-client provider) so the orchestration is
// unit-testable with an in-memory store and an httptest token endpoint.
type AuthorizeService struct {
	store    oauthFlowStore
	provider *auth.GenericOAuth2
	owner    string
	now      func() time.Time
}

// NewAuthorizeService builds the service for the CLI from a DB pool, the
// at-rest encryption key, the operator OAuth config, and the owner the tokens
// are persisted under. It fails loud when the at-rest key is empty (the
// encrypted store refuses the plaintext path) or when the owner is empty.
func NewAuthorizeService(pool *pgxpool.Pool, atRestKey string, oauthCfg TwitterOAuthConfig, owner string) (*AuthorizeService, error) {
	if owner == "" {
		return nil, fmt.Errorf("twitter authorize: owner user id is required")
	}
	store, err := newOAuthStore(pool, atRestKey)
	if err != nil {
		return nil, err
	}
	return &AuthorizeService{
		store:    store,
		provider: newTwitterOAuthProvider(oauthCfg),
		owner:    owner,
		now:      time.Now,
	}, nil
}

// Begin starts a PKCE flow: it generates a code_verifier + S256 code_challenge
// + a CSRF state token, persists a 15-minute twitter_oauth_states row carrying
// the verifier, and returns the authorize URL + state for the operator to open
// in a browser. Fails loud when oauth_client_id or oauth_redirect_url is empty
// (the operator must register a confidential client first). The verifier is
// NEVER returned or printed — only the S256 challenge travels in the URL.
func (s *AuthorizeService) Begin(ctx context.Context) (AuthorizeBeginResult, error) {
	if s.provider.Config.ClientID == "" {
		return AuthorizeBeginResult{}, fmt.Errorf(
			"twitter authorize-begin: oauth_client_id is required (set connectors.twitter.oauth_client_id " +
				"in config/smackerel.yaml → TWITTER_OAUTH_CLIENT_ID); register a Twitter OAuth 2.0 confidential client first")
	}
	if s.provider.Config.RedirectURL == "" {
		return AuthorizeBeginResult{}, fmt.Errorf(
			"twitter authorize-begin: oauth_redirect_url is required (set connectors.twitter.oauth_redirect_url " +
				"in config/smackerel.yaml → TWITTER_OAUTH_REDIRECT_URL); it MUST match the registered redirect URI")
	}

	verifier, challenge, err := auth.GeneratePKCEPair()
	if err != nil {
		return AuthorizeBeginResult{}, fmt.Errorf("twitter authorize-begin: generate pkce pair: %w", err)
	}
	state, err := generateStateToken()
	if err != nil {
		return AuthorizeBeginResult{}, fmt.Errorf("twitter authorize-begin: generate state token: %w", err)
	}

	scopes := twitterOAuthScopes()
	now := s.now()
	st := pkceState{
		StateToken:   state,
		OwnerUserID:  s.owner,
		ConnectorID:  twitterConnectorID,
		CodeVerifier: verifier,
		Scopes:       scopes,
		CreatedAt:    now,
		ExpiresAt:    now.Add(twitterOAuthStateTTL),
	}
	if err := s.store.SaveState(ctx, st); err != nil {
		return AuthorizeBeginResult{}, fmt.Errorf("twitter authorize-begin: persist state: %w", err)
	}

	return AuthorizeBeginResult{
		AuthURL: s.provider.AuthURLWithPKCE(scopes, state, challenge),
		State:   state,
	}, nil
}

// Finalize consumes the state row (TTL-checked + deleted), exchanges the pasted
// authorization code together with the stored code_verifier at the token
// endpoint (confidential-client HTTP Basic auth), and persists the returned
// access+refresh pair AES-256-GCM-encrypted. Fails loud on an unknown/expired
// state or a failed exchange. Returns the persisted token (the caller prints
// only its expiry, never a token value).
func (s *AuthorizeService) Finalize(ctx context.Context, state, code string) (*auth.Token, error) {
	if state == "" || code == "" {
		return nil, fmt.Errorf("twitter authorize-finalize: both --state and --code are required")
	}
	st, err := s.store.ConsumeState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("twitter authorize-finalize: %w", err)
	}
	tok, err := s.provider.ExchangeCodeWithVerifier(ctx, code, st.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("twitter authorize-finalize: token exchange: %w", err)
	}
	if err := s.store.SaveTokens(ctx, s.owner, tok); err != nil {
		return nil, fmt.Errorf("twitter authorize-finalize: persist tokens: %w", err)
	}
	return tok, nil
}

// Status reports whether a persisted user-context token exists for the owner.
// It drives the operator preflight and (in Scope C) the fail-loud-when-absent
// guard. It never returns or prints a token value.
func (s *AuthorizeService) Status(ctx context.Context) (bool, error) {
	return s.store.HasValidUserContext(ctx, s.owner)
}

// generateStateToken returns a cryptographically-random base64url-nopad CSRF
// state token bound to a single authorize flow.
func generateStateToken() (string, error) {
	b := make([]byte, stateTokenBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate state token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
