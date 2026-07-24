# SCOPE-04: Today Cockpit Composition (spec-106 `Today` body)

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-03

## Outcome

Compose the decision-first Today cockpit as the **body of the spec-106 `Today`
root destination** (106 owns the route; 107 owns the composition inside it),
top-to-bottom in observe-first reading order: the current Daily Smackerel digest
lede, the `FOR YOU NOW` region of `permit`/`escalated` proactive cards, a
what-changed strip (a `WhatChangedRead` summary), and — secondary, not the hero —
the ask-or-capture bar that opens the P4 palette; with the budget meter
(`BudgetMeterRead`) in the header. The cockpit leads with real produced
intelligence, never a blank canvas, and renders honest quiet/partial/degraded
regions through `HonestStatePresenter` rather than a fabricated card.

## Requirements And Scenarios

- FR-107-001, FR-107-002, FR-107-022, FR-107-023, FR-107-024, FR-107-029, NFR-107-002
- SCN-107-001, SCN-107-002, SCN-107-017

```gherkin
Scenario: SCN-107-001 Today cockpit leads with produced intelligence
  Given an authenticated web user with a current digest and controller-permitted cards for today
  When the user opens Smackerel
  Then the default landing fuses the current digest, the day's permitted proactive cards, a what-changed summary, and one ask-or-capture bar
  And the surface leads with real produced intelligence rather than a blank canvas
```

```gherkin
Scenario: SCN-107-002 Quiet day is honest, not fabricated
  Given the surfacing controller permitted nothing notable today
  When the user opens the Today cockpit
  Then the cockpit shows an honest quiet state
  And it does not fabricate a proactive card to fill the surface
```

```gherkin
Scenario: SCN-107-017 Producer failure is not a normal card
  Given a producer degrades or fails while composing the cockpit
  When the surface renders
  Then the affected region shows a distinct error or degraded state
  And it is not rendered as a normal proactive card and is not fabricated
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Lead with produced intelligence | Disposable stack; a current digest + controller-permitted cards; valid scoped session | Open Smackerel on the `Today` body | Digest lede + `FOR YOU NOW` permitted cards + what-changed strip + secondary ask-or-capture bar + budget meter; observe-first ordering; not a blank canvas | e2e-ui |
| Quiet day honesty | Controller permitted nothing notable today | Open the cockpit | Honest quiet state via `HonestStatePresenter`; no fabricated card; the ask-or-capture bar stays available | integration / e2e-ui |
| Producer failure region | A producer degrades/fails while composing | Render the cockpit | The affected region shows a distinct `Degraded`/error state; the rest stays usable; no normal or fabricated card | integration / e2e-ui |
| Landing budget | Representative populated cockpit | Measure time-to-interactive | Cockpit interactive at P95 within the spec-106 shared-shell landing budget (NFR-107-002) | stress |

## Implementation Plan

1. Compose the cockpit inside the spec-106 `Today` destination body (coordination note 3; 106 owns the route registration). Reuse the spec-106 workspace header (breadcrumb, one h1, page commands); add no hero copy and no new nav destination.
2. Order the body observe-first: (a) the current digest summary as a read-only lede (`Open digest` navigates to the 106 Digest surface; never re-generate digest content), (b) the `FOR YOU NOW` region of `ProactiveCardModel` cards (permit/escalated only, from SCOPE-02), (c) the what-changed strip (a bounded `WhatChangedRead` summary; the full feed is SCOPE-07), (d) the secondary ask-or-capture bar (opens the SCOPE-06 palette). Place the `BudgetMeterRead` "N of M used today" in the header.
3. Render honest states through `HonestStatePresenter`: `quiet-day` (controller permitted nothing notable — consistent with "skip the digest if nothing notable happened"), `partial` (one region failed; the rest stays usable), `degraded` (a producer degraded), `unauthorized`. None renders as a normal or fabricated card (FR-107-002, FR-107-022).
4. Honor invisible-by-default: the cockpit surfaces exactly what the controller already permitted; it never inflates nudge volume to fill the surface and honors the daily budget and the ≤3 system-initiated prompts/week contract (FR-107-024).
5. Expose the stable spec-106 `data-*` contract on every cockpit region with closed, content-free token values; meet the spec-106 landing budget at P95 (NFR-107-002).
6. Update architecture/testing documentation through the docs owner during implementation; do not modify `specs/106-*` (the landing-route confirmation is a coordination note).

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-106 `Today` route, shell, workspace header, tokens, `data-*` DOM contract, and Empty/degraded state primitive; the spec-078 verdict/budget the cockpit reads; the SCOPE-01/02 card + `WhatChangedRead` summary.
- **Independent canaries:** the existing spec-106 Digest surface and Today context stay green; existing shell navigation is unchanged; the existing `smackerel_surfacing_budget_remaining` gauge is unchanged.
- **Rollback:** the cockpit body is additive inside an owner-registered route; disabling it leaves the spec-106 `Today` destination with its own default; no route, budget, or store is mutated.

## Change Boundary

**Allowed during execution:** the cockpit body composition, its region `data-*`
hooks, the budget-meter header render, the what-changed strip summary call, and
tests/docs named by this scope.  
**Excluded:** editing `specs/106-*` or registering a nav/landing route;
re-generating digest content or building a new producer; the full what-changed
feed (SCOPE-07); the palette internals (SCOPE-06); the rail (SCOPE-05).

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-001-U | Unit | `unit` | SCN-107-001 | `web/pwa/tests/today_cockpit_compose_test.ts` - `SCN-107-001 cockpit composes digest+cards+what-changed+ask-or-capture` | `./smackerel.sh test unit` | No |
| T107-001-I | Integration | `integration` | SCN-107-001 | `tests/integration/proactive/today_cockpit_test.go` - `SCN-107-001 cockpit reads real digest and permitted cards` | `./smackerel.sh test integration` | Yes |
| T107-001-A | E2E API regression | `e2e-api` | SCN-107-001 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-001 cockpit composition API` | `./smackerel.sh test e2e` | Yes |
| T107-001-W | E2E UI regression | `e2e-ui` | SCN-107-001 | `web/pwa/tests/today-cockpit.spec.ts` - `SCN-107-001 landing leads with produced intelligence, not a blank canvas` | `./smackerel.sh test e2e-ui` | Yes |
| T107-002-U | Unit | `unit` | SCN-107-002 | `web/pwa/tests/today_cockpit_compose_test.ts` - `SCN-107-002 quiet day maps to an honest quiet state` | `./smackerel.sh test unit` | No |
| T107-002-I | Integration | `integration` | SCN-107-002 | `tests/integration/proactive/today_cockpit_test.go` - `SCN-107-002 no permitted cards yields honest quiet state` | `./smackerel.sh test integration` | Yes |
| T107-002-A | E2E API regression | `e2e-api` | SCN-107-002 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-002 quiet-day cockpit API` | `./smackerel.sh test e2e` | Yes |
| T107-002-W | E2E UI regression | `e2e-ui` | SCN-107-002 | `web/pwa/tests/today-cockpit.spec.ts` - `SCN-107-002 quiet day shows honest state and fabricates no card` | `./smackerel.sh test e2e-ui` | Yes |
| T107-017-U | Unit | `unit` | SCN-107-017 | `web/pwa/tests/today_cockpit_compose_test.ts` - `SCN-107-017 degraded producer maps to a distinct region error` | `./smackerel.sh test unit` | No |
| T107-017-I | Integration | `integration` | SCN-107-017 | `tests/integration/proactive/today_cockpit_test.go` - `SCN-107-017 producer failure is a distinct region state` | `./smackerel.sh test integration` | Yes |
| T107-017-A | E2E API regression | `e2e-api` | SCN-107-017 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-017 producer failure API` | `./smackerel.sh test e2e` | Yes |
| T107-017-W | E2E UI regression | `e2e-ui` | SCN-107-017 | `web/pwa/tests/today-cockpit.spec.ts` - `SCN-107-017 producer failure is not a normal or fabricated card` | `./smackerel.sh test e2e-ui` | Yes |
| T107-04-LANDING | Stress | `stress` | SCN-107-001 | `tests/stress/proactive_cockpit_landing_test.go` - `NFR-107-002 cockpit interactive at P95 within the shared-shell landing budget` | `./smackerel.sh test stress` | Yes |
| T107-04-CANARY | Shared-shell canary | `e2e-ui` | SCN-107-001 | `web/pwa/tests/unified_journey.spec.ts` - `cockpit body preserves the existing Today/Digest surface and shell navigation` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-001 Today cockpit leads with produced intelligence: the default landing fuses the current digest, the day's permitted cards, a what-changed summary, and one ask-or-capture bar, leading with real produced intelligence rather than a blank canvas.
- [ ] SCN-107-002 Quiet day is honest, not fabricated: when the controller permitted nothing notable, the cockpit shows an honest quiet state and fabricates no proactive card.
- [ ] SCN-107-017 Producer failure is not a normal card: a degraded/failed producer renders a distinct region error/degraded state, not a normal or fabricated card, with the rest of the surface usable.
- [ ] The cockpit is the body of the spec-106 `Today` destination (no new nav/landing route, `specs/106-*` unmodified), honors invisible-by-default (daily budget + ≤3 prompts/week, never inflated), and meets the spec-106 landing budget at P95 (NFR-107-002).

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-001-U passes with current-session evidence in `report.md#t107-001-u`.
- [ ] T107-001-I passes against the disposable stack with current-session evidence in `report.md#t107-001-i`.
- [ ] T107-001-A passes through production HTTP routes with current-session evidence in `report.md#t107-001-a`.
- [ ] T107-001-W passes without interception and proves observe-first composition in `report.md#t107-001-w`.
- [ ] T107-002-U passes with current-session evidence in `report.md#t107-002-u`.
- [ ] T107-002-I passes against the disposable stack with current-session evidence in `report.md#t107-002-i`.
- [ ] T107-002-A passes through production HTTP routes with current-session evidence in `report.md#t107-002-a`.
- [ ] T107-002-W passes without interception and proves the honest quiet state (no fabricated card) in `report.md#t107-002-w`.
- [ ] T107-017-U passes with current-session evidence in `report.md#t107-017-u`.
- [ ] T107-017-I passes against the disposable stack with current-session evidence in `report.md#t107-017-i`.
- [ ] T107-017-A passes through production HTTP routes with current-session evidence in `report.md#t107-017-a`.
- [ ] T107-017-W passes without interception and proves the distinct producer-failure region in `report.md#t107-017-w`.
- [ ] T107-04-LANDING proves the P95 landing budget in `report.md#t107-04-landing`.
- [ ] T107-04-CANARY independently proves the existing Today/Digest surface and shell navigation stay green in `report.md#t107-04-canary`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, architecture documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. `specs/106-*` is a compose-over
dependency; the landing-route confirmation is a coordination note, not an edit.
