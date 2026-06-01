//go:build e2e

// Spec 065 SCOPE-1 — Regression E2E for SCN-065-A07.
//
// The scenario asserts that omitting the
// ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER SST key aborts core
// startup with a fail-loud error naming the missing key — no fallback
// geocoder is silently chosen (design.md "Patterns to Avoid": no
// hidden provider fallback chains, no config defaults).
//
// The test runs out-of-stack: it builds the smackerel-core binary
// from cmd/core and executes it directly with a stripped env so the
// process aborts before opening any sockets. This is the cheapest
// way to assert the fail-loud contract end-to-end without paying for
// a full docker compose stack just to observe a startup error.
//
// Skips honestly when the test harness has not been wired to inject
// the live test stack's resolved env file via SMACKEREL_TEST_ENV_FILE.
// The shell runner (./smackerel.sh test e2e) exports this variable
// when the test stack is up; bubbles.test will run this case under
// that runner.

package assistant_e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMicroToolsE2E_MissingLocationProviderFailsStartup asserts SCN-065-A07.
//
// Test shape:
//
//  1. Resolve the test-stack env file path from SMACKEREL_TEST_ENV_FILE
//     (skip honestly if unset — the harness owns injection).
//  2. Read its contents, strip the
//     ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER= line so the
//     variable resolves to empty.
//  3. `go run ./cmd/core` with the stripped env.
//  4. Expect non-zero exit AND stderr/stdout naming the missing key.
func TestMicroToolsE2E_MissingLocationProviderFailsStartup(t *testing.T) {
	envFile := os.Getenv("SMACKEREL_TEST_ENV_FILE")
	if envFile == "" {
		t.Skip("e2e: SMACKEREL_TEST_ENV_FILE not set — the runner must inject the resolved test env file path so this test can strip one required key and re-launch cmd/core. Without it the test cannot honestly exercise the fail-loud SST contract.")
	}
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file %s: %v", envFile, err)
	}

	const targetKey = "ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER"
	stripped, removed := stripEnvKey(string(raw), targetKey)
	if !removed {
		t.Fatalf("test env file %s did not contain %s=...; cannot exercise SCN-065-A07", envFile, targetKey)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	envSlice := parseDotEnv(stripped)
	// Defensive: explicitly null the variable so any inherited
	// environment value cannot mask the missing-key signal.
	envSlice = append(envSlice, targetKey+"=")

	cmd := exec.Command("go", "run", "./cmd/core")
	cmd.Dir = repoRoot
	cmd.Env = envSlice
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// 60s is generous: cmd/core aborts in <2s once env validation
	// fails. The wall-clock cap protects against the binary
	// accidentally proceeding to socket-listen and hanging.
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cmd/core: %v", err)
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("cmd/core unexpectedly exited 0 despite missing %s. Output:\n%s", targetKey, out.String())
		}
		body := out.String()
		if !strings.Contains(body, targetKey) {
			t.Fatalf("cmd/core failed but did not name %s in output (got:\n%s\n)", targetKey, body)
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("cmd/core did not abort within 60s after stripping %s; fail-loud contract regressed. Output so far:\n%s", targetKey, out.String())
	}
}

// stripEnvKey removes any KEY=... line for the named key. Returns
// the rewritten body and whether at least one line was removed.
func stripEnvKey(body, key string) (string, bool) {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	removed := false
	prefix := key + "="
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			removed = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), removed
}

// parseDotEnv converts a dotenv-style file body into KEY=value
// entries suitable for exec.Cmd.Env. Lines starting with # and blank
// lines are skipped. No shell expansion is performed.
func parseDotEnv(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(trimmed, "=") {
			continue
		}
		out = append(out, trimmed)
	}
	// Inherit PATH so `go` can be located. Other host vars are
	// deliberately excluded to keep the test hermetic.
	if p := os.Getenv("PATH"); p != "" {
		out = append(out, "PATH="+p)
	}
	if h := os.Getenv("HOME"); h != "" {
		out = append(out, "HOME="+h)
	}
	if g := os.Getenv("GOPATH"); g != "" {
		out = append(out, "GOPATH="+g)
	}
	if g := os.Getenv("GOCACHE"); g != "" {
		out = append(out, "GOCACHE="+g)
	}
	return out
}

// findRepoRoot walks up from the current working directory until it
// finds a smackerel.sh sibling.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "smackerel.sh")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
