# Execution Report: BUG-031-003 Capture process/search E2E processing timeout

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

### Summary

BUG-031-003 covers the live-stack capture -> process -> search timeout in `tests/e2e/capture_process_search_test.go`. The repair restored the real pipeline proof instead of weakening the E2E assertion: the captured artifact must still reach `processed` or `completed`, and empty, missing, failed, or unknown status values fail loudly.

The root-cause repair addressed the runtime layers that kept the captured artifact from becoming searchable: disposable test image freshness, ML package metadata preservation, writable/prewarmed embedding cache setup for the non-root ML runtime, pending-counter cleanup on model-load failure, SST-gated degraded processing fallback, and degraded text search over captured raw content.

Full integration green is not claimed. `./smackerel.sh test integration` still exits 1 only on the unrelated BUG-022-001 NATS workqueue/MaxDeliver failures, while target-adjacent ML readiness and timeout fallback checks pass in that integration run.

### Test Evidence

**Phase:** audit  
**Command:** `timeout 1200 ./smackerel.sh test e2e --go-run TestE2E_CaptureProcessSearch`  
**Exit Code:** 0  
**Claim Source:** executed

```text
$ timeout 1200 ./smackerel.sh test e2e --go-run TestE2E_CaptureProcessSearch
Preparing disposable test stack...
Container smackerel-test-postgres-1        Healthy
Container smackerel-test-nats-1            Healthy
Container smackerel-test-smackerel-ml-1    Healthy
Container smackerel-test-smackerel-core-1  Healthy
go-e2e: applying -run selector: TestE2E_CaptureProcessSearch
=== RUN   TestE2E_CaptureProcessSearch
capture_process_search_test.go:95: captured artifact: id=01KQB226Q7T3MS6GFQGNWKS8AH title="This is a test artifact about Mediterranean cooking techniques. Unique marker: e2e-test-177741398703" type=generic
capture_process_search_test.go:131: waiting for processing... status=pending
capture_process_search_test.go:128: artifact processed: status=processed
capture_process_search_test.go:177: search returned 1 results (mode=text_fallback, candidates=1)
capture_process_search_test.go:185: found captured artifact in search results: This is a test artifact about Mediterranean cooking techniques. Unique marker: e2e-test-177741398703
capture_process_search_test.go:197: e2e capture->process->search test completed, artifact_id=01KQB226Q7T3MS6GFQGNWKS8AH
--- PASS: TestE2E_CaptureProcessSearch (36.24s)
=== RUN   TestE2E_CaptureProcessSearch_AdversarialEmptyStatus
--- PASS: TestE2E_CaptureProcessSearch_AdversarialEmptyStatus (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        36.261s
PASS: go-e2e
```

**Phase:** audit  
**Command:** `timeout 3600 ./smackerel.sh test e2e`  
**Exit Code:** 0  
**Claim Source:** executed

```text
$ timeout 3600 ./smackerel.sh test e2e
Shell E2E Test Results
PASS: test_compose_start.sh
PASS: test_persistence.sh
PASS: test_postgres_readiness_gate.sh
PASS: test_config_fail.sh
PASS: test_capture_pipeline.sh
PASS: test_search.sh
PASS: test_search_filters.sh
PASS: test_search_empty.sh
PASS: test_connector_framework.sh
PASS: test_browser_sync.sh
Total:  34
Passed: 34
Failed: 0
=== RUN   TestE2E_CaptureProcessSearch
capture_process_search_test.go:95: captured artifact: id=01KQB2RW52ZEQ9EK8GA39JAJQS title="This is a test artifact about Mediterranean cooking techniques. Unique marker: e2e-test-177741472986" type=generic
capture_process_search_test.go:128: artifact processed: status=processed
capture_process_search_test.go:177: search returned 1 results (mode=text_fallback, candidates=1)
capture_process_search_test.go:185: found captured artifact in search results: This is a test artifact about Mediterranean cooking techniques. Unique marker: e2e-test-177741472986
--- PASS: TestE2E_CaptureProcessSearch (36.27s)
--- PASS: TestE2E_CaptureProcessSearch_AdversarialEmptyStatus (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        103.265s
PASS: go-e2e
```

**Phase:** regression  
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/capture_process_search_test.go`  
**Exit Code:** 0  
**Claim Source:** executed

The regression-quality guard ran in bugfix mode, scanned `tests/e2e/capture_process_search_test.go`, detected the adversarial signal, and completed with 0 violations and 0 warnings across 1 scanned file.

Skip-marker and request-interception scans of `tests/e2e/capture_process_search_test.go` both returned exit 1 with no stdout. Those clean grep outcomes are intentionally recorded as prose rather than terminal-output fences because there was no substantive terminal output beyond the exit status.

### Code Diff Evidence

**Phase:** validate  
**Command:** `git status --short -- internal/api/search.go ml/Dockerfile ml/app/embedder.py ml/app/processor.py ml/tests/test_embedder.py tests/e2e/capture_process_search_test.go tests/integration/test_runtime_health.sh config/smackerel.yaml docker-compose.yml scripts/commands/config.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

```text
$ git status --short -- internal/api/search.go ml/Dockerfile ml/app/embedder.py ml/app/processor.py ml/tests/test_embedder.py tests/e2e/capture_process_search_test.go tests/integration/test_runtime_health.sh config/smackerel.yaml docker-compose.yml scripts/commands/config.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout
 M config/smackerel.yaml
 M docker-compose.yml
 M internal/api/search.go
 M ml/Dockerfile
 M ml/app/embedder.py
 M ml/app/processor.py
 M scripts/commands/config.sh
 M tests/e2e/capture_process_search_test.go
 M tests/integration/test_runtime_health.sh
?? ml/tests/test_embedder.py
?? specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout/
echo "$?"
0
```

Validation-owned change-boundary review found the changed surfaces inside the allowed bug packet, E2E, ML sidecar, config-generation, Docker test-runtime plumbing, and search fallback boundaries. It did not claim changes to parent 031 artifacts outside this bug packet, feature 039 artifacts or certification fields, connector-specific runtime/E2E files, NATS contract/runtime code, or foreign-owned certification fields.

### Integration Caveat

**Phase:** audit  
**Command:** `timeout 1200 ./smackerel.sh test integration`  
**Exit Code:** 1  
**Claim Source:** executed

```text
$ timeout 1200 ./smackerel.sh test integration
=== RUN   TestMLReadiness_WaitForHealthy
--- PASS: TestMLReadiness_WaitForHealthy (2.03s)
=== RUN   TestMLReadiness_TimeoutFallback
--- PASS: TestMLReadiness_TimeoutFallback (3.00s)
=== RUN   TestMLReadiness_EmptyURL
--- PASS: TestMLReadiness_EmptyURL (0.00s)
=== RUN   TestMLReadiness_ZeroTimeout
--- PASS: TestMLReadiness_ZeroTimeout (0.00s)
=== RUN   TestMLReadiness_Chaos_ContextCancelledMidWait
--- PASS: TestMLReadiness_Chaos_ContextCancelledMidWait (1.00s)
=== RUN   TestNATS_PublishSubscribe_Artifacts
nats_stream_test.go:92: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Artifacts (0.01s)
=== RUN   TestNATS_PublishSubscribe_Domain
nats_stream_test.go:164: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain (0.01s)
=== RUN   TestNATS_Chaos_MaxDeliverExhaustion
nats_stream_test.go:369: expected 0 messages after MaxDeliver exhaustion, got 1 - dead-message path broken
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion (2.02s)
FAIL    github.com/smackerel/smackerel/tests/integration        18.004s
PASS    github.com/smackerel/smackerel/tests/integration/agent  3.128s
PASS    github.com/smackerel/smackerel/tests/integration/drive  1.509s
FAIL
Command exited with code 1
```

The integration failure is preserved as the known unrelated BUG-022-001 NATS caveat. It is not used as BUG-031 completion evidence, and no full-integration green claim is made.

### Test Baseline Comparison

| Category | Before | After | Delta | Status |
|---|---|---|---|---|
| Focused BUG-031-003 E2E | Timed out or failed before the repair | Exit 0 with processed status and search hit | Repaired | CLEAN |
| Broad E2E | Blocked by capture processing timeout | Exit 0 with Shell E2E 34/34 and target Go E2E passing | Repaired | CLEAN |
| Go unit | Required after implementation changes | Exit 0 in audit evidence | Stable | CLEAN |
| Python ML unit | Required after ML-sidecar changes | Exit 0 with 352 passed and 2 warnings | Stable | CLEAN |
| Integration | Known BUG-022-001 NATS failures | Same NATS failures; ML readiness checks pass | Stable caveat | NO NEW TARGET REGRESSION |

### Cross-Spec Impact Scan

| Affected Spec/Area | Shared Surface | Regression Evidence | Status |
|---|---|---|---|
| `specs/031-live-stack-testing` | `tests/e2e/capture_process_search_test.go`, live stack process/search contract | Focused E2E and broad Go E2E exit 0 with processed status and search hit | CLEAN |
| `specs/002-phase1-foundation` | capture pipeline, capture API, LLM failure E2E, search E2E scripts | Broad E2E reports capture pipeline, ML-unavailable capture, and search scripts in the 34/34 shell result | CLEAN |
| `specs/022-operational-resilience` | ML readiness timeout fallback and `text_fallback` search semantics | Integration ML readiness/fallback checks pass; focused and broad E2E show `mode=text_fallback` returning the captured artifact | CLEAN WITH NATS CAVEAT |
| `specs/026-domain-extraction` | processed-status polling and search-after-processing live flow | Broad Go E2E suite remains compatible with processed-status and search flow | CLEAN |

### Validation Evidence

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

The post-promotion artifact lint detected status `done`, verified top-level status matches certification status, found required specialist phases and phase-scope coherence, validated all 6 report evidence blocks as legitimate, and completed with `Artifact lint PASSED`. Deprecated-field warnings for `scopeProgress`, `statusDiscipline`, and `scopeLayout` were non-blocking.

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

The post-promotion traceability guard verified both scenario contracts, confirmed linked test coverage in `tests/e2e/capture_process_search_test.go`, found report evidence for the concrete test, and ended with `RESULT: PASSED (0 warnings)`.

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

The post-promotion state-transition guard read status `done`, verified `bugfix-fastlane` permits `done`, confirmed top-level status matches `certification.status`, found all 11 DoD items checked, confirmed the single Done scope matches `completedScopes`, verified phase provenance including `bubbles.validate`, and passed artifact lint, artifact freshness, implementation delta evidence, phase-scope coherence, and implementation reality checks. The guard ended with `TRANSITION PERMITTED with 2 warning(s)` and exit code 0.

**Interpretation:** The two state-transition warnings were non-blocking: `completedAt` timestamps are absent, and the guard could not extract concrete test file paths from the Test Plan even though traceability guard separately verified the concrete linked E2E test. The done-mode gates permit final certification.

### Audit Evidence

**Phase:** audit  
**Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

```text
$ timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout
Required artifact exists: spec.md
Required artifact exists: design.md
Required artifact exists: uservalidation.md
Required artifact exists: state.json
Required artifact exists: scopes.md
Required artifact exists: report.md
No forbidden sidecar artifacts present
Found DoD section in scopes.md
scopes.md DoD contains checkbox items
All DoD bullet items use checkbox syntax in scopes.md
Found Checklist section in uservalidation.md
uservalidation checklist contains checkbox entries
All checked DoD items in scopes.md have evidence blocks
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Phase:** audit  
**Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`  
**Exit Code:** 0  
**Claim Source:** executed

```text
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout
BUBBLES TRACEABILITY GUARD
scenario-manifest.json covers 2 scenario contract(s)
scenario-manifest.json linked test exists: tests/e2e/capture_process_search_test.go
scenario-manifest.json records evidenceRefs
All linked tests from scenario-manifest.json exist
Scope 1: Restore capture process search live-stack proof scenario mapped to Test Plan row: Capture process search pipeline reaches processed status
Scope 1: Restore capture process search live-stack proof scenario maps to concrete test file: tests/e2e/capture_process_search_test.go
Scope 1: Restore capture process search proof report references concrete test evidence: tests/e2e/capture_process_search_test.go
Scope 1: Restore capture process search proof scenario mapped to Test Plan row: Capture process search regression fails on empty processing status
Scope 1: Restore capture process search proof scenario maps to concrete test file: tests/e2e/capture_process_search_test.go
Scope 1: Restore capture process search proof report references concrete test evidence: tests/e2e/capture_process_search_test.go
DoD fidelity scenarios: 2 (mapped: 2, unmapped: 0)
RESULT: PASSED (0 warnings)
```

### Completion Statement

BUG-031-003 has runtime proof for the targeted bug behavior: focused and broad E2E commands exit 0, the captured artifact reaches `processed`, and search returns the captured artifact. The adversarial status regression remains present so missing, empty, whitespace, failed, and unknown `processing_status` values cannot silently satisfy the live polling contract.

The validate phase promoted BUG-031-003 to `done` after post-promotion artifact lint, traceability guard, and state-transition guard all exited 0. The NATS integration caveat remains explicit and unrelated to this BUG-031 capture/process/search repair.
