# Scope 07: Shared API And Accessible UI Projections

**Status:** Not Started
**Depends On:** 06
**Scope-Kind:** runtime-behavior

## Outcome

Availability API, compatibility provider API, request page, watches, and operator status render one authoritative contract with distinct accessible states, permitted actions, provenance, and authorization boundaries.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-07 Auth and privacy boundaries hold
	Given daily operator expired-session and unauthorized callers
	When availability providers request watch and status surfaces render
	Then each caller receives only its allowed safe projection
	And unauthorized DOM accessibility and API output contains no query watch provider identity health cause setup detail or secret

Scenario: SCN-039-005-09 Recommendation availability is accessible
	Given a keyboard or screen-reader user at desktop 320 pixels and 200 percent zoom
	When checking available needs-setup degraded unavailable no-match filtered-empty or typed error renders
	Then state reason provenance and permitted actions are perceivable and operable
	And no overlap horizontal scroll pointer-only color-only focus-loss or stale terminal state occurs
```

## Implementation Plan

1. Add the authenticated category/operation availability route and make the providers compatibility route project the same inventory rather than probe independently.
2. Feed immutable snapshots and persisted execution outcomes into request, watch, and operator status templates; remove flag/route/cardinality readiness inference.
3. Implement the shared availability header, coverage summary, eligibility gate, outcome region, provenance strip, and mutation feedback primitives from the spec.
4. Render withheld actions with adjacent reasons; retain safe form values and existing watches; clear stale outcomes on terminal transitions.
5. Enforce operator authorization for expanded inventory, session-bound CSRF for cookie mutations, strict CSP, safe external links, and no sensitive client storage.
6. Validate responsive reflow, keyboard/focus order, semantic status/alert behavior, reduced motion, forced colors, and light/dark/system themes.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Healthy and degraded requests | Real authenticated validate stack with protocol-compatible providers | Open Request, submit once, inspect result/Why | Available or Degraded, sourced result, limitation, persisted feedback | `e2e-ui` |
| Zero/all-unhealthy and direct watch route | Optional zero or typed provider failures | Open Request/Watches and stale direct editor route | Exact unavailable cause, actions withheld/refused, unchanged state | `e2e-ui` |
| No-match versus filtered-empty | Healthy empty response, then policy elimination | Submit each request | Mutually exclusive copy, provenance, and recovery action | `e2e-ui` |
| Authorization/privacy | Expired, daily, and operator sessions | Open request/watch/status/API views | Safe re-auth/denial; operator-only detail excluded elsewhere | `e2e-ui` |
| Responsive/assistive matrix | Desktop, 320px, 200% zoom, keyboard, screen reader, reduced motion, forced colors, each theme | Traverse and invoke permitted actions | Equivalent information/actions, stable focus, no overflow | `e2e-ui` |

## Consumer Impact Sweep

Update request/watch/status templates and handlers, route registration, provider compatibility clients, navigation/deep links, breadcrumbs, auth/CSRF consumers, CSS/data hooks, API docs, accessibility tests, and product claims. Stale-reference scans must find no UI/API readiness branch based only on enabled flag, route existence, or provider count.

## Change Boundary

Allowed: recommendation availability/provider API projections, recommendation request/watch/status web handlers/templates/styles, Card-independent shared UI primitives only when surgically reusable, focused API/browser/security tests. Excluded: shared app-shell rewrite, unrelated routes, provider protocol logic, Card Rewards, and any HTMX/auth bootstrap replacement.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC07-TP01 | Renderer/security unit | `unit` | `internal/web/recommendations_render_test.go` | SCN-039-005-05, 07, 09 | Closed state rendering, stale-markup replacement, operator redaction, semantic hooks, and CSRF mapping | `./smackerel.sh test unit --go` | No |
| REC07-TP02 | API authorization integration | `integration` | `tests/integration/recommendation_privacy_test.go` | SCN-039-005-07 | Daily/operator/expired/unauthorized projections and secret/query/provider-detail absence through real router | `./smackerel.sh test integration` | Yes |
| REC07-TP03 | Availability API E2E | `e2e-api` | `tests/e2e/recommendations_providers_test.go` | SCN-039-005-01 through 08 | `TestAvailabilityAndProviderCompatibilityRoutesShareOneSnapshot` | `./smackerel.sh test e2e` | Yes |
| REC07-TP04 | Adversarial Playwright | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts` | SCN-039-005-02, 05 | `SCN-039-005-02 Regression: enabled zero-provider UI never mounts ready actions` is red before repair and uses no first-party interception | `./smackerel.sh test e2e-ui` | Yes |
| REC07-TP05 | State matrix Playwright | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts` | SCN-039-005-01, 03, 05, 08 | `SCN-039-005-05 Regression: no-match filtered-empty and typed failures remain exclusive` on live stack | `./smackerel.sh test e2e-ui` | Yes |
| REC07-TP06 | A11y/responsive Playwright | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts` | SCN-039-005-07, 09 | `SCN-039-005-09 Regression: recommendation states remain operable at 320px and 200 percent zoom` with keyboard, themes, reduced motion, and forced colors | `./smackerel.sh test e2e-ui` | Yes |
| REC07-TP07 | Browser authenticity scan | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts`, `web/pwa/tests/recommendation_watches_readiness.spec.ts` | SCN-039-005-01 through 09 | Regression-quality guard and source scan prove no `page.route`, `context.route`, interception, bailout return, or optional assertion | `./smackerel.sh test e2e-ui` | Yes |
| REC07-TP08 | Disabled-provider projection Playwright | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts` | SCN-039-005-10 | `SCN-039-005-10 Regression: disabled providers stay operator-only setup inventory and never dilute daily-user readiness` on the live stack with no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-07 Auth and privacy boundaries hold: daily/operator/session projections enforce authorization and omit all sensitive or inaccessible recommendation detail.
- [ ] SCN-039-005-09 Recommendation availability is accessible: every state and permitted action remains perceivable, operable, responsive, and mutually exclusive.
- [ ] API, request, watch, and status consumers render one availability/evidence contract and preserve compatibility paths.
- [ ] Every readiness, outcome, mutation, auth, and recovery state is mutually exclusive, privacy-safe, responsive, and accessible.
- [ ] Cookie mutations have explicit CSRF protection; strict CSP, safe links, and no sensitive client storage remain intact.
- [ ] Consumer and change-boundary sweeps show zero stale readiness inference, orphan link, or collateral shared-shell/auth rewrite.

#### Test Evidence - 8 Rows / 8 Items

- [ ] REC07-TP01 renderer/security-unit evidence is recorded.
- [ ] REC07-TP02 API-authorization integration evidence is recorded.
- [ ] REC07-TP03 shared-snapshot E2E API evidence is recorded.
- [ ] REC07-TP04 adversarial red-to-green Playwright evidence is recorded.
- [ ] REC07-TP05 state-matrix live Playwright evidence is recorded.
- [ ] REC07-TP06 accessibility/responsive live Playwright evidence is recorded.
- [ ] REC07-TP07 no-interception/no-bailout browser-authenticity evidence is recorded.
- [ ] REC07-TP08 disabled-provider projection live Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused checks, complete recommendation Playwright/API regressions, lint, format check, CSP/CSRF scans, bundle freshness, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.