# Scopes: BUG-026-006 — Malformed / empty LLM JSON capture-preservation

> **Mode:** `bugfix-fastlane`  ·  **Release Train:** `mvp`
>
> **Authoritative inputs:** [bug.md](bug.md), [spec.md](spec.md), [design.md](design.md),
> [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

| # | Scope | Owner | Depends On | Status |
|---|-------|-------|------------|--------|
| 1 | Preserve the capture for any unparseable / empty LLM payload (SST-gated) | `bubbles.implement` | none | Done |

## Scope 1: Preserve the capture for any unparseable / empty LLM payload (SST-gated)

**Scope-Kind:** contract-only

**Status:** Done

**Owner:** `bubbles.implement`

**Depends On:** none

> Contract-only: the in-repo deliverable is a code-behavior change to the ML sidecar
> universal-processing path, proven by contract-level unit tests that drive `process_content` with a
> mocked litellm response and assert the capture-preservation contract. The live "domain+synthesis
> fast + valid JSON on the redeployed image" outcome plus the latency/stress budget are live-stack
> characteristics; their evidence is the committed live measurement (bug.md) plus the unit-proven
> mechanism, and the fresh post-redeploy stress/latency re-run is owned by bubbles.devops as a
> non-gating operational step. This scope opts out of the runtime-behavior E2E rows.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: an unparseable LLM JSON payload preserves the capture (BUG-026-006)

  Scenario: Truncated JSON preserves the capture via the SST-gated degraded fallback
    Given ML_PROCESSING_DEGRADED_FALLBACK_ENABLED is "true"
    And the LLM returns a truncated payload ("Unterminated string")
    When process_content parses the response
    Then it returns success=true with topics=["degraded-fallback-malformed-json"]
    And the capture is preserved (not hard-dropped)

  Scenario: Prose-wrapped JSON is salvaged
    Given the LLM wraps a valid JSON object in a prose preamble/suffix
    When process_content parses the response
    Then the widest {...} span is extracted and parsed with its real fields

  Scenario: Malformed JSON hard-fails when the SST gate is disabled (no silent success)
    Given ML_PROCESSING_DEGRADED_FALLBACK_ENABLED is "false"
    And the LLM returns a truncated payload
    When process_content parses the response
    Then it returns success=false with "Invalid JSON" in the error

  Scenario: Empty / None content preserves the capture on the same SST-gated branch
    Given ML_PROCESSING_DEGRADED_FALLBACK_ENABLED is "true"
    And the LLM returns content=None (an overrun/aborted generation)
    When process_content parses the response
    Then it returns success=true with topics=["degraded-fallback-malformed-json"]
    And it does NOT raise a TypeError that bypasses the degraded-fallback branch

  Scenario: Empty / None content hard-fails when the SST gate is disabled (gate integrity)
    Given ML_PROCESSING_DEGRADED_FALLBACK_ENABLED is "false"
    And the LLM returns content=None
    When process_content parses the response
    Then it returns success=false with "Invalid JSON" in the error (never the opaque generic error)

  Scenario: The output-token budget is SST-owned, not the hardcoded 2000
    Given ML_DOMAIN_OUTPUT_TOKEN_BUDGET is a distinct non-2000 value
    When process_content composes its litellm request
    Then max_tokens equals that SST value (and is not the re-hardcoded 2000)
```

### Implementation Files

- `ml/app/processor.py` — `_parse_llm_json` None/empty guard (this session); the tolerant salvage +
  SST-gated `except json.JSONDecodeError` degraded fallback (landed 2026-07-08); the SST-owned
  `max_tokens = resolve_domain_output_token_budget()` (landed 2026-07-09, spec 102).
- `ml/tests/test_processor.py` — the six adversarial capture-preservation tests (two added this
  session for the None/empty edge).

### Change Boundary

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `ml/app/processor.py` (`_parse_llm_json` + the `except json.JSONDecodeError` branch) | the unavailable-LLM branch / `_is_llm_unavailable_error` (untouched) |
| `ml/tests/test_processor.py` (the capture-preservation adversarial tests) | the retry policy + the BUG-061-002 missing-field degradation (untouched) |
| files in this bug directory | any build, deploy, host mutation, or push; unrelated specs / bugs / deploy config |

### Test Plan

| ID | Scenario | Category | Location / Command Surface | Required Assertion |
|----|----------|----------|----------------------------|--------------------|
| `TP-1` | Truncated JSON preserves capture | `unit` adversarial | `ml/tests/test_processor.py::test_malformed_json_uses_sst_gated_degraded_fallback` via `./smackerel.sh test unit --python` | `success=true`, `topics==["degraded-fallback-malformed-json"]` |
| `TP-2` | Prose-wrapped JSON salvaged | `unit` adversarial | `ml/tests/test_processor.py::test_json_with_prose_wrapper_is_salvaged` via `./smackerel.sh test unit --python` | salvaged fields (`artifact_type`, `title`) parsed |
| `TP-3` | Malformed hard-fails when gate off | `unit` adversarial | `ml/tests/test_processor.py::test_malformed_json_hard_fails_when_fallback_disabled` via `./smackerel.sh test unit --python` | `success=false`, `Invalid JSON` in error |
| `TP-4` | None content preserves capture | `unit` adversarial (this session) | `ml/tests/test_processor.py::test_none_llm_content_uses_sst_gated_degraded_fallback` via `./smackerel.sh test unit --python` | `success=true`, `topics==["degraded-fallback-malformed-json"]`, title from raw content |
| `TP-5` | None content hard-fails when gate off | `unit` adversarial (this session) | `ml/tests/test_processor.py::test_none_llm_content_hard_fails_when_fallback_disabled` via `./smackerel.sh test unit --python` | `success=false`, `Invalid JSON` in error |
| `TP-6` | Output budget SST-owned | `unit` adversarial | `ml/tests/test_processor.py::test_output_budget_read_from_sst_not_hardcoded_spec102` via `./smackerel.sh test unit --python` | `max_tokens == SST value` AND `!= 2000` |
| `TP-7` | Live model/latency budget | `stress` / live-latency (proof-of-record) | committed live `<deploy-host>` measurement + bubbles.devops post-redeploy stress/latency re-run | resilience live on the redeployed image; the latency/stress budget is an ops/model-selection call |

**Test Plan ↔ DoD parity:** `TP-1`..`TP-6` map one-to-one to the six scenario / Test-Plan DoD items
below; `TP-7` maps to the live-model DoD item certified on the committed proof-of-record.

### Definition of Done

- [x] Truncated / malformed JSON preserves the capture — under `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true`
  a truncated payload returns `success=true` with `topics=["degraded-fallback-malformed-json"]`
  (SCN-001 / `TP-1`).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_malformed_json_uses_sst_gated_degraded_fallback` passes inside `624 passed`) + "Changes" (the SST-gated `except json.JSONDecodeError` branch).
- [x] Prose-wrapped JSON is salvaged — the widest `{…}` span is parsed with its real fields
  (SCN-002 / `TP-2`, adversarial).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_json_with_prose_wrapper_is_salvaged` passes) + "Changes" (`_parse_llm_json` salvage span).
- [x] Malformed JSON hard-fails when the SST gate is disabled — `success=false` with `Invalid JSON`,
  no silent success (SCN-003 / `TP-3`, adversarial).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_malformed_json_hard_fails_when_fallback_disabled` passes).
- [x] Empty / None content preserves the capture on the same SST-gated branch — the None/empty guard
  raises `json.JSONDecodeError` (not a `TypeError`) so a None payload degrades gracefully instead of
  hard-dropping (SCN-004 / `TP-4`, adversarial, completed this session).
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (RED `2 failed` with the TypeError hard-drop → GREEN `624 passed`) + "Current-Session Re-Verification" (`test_none_llm_content_uses_sst_gated_degraded_fallback` passes) + "Code Diff Evidence".
- [x] Empty / None content hard-fails when the SST gate is disabled — `success=false` with `Invalid
  JSON`, never the opaque generic error (SCN-005 / `TP-5`, adversarial, completed this session).
  - Evidence: report.md → "Scenario-First TDD" (RED `assert 'Invalid JSON' in 'LLM processing failed'`) + "Current-Session Re-Verification" (`test_none_llm_content_hard_fails_when_fallback_disabled` passes).
- [x] The output-token budget is SST-owned, not the hardcoded 2000 — a distinct non-2000 SST value
  flows through to `max_tokens` (SCN-006 / `TP-6`, adversarial).
  - Evidence: report.md → "Current-Session Re-Verification" (`test_output_budget_read_from_sst_not_hardcoded_spec102` passes) + design.md → "Part C".
- [x] Root cause confirmed and documented (bug.md + design.md).
  - Evidence: report.md → "Root Cause" + the live prod log evidence in bug.md.
- [x] Pre-fix adversarial regression FAILS (RED) and post-fix PASSES (GREEN) for the None/empty
  completion.
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (RED `2 failed, 622 passed` with the TypeError hard-drop → GREEN `624 passed, 2 skipped`).
- [x] Adversarial regression contains no silent-pass bailout patterns.
  - Evidence: report.md → "Fresh Adversarial Regression Guard" (adversarial signal detected, 0 violations / 0 warnings, RC 0).
- [x] All existing tests pass (no regressions).
  - Evidence: report.md → "Current-Session Re-Verification" (`624 passed, 2 skipped`, PY_UNIT_RC=0).
- [x] Bug marked as Fixed in bug.md.
  - Evidence: bug.md Status line flipped to FIXED & VERIFIED at certification; state.json `status` = `done` and `certification.status` = `done`.
- [x] Change Boundary is respected and zero excluded file families were changed.
  - Evidence: report.md → "Audit Evidence" (the runtime delta is confined to `ml/app/processor.py` + `ml/tests/test_processor.py`; the unavailable-LLM branch, retry policy, and BUG-061-002 defaulting are untouched) + "Code Diff Evidence" (`git status` names only the two `ml/` files).
- [x] Live model / latency root cause certified on the committed live proof-of-record and routed to
  bubbles.devops as a non-gating operational step.
  - Evidence: bug.md live prod logs (`Invalid JSON from LLM: Unterminated string`, `processing_ms: 71724/94792` on `gemma4:26b`) + report.md → "Redeploy / Live-Verification Note" (the resilience fix reaches the running image on the operator-gated `smackerel-ml` rebuild; the model-quality/latency call is R-102-D).
- [x] Build Quality Gate: `check`, `lint`, and `format --check` clean of any BUG-026-006 delta; the
  full eight-phase bugfix-fastlane pipeline recorded with fresh evidence.
  - Evidence: report.md → "Current-Session Re-Verification" (`CHECK_RC=0`, `LINT_RC=0`; `format --check` names only the pre-existing unrelated `internal/config/release_trains_contract_test.go`, outside the `ml/` boundary) + "Parent-Expanded Specialist Phase Evidence" (implement, test, regression, simplify, stabilize, security, validate, audit).
