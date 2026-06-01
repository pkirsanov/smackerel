//go:build integration

// Spec 066 SCOPE-3 — TestNaturalLanguageFindUsesRetrievalScenarioNotSlashHandler.
//
// Retirement guarantee proof for the SCN-066-A02 scenario. This test
// is the in-process live spec-066 contract (the e2e-api rows MAY use
// the SCOPE-2 skip-pending-harness pattern; this row MUST pass without
// skip per scope DoD).
//
// Orthogonal claim vs spec 068 SCN-068-A02
// (TestIntentReadRoutingFacade_RetrievalReceivesStructuredContext):
// spec 068 proves "facade compiles before routing and receives
// structured context"; this spec-066 test proves the absence
// guarantee — when a plain-English find/rate request is routed via
// the assistant facade, the retired
// `internal/telegram/annotation.go::handleRate` codepath is NOT
// reachable from the Telegram command dispatcher and the request
// instead resolves to the retrieval scenario.

package assistant_integration

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestNaturalLanguageFindUsesRetrievalScenarioNotSlashHandler asserts
// two orthogonal facets of the SCOPE-3 retirement:
//
//  1. Structural retirement (call-order/spy boundary by AST): the
//     Telegram command dispatcher in internal/telegram/bot.go no
//     longer routes "/rate" to b.handleRate, and the handleRate
//     method itself is gone from internal/telegram/annotation.go.
//
//  2. Live behavioral routing: driving the assistant.Facade with a
//     plain-English "find my notes about ACL tags" turn lands on the
//     retrieval_qa scenario via the spec 068 compiled-intent path
//     (NOT via any /find or /rate slash dispatch).
func TestNaturalLanguageFindUsesRetrievalScenarioNotSlashHandler(t *testing.T) {
	repoRoot := findRepoRootForSpec066(t)

	// --- (1) Structural absence guarantee on the retired call site.
	botPath := filepath.Join(repoRoot, "internal", "telegram", "bot.go")
	annotPath := filepath.Join(repoRoot, "internal", "telegram", "annotation.go")
	assertFileLacks(t, botPath, `case "rate":`)
	assertFileLacks(t, botPath, `b.handleRate(`)
	assertFileLacks(t, annotPath, `func (b *Bot) handleRate(`)

	// AST proof: handleRate is not declared on *Bot anywhere in the
	// telegram package. This is the spy boundary — even if a future
	// edit re-introduced the string "handleRate" inside a comment,
	// only a real FuncDecl with receiver *Bot would re-trigger this
	// assertion.
	fset := token.NewFileSet()
	pkgDir := filepath.Join(repoRoot, "internal", "telegram")
	pkgs, err := parser.ParseDir(fset, pkgDir, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", pkgDir, err)
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Name == nil || fd.Recv == nil || fd.Name.Name != "handleRate" {
					continue
				}
				if len(fd.Recv.List) == 0 {
					continue
				}
				if recvIsBotPointer(fd.Recv.List[0]) {
					t.Fatalf("AST still declares (*Bot).handleRate at %s — retired dispatch not removed", fset.Position(fd.Pos()))
				}
			}
		}
	}

	// --- (2) Live routing: plain-English find lands on retrieval_qa,
	// NOT via the retired /find or /rate slash dispatch.
	ft := &stubTransport{resolve: func(_ string) string { return retrievalIntentJSON(t) }}
	compiler := buildCompiler(t, ft)

	sc := &agent.Scenario{ID: "retrieval_qa", Version: "v1"}
	router := &recordingRouter{byID: map[string]*agent.Scenario{"retrieval_qa": sc}}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"retrieval_qa": sc})
	f := buildReadFacade(t, compiler, router, registry, map[string]assistant.ManifestEntryForTest{
		"retrieval_qa": {UserFacingLabel: "retrieval", EnableSSTKey: "assistant.skills.retrieval_qa.enabled", Enabled: true},
	})

	if _, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-spec-066-scope-3",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "find my notes about ACL tags",
	}); err != nil {
		t.Fatalf("facade.Handle: %v", err)
	}

	env := router.lastEnvelope(t)
	if env.ScenarioID != "retrieval_qa" {
		t.Fatalf("plain-English find routed to %q, want retrieval_qa (legacy /find slash dispatch must NOT be invoked)", env.ScenarioID)
	}
}

// findRepoRootForSpec066 walks up from the test cwd looking for go.mod
// so the structural assertions stay independent of the cwd Go test
// chooses. Suffixed to avoid collision with any other helper in this
// test package.
func findRepoRootForSpec066(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", cwd)
		}
		dir = parent
	}
}

func assertFileLacks(t *testing.T, path, needle string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(data), needle) {
		t.Fatalf("%s still contains %q — retired call site must be removed", path, needle)
	}
}

func recvIsBotPointer(field *ast.Field) bool {
	star, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "Bot"
}
