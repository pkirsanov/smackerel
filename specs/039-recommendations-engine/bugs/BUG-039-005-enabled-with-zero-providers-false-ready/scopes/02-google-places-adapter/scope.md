# Scope 02: Google Places Production Adapter

**Status:** Not Started
**Depends On:** 01
**Scope-Kind:** runtime-behavior

## Outcome

The Google Places declaration can construct a real production-class adapter whose bounded health, fetch, attribution, no-match, and typed failures are verified against a protocol-compatible external test service.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-11 Google Places production adapter honors the provider contract
	Given Google Places is explicitly configured and the protocol-compatible provider can return results, empty, authentication, quota, timeout, malformed, and provider-error outcomes
	When production health and fetch execute
	Then normalized facts, healthy empty evidence, attribution, and closed safe failures are preserved
	And the real production adapter performs the external protocol exchange without exposing raw or sensitive details
```

## Implementation Plan

1. Implement the production adapter through the existing `provider.Provider` contract using explicit Google SST declarations and bounded timeouts.
2. Decode and validate the external response into existing normalized facts and attribution without exposing raw payloads.
3. Implement bounded health and failure classification for authentication, quota, timeout, malformed response, and generic provider error.
4. Wire only explicitly enabled, fully validated Google configuration into the production registry.
5. Exercise the real adapter over HTTP against a protocol-compatible external boundary server; do not replace the adapter or internal registry with a fixture.

## Consumer Impact Sweep

Confirm provider facts, attribution links, health state, metrics labels, logs, request execution, and operator inventory consume the descriptor and safe error types. No API/web consumer branches directly on a Google concrete type.

## Change Boundary

Allowed: Google recommendation adapter, provider declaration wiring, focused adapter/registry tests. Excluded: Yelp adapter, availability decisions, request/watch persistence semantics, UI templates, unrelated connectors, and deployment configuration.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC02-TP01 | Adapter unit | `unit` | `internal/recommendation/provider/google_places_test.go` | SCN-039-005-11 | `TestGooglePlacesNormalizesAttributedFactsAndHealthyEmpty` | `./smackerel.sh test unit --go` | No |
| REC02-TP02 | Failure unit | `unit` | `internal/recommendation/provider/google_places_test.go` | SCN-039-005-11 | `TestGooglePlacesClassifiesSafeFailuresAndRedactsPayloads` | `./smackerel.sh test unit --go` | No |
| REC02-TP03 | Protocol integration | `integration` | `tests/integration/recommendation_providers_test.go` | SCN-039-005-11 | Real production adapter performs provider-compatible health/fetch protocol against the controlled external boundary server | `./smackerel.sh test integration` | Yes |
| REC02-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendations_providers_test.go` | SCN-039-005-11 | `TestRegressionGoogleConfiguredWithoutAdapterCannotReportReady` is red before wiring and green with the real adapter | `./smackerel.sh test e2e` | Yes |
| REC02-TP05 | Attribution E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go` | SCN-039-005-11 | `TestGoogleProductionAdapterReturnsSourcedResultAndHealthyNoMatch` uses the live stack and protocol service | `./smackerel.sh test e2e` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-11 Google Places production adapter honors the provider contract: real protocol results, healthy empty evidence, attribution, and every safe failure class are verified.
- [ ] Google Places is a production-class adapter with explicit categories, health, bounded protocol behavior, attribution, and safe typed failures.
- [ ] Healthy empty responses retain successful provider evidence; malformed/upstream failures cannot masquerade as no-match.
- [ ] No fixture provider or internal adapter mock participates in production-compatible live validation.
- [ ] Consumer and change-boundary sweeps show no provider-specific readiness branch or collateral connector change.

#### Test Evidence - 5 Rows / 5 Items

- [ ] REC02-TP01 adapter-unit evidence is recorded.
- [ ] REC02-TP02 failure/redaction-unit evidence is recorded.
- [ ] REC02-TP03 protocol-integration evidence is recorded.
- [ ] REC02-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC02-TP05 attribution/no-match E2E API evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader provider checks, lint, format check, source-lock verification, artifact lint, traceability, documentation alignment, and zero-warning output pass with current-session evidence.