// Spec 061 SCOPE-06c (Round 71d) — TestMain sets the env vars the scenario
// loader expands via os.ExpandEnv (e.g. retrieval-qa-v1.yaml's
// `timeout_ms: ${RETRIEVAL_QA_TIMEOUT_MS}`). The actual values come from
// `./smackerel.sh config generate` at runtime; for unit tests we pin to the
// cpu-tier interactive cell so the loader sees integer values and the
// rule-based validators don't reject the contract.
package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("RETRIEVAL_QA_TIMEOUT_MS") == "" {
		os.Setenv("RETRIEVAL_QA_TIMEOUT_MS", "15000")
	}
	if os.Getenv("RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS") == "" {
		os.Setenv("RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS", "7500")
	}
	// BUG-061-003 — recipe-search-v1 expansion.
	if os.Getenv("RECIPE_SEARCH_TIMEOUT_MS") == "" {
		os.Setenv("RECIPE_SEARCH_TIMEOUT_MS", "15000")
	}
	if os.Getenv("RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS") == "" {
		os.Setenv("RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS", "7500")
	}
	os.Exit(m.Run())
}
