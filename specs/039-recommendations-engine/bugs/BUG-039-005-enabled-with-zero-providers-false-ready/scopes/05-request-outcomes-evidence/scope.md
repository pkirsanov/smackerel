# Scope 05: Request Outcomes And Provider Evidence

**Status:** Not Started
**Depends On:** 04
**Scope-Kind:** runtime-behavior

## Outcome

Recommendation requests gate before persistence and preserve orthogonal availability, result outcome, safe error class, and provider evidence through PostgreSQL, API responses, explanations, and feedback.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-05 Healthy no-match remains valid
	Given at least one healthy production provider completes with zero facts
	When the request result is persisted and rendered
	Then outcome is no-match with successful provider evidence
	And it is not unavailable filtered-empty or failed

Scenario: SCN-039-005-08 Partial provider degradation is transparent
	Given one ready provider returns useful facts and another returns a typed failure
	When the request completes
	Then sourced results are retained with degraded availability
	And participating and missing provider evidence names the coverage limitation

Scenario: SCN-039-005-13 Request refusal and attempted failure preserve evidence boundaries
	Given availability refuses before execution or every attempted provider fails
	When the request API responds and persistence is inspected
	Then pre-execution refusal creates no request or trace while attempted failure retains typed redacted evidence
	And no secret query precise location personal data or raw upstream error escapes
```

## Implementation Plan

1. Parse enough validated intent to select category, obtain the exact request snapshot, and gate before request/trace persistence.
2. Execute only providers eligible in the unexpired snapshot and collect typed evidence for every attempt.
3. Separate results, healthy no-match, policy-filtered empty, ambiguous, refused, and failed outcomes in service and API contracts.
4. Add dual-read/new-write persistence for availability state/cause, outcome, safe error, and schema-versioned provider evidence; preserve legacy rows as unknown where proof is absent.
5. Make request, explanation, feedback, attribution, policy, and quality consumers use the persisted evidence chain without replacing ranking or consent rules.
6. Clear stale result state on every new terminal outcome and keep safe request fields available for retry.

## Consumer Impact Sweep

Update request API DTOs, reactive engine outcome mapping, store readers/writers, provider facts/candidate joins, explanation/Why projection, feedback flow, metrics, docs, and tests. Preserve existing IDs and compatibility fields while eliminating empty-list inference.

## Change Boundary

Allowed: recommendation request command/engine/store/API, request evidence migration usage, explanation/feedback projections, focused tests. Excluded: watch mutations, scheduler, provider protocols, UI layout beyond request outcome hooks, ranking formula, consent policy, and unrelated APIs.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC05-TP01 | Outcome unit | `unit` | `internal/api/recommendations_test.go` | SCN-039-005-05, SCN-039-005-08, SCN-039-005-13 | Typed HTTP/body mapping for no-match, filtered-empty, degraded, refusal, and attempted failure | `./smackerel.sh test unit --go` | No |
| REC05-TP02 | Engine integration | `integration` | `tests/integration/recommendations_test.go` | SCN-039-005-05, 08 | Real engine/store with protocol providers persists normalized facts and orthogonal outcomes in ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| REC05-TP03 | Persistence integration | `integration` | `tests/integration/recommendation_privacy_test.go` | SCN-039-005-13 | Pre-execution refusal writes zero request/trace rows; attempted all-provider failure writes typed redacted evidence | `./smackerel.sh test integration` | Yes |
| REC05-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go` | SCN-039-005-05 | `TestRegressionAllProviderFailuresCannotRenderAsNoMatch` is red before outcome separation and green after repair | `./smackerel.sh test e2e` | Yes |
| REC05-TP05 | Request outcome E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go` | SCN-039-005-05, 08 | `TestRequestDistinguishesNoMatchFilteredEmptyDegradedAndFailed` against the live stack | `./smackerel.sh test e2e` | Yes |
| REC05-TP06 | Existing journey regression | `e2e-api` | `tests/e2e/recommendations_full_regression_test.go` | SCN-039-005-13 | Why, feedback, constraints, consent, attribution, and confidence still use authoritative evidence | `./smackerel.sh test e2e` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-05 Healthy no-match remains valid: successful zero-fact evidence produces no-match and never an outage or filtered-empty state.
- [ ] SCN-039-005-08 Partial provider degradation is transparent: useful sourced results retain participating and missing coverage evidence.
- [ ] SCN-039-005-13 Request refusal and attempted failure preserve evidence boundaries: refusals create no business rows while attempted failures retain only typed redacted evidence.
- [ ] Request eligibility is checked before business persistence and refused requests create no request/trace identity.
- [ ] Availability, outcome, error, and provider evidence remain orthogonal and persisted truthfully for attempted execution.
- [ ] No-match, filtered-empty, degraded results, and typed failures are mutually distinguishable without privacy leakage.
- [ ] Request consumer and change-boundary sweeps preserve ranking, consent, Why, feedback, and compatibility contracts.

#### Test Evidence - 6 Rows / 6 Items

- [ ] REC05-TP01 outcome-unit evidence is recorded.
- [ ] REC05-TP02 engine-integration evidence is recorded.
- [ ] REC05-TP03 persistence/privacy integration evidence is recorded.
- [ ] REC05-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC05-TP05 request-outcome E2E API evidence is recorded.
- [ ] REC05-TP06 existing-journey E2E regression evidence is recorded.

#### Build Quality Gate

- [ ] Focused checks, broader recommendation API/integration/E2E, lint, format check, migration compatibility, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.