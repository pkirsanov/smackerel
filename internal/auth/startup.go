// Spec 044 Scope 01 — runtime startup auth validation.
//
// ValidateRuntimeAuthStartup is the canonical runtime-side defense-in-
// depth check that the wiring layer (cmd/core/wiring.go) calls before
// the HTTP server starts. config.Load already enforces the same
// invariants at the loader boundary; this helper exists so the
// wiring layer can call a single function without importing config-
// package internals AND so the runtime check is unit-testable from
// outside the cmd/core/main package.
//
// Contract (mirrors design.md §3 OQ-1, OQ-8):
//
//   - In production with auth.enabled=true:
//   - signing.active_private_key MUST be non-empty
//   - signing.active_key_id MUST be non-empty
//   - at_rest_hashing_key MUST be non-empty
//   - at_rest_hashing_key MUST differ from signing.active_private_key
//   - In any other combination (auth disabled OR not production):
//     returns nil unconditionally — bootstrap and dev flows MUST be
//     allowed to start with empty signing material because operators
//     mint material AFTER first start (CLI keygen → CLI bootstrap).
//
// Spec 052 FR-052-007 amendment: every AUTH_* field that is non-empty
// is additionally checked against the SST placeholder marker (see
// internal/config/secret_keys.go for the canonical declaration). A
// placeholder reaching this validator means the deploy adapter (knb)
// failed to substitute the secret before container start; the runtime
// MUST refuse to boot. The returned error names the offending KEY
// only — it never echoes the placeholder marker literal or the
// resolved value (FR-051-007 redaction contract extended). The
// placeholder format mirror lives in placeholderPrefix /
// placeholderSuffix below; a parity test in startup_placeholder_test.go
// asserts equality with config.Placeholder() to detect drift between
// the two locations. internal/auth deliberately does NOT import
// internal/config to preserve the explicit decoupling established by
// spec 044 (no AuthConfig type alias, no transitive dependency on the
// config loader).
package auth

import "fmt"

// placeholderPrefix and placeholderSuffix are the spec 052 FR-052-002
// SST placeholder marker bookends. They MUST match the canonical
// declaration in internal/config/secret_keys.go (placeholderPrefix /
// placeholderSuffix). The startup_placeholder_test.go parity test
// asserts equality at compile time via config.Placeholder() to catch
// drift; production code uses these inlined constants to avoid an
// internal/auth → internal/config production import that would create
// a test-time cycle (log_redaction_test.go imports auth from package
// config).
const (
	placeholderPrefix = "__SECRET_PLACEHOLDER__"
	placeholderSuffix = "__"
)

// expectedPlaceholder returns the deterministic placeholder marker for
// a managed secret key. Mirrors internal/config.Placeholder().
func expectedPlaceholder(key string) string {
	return placeholderPrefix + key + placeholderSuffix
}

// RuntimeAuthConfig is the minimal AUTH_* surface the runtime startup
// guard needs. Defined here (NOT a struct alias of config.AuthConfig)
// to avoid importing internal/config from internal/auth and to keep
// the contract explicit. Callers (cmd/core/wiring.go) construct this
// from cfg.Auth.* fields at call time.
type RuntimeAuthConfig struct {
	Enabled                 bool
	SigningActivePrivateKey string
	SigningActiveKeyID      string
	AtRestHashingKey        string
}

// ValidateRuntimeAuthStartup enforces the spec 044 production-mode
// signing-material contract at runtime. Returns nil when auth is
// disabled OR the environment is not production.
func ValidateRuntimeAuthStartup(environment string, cfg RuntimeAuthConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if environment != "production" {
		return nil
	}
	if cfg.SigningActivePrivateKey == "" {
		return fmt.Errorf("auth: AUTH_SIGNING_ACTIVE_PRIVATE_KEY must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true")
	}
	// Spec 052 FR-052-007 — refuse a placeholder marker that survived
	// the deploy-adapter substitution step. Naming only; never echo
	// the marker or the value (FR-051-007 redaction contract).
	if cfg.SigningActivePrivateKey == expectedPlaceholder("AUTH_SIGNING_ACTIVE_PRIVATE_KEY") {
		return fmt.Errorf("auth: AUTH_SIGNING_ACTIVE_PRIVATE_KEY still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)")
	}
	if cfg.SigningActiveKeyID == "" {
		return fmt.Errorf("auth: AUTH_SIGNING_ACTIVE_KEY_ID must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true")
	}
	if cfg.AtRestHashingKey == "" {
		return fmt.Errorf("auth: AUTH_AT_REST_HASHING_KEY must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true")
	}
	// Spec 052 FR-052-007 — same placeholder defense for the at-rest
	// hashing key.
	if cfg.AtRestHashingKey == expectedPlaceholder("AUTH_AT_REST_HASHING_KEY") {
		return fmt.Errorf("auth: AUTH_AT_REST_HASHING_KEY still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)")
	}
	if cfg.AtRestHashingKey == cfg.SigningActivePrivateKey {
		return fmt.Errorf("auth: AUTH_AT_REST_HASHING_KEY must differ from AUTH_SIGNING_ACTIVE_PRIVATE_KEY (spec 044 OQ-8)")
	}
	return nil
}
