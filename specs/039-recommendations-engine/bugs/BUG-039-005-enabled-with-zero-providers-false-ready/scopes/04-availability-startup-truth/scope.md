# Scope 04: Availability And Startup Truth

**Status:** Not Started
**Depends On:** 03
**Scope-Kind:** runtime-behavior

## Outcome

One category- and operation-aware availability service owns recommendation readiness, required startup refusal, optional isolation, health freshness, safe causes, and bounded telemetry.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-01 Healthy provider makes capability ready
	Given a configured registered category-compatible production provider has fresh healthy evidence
	When availability is evaluated for the exact category and operation
	Then the operation is available or degraded only as coverage requires
	And the snapshot is bounded by category operation evaluated time validity and provider evidence

Scenario: SCN-039-005-02 Enabled zero-provider state is not ready
	Given recommendations are enabled with zero configured usable production providers
	When required startup and optional availability are evaluated
	Then required mode refuses startup and optional mode reports unavailable
	And no operation is eligible from enablement alone

Scenario: SCN-039-005-03 Unhealthy provider is degraded or unavailable
	Given relevant configured providers are unhealthy stale or adapter-missing
	When availability is evaluated for a category and operation
	Then all-unusable coverage is unavailable with a safe cause
	And mixed healthy and unusable coverage is degraded with participating and missing evidence

Scenario: SCN-039-005-10 Disabled providers do not dilute readiness
	Given one operator-selected provider is eligible and healthy while another declared provider is disabled or unconfigured
	When availability computes the readiness denominator for the requested category
	Then the disabled or unconfigured provider is excluded from the numerator and denominator
	And one eligible healthy provider remains sufficient for first readiness
```

## Implementation Plan

1. Build inventory by joining explicit declarations, typed registry descriptors, category support, and bounded runtime health without reading secrets.
2. Implement immutable snapshots, closed state/cause/count/evidence types, deterministic ordering, freshness expiry, and operation matching.
3. Enforce required startup invariants after production adapter construction and initial bounded health checks; preserve optional product startup.
4. Add the availability gate that rejects disabled, stale, mismatched, unavailable, or fixture-derived snapshots before dependent commands.
5. Persist safe current provider runtime projection and emit bounded availability/health/refusal metrics, logs, spans, and alerts.
6. Replace direct readiness decisions in startup and shared wiring with the availability service; leave request/watch consumers for their owning scopes.

## Shared Infrastructure Impact Sweep

- Downstream contracts: request commands, watches, scheduler, provider compatibility endpoint, web request/watch/status handlers, metrics, alerts, and product claims.
- Canary: an unrelated healthy core route remains available when optional recommendations have zero providers.
- Restore: switch consumers back only before semantic writes begin; optional unavailable state remains isolated and provider runtime projection is replaceable.

## Consumer Impact Sweep

Inventory every `RecommendationsEnabled`, `Registry.Len()`, direct provider-health call, and route-presence readiness decision. This scope replaces startup/wiring decisions; later scopes replace request/watch/UI projections through the same service.

## Change Boundary

Allowed: availability package, provider inventory/runtime projection, startup wiring, recommendation metrics/alerts, focused tests. Excluded: request outcome writes, watch writes, UI templates, ranking/policy algorithms, and provider adapter protocol implementations.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC04-TP01 | Availability unit | `unit` | `internal/recommendation/availability/service_test.go` | SCN-039-005-01, 02, 03 | Table-driven category/operation state, cause, freshness, fixture, and deterministic evidence cases | `./smackerel.sh test unit --go` | No |
| REC04-TP02 | Startup/config integration | `integration` | `tests/integration/recommendation_provider_registry_test.go` | SCN-039-005-02 | Required zero/fixture/wrong-category/stale provider startup refuses; optional stack reaches healthy core with unavailable Cards | `./smackerel.sh test integration` | Yes |
| REC04-TP03 | Availability API E2E | `e2e-api` | `tests/e2e/recommendations_providers_test.go` | SCN-039-005-01, 02, 03 | `TestAvailabilityContractDistinguishesAvailableDegradedAndUnavailable` on the real stack | `./smackerel.sh test e2e` | Yes |
| REC04-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go` | SCN-039-005-02 | `TestRegressionEnabledEmptyRegistryCannotBecomeReady` is red on the old flag/cardinality behavior and green after repair | `./smackerel.sh test e2e` | Yes |
| REC04-TP05 | Optional isolation E2E API | `e2e-api` | `tests/e2e/recommendations_full_regression_test.go` | SCN-039-005-02 | `TestOptionalRecommendationOutageDoesNotBecomeProductOutage` verifies unrelated authenticated core behavior remains available | `./smackerel.sh test e2e` | Yes |
| REC04-TP06 | Telemetry integration | `integration` | `tests/integration/recommendation_metrics_test.go` | SCN-039-005-02, 03 | Bounded state/cause/count metrics and redacted structured logs reflect actual snapshots | `./smackerel.sh test integration` | Yes |
| REC04-TP07 | Denominator integration | `integration` | `internal/recommendation/availability/service_test.go` | SCN-039-005-10 | `TestReadinessDenominatorExcludesDisabledUnconfiguredFixtureAndCategoryIrrelevantProviders` proves one eligible healthy provider suffices while unused declarations stay operator-only inventory | `./smackerel.sh test integration` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-01 Healthy provider makes capability ready: only fresh healthy category-compatible production coverage permits the exact action.
- [ ] SCN-039-005-02 Enabled zero-provider state is not ready: required startup refuses and optional capability is unavailable with no ready action.
- [ ] SCN-039-005-03 Unhealthy provider is degraded or unavailable: complete versus partial loss remains typed and evidence-backed.
- [ ] SCN-039-005-10 Disabled providers do not dilute readiness: disabled, unconfigured, fixture, and category-irrelevant declarations are excluded from numerator and denominator, and one eligible healthy provider suffices for first readiness.
- [ ] Availability is the sole startup/wiring readiness authority and separates enabled, configured, registered, class, category, health, and freshness.
- [ ] Required failure refuses startup while optional unavailability remains isolated from core product liveness.
- [ ] Safe deterministic snapshots and bounded telemetry distinguish complete, partial, and absent coverage.
- [ ] Shared-infrastructure canary, rollback, consumer, and change-boundary checks are complete.

#### Test Evidence - 7 Rows / 7 Items

- [ ] REC04-TP01 availability-unit evidence is recorded.
- [ ] REC04-TP02 startup/config integration evidence is recorded.
- [ ] REC04-TP03 availability E2E API evidence is recorded.
- [ ] REC04-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC04-TP05 optional-isolation E2E API evidence is recorded.
- [ ] REC04-TP06 telemetry-integration evidence is recorded.
- [ ] REC04-TP07 denominator-integration evidence is recorded.

#### Build Quality Gate

- [ ] Focused checks, startup canary, broader integration/E2E, lint, format check, artifact lint, traceability, docs/alerts alignment, and zero-warning output pass with current-session evidence.