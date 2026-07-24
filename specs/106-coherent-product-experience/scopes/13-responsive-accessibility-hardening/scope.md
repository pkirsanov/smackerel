# SCOPE-106-13: Cross-Surface Responsive And Accessibility Hardening

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-06, SCOPE-106-07, SCOPE-106-08, SCOPE-106-09, SCOPE-106-10, SCOPE-106-11, SCOPE-106-12

## Outcome

Every integrated primary surface satisfies one responsive, appearance, typography, input, focus, announcement, and state-visibility contract across desktop, tablet, 390px, 320px at 200% zoom, keyboard, screen reader, coarse pointer, System/Light/Dark, reduced motion, and forced colors.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-009 Theme follows the user across renderers
  Given the user selects System Light or Dark and Comfortable or Compact
  When the user traverses server PWA Graph Card and admin surfaces
  Then the first paint and settled content use the same semantic tokens and typography
  And focus errors controls data states and graph encodings remain perceivable

Scenario: SCN-106-011 Mobile layout keeps primary actions available
  Given the product is opened at 390 or 320 CSS pixels and up to 200 percent zoom
  When the user completes each primary workflow
  Then text controls navigation dialogs tables panels sheets and fixed regions do not overlap clip or hide required actions
  And coarse-pointer targets are at least 44 by 44 CSS pixels

Scenario: SCN-106-012 Keyboard and screen reader journey parity
  Given the user operates without a pointer
  When the user navigates searches reads Today asks completes forms opens sheets and returns
  Then every action and terminal state is available in logical semantic order
  And focus and announcements remain predictable after navigation validation success error and privacy clearing
```

## UI Scenario Matrix

| UX ID | Required Modes | Exact Planned Test Title | File |
|---|---|---|---|
| UX-E2E-106-009 | System/Light/Dark; all viewports | `UX-E2E-106-009 appearance persists across server PWA Graph Cards and Settings with no opposite-theme frame` | `web/pwa/tests/coherent_accessibility.spec.ts` |
| UX-E2E-106-010 | Comfortable/Compact; coarse pointer | `UX-E2E-106-010 density changes spacing only and preserves content focus and 44px targets` | `web/pwa/tests/coherent_accessibility.spec.ts` |
| UX-E2E-106-011 | Keyboard, screen reader, 200% zoom | `UX-E2E-106-011 the complete primary loop works without pointer traps hidden state or focus loss` | `web/pwa/tests/coherent_accessibility.spec.ts` |
| UX-E2E-106-012 | Reduced motion, forced colors, all roots | `UX-E2E-106-012 motion and authored color are never required to perceive state selection error or graph meaning` | `web/pwa/tests/coherent_accessibility.spec.ts` |

## Implementation Plan

1. Apply stable responsive tracks and dimensions to every integrated shell/workspace/local view, reserving navigation, headers, controls, tables, graph bounds, composer, state bands, and mobile safe areas before dynamic content settles.
2. Enforce text wrapping, no viewport-scaled font sizes, zero letter spacing changes, no overlap/clipping/horizontal page overflow, and complete mobile labeled-record projection for dense tables without hidden fields.
3. Enforce one h1, logical landmarks, labels/help/error association, 44px coarse-pointer targets, visible unclipped two-layer focus, named icon controls with tooltips, and text on unfamiliar/destructive/consequential commands.
4. Enforce dialog/sheet focus trap and restoration, virtual-keyboard-safe composer/footers, keyboard alternatives to drag, and stable invoker/row context after changes.
5. Enforce concise status/alert/live announcements without repeated full-region output or unexpected focus theft.
6. Verify System/Light/Dark first paint, Comfortable/Compact semantics, WCAG 2.2 AA contrast, forced-colors boundaries, and reduced-motion removal of nonessential movement while preserving state.
7. Capture settled screenshots, accessibility snapshots, bounding boxes, target sizes, computed styles, contrast, overflow, focus sequences, announcement counts, and before/after first-paint attributes. Screenshots supplement direct behavior assertions.

## Shared Infrastructure Impact Sweep

Protected surfaces include global tokens, appearance cookie, shell layout, shared components, dialog/sheet helpers, focus/live-region helpers, Canvas sizing, and Playwright projects. Independent server, PWA, Graph, Card, Search, Assistant, form, table, and admin canaries run before the matrix.

## Change Boundary

**Allowed:** shared responsive/a11y/theme primitives and surgical per-surface composition fixes, computed-style/visual/accessibility tests.

**Excluded:** domain behavior, APIs, stores, owner tests, new UI framework, decorative redesign, foreign packets, spec 079, deployment, knb, CCManager, and readiness derivation.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-13-U | `ui-unit` | `internal/web/experience_accessibility_test.go` | SCN-106-009, 011, 012 | `TestSharedComponentsExposeStableSemanticResponsiveThemeAndFocusContracts` | `./smackerel.sh test unit --go` | No |
| XP106-13-I | `integration` | `tests/integration/experience/accessibility_render_test.go` | SCN-106-009, 011, 012 | `TestEveryIntegratedSurfaceRendersCompleteHooksFieldsAndStatesAtAllLayoutClasses` | `./smackerel.sh test integration` | Yes |
| XP106-13-A | `e2e-api` | `tests/e2e/experience_accessibility_state_e2e_test.go` | SCN-106-009, 011, 012 | `Appearance responsive and accessibility modes preserve real auth state and owner outcomes` | `./smackerel.sh test e2e` | Yes |
| UX-E2E-106-009 | `e2e-ui` | `web/pwa/tests/coherent_accessibility.spec.ts` | SCN-106-009 | `UX-E2E-106-009 appearance persists across server PWA Graph Cards and Settings with no opposite-theme frame` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-010 | `e2e-ui` | `web/pwa/tests/coherent_accessibility.spec.ts` | SCN-106-011 | `UX-E2E-106-010 density changes spacing only and preserves content focus and 44px targets` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-011 | `e2e-ui` | `web/pwa/tests/coherent_accessibility.spec.ts` | SCN-106-012 | `UX-E2E-106-011 the complete primary loop works without pointer traps hidden state or focus loss` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-012 | `e2e-ui` | `web/pwa/tests/coherent_accessibility.spec.ts` | SCN-106-012 | `UX-E2E-106-012 motion and authored color are never required to perceive state selection error or graph meaning` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-009, SCN-106-011, and SCN-106-012 provide outcome parity across every required viewport, zoom, input, theme, motion, and color mode.
- [ ] Stable dimensions, no overlap/clipping, complete fields/actions, focus, announcements, tooltips, controls, typography, and contrast are directly measured.
- [ ] Shared component canaries and owner regressions remain intact; no hardcoded visual, nested-card, or hidden-action drift remains.

#### Test Evidence - 7 Rows / 7 Items

- [ ] XP106-13-U passes with evidence in `report.md#xp106-13-u`.
- [ ] XP106-13-I passes with evidence in `report.md#xp106-13-i`.
- [ ] XP106-13-A passes with evidence in `report.md#xp106-13-a`.
- [ ] UX-E2E-106-009 passes with screenshots and computed-style evidence in `report.md#ux-e2e-106-009`.
- [ ] UX-E2E-106-010 passes with bounding-box and target-size evidence in `report.md#ux-e2e-106-010`.
- [ ] UX-E2E-106-011 passes with keyboard/accessibility/focus evidence in `report.md#ux-e2e-106-011`.
- [ ] UX-E2E-106-012 passes with reduced-motion/forced-colors evidence in `report.md#ux-e2e-106-012`.

#### Build Quality Gate

- [ ] Full visual/accessibility matrix, contrast/overflow/target/focus scans, screenshots, bundle freshness, no-hardcoded-token, no-nested-card, no-interception, check, lint, format, artifact lint, traceability, and directly affected accessibility/design documentation checks pass with zero warnings.
