# SCOPE-07: Responsive, Mobile, Reduced-Motion, And Theming

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-06

## Outcome

Deliver the complete graph journey at wide desktop, compact desktop, tablet,
390px, and 320px. Touch, explicit controls, sheets, safe areas, 200% zoom,
reduced motion, forced colors, and light/dark/system themes preserve identical
authorized graph outcomes without overlap or hidden controls.

## Requirements And Scenarios

- FR-105-006, FR-105-012, FR-105-018
- NFR-105-002, NFR-105-005, NFR-105-006
- SCN-105-010

```gherkin
Scenario: SCN-105-010 Mobile controls do not occlude the graph
  Given the explorer is open at a 320 CSS pixel coarse-pointer viewport
  When the user pans, zooms, focuses, expands, filters, inspects, finds a path, and closes a sheet
  Then every primary target is at least 44 CSS pixels
  And no sheet, safe area, label, or control overlaps the focused-node visibility region or required controls
  And closing restores focus and graph context
  And reduced motion and every supported theme preserve the same state and meaning
```

## UI Scenario Matrix

| Scenario | Viewport / Preference | Steps | Expected | Test Type |
|---|---|---|---|---|
| Mobile touch | 390x844 coarse pointer | Pan, pinch, explicit zoom, focus, filters, inspector | Gestures do not navigate; sheet does not occlude; close restores context | e2e-ui |
| Narrow completion | 320x568, 200% zoom | Focus, expand, path, open detail, return | 44px targets, no page overflow/clipping, complete flow | e2e-ui |
| Tablet sheets | 820x1180 | Open filters and inspector | Correct focus trap/restoration; graph track stable | e2e-ui |
| Desktop themes | 1440x900 light/dark/system/forced-colors | Inspect nodes, edges, paths, states | Text/focus/selection/status remain legible and equivalent | e2e-ui |
| Reduced motion | 1440x900 reduce | Expand, change layout, fit path | Immediate settled state; no force/zoom animation; same announcements | e2e-ui |

## Implementation Plan

1. Implement stable responsive tracks: wide desktop side panels, compact desktop collapsed filters, tablet modal sheets, and mobile bottom sheets with safe-area padding.
2. Map one-finger pan, pinch zoom, tap focus, and explicit zoom/fit/reset controls without making any precision gesture the sole action.
3. Preserve focused-node visibility and Graph controls across sheet snap points; restore focus to the invoking control or focused graph target on close.
4. Guarantee at least 44-by-44 CSS-pixel coarse-pointer targets, finite labels, no horizontal page overflow, and no text/control overlap at 320px and 200% zoom.
5. Honor reduced motion by drawing settled deterministic geometry without force settling, animated zoom, edge marching, or pulsing while preserving status announcements.
6. Use existing design tokens for light/dark/system themes and shape/stroke/text for forced-colors meaning; add no hardcoded colors or decorative background topology.
7. Recompute DPR/layout/hit geometry after viewport or preference changes and prove the graph remains nonblank and accurately framed.

## Change Boundary

**Allowed:** explorer responsive CSS, mobile/tablet sheets, gesture adapter,
theme/motion token consumption, viewport/DPR handling, responsive/a11y tests and
docs.  
**Excluded:** graph truth/query/path algorithms, auth policy, source-lock choice,
saved-view schema, deploy adapter, unrelated PWA screens.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-010-U | Unit | `unit` | SCN-105-010 | `web/pwa/tests/graph_responsive_contract_test.go` - `SCN-105-010 responsive contract unit` | `./smackerel.sh test unit` | No |
| T105-010-I | Integration | `integration` | SCN-105-010 | `tests/integration/graph_explorer/responsive_state_test.go` - `SCN-105-010 responsive state integration` | `./smackerel.sh test integration` | Yes |
| T105-010-A | E2E API regression | `e2e-api` | SCN-105-010 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-010 mobile bounded API` | `./smackerel.sh test e2e` | Yes |
| T105-010-W | E2E UI regression | `e2e-ui` | SCN-105-010 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-010 mobile controls and sheets` | `./smackerel.sh test e2e-ui` | Yes |
| T105-07-MOTION | E2E UI regression | `e2e-ui` | SCN-105-010 | `web/pwa/tests/graph-explorer.spec.ts` - `Reduced motion removes force and zoom animation without changing outcomes` | `./smackerel.sh test e2e-ui` | Yes |
| T105-07-THEME | E2E UI accessibility | `e2e-ui` | SCN-105-010 | `web/pwa/tests/graph-explorer.spec.ts` - `Light dark system and forced colors preserve graph meaning and contrast` | `./smackerel.sh test e2e-ui` | Yes |
| T105-07-VIEWPORTS | E2E UI regression | `e2e-ui` | SCN-105-010 | `web/pwa/tests/graph-explorer.spec.ts` - `Desktop tablet 390 and 320 viewports have no overlap clipping or horizontal overflow` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-010 Mobile controls do not occlude the graph: the complete Graph/Outline/Table journey remains operable on desktop, tablet, 390px, and 320px with touch, explicit controls, keyboard, and 200% zoom.
- [ ] Sheets, safe areas, targets, labels, and controls never hide the focused graph target or required actions, and focus restores on close.
- [ ] Reduced motion, light/dark/system themes, and forced colors preserve identical semantic outcomes without animation or color dependence.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-010-U passes with evidence in `report.md#t105-010-u`.
- [ ] T105-010-I passes with evidence in `report.md#t105-010-i`.
- [ ] T105-010-A passes with evidence in `report.md#t105-010-a`.
- [ ] T105-010-W passes without interception with bounding-box and focus evidence in `report.md#t105-010-w`.
- [ ] T105-07-MOTION passes with animation-duration and settled-state evidence in `report.md#t105-07-motion`.
- [ ] T105-07-THEME passes with screenshots, contrast, and semantic evidence in `report.md#t105-07-theme`.
- [ ] T105-07-VIEWPORTS passes at every declared viewport and zoom level in `report.md#t105-07-viewports`.

#### Build Quality Gate

- [ ] Scope tests, screenshot/overlap/target-size scans, accessibility checks, check, lint, format, design-token review, docs, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no responsive, touch, theme, motion, or
browser validation was executed by the planning owner.