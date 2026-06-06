package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// BUG-051-001 / SCN-051-001-A through SCN-051-001-D — Go driver for the
// SST-loader home-lab runtime-env-mode shell regression.
//
// The shell file scripts/commands/config_home_lab_runtime_env_test.sh owns
// the actual SST-loader invocation and assertions; this Go test invokes it
// under `go test` so the same `./smackerel.sh test unit --go` command
// surfaces both unit-tier and shell-tier coverage of the BUG-051-001 fix.
//
// The shell test patches a temp copy of config/smackerel.yaml with a
// non-default Postgres password (so the orthogonal FR-051-005 generator-side
// rejection does not block the SMACKEREL_ENV-mapping assertion), then runs
// the SST loader against four target envs and asserts:
//
//   - Sub-test 1 (BUG-051-001 core): TARGET_ENV=home-lab emits
//     SMACKEREL_ENV=production into the generated env file.
//   - Sub-test 2 (canary): TARGET_ENV=dev still emits
//     SMACKEREL_ENV=development.
//   - Sub-test 3 (canary): TARGET_ENV=test still emits SMACKEREL_ENV=test.
//   - Sub-test 4 (defense-in-depth): TARGET_ENV=home-lab against the
//     unpatched live yaml with the dev-default Postgres password is still
//     rejected by the FR-051-005 generator-side guard.
//
// Adversarial proof: reverting the home-lab arm of the per-target case in
// scripts/commands/config.sh makes Sub-test 1 fail because the home-lab
// bundle reverts to SMACKEREL_ENV=development (the previous SEC-HL-001
// defense-in-depth bypass).
func TestSSTLoader_HomeLabEmitsProductionRuntimeEnv_BUG051001(t *testing.T) {
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
	scriptPath := filepath.Join(repoRoot, "scripts", "commands", "config_home_lab_runtime_env_test.sh")

	cmd := exec.Command("bash", scriptPath)
	// REPO_ROOT lets config.sh resolve the repo without re-deriving it; an
	// explicit SMACKEREL_HARDWARE_TIER keeps the test hermetic w.r.t. the
	// ambient shell (config.sh requires the tier and is normally fed it by
	// the smackerel.sh wrapper, which this direct exec bypasses). Mirrors the
	// sibling sst_loader_test.go cmd.Env.
	cmd.Env = append(cmd.Environ(), "REPO_ROOT="+repoRoot, "SMACKEREL_HARDWARE_TIER=cpu")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BUG-051-001 SST-loader shell test failed: %v\n--- output ---\n%s\n--- end ---", err, string(out))
	}
	t.Logf("BUG-051-001 SST-loader shell test output:\n%s", string(out))
}
