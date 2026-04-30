# Scopes: BUG-025-002 Knowledge synthesis E2E external URL extraction failure

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Make knowledge synthesis E2E deterministic

**Status:** Done
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

### Consumer Impact Sweep
- API client: no product API route or response contract changed; only the required E2E fixture stopped sending a non-owned URL.
- generated client: no generated client surface exists for this test fixture.
- navigation, breadcrumb, redirect, deep link: no first-party UI routing surface changed.
- stale-reference target: `tests/e2e/knowledge_synthesis_test.go` keeps `example.com/synthesis-e2e-test` only inside the regression guard that fails if the encoded required fixture depends on it again.

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
  - **Phase:** validate
  - **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/knowledge_synthesis_test.go`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** Regression quality guard reported `Adversarial signal detected in tests/e2e/knowledge_synthesis_test.go` and `REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)`. The focused E2E evidence above shows the guarded path still passes through the live stack.
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
- [x] Broader E2E regression suite passes
  - **Phase:** validate
  - **Command:** existing BUG-025-002 focused evidence review plus c6d2b26 broad E2E baseline evidence from `specs/039-recommendations-engine/report.md`
  - **Exit Code:** c6d2b26 broad baseline 0; not rerun during metadata-only closeout
  - **Claim Source:** interpreted
  - **Evidence:** report.md `### Validation Evidence` records the focused knowledge synthesis proof and the later baseline: `timeout 3600 ./smackerel.sh test e2e` exit 0, shell E2E `34 total, 34 passed, 0 failed`, and Go E2E packages passed.
  - **Interpretation:** The implementation-stage broad E2E timed out before certification, but the focused BUG-025-002 E2E had already passed without the external URL dependency. The later c6d2b26 full E2E baseline proves the broad suite no longer reports the knowledge synthesis external URL extraction failure.
- [x] Regression tests contain no silent-pass bailout patterns
  - **Phase:** validate
  - **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/knowledge_synthesis_test.go`
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** Regression quality guard scanned `tests/e2e/knowledge_synthesis_test.go`, detected the adversarial signal, and returned `REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)`.
- [x] Consumer impact sweep confirms zero stale first-party references remain
  - **Phase:** validate
  - **Command:** `git grep -n "example.com/synthesis-e2e-test" -- tests/e2e/knowledge_synthesis_test.go`
  - **Exit Code:** 0
  - **Claim Source:** interpreted
  - **Evidence:** `git grep` returned one line, `tests/e2e/knowledge_synthesis_test.go:32`, where the old external URL appears inside `assertDeterministicKnowledgeSynthesisFixture` as a regression-guard rejection string.
  - **Interpretation:** The required E2E fixture no longer sends the external URL as a success dependency; the remaining first-party reference is the guard that fails if the dependency returns.
- [x] Bug marked as Fixed in bug.md by the validation owner
  - **Phase:** validate
  - **Command:** validation closeout artifact edit
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** bug.md `## Status` now checks Reported, Confirmed, In Progress, Fixed, Verified, and Closed; state.json now records `status=done`, `certification.status=done`, `currentPhase=finalize`, and `currentScope=null` for BUG-025-002.
