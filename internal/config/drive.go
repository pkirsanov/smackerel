package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DriveConfig captures the fully-resolved Cloud Drives Integration (spec 038)
// configuration block sourced from config/smackerel.yaml via the SST pipeline.
// Every field is required at runtime; OAuth secrets are tolerated as empty
// strings only when DriveConfig.Enabled is false.
type DriveConfig struct {
	Enabled        bool
	Classification DriveClassificationConfig
	Scan           DriveScanConfig
	Monitor        DriveMonitorConfig
	Policy         DrivePolicyConfig
	Telegram       DriveTelegramConfig
	Limits         DriveLimitsConfig
	IOLimits       DriveIOLimitsConfig
	RateLimits     DriveRateLimitsConfig
	Save           DriveSaveConfig
	Providers      DriveProvidersConfig
}

type DriveClassificationConfig struct {
	Enabled                bool
	ConfidenceThreshold    float64
	LowConfidenceAction    string // pause | skip | allow
	ConfirmThreshold       float64
	ConfirmationTTLSeconds int
}

type DriveScanConfig struct {
	Parallelism int
	BatchSize   int
}

type DriveMonitorConfig struct {
	PollIntervalSeconds              int
	CursorInvalidationRescanMaxFiles int
}

type DrivePolicyConfig struct {
	SensitivityDefault    string // public | internal | sensitive | secret
	SensitivityThresholds DriveSensitivityThresholds
}

type DriveSensitivityThresholds struct {
	Public    float64
	Internal  float64
	Sensitive float64
	Secret    float64
}

type DriveTelegramConfig struct {
	MaxInlineSizeBytes   int64
	MaxLinkFilesPerReply int
}

type DriveLimitsConfig struct {
	MaxFileSizeBytes int64
}

// DriveIOLimitsConfig holds the SST byte caps applied around `io.ReadAll`
// calls on HTTP boundaries in the Google Drive provider (MIT-038-S-004).
// Values come from the `drive.io_limits.*` SST keys; missing or non-positive
// values are a fail-loud config error. The caps are defense-in-depth on
// top of the existing `drive.limits.max_file_size_bytes` enforcement at
// the bytes-processing path in internal/drive/extract/service.go:255.
//
//	ProviderResponseMaxBytes — JSON metadata responses (Drive list, about,
//	  changes, file-metadata error bodies, EnsureFolder, PutFile response).
//	ProviderBinaryMaxBytes   — binary file content read in PutFile (where
//	  body.Reader is the uploaded content).
//	OAuthResponseMaxBytes    — OAuth token-exchange response body.
type DriveIOLimitsConfig struct {
	ProviderResponseMaxBytes int64
	ProviderBinaryMaxBytes   int64
	OAuthResponseMaxBytes    int64
}

type DriveRateLimitsConfig struct {
	RequestsPerMinute int
}

// DriveSaveConfig captures the Save Rules and Write-Back configuration
// added in spec 038 Scope 5. ProviderURLPrefix is required even in dev so
// the framework's SST guard can prove the value flows from
// config/smackerel.yaml all the way to runtime.
type DriveSaveConfig struct {
	ProviderURLPrefix string
}

type DriveProvidersConfig struct {
	Google DriveGoogleProviderConfig
}

type DriveGoogleProviderConfig struct {
	OAuthClientID     string
	OAuthClientSecret string
	OAuthRedirectURL  string
	OAuthBaseURL      string
	APIBaseURL        string
	ScopeDefaults     []string
	// IOLimits carries the SST-resolved drive.io_limits.* caps so the
	// Google provider can wrap io.ReadAll(resp.Body) sites with
	// io.LimitReader without depending on the top-level config package
	// (MIT-038-S-004). Populated by loadDriveConfig from cfg.IOLimits.
	IOLimits DriveIOLimitsConfig
}

// validSensitivityTiers enumerates the policy.sensitivity_default options.
var validSensitivityTiers = map[string]struct{}{
	"public":    {},
	"internal":  {},
	"sensitive": {},
	"secret":    {},
}

// validLowConfidenceActions enumerates classification.low_confidence_action options.
var validLowConfidenceActions = map[string]struct{}{
	"pause": {},
	"skip":  {},
	"allow": {},
}

// loadDriveConfig parses every DRIVE_* env var into a DriveConfig and validates
// it. Returns an error naming every missing or invalid field. Empty OAuth
// secrets are accepted only when drive.enabled=false.
func loadDriveConfig() (DriveConfig, error) {
	var cfg DriveConfig
	var errs []string

	enabledRaw := os.Getenv("DRIVE_ENABLED")
	if enabledRaw == "" {
		errs = append(errs, "DRIVE_ENABLED")
	} else if enabledRaw != "true" && enabledRaw != "false" {
		errs = append(errs, "DRIVE_ENABLED (must be true or false)")
	} else {
		cfg.Enabled = enabledRaw == "true"
	}

	// classification
	clEnabledRaw := os.Getenv("DRIVE_CLASSIFICATION_ENABLED")
	if clEnabledRaw == "" {
		errs = append(errs, "DRIVE_CLASSIFICATION_ENABLED")
	} else if clEnabledRaw != "true" && clEnabledRaw != "false" {
		errs = append(errs, "DRIVE_CLASSIFICATION_ENABLED (must be true or false)")
	} else {
		cfg.Classification.Enabled = clEnabledRaw == "true"
	}

	if v := os.Getenv("DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD"); v == "" {
		errs = append(errs, "DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 || f > 1 {
		errs = append(errs, "DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD (must be a float in [0, 1])")
	} else {
		cfg.Classification.ConfidenceThreshold = f
	}

	cfg.Classification.LowConfidenceAction = os.Getenv("DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION")
	if cfg.Classification.LowConfidenceAction == "" {
		errs = append(errs, "DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION")
	} else if _, ok := validLowConfidenceActions[cfg.Classification.LowConfidenceAction]; !ok {
		errs = append(errs, "DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION (must be one of pause|skip|allow)")
	}

	if v := os.Getenv("DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD"); v == "" {
		errs = append(errs, "DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 || f > 1 {
		errs = append(errs, "DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD (must be a float in [0, 1])")
	} else {
		cfg.Classification.ConfirmThreshold = f
	}

	cfg.Classification.ConfirmationTTLSeconds, errs = parsePositiveInt("DRIVE_CLASSIFICATION_CONFIRMATION_TTL_SECONDS", errs)

	// scan
	cfg.Scan.Parallelism, errs = parsePositiveInt("DRIVE_SCAN_PARALLELISM", errs)
	cfg.Scan.BatchSize, errs = parsePositiveInt("DRIVE_SCAN_BATCH_SIZE", errs)

	// monitor
	cfg.Monitor.PollIntervalSeconds, errs = parsePositiveInt("DRIVE_MONITOR_POLL_INTERVAL_SECONDS", errs)
	cfg.Monitor.CursorInvalidationRescanMaxFiles, errs = parsePositiveInt("DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES", errs)

	// policy
	cfg.Policy.SensitivityDefault = os.Getenv("DRIVE_POLICY_SENSITIVITY_DEFAULT")
	if cfg.Policy.SensitivityDefault == "" {
		errs = append(errs, "DRIVE_POLICY_SENSITIVITY_DEFAULT")
	} else if _, ok := validSensitivityTiers[cfg.Policy.SensitivityDefault]; !ok {
		errs = append(errs, "DRIVE_POLICY_SENSITIVITY_DEFAULT (must be one of public|internal|sensitive|secret)")
	}

	cfg.Policy.SensitivityThresholds.Public, errs = parseUnitFloat("DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC", errs)
	cfg.Policy.SensitivityThresholds.Internal, errs = parseUnitFloat("DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL", errs)
	cfg.Policy.SensitivityThresholds.Sensitive, errs = parseUnitFloat("DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE", errs)
	cfg.Policy.SensitivityThresholds.Secret, errs = parseUnitFloat("DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET", errs)

	// telegram
	cfg.Telegram.MaxInlineSizeBytes, errs = parsePositiveInt64("DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES", errs)
	cfg.Telegram.MaxLinkFilesPerReply, errs = parsePositiveInt("DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY", errs)

	// limits
	cfg.Limits.MaxFileSizeBytes, errs = parsePositiveInt64("DRIVE_LIMITS_MAX_FILE_SIZE_BYTES", errs)

	// io_limits (MIT-038-S-004 — defense-in-depth byte caps on Google
	// provider HTTP boundaries; fail-loud on missing or non-positive values).
	cfg.IOLimits.ProviderResponseMaxBytes, errs = parsePositiveInt64("DRIVE_IO_LIMITS_PROVIDER_RESPONSE_MAX_BYTES", errs)
	cfg.IOLimits.ProviderBinaryMaxBytes, errs = parsePositiveInt64("DRIVE_IO_LIMITS_PROVIDER_BINARY_MAX_BYTES", errs)
	cfg.IOLimits.OAuthResponseMaxBytes, errs = parsePositiveInt64("DRIVE_IO_LIMITS_OAUTH_RESPONSE_MAX_BYTES", errs)

	// rate_limits
	cfg.RateLimits.RequestsPerMinute, errs = parsePositiveInt("DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE", errs)

	// save (spec 038 Scope 5)
	cfg.Save.ProviderURLPrefix = os.Getenv("DRIVE_SAVE_PROVIDER_URL_PREFIX")
	if cfg.Save.ProviderURLPrefix == "" {
		errs = append(errs, "DRIVE_SAVE_PROVIDER_URL_PREFIX")
	}

	// providers.google
	cfg.Providers.Google.OAuthClientID = os.Getenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID")
	cfg.Providers.Google.OAuthClientSecret = os.Getenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET")
	cfg.Providers.Google.OAuthRedirectURL = os.Getenv("DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL")
	if cfg.Providers.Google.OAuthRedirectURL == "" {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL")
	}

	cfg.Providers.Google.OAuthBaseURL = os.Getenv("DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL")
	if cfg.Providers.Google.OAuthBaseURL == "" {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL")
	} else if !strings.HasPrefix(cfg.Providers.Google.OAuthBaseURL, "http://") && !strings.HasPrefix(cfg.Providers.Google.OAuthBaseURL, "https://") {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL (must be an absolute http(s) URL)")
	}

	cfg.Providers.Google.APIBaseURL = os.Getenv("DRIVE_PROVIDER_GOOGLE_API_BASE_URL")
	if cfg.Providers.Google.APIBaseURL == "" {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_API_BASE_URL")
	} else if !strings.HasPrefix(cfg.Providers.Google.APIBaseURL, "http://") && !strings.HasPrefix(cfg.Providers.Google.APIBaseURL, "https://") {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_API_BASE_URL (must be an absolute http(s) URL)")
	}

	scopeRaw := os.Getenv("DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS")
	if scopeRaw == "" || scopeRaw == "[]" {
		errs = append(errs, "DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS (must be a non-empty JSON list)")
	} else {
		var scopes []string
		if err := json.Unmarshal([]byte(scopeRaw), &scopes); err != nil {
			errs = append(errs, "DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS (invalid JSON)")
		} else if len(scopes) == 0 {
			errs = append(errs, "DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS (must contain at least one scope)")
		} else {
			cfg.Providers.Google.ScopeDefaults = scopes
		}
	}

	// Fail-loud secret enforcement when the subsystem is enabled.
	if cfg.Enabled {
		if cfg.Providers.Google.OAuthClientID == "" {
			errs = append(errs, "DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID (required when drive.enabled=true)")
		}
		if cfg.Providers.Google.OAuthClientSecret == "" {
			errs = append(errs, "DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET (required when drive.enabled=true)")
		}
	}

	// MIT-038-S-004 — propagate the SST-resolved drive.io_limits.* caps
	// into the per-provider config so the Google provider's wiring path
	// (cmd/core/wiring.go calls ConfigureRuntime with this struct) carries
	// the caps down to internal/drive/google without that package importing
	// the top-level DriveConfig.
	cfg.Providers.Google.IOLimits = cfg.IOLimits

	if len(errs) > 0 {
		return DriveConfig{}, fmt.Errorf("missing or invalid required drive configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

// parsePositiveInt reads an env var as a positive int, accumulating an error
// in errs when missing or invalid. Returns the parsed value (or 0).
func parsePositiveInt(key string, errs []string) (int, []string) {
	v := os.Getenv(key)
	if v == "" {
		return 0, append(errs, key)
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, append(errs, key+" (must be a positive integer)")
	}
	return n, errs
}

// parsePositiveInt64 reads an env var as a positive int64.
func parsePositiveInt64(key string, errs []string) (int64, []string) {
	v := os.Getenv(key)
	if v == "" {
		return 0, append(errs, key)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return 0, append(errs, key+" (must be a positive integer)")
	}
	return n, errs
}

// parseUnitFloat reads an env var as a float64 in [0, 1].
func parseUnitFloat(key string, errs []string) (float64, []string) {
	v := os.Getenv(key)
	if v == "" {
		return 0, append(errs, key)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f > 1 {
		return 0, append(errs, key+" (must be a float in [0, 1])")
	}
	return f, errs
}
