// Package docfreshness — contract test that keeps docs/Development.md in sync
// with the three drift-prone, enumerable-from-disk inventories that spec 032
// (Documentation Freshness) is responsible for:
//
//  1. Go packages under internal/ (the Go Packages table)
//  2. Database migrations under internal/db/migrations/ (the Database Migrations table)
//  3. Prompt contracts under config/prompt_contracts/ (the Prompt Contracts table)
//
// Backstory (spec 032 / BUG-003): spec 032's acceptance criteria require
// docs/Development.md to list every Go package under internal/, every migration
// on disk, and every prompt contract on disk. The design.md Risk #1 mitigation
// ("CI freshness check comparing documented packages to `go list ./...`") was
// left optional/deferred. Predictably the inventories drifted: a DevOps sweep on
// 2026-06-06 found internal/scopesdriftguard/ (added 2026-06-05) absent from the
// Go Packages table — 33 package directories on disk vs 32 documented. There was
// no automated guard, so the drift was invisible until a manual probe.
//
// This test is that guard. It runs under `./smackerel.sh test unit --go` and CI
// with no new CLI/CI surface. Detection is mechanical: every on-disk inventory
// item MUST appear in docs/Development.md, or the test fails loud with the exact
// missing item(s). When a new package/migration/contract is added, the author
// adds the corresponding documentation row in the same change or this test fails.
//
// Package-detection semantics match the spec 032 freshness probe
// (`find internal -mindepth 1 -maxdepth 1 -type d`): every top-level directory
// under internal/ that contains at least one Go source file anywhere in its tree
// is a package that MUST be documented. This intentionally includes test-only
// guard packages (internal/scopesdriftguard/, internal/docfreshness/) and
// container packages whose Go files live only in subdirectories
// (internal/whatsapp/).
package docfreshness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// docFreshnessRepoRoot resolves the repo root from this test file's on-disk
// location, so the test is independent of the working directory.
func docFreshnessRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	// internal/docfreshness/ -> repo root is 2 parents up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func readDevelopmentDoc(t *testing.T, repoRoot string) string {
	t.Helper()
	p := filepath.Join(repoRoot, "docs", "Development.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read docs/Development.md: %v", err)
	}
	return string(b)
}

// scanInternalPackages returns every top-level directory under internal/ that
// contains at least one Go source file anywhere in its tree. Matches the spec
// 032 freshness probe (`find internal -mindepth 1 -maxdepth 1 -type d`)
// restricted to real Go package trees.
func scanInternalPackages(repoRoot string) ([]string, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, fmt.Errorf("read internal dir: %w", err)
	}
	var pkgs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		hasGo, err := dirContainsGoFile(filepath.Join(internalDir, e.Name()))
		if err != nil {
			return nil, err
		}
		if hasGo {
			pkgs = append(pkgs, e.Name())
		}
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

func dirContainsGoFile(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("walk %s: %w", dir, err)
	}
	return found, nil
}

// listFilesWithSuffix returns the names (basename only) of files directly inside
// dir whose name ends with suffix, sorted.
func listFilesWithSuffix(dir, suffix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// undocumented returns the subset of items whose needle(item) string does not
// appear anywhere in doc.
func undocumented(doc string, items []string, needle func(string) string) []string {
	var missing []string
	for _, it := range items {
		if !strings.Contains(doc, needle(it)) {
			missing = append(missing, it)
		}
	}
	return missing
}

func packageNeedle(pkg string) string { return "internal/" + pkg + "/" }

func fileNeedle(name string) string { return name }

// TestDocFreshness_AllInternalPackagesDocumented asserts every internal/ Go
// package is referenced in docs/Development.md (spec 032 acceptance criterion:
// "docs/Development.md lists all Go packages under internal/").
func TestDocFreshness_AllInternalPackagesDocumented(t *testing.T) {
	repoRoot := docFreshnessRepoRoot(t)
	doc := readDevelopmentDoc(t, repoRoot)
	pkgs, err := scanInternalPackages(repoRoot)
	if err != nil {
		t.Fatalf("scan internal packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("discovered 0 internal/ packages — probe is broken")
	}
	missing := undocumented(doc, pkgs, packageNeedle)
	t.Logf("internal/ package freshness: %d packages on disk, %d undocumented", len(pkgs), len(missing))
	if len(missing) > 0 {
		t.Fatalf("docs/Development.md is STALE: %d internal/ package(s) exist on disk but are undocumented: %s\n\nspec 032 requires the Go Packages table to list every internal/ package. Add a `| `internal/<name>/` | <purpose> |` row for each.", len(missing), strings.Join(missing, ", "))
	}
}

// TestDocFreshness_AllMigrationsDocumented asserts every migration file on disk
// is referenced in docs/Development.md (spec 032 acceptance criterion:
// "docs/Development.md lists every migration on disk with purpose").
func TestDocFreshness_AllMigrationsDocumented(t *testing.T) {
	repoRoot := docFreshnessRepoRoot(t)
	doc := readDevelopmentDoc(t, repoRoot)
	dir := filepath.Join(repoRoot, "internal", "db", "migrations")
	files, err := listFilesWithSuffix(dir, ".sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("discovered 0 migration files — probe is broken")
	}
	missing := undocumented(doc, files, fileNeedle)
	t.Logf("migration freshness: %d migration files on disk, %d undocumented", len(files), len(missing))
	if len(missing) > 0 {
		t.Fatalf("docs/Development.md is STALE: %d migration file(s) on disk are undocumented: %s\n\nspec 032 requires the Database Migrations table to list every migration on disk.", len(missing), strings.Join(missing, ", "))
	}
}

// TestDocFreshness_AllPromptContractsDocumented asserts every prompt contract on
// disk is referenced in docs/Development.md (spec 032 acceptance criterion:
// "docs/Development.md lists every prompt contract on disk with purpose").
func TestDocFreshness_AllPromptContractsDocumented(t *testing.T) {
	repoRoot := docFreshnessRepoRoot(t)
	doc := readDevelopmentDoc(t, repoRoot)
	dir := filepath.Join(repoRoot, "config", "prompt_contracts")
	files, err := listFilesWithSuffix(dir, ".yaml")
	if err != nil {
		t.Fatalf("list prompt contracts: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("discovered 0 prompt contracts — probe is broken")
	}
	missing := undocumented(doc, files, fileNeedle)
	t.Logf("prompt-contract freshness: %d contracts on disk, %d undocumented", len(files), len(missing))
	if len(missing) > 0 {
		t.Fatalf("docs/Development.md is STALE: %d prompt contract(s) on disk are undocumented: %s\n\nspec 032 requires the Prompt Contracts table to list every contract on disk.", len(missing), strings.Join(missing, ", "))
	}
}

// TestDocFreshness_AdversarialUndocumentedItemsDetected proves the guard is not
// tautological: against a synthetic repo whose Development.md documents nothing,
// each inventory scan MUST report exactly its synthetic missing item, and the
// same scan MUST report zero missing once the item is documented. Without this,
// a future refactor could silently neuter the freshness checks above.
func TestDocFreshness_AdversarialUndocumentedItemsDetected(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "internal", "ghostpkg", "x.go"), "package ghostpkg\n")
	writeFile(t, filepath.Join(tmp, "internal", "db", "migrations", "099_ghost.sql"), "-- ghost migration\n")
	writeFile(t, filepath.Join(tmp, "config", "prompt_contracts", "ghost-extraction-v1.yaml"), "version: ghost-extraction-v1\n")
	writeFile(t, filepath.Join(tmp, "docs", "Development.md"), "# Development\n\nThis synthetic doc intentionally documents none of the inventories.\n")
	emptyDoc := readFileString(t, filepath.Join(tmp, "docs", "Development.md"))

	// Packages: ghostpkg is discovered and reported missing; internal/db (only
	// a .sql, no .go) is correctly NOT treated as a package.
	pkgs, err := scanInternalPackages(tmp)
	if err != nil {
		t.Fatalf("scan synthetic packages: %v", err)
	}
	if got := undocumented(emptyDoc, pkgs, packageNeedle); len(got) != 1 || got[0] != "ghostpkg" {
		t.Fatalf("ADVERSARIAL FAILURE (packages): expected [ghostpkg] reported missing, got %v (discovered pkgs=%v)", got, pkgs)
	}

	// Migrations.
	migs, err := listFilesWithSuffix(filepath.Join(tmp, "internal", "db", "migrations"), ".sql")
	if err != nil {
		t.Fatalf("list synthetic migrations: %v", err)
	}
	if got := undocumented(emptyDoc, migs, fileNeedle); len(got) != 1 || got[0] != "099_ghost.sql" {
		t.Fatalf("ADVERSARIAL FAILURE (migrations): expected [099_ghost.sql] reported missing, got %v", got)
	}

	// Prompt contracts.
	contracts, err := listFilesWithSuffix(filepath.Join(tmp, "config", "prompt_contracts"), ".yaml")
	if err != nil {
		t.Fatalf("list synthetic contracts: %v", err)
	}
	if got := undocumented(emptyDoc, contracts, fileNeedle); len(got) != 1 || got[0] != "ghost-extraction-v1.yaml" {
		t.Fatalf("ADVERSARIAL FAILURE (contracts): expected [ghost-extraction-v1.yaml] reported missing, got %v", got)
	}

	// Green path: a doc that documents all three reports zero missing. Proves
	// the checks pass for real once the inventory is documented (not always-fail).
	fullDoc := "internal/ghostpkg/ 099_ghost.sql ghost-extraction-v1.yaml"
	if got := undocumented(fullDoc, pkgs, packageNeedle); len(got) != 0 {
		t.Fatalf("ADVERSARIAL FAILURE (package green path): documented package still reported missing: %v", got)
	}
	if got := undocumented(fullDoc, migs, fileNeedle); len(got) != 0 {
		t.Fatalf("ADVERSARIAL FAILURE (migration green path): documented migration still reported missing: %v", got)
	}
	if got := undocumented(fullDoc, contracts, fileNeedle); len(got) != 0 {
		t.Fatalf("ADVERSARIAL FAILURE (contract green path): documented contract still reported missing: %v", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
