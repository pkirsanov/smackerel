# SCOPE-08: Privacy, Security, And Honest States

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-07

## Outcome

Make every graph state truthful and private. Branch failure retains useful
authorized state; auth loss clears all personal graph representations before
paint; first-use empty, filtered-empty, isolated, loading, partial, degraded,
limit-reached, unavailable, render-unavailable, not-found, and stale states are
exclusive and recoverable without fake topology.

## Requirements And Scenarios

- FR-105-002, FR-105-014, FR-105-015, FR-105-019, FR-105-020
- NFR-105-007, NFR-105-008
- SCN-105-004, SCN-105-011, SCN-105-012

```gherkin
Scenario: SCN-105-004 Expansion failure preserves the graph
  Given a useful authorized graph is already visible
  When one real expansion dependency returns a typed failure
  Then the prior graph, geometry, focus, viewport, filters, path, and semantic rows remain usable
  And only the failed branch exposes Retry
  And no empty or generic whole-view replacement appears

Scenario: SCN-105-011 Auth failure is not an empty graph
  Given personal labels, semantic rows, geometry, hit maps, and topology pixels are visible
  When the next real read rejects the session or graph scope
  Then authorized graph state is synchronously cleared before the auth view paints
  And no prior label, identity, reason, count, geometry, hit region, or non-background topology pixel remains
  And the state is re-authentication or access denied, never empty knowledge

Revalidation Case: SCN-105-012 First-use empty state is actionable
  Given every required authorized graph read succeeds with zero nodes
  When the explorer settles
  Then the state is true-empty with permitted Capture and source guidance
  And no sample node, fake edge, Retry, unavailable message, or nonblank topology assertion appears
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Branch failure | Useful loaded graph; one real dependency fault | Expand affected node, inspect Retry | Existing graph stays operable; failure is branch-local | e2e-ui |
| Session expiry | Personal graph visible | Expire session before expansion/read | DOM/a11y/canvas/history state cleared before auth view | e2e-api / e2e-ui |
| Scope denial | Valid session without graph scope | Open direct graph URL | No existence/label/count disclosure; safe return | e2e-ui |
| True empty | Successful zero-node disposable user | Open Graph | Actionable first-use; no sample or error state | e2e-api / e2e-ui |
| State matrix | Separate real fixtures | Exercise loading/partial/degraded/limit/unavailable/render/not-found/stale | Exclusive copy, actions, counts, and preserved verified data | e2e-ui |

## Implementation Plan

1. Implement one closed state model and response decoder; clients cannot infer empty from item count, 404, generic exception, or renderer absence.
2. Keep initial/read/path/restore operations independently cancellable and branch-local. Preserve verified data only while authorization remains valid and label limitations/observation time.
3. Implement synchronous `clearAuthorizedGraphState()` before unauthorized paint: abort controllers; clear nodes, edges, adjacency, expansion, path, focus, filters, inspector, semantic DOM, geometry/hit maps, canvas pixels, pending announcements, and sensitive URL IDs.
4. Distinguish first-use empty, filtered-empty, isolated, not-found, partial, degraded, limit-reached, unavailable, render-unavailable, unauthorized-session, unauthorized-scope, loading, stale, and complete in every projection.
5. Keep Graph API and saved-view responses private/no-store and outside service-worker, localStorage, sessionStorage, IndexedDB, CacheStorage, and browser history payloads.
6. Validate safe same-origin evidence/detail links and CSRF on saved-view mutations; reject actor/body identity, unsafe schemes, unknown telemetry fields, and graph content in client observations.
7. Emit content-free errors/logs/metrics/traces containing only operation class, bounds, counts, duration, completeness, and closed cause.

## Security And Source Privacy Sweep

- No labels, IDs, people/place names, artifact titles/content, search text,
  filters, reasons, evidence, cursor/scope tokens, auth material, or saved-view
  names may appear in logs, metrics, traces, client observations, URLs beyond
  allowed opaque identifiers, or durable client storage.
- Source metadata is allowlisted by node kind; person contacts, raw artifact
  content, precise locations, unsupported hospitality kinds, and unauthorized
  evidence are excluded or redacted without existence detail.
- Tests scan success and every failure class, not only nominal output.

## Change Boundary

**Allowed:** graph state/error decoder, privacy-clear reducer command, graph
cache headers/storage guards, auth/error UI, link validation, content-free
telemetry validation, security/integration/E2E tests/docs.  
**Excluded:** global auth redesign, graph mutation, query/path algorithm changes
except typed failure propagation, external telemetry, deploy adapter secrets.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-004-U | Unit | `unit` | SCN-105-004 | `web/pwa/tests/graph_state_reducer_test.go` - `SCN-105-004 branch failure unit` | `./smackerel.sh test unit` | No |
| T105-004-I | Integration | `integration` | SCN-105-004 | `tests/integration/graph_explorer/expansion_state_test.go` - `SCN-105-004 branch failure integration` | `./smackerel.sh test integration` | Yes |
| T105-004-A | E2E API regression | `e2e-api` | SCN-105-004 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-004 branch failure API` | `./smackerel.sh test e2e` | Yes |
| T105-004-W | E2E UI regression | `e2e-ui` | SCN-105-004 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-004 branch failure retains graph` | `./smackerel.sh test e2e-ui` | Yes |
| T105-011-U | Unit | `unit` | SCN-105-011 | `web/pwa/tests/graph_privacy_clear_test.go` - `SCN-105-011 privacy clear unit` | `./smackerel.sh test unit` | No |
| T105-011-I | Integration | `integration` | SCN-105-011 | `tests/integration/graph_explorer/auth_privacy_test.go` - `SCN-105-011 auth privacy integration` | `./smackerel.sh test integration` | Yes |
| T105-011-A | E2E API regression | `e2e-api` | SCN-105-011 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-011 auth failure API` | `./smackerel.sh test e2e` | Yes |
| T105-011-W | E2E UI regression | `e2e-ui` | SCN-105-011 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-011 auth clears DOM and pixels` | `./smackerel.sh test e2e-ui` | Yes |
| T105-08-PRIVACY | Security regression | `e2e-api` | SCN-105-011 | `tests/e2e/graph_explorer_privacy_e2e_test.go` - `Graph sources logs metrics traces storage and observations contain no private content or tokens` | `./smackerel.sh test e2e` | Yes |
| T105-08-AUTH | E2E UI security | `e2e-ui` | SCN-105-011 | `web/pwa/tests/graph-explorer.spec.ts` - `Expired session and denied scope clear prior labels semantics geometry and pixels before recovery` | `./smackerel.sh test e2e-ui` | Yes |
| T105-08-STATES | E2E UI regression | `e2e-ui` | SCN-105-004, SCN-105-012 | `web/pwa/tests/graph-explorer.spec.ts` - `Loading empty filtered isolated partial degraded limit unavailable render not-found and stale states stay exclusive` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-004 Expansion failure preserves the graph: a failed branch retains authorized graph, geometry, focus, viewport, filters, path, and semantic state with branch-local Retry.
- [ ] SCN-105-011 Auth failure is not an empty graph: auth/session loss clears every visual, semantic, geometric, history, pending, and pixel representation before recovery paints.
- [ ] Source allowlists, storage/cache controls, same-origin links, saved-view auth/CSRF, and content-free telemetry prevent graph/private data leakage.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-004-U passes with evidence in `report.md#t105-004-u`.
- [ ] T105-004-I passes with evidence in `report.md#t105-004-i`.
- [ ] T105-004-A passes with evidence in `report.md#t105-004-a`.
- [ ] T105-004-W passes without interception with evidence in `report.md#t105-004-w`.
- [ ] T105-011-U passes with evidence in `report.md#t105-011-u`.
- [ ] T105-011-I passes with evidence in `report.md#t105-011-i`.
- [ ] T105-011-A passes with evidence in `report.md#t105-011-a`.
- [ ] T105-011-W passes with DOM/accessibility/geometry/pixel clear evidence in `report.md#t105-011-w`.
- [ ] T105-08-PRIVACY passes value-safe source/log/metric/trace/storage scans in `report.md#t105-08-privacy`.
- [ ] T105-08-AUTH passes real session/scope privacy clearing in `report.md#t105-08-auth`.
- [ ] T105-08-STATES passes the complete visible closed-state matrix in `report.md#t105-08-states`.

#### Build Quality Gate

- [ ] Scope tests, auth/CSRF/IDOR/privacy/cache/CSP scans, no-interception and no-bailout checks, check, lint, format, docs, artifact lint, traceability, environment-pollution guard, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no security, privacy, honest-state,
browser, telemetry, or runtime validation was executed by the planning owner.