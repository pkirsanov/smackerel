//go:build integration

package integration

import (
	"strings"
	"testing"
)

// Scenario: Tests must use the disposable test stack, never the persistent dev stack
//   Given the live-stack integration helpers read DATABASE_URL and NATS_URL
//   When either points at the persistent dev/prod stack
//   Then the helper refuses to open a connection
//
// HARDEN-031-001. This package commits destructive DDL —
// TestMigrations_TableDropAndRecreate runs `DROP TABLE ... CASCADE` and commits
// it — against whatever DATABASE_URL resolves to, and testPool previously
// applied no stack assertion. The sibling stress lane already guards this way
// (tests/stress/ml_readiness_timeout_stress_test.go: requireDisposableStack),
// but its markers cover only the core host-port prefixes (:4000/:4100) and so
// would NOT catch the dev postgres on 42001 — the adversarial case below.

// devPostgresURL is the persistent dev stack's postgres (config/smackerel.yaml
// environments.dev.postgres_host_port = 42001). Reintroducing the unguarded
// helper makes this case pass, which is exactly the regression to catch.
//
// The embedded password is a synthetic fixture value, not a real credential,
// and it is load-bearing: TestDisposableStackGuard_DoesNotLeakCredentials
// asserts the refusal never echoes it. Removing it would make that assertion
// tautological.
const devPostgresURL = "postgres://smackerel:devsecret@127.0.0.1:42001/smackerel?sslmode=disable" // gitleaks:allow

func TestDisposableStackGuard_RejectsDevPostgresHostPort(t *testing.T) {
	err := disposableStackViolation(func(key string) string {
		if key == "DATABASE_URL" {
			return devPostgresURL
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected refusal for DATABASE_URL on the dev postgres host port 42001, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("refusal must name the offending variable, got: %v", err)
	}
}

func TestDisposableStackGuard_RejectsDevNATSHostPort(t *testing.T) {
	err := disposableStackViolation(func(key string) string {
		if key == "NATS_URL" {
			return "nats://token@127.0.0.1:42002"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected refusal for NATS_URL on the dev nats host port 42002, got nil")
	}
}

// The refusal must not echo the URL: DATABASE_URL and NATS_URL embed
// credentials, and test output is not a safe place for them.
func TestDisposableStackGuard_DoesNotLeakCredentials(t *testing.T) {
	err := disposableStackViolation(func(key string) string {
		if key == "DATABASE_URL" {
			return devPostgresURL
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if strings.Contains(err.Error(), "devsecret") {
		t.Errorf("refusal leaked the credential embedded in DATABASE_URL: %v", err)
	}
}

// Anti-over-blocking: the exact URL shapes the `test integration`,
// `test integration-light`, and `test e2e` lanes inject MUST still pass, or the
// guard would break every sanctioned live-stack lane instead of protecting it.
func TestDisposableStackGuard_AllowsTestStackURLs(t *testing.T) {
	cases := []struct {
		name string
		db   string
		nats string
	}{
		{
			name: "in-network container ports injected by smackerel.sh",
			db:   "postgres://smackerel:pw@postgres:5432/smackerel?sslmode=disable",
			nats: "nats://token@nats:4222",
		},
		{
			name: "disposable test stack host ports",
			db:   "postgres://smackerel:pw@127.0.0.1:47001/smackerel?sslmode=disable",
			nats: "nats://token@127.0.0.1:47002",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := disposableStackViolation(func(key string) string {
				switch key {
				case "DATABASE_URL":
					return tc.db
				case "NATS_URL":
					return tc.nats
				}
				return ""
			})
			if err != nil {
				t.Fatalf("test-stack URLs must be permitted, got refusal: %v", err)
			}
		})
	}
}

// Unset env must stay a no-op so the helpers keep their existing skip
// semantics ("live stack not available") rather than turning into a failure.
func TestDisposableStackGuard_AllowsUnsetEnv(t *testing.T) {
	if err := disposableStackViolation(func(string) string { return "" }); err != nil {
		t.Fatalf("unset env must not be a violation, got: %v", err)
	}
}
