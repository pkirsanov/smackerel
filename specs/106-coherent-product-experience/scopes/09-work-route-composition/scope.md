# SCOPE-106-09: Lists Meals And Expenses Route Composition

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gate:** SCOPE-106-02 must identify exact current API contracts, registered browser routes, route owners, auth scopes, and real owner journey evidence for each leaf.

## Outcome

Lists, Meals/Recipes, and Expenses become Work local views only from exact existing owner contracts. A leaf with no registered browser route and complete owner journey stays Unavailable with null href; this scope never invents `/work`, a substitute endpoint, a parallel store, or browser business logic.

## Work-Route Ownership (2026-07-24 harden)

- Spec 106 owns the route-free **Work navigation group** (null parent href) and its readiness-driven visibility, and owns the **Lists, Meals/Recipes, and Expenses browser routes and user journeys** composed over the existing owner domain APIs. Those domain APIs and their owners are unchanged; this scope reimplements none of them.
- The Work grouping is a readiness-driven navigation-catalog decision covered by the **declared partial-supersession of spec 100's fixed assistant-first inventory** (`state.json.predecessorContractDisposition[100]`). The certified artifacts of specs 100 and 092 are not edited. Cards (spec 092 domain, SCOPE-106-10) is a separate top-level surface, not a Work leaf, so there is no Work/Cards ownership conflict.
- Every shell-reached Work mutation binds to **BUG-070-001 `MutationTrustGuard`** (server-validated session-bound anti-CSRF/Origin proof, 403 before mutation). Spec 106 consumes this enforcement and does not implement it.
- Analyst scenarios `SCN-106-017` (Lists), `SCN-106-018` (Meals/Recipes), and `SCN-106-019` (Expenses) are covered by this scope through `UX-E2E-106-037/038/039` in `scenario-manifest.json`. The scope-local Gherkin tags `SCN-106-003`/`SCN-106-010` remain coverage aliases per the scenario-authority note and are not the analyst definitions of those IDs.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-003 Work navigation binds only proven destinations
  Given the exact catalog inventory for Lists Meals and Expenses
  When a leaf has a registered authorized browser route and complete owner journey
  Then it may appear under the route-free Work group with its exact existing route
  When any required contract is absent
  Then the leaf remains Unavailable without a clickable substitute

Scenario: SCN-106-010 Work mutations use owner-authoritative outcomes
  Given a proven Work leaf exposes a create edit complete archive generate correct or remove command
  When the user invokes it through shared composition
  Then pending and terminal feedback comes from the owner contract
  And spec 106 adds no persistence transaction or API semantics
```

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-037 | `UX-E2E-106-037 Lists composes only from proven routes and round-trips owner lifecycle without guilt counters` | `web/pwa/tests/coherent_work.spec.ts` |
| UX-E2E-106-038 | `UX-E2E-106-038 Meals composes owner plan recipe shopping and calendar outcomes without drag dependence` | `web/pwa/tests/coherent_work.spec.ts` |
| UX-E2E-106-039 | `UX-E2E-106-039 Expenses composes owner ledger provenance correction and safe export without advice claims` | `web/pwa/tests/coherent_work.spec.ts` |

## Implementation Plan

1. Consume SCOPE-106-02's exact route/API/owner matrix. For each leaf, require one registered browser route, current owner implementation evidence, auth policy, complete read/command state contract, and real owner E2E before adding href.
2. Keep Work a route-free group control. It opens a named child menu and never redirects to a guessed parent or hidden first child.
3. Compose Lists through owner list/item lifecycle, Meals through owner recipe/plan/slot/shopping/calendar outcomes, and Expenses through owner ledger/provenance/correction/export outcomes only where those contracts exist.
4. Apply shared workspace header, local switcher, rows/tables, inspector/sheet, state bands, mutation footer, evidence rows, responsive records, keyboard alternatives, and theme tokens without copying domain code.
5. Preserve every exact owner route, method, DTO, auth/CSRF rule, deep link, and test hook. Unsupported leaves remain non-actionable and truthfully explained.
6. Maintain Product Principle boundaries: no guilt counters, marketing gallery, accounting conclusion, tax advice, financial advice, hidden fields, or drag-only controls.

## Consumer Impact Sweep

Trace catalog leaves, route manifests, owner APIs/clients, navigation, breadcrumbs, redirects, deep links, forms, local switchers, service worker, auth/CSRF, stable hooks, docs, tests, and acceptance manifests. A consumer cannot switch to a route absent from the verified inventory.

## Change Boundary

**Allowed:** shared Work grouping and composition over exact owner browser surfaces, responsive/shared primitives, cross-surface tests.

**Excluded:** new endpoints/routes/stores, Lists/Meals/Expenses domain implementation, migrations, owner API/client tests, guessed paths, foreign packets, spec 079, deployment, knb, CCManager, and readiness derivation.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-09-U | `unit` | `internal/experience/work_route_admission_test.go` | SCN-106-003 | `TestWorkLeafRequiresExactRegisteredRouteOwnerContractAndJourneyEvidence` | `./smackerel.sh test unit --go` | No |
| XP106-09-I | `integration` | `tests/integration/experience/work_composition_test.go` | SCN-106-003, 010 | `TestWorkCompositionUsesExactOwnerRoutesAndLeavesUnprovenCapabilitiesUnavailable` | `./smackerel.sh test integration` | Yes |
| XP106-09-A | `e2e-api` | `tests/e2e/work_composition_e2e_test.go` | SCN-106-003, 010 | `Work leaf route methods auth states and owner readbacks match the exact inventory` | `./smackerel.sh test e2e` | Yes |
| UX-E2E-106-037 | `e2e-ui` | `web/pwa/tests/coherent_work.spec.ts` | SCN-106-010 | `UX-E2E-106-037 Lists composes only from proven routes and round-trips owner lifecycle without guilt counters` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-038 | `e2e-ui` | `web/pwa/tests/coherent_work.spec.ts` | SCN-106-010 | `UX-E2E-106-038 Meals composes owner plan recipe shopping and calendar outcomes without drag dependence` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-039 | `e2e-ui` | `web/pwa/tests/coherent_work.spec.ts` | SCN-106-010 | `UX-E2E-106-039 Expenses composes owner ledger provenance correction and safe export without advice claims` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-09-C | `functional` | `internal/experience/work_consumer_inventory_test.go` | SCN-106-003 | `TestWorkConsumersContainNoInventedEndpointParentRouteOrUnprovenReadyLeaf` | `./smackerel.sh check` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-003 Work navigation binds only proven destinations`: each Work leaf receives its exact existing route only when a registered authorized browser route and complete owner journey are proven; every unproven leaf remains Unavailable without a clickable substitute or guessed parent.
- [ ] `SCN-106-010 Work mutations use owner-authoritative outcomes`: shared composition mirrors owner pending, persisted, idempotent, conflict, refused, partial, and failed outcomes without adding persistence, transaction, or API semantics.
- [ ] Route-free grouping, responsive/accessible controls, consumer trace, and product boundaries remain complete.

#### Test Evidence - 7 Rows / 7 Items

- [ ] XP106-09-U passes with evidence in `report.md#xp106-09-u`.
- [ ] XP106-09-I passes with evidence in `report.md#xp106-09-i`.
- [ ] XP106-09-A passes with evidence in `report.md#xp106-09-a`.
- [ ] UX-E2E-106-037 passes with evidence in `report.md#ux-e2e-106-037`.
- [ ] UX-E2E-106-038 passes with evidence in `report.md#ux-e2e-106-038`.
- [ ] UX-E2E-106-039 passes with evidence in `report.md#ux-e2e-106-039`.
- [ ] XP106-09-C passes with evidence in `report.md#xp106-09-c`.

#### Build Quality Gate

- [ ] Exact-inventory, no-invented-endpoint, owner-evidence, auth/CSRF, consumer trace, responsive/a11y, no-interception, check, lint, format, artifact lint, traceability, and directly affected user/testing documentation checks pass with zero warnings.
