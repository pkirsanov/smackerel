# Bug: BUG-031-005 Stress stack health readiness regression

## Summary
`./smackerel.sh test stress` can leave the Go stress phase pointed at an unhealthy or mismatched live stack, causing shared stress packages to fail on `http://127.0.0.1:40001` health or database reachability before their workload assertions run.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Shared stress validation blocks multiple feature deliveries and can misclassify infrastructure readiness as package workload failure
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the feature-level regression phase for spec 039 through the repo-standard stress command: `./smackerel.sh test stress`.
2. Observe shell stress checks pass for health and search against the managed stress stack.
3. Observe the Go stress package phase start after the shell stress checks.
4. Observe knowledge, photos, recommendation, drive, and agent stress tests fail or time out while waiting for stack health or database reachability.
5. Compare the failing packages: they cross spec 025, spec 037, spec 038, spec 039, and spec 040 ownership, so the common failure point is shared stress stack readiness and environment handoff rather than a recommendation-engine workload defect.

## Expected Behavior
`./smackerel.sh test stress` should run every stress phase against the correct disposable test stack using SST-derived configuration. The command should prove core health, database reachability, NATS reachability, and auth wiring before package workloads start. If the stress stack is unhealthy or miswired, the command should fail once with a clear infrastructure readiness error. If the stack is healthy, package stress failures should remain visible as workload failures and should not be masked by readiness skips or broad bailouts.

## Actual Behavior
The current stress evidence shows the shell stress phase passing before multiple Go stress packages fail against `http://127.0.0.1:40001` or fail to ping the database. Source inspection shows the stress command runs shell stress scripts against `--env test`, then generates and uses `dev` environment values for the Go stress Docker phase. The Go stress phase receives `CORE_EXTERNAL_URL`, `SMACKEREL_AUTH_TOKEN`, and `DATABASE_URL`, but does not receive `NATS_URL` even though agent stress requires it after DB reachability.

## Environment
- Service: `./smackerel.sh test stress`, shell stress scripts, Go stress Docker phase, disposable/live stack readiness
- Parent owner: `specs/031-live-stack-testing/`
- Blocking feature: `specs/039-recommendations-engine/`
- Version: Workspace evidence recorded in spec 039 report on 2026-05-03T22:05:34Z
- Platform: Linux, Docker-backed local validation stack

## Error Output
```text
$ ./smackerel.sh test stress
Health stress test passed with 25/25 successful requests
=== Search Stress Results ===
  Artifacts in DB:    1100
  Queries executed:   10
  Average time:       1336ms
  Threshold:          3000ms
  Failures:           0
Search stress test passed: all queries completed under 3000ms with 1100 artifacts
=== RUN   TestKnowledge_LintAt1000ArtifactScale
  knowledge_stress_test.go:111: stress: services not healthy after 2m0s at http://127.0.0.1:40001
--- FAIL: TestKnowledge_LintAt1000ArtifactScale (126.06s)
Exit Code: 1
```

```text
$ ./smackerel.sh test stress
=== RUN   TestKnowledge_ConceptQueryPerformance
  knowledge_stress_test.go:150: stress: services not healthy after 2m0s at http://127.0.0.1:40001
--- FAIL: TestKnowledge_ConceptQueryPerformance (126.05s)
=== RUN   TestKnowledge_SearchWithKnowledgeLayerPerformance
  knowledge_stress_test.go:209: stress: services not healthy after 2m0s at http://127.0.0.1:40001
--- FAIL: TestKnowledge_SearchWithKnowledgeLayerPerformance (126.06s)
=== RUN   TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget
  photos_ingest_stress_test.go:54: stress: services not healthy after 2m0s at http://127.0.0.1:40001
--- FAIL: TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget (126.07s)
Exit Code: 1
```

```text
$ ./smackerel.sh test stress
=== RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
panic: test timed out after 12m0s
    running tests:
        TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (1m30s)
github.com/smackerel/smackerel/tests/stress.stressWaitForHealth(0xc000283c00, {{0xc00003c012?, 0x1017360?}, {0xc00003e015?, 0x45e7a9?}}, 0x1bf08eb000)
    /workspace/tests/stress/knowledge_stress_test.go:56 +0x97
github.com/smackerel/smackerel/tests/stress.TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests(0xc000283c00)
    /workspace/tests/stress/recommendations_test.go:53 +0x7b
FAIL    github.com/smackerel/smackerel/tests/stress     720.055s
Exit Code: 1
```

```text
$ ./smackerel.sh test stress
=== RUN   TestConcurrentInvocationIsolation_BS018
  concurrency_test.go:183: ping db: context deadline exceeded
--- FAIL: TestConcurrentInvocationIsolation_BS018 (10.01s)
FAIL    github.com/smackerel/smackerel/tests/stress/agent       10.032s
=== RUN   TestDriveScaleStress_FiveThousandFilesMonitorReplayAndSaveBurst
  drive_scale_stress_test.go:67: stress: live stack not healthy at http://127.0.0.1:40001
--- FAIL: TestDriveScaleStress_FiveThousandFilesMonitorReplayAndSaveBurst (126.06s)
FAIL    github.com/smackerel/smackerel/tests/stress/drive       126.081s
FAIL
Command exited with code 1
Exit Code: 1
```

## Root Cause
Classification root cause is confirmed at the shared stress command/harness boundary: `smackerel.sh` runs shell stress scripts against the `test` environment, then switches the Go stress Docker phase to SST-generated `dev` environment values. That explains the common `http://127.0.0.1:40001` health target and the cross-package DB reachability failures. The exact implementation repair remains routed to the next owner.

## Related
- Feature: `specs/031-live-stack-testing/`
- Blocking regression evidence: `specs/039-recommendations-engine/report.md`
- Affected stress surfaces: `tests/stress/knowledge_stress_test.go`, `tests/stress/photos_ingest_stress_test.go`, `tests/stress/recommendations_test.go`, `tests/stress/drive/drive_scale_stress_test.go`, `tests/stress/agent/concurrency_test.go`
- Harness surfaces: `smackerel.sh`, `scripts/runtime/go-stress.sh`, `tests/stress/test_health_stress.sh`, `tests/stress/test_search_stress.sh`
