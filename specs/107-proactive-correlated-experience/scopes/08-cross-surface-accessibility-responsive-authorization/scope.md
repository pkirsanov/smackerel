# SCOPE-08: Cross-Surface Accessibility, Responsive & Authorization Hardening

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-07

## Outcome

Deliver equivalent keyboard, screen-reader, and mobile journeys, and fail-closed
per-surface authorization + content-free telemetry, across the entire proactive
surface (cockpit, proactive card, correlation rail, command palette, what-changed
feed). Every action, provenance line, correlation, and state change is reachable in
logical semantic order with predictable focus; every surface is usable at 320px and
200% zoom with 44×44 CSS px touch targets and no overlap; and every card,
provenance line, correlation, and activity row is re-authorized against the
identity's grants with telemetry carrying only the bounded non-sensitive
vocabulary.

## Requirements And Scenarios

- FR-107-025, FR-107-026, FR-107-027, FR-107-028, NFR-107-005
- SCN-107-018, SCN-107-019, SCN-107-020

```gherkin
Scenario: SCN-107-018 Keyboard and screen-reader parity for the proactive surface
  Given the user operates without a pointer
  When the user reaches the cockpit, a card, the correlation rail, the palette, and the activity feed
  Then every action, provenance line, correlation, and state change is available in logical semantic order
  And focus moves predictably after acting, snoozing, dismissing, asking, or capturing
```

```gherkin
Scenario: SCN-107-019 Mobile proactive surface does not overlap
  Given the product is opened on a narrow touch viewport
  When the user acts on cards, opens the correlation rail, uses the palette, and reads the feed
  Then controls do not overlap or require precision tapping
  And primary touch targets are at least 44 by 44 CSS pixels
```

```gherkin
Scenario: SCN-107-020 Provenance and correlations respect authorization
  Given an identity may read only a granted projection of the global corpus
  When cards, provenance lines, correlations, and activity events render
  Then only authorized content is shown
  And nudge and correlation telemetry contains no secret values, node labels, or personal content
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Keyboard + screen-reader parity | Disposable stack; a populated proactive surface; valid scoped session | Reach the cockpit, a card, the rail, the palette, and the feed without a pointer | Every action/provenance/correlation/state change is available in logical heading/landmark/label/focus order; focus moves predictably after act/snooze/dismiss/ask/capture; state changes use concise live announcements without stealing focus | e2e-ui |
| Mobile no-overlap | 320px / 390px touch viewport; 200% zoom | Act on cards, open the rail sheet, use the palette, read the feed | No overlap with fixed nav/rail/palette; primary targets ≥44×44 CSS px; browser safe areas respected; reduced-motion removes nonessential motion while preserving every state change | e2e-ui |
| Authorization + telemetry | An identity with a granted-only projection | Render cards, provenance, correlations, activity | Only authorized content is shown; unauthorized content and existence metadata are never revealed; nudge/correlation/activity telemetry carries only producer/channel/verdict/timing/count and no secret value, node label, query text, or personal content | integration / unit |
| Contrast | All shared themes | Render card text, provenance, badges, focus, state indicators | WCAG 2.2 AA contrast for all applicable controls/content/focus/state; meaning never carried by color/icon/motion alone | e2e-ui |

## Implementation Plan

1. Keyboard/screen-reader: ensure the cockpit, card, rail, palette, and feed are each completable by keyboard alone with logical heading, landmark, label, and focus order; act/snooze/dismiss/ask/capture/budget/suppression/correlation state changes use concise live announcements without stealing focus unexpectedly; focus moves predictably after each action (FR-107-025, NFR-107-005).
2. Provenance and honest-state meaning are never carried by color, icon, or motion alone; reduced-motion preference removes nonessential motion while preserving every state change (spec-106 motion tokens).
3. Responsive/touch: every surface is usable at 320px and 200% zoom without overlap, with primary touch targets ≥44×44 CSS px, browser safe areas respected, and no overlap with fixed navigation, the rail, or the palette (FR-107-026); the command palette is reachable by the accessible shortcut and an equivalent visible control.
4. Contrast: all themes meet WCAG 2.2 AA for card text, provenance, badges, focus, and state indicators, consistent with the spec-106 shared theme.
5. Authorization: re-authorize every card, provenance line, correlation, and activity row against the identity's explicit grants over the operator-owned global corpus before render; unauthorized content and existence metadata are never revealed; the corpus model claims no tenant/user row isolation (FR-107-027).
6. Telemetry: nudge, correlation, and activity telemetry carry only the bounded non-sensitive vocabulary (producer, channel, verdict, timing, counts); no secret value, node label, query text, or personal content is a label value or a wire payload (FR-107-028); the `NudgeRef` opaque-ref boundary holds across every surface.
7. Update accessibility/testing documentation through the docs owner during implementation; add no new nav destination and modify no owner spec.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-106 accessibility/responsive/motion/theme primitives, the four availability labels, and the stable `data-*` DOM contract; the per-surface authorizers (card, rail via graph-reader allowlist, feed via store gating); the `NudgeRef` anti-leak boundary and the `smackerel_surfacing_*` telemetry vocabulary.
- **Independent canaries:** existing shell accessibility and responsive behavior stay green; existing authorization gates on non-proactive surfaces are unchanged; existing telemetry stays content-free.
- **Rollback:** hardening is additive over the shared primitives; disabling a proactive surface leaves the shell's own accessibility/responsive behavior intact; no authorizer or telemetry contract is mutated.

## Change Boundary

**Allowed during execution:** accessibility/responsive/contrast/motion refinements
on the 107 surfaces, per-surface re-authorization wiring over existing authorizers,
telemetry content-free enforcement, and tests/docs named by this scope.  
**Excluded:** editing `specs/106-*` primitives or any owner authorizer/telemetry
contract; introducing a nav destination; adding a client cache; the earlier
surface scopes' composition internals.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-018-U | Unit | `unit` | SCN-107-018 | `web/pwa/tests/proactive_a11y_semantics_test.ts` - `SCN-107-018 semantic order and focus model across surfaces` | `./smackerel.sh test unit` | No |
| T107-018-I | Integration | `integration` | SCN-107-018 | `tests/integration/proactive/a11y_projection_test.go` - `SCN-107-018 nonvisual projection parity for each surface` | `./smackerel.sh test integration` | Yes |
| T107-018-A | E2E API regression | `e2e-api` | SCN-107-018 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-018 semantic projection authorization API` | `./smackerel.sh test e2e` | Yes |
| T107-018-W | E2E UI regression | `e2e-ui` | SCN-107-018 | `web/pwa/tests/proactive-accessibility.spec.ts` - `SCN-107-018 keyboard and screen-reader parity with predictable focus` | `./smackerel.sh test e2e-ui` | Yes |
| T107-019-U | Unit | `unit` | SCN-107-019 | `web/pwa/tests/proactive_responsive_contract_test.ts` - `SCN-107-019 44x44 targets and no-overlap contract` | `./smackerel.sh test unit` | No |
| T107-019-I | Integration | `integration` | SCN-107-019 | `tests/integration/proactive/responsive_state_test.go` - `SCN-107-019 responsive state across viewports` | `./smackerel.sh test integration` | Yes |
| T107-019-A | E2E API regression | `e2e-api` | SCN-107-019 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-019 responsive read API` | `./smackerel.sh test e2e` | Yes |
| T107-019-W | E2E UI regression | `e2e-ui` | SCN-107-019 | `web/pwa/tests/proactive-responsive.spec.ts` - `SCN-107-019 320px/200% no overlap, 44x44 touch-safe` | `./smackerel.sh test e2e-ui` | Yes |
| T107-020-U | Unit | `unit` | SCN-107-020 | `internal/web/proactive/authorization_telemetry_test.go` - `SCN-107-020 telemetry carries no secret/node-label/personal content` | `./smackerel.sh test unit` | No |
| T107-020-I | Integration | `integration` | SCN-107-020 | `tests/integration/proactive/authorization_render_test.go` - `SCN-107-020 unauthorized content never rendered on any surface` | `./smackerel.sh test integration` | Yes |
| T107-020-A | E2E API regression | `e2e-api` | SCN-107-020 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-020 per-surface re-authorization API` | `./smackerel.sh test e2e` | Yes |
| T107-020-W | E2E UI regression | `e2e-ui` | SCN-107-020 | `web/pwa/tests/proactive-authorization.spec.ts` - `SCN-107-020 only authorized cards/provenance/correlations/activity render` | `./smackerel.sh test e2e-ui` | Yes |
| T107-08-CONTRAST | Accessibility | `e2e-ui` | SCN-107-018 | `web/pwa/tests/proactive-accessibility.spec.ts` - `NFR-107-005 WCAG 2.2 AA contrast and forced-colors across themes` | `./smackerel.sh test e2e-ui` | Yes |
| T107-08-TELEMETRY | Unit | `unit` | SCN-107-020 | `internal/web/proactive/authorization_telemetry_test.go` - `FR-107-028 bounded content-free telemetry vocabulary only` | `./smackerel.sh test unit` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-018 Keyboard and screen-reader parity for the proactive surface: every action, provenance line, correlation, and state change is reachable without a pointer in logical semantic order, with predictable focus after acting/snoozing/dismissing/asking/capturing.
- [ ] SCN-107-019 Mobile proactive surface does not overlap: on a narrow touch viewport and at 200% zoom, controls do not overlap or require precision tapping and primary touch targets are ≥44×44 CSS px.
- [ ] SCN-107-020 Provenance and correlations respect authorization: only authorized content renders across cards/provenance/correlations/activity, and nudge/correlation/activity telemetry contains no secret values, node labels, query text, or personal content.
- [ ] All themes meet WCAG 2.2 AA contrast; meaning is never carried by color/icon/motion alone; reduced-motion preserves every state change; the `NudgeRef` anti-leak boundary and content-free telemetry hold across every surface; no owner spec is modified.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-018-U passes with current-session evidence in `report.md#t107-018-u`.
- [ ] T107-018-I passes against the disposable stack with current-session evidence in `report.md#t107-018-i`.
- [ ] T107-018-A passes through production HTTP routes with current-session evidence in `report.md#t107-018-a`.
- [ ] T107-018-W passes without interception and proves keyboard/screen-reader parity in `report.md#t107-018-w`.
- [ ] T107-019-U passes with current-session evidence in `report.md#t107-019-u`.
- [ ] T107-019-I passes against the disposable stack with current-session evidence in `report.md#t107-019-i`.
- [ ] T107-019-A passes through production HTTP routes with current-session evidence in `report.md#t107-019-a`.
- [ ] T107-019-W passes without interception and proves no-overlap/44×44 touch-safe in `report.md#t107-019-w`.
- [ ] T107-020-U passes with current-session evidence in `report.md#t107-020-u`.
- [ ] T107-020-I passes against the disposable stack with current-session evidence in `report.md#t107-020-i`.
- [ ] T107-020-A passes through production HTTP routes with current-session evidence in `report.md#t107-020-a`.
- [ ] T107-020-W passes without interception and proves only-authorized rendering in `report.md#t107-020-w`.
- [ ] T107-08-CONTRAST proves WCAG 2.2 AA contrast and forced-colors across themes in `report.md#t107-08-contrast`.
- [ ] T107-08-TELEMETRY proves the bounded content-free telemetry vocabulary in `report.md#t107-08-telemetry`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, accessibility documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. No owner spec, authorizer, or
telemetry contract is modified; hardening is additive over the shared spec-106
primitives.
