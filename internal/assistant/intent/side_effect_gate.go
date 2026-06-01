// Spec 068 SCOPE-3 — Side-effect gating contract.
//
// RequiresConfirmation classifies a compiled intent against the
// SCN-068-A09 gate. Any side_effect_class ∈ {write, external_write}
// MUST run through the existing confirmation gate before the
// executor mutates persistent or external state. SideEffectBlockedTotal
// counts every gate fire so dashboards can prove the carve-out
// stayed tiny.

package intent

import "github.com/prometheus/client_golang/prometheus"

// RequiresConfirmation returns true when the compiled intent's
// side_effect_class triggers the SCN-068-A09 confirmation gate.
// SideEffectNone, SideEffectRead, and SideEffectExternalRead pass
// through without confirmation.
func RequiresConfirmation(c CompiledIntent) bool {
	switch c.SideEffectClass {
	case SideEffectWrite, SideEffectExternalWrite:
		return true
	}
	return false
}

// SideEffectBlockedTotal counts ungated write/external_write turns
// blocked by the facade-level side-effect gate. cause ∈
// {"missing_confirmation"} for now; future causes (e.g. revoked
// confirmation, replay) will land alongside the wiring that emits
// them.
var SideEffectBlockedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_side_effect_blocked_total",
		Help: "Side-effect gate firings by side_effect_class and cause (spec 068 SCN-068-A09).",
	},
	[]string{"side_effect_class", "cause"},
)

func init() {
	prometheus.MustRegister(SideEffectBlockedTotal)
}
