// Package config — Spec 021 Scope 4: unified surfacing controller SST.
//
// SurfacingConfig governs the cross-channel surfacing budget /
// dedupe / suppression / urgent-escalation contract that the
// internal/intelligence/surfacing.Controller enforces. Every field
// originates in `surfacing.*` of config/smackerel.yaml and flows
// through scripts/commands/config.sh into the generated env file as
// SURFACING_* variables. There are no in-source defaults
// (smackerel-no-defaults policy); Load() returns a non-nil error
// naming every missing field.
package config

import (
	"fmt"
	"strings"
)

// SurfacingConfig is the SST surface for spec 021 Scope 4.
type SurfacingConfig struct {
	// DailyNudgeBudget is the per-user cross-channel ceiling on nudges
	// per day. Sourced from SURFACING_DAILY_NUDGE_BUDGET. MUST be > 0.
	DailyNudgeBudget int

	// SuppressionWindowHours is how long after an ack/dismiss a
	// follow-up nudge for the same content_key is suppressed. Sourced
	// from SURFACING_SUPPRESSION_WINDOW_HOURS. MUST be > 0.
	SuppressionWindowHours int

	// DedupeWindowHours is how long the cross-channel dedupe index
	// remembers a delivered content_key. Sourced from
	// SURFACING_DEDUPE_WINDOW_HOURS. MUST be > 0.
	DedupeWindowHours int

	// UrgentEscalationEnabled gates whether priority-1 + time_critical
	// candidates may bypass an exhausted budget. Sourced from
	// SURFACING_URGENT_ESCALATION_ENABLED.
	UrgentEscalationEnabled bool
}

// Validate returns nil when every required field is populated. Failure
// mode: a joined error naming every missing or zero-valued field.
func (c *SurfacingConfig) Validate() error {
	var missing []string
	if c.DailyNudgeBudget <= 0 {
		missing = append(missing, "SURFACING_DAILY_NUDGE_BUDGET")
	}
	if c.SuppressionWindowHours <= 0 {
		missing = append(missing, "SURFACING_SUPPRESSION_WINDOW_HOURS")
	}
	if c.DedupeWindowHours <= 0 {
		missing = append(missing, "SURFACING_DEDUPE_WINDOW_HOURS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid required surfacing configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

// loadSurfacingConfig reads the SURFACING_* env vars, validates every
// required field with fail-loud semantics, and returns the populated
// config. Errors are joined into one report.
func loadSurfacingConfig() (SurfacingConfig, error) {
	var cfg SurfacingConfig
	var errs []string

	cfg.DailyNudgeBudget, errs = parsePositiveInt("SURFACING_DAILY_NUDGE_BUDGET", errs)
	cfg.SuppressionWindowHours, errs = parsePositiveInt("SURFACING_SUPPRESSION_WINDOW_HOURS", errs)
	cfg.DedupeWindowHours, errs = parsePositiveInt("SURFACING_DEDUPE_WINDOW_HOURS", errs)
	cfg.UrgentEscalationEnabled, errs = requiredBool("SURFACING_URGENT_ESCALATION_ENABLED", errs)

	if len(errs) > 0 {
		return SurfacingConfig{}, fmt.Errorf("missing or invalid required surfacing configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return SurfacingConfig{}, err
	}
	return cfg, nil
}
