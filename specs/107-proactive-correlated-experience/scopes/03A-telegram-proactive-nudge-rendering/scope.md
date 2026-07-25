# SCOPE-03A: Telegram Proactive Nudge Rendering (Buildable Now)

**Status:** In Progress  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-01  
**External Gate:** `specs/078-cross-surface-surfacing-prioritizer` + `internal/intelligence/surfacing/` controller usable (in-tree). NOT gated on SCOPE-02, spec-106, or spec-072.

## Outcome

Render a `permit`/`escalated` `ProactiveCardModel` as a **Telegram
inline-keyboard message** — a title, a plain-language "why am I seeing this"
provenance line, and one inline row of `Act`/`Snooze`/`Dismiss` buttons encoded
with the committed additive `a:n:<ref>:<a|s|d>` callback family — wire the
producer → single spec-078 `controller.Propose` → Telegram render path, and route
a tap through the shared SCOPE-01 `NudgeRef` registry to the one
`NudgeAck.Handle` → `Acknowledge(content_key)` path (single-channel). Render the
honest Telegram states — budget-exhausted, deduped-not-drawn,
already-acted/suppressed, expired — never a fabricated card.

This scope is **buildable now**: it consumes only the completed SCOPE-01
foundation (the `ProactiveCardModel` projection, the `NudgeRef` registry, the
`NudgeAck` path, and the additive `a:n:` Telegram callback family — all committed
in `d652cc19`) and the in-tree spec-078 controller. It has **no** web (spec-106),
WhatsApp (spec-072), or cross-channel-everywhere dependency. The cross-channel
act-once-suppressed-**everywhere** parity assertion and the WhatsApp interactive
rendering are `SCOPE-03B` and stay gated.

## Requirements And Scenarios

- FR-107-009 (Telegram inline-button rendering portion), FR-107-010 (Telegram act/snooze/dismiss controls extend the adapter callback pattern without colliding with the confirm/disambiguation/spec-028 list families), FR-107-028 (no `content_key`/label/query on any wire)
- Telegram-observed, single-channel portions of FR-107-007 (the controller's unmodified suppression behavior seen on Telegram), FR-107-008 (budget-defer / urgent-escalation seen on Telegram), and NFR-107-004 (a Telegram act suppresses the same `content_key` on a Telegram re-render)
- SCN-107-005 (the Telegram-specific scenario; SCN-107-006 WhatsApp and SCN-107-007 cross-channel-everywhere are `SCOPE-03B`)

```gherkin
Scenario: SCN-107-005 Telegram renders the nudge as inline actions
  Given the same candidate is permitted for the telegram channel
  When the nudge is delivered to the user on Telegram
  Then it renders as a message with inline act, snooze, and dismiss buttons and a provenance line
  And tapping an action routes to the same surfacing acknowledgement path
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Telegram inline actions | Disposable/stores-only stack; a candidate permitted for telegram; bound Telegram identity | Deliver the nudge; tap an inline action | Message body + `Why:` line + inline `Act`/`Snooze`/`Dismiss` encoded `a:n:<ref>:<a\|s\|d>`; tapping routes through the shared registry to the one ack path; the message edits in place and removes the buttons once terminal | integration / e2e-api |
| Adapter-level golden render | A permitted `ProactiveCardModel` for telegram | Project the card; render the inline message | Golden inline message asserts the title, the producer-derived `Why:` provenance line, and the three `a:n:` buttons — no web surface, an adapter-level golden (not Playwright) | unit |
| Act-once-suppresses-on-Telegram | A telegram-permitted `content_key`; `suppression_window_hours` active | Act from Telegram; re-render on Telegram | The same `content_key` renders as `already-acted`/suppressed on the Telegram re-render via one `Acknowledge(content_key)`; no duplicate Telegram prompt (single-channel; web/whatsapp reflection is `SCOPE-03B`) | integration |
| Honest Telegram states | A candidate that is budget-exhausted / deduped / already-acted / expired | Attempt to render each condition on Telegram | Each condition renders as its own distinct honest Telegram state, never a normal or fabricated card | integration |
| Non-collision | Existing Telegram callbacks live | Decode `a:c:`/`a:d:`/spec-028 and `a:n:` payloads | `a:n:` decodes to `callbackKindNudge` and never collides with the confirm/disambiguation/list families; all stay inside 64 bytes | unit |

## Implementation Plan

1. Consume the committed SCOPE-01 foundation (no re-implementation): the
   `ProactiveCardModel` projection of one `permit`/`escalated` verdict, the
   ephemeral process-local `NudgeRef` registry, the single `NudgeAck.Handle`
   → `Acknowledge(content_key)` path, and the additive
   `callbackNudgePrefix = "a:n:"` + `decodeNudge` → `callbackKindNudge` already
   present in `internal/telegram/assistant_adapter/callbacks.go` (`d652cc19`).
2. Render one inline row `Act`/`Snooze`/`Dismiss` (`a:n:<ref>:<a|s|d>`, ~32 bytes,
   inside Telegram's 64-byte `callback_data` bound) plus the `Why:` provenance
   line derived from the card's real `Producer`; tapping edits the message in
   place and removes the buttons once terminal.
3. Wire the producer → single spec-078 `controller.Propose` verdict → Telegram
   render path so a Telegram card exists only for a `permit`/`escalated` verdict;
   route act/snooze/dismiss through the shared registry to the one
   `Acknowledge(content_key)` (no second budget, no second store, no parallel
   surfacing path). The identity join reuses the transport's existing per-user
   Telegram auth mapping — no new identity store.
4. Render the honest Telegram states through the SCOPE-01 `HonestStatePresenter`:
   budget-exhausted (FR-107-008, observed on Telegram), deduped-not-drawn,
   already-acted/suppressed (single-channel suppression, FR-107-007/NFR-107-004
   observed on Telegram), and expired (`nudge_ref_ttl_hours` lapse) — never a
   fabricated card.
5. Keep only the bounded non-sensitive vocabulary on the wire (opaque `NudgeRef`,
   action, closed producer/channel/verdict/timing/count labels); no `content_key`,
   node label, or query text on any `callback_data` or telemetry label
   (FR-107-028).
6. Update Telegram transport/testing documentation through the docs owner during
   implementation; do not modify `specs/072-*` or `specs/078-*` (coordination
   notes only).

## SST No-Default Decision (Reserved)

- Snooze (MVP) reuses `suppression_window_hours` on Telegram: `Snooze` calls the
  same `Acknowledge(content_key)`; MVP ships no distinct `snooze_window_hours`
  (SCOPE-01 decision; design.md OQ6). `expired` is governed by
  `nudge_ref_ttl_hours` (SCOPE-01). No new SST key is introduced by this scope.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the Telegram `a:` callback namespace (`a:c:`/`a:d:`/spec-028); the spec-078 `Channel`/`Producer` enums and `content_key`-keyed `Acknowledge`; the Telegram transport's existing per-user auth mapping; the SCOPE-01 `NudgeRef`/`NudgeAck`/`ProactiveCardModel` contract.
- **Independent canaries:** existing `a:c:`/`a:d:`/spec-028 callbacks still decode; the existing controller suppression window is unchanged; existing Telegram assistant turns still work.
- **Rollback:** the `a:n:` family and the Telegram nudge rendering are additive; disabling them leaves the existing callback families and transport untouched; no enum, budget, or store is mutated; the `NudgeRef` registry is in-memory (a restart drops it, resolving stale refs to `expired`).

## Change Boundary

**Allowed during execution:** the Telegram `a:n:` inline rendering + producer→controller→Telegram wiring, the single-channel `Acknowledge(content_key)` routing, the honest Telegram-state rendering, the shared-registry resolution for Telegram, and tests/docs named by this scope.  
**Excluded:** the WhatsApp interactive rendering and the cross-channel act-once-suppressed-**everywhere** parity assertion (`SCOPE-03B`); editing `internal/intelligence/surfacing/types.go` (the `whatsapp` enum is a `SCOPE-03B` coordination note); `specs/072-*` or `specs/078-*`; the confirm/disambiguation/list callback families; a second budget or suppression path; the web card (`SCOPE-02`); the cockpit/rail/palette/feed surfaces; any web Playwright coverage.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-005-U | Unit | `unit` | SCN-107-005 | `internal/telegram/assistant_adapter/nudge_callbacks_test.go` - `SCN-107-005 a:n: encode/decode within 64 bytes` | `./smackerel.sh test unit` | No |
| T107-005-I | Integration | `integration` | SCN-107-005 | `tests/integration/proactive/telegram_nudge_ack_test.go` - `SCN-107-005 Telegram tap routes to the one ack path` | `./smackerel.sh test integration` | Yes |
| T107-005-A | E2E API | `e2e-api` | SCN-107-005 | `tests/e2e/proactive_channel_parity_e2e_test.go` - `SCN-107-005 Telegram inline nudge acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-03A-GOLDEN | Unit (adapter golden) | `unit` | SCN-107-005 | `internal/telegram/assistant_adapter/nudge_render_golden_test.go` - `SCN-107-005 golden inline message: title + Why line + a:n: Act/Snooze/Dismiss` | `./smackerel.sh test unit` | No |
| T107-03A-TGSUPPRESS | Integration | `integration` | SCN-107-005 | `tests/integration/proactive/telegram_single_channel_suppression_test.go` - `SCN-107-005 act on Telegram suppresses the same content_key on a Telegram re-render` | `./smackerel.sh test integration` | Yes |
| T107-03A-HONEST | Integration | `integration` | SCN-107-005 | `tests/integration/proactive/telegram_honest_states_test.go` - `SCN-107-005 budget-exhausted/deduped/already-acted/expired render on Telegram, never a fabricated card` | `./smackerel.sh test integration` | Yes |
| T107-03-COLLISION | Unit | `unit` | SCN-107-005 | `internal/telegram/assistant_adapter/nudge_callbacks_test.go` - `a:n: never collides with a:c:/a:d:/spec-028 list family` | `./smackerel.sh test unit` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [x] SCN-107-005 Telegram renders the nudge as inline actions: the nudge renders as a Telegram message with inline act/snooze/dismiss (`a:n:<ref>:<a|s|d>`) and a producer-derived provenance line, and tapping routes to the same surfacing acknowledgement path.
- [x] Act-once-on-Telegram suppresses the same `content_key` on a Telegram re-render through the one `Acknowledge(content_key)` path, with no duplicate Telegram prompt (single-channel; cross-channel-everywhere parity is `SCOPE-03B`).
- [x] The honest Telegram states (budget-exhausted, deduped-not-drawn, already-acted/suppressed, expired) each render distinctly on Telegram, never a normal or fabricated card.
- [x] Every Telegram card originates from one `permit`/`escalated` spec-078 controller verdict; act/snooze/dismiss route to the one `Acknowledge(content_key)`; no second budget/store/cache; the additive `a:n:` family never collides with `a:c:`/`a:d:`/spec-028 and stays inside the 64-byte `callback_data` bound.
- [x] No `content_key`, node label, or query text reaches any `callback_data` or telemetry label (FR-107-028).

#### Test Evidence - One Item Per Test Plan Row

- [x] T107-005-U passes with current-session evidence in `report.md#t107-005-u`.
- [x] T107-005-I passes against the disposable/stores-only stack with current-session evidence in `report.md#t107-005-i`.
- [x] T107-005-A passes through production routes with current-session evidence in `report.md#t107-005-a`.
- [x] T107-03A-GOLDEN proves the adapter-level golden inline render (title + `Why:` + `a:n:` buttons) in `report.md#t107-03a-golden`.
- [x] T107-03A-TGSUPPRESS proves single-channel Telegram suppression on re-render in `report.md#t107-03a-tgsuppress`.
- [x] T107-03A-HONEST proves the honest Telegram states render distinctly in `report.md#t107-03a-honest`.
- [x] T107-03-COLLISION proves `a:n:` non-collision with the existing callback families in `report.md#t107-03-collision`.

#### Build Quality Gate

- [x] Scope tests, check, lint, format, source/config validation, Telegram transport documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.
  → Evidence: `report.md#build-quality` — lint 0, config generate 0, all 10 own `.go` files gofmt-clean (repo-wide `format --check` exit 1 is FOREIGN-only: internal/api/graphapi/activation.go + internal/web/handler_test.go, not touched), artifact-lint 0, traceability 0 (0 warnings); check at `report.md#build-check`; consumer/ack at `report.md#t107-005-i`; change-boundary at the report.md Summary Boundary note.

## Uncertainty Declaration

All 13 DoD items are now checked with real current-session execution evidence
(implement phase, bubbles.implement); no remaining uncertainty for SCOPE-03A. This scope was buildable on the
committed SCOPE-01 foundation + the in-tree spec-078 controller; it does not wait
on spec-106, spec-072, or `SCOPE-02`. `specs/072-*` and `specs/078-*` are
consume-only dependencies and are not modified; the `whatsapp` `Channel` value is
a `SCOPE-03B` coordination note to the spec-078 owner.
