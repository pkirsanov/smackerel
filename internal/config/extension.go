// Package config — Spec 058 Scope 1: extension ingest SST.
//
// ExtensionIngestConfig governs POST /v1/connectors/extension/ingest.
// Every field originates in `extension.ingest.*` of config/smackerel.yaml
// and flows through scripts/commands/config.sh into the generated env
// file as EXTENSION_INGEST_* variables. There are no in-source defaults
// — Validate() returns a non-nil error naming every missing field per
// the smackerel-no-defaults policy.
package config

import (
	"fmt"
	"strings"
)

// ExtensionConfig groups every spec-058 extension subsystem
// config block. Only Ingest is populated in scope today; future
// scopes (devices admin view, build/release wiring) extend this
// surface in place rather than introducing parallel root keys.
type ExtensionConfig struct {
	Ingest ExtensionIngestConfig
}

// ExtensionIngestConfig is the SST surface for the spec-058 extension
// ingest endpoint. Spec 058 design §6.
type ExtensionIngestConfig struct {
	// Enabled gates whether the route is mounted at all. Sourced from
	// EXTENSION_INGEST_ENABLED.
	Enabled bool

	// MaxBatchItems is the hard upper bound on items per POST body.
	// Sourced from EXTENSION_INGEST_MAX_BATCH_ITEMS. Spec 058 design
	// §3.1 fixes this at 256.
	MaxBatchItems int

	// MaxBodyBytes is the hard upper bound on request body size in
	// bytes. Sourced from EXTENSION_INGEST_MAX_BODY_BYTES. Spec 058
	// design §3.1 fixes this at 1 MiB.
	MaxBodyBytes int64

	// DefaultDedupWindowSeconds is the per-(url, device) window in
	// seconds used to compute the time bucket for non-bookmark content
	// types when the per-request Metadata.dedup_window_seconds is
	// absent or out of range. Sourced from
	// EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS. No source default
	// — operator MUST set in config/smackerel.yaml.
	DefaultDedupWindowSeconds int

	// AcceptedContentTypes is the closed allowlist of RawArtifact
	// ContentType values accepted by the ingest handler. Sourced from
	// EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES (JSON array).
	AcceptedContentTypes []string

	// RequiredTokenScope is the literal scope name the spec 060
	// RequireScope middleware enforces on this surface. Sourced from
	// EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE. The handler does not
	// consume it directly — it lives in SST so the route-mount wiring
	// in router.go and the operator-facing docs stay aligned with a
	// single source of truth.
	RequiredTokenScope string
}

// Validate returns nil when every required field is populated and
// non-empty per the smackerel-no-defaults policy. Failure mode: a
// joined error naming every missing or zero-valued field so the
// operator fixes them all in one round.
func (c *ExtensionIngestConfig) Validate() error {
	var missing []string
	if c.MaxBatchItems <= 0 {
		missing = append(missing, "EXTENSION_INGEST_MAX_BATCH_ITEMS")
	}
	if c.MaxBodyBytes <= 0 {
		missing = append(missing, "EXTENSION_INGEST_MAX_BODY_BYTES")
	}
	if c.DefaultDedupWindowSeconds <= 0 {
		missing = append(missing, "EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS")
	}
	if len(c.AcceptedContentTypes) == 0 {
		missing = append(missing, "EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES")
	} else {
		for _, ct := range c.AcceptedContentTypes {
			if strings.TrimSpace(ct) == "" {
				missing = append(missing, "EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES (contains empty entry)")
				break
			}
		}
	}
	if strings.TrimSpace(c.RequiredTokenScope) == "" {
		missing = append(missing, "EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid required extension ingest configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

// loadExtensionConfig loads every extension subsystem block.
func loadExtensionConfig() (ExtensionConfig, error) {
	ingest, err := loadExtensionIngestConfig()
	if err != nil {
		return ExtensionConfig{}, err
	}
	return ExtensionConfig{Ingest: ingest}, nil
}

// loadExtensionIngestConfig reads the EXTENSION_INGEST_* env vars,
// validates every required field with fail-loud semantics, and
// returns the populated config. Errors are joined into one report.
func loadExtensionIngestConfig() (ExtensionIngestConfig, error) {
	var cfg ExtensionIngestConfig
	var errs []string

	cfg.Enabled, errs = requiredBool("EXTENSION_INGEST_ENABLED", errs)
	cfg.MaxBatchItems, errs = parsePositiveInt("EXTENSION_INGEST_MAX_BATCH_ITEMS", errs)
	cfg.MaxBodyBytes, errs = parsePositiveInt64("EXTENSION_INGEST_MAX_BODY_BYTES", errs)
	cfg.DefaultDedupWindowSeconds, errs = parsePositiveInt("EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS", errs)
	cfg.AcceptedContentTypes, errs = requiredStringList("EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES", errs)
	cfg.RequiredTokenScope, errs = requiredNonEmptyString("EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE", errs)

	if len(errs) > 0 {
		return ExtensionIngestConfig{}, fmt.Errorf("missing or invalid required extension ingest configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return ExtensionIngestConfig{}, err
	}
	return cfg, nil
}
