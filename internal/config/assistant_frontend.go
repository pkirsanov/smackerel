// Package config — Spec 073 SCOPE-1b: web/mobile assistant frontend SST.
//
// Defines the eight `web.assistant.*` / `mobile.assistant.*` SST keys
// flowing through scripts/commands/config.sh as WEB_ASSISTANT_* /
// MOBILE_ASSISTANT_* env vars. Every key is REQUIRED at the generator
// boundary (Gate G028 / smackerel-no-defaults); missing or empty values
// fail loud at startup with [F073-SST-MISSING] or [F073-SST-INVALID].
//
// Per spec 073 design.md → "Hard Configuration Surface":
//   - web.assistant.backend_base_url: explicit same-origin marker or
//     explicit non-empty URL from SST.
//   - mobile.assistant.backend_base_url: explicit non-empty HTTPS URL.
//   - web.assistant.schema_version, mobile.assistant.schema_version:
//     must equal the spec 069 pinned value "v1".
//   - mobile.assistant.platforms: explicit set containing both
//     "ios" and "android".
//   - mobile.assistant.auth_mode: explicit auth-owner-approved mode
//     (non-empty closed string; values are owned by the auth surface).
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// FrontendSchemaVersionV1 is the spec 069 pinned wire-schema version
// that both frontend surfaces must declare.
const FrontendSchemaVersionV1 = "v1"

// WebBackendSameOriginMarker is the explicit literal accepted for the
// web backend base URL when the web client posts same-origin.
const WebBackendSameOriginMarker = "same-origin"

// WebAssistantConfig is the spec 073 SCOPE-1b SST surface for the web
// frontend client.
type WebAssistantConfig struct {
	// Enabled gates the web assistant route.
	Enabled bool
	// BackendBaseURL is the explicit same-origin marker
	// (WebBackendSameOriginMarker) or an explicit non-empty URL.
	BackendBaseURL string
	// SchemaVersion MUST equal FrontendSchemaVersionV1.
	SchemaVersion string
}

// MobileAssistantConfig is the spec 073 SCOPE-1b SST surface for the
// shared mobile frontend client.
type MobileAssistantConfig struct {
	// Enabled gates the shared mobile assistant build.
	Enabled bool
	// BackendBaseURL is an explicit non-empty HTTPS URL.
	BackendBaseURL string
	// SchemaVersion MUST equal FrontendSchemaVersionV1.
	SchemaVersion string
	// Platforms MUST contain both "ios" and "android".
	Platforms []string
	// AuthMode is an explicit auth-owner-approved mode identifier.
	AuthMode string
}

// LoadWebAssistant reads every WEB_ASSISTANT_* env var and validates.
func LoadWebAssistant() (WebAssistantConfig, error) {
	var cfg WebAssistantConfig
	var errs []string

	cfg.Enabled, errs = lookupBoolStrict("WEB_ASSISTANT_ENABLED", errs)
	cfg.BackendBaseURL, errs = lookupNonEmptyString("WEB_ASSISTANT_BACKEND_BASE_URL", errs)
	cfg.SchemaVersion, errs = lookupNonEmptyString("WEB_ASSISTANT_SCHEMA_VERSION", errs)

	if len(errs) > 0 {
		return WebAssistantConfig{}, fmt.Errorf("[F073-SST-MISSING] missing or empty required web.assistant configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return WebAssistantConfig{}, err
	}
	return cfg, nil
}

// LoadMobileAssistant reads every MOBILE_ASSISTANT_* env var and validates.
func LoadMobileAssistant() (MobileAssistantConfig, error) {
	var cfg MobileAssistantConfig
	var errs []string

	cfg.Enabled, errs = lookupBoolStrict("MOBILE_ASSISTANT_ENABLED", errs)
	cfg.BackendBaseURL, errs = lookupNonEmptyString("MOBILE_ASSISTANT_BACKEND_BASE_URL", errs)
	cfg.SchemaVersion, errs = lookupNonEmptyString("MOBILE_ASSISTANT_SCHEMA_VERSION", errs)
	var platformsCSV string
	platformsCSV, errs = lookupNonEmptyString("MOBILE_ASSISTANT_PLATFORMS", errs)
	cfg.Platforms = splitCSV(platformsCSV)
	cfg.AuthMode, errs = lookupNonEmptyString("MOBILE_ASSISTANT_AUTH_MODE", errs)

	if len(errs) > 0 {
		return MobileAssistantConfig{}, fmt.Errorf("[F073-SST-MISSING] missing or empty required mobile.assistant configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return MobileAssistantConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the closed-vocabulary and shape rules for web.
func (c *WebAssistantConfig) Validate() error {
	var errs []string
	if c.SchemaVersion != FrontendSchemaVersionV1 {
		errs = append(errs, fmt.Sprintf("web.assistant.schema_version (must equal %q for spec 069 v1, got %q)", FrontendSchemaVersionV1, c.SchemaVersion))
	}
	// backend_base_url must be either the explicit same-origin marker
	// or a parseable absolute URL with http/https scheme.
	if c.BackendBaseURL != WebBackendSameOriginMarker {
		if err := validateAbsoluteHTTPURL(c.BackendBaseURL, false); err != nil {
			errs = append(errs, fmt.Sprintf("web.assistant.backend_base_url (must be %q or an explicit absolute http(s) URL, got %q: %v)", WebBackendSameOriginMarker, c.BackendBaseURL, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("[F073-SST-INVALID] invalid web.assistant configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// Validate enforces the closed-vocabulary and shape rules for mobile.
func (c *MobileAssistantConfig) Validate() error {
	var errs []string
	if c.SchemaVersion != FrontendSchemaVersionV1 {
		errs = append(errs, fmt.Sprintf("mobile.assistant.schema_version (must equal %q for spec 069 v1, got %q)", FrontendSchemaVersionV1, c.SchemaVersion))
	}
	// mobile.assistant.backend_base_url must be an explicit HTTPS URL.
	if err := validateAbsoluteHTTPURL(c.BackendBaseURL, true); err != nil {
		errs = append(errs, fmt.Sprintf("mobile.assistant.backend_base_url (must be an explicit non-empty https URL, got %q: %v)", c.BackendBaseURL, err))
	}
	if !containsString(c.Platforms, "ios") || !containsString(c.Platforms, "android") {
		errs = append(errs, fmt.Sprintf("mobile.assistant.platforms (must contain both \"ios\" and \"android\", got %v)", c.Platforms))
	}
	if strings.TrimSpace(c.AuthMode) == "" {
		errs = append(errs, "mobile.assistant.auth_mode (must be a non-empty auth-owner-approved mode)")
	}
	if len(errs) > 0 {
		return fmt.Errorf("[F073-SST-INVALID] invalid mobile.assistant configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// lookupBoolStrict reads an env var that MUST be exactly "true" or
// "false". Missing (LookupEnv == false) or empty value → error.
func lookupBoolStrict(key string, errs []string) (bool, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return false, append(errs, key+" (env var not set or empty)")
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, append(errs, fmt.Sprintf("%s (must be a boolean, got %q)", key, v))
	}
	return b, errs
}

// lookupNonEmptyString reads an env var. Missing OR empty → error.
// Unlike lookupString (which tolerates "" at load time), this enforces
// non-empty at the loader boundary so spec 073 SCOPE-1b's "no fallback,
// no empty value" rule is enforced before Validate() runs.
func lookupNonEmptyString(key string, errs []string) (string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", append(errs, key+" (env var not set or empty)")
	}
	return v, errs
}

// validateAbsoluteHTTPURL parses raw and requires an absolute URL with
// http or https scheme (or only https when httpsOnly is true) and a
// non-empty host.
func validateAbsoluteHTTPURL(raw string, httpsOnly bool) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if httpsOnly {
			return fmt.Errorf("scheme must be https, got http")
		}
		return nil
	default:
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
