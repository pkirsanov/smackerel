package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxTokenResponseBytes limits the size of token endpoint responses to 1 MB.
// Prevents memory exhaustion from misconfigured OAuth providers or proxy redirects.
const maxTokenResponseBytes = 1 << 20 // 1 MB

// Token represents an OAuth2 token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scopes       []string  `json:"scopes"`
}

// IsExpired returns true if the token has expired.
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// OAuth2Provider defines the OAuth2 abstraction for connectors.
type OAuth2Provider interface {
	// AuthURL returns the authorization URL for the consent screen.
	AuthURL(scopes []string, state string) string

	// ExchangeCode exchanges an authorization code for tokens.
	ExchangeCode(ctx context.Context, code string) (*Token, error)

	// RefreshToken refreshes an expired access token.
	RefreshToken(ctx context.Context, refreshToken string) (*Token, error)

	// ProviderName returns the name of this provider.
	ProviderName() string
}

// OAuth2Config holds the configuration for an OAuth2 provider.
type OAuth2Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	AuthEndpoint  string
	TokenEndpoint string
	// HTTPTimeoutSeconds is the per-call timeout (seconds) applied to
	// every http.Client constructed inside tokenRequest. SST zero-defaults:
	// MUST be > 0 in production wiring. Sourced from
	// `cfg.AuthOAuthHTTPTimeoutSeconds` (yaml `auth.oauth.http_timeout_seconds`,
	// env `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS`). Replaces the pre-fix hardcoded
	// 15-second HTTP client timeout literal (BUG-020-009).
	HTTPTimeoutSeconds int
	// TokenEndpointAuthStyle selects how client credentials are presented at
	// the token endpoint (RFC 6749 §2.3.1). Empty or "body" (the default,
	// preserving existing-caller behavior) sends client_id/client_secret in
	// the form body. "basic" sends them via an Authorization: Basic
	// base64(id:secret) header and omits client_secret from the body, as
	// required by confidential OAuth 2.0 clients such as Twitter/X
	// (BUG-056-002).
	TokenEndpointAuthStyle string
}

// GenericOAuth2 implements OAuth2Provider for generic OAuth2 providers.
type GenericOAuth2 struct {
	Config OAuth2Config
	Name   string
}

// NewGenericOAuth2 creates a new generic OAuth2 provider.
func NewGenericOAuth2(name string, cfg OAuth2Config) *GenericOAuth2 {
	return &GenericOAuth2{Config: cfg, Name: name}
}

// ProviderName returns the provider name.
func (g *GenericOAuth2) ProviderName() string {
	return g.Name
}

// AuthURL builds the authorization URL.
func (g *GenericOAuth2) AuthURL(scopes []string, state string) string {
	params := url.Values{
		"client_id":     {g.Config.ClientID},
		"redirect_uri":  {g.Config.RedirectURL},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
		"response_type": {"code"},
	}
	return fmt.Sprintf("%s?%s", g.Config.AuthEndpoint, params.Encode())
}

// pkceVerifierBytes is the number of cryptographically-random bytes used to
// build a PKCE code_verifier. 32 bytes base64url-nopad-encode to a 43-char
// string drawn from the RFC 7636 unreserved set [A-Za-z0-9-_], within the
// 43..128 char bound.
const pkceVerifierBytes = 32

// GeneratePKCEPair returns a fresh RFC 7636 PKCE pair: a crypto-random
// code_verifier and its S256 code_challenge
// (base64url-nopad(SHA-256(ASCII(code_verifier)))). The verifier is a 43-char
// string from the unreserved set [A-Za-z0-9-_].
func GeneratePKCEPair() (verifier, challenge string, err error) {
	b := make([]byte, pkceVerifierBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", fmt.Errorf("generate pkce code_verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	return verifier, PKCEChallengeS256(verifier), nil
}

// PKCEChallengeS256 derives the S256 code_challenge for a given code_verifier:
// base64url-nopad(SHA-256(ASCII(code_verifier))) per RFC 7636 §4.2.
func PKCEChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURLWithPKCE builds the authorization URL with the PKCE S256 challenge
// parameters (code_challenge + code_challenge_method=S256) in addition to the
// standard authorize parameters. The shared OAuth2Provider interface is
// unchanged; this is an additive concrete-struct method.
func (g *GenericOAuth2) AuthURLWithPKCE(scopes []string, state, codeChallenge string) string {
	params := url.Values{
		"client_id":             {g.Config.ClientID},
		"redirect_uri":          {g.Config.RedirectURL},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"response_type":         {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return fmt.Sprintf("%s?%s", g.Config.AuthEndpoint, params.Encode())
}

// ExchangeCode exchanges an authorization code for tokens.
func (g *GenericOAuth2) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {g.Config.RedirectURL},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
	}
	return g.tokenRequest(ctx, data)
}

// RefreshToken refreshes an expired access token.
func (g *GenericOAuth2) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
	}
	return g.tokenRequest(ctx, data)
}

// ExchangeCodeWithVerifier exchanges an authorization code for tokens,
// presenting the PKCE code_verifier (RFC 7636 §4.5). Client credentials are
// presented according to TokenEndpointAuthStyle (Basic header for "basic",
// otherwise in the body). Additive to the OAuth2Provider interface.
func (g *GenericOAuth2) ExchangeCodeWithVerifier(ctx context.Context, code, codeVerifier string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {g.Config.RedirectURL},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
		"code_verifier": {codeVerifier},
	}
	return g.tokenRequest(ctx, data)
}

// RefreshTokenBasic refreshes an access token honoring TokenEndpointAuthStyle
// (used by confidential clients such as Twitter/X that present credentials via
// HTTP Basic auth). The provider may rotate the refresh token, so callers MUST
// persist the returned pair. Additive to the OAuth2Provider interface.
func (g *GenericOAuth2) RefreshTokenBasic(ctx context.Context, refreshToken string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
	}
	return g.tokenRequest(ctx, data)
}

// tokenRequest makes a POST to the token endpoint and parses the response.
func (g *GenericOAuth2) tokenRequest(ctx context.Context, data url.Values) (*Token, error) {
	// RFC 6749 §2.3.1 confidential-client auth: when the Basic style is
	// selected, the client_secret travels in the Authorization header rather
	// than the body, and MUST be omitted from the body.
	useBasic := g.Config.TokenEndpointAuthStyle == "basic"
	if useBasic {
		data.Del("client_secret")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Config.TokenEndpoint,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if useBasic {
		req.SetBasicAuth(url.QueryEscape(g.Config.ClientID), url.QueryEscape(g.Config.ClientSecret))
	}

	client := &http.Client{Timeout: time.Duration(g.Config.HTTPTimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request to %s: %w", g.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read truncated error body for diagnostic context
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		detail := strings.TrimSpace(string(errBody))
		if detail != "" {
			return nil, fmt.Errorf("token endpoint returned %d for %s: %s", resp.StatusCode, g.Name, detail)
		}
		return nil, fmt.Errorf("token endpoint returned %d for %s", resp.StatusCode, g.Name)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenResponseBytes)).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	var scopes []string
	if tokenResp.Scope != "" {
		scopes = strings.Split(tokenResp.Scope, " ")
	}

	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    tokenResp.TokenType,
		Scopes:       scopes,
	}, nil
}

// GoogleOAuth2Scopes returns the scopes needed for Google services.
// Single consent screen covers Gmail IMAP + Calendar + YouTube.
func GoogleOAuth2Scopes() []string {
	return []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/youtube.readonly",
	}
}
