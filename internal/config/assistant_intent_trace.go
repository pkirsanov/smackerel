// Spec 071 SCOPE-01 — IntentTrace observability SST config.
//
// All keys are REQUIRED at the generator boundary (Gate G028 /
// smackerel-no-defaults). Missing values fail loud at startup; there
// are no fallback sampling, retention, or export defaults. The
// loader follows the same aggregate-then-error pattern used by
// loadIntentCompilerConfig (spec 068 SCOPE-1).

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// IntentTraceConfig holds the spec 071 SCOPE-01 SST values.
type IntentTraceConfig struct {
	// SamplingRatio is the deterministic full-trace sampling
	// fraction. 0.0..1.0. 1.0 means every compiled turn writes a
	// full trace; 0.0 means every turn writes a sampled-out
	// envelope (total-turn accounting still increments).
	SamplingRatio float64

	// RetentionDays is the TTL after which a trace row is eligible
	// for the retention sweep. >= 1.
	RetentionDays int

	// ExportTargets is the set of export sinks the recorder/exporter
	// must emit to. v1 closed vocabulary:
	//   structured_log | otel | prometheus
	// Non-empty.
	ExportTargets []string

	// ReplayEnabled gates the spec 071 SCOPE-03 replay CLI surface.
	ReplayEnabled bool

	// RetentionSweepInterval is the retention sweep cadence. > 0.
	RetentionSweepInterval time.Duration
}

// allowedIntentTraceExportTargets is the v1 export-target vocabulary.
var allowedIntentTraceExportTargets = map[string]bool{
	"structured_log": true,
	"otel":           true,
	"prometheus":     true,
}

// loadIntentTraceConfig populates cfg.Assistant.IntentTrace from
// ASSISTANT_INTENT_TRACE_* env vars. Appends to errs using the same
// pattern as loadIntentCompilerConfig.
func loadIntentTraceConfig(cfg *Config, errs *[]string) {
	mustBool := func(key string, dst *bool) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		switch v {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			*errs = append(*errs, fmt.Sprintf("%s (must be \"true\"|\"false\", got %q)", key, v))
		}
	}
	mustFloatInRange := func(key string, dst *float64, lo, hi float64) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return
		}
		if f < lo || f > hi {
			*errs = append(*errs, fmt.Sprintf("%s (must be in [%g,%g], got %g)", key, lo, hi, f))
			return
		}
		*dst = f
	}
	mustIntAtLeast := func(key string, dst *int, minVal int) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n < minVal {
			*errs = append(*errs, fmt.Sprintf("%s (must be >= %d, got %d)", key, minVal, n))
			return
		}
		*dst = n
	}
	mustDurationPositive := func(key string, dst *time.Duration) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be a Go duration, got %q)", key, v))
			return
		}
		if d <= 0 {
			*errs = append(*errs, fmt.Sprintf("%s (must be > 0, got %s)", key, d))
			return
		}
		*dst = d
	}
	mustExportTargets := func(key string, dst *[]string) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		seen := map[string]bool{}
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" {
				continue
			}
			if !allowedIntentTraceExportTargets[t] {
				*errs = append(*errs, fmt.Sprintf("%s (unknown target %q; allowed: otel, prometheus, structured_log)", key, t))
				return
			}
			if seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
		if len(out) == 0 {
			*errs = append(*errs, fmt.Sprintf("%s (must be a non-empty comma-separated list)", key))
			return
		}
		*dst = out
	}

	mustFloatInRange("ASSISTANT_INTENT_TRACE_SAMPLING_RATIO", &cfg.Assistant.IntentTrace.SamplingRatio, 0, 1)
	mustIntAtLeast("ASSISTANT_INTENT_TRACE_RETENTION_DAYS", &cfg.Assistant.IntentTrace.RetentionDays, 1)
	mustExportTargets("ASSISTANT_INTENT_TRACE_EXPORT_TARGETS", &cfg.Assistant.IntentTrace.ExportTargets)
	mustBool("ASSISTANT_INTENT_TRACE_REPLAY_ENABLED", &cfg.Assistant.IntentTrace.ReplayEnabled)
	mustDurationPositive("ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL", &cfg.Assistant.IntentTrace.RetentionSweepInterval)
}

// IntentTraceMissingKeyError formats an aggregate fail-loud error for
// the spec 071 SCOPE-01 keys.
func IntentTraceMissingKeyError(missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("[F071-SST-MISSING] missing or invalid required assistant intent trace configuration: %s", strings.Join(missing, ", "))
}
