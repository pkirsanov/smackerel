package deploy

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDeployTargetStatusDelegationAndFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("deploy-target shell test requires bash; skipping on windows")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "scripts", "commands", "deploy_target_status_test.sh")

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(cmd.Environ(), "REPO_ROOT="+repoRoot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy-target status shell test failed: %v\n--- output ---\n%s\n--- end ---", err, string(out))
	}
	t.Logf("deploy-target status shell test output:\n%s", string(out))
}
