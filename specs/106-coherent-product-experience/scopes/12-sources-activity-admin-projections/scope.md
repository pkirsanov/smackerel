# SCOPE-106-12: Sources Photos Drive Models Activity And Admin Projections

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gates:** Each destination requires its exact current route, owner state/command contract, auth policy, and real owner journey evidence from SCOPE-106-02. An unproven destination remains unavailable or omitted by authorization.

## Outcome

Existing Connectors/Drive, Photos, Models, Notifications/Activity, and authorized Admin tools compose under route-free Sources and Settings/Admin groups with one shell, appearance, state, and feedback language. Domain owners retain every API, provider, setup, sync, photo, model, notification, and admin behavior.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-004 Optional capability is represented honestly
  Given an optional connector photo source or model provider is unconfigured disabled degraded or unsupported
  When its owner-ready surface composes under Sources or Admin
  Then the shell presents the exact permitted state and action without exposing secrets or advertising readiness

Scenario: SCN-106-010 Source and Activity mutations report authoritative outcomes
  Given an authorized user tests syncs connects reviews decides or changes an owner-managed record
  When the owner command returns pending persisted conflict refused partial or failed
  Then shared feedback mirrors that result and refreshes owner state without duplicate submission
```

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-014 | `UX-E2E-106-014 first-run source activation moves Needs setup to Available without rendering secrets` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-044 | `UX-E2E-106-044 Connectors setup tests and activates through real owner state with no secret disclosure` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-045 | `UX-E2E-106-045 disabled or unsupported Connector is Unavailable with no impossible action` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-046 | `UX-E2E-106-046 degraded Connector retains last verified state age limitation and Retry outcome` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-047 | `UX-E2E-106-047 Photos owner workflows retain setup search capture duplicate lifecycle and confirmed actions` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-048 | `UX-E2E-106-048 Activity shows no quiet badge and round-trips actionable decisions truthfully` | `web/pwa/tests/coherent_sources_activity.spec.ts` |
| UX-E2E-106-052 | `UX-E2E-106-052 Admin is omitted for ordinary users and complete but redacted for authorized operators` | `web/pwa/tests/coherent_sources_activity.spec.ts` |

## Implementation Plan

1. Keep Sources and Admin as route-free groups. Bind only exact observed child routes, including current connector/Drive, Photos, model-connection, notification, Settings, and admin tools from the catalog inventory.
2. Compose each owner surface with shared workspace/local navigation, state bands, rows/tables, inspectors, forms, mutation footer, evidence/provenance, responsive records, theme, focus, and announcements.
3. Preserve owner distinctions among Available, Needs setup, Degraded, Unavailable, loading, true empty, stale, auth, provider/schema/rate/network error, pending, and terminal command outcomes.
4. Omit operator controls and Admin navigation for unauthorized actors; 403 never becomes login, ready, empty, or existence disclosure.
5. Keep provider/connector/model/photo secrets write-only through owner boundaries; projections expose presence/status classes only and never values, lengths, hashes, mappings, raw payloads, or internal topology.
6. Activity shows a global badge only when current owner evidence says action is required. Quiet and suppressed history remain inspectable without unread/missed counts.
7. Run every owner regression independently before cross-surface composition; no shell test replaces owner setup/sync/photo/model/notification semantics.

## Consumer Impact Sweep

Trace exact child routes/APIs, server/PWA nav, local views, breadcrumbs, deep links, auth gates, forms, owner clients, service worker, status/health, stable hooks, docs, tests, metrics, and acceptance manifest. No generic parent route or provider action is invented.

## Change Boundary

**Allowed:** outer Sources/Activity/Admin composition, route-free grouping, shared visual/state/responsive adapters, cross-surface tests.

**Excluded:** connector/Drive/Photo/model/notification/admin domain logic, APIs, credentials, providers, migrations, fault controls, owner tests, foreign packets, spec 079, deployment, knb, CCManager, and readiness derivation.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-12-U | `unit` | `internal/web/coherent_sources_activity_test.go` | SCN-106-004, 010 | `TestSourcesActivityAdminAdaptersConsumeExactOwnerRoutesStatesAuthAndRedaction` | `./smackerel.sh test unit --go` | No |
| XP106-12-I | `integration` | `tests/integration/experience/sources_activity_composition_test.go` | SCN-106-004, 010 | `TestSourcesPhotosDriveModelsActivityAndAdminRemainOwnerTruthUnderSharedComposition` | `./smackerel.sh test integration` | Yes |
| XP106-12-A | `e2e-api` | `tests/e2e/sources_activity_composition_e2e_test.go` | SCN-106-004, 010 | `Sources Activity and Admin routes preserve owner auth schemas state and zero-secret projections` | `./smackerel.sh test e2e` | Yes |
| XP106-12-O | `functional` | owner evidence register and exact route inventory | SCN-106-004, 010 | `TestSourcesActivityCompositionRequiresCurrentOwnerEvidenceAndNoInventedParentOrAction` | `./smackerel.sh check` | No |
| UX-E2E-106-014 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-004 | `UX-E2E-106-014 first-run source activation moves Needs setup to Available without rendering secrets` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-044 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-010 | `UX-E2E-106-044 Connectors setup tests and activates through real owner state with no secret disclosure` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-045 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-004 | `UX-E2E-106-045 disabled or unsupported Connector is Unavailable with no impossible action` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-046 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-004 | `UX-E2E-106-046 degraded Connector retains last verified state age limitation and Retry outcome` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-047 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-010 | `UX-E2E-106-047 Photos owner workflows retain setup search capture duplicate lifecycle and confirmed actions` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-048 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-010 | `UX-E2E-106-048 Activity shows no quiet badge and round-trips actionable decisions truthfully` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-052 | `e2e-ui` | `web/pwa/tests/coherent_sources_activity.spec.ts` | SCN-106-004 | `UX-E2E-106-052 Admin is omitted for ordinary users and complete but redacted for authorized operators` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-004 Optional capability is represented honestly`: owner-reported unconfigured, disabled, degraded, or unsupported connector, photo, Drive, model, Activity, or Admin state renders only its exact permitted label and action, without secrets, product-outage language, or a ready daily promise.
- [ ] `SCN-106-010 Source and Activity mutations report authoritative outcomes`: setup, test, sync, review, and decision actions mirror owner pending, persisted, conflict, refused, partial, and failed outcomes, prevent duplicates, and refresh authoritative state.
- [ ] Sources, Photos, Drive, Models, Activity, and Admin compose only from exact owner-ready routes and outcomes with no secret, provider, or domain duplication.
- [ ] Route-free groups, authorization omission, owner regressions, consumer trace, and privacy boundaries remain complete.

#### Test Evidence - 11 Rows / 11 Items

- [ ] XP106-12-U passes with evidence in `report.md#xp106-12-u`.
- [ ] XP106-12-I passes with evidence in `report.md#xp106-12-i`.
- [ ] XP106-12-A passes with evidence in `report.md#xp106-12-a`.
- [ ] XP106-12-O passes with evidence in `report.md#xp106-12-o`.
- [ ] UX-E2E-106-014 passes with evidence in `report.md#ux-e2e-106-014`.
- [ ] UX-E2E-106-044 passes with evidence in `report.md#ux-e2e-106-044`.
- [ ] UX-E2E-106-045 passes with evidence in `report.md#ux-e2e-106-045`.
- [ ] UX-E2E-106-046 passes with evidence in `report.md#ux-e2e-106-046`.
- [ ] UX-E2E-106-047 passes with evidence in `report.md#ux-e2e-106-047`.
- [ ] UX-E2E-106-048 passes with evidence in `report.md#ux-e2e-106-048`.
- [ ] UX-E2E-106-052 passes with evidence in `report.md#ux-e2e-106-052`.

#### Build Quality Gate

- [ ] Owner suites, route/auth/privacy/secret scans, state exclusivity, responsive/a11y/theme, consumer trace, no-interception, check, lint, format, artifact lint, traceability, and directly affected integration documentation checks pass with zero warnings.
