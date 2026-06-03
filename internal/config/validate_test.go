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
	// Clear all required vars (with no LLM_PROVIDER set, Ollama vars are NOT required).
	// MIT-040-S-004: SMACKEREL_AUTH_TOKEN is removed from the always-required
	// set — it is required only when SMACKEREL_ENV=production. The
	// production-mode requirement is covered by the dedicated
	// TestRuntimeConfig_S004_* tests.
	for _, key := range []string{
		"DATABASE_URL", "NATS_URL", "LLM_PROVIDER",
		"LLM_MODEL", "LLM_API_KEY",
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
		"LLM_MODEL", "LLM_API_KEY",
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

// --- Spec 046 — NATS production hardening fail-loud envelope ---

func TestValidate_NATS_MaxReconnectAttempts_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_MAX_RECONNECT_ATTEMPTS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_MAX_RECONNECT_ATTEMPTS")
	}
	if !strings.Contains(err.Error(), "NATS_MAX_RECONNECT_ATTEMPTS") {
		t.Errorf("error should name NATS_MAX_RECONNECT_ATTEMPTS, got: %v", err)
	}
}

func TestValidate_NATS_ReconnectTimeWait_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_RECONNECT_TIME_WAIT_SECONDS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_RECONNECT_TIME_WAIT_SECONDS")
	}
	if !strings.Contains(err.Error(), "NATS_RECONNECT_TIME_WAIT_SECONDS") {
		t.Errorf("error should name NATS_RECONNECT_TIME_WAIT_SECONDS, got: %v", err)
	}
}

func TestValidate_NATS_MaxPayloadBytes_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_MAX_PAYLOAD_BYTES", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_MAX_PAYLOAD_BYTES")
	}
	if !strings.Contains(err.Error(), "NATS_MAX_PAYLOAD_BYTES") {
		t.Errorf("error should name NATS_MAX_PAYLOAD_BYTES, got: %v", err)
	}
}

func TestValidate_NATS_MaxFileStoreBytes_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_MAX_FILE_STORE_BYTES", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_MAX_FILE_STORE_BYTES")
	}
	if !strings.Contains(err.Error(), "NATS_MAX_FILE_STORE_BYTES") {
		t.Errorf("error should name NATS_MAX_FILE_STORE_BYTES, got: %v", err)
	}
}

func TestValidate_NATS_MaxMemStoreBytes_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_MAX_MEM_STORE_BYTES", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_MAX_MEM_STORE_BYTES")
	}
	if !strings.Contains(err.Error(), "NATS_MAX_MEM_STORE_BYTES") {
		t.Errorf("error should name NATS_MAX_MEM_STORE_BYTES, got: %v", err)
	}
}

func TestValidate_NATS_StreamMaxBytesJSON_Missing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_STREAM_MAX_BYTES_JSON", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_STREAM_MAX_BYTES_JSON")
	}
	if !strings.Contains(err.Error(), "NATS_STREAM_MAX_BYTES_JSON") {
		t.Errorf("error should name NATS_STREAM_MAX_BYTES_JSON, got: %v", err)
	}
}

func TestValidate_NATS_StreamMaxBytesJSON_InvalidJSON(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_STREAM_MAX_BYTES_JSON", "{not valid json")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid NATS_STREAM_MAX_BYTES_JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention invalid JSON, got: %v", err)
	}
}

func TestValidate_NATS_StreamMaxBytesJSON_NonPositiveBytes(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_STREAM_MAX_BYTES_JSON", `[{"stream":"ARTIFACTS","bytes":0}]`)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-positive bytes value")
	}
	if !strings.Contains(err.Error(), "non-positive") {
		t.Errorf("error should mention non-positive, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ARTIFACTS") {
		t.Errorf("error should name the offending stream ARTIFACTS, got: %v", err)
	}
}

func TestValidate_NATS_StreamMaxBytesJSON_DuplicateStream(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_STREAM_MAX_BYTES_JSON", `[{"stream":"ARTIFACTS","bytes":1024},{"stream":"ARTIFACTS","bytes":2048}]`)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for duplicate stream entry")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestValidate_NATS_MaxPayloadBytes_NonInteger(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_MAX_PAYLOAD_BYTES", "not-a-number")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-integer NATS_MAX_PAYLOAD_BYTES")
	}
	if !strings.Contains(err.Error(), "NATS_MAX_PAYLOAD_BYTES") {
		t.Errorf("error should name NATS_MAX_PAYLOAD_BYTES, got: %v", err)
	}
}

func TestValidate_NATS_ReconnectTimeWait_NonPositive(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NATS_RECONNECT_TIME_WAIT_SECONDS", "0")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-positive NATS_RECONNECT_TIME_WAIT_SECONDS")
	}
	if !strings.Contains(err.Error(), "NATS_RECONNECT_TIME_WAIT_SECONDS") {
		t.Errorf("error should name NATS_RECONNECT_TIME_WAIT_SECONDS, got: %v", err)
	}
}

func TestValidate_NATS_EnvelopeAcceptedWhenComplete(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.NATSMaxReconnectAttempts != -1 {
		t.Errorf("expected NATSMaxReconnectAttempts=-1 (indefinite), got %d", cfg.NATSMaxReconnectAttempts)
	}
	if cfg.NATSReconnectTimeWaitSecs != 2 {
		t.Errorf("expected NATSReconnectTimeWaitSecs=2, got %d", cfg.NATSReconnectTimeWaitSecs)
	}
	if cfg.NATSMaxPayloadBytes != 8388608 {
		t.Errorf("expected NATSMaxPayloadBytes=8388608, got %d", cfg.NATSMaxPayloadBytes)
	}
	if cfg.NATSMaxFileStoreBytes != 10737418240 {
		t.Errorf("expected NATSMaxFileStoreBytes=10737418240, got %d", cfg.NATSMaxFileStoreBytes)
	}
	if cfg.NATSMaxMemStoreBytes != 1073741824 {
		t.Errorf("expected NATSMaxMemStoreBytes=1073741824, got %d", cfg.NATSMaxMemStoreBytes)
	}
	// Every stream returned by AllStreams must have a positive cap.
	for _, sc := range []string{"ARTIFACTS", "SEARCH", "DIGEST", "KEEP", "INTELLIGENCE", "ALERTS", "SYNTHESIS", "DOMAIN", "DRIVE", "PHOTOS", "ANNOTATIONS", "LISTS", "AGENT", "WEATHER", "DEADLETTER"} {
		v, ok := cfg.NATSStreamMaxBytes[sc]
		if !ok {
			t.Errorf("NATSStreamMaxBytes missing stream %q", sc)
			continue
		}
		if v <= 0 {
			t.Errorf("NATSStreamMaxBytes[%s] = %d; want positive", sc, v)
		}
	}
}

func TestValidate_QFDecisionsDisabledAllowsEmptyValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "false")
	t.Setenv("QF_DECISIONS_BASE_URL", "")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "")
	// BUG-020-008 — QF_DECISIONS_PACKET_VERSION and QF_DECISIONS_PAGE_SIZE
	// are unconditionally SST-required (the SST generator emits them for
	// every environment, and the silent-default behavior they previously
	// relied on was the bug). Even when QF is disabled, the env vars
	// must be present and parseable; the string-form fields stay
	// connector-conditional.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QFDecisionsEnabled {
		t.Fatal("QFDecisionsEnabled should be false")
	}
}

func TestValidate_QFDecisionsEnabledRequiresExplicitValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "")
	// BUG-020-008 — PACKET_VERSION and PAGE_SIZE are now surfaced by the
	// consolidated requiredVars()/intLoadErrs error in Load() BEFORE
	// validateQFDecisionsConfig runs. To keep this test focused on the
	// connector-conditional string fields, keep the int values valid via
	// setRequiredEnv so validateQFDecisionsConfig is reached.

	_, err := Load()
	if err == nil {
		t.Fatal("expected qf-decisions config error")
	}
	for _, key := range []string{
		"QF_DECISIONS_BASE_URL",
		"QF_DECISIONS_CREDENTIAL_REF",
		"QF_DECISIONS_SYNC_SCHEDULE",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error should include %s: %v", key, err)
		}
	}
}

func TestValidate_QFDecisionsEnabledAcceptsValidValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "https://qf.example.test")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "qf-service-token")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "*/5 * * * *")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "1")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "25")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QFDecisionsBaseURL != "https://qf.example.test" {
		t.Fatalf("QFDecisionsBaseURL = %q", cfg.QFDecisionsBaseURL)
	}
	if cfg.QFDecisionsPacketVersion != 1 || cfg.QFDecisionsPageSize != 25 {
		t.Fatalf("packet/page config = %d/%d", cfg.QFDecisionsPacketVersion, cfg.QFDecisionsPageSize)
	}
}

func TestValidate_QFDecisionsEnabledRejectsInvalidValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "qf.example.test")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "qf-service-token")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "every five minutes")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "0")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "101")

	_, err := Load()
	if err == nil {
		t.Fatal("expected qf-decisions validation error")
	}
	for _, key := range []string{
		"QF_DECISIONS_BASE_URL",
		"QF_DECISIONS_SYNC_SCHEDULE",
		"QF_DECISIONS_PACKET_VERSION",
		"QF_DECISIONS_PAGE_SIZE",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error should include %s: %v", key, err)
		}
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
	t.Setenv("MAPS_ENABLED", "true")

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
	if !cfg.MapsEnabled {
		t.Error("expected MapsEnabled=true when MAPS_ENABLED=true")
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
	if cfg.MapsEnabled {
		t.Error("expected MapsEnabled=false when MAPS_ENABLED not set")
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
	// MIT-040-S-004 — SMACKEREL_ENV is a fail-loud SST signal. Default tests
	// run as the test environment so the empty-token dev-mode bypass is in
	// effect when individual cases override SMACKEREL_AUTH_TOKEN to "".
	t.Setenv("SMACKEREL_ENV", "test")
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
	t.Setenv("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES", "30")
	t.Setenv("TELEGRAM_COOK_SESSION_MAX_PER_CHAT", "3")
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
	t.Setenv("DRIVE_ENABLED", "false")
	t.Setenv("DRIVE_CLASSIFICATION_ENABLED", "true")
	t.Setenv("DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD", "0.7")
	t.Setenv("DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION", "pause")
	t.Setenv("DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD", "0.6")
	t.Setenv("DRIVE_CLASSIFICATION_CONFIRMATION_TTL_SECONDS", "86400")
	t.Setenv("DRIVE_SCAN_PARALLELISM", "4")
	t.Setenv("DRIVE_SCAN_BATCH_SIZE", "100")
	t.Setenv("DRIVE_MONITOR_POLL_INTERVAL_SECONDS", "300")
	t.Setenv("DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES", "5000")
	t.Setenv("DRIVE_POLICY_SENSITIVITY_DEFAULT", "internal")
	t.Setenv("DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC", "0.95")
	t.Setenv("DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL", "0.80")
	t.Setenv("DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE", "0.60")
	t.Setenv("DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET", "0.50")
	t.Setenv("DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES", "5242880")
	t.Setenv("DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY", "10")
	t.Setenv("DRIVE_LIMITS_MAX_FILE_SIZE_BYTES", "104857600")
	t.Setenv("DRIVE_IO_LIMITS_PROVIDER_RESPONSE_MAX_BYTES", "10485760")
	t.Setenv("DRIVE_IO_LIMITS_PROVIDER_BINARY_MAX_BYTES", "524288000")
	t.Setenv("DRIVE_IO_LIMITS_OAUTH_RESPONSE_MAX_BYTES", "65536")
	t.Setenv("DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE", "600")
	t.Setenv("DRIVE_SAVE_PROVIDER_URL_PREFIX", "https://drive.google.com/file/d")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID", "")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET", "")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL", "http://127.0.0.1:40001/v1/connectors/drive/oauth/callback")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL", "https://accounts.google.com")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_API_BASE_URL", "https://www.googleapis.com")
	t.Setenv("DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS", `["https://www.googleapis.com/auth/drive.file","https://www.googleapis.com/auth/drive.readonly"]`)
	t.Setenv("PHOTOS_ENABLED", "true")
	t.Setenv("PHOTOS_SCAN_PARALLELISM", "2")
	t.Setenv("PHOTOS_SCAN_BATCH_SIZE", "50")
	t.Setenv("PHOTOS_SCAN_MAX_FILE_SIZE_BYTES", "52428800")
	t.Setenv("PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS", "2500")
	t.Setenv("PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES", "10485760")
	t.Setenv("PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES", "104857600")
	t.Setenv("PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES", "26214400")
	t.Setenv("PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD", "0.8")
	t.Setenv("PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD", "0.92")
	t.Setenv("PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD", "0.75")
	t.Setenv("PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS", "600")
	t.Setenv("PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS", "86400")
	t.Setenv("PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS", "86400")
	t.Setenv("PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES", "8388608")
	t.Setenv("PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE", "50")
	t.Setenv("PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_EMBED_MODEL", "nomic-embed-text")
	t.Setenv("PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_OCR_MODEL", "deepseek-ocr:3b")
	t.Setenv("PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR", "2")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_ENABLED", "false")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_BASE_URL", "")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_API_KEY", "")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS", "300")
	t.Setenv("PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS", `["v1"]`)
	t.Setenv("PHOTOS_PROVIDER_PHOTOPRISM_ENABLED", "false")
	t.Setenv("PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL", "")
	t.Setenv("PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN", "")
	t.Setenv("PHOTOS_PROVIDER_PHOTOPRISM_POLL_INTERVAL_SECONDS", "600")
	t.Setenv("PHOTOS_PROVIDER_PHOTOPRISM_SUPPORTED_API_VERSIONS", `["v1"]`)
	t.Setenv("RECOMMENDATIONS_ENABLED", "true")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED", "false")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_CATEGORIES", `["place","event"]`)
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY", "")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_QUOTA_WINDOW_SECONDS", "60")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_MAX_REQUESTS_PER_WINDOW", "30")
	t.Setenv("RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ATTRIBUTION_LABEL", "Google Places")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_ENABLED", "false")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_CATEGORIES", `["place"]`)
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_API_KEY", "")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_QUOTA_WINDOW_SECONDS", "60")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_MAX_REQUESTS_PER_WINDOW", "30")
	t.Setenv("RECOMMENDATIONS_PROVIDER_YELP_ATTRIBUTION_LABEL", "Yelp")
	t.Setenv("RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD", "city")
	t.Setenv("RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD", "neighborhood")
	t.Setenv("RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD", "neighborhood")
	t.Setenv("RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM", "geohash")
	t.Setenv("RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL", "6")
	t.Setenv("RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW", "3")
	t.Setenv("RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS", "86400")
	t.Setenv("RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND", `{"place":86400,"product":604800,"deal":86400,"event":86400,"content":86400}`)
	t.Setenv("RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY", `{"enabled":true,"start":"22:00","end":"07:00"}`)
	t.Setenv("RECOMMENDATIONS_WATCHES_POLL_CRON", "*/5 * * * *")
	t.Setenv("RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS", "604800")
	t.Setenv("RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS", "2592000")
	t.Setenv("RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER", "25")
	t.Setenv("RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS", "10")
	t.Setenv("RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT", "5")
	t.Setenv("RECOMMENDATIONS_RANKING_STANDARD_STYLE", "balanced")
	t.Setenv("RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD", "0.55")
	t.Setenv("RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED", "false")
	t.Setenv("RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES", `["adult","gambling","weapons"]`)
	t.Setenv("RECOMMENDATIONS_POLICY_SAFETY_SOURCES", `["local-policy","provider-rating"]`)
	t.Setenv("RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED", "true")
	t.Setenv("RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED", "true")
	t.Setenv("RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED", "true")

	// Spec 044 — Per-User Bearer Auth Foundation. Defaults mirror dev/test
	// (Auth.Enabled=false, secret-bearing fields empty). Production-mode
	// fail-loud paths are exercised by dedicated TestValidate_AuthConfig_*
	// cases that override SMACKEREL_ENV=production AND AUTH_ENABLED=true.
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("AUTH_TOKEN_FORMAT", "paseto-v4-public")
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "")
	t.Setenv("AUTH_SIGNING_PRIOR_PUBLIC_KEY", "")
	t.Setenv("AUTH_SIGNING_PRIOR_KEY_ID", "")
	t.Setenv("AUTH_TOKEN_TTL_HOURS", "720")
	t.Setenv("AUTH_ROTATION_GRACE_WINDOW_HOURS", "168")
	t.Setenv("AUTH_CLOCK_SKEW_TOLERANCE_SECONDS", "30")
	t.Setenv("AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS", "30")
	t.Setenv("AUTH_REVOCATION_NATS_SUBJECT", "auth.revocations")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "")
	t.Setenv("AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED", "false")
	t.Setenv("AUTH_TELEMETRY_ENABLED", "true")
	t.Setenv("AUTH_TELEMETRY_METRIC_PREFIX", "smackerel_auth")
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")
	// BUG-020-009 — HTTP timeouts required and > 0. Test defaults match
	// the pre-fix literals so unrelated tests stay numerically stable;
	// dedicated TestBUG020009_* cases override with adversarial values.
	t.Setenv("FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS", "10")
	t.Setenv("AUTH_OAUTH_HTTP_TIMEOUT_SECONDS", "15")

	// Spec 045 FR-045-001 / FR-045-002 — deploy resource envelope and
	// ML model memory profile. Defaults mirror smackerel.yaml so test
	// loads succeed; per-test overrides exercise fail-loud paths.
	t.Setenv("POSTGRES_CPU_LIMIT", "1.0")
	t.Setenv("POSTGRES_MEMORY_LIMIT", "1G")
	t.Setenv("NATS_CPU_LIMIT", "0.5")
	t.Setenv("NATS_MEMORY_LIMIT", "512M")
	t.Setenv("CORE_CPU_LIMIT", "2.0")
	t.Setenv("CORE_MEMORY_LIMIT", "1G")
	t.Setenv("ML_CPU_LIMIT", "2.0")
	t.Setenv("ML_MEMORY_LIMIT", "3G")
	t.Setenv("OLLAMA_CPU_LIMIT", "4.0")
	t.Setenv("OLLAMA_MEMORY_LIMIT", "8G")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON", `[{"model":"llama3.2","memory_mib":2048},{"model":"all-MiniLM-L6-v2","memory_mib":256},{"model":"gpt-4o-mini","memory_mib":512},{"model":"gemma4:26b","memory_mib":3072},{"model":"nomic-embed-text","memory_mib":256},{"model":"deepseek-ocr:3b","memory_mib":2560}]`)

	// BUG-045-001 — Per-service envelope routing requires the SST
	// emission of every ollama-routed and ml-sidecar-routed model env
	// var. Defaults below mirror smackerel.yaml HEAD so tests run
	// green; per-test overrides exercise the per-bucket fail-loud
	// paths in validateModelEnvelopes(). OLLAMA_OCR_MODEL,
	// OLLAMA_REASONING_MODEL, and OLLAMA_FAST_MODEL are intentionally
	// absent below because config.sh does not emit them today.
	t.Setenv("OLLAMA_VISION_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_EMBED_MODEL", "nomic-embed-text")
	t.Setenv("PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "gemma4:26b")
	t.Setenv("PHOTOS_INTELLIGENCE_OCR_MODEL", "deepseek-ocr:3b")
	t.Setenv("AGENT_PROVIDER_DEFAULT_MODEL", "gemma4:26b")
	t.Setenv("AGENT_PROVIDER_REASONING_MODEL", "gemma4:26b")
	t.Setenv("AGENT_PROVIDER_FAST_MODEL", "gemma4:26b")
	t.Setenv("AGENT_PROVIDER_VISION_MODEL", "gemma4:26b")
	t.Setenv("AGENT_PROVIDER_OCR_MODEL", "deepseek-ocr:3b")

	// Spec 046 FR-046-001 / FR-046-002 / FR-046-003 — NATS production
	// hardening envelope. Defaults mirror smackerel.yaml so test loads
	// succeed; per-test overrides exercise fail-loud paths.
	t.Setenv("NATS_MAX_RECONNECT_ATTEMPTS", "-1")
	t.Setenv("NATS_RECONNECT_TIME_WAIT_SECONDS", "2")
	t.Setenv("NATS_MAX_PAYLOAD_BYTES", "8388608")
	t.Setenv("NATS_MAX_FILE_STORE_BYTES", "10737418240")
	t.Setenv("NATS_MAX_MEM_STORE_BYTES", "1073741824")
	t.Setenv("NATS_STREAM_MAX_BYTES_JSON", `[{"stream":"ARTIFACTS","bytes":1073741824},{"stream":"SEARCH","bytes":536870912},{"stream":"DIGEST","bytes":268435456},{"stream":"KEEP","bytes":268435456},{"stream":"INTELLIGENCE","bytes":536870912},{"stream":"ALERTS","bytes":134217728},{"stream":"SYNTHESIS","bytes":536870912},{"stream":"DOMAIN","bytes":268435456},{"stream":"DRIVE","bytes":536870912},{"stream":"PHOTOS","bytes":1073741824},{"stream":"ANNOTATIONS","bytes":134217728},{"stream":"LISTS","bytes":134217728},{"stream":"AGENT","bytes":268435456},{"stream":"WEATHER","bytes":67108864},{"stream":"DEADLETTER","bytes":67108864}]`)

	// Spec 048 — Backup and Restore Automation. Defaults mirror
	// smackerel.yaml so test Load() succeeds; per-test overrides
	// exercise fail-loud paths.
	t.Setenv("BACKUP_LOCAL_DIR", "./backups")
	t.Setenv("BACKUP_STATUS_FILE", "./backups/.backup-status.json")
	t.Setenv("BACKUP_RETENTION_DAILY", "7")
	t.Setenv("BACKUP_RETENTION_WEEKLY", "4")
	t.Setenv("BACKUP_WATCHER_POLL_SECONDS", "60")

	// Spec 061 SCOPE-01 — Conversational Assistant SST envelope.
	// Defaults mirror smackerel.yaml so test Load() succeeds; per-test
	// overrides exercise the fail-loud + rule-based validation paths in
	// loadAssistantConfig / validateAssistantConfig.
	t.Setenv("ASSISTANT_ENABLED", "true")
	t.Setenv("ASSISTANT_BORDERLINE_FLOOR", "0.75")
	t.Setenv("ASSISTANT_CONTEXT_WINDOW_TURNS", "8")
	t.Setenv("ASSISTANT_CONTEXT_IDLE_TIMEOUT", "30m")
	t.Setenv("ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL", "5m")
	t.Setenv("ASSISTANT_CONTEXT_STATE_KEY", "transport_user")
	t.Setenv("ASSISTANT_SOURCES_MAX", "5")
	t.Setenv("ASSISTANT_BODY_MAX_CHARS", "4000")
	t.Setenv("ASSISTANT_STATUS_MAX_DURATION", "60s")
	t.Setenv("ASSISTANT_DISAMBIGUATE_TIMEOUT", "2m")
	t.Setenv("ASSISTANT_ERROR_CAPTURE_TIMEOUT", "10s")
	t.Setenv("ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM", "30")
	t.Setenv("ASSISTANT_RATE_LIMIT_WEATHER_RPM", "20")
	t.Setenv("ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM", "10")
	t.Setenv("ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM", "20")
	t.Setenv("ASSISTANT_SKILLS_RETRIEVAL_ENABLED", "true")
	t.Setenv("ASSISTANT_SKILLS_RETRIEVAL_TOP_K", "8")
	t.Setenv("ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED", "true")
	t.Setenv("ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K", "8")
	t.Setenv("ASSISTANT_SKILLS_WEATHER_ENABLED", "false")
	t.Setenv("ASSISTANT_SKILLS_WEATHER_PROVIDER", "open-meteo")
	t.Setenv("ASSISTANT_SKILLS_WEATHER_API_KEY_REF", "")
	t.Setenv("ASSISTANT_SKILLS_WEATHER_CACHE_TTL", "10m")
	// Spec 061 design §18.3 — external-provider URL injection seam.
	t.Setenv("ASSISTANT_SKILLS_WEATHER_GEOCODE_URL", "https://geocoding-api.open-meteo.com/v1/search")
	t.Setenv("ASSISTANT_SKILLS_WEATHER_FORECAST_URL", "https://api.open-meteo.com/v1/forecast")
	t.Setenv("ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED", "false")
	t.Setenv("ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT", "5m")
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED", "true")
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE", "MarkdownV2")
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS", "4096")
	// Spec 061 SCOPE-05 design §17 — Telegram webhook mode keys. The
	// default test fixture uses long_poll so existing tests do not
	// need a resolved webhook secret. The webhook-specific tests
	// under TestValidateAssistant_*Webhook* override mode + ref + the
	// resolved env var explicitly.
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_MODE", "long_poll")
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF", "")
	t.Setenv("ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH", "/v1/telegram/webhook")
	// Spec 061 SCOPE-10 design §13 item 6 — offline evaluation
	// acceptance thresholds. Defaults mirror smackerel.yaml so existing
	// connector / hospitable / telegram tests don't trip the assistant
	// SST validator while exercising unrelated config surfaces.
	t.Setenv("ASSISTANT_EVAL_ROUTING_ACCURACY_MIN", "0.85")
	t.Setenv("ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN", "1.0")
	// Spec 061 SCOPE-09a — OTel SDK substrate SST. Defaults mirror
	// smackerel.yaml so unrelated tests don't trip the new validator.
	t.Setenv("ASSISTANT_OBSERVABILITY_OTEL_ENABLED", "false")
	t.Setenv("ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT", "")
	t.Setenv("ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME", "smackerel-core")
	// BUG-061-004 — routing embedder SST defaults for shared fixture.
	t.Setenv("ASSISTANT_ROUTING_EMBEDDER_MODE", "sidecar")
	t.Setenv("ASSISTANT_ROUTING_EMBED_TIMEOUT_MS", "500")
	// Spec 061 SCOPE-01 design §7.2 rule #2 — agent routing floor is
	// referenced by validateAssistantConfig for the borderline check.
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	t.Setenv("NOTIFICATION_INTELLIGENCE_ENABLED", "true")
	t.Setenv("NOTIFICATION_PERSISTENCE_THRESHOLD", "2")
	t.Setenv("NOTIFICATION_ESCALATION_SEVERITY", "high")
	t.Setenv("NOTIFICATION_LOW_CONFIDENCE_THRESHOLD", "0.55")
	t.Setenv("NOTIFICATION_MAX_RETRIES", "2")
	t.Setenv("NOTIFICATION_OUTPUT_CHANNELS", `["dashboard"]`)
	t.Setenv("NTFY_SOURCES_JSON", `[]`)

	// BUG-020-008 — the 8 SST-required int env vars previously routed
	// through the silent-default parseIntEnv helper. Defaults mirror
	// config/smackerel.yaml so test Load() succeeds; per-test overrides
	// exercise the fail-loud paths in mustParseIntEnv / Validate().
	t.Setenv("BOOKMARKS_MIN_URL_LENGTH", "10")
	t.Setenv("BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS", "30")
	t.Setenv("BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD", "3")
	t.Setenv("BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY", "5")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "1")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "25")
	t.Setenv("HOSPITABLE_INITIAL_LOOKBACK_DAYS", "90")
	t.Setenv("HOSPITABLE_PAGE_SIZE", "100")

	// Spec 058 Scope 1 — Chrome Extension Bridge ingest SST. Defaults
	// mirror config/smackerel.yaml so test Load() succeeds; dedicated
	// TestExtensionIngestConfig_Validate_* cases exercise the fail-loud
	// paths against each missing field.
	t.Setenv("EXTENSION_INGEST_ENABLED", "true")
	t.Setenv("EXTENSION_INGEST_MAX_BATCH_ITEMS", "256")
	t.Setenv("EXTENSION_INGEST_MAX_BODY_BYTES", "1048576")
	t.Setenv("EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS", "1800")
	t.Setenv("EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES", `["bookmark","browser_history_visit"]`)
	t.Setenv("EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE", "extension:bookmarks,history")

	// Spec 021 Scope 4 — Unified surfacing controller SST baselines.
	t.Setenv("SURFACING_DAILY_NUDGE_BUDGET", "5")
	t.Setenv("SURFACING_SUPPRESSION_WINDOW_HOURS", "4")
	t.Setenv("SURFACING_DEDUPE_WINDOW_HOURS", "6")
	t.Setenv("SURFACING_URGENT_ESCALATION_ENABLED", "true")

	// Spec 064 SCOPE-03 — open-ended knowledge agent SST baselines.
	// Defaults mirror config/smackerel.yaml (enabled=false ships) so
	// Load() succeeds; dedicated openknowledge_test.go cases exercise
	// the fail-loud paths against each missing/invalid field.
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_ENABLED", "false")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER", "searxng")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT", "")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY", "")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID", "")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS", "8")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET", "8000")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET", "0.05")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD", "0")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD", "0")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST", `[]`)
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED", "false")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS", "30000")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS", `[]`)
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD", "5")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS", "60")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS", "30")
	t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE", "shadow")
	t.Setenv("ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR", "0.6")
	t.Setenv("ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED", "true")

	// Spec 068 SCOPE-1 — Structured Intent Compiler SST baselines.
	// Defaults mirror config/smackerel.yaml so Load() succeeds; the
	// dedicated TestIntentCompilerConfigRequiresEverySSTKey exercises
	// the fail-loud paths against each missing/invalid field.
	t.Setenv("ASSISTANT_INTENT_COMPILER_ENABLED", "false")
	t.Setenv("ASSISTANT_INTENT_COMPILER_MODEL_ROLE", "assistant_intent_compiler")
	t.Setenv("ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION", "intent-compiler-v1")
	t.Setenv("ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION", "v1")
	t.Setenv("ASSISTANT_INTENT_COMPILER_TIMEOUT_MS", "5000")
	t.Setenv("ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR", "0.6")
	t.Setenv("ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS", "8")
	t.Setenv("ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES", "16384")
	t.Setenv("ASSISTANT_INTENT_COMPILER_RETRY_BUDGET", "1")

	// Spec 072 SCOPE-1 — WhatsApp Business Cloud API transport SST.
	// Defaults mirror config/smackerel.yaml (enabled=false). All keys
	// are permissively loaded when disabled; dedicated WhatsApp tests
	// override individual values to exercise fail-loud paths.
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED", "false")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH", "/v1/whatsapp/webhook")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION", "")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE", "30")
	t.Setenv("ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS", "4096")

	// Spec 069 SCOPE-1c-bis — HTTP transport SST. Defaults mirror
	// config/smackerel.yaml so unrelated tests Load() cleanly; the
	// dedicated TestAssistantHTTPTransportConfig* cases exercise the
	// fail-loud paths.
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_ENABLED", "true")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION", "v1")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES", "65536")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE", "60")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS", "86400")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE", "assistant:turn")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID", "shared")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST", "web,mobile,bridge")

	// Spec 074 SCOPE-1 — capture-as-fallback policy SST. Inviolable
	// foundation: values must always be present (no enable flag).
	// Defaults mirror config/smackerel.yaml.
	t.Setenv("CAPTURE_AS_FALLBACK_DEDUP_WINDOW", "24h")
	t.Setenv("CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT", "10m")
	t.Setenv("CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY", NormalizationPolicyV1)
	t.Setenv("CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY", "test-dedup-hash-key")
	t.Setenv("CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS", "90")

	// Spec 075 — legacy retirement telemetry SST. Defaults mirror
	// config/smackerel.yaml so unrelated tests can Load() cleanly;
	// dedicated TestLegacyRetirement_* cases exercise per-field
	// fail-loud paths.
	t.Setenv("LEGACY_RETIREMENT_WINDOW_ID", "test-window")
	t.Setenv("LEGACY_RETIREMENT_WINDOW_STATE", "open")
	t.Setenv("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS", "5.0")
	t.Setenv("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE", "3")
	t.Setenv("LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS", "30")
	t.Setenv("LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS", "7")
	t.Setenv("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY", "test-hmac-key")
	t.Setenv("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND", `{"/weather":"plain English now","/remind":"plain English now"}`)
	t.Setenv("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY", `{"/weather":"plain English now","/remind":"plain English now"}`)
	// Spec 076 SCOPE-6a — runtime wiring SST keys.
	t.Setenv("LEGACY_RETIREMENT_THRESHOLD_EVALUATOR_INTERVAL_SECONDS", "300")
	t.Setenv("LEGACY_RETIREMENT_OBSERVATION_CRON_EXPR", "0 4 * * *")
	t.Setenv("LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS", "100")

	// Spec 065 — assistant micro-tools SST.
	setAllAssistantToolsKeys(t)
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

// F-DEVOPS-009-002: Regression — bookmarks SST config fields must flow through env pipeline.
// This test would fail if BookmarksWatchInterval, BookmarksArchiveProcessed,
// BookmarksProcessingTier, BookmarksMinURLLength, or BookmarksExcludeDomains
// were reverted to hardcoded defaults instead of reading from env.
func TestLoad_BookmarksSST_AllConfigFieldsFlow(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BOOKMARKS_ENABLED", "true")
	t.Setenv("BOOKMARKS_IMPORT_DIR", "/data/bookmarks")
	t.Setenv("BOOKMARKS_SYNC_SCHEDULE", "*/15 * * * *")
	t.Setenv("BOOKMARKS_WATCH_INTERVAL", "10m")
	t.Setenv("BOOKMARKS_ARCHIVE_PROCESSED", "true")
	t.Setenv("BOOKMARKS_PROCESSING_TIER", "standard")
	t.Setenv("BOOKMARKS_MIN_URL_LENGTH", "25")
	t.Setenv("BOOKMARKS_EXCLUDE_DOMAINS", "[\"example.com\",\"test.org\"]")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.BookmarksEnabled {
		t.Error("expected BookmarksEnabled=true")
	}
	if cfg.BookmarksImportDir != "/data/bookmarks" {
		t.Errorf("expected BookmarksImportDir=/data/bookmarks, got %q", cfg.BookmarksImportDir)
	}
	if cfg.BookmarksSyncSchedule != "*/15 * * * *" {
		t.Errorf("expected BookmarksSyncSchedule=*/15 * * * *, got %q", cfg.BookmarksSyncSchedule)
	}
	if cfg.BookmarksWatchInterval != "10m" {
		t.Errorf("expected BookmarksWatchInterval=10m, got %q", cfg.BookmarksWatchInterval)
	}
	if !cfg.BookmarksArchiveProcessed {
		t.Error("expected BookmarksArchiveProcessed=true")
	}
	if cfg.BookmarksProcessingTier != "standard" {
		t.Errorf("expected BookmarksProcessingTier=standard, got %q", cfg.BookmarksProcessingTier)
	}
	if cfg.BookmarksMinURLLength != 25 {
		t.Errorf("expected BookmarksMinURLLength=25, got %d", cfg.BookmarksMinURLLength)
	}
	if cfg.BookmarksExcludeDomains != "[\"example.com\",\"test.org\"]" {
		t.Errorf("expected BookmarksExcludeDomains JSON, got %q", cfg.BookmarksExcludeDomains)
	}
}

// BUG-020-008 — BOOKMARKS_MIN_URL_LENGTH is now SST-required and fail-loud.
// Pre-fix this test pinned the silent-default-to-0 behavior, which WAS the
// bug. Post-fix the contract is: an unset env var MUST cause Load() to
// return an error naming the key, not silently return cfg.BookmarksMinURLLength=0.
func TestLoad_BookmarksMinURLLength_MissingEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BOOKMARKS_MIN_URL_LENGTH", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when BOOKMARKS_MIN_URL_LENGTH is unset (BUG-020-008)")
	}
	if !strings.Contains(err.Error(), "BOOKMARKS_MIN_URL_LENGTH") {
		t.Errorf("error should name BOOKMARKS_MIN_URL_LENGTH, got: %v", err)
	}
}

func TestLoad_GuestHostConnectorFields(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUESTHOST_ENABLED", "true")
	t.Setenv("GUESTHOST_BASE_URL", "https://myhost.example.com")
	t.Setenv("GUESTHOST_API_KEY", "tkn_test123")
	t.Setenv("GUESTHOST_SYNC_SCHEDULE", "*/5 * * * *")
	t.Setenv("GUESTHOST_EVENT_TYPES", "booking.created,review.received")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.GuestHostEnabled {
		t.Error("expected GuestHostEnabled=true")
	}
	if cfg.GuestHostBaseURL != "https://myhost.example.com" {
		t.Errorf("expected GuestHostBaseURL, got %q", cfg.GuestHostBaseURL)
	}
	if cfg.GuestHostAPIKey != "tkn_test123" {
		t.Errorf("expected GuestHostAPIKey, got %q", cfg.GuestHostAPIKey)
	}
	if cfg.GuestHostSyncSchedule != "*/5 * * * *" {
		t.Errorf("expected GuestHostSyncSchedule, got %q", cfg.GuestHostSyncSchedule)
	}
	if cfg.GuestHostEventTypes != "booking.created,review.received" {
		t.Errorf("expected GuestHostEventTypes, got %q", cfg.GuestHostEventTypes)
	}
}

func TestLoad_GuestHostConnectorFieldsOptional(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GuestHostEnabled {
		t.Error("expected GuestHostEnabled=false when GUESTHOST_ENABLED is unset")
	}
	if cfg.GuestHostBaseURL != "" {
		t.Errorf("expected empty GuestHostBaseURL, got %q", cfg.GuestHostBaseURL)
	}
	if cfg.GuestHostAPIKey != "" {
		t.Errorf("expected empty GuestHostAPIKey, got %q", cfg.GuestHostAPIKey)
	}
}

// --- Hospitable SST regression tests ---

func TestLoad_HospitableEnabled_MissingLookbackDays_Fails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HOSPITABLE_ENABLED", "true")
	t.Setenv("HOSPITABLE_ACCESS_TOKEN", "tok-test")
	t.Setenv("HOSPITABLE_BASE_URL", "https://api.hospitable.com")
	t.Setenv("HOSPITABLE_SYNC_SCHEDULE", "0 */2 * * *")
	t.Setenv("HOSPITABLE_INITIAL_LOOKBACK_DAYS", "") // explicitly unset (BUG-020-008: fail-loud)
	t.Setenv("HOSPITABLE_PAGE_SIZE", "100")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when HOSPITABLE_INITIAL_LOOKBACK_DAYS is missing and Hospitable is enabled")
	}
	if !strings.Contains(err.Error(), "HOSPITABLE_INITIAL_LOOKBACK_DAYS") {
		t.Errorf("error should name HOSPITABLE_INITIAL_LOOKBACK_DAYS, got: %v", err)
	}
}

func TestLoad_HospitableEnabled_MissingPageSize_Fails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HOSPITABLE_ENABLED", "true")
	t.Setenv("HOSPITABLE_ACCESS_TOKEN", "tok-test")
	t.Setenv("HOSPITABLE_BASE_URL", "https://api.hospitable.com")
	t.Setenv("HOSPITABLE_SYNC_SCHEDULE", "0 */2 * * *")
	t.Setenv("HOSPITABLE_INITIAL_LOOKBACK_DAYS", "90")
	t.Setenv("HOSPITABLE_PAGE_SIZE", "") // explicitly unset (BUG-020-008: fail-loud)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when HOSPITABLE_PAGE_SIZE is missing and Hospitable is enabled")
	}
	if !strings.Contains(err.Error(), "HOSPITABLE_PAGE_SIZE") {
		t.Errorf("error should name HOSPITABLE_PAGE_SIZE, got: %v", err)
	}
}

func TestLoad_HospitableDisabled_MissingNumericFields_OK(t *testing.T) {
	setRequiredEnv(t)
	// HOSPITABLE_ENABLED not set (defaults to false) — numeric fields not required
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HospitableEnabled {
		t.Error("expected HospitableEnabled=false when unset")
	}
}

func TestLoad_HospitableSyncFlags_RequireExplicitTrue(t *testing.T) {
	setRequiredEnv(t)
	// When sync flag env vars are UNSET, they must default to false (not true)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HospitableSyncProperties {
		t.Error("HospitableSyncProperties should be false when HOSPITABLE_SYNC_PROPERTIES is unset")
	}
	if cfg.HospitableSyncReservations {
		t.Error("HospitableSyncReservations should be false when HOSPITABLE_SYNC_RESERVATIONS is unset")
	}
	if cfg.HospitableSyncMessages {
		t.Error("HospitableSyncMessages should be false when HOSPITABLE_SYNC_MESSAGES is unset")
	}
	if cfg.HospitableSyncReviews {
		t.Error("HospitableSyncReviews should be false when HOSPITABLE_SYNC_REVIEWS is unset")
	}
}

// --- Spec 044 — Per-User Bearer Auth Foundation production-mode validation ---
//
// T1-01..T1-03 — fail-loud cases for production-mode auth configuration.
// SCN-AUTH-006 (production startup with empty signing material MUST refuse
// to start) and design.md §4 SST validation contract. Each test starts from
// a valid baseline (SMACKEREL_ENV=production, AUTH_ENABLED=true, all keys
// populated) and then clears one key to prove the loader names it in the
// returned error.

// setProductionAuthBaseline upgrades the dev-test setRequiredEnv defaults
// to a valid production-mode auth configuration. Tests then knock out one
// field at a time to prove the loader fails loud with the expected name.
func setProductionAuthBaseline(t *testing.T) {
	t.Helper()
	t.Setenv("SMACKEREL_ENV", "production")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "production-shared-token-baseline")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "k4.secret.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "key-2026-05")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "hmac-secret-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
}

// T1-01 — production-mode missing signing key fails loud.
func TestValidate_AuthConfig_FailsLoudOnMissingSigningKey_Production(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_SIGNING_ACTIVE_PRIVATE_KEY is empty in production with auth.enabled=true")
	}
	if !strings.Contains(err.Error(), "AUTH_SIGNING_ACTIVE_PRIVATE_KEY") {
		t.Errorf("error should name AUTH_SIGNING_ACTIVE_PRIVATE_KEY, got: %v", err)
	}
	if !strings.Contains(err.Error(), "production") {
		t.Errorf("error should mention production, got: %v", err)
	}
}

// T1-02 — production-mode missing at-rest hashing key fails loud.
func TestValidate_AuthConfig_FailsLoudOnMissingHashingKey_Production(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_AT_REST_HASHING_KEY is empty in production with auth.enabled=true")
	}
	if !strings.Contains(err.Error(), "AUTH_AT_REST_HASHING_KEY") {
		t.Errorf("error should name AUTH_AT_REST_HASHING_KEY, got: %v", err)
	}
}

// T1-03 — invalid (out-of-range) rotation grace window fails loud regardless
// of environment. NFR-AUTH-003 floor: ≥ 24 hours.
func TestValidate_AuthConfig_FailsLoudOnInvalidGraceWindow(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AUTH_ROTATION_GRACE_WINDOW_HOURS", "12")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_ROTATION_GRACE_WINDOW_HOURS < 24")
	}
	if !strings.Contains(err.Error(), "AUTH_ROTATION_GRACE_WINDOW_HOURS") {
		t.Errorf("error should name AUTH_ROTATION_GRACE_WINDOW_HOURS, got: %v", err)
	}
	if !strings.Contains(err.Error(), "24") {
		t.Errorf("error should mention the 24-hour floor, got: %v", err)
	}
}

// Additional T1-01 hardening — production-mode missing key id also fails.
func TestValidate_AuthConfig_FailsLoudOnMissingKeyID_Production(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_SIGNING_ACTIVE_KEY_ID is empty in production with auth.enabled=true")
	}
	if !strings.Contains(err.Error(), "AUTH_SIGNING_ACTIVE_KEY_ID") {
		t.Errorf("error should name AUTH_SIGNING_ACTIVE_KEY_ID, got: %v", err)
	}
}

// Additional T1-02 hardening — at-rest key MUST differ from signing key.
func TestValidate_AuthConfig_RejectsHashingKeyEqualsSigningKey_Production(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	shared := "k4.secret.SHARED-SECRET-VALUE-USED-FOR-BOTH-FIELDS-WHICH-IS-A-BUG"
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", shared)
	t.Setenv("AUTH_AT_REST_HASHING_KEY", shared)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_AT_REST_HASHING_KEY == AUTH_SIGNING_ACTIVE_PRIVATE_KEY")
	}
	if !strings.Contains(err.Error(), "OQ-8") && !strings.Contains(err.Error(), "differ") {
		t.Errorf("error should call out the OQ-8 separation requirement, got: %v", err)
	}
}

// Production fail-loud is suppressed when auth.enabled=false (preserves the
// dev/test ergonomic; SCN-AUTH-005 / SCN-AUTH-011).
func TestValidate_AuthConfig_AllowsEmptyKeysWhenAuthDisabled_Production(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "production")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "production-shared-token-baseline")
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed in production when AUTH_ENABLED=false even with empty signing material, got: %v", err)
	}
}

// Dev/test environments allow empty signing material with auth.enabled=true
// (single-tenant ergonomic preserved while the operator is staging keys).
func TestValidate_AuthConfig_AllowsEmptyKeysInDev_AuthEnabled(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "development")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed in development with empty signing material, got: %v", err)
	}
}

// =============================================================================
// Spec 051 — Deployment Secret and Auth Contract
// =============================================================================
//
// SCN-051-S01 — bootstrap token is REQUIRED at config-load time when running
// in production with auth.enabled=true. The dev/test ergonomic of an empty
// bootstrap token is preserved.

// TestLoadAuthConfig_BootstrapTokenRequiredWithEnabledProduction proves that
// loadAuthConfig fails loud when AUTH_BOOTSTRAP_TOKEN is empty in production
// with auth.enabled=true (spec 051 FR-051-004 / SCN-051-S01).
func TestLoadAuthConfig_BootstrapTokenRequiredWithEnabledProduction(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	// Spec 051: bootstrap token is required in production with auth on.
	// setRequiredEnv already sets AUTH_BOOTSTRAP_TOKEN="" for dev/test;
	// setProductionAuthBaseline doesn't set it, so we explicitly clear
	// it here to make the test intent unambiguous.
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUTH_BOOTSTRAP_TOKEN is empty in production with auth.enabled=true")
	}
	if !strings.Contains(err.Error(), "AUTH_BOOTSTRAP_TOKEN") {
		t.Errorf("error should name AUTH_BOOTSTRAP_TOKEN, got: %v", err)
	}
	if !strings.Contains(err.Error(), "spec 051") {
		t.Errorf("error should reference spec 051, got: %v", err)
	}
}

// TestLoadAuthConfig_BootstrapTokenAcceptedInDev proves the dev/test
// ergonomic is preserved (empty bootstrap token is allowed when
// SMACKEREL_ENV is not "production").
func TestLoadAuthConfig_BootstrapTokenAcceptedInDev(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "development")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed in development with empty AUTH_BOOTSTRAP_TOKEN, got: %v", err)
	}
}

// TestLoadAuthConfig_BootstrapTokenAcceptedWhenAuthDisabled proves the
// production-mode gate does not fire when auth.enabled=false (matches
// the existing AllowsEmptyKeysWhenAuthDisabled_Production behavior so
// operators can run a no-auth production deployment for ops backstop
// scenarios). Bootstrap token is meaningless without auth enabled.
func TestLoadAuthConfig_BootstrapTokenAcceptedWhenAuthDisabled(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "production")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "production-shared-token-baseline")
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "")
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed in production with auth.enabled=false even with empty AUTH_BOOTSTRAP_TOKEN, got: %v", err)
	}
}

// SCN-051-S02 — defense-in-depth: even if the SST loader misses a dev-default
// Postgres password, runtime Validate() rejects it when SMACKEREL_ENV is
// "production". Dev/test environments retain the convenient default.

// TestValidate_RejectsDevDBPassword_Production proves runtime rejection of
// the dev-default Postgres password (spec 051 FR-051-005 / SCN-051-S02).
func TestValidate_RejectsDevDBPassword_Production(t *testing.T) {
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "real-bootstrap-token-not-a-default")
	// Construct a DATABASE_URL whose password component is a known dev
	// default. The build-up keeps gitleaks scanners satisfied (no
	// inline-credential URL literal is committed to the source tree).
	devPassword := DevDBPasswords[0] // "smackerel"
	dbURL := "postgres://" + "smackerel" + ":" + devPassword + "@localhost:5432/smackerel"
	t.Setenv("DATABASE_URL", dbURL)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL password is a known dev-default in production")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name DATABASE_URL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "spec 051") {
		t.Errorf("error should reference spec 051, got: %v", err)
	}
	// FR-051-007 redaction contract: the error MUST NOT echo the
	// dev-default value as a free-standing token.
	if strings.Contains(err.Error(), devPassword) {
		t.Errorf("error MUST NOT echo the dev-default password value, got: %v", err)
	}
}

// TestValidate_AcceptsDevDBPasswordInDev proves the dev/test ergonomic is
// preserved (the dev-default password is allowed when SMACKEREL_ENV is not
// "production").
func TestValidate_AcceptsDevDBPasswordInDev(t *testing.T) {
	setRequiredEnv(t)
	// Test environment uses the same dev default.
	devPassword := DevDBPasswords[0]
	dbURL := "postgres://" + "smackerel" + ":" + devPassword + "@localhost:5432/smackerel"
	t.Setenv("DATABASE_URL", dbURL)

	_, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed in test with dev-default DATABASE_URL password, got: %v", err)
	}
}

// TestIsDevDBPassword_KnownValues proves IsDevDBPassword recognises every
// entry in DevDBPasswords (case-insensitive) and rejects empty / non-default
// values.
func TestIsDevDBPassword_KnownValues(t *testing.T) {
	for _, dev := range DevDBPasswords {
		if !IsDevDBPassword(dev) {
			t.Errorf("IsDevDBPassword(%q) = false, want true", dev)
		}
		// Case-insensitivity check.
		if !IsDevDBPassword(strings.ToUpper(dev)) {
			t.Errorf("IsDevDBPassword(%q) = false, want true (case-insensitive)", strings.ToUpper(dev))
		}
	}
	if IsDevDBPassword("") {
		t.Error("IsDevDBPassword(\"\") = true, want false")
	}
	if IsDevDBPassword("a-real-strong-password-not-a-default") {
		t.Error("IsDevDBPassword on a real password returned true")
	}
}

// TestExtractDatabasePassword_Shapes proves extractDatabasePassword handles
// the URL shapes Smackerel actually uses. Anything weird returns "".
func TestExtractDatabasePassword_Shapes(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"standard", "postgres://" + "user" + ":" + "secret" + "@host:5432/db", "secret"},
		{"with-query", "postgres://" + "user" + ":" + "secret" + "@host/db?sslmode=disable", "secret"},
		{"no-password", "postgres://" + "user" + "@host/db", ""},
		{"no-userinfo", "postgres://host/db", ""},
		{"empty", "", ""},
		{"no-scheme", "user:secret@host/db", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDatabasePassword(tc.url); got != tc.want {
				t.Errorf("extractDatabasePassword(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}
