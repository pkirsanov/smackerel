// Package provenance owns the spec 061 Principle 8 ("Trust Through
// Transparency") hard constraint at the capability layer. Every
// scenario whose sibling-manifest metadata sets requires_provenance=true
// MUST attach at least one contracts.Source to its response; otherwise
// the response is rewritten to an honest refusal (StatusUnavailable +
// ErrNoGroundedAnswer) so the user never sees a synthesized-without-
// sources body — and never the band-low "saved as an idea" capture
// acknowledgement (BUG-061-009: a matched, executed request that cannot
// be grounded is a high-band refusal, not a capture).
//
// The rewrite is intentionally lossless from the user's perspective:
// the original Invocation/Routing references are preserved so traces
// remain queryable. Only the user-visible fields (Body, Status,
// ErrorCause, Sources, CaptureRoute) are normalized.
package provenance

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// CanonicalRefusalBody is the user-facing refusal text when a
// requires-provenance scenario produced a non-empty body without
// any sources. Stable per design §4.3 so UI assertions are easy.
// Preserved as the package-level constant for backward
// compatibility with existing call sites and tests; new call sites
// that want a cause-specific refusal body should call
// contracts.CanonicalRefusalBodyFor (PKT-061-A from spec 064).
const CanonicalRefusalBody = "I don't have a sourced answer for that."

// acceptedSourceKinds is the closed set of Source.Kind values the
// gate accepts as "sourced". Built once from contracts.AllSourceKinds
// at init time so the gate and the contracts taxonomy can never
// drift (adversarial: a new Kind added in contracts without a
// matching test here is still accepted; a Kind removed from the
// contracts list is automatically rejected here).
var acceptedSourceKinds map[contracts.SourceKind]struct{}

func init() {
	acceptedSourceKinds = make(map[contracts.SourceKind]struct{}, len(contracts.AllSourceKinds))
	for _, k := range contracts.AllSourceKinds {
		acceptedSourceKinds[k] = struct{}{}
	}
}

// ViolationsCounter records every time the gate rewrote a response
// to the canonical refusal. Labelled by scenario_id AND cause so
// dashboards can distinguish graph-drift (missing_artifact,
// lookup_error) from LLM fabrication (fabricated_source) and SST
// misconfiguration (dropped_for_quota). Spec 061 SCOPE-09. Exposed
// as a package-level vector so tests can sample it deterministically.
var ViolationsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_provenance_violations_total",
		Help: "Number of capability-layer responses rewritten to the canonical refusal because a requires-provenance scenario returned a body with empty Sources (spec 061 Principle 8 hard constraint). Labeled by scenario_id (originating scenario) and cause (missing_artifact / lookup_error / fabricated_source / dropped_for_quota) so dashboards can attribute each rewrite to the upstream condition.",
	},
	[]string{"scenario_id", "cause"},
)

func init() {
	prometheus.MustRegister(ViolationsCounter)
}

// Enforce applies the requires-provenance check.
//
// Behavior:
//   - When requiresProvenance is false → return resp unchanged.
//   - When requiresProvenance is true AND len(resp.Sources) > 0 →
//     return resp unchanged (passthrough).
//   - When requiresProvenance is true AND len(resp.Sources) == 0 AND
//     resp.Body has any non-empty content → rewrite to the honest
//     refusal (Body=CanonicalRefusalBody, Status=StatusUnavailable,
//     ErrorCause=ErrNoGroundedAnswer, CaptureRoute=false) and increment
//     the violations counter labeled by (scenarioLabel, cause).
//   - When requiresProvenance is true AND len(resp.Sources) == 0 AND
//     resp.Body is empty → return resp unchanged. An empty body with
//     no sources is itself a refusal; the caller (the facade) is
//     responsible for setting Status/CaptureRoute. The gate does not
//     double-count empty-empty as a violation.
//
// scenarioLabel is the metric label value used when a rewrite fires.
// Pass the resolved scenario id; if unknown, pass "unknown" so the
// label cardinality stays bounded.
//
// cause is the attribution hint from the upstream source-assembler.
// Empty cause defaults to ProvenanceCauseFabricatedSource (a body
// with no sources is, by definition, fabricated if no upstream
// condition explained the drop).
func Enforce(requiresProvenance bool, scenarioLabel string, cause contracts.ProvenanceCause, resp contracts.AssistantResponse) contracts.AssistantResponse {
	if !requiresProvenance {
		return resp
	}
	if len(resp.Sources) > 0 && allSourceKindsAccepted(resp.Sources) {
		return resp
	}
	if resp.Body == "" {
		return resp
	}
	if scenarioLabel == "" {
		scenarioLabel = "unknown"
	}
	if cause == "" {
		cause = contracts.ProvenanceCauseFabricatedSource
	}
	ViolationsCounter.WithLabelValues(scenarioLabel, string(cause)).Inc()

	// BUG-061-009 — refuse into an HONEST high-band shape, not a band-low
	// capture. A requires_provenance scenario that produced a body with no
	// valid sources is a matched, executed request the system could not
	// ground; the user must see the honest refusal, never the "saved as an
	// idea" capture acknowledgement (that is band-low-only). The canonical
	// refusal body is preserved; Status becomes StatusUnavailable with the
	// ErrNoGroundedAnswer cause so the transport renders it honestly and
	// refusal-vs-answer stays structurally distinguishable (no user-visible
	// capture string).
	resp.Body = CanonicalRefusalBody
	resp.Status = contracts.StatusUnavailable
	resp.ErrorCause = contracts.ErrNoGroundedAnswer
	resp.CaptureRoute = false
	// Drop any sources that included an unrecognised Kind — the
	// rewrite path MUST NOT surface partially-invalid provenance.
	resp.Sources = nil
	return resp
}

// allSourceKindsAccepted reports whether every Source.Kind in srcs
// is in the gate's accepted taxonomy. An empty slice returns true
// (the caller separately enforces len > 0).
func allSourceKindsAccepted(srcs []contracts.Source) bool {
	for _, s := range srcs {
		if _, ok := acceptedSourceKinds[s.Kind]; !ok {
			return false
		}
	}
	return true
}
