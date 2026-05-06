# Bug Specification: BUG-031-005 Stress Stack Health Readiness

## Problem Statement
The stress gate is red during spec 039 finalization because the Go stress phase cannot reliably observe a healthy live stack or reachable database. The failure affects multiple feature-owned stress packages, so the bug belongs to shared live-stack/stress lifecycle readiness rather than the recommendations engine.

## Outcome Contract
**Intent:** The stress command uses one coherent SST-derived test-stack contract across shell and Go stress phases.

**Success Signal:** `./smackerel.sh test stress` first proves the stress stack health, database, NATS, and auth wiring through the same environment that all stress packages use, then runs the package workloads. Infrastructure failures are reported as infrastructure failures, and package workload failures remain visible when readiness is healthy.

**Hard Constraints:**
- Runtime validation continues to flow through `./smackerel.sh`.
- Configuration values originate from `config/smackerel.yaml` and generated env, not hardcoded defaults.
- Generated files under `config/generated/` are not edited by hand.
- Stress validation uses disposable test storage or explicitly managed test lifecycle cleanup.
- The fix must not weaken package workload assertions, skip stress packages silently, or convert workload errors into readiness passes.

**Failure Condition:** The bug remains unresolved if Go stress tests still target an unstarted or wrong stack, if missing env values skip required live stress tests, if DB/NATS reachability fails without a clear stress readiness diagnostic, or if readiness checks mask real workload failures after the stack is healthy.

## Goals
- Align shell and Go stress phases on the same repo-managed stress stack environment.
- Add a shared readiness canary before feature-owned stress workloads run.
- Verify `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` are present, SST-derived, and reachable in the Go stress container.
- Preserve package-specific stress assertions after the readiness canary passes.
- Record adversarial regression coverage for unhealthy stack, wrong-stack URL, DB reachability failure, and workload failure visibility.

## Non-Goals
- Rewriting knowledge, recommendation, photos, drive, or agent workload logic.
- Changing feature 039 recommendation ranking, providers, attribution, or policy behavior.
- Editing generated config by hand.
- Bypassing the repo CLI with ad-hoc `go test`, `pytest`, or direct Docker Compose commands for normal validation.

## Requirements
- R-BUG-031-005-001: `./smackerel.sh test stress` must use a single, explicit stress environment contract for all stress phases.
- R-BUG-031-005-002: The Go stress phase must receive `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` from SST-derived env values.
- R-BUG-031-005-003: The stress readiness gate must fail clearly when the core health endpoint is unavailable, returning an infrastructure readiness error before package workloads start.
- R-BUG-031-005-004: The stress readiness gate must fail clearly when the database cannot be reached from the Go stress container.
- R-BUG-031-005-005: The stress readiness gate must fail clearly when NATS cannot be reached from the Go stress container.
- R-BUG-031-005-006: When readiness succeeds, feature-owned stress workloads must run and surface their own latency, throughput, or correctness failures without being masked by broad skips.
- R-BUG-031-005-007: Regression tests must include adversarial wrong-stack or unhealthy-stack cases that fail if the Go stress phase returns to dev-stack or missing-env behavior.

## User Scenarios (Gherkin)

```gherkin
Feature: BUG-031-005 stress stack health readiness

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

## Acceptance Criteria
- [ ] Stress Go phase no longer targets an unstarted dev stack when the command prepared a disposable test stack.
- [ ] Stress Go phase receives and verifies `CORE_EXTERNAL_URL`, `DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` through SST-derived env.
- [ ] A readiness canary fails clearly for unhealthy core, unreachable DB, unreachable NATS, or wrong-stack URL.
- [ ] At least one adversarial regression case would fail if the Go stress phase switched back to dev env after shell stress used test env.
- [ ] At least one regression case proves a healthy stack does not mask package workload failures.
- [ ] `./smackerel.sh test stress` passes after the shared readiness repair or reports only package-specific residual failures that can be routed to their owning specs.
