# Report: BUG-038-002 Provider-neutral Drive search omission

## Summary

This packet owns `BROAD-DRIVE-SEARCH-001` from synthesis closeout. It separates the provider-neutral search omission from the later core/network cascade and requires a fresh live RED reproduction before source changes.

## Completion Statement

Status is `in_progress`. No validate-owned certification or completion claim is made in this source-delivery packet.

## Routing Provenance

**Phase:** bug
**Claim Source:** interpreted
**Source:** `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/report.md#independent-broad-findings-routed-out-of-packet`

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
drive scan: completed provider=google seen=1 indexed=1 skipped=0
drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
drive_cross_feature_e2e_test.go:147: /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (3.87s)
--- FAIL: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (121.94s)
drive_policy_e2e_test.go:38: e2e: services not healthy after 30s at http://smackerel-core:8080
FAIL github.com/smackerel/smackerel/tests/e2e/drive 300.054s
spec_076_migrations_e2e_test.go:59: lookup postgres on 127.0.0.11:53: no such host
FAIL github.com/smackerel/smackerel/tests/e2e/foundation 0.056s
```

**Interpretation:** The search assertion failed while core was still serving the request, before the independently routed health collapse. Later Drive/foundation failures are not evidence of additional search defects.

## RED: Bug Reproduction Before Fix

**Phase:** test
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$'`
**Exit Code:** 1
**Claim Source:** executed

```text
PRECHECK: concurrent Smackerel test processes clean
PRECHECK: smackerel-test containers/networks/volumes clean
go-e2e: applying -run selector: ^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
2026/07/19 18:21:53 INFO drive scan: completed provider=google seen=1 indexed=1 skipped=0
2026/07/19 18:21:53 INFO drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
drive_cross_feature_e2e_test.go:171: /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (2.13s)
FAIL
FAIL github.com/smackerel/smackerel/tests/e2e/drive 2.174s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

Twenty exact-title `Tomato salad` rows fill the bounded result window and reproduce the broad failure without relying on another package's residue.

## Root Cause Evidence

Fresh-stack isolated and neighbor runs passed before contaminants were introduced. The deterministic contender run failed while both provider rows were persisted and directly loadable. Running the same RED immediately before observability produced an observability PASS, ruling out core destabilization.

## Test Evidence

### GREEN: Focused Post-Fix Verification

Concrete regression: `tests/e2e/drive/drive_cross_feature_e2e_test.go`.
Provider-neutral integration coverage: `tests/integration/drive/drive_cross_feature_test.go` and `tests/integration/drive/drive_multi_provider_search_test.go`.

**Phase:** test
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$'`
**Exit Code:** 0
**Claim Source:** executed

```text
PRECHECK: concurrent Smackerel test processes clean
PRECHECK: smackerel-test containers/networks/volumes clean
go-e2e: applying -run selector: ^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
2026/07/19 18:27:03 INFO drive scan: completed provider=google seen=1 indexed=1 skipped=0
2026/07/19 18:27:03 INFO drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (2.15s)
PASS
ok github.com/smackerel/smackerel/tests/e2e/drive 2.198s
PASS: go-e2e
Skipping Ollama agent E2E
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

## Change Boundary Evidence

The packet permits changes only in the confirmed search implementation/tests, Drive integration/E2E tests, and this bug folder. Parent synthesis and assistant packets are excluded.

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
PASS: synthesis, assistant, knb, deploy, and release-train paths untouched
```

### Broader Verification

**Phase:** test
**Commands:** focused Go search units; full Python units; focused Drive integration; full serialized Drive E2E selector; `check`; `lint`; `format --check`
**Exit Code:** 0 for every listed command
**Claim Source:** executed

```text
=== RUN   TestSearchPage_NilPool
--- PASS: TestSearchPage_NilPool (0.00s)
=== RUN   TestSearchResults_KnowledgeMatchTemplate
--- PASS: TestSearchResults_KnowledgeMatchTemplate (0.00s)
[go-unit] go test ./... finished OK
708 passed, 2 deselected in 16.04s
[py-unit] pytest ml/tests finished OK
--- PASS: TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters (0.09s)
ok github.com/smackerel/smackerel/tests/integration/drive 0.324s
PASS: go-integration
1 passed in 0.46s
PASS: python-integration
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (0.26s)
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (0.13s)
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

## Parent Consolidation Reference

Parent consolidation should cite this report's current-session RED/green sections and the final pushed commit, not the inherited routing excerpt above.
