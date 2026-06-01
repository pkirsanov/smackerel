// Spec 065 SCOPE-1 — Generic micro-tools SST config.
//
// All keys are REQUIRED at the generator boundary (smackerel-no-defaults /
// Gate G028). Missing values fail loud during config load; there are
// no fallback providers, timeouts, catalog versions, or confidence
// floors (design.md §"Patterns to Avoid": no hidden provider fallback
// chains, no config defaults).
//
// SCOPE-1 ships the four tool config blocks and the loader. SCOPE-2..4
// consume the resolved values to wire concrete tools. The provider
// selectors (`location_normalize.provider`, `entity_resolve.*`) are
// REQUIRED string values even when `enabled=false`; the SST policy is
// "every key must resolve, every value must be explicit, the
// downstream consumer decides whether to actually wire a provider".

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// AssistantToolsConfig holds the spec 065 generic micro-tool SST
// values. Each sub-struct corresponds to one tool. SCOPE-1 only
// populates and validates these; SCOPE-2..4 register the
// corresponding agent.Tool entries and consume the resolved values.
type AssistantToolsConfig struct {
	LocationNormalize LocationNormalizeConfig
	UnitConvert       UnitConvertConfig
	Calculator        CalculatorConfig
	EntityResolve     EntityResolveConfig
}

// LocationNormalizeConfig — SCOPE-2 consumer.
type LocationNormalizeConfig struct {
	// Enabled gates registration of the location_normalize tool.
	Enabled bool
	// Provider selects the canonical-location backend. SCOPE-2 only
	// implements "open-meteo"; unknown providers fail loud at the
	// provider-selection point.
	Provider string
	// Timeout is the per-call deadline applied to provider lookups.
	Timeout time.Duration
	// CacheTTL is how long a resolved/ambiguous result stays in the
	// tool-local LRU cache.
	CacheTTL time.Duration
	// CacheMaxEntries bounds the LRU.
	CacheMaxEntries int
}

// UnitConvertConfig — SCOPE-3 consumer.
type UnitConvertConfig struct {
	// Enabled gates registration of the unit_convert tool.
	Enabled bool
	// CatalogVersion identifies the deterministic conversion catalog
	// the handler must load. Bumping forces a coordinated update of
	// catalog fixtures and source attribution.
	CatalogVersion string
}

// CalculatorConfig — SCOPE-3 consumer.
type CalculatorConfig struct {
	// Enabled gates registration of the calculator tool.
	Enabled bool
	// MaxExpressionChars caps the input expression length so the
	// parser cannot be DoS'd by an unbounded LLM prompt.
	MaxExpressionChars int
}

// EntityResolveConfig — SCOPE-4 consumer.
type EntityResolveConfig struct {
	// Enabled gates registration of the entity_resolve tool.
	Enabled bool
	// ConfidenceFloor is the [0,1] floor below which a candidate is
	// treated as ambiguous rather than resolved.
	ConfidenceFloor float64
	// Timeout is the per-call deadline applied to graph/vector
	// lookups during resolution.
	Timeout time.Duration
}

// loadAssistantToolsConfig populates cfg.Assistant.Tools from
// ASSISTANT_TOOLS_* env vars. It follows the same errs-collection
// pattern as the other loaders in this package so the top-level
// loadAssistantConfig can emit one aggregate fail-loud error.
//
// Every key is REQUIRED; missing/unparsable values are appended to
// errs and abort startup once the caller collects them.
func loadAssistantToolsConfig(cfg *Config, errs *[]string) {
	mustBool := func(key string, dst *bool) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		switch v {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			*errs = append(*errs, fmt.Sprintf("%s (must be \"true\"|\"false\", got %q)", key, v))
		}
	}
	mustString := func(key string, dst *string) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		*dst = v
	}
	mustInt := func(key string, dst *int, minVal int) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n < minVal {
			*errs = append(*errs, fmt.Sprintf("%s (must be >= %d, got %d)", key, minVal, n))
			return
		}
		*dst = n
	}
	mustFloatInRange := func(key string, dst *float64, lo, hi float64) {
		v := os.Getenv(key)
		if v == "" {
			*errs = append(*errs, key)
			return
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return
		}
		if f < lo || f > hi {
			*errs = append(*errs, fmt.Sprintf("%s (must be in [%g,%g], got %g)", key, lo, hi, f))
			return
		}
		*dst = f
	}

	// location_normalize
	mustBool("ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED", &cfg.Assistant.Tools.LocationNormalize.Enabled)
	mustString("ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER", &cfg.Assistant.Tools.LocationNormalize.Provider)
	var locTimeoutMs int
	mustInt("ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS", &locTimeoutMs, 1)
	cfg.Assistant.Tools.LocationNormalize.Timeout = time.Duration(locTimeoutMs) * time.Millisecond
	var locTTLSeconds int
	mustInt("ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS", &locTTLSeconds, 1)
	cfg.Assistant.Tools.LocationNormalize.CacheTTL = time.Duration(locTTLSeconds) * time.Second
	mustInt("ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES", &cfg.Assistant.Tools.LocationNormalize.CacheMaxEntries, 1)

	// unit_convert
	mustBool("ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED", &cfg.Assistant.Tools.UnitConvert.Enabled)
	mustString("ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION", &cfg.Assistant.Tools.UnitConvert.CatalogVersion)

	// calculator
	mustBool("ASSISTANT_TOOLS_CALCULATOR_ENABLED", &cfg.Assistant.Tools.Calculator.Enabled)
	mustInt("ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS", &cfg.Assistant.Tools.Calculator.MaxExpressionChars, 1)

	// entity_resolve
	mustBool("ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED", &cfg.Assistant.Tools.EntityResolve.Enabled)
	mustFloatInRange("ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR", &cfg.Assistant.Tools.EntityResolve.ConfidenceFloor, 0, 1)
	var erTimeoutMs int
	mustInt("ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS", &erTimeoutMs, 1)
	cfg.Assistant.Tools.EntityResolve.Timeout = time.Duration(erTimeoutMs) * time.Millisecond
}

// AssistantToolsRequiredKeys returns the canonical ordered list of
// REQUIRED ASSISTANT_TOOLS_* environment variables, in the order
// loadAssistantToolsConfig reads them. Exposed for the SCOPE-1 unit
// test and for documentation/audit consumers.
func AssistantToolsRequiredKeys() []string {
	return []string{
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES",
		"ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED",
		"ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION",
		"ASSISTANT_TOOLS_CALCULATOR_ENABLED",
		"ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS",
	}
}

// AssistantToolsMissingKeyError formats an aggregate fail-loud error
// for the spec 065 SCOPE-1 keys. Exposed for the dedicated test.
func AssistantToolsMissingKeyError(missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("[F065-SST-MISSING] missing or invalid required assistant micro-tools configuration: %s", strings.Join(missing, ", "))
}
