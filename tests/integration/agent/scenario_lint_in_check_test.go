//go:build integration

// Spec 037 Scope 10 — `cmd/scenario-lint` wired into ./smackerel.sh check.
//
// Scope 10's DoD requires that the scenario linter actually runs as
// part of `./smackerel.sh check`. This test asserts that the wiring is
// in place by inspecting smackerel.sh + scripts/runtime/scenario-lint.sh
// — a regression that disables either side would fail this test before
// the next CI build silently lets a malformed scenario reach
// production.
//
// We do NOT shell out to `./smackerel.sh check` from this test because:
//   - That would require the full Docker tooling stack inside the
//     integration container (recursive docker-in-docker).
//   - The check command is a long bash pipeline whose pass/fail is
//     already validated by `./smackerel.sh check` itself in CI.
//
// Instead we assert the static contract: the check command sources the
// scenario-lint script, the script invokes `cmd/scenario-lint` with
// the AGENT_SCENARIO_DIR + AGENT_SCENARIO_GLOB from the generated env
// file, and the linter's main package compiles and runs cleanly
// against the real tree.
package agent_integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestScope10_ScenarioLintWired_InCheckCommand asserts that
// smackerel.sh's `check` subcommand calls scripts/runtime/scenario-lint.sh.
//
// Adversarial gates:
//
//	G1: smackerel.sh contains the scenario-lint invocation block
//	G2: scripts/runtime/scenario-lint.sh exists and runs cmd/scenario-lint
//	G3: the script reads AGENT_SCENARIO_DIR from the generated env file
//	    (proving SST-driven dir, not a hardcoded path)
func TestScope10_ScenarioLintWired_InCheckCommand(t *testing.T) {
	root := repoRootForTests(t)

	// G1: check command wires the scenario-lint script.
	smackerelSh, err := os.ReadFile(filepath.Join(root, "smackerel.sh"))
	if err != nil {
		t.Fatalf("read smackerel.sh: %v", err)
	}
	if !strings.Contains(string(smackerelSh), "scripts/runtime/scenario-lint.sh") {
		t.Fatal("G1: smackerel.sh check does not invoke scripts/runtime/scenario-lint.sh")
	}
	if !strings.Contains(string(smackerelSh), "scenario-lint: OK") {
		t.Fatal("G1: smackerel.sh missing the post-lint OK echo (regression: the wiring may have been removed)")
	}

	// G2: the lint script exists and shells into cmd/scenario-lint.
	lintScript, err := os.ReadFile(filepath.Join(root, "scripts", "runtime", "scenario-lint.sh"))
	if err != nil {
		t.Fatalf("G2: scenario-lint.sh missing: %v", err)
	}
	lintBody := string(lintScript)
	if !strings.Contains(lintBody, "go run ./cmd/scenario-lint") {
		t.Fatal("G2: scenario-lint.sh does not invoke the cmd/scenario-lint binary")
	}

	// G3: SST-driven dir resolution (no hardcoded scenario path).
	if !strings.Contains(lintBody, "AGENT_SCENARIO_DIR") {
		t.Fatal("G3: scenario-lint.sh does not read AGENT_SCENARIO_DIR from the generated env file")
	}
	if !strings.Contains(lintBody, "AGENT_SCENARIO_GLOB") {
		t.Fatal("G3: scenario-lint.sh does not read AGENT_SCENARIO_GLOB from the generated env file")
	}
}

// TestScope10_ScenarioLint_RunsCleanOnRealTree compiles + runs
// `go run ./cmd/scenario-lint config/prompt_contracts` against the
// repo's actual scenario directory. Exit 0 on the present tree means
// the wired-in lint step would not fail `./smackerel.sh check`. If
// this test fails locally, run `./smackerel.sh check` to surface the
// rejection list directly.
//
// The integration runner is a fresh golang container with the
// workspace mounted at /workspace; a `go run` invocation here just
// re-uses the same toolchain the production check step uses, so the
// behaviour is identical.
func TestScope10_ScenarioLint_RunsCleanOnRealTree(t *testing.T) {
	root := repoRootForTests(t)
	dir := filepath.Join(root, "config", "prompt_contracts")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("scenario dir %s missing: %v", dir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/scenario-lint", "-glob", "*.yaml", dir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-lint failed (would break ./smackerel.sh check):\n%s\nerr=%v", string(out), err)
	}
	// Sanity-check the linter actually scanned the directory (a
	// silent zero-files run would be a setup regression).
	if !strings.Contains(string(out), "scenarios registered:") {
		t.Fatalf("scenario-lint output missing summary line:\n%s", string(out))
	}
}
