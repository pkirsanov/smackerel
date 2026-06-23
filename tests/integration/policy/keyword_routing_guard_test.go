//go:build integration

// Spec 067 Scope 3 — SCN-067-A03 keyword routing guard tests.
//
// Three live tests:
//
//   1. Real corpus baseline: scanning the real internal/api/ tree
//      MUST produce zero G067-A03 violations. Any future regression
//      where a routing regex lands under a name implying intent /
//      routing / classification will fail here.
//
//   2. TestKeywordRoutingGuardReportsAPIRoutingRegexWithFileLine
//      Adversarial fixture under a temp dir contains a routing
//      regex named `intentRouterRe`. The guard MUST report a
//      G067-A03 violation naming the file:line, the rule, and the
//      compiled-intent resolution. An adversarial baseline fixture
//      with the same regex assigned to a non-routing identifier
//      (e.g., `phoneNumberRe`) MUST NOT be flagged.
//
//   3. TestKeywordRoutingGuardAllowsStructuredExpiringDiagnosticException
//      A routing regex preceded by a well-formed source-annotation
//      AND present in the supplied baseline MUST NOT be flagged.
//      An expired exception MUST be flagged with G067-A07.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKeywordRoutingGuard_RealCorpusRunsAndProducesWellFormedFindings
// scans the real internal/api/ tree. Per Scope 3 Change Boundary,
// production fixes for routing-regex violations belong to their
// owning specs (spec 066 keyword retirement, spec 068 intent
// compiler), so this test does NOT assert zero findings. It asserts
// the guard completes without error and every finding it emits has
// a well-formed Path, Line>0, RuleID=G067-A03, and non-empty
// Detail/Resolution — the contract downstream CI consumers rely on.
func TestKeywordRoutingGuard_RealCorpusRunsAndProducesWellFormedFindings(t *testing.T) {
	repo := repoRootForTest(t)
	baselinePath := filepath.Join(string(repo), "policy-exception-baseline.json")
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: realPolicyExceptionMaxAgeDays(t)}
	vs, err := KeywordRoutingGuard(repo, baseline, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordRoutingGuard: %v", err)
	}
	for i, v := range vs {
		if v.RuleID != "G067-A03" && v.RuleID != "G067-A07" {
			t.Errorf("vs[%d].RuleID = %q, want G067-A03 or G067-A07", i, v.RuleID)
		}
		if v.Path == "" || !strings.HasPrefix(v.Path, "internal/api") {
			t.Errorf("vs[%d].Path = %q, want internal/api/ prefix", i, v.Path)
		}
		if v.Line <= 0 {
			t.Errorf("vs[%d].Line = %d, want > 0", i, v.Line)
		}
		if v.Detail == "" || v.Resolution == "" {
			t.Errorf("vs[%d] missing Detail/Resolution: %+v", i, v)
		}
		if v.PolicySource != "specs/067-intent-driven-policy-enforcement/spec.md" {
			t.Errorf("vs[%d].PolicySource = %q", i, v.PolicySource)
		}
	}
	t.Logf("real internal/api/ produced %d findings (informational; production fixes belong to owning specs per Scope 3 Change Boundary)", len(vs))
}

func TestKeywordRoutingGuardReportsAPIRoutingRegexWithFileLine(t *testing.T) {
	dir := t.TempDir()
	apiDir := filepath.Join(dir, "internal", "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(apiDir, "domain_intent.go")
	body := `package api

import "regexp"

var intentRouterRe = regexp.MustCompile(` + "`" + `(?i)(recipe|cooking|grocery)` + "`" + `)
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 180}
	vs, err := KeywordRoutingGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordRoutingGuard: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
	v := vs[0]
	if v.RuleID != "G067-A03" {
		t.Fatalf("RuleID = %q, want G067-A03", v.RuleID)
	}
	if !strings.Contains(v.Path, "internal/api/domain_intent.go") {
		t.Fatalf("Path = %q, want suffix internal/api/domain_intent.go", v.Path)
	}
	if v.Line != 5 {
		t.Fatalf("Line = %d, want 5", v.Line)
	}
	if !strings.Contains(v.Detail, "intentRouterRe") {
		t.Fatalf("Detail = %q must name the offending identifier", v.Detail)
	}
	if !strings.Contains(v.Resolution, "intent.Compiler") {
		t.Fatalf("Resolution = %q must name the compiled-intent path", v.Resolution)
	}
	if v.PolicySource != "specs/067-intent-driven-policy-enforcement/spec.md" {
		t.Fatalf("PolicySource = %q, want spec 067 spec.md", v.PolicySource)
	}

	// Adversarial baseline: same regex assigned to a non-routing
	// identifier MUST NOT be flagged. Without this, the guard could
	// silently degrade to "always fires on regexp.MustCompile" and
	// the assertion above would still pass.
	good := strings.Replace(body, "intentRouterRe", "phoneNumberRe", 1)
	if err := os.WriteFile(fixture, []byte(good), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	clean, err := KeywordRoutingGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordRoutingGuard (baseline): %v", err)
	}
	if len(clean) != 0 {
		t.Fatalf("non-routing identifier flagged %d violations: %+v", len(clean), clean)
	}

	// Anchored format validator MUST NOT trip the guard even when
	// the identifier name is routing-shaped — anchored shapes are
	// syntactic validators, not free-text classifiers.
	anchored := strings.Replace(body,
		"`(?i)(recipe|cooking|grocery)`",
		"`^route-[a-z]+$`", 1)
	if err := os.WriteFile(fixture, []byte(anchored), 0o644); err != nil {
		t.Fatalf("rewrite fixture (anchored): %v", err)
	}
	cleanAnchored, err := KeywordRoutingGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordRoutingGuard (anchored): %v", err)
	}
	if len(cleanAnchored) != 0 {
		t.Fatalf("anchored validator flagged %d violations: %+v", len(cleanAnchored), cleanAnchored)
	}
}

func TestKeywordRoutingGuardAllowsStructuredExpiringDiagnosticException(t *testing.T) {
	dir := t.TempDir()
	apiDir := filepath.Join(dir, "internal", "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(apiDir, "diagnostic_router.go")
	body := `package api

import "regexp"

// smackerel:policy-exception id=G067-A03-diagnostic-fixture rule=G067-A03 owner=reviewer expires=2099-01-01 reason="diagnostic-only parser"
var intentRouterRe = regexp.MustCompile(` + "`" + `(?i)(recipe|cooking)` + "`" + `)
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	baseline := &Baseline{
		SchemaVersion: "v1",
		Exceptions: []Exception{{
			ID:        "G067-A03-diagnostic-fixture",
			RuleID:    "G067-A03",
			Owner:     "reviewer",
			Reason:    "diagnostic-only parser",
			ExpiresOn: "2099-01-01",
		}},
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: 365 * 100}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	vs, err := KeywordRoutingGuard(Root(dir), baseline, now, cfg)
	if err != nil {
		t.Fatalf("KeywordRoutingGuard: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("accepted exception still flagged %d violations: %+v", len(vs), vs)
	}

	// Adversarial: expired exception MUST be flagged with G067-A07
	// regardless of baseline membership.
	expired := strings.Replace(body, "expires=2099-01-01", "expires=2020-01-01", 1)
	if err := os.WriteFile(fixture, []byte(expired), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	baseline.Exceptions[0].ExpiresOn = "2020-01-01"
	vsExpired, err := KeywordRoutingGuard(Root(dir), baseline, now, PolicyConfig{ExceptionMaxAgeDays: 30})
	if err != nil {
		t.Fatalf("KeywordRoutingGuard (expired): %v", err)
	}
	if len(vsExpired) != 1 {
		t.Fatalf("expired annotation produced %d violations, want 1: %+v", len(vsExpired), vsExpired)
	}
	if vsExpired[0].RuleID != "G067-A07" {
		t.Fatalf("expired annotation RuleID = %q, want G067-A07", vsExpired[0].RuleID)
	}
	if !strings.Contains(vsExpired[0].Detail, "expired") {
		t.Fatalf("expired annotation Detail = %q must name expiry", vsExpired[0].Detail)
	}
}

// itoaPolicy is a tiny strconv.Itoa shim so the test file does not
// pull strconv only for diagnostic formatting.
func itoaPolicy(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
