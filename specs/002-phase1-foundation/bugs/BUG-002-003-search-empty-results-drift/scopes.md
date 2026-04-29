# Scopes: BUG-002-003 Search empty results drift

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore search empty-results live-stack contract

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-002-003 restore search empty-results contract
  Scenario: Unknown query returns honest empty result
    Given the disposable live stack contains artifacts unrelated to a deliberately unknown query
    When the user searches for that unknown query
    Then the search response contains zero results
    And the response includes the honest nothing-found message

  Scenario: Empty-results regression rejects leaked broad-suite artifacts
    Given prior E2E scenarios have created searchable artifacts
    When the empty-results scenario runs in the same broad suite
    Then unrelated artifacts are not returned as matches for the unknown query
```

### Implementation Plan
1. Reproduce the targeted `SCN-002-023` failure and record the unknown query, response body, and returned result identifiers.
2. Determine whether the results come from fixture leakage, query matching thresholds, fallback search behavior, or scenario/test drift.
3. Fix the first confirmed broken contract with a narrow change boundary.
4. Preserve strict empty-results assertions and known-query search coverage.
5. Re-run targeted search E2E and the broader E2E suite through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-002-003-01 | Unknown query returns zero results | e2e-api | `tests/e2e/test_search.sh` or `tests/e2e/test_search_empty.sh` | Unknown query returns count 0 and the honest empty message | BUG-002-003-SCN-001 |
| T-BUG-002-003-02 | Regression E2E: leaked artifacts rejected | e2e-api | `tests/e2e/test_search.sh` | Empty-results scenario rejects unrelated artifacts created earlier in the suite | BUG-002-003-SCN-002 |
| T-BUG-002-003-03 | Known search still works | e2e-api | `tests/e2e/test_search.sh` | Known search scenario still returns expected matching artifact | SCN-002-020 |
| T-BUG-002-003-04 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the search empty-results failure | BUG-002-003-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence. **Phase:** implement. **Claim Source:** interpreted/executed. Evidence: workflow supplied `test_search.sh SCN-002-023` expected 0 actual 5; source inspection confirmed unfiltered vector nearest-neighbor results had no raw confidence gate; added SCN-002-023 unit regressions failed red under `timeout 600 ./smackerel.sh test unit` before implementation.
- [x] Unknown-query response returns zero results and the honest empty message in the live stack. **Phase:** implement. **Claim Source:** executed. Evidence: post-fix broad E2E shell block reported `PASS: SCN-002-023: Empty results handled gracefully` in `test_search.sh` with message `I don't have anything about that yet`, and `PASS: SCN-002-023: Empty results return graceful message` in `test_search_empty.sh`.
- [x] Adversarial regression case proves broad-suite artifacts do not leak into the empty-results query. **Phase:** implement. **Claim Source:** executed. Evidence: `TestSCN002023_VectorSearchRejectsLowConfidenceUnfilteredResults` rejects a low raw similarity unfiltered match, and `TestSCN002023_VectorSearchUsesRawSimilarityBeforeAnnotationBoost` proves annotation boosts cannot promote weak raw semantic matches.
- [x] Pre-fix regression test fails for the five-result drift. **Phase:** implement. **Claim Source:** executed/interpreted. Evidence: the new SCN-002-023 confidence-gate regressions failed red before implementation because the vector confidence contract was absent. The exact routed five-result live failure was supplied by workflow context and was not reproduced by the local pre-edit broad E2E attempt.
- [x] Post-fix targeted search E2E regression passes. **Phase:** implement. **Claim Source:** executed. Evidence: the repo CLI has no single shell-script selector, so the search scripts were verified in `timeout 3600 ./smackerel.sh test e2e`; `test_search.sh`, `test_search_filters.sh`, and `test_search_empty.sh` passed post-fix.
- [x] Known-query search behavior remains green. **Phase:** implement. **Claim Source:** executed. Evidence: post-fix `test_search.sh` reported `Results for 'pricing strategy': 1`, first result `SaaS Pricing Strategy Guide`, and `PASS: SCN-002-020: Search returns results`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. **Phase:** implement. **Claim Source:** executed/interpreted. Evidence: existing scenario-specific shell E2E coverage `tests/e2e/test_search.sh` and `tests/e2e/test_search_empty.sh` both exercised SCN-002-023 post-fix; new unit regressions cover the vector confidence gate that caused the drift.
- [ ] Broader E2E regression suite passes. **Phase:** implement. **Claim Source:** executed. Evidence: broad `timeout 3600 ./smackerel.sh test e2e` exited 1; the shell summary reported unrelated failures in `test_persistence.sh`, `test_postgres_readiness_gate.sh`, `test_digest_telegram.sh`, and `test_topic_lifecycle.sh`, and Go E2E failed `TestE2E_DomainExtraction` plus `TestOperatorStatus_RecommendationProvidersEmptyByDefault`. **Uncertainty Declaration:** The search-specific bug evidence is green, but this broad-suite item remains unchecked because non-search failures prevent a clean suite-level claim.
- [x] Regression tests contain no silent-pass bailout patterns. **Phase:** implement. **Claim Source:** interpreted. Evidence: new unit regressions use direct `t.Fatalf`/`t.Fatal` assertions, and the existing shell search tests keep strict `e2e_assert_eq` result-count and message checks.
- [ ] Bug marked as Fixed in bug.md by the validation owner. **Phase:** implement. **Claim Source:** not-run. Evidence: `bubbles.implement` did not edit validation-owned certification fields or mark the bug fixed. **Uncertainty Declaration:** This item remains unchecked because fixed-status certification belongs to the validation owner after reviewing the implementation evidence and unresolved broad-suite failures.
