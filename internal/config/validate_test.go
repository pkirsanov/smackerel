package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidate_AllPresent(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DatabaseURL != "postgres://test:test@localhost:5432/test" {
		t.Errorf("unexpected DatabaseURL: %s", cfg.DatabaseURL)
	}
}

func TestValidate_MissingDatabaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name DATABASE_URL, got: %v", err)
	}
}

func TestValidate_MissingMultiple(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing vars")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name DATABASE_URL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "LLM_API_KEY") {
		t.Errorf("error should name LLM_API_KEY, got: %v", err)
	}
}

func TestValidate_MissingAllRequired(t *testing.T) {
	// Clear all required vars
	for _, key := range []string{
		"DATABASE_URL", "NATS_URL", "LLM_PROVIDER",
		"LLM_MODEL", "LLM_API_KEY", "SMACKEREL_AUTH_TOKEN",
	} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing all required vars")
	}
	for _, key := range []string{
		"DATABASE_URL", "NATS_URL", "LLM_PROVIDER",
		"LLM_MODEL", "LLM_API_KEY", "SMACKEREL_AUTH_TOKEN",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should name %s, got: %v", key, err)
		}
	}
}

func TestValidate_OptionalDefaults(t *testing.T) {
	setRequiredEnv(t)
	// Do not set any optional vars
	for _, key := range []string{"OLLAMA_URL", "OLLAMA_MODEL", "EMBEDDING_MODEL", "DIGEST_CRON", "LOG_LEVEL", "PORT"} {
		os.Unsetenv(key)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got: %s", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got: %s", cfg.LogLevel)
	}
	if cfg.EmbeddingModel != "all-MiniLM-L6-v2" {
		t.Errorf("expected default embedding model, got: %s", cfg.EmbeddingModel)
	}
}

func TestValidate_TelegramChatIDs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("TELEGRAM_CHAT_IDS", "123,456,789")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(cfg.TelegramChatIDs) != 3 {
		t.Errorf("expected 3 chat IDs, got: %d", len(cfg.TelegramChatIDs))
	}
}

func TestValidate_NoHiddenDefaults_Required(t *testing.T) {
	// Ensure required vars have no hidden defaults when env is empty.
	for _, key := range []string{
		"DATABASE_URL", "NATS_URL", "LLM_PROVIDER",
		"LLM_MODEL", "LLM_API_KEY", "SMACKEREL_AUTH_TOKEN",
	} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected failure with all required vars empty — no hidden defaults allowed")
	}
}

// setRequiredEnv sets all required env vars with test values.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_MODEL", "gpt-4o-mini")
	t.Setenv("LLM_API_KEY", "sk-test-key")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "test-token")
}
