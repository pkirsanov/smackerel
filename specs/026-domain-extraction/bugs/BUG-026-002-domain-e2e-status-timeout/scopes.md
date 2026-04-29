# Scopes: BUG-026-002 Domain E2E status timeout

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore domain extraction live-stack status proof

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-026-002 restore domain extraction E2E status proof
  Scenario: Recipe capture completes domain extraction in the live stack
    Given the disposable live stack is healthy
    When an E2E test captures recipe-like text
    Then artifact detail eventually reports processing_status as processed or completed
    And domain_extraction_status as completed
    And domain_data contains recipe structure

  Scenario: Domain extraction regression fails on empty statuses
    Given artifact detail returns empty processing or domain extraction status
    When the E2E polling loop evaluates the captured artifact
    Then the test fails with diagnostic output instead of treating the extraction as complete
```

### Implementation Plan
1. Reproduce the targeted domain E2E failure and record the last artifact detail payload.
2. Trace the artifact through capture, processing, domain extraction NATS subjects, ML sidecar handling, and persistence.
3. Fix the first failing production or harness contract with a narrow change boundary.
4. Preserve strict status and `domain_data` assertions.
5. Re-run targeted domain E2E and the broader E2E suite through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-026-002-01 | Domain extraction reaches completed status | e2e-api | `tests/e2e/domain_e2e_test.go` | Recipe artifact reaches processed/completed status, domain status completed, and domain data exists | BUG-026-002-SCN-001 |
| T-BUG-026-002-02 | Regression E2E: empty statuses fail loudly | e2e-api | `tests/e2e/domain_e2e_test.go` | Empty processing or domain status cannot silently pass | BUG-026-002-SCN-002 |
| T-BUG-026-002-03 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the domain status timeout | BUG-026-002-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction` exited 1 before the full fix. The focused run reached `processing=processed` while `domain_extraction_status` stayed empty and failed with `domain extraction not completed within 90s timeout -- last domain_status=`.
- [x] Captured recipe artifacts transition through processing and domain extraction statuses in the live stack
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** Focused green run captured artifact `01KQA4DMXN6CX7QW4VSF7Q1HKT` and logged `processing=processed domain=pending` followed by `processing=processed domain=completed`. Broad run captured artifact `01KQA5AD4QXMKGW5JRVDESB8N3` and logged the same pending-to-completed domain transition.
- [x] Artifact detail exposes domain status and domain data asserted by the E2E test
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `internal/api/capture.go` now uses the domain-aware artifact read path and returns `domain_extraction_status` plus `domain_data`; `internal/db/postgres.go` scans both fields in `GetArtifactWithDomain`; `internal/api/capture_test.go` asserts the status/data response path.
- [x] Pre-fix regression test fails for the domain status timeout
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** The unchanged `TestE2E_DomainExtraction` failed before the full fix with `domain extraction not completed within 90s timeout -- last domain_status=` after repeatedly observing `processing=processed domain=`.
- [x] Adversarial regression case exists for empty or missing processing/domain status
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** The live E2E poller only sets success when processing is `processed`/`completed`, domain status is exactly `completed`, and `domain_data` is non-empty; empty domain status produced the pre-fix failure above. Added focused adversarial unit regressions also protect the root causes: generic cooking-note text does not over-classify as recipe, broad ML types do not erase stored domain-specific types, and degraded domain fallback is enabled only by config.
- [x] Post-fix targeted domain E2E regression passes
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** `timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction` exited 0. The test logged `domain=completed`, recipe `domain_data` keys, search hit for the captured artifact, and `--- PASS: TestE2E_DomainExtraction (35.18s)`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` is the scenario-specific live-stack regression for both BUG-026-002 scenarios: it proves completed status/data/search for recipe capture and fails loudly on empty statuses. No request interception or assertion weakening was introduced.
- [ ] Broader E2E regression suite passes
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** `timeout 3600 ./smackerel.sh --env test test e2e` exited 1. The domain extraction test passed in the broad run (`--- PASS: TestE2E_DomainExtraction (11.11s)`) and shell E2E was 34/34, but the broad Go E2E suite still failed on `TestOperatorStatus_RecommendationProvidersEmptyByDefault` (`status page missing Recommendation Providers block`).
- [x] Regression tests contain no silent-pass bailout patterns
  - **Phase:** test
  - **Claim Source:** executed
  - **Evidence:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/domain_e2e_test.go` exited 0 with `REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)` and `Adversarial signal detected in tests/e2e/domain_e2e_test.go`.
- [ ] Bug marked as Fixed in bug.md by the validation owner
  - **Phase:** validate
  - **Claim Source:** not-run
  - **Evidence:** Validation-owned certification remains open because the broad E2E command still exits 1 on an unrelated operator status assertion.
