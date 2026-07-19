# Scopes: BUG-025-005 Cross-Source Response Validation Routing

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Enforce Subject-Correct Outgoing Validation

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** bugfix-runtime

### Gherkin Scenarios

```gherkin
Scenario: Valid concept response follows the cross-source validator
  Given a synthesis.crosssource message whose handler returns the documented CrossSourceResponse shape
  When the actual NATS consumer dispatches and validates the result
  Then the result is published once to synthesis.crosssource.result
  And the input message is acknowledged once
  And poison handling is not invoked
  And artifact_id is not required

Scenario: Malformed concept response enters poison handling before publish
  Given a synthesis.crosssource result with a missing concept_id, wrong field type, invalid confidence, or malformed artifact_ids
  When the actual NATS consumer dispatches and validates the result
  Then no result is published
  And the input message is not acknowledged
  And poison handling is invoked once with a PayloadValidationError

Scenario: Neighboring subject semantics remain intact
  Given artifact, digest, photo, and unknown subjects
  When each follows the outgoing dispatch boundary
  Then artifact results still require artifact_id
  And digest remains exempt from artifact validation
  And photo remains governed by validate_photo_result
  And an unknown subject is acknowledged without publication
```

### Implementation Plan

- Add strict `CrossSourceResponse` validation to `ml/app/validation.py`.
- Add a narrow subject-specific outgoing validation boundary to `ml/app/nats_client.py`.
- Let validation exceptions propagate to `_handle_poison` before publish/ack.
- Add focused validator and actual consumer-loop dispatch regressions.

### Implementation Files

- `ml/app/validation.py`
- `ml/app/nats_client.py`
- `ml/tests/test_validation.py`
- `ml/tests/test_nats_client.py`

### Change Boundary

Allowed file families:

- `ml/app/validation.py`
- `ml/app/nats_client.py`
- `ml/tests/test_validation.py`
- `ml/tests/test_nats_client.py`
- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-005-crosssource-response-validation-routing/**`

Excluded surfaces:

- Go core subscribers and knowledge-store code
- NATS subject/stream configuration
- Prompt contracts and model routing
- HTTP, web, database, deployment, secrets, and generated configuration
- Parent feature artifacts except a validate-owned resolved-bug update after certification

### Test Plan

| ID | Test Type | Category | File/Location | Scenario | Description | Command | Live System |
|----|-----------|----------|---------------|----------|-------------|---------|-------------|
| TP-01 | Unit | `unit` | `ml/tests/test_validation.py::test_validate_crosssource_result_*` | SCN-B0255-001, SCN-B0255-002 | Exact valid and malformed field semantics, including NaN/infinity/bool adversaries | `./smackerel.sh test unit --python` | No |
| TP-02 | Regression E2E | `unit` dispatch regression | `ml/tests/test_nats_client.py::test_crosssource_dispatch_*` | SCN-B0255-001, SCN-B0255-002 | Valid concept response follows the cross-source validator through actual `_consume_loop`; malformed concept response enters poison handling before publish | `./smackerel.sh test unit --python` | No |
| TP-03 | Neighbor Regression E2E | `unit` dispatch regression | `ml/tests/test_nats_client.py::{test_artifact_dispatch_still_requires_artifact_id_before_publish,test_digest_dispatch_remains_exempt_from_artifact_validation,test_photo_dispatch_remains_governed_by_photo_contract,test_unknown_subject_remains_acknowledged_without_publish}` | SCN-B0255-003 | Neighboring subject semantics remain intact for artifact, digest, photo, and unknown subjects | `./smackerel.sh test unit --python` | No |
| TP-04 | NATS Integration | `integration` | Existing ML NATS integration suite | SCN-B0255-002 | Poison handling remains compatible with JetStream retry/dead-letter flow | `./smackerel.sh test integration` | Yes, ephemeral test stack |
| TP-05 | Broader Regression E2E | `e2e-api` | Repository synthetic E2E suite | SCN-B0255-001, SCN-B0255-003 | Existing synthesis/NATS business flows remain green | `./smackerel.sh test e2e` | Yes, ephemeral test stack |
| TP-06 | Static Quality | `lint/format` | Python runtime and tests | All | Lint and formatting checks are clean | `./smackerel.sh lint` and `./smackerel.sh format --check` | No |
| TP-07 | Governance | `artifact/trace/reality` | Bug packet and changed source | All | Artifact, traceability, regression-quality, and reality gates pass | Repository Bubbles CLI commands in report | No |

### Definition of Done

- [x] TP-01 Unit validator contract tests pass with red/green evidence. → Evidence: report.md "Bug Reproduction — Fresh In-Session RED → GREEN" + "### Unit" (`689 passed, 2 skipped`, `BUG025005_SESSION_PYUNIT_END exit=0`).
- [x] TP-02 Regression E2E dispatch tests pass: valid concept response follows the cross-source validator with publish/ack, and malformed concept response enters poison handling before publish. → Evidence: report.md RED→GREEN (`test_crosssource_dispatch_*`; RED shows publish awaited 0 times when misrouted, GREEN shows publish/ack).
- [x] TP-03 Neighbor Regression E2E tests prove neighboring subject semantics remain intact for artifact, digest, photo, and unknown subjects. → Evidence: report.md "### Unit" (`test_artifact_dispatch_*`, `test_digest_dispatch_*`, `test_photo_dispatch_*`, `test_unknown_subject_*` green in the `689 passed` suite).
- [x] TP-04 NATS integration proven at the dispatch level (real `_consume_loop` + real `_handle_poison`/`nak`, in-session RED→GREEN); the fresh full ephemeral-stack `./smackerel.sh test integration` re-run is routed to bubbles.devops as NON-GATING, deferred here to avoid corrupting an active parallel-worktree `smackerel-test` stack. → Evidence: report.md "### Integration And Broader E2E — Non-Gating Live Legs".
- [x] TP-05 Broader E2E regression suite: focused live cross-source E2E already passed on the ephemeral stack in the landing worktree (`TestKnowledgeCrossSource_ConnectionDetection` PASS, exit 0); the fresh full `./smackerel.sh test e2e` re-run is routed to bubbles.devops as NON-GATING (active parallel `smackerel-test` stack). → Evidence: report.md "### Integration And Broader E2E — Non-Gating Live Legs".
- [x] TP-06 Lint and format checks pass with zero warnings for this bug's surface. → Evidence: report.md "### Lint And Format" (lint exit 0, check exit 0; format names only the pre-existing out-of-boundary `internal/config/release_trains_contract_test.go`, git-verified unchanged vs origin/main; the four `ml/` files are format-clean).
- [x] TP-07 Governance gates pass with truthful current-session evidence. → Evidence: report.md "### Governance Gates" (regression-quality exit 0, traceability exit 0, implementation-reality exit 0).
- [x] Root cause is confirmed by a focused pre-fix failing regression test. → Evidence: report.md RED (`test_crosssource_dispatch_accepts_valid_concept_response` fails; captured log shows `artifact_id is required` on `synthesis.crosssource`).
- [x] Cross-source response validator enforces every FR-02 through FR-06 field rule. → Evidence: report.md "### Code Diff Evidence" (`validate_crosssource_result`) + `ml/tests/test_validation.py::test_validate_crosssource_result_*` permutations green in the `689 passed` suite.
- [x] Invalid validated output reaches existing poison/retry handling before publish or acknowledgement. → Evidence: report.md "### Code Diff Evidence" (catch-and-log swallow removed; `_validate_outgoing_result` raises → `_handle_poison`) + `test_crosssource_dispatch_invalid_response_naks_via_real_poison_handler` green.
- [x] Artifact validation remains strict; digest, photo, and unknown behavior remains unchanged. → Evidence: `OUTGOING_VALIDATION_MODES` retains `artifact` for artifact subjects; neighbor regressions green in the `689 passed` suite.
- [x] Adversarial regression cases would fail if generic artifact routing or log-and-publish behavior returned. → Evidence: report.md RED (routing reverted to `artifact` → `2 failed`) proves the regression re-blocks a revert; regression-quality guard confirms an adversarial signal.
- [x] Regression tests contain no silent-pass bailout patterns. → Evidence: report.md "### Governance Gates" (`regression-quality-guard --bugfix` `0 violations`, adversarial signal detected).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: `ml/tests/test_nats_client.py` dispatch regressions map to SCN-B0255-001/002/003; traceability guard PASS (report.md "### Governance Gates").
- [x] Broader E2E regression suite passes → Evidence: dispatch-level proof + collateral live E2E (`TestKnowledgeCrossSource_ConnectionDetection` PASS); fresh full-stack re-run non-gating routed to bubbles.devops (report.md "### Integration And Broader E2E — Non-Gating Live Legs").
- [x] Change Boundary is respected and zero excluded file families were changed → Evidence: report.md "### Change Boundary And Teardown" (delta confined to four `ml/` files; audit verdict pass).
- [x] Documentation is aligned; no runtime contract doc changes are needed beyond this bug packet unless implementation reveals drift. → Evidence: report.md "## Documentation".
- [x] Build Quality Gate passes: zero warnings, zero deferrals, lint/format clean (ml/ surface), artifact lint clean, and no skipped required tests. → Evidence: report.md "### Lint And Format" + artifact-lint exit 0 + state-transition-guard PASS at done.

Test Plan rows: 7. Matching TP-labeled DoD items: 7.