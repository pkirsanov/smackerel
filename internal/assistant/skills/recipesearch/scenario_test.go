// BUG-061-003 — recipe_search scenario contract regression.
//
// S01 (clean "find best recipe") and S02 (misspelled "find best
// recepie") routing to the recipe_search scenario at BandHigh is
// proven at the router boundary by
// internal/agent/normalize_test.go::TestRouter_NormalizesBeforeEmbed_BUG061003
// (the normalizer pre-pass rewrites the misspelled form into the
// canonical form before embedding, so by the time the executor —
// and therefore this skill's assembler — sees the request, both
// inputs are indistinguishable from each other).
//
// This file pins the scenario contract that BOTH S01 and S02 depend
// on, so any change to the scenario id or the user-facing YAML
// shape that would break end-to-end recipe-search routing is caught
// here BEFORE the integration suite runs.
package recipesearch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRecipeSearchScenarioContract_BUG061003 — assert the scenario
// id matches the manifest entry and the prompt contract file is
// present and shaped as recipe-search-v1 with the rule-3 empty-
// graph contract the assembler depends on.
func TestRecipeSearchScenarioContract_BUG061003(t *testing.T) {
	t.Parallel()

	if ScenarioID != "recipe_search" {
		t.Fatalf("ScenarioID = %q; want %q (manifest + assembler wiring depend on this)", ScenarioID, "recipe_search")
	}

	// Locate the prompt contract relative to this test file so the
	// test works in any working directory.
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	contract := filepath.Join(repoRoot, "config", "prompt_contracts", "recipe-search-v1.yaml")

	data, err := os.ReadFile(contract)
	if err != nil {
		t.Fatalf("read %s: %v", contract, err)
	}
	s := string(data)

	for _, must := range []string{
		"id: recipe_search",
		"version: \"recipe-search-v1\"",
		"type: \"scenario\"",
		"name: recipe_search", // allowed_tools entry
		"required: [ answer, cited_artifact_ids ]",
		"${RECIPE_SEARCH_TIMEOUT_MS}",
		"${RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS}",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("recipe-search-v1.yaml MUST contain %q", must)
		}
	}
}
