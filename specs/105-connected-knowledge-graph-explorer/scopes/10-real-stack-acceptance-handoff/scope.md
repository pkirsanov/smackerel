# SCOPE-10: Real-Stack Acceptance And Deployment Handoff

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-09

## Outcome

Certify the completed product slice against the real disposable stack and
produce a value-safe deployment acceptance handoff. This scope reruns every
SCN-105 contract across API/UI, verifies graph pixels plus semantic truth,
proves migration and rollback behavior, and leaves concrete deployment to
`bubbles.devops` without target details or operated-data writes.

## Acceptance Scenario Groups

```gherkin
Acceptance Group: SCN-105-001 through SCN-105-007 complete the desktop graph journey
  Given the real disposable stack contains bounded populated, empty, isolated, failure, no-path, and partial fixtures
  When the authenticated user opens, expands, filters, explains a path, navigates Back, and retries a failed branch
  Then every named specification outcome and NFR is directly observable without interception or fake topology

Acceptance Group: SCN-105-008 through SCN-105-010 complete equivalent access
  Given desktop, tablet, mobile, keyboard, screen-reader, reduced-motion, theme, and forced-colors configurations
  When each actor completes the same exploration goal
  Then identity, relationship, reason, evidence, focus, filter, and path outcomes remain equivalent and unobscured

Scenario: SCN-105-012 First-use empty remains truthful in final acceptance
  Given every required authorized graph read succeeds with zero nodes in the real disposable stack
  When the explorer settles across Graph, Outline, and Table
  Then the state is actionable true-empty
  And no sample node, fake edge, unavailable message, Retry action, or topology pixel appears
```

## Final Playwright Matrix

- 1440x900 Graph/Outline/Table desktop journeys;
- 820x1180 tablet filters/inspector focus trap and restoration;
- 390x844 touch pan/pinch/explicit controls/sheets;
- 320x568 at 200% zoom with target-size, overflow, and clipping checks;
- keyboard-only adjacency, expansion, filters, path, detail, and Back;
- accessibility snapshots and live-region terminal announcements;
- light, dark, system, reduced-motion, and forced-colors variants;
- populated nonblank Canvas pixel histogram, quadrant distribution, finite
  geometry, known semantic IDs, hit tests, resize, and two DPR values;
- true-empty successful zero response with no node/edge pixels or sample data;
- auth clear proving prior labels, semantic rows, geometry, hit maps, and
  topology pixels absent before recovery;
- no `page.route`, `context.route`, `route.fulfill`, MSW, Nock, optional
  locators, early-return bailouts, URL-only success, or assertions on setup data.

## Migration And Rollback Acceptance

1. Apply migration 063 to a disposable database and prove graph records are
   untouched, indexes exist, saved-view constraints/ownership hold, and queries
   use intended plans.
2. Prove rollback before first saved-view row can remove additive preferences
   safely; after rows exist, source/config pointer rollback preserves the table
   and rows for compatible forward recovery.
3. Prove application rollback restores the prior static asset pointer/cache
   version and explicitly disables Explorer when required; it may not rebuild,
   mutate graph records, or restore BUG-080 warning-and-nil behavior.

## Deployment Acceptance Handoff

- Product output contains fixed journey IDs, safe outcome, duration/count
  classes, source SHA/config identity, and evidence references only.
- The handoff requires BUG-080 authenticated Graph read synthetic plus Explorer
  API, UI, Canvas, accessibility, mobile, privacy, and NFR acceptance.
- Feature tests run on validate-plane disposable data and `env=test*`
  telemetry. Operate-plane queries are read-only and deployment-owner scoped.
- Concrete target, host, endpoint, secret, manifest, adapter, and promotion
  operations remain `bubbles.devops` owned and are absent from this packet.

## Consumer And Shared-Shell Final Sweep

- Shared navigation, Knowledge local navigation, breadcrumbs, redirects,
  manifest, service worker, Wiki/detail/search entries, API clients, validators,
  history, saved views, CSP, docs, config, synthetics, and tests contain no stale
  references.
- Existing Wiki, login/session, health, non-Graph PWA routes, and Graph family
  synthetic pass independently before the full suite.

## Test Plan

| ID | Test Type | Category | Scenario / Contract | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-012-U | Unit | `unit` | SCN-105-012 | `web/pwa/tests/graph_honest_states_test.go` - `SCN-105-012 true empty unit` | `./smackerel.sh test unit` | No |
| T105-012-I | Integration | `integration` | SCN-105-012 | `tests/integration/graph_explorer/empty_state_test.go` - `SCN-105-012 true empty integration` | `./smackerel.sh test integration` | Yes |
| T105-012-A | E2E API regression | `e2e-api` | SCN-105-012 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-012 true empty API` | `./smackerel.sh test e2e` | Yes |
| T105-012-W | E2E UI regression | `e2e-ui` | SCN-105-012 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-012 first use has no fake topology` | `./smackerel.sh test e2e-ui` | Yes |
| T105-10-API | Full E2E API | `e2e-api` | SCN-105-001..013 | `tests/e2e/graph_explorer_e2e_test.go` - `Connected Graph Explorer complete authorized API acceptance` | `./smackerel.sh test e2e` | Yes |
| T105-10-PLAYWRIGHT | Full E2E UI | `e2e-ui` | SCN-105-012; full SCN-105-001..013 matrix | `web/pwa/tests/graph-explorer.spec.ts` - `Connected Graph Explorer complete desktop mobile keyboard privacy and pixel acceptance` | `./smackerel.sh test e2e-ui` | Yes |
| T105-10-NO-INTERCEPT | Regression quality | `e2e-ui` | SCN-105-001..013 | `web/pwa/tests/graph-explorer.spec.ts` - `Required graph scenarios contain no interception bailout or optional assertion` | `bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/graph-explorer.spec.ts` | No |
| T105-10-MIGRATION | Migration integration | `integration` | Migration 063 | `tests/integration/graph_explorer/migration_test.go` - `Graph explorer migration applies and pre-data rollback preserves canonical graph` | `./smackerel.sh test integration` | Yes |
| T105-10-ROLLBACK | Rollback integration | `integration` | Rollback contract | `tests/integration/graph_explorer/rollback_test.go` - `Explorer source asset and saved-view rollback is pointer-safe and non-destructive` | `./smackerel.sh test integration` | Yes |
| T105-10-SHELL | Shared-shell canary | `e2e-ui` | Consumer sweep | `web/pwa/tests/wiki.spec.ts` - `Existing Wiki login shell navigation and non-Graph PWA journeys remain intact` | `./smackerel.sh test e2e-ui` | Yes |
| T105-10-HANDOFF | Deployment acceptance contract | `e2e-api` | Value-safe handoff | `tests/e2e/graph_explorer_acceptance_e2e_test.go` - `Product acceptance emits complete value-safe deployment handoff without operated writes` | `./smackerel.sh test e2e` | Yes |
| T105-10-REGRESSION | Broad regression | `e2e-ui` | All changed behavior | `web/pwa/tests/wiki.spec.ts`, `web/pwa/tests/graph-activation.spec.ts`, `web/pwa/tests/graph-explorer.spec.ts` - `Knowledge Wiki Graph activation and explorer journeys remain coherent` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-012 First-use empty remains truthful: a successful zero-node real-stack read produces actionable true-empty across projections with no sample topology, Retry, unavailable state, or topology pixels.
- [ ] Migration, pre-data rollback, post-data preservation, source/config pointer rollback, and static asset cache behavior are non-destructive and never mutate canonical graph records.
- [ ] The complete consumer/shared-shell sweep passes and the value-safe acceptance packet routes concrete deployment to `bubbles.devops` without target or secret content.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-012-U passes with evidence in `report.md#t105-012-u`.
- [ ] T105-012-I passes with evidence in `report.md#t105-012-i`.
- [ ] T105-012-A passes with evidence in `report.md#t105-012-a`.
- [ ] T105-012-W passes with successful-zero and no-fake-topology evidence in `report.md#t105-012-w`.
- [ ] T105-10-API passes with complete scenario evidence in `report.md#t105-10-api`.
- [ ] T105-10-PLAYWRIGHT passes the full viewport/accessibility/pixel matrix in `report.md#t105-10-playwright`.
- [ ] T105-10-NO-INTERCEPT passes with guard output in `report.md#t105-10-no-intercept`.
- [ ] T105-10-MIGRATION passes apply and permitted rollback evidence in `report.md#t105-10-migration`.
- [ ] T105-10-ROLLBACK passes non-destructive source/asset/preference rollback evidence in `report.md#t105-10-rollback`.
- [ ] T105-10-SHELL passes independent shared-shell/Wiki/auth canary evidence in `report.md#t105-10-shell`.
- [ ] T105-10-HANDOFF passes complete value-safe product acceptance evidence in `report.md#t105-10-handoff`.
- [ ] T105-10-REGRESSION passes broad coherent Knowledge/Wiki/Graph evidence in `report.md#t105-10-regression`.

#### Build Quality Gate

- [ ] All packet tests, stress/load/SLO, check, build, lint, format, source lock, migration, rollback, API/docs, accessibility, bundle freshness, artifact lint, traceability, state-transition guard, consumer sweep, zero warnings, no interception, and no fake topology checks pass with executed evidence before validation or deployment routing.

## Uncertainty Declaration

All items remain unchecked because no implementation, full-stack test,
migration, rollback, browser, acceptance, deployment, commit, or push execution
was performed by the planning owner. Concrete deployment remains foreign-owned.