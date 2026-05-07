package config

import (
	"os"
	"strings"
	"testing"
)

var photosSSTKeys = []string{
	"PHOTOS_ENABLED",
	"PHOTOS_SCAN_PARALLELISM",
	"PHOTOS_SCAN_BATCH_SIZE",
	"PHOTOS_SCAN_MAX_FILE_SIZE_BYTES",
	"PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS",
	"PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES",
	"PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES",
	"PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES",
	"PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD",
	"PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD",
	"PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD",
	"PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS",
	"PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS",
	"PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS",
	"PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES",
	"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL",
	"PHOTOS_INTELLIGENCE_EMBED_MODEL",
	"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
	"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL",
	"PHOTOS_INTELLIGENCE_OCR_MODEL",
	"PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR",
	"PHOTOS_PROVIDER_IMMICH_ENABLED",
	"PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS",
	"PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS",
}

var photosProviderSecretPresenceKeys = []string{
	"PHOTOS_PROVIDER_IMMICH_BASE_URL",
	"PHOTOS_PROVIDER_IMMICH_API_KEY",
}

func TestPhotosConfigValidationRequiresEverySSTField(t *testing.T) {
	for _, key := range photosSSTKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error must name %s, got: %v", key, err)
			}
		})
	}
}

func TestPhotosConfigProviderSecretEnvMustExist(t *testing.T) {
	for _, key := range photosProviderSecretPresenceKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is absent", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error must name %s, got: %v", key, err)
			}
		})
	}
}

func TestPhotosConfigDisabledProviderAllowsEmptyCredentials(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PHOTOS_PROVIDER_IMMICH_ENABLED", "false")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_BASE_URL", "")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("disabled Immich provider with empty credentials must load: %v", err)
	}
	if !cfg.Photos.Enabled {
		t.Fatal("photos should be enabled in the fixture")
	}
	if cfg.Photos.Providers.Immich.Enabled {
		t.Fatal("Immich should be disabled in the fixture")
	}
}

func TestPhotosConfigEnabledProviderRequiresCredentials(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PHOTOS_PROVIDER_IMMICH_ENABLED", "true")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_BASE_URL", "")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected enabled Immich provider with empty credentials to fail")
	}
	for _, want := range []string{"PHOTOS_PROVIDER_IMMICH_BASE_URL", "PHOTOS_PROVIDER_IMMICH_API_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name %s, got: %v", want, err)
		}
	}
}

func TestPhotosConfigPopulatesEveryField(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := cfg.Photos
	if !p.Enabled {
		t.Fatal("Photos.Enabled = false, want true")
	}
	if p.Scan.Parallelism != 2 || p.Scan.BatchSize != 50 || p.Scan.MaxFileSizeBytes != 52428800 {
		t.Fatalf("Photos.Scan = %+v", p.Scan)
	}
	if p.Policy.LifecycleConfirmationThreshold != 0.8 || p.Policy.DuplicateConfirmationThreshold != 0.92 || p.Policy.RoutingConfidenceThreshold != 0.75 {
		t.Fatalf("Photos.Policy thresholds = %+v", p.Policy)
	}
	if p.Policy.SensitivityRevealTTLSeconds != 600 || p.Policy.ArchiveActionTokenTTLSeconds != 86400 || p.Policy.DeleteActionTokenTTLSeconds != 86400 {
		t.Fatalf("Photos.Policy TTLs = %+v", p.Policy)
	}
	if p.Intelligence.ClassifyModel == "" || p.Intelligence.EmbedModel == "" || p.Intelligence.OCRModel == "" {
		t.Fatalf("Photos.Intelligence missing model fields: %+v", p.Intelligence)
	}
	if p.Intelligence.MaxInflightPerConnector != 2 {
		t.Fatalf("MaxInflightPerConnector = %d, want 2", p.Intelligence.MaxInflightPerConnector)
	}
	if p.Providers.Immich.PollIntervalSeconds != 300 {
		t.Fatalf("Immich poll interval = %d, want 300", p.Providers.Immich.PollIntervalSeconds)
	}
	if len(p.Providers.Immich.SupportedAPIVersions) == 0 {
		t.Fatal("Immich supported API versions must be populated")
	}
}
