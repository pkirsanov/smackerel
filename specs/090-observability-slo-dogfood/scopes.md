# Scopes — Spec 090 Observability SLO Dogfood

Scope layout: single-file. One scope.

---

## Scope 1: core.health SLO contract + real captured evidence + G100 enforcement

**Status:** Done

**Depends On:** none

### Gherkin Scenarios

```gherkin
Scenario: SLO registry resolves for the dogfood workflow
  Given .github/bubbles-project.yaml declares posture wired (schemaVersion 1)
  When traceContracts.observability.slos.core.health and
       traceContracts.workflows.core.health.slo are added
  Then the G100 guard resolves target {latencyP99Ms:200, errorRatePct:0.1, availabilityPct:99.9}

Scenario: Adding the SLO link without a scope token does not block other specs
  Given the slos + workflows link are present
  And no scope declares observabilityWorkflow: core.health
  Then observability-slo-guard reports G100 no-op (exit 0)

Scenario: Capture tool refuses to fabricate evidence when the core is down
  Given the target endpoint returns HTTP 000 (unreachable)
  When capture-slo.sh run is invoked
  Then it exits non-zero and writes no within-target evidence

Scenario: Real telemetry is captured under load and meets the contract
  Given the smackerel core /api/health is live
  When capture-slo.sh run drives 600 requests at concurrency 20
  Then it writes core.health.slo.json with observed latencyP99Ms <= 200,
       errorRatePct <= 0.1, availabilityPct >= 99.9

Scenario: G100 enforces the captured evidence for Smackerel
  Given the scope Test Plan declares observabilityWorkflow: core.health
  And the captured evidence file is present and within target
  When observability-slo-guard.sh --repo-root . runs
  Then it reports the workflow ENFORCED and within target (exit 0)
```

### Implementation Plan

1. Add `traceContracts.observability.slos.core.health` + `traceContracts.workflows.core.health.slo` to `.github/bubbles-project.yaml`.
2. Build `scripts/observability/capture-slo.sh` (`run` + `compute`), fail-loud, no bypass.
3. Bring up the smackerel core + deps via `./smackerel.sh up` (dev).
4. Capture real telemetry against `http://127.0.0.1:40001/api/health`.
5. Arm G100 by declaring `observabilityWorkflow: core.health` in this Test Plan.

### Test Plan

> The row tagged with the workflow key below carries the literal token
> `observabilityWorkflow: core.health` that arms the Bubbles G100 gate for this
> repo.

| # | Test Type | Category | File/Location | Description | Command | Live System |
|---|-----------|----------|---------------|-------------|---------|-------------|
| 1 | Config resolution | `functional` | `.github/bubbles-project.yaml` | yq resolves slo target for `core.health` | `yq -o=json '.traceContracts.observability.slos["core.health"]' .github/bubbles-project.yaml` | No |
| 2 | Capture unit (within-target) | `unit` | `scripts/observability/capture-slo.sh` | compute path: 100-sample input → correct p99/err/avail JSON | `bash scripts/observability/capture-slo.sh compute --workflow core.health --samples <input> --out /tmp/o.json` | No |
| 3 | Capture unit (breach) | `unit` | `scripts/observability/capture-slo.sh` | compute path: 10% 5xx input → BREACH verdict | `bash scripts/observability/capture-slo.sh compute ... --out /tmp/b.json` | No |
| 4 | Capture lint | `unit` | `scripts/observability/capture-slo.sh` | shellcheck clean | `shellcheck -x scripts/observability/capture-slo.sh` | No |
| 5 | Down-endpoint refusal | `functional` | `scripts/observability/capture-slo.sh` | run against HTTP 000 → exit 1, no fake evidence | `bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:1/api/health` | No |
| 6 | Real capture (observabilityWorkflow: core.health) | `e2e-api` | `scripts/observability/capture-slo.sh` + live smackerel core | 600 reqs @20 against live `/api/health`; emit evidence within target | `bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 600 --concurrency 20` | Yes |
| 7 | G100 enforce + pass | `integration` | `.github/bubbles/scripts/observability-slo-guard.sh` | guard ENFORCES `core.health` and asserts captured evidence within target | `bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .` | Yes |
| 8 | Regression E2E (re-run capture + re-assert G100) | `e2e-api` | `scripts/observability/capture-slo.sh` + `observability-slo-guard.sh` | Regression: re-running the capture keeps the SLO green (G100 exit 0) | `bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 600 --concurrency 20 && bash .github/bubbles/scripts/observability-slo-guard.sh --repo-root .` | Yes |
| 9 | Stress | `stress` | `scripts/observability/capture-slo.sh` + live smackerel core | Heavier load 1500 reqs @ concurrency 50; p99 stays within 200ms target | `bash scripts/observability/capture-slo.sh run --workflow core.health --url http://127.0.0.1:40001/api/health --requests 1500 --concurrency 50 --out /tmp/core-stress.slo.json` | Yes |
| 10 | No hidden defaults / no PII | `functional` | `.github/bubbles/scripts/pii-scan.sh` | repo PII scan stays green on changed files | `bash .github/bubbles/scripts/pii-scan.sh` | No |

### Definition of Done

- [x] SCN-1: The SLO registry resolves for the dogfood workflow — the G100 guard resolves the `core.health` SLO target from `.github/bubbles-project.yaml` → Evidence: [report.md#dod-1](report.md#dod-1)
- [x] SCN-2: Adding the SLO link without a scope token keeps `observability-slo-guard` a no-op (exit 0), not blocking other specs → Evidence: [report.md#dod-4](report.md#dod-4)
- [x] SCN-3: `capture-slo.sh run` refuses to fabricate evidence when the core is down (exit non-zero, no within-target file; no bypass flag) → Evidence: [report.md#dod-3](report.md#dod-3)
- [x] SCN-4: Real telemetry captured under load meets the contract (observed p99/err/avail within target) → Evidence: [report.md#dod-5](report.md#dod-5)
- [x] SCN-5: `observability-slo-guard` ENFORCES `core.health` and asserts the captured evidence within target (exit 0) → Evidence: [report.md#dod-4](report.md#dod-4)
- [x] `capture-slo.sh` emits G100-schema evidence and the compute path is unit-tested (within-target + breach) → Evidence: [report.md#dod-2](report.md#dod-2)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (the re-runnable capture keeps the SLO green) → Evidence: [report.md#dod-regression](report.md#dod-regression)
- [x] Broader E2E regression suite passes (heavier 1500 @ 50 stress load stays within the 200ms target) → Evidence: [report.md#dod-regression](report.md#dod-regression)
- [x] Build Quality Gate (grouped): `capture-slo.sh` shellcheck-clean; smackerel `pii-scan.sh` green; config valid YAML; no bypass flags; captured evidence is gitignored runtime output → Evidence: [report.md#dod-bqg](report.md#dod-bqg)
