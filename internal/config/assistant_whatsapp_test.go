// TP-072-04 — unit: enabled WhatsApp with missing access token (or
// any other required credential) fails loud. SCN-072-A06.

package config

import (
	"os"
	"strings"
	"testing"
)

// baseWhatsappCfg returns a Config with assistant.enabled=true and
// the WhatsApp transport enabled with the canonical happy-path SST
// values minus whichever *Ref the test case wants to drop.
func baseWhatsappCfg() *Config {
	return &Config{Assistant: AssistantConfig{
		Enabled:                  true,
		BorderlineFloor:          0.75,
		ContextStateKey:          "transport_user",
		TelegramEnabled:          true,
		TelegramMode:             "long_poll",
		TelegramWebhookSecretRef: "",
		TelegramWebhookPath:      "/v1/telegram/webhook",

		WhatsappEnabled:                   true,
		WhatsappWebhookPath:               "/v1/assistant/transports/whatsapp/webhook",
		WhatsappPhoneNumberID:             "pid-1",
		WhatsappBusinessAccountID:         "biz-1",
		WhatsappWebhookVerifyTokenRef:     "WA_VERIFY_TOKEN",
		WhatsappAppSecretRef:              "WA_APP_SECRET",
		WhatsappAccessTokenRef:            "WA_ACCESS_TOKEN",
		WhatsappIdentityHashKeyRef:        "WA_IDENTITY_HASH_KEY",
		WhatsappAPIBaseURL:                "https://graph.facebook.com",
		WhatsappAPIVersion:                "v20.0",
		WhatsappRateLimitPerUserPerMinute: 30,
		WhatsappMaxTextChars:              4096,
	}}
}

func setWhatsappSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("WA_VERIFY_TOKEN", "verify-tok")
	t.Setenv("WA_APP_SECRET", "app-secret")
	t.Setenv("WA_ACCESS_TOKEN", "access-token")
	t.Setenv("WA_IDENTITY_HASH_KEY", "hash-key")
}

func TestValidateAssistantConfig_Whatsapp_HappyPath(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	setWhatsappSecrets(t)
	cfg := baseWhatsappCfg()
	if err := cfg.validateAssistantConfig(); err != nil {
		t.Fatalf("happy-path WhatsApp config should validate, got: %v", err)
	}
	// Resolved secrets are mirrored onto the non-Ref fields.
	if cfg.Assistant.WhatsappAccessToken != "access-token" {
		t.Errorf("WhatsappAccessToken: want %q, got %q", "access-token", cfg.Assistant.WhatsappAccessToken)
	}
	if cfg.Assistant.WhatsappAppSecret != "app-secret" {
		t.Errorf("WhatsappAppSecret: want %q, got %q", "app-secret", cfg.Assistant.WhatsappAppSecret)
	}
	if cfg.Assistant.WhatsappWebhookVerifyToken != "verify-tok" {
		t.Errorf("WhatsappWebhookVerifyToken: want %q, got %q", "verify-tok", cfg.Assistant.WhatsappWebhookVerifyToken)
	}
	if cfg.Assistant.WhatsappIdentityHashKey != "hash-key" {
		t.Errorf("WhatsappIdentityHashKey: want %q, got %q", "hash-key", cfg.Assistant.WhatsappIdentityHashKey)
	}
}

func TestValidateAssistantConfig_Whatsapp_MissingAccessTokenFailsLoud(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	setWhatsappSecrets(t)
	// Unset the access-token env var that the *Ref points at — the
	// validator MUST refuse with a fail-loud, named error.
	_ = os.Unsetenv("WA_ACCESS_TOKEN")

	cfg := baseWhatsappCfg()
	err := cfg.validateAssistantConfig()
	if err == nil {
		t.Fatalf("expected fail-loud error for missing access_token, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "access_token_ref") {
		t.Errorf("error must name the failing SST key (access_token_ref); got: %v", err)
	}
	if !strings.Contains(msg, "WA_ACCESS_TOKEN") {
		t.Errorf("error must name the unresolved env var (WA_ACCESS_TOKEN); got: %v", err)
	}
}

func TestValidateAssistantConfig_Whatsapp_MissingRefFailsLoud(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	setWhatsappSecrets(t)
	cfg := baseWhatsappCfg()
	cfg.Assistant.WhatsappAccessTokenRef = "" // simulate empty SST key
	err := cfg.validateAssistantConfig()
	if err == nil {
		t.Fatalf("expected fail-loud error for empty access_token_ref, got nil")
	}
	if !strings.Contains(err.Error(), "ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF") {
		t.Errorf("error must name the missing SST env var; got: %v", err)
	}
}

func TestValidateAssistantConfig_Whatsapp_DisabledSkipsCredentialResolution(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	// All WhatsApp env vars intentionally unset.
	cfg := baseWhatsappCfg()
	cfg.Assistant.WhatsappEnabled = false
	cfg.Assistant.WhatsappPhoneNumberID = ""
	cfg.Assistant.WhatsappBusinessAccountID = ""
	cfg.Assistant.WhatsappWebhookVerifyTokenRef = ""
	cfg.Assistant.WhatsappAppSecretRef = ""
	cfg.Assistant.WhatsappAccessTokenRef = ""
	cfg.Assistant.WhatsappIdentityHashKeyRef = ""
	if err := cfg.validateAssistantConfig(); err != nil {
		t.Fatalf("disabled WhatsApp transport must skip credential checks, got: %v", err)
	}
}

func TestValidateAssistantConfig_Whatsapp_WebhookPathMustStartWithSlash(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	cfg := baseWhatsappCfg()
	cfg.Assistant.WhatsappEnabled = false // path rule applies even when disabled
	cfg.Assistant.WhatsappWebhookPath = "v1/no-leading-slash"
	err := cfg.validateAssistantConfig()
	if err == nil {
		t.Fatalf("expected fail-loud error for non-/ prefix, got nil")
	}
	if !strings.Contains(err.Error(), "ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH") {
		t.Errorf("error must name the failing key; got: %v", err)
	}
}

func TestValidateAssistantConfig_Whatsapp_APIBaseURLMustBeHTTPS(t *testing.T) {
	t.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.65")
	setWhatsappSecrets(t)
	cfg := baseWhatsappCfg()
	cfg.Assistant.WhatsappAPIBaseURL = "http://graph.facebook.com"
	err := cfg.validateAssistantConfig()
	if err == nil {
		t.Fatalf("expected fail-loud error for non-HTTPS api_base_url, got nil")
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("error must mention HTTPS requirement; got: %v", err)
	}
}
