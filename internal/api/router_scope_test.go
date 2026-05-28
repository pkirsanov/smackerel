// Spec 060 scope 1 — bearerAuthMiddleware populates Session.Scopes
// end-to-end for both scoped and legacy tokens.
package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

func TestBearerAuthMiddleware_PopulatesSessionScopes(t *testing.T) {
	deps, priv, _ := newProductionAuthDeps(t)

	cases := []struct {
		name       string
		scopes     []string
		wantScopes []string
	}{
		{"scoped_token", []string{"extension:bookmarks,history"}, []string{"extension:bookmarks,history"}},
		{"legacy_token_yields_nil_scopes", nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issued, err := auth.IssueToken(auth.IssueOptions{
				UserID:     "alice",
				TokenID:    "tok-" + tc.name,
				SigningKey: priv,
				KeyID:      deps.AuthConfig.SigningActiveKeyID,
				TTL:        time.Hour,
				Issuer:     "smackerel",
				Now:        time.Now,
				Scopes:     tc.scopes,
			})
			if err != nil {
				t.Fatalf("issue: %v", err)
			}

			var gotScopes []string
			var gotSource auth.SessionSource
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sess, ok := auth.SessionFromContext(r.Context())
				if !ok {
					t.Fatal("no session")
				}
				gotScopes = sess.Scopes
				gotSource = sess.Source
				w.WriteHeader(http.StatusOK)
			})
			mw := deps.bearerAuthMiddleware(h)

			req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if gotSource != auth.SessionSourcePerUserToken {
				t.Errorf("expected per_user_token source, got %s", gotSource)
			}
			if !slices.Equal(gotScopes, tc.wantScopes) {
				t.Errorf("Session.Scopes mismatch: got %v want %v", gotScopes, tc.wantScopes)
			}
		})
	}
}
