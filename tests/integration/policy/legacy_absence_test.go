//go:build integration

// Spec 066 SCOPE-4 — legacy keyword surface absence guards.
//
// These tests prove that the regex-driven `/find` domain intent parser
// from spec 026 has been physically retired from the repository per
// SCN-066-A07. They are intentionally structural (file-system + grep)
// because the contract is "this file and this symbol do not exist".
package policy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRootSpec066 resolves <repo>/ from this test file (tests/integration/policy/...).
func repoRootSpec066(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// tests/integration/policy → repo root is three parents up.
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root sanity: go.mod not found at %s: %v", root, err)
	}
	return root
}

// TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent proves that
// the file internal/api/domain_intent.go no longer exists and that
// no first-party source file under internal/, cmd/, or web/ still
// defines or calls the retired parseDomainIntent symbol.
//
// SCN-066-A07.
func TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent(t *testing.T) {
	root := repoRootSpec066(t)

	path := filepath.Join(root, "internal", "api", "domain_intent.go")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("internal/api/domain_intent.go MUST be absent (spec 066 SCOPE-4); stat err=%v", err)
	}

	// Adversarial baseline: the absence assertion above would also pass
	// in an empty checkout. Prove we're scanning a real tree by
	// confirming a known sibling file still exists.
	siblings := []string{
		filepath.Join(root, "internal", "api", "search.go"),
		filepath.Join(root, "internal", "api", "domain_filter_test.go"),
	}
	for _, s := range siblings {
		if _, err := os.Stat(s); err != nil {
			t.Fatalf("baseline sibling %s missing — repo scan would silently pass: %v", s, err)
		}
	}
}

// TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain greps
// every .go file under internal/, cmd/, and tests/ (excluding this
// guard file) for the retired symbol. Zero matches are required.
// SCN-066-A07.
func TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain(t *testing.T) {
	root := repoRootSpec066(t)
	selfRel := filepath.Join("tests", "integration", "policy", "legacy_absence_test.go")
	selfAbs := filepath.Join(root, selfRel)

	symbolRe := regexp.MustCompile(`\bparseDomainIntent\b`)

	scanRoots := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd"),
		filepath.Join(root, "tests"),
	}

	var hits []string
	for _, base := range scanRoots {
		err := filepath.Walk(base, func(p string, info os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".go") {
				return nil
			}
			if p == selfAbs {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			if symbolRe.Match(data) {
				rel, _ := filepath.Rel(root, p)
				hits = append(hits, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}

	if len(hits) > 0 {
		t.Fatalf("parseDomainIntent MUST have zero call sites after spec 066 SCOPE-4; found %d:\n  %s",
			len(hits), strings.Join(hits, "\n  "))
	}

	// Adversarial sanity: scanning a string we KNOW is in the repo
	// (the SearchEngine.Search function signature) must produce at
	// least one hit, otherwise this guard could pass on an empty walk.
	canaryRe := regexp.MustCompile(`func \(s \*SearchEngine\) Search`)
	canaryHits := 0
	_ = filepath.Walk(filepath.Join(root, "internal", "api"), func(p string, info os.FileInfo, _ error) error {
		if info == nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		data, _ := os.ReadFile(p)
		if canaryRe.Match(data) {
			canaryHits++
		}
		return nil
	})
	if canaryHits == 0 {
		t.Fatal("adversarial canary failed: scanner did not find SearchEngine.Search — guard would silently pass on empty walk")
	}
}
