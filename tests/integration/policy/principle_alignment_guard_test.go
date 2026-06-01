//go:build integration

// Spec 067 Scope 2 — SCN-067-A01 (principle alignment guard).
//
// Three tests anchor the contract:
//
//   1. Real corpus baseline: every scenario YAML under
//      config/prompt_contracts/ MUST declare a principleAlignment
//      block listing IDs present in docs/Product-Principles.md. If
//      this fails, the guard names the offending scenario and the
//      policy source verbatim — that is the contract Scopes 3/4 and
//      CI consumers rely on.
//
//   2. TestPrincipleAlignmentGuardReportsMissingBlockWithPolicySource
//      Adversarial fixture with the block stripped MUST produce a
//      G067-A01 violation naming the missing block AND the
//      docs/Product-Principles.md catalog path.
//
//   3. TestPrincipleAlignmentGuardRejectsUnknownPrincipleID
//      Adversarial fixture citing `Principle 99` MUST produce a
//      G067-A01 violation naming the unknown ID.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRootForTest(t *testing.T) Root {
	t.Helper()
	// tests/integration/policy/ → repo root is three levels up.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return Root(filepath.Clean(filepath.Join(wd, "..", "..", "..")))
}

func parseAll(t *testing.T, paths []string) []ScenarioFile {
	t.Helper()
	var out []ScenarioFile
	for _, p := range paths {
		sf, err := ParseScenarioYAML(p)
		if err != nil {
			t.Fatalf("ParseScenarioYAML(%s): %v", p, err)
		}
		out = append(out, sf)
	}
	return out
}

// TestPrincipleAlignmentGuardReportsMissingBlockWithPolicySource —
// SCN-067-A01. Strips the principleAlignment block from a temp
// fixture YAML and asserts the resulting violation names the
// missing block AND the product-principles catalog path.
func TestPrincipleAlignmentGuardReportsMissingBlockWithPolicySource(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "config", "prompt_contracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(contracts, "missing-alignment-v1.yaml")
	body := `id: missing_alignment_fixture
version: missing-alignment-v1
system_prompt: |
  one line prompt
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	paths, err := ListScenarioYAMLs(Root(dir))
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	files := parseAll(t, paths)

	validIDs := map[string]struct{}{"Principle 1": {}, "P1": {}}
	v := PrincipleAlignmentGuard(files, validIDs)
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(v), v)
	}
	if v[0].RuleID != "G067-A01" {
		t.Fatalf("RuleID = %q, want G067-A01", v[0].RuleID)
	}
	if !strings.Contains(v[0].Detail, "missing_alignment_fixture") {
		t.Fatalf("Detail %q must name the scenario id", v[0].Detail)
	}
	if !strings.Contains(v[0].Detail, "docs/Product-Principles.md") {
		t.Fatalf("Detail %q must reference docs/Product-Principles.md", v[0].Detail)
	}
	if v[0].PolicySource != "specs/067-intent-driven-policy-enforcement/spec.md" {
		t.Fatalf("PolicySource = %q, want spec 067 spec.md", v[0].PolicySource)
	}

	// Adversarial baseline: same fixture WITH the block must NOT
	// produce a violation.
	bodyOK := body + "principleAlignment:\n  - Principle 1\n"
	if err := os.WriteFile(fixture, []byte(bodyOK), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	files = parseAll(t, paths)
	if vClean := PrincipleAlignmentGuard(files, validIDs); len(vClean) != 0 {
		t.Fatalf("clean fixture flagged %d: %+v", len(vClean), vClean)
	}
}

// TestPrincipleAlignmentGuardRejectsUnknownPrincipleID — SCN-067-A01
// adversarial vocabulary case. A scenario citing a principle ID not
// present in docs/Product-Principles.md MUST be flagged with
// G067-A01 naming the offending ID.
func TestPrincipleAlignmentGuardRejectsUnknownPrincipleID(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "config", "prompt_contracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(contracts, "unknown-principle-v1.yaml")
	body := `id: unknown_principle_fixture
version: unknown-principle-v1
principleAlignment:
  - Principle 99
system_prompt: |
  one line
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	paths, err := ListScenarioYAMLs(Root(dir))
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	files := parseAll(t, paths)
	validIDs := map[string]struct{}{"Principle 1": {}, "P1": {}}
	v := PrincipleAlignmentGuard(files, validIDs)
	if len(v) != 1 || v[0].RuleID != "G067-A01" {
		t.Fatalf("expected one G067-A01 violation, got %+v", v)
	}
	if !strings.Contains(v[0].Detail, "Principle 99") {
		t.Fatalf("Detail %q must name the unknown principle id", v[0].Detail)
	}
}

// TestPrincipleAlignmentGuardRealCorpusIsClean is the real-corpus
// canary. Run against the live config/prompt_contracts/ tree, no
// scenario YAML may be missing principleAlignment or cite an
// unknown principle. Without this, the previous two tests prove
// only the scanner mechanics; this test pins the lived guarantee
// the spec promises.
func TestPrincipleAlignmentGuardRealCorpusIsClean(t *testing.T) {
	repo := repoRootForTest(t)
	paths, err := ListScenarioYAMLs(repo)
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no scenario YAMLs found under %s", repo)
	}
	files := parseAll(t, paths)
	validIDs, err := LoadProductPrincipleIDs(filepath.Join(string(repo), "docs", "Product-Principles.md"))
	if err != nil {
		t.Fatalf("LoadProductPrincipleIDs: %v", err)
	}
	v := PrincipleAlignmentGuard(files, validIDs)
	if len(v) != 0 {
		var b strings.Builder
		for _, vv := range v {
			b.WriteString(vv.Path + ": " + vv.Detail + "\n")
		}
		t.Fatalf("real-corpus principleAlignment violations:\n%s", b.String())
	}
}
