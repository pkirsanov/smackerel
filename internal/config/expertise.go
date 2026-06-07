// Package config — Spec 021 BUG-021-008: expertise-mapping OPERATIONAL SST.
//
// The "how expert is the user in this topic, and is it growing?" JUDGMENT is
// LLM-driven (the expertise_classify scenario), NOT a hardcoded weighted score
// or tier/velocity threshold. These keys carry only OPERATIONAL bounds for
// expertise-map generation — a per-request topic cap, a data-sufficiency floor,
// and the blind-spot gap-detection bounds. No in-source defaults (Gate G028,
// smackerel-no-defaults): every env var MUST be present at load time; deep
// validation rejects empty / invalid values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ExpertiseConfig is the SST surface for the operational bounds of the
// LLM-driven expertise map:
//
//   - intelligence.expertise.max_topics
//   - intelligence.expertise.maturity_days
//   - intelligence.expertise.blind_spot_min_mentions
//   - intelligence.expertise.blind_spot_max_captures
//   - intelligence.expertise.blind_spot_limit
type ExpertiseConfig struct {
	// MaxTopics is the per-request cap on topics retrieved and classified.
	// Operational throughput bound. MUST be > 0.
	MaxTopics int
	// MaturityDays is the data-sufficiency floor: the map is flagged "mature"
	// only once at least this many days of data exist. Operational. MUST be > 0.
	MaturityDays int
	// BlindSpotMinMentions is the minimum mention count for a topic to be
	// considered an under-captured blind spot. Operational. MUST be > 0.
	BlindSpotMinMentions int
	// BlindSpotMaxCaptures is the capture-count ceiling below which a mentioned
	// topic counts as under-captured. Operational. MUST be > 0.
	BlindSpotMaxCaptures int
	// BlindSpotLimit caps how many blind spots are returned. Operational.
	// MUST be > 0.
	BlindSpotLimit int
}

// LoadExpertiseConfig reads every INTELLIGENCE_EXPERTISE_* env var and returns a
// populated config plus Validate() result. Missing env vars are a fail-loud
// error.
func LoadExpertiseConfig() (ExpertiseConfig, error) {
	var cfg ExpertiseConfig
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

	readInt("INTELLIGENCE_EXPERTISE_MAX_TOPICS", &cfg.MaxTopics)
	readInt("INTELLIGENCE_EXPERTISE_MATURITY_DAYS", &cfg.MaturityDays)
	readInt("INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS", &cfg.BlindSpotMinMentions)
	readInt("INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES", &cfg.BlindSpotMaxCaptures)
	readInt("INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT", &cfg.BlindSpotLimit)

	if len(errs) > 0 {
		return ExpertiseConfig{}, fmt.Errorf("missing or invalid required intelligence.expertise configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return ExpertiseConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *ExpertiseConfig) Validate() error {
	var errs []string
	if c.MaxTopics <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.expertise.max_topics (must be > 0, got %d)", c.MaxTopics))
	}
	if c.MaturityDays <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.expertise.maturity_days (must be > 0, got %d)", c.MaturityDays))
	}
	if c.BlindSpotMinMentions <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.expertise.blind_spot_min_mentions (must be > 0, got %d)", c.BlindSpotMinMentions))
	}
	if c.BlindSpotMaxCaptures <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.expertise.blind_spot_max_captures (must be > 0, got %d)", c.BlindSpotMaxCaptures))
	}
	if c.BlindSpotLimit <= 0 {
		errs = append(errs, fmt.Sprintf("intelligence.expertise.blind_spot_limit (must be > 0, got %d)", c.BlindSpotLimit))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid intelligence.expertise configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
