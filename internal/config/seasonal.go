// Package config — Spec 021 BUG-021-009: seasonal-detection OPERATIONAL SST.
//
// The "is this year-over-year volume change a meaningful seasonal pattern?"
// JUDGMENT is LLM-driven (the seasonal.analyze ML scenario), NOT a hardcoded
// volume-ratio threshold. These keys carry only OPERATIONAL bounds for seasonal
// detection — a data-sufficiency floor, a topic-candidate floor + cap, and an
// observation cap. No in-source defaults (Gate G028, smackerel-no-defaults):
// every env var MUST be present at load time; deep validation rejects empty /
// invalid values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SeasonalConfig is the SST surface for the operational bounds of LLM-driven
// seasonal pattern detection:
//
//   - intelligence.seasonal.min_data_days
//   - intelligence.seasonal.topic_min_captures
//   - intelligence.seasonal.topic_candidate_limit
//   - intelligence.seasonal.max_observations
type SeasonalConfig struct {
	// MinDataDays is the data-sufficiency floor: seasonal detection only runs
	// once at least this many days of data exist (R-508 needs 6+ months).
	// Operational. MUST be > 0.
	MinDataDays int
	// TopicMinCaptures is the minimum same-month captures for a topic to be a
	// candidate sent to the LLM. Operational. MUST be > 0.
	TopicMinCaptures int
	// TopicCandidateLimit caps how many topic candidates are sent to the LLM.
	// Operational. MUST be > 0.
	TopicCandidateLimit int
	// MaxObservations caps how many seasonal observations are returned.
	// Operational. MUST be > 0.
	MaxObservations int
}

// LoadSeasonalConfig reads every INTELLIGENCE_SEASONAL_* env var and returns a
// populated config plus Validate() result. Missing env vars are a fail-loud
// error.
func LoadSeasonalConfig() (SeasonalConfig, error) {
	var cfg SeasonalConfig
	var errs []string

	readInt := func(key string, dst *int) {
		if v, ok := os.LookupEnv(key); !ok {
			errs = append(errs, key+" (env var not set)")
		} else if v == "" {
			errs = append(errs, key+" (empty)")
		} else if n, err := strconv.Atoi(v); err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
		} else {
			*dst = n
		}
	}

	readInt("INTELLIGENCE_SEASONAL_MIN_DATA_DAYS", &cfg.MinDataDays)
	readInt("INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES", &cfg.TopicMinCaptures)
	readInt("INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT", &cfg.TopicCandidateLimit)
	readInt("INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS", &cfg.MaxObservations)

	if len(errs) > 0 {
		return SeasonalConfig{}, fmt.Errorf("missing or invalid required intelligence.seasonal configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return SeasonalConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *SeasonalConfig) Validate() error {
	var errs []string
	if c.MinDataDays <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.seasonal.min_data_days (must be > 0, got %d)", c.MinDataDays))
	}
	if c.TopicMinCaptures <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.seasonal.topic_min_captures (must be > 0, got %d)", c.TopicMinCaptures))
	}
	if c.TopicCandidateLimit <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.seasonal.topic_candidate_limit (must be > 0, got %d)", c.TopicCandidateLimit))
	}
	if c.MaxObservations <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.seasonal.max_observations (must be > 0, got %d)", c.MaxObservations))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid intelligence.seasonal configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
