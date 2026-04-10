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
	BookmarksImportDir string
	BrowserHistoryPath string
	MapsImportDir      string
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

		BookmarksImportDir: os.Getenv("BOOKMARKS_IMPORT_DIR"),
		BrowserHistoryPath: os.Getenv("BROWSER_HISTORY_PATH"),
		MapsImportDir:      os.Getenv("MAPS_IMPORT_DIR"),
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
