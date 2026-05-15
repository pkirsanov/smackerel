package main

// AST regression guard for Gate G028 (NO-DEFAULTS / fail-loud SST) and
// HL-RESCAN-014 (helpers-unused-fail-soft cleanup).
//
// Bug context: BUG-020-003 deleted five env-reading helpers from
// cmd/core/helpers.go (parseFloatEnv, parseJSONArrayEnv,
// parseJSONObjectEnv, parseJSONObject, parseJSONObjectVal) because they
// were dead-set wiring scaffolding that returned a silent zero/nil on
// missing or malformed env vars instead of propagating failure to the
// operator. Their continued existence in the wiring layer was a constant
// invitation to wire fail-soft fallbacks for new connectors.
//
// This guard prevents regression by mechanically failing the build if
// any future helper in cmd/core/*.go re-introduces the same shape:
// reading an env var with os.Getenv(...) and silently returning a
// literal / nil on empty, instead of failing loud (panic / log.Fatal /
// os.Exit / returning an error).
//
// Pattern modeled after internal/drive/consumers/consumer_contract_test.go
// (downstream-coupling guard) — same approach: walk the package's Go
// files, parse with go/parser, inspect AST, aggregate violations with
// file:line, fail loud with named guidance.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minScannedFilesInCmdCore is the lower bound on non-test *.go files the
// scanner MUST find under cmd/core/ for the test to be considered to
// have actually run. If cmd/core/ is ever pruned to fewer than 3 source
// files (today: helpers.go + connectors.go + main.go + … >= 3) the
// scanner must fail loud rather than silently passing against an empty
// directory.
const minScannedFilesInCmdCore = 3

// TestNoSilentFallbackHelpersInCmdCore mechanically detects functions
// whose shape matches the BUG-020-003 / HL-RESCAN-014 anti-pattern:
//
//	func fooEnv(key string) <T> {
//	    s := os.Getenv(key)
//	    if s == "" {
//	        return <literal-or-nil>   // ← silent fail-soft fallback
//	    }
//	    // ... try to parse, on error: return <literal-or-nil> ...
//	}
//
// The matcher requires BOTH conditions in the same FuncDecl body:
//
//  1. At least one CallExpr to os.Getenv(...) (SelectorExpr X="os",
//     Sel="Getenv").
//  2. At least one IfStmt whose condition is a BinaryExpr `<ident> == ""`
//     and whose Then-block contains a ReturnStmt that returns ONLY
//     literal / nil values WITHOUT a preceding panic / log.Fatal /
//     os.Exit and WITHOUT propagating an error result and WITHOUT any
//     preceding function call (which would indicate fail-loud
//     signaling such as fmt.Fprintln(os.Stderr, ...), slog.Error, or a
//     log.Print).
//
// The "no preceding call" rule is critical for cmd/core specifically:
// CLI subcommand handlers (cmd_*.go) legitimately read os.Getenv,
// print a named error to stderr, and return a non-zero int exit code
// when a required env var is missing. That is the CLI form of
// fail-loud SST and is NOT the bug shape. The deleted helpers had
// bare `return 0` / `return nil` with NO preceding call — zero
// signal to the operator. The matcher distinguishes these two shapes
// by requiring the IfStmt body to be devoid of any preceding
// CallExpr.
//
// Any function in cmd/core/*.go that matches this shape fails the test
// with file:line and a Gate G028 / HL-RESCAN-014 reference. Production
// code that needs an env value MUST either (a) panic / log.Fatal /
// os.Exit on missing config (fail-loud SST per
// .github/instructions/smackerel-no-defaults.instructions.md), or (b)
// return an error and let the caller fail loud, or (c) print a named
// error to stderr and return a non-zero exit code (CLI form).
//
// Adversarial coverage is provided by the "AdversarialSyntheticAST"
// sub-test below, which proves the matcher fires on a hand-crafted
// forbidden function AND does NOT fire on the legitimate fail-loud
// CLI subcommand shape — closing the RED→GREEN regression loop
// without requiring a real production regression to validate the
// guard.
func TestNoSilentFallbackHelpersInCmdCore(t *testing.T) {
	root := repoRootCmdCore(t)
	cmdCoreDir := filepath.Join(root, "cmd", "core")

	type violation struct {
		file string
		line int
		fn   string
	}
	var violations []violation
	scannedFiles := 0

	walkErr := filepath.Walk(cmdCoreDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
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
		astFile, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		for _, decl := range astFile.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Body == nil {
				continue
			}
			if !functionMatchesSilentFallbackShape(fn) {
				continue
			}
			pos := fset.Position(fn.Pos())
			violations = append(violations, violation{
				file: relPathCmdCore(root, path),
				line: pos.Line,
				fn:   fn.Name.Name,
			})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", cmdCoreDir, walkErr)
	}

	if scannedFiles < minScannedFilesInCmdCore {
		t.Fatalf(
			"silent-fallback scan covered only %d Go files under cmd/core/ "+
				"(expected >= %d) — guard would silently pass against an "+
				"empty or pruned directory; check file walk and exclusions",
			scannedFiles, minScannedFilesInCmdCore,
		)
	}

	if len(violations) > 0 {
		for _, v := range violations {
			t.Errorf(
				"%s:%d: function %s matches silent-fallback signature shape "+
					"(reads os.Getenv and returns literal/nil on empty without "+
					"panic/log.Fatal/os.Exit/error propagation) — "+
					"Gate G028 / HL-RESCAN-014 forbids fail-soft env helpers in cmd/core; "+
					"either fail loud on missing config or return an error",
				v.file, v.line, v.fn,
			)
		}
		t.Fatalf(
			"%d silent-fallback helper(s) detected in cmd/core/ (scanned %d files); "+
				"see Gate G028 NO-DEFAULTS policy and "+
				".github/instructions/smackerel-no-defaults.instructions.md",
			len(violations), scannedFiles,
		)
	}
}

// TestNoSilentFallbackHelpersInCmdCore_AdversarialSyntheticAST proves
// that the matcher in functionMatchesSilentFallbackShape actually fires
// on a forbidden pattern AND correctly excludes the legitimate
// fail-loud CLI subcommand pattern. Without this, a broken matcher
// (e.g. one that always returns false, or one that fires on every CLI
// handler) would either let the production scan above pass vacuously
// and silently regress the bug, or generate false-positive noise that
// trains operators to ignore it.
//
// The synthetic source contains three functions:
//
//   - bad: reads os.Getenv, returns 0 on empty string with NO
//     preceding call. MUST trip the matcher (this is the deleted-
//     helper shape).
//   - goodPanic: reads os.Getenv, panics on empty string (fail-loud
//     SST). MUST NOT trip the matcher.
//   - goodCLI: reads os.Getenv, prints a named error to stderr and
//     returns a non-zero exit code on empty string (fail-loud CLI).
//     MUST NOT trip the matcher.
//
// If any expectation is violated, the regression guard is itself
// broken and the test fails loud.
func TestNoSilentFallbackHelpersInCmdCore_AdversarialSyntheticAST(t *testing.T) {
	const synthetic = `package fake

import (
	"fmt"
	"os"
)

// bad MUST be flagged: reads env, returns literal on empty without
// fail-loud guard or any preceding signaling call.
func bad(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	return 1.5
}

// goodPanic MUST NOT be flagged: panics on empty (fail-loud SST).
func goodPanic(key string) string {
	s := os.Getenv(key)
	if s == "" {
		panic("missing required env: " + key)
	}
	return s
}

// goodCLI MUST NOT be flagged: prints a named error to stderr and
// returns a non-zero exit code (fail-loud CLI subcommand pattern).
func goodCLI(key string) int {
	s := os.Getenv(key)
	if s == "" {
		fmt.Fprintln(os.Stderr, "missing required env: "+key)
		return 2
	}
	_ = s
	return 0
}
`

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "synthetic.go", synthetic, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}

	var flagged []string
	for _, decl := range astFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if functionMatchesSilentFallbackShape(fn) {
			flagged = append(flagged, fn.Name.Name)
		}
	}

	if len(flagged) != 1 || flagged[0] != "bad" {
		t.Fatalf(
			"adversarial synthetic AST: matcher flagged %v, expected exactly [bad] — "+
				"matcher is broken and would either let real regressions slip through or "+
				"flag legitimate fail-loud CLI subcommand handlers",
			flagged,
		)
	}
}

// functionMatchesSilentFallbackShape returns true when fn's body both
// (a) calls os.Getenv(...) and (b) contains an IfStmt of the form
// `if <ident> == "" { return <literal-or-nil> ... }` whose Then-block
// does NOT contain a panic / log.Fatal / os.Exit guard and does NOT
// return an error.
func functionMatchesSilentFallbackShape(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}

	hasGetenv := false
	hasSilentEmptyReturn := false

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if isOsGetenvCall(node) {
				hasGetenv = true
			}
		case *ast.IfStmt:
			if isEmptyStringEqualityCheck(node.Cond) && bodyReturnsSilentLiteral(node.Body) {
				hasSilentEmptyReturn = true
			}
		}
		return true
	})

	return hasGetenv && hasSilentEmptyReturn
}

// isOsGetenvCall returns true when ce is a call to os.Getenv (any args).
func isOsGetenvCall(ce *ast.CallExpr) bool {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "os" && sel.Sel.Name == "Getenv"
}

// isEmptyStringEqualityCheck returns true when expr is `<ident> == ""`.
func isEmptyStringEqualityCheck(expr ast.Expr) bool {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op != token.EQL {
		return false
	}
	if _, ok := bin.X.(*ast.Ident); !ok {
		return false
	}
	lit, ok := bin.Y.(*ast.BasicLit)
	if !ok {
		return false
	}
	return lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
}

// bodyReturnsSilentLiteral returns true when block contains a
// ReturnStmt that returns only literal / nil values AND the block does
// NOT contain a preceding fail-loud guard (panic / log.Fatal / os.Exit)
// AND the block contains NO function call before the return (a
// preceding call indicates fail-loud signaling: fmt.Fprintln to
// stderr, slog.Error, log.Println, etc.) AND the return does NOT
// propagate an error (i.e. does not include a non-nil error
// identifier as the last result).
func bodyReturnsSilentLiteral(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}

	if blockContainsFailLoudGuard(block) {
		return false
	}
	if blockContainsCallBeforeReturn(block) {
		return false
	}

	for _, stmt := range block.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok {
			continue
		}
		if returnIsSilentLiteral(ret) {
			return true
		}
	}
	return false
}

// blockContainsCallBeforeReturn returns true when block contains any
// CallExpr in any statement BEFORE the first ReturnStmt. This
// distinguishes the bare-return bug shape (deleted helpers had
// `if s == "" { return 0 }` with NO call) from the fail-loud CLI
// subcommand shape (handlers print a named error to stderr first,
// then return a non-zero exit code).
func blockContainsCallBeforeReturn(block *ast.BlockStmt) bool {
	for _, stmt := range block.List {
		if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
			return false
		}
		hasCall := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if hasCall {
				return false
			}
			if _, ok := n.(*ast.CallExpr); ok {
				hasCall = true
				return false
			}
			return true
		})
		if hasCall {
			return true
		}
	}
	return false
}

// blockContainsFailLoudGuard returns true when the block contains a
// panic, log.Fatal*, or os.Exit call statement (any arguments).
func blockContainsFailLoudGuard(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		expr, ok := n.(*ast.ExprStmt)
		if !ok {
			return true
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		// panic(...)
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "panic" {
			found = true
			return false
		}
		// log.Fatal*, os.Exit, slog.Error+os.Exit, etc.
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg.Name == "os" && sel.Sel.Name == "Exit" {
			found = true
			return false
		}
		if pkg.Name == "log" && strings.HasPrefix(sel.Sel.Name, "Fatal") {
			found = true
			return false
		}
		return true
	})
	return found
}

// returnIsSilentLiteral returns true when ret returns ONLY literal,
// nil, or composite-literal values (no identifiers other than nil, no
// function calls, no error propagation). An empty `return` with no
// results is also treated as silent (the caller has no error signal).
func returnIsSilentLiteral(ret *ast.ReturnStmt) bool {
	if len(ret.Results) == 0 {
		return true
	}
	for _, res := range ret.Results {
		if !isLiteralOrNil(res) {
			return false
		}
	}
	return true
}

// isLiteralOrNil returns true when expr is a basic literal, the
// identifier `nil`, a unary literal (e.g. -1), or a composite literal
// (e.g. []byte{}, map[string]string{}). Identifiers that name variables
// (including error variables) are NOT literals — those signal real
// propagation and exempt the function from the silent-fallback shape.
func isLiteralOrNil(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return node.Name == "nil"
	case *ast.UnaryExpr:
		return isLiteralOrNil(node.X)
	case *ast.CompositeLit:
		return true
	case *ast.CallExpr:
		// Type conversions of literals (e.g. float64(0)) count as literal.
		if len(node.Args) != 1 {
			return false
		}
		if _, ok := node.Fun.(*ast.Ident); !ok {
			return false
		}
		return isLiteralOrNil(node.Args[0])
	default:
		return false
	}
}

// repoRootCmdCore walks upward from the test binary's directory to find
// the repository root (identified by go.mod). Same pattern as
// internal/drive/consumers/consumer_contract_test.go's repoRoot.
func repoRootCmdCore(t *testing.T) string {
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

// relPathCmdCore returns path relative to root, or path verbatim on
// error. Same helper shape as relPath in the drive consumers test.
func relPathCmdCore(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
