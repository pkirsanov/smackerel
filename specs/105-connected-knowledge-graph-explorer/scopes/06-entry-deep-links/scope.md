# SCOPE-06: Entry Points, Deep Links, And Restoration

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-05

## Outcome

Make Graph Explorer first-class from shared navigation and every supported
knowledge object. Topic, person, place, concept, entity, and artifact entries,
browser history, non-sensitive deep links, and per-user saved views all restore
through fresh authorization without leaking removed labels or graph payloads.

## Requirements And Scenarios

- FR-105-001, FR-105-011, FR-105-016, FR-105-017
- SCN-105-002, SCN-105-013

```gherkin
Scenario: SCN-105-002 Enter from a topic detail
  Given an authorized topic detail has related artifacts and people
  When the user chooses Explore connections
  Then the topic is freshly authorized as seed and focus
  And its stored immediate relationships are visible
  And browser Back returns to detail while Forward restores current authorized graph context

Scenario: SCN-105-013 Restore only current authorized state
  Given a deep link or saved view names seed, focus, filters, projection, layout, and path endpoints
  When the explorer restores it
  Then every identifier is re-authorized through fresh query and path operations
  And removed or unauthorized records are omitted without prior labels or existence hints
  And the remaining state stays coherent
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Entry matrix | One authorized object of each supported kind | Open Explore connections from topic/person/place/concept/entity/artifact | Correct typed seed/focus and same-origin detail return | e2e-ui |
| Shared navigation | Authenticated shell | Open Knowledge then Graph | First-class destination, active breadcrumb, no stale nav target | e2e-ui |
| Deep-link restore | Authorized IDs and one removed/denied ID | Open canonical URL | Fresh reads; denied ID/label absent; remaining state coherent | e2e-api / e2e-ui |
| Saved-view round trip | Scoped user; disposable preference store | Create, rename, reload, open, delete | Exact allowed identifier/preferences round trip with optimistic versioning | e2e-api / e2e-ui |

## Implementation Plan

1. Add Graph to server and PWA navigation, Knowledge local navigation, breadcrumbs, and manifest/shortcut only where the complete journey is available.
2. Add `Explore connections` to topic, person, place, concept/entity, and artifact detail/search surfaces using canonical typed IDs and safe return targets.
3. Parse and serialize the canonical Graph URL allowlist. Exclude labels, query text, graph payload, evidence, cursors, scope tokens, credentials, and viewport coordinates.
4. Restore seed, focus, filters, projection, layout, and path by performing fresh authorized server queries; never trust history labels or prior existence.
5. Implement per-user saved-view preference migration and CRUD with `knowledge-graph:views`, CSRF, context-derived actor, optimistic versioning, schema validation, and write/read round-trip behavior.
6. Ensure Back/Forward and detail return restore the invoker/focus without creating pan/zoom history noise.
7. Update service-worker static assets and all consumer references atomically; preserve existing Wiki/detail deep links.

## Consumer Impact Sweep

- server shell navigation and Knowledge local navigation;
- PWA navigation, breadcrumbs, redirects, manifest shortcuts, service worker, and safe return targets;
- Wiki topic/person/place/time, concept/entity, artifact, search, and detail launch links;
- Graph API and saved-view clients, validators, URL parser, browser history, and generated/static contracts;
- docs, configuration, CSP tests, route inventories, synthetic checks, unit/integration/E2E tests;
- stale-reference scans for old Knowledge inventories, missing Graph routes, obsolete query names, and direct label-bearing URLs.

## Shared Infrastructure Impact Sweep

- Saved-view migration uses disposable storage and a snapshot-free per-test user namespace.
- Independent canaries verify existing Wiki deep links, login/session return, shell navigation order, service-worker API exclusion, and non-Graph PWA routes.
- Rollback preserves nonempty saved-view rows for later compatible binaries; table drop is allowed only before any row exists and with explicit migration evidence.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-002-U | Unit | `unit` | SCN-105-002 | `web/pwa/tests/graph_entry_state_test.go` - `SCN-105-002 entry URL state unit` | `./smackerel.sh test unit` | No |
| T105-002-I | Integration | `integration` | SCN-105-002 | `tests/integration/graph_explorer/entry_restore_test.go` - `SCN-105-002 entry restore integration` | `./smackerel.sh test integration` | Yes |
| T105-002-A | E2E API regression | `e2e-api` | SCN-105-002 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-002 entry reauthorization API` | `./smackerel.sh test e2e` | Yes |
| T105-002-W | E2E UI regression | `e2e-ui` | SCN-105-002 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-002 topic entry back and forward` | `./smackerel.sh test e2e-ui` | Yes |
| T105-013-U | Unit | `unit` | SCN-105-013 | `web/pwa/tests/graph_restore_state_test.go` - `SCN-105-013 restore state unit` | `./smackerel.sh test unit` | No |
| T105-013-I | Integration | `integration` | SCN-105-013 | `tests/integration/graph_explorer/entry_restore_test.go` - `SCN-105-013 restore reauthorization integration` | `./smackerel.sh test integration` | Yes |
| T105-013-A | E2E API regression | `e2e-api` | SCN-105-013 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-013 restore authorization API` | `./smackerel.sh test e2e` | Yes |
| T105-013-W | E2E UI regression | `e2e-ui` | SCN-105-013 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-013 restore omits unauthorized state` | `./smackerel.sh test e2e-ui` | Yes |
| T105-06-ENTRY | E2E UI regression | `e2e-ui` | SCN-105-002 | `web/pwa/tests/graph-explorer.spec.ts` - `Topic person place concept entity and artifact entries seed the same explorer` | `./smackerel.sh test e2e-ui` | Yes |
| T105-06-SAVED | E2E API/UI round trip | `e2e-ui` | SCN-105-013 | `web/pwa/tests/graph-explorer.spec.ts` - `Saved view create rename reload open and delete round trips authorized preferences only` | `./smackerel.sh test e2e-ui` | Yes |
| T105-06-CONSUMERS | Consumer regression | `e2e-ui` | SCN-105-002, SCN-105-013 | `web/pwa/tests/wiki.spec.ts` - `Graph navigation breadcrumbs redirects deep links and Wiki consumers contain no stale target` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-002 Enter from a topic detail: shared navigation and every supported object surface enter one explorer with canonical typed seed/focus and coherent detail return through Back/Forward.
- [ ] SCN-105-013 Restore only current authorized state: deep links, history, and saved views contain allowed identifiers/preferences only and restore exclusively through current authorization without prior-label leakage.
- [ ] Consumer and shared-infrastructure sweeps prove zero stale references and preserve Wiki, auth/session, shell, service-worker, and rollback contracts.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-002-U passes with evidence in `report.md#t105-002-u`.
- [ ] T105-002-I passes with evidence in `report.md#t105-002-i`.
- [ ] T105-002-A passes with evidence in `report.md#t105-002-a`.
- [ ] T105-002-W passes without interception with evidence in `report.md#t105-002-w`.
- [ ] T105-013-U passes with evidence in `report.md#t105-013-u`.
- [ ] T105-013-I passes with evidence in `report.md#t105-013-i`.
- [ ] T105-013-A passes with evidence in `report.md#t105-013-a`.
- [ ] T105-013-W passes without interception and without prior-label leakage in `report.md#t105-013-w`.
- [ ] T105-06-ENTRY passes for topic/person/place/concept/entity/artifact entry in `report.md#t105-06-entry`.
- [ ] T105-06-SAVED passes write/read/version/delete round-trip checks in `report.md#t105-06-saved`.
- [ ] T105-06-CONSUMERS passes the complete consumer and stale-reference sweep in `report.md#t105-06-consumers`.

#### Build Quality Gate

- [ ] Scope tests, migration up/down validation, CSRF/auth/privacy checks, service-worker/CSP canaries, check, lint, format, docs, artifact lint, traceability, regression quality, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no navigation, saved-view, migration,
consumer, browser, or runtime validation was executed by the planning owner.