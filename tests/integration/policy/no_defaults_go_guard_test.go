//go:build integration

// Spec 067 Scope 4 — SCN-067-A06 Go NO-DEFAULTS guard tests.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGoNoDefaultsGuard_RealCorpusIsClean — the real internal/ tree
// MUST produce zero G067-A06 findings. Safe fail-loud patterns
// (append to error slice in the empty branch) are not flagged; only
// literal-fallback assignments to the bound variable are.
func TestGoNoDefaultsGuard_RealCorpusIsClean(t *testing.T) {
	repo := repoRootForTest(t)
	baselinePath := filepath.Join(string(repo), "policy-exception-baseline.json")
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: 365 * 10}
	vs, err := GoNoDefaultsGuard(repo, baseline, time.Now(), cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard: %v", err)
	}
	if len(vs) != 0 {
		var msgs []string
		for _, v := range vs {
			msgs = append(msgs, v.Path+":"+v.Detail)
		}
		t.Fatalf("real internal/ produced %d G067-A06 findings:\n%s",
			len(vs), strings.Join(msgs, "\n"))
	}
}

// TestNoDefaultsGoGuardReportsLiteralFallbackAfterRuntimeRead —
// SCN-067-A06. Two-step pattern: v := os.Getenv("K"); if v == "" {
// v = "literal" }. MUST be reported with file:line, SST key, and
// fail-loud resolution. Safe baseline using append() in the empty
// branch MUST NOT be flagged.
func TestNoDefaultsGoGuardReportsLiteralFallbackAfterRuntimeRead(t *testing.T) {
	dir := t.TempDir()
	intDir := filepath.Join(dir, "internal", "loader")
	if err := os.MkdirAll(intDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(intDir, "loader.go")
	bad := `package loader

import "os"

func Load() string {
	v := os.Getenv("SMACKEREL_DB_HOST")
	if v == "" {
		v = "localhost"
	}
	return v
}
`
	if err := os.WriteFile(fixture, []byte(bad), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 180}
	vs, err := GoNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
	v := vs[0]
	if v.RuleID != "G067-A06" {
		t.Fatalf("RuleID = %q, want G067-A06", v.RuleID)
	}
	if !strings.HasSuffix(v.Path, "internal/loader/loader.go") {
		t.Fatalf("Path = %q", v.Path)
	}
	if v.Line != 6 {
		t.Fatalf("Line = %d, want 6 (the os.Getenv line)", v.Line)
	}
	if !strings.Contains(v.Detail, "SMACKEREL_DB_HOST") {
		t.Fatalf("Detail must name the SST key: %q", v.Detail)
	}
	if !strings.Contains(v.Detail, "localhost") {
		t.Fatalf("Detail must name the literal fallback: %q", v.Detail)
	}
	if v.PolicySource != ".github/instructions/smackerel-no-defaults.instructions.md" {
		t.Fatalf("PolicySource = %q", v.PolicySource)
	}
	if v.Resolution == "" {
		t.Fatalf("Resolution must be set")
	}

	// Safe baseline: append to error slice instead of literal
	// fallback. MUST NOT be flagged.
	good := `package loader

import "os"

func Load() ([]string, string) {
	var errs []string
	v := os.Getenv("SMACKEREL_DB_HOST")
	if v == "" {
		errs = append(errs, "SMACKEREL_DB_HOST")
	}
	return errs, v
}
`
	if err := os.WriteFile(fixture, []byte(good), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	clean, err := GoNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard (clean): %v", err)
	}
	if len(clean) != 0 {
		t.Fatalf("safe append-to-errors pattern flagged %d violations: %+v", len(clean), clean)
	}

	// Inline if-init form: also flagged.
	inline := `package loader

import "os"

func Load() string {
	cfg := ""
	if v := os.Getenv("SMACKEREL_DB_PORT"); v == "" {
		cfg = "5432"
	} else {
		cfg = v
	}
	return cfg
}
`
	if err := os.WriteFile(fixture, []byte(inline), 0o644); err != nil {
		t.Fatalf("rewrite fixture (inline): %v", err)
	}
	vsInline, err := GoNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard (inline): %v", err)
	}
	if len(vsInline) != 1 {
		t.Fatalf("inline if-init form: got %d violations, want 1: %+v", len(vsInline), vsInline)
	}
	if !strings.Contains(vsInline[0].Detail, "SMACKEREL_DB_PORT") {
		t.Fatalf("inline Detail must name SST key: %q", vsInline[0].Detail)
	}
}

// TestNoDefaultsGoGuardAllowsStructuredExpiringException —
// SCN-067-A07 cross-cutting for the Go scanner.
func TestNoDefaultsGoGuardAllowsStructuredExpiringException(t *testing.T) {
	dir := t.TempDir()
	intDir := filepath.Join(dir, "internal", "loader")
	if err := os.MkdirAll(intDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(intDir, "loader.go")
	body := `package loader

import "os"

func Load() string {
	// smackerel:policy-exception id=G067-A06-test-1 rule=G067-A06 owner=reviewer expires=2099-01-01 reason="migration window"
	v := os.Getenv("SMACKEREL_DB_HOST")
	if v == "" {
		v = "localhost"
	}
	return v
}
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 365 * 100}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	noBase, err := GoNoDefaultsGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, now, cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard: %v", err)
	}
	if len(noBase) != 1 || noBase[0].RuleID != "G067-A07" {
		t.Fatalf("missing baseline must produce G067-A07; got %+v", noBase)
	}

	baseline := &Baseline{
		SchemaVersion: "v1",
		Exceptions: []Exception{{
			ID: "G067-A06-test-1", RuleID: "G067-A06",
			Owner: "reviewer", Reason: "migration window",
			ExpiresOn: "2099-01-01",
		}},
	}
	vs, err := GoNoDefaultsGuard(Root(dir), baseline, now, cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard (waived): %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("accepted exception still flagged: %+v", vs)
	}
}
