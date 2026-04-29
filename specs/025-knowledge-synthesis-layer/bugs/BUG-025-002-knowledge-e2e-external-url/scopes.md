# Scopes: BUG-025-002 Knowledge synthesis E2E external URL extraction failure

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Make knowledge synthesis E2E deterministic

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-025-002 keep knowledge synthesis E2E deterministic
  Scenario: Knowledge synthesis E2E uses deterministic stack-owned input
    Given the disposable live stack is healthy
    When the knowledge synthesis E2E captures deterministic article content
    Then capture and synthesis processing complete through real services
    And knowledge stats reflect the processing outcome

  Scenario: Knowledge synthesis regression fails on non-owned external URL dependency
    Given a required E2E fixture points at a non-owned external URL
    When the test requires that remote extraction for success
    Then the regression fails and routes the fixture back to deterministic stack-owned input
```

### Implementation Plan
1. Capture targeted red-stage evidence for the current external URL extraction failure.
2. Replace the required E2E fixture with deterministic stack-owned input, or serve URL content from a fixture owned by the disposable stack.
3. Preserve real capture/synthesis/stats assertions.
4. Add a regression guard against mandatory non-owned external URL dependencies in the knowledge synthesis E2E.
5. Re-run targeted knowledge synthesis E2E and broad E2E through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-025-002-01 | Knowledge synthesis round trip uses deterministic input | e2e-api | `tests/e2e/knowledge_synthesis_test.go` | E2E captures stack-owned deterministic content and observes synthesis/stats signal | BUG-025-002-SCN-001 |
| T-BUG-025-002-02 | Regression E2E: no mandatory external URL fixture | e2e-api | `tests/e2e/knowledge_synthesis_test.go` or quality guard owned by test phase | Fails if the required knowledge synthesis E2E depends on a non-owned external URL for success | BUG-025-002-SCN-002 |
| T-BUG-025-002-03 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports knowledge synthesis external URL extraction failure | BUG-025-002-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence
  - **Phase:** implement
  - **Command:** `./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 1
  - **Claim Source:** executed
  - **Evidence:** Pre-fix targeted E2E failed before source changes with `capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}`. Source inspection confirmed `/api/capture` processing selects URL extraction before text extraction when both fields are present, making the non-owned URL mandatory rather than fallback content.
- [x] Knowledge synthesis E2E uses deterministic stack-owned input for required success path
  - **Phase:** implement
  - **Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** `TestKnowledgeSynthesis_PipelineRoundTrip` now captures deterministic text-only content through real `/api/capture`, waits for real artifact processing, and observes knowledge synthesis stats increasing after that capture. Focused output included `capture response: 200`, `synthesis stats: completed=0 pending=1 failed=0 total=1`, and `--- PASS: TestKnowledgeSynthesis_PipelineRoundTrip (34.24s)`.
- [x] Required E2E does not depend on successful remote extraction from `https://example.com/synthesis-e2e-test`
  - **Phase:** implement
  - **Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** The required fixture now contains only `text` and `context`; the regression guard fails if the encoded request contains `http://`, `https://`, or `example.com/synthesis-e2e-test`. The focused E2E passed through the live stack without URL extraction.
- [x] Pre-fix regression test fails for the external URL dependency
  - **Phase:** implement
  - **Command:** `./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 1
  - **Claim Source:** executed
  - **Evidence:** The pre-fix required regression path failed on the external fixture with `EXTRACTION_FAILED` and `HTTP 404 fetching https://example.com/synthesis-e2e-test`, proving the old test depended on successful remote extraction.
- [x] Adversarial regression case exists for non-owned external URL fixture dependency
  - **Phase:** implement
  - **Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 0
  - **Claim Source:** interpreted
  - **Evidence:** `assertDeterministicKnowledgeSynthesisFixture` is part of the focused E2E and fails the test if the required fixture includes a `url` field or encoded `http://`, `https://`, or `example.com/synthesis-e2e-test` content.
- [x] Post-fix targeted knowledge synthesis E2E regression passes
  - **Phase:** implement
  - **Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** Focused output: `go-e2e: applying -run selector: TestKnowledgeSynthesis_PipelineRoundTrip`; `--- PASS: TestKnowledgeSynthesis_PipelineRoundTrip (34.24s)`; `ok github.com/smackerel/smackerel/tests/e2e 34.249s`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - **Phase:** implement
  - **Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** The focused E2E covers BUG-025-002-SCN-001 with deterministic live-stack capture/process/stats verification and BUG-025-002-SCN-002 with the external-URL guard in the same required regression path.
- [ ] Broader E2E regression suite passes
  - **Phase:** implement
  - **Command:** `./smackerel.sh test e2e`
  - **Exit Code:** 124
  - **Claim Source:** executed
  - **Uncertainty Declaration:** The broad run timed out, so a full broad-suite pass is not proven. Visible shell scenarios through IMAP sync passed, and no recurrence of the knowledge synthesis external URL failure appeared in the captured output, but the command did not complete successfully.
- [x] Regression tests contain no silent-pass bailout patterns
  - **Phase:** implement
  - **Command:** source review of `tests/e2e/knowledge_synthesis_test.go`
  - **Exit Code:** not-run
  - **Claim Source:** interpreted
  - **Evidence:** The test uses fatal assertions for fixture drift, capture failures, artifact processing failures, stats parsing failures, and missing stats increase. It does not contain login/url bailout returns, skipped assertions, request interception, or external network dependency.
- [ ] Bug marked as Fixed in bug.md by the validation owner
  - **Phase:** implement
  - **Command:** none
  - **Exit Code:** not-run
  - **Claim Source:** not-run
  - **Uncertainty Declaration:** This item is validation-owned and was not modified by `bubbles.implement`; route remains open for validation ownership after the unresolved broad-suite timeout is addressed.
