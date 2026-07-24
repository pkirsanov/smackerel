# SCOPE-01: Single-Controller Card Projection & Nudge-Ack Foundation

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Tags:** foundation:true  
**Depends On:** External gate - `specs/078-cross-surface-surfacing-prioritizer` + `internal/intelligence/surfacing/` controller usable

## Outcome

Deliver the one composition contract every proactive surface consumes: a
`ProactiveCardModel` that exists only for a `permit`/`escalated` verdict from the
single spec-078 controller, the ephemeral process-local `NudgeRef` registry that
is the sole anti-leak boundary, the single `NudgeAck` path that calls
`Acknowledge(content_key)` for act/snooze/dismiss on every channel, the
`HonestStatePresenter` and `BudgetMeterRead`, and the `a:n:<ref>:<a|s|d>`
encode/decode shared by Telegram callbacks and WhatsApp reply-ids. This
foundation proves the single-controller verdict→card→ack truth — no parallel
surfacing path, no second budget, no `content_key` on any wire — before any
surface renders.

## Requirements And Scenarios

- FR-107-003, FR-107-005, FR-107-006, FR-107-007, FR-107-008, FR-107-010, FR-107-023, FR-107-024, FR-107-028, NFR-107-001, NFR-107-006
- SCN-107-004, SCN-107-008, SCN-107-009

```gherkin
Scenario: SCN-107-004 Acting on a card acknowledges through the controller
  Given a proactive card with content_key "artifact-42"
  When the user taps act
  Then the action acknowledges content_key "artifact-42" through the surfacing controller
  And the visible budget and suppression state update honestly
```

```gherkin
Scenario: SCN-107-008 Budget exhaustion defers identically across channels
  Given the daily_nudge_budget is 5 and 5 non-urgent nudges were already permitted today
  When a sixth non-urgent candidate is proposed
  Then the controller returns deferred-budget-exhausted
  And no sixth nudge appears on web, telegram, or whatsapp
  And the visible budget state shows the day's nudge budget is exhausted
```

```gherkin
Scenario: SCN-107-009 Urgent escalation surfaces identically across channels
  Given the daily budget is exhausted and urgent_escalation_enabled is true
  When a priority-1, time-critical candidate is proposed
  Then the controller returns escalated
  And the urgent nudge surfaces on web, telegram, and whatsapp
  And its provenance line marks it as an urgent escalation
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Acknowledge through controller | Disposable stack; one permitted card with a known content_key; valid scoped session | Act on the card; inspect the controller ack registry and the visible budget/suppression state | One `Acknowledge(content_key)` on the process-wide registry; budget/suppression state updates honestly; no second budget path | integration / e2e-ui |
| Budget exhausted defer | Seeded controller at `daily_nudge_budget=5` with 5 non-urgent nudges already permitted | Propose a sixth non-urgent candidate; render every channel | `deferred-budget-exhausted`; no sixth card on web/telegram/whatsapp; honest budget-exhausted state, never a fabricated card | integration / e2e-ui |
| Urgent escalation | Budget exhausted; `urgent_escalation_enabled=true` | Propose a priority-1 time-critical candidate | `escalated`; the urgent nudge surfaces on every targeted channel with an urgent-escalation provenance line | integration / e2e-ui |
| Anti-leak boundary | A card dispatched to any channel | Inspect the callback/reply-id/web body and telemetry | Only the opaque `NudgeRef` and bounded action reach the wire; no `content_key`, node label, or query text anywhere | unit |

## Implementation Plan

1. Consume `internal/intelligence/surfacing/controller.go` `Propose(ctx, SurfacingCandidate) (SurfacingDecision, error)` and its five terminal verdicts (`types.go`) without re-running or bypassing the dedupe → suppress → budget → escalate pipeline; construct nothing that reaches a channel without a `permit`/`escalated` verdict.
2. Add the immutable `ProactiveCardModel` projection (title, `Producer`-derived provenance line, honest-state token, opaque `NudgeRef`, three-action set) built only for `permit`/`escalated`; `deduped`/`suppressed`/`deferred-budget-exhausted` inform the budget meter / honest state and never produce a card.
3. Add the ephemeral, process-local, expiring `NudgeRef` registry (`ref → {content_key, producer, channel, principal, issued_at}`) mirroring the existing `DedupeIndex`/`InMemoryAck` process-local pattern; it mints opaque ULID-shaped refs and resolves a wire ref back to `(content_key, principal, action)`. It is the sole anti-leak boundary — no `content_key` reaches any `callback_data`, `reply.id`, web body, `data-*` hook, or telemetry. A stale/expired ref resolves to an honest `expired`/`already-handled` render.
4. Add the single `NudgeAck` path: act/snooze/dismiss from any channel resolve their `NudgeRef` and call one `Acknowledge(content_key)` on the process-wide `AckLookup` (`suppression.go`), returning the resolved render (`acted`/`snoozed`/`suppressed`/`already-handled`/`expired`). Snooze and dismiss share the same `Acknowledge`; the difference is intent/label/window, never a second store.
5. Add `encodeNudgeCallback(ref, action) → "a:n:<ref>:<a|s|d>"` and `decodeNudge` producing a new `callbackKindNudge` by extending `decodeCallbackData` in `internal/telegram/assistant_adapter/callbacks.go` additively, staying at ~32 bytes inside Telegram's 64-byte `callback_data` bound and never colliding with `a:c:`/`a:d:`/the spec-028 list family. Reserve the identical logical `a:n:<ref>:<a|s|d>` shape for the WhatsApp `reply.id` (rendered in SCOPE-03).
6. Add `HonestStatePresenter` (map quiet/budget-exhausted/suppressed/deduped/no-correlation/degraded/error/unauthorized onto a spec-106 `data-view-state`/`data-operation-state` token; unknown → fail-closed `error`, never a normal card) and `BudgetMeterRead` (read the existing `smackerel_surfacing_budget_remaining` gauge + `daily_nudge_budget` SST into "N of M used today"; exhaustion is an explicit content state). Add no new metric family.
7. Keep NFR-107-001 intact: rendering a card, its provenance line, and its actions adds no I/O to the controller `Propose` hot path; the `<5ms` p99 budget is preserved and proven by a stress/benchmark check.
8. Update API/architecture/testing/operator documentation through the docs owner during implementation; this planning packet edits no doc surface and edits no owner (spec-078 `types.go`, the controller, or the enum) — the `whatsapp` channel value is a coordination note to the spec-078 owner.

## SST No-Default Decision (Reserved)

- `nudge_ref_ttl_hours = 6` — the `NudgeRef` registry TTL. Fail-loud SST key under `config/smackerel.yaml`; config-compile validates it is an integer ≥ `max(suppression_window_hours=4, dedupe_window_hours=6)` so a late tap resolves to an honest `expired`/`already-handled` render. No `${VAR:-default}`/`os.getenv(k, default)` fallback.
- Snooze window (MVP) reuses `suppression_window_hours`; MVP ships **no** distinct `snooze_window_hours`. A distinct key is a bounded additive future SST value resolved through the same `Acknowledge` mechanism (no snooze store); its addition is deferred, not implemented. (design.md OQ6.)

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the single spec-078 `controller.Propose`/verdict/budget/dedupe/suppression/`Acknowledge`; the Telegram `a:` callback namespace (`a:c:`/`a:d:`/spec-028); `internal/intelligence/surfacing/types.go` (`Channel`/`Producer`/`DecisionKind` enums, owned by spec-078); the existing `smackerel_surfacing_*` metric families.
- **Independent canaries:** existing controller budget/dedupe/suppression behavior stays green; existing `a:c:`/`a:d:`/spec-028 callbacks still decode; the existing `smackerel_surfacing_budget_remaining` gauge is unchanged.
- **Rollback:** the `NudgeRef` registry is in-memory (restart drops it, stale refs resolve `expired`); no owner store, budget, or enum is mutated; rollback is a source/config pointer swap leaving the controller and enums untouched.

## Change Boundary

**Allowed during execution:** the `ProactiveCardModel`/`NudgeRef`/`NudgeAck`/`HonestStatePresenter`/`BudgetMeterRead` composition contracts, the additive `a:n:` Telegram callback encode/decode, the `proactive:` SST block (`nudge_ref_ttl_hours`), foundation-scoped tests/docs named by this scope.  
**Excluded:** editing `internal/intelligence/surfacing/types.go` or the controller/budget/dedupe/suppression internals; the `whatsapp` enum edit (coordination note to spec-078); any surface rendering (web card, cockpit, rail, palette, feed); the WhatsApp/Telegram message send (SCOPE-03); a second budget, a second store, or a client cache.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-004-U | Unit | `unit` | SCN-107-004 | `internal/web/proactive/nudge_ack_test.go` - `SCN-107-004 act acknowledges content_key through the one controller` | `./smackerel.sh test unit` | No |
| T107-004-I | Integration | `integration` | SCN-107-004 | `tests/integration/proactive/nudge_ack_controller_test.go` - `SCN-107-004 ack routes to the process-wide registry` | `./smackerel.sh test integration` | Yes |
| T107-004-A | E2E API regression | `e2e-api` | SCN-107-004 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-004 act acknowledges through controller API` | `./smackerel.sh test e2e` | Yes |
| T107-004-W | E2E UI regression | `e2e-ui` | SCN-107-004 | `web/pwa/tests/proactive-card.spec.ts` - `SCN-107-004 acting updates honest budget and suppression state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-008-U | Unit | `unit` | SCN-107-008 | `internal/intelligence/surfacing/budget_meter_test.go` - `SCN-107-008 deferred-budget-exhausted yields no card` | `./smackerel.sh test unit` | No |
| T107-008-I | Integration | `integration` | SCN-107-008 | `tests/integration/proactive/budget_defer_parity_test.go` - `SCN-107-008 sixth non-urgent deferred on every channel` | `./smackerel.sh test integration` | Yes |
| T107-008-A | E2E API regression | `e2e-api` | SCN-107-008 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-008 budget exhausted defer API` | `./smackerel.sh test e2e` | Yes |
| T107-008-W | E2E UI regression | `e2e-ui` | SCN-107-008 | `web/pwa/tests/proactive-card.spec.ts` - `SCN-107-008 honest budget-exhausted state, no sixth card` | `./smackerel.sh test e2e-ui` | Yes |
| T107-009-U | Unit | `unit` | SCN-107-009 | `internal/intelligence/surfacing/escalation_projection_test.go` - `SCN-107-009 escalated verdict projects an urgent card` | `./smackerel.sh test unit` | No |
| T107-009-I | Integration | `integration` | SCN-107-009 | `tests/integration/proactive/escalation_parity_test.go` - `SCN-107-009 urgent escalation surfaces on every channel` | `./smackerel.sh test integration` | Yes |
| T107-009-A | E2E API regression | `e2e-api` | SCN-107-009 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-009 urgent escalation API` | `./smackerel.sh test e2e` | Yes |
| T107-009-W | E2E UI regression | `e2e-ui` | SCN-107-009 | `web/pwa/tests/proactive-card.spec.ts` - `SCN-107-009 urgent escalation provenance is marked` | `./smackerel.sh test e2e-ui` | Yes |
| T107-01-HOTPATH | Stress | `stress` | SCN-107-004 | `tests/stress/proactive_hotpath_test.go` - `NFR-107-001 card projection adds no controller hot-path I/O (<5ms p99 preserved)` | `./smackerel.sh test stress` | Yes |
| T107-01-LEAK | Unit | `unit` | SCN-107-004 | `internal/web/proactive/nudge_ref_leak_test.go` - `FR-107-028 no content_key/node label/query on any wire or telemetry` | `./smackerel.sh test unit` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-004 Acting on a card acknowledges through the controller: act/snooze/dismiss from any channel resolve their `NudgeRef` and call one `Acknowledge(content_key)` on the process-wide registry, and the visible budget and suppression state update honestly.
- [ ] SCN-107-008 Budget exhaustion defers identically across channels: a sixth non-urgent candidate returns `deferred-budget-exhausted`, produces no card on web/telegram/whatsapp, and renders an honest budget-exhausted state, never a fabricated card.
- [ ] SCN-107-009 Urgent escalation surfaces identically across channels: a priority-1 time-critical candidate returns `escalated` and surfaces on every targeted channel with an urgent-escalation provenance line.
- [ ] A `ProactiveCardModel` exists only for a `permit`/`escalated` verdict; no composition path renders a card that bypassed `controller.Propose`, and no second budget, second store, or client cache is introduced.
- [ ] The `NudgeRef` registry is the single anti-leak boundary: no `content_key`, node label, or query text reaches any `callback_data`, `reply.id`, web body, `data-*` hook, or telemetry; the `a:n:` family never collides with `a:c:`/`a:d:`/spec-028; NFR-107-001 controller hot-path is preserved.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-004-U passes with current-session evidence in `report.md#t107-004-u`.
- [ ] T107-004-I passes against the disposable stack with current-session evidence in `report.md#t107-004-i`.
- [ ] T107-004-A passes through production HTTP routes with current-session evidence in `report.md#t107-004-a`.
- [ ] T107-004-W passes without interception and proves honest budget/suppression update in `report.md#t107-004-w`.
- [ ] T107-008-U passes with current-session evidence in `report.md#t107-008-u`.
- [ ] T107-008-I passes against the disposable stack with current-session evidence in `report.md#t107-008-i`.
- [ ] T107-008-A passes through production HTTP routes with current-session evidence in `report.md#t107-008-a`.
- [ ] T107-008-W passes without interception and proves the honest budget-exhausted state in `report.md#t107-008-w`.
- [ ] T107-009-U passes with current-session evidence in `report.md#t107-009-u`.
- [ ] T107-009-I passes against the disposable stack with current-session evidence in `report.md#t107-009-i`.
- [ ] T107-009-A passes through production HTTP routes with current-session evidence in `report.md#t107-009-a`.
- [ ] T107-009-W passes without interception and proves the urgent-escalation provenance in `report.md#t107-009-w`.
- [ ] T107-01-HOTPATH proves the controller `<5ms` p99 hot path is preserved in `report.md#t107-01-hotpath`.
- [ ] T107-01-LEAK proves no `content_key`/node label/query reaches any wire or telemetry in `report.md#t107-01-leak`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation (including the `nudge_ref_ttl_hours` no-default SST key), API documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. No owner contract (spec-078
controller, `types.go` enum) is edited; the `whatsapp` channel value is a
coordination note only.
