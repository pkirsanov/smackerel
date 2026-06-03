// Package config — Spec 075 SCOPE-1: legacy-surface deprecation telemetry SST.
//
// LegacyRetirementConfig governs the top-level `legacy_retirement.*`
// block. Every field originates in config/smackerel.yaml and flows
// through scripts/commands/config.sh into the generated env file as
// LEGACY_RETIREMENT_* variables. There are no in-source defaults
// (Gate G028, smackerel-no-defaults): every env var MUST be present
// at load time and Validate() rejects empty / out-of-range values
// unconditionally (this is a foundation block; there is no
// enabled=false short-circuit — operators who do not run a window
// still pay for explicit SST values).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// LegacyRetirementWindowState enumerates the operator-set SST window
// states. The third runtime state ("paused") is set by the threshold
// evaluator in Scope 4 and persisted in
// assistant_legacy_retirement_state, never in SST.
const (
	LegacyRetirementWindowOpen   = "open"
	LegacyRetirementWindowClosed = "closed"
)

// LegacyRetirementConfig is the SST surface for spec 075.
type LegacyRetirementConfig struct {
	// WindowID is the stable identifier that keys the dedup ledger
	// (assistant_conversations.legacy_retirement_notices) and the
	// runtime pause state row. MUST be non-empty.
	WindowID string
	// WindowState is the operator-set state. Allowed values: "open"
	// or "closed". The runtime "paused" state is never written here.
	WindowState string
	// RollbackThresholdPercentActiveUsers is the percent of active
	// users (denominator = ActiveUserWindowDays) above which residual
	// usage for a single retired command counts toward the
	// consecutive-day breach. > 0 and <= 100.
	RollbackThresholdPercentActiveUsers float64
	// RollbackThresholdDaysConsecutive is the number of consecutive
	// daily threshold breaches that automatically transitions the
	// window from open to paused. >= 1.
	RollbackThresholdDaysConsecutive int
	// PostWindowObservationDays is the operator-defined observation
	// window (in days) after window_state flips to closed. Final
	// handler deletion is gated on zero retired-handler invocations
	// over this period. >= 1.
	PostWindowObservationDays int
	// ActiveUserWindowDays is the lookback window for the active-user
	// denominator used by the rollback threshold evaluator. >= 1.
	ActiveUserWindowDays int
	// UserBucketHMACKey is the secret used to compute the
	// privacy-preserving HMAC-SHA256 user bucket label on residual
	// usage telemetry. MUST be non-empty; operators must override
	// the dev placeholder before any non-local deployment.
	UserBucketHMACKey string
	// NoticeCopyPerCommand maps each retired command token (e.g.
	// "/weather") to the short addendum rendered alongside the
	// primary NL response during an open window. The map MUST cover
	// every command in the spec 066 retired-command catalog; missing
	// entries fail loud at startup (validated by the policy layer
	// once it is wired to the catalog).
	NoticeCopyPerCommand map[string]string
	// PostWindowUnknownResponseCopy maps each retired command token
	// to the canonical unknown-command response body returned after
	// window_state=closed. Same coverage rule as NoticeCopyPerCommand.
	PostWindowUnknownResponseCopy map[string]string
	// Spec 076 SCOPE-6a — runtime wiring SST keys.
	//
	// ThresholdEvaluatorIntervalSeconds is the polling interval (in
	// seconds, >= 1) for the legacy-residual breach evaluator job
	// registered on the core scheduler.
	ThresholdEvaluatorIntervalSeconds int
	// ObservationCronExpr is the cron expression for the post-window
	// observation job that persists the zero-invocation gate report.
	// Non-empty; parsed by the scheduler at startup (fail-loud on
	// invalid expression).
	ObservationCronExpr string
	// RollbackThresholdDailyInvocations is the per-day flat-count
	// safety gate consulted alongside the percent-of-active-users
	// threshold. A day counts as breaching when EITHER gate fires.
	// >= 1.
	RollbackThresholdDailyInvocations int64
}

// LoadLegacyRetirement reads every LEGACY_RETIREMENT_* env var and
// returns a populated LegacyRetirementConfig plus Validate() result.
// Missing env vars (LookupEnv == false) are always a fail-loud
// [F075-SST-MISSING] error. Empty/invalid values are routed through
// Validate() which produces [F075-SST-INVALID].
func LoadLegacyRetirement() (LegacyRetirementConfig, error) {
	var cfg LegacyRetirementConfig
	var errs []string

	cfg.WindowID, errs = lookupString("LEGACY_RETIREMENT_WINDOW_ID", errs)
	cfg.WindowState, errs = lookupString("LEGACY_RETIREMENT_WINDOW_STATE", errs)
	cfg.RollbackThresholdPercentActiveUsers, errs = lookupFloat("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS", errs)
	cfg.RollbackThresholdDaysConsecutive, errs = lookupInt("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE", errs)
	cfg.PostWindowObservationDays, errs = lookupInt("LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS", errs)
	cfg.ActiveUserWindowDays, errs = lookupInt("LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS", errs)
	cfg.UserBucketHMACKey, errs = lookupString("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY", errs)
	cfg.NoticeCopyPerCommand, errs = lookupJSONStringMap("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND", errs)
	cfg.PostWindowUnknownResponseCopy, errs = lookupJSONStringMap("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY", errs)
	// Spec 076 SCOPE-6a — runtime wiring SST keys.
	cfg.ThresholdEvaluatorIntervalSeconds, errs = lookupInt("LEGACY_RETIREMENT_THRESHOLD_EVALUATOR_INTERVAL_SECONDS", errs)
	cfg.ObservationCronExpr, errs = lookupString("LEGACY_RETIREMENT_OBSERVATION_CRON_EXPR", errs)
	dailyInv, errs2 := lookupInt("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS", errs)
	errs = errs2
	cfg.RollbackThresholdDailyInvocations = int64(dailyInv)

	if len(errs) > 0 {
		return LegacyRetirementConfig{}, fmt.Errorf("[F075-SST-MISSING] missing or invalid required legacy_retirement configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return LegacyRetirementConfig{}, err
	}
	return cfg, nil
}

// Validate enforces spec 075 design §"Configuration And Migrations".
// No enabled=false short-circuit: this foundation always validates.
func (c *LegacyRetirementConfig) Validate() error {
	var errs []string

	if strings.TrimSpace(c.WindowID) == "" {
		errs = append(errs, "legacy_retirement.window_id (empty)")
	}
	switch c.WindowState {
	case LegacyRetirementWindowOpen, LegacyRetirementWindowClosed:
	default:
		errs = append(errs, fmt.Sprintf("legacy_retirement.window_state (must be %q or %q, got %q; %q is runtime-only and never SST)",
			LegacyRetirementWindowOpen, LegacyRetirementWindowClosed, c.WindowState, "paused"))
	}
	if c.RollbackThresholdPercentActiveUsers <= 0 || c.RollbackThresholdPercentActiveUsers > 100 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.rollback_threshold_percent_active_users (must be > 0 and <= 100, got %f)", c.RollbackThresholdPercentActiveUsers))
	}
	if c.RollbackThresholdDaysConsecutive < 1 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.rollback_threshold_days_consecutive (must be >= 1, got %d)", c.RollbackThresholdDaysConsecutive))
	}
	if c.PostWindowObservationDays < 1 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.post_window_observation_days (must be >= 1, got %d)", c.PostWindowObservationDays))
	}
	if c.ActiveUserWindowDays < 1 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.active_user_window_days (must be >= 1, got %d)", c.ActiveUserWindowDays))
	}
	if strings.TrimSpace(c.UserBucketHMACKey) == "" {
		errs = append(errs, "legacy_retirement.user_bucket_hmac_key (empty; HMAC bucket helper requires a non-empty secret)")
	}
	if len(c.NoticeCopyPerCommand) == 0 {
		errs = append(errs, "legacy_retirement.notice_copy_per_command (empty; must cover every retired command in the spec 066 catalog)")
	} else {
		for cmd, body := range c.NoticeCopyPerCommand {
			if strings.TrimSpace(cmd) == "" {
				errs = append(errs, "legacy_retirement.notice_copy_per_command (contains empty command key)")
				break
			}
			if strings.TrimSpace(body) == "" {
				errs = append(errs, fmt.Sprintf("legacy_retirement.notice_copy_per_command[%q] (empty body)", cmd))
			}
		}
	}
	if len(c.PostWindowUnknownResponseCopy) == 0 {
		errs = append(errs, "legacy_retirement.post_window_unknown_response_copy (empty; must cover every retired command in the spec 066 catalog)")
	} else {
		for cmd, body := range c.PostWindowUnknownResponseCopy {
			if strings.TrimSpace(cmd) == "" {
				errs = append(errs, "legacy_retirement.post_window_unknown_response_copy (contains empty command key)")
				break
			}
			if strings.TrimSpace(body) == "" {
				errs = append(errs, fmt.Sprintf("legacy_retirement.post_window_unknown_response_copy[%q] (empty body)", cmd))
			}
		}
	}

	// Spec 076 SCOPE-6a — runtime wiring SST keys.
	if c.ThresholdEvaluatorIntervalSeconds < 1 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.threshold_evaluator_interval_seconds (must be >= 1, got %d)", c.ThresholdEvaluatorIntervalSeconds))
	}
	if strings.TrimSpace(c.ObservationCronExpr) == "" {
		errs = append(errs, "legacy_retirement.observation_cron_expr (empty)")
	}
	if c.RollbackThresholdDailyInvocations < 1 {
		errs = append(errs, fmt.Sprintf("legacy_retirement.rollback_threshold_daily_invocations (must be >= 1, got %d)", c.RollbackThresholdDailyInvocations))
	}

	if len(errs) > 0 {
		return fmt.Errorf("[F075-SST-INVALID] invalid legacy_retirement configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// lookupJSONStringMap reads an env var as a JSON object whose values
// are strings (the YAML generator emits maps via yaml_get_json which
// produces a JSON object with scalar string values for our copy maps).
// Missing var → fail-loud; empty value tolerated for Validate() to
// catch; invalid JSON → typed error.
func lookupJSONStringMap(key string, errs []string) (map[string]string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil, append(errs, key+" (env var not set)")
	}
	if v == "" {
		return nil, errs
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, append(errs, fmt.Sprintf("%s (invalid JSON object<string,string>: %v)", key, err))
	}
	return out, errs
}
