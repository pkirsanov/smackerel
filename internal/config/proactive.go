// Package config — Spec 107 SCOPE-01: Proactive & Correlated Experience
// foundation SST.
//
// ProactiveConfig governs the process-local NudgeRef registry TTL that the
// internal/proactive foundation depends on. The field originates in
// `proactive.*` of config/smackerel.yaml and flows through
// scripts/commands/config.sh into the generated env file as PROACTIVE_* vars.
// There are no in-source defaults (smackerel-no-defaults policy); loading fails
// loud, naming the offending field, when it is missing, non-positive, or below
// its cross-field bound.
package config

import (
	"fmt"
	"strings"
)

// ProactiveConfig is the SST surface for spec 107 SCOPE-01.
type ProactiveConfig struct {
	// NudgeRefTTLHours is how long a minted NudgeRef resolves before a late tap
	// renders an honest expired/already-handled state. Sourced from
	// PROACTIVE_NUDGE_REF_TTL_HOURS. MUST be > 0 AND >=
	// max(SurfacingConfig.SuppressionWindowHours, SurfacingConfig.DedupeWindowHours)
	// so a ref outlives both windows (design.md OQ2 / SCOPE-01 SST decision).
	NudgeRefTTLHours int
}

// Validate returns nil when the TTL is populated and satisfies its cross-field
// bound against the surfacing suppression/dedupe windows. Failure mode: an error
// naming the field and the bound it violated.
func (c *ProactiveConfig) Validate(surfacing SurfacingConfig) error {
	if c.NudgeRefTTLHours <= 0 {
		return fmt.Errorf("missing or invalid required proactive configuration: PROACTIVE_NUDGE_REF_TTL_HOURS (must be a positive integer)")
	}
	bound := surfacing.SuppressionWindowHours
	if surfacing.DedupeWindowHours > bound {
		bound = surfacing.DedupeWindowHours
	}
	if c.NudgeRefTTLHours < bound {
		return fmt.Errorf(
			"missing or invalid required proactive configuration: PROACTIVE_NUDGE_REF_TTL_HOURS (%d) must be >= max(SURFACING_SUPPRESSION_WINDOW_HOURS=%d, SURFACING_DEDUPE_WINDOW_HOURS=%d) = %d",
			c.NudgeRefTTLHours, surfacing.SuppressionWindowHours, surfacing.DedupeWindowHours, bound,
		)
	}
	return nil
}

// loadProactiveConfig reads the PROACTIVE_* env vars, validates the required
// field with fail-loud semantics (including the cross-field bound against the
// already-loaded surfacing config), and returns the populated config.
func loadProactiveConfig(surfacing SurfacingConfig) (ProactiveConfig, error) {
	var cfg ProactiveConfig
	var errs []string

	cfg.NudgeRefTTLHours, errs = parsePositiveInt("PROACTIVE_NUDGE_REF_TTL_HOURS", errs)

	if len(errs) > 0 {
		return ProactiveConfig{}, fmt.Errorf("missing or invalid required proactive configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(surfacing); err != nil {
		return ProactiveConfig{}, err
	}
	return cfg, nil
}
