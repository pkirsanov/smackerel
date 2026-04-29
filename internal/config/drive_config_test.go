package config

import (
	"strings"
	"testing"
)

// driveSSTKeys lists every required DRIVE_* environment variable enforced by
// loadDriveConfig. The list mirrors the SST schema in config/smackerel.yaml
// under the `drive:` block. When `DRIVE_ENABLED=true`, OAuth client_id and
// client_secret are also required (covered separately below).
var driveSSTKeys = []string{
	"DRIVE_ENABLED",
	"DRIVE_CLASSIFICATION_ENABLED",
	"DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD",
	"DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION",
	"DRIVE_SCAN_PARALLELISM",
	"DRIVE_SCAN_BATCH_SIZE",
	"DRIVE_MONITOR_POLL_INTERVAL_SECONDS",
	"DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES",
	"DRIVE_POLICY_SENSITIVITY_DEFAULT",
	"DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC",
	"DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL",
	"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE",
	"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET",
	"DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES",
	"DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY",
	"DRIVE_LIMITS_MAX_FILE_SIZE_BYTES",
	"DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE",
	"DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL",
	"DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL",
	"DRIVE_PROVIDER_GOOGLE_API_BASE_URL",
	"DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS",
}

// TestDriveConfigValidationRequiresEverySSTField is the unit row for SCN-038-001.
// It proves that loadDriveConfig fails loud naming the offending env var when
// any required drive SST field is missing.
func TestDriveConfigValidationRequiresEverySSTField(t *testing.T) {
	for _, key := range driveSSTKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error must name %s, got: %v", key, err)
			}
		})
	}
}

// TestDriveConfigEnabledRequiresOAuthSecrets proves the conditional fail-loud
// rule: empty oauth_client_id / oauth_client_secret are accepted when
// drive.enabled=false but rejected when drive.enabled=true.
func TestDriveConfigEnabledRequiresOAuthSecrets(t *testing.T) {
	setRequiredEnv(t)
	// Baseline: DRIVE_ENABLED=false with empty secrets is accepted.
	if _, err := Load(); err != nil {
		t.Fatalf("baseline (drive.enabled=false, empty secrets) must load: %v", err)
	}

	setRequiredEnv(t)
	t.Setenv("DRIVE_ENABLED", "true")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID", "")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when drive.enabled=true with empty OAuth secrets")
	}
	if !strings.Contains(err.Error(), "DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID") {
		t.Errorf("error must name DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), "DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET") {
		t.Errorf("error must name DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET, got: %v", err)
	}
}

// TestDriveConfigPopulatesEveryField loads the test fixture (matching the
// dev SST defaults) and verifies that every required field round-trips into
// the typed Config.Drive struct.
func TestDriveConfigPopulatesEveryField(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := cfg.Drive
	if d.Enabled {
		t.Errorf("Drive.Enabled = true, want false (test fixture default)")
	}
	if !d.Classification.Enabled {
		t.Errorf("Drive.Classification.Enabled = false, want true")
	}
	if d.Classification.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %v, want 0.7", d.Classification.ConfidenceThreshold)
	}
	if d.Classification.LowConfidenceAction != "pause" {
		t.Errorf("LowConfidenceAction = %q, want pause", d.Classification.LowConfidenceAction)
	}
	if d.Scan.Parallelism != 4 || d.Scan.BatchSize != 100 {
		t.Errorf("Scan parallelism/batch_size = %d/%d, want 4/100", d.Scan.Parallelism, d.Scan.BatchSize)
	}
	if d.Monitor.PollIntervalSeconds != 300 || d.Monitor.CursorInvalidationRescanMaxFiles != 5000 {
		t.Errorf("Monitor = %+v", d.Monitor)
	}
	if d.Policy.SensitivityDefault != "internal" {
		t.Errorf("SensitivityDefault = %q", d.Policy.SensitivityDefault)
	}
	if d.Policy.SensitivityThresholds.Public != 0.95 ||
		d.Policy.SensitivityThresholds.Internal != 0.80 ||
		d.Policy.SensitivityThresholds.Sensitive != 0.60 ||
		d.Policy.SensitivityThresholds.Secret != 0.50 {
		t.Errorf("SensitivityThresholds = %+v", d.Policy.SensitivityThresholds)
	}
	if d.Telegram.MaxInlineSizeBytes != 5242880 || d.Telegram.MaxLinkFilesPerReply != 10 {
		t.Errorf("Telegram = %+v", d.Telegram)
	}
	if d.Limits.MaxFileSizeBytes != 104857600 {
		t.Errorf("MaxFileSizeBytes = %d", d.Limits.MaxFileSizeBytes)
	}
	if d.RateLimits.RequestsPerMinute != 600 {
		t.Errorf("RequestsPerMinute = %d", d.RateLimits.RequestsPerMinute)
	}
	if d.Providers.Google.OAuthRedirectURL == "" {
		t.Error("OAuthRedirectURL is empty")
	}
	if d.Providers.Google.OAuthBaseURL == "" {
		t.Error("OAuthBaseURL is empty")
	}
	if d.Providers.Google.APIBaseURL == "" {
		t.Error("APIBaseURL is empty")
	}
	if len(d.Providers.Google.ScopeDefaults) == 0 {
		t.Error("ScopeDefaults must contain at least one scope")
	}
}

// TestDriveConfigRejectsInvalidEnumValues proves enum validation rejects
// out-of-range strings at the boundary.
func TestDriveConfigRejectsInvalidEnumValues(t *testing.T) {
	cases := []struct {
		key     string
		bad     string
		mustSay string
	}{
		{"DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION", "drop", "DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION"},
		{"DRIVE_POLICY_SENSITIVITY_DEFAULT", "topsecret", "DRIVE_POLICY_SENSITIVITY_DEFAULT"},
		{"DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD", "1.5", "DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD"},
		{"DRIVE_SCAN_PARALLELISM", "0", "DRIVE_SCAN_PARALLELISM"},
		{"DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS", "[]", "DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.key+"="+tc.bad, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for %s=%q", tc.key, tc.bad)
			}
			if !strings.Contains(err.Error(), tc.mustSay) {
				t.Errorf("error must name %s, got: %v", tc.mustSay, err)
			}
		})
	}
}
