# SCOPE-09: Real-Stack Acceptance & Implementation Handoff

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-08

## Outcome

Prove the whole proactive experience end-to-end on the real disposable stack:
inspect that every proactive card on every surface and channel corresponds to a
`permit`/`escalated` verdict and that none bypassed `controller.Propose`
(SCN-107-016); run the complete no-interception Playwright web matrix; cover the
Telegram and WhatsApp renderings at adapter level; exercise the full honest-state
matrix (budget-exhausted, deduped, suppressed, no-related-items, unavailable); and
re-run SCN-107-001..020 as the acceptance regression. Produce a value-safe
implementation handoff that makes no implementation, migration, browser-executed,
or deployment claim in this planning packet.

## Requirements And Scenarios

- FR-107-003, FR-107-006, FR-107-007, FR-107-008, FR-107-009, FR-107-022, NFR-107-004
- SCN-107-016 (acceptance rerun of SCN-107-001..020)

```gherkin
Scenario: SCN-107-016 Every card originates from the controller
  Given any proactive card rendered on any channel
  When its origin is inspected
  Then it corresponds to a permit or escalated verdict from the surfacing controller
  And no card was produced by a path that bypasses controller.Propose
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Controller-origin invariant | Disposable stack; cards rendered on web/Telegram/WhatsApp; valid scoped session | Inspect the origin of every rendered card across every surface/channel | Each corresponds to a `permit`/`escalated` verdict; no card was produced by a path that bypassed `controller.Propose`; no parallel surfacing path exists | e2e-ui |
| Acceptance matrix | The full stack after SCOPE-01..08 | Run SCN-107-001..020 without interception | Every scenario passes on the real stack with direct user-visible assertions; no `page.route`/`context.route`/MSW/Nock | e2e-ui |
| Channel parity | Bound Telegram + WhatsApp identities | Exercise both messaging renderings and the cross-channel ack | Telegram/WhatsApp adapter-level parity holds; act-once-suppressed-everywhere within the window | integration / e2e-api |
| Honest-state matrix | Seeded budget-exhausted, deduped, suppressed, no-related, unavailable conditions | Render each condition on each surface | Each renders distinctly and never as a normal card or a decorative correlation | e2e-ui |

## Implementation Plan

1. Assert the controller-origin invariant (SCN-107-016): for every card rendered on web, Telegram, and WhatsApp, prove its origin is a `permit`/`escalated` verdict from `controller.Propose` and that no composition path produced a card bypassing the controller, added a second budget, or introduced a parallel surfacing path.
2. Run the complete no-interception Playwright web matrix over the cockpit, card, rail, palette, and feed; every assertion combines the controller verdict, the real producer-derived provenance, and the honest-state token; element existence alone cannot pass; no `page.route`/`context.route`/`route.fulfill`/MSW/Nock.
3. Cover the Telegram (`a:n:`) and WhatsApp (interactive + fallback) renderings at adapter level against the live stack, including the cross-channel ack propagation within `suppression_window_hours` (NFR-107-004) and identical budget-defer / urgent-escalation.
4. Exercise the honest-state matrix: budget-exhausted, deduped, suppressed, no-related-items, degraded, and unavailable each render distinctly and never as a normal card or a decorative correlation (FR-107-022).
5. Re-run SCN-107-001..020 as the acceptance regression across all surfaces and channels; every mutation uses disposable, ephemeral state (no cleanup-based isolation); the `NudgeRef` registry is process-local and dropped on restart.
6. Produce the value-safe implementation handoff: the test inventory (`test-plan.json`), the scenario contracts (`scenario-manifest.json`, all PLANNED), the reserved SST no-default keys (`RAIL_MAX`, `what_changed_page_cap`, `nudge_ref_ttl_hours`; snooze reuses `suppression_window_hours`), the coordination notes (spec-078 `whatsapp` enum; spec-105 seed/launch; spec-106 landing route), and the migration reservation (none expected; the `NudgeRef` registry is in-memory — a number is allocated only at pickup if implementation proves one is needed). This planning packet makes no implementation, migration, browser-executed, or deployment claim.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** every owner contract exercised end-to-end — the spec-078 controller/verdict/budget/suppression, the spec-106 shell/session, the spec-105 neighborhood/deep-link, the spec-072 WhatsApp transport, the spec-074/061/073 capture/turn, `agent_traces`/topic lifecycle — plus the disposable Compose stack, seeded PostgreSQL, and the no-interception harness.
- **Independent canaries:** the existing surfacing budget/dedupe/suppression, the existing Telegram/WhatsApp assistant turns, the existing assistant capture/correction path, the spec-105 explorer deep-link, and the authenticated shell all stay green under the acceptance run.
- **Rollback:** acceptance mutates no owner store; every proactive surface is additive; disabling any surface is an explicit honest state; the `NudgeRef` registry is dropped on restart; rollback is a source/config pointer swap.

## Change Boundary

**Allowed during execution:** the acceptance test matrix (no-interception
Playwright web + adapter-level Telegram/WhatsApp + honest-state), the controller-
origin invariant assertions, and the implementation-handoff record.  
**Excluded:** editing any owner spec or contract; introducing an implementation,
migration, browser-executed, or deployment claim in this planning packet; adding a
second store, budget, or surfacing path.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-016-U | Unit | `unit` | SCN-107-016 | `internal/web/proactive/card_origin_invariant_test.go` - `SCN-107-016 a card is built only from a permit/escalated verdict` | `./smackerel.sh test unit` | No |
| T107-016-I | Integration | `integration` | SCN-107-016 | `tests/integration/proactive/controller_origin_test.go` - `SCN-107-016 no card bypasses controller.Propose on any channel` | `./smackerel.sh test integration` | Yes |
| T107-016-A | E2E API regression | `e2e-api` | SCN-107-016 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-016 controller-origin invariant API` | `./smackerel.sh test e2e` | Yes |
| T107-016-W | E2E UI regression | `e2e-ui` | SCN-107-016 | `web/pwa/tests/proactive-acceptance.spec.ts` - `SCN-107-016 every rendered card originates from the controller` | `./smackerel.sh test e2e-ui` | Yes |
| T107-09-MATRIX | E2E UI acceptance | `e2e-ui` | SCN-107-016 | `web/pwa/tests/proactive-acceptance.spec.ts` - `acceptance rerun of SCN-107-001..020 with no interception` | `./smackerel.sh test e2e-ui` | Yes |
| T107-09-CHANNELS | Integration | `integration` | SCN-107-016 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `Telegram + WhatsApp adapter-level parity and cross-channel ack` | `./smackerel.sh test e2e` | Yes |
| T107-09-HONEST | E2E UI acceptance | `e2e-ui` | SCN-107-016 | `web/pwa/tests/proactive-acceptance.spec.ts` - `honest-state matrix: budget-exhausted, deduped, suppressed, no-related, unavailable render distinctly` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-016 Every card originates from the controller: every card on every surface and channel corresponds to a `permit`/`escalated` verdict, and no card was produced by a path that bypasses `controller.Propose` (no parallel surfacing path, no second budget).
- [ ] The complete no-interception Playwright web matrix and the SCN-107-001..020 acceptance rerun pass on the real disposable stack with direct user-visible assertions and no request interception.
- [ ] Telegram and WhatsApp adapter-level parity holds (act-once-suppressed-everywhere within `suppression_window_hours`, identical budget-defer and urgent-escalation), and the honest-state matrix (budget-exhausted, deduped, suppressed, no-related-items, unavailable) renders each state distinctly and never as a normal card.
- [ ] The implementation handoff records the test inventory, the PLANNED scenario contracts, the reserved SST no-default keys, the coordination notes, and the migration reservation, making no implementation, migration, browser-executed, or deployment claim in this planning packet.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-016-U passes with current-session evidence in `report.md#t107-016-u`.
- [ ] T107-016-I passes against the disposable stack with current-session evidence in `report.md#t107-016-i`.
- [ ] T107-016-A passes through production HTTP routes with current-session evidence in `report.md#t107-016-a`.
- [ ] T107-016-W passes without interception and proves the controller-origin invariant in `report.md#t107-016-w`.
- [ ] T107-09-MATRIX runs the SCN-107-001..020 acceptance rerun without interception in `report.md#t107-09-matrix`.
- [ ] T107-09-CHANNELS proves Telegram + WhatsApp adapter-level parity and cross-channel ack in `report.md#t107-09-channels`.
- [ ] T107-09-HONEST proves the honest-state matrix renders each state distinctly in `report.md#t107-09-honest`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence, and the value-safe implementation handoff makes no implementation/migration/browser/deployment claim.

## Uncertainty Declaration

All items remain unchecked because implementation, authored tests, migration,
browser verification, and deployment acceptance were not executed by the planning
owner. This scope is the acceptance + handoff contract; its execution belongs to
`bubbles.implement`/`bubbles.test`/`bubbles.validate` at pickup, not to this
planning packet.
