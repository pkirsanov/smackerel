// Package config — Spec 021 BUG-021-010: hospitality-alert OPERATIONAL SST.
//
// The "is this guest/property a concern worth flagging?" JUDGMENT is LLM-driven
// (the hospitality_concern_evaluate scenario), NOT a hardcoded sentiment/rating/
// issue-count threshold. These keys carry only OPERATIONAL candidate-retrieval
// caps for the daily digest. No in-source defaults (Gate G028,
// smackerel-no-defaults): every env var MUST be present at load time; deep
// validation rejects empty / invalid values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// HospitalityConfig is the SST surface for the operational candidate caps of
// the LLM-driven hospitality alerts:
//
//   - digest.hospitality.guest_candidate_limit
//   - digest.hospitality.property_candidate_limit
type HospitalityConfig struct {
	// GuestCandidateLimit caps how many guest candidates are gathered and sent
	// to the LLM per digest. Operational throughput bound. MUST be > 0.
	GuestCandidateLimit int
	// PropertyCandidateLimit caps how many property candidates are gathered and
	// sent to the LLM per digest. Operational. MUST be > 0.
	PropertyCandidateLimit int
}

// LoadHospitalityConfig reads every DIGEST_HOSPITALITY_* env var and returns a
// populated config plus Validate() result. Missing env vars are a fail-loud
// error.
func LoadHospitalityConfig() (HospitalityConfig, error) {
	var cfg HospitalityConfig
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

	readInt("DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT", &cfg.GuestCandidateLimit)
	readInt("DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT", &cfg.PropertyCandidateLimit)

	if len(errs) > 0 {
		return HospitalityConfig{}, fmt.Errorf("missing or invalid required digest.hospitality configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return HospitalityConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *HospitalityConfig) Validate() error {
	var errs []string
	if c.GuestCandidateLimit <= 0 {
		errs = append(errs, fmt.Sprintf("digest.hospitality.guest_candidate_limit (must be > 0, got %d)", c.GuestCandidateLimit))
	}
	if c.PropertyCandidateLimit <= 0 {
		errs = append(errs, fmt.Sprintf("digest.hospitality.property_candidate_limit (must be > 0, got %d)", c.PropertyCandidateLimit))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid digest.hospitality configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
