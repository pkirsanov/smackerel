# SCOPE-03: Source-Locked Renderer And Assets

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-02

## Outcome

Deliver a deterministic visual projection of the already-authorized normalized
state, with source-locked assets, finite geometry, stable identities, accurate
hit testing, high-DPI resize behavior, and nonblank real-topology evidence.

## Requirements And Scenarios

- FR-105-002, FR-105-006, FR-105-013, FR-105-018
- Re-validates SCN-105-001 and SCN-105-003 through the visual projection

```gherkin
Revalidation Case: SCN-105-001 Visual projection contains the same real bounded graph
  Given the bounded query foundation has settled real authorized nodes and stored edges
  When the Graph projection renders
  Then the rendered identity counts equal Outline and Table
  And non-background pixels and hit regions prove more than a shell border or loading glyph
  And no decorative or fabricated topology is drawn

Scenario: SCN-105-003 Deterministic rendering preserves prior geometry
  Given a settled visual graph and a successful bounded expansion
  When new authorized records merge
  Then existing coordinates remain within the declared stability tolerance
  And new finite coordinates and hit regions are deterministic across equivalent runs
```

## Renderer And Dependency Admission Gate

1. Inventory already permitted graph/rendering dependencies and source
   allowlists before implementation.
2. If a proven graph renderer already permitted by the repository satisfies
   deterministic preset coordinates, Canvas/CSP, offline packaging, hit testing,
   mobile, and privacy constraints, pin and use that exact locked version.
3. If no permitted library satisfies the contract and implementation proposes a
   third-party graph or physics package, route design reconciliation first,
   then perform an explicit reviewed source-allowlist and lockfile-strict install
   through `./smackerel.sh`; no CDN, mutable URL, implicit registry, or unreviewed
   transitive source is allowed.
4. The active design's native Canvas path may contain deterministic algebraic
   placement only. It must not contain a force solver. Any force-directed
   physics must come from a proven source-locked library; hand-rolled force
   physics is prohibited.

## Implementation Plan

1. Implement or adapt the design's pure renderer contract: visible graph derivation, deterministic layout, geometry, draw, and hit test over explicit inputs only.
2. Draw stored edge semantics without using color, thickness, or position as the sole meaning; keep accessible text in semantic projections.
3. Use `ResizeObserver` and explicit DPR transforms so CSS bounds, backing dimensions, draw coordinates, and pointer coordinates remain aligned.
4. Emit a content-free settled marker, node/edge counts, and layout hash only after a complete stable frame.
5. Package all renderer modules, fonts/icons, CSS, and worker assets as same-origin source-locked PWA assets; update service-worker cache version without caching Graph API responses.
6. Provide render-error containment that selects Outline/Table without a second graph fetch or blank-canvas success.

## Change Boundary

**Allowed:** renderer adapter/pure geometry modules, graph CSS/static assets,
service-worker static inventory, source-lock/manifest/lockfile surfaces required
by an approved dependency, renderer unit/E2E tests, renderer docs.  
**Excluded:** query/path truth, auth policy, graph records, general PWA shell,
deployment adapter, external asset origins, custom force physics.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-003-U | Unit | `unit` | SCN-105-003 | `web/pwa/tests/graph_state_reducer_test.go` - `SCN-105-003 expansion merge unit` | `./smackerel.sh test unit` | No |
| T105-003-I | Integration | `integration` | SCN-105-003 | `tests/integration/graph_explorer/expansion_state_test.go` - `SCN-105-003 expansion state integration` | `./smackerel.sh test integration` | Yes |
| T105-003-A | E2E API regression | `e2e-api` | SCN-105-003 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-003 bounded expansion API` | `./smackerel.sh test e2e` | Yes |
| T105-003-W | E2E UI regression | `e2e-ui` | SCN-105-003 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-003 expansion preserves orientation` | `./smackerel.sh test e2e-ui` | Yes |
| T105-03-ADMISSION | Supply-chain regression | `unit` | SCN-105-001 | `web/pwa/tests/graph_renderer_admission_test.go` - `Graph renderer dependency and assets are explicit source locked and contain no custom force solver` | `./smackerel.sh test unit` | No |
| T105-03-GEOMETRY | Unit | `unit` | SCN-105-003 | `web/pwa/tests/graph_layout_test.go` - `Deterministic finite layout and hit testing preserve prior geometry` | `./smackerel.sh test unit` | No |
| T105-03-ASSET | Integration | `integration` | SCN-105-001 | `tests/integration/graph_explorer/asset_contract_test.go` - `Explorer assets are same origin fresh and API responses remain network only` | `./smackerel.sh test integration` | Yes |
| T105-03-PIXEL | E2E UI regression | `e2e-ui` | SCN-105-001 | `web/pwa/tests/graph-explorer.spec.ts` - `Populated Graph pixels geometry and semantic IDs prove real nonblank topology` | `./smackerel.sh test e2e-ui` | Yes |
| T105-03-RESIZE | E2E UI regression | `e2e-ui` | SCN-105-003 | `web/pwa/tests/graph-explorer.spec.ts` - `Graph remains framed nonblank and hittable across resize and DPR changes` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] The renderer admission decision is explicit and source-locked; no CDN, implicit source, unreviewed graph package, or hand-rolled force physics exists.
- [ ] SCN-105-003 Deterministic rendering preserves prior geometry: Graph, Outline, and Table share exact authorized identity sets while Canvas geometry, pixels, resize, DPR, and hit testing remain deterministic.
- [ ] A populated graph cannot pass as blank, decorative, loading-only, or fake topology, and render failure preserves semantic projections.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-003-U passes with evidence in `report.md#t105-003-u`.
- [ ] T105-003-I passes with evidence in `report.md#t105-003-i`.
- [ ] T105-003-A passes with evidence in `report.md#t105-003-a`.
- [ ] T105-003-W passes without interception with evidence in `report.md#t105-003-w`.
- [ ] T105-03-ADMISSION passes with dependency/source evidence in `report.md#t105-03-admission`.
- [ ] T105-03-GEOMETRY passes with deterministic geometry evidence in `report.md#t105-03-geometry`.
- [ ] T105-03-ASSET passes with same-origin/cache behavior evidence in `report.md#t105-03-asset`.
- [ ] T105-03-PIXEL passes without interception using semantic plus pixel/geometry checks in `report.md#t105-03-pixel`.
- [ ] T105-03-RESIZE passes across desktop/mobile and two DPR values in `report.md#t105-03-resize`.

#### Build Quality Gate

- [ ] Scope tests, source allowlist/lockfile checks, asset freshness, CSP/service-worker checks, check, lint, format, docs, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked. The active design selects deterministic native
Canvas and rejects force simulation; any implementation request for a graph or
physics dependency requires design-owner reconciliation and source-lock review
before installation.