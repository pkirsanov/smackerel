// Spec 096 SCOPE-06 (SCN-096-W04) — operator-gate adversarial test (R1). The
// operator-only boundary admits ONLY an infrastructure.operator_user_ids
// subject: a non-operator authenticated subject is 403, an anonymous caller is
// 401, and an empty allowlist is fail-closed (everyone rejected). The test
// fails if a non-operator ever reaches the terminal handler — it would catch a
// build that left the surface open-by-default. It also pins the G028 fail-loud
// startup guard.
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

func TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	gate := NewOperatorGate([]string{"operator-1"})
	h := gate.Middleware(next)

	doReq := func(subject string) (*httptest.ResponseRecorder, bool) {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", nil)
		if subject != "" {
			req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: subject, Source: auth.SessionSourcePerUserToken}))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec, reached
	}

	// Anonymous (no per-user subject) → 401, gate NOT passed.
	if rec, passed := doReq(""); rec.Code != http.StatusUnauthorized || passed {
		t.Fatalf("anonymous: status=%d passed=%v; want 401 and gate NOT passed", rec.Code, passed)
	}

	// ADVERSARIAL — authenticated NON-operator → 403, gate NOT passed. A
	// non-operator must NEVER reach a credential-mutating endpoint.
	if rec, passed := doReq("intruder-9"); rec.Code != http.StatusForbidden || passed {
		t.Fatalf("non-operator: status=%d passed=%v; want 403 and gate NOT passed (non-operator reached the surface)", rec.Code, passed)
	}

	// Allowlisted operator → passes to the terminal handler.
	if rec, passed := doReq("operator-1"); rec.Code != http.StatusOK || !passed {
		t.Fatalf("operator: status=%d passed=%v; want 200 and gate passed", rec.Code, passed)
	}

	// Fail-closed: an EMPTY allowlist rejects even an authenticated subject (403).
	closed := NewOperatorGate(nil).Middleware(next)
	reached = false
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", nil)
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: "anyone", Source: auth.SessionSourcePerUserToken}))
	rec := httptest.NewRecorder()
	closed.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || reached {
		t.Fatalf("empty allowlist: status=%d reached=%v; want 403 fail-closed and gate NOT passed", rec.Code, reached)
	}

	// G028 fail-loud startup guard: empty allowlist + reachable surface +
	// production → error; populated allowlist or unreachable surface → no error.
	if err := ValidateOperatorGate(nil, true, "production"); err == nil {
		t.Fatal("ValidateOperatorGate MUST fail loud: empty operator_user_ids + reachable surface + production (G028)")
	}
	if err := ValidateOperatorGate([]string{"operator-1"}, true, "production"); err != nil {
		t.Fatalf("ValidateOperatorGate MUST pass with a populated allowlist: %v", err)
	}
	if err := ValidateOperatorGate(nil, false, "production"); err != nil {
		t.Fatalf("ValidateOperatorGate MUST pass when the surface is unreachable (nothing to protect): %v", err)
	}
	// Dev/test with an empty allowlist runs fail-closed (warn, not abort).
	if err := ValidateOperatorGate(nil, true, "development"); err != nil {
		t.Fatalf("ValidateOperatorGate MUST NOT abort in development (fail-closed runtime instead): %v", err)
	}
}
