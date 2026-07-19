# Execution Report: BUG-025-005 Cross-Source Response Validation Routing

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

- Bug packet initialized from clean isolated worktree commit `4b28bb9f0c2cc3a48ab78aa04395ebe817c50864` on 2026-07-19.
- Accepted live stabilization facts and direct source inspection identify contract misrouting and swallowed outgoing-validation failures.
- Completion and certification are not claimed. Red, green, integration, regression, quality, and governance evidence will be appended only after execution.

## Bug Reproduction - Before Fix

### Accepted Live Observation

**Claim Source:** interpreted

The user supplied a dated stabilization observation from source `a7ce`: valid `synthesis.crosssource` results emitted false `artifact_id required` validation errors and were then published. Current source at `4b28bb9f0c2cc3a48ab78aa04395ebe817c50864` retains the same routing and catch-and-publish path. This observation is context, not a substitute for the required current-session failing regression command.

### Pre-Fix Regression Test

**Executed:** YES (current session)
**Command:** `./smackerel.sh --env dev test unit --python`
**Exit Code:** 1 (expected RED)
**Claim Source:** executed

```text
=================================== FAILURES ===================================
___________ test_crosssource_dispatch_accepts_valid_concept_response ___________
>       artifact_validator.assert_not_called()
E       AssertionError: Expected 'validate_processed_result' to not have been called. Called 1 times.
E       Calls: [call({'concept_id': 'concept-1', 'has_genuine_connection': True,
E       'insight_text': 'Two independent sources describe one decision.',
E       'confidence': 0.91, 'artifact_ids': ['artifact-1', 'artifact-2'],
E       'prompt_contract_version': 'cross-source-connection-v1',
E       'processing_time_ms': 0, 'model_used': 'test-model'})].
FAILED ml/tests/test_nats_client.py::test_crosssource_dispatch_accepts_valid_concept_response
1 failed, 630 passed, 2 skipped in 13.12s
```

**Result:** PASS for the red-stage claim: the focused test failed because the generic artifact validator was selected for a valid concept response.

## Code Diff Evidence

Changed runtime and regression surfaces:

- `ml/app/validation.py` - strict `CrossSourceResponse` field validation.
- `ml/app/nats_client.py` - closed subject-to-validation-mode dispatch and fail-loud validation before publish/ack.
- `ml/tests/test_validation.py` - field type, range, finite-number, artifact-list, prompt, timing, and model adversaries.
- `ml/tests/test_nats_client.py` - actual consumer-loop red/green, poison/nak, artifact, digest, photo, and unknown-subject regressions.

**Claim Source:** interpreted from the current working-tree diff; final git-backed command evidence is recorded before commit.

## Test Evidence

### Unit

**Executed:** YES (current session)
**Command:** `./smackerel.sh --env dev test unit --python`
**Exit Code:** 0
**Claim Source:** executed

```text
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 10%]
.......................................................s................ [ 20%]
........................................................................ [ 31%]
........................................................................ [ 41%]
........................................................................ [ 52%]
........................................................................ [ 62%]
........................................................................ [ 72%]
........................................................................ [ 83%]
........................................................................ [ 93%]
............................................                             [100%]
689 passed, 2 skipped in 13.44s
[py-unit] pytest ml/tests finished OK
BUG025005_STRONG_FINAL_PYUNIT_END exit=0
```

The concrete scenario tests in `ml/tests/test_nats_client.py` execute the real `handle_crosssource` handler and real `_handle_poison` retry branch. Only the external LiteLLM completion is replaced. Strict field permutations are in `ml/tests/test_validation.py`.

### Integration

**Claim Source:** not-run

Uncertainty Declaration: `./smackerel.sh --env test test integration` was invoked, but concurrent workspace terminal input delivered SIGINT before the marked command produced an admissible result. No integration pass/fail claim is made from that capture. The focused live cross-source E2E lane below completed on the ephemeral stack and is the only live-system result claimed.

### E2E Regression

**Executed:** YES (current session)
**Command:** `./smackerel.sh --env test test e2e --go-run TestKnowledgeCrossSource_ConnectionDetection`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying -run selector: TestKnowledgeCrossSource_ConnectionDetection
=== RUN   TestKnowledgeCrossSource_ConnectionDetection
	knowledge_crosssource_test.go:48: total concepts: 0, multi-source: 0
--- PASS: TestKnowledgeCrossSource_ConnectionDetection (1.51s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        1.690s
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
BUG025005_E2E_CACHED_RETRY_END exit=0
```

This is collateral live API coverage only: the existing test permits an empty concept store. Exact validation-routing behavior is proven by the actual `_consume_loop` regression above, not by overstating this E2E assertion.

### Lint And Format

**Executed:** YES (current session)
**Commands:** `./smackerel.sh lint`, `./smackerel.sh check`, `./smackerel.sh format --check`
**Exit Codes:** lint `0`; check `0`; format `1`
**Claim Source:** executed

```text
All checks passed!
=== Validating web manifests ===
	OK: web/pwa/manifest.json
	OK: PWA manifest has required fields
	OK: web/extension/manifest.json
	OK: Chrome extension manifest has required fields (MV3)
=== Validating JS syntax ===
	OK: web/pwa/app.js
	OK: web/pwa/sw.js
Web validation passed
BUG025005_STRONG_FINAL_LINT_END exit=0
config-validate: config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
BUG025005_CHECK_END exit=0
```

The repo-wide format check names only `internal/config/release_trains_contract_test.go` and exits `1`. `git diff --exit-code origin/main -- internal/config/release_trains_contract_test.go` exits `0`, proving that finding is unchanged from the requested baseline and outside this bug's declared boundary. No format-pass claim is made.

### Governance Gates

**Executed:** YES (current session)
**Claim Source:** executed

```text
BUBBLES REGRESSION QUALITY GUARD
Bugfix mode: true
Scanning ml/tests/test_nats_client.py
Adversarial signal detected in ml/tests/test_nats_client.py
Scanning ml/tests/test_validation.py
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 2
Files with adversarial signals: 1
BUG025005_STRONG_FINAL_REGRESSION_END exit=0
```

```text
BUBBLES TRACEABILITY GUARD
scenario-manifest.json covers 3 scenario contract(s)
All linked tests from scenario-manifest.json exist
Scope 1 scenario mapped to Test Plan row: Valid concept response follows the cross-source validator
Scope 1 scenario mapped to Test Plan row: Malformed concept response enters poison handling before publish
Scope 1 scenario mapped to Test Plan row: Neighboring subject semantics remain intact
BUG025005_TRACEABILITY_DETACHED_END exit=0
```

```text
IMPLEMENTATION REALITY SCAN RESULT
Resolved 4 implementation file(s) to scan
Scan 1: Gateway/Backend Stub Patterns
Scan 1B: Handler / Endpoint Execution Depth
Scan 1D: External Integration Authenticity
Scan 5: Default/Fallback Value Patterns
Scan 6: Live-System Test Interception
Files scanned: 4
Violations: 0
Warnings: 0
BUG025005_REALITY_RETRY_END exit=0
```

### Change Boundary And Teardown

**Executed:** YES (current session)
**Commands:** `git diff --check && git diff --stat && git status --short --untracked-files=all`; `docker ps -a --format '{{.Names}} {{.Status}}'`
**Exit Code:** 0
**Claim Source:** executed

```text
ml/app/nats_client.py        |  86 ++++++++++++-----
ml/app/validation.py         |  52 ++++++++++
ml/tests/test_nats_client.py | 219 +++++++++++++++++++++++++++++++++++++++++++
ml/tests/test_validation.py  | 100 ++++++++++++++++++++
4 files changed, 435 insertions(+), 22 deletions(-)
M ml/app/nats_client.py
M ml/app/validation.py
M ml/tests/test_nats_client.py
M ml/tests/test_validation.py
?? specs/025-knowledge-synthesis-layer/bugs/BUG-025-005-crosssource-response-validation-routing/
BUG025005_DIFF_AUDIT_END exit=0
BUG025005_STACK_FINAL_START
BUG025005_STACK_FINAL_END
```

## Documentation

The parent feature design already documents the exact `CrossSourceResponse` wire shape and confidence semantics. No runtime contract document changed; this bug packet records the repaired validation and poison-routing behavior.

## Ownership And Certification

- `bubbles.bug` directly ran its authorized persisted `bugfix-fastlane` workflow because no `runSubagent` capability is exposed in this session.
- Source and tests were implemented and executed, but no specialist invocation is fabricated in `state.json`.
- Validate-owned certification remains `in_progress` pending the routed security, validate, and audit chain.
- No deployment was performed.

## Completion Statement

BUG-025-005 is fixed in the isolated worktree and remains `in_progress` for certification. Validate-owned certification remains `in_progress`. No deployment is authorized by this packet.

## Invocation Audit

No subagents were invoked because no `runSubagent` capability is exposed in this session. The top-level `bubbles.bug` runtime is authorized for persisted `bugfix-fastlane`, but the invocation ledger does not impersonate `bubbles.implement`, `bubbles.test`, `bubbles.security`, `bubbles.validate`, or `bubbles.audit`. Certification is not self-asserted.