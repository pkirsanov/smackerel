package intelligence

import (
	"fmt"
	"time"
)

// SynthesisFreshnessPolicy is the single cadence-to-budget mapping consumed by
// synthesis readers. Its fields are private so production code can only create
// a usable policy through the validating constructor.
type SynthesisFreshnessPolicy struct {
	daily  time.Duration
	weekly time.Duration
}

// RequiredSynthesisCadenceCount is the closed number of synthesis cadences
// that must be green before aggregate readiness can be green.
const RequiredSynthesisCadenceCount = 2

// NewSynthesisFreshnessPolicy constructs a fail-loud cadence freshness policy.
func NewSynthesisFreshnessPolicy(
	daily time.Duration,
	weekly time.Duration,
) (SynthesisFreshnessPolicy, error) {
	if daily <= 0 {
		return SynthesisFreshnessPolicy{}, fmt.Errorf("daily synthesis freshness must be positive, got %s", daily)
	}
	if weekly <= 0 {
		return SynthesisFreshnessPolicy{}, fmt.Errorf("weekly synthesis freshness must be positive, got %s", weekly)
	}
	return SynthesisFreshnessPolicy{daily: daily, weekly: weekly}, nil
}

// BudgetFor returns the configured budget for a supported synthesis cadence.
func (p SynthesisFreshnessPolicy) BudgetFor(cadence SynthesisCadence) (time.Duration, error) {
	var budget time.Duration
	switch cadence {
	case CadenceDaily:
		budget = p.daily
	case CadenceWeekly:
		budget = p.weekly
	default:
		return 0, fmt.Errorf("unknown synthesis freshness cadence %q", cadence)
	}
	if budget <= 0 {
		return 0, fmt.Errorf("synthesis freshness budget for cadence %q must be positive", cadence)
	}
	return budget, nil
}

// RequiredCadences returns a fresh, caller-owned copy of the closed readiness
// set. Keeping the set on the freshness policy prevents health consumers from
// choosing one cadence while still requiring each cadence's own BudgetFor value.
func (SynthesisFreshnessPolicy) RequiredCadences() [RequiredSynthesisCadenceCount]SynthesisCadence {
	return [RequiredSynthesisCadenceCount]SynthesisCadence{CadenceDaily, CadenceWeekly}
}
