package config

import (
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
	// Clear all required vars (with no LLM_PROVIDER set, Ollama vars are NOT required)
	for _, key := range []string{
		"DATABASE_URL", "NATS_URL", "LLM_PROVIDER",
		"LLM_MODEL", "LLM_API_KEY", "SMACKEREL_AUTH_TOKEN",
		"EMBEDDING_MODEL",
		"DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL",
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
		"EMBEDDING_MODEL",
		"DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should name %s, got: %v", key, err)
		}
	}
}

func TestValidate_MissingGeneratedRuntimeValues(t *testing.T) {
	setRequiredEnv(t)
	// OLLAMA_URL/OLLAMA_MODEL are only required when LLM_PROVIDER=ollama;
	// setRequiredEnv sets LLM_PROVIDER=openai so they are NOT required
	for _, key := range []string{"EMBEDDING_MODEL", "DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL"} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when generated runtime values are missing")
	}
	for _, key := range []string{"EMBEDDING_MODEL", "DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should name %s, got: %v", key, err)
		}
	}
}

func TestValidate_OllamaRequiresOllamaVars(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_URL", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("LLM_API_KEY", "") // Not required for Ollama
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing Ollama vars")
	}
	if !strings.Contains(err.Error(), "OLLAMA_URL") {
		t.Errorf("error should name OLLAMA_URL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "OLLAMA_MODEL") {
		t.Errorf("error should name OLLAMA_MODEL, got: %v", err)
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

func TestValidate_PlaceholderAuthTokenRejected(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_AUTH_TOKEN", "development-change-me")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for placeholder auth token")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error should mention placeholder, got: %v", err)
	}
}

func TestValidate_ShortAuthTokenRejected(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_AUTH_TOKEN", "short")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short auth token")
	}
	if !strings.Contains(err.Error(), "at least 16 characters") {
		t.Errorf("error should mention length, got: %v", err)
	}
}

func TestValidate_NoHiddenDefaults_Required(t *testing.T) {
	// Ensure truly required vars have no hidden defaults when env is empty.
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
	t.Setenv("SMACKEREL_AUTH_TOKEN", "a-secure-test-token-for-unit-tests")
	t.Setenv("OLLAMA_URL", "http://ollama:11434")
	t.Setenv("OLLAMA_MODEL", "llama3.2")
	t.Setenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
	t.Setenv("DIGEST_CRON", "0 7 * * *")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("PORT", "8080")
	t.Setenv("ML_SIDECAR_URL", "http://smackerel-ml:8081")
}
