package readiness

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFromEnvRequiresAllStressValues(test *testing.T) {
	test.Setenv("CORE_EXTERNAL_URL", "")
	test.Setenv("DATABASE_URL", "")
	test.Setenv("NATS_URL", "")
	test.Setenv("SMACKEREL_AUTH_TOKEN", "")

	_, err := ConfigFromEnv()
	if err == nil {
		test.Fatal("expected missing env error")
	}
	for _, key := range []string{"CORE_EXTERNAL_URL", "DATABASE_URL", "NATS_URL", "SMACKEREL_AUTH_TOKEN"} {
		if !strings.Contains(err.Error(), key) {
			test.Fatalf("expected error to name %s, got %q", key, err.Error())
		}
	}
}

func TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS(test *testing.T) {
	databaseCalled := false
	natsCalled := false
	probes := Probes{
		HTTPClient: staticHTTPClient(http.StatusOK, `{"status":"healthy"}`),
		PingDatabase: func(context.Context, string) error {
			databaseCalled = true
			return nil
		},
		ConnectNATS: func(context.Context, string, string) error {
			natsCalled = true
			return nil
		},
	}

	err := CheckWithProbes(context.Background(), validConfig(), probes)
	if err == nil {
		test.Fatal("expected wrong-stack core health to fail")
	}
	if !strings.Contains(err.Error(), "authenticated health response did not include service topology") {
		test.Fatalf("expected authenticated topology error, got %q", err.Error())
	}
	if databaseCalled || natsCalled {
		test.Fatalf("expected core failure before DB/NATS probes, databaseCalled=%v natsCalled=%v", databaseCalled, natsCalled)
	}
}

func TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS(test *testing.T) {
	natsCalled := false
	probes := Probes{
		HTTPClient: staticHTTPClient(http.StatusOK, healthyTopologyBody()),
		PingDatabase: func(context.Context, string) error {
			return errors.New("connection refused")
		},
		ConnectNATS: func(context.Context, string, string) error {
			natsCalled = true
			return nil
		},
	}

	err := CheckWithProbes(context.Background(), validConfig(), probes)
	if err == nil {
		test.Fatal("expected database readiness failure")
	}
	if !strings.Contains(err.Error(), "database readiness failed") || !strings.Contains(err.Error(), "connection refused") {
		test.Fatalf("expected database reachability context, got %q", err.Error())
	}
	if natsCalled {
		test.Fatal("expected DB failure before NATS probe")
	}
}

func TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes(test *testing.T) {
	config := validConfig()
	config.NATSURL = ""
	probes := Probes{
		HTTPClient: staticHTTPClient(http.StatusOK, healthyTopologyBody()),
		PingDatabase: func(context.Context, string) error {
			test.Fatal("database probe should not run when required env is missing")
			return nil
		},
		ConnectNATS: func(context.Context, string, string) error {
			test.Fatal("NATS probe should not run when required env is missing")
			return nil
		},
	}

	err := CheckWithProbes(context.Background(), config, probes)
	if err == nil {
		test.Fatal("expected missing NATS_URL failure")
	}
	if !strings.Contains(err.Error(), "NATS_URL") {
		test.Fatalf("expected NATS_URL in error, got %q", err.Error())
	}
}

func TestCheckWithProbes_UnreachableNATSFailsAfterDatabase(test *testing.T) {
	databaseCalled := false
	probes := Probes{
		HTTPClient: staticHTTPClient(http.StatusOK, healthyTopologyBody()),
		PingDatabase: func(context.Context, string) error {
			databaseCalled = true
			return nil
		},
		ConnectNATS: func(context.Context, string, string) error {
			return errors.New("nats: no servers available for connection")
		},
	}

	err := CheckWithProbes(context.Background(), validConfig(), probes)
	if err == nil {
		test.Fatal("expected NATS readiness failure")
	}
	if !databaseCalled {
		test.Fatal("expected DB probe before NATS probe")
	}
	if !strings.Contains(err.Error(), "nats readiness failed") || !strings.Contains(err.Error(), "no servers available") {
		test.Fatalf("expected NATS reachability context, got %q", err.Error())
	}
}

func TestGoStressHarness_WorkloadFailurePropagatesAfterCanary(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	fakeBinDir := test.TempDir()
	fakeGoPath := filepath.Join(fakeBinDir, "go")
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
	"list -tags stress ./tests/stress/..."*)
		echo "github.com/smackerel/smackerel/tests/stress"
		echo "github.com/smackerel/smackerel/tests/stress/agent"
		exit 0
		;;
	esac
	printf 'fake-go args: %s\n' "$*"
	case "$*" in
	*"^TestStressReadinessCanary_Live$"*)
    echo "fake-go canary pass"
    exit 0
    ;;
	*"-list TestForcedWorkloadFailure github.com/smackerel/smackerel/tests/stress"*)
		echo "TestForcedWorkloadFailure"
		exit 0
		;;
	*"github.com/smackerel/smackerel/tests/stress"*)
    echo "fake-go workload failure"
    exit 42
    ;;
	*)
		echo "unexpected fake-go args: $*"
		exit 43
		;;
esac
`
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		test.Fatalf("write fake go: %v", err)
	}

	workspaceDir := test.TempDir()
	captureDir := test.TempDir()
	scriptPath := filepath.Join(repoRoot, "scripts", "runtime", "go-stress.sh")
	command := exec.Command("bash", scriptPath, "--run", "TestForcedWorkloadFailure")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SMACKEREL_STRESS_WORKSPACE="+workspaceDir,
		"TMPDIR="+captureDir,
		"CORE_EXTERNAL_URL=http://stress-core.invalid",
		"DATABASE_URL=postgres://stress-user:stress-pass@stress-db.invalid/stress?sslmode=disable",
		"NATS_URL=nats://stress-nats.invalid:4222",
		"SMACKEREL_AUTH_TOKEN=stress-auth-token-for-script-test",
	)
	outputBytes, err := command.CombinedOutput()
	if err == nil {
		test.Fatalf("expected workload command failure, output:\n%s", string(outputBytes))
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		test.Fatalf("expected workload exit status, got %T: %v", err, err)
	}
	if exitError.ExitCode() != 42 {
		test.Fatalf("expected workload exit 42 to propagate, got %d, output:\n%s", exitError.ExitCode(), string(outputBytes))
	}
	assertGoStressCaptureDirectoryEmpty(test, captureDir)
	output := string(outputBytes)
	if !strings.Contains(output, "go-stress: readiness canary passed") {
		test.Fatalf("expected canary pass before workload, output:\n%s", output)
	}
	if !strings.Contains(output, "fake-go workload failure") {
		test.Fatalf("expected workload failure to remain visible, output:\n%s", output)
	}
	if !strings.Contains(output, "go-stress: running workload package github.com/smackerel/smackerel/tests/stress") {
		test.Fatalf("expected workload package progress before long-running tests, output:\n%s", output)
	}
	canaryIndex := strings.Index(output, "go-stress: readiness canary passed")
	packageIndex := strings.Index(output, "go-stress: running workload package github.com/smackerel/smackerel/tests/stress")
	workloadIndex := strings.Index(output, "fake-go workload failure")
	if canaryIndex < 0 || packageIndex < 0 || workloadIndex < 0 || packageIndex < canaryIndex || workloadIndex < packageIndex {
		test.Fatalf("expected package progress and workload failure after canary pass, output:\n%s", output)
	}
}

func TestGoStressHarness_ListFailurePropagates(test *testing.T) {
	testGoStressHarnessListFailurePropagates(test)
}

func testGoStressHarnessListFailurePropagates(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	fakeBinDir := test.TempDir()
	fakeGoPath := filepath.Join(fakeBinDir, "go")
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
	"list -tags stress ./tests/stress/..."*)
		echo "github.com/smackerel/smackerel/tests/stress/pass"
		echo "github.com/smackerel/smackerel/tests/stress/fail"
		exit 0
		;;
	*"^TestStressReadinessCanary_Live$"*)
		echo "fake-go canary pass"
		exit 0
		;;
	*"-list TestListFailureSelector github.com/smackerel/smackerel/tests/stress/pass"*)
		echo "TestListFailureSelector"
		exit 0
		;;
	*"github.com/smackerel/smackerel/tests/stress/pass"*)
		echo "PASSING_PACKAGE_STREAMED"
		exit 0
		;;
	*"-list TestListFailureSelector github.com/smackerel/smackerel/tests/stress/fail"*)
		echo "forced discovery failure for fail package" >&2
		exit 42
		;;
	*)
		echo "unexpected fake-go args: $*"
		exit 43
		;;
esac
`
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		test.Fatalf("write fake go: %v", err)
	}

	captureDir := test.TempDir()
	scriptPath := filepath.Join(repoRoot, "scripts", "runtime", "go-stress.sh")
	command := exec.Command("bash", scriptPath, "--run", "TestListFailureSelector")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SMACKEREL_STRESS_WORKSPACE="+test.TempDir(),
		"TMPDIR="+captureDir,
		"CORE_EXTERNAL_URL=http://stress-core.invalid",
		"DATABASE_URL=postgres://stress-user:testpass@stress-db.invalid/stress?sslmode=disable",
		"NATS_URL=nats://stress-nats.invalid:4222",
		"SMACKEREL_AUTH_TOKEN=stress-auth-token-for-list-failure",
	)
	outputBytes, err := command.CombinedOutput()
	if err == nil {
		test.Fatalf("expected discovery failure status 42, output:\n%s", string(outputBytes))
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		test.Fatalf("expected discovery exit status, got %T: %v", err, err)
	}
	output := string(outputBytes)
	if exitError.ExitCode() != 42 {
		test.Fatalf("expected discovery exit 42 to propagate, got %d, output:\n%s", exitError.ExitCode(), output)
	}
	if !strings.Contains(output, "PASSING_PACKAGE_STREAMED") {
		test.Fatalf("expected the package before the discovery failure to pass, output:\n%s", output)
	}
	if !strings.Contains(output, "forced discovery failure for fail package") {
		test.Fatalf("expected discovery diagnostic to remain visible, output:\n%s", output)
	}
	if strings.Contains(output, "skipping workload package github.com/smackerel/smackerel/tests/stress/fail") {
		test.Fatalf("discovery failure must not become selector no-match, output:\n%s", output)
	}
	if strings.Contains(output, "selector matched zero stress packages") {
		test.Fatalf("discovery failure must remain the root failure, output:\n%s", output)
	}
	assertGoStressCaptureDirectoryEmpty(test, captureDir)
}

func TestGoStressHarness_JSONWarningAndQuotedLiteralClassification(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	cleanOutput := "=== RUN   TestOutputContract\n--- PASS: TestOutputContract (0.00s)\nPASS"
	testCases := []struct {
		name           string
		workloadOutput string
		wantExit       int
		wantClass      string
	}{
		{
			name:           "compact JSON warning rejected",
			workloadOutput: `{"level":"WARN","message":"degraded"}`,
			wantExit:       1,
			wantClass:      "runtime-warning-json",
		},
		{
			name:           "JSON warning after another field rejected",
			workloadOutput: `{"time":"2026-09-02T00:00:00Z","level":"WARN","message":"degraded"}`,
			wantExit:       1,
			wantClass:      "runtime-warning-json",
		},
		{
			name:           "quoted logfmt warning literal accepted",
			workloadOutput: `time=2026-09-02T00:00:00Z level=INFO msg="example level=WARN is not a runtime warning"`,
		},
		{
			name:           "quoted timestamp warning literal accepted",
			workloadOutput: `time=2026-09-02T00:00:00Z level=INFO msg="example 2026-09-02T00:00:01Z WARN is quoted"`,
		},
		{
			name:           "escaped JSON warning literal accepted",
			workloadOutput: `{"level":"INFO","message":"example {\"level\":\"WARN\"} is quoted"}`,
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			output, exitCode := runGoStressOutputContractFixture(test, repoRoot, cleanOutput, testCase.workloadOutput)
			if exitCode != testCase.wantExit {
				test.Fatalf("expected exit %d, got %d, output:\n%s", testCase.wantExit, exitCode, output)
			}
			if testCase.wantClass == "" {
				if strings.Contains(output, "forbidden output class") {
					test.Fatalf("quoted warning literal must remain benign, output:\n%s", output)
				}
				return
			}
			if !strings.Contains(output, "forbidden output class "+testCase.wantClass) {
				test.Fatalf("expected actionable %s error, output:\n%s", testCase.wantClass, output)
			}
		})
	}
}

func TestGoStressHarness_ClassifierCommandFailurePropagates(test *testing.T) {
	testGoStressHarnessClassifierCommandFailurePropagates(test)
}

func testGoStressHarnessClassifierCommandFailurePropagates(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	realGrepPath, err := exec.LookPath("grep")
	if err != nil {
		test.Fatalf("resolve real grep: %v", err)
	}
	test.Setenv("REAL_GREP", realGrepPath)
	prependFakeExecutable(test, "grep", `#!/usr/bin/env bash
set -euo pipefail
target=""
for argument in "$@"; do
	target="$argument"
done
if [[ -r "$target" ]]; then
	while IFS= read -r line || [[ -n "$line" ]]; do
		case "$line" in
			*CLASSIFIER_FAILURE_STREAMED*)
				echo "forced grep classifier failure" >&2
				exit 44
				;;
		esac
	done < "$target"
fi
exec "${REAL_GREP:?}" "$@"
`)

	cleanOutput := "=== RUN   TestOutputContract\n--- PASS: TestOutputContract (0.00s)\nPASS"
	output, exitCode := runGoStressOutputContractFixture(test, repoRoot, cleanOutput, cleanOutput+"\nCLASSIFIER_FAILURE_STREAMED")
	if exitCode == 0 {
		test.Fatalf("expected classifier command failure, output:\n%s", output)
	}
	if !strings.Contains(output, "forced grep classifier failure") {
		test.Fatalf("expected grep failure diagnostic to remain visible, output:\n%s", output)
	}
	if !strings.Contains(output, "output classification could not be trusted") || !strings.Contains(output, "grep exit 44") {
		test.Fatalf("expected actionable classifier failure with grep status, output:\n%s", output)
	}
}

func TestGoStressHarness_CaptureFailurePropagates(test *testing.T) {
	testGoStressHarnessCaptureFailurePropagates(test)
}

func testGoStressHarnessCaptureFailurePropagates(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	prependFakeExecutable(test, "grep", `#!/usr/bin/env bash
set -euo pipefail
echo "CLASSIFIER_RAN_AFTER_CAPTURE_FAILURE" >&2
exit 46
`)
	prependFakeExecutable(test, "tee", `#!/usr/bin/env bash
set -euo pipefail
while IFS= read -r line || [[ -n "$line" ]]; do
	printf '%s\n' "$line"
done
echo "forced tee capture failure" >&2
exit 45
`)

	cleanOutput := "=== RUN   TestOutputContract\n--- PASS: TestOutputContract (0.00s)\nPASS"
	output, exitCode := runGoStressOutputContractFixture(test, repoRoot, cleanOutput, cleanOutput)
	if exitCode == 0 {
		test.Fatalf("expected capture writer failure, output:\n%s", output)
	}
	if !strings.Contains(output, "forced tee capture failure") || !strings.Contains(output, "output capture failed (tee exit 45)") {
		test.Fatalf("expected actionable capture failure with tee status, output:\n%s", output)
	}
	if strings.Contains(output, "CLASSIFIER_RAN_AFTER_CAPTURE_FAILURE") {
		test.Fatalf("classification must not run against incomplete output, output:\n%s", output)
	}
}

func TestGoStressHarness_FailurePathsCleanCaptureFiles(test *testing.T) {
	test.Run("list failure", testGoStressHarnessListFailurePropagates)
	test.Run("classifier failure", testGoStressHarnessClassifierCommandFailurePropagates)
	test.Run("capture failure", testGoStressHarnessCaptureFailurePropagates)
	test.Run("forbidden output", func(test *testing.T) {
		repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
		if err != nil {
			test.Fatalf("resolve repo root: %v", err)
		}
		cleanOutput := "=== RUN   TestOutputContract\n--- PASS: TestOutputContract (0.00s)\nPASS"
		_, exitCode := runGoStressOutputContractFixture(test, repoRoot, cleanOutput, "=== RUN   TestOutputContract\n--- SKIP: TestOutputContract (0.00s)")
		if exitCode == 0 {
			test.Fatal("expected forbidden output failure")
		}
	})
	test.Run("workload failure", TestGoStressHarness_WorkloadFailurePropagatesAfterCanary)
}

func TestGoStressHarness_ZeroSkipZeroRuntimeWarningContract(test *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		test.Fatalf("resolve repo root: %v", err)
	}
	cleanOutput := "=== RUN   TestOutputContract\n--- PASS: TestOutputContract (0.00s)\nPASS"
	testCases := []struct {
		name           string
		canaryOutput   string
		workloadOutput string
		wantExit       int
		wantClass      string
		wantPackage    string
		wantVisible    string
	}{
		{
			name:           "clean output accepted",
			canaryOutput:   cleanOutput,
			workloadOutput: cleanOutput + "\nCLEAN_OUTPUT_STREAMED",
			wantVisible:    "CLEAN_OUTPUT_STREAMED",
		},
		{
			name:           "go test skip rejected",
			canaryOutput:   cleanOutput,
			workloadOutput: "=== RUN   TestOutputContract\n--- SKIP: TestOutputContract (0.00s)\nSKIP_OUTPUT_STREAMED",
			wantExit:       1,
			wantClass:      "go-test-skip",
			wantPackage:    "github.com/smackerel/smackerel/tests/stress",
			wantVisible:    "SKIP_OUTPUT_STREAMED",
		},
		{
			name:           "level warning rejected",
			canaryOutput:   cleanOutput,
			workloadOutput: "time=2026-08-31T12:00:00Z level=WARN msg=degraded\nLEVEL_WARNING_STREAMED\nPASS",
			wantExit:       1,
			wantClass:      "runtime-warning-level",
			wantPackage:    "github.com/smackerel/smackerel/tests/stress",
			wantVisible:    "LEVEL_WARNING_STREAMED",
		},
		{
			name:           "timestamp warning rejected",
			canaryOutput:   cleanOutput,
			workloadOutput: "2026-08-31T12:00:00.123Z WARN degraded runtime\nTIMESTAMP_WARNING_STREAMED\nPASS",
			wantExit:       1,
			wantClass:      "runtime-warning-timestamp",
			wantPackage:    "github.com/smackerel/smackerel/tests/stress",
			wantVisible:    "TIMESTAMP_WARNING_STREAMED",
		},
		{
			name:           "benign no tests warning accepted",
			canaryOutput:   cleanOutput,
			workloadOutput: "testing: warning: no tests to run\nBENIGN_WARNING_STREAMED\nPASS",
			wantVisible:    "BENIGN_WARNING_STREAMED",
		},
		{
			name:           "readiness canary skip rejected",
			canaryOutput:   "=== RUN   TestStressReadinessCanary_Live\n--- SKIP: TestStressReadinessCanary_Live (0.00s)\nCANARY_SKIP_STREAMED",
			workloadOutput: cleanOutput,
			wantExit:       1,
			wantClass:      "go-test-skip",
			wantPackage:    "./tests/stress/readiness",
			wantVisible:    "CANARY_SKIP_STREAMED",
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			output, exitCode := runGoStressOutputContractFixture(test, repoRoot, testCase.canaryOutput, testCase.workloadOutput)
			if exitCode != testCase.wantExit {
				test.Fatalf("expected exit %d, got %d, output:\n%s", testCase.wantExit, exitCode, output)
			}
			if !strings.Contains(output, testCase.wantVisible) {
				test.Fatalf("expected complete test output to stream marker %q, output:\n%s", testCase.wantVisible, output)
			}
			if testCase.wantClass == "" {
				if strings.Contains(output, "forbidden output class") {
					test.Fatalf("expected output to be accepted, output:\n%s", output)
				}
				return
			}
			if !strings.Contains(output, "forbidden output class "+testCase.wantClass) {
				test.Fatalf("expected actionable %s error, output:\n%s", testCase.wantClass, output)
			}
			if !strings.Contains(output, "package "+testCase.wantPackage) {
				test.Fatalf("expected error to name package %s, output:\n%s", testCase.wantPackage, output)
			}
		})
	}
}

func runGoStressOutputContractFixture(test *testing.T, repoRoot string, canaryOutput string, workloadOutput string) (string, int) {
	test.Helper()
	fakeBinDir := test.TempDir()
	fakeGoPath := filepath.Join(fakeBinDir, "go")
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
	"list -tags stress ./tests/stress/..."*)
		echo "github.com/smackerel/smackerel/tests/stress"
		exit 0
		;;
	*"^TestStressReadinessCanary_Live$"*)
		printf '%s\n' "${FAKE_CANARY_OUTPUT:?}"
		exit 0
		;;
	*"-list TestOutputContract github.com/smackerel/smackerel/tests/stress"*)
		echo "TestOutputContract"
		exit 0
		;;
	*"github.com/smackerel/smackerel/tests/stress"*)
		printf '%s\n' "${FAKE_WORKLOAD_OUTPUT:?}"
		exit 0
		;;
	*)
		echo "unexpected fake-go args: $*"
		exit 43
		;;
esac
`
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		test.Fatalf("write fake go: %v", err)
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "runtime", "go-stress.sh")
	captureDir := test.TempDir()
	command := exec.Command("bash", scriptPath, "--run", "TestOutputContract")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SMACKEREL_STRESS_WORKSPACE="+test.TempDir(),
		"TMPDIR="+captureDir,
		"CORE_EXTERNAL_URL=http://stress-core.invalid",
		"DATABASE_URL=postgres://stress-user:testpass@stress-db.invalid/stress?sslmode=disable",
		"NATS_URL=nats://stress-nats.invalid:4222",
		"SMACKEREL_AUTH_TOKEN=stress-auth-token-for-output-contract",
		"FAKE_CANARY_OUTPUT="+canaryOutput,
		"FAKE_WORKLOAD_OUTPUT="+workloadOutput,
	)
	outputBytes, err := command.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			test.Fatalf("execute go-stress output fixture: %v, output:\n%s", err, string(outputBytes))
		}
		exitCode = exitError.ExitCode()
	}
	assertGoStressCaptureDirectoryEmpty(test, captureDir)
	return string(outputBytes), exitCode
}

func assertGoStressCaptureDirectoryEmpty(test *testing.T, captureDir string) {
	test.Helper()
	entries, err := os.ReadDir(captureDir)
	if err != nil {
		test.Fatalf("read go-stress capture directory: %v", err)
	}
	if len(entries) != 0 {
		test.Fatalf("expected go-stress capture cleanup, found %d entries including %s", len(entries), entries[0].Name())
	}
}

func prependFakeExecutable(test *testing.T, name string, contents string) {
	test.Helper()
	fakeBinDir := test.TempDir()
	executablePath := filepath.Join(fakeBinDir, name)
	if err := os.WriteFile(executablePath, []byte(contents), 0o755); err != nil {
		test.Fatalf("write fake %s: %v", name, err)
	}
	test.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func validConfig() Config {
	return Config{
		CoreURL:     "http://stress-core.example",
		DatabaseURL: "postgres://stress-user:stress-pass@stress-db.example/stress?sslmode=disable",
		NATSURL:     "nats://stress-nats.example:4222",
		AuthToken:   "stress-auth-token",
	}
}

func healthyTopologyBody() string {
	return `{"status":"healthy","services":{"postgres":{"status":"up"},"nats":{"status":"up"}}}`
}

func staticHTTPClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer stress-auth-token" {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
