# Bug: BUG-025-002 Knowledge synthesis E2E external URL extraction failure

## Summary
The knowledge synthesis E2E captures `https://example.com/synthesis-e2e-test` and fails on external URL extraction, making the test depend on a non-owned network/resource path instead of a deterministic live-stack fixture.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Knowledge synthesis live-stack E2E blocked by non-deterministic external extraction
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [ ] Confirmed (targeted red-stage output to be captured by owner)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the full E2E suite through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/knowledge_synthesis_test.go::TestKnowledgeSynthesis_PipelineRoundTrip` to execute.
3. The test posts capture JSON containing `url: https://example.com/synthesis-e2e-test` and text content.
4. The capture or synthesis path attempts external URL extraction and the E2E fails before producing deterministic knowledge stats proof.

## Expected Behavior
Knowledge synthesis E2E should use deterministic inputs owned by the disposable stack and prove capture -> process -> synthesize -> stats behavior without depending on a remote URL fetch.

## Actual Behavior
The E2E depends on external URL extraction for `example.com`, and the broad E2E suite reports this as a knowledge synthesis failure.

## Environment
- Service: Go core capture API, knowledge synthesis pipeline, Python ML sidecar
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Workflow context from bubbles.stabilize: knowledge synthesis e2e fails on external URL extraction.
Relevant test path: tests/e2e/knowledge_synthesis_test.go.
The test body includes url "https://example.com/synthesis-e2e-test" plus text content.
```

## Root Cause (initial analysis)
The E2E fixture appears to rely on an external URL extraction path that is not owned by the test stack. Live-stack E2E should exercise real internal code paths, but external network dependencies need deterministic local fixtures or explicit, fail-loud environment gating.

## Related
- Feature: `specs/025-knowledge-synthesis-layer/`
- E2E test: `tests/e2e/knowledge_synthesis_test.go`
- Related sibling bug: `BUG-025-001-knowledge-stats-empty-store`
