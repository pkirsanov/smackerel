# SCOPE-01: Graph Contract And Query Foundation

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Tags:** foundation:true  
**Depends On:** External gate - `BUG-080-001` certified Done

## Outcome

Deliver one authorized, bounded overview vertical slice from canonical
PostgreSQL graph records through `GraphQueryService`, the public API, normalized
browser state, and a semantic scope summary/Table projection. This scope proves
real graph truth before cursor, renderer, or interaction overlays begin.

## Requirements And Scenarios

- FR-105-001, FR-105-002, FR-105-003, FR-105-005, FR-105-014, FR-105-019, FR-105-020, FR-105-022, FR-105-023, FR-105-024
- SCN-105-001, SCN-105-014, SCN-105-015

```gherkin
Scenario: SCN-105-001 Open a real bounded graph
  Given BUG-080-001 is certified and an authenticated allowlisted reader has stored canonical graph records
  When the reader opens the Graph Explorer bounded overview
  Then one authorized query returns no more than the configured node, edge, and byte limits
  And every node and edge resolves from the canonical PostgreSQL graph
  And the semantic scope summary reports the seed, completeness, loaded counts, visible counts, and more-available state
  And no complete-store query, sample topology, or fabricated explanation is used
```

```gherkin
Scenario: SCN-105-014 Connected overview meets a real-edge minimum
  Given the authorized corpus contains isolated nodes and one stored component of at least two nodes and one edge
  When the bounded overview settles
  Then it may identify connected knowledge only from the real component
  And reports declared node, edge, hop, and continuation bounds
  And never creates a relationship to make isolated nodes look connected
```

```gherkin
Scenario: SCN-105-015 Isolated-only corpus is honest
  Given authorized graph nodes exist but no stored relationship joins any pair in the current permitted corpus
  When the user opens the explorer
  Then the product shows a no-connected-overview state and the real isolated-node count
  And offers safe detail, capture, or source actions without sample topology
  And does not call the result first-use empty, connected, failed, or unavailable
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Bounded overview | Disposable seeded graph; valid scoped session | Open Knowledge, choose Graph, inspect scope and Table | Real authorized IDs, bounded counts, complete/partial state, no fake topology | e2e-ui |
| Connected minimum | Seeded graph with isolated nodes plus one component of >=2 nodes and >=1 stored edge | Open Graph and settle the bounded overview | Connected claim only from the real component; declared node/edge/hop/continuation bounds reported; no synthesized edge | integration / e2e-ui |
| Isolated-only honesty | Seeded authorized nodes with zero qualifying stored edges in a complete read | Open Graph | `no-connected-overview` with real isolated-node count and safe next actions; not first-use empty, connected, failed, or unavailable | integration / e2e-ui |
| True query failure | Real query dependency unavailable | Open Graph | Typed unavailable state, never empty success | e2e-api / e2e-ui |

## Implementation Plan

1. Consume the BUG-080 `AuthorizedGraphRead`, route manifest, activation state, reader authorization, cursor-secret boundary, and closed outcome vocabulary without duplicating them.
2. Add normalized `GraphNodeV1`, `GraphEdgeV1`, reason/evidence, scope, omission, and completeness contracts for artifact, topic, person, place, concept, and entity kinds.
3. Implement bounded canonical node/edge repositories and `GraphQueryService` overview selection with explicit configured limits, deterministic ordering, request cancellation, byte bounds, and no parallel graph store.
4. Compute connectedness with a component analyzer over authorized stored edges: `connected` requires at least one component of two distinct real nodes joined by one stored edge; a complete read with no qualifying edge yields `isolated-only`/`no-connected-overview` and reports the real isolated-node count; a bounded/limited read with no observed qualifying edge yields `unresolved-partial`. No edge is ever synthesized to make isolated nodes look connected, and the response reports declared node, edge, hop, and continuation bounds.
5. Register `POST /api/graph/query` atomically behind session auth, `knowledge-graph:read`, and the explicit reader allowlist. Actor identity comes only from context.
6. Add strict browser response validation, normalized in-memory state, scope summary, connectedness state (`connected` / `isolated-only` / `no-connected-overview` / `unresolved-partial`), and semantic Table projection so the first slice is user-visible and testable before Canvas exists.
7. Emit closed, content-free query metrics/logs/traces. Do not tag rows with `observabilityWorkflow` until a graph workflow is added to project config.
8. Update API/architecture/testing/operator documentation through the docs owner during implementation; this planning packet does not edit those surfaces.

## Shared Infrastructure Impact Sweep

- **Protected contracts:** BUG-080 activation, auth middleware order, reader allowlist, cursor secret resolution, graph route manifest, PostgreSQL test bootstrap, PWA shell/session hydration.
- **Independent canaries:** existing fixed-order Graph family synthetic; authenticated non-Graph shell navigation; current Wiki list/detail reads; service-worker `/api/*` network-only behavior.
- **Rollback:** source/config pointer rollback leaves canonical graph records unchanged and may not restore nullable handler activation.

## Change Boundary

**Allowed during execution:** graph API contracts/repositories/query service,
graph-specific config compiler/schema, PWA explorer state/Table shell, graph
metrics/traces, migration/tests/docs named by this scope.  
**Excluded:** force/renderer implementation, path traversal, saved-view writes,
concrete deploy adapters, release-train mutation, graph record construction,
and unrelated product routes.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-001-U | Unit | `unit` | SCN-105-001 | `internal/api/graphapi/query_service_test.go` - `SCN-105-001 bounded canonical overview unit` | `./smackerel.sh test unit` | No |
| T105-001-I | Integration | `integration` | SCN-105-001 | `tests/integration/graphapi/query_service_test.go` - `SCN-105-001 bounded canonical overview integration` | `./smackerel.sh test integration` | Yes |
| T105-001-A | E2E API regression | `e2e-api` | SCN-105-001 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-001 bounded canonical overview API` | `./smackerel.sh test e2e` | Yes |
| T105-001-W | E2E UI regression | `e2e-ui` | SCN-105-001 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-001 real bounded graph is nonblank and truthful` | `./smackerel.sh test e2e-ui` | Yes |
| T105-014-U | Unit | `unit` | SCN-105-014 | `internal/api/graphapi/component_analyzer_test.go` - `SCN-105-014 connected component minimum unit` | `./smackerel.sh test unit` | No |
| T105-014-I | Integration | `integration` | SCN-105-014 | `tests/integration/graphapi/overview_component_test.go` - `SCN-105-014 connected overview minimum integration` | `./smackerel.sh test integration` | Yes |
| T105-014-A | E2E API regression | `e2e-api` | SCN-105-014 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-014 connected overview minimum API` | `./smackerel.sh test e2e` | Yes |
| T105-014-W | E2E UI regression | `e2e-ui` | SCN-105-014 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-014 connected overview real edge minimum` | `./smackerel.sh test e2e-ui` | Yes |
| T105-015-U | Unit | `unit` | SCN-105-015 | `internal/api/graphapi/component_analyzer_test.go` - `SCN-105-015 isolated-only classification unit` | `./smackerel.sh test unit` | No |
| T105-015-I | Integration | `integration` | SCN-105-015 | `tests/integration/graphapi/overview_component_test.go` - `SCN-105-015 isolated-only honest integration` | `./smackerel.sh test integration` | Yes |
| T105-015-A | E2E API regression | `e2e-api` | SCN-105-015 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-015 no-connected-overview API` | `./smackerel.sh test e2e` | Yes |
| T105-015-W | E2E UI regression | `e2e-ui` | SCN-105-015 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-015 isolated-only honest state` | `./smackerel.sh test e2e-ui` | Yes |
| T105-01-CANARY | Shared-shell canary | `e2e-ui` | SCN-105-001 | `web/pwa/tests/wiki.spec.ts` - `Graph foundation preserves authenticated Wiki and shell navigation` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-001 Open a real bounded graph: one foundation-owned authorized query contract serves canonical graph truth to the API and semantic browser projection within explicit bounds.
- [ ] SCN-105-014 Connected overview meets a real-edge minimum: the component analyzer claims `connected` only from a real component of >=2 nodes and >=1 stored edge, reports declared node/edge/hop/continuation bounds, and synthesizes no edge.
- [ ] SCN-105-015 Isolated-only corpus is honest: a complete read with no qualifying edge yields `no-connected-overview` with the real isolated-node count and safe next actions, distinct from first-use empty, connected, failed, and unavailable.
- [ ] Every public node/edge/reason/evidence field is allowlisted, source-backed, and principal-authorized; unsupported semantics produce typed partial state.
- [ ] Shared auth, activation, Wiki, shell, service-worker, and rollback contracts remain intact.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-001-U passes with current-session evidence in `report.md#t105-001-u`.
- [ ] T105-001-I passes against disposable PostgreSQL with current-session evidence in `report.md#t105-001-i`.
- [ ] T105-001-A passes through production HTTP routes with current-session evidence in `report.md#t105-001-a`.
- [ ] T105-001-W passes without interception and proves real bounded identity/count state in `report.md#t105-001-w`.
- [ ] T105-014-U passes with current-session evidence in `report.md#t105-014-u`.
- [ ] T105-014-I passes against disposable PostgreSQL with current-session evidence in `report.md#t105-014-i`.
- [ ] T105-014-A passes through production HTTP routes with current-session evidence in `report.md#t105-014-a`.
- [ ] T105-014-W passes without interception and proves the connected minimum from real topology in `report.md#t105-014-w`.
- [ ] T105-015-U passes with current-session evidence in `report.md#t105-015-u`.
- [ ] T105-015-I passes against disposable PostgreSQL with current-session evidence in `report.md#t105-015-i`.
- [ ] T105-015-A passes through production HTTP routes with current-session evidence in `report.md#t105-015-a`.
- [ ] T105-015-W passes without interception and proves the honest no-connected-overview state in `report.md#t105-015-w`.
- [ ] T105-01-CANARY independently proves Wiki and authenticated shell behavior in `report.md#t105-01-canary`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, API documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because the dependency bug, implementation, tests,
and runtime validation have not been executed by the planning owner.