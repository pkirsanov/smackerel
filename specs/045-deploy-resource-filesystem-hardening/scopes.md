# Scopes: Deploy Resource and Filesystem Hardening

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Resource and ML model envelope contract

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-045-A01 Operator sees bounded service resources
  Given Smackerel is prepared for deployment
  When the deploy contract or generated runtime config is inspected
  Then core, ML, postgres, nats, and ollama services have explicit CPU and memory envelopes
  And those values originate from the SST configuration pipeline (config/smackerel.yaml)
  And no hardcoded resource literal remains in deploy/compose.deploy.yml

Scenario: SCN-045-A02 ML model selection fits the memory envelope
  Given the operator configures llm.ollama_model and services.ml.model_memory_profiles
  When config validation runs at Go core startup
  Then the configured model is accepted only if its profile memory <= ML_MEMORY_LIMIT
  And incompatible model choices fail loudly with a named error before runtime start
  And missing model_memory_profiles entries for any used model fail loudly
```

### Implementation Plan

1. Add `deploy_resources.<service>` block to `config/smackerel.yaml` for postgres, nats, smackerel_core, smackerel_ml, ollama (cpus + memory each).
2. Add `services.ml.model_memory_profiles` map covering all configured ml/llm models.
3. Wire `scripts/commands/config.sh` to `required_value` each `deploy_resources.*` key and emit `<SERVICE>_CPU_LIMIT` / `<SERVICE>_MEMORY_LIMIT` env vars.
4. Wire `scripts/commands/config.sh` to extract `services.ml.model_memory_profiles` as JSON (`ML_MODEL_MEMORY_PROFILES_JSON`) and the active `ML_MEMORY_LIMIT`.
5. Substitute env vars into every `deploy.resources.limits.{cpus,memory}` block in `deploy/compose.deploy.yml`. Remove all hardcoded literals.
6. Add Go config fields (`MLMemoryLimitMiB int`, `MLModelMemoryProfiles map[string]int`) to `internal/config/config.go` and a `validateMLModelEnvelope()` method called from `Validate()`.
7. Extend `internal/deploy/compose_contract_test.go` with `assertResourceContract()` enforcing every contract service has non-empty cpus + memory limits. Include adversarial sub-tests proving regression detection.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-045-001 | unit/config | `internal/config/validate_test.go` | SCN-045-A01 | `Load()` fails when `ML_MEMORY_LIMIT` env var is missing. |
| T-045-002 | contract | `internal/deploy/compose_contract_test.go` | SCN-045-A01 | Live `deploy/compose.deploy.yml` has `deploy.resources.limits.cpus` AND `memory` populated for every contract service via `${VAR}` substitution. |
| T-045-003 | unit/config | `internal/config/validate_test.go` | SCN-045-A02 | `Validate()` rejects ML model whose required memory exceeds `ML_MEMORY_LIMIT`; rejects unknown model with no profile entry. |
| T-045-006a | adversarial | `internal/deploy/compose_contract_test.go` | SCN-045-A01 | Mutated compose fixture missing `cpus` limit FAILS the contract; mutated fixture missing `memory` limit FAILS. |

### Definition of Done

- [ ] SCN-045-A01: `config/smackerel.yaml` contains `deploy_resources.<service>.{cpus,memory}` for postgres, nats, smackerel_core, smackerel_ml, ollama (operator sees bounded service resources).
- [ ] SCN-045-A01: `scripts/commands/config.sh` extracts every new `deploy_resources.*` key via `required_value` and emits the env vars into `config/generated/<env>.env` (originates from the SST configuration pipeline).
- [ ] SCN-045-A01: `deploy/compose.deploy.yml` substitutes `${SERVICE_CPU_LIMIT:?...}` and `${SERVICE_MEMORY_LIMIT:?...}` into every `deploy.resources.limits` block; zero hardcoded resource literals remain (no hardcoded resource literal remains in deploy/compose.deploy.yml).
- [ ] SCN-045-A01: T-045-002 passes — live compose file has populated cpus + memory limit fields for every contract service via `${VAR}` substitution.
- [ ] SCN-045-A01: T-045-006a passes — adversarial mutated compose fixtures missing `cpus` or `memory` FAIL the contract assertion (proves test would catch regression).
- [ ] SCN-045-A02: `config/smackerel.yaml` contains `services.ml.model_memory_profiles` for every ml/llm model name referenced elsewhere in the file (operator configures llm.ollama_model and services.ml.model_memory_profiles).
- [ ] SCN-045-A02: `internal/config/config.go` parses `ML_MEMORY_LIMIT` and `ML_MODEL_MEMORY_PROFILES_JSON`; `Validate()` rejects ML model whose required memory exceeds the envelope with a fail-loud error naming the model + required + envelope MiB (incompatible model choices fail loudly with a named error before runtime start).
- [ ] SCN-045-A02: T-045-001 passes — missing `ML_MEMORY_LIMIT` causes Go core `Load()` to return an error naming the env var.
- [ ] SCN-045-A02: T-045-003 passes — `Validate()` rejects oversized ML model AND rejects models with no profile entry (missing model_memory_profiles entries for any used model fail loudly).
- [ ] `./smackerel.sh test unit --go` passes for `internal/config/...` and `internal/deploy/...`.
- [ ] `./smackerel.sh config generate --env dev` succeeds and the generated env file contains every new SST key.

## Scope 2: Read-only root filesystem contract

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-045-A03 Container roots are read-only except explicit mounts
  Given the runtime stack is generated
  When deploy/compose.deploy.yml is inspected
  Then smackerel-core, smackerel-ml, and ollama declare read_only: true
  And postgres and nats do NOT declare read_only: true (their data dirs need writes)
  And every read-only service declares its writable paths via tmpfs in an allowlist
  And the contract test would FAIL if any read-only service introduced an unauthorized writable mount

Scenario: SCN-045-A04 Hardened stack still passes live health checks
  Given resource limits and read-only root are applied
  When ./smackerel.sh up brings the stack up under the hardened compose
  Then every service container starts and reaches its healthcheck
  And /api/health returns 200 within the wait timeout
```

### Implementation Plan

1. Add `read_only: true` to smackerel-core, smackerel-ml, ollama in `deploy/compose.deploy.yml`.
2. Add `tmpfs:` blocks for required writable paths per the design doc table (smackerel-core: /tmp 64M; smackerel-ml: /tmp 768M with HF_HOME and SENTENCE_TRANSFORMERS_HOME env overrides pointing under /tmp; ollama: /tmp + /.ollama_tmp).
3. Verify postgres + nats remain unchanged (no read_only flag, named volumes intact).
4. Extend `internal/deploy/compose_contract_test.go` with `assertFilesystemContract()` enforcing: read-only allowlist, postgres/nats exemption, tmpfs allowlist match. Include adversarial sub-tests.
5. Mirror `read_only` + tmpfs into `docker-compose.yml` (dev) so the dev stack also runs hardened — this keeps spec 020 docker_security_test.go aligned and surfaces issues before deploy.
6. Run `./smackerel.sh up` smoke against dev stack and capture `/api/health` evidence.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-045-004 | contract | `internal/deploy/compose_contract_test.go` | SCN-045-A03 | smackerel-core, smackerel-ml, ollama all have `read_only: true`; postgres + nats do NOT. |
| T-045-005 | contract | `internal/deploy/compose_contract_test.go` | SCN-045-A03 | Read-only services' tmpfs entries match the documented allowlist. |
| T-045-006b | adversarial | `internal/deploy/compose_contract_test.go` | SCN-045-A03 | Mutated fixture: smackerel-core missing read_only → FAIL; postgres with read_only → FAIL; smackerel-ml with unauthorized tmpfs → FAIL. |
| T-045-007 | live-stack smoke | manual via `./smackerel.sh up` + curl | SCN-045-A04 | After `./smackerel.sh up`, `curl http://127.0.0.1:<CORE_HOST_PORT>/api/health` returns 200. |
| T-045-008 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] SCN-045-A03: `deploy/compose.deploy.yml`: smackerel-core, smackerel-ml, ollama have `read_only: true` AND explicit `tmpfs:` mounts per the design doc table (smackerel-core, smackerel-ml, and ollama declare read_only: true; required writable directories are backed by explicit tmpfs).
- [ ] SCN-045-A03: `deploy/compose.deploy.yml`: postgres + nats do NOT have `read_only: true` (postgres and nats do NOT declare read_only: true — their data dirs need writes).
- [ ] SCN-045-A03: `docker-compose.yml`: smackerel-core, smackerel-ml, ollama mirror the same `read_only: true` + tmpfs blocks so dev stack runs hardened (every read-only service declares its writable paths via tmpfs in an allowlist).
- [ ] SCN-045-A03: `internal/deploy/compose_contract_test.go` `assertFilesystemContract()` enforces read-only allowlist + tmpfs allowlist + postgres/nats exemption.
- [ ] SCN-045-A03: T-045-004 passes — live compose file matches the read-only allowlist (smackerel-core, smackerel-ml, ollama all have `read_only: true`; postgres + nats do NOT).
- [ ] SCN-045-A03: T-045-005 passes — tmpfs entries match the documented allowlist for each read-only service.
- [ ] SCN-045-A03: T-045-006b passes — adversarial mutated fixtures (missing read_only, postgres set read_only, unauthorized writable mount) FAIL the contract assertion (proves the contract test would FAIL if any read-only service introduced an unauthorized writable mount).
- [ ] SCN-045-A04: T-045-007 passes — live-stack smoke proves `/api/health` returns 200 with hardened limits applied (every service container starts and reaches its healthcheck; /api/health returns 200 within the wait timeout).
- [ ] T-045-008 passes: `bash .github/bubbles/scripts/artifact-lint.sh specs/045-deploy-resource-filesystem-hardening` exits 0.
- [ ] `./smackerel.sh test unit --go` passes for `internal/config/...` and `internal/deploy/...` after Scope 2 changes.
