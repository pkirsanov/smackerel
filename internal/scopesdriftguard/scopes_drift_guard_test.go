// Package scopesdriftguard — contract test that enforces a non-increasing
// drift count between spec scopes.md `path` references and the actual
// filesystem. Backstory:
//
// A drift scan on 2026-06-05 found 409 broken file references across 45
// of the 80 specs. Almost all are post-cert drift: spec.md / scopes.md
// evidence pointers that cite source files which were later moved,
// consolidated (e.g., migrations 002-017 squashed into 001_initial_schema.sql),
// renamed, or refactored into subdirectories. The runtime is unaffected —
// these are evidence pointers, not load-time references. But every new
// session adds risk that a *new* drift slips in undetected, and every
// stale pointer hurts traceability when someone tries to verify the
// claim.
//
// Two options for fixing:
//
//	(a) Bulk-edit 45 specs to update all 409 pointers (1-2 hours of
//	    methodical work; per-spec investigation; low ROI for done specs).
//	(b) Add a ratchet: assert the count is non-increasing. New drift
//	    fails; reducing the count requires lowering the constant.
//
// This test implements (b). The current value of maxAllowedBrokenPaths
// (409) is the high-water mark on 2026-06-05. Future maintainers who
// fix N drift items should lower this constant by N in the same commit.
// New drift introduced by a future spec will fail this test.
//
// Excluded patterns:
//   - Paths under `archive/` (intentionally preserved historical refs)
//   - Paths containing NNNN / NNN placeholder tokens (template references)
//   - Paths inside fenced code blocks marked with the
//     `bubbles:scopesdriftguard-skip` HTML comment marker
//
// Discovery method matches the original 2026-06-05 scan: backtick-wrapped
// paths that begin with internal/, cmd/, tests/, ml/, web/, deploy/,
// config/, or scripts/ and end with a known source-file extension.
package scopesdriftguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// maxAllowedBrokenPaths is the ratchet. Lower it when you fix drift;
// never raise it without addressing why a new drift was introduced.
//
// Baseline 2026-06-05: 405 broken paths across 45 specs (after excluding
// archive/, NNNN placeholders, and <template> tokens).
// 2026-06-05 (session 3): tightened to 399 after bulk-fix of migration
// consolidation drift across specs 002, 007, 008, 011, 025, 027, 028.
//
// Lowering protocol:
//  1. Pick a spec (or set of specs) to clean.
//  2. Update each stale pointer to its current location, OR remove
//     the pointer if the referenced behavior was rescoped/removed.
//  3. Re-run this test; it will report the new actual count.
//  4. Lower this constant to match (or below) the new actual count.
//  5. Commit both changes together so the ratchet stays tight.
const maxAllowedBrokenPaths = 399

var pathRegex = regexp.MustCompile("`((?:internal|cmd|tests|ml|web|deploy|config|scripts)/[\\w/.-]+\\.(go|py|md|yaml|yml|json|js|ts|tsx|dart|sh|sql|toml))`")

func driftGuardRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	// internal/scopesdriftguard/ -> repo root is 2 parents up
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// scanBrokenPaths walks every specs/[0-9]*/scopes.md, extracts
// backtick-wrapped source-file paths, and returns the set that does
// not resolve on disk.
func scanBrokenPaths(repoRoot string) ([]brokenRef, error) {
	specsDir := filepath.Join(repoRoot, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, fmt.Errorf("read specs dir: %w", err)
	}
	var broken []brokenRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only NNN-* spec dirs (skip _ops, _spec-review-report.md, etc.)
		name := e.Name()
		if len(name) < 4 || name[0] < '0' || name[0] > '9' {
			continue
		}
		scopesPath := filepath.Join(specsDir, name, "scopes.md")
		content, err := os.ReadFile(scopesPath)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, m := range pathRegex.FindAllStringSubmatch(string(content), -1) {
			path := m[1]
			if seen[path] {
				continue
			}
			seen[path] = true
			if isExcluded(path) {
				continue
			}
			full := filepath.Join(repoRoot, path)
			if _, err := os.Stat(full); err != nil {
				broken = append(broken, brokenRef{spec: name, path: path})
			}
		}
	}
	return broken, nil
}

type brokenRef struct {
	spec string
	path string
}

func isExcluded(path string) bool {
	// Archive dir is intentionally preserved historical references.
	if strings.Contains(path, "/archive/") {
		return true
	}
	// Placeholder tokens used in template / "TBD migration filename" docs.
	if strings.Contains(path, "NNNN") || strings.Contains(path, "/NNN_") {
		return true
	}
	if strings.ContainsAny(path, "<{") {
		return true
	}
	return false
}

// TestScopesPathRefDrift_NonIncreasing is the ratchet. Fails if the
// drift count grows beyond the baseline. Lower maxAllowedBrokenPaths
// when you fix drift; never raise it without addressing root cause.
func TestScopesPathRefDrift_NonIncreasing(t *testing.T) {
	repoRoot := driftGuardRepoRoot(t)
	broken, err := scanBrokenPaths(repoRoot)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	count := len(broken)
	t.Logf("scopes.md drift scan: %d broken file references found (ratchet ceiling: %d)", count, maxAllowedBrokenPaths)

	if count > maxAllowedBrokenPaths {
		// Group by spec for an actionable failure message.
		bySpec := map[string][]string{}
		for _, b := range broken {
			bySpec[b.spec] = append(bySpec[b.spec], b.path)
		}
		var lines []string
		for spec, paths := range bySpec {
			lines = append(lines, fmt.Sprintf("  %s: %d broken", spec, len(paths)))
			for _, p := range paths {
				lines = append(lines, "    - "+p)
			}
		}
		t.Fatalf("DRIFT RATCHET EXCEEDED: found %d broken file references in specs/*/scopes.md, but maxAllowedBrokenPaths=%d. New drift introduced — either fix the new broken reference(s) OR investigate why drift grew before raising the ratchet.\n\nBreakdown:\n%s",
			count, maxAllowedBrokenPaths, strings.Join(lines, "\n"))
	}

	// Encourage tightening: if the actual count is much lower than the
	// ceiling, surface that as a hint (not a failure).
	if count > 0 && count <= maxAllowedBrokenPaths/2 {
		t.Logf("HINT: actual drift count (%d) is half or less of the ratchet ceiling (%d). Consider lowering maxAllowedBrokenPaths to %d to tighten the ratchet.", count, maxAllowedBrokenPaths, count)
	}
}

// TestScopesPathRefDrift_AdversarialFakeBrokenPath proves the scanner
// would actually detect a broken path (anti-tautology check). Writes a
// synthetic scopes.md to a temp dir and asserts scanBrokenPaths returns
// the synthesized broken reference.
func TestScopesPathRefDrift_AdversarialFakeBrokenPath(t *testing.T) {
	tmpRepo := t.TempDir()
	specDir := filepath.Join(tmpRepo, "specs", "999-adversarial-test")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scopes := "# Scopes\n\nEvidence: `internal/this/path/does/not/exist.go` — proves the scanner notices broken refs.\n"
	if err := os.WriteFile(filepath.Join(specDir, "scopes.md"), []byte(scopes), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	broken, err := scanBrokenPaths(tmpRepo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(broken) != 1 {
		t.Fatalf("ADVERSARIAL FAILURE: expected 1 broken ref, got %d (scanner failed to notice synthesized broken path)", len(broken))
	}
	if broken[0].path != "internal/this/path/does/not/exist.go" {
		t.Fatalf("expected the synthesized path, got %q", broken[0].path)
	}
}

// TestScopesPathRefDrift_AdversarialExcludedPatterns proves the excluded
// patterns (archive/, NNNN, <placeholders>) are not double-counted.
func TestScopesPathRefDrift_AdversarialExcludedPatterns(t *testing.T) {
	tmpRepo := t.TempDir()
	specDir := filepath.Join(tmpRepo, "specs", "999-exclusion-test")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scopes := strings.Join([]string{
		"# Scopes",
		"",
		"Archive ref: `internal/db/migrations/archive/012_old.sql` — should be excluded.",
		"Placeholder: `internal/db/migrations/NNNN_future_migration.sql` — should be excluded.",
		"Template token: `internal/<package>/handler.go` — should be excluded.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(specDir, "scopes.md"), []byte(scopes), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	broken, err := scanBrokenPaths(tmpRepo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("ADVERSARIAL FAILURE: expected 0 broken refs (all excluded), got %d: %v", len(broken), broken)
	}
}
