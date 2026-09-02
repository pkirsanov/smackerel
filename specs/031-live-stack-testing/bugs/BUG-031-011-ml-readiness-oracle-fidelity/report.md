# Report: [BUG-031-011] ML Readiness Oracle Fidelity

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

The readiness stress test now replaces the one-sided HTTP 200 oracle with separate HTTP 503 and HTTP 200 cases against the production `WaitForMLReady` call path. Both cases require an observed `GET /health` probe. The HTTP 503 case also bounds elapsed time and probe count. Existing timeout-warning capture changes remain in place. No production source, Docker, configuration, or deployment file changed.

## Completion Statement

The bug is not fixed, runtime-verified, or certified. The packet remains `in_progress`. The operator limited this invocation to static and editor checks, so the focused stress test, mutation proof, Docker-backed suites, validation, and audit remain unexecuted.

## Bug Reproduction - Before Fix

**Phase:** bug
**Command:** not run
**Exit Code:** not applicable
**Claim Source:** not-run

The source inspection found a one-sided test arrangement. No regression command ran because the operator required artifact-only work and prohibited Docker.

## Source-Grounded Findings

**Claim Source:** interpreted

| Source | Observed fact | Finding |
|---|---|---|
| `tests/stress/ml_readiness_timeout_stress_test.go::TestMLReadinessAlways200Regression` | The handler always returns HTTP 200 and the test requires `ready=true` | The test proves the healthy path, not rejection of a non-ready dependency |
| `internal/api/search.go::probeMLHealth` | The request path is `${MLSidecarURL}/health` | `/ml/readyz` is not the dependency route exercised by this test |
| `internal/api/router.go` | Core `GET /readyz` maps to `Dependencies.ReadyzHandler` | Core readiness is a separate route |
| `internal/api/health.go::ReadyzHandler` | Default readiness checks database health | This packet must not claim core `/readyz` represents ML health |
| `tests/integration/ml_readiness_test.go::TestMLReadiness_TimeoutFallback` | Existing coverage returns HTTP 503 and expects false | The new guard must add two-sided discrimination and mutation proof |

## Code Diff Evidence

**Claim Source:** interpreted

- `tests/stress/ml_readiness_timeout_stress_test.go` now contains `TestMLReadinessNonReadyDependencyCannotBeMaskedAsReady/non_ready_dependency` and `/healthy_control`.
- The non-ready case returns HTTP 503, requires `ready=false`, requires at least one `GET /health` probe, and enforces bounded elapsed time and probe count.
- The healthy control returns HTTP 200, requires `ready=true`, and requires at least one `GET /health` probe.
- The pre-existing warning-capture changes in both timeout tests remain present.
- Core `GET /readyz` is not called. No production source changed.

## Test Evidence

### Static Regression Quality Guard

**Executed:** YES (in current session)
**Command:** `timeout 60 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/ml_readiness_timeout_stress_test.go`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: ~/smackerel
	Timestamp: 2026-09-02T02:41:45Z
	Bugfix mode: true
============================================================

ℹ️  Scanning tests/stress/ml_readiness_timeout_stress_test.go
✅ Adversarial signal detected in tests/stress/ml_readiness_timeout_stress_test.go

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 1
	Files with adversarial signals: 1
============================================================
```

**Result:** PASS for the static regression-quality guard only.

### Runtime Test Execution

**Executed:** NO
**Command:** not run
**Exit Code:** not applicable
**Claim Source:** not-run

No focused stress, mutation, integration, E2E, full stress, or Docker result is claimed.

## Uncertainty Declaration

- **What was attempted:** Source and packet inspection, the bounded test-only oracle edit, editor diagnostics, diff whitespace validation, and the static bugfix regression-quality guard.
- **What was observed:** The edited test has separate HTTP 503 and HTTP 200 controls, requires real `GET /health` probes, and carries an adversarial always-ready failure assertion. The static guard reported zero violations and zero warnings.
- **Why this is uncertain:** No Go test process, mutation run, Docker stack, integration suite, E2E suite, or full stress suite ran in this invocation.
- **What would resolve this:** Run the focused readiness test and mutation proof first, then the integration companion, E2E, and broader stress checks through `./smackerel.sh` in a later authorized runtime-validation invocation.

## Planned Evidence Anchors

- `tp-031-011-01-pre-fix-mutation-survival`
- `tp-031-011-02-focused-readiness-discriminator`
- `tp-031-011-03-integration-companion`
- `tp-031-011-04-broader-e2e`
- `tp-031-011-05-broader-stress`
- `tp-031-011-06-regression-quality`

## Validation Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.validate`
**Claim Source:** not-run

No completion validation or certification occurred.

## Audit Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.audit`
**Claim Source:** not-run

No audit verdict is claimed.

## Chaos Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.chaos`
**Claim Source:** not-run

No chaos result is claimed. The planned adversarial case belongs to the test phase.