// Adversarial regression tests for MIT-040-S-004 (spec 040 SST hardening).
//
// MIT-040-S-004 requires the runtime config loader to fail-loud when
// SMACKEREL_ENV=production AND SMACKEREL_AUTH_TOKEN is empty, while
// preserving the dev-mode warn-and-continue ergonomic when
// SMACKEREL_ENV=development|test. Each test below MUST FAIL if the
// production-environment guard or the SMACKEREL_ENV allowlist is removed
// from internal/config/config.go (the loader file).
package config

import (
	"strings"
	"testing"
)

// TestRuntimeConfig_S004_ProductionEnvFailsFastWhenAuthTokenEmpty asserts
// that Load() returns an error mentioning both "production" and
// "SMACKEREL_AUTH_TOKEN" when SMACKEREL_ENV=production and the auth token
// is empty. Adversarial proof: deleting the production-mode AUTH_TOKEN
// branch in Validate() makes this test fail (Load returns nil error).
func TestRuntimeConfig_S004_ProductionEnvFailsFastWhenAuthTokenEmpty(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "production")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when SMACKEREL_ENV=production and SMACKEREL_AUTH_TOKEN is empty")
	}
	msg := err.Error()
	if !strings.Contains(msg, "production") {
		t.Errorf("error should mention production, got: %v", err)
	}
	if !strings.Contains(msg, "SMACKEREL_AUTH_TOKEN") {
		t.Errorf("error should mention SMACKEREL_AUTH_TOKEN, got: %v", err)
	}
}

// TestRuntimeConfig_S004_DevelopmentEnvAllowsEmptyAuthTokenWithWarning
// asserts that Load() returns nil error when SMACKEREL_ENV=development and
// the auth token is empty. The runtime warn-and-continue path is exercised
// by configureLogging() (cmd/core/wiring.go) and the bearer middleware
// (internal/api/router.go) — covered by their own dedicated tests.
//
// Adversarial proof: making AUTH_TOKEN unconditionally required (i.e.
// removing the c.Environment == "production" branch in requiredVars())
// makes this test fail because Load() returns "missing required
// configuration: SMACKEREL_AUTH_TOKEN".
func TestRuntimeConfig_S004_DevelopmentEnvAllowsEmptyAuthTokenWithWarning(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "development")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected nil error in development env with empty token, got: %v", err)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected Environment=development, got %q", cfg.Environment)
	}
	if cfg.AuthToken != "" {
		t.Errorf("expected empty AuthToken to be preserved, got %q", cfg.AuthToken)
	}
}

// TestRuntimeConfig_S004_TestEnvAllowsEmptyAuthTokenWithWarning mirrors the
// development-env case for the test environment. This is what
// ./smackerel.sh test integration / e2e / stress rely on so the disposable
// stack continues to start with the dev-mode bypass under SMACKEREL_ENV=test.
//
// Adversarial proof: making AUTH_TOKEN unconditionally required (or
// restricting the dev-mode bypass to "development" only) makes this test
// fail.
func TestRuntimeConfig_S004_TestEnvAllowsEmptyAuthTokenWithWarning(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "test")
	t.Setenv("SMACKEREL_AUTH_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected nil error in test env with empty token, got: %v", err)
	}
	if cfg.Environment != "test" {
		t.Errorf("expected Environment=test, got %q", cfg.Environment)
	}
}

// TestRuntimeConfig_S004_UnknownEnvironmentValueIsFatal asserts that an
// unrecognized SMACKEREL_ENV value is rejected by the allowlist with both
// the offending value and the allowed-set in the error.
//
// Adversarial proof: removing the allowlist switch in Validate() makes
// this test fail (Load returns nil error for SMACKEREL_ENV=staging).
func TestRuntimeConfig_S004_UnknownEnvironmentValueIsFatal(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "staging")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unknown SMACKEREL_ENV value 'staging'")
	}
	msg := err.Error()
	if !strings.Contains(msg, "staging") {
		t.Errorf("error should mention the offending value 'staging', got: %v", err)
	}
	if !strings.Contains(msg, "development|test|production") {
		t.Errorf("error should mention the allowlist 'development|test|production', got: %v", err)
	}
}

// TestRuntimeConfig_S004_MissingEnvironmentIsFatal asserts that an unset
// SMACKEREL_ENV is treated as a configuration error. SMACKEREL_ENV is a
// fail-loud SST signal — there is no default.
func TestRuntimeConfig_S004_MissingEnvironmentIsFatal(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMACKEREL_ENV", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when SMACKEREL_ENV is unset")
	}
	if !strings.Contains(err.Error(), "SMACKEREL_ENV") {
		t.Errorf("error should name SMACKEREL_ENV, got: %v", err)
	}
}
