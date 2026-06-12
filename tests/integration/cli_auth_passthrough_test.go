//go:build integration

// Spec 060 Scope 3 — DI-060-02 quick-win.
//
// Integration test for the `./smackerel.sh auth` passthrough wrapper.
// The wrapper forwards args verbatim to `smackerel auth ...` inside
// the running smackerel-core container and MUST propagate the
// in-container exit code unchanged.
//
// Spec 060 Scope 3 DoD line 472 ("CLI passthrough integration test
// validates exit code propagation and arg forwarding") was deferred
// at certification with concern DI-060-02. This file discharges that
// deferral.
//
// Validates:
//  1. `./smackerel.sh --env test auth` (no subcommand) → exit 2 with
//     usage banner. Proves the wrapper reaches the in-container CLI
//     and the CLI's "usage" exit code (2) crosses `docker compose
//     exec` and the shell wrapper unchanged.
//  2. `./smackerel.sh --env test auth not-a-real-subcommand` → exit
//     2 with "unknown subcommand" message. Proves the arg string
//     reaches the in-container CLI verbatim (not flag-rewritten,
//     not comma-split).
//
// Prerequisites: live test stack up via `./smackerel.sh test
// integration` (or equivalent). Both sub-tests assume the
// smackerel-core container in the `test` compose project is running.
// No mocks, no fakes — the test exercises the real shell wrapper
// invoking real `docker compose exec` against the real container.
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cliPassthroughRepoRoot climbs from the test CWD looking for
// smackerel.sh + config/smackerel.yaml. Mirrors the per-test
// repo-root helper pattern used by config_validate_test.go.
func cliPassthroughRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		_, errSh := os.Stat(filepath.Join(dir, "smackerel.sh"))
		_, errYAML := os.Stat(filepath.Join(dir, "config", "smackerel.yaml"))
		if errSh == nil && errYAML == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s (looking for smackerel.sh + config/smackerel.yaml)", wd)
	return ""
}

// runSmackerelAuth invokes ./smackerel.sh --env test auth <args...>
// and returns (exitCode, combinedOutput). exitCode is the propagated
// in-container exit code (0 on success, non-zero from the
// in-container CLI; *exec.ExitError unwraps to the actual numeric
// code).
func runSmackerelAuth(t *testing.T, root string, args ...string) (int, string) {
	t.Helper()
	// The auth passthrough wrapper shells out to `docker compose exec`
	// into the running smackerel-core container, so it requires the docker
	// CLI + daemon on PATH. The containerized go-integration runner
	// (golang:bookworm, no docker socket) cannot satisfy that — the `auth)`
	// arm's require_docker aborts with "docker is required" (exit 1) before
	// any compose exec. Honestly skip there (consistent with the repo's
	// env-gated integration skips, e.g. assistant_transport_hint_test.go).
	// On a host with docker + a running test stack the wrapper exit-code
	// propagation is exercised fully.
	if _, lookErr := exec.LookPath("docker"); lookErr != nil {
		t.Skip("integration: docker CLI not on PATH (containerized go-integration runner); ./smackerel.sh auth passthrough requires host docker to exec into smackerel-core")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	full := append([]string{"--env", "test", "auth"}, args...)
	cmd := exec.CommandContext(ctx, filepath.Join(root, "smackerel.sh"), full...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("exec ./smackerel.sh --env test auth %v: %v output=%s",
				args, err, string(out))
		}
		exitCode = ee.ExitCode()
	}
	return exitCode, string(out)
}

// TestCLIAuthPassthrough_NoArgsExitsTwo asserts that calling the
// wrapper with no subcommand returns exit code 2 (the in-container
// CLI's usage-error code). Proves exit-code propagation through
// `docker compose exec` → bash wrapper.
func TestCLIAuthPassthrough_NoArgsExitsTwo(t *testing.T) {
	root := cliPassthroughRepoRoot(t)
	exitCode, out := runSmackerelAuth(t, root /* no args */)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for `auth` with no subcommand, got %d\noutput:\n%s",
			exitCode, out)
	}
	if !strings.Contains(out, "usage: smackerel auth") {
		t.Errorf("expected usage banner (\"usage: smackerel auth\") in output; got:\n%s", out)
	}
}

// TestCLIAuthPassthrough_UnknownSubcommandExitsTwo asserts that
// an unknown subcommand string is forwarded verbatim and the
// in-container CLI returns exit 2 with an "unknown subcommand"
// message. Proves args are NOT rewritten or filtered by the
// wrapper.
func TestCLIAuthPassthrough_UnknownSubcommandExitsTwo(t *testing.T) {
	root := cliPassthroughRepoRoot(t)
	exitCode, out := runSmackerelAuth(t, root, "not-a-real-subcommand")

	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for unknown subcommand, got %d\noutput:\n%s",
			exitCode, out)
	}
	if !strings.Contains(out, "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand' message in output; got:\n%s", out)
	}
	// Verify the arg string crossed the wrapper unchanged.
	if !strings.Contains(out, "not-a-real-subcommand") {
		t.Errorf("expected forwarded arg 'not-a-real-subcommand' to appear in error output; got:\n%s", out)
	}
}
