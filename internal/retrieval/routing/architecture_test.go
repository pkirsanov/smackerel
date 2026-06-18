// Spec 095 SCOPE-03 — architecture tests enforcing the single-graph invariant
// (Principle 5 — One Graph, Many Views). These tests mechanically prove that
// NO package under internal/retrieval introduces a parallel vector index,
// database, or graph store, and that the router never re-classifies intent
// (NFR-1). Each test carries a `would_catch_regression` adversarial sub-test
// that constructs the forbidden pattern and asserts the scanner trips — so a
// future regression cannot silently slip a second store past the gate
// (bubbles-test-integrity: no tautological guard).
//
// References:
//   - specs/095-retrieval-strategy-routing/design.md §1, §12
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-03
package routing

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbiddenStoreConstructors are call patterns that CREATE a backing store. A
// strategy that opens one of these has introduced a parallel store (Principle
// 5 violation). The existing store is INJECTED as an interface; internal/
// retrieval never constructs one.
var forbiddenStoreConstructors = []string{
	"pgxpool.New",
	"pgxpool.NewWithConfig",
	"pgx.Connect",
	"pgx.ConnectConfig",
	"sql.Open",
	"sqlx.Open",
	"sqlx.Connect",
}

// forbiddenStoreImportSubstrings are import-path substrings of second
// store/index/graph backends. Importing one means a parallel store was
// introduced.
var forbiddenStoreImportSubstrings = []string{
	"jackc/pgx", // a Postgres driver — the existing store is injected, not opened here
	"qdrant",
	"weaviate",
	"milvus",
	"pinecone",
	"neo4j",
	"go-elasticsearch",
	"opensearch",
	"meilisearch",
	"blevesearch",
	"tantivy",
}

// forbiddenReclassifyImportSubstrings are import paths that would let the
// router re-run intent classification (a second LLM round-trip — NFR-1
// violation). The router consumes the ALREADY-COMPUTED CompiledIntent from
// internal/assistant/intent (the TYPE package, allowed) but must not reach the
// agent/LLM bridge or the ML sidecar client.
var forbiddenReclassifyImportSubstrings = []string{
	"internal/agent",
	"internal/ml",
	"mlclient",
	"internal/assistant/provenance",
}

// forbiddenReclassifyCalls are method calls that would re-classify intent.
var forbiddenReclassifyCalls = []string{
	".Compile(",
	".Recompile(",
	".ClassifyIntent(",
	".CompileIntent(",
}

// retrievalRoot returns the absolute path to internal/retrieval/ derived from
// this test file's location (robust regardless of CWD).
func retrievalRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../internal/retrieval/routing/architecture_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

// collectGoSources walks root and returns {absPath: content} for every
// non-_test.go file.
func collectGoSources(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rErr := os.ReadFile(path)
		if rErr != nil {
			return rErr
		}
		out[path] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(out) == 0 {
		t.Fatalf("no Go sources found under %s — scan would be vacuous", root)
	}
	return out
}

// scanStoreViolations parses content (as filename) and returns the list of
// forbidden store constructors / imports it contains. Reused by both the
// real-tree scan and the would_catch_regression synthetic scan.
func scanStoreViolations(filename, content string) []string {
	var hits []string
	for _, ctor := range forbiddenStoreConstructors {
		if strings.Contains(content, ctor) {
			hits = append(hits, "store-constructor:"+ctor)
		}
	}
	for _, imp := range importPaths(filename, content) {
		for _, bad := range forbiddenStoreImportSubstrings {
			if strings.Contains(imp, bad) {
				hits = append(hits, "store-import:"+imp)
			}
		}
	}
	return hits
}

// scanReclassifyViolations returns forbidden re-classification imports/calls.
func scanReclassifyViolations(filename, content string) []string {
	var hits []string
	for _, call := range forbiddenReclassifyCalls {
		if strings.Contains(content, call) {
			hits = append(hits, "reclassify-call:"+call)
		}
	}
	for _, imp := range importPaths(filename, content) {
		for _, bad := range forbiddenReclassifyImportSubstrings {
			if strings.Contains(imp, bad) {
				hits = append(hits, "reclassify-import:"+imp)
			}
		}
	}
	return hits
}

// importPaths parses content and returns its import paths. Falls back to an
// empty slice when the source is unparseable (the constructor/call scan still
// runs on raw content).
func importPaths(filename, content string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, content, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out
}

// TestNoParallelStore proves no package under internal/retrieval opens a
// second DB pool, vector index, or graph store (Principle 5).
func TestNoParallelStore(t *testing.T) {
	root := retrievalRoot(t)
	for path, content := range collectGoSources(t, root) {
		if hits := scanStoreViolations(path, content); len(hits) > 0 {
			t.Errorf("%s introduces a parallel store (Principle 5 violation): %v", path, hits)
		}
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// A strategy that opens its own pool MUST be flagged.
		bad := `package x
import "github.com/jackc/pgx/v5/pgxpool"
func New(dsn string) { pool, _ := pgxpool.New(nil, dsn); _ = pool }`
		hits := scanStoreViolations("synthetic.go", bad)
		if len(hits) == 0 {
			t.Fatal("scanner failed to catch a pgxpool.New parallel-store regression")
		}
		// A second vector index import MUST be flagged.
		bad2 := `package x
import qdrant "github.com/qdrant/go-client/qdrant"
var _ = qdrant.Client{}`
		if hits2 := scanStoreViolations("synthetic2.go", bad2); len(hits2) == 0 {
			t.Fatal("scanner failed to catch a qdrant parallel-index import regression")
		}
	})
}

// TestRouterDoesNotReclassify proves the routing core consumes the
// already-computed CompiledIntent and never reaches the agent/LLM bridge or
// re-runs classification (NFR-1).
func TestRouterDoesNotReclassify(t *testing.T) {
	// Scan only the routing-core package dir (not the strategy overlays).
	_, thisFile, _, _ := runtime.Caller(0)
	coreDir := filepath.Dir(thisFile)
	for path, content := range collectGoSources(t, coreDir) {
		// Only the immediate package dir, not nested strategy packages.
		if filepath.Dir(path) != coreDir {
			continue
		}
		if hits := scanReclassifyViolations(path, content); len(hits) > 0 {
			t.Errorf("%s re-classifies intent (NFR-1 violation): %v", path, hits)
		}
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		bad := `package routing
import "github.com/smackerel/smackerel/internal/agent"
func (r *Router) reroute() { _ = agent.JudgmentRunner(nil) }`
		if hits := scanReclassifyViolations("synthetic.go", bad); len(hits) == 0 {
			t.Fatal("scanner failed to catch an internal/agent re-classification import regression")
		}
		bad2 := `package routing
func (r *Router) reroute(c Compiler) { c.Compile(nil, RawTurn{}) }`
		if hits2 := scanReclassifyViolations("synthetic2.go", bad2); len(hits2) == 0 {
			t.Fatal("scanner failed to catch a .Compile( re-classification call regression")
		}
	})
}

// TestReadsExistingStoreOnly proves the strategy overlays read the existing
// store ONLY through injected interfaces (ArtifactFetcher / SpendAggregator /
// VagueRecallExecutor declared in strategy.go) and never construct a store.
func TestReadsExistingStoreOnly(t *testing.T) {
	root := retrievalRoot(t)

	// The injected dependency interfaces MUST be declared (the strategies
	// consume the existing store through them, not through a new pool).
	stratSrc := mustReadFile(t, filepath.Join(filepath.Dir(callerFile(t)), "strategy.go"))
	for _, iface := range []string{"ArtifactFetcher interface", "SpendAggregator interface", "VagueRecallExecutor interface"} {
		if !strings.Contains(stratSrc, iface) {
			t.Errorf("strategy.go must declare the injected dependency interface %q (existing-store-only contract)", iface)
		}
	}

	// Every strategy-overlay file reads the existing store via injected
	// interfaces — it constructs no store and imports no second backend.
	stratRoot := filepath.Join(root, "routing", "strategies")
	if _, err := os.Stat(stratRoot); err == nil {
		for path, content := range collectGoSources(t, stratRoot) {
			if hits := scanStoreViolations(path, content); len(hits) > 0 {
				t.Errorf("%s reads a NEW store instead of the existing one (Principle 5 violation): %v", path, hits)
			}
			if !strings.Contains(content, "internal/retrieval/routing") {
				t.Errorf("%s must consume the injected routing dependency interfaces (import internal/retrieval/routing)", path)
			}
		}
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		bad := `package wholedocument
import "database/sql"
func fetch(dsn string) { db, _ := sql.Open("postgres", dsn); _ = db }`
		if hits := scanStoreViolations("synthetic.go", bad); len(hits) == 0 {
			t.Fatal("scanner failed to catch a sql.Open parallel-store regression in a strategy overlay")
		}
	})
}

func callerFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return thisFile
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
