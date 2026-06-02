//go:build e2e

package e2e

// Spec 062 SCN-062-A05 — fail-loud end-to-end check.
//
// Build the smackerel-core binary, launch it with config/generated/test.env
// loaded into the subprocess environment with ONE required HTTP transport
// env var removed, and assert:
//   1. the process exits non-zero, AND
//   2. stderr contains the registry's exact FailLoudMsg for that key.
//
// This proves the SCOPE-2 wiring: a missing required key reaches the
// operator as the registry-defined message, not a generic config error.

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/transportconfig"
)

func TestHTTPAdapter_MissingRequiredKey_FailsLoud(t *testing.T) {
	// Resolve repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		t.Fatalf("repo root resolution failed (%q): %v", repoRoot, err)
	}

	envFile := filepath.Join(repoRoot, "config", "generated", "test.env")
	envMap, err := loadEnvFile(envFile)
	if err != nil {
		t.Skipf("cannot read %q (%v); regenerate via './smackerel.sh config generate' as the test user", envFile, err)
	}
	if len(envMap) == 0 {
		t.Fatalf("%q parsed empty — refusing to run without a baseline env", envFile)
	}

	// Pick the canonical HTTP key under test. Registry order is
	// stable so this test is deterministic across runs.
	targetKey := "ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID"
	wantMsg := transportconfig.FailLoudMessageFor(targetKey)
	if wantMsg == "" {
		t.Fatalf("registry has no FailLoudMsg for %q", targetKey)
	}
	if _, present := envMap[targetKey]; !present {
		t.Fatalf("baseline env file %q does not contain %q (cannot remove what is not present)", envFile, targetKey)
	}
	delete(envMap, targetKey)

	// Build the binary into a temp dir so we do not pollute the repo.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "smackerel-core")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/core")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build smackerel-core failed: %v\n%s", err, out)
	}

	// Run the binary with the stripped env. We expect immediate
	// fail-loud, so a short timeout is sufficient. If wiring breaks
	// and the process starts a real listener instead of exiting, the
	// timeout aborts the test with a clear message.
	cmd := exec.Command(binPath)
	cmd.Env = envMapToSlice(envMap)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start smackerel-core: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		exitErr, ok := waitErr.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected non-zero exit; got nil error. stderr:\n%s", stderr.String())
		}
		if exitErr.ExitCode() == 0 {
			t.Fatalf("expected non-zero exit; got 0. stderr:\n%s", stderr.String())
		}
		combined := stdout.String() + "\n" + stderr.String()
		if !strings.Contains(combined, wantMsg) {
			t.Fatalf("missing registry FailLoudMsg %q in output\nstderr:\n%s", wantMsg, stderr.String())
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("smackerel-core did not exit within 15s with %q removed; fail-loud regression suspected. stderr:\n%s", targetKey, stderr.String())
	}
}

// loadEnvFile parses a KEY=VALUE-per-line env file. Lines starting
// with '#' and blank lines are skipped. Surrounding quotes on the
// value are preserved verbatim — the test does not interpret shell
// quoting, it just hands bytes to the subprocess.
func loadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		out[line[:eq]] = line[eq+1:]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func envMapToSlice(m map[string]string) []string {
	out := make([]string, 0, len(m)+2)
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	// Preserve PATH so go-built binary can find any helper tools it
	// might exec (none expected on the fail-loud path, but cheap to
	// pass through).
	if p := os.Getenv("PATH"); p != "" {
		out = append(out, "PATH="+p)
	}
	if h := os.Getenv("HOME"); h != "" {
		out = append(out, "HOME="+h)
	}
	return out
}
