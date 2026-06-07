// Spec 060 / spec 058 regression — the extension ingest endpoint must
// accept a per-user token whose single canonical scope claim is
// "extension:bookmarks,history" (spec 060 spec.md L15/L70/L138; spec 058
// design.md L295/L330/L498/L684). The PASETO `scope` claim carries that
// value as ONE comma-joined element (getScopeClaim does NOT split on ","),
// and auth.RequireScope matches scopes with exact slices.Contains. A
// regression that splits the gate into two separate scopes —
// RequireScope("extension:bookmarks", "extension:history") — therefore
// 403s EVERY real per-user token, because neither bare substring is an
// element of ["extension:bookmarks,history"]. The defect shipped only
// because dev/test shared-token / bootstrap sessions bypass the scope
// gate; per-user PASETO tokens (the production extension flow) hit it.
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// TestExtensionIngest_CanonicalScopeReachesHandler fails (403) if the
// extension ingest scope gate is mis-wired as two separate scopes.
func TestExtensionIngest_CanonicalScopeReachesHandler(t *testing.T) {
	deps, priv, _ := newProductionAuthDeps(t)

	reached := false
	deps.ExtensionIngestHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	r := NewRouter(deps)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-ext-canonical",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     []string{"extension:bookmarks,history"},
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/extension/ingest", nil)
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("canonical-scope token was 403'd — extension ingest scope gate is mis-wired; "+
			"it MUST be auth.RequireScope(\"extension:bookmarks,history\") as ONE scope, not two separate scopes. body=%s",
			rec.Body.String())
	}
	if !reached {
		t.Fatalf("expected the ingest handler to be reached for a canonical-scope token, got code=%d body=%s",
			rec.Code, rec.Body.String())
	}
}

// TestExtensionIngest_MissingScopeRejected is the adversarial twin: a
// per-user token WITHOUT the canonical scope (a legacy spec-044 token
// with no `scope` claim) MUST still be rejected 403 — proving the gate
// keeps enforcing and the fix does not fling the endpoint open.
func TestExtensionIngest_MissingScopeRejected(t *testing.T) {
	deps, priv, _ := newProductionAuthDeps(t)

	deps.ExtensionIngestHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r := NewRouter(deps)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "mallory",
		TokenID:    "tok-ext-noscope",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     nil, // legacy spec-044 token: no scope claim
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/extension/ingest", nil)
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a token lacking extension:bookmarks,history, got code=%d body=%s",
			rec.Code, rec.Body.String())
	}
}
