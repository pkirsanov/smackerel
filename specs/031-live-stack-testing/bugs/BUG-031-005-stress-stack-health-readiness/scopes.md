# Scopes: BUG-031-005 Stress stack health readiness

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Repair stress stack readiness and env handoff

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-005 stress stack readiness and env handoff
  Scenario: Go stress uses the same disposable test stack as shell stress
    Given the repo stress command has prepared the disposable test stack
    When the Go stress phase starts inside its container
    Then CORE_EXTERNAL_URL, DATABASE_URL, NATS_URL, and SMACKEREL_AUTH_TOKEN resolve from the same SST environment
    And the health, database, and NATS readiness canary passes before workload tests run

  Scenario: Unhealthy stress stack fails clearly before workloads
    Given the stress stack health endpoint is unreachable or bound to the wrong environment
    When ./smackerel.sh test stress reaches the readiness canary
    Then the command fails with a single infrastructure readiness diagnostic
    And package workload failures are not reported as if they ran

  Scenario: Workload failures remain visible after readiness succeeds
    Given the stress stack health, database, NATS, and auth canary has passed
    When a feature-owned stress workload violates its latency or correctness budget
    Then ./smackerel.sh test stress reports that workload failure directly
    And the readiness gate does not convert the workload failure into a skip or pass

  Scenario: Agent stress DB and NATS wiring are complete
    Given the agent concurrency stress package runs in the Go stress phase
    When it connects to the database and NATS using environment values provided by the stress runner
    Then database ping and NATS connection succeed before concurrent invocation isolation assertions run
```

### Implementation Plan
1. Confirm the current failing stress command path using the authoritative 039 red evidence and a fresh repo-standard stress run if the owner is allowed to run it.
2. Decide whether the stress command should manage the existing `test` environment for all phases or introduce an SST-managed stress environment.
3. Update the repo CLI stress flow so shell stress and Go stress receive the same environment values and lifecycle ownership.
4. Pass `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` into the Go stress container from SST-derived env.
5. Add a shared readiness canary before package workloads and make it fail loudly for core, DB, NATS, auth, and wrong-stack targets.
6. Preserve package workload assertions after readiness succeeds; do not replace package failures with readiness skips.
7. Add adversarial regression coverage for wrong-stack core URL, unreachable DB, missing/unreachable NATS, and workload-failure visibility.
8. Run the repo-standard validation gates and record raw evidence in `report.md` before asking validate to certify.

### Shared Infrastructure Impact Sweep
- Downstream package surfaces: `tests/stress/knowledge_stress_test.go`, `tests/stress/photos_ingest_stress_test.go`, `tests/stress/recommendations_test.go`, `tests/stress/drive/drive_scale_stress_test.go`, `tests/stress/agent/concurrency_test.go`.
- Contract surfaces: stress command lifecycle, generated env loading, Docker network mode, core external URL, DB URL, NATS URL, auth token, stack cleanup, and test storage isolation.
- Blast radius: all stress packages and any workflow that treats `./smackerel.sh test stress` as a final delivery gate.
- Canary requirement: a shared readiness canary must run before feature-owned workload tests and must be independently capable of failing the infrastructure handoff.

### Change Boundary
Allowed file families:
- `smackerel.sh`
- `scripts/runtime/go-stress.sh`
- `tests/stress/**`
- `scripts/lib/runtime.sh` only if existing runtime helpers need a narrowly scoped env/lifecycle utility
- `config/smackerel.yaml` and config-generation scripts only if an SST value is missing and the generator is updated in the same change
- `docs/Testing.md` and `docs/Development.md` if command semantics change
- This bug packet under `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/`

Excluded file families:
- `internal/recommendation/**` and recommendation feature behavior
- `internal/knowledge/**`, `internal/drive/**`, `internal/agent/**`, and photos connector workload logic unless residual package-specific failures remain after readiness is green
- `config/generated/**`
- Parent feature certification fields outside this bug packet unless `bubbles.validate` owns the transition

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-031-005-01 | Canary: Go stress env wiring uses one SST environment | integration/devops | `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/readiness/live_canary_test.go`, `tests/stress/readiness/canary_test.go` | Core URL, DB URL, NATS URL, and auth token are present, SST-derived, and mutually consistent before workloads run through `./smackerel.sh test stress` | BUG-031-005-SCN-001 |
| T-BUG-031-005-02 | Regression E2E: wrong-stack core URL fails clearly | Regression E2E / stress-devops | `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/readiness/canary_test.go` | If Go stress points at an unstarted dev URL or unused port after test-stack setup, the command fails with one readiness diagnostic before package workloads run | BUG-031-005-SCN-002 |
| T-BUG-031-005-03 | Regression E2E: DB reachability failure is infrastructure-owned | Regression E2E / stress-devops | `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/readiness/canary_test.go`, `tests/stress/agent/concurrency_test.go` | Unreachable `DATABASE_URL` fails readiness with DB reachability context and does not appear as a knowledge/photos/drive/recommendation workload timeout | BUG-031-005-SCN-002 |
| T-BUG-031-005-04 | Regression E2E: NATS reachability is wired for agent stress | Regression E2E / stress-devops | `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/readiness/canary_test.go`, `tests/stress/agent/concurrency_test.go` | Missing or unreachable `NATS_URL` fails readiness before `TestConcurrentInvocationIsolation_BS018` starts | BUG-031-005-SCN-004 |
| T-BUG-031-005-05 | Adversarial workload preservation: readiness does not mask workload failure | stress | `tests/stress/recommendations_test.go`, `tests/stress/knowledge_stress_test.go`, `tests/stress/photos_ingest_stress_test.go`, and `tests/stress/drive/drive_scale_stress_test.go` after canary passes | With readiness healthy, a real package workload assertion failure remains reported as that package failure, not as a readiness skip or command pass | BUG-031-005-SCN-003 |
| T-BUG-031-005-06 | Broad stress gate | stress | `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/readiness/live_canary_test.go`, `tests/stress/recommendations_test.go`, `tests/stress/agent/concurrency_test.go` | `./smackerel.sh test stress` shows shared readiness green and any residual failures are package-specific with clear owner routing | BUG-031-005-SCN-001, BUG-031-005-SCN-003, BUG-031-005-SCN-004 |
| T-BUG-031-005-07 | Broader E2E regression suite | e2e-api | `smackerel.sh`, `tests/e2e/knowledge_lint_test.go`, `tests/e2e/recommendations_full_regression_test.go`, `tests/e2e/drive/drive_foundation_e2e_test.go` | `./smackerel.sh test e2e` keeps live-stack E2E lifecycle healthy after stress lifecycle changes | BUG-031-005-SCN-001 |
| T-BUG-031-005-08 | Broad integration regression suite | integration | `smackerel.sh`, `tests/integration/knowledge_lint_test.go`, `tests/integration/agent/pipeline_bridge_test.go`, `tests/integration/drive/drive_foundation_canary_test.go` | `./smackerel.sh test integration` keeps disposable test-stack integration behavior healthy after stress lifecycle changes | BUG-031-005-SCN-001 |
| T-BUG-031-005-09 | Regression quality guard | quality | `tests/stress/readiness/canary_test.go`, `tests/stress/readiness/live_canary_test.go`, `scripts/runtime/go-stress.sh`, `smackerel.sh`, `tests/stress/test_health_stress.sh` | Regression quality guard finds no silent-pass bailout patterns and confirms adversarial failure cases in the listed stress readiness surfaces | BUG-031-005-SCN-002, BUG-031-005-SCN-003, BUG-031-005-SCN-004 |

### Definition of Done
- [x] Root cause confirmed and documented with source inspection and pre-fix stress evidence.
  - **Phase:** stabilize
  - **Command:** `timeout 1800 ./smackerel.sh test stress`; `docker top pedantic_wozniak`; `curl --max-time 5 -fsS http://127.0.0.1:40001/api/health`; `curl --max-time 5 -fsS http://127.0.0.1:45001/api/health`
  - **Exit Code:** stress diagnostic stopped after evidence; `docker top` 0; dev core curl 28; test core curl 7
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  Health stress test passed with 25/25 successful requests
  === Search Stress Results ===
    Artifacts in DB:    1100
    Queries executed:   10
    Average time:       1174ms
    Threshold:          3000ms
    Failures:           0
  Search stress test passed: all queries completed under 3000ms with 1100 artifacts

  $ docker top pedantic_wozniak
  bash /workspace/scripts/runtime/go-stress.sh
  go test -tags stress -v -count=1 -timeout 720s ./tests/stress/...

  $ curl --max-time 5 -fsS http://127.0.0.1:40001/api/health
  curl: (28) Connection timed out after 5002 milliseconds
  Command exited with code 28

  $ curl --max-time 5 -fsS http://127.0.0.1:45001/api/health
  curl: (7) Failed to connect to 127.0.0.1 port 45001 after 0 ms: Couldn't connect to server
  Command exited with code 7
  ```

  - **Interpretation:** shell stress passes on the disposable test stack, but the Go stress phase starts package workloads after that stack is gone and while the dev core target is unavailable. Source inspection confirms the same mismatch: `smackerel.sh test stress` switches from shell `test` env to Go `dev` env and omits `NATS_URL`.
- [x] Stress command uses one SST-derived environment contract across shell and Go stress phases.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1 due residual recommendation workload failure after readiness passed
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  Container smackerel-test-postgres-1 Healthy
  Container smackerel-test-nats-1 Healthy
  Container smackerel-test-smackerel-ml-1 Healthy
  Container smackerel-test-smackerel-core-1 Healthy
  Health stress test passed with 25/25 successful requests
  === Search Stress Results ===
    Artifacts in DB:    1100
    Queries executed:   10
    Failures:           0
  Search stress test passed: all queries completed under 3000ms with 1100 artifacts
  go-stress: running readiness canary
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  go-stress: readiness canary passed
  Exit Code: 1
  ```

  - **Interpretation:** the stress command now keeps shell and Go phases on the disposable `test` stack until the Go canary has passed; the non-zero exit is a later workload failure, not the prior shared env/lifecycle handoff.
- [x] Go stress phase receives `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` from the intended stress environment.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1 due residual recommendation workload failure after readiness passed
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  go-stress: running readiness canary
  === RUN   TestStressReadinessCanary_Live
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  PASS
  go-stress: readiness canary passed
  === RUN   TestConcurrentInvocationIsolation_BS018
      concurrency_test.go:185: BS-018: ran 200 concurrent invocations in 507.776662ms
  --- PASS: TestConcurrentInvocationIsolation_BS018 (0.51s)
  Exit Code: 1
  ```

  - **Interpretation:** the canary requires `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN`; the live canary and agent DB/NATS stress both passed with the test-env values provided by `smackerel.sh`.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1 due residual recommendation workload failure after readiness passed
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  go-stress: running readiness canary
  === RUN   TestStressReadinessCanary_Live
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  PASS
  ok      github.com/smackerel/smackerel/tests/stress/readiness   2.081s
  go-stress: readiness canary passed
  === RUN   TestKnowledge_LintAt1000ArtifactScale
  --- PASS: TestKnowledge_LintAt1000ArtifactScale
  Exit Code: 1
  ```

  - **Interpretation:** `go-stress.sh` ran the live readiness package before broad `./tests/stress/...` workloads; workload packages began only after the canary passed.
- [x] Wrong-stack or unhealthy-core adversarial regression fails clearly before package workload tests run.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS
  --- PASS: TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS (0.00s)
  === RUN   TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS
  --- PASS: TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS (0.00s)
  === RUN   TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes
  --- PASS: TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes (0.00s)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```

  - **Interpretation:** the wrong-stack core response without authenticated service topology fails before DB or NATS probes can run, preventing package workload misclassification.
- [x] DB reachability adversarial regression fails clearly as infrastructure readiness.
  - **Evidence:** Existing executed Go unit block below.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS
  --- PASS: TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS (0.00s)
  === RUN   TestCheckWithProbes_UnreachableNATSFailsAfterDatabase
  --- PASS: TestCheckWithProbes_UnreachableNATSFailsAfterDatabase (0.00s)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```

  - **Interpretation:** an unreachable DB is reported by the readiness layer before NATS and before broad package workloads.
- [x] NATS reachability adversarial regression fails clearly before agent concurrency starts.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes
  --- PASS: TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes (0.00s)
  === RUN   TestCheckWithProbes_UnreachableNATSFailsAfterDatabase
  --- PASS: TestCheckWithProbes_UnreachableNATSFailsAfterDatabase (0.00s)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```

  - **Interpretation:** missing and unreachable NATS are readiness failures that stop execution before `TestConcurrentInvocationIsolation_BS018` can begin.
- [x] Agent stress DB and NATS wiring are complete before concurrent invocation isolation assertions run.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1 due residual recommendation workload failure after readiness passed
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  go-stress: readiness canary passed
  === RUN   TestConcurrentInvocationIsolation_BS018
      concurrency_test.go:185: BS-018: ran 200 concurrent invocations in 507.776662ms
  --- PASS: TestConcurrentInvocationIsolation_BS018 (0.51s)
  PASS
  ok      github.com/smackerel/smackerel/tests/stress/agent       0.532s
  Exit Code: 1
  ```

  - **Interpretation:** the agent package got working DB and NATS env from the repaired Go stress runner before its concurrency assertions ran.
- [x] Adversarial regression case exists and would fail if the Go stress phase switched back to dev env after shell stress used test env.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS
  --- PASS: TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS (0.00s)
  === RUN   TestConfigFromEnvRequiresAllStressValues
  --- PASS: TestConfigFromEnvRequiresAllStressValues (0.00s)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```

  - **Interpretation:** a core health response that lacks authenticated service topology is rejected before DB/NATS probes, which catches the old wrong-stack shape instead of letting workloads time out.
- [x] Workload failure preservation regression proves readiness does not mask real package workload failures.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestGoStressHarness_WorkloadFailurePropagatesAfterCanary
  --- PASS: TestGoStressHarness_WorkloadFailurePropagatesAfterCanary (0.01s)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```

  - **Interpretation:** `go-stress.sh` does not convert a post-canary workload failure into a readiness skip or command pass.
- [x] Pre-fix regression test fails with recorded raw output.
  - **Phase:** stabilize
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** stopped after evidence; dev core curl 28; test core curl 7
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  Health stress test passed with 25/25 successful requests
  Search stress test passed: all queries completed under 3000ms with 1100 artifacts
  $ docker top pedantic_wozniak
  bash /workspace/scripts/runtime/go-stress.sh
  go test -tags stress -v -count=1 -timeout 720s ./tests/stress/...
  $ curl --max-time 5 -fsS http://127.0.0.1:40001/api/health
  curl: (28) Connection timed out after 5002 milliseconds
  $ curl --max-time 5 -fsS http://127.0.0.1:45001/api/health
  curl: (7) Failed to connect to 127.0.0.1 port 45001 after 0 ms: Couldn't connect to server
  ```
- [x] Post-fix regression test passes with recorded raw output.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`; `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** unit 0; stress 1 due residual recommendation workload failure after readiness passed
  - **Claim Source:** interpreted

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0

  $ timeout 1800 ./smackerel.sh test stress
  go-stress: running readiness canary
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  go-stress: readiness canary passed
  === RUN   TestConcurrentInvocationIsolation_BS018
  --- PASS: TestConcurrentInvocationIsolation_BS018 (0.51s)
  Exit Code: 1
  ```

  - **Interpretation:** the post-fix readiness regression suite passes; the broader stress command still exits 1 only after readiness due a routed recommendation workload failure.
- [x] `./smackerel.sh test stress` passes or returns only residual package-specific failures with owner routing after shared readiness is repaired.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1
  - **Claim Source:** interpreted

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  go-stress: readiness canary passed
  === RUN   TestConcurrentInvocationIsolation_BS018
  --- PASS: TestConcurrentInvocationIsolation_BS018 (0.51s)
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:169: stress: zero samples collected — workers never produced any observations
  --- FAIL: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (60.21s)
  FAIL
  Command exited with code 1
  Exit Code: 1
  ```

  - **Interpretation:** the remaining stress failure is package-specific after shared readiness passed, so ownership is routed to `specs/039-recommendations-engine`.
- [x] `./smackerel.sh test integration` passes after lifecycle changes.
  - **Phase:** devops
  - **Command:** `timeout 900 ./smackerel.sh test integration`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 900 ./smackerel.sh test integration
  Preparing disposable test stack...
  Container smackerel-test-postgres-1 Healthy
  Container smackerel-test-nats-1 Healthy
  Container smackerel-test-smackerel-core-1 Healthy
  Container smackerel-test-smackerel-ml-1 Healthy
  PASS
  ok      github.com/smackerel/smackerel/tests/integration        31.762s
  PASS
  ok      github.com/smackerel/smackerel/tests/integration/agent  3.743s
  PASS
  ok      github.com/smackerel/smackerel/tests/integration/drive  8.610s
  Exit Code: 0
  ```
- [x] `./smackerel.sh test e2e` passes after lifecycle changes.
  - **Phase:** devops
  - **Command:** `timeout 1200 ./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 1200 ./smackerel.sh test e2e
  Shell E2E Test Results
    Total:  35
    Passed: 35
    Failed: 0
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e        95.928s
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e/agent  3.952s
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e/drive  22.536s
  PASS: go-e2e
  Exit Code: 0
  ```
- [x] `./smackerel.sh test unit` passes after implementation changes.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 ./smackerel.sh test unit --go
  ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
  ok      github.com/smackerel/smackerel/internal/db      (cached)
  ok      github.com/smackerel/smackerel/internal/digest  (cached)
  ok      github.com/smackerel/smackerel/internal/domain  (cached)
  ok      github.com/smackerel/smackerel/internal/drive   (cached)
  ok      github.com/smackerel/smackerel/internal/pipeline        (cached)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   0.029s
  Exit Code: 0
  ```
- [x] `./smackerel.sh check` passes.
  - **Phase:** devops
  - **Command:** `timeout 120 ./smackerel.sh check`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Config is in sync with SST
  env_file drift guard: OK
  scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
  scenarios registered: 4, rejected: 0
  scenario-lint: OK
  ```
- [x] `./smackerel.sh lint` passes.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh lint`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Successfully built smackerel-ml
  All checks passed!
  === Validating web manifests ===
      OK: web/pwa/manifest.json
      OK: PWA manifest has required fields
      OK: web/extension/manifest.json
      OK: Chrome extension manifest has required fields (MV3)
      OK: web/extension/manifest.firefox.json
      OK: Firefox extension manifest has required fields (MV2 + gecko)
  Web validation passed
  ```
- [x] `./smackerel.sh format --check` passes.
  - **Phase:** devops
  - **Command:** `timeout 600 ./smackerel.sh format --check`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Obtaining file:///workspace/ml
  Installing build dependencies: started
  Installing build dependencies: finished with status 'done'
  Successfully built smackerel-ml
  49 files already formatted
  Exit Code: 0
  ```
- [x] Regression tests contain no silent-pass bailout patterns.
  - **Phase:** devops
  - **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/readiness/canary_test.go tests/stress/readiness/live_canary_test.go scripts/runtime/go-stress.sh smackerel.sh tests/stress/test_health_stress.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  BUBBLES REGRESSION QUALITY GUARD
  Bugfix mode: true
  Scanning tests/stress/readiness/canary_test.go
  Adversarial signal detected in tests/stress/readiness/canary_test.go
  Scanning scripts/runtime/go-stress.sh
  Adversarial signal detected in scripts/runtime/go-stress.sh
  Scanning smackerel.sh
  Adversarial signal detected in smackerel.sh
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 5
  Exit Code: 0
  ```

  - **Phase:** test
  - **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/readiness/canary_test.go tests/stress/readiness/live_canary_test.go scripts/runtime/go-stress.sh smackerel.sh tests/stress/test_health_stress.sh`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  BUBBLES REGRESSION QUALITY GUARD
  Repo: <home>/smackerel
  Timestamp: 2026-05-04T15:10:47Z
  Bugfix mode: true
  Scanning tests/stress/readiness/canary_test.go
  Adversarial signal detected in tests/stress/readiness/canary_test.go
  Scanning tests/stress/readiness/live_canary_test.go
  Scanning scripts/runtime/go-stress.sh
  Adversarial signal detected in scripts/runtime/go-stress.sh
  Scanning smackerel.sh
  Adversarial signal detected in smackerel.sh
  Scanning tests/stress/test_health_stress.sh
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 5
  Files with adversarial signals: 3
  Exit Code: 0
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`; `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** stress 1 due residual recommendation workload failure after readiness passed; unit 0
  - **Claim Source:** interpreted

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  go-stress: running readiness canary
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  go-stress: readiness canary passed

  $ timeout 600 ./smackerel.sh test unit --go
  === RUN   TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS
  --- PASS: TestCheckWithProbes_WrongStackCoreURLFailsBeforeDatabaseOrNATS (0.00s)
  === RUN   TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS
  --- PASS: TestCheckWithProbes_UnreachableDatabaseFailsBeforeNATS (0.00s)
  === RUN   TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes
  --- PASS: TestCheckWithProbes_MissingNATSURLFailsBeforeNetworkProbes (0.00s)
  === RUN   TestGoStressHarness_WorkloadFailurePropagatesAfterCanary
  --- PASS: TestGoStressHarness_WorkloadFailurePropagatesAfterCanary (0.01s)
  ```

  - **Interpretation:** scenario-specific regression coverage is split between live stress canary execution and adversarial readiness/harness unit tests; no package workload logic was modified.
- [x] Broader E2E regression suite passes.
  - **Phase:** devops
  - **Command:** `timeout 1200 ./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Shell E2E Test Results
    Total:  35
    Passed: 35
    Failed: 0
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e        95.928s
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e/agent  3.952s
  PASS
  ok      github.com/smackerel/smackerel/tests/e2e/drive  22.536s
  PASS: go-e2e
  ```
- [x] Rollback or restore path for shared infrastructure changes is documented and verified.
  - **Phase:** devops
  - **Command:** `timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1 with cleanup completed
  - **Claim Source:** executed

  ```text
  $ timeout 1800 ./smackerel.sh test stress
  Container smackerel-test-smackerel-core-1 Removed
  Container smackerel-test-smackerel-ml-1 Removed
  Container smackerel-test-postgres-1 Removed
  Container smackerel-test-nats-1 Removed
  Volume smackerel-test-postgres-data Removed
  Volume smackerel-test-nats-data Removed
  Network smackerel-test_default Removed
  Exit Code: 1
  ```

  - **Interpretation:** the shared stress command restores the disposable test stack state with project-scoped teardown even when a post-readiness workload fails.
- [x] Change Boundary is respected and zero excluded file families were changed.
  - **Phase:** devops
  - **Command:** `git status --short`
  - **Exit Code:** 0
  - **Claim Source:** interpreted

  ```text
   M docs/Development.md
   M docs/Testing.md
   M scripts/runtime/go-stress.sh
   M smackerel.sh
   M tests/stress/test_health_stress.sh
  ?? specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/
  ?? tests/stress/readiness/
  ```

  - **Interpretation:** the BUG-031-005-owned delta is contained to allowed stress harness, stress tests, docs, and this bug packet. Other modified files in the worktree belong to unrelated in-flight lanes and are not claimed by this DoD item.
- [x] Artifact lint passes for this bug packet.
  - **Phase:** devops-artifact-validation
  - **Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Required artifact exists: spec.md
  Required artifact exists: design.md
  Required artifact exists: uservalidation.md
  Required artifact exists: state.json
  Required artifact exists: scopes.md
  Required artifact exists: report.md
  Top-level status matches certification.status
  All checked DoD items in scopes.md have evidence blocks
  No repo-CLI bypass detected in report.md command evidence
  Artifact lint PASSED.
  Exit Code: 0
  ```
- [x] Traceability guard passes for this bug packet.
  - **Phase:** devops-artifact-validation
  - **Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  BUBBLES TRACEABILITY GUARD
  scenario-manifest.json covers 4 scenario contract(s)
  All linked tests from scenario-manifest.json exist
  Scope 1: Repair stress stack readiness and env handoff scenario mapped to Test Plan row: Go stress uses the same disposable test stack as shell stress
  Scope 1: Repair stress stack readiness and env handoff scenario mapped to Test Plan row: Unhealthy stress stack fails clearly before workloads
  Scope 1: Repair stress stack readiness and env handoff scenario mapped to Test Plan row: Workload failures remain visible after readiness succeeds
  Scope 1: Repair stress stack readiness and env handoff scenario mapped to Test Plan row: Agent stress DB and NATS wiring are complete
  DoD fidelity: 4 scenarios checked, 4 mapped to DoD, 0 unmapped
  RESULT: PASSED (0 warnings)
  Exit Code: 0
  ```
- [x] Bug marked as Fixed in `bug.md` by the validation owner after all evidence is recorded.
  - **Phase:** validate
  - **Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`; `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`; `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`; `timeout 120 ./smackerel.sh check`; `timeout 600 ./smackerel.sh format --check`; `timeout 600 ./smackerel.sh lint`; `timeout 600 ./smackerel.sh test unit --go`; `timeout 600 bash .github/bubbles/scripts/implementation-reality-scan.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness --verbose`; `timeout 600 bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`
  - **Exit Code:** artifact lint 0; traceability 0; state-transition 1 before certification edits with only validate/audit/status-family blockers remaining; check 0; format-check 0; lint 0; Go unit 0; implementation reality scan 0; artifact freshness 0
  - **Claim Source:** interpreted

  ```text
  Artifact lint PASSED.
  RESULT: PASSED (0 warnings)
  TRANSITION BLOCKED: 7 failure(s), 3 warning(s)
  BLOCK: Resolved scope artifacts have 1 UNCHECKED DoD items
  BLOCK: Resolved scope artifacts have 1 scope(s) still marked 'In Progress'
  BLOCK: Required phase 'validate' NOT in execution/certification phase records
  BLOCK: Required phase 'audit' NOT in execution/certification phase records
  BLOCK: Execution/certification phases claim implement/test phases but completedScopes is EMPTY
  Config is in sync with SST
  49 files already formatted
  All checks passed!
  ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
  Implementation reality scan PASSED with 1 warning(s)
  RESULT: PASS (0 failures, 0 warnings)
  ```

  - **Interpretation:** validation accepted the recorded shared-readiness repair evidence: the BUG-031-005 stress harness now reaches a green core/DB/NATS/auth readiness canary before workloads, and the remaining red stress signals occur after readiness in package-owned workloads. The validation owner therefore marked `bug.md` Fixed, set Scope 1 to Done, and recorded validate phase/certification fields in `state.json`. The audit phase is not claimed by this evidence.

### Residual Owner Routing For Post-Readiness Stress Failures

**Claim Source:** interpreted from executed stress evidence recorded in `report.md`.

**Interpretation:** Full stress remains non-zero only after the shared readiness canary passes. `TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` zero samples remains owned by `specs/039-recommendations-engine`; `TestKnowledge_LintAt1000ArtifactScale` / knowledge health timing signals remain owned by the knowledge/spec 025 classification lane. These residuals do not contradict BUG-031-005's shared-readiness outcome contract.
