package preflight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validEnv returns a minimal env map carrying both required threshold keys.
func validEnv(ramMB, diskGB string) map[string]string {
	return map[string]string{
		EnvKeyMinRAMMB:  ramMB,
		EnvKeyMinDiskGB: diskGB,
	}
}

// --- Evaluate (pure comparison) -------------------------------------------

func TestEvaluate_AtOrAboveThreshold(t *testing.T) {
	th := Thresholds{MinAvailableRAMMB: 6000, MinAvailableDiskGB: 15}
	// Exactly at threshold (RAM == min, disk == min*1024) must be OK.
	res := Resources{AvailableRAMMB: 6000, AvailableDiskMB: 15 * 1024}
	got := Evaluate(res, th)
	if !got.OK || got.RAMShort || got.DiskShort {
		t.Fatalf("expected OK at threshold, got %+v", got)
	}

	// Comfortably above.
	res = Resources{AvailableRAMMB: 12000, AvailableDiskMB: 100 * 1024}
	got = Evaluate(res, th)
	if !got.OK {
		t.Fatalf("expected OK above threshold, got %+v", got)
	}
}

func TestEvaluate_BelowThreshold(t *testing.T) {
	th := Thresholds{MinAvailableRAMMB: 6000, MinAvailableDiskGB: 15}

	// RAM short only.
	got := Evaluate(Resources{AvailableRAMMB: 5999, AvailableDiskMB: 100 * 1024}, th)
	if got.OK || !got.RAMShort || got.DiskShort {
		t.Fatalf("expected RAM-short, got %+v", got)
	}

	// Disk short only (one MB under the 15 GB floor).
	got = Evaluate(Resources{AvailableRAMMB: 12000, AvailableDiskMB: 15*1024 - 1}, th)
	if got.OK || got.RAMShort || !got.DiskShort {
		t.Fatalf("expected disk-short, got %+v", got)
	}

	// Both short.
	got = Evaluate(Resources{AvailableRAMMB: 100, AvailableDiskMB: 100}, th)
	if got.OK || !got.RAMShort || !got.DiskShort {
		t.Fatalf("expected both-short, got %+v", got)
	}
}

// --- ParseThresholds (fail-loud, NO-DEFAULTS) ------------------------------

func TestParseThresholds_Valid(t *testing.T) {
	th, err := ParseThresholds(validEnv("6000", "15"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.MinAvailableRAMMB != 6000 || th.MinAvailableDiskGB != 15 {
		t.Fatalf("unexpected thresholds: %+v", th)
	}
}

// Adversarial: a missing key MUST fail loud naming that key, never silently
// defaulting. This test would FAIL if ParseThresholds substituted a default.
func TestParseThresholds_MissingKeyFailsLoud(t *testing.T) {
	// Missing RAM key.
	_, err := ParseThresholds(map[string]string{EnvKeyMinDiskGB: "15"})
	if err == nil {
		t.Fatal("expected error for missing RAM key, got nil (silent default?)")
	}
	if !strings.Contains(err.Error(), EnvKeyMinRAMMB) {
		t.Fatalf("error must name the missing key %q, got: %v", EnvKeyMinRAMMB, err)
	}

	// Missing disk key.
	_, err = ParseThresholds(map[string]string{EnvKeyMinRAMMB: "6000"})
	if err == nil {
		t.Fatal("expected error for missing disk key, got nil (silent default?)")
	}
	if !strings.Contains(err.Error(), EnvKeyMinDiskGB) {
		t.Fatalf("error must name the missing key %q, got: %v", EnvKeyMinDiskGB, err)
	}
}

func TestParseThresholds_EmptyFailsLoud(t *testing.T) {
	_, err := ParseThresholds(validEnv("", "15"))
	if err == nil || !strings.Contains(err.Error(), EnvKeyMinRAMMB) {
		t.Fatalf("expected fail-loud naming %q for empty value, got: %v", EnvKeyMinRAMMB, err)
	}
}

func TestParseThresholds_NonNumericFailsLoud(t *testing.T) {
	_, err := ParseThresholds(validEnv("6000", "lots"))
	if err == nil || !strings.Contains(err.Error(), EnvKeyMinDiskGB) {
		t.Fatalf("expected fail-loud naming %q for non-numeric value, got: %v", EnvKeyMinDiskGB, err)
	}
}

func TestParseThresholds_NonPositiveFailsLoud(t *testing.T) {
	for _, bad := range []string{"0", "-1"} {
		_, err := ParseThresholds(validEnv(bad, "15"))
		if err == nil || !strings.Contains(err.Error(), EnvKeyMinRAMMB) {
			t.Fatalf("expected fail-loud naming %q for value %q, got: %v", EnvKeyMinRAMMB, bad, err)
		}
	}
}

// --- Run (decision path + exit code + override) ----------------------------

func TestPreflightRun_AtOrAboveThresholdExitsZero(t *testing.T) {
	report, code, err := Run(validEnv("6000", "15"),
		Resources{AvailableRAMMB: 12000, AvailableDiskMB: 100 * 1024}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0 above threshold, got %d", code)
	}
	if !strings.Contains(report, "OK") {
		t.Fatalf("expected OK status in report, got:\n%s", report)
	}
}

func TestPreflightRun_BelowThresholdExitsOne(t *testing.T) {
	report, code, err := Run(validEnv("6000", "15"),
		Resources{AvailableRAMMB: 2048, AvailableDiskMB: 4 * 1024}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit 1 below threshold, got %d", code)
	}
	// The report must state BOTH current and required for the operator.
	for _, want := range []string{"BELOW THRESHOLD", "2048", "6000", "Remediation", "clean smart"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q; got:\n%s", want, report)
		}
	}
}

func TestPreflightRun_OverrideBelowThresholdExitsZeroWithWarning(t *testing.T) {
	report, code, err := Run(validEnv("6000", "15"),
		Resources{AvailableRAMMB: 100, AvailableDiskMB: 100}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("override must force exit 0 even below threshold, got %d", code)
	}
	if !strings.Contains(report, "WARNING") || !strings.Contains(report, OverrideEnvKey) {
		t.Fatalf("override report must carry a loud WARNING naming %q; got:\n%s", OverrideEnvKey, report)
	}
}

func TestPreflightRun_MissingKeyReturnsError(t *testing.T) {
	_, _, err := Run(map[string]string{EnvKeyMinRAMMB: "6000"}, // disk key absent
		Resources{AvailableRAMMB: 12000, AvailableDiskMB: 100 * 1024}, false)
	if err == nil || !strings.Contains(err.Error(), EnvKeyMinDiskGB) {
		t.Fatalf("expected fail-loud error naming %q, got: %v", EnvKeyMinDiskGB, err)
	}
}

// Adversarial: the env map carries a secret (mirroring the real generated env
// file, which contains SMACKEREL_AUTH_TOKEN). The rendered report MUST NOT echo
// it. A naive "dump the env" implementation would FAIL this test.
func TestPreflightRun_NoSecretEcho(t *testing.T) {
	const planted = "SUPERSECRET_TOKEN_DO_NOT_LEAK_42"
	env := validEnv("6000", "15")
	env["SMACKEREL_AUTH_TOKEN"] = planted
	env["LLM_API_KEY"] = planted + "_apikey"

	report, _, err := Run(env, Resources{AvailableRAMMB: 100, AvailableDiskMB: 100}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(report, planted) {
		t.Fatalf("report leaked a secret value; got:\n%s", report)
	}
}

// --- Host I/O helpers ------------------------------------------------------

func TestReadMemAvailableMBFrom_Synthetic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	content := "MemTotal:       32000000 kB\n" +
		"MemFree:         1000000 kB\n" +
		"MemAvailable:    8388608 kB\n" + // 8388608 kB = 8192 MB
		"Buffers:          100000 kB\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readMemAvailableMBFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 8192 {
		t.Fatalf("expected 8192 MB, got %d", got)
	}
}

func TestReadMemAvailableMBFrom_MissingField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	if err := os.WriteFile(path, []byte("MemTotal: 32000000 kB\nMemFree: 1000000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMemAvailableMBFrom(path); err == nil {
		t.Fatal("expected error when MemAvailable is absent")
	}
}

func TestReadDiskAvailableMB_TempDir(t *testing.T) {
	// A real temp dir on a real filesystem must report a positive available MB.
	got, err := ReadDiskAvailableMB(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("expected positive available disk MB, got %d", got)
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.env")
	content := "# comment line\n" +
		"\n" +
		"PREFLIGHT_MIN_AVAILABLE_RAM_MB=6000\n" +
		"PREFLIGHT_MIN_AVAILABLE_DISK_GB=15\n" +
		"DATABASE_URL=postgres://u:p@h:5432/db?sslmode=disable\n" // value contains '='
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	env, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env[EnvKeyMinRAMMB] != "6000" || env[EnvKeyMinDiskGB] != "15" {
		t.Fatalf("threshold keys not parsed: %+v", env)
	}
	if env["DATABASE_URL"] != "postgres://u:p@h:5432/db?sslmode=disable" {
		t.Fatalf("value with '=' was truncated: %q", env["DATABASE_URL"])
	}
	if _, ok := env["# comment line"]; ok {
		t.Fatal("comment line was parsed as a key")
	}
}

func TestTruthy(t *testing.T) {
	for _, s := range []string{"1", "true", "TRUE", "yes", "on", " On "} {
		if !Truthy(s) {
			t.Fatalf("expected %q truthy", s)
		}
	}
	for _, s := range []string{"", "0", "false", "no", "off", "maybe"} {
		if Truthy(s) {
			t.Fatalf("expected %q falsey", s)
		}
	}
}
