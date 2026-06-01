// Spec 065 SCOPE-1 — assistant micro-tools SST fail-loud test.

package config

import (
	"strings"
	"testing"
)

// TestAssistantToolsConfigRequiresEveryMicroToolKey asserts every
// ASSISTANT_TOOLS_* key listed in AssistantToolsRequiredKeys is
// REQUIRED at the generator boundary (smackerel-no-defaults /
// Gate G028). When any key is missing or unparsable the loader
// records it in errs and the aggregate AssistantToolsMissingKeyError
// names every offender with the F065-SST-MISSING tag.
func TestAssistantToolsConfigRequiresEveryMicroToolKey(t *testing.T) {
	required := AssistantToolsRequiredKeys()

	// Sub-test 1: every key missing → loader names every required key
	// in the aggregate error.
	t.Run("all_missing_names_every_key", func(t *testing.T) {
		for _, k := range required {
			t.Setenv(k, "")
		}
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) == 0 {
			t.Fatalf("expected loadAssistantToolsConfig to record missing keys, got none")
		}
		joined := strings.Join(errs, ",")
		for _, k := range required {
			if !strings.Contains(joined, k) {
				t.Errorf("expected missing-key error to name %q, got %q", k, joined)
			}
		}
		err := AssistantToolsMissingKeyError(errs)
		if err == nil {
			t.Fatalf("expected non-nil aggregate error")
		}
		if !strings.Contains(err.Error(), "[F065-SST-MISSING]") {
			t.Fatalf("expected aggregate error to carry [F065-SST-MISSING] tag, got %q", err.Error())
		}
	})

	// Sub-test 2: adversarial — removing only ONE key still names
	// that key. This is the regression-anchor for the SCN-065-A07
	// scenario "missing location_normalize.provider fails loud".
	t.Run("missing_only_location_provider_names_that_key", func(t *testing.T) {
		setAllAssistantToolsKeys(t)
		t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER", "")
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) != 1 {
			t.Fatalf("expected exactly one missing-key error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0], "ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER") {
			t.Fatalf("expected the missing-key error to name ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER, got %q", errs[0])
		}
	})

	// Sub-test 3: fully-populated env produces zero errors and every
	// field is populated correctly.
	t.Run("fully_populated_no_errors", func(t *testing.T) {
		setAllAssistantToolsKeys(t)
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) != 0 {
			t.Fatalf("expected zero errs, got %v", errs)
		}
		tc := cfg.Assistant.Tools
		if !tc.LocationNormalize.Enabled || tc.LocationNormalize.Provider != "open-meteo" ||
			tc.LocationNormalize.Timeout.Milliseconds() != 2000 ||
			tc.LocationNormalize.CacheTTL.Seconds() != 600 ||
			tc.LocationNormalize.CacheMaxEntries != 512 {
			t.Fatalf("loaded LocationNormalize does not match env: %+v", tc.LocationNormalize)
		}
		if !tc.UnitConvert.Enabled || tc.UnitConvert.CatalogVersion != "v1" {
			t.Fatalf("loaded UnitConvert does not match env: %+v", tc.UnitConvert)
		}
		if !tc.Calculator.Enabled || tc.Calculator.MaxExpressionChars != 256 {
			t.Fatalf("loaded Calculator does not match env: %+v", tc.Calculator)
		}
		if !tc.EntityResolve.Enabled || tc.EntityResolve.ConfidenceFloor != 0.7 ||
			tc.EntityResolve.Timeout.Milliseconds() != 1500 {
			t.Fatalf("loaded EntityResolve does not match env: %+v", tc.EntityResolve)
		}
	})

	// Sub-test 4: out-of-range confidence floor is rejected loudly.
	t.Run("confidence_floor_out_of_range_rejected", func(t *testing.T) {
		setAllAssistantToolsKeys(t)
		t.Setenv("ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR", "1.5")
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) != 1 || !strings.Contains(errs[0], "ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR") {
			t.Fatalf("expected single out-of-range error for confidence floor, got %v", errs)
		}
	})

	// Sub-test 5: bool key must be strict.
	t.Run("non_strict_bool_rejected", func(t *testing.T) {
		setAllAssistantToolsKeys(t)
		t.Setenv("ASSISTANT_TOOLS_CALCULATOR_ENABLED", "1")
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) != 1 || !strings.Contains(errs[0], "ASSISTANT_TOOLS_CALCULATOR_ENABLED") {
			t.Fatalf("expected single strict-bool error for calculator enabled, got %v", errs)
		}
	})
}

// setAllAssistantToolsKeys populates every ASSISTANT_TOOLS_* env var
// with the literal values shipped by config/smackerel.yaml (kept in
// sync; if the yaml changes a value the test should change with it).
func setAllAssistantToolsKeys(t *testing.T) {
	t.Helper()
	t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED", "true")
	t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER", "open-meteo")
	t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS", "2000")
	t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS", "600")
	t.Setenv("ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES", "512")
	t.Setenv("ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED", "true")
	t.Setenv("ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION", "v1")
	t.Setenv("ASSISTANT_TOOLS_CALCULATOR_ENABLED", "true")
	t.Setenv("ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS", "256")
	t.Setenv("ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED", "true")
	t.Setenv("ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR", "0.7")
	t.Setenv("ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS", "1500")
}
