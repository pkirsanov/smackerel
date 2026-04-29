package config

import (
	"os"
	"strings"
	"testing"
)

var recommendationSSTKeys = []string{
	"RECOMMENDATIONS_ENABLED",
	"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED",
	"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_CATEGORIES",
	"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_QUOTA_WINDOW_SECONDS",
	"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_MAX_REQUESTS_PER_WINDOW",
	"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ATTRIBUTION_LABEL",
	"RECOMMENDATIONS_PROVIDER_YELP_ENABLED",
	"RECOMMENDATIONS_PROVIDER_YELP_CATEGORIES",
	"RECOMMENDATIONS_PROVIDER_YELP_QUOTA_WINDOW_SECONDS",
	"RECOMMENDATIONS_PROVIDER_YELP_MAX_REQUESTS_PER_WINDOW",
	"RECOMMENDATIONS_PROVIDER_YELP_ATTRIBUTION_LABEL",
	"RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD",
	"RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD",
	"RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD",
	"RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM",
	"RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL",
	"RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW",
	"RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS",
	"RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND",
	"RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY",
	"RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS",
	"RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS",
	"RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER",
	"RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS",
	"RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT",
	"RECOMMENDATIONS_RANKING_STANDARD_STYLE",
	"RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD",
	"RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED",
	"RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES",
	"RECOMMENDATIONS_POLICY_SAFETY_SOURCES",
	"RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED",
	"RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED",
	"RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED",
}

func TestRecommendationConfig_MissingRequiredKeyFailsLoudly(t *testing.T) {
	for _, key := range recommendationSSTKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is empty", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error must name %s, got: %v", key, err)
			}
		})
	}
}

func TestRecommendationConfig_MissingProviderAPIKeyEnvFailsLoudly(t *testing.T) {
	for _, key := range []string{
		"RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY",
		"RECOMMENDATIONS_PROVIDER_YELP_API_KEY",
	} {
		key := key
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is not present", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error must name %s, got: %v", key, err)
			}
		})
	}
}

func TestRecommendationConfig_DisabledProvidersAllowEmptyAPIKeys(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY", "")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_API_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("disabled recommendation providers with empty API keys must load: %v", err)
	}
	if !cfg.Recommendations.Enabled {
		t.Fatal("recommendations should be enabled in the fixture")
	}
	if cfg.Recommendations.Providers.GooglePlaces.Enabled || cfg.Recommendations.Providers.Yelp.Enabled {
		t.Fatal("provider fixtures should be disabled by default")
	}
}

func TestRecommendationConfig_EnabledProviderRequiresAPIKey(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED", "true")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected enabled provider with empty API key to fail")
	}
	if !strings.Contains(err.Error(), "RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY") {
		t.Fatalf("error must name provider API key, got: %v", err)
	}
}
