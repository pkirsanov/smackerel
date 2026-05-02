package consumers

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// downstreamPackages enumerates every Smackerel feature that the spec
// 038 Scope 8 contract identifies as a downstream consumer of drive
// artifacts. Each entry is the import-path-relative directory under the
// repo root (resolved via repoRoot()).
//
// IMPORTANT: this list is the authoritative cross-feature consumer
// surface. Adding a new downstream consumer of drive metadata REQUIRES
// adding the package here so the import-discipline guard runs against
// it. Removing a consumer from this list to "make the test pass" is
// considered fabrication and is forbidden.
var downstreamPackages = []string{
	"internal/recipe",
	"internal/intelligence",
	"internal/mealplan",
	"internal/list",
	"internal/annotation",
	"internal/digest",
	"internal/agent",
	"internal/domain",
	"internal/api",
	"internal/web",
	"internal/telegram",
}

// forbiddenImports lists drive provider-specific Go import paths that
// downstream consumers MUST NOT depend on. The single legitimate place
// these may be imported is the production wiring entrypoint (cmd/core)
// which assembles the registry. Tests that import provider-specific
// packages to build fixtures are also exempt from this contract — only
// non-test source files are scanned.
var forbiddenImports = []string{
	"github.com/smackerel/smackerel/internal/drive/google",
	"github.com/smackerel/smackerel/internal/drive/memprovider",
}

// allowedDownstreamDriveImports lists the provider-neutral drive
// sub-packages a downstream consumer MAY import. Any other internal
// drive sub-package MUST go through this consumers package or through
// a documented provider-neutral surface (e.g. drive.Provider, the
// retrieve service, the save service).
//
// The intent is to make accidental drift to a provider-specific package
// fail loud at unit-test time rather than at integration-test time.
var allowedDownstreamDriveImports = map[string]struct{}{
	"github.com/smackerel/smackerel/internal/drive":               {}, // provider-neutral interface + registry
	"github.com/smackerel/smackerel/internal/drive/rules":         {}, // save rules engine
	"github.com/smackerel/smackerel/internal/drive/save":          {}, // save service
	"github.com/smackerel/smackerel/internal/drive/retrieve":      {}, // retrieval service
	"github.com/smackerel/smackerel/internal/drive/policy":        {}, // policy engine
	"github.com/smackerel/smackerel/internal/drive/extract":       {}, // extraction service
	"github.com/smackerel/smackerel/internal/drive/scan":          {}, // scan service
	"github.com/smackerel/smackerel/internal/drive/monitor":       {}, // monitor service
	"github.com/smackerel/smackerel/internal/drive/health":        {}, // health tracker
	"github.com/smackerel/smackerel/internal/drive/confirm":       {}, // confirmations service
	"github.com/smackerel/smackerel/internal/drive/tools":         {}, // agent tool registration
	"github.com/smackerel/smackerel/internal/drive/consumers":     {}, // this package
	"github.com/smackerel/smackerel/internal/drive/observability": {}, // metrics + log helpers
}

// TestDriveConsumersUseArtifactStoreAndNeverProviderPackages enforces
// the SCN-038-022 contract: downstream features (recipes, expenses,
// lists, annotations, meal planning, digest, domain extraction, agent,
// API, web, Telegram) MUST consume drive artifacts through the
// provider-neutral surface and MUST NOT import any drive provider
// implementation package directly.
//
// Mechanism: parse every non-test Go file under each downstream package
// (recursively) and assert no import path appears in forbiddenImports
// AND any internal/drive/* import is in allowedDownstreamDriveImports.
//
// Adversarial guards:
//
//   - The forbidden list explicitly names internal/drive/google so the
//     test fails the first time any downstream consumer adds a Google
//     Drive type assertion (the bug Scope 8 is designed to prevent).
//   - The allowed list is closed: a downstream consumer that imports
//     internal/drive/<new-provider> fails the test even if the new
//     package is also provider-neutral. This forces the architect to
//     either add the new package to the allowlist (with a comment) or
//     route it through this consumers package, preventing accidental
//     coupling.
//   - The walk descends into subdirectories so a single naive consumer
//     under any subpackage is caught.
func TestDriveConsumersUseArtifactStoreAndNeverProviderPackages(t *testing.T) {
	root := repoRoot(t)

	type violation struct {
		file    string
		imp     string
		pkgPath string
	}
	var violations []violation
	scannedFiles := 0

	for _, pkg := range downstreamPackages {
		pkgRoot := filepath.Join(root, pkg)
		walkErr := filepath.Walk(pkgRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Skip generated test fixtures and the consumers
				// package's own test (which intentionally references
				// the forbidden imports for documentation purposes).
				if path == filepath.Join(root, "internal/drive/consumers") {
					return nil
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			scannedFiles++

			fset := token.NewFileSet()
			astFile, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				t.Fatalf("parse %s: %v", path, parseErr)
			}
			for _, imp := range astFile.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, forbidden := range forbiddenImports {
					if importPath == forbidden {
						violations = append(violations, violation{
							file:    relPath(root, path),
							imp:     importPath,
							pkgPath: pkg,
						})
					}
				}
				if strings.HasPrefix(importPath, "github.com/smackerel/smackerel/internal/drive") {
					if _, ok := allowedDownstreamDriveImports[importPath]; !ok {
						violations = append(violations, violation{
							file:    relPath(root, path),
							imp:     importPath,
							pkgPath: pkg,
						})
					}
				}
			}
			return nil
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			t.Fatalf("walk %s: %v", pkgRoot, walkErr)
		}
	}

	if scannedFiles < 50 {
		t.Fatalf(
			"consumer contract scan covered only %d Go files; downstream packages must "+
				"contain real source code or this test silently passes against an empty workspace",
			scannedFiles,
		)
	}

	if len(violations) > 0 {
		for _, v := range violations {
			t.Errorf(
				"downstream package %s file %s imports forbidden / un-allowlisted drive package %q — "+
					"route through internal/drive/consumers or extend allowedDownstreamDriveImports with rationale",
				v.pkgPath, v.file, v.imp,
			)
		}
		t.Fatalf("%d provider-coupling violation(s) detected (scanned %d files)", len(violations), scannedFiles)
	}
}

// repoRoot walks upward from the test binary's directory to find the
// repository root (identified by the go.mod file). Returning the root
// lets the test scan canonical source paths under internal/* without
// hardcoding the developer's checkout location.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", wd)
		}
		dir = parent
	}
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
