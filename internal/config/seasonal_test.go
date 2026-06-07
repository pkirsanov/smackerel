// Spec 021 BUG-021-009 — tests for the seasonal-detection OPERATIONAL SST loader.
package config

import (
	"strings"
	"testing"
)

func setValidSeasonalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INTELLIGENCE_SEASONAL_MIN_DATA_DAYS", "180")
	t.Setenv("INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES", "5")
	t.Setenv("INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT", "5")
	t.Setenv("INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS", "2")
}

func TestLoadSeasonalConfig_Populates(t *testing.T) {
	setValidSeasonalEnv(t)
	cfg, err := LoadSeasonalConfig()
	if err != nil {
		t.Fatalf("LoadSeasonalConfig: %v", err)
	}
	if cfg.MinDataDays != 180 {
		t.Errorf("MinDataDays = %d, want 180", cfg.MinDataDays)
	}
	if cfg.TopicMinCaptures != 5 || cfg.TopicCandidateLimit != 5 {
		t.Errorf("topic bounds = %d/%d, want 5/5", cfg.TopicMinCaptures, cfg.TopicCandidateLimit)
	}
	if cfg.MaxObservations != 2 {
		t.Errorf("MaxObservations = %d, want 2", cfg.MaxObservations)
	}
}

func TestLoadSeasonalConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"INTELLIGENCE_SEASONAL_MIN_DATA_DAYS",
		"INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES",
		"INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT",
		"INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidSeasonalEnv(t)
			t.Setenv(missing, "")
			_, err := LoadSeasonalConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadSeasonalConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"min_data_days_zero", "INTELLIGENCE_SEASONAL_MIN_DATA_DAYS", "0"},
		{"min_data_days_negative", "INTELLIGENCE_SEASONAL_MIN_DATA_DAYS", "-30"},
		{"topic_min_captures_zero", "INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES", "0"},
		{"topic_candidate_limit_zero", "INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT", "0"},
		{"max_observations_negative", "INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidSeasonalEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadSeasonalConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
