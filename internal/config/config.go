package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ExpenseCategory defines an expense classification category.
type ExpenseCategory struct {
	Slug        string `json:"slug"`
	Display     string `json:"display"`
	TaxCategory string `json:"tax_category"`
}

// Config holds all configuration values for smackerel-core.
type Config struct {
	DatabaseURL      string
	NATSURL          string
	LLMProvider      string
	LLMModel         string
	LLMAPIKey        string
	AuthToken        string
	TelegramBotToken string
	TelegramChatIDs  []string
	OllamaURL        string
	OllamaModel      string
	EmbeddingModel   string
	DigestCron       string
	LogLevel         string
	Port             string
	MLSidecarURL     string
	CoreAPIURL       string

	// DB pool sizing (SST-compliant — from smackerel.yaml via config generate)
	DBMaxConns int32
	DBMinConns int32

	// Shutdown timeout in seconds for graceful shutdown (SST-compliant)
	ShutdownTimeoutS int

	// ML sidecar health cache TTL in seconds (SST-compliant)
	MLHealthCacheTTLS int

	// ML sidecar readiness timeout in seconds (SST-compliant)
	// Core blocks at startup until ML sidecar is healthy or timeout elapses.
	MLReadinessTimeoutS int

	// Optional connector path fields (SST-compliant — read from env, sourced from smackerel.yaml)
	BookmarksImportDir        string
	BookmarksEnabled          bool
	BookmarksSyncSchedule     string
	BookmarksWatchInterval    string
	BookmarksArchiveProcessed bool
	BookmarksProcessingTier   string
	BookmarksMinURLLength     int
	BookmarksExcludeDomains   string
	BrowserHistoryPath        string
	MapsImportDir             string

	// Telegram assembly config (SST-compliant — from smackerel.yaml via config generate)
	TelegramAssemblyWindowSeconds        int
	TelegramAssemblyMaxMessages          int
	TelegramMediaGroupWindowSeconds      int
	TelegramDisambiguationTimeoutSeconds int

	// Knowledge layer config (SST-compliant — from smackerel.yaml via config generate)
	KnowledgeEnabled                        bool
	KnowledgeSynthesisTimeoutSeconds        int
	KnowledgeLintCron                       string
	KnowledgeLintStaleDays                  int
	KnowledgeConceptMaxTokens               int
	KnowledgeConceptSearchThreshold         float64
	KnowledgeCrossSourceConfidenceThreshold float64
	KnowledgeMaxSynthesisRetries            int
	KnowledgePromptContractIngestSynthesis  string
	KnowledgePromptContractCrossSource      string
	KnowledgePromptContractLintAudit        string
	KnowledgePromptContractQueryAugment     string
	KnowledgePromptContractDigestAssembly   string

	// Prompt contracts directory (SST-compliant — from smackerel.yaml via config generate)
	PromptContractsDir string

	// Observability config (SST-compliant — from smackerel.yaml via config generate)
	OTELEnabled          bool
	OTELExporterEndpoint string

	// Expense tracking config (SST-compliant — from smackerel.yaml via config generate)
	ExpensesEnabled                       bool
	ExpensesDefaultCurrency               string
	ExpensesExportMaxRows                 int
	ExpensesExportQBDateFormat            string
	ExpensesExportStdDateFormat           string
	ExpensesSuggestionsMinConfidence      float64
	ExpensesSuggestionsMinPastBusiness    int
	ExpensesSuggestionsMaxPerDigest       int
	ExpensesSuggestionsReclassifyBatchLim int
	ExpensesVendorCacheSize               int
	ExpensesDigestMaxWords                int
	ExpensesDigestNeedsReviewLimit        int
	ExpensesDigestMissingReceiptLookback  int
	IMAPExpenseLabels                     map[string]string
	ExpensesBusinessVendors               []string
	ExpensesCategories                    []ExpenseCategory

	// Telegram cook session config (SST-compliant — from smackerel.yaml via config generate)
	TelegramCookSessionTimeoutMinutes int
	TelegramCookSessionMaxPerChat     int

	// Meal planning config (SST-compliant — from smackerel.yaml via config generate)
	MealPlanEnabled          bool
	MealPlanDefaultServings  int
	MealPlanMealTypes        []string
	MealPlanMealTimes        map[string]string
	MealPlanCalendarSync     bool
	MealPlanAutoComplete     bool
	MealPlanAutoCompleteCron string

	// Connector enable/credential/schedule fields (SST-compliant — from smackerel.yaml via config generate)
	MapsSyncSchedule              string
	MapsWatchInterval             string
	MapsArchiveProcessed          bool
	MapsHomeDetection             string
	MapsCommuteWeekdaysOnly       bool
	MapsMinDistanceM              float64
	MapsMinDurationMin            float64
	MapsLocationRadiusM           float64
	MapsCommuteMinOccurrences     float64
	MapsCommuteWindowDays         float64
	MapsTripMinDistanceKm         float64
	MapsTripMinOvernightHours     float64
	MapsLinkTimeExtendMin         float64
	MapsLinkProximityRadiusM      float64
	DiscordEnabled                bool
	DiscordBotToken               string
	DiscordSyncSchedule           string
	DiscordEnableGateway          bool
	DiscordBackfillLimit          float64
	DiscordIncludeThreads         bool
	DiscordIncludePins            bool
	DiscordCaptureCommands        []interface{}
	DiscordMonitoredChannels      []interface{}
	TwitterEnabled                bool
	TwitterBearerToken            string
	TwitterSyncSchedule           string
	TwitterSyncMode               string
	TwitterArchiveDir             string
	WeatherEnabled                bool
	WeatherSyncSchedule           string
	WeatherLocations              []interface{}
	WeatherEnableAlerts           bool
	WeatherForecastDays           float64
	WeatherPrecision              float64
	GovAlertsEnabled              bool
	GovAlertsSyncSchedule         string
	GovAlertsAirnowAPIKey         string
	GovAlertsLocations            []interface{}
	GovAlertsMinEarthquakeMag     float64
	GovAlertsTravelLocations      []interface{}
	GovAlertsSourceEarthquake     bool
	GovAlertsSourceWeather        bool
	GovAlertsSourceTsunami        bool
	GovAlertsSourceVolcano        bool
	GovAlertsSourceWildfire       bool
	GovAlertsSourceAirnow         bool
	GovAlertsSourceGdacs          bool
	FinancialMarketsEnabled       bool
	FinancialMarketsSyncSchedule  string
	FinancialMarketsFinnhubAPIKey string
	FinancialMarketsFredAPIKey    string
	FinancialMarketsWatchlist     map[string]interface{}
	FinancialMarketsAlertThresh   float64
	FinancialMarketsCoingecko     bool
	FinancialMarketsFredEnabled   bool
	FinancialMarketsFredSeries    []interface{}
	IMAPSyncSchedule              string
	CalDAVSyncSchedule            string
	YouTubeSyncSchedule           string

	// CORS allowed origins (SST-compliant — from smackerel.yaml via config generate)
	CORSAllowedOrigins []string
}

// Load reads configuration from environment variables.
// It returns an error naming every missing required variable.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		NATSURL:          os.Getenv("NATS_URL"),
		LLMProvider:      os.Getenv("LLM_PROVIDER"),
		LLMModel:         os.Getenv("LLM_MODEL"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		AuthToken:        os.Getenv("SMACKEREL_AUTH_TOKEN"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		OllamaURL:        os.Getenv("OLLAMA_URL"),
		OllamaModel:      os.Getenv("OLLAMA_MODEL"),
		EmbeddingModel:   os.Getenv("EMBEDDING_MODEL"),
		DigestCron:       os.Getenv("DIGEST_CRON"),
		LogLevel:         os.Getenv("LOG_LEVEL"),
		Port:             os.Getenv("PORT"),
		MLSidecarURL:     os.Getenv("ML_SIDECAR_URL"),
		CoreAPIURL:       os.Getenv("CORE_API_URL"),

		BookmarksImportDir:        os.Getenv("BOOKMARKS_IMPORT_DIR"),
		BookmarksEnabled:          os.Getenv("BOOKMARKS_ENABLED") == "true",
		BookmarksSyncSchedule:     os.Getenv("BOOKMARKS_SYNC_SCHEDULE"),
		BookmarksWatchInterval:    os.Getenv("BOOKMARKS_WATCH_INTERVAL"),
		BookmarksArchiveProcessed: os.Getenv("BOOKMARKS_ARCHIVE_PROCESSED") == "true",
		BookmarksProcessingTier:   os.Getenv("BOOKMARKS_PROCESSING_TIER"),
		BookmarksMinURLLength:     parseIntEnv("BOOKMARKS_MIN_URL_LENGTH", 0),
		BookmarksExcludeDomains:   os.Getenv("BOOKMARKS_EXCLUDE_DOMAINS"),
		BrowserHistoryPath:        os.Getenv("BROWSER_HISTORY_PATH"),
		MapsImportDir:             os.Getenv("MAPS_IMPORT_DIR"),

		// Connector enable/credential/schedule (SST-compliant)
		MapsSyncSchedule:              os.Getenv("MAPS_SYNC_SCHEDULE"),
		MapsWatchInterval:             os.Getenv("MAPS_WATCH_INTERVAL"),
		MapsArchiveProcessed:          os.Getenv("MAPS_ARCHIVE_PROCESSED") == "true",
		MapsHomeDetection:             os.Getenv("MAPS_HOME_DETECTION"),
		MapsCommuteWeekdaysOnly:       os.Getenv("MAPS_COMMUTE_WEEKDAYS_ONLY") == "true",
		MapsMinDistanceM:              parseEnvFloat("MAPS_MIN_DISTANCE_M"),
		MapsMinDurationMin:            parseEnvFloat("MAPS_MIN_DURATION_MIN"),
		MapsLocationRadiusM:           parseEnvFloat("MAPS_LOCATION_RADIUS_M"),
		MapsCommuteMinOccurrences:     parseEnvFloat("MAPS_COMMUTE_MIN_OCCURRENCES"),
		MapsCommuteWindowDays:         parseEnvFloat("MAPS_COMMUTE_WINDOW_DAYS"),
		MapsTripMinDistanceKm:         parseEnvFloat("MAPS_TRIP_MIN_DISTANCE_KM"),
		MapsTripMinOvernightHours:     parseEnvFloat("MAPS_TRIP_MIN_OVERNIGHT_HOURS"),
		MapsLinkTimeExtendMin:         parseEnvFloat("MAPS_LINK_TIME_EXTEND_MIN"),
		MapsLinkProximityRadiusM:      parseEnvFloat("MAPS_LINK_PROXIMITY_RADIUS_M"),
		DiscordEnabled:                os.Getenv("DISCORD_ENABLED") == "true",
		DiscordBotToken:               os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordSyncSchedule:           os.Getenv("DISCORD_SYNC_SCHEDULE"),
		DiscordEnableGateway:          os.Getenv("DISCORD_ENABLE_GATEWAY") == "true",
		DiscordBackfillLimit:          parseEnvFloat("DISCORD_BACKFILL_LIMIT"),
		DiscordIncludeThreads:         os.Getenv("DISCORD_INCLUDE_THREADS") == "true",
		DiscordIncludePins:            os.Getenv("DISCORD_INCLUDE_PINS") == "true",
		DiscordCaptureCommands:        parseEnvJSONArray("DISCORD_CAPTURE_COMMANDS"),
		DiscordMonitoredChannels:      parseEnvJSONArray("DISCORD_MONITORED_CHANNELS"),
		TwitterEnabled:                os.Getenv("TWITTER_ENABLED") == "true",
		TwitterBearerToken:            os.Getenv("TWITTER_BEARER_TOKEN"),
		TwitterSyncSchedule:           os.Getenv("TWITTER_SYNC_SCHEDULE"),
		TwitterSyncMode:               os.Getenv("TWITTER_SYNC_MODE"),
		TwitterArchiveDir:             os.Getenv("TWITTER_ARCHIVE_DIR"),
		WeatherEnabled:                os.Getenv("WEATHER_ENABLED") == "true",
		WeatherSyncSchedule:           os.Getenv("WEATHER_SYNC_SCHEDULE"),
		WeatherLocations:              parseEnvJSONArray("WEATHER_LOCATIONS"),
		WeatherEnableAlerts:           os.Getenv("WEATHER_ENABLE_ALERTS") == "true",
		WeatherForecastDays:           parseEnvFloat("WEATHER_FORECAST_DAYS"),
		WeatherPrecision:              parseEnvFloat("WEATHER_PRECISION"),
		GovAlertsEnabled:              os.Getenv("GOV_ALERTS_ENABLED") == "true",
		GovAlertsSyncSchedule:         os.Getenv("GOV_ALERTS_SYNC_SCHEDULE"),
		GovAlertsAirnowAPIKey:         os.Getenv("GOV_ALERTS_AIRNOW_API_KEY"),
		GovAlertsLocations:            parseEnvJSONArray("GOV_ALERTS_LOCATIONS"),
		GovAlertsMinEarthquakeMag:     parseEnvFloat("GOV_ALERTS_MIN_EARTHQUAKE_MAG"),
		GovAlertsTravelLocations:      parseEnvJSONArray("GOV_ALERTS_TRAVEL_LOCATIONS"),
		GovAlertsSourceEarthquake:     os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true",
		GovAlertsSourceWeather:        os.Getenv("GOV_ALERTS_SOURCE_WEATHER") == "true",
		GovAlertsSourceTsunami:        os.Getenv("GOV_ALERTS_SOURCE_TSUNAMI") == "true",
		GovAlertsSourceVolcano:        os.Getenv("GOV_ALERTS_SOURCE_VOLCANO") == "true",
		GovAlertsSourceWildfire:       os.Getenv("GOV_ALERTS_SOURCE_WILDFIRE") == "true",
		GovAlertsSourceAirnow:         os.Getenv("GOV_ALERTS_SOURCE_AIRNOW") == "true",
		GovAlertsSourceGdacs:          os.Getenv("GOV_ALERTS_SOURCE_GDACS") == "true",
		FinancialMarketsEnabled:       os.Getenv("FINANCIAL_MARKETS_ENABLED") == "true",
		FinancialMarketsSyncSchedule:  os.Getenv("FINANCIAL_MARKETS_SYNC_SCHEDULE"),
		FinancialMarketsFinnhubAPIKey: os.Getenv("FINANCIAL_MARKETS_FINNHUB_API_KEY"),
		FinancialMarketsFredAPIKey:    os.Getenv("FINANCIAL_MARKETS_FRED_API_KEY"),
		FinancialMarketsWatchlist:     parseEnvJSONObject("FINANCIAL_MARKETS_WATCHLIST"),
		FinancialMarketsAlertThresh:   parseEnvFloat("FINANCIAL_MARKETS_ALERT_THRESHOLD"),
		FinancialMarketsCoingecko:     os.Getenv("FINANCIAL_MARKETS_COINGECKO_ENABLED") == "true",
		FinancialMarketsFredEnabled:   os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
		FinancialMarketsFredSeries:    parseEnvJSONArray("FINANCIAL_MARKETS_FRED_SERIES"),
		IMAPSyncSchedule:              os.Getenv("IMAP_SYNC_SCHEDULE"),
		CalDAVSyncSchedule:            os.Getenv("CALDAV_SYNC_SCHEDULE"),
		YouTubeSyncSchedule:           os.Getenv("YOUTUBE_SYNC_SCHEDULE"),
	}

	// Parse CORS allowed origins (comma-separated)
	if corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); corsOrigins != "" {
		for _, o := range strings.Split(corsOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
			}
		}
	}

	if chatIDs := os.Getenv("TELEGRAM_CHAT_IDS"); chatIDs != "" {
		cfg.TelegramChatIDs = strings.Split(chatIDs, ",")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Parse numeric config after string validation passes
	dbMaxConnsStr := os.Getenv("DB_MAX_CONNS")
	dbMinConnsStr := os.Getenv("DB_MIN_CONNS")
	shutdownTimeoutStr := os.Getenv("SHUTDOWN_TIMEOUT_S")
	mlHealthCacheTTLStr := os.Getenv("ML_HEALTH_CACHE_TTL_S")
	mlReadinessTimeoutStr := os.Getenv("ML_READINESS_TIMEOUT_S")

	var parseErrors []string

	if dbMaxConnsStr == "" {
		parseErrors = append(parseErrors, "DB_MAX_CONNS")
	} else if v, err := strconv.ParseInt(dbMaxConnsStr, 10, 32); err != nil || v < 1 {
		parseErrors = append(parseErrors, "DB_MAX_CONNS (must be a positive integer)")
	} else {
		cfg.DBMaxConns = int32(v)
	}

	if dbMinConnsStr == "" {
		parseErrors = append(parseErrors, "DB_MIN_CONNS")
	} else if v, err := strconv.ParseInt(dbMinConnsStr, 10, 32); err != nil || v < 0 {
		parseErrors = append(parseErrors, "DB_MIN_CONNS (must be a non-negative integer)")
	} else {
		cfg.DBMinConns = int32(v)
	}

	if shutdownTimeoutStr == "" {
		parseErrors = append(parseErrors, "SHUTDOWN_TIMEOUT_S")
	} else if v, err := strconv.Atoi(shutdownTimeoutStr); err != nil || v < 1 {
		parseErrors = append(parseErrors, "SHUTDOWN_TIMEOUT_S (must be a positive integer)")
	} else {
		cfg.ShutdownTimeoutS = v
	}

	if mlHealthCacheTTLStr == "" {
		parseErrors = append(parseErrors, "ML_HEALTH_CACHE_TTL_S")
	} else if v, err := strconv.Atoi(mlHealthCacheTTLStr); err != nil || v < 1 {
		parseErrors = append(parseErrors, "ML_HEALTH_CACHE_TTL_S (must be a positive integer)")
	} else {
		cfg.MLHealthCacheTTLS = v
	}

	if mlReadinessTimeoutStr == "" {
		parseErrors = append(parseErrors, "ML_READINESS_TIMEOUT_S")
	} else if v, err := strconv.Atoi(mlReadinessTimeoutStr); err != nil || v < 0 {
		parseErrors = append(parseErrors, "ML_READINESS_TIMEOUT_S (must be a non-negative integer)")
	} else {
		cfg.MLReadinessTimeoutS = v
	}

	if len(parseErrors) > 0 {
		return nil, fmt.Errorf("missing or invalid required configuration: %s", strings.Join(parseErrors, ", "))
	}

	// Cross-validate: DBMinConns must not exceed DBMaxConns
	if cfg.DBMinConns > cfg.DBMaxConns {
		return nil, fmt.Errorf("DB_MIN_CONNS (%d) must not exceed DB_MAX_CONNS (%d)", cfg.DBMinConns, cfg.DBMaxConns)
	}

	// Parse optional telegram assembly config (SST-compliant — defaults in smackerel.yaml)
	if v := os.Getenv("TELEGRAM_ASSEMBLY_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 && n <= 60 {
			cfg.TelegramAssemblyWindowSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_ASSEMBLY_WINDOW_SECONDS must be an integer in range [5, 60] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_ASSEMBLY_MAX_MESSAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10 && n <= 500 {
			cfg.TelegramAssemblyMaxMessages = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_ASSEMBLY_MAX_MESSAGES must be an integer in range [10, 500] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 10 {
			cfg.TelegramMediaGroupWindowSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS must be an integer in range [2, 10] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 30 && n <= 600 {
			cfg.TelegramDisambiguationTimeoutSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS must be an integer in range [30, 600] (got %q)", v)
		}
	}

	// Parse telegram cook session config (SST-compliant — from smackerel.yaml via config generate)
	cookTimeoutStr := os.Getenv("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES")
	if cookTimeoutStr == "" {
		return nil, fmt.Errorf("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES is required")
	}
	cookTimeoutVal, err := strconv.Atoi(cookTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES: %w", err)
	}
	cfg.TelegramCookSessionTimeoutMinutes = cookTimeoutVal

	cookMaxStr := os.Getenv("TELEGRAM_COOK_SESSION_MAX_PER_CHAT")
	if cookMaxStr == "" {
		return nil, fmt.Errorf("TELEGRAM_COOK_SESSION_MAX_PER_CHAT is required")
	}
	cookMaxVal, err := strconv.Atoi(cookMaxStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_COOK_SESSION_MAX_PER_CHAT: %w", err)
	}
	cfg.TelegramCookSessionMaxPerChat = cookMaxVal

	// Parse knowledge layer config (SST-compliant — from smackerel.yaml via config generate)
	knowledgeEnabledStr := os.Getenv("KNOWLEDGE_ENABLED")
	if knowledgeEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: KNOWLEDGE_ENABLED")
	}
	cfg.KnowledgeEnabled = knowledgeEnabledStr == "true"

	if cfg.KnowledgeEnabled {
		var knowledgeErrors []string

		synthTimeoutStr := os.Getenv("KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS")
		if synthTimeoutStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS")
		} else if v, err := strconv.Atoi(synthTimeoutStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS (must be a positive integer)")
		} else {
			cfg.KnowledgeSynthesisTimeoutSeconds = v
		}

		cfg.KnowledgeLintCron = os.Getenv("KNOWLEDGE_LINT_CRON")
		if cfg.KnowledgeLintCron == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_CRON")
		} else if !isValidCronExpr(cfg.KnowledgeLintCron) {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_CRON (not a valid cron expression)")
		}

		staleDaysStr := os.Getenv("KNOWLEDGE_LINT_STALE_DAYS")
		if staleDaysStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_STALE_DAYS")
		} else if v, err := strconv.Atoi(staleDaysStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_STALE_DAYS (must be a positive integer)")
		} else {
			cfg.KnowledgeLintStaleDays = v
		}

		maxTokensStr := os.Getenv("KNOWLEDGE_CONCEPT_MAX_TOKENS")
		if maxTokensStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_MAX_TOKENS")
		} else if v, err := strconv.Atoi(maxTokensStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_MAX_TOKENS (must be a positive integer)")
		} else {
			cfg.KnowledgeConceptMaxTokens = v
		}

		conceptSearchThresholdStr := os.Getenv("KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD")
		if conceptSearchThresholdStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD")
		} else if v, err := strconv.ParseFloat(conceptSearchThresholdStr, 64); err != nil || v < 0 || v > 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD (must be a float in [0, 1])")
		} else {
			cfg.KnowledgeConceptSearchThreshold = v
		}

		crossSourceThresholdStr := os.Getenv("KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD")
		if crossSourceThresholdStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD")
		} else if v, err := strconv.ParseFloat(crossSourceThresholdStr, 64); err != nil || v < 0 || v > 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD (must be a float in [0, 1])")
		} else {
			cfg.KnowledgeCrossSourceConfidenceThreshold = v
		}

		maxRetriesStr := os.Getenv("KNOWLEDGE_MAX_SYNTHESIS_RETRIES")
		if maxRetriesStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_MAX_SYNTHESIS_RETRIES")
		} else if v, err := strconv.Atoi(maxRetriesStr); err != nil || v < 0 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_MAX_SYNTHESIS_RETRIES (must be a non-negative integer)")
		} else {
			cfg.KnowledgeMaxSynthesisRetries = v
		}

		cfg.KnowledgePromptContractIngestSynthesis = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS")
		if cfg.KnowledgePromptContractIngestSynthesis == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS")
		}

		cfg.KnowledgePromptContractCrossSource = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE")
		if cfg.KnowledgePromptContractCrossSource == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE")
		}

		cfg.KnowledgePromptContractLintAudit = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT")
		if cfg.KnowledgePromptContractLintAudit == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT")
		}

		cfg.KnowledgePromptContractQueryAugment = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT")
		if cfg.KnowledgePromptContractQueryAugment == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT")
		}

		cfg.KnowledgePromptContractDigestAssembly = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY")
		if cfg.KnowledgePromptContractDigestAssembly == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY")
		}

		if len(knowledgeErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required knowledge configuration: %s", strings.Join(knowledgeErrors, ", "))
		}
	}

	// Parse prompt contracts dir (SST-compliant — from smackerel.yaml via config generate)
	cfg.PromptContractsDir = os.Getenv("PROMPT_CONTRACTS_DIR")

	// Parse observability config (SST-compliant — opt-in, disabled by default)
	cfg.OTELEnabled = os.Getenv("OTEL_ENABLED") == "true"
	cfg.OTELExporterEndpoint = os.Getenv("OTEL_EXPORTER_ENDPOINT")

	// Parse expense tracking config (SST-compliant — from smackerel.yaml via config generate)
	expensesEnabledStr := os.Getenv("EXPENSES_ENABLED")
	if expensesEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: EXPENSES_ENABLED")
	}
	cfg.ExpensesEnabled = expensesEnabledStr == "true"

	if cfg.ExpensesEnabled {
		var expenseErrors []string

		cfg.ExpensesDefaultCurrency = os.Getenv("EXPENSES_DEFAULT_CURRENCY")
		if cfg.ExpensesDefaultCurrency == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DEFAULT_CURRENCY")
		}

		exportMaxRowsStr := os.Getenv("EXPENSES_EXPORT_MAX_ROWS")
		if exportMaxRowsStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_MAX_ROWS")
		} else if v, err := strconv.Atoi(exportMaxRowsStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_MAX_ROWS (must be a positive integer)")
		} else {
			cfg.ExpensesExportMaxRows = v
		}

		cfg.ExpensesExportQBDateFormat = os.Getenv("EXPENSES_EXPORT_QB_DATE_FORMAT")
		if cfg.ExpensesExportQBDateFormat == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_QB_DATE_FORMAT")
		}

		cfg.ExpensesExportStdDateFormat = os.Getenv("EXPENSES_EXPORT_STD_DATE_FORMAT")
		if cfg.ExpensesExportStdDateFormat == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_STD_DATE_FORMAT")
		}

		minConfStr := os.Getenv("EXPENSES_SUGGESTIONS_MIN_CONFIDENCE")
		if minConfStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_CONFIDENCE")
		} else if v, err := strconv.ParseFloat(minConfStr, 64); err != nil || v < 0 || v > 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_CONFIDENCE (must be a float in [0, 1])")
		} else {
			cfg.ExpensesSuggestionsMinConfidence = v
		}

		minPastStr := os.Getenv("EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS")
		if minPastStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS")
		} else if v, err := strconv.Atoi(minPastStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsMinPastBusiness = v
		}

		maxPerDigestStr := os.Getenv("EXPENSES_SUGGESTIONS_MAX_PER_DIGEST")
		if maxPerDigestStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MAX_PER_DIGEST")
		} else if v, err := strconv.Atoi(maxPerDigestStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MAX_PER_DIGEST (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsMaxPerDigest = v
		}

		reclassLimStr := os.Getenv("EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT")
		if reclassLimStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT")
		} else if v, err := strconv.Atoi(reclassLimStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsReclassifyBatchLim = v
		}

		vendorCacheStr := os.Getenv("EXPENSES_VENDOR_CACHE_SIZE")
		if vendorCacheStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_VENDOR_CACHE_SIZE")
		} else if v, err := strconv.Atoi(vendorCacheStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_VENDOR_CACHE_SIZE (must be a positive integer)")
		} else {
			cfg.ExpensesVendorCacheSize = v
		}

		digestMaxWordsStr := os.Getenv("EXPENSES_DIGEST_MAX_WORDS")
		if digestMaxWordsStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MAX_WORDS")
		} else if v, err := strconv.Atoi(digestMaxWordsStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MAX_WORDS (must be a positive integer)")
		} else {
			cfg.ExpensesDigestMaxWords = v
		}

		digestNeedsReviewStr := os.Getenv("EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT")
		if digestNeedsReviewStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT")
		} else if v, err := strconv.Atoi(digestNeedsReviewStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT (must be a positive integer)")
		} else {
			cfg.ExpensesDigestNeedsReviewLimit = v
		}

		missingReceiptStr := os.Getenv("EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS")
		if missingReceiptStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS")
		} else if v, err := strconv.Atoi(missingReceiptStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS (must be a positive integer)")
		} else {
			cfg.ExpensesDigestMissingReceiptLookback = v
		}

		// JSON-encoded complex config
		imapLabelsStr := os.Getenv("IMAP_EXPENSE_LABELS")
		if imapLabelsStr == "" || imapLabelsStr == "{}" {
			cfg.IMAPExpenseLabels = make(map[string]string)
		} else if err := json.Unmarshal([]byte(imapLabelsStr), &cfg.IMAPExpenseLabels); err != nil {
			expenseErrors = append(expenseErrors, "IMAP_EXPENSE_LABELS (invalid JSON)")
		}

		businessVendorsStr := os.Getenv("EXPENSES_BUSINESS_VENDORS")
		if businessVendorsStr == "" || businessVendorsStr == "[]" {
			cfg.ExpensesBusinessVendors = []string{}
		} else if err := json.Unmarshal([]byte(businessVendorsStr), &cfg.ExpensesBusinessVendors); err != nil {
			expenseErrors = append(expenseErrors, "EXPENSES_BUSINESS_VENDORS (invalid JSON)")
		}

		categoriesStr := os.Getenv("EXPENSES_CATEGORIES")
		if categoriesStr == "" || categoriesStr == "[]" {
			expenseErrors = append(expenseErrors, "EXPENSES_CATEGORIES (must contain at least one category)")
		} else if err := json.Unmarshal([]byte(categoriesStr), &cfg.ExpensesCategories); err != nil {
			expenseErrors = append(expenseErrors, "EXPENSES_CATEGORIES (invalid JSON)")
		}

		if len(expenseErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required expense configuration: %s", strings.Join(expenseErrors, ", "))
		}
	}

	// Parse meal planning config (SST-compliant — from smackerel.yaml via config generate)
	mealPlanEnabledStr := os.Getenv("MEAL_PLANNING_ENABLED")
	if mealPlanEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: MEAL_PLANNING_ENABLED")
	}
	cfg.MealPlanEnabled = mealPlanEnabledStr == "true"

	if cfg.MealPlanEnabled {
		var mealPlanErrors []string

		defaultServStr := os.Getenv("MEAL_PLANNING_DEFAULT_SERVINGS")
		if defaultServStr == "" {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_DEFAULT_SERVINGS")
		} else if v, err := strconv.Atoi(defaultServStr); err != nil || v < 1 {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_DEFAULT_SERVINGS (must be a positive integer)")
		} else {
			cfg.MealPlanDefaultServings = v
		}

		mealTypesStr := os.Getenv("MEAL_PLANNING_MEAL_TYPES")
		if mealTypesStr == "" {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_MEAL_TYPES")
		} else {
			// Parse comma-separated, stripping brackets and quotes from YAML array format
			cleaned := strings.Trim(mealTypesStr, "[] ")
			var types []string
			for _, t := range strings.Split(cleaned, ",") {
				t = strings.Trim(strings.TrimSpace(t), "\"'")
				if t != "" {
					types = append(types, t)
				}
			}
			if len(types) == 0 {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_MEAL_TYPES (must contain at least one type)")
			} else {
				cfg.MealPlanMealTypes = types
			}
		}

		cfg.MealPlanMealTimes = make(map[string]string)
		mealTimeKeys := map[string]string{
			"MEAL_PLANNING_MEAL_TIME_BREAKFAST": "breakfast",
			"MEAL_PLANNING_MEAL_TIME_LUNCH":     "lunch",
			"MEAL_PLANNING_MEAL_TIME_DINNER":    "dinner",
			"MEAL_PLANNING_MEAL_TIME_SNACK":     "snack",
		}
		for envKey, mealKey := range mealTimeKeys {
			if v := os.Getenv(envKey); v != "" {
				cfg.MealPlanMealTimes[mealKey] = v
			}
		}

		cfg.MealPlanCalendarSync = os.Getenv("MEAL_PLANNING_CALENDAR_SYNC") == "true"

		cfg.MealPlanAutoComplete = os.Getenv("MEAL_PLANNING_AUTO_COMPLETE") == "true"

		cfg.MealPlanAutoCompleteCron = os.Getenv("MEAL_PLANNING_AUTO_COMPLETE_CRON")
		if cfg.MealPlanAutoComplete {
			if cfg.MealPlanAutoCompleteCron == "" {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_AUTO_COMPLETE_CRON")
			} else if !isValidCronExpr(cfg.MealPlanAutoCompleteCron) {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_AUTO_COMPLETE_CRON (not a valid cron expression)")
			}
		}

		if len(mealPlanErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required meal planning configuration: %s", strings.Join(mealPlanErrors, ", "))
		}
	}

	return cfg, nil
}

// requiredVars returns the list of required environment variable names
// and their corresponding values from the config.
func (c *Config) requiredVars() []struct {
	Name  string
	Value string
} {
	vars := []struct {
		Name  string
		Value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"NATS_URL", c.NATSURL},
		{"LLM_PROVIDER", c.LLMProvider},
		{"LLM_MODEL", c.LLMModel},
		{"SMACKEREL_AUTH_TOKEN", c.AuthToken},
		{"EMBEDDING_MODEL", c.EmbeddingModel},
		{"DIGEST_CRON", c.DigestCron},
		{"LOG_LEVEL", c.LogLevel},
		{"PORT", c.Port},
		{"ML_SIDECAR_URL", c.MLSidecarURL},
		{"CORE_API_URL", c.CoreAPIURL},
	}
	// Ollama vars are only required when using Ollama as the LLM provider
	if strings.EqualFold(c.LLMProvider, "ollama") {
		vars = append(vars,
			struct{ Name, Value string }{"OLLAMA_URL", c.OllamaURL},
			struct{ Name, Value string }{"OLLAMA_MODEL", c.OllamaModel},
		)
	}
	return vars
}

// Validate checks that all required configuration values are present.
// Returns an error listing all missing variables.
func (c *Config) Validate() error {
	var missing []string
	for _, v := range c.requiredVars() {
		if v.Value == "" {
			missing = append(missing, v.Name)
		}
	}
	// LLM_API_KEY is required unless using Ollama
	if !strings.EqualFold(c.LLMProvider, "ollama") && c.LLMAPIKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	// Reject known placeholder auth tokens — these are guessable defaults
	placeholders := []string{
		"development-change-me",
		"changeme",
		"change-me",
		"placeholder",
		"test-token",
		"default",
		"dev-token-smackerel-2026",
	}
	for _, p := range placeholders {
		if strings.EqualFold(c.AuthToken, p) {
			return fmt.Errorf("SMACKEREL_AUTH_TOKEN is set to a known placeholder value %q — generate a secure random token: openssl rand -hex 24", c.AuthToken)
		}
	}
	// Reject any token starting with "dev-token-" — these are development-only patterns
	if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") {
		return fmt.Errorf("SMACKEREL_AUTH_TOKEN starts with 'dev-token-' which is a development placeholder pattern — generate a secure random token: openssl rand -hex 24")
	}
	if len(c.AuthToken) < 16 {
		return fmt.Errorf("SMACKEREL_AUTH_TOKEN must be at least 16 characters (got %d)", len(c.AuthToken))
	}

	// Semantic validation: PORT must be a valid TCP port number
	if c.Port != "" {
		port, err := strconv.Atoi(c.Port)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("PORT must be a number between 1 and 65535 (got %q)", c.Port)
		}
	}

	// Semantic validation: LOG_LEVEL must be a recognized value
	if c.LogLevel != "" {
		switch strings.ToLower(c.LogLevel) {
		case "debug", "info", "warn", "error":
			// valid
		default:
			return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error (got %q)", c.LogLevel)
		}
	}

	// Semantic validation: DIGEST_CRON must look like a valid 5-field cron expression
	if c.DigestCron != "" {
		if !isValidCronExpr(c.DigestCron) {
			return fmt.Errorf("DIGEST_CRON is not a valid cron expression (got %q)", c.DigestCron)
		}
	}

	return nil
}

// cronFieldPattern matches a single cron field: number, *, ranges, steps, lists.
var cronFieldPattern = regexp.MustCompile(`^(\*|[0-9]+(-[0-9]+)?)((/[0-9]+)|(,[0-9]+(-[0-9]+)?)*)$`)

// isValidCronExpr validates a 5-field standard cron expression (minute hour dom month dow).
func isValidCronExpr(expr string) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	for _, f := range fields {
		if !cronFieldPattern.MatchString(f) {
			return false
		}
	}
	return true
}

// parseEnvFloat reads an env var and returns its float64 value, or 0 if unset/invalid.
func parseEnvFloat(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseEnvJSONArray reads an env var containing a JSON array and returns []interface{}.
func parseEnvJSONArray(key string) []interface{} {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseEnvJSONObject reads an env var containing a JSON object and returns map[string]interface{}.
func parseEnvJSONObject(key string) map[string]interface{} {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseIntEnv reads an env var as an int, returning defaultVal when empty or unparseable.
func parseIntEnv(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
