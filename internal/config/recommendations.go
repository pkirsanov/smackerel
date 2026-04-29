package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RecommendationsConfig captures the spec-039 recommendations SST block.
type RecommendationsConfig struct {
	Enabled           bool
	Providers         RecommendationProvidersConfig
	LocationPrecision RecommendationLocationPrecisionConfig
	Watches           RecommendationWatchesConfig
	Retention         RecommendationRetentionConfig
	Ranking           RecommendationRankingConfig
	Policy            RecommendationPolicyConfig
	Delivery          RecommendationDeliveryConfig
}

type RecommendationProvidersConfig struct {
	GooglePlaces RecommendationProviderConfig
	Yelp         RecommendationProviderConfig
}

type RecommendationProviderConfig struct {
	Enabled              bool
	Categories           []string
	APIKey               string
	QuotaWindowSeconds   int
	MaxRequestsPerWindow int
	AttributionLabel     string
}

type RecommendationLocationPrecisionConfig struct {
	UserStandard           string
	MobileStandard         string
	WatchStandard          string
	NeighborhoodCellSystem string
	NeighborhoodCellLevel  int
}

type RecommendationWatchesConfig struct {
	MaxAlertsPerWindow    int
	AlertWindowSeconds    int
	CooldownSecondsByKind map[string]int
	QuietHoursPolicy      map[string]any
}

type RecommendationRetentionConfig struct {
	RawProviderPayloadSeconds int
	TraceRetentionSeconds     int
}

type RecommendationRankingConfig struct {
	MaxCandidatesPerProvider int
	MaxFinalResults          int
	StandardResultCount      int
	StandardStyle            string
	LowConfidenceThreshold   float64
}

type RecommendationPolicyConfig struct {
	SponsoredPromotionsEnabled bool
	RestrictedCategories       []string
	SafetySources              []string
}

type RecommendationDeliveryConfig struct {
	TelegramEnabled    bool
	DigestEnabled      bool
	TripDossierEnabled bool
}

var validRecommendationPrecision = map[string]struct{}{
	"exact":        {},
	"neighborhood": {},
	"city":         {},
}

var validRecommendationStyles = map[string]struct{}{
	"familiar": {},
	"novel":    {},
	"balanced": {},
}

func loadRecommendationsConfig() (RecommendationsConfig, error) {
	var cfg RecommendationsConfig
	var errs []string

	cfg.Enabled, errs = requiredBool("RECOMMENDATIONS_ENABLED", errs)

	cfg.Providers.GooglePlaces, errs = loadRecommendationProviderConfig("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES", errs)
	cfg.Providers.Yelp, errs = loadRecommendationProviderConfig("RECOMMENDATIONS_PROVIDER_YELP", errs)

	cfg.LocationPrecision.UserStandard, errs = requiredEnum("RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD", validRecommendationPrecision, "exact|neighborhood|city", errs)
	cfg.LocationPrecision.MobileStandard, errs = requiredEnum("RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD", validRecommendationPrecision, "exact|neighborhood|city", errs)
	cfg.LocationPrecision.WatchStandard, errs = requiredEnum("RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD", validRecommendationPrecision, "exact|neighborhood|city", errs)
	cfg.LocationPrecision.NeighborhoodCellSystem, errs = requiredNonEmptyString("RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM", errs)
	cfg.LocationPrecision.NeighborhoodCellLevel, errs = parsePositiveInt("RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL", errs)

	cfg.Watches.MaxAlertsPerWindow, errs = parsePositiveInt("RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW", errs)
	cfg.Watches.AlertWindowSeconds, errs = parsePositiveInt("RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS", errs)
	cfg.Watches.CooldownSecondsByKind, errs = requiredIntMap("RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND", errs)
	cfg.Watches.QuietHoursPolicy, errs = requiredObject("RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY", errs)

	cfg.Retention.RawProviderPayloadSeconds, errs = parsePositiveInt("RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS", errs)
	cfg.Retention.TraceRetentionSeconds, errs = parsePositiveInt("RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS", errs)

	cfg.Ranking.MaxCandidatesPerProvider, errs = parsePositiveInt("RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER", errs)
	cfg.Ranking.MaxFinalResults, errs = parsePositiveInt("RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS", errs)
	cfg.Ranking.StandardResultCount, errs = parsePositiveInt("RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT", errs)
	if cfg.Ranking.StandardResultCount > 10 {
		errs = append(errs, "RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT (must be between 1 and 10)")
	}
	cfg.Ranking.StandardStyle, errs = requiredEnum("RECOMMENDATIONS_RANKING_STANDARD_STYLE", validRecommendationStyles, "familiar|novel|balanced", errs)
	cfg.Ranking.LowConfidenceThreshold, errs = parseUnitFloat("RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD", errs)

	cfg.Policy.SponsoredPromotionsEnabled, errs = requiredBool("RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED", errs)
	cfg.Policy.RestrictedCategories, errs = requiredStringList("RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES", errs)
	cfg.Policy.SafetySources, errs = requiredStringList("RECOMMENDATIONS_POLICY_SAFETY_SOURCES", errs)

	cfg.Delivery.TelegramEnabled, errs = requiredBool("RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED", errs)
	cfg.Delivery.DigestEnabled, errs = requiredBool("RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED", errs)
	cfg.Delivery.TripDossierEnabled, errs = requiredBool("RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED", errs)

	if len(errs) > 0 {
		return RecommendationsConfig{}, fmt.Errorf("missing or invalid required recommendation configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

func loadRecommendationProviderConfig(prefix string, errs []string) (RecommendationProviderConfig, []string) {
	var cfg RecommendationProviderConfig
	cfg.Enabled, errs = requiredBool(prefix+"_ENABLED", errs)
	cfg.Categories, errs = requiredStringList(prefix+"_CATEGORIES", errs)
	apiKey, ok := os.LookupEnv(prefix + "_API_KEY")
	if !ok {
		errs = append(errs, prefix+"_API_KEY")
	} else {
		cfg.APIKey = apiKey
	}
	cfg.QuotaWindowSeconds, errs = parsePositiveInt(prefix+"_QUOTA_WINDOW_SECONDS", errs)
	cfg.MaxRequestsPerWindow, errs = parsePositiveInt(prefix+"_MAX_REQUESTS_PER_WINDOW", errs)
	cfg.AttributionLabel, errs = requiredNonEmptyString(prefix+"_ATTRIBUTION_LABEL", errs)
	if cfg.Enabled && cfg.APIKey == "" {
		errs = append(errs, prefix+"_API_KEY (required when provider is enabled)")
	}
	return cfg, errs
}

func requiredBool(key string, errs []string) (bool, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return false, append(errs, key)
	}
	if v != "true" && v != "false" {
		return false, append(errs, key+" (must be true or false)")
	}
	return v == "true", errs
}

func requiredNonEmptyString(key string, errs []string) (string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", append(errs, key)
	}
	return v, errs
}

func requiredEnum(key string, allowed map[string]struct{}, label string, errs []string) (string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", append(errs, key)
	}
	if _, ok := allowed[v]; !ok {
		return "", append(errs, key+" (must be one of "+label+")")
	}
	return v, errs
}

func requiredStringList(key string, errs []string) ([]string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" || v == "[]" {
		return nil, append(errs, key+" (must be a non-empty JSON list)")
	}
	var values []string
	if err := json.Unmarshal([]byte(v), &values); err != nil {
		return nil, append(errs, key+" (invalid JSON list)")
	}
	if len(values) == 0 {
		return nil, append(errs, key+" (must be a non-empty JSON list)")
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, append(errs, key+" (must not contain empty values)")
		}
	}
	return values, errs
}

func requiredIntMap(key string, errs []string) (map[string]int, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" || v == "{}" {
		return nil, append(errs, key+" (must be a non-empty JSON object)")
	}
	var values map[string]int
	if err := json.Unmarshal([]byte(v), &values); err != nil {
		return nil, append(errs, key+" (invalid JSON object)")
	}
	if len(values) == 0 {
		return nil, append(errs, key+" (must be a non-empty JSON object)")
	}
	for name, value := range values {
		if strings.TrimSpace(name) == "" || value < 0 {
			return nil, append(errs, key+" (keys must be non-empty and values non-negative)")
		}
	}
	return values, errs
}

func requiredObject(key string, errs []string) (map[string]any, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" || v == "{}" {
		return nil, append(errs, key+" (must be a non-empty JSON object)")
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(v), &values); err != nil {
		return nil, append(errs, key+" (invalid JSON object)")
	}
	if len(values) == 0 {
		return nil, append(errs, key+" (must be a non-empty JSON object)")
	}
	return values, errs
}
