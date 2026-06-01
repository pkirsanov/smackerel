// Package provenance owns the spec 061 Principle 8 ("Trust Through
// Transparency") hard constraint at the capability layer. Every
// scenario whose sibling-manifest metadata sets requires_provenance=true
// MUST attach at least one contracts.Source to its response; otherwise
// the response is rewritten to a canonical refusal + capture-route so
// the user never sees a synthesized-without-sources body.
//
// The rewrite is intentionally lossless from the user's perspective:
// the original Invocation/Routing references are preserved so traces
// remain queryable. Only the user-visible fields (Body, Status,
// Sources, CaptureRoute) are normalized.
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
//     resp.Body has any non-empty content → rewrite to the canonical
//     refusal (Body=CanonicalRefusalBody, Status=StatusSavedAsIdea,
//     CaptureRoute=true) and increment the violations counter
//     labeled by (scenarioLabel, cause).
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

	resp.Body = CanonicalRefusalBody
	resp.Status = contracts.StatusSavedAsIdea
	resp.CaptureRoute = true
	// Drop any sources that included an unrecognised Kind — the
	// rewrite path MUST NOT surface partially-invalid provenance.
	resp.Sources = nil
	// ErrorCause is intentionally not set — the response is a soft
	// refusal, not an unavailability error.
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

// EnforceRefusal unconditionally rewrites resp to the canonical
// refusal body for the given RefusalCause, regardless of the
// response's current Sources. Used by spec 064's open-knowledge
// agent when a known termination condition (budget exhausted, tool
// unavailable, fabricated-source rejection, internal-only
// restriction, ambiguous query) requires a cause-specific refusal
// body instead of the default "no sourced answer" text.
//
// The function also increments the ViolationsCounter labelled by
// (scenarioLabel, refusalCauseToProvenanceCause(refusalCause)) so
// dashboards can attribute open-knowledge refusals alongside the
// existing artifact-grounded provenance violations. The mapping is
// intentionally narrow: every new cause maps to a single existing
// ProvenanceCause bucket so the metric's label cardinality stays
// bounded.
func EnforceRefusal(scenarioLabel string, refusalCause contracts.RefusalCause, resp contracts.AssistantResponse) contracts.AssistantResponse {
	if scenarioLabel == "" {
		scenarioLabel = "unknown"
	}
	provCause := refusalCauseToProvenanceCause(refusalCause)
	ViolationsCounter.WithLabelValues(scenarioLabel, string(provCause)).Inc()

	resp.Body = contracts.CanonicalRefusalBodyFor(refusalCause)
	resp.Status = contracts.StatusSavedAsIdea
	resp.CaptureRoute = true
	resp.Sources = nil
	return resp
}

// refusalCauseToProvenanceCause maps a RefusalCause to the existing
// ProvenanceCause vocabulary used by the violations counter. The
// mapping is deliberate and stable per PKT-061-A so dashboards
// built against ProvenanceCause continue to work without a schema
// migration.
func refusalCauseToProvenanceCause(c contracts.RefusalCause) contracts.ProvenanceCause {
	switch c {
	case contracts.RefusalFabricatedSourceBlocked:
		return contracts.ProvenanceCauseFabricatedSource
	case contracts.RefusalToolUnavailable:
		return contracts.ProvenanceCauseLookupError
	case contracts.RefusalBudgetExhausted,
		contracts.RefusalInternalOnlyRestricted,
		contracts.RefusalAmbiguousNotClarified:
		return contracts.ProvenanceCauseMissingArtifact
	default:
		return contracts.ProvenanceCauseFabricatedSource
	}
}
