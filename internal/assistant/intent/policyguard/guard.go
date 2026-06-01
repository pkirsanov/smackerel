// Spec 068 SCOPE-4 — Raw-route bypass policy guard (SCN-068-A08).
//
// ReportRawRouteBypasses scans a Go source tree and reports every
// file that calls Router.Route (any receiver) without invoking the
// spec 068 intent.Compiler beforehand in the same file. The guard
// is scoped narrowly to the facade-ingress surface; the spec 067
// follow-on will lift this to the full user-facing surface.
//
// A Finding carries the file path and a fixed-format message naming
// "missing intent.Compiler step" so the spec 067 policy-guard output
// e2e (tests/e2e/policy/intent_policy_guard_output_test.go) can
// assert the wording verbatim.

package policyguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding is one bypass report.
type Finding struct {
	File    string
	Message string
}

// MissingCompilerStep is the canonical phrase the guard uses when it
// finds a Router.Route call site that does not invoke the spec 068
// intent.Compiler first. The phrase is stable so guard-output tests
// can match it verbatim.
const MissingCompilerStep = "missing intent.Compiler step before Router.Route"

// AllowedRouteCallers is the closed allowlist of files that may call
// Router.Route in the user-facing assistant ingress surface. Every
// other call site is reported. Entries are matched as suffix matches
// against the cleaned, slash-separated relative path of each file
// (relative to the scan root). Test files (`_test.go`) are always
// excluded from scanning and never need allowlisting.
var AllowedRouteCallers = []string{
	"facade.go",
}

// ScanSubdirs is the closed list of repository-relative subdirectories
// the guard scans. Scope 4 ships the FIRST version of the guard scoped
// to the facade-ingress surface only (`internal/assistant/`). The
// spec 067 follow-on will widen this to every user-facing surface.
// `internal/agent/` is intentionally excluded because `bridge.go`
// hosts the programmatic agent API consumed by scheduler/pipeline,
// not user-facing natural-language routing.
var ScanSubdirs = []string{
	"internal/assistant",
}

var (
	reRouterRoute = regexp.MustCompile(`\b\w+\.Route\s*\(`)
	reCompiler    = regexp.MustCompile(`intent\.Compiler|intentCompiler|IntentCompiler`)
)

// ReportRawRouteBypasses walks root and returns one Finding per file
// that calls Router.Route without an intent.Compiler reference. Files
// in AllowedRouteCallers and files ending in _test.go are skipped.
// vendor/ and .git/ directories are skipped. Callers are expected to
// pass the root of the subtree they want to scan (typically a path
// from ScanSubdirs joined with the repo root).
func ReportRawRouteBypasses(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, allowed := range AllowedRouteCallers {
			if strings.HasSuffix(rel, allowed) || rel == allowed {
				return nil
			}
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		if !reRouterRoute.MatchString(text) {
			return nil
		}
		if reCompiler.MatchString(text) {
			return nil
		}
		findings = append(findings, Finding{
			File:    rel,
			Message: fmt.Sprintf("%s: %s", rel, MissingCompilerStep),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}
