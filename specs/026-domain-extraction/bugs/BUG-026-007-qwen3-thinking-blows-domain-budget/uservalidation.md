# User Validation: BUG-026-007

## Checklist

### [Bug Fix] BUG-026-007 qwen3 thinking-disable on structured-JSON extraction
- [x] **What:** Domain / synthesis / processor / search-rerank / card-categories / drive-classify
  structured-JSON extraction calls disable qwen3 thinking (via `/no_think`) when
  `ML_STRUCTURED_EXTRACTION_THINKING=false`, so qwen3 returns JSON in ~10s instead of ~113s.
  - **Steps:**
    1. Set `ML_STRUCTURED_EXTRACTION_THINKING=false` (the SST default posture in
       `config/smackerel.yaml`).
    2. Run `./smackerel.sh test unit --python`.
  - **Expected:** All per-call-site tests prove the `litellm.acompletion` request carries
    `/no_think`; the agent-boundary test proves the agent path does NOT; the resolver fail-loud
    tests pass; the full suite is green.
  - **Verify:** `./smackerel.sh test unit --python`
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** Live "< 30s domain extraction" is verified only AFTER the orchestrator rebuilds +
    signs + redeploys `smackerel-ml` on evo-x2 — pending redeploy (out of this repo's scope).
- [x] **What:** The switch is fail-loud (NO default) — a missing/invalid value stops the sidecar.
  - **Steps:** Unset `ML_STRUCTURED_EXTRACTION_THINKING` for the ollama provider and start the
    sidecar (or call the resolver).
  - **Expected:** Fail loud (`RuntimeError` / `sys.exit(1)`), never a silent default.
  - **Verify:** `./smackerel.sh test unit --python` (resolver + `_check_required_config` tests)
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** smackerel `smackerel-no-defaults` policy.
- [x] **What:** The agent reasoning path is unchanged (still allows qwen3 thinking).
  - **Steps:** Invoke the agent path with `ML_STRUCTURED_EXTRACTION_THINKING=false`.
  - **Expected:** No `/no_think` in the agent request (quality > latency preserved).
  - **Verify:** `test_agent_path_keeps_thinking_even_when_disabled`
  - **Evidence:** report.md → "Test Evidence"
  - **Notes:** Hard scope boundary.
