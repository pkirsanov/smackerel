// Package config secret-key manifest.
//
// Spec 052 FR-052-001 / FR-052-002 — Go-side mirror of the canonical
// secret-key manifest declared at config/smackerel.yaml under
// infrastructure.secret_keys. The list lives in three places that MUST
// agree:
//
//  1. config/smackerel.yaml          (yaml source of truth)
//  2. internal/config/secret_keys.go (this file — Go mirror)
//  3. scripts/commands/config.sh     (shell mirror, added in Scope 2)
//
// Drift between the three mirrors is detected by the contract test
// internal/deploy/bundle_secret_contract_test.go added in Scope 3.
//
// To add a new managed secret: update all three mirrors AND ship a real
// value via the deploy adapter at knb/smackerel/secrets/<target>.enc.env.
// Doing only some of those steps is the failure mode the contract test
// catches.
package config

// secretKeys is the canonical, ordered list of SST-managed secret keys.
//
// Order is documented and significant: the contract test compares the
// yaml manifest and this slice byte-for-byte (entries AND order). When
// a future spec adds or removes a managed secret, both this slice and
// config/smackerel.yaml infrastructure.secret_keys MUST be updated in
// the same change set. The shell mirror in scripts/commands/config.sh
// MUST also be kept in sync; the contract test fails loud otherwise.
var secretKeys = []string{
	"POSTGRES_PASSWORD",
	"AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
	"AUTH_AT_REST_HASHING_KEY",
	"AUTH_BOOTSTRAP_TOKEN",
	"TELEGRAM_BOT_TOKEN",
}

// placeholderPrefix and placeholderSuffix bracket the deterministic,
// key-derived placeholder marker emitted by the SST loader for every
// managed secret key when TARGET_ENV is a production-class target.
//
// Format: __SECRET_PLACEHOLDER__<KEY>__
//
// The marker has no nonce, no timestamp, and no random suffix per
// design.md OQ-052-02 resolution. Determinism is required so identical
// (sourceSha, env, smackerel.yaml) inputs produce byte-identical bundle
// bytes (spec.md NFR "Determinism").
const (
	placeholderPrefix = "__SECRET_PLACEHOLDER__"
	placeholderSuffix = "__"
)

// SecretKeys returns a defensive copy of the canonical secret-key list.
//
// The returned slice is a fresh allocation; callers may freely mutate
// it without affecting the package-level canonical state. Order matches
// the yaml manifest exactly so the contract test's byte-for-byte parity
// check holds.
func SecretKeys() []string {
	out := make([]string, len(secretKeys))
	copy(out, secretKeys)
	return out
}

// Placeholder returns the deterministic placeholder marker for the
// given secret key.
//
// The format is "__SECRET_PLACEHOLDER__<KEY>__". The function is pure
// and deterministic: two calls with the same key return byte-identical
// strings. No timestamp, no nonce, no source-SHA mixing.
//
// Placeholder does NOT validate that key is in SecretKeys(); the SST
// loader is responsible for only invoking it for declared keys. The
// IsPlaceholder helper, by contrast, is strict and only returns true
// for declared keys.
func Placeholder(key string) string {
	return placeholderPrefix + key + placeholderSuffix
}

// IsPlaceholder reports whether value is a placeholder marker for any
// key in SecretKeys().
//
// Returns true iff value equals "__SECRET_PLACEHOLDER__<KEY>__" for
// some KEY in the canonical list. Returns false for the empty string,
// for real secret values, for placeholder-shaped strings whose KEY is
// not declared, and for partial matches (e.g., missing trailing "__").
//
// The check is exact-match (byte-for-byte) and case-sensitive: keys are
// uppercase by convention and the loader never emits any other casing.
func IsPlaceholder(value string) bool {
	if value == "" {
		return false
	}
	for _, key := range secretKeys {
		if value == Placeholder(key) {
			return true
		}
	}
	return false
}
