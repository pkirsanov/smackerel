// Package config — Spec 021 BUG-021-006: alert-timing OPERATIONAL SST.
//
// The "should I alert about this upcoming event NOW?" JUDGMENT is LLM-driven
// (the alert_timing_evaluate scenario), NOT a hardcoded N-day window. These
// keys carry only OPERATIONAL bounds for the bill / trip-prep / return-window
// producers — a candidate-retrieval lookahead horizon, a per-run throughput
// cap, and a decision-confidence safety gate. No in-source defaults (Gate
// G028, smackerel-no-defaults): every env var MUST be present at load time;
// deep validation rejects empty / invalid values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AlertTimingConfig is the SST surface for the operational bounds of the
// LLM-driven alert-timing producers:
//
//   - intelligence.alert_timing.lookahead_days
//   - intelligence.alert_timing.max_candidates
//   - intelligence.alert_timing.confidence_floor
type AlertTimingConfig struct {
	// LookaheadDays is how far ahead to retrieve candidate events for the LLM
	// to judge. Operational candidate-retrieval horizon. MUST be > 0.
	LookaheadDays int
	// MaxCandidates is the per-run cap on candidates evaluated per producer.
	// MUST be > 0.
	MaxCandidates int
	// ConfidenceFloor is the decision-confidence safety gate; reminders below
	// it are withheld. MUST be in [0,1].
	ConfidenceFloor float64
}

// LoadAlertTimingConfig reads every INTELLIGENCE_ALERT_TIMING_* env var and
// returns a populated config plus Validate() result. Missing env vars are a
// fail-loud error.
func LoadAlertTimingConfig() (AlertTimingConfig, error) {
	var cfg AlertTimingConfig
	var errs []string

	if v, ok := os.LookupEnv("INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS"); !ok {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS (must be an integer, got %q)", v))
	} else {
		cfg.LookaheadDays = n
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES"); !ok {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES (must be an integer, got %q)", v))
	} else {
		cfg.MaxCandidates = n
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR"); !ok {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR (empty)")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR (must be a float, got %q)", v))
	} else {
		cfg.ConfidenceFloor = f
	}

	if len(errs) > 0 {
		return AlertTimingConfig{}, fmt.Errorf("missing or invalid required intelligence.alert_timing configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return AlertTimingConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *AlertTimingConfig) Validate() error {
	var errs []string
	if c.LookaheadDays <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.alert_timing.lookahead_days (must be > 0, got %d)", c.LookaheadDays))
	}
	if c.MaxCandidates <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.alert_timing.max_candidates (must be > 0, got %d)", c.MaxCandidates))
	}
	if c.ConfidenceFloor < 0 || c.ConfidenceFloor > 1 {
		errs = append(errs, fmt.Sprintf("intelligence.alert_timing.confidence_floor (must be in [0,1], got %g)", c.ConfidenceFloor))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid intelligence.alert_timing configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
