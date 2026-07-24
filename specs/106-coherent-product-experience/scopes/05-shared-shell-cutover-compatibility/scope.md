# SCOPE-106-05: Shared Shell Cutover And Compatibility

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-04
**External Entry Gate:** BUG-070-001 complete production-session evidence.

## Outcome

Server, PWA, and Card chrome switch atomically from independent navigation authorities to one generated projection while every supported URL, bookmark, form action, active parent/child state, auth transition, stable hook, and safe return remains compatible.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-001 Credential login works across the product
  Given a valid invited user enters username and password through the real form
  When BUG-070 establishes one browser-purpose session
  Then the shared shell reaches permitted legacy PWA API Card and admin surfaces without a second credential

Scenario: SCN-106-002 Expired session is not an empty state
  Given protected content is visible in the shared shell
  When the unified session expires or is revoked
  Then protected content clears before Your session ended renders with a safe return path
  And no normal empty state or prior personal content remains

Scenario: SCN-106-015 Deep links remain compatible
  Given every existing supported server PWA Wiki Card connector photo recommendation notification settings model and admin bookmark
  When the shared shell cutover is active
  Then each reaches its intended content or an explicit compatible redirect
  And all first-party consumers contain no stale target
```

## UI Scenario Matrix

| UX ID | Viewports | Exact Planned Test Title | File |
|---|---|---|---|
| UX-E2E-106-001 | Desktop, 390px | `UX-E2E-106-001 one login reaches Assistant and representative legacy PWA and Card routes` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-002 | Desktop, 390px | `UX-E2E-106-002 invalid username and password remain non-enumerating and private` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-003 | Desktop, 390px | `UX-E2E-106-003 expired revoked malformed and wrong-purpose sessions clear content before recovery` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-004 | Desktop, 390px | `UX-E2E-106-004 logout closes legacy PWA API and Card trust paths` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-005 | 1440x900 | `UX-E2E-106-005 desktop rail order labels active hierarchy and availability agree across renderers` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-006 | 820x1180 | `UX-E2E-106-006 tablet icon rail tooltips views menu and content tracks never overlap` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-007 | 390x844, 320x568 | `UX-E2E-106-007 mobile bottom bar and More reach every authorized destination without overflow` | `web/pwa/tests/coherent_shell.spec.ts` |
| UX-E2E-106-008 | Desktop, mobile | `UX-E2E-106-008 every supported deep link reaches intended content or explicit redirect` | `web/pwa/tests/coherent_shell.spec.ts` |

## Implementation Plan

1. Switch server, PWA, and Card navigation to `ExperienceProjection` in one compatibility change; remove old arrays/extras only after shadow and canary evidence is current. No hidden static fallback remains.
2. Implement the UX responsive navigation tracks: 232px desktop rail, 64px tablet icon rail with accessible names/tooltips, mobile Today/Assistant/Capture/Search/More bar, route-free group menus, local child switchers, and bottom-anchored Settings/user controls.
3. Preserve route authorization independently from visibility. Unauthorized Admin is omitted, direct access rechecks authorization, 401 and 403 remain distinct, and an unavailable destination never points to a missing route.
4. Preserve all current paths and form methods. Apply explicit compatible redirects only from the completed consumer inventory; never invent `/today`, `/work`, `/sources`, or first-child fallbacks.
5. Preserve stable `data-*` hooks until every first-party consumer has moved, then remove stale hooks only with consumer-facing regression proof.
6. Keep page sections unframed, commands icon-led only when familiar, all icon-only controls named/tooltipped, and shell dimensions reserved before content loads.
7. Atomically advance service-worker assets and browser manifest shortcuts; protected responses remain network-only.

## Consumer Impact Sweep

Update and validate server/PWA/Card nav, breadcrumbs, redirects, deep links, native forms, HTMX targets, request clients, manifest shortcuts, service worker, history, safe return, stable hooks, docs, config, tests, and acceptance manifests. Zero stale references are required before deleting old arrays or aliases.

## Shared Infrastructure Impact And Rollback

Run auth, native Search, HTMX read/mutation, Digest, Assistant, Wiki, Card PRG, PWA auth, logout/replay, service-worker, and non-UI core canaries before broad traversal. Rollback is an immutable shell/assets pointer swap preserving all routes and domain data; it never restores the split session, optimistic static readiness, remote HTMX, or broken known destinations.

## Change Boundary

**Allowed:** shared shell/navigation renderer cutover, route grouping/active state, compatible redirects, responsive shell CSS/assets, consumer updates, focused tests.

**Excluded:** domain page-body repairs, auth implementation, product-data APIs, readiness derivation, foreign packet edits, spec 079, deployment, knb, CCManager, and managed claims.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title / Behavior | Command | Live |
|---|---|---|---|---|---|---|
| XP106-05-U | `unit` | `internal/experience/navigation_projection_test.go` | SCN-106-001, 002, 015 | `TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap` | `./smackerel.sh test unit --go` | No |
| XP106-05-I | `integration` | `tests/integration/experience/shell_cutover_test.go` | SCN-106-001, 002, 015 | `TestServerPWAAndCardCutoverShareProjectionSessionAndDeepLinkContracts` | `./smackerel.sh test integration` | Yes |
| XP106-05-A | `e2e-api` | `tests/e2e/product_shell_e2e_test.go` | SCN-106-001, 002, 015 | `Shared shell routes preserve auth status methods and compatibility without guessed destinations` | `./smackerel.sh test e2e` | Yes |
| XP106-05-C | `functional` | `internal/experience/consumer_inventory_test.go` | SCN-106-015 | `TestShellCutoverLeavesNoStaleNavigationRedirectManifestServiceWorkerDocOrTestTarget` | `./smackerel.sh check` | No |
| UX-E2E-106-001 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-001 | `UX-E2E-106-001 one login reaches Assistant and representative legacy PWA and Card routes` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-002 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-001 | `UX-E2E-106-002 invalid username and password remain non-enumerating and private` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-003 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-002 | `UX-E2E-106-003 expired revoked malformed and wrong-purpose sessions clear content before recovery` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-004 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-001 | `UX-E2E-106-004 logout closes legacy PWA API and Card trust paths` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-005 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-003 | `UX-E2E-106-005 desktop rail order labels active hierarchy and availability agree across renderers` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-006 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-003 | `UX-E2E-106-006 tablet icon rail tooltips views menu and content tracks never overlap` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-007 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-003 | `UX-E2E-106-007 mobile bottom bar and More reach every authorized destination without overflow` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-008 | `e2e-ui` | `web/pwa/tests/coherent_shell.spec.ts` | SCN-106-015 | `UX-E2E-106-008 every supported deep link reaches intended content or explicit redirect` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-001, SCN-106-002, and SCN-106-015 hold through one generated shell, one session, truthful recovery, and complete deep-link compatibility.
- [ ] Desktop, tablet, 390px, and 320px navigation remains stable, named, tooltip-complete, touch-safe, non-overlapping, and authorization-correct.
- [ ] Old authorities and stale targets are removed only after every consumer and canary passes; rollback remains atomic and non-destructive.

#### Test Evidence - 12 Rows / 12 Items

- [ ] XP106-05-U passes with evidence in `report.md#xp106-05-u`.
- [ ] XP106-05-I passes with evidence in `report.md#xp106-05-i`.
- [ ] XP106-05-A passes with evidence in `report.md#xp106-05-a`.
- [ ] XP106-05-C passes with evidence in `report.md#xp106-05-c`.
- [ ] UX-E2E-106-001 passes with evidence in `report.md#ux-e2e-106-001`.
- [ ] UX-E2E-106-002 passes with evidence in `report.md#ux-e2e-106-002`.
- [ ] UX-E2E-106-003 passes with evidence in `report.md#ux-e2e-106-003`.
- [ ] UX-E2E-106-004 passes with evidence in `report.md#ux-e2e-106-004`.
- [ ] UX-E2E-106-005 passes with evidence in `report.md#ux-e2e-106-005`.
- [ ] UX-E2E-106-006 passes with evidence in `report.md#ux-e2e-106-006`.
- [ ] UX-E2E-106-007 passes with evidence in `report.md#ux-e2e-106-007`.
- [ ] UX-E2E-106-008 passes with evidence in `report.md#ux-e2e-106-008`.

#### Build Quality Gate

- [ ] Cutover canaries, consumer trace, auth/privacy, CSP/service-worker, responsive layout, no-interception, rollback, check, lint, format, artifact lint, traceability, and directly affected shell documentation checks pass with zero warnings.
