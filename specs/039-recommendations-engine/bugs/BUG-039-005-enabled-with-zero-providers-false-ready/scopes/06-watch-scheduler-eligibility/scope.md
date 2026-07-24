# Scope 06: Watch And Scheduler Eligibility

**Status:** Not Started
**Depends On:** 05
**Scope-Kind:** runtime-behavior

## Outcome

Watch create, enable, resume, refresh, and scheduler evaluation use the exact shared availability gate before persistence, while pause, silence, delete, and existing-watch reads remain safe during outages.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-06 Watch creation requires provider readiness
	Given no healthy category-compatible production provider exists or the eligible snapshot becomes stale before the write
	When a user creates enables resumes or refreshes a watch or the scheduler evaluates it
	Then the command is visibly refused before persistence and prior authoritative state remains
	And no watch run next-due success or synthetic evidence is created while pause silence delete and authorized reads remain available
```

## Implementation Plan

1. Introduce a watch command boundary that derives category/operation and obtains an unexpired matching snapshot before store calls.
2. Gate create, provider-dependent update/enable, resume, trigger, and scheduler execution; leave pause, silence, delete, and reads independent of provider health.
3. Revalidate snapshot expiry immediately before write and preserve unchanged-state semantics on refusal/conflict.
4. Write typed availability/outcome/provider evidence only for real runs that begin execution; never synthesize a refused run.
5. Keep existing watches visible with provider-unavailable presentation and preserve user-controlled enabled state during transient outages.
6. Make manual refresh and scheduler use the same gate/idempotency path and prevent duplicate activation.

## Consumer Impact Sweep

Update watch API handlers, service/store command boundaries, scheduler evaluator, run persistence, Telegram watch consumers where applicable, web watch editor/list, metrics, docs, and tests. Preserve stable watch IDs, consent, and safe control semantics.

## Change Boundary

Allowed: recommendation watch command/service/store/API/scheduler, watch-run evidence, watch UI hooks, focused tests. Excluded: request ranking, provider adapter protocols, unrelated scheduler jobs, Telegram delivery semantics beyond availability consumption, and shared auth internals.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC06-TP01 | Command unit | `unit` | `internal/api/recommendation_watches_test.go` | SCN-039-005-03, 06 | Operation/category gating, stale snapshot refusal, safe-control exemptions, and response mapping | `./smackerel.sh test unit --go` | No |
| REC06-TP02 | Watch integration | `integration` | `tests/integration/recommendation_watches_test.go` | SCN-039-005-06 | Create/enable/resume/refresh refusal writes no inert watch or run in ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| REC06-TP03 | Scheduler integration | `integration` | `tests/integration/recommendation_watch_audit_test.go` | SCN-039-005-03, 07 | Manual/scheduled overlap uses one gate and one real run; refusal and typed attempted failure remain auditable | `./smackerel.sh test integration` | Yes |
| REC06-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendation_watch_consent_test.go` | SCN-039-005-06 | `TestRegressionDirectUnavailableWatchRouteCannotPersistInertWatch` is red before command gating and green after repair | `./smackerel.sh test e2e` | Yes |
| REC06-TP05 | Watch lifecycle E2E API | `e2e-api` | `tests/e2e/recommendations_watches_web_test.go` | SCN-039-005-03, 06 | Healthy create/pause/resume/refresh read-back plus provider-loss refusal and retained existing watch | `./smackerel.sh test e2e` | Yes |
| REC06-TP06 | Watch Playwright regression | `e2e-ui` | `web/pwa/tests/recommendation_watches_readiness.spec.ts` | SCN-039-005-06, 07 | `SCN-039-005-06 Regression: unavailable create and refresh refuse without inert state` on live UI with no request interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-06 Watch creation requires provider readiness: every provider-dependent watch/scheduler action refuses before persistence when readiness is absent or stale and safe reducing actions remain available.
- [ ] Every provider-dependent watch mutation and scheduler run gates before persistence with stale-snapshot fail-closed behavior.
- [ ] Safe reducing controls and existing-watch visibility remain available during provider outages.
- [ ] Refused commands create no watch/run while attempted executions retain typed redacted provider evidence.
- [ ] Watch/scheduler consumer and change-boundary sweeps preserve consent, IDs, delivery behavior, and unrelated jobs.

#### Test Evidence - 6 Rows / 6 Items

- [ ] REC06-TP01 command-unit evidence is recorded.
- [ ] REC06-TP02 watch-integration evidence is recorded.
- [ ] REC06-TP03 scheduler/audit integration evidence is recorded.
- [ ] REC06-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC06-TP05 watch-lifecycle E2E API evidence is recorded.
- [ ] REC06-TP06 live no-interception Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused checks, broader watch/scheduler/Telegram regressions, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.