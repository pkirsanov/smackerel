package config

import (
	"strings"
	"testing"
)

// Spec 052 SCN-052-S07 / FR-052-007 — runtime defense-in-depth tests.
//
// These tests prove that Validate() refuses to start when ANY managed
// secret key surfaces with the SST placeholder marker as its value.
// The placeholder marker reaches Validate() only when the deploy
// adapter (knb) failed to substitute the secret before container
// start. The runtime MUST refuse to boot in that case.
//
// FR-051-007 redaction contract (extended for spec 052): on rejection,
// the error MUST name the offending KEY and MUST NOT echo the
// placeholder marker literal nor the resolved value.

// TestValidate_RejectsPlaceholderValues drives Validate() with a
// placeholder marker in each managed secret slot and asserts the
// loader refuses to start with a KEY-named, value-redacted error.
//
// T-052-014 / SCN-052-S07 / BS-052-007.
func TestValidate_RejectsPlaceholderValues(t *testing.T) {
	cases := []struct {
		name        string
		key         string
		envOverride func(t *testing.T)
	}{
		{
			name: "POSTGRES_PASSWORD",
			key:  "POSTGRES_PASSWORD",
			envOverride: func(t *testing.T) {
				placeholder := Placeholder("POSTGRES_PASSWORD")
				dbURL := "postgres://" + "u" + ":" + placeholder + "@h:5432/d"
				t.Setenv("DATABASE_URL", dbURL)
			},
		},
		{
			name: "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
			key:  "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
			envOverride: func(t *testing.T) {
				t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", Placeholder("AUTH_SIGNING_ACTIVE_PRIVATE_KEY"))
			},
		},
		{
			name: "AUTH_AT_REST_HASHING_KEY",
			key:  "AUTH_AT_REST_HASHING_KEY",
			envOverride: func(t *testing.T) {
				t.Setenv("AUTH_AT_REST_HASHING_KEY", Placeholder("AUTH_AT_REST_HASHING_KEY"))
			},
		},
		{
			name: "AUTH_BOOTSTRAP_TOKEN",
			key:  "AUTH_BOOTSTRAP_TOKEN",
			envOverride: func(t *testing.T) {
				t.Setenv("AUTH_BOOTSTRAP_TOKEN", Placeholder("AUTH_BOOTSTRAP_TOKEN"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequiredEnv(t)
			setProductionAuthBaseline(t)
			// setProductionAuthBaseline does NOT set AUTH_BOOTSTRAP_TOKEN
			// (spec 051 FR-051-004 expects operators to provide it
			// explicitly per-environment); we set a non-placeholder
			// value here so the loadAuthConfig "AUTH_BOOTSTRAP_TOKEN
			// REQUIRED in production" gate does NOT fire BEFORE the
			// FR-052-007 placeholder loop in Validate(). The
			// AUTH_BOOTSTRAP_TOKEN subtest case overrides this with
			// the placeholder marker AFTER this baseline.
			t.Setenv("AUTH_BOOTSTRAP_TOKEN", "production-bootstrap-token-baseline-AAAA") // gitleaks:allow — non-secret test fixture
			tc.envOverride(t)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error from Load when %s equals its placeholder marker, got nil", tc.key)
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.key) {
				t.Errorf("error must name %q, got: %v", tc.key, err)
			}
			// FR-051-007 + FR-052-007: redact the placeholder marker
			// itself (so logs cannot leak which marker leaked).
			if strings.Contains(errStr, "__SECRET_PLACEHOLDER__") {
				t.Errorf("error must NOT echo placeholder marker substring '__SECRET_PLACEHOLDER__', got: %v", err)
			}
			if !strings.Contains(errStr, "spec 052 FR-052-007") {
				t.Errorf("error must reference spec 052 FR-052-007 for operator traceability, got: %v", err)
			}
		})
	}
}

// TestRuntimeRejection_NameKeyOnly_NoValueLeakage extends the spec 051
// LEAKCANARY redaction surface to cover spec 052 FR-052-007 paths.
// Each canary substitutes a sentinel value in one slot; the test
// asserts that the placeholder rejection error neither echoes the
// sentinel nor reveals the placeholder marker literal.
//
// T-052-016 / SCN-052-S07 / BS-052-008.
func TestRuntimeRejection_NameKeyOnly_NoValueLeakage(t *testing.T) {
	// Build a placeholder-bearing baseline so Validate() will reach
	// the FR-052-007 branch first; every OTHER secret slot is seeded
	// with a unique LEAKCANARY-* sentinel so any leak is detectable.
	setEnv001(t)
	// Override the signing key with a placeholder — the FR-052-007
	// loop iterates SecretKeys() in declaration order
	// (POSTGRES_PASSWORD, AUTH_SIGNING_ACTIVE_PRIVATE_KEY,
	// AUTH_AT_REST_HASHING_KEY, AUTH_BOOTSTRAP_TOKEN); the
	// POSTGRES_PASSWORD slot is non-placeholder (canary), so the
	// loop will trip on AUTH_SIGNING_ACTIVE_PRIVATE_KEY.
	t.Setenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY", Placeholder("AUTH_SIGNING_ACTIVE_PRIVATE_KEY"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from Load when AUTH_SIGNING_ACTIVE_PRIVATE_KEY equals its placeholder marker, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "AUTH_SIGNING_ACTIVE_PRIVATE_KEY") {
		t.Errorf("error must name AUTH_SIGNING_ACTIVE_PRIVATE_KEY, got: %v", err)
	}
	// FR-051-007: the error MUST NOT echo any sentinel substring,
	// regardless of which slot it landed in.
	assertNoCanaryLeak(t, errStr, "Validate placeholder-rejection path")
	// FR-052-007: the error MUST NOT echo the placeholder marker
	// substring either (so logs cannot leak which marker tripped).
	if strings.Contains(errStr, "__SECRET_PLACEHOLDER__") {
		t.Errorf("error must NOT echo placeholder marker substring '__SECRET_PLACEHOLDER__', got: %v", err)
	}
}

// TestPlaceholder_FormatStability is a guardrail: it pins the exact
// placeholder format produced by config.Placeholder() against the
// FR-052-002 specification literal. If this test breaks, the auth
// package's expectedPlaceholder() function (and the bundle contract
// test in internal/deploy) MUST be updated in lockstep.
//
// Drift detector for the cross-package format mirror.
func TestPlaceholder_FormatStability(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"POSTGRES_PASSWORD", "__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__"},
		{"AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "__SECRET_PLACEHOLDER__AUTH_SIGNING_ACTIVE_PRIVATE_KEY__"},
		{"AUTH_AT_REST_HASHING_KEY", "__SECRET_PLACEHOLDER__AUTH_AT_REST_HASHING_KEY__"},
		{"AUTH_BOOTSTRAP_TOKEN", "__SECRET_PLACEHOLDER__AUTH_BOOTSTRAP_TOKEN__"},
	}
	for _, tc := range cases {
		got := Placeholder(tc.key)
		if got != tc.want {
			t.Errorf("Placeholder(%q) = %q, want %q (FR-052-002 format pinned)", tc.key, got, tc.want)
		}
		if !IsPlaceholder(got) {
			t.Errorf("IsPlaceholder(Placeholder(%q)) = false, want true (round-trip)", tc.key)
		}
	}
}
