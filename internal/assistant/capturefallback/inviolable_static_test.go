// Spec 074 SCOPE-1 TP-074-02 — inviolability static guard.
//
// SCN-074-A09 — no SST key, env var, source path, or Policy.Decide
// branch may suppress fallback capture for an eligible turn.
//
// This test enforces SCN-074-A09 mechanically in two layers:
//
//  1. Codebase scan: assert no reference to a `disable_capture_as_fallback`
//     identifier (case-insensitive, hyphen/underscore/dot variants)
//     exists in production source (internal/, cmd/, ml/app/, scripts/,
//     config/, docker-compose*.yml, Dockerfile). Tests, docs, specs,
//     and this guard file itself are excluded.
//
//  2. Policy.Decide contract: enumerate every Cause in AllCauses and
//     assert that Decide returns nil error + ProvenanceFallback for
//     every well-formed Request. There is no suppression return path
//     in Decision (no Suppress, Skip, Disabled, or DropCapture field),
//     enforced structurally by reflecting on the Decision type.
//
// The adversarial sub-test feeds the scanner a temporary fixture that
// contains the forbidden literal and asserts the scanner would have
// rejected it — proves the guard is not vacuously passing.
package capturefallback

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// forbiddenSuppressionTokens is the closed set of identifier shapes
// that would represent a suppression switch. Match is case-insensitive
// and substring-based across the file body. Adding a new shape here
// also requires a spec amendment.
var forbiddenSuppressionTokens = []string{
	"disable_capture_as_fallback",
	"disable-capture-as-fallback",
	"DisableCaptureAsFallback",
	"capture_as_fallback.disabled",
	"capture_as_fallback_disabled",
	"capture_as_fallback.enabled",  // an enabled flag is a disable switch in disguise
	"CAPTURE_AS_FALLBACK_DISABLED", // env-var spelling
	"CAPTURE_AS_FALLBACK_ENABLED",
}

// forbiddenSuppressionDecisionFields names fields on Decision whose
// presence would constitute a suppression return path. Reflecting on
// Decision instead of grepping keeps the guard truthful even after
// renames.
var forbiddenSuppressionDecisionFields = []string{
	"suppress",
	"skip",
	"disabled",
	"dropcapture",
	"nocapture",
}

// inviolableGuardRoots matches the production source tree the spec
// 074 scope contract owns. Test files (*_test.go), specs/, docs/,
// bubbles/, .github/, and this guard file itself are excluded inside
// the walker.
var inviolableGuardRoots = []string{
	"../../../internal",
	"../../../cmd",
	"../../../ml/app",
	"../../../scripts",
	"../../../config",
}

// inviolableGuardRootFiles are individual files at repo root the
// scanner inspects (Docker / compose surfaces).
var inviolableGuardRootFiles = []string{
	"../../../docker-compose.yml",
	"../../../docker-compose.prod.yml",
	"../../../Dockerfile",
}

func TestInviolableGuard_NoSuppressionTokenInProductionSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path layout assumes POSIX repo root; harmless to skip on windows runners")
	}
	hits := scanForSuppressionTokens(t)
	if len(hits) > 0 {
		for _, h := range hits {
			t.Errorf("inviolability regression: %s contains forbidden suppression token %q (SCN-074-A09)", h.path, h.token)
		}
	}
}

// TestInviolableGuard_AdversarialFixtureIsRejected proves the scanner
// would catch a regression. A temp file under a guarded root containing
// a forbidden token MUST be flagged.
func TestInviolableGuard_AdversarialFixtureIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path layout assumes POSIX repo root; harmless to skip on windows runners")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(bad, []byte("package x\nvar disable_capture_as_fallback = false\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	hits := scanRootsForSuppression([]string{dir}, nil, t)
	if len(hits) == 0 {
		t.Fatalf("scanner failed to flag adversarial fixture; the production scan would also be vacuously passing")
	}
	found := false
	for _, h := range hits {
		if strings.HasSuffix(h.path, "fixture.go") && h.token == "disable_capture_as_fallback" {
			found = true
		}
	}
	if !found {
		t.Errorf("scanner produced hits but missed the forbidden token in the adversarial fixture: %+v", hits)
	}
}

func TestPolicyDecide_HasNoSuppressionPathForEligibleCauses(t *testing.T) {
	cfg := Config{
		DedupWindow:         24 * time.Hour,
		NormalizationPolicy: NormalizationPolicyV1,
		DedupHashKey:        "test-key",
	}
	p, err := New(cfg, nil, nil)
	if err != nil {
		t.Fatalf("New(cfg) failed: %v", err)
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range AllCauses {
		req := Request{
			UserID:       "user-1",
			Transport:    "telegram",
			OriginalText: "hello there",
			Cause:        c,
			OccurredAt:   now,
		}
		dec, err := p.Decide(context.Background(), req)
		if err != nil {
			t.Errorf("Decide returned error for eligible cause %q: %v (SCN-074-A09: no suppression path)", c, err)
			continue
		}
		if dec.Provenance != ProvenanceFallback {
			t.Errorf("Decide for cause %q produced provenance %q, want %q", c, dec.Provenance, ProvenanceFallback)
		}
		if dec.Cause != c {
			t.Errorf("Decide for cause %q produced cause %q", c, dec.Cause)
		}
		if dec.NormalizedTextHash == "" {
			t.Errorf("Decide for cause %q produced empty NormalizedTextHash", c)
		}
	}
}

func TestDecision_HasNoSuppressionField(t *testing.T) {
	rt := reflect.TypeOf(Decision{})
	for i := 0; i < rt.NumField(); i++ {
		name := strings.ToLower(rt.Field(i).Name)
		for _, bad := range forbiddenSuppressionDecisionFields {
			if strings.Contains(name, bad) {
				t.Errorf("Decision field %q matches forbidden suppression vocabulary %q (SCN-074-A09)", rt.Field(i).Name, bad)
			}
		}
	}
}

// --- scanner helpers ---

type suppressionHit struct {
	path  string
	token string
}

func scanForSuppressionTokens(t *testing.T) []suppressionHit {
	t.Helper()
	roots := inviolableGuardRoots
	rootFiles := inviolableGuardRootFiles
	resolvedRoots := make([]string, 0, len(roots))
	for _, r := range roots {
		if info, err := os.Stat(r); err == nil && info.IsDir() {
			resolvedRoots = append(resolvedRoots, r)
		}
	}
	resolvedFiles := make([]string, 0, len(rootFiles))
	for _, f := range rootFiles {
		if info, err := os.Stat(f); err == nil && !info.IsDir() {
			resolvedFiles = append(resolvedFiles, f)
		}
	}
	return scanRootsForSuppression(resolvedRoots, resolvedFiles, t)
}

func scanRootsForSuppression(roots, rootFiles []string, t *testing.T) []suppressionHit {
	t.Helper()
	var hits []suppressionHit
	check := func(path string, body []byte) {
		// Per-line scan: skip lines whose first non-whitespace bytes
		// are a comment marker. The inviolability invariant is about
		// suppression code/keys actually executing; narrative lines
		// that NAME the forbidden token to declare its absence are
		// the SPEC, not a regression.
		lower := strings.ToLower(string(body))
		for _, line := range strings.Split(lower, "\n") {
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			for _, tok := range forbiddenSuppressionTokens {
				if strings.Contains(trimmed, strings.ToLower(tok)) {
					hits = append(hits, suppressionHit{path: path, token: tok})
				}
			}
		}
		_ = lower
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				base := d.Name()
				switch base {
				case "node_modules", ".git", ".venv", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if shouldSkipForSuppressionScan(path) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			check(path, body)
			return nil
		})
	}
	for _, f := range rootFiles {
		body, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		check(f, body)
	}
	return hits
}

func shouldSkipForSuppressionScan(path string) bool {
	base := filepath.Base(path)
	// This guard file itself names every forbidden token in literal
	// form — exclude it explicitly so the scan is meaningful.
	if base == "inviolable_static_test.go" {
		return true
	}
	if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_test.py") {
		return true
	}
	// Limit to source/text file extensions where these literals would
	// be runtime-meaningful.
	switch filepath.Ext(base) {
	case ".go", ".py", ".sh", ".yml", ".yaml", ".json":
		return false
	default:
		// Unrecognized extensions (binaries, lockfiles, etc.) are
		// skipped — runtime suppression cannot live there.
		if base == "Dockerfile" {
			return false
		}
		return true
	}
}
