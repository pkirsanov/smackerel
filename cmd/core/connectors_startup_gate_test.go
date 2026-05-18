package main

// BUG-029-005: Static-file regression guard that prevents reintroduction
// of the redundant `<Connector>Enabled && cfg.<Path> != ""` guard
// pattern in cmd/core/connectors.go.
//
// Context: prior to BUG-029-005, three connector auto-start branches
// (bookmarks, browser-history, google-maps-timeline) gated startup on
// BOTH the boolean enable flag AND a non-empty path. This was a
// double-load-bearing-signal anti-pattern: the empty-string state of
// the path env var was overloaded to mean "connector disabled" at the
// startup gate, while the boolean enable flag was used elsewhere for
// the same intent. BUG-029-005 made the SST always emit a non-empty
// path (shell-env > yaml > repo-default fallback per the BUG-029-003
// DD-2 precedent), so the path-emptiness check on the startup gate
// became dead code that misled future readers about which signal is
// authoritative.
//
// The fix dropped the `&& cfg.<Path> != ""` clause from all three
// branches. The twitter connector branch already used the bare-boolean
// pattern. This guard locks the new contract: the boolean enable flag
// is the SOLE load-bearing signal for connector startup; the path is
// SST-guaranteed non-empty.
//
// References:
//   - cmd/core/connectors.go (the production surface this test lints)
//   - specs/029-devops-pipeline/bugs/BUG-029-005-connector-volume-mount-fail-loud-sweep/
//   - scripts/commands/config.sh (the SST that guarantees non-empty paths)

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// forbiddenStartupGatePatterns are the exact-string regex patterns that
// the live cmd/core/connectors.go file MUST NOT contain. Each pattern
// encodes the pre-fix redundant-guard form for one of the 4 connectors
// (bookmarks, browser-history, google-maps-timeline, twitter). The
// twitter pattern is included even though the pre-fix code already used
// the bare-boolean form, so the guard locks the contract for ALL FOUR
// connectors symmetrically and catches a future regression that
// introduces the anti-pattern on twitter.
var forbiddenStartupGatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`BookmarksEnabled\s*&&\s*cfg\.BookmarksImportDir\s*!=\s*""`),
	regexp.MustCompile(`BrowserHistoryEnabled\s*&&\s*cfg\.BrowserHistoryPath\s*!=\s*""`),
	regexp.MustCompile(`MapsEnabled\s*&&\s*cfg\.MapsImportDir\s*!=\s*""`),
	regexp.MustCompile(`TwitterEnabled\s*&&\s*cfg\.TwitterArchiveDir\s*!=\s*""`),
}

// connectorsGoPath resolves the absolute path to cmd/core/connectors.go
// independent of `go test` CWD. Using runtime.Caller(0) anchors the
// lookup at the test file's own location, so the test works both from
// `cd cmd/core && go test` and from `cd /workspace && go test ./...`.
func connectorsGoPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "connectors.go")
}

// findForbiddenStartupGatePatterns scans goSource line by line and
// returns a sorted list of "<lineNum>: <pattern>" entries for every
// occurrence of any forbidden pattern. Comment-only lines (first
// non-space character is `//`) are skipped because the existing
// BUG-029-005 commentary in connectors.go quotes the dropped clauses
// for context — those documentation references must not be flagged.
//
// The function is pure so the adversarial sub-case can feed a synthetic
// fixture and prove RED→GREEN behavior.
func findForbiddenStartupGatePatterns(goSource []byte, patterns []*regexp.Regexp) []string {
	var violations []string
	for lineNum, rawLine := range strings.Split(string(goSource), "\n") {
		trimmed := strings.TrimLeft(rawLine, " \t")
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, p := range patterns {
			if p.MatchString(rawLine) {
				violations = append(violations, fmt.Sprintf("line %d: forbidden pattern %s matched in: %s", lineNum+1, p.String(), strings.TrimSpace(rawLine)))
			}
		}
	}
	return violations
}

// TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal is the
// LIVE-FILE positive assertion that cmd/core/connectors.go does NOT
// re-introduce the redundant `<Connector>Enabled && cfg.<Path> != ""`
// guard pattern for any of the 4 connectors.
//
// The 4 boolean enable flags (BookmarksEnabled / BrowserHistoryEnabled
// / MapsEnabled / TwitterEnabled) are now the SOLE load-bearing signal
// for connector startup; the path env vars are SST-guaranteed non-empty
// via the repo-default fallback in scripts/commands/config.sh.
func TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal(t *testing.T) {
	srcPath := connectorsGoPath(t)
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read live source file %q: %v", srcPath, err)
	}
	violations := findForbiddenStartupGatePatterns(src, forbiddenStartupGatePatterns)
	if len(violations) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "cmd/core/connectors.go violates BUG-029-005 startup-gate contract — the boolean enable flag MUST be the SOLE load-bearing signal for connector startup; the path is SST-guaranteed non-empty.\n")
		for _, v := range violations {
			fmt.Fprintf(&b, "  - %s\n", v)
		}
		fmt.Fprintf(&b, "\nFix: drop the redundant `&& cfg.<Path> != \"\"` clause from the auto-start `if` statement. See specs/029-devops-pipeline/bugs/BUG-029-005-connector-volume-mount-fail-loud-sweep/ for rationale.\n")
		t.Fatal(b.String())
	}
	t.Logf("contract OK: cmd/core/connectors.go has zero `<Connector>Enabled && cfg.<Path> != \"\"` redundant guards across all 4 connectors (bookmarks / browser-history / google-maps-timeline / twitter)")
}

// TestConnectorStartupGate_AdversarialReintroduction proves the helper
// REJECTS a synthetic fixture that re-introduces one of the forbidden
// patterns. Without this adversarial half, the live-file assertion
// would be tautological — it would PASS for any reason (including a
// broken regex that never matches anything).
func TestConnectorStartupGate_AdversarialReintroduction(t *testing.T) {
	const fixture = `package main

func registerConnectors() {
	if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {
		// reintroduced redundant guard — must be detected
	}
	if cfg.TwitterEnabled {
		// correct bare-boolean form — must NOT be flagged
	}
}
`
	violations := findForbiddenStartupGatePatterns([]byte(fixture), forbiddenStartupGatePatterns)
	if len(violations) == 0 {
		t.Fatal("adversarial contract test failed: helper accepted the regression fixture; the reintroduced `cfg.BookmarksEnabled && cfg.BookmarksImportDir != \"\"` form should have been rejected (the contract is tautological — it would NOT catch a regression to the BUG-029-005 anti-pattern)")
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "BookmarksImportDir") {
		t.Fatalf("adversarial contract test failed: violation list did NOT mention BookmarksImportDir; got:\n%s", joined)
	}
	if strings.Contains(joined, "TwitterEnabled") {
		t.Fatalf("adversarial contract test failed: bare-boolean `cfg.TwitterEnabled` form appeared in violation list; the regex matched the wrong pattern; got:\n%s", joined)
	}
	t.Logf("adversarial OK: reintroduced redundant guard rejected; bare-boolean form ignored; violations:\n%s", joined)
}
