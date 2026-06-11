// Package config — Spec 083 Card Rewards Companion (Scope 01) SST surface.
//
// All card-rewards tunables originate from config/smackerel.yaml (FR-CR-020,
// Gate G028, smackerel-no-defaults). When the feature is ENABLED, every
// required key MUST be present and valid or LoadCardRewardsConfig fails loud —
// there are NO in-source defaults and NO ${VAR:-default} fallbacks. When the
// feature is DISABLED (or the enable flag is unset), no extraction/source
// fields are required and the loader returns a zero-value disabled config so
// the service starts normally (SCN-083-A07).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CardRewardsSource is one configured rotating-category source page
// (Doctor of Credit, an issuer page, …). Provenance is preserved on every
// extraction (Principle 4).
type CardRewardsSource struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	IssuerHint string `json:"issuer_hint"`
}

// CardRewardsExtractionConfig holds the LLM-extraction tunables. The model
// call itself lives in the Python ML sidecar (Constitution C2); these values
// flow to the sidecar/orchestrator at runtime.
type CardRewardsExtractionConfig struct {
	// Model is the host Ollama model name (e.g. "gpt-oss:20b"). REQUIRED when enabled.
	Model string
	// Endpoint is the host Ollama URL (e.g. the home-lab host endpoint). REQUIRED when enabled.
	Endpoint string
	// ConfidenceThreshold below which a reconciled record is flagged
	// needs_verification. REQUIRED when enabled; MUST be in [0,1].
	ConfidenceThreshold float64
	// MaxSourcesPerCard caps cross-source reconciliation fan-out. REQUIRED when enabled; MUST be > 0.
	MaxSourcesPerCard int
}

// CardRewardsConfig is the SST surface for the card-rewards feature.
type CardRewardsConfig struct {
	// Enabled gates the whole feature. When false, no other field is required.
	Enabled bool
	// ImportDataDir is the path to the CCManager `data/` directory consumed by
	// the one-time JSON→PostgreSQL importer (Scope 03). It is OPTIONAL at
	// startup (an empty value never blocks boot) and is read regardless of
	// Enabled; the importer fails loud when it is invoked with no resolved
	// directory (CLI --data-dir flag overrides this value). It is never a
	// committed real path — the operator supplies their environment's CCManager
	// location (the SST placeholder is empty).
	ImportDataDir string
	// ScrapeCron is the daily refresh cron. REQUIRED when enabled.
	ScrapeCron string
	// MonthlyRecommendCron is the monthly recommendation cron. REQUIRED when enabled.
	MonthlyRecommendCron string
	// CalendarSync opts into CalDAV delivery.
	CalendarSync bool
	// CalendarUIDPrefix is the stable CalDAV UID prefix. REQUIRED when CalendarSync.
	CalendarUIDPrefix string
	// FetchTimeoutSeconds bounds per-source fetches. REQUIRED when enabled; MUST be > 0.
	FetchTimeoutSeconds int
	// Extraction holds the LLM-extraction tunables. REQUIRED when enabled.
	Extraction CardRewardsExtractionConfig
	// Sources is the non-empty list of configured source pages. REQUIRED non-empty when enabled.
	Sources []CardRewardsSource
	// TrackedCategories is the non-empty list of spend categories to recommend
	// for. REQUIRED non-empty when enabled.
	TrackedCategories []string
}

// LoadCardRewardsConfig reads every CARD_REWARDS_* env var and returns a
// populated, validated config. When CARD_REWARDS_ENABLED is not "true" the
// feature is disabled and an empty config is returned with no error
// (SCN-083-A07). When enabled, missing/invalid required values are a fail-loud
// error naming each offending variable (SCN-083-A03, A04).
func LoadCardRewardsConfig() (CardRewardsConfig, error) {
	var cfg CardRewardsConfig

	cfg.Enabled = os.Getenv("CARD_REWARDS_ENABLED") == "true"
	// ImportDataDir is read regardless of Enabled: the one-time importer (Scope
	// 03) may run against a disabled feature, and it is invocation-gated (the
	// importer fails loud if neither this value nor --data-dir is set) rather
	// than startup-gated, so it is never part of the fail-loud required set.
	cfg.ImportDataDir = strings.TrimSpace(os.Getenv("CARD_REWARDS_IMPORT_DIR"))
	if !cfg.Enabled {
		// Disabled: no extraction/source fields required (SCN-083-A07).
		return cfg, nil
	}

	var errs []string

	readString := func(key string, dst *string) {
		v := os.Getenv(key)
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (required when card_rewards enabled)")
			return
		}
		*dst = v
	}
	readCron := func(key string, dst *string) {
		v := os.Getenv(key)
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (required when card_rewards enabled)")
			return
		}
		if !isValidCronExpr(v) {
			errs = append(errs, fmt.Sprintf("%s (not a valid cron expression, got %q)", key, v))
			return
		}
		*dst = v
	}
	readPositiveInt := func(key string, dst *int) {
		v := os.Getenv(key)
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (required when card_rewards enabled)")
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n <= 0 {
			errs = append(errs, fmt.Sprintf("%s (must be > 0, got %d)", key, n))
			return
		}
		*dst = n
	}

	readCron("CARD_REWARDS_SCRAPE_CRON", &cfg.ScrapeCron)
	readCron("CARD_REWARDS_MONTHLY_RECOMMEND_CRON", &cfg.MonthlyRecommendCron)
	readPositiveInt("CARD_REWARDS_FETCH_TIMEOUT_SECONDS", &cfg.FetchTimeoutSeconds)

	cfg.CalendarSync = os.Getenv("CARD_REWARDS_CALENDAR_SYNC") == "true"
	if cfg.CalendarSync {
		readString("CARD_REWARDS_CALENDAR_UID_PREFIX", &cfg.CalendarUIDPrefix)
	} else {
		// Optional when sync is off; carry whatever was provided.
		cfg.CalendarUIDPrefix = os.Getenv("CARD_REWARDS_CALENDAR_UID_PREFIX")
	}

	// Extraction sub-config.
	readString("CARD_REWARDS_EXTRACTION_MODEL", &cfg.Extraction.Model)
	readString("CARD_REWARDS_EXTRACTION_ENDPOINT", &cfg.Extraction.Endpoint)
	readPositiveInt("CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD", &cfg.Extraction.MaxSourcesPerCard)
	if v := os.Getenv("CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD"); strings.TrimSpace(v) == "" {
		errs = append(errs, "CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD (required when card_rewards enabled)")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil {
		errs = append(errs, fmt.Sprintf("CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD (must be a number, got %q)", v))
	} else if f < 0 || f > 1 {
		errs = append(errs, fmt.Sprintf("CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD (must be in [0,1], got %v)", f))
	} else {
		cfg.Extraction.ConfidenceThreshold = f
	}

	// Sources (JSON array of {name,url,issuer_hint}); REQUIRED non-empty.
	sources, srcErr := parseCardRewardsSources(os.Getenv("CARD_REWARDS_SOURCES"))
	if srcErr != "" {
		errs = append(errs, srcErr)
	} else {
		cfg.Sources = sources
	}

	// Tracked categories (JSON array or YAML bracket/CSV); REQUIRED non-empty.
	cats := parseCardRewardsStringList(os.Getenv("CARD_REWARDS_TRACKED_CATEGORIES"))
	if len(cats) == 0 {
		errs = append(errs, "CARD_REWARDS_TRACKED_CATEGORIES (required non-empty when card_rewards enabled)")
	} else {
		cfg.TrackedCategories = cats
	}

	if len(errs) > 0 {
		return CardRewardsConfig{}, fmt.Errorf("missing or invalid required card_rewards configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

// parseCardRewardsSources parses the CARD_REWARDS_SOURCES JSON array. Returns
// the parsed sources and an empty error string on success, or a non-empty
// error string describing the failure. An empty/whitespace value, a non-array,
// an empty array, or any entry missing name/url is rejected (SCN-083-A04).
func parseCardRewardsSources(raw string) ([]CardRewardsSource, string) {
	if strings.TrimSpace(raw) == "" {
		return nil, "CARD_REWARDS_SOURCES (required non-empty when card_rewards enabled)"
	}
	var sources []CardRewardsSource
	if err := json.Unmarshal([]byte(raw), &sources); err != nil {
		return nil, fmt.Sprintf("CARD_REWARDS_SOURCES (must be a JSON array of {name,url,issuer_hint}, got %q)", raw)
	}
	if len(sources) == 0 {
		return nil, "CARD_REWARDS_SOURCES (required non-empty when card_rewards enabled)"
	}
	for i, s := range sources {
		if strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.URL) == "" {
			return nil, fmt.Sprintf("CARD_REWARDS_SOURCES[%d] (each source requires non-empty name and url)", i)
		}
	}
	return sources, ""
}

// parseCardRewardsStringList parses a list env var that may be a JSON array of
// strings (preferred, emitted by yaml_get_json) or a YAML bracket / CSV form
// (e.g. `[ "Dining", "Groceries" ]`), mirroring how MealPlanMealTypes is
// parsed. Returns a slice of trimmed, non-empty entries.
func parseCardRewardsStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Try strict JSON array first.
	var jsonList []string
	if err := json.Unmarshal([]byte(raw), &jsonList); err == nil {
		var out []string
		for _, s := range jsonList {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	// Fall back to YAML bracket / CSV form.
	cleaned := strings.Trim(raw, "[] ")
	var out []string
	for _, t := range strings.Split(cleaned, ",") {
		t = strings.Trim(strings.TrimSpace(t), "\"'")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
