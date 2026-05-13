// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// This test file extends the deploy compose contract with spec 045
// resource envelope invariants. It is in the same `package deploy` as
// compose_contract_test.go and re-uses repoRoot() and the composeDoc
// shape from that file (extended here with deploy.resources.limits).
//
// The contract enforced by spec 045 FR-045-001:
//
//   1. Every service in the contract set (postgres, nats, smackerel-core,
//      smackerel-ml, ollama) declares BOTH `cpus` and `memory` under
//      `deploy.resources.limits`.
//
//   2. BOTH values use the fail-loud SST substitution form
//      `${SERVICE_<KIND>_LIMIT:?<message>}`. Hardcoded literals
//      (`memory: 1G`) are FORBIDDEN — they would silently drift from the
//      SST source of truth in config/smackerel.yaml.
//
//   3. The `${VAR:-default}` form is FORBIDDEN per Gate G028 (NO-DEFAULTS /
//      fail-loud SST). Only the `${VAR:?error}` form is permitted, so
//      compose aborts at start time with the named error if the deploy
//      adapter forgets to emit the env var into app.env.
//
// Three adversarial sub-tests prove the contract function would FAIL if
// any future edit regresses the contract:
//   - TestComposeResourceContract_AdversarialMissingCPU: cpus block omitted
//   - TestComposeResourceContract_AdversarialMissingMemory: memory block omitted
//   - TestComposeResourceContract_AdversarialHardcodedLiteral: literal "1G" instead of substitution
//
// Cross-reference:
//   - specs/045-deploy-resource-filesystem-hardening/spec.md  FR-045-001
//   - specs/045-deploy-resource-filesystem-hardening/scenario-manifest.json SCN-045-A01
//   - config/smackerel.yaml deploy_resources.* (SST source of truth)
//   - scripts/commands/config.sh (env emission)

package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeResourceDoc captures the deploy.resources.limits block for every
// service. It is intentionally minimal — adding unrelated fields stays
// a non-event. The Limits.Cpus and Limits.Memory fields are strings (not
// numbers) because they hold compose substitution syntax like
// `${POSTGRES_CPU_LIMIT:?...}` rather than literal numeric values.
type composeResourceDoc struct {
	Services map[string]struct {
		Deploy struct {
			Resources struct {
				Limits struct {
					Cpus   string `yaml:"cpus"`
					Memory string `yaml:"memory"`
				} `yaml:"limits"`
			} `yaml:"resources"`
		} `yaml:"deploy"`
	} `yaml:"services"`
}

// servicesUnderResourceContract is the canonical set of services that
// MUST declare both cpus and memory limits per spec 045 FR-045-001.
// Adding a future service requires extending this list AND the
// deploy_resources block in config/smackerel.yaml AND the env emission
// in scripts/commands/config.sh AND the substitution in
// deploy/compose.deploy.yml. The build-time test makes the contract
// boundary mechanical.
var servicesUnderResourceContract = []struct {
	service   string
	envCPUVar string // env var name expected in the cpus substitution
	envMemVar string // env var name expected in the memory substitution
}{
	{"postgres", "POSTGRES_CPU_LIMIT", "POSTGRES_MEMORY_LIMIT"},
	{"nats", "NATS_CPU_LIMIT", "NATS_MEMORY_LIMIT"},
	{"smackerel-core", "CORE_CPU_LIMIT", "CORE_MEMORY_LIMIT"},
	{"smackerel-ml", "ML_CPU_LIMIT", "ML_MEMORY_LIMIT"},
	{"ollama", "OLLAMA_CPU_LIMIT", "OLLAMA_MEMORY_LIMIT"},
}

// assertResourceContract returns nil iff every service in the contract
// set declares BOTH cpus and memory under deploy.resources.limits, AND
// each value uses the fail-loud `${VAR:?...}` substitution form pointing
// at the canonical env var name. Returns a non-nil error naming the
// specific service and violation on any breach.
func assertResourceContract(yamlBytes []byte) error {
	var doc composeResourceDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	for _, contract := range servicesUnderResourceContract {
		svc, ok := doc.Services[contract.service]
		if !ok {
			return fmt.Errorf("contract violation: services.%s not found in compose document", contract.service)
		}
		cpus := svc.Deploy.Resources.Limits.Cpus
		memory := svc.Deploy.Resources.Limits.Memory

		if cpus == "" {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.cpus is missing — spec 045 FR-045-001 requires every service to declare a CPU limit (expected `cpus: \"${%s:?...}\"`)", contract.service, contract.envCPUVar)
		}
		if memory == "" {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.memory is missing — spec 045 FR-045-001 requires every service to declare a memory limit (expected `memory: \"${%s:?...}\"`)", contract.service, contract.envMemVar)
		}

		// The fail-loud SST form is the ONLY accepted form per Gate G028.
		// `${VAR:-default}` (default fallback) is FORBIDDEN.
		// Hardcoded literals (e.g. `1G`) are FORBIDDEN.
		expectedCPUPrefix := "${" + contract.envCPUVar + ":?"
		expectedMemPrefix := "${" + contract.envMemVar + ":?"

		if !strings.HasPrefix(cpus, expectedCPUPrefix) {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.cpus=%q does not use the fail-loud SST form %s...} — Gate G028 NO-DEFAULTS forbids hardcoded literals AND `${VAR:-default}` fallbacks; the deploy adapter MUST emit %s in app.env or compose aborts at start time", contract.service, cpus, expectedCPUPrefix, contract.envCPUVar)
		}
		if !strings.HasPrefix(memory, expectedMemPrefix) {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.memory=%q does not use the fail-loud SST form %s...} — Gate G028 NO-DEFAULTS forbids hardcoded literals AND `${VAR:-default}` fallbacks; the deploy adapter MUST emit %s in app.env or compose aborts at start time", contract.service, memory, expectedMemPrefix, contract.envMemVar)
		}

		// Belt-and-braces: explicitly reject the `${VAR:-default}` form.
		// The HasPrefix check above already rejects it implicitly because
		// the `:?` and `:-` differ at byte 1 of the substitution form,
		// but naming the violation explicitly improves the error message.
		if strings.Contains(cpus, ":-") {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.cpus=%q uses the FORBIDDEN `${VAR:-default}` fallback form — Gate G028 requires fail-loud `${VAR:?error}`", contract.service, cpus)
		}
		if strings.Contains(memory, ":-") {
			return fmt.Errorf("contract violation: services.%s.deploy.resources.limits.memory=%q uses the FORBIDDEN `${VAR:-default}` fallback form — Gate G028 requires fail-loud `${VAR:?error}`", contract.service, memory)
		}
	}

	return nil
}

// TestComposeResourceContract_LiveFile is the primary spec 045 FR-045-001
// assertion. It loads the live deploy/compose.deploy.yml and proves the
// resource envelope contract holds for every service in the set.
func TestComposeResourceContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertResourceContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 045 FR-045-001 resource envelope contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml satisfies spec 045 FR-045-001 (every service in {postgres, nats, smackerel-core, smackerel-ml, ollama} declares both cpus and memory under deploy.resources.limits using the fail-loud ${VAR:?...} SST form)")
}

// TestComposeResourceContract_AdversarialMissingCPU proves the contract
// catches a regression where a service's `cpus` line is omitted.
// SCN-045-A01 — fact-of-life: A future edit might revert to memory-only
// limits, leaving CPU unbounded; this test is the build-time guard.
func TestComposeResourceContract_AdversarialMissingCPU(t *testing.T) {
	const fixture = `services:
  postgres:
    deploy:
      resources:
        limits:
          memory: "${POSTGRES_MEMORY_LIMIT:?POSTGRES_MEMORY_LIMIT must be set by deploy adapter}"
  nats:
    deploy:
      resources:
        limits:
          cpus: "${NATS_CPU_LIMIT:?NATS_CPU_LIMIT must be set by deploy adapter}"
          memory: "${NATS_MEMORY_LIMIT:?NATS_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-core:
    deploy:
      resources:
        limits:
          cpus: "${CORE_CPU_LIMIT:?CORE_CPU_LIMIT must be set by deploy adapter}"
          memory: "${CORE_MEMORY_LIMIT:?CORE_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-ml:
    deploy:
      resources:
        limits:
          cpus: "${ML_CPU_LIMIT:?ML_CPU_LIMIT must be set by deploy adapter}"
          memory: "${ML_MEMORY_LIMIT:?ML_MEMORY_LIMIT must be set by deploy adapter}"
  ollama:
    deploy:
      resources:
        limits:
          cpus: "${OLLAMA_CPU_LIMIT:?OLLAMA_CPU_LIMIT must be set by deploy adapter}"
          memory: "${OLLAMA_MEMORY_LIMIT:?OLLAMA_MEMORY_LIMIT must be set by deploy adapter}"
`
	err := assertResourceContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: postgres without cpus block was accepted (the contract is tautological — it would NOT catch a regression that drops the cpus limit)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	if !strings.Contains(err.Error(), "cpus is missing") {
		t.Fatalf("adversarial contract test failed: error did not mention 'cpus is missing': %v", err)
	}
	t.Logf("adversarial OK: missing cpus on postgres is rejected with: %v", err)
}

// TestComposeResourceContract_AdversarialMissingMemory proves the contract
// catches a regression where a service's `memory` line is omitted.
// SCN-045-A01 — fact-of-life: same risk as missing cpus, opposite axis.
func TestComposeResourceContract_AdversarialMissingMemory(t *testing.T) {
	const fixture = `services:
  postgres:
    deploy:
      resources:
        limits:
          cpus: "${POSTGRES_CPU_LIMIT:?POSTGRES_CPU_LIMIT must be set by deploy adapter}"
          memory: "${POSTGRES_MEMORY_LIMIT:?POSTGRES_MEMORY_LIMIT must be set by deploy adapter}"
  nats:
    deploy:
      resources:
        limits:
          cpus: "${NATS_CPU_LIMIT:?NATS_CPU_LIMIT must be set by deploy adapter}"
          memory: "${NATS_MEMORY_LIMIT:?NATS_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-core:
    deploy:
      resources:
        limits:
          cpus: "${CORE_CPU_LIMIT:?CORE_CPU_LIMIT must be set by deploy adapter}"
          memory: "${CORE_MEMORY_LIMIT:?CORE_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-ml:
    deploy:
      resources:
        limits:
          cpus: "${ML_CPU_LIMIT:?ML_CPU_LIMIT must be set by deploy adapter}"
  ollama:
    deploy:
      resources:
        limits:
          cpus: "${OLLAMA_CPU_LIMIT:?OLLAMA_CPU_LIMIT must be set by deploy adapter}"
          memory: "${OLLAMA_MEMORY_LIMIT:?OLLAMA_MEMORY_LIMIT must be set by deploy adapter}"
`
	err := assertResourceContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: smackerel-ml without memory block was accepted")
	}
	if !strings.Contains(err.Error(), "smackerel-ml") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-ml': %v", err)
	}
	if !strings.Contains(err.Error(), "memory is missing") {
		t.Fatalf("adversarial contract test failed: error did not mention 'memory is missing': %v", err)
	}
	t.Logf("adversarial OK: missing memory on smackerel-ml is rejected with: %v", err)
}

// TestComposeResourceContract_AdversarialHardcodedLiteral proves the contract
// catches a regression where a substitution is replaced with a hardcoded
// literal (the spec 020 / pre-spec-045 form). This is the most likely
// regression mode because copy-paste from older compose files would
// reintroduce the literal form.
func TestComposeResourceContract_AdversarialHardcodedLiteral(t *testing.T) {
	const fixture = `services:
  postgres:
    deploy:
      resources:
        limits:
          cpus: "${POSTGRES_CPU_LIMIT:?POSTGRES_CPU_LIMIT must be set by deploy adapter}"
          memory: 1G
  nats:
    deploy:
      resources:
        limits:
          cpus: "${NATS_CPU_LIMIT:?NATS_CPU_LIMIT must be set by deploy adapter}"
          memory: "${NATS_MEMORY_LIMIT:?NATS_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-core:
    deploy:
      resources:
        limits:
          cpus: "${CORE_CPU_LIMIT:?CORE_CPU_LIMIT must be set by deploy adapter}"
          memory: "${CORE_MEMORY_LIMIT:?CORE_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-ml:
    deploy:
      resources:
        limits:
          cpus: "${ML_CPU_LIMIT:?ML_CPU_LIMIT must be set by deploy adapter}"
          memory: "${ML_MEMORY_LIMIT:?ML_MEMORY_LIMIT must be set by deploy adapter}"
  ollama:
    deploy:
      resources:
        limits:
          cpus: "${OLLAMA_CPU_LIMIT:?OLLAMA_CPU_LIMIT must be set by deploy adapter}"
          memory: "${OLLAMA_MEMORY_LIMIT:?OLLAMA_MEMORY_LIMIT must be set by deploy adapter}"
`
	err := assertResourceContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: hardcoded literal `memory: 1G` on postgres was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 form)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	if !strings.Contains(err.Error(), "POSTGRES_MEMORY_LIMIT") {
		t.Fatalf("adversarial contract test failed: error did not name the missing env var POSTGRES_MEMORY_LIMIT: %v", err)
	}
	t.Logf("adversarial OK: hardcoded literal on postgres memory is rejected with: %v", err)
}

// TestComposeResourceContract_AdversarialDefaultFallback proves the
// contract catches a regression where the FORBIDDEN `${VAR:-default}`
// fallback form is used instead of the fail-loud `${VAR:?error}` form.
// Gate G028 requires fail-loud — a default fallback would silently
// hide a missing env var and let production deploy with the wrong sizing.
func TestComposeResourceContract_AdversarialDefaultFallback(t *testing.T) {
	const fixture = `services:
  postgres:
    deploy:
      resources:
        limits:
          cpus: "${POSTGRES_CPU_LIMIT:-1.0}"
          memory: "${POSTGRES_MEMORY_LIMIT:?POSTGRES_MEMORY_LIMIT must be set by deploy adapter}"
  nats:
    deploy:
      resources:
        limits:
          cpus: "${NATS_CPU_LIMIT:?NATS_CPU_LIMIT must be set by deploy adapter}"
          memory: "${NATS_MEMORY_LIMIT:?NATS_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-core:
    deploy:
      resources:
        limits:
          cpus: "${CORE_CPU_LIMIT:?CORE_CPU_LIMIT must be set by deploy adapter}"
          memory: "${CORE_MEMORY_LIMIT:?CORE_MEMORY_LIMIT must be set by deploy adapter}"
  smackerel-ml:
    deploy:
      resources:
        limits:
          cpus: "${ML_CPU_LIMIT:?ML_CPU_LIMIT must be set by deploy adapter}"
          memory: "${ML_MEMORY_LIMIT:?ML_MEMORY_LIMIT must be set by deploy adapter}"
  ollama:
    deploy:
      resources:
        limits:
          cpus: "${OLLAMA_CPU_LIMIT:?OLLAMA_CPU_LIMIT must be set by deploy adapter}"
          memory: "${OLLAMA_MEMORY_LIMIT:?OLLAMA_MEMORY_LIMIT must be set by deploy adapter}"
`
	err := assertResourceContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: ${POSTGRES_CPU_LIMIT:-1.0} default fallback was accepted (Gate G028 NO-DEFAULTS bypassed)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	t.Logf("adversarial OK: ${VAR:-default} fallback on postgres cpus is rejected with: %v", err)
}
