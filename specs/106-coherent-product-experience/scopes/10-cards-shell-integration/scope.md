# SCOPE-106-10: Cards Shell Integration

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gate:** BUG-083-002 Scopes 01-18 complete owner evidence, including its own shell, errors, accessibility, security, and parity loop.

## Outcome

BUG-083's completed Card product composes under the global Work/Cards shell and shared visual/state foundation without duplicating any of its 16 domain parity rows. Existing Card deep links and dense workflows remain intact; only shell, token, layout, state-language, and cross-product context are integrated.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-010 Card mutations keep authoritative owner outcomes
  Given BUG-083 has completed owner-scoped Card commands and read-back
  When the Card owner acts through the global shell
  Then shared pending and terminal presentation mirrors the owner result exactly
  And no cross-product adapter announces success early or retries twice

Scenario: SCN-106-011 Card workflows remain available on narrow layouts
  Given the complete BUG-083 parity product
  When it renders under the global shell at desktop tablet 390 and 320 pixels
  Then every required field action status and local view remains visible non-overlapping and touch-safe
```

## Ownership Boundary

BUG-083 owns owner binding, migrations, wallet, offers/caps, selections, bonuses/Calendar, optimization versions, sources/media, audit, configuration, portability, pending actions, reports, operations, Card errors, Card accessibility/security, and all 16 parity rows. Spec 106 owns the outer global shell, global appearance assets, Work parent placement, cross-product state vocabulary, no-nested-card composition guard, and navigation back to other product journeys.

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-053 | `UX-E2E-106-053 Cards wallet lifecycle remains owner-scoped and authoritative under the global shell` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-054 | `UX-E2E-106-054 Cards benefits preserve one multi-category offer and shared cap under flat composition` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-055 | `UX-E2E-106-055 Cards selection lifecycle and keyboard alternatives remain complete under shared navigation` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-056 | `UX-E2E-106-056 Cards bonus and Calendar outcomes remain paired idempotent and truthful` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-057 | `UX-E2E-106-057 Cards optimization versions preserve manual choices compare and restore` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-058 | `UX-E2E-106-058 Cards sources preserve provenance disagreement and typed partial failure` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-059 | `UX-E2E-106-059 Cards audit remains immutable filterable owner-authorized and non-editable` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-060 | `UX-E2E-106-060 Cards missing required config remains Unavailable and value-safe` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-061 | `UX-E2E-106-061 Cards versioned import export dry-run conflict replay and refusal remain transactional` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-062 | `UX-E2E-106-062 Cards pending actions persist resolve and remain free of unread guilt counters` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-063 | `UX-E2E-106-063 Cards reports keep current historical stale no-match and failure states distinct` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-064 | `UX-E2E-106-064 Cards scheduled and manual operations deduplicate under one durable identity` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-065 | `UX-E2E-106-065 Cards seven local views and all deep links share one global session theme and state language` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-066 | `UX-E2E-106-066 Cards boundary failures retain safe input and never announce false success` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-067 | `UX-E2E-106-067 Cards representative workflows remain complete at 320px zoom keyboard screen reader motion color and themes` | `web/pwa/tests/coherent_cards.spec.ts` |
| UX-E2E-106-068 | `UX-E2E-106-068 Cards security adversaries fail closed without disclosure or mutation` | `web/pwa/tests/coherent_cards.spec.ts` |

## Implementation Plan

1. Map BUG-083's seven local views and existing ten deep links into global Work/Cards parent state without changing route methods, domain components, selectors, or owner behavior.
2. Remove the independent Card head/chrome only after BUG-083 Scope 14 and global shell canaries pass; preserve Card local view labels and all stable data hooks.
3. Map legacy Card token aliases to shared semantic tokens, then remove copied hardcoded values after visual parity proof. Keep dense tables/rows and reserve entity cards only for wallet cards; no nested cards or card-styled sections.
4. Apply shared workspace header, availability/state bands, evidence rows, mutation footer, icons/tooltips, responsive tracks, appearance, and global navigation around owner components.
5. Keep Import/Export a command workflow rather than a local navigation silo and preserve owner authorization for Sources/Audit/Admin views.
6. Execute all BUG-083 owner tests independently before the spec-106 shell loop. No global test can waive a failed Card row.

## Consumer Impact Sweep

Trace global/Card nav, local views, breadcrumbs, redirects, all deep links, forms, data hooks, browser history, assets/tokens, service worker, owner APIs, docs, tests, and CCManager references. Runtime CCManager traffic or duplicated domain logic is forbidden.

## Shared Infrastructure Impact And Rollback

Canary non-Card Assistant/Search shell, all existing Card deep links, Card PRG forms, auth/session, theme, and owner parity before removing Card chrome. Rollback preserves Card routes/domain data and returns a typed unavailable outer integration if needed; it never restores a separate Card login or weakens owner security.

## Change Boundary

**Allowed:** outer Cards shell/token/state composition, Work parent/local active state, Card template chrome adapters, focused cross-product tests.

**Excluded:** all BUG-083 domain behavior/migrations/tests, CCManager, broad shell redesign beyond planned foundation, foreign packets, spec 079, deployment, knb, release config, and readiness derivation.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-10-U | `unit` | `internal/web/coherent_cards_shell_test.go` | SCN-106-010, 011 | `TestCardsGlobalShellAdapterPreservesSevenViewsDeepLinksHooksAndOwnerStates` | `./smackerel.sh test unit --go` | No |
| XP106-10-I | `integration` | `tests/integration/experience/cards_shell_test.go` | SCN-106-010, 011 | `TestGlobalCardsCompositionPreservesBUG083RoutesPRGStateAndNoNestedCards` | `./smackerel.sh test integration` | Yes |
| XP106-10-A | `e2e-api` | `tests/e2e/cards_shell_e2e_test.go` | SCN-106-010 | `Cards outer composition preserves owner APIs auth readback errors and deep-link contracts` | `./smackerel.sh test e2e` | Yes |
| XP106-10-O | `functional` | BUG-083 evidence register | SCN-106-010, 011 | `TestCardsShellIntegrationRequiresAllEighteenBUG083ScopesCurrent` | `./smackerel.sh check` | No |
| UX-E2E-106-053 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-053 Cards wallet lifecycle remains owner-scoped and authoritative under the global shell` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-054 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-054 Cards benefits preserve one multi-category offer and shared cap under flat composition` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-055 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-055 Cards selection lifecycle and keyboard alternatives remain complete under shared navigation` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-056 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-056 Cards bonus and Calendar outcomes remain paired idempotent and truthful` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-057 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-057 Cards optimization versions preserve manual choices compare and restore` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-058 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-058 Cards sources preserve provenance disagreement and typed partial failure` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-059 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-059 Cards audit remains immutable filterable owner-authorized and non-editable` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-060 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-060 Cards missing required config remains Unavailable and value-safe` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-061 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-061 Cards versioned import export dry-run conflict replay and refusal remain transactional` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-062 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-062 Cards pending actions persist resolve and remain free of unread guilt counters` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-063 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-063 Cards reports keep current historical stale no-match and failure states distinct` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-064 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-064 Cards scheduled and manual operations deduplicate under one durable identity` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-065 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-011 | `UX-E2E-106-065 Cards seven local views and all deep links share one global session theme and state language` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-066 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-066 Cards boundary failures retain safe input and never announce false success` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-067 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-011 | `UX-E2E-106-067 Cards representative workflows remain complete at 320px zoom keyboard screen reader motion color and themes` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-068 | `e2e-ui` | `web/pwa/tests/coherent_cards.spec.ts` | SCN-106-010 | `UX-E2E-106-068 Cards security adversaries fail closed without disclosure or mutation` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-010 Card mutations keep authoritative owner outcomes`: every Card action under the global shell mirrors BUG-083 pending and terminal owner results, refreshes authoritative state, and never announces success early or retries twice.
- [ ] `SCN-106-011 Card workflows remain available on narrow layouts`: every BUG-083 field, action, status, local view, route, and security boundary remains visible, non-overlapping, touch-safe, and complete across desktop, tablet, 390px, and 320px composition.
- [ ] Shared assets/tokens/state language remove split chrome without nested cards, hardcoded visual drift, CCManager runtime traffic, or domain duplication.
- [ ] All BUG-083 owner evidence remains independently current, and rollback preserves routes/data/security.

#### Test Evidence - 20 Rows / 20 Items

- [ ] XP106-10-U passes with evidence in `report.md#xp106-10-u`.
- [ ] XP106-10-I passes with evidence in `report.md#xp106-10-i`.
- [ ] XP106-10-A passes with evidence in `report.md#xp106-10-a`.
- [ ] XP106-10-O passes with evidence in `report.md#xp106-10-o`.
- [ ] UX-E2E-106-053 passes with evidence in `report.md#ux-e2e-106-053`.
- [ ] UX-E2E-106-054 passes with evidence in `report.md#ux-e2e-106-054`.
- [ ] UX-E2E-106-055 passes with evidence in `report.md#ux-e2e-106-055`.
- [ ] UX-E2E-106-056 passes with evidence in `report.md#ux-e2e-106-056`.
- [ ] UX-E2E-106-057 passes with evidence in `report.md#ux-e2e-106-057`.
- [ ] UX-E2E-106-058 passes with evidence in `report.md#ux-e2e-106-058`.
- [ ] UX-E2E-106-059 passes with evidence in `report.md#ux-e2e-106-059`.
- [ ] UX-E2E-106-060 passes with evidence in `report.md#ux-e2e-106-060`.
- [ ] UX-E2E-106-061 passes with evidence in `report.md#ux-e2e-106-061`.
- [ ] UX-E2E-106-062 passes with evidence in `report.md#ux-e2e-106-062`.
- [ ] UX-E2E-106-063 passes with evidence in `report.md#ux-e2e-106-063`.
- [ ] UX-E2E-106-064 passes with evidence in `report.md#ux-e2e-106-064`.
- [ ] UX-E2E-106-065 passes with evidence in `report.md#ux-e2e-106-065`.
- [ ] UX-E2E-106-066 passes with evidence in `report.md#ux-e2e-106-066`.
- [ ] UX-E2E-106-067 passes with evidence in `report.md#ux-e2e-106-067`.
- [ ] UX-E2E-106-068 passes with evidence in `report.md#ux-e2e-106-068`.

#### Build Quality Gate

- [ ] BUG-083 owner suites, deep-link/consumer trace, no-CCManager-runtime, no-nested-card/token, security/privacy, responsive/a11y/theme, no-interception, bundle freshness, rollback, check, lint, format, artifact lint, traceability, and directly affected Card integration documentation checks pass with zero warnings.
