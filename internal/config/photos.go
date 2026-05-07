package config

import (
	"fmt"
	"os"
	"strings"
)

type PhotosConfig struct {
	Enabled      bool
	Scan         PhotosScanConfig
	Monitor      PhotosMonitorConfig
	IOLimits     PhotosIOLimitsConfig
	Policy       PhotosPolicyConfig
	Intelligence PhotosIntelligenceConfig
	Providers    PhotosProvidersConfig
}

type PhotosScanConfig struct {
	Parallelism      int
	BatchSize        int
	MaxFileSizeBytes int64
}

type PhotosMonitorConfig struct {
	CursorInvalidationRescanMaxItems int
}

// PhotosIOLimitsConfig holds the SST byte caps applied around
// `io.ReadAll` calls on HTTP boundaries (MIT-040-S-006). Values come
// from the `photos.io_limits.*` SST keys; missing or non-positive
// values are a fail-loud config error.
type PhotosIOLimitsConfig struct {
	ProviderMetadataMaxBytes int64
	PhotoBinaryMaxBytes      int64
	TelegramResponseMaxBytes int64
}

type PhotosPolicyConfig struct {
	LifecycleConfirmationThreshold float64
	DuplicateConfirmationThreshold float64
	RoutingConfidenceThreshold     float64
	SensitivityRevealTTLSeconds    int
	ArchiveActionTokenTTLSeconds   int
	DeleteActionTokenTTLSeconds    int
	TelegramMaxInlineSizeBytes     int64
}

type PhotosIntelligenceConfig struct {
	ClassifyModel           string
	EmbedModel              string
	SensitivityModel        string
	AestheticModel          string
	OCRModel                string
	MaxInflightPerConnector int
}

type PhotosProvidersConfig struct {
	Immich     PhotosImmichProviderConfig
	Photoprism PhotosPhotoprismProviderConfig
}

type PhotosImmichProviderConfig struct {
	Enabled              bool
	BaseURL              string
	APIKey               string
	PollIntervalSeconds  int
	SupportedAPIVersions []string
}

// PhotosPhotoprismProviderConfig is the SST shape for the second
// provider adapter introduced by Spec 040 Scope 5. The fields mirror
// the Immich provider so the same config-generate pipeline applies.
type PhotosPhotoprismProviderConfig struct {
	Enabled              bool
	BaseURL              string
	APIToken             string
	PollIntervalSeconds  int
	SupportedAPIVersions []string
}

func loadPhotosConfig() (PhotosConfig, error) {
	var cfg PhotosConfig
	var errs []string

	cfg.Enabled, errs = requiredBool("PHOTOS_ENABLED", errs)
	cfg.Scan.Parallelism, errs = parsePositiveInt("PHOTOS_SCAN_PARALLELISM", errs)
	cfg.Scan.BatchSize, errs = parsePositiveInt("PHOTOS_SCAN_BATCH_SIZE", errs)
	cfg.Scan.MaxFileSizeBytes, errs = parsePositiveInt64("PHOTOS_SCAN_MAX_FILE_SIZE_BYTES", errs)
	cfg.Monitor.CursorInvalidationRescanMaxItems, errs = parsePositiveInt("PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS", errs)

	cfg.IOLimits.ProviderMetadataMaxBytes, errs = parsePositiveInt64("PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES", errs)
	cfg.IOLimits.PhotoBinaryMaxBytes, errs = parsePositiveInt64("PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES", errs)
	cfg.IOLimits.TelegramResponseMaxBytes, errs = parsePositiveInt64("PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES", errs)

	cfg.Policy.LifecycleConfirmationThreshold, errs = parseUnitFloat("PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD", errs)
	cfg.Policy.DuplicateConfirmationThreshold, errs = parseUnitFloat("PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD", errs)
	cfg.Policy.RoutingConfidenceThreshold, errs = parseUnitFloat("PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD", errs)
	cfg.Policy.SensitivityRevealTTLSeconds, errs = parsePositiveInt("PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS", errs)
	cfg.Policy.ArchiveActionTokenTTLSeconds, errs = parsePositiveInt("PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS", errs)
	cfg.Policy.DeleteActionTokenTTLSeconds, errs = parsePositiveInt("PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS", errs)
	cfg.Policy.TelegramMaxInlineSizeBytes, errs = parsePositiveInt64("PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES", errs)

	cfg.Intelligence.ClassifyModel, errs = requiredNonEmptyString("PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", errs)
	cfg.Intelligence.EmbedModel, errs = requiredNonEmptyString("PHOTOS_INTELLIGENCE_EMBED_MODEL", errs)
	cfg.Intelligence.SensitivityModel, errs = requiredNonEmptyString("PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", errs)
	cfg.Intelligence.AestheticModel, errs = requiredNonEmptyString("PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", errs)
	cfg.Intelligence.OCRModel, errs = requiredNonEmptyString("PHOTOS_INTELLIGENCE_OCR_MODEL", errs)
	cfg.Intelligence.MaxInflightPerConnector, errs = parsePositiveInt("PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR", errs)

	cfg.Providers.Immich, errs = loadImmichPhotosProviderConfig(errs)
	cfg.Providers.Photoprism, errs = loadPhotoprismPhotosProviderConfig(errs)

	if len(errs) > 0 {
		return PhotosConfig{}, fmt.Errorf("missing or invalid required photos configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

func loadImmichPhotosProviderConfig(errs []string) (PhotosImmichProviderConfig, []string) {
	var cfg PhotosImmichProviderConfig
	cfg.Enabled, errs = requiredBool("PHOTOS_PROVIDER_IMMICH_ENABLED", errs)

	baseURL, ok := os.LookupEnv("PHOTOS_PROVIDER_IMMICH_BASE_URL")
	if !ok {
		errs = append(errs, "PHOTOS_PROVIDER_IMMICH_BASE_URL")
	} else {
		cfg.BaseURL = baseURL
	}
	apiKey, ok := os.LookupEnv("PHOTOS_PROVIDER_IMMICH_API_KEY")
	if !ok {
		errs = append(errs, "PHOTOS_PROVIDER_IMMICH_API_KEY")
	} else {
		cfg.APIKey = apiKey
	}
	cfg.PollIntervalSeconds, errs = parsePositiveInt("PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS", errs)
	cfg.SupportedAPIVersions, errs = requiredStringList("PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS", errs)

	if cfg.Enabled {
		if strings.TrimSpace(cfg.BaseURL) == "" {
			errs = append(errs, "PHOTOS_PROVIDER_IMMICH_BASE_URL (required when provider is enabled)")
		} else if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
			errs = append(errs, "PHOTOS_PROVIDER_IMMICH_BASE_URL (must be an absolute http(s) URL)")
		}
		if strings.TrimSpace(cfg.APIKey) == "" {
			errs = append(errs, "PHOTOS_PROVIDER_IMMICH_API_KEY (required when provider is enabled)")
		}
	}
	return cfg, errs
}

// loadPhotoprismPhotosProviderConfig validates the Spec 040 Scope 5
// PhotoPrism provider config. The same SST contract applies as for
// Immich: every required env var must be present (even when the
// provider is disabled), and enabling the provider with empty
// secrets is a fail-loud error (zero hardcoded fallbacks).
func loadPhotoprismPhotosProviderConfig(errs []string) (PhotosPhotoprismProviderConfig, []string) {
	var cfg PhotosPhotoprismProviderConfig
	cfg.Enabled, errs = requiredBool("PHOTOS_PROVIDER_PHOTOPRISM_ENABLED", errs)

	baseURL, ok := os.LookupEnv("PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL")
	if !ok {
		errs = append(errs, "PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL")
	} else {
		cfg.BaseURL = baseURL
	}
	apiToken, ok := os.LookupEnv("PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN")
	if !ok {
		errs = append(errs, "PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN")
	} else {
		cfg.APIToken = apiToken
	}
	cfg.PollIntervalSeconds, errs = parsePositiveInt("PHOTOS_PROVIDER_PHOTOPRISM_POLL_INTERVAL_SECONDS", errs)
	cfg.SupportedAPIVersions, errs = requiredStringList("PHOTOS_PROVIDER_PHOTOPRISM_SUPPORTED_API_VERSIONS", errs)

	if cfg.Enabled {
		if strings.TrimSpace(cfg.BaseURL) == "" {
			errs = append(errs, "PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL (required when provider is enabled)")
		} else if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
			errs = append(errs, "PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL (must be an absolute http(s) URL)")
		}
		if strings.TrimSpace(cfg.APIToken) == "" {
			errs = append(errs, "PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN (required when provider is enabled)")
		}
	}
	return cfg, errs
}
