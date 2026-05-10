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
package auth

import "fmt"

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
	if cfg.SigningActiveKeyID == "" {
		return fmt.Errorf("auth: AUTH_SIGNING_ACTIVE_KEY_ID must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true")
	}
	if cfg.AtRestHashingKey == "" {
		return fmt.Errorf("auth: AUTH_AT_REST_HASHING_KEY must be set when SMACKEREL_ENV=production AND AUTH_ENABLED=true")
	}
	if cfg.AtRestHashingKey == cfg.SigningActivePrivateKey {
		return fmt.Errorf("auth: AUTH_AT_REST_HASHING_KEY must differ from AUTH_SIGNING_ACTIVE_PRIVATE_KEY (spec 044 OQ-8)")
	}
	return nil
}
