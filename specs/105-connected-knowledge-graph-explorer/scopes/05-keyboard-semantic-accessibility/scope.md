# SCOPE-05: Keyboard, Semantic Projections, And Accessibility

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-04

## Outcome

Make Graph, Outline, and Table equivalent operating projections. A keyboard or
screen-reader user can focus neighbors and relationships, expand/collapse,
filter, inspect reasons/evidence, select path endpoints, follow a path, fit the
view, and open details without depending on coordinates, pointer input, color,
or animation.

## Requirements And Scenarios

- FR-105-006, FR-105-010, FR-105-012, FR-105-018, FR-105-019, FR-105-025
- NFR-105-005, NFR-105-006
- SCN-105-008, SCN-105-009, SCN-105-016

```gherkin
Scenario: SCN-105-008 Keyboard-equivalent exploration
  Given the loaded graph has semantic adjacency and visible focus
  When a keyboard user moves to a neighbor, expands it, applies a filter, fits the view, and opens details
  Then each command produces the same reducer outcome as pointer input
  And focus remains visible and predictable with no keyboard trap
  And graph changes are announced without treating screen coordinates as meaning

Scenario: SCN-105-009 Screen-reader relationship outline
  Given Graph contains a focused topic with three relationships and a selected path
  When Outline renders from the shared ExplorerState
  Then its node and edge identities, reasons, evidence, filters, focus, and ordered path equal Graph state
  And expansion, filtering, path selection, and detail navigation use semantic buttons and links in logical order
```

```gherkin
Scenario: SCN-105-016 Graph Outline and Table are equivalent
  Given one bounded authorized explorer state with active filters, focus, and a selected path
  When the user switches among Graph, Outline, and Table
  Then each view exposes the same authorized node IDs, relationship IDs, directions, reasons, path order, filters, and settled counts
  And a visual render failure leaves Outline and Table usable without changing graph truth
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Keyboard Graph | Populated graph; pointer unused | Enter Graph, move by adjacency, expand, filter, fit, inspect, Escape | Real state changes, visible focus, invoker restoration, no trap | e2e-ui |
| Outline parity | Same graph/path/filter state | Switch Graph to Outline and operate controls | Exact ID/reason/path parity in semantic order | integration / e2e-ui |
| Table parity | Same graph state | Switch to Table, sort, select edge, continue page | Sort semantics; same focused edge; bounded continuation | e2e-ui |
| Three-view equivalence | One bounded state with filters, focus, and a selected path; Canvas-failure profile available | Switch among Graph, Outline, Table; force a Canvas render failure | Identical node/edge IDs, directions, reasons, path order, filters, and settled counts; Outline/Table stay usable when Canvas fails | integration / e2e-ui |
| Announcements | Initial, expansion, path, partial failure | Complete each operation | One concise terminal announcement; no repeated full graph | e2e-ui |

## Implementation Plan

1. Define one command map over semantic adjacency. Arrow/next/previous behavior follows graph relationships, never arbitrary canvas direction.
2. Complete Outline with nested lists and native controls unless the full ARIA tree contract is implemented; incomplete ARIA tree semantics are prohibited.
3. Complete Table with semantic headers, sort state, text direction, responsive record form, and bounded continuation from the same state.
4. Synchronize DOM focus, reducer focus, Canvas selection, inspector, filters, and path state across projection changes without another graph fetch; a Canvas render failure leaves Outline and Table fully operable on the same ExplorerState and never triggers a refetch or changes graph truth.
5. Add concise operation live regions, blocking alert behavior, focus restoration, shortcut/help dialog, 200% zoom/reflow, forced-colors, and contrast checks.
6. Ensure spatial position, color, size, stroke, and animation are supplemental only; type, direction, status, reason, evidence, and path order remain textual.
7. Run accessibility snapshots and direct keyboard journeys against real stack data; do not substitute mocked DOM fixtures for live E2E acceptance.

## Change Boundary

**Allowed:** semantic projection components, command map, focus/live-region
logic, Outline/Table styles, accessibility tests/docs, shared reducer events
needed for parity.  
**Excluded:** query/path algorithms, auth policy, renderer physics/layout,
mobile sheet composition, deploy adapters, graph data mutation.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-008-U | Unit | `unit` | SCN-105-008 | `web/pwa/tests/graph_keyboard_commands_test.go` - `SCN-105-008 keyboard command unit` | `./smackerel.sh test unit` | No |
| T105-008-I | Integration | `integration` | SCN-105-008 | `tests/integration/graph_explorer/keyboard_projection_test.go` - `SCN-105-008 keyboard parity integration` | `./smackerel.sh test integration` | Yes |
| T105-008-A | E2E API regression | `e2e-api` | SCN-105-008 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-008 keyboard authorization API` | `./smackerel.sh test e2e` | Yes |
| T105-008-W | E2E UI regression | `e2e-ui` | SCN-105-008 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-008 keyboard-only exploration` | `./smackerel.sh test e2e-ui` | Yes |
| T105-009-U | Unit | `unit` | SCN-105-009 | `web/pwa/tests/graph_semantic_projection_test.go` - `SCN-105-009 semantic outline unit` | `./smackerel.sh test unit` | No |
| T105-009-I | Integration | `integration` | SCN-105-009 | `tests/integration/graph_explorer/keyboard_projection_test.go` - `SCN-105-009 projection parity integration` | `./smackerel.sh test integration` | Yes |
| T105-009-A | E2E API regression | `e2e-api` | SCN-105-009 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-009 semantic parity API` | `./smackerel.sh test e2e` | Yes |
| T105-009-W | E2E UI regression | `e2e-ui` | SCN-105-009 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-009 screen-reader outline parity` | `./smackerel.sh test e2e-ui` | Yes |
| T105-016-U | Unit | `unit` | SCN-105-016 | `web/pwa/tests/graph_projection_parity_test.go` - `SCN-105-016 projection parity unit` | `./smackerel.sh test unit` | No |
| T105-016-I | Integration | `integration` | SCN-105-016 | `tests/integration/graph_explorer/projection_equivalence_test.go` - `SCN-105-016 projection equivalence integration` | `./smackerel.sh test integration` | Yes |
| T105-016-A | E2E API regression | `e2e-api` | SCN-105-016 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-016 projection identity API` | `./smackerel.sh test e2e` | Yes |
| T105-016-W | E2E UI regression | `e2e-ui` | SCN-105-016 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-016 graph outline table equivalent and Canvas-failure safe` | `./smackerel.sh test e2e-ui` | Yes |
| T105-05-A11Y | E2E UI accessibility | `e2e-ui` | SCN-105-008, SCN-105-009 | `web/pwa/tests/graph-explorer.spec.ts` - `Graph Outline and Table remain equivalent at 200 percent zoom and forced colors` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-008 Keyboard-equivalent exploration: every Graph command has an equivalent semantic command and produces the same authorized state transition without pointer input or coordinate meaning.
- [ ] SCN-105-009 Screen-reader relationship outline: Outline/Table identities, reasons, evidence, filters, focus, path, completeness, and continuation match Graph exactly in semantic order.
- [ ] SCN-105-016 Graph, Outline, and Table are equivalent: switching among the three views preserves identical authorized node/relationship IDs, directions, reasons, path order, filters, focus, and settled counts, and a Canvas render failure leaves Outline and Table usable without changing graph truth.
- [ ] Focus, announcements, native semantics, zoom/reflow, forced colors, contrast, and keyboard behavior satisfy WCAG 2.2 AA without traps or silent bailouts.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-008-U passes with evidence in `report.md#t105-008-u`.
- [ ] T105-008-I passes with evidence in `report.md#t105-008-i`.
- [ ] T105-008-A passes with evidence in `report.md#t105-008-a`.
- [ ] T105-008-W passes using keyboard only and no interception in `report.md#t105-008-w`.
- [ ] T105-009-U passes with evidence in `report.md#t105-009-u`.
- [ ] T105-009-I passes with evidence in `report.md#t105-009-i`.
- [ ] T105-009-A passes with evidence in `report.md#t105-009-a`.
- [ ] T105-009-W passes with accessibility snapshot and direct action evidence in `report.md#t105-009-w`.
- [ ] T105-016-U passes with evidence in `report.md#t105-016-u`.
- [ ] T105-016-I passes with evidence in `report.md#t105-016-i`.
- [ ] T105-016-A passes with evidence in `report.md#t105-016-a`.
- [ ] T105-016-W passes without interception and proves three-view equivalence plus Canvas-failure safety in `report.md#t105-016-w`.
- [ ] T105-05-A11Y passes at 200% zoom and forced colors with evidence in `report.md#t105-05-a11y`.

#### Build Quality Gate

- [ ] Scope tests, accessibility snapshots/scans, keyboard and no-bailout scans, check, lint, format, docs, artifact lint, traceability, broad PWA regression, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no keyboard, accessibility, or real-stack
projection validation was executed by the planning owner.