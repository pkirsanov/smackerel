//go:build integration

// Spec 043 Scope 1 — T1-01 + T1-02 SST validation contract.
//
// SCN-OLLAMA-006 requires `infrastructure.ollama.test.image`,
// `infrastructure.ollama.test.model`, and `environments.test.ollama_host_port`
// to be `required_value` keys in `scripts/commands/config.sh`. This test
// proves fail-loud at the SST→env-file boundary by:
//
//  1. Asserting `config/generated/test.env` carries every Ollama SST key
//     (proves the generator emits them — combined with the env_file drift
//     guard inside `./smackerel.sh check`, this proves SST→env round-trip
//     stays in sync).
//  2. Adversarial: stripping `infrastructure.ollama.test.model` from a
//     temp copy of `config/smackerel.yaml` causes the generator to exit
//     non-zero and name the missing key in stderr.
//  3. Adversarial: stripping `infrastructure.ollama.test.image` likewise
//     causes the generator to fail-loud naming the key.
//
// References:
//   - specs/043-ollama-test-infrastructure/scopes.md (Scope 1, T1-01/T1-02)
//   - specs/043-ollama-test-infrastructure/design.md §3 (configuration plan)
//   - tests/integration/drive/drive_config_contract_test.go (pattern source)
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoRootForOllamaSSTContract climbs from CWD looking for
// config/smackerel.yaml. Independent of `go test` working dir.
func repoRootForOllamaSSTContract(t *testing.T) string {
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

// TestOllamaConfigGenerateAndRuntimeValidationStayInSync is the primary
// SST contract. It asserts the generator emits every required Ollama key
// to `config/generated/test.env`, then runs two adversarial config-strip
// runs to prove fail-loud at the SST→env-file boundary.
func TestOllamaConfigGenerateAndRuntimeValidationStayInSync(t *testing.T) {
	root := repoRootForOllamaSSTContract(t)
	srcYAML := filepath.Join(root, "config", "smackerel.yaml")

	envPath := filepath.Join(root, "config", "generated", "test.env")
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read generated test.env: %v", err)
	}
	envText := string(envBytes)
	required := []string{
		"OLLAMA_IMAGE=",
		"OLLAMA_TEST_MODEL=",
		"OLLAMA_TEST_PULL_TIMEOUT_SECONDS=",
		"OLLAMA_TEST_REQUEST_TEMPERATURE=",
		"OLLAMA_TEST_REQUEST_TOP_P=",
		"OLLAMA_TEST_REQUEST_TOP_K=",
		"OLLAMA_TEST_REQUEST_SEED=",
		"OLLAMA_TEST_REQUEST_NUM_PREDICT=",
		"OLLAMA_HOST_PORT=",
		"OLLAMA_VOLUME_NAME=",
		"OLLAMA_CONTAINER_PORT=",
		"ENABLE_OLLAMA=",
	}
	for _, want := range required {
		if !strings.Contains(envText, want) {
			t.Errorf("generated test.env missing %q", want)
		}
	}
	// SCN-OLLAMA-001 — test env auto-enables Ollama.
	if !strings.Contains(envText, "ENABLE_OLLAMA=true") {
		t.Errorf("generated test.env should have ENABLE_OLLAMA=true (per environments.test.ollama_enabled=true); got envText with: %q", findLine(envText, "ENABLE_OLLAMA"))
	}
	// SCN-OLLAMA-006 — test env carries pinned deterministic model.
	if !strings.Contains(envText, "OLLAMA_TEST_MODEL=qwen2.5:0.5b-instruct") {
		t.Errorf("generated test.env should pin OLLAMA_TEST_MODEL=qwen2.5:0.5b-instruct; got: %q", findLine(envText, "OLLAMA_TEST_MODEL"))
	}
	t.Logf("generated test.env contains every required OLLAMA_* key (%d keys checked) with deterministic test model pinned", len(required))

	// Adversarial 1: strip infrastructure.ollama.test.model — generator must fail-loud.
	srcBytes, err := os.ReadFile(srcYAML)
	if err != nil {
		t.Fatalf("read source yaml: %v", err)
	}
	t.Run("AdversarialMissingTestModel", func(t *testing.T) {
		assertConfigGenerateFailsLoudWhenStripped(
			t, root, srcBytes,
			"      model: qwen2.5:0.5b-instruct",
			"infrastructure.ollama.test.model",
		)
	})

	// Adversarial 2: strip infrastructure.ollama.test.image — generator must fail-loud.
	// spec 043 / BUG-001 (2026-05-10) — strip target updated to track the live
	// re-pinned tag (`0.23.2`); pre-fix this fixture targeted `0.6` which is
	// yanked from Docker Hub.
	t.Run("AdversarialMissingTestImage", func(t *testing.T) {
		assertConfigGenerateFailsLoudWhenStripped(
			t, root, srcBytes,
			"      image: ollama/ollama:0.23.2",
			"infrastructure.ollama.test.image",
		)
	})

	// Adversarial 3: strip infrastructure.ollama.test.request_seed — generator must fail-loud.
	// Validates determinism knobs are required (not optional with a fallback).
	t.Run("AdversarialMissingRequestSeed", func(t *testing.T) {
		assertConfigGenerateFailsLoudWhenStripped(
			t, root, srcBytes,
			"      request_seed: 42",
			"infrastructure.ollama.test.request_seed",
		)
	})
}

// assertConfigGenerateFailsLoudWhenStripped writes a temp copy of the
// source YAML with `targetLine` removed, runs config.sh against it, and
// asserts the run exits non-zero and the output mentions `expectedKey`.
// Tautological-guard: the function fails the test if `targetLine` is not
// found exactly once in the source (would mean the YAML moved and we'd
// silently strip nothing).
func assertConfigGenerateFailsLoudWhenStripped(
	t *testing.T,
	root string,
	srcBytes []byte,
	targetLine string,
	expectedKey string,
) {
	t.Helper()
	if !strings.Contains(string(srcBytes), targetLine) {
		t.Fatalf("source yaml missing expected target line %q (yaml block moved? The strip would silently strip nothing, making the adversarial test tautological.)", targetLine)
	}
	stripped := 0
	out := make([]string, 0)
	for _, ln := range strings.Split(string(srcBytes), "\n") {
		if ln == targetLine {
			stripped = stripped + 1
			continue
		}
		out = append(out, ln)
	}
	if stripped == 0 {
		t.Fatalf("expected to strip exactly one line matching %q; stripped=%d", targetLine, stripped)
	}
	tmpYAML := filepath.Join(t.TempDir(), "smackerel.yaml")
	if err := os.WriteFile(tmpYAML, []byte(strings.Join(out, "\n")), 0o600); err != nil {
		t.Fatalf("write stripped yaml: %v", err)
	}

	advCtx, advCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer advCancel()
	advCmd := exec.CommandContext(advCtx, "bash",
		filepath.Join(root, "scripts", "commands", "config.sh"),
		"--config", tmpYAML,
		"--env", "test",
	)
	advCmd.Env = append(os.Environ(), "TARGET_ENV_GUARD=integration-043-001-adv")
	advOut, advErr := advCmd.CombinedOutput()
	advExit := 0
	if advErr != nil {
		if ee, ok := advErr.(*exec.ExitError); ok {
			advExit = ee.ExitCode()
		} else {
			t.Fatalf("run adversarial config.sh: %v output=%s", advErr, string(advOut))
		}
	}
	t.Logf("adversarial config.sh exit=%d (expected non-zero) output=%s", advExit, strings.TrimSpace(string(advOut)))
	if advExit == 0 {
		t.Fatalf("adversarial config.sh exit=0 with missing %s; expected non-zero (SST fail-loud violated)", expectedKey)
	}
	if !strings.Contains(string(advOut), expectedKey) {
		t.Errorf("adversarial output does not name the missing key %q: %s", expectedKey, string(advOut))
	}
}

// findLine returns the first line in `text` containing `prefix`, or empty
// string if none. Used for diagnostic-only error messages above.
func findLine(text, prefix string) string {
	for _, ln := range strings.Split(text, "\n") {
		if strings.Contains(ln, prefix) {
			return ln
		}
	}
	return ""
}
