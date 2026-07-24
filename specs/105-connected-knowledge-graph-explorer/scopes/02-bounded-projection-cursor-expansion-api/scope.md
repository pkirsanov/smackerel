# SCOPE-02: Bounded Projection, Cursor, Expansion, And Path API

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-01

## Outcome

Deliver bounded progressive expansion and path finding end to end: principal-
bound cursors/scope tokens, deterministic repositories, normalized merge and
collapse ownership, semantic controls, reason/evidence-bearing paths, and
branch-local failure recovery.

## Requirements And Scenarios

- FR-105-004, FR-105-005, FR-105-009, FR-105-010, FR-105-013, FR-105-015, FR-105-019
- Primary scenario: SCN-105-006. This scope also plans prerequisite regression rows for SCN-105-003 and SCN-105-007.

```gherkin
Scenario: SCN-105-006 Explain a multi-step path
  Given two authorized endpoints have a stored bounded path
  When the user requests a path with the signed current scope
  Then every ordered step carries stored edge identity, endpoints, direction, reason, and evidence state
  And focusing a step selects the same edge in every available projection
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Expand twice | Seeded multi-hop graph | Focus node, expand twice, collapse first branch | Deduplicated real IDs; shared/path nodes retained; context stable | e2e-ui |
| Explain bridge | Two connected endpoints | Set start/end, find path, focus each step | Ordered stored reasons and evidence links | e2e-api / e2e-ui |
| No path vs partial | Separate exhaustive and bounded-failure fixtures | Resolve each path | Exclusive truthful states and safe retry | integration / e2e-ui |

## Implementation Plan

1. Add `GraphCursorV2` and signed scope tokens bound to principal hash, operation, seed, filters, depth, as-of boundary, stable edge key, issuance, and expiry; reject malformed, tampered, cross-user, cross-query, and expired tokens.
2. Implement deterministic incoming/outgoing bounded edge pages, keyset continuation, node batch resolution, explicit omissions, and configured client/session caps.
3. Add neighborhood mode to `POST /api/graph/query` and normalized merge/dedup/expansion-ownership reducer events. Failed work changes only the addressed branch.
4. Implement server-side bounded bidirectional BFS for `POST /api/graph/path`, preserving stored direction and truthful found/no-path/partial distinctions.
5. Extend the semantic projection and inspector with expand, collapse, set path endpoints, ordered reasons/evidence, retry, and progress controls so this remains a vertical slice before Canvas.
6. Add query indexes only through the planned migration; prove representative plans use those indexes and provide rollback/preservation rules.
7. Keep graph records read-only. No client path inference, offset cursor reuse, hidden clamping, or background whole-graph work is permitted.

## Security And Privacy

- Cursor/scope tokens never enter logs, metrics, traces, saved views, URLs, or durable client storage.
- Request bodies cannot select actor identity or weaken reader allowlisting.
- Evidence links remain same-origin and re-authorize on open; redacted evidence exposes no IDs or labels.

## Change Boundary

**Allowed:** graph query/path services, edge/node repositories, cursor/scope
codec, reducer expansion/path events, semantic controls, migration 063 query
indexes, tests/docs.  
**Excluded:** Canvas drawing, mobile composition, general auth behavior,
concrete deployment, graph mutation, parallel search/index services.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-006-U | Unit | `unit` | SCN-105-006 | `internal/api/graphapi/path_service_test.go` - `SCN-105-006 path reason unit` | `./smackerel.sh test unit` | No |
| T105-006-I | Integration | `integration` | SCN-105-006 | `tests/integration/graphapi/path_service_test.go` - `SCN-105-006 path evidence integration` | `./smackerel.sh test integration` | Yes |
| T105-006-A | E2E API regression | `e2e-api` | SCN-105-006 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-006 path API` | `./smackerel.sh test e2e` | Yes |
| T105-006-W | E2E UI regression | `e2e-ui` | SCN-105-006 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-006 ordered explained path` | `./smackerel.sh test e2e-ui` | Yes |
| T105-02-CURSOR | Security regression | `e2e-api` | SCN-105-003 | `tests/e2e/graph_explorer_e2e_test.go` - `Graph cursors reject tamper cross-user cross-query and expiry without disclosure` | `./smackerel.sh test e2e` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] Query continuation and path scope are principal/query-bound, deterministic, bounded, cancellable, and truthful about limits or failure.
- [ ] Expansion merge/collapse preserves orientation, shared identities, focus, filters, viewport, path endpoints, and prior useful state.
- [ ] SCN-105-006 Explain a multi-step path: path steps expose stored direction, reasons, and authorized evidence; no client inference or false no-path exists.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-006-U passes with evidence in `report.md#t105-006-u`.
- [ ] T105-006-I passes with evidence in `report.md#t105-006-i`.
- [ ] T105-006-A passes with evidence in `report.md#t105-006-a`.
- [ ] T105-006-W passes without interception with evidence in `report.md#t105-006-w`.
- [ ] T105-02-CURSOR passes with value-safe evidence in `report.md#t105-02-cursor`.

#### Build Quality Gate

- [ ] Scope tests, migration/query-plan checks, auth/privacy scans, check, lint, format, docs, artifact lint, traceability, regression quality, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation and validation have not been
executed by the planning owner.