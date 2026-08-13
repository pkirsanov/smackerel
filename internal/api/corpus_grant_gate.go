// Spec 108 Scope 02 — the OBSERVE half of the corpus-grant stage machine
// (design.md §4).
//
// CorpusGrantGate is the first production caller of
// `auth.GateGlobalCorpusRead`, which until now existed only as a policy
// function with a test referent. The gate evaluates that decision on every
// corpus route group and records the counterfactual — "this request WOULD be
// denied under ENFORCE" — without ever denying.
//
// MOUNTED IN BOTH STAGES. The gate is stage-independent by construction: it
// takes no stage parameter that can switch its decision, and `Observe` has no
// code path that writes a status or short-circuits `next`. That is what makes
// OBSERVE emit anything at all — a gate mounted only under ENFORCE would leave
// the observation window silent, and the operator would flip the flag against
// a counter that was structurally incapable of moving. The ENFORCE half
// (`auth.RequireScope(auth.GrantGlobalCorpusRead)`) and the mounting of both
// onto the sixteen corpus route groups in `router.go` are Scope 03.
package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Structured-log value for the `enforcement_mode` field (design.md §4). These
// are the lowercase log-field spellings; the uppercase OBSERVE / ENFORCE stage
// names in `cmd/core` are the operator-facing vocabulary for the same two
// stages.
const (
	corpusGrantModeObserve = "observe"
	corpusGrantModeEnforce = "enforce"
)

// CorpusGrantGate observes corpus-read authority. It holds the resolved stage
// for telemetry and logging only — the stage NEVER changes whether Observe
// admits a request, because Observe never denies in either stage.
type CorpusGrantGate struct {
	mode string
}

// NewCorpusGrantGate builds the gate from the stage resolved once at startup
// by `cmd/core` (fail-loud; absent, empty, or malformed config never reaches
// here because the process refuses to boot).
func NewCorpusGrantGate(enforce bool) *CorpusGrantGate {
	mode := corpusGrantModeObserve
	if enforce {
		mode = corpusGrantModeEnforce
	}
	return &CorpusGrantGate{mode: mode}
}

// Mode reports the stage this gate was constructed with, as the lowercase
// `enforcement_mode` log-field value.
func (g *CorpusGrantGate) Mode() string { return g.mode }

// Observe returns middleware for one corpus route group. It evaluates
// `auth.GateGlobalCorpusRead` and records the outcome, then ALWAYS calls the
// next handler. There is no denial branch — under OBSERVE that is the point,
// and under ENFORCE the denial is `auth.RequireScope`'s job (Scope 03), not
// this gate's.
//
// routeGroup is validated at CONSTRUCTION and panics when it is outside the
// closed sixteen-value set, mirroring the fail-loud wiring guards already used
// for the graph route manifest and `auth.RequireScope`. Validating here rather
// than per request is what makes an unbounded label structurally impossible: a
// route group is a wiring-time constant, so a raw request path can never
// become a label value (R-108-O3/O4).
func (g *CorpusGrantGate) Observe(routeGroup metrics.CorpusRouteGroup) func(http.Handler) http.Handler {
	if err := metrics.ValidateCorpusRouteGroup(routeGroup); err != nil {
		panic(fmt.Sprintf("api: CorpusGrantGate.Observe: %v", err))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g.record(r, routeGroup)
			next.ServeHTTP(w, r)
		})
	}
}

// record evaluates the corpus decision and emits telemetry. It is split out so
// the never-denies guarantee in Observe is a single unconditional
// `next.ServeHTTP` with nothing that can return early ahead of it.
func (g *CorpusGrantGate) record(r *http.Request, routeGroup metrics.CorpusRouteGroup) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok {
		// Wiring defect — bearerAuthMiddleware must run before this gate and
		// populate the session. Recording a would-be denial here would invent
		// a principal that does not exist, so nothing is emitted.
		slog.Error("api: CorpusGrantGate reached without session in context (middleware misconfigured)",
			"route_group", string(routeGroup),
			"enforcement_mode", g.mode,
		)
		return
	}

	// Mirror the documented `auth.RequireScope` source switch. Shared-token
	// and bootstrap sessions carry no scope claim and are BYPASSED by
	// RequireScope under ENFORCE, so counting them as would-be denials would
	// make this counter a false predictor of the ENFORCE outcome and inflate
	// the UC-108-001 grant list with principals that will never be denied.
	// They are not double-counted into AuthScopeCheckBypassed either — that
	// counter belongs to the RequireScope mount point.
	switch sess.Source {
	case auth.SessionSourceSharedToken, auth.SessionSourceBootstrap:
		return
	}

	sessionSource := string(sess.Source)
	if auth.GateGlobalCorpusRead(sess).Allowed {
		if err := metrics.RecordCorpusGrantAllowed(routeGroup, sess.UserID, sessionSource); err != nil {
			slog.Error("api: corpus grant allowed counter not emitted", "error", err)
		}
		return
	}

	if err := metrics.RecordCorpusGrantWouldDeny(routeGroup, sess.UserID, sessionSource); err != nil {
		slog.Error("api: corpus grant would-deny counter not emitted", "error", err)
	}
	// Answers WHO would have been denied, never WHAT they asked for: no query
	// text, no artifact id, no result count, no path (design.md §4).
	slog.Warn("auth: corpus_grant_would_deny",
		"event", "corpus_grant_would_deny",
		"route_group", string(routeGroup),
		"user_id", sess.UserID,
		"session_source", sessionSource,
		"required_grant", auth.GrantGlobalCorpusRead,
		"enforcement_mode", g.mode,
	)
}
