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
	scriptPath := filepath.Join(repoRoot, "scripts", "runtime", "go-stress.sh")
	command := exec.Command("bash", scriptPath, "--run", "TestForcedWorkloadFailure")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SMACKEREL_STRESS_WORKSPACE="+workspaceDir,
		"CORE_EXTERNAL_URL=http://stress-core.invalid",
		"DATABASE_URL=postgres://stress-user:stress-pass@stress-db.invalid/stress?sslmode=disable",
		"NATS_URL=nats://stress-nats.invalid:4222",
		"SMACKEREL_AUTH_TOKEN=stress-auth-token-for-script-test",
	)
	outputBytes, err := command.CombinedOutput()
	if err == nil {
		test.Fatalf("expected workload command failure, output:\n%s", string(outputBytes))
	}
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
