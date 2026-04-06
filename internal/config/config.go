package config

import (
	"fmt"
	"os"
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
		OllamaURL:        getEnvDefault("OLLAMA_URL", "http://ollama:11434"),
		OllamaModel:      os.Getenv("OLLAMA_MODEL"),
		EmbeddingModel:   getEnvDefault("EMBEDDING_MODEL", "all-MiniLM-L6-v2"),
		DigestCron:       getEnvDefault("DIGEST_CRON", "0 7 * * *"),
		LogLevel:         getEnvDefault("LOG_LEVEL", "info"),
		Port:             getEnvDefault("PORT", "8080"),
		MLSidecarURL:     getEnvDefault("ML_SIDECAR_URL", "http://smackerel-ml:8081"),
	}

	if chatIDs := os.Getenv("TELEGRAM_CHAT_IDS"); chatIDs != "" {
		cfg.TelegramChatIDs = strings.Split(chatIDs, ",")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requiredVars returns the list of required environment variable names
// and their corresponding values from the config.
// requiredVars returns the list of required environment variable names
// and their corresponding values from the config.
func (c *Config) requiredVars() []struct {
	Name  string
	Value string
} {
	return []struct {
		Name  string
		Value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"NATS_URL", c.NATSURL},
		{"LLM_PROVIDER", c.LLMProvider},
		{"LLM_MODEL", c.LLMModel},
		{"SMACKEREL_AUTH_TOKEN", c.AuthToken},
	}
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
	return nil
}
