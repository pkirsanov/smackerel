# SCOPE-03B1: WhatsApp Proactive Nudge Rendering (Buildable Now)

**Status:** Done  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-03A  
**External Gate:** `specs/072-whatsapp-business-transport` interactive transport usable (spec-072 is `done`/certified and `internal/whatsapp/assistant_adapter/` is in-tree) + the committed SCOPE-01 foundation + the in-tree spec-078 controller. NOT gated on SCOPE-02, spec-106, or spec-105.

## Outcome

Render a `permit`/`escalated` `ProactiveCardModel` as a **WhatsApp interactive
message** over the spec-072 transport — a body, a plain-language "why am I seeing
this" provenance line, and three reply buttons `Act`/`Snooze`/`Dismiss` whose
`reply.id` carries the same logical `a:n:<ref>:<a|s|d>` shape as the Telegram
callback (plus the list-message and numbered plain-text `1|2|3 → a|s|d`
fallbacks) — resolve the chosen `reply.id` through the shared SCOPE-01 `NudgeRef`
registry to the one `NudgeAck.Handle` → `Acknowledge(content_key)` path, and prove
**WhatsApp↔Telegram single-pair** suppression: acting once on either channel
suppresses the same `content_key` on the other's re-render within
`suppression_window_hours`. Render the honest WhatsApp states — budget-exhausted,
deduped-not-drawn, already-acted/suppressed, expired — never a fabricated card.

This scope is **buildable now**: it consumes only the completed SCOPE-01
foundation (the `ProactiveCardModel` projection, the `NudgeRef` registry, the
`NudgeAck` path, and the shared `a:n:` logical shape), the SCOPE-03A Telegram
render (as the Telegram side of the single-pair suppression), the in-tree spec-078
controller, and the shipped spec-072 WhatsApp interactive transport
(`internal/whatsapp/assistant_adapter/`). It has **no** web (spec-106) dependency.
The cross-channel act-once-suppressed-**everywhere** parity assertion (web +
Telegram + WhatsApp) and the Telegram/WhatsApp→web reflection are `SCOPE-03B2` and
stay gated.

## Requirements And Scenarios

- FR-107-009 (WhatsApp interactive rendering portion), FR-107-011 (WhatsApp uses the spec-072 interactive transport with the list/text fallback), FR-107-012 (`whatsapp` `Channel` enum extension decision, routed as a coordination note), FR-107-028 (no `content_key`/label/query on any `reply.id` or telemetry)
- WhatsApp-observed, single-pair portions of FR-107-007 (the controller's unmodified suppression seen across the WhatsApp↔Telegram pair) and FR-107-008 (budget-defer / urgent-escalation seen on WhatsApp)
- SCN-107-006 (the WhatsApp-render scenario; SCN-107-007 cross-channel-everywhere is `SCOPE-03B2`)

```gherkin
Scenario: SCN-107-006 WhatsApp renders the nudge as an interactive message
  Given the same candidate is permitted for the whatsapp channel
  When the nudge is delivered to the user on WhatsApp
  Then it renders as an interactive message offering act, snooze, and dismiss with a provenance line
  And choosing an action routes to the same surfacing acknowledgement path
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| WhatsApp interactive | Disposable/stores-only stack; a candidate permitted for whatsapp; the in-tree spec-072 transport with an injected send/receive seam (no real WhatsApp network) | Deliver the interactive message; choose an action | Body + `Why:` line + three reply buttons (list/text fallback preserved); `reply.id` carries the same `a:n:<ref>:<a\|s\|d>`; choosing routes through the shared registry to the same ack path | integration / e2e-api |
| Adapter-level golden render | A permitted `ProactiveCardModel` for whatsapp | Project the card; render the interactive message | Golden interactive message asserts the body, the producer-derived `Why:` provenance line, the three `a:n:` reply buttons, and the list/numbered-text fallback — an adapter-level golden (not Playwright), no web surface | unit |
| Act-once-suppresses-across-the-pair | A whatsapp-permitted `content_key` also permitted on telegram; `suppression_window_hours` active | Act from WhatsApp; re-render on Telegram (and vice versa) | The same `content_key` renders as `already-acted`/suppressed on the other channel's re-render via one `Acknowledge(content_key)`; no duplicate prompt on either channel (WhatsApp↔Telegram pair; web reflection is `SCOPE-03B2`) | integration |
| Honest WhatsApp states | A candidate that is budget-exhausted / deduped / already-acted / expired | Attempt to render each condition on WhatsApp | Each condition renders as its own distinct honest WhatsApp state, never a normal or fabricated card | integration |

## Implementation Plan

1. Consume the committed SCOPE-01 foundation (no re-implementation): the
   `ProactiveCardModel` projection of one `permit`/`escalated` verdict, the
   ephemeral process-local `NudgeRef` registry, the single `NudgeAck.Handle`
   → `Acknowledge(content_key)` path, and the shared `a:n:<ref>:<a|s|d>` logical
   shape (the same shape SCOPE-03A encodes into the Telegram `callback_data`).
2. WhatsApp: use the shipped spec-072 interactive transport
   (`internal/whatsapp/assistant_adapter/`) — consume it, never re-own its webhook
   verification or mapping table. Send three reply buttons `Act`/`Snooze`/`Dismiss`
   with `reply.id = "a:n:<ref>:<a|s|d>"`; provide the list-message and numbered
   plain-text fallbacks (`1|2|3` → `a|s|d`) that preserve the same three actions and
   the provenance line. Resolve `interactive.button_reply.id` / `list_reply.id`
   through the same SCOPE-01 `NudgeRef` registry to the same `NudgeAck` path
   (FR-107-009, FR-107-011).
3. Reserve `whatsapp` as a new bounded `Channel` enum value via the documented
   `ProducerNotification` extension precedent, routed as a **coordination note to
   the spec-078 enum owner**; `internal/intelligence/surfacing/types.go` is NOT
   edited here (FR-107-012). Its Prometheus-cardinality justification (finite
   bounded enum) is recorded for the owner.
4. Single-pair suppression: WhatsApp `Act`/`Snooze`/`Dismiss` route to the one
   `Acknowledge(content_key)`; suppression is already channel-agnostic
   (`content_key`-keyed on one process-wide registry), so acting once on WhatsApp
   suppresses the same `content_key` on a Telegram re-render (and vice versa)
   within `suppression_window_hours` (WhatsApp-observed portion of FR-107-007 /
   NFR-107-004 across the WhatsApp↔Telegram pair). Budget exhaustion defers and
   urgent escalation surfaces on WhatsApp identically to the other channels
   (WhatsApp-observed portion of FR-107-008). The identity join reuses the
   transport's existing per-user WhatsApp verified-phone mapping — no new identity
   store. The web side of "suppressed everywhere" is `SCOPE-03B2`.
5. Consume the `SCOPE-03A` Telegram rendering as the Telegram side of the
   single-pair suppression assertion; do not re-implement the Telegram `a:n:`
   render here.
6. Render the honest WhatsApp states through the SCOPE-01 `HonestStatePresenter`:
   budget-exhausted (FR-107-008, observed on WhatsApp), deduped-not-drawn,
   already-acted/suppressed, and expired (`nudge_ref_ttl_hours` lapse) — never a
   fabricated card.
7. Keep only the bounded non-sensitive vocabulary on every wire (opaque
   `NudgeRef`, action, closed producer/channel/verdict/timing/count labels); no
   `content_key`, node label, or query text on any `reply.id` or telemetry label
   (FR-107-028).
8. Update WhatsApp transport/testing documentation through the docs owner during
   implementation; do not modify `specs/072-*` or `specs/078-*` (coordination
   notes only).

## SST No-Default Decision (Reserved)

- Snooze (MVP) reuses `suppression_window_hours` on WhatsApp: `Snooze` calls the
  same `Acknowledge(content_key)`; MVP ships no distinct `snooze_window_hours`
  (SCOPE-01 decision; design.md OQ6). `expired` is governed by
  `nudge_ref_ttl_hours` (SCOPE-01). No new SST key is introduced by this scope.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-072 WhatsApp transport, webhook verification, and `AssistantResponse → WhatsApp` mapping (`internal/whatsapp/assistant_adapter/`); the spec-078 `Channel`/`Producer` enums and `content_key`-keyed `Acknowledge`; the `SCOPE-03A` Telegram rendering + the `a:n:` logical shape; the WhatsApp transport's existing per-user verified-phone auth mapping; the SCOPE-01 `NudgeRef`/`NudgeAck`/`ProactiveCardModel` contract.
- **Independent canaries:** the existing WhatsApp transport send/receive stays green; the existing controller suppression window is unchanged; the `SCOPE-03A` Telegram callbacks still decode; existing WhatsApp assistant turns still work.
- **Rollback:** the WhatsApp nudge rendering and the single-pair suppression are additive; disabling them leaves the existing transport, the Telegram rendering, and the controller untouched; no enum, budget, or store is mutated (the `whatsapp` enum edit is the spec-078 owner's, not made here); the `NudgeRef` registry is in-memory (a restart drops it, resolving stale refs to `expired`).

## Change Boundary

**Allowed during execution:** the WhatsApp nudge interactive/list/text rendering
over the spec-072 transport, the shared-registry resolution for WhatsApp, the
WhatsApp↔Telegram single-pair act-once-suppression assertion, the honest
WhatsApp-state rendering, and tests/docs named by this scope.  
**Excluded:** re-implementing the `SCOPE-03A` Telegram `a:n:` inline render; the
cross-channel act-once-suppressed-**everywhere** parity assertion and the
Telegram/WhatsApp→web reflection (`SCOPE-03B2`); editing
`internal/intelligence/surfacing/types.go` (the `whatsapp` enum is a coordination
note); `specs/072-*` or `specs/078-*`; the confirm/disambiguation/list callback
families; a second budget or suppression path; the web card (`SCOPE-02`); the
cockpit/rail/palette/feed surfaces; any web Playwright coverage.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-006-U | Unit | `unit` | SCN-107-006 | `internal/whatsapp/assistant_adapter/nudge_reply_test.go` - `SCN-107-006 WhatsApp reply.id carries a:n:<ref>:<a\|s\|d>` (opaque; no content_key/label)` | `./smackerel.sh test unit` | No |
| T107-006-I | Integration | `integration` | SCN-107-006 | `tests/integration/proactive/whatsapp_nudge_ack_test.go` - `SCN-107-006 WhatsApp interactive choice routes to the one ack path (injected transport seam)` | `./smackerel.sh test integration` | Yes |
| T107-006-A | E2E API | `e2e-api` | SCN-107-006 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-006 WhatsApp interactive nudge acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-03B1-GOLDEN | Unit (adapter golden) | `unit` | SCN-107-006 | `internal/whatsapp/assistant_adapter/nudge_render_golden_test.go` - `SCN-107-006 golden interactive message: body + Why line + a:n: Act/Snooze/Dismiss + list/text fallback (adapter-level, no web)` | `./smackerel.sh test unit` | No |
| T107-03B1-FALLBACK | Unit | `unit` | SCN-107-006 | `internal/whatsapp/assistant_adapter/nudge_reply_test.go` - `SCN-107-006 list-message + numbered text fallback (1\|2\|3 → a\|s\|d) preserves the three actions + provenance line` | `./smackerel.sh test unit` | No |
| T107-03B1-WATGSUPPRESS | Integration | `integration` | SCN-107-006 | `tests/integration/proactive/whatsapp_telegram_pair_suppression_test.go` - `SCN-107-006 act on WhatsApp suppresses the same content_key on a Telegram re-render (WhatsApp↔Telegram pair; no web)` | `./smackerel.sh test integration` | Yes |
| T107-03B1-HONEST | Integration | `integration` | SCN-107-006 | `tests/integration/proactive/whatsapp_honest_states_test.go` - `SCN-107-006 budget-exhausted/deduped/already-acted/expired render on WhatsApp, never a fabricated card` | `./smackerel.sh test integration` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [x] SCN-107-006 WhatsApp renders the nudge as an interactive message: the nudge renders as a spec-072 interactive message (three reply buttons + list/text fallback) with a producer-derived provenance line, and choosing an action routes through the shared registry to the same surfacing acknowledgement path.
- [x] Act-once-across-the-pair: acting from WhatsApp suppresses the same `content_key` on a Telegram re-render (and vice versa) within `suppression_window_hours` through the one `Acknowledge(content_key)` path, with no duplicate prompt on either channel (WhatsApp↔Telegram pair; cross-channel-everywhere + web reflection is `SCOPE-03B2`). → Evidence: [report.md#t107-03b1-watgsuppress](../report.md) both-direction PASS.
- [x] The honest WhatsApp states (budget-exhausted, deduped-not-drawn, already-acted/suppressed, expired) each render distinctly on WhatsApp, never a normal or fabricated card; budget-defer and urgent-escalation surface on WhatsApp identically to the other channels. → Evidence: [report.md#t107-03b1-honest](../report.md) 4 distinct honest states PASS.
- [x] Every WhatsApp card originates from one `permit`/`escalated` spec-078 controller verdict; act/snooze/dismiss route to the one `Acknowledge(content_key)`; no second budget/store/cache; `whatsapp` is reserved as a coordination note and `types.go` is not edited. → Evidence: [report.md#t107-006-i](../report.md) every test drives `controller.Propose`; `types.go` unedited (call-site `surfacing.Channel("whatsapp")` reservation only).
- [x] No `content_key`, node label, or query text reaches any `reply.id` or telemetry label; the WhatsApp `reply.id` carries only the opaque `a:n:<ref>:<a|s|d>` shape.

#### Test Evidence - One Item Per Test Plan Row

- [x] T107-006-U passes with current-session evidence in `report.md#t107-006-u`.
- [x] T107-006-I passes against the disposable stack (injected transport seam, no real WhatsApp network) with current-session evidence in `report.md#t107-006-i`.
- [x] T107-006-A passes through production routes with current-session evidence in `report.md#t107-006-a`.
- [x] T107-03B1-GOLDEN proves the adapter-level golden interactive render (body + `Why:` + `a:n:` buttons + fallback) in `report.md#t107-03b1-golden`.
- [x] T107-03B1-FALLBACK proves the list/numbered-text fallback preserves the three actions + provenance in `report.md#t107-03b1-fallback`.
- [x] T107-03B1-WATGSUPPRESS proves WhatsApp↔Telegram single-pair suppression on re-render in `report.md#t107-03b1-watgsuppress`.
- [x] T107-03B1-HONEST proves the honest WhatsApp states render distinctly in `report.md#t107-03b1-honest`.

#### Build Quality Gate

- [x] Scope tests, check, lint, format, source/config validation, WhatsApp transport documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence. → Evidence: [report.md#build-quality](../report.md) — lint 0, all 7 own `.go` files gofmt-clean, artifact-lint 0, traceability 0 (0 warnings, 20/20), spec-072 own tests green; repo-wide `format --check` exit 1 is FOREIGN-only (`internal/api/graphapi/activation.go` + `internal/web/handler_test.go`, not SCOPE-03B1 files, not touched, not bypassed).

## Uncertainty Declaration

All items remain unchecked because implementation, authored tests, and runtime
validation have not been executed by the planning owner. This scope is
**buildable now**: it depends only on the committed SCOPE-01 foundation, the
delivered SCOPE-03A Telegram render, the in-tree spec-078 controller, and the
shipped spec-072 WhatsApp interactive transport — it does NOT wait on spec-106,
spec-105, or `SCOPE-02`. `specs/072-*` and `specs/078-*` are consume-only
dependencies and are not modified; the `whatsapp` `Channel` value is a
coordination note to the spec-078 owner.
