// Package webcodegen_drift_test also owns spec 073 Scope 1c
// TP-073-06: a static guard that scans web/pwa/assistant*.js and
// the entire web/pwa/generated/ directory for forbidden browser
// storage APIs tied to bearer or session material. The guard runs
// under `./smackerel.sh test unit`.
//
// Scope 073 SCN-073-A11: the web assistant client MUST NOT persist
// bearer or session material in localStorage, sessionStorage,
// IndexedDB, or the service-worker cache. A future Scope 2 commit
// will add web/pwa/assistant*.js; this guard already covers that
// glob plus the Scope 1c generated artifacts.
package webcodegen_drift_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbiddenStoragePatterns enumerates the storage-API surface that
// MUST NOT be reached from any committed web assistant source.
// The patterns are intentionally over-inclusive: any reference is a
// failure. Tests that need to demonstrate a violation construct an
// in-memory string, never a real file under the guarded globs.
var forbiddenStoragePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\blocalStorage\b`),
	regexp.MustCompile(`\bsessionStorage\b`),
	regexp.MustCompile(`\bindexedDB\b`),
	regexp.MustCompile(`\bIDBFactory\b`),
	regexp.MustCompile(`\bcaches\s*\.\s*open\b`),
	regexp.MustCompile(`\bcaches\s*\.\s*match\b`),
	regexp.MustCompile(`\bCacheStorage\b`),
}

// guardedPaths returns the concrete files this guard scans.
// Discovery is dynamic: future scope 2 assistant*.js files are
// covered automatically the moment they land.
func guardedPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	// web/pwa/assistant*.js — top-level assistant code, when added.
	matches, err := filepath.Glob(filepath.Join(root, "web", "pwa", "assistant*.js"))
	if err != nil {
		t.Fatalf("glob assistant*.js: %v", err)
	}
	paths = append(paths, matches...)
	// Spec 073 SCOPE-073-05 TP-073-30 — extend coverage to the
	// knowledge-graph wiki browse surface (web/pwa/wiki*.js).
	wikiMatches, err := filepath.Glob(filepath.Join(root, "web", "pwa", "wiki*.js"))
	if err != nil {
		t.Fatalf("glob wiki*.js: %v", err)
	}
	paths = append(paths, wikiMatches...)
	// web/pwa/generated/ — Scope 1c committed artifacts.
	genDir := filepath.Join(root, "web", "pwa", "generated")
	if err := filepath.Walk(genDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		t.Fatalf("walk web/pwa/generated: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("storage guard: no files matched the guarded globs; generated/ must exist after Scope 1c")
	}
	return paths
}

func TestWebAssistantStorageGuard_TP_073_06(t *testing.T) {
	root := repoRoot(t)
	paths := guardedPaths(t, root)
	var hits []string
	for _, p := range paths {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		// Strip line-comments so a comment that names an API for
		// documentation doesn't trip the guard. Generated files use
		// `//` headers; production code may too.
		stripped := stripLineComments(string(body))
		for _, pat := range forbiddenStoragePatterns {
			if loc := pat.FindStringIndex(stripped); loc != nil {
				rel, _ := filepath.Rel(root, p)
				hits = append(hits, rel+": matched "+pat.String())
			}
		}
	}
	if len(hits) > 0 {
		t.Fatalf("forbidden browser-storage API used in web assistant surface (SCN-073-A11):\n  %s", strings.Join(hits, "\n  "))
	}
}

// TestWebAssistantStorageGuard_Adversarial_TP_073_06 proves the
// guard would catch a real violation. We build a synthetic payload
// containing each forbidden pattern and confirm every pattern
// matches it. Without this, the guard could be silently broken.
func TestWebAssistantStorageGuard_Adversarial_TP_073_06(t *testing.T) {
	adversarial := `
		const token = localStorage.getItem("bearer");
		sessionStorage.setItem("session", token);
		const db = indexedDB.open("auth");
		const factory = self.IDBFactory;
		caches.open("v1").then(c => c.put("/auth", token));
		caches.match("/auth");
		const cs = self.CacheStorage;
	`
	for _, pat := range forbiddenStoragePatterns {
		if !pat.MatchString(adversarial) {
			t.Fatalf("storage guard pattern %s failed to match adversarial sample — the guard would not detect a real violation", pat.String())
		}
	}

	// Prove comment-stripping does not mask a real call.
	masked := "// localStorage is forbidden\n const x = localStorage.getItem('a');"
	stripped := stripLineComments(masked)
	if !regexp.MustCompile(`\blocalStorage\b`).MatchString(stripped) {
		t.Fatal("comment stripping removed a real localStorage call — guard would miss violations")
	}
}

// stripLineComments removes `//` line comments so the guard does
// not flag the literal pattern names that appear in this very
// file's own header comments inside the generated artifacts.
func stripLineComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			b.WriteString(line[:idx])
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}
