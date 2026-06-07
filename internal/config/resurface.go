// Package config — Spec 021 BUG-021-007: resurfacing OPERATIONAL SST.
//
// The "is this dormant artifact worth resurfacing?" JUDGMENT is LLM-driven (the
// resurface_evaluate scenario), NOT a hardcoded dormancy/relevance threshold.
// These keys carry only OPERATIONAL bounds for the dormancy strategy — a
// candidate-retrieval dormancy floor, a per-run throughput cap, and a
// decision-confidence safety gate. No in-source defaults (Gate G028,
// smackerel-no-defaults): every env var MUST be present at load time; deep
// validation rejects empty / invalid values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ResurfaceConfig is the SST surface for the operational bounds of the
// LLM-driven resurfacing dormancy strategy:
//
//   - intelligence.resurface.min_dormancy_days
//   - intelligence.resurface.max_candidates
//   - intelligence.resurface.confidence_floor
type ResurfaceConfig struct {
	// MinDormancyDays is the candidate-retrieval floor: exclude artifacts
	// accessed more recently than this so the LLM judges genuinely dormant
	// items. Operational. MUST be > 0.
	MinDormancyDays int
	// MaxCandidates is the per-run cap on dormant artifacts evaluated with the
	// LLM. MUST be > 0.
	MaxCandidates int
	// ConfidenceFloor is the decision-confidence safety gate; resurfacing below
	// it is withheld. MUST be in [0,1].
	ConfidenceFloor float64
}

// LoadResurfaceConfig reads every INTELLIGENCE_RESURFACE_* env var and returns
// a populated config plus Validate() result. Missing env vars are a fail-loud
// error.
func LoadResurfaceConfig() (ResurfaceConfig, error) {
	var cfg ResurfaceConfig
	var errs []string

	if v, ok := os.LookupEnv("INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS"); !ok {
		errs = append(errs, "INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS (must be an integer, got %q)", v))
	} else {
		cfg.MinDormancyDays = n
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_RESURFACE_MAX_CANDIDATES"); !ok {
		errs = append(errs, "INTELLIGENCE_RESURFACE_MAX_CANDIDATES (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RESURFACE_MAX_CANDIDATES (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RESURFACE_MAX_CANDIDATES (must be an integer, got %q)", v))
	} else {
		cfg.MaxCandidates = n
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR"); !ok {
		errs = append(errs, "INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR (empty)")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR (must be a float, got %q)", v))
	} else {
		cfg.ConfidenceFloor = f
	}

	if len(errs) > 0 {
		return ResurfaceConfig{}, fmt.Errorf("missing or invalid required intelligence.resurface configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return ResurfaceConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *ResurfaceConfig) Validate() error {
	var errs []string
	if c.MinDormancyDays <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.resurface.min_dormancy_days (must be > 0, got %d)", c.MinDormancyDays))
	}
	if c.MaxCandidates <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.resurface.max_candidates (must be > 0, got %d)", c.MaxCandidates))
	}
	if c.ConfidenceFloor < 0 || c.ConfidenceFloor > 1 {
		errs = append(errs, fmt.Sprintf("intelligence.resurface.confidence_floor (must be in [0,1], got %g)", c.ConfidenceFloor))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid intelligence.resurface configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
