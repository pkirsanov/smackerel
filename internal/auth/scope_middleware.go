// Package auth — RequireScope middleware (spec 060).
//
// auth.RequireScope returns a chi-compatible middleware that gates a
// handler on the presence of all `required` scopes in the request's
// authenticated Session. Semantics per design §4:
//
//   - AND semantics: every entry in `required` must appear in
//     Session.Scopes; the FIRST missing entry is the value echoed
//     in both the 403 response body's `required` field and the
//     `required_scope` metric label.
//   - Construction-time panic when len(required) == 0 (caller bug;
//     a no-op middleware is a wiring mistake).
//   - 500 + structured ERROR log when no Session is present in the
//     context (the bearer middleware was misconfigured to run after
//     this one). No `auth_scope_rejected_total` increment in this
//     branch — wiring bugs are NOT scope rejections.
//   - SessionSourceSharedToken and SessionSourceBootstrap pass
//     through with an `auth_scope_check_bypassed_total{source=...}`
//     increment. These sources do NOT carry scopes; gating them on
//     scope membership would break the dev/test ergonomic and the
//     one-shot enrollment flow.
//   - All other sources (per-user PASETO) MUST contain every
//     required scope or be rejected with 403 `scope_required`.
//
// Spec 060 BS-002 adversarial invariant: a legacy spec-044 token
// (Session.Scopes == nil) MUST be rejected. If a future change
// causes `getScopeClaim` to fall back to a wildcard, the BS-002
// regression test will fail loudly.
package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/smackerel/smackerel/internal/metrics"
)

// RequireScope returns a middleware that enforces AND-semantics
// presence of every `required` scope in the request session. Panics
// at construction time when `required` is empty.
func RequireScope(required ...string) func(http.Handler) http.Handler {
	if len(required) == 0 {
		panic("auth: RequireScope requires at least one scope")
	}
	// Defensive copy so a caller mutating the supplied slice after
	// construction does not change middleware behavior.
	want := make([]string, len(required))
	copy(want, required)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				// Wiring bug — bearerAuthMiddleware should always
				// have populated a session by the time we reach
				// here. NOT a scope rejection.
				slog.Error("auth: RequireScope reached without session in context (middleware misconfigured)",
					"path", r.URL.Path,
					"required", want)
				writeScopeError(w, http.StatusInternalServerError, "middleware_misconfigured", nil)
				return
			}

			// Bypass: shared-token and bootstrap sources do not
			// carry scopes; they pass through with a bypass counter
			// increment for operator visibility.
			switch sess.Source {
			case SessionSourceSharedToken:
				metrics.AuthScopeCheckBypassed.WithLabelValues("shared_token").Inc()
				next.ServeHTTP(w, r)
				return
			case SessionSourceBootstrap:
				metrics.AuthScopeCheckBypassed.WithLabelValues("bootstrap").Inc()
				next.ServeHTTP(w, r)
				return
			}

			// Per-user PASETO source (or any other source that ought
			// to carry scopes): every required scope MUST be present.
			for _, scope := range want {
				if !slices.Contains(sess.Scopes, scope) {
					metrics.AuthScopeRejected.WithLabelValues(scope, sess.UserID).Inc()
					slog.Warn("auth: scope_rejected",
						"event", "scope_rejected",
						"required_scope", scope,
						"user_id", sess.UserID,
						"token_scopes", sess.Scopes,
						"endpoint", r.URL.Path,
					)
					writeScopeError(w, http.StatusForbidden, "scope_required", []string{scope})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeScopeError emits the canonical JSON response shape used by
// RequireScope. `required` is omitted from the body when nil so the
// 500 wiring-bug response does not leak the configured scopes.
func writeScopeError(w http.ResponseWriter, status int, errCode string, required []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{"error": errCode}
	if required != nil {
		body["required"] = required
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Last-resort: a write failure here usually means the
		// client hung up; we log and move on.
		slog.Warn("auth: write scope error body failed", "err", fmt.Sprint(err))
	}
}
