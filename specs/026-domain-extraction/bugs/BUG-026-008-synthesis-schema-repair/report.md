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

## Invocation Audit

No subagent invocation API is available in this runtime. The authorized top-level `bubbles.bug` runner uses Smackerel's recorded parent-expanded phase convention and records each phase's real command provenance separately; it does not claim a `runSubagent` call occurred.
