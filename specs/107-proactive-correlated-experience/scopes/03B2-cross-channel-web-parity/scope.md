# SCOPE-03B2: Cross-Channel Web Parity (Gated)

**Status:** Blocked  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-03B1, SCOPE-02  
**External Gate:** `specs/106-coherent-product-experience` shell + `SCOPE-02` web proactive card usable (the web side of the cross-channel parity assertion). Remains **Blocked** until spec-106 / `SCOPE-02` land.

## Outcome

Prove the **cross-channel act-once-suppressed-everywhere** parity assertion across
**web + Telegram + WhatsApp**: acting once on any channel routes one `content_key`
acknowledgement to the one controller, so the item is suppressed on every channel
within `suppression_window_hours`; budget exhaustion defers identically and urgent
escalation surfaces identically across all three channels; and a Telegram act and
a WhatsApp act each reflect on the spec-106 web card state — form varies,
controller truth does not.

This scope stays **honestly gated**. The Telegram single-channel rendering is
`SCOPE-03A` (delivered); the WhatsApp interactive rendering and the WhatsApp↔Telegram
single-pair suppression are `SCOPE-03B1` (buildable now). The
act-once-suppressed-**everywhere** assertion and the Telegram/WhatsApp→web
reflection need the spec-106 web card (`SCOPE-02`), so they live here and remain
`Blocked` until spec-106 / `SCOPE-02` are usable.

## Requirements And Scenarios

- FR-107-007 (cross-channel budget/dedupe/suppression across **every** channel), FR-107-008 (budget-defer / urgent-escalation parity across **every** channel), NFR-107-004 (cross-channel suppression propagation across web/Telegram/WhatsApp)
- SCN-107-007 (the cross-channel-everywhere scenario; SCN-107-005 Telegram render is `SCOPE-03A` and SCN-107-006 WhatsApp render is `SCOPE-03B1`)

```gherkin
Scenario: SCN-107-007 Acting on one channel suppresses the item on the others
  Given content_key "insight-7" was permitted on web, telegram, and whatsapp
  And the suppression_window_hours is 4
  When the user acts on the item from Telegram
  Then within 4 hours the same content_key is suppressed on web and whatsapp
  And no duplicate action prompt for "insight-7" is shown on any channel
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Act-once-suppressed-everywhere | `content_key "insight-7"` permitted on web/telegram/whatsapp; `suppression_window_hours=4`; spec-106 web card usable | Act from Telegram; re-render each channel | Within the window the same `content_key` is suppressed on web and whatsapp; no duplicate prompt on any channel (web assertion) | e2e-ui |
| Telegram→web reflection parity | `SCOPE-03A` Telegram render live + spec-106 web card | Act on Telegram; re-render the web card | The web card reflects the `content_key` suppression | e2e-ui |
| WhatsApp→web reflection parity | `SCOPE-03B1` WhatsApp render live + spec-106 web card | Act on WhatsApp; re-render the web card | The web card reflects the `content_key` suppression | e2e-ui |

## Implementation Plan

1. Consume the delivered `SCOPE-03A` Telegram render and the `SCOPE-03B1` WhatsApp
   render as the "act on Telegram" / "act on WhatsApp" sides of the parity
   assertion; do not re-implement either channel's `a:n:` render here.
2. Consume the `SCOPE-02` web proactive card + authenticated action transport as
   the web side of the assertion; do not re-implement the web card here.
3. Cross-channel parity: every channel renders one `permit`/`escalated` verdict
   and routes act/snooze/dismiss to the one `Acknowledge(content_key)`;
   suppression is already channel-agnostic (`content_key`-keyed on one
   process-wide registry), so acting once suppresses **everywhere** (web +
   Telegram + WhatsApp) within `suppression_window_hours` (NFR-107-004,
   FR-107-007). Budget exhaustion defers identically and urgent escalation
   surfaces identically across all three channels (FR-107-008). The identity join
   reuses each transport's existing per-user auth mapping (web cookie → principal,
   Telegram per-user token, WhatsApp verified phone) — no new identity store.
4. Prove the Telegram→web and WhatsApp→web reflection: acting on a messaging
   channel reflects the `content_key` suppression on the spec-106 web card
   re-render, with no duplicate prompt on any channel.
5. Keep only the bounded non-sensitive vocabulary on every wire (opaque
   `NudgeRef`, action, closed producer/channel/verdict/timing/count labels); no
   `content_key`, node label, or query text on any wire or telemetry label
   (FR-107-028, inherited).
6. Update transport/testing documentation through the docs owner during
   implementation; do not modify `specs/072-*`, `specs/078-*`, or `specs/106-*`
   (coordination notes only).

## SST No-Default Decision (Reserved)

- Snooze (MVP) reuses `suppression_window_hours` on every channel; MVP ships no
  distinct `snooze_window_hours` (SCOPE-01 decision; design.md OQ6). Cross-channel
  suppression propagation is validated within `suppression_window_hours`
  (NFR-107-004). No new SST key is introduced by this scope.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-106 web card (`SCOPE-02`); the `SCOPE-03A` Telegram rendering; the `SCOPE-03B1` WhatsApp rendering; the spec-078 `Channel`/`Producer` enums and `content_key`-keyed `Acknowledge`; each transport's existing per-user auth mapping; the SCOPE-01 `NudgeRef`/`NudgeAck`/`ProactiveCardModel` contract.
- **Independent canaries:** the existing controller suppression window is unchanged; the `SCOPE-03A` Telegram callbacks and the `SCOPE-03B1` WhatsApp reply-ids still decode; the spec-106 web card still renders and acts; existing web/Telegram/WhatsApp assistant turns still work.
- **Rollback:** the cross-channel parity assertion is additive (a proof over already-delivered renders + the web card); it introduces no new render surface; disabling it leaves the three renders and the controller untouched; no enum, budget, or store is mutated.

## Change Boundary

**Allowed during execution:** the cross-channel act-once-suppressed-**everywhere**
parity assertion (web + Telegram + WhatsApp), the Telegram→web and WhatsApp→web
reflection e2e, and tests/docs named by this scope.  
**Excluded:** re-implementing the `SCOPE-03A` Telegram render, the `SCOPE-03B1`
WhatsApp render, or the `SCOPE-02` web card; editing
`internal/intelligence/surfacing/types.go`; `specs/072-*`, `specs/078-*`, or
`specs/106-*`; the confirm/disambiguation/list callback families; a second budget
or suppression path; the cockpit/rail/palette/feed surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-007-U | Unit | `unit` | SCN-107-007 | `internal/intelligence/surfacing/cross_channel_ack_test.go` - `SCN-107-007 one Acknowledge(content_key) suppresses every channel` | `./smackerel.sh test unit` | No |
| T107-007-I | Integration | `integration` | SCN-107-007 | `tests/integration/proactive/cross_channel_suppression_test.go` - `SCN-107-007 act on Telegram suppresses web and whatsapp in window` | `./smackerel.sh test integration` | Yes |
| T107-007-A | E2E API regression | `e2e-api` | SCN-107-007 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-007 cross-channel suppression API` | `./smackerel.sh test e2e` | Yes |
| T107-007-W | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 no duplicate prompt on web after acting on Telegram` | `./smackerel.sh test e2e-ui` | Yes |
| T107-03-SUPPRESS | Stress | `stress` | SCN-107-007 | `tests/stress/proactive_suppression_propagation_test.go` - `NFR-107-004 suppression propagates within suppression_window_hours across channels` | `./smackerel.sh test stress` | Yes |
| T107-03B2-TGWEB-PARITY | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 Telegram act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-03B2-WAWEB-PARITY | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 WhatsApp act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-007 Acting on one channel suppresses the item on the others: acting from Telegram suppresses the same `content_key` on web and whatsapp within `suppression_window_hours`, with no duplicate action prompt on any channel.
- [ ] Cross-channel parity: every channel renders one `permit`/`escalated` verdict and routes act/snooze/dismiss to the one `Acknowledge(content_key)`; budget-defer and urgent-escalation are identical across web + Telegram + WhatsApp.
- [ ] The Telegram→web and WhatsApp→web reflection each hold: acting on a messaging channel reflects the `content_key` suppression on the spec-106 web card re-render, with no duplicate prompt on any channel.
- [ ] No `content_key`, node label, or query text reaches any wire or telemetry label across the cross-channel assertion (FR-107-028, inherited).

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-007-U passes with current-session evidence in `report.md#t107-007-u`.
- [ ] T107-007-I passes against the disposable stack with current-session evidence in `report.md#t107-007-i`.
- [ ] T107-007-A passes through production routes with current-session evidence in `report.md#t107-007-a`.
- [ ] T107-007-W passes without interception and proves no duplicate prompt after acting elsewhere in `report.md#t107-007-w`.
- [ ] T107-03-SUPPRESS proves suppression propagation within the window across channels in `report.md#t107-03-suppress`.
- [ ] T107-03B2-TGWEB-PARITY proves the Telegram act reflects on the web card state in `report.md#t107-03b2-tgweb-parity`.
- [ ] T107-03B2-WAWEB-PARITY proves the WhatsApp act reflects on the web card state in `report.md#t107-03b2-waweb-parity`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, transport documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, authored tests, and runtime
validation have not been executed by the planning owner, and this scope is
honestly gated on `SCOPE-02` / spec-106 (the web card). The Telegram render
(`SCOPE-03A`) is delivered and the WhatsApp render (`SCOPE-03B1`) is buildable now,
but the act-once-suppressed-**everywhere** assertion and the messaging→web
reflection require the spec-106 web card. `specs/072-*`, `specs/078-*`, and
`specs/106-*` are consume-only dependencies and are not modified.
