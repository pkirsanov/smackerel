//go:build integration

// Spec 061 BS-002-LLM-PROVIDER-TIMEOUT — SST contract for the per-env
// AGENT_PROVIDER_DEFAULT_MODEL override.
//
// Background: retrieval-qa-v1.yaml declares model_preference: "default" and
// limits.timeout_ms: 5000. The "default" tier routes through the env var
// AGENT_PROVIDER_DEFAULT_MODEL (see ml/app/agent.py::_PROVIDER_ENV_KEYS).
// On test hardware, warm gemma3:4b inference takes ~71s for a 2-token reply
// (captured verbatim in specs/061-conversational-assistant/report.md
// #round-56-defect3-verify), structurally unreachable inside 5000ms. The
// authorized fix is a per-env SST override that pins the default-tier model
// to qwen2.5:0.5b-instruct in the test environment ONLY, leaving dev /
// home-lab / prod on gemma3:4b via the base agent.provider_routing.default.model.
//
// This test enforces both halves of the override at the env-file boundary,
// so any future regression (silently dropping the override, or silently
// weakening the production binding) FAILs the integration suite naming the
// specific broken assertion.
//
// Pattern mirrored from tests/integration/ollama_config_contract_test.go
// (the SST→env-file boundary precedent).
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootForAgentProviderDefaultTestOverride climbs from CWD looking for
// config/smackerel.yaml. Independent of `go test` working dir.
func repoRootForAgentProviderDefaultTestOverride(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i = i + 1 {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

// TestAgentProviderDefaultModelTestOverride asserts the spec 061
// BS-002-LLM-PROVIDER-TIMEOUT fix at the SST→env-file boundary:
//
//  1. Test env (config/generated/test.env) MUST carry the per-env override
//     `AGENT_PROVIDER_DEFAULT_MODEL=qwen2.5:0.5b-instruct` so retrieval-qa-v1
//     (model_preference: "default", timeout_ms: 5000) is reachable on test
//     hardware.
//
//  2. Dev env (config/generated/dev.env) MUST keep the base production
//     binding `AGENT_PROVIDER_DEFAULT_MODEL=gemma3:4b` — the override is
//     test-only by design.
//
// Adversarial properties:
//
//   - If the SST override is silently dropped from config/smackerel.yaml or
//     from scripts/commands/config.sh, test.env will show gemma3:4b and the
//     first assertion FAILs naming the missing override.
//
//   - If the production binding is silently weakened (e.g. someone changes
//     agent.provider_routing.default.model itself to qwen2.5:0.5b-instruct
//     to "save a config line"), dev.env will show qwen and the second
//     assertion FAILs naming the broken production binding.
//
//   - Neither assertion early-returns; both are unconditional after the
//     env-file read. No bailout-style `if path missing { return }`.
func TestAgentProviderDefaultModelTestOverride(t *testing.T) {
	root := repoRootForAgentProviderDefaultTestOverride(t)

	testEnvPath := filepath.Join(root, "config", "generated", "test.env")
	testEnvBytes, err := os.ReadFile(testEnvPath)
	if err != nil {
		t.Fatalf("read generated test.env at %s: %v (regenerate via `./smackerel.sh config generate`)", testEnvPath, err)
	}
	testEnvText := string(testEnvBytes)

	devEnvPath := filepath.Join(root, "config", "generated", "dev.env")
	devEnvBytes, err := os.ReadFile(devEnvPath)
	if err != nil {
		t.Fatalf("read generated dev.env at %s: %v (regenerate via `./smackerel.sh config generate`)", devEnvPath, err)
	}
	devEnvText := string(devEnvBytes)

	const wantTestModelLine = "AGENT_PROVIDER_DEFAULT_MODEL=qwen2.5:0.5b-instruct"
	const wantDevModelLine = "AGENT_PROVIDER_DEFAULT_MODEL=gemma3:4b"

	if !strings.Contains(testEnvText, wantTestModelLine) {
		t.Errorf("generated test.env must contain %q (spec 061 BS-002-LLM-PROVIDER-TIMEOUT fix); got line: %q",
			wantTestModelLine,
			findAgentProviderDefaultLine(testEnvText),
		)
	}

	if !strings.Contains(devEnvText, wantDevModelLine) {
		t.Errorf("generated dev.env must contain %q (production binding preserved — override is test-only); got line: %q",
			wantDevModelLine,
			findAgentProviderDefaultLine(devEnvText),
		)
	}

	t.Logf("test.env pins AGENT_PROVIDER_DEFAULT_MODEL=qwen2.5:0.5b-instruct; dev.env keeps AGENT_PROVIDER_DEFAULT_MODEL=gemma3:4b (per-env override working)")
}

// findAgentProviderDefaultLine returns the first line in `text` whose key is
// AGENT_PROVIDER_DEFAULT_MODEL, or the literal string `<not found>`.
// Diagnostic helper for the error messages above.
func findAgentProviderDefaultLine(text string) string {
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(ln, "AGENT_PROVIDER_DEFAULT_MODEL=") {
			return ln
		}
	}
	return "<not found>"
}
