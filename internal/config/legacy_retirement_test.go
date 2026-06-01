package config

import (
	"os"
	"strings"
	"testing"
)

// Spec 075 SCOPE-1 — TP-075-01 / SCN-075-A10.
//
// LoadLegacyRetirement MUST fail loud when any LEGACY_RETIREMENT_* env
// var is missing (LookupEnv == false) — the deprecation window cannot
// be opened without a fully-populated SST block. This is the
// adversarial coverage for the NO-DEFAULTS / fail-loud SST policy
// (Gate G028 / smackerel-no-defaults): a regression that reintroduces
// a hidden default for the rollback threshold would let the test
// loader proceed without it, and the assertions below would trip.

// baseLegacyRetirementEnv returns a fully-populated set of every
// LEGACY_RETIREMENT_* env var. Validate() passes on this baseline.
func baseLegacyRetirementEnv() map[string]string {
	return map[string]string{
		"LEGACY_RETIREMENT_WINDOW_ID":                               "2026-05-retirement",
		"LEGACY_RETIREMENT_WINDOW_STATE":                            "open",
		"LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS": "5.0",
		"LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE":     "3",
		"LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS":            "14",
		"LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS":                 "7",
		"LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY":                    "test-hmac-key-not-for-prod",
		"LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND":                 `{"/weather":"Try plain English: weather in Barcelona tomorrow."}`,
		"LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY":       `{"/weather":"I do not use /weather anymore."}`,
	}
}

// clearLegacyRetirementEnv unsets every LEGACY_RETIREMENT_* env var so
// the test does not inherit values from the surrounding shell or a
// sibling test.
func clearLegacyRetirementEnv(t *testing.T) {
	t.Helper()
	for k := range baseLegacyRetirementEnv() {
		_ = os.Unsetenv(k)
	}
}

func applyLegacyRetirementEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// TestLegacyRetirement_MissingRollbackThresholdKeyFailsLoud is the
// adversarial regression for SCN-075-A10. A regression that quietly
// defaults LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS
// to a hidden value (e.g. 0 or 5) would let LoadLegacyRetirement
// return a populated config when the env var is absent; the assertions
// below trip if that happens.
func TestLegacyRetirement_MissingRollbackThresholdKeyFailsLoud(t *testing.T) {
	clearLegacyRetirementEnv(t)
	env := baseLegacyRetirementEnv()
	delete(env, "LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS")
	applyLegacyRetirementEnv(t, env)

	_, err := LoadLegacyRetirement()
	if err == nil {
		t.Fatalf("LoadLegacyRetirement must fail loud when LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS is unset; got nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "[F075-SST-MISSING]") {
		t.Errorf("error must carry the [F075-SST-MISSING] fail-loud prefix; got %q", msg)
	}
	if !strings.Contains(msg, "LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS") {
		t.Errorf("error must name the missing key; got %q", msg)
	}
}

// TestLegacyRetirement_MissingEachRequiredKeyFailsLoud covers every
// required env var, not just the rollback threshold. Each subtest
// drops exactly one key and asserts that the loader fails loud
// naming that key — preventing a future regression that adds a
// silent default to any single field.
func TestLegacyRetirement_MissingEachRequiredKeyFailsLoud(t *testing.T) {
	base := baseLegacyRetirementEnv()
	for missing := range base {
		missing := missing
		t.Run(missing, func(t *testing.T) {
			clearLegacyRetirementEnv(t)
			env := baseLegacyRetirementEnv()
			delete(env, missing)
			applyLegacyRetirementEnv(t, env)

			_, err := LoadLegacyRetirement()
			if err == nil {
				t.Fatalf("missing %s must fail loud; got nil error", missing)
			}
			if !strings.Contains(err.Error(), "[F075-SST-MISSING]") {
				t.Errorf("missing %s must carry [F075-SST-MISSING]; got %q", missing, err.Error())
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("missing %s error must name the key; got %q", missing, err.Error())
			}
		})
	}
}

// TestLegacyRetirement_BaselineLoadsAndValidates is the canary that
// proves the baseline env we use as the test foundation actually
// passes Validate(). A drift between the baseline and the validator
// would make the missing-key subtests pass for the wrong reason.
func TestLegacyRetirement_BaselineLoadsAndValidates(t *testing.T) {
	clearLegacyRetirementEnv(t)
	applyLegacyRetirementEnv(t, baseLegacyRetirementEnv())

	cfg, err := LoadLegacyRetirement()
	if err != nil {
		t.Fatalf("baseline LoadLegacyRetirement must succeed; got %v", err)
	}
	if cfg.WindowID == "" {
		t.Errorf("baseline WindowID must be populated")
	}
	if cfg.RollbackThresholdPercentActiveUsers <= 0 {
		t.Errorf("baseline RollbackThresholdPercentActiveUsers must be > 0; got %f", cfg.RollbackThresholdPercentActiveUsers)
	}
	if _, ok := cfg.NoticeCopyPerCommand["/weather"]; !ok {
		t.Errorf("baseline NoticeCopyPerCommand must include /weather; got %v", cfg.NoticeCopyPerCommand)
	}
}

// TestLegacyRetirement_InvalidValuesFailLoud covers Validate() rules
// that complement the load-time missing-key checks. A regression that
// silently clamps an out-of-range value (e.g. percent > 100 → 100,
// days < 1 → 1) would let the loader return a struct; the assertions
// below trip if that happens.
func TestLegacyRetirement_InvalidValuesFailLoud(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantSub string
	}{
		{
			name:    "window_state_paused_is_runtime_only",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_WINDOW_STATE"] = "paused" },
			wantSub: "legacy_retirement.window_state",
		},
		{
			name:    "rollback_threshold_percent_zero",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS"] = "0" },
			wantSub: "rollback_threshold_percent_active_users",
		},
		{
			name:    "rollback_threshold_percent_above_100",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS"] = "150" },
			wantSub: "rollback_threshold_percent_active_users",
		},
		{
			name:    "rollback_days_below_one",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE"] = "0" },
			wantSub: "rollback_threshold_days_consecutive",
		},
		{
			name:    "observation_days_below_one",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS"] = "0" },
			wantSub: "post_window_observation_days",
		},
		{
			name:    "active_user_window_below_one",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS"] = "0" },
			wantSub: "active_user_window_days",
		},
		{
			name:    "hmac_key_empty",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY"] = "" },
			wantSub: "user_bucket_hmac_key",
		},
		{
			name:    "notice_copy_empty_map",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND"] = "{}" },
			wantSub: "notice_copy_per_command",
		},
		{
			name:    "closed_copy_empty_map",
			mutate:  func(m map[string]string) { m["LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY"] = "{}" },
			wantSub: "post_window_unknown_response_copy",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			clearLegacyRetirementEnv(t)
			env := baseLegacyRetirementEnv()
			tc.mutate(env)
			applyLegacyRetirementEnv(t, env)

			_, err := LoadLegacyRetirement()
			if err == nil {
				t.Fatalf("%s must fail; got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("%s error must mention %q; got %q", tc.name, tc.wantSub, err.Error())
			}
		})
	}
}
