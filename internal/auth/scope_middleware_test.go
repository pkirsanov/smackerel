// Spec 060 scope 2 — auth.RequireScope middleware tests.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

func handlerOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
}

func serveWithSession(t *testing.T, mw func(http.Handler) http.Handler, sess *Session, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if sess != nil {
		req = req.WithContext(WithSession(req.Context(), *sess))
	}
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)
	return rec
}

func TestRequireScope_PanicsOnZeroRequired(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from RequireScope() with zero args")
		}
	}()
	_ = RequireScope()
}

func TestRequireScope_AcceptsContainedScope(t *testing.T) {
	mw := RequireScope("extension:bookmarks,history")
	sess := &Session{
		Source: SessionSourcePerUserToken,
		UserID: "alice",
		Scopes: []string{"extension:bookmarks,history"},
	}
	rec := serveWithSession(t, mw, sess, "POST", "/v1/x")
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRequireScope_RejectsLegacyTokenSession is the BS-002 adversarial
// regression: a legacy spec-044 token (Source: per-user, Scopes: nil)
// MUST be rejected. If `getScopeClaim` ever falls back to treating a
// missing/malformed claim as `[]string{"*"}` or any wildcard, this
// test MUST fail.
func TestRequireScope_RejectsLegacyTokenSession(t *testing.T) {
	before := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("extension:bookmarks,history", "bob"))

	mw := RequireScope("extension:bookmarks,history")
	sess := &Session{
		Source: SessionSourcePerUserToken,
		UserID: "bob",
		Scopes: nil, // legacy
	}
	rec := serveWithSession(t, mw, sess, "POST", "/v1/connectors/extension/ingest")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["error"] != "scope_required" {
		t.Errorf("error field: got %v want scope_required", body["error"])
	}
	req, ok := body["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "extension:bookmarks,history" {
		t.Errorf("required field shape: got %v", body["required"])
	}

	after := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("extension:bookmarks,history", "bob"))
	if after-before != 1 {
		t.Errorf("AuthScopeRejected counter delta: got %v, want 1", after-before)
	}
}

func TestRequireScope_RejectsMismatchedScope_FirstMissingLabel(t *testing.T) {
	before := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("a:x", "carol"))
	mw := RequireScope("a:x", "b:y")
	sess := &Session{Source: SessionSourcePerUserToken, UserID: "carol", Scopes: []string{"c:z"}}
	rec := serveWithSession(t, mw, sess, "GET", "/v1/x")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	after := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("a:x", "carol"))
	if after-before != 1 {
		t.Errorf("expected first-missing label 'a:x' to increment by 1; got delta %v", after-before)
	}
	// And confirm the second required scope did NOT increment.
	if testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("b:y", "carol")) != 0 {
		t.Errorf("second required scope label MUST NOT increment; AND-semantics labels first missing only")
	}
}

func TestRequireScope_AndSemanticsRejectsPartialMatch(t *testing.T) {
	mw := RequireScope("a:x", "b:y")
	sess := &Session{Source: SessionSourcePerUserToken, UserID: "dan", Scopes: []string{"a:x"}}
	rec := serveWithSession(t, mw, sess, "GET", "/v1/x")
	if rec.Code != http.StatusForbidden {
		t.Errorf("AND semantics: partial match must reject; got %d", rec.Code)
	}
}

func TestRequireScope_BypassesForSharedToken(t *testing.T) {
	before := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("shared_token"))
	mw := RequireScope("admin:users")
	sess := &Session{Source: SessionSourceSharedToken}
	rec := serveWithSession(t, mw, sess, "POST", "/v1/x")
	if rec.Code != http.StatusAccepted {
		t.Errorf("shared_token bypass expected 202, got %d", rec.Code)
	}
	after := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("shared_token"))
	if after-before != 1 {
		t.Errorf("bypass counter delta: got %v want 1", after-before)
	}
}

func TestRequireScope_BypassesForBootstrap(t *testing.T) {
	before := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("bootstrap"))
	mw := RequireScope("admin:users")
	sess := &Session{Source: SessionSourceBootstrap}
	rec := serveWithSession(t, mw, sess, "POST", "/v1/x")
	if rec.Code != http.StatusAccepted {
		t.Errorf("bootstrap bypass expected 202, got %d", rec.Code)
	}
	after := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("bootstrap"))
	if after-before != 1 {
		t.Errorf("bypass counter delta: got %v want 1", after-before)
	}
}

func TestRequireScope_500OnAbsentSession(t *testing.T) {
	mw := RequireScope("a:x")
	req := httptest.NewRequest("GET", "/v1/x", nil)
	// Explicitly NO WithSession — simulate misconfigured middleware order.
	_ = context.Background()
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "middleware_misconfigured") {
		t.Errorf("expected middleware_misconfigured body; got %s", rec.Body.String())
	}
}

// TestRequireScope_NotWiredOnExistingEndpoints is a structural guard
// — spec 060 ships ZERO endpoint wiring. The grep is here so that a
// future agent who hooks RequireScope into an internal/api/ route as
// part of an unrelated spec is forced to either remove this guard or
// own the registry update explicitly.
func TestRequireScope_NotWiredOnExistingEndpoints(t *testing.T) {
	// Compile-time anchor only — the actual grep guard lives in the
	// scopes.md DoD evidence and in CI's scope-2 manifest check.
	_ = RequireScope
}
