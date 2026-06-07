// Package config — Spec 021 BUG-021-005: relationship-cooling OPERATIONAL SST.
//
// The "is this relationship cooling?" JUDGMENT is LLM-driven (the
// relationship_cooling_evaluate scenario), NOT a hardcoded threshold. These
// keys carry only OPERATIONAL bounds for the cooling job — a per-run
// throughput cap, a decision-confidence safety gate, and an anti-spam re-alert
// window. No in-source defaults (Gate G028, smackerel-no-defaults): every env
// var MUST be present at load time; deep validation rejects empty / invalid
// values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RelationshipCoolingConfig is the SST surface for the operational bounds of
// the LLM-driven relationship-cooling producer:
//
//   - intelligence.relationship_cooling.max_candidates
//   - intelligence.relationship_cooling.confidence_floor
//   - intelligence.relationship_cooling.dedup_window_days
type RelationshipCoolingConfig struct {
	// MaxCandidates is the per-run cap on how many of the most-dormant
	// contacts the cooling job evaluates with the LLM. MUST be > 0.
	MaxCandidates int
	// ConfidenceFloor is the decision-confidence safety gate; cooling nudges
	// below it are withheld. MUST be in [0,1].
	ConfidenceFloor float64
	// DedupWindowDays suppresses re-alerting a contact whose cooling nudge is
	// still pending/delivered within this many days. MUST be > 0.
	DedupWindowDays int
}

// LoadRelationshipCoolingConfig reads every
// INTELLIGENCE_RELATIONSHIP_COOLING_* env var and returns a populated config
// plus Validate() result. Missing env vars are a fail-loud error.
func LoadRelationshipCoolingConfig() (RelationshipCoolingConfig, error) {
	var cfg RelationshipCoolingConfig
	var errs []string

	if v, ok := os.LookupEnv("INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES"); !ok {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES (must be an integer, got %q)", v))
	} else {
		cfg.MaxCandidates = n
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR"); !ok {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR (empty)")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR (must be a float, got %q)", v))
	} else {
		cfg.ConfidenceFloor = f
	}

	if v, ok := os.LookupEnv("INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS"); !ok {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS (env var not set)")
	} else if v == "" {
		errs = append(errs, "INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS (empty)")
	} else if n, err := strconv.Atoi(v); err != nil {
		errs = append(errs, fmt.Sprintf("INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS (must be an integer, got %q)", v))
	} else {
		cfg.DedupWindowDays = n
	}

	if len(errs) > 0 {
		return RelationshipCoolingConfig{}, fmt.Errorf("missing or invalid required intelligence.relationship_cooling configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return RelationshipCoolingConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *RelationshipCoolingConfig) Validate() error {
	var errs []string
	if c.MaxCandidates <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.relationship_cooling.max_candidates (must be > 0, got %d)", c.MaxCandidates))
	}
	if c.ConfidenceFloor < 0 || c.ConfidenceFloor > 1 {
		errs = append(errs, fmt.Sprintf("intelligence.relationship_cooling.confidence_floor (must be in [0,1], got %g)", c.ConfidenceFloor))
	}
	if c.DedupWindowDays <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.relationship_cooling.dedup_window_days (must be > 0, got %d)", c.DedupWindowDays))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid intelligence.relationship_cooling configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
