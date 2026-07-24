# SCOPE-106-06: Search Today Digest And Synthesis Composition

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gates:** BUG-002-006, BUG-002-007, and BUG-004-004 complete owner evidence.

## Outcome

Search and Today compose repaired owner truth under the shared shell. The scope adds no search engine, digest SQL, synthesis persistence, health derivation, or new data endpoint; it aligns layout, tokens, state presentation, context return, source disclosure, and cross-surface journeys.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-006 Search submits and renders a terminal state
  Given an authenticated user enters a non-empty query
  When the semantic baseline or source-locked enhancement submits
  Then exactly one real request reaches the repaired Search owner
  And loading resolves to results no matches degraded output or a useful error

Scenario: SCN-106-007 Stored digest renders instead of false empty
  Given the owner store contains a current non-empty digest
  When Today opens
  Then exact digest content date and sources render under the shared state contract
  And read decode auth stale quiet synthesis and first-use states remain mutually truthful
```

## Ownership Boundary

- BUG-002-006 owns same-origin HTMX, semantic form, exactly-one Search submission, Search states, and Search fault fixtures.
- BUG-002-007 owns canonical typed Digest reads, date semantics, freshness, false-empty exclusion, and auth/privacy.
- BUG-004-004 owns synthesis persistence, citations, run/health lifecycle, Retry, and API truth.
- Spec 106 owns only shared shell placement, Today composition, shared tokens/state bands, source/evidence row layout, context restoration, and cross-surface assertions.

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-025 | `UX-E2E-106-025 Search submits one real query and restores result context` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-026 | `UX-E2E-106-026 native Search remains complete when enhancement is unavailable and asset integrity stays strict` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-027 | `UX-E2E-106-027 whitespace Search focuses linked validation and sends no request` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-028 | `UX-E2E-106-028 Search no-match auth timeout network and server states are mutually exclusive` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-029 | `UX-E2E-106-029 Search preserves verified partial results with Degraded provenance` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-030 | `UX-E2E-106-030 Today renders current digest content and database calendar date without false empty` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-031 | `UX-E2E-106-031 Today renders persisted quiet digest without never-generated copy` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-032 | `UX-E2E-106-032 Today shows true first-use empty only after successful no-row read` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-033 | `UX-E2E-106-033 Today keeps stale digest and typed failures distinct from current and empty` | `web/pwa/tests/coherent_search_today.spec.ts` |
| UX-E2E-106-050 | `UX-E2E-106-050 synthesis never-run current quiet stale partial and failed states remain durable and distinct` | `web/pwa/tests/coherent_search_today.spec.ts` |

## Implementation Plan

1. Apply the shared workspace header, Search form/result rows, Today bands, state presenter, evidence rows, typography, controls, and responsive tracks to owner-delivered models.
2. Preserve `/` and `POST /search`; the shell adds no fetch/submit logic. Back restores query, filters, scroll, and focus through browser history, not a client business-data cache.
3. Preserve `/digest`; label it Today without adding `/today`. Compose base Digest and durable Synthesis as independent owner states so one failure cannot erase or fabricate the other.
4. Render current, quiet, first-use empty, selected-date empty, stale, degraded, unauthorized, and error states using exact owner evidence. No unread, missed-days, backlog, sample prose, or current-date substitution.
5. Keep source links/provenance authorized and safe. Failed reads clear affected content but not independently verified sibling content.
6. Run owner regression suites unchanged, then cross-surface Search-to-detail-to-Back and Today-to-source-to-Back journeys in the shared shell.

## Consumer Impact Sweep

Preserve Search/Digest routes, forms, HTMX targets, owner DTOs, browser history, source links, Today label, deep links, service-worker network policy, metrics, docs, tests, and stable hooks. Remove no owner field or selector until owner regressions and stale-reference scans pass.

## Change Boundary

**Allowed:** Search/Today/Synthesis shared composition, tokens/state primitives, context/history adapters, focused cross-surface tests.

**Excluded:** Search/Digest/Synthesis domain logic, SQL, APIs, persistence, scheduler, health, fault controls, auth, owner tests, foreign packets, spec 079, deployment, knb, CCManager, and readiness claims.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-06-U | `unit` | `internal/web/coherent_search_today_test.go` | SCN-106-006, 007 | `TestSearchTodayCompositionConsumesOwnerStatesWithoutTransportOrPersistenceLogic` | `./smackerel.sh test unit --go` | No |
| XP106-06-I | `integration` | `tests/integration/experience/search_today_composition_test.go` | SCN-106-006, 007 | `TestSearchDigestAndSynthesisOwnerModelsRemainIndependentUnderSharedComposition` | `./smackerel.sh test integration` | Yes |
| XP106-06-A | `e2e-api` | `tests/e2e/search_today_composition_e2e_test.go` | SCN-106-006, 007 | `Search Today and synthesis routes preserve owner HTTP state date and provenance contracts` | `./smackerel.sh test e2e` | Yes |
| XP106-06-O | `functional` | owner Search Digest and synthesis test-plan evidence | SCN-106-006, 007 | `TestSearchTodayCompositionRequiresCurrentIndependentOwnerRegressionEvidence` | `./smackerel.sh check` | No |
| UX-E2E-106-025 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-006 | `UX-E2E-106-025 Search submits one real query and restores result context` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-026 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-006 | `UX-E2E-106-026 native Search remains complete when enhancement is unavailable and asset integrity stays strict` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-027 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-006 | `UX-E2E-106-027 whitespace Search focuses linked validation and sends no request` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-028 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-006 | `UX-E2E-106-028 Search no-match auth timeout network and server states are mutually exclusive` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-029 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-006 | `UX-E2E-106-029 Search preserves verified partial results with Degraded provenance` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-030 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-007 | `UX-E2E-106-030 Today renders current digest content and database calendar date without false empty` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-031 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-007 | `UX-E2E-106-031 Today renders persisted quiet digest without never-generated copy` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-032 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-007 | `UX-E2E-106-032 Today shows true first-use empty only after successful no-row read` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-033 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-007 | `UX-E2E-106-033 Today keeps stale digest and typed failures distinct from current and empty` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-050 | `e2e-ui` | `web/pwa/tests/coherent_search_today.spec.ts` | SCN-106-007 | `UX-E2E-106-050 synthesis never-run current quiet stale partial and failed states remain durable and distinct` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-006 and SCN-106-007 complete through shared composition without duplicating owner transport, reader, persistence, or health logic.
- [ ] Search request cardinality/context and Today Digest/Synthesis state independence remain truthful, private, responsive, and accessible.
- [ ] Owner regression evidence, consumer compatibility, and stale-reference scans remain current.

#### Test Evidence - 14 Rows / 14 Items

- [ ] XP106-06-U passes with evidence in `report.md#xp106-06-u`.
- [ ] XP106-06-I passes with evidence in `report.md#xp106-06-i`.
- [ ] XP106-06-A passes with evidence in `report.md#xp106-06-a`.
- [ ] XP106-06-O passes with evidence in `report.md#xp106-06-o`.
- [ ] UX-E2E-106-025 passes with evidence in `report.md#ux-e2e-106-025`.
- [ ] UX-E2E-106-026 passes with evidence in `report.md#ux-e2e-106-026`.
- [ ] UX-E2E-106-027 passes with evidence in `report.md#ux-e2e-106-027`.
- [ ] UX-E2E-106-028 passes with evidence in `report.md#ux-e2e-106-028`.
- [ ] UX-E2E-106-029 passes with evidence in `report.md#ux-e2e-106-029`.
- [ ] UX-E2E-106-030 passes with evidence in `report.md#ux-e2e-106-030`.
- [ ] UX-E2E-106-031 passes with evidence in `report.md#ux-e2e-106-031`.
- [ ] UX-E2E-106-032 passes with evidence in `report.md#ux-e2e-106-032`.
- [ ] UX-E2E-106-033 passes with evidence in `report.md#ux-e2e-106-033`.
- [ ] UX-E2E-106-050 passes with evidence in `report.md#ux-e2e-106-050`.

#### Build Quality Gate

- [ ] Owner suites, state exclusivity, privacy, no-interception, browser history, accessibility, responsive layout, consumer trace, check, lint, format, artifact lint, traceability, and directly affected user/testing documentation checks pass with zero warnings.
