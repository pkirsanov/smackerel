# Scope 03: Yelp Production Adapter

**Status:** Not Started
**Depends On:** 02
**Scope-Kind:** runtime-behavior

## Outcome

Yelp is an independently configurable production adapter whose results and failures can combine with Google Places without fabricating complete coverage.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-12 Yelp production adapter is independent and composes partial coverage
	Given Yelp is explicitly configured and can run alone or beside Google Places
	When real provider-compatible results and every closed failure class execute in both mixed directions
	Then Yelp facts and attribution remain production-sourced and useful mixed results retain participating and missing evidence
	And neither adapter depends on the other or fabricates complete coverage
```

## Implementation Plan

1. Implement Yelp through the same descriptor, health, fetch, error, and attribution contracts established by scopes 01-02.
2. Wire explicit fully validated Yelp declarations into the production registry without making either provider mandatory by name.
3. Normalize supported categories and attribution independently from Google-specific response shapes.
4. Verify mixed success/failure execution records participating and missing evidence deterministically.
5. Exercise real Yelp adapter protocol over a controlled provider-compatible external boundary server with no first-party interception.

## Consumer Impact Sweep

Verify the registry, inventory, engine, evidence ordering, status, metrics, and attribution render provider-neutral collections. No consumer may assume Google is the only production provider or erase one provider's failure when another succeeds.

## Change Boundary

Allowed: Yelp recommendation adapter, declaration wiring, multi-provider evidence tests. Excluded: availability policy implementation, watch behavior, UI composition, Google adapter internals except shared-contract fixes proven necessary, and unrelated source connectors.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC03-TP01 | Adapter unit | `unit` | `internal/recommendation/provider/yelp_test.go` | SCN-039-005-12 | `TestYelpNormalizesProductionFactsAndAttribution` | `./smackerel.sh test unit --go` | No |
| REC03-TP02 | Failure unit | `unit` | `internal/recommendation/provider/yelp_test.go` | SCN-039-005-12 | `TestYelpClassifiesSafeFailuresWithoutRawDetails` | `./smackerel.sh test unit --go` | No |
| REC03-TP03 | Multi-provider integration | `integration` | `tests/integration/recommendation_providers_test.go` | SCN-039-005-12 | Real Google/Yelp adapters execute against protocol-compatible boundary services; one succeeds while the other fails in both directions | `./smackerel.sh test integration` | Yes |
| REC03-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendations_providers_test.go` | SCN-039-005-12 | `TestRegressionOneProviderSuccessCannotEraseOtherProviderFailure` is red before typed evidence and green after repair | `./smackerel.sh test e2e` | Yes |
| REC03-TP05 | Provider independence E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go` | SCN-039-005-12 | `TestYelpOnlyAndMixedProviderExecutionRemainSourced` proves independent and partial execution on the live stack | `./smackerel.sh test e2e` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-12 Yelp production adapter is independent and composes partial coverage: Yelp-only and mixed outcomes retain sourced facts and honest limitations.
- [ ] Yelp satisfies the shared production adapter contract independently and never introduces a provider-specific consumer branch.
- [ ] Mixed provider outcomes retain deterministic participating/missing provenance and cannot claim complete availability.
- [ ] Provider-compatible validation executes both production adapters without internal provider fixtures.
- [ ] Consumer and change-boundary sweeps prove no Google-only assumption or collateral connector edits remain.

#### Test Evidence - 5 Rows / 5 Items

- [ ] REC03-TP01 adapter-unit evidence is recorded.
- [ ] REC03-TP02 failure/redaction-unit evidence is recorded.
- [ ] REC03-TP03 multi-provider integration evidence is recorded.
- [ ] REC03-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC03-TP05 provider-independence E2E API evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader provider checks, lint, format check, source-lock verification, artifact lint, traceability, documentation alignment, and zero-warning output pass with current-session evidence.