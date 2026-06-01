// Spec 069 SCOPE-5 — Real-repo cleanliness check for the
// transport-branch guard (SCN-069-A08). This is the equivalent of
// the integration-tagged test in tests/integration/policy/ but
// scoped to live in the same package as the guard so it does not
// import the (transitively-broken-during-active-spec-work) config
// package and can run as part of `go test
// ./internal/assistant/intent/policyguard/...`.
//
// If this ever turns red, do NOT silence it — fix the new
// scenario/facade/executor site that started branching on
// AssistantMessage.Transport, or add the file to
// AllowedTransportInspectors if (and only if) it is genuinely an
// adapter or audit/context-store/transport-identity layer.

package policyguard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func realRepoRoot(t *testing.T) string {
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

func TestReportTransportBranchViolations_RealAssistantSubtreeIsClean(t *testing.T) {
	root := realRepoRoot(t)
	got, err := ReportTransportBranchViolations(filepath.Join(root, "internal", "assistant"))
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if len(got) != 0 {
		msgs := make([]string, 0, len(got))
		for _, f := range got {
			msgs = append(msgs, f.Message)
		}
		t.Fatalf("internal/assistant transport-branch findings = %d, want 0:\n%s",
			len(got), strings.Join(msgs, "\n"))
	}
}
