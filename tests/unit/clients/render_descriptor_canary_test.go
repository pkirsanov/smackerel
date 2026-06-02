// Cross-language renderer canary for spec 073 TP-073-03.
//
// The canary asserts that the JavaScript renderer
// (`web/pwa/lib/render_descriptor_v1_cli.js`) and the Dart renderer
// (`clients/mobile/assistant/tool/render_descriptor_v1_cli.dart`) each
// project every spec 069 `assistant_turn_v1` response fixture under
// `tests/fixtures/assistant_response_v1/` into a render descriptor that:
//
//  1. conforms to `render-descriptor-v1.json` (shape-checked via
//     `validateDescriptor`), and
//  2. is deep-equal to the paired `<name>.descriptor.json` golden.
//
// The Dart adapter projections for iOS and Android are produced by the
// same shared renderer core (`clients/mobile/assistant/lib/core/
// render_descriptor_v1.dart`); they are exercised here through the
// shared CLI because the platform adapters are thin and do not mutate
// the descriptor output.
//
// This test is the gap-fill implementation of TP-073-03 (spec 073
// SCOPE-1). It runs under `./smackerel.sh test unit` because it is a
// pure projection canary — no live server is required. It does require
// `node` and `dart` on PATH; the test fails loud if either is missing so
// the canary cannot silently degrade.

package clients_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Package-level state populated by TestMain. The Dart CLI is pre-compiled to an
// AOT executable once per `go test` binary invocation; per-fixture subtests then
// exec that binary directly. This eliminates the BUG-073-001 race: `dart run`
// loads the VM, resolves pub-cache, and touches the shared
// clients/mobile/assistant/.dart_tool/ kernel snapshot cache on every call,
// which flakes under parallel `go test ./...` CPU/IO pressure.
var (
	dartExePath    string
	dartCompileErr error
	dartTempDir    string
)

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("dart"); err == nil {
		if err := compileDartRendererCLI(); err != nil {
			dartCompileErr = err
		}
	}
	code := m.Run()
	if dartTempDir != "" {
		_ = os.RemoveAll(dartTempDir)
	}
	os.Exit(code)
}

func compileDartRendererCLI() error {
	repoRoot, err := findRepoRootFromCwd()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "smackerel-render-canary-*")
	if err != nil {
		return fmt.Errorf("mkdir tempdir for dart AOT exe: %w", err)
	}
	dartTempDir = tmp
	exePath := filepath.Join(tmp, "render_descriptor_v1_cli")
	pkgDir := filepath.Join(repoRoot, dartPkgRelPath)
	cmd := exec.Command("dart", "compile", "exe", dartCliRelPath, "-o", exePath)
	cmd.Dir = pkgDir
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dart compile exe failed: %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("dart compile reported success but %s is missing: %w", exePath, err)
	}
	dartExePath = exePath
	return nil
}

func findRepoRootFromCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat go.mod under %s: %w", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repo root (go.mod) walking up from %s", cwd)
		}
		dir = parent
	}
}

const (
	fixtureRelPath = "tests/fixtures/assistant_response_v1"
	jsCliRelPath   = "web/pwa/lib/render_descriptor_v1_cli.js"
	dartPkgRelPath = "clients/mobile/assistant"
	dartCliRelPath = "tool/render_descriptor_v1_cli.dart"
)

func TestRenderDescriptorV1_CrossLanguageCanary(t *testing.T) {
	repoRoot := findRepoRoot(t)

	if _, err := exec.LookPath("node"); err != nil {
		t.Fatalf("node not on PATH; the spec 073 cross-language renderer canary requires both node and dart: %v", err)
	}
	if _, err := exec.LookPath("dart"); err != nil {
		t.Fatalf("dart not on PATH; the spec 073 cross-language renderer canary requires both node and dart: %v", err)
	}
	if dartCompileErr != nil {
		t.Fatalf("dart AOT pre-compile failed in TestMain (BUG-073-001 fix requires it): %v", dartCompileErr)
	}
	if dartExePath == "" {
		t.Fatalf("dartExePath is empty after TestMain; renderer canary would fall back to per-fixture `dart run` which races under parallel `go test ./...` load (BUG-073-001)")
	}

	fixtureDir := filepath.Join(repoRoot, fixtureRelPath)
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir %s: %v", fixtureDir, err)
	}

	var fixtureNames []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".input.json") {
			continue
		}
		base := strings.TrimSuffix(name, ".input.json")
		fixtureNames = append(fixtureNames, base)
	}

	required := []string{
		"text_only",
		"with_sources",
		"disambiguation",
		"confirm_accept_decline",
		"capture_acknowledgement",
		"error_retry",
		"unknown_shape",
	}
	have := map[string]bool{}
	for _, n := range fixtureNames {
		have[n] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Fatalf("missing required fixture %q under %s (spec 073 design.md requires all 7 named fixtures)", r, fixtureRelPath)
		}
	}

	descriptorSchema := loadDescriptorSchemaSentinel(t, repoRoot)

	for _, name := range fixtureNames {
		name := name
		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join(fixtureDir, name+".input.json")
			goldenPath := filepath.Join(fixtureDir, name+".descriptor.json")

			inputBytes, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("read input %s: %v", inputPath, err)
			}
			goldenBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v", goldenPath, err)
			}

			jsOut := runRenderer(t, repoRoot, "node",
				[]string{filepath.Join(repoRoot, jsCliRelPath)},
				inputBytes, "")
			// Use the pre-compiled AOT executable produced by TestMain; do NOT
			// fall back to `dart run` here (would reintroduce BUG-073-001).
			dartOut := runRenderer(t, repoRoot, dartExePath, nil, inputBytes, "")

			var golden, js, dart any
			if err := json.Unmarshal(goldenBytes, &golden); err != nil {
				t.Fatalf("golden %s is not valid JSON: %v", goldenPath, err)
			}
			if err := json.Unmarshal(jsOut, &js); err != nil {
				t.Fatalf("js renderer output is not valid JSON: %v\nstdout:\n%s", err, string(jsOut))
			}
			if err := json.Unmarshal(dartOut, &dart); err != nil {
				t.Fatalf("dart renderer output is not valid JSON: %v\nstdout:\n%s", err, string(dartOut))
			}

			validateDescriptorAgainstSentinel(t, "golden", golden, descriptorSchema)
			validateDescriptorAgainstSentinel(t, "js", js, descriptorSchema)
			validateDescriptorAgainstSentinel(t, "dart", dart, descriptorSchema)

			if !reflect.DeepEqual(js, golden) {
				t.Fatalf("js renderer output deviates from golden\n--- js ---\n%s\n--- golden ---\n%s", string(jsOut), string(goldenBytes))
			}
			if !reflect.DeepEqual(dart, golden) {
				t.Fatalf("dart renderer output deviates from golden\n--- dart ---\n%s\n--- golden ---\n%s", string(dartOut), string(goldenBytes))
			}
			if !reflect.DeepEqual(js, dart) {
				t.Fatalf("js and dart renderer outputs disagree for fixture %q\n--- js ---\n%s\n--- dart ---\n%s", name, string(jsOut), string(dartOut))
			}
		})
	}
}

// runRenderer invokes a renderer CLI with the given stdin, returning
// stdout bytes. Fails the test on non-zero exit or stderr output (CLIs
// MUST be silent on success so JSON parsing in the test cannot be
// poisoned).
func runRenderer(t *testing.T, repoRoot, bin string, args []string, stdin []byte, workdir string) []byte {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if workdir != "" {
		cmd.Dir = workdir
	} else {
		cmd.Dir = repoRoot
	}
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s failed: %v\nstderr:\n%s\nstdout:\n%s",
			bin, strings.Join(args, " "), err, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("%s %s wrote to stderr (CLI must be silent on success):\n%s",
			bin, strings.Join(args, " "), stderr.String())
	}
	return stdout.Bytes()
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("stat go.mod under %s: %v", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) walking up from %s", cwd)
		}
		dir = parent
	}
}

// descriptorSchemaSentinel is a structurally validated stand-in for the
// full JSON-Schema; the Go canary asserts every closed-vocabulary value
// and required key without pulling in a third-party JSON-Schema validator.
type descriptorSchemaSentinel struct {
	actionKinds map[string]struct{}
	kindOK      map[string]struct{}
}

func loadDescriptorSchemaSentinel(t *testing.T, repoRoot string) descriptorSchemaSentinel {
	t.Helper()
	// Sanity-check that the schema file exists; we do not parse it because
	// the canary enforces the same vocabulary inline. If the schema file is
	// renamed, the canary will fail loud.
	schemaPath := filepath.Join(repoRoot, fixtureRelPath, "render-descriptor-v1.json")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("render-descriptor-v1.json missing at %s: %v", schemaPath, err)
	}
	return descriptorSchemaSentinel{
		actionKinds: map[string]struct{}{
			"disambiguation_choice": {},
			"confirm_accept":        {},
			"confirm_decline":       {},
			"reset":                 {},
			"retry":                 {},
			"open_source":           {},
		},
		kindOK: map[string]struct{}{
			"text":     {},
			"quote":    {},
			"action":   {},
			"citation": {},
		},
	}
}

func validateDescriptorAgainstSentinel(t *testing.T, label string, descriptor any, schema descriptorSchemaSentinel) {
	t.Helper()
	root, ok := descriptor.(map[string]any)
	if !ok {
		t.Fatalf("%s descriptor is not a JSON object", label)
	}
	if sv, _ := root["schema_version"].(string); sv != "render-descriptor.v1" {
		t.Fatalf("%s descriptor.schema_version = %q, want %q", label, sv, "render-descriptor.v1")
	}
	nodesRaw, ok := root["nodes"].([]any)
	if !ok {
		t.Fatalf("%s descriptor.nodes is not an array", label)
	}
	for i, n := range nodesRaw {
		node, ok := n.(map[string]any)
		if !ok {
			t.Fatalf("%s descriptor.nodes[%d] is not an object", label, i)
		}
		kind, _ := node["kind"].(string)
		if _, ok := schema.kindOK[kind]; !ok {
			t.Fatalf("%s descriptor.nodes[%d].kind = %q outside closed vocabulary", label, i, kind)
		}
		switch kind {
		case "text", "quote":
			if _, ok := node["text"].(string); !ok {
				t.Fatalf("%s descriptor.nodes[%d] (%s) missing string text", label, i, kind)
			}
		case "action":
			ak, _ := node["action_kind"].(string)
			if _, ok := schema.actionKinds[ak]; !ok {
				t.Fatalf("%s descriptor.nodes[%d].action_kind = %q outside closed vocabulary", label, i, ak)
			}
			if _, ok := node["ref"].(string); !ok {
				t.Fatalf("%s descriptor.nodes[%d] action missing string ref", label, i)
			}
			if _, ok := node["label"].(string); !ok {
				t.Fatalf("%s descriptor.nodes[%d] action missing string label", label, i)
			}
		case "citation":
			if _, ok := node["source_id"].(string); !ok {
				t.Fatalf("%s descriptor.nodes[%d] citation missing string source_id", label, i)
			}
			if _, ok := node["label"].(string); !ok {
				t.Fatalf("%s descriptor.nodes[%d] citation missing string label", label, i)
			}
		}
	}
}

// TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun is the adversarial
// regression guard for BUG-073-001. It asserts that the Dart CLI was
// AOT-compiled to a native executable in TestMain. If a future change reverts
// the pre-compile (drops TestMain or restores per-fixture `dart run`),
// dartExePath stays empty and this test FAILS — independent of whether the
// underlying flake reproduces on the current machine.
func TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun(t *testing.T) {
	if _, err := exec.LookPath("dart"); err != nil {
		t.Fatalf("dart not on PATH; the spec 073 cross-language renderer canary requires dart: %v", err)
	}
	if dartCompileErr != nil {
		t.Fatalf("dart AOT pre-compile failed in TestMain (BUG-073-001 fix requires it): %v", dartCompileErr)
	}
	if dartExePath == "" {
		t.Fatalf("dartExePath is empty; renderer canary would fall back to `dart run` which races under parallel `go test ./...` load (BUG-073-001)")
	}
	info, err := os.Stat(dartExePath)
	if err != nil {
		t.Fatalf("stat dartExePath %s: %v", dartExePath, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("dartExePath %s is not executable (mode=%v); BUG-073-001 fix requires a native AOT binary", dartExePath, info.Mode())
	}
}
