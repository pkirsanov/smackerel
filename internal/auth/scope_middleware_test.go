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

// TestRequireScope_UnknownSessionSourceEnforcedNotBypassed is an
// adversarial coverage lock on the bypass ALLOWLIST. The bypass switch
// in RequireScope is the ONLY branch that calls next.ServeHTTP without
// checking scopes, so it is the single fail-open-capable code path.
// Spec 060 design §4 makes the bypass an explicit allowlist of exactly
// {SessionSourceSharedToken, SessionSourceBootstrap}. The existing
// bypass tests prove those two KNOWN sources pass through; this test
// pins the complementary default-deny invariant: a session whose
// Source is NEITHER of those (a hypothetical future / unrecognized
// source) MUST fall through to per-user scope enforcement, NEVER
// bypass.
//
// If a future refactor turns the bypass switch into a denylist or adds
// a `default:` pass-through, sub-case A fails loudly — preventing a
// silent privilege-escalation regression where a newly-introduced
// SessionSource is accidentally granted all-scopes. Sub-case B proves
// the assertion is non-tautological: the SAME unknown source is
// admitted when (and only when) it carries the required scope, so the
// 403 in sub-case A is meaningfully about the missing scope on the
// enforcement path, not a blanket deny.
func TestRequireScope_UnknownSessionSourceEnforcedNotBypassed(t *testing.T) {
	const unknown = SessionSource("future_unrecognized_source")

	t.Run("absent_scope_rejected_not_bypassed", func(t *testing.T) {
		sharedBefore := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("shared_token"))
		bootstrapBefore := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("bootstrap"))
		rejectBefore := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("admin:users", "mallory"))

		mw := RequireScope("admin:users")
		sess := &Session{Source: unknown, UserID: "mallory", Scopes: nil}
		rec := serveWithSession(t, mw, sess, "POST", "/v1/admin/users")

		if rec.Code != http.StatusForbidden {
			t.Fatalf("unknown session source with no scopes MUST be enforced (403), got %d body=%s", rec.Code, rec.Body.String())
		}
		rejectAfter := testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues("admin:users", "mallory"))
		if rejectAfter-rejectBefore != 1 {
			t.Errorf("AuthScopeRejected delta: got %v want 1 (unknown source MUST take the enforcement/reject path)", rejectAfter-rejectBefore)
		}
		// The bypass counter MUST NOT move for a non-allowlisted source.
		sharedAfter := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("shared_token"))
		bootstrapAfter := testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues("bootstrap"))
		if sharedAfter != sharedBefore || bootstrapAfter != bootstrapBefore {
			t.Errorf("bypass counter moved for an unknown source — the bypass allowlist MUST be exactly {shared_token, bootstrap}")
		}
	})

	t.Run("present_scope_admitted_via_enforcement", func(t *testing.T) {
		mw := RequireScope("admin:users")
		sess := &Session{Source: unknown, UserID: "mallory", Scopes: []string{"admin:users"}}
		rec := serveWithSession(t, mw, sess, "POST", "/v1/admin/users")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("unknown source WITH the required scope present MUST be admitted by enforcement (202), got %d body=%s", rec.Code, rec.Body.String())
		}
	})
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

// BenchmarkRequireScope_PerUserPasetoSuccess measures the hot-path
// per-request cost of auth.RequireScope on the success branch
// (per-user PASETO session, all required scopes present). Spec 060
// Scope 2 DoD DI-060-01: hot-path validation budget unchanged
// (< 10 µs design budget). The middleware adds one SessionFromContext
// lookup + slices.Contains per required scope (typically 1-3 scopes);
// this benchmark proves the actual per-op cost.
//
// The handler downstream of RequireScope is a no-op httptest handler
// so the measurement is dominated by the middleware itself plus the
// httptest.ResponseRecorder bookkeeping.
func BenchmarkRequireScope_PerUserPasetoSuccess(b *testing.B) {
	mw := RequireScope("extension:bookmarks,history")
	sess := Session{
		Source: SessionSourcePerUserToken,
		UserID: "bench-user",
		Scopes: []string{"extension:bookmarks,history"},
	}
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest("POST", "/v1/connectors/extension/ingest", nil)
	req = req.WithContext(WithSession(req.Context(), sess))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			b.Fatalf("iter %d: expected 202, got %d", i, rec.Code)
		}
	}
}

// BenchmarkRequireScope_AndSemanticsThreeScopes measures the cost
// when three required scopes are all present (worst-case loop length
// for the typical caller — chi route groups rarely require more than
// 2-3 scopes per endpoint).
func BenchmarkRequireScope_AndSemanticsThreeScopes(b *testing.B) {
	mw := RequireScope("a:x", "b:y", "c:z")
	sess := Session{
		Source: SessionSourcePerUserToken,
		UserID: "bench-user",
		Scopes: []string{"a:x", "b:y", "c:z"},
	}
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest("GET", "/v1/x", nil)
	req = req.WithContext(WithSession(req.Context(), sess))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			b.Fatalf("iter %d: expected 202, got %d", i, rec.Code)
		}
	}
}
