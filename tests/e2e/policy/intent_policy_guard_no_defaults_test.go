//go:build e2e

// Spec 067 Scope 4 — SCN-067-A05 + SCN-067-A06 e2e regression.
//
// Pins the CI-output contract for spec 067 Scope 4 NO-DEFAULTS
// failures: both the JSON and plain-text rendering MUST name the
// offending file:line, the rule ID, the SST key, the owner, the
// smackerel-no-defaults policy source, and an actionable resolution.
// Without this e2e, only the integration-layer guards prove
// detection — not that the rendering downstream consumers see
// actually includes the SST key + policy-doc link the CI policy
// requires.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	policyfoundation "github.com/smackerel/smackerel/tests/integration/policy"
)

func TestIntentPolicyGuardE2E_NoDefaultsFailuresNameSSTKey(t *testing.T) {
	dir := t.TempDir()
	mlDir := filepath.Join(dir, "ml", "app")
	intDir := filepath.Join(dir, "internal", "loader")
	for _, d := range []string{mlDir, intDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	pyFixture := filepath.Join(mlDir, "embedder.py")
	if err := os.WriteFile(pyFixture, []byte(`import os

MODEL = os.environ.get("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
`), 0o644); err != nil {
		t.Fatalf("write py fixture: %v", err)
	}
	goFixture := filepath.Join(intDir, "loader.go")
	if err := os.WriteFile(goFixture, []byte(`package loader

import "os"

func Load() string {
	v := os.Getenv("SMACKEREL_DB_HOST")
	if v == "" {
		v = "localhost"
	}
	return v
}
`), 0o644); err != nil {
		t.Fatalf("write go fixture: %v", err)
	}

	cfg := policyfoundation.PolicyConfig{ExceptionMaxAgeDays: 180}
	baseline := &policyfoundation.Baseline{SchemaVersion: "v1"}
	now := time.Now()

	var vs []policyfoundation.Violation
	a5, err := policyfoundation.PythonNoDefaultsGuard(policyfoundation.Root(dir), baseline, now, cfg)
	if err != nil {
		t.Fatalf("PythonNoDefaultsGuard: %v", err)
	}
	vs = append(vs, a5...)
	a6, err := policyfoundation.GoNoDefaultsGuard(policyfoundation.Root(dir), baseline, now, cfg)
	if err != nil {
		t.Fatalf("GoNoDefaultsGuard: %v", err)
	}
	vs = append(vs, a6...)

	report := policyfoundation.BuildReport(2, vs,
		policyfoundation.ExceptionDelta{BaselineCount: 0, CurrentCount: 0, DeltaStatus: "unchanged"})
	if report.Status != "failed" {
		t.Fatalf("status = %q, want failed", report.Status)
	}

	text := report.Text()
	for _, want := range []string{
		"rule_id=G067-A05",
		"rule_id=G067-A06",
		"ml/app/embedder.py",
		"internal/loader/loader.go",
		"EMBEDDING_MODEL",
		"SMACKEREL_DB_HOST",
		"owner=intent-policy-reviewer",
		"policy_source=.github/instructions/smackerel-no-defaults.instructions.md",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q\nfull:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("text contains ANSI color escape; expected color-free")
	}

	js, err := report.JSON()
	if err != nil {
		t.Fatalf("report.JSON: %v", err)
	}
	jsStr := string(js)
	for _, want := range []string{
		`"rule_id": "G067-A05"`,
		`"rule_id": "G067-A06"`,
		`"owner": "intent-policy-reviewer"`,
		`"policy_source": ".github/instructions/smackerel-no-defaults.instructions.md"`,
		`"resolution":`,
		"EMBEDDING_MODEL",
		"SMACKEREL_DB_HOST",
	} {
		if !strings.Contains(jsStr, want) {
			t.Errorf("json missing %q\nfull:\n%s", want, jsStr)
		}
	}
}
