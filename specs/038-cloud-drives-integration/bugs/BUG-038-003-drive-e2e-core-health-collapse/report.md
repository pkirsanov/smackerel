# Report: BUG-038-003 Drive E2E core health collapse

## Summary

This packet owns `BROAD-DRIVE-HEALTH-001` from synthesis closeout. It treats later Drive/foundation/retirement/transport/wiki failures as cascade noise until the first core or lifecycle defect is reproduced.

## Completion Statement

Status is `in_progress`. No validate-owned certification or completion claim is made in this source-delivery packet.

## Routing Provenance

**Phase:** bug
**Claim Source:** interpreted
**Source:** `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/report.md#independent-broad-findings-routed-out-of-packet`

```text
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
drive_observability_e2e_test.go:48: e2e: services not healthy after 2m0s at http://smackerel-core:8080
--- FAIL: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (121.94s)
drive_policy_e2e_test.go:38: e2e: services not healthy after 30s at http://smackerel-core:8080
drive_scan_e2e_test.go:17: waitForHealth
FAIL github.com/smackerel/smackerel/tests/e2e/drive 300.054s
spec_076_migrations_e2e_test.go:59: lookup postgres on 127.0.0.11:53: no such host
FAIL github.com/smackerel/smackerel/tests/e2e/foundation 0.056s
```

**Interpretation:** The first health-specific symptom is the observability test's two-minute core-health failure. Later missing-service and DNS errors are one cascade class until a fresh run proves otherwise.

## RED: Inherited Bug Reproduction Before Fix

Drive-local reproduction was attempted on fresh disposable stacks in four discriminating shapes: observability alone; cross-feature then observability; every Drive test in package order; and assistant failures/package followed by both Drive probes. All Drive probes passed. The inherited health failure is therefore not reproduced as a Drive-local defect.

## Root Cause Evidence

The owning BUG-031-009 controlled interruption produced current-session RED: the Go runner container remained active at the exact log boundary `Running project-scoped test stack teardown (exit cleanup)`. See `../../../031-live-stack-testing/bugs/BUG-031-009-docker-e2e-runner-interruption-leak/report.md#bug-reproduction---before-fix`.

## Test Evidence

### GREEN: Drive-Local Falsification And Cascade Classification

Concrete neighbor regressions: `tests/e2e/drive/drive_cross_feature_e2e_test.go` and `tests/e2e/drive/drive_observability_e2e_test.go`.

**Phase:** test
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^(TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers|TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture)$'`
**Exit Code:** 1 because the deliberately contaminated search regression was RED; observability passed
**Claim Source:** executed

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
2026/07/19 18:24:30 INFO drive scan: completed provider=google seen=1 indexed=1 skipped=0
2026/07/19 18:24:30 INFO drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
drive_cross_feature_e2e_test.go:171: /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (2.12s)
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
2026/07/19 18:24:32 INFO drive scan: completed provider=google seen=3 indexed=3 skipped=0
2026/07/19 18:24:32 INFO drive scan: completed provider=memdrive seen=2 indexed=2 skipped=0
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (0.11s)
FAIL github.com/smackerel/smackerel/tests/e2e/drive 2.271s
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

The failed search request itself does not exhaust or stop core. Policy/retrieve/save/scan, foundation, retirement, transports, and wiki failures after parent-owned teardown remain one cascade class.

### Post-Fix Neighbor And Drive-Package Proof

**Phase:** test
**Commands:** focused cross-feature/observability neighbor selector and full serialized Drive selector through `./smackerel.sh test e2e --go-run ...`
**Exit Code:** 0 for both commands
**Claim Source:** executed

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (0.92s)
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (0.11s)
ok github.com/smackerel/smackerel/tests/e2e/drive 1.072s
PASS: go-e2e
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (0.26s)
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (0.13s)
=== RUN   TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare
--- PASS: TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare (0.04s)
=== RUN   TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly
--- PASS: TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly (0.09s)
=== RUN   TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable
--- PASS: TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable (0.08s)
=== RUN   TestDriveSaveE2E_ConcurrentMissingFolderCreatesExactlyOneFolder
--- PASS: TestDriveSaveE2E_ConcurrentMissingFolderCreatesExactlyOneFolder (0.17s)
=== RUN   TestDriveScanE2E_EmptyDriveCreatesNoArtifacts
--- PASS: TestDriveScanE2E_EmptyDriveCreatesNoArtifacts (0.07s)
ok github.com/smackerel/smackerel/tests/e2e/drive 12.285s
PASS: go-e2e
```

## Cascade Classification

Policy, retrieve, save, scan, foundation, retirement, transports, and wiki failures after core/network disappearance are classified as cascade noise. They do not receive separate packets unless they remain after this packet and BUG-038-002 pass on a fresh stack.

## Change Boundary Evidence

The packet permits only the proven Drive observability/helper/owning runtime or E2E lifecycle fix and this bug folder. Parent synthesis and assistant packets are excluded.

### Code Diff Evidence

**Phase:** bug
**Command:** `git diff --name-only` plus excluded-path scan
**Exit Code:** 0
**Claim Source:** executed

```text
smackerel.sh
tests/e2e/drive/drive_cross_feature_e2e_test.go
tests/e2e/test_timeout_process_cleanup.sh
--- forbidden boundary paths ---
PASS: synthesis, assistant, knb, deploy, and release-train paths untouched
```

## Parent Consolidation Reference

Parent consolidation should cite this report's isolated/package-order RED, green Drive package, and final pushed commit.
