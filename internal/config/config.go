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
		OllamaURL:        os.Getenv("OLLAMA_URL"),
		OllamaModel:      os.Getenv("OLLAMA_MODEL"),
		EmbeddingModel:   os.Getenv("EMBEDDING_MODEL"),
		DigestCron:       os.Getenv("DIGEST_CRON"),
		LogLevel:         os.Getenv("LOG_LEVEL"),
		Port:             os.Getenv("PORT"),
		MLSidecarURL:     os.Getenv("ML_SIDECAR_URL"),
	}

	if chatIDs := os.Getenv("TELEGRAM_CHAT_IDS"); chatIDs != "" {
		cfg.TelegramChatIDs = strings.Split(chatIDs, ",")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
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
	return nil
}
