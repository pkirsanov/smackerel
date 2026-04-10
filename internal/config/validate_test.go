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
		"CORE_API_URL",
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
		"CORE_API_URL",
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
	for _, key := range []string{"EMBEDDING_MODEL", "DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL", "CORE_API_URL"} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when generated runtime values are missing")
	}
	for _, key := range []string{"EMBEDDING_MODEL", "DIGEST_CRON", "LOG_LEVEL", "PORT", "ML_SIDECAR_URL", "CORE_API_URL"} {
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

func TestValidate_PortSemanticValid(t *testing.T) {
	setRequiredEnv(t)
	for _, port := range []string{"1", "80", "8080", "65535"} {
		t.Setenv("PORT", port)
		_, err := Load()
		if err != nil {
			t.Errorf("PORT=%s should be valid, got: %v", port, err)
		}
	}
}

func TestValidate_PortSemanticInvalid(t *testing.T) {
	setRequiredEnv(t)
	for _, port := range []string{"abc", "0", "65536", "-1", "8080x"} {
		t.Setenv("PORT", port)
		_, err := Load()
		if err == nil {
			t.Errorf("PORT=%s should be rejected", port)
		}
		if err != nil && !strings.Contains(err.Error(), "PORT") {
			t.Errorf("PORT=%s error should mention PORT, got: %v", port, err)
		}
	}
}

func TestValidate_DigestCronValid(t *testing.T) {
	setRequiredEnv(t)
	for _, cron := range []string{"0 7 * * *", "*/5 * * * *", "0 0 1 1 0"} {
		t.Setenv("DIGEST_CRON", cron)
		_, err := Load()
		if err != nil {
			t.Errorf("DIGEST_CRON=%q should be valid, got: %v", cron, err)
		}
	}
}

func TestValidate_DigestCronInvalid(t *testing.T) {
	setRequiredEnv(t)
	for _, cron := range []string{"every day", "* * *", "0 7 * * * *"} {
		t.Setenv("DIGEST_CRON", cron)
		_, err := Load()
		if err == nil {
			t.Errorf("DIGEST_CRON=%q should be rejected", cron)
		}
		if err != nil && !strings.Contains(err.Error(), "DIGEST_CRON") {
			t.Errorf("DIGEST_CRON=%q error should mention DIGEST_CRON, got: %v", cron, err)
		}
	}
}

func TestValidate_TelegramChatIDs_Empty(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("TELEGRAM_CHAT_IDS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.TelegramChatIDs) != 0 {
		t.Errorf("expected 0 chat IDs for empty env, got %d", len(cfg.TelegramChatIDs))
	}
}

func TestValidate_TelegramChatIDs_SingleID(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("TELEGRAM_CHAT_IDS", "12345")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.TelegramChatIDs) != 1 {
		t.Errorf("expected 1 chat ID, got %d", len(cfg.TelegramChatIDs))
	}
	if cfg.TelegramChatIDs[0] != "12345" {
		t.Errorf("expected '12345', got %q", cfg.TelegramChatIDs[0])
	}
}

func TestValidate_OllamaProvider_LLMAPIKeyNotRequired(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_URL", "http://ollama:11434")
	t.Setenv("OLLAMA_MODEL", "llama3.2")
	t.Setenv("LLM_API_KEY", "") // Should not be required for Ollama
	_, err := Load()
	if err != nil {
		t.Fatalf("ollama provider should not require LLM_API_KEY: %v", err)
	}
}

func TestValidate_AuthTokenExactly16Chars(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_AUTH_TOKEN", "exactly16chars!!")
	_, err := Load()
	if err != nil {
		t.Errorf("16-char auth token should be valid: %v", err)
	}
}

func TestValidate_AuthTokenAllPlaceholdersRejected(t *testing.T) {
	setRequiredEnv(t)
	placeholders := []string{
		"development-change-me",
		"changeme",
		"change-me",
		"placeholder",
		"test-token",
		"default",
	}
	for _, p := range placeholders {
		t.Setenv("SMACKEREL_AUTH_TOKEN", p)
		_, err := Load()
		if err == nil {
			t.Errorf("placeholder %q should be rejected", p)
		}
	}
}

func TestValidate_AuthTokenCaseInsensitivePlaceholder(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_AUTH_TOKEN", "DEVELOPMENT-CHANGE-ME")
	_, err := Load()
	if err == nil {
		t.Fatal("uppercase placeholder should also be rejected")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error should mention placeholder, got: %v", err)
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
	t.Setenv("CORE_API_URL", "http://smackerel-core:8080")
}
