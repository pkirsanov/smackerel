# User Validation: BUG-026-006

## Checklist

### [Bug Fix] BUG-026-006 malformed / empty LLM JSON preserves the capture

- [x] **What:** A malformed / truncated LLM JSON payload preserves the user's capture (via the
  SST-gated degraded fallback) instead of silently dropping it.
  - **Steps:**
    1. Set `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true` (the SST posture in `config/smackerel.yaml`).
    2. Run `./smackerel.sh test unit --python`.
  - **Expected:** `test_malformed_json_uses_sst_gated_degraded_fallback` proves a truncated payload
    returns `success=true` with `topics=["degraded-fallback-malformed-json"]`;
    `test_json_with_prose_wrapper_is_salvaged` proves prose-wrapped JSON is salvaged.
  - **Verify:** `./smackerel.sh test unit --python`
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** The resilience fix reaches the running image only AFTER the orchestrator rebuilds +
    signs + redeploys `smackerel-ml` on `<deploy-host>` — the operator-gated redeploy is outside this
    repo's scope.
- [x] **What:** An EMPTY / None LLM content preserves the capture on the same branch (completed this
  session) instead of hard-dropping via a `TypeError`.
  - **Steps:**
    1. Set `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true`.
    2. Run `./smackerel.sh test unit --python`.
  - **Expected:** `test_none_llm_content_uses_sst_gated_degraded_fallback` proves a None payload
    returns `success=true` with the capture preserved; the RED→GREEN ordering (`2 failed → 624
    passed`) proves the pre-completion `TypeError` hard-drop is closed.
  - **Verify:** `./smackerel.sh test unit --python`
  - **Evidence:** report.md → "Scenario-First TDD — RED → GREEN Ordering"
  - **Notes:** Completes the redteam F2 residual on the None/empty edge.
- [x] **What:** The SST gate is fail-loud in both directions — a malformed / None payload hard-fails
  (no silent success) when the degraded fallback is disabled.
  - **Steps:** Set `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=false` and drive a malformed / None
    payload.
  - **Expected:** `success=false` with `Invalid JSON` in the error, never the opaque generic error.
  - **Verify:** `test_malformed_json_hard_fails_when_fallback_disabled` +
    `test_none_llm_content_hard_fails_when_fallback_disabled`
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** smackerel `smackerel-no-defaults` policy.
- [x] **What:** The output-token budget is SST-owned (not the hardcoded 2000 that could truncate the
  schema and manufacture the malformed payload).
  - **Steps:** Set `ML_DOMAIN_OUTPUT_TOKEN_BUDGET` to a distinct non-2000 value and inspect the
    litellm request.
  - **Expected:** `max_tokens` equals the SST value and is not 2000.
  - **Verify:** `test_output_budget_read_from_sst_not_hardcoded_spec102`
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** Landed under spec 102 SCOPE-102-03; certified here as part of the capture-preservation
    contract.
