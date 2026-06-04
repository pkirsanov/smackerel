package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConsumerEnvKeysEmittedInGeneratedEnv (T-081-U6 / D01-2) asserts
// the SST plumbing for the spec 081 consumer contract:
//
//   - config/smackerel.yaml declares infrastructure.nats.consumer with
//     both max_deliver and ack_wait_seconds keys.
//   - scripts/commands/config.sh reads both keys via required_value and
//     emits NATS_CONSUMER_MAX_DELIVER / NATS_CONSUMER_ACK_WAIT_SECONDS
//     into the generated env file.
//
// We assert against the source files directly (not the generated env)
// so the test does not depend on a runtime shell, yq, or docker being
// available inside the go-test container. Live emission is covered by
// the live-stack integration test driven by `./smackerel.sh test
// integration` (T-081-I1).
func TestConsumerEnvKeysEmittedInGeneratedEnv(t *testing.T) {
	repoRoot := findRepoRoot081(t)

	yaml := mustRead081(t, filepath.Join(repoRoot, "config", "smackerel.yaml"))
	for _, fragment := range []string{
		"consumer:",
		"max_deliver:",
		"ack_wait_seconds:",
	} {
		if !strings.Contains(yaml, fragment) {
			t.Errorf("config/smackerel.yaml missing %q (spec 081 FR-081-001 / FR-081-002)", fragment)
		}
	}

	sh := mustRead081(t, filepath.Join(repoRoot, "scripts", "commands", "config.sh"))
	for _, fragment := range []string{
		`NATS_CONSUMER_MAX_DELIVER="$(required_value infrastructure.nats.consumer.max_deliver)"`,
		`NATS_CONSUMER_ACK_WAIT_SECONDS="$(required_value infrastructure.nats.consumer.ack_wait_seconds)"`,
		`NATS_CONSUMER_MAX_DELIVER=${NATS_CONSUMER_MAX_DELIVER}`,
		`NATS_CONSUMER_ACK_WAIT_SECONDS=${NATS_CONSUMER_ACK_WAIT_SECONDS}`,
	} {
		if !strings.Contains(sh, fragment) {
			t.Errorf("scripts/commands/config.sh missing fragment %q (spec 081 D01-2)", fragment)
		}
	}
}

func mustRead081(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func findRepoRoot081(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}
