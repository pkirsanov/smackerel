// BUG-069-005 SCN-BUG069005-006 — required assistant E2E tests must never skip.
//
// The bug: five REQUIRED assistant E2E tests reported package exit 0 while
// 0 of 5 required behaviors passed and all 5 SKIPPED. A false green. The
// sibling guard at tests/e2e/agent/no_skip_guard_test.go already forbids
// skipping, but it resolves its scan directory from runtime.Caller(0) and
// reads that ONE directory non-recursively — so it covers tests/e2e/agent/
// and nothing else. tests/e2e/assistant/ had no skip guard at all. That is
// the hole this bug fell through.
//
// The required list below is transcribed from the packet's scenario
// manifest, which is the source of truth for "required":
//
//	specs/069-assistant-http-transport/bugs/
//	  BUG-069-005-required-e2e-false-green/scenario-manifest.json
//
// (see SCN-BUG069005-001..005 `linkedTests`, and SCN-BUG069005-006 which
// enumerates all five together).
//
// # Why a denylist, not a directory-wide assertion
//
// tests/e2e/assistant/ contains ~30 legitimate t.Skip/t.Skipf calls across
// 21 files: live-stack-unavailable guards, environment-branch guards, and
// status=planned scenario placeholders. In THIS directory skipping is the
// norm and not-skipping is the exception, so a directory-wide no-skip
// assertion would be wrong and would fail immediately. This guard is
// therefore the inverse of the agent guard's permitted-skip allowlist: an
// explicit REQUIRED-test denylist. Everything not named here keeps its
// freedom to skip.
//
// # Why the assertion is scoped to the function body
//
// A required test must not skip even when an unrelated helper in the same
// file legitimately may. Whole-file matching would couple the required
// test's verdict to its neighbours. Body boundaries come from go/parser
// rather than brace counting because these files embed JSON string
// literals containing braces, which a counter would mis-balance. Detection
// inside those boundaries then uses the SAME regex family as the sibling
// guard so both guards agree on what counts as a skip; running the regex
// over the raw body text (comments and strings included) is deliberately
// fail-closed, which is the correct direction of error for a guard whose
// whole purpose is to refuse a silent pass.
//
// # Why there is no build tag
//
// Every one of the 47 assistant e2e files carries `//go:build e2e`. This
// guard deliberately omits the tag — exactly as the sibling agent guard
// does — so it runs under a plain `go test ./...` even when the gated e2e
// suite is switched off. A guard that only runs when the suite runs cannot
// catch the suite silently not running, which is the failure mode of this
// very bug.
//
// # Gates
//
//	G1: each named required test MUST exist in its named file. A guard that
//	    silently passes when the test it protects was deleted or renamed
//	    reproduces this bug in a new form, so a missing test is FATAL.
//	G2: the body of each named required test MUST contain no t.Skip-family
//	    call. Violations name file, function and line.
//	G3: the adversarial self-test proves the detector actually fires — a
//	    broken extractor or regex would otherwise turn G1/G2 into a no-op.
//	    Its fixtures are in-memory source strings, never files in a scanned
//	    directory, so they cannot pollute the production assertion.
package assistant_e2e

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// requiredNoSkipTest names one manifest-required test that must fail
// loudly rather than skip.
type requiredNoSkipTest struct {
	File       string
	Func       string
	ScenarioID string
}

// requiredNoSkipTests is the denylist, transcribed from
// scenario-manifest.json. Adding a required scenario to the manifest
// without adding it here leaves that scenario unguarded — keep the two in
// step.
var requiredNoSkipTests = []requiredNoSkipTest{
	{
		File:       "annotation_intent_test.go",
		Func:       "TestAnnotationIntentE2E_SlotsComeFromCompiledIntent",
		ScenarioID: "SCN-BUG069005-001",
	},
	{
		File:       "intent_clarify_test.go",
		Func:       "TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation",
		ScenarioID: "SCN-BUG069005-002",
	},
	{
		File:       "http_disambiguation_test.go",
		Func:       "TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn",
		ScenarioID: "SCN-BUG069005-003",
	},
	{
		File:       "intent_side_effect_test.go",
		Func:       "TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence",
		ScenarioID: "SCN-BUG069005-004",
	},
	{
		File:       "http_confirm_test.go",
		Func:       "TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce",
		ScenarioID: "SCN-BUG069005-005",
	},
}

// requiredNoSkipPattern matches the t.Skip family as a method call on a
// *testing.T receiver. Same expression as the sibling agent guard; the \b
// boundary prevents false matches on identifiers like runner.SkipFooBar.
var requiredNoSkipPattern = regexp.MustCompile(`\bt\.(Skip|SkipNow|Skipf)\(`)

// errRequiredNoSkipTestMissing reports that a named required test is not
// present in its file — deleted, renamed, or moved.
var errRequiredNoSkipTestMissing = errors.New("required test function not found")

// assistantE2ERequiredGuardDir locates tests/e2e/assistant/ relative to
// this file.
func assistantE2ERequiredGuardDir(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate this test file")
	}
	return filepath.Dir(here)
}

// scanRequiredTestBodyForSkips returns the 1-based line numbers of every
// t.Skip-family call inside funcName's body. It returns
// errRequiredNoSkipTestMissing when funcName is absent from src.
func scanRequiredTestBodyForSkips(filename string, src []byte, funcName string) ([]int, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != funcName {
			continue
		}
		if fn.Body == nil {
			return nil, fmt.Errorf("%w: %s is declared without a body", errRequiredNoSkipTestMissing, funcName)
		}

		start := fset.Position(fn.Body.Lbrace).Offset
		end := fset.Position(fn.Body.Rbrace).Offset
		if start < 0 || end < start || end >= len(src) {
			return nil, fmt.Errorf("body offsets [%d,%d] out of range for %s in %s", start, end, funcName, filename)
		}

		var lines []int
		for _, m := range requiredNoSkipPattern.FindAllIndex(src[start:end+1], -1) {
			abs := start + m[0]
			lines = append(lines, 1+strings.Count(string(src[:abs]), "\n"))
		}
		return lines, nil
	}

	return nil, errRequiredNoSkipTestMissing
}

// TestRequiredAssistantE2ETestsNeverSkip is the production guard (G1+G2).
func TestRequiredAssistantE2ETestsNeverSkip(t *testing.T) {
	dir := assistantE2ERequiredGuardDir(t)

	if len(requiredNoSkipTests) == 0 {
		t.Fatal("required-test denylist is empty — the guard would be a no-op; restore it from scenario-manifest.json")
	}

	var violations []string
	for _, rt := range requiredNoSkipTests {
		path := filepath.Join(dir, rt.File)
		src, err := os.ReadFile(path)
		if err != nil {
			violations = append(violations, fmt.Sprintf(
				"%s — required file unreadable (%v); %s has no enforcement",
				rt.File, err, rt.ScenarioID))
			continue
		}

		lines, err := scanRequiredTestBodyForSkips(rt.File, src, rt.Func)
		if errors.Is(err, errRequiredNoSkipTestMissing) {
			violations = append(violations, fmt.Sprintf(
				"%s — required test %s is MISSING (deleted, renamed or moved); %s is now unenforced",
				rt.File, rt.Func, rt.ScenarioID))
			continue
		}
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s — %v", rt.File, err))
			continue
		}

		for _, line := range lines {
			violations = append(violations, fmt.Sprintf(
				"%s:%d — t.Skip-family call inside required test %s (%s)",
				rt.File, line, rt.Func, rt.ScenarioID))
		}
	}

	if len(violations) > 0 {
		t.Fatalf("BUG-069-005 SCN-BUG069005-006 violation: %d problem(s) with the manifest-required assistant E2E tests:\n  %s\n\n"+
			"A required test must FAIL loudly when its behavior is absent, never skip, and must never vanish.\n"+
			"To fix: either\n"+
			"  (a) make the test FAIL fast on the missing dependency (preferred), or\n"+
			"  (b) if the requirement genuinely changed, update scenario-manifest.json FIRST and then this denylist.\n"+
			"DO NOT silently bail out — a skipped required test is the false green this guard exists to prevent.",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestRequiredNoSkipGuard_AdversarialFinding proves the detector fires
// (G3). Fixtures are in-memory source strings, so they are never scanned
// by the production assertion above.
func TestRequiredNoSkipGuard_AdversarialFinding(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		target      string
		wantSkips   int
		wantMissing bool
	}{
		{
			name:      "skip_inside_required_test_is_caught",
			body:      "package p\n\nimport \"testing\"\n\nfunc TestTarget(t *testing.T) {\n\tt.Skip(\"nope\")\n}\n",
			target:    "TestTarget",
			wantSkips: 1,
		},
		{
			name:      "skipnow_and_skipf_are_caught",
			body:      "package p\n\nimport \"testing\"\n\nfunc TestTarget(t *testing.T) {\n\tt.SkipNow()\n\tt.Skipf(\"nope %s\", \"x\")\n}\n",
			target:    "TestTarget",
			wantSkips: 2,
		},
		{
			name:      "clean_required_test_reports_nothing",
			body:      "package p\n\nimport \"testing\"\n\nfunc TestTarget(t *testing.T) {\n\tt.Fatal(\"loud\")\n}\n",
			target:    "TestTarget",
			wantSkips: 0,
		},
		{
			// Body scoping: a neighbour may skip; the required test's
			// verdict must not be coupled to it.
			name:      "skip_in_neighbouring_func_is_ignored",
			body:      "package p\n\nimport \"testing\"\n\nfunc helper(t *testing.T) {\n\tt.Skip(\"allowed here\")\n}\n\nfunc TestTarget(t *testing.T) {\n\tt.Fatal(\"loud\")\n}\n",
			target:    "TestTarget",
			wantSkips: 0,
		},
		{
			// Brace counting would mis-balance on this literal and walk
			// off the end of the body; go/parser does not.
			name:      "brace_in_string_literal_does_not_break_boundaries",
			body:      "package p\n\nimport \"testing\"\n\nfunc TestTarget(t *testing.T) {\n\tpayload := `{\"a\":{\"b\":1}}`\n\t_ = payload\n\tt.Skip(\"still caught\")\n}\n\nfunc after(t *testing.T) {\n\tt.Skip(\"outside\")\n}\n",
			target:    "TestTarget",
			wantSkips: 1,
		},
		{
			name:        "renamed_or_deleted_required_test_is_reported_missing",
			body:        "package p\n\nimport \"testing\"\n\nfunc TestSomethingElse(t *testing.T) {\n\tt.Fatal(\"loud\")\n}\n",
			target:      "TestTarget",
			wantMissing: true,
		},
		{
			name:      "method_named_skip_is_not_a_match",
			body:      "package p\n\nimport \"testing\"\n\nfunc TestTarget(t *testing.T) {\n\trunner := struct{ SkipFooBar func() }{SkipFooBar: func() {}}\n\trunner.SkipFooBar()\n}\n",
			target:    "TestTarget",
			wantSkips: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// scanRequiredTestBodyForSkips parses src and never opens the
			// path, so the fixture needs no file on disk.
			lines, err := scanRequiredTestBodyForSkips("fixture_test.go", []byte(c.body), c.target)

			if c.wantMissing {
				if !errors.Is(err, errRequiredNoSkipTestMissing) {
					t.Fatalf("want errRequiredNoSkipTestMissing for a renamed/deleted required test, got err=%v lines=%v — "+
						"the guard would silently pass while the test it protects is gone", err, lines)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(lines) != c.wantSkips {
				t.Fatalf("detector found %d skip(s) at lines %v, want %d — the guard would not catch the regression in production",
					len(lines), lines, c.wantSkips)
			}
		})
	}
}
