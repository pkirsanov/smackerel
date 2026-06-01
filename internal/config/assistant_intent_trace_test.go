// Spec 071 SCOPE-01 — IntentTrace SST fail-loud test (SCN-071-A05).

package config

import (
	"strings"
	"testing"
)

// TestIntentTraceConfigRequiresEverySSTKey asserts the spec 071
// SCOPE-01 SST keys are all REQUIRED (no defaults; Gate G028 /
// smackerel-no-defaults). Mirrors the spec 068 SCOPE-1 pattern.
func TestIntentTraceConfigRequiresEverySSTKey(t *testing.T) {
	requiredKeys := []string{
		"ASSISTANT_INTENT_TRACE_SAMPLING_RATIO",
		"ASSISTANT_INTENT_TRACE_RETENTION_DAYS",
		"ASSISTANT_INTENT_TRACE_EXPORT_TARGETS",
		"ASSISTANT_INTENT_TRACE_REPLAY_ENABLED",
		"ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL",
	}

	t.Run("all_missing_names_every_key", func(t *testing.T) {
		for _, k := range requiredKeys {
			t.Setenv(k, "")
		}
		var errs []string
		cfg := &Config{}
		loadIntentTraceConfig(cfg, &errs)
		if len(errs) == 0 {
			t.Fatalf("expected loadIntentTraceConfig to record missing keys, got none")
		}
		joined := strings.Join(errs, ",")
		for _, k := range requiredKeys {
			if !strings.Contains(joined, k) {
				t.Errorf("expected missing-key error to name %q, got %q", k, joined)
			}
		}
		err := IntentTraceMissingKeyError(errs)
		if err == nil {
			t.Fatalf("expected non-nil aggregate error")
		}
		if !strings.Contains(err.Error(), "[F071-SST-MISSING]") {
			t.Fatalf("expected aggregate error to carry [F071-SST-MISSING] tag, got %q", err.Error())
		}
	})

	t.Run("fully_populated_no_errors", func(t *testing.T) {
		t.Setenv("ASSISTANT_INTENT_TRACE_SAMPLING_RATIO", "0.25")
		t.Setenv("ASSISTANT_INTENT_TRACE_RETENTION_DAYS", "14")
		t.Setenv("ASSISTANT_INTENT_TRACE_EXPORT_TARGETS", "structured_log,otel,prometheus")
		t.Setenv("ASSISTANT_INTENT_TRACE_REPLAY_ENABLED", "true")
		t.Setenv("ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL", "1h")
		var errs []string
		cfg := &Config{}
		loadIntentTraceConfig(cfg, &errs)
		if len(errs) != 0 {
			t.Fatalf("expected zero errs, got %v", errs)
		}
		it := cfg.Assistant.IntentTrace
		if it.SamplingRatio != 0.25 || it.RetentionDays != 14 ||
			len(it.ExportTargets) != 3 || !it.ReplayEnabled ||
			it.RetentionSweepInterval.String() != "1h0m0s" {
			t.Fatalf("loaded IntentTrace does not match env: %+v", it)
		}
	})

	t.Run("each_key_independently_required", func(t *testing.T) {
		base := map[string]string{
			"ASSISTANT_INTENT_TRACE_SAMPLING_RATIO":           "1.0",
			"ASSISTANT_INTENT_TRACE_RETENTION_DAYS":           "7",
			"ASSISTANT_INTENT_TRACE_EXPORT_TARGETS":           "structured_log",
			"ASSISTANT_INTENT_TRACE_REPLAY_ENABLED":           "false",
			"ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL": "30m",
		}
		for _, target := range requiredKeys {
			t.Run("missing_"+target, func(t *testing.T) {
				for k, v := range base {
					if k == target {
						t.Setenv(k, "")
					} else {
						t.Setenv(k, v)
					}
				}
				var errs []string
				loadIntentTraceConfig(&Config{}, &errs)
				if len(errs) == 0 {
					t.Fatalf("missing %s should produce an err entry; got none", target)
				}
				if !strings.Contains(strings.Join(errs, ","), target) {
					t.Fatalf("missing-key err for %s not present: %v", target, errs)
				}
			})
		}
	})

	t.Run("unknown_export_target_rejected", func(t *testing.T) {
		t.Setenv("ASSISTANT_INTENT_TRACE_SAMPLING_RATIO", "1.0")
		t.Setenv("ASSISTANT_INTENT_TRACE_RETENTION_DAYS", "7")
		t.Setenv("ASSISTANT_INTENT_TRACE_EXPORT_TARGETS", "structured_log,loki_secret_sink")
		t.Setenv("ASSISTANT_INTENT_TRACE_REPLAY_ENABLED", "false")
		t.Setenv("ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL", "30m")
		var errs []string
		loadIntentTraceConfig(&Config{}, &errs)
		if len(errs) == 0 {
			t.Fatalf("expected unknown export target to fail loud")
		}
		if !strings.Contains(strings.Join(errs, ","), "loki_secret_sink") {
			t.Fatalf("expected error to name unknown target, got %v", errs)
		}
	})
}
