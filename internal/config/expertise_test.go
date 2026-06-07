// Spec 021 BUG-021-008 — tests for the expertise-mapping OPERATIONAL SST loader.
package config

import (
	"strings"
	"testing"
)

func setValidExpertiseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INTELLIGENCE_EXPERTISE_MAX_TOPICS", "100")
	t.Setenv("INTELLIGENCE_EXPERTISE_MATURITY_DAYS", "90")
	t.Setenv("INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS", "5")
	t.Setenv("INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES", "5")
	t.Setenv("INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT", "10")
}

func TestLoadExpertiseConfig_Populates(t *testing.T) {
	setValidExpertiseEnv(t)
	cfg, err := LoadExpertiseConfig()
	if err != nil {
		t.Fatalf("LoadExpertiseConfig: %v", err)
	}
	if cfg.MaxTopics != 100 {
		t.Errorf("MaxTopics = %d, want 100", cfg.MaxTopics)
	}
	if cfg.MaturityDays != 90 {
		t.Errorf("MaturityDays = %d, want 90", cfg.MaturityDays)
	}
	if cfg.BlindSpotMinMentions != 5 || cfg.BlindSpotMaxCaptures != 5 || cfg.BlindSpotLimit != 10 {
		t.Errorf("blind-spot bounds = %d/%d/%d, want 5/5/10", cfg.BlindSpotMinMentions, cfg.BlindSpotMaxCaptures, cfg.BlindSpotLimit)
	}
}

func TestLoadExpertiseConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"INTELLIGENCE_EXPERTISE_MAX_TOPICS",
		"INTELLIGENCE_EXPERTISE_MATURITY_DAYS",
		"INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS",
		"INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES",
		"INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidExpertiseEnv(t)
			t.Setenv(missing, "")
			_, err := LoadExpertiseConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadExpertiseConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"max_topics_zero", "INTELLIGENCE_EXPERTISE_MAX_TOPICS", "0"},
		{"max_topics_negative", "INTELLIGENCE_EXPERTISE_MAX_TOPICS", "-5"},
		{"maturity_zero", "INTELLIGENCE_EXPERTISE_MATURITY_DAYS", "0"},
		{"blind_spot_min_mentions_zero", "INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS", "0"},
		{"blind_spot_max_captures_zero", "INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES", "0"},
		{"blind_spot_limit_negative", "INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidExpertiseEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadExpertiseConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
