// Spec 021 BUG-021-007 — tests for the resurfacing OPERATIONAL SST loader.
package config

import (
	"strings"
	"testing"
)

func setValidResurfaceEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS", "7")
	t.Setenv("INTELLIGENCE_RESURFACE_MAX_CANDIDATES", "25")
	t.Setenv("INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR", "0.7")
}

func TestLoadResurfaceConfig_Populates(t *testing.T) {
	setValidResurfaceEnv(t)
	cfg, err := LoadResurfaceConfig()
	if err != nil {
		t.Fatalf("LoadResurfaceConfig: %v", err)
	}
	if cfg.MinDormancyDays != 7 {
		t.Errorf("MinDormancyDays = %d, want 7", cfg.MinDormancyDays)
	}
	if cfg.MaxCandidates != 25 {
		t.Errorf("MaxCandidates = %d, want 25", cfg.MaxCandidates)
	}
	if cfg.ConfidenceFloor != 0.7 {
		t.Errorf("ConfidenceFloor = %g, want 0.7", cfg.ConfidenceFloor)
	}
}

func TestLoadResurfaceConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS",
		"INTELLIGENCE_RESURFACE_MAX_CANDIDATES",
		"INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidResurfaceEnv(t)
			t.Setenv(missing, "")
			_, err := LoadResurfaceConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadResurfaceConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"dormancy_zero", "INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS", "0"},
		{"dormancy_negative", "INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS", "-3"},
		{"max_candidates_zero", "INTELLIGENCE_RESURFACE_MAX_CANDIDATES", "0"},
		{"confidence_above_one", "INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR", "1.2"},
		{"confidence_negative", "INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR", "-0.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidResurfaceEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadResurfaceConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
