package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/smackerel/smackerel/internal/testsupport/configgen"
)

// Spec 051 SCN-051-S02 / FR-051-005 — Go driver for the SST-loader
// shell test. The shell file scripts/commands/config_secret_rejection_test.sh
// owns the actual SST-loader invocation and assertions; this Go test
// invokes it under `go test` so the same `./smackerel.sh test unit --go`
// command surfaces both unit-tier and shell-tier coverage of the spec
// 051 contract.

// TestSSTLoader_RejectsDevPostgresPassword_SelfHosted is the linked test
// referenced by specs/051-deployment-secret-auth-contract/scenario-manifest.json
// for SCN-051-S02 evidence. It defers the assertion logic to the
// shell driver (which is the same script an operator would run by
// hand to repro a deployment refusal).
func TestSSTLoader_RejectsDevPostgresPassword_SelfHosted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SST loader shell test requires bash; skipping on windows")
	}

	// Resolve repo root from this test file's path. internal/config is
	// two levels deep from the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "scripts", "commands", "config_secret_rejection_test.sh")

	cmd := exec.Command("bash", scriptPath)
	// configgen.HermeticEnv supplies the fail-loud inputs the SST loader
	// requires and deliberately has no committed value for; the shell script
	// inherits them when it invokes config.sh.
	cmd.Env = append(append(cmd.Environ(), "REPO_ROOT="+repoRoot), configgen.HermeticEnv()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("SST loader shell test failed: %v\n--- output ---\n%s\n--- end ---", err, string(out))
	}
	t.Logf("SST loader shell test output:\n%s", string(out))
}
