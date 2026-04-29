//go:build integration

// Spec 038 Scope 1 — SCN-038-001 integration row.
//
// TestDriveConfigGenerateAndRuntimeValidationStayInSync proves that the
// config-generation pipeline and the Go runtime config loader agree on
// the set of required drive SST keys:
//
//  1. The generated config/generated/dev.env carries every required
//     DRIVE_* env var (the env_file drift guard inside `./smackerel.sh
//     check` already verifies the file is in sync with smackerel.yaml,
//     so its presence here proves the generator emitted them).
//  2. Adversarial — stripping a required drive key from a temp copy of
//     smackerel.yaml causes the generator to exit non-zero and name the
//     missing key in stderr.
//
// The drift-positive direction (loader expects a key the generator does
// not emit) is covered by the live integration startup itself: the test
// stack will not become healthy unless the loader successfully parses
// every generator-produced env var. Combining these signals proves the
// two sides stay in sync — adding a drive key without updating the
// generator (or vice versa) would fail at least one of these gates.
package drive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repoRootForConfigContract(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i = i + 1 {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

func TestDriveConfigGenerateAndRuntimeValidationStayInSync(t *testing.T) {
	root := repoRootForConfigContract(t)
	srcYAML := filepath.Join(root, "config", "smackerel.yaml")

	envPath := filepath.Join(root, "config", "generated", "dev.env")
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read generated dev.env: %v", err)
	}
	envText := string(envBytes)
	required := []string{
		"DRIVE_ENABLED=",
		"DRIVE_CLASSIFICATION_ENABLED=",
		"DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD=",
		"DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION=",
		"DRIVE_SCAN_PARALLELISM=",
		"DRIVE_SCAN_BATCH_SIZE=",
		"DRIVE_MONITOR_POLL_INTERVAL_SECONDS=",
		"DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES=",
		"DRIVE_POLICY_SENSITIVITY_DEFAULT=",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC=",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL=",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE=",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET=",
		"DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES=",
		"DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY=",
		"DRIVE_LIMITS_MAX_FILE_SIZE_BYTES=",
		"DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE=",
		"DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL=",
		"DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS=",
	}
	for _, want := range required {
		if !strings.Contains(envText, want) {
			t.Errorf("generated dev.env missing %q", want)
		}
	}
	t.Logf("generated dev.env contains every required DRIVE_ key (%d keys checked)", len(required))

	srcBytes, err := os.ReadFile(srcYAML)
	if err != nil {
		t.Fatalf("read source yaml: %v", err)
	}
	const target = "    confidence_threshold:"
	if !strings.Contains(string(srcBytes), target) {
		t.Fatalf("source yaml missing expected %q (drive block moved?)", target)
	}
	stripped := 0
	out := make([]string, 0)
	for _, ln := range strings.Split(string(srcBytes), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "confidence_threshold:") &&
			strings.Contains(ln, "min confidence to apply classification") {
			stripped = stripped + 1
			continue
		}
		out = append(out, ln)
	}
	if stripped == 0 {
		t.Fatalf("expected to strip at least one confidence_threshold line, stripped=%d", stripped)
	}
	tmpYAML := filepath.Join(t.TempDir(), "smackerel.yaml")
	if err := os.WriteFile(tmpYAML, []byte(strings.Join(out, "\n")), 0o600); err != nil {
		t.Fatalf("write stripped yaml: %v", err)
	}

	advCtx, advCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer advCancel()
	advCmd := exec.CommandContext(advCtx, "bash",
		filepath.Join(root, "scripts", "commands", "config.sh"),
		"--config", tmpYAML,
		"--env", "dev",
	)
	advCmd.Env = append(os.Environ(), "TARGET_ENV_GUARD=integration-038-001-adv")
	advOut, advErr := advCmd.CombinedOutput()
	advExit := 0
	if advErr != nil {
		if ee, ok := advErr.(*exec.ExitError); ok {
			advExit = ee.ExitCode()
		} else {
			t.Fatalf("run adversarial config.sh: %v output=%s", advErr, string(advOut))
		}
	}
	t.Logf("adversarial config.sh exit=%d output=%s", advExit, strings.TrimSpace(string(advOut)))
	if advExit == 0 {
		t.Fatalf("adversarial config.sh exit=0 with missing drive.classification.confidence_threshold; expected non-zero")
	}
	if !strings.Contains(string(advOut), "drive.classification.confidence_threshold") {
		t.Errorf("adversarial output does not name the missing key: %s", string(advOut))
	}
}
