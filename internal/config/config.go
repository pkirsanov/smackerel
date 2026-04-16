package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

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

	// Optional connector path fields (SST-compliant — read from env, sourced from smackerel.yaml)
	BookmarksImportDir    string
	BookmarksEnabled      bool
	BookmarksSyncSchedule string
	BrowserHistoryPath    string
	MapsImportDir         string

	// Telegram assembly config (SST-compliant — from smackerel.yaml via config generate)
	TelegramAssemblyWindowSeconds   int
	TelegramAssemblyMaxMessages     int
	TelegramMediaGroupWindowSeconds int

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

		BookmarksImportDir:    os.Getenv("BOOKMARKS_IMPORT_DIR"),
		BookmarksEnabled:      os.Getenv("BOOKMARKS_ENABLED") == "true",
		BookmarksSyncSchedule: os.Getenv("BOOKMARKS_SYNC_SCHEDULE"),
		BrowserHistoryPath:    os.Getenv("BROWSER_HISTORY_PATH"),
		MapsImportDir:         os.Getenv("MAPS_IMPORT_DIR"),
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
	}
	for _, p := range placeholders {
		if strings.EqualFold(c.AuthToken, p) {
			return fmt.Errorf("SMACKEREL_AUTH_TOKEN is set to a known placeholder value %q — generate a secure random token", c.AuthToken)
		}
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
