# SCOPE-03B: WhatsApp Interactive + Cross-Channel Parity (Gated)

**Status:** Blocked  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-02, SCOPE-03A  
**External Gate:** `specs/072-whatsapp-business-transport` interactive transport usable + `specs/106-coherent-product-experience` shell usable (for the web side of the cross-channel parity assertion). Remains blocked until spec-072 and spec-106/`SCOPE-02` land.

## Outcome

Render the same `permit`/`escalated` card on WhatsApp via the spec-072 interactive
transport (three reply buttons + list/text fallback, `reply.id =
a:n:<ref>:<a|s|d>`), and prove the **cross-channel act-once-suppressed-everywhere**
parity assertion: acting once on any channel routes one `content_key`
acknowledgement to the one controller, so the item is suppressed on **web +
Telegram + WhatsApp** within `suppression_window_hours`, budget exhaustion defers
identically, and urgent escalation surfaces identically — form varies, controller
truth does not.

This scope stays **honestly gated**. The Telegram single-channel rendering is
delivered by `SCOPE-03A` (buildable now); the WhatsApp interactive rendering
needs the spec-072 transport, and the cross-channel-**everywhere** assertion needs
the spec-106 web card (`SCOPE-02`). Neither WhatsApp nor web parity is folded into
`SCOPE-03A`.

## Requirements And Scenarios

- FR-107-007 (cross-channel budget/dedupe/suppression across every channel), FR-107-008 (budget-defer / urgent-escalation parity across channels), FR-107-009 (WhatsApp interactive rendering portion), FR-107-011 (WhatsApp uses the spec-072 interactive transport with text fallback), FR-107-012 (`whatsapp` `Channel` enum extension decision), FR-107-028, NFR-107-004 (cross-channel suppression propagation across web/Telegram/WhatsApp)
- SCN-107-006, SCN-107-007 (SCN-107-005 Telegram rendering is `SCOPE-03A`)

```gherkin
Scenario: SCN-107-006 WhatsApp renders the nudge as an interactive message
  Given the same candidate is permitted for the whatsapp channel
  When the nudge is delivered to the user on WhatsApp
  Then it renders as an interactive message offering act, snooze, and dismiss with a provenance line
  And choosing an action routes to the same surfacing acknowledgement path
```

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
| WhatsApp interactive | A candidate permitted for whatsapp; verified phone mapping; spec-072 transport usable | Deliver the interactive message; choose an action | Body + `Why:` line + three reply buttons (list/text fallback preserved); `reply.id` carries the same `a:n:<ref>:<a\|s\|d>`; choosing routes to the same ack path | integration / e2e-api |
| Act-once-suppressed-everywhere | `content_key "insight-7"` permitted on web/telegram/whatsapp; `suppression_window_hours=4`; spec-106 web card usable | Act from Telegram; re-render each channel | Within the window the same `content_key` is suppressed on web and whatsapp; no duplicate prompt on any channel (web assertion) | e2e-ui |
| Telegram→web reflection parity | `SCOPE-03A` Telegram render live + spec-106 web card | Act on Telegram; re-render the web card | The web card reflects the `content_key` suppression (the cross-channel reflection formerly bundled into the Telegram row, now the parity assertion) | e2e-ui |

## Implementation Plan

1. WhatsApp: use the spec-072 interactive transport (consume, never re-own its
   webhook verification or mapping table). Send three reply buttons
   `Act`/`Snooze`/`Dismiss` with `reply.id = "a:n:<ref>:<a|s|d>"`; provide the
   list-message and numbered plain-text fallbacks (`1|2|3` → `a|s|d`) that
   preserve the same three actions and the provenance line. Resolve
   `interactive.button_reply.id`/`list_reply.id` through the same SCOPE-01
   `NudgeRef` registry to the same `NudgeAck` path (FR-107-011).
2. Reserve `whatsapp` as a new bounded `Channel` enum value via the documented
   `ProducerNotification` extension precedent, routed as a **coordination note to
   the spec-078 enum owner**; `internal/intelligence/surfacing/types.go` is NOT
   edited here (FR-107-012). Its Prometheus-cardinality justification (finite
   bounded enum) is recorded for the owner.
3. Cross-channel parity: every channel renders one `permit`/`escalated` verdict
   and routes act/snooze/dismiss to the one `Acknowledge(content_key)`;
   suppression is already channel-agnostic (`content_key`-keyed on one
   process-wide registry), so acting once suppresses everywhere within
   `suppression_window_hours` (NFR-107-004, FR-107-007). Budget exhaustion defers
   identically and urgent escalation surfaces identically across web + Telegram +
   WhatsApp (FR-107-008). The identity join reuses each transport's existing
   per-user auth mapping (web cookie → principal, Telegram per-user token,
   WhatsApp verified phone) — no new identity store.
4. Consume the `SCOPE-03A` Telegram rendering as the "act on Telegram" side of the
   parity assertion; do not re-implement the Telegram `a:n:` render here.
5. Keep only the bounded non-sensitive vocabulary on every wire (opaque
   `NudgeRef`, action, closed producer/channel/verdict/timing/count labels); no
   `content_key`, node label, or query text on any `reply.id` or telemetry label
   (FR-107-028).
6. Update transport/testing documentation through the docs owner during
   implementation; do not modify `specs/072-*` or `specs/078-*` (coordination
   notes only).

## SST No-Default Decision (Reserved)

- Snooze (MVP) reuses `suppression_window_hours` on every channel:
  WhatsApp/web/Telegram `Snooze` calls the same `Acknowledge(content_key)`; MVP
  ships no distinct `snooze_window_hours` (SCOPE-01 decision; design.md OQ6).
  Cross-channel suppression propagation is validated within
  `suppression_window_hours` (NFR-107-004).

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-072 WhatsApp transport, webhook verification, and `AssistantResponse → WhatsApp` mapping; the spec-078 `Channel`/`Producer` enums and `content_key`-keyed `Acknowledge`; the spec-106 web card (`SCOPE-02`); the `SCOPE-03A` Telegram rendering; each transport's existing per-user auth mapping.
- **Independent canaries:** the existing WhatsApp transport send/receive stays green; the existing controller suppression window is unchanged; the `SCOPE-03A` Telegram callbacks still decode; existing WhatsApp/web assistant turns still work.
- **Rollback:** the WhatsApp nudge rendering and the cross-channel parity assertion are additive; disabling them leaves the existing transport, the Telegram rendering, and the web card untouched; no enum, budget, or store is mutated (the `whatsapp` enum edit is the spec-078 owner's, not made here).

## Change Boundary

**Allowed during execution:** the WhatsApp nudge interactive/list/text rendering
over the spec-072 transport, the shared-registry resolution for WhatsApp, the
cross-channel act-once-suppressed-everywhere parity assertion (web + Telegram +
WhatsApp), the Telegram→web reflection e2e, and tests/docs named by this scope.  
**Excluded:** re-implementing the `SCOPE-03A` Telegram `a:n:` inline render;
editing `internal/intelligence/surfacing/types.go` (the `whatsapp` enum is a
coordination note); `specs/072-*` or `specs/078-*`; the confirm/disambiguation/list
callback families; a second budget or suppression path; the cockpit/rail/palette/feed
surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-006-U | Unit | `unit` | SCN-107-006 | `internal/whatsapp/nudge_reply_test.go` - `SCN-107-006 WhatsApp reply.id carries a:n:<ref>:<a\|s\|d>` | `./smackerel.sh test unit` | No |
| T107-006-I | Integration | `integration` | SCN-107-006 | `tests/integration/proactive/whatsapp_nudge_ack_test.go` - `SCN-107-006 WhatsApp interactive choice routes to the one ack path` | `./smackerel.sh test integration` | Yes |
| T107-006-A | E2E API regression | `e2e-api` | SCN-107-006 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-006 WhatsApp interactive nudge acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-006-W | E2E UI regression | `e2e-ui` | SCN-107-006 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-006 WhatsApp act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-007-U | Unit | `unit` | SCN-107-007 | `internal/intelligence/surfacing/cross_channel_ack_test.go` - `SCN-107-007 one Acknowledge(content_key) suppresses every channel` | `./smackerel.sh test unit` | No |
| T107-007-I | Integration | `integration` | SCN-107-007 | `tests/integration/proactive/cross_channel_suppression_test.go` - `SCN-107-007 act on Telegram suppresses web and whatsapp in window` | `./smackerel.sh test integration` | Yes |
| T107-007-A | E2E API regression | `e2e-api` | SCN-107-007 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-007 cross-channel suppression API` | `./smackerel.sh test e2e` | Yes |
| T107-007-W | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 no duplicate prompt on web after acting on Telegram` | `./smackerel.sh test e2e-ui` | Yes |
| T107-03-SUPPRESS | Stress | `stress` | SCN-107-007 | `tests/stress/proactive_suppression_propagation_test.go` - `NFR-107-004 suppression propagates within suppression_window_hours across channels` | `./smackerel.sh test stress` | Yes |
| T107-03B-TGWEB-PARITY | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 Telegram act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-006 WhatsApp renders the nudge as an interactive message: the nudge renders as a spec-072 interactive message (three reply buttons + list/text fallback) with a provenance line, and choosing an action routes to the same surfacing acknowledgement path.
- [ ] SCN-107-007 Acting on one channel suppresses the item on the others: acting from Telegram suppresses the same `content_key` on web and whatsapp within `suppression_window_hours`, with no duplicate action prompt on any channel.
- [ ] Cross-channel parity: every channel renders one `permit`/`escalated` verdict and routes act/snooze/dismiss to the one `Acknowledge(content_key)`; budget-defer and urgent-escalation are identical across web + Telegram + WhatsApp; `whatsapp` is reserved as a coordination note and `types.go` is not edited.
- [ ] No `content_key`, node label, or query text reaches any `reply.id` or telemetry label; the WhatsApp `reply.id` carries only the opaque `a:n:<ref>:<a|s|d>` shape.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-006-U passes with current-session evidence in `report.md#t107-006-u`.
- [ ] T107-006-I passes against the disposable stack with current-session evidence in `report.md#t107-006-i`.
- [ ] T107-006-A passes through production routes with current-session evidence in `report.md#t107-006-a`.
- [ ] T107-006-W passes without interception and proves cross-channel web reflection in `report.md#t107-006-w`.
- [ ] T107-007-U passes with current-session evidence in `report.md#t107-007-u`.
- [ ] T107-007-I passes against the disposable stack with current-session evidence in `report.md#t107-007-i`.
- [ ] T107-007-A passes through production routes with current-session evidence in `report.md#t107-007-a`.
- [ ] T107-007-W passes without interception and proves no duplicate prompt after acting elsewhere in `report.md#t107-007-w`.
- [ ] T107-03-SUPPRESS proves suppression propagation within the window across channels in `report.md#t107-03-suppress`.
- [ ] T107-03B-TGWEB-PARITY proves the Telegram act reflects on the web card state in `report.md#t107-03b-tgweb-parity`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, transport documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner, and this scope is honestly gated on
`specs/072-*` (WhatsApp transport) + `SCOPE-02`/spec-106 (web card). `specs/072-*`
and `specs/078-*` are consume-only dependencies and are not modified; the
`whatsapp` `Channel` value is a coordination note to the spec-078 owner.
