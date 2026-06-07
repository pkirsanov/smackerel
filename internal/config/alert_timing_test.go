// Spec 021 BUG-021-006 — tests for the alert-timing OPERATIONAL SST loader.
package config

import (
	"strings"
	"testing"
)

func setValidAlertTimingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS", "30")
	t.Setenv("INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES", "25")
	t.Setenv("INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR", "0.7")
}

func TestLoadAlertTimingConfig_Populates(t *testing.T) {
	setValidAlertTimingEnv(t)
	cfg, err := LoadAlertTimingConfig()
	if err != nil {
		t.Fatalf("LoadAlertTimingConfig: %v", err)
	}
	if cfg.LookaheadDays != 30 {
		t.Errorf("LookaheadDays = %d, want 30", cfg.LookaheadDays)
	}
	if cfg.MaxCandidates != 25 {
		t.Errorf("MaxCandidates = %d, want 25", cfg.MaxCandidates)
	}
	if cfg.ConfidenceFloor != 0.7 {
		t.Errorf("ConfidenceFloor = %g, want 0.7", cfg.ConfidenceFloor)
	}
}

func TestLoadAlertTimingConfig_FailLoudOnMissing(t *testing.T) {
	keys := []string{
		"INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS",
		"INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES",
		"INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidAlertTimingEnv(t)
			t.Setenv(missing, "")
			_, err := LoadAlertTimingConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

func TestLoadAlertTimingConfig_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name, key, bad string
	}{
		{"lookahead_zero", "INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS", "0"},
		{"lookahead_negative", "INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS", "-5"},
		{"max_candidates_zero", "INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES", "0"},
		{"confidence_above_one", "INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR", "1.4"},
		{"confidence_negative", "INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR", "-0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidAlertTimingEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadAlertTimingConfig()
			if err == nil {
				t.Fatalf("expected validation error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}
