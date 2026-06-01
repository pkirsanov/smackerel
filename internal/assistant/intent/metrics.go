// Spec 068 SCOPE-1 — Intent compiler metrics.
//
// design.md §"Observability And Failure Handling" enumerates the
// metric series. Scope 1a ships the foundation set; the route-
// selection and write-gate metrics are owned by Scopes 2-4.

package intent

import "github.com/prometheus/client_golang/prometheus"

// CompilerRequestsTotal counts compiler results by outcome and action
// class. action_class is empty when the compiler did not produce a
// valid intent (schema_invalid, provider_error, operational_command_bypass).
var CompilerRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_intent_compiler_requests_total",
		Help: "Intent compiler result count by outcome and action_class (spec 068 design §Observability).",
	},
	[]string{"outcome", "action_class"},
)

// CompilerErrorTotal counts compiler failure causes. cause ∈
// {"schema_invalid","json_invalid","provider_error","config_error"}.
// SCN-068-A06 contracts the schema_invalid increment.
var CompilerErrorTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_intent_compiler_error_total",
		Help: "Intent compiler failure count by cause (spec 068 SCN-068-A06).",
	},
	[]string{"cause"},
)

// BypassTotal counts operational-command bypass turns by command.
// SCN-068-A07 contracts the trace-label-stamping behaviour and this
// counter proves the carve-out stayed tiny.
var BypassTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_intent_bypass_total",
		Help: "Operational-command bypass count by command (spec 068 SCN-068-A07).",
	},
	[]string{"command"},
)

func init() {
	prometheus.MustRegister(CompilerRequestsTotal, CompilerErrorTotal, BypassTotal)
}
