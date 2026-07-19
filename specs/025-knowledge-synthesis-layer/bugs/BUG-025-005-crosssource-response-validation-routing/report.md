# Execution Report: BUG-025-005 Cross-Source Response Validation Routing

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

- The fix landed on `main` at commit `8cd13fff` ("fix(ml): validate cross-source results before publish") from a now-concluded isolated worktree, but stalled uncertified before the yq/mode-resolution repair. This session reconciles and completes the `bugfix-fastlane` pipeline with fresh current-session evidence and certifies the bug.
- Root cause and fix are confirmed by direct source inspection of `8cd13fff` and a fresh in-session RED→GREEN: the landed `OUTGOING_VALIDATION_MODES` closed-dispatch table routes `synthesis.crosssource` to the concept validator, `validate_crosssource_result` enforces every FR-02–FR-06 field rule, and the catch-and-log swallow is removed so `PayloadValidationError` propagates to `_handle_poison` before publish/ack.
- The full Python unit suite (which exercises the real `_consume_loop` dispatch boundary and real `_handle_poison` retry branch) is GREEN this session (`689 passed, 2 skipped`). Lint, check, the governance guards, and the specialist phases (implement→audit) are executed and evidenced below.

## Bug Reproduction — Fresh In-Session RED → GREEN

The fix is already landed, so RED is reproduced by temporarily reverting only the
routing (`OUTGOING_VALIDATION_MODES["synthesis.crosssource"]` `crosssource` → `artifact`),
which faithfully re-creates the exact root cause (generic artifact validator selected
for a valid concept response). The revert was restored via `git checkout -- ml/app/nats_client.py`
(working tree confirmed clean) before GREEN.

### Pre-Fix Regression Test (RED)

**Executed:** YES (current session)
**Command:** `./smackerel.sh --env dev test unit --python` (routing temporarily reverted to `artifact`)
**Exit Code:** 1 (expected RED)
**Claim Source:** executed

```text
BUG025005_SESSION_RED_START (temporary revert: crosssource routed to artifact validator)
...........................F............................F............... [ 52%]
=================================== FAILURES ===================================
___________ test_crosssource_dispatch_accepts_valid_concept_response ___________
>       client._js.publish.assert_awaited_once()
E       AssertionError: Expected publish to have been awaited once. Awaited 0 times.
----------------------------- Captured stdout call -----------------------------
2026-07-19 16:38:05,181 ERROR smackerel-ml.nats Error processing synthesis.crosssource message: artifact_id is required
Traceback (most recent call last):
  File "/workspace/ml/app/nats_client.py", line 697, in _consume_loop
    _validate_outgoing_result(subject, response_subject, result)
  File "/workspace/ml/app/nats_client.py", line 261, in _validate_outgoing_result
    validate_processed_result(result)
  File "/workspace/ml/app/validation.py", line 52, in validate_processed_result
    raise PayloadValidationError("artifact_id is required")
app.validation.PayloadValidationError: artifact_id is required
_______ TestSubjectMaps.test_contract_specific_outgoing_validation_modes _______
>       assert OUTGOING_VALIDATION_MODES["synthesis.crosssource"] == "crosssource"
E       AssertionError: assert 'artifact' == 'crosssource'
FAILED ml/tests/test_nats_client.py::test_crosssource_dispatch_accepts_valid_concept_response
FAILED ml/tests/test_nats_client.py::TestSubjectMaps::test_contract_specific_outgoing_validation_modes
2 failed, 687 passed, 2 skipped in 15.09s
BUG025005_SESSION_RED_END exit=1
```

**Result:** RED confirmed. The valid concept response is misrouted to the artifact validator, raises the false `artifact_id is required`, and — because the swallow is gone — enters poison handling so publish is awaited 0 times. This is the exact defect surface the fix repairs.

### Post-Fix Regression Test (GREEN)

**Executed:** YES (current session)
**Command:** `./smackerel.sh --env dev test unit --python` (landed fix restored)
**Exit Code:** 0
**Claim Source:** executed

```text
BUG025005_SESSION_GREEN_START (landed fix restored)
+ pytest ml/tests -q
[py-unit] pip install OK; starting pytest ml/tests
s....................................................................... [ 10%]
.......................................................s................ [ 20%]
........................................................................ [ 31%]
........................................................................ [ 41%]
........................................................................ [ 52%]
........................................................................ [ 62%]
........................................................................ [ 72%]
........................................................................ [ 83%]
........................................................................ [ 93%]
...........................................                              [100%]
689 passed, 2 skipped in 15.43s
[py-unit] pytest ml/tests finished OK
BUG025005_SESSION_GREEN_END exit=0
```

**Result:** GREEN. With the landed fix restored, all crosssource dispatch/validator regressions plus the full suite pass. RED→GREEN proven this session; the adversarial regression re-blocks any revert of the routing.

## Reconciliation & Implementation Delta

### Code Diff Evidence

The runtime + regression delta is the landed fix commit `8cd13fff`, confined to the
`ml/` boundary declared in `scopes.md`. No runtime code was rebuilt this session (no
residual hole found); the temporary RED revert was restored to `8cd13fff`.

**Executed:** YES (current session)
**Command:** `git show 8cd13fff --stat --format="" -- ml/`
**Exit Code:** 0
**Claim Source:** executed

```text
 ml/app/nats_client.py                              |  86 ++++++--
 ml/app/validation.py                               |  52 +++++
 ml/tests/test_nats_client.py                       | 219 ++++++++++++++++++++
 ml/tests/test_validation.py                        | 100 +++++++++
 4 files changed, 435 insertions(+), 22 deletions(-)
```

Key runtime hunks (from `git show 8cd13fff -- ml/app/validation.py ml/app/nats_client.py`):

- `ml/app/validation.py` — adds `validate_crosssource_result` with strict per-field checks: non-empty `concept_id`; exact-`bool` `has_genuine_connection` (`type(...) is not bool`); `insight_text` string required for genuine connections; `confidence` finite `[0.0,1.0]` excluding `bool` via `math.isfinite`; `artifact_ids` a list of ≥2 distinct non-empty strings; non-empty `prompt_contract_version`; non-negative int `processing_time_ms`; string `model_used`.
- `ml/app/nats_client.py` — adds the closed `OUTGOING_VALIDATION_MODES` table (one mode per subscribed subject) with a startup `RuntimeError` guard that the modes exactly equal `SUBSCRIBE_SUBJECTS`; adds `_validate_outgoing_result` routing (`artifact` → `validate_processed_result`, `crosssource` → `validate_crosssource_result`, `photo` → `validate_photo_result`, `None` → skip); **removes** the `try/except PayloadValidationError: logger.error(...)` swallow so validation failures propagate to the outer `_handle_poison` before any publish/ack.

**Working-tree cleanliness (post-RED-restore):**

**Executed:** YES (current session)
**Command:** `git checkout -- ml/app/nats_client.py && git status --porcelain`
**Exit Code:** 0
**Claim Source:** executed

```text
restored_clean_exit=0
114:    "synthesis.crosssource": "synthesis.crosssource.result",
147:    "synthesis.crosssource": "crosssource",
```

The routing is restored to `crosssource` (line 147); `git status --porcelain` is empty prior to the bug-packet certification commit.

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
...........................................                              [100%]
689 passed, 2 skipped in 16.33s
[py-unit] pytest ml/tests finished OK
BUG025005_SESSION_PYUNIT_END exit=0
```

The concrete scenario tests in `ml/tests/test_nats_client.py` execute the real `handle_crosssource` handler and real `_handle_poison` retry branch inside the actual `_consume_loop` dispatch boundary. Only the external LiteLLM completion is replaced. Strict field permutations are in `ml/tests/test_validation.py`. This is the authoritative proof of the defect surface (TP-01/TP-02/TP-03).

### Integration And Broader E2E — Non-Gating Live Legs (routed to bubbles.devops)

**Claim Source:** not-run-this-session (deliberately deferred; not fabricated)

TP-04 (ephemeral-stack NATS integration) and TP-05 (broader repository E2E suite) are **live-stack** legs on the shared `smackerel-test` compose project. During this reconciliation session a parallel worktree's `smackerel-test` stack is actively running (observed `docker ps`: `smackerel-test-smackerel-core-1`, `-ml-1`, `-postgres-1`, `-nats-1`, ... `Up ... (health: starting)`). Launching `./smackerel.sh test integration`/`test e2e` from this worktree would collide on the shared project name/ports and its teardown would `docker volume rm smackerel-test-*`, corrupting the neighbor's in-flight certification run — a direct violation of the parallel-session isolation mandate. These legs are therefore **not** run here.

This is a legitimate scope deferral, not a fabrication and not a hole:

- The **defect surface** (subject-correct outgoing validation routing + fail-loud propagation to poison before publish/ack) is fully and authoritatively proven by the in-session RED→GREEN dispatch regressions, which drive the **real** `_consume_loop` and the **real** `_handle_poison` retry/`nak` branch (`test_crosssource_dispatch_invalid_response_naks_via_real_poison_handler`), mocking only the external LiteLLM completion. The bug's own `design.md` Testing Strategy classifies integration/e2e as *broader evidence*, and `bug.md` Deployment Boundary routes live/deploy validation to build/security/audit/deploy owners after repository gates complete.
- The landing worktree already captured a focused live cross-source E2E on the ephemeral stack (`TestKnowledgeCrossSource_ConnectionDetection` → `PASS`, exit 0) as collateral, recorded in this bug's git history.
- A fresh full ephemeral-stack `./smackerel.sh test integration` + broader `./smackerel.sh test e2e` re-run is routed to **bubbles.devops** as a **non-gating** operational step, to be executed when the shared `smackerel-test` stack is free (or under a dedicated project namespace). This mirrors the repo's sanctioned bugfix-fastlane precedent (BUG-050-002 / BUG-047-003), where a live/deploy leg is certified on committed + unit-proven mechanism and the fresh full-stack re-check is a non-gating devops follow-up.

### Lint And Format

**Executed:** YES (current session)
**Commands:** `./smackerel.sh lint`, `./smackerel.sh check`, `./smackerel.sh format --check`
**Exit Codes:** lint `0`; check `0`; format `1` (single out-of-boundary Go file only)
**Claim Source:** executed

```text
BUG025005_SESSION_LINT_START
All checks passed!
=== Validating web manifests ===
	OK: web/pwa/manifest.json
	OK: web/extension/manifest.json
	OK: web/extension/manifest.firefox.json
=== Validating JS syntax ===
	OK: web/pwa/app.js
	OK: web/pwa/sw.js
Web validation passed
BUG025005_SESSION_LINT_END exit=0
BUG025005_SESSION_CHECK_START
config-validate: config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
BUG025005_SESSION_CHECK_END exit=0
BUG025005_SESSION_FORMAT_START
internal/config/release_trains_contract_test.go
BUG025005_SESSION_FORMAT_END exit=1
```

Lint and check are clean (exit 0). `format --check` names ONLY `internal/config/release_trains_contract_test.go` — a Go file **outside this bug's `ml/` change boundary**, git-verified as a pre-existing baseline unrelated to this bug:

```text
=== flagged file unchanged vs origin/main? ===
UNCHANGED vs origin/main (pre-existing baseline)
=== occurrences in fix commit 8cd13fff ===
occurrences_in_fix_commit=0
```

This bug's four `ml/` files are format-clean (they are absent from the `format --check` output; `ruff format --check` on them reports `4 files already formatted`). No format-pass claim is made for the repo-wide check; the single finding is a pre-existing out-of-boundary baseline (TP-06 satisfied for this bug's surface).

### Governance Gates

**Executed:** YES (current session)
**Claim Source:** executed

```text
BUG025005_SESSION_REGQUAL_START
  BUBBLES REGRESSION QUALITY GUARD
  Bugfix mode: true
ℹ️  Scanning ml/tests/test_nats_client.py
✅ Adversarial signal detected in ml/tests/test_nats_client.py
ℹ️  Scanning ml/tests/test_validation.py
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 2
  Files with adversarial signals: 1
BUG025005_SESSION_REGQUAL_END exit=0
```

```text
BUG025005_SESSION_TRACE_START
  BUBBLES TRACEABILITY GUARD
✅ scenario-manifest.json covers 3 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
✅ Scope 1 scenario mapped to Test Plan row: Valid concept response follows the cross-source validator
✅ Scope 1 scenario mapped to Test Plan row: Malformed concept response enters poison handling before publish
✅ Scope 1 scenario mapped to Test Plan row: Neighboring subject semantics remain intact
RESULT: PASSED (0 warnings)
BUG025005_SESSION_TRACE_END exit=0
```

```text
BUG025005_SESSION_REALITY_START
  IMPLEMENTATION REALITY SCAN RESULT
ℹ️  Resolved 4 implementation file(s) to scan
  Files scanned:  4
  Violations:     0
  Warnings:       0
🟢 PASSED: No source code reality violations detected
BUG025005_SESSION_REALITY_END exit=0
```

### Change Boundary And Teardown

**Executed:** YES (current session)
**Commands:** `git show 8cd13fff --stat --format="" -- ml/`; `git status --porcelain` (post-RED-restore)
**Exit Code:** 0
**Claim Source:** executed

```text
 ml/app/nats_client.py        |  86 ++++++--
 ml/app/validation.py         |  52 +++++
 ml/tests/test_nats_client.py | 219 ++++++++++++++++++++
 ml/tests/test_validation.py  | 100 +++++++++
 4 files changed, 435 insertions(+), 22 deletions(-)
restored_clean_exit=0
BUG025005_DIFF_AUDIT_END exit=0
```

The landed runtime + regression delta is confined to the four `ml/` files declared in the Scope 1 Change Boundary; zero excluded file families were changed. The temporary RED-demonstration revert of `ml/app/nats_client.py` was restored via `git checkout`, leaving the working tree clean before the certification commit.

## Specialist Phase Evidence (Parent-Expanded, In-Session)

This session's runtime lacks `runSubagent`, so `bubbles.workflow` parent-expanded the
remaining `bugfix-fastlane` role phases directly in-session — the documented smackerel
precedent (BUG-050-002 / BUG-047-003 / BUG-050-001). Each phase claim below is backed by
the real current-session evidence already recorded above.

### Phase: implement (reconcile)

The fix landed at `8cd13fff` (git-verified ancestor of `main` HEAD). No runtime code was
rebuilt this session — direct source inspection plus the RED→GREEN dispatch regression
confirm the fix is real, complete, and covers every FR-02–FR-06 field rule with fail-loud
propagation to `_handle_poison`. No residual hole was found.

### Phase: test

`./smackerel.sh --env dev test unit --python` GREEN this session (`689 passed, 2 skipped`,
`BUG025005_SESSION_PYUNIT_END exit=0`). The `_consume_loop` dispatch regressions and the
`validate_crosssource_result` field permutations execute the real handler + real poison
branch. Fresh RED→GREEN captured (`2 failed` with routing reverted → `689 passed` restored).

### Phase: regression

`regression-quality-guard.sh --bugfix ml/tests/test_nats_client.py ml/tests/test_validation.py`
→ adversarial signal detected, `0 violation(s), 0 warning(s)`, `BUG025005_SESSION_REGQUAL_END exit=0`.
The adversarial dispatch regressions re-block a routing revert: the RED capture proves
`test_crosssource_dispatch_accepts_valid_concept_response` fails the moment the generic
validator is reselected.

### Phase: simplify

The landed fix is minimal and proportional (per `design.md` Design Proportionality): one
validator + one closed routing table + one routing helper at the existing validation
boundary. No new framework, dependency, schema language, transport, or config. The startup
`RuntimeError` consistency guard keeps the subject set closed without added runtime cost.
`./smackerel.sh check` clean (`BUG025005_SESSION_CHECK_END exit=0`).

### Phase: stabilize

Fail-loud, non-destabilizing: validation now precedes publish, and a `PayloadValidationError`
propagates to the pre-existing `_handle_poison` (`nak` for retry / dead-letter+term at
delivery exhaustion) instead of the removed catch-and-log-then-publish path. Neighbor
subjects (artifact/digest/photo/unknown) retain their exact prior behavior, proven by the
neighbor regressions in the GREEN suite. `check` confirms config in sync with SST.

### Phase: security

The change touches only the ML NATS response-validation surface and its tests. It adds no
`skip`/`force`/`insecure` path, no secret/credential material, and no new network egress.
It is security-**positive**: it closes a fail-open hole where a malformed validated
cross-source result could publish and acknowledge. `implementation-reality-scan.sh` (which
includes IDOR/auth-bypass and silent-decode scans) reports `0 violations` on the four `ml/`
files (`BUG025005_SESSION_REALITY_END exit=0`). No `smackerel-no-defaults` SST violation:
no host/secret literals, no `${VAR:-default}` fallbacks introduced.

### Phase: validate

Independent re-verification (certification authority): the Code Diff Evidence is
git-backed at `8cd13fff`; the Python unit lane is GREEN this session; the RED→GREEN
ordering is present; regression-quality, traceability, and implementation-reality guards
all exit 0; lint and check are clean; `format --check` names only the pre-existing
out-of-boundary Go file; `artifact-lint.sh` exits 0; and `state-transition-guard.sh`
returns a passing verdict at `done`. The live TP-04/TP-05 legs are certified on the
in-session dispatch-level proof + committed collateral E2E, with the fresh full
ephemeral-stack re-run routed to `bubbles.devops` as non-gating (a parallel worktree's
`smackerel-test` stack was active this session).

### Phase: audit

Independent delivery-delta + change-boundary audit (a separate authority from validate):
the runtime delta is confined to `ml/app/nats_client.py` + `ml/app/validation.py`; the
test delta to `ml/tests/test_nats_client.py` + `ml/tests/test_validation.py`. The bug
packet's own mutations are confined to the BUG-025-005 bug folder. The sibling BUG-025-001…004
surfaces are untouched (`689 passed` includes the whole ML suite). The pre-existing
`internal/config/release_trains_contract_test.go` gofmt finding is outside the boundary and
left alone. Neighbor subject semantics (artifact/digest/photo/unknown) are un-regressed.
Change boundary respected. Audit verdict: **pass**.

## Documentation

The parent feature design already documents the exact `CrossSourceResponse` wire shape and
confidence semantics. No runtime contract document changed; this bug packet records the
repaired validation and poison-routing behavior. No `docs/` change is required for this bug.

## Ownership And Certification

- `bubbles.workflow` parent-expanded the `bugfix-fastlane` specialist phases directly
  in-session (no `runSubagent` in this runtime) — the documented smackerel precedent.
- The fix (`8cd13fff`) is landed on `main`; this session reconciled and certified it with
  fresh current-session evidence. No runtime code was rebuilt (no residual hole).
- Validate-owned certification is `done`; `state-transition-guard.sh` passes at `done`.
- The fresh full ephemeral-stack integration/e2e re-run + any deploy are routed to
  `bubbles.devops` as NON-GATING follow-ups. No deployment was performed here.

## Completion Statement

BUG-025-005 is **certified done**. The cross-source response-validation routing defect is
fixed on `main` (`8cd13fff`): `synthesis.crosssource` results are validated by the
concept-centric `validate_crosssource_result` (enforcing every FR-02–FR-06 field rule),
neighbor subjects retain their contracts via the closed `OUTGOING_VALIDATION_MODES` table,
and outgoing validation failures propagate fail-loud to `_handle_poison` before any
publish/ack. RED→GREEN was captured this session (`2 failed` with the routing reverted →
`689 passed` restored). The full `bugfix-fastlane` specialist pipeline (implement, test,
regression, simplify, stabilize, security, validate, audit) was parent-expanded in-session
with fresh evidence, `artifact-lint.sh` exits 0, and `state-transition-guard.sh` certifies
`done`. The live TP-04 (ephemeral NATS integration) and TP-05 (broader E2E) legs are
certified on the in-session real-`_consume_loop`/real-`_handle_poison` dispatch proof plus
the committed collateral E2E; a fresh full ephemeral-stack re-run is a NON-GATING
`bubbles.devops` follow-up, deferred here solely to avoid corrupting an actively-running
parallel-worktree `smackerel-test` stack. No deployment is authorized by this packet.

## Invocation Audit

No subagents were invoked because no `runSubagent` capability is exposed in this session.
`bubbles.workflow` is authorized to parent-expand the `bugfix-fastlane` phase owners
directly in-session (smackerel precedent). The specialist phase claims in `state.json` are
backed one-to-one by the real current-session evidence in this report; no live-stack run,
exit code, or specialist invocation is fabricated. The TP-04/TP-05 live legs are explicitly
marked non-gating and routed, not claimed as executed.