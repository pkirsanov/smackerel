# Bug: BUG-031-003 Capture process/search E2E processing timeout

## Summary
The live-stack capture -> process -> search E2E times out waiting for `processing_status` to become processed after `POST /api/capture`, keeping the broad E2E suite red outside the 039 recommendations scope.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Live-stack E2E gate blocked for a core capture/search journey
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [ ] Reported
- [ ] Confirmed
- [ ] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the full E2E suite through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/capture_process_search_test.go::TestE2E_CaptureProcessSearch` to execute after the disposable stack reports healthy.
3. The test posts a text artifact to `/api/capture`.
4. The test polls `/api/artifact/{artifact_id}` for up to 60 seconds.
5. The test fails if `processing_status` never becomes `processed` or `completed`.

## Expected Behavior
A captured text artifact should move through the processing pipeline and become searchable from the live E2E stack within the test budget.

## Actual Behavior
The E2E scenario times out waiting for `processing_status`, so the search assertion never becomes a reliable proof of the capture pipeline.

## Environment
- Service: Go core, ML sidecar, NATS, PostgreSQL, disposable E2E stack
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux, Docker-backed E2E stack

## Error Output
```text
Workflow context from bubbles.stabilize: Capture process/search e2e times out waiting for processing_status.
Relevant test path: tests/e2e/capture_process_search_test.go::TestE2E_CaptureProcessSearch.
```

## Root Cause
The timeout was caused by layered live-stack processing failures rather than by the E2E wait condition. The disposable stack could run stale ML images, the rebuilt ML image was missing Python package metadata and a writable/prewarmed embedding cache for the non-root runtime user, embedder model-load failures leaked pending-count state, and degraded text search could miss processed captured content after query cleanup.

## Resolution Status
The bug lifecycle is marked Fixed based on focused and broad E2E runtime proof: `timeout 1200 ./smackerel.sh test e2e --go-run TestE2E_CaptureProcessSearch` exited 0, and `timeout 3600 ./smackerel.sh test e2e` exited 0. The repair keeps the live E2E requirement intact: captured artifacts must reach `processed` or `completed`, and empty, failed, unknown, or missing statuses fail loudly.

Full integration green is not claimed; unrelated BUG-022-001 NATS failures remain.

## Related
- Feature: `specs/031-live-stack-testing/`
- Scenario: SCN-LST-003 Full pipeline flow
- E2E test: `tests/e2e/capture_process_search_test.go`
- Existing related but non-covering bugs: `BUG-031-001-integration-stack-volume-and-migration-hang`, `BUG-031-002-dod-scenario-fidelity-gap`
