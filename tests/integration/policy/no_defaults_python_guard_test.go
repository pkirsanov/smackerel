//go:build integration

// Spec 067 Scope 4 — SCN-067-A05 Python NO-DEFAULTS guard tests.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPythonNoDefaultsGuard_RealCorpusIsClean — the real ml/app/
// tree MUST produce zero G067-A05 findings. This is the proactive
// regression that prevents a silent default from creeping into the
// Python sidecar.
func TestPythonNoDefaultsGuard_RealCorpusIsClean(t *testing.T) {
	repo := repoRootForTest(t)
	baselinePath := filepath.Join(string(repo), "policy-exception-baseline.json")
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: 365 * 10}
	vs, err := PythonNoDefaultsGuard(repo, baseline, time.Now(), cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard: %v", err)
	}
	if len(vs) != 0 {
		var msgs []string
		for _, v := range vs {
			msgs = append(msgs, v.Path+":"+v.Detail)
		}
		t.Fatalf("real ml/app/ produced %d G067-A05 findings:\n%s",
			len(vs), strings.Join(msgs, "\n"))
	}
}

// TestNoDefaultsPythonGuardReportsRuntimeFallbackWithPolicySource —
// SCN-067-A05.
//
// Adversarial fixture under a temp ml/app/ tree contains
// os.environ.get("EMBEDDING_MODEL", "all-MiniLM-L6-v2"). The guard
// MUST report a G067-A05 violation that names the file:line, the
// SST key, and the NO-DEFAULTS policy source. An adversarial
// baseline using an empty-string fallback MUST NOT be flagged
// (the fail-loud delegation pattern).
func TestNoDefaultsPythonGuardReportsRuntimeFallbackWithPolicySource(t *testing.T) {
	dir := t.TempDir()
	mlDir := filepath.Join(dir, "ml", "app")
	if err := os.MkdirAll(mlDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(mlDir, "main.py")
	bad := `import os

def load():
    return os.environ.get("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
`
	if err := os.WriteFile(fixture, []byte(bad), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 180}
	vs, err := PythonNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
	v := vs[0]
	if v.RuleID != "G067-A05" {
		t.Fatalf("RuleID = %q, want G067-A05", v.RuleID)
	}
	if !strings.HasSuffix(v.Path, "ml/app/main.py") {
		t.Fatalf("Path = %q, want suffix ml/app/main.py", v.Path)
	}
	if v.Line != 4 {
		t.Fatalf("Line = %d, want 4", v.Line)
	}
	if !strings.Contains(v.Detail, "EMBEDDING_MODEL") {
		t.Fatalf("Detail must name the SST key: %q", v.Detail)
	}
	if v.PolicySource != ".github/instructions/smackerel-no-defaults.instructions.md" {
		t.Fatalf("PolicySource = %q, want smackerel-no-defaults policy doc", v.PolicySource)
	}
	if v.Resolution == "" {
		t.Fatalf("Resolution must be set")
	}

	// Adversarial baseline 1: empty-string fallback is the
	// fail-loud delegation pattern (caller decides) and MUST NOT
	// be flagged.
	good := `import os

def load():
    return os.environ.get("EMBEDDING_MODEL", "")
`
	if err := os.WriteFile(fixture, []byte(good), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	clean, err := PythonNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard (clean): %v", err)
	}
	if len(clean) != 0 {
		t.Fatalf("empty-string fallback flagged %d violations: %+v", len(clean), clean)
	}

	// Adversarial baseline 2: comment-only line that quotes the
	// forbidden form (as docs do) MUST NOT be flagged.
	docsComment := `import os
# Forbidden form example: os.getenv("KEY", "default")
`
	if err := os.WriteFile(fixture, []byte(docsComment), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	cleanDocs, err := PythonNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard (docs comment): %v", err)
	}
	if len(cleanDocs) != 0 {
		t.Fatalf("comment-only docs line flagged %d violations: %+v", len(cleanDocs), cleanDocs)
	}
}

// TestNoDefaultsPythonGuardAllowsStructuredExpiringException —
// SCN-067-A07 cross-cutting: a Python fallback preceded by a
// well-formed source annotation AND present in the baseline MUST
// NOT be flagged. An annotation present without a baseline entry
// MUST be flagged with G067-A07.
func TestNoDefaultsPythonGuardAllowsStructuredExpiringException(t *testing.T) {
	dir := t.TempDir()
	mlDir := filepath.Join(dir, "ml", "app")
	if err := os.MkdirAll(mlDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(mlDir, "embedder.py")
	body := `import os

# smackerel:policy-exception id=G067-A05-test-1 rule=G067-A05 owner=reviewer expires=2099-01-01 reason="migration window"
MODEL = os.environ.get("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 365 * 100}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Without the annotation in the baseline, G067-A07 fires.
	noBase, err := PythonNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, now, cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard: %v", err)
	}
	if len(noBase) != 1 || noBase[0].RuleID != "G067-A07" {
		t.Fatalf("missing baseline must produce G067-A07; got %+v", noBase)
	}

	// With the annotation in the baseline, no violation.
	baseline := &Baseline{
		SchemaVersion: "v1",
		Exceptions: []Exception{{
			ID: "G067-A05-test-1", RuleID: "G067-A05",
			Owner: "reviewer", Reason: "migration window",
			ExpiresOn: "2099-01-01",
		}},
	}
	vs, err := PythonNoDefaultsGuard(Root(dir), baseline, now, cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard (waived): %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("accepted exception still flagged: %+v", vs)
	}
}
