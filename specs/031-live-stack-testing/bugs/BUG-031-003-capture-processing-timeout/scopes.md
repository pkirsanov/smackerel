# Scopes: BUG-031-003 Capture process/search E2E processing timeout

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore capture process search live-stack proof

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-003 restore capture process search E2E
  Scenario: Capture process search pipeline reaches processed status
    Given the disposable live stack is healthy
    When an E2E test captures a unique text artifact
    Then artifact detail eventually reports processing_status as processed or completed
    And searching for content from that artifact returns the artifact

  Scenario: Capture process search regression fails on empty processing status
    Given the artifact detail response omits or empties processing_status
    When the E2E polling loop evaluates the captured artifact
    Then the test fails with diagnostic output instead of treating the artifact as processed
```

### Implementation Plan
1. Reproduce the targeted E2E failure and record the final artifact detail response.
2. Trace the captured artifact through the database row, NATS processing message, ML sidecar result, and search query.
3. Fix the first failing production or harness contract with the smallest change boundary.
4. Keep the E2E assertion direct: processed status is required before search.
5. Re-run the targeted E2E test and the broad E2E suite through the repo CLI.

### Change Boundary

Allowed file families for this narrow repair:
- `tests/e2e/capture_process_search_test.go` for the live capture/process/search proof and adversarial status assertions.
- `tests/integration/test_runtime_health.sh` for disposable test-stack image freshness before live E2E execution.
- `ml/Dockerfile`, `ml/app/embedder.py`, `ml/app/processor.py`, `ml/app/main.py`, and `ml/tests/test_embedder.py` for ML-sidecar package metadata, model-cache, pending-counter, and degraded fallback behavior required by the captured-artifact processing path.
- `internal/api/search.go` for degraded text search fallback that returns processed captured artifacts.
- `config/smackerel.yaml`, `scripts/commands/config.sh`, and `docker-compose.yml` only for SST-driven ML degraded fallback and test-stack runtime plumbing needed by this bug.
- `specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout/scopes.md` and `uservalidation.md` for this planning reconciliation.

Excluded surfaces that must remain untouched by this scope:
- Parent `specs/031-live-stack-testing/` artifacts outside this bug packet.
- Feature 039 artifacts, recommendation-engine runtime code, and certification fields.
- Connector-specific runtime code and connector-specific E2E tests.
- NATS contract/runtime behavior beyond disposable test-stack lifecycle.
- `bug.md` lifecycle status and `state.json` certification or phase-provenance fields, which remain foreign-owned.

Validation boundary: this scope claims the focused capture/process/search E2E and broad E2E suite evidence only. Full `./smackerel.sh test integration` is not claimed green because unrelated BUG-022-001 NATS failures remain.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-031-003-01 | Capture process search reaches processed status | e2e-api | `tests/e2e/capture_process_search_test.go` | Captured artifact reaches processed/completed status and appears in search results | BUG-031-003-SCN-001 |
| T-BUG-031-003-02 | Regression E2E: empty processing status fails loudly | e2e-api | `tests/e2e/capture_process_search_test.go` | Empty or missing `processing_status` cannot silently pass the scenario | BUG-031-003-SCN-002 |
| T-BUG-031-003-03 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the capture processing timeout | BUG-031-003-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence  
  **Evidence:** Red-stage and repair-loop evidence showed a layered live-stack failure: stale test images prevented the ML fallback code from running; the rebuilt ML image then failed because package metadata had been stripped from `site-packages`; failed model loads leaked the embedder pending counter; the non-root ML runtime had no writable/prewarmed Hugging Face cache; and degraded text search did not match the processed capture after domain-intent query cleanup.
- [x] Captured artifacts transition to processed/completed status in the live-stack pipeline  
  **Evidence:** `timeout 1200 ./smackerel.sh test e2e --go-run TestE2E_CaptureProcessSearch` captured artifact `01KQ8N1HC5QV99FTBG4YDGFZG0`, logged repeated `status=pending`, then logged `artifact processed: status=processed`. **Phase:** implement. **Claim Source:** executed.
- [x] Artifact detail exposes the status value asserted by the E2E test  
  **Evidence:** The same focused E2E run read `/api/artifact/{artifact_id}` and `parseProcessingStatus` accepted the non-empty response value only after it was `processed`; missing, empty, whitespace, failed, and unknown status values were rejected by `TestE2E_CaptureProcessSearch_AdversarialEmptyStatus`. **Phase:** implement. **Claim Source:** executed.
- [x] Pre-fix regression test fails for the processing timeout  
  **Evidence:** Original test failure captured in red-stage evidence showing 60s timeout with "failed" status
- [x] Adversarial regression case exists for empty or missing processing status  
  **Evidence:** Added `TestE2E_CaptureProcessSearch_AdversarialEmptyStatus()` that fails if `processing_status` is empty
- [x] Post-fix targeted E2E regression passes  
  **Evidence:** `timeout 1200 ./smackerel.sh test e2e --go-run TestE2E_CaptureProcessSearch` returned `PASS` / `ok github.com/smackerel/smackerel/tests/e2e 30.213s`; it logged `search returned 9 results (mode=text_fallback, candidates=9)` and `found captured artifact in search results: This is a test artifact about Mediterranean cooking techniques. Unique marker: e2e-test-177733322483`. **Phase:** implement. **Claim Source:** executed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior  
  **Evidence:** `tests/e2e/capture_process_search_test.go::TestE2E_CaptureProcessSearch_AdversarialEmptyStatus` now exercises missing, empty, whitespace, failed, unknown, pending, processed, and completed status cases through the same status evaluator used by the live polling loop.
- [x] Broader E2E regression suite passes  
  **Evidence:** `timeout 3600 ./smackerel.sh test e2e` returned exit code 0 in validation, with Shell E2E 34/34 passed, Go E2E passed, and `TestE2E_CaptureProcessSearch` reaching `processed` for artifact `01KQAS0PKS6SEEDVHCXRX84EV7` before search returned the captured artifact. **Phase:** validate. **Claim Source:** executed.
- [x] Regression tests contain no silent-pass bailout patterns  
  **Evidence:** Adversarial test uses `t.Fatal()` for empty status - no early returns or conditional skips
- [x] Change Boundary is respected and zero excluded file families were changed  
  **Evidence:** Validation Code Diff Evidence lists changes in the allowed bug packet, E2E, ML sidecar, config-generation, Docker test-runtime plumbing, and search fallback surfaces. It does not list parent 031 artifacts outside this bug packet, feature 039 artifacts or certification fields, connector-specific runtime/E2E files, NATS contract/runtime code, `bug.md`, or `state.json` certification fields. **Phase:** validate. **Claim Source:** interpreted.
- [x] Foreign-owned lifecycle and certification artifacts remain excluded from plan-owned completion  
  **Evidence:** This planning reconciliation updates only `scopes.md` and `uservalidation.md`; `bug.md` lifecycle status and `state.json` certification or phase-provenance fields remain routed to their owning agents. **Phase:** plan. **Claim Source:** interpreted.
