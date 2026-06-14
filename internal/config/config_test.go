package config

import (
	"strings"
	"testing"
)

// Spec 091 SCOPE-01 — loadAuthConfig must load WEB_REGISTRATION_INVITE_TOKEN
// from the environment as an OPTIONAL secret. Unlike AUTH_BOOTSTRAP_TOKEN it
// is NOT production-required: an empty value must NEVER add an authErrors
// entry, so registration is fail-loud-at-POST (the handler refuses), never
// fail-loud-at-boot. These tests drive the unexported loadAuthConfig directly
// (same package) with a fully-valid production auth baseline so the only
// variable under test is the invite token.

// setValidProductionAuthEnv wires a COMPLETE, valid production auth env so
// that loadAuthConfig(&Config{Environment: "production"}) returns nil. Each
// test then flips exactly one variable to isolate the property under test.
func setValidProductionAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_TOKEN_FORMAT", "paseto-v4-public")
	t.Setenv("AUTH_TOKEN_TTL_HOURS", "1")
	t.Setenv("AUTH_ROTATION_GRACE_WINDOW_HOURS", "24")
	t.Setenv("AUTH_CLOCK_SKEW_TOLERANCE_SECONDS", "30")
	t.Setenv("AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS", "60")
	t.Setenv("AUTH_REVOCATION_NATS_SUBJECT", "auth.revocations")
	t.Setenv("AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED", "false")
	t.Setenv("AUTH_TELEMETRY_ENABLED", "true")
	t.Setenv("AUTH_TELEMETRY_METRIC_PREFIX", "smackerel_auth")
	// Production-required secret material (all non-empty; the hashing key
	// differs from the signing key per spec 044 OQ-8).
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "k4.secret.PRODSIGNINGKEYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("AUTH_SIGNING_ACTIVE_KEY_ID", "key-2026-06")
	t.Setenv("AUTH_AT_REST_HASHING_KEY", "hmac-secret-PRODATRESTBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "prod-bootstrap-token-baseline")
	// Prior-key rotation slot left empty (both empty is a valid, consistent
	// state per the partial-rotation guard in loadAuthConfig).
	t.Setenv("AUTH_SIGNING_PRIOR_PUBLIC_KEY", "")
	t.Setenv("AUTH_SIGNING_PRIOR_KEY_ID", "")
}

// TestLoadAuthConfig_WebRegistrationInviteToken_LoadsFromEnv — the env value
// is copied verbatim into cfg.Auth.WebRegistrationInviteToken.
func TestLoadAuthConfig_WebRegistrationInviteToken_LoadsFromEnv(t *testing.T) {
	setValidProductionAuthEnv(t)
	t.Setenv("WEB_REGISTRATION_INVITE_TOKEN", "invite-secret-abc-123")

	cfg := &Config{Environment: "production"}
	if err := loadAuthConfig(cfg); err != nil {
		t.Fatalf("loadAuthConfig returned error with a valid production baseline + a set invite token: %v", err)
	}
	if got := cfg.Auth.WebRegistrationInviteToken; got != "invite-secret-abc-123" {
		t.Errorf("WebRegistrationInviteToken = %q, want %q (must load from WEB_REGISTRATION_INVITE_TOKEN)", got, "invite-secret-abc-123")
	}
}

// TestLoadAuthConfig_WebRegistrationInviteToken_EmptyIsOptional_ProductionBootSucceeds
// — an empty invite token in production+auth.enabled MUST still produce a
// valid Config (registration is disabled at POST, never a boot failure).
func TestLoadAuthConfig_WebRegistrationInviteToken_EmptyIsOptional_ProductionBootSucceeds(t *testing.T) {
	setValidProductionAuthEnv(t)
	t.Setenv("WEB_REGISTRATION_INVITE_TOKEN", "") // explicitly empty / unset

	cfg := &Config{Environment: "production"}
	if err := loadAuthConfig(cfg); err != nil {
		t.Fatalf("loadAuthConfig must NOT fail when WEB_REGISTRATION_INVITE_TOKEN is empty in "+
			"production+auth.enabled (it is OPTIONAL — empty = registration disabled at POST, "+
			"never a boot failure); got: %v", err)
	}
	if cfg.Auth.WebRegistrationInviteToken != "" {
		t.Errorf("WebRegistrationInviteToken = %q, want \"\" when unset", cfg.Auth.WebRegistrationInviteToken)
	}
}

// TestLoadAuthConfig_InviteTokenAbsentFromProductionAuthErrors — adversarial
// contrast proving the optional property has bite, not a tautology. With the
// SAME production baseline, an empty AUTH_BOOTSTRAP_TOKEN (which IS
// production-required) DOES fail boot and names AUTH_BOOTSTRAP_TOKEN, while
// the empty WEB_REGISTRATION_INVITE_TOKEN never appears in the error. This
// demonstrates the production gate is genuinely active in this config and that
// the invite token is deliberately excluded from it.
func TestLoadAuthConfig_InviteTokenAbsentFromProductionAuthErrors(t *testing.T) {
	setValidProductionAuthEnv(t)
	t.Setenv("WEB_REGISTRATION_INVITE_TOKEN", "") // empty OPTIONAL secret
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")          // empty REQUIRED secret → must fail

	cfg := &Config{Environment: "production"}
	err := loadAuthConfig(cfg)
	if err == nil {
		t.Fatal("expected loadAuthConfig to fail with an empty AUTH_BOOTSTRAP_TOKEN in production " +
			"(adversarial harness check — proves the production gate is active in this config)")
	}
	if !strings.Contains(err.Error(), "AUTH_BOOTSTRAP_TOKEN") {
		t.Errorf("production error must name the REQUIRED AUTH_BOOTSTRAP_TOKEN, got: %v", err)
	}
	if strings.Contains(err.Error(), "WEB_REGISTRATION_INVITE_TOKEN") {
		t.Errorf("the OPTIONAL WEB_REGISTRATION_INVITE_TOKEN must NEVER appear in the production "+
			"authErrors set, but the error mentioned it: %v", err)
	}
}
