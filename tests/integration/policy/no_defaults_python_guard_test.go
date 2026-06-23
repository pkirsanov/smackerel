//go:build integration

// Spec 067 Scope 4 — SCN-067-A05 Python NO-DEFAULTS guard tests.

package policy

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	cfg := PolicyConfig{ExceptionMaxAgeDays: realPolicyExceptionMaxAgeDays(t)}
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

// realPolicyExceptionMaxAgeDays reads the SST cap
// (policy.policy_exception_max_age_days) directly from the committed
// config/smackerel.yaml so policy tests validate the REAL committed
// baseline at the REAL cap instead of an inflated magic constant.
//
// Spec 067 BUG-067-001 (GAP-067-G02): the prior real-baseline tests
// overrode ExceptionMaxAgeDays to 365*10 / 180, which masked an
// over-age committed exception. Anchoring to the single source of
// truth removes both the magic constant and the masking.
func realPolicyExceptionMaxAgeDays(t *testing.T) int {
	t.Helper()
	repo := repoRootForTest(t)
	yamlPath := filepath.Join(string(repo), "config", "smackerel.yaml")
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read SST config %s: %v", yamlPath, err)
	}
	re := regexp.MustCompile(`(?m)^\s*policy_exception_max_age_days:\s*([0-9]+)\s*$`)
	m := re.FindSubmatch(raw)
	if m == nil {
		t.Fatalf("policy.policy_exception_max_age_days not found in %s", yamlPath)
	}
	days, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("policy.policy_exception_max_age_days not an integer in %s: %v", yamlPath, err)
	}
	if days <= 0 {
		t.Fatalf("policy.policy_exception_max_age_days must be positive, got %d", days)
	}
	return days
}

// TestRealBaselineHasNoOverAgeExceptionsAtRealCap — Spec 067
// BUG-067-001 adversarial regression for GAP-067-G01/G02. Loads the
// REAL committed policy-exception-baseline.json and validates EVERY
// exception via the production ValidateException at the REAL SST cap
// (policy.policy_exception_max_age_days, read from config/smackerel.yaml
// by realPolicyExceptionMaxAgeDays).
//
// RED-if-reintroduced: the removed G067-A05-ml-log-level entry had
// expires_on=2026-12-01 (~162 days out at fix time), exceeding the
// 90-day cap, which ValidateException flags as G067-A07. If any
// over-age exception is re-added to the committed baseline, this test
// fails. The prior real-baseline tests could NOT catch it because they
// overrode the cap to 365*10 / 180 (the GAP-067-G02 masking).
func TestRealBaselineHasNoOverAgeExceptionsAtRealCap(t *testing.T) {
	repo := repoRootForTest(t)
	baselinePath := filepath.Join(string(repo), "policy-exception-baseline.json")
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: realPolicyExceptionMaxAgeDays(t)}
	now := time.Now()
	for _, e := range baseline.Exceptions {
		if v := ValidateException(e, now, cfg); v != nil {
			t.Errorf("committed baseline exception %q violates the real %d-day SST cap: %s — %s",
				e.ID, cfg.ExceptionMaxAgeDays, v.RuleID, v.Detail)
		}
	}
}

// TestValidateExceptionFlagsOverAgeAtRealCap — Spec 067 BUG-067-001.
// Proves TestRealBaselineHasNoOverAgeExceptionsAtRealCap is NOT
// tautological: at the REAL SST cap, a synthetic exception ~162 days
// out (the exact over-age shape of the removed G067-A05-ml-log-level
// entry) MUST be flagged G067-A07, and a synthetic exception within
// the cap (80 days) MUST NOT be flagged. If a future change widens the
// cap to mask over-age exceptions again (the GAP-067-G02 test hole),
// the over-age assertion below fails.
func TestValidateExceptionFlagsOverAgeAtRealCap(t *testing.T) {
	capDays := realPolicyExceptionMaxAgeDays(t)
	cfg := PolicyConfig{ExceptionMaxAgeDays: capDays}
	now := time.Now()

	overAge := Exception{
		ID: "FIXTURE-over-age-ml-log-level", RuleID: "G067-A05",
		Path: "ml/app/main.py", Owner: "ml-sidecar",
		Reason:    "adversarial fixture mirroring the removed over-age exception",
		ExpiresOn: now.AddDate(0, 0, 162).Format("2006-01-02"),
	}
	v := ValidateException(overAge, now, cfg)
	if v == nil {
		t.Fatalf("over-age exception (162 d) MUST be flagged at the real %d-day cap, got nil", capDays)
	}
	if v.RuleID != "G067-A07" {
		t.Fatalf("over-age exception RuleID = %q, want G067-A07", v.RuleID)
	}

	// No false positive within the cap.
	inRange := overAge
	inRange.ID = "FIXTURE-in-range"
	inRange.ExpiresOn = now.AddDate(0, 0, 80).Format("2006-01-02")
	if v := ValidateException(inRange, now, cfg); v != nil {
		t.Fatalf("in-range exception (80 d) must be clean at the real %d-day cap, got %s — %s",
			capDays, v.RuleID, v.Detail)
	}
}
