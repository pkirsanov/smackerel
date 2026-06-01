// Spec 071 SCOPE-03 — Bypass guard read path over IntentTrace
// ancestor fields (SCN-071-A08).
//
// The spec 068 ReportRawRouteBypasses guard is a static-source
// scanner that flags Router.Route call sites without a nearby
// intent.Compiler reference. Spec 071 adds a runtime ancestor check
// that the policy guard uses when it observes a tool call through
// OpenTelemetry: it reads the surrounding IntentTrace ancestor
// fields (compiler_invoked, route_decision, tool_calls[].name) and
// reports any tool call that lacks a compiled-intent ancestor.
//
// This is intentionally a tiny pure-function surface. The spec 067
// guard wires it against the live OTel span tree; the spec 071
// integration test feeds it synthetic ancestors so a raw-route
// bypass is provably rejected without depending on the full OTel
// pipeline being available in the test stack.

package policyguard

import "fmt"

// MissingIntentTraceAncestor is the canonical phrase the runtime
// ancestor check uses when an observed tool call has no IntentTrace
// ancestor with compiler_invoked=true. Stable so guard-output tests
// can match it verbatim.
const MissingIntentTraceAncestor = "missing IntentTrace ancestor with compiler_invoked=true for observed tool call"

// MissingRouteDecisionAncestor is the canonical phrase used when an
// IntentTrace ancestor exists but does not name a route_decision.
const MissingRouteDecisionAncestor = "IntentTrace ancestor present but route_decision is empty for observed tool call"

// UnknownToolCallAncestor is the canonical phrase used when the
// observed tool call name is absent from the ancestor's tool_calls
// list.
const UnknownToolCallAncestor = "observed tool call %q absent from IntentTrace ancestor tool_calls"

// IntentTraceAncestor is the minimal projection of an IntentTrace
// row that the runtime bypass guard needs. The fields mirror the v1
// payload (compiler_invoked, route_decision, tool_calls[].name) so
// the spec 067 guard can populate it from a persisted trace row or
// from OTel span attributes without re-shaping data.
type IntentTraceAncestor struct {
	// Present is false when the observation has NO IntentTrace
	// ancestor at all — the classic raw-route bypass shape.
	Present bool
	// CompilerInvoked mirrors the v1 schema field of the same name.
	CompilerInvoked bool
	// RouteDecision mirrors the v1 schema field of the same name.
	RouteDecision string
	// ToolCallNames mirrors the v1 schema tool_calls[].name field.
	ToolCallNames []string
}

// ToolCallObservation is one tool-call observation drawn from an
// OTel span tree (or test fixture). Spec 067 wires this from the
// live span attribute family `assistant.tool.*`.
type ToolCallObservation struct {
	// SpanName is the span identifier used in the finding message.
	SpanName string
	// ToolName is the tool name extracted from the span.
	ToolName string
	// Ancestor is the projected IntentTrace ancestor for this span.
	Ancestor IntentTraceAncestor
}

// CheckIntentTraceAncestor returns one Finding per observation that
// fails the ancestor invariants. Zero findings means every observed
// tool call had a valid compiled-intent ancestor with a matching
// route_decision and tool name. The function is pure and has no I/O
// — it is safe to call from any code path including tests.
func CheckIntentTraceAncestor(observations []ToolCallObservation) []Finding {
	var findings []Finding
	for _, obs := range observations {
		if !obs.Ancestor.Present || !obs.Ancestor.CompilerInvoked {
			findings = append(findings, Finding{
				File:    obs.SpanName,
				Message: fmt.Sprintf("%s: %s (tool=%s)", obs.SpanName, MissingIntentTraceAncestor, obs.ToolName),
			})
			continue
		}
		if obs.Ancestor.RouteDecision == "" {
			findings = append(findings, Finding{
				File:    obs.SpanName,
				Message: fmt.Sprintf("%s: %s (tool=%s)", obs.SpanName, MissingRouteDecisionAncestor, obs.ToolName),
			})
			continue
		}
		if !containsString(obs.Ancestor.ToolCallNames, obs.ToolName) {
			findings = append(findings, Finding{
				File:    obs.SpanName,
				Message: fmt.Sprintf("%s: "+UnknownToolCallAncestor, obs.SpanName, obs.ToolName),
			})
		}
	}
	return findings
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
