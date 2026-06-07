// Spec 021 BUG-021-010 — tests for the hospitality-alert OPERATIONAL SST loader.
package config

import (
	"strings"
	"testing"
)

func setValidHospitalityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT", "50")
	t.Setenv("DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT", "50")
}

func TestLoadHospitalityConfig_Populates(t *testing.T) {
	setValidHospitalityEnv(t)
	cfg, err := LoadHospitalityConfig()
	if err != nil {
		t.Fatalf("LoadHospitalityConfig: %v", err)
	}
	if cfg.GuestCandidateLimit != 50 {
		t.Errorf("GuestCandidateLimit = %d, want 50", cfg.GuestCandidateLimit)
	}
	if cfg.PropertyCandidateLimit != 50 {
		t.Errorf("PropertyCandidateLimit = %d, want 50", cfg.PropertyCandidateLimit)
	}
}

func TestLoadHospitalityConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT",
		"DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidHospitalityEnv(t)
			t.Setenv(missing, "")
			_, err := LoadHospitalityConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadHospitalityConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"guest_zero", "DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT", "0"},
		{"guest_negative", "DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT", "-5"},
		{"property_zero", "DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT", "0"},
		{"property_negative", "DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidHospitalityEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadHospitalityConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
