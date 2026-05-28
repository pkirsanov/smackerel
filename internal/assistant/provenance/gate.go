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
const CanonicalRefusalBody = "I don't have a sourced answer for that."

// ViolationsCounter records every time the gate rewrote a response
// to the canonical refusal. Labelled by scenario id so dashboards can
// spot which skill is leaking unsourced synthesis. Exposed as a
// package-level vector so tests can sample it deterministically.
var ViolationsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_provenance_violations_total",
		Help: "Number of capability-layer responses rewritten to the canonical refusal because a requires-provenance scenario returned a body with empty Sources (spec 061 Principle 8 hard constraint).",
	},
	[]string{"scenario"},
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
//     CaptureRoute=true) and increment the violations counter.
//   - When requiresProvenance is true AND len(resp.Sources) == 0 AND
//     resp.Body is empty → return resp unchanged. An empty body with
//     no sources is itself a refusal; the caller (the facade) is
//     responsible for setting Status/CaptureRoute. The gate does not
//     double-count empty-empty as a violation.
//
// scenarioLabel is the metric label value used when a rewrite fires.
// Pass the resolved scenario id; if unknown, pass "unknown" so the
// label cardinality stays bounded.
func Enforce(requiresProvenance bool, scenarioLabel string, resp contracts.AssistantResponse) contracts.AssistantResponse {
	if !requiresProvenance {
		return resp
	}
	if len(resp.Sources) > 0 {
		return resp
	}
	if resp.Body == "" {
		return resp
	}
	if scenarioLabel == "" {
		scenarioLabel = "unknown"
	}
	ViolationsCounter.WithLabelValues(scenarioLabel).Inc()

	resp.Body = CanonicalRefusalBody
	resp.Status = contracts.StatusSavedAsIdea
	resp.CaptureRoute = true
	// Sources stays nil/empty; ErrorCause is intentionally not set —
	// the response is a soft refusal, not an unavailability error.
	return resp
}
