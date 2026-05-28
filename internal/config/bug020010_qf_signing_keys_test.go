package config

import (
	"strings"
	"testing"
)

// BUG-020-010 — Adversarial regression: Config.Validate() MUST fail loud
// when QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON is set to invalid JSON,
// and MUST permit an empty value (PERMISSIVE policy — preserves the
// today-allowed "signing not configured in this environment" shape).
// SCN-SM-020-bug-010.

func TestBUG020010_ValidateFailsLoudOnMalformedSigningKeysJSON(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "https://qf.example.test")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "qf-service-token")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "*/5 * * * *")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "1")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "25")
	t.Setenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON", "not-valid-json")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error for malformed QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON")
	}
	if !strings.Contains(err.Error(), "QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON") {
		t.Fatalf("error should name the field: %v", err)
	}
}

func TestBUG020010_ValidateAllowsEmptySigningKeysJSON(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "https://qf.example.test")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "qf-service-token")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "*/5 * * * *")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "1")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "25")
	t.Setenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("PERMISSIVE policy violated: unexpected error for empty signing keys JSON: %v", err)
	}
	if cfg.QFDecisionsCallbackSigningKeysJSON != "" {
		t.Fatalf("QFDecisionsCallbackSigningKeysJSON: want empty, got %q", cfg.QFDecisionsCallbackSigningKeysJSON)
	}
}

func TestBUG020010_ConfigPopulatesSigningKeysJSONFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("QF_DECISIONS_ENABLED", "true")
	t.Setenv("QF_DECISIONS_BASE_URL", "https://qf.example.test")
	t.Setenv("QF_DECISIONS_CREDENTIAL_REF", "qf-service-token")
	t.Setenv("QF_DECISIONS_SYNC_SCHEDULE", "*/5 * * * *")
	t.Setenv("QF_DECISIONS_PACKET_VERSION", "1")
	t.Setenv("QF_DECISIONS_PAGE_SIZE", "25")
	validJSON := `[{"key_id":"bug020010-cfg","secret":"s","not_before":"2026-01-01T00:00:00Z"}]`
	t.Setenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON", validJSON)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QFDecisionsCallbackSigningKeysJSON != validJSON {
		t.Fatalf("QFDecisionsCallbackSigningKeysJSON: want %q, got %q", validJSON, cfg.QFDecisionsCallbackSigningKeysJSON)
	}
}
