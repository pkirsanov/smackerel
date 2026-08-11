// Spec 108 Scope 04 — T1 structural grep guard for the Telegram bridge.
//
// design.md §10.10 layer T1: `internal/telegram/**` contains no string
// matching `ScopeNameRegex` (`^[a-z][a-z0-9-]*:[a-z0-9,_-]+$`).
// Precedent: `internal/auth/sst_grep_guard_test.go`.
//
// WHY THIS LAYER EXISTS. The hardcoded `Scopes: []string{"annotation:edit"}`
// at `per_user_token.go:201` was REPLACED by derivation from the mapped
// principal's recorded grants (spec.md §18 decision 3: authority is defined
// at the principal, never at the minter). Every behavioral layer (T2–T5)
// asserts what the code DOES, so each is only as good as its fixtures. This
// layer constrains what the source may CONTAIN, so no fixture choice can
// make it pass falsely. §10.10 calls it "the only layer that survives an
// author who is actively trying to reintroduce the shortcut."
//
// A scope literal reappearing anywhere in this package means authority has
// drifted back to the minter. The sanctioned alternative is to name the
// grant symbolically via `auth.TelegramBridgeDelegableGrants` (the closed
// delegation ceiling) or `auth.GrantAnnotationEdit` — which is exactly what
// `per_user_token_test.go` already does, and what this file does below.
//
// # Two layers, and why `_test.go` is not simply exempt
//
// The `ScopeNameRegex` SHAPE is ambiguous: it matches far more than scope
// names. Ollama model tags (`deepseek-r1:7b`, `gemma4:26b`), digests
// (`sha256:x`), and Telegram callback payloads (`cook:42`, `jump:5`) all
// share it, and this package's tests legitimately hold ~60 of them. Applying
// the shape to `_test.go` would therefore be ~60 false positives and zero
// true ones — a guard that must be muzzled to stay green gets muzzled, and
// then it guards nothing.
//
// Exempting tests wholesale would be worse: it creates precisely the hiding
// place a determined author would use. So tests are NOT exempt. They are
// held to a NARROWER rule that targets the thing that actually matters:
//
//	Layer A (non-`_test.go`): the full `ScopeNameRegex` shape. Zero
//	  tolerance. This is the surface that compiles into the bridge binary,
//	  so this is where a minter-side grant list would have to live to have
//	  any effect. Verified against the tree: zero matches today, so the
//	  strict shape is affordable here. A future legitimate colon-shaped
//	  literal (say a model tag) must be dealt with consciously — that
//	  friction is the point of a ratchet.
//
//	Layer B (`_test.go`): exact-match scan for the grant VOCABULARY only.
//	  The forbidden set is read from `auth.TelegramBridgeDelegableGrants`
//	  at run time rather than re-typed, so widening the ceiling
//	  automatically forbids the new grant's literal form here too — the
//	  guard cannot drift away from the thing it guards.
//
// Layer B is a real constraint rather than a formality even though Go's
// build model already prevents a `_test.go` symbol from being referenced by
// production code (so a literal there could not itself become the minter's
// authority source). What it stops is the copy-paste seed: a grant list
// sitting in a test fixture is the draft of the next production regression.
//
// This file obeys its own rule. It never writes a grant literal; the
// adversarial fixtures below are built from `auth.*` constants at run time.
//
// # Anti-vacuity
//
// A structural guard whose detector is broken is decorative: it passes
// forever and catches nothing. Three sub-tests exist solely to prove this
// one can fail —
//
//   - `TestScopeLiteralGuard_MatcherIsNotVacuous` pins the matcher against a
//     table of positives and negatives, including the exact strings the
//     regex must NOT claim (`https://example.com`, `key: value`).
//   - `TestScopeLiteralGuard_ShapeScanIsNotVacuous` plants a grant literal in
//     a production-shaped fixture and asserts the walker reports it.
//   - `TestScopeLiteralGuard_VocabularyScanIsNotVacuous` plants one in a
//     `_test.go`-shaped fixture and asserts Layer B reports it, while a model
//     tag in the same fixture is correctly ignored.
//
// References:
//   - specs/108-corpus-grant-enforcement/design.md §10.10 (proof layers)
//   - specs/108-corpus-grant-enforcement/scopes.md Scope 04 (SCN-108-E04)
//   - internal/auth/bridge_delegation.go (the sanctioned grant vocabulary)
package telegram

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

// scopeNameRegex is `ScopeNameRegex` from design.md §10.10, verbatim.
//
// It is ANCHORED, so it describes a whole string rather than a substring.
// That is what keeps it usable: `https://example.com` and `key: value` are
// rejected because `/`, `.` and ` ` fall outside the post-colon class.
var scopeNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*:[a-z0-9,_-]+$`)

// scopeGuardScanRoot is the package tree T1 constrains, walked recursively
// so the `render` and `assistant_adapter` subpackages are covered too.
const scopeGuardScanRoot = "internal/telegram"

// scopeGuardFinding is one offending literal, located for the failure
// message. Reporting file:line makes a failure directly actionable rather
// than sending the reader back to grep.
type scopeGuardFinding struct {
	RelPath string
	Line    int
	Literal string
	Reason  string
}

func (f scopeGuardFinding) String() string {
	return f.RelPath + ":" + strconv.Itoa(f.Line) + ": " +
		strconv.Quote(f.Literal) + " — " + f.Reason
}

// scopeGuardForbiddenVocabulary is Layer B's forbidden set: the literal
// spelling of every grant the bridge may carry.
//
// Sourced from the ceiling rather than re-typed. If a grant is added to
// `auth.TelegramBridgeDelegableGrants`, its literal form becomes forbidden
// in this package's tests on the next run, with no edit here.
func scopeGuardForbiddenVocabulary() []string {
	vocabulary := slices.Clone(auth.TelegramBridgeDelegableGrants)
	if !slices.Contains(vocabulary, auth.GrantAnnotationEdit) {
		vocabulary = append(vocabulary, auth.GrantAnnotationEdit)
	}
	return vocabulary
}

// scopeGuardRepoRoot climbs from this file to the repo root by looking for
// config/smackerel.yaml, mirroring internal/auth/sst_grep_guard_test.go so
// the walk is independent of `go test` CWD.
func scopeGuardRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", thisFile)
	return ""
}

// scopeGuardStringLiterals returns every string literal in a Go file with
// its line number.
//
// This parses to an AST instead of scanning lines. Two reasons, both about
// precision rather than elegance:
//
//   - Comments are dropped by construction. The precedent guard needed a
//     "skip comment-only lines" hack plus a sub-test to prove the hack
//     worked; a prose sentence that happens to mention a grant cannot reach
//     the matcher here at all.
//   - `ScopeNameRegex` is anchored, so it is a claim about a COMPLETE
//     string. Only the AST yields complete, unquoted literal values —
//     a line scanner sees fragments and would have to re-derive the
//     boundaries with a second regex.
//
// Raw (backtick) strings are covered: `strconv.Unquote` handles both forms.
// Struct tags are `BasicLit` strings too, but their values embed quotes
// (`json:"user_id"`), which the post-colon character class excludes.
func scopeGuardStringLiterals(absPath string) ([]scopeGuardFinding, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absPath, nil, 0)
	if err != nil {
		return nil, err
	}
	literals := make([]scopeGuardFinding, 0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			return true
		}
		literals = append(literals, scopeGuardFinding{
			Line:    fset.Position(lit.Pos()).Line,
			Literal: value,
		})
		return true
	})
	return literals, nil
}

// scopeGuardScan walks a directory tree and applies Layer A to non-test Go
// files and Layer B to `_test.go` files.
//
// Exported as a helper (rather than inlined into the primary test) so the
// anti-vacuity sub-tests can point it at a planted fixture and prove it
// actually reports.
func scopeGuardScan(rootDir, scanPath string) ([]scopeGuardFinding, error) {
	findings := make([]scopeGuardFinding, 0)
	vocabulary := scopeGuardForbiddenVocabulary()
	walkRoot := filepath.Join(rootDir, scanPath)

	if _, err := os.Stat(walkRoot); err != nil {
		if os.IsNotExist(err) {
			return findings, nil
		}
		return nil, err
	}

	err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		literals, parseErr := scopeGuardStringLiterals(path)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		isTest := strings.HasSuffix(path, "_test.go")

		for _, lit := range literals {
			var reason string
			switch {
			case isTest && slices.Contains(vocabulary, lit.Literal):
				// Layer B — a grant literal in a test fixture is the
				// draft of the next production regression.
				reason = "grant literal in a test fixture; reference auth.TelegramBridgeDelegableGrants symbolically instead"
			case !isTest && scopeNameRegex.MatchString(lit.Literal):
				// Layer A — scope-shaped literal on the compiled surface.
				reason = "scope-shaped literal on the compiled surface; authority must be derived from the principal's recorded grants, never named at the minter"
			default:
				continue
			}
			findings = append(findings, scopeGuardFinding{
				RelPath: rel,
				Line:    lit.Line,
				Literal: lit.Literal,
				Reason:  reason,
			})
		}
		return nil
	})
	return findings, err
}

func formatScopeGuardFindings(findings []scopeGuardFinding) string {
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		lines = append(lines, f.String())
	}
	return strings.Join(lines, "\n  ")
}

// TestScopeLiteralGuard_NoScopeLiteralsInTelegramPackage is the T1 guard.
//
// Failure means authority has drifted back to the minter: some file under
// internal/telegram/** now names a scope, which is the shortcut spec.md §18
// decision 3 permanently closed.
func TestScopeLiteralGuard_NoScopeLiteralsInTelegramPackage(t *testing.T) {
	root := scopeGuardRepoRoot(t)

	findings, err := scopeGuardScan(root, scopeGuardScanRoot)
	if err != nil {
		t.Fatalf("scan %s failed: %v", scopeGuardScanRoot, err)
	}
	if len(findings) > 0 {
		t.Fatalf(`T1 structural guard violation (spec 108 design.md §10.10).

%s/** must contain no scope literal. Authority for a bridged principal is
DERIVED from that principal's recorded grants — a scope named here relocates
authority to the minter, which is the shortcut spec.md §18 decision 3 closed.

Use auth.TelegramBridgeDelegableGrants (or auth.GrantAnnotationEdit) instead
of a literal.

Findings:
  %s`, scopeGuardScanRoot, formatScopeGuardFindings(findings))
	}
	t.Logf("T1 guard OK: no scope literal under %s/** (shape %q enforced on the compiled surface; grant vocabulary %v additionally forbidden in _test.go)",
		scopeGuardScanRoot, scopeNameRegex.String(), scopeGuardForbiddenVocabulary())
}

// TestScopeLiteralGuard_MatcherIsNotVacuous pins the detector itself.
//
// A guard is only as good as its matcher: a regex that matches nothing
// would make the primary test pass forever while catching nothing. This
// asserts both arms — real scope names match, and the near-miss strings a
// naive unanchored regex would wrongly claim do not.
func TestScopeLiteralGuard_MatcherIsNotVacuous(t *testing.T) {
	// Built from the constants so this file holds no grant literal of its
	// own — the same discipline the guard imposes on the package.
	cases := []struct {
		name  string
		input string
		want  bool
		why   string
	}{
		{
			name:  "real delegable grant (annotation)",
			input: auth.GrantAnnotationEdit,
			want:  true,
			why:   "the literal removed from per_user_token.go:201 — the guard is worthless if this is not caught",
		},
		{
			name:  "real delegable grant (corpus)",
			input: auth.TelegramBridgeDelegableGrants[0],
			want:  true,
			why:   "the grant whose absence SCN-108-E04 turns into a 403",
		},
		{
			name:  "scope shape with comma-separated value",
			input: "corpus:read,write",
			want:  true,
			why:   "the character class admits commas; a multi-value claim is still a scope",
		},
		{
			name:  "scope shape with hyphenated surface",
			input: "hospitality-read:all",
			want:  true,
			why:   "surface names may contain hyphens (spec 109 vocabulary)",
		},
		{
			name:  "url",
			input: "https://example.com",
			want:  false,
			why:   "slashes and dots are outside the post-colon class; flagging URLs would make the guard unusable",
		},
		{
			name:  "yaml-ish key/value with a space",
			input: "key: value",
			want:  false,
			why:   "a space is outside the post-colon class",
		},
		{
			name:  "capitalised prefix",
			input: "Corpus:read",
			want:  false,
			why:   "the shape is lowercase-anchored",
		},
		{
			name:  "no colon at all",
			input: "annotation",
			want:  false,
			why:   "a bare word is not a scope name",
		},
		{
			name:  "empty value after colon",
			input: "corpus:",
			want:  false,
			why:   "the post-colon class requires at least one character",
		},
		{
			name:  "substring of a longer sentence",
			input: "the corpus:read grant is required",
			want:  false,
			why:   "the regex is anchored — it describes a whole literal, not a fragment",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeNameRegex.MatchString(tc.input); got != tc.want {
				t.Errorf("scopeNameRegex.MatchString(%q) = %v, want %v — %s",
					tc.input, got, tc.want, tc.why)
			}
		})
	}
}

// TestScopeLiteralGuard_ShapeScanIsNotVacuous proves Layer A reports.
//
// The matcher test above proves the regex works in isolation; this proves
// the walk, the AST extraction, and the non-test classification actually
// deliver a literal to it. Both are needed: a correct regex behind a broken
// walker is still a guard that can never fail.
func TestScopeLiteralGuard_ShapeScanIsNotVacuous(t *testing.T) {
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, scopeGuardScanRoot)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture package: %v", err)
	}

	// The regression this guard exists to catch, reconstructed from the
	// constant so no grant literal is typed into this file.
	regression := "package telegram\n\n" +
		"var mintedScopes = []string{" + strconv.Quote(auth.GrantAnnotationEdit) + "}\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "per_user_token.go"), []byte(regression), 0o600); err != nil {
		t.Fatalf("write regression fixture: %v", err)
	}

	// A comment mentioning the grant must NOT be flagged — parsing to an
	// AST is what makes that true by construction rather than by filter.
	narrative := "package telegram\n\n" +
		"// The bridge once hardcoded " + auth.GrantAnnotationEdit + " here.\n" +
		"// design.md §10.10 explains why it no longer may.\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "narrative.go"), []byte(narrative), 0o600); err != nil {
		t.Fatalf("write narrative fixture: %v", err)
	}

	findings, err := scopeGuardScan(tmp, scopeGuardScanRoot)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("Layer A is vacuous or over-broad: expected exactly 1 finding (the planted literal in per_user_token.go), got %d:\n  %s",
			len(findings), formatScopeGuardFindings(findings))
	}
	if !strings.HasSuffix(findings[0].RelPath, "per_user_token.go") {
		t.Errorf("finding attributed to %q, want the planted per_user_token.go", findings[0].RelPath)
	}
	if findings[0].Literal != auth.GrantAnnotationEdit {
		t.Errorf("reported literal = %q, want the planted grant", findings[0].Literal)
	}
	t.Logf("Layer A OK: planted literal reported as %s; the comment-only fixture was correctly ignored", findings[0])
}

// TestScopeLiteralGuard_VocabularyScanIsNotVacuous proves Layer B reports,
// and that the `_test.go` carve-out is a narrowing rather than an exemption.
//
// Both arms matter. If the grant literal were NOT flagged, tests would be
// the hiding place the two-layer split exists to deny. If the model tag WERE
// flagged, the guard would be unrunnable against this package's real tests
// and would end up disabled.
func TestScopeLiteralGuard_VocabularyScanIsNotVacuous(t *testing.T) {
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, scopeGuardScanRoot)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture package: %v", err)
	}

	const modelTag = "deepseek-r1:7b"
	// Sanity: the exemption is only meaningful if the model tag really does
	// share the scope shape. If this ever stops being true, the two-layer
	// split is unnecessary complexity and should be collapsed.
	if !scopeNameRegex.MatchString(modelTag) {
		t.Fatalf("premise broken: %q no longer matches the scope shape, so Layer A could be applied to _test.go directly and this split should be removed", modelTag)
	}

	fixture := "package telegram\n\n" +
		"var forbidden = " + strconv.Quote(auth.GrantAnnotationEdit) + "\n" +
		"var allowed = " + strconv.Quote(modelTag) + "\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "fixture_test.go"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("write test-shaped fixture: %v", err)
	}

	findings, err := scopeGuardScan(tmp, scopeGuardScanRoot)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("Layer B is vacuous or over-broad: expected exactly 1 finding (the grant literal, NOT the model tag), got %d:\n  %s",
			len(findings), formatScopeGuardFindings(findings))
	}
	if findings[0].Literal != auth.GrantAnnotationEdit {
		t.Errorf("reported literal = %q, want the planted grant — the model tag must not be flagged and the grant must be",
			findings[0].Literal)
	}
	t.Logf("Layer B OK: grant literal in a _test.go reported as %s; model tag %q correctly ignored", findings[0], modelTag)
}
