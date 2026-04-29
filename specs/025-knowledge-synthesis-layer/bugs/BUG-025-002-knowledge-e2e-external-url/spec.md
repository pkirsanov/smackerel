# Feature: BUG-025-002 Knowledge synthesis E2E external URL extraction failure

## Problem Statement
Knowledge synthesis E2E should prove the product's internal capture and synthesis flow. A test fixture that reaches out to an external URL introduces non-determinism and can fail for reasons unrelated to synthesis correctness.

## Outcome Contract
**Intent:** Knowledge synthesis E2E uses deterministic, stack-owned input while still exercising the real capture, processing, synthesis, and stats code paths.
**Success Signal:** The E2E completes capture and stats verification without relying on a remote URL extraction result.
**Hard Constraints:** Required live-stack E2E must use real internal services and real API responses; no request interception, canned backend response, or silent skip is allowed.
**Failure Condition:** The test still requires a successful fetch/extraction from a non-owned external URL, or passes without proving synthesis activity.

## Goals
- Capture targeted red-stage evidence for the external URL extraction failure.
- Replace or isolate the non-owned URL dependency with deterministic stack-owned input.
- Preserve real capture and synthesis processing assertions.

## Non-Goals
- Disabling URL capture support in production.
- Replacing the live E2E with a mocked unit test.
- Seeding stats directly instead of exercising the capture/synthesis path.

## Requirements
- E2E fixture content must be deterministic and owned by the test stack.
- The regression must fail if the test again depends on remote URL extraction for success.
- The test must still assert a knowledge stats or synthesis signal produced by real code.
- External URL extraction errors must surface clearly when the product is asked to fetch remote content.

## User Scenarios (Gherkin)

```gherkin
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

## Acceptance Criteria
- Targeted pre-fix failure output captures the external URL extraction failure.
- Post-fix E2E does not require `https://example.com/synthesis-e2e-test` to be fetched successfully.
- The test still proves real capture/synthesis/stats behavior against the live stack.
