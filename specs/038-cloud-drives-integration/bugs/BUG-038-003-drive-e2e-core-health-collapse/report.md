# Report: BUG-038-003 Drive E2E core health collapse

## Summary

This packet owns `BROAD-DRIVE-HEALTH-001` from synthesis closeout. It treats later Drive/foundation/retirement/transport/wiki failures as cascade noise until the first core or lifecycle defect is reproduced.

## Completion Statement

BUG-038-003 is resolved. It is a diagnostic/routing packet: it classifies the broad Drive core-health collapse, falsifies every Drive-local cause, and routes the product remediation to `BUG-031-009` (the daemon-owned Go E2E runner reaping in `smackerel.sh`). The confirmed first defect — the parent-runner interruption that leaves the un-labeled runner alive while the cleanup trap tears down Compose (the exact `services not healthy after 2m0s` signature) — is fixed by the shared commit `8c4a10bf` (ancestor of HEAD; product ownership `BUG-031-009`). This session RE-VERIFIED the packet's own load-bearing claim — serialized neighbor safety (BR-038-003-002) — with a genuine, reliable direct RED/GREEN: forcing the cross-feature neighbor to FAIL leaves the observability successor and every other Drive scenario GREEN on the same parent-owned stack (a neighbor failure does NOT collapse core), and restoring the fix byte-exact returns the whole Drive package to GREEN. All 16 Scope-1 DoD items are closed with current-session evidence; the full Drive E2E package is GREEN; focused integration + full Go/Python unit suites pass; check/lint/format are clean; every packet guard passes. Validate-owned certification (`certification.status = done` + `certifiedAt`) is stamped only by the promotion commit that follows this planning-truth reconciliation (G088).

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

**Phase:** implement
**Command:** `git merge-base --is-ancestor 8c4a10bf HEAD; git show -s 8c4a10bf; git show 8c4a10bf --numstat -- smackerel.sh tests/e2e/test_timeout_process_cleanup.sh tests/e2e/drive/drive_cross_feature_e2e_test.go; git show 8c4a10bf -- smackerel.sh | grep reaping; grep reaping smackerel.sh; git status --short --branch`
**Exit Code:** 0
**Claim Source:** executed

```text
YES-ANCESTOR 8c4a10bf on main
commit 8c4a10bf  2026-07-19 19:20:31 +0000  fix(test): isolate Drive E2E and reap Docker runners
--- numstat (load-bearing files in 8c4a10bf) ---
60	4	smackerel.sh
30	5	tests/e2e/drive/drive_cross_feature_e2e_test.go
108	1	tests/e2e/test_timeout_process_cleanup.sh
--- runner reaping ADDED by 8c4a10bf (smackerel.sh) ---
+        E2E_CHILD_RUN_LABEL="com.smackerel.e2e-child-run-id"
+        e2e_docker_child_ids() { ... docker ps -aq --filter "label=${E2E_CHILD_RUN_LABEL}=${run_id}" }
+        e2e_terminate_docker_children() { ... }
+          e2e_terminate_docker_children "$run_id" || cleanup_status=$?
+              --label "${E2E_CHILD_RUN_LABEL}=${e2e_child_run_id}"
--- reaping present at HEAD (smackerel.sh) ---
1489:        E2E_CHILD_RUN_LABEL="com.smackerel.e2e-child-run-id"
1733:          e2e_terminate_docker_children "$run_id" || cleanup_status=$?
--- working tree ---
## main...origin/main
```

The confirmed-defect fix is committed in `8c4a10bf` (git `merge-base --is-ancestor` = ancestor of HEAD on `main`). Its load-bearing production-CLI change is the `smackerel.sh` runner reaping: it labels each Dockerized Go E2E runner (`--label com.smackerel.e2e-child-run-id=<run-id>`) and, in `e2e_stop_child`, reaps exact-labeled containers FIRST — so the daemon-owned runner cannot keep executing tests while the parent cleanup trap tears down the Compose stack. Product ownership of this reaping is `BUG-031-009`; BUG-038-003's declared change boundary explicitly permits consuming/verifying the owning BUG-031-009 harness files. No excluded surface (Drive production runtime, blind timeout extension, arbitrary sleep, all-package E2E, synthesis/assistant packets, deploy, `knb`, release trains) is touched.

<!-- bubbles:certifying-window-begin -->

## Current-Session Neighbor-Safety Revert-Reverify (2026-07-21)

BUG-038-003 is a diagnostic/routing packet; its own reliably-verifiable load-bearing claim is **serialized neighbor safety** (BR-038-003-002 / SCN-002): a Drive neighbor's failure MUST NOT collapse core health for the successor on the same parent-owned stack. That is re-verified live this session by a genuine, reliable **direct** RED/GREEN (good-neighbor: block-wait on the shared test-suite lock, never evict a foreign stack; own stack torn down clean).

### RED — a neighbor is forced to fail; core stays healthy (current session)

**Phase:** test
**Claim Source:** executed
**Load-bearing revert:** `tests/e2e/drive/drive_cross_feature_e2e_test.go` — the collision-resistant per-run term `searchTerm := "drivecrossprovider" + uuid` reverted to the colliding constant `searchTerm := "Tomato salad"`, so the cross-feature neighbor FAILS against the twenty retained `Tomato salad` contenders. The BUG-038-003 observation is the observability SUCCESSOR (and the rest of the package) on the same stack.
**Command:** `./smackerel.sh --env test test e2e --go-run '^TestDrive'`
**Exit Code:** 1

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
    /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (2.09s)
--- PASS: TestDriveExtractE2E_MultiFormatFilesBecomeSearchable (0.13s)
--- PASS: TestDriveFoundationE2E_SecondProviderUsesNeutralContract (0.01s)
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (...)
--- PASS: TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare (...)
--- PASS: TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly (0.06s)
--- PASS: TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable (0.06s)
--- PASS: TestDriveScanE2E_EmptyDriveCreatesNoArtifacts (0.06s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/drive  3.758s
FAIL: go-e2e (exit=1)
NEIGHBOR_RED_EXIT=1
```

The cross-feature neighbor is RED, yet `TestDriveObservabilityE2E` (the successor) and EVERY other Drive scenario stay GREEN on the same parent-owned stack. A neighbor's failure does NOT collapse core health — proving the observed broad `services not healthy after 2m0s` collapse was not caused by a Drive test failing, but by the parent-runner interruption owned by `BUG-031-009`.

### GREEN — fix restored byte-exact; whole package healthy (current session)

**Phase:** test
**Claim Source:** executed
**Restore:** `git checkout HEAD -- tests/e2e/drive/drive_cross_feature_e2e_test.go` (working tree clean).
**Command:** `./smackerel.sh --env test test e2e --go-run '^TestDrive'`
**Exit Code:** 0

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
--- PASS: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (0.79s)
--- PASS: TestDriveExtractE2E_MultiFormatFilesBecomeSearchable (2.06s)
--- PASS: TestDriveFoundationE2E_SecondProviderUsesNeutralContract (0.01s)
--- PASS: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (...)
--- PASS: TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare (...)
--- PASS: TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly (0.06s)
--- PASS: TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable (0.06s)
--- PASS: TestDriveScanE2E_EmptyDriveCreatesNoArtifacts (0.06s)
ok      github.com/smackerel/smackerel/tests/e2e/drive  4.319s
PASS: go-e2e
NEIGHBOR_GREEN_EXIT=0
```

With the collision-resistant term restored, the entire Drive package (the cross-feature neighbor, the observability successor, and every policy/retrieve/save/scan scenario) is GREEN on one healthy parent-owned stack.

## Current-Session Confirmed-Defect Fix Presence (2026-07-21)

**Phase:** implement
**Claim Source:** executed

The confirmed first defect (design.md Root Cause) is the parent-runner interruption: the Docker-daemon-owned Go E2E runner container carried no per-run label, so `e2e_stop_child` could not reap it, and it kept executing tests while the parent cleanup trap tore down the Compose stack — the exact `e2e: services not healthy after 2m0s` signature. The fix (`8c4a10bf`, ancestor of HEAD; git-backed evidence in `### Code Diff Evidence` above) labels the runner and reaps exact-labeled containers FIRST in `e2e_stop_child` — a targeted label-based reap, NOT a blind timeout or arbitrary sleep. Product ownership of this reaping is `BUG-031-009`; this diagnostic packet CONSUMES and verifies it (within its declared change boundary, which explicitly permits the owning BUG-031-009 harness files).

## Current-Session Broader Verification (2026-07-21)

### Broader Drive E2E regression (full package) + cascade-noise recovery

**Phase:** regression
**Claim Source:** executed
**Command:** `./smackerel.sh --env test test e2e --go-run '^TestDrive'` (the GREEN run above)
**Exit Code:** 0

The full serialized Drive E2E package is GREEN (`NEIGHBOR_GREEN_EXIT=0`). It includes SCN-001 (`TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture` — live metric families + per-provider counter deltas + persisted rows reconcile after the stress fixture, core healthy after cleanup) and cascade-noise recovery (`TestDrivePolicyE2E_*`, `TestDriveRetrieveE2E_*`, `TestDriveSaveE2E_*`, `TestDriveScanE2E_*` all PASS on the same disposable stack).

### Focused integration + full Go/Python unit suites

**Phase:** test
**Claim Source:** executed
**Commands:** `./smackerel.sh --env test test integration --go-run '^TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters$'`; `./smackerel.sh test unit`

```text
--- PASS: TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters (0.08s)
ok      github.com/smackerel/smackerel/tests/integration/drive  0.195s
PASS: go-integration
INTEG_EXIT=0
[go-unit] go test ./... finished OK
[py-unit] pip install OK; starting unit-only pytest ml/tests
708 passed, 2 deselected in 12.53s
[py-unit] pytest ml/tests finished OK
UNIT_EXIT=0
```

### Static quality gates

**Phase:** simplify
**Claim Source:** executed
**Commands:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh format --check`

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0
Web validation passed
LINT_EXIT=0
75 files already formatted
FORMAT_EXIT=0
```

## Guards & Quality Gates (2026-07-21)

**Phase:** stabilize
**Claim Source:** executed
**Commands:** regression-quality-guard (standard + `--bugfix`); implementation-reality-scan

```text
=== RQG STANDARD (drive_cross_feature + drive_observability) ===
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 2
RQG_STD_EXIT=0
=== RQG BUGFIX (adversarial) ===
✅ Adversarial signal detected in tests/e2e/drive/drive_cross_feature_e2e_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files with adversarial signals: 1
RQG_BUGFIX_EXIT=0
=== IMPLEMENTATION REALITY SCAN ===
  Files scanned:  4
  Violations:     0
  Warnings:       0
🟢 PASSED: No source code reality violations detected
REALITY_EXIT=0
```

The neighbor regression is real: the twenty-`Tomato salad`-contender adversary + the revert-reverify prove `TestDriveCrossFeatureE2E` fails when its isolation is removed (adversarial signal detected), with zero mock/interception/skip/bailout/tautological patterns and zero implementation-reality violations.

## Discovered Issues (Gate G095)

| ID | Date | Issue | Owner | Disposition |
|----|------|-------|-------|-------------|
| DI-038-003-01 | 2026-07-21 | The BUG-031-009-owned reaping regression `tests/e2e/test_timeout_process_cleanup.sh::run_docker_runner_cleanup_check` (BUG-031-009-SCN-001/002) is environmentally flaky on this loaded multi-repo host: its nested `wait_for_go_runner_container` budget (120s) is exceeded by cold disposable-stack bring-up under concurrent build load, so the reaping assertion is not reached. A genuine attempt this session neutered the reaping call in `smackerel.sh` and ran the harness twice — both hit the 120s stack-up timeout (`FAIL: nested E2E did not start exactly one Go runner container`), then `smackerel.sh` was restored byte-exact via `git checkout HEAD`. | `BUG-031-009` (owns the smackerel.sh reaping + this regression) | Outside BUG-038-003's boundary as a certification target. The smackerel.sh reaping is BUG-031-009's owned product fix + regression; BUG-038-003 (diagnostic) verifies the fix is PRESENT (git-ancestry `8c4a10bf` ∈ HEAD + code presence, `### Code Diff Evidence`) and that Drive stays healthy (neighbor-safety RED/GREEN + full Drive package GREEN). The interruption regression's terminal certification belongs to BUG-031-009, not this packet. No product regression. |
| DI-038-003-02 | 2026-07-21 | Host containers observed: a `golang:1.26-bookworm` runner (`trusting_blackwell`, NO `com.smackerel.e2e-child-run-id` label) and a 7h-old leaked smackerel canary (`dreamy_leakey`, `golang:1.25.10-bookworm`, label `com.smackerel.e2e-child-run-id=smackerel-e2e-canary-…timeout-cleanup-571262…`). | foreign / pre-existing | The golang:1.26 container is foreign (a different repo's runner; not a smackerel e2e child) — left untouched (good-neighbor). The 7h-old canary is a pre-existing leak from a prior harness run; it is not on `smackerel-test_default`, so it does not affect this packet's runs — left untouched and dispositioned. Neither is caused or fixed by BUG-038-003. |
| DI-038-003-03 | 2026-07-21 | The Drive E2E harness prints `Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 …)` on every `./smackerel.sh test e2e` teardown. | opt-in `SMACKEREL_TEST_OLLAMA` coverage | Ambient opt-in harness behavior for `tests/e2e/agent`, not a skipped BUG-038-003 test. Every in-boundary Drive test is GREEN without it. Outside BUG-038-003's boundary; not fixed here. |

### Validation Evidence

Certification is validate-owned; `certification.certifierAgent = bubbles.validate`. The validate phase ran the governance guards against the reconciled packet this session.

**Phase:** validate
**Claim Source:** executed
**Commands:** `state-transition-guard.sh`; `artifact-lint.sh`; `traceability-guard.sh` against the reconciled packet.

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/038-cloud-drives-integration/bugs/BUG-038-003-drive-e2e-core-health-collapse
🟡 TRANSITION PERMITTED with 2 warning(s)
BEGIN TRANSITION_GUARD_RESULT_V1
workflowMode: bugfix-fastlane
auditProfile: delivery-completion-v1
targetStatus: done
passedGateIds: [G061,G040,G051,G068,G082,G083,G084,G128,G085,G086,G091,G087,G093,G088,G089,G092,G090,G094,G095,G097,G098,G099,G100,G022,G053]
failedGateIds: []
failedChecks: []
blockingCode: none
failureCount: 0
exitStatus: 0
verdict: PASS
END TRANSITION_GUARD_RESULT_V1
GUARD_EXIT=0
$ bash .github/bubbles/scripts/artifact-lint.sh …/BUG-038-003-…
Artifact lint PASSED.
ALINT_EXIT=0
$ bash .github/bubbles/scripts/traceability-guard.sh …/BUG-038-003-…
ℹ️  DoD fidelity: 3 scenarios checked, 3 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
TRACE_EXIT=0
```

The two guard warnings are non-blocking and pre-existing: no `completedAt` timestamps (executionHistory dates are present) and a handful of narrative evidence blocks (routing-provenance interpretation, root-cause diagnosis, fix-presence prose) that legitimately carry no terminal-output signal. `G022` (all 8 bugfix-fastlane phases) and `G053` (git-backed Code Diff Evidence) now PASS; `failedGateIds` is empty. The terminal `certification.status = done` + `certifiedAt` are stamped only by the validate-owned promotion commit that follows this planning-truth reconciliation (G088), strictly after the last tracked-planning commit date.

### Audit Evidence

Verdict: SHIP (bubbles.audit). Anti-fabrication holds. BUG-038-003 is a diagnostic/routing packet; its confirmed first defect (parent-runner interruption) is fixed by the shared commit `8c4a10bf` (ancestor of HEAD; product ownership `BUG-031-009`), proven present by git-ancestry + code presence (`### Code Diff Evidence`). The packet's OWN load-bearing claim — serialized neighbor safety (BR-038-003-002) — is a non-fabricated current-session proof: reverting only the collision-resistant per-run search term forces the cross-feature neighbor RED (`NEIGHBOR_RED_EXIT=1`) while the observability successor and every other Drive scenario stay GREEN on the same stack, and the byte-exact `git checkout HEAD` restore returns the whole Drive package GREEN (`NEIGHBOR_GREEN_EXIT=0`). The reconciliation working tree is packet-only (git status clean after byte-exact restore), so no foreign files, specs, or worktrees were touched (good-neighbor); zero leaked smackerel-test containers verified after teardown. No production runtime, sleep, mock, or weakened assertion was introduced (reality-scan `REALITY_EXIT=0`, RQG standard+bugfix `0 violations` with adversarial signal). The environmentally-flaky BUG-031-009-owned reaping regression, the foreign golang:1.26 container, the pre-existing leaked canary, and the opt-in Ollama agent E2E skip are dispositioned honestly in Discovered Issues (DI-038-003-01/02/03, G095) with no product regression and no cross-packet overreach.

## Parent Consolidation Reference

Parent consolidation should cite this report's Current-Session Neighbor-Safety Revert-Reverify (RED/GREEN), the full Drive package GREEN, the git-backed Code Diff Evidence, and the final pushed promotion commit — not the inherited routing excerpt above.
