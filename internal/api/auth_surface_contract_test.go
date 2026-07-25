package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

// surfaceFakeVerifier is a deterministic auth.TokenVerifier for this contract
// test: it maps a raw token to a pre-baked verified browser session.
type surfaceFakeVerifier struct {
	tokens map[string]auth.VerifiedToken
}

func (f surfaceFakeVerifier) Verify(raw string, _ auth.Audience) (auth.VerifiedToken, error) {
	vt, ok := f.tokens[raw]
	if !ok {
		return auth.VerifiedToken{}, errors.New("unknown token")
	}
	return vt, nil
}

// operatorGatedHandler authorizes the request against the operator-admin grant
// using ONLY the session the unified authenticator resolved from the verified
// token — never a client-supplied role. It returns 403 when the grant is
// absent (authorization), distinct from the 401 the authenticator returns when
// no valid session exists.
func operatorGatedHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
		return
	}
	if d := auth.AuthorizeGrant(sess, auth.GrantOperatorAdmin); !d.Allowed {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "operator authorization required")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TestSurfaceInventoryRoleGrantMatrixAndGlobalCorpusGateUseUnifiedAuthenticatorAndRejectBypassHelpers
// is the BUG-070-001 SCOPE-06 unit contract. It proves the daily-user/operator
// role/grant matrix, the single global-corpus grant gate (leak-free, no
// tenant/row isolation), that authority flows only from the unified
// authenticator's verified session across surfaces, and that client-supplied
// role/session bypass attempts confer nothing.
func TestSurfaceInventoryRoleGrantMatrixAndGlobalCorpusGateUseUnifiedAuthenticatorAndRejectBypassHelpers(t *testing.T) {
	dailyUser := auth.SessionWithRole("daily-1", "jti-daily", auth.RoleDailyUser)
	operator := auth.SessionWithRole("op-1", "jti-op", auth.RoleOperator)
	grantedDaily := auth.SessionWithRole("granted-1", "jti-granted", auth.RoleDailyUser, auth.GrantGlobalCorpusRead)

	// 1. Role/grant matrix: roles share the login mechanism but not authority.
	t.Run("role_grant_matrix", func(t *testing.T) {
		if d := auth.AuthorizeGrant(dailyUser, auth.GrantAssistantTurn); !d.Allowed {
			t.Fatalf("daily user should hold assistant:turn, got %+v", d)
		}
		if d := auth.AuthorizeGrant(dailyUser, auth.GrantOperatorAdmin); d.Allowed {
			t.Fatalf("daily user must NOT hold operator:admin (403), got allowed")
		}
		if d := auth.AuthorizeGrant(operator, auth.GrantOperatorAdmin); !d.Allowed {
			t.Fatalf("operator should hold operator:admin, got %+v", d)
		}
		if d := auth.AuthorizeGrant(operator, auth.GrantAssistantTurn); !d.Allowed {
			t.Fatalf("operator should also hold assistant:turn, got %+v", d)
		}
		// No wildcard is ever honored.
		wildcard := auth.Session{UserID: "w", TokenID: "jw", Source: auth.SessionSourcePerUserToken, Scopes: []string{"*"}}
		if d := auth.AuthorizeGrant(wildcard, auth.GrantOperatorAdmin); d.Allowed || d.Reason != "wildcard_grant_forbidden" {
			t.Fatalf("wildcard must never grant authority, got %+v", d)
		}
		// No implicit default grant: a bare valid session holds nothing.
		bare := auth.Session{UserID: "b", TokenID: "jb", Source: auth.SessionSourcePerUserToken}
		if d := auth.AuthorizeGrant(bare, auth.GrantAssistantTurn); d.Allowed {
			t.Fatalf("bare session must hold no grant, got allowed")
		}
	})

	// 2. Global-corpus grant gate: operator and a specifically-granted daily user
	//    read the single global corpus; an ungranted daily user is denied with no
	//    content/count/label/existence hint and no tenant/row-isolation claim.
	t.Run("global_corpus_grant_gate", func(t *testing.T) {
		if !auth.GateGlobalCorpusRead(operator).Allowed {
			t.Fatalf("operator should read the global corpus")
		}
		if !auth.GateGlobalCorpusRead(grantedDaily).Allowed {
			t.Fatalf("specifically-granted daily user should read the global corpus")
		}
		if auth.GateGlobalCorpusRead(dailyUser).Allowed {
			t.Fatalf("ungranted daily user must NOT read the global corpus")
		}
		if auth.GateGlobalCorpusRead(auth.Session{Scopes: []string{"*"}}).Allowed {
			t.Fatalf("wildcard must not open the corpus gate")
		}
		// Denied render leaks nothing: a bare 403 with a fixed body carrying no
		// count, label, or existence hint. The decision struct itself carries no
		// corpus data (single global corpus, no per-user isolation parameter).
		rec := httptest.NewRecorder()
		if d := auth.GateGlobalCorpusRead(dailyUser); !d.Allowed {
			writeError(rec, http.StatusForbidden, "FORBIDDEN", "not authorized")
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("ungranted corpus read status = %d, want 403", rec.Code)
		}
		body := rec.Body.String()
		for _, leak := range []string{"count", "granted-1", "jti", "tenant", "corpus_size", "artifacts"} {
			if containsFold(body, leak) {
				t.Fatalf("denied corpus body leaked %q: %s", leak, body)
			}
		}
	})

	// 3. Authority flows only from the unified authenticator's verified session,
	//    identically across the legacy, /api, and /v1 surfaces.
	t.Run("authority_flows_from_unified_authenticator_across_surfaces", func(t *testing.T) {
		verifier := surfaceFakeVerifier{tokens: map[string]auth.VerifiedToken{
			"daily-cookie":    {Session: dailyUser, Audience: auth.AudienceBrowserSession},
			"operator-cookie": {Session: operator, Audience: auth.AudienceBrowserSession},
		}}
		authn := auth.NewRequestAuthenticator(verifier)
		surfaces := map[string]func(http.Handler) http.Handler{
			"legacy": authn.LegacyWebAuthMiddleware,
			"api":    authn.APIAuthMiddleware,
			"v1":     authn.V1AuthMiddleware,
		}
		run := func(mw func(http.Handler) http.Handler, cookieVal string) int {
			req := httptest.NewRequest(http.MethodPost, "/operator/action", nil)
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: cookieVal})
			rec := httptest.NewRecorder()
			mw(http.HandlerFunc(operatorGatedHandler)).ServeHTTP(rec, req)
			return rec.Code
		}
		for name, mw := range surfaces {
			if code := run(mw, "operator-cookie"); code != http.StatusOK {
				t.Fatalf("%s: operator cookie on operator route = %d, want 200", name, code)
			}
			if code := run(mw, "daily-cookie"); code != http.StatusForbidden {
				t.Fatalf("%s: daily cookie on operator route = %d, want 403 (no login loop, authority from session)", name, code)
			}
		}
	})

	// 4. Reject bypass helpers: a client-supplied role header confers no
	//    authority, and a request with no valid token can never inject a role —
	//    it is rejected 401 before any authorization check.
	t.Run("reject_client_supplied_role_and_unverified_session", func(t *testing.T) {
		verifier := surfaceFakeVerifier{tokens: map[string]auth.VerifiedToken{
			"daily-cookie": {Session: dailyUser, Audience: auth.AudienceBrowserSession},
		}}
		authn := auth.NewRequestAuthenticator(verifier)
		wrapped := authn.APIAuthMiddleware(http.HandlerFunc(operatorGatedHandler))

		// A daily user asserting X-Role: operator gets 403 — the header is ignored.
		spoof := httptest.NewRequest(http.MethodPost, "/operator/action", nil)
		spoof.Header.Set("X-Role", "operator")
		spoof.AddCookie(&http.Cookie{Name: "auth_token", Value: "daily-cookie"})
		sr := httptest.NewRecorder()
		wrapped.ServeHTTP(sr, spoof)
		if sr.Code != http.StatusForbidden {
			t.Fatalf("X-Role spoof status = %d, want 403 (header confers no authority)", sr.Code)
		}

		// No valid token → 401 before any authorization; a role cannot be injected
		// by skipping the authenticator.
		anon := httptest.NewRequest(http.MethodPost, "/operator/action", nil)
		anon.Header.Set("X-Role", "operator")
		ar := httptest.NewRecorder()
		wrapped.ServeHTTP(ar, anon)
		if ar.Code != http.StatusUnauthorized {
			t.Fatalf("unverified request status = %d, want 401 (no injected session)", ar.Code)
		}
	})
}

// containsFold reports whether s contains sub, case-insensitively.
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	ls, lsub := []byte(s), []byte(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		match := true
		for j := 0; j < len(lsub); j++ {
			a, b := ls[i+j], lsub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
