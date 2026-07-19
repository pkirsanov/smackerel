# Report: BUG-026-008 Bounded synthesis schema repair

## Summary

The accepted runtime exposed a parsed JSON extraction response that omitted required `concepts`. Source tracing confirmed that `handle_extract` returns permanent failure immediately after its first schema validation error. This report records only evidence executed from the isolated bug worktree at the pinned baseline and later reconciled remote head.

## Completion Statement

The source/config/test/doc implementation is delivered and the scenario-specific regression is red-to-green. The bug remains `in_progress`: validate-owned certification and audit have not run in this runtime, the broader full-unit lane exposed routed linked-worktree infrastructure failures, and deployment is intentionally not performed here.

## RED: Bug Reproduction - Before Fix

**Phase:** bug
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1
**Claim Source:** executed

```text
......F..................................................                [100%]
=================================== FAILURES ===================================
______________ test_handle_extract_repairs_missing_concepts_once _______________
>       assert mock_litellm.acompletion.await_count == 2, result
E       AssertionError: {'artifact_id': 'card-rewards-artifact', 'success': False,
E       'error': "Schema validation failed: Schema validation failed: 'concepts' is a required property", ...}
E       assert 1 == 2
ml/tests/test_synthesis.py:225: AssertionError
ERROR smackerel-ml.synthesis Extraction output failed schema validation:
Schema validation failed: 'concepts' is a required property
FAILED ml/tests/test_synthesis.py::test_handle_extract_repairs_missing_concepts_once
1 failed, 630 passed, 2 skipped in 14.11s
```

The red proves the original handler made one call and returned permanent failure for the accepted-runtime missing-`concepts` shape.

## Test Evidence

### GREEN: Post-Fix Python Unit Lane

**Phase:** bug
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 0
**Claim Source:** executed

```text
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.......................................................s................ [ 22%]
........................................................................ [ 33%]
........................................................................ [ 44%]
........................................................................ [ 55%]
........................................................................ [ 66%]
........................................................................ [ 77%]
........................................................................ [ 88%]
........................................................................ [ 99%]
...                                                                      [100%]
649 passed, 2 skipped in 13.09s
[py-unit] pytest ml/tests finished OK
BUG026008_TRACE_PYTHON_END exit=0
```

The two skips are repository opt-in live-Ollama/deadletter tests; every changed BUG-026-008 test executed. The lane also contains BUG-026-006 malformed-JSON preservation and BUG-026-007 thinking/profile tests.

### Focused Go Response Contract

**Phase:** bug
**Command:** `./smackerel.sh test unit --go --go-run 'TestSynthesisExtractResponse_(FullPipelinePayload|FailureMarksFailed|SuccessMarksCompleted)' --verbose`
**Exit Code:** 0
**Claim Source:** executed

```text
=== RUN   TestSynthesisExtractResponse_SuccessMarksCompleted
--- PASS: TestSynthesisExtractResponse_SuccessMarksCompleted (0.00s)
=== RUN   TestSynthesisExtractResponse_FailureMarksFailed
--- PASS: TestSynthesisExtractResponse_FailureMarksFailed (0.00s)
=== RUN   TestSynthesisExtractResponse_FullPipelinePayload
--- PASS: TestSynthesisExtractResponse_FullPipelinePayload (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/pipeline        0.035s
[go-unit] go test ./... finished OK
BUG026008_GO_RESPONSE_CONTRACT_END exit=0
```

### Live Disposable-Stack Synthesis Round Trip

**Phase:** bug
**Command:** `./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
**Exit Code:** 0
**Claim Source:** executed

```text
Smackerel pre-flight resource check: OK
Preparing disposable test stack...
Building disposable test stack images before up (freshness convention)...
Container smackerel-test-smackerel-core-1  Healthy
Container smackerel-test-smackerel-ml-1  Healthy
go-e2e: applying -run selector: TestKnowledgeSynthesis_PipelineRoundTrip
=== RUN   TestKnowledgeSynthesis_PipelineRoundTrip
knowledge_synthesis_test.go:115: capture response: 200 {"artifact_id":"01KXX7WEBFXT51DNF50CGPSKZ8",...}
knowledge_synthesis_test.go:171: synthesis stats: completed=0 pending=3 failed=3 total=6
--- PASS: TestKnowledgeSynthesis_PipelineRoundTrip (6.07s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        6.204s
PASS: go-e2e
Volume smackerel-test-nats-data  Removed
Volume smackerel-test-ollama-data  Removed
Volume smackerel-test-postgres-data  Removed
Network smackerel-test_default  Removed
BUG026008_GO_SYNTHESIS_E2E_END exit=0
```

### Adversarial Regression Guards

**Phase:** bug
**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_synthesis.py` and the same guard for `ml/tests/test_main.py ml/tests/test_ollama_keepalive.py`
**Exit Code:** 0
**Claim Source:** executed

```text
BUBBLES REGRESSION QUALITY GUARD
Bugfix mode: true
Scanning ml/tests/test_synthesis.py
Adversarial signal detected in ml/tests/test_synthesis.py
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 1
Files with adversarial signals: 1
Scanning ml/tests/test_main.py
Adversarial signal detected in ml/tests/test_main.py
Scanning ml/tests/test_ollama_keepalive.py
Adversarial signal detected in ml/tests/test_ollama_keepalive.py
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 2
Files with adversarial signals: 2
BUG026008_REGRESSION_GUARD_CONFIG_END exit=0
```

## Validation Evidence

### SST Check

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=<validated-local-tier> ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
BUG026008_FINAL_CHECK_START
<repo-root>
4b28bb9f0c2cc3a48ab78aa04395ebe817c50864
config-validate: <repo-root>/config/generated/dev.env.tmp.2444908 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
BUG026008_FINAL_CHECK_END exit=0
```

### Lint

**Phase:** bug
**Command:** `./smackerel.sh lint`
**Exit Code:** 0
**Claim Source:** executed

```text
All checks passed!
=== Validating web manifests ===
	OK: web/pwa/manifest.json
	OK: PWA manifest has required fields
	OK: web/extension/manifest.json
	OK: Chrome extension manifest has required fields (MV3)
	OK: web/extension/manifest.firefox.json
	OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
	OK: web/pwa/app.js
	OK: web/pwa/sw.js
	OK: web/pwa/lib/queue.js
	OK: web/extension/background.js
	OK: web/extension/popup/popup.js
	OK: web/extension/lib/queue.js
	OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
	OK: Extension versions match (1.0.0)
Web validation passed
BUG026008_FINAL_LINT_END exit=0
```

### Format Classification

**Phase:** bug
**Command:** `./smackerel.sh format --check` followed by `git diff --exit-code origin/main -- internal/config/release_trains_contract_test.go` and `git diff --check`
**Exit Code:** 1 for repo-wide format; 0 for baseline parity and active-diff whitespace
**Claim Source:** executed

```text
BUG026008_FORMAT_CHECK_START
<repo-root>
4b28bb9f0c2cc3a48ab78aa04395ebe817c50864
internal/config/release_trains_contract_test.go
BUG026008_FORMAT_CHECK_END exit=1
BUG026008_FORMAT_BASELINE_PROOF_START
unchanged_format_file=true
diff_check_exit=0
BUG026008_FORMAT_BASELINE_PROOF_END exit=0
```

### Governance Gates

**Phase:** bug
**Command:** `artifact-lint.sh`, `traceability-guard.sh`, `implementation-reality-scan.sh --verbose`, and bugfix regression guards against the BUG-026-008 packet/tests
**Exit Code:** 0 for every listed gate
**Claim Source:** executed

```text
BUG026008_PRECOMMIT_ARTIFACT_START
Artifact lint PASSED.
BUG026008_PRECOMMIT_ARTIFACT_END exit=0
BUG026008_PRECOMMIT_TRACE_START
scenario-manifest.json covers 7 scenario contract(s)
Scenarios checked: 7
Scenario-to-row mappings: 7
Concrete test file references: 7
Report evidence references: 7
DoD fidelity scenarios: 7 (mapped: 7, unmapped: 0)
RESULT: PASSED (0 warnings)
BUG026008_PRECOMMIT_TRACE_END exit=0
BUG026008_REALITY_WARNING_FREE_START
Files scanned:  13
Violations:     0
Warnings:       0
PASSED: No source code reality violations detected
BUG026008_REALITY_WARNING_FREE_END exit=0
```

The state-transition guard was also executed honestly at the done ceiling and returned nonzero because all DoD items, specialist phases, validate/audit certification, and final closure remain unset. It explicitly required that `state.json` stay non-terminal; this packet complies and routes the next phase to `bubbles.validate`.

### Post-Rebase Combined-Head Verification

**Phase:** bug
**Command:** `./smackerel.sh test unit --python`, focused Go synthesis response tests, `./smackerel.sh check`, `./smackerel.sh lint`, artifact lint, traceability guard, and implementation-reality scan after rebasing onto `origin/main` `8cd13fff`
**Exit Code:** 0 for every listed command
**Claim Source:** executed

```text
BUG026008_POST_REBASE_PYTHON_START
head=e2ac9405b453698b5325b4b92f8b9cab4bd3cc35
+ pytest ml/tests -q
s....................................................................... [ 10%]
.......................................................s................ [ 20%]
........................................................................ [ 30%]
........................................................................ [ 40%]
........................................................................ [ 50%]
........................................................................ [ 60%]
........................................................................ [ 70%]
........................................................................ [ 81%]
........................................................................ [ 91%]
..............................................................           [100%]
708 passed, 2 skipped in 12.70s
BUG026008_POST_REBASE_PYTHON_END exit=0
TestSynthesisExtractResponse_SuccessMarksCompleted PASS
TestSynthesisExtractResponse_FailureMarksFailed PASS
TestSynthesisExtractResponse_FullPipelinePayload PASS
BUG026008_POST_REBASE_GO_END exit=0
Config is in sync with SST
scenario-lint: OK
BUG026008_POST_REBASE_CHECK_END exit=0
Web validation passed
BUG026008_POST_REBASE_LINT_END exit=0
Artifact lint PASSED.
Traceability RESULT: PASSED (0 warnings)
Implementation reality: 13 files, 0 violations, 0 warnings
```

### Code Diff Evidence

Runtime/config/contract/test/doc delta:

- `ml/app/synthesis.py`: one schema-guided correction, profile/context/trace retention, summed accounting, content-free terminal errors.
- `config/smackerel.yaml`, `scripts/commands/config.sh`, `ml/app/main.py`: fail-loud exact-one retry budget.
- `ml/app/metrics.py`: zero-label repair-attempt counter.
- `internal/pipeline/synthesis_types.go`: additive response `trace_id`.
- `ml/tests/test_synthesis.py`, `ml/tests/test_main.py`, `ml/tests/test_ollama_keepalive.py`, `internal/pipeline/synthesis_subscriber_test.go`, and fixture: adversarial contract coverage.
- `docs/Development.md`: operator-facing SST contract.

## Outcome Contract Evidence

The red shows one-call permanent failure; the green shows invalid-then-valid succeeds in exactly two calls. Adversarial tests cover invalid twice, malformed repair, repair exception, valid-first one call, profile/context/trace retention, summed tokens/time, content-free errors/logs, and no empty required-semantic normalization. The Go E2E proves the live disposable synthesis pipeline still progresses and cleans up.

## Discovered Issues

| Observed | Disposition | Artifact |
|---|---|---|
| Parsed schema-invalid synthesis output is terminal after one LLM call | `bug-filed` | `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/bug.md` |
| Targeted shell E2E marks the stack parent-managed but does not boot it, then times out because PostgreSQL is not running | `routed` to `bubbles.test` / spec 031 test-infrastructure owner | `state.json` transition request `TR-026-008-TEST-001` |
| Dockerized full-unit tests cannot resolve a linked-worktree `.git` pointer because only the worktree path is mounted | `routed` to `bubbles.test` / spec 031 test-infrastructure owner | `state.json` transition request `TR-026-008-TEST-002` |
| The repo CLI's full Python lane always collects two explicit opt-in live tests as skips, so this session cannot claim a literal zero-skip suite even though every changed BUG-026-008 test executed | `routed` to `bubbles.test` for a no-skip required-test selector or explicit category isolation | `state.json` transition request `TR-026-008-TEST-003` |
| Repo-wide format check names `internal/config/release_trains_contract_test.go`, byte-identical to current baseline | `routed` to repository quality owner | `state.json` transition request `TR-026-008-QUALITY-001` |
| Source/test/doc delivery awaits independent certification; no validate/audit specialist invocation tool exists in this runtime | `routed` to `bubbles.validate` then `bubbles.audit` | `state.json` transition request `TR-026-008-VALIDATE-001` |
| Pushed source must be built and deployed through the signed delivery path before live accepted-runtime confirmation | `routed` to `bubbles.devops` | `state.json` transition request `TR-026-008-DEVOPS-001` |

## Interrupted Test Closeout Evidence - 2026-07-19

### Harness And Category Repairs

**Executed:** YES (current session, recovered from the interrupted terminal resources)
**Commands:** `./smackerel.sh --env test test e2e --shell-run test_synthesis.sh`; `bash tests/unit/cli/linked_worktree_tooling_mount_test.sh`; `./smackerel.sh test unit --go`; `./smackerel.sh test unit --python --python-k 'crosssource or schema_repair or malformed_json or structured_extraction_thinking or output_token_budget'`; `./smackerel.sh test unit --python`; `./smackerel.sh --env test test integration`
**Exit Code:** 0 for every listed command
**Claim Source:** executed

```text
Container smackerel-test-postgres-1 Healthy
Container smackerel-test-smackerel-core-1 Healthy
Container smackerel-test-smackerel-ml-1 Healthy
=== Synthesis Engine E2E ===
Services healthy after 0s
PASS: Intelligence tables exist
PASS: SCN-004-001: Cross-domain cluster detected (size=3)
PASS: test_synthesis.sh
Total: 1
Passed: 1
Failed: 0
PASS: linked worktree tooling mounts common Git metadata read-only
[go-unit] go test ./... finished OK
78 passed, 632 deselected in 1.11s
708 passed, 2 deselected in 13.95s
PASS: go-integration
[py-integration] pip install OK; starting live integration pytest
.                                                                        [100%]
1 passed in 0.48s
[py-integration] live integration pytest finished OK
PASS: python-integration
```

The selector and full Python lane report deselection, not skipping: required live integration and external-Ollama categories are now excluded from the unit collection and the dead-letter test runs in the canonical integration lane. This directly addresses the behavioral substance of `TR-026-008-TEST-001`, `TR-026-008-TEST-002`, and `TR-026-008-TEST-003`; state accounting is updated only after the focused closeout checks rerun.

### Focused Live Regression And Retirement Isolation

**Executed:** YES (current session, recovered from the interrupted terminal resources)
**Commands:** `./smackerel.sh --env test test e2e --go-run 'TestKnowledge(Synthesis_PipelineRoundTrip|CrossSource_ConnectionDetection)'`; `./smackerel.sh --env test test e2e --go-run TestLegacyRetirement_FullScenarioMatrix`
**Exit Code:** 0 for both commands
**Claim Source:** executed

```text
go-e2e: applying -run selector: TestKnowledge(Synthesis_PipelineRoundTrip|CrossSource_ConnectionDetection)
=== RUN   TestKnowledgeCrossSource_ConnectionDetection
--- PASS: TestKnowledgeCrossSource_ConnectionDetection (0.01s)
=== RUN   TestKnowledgeSynthesis_PipelineRoundTrip
--- PASS: TestKnowledgeSynthesis_PipelineRoundTrip (8.03s)
PASS
ok github.com/smackerel/smackerel/tests/e2e 8.150s
PASS: go-e2e
go-e2e: applying -run selector: TestLegacyRetirement_FullScenarioMatrix
=== RUN   TestLegacyRetirement_FullScenarioMatrix
--- PASS: TestLegacyRetirement_FullScenarioMatrix (0.15s)
--- PASS: TestLegacyRetirement_FullScenarioMatrix/A01_FirstWeatherShowsNoticeAndServesBody (0.02s)
--- PASS: TestLegacyRetirement_FullScenarioMatrix/A02_SecondWeatherDoesNotRenotify (0.02s)
--- PASS: TestLegacyRetirement_FullScenarioMatrix/A03_RemindProducesIndependentNotice (0.01s)
--- PASS: TestLegacyRetirement_FullScenarioMatrix/A04_ResidualMetricRegistered (0.00s)
PASS
ok github.com/smackerel/smackerel/tests/e2e/legacy_retirement 0.190s
PASS: go-e2e
```

The cross-source live test remains collateral coverage because it accepts an empty concept store. It is not used to overstate exact outgoing-validation routing, which is covered by the focused Python dispatch regression.

### Broad E2E Is RED

**Executed:** YES (current session, recovered from the saved terminal resource)
**Command:** `./smackerel.sh --env test test e2e`
**Exit Code:** nonzero; the exact numeric exit was not retained after terminal scrollback truncation
**Claim Source:** executed

```text
--- FAIL: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (0.03s)
web_pwa_chat_e2e_test.go:107: assistant.js must not reference forbidden auth surface "localStorage" (SCN-073-A11)
--- FAIL: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.02s)
web_pwa_retry_e2e_test.go:67: trace.assistant_turn_id must be non-empty: first="" second=""
--- FAIL: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (0.17s)
web_pwa_retry_e2e_test.go:89: trace.assistant_turn_id must be non-empty: a="" b=""
--- FAIL: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (0.01s)
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
drive scan: completed provider=google seen=1 indexed=1 skipped=0
drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
drive_cross_feature_e2e_test.go:147: /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (3.87s)
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
drive_observability_e2e_test.go:48: e2e: services not healthy after 2m0s at http://smackerel-core:8080
--- FAIL: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (121.94s)
drive_policy_e2e_test.go:38: e2e: services not healthy after 30s at http://smackerel-core:8080
drive_scan_e2e_test.go:17: waitForHealth
FAIL github.com/smackerel/smackerel/tests/e2e/drive 300.054s
spec_076_migrations_e2e_test.go:59: lookup postgres on 127.0.0.11:53: no such host
FAIL github.com/smackerel/smackerel/tests/e2e/foundation 0.056s
```

**Uncertainty Declaration:** the named failures and timings above were retained verbatim, while the command header/footer was lost to terminal scrollback. The run is definitively RED. No broad-E2E pass or exact numeric exit claim is made.

### Independent Broad Findings Routed Out Of Packet

| Finding ID | Independent observation | Interpretation | Route |
|---|---|---|---|
| `BROAD-ASSISTANT-TRANSPORT-001` | Web/mobile transport-hint parity failed before the Drive package ran. | The E2E compares two state-mutating `/reset` calls even though the current adapter contract records the hint in telemetry-only metadata; the test appears stale or state-contaminated. | `bubbles.bug`, likely spec 073 / assistant ownership |
| `BROAD-ASSISTANT-PWA-SCAN-001` | The served-JS test rejected the literal token `localStorage`. | `web/pwa/assistant.js` contains the token only in a prohibition comment, so the raw substring scanner is a false positive. | `bubbles.bug`, likely spec 073 / assistant ownership |
| `BROAD-ASSISTANT-RETRY-001` | Both retry tests received empty `trace.assistant_turn_id` values. | The tests use `/reset`, whose short-circuit response carries empty trace fields; shared context-reset state may also contaminate sequential cases. | `bubbles.bug`, likely spec 073 / assistant ownership |
| `BROAD-DRIVE-SEARCH-001` | Google and memdrive scans each indexed one row, but `/api/search` returned neither expected provider row. | This is an independent cross-feature search failure; later stack loss does not explain it. | `bubbles.bug`, Drive/search ownership |
| `BROAD-DRIVE-HEALTH-001` | The Drive observability test independently waited two minutes and observed core unhealthy. | Investigate whether the observability fixture makes core unhealthy or exposes an existing health collapse. | `bubbles.bug`, Drive/observability ownership |

All Drive, foundation, retirement, transport, and wiki failures after the core/container-network disappearance are classified as cascade noise until one of the independent health findings is reproduced in its owning packet. The two broader-E2E DoD items remain unchecked.

### Final Cheap Closeout Checks

**Executed:** YES (current session)
**Commands:** targeted ShellCheck/shfmt and both CLI contracts; `./smackerel.sh format --check`; `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `./smackerel.sh lint`; packet artifact/traceability/reality/regression guards
**Exit Code:** 0 for every listed check
**Claim Source:** executed

```text
=== SHELLCHECK SUPPORT FILES PASS ===
=== SHELLCHECK CLI PASS ===
=== SHFMT NEW FILES PASS ===
=== SHFMT CHANGED FILES PARSE PASS ===
=== LINKED WORKTREE CONTRACT PASS ===
=== SYNTHESIS HARNESS CONTRACT PASS ===
=== REPO FORMAT CHECK PASS ===
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
=== REPO CHECK PASS ===
All checks passed!
Web validation passed
=== REPO LINT PASS ===
Artifact lint PASSED.
Traceability RESULT: PASSED (0 warnings)
Implementation reality: 13 files, 0 violations, 0 warnings
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
=== BUG-026 REGRESSION PASS ===
```

These checks close the test-infrastructure and format requests only. The broad E2E result remains RED, and validate-owned certification remains unchanged.

## Invocation Audit

No subagent invocation API is available in this runtime. The authorized top-level `bubbles.bug` runner uses Smackerel's recorded parent-expanded phase convention and records each phase's real command provenance separately; it does not claim a `runSubagent` call occurred.
