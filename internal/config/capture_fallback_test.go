// Spec 074 SCOPE-1 TP-074-01 — capture_as_fallback SST fail-loud test.
//
// SCN-074-A08 — when capture_as_fallback.dedup_window is unset, the
// core process MUST fail startup with a NO-DEFAULTS error naming the
// missing key. Mirrors the loader test pattern used for spec 075
// (legacy_retirement) and spec 071 (intent_trace).
package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadCaptureFallback_MissingDedupWindowFailsLoud(t *testing.T) {
	t.Setenv("CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT", "10m")
	t.Setenv("CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY", NormalizationPolicyV1)
	t.Setenv("CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY", "test-key")
	t.Setenv("CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS", "90")
	unsetEnvForTest(t, "CAPTURE_AS_FALLBACK_DEDUP_WINDOW")

	_, err := LoadCaptureFallback()
	if err == nil {
		t.Fatalf("LoadCaptureFallback succeeded; expected fail-loud error for missing CAPTURE_AS_FALLBACK_DEDUP_WINDOW")
	}
	msg := err.Error()
	if !strings.Contains(msg, "[F074-SST-MISSING]") {
		t.Errorf("error prefix mismatch: got %q, want substring %q", msg, "[F074-SST-MISSING]")
	}
	if !strings.Contains(msg, "CAPTURE_AS_FALLBACK_DEDUP_WINDOW") {
		t.Errorf("missing-key name not surfaced: got %q, want substring %q", msg, "CAPTURE_AS_FALLBACK_DEDUP_WINDOW")
	}
}

func TestLoadCaptureFallback_AllPresentSucceeds(t *testing.T) {
	t.Setenv("CAPTURE_AS_FALLBACK_DEDUP_WINDOW", "24h")
	t.Setenv("CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT", "10m")
	t.Setenv("CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY", NormalizationPolicyV1)
	t.Setenv("CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY", "test-key")
	t.Setenv("CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS", "90")

	cfg, err := LoadCaptureFallback()
	if err != nil {
		t.Fatalf("LoadCaptureFallback failed: %v", err)
	}
	if cfg.DedupWindow.String() != "24h0m0s" {
		t.Errorf("DedupWindow = %s, want 24h0m0s", cfg.DedupWindow)
	}
	if cfg.NormalizationPolicy != NormalizationPolicyV1 {
		t.Errorf("NormalizationPolicy = %q, want %q", cfg.NormalizationPolicy, NormalizationPolicyV1)
	}
	if cfg.RetentionAuditDays != 90 {
		t.Errorf("RetentionAuditDays = %d, want 90", cfg.RetentionAuditDays)
	}
}

func TestCaptureFallbackConfig_ValidateRejectsBadValues(t *testing.T) {
	cfg := CaptureFallbackConfig{
		DedupWindow:           0,
		ClarifyAbandonTimeout: 0,
		NormalizationPolicy:   "unknown_policy",
		DedupHashKey:          "",
		RetentionAuditDays:    0,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("Validate accepted empty config; expected [F074-SST-INVALID]")
	}
	for _, sub := range []string{
		"[F074-SST-INVALID]",
		"dedup_window",
		"clarify_abandon_timeout",
		"normalization_policy",
		"dedup_hash_key",
		"retention_audit_days",
	} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("validate error missing %q: %v", sub, err)
		}
	}
}

// unsetEnvForTest unsets a variable for the duration of the test and
// restores any inherited parent value at cleanup time. t.Setenv only
// supports set+restore; the equivalent unset+restore is open-coded.
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("os.Unsetenv(%q): %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
