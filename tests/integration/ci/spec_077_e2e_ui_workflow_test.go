//go:build integration

// Spec 077 SCOPE-2 — TP-077-02-04 (SCN-077-A06).
//
// Static contract test: the .github/workflows/e2e-ui.yml CI workflow
// MUST exist and MUST invoke `./smackerel.sh test e2e-ui`. The check
// is intentionally static (file-level) — the integration runner has
// no GitHub Actions surface to introspect. The static contract is
// adversarial: if a future edit silently removes the e2e-ui step
// (or renames it), this test fails immediately.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpec077E2EUIWorkflowExists_AndInvokesSmackerelTestE2EUI(t *testing.T) {
	repoRoot := repoRootForSpec077E2EUIWorkflow(t)
	wf := filepath.Join(repoRoot, ".github", "workflows", "e2e-ui.yml")

	data, err := os.ReadFile(wf)
	if err != nil {
		t.Fatalf("CI workflow not found at %s: %v", wf, err)
	}
	body := string(data)

	mustContain := []string{
		"name: E2E UI",
		"./smackerel.sh test e2e-ui",
		"on:",
		"pull_request:",
		"push:",
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("CI workflow %s missing required token %q", wf, want)
		}
	}

	// Adversarial: prove the assertion is not satisfied by a workflow
	// that merely mentions the command in a comment. Require it to
	// appear on a `run:` line (the GitHub Actions step body).
	foundRun := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "run:") &&
			strings.Contains(trimmed, "./smackerel.sh test e2e-ui") {
			foundRun = true
			break
		}
	}
	if !foundRun {
		t.Errorf("CI workflow %s must invoke `./smackerel.sh test e2e-ui` from a `run:` step, not only in comments", wf)
	}

	// Adversarial: a missing-artifact-upload step would silently lose
	// observability on failure. Require an upload-artifact action.
	if !strings.Contains(body, "upload-artifact@") {
		t.Errorf("CI workflow %s must upload Playwright artifacts on failure (actions/upload-artifact)", wf)
	}
}

func repoRootForSpec077E2EUIWorkflow(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root from %s", wd)
		}
		dir = parent
	}
}
