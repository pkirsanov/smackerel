package config

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

// Spec 051 SCN-051-S03 / FR-051-007 — security-static log-redaction
// adversarial test. Every error path that could plausibly receive a
// secret value MUST refuse to echo the value. The test seeds each
// secret env var with a unique LEAKCANARY-* sentinel substring,
// drives the error path, and asserts:
//
//  1. The returned error names the offending KEY (so operators can act).
//  2. The returned error does NOT contain ANY sentinel substring.
//
// New secret env vars added to loadAuthConfig, Validate, or
// ValidateRuntimeAuthStartup MUST extend this test or a parallel
// dedicated test — adding a secret without a redaction-proof entry
// is a contract violation under spec 051.

const (
	canarySigningKey   = "LEAKCANARY-signing-key-9b72b1cd"     // gitleaks:allow — LEAKCANARY-* sentinel; not a real key
	canaryHashingKey   = "LEAKCANARY-hashing-key-21f9f49f"     // gitleaks:allow — LEAKCANARY-* sentinel; not a real key
	canaryBootstrap    = "LEAKCANARY-bootstrap-token-e0c14c34" // gitleaks:allow — LEAKCANARY-* sentinel; not a real token
	canaryAuthToken    = "LEAKCANARY-shared-token-5b8a3211"    // gitleaks:allow — LEAKCANARY-* sentinel; not a real token
	canaryDBPassword   = "LEAKCANARY-db-password-77c0a06e"     // gitleaks:allow — LEAKCANARY-* sentinel; not a real password
	canarySharedSecret = "LEAKCANARY-shared-secret-d3a5e8b1"   // gitleaks:allow — LEAKCANARY-* sentinel; not a real secret
	// canaryFullDBURL is built by tests on the fly via string concat
	// so gitleaks does not see an inline-credential URL literal.
)

// canaries returns the full set of sentinel substrings that MUST NOT
// appear in any error returned by config or auth startup paths.
func canaries() []string {
	return []string{
		canarySigningKey,
		canaryHashingKey,
		canaryBootstrap,
		canaryAuthToken,
		canaryDBPassword,
		canarySharedSecret,
	}
}

// assertNoCanaryLeak asserts that errStr contains zero sentinel
// substrings. On failure, the test reports which canary leaked AND
// the full error message so the operator/agent can see the regression.
func assertNoCanaryLeak(t *testing.T, errStr string, context string) {
	t.Helper()
	for _, sentinel := range canaries() {
		if strings.Contains(errStr, sentinel) {
			t.Errorf("%s: error message contains LEAKCANARY substring %q (FR-051-007 redaction violation): %s", context, sentinel, errStr)
		}
	}
}

// setEnv001 wires the test environment for spec 051 redaction tests by
// reusing the existing canonical helpers (setRequiredEnv +
// setProductionAuthBaseline) and then overriding the auth secrets and
// the DB password with sentinel canary values. Each individual test
// flips ONE additional env var to force the targeted error path.
func setEnv001(t *testing.T) {
	t.Helper()
	setRequiredEnv(t)
	setProductionAuthBaseline(t)
	// Override the production baseline secrets with canaries so any
	// echoed value is detectable. Each canary contains the unique
	// LEAKCANARY-* substring asserted by assertNoCanaryLeak.
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", canarySigningKey)
	t.Setenv("AUTH_AT_REST_HASHING_KEY", canaryHashingKey)
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", canaryBootstrap)
	t.Setenv("SMACKEREL_AUTH_TOKEN", canarySharedSecret+"-with-suffix")
	// Construct DATABASE_URL from a canary password substring so the
	// DB-password redaction tests can detect any echo.
	t.Setenv("DATABASE_URL", "postgres://"+"u"+":"+canaryDBPassword+"@h:5432/d") // gitleaks:allow — canary URL built from LEAKCANARY-* sentinel; not a real credential
}

// TestErrorPaths_NeverEchoSignatureKey — driving loadAuthConfig with
// an empty AUTH_SIGNING_ACTIVE_PRIVATE_KEY in production must surface
// the KEY name without echoing the (canary) value of any other secret
// the loader has already read.
func TestErrorPaths_NeverEchoSignatureKey(t *testing.T) {
	setEnv001(t)
	// Force the failure: missing signing key in production.
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "")

	c, err := Load()
	if err == nil {
		t.Fatalf("expected error from config.Load when AUTH_SIGNING_ACTIVE_PRIVATE_KEY is empty in production, got nil; cfg=%+v", c)
	}
	if !strings.Contains(err.Error(), "AUTH_SIGNING_ACTIVE_PRIVATE_KEY") {
		t.Errorf("error must name AUTH_SIGNING_ACTIVE_PRIVATE_KEY, got: %v", err)
	}
	assertNoCanaryLeak(t, err.Error(), "loadAuthConfig signing-key path")
}

// TestErrorPaths_NeverEchoBootstrapToken — driving loadAuthConfig with
// an empty AUTH_BOOTSTRAP_TOKEN in production must surface the KEY
// name without echoing the (canary) value of any other secret.
func TestErrorPaths_NeverEchoBootstrapToken(t *testing.T) {
	setEnv001(t)
	// Force the failure: missing bootstrap token in production.
	t.Setenv("AUTH_BOOTSTRAP_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from config.Load when AUTH_BOOTSTRAP_TOKEN is empty in production")
	}
	if !strings.Contains(err.Error(), "AUTH_BOOTSTRAP_TOKEN") {
		t.Errorf("error must name AUTH_BOOTSTRAP_TOKEN, got: %v", err)
	}
	assertNoCanaryLeak(t, err.Error(), "loadAuthConfig bootstrap-token path")
}

// TestErrorPaths_NeverEchoDBPassword — driving Validate with a
// dev-default DATABASE_URL password in production must surface the
// DATABASE_URL name without echoing the password value or any other
// canary substring.
func TestErrorPaths_NeverEchoDBPassword(t *testing.T) {
	setEnv001(t)
	// Force the failure: dev-default DB password in production.
	devPassword := DevDBPasswords[0] // "smackerel"
	dbURL := "postgres://" + "u" + ":" + devPassword + "@h:5432/d"
	t.Setenv("DATABASE_URL", dbURL)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from config.Load when DATABASE_URL password is dev-default in production")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error must name DATABASE_URL, got: %v", err)
	}
	if strings.Contains(err.Error(), devPassword) {
		t.Errorf("error must NOT echo dev-default value %q, got: %v", devPassword, err)
	}
	assertNoCanaryLeak(t, err.Error(), "Validate DB-password path")
}

// TestErrorPaths_RuntimeAuthStartup_NeverEchoesSecrets exercises the
// wiring-time defense-in-depth check directly.
func TestErrorPaths_RuntimeAuthStartup_NeverEchoesSecrets(t *testing.T) {
	cases := []struct {
		name     string
		cfg      auth.RuntimeAuthConfig
		wantName string
	}{
		{
			name: "missing-signing-key",
			cfg: auth.RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: "",
				SigningActiveKeyID:      "key-2026-05",
				AtRestHashingKey:        canaryHashingKey,
			},
			wantName: "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
		},
		{
			name: "missing-key-id",
			cfg: auth.RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: canarySigningKey,
				SigningActiveKeyID:      "",
				AtRestHashingKey:        canaryHashingKey,
			},
			wantName: "AUTH_SIGNING_ACTIVE_KEY_ID",
		},
		{
			name: "missing-hashing-key",
			cfg: auth.RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: canarySigningKey,
				SigningActiveKeyID:      "key-2026-05",
				AtRestHashingKey:        "",
			},
			wantName: "AUTH_AT_REST_HASHING_KEY",
		},
		{
			name: "hashing-equals-signing",
			cfg: auth.RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: canarySigningKey,
				SigningActiveKeyID:      "key-2026-05",
				AtRestHashingKey:        canarySigningKey,
			},
			wantName: "AUTH_AT_REST_HASHING_KEY",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidateRuntimeAuthStartup("production", tc.cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantName) {
				t.Errorf("error must name %q, got: %v", tc.wantName, err)
			}
			assertNoCanaryLeak(t, err.Error(), "auth.ValidateRuntimeAuthStartup "+tc.name)
		})
	}
}

// TestErrorPaths_NeverEchoPlaceholderAuthToken — closes the canary blind
// spot for the SMACKEREL_AUTH_TOKEN placeholder-rejection branch in
// Validate() (FR-051-007).
//
// setEnv001 seeds SMACKEREL_AUTH_TOKEN with a ≥16-char NON-placeholder
// canary, so the placeholder-rejection branch was NEVER exercised by the
// redaction proof — false confidence. This adversarial test drives that
// EXACT branch by setting SMACKEREL_AUTH_TOKEN to a public placeholder
// constant ("changeme", drawn from the Validate() rejection list — a safe
// public value, not a real secret) and asserts the error:
//
//  1. is non-nil,
//  2. names the offending KEY (SMACKEREL_AUTH_TOKEN), and
//  3. does NOT echo the offending placeholder VALUE.
//
// This is a RED→GREEN guard: against the pre-fix `%q` echo it FAILS
// (the value is reflected into the error); after the value echo is
// removed it PASSES. Any future regression that reintroduces a value
// echo on this branch re-fails this test.
func TestErrorPaths_NeverEchoPlaceholderAuthToken(t *testing.T) {
	setEnv001(t)
	// Force the failure: a known placeholder shared auth token in
	// production. "changeme" is a public placeholder constant from the
	// Validate() rejection list — safe to reference, not a real secret.
	const placeholder = "changeme"
	t.Setenv("SMACKEREL_AUTH_TOKEN", placeholder)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from config.Load when SMACKEREL_AUTH_TOKEN is a known placeholder in production")
	}
	if !strings.Contains(err.Error(), "SMACKEREL_AUTH_TOKEN") {
		t.Errorf("error must name SMACKEREL_AUTH_TOKEN, got: %v", err)
	}
	if strings.Contains(err.Error(), placeholder) {
		t.Errorf("error must NOT echo placeholder value %q (FR-051-007 redaction contract), got: %v", placeholder, err)
	}
	assertNoCanaryLeak(t, err.Error(), "Validate placeholder-auth-token path")
}
