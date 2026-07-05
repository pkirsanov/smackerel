// Drift-detector contract test for the spec 099 resource pre-flight guard.
//
// This test parses the LIVE smackerel.sh + config pipeline (config.sh,
// config/smackerel.yaml, and the generated env files) and asserts that the
// resource guard is actually wired in, mirroring the style of
// internal/deploy/compose_contract_test.go:
//
//  1. The evaluator-carrying helper smackerel_assert_host_resources_profile()
//     is defined AND invokes the Go evaluator (cmd/preflight) — proving the
//     guard runs real logic — and the thin back-compat wrapper
//     smackerel_assert_host_resources() delegates to it (BUG-099-001 made the
//     path selection OS-aware, moving the evaluator into the _profile helper).
//  2. Every heavy-op command path (build, up, test integration|e2e|e2e-ui|
//     stress) and the standalone pre-flight command invoke the wrapper.
//  3. The SST thresholds (runtime.preflight.*) are present in
//     config/smackerel.yaml, emitted by config.sh via required_value, and carried
//     by the generated env files with positive integer values.
//
// Two adversarial sub-tests prove the contract is non-tautological: removing the
// guard from the build block, or removing the cmd/preflight invocation from the
// evaluator-carrying helper, makes assertGuardWired REJECT.
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

// heavyGuardedOps are the command paths that MUST invoke the HEAVY resource
// guard (the 6000 MB floor). pre-flight is the standalone check; the rest are
// the heavy ops. NOTE: e2e-ui is deliberately ABSENT — spec 100 F-100-OPT-02
// moved it to the LOWER `ui` profile (the no-ML PWA browser lane), which is
// locked separately by TestGuardWiring_E2EUILaneUsesUIProfile.
var heavyGuardedOps = []string{
	"build", "up", "integration", "e2e", "stress", "pre-flight",
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

// After BUG-099-001's OS-aware refactor the Go evaluator moved into
// smackerel_assert_host_resources_profile(); smackerel_assert_host_resources()
// became a thin back-compat wrapper that delegates to it with the `heavy`
// profile (the heavy-op case blocks still call the wrapper).
const (
	evaluatorHelper = "smackerel_assert_host_resources_profile"
	guardWrapper    = "smackerel_assert_host_resources"
)

// funcBodyStr returns the body of the shell function `<name>() {` (decl to the
// next top-level `}` on its own line), or an error if absent/unclosed. Because
// the search literal ends in `() {`, requesting guardWrapper matches the
// wrapper decl and NOT the longer `..._profile() {` decl.
func funcBodyStr(script, name string) (string, error) {
	decl := name + "() {"
	i := strings.Index(script, decl)
	if i < 0 {
		return "", fmt.Errorf("helper %s is not defined", name)
	}
	rest := script[i:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return "", fmt.Errorf("helper %s is not closed", name)
	}
	return rest[:end], nil
}

// assertGuardWired returns nil iff the resource guard is fully wired into
// script. On any violation it returns a non-nil error naming the specific gap,
// so the adversarial sub-tests can pattern-match the failure mode.
func assertGuardWired(script string) error {
	// (1) The evaluator-carrying helper (post-BUG-099-001) MUST INVOKE the
	// evaluator via one of the two real command forms — not merely mention
	// "cmd/preflight" in a comment. The Linux host path runs
	// `go run ./cmd/preflight`; the macOS/dockerized path runs
	// scripts/runtime/preflight.sh (which itself runs cmd/preflight, asserted
	// separately by TestGuardWiring_LiveFile).
	body, err := funcBodyStr(script, evaluatorHelper)
	if err != nil {
		return err
	}
	if !strings.Contains(body, "go run ./cmd/preflight") && !strings.Contains(body, "scripts/runtime/preflight.sh") {
		return fmt.Errorf("helper %s does not invoke the Go evaluator (no `go run ./cmd/preflight` and no scripts/runtime/preflight.sh call) — the guard would not run real logic", evaluatorHelper)
	}
	// (2) The thin back-compat wrapper MUST delegate to the evaluator-carrying
	// helper, otherwise the heavy-op case blocks (which call the wrapper, not
	// _profile directly) would never reach the evaluator.
	wrapperBody, werr := funcBodyStr(script, guardWrapper)
	if werr != nil {
		return werr
	}
	if !strings.Contains(wrapperBody, evaluatorHelper) {
		return fmt.Errorf("wrapper %s does not delegate to %s — the heavy-op paths would not reach the Go evaluator", guardWrapper, evaluatorHelper)
	}
	// (3) Every heavy-op case block MUST invoke the guard. guardWrapper is a
	// prefix of evaluatorHelper, so a direct _profile call also satisfies this.
	for _, op := range heavyGuardedOps {
		block, berr := caseBlockStr(script, op)
		if berr != nil {
			return fmt.Errorf("command %q): %w", op, berr)
		}
		if !strings.Contains(block, guardWrapper) {
			return fmt.Errorf("command %q) block does not invoke %s — resource guard not wired into this heavy-op path", op, guardWrapper)
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

	// The evaluator-carrying helper selects host `go run` on Linux and falls
	// back to the dockerized runner on macOS; assert BOTH real command forms.
	body, err := funcBodyStr(script, evaluatorHelper)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go run ./cmd/preflight", "scripts/runtime/preflight.sh"} {
		if !strings.Contains(body, want) {
			t.Fatalf("helper %s body missing %q; got:\n%s", evaluatorHelper, want, body)
		}
	}

	// The thin back-compat wrapper delegates to the evaluator-carrying helper,
	// so the heavy-op paths that call the wrapper actually reach the evaluator.
	wrapperBody, werr := funcBodyStr(script, guardWrapper)
	if werr != nil {
		t.Fatal(werr)
	}
	if !strings.Contains(wrapperBody, evaluatorHelper) {
		t.Fatalf("wrapper %s does not delegate to %s; got:\n%s", guardWrapper, evaluatorHelper, wrapperBody)
	}

	// The dockerized fallback wrapper exists and runs the evaluator.
	wrapper := readFile(t, root, "scripts/runtime/preflight.sh")
	if !strings.Contains(wrapper, "go run ./cmd/preflight") {
		t.Fatalf("scripts/runtime/preflight.sh does not run cmd/preflight; got:\n%s", wrapper)
	}
}

// TestGuardWiring_E2EUILaneUsesUIProfile locks spec 100 F-100-OPT-02: the
// e2e-ui case block gates on the LOWER `ui` preflight profile (not the 6000 MB
// heavy floor), via a DIRECT `smackerel_assert_host_resources_profile test ui`
// call — mirroring how the integration-light lane calls the _profile helper
// with `light` directly. This is the mechanical lock that keeps the no-ML
// browser lane on its honest, lowered floor.
func TestGuardWiring_E2EUILaneUsesUIProfile(t *testing.T) {
	root := repoRoot(t)
	script := readFile(t, root, "smackerel.sh")

	block, err := caseBlockStr(script, "e2e-ui")
	if err != nil {
		t.Fatalf("e2e-ui case block: %v", err)
	}
	if !strings.Contains(block, "smackerel_assert_host_resources_profile test ui") {
		t.Fatalf("e2e-ui lane must gate on the ui preflight profile via `smackerel_assert_host_resources_profile test ui` (spec 100 F-100-OPT-02); block was:\n%s", block)
	}
	// It MUST NOT still call the heavy back-compat wrapper form
	// `smackerel_assert_host_resources test` (which re-imposes the 6000 MB heavy
	// floor). The wrapper name is a PREFIX of the _profile helper, so this exact
	// literal only matches the heavy-wrapper invocation, never the _profile one
	// (which reads `..._profile test ui`).
	if strings.Contains(block, "smackerel_assert_host_resources test") {
		t.Fatalf("e2e-ui lane still calls the heavy wrapper `smackerel_assert_host_resources test` — the ui floor is not applied; block was:\n%s", block)
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

	// Spec 100 F-100-OPT-02 — the ui profile keys are a THIRD independently
	// fail-loud SST pair (the no-ML e2e-ui lane). Present in the yaml, read via
	// required_value, and emitted into the generated env file.
	for _, want := range []string{"min_available_ram_mb_ui:", "min_available_disk_gb_ui:"} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("config/smackerel.yaml missing %q (SST source for the ui preflight floor)", want)
		}
	}
	for _, want := range []string{
		"required_value runtime.preflight.min_available_ram_mb_ui",
		"required_value runtime.preflight.min_available_disk_gb_ui",
	} {
		if !strings.Contains(configSh, want) {
			t.Fatalf("config.sh does not read %q via required_value (fail-loud SST)", want)
		}
	}
	for _, want := range []string{
		"PREFLIGHT_MIN_AVAILABLE_RAM_MB_UI=${PREFLIGHT_MIN_AVAILABLE_RAM_MB_UI}",
		"PREFLIGHT_MIN_AVAILABLE_DISK_GB_UI=${PREFLIGHT_MIN_AVAILABLE_DISK_GB_UI}",
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

		// Spec 100 F-100-OPT-02 — the ui profile keys are carried too, and are
		// read by the SAME production path cmd/preflight uses for --profile ui
		// (LoadEnvFile + ParseThresholdsForProfile(ProfileUI)). This is the
		// no-stack proof that a dry `cmd/preflight --profile ui` reads the real
		// generated floor rather than a default.
		uiTh, err := ParseThresholdsForProfile(m, ProfileUI)
		if err != nil {
			t.Fatalf("%s does not carry valid ui preflight thresholds (cmd/preflight --profile ui would fail loud): %v", rel, err)
		}
		if uiTh.MinAvailableRAMMB <= 0 || uiTh.MinAvailableDiskGB <= 0 {
			t.Fatalf("%s ui thresholds must be positive, got %+v", rel, uiTh)
		}
	}
}
