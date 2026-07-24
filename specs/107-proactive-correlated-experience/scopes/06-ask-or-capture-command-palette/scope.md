# SCOPE-06: Ask-or-Capture Command Palette

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-05

## Outcome

Provide one global ask-or-capture command surface (Cmd/Ctrl-K) reachable anywhere
in the web product, rendered as a spec-106 global overlay and routed by
`PaletteTurnRouter` through the existing assistant `Facade.Handle` turn to exactly
one of: a grounded answer with sources, a spec-074 capture-as-fallback Idea with
the shared "saved as an idea" acknowledgement, an honest refusal, or an error. The
palette forks nothing (capture is the spec-074 path unchanged) and honors the
`OutcomeOK`-only capture gate — a failed or refused ask is never rendered as
"saved as an idea".

## Requirements And Scenarios

- FR-107-017, FR-107-018, FR-107-019, FR-107-022, NFR-107-006
- SCN-107-012, SCN-107-013

```gherkin
Scenario: SCN-107-012 Command palette answers a grounded turn
  Given the user presses Cmd/Ctrl-K anywhere in the web product
  When the user enters an answerable question
  Then one input returns a grounded answer with sources
```

```gherkin
Scenario: SCN-107-013 Command palette captures as fallback
  Given the user presses Cmd/Ctrl-K and enters a turn that no scenario handles and the agent cannot ground
  When the turn is submitted
  Then it is captured through the spec-074 capture-as-fallback path as exactly one Idea with provenance "capture-as-fallback"
  And the user sees the shared saved-as-idea acknowledgement
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Grounded answer | Disposable stack; a groundable corpus; valid scoped session | Press Cmd/Ctrl-K; enter an answerable question | One input returns a grounded answer with sources (`current` view-state); the `OutcomeOK` capture/provenance gate ran | e2e-ui |
| Capture as fallback | A turn no scenario handles and the agent cannot ground | Submit the turn | Exactly one spec-074 Idea with provenance `capture-as-fallback` and the shared saved-as-idea ack (`captured-as-idea`); a duplicate within the dedup window makes no second Idea | integration / e2e-ui |
| Failed ask is not saved-as-idea | A turn whose ask fails/refuses (`StatusUnavailable`/non-`OutcomeOK`) | Submit the turn | An honest refusal/error (`refused`/`error`), structurally distinct from the capture ack; never "saved as an idea" | unit |
| Capture canary | Existing assistant capture path live | Trigger the existing band-low capture | The existing spec-074 capture and "saved as an idea" band-low path still works; the palette does not fork it | e2e-ui |

## Implementation Plan

1. Add the Cmd/Ctrl-K global overlay above the spec-106 shell (reachable by the accessible shortcut and an equivalent visible control for pointer/touch users); it registers no new turn endpoint and adds no nav destination.
2. Route one input through `PaletteTurnRouter` → the existing assistant `Facade.Handle` (spec 061, transport web per spec 073). Map the `Facade` result onto structurally distinct spec-106 tokens:
   - `agent.OutcomeOK` with grounded Sources → answer body + sources (`current`) — grounded ask (FR-107-019).
   - `CaptureRoute == true` (band-low / unresolvable, capture-eligible) → one spec-074 Idea (`internal/assistant/capturefallback/payload.go`) + shared saved-as-idea ack (`captured-as-idea`); the dedup window collapses a duplicate; "that was just chat" uses the existing correction path (FR-107-018).
   - `OutcomeOK` without grounded sources → `StatusUnavailable` + `ErrNoGroundedAnswer`, or any non-`OutcomeOK` outcome → honest refusal/error (`refused`/`error`).
3. Honor the smackerel assistant-honesty contract: the capture/provenance gate runs only on `OutcomeOK` (`internal/assistant/facade.go`); the refusal token and the capture-ack token are structurally distinct; "saved as an idea" is band-low capture only. A failed ask is never rendered as the capture ack.
4. The palette is a consumer of the `Facade`; it does not re-implement capture, grounding, or refusal, and it forks no capture path (FR-107-018). Preserve the existing HttpOnly same-origin session; no token or personal content enters durable client storage.
5. Expose the stable spec-106 `data-*` contract on the palette (`idle`/`thinking`/`answered`/`captured-as-idea`/`error`) with closed, content-free token values (no query text in a test hook).
6. Update assistant/testing documentation through the docs owner during implementation; do not modify `specs/074-*`, `specs/061-*`, or `specs/073-*` (consume only).

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the assistant `Facade.Handle` turn (spec 061/073), `CaptureRoute`/`StatusSavedAsIdea`/`StatusUnavailable`, the `OutcomeOK`-only capture/provenance gate; the spec-074 capture path + `capturefallback/payload.go`; the spec-106 overlay/shell/session.
- **Independent canaries:** the existing assistant turn and grounding stay green; the existing band-low "saved as an idea" capture still works; the existing "that was just chat" correction path is unchanged; the session/CSP contract is unchanged.
- **Rollback:** the palette is an additive overlay consuming the `Facade`; disabling it leaves the assistant surfaces and capture path intact; no capture path is forked and no turn endpoint is added.

## Change Boundary

**Allowed during execution:** the Cmd/Ctrl-K overlay, `PaletteTurnRouter` (a thin
consumer of `Facade.Handle`), the palette `data-*` hooks, and tests/docs named by
this scope.  
**Excluded:** editing `specs/074-*`/`specs/061-*`/`specs/073-*` or the capture/
turn/grounding/refusal internals; forking a capture path; rendering a failed ask
as "saved as an idea"; the cockpit/rail/feed surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-012-U | Unit | `unit` | SCN-107-012 | `web/pwa/tests/command_palette_router_test.ts` - `SCN-107-012 grounded OutcomeOK renders answer + sources` | `./smackerel.sh test unit` | No |
| T107-012-I | Integration | `integration` | SCN-107-012 | `tests/integration/proactive/palette_turn_router_test.go` - `SCN-107-012 palette routes an answerable turn through the Facade` | `./smackerel.sh test integration` | Yes |
| T107-012-A | E2E API regression | `e2e-api` | SCN-107-012 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-012 grounded answer API` | `./smackerel.sh test e2e` | Yes |
| T107-012-W | E2E UI regression | `e2e-ui` | SCN-107-012 | `web/pwa/tests/command-palette.spec.ts` - `SCN-107-012 Cmd/Ctrl-K returns a grounded answer with sources` | `./smackerel.sh test e2e-ui` | Yes |
| T107-013-U | Unit | `unit` | SCN-107-013 | `web/pwa/tests/command_palette_router_test.ts` - `SCN-107-013 CaptureRoute renders captured-as-idea ack` | `./smackerel.sh test unit` | No |
| T107-013-I | Integration | `integration` | SCN-107-013 | `tests/integration/proactive/palette_turn_router_test.go` - `SCN-107-013 unground-able turn captures exactly one spec-074 Idea` | `./smackerel.sh test integration` | Yes |
| T107-013-A | E2E API regression | `e2e-api` | SCN-107-013 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-013 capture-as-fallback API (one Idea, dedup window)` | `./smackerel.sh test e2e` | Yes |
| T107-013-W | E2E UI regression | `e2e-ui` | SCN-107-013 | `web/pwa/tests/command-palette.spec.ts` - `SCN-107-013 fallback captures one Idea with the shared saved-as-idea ack` | `./smackerel.sh test e2e-ui` | Yes |
| T107-06-HONESTY | Unit | `unit` | SCN-107-013 | `web/pwa/tests/command_palette_router_test.ts` - `failed/refused ask renders refused/error, never saved-as-idea` | `./smackerel.sh test unit` | No |
| T107-06-CANARY | Shared-shell canary | `e2e-ui` | SCN-107-012 | `web/pwa/tests/assistant_accessibility.spec.ts` - `palette preserves the existing assistant capture and correction path` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-012 Command palette answers a grounded turn: pressing Cmd/Ctrl-K opens one input that returns a grounded answer with sources when the turn is answerable.
- [ ] SCN-107-013 Command palette captures as fallback: an unground-able, unhandled turn is captured through the spec-074 path as exactly one Idea with provenance `capture-as-fallback` and the shared saved-as-idea acknowledgement, with a duplicate collapsed by the dedup window.
- [ ] The palette consumes the assistant `Facade` and forks nothing; the refusal token and the capture-ack token are structurally distinct, and a failed/refused ask is never rendered as "saved as an idea" (`OutcomeOK`-only capture gate); `specs/074-*`/`061-*`/`073-*` are not modified.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-012-U passes with current-session evidence in `report.md#t107-012-u`.
- [ ] T107-012-I passes against the disposable stack with current-session evidence in `report.md#t107-012-i`.
- [ ] T107-012-A passes through production HTTP routes with current-session evidence in `report.md#t107-012-a`.
- [ ] T107-012-W passes without interception and proves the grounded answer + sources in `report.md#t107-012-w`.
- [ ] T107-013-U passes with current-session evidence in `report.md#t107-013-u`.
- [ ] T107-013-I passes against the disposable stack with current-session evidence in `report.md#t107-013-i`.
- [ ] T107-013-A passes through production HTTP routes with current-session evidence in `report.md#t107-013-a`.
- [ ] T107-013-W passes without interception and proves one Idea + the shared ack in `report.md#t107-013-w`.
- [ ] T107-06-HONESTY proves a failed ask renders refused/error, never saved-as-idea, in `report.md#t107-06-honesty`.
- [ ] T107-06-CANARY independently proves the existing assistant capture/correction path stays green in `report.md#t107-06-canary`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, assistant documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. `specs/074-*`, `specs/061-*`, and
`specs/073-*` are consume-only dependencies and are not modified.
