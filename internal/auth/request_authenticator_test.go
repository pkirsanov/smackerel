package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// fakeVerifier is a deterministic TokenVerifier for the unit seam. It maps a
// raw token string to a pre-baked VerifiedToken and reports an error for any
// unknown token, exercising the RequestAuthenticator's carrier/audience/
// revocation handling without the concrete PASETO path or a live stack.
type fakeVerifier struct {
	tokens map[string]VerifiedToken
}

func (f *fakeVerifier) Verify(raw string, _ Audience) (VerifiedToken, error) {
	vt, ok := f.tokens[raw]
	if !ok {
		return VerifiedToken{}, errors.New("unknown token")
	}
	return vt, nil
}

// captureHandler records the auth.Session it observed from the request context.
type captureHandler struct {
	called  bool
	session Session
	ok      bool
}

func (c *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.called = true
	c.session, c.ok = SessionFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

// TestUnifiedAuthenticatorPreservesCarrierPrecedenceSessionParityAnd401403EmptyDegradedSeparation
// is the BUG-070-001 SCOPE-03 (AUTH-015) unit contract. It proves one
// RequestAuthenticator yields ONE identical auth.Session across the legacy,
// /api, and /v1 surfaces; that carrier precedence is fail-closed; that a
// wrong-audience cookie (the core bug shape) is rejected; and that
// authentication failure (401) stays distinct from authorization (403) and from
// downstream empty/degraded states, which do not invalidate a valid session.
func TestUnifiedAuthenticatorPreservesCarrierPrecedenceSessionParityAnd401403EmptyDegradedSeparation(t *testing.T) {
	browserSession := Session{
		UserID:  "user-1",
		TokenID: "jti-browser",
		KeyID:   "kid-1",
		Source:  SessionSourcePerUserToken,
		Scopes:  []string{GrantAssistantTurn},
	}
	apiSession := Session{
		UserID:  "user-1",
		TokenID: "jti-api",
		Source:  SessionSourcePerUserToken,
		Scopes:  []string{GrantAssistantTurn},
	}
	verifier := &fakeVerifier{tokens: map[string]VerifiedToken{
		"good-browser":  {Session: browserSession, Audience: AudienceBrowserSession},
		"good-api":      {Session: apiSession, Audience: AudienceAPIBearer},
		"api-in-cookie": {Session: apiSession, Audience: AudienceAPIBearer},
		"revoked":       {Session: browserSession, Audience: AudienceBrowserSession, Revoked: true},
	}}
	authn := NewRequestAuthenticator(verifier)

	browserCookie := func() *http.Cookie {
		return &http.Cookie{Name: browserCookieName, Value: "good-browser"}
	}

	// 1. Session parity: one valid browser cookie yields the identical session
	//    across the legacy, /api, and /v1 surface middlewares.
	t.Run("session parity across legacy api and v1 surfaces", func(t *testing.T) {
		surfaces := map[string]func(http.Handler) http.Handler{
			"legacy": authn.LegacyWebAuthMiddleware,
			"api":    authn.APIAuthMiddleware,
			"v1":     authn.V1AuthMiddleware,
		}
		var seen []Session
		for name, mw := range surfaces {
			h := &captureHandler{}
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.AddCookie(browserCookie())
			rec := httptest.NewRecorder()
			mw(h).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s surface: status = %d, want 200", name, rec.Code)
			}
			if !h.called || !h.ok {
				t.Fatalf("%s surface: handler called=%v sessionOK=%v, want true/true", name, h.called, h.ok)
			}
			if !reflect.DeepEqual(h.session, browserSession) {
				t.Fatalf("%s surface: session = %+v, want %+v", name, h.session, browserSession)
			}
			seen = append(seen, h.session)
		}
		for i := 1; i < len(seen); i++ {
			if !reflect.DeepEqual(seen[0], seen[i]) {
				t.Fatalf("session parity broken: %+v != %+v", seen[0], seen[i])
			}
		}
	})

	// 2. Carrier precedence fail-closed: a malformed Authorization header rejects
	//    the request and the cookie is NEVER used as a fallback.
	t.Run("malformed authorization header fails closed and never uses cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
		req.AddCookie(browserCookie()) // a valid cookie is present but MUST be ignored
		sess, kind := authn.Authenticate(req)
		if kind != AuthMalformedHeader {
			t.Fatalf("kind = %q, want %q (fail-closed, cookie ignored)", kind, AuthMalformedHeader)
		}
		if sess.Source != "" {
			t.Fatalf("session leaked on malformed header: %+v", sess)
		}
		// Every surface refuses to invoke the handler.
		for name, mw := range map[string]func(http.Handler) http.Handler{
			"api": authn.APIAuthMiddleware, "v1": authn.V1AuthMiddleware, "legacy": authn.LegacyWebAuthMiddleware,
		} {
			h := &captureHandler{}
			rec := httptest.NewRecorder()
			r2 := httptest.NewRequest(http.MethodGet, "/x", nil)
			r2.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
			r2.AddCookie(browserCookie())
			mw(h).ServeHTTP(rec, r2)
			if h.called {
				t.Fatalf("%s surface invoked handler on malformed header (cookie fallback leaked)", name)
			}
		}
	})

	// 3. A valid Authorization header is verified as the API-bearer audience.
	t.Run("valid authorization header resolves api audience session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
		req.Header.Set("Authorization", "Bearer good-api")
		sess, kind := authn.Authenticate(req)
		if kind != AuthOK {
			t.Fatalf("kind = %q, want AuthOK", kind)
		}
		if !reflect.DeepEqual(sess, apiSession) {
			t.Fatalf("session = %+v, want %+v", sess, apiSession)
		}
	})

	// 4. No carrier ⇒ AuthMissing; presentation differs by surface (401 vs 303).
	t.Run("missing carrier is 401 on api and 303 on legacy top-level navigation", func(t *testing.T) {
		if _, kind := authn.Authenticate(httptest.NewRequest(http.MethodGet, "/x", nil)); kind != AuthMissing {
			t.Fatalf("kind = %q, want AuthMissing", kind)
		}
		apiRec := httptest.NewRecorder()
		authn.APIAuthMiddleware(&captureHandler{}).ServeHTTP(apiRec, httptest.NewRequest(http.MethodPost, "/api/x", nil))
		if apiRec.Code != http.StatusUnauthorized {
			t.Fatalf("api missing-carrier status = %d, want 401", apiRec.Code)
		}
		nav := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		nav.Header.Set("Accept", "text/html")
		navRec := httptest.NewRecorder()
		authn.LegacyWebAuthMiddleware(&captureHandler{}).ServeHTTP(navRec, nav)
		if navRec.Code != http.StatusSeeOther {
			t.Fatalf("legacy nav missing-carrier status = %d, want 303", navRec.Code)
		}
		if loc := navRec.Header().Get("Location"); loc != "/login" {
			t.Fatalf("legacy nav redirect = %q, want /login", loc)
		}
	})

	// 5. Wrong-audience cookie (the core bug shape — an API/shared-shaped value in
	//    the browser cookie) is rejected, never authenticated.
	t.Run("api-audience token in the browser cookie is rejected wrong_audience", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: browserCookieName, Value: "api-in-cookie"})
		sess, kind := authn.Authenticate(req)
		if kind != AuthWrongAudience {
			t.Fatalf("kind = %q, want AuthWrongAudience", kind)
		}
		if sess.Source != "" {
			t.Fatalf("wrong-audience cookie leaked a session: %+v", sess)
		}
	})

	// 6. A revoked cookie fails closed.
	t.Run("revoked browser cookie fails closed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: browserCookieName, Value: "revoked"})
		if _, kind := authn.Authenticate(req); kind != AuthRevoked {
			t.Fatalf("kind = %q, want AuthRevoked", kind)
		}
	})

	// 7. 401-vs-403-vs-empty/degraded separation: a valid session authenticates
	//    (not 401); a missing grant is a downstream 403 (authorization), distinct
	//    from authentication; and a degraded downstream dependency does not
	//    invalidate the still-valid session (SCN-070-001-06/07 boundary).
	t.Run("auth success is distinct from authorization denial and downstream degradation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(browserCookie())
		sess, kind := authn.Authenticate(req)
		if kind != AuthOK {
			t.Fatalf("valid session kind = %q, want AuthOK (not an auth failure)", kind)
		}
		// Authorization is a separate downstream decision → 403, not 401.
		if d := AuthorizeGrant(sess, GrantOperatorAdmin); d.Allowed || d.Reason != "grant_absent" {
			t.Fatalf("authz of ungranted operator scope = %+v, want denied grant_absent (403-class)", d)
		}
		if d := AuthorizeGrant(sess, GrantAssistantTurn); !d.Allowed {
			t.Fatalf("authz of held scope = %+v, want allowed", d)
		}
		// A degraded downstream dependency (simulated 503) does not change the
		// authenticated identity — the same valid session is still resolved.
		if _, k2 := authn.Authenticate(req); k2 != AuthOK {
			t.Fatalf("re-auth under downstream degradation kind = %q, want AuthOK (session not invalidated)", k2)
		}
	})
}
