// Spec 096 SCOPE-06 — the operator gate (R1) in front of the credential-mutating
// /v1/admin/model-connections* surface (design §11.4).
//
// R1 RESOLUTION. Design §11.4 left the exact operator-gate mechanism as a
// residual decision "to confirm against the live auth model in plan". Resolved
// here: the operator gate is the SST `infrastructure.operator_user_ids`
// allowlist checked against the existing claim-bound bearer/session subject
// (auth.UserIDFromContext — the spec-044 actor, NEVER a body field), layered
// over the existing bearerAuthMiddleware. Today's shared-token "operator" read
// is NOT sufficient for a credential-mutating surface, so a shared-token /
// dev-bypass session (empty per-user subject) is rejected.
//
// FAIL-CLOSED (binding). An EMPTY allowlist locks everyone out — there is never
// an open-by-default operator surface in ANY environment: an authenticated
// non-operator is 403, an anonymous caller is 401, and an allowlisted operator
// passes. The production startup fail-loud (G028) that refuses to serve the
// surface with an empty allowlist lives in ValidateOperatorGate.
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/auth"
)

// OperatorGate is the SST operator_user_ids allowlist enforced as middleware.
type OperatorGate struct {
	operators map[string]struct{}
}

// NewOperatorGate builds the gate from the SST operator_user_ids allowlist.
// Empty/whitespace ids are dropped; an empty result is a fail-closed gate.
func NewOperatorGate(operatorUserIDs []string) *OperatorGate {
	set := make(map[string]struct{}, len(operatorUserIDs))
	for _, id := range operatorUserIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return &OperatorGate{operators: set}
}

// Configured reports whether at least one operator id is present.
func (g *OperatorGate) Configured() bool { return len(g.operators) > 0 }

// IsOperator reports whether subject is an allowlisted operator. An empty
// subject (anonymous / shared-token / dev-bypass — no per-user identity) is
// never an operator.
func (g *OperatorGate) IsOperator(subject string) bool {
	if subject == "" {
		return false
	}
	_, ok := g.operators[subject]
	return ok
}

// Middleware enforces the operator-only boundary. It reads the claim-bound
// subject the bearer middleware attached (NEVER a body field). An empty subject
// is 401 (anonymous); an authenticated non-operator subject is 403; an
// allowlisted operator passes. An empty allowlist therefore rejects everyone
// (fail-closed).
func (g *OperatorGate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject := auth.UserIDFromContext(r.Context())
		if subject == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "operator authentication required")
			return
		}
		if !g.IsOperator(subject) {
			slog.Warn("operator gate: non-operator subject rejected", "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "FORBIDDEN", "operator privilege required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ValidateOperatorGate is the G028 fail-loud startup guard (R1). When the
// credential-mutating admin surface is reachable (≥1 db-mode connection
// declared) AND the operator allowlist is empty AND the deployment is
// production, startup MUST abort — there is no open-by-default operator surface
// and no NO-DEFAULTS fallback. In dev/test the surface is allowed to run
// fail-closed (the Middleware locks everyone out) with a loud warning, mirroring
// the repo's existing empty-token auth precedent (MIT-040-S-004).
func ValidateOperatorGate(operatorUserIDs []string, surfaceReachable bool, environment string) error {
	if !surfaceReachable {
		return nil
	}
	configured := false
	for _, id := range operatorUserIDs {
		if strings.TrimSpace(id) != "" {
			configured = true
			break
		}
	}
	if configured {
		return nil
	}
	if environment == "production" {
		return fmt.Errorf(
			"operator gate: infrastructure.operator_user_ids is empty while the credential-mutating " +
				"/v1/admin/model-connections* surface is reachable (db-mode connections declared) — refusing to " +
				"start an open-by-default operator surface in production (G028, NO-DEFAULTS); populate " +
				"infrastructure.operator_user_ids in the deploy overlay")
	}
	slog.Warn("operator gate: infrastructure.operator_user_ids is empty; the /v1/admin/model-connections* surface "+
		"will run FAIL-CLOSED (every request 401/403) until an operator id is configured", "environment", environment)
	return nil
}
