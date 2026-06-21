// Drift-detector contract test for the spec 099 resource pre-flight guard.
//
// This test parses the LIVE smackerel.sh + config pipeline (config.sh,
// config/smackerel.yaml, and the generated env files) and asserts that the
// resource guard is actually wired in, mirroring the style of
// internal/deploy/compose_contract_test.go:
//
//  1. The helper smackerel_assert_host_resources() is defined AND invokes the
//     Go evaluator (cmd/preflight) — proving the guard runs real logic.
//  2. Every heavy-op command path (build, up, test integration|e2e|e2e-ui|
//     stress) and the standalone pre-flight command invoke the helper.
//  3. The SST thresholds (runtime.preflight.*) are present in
//     config/smackerel.yaml, emitted by config.sh via required_value, and carried
//     by the generated env files with positive integer values.
//
// Two adversarial sub-tests prove the contract is non-tautological: removing the
// guard from the build block, or removing the cmd/preflight invocation from the
// helper, makes assertGuardWired REJECT.
//
// References:
//   - specs/099-preflight-resource-guard/spec.md
//   - specs/099-preflight-resource-guard/design.md (Contract-test design)
package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// repoRoot climbs two directories up from this test file
// (internal/preflight/ -> repo root), independent of `go test` CWD so it works
// both from `cd internal/preflight && go test` and `cd /workspace && go test ./...`.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// heavyGuardedOps are the command paths that MUST invoke the resource guard.
// pre-flight is the standalone check; the rest are the heavy ops.
var heavyGuardedOps = []string{
	"build", "up", "integration", "e2e", "stress", "e2e-ui", "pre-flight",
}

// caseBlockStr returns the body of a shell `case` arm (from `<label>)` to its
// terminating `;;`), correctly skipping `;;` inside nested case…esac blocks via
// depth tracking. Returns an error if the label is not found.
func caseBlockStr(script, label string) (string, error) {
	re := regexp.MustCompile(`(?m)^[[:space:]]*` + regexp.QuoteMeta(label) + `\)[[:space:]]*$`)
	loc := re.FindStringIndex(script)
	if loc == nil {
		return "", fmt.Errorf("case label %q) not found", label)
	}
	var b strings.Builder
	depth := 0
	for _, line := range strings.Split(script[loc[1]:], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "case ") && strings.HasSuffix(trimmed, " in") {
			depth++
		}
		if trimmed == "esac" && depth > 0 {
			depth--
		}
		if depth == 0 && strings.Contains(line, ";;") {
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// helperBodyStr returns the body of the smackerel_assert_host_resources()
// function (decl to the next top-level `}`), or an error if absent/unclosed.
func helperBodyStr(script string) (string, error) {
	const decl = "smackerel_assert_host_resources() {"
	i := strings.Index(script, decl)
	if i < 0 {
		return "", fmt.Errorf("helper smackerel_assert_host_resources is not defined")
	}
	rest := script[i:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return "", fmt.Errorf("helper smackerel_assert_host_resources is not closed")
	}
	return rest[:end], nil
}

// assertGuardWired returns nil iff the resource guard is fully wired into
// script. On any violation it returns a non-nil error naming the specific gap,
// so the adversarial sub-tests can pattern-match the failure mode.
func assertGuardWired(script string) error {
	body, err := helperBodyStr(script)
	if err != nil {
		return err
	}
	// Assert the helper actually INVOKES the evaluator via one of the two real
	// command forms — not merely mentions "cmd/preflight" in a comment. The
	// host path runs `go run ./cmd/preflight`; the dockerized fallback runs
	// scripts/runtime/preflight.sh (which itself runs cmd/preflight, asserted
	// separately by TestGuardWiring_LiveFile).
	if !strings.Contains(body, "go run ./cmd/preflight") && !strings.Contains(body, "scripts/runtime/preflight.sh") {
		return fmt.Errorf("helper smackerel_assert_host_resources does not invoke the Go evaluator (no `go run ./cmd/preflight` and no scripts/runtime/preflight.sh call) — the guard would not run real logic")
	}
	for _, op := range heavyGuardedOps {
		block, berr := caseBlockStr(script, op)
		if berr != nil {
			return fmt.Errorf("command %q): %w", op, berr)
		}
		if !strings.Contains(block, "smackerel_assert_host_resources") {
			return fmt.Errorf("command %q) block does not invoke smackerel_assert_host_resources — resource guard not wired into this heavy-op path", op)
		}
	}
	return nil
}

// --- Live-file contract ----------------------------------------------------

func TestGuardWiring_LiveFile(t *testing.T) {
	root := repoRoot(t)
	script := readFile(t, root, "smackerel.sh")
	if err := assertGuardWired(script); err != nil {
		t.Fatalf("resource guard wiring contract violated in smackerel.sh: %v", err)
	}

	// The helper prefers host go run and falls back to the dockerized wrapper.
	body, err := helperBodyStr(script)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go run ./cmd/preflight", "scripts/runtime/preflight.sh"} {
		if !strings.Contains(body, want) {
			t.Fatalf("helper body missing %q; got:\n%s", want, body)
		}
	}

	// The dockerized fallback wrapper exists and runs the evaluator.
	wrapper := readFile(t, root, "scripts/runtime/preflight.sh")
	if !strings.Contains(wrapper, "go run ./cmd/preflight") {
		t.Fatalf("scripts/runtime/preflight.sh does not run cmd/preflight; got:\n%s", wrapper)
	}
}

// --- Adversarial: prove the contract is non-tautological -------------------

func TestGuardWiring_AdversarialMissingBuildGuard(t *testing.T) {
	root := repoRoot(t)
	script := readFile(t, root, "smackerel.sh")

	// Surgically remove the guard from the build block only (the build-specific
	// 3-line sequence pins it to build, not up/pre-flight which share the call).
	const buildWithGuard = "smackerel_generate_config \"$TARGET_ENV\" >/dev/null\n    smackerel_assert_host_resources \"$TARGET_ENV\"\n    build_args=(build)"
	const buildWithoutGuard = "smackerel_generate_config \"$TARGET_ENV\" >/dev/null\n    build_args=(build)"
	if !strings.Contains(script, buildWithGuard) {
		t.Fatalf("precondition failed: build block guard sequence not found verbatim — update this adversarial test")
	}
	mutated := strings.Replace(script, buildWithGuard, buildWithoutGuard, 1)

	err := assertGuardWired(mutated)
	if err == nil {
		t.Fatal("expected assertGuardWired to REJECT a build block with the guard removed, got nil (tautological test?)")
	}
	if !strings.Contains(err.Error(), `"build"`) {
		t.Fatalf("expected the rejection to name the build path, got: %v", err)
	}
}

func TestGuardWiring_AdversarialHelperNotRunningEvaluator(t *testing.T) {
	root := repoRoot(t)
	script := readFile(t, root, "smackerel.sh")

	// Neuter the helper so it no longer invokes the Go evaluator.
	mutated := strings.ReplaceAll(script, "go run ./cmd/preflight", "true # disabled")
	mutated = strings.ReplaceAll(mutated, "scripts/runtime/preflight.sh", "scripts/runtime/disabled.sh")

	err := assertGuardWired(mutated)
	if err == nil {
		t.Fatal("expected assertGuardWired to REJECT a helper that does not run cmd/preflight, got nil")
	}
	if !strings.Contains(err.Error(), "cmd/preflight") {
		t.Fatalf("expected the rejection to cite the missing cmd/preflight invocation, got: %v", err)
	}
}

// --- Config / SST wiring ---------------------------------------------------

func TestConfigWiring_YamlAndConfigScript(t *testing.T) {
	root := repoRoot(t)

	yaml := readFile(t, root, "config/smackerel.yaml")
	for _, want := range []string{"preflight:", "min_available_ram_mb:", "min_available_disk_gb:"} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("config/smackerel.yaml missing %q (SST source of truth for the thresholds)", want)
		}
	}

	configSh := readFile(t, root, "scripts/commands/config.sh")
	// Read with fail-loud required_value (NO-DEFAULTS).
	for _, want := range []string{
		"required_value runtime.preflight.min_available_ram_mb",
		"required_value runtime.preflight.min_available_disk_gb",
	} {
		if !strings.Contains(configSh, want) {
			t.Fatalf("config.sh does not read %q via required_value (fail-loud SST)", want)
		}
	}
	// Emit both keys into the generated env file.
	for _, want := range []string{
		"PREFLIGHT_MIN_AVAILABLE_RAM_MB=${PREFLIGHT_MIN_AVAILABLE_RAM_MB}",
		"PREFLIGHT_MIN_AVAILABLE_DISK_GB=${PREFLIGHT_MIN_AVAILABLE_DISK_GB}",
	} {
		if !strings.Contains(configSh, want) {
			t.Fatalf("config.sh does not emit %q into the generated env file", want)
		}
	}
}

// TestConfigWiring_GeneratedEnvCarriesThresholds proves the thresholds reach the
// generated env files with positive integer values, parsed by the SAME
// production parser the guard uses (LoadEnvFile + ParseThresholds). Skips when a
// file is absent (fresh checkout that has not run `config generate`).
func TestConfigWiring_GeneratedEnvCarriesThresholds(t *testing.T) {
	root := repoRoot(t)
	for _, env := range []string{"dev", "test"} {
		rel := filepath.Join("config", "generated", env+".env")
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s not present (run ./smackerel.sh config generate); skipping", rel)
			continue
		}
		m, err := LoadEnvFile(path)
		if err != nil {
			t.Fatalf("LoadEnvFile(%s): %v", rel, err)
		}
		th, err := ParseThresholds(m)
		if err != nil {
			t.Fatalf("%s does not carry valid preflight thresholds: %v", rel, err)
		}
		if th.MinAvailableRAMMB <= 0 || th.MinAvailableDiskGB <= 0 {
			t.Fatalf("%s thresholds must be positive, got %+v", rel, th)
		}
	}
}
