package auth_test

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
)

// Spec 052 SCN-052-S07 / FR-052-007 — auth-side runtime placeholder
// rejection tests. These mirror the Validate() coverage in
// internal/config/placeholder_runtime_test.go but exercise the
// dedicated wiring-time defense in auth.ValidateRuntimeAuthStartup.
//
// T-052-015 / SCN-052-S07 / BS-052-008.

// validBaselineCfg returns a non-placeholder, non-empty
// RuntimeAuthConfig that ValidateRuntimeAuthStartup accepts. Each
// table case below mutates exactly one field to its placeholder
// marker to trip the FR-052-007 branch.
func validBaselineCfg() auth.RuntimeAuthConfig {
	return auth.RuntimeAuthConfig{
		Enabled:                 true,
		SigningActivePrivateKey: "k4.secret.AAAA-not-a-real-key", // gitleaks:allow — non-secret test fixture
		SigningActiveKeyID:      "key-2026-05",
		AtRestHashingKey:        "hmac-secret-AAAA-not-a-real-key", // gitleaks:allow — non-secret test fixture
	}
}

// TestValidateRuntimeAuthStartup_RejectsPlaceholderValues asserts the
// runtime startup guard refuses to start when an AUTH_* slot still
// equals its placeholder marker. The rejection message MUST name the
// offending KEY and MUST NOT echo the placeholder marker substring.
func TestValidateRuntimeAuthStartup_RejectsPlaceholderValues(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		mutator func(*auth.RuntimeAuthConfig)
	}{
		{
			name: "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
			key:  "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
			mutator: func(c *auth.RuntimeAuthConfig) {
				c.SigningActivePrivateKey = config.Placeholder("AUTH_SIGNING_ACTIVE_PRIVATE_KEY")
			},
		},
		{
			name: "AUTH_AT_REST_HASHING_KEY",
			key:  "AUTH_AT_REST_HASHING_KEY",
			mutator: func(c *auth.RuntimeAuthConfig) {
				c.AtRestHashingKey = config.Placeholder("AUTH_AT_REST_HASHING_KEY")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaselineCfg()
			tc.mutator(&cfg)

			err := auth.ValidateRuntimeAuthStartup("production", cfg)
			if err == nil {
				t.Fatalf("expected error from ValidateRuntimeAuthStartup when %s equals its placeholder marker, got nil", tc.key)
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.key) {
				t.Errorf("error must name %q, got: %v", tc.key, err)
			}
			if strings.Contains(errStr, "__SECRET_PLACEHOLDER__") {
				t.Errorf("error must NOT echo placeholder marker substring '__SECRET_PLACEHOLDER__', got: %v", err)
			}
			if !strings.Contains(errStr, "spec 052 FR-052-007") {
				t.Errorf("error must reference spec 052 FR-052-007 for operator traceability, got: %v", err)
			}
		})
	}
}

// TestValidateRuntimeAuthStartup_PlaceholderFormatParity is the
// drift-detector for the cross-package placeholder format mirror. The
// auth package inlines placeholderPrefix / placeholderSuffix to avoid
// a production-time import of internal/config (see startup.go
// rationale block). This test asserts that the format produced by
// config.Placeholder() — the canonical declaration — matches what
// the auth package would compute for each managed AUTH_* key. If
// either side changes, this test breaks and forces the other side to
// follow.
func TestValidateRuntimeAuthStartup_PlaceholderFormatParity(t *testing.T) {
	authKeys := []string{
		"AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
		"AUTH_AT_REST_HASHING_KEY",
	}
	for _, key := range authKeys {
		want := config.Placeholder(key)
		// Drive the auth package's check by setting the field to the
		// canonical config.Placeholder() value; if the auth-side
		// format mirror has drifted, the rejection branch would NOT
		// fire and the test would fail.
		cfg := validBaselineCfg()
		switch key {
		case "AUTH_SIGNING_ACTIVE_PRIVATE_KEY":
			cfg.SigningActivePrivateKey = want
		case "AUTH_AT_REST_HASHING_KEY":
			cfg.AtRestHashingKey = want
		}
		err := auth.ValidateRuntimeAuthStartup("production", cfg)
		if err == nil {
			t.Errorf("auth.ValidateRuntimeAuthStartup did NOT detect placeholder for %s — placeholder format mirror has drifted: config.Placeholder(%q) = %q", key, key, want)
			continue
		}
		if !strings.Contains(err.Error(), key) {
			t.Errorf("auth rejection for %s did not name the key, got: %v", key, err)
		}
	}
}
