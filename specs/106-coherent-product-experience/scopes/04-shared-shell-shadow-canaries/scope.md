# SCOPE-106-04: Shared Shell Shadow Adapters And Canaries

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** In Progress
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-01, SCOPE-106-02, SCOPE-106-03
**External Entry Gate:** BUG-070-001 must supply the production browser-session canary before API-backed shadow acceptance.

## Outcome

Server, PWA, and Card chrome consume the same catalog, appearance, and state contracts in shadow comparison while the current user-visible shell remains unchanged. Golden projections and independent canaries prove high-fan-out compatibility before cutover.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-003 Server and PWA navigation are one projection
  Given one authenticated principal one catalog digest and one set of owner availability outcomes
  When server PWA and Card shadow adapters render
  Then their surface IDs labels order hrefs parents audience availability and projection digest agree exactly
  And adapter failure is visible in shadow evidence without activating an optimistic fallback
```

## Implementation Plan

1. Add server template and PWA DOM adapters over `ExperienceProjection`; PWA constructs nodes safely without `innerHTML`, bearer injection, or static-ready fallback.
2. Add the Card chrome adapter as a consumer of the shared shell contract while preserving every existing Card route, form action, data hook, and owner behavior.
3. Render shadow projections to content-free contract fixtures and comparison telemetry only; do not change active links, page body behavior, route authorization, or capability claims.
4. Compare exact surface ID, parent, order, label, href, current/parent-current, availability, action, and projection digest for equivalent principal/release input.
5. Establish `data-experience-version`, `data-product-navigation`, `data-projection-digest`, `data-surface-id`, `data-parent-surface-id`, and terminal settled markers without embedding user content or evidence IDs.
6. Run independent native Search, HTMX read, HTMX mutation, Digest, Assistant shell, Wiki, Card PRG, PWA auth, logout/replay, service-worker, and non-UI core canaries before cutover.
7. Capture an explicit baseline of current renderer behavior and prove the atomic asset/adapter rollback restores it without reintroducing known unsafe states.

## Shared Infrastructure Impact Sweep

Protected contracts include shared heads, shell ordering/dimensions, session/context hydration, authorization audience, route-active semantics, native forms, HTMX targets/swaps, PWA fetch credentials, Card PRG redirects, service-worker cache identity, browser history, focus restoration, and Playwright bootstrap. Canaries validate unchanged downstream contracts rather than only the new projection fixture.

## Rollback

Shadow adapters can be disabled by reverting the product release; active user behavior is unchanged until SCOPE-106-05. The rollback proof keeps generated catalog and comparison diagnostics, preserves routes/data, and never installs a static optimistic fallback.

## Change Boundary

**Allowed file families:** server/PWA/Card shell adapters in shadow mode, content-free golden fixtures, settled hooks, comparison telemetry, independent canary tests.

**Excluded surfaces:** user-visible navigation cutover, page-body redesign, domain behavior, route changes, readiness derivation, auth implementation, foreign packets, spec 079, deployment, knb, CCManager, and managed claims.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Test Title / Behavior | Command | Live System |
|---|---|---|---|---|---|---|---|
| XP106-04-U | Unit | `unit` | `internal/experience/renderer_projection_test.go` | SCN-106-003 | `TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection` | `./smackerel.sh test unit --go` | No |
| XP106-04-I | Integration | `integration` | `tests/integration/experience/shadow_projection_test.go` | SCN-106-003 | `TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover` | `./smackerel.sh test integration` | Yes |
| XP106-04-A | Regression E2E (API) | `e2e-api` | `tests/e2e/experience_shadow_e2e_test.go` | SCN-106-003 | `Shadow projection digests agree while real routes preserve current authorization and behavior` | `./smackerel.sh test e2e` | Yes |
| XP106-04-W | E2E UI regression | `e2e-ui` | `web/pwa/tests/coherent_shell_shadow.spec.ts` | SCN-106-003 | `shadow shell settles with exact parity and does not alter current navigation or page bodies` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-04-C | Shared-infrastructure canary | `e2e-ui` | `web/pwa/tests/coherent_foundation_canary.spec.ts` | SCN-106-003 | `Canary: shadow adapters preserve native Search HTMX read mutation Digest Assistant Wiki Card PRG PWA auth logout and service-worker contracts` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-04-R | Rollback integration | `integration` | `tests/integration/experience/shell_rollback_test.go` | SCN-106-003 | `TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation` | `./smackerel.sh test integration` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [x] Server, PWA, and Card shadow adapters produce identical content-free projections from one catalog and owner truth without changing active navigation.
- [x] Settled hooks, projection digests, audience/current state, safe DOM construction, and fail-closed adapter errors are consistent.
- [ ] Every high-fan-out downstream canary passes before cutover, and the rollback restores the captured baseline without route/data/preference mutation.

#### Test Evidence - 6 Rows / 6 Items

- [x] XP106-04-U passes with current-session evidence in `report.md#xp106-04-u`.
- [x] XP106-04-I passes against real session and owner inputs in `report.md#xp106-04-i`.
- [x] XP106-04-A passes through real routes in `report.md#xp106-04-a`.
- [ ] XP106-04-W passes without interception in `report.md#xp106-04-w`.
- [ ] XP106-04-C passes every independent canary in `report.md#xp106-04-c`.
- [x] XP106-04-R passes the atomic rollback proof in `report.md#xp106-04-r`.

#### Shared-Infrastructure And Regression Planning

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in the shadow adapters (server/PWA/Card projection parity, fail-closed adapter errors, settled markers) exist and pass — see `report.md#xp106-04-a`.
- [ ] Broader E2E regression suite passes with no shadow-adapter-induced regression to current navigation or page bodies — see `report.md#xp106-04-w`.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (native Search, HTMX read, HTMX mutation, Digest, Assistant shell, Wiki, Card PRG, PWA auth, logout/replay, service-worker, non-UI core) — see `report.md#xp106-04-c`.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified (atomic asset/adapter release pointer swap that preserves routes and data and installs no static optimistic fallback) — see `report.md#xp106-04-r`.

#### Build Quality Gate

- [ ] Golden parity, canary ordering, privacy, CSP, service-worker, no-interception, rollback, check, lint, format, artifact lint, traceability, and directly affected testing/architecture documentation checks pass with zero warnings.
- [ ] Change Boundary is respected and zero excluded file families were changed — see `report.md#xp106-04-change-boundary`.

> **Slice status (honest coupling):** The XP106-04-U unit golden-parity, XP106-04-I integration, XP106-04-R rollback, and — added in this slice — the XP106-04-A e2e-api shadow-safety lane are proven with current-session evidence in `report.md#xp106-04-u`, `report.md#xp106-04-i`, `report.md#xp106-04-r`, and `report.md#xp106-04-a`; Core Outcomes 1 and 2, XP106-04-U, XP106-04-I, XP106-04-R, XP106-04-A, the shared-infrastructure rollback/restore-path row, and the scenario-specific E2E-regression row for the shadow-adapter behaviors (parity, fail-closed adapter errors, settled markers) are checked. XP106-04-A runs UNAUTHENTICATED and proves, through the real routes, the authorization DECISION shadow mode preserves (protected→auth outcome, public→served) plus real-route parity, shadow digest agreement, and no live shadow-marker leak; the AUTHENTICATED-SESSION shadow-parity acceptance is coupled-forward to BUG-070-001's unified production browser session. The e2e-ui (XP106-04-W) and shared-infrastructure canary (XP106-04-C) lanes, Core Outcome 3 (its high-fan-out canary half is still open), the broader-E2E and canary regression rows, and the Build Quality Gate are coupled-forward to a later slice (they require the live PWA browser DOM); the authenticated PWA-auth canary is additionally gated on BUG-070-001's unified production browser session. Those remain unchecked.
