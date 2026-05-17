# Bug Fix Design: BUG-045-002 — Chronic CI integration failure persists despite BUG-045-001

> **Authoritative inputs:** [spec.md](spec.md) (6 ACs), [report.md](report.md) (RCA evidence + Evidence 6/7 captured this phase), [state.json](state.json) (version 3 control plane).
>
> **HEAD at design completion:** `5c8d857e80a07f59600f51b9e9bce906814a6311`, 2026-05-16.
>
> **Phase 1 → Phase 2 transition:** `bubbles.bug` produced the discovery skeleton; this revision is the `bubbles.design` Phase 2 output. The AC-2 gate has been resolved by a **CI-environment local reproduction** (Step 3 below) rather than authenticated `gh` artifact download — both methods produce equivalent verbatim failing-test attribution, and the local reproduction is in fact a *stronger* form of evidence because it proves the root-cause class is reproducible (not just observed). The AC-2 Uncertainty Declaration in [spec.md](spec.md) is RESOLVED by [report.md § Evidence 6/7](report.md).

## Status

`ACTIVE — design phase complete. Ready for bubbles.plan handoff.`

## Design Brief

### Current State

The `integration` job in [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) (lines 110–268) ships a deliberately narrow service topology: GitHub Actions `services: postgres:` container + a single `docker run nats` sidecar. It then invokes `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m` with default test-binary parallelism. The local `./smackerel.sh test integration` command ([smackerel.sh](../../../../smackerel.sh) lines 687–723) takes a different path: it brings up the FULL Compose stack (`postgres + nats + ollama + smackerel-core + smackerel-ml`) via [tests/integration/test_runtime_health.sh](../../../../tests/integration/test_runtime_health.sh) (which asserts `services.ml_sidecar.status == "up"` before proceeding), then runs Go tests via [scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh) with `go test -p 1 …`. That divergence is the single cause of the 20-consecutive-failure chronic-CI pattern.

### Target State

The CI `integration` job invokes `./smackerel.sh test integration` against the same disposable test-environment Compose project the local orchestrator uses. The GitHub Actions `services:` block and the `docker run nats` sidecar are removed (they conflict with the Compose lifecycle the CLI owns). A build-time guard test in `internal/deploy/` parses [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml), finds the `integration` job, and fail-loudly rejects any future workflow revision that (a) re-introduces a GH `services:` block on this job, (b) re-introduces an inline `docker run` sidecar for `postgres`/`nats`/`ollama` on this job, or (c) invokes a raw `go test` command targeting `./tests/integration/...` instead of routing through `./smackerel.sh test integration`. Result: spec-031 live-stack-testing contract is restored end-to-end; the 20-consecutive-failure chronic-CI pattern is broken at root.

### Patterns to Follow

- **[smackerel.sh](../../../../smackerel.sh) lines 687–723 (`integration)` case)** — canonical full-stack lifecycle: `smackerel_generate_config test` → `KEEP_STACK_UP=1 test_runtime_health.sh` (asserts ml_sidecar healthy) → `docker run --rm --network host … golang:1.25.10-bookworm bash /workspace/scripts/runtime/go-integration.sh` → trap-driven `down --volumes` cleanup. The CI job should invoke this verbatim, not re-implement it.
- **[.github/workflows/build.yml](../../../../.github/workflows/build.yml)** — image build with cosign sign / SBOM / SLSA attestation. CI's `build` job already produces `smackerel-core` + `smackerel-ml` images. The `integration` job currently does NOT consume those built images; Path A inherits whatever the local Compose build produces, which is consistent with `./smackerel.sh` behaviour. Image-reuse optimisation is out of scope for this packet (separate optimisation spec, future work — see OQ-1).
- **[internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go)** — canonical "parse YAML + assert contract + adversarial sub-tests" pattern for the AC-4 build-time guard. The guard test for the workflow YAML mirrors this file's shape: open the file, parse with `gopkg.in/yaml.v3`, walk the structure, fail with a specific error message naming the offending field.
- **[internal/deploy/build_workflow_vuln_gate_contract_test.go](../../../../internal/deploy/build_workflow_vuln_gate_contract_test.go)** — concrete prior art for workflow-YAML contract guarding in this repo. The AC-4 guard SHOULD live alongside this file (`internal/deploy/ci_integration_topology_contract_test.go`) so all workflow-YAML guards are co-located.

### Patterns to Avoid

- **The current CI integration job's "GH service container + docker-run nats sidecar" pattern.** It made sense when `./smackerel.sh test integration` did not exist as a stable CLI surface, but it has been the cause of 20 consecutive `main` failures. The header comment at [ci.yml line 268](../../../../.github/workflows/ci.yml) ("CI integration uses raw go test because GitHub Actions service containers replace the Docker Compose stack that ./smackerel.sh test integration manages. The CLI command expects to bring up/down its own Compose project, which conflicts with Actions service lifecycle.") describes a real concern, but the resolution is to STOP using the service container — NOT to keep the divergent path that never reaches parity.
- **The "BUG-045-001 done with known drift" pattern.** BUG-045-001 was certified `done, passed-with-known-drift` at HEAD `5c8d857e` on the basis of local-only `./smackerel.sh test integration` exit 0, despite its `spec.md` AC-1 explicitly requiring "chronic CI integration failure on `main` resolves to green". The certification was structurally wrong: an AC that requires CI evidence cannot be discharged by local evidence alone. This packet exists because of that wrong pattern. Do not repeat it: this packet's `bubbles.validate` phase MUST capture the post-fix-HEAD CI run JSON inline, not just local exit 0.
- **`t.Skip()` to silence the failing tests.** Forbidden by the integration test suite's "no skips, fail loudly" contract (referenced in [tests/integration/ollama_healthcheck_test.go lines 36–37](../../../../tests/integration/ollama_healthcheck_test.go) and [tests/integration/ollama_image_availability_test.go line 23](../../../../tests/integration/ollama_image_availability_test.go)). Skip is Option C; Option C is rejected by user mandate.
- **Lowering the CI integration timeout to "make the failure window smaller".** Forbidden by the discovery skeleton's risk register. The fix is the topology change, not the timeout.

### Resolved Decisions

- **DD-1** — **Path A (Parity)**: CI runs `./smackerel.sh test integration` against the full Compose stack. Rationale + spec citations in § Decision DD-1 below.
- **DD-2** — **AC-4 guard test location**: `internal/deploy/ci_integration_topology_contract_test.go`, alongside `compose_contract_test.go` and `build_workflow_vuln_gate_contract_test.go`. Co-location keeps every workflow-YAML guard discoverable in one directory.
- **DD-3** — **CI timeout bump**: `timeout-minutes: 15 → 30` for the `integration` job. Justified by the cold-cache Ollama image pull (~4 GB) + test model pull (~3 GB for `gemma3:4b`) + Compose build of `smackerel-core`/`smackerel-ml`. Build-time caching is a future optimisation, not a blocker for this fix.
- **DD-4** — **No partial test partition**: every test in `tests/integration/...` runs under the full-stack path. Build-tag partition (Path B) is rejected because it requires ongoing classification discipline that has no enforcement mechanism (the AC-4 guard cannot tell whether a new test "needs" the full stack until it fails in CI — which is the failure mode we are eliminating).
- **DD-5** — **`-p 1` (sequential test-binary execution) is preserved by routing through `./smackerel.sh test integration`** (which calls `scripts/runtime/go-integration.sh` whose body is `go test -p 1 …`). Two of the 5 verified CI failures (TestKnowledgeStats and TestDriveScanFixture) only manifest under the default GH-Actions test-binary parallelism (≥2). Path A inherits `-p 1` for free.

### Open Questions

- **OQ-1 (deferred, not blocking)** — Should the CI `build` job's `smackerel-core` + `smackerel-ml` images be pushed to a job-scoped registry tag and pulled by the `integration` job instead of rebuilt in-Compose? This would shave ~3–5 minutes from the integration job. Deferred to a future optimisation spec because it requires a registry-credential change to the integration job and would interact with the spec-047 vulnerability gate (gated images need re-verification when pulled into a downstream job). **Not a blocker for this packet.**

---

## Root Cause (verified attribution)

The Phase 1 Uncertainty Declaration is RESOLVED by [report.md § Evidence 6](report.md). A **CI-environment local reproduction** ran the exact CI service topology (postgres + nats only, no ollama / smackerel-core / smackerel-ml) and the exact CI test command (`go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m`) against HEAD `5c8d857e`. The reproduction produced 5 verified failing tests:

| # | Failing Test (verbatim) | Verbatim Error (verbatim) | Root-Cause Class | Service / Setting Missing in CI |
|---|---|---|---|---|
| 1 | `TestKnowledgeStats_EmptyStoreReturnsZeroValues` | `knowledge_stats_test.go:40: SynthesisPending = 2, want 0` | Test-isolation defect that ONLY manifests when test binaries run in parallel (no `-p 1`) | The `-p 1` flag, which the local runner sets in [scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh) and CI omits. |
| 2 | `TestPhotosContractCanary_ConfigNATSDBAndMLAgree/ml_sidecar_photos_contract_response` | `photos_contract_canary_test.go:50: wait for photos.classified canary response: nats: timeout` | Missing service | `smackerel-ml` sidecar (subscribes to `photos.classify`, publishes to `photos.classified`). Not present in CI. |
| 3 | `TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList` | `drive_connectors_endpoint_test.go:63: GET http://127.0.0.1:45001/v1/connectors/drive: dial tcp 127.0.0.1:45001: connect: connection refused (live test stack must be up via ./smackerel.sh test integration)` | Missing service | `smackerel-core` HTTP API on `CORE_HOST_PORT`. Not present in CI. The test's error message ALREADY names the fix: "must be up via ./smackerel.sh test integration" — i.e. the test was designed for Path A and broke under the CI's divergent path. |
| 4 | `TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/nats_DRIVE_stream_in_jetstream` | `drive_foundation_canary_test.go:172: DRIVE stream lookup: nats: API error: code=404 err_code=10059 description=stream not found (must be created by Go EnsureStreams on startup)` | Missing service | `smackerel-core` is the process that calls `EnsureStreams` on boot, creating the DRIVE JetStream stream. CI's NATS sidecar has JetStream enabled but no consumer ever calls EnsureStreams. |
| 5 | `TestDriveScanFixturePreservesHierarchyAndMetadata` | `drive_scan_fixture_test.go:40: drive_files count = 227, want 1200` | Test-isolation defect under parallel test-binary execution | Same as #1 — race condition between `tests/integration/` and `tests/integration/drive/` packages contending for the shared `drive_files` table when `go test` uses default `-p` (= GOMAXPROCS = 2-4 on GH Actions runners). |

Cause classes mapped to the local-vs-CI gap:

- **3 of 5 failures (60%)** are caused directly by missing services that the local Compose stack provides: `smackerel-ml` (test 2), `smackerel-core` (tests 3 and 4).
- **2 of 5 failures (40%)** are caused by the missing `-p 1` flag (tests 1 and 5) — a flag the local runner sets but CI omits.

Both cause classes share the same root: CI bypasses the canonical `./smackerel.sh test integration` entrypoint and rebuilds its own divergent test orchestration that omits BOTH the full Compose stack AND the `-p 1` flag. The fix collapses both classes into a single change: route CI through the canonical CLI.

The fact that BUG-045-001's validator-routing fix (a real bug; correctly closed) did NOT change the CI failure state proves the CI failure has no causal relationship to the validator chain — confirming the BUG-045-001 close-out Uncertainty Declaration was justified and that the actual root cause is environmental, not source-level.

---

## Architecture overview

### Services delta — verified

| Service / Setting | Local (`./smackerel.sh test integration`) | CI (current `integration` job) | Required by which tests (verified)? |
|---|---|---|---|
| `postgres` (pgvector/pg16) | ✓ Compose container `smackerel-test-postgres` | ✓ GH `services:` block | All tests using `pgxpool.Pool` |
| `nats` (2.10-alpine, JetStream + auth) | ✓ Compose container `smackerel-test-nats` | ✓ `docker run` sidecar `nats-ci` | All tests using `*nats.Conn` |
| `ollama` (0.23.2) | ✓ Compose container `smackerel-test-ollama` | ✗ **MISSING** | `TestPhotosContractCanary_…/ml_sidecar_photos_contract_response` (transitively, via ml-sidecar) |
| `smackerel-ml` (FastAPI sidecar) | ✓ Compose container `smackerel-test-ml-1` | ✗ **MISSING** | `TestPhotosContractCanary_…/ml_sidecar_photos_contract_response`; any test that publishes to `photoz.classify` and expects a `photos.classified` response |
| `smackerel-core` (Go runtime) | ✓ Compose container `smackerel-test-core-1` | ✗ **MISSING** | `TestDriveConnectorsEndpoint_…` (needs HTTP API), `TestDriveFoundationCanary_…/nats_DRIVE_stream_in_jetstream` (needs EnsureStreams on boot) |
| `go test -p 1` (sequential test binaries) | ✓ Set in [scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh) | ✗ **MISSING** (default `-p` = GOMAXPROCS) | `TestKnowledgeStats_…`, `TestDriveScanFixture_…` (race against parallel test packages) |

Source-of-truth references:

- Local stack lifecycle: [smackerel.sh:687-723](../../../../smackerel.sh) (`integration)` case in the `test()` function)
- Local health probe (asserts `ml_sidecar.status == "up"`): [tests/integration/test_runtime_health.sh:51-58](../../../../tests/integration/test_runtime_health.sh)
- Local test invocation: [scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh) (`go test -p 1 -tags integration -v -count=1 -timeout 300s ./tests/integration/...`)
- CI service block: [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) `jobs.integration.services.postgres` (lines ~117-127)
- CI NATS sidecar: [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) step `Start NATS with auth and JetStream` (lines ~135-144)
- CI test invocation: [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) step `Run integration tests` (lines ~244-252)

### Target topology

After Path A lands, the CI `integration` job collapses to four functional steps (excluding setup-go / checkout / cleanup):

1. **Generate config bundles** — `./smackerel.sh config generate && ./smackerel.sh config generate --env test`. Pre-step required because the integration test pre-amble reads `config/generated/test.env`.
2. **Run integration tests via canonical CLI** — `./smackerel.sh test integration`. The CLI handles: full-stack bring-up via `smackerel.sh --env test build && smackerel.sh --env test up`, health-probe with `KEEP_STACK_UP=1`, sequential `go test -p 1`, trap-driven teardown.
3. **Capture log artifact** — `tee integration-test.log` already wired by the existing CI step; KEEP it (only the underlying test-invocation step changes, the log capture stays).
4. **Fail-job-on-test-failure aggregator** — the existing `Fail job if integration tests failed` pattern KEEPS because `continue-on-error: true` on the test step is still needed for the artifact upload to run on failure.

Compared to current CI:

- **REMOVED**: `jobs.integration.services.postgres` block
- **REMOVED**: `Start NATS with auth and JetStream` step
- **REMOVED**: `Apply database migrations via db.Migrate` step (the local CLI runs migrations transitively via `smackerel-core`'s startup)
- **CHANGED**: `Run integration tests` step's body from `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m 2>&1 | tee integration-test.log` to `./smackerel.sh test integration 2>&1 | tee integration-test.log`
- **CHANGED**: `timeout-minutes: 15 → 30` (DD-3)
- **PRESERVED**: `Generate SST config files for integration tests`, `Upload integration test log`, `Fail job if integration tests failed`

---

## Decision DD-1 — Path A (Parity): CI invokes `./smackerel.sh test integration`

### Decision

The CI `integration` job is restructured to invoke `./smackerel.sh test integration` against a full Compose stack brought up by the same CLI. The GH `services:` container and the `docker run nats` sidecar are removed. The job timeout is raised to 30 minutes to absorb the cold-cache Ollama image + model pull.

### Rationale (grounded in verbatim AC-2 evidence)

| Reason | Backing evidence |
|---|---|
| **Path A eliminates ALL 5 verified failures with a single change.** Tests 2, 3, 4 are fixed by full-stack bring-up; tests 1 and 5 are fixed by inheriting `-p 1` from the canonical CLI. No test partitioning, no skip annotations, no per-test refactor. | [report.md § Evidence 6](report.md) verbatim failing-test list + this design's § Root Cause services-delta table. |
| **Spec 031 (live-stack-testing) requires it.** Per [specs/031-live-stack-testing/spec.md](../../../../specs/031-live-stack-testing/spec.md) and reinforced in [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) → "Live-Stack Test Authenticity": *"Tests labeled `integration`, `e2e-api`, `e2e-ui`, or otherwise described as live-stack MUST hit the real running system."* The CI integration job today violates that contract — every test that depends on `smackerel-core` / `smackerel-ml` / `ollama` is silently running without those services. Path A restores compliance. | [specs/031-live-stack-testing/spec.md](../../../../specs/031-live-stack-testing/spec.md) + [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) "Live-Stack Test Authenticity" section. |
| **Spec 023 (engineering-quality) "Adversarial Regression Tests for Bug Fixes" mandates an adversarial test that fails if the bug is reintroduced.** Path A enables AC-4: a contract test that parses the CI workflow YAML and rejects any future revision that re-introduces the divergent topology. Path B (partition) cannot offer an equivalent guard because it cannot statically decide whether a new test "needs" the full stack — the guard would only catch violators after they fail in CI, which is the failure mode we are eliminating. | [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) → "Adversarial Regression Tests For Bug Fixes" section; this design's § Test Strategy below. |
| **[.github/skills/bubbles-test-environment-isolation/SKILL.md](../../../../.github/skills/bubbles-test-environment-isolation/SKILL.md) is honoured.** Path A uses `./smackerel.sh --env test up` which the orchestrator brings up under the Compose project `smackerel-test-…` (isolated from the persistent dev project `smackerel-dev-…`). The trap-driven `down --volumes` teardown discards all state at job end. No test-environment-isolation regression. | [.github/skills/bubbles-test-environment-isolation/SKILL.md](../../../../.github/skills/bubbles-test-environment-isolation/SKILL.md) ephemeral-only-state contract; [smackerel.sh:687-723](../../../../smackerel.sh) trap-driven cleanup. |
| **The cited "GH Actions service-container conflict" is resolved by removing the service container.** The current workflow header comment at [ci.yml](../../../../.github/workflows/ci.yml) ("the CLI command expects to bring up/down its own Compose project, which conflicts with Actions service lifecycle") accurately describes the conflict, BUT the resolution is to STOP using the GH service container — not to keep the divergent path that never reaches local parity. Path A removes the conflict entirely. | This design's § Architecture overview "REMOVED" list. |

### Rejected alternatives (with rationale)

- **Path B (build-tag partition)** — Add `//go:build integration && local_stack` to tests requiring `smackerel-core`/`smackerel-ml`/`ollama`; CI runs only the topology-friendly subset; nightly self-hosted workflow runs the full set.

  **Rejected because:** (a) classification discipline has no enforcement mechanism — a new integration test that quietly depends on `smackerel-core` would land in the CI-runnable subset (no build tag) and quietly degrade coverage; the AC-4 guard cannot statically detect this. (b) Tests 1 and 5 (the `-p 1` failures) are NOT fixed by partition — they still race under default parallelism unless the CI command also adds `-p 1`, which is what Path A does for free. (c) Creates a NEW failure mode: nightly tier-2 runs decoupled from PR feedback; regressions in tier-2 tests would only surface on the next nightly. (d) The proposed alternative path described in the bug spec ("if `e2e-api` already runs the full stack, **Option B** is the cleanest long-term fix because it leverages existing infrastructure") is contingent on an `e2e-api` job that **does not exist** in this repo. Verified: [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) contains only `lint-and-test`, `build`, `integration` jobs; [.github/workflows/build.yml](../../../../.github/workflows/build.yml) contains only `build-images`, `build-bundles`, `publish-build-manifest`. Path B would require creating a new full-stack job — which is essentially Path A with a partition surface bolted on for no functional benefit.

- **Path C (`t.Skip("requires full stack")` on the failing tests)** — Forbidden by user mandate "long-term solutions, no shortcuts" and by the integration suite's "no skips, fail loudly" contract enforced in [tests/integration/ollama_healthcheck_test.go lines 36-37](../../../../tests/integration/ollama_healthcheck_test.go). Skip hides coverage gaps without fixing them; the next test added that depends on the full stack would silently fail in CI again. **Hard-rejected.**

---

## Test Strategy (AC-4 build-time guard)

### Guard test contract

A new contract test in `internal/deploy/ci_integration_topology_contract_test.go` parses [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) at runtime using `gopkg.in/yaml.v3` and asserts:

| Assertion | Failure surface |
|---|---|
| `jobs.integration` exists | If absent, the workflow was restructured and this guard MUST be reviewed; fail with explicit "integration job not found" message. |
| `jobs.integration.services` is absent or empty | If present, asserts the file regressed to the spec-020-style GH service-container pattern; fail with verbatim name of the offending service. |
| No step in `jobs.integration.steps` matches the regex `docker\s+run\s+.*(postgres\|nats\|ollama)` | If matched, asserts the file regressed to the spec-020-style inline-sidecar pattern; fail with the offending step name + line number. |
| At least one step in `jobs.integration.steps` invokes `./smackerel.sh test integration` (matching regex `\./smackerel\.sh\s+test\s+integration`) | If absent, asserts the CI lost the canonical-CLI invocation; fail with a list of what the steps DO run. |
| No step in `jobs.integration.steps` invokes raw `go test -tags=integration` for tests in `./tests/integration/…` | If present, asserts the workflow regressed to the divergent path; fail with the offending step's run-block excerpt. |
| `jobs.integration.timeout-minutes` is `>= 30` | If lower, the cold-cache run will time out before Ollama model pull completes; fail with the current value + the minimum required. |

### Adversarial sub-tests (per [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) "Adversarial Regression Tests For Bug Fixes")

The guard test MUST include three adversarial sub-tests, each running the same assertion helper against a synthetic YAML fixture:

1. **`TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock`** — synthetic CI YAML with a `jobs.integration.services.postgres:` block; assertion MUST return a violation naming "postgres" and citing this packet's BUG-045-002 spec path.
2. **`TestCIIntegrationTopology_AdversarialRejectsDockerRunNatsSidecar`** — synthetic CI YAML whose `jobs.integration.steps` contains a `run: docker run -d --name nats-ci --network host nats …` step; assertion MUST return a violation naming the offending step and citing this packet.
3. **`TestCIIntegrationTopology_AdversarialRejectsRawGoTest`** — synthetic CI YAML whose `jobs.integration.steps` contains `run: go test -tags=integration ./tests/integration/...`; assertion MUST return a violation naming the divergent test command and citing this packet.

Each adversarial test asserts the violation message names BOTH (a) the offending field/line AND (b) the canonical alternative (`./smackerel.sh test integration`). This satisfies the "guidance on the failure" pattern that [internal/deploy/build_workflow_vuln_gate_contract_test.go](../../../../internal/deploy/build_workflow_vuln_gate_contract_test.go) already uses for the spec-047 Trivy gate guard.

### Bailout-pattern audit (per [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md))

The guard test file MUST contain ZERO instances of:

- `t.Skip(`, `t.SkipNow(`, `t.Skipf(`
- Early-return on missing fixture file (a missing live CI workflow YAML is a fail-loud condition, not a skip)
- `if … return` bailouts on the negation of the assertion (would silent-pass on a regression)

The Scope 2 DoD includes a grep guard: `grep -nE 't\.Skip\|return\s*$' internal/deploy/ci_integration_topology_contract_test.go` MUST produce no matches.

### Scenario-to-test mapping

| AC / SCN | Test |
|---|---|
| AC-1 (CI integration job green on main) | Post-fix-HEAD CI run JSON capture (`bubbles.validate`). Verifiable via `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/runs/<FIX_RUN_ID>/jobs` JSON inline. |
| AC-2 (verbatim failing-test attribution) | RESOLVED by [report.md § Evidence 6](report.md) (local CI-environment reproduction). |
| AC-3 (service-topology contract) | DD-1 records Path A; AC-4 guard enforces it. |
| AC-4 (build-time guard) | `internal/deploy/ci_integration_topology_contract_test.go` (6 live assertions + 3 adversarial sub-tests). |
| AC-5 (chronic-failure pattern broken) | Post-fix-HEAD curl of `actions/workflows/ci.yml/runs?branch=main&per_page=3` after 3 main pushes; 3/3 conclusion=success. |
| AC-6 (BUG-045-001 close-out reconciled) | This packet's `bubbles.validate` adds `subsequentResolutions` field to BUG-045-001's `state.json`. |
| SCN-045-002-A (authenticated gh CLI captures failing-test name) | RESOLVED equivalently by local CI-environment reproduction; see [report.md § Evidence 6](report.md). |
| SCN-045-002-B (failing test reaches for CI-absent service) | RESOLVED by [report.md § Evidence 7](report.md) — every failing test's source line + the absent service it reaches for is captured. |
| SCN-045-002-C (fix-HEAD CI integration green) | Post-fix `bubbles.validate` evidence. |
| SCN-045-002-D (3-consecutive-success pattern) | Post-fix `bubbles.validate` evidence after 3 main pushes. |
| SCN-045-002-E (adversarial guard rejects regression) | Three adversarial sub-tests in the AC-4 guard file (see above). |

---

## Configuration

| File | Change | Justification |
|---|---|---|
| [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) | Remove `jobs.integration.services.postgres` block; remove `Start NATS with auth and JetStream` step; remove `Apply database migrations via db.Migrate` step; change `Run integration tests` step body from raw `go test` to `./smackerel.sh test integration 2>&1 \| tee integration-test.log`; raise `timeout-minutes: 15 → 30`. | Path A core change. The `Generate SST config files for integration tests` step stays (CI needs it for the dev.env generation that happens before any `--env test` call). The migration step is removed because `./smackerel.sh test integration` runs migrations transitively via `smackerel-core`'s startup. |
| `internal/deploy/ci_integration_topology_contract_test.go` (NEW) | AC-4 guard test file. | Build-time contract per DD-2. |

No config-file changes in `config/smackerel.yaml`, no Compose changes in `docker-compose.yml`, no source changes outside the workflow + new guard file. No data migration. No schema changes.

---

## Migration / rollout

This is a CI infrastructure change. Rollout is a single commit landing on `main`:

1. Commit the workflow change + the new guard test in one PR (so the guard ships at the same moment the topology changes).
2. The first post-fix `main` CI run validates the change (`bubbles.validate` Phase 5 captures it).
3. The next 2 main pushes after the fix HEAD validate the chronic-failure-pattern break (AC-5).
4. No data migration, no schema migration, no service restart, no operator action.

No staged rollout (CI is a single environment). No feature flag (workflow YAML can't be feature-flagged sanely). No canary (the guard test catches the regression class before merge).

---

## Observability / failure handling

The existing `Upload integration test log` step STAYS. The log capture continues to work because `./smackerel.sh test integration 2>&1 | tee integration-test.log` preserves the same `tee` pattern. Future operators investigating a CI failure download the artifact via `gh run download <RUN_ID> -n integration-test-log` (the same path BUG-045-002's spec.md AC-2 cited).

The CI cold-cache failure modes are predictable:

- **Ollama image pull failure** → manifests as `docker compose up` exit non-zero with a clear "manifest unknown" or "TLS handshake" error in the stack-up phase. Caught by the existing `test_runtime_health.sh` health probe (60s timeout); job fails fast with the verbatim docker error in the test log.
- **Ollama model pull timeout** → manifests as `services.ml_sidecar.status != "up"` in the health probe; job fails at the health-probe step with the verbatim probe error.
- **Compose build failure** → manifests as a smackerel-core or smackerel-ml build error before stack-up; job fails at the `--env test build` step with the verbatim build error.

In all three cases, the failure is loud, the error is verbatim, and the log artifact is captured. There is no silent-degradation surface.

---

## Risks

| ID | Risk | Impact | Mitigation |
|---|---|---|---|
| R-1 | Cold-cache CI runs may approach or exceed the new 30-minute timeout because Ollama image (~4 GB) + test model (~3 GB) + Compose build of `smackerel-core`/`smackerel-ml` are all downloaded/built on every job. | Job times out; chronic-failure pattern persists for a different reason. | Phase 1 mitigation: DD-3 raises timeout to 30 min. Phase 2 mitigation (deferred, OQ-1): pull pre-built images from the `build` job's GHCR push, and cache the Ollama model. The 30-min ceiling is generous enough to absorb the cold-cache case for now. The guard test fail-loudly rejects a future revision that lowers the timeout below 30. |
| R-2 | Spec 031 (live-stack-testing) restructure (in flight per the spec's roadmap) may collide with this change if both touch the CI integration job in overlapping ways. | Merge conflict; one of the two specs has to rebase. | Spec 031 work happens in a separate spec branch; this packet's workflow change is small (≤30 lines of diff) and easy to rebase. Coordination via spec cross-reference in the PR description. |
| R-3 | Spec 052 (Bundle secret injection contract) restructured the CI build workflow surface; if any of that work spills into `ci.yml` (rather than `build.yml`), it may collide. | Merge conflict. | Mitigation: keep the diff small; rebase. Spec 052 has been landing through `build.yml` so far per recent commit history; collision risk is low. |
| R-4 | The AC-4 guard test fixture format (synthetic YAML) may drift from the live `ci.yml` format if a future GitHub Actions schema change is adopted. | Adversarial sub-tests no longer mirror real CI structure; guard becomes less effective. | The live assertion ALWAYS parses the real `ci.yml`, so any schema drift in the live file produces a fail-loud error at the live assertion before the synthetic adversarial tests run. The synthetic fixtures are intentionally minimal — only the fields the guard inspects are present — minimising drift surface. |
| R-5 | A future test that NEEDS the full stack but is not classified explicitly may still pass under the AC-4 guard because the guard only enforces the WORKFLOW shape, not per-test classification. | Coverage gap; an in-CI failure would still surface but not be caught at PR review. | Acceptable: per the DD-4 rationale, Path A's full-stack default means every new integration test runs against the full stack — there is no "wrong tier" to land in. The guard's job is to prevent the workflow from regressing AWAY from full-stack default, not to classify individual tests. |
| R-6 | The 5 verified failing tests (especially #1 `TestKnowledgeStats` and #5 `TestDriveScanFixture`) may have latent test-isolation defects that Path A's `-p 1` flag MASKS rather than fixes. A future operator who tries `go test -p 2` on the integration suite will see the same race condition. | Latent defect; not a regression of the chronic CI pattern but a separate maintenance debt. | Documented in [report.md § Evidence 7](report.md) and flagged as a follow-on bug candidate. Out of scope for this packet (this packet's contract is "CI integration green on main", not "all integration tests pass with arbitrary `-p` values"). |

---

## Rollback Strategy

If the post-fix CI run is GREEN on the fix HEAD but RED on the next push (i.e. the chronic-failure pattern returns under Path A for a NEW reason), rollback is a single revert commit:

1. `git revert <FIX_SHA>` — reverts the workflow change + the new guard test in one operation.
2. CI returns to the divergent path; the chronic-failure pattern resumes; this packet's certification is downgraded to `failed-rollback-required` and a follow-on packet is opened to investigate the new root cause.

No data rollback needed (this is a CI infrastructure change). No service-level rollback (no production deploy touched). No coordination with operators (CI failure does not break running deploys).

The revert is reviewed by the same `bubbles.audit` agent that audited the original change.

---

## Alternatives & Tradeoffs (summary)

| Path | Status | Rationale |
|---|---|---|
| **A — Parity (full stack in CI via `./smackerel.sh test integration`)** | **CHOSEN (DD-1)** | Eliminates all 5 verified failures with one change; spec-031 compliance; zero ongoing classification discipline; AC-4 guard is structurally simple. |
| B — Build-tag partition + new full-stack job | Rejected | No `e2e-api` job exists to leverage; partition would require creating a new full-stack job (= Path A + classification surface for no benefit); ongoing classification discipline has no enforcement mechanism. |
| C — `t.Skip()` on failing tests | Hard-rejected | Forbidden by user mandate "no shortcuts"; forbidden by the integration suite's "no skips, fail loudly" contract; hides coverage gap without fixing root cause. |

---

## Patterns to Follow (long form)

- **[smackerel.sh:687-723](../../../../smackerel.sh)** — the canonical integration test lifecycle. Call this, don't re-implement it.
- **[scripts/runtime/go-integration.sh](../../../../scripts/runtime/go-integration.sh)** — owns the `-p 1` flag and the in-container Go invocation. CI invokes `smackerel.sh test integration` which calls this; no duplication.
- **[tests/integration/test_runtime_health.sh](../../../../tests/integration/test_runtime_health.sh)** — the canonical health probe. Asserts `postgres.status == "up"` + `nats.status == "up"` + `ml_sidecar.status == "up"`. Called transitively by `smackerel.sh test integration` with `KEEP_STACK_UP=1`.
- **[internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go)** — the canonical "parse YAML + assert contract + adversarial sub-tests" pattern. Mirror this shape in the new `ci_integration_topology_contract_test.go`.
- **[internal/deploy/build_workflow_vuln_gate_contract_test.go](../../../../internal/deploy/build_workflow_vuln_gate_contract_test.go)** — concrete prior art for workflow-YAML guarding. Co-locate the new guard alongside this file.

## Patterns to Avoid (long form)

- **The "BUG-045-001 done with known drift" pattern** — Certifying a bug as `done` on local-only evidence when the spec.md AC explicitly requires CI evidence. This packet's `bubbles.validate` MUST capture post-fix CI run JSON inline; local exit 0 is NOT sufficient.
- **The "GH service container + docker-run sidecar" pattern in the integration job** — Documented as the cause of 20 consecutive failures. The AC-4 guard rejects any future revision that re-introduces this pattern.
- **Raw `go test` invocation in CI for integration tests** — Bypasses `-p 1`, bypasses health-probe, bypasses full-stack bring-up. The AC-4 guard rejects raw `go test` invocations targeting `./tests/integration/...`.
- **Lowering `timeout-minutes` below 30** — Cold-cache Ollama pull would not complete; chronic-failure pattern returns. The AC-4 guard asserts `>= 30`.
- **`t.Skip()` in any integration test that has a remediable failure** — Per [.github/copilot-instructions.md](../../../../.github/copilot-instructions.md) and the existing "no skips" contract in [tests/integration/ollama_healthcheck_test.go](../../../../tests/integration/ollama_healthcheck_test.go).

---

## Open Questions

All Phase 1 open questions are resolved by this design:

- **Q-1 (failing test name)** — RESOLVED. 5 failing tests captured verbatim in [report.md § Evidence 6](report.md) and § Root Cause table above.
- **Q-2 (Path A vs B vs C)** — RESOLVED. DD-1 chooses Path A; B and C rejected with rationale grounded in verbatim evidence.
- **Q-3 (re-gate build manifest on integration success)** — Deferred to spec 047 / spec 052. Not blocking for this packet. Tracking remains in [state.json crossReferences.peer](state.json) and the spec 052 cross-reference.

OQ-1 (image reuse from `build` job to `integration` job for cold-cache mitigation) is documented above as deferred — NOT blocking. Future optimisation spec.
