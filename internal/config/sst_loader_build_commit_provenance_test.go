package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// redteam F6 / BUG-029-008 — Go driver for the build-commit provenance shell
// regression.
//
// The shell file scripts/commands/config_build_commit_provenance_test.sh owns
// the actual config.sh invocation and assertions; this Go test invokes it
// under `go test` so the same `./smackerel.sh test unit --go` command surfaces
// unit-tier coverage of the provenance fix.
//
// The shell test runs the SST generator twice against config/smackerel.yaml:
//
//   - Sub-test 1 (core): with SMACKEREL_COMMIT UNSET, dev.env carries a real
//     12-hex git SHA (optionally `-dirty`), never the literal "unknown" the
//     redteam observed on the live local-operator images.
//   - Sub-test 2 (precedence): an exported SMACKEREL_COMMIT sentinel is passed
//     through verbatim (CI / shell-env override wins).
//
// Adversarial proof: reverting the git-derivation arm in
// scripts/commands/config.sh (back to SMACKEREL_COMMIT="unknown") makes
// Sub-test 1 fail.
func TestSSTLoader_BuildCommitProvenance_BUG029008(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("config generator shell test requires bash; skipping on windows")
	}

	// Resolve repo root from this test file's path. internal/config is two
	// levels deep from the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "scripts", "commands", "config_build_commit_provenance_test.sh")

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(cmd.Environ(),
		"REPO_ROOT="+repoRoot,
		// config.sh requires the hardware tier and is normally fed it by the
		// smackerel.sh wrapper, which this direct exec bypasses. Mirrors the
		// sibling sst_loader_home_lab_runtime_env_test.go cmd.Env.
		"SMACKEREL_HARDWARE_TIER=cpu",
		// Make config.sh's `git -C "$REPO_ROOT" rev-parse` work under the Docker
		// test surface (golang container runs as root; the host-owned /workspace
		// mount otherwise trips git's "dubious ownership" guard). Test-harness
		// only — the real evo-x2 run is by the repo owner, so config.sh itself
		// never needs this. Mirrors internal/deploy/local_client_build_test.go.
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BUG-029-008 build-commit provenance shell test failed: %v\n--- output ---\n%s\n--- end ---", err, string(out))
	}
	t.Logf("BUG-029-008 build-commit provenance shell test output:\n%s", string(out))
}
