// Package config — Spec 074 SCOPE-1: capture-as-fallback policy SST.
//
// CaptureFallbackConfig governs the `assistant.capture_as_fallback.*`
// block. Every field originates in config/smackerel.yaml and flows
// through scripts/commands/config.sh into the generated env file as
// CAPTURE_AS_FALLBACK_* variables. There are no in-source defaults
// (Gate G028, smackerel-no-defaults): every env var MUST be present
// at load time and Validate() rejects empty / out-of-range values
// unconditionally. There is no `disable_capture_as_fallback` key by
// design — capture-as-fallback is inviolable for eligible turns
// (spec 074 Hard Constraint #2).
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// NormalizationPolicyV1 is the v1 closed-vocabulary normalization
// policy identifier (NFKC + casefold + whitespace collapse).
const NormalizationPolicyV1 = "nfkc_casefold_ws_v1"

// CaptureFallbackConfig is the SST surface for spec 074 SCOPE-1.
type CaptureFallbackConfig struct {
	// DedupWindow is the per-user same-normalized-text dedup bucket
	// duration. MUST be > 0.
	DedupWindow time.Duration
	// ClarifyAbandonTimeout is the clarification-turn abandonment
	// TTL consumed by Scope 4. MUST be > 0.
	ClarifyAbandonTimeout time.Duration
	// NormalizationPolicy is the closed-vocabulary normalization
	// policy identifier. v1: only NormalizationPolicyV1 is allowed.
	NormalizationPolicy string
	// DedupHashKey is the HMAC-SHA256 secret used to derive the
	// normalized-text hash that scopes dedup lookups. MUST be
	// non-empty; operators must override the dev placeholder before
	// any non-local deployment.
	DedupHashKey string
	// RetentionAuditDays is the metadata audit retention horizon for
	// artifact_capture_policy rows. MUST be >= 1.
	RetentionAuditDays int
}

// LoadCaptureFallback reads every CAPTURE_AS_FALLBACK_* env var and
// returns a populated CaptureFallbackConfig plus Validate() result.
// Missing env vars (LookupEnv == false) are a fail-loud
// [F074-SST-MISSING] error. Empty/invalid values are routed through
// Validate() which produces [F074-SST-INVALID].
func LoadCaptureFallback() (CaptureFallbackConfig, error) {
	var cfg CaptureFallbackConfig
	var errs []string

	cfg.DedupWindow, errs = lookupDuration("CAPTURE_AS_FALLBACK_DEDUP_WINDOW", errs)
	cfg.ClarifyAbandonTimeout, errs = lookupDuration("CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT", errs)
	cfg.NormalizationPolicy, errs = lookupString("CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY", errs)
	cfg.DedupHashKey, errs = lookupString("CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY", errs)
	cfg.RetentionAuditDays, errs = lookupInt("CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS", errs)

	if len(errs) > 0 {
		return CaptureFallbackConfig{}, fmt.Errorf("[F074-SST-MISSING] missing or invalid required capture_as_fallback configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return CaptureFallbackConfig{}, err
	}
	return cfg, nil
}

// Validate enforces spec 074 design §"Configuration And Migrations".
// No enabled=false short-circuit: this foundation always validates
// because capture-as-fallback is inviolable.
func (c *CaptureFallbackConfig) Validate() error {
	var errs []string

	if c.DedupWindow <= 0 {
		errs = append(errs, fmt.Sprintf("capture_as_fallback.dedup_window (must be > 0, got %s)", c.DedupWindow))
	}
	if c.ClarifyAbandonTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("capture_as_fallback.clarify_abandon_timeout (must be > 0, got %s)", c.ClarifyAbandonTimeout))
	}
	if c.NormalizationPolicy != NormalizationPolicyV1 {
		errs = append(errs, fmt.Sprintf("capture_as_fallback.normalization_policy (must be %q for v1, got %q)", NormalizationPolicyV1, c.NormalizationPolicy))
	}
	if strings.TrimSpace(c.DedupHashKey) == "" {
		errs = append(errs, "capture_as_fallback.dedup_hash_key (empty; HMAC hash derivation requires a non-empty secret)")
	}
	if c.RetentionAuditDays < 1 {
		errs = append(errs, fmt.Sprintf("capture_as_fallback.retention_audit_days (must be >= 1, got %d)", c.RetentionAuditDays))
	}

	if len(errs) > 0 {
		return fmt.Errorf("[F074-SST-INVALID] invalid capture_as_fallback configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// lookupDuration reads an env var as a Go duration. Missing var or
// unparseable value → typed error. Range checks live in Validate().
func lookupDuration(key string, errs []string) (time.Duration, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, append(errs, key+" (env var not set)")
	}
	if v == "" {
		return 0, errs
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, append(errs, fmt.Sprintf("%s (must be a Go duration, got %q)", key, v))
	}
	return d, errs
}
