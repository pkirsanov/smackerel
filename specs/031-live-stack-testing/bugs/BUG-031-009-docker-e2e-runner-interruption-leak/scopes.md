# Scopes: BUG-031-009 Dockerized Go E2E runner interruption leak

## Scope 1: Reap Dockerized E2E children before stack teardown

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** test-harness lifecycle bugfix

### Gherkin Scenarios

```gherkin
Feature: Parent interruption cannot leave Dockerized E2E work running

  Scenario: Interrupted Go E2E runner is removed before stack teardown
    Given a focused Go E2E run is active in a Docker runner container
    And the container carries the current parent run identity
    When the parent E2E command receives TERM
    Then the exact runner container is force-removed
    And disposable stack teardown begins only after the runner is absent

  Scenario: Cleanup remains scoped to the interrupted invocation
    Given another Docker container has a different or missing run identity
    When the interrupted parent reaps its children
    Then only containers with the exact active run identity are removed

  Scenario: Existing stubborn shell-child cleanup remains intact
    Given a shell grandchild ignores TERM
    When the E2E parent is interrupted
    Then process-group and marker cleanup still remove the shell child
```

### Implementation Plan

1. Add the controlled nested Docker-runner interruption case to the existing cleanup regression and execute it RED.
2. Add exact run-ID labels to Dockerized E2E children.
3. Add a scoped Docker child reaper before host process and stack teardown.
4. Execute the cleanup regression green and prove the adversarial detector catches a survivor.
5. Re-run focused Drive neighbors, full Drive package, quality checks, and packet gates.
6. Commit and push through normal hooks with certification still in progress.

### Implementation Files

- `smackerel.sh`
- `tests/e2e/test_timeout_process_cleanup.sh`
- `specs/031-live-stack-testing/bugs/BUG-031-009-docker-e2e-runner-interruption-leak/`

### Consumer Impact Sweep

No public route, API client, generated client, symbol, deep link, navigation, breadcrumb, or redirect is renamed or removed. The affected first-party consumers are the baseline Go E2E Docker runner, the opt-in Ollama Go runner, targeted shell E2E nesting, and the parent cleanup trap. All consume the same `e2e_run_child` / `e2e_stop_child` contract.

### Change Boundary

**Allowed file families:** `smackerel.sh`, `tests/e2e/test_timeout_process_cleanup.sh`, this BUG-031-009 packet, and BUG-038-003 routing metadata.

**Excluded surfaces:** product behavior, all-package E2E, parent synthesis/assistant packets, deployment, `knb`, and release-train config.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Docker runner identity/reaper | `unit` | focused shell contract/selftest | Exact label projection and exact-label cleanup, including nonmatching-container adversary | `./smackerel.sh test unit --go ...` or focused shell contract | No/isolated Docker |
| Regression E2E Docker interruption | `e2e-api` | `tests/e2e/test_timeout_process_cleanup.sh` | Interrupt active nested focused Go E2E and assert runner absent before teardown completes | `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` | Yes; disposable stack |
| Cleanup remains scoped to the interrupted invocation | `e2e-api` | `tests/e2e/test_timeout_process_cleanup.sh` | Differently labeled canary remains alive while the exact-run runner is removed | `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` | Yes; isolated Docker |
| Regression E2E shell child | `e2e-api` | same | Existing stubborn descendant process remains fully reaped | same | Yes; disposable stack |
| Broader E2E regression | `e2e-api` | Drive package | Search, observability, successor scenarios stay healthy in serialized order | focused Drive selector through repository CLI | Yes; disposable stack |
| Static quality | `lint` | changed shell/tests | Check, lint, ShellCheck, shfmt/format pass | repository CLI | No |
| Governance | `artifact` | packet and changed files | Artifact, traceability, implementation-reality, state, and regression guards | committed Bubbles scripts | No |

### Definition of Done

- [ ] Root cause confirmed with current-session Docker-runner interruption RED evidence.
- [ ] Pre-fix regression detects a surviving Dockerized Go E2E runner.
- [ ] Every E2E Docker child carries the exact active run-ID label.
- [ ] Exact-label runner containers are removed before stack teardown.
- [ ] Cleanup remains scoped to the interrupted invocation; nonmatching Docker containers are preserved.
- [ ] Existing stubborn shell-child cleanup remains green.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Full serialized Drive package passes without core/network cascade.
- [ ] Regression tests contain no bailout, skip/only, interception, or tautological patterns.
- [ ] Check, lint, format, ShellCheck, and portability checks pass.
- [ ] Consumer impact sweep is complete and zero stale first-party references remain.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Packet artifact, traceability, implementation-reality, state-transition, and regression guards pass at `in_progress`.
- [ ] Source branch is committed and pushed through normal hooks; validate-owned certification remains `in_progress`.
