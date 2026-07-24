# SCOPE-03: Telegram & WhatsApp Nudge Renderings + Cross-Channel Parity

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-02

## Outcome

Render the same `permit`/`escalated` card channel-appropriately on Telegram (a new
additive inline `a:n:` callback family) and WhatsApp (the spec-072 interactive
transport: three reply buttons + list/text fallback), both resolving through the
shared SCOPE-01 `NudgeRef` registry to the one `NudgeAck` path. Prove
cross-frontend parity: acting once on any channel routes one `content_key`
acknowledgement to the one controller, so the item is suppressed on every channel
within `suppression_window_hours`, budget exhaustion defers identically, and
urgent escalation surfaces identically — form varies, controller truth does not.

## Requirements And Scenarios

- FR-107-007, FR-107-008, FR-107-009, FR-107-010, FR-107-011, FR-107-012, FR-107-028, NFR-107-004
- SCN-107-005, SCN-107-006, SCN-107-007

```gherkin
Scenario: SCN-107-005 Telegram renders the nudge as inline actions
  Given the same candidate is permitted for the telegram channel
  When the nudge is delivered to the user on Telegram
  Then it renders as a message with inline act, snooze, and dismiss buttons and a provenance line
  And tapping an action routes to the same surfacing acknowledgement path
```

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
| Telegram inline actions | Disposable stack; a candidate permitted for telegram; bound Telegram identity | Deliver the nudge; tap an inline action | Message body + `Why:` line + inline `Act`/`Snooze`/`Dismiss` encoded `a:n:<ref>:<a\|s\|d>`; tapping routes through the shared registry to the one ack path; the message edits in place | integration / e2e-api |
| WhatsApp interactive | A candidate permitted for whatsapp; verified phone mapping | Deliver the interactive message; choose an action | Body + `Why:` line + three reply buttons (list/text fallback preserved); `reply.id` carries the same `a:n:<ref>:<a\|s\|d>`; choosing routes to the same ack path | integration / e2e-api |
| Act-once-suppressed-everywhere | `content_key "insight-7"` permitted on web/telegram/whatsapp; `suppression_window_hours=4` | Act from Telegram; re-render each channel | Within the window the same `content_key` is suppressed on web and whatsapp; no duplicate prompt on any channel (web assertion) | e2e-ui |
| Non-collision | Existing Telegram callbacks live | Decode `a:c:`/`a:d:`/spec-028 and `a:n:` payloads | `a:n:` decodes to `callbackKindNudge` and never collides with the confirm/disambiguation/list families; all stay inside 64 bytes | unit |

## Implementation Plan

1. Telegram: add the additive `callbackNudgePrefix = "a:n:"` and a `decodeNudge` producing a new `callbackKindNudge` by extending `decodeCallbackData`'s switch in `internal/telegram/assistant_adapter/callbacks.go`; `encodeNudgeCallback(ref, action) → "a:n:<ref>:<a|s|d>"` (~32 bytes, inside the 64-byte `callback_data` bound), never colliding with `a:c:`/`a:d:`/the spec-028 list family. Render one inline row `Act`/`Snooze`/`Dismiss` plus the `Why:` provenance line; tapping edits the message in place and removes the buttons once terminal.
2. WhatsApp: use the spec-072 interactive transport (consume, never re-own its webhook verification or mapping table). Send three reply buttons `Act`/`Snooze`/`Dismiss` with `reply.id = "a:n:<ref>:<a|s|d>"`; provide the list-message and numbered plain-text fallbacks (`1|2|3` → `a|s|d`) that preserve the same three actions and the provenance line. Resolve `interactive.button_reply.id`/`list_reply.id` through the same `NudgeRef` registry to the same `NudgeAck` path.
3. Reserve `whatsapp` as a new bounded `Channel` enum value via the documented `ProducerNotification` extension precedent, routed as a **coordination note to the spec-078 enum owner**; `internal/intelligence/surfacing/types.go` is NOT edited here. Its Prometheus-cardinality justification (finite bounded enum) is recorded for the owner.
4. Cross-channel parity: every channel renders one `permit`/`escalated` verdict and routes act/snooze/dismiss to the one `Acknowledge(content_key)`; suppression is already channel-agnostic (`content_key`-keyed on one process-wide registry), so acting once suppresses everywhere within `suppression_window_hours` (NFR-107-004). The identity join reuses each transport's existing per-user auth mapping (web cookie → principal, Telegram per-user token, WhatsApp verified phone) — no new identity store.
5. Keep only the bounded non-sensitive vocabulary on every wire (opaque `NudgeRef`, action, closed producer/channel/verdict/timing/count labels); no `content_key`, node label, or query text on any `callback_data`, `reply.id`, or telemetry label (FR-107-028).
6. Update transport/testing documentation through the docs owner during implementation; do not modify `specs/072-*` or `specs/078-*` (coordination notes only).

## SST No-Default Decision (Reserved)

- Snooze (MVP) reuses `suppression_window_hours` on every channel: Telegram/WhatsApp/web `Snooze` calls the same `Acknowledge(content_key)`; MVP ships no distinct `snooze_window_hours` (SCOPE-01 decision; design.md OQ6). Cross-channel suppression propagation is validated within `suppression_window_hours` (NFR-107-004).

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the Telegram `a:` callback namespace (`a:c:`/`a:d:`/spec-028); the spec-072 WhatsApp transport, webhook verification, and `AssistantResponse → WhatsApp` mapping; the spec-078 `Channel`/`Producer` enums and `content_key`-keyed `Acknowledge`; each transport's existing per-user auth mapping.
- **Independent canaries:** existing `a:c:`/`a:d:`/spec-028 callbacks still decode; the existing WhatsApp transport send/receive stays green; the existing controller suppression window is unchanged; existing Telegram/WhatsApp assistant turns still work.
- **Rollback:** the `a:n:` family and the WhatsApp nudge rendering are additive; disabling them leaves the existing callback families and transport untouched; no enum, budget, or store is mutated (the `whatsapp` enum edit is the spec-078 owner's, not made here).

## Change Boundary

**Allowed during execution:** the additive `a:n:` Telegram encode/decode + inline
rendering, the WhatsApp nudge interactive/list/text rendering over the spec-072
transport, the shared registry resolution for both, and tests/docs named by this
scope.  
**Excluded:** editing `internal/intelligence/surfacing/types.go` (the `whatsapp`
enum is a coordination note), `specs/072-*`, or `specs/078-*`; the confirm/
disambiguation/list callback families; a second budget or suppression path; the
cockpit/rail/palette/feed surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-005-U | Unit | `unit` | SCN-107-005 | `internal/telegram/assistant_adapter/nudge_callbacks_test.go` - `SCN-107-005 a:n: encode/decode within 64 bytes` | `./smackerel.sh test unit` | No |
| T107-005-I | Integration | `integration` | SCN-107-005 | `tests/integration/proactive/telegram_nudge_ack_test.go` - `SCN-107-005 Telegram tap routes to the one ack path` | `./smackerel.sh test integration` | Yes |
| T107-005-A | E2E API regression | `e2e-api` | SCN-107-005 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-005 Telegram inline nudge acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-005-W | E2E UI regression | `e2e-ui` | SCN-107-005 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-005 Telegram act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-006-U | Unit | `unit` | SCN-107-006 | `internal/whatsapp/nudge_reply_test.go` - `SCN-107-006 WhatsApp reply.id carries a:n:<ref>:<a\|s\|d>` | `./smackerel.sh test unit` | No |
| T107-006-I | Integration | `integration` | SCN-107-006 | `tests/integration/proactive/whatsapp_nudge_ack_test.go` - `SCN-107-006 WhatsApp interactive choice routes to the one ack path` | `./smackerel.sh test integration` | Yes |
| T107-006-A | E2E API regression | `e2e-api` | SCN-107-006 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-006 WhatsApp interactive nudge acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-006-W | E2E UI regression | `e2e-ui` | SCN-107-006 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-006 WhatsApp act reflects on the web card state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-007-U | Unit | `unit` | SCN-107-007 | `internal/intelligence/surfacing/cross_channel_ack_test.go` - `SCN-107-007 one Acknowledge(content_key) suppresses every channel` | `./smackerel.sh test unit` | No |
| T107-007-I | Integration | `integration` | SCN-107-007 | `tests/integration/proactive/cross_channel_suppression_test.go` - `SCN-107-007 act on Telegram suppresses web and whatsapp in window` | `./smackerel.sh test integration` | Yes |
| T107-007-A | E2E API regression | `e2e-api` | SCN-107-007 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-007 cross-channel suppression API` | `./smackerel.sh test e2e` | Yes |
| T107-007-W | E2E UI regression | `e2e-ui` | SCN-107-007 | `web/pwa/tests/proactive-cross-channel.spec.ts` - `SCN-107-007 no duplicate prompt on web after acting on Telegram` | `./smackerel.sh test e2e-ui` | Yes |
| T107-03-SUPPRESS | Stress | `stress` | SCN-107-007 | `tests/stress/proactive_suppression_propagation_test.go` - `NFR-107-004 suppression propagates within suppression_window_hours across channels` | `./smackerel.sh test stress` | Yes |
| T107-03-COLLISION | Unit | `unit` | SCN-107-005 | `internal/telegram/assistant_adapter/nudge_callbacks_test.go` - `a:n: never collides with a:c:/a:d:/spec-028 list family` | `./smackerel.sh test unit` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-005 Telegram renders the nudge as inline actions: the nudge renders as a Telegram message with inline act/snooze/dismiss (`a:n:<ref>:<a|s|d>`) and a provenance line, and tapping routes to the same surfacing acknowledgement path.
- [ ] SCN-107-006 WhatsApp renders the nudge as an interactive message: the nudge renders as a spec-072 interactive message (three reply buttons + list/text fallback) with a provenance line, and choosing an action routes to the same surfacing acknowledgement path.
- [ ] SCN-107-007 Acting on one channel suppresses the item on the others: acting from Telegram suppresses the same `content_key` on web and whatsapp within `suppression_window_hours`, with no duplicate action prompt on any channel.
- [ ] Every channel renders one `permit`/`escalated` verdict and routes act/snooze/dismiss to the one `Acknowledge(content_key)`; budget-defer and urgent-escalation are identical across channels; `whatsapp` is reserved as a coordination note and `types.go` is not edited.
- [ ] No `content_key`, node label, or query text reaches any `callback_data`, `reply.id`, or telemetry label; `a:n:` never collides with `a:c:`/`a:d:`/spec-028 and stays inside the 64-byte bound.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-005-U passes with current-session evidence in `report.md#t107-005-u`.
- [ ] T107-005-I passes against the disposable stack with current-session evidence in `report.md#t107-005-i`.
- [ ] T107-005-A passes through production routes with current-session evidence in `report.md#t107-005-a`.
- [ ] T107-005-W passes without interception and proves cross-channel web reflection in `report.md#t107-005-w`.
- [ ] T107-006-U passes with current-session evidence in `report.md#t107-006-u`.
- [ ] T107-006-I passes against the disposable stack with current-session evidence in `report.md#t107-006-i`.
- [ ] T107-006-A passes through production routes with current-session evidence in `report.md#t107-006-a`.
- [ ] T107-006-W passes without interception and proves cross-channel web reflection in `report.md#t107-006-w`.
- [ ] T107-007-U passes with current-session evidence in `report.md#t107-007-u`.
- [ ] T107-007-I passes against the disposable stack with current-session evidence in `report.md#t107-007-i`.
- [ ] T107-007-A passes through production routes with current-session evidence in `report.md#t107-007-a`.
- [ ] T107-007-W passes without interception and proves no duplicate prompt after acting elsewhere in `report.md#t107-007-w`.
- [ ] T107-03-SUPPRESS proves suppression propagation within the window across channels in `report.md#t107-03-suppress`.
- [ ] T107-03-COLLISION proves `a:n:` non-collision with existing callback families in `report.md#t107-03-collision`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, transport documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. `specs/072-*` and `specs/078-*` are
consume-only dependencies and are not modified; the `whatsapp` `Channel` value is
a coordination note to the spec-078 owner.
