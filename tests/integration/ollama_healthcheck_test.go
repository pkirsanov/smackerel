//go:build integration

// spec 043 / BUG-002 — Adversarial healthcheck-binary guard for the
// pinned ollama image.
//
// Why this test exists:
//   - BUG-001 bumped the ollama image from `ollama/ollama:0.6` → `0.23.2`.
//     The new image ships only the `ollama` binary in PATH; it does NOT
//     ship `wget` or `curl`. The pre-fix `services.ollama.healthcheck.test`
//     in docker-compose.yml + deploy/compose.deploy.yml hardcoded a
//     `wget`-based probe, so every healthcheck invocation exited 127
//     ("executable file not found in $PATH"). `docker compose up -d --wait`
//     then exited 124 with `container smackerel-test-ollama-1 is unhealthy`,
//     blocking `./smackerel.sh test integration` at the stack-up step.
//
//   - This test parses BOTH live compose files and asserts the ollama
//     service's healthcheck command starts with a binary that actually
//     exists inside the pinned ollama image.
//
// SCN-BUG-002-001: live compose files MUST use an in-image binary.
//   Allowlist: `ollama` (or `/usr/bin/ollama` / `/bin/ollama`) — the only
//   binary the upstream ollama image guarantees in PATH.
//
// SCN-BUG-002-002: adversarial — synthetic compose snippets that put `wget`
//   or `curl` (CMD form OR CMD-SHELL form) in the ollama healthcheck MUST
//   be rejected with an error that names the offending binary AND the
//   image (`ollama/ollama:0.23.2`). Three sub-tests cover the matrix.
//
// References:
//   - specs/043-ollama-test-infrastructure/bugs/BUG-002-ollama-healthcheck-uses-missing-wget/spec.md
//   - specs/043-ollama-test-infrastructure/bugs/BUG-002-ollama-healthcheck-uses-missing-wget/design.md
//   - specs/043-ollama-test-infrastructure/bugs/BUG-002-ollama-healthcheck-uses-missing-wget/scopes.md (Scope 01, T01-01..T01-04)
//
// Hard constraints (per .github/copilot-instructions.md → Adversarial
// Regression Tests for Bug Fixes + spec 043 Scope 02 no-skip-guard):
//   - NO `t.Skip()` / `t.SkipNow()` / `t.Skipf(...)` calls anywhere in
//     this file. Missing file, parse failure, or contract violation are
//     all fail-loud conditions.
//   - The adversarial sub-tests MUST exercise the same helper as the live
//     test, with different synthetic inputs that are known to fail.

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// pinnedOllamaImage is the image whose binary set we assert against.
// Pinned here as a literal because the assertion message MUST cite the
// image — keeping it in lockstep with config/smackerel.yaml's
// `infrastructure.ollama.test.image` is enforced by other tests
// (tests/integration/ollama_image_availability_test.go +
// tests/integration/ollama_config_contract_test.go).
const pinnedOllamaImage = "ollama/ollama:0.23.2"

// inImageBinaryAllowlist enumerates the binaries the ollama image is
// known to ship in PATH (verified live via `docker exec smackerel-test-ollama-1
// which ollama` → `/usr/bin/ollama`). The first command token in the
// healthcheck MUST match one of these, OR be a path that resolves to
// `ollama`.
var inImageBinaryAllowlist = map[string]struct{}{
	"ollama":          {},
	"/usr/bin/ollama": {},
	"/bin/ollama":     {},
}

// forbiddenBinaries enumerates probe binaries that are NOT in the pinned
// ollama image. A regression that puts any of these as the first token
// MUST be rejected with a message naming the offending binary AND the
// image. Verified live (2026-05-10):
//
//	$ docker exec smackerel-test-ollama-1 wget --spider http://localhost:11434/api/tags
//	OCI runtime exec failed: exec failed: unable to start container process: exec: "wget": executable file not found in $PATH
//	$ docker exec smackerel-test-ollama-1 curl -sS http://localhost:11434/api/tags
//	OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH
var forbiddenBinaries = map[string]struct{}{
	"wget":          {},
	"curl":          {},
	"/usr/bin/wget": {},
	"/usr/bin/curl": {},
	"/bin/wget":     {},
	"/bin/curl":     {},
}

// healthcheckComposeDoc is the minimal YAML shape this contract needs.
// Only `services.ollama.healthcheck.test` is inspected.
type healthcheckComposeDoc struct {
	Services map[string]struct {
		Healthcheck struct {
			Test []string `yaml:"test"`
		} `yaml:"healthcheck"`
	} `yaml:"services"`
}

// extractOllamaHealthcheckFirstToken parses the healthcheck `test:` array
// and returns:
//   - the first command token (the binary name actually invoked)
//   - a boolean indicating whether the form was CMD-SHELL (true) or CMD (false)
//
// docker compose healthcheck syntax (per docker-compose schema):
//   - `["CMD", "binary", "arg1", …]` — exec form; first token is "CMD",
//     second token is the binary.
//   - `["CMD-SHELL", "binary args"]` — shell form; first token is
//     "CMD-SHELL", second token is a shell-string whose first
//     whitespace-delimited word is the binary.
//   - `["NONE"]` — disabled (we treat as a violation; the contract is that
//     a healthcheck MUST exist for ollama because spec 043 gates on it).
//
// Returns a non-nil error when the array is empty, when neither
// CMD/CMD-SHELL/NONE is the first token (compose schema guarantees one
// of those, but defend anyway), or when CMD-SHELL has no command body.
func extractOllamaHealthcheckFirstToken(testArray []string) (firstBinary string, viaCMDShell bool, err error) {
	if len(testArray) == 0 {
		return "", false, fmt.Errorf("healthcheck.test array is empty")
	}
	switch testArray[0] {
	case "NONE":
		return "", false, fmt.Errorf("healthcheck.test=[\"NONE\"] disables the healthcheck — spec 043 requires a real liveness probe for the ollama service")
	case "CMD":
		if len(testArray) < 2 {
			return "", false, fmt.Errorf("healthcheck.test=[\"CMD\"] is missing the binary name (need [\"CMD\", \"<binary>\", …])")
		}
		return testArray[1], false, nil
	case "CMD-SHELL":
		if len(testArray) < 2 {
			return "", true, fmt.Errorf("healthcheck.test=[\"CMD-SHELL\"] is missing the shell command body")
		}
		shellCmd := strings.TrimSpace(testArray[1])
		if shellCmd == "" {
			return "", true, fmt.Errorf("healthcheck.test=[\"CMD-SHELL\", \"\"] has empty command body")
		}
		// First whitespace-delimited token of the shell command body is
		// the binary that gets invoked first. Good enough for the
		// forbidden-binary check; we are not building a full shell
		// parser, just rejecting common probe-binary names.
		fields := strings.Fields(shellCmd)
		if len(fields) == 0 {
			return "", true, fmt.Errorf("healthcheck.test=[\"CMD-SHELL\", %q] has no whitespace-delimited tokens", testArray[1])
		}
		return fields[0], true, nil
	default:
		return "", false, fmt.Errorf("healthcheck.test first token %q is not one of CMD / CMD-SHELL / NONE (docker compose healthcheck schema violation)", testArray[0])
	}
}

// assertOllamaHealthcheckUsesInImageBinary returns nil iff the ollama
// service's healthcheck command starts with a binary that exists in the
// pinned ollama image. Returns a non-nil error naming the offending
// binary AND the image when the contract is violated, so the adversarial
// sub-tests can pattern-match the rejection.
func assertOllamaHealthcheckUsesInImageBinary(yamlBytes []byte) error {
	var doc healthcheckComposeDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	ollama, ok := doc.Services["ollama"]
	if !ok {
		return fmt.Errorf("contract violation: services.ollama not found in compose document — cannot inspect healthcheck")
	}

	firstBinary, viaCMDShell, err := extractOllamaHealthcheckFirstToken(ollama.Healthcheck.Test)
	if err != nil {
		return fmt.Errorf("contract violation: services.ollama.healthcheck malformed: %w", err)
	}

	formNote := ""
	if viaCMDShell {
		formNote = " (extracted from CMD-SHELL form)"
	}

	if _, isForbidden := forbiddenBinaries[firstBinary]; isForbidden {
		return fmt.Errorf(
			"contract violation: services.ollama.healthcheck first token %q%s is in the forbidden-binaries set "+
				"(binary not present in %s image — `docker exec … %s` returns exit 127 'executable file not found in $PATH'); "+
				"use `ollama list` instead",
			firstBinary, formNote, pinnedOllamaImage, firstBinary)
	}

	if _, ok := inImageBinaryAllowlist[firstBinary]; !ok {
		return fmt.Errorf(
			"contract violation: services.ollama.healthcheck first token %q%s is not in the in-image-binary allowlist for %s "+
				"(allowlist: %v). If the image now ships another viable probe binary, extend the allowlist explicitly with a verification log; do not rely on hope.",
			firstBinary, formNote, pinnedOllamaImage, sortedAllowlistKeys())
	}

	return nil
}

// sortedAllowlistKeys returns the allowlist keys for stable error
// messages.
func sortedAllowlistKeys() []string {
	out := make([]string, 0, len(inImageBinaryAllowlist))
	for k := range inImageBinaryAllowlist {
		out = append(out, k)
	}
	// Insertion order is non-deterministic over maps; sort lexically for
	// stable error text.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// repoRootForOllamaHealthcheckGuard climbs from CWD looking for
// docker-compose.yml. Independent of `go test` working dir.
func repoRootForOllamaHealthcheckGuard(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "deploy", "compose.deploy.yml")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s (need both docker-compose.yml and deploy/compose.deploy.yml present)", wd)
	return ""
}

// TestOllamaHealthcheck_LiveFiles is the primary contract assertion. It
// loads BOTH `docker-compose.yml` and `deploy/compose.deploy.yml` from
// the repo root and asserts the ollama service's healthcheck command
// starts with a binary that exists in `ollama/ollama:0.23.2`. Would FAIL
// if a future edit regresses either file to use `wget`, `curl`, or any
// other not-in-image binary.
//
// Fails loudly (no `t.Skip()`) when:
//   - either compose file is missing
//   - either compose file fails to parse
//   - the ollama service is missing from either file
//   - the healthcheck command is malformed (empty, NONE, missing CMD body)
//   - the first command token is in the forbidden-binaries set
//   - the first command token is not in the in-image-binary allowlist
func TestOllamaHealthcheck_LiveFiles(t *testing.T) {
	root := repoRootForOllamaHealthcheckGuard(t)

	composeFiles := []string{
		filepath.Join(root, "docker-compose.yml"),
		filepath.Join(root, "deploy", "compose.deploy.yml"),
	}

	for _, path := range composeFiles {
		yamlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read live compose file %q: %v", path, err)
		}
		if err := assertOllamaHealthcheckUsesInImageBinary(yamlBytes); err != nil {
			t.Fatalf("live compose file %q violates spec 043 / BUG-002 ollama-healthcheck contract: %v", path, err)
		}

		// Re-parse to log the actual first token (for explicit evidence
		// that the test is doing the work it claims to do).
		var doc healthcheckComposeDoc
		if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
			t.Fatalf("re-parse failed for %q: %v", path, err)
		}
		firstBinary, _, _ := extractOllamaHealthcheckFirstToken(doc.Services["ollama"].Healthcheck.Test)
		rel, _ := filepath.Rel(root, path)
		t.Logf("contract OK: %s ollama healthcheck first-token %q is in the in-image-binary allowlist", rel, firstBinary)
	}
}

// TestOllamaHealthcheck_AdversarialMissingBinary proves the contract
// catches a regression where the ollama healthcheck calls `wget` (CMD
// form). This is the exact failure mode of BUG-002 (modulo the original
// CMD-SHELL wrapper, which is covered by a separate adversarial sub-test
// below).
func TestOllamaHealthcheck_AdversarialMissingBinary(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ollama/ollama:0.23.2
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:11434/api/tags"]
      interval: 10s
`
	err := assertOllamaHealthcheckUsesInImageBinary([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: synthetic compose with `wget` healthcheck was accepted (the contract is tautological — it would NOT catch a regression that re-introduces a not-in-image probe binary)")
	}
	if !strings.Contains(err.Error(), `"wget"`) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the offending binary `wget`: %v", err)
	}
	if !strings.Contains(err.Error(), pinnedOllamaImage) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the pinned image %q (operator must know which image is missing the binary): %v", pinnedOllamaImage, err)
	}
	if !strings.Contains(err.Error(), "ollama list") {
		t.Fatalf("adversarial contract test failed: rejection error did not suggest the fix (`ollama list`): %v", err)
	}
	t.Logf("adversarial OK: synthetic compose with `wget` healthcheck rejected with: %v", err)
}

// TestOllamaHealthcheck_AdversarialMissingBinaryCurl proves the
// forbidden-binaries set is enforced for `curl` too, not just `wget`.
// `curl` is also not in the `ollama/ollama:0.23.2` image (verified live;
// see forbiddenBinaries comment).
func TestOllamaHealthcheck_AdversarialMissingBinaryCurl(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ollama/ollama:0.23.2
    healthcheck:
      test: ["CMD", "curl", "-sS", "http://localhost:11434/api/tags"]
      interval: 10s
`
	err := assertOllamaHealthcheckUsesInImageBinary([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: synthetic compose with `curl` healthcheck was accepted (the forbidden-binaries set is not enforced for `curl` — partial coverage)")
	}
	if !strings.Contains(err.Error(), `"curl"`) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the offending binary `curl`: %v", err)
	}
	if !strings.Contains(err.Error(), pinnedOllamaImage) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the pinned image %q: %v", pinnedOllamaImage, err)
	}
	t.Logf("adversarial OK: synthetic compose with `curl` healthcheck rejected with: %v", err)
}

// TestOllamaHealthcheck_AdversarialCMDShellWrappedWget proves the
// validator handles the `CMD-SHELL` form (the original BUG-002 shape),
// not just the bare `CMD` form. Pre-fix, the live compose files used
// exactly this form; the validator MUST reject it identically.
func TestOllamaHealthcheck_AdversarialCMDShellWrappedWget(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ollama/ollama:0.23.2
    healthcheck:
      test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://localhost:${OLLAMA_CONTAINER_PORT}/api/tags || exit 1"]
      interval: 10s
`
	err := assertOllamaHealthcheckUsesInImageBinary([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: synthetic compose with the original `CMD-SHELL` `wget …` form was accepted (the validator does not handle CMD-SHELL form — would have missed BUG-002)")
	}
	if !strings.Contains(err.Error(), `"wget"`) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the offending binary `wget` extracted from CMD-SHELL form: %v", err)
	}
	if !strings.Contains(err.Error(), "CMD-SHELL form") {
		t.Fatalf("adversarial contract test failed: rejection error did not annotate that the binary was extracted from CMD-SHELL form (operator must know which form to fix): %v", err)
	}
	if !strings.Contains(err.Error(), pinnedOllamaImage) {
		t.Fatalf("adversarial contract test failed: rejection error did not mention the pinned image %q: %v", pinnedOllamaImage, err)
	}
	t.Logf("adversarial OK: synthetic compose with `CMD-SHELL` `wget …` wrapper rejected with: %v", err)
}
