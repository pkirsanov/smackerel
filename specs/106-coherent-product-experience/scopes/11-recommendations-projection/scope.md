# SCOPE-106-11: Recommendations Projection

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-05
**External Entry Gate:** BUG-039-005 Scopes 01-08 complete owner evidence. Recommendation actions remain unavailable before that gate.

## Outcome

Recommendation request, watches, provenance, limitations, feedback, and operation state compose under Work using BUG-039's real provider-backed availability and execution outcomes. Spec 106 adds no provider, registry, health policy, request/watch persistence, ranking, or scheduler logic.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-005 Enabled capability with no working provider is not ready
  Given recommendations are enabled without a usable production provider for the exact category and operation
  When Work Recommendations renders
  Then availability is Unavailable or Needs setup from BUG-039 policy
  And request watch enable resume and refresh actions cannot create inert state

Scenario: SCN-106-010 Recommendation actions report authoritative outcome
  Given provider-backed operation eligibility and owner command contracts are current
  When a request or watch action runs
  Then shared pending and terminal presentation preserves no-match filtered-empty degraded failed refused and persisted distinctions
```

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-040 | `UX-E2E-106-040 healthy production provider yields Available sourced recommendation actions` | `web/pwa/tests/coherent_recommendations.spec.ts` |
| UX-E2E-106-041 | `UX-E2E-106-041 enabled zero-provider and fixture-only production states remain Unavailable` | `web/pwa/tests/coherent_recommendations.spec.ts` |
| UX-E2E-106-042 | `UX-E2E-106-042 no-match all-provider failure and partial-provider degradation remain distinct` | `web/pwa/tests/coherent_recommendations.spec.ts` |
| UX-E2E-106-043 | `UX-E2E-106-043 provider-backed watch lifecycle round-trips while unavailable actions write nothing` | `web/pwa/tests/coherent_recommendations.spec.ts` |

## Implementation Plan

1. Compose `/recommendations` and current watch routes under Work using BUG-039's `AvailabilitySnapshot`, outcome class, provider evidence, and command results.
2. Render Available, Needs setup, Degraded, and Unavailable from owner state only; no global flag, route, registry count, provider list length, or empty result implies readiness.
3. Show/enable request and watch actions only when the exact category/operation is eligible. Pause, silence, delete, and authorized reads remain available as BUG-039 permits.
4. Apply shared workspace header, local views, state band, evidence row, mutation feedback, flat rows, mobile records, controls, icons/tooltips, theme, focus, and announcements without changing owner DTOs or persistence.
5. Preserve provider provenance and safe limitations while excluding raw query, location, upstream error/body, credentials, and operator-only details.
6. Run all BUG-039 owner provider/migration/request/watch/UI/security/stress tests independently before shared-shell tests.

## Consumer Impact Sweep

Trace recommendation routes/APIs, watches, scheduler, provider compatibility, nav, breadcrumbs, deep links, owner selectors, state hooks, status, docs, tests, metrics, and acceptance manifests. Remove no owner field or compatibility path.

## Change Boundary

**Allowed:** outer Work/Recommendations composition, shared state/token/responsive adapters, focused cross-product tests.

**Excluded:** providers, registry, availability derivation, migrations, request/watch business writes, scheduler, ranking/consent/delivery, BUG-039 tests/artifacts, foreign packets, spec 079, deployment, knb, CCManager, and readiness claims.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-11-U | `unit` | `internal/web/coherent_recommendations_test.go` | SCN-106-005, 010 | `TestRecommendationsShellAdapterConsumesOwnerAvailabilityOutcomesAndActionsOnly` | `./smackerel.sh test unit --go` | No |
| XP106-11-I | `integration` | `tests/integration/experience/recommendations_composition_test.go` | SCN-106-005, 010 | `TestRecommendationCompositionPreservesProviderEvidenceActionGatesAndOwnerReadback` | `./smackerel.sh test integration` | Yes |
| XP106-11-A | `e2e-api` | `tests/e2e/recommendations_composition_e2e_test.go` | SCN-106-005, 010 | `Recommendations outer composition preserves BUG039 auth availability outcomes and zero-write refusal` | `./smackerel.sh test e2e` | Yes |
| XP106-11-O | `functional` | BUG-039 evidence register | SCN-106-005, 010 | `TestRecommendationCompositionRequiresAllEightBUG039ScopesCurrent` | `./smackerel.sh check` | No |
| UX-E2E-106-040 | `e2e-ui` | `web/pwa/tests/coherent_recommendations.spec.ts` | SCN-106-005 | `UX-E2E-106-040 healthy production provider yields Available sourced recommendation actions` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-041 | `e2e-ui` | `web/pwa/tests/coherent_recommendations.spec.ts` | SCN-106-005 | `UX-E2E-106-041 enabled zero-provider and fixture-only production states remain Unavailable` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-042 | `e2e-ui` | `web/pwa/tests/coherent_recommendations.spec.ts` | SCN-106-005 | `UX-E2E-106-042 no-match all-provider failure and partial-provider degradation remain distinct` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-043 | `e2e-ui` | `web/pwa/tests/coherent_recommendations.spec.ts` | SCN-106-010 | `UX-E2E-106-043 provider-backed watch lifecycle round-trips while unavailable actions write nothing` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-005 Enabled capability with no working provider is not ready`: zero usable production providers yields BUG-039-owned Unavailable or Needs setup, and request, watch enable, resume, and refresh actions cannot create inert rows from enablement or route presence.
- [ ] `SCN-106-010 Recommendation actions report authoritative outcome`: shared composition preserves owner pending, persisted, refused, no-match, filtered-empty, degraded, and failed outcomes with exact provider evidence and authoritative read-back.
- [ ] Owner regressions, privacy/security, compatibility, and consumer trace remain intact.

#### Test Evidence - 8 Rows / 8 Items

- [ ] XP106-11-U passes with evidence in `report.md#xp106-11-u`.
- [ ] XP106-11-I passes with evidence in `report.md#xp106-11-i`.
- [ ] XP106-11-A passes with evidence in `report.md#xp106-11-a`.
- [ ] XP106-11-O passes with evidence in `report.md#xp106-11-o`.
- [ ] UX-E2E-106-040 passes with evidence in `report.md#ux-e2e-106-040`.
- [ ] UX-E2E-106-041 passes with evidence in `report.md#ux-e2e-106-041`.
- [ ] UX-E2E-106-042 passes with evidence in `report.md#ux-e2e-106-042`.
- [ ] UX-E2E-106-043 passes with evidence in `report.md#ux-e2e-106-043`.

#### Build Quality Gate

- [ ] BUG-039 owner suites, provider/privacy/CSRF, zero-write refusal, consumer trace, responsive/a11y/theme, no-interception, check, lint, format, artifact lint, traceability, and directly affected integration documentation checks pass with zero warnings.
