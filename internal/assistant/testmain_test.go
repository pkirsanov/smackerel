// Spec 061 SCOPE-06c (Round 71d) — TestMain pins the env vars the scenario
// loader expands via os.ExpandEnv (retrieval-qa-v1.yaml's
// `timeout_ms: ${RETRIEVAL_QA_TIMEOUT_MS}`) so unit tests that load live
// prompt_contracts see integer values for the rule-based validators.
package assistant

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
	os.Exit(m.Run())
}
