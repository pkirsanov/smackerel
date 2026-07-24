# SCOPE-05: Correlation Rail (bounded spec-105 neighborhood + deep-link)

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-04

## Outcome

Attach an always-on "related / heads-up" correlation rail (a spec-106 Inspector)
to any knowledge item view (topic, person, place, capture, artifact), populated by
`CorrelationRailRead` — a `RAIL_MAX`-bounded `GraphQueryService.Neighborhood(seed,
depth=1)` call of the **same** spec-105 contract under the **same**
`GraphReaderAuthorizer`, returning real `GraphNodeV1` rows + `GraphReasonResolver`
reasons + the stable `<kind>:<id>` seed. `See full graph` deep-links into the
spec-105 explorer on the current seed. An honest `no-related-items` (105
`isolated-only`) is distinct from `unavailable` (read failure); the rail is a
tighter-bounded projection that never re-implements the explorer, never caches
graph data, and never draws a decorative correlation.

## Requirements And Scenarios

- FR-107-013, FR-107-014, FR-107-015, FR-107-016, FR-107-027, FR-107-030, NFR-107-003
- SCN-107-010, SCN-107-011

```gherkin
Scenario: SCN-107-010 Correlation rail shows real edges and deep-links into the explorer
  Given the user opens a topic that has stored edges to people, captures, and artifacts
  When the item view renders
  Then an always-on related/heads-up rail shows a bounded set of those real correlations
  And each correlation deep-links into the spec-105 graph explorer for the full view
```

```gherkin
Scenario: SCN-107-011 No-correlation item is honest, not decorative
  Given the user opens an item that has no stored edge to any other authorized item
  When the item view renders
  Then the correlation rail shows an honest no-related-items state
  And it draws no decorative or fabricated correlations
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Real edges + deep-link | Disposable stack; a topic with stored edges to people/captures/artifacts; allowlisted reader | Open the item; read the rail; follow a correlation and `See full graph` | A bounded (`RAIL_MAX`) set of real `GraphNodeV1` rows with typed reasons; each row + `See full graph` deep-links into the spec-105 explorer on the `<kind>:<id>` seed | e2e-ui |
| No-related honesty | An item with no stored edge to any authorized item (a complete read) | Open the item | Honest `no-related-items` state (105 `isolated-only`); no decorative, inferred-without-provenance, or fabricated correlation | integration / e2e-ui |
| Unavailable ≠ empty | The graph read fails/unavailable | Open the item | A distinct `unavailable` state, never an empty `no-related-items`; a principal outside the reader allowlist sees `unavailable`/absent, never a fabricated correlation | integration / e2e-ui |
| Bounded read | A high-degree seed | Render the rail | The rail requests only `RAIL_MAX` neighbors at `depth=1` and never renders the full edge store (NFR-107-003) | unit |

## Implementation Plan

1. Implement `CorrelationRailRead(principal, seed=<kind>:<id>)` as a tighter-bounded call of the spec-105 neighborhood contract: `GraphQueryService.Neighborhood(seed, depth=1, limit=RAIL_MAX)` through the same `GraphReaderAuthorizer` (`auth.RequireScope("knowledge-graph:read")` + the `knowledge_graph_api.reader_user_ids` allowlist) and the same `GraphNodeV1`/`GraphEdgeV1`/`GraphReasonResolver` outputs. It is NOT a second graph read path (FR-107-014, coordination note 2).
2. Render each row as a real `GraphEdgeV1` to a typed related `GraphNodeV1` with its `GraphReasonResolver` reason (`mentions`/`related to`/`about`/`supports`/`located`) and a focusable open action, inside a spec-106 Inspector (desktop side rail, mobile bottom sheet, nonvisual outline — three projections of the same bounded read). Every row is a real stored edge; no decorative or fabricated correlation (FR-107-015).
3. Deep-link: `See full graph ▸` → `/knowledge/graph?view=graph&layout=neighborhood&seed=<current-key>&focus=<current-key>` (the exact spec-105 shape); a single row's open action → `...&seed=<current-key>&focus=<related-key>`. Pass only the authorized seed identifier; the explorer re-authorizes on entry (105 FR-105-016). Reuse the existing spec-105 "Explore connections" launch (105 UC-105-002).
4. Honest outcomes through `HonestStatePresenter`: `correlated` (1..N real edges), `no-related-items` (105 `isolated-only` — a successful zero-edge read), `unavailable` (read failed), `unauthorized` (principal not in the reader allowlist → `unavailable`/absent). A no-correlation item and a failed read are never the same state (FR-107-016).
5. No parallel store, no client cache: the bounded result lives in memory for the current item view only; it is never persisted to localStorage/sessionStorage/IndexedDB/CacheStorage/the service worker; each item view re-queries + re-authorizes (FR-107-027). NFR-107-003: never render the full edge store.
6. Update architecture/testing documentation through the docs owner during implementation; do not modify `specs/105-*` (the seed/launch stability is a coordination note).

## SST No-Default Decision (Reserved)

- `RAIL_MAX = 6` — the rail neighborhood bound. Fail-loud SST key under `config/smackerel.yaml`; config-compile validates it is an integer in `1..N` (tighter than the spec-105 explorer workspace bound). It caps `GraphQueryService.Neighborhood(seed, depth=1, limit=RAIL_MAX)`; the rail never renders the full edge store (NFR-107-003). No `${VAR:-default}`/`os.getenv(k, default)` fallback. (design.md OQ5.)

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-105 `GraphQueryService.Neighborhood`, `GraphNodeV1`/`GraphEdgeV1`/`GraphReasonResolver`, `GraphReaderAuthorizer`, the `<kind>:<id>` seed + `/knowledge/graph?...` deep-link, and `isolated-only`; the spec-080 graph-read authorization; the spec-106 Inspector primitive.
- **Independent canaries:** the existing spec-105 explorer deep-link entry stays green; existing item/detail views stay usable; the service worker never caches `/api/*` (including the neighborhood read); the graph reader allowlist is unchanged.
- **Rollback:** the rail is additive and reads only through the owner contract; disabling it leaves item views intact and the explorer deep-link untouched; no graph store, bound, or authorizer is mutated.

## Change Boundary

**Allowed during execution:** the `CorrelationRailRead` composition endpoint (a
bounded call of the spec-105 contract), the rail Inspector render (desktop/mobile/
nonvisual), the `proactive:` SST `RAIL_MAX` key, and tests/docs named by this
scope.  
**Excluded:** editing `specs/105-*`, the `GraphQueryService`, its bounds, layout,
reasons, or store; introducing a second graph read path or a graph client cache;
the cockpit/palette/feed surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-010-U | Unit | `unit` | SCN-107-010 | `web/pwa/tests/correlation_rail_model_test.ts` - `SCN-107-010 rail renders bounded real GraphNodeV1 rows with reasons` | `./smackerel.sh test unit` | No |
| T107-010-I | Integration | `integration` | SCN-107-010 | `tests/integration/proactive/correlation_rail_read_test.go` - `SCN-107-010 RAIL_MAX-bounded neighborhood via the spec-105 contract` | `./smackerel.sh test integration` | Yes |
| T107-010-A | E2E API regression | `e2e-api` | SCN-107-010 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-010 correlation rail read API` | `./smackerel.sh test e2e` | Yes |
| T107-010-W | E2E UI regression | `e2e-ui` | SCN-107-010 | `web/pwa/tests/correlation-rail.spec.ts` - `SCN-107-010 real edges deep-link into the spec-105 explorer` | `./smackerel.sh test e2e-ui` | Yes |
| T107-011-U | Unit | `unit` | SCN-107-011 | `web/pwa/tests/correlation_rail_model_test.ts` - `SCN-107-011 zero-edge read maps to honest no-related-items` | `./smackerel.sh test unit` | No |
| T107-011-I | Integration | `integration` | SCN-107-011 | `tests/integration/proactive/correlation_rail_read_test.go` - `SCN-107-011 isolated-only is honest no-related, unavailable is distinct` | `./smackerel.sh test integration` | Yes |
| T107-011-A | E2E API regression | `e2e-api` | SCN-107-011 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-011 no-related vs unavailable API` | `./smackerel.sh test e2e` | Yes |
| T107-011-W | E2E UI regression | `e2e-ui` | SCN-107-011 | `web/pwa/tests/correlation-rail.spec.ts` - `SCN-107-011 no-related-items draws no decorative correlation` | `./smackerel.sh test e2e-ui` | Yes |
| T107-05-BOUND | Unit | `unit` | SCN-107-010 | `tests/integration/proactive/correlation_rail_read_test.go` - `NFR-107-003 rail requests only RAIL_MAX neighbors and never the full store` | `./smackerel.sh test unit` | No |
| T107-05-CANARY | Shared-shell canary | `e2e-ui` | SCN-107-010 | `web/pwa/tests/photos_lifecycle_review.spec.ts` - `correlation rail preserves existing item views and explorer deep-link` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-010 Correlation rail shows real edges and deep-links into the explorer: an always-on rail shows a bounded (`RAIL_MAX`) set of real stored correlations for the item, each deep-linking into the spec-105 explorer on the seed.
- [ ] SCN-107-011 No-correlation item is honest, not decorative: an item with no authorized stored edge shows an honest `no-related-items` state and draws no decorative or fabricated correlation, distinct from `unavailable`.
- [ ] The rail is a tighter-bounded call of the spec-105 neighborhood contract (no second graph path, no graph client cache), re-authorizes each render (FR-107-027), and never renders the full edge store (NFR-107-003); `specs/105-*` is not modified.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-010-U passes with current-session evidence in `report.md#t107-010-u`.
- [ ] T107-010-I passes against the disposable stack with current-session evidence in `report.md#t107-010-i`.
- [ ] T107-010-A passes through production HTTP routes with current-session evidence in `report.md#t107-010-a`.
- [ ] T107-010-W passes without interception and proves the real-edge deep-link in `report.md#t107-010-w`.
- [ ] T107-011-U passes with current-session evidence in `report.md#t107-011-u`.
- [ ] T107-011-I passes against the disposable stack with current-session evidence in `report.md#t107-011-i`.
- [ ] T107-011-A passes through production HTTP routes with current-session evidence in `report.md#t107-011-a`.
- [ ] T107-011-W passes without interception and proves the honest no-related-items state in `report.md#t107-011-w`.
- [ ] T107-05-BOUND proves the `RAIL_MAX` bound and no full-store render in `report.md#t107-05-bound`.
- [ ] T107-05-CANARY independently proves existing item views and the explorer deep-link stay green in `report.md#t107-05-canary`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation (including the `RAIL_MAX` no-default SST key), architecture documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. `specs/105-*` is a deep-link
dependency; the seed/launch-stability confirmation is a coordination note, not an
edit.
