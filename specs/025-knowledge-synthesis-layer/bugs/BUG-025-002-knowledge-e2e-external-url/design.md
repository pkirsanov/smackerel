# Bug Fix Design: BUG-025-002

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 workflow context reports the knowledge synthesis E2E failing on external URL extraction. Source inspection confirms `tests/e2e/knowledge_synthesis_test.go` sends a capture body containing `https://example.com/synthesis-e2e-test` and text content, then checks knowledge stats.

### Root Cause
The E2E fixture depends on a non-owned external URL extraction path. That dependency is unsuitable for a deterministic disposable-stack E2E gate because network availability, remote content behavior, and extraction semantics can fail independently of knowledge synthesis correctness.

### Impact Analysis
- Affected components: knowledge synthesis E2E fixture, capture API URL extraction path, broad E2E suite.
- Affected data: disposable E2E capture artifacts.
- Affected users: delivery workflow and operator confidence in knowledge synthesis health.

## Fix Design

### Solution Approach
Use deterministic stack-owned content for the required synthesis E2E. If URL capture behavior must be exercised, provide a local fixture served by the test stack or create a separate test with explicit ownership and fail-loud preconditions. Keep the synthesis E2E focused on real internal capture, processing, synthesis, and stats behavior.

### Alternative Approaches Considered
1. Keep the external URL and increase retry budget. Rejected because it preserves the non-owned dependency.
2. Remove the knowledge synthesis E2E. Rejected because persistent scenario-specific E2E coverage is required.
3. Mock URL extraction. Rejected for live-stack E2E because request interception or canned internal responses would violate live-test authenticity.

## Affected Files
- `tests/e2e/knowledge_synthesis_test.go`
- Potential local fixture or test-stack route if URL behavior remains part of the scenario
- Capture/extraction production code only if targeted red-stage evidence shows product behavior is wrong rather than fixture design

## Regression Test Design
- Targeted E2E regression: knowledge synthesis pipeline round trip uses deterministic stack-owned input and asserts real stats/synthesis signal.
- Adversarial regression: the required E2E fails if it reintroduces a mandatory non-owned external URL dependency.
- Broader regression: `./smackerel.sh test e2e` after targeted repair.

## Ownership
- Owning feature/spec: `specs/025-knowledge-synthesis-layer`
- Fix owner: `bubbles.implement`
- Test owner: `bubbles.test`
- Validation owner: `bubbles.validate`
