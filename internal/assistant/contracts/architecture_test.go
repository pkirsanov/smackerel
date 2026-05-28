package contracts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test working directory until it finds
// a go.mod file (module root).
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %q", wd)
	return ""
}

// forbiddenAssistantSubpackages is the canonical list from
// design.md §10 (Forbidden paths) + §11.3 test #1. Their existence
// would re-introduce a parallel agent substrate that spec 037 already
// owns and that spec 061 §3.1.4 explicitly forbids.
var forbiddenAssistantSubpackages = []string{
	"router",
	"registry",
	"executor",
	"tracer",
	"loader",
	"llm",
	"nats",
}

// TestArchitecture_NoForbiddenAssistantSubpackages — design.md §11.3
// test #1. Fails if any of internal/assistant/{router,registry,
// executor,tracer,loader,llm,nats}/ exists.
func TestArchitecture_NoForbiddenAssistantSubpackages(t *testing.T) {
	root := repoRoot(t)
	assistantDir := filepath.Join(root, "internal", "assistant")
	if _, err := os.Stat(assistantDir); os.IsNotExist(err) {
		t.Fatalf("internal/assistant does not exist; expected the parent package to be present")
	}
	for _, sub := range forbiddenAssistantSubpackages {
		path := filepath.Join(assistantDir, sub)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			t.Errorf("forbidden package directory exists: internal/assistant/%s/ (design §10 + §11.3 forbid this — re-introduces parallel spec 037 substrate)", sub)
		}
	}
}

// forbiddenTransportImportPrefixes is the canonical list from
// design.md §11.3 test #2. Any internal/assistant/... package that
// imports one of these prefixes is a capability→transport leak.
var forbiddenTransportImportPrefixes = []string{
	"github.com/smackerel/smackerel/internal/telegram",
	"github.com/smackerel/smackerel/internal/whatsapp",
	"github.com/smackerel/smackerel/internal/webchat",
	"github.com/smackerel/smackerel/internal/mobile",
}

// TestArchitecture_NoCapabilityToTransportImports — design.md §11.3
// test #2. Walks every .go file under internal/assistant/... and
// fails on any import path whose value begins with one of
// forbiddenTransportImportPrefixes.
func TestArchitecture_NoCapabilityToTransportImports(t *testing.T) {
	root := repoRoot(t)
	assistantDir := filepath.Join(root, "internal", "assistant")
	fset := token.NewFileSet()

	var failures []string
	err := filepath.WalkDir(assistantDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Architecture test itself contains the forbidden prefix
		// strings as DATA (not as imports). Skip it to avoid a
		// self-tautology; the AST-import walk below would not
		// match strings inside an []string literal anyway, but
		// we also conservatively exclude the test file by name.
		base := filepath.Base(path)
		if base == "architecture_test.go" {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			// A parse error here would mask architecture
			// violations — surface it as a test failure.
			failures = append(failures, "parse error: "+path+": "+err.Error())
			return nil
		}
		for _, imp := range f.Imports {
			if imp.Path == nil {
				continue
			}
			lit := strings.Trim(imp.Path.Value, `"`)
			for _, fp := range forbiddenTransportImportPrefixes {
				if lit == fp || strings.HasPrefix(lit, fp+"/") {
					rel, _ := filepath.Rel(root, path)
					failures = append(failures, "capability→transport leak: "+rel+" imports "+lit)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q): %v", assistantDir, err)
	}
	for _, f := range failures {
		t.Error(f)
	}
}

// TestArchitecture_ImportLintCatchesDeliberatelyBrokenFixture —
// adversarial proof that the import-lint walker above would catch a
// regression. We build a synthetic AST file in-memory whose import
// list includes a forbidden prefix, parse it, and assert the same
// detection predicate fires. This guards the test logic itself, not
// the runtime tree.
func TestArchitecture_ImportLintCatchesDeliberatelyBrokenFixture(t *testing.T) {
	src := `package assistantbroken
import (
    _ "github.com/smackerel/smackerel/internal/telegram"
)
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "broken.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		imp, ok := n.(*ast.ImportSpec)
		if !ok || imp.Path == nil {
			return true
		}
		lit := strings.Trim(imp.Path.Value, `"`)
		for _, fp := range forbiddenTransportImportPrefixes {
			if lit == fp || strings.HasPrefix(lit, fp+"/") {
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		t.Fatal("import-lint predicate did NOT detect a deliberately-broken fixture importing internal/telegram — the runtime check would also miss real regressions")
	}
}
