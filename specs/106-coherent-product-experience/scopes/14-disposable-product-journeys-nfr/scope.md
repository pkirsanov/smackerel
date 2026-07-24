# SCOPE-106-14: Disposable-Stack Product Journeys And NFR Proof

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-13
**External Entry Gates:** BUG-070 before all API-backed acceptance; BUG-002-006, BUG-002-007, BUG-004-004, BUG-039-005, BUG-073-006, BUG-080-001, BUG-083-002, spec 104 Scope 8, and spec 105 before their journey rows execute.

## Outcome

A real disposable Smackerel stack proves all implemented primary journeys, owner mutations, cross-surface return paths, privacy, and the declared settle-time/layout NFRs before BUG-032 and BUG-102 consume any journey evidence.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-014 Real-stack browser proof validates behavior
  Given an ephemeral live Smackerel stack an invited authenticated test identity uniquely owned PostgreSQL fixtures and validate-plane telemetry
  When the primary journey matrix runs in a real browser
  Then login navigation Search Today Assistant Capture Knowledge Graph Work Cards Recommendations Sources Photos Activity Settings and Admin exercise visible user outcomes
  And no internal request is intercepted replaced or satisfied from canned first-party data
  And every mutable fixture is isolated and restored or absent after teardown
```

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-013 | `UX-E2E-106-013 first-run opens the product with truthful states and no wizard tour or sample data` | `web/pwa/tests/coherent_product_journeys.spec.ts` |
| UX-E2E-106-015 | `UX-E2E-106-015 returning user sees current context without unread missed backlog or streak copy` | `web/pwa/tests/coherent_product_journeys.spec.ts` |
| UX-E2E-106-017 | `UX-E2E-106-017 weekly review opens owning workspaces and Back restores exact context and focus` | `web/pwa/tests/coherent_product_journeys.spec.ts` |
| UX-E2E-106-069 | `UX-E2E-106-069 representative mutations show one pending state and authoritative complete or failed outcome` | `web/pwa/tests/coherent_product_journeys.spec.ts` |

## Implementation Plan

1. Extend the existing disposable `./smackerel.sh test e2e-ui` lane with a uniquely named Compose project, test identity, database namespace, external-boundary controls, browser profile, and `env=test*` telemetry. Never use persistent dev/operate data, prod monitoring, backup paths, release config, or deploy manifests.
2. Authenticate through the real login/invite/password flow and browser cookie jar. Prohibit cookie/token/storage injection, direct database reads from browser assertions, first-party request interception, canned responses, silent returns, optional locators, and URL-only success.
3. Partition tests by feature files; run every owner suite independently and add cross-product journeys only for shell/context/state assertions. No mega-test substitutes for owner evidence.
4. For each mutation, assert one request, pending UI, exact terminal response, visible outcome, authoritative reload/read-back, duplicate prevention, and unchanged/rolled-back state for adversarial failures.
5. Run desktop `1440x900`, tablet `820x1180`, mobile `390x844`, and narrow `320x568` at 200% zoom; System/Light/Dark, reduced motion, forced colors, keyboard, screen-reader snapshots, screenshots, bounding boxes, computed styles, target sizes, focus, and announcements follow SCOPE-106-13.
6. Measure NFR-106-001 from completion of the authenticated same-host read to `data-experience-settled=true`; stress representative populated/empty/degraded/error states. Measure NFR-106-002 for layout shift, theme flash, overlap, and unsent-input preservation under sustained transitions.
7. Capture content-free validate-plane metrics/traces/logs using existing project wiring without tagging a nonexistent `observabilityWorkflow`; project config currently registers only unrelated `core.health`.
8. Teardown all fixtures, external controls, browser state, and validate-plane artifacts on success or failure; any residue blocks evidence eligibility.

## Shared Infrastructure Impact Sweep

Protected surfaces are Playwright config/projects, auth bootstrap, database migration/fixtures, service startup ordering, session hydration, external boundary controls, service worker, trace capture, screenshots, and teardown. Independent current auth, Search, Digest, Assistant, Graph, Card, and non-UI canaries run before the broad matrix. Rollback restores the prior harness configuration and proves owner suites still run independently.

## Change Boundary

**Allowed:** spec-106 disposable journey fixtures/harness extensions, cross-product Playwright/API/NFR tests, validate-plane content-free evidence adapters.

**Excluded:** owner repair code/tests, shared harness wholesale replacement, production/operate writes, deployment/knb, spec 079, CCManager, release claims, and product-data endpoints.

## Test Plan

| ID | Category | File/Location | Scenario/NFR | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-14-U | `unit` | `internal/experience/journey_manifest_test.go` | SCN-106-014 | `TestSpec106JourneyMatrixCoversEveryScenarioUXRowModeAndForbidsInterceptionInjectionAndBailout` | `./smackerel.sh test unit --go` | No |
| XP106-14-I | `integration` | `tests/integration/experience/disposable_journey_harness_test.go` | SCN-106-014 | `TestDisposableJourneyHarnessOwnsIdentityStateTelemetryAndGuaranteedTeardown` | `./smackerel.sh test integration` | Yes |
| XP106-14-A | `e2e-api` | `tests/e2e/coherent_product_journeys_e2e_test.go` | SCN-106-014 | `All implemented primary journeys expose exact real API status schema state and readback` | `./smackerel.sh test e2e` | Yes |
| XP106-14-W | `e2e-ui` | `web/pwa/tests/coherent_product_journeys.spec.ts` | SCN-106-014 | `complete coherent product journey uses real login routes data and visible outcomes without interception` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-14-O | `functional` | owner evidence register | SCN-106-014 | `TestCrossProductAcceptanceRequiresEveryApplicableOwnerRegressionCurrent` | `./smackerel.sh check` | No |
| UX-E2E-106-013 | `e2e-ui` | `web/pwa/tests/coherent_product_journeys.spec.ts` | SCN-106-014 | `UX-E2E-106-013 first-run opens the product with truthful states and no wizard tour or sample data` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-015 | `e2e-ui` | `web/pwa/tests/coherent_product_journeys.spec.ts` | SCN-106-014 | `UX-E2E-106-015 returning user sees current context without unread missed backlog or streak copy` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-017 | `e2e-ui` | `web/pwa/tests/coherent_product_journeys.spec.ts` | SCN-106-014 | `UX-E2E-106-017 weekly review opens owning workspaces and Back restores exact context and focus` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-069 | `e2e-ui` | `web/pwa/tests/coherent_product_journeys.spec.ts` | SCN-106-010, 014 | `UX-E2E-106-069 representative mutations show one pending state and authoritative complete or failed outcome` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-14-S | `stress` | `tests/stress/coherent_product_experience_stress_test.go` | NFR-106-001 | `Primary populated empty degraded and error states settle within the declared P95 budget` | `./smackerel.sh test stress` | Yes |
| XP106-14-L | `load` | `tests/stress/coherent_product_experience_load_test.go` | NFR-106-002 | `Sustained authenticated route and theme transitions preserve layout and unsent input without overlap` | `./smackerel.sh test stress` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-014 executes all implemented primary journeys with real auth, services, storage, routes, visible assertions, accessibility, privacy, and no interception.
- [ ] Mutable fixtures and validate telemetry remain isolated and leave zero residue; owner suites remain independently required.
- [ ] Stress/load exists only for NFR-106-001 and NFR-106-002 and proves populated behavior, not route mounting or fast empty shells.
- [ ] The resulting content-free journey evidence is current and suitable for downstream BUG-032/BUG-102 consumption without claiming their completion.

#### Test Evidence - 11 Rows / 11 Items

- [ ] XP106-14-U passes with evidence in `report.md#xp106-14-u`.
- [ ] XP106-14-I passes with evidence in `report.md#xp106-14-i`.
- [ ] XP106-14-A passes with evidence in `report.md#xp106-14-a`.
- [ ] XP106-14-W passes with evidence in `report.md#xp106-14-w`.
- [ ] XP106-14-O passes with evidence in `report.md#xp106-14-o`.
- [ ] UX-E2E-106-013 passes with evidence in `report.md#ux-e2e-106-013`.
- [ ] UX-E2E-106-015 passes with evidence in `report.md#ux-e2e-106-015`.
- [ ] UX-E2E-106-017 passes with evidence in `report.md#ux-e2e-106-017`.
- [ ] UX-E2E-106-069 passes with evidence in `report.md#ux-e2e-106-069`.
- [ ] XP106-14-S passes NFR-106-001 with evidence in `report.md#xp106-14-s`.
- [ ] XP106-14-L passes NFR-106-002 with evidence in `report.md#xp106-14-l`.

#### Build Quality Gate

- [ ] Owner suites, disposable isolation/teardown, no-interception/no-bailout, accessibility/visual matrix, privacy, NFR evidence, environment-pollution guard, bundle freshness, check, lint, format, artifact lint, traceability, and directly affected testing/operations documentation checks pass with zero warnings.
