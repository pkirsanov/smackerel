// Spec 021 BUG-021-005 — tests for the relationship-cooling OPERATIONAL SST
// loader. Self-contained: each case sets only the env it needs.
package config

import (
	"strings"
	"testing"
)

func setValidCoolingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES", "25")
	t.Setenv("INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR", "0.7")
	t.Setenv("INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS", "30")
}

func TestLoadRelationshipCoolingConfig_Populates(t *testing.T) {
	setValidCoolingEnv(t)
	cfg, err := LoadRelationshipCoolingConfig()
	if err != nil {
		t.Fatalf("LoadRelationshipCoolingConfig: %v", err)
	}
	if cfg.MaxCandidates != 25 {
		t.Errorf("MaxCandidates = %d, want 25", cfg.MaxCandidates)
	}
	if cfg.ConfidenceFloor != 0.7 {
		t.Errorf("ConfidenceFloor = %g, want 0.7", cfg.ConfidenceFloor)
	}
	if cfg.DedupWindowDays != 30 {
		t.Errorf("DedupWindowDays = %d, want 30", cfg.DedupWindowDays)
	}
}

func TestLoadRelationshipCoolingConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES",
		"INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR",
		"INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidCoolingEnv(t)
			t.Setenv(missing, "")
			_, err := LoadRelationshipCoolingConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadRelationshipCoolingConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"max_candidates_zero", "INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES", "0"},
		{"max_candidates_negative", "INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES", "-1"},
		{"confidence_floor_above_one", "INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR", "1.5"},
		{"confidence_floor_negative", "INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR", "-0.1"},
		{"dedup_window_zero", "INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidCoolingEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadRelationshipCoolingConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
