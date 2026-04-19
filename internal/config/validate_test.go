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

func TestValidate_AuthTokenDevTokenPrefixRejected(t *testing.T) {
	setRequiredEnv(t)
	devTokens := []string{
		"dev-token-smackerel-2026",
		"dev-token-anything-here-1234",
		"Dev-Token-MyProject-9999",
	}
	for _, token := range devTokens {
		t.Run(token, func(t *testing.T) {
			t.Setenv("SMACKEREL_AUTH_TOKEN", token)
			_, err := Load()
			if err == nil {
				t.Fatalf("dev-token- prefix %q should be rejected", token)
			}
		})
	}
}

// SCN-023-04: Connector paths flow through config.Config (SST).
func TestLoad_ConnectorPathFields(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BOOKMARKS_IMPORT_DIR", "/data/bookmarks")
	t.Setenv("BROWSER_HISTORY_PATH", "/home/user/.config/google-chrome/Default/History")
	t.Setenv("MAPS_IMPORT_DIR", "/data/maps-takeout")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BookmarksImportDir != "/data/bookmarks" {
		t.Errorf("expected BookmarksImportDir=/data/bookmarks, got %q", cfg.BookmarksImportDir)
	}
	if cfg.BrowserHistoryPath != "/home/user/.config/google-chrome/Default/History" {
		t.Errorf("expected BrowserHistoryPath, got %q", cfg.BrowserHistoryPath)
	}
	if cfg.MapsImportDir != "/data/maps-takeout" {
		t.Errorf("expected MapsImportDir=/data/maps-takeout, got %q", cfg.MapsImportDir)
	}
}

func TestLoad_ConnectorPathFieldsOptional(t *testing.T) {
	setRequiredEnv(t)
	// Not setting connector env vars — should still load successfully

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BookmarksImportDir != "" {
		t.Errorf("expected empty BookmarksImportDir, got %q", cfg.BookmarksImportDir)
	}
	if cfg.BrowserHistoryPath != "" {
		t.Errorf("expected empty BrowserHistoryPath, got %q", cfg.BrowserHistoryPath)
	}
	if cfg.MapsImportDir != "" {
		t.Errorf("expected empty MapsImportDir, got %q", cfg.MapsImportDir)
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
	t.Setenv("DB_MAX_CONNS", "10")
	t.Setenv("DB_MIN_CONNS", "2")
	t.Setenv("SHUTDOWN_TIMEOUT_S", "25")
	t.Setenv("ML_HEALTH_CACHE_TTL_S", "30")
	t.Setenv("ML_READINESS_TIMEOUT_S", "60")
	t.Setenv("KNOWLEDGE_ENABLED", "true")
	t.Setenv("KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS", "30")
	t.Setenv("KNOWLEDGE_LINT_CRON", "0 3 * * *")
	t.Setenv("KNOWLEDGE_LINT_STALE_DAYS", "90")
	t.Setenv("KNOWLEDGE_CONCEPT_MAX_TOKENS", "4000")
	t.Setenv("KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD", "0.4")
	t.Setenv("KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD", "0.7")
	t.Setenv("KNOWLEDGE_MAX_SYNTHESIS_RETRIES", "3")
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS", "ingest-synthesis-v1")
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE", "cross-source-connection-v1")
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT", "lint-audit-v1")
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT", "query-augment-v1")
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY", "digest-assembly-v1")
	t.Setenv("EXPENSES_ENABLED", "true")
	t.Setenv("EXPENSES_DEFAULT_CURRENCY", "USD")
	t.Setenv("EXPENSES_EXPORT_MAX_ROWS", "10000")
	t.Setenv("EXPENSES_EXPORT_QB_DATE_FORMAT", "01/02/2006")
	t.Setenv("EXPENSES_EXPORT_STD_DATE_FORMAT", "2006-01-02")
	t.Setenv("EXPENSES_SUGGESTIONS_MIN_CONFIDENCE", "0.6")
	t.Setenv("EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS", "2")
	t.Setenv("EXPENSES_SUGGESTIONS_MAX_PER_DIGEST", "3")
	t.Setenv("EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT", "100")
	t.Setenv("EXPENSES_VENDOR_CACHE_SIZE", "500")
	t.Setenv("EXPENSES_DIGEST_MAX_WORDS", "100")
	t.Setenv("EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT", "5")
	t.Setenv("EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS", "35")
	t.Setenv("IMAP_EXPENSE_LABELS", "{}")
	t.Setenv("EXPENSES_BUSINESS_VENDORS", "[]")
	t.Setenv("EXPENSES_CATEGORIES", `[{"slug":"food-and-drink","display":"Food & Drink","tax_category":"Meals"}]`)
	t.Setenv("MEAL_PLANNING_ENABLED", "true")
	t.Setenv("MEAL_PLANNING_DEFAULT_SERVINGS", "2")
	t.Setenv("MEAL_PLANNING_MEAL_TYPES", "breakfast,lunch,dinner,snack")
	t.Setenv("MEAL_PLANNING_MEAL_TIME_BREAKFAST", "08:00")
	t.Setenv("MEAL_PLANNING_MEAL_TIME_LUNCH", "12:00")
	t.Setenv("MEAL_PLANNING_MEAL_TIME_DINNER", "18:00")
	t.Setenv("MEAL_PLANNING_MEAL_TIME_SNACK", "15:00")
	t.Setenv("MEAL_PLANNING_CALENDAR_SYNC", "false")
	t.Setenv("MEAL_PLANNING_AUTO_COMPLETE", "true")
	t.Setenv("MEAL_PLANNING_AUTO_COMPLETE_CRON", "0 1 * * *")
}

func TestValidate_DBMaxConns_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_CONNS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DB_MAX_CONNS")
	}
	if !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Errorf("error should name DB_MAX_CONNS, got: %v", err)
	}
}

func TestValidate_DBMinConns_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MIN_CONNS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DB_MIN_CONNS")
	}
	if !strings.Contains(err.Error(), "DB_MIN_CONNS") {
		t.Errorf("error should name DB_MIN_CONNS, got: %v", err)
	}
}

func TestValidate_ShutdownTimeoutS_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SHUTDOWN_TIMEOUT_S", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing SHUTDOWN_TIMEOUT_S")
	}
	if !strings.Contains(err.Error(), "SHUTDOWN_TIMEOUT_S") {
		t.Errorf("error should name SHUTDOWN_TIMEOUT_S, got: %v", err)
	}
}

func TestValidate_MLHealthCacheTTLS_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ML_HEALTH_CACHE_TTL_S", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ML_HEALTH_CACHE_TTL_S")
	}
	if !strings.Contains(err.Error(), "ML_HEALTH_CACHE_TTL_S") {
		t.Errorf("error should name ML_HEALTH_CACHE_TTL_S, got: %v", err)
	}
}

func TestValidate_DBMinConns_ExceedsMaxConns(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MIN_CONNS", "20")
	t.Setenv("DB_MAX_CONNS", "5")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DB_MIN_CONNS > DB_MAX_CONNS")
	}
	if !strings.Contains(err.Error(), "DB_MIN_CONNS") || !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Errorf("error should name both DB_MIN_CONNS and DB_MAX_CONNS, got: %v", err)
	}
}

func TestValidate_DBMinConns_EqualsMaxConns(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MIN_CONNS", "10")
	t.Setenv("DB_MAX_CONNS", "10")
	_, err := Load()
	if err != nil {
		t.Fatalf("DB_MIN_CONNS == DB_MAX_CONNS should be valid, got: %v", err)
	}
}

// --- Telegram assembly config validation tests (spec 008) ---

func TestValidate_TelegramAssemblyWindowSeconds_Valid(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"5", "10", "30", "60"} {
		t.Setenv("TELEGRAM_ASSEMBLY_WINDOW_SECONDS", val)
		_, err := Load()
		if err != nil {
			t.Errorf("TELEGRAM_ASSEMBLY_WINDOW_SECONDS=%s should be valid, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramAssemblyWindowSeconds_OutOfRange(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"0", "1", "4", "61", "100", "-1", "abc"} {
		t.Setenv("TELEGRAM_ASSEMBLY_WINDOW_SECONDS", val)
		_, err := Load()
		if err == nil {
			t.Errorf("TELEGRAM_ASSEMBLY_WINDOW_SECONDS=%s should be rejected", val)
		}
		if err != nil && !strings.Contains(err.Error(), "TELEGRAM_ASSEMBLY_WINDOW_SECONDS") {
			t.Errorf("TELEGRAM_ASSEMBLY_WINDOW_SECONDS=%s error should name the field, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramAssemblyMaxMessages_Valid(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"10", "100", "250", "500"} {
		t.Setenv("TELEGRAM_ASSEMBLY_MAX_MESSAGES", val)
		_, err := Load()
		if err != nil {
			t.Errorf("TELEGRAM_ASSEMBLY_MAX_MESSAGES=%s should be valid, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramAssemblyMaxMessages_OutOfRange(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"0", "5", "9", "501", "1000", "-1", "abc"} {
		t.Setenv("TELEGRAM_ASSEMBLY_MAX_MESSAGES", val)
		_, err := Load()
		if err == nil {
			t.Errorf("TELEGRAM_ASSEMBLY_MAX_MESSAGES=%s should be rejected", val)
		}
		if err != nil && !strings.Contains(err.Error(), "TELEGRAM_ASSEMBLY_MAX_MESSAGES") {
			t.Errorf("TELEGRAM_ASSEMBLY_MAX_MESSAGES=%s error should name the field, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramMediaGroupWindowSeconds_Valid(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"2", "3", "5", "10"} {
		t.Setenv("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS", val)
		_, err := Load()
		if err != nil {
			t.Errorf("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS=%s should be valid, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramMediaGroupWindowSeconds_OutOfRange(t *testing.T) {
	setRequiredEnv(t)
	for _, val := range []string{"0", "1", "11", "100", "-1", "abc"} {
		t.Setenv("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS", val)
		_, err := Load()
		if err == nil {
			t.Errorf("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS=%s should be rejected", val)
		}
		if err != nil && !strings.Contains(err.Error(), "TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS") {
			t.Errorf("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS=%s error should name the field, got: %v", val, err)
		}
	}
}

func TestValidate_TelegramAssemblyConfig_Defaults(t *testing.T) {
	setRequiredEnv(t)
	// Not setting any assembly config env vars — should load with zero values
	// (defaults applied at assembler construction time, not at config load)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TelegramAssemblyWindowSeconds != 0 {
		t.Errorf("expected 0 (defaults applied at assembler init), got %d", cfg.TelegramAssemblyWindowSeconds)
	}
	if cfg.TelegramAssemblyMaxMessages != 0 {
		t.Errorf("expected 0 (defaults applied at assembler init), got %d", cfg.TelegramAssemblyMaxMessages)
	}
	if cfg.TelegramMediaGroupWindowSeconds != 0 {
		t.Errorf("expected 0 (defaults applied at assembler init), got %d", cfg.TelegramMediaGroupWindowSeconds)
	}
}

func TestValidate_DBPoolConfig_Valid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "5")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBMaxConns != 20 {
		t.Errorf("expected DBMaxConns=20, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 5 {
		t.Errorf("expected DBMinConns=5, got %d", cfg.DBMinConns)
	}
}

func TestValidate_DBMaxConns_Invalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_CONNS", "abc")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DB_MAX_CONNS")
	}
	if !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Errorf("error should name DB_MAX_CONNS, got: %v", err)
	}
}

func TestValidate_LogLevel_Invalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LOG_LEVEL", "verbose")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("error should name LOG_LEVEL, got: %v", err)
	}
}

func TestValidate_LogLevel_ValidValues(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("LOG_LEVEL", level)
			_, err := Load()
			if err != nil {
				t.Fatalf("expected no error for LOG_LEVEL=%s, got: %v", level, err)
			}
		})
	}
}

// --- Knowledge layer config validation tests (spec 025) ---

func TestValidate_KnowledgeEnabled_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_ENABLED", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing KNOWLEDGE_ENABLED")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_ENABLED") {
		t.Errorf("error should name KNOWLEDGE_ENABLED, got: %v", err)
	}
}

func TestValidate_KnowledgeEnabled_False_SkipsValidation(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_ENABLED", "false")
	// Clear all knowledge-specific env vars — should still pass because disabled
	for _, key := range []string{
		"KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS",
		"KNOWLEDGE_LINT_CRON",
		"KNOWLEDGE_LINT_STALE_DAYS",
		"KNOWLEDGE_CONCEPT_MAX_TOKENS",
		"KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD",
		"KNOWLEDGE_MAX_SYNTHESIS_RETRIES",
		"KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS",
		"KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE",
		"KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT",
		"KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT",
		"KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY",
	} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err != nil {
		t.Fatalf("knowledge disabled should skip sub-field validation, got: %v", err)
	}
}

func TestValidate_KnowledgeEnabled_True_MissingSynthesisTimeout(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS") {
		t.Errorf("error should name KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS, got: %v", err)
	}
}

func TestValidate_KnowledgeEnabled_True_MissingPromptContract(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS") {
		t.Errorf("error should name KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS, got: %v", err)
	}
}

func TestValidate_KnowledgeConfig_AllFieldsParsed(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.KnowledgeEnabled {
		t.Error("expected KnowledgeEnabled=true")
	}
	if cfg.KnowledgeSynthesisTimeoutSeconds != 30 {
		t.Errorf("expected SynthesisTimeoutSeconds=30, got %d", cfg.KnowledgeSynthesisTimeoutSeconds)
	}
	if cfg.KnowledgeLintCron != "0 3 * * *" {
		t.Errorf("expected LintCron='0 3 * * *', got %q", cfg.KnowledgeLintCron)
	}
	if cfg.KnowledgeLintStaleDays != 90 {
		t.Errorf("expected LintStaleDays=90, got %d", cfg.KnowledgeLintStaleDays)
	}
	if cfg.KnowledgeConceptMaxTokens != 4000 {
		t.Errorf("expected ConceptMaxTokens=4000, got %d", cfg.KnowledgeConceptMaxTokens)
	}
	if cfg.KnowledgeConceptSearchThreshold != 0.4 {
		t.Errorf("expected ConceptSearchThreshold=0.4, got %f", cfg.KnowledgeConceptSearchThreshold)
	}
	if cfg.KnowledgeCrossSourceConfidenceThreshold != 0.7 {
		t.Errorf("expected CrossSourceConfidenceThreshold=0.7, got %f", cfg.KnowledgeCrossSourceConfidenceThreshold)
	}
	if cfg.KnowledgeMaxSynthesisRetries != 3 {
		t.Errorf("expected MaxSynthesisRetries=3, got %d", cfg.KnowledgeMaxSynthesisRetries)
	}
	if cfg.KnowledgePromptContractIngestSynthesis != "ingest-synthesis-v1" {
		t.Errorf("expected PromptContractIngestSynthesis='ingest-synthesis-v1', got %q", cfg.KnowledgePromptContractIngestSynthesis)
	}
	if cfg.KnowledgePromptContractCrossSource != "cross-source-connection-v1" {
		t.Errorf("expected PromptContractCrossSource='cross-source-connection-v1', got %q", cfg.KnowledgePromptContractCrossSource)
	}
}

func TestValidate_KnowledgeLintCron_Invalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_LINT_CRON", "every day")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid KNOWLEDGE_LINT_CRON")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_LINT_CRON") {
		t.Errorf("error should name KNOWLEDGE_LINT_CRON, got: %v", err)
	}
}

func TestValidate_KnowledgeCrossSourceConfidence_OutOfRange(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD", "1.5")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for out-of-range KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD") {
		t.Errorf("error should name KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD, got: %v", err)
	}
}
