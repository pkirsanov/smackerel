//go:build integration

// Spec 068 Scope 4 — Raw-route bypass guard (SCN-068-A08).
//
// This live integration test runs the policy guard against the real
// repository tree and asserts:
//
//   1. Zero findings under the scoped user-facing surface
//      (internal/assistant/...). i.e. every Router.Route call site
//      in the assistant facade is properly preceded by an
//      intent.Compiler reference.
//   2. A planted fixture file that calls router.Route() without any
//      intent.Compiler reference IS reported, and the message names
//      both the file and the missing compiler step.

package policy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent/policyguard"
)

// repoRoot returns the repository root by walking up from this test
// file until go.mod is found.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("go.mod not found walking up from %s", file)
	return ""
}

// TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent
// SCN-068-A08.
//
// Live guard run. The real internal/assistant subtree MUST be clean
// (zero findings). A planted fixture that calls router.Route() with
// no intent.Compiler reference MUST be reported with a message that
// names the file AND the missing compiler step.
func TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent(t *testing.T) {
	root := repoRoot(t)

	// 1. Real assistant surface MUST be clean.
	clean, err := policyguard.ReportRawRouteBypasses(filepath.Join(root, "internal", "assistant"))
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses(real): %v", err)
	}
	if len(clean) != 0 {
		var msgs []string
		for _, f := range clean {
			msgs = append(msgs, f.Message)
		}
		t.Fatalf("expected zero findings under internal/assistant, got %d:\n%s",
			len(clean), strings.Join(msgs, "\n"))
	}

	// 2. Planted fixture with raw bypass MUST be flagged.
	dir := t.TempDir()
	fixture := filepath.Join(dir, "bad_ingress.go")
	body := `package bad

import "context"

type Router interface {
	Route(ctx context.Context, env interface{}) (string, error)
}

func Handle(ctx context.Context, r Router, raw string) {
	// raw-text routing without any compiler step.
	_, _ = r.Route(ctx, raw)
}
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := policyguard.ReportRawRouteBypasses(dir)
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses(fixture): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("fixture: got %d findings, want 1", len(got))
	}
	f := got[0]
	if !strings.Contains(f.Message, "bad_ingress.go") {
		t.Fatalf("finding message does not name the file: %q", f.Message)
	}
	if !strings.Contains(f.Message, policyguard.MissingCompilerStep) {
		t.Fatalf("finding message does not name the missing compiler step: %q", f.Message)
	}

	// 3. Adversarial baseline: same fixture body but with an
	// intent.Compiler reference MUST NOT be flagged. Without this,
	// a regression where the guard always-fires would still pass
	// the planted-fixture assertion above.
	good := strings.Replace(body,
		"// raw-text routing without any compiler step.",
		"// compiled by intent.Compiler upstream\n\t_ = \"intent.Compiler\"",
		1)
	if err := os.WriteFile(fixture, []byte(good), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	got2, err := policyguard.ReportRawRouteBypasses(dir)
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses(fixture v2): %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("adversarial baseline: fixture with intent.Compiler reference was flagged: %+v", got2)
	}
}
