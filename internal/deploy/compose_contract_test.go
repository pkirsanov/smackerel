// Package deploy contains static-file invariant tests for the deployment
// compose contract enforced by spec 042 (Tailnet-Edge Bind Pattern).
//
// The contract:
//
//  1. The smackerel-core service publishes its host port using the prefix
//     "${HOST_BIND_ADDRESS:-127.0.0.1}:" so a deploy adapter can override
//     the bind address for tailnet-edge fronting (default keeps loopback).
//  2. The smackerel-ml service publishes its host port using the same prefix.
//  3. The postgres service publishes NO host port — DevOps reaches it via
//     `tailscale ssh <host> -- docker exec -it <container> psql ...`
//     (Pattern P1).
//  4. The nats service publishes NO host port — same Pattern P1 access.
//
// These invariants live in deploy/compose.deploy.yml. This test parses that
// file with gopkg.in/yaml.v3 and asserts the four conditions hold. Two
// adversarial sub-tests guarantee the contract function would FAIL if
// either invariant regressed (proves the test is not tautological).
//
// References:
//   - specs/042-tailnet-edge-bind-pattern/spec.md
//   - specs/042-tailnet-edge-bind-pattern/design.md
//   - bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md (canonical pattern)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeDoc is the minimal YAML shape this test needs to assert the
// contract. It intentionally does NOT model every field of compose.deploy.yml
// so that adding unrelated services or fields stays a non-event.
type composeDoc struct {
	Services map[string]struct {
		// Ports is left as []string because compose port entries can be
		// declared as either short-form strings ("HOST:CONT") or long-form
		// objects, and the contract uses short-form throughout. If a future
		// service migrates to long-form ports, this test will fail loudly
		// for that service and the contract assertion can be extended.
		Ports []string `yaml:"ports"`
	} `yaml:"services"`
}

const (
	requiredCorePrefix = `${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:`
	requiredMLPrefix   = `${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:`
)

// repoRoot returns the repository root by climbing two directories up from
// the directory containing this test file (internal/deploy/ -> repo root).
// Using runtime.Caller makes the path independent of `go test` CWD, which
// makes the test work both from `cd internal/deploy && go test` and from
// `cd /workspace && go test ./...` (the path used by go-unit.sh).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// assertComposeContract returns nil iff the four invariants hold for the
// compose document encoded in yamlBytes. On any violation it returns a
// non-nil error naming the specific service and the specific violation, so
// the adversarial sub-tests can pattern-match the failure mode.
func assertComposeContract(yamlBytes []byte) error {
	var doc composeDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	core, ok := doc.Services["smackerel-core"]
	if !ok {
		return fmt.Errorf("contract violation: services.smackerel-core not found in compose document")
	}
	if len(core.Ports) == 0 {
		return fmt.Errorf("contract violation: services.smackerel-core.ports is empty (expected one entry with prefix %q)", requiredCorePrefix)
	}
	if !strings.HasPrefix(core.Ports[0], requiredCorePrefix) {
		return fmt.Errorf("contract violation: services.smackerel-core.ports[0]=%q does not start with required prefix %q (literal 127.0.0.1: prefix is the spec 020 form and is forbidden by spec 042)", core.Ports[0], requiredCorePrefix)
	}

	ml, ok := doc.Services["smackerel-ml"]
	if !ok {
		return fmt.Errorf("contract violation: services.smackerel-ml not found in compose document")
	}
	if len(ml.Ports) == 0 {
		return fmt.Errorf("contract violation: services.smackerel-ml.ports is empty (expected one entry with prefix %q)", requiredMLPrefix)
	}
	if !strings.HasPrefix(ml.Ports[0], requiredMLPrefix) {
		return fmt.Errorf("contract violation: services.smackerel-ml.ports[0]=%q does not start with required prefix %q (literal 127.0.0.1: prefix is the spec 020 form and is forbidden by spec 042)", ml.Ports[0], requiredMLPrefix)
	}

	if pg, ok := doc.Services["postgres"]; ok {
		if len(pg.Ports) > 0 {
			return fmt.Errorf("contract violation: services.postgres.ports is non-empty (got %v) — postgres must have NO host port mapping per spec 042 (Pattern P1: tailscale ssh + docker exec)", pg.Ports)
		}
	}

	if n, ok := doc.Services["nats"]; ok {
		if len(n.Ports) > 0 {
			return fmt.Errorf("contract violation: services.nats.ports is non-empty (got %v) — nats must have NO host port mapping per spec 042 (Pattern P1: tailscale ssh + docker exec)", n.Ports)
		}
	}

	return nil
}

// TestComposeContract_LiveFile is the primary contract assertion. It loads
// the live deploy/compose.deploy.yml from the repo root and proves the
// four invariants hold. This is the test that would FAIL if any future
// edit regresses the contract.
func TestComposeContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertComposeContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 042 tailnet-edge bind contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml satisfies spec 042 (backend ports use ${HOST_BIND_ADDRESS:-127.0.0.1}:; postgres and nats have no host ports)")
}

// TestComposeContract_AdversarialLiteralBind proves the contract function
// catches a regression to the spec 020 hardcoded form. It feeds the
// function a fixture identical in shape to the live file except that the
// smackerel-core port prefix is the literal "127.0.0.1:". The contract
// MUST return a non-nil error mentioning "smackerel-core" and the literal
// prefix being forbidden. This sub-test is the adversarial regression
// guarantee that the live-file contract assertion is not tautological.
func TestComposeContract_AdversarialLiteralBind(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: literal 127.0.0.1: prefix on smackerel-core was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 form)")
	}
	if !strings.Contains(err.Error(), "smackerel-core") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-core': %v", err)
	}
	if !strings.Contains(err.Error(), "spec 020") {
		t.Fatalf("adversarial contract test failed: error did not mention 'spec 020' (the regression target the test guards against): %v", err)
	}
	t.Logf("adversarial OK: literal 127.0.0.1: prefix on smackerel-core is rejected with: %v", err)
}

// TestComposeContract_AdversarialInfraHasPorts proves the contract function
// catches a regression where postgres re-acquires a host port mapping. It
// feeds the function a fixture where postgres has a ports block. The
// contract MUST return a non-nil error mentioning "postgres" and Pattern
// P1. This sub-test is the adversarial regression guarantee for the infra
// no-host-port invariant.
func TestComposeContract_AdversarialInfraHasPorts(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres:
    ports:
      - "127.0.0.1:5432:5432"
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: postgres ports block was accepted (the contract is tautological — it would NOT catch a regression that re-publishes a host port for postgres)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	if !strings.Contains(err.Error(), "Pattern P1") {
		t.Fatalf("adversarial contract test failed: error did not mention 'Pattern P1' (the prescribed access path for infra services): %v", err)
	}
	t.Logf("adversarial OK: postgres ports block is rejected with: %v", err)
}
