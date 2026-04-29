# Bug Fix Design: BUG-031-003

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 workflow context reports that `tests/e2e/capture_process_search_test.go::TestE2E_CaptureProcessSearch` times out while polling artifact detail for `processing_status`. Source inspection confirms the test captures text, polls `/api/artifact/{id}` for 60 seconds, and only proceeds to search after status is `processed` or `completed`.

### Root Cause
Unconfirmed at packetization time. The owner must capture red-stage output before edits. The likely investigation path is: capture response artifact ID -> artifact detail payload fields -> database artifact row status -> NATS processing message delivery -> ML sidecar result handling -> search indexing.

### Impact Analysis
- Affected components: capture API, artifact detail API, processing pipeline, ML sidecar readiness, search indexing, E2E harness sequencing.
- Affected data: disposable E2E artifacts only when run through the test stack.
- Affected users: delivery workflow and any user relying on capture-to-search freshness.

## Fix Design

### Solution Approach
Start with targeted reproduction that logs the final artifact detail response and the processing pipeline status source. Fix the first production contract that fails: either persist status transitions correctly, expose the stored status in artifact detail, repair NATS/ML completion handling, or adjust the E2E wait only if the spec-defined status vocabulary has changed.

### Alternative Approaches Considered
1. Increase the timeout only. Rejected unless profiling proves the pipeline is healthy but legitimately exceeds the current budget.
2. Search without waiting for processed status. Rejected because it weakens the specified SCN-LST-003 pipeline proof.

## Affected Files
- `tests/e2e/capture_process_search_test.go` for diagnostic and regression assertions
- Potential production surfaces under `internal/api`, `internal/pipeline`, `internal/db`, or `ml/app` depending on confirmed root cause
- E2E harness scripts only if startup sequencing is proven to hide pipeline readiness

## Regression Test Design
- Targeted E2E regression: `TestE2E_CaptureProcessSearch` must pass with real processed status.
- Adversarial regression: injected or observed empty status must fail the test with explicit diagnostics.
- Broader regression: `./smackerel.sh test e2e` after targeted repair.

## Ownership
- Owning feature/spec: `specs/031-live-stack-testing`
- Fix owner: `bubbles.implement` or `bubbles.devops` if harness readiness is confirmed as the cause
- Test owner: `bubbles.test`
- Validation owner: `bubbles.validate`
