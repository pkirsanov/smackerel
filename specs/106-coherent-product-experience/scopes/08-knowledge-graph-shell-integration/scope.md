# SCOPE-106-08: Knowledge And Graph Shell Integration

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gate:** BUG-080-001 must be complete before spec 105; spec 105 must then be complete before this scope starts.

## Outcome

Knowledge Browse, current Wiki deep links, and spec 105's completed Graph Explorer compose as one workspace under the shared shell. Browse remains usable when Graph is unavailable; spec 106 adds no graph query, renderer, path, saved-view, privacy, scale, or API behavior.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-003 Knowledge and Graph are discoverable only when ready
  Given BUG-080 activation and spec 105 owner evidence are current
  When the user opens Knowledge Browse or an existing Wiki deep link
  Then Knowledge is the active parent and Graph is an available local view
  When either owner contract is unavailable
  Then Browse remains usable and Graph is labeled Unavailable without a missing link empty graph or fabricated topology
```

## Ownership Boundary

BUG-080 owns Graph activation, route manifest, authorized family reads, typed outcomes, synthetic, and readiness facts. Spec 105 owns Graph query/path contracts, deterministic Canvas, Outline/Table parity, interactions, deep links, saved views, pixels, privacy clear, scale, and Graph acceptance. Spec 106 owns only parent/local navigation, shell/theme/layout tracks, workspace header, shared state presentation, and cross-product context.

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-034 | `UX-E2E-106-034 Knowledge browse filters real projections and Explore opens the spec 105 seed` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` |
| UX-E2E-106-035 | `UX-E2E-106-035 Graph missing or disabled stays Unavailable while Knowledge Browse remains truthful` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` |
| UX-E2E-106-036 | `UX-E2E-106-036 shared shell preserves the complete spec 105 interaction semantic pixel privacy and scale contract` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` |

## Implementation Plan

1. Compose `/knowledge`, existing Wiki routes, and `/knowledge/graph` into one Knowledge parent/local-view projection only after exact owner route/evidence gates close.
2. Preserve every Wiki route, detail launch, Graph deep link, Back/Forward state, saved-view identity, and spec 105 stable hook. Do not add a parallel Browse or Graph API.
3. Apply shared workspace header, local switcher, shell dimensions, theme/density tokens, state band, evidence row, and mobile Views menu around owner components.
4. Keep Browse and Graph availability independent. Graph unavailable never changes Browse into 404/empty, and Browse rows never synthesize graph nodes or edges.
5. Reuse spec 105's desktop/tablet/mobile/keyboard/screen-reader/reduced-motion/forced-colors/DPR/pixel tests as owner evidence; add only shell integration assertions.
6. Preserve synchronous graph privacy clear and prevent shell breadcrumbs, accessible names, history, screenshots, or projection hooks from retaining private labels after auth loss.

## Consumer Impact Sweep

Trace server/PWA nav, Knowledge local nav, Wiki routes, Explore links, breadcrumbs, redirects, history, saved views, manifest/service worker, Graph assets, stable hooks, docs, tests, and acceptance manifest. Existing IDs/routes remain owner-defined.

## Change Boundary

**Allowed:** Knowledge/Graph shell placement, parent/local active state, shared workspace/tokens/layout, integration tests.

**Excluded:** BUG-080/spec105 code/tests/artifacts, graph APIs/SQL/state/renderer/physics/path/saved views/privacy/scale, auth, foreign packets, spec 079, deployment, knb, CCManager, and readiness derivation.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-08-U | `unit` | `internal/web/coherent_knowledge_graph_test.go` | SCN-106-003 | `TestKnowledgeGraphShellAdapterOwnsNoGraphTruthAndPreservesParentLocalState` | `./smackerel.sh test unit --go` | No |
| XP106-08-I | `integration` | `tests/integration/experience/knowledge_graph_shell_test.go` | SCN-106-003 | `TestKnowledgeBrowseAndSpec105ExplorerShareShellWithoutSharingFailureOrStateAuthority` | `./smackerel.sh test integration` | Yes |
| XP106-08-A | `e2e-api` | `tests/e2e/knowledge_graph_shell_e2e_test.go` | SCN-106-003 | `Knowledge and Graph routes preserve BUG080 and spec105 auth state and completeness contracts` | `./smackerel.sh test e2e` | Yes |
| XP106-08-O | `functional` | BUG-080 and spec105 evidence register | SCN-106-003 | `TestKnowledgeGraphShellRequiresCurrentIndependentBUG080AndSpec105Evidence` | `./smackerel.sh check` | No |
| UX-E2E-106-034 | `e2e-ui` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` | SCN-106-003 | `UX-E2E-106-034 Knowledge browse filters real projections and Explore opens the spec 105 seed` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-035 | `e2e-ui` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` | SCN-106-003 | `UX-E2E-106-035 Graph missing or disabled stays Unavailable while Knowledge Browse remains truthful` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-036 | `e2e-ui` | `web/pwa/tests/coherent_knowledge_graph.spec.ts` | SCN-106-003 | `UX-E2E-106-036 shared shell preserves the complete spec 105 interaction semantic pixel privacy and scale contract` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-003 Knowledge and Graph are discoverable only when ready`: Knowledge is the active parent, Graph is a local view only after BUG-080 then spec 105 evidence is current, and owner unavailability leaves Browse usable with Graph labeled Unavailable rather than a missing link, empty graph, or fabricated topology.
- [ ] Knowledge Browse, Wiki, and Graph compose under one parent/local shell without duplicating or weakening BUG-080/spec105 ownership.
- [ ] Route, history, saved-view, shell/theme, privacy, pixel, semantic, responsive, and scale owner regressions remain intact.

#### Test Evidence - 7 Rows / 7 Items

- [ ] XP106-08-U passes with evidence in `report.md#xp106-08-u`.
- [ ] XP106-08-I passes with evidence in `report.md#xp106-08-i`.
- [ ] XP106-08-A passes with evidence in `report.md#xp106-08-a`.
- [ ] XP106-08-O passes with evidence in `report.md#xp106-08-o`.
- [ ] UX-E2E-106-034 passes with evidence in `report.md#ux-e2e-106-034`.
- [ ] UX-E2E-106-035 passes with evidence in `report.md#ux-e2e-106-035`.
- [ ] UX-E2E-106-036 passes with evidence in `report.md#ux-e2e-106-036`.

#### Build Quality Gate

- [ ] Owner suites, route/deep-link consumer trace, privacy pixels, semantic parity, no-interception, responsive/theme/a11y, source locking, check, lint, format, artifact lint, traceability, and directly affected user/testing documentation checks pass with zero warnings.
