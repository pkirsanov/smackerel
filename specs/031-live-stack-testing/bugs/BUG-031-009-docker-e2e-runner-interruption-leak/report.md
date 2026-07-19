# Report: BUG-031-009 Dockerized Go E2E runner interruption leak

## Summary

This packet owns the test-harness root cause routed from BUG-038-003. Host process cleanup does not own Docker daemon-side runner lifetime, allowing interrupted Go tests to continue against a stack being removed.

## Completion Statement

Status is `in_progress`. No validate-owned certification or terminal completion is claimed.

## RED: Bug Reproduction Before Fix

**Phase:** test
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`
**Exit Code:** 1
**Claim Source:** executed

```text
=== BUG-031-004-SCN-002: regression detects surviving child work ===
PASS: BUG-031-004-SCN-002
=== BUG-031-004-SCN-001: E2E interruption terminates child processes ===
PASS: BUG-031-004-SCN-001
=== BUG-031-009-SCN-001/002: interrupted Docker Go runner is reaped before teardown ===
Observed Go E2E runner container: faa2f9f67489
Observed nested runner log marker: === RUN   TestDrive
Interrupting nested Dockerized E2E runner pid 1808965
Observed nested runner log marker: Running project-scoped test stack teardown (exit cleanup
FAIL: Dockerized Go E2E runner faa2f9f67489 survived until stack teardown began
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
FAIL: test_timeout_process_cleanup.sh (exit=1)
```

## Root Cause Evidence

Source inspection and the controlled RED agree: `e2e_child_run_id` was assigned to the host command, both Go `docker run` invocations had no run-ID label, and `e2e_stop_child` had no Docker-container reaper. The runner remained active at teardown start.

## Test Evidence

### GREEN: Post-Fix Interruption Verification

Concrete regression file: `tests/e2e/test_timeout_process_cleanup.sh`.

**Phase:** test
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
=== BUG-031-004-SCN-002: regression detects surviving child work ===
Detector reported surviving child work: Surviving child work for marker ...-adversarial
Marker processes absent for ...-adversarial
PASS: BUG-031-004-SCN-002
=== BUG-031-004-SCN-001: E2E interruption terminates child processes ===
Nested E2E runner returned nonzero after interruption: -1
Marker processes absent for ...-runner
PASS: BUG-031-004-SCN-001
=== BUG-031-009-SCN-001/002: interrupted Docker Go runner is reaped before teardown ===
Observed Go E2E runner container: 7a546e338d0b
Observed nested runner log marker: === RUN   TestDrive
Interrupting nested Dockerized E2E runner pid 1908149
Observed nested runner log marker: Running project-scoped test stack teardown (exit cleanup
PASS: BUG-031-009-SCN-001
PASS: BUG-031-009-SCN-002
PASS: BUG-031-004 timeout process cleanup regression
PASS: test_timeout_process_cleanup.sh
Total: 1
Passed: 1
Failed: 0
```

### Broader Recovery And Quality Evidence

**Phase:** test
**Commands:** focused Go search units; full Python units; focused Drive integration; Drive neighbor E2E; full serialized Drive E2E selector; `check`; `lint`; `format --check`
**Exit Code:** 0 for every listed command
**Claim Source:** executed

```text
[go-unit] go test ./... finished OK
708 passed, 2 deselected in 16.04s
[py-unit] pytest ml/tests finished OK
--- PASS: TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters (0.09s)
ok github.com/smackerel/smackerel/tests/integration/drive 0.324s
PASS: go-integration
1 passed in 0.46s
PASS: python-integration
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (0.92s)
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (0.11s)
--- PASS: TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare (0.04s)
--- PASS: TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly (0.09s)
--- PASS: TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable (0.08s)
--- PASS: TestDriveSaveE2E_ConcurrentMissingFolderCreatesExactlyOneFolder (0.17s)
--- PASS: TestDriveScanE2E_EmptyDriveCreatesNoArtifacts (0.07s)
ok github.com/smackerel/smackerel/tests/e2e/drive 12.285s
PASS: go-e2e
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
All checks passed!
Web validation passed
75 files already formatted
```

## Change Boundary Evidence

Allowed changes are limited to the E2E lifecycle, existing cleanup regression, this packet, and originating BUG-038-003 routing metadata.

### Code Diff Evidence

**Phase:** bug
**Command:** `git diff --stat && git status --short --branch`
**Exit Code:** 0
**Claim Source:** executed

```text
 smackerel.sh                                    |  64 +++++++++++++-
 tests/e2e/drive/drive_cross_feature_e2e_test.go |  35 ++++++--
 tests/e2e/test_timeout_process_cleanup.sh       | 109 +++++++++++++++++++++++-
 3 files changed, 198 insertions(+), 10 deletions(-)
## bug/drive-broad-e2e-20260719
 M smackerel.sh
 M tests/e2e/drive/drive_cross_feature_e2e_test.go
 M tests/e2e/test_timeout_process_cleanup.sh
?? specs/031-live-stack-testing/bugs/BUG-031-009-docker-e2e-runner-interruption-leak/
?? specs/038-cloud-drives-integration/bugs/BUG-038-002-provider-neutral-search-omission/
?? specs/038-cloud-drives-integration/bugs/BUG-038-003-drive-e2e-core-health-collapse/
```

## Parent Consolidation Reference

BUG-038-003 and the synthesis parent should cite this report's Docker-runner RED/green evidence and final pushed commit.
