package twitter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// fakeFlowStore is an in-memory oauthFlowStore for the authorize-flow
// orchestration tests. It mirrors the real *oauthStore semantics that the
// orchestration depends on: SaveState/ConsumeState with delete-on-consume + a
// TTL check, and owner-scoped token persistence. The at-rest AES-256-GCM
// encryption and the real DB round-trip are covered separately (the Scope A
// oauth_store_test.go crypto round-trip + the integration migration pass), so
// these tests stay CI-runnable without a database while still proving the
// begin/finalize/status logic end to end against a real httptest token
// endpoint.
type fakeFlowStore struct {
	states map[string]pkceState
	tokens map[string]*auth.Token
}

func newFakeFlowStore() *fakeFlowStore {
	return &fakeFlowStore{
		states: map[string]pkceState{},
		tokens: map[string]*auth.Token{},
	}
}

func (f *fakeFlowStore) SaveState(_ context.Context, st pkceState) error {
	f.states[st.StateToken] = st
	return nil
}

// ConsumeState deletes the row on lookup (mirroring the real store's SQL
// DELETE ... RETURNING) and then enforces the TTL, so a stale binding cannot be
// replayed even when expired.
func (f *fakeFlowStore) ConsumeState(_ context.Context, stateToken string) (pkceState, error) {
	st, ok := f.states[stateToken]
	if !ok {
		return pkceState{}, fmt.Errorf("twitter oauth state %q not found", stateToken)
	}
	delete(f.states, stateToken)
	if time.Now().After(st.ExpiresAt) {
		return pkceState{}, fmt.Errorf("twitter oauth state %q expired at %s", stateToken, st.ExpiresAt.Format(time.RFC3339))
	}
	return st, nil
}

func (f *fakeFlowStore) SaveTokens(_ context.Context, owner string, t *auth.Token) error {
	cp := *t
	f.tokens[owner] = &cp
	return nil
}

// GetTokens returns a copy of the persisted token for owner, or an error when
// none exists (mirroring the real *oauthStore's no-rows error). This makes the
// fake a complete userContextTokenStore so the Scope C Pass 2 token-manager /
// refresh tests can drive it without a database.
func (f *fakeFlowStore) GetTokens(_ context.Context, owner string) (*auth.Token, error) {
	t, ok := f.tokens[owner]
	if !ok {
		return nil, fmt.Errorf("no twitter user-context token persisted for owner %q", owner)
	}
	cp := *t
	return &cp, nil
}

func (f *fakeFlowStore) HasValidUserContext(_ context.Context, owner string) (bool, error) {
	_, ok := f.tokens[owner]
	return ok, nil
}

// Compile-time proof that the production store satisfies the same surface the
// CLI orchestration depends on, so the fake is a faithful stand-in.
var _ oauthFlowStore = (*oauthStore)(nil)
var _ oauthFlowStore = (*fakeFlowStore)(nil)

// Compile-time proof that the fake also satisfies the narrow token-manager
// store surface (GetTokens + SaveTokens), so the Scope C Pass 2 refresh tests
// can back a real userContextManager with it.
var _ userContextTokenStore = (*fakeFlowStore)(nil)

const (
	testOAuthClientID    = "test-client-id"
	testOAuthSecret      = "test-secret"
	testOAuthRedirectURL = "http://127.0.0.1/callback"
)

func testOAuthCfg() TwitterOAuthConfig {
	return TwitterOAuthConfig{
		ClientID:           testOAuthClientID,
		ClientSecret:       testOAuthSecret,
		RedirectURL:        testOAuthRedirectURL,
		HTTPTimeoutSeconds: 15,
	}
}

// TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL proves SCN-BUG-056-002-006:
// authorize-begin generates a verifier+S256 challenge+state, persists a state
// row with a 15-minute TTL carrying the verifier, and builds an authorize URL
// that carries code_challenge + code_challenge_method=S256 + state + the LOCKED
// scopes — while leaking neither the verifier nor the client secret. It also
// proves the fail-loud guard when client_id / redirect_url is empty.
func TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL(t *testing.T) {
	t.Parallel()

	fake := newFakeFlowStore()
	fixedNow := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	svc := &AuthorizeService{
		store:    fake,
		provider: newTwitterOAuthProvider(testOAuthCfg()),
		owner:    "owner-1",
		now:      func() time.Time { return fixedNow },
	}

	res, err := svc.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if res.State == "" {
		t.Fatal("Begin returned an empty state token")
	}

	// A state row is persisted with the verifier, owner, and a 15-min TTL.
	st, ok := fake.states[res.State]
	if !ok {
		t.Fatalf("no twitter_oauth_states row persisted for state %q", res.State)
	}
	if st.CodeVerifier == "" {
		t.Fatal("persisted state row has an empty code_verifier")
	}
	if st.OwnerUserID != "owner-1" {
		t.Fatalf("persisted owner mismatch: got %q want owner-1", st.OwnerUserID)
	}
	if st.ConnectorID != twitterConnectorID {
		t.Fatalf("persisted connector_id mismatch: got %q want %q", st.ConnectorID, twitterConnectorID)
	}
	if wantExpiry := fixedNow.Add(twitterOAuthStateTTL); !st.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("state TTL mismatch: got %s want %s (15-min window)", st.ExpiresAt, wantExpiry)
	}

	// The authorize URL is the LOCKED endpoint and carries the PKCE S256 params.
	u, err := url.Parse(res.AuthURL)
	if err != nil {
		t.Fatalf("parse authorize URL %q: %v", res.AuthURL, err)
	}
	if endpoint := u.Scheme + "://" + u.Host + u.Path; endpoint != twitterAuthorizeEndpoint {
		t.Fatalf("authorize endpoint mismatch: got %q want %q", endpoint, twitterAuthorizeEndpoint)
	}
	q := u.Query()
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method mismatch: got %q want S256", got)
	}
	if got := q.Get("state"); got != res.State {
		t.Fatalf("state param mismatch: URL has %q, result has %q", got, res.State)
	}
	if got := q.Get("response_type"); got != "code" {
		t.Fatalf("response_type mismatch: got %q want code", got)
	}
	if got := q.Get("redirect_uri"); got != testOAuthRedirectURL {
		t.Fatalf("redirect_uri mismatch: got %q want %q", got, testOAuthRedirectURL)
	}
	// The challenge MUST be the S256 of the PERSISTED verifier (binds the URL to
	// the stored state — non-tautological).
	if got, want := q.Get("code_challenge"), auth.PKCEChallengeS256(st.CodeVerifier); got != want {
		t.Fatalf("code_challenge does not match S256(persisted verifier): got %q want %q", got, want)
	}
	// Scopes are exact + space-joined, and every locked scope is present.
	if got, want := q.Get("scope"), strings.Join(twitterOAuthScopes(), " "); got != want {
		t.Fatalf("scope mismatch: got %q want %q", got, want)
	}
	for _, sc := range []string{"offline.access", "tweet.read", "users.read", "bookmark.read", "like.read"} {
		if !strings.Contains(q.Get("scope"), sc) {
			t.Fatalf("locked scope %q missing from %q", sc, q.Get("scope"))
		}
	}

	// The verifier and the client secret MUST NOT leak into the URL.
	if strings.Contains(res.AuthURL, st.CodeVerifier) {
		t.Fatal("code_verifier leaked into the authorize URL")
	}
	if strings.Contains(res.AuthURL, testOAuthSecret) {
		t.Fatal("client secret leaked into the authorize URL")
	}

	// Fail loud when client_id is empty.
	svcNoClient := &AuthorizeService{
		store:    newFakeFlowStore(),
		provider: newTwitterOAuthProvider(TwitterOAuthConfig{RedirectURL: testOAuthRedirectURL}),
		owner:    "owner-1",
		now:      time.Now,
	}
	if _, err := svcNoClient.Begin(context.Background()); err == nil {
		t.Fatal("expected authorize-begin to fail loud when oauth_client_id is empty")
	} else if !strings.Contains(err.Error(), "oauth_client_id") {
		t.Fatalf("fail-loud error must name oauth_client_id, got: %v", err)
	}

	// Fail loud when redirect_url is empty.
	svcNoRedirect := &AuthorizeService{
		store:    newFakeFlowStore(),
		provider: newTwitterOAuthProvider(TwitterOAuthConfig{ClientID: testOAuthClientID}),
		owner:    "owner-1",
		now:      time.Now,
	}
	if _, err := svcNoRedirect.Begin(context.Background()); err == nil {
		t.Fatal("expected authorize-begin to fail loud when oauth_redirect_url is empty")
	} else if !strings.Contains(err.Error(), "oauth_redirect_url") {
		t.Fatalf("fail-loud error must name oauth_redirect_url, got: %v", err)
	}
}

// TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted proves
// SCN-BUG-056-002-007: authorize-finalize consumes (TTL-checks + DELETEs) the
// state, exchanges the code+verifier at the token endpoint with confidential
// HTTP Basic auth (client_secret omitted from the body), persists the returned
// access+refresh pair, and a second finalize for the same state fails (the row
// is gone). The fixture is a real httptest.Server emulating Twitter's
// /2/oauth2/token — NOT a mock-and-mislabel, and NOT the real Twitter API.
func TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted(t *testing.T) {
	t.Parallel()

	var gotUser, gotPass string
	var gotBasicOK bool
	var gotForm url.Values
	var gotRawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotBasicOK = r.BasicAuth()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		gotRawBody = string(body)
		gotForm, _ = url.ParseQuery(gotRawBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"token_type":"bearer","expires_in":7200,`+
			`"access_token":"NEW-ACCESS-TOKEN","refresh_token":"NEW-REFRESH-TOKEN",`+
			`"scope":"offline.access tweet.read users.read bookmark.read like.read"}`)
	}))
	defer srv.Close()

	fake := newFakeFlowStore()
	provider := newTwitterOAuthProvider(testOAuthCfg())
	provider.Config.TokenEndpoint = srv.URL // redirect the exchange at the fixture
	svc := &AuthorizeService{store: fake, provider: provider, owner: "owner-1", now: time.Now}

	// Begin persists a real state + verifier we can assert is presented.
	res, err := svc.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	wantVerifier := fake.states[res.State].CodeVerifier
	if wantVerifier == "" {
		t.Fatal("precondition: persisted verifier is empty")
	}

	tok, err := svc.Finalize(context.Background(), res.State, "auth-code-xyz")
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// The returned + persisted token match the fixture response.
	if tok.AccessToken != "NEW-ACCESS-TOKEN" || tok.RefreshToken != "NEW-REFRESH-TOKEN" {
		t.Fatalf("returned token mismatch: %+v", tok)
	}
	persisted, ok := fake.tokens["owner-1"]
	if !ok {
		t.Fatal("no token persisted for owner-1 after finalize")
	}
	if persisted.AccessToken != "NEW-ACCESS-TOKEN" || persisted.RefreshToken != "NEW-REFRESH-TOKEN" {
		t.Fatalf("persisted token mismatch: %+v", persisted)
	}

	// The exchange used confidential-client HTTP Basic auth with the configured
	// id/secret, presented the stored code_verifier, and omitted client_secret
	// from the body.
	if !gotBasicOK || gotUser != testOAuthClientID || gotPass != testOAuthSecret {
		t.Fatalf("token endpoint Basic auth mismatch: ok=%v user=%q pass=%q", gotBasicOK, gotUser, gotPass)
	}
	if got := gotForm.Get("code_verifier"); got != wantVerifier {
		t.Fatalf("code_verifier presented to token endpoint mismatch: got %q want %q", got, wantVerifier)
	}
	if got := gotForm.Get("grant_type"); got != "authorization_code" {
		t.Fatalf("grant_type mismatch: got %q want authorization_code", got)
	}
	if got := gotForm.Get("code"); got != "auth-code-xyz" {
		t.Fatalf("code mismatch: got %q want auth-code-xyz", got)
	}
	if got := gotForm.Get("redirect_uri"); got != testOAuthRedirectURL {
		t.Fatalf("redirect_uri mismatch: got %q want %q", got, testOAuthRedirectURL)
	}
	if gotForm.Has("client_secret") {
		t.Fatalf("client_secret MUST be omitted from the body under Basic auth; body=%q", gotRawBody)
	}

	// Delete-on-consume: the state row is gone and a second finalize fails loud.
	if _, ok := fake.states[res.State]; ok {
		t.Fatal("state row was not deleted on consume")
	}
	if _, err := svc.Finalize(context.Background(), res.State, "auth-code-xyz"); err == nil {
		t.Fatal("expected a second finalize for a consumed state to fail loud")
	}
}

// TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud proves the
// fail-loud path: an unknown state token and an expired state token both make
// finalize return a non-nil error, and the token endpoint is NEVER contacted in
// either case (the exchange is gated behind a successful state consume).
func TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud(t *testing.T) {
	t.Parallel()

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"access_token":"should-not-be-reached"}`)
	}))
	defer srv.Close()

	// Unknown state → fail loud, no exchange.
	fakeUnknown := newFakeFlowStore()
	providerUnknown := newTwitterOAuthProvider(testOAuthCfg())
	providerUnknown.Config.TokenEndpoint = srv.URL
	svcUnknown := &AuthorizeService{store: fakeUnknown, provider: providerUnknown, owner: "owner-1", now: time.Now}
	if _, err := svcUnknown.Finalize(context.Background(), "not-a-real-state", "code"); err == nil {
		t.Fatal("expected finalize to fail loud for an unknown state token")
	}

	// Expired state → fail loud, no exchange. now() 20 min in the past makes the
	// 15-min TTL land 5 min in the past relative to the real clock.
	fakeExpired := newFakeFlowStore()
	providerExpired := newTwitterOAuthProvider(testOAuthCfg())
	providerExpired.Config.TokenEndpoint = srv.URL
	past := time.Now().Add(-20 * time.Minute)
	svcExpired := &AuthorizeService{store: fakeExpired, provider: providerExpired, owner: "owner-1", now: func() time.Time { return past }}
	res, err := svcExpired.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin (expired setup): %v", err)
	}
	if _, err := svcExpired.Finalize(context.Background(), res.State, "code"); err == nil {
		t.Fatal("expected finalize to fail loud for an expired state token")
	}
	// The expired row is still deleted on consume (no replay).
	if _, ok := fakeExpired.states[res.State]; ok {
		t.Fatal("expired state row was not deleted on consume")
	}

	if hits != 0 {
		t.Fatalf("token endpoint must NOT be contacted for unknown/expired state; got %d hit(s)", hits)
	}
}

// TestTwitterAuthorize_StatusReflectsPersistedToken proves SCN-BUG-056-002-008:
// authorize-status reports absence before any token is persisted and presence
// after, scoped to the owner.
func TestTwitterAuthorize_StatusReflectsPersistedToken(t *testing.T) {
	t.Parallel()

	fake := newFakeFlowStore()
	svc := &AuthorizeService{
		store:    fake,
		provider: newTwitterOAuthProvider(testOAuthCfg()),
		owner:    "owner-1",
		now:      time.Now,
	}

	present, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status (before): %v", err)
	}
	if present {
		t.Fatal("expected NOT authorized before any token is persisted")
	}

	fake.tokens["owner-1"] = &auth.Token{
		AccessToken:  "a",
		RefreshToken: "r",
		TokenType:    "bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	present, err = svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status (after): %v", err)
	}
	if !present {
		t.Fatal("expected AUTHORIZED after a token is persisted for the owner")
	}

	// Owner scoping: a different owner is still unauthorized.
	other := &AuthorizeService{store: fake, provider: svc.provider, owner: "owner-2", now: time.Now}
	present, err = other.Status(context.Background())
	if err != nil {
		t.Fatalf("Status (owner-2): %v", err)
	}
	if present {
		t.Fatal("owner-2 must be unauthorized (token presence is owner-scoped)")
	}
}
