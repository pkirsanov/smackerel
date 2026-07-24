# SCOPE-106-16: Final Cross-Product Acceptance Handoff

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-15
**External Entry Gates:** BUG-102-001 SCOPE-05 and BUG-032-004 SCOPE-05 complete only after consuming current SCOPE-106-14 and SCOPE-106-15 evidence.

## Outcome

The complete coherent-product matrix runs once more against the real disposable stack, proves every stable SCN-106 and UX-E2E-106 contract, validates no interception or stale consumers, and emits a content-free owner handoff for product acceptance/readiness. Concrete deployment, pointer decisions, managed-doc publication, and certification remain with their owners.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-014 Real-stack browser proof validates behavior
  Given every internal scope and external owner gate is current for one release
  When the complete authenticated browser and API acceptance matrix runs
  Then every required SCN-106 and UX-E2E-106 row has a direct visible behavior result
  And owner regressions remain independently current
  And no route-mounted health-only screenshot-only intercepted or fabricated result can pass
```

## UI Scenario Matrix

| UX ID | Required Coverage | Exact Planned Test Title | File |
|---|---|---|---|
| UX-E2E-106-072 | Every primary journey and required mode | `UX-E2E-106-072 complete coherent product acceptance proves visible behavior with no intercepted internal request` | `web/pwa/tests/coherent_product_acceptance.spec.ts` |

## Implementation Plan

1. Compile the complete stable scenario register from `scenario-manifest.json` and `test-plan.json`; require one result per planned row and reject missing, duplicate, skipped, bailed-out, or unrecognized outcomes.
2. Re-run all focused owner and spec-106 tests before the aggregate. Aggregate results cannot waive a failed owner packet, viewport, accessibility mode, NFR, privacy, consumer, or rollback check.
3. Execute the full real disposable-stack browser loop with actual login, network, APIs, PostgreSQL, owner states, and visible assertions. No internal interception, canned first-party data, injected auth, optional locator, or URL-only proxy is allowed.
4. Run regression-quality guards over every required Playwright file and stale-reference/consumer scans across navigation, routes, redirects, APIs, generated clients, manifests, service worker, docs, config, tests, and acceptance mappings.
5. Emit only stable journey IDs, closed outcomes, timings/count classes, release/catalog/manifest digests, and evidence references. Exclude credentials, user content, target details, prompts, prose, graph/card/photo/provider facts, raw bodies, screenshots with personal data, and operator paths.
6. Hand the validated content-free result to BUG-102/BUG-032 owner contracts. `bubbles.devops` owns target invocation and pointer keep/rollback; `bubbles.docs` owns readiness publication; `bubbles.validate` owns certification.

## Consumer And Shared Infrastructure Final Sweep

Verify generated catalog, server/PWA/Card shell, all primary/local navigation, breadcrumbs, redirects, deep links, native forms, HTMX, request helpers, manifests, service worker, stable hooks, owner APIs, auth/session, appearance assets, accessibility modes, docs references, test discovery, scenario manifest, test plan, and acceptance mappings. Independent login, Search, Digest, Assistant, Graph, Cards, Recommendations, Sources, Activity, readiness, and non-UI canaries must pass before aggregate execution.

## Rollback

A failed candidate remains unaccepted. Product rollback is the existing immutable release pointer contract owned by deployment; spec 106 neither rebuilds nor changes a pointer. Shared experience rollback preserves domain data/evidence, exact routes, and current owner results and never restores known broken session, false-empty, blank, false-ready, remote-asset, or static optimistic behavior.

## Change Boundary

**Allowed:** final spec-106 acceptance orchestration, scenario/test-plan/result validation, content-free handoff generation, consumer/regression guards, focused tests.

**Excluded:** owner implementations/tests/artifacts, readiness/acceptance derivation, deployment/knb, managed docs, certification, production writes, spec 079, CCManager, commit, and push.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-16-I | `integration` | `tests/integration/experience/acceptance_register_test.go` | SCN-106-014 | `TestAcceptanceRegisterContainsEverySCNAndUXRowExactlyOnceWithCurrentOwnerEvidence` | `./smackerel.sh test integration` | Yes |
| XP106-16-A | `e2e-api` | `tests/e2e/coherent_product_acceptance_e2e_test.go` | SCN-106-001..015 | `Complete coherent product API acceptance preserves every owner contract and closed outcome` | `./smackerel.sh test e2e` | Yes |
| UX-E2E-106-072 | `e2e-ui` | `web/pwa/tests/coherent_product_acceptance.spec.ts` | SCN-106-014 | `UX-E2E-106-072 complete coherent product acceptance proves visible behavior with no intercepted internal request` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-16-G | `e2e-ui` | all planned `web/pwa/tests/coherent_*.spec.ts` files | SCN-106-014 | `Required coherent product tests contain no interception bailout optional assertion or URL-only success` | `bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/coherent_*.spec.ts` | No |
| XP106-16-C | `functional` | `internal/experience/consumer_inventory_test.go` | SCN-106-015 | `TestFinalConsumerAndScenarioInventoriesContainNoStaleMissingDuplicateOrUnownedTarget` | `./smackerel.sh check` | No |
| XP106-16-H | `e2e-api` | `tests/e2e/coherent_product_handoff_e2e_test.go` | SCN-106-014 | `Product handoff is complete release-matched content-free and contains no deployment or docs mutation` | `./smackerel.sh test e2e` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-014 Real-stack browser proof validates behavior`: every required SCN-106 and UX-E2E-106 contract has one direct visible real-stack result, owner regressions remain independently current, and no route-mounted, health-only, screenshot-only, intercepted, or fabricated result can pass.
- [ ] Complete browser/API acceptance, accessibility modes, NFR evidence, privacy, consumer trace, rollback, and no-interception guards pass without proxy or aggregate masking.
- [ ] The content-free result is release-matched and owner-consumable while deployment, docs publication, and certification remain untouched.

#### Test Evidence - 6 Rows / 6 Items

- [ ] XP106-16-I passes with evidence in `report.md#xp106-16-i`.
- [ ] XP106-16-A passes with evidence in `report.md#xp106-16-a`.
- [ ] UX-E2E-106-072 passes with evidence in `report.md#ux-e2e-106-072`.
- [ ] XP106-16-G passes with evidence in `report.md#xp106-16-g`.
- [ ] XP106-16-C passes with evidence in `report.md#xp106-16-c`.
- [ ] XP106-16-H passes with evidence in `report.md#xp106-16-h`.

#### Build Quality Gate

- [ ] Full owner/spec-106 test matrix, no-interception/no-bailout, scenario/test-plan parity, consumer trace, privacy/content-free handoff, accessibility/NFR, rollback, build, check, lint, format, artifact lint, traceability, regression, and owner-routing checks pass with zero warnings or unresolved findings.
