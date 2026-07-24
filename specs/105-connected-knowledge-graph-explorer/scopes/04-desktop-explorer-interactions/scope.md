# SCOPE-04: Desktop Explorer Interactions

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-03

## Outcome

Deliver the desktop Graph workspace as a coherent user outcome: real topology,
pan/zoom/fit/reset, focus and selection, filters, expansion/collapse, inspector,
path controls, and reversible history over one normalized state.

## Requirements And Scenarios

- FR-105-006, FR-105-007, FR-105-008, FR-105-011, FR-105-013, FR-105-017
- SCN-105-005; interaction regressions revalidate SCN-105-003 and SCN-105-006

```gherkin
Scenario: SCN-105-005 Filtered empty differs from no data
  Given a loaded authorized graph contains people and artifacts
  When the user applies a node-kind filter with no visible matches
  Then loaded counts remain nonzero while visible counts become zero
  And Graph, Outline, and Table name the active filters and offer Clear filters
  And clearing filters restores the exact loaded identity set without another truth-changing query
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Navigate topology | Settled populated graph | Pan, zoom in/out, fit, reset, focus node and edge | Viewport changes without accidental selection; focus remains visible | e2e-ui |
| Filter and recover | Loaded people/artifact graph | Apply type/source/time/relevance filters, reach zero, clear | Filtered-empty is exclusive; same loaded IDs return | e2e-ui |
| Expand and inspect | Expandable focused node | Expand, focus new node, inspect reason, collapse | Stable prior geometry; real reason/evidence; safe collapse | e2e-ui |
| Explain path | Two connected nodes | Set endpoints, find path, focus step, clear path | Ordered reason-bearing path synchronized with Graph | e2e-ui |
| Local history | Three focus changes | Use workspace and browser Back/Forward | Focus/filter/layout/path restore; pan alone does not flood history | e2e-ui |

## Implementation Plan

1. Compose the desktop workspace from the shared shell, search, projection/layout controls, scope summary, filters, Graph region, inspector, path panel, history, and honest state band.
2. Route every pointer action through reducer commands shared with semantic projections; pan gestures cannot select or navigate and selection cannot silently open details.
3. Implement explicit zoom, fit, reset, focus, expand, collapse, path endpoint, reason/evidence, and Open details controls with stable dimensions and accessible names/tooltips.
4. Implement client visibility filters over loaded truth. Filter changes never delete normalized records or rewrite completeness.
5. Preserve existing positions and viewport during expansion; full relayout occurs only from an explicit layout/reset command.
6. Use bounded browser/workspace history for focus, filters, projection, layout, and path. Use replace semantics for pan/zoom and store no graph payload.
7. Keep existing Wiki/detail routes reachable and avoid nested cards, decorative dashboard composition, or visible instructional prose.

## Change Boundary

**Allowed:** Graph workspace composition, desktop CSS/layout, interaction
commands, filter/inspector/path/history UI, related reducer events, desktop
Playwright/unit/integration tests, user-facing docs.  
**Excluded:** mobile sheets, semantic keyboard contract, auth policy, query/path
algorithms, renderer dependency choice, concrete deployment.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-005-U | Unit | `unit` | SCN-105-005 | `web/pwa/tests/graph_state_reducer_test.go` - `SCN-105-005 filtered empty unit` | `./smackerel.sh test unit` | No |
| T105-005-I | Integration | `integration` | SCN-105-005 | `tests/integration/graph_explorer/filter_state_test.go` - `SCN-105-005 filter parity integration` | `./smackerel.sh test integration` | Yes |
| T105-005-A | E2E API regression | `e2e-api` | SCN-105-005 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-005 filter scope API` | `./smackerel.sh test e2e` | Yes |
| T105-005-W | E2E UI regression | `e2e-ui` | SCN-105-005 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-005 filtered empty reset` | `./smackerel.sh test e2e-ui` | Yes |
| T105-04-DESKTOP | E2E UI regression | `e2e-ui` | SCN-105-003, SCN-105-006 | `web/pwa/tests/graph-explorer.spec.ts` - `Desktop pan zoom fit focus filter expand collapse path inspector and history operate on real graph state` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-005 Filtered empty differs from no data: desktop users can filter to zero, retain loaded graph truth, clear filters, and recover the exact identity set while pan, zoom, fit, focus, expansion, path, inspector, and history remain coherent.
- [ ] Filtered-empty, loaded, focused, expanded, path, and inspector states stay synchronized across projections and browser history.
- [ ] Controls remain stable, accessible, discoverable through familiar icons/tooltips, and separate selection from navigation.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-005-U passes with evidence in `report.md#t105-005-u`.
- [ ] T105-005-I passes with evidence in `report.md#t105-005-i`.
- [ ] T105-005-A passes with evidence in `report.md#t105-005-a`.
- [ ] T105-005-W passes without interception with evidence in `report.md#t105-005-w`.
- [ ] T105-04-DESKTOP passes with direct DOM, geometry, pixel, focus, request-count, and history assertions in `report.md#t105-04-desktop`.

#### Build Quality Gate

- [ ] Scope tests, screenshots, no-interception and no-bailout scans, check, lint, format, docs, artifact lint, traceability, broad Wiki regression, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no desktop implementation or browser
validation was executed by the planning owner.