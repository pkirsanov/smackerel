# BUG-026-007 — qwen3 default thinking-mode blows the 30s domain-extraction budget

- **Severity:** HIGH (redteam **F2**, latency half)
- **Owning spec:** `026-domain-extraction` (owns the domain-extraction 30s-budget ML-processing contract)
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on evo-x2 (both models WARM)
- **Status:** CODE + UNIT TESTS FIXED IN-REPO — not pushed; live verification pending orchestrator redeploy

## Summary

smackerel's ML sidecar calls `qwen3:30b-a3b` (the prod `LLM_MODEL`) for structured-JSON
extraction, but qwen3 runs with its **thinking mode ON by default**, generating a large hidden
`<think>…</think>` reasoning block BEFORE the JSON. Measured live on the evo-x2 SHARED ollama
daemon — both models already WARM, identical recipe-extraction task, ground truth 9 ingredients /
5 steps:

| Config | Wall | Valid JSON | Ingredients | Verdict |
|--------|------|-----------|-------------|---------|
| qwen3 `think=TRUE` (current default) | **113.4s** | yes | 9/9 | **~4× OVER the 30s budget** |
| qwen3 `think=FALSE` | **8.5–12.9s** | yes | 9/9 | within budget, identical accuracy |
| gemma4:26b (comparison only) | 46.9s | 2/3 (1 timeout) | 5/9 | slower + less accurate — NOT the fix |

`ml/app/domain.py` enforces `DOMAIN_EXTRACTION_TIMEOUT = 30`. The current qwen3 default-thinking
path (~113s) **exceeds the 30s budget**, so domain extraction is **silently timing out into the
degraded recipe fallback** (`_degraded_domain_fallback`) in production instead of returning the
full LLM extraction. Disabling thinking gives qwen3's FULL accuracy (9/9) at ~10s — comfortably
inside budget.

This is the **remaining half of the redteam F2 (LLM-enrichment latency) fix**:

- The keep_alive change (`ml/app/ollama_keepalive.py`, sibling BUG-026-006 family) already removed
  the **cold-load** latency (~22-45s).
- This change removes the **thinking** latency (~113s → ~10s) on the structured-JSON extraction
  path.

## Reproduction

**Redteam (live evo-x2 prod, both models WARM):** identical recipe-extraction task against the
shared ollama daemon —

- `qwen3:30b-a3b` with ollama native top-level `"think": true` (the current default): **113.4s**
  wall, valid JSON, 9/9 ingredients.
- `qwen3:30b-a3b` with ollama native top-level `"think": false`: **8.5–12.9s** wall, valid JSON,
  9/9 ingredients (identical accuracy).

Because 113.4s > `DOMAIN_EXTRACTION_TIMEOUT = 30`, `handle_domain_extract` raises
`asyncio.TimeoutError` and returns the SST-gated degraded fallback (or a hard `success: False`),
so prod never sees the full qwen3 extraction.

**In-repo static confirmation:** every structured-JSON extraction call on the `LLM_MODEL` (qwen3)
text path composes its `litellm.acompletion` request WITHOUT any thinking-disable directive, so
qwen3 uses its default (thinking ON):

- [ml/app/domain.py](../../../../ml/app/domain.py) `_do_domain_extract` `completion_kwargs` (the 30s-budget path — the proven timeout)
- [ml/app/synthesis.py](../../../../ml/app/synthesis.py) `handle_extract` `completion_kwargs` + `handle_crosssource` `crosssource_kwargs`
- [ml/app/processor.py](../../../../ml/app/processor.py) `process_content` `completion_kwargs`
- [ml/app/nats_client.py](../../../../ml/app/nats_client.py) `_handle_search_rerank` (search re-rank; `LLM_MODEL`, `response_format=json_object`)
- [ml/app/card_categories.py](../../../../ml/app/card_categories.py) `extract_card_categories` (`os.environ["LLM_MODEL"]`, strict-schema JSON)
- [ml/app/drive_classify.py](../../../../ml/app/drive_classify.py) `classify_drive_file` (`LLM_MODEL` via NATS dispatch, `response_format=json_object`)

## Root cause

qwen3 (a "thinking" model) defaults to emitting a hidden chain-of-thought reasoning block before
its answer. On the structured-JSON **extraction** path — where the OUTPUT is a machine-consumed
JSON object and reasoning quality of the prose is irrelevant — that hidden block is pure latency
(~100s of extra wall time) that pushes a single artifact extraction from ~10s to ~113s, past the
30s domain budget. The sidecar never disabled thinking on these calls, so it inherited qwen3's
latency-maximizing default.

## Fix (in-repo — SST-gated thinking-disable on the structured-JSON extraction path)

New SST-owned, fail-loud switch that **disables qwen3 thinking on structured-JSON extraction
calls** while leaving the agent reasoning path unchanged.

**Mechanism chosen: the qwen `/no_think` control token injected into the extraction request
messages** (NOT a top-level `think=False` param). Rationale:

- Several in-scope calls (`nats_client` search-rerank, `drive_classify`) route through litellm's
  **legacy `ollama/` (`/api/generate`) transform**, which — exactly like `keep_alive` — buries
  unknown top-level params under `options`, where Ollama never sees them (documented in
  `ollama_keepalive.py`). A top-level `think=False` would therefore be a **silent no-op** on those
  routes.
- `/no_think` is **route-agnostic** (it is prompt text the model always sees), **version-agnostic**
  (no dependence on a specific litellm passthrough), and a **documented no-op on non-qwen models**
  (e.g. `gemma3:4b` in dev), so it is safe on every `LLM_MODEL`.
- The codebase **already uses `/no_think`** as a prompt prefix to suppress reasoning on the digest
  generation path (`nats_client.py::_handle_generate_digest`) — a proven precedent in this repo.

**SST wiring (mirrors `ml/app/ollama_keepalive.py` exactly — fail-loud, NO hardcoded default):**

- New resolver `ml/app/ollama_thinking.py::resolve_structured_extraction_thinking()` reading
  `ML_STRUCTURED_EXTRACTION_THINKING`; fail-loud `RuntimeError` on unset/empty/invalid.
- New helper `apply_structured_extraction_thinking(messages, provider)` — provider-gated to
  ollama, SST-gated, injects `/no_think` only when thinking is DISABLED.
- SST source key `services.ml.structured_extraction_thinking: false` added to
  `config/smackerel.yaml`, emitted by `scripts/commands/config.sh` next to `ml_ollama_keep_alive`,
  and validated at startup in `ml/app/main.py::_check_required_config` (ollama-conditional,
  true/false, `sys.exit(1)` on invalid — mirrors the keep_alive validation).
- Value semantics: `false` = disable thinking (the fix's posture, ~10s); `true` = keep thinking.
  Fail loud on any other value.

## Explicitly OUT of scope (unchanged)

- [ml/app/agent.py](../../../../ml/app/agent.py) `handle_invoke` — the agent reasoning/planning
  path (separate `AGENT_PROVIDER_*` model resolution). Thinking is VALUABLE there (quality >
  latency). Guarded by an explicit scope-boundary test.
- `ml/app/main.py::_warmup_domain_model` — best-effort warmup (`max_tokens=1`), not a structured
  extraction.
- `ml/app/nats_client.py::_handle_generate_digest` — plain-text digest, already carries `/no_think`.
- `ml/app/routes/chat.py` — interactive assistant/chat surface.

## Test evidence

See [report.md](report.md) → "Test Evidence" for the RED (pre-fix) and GREEN (post-fix) runs.

## Redeploy note

This is a **code change to `smackerel-ml`**. The running prod image on evo-x2 is **unchanged** until
the orchestrator rebuilds + signs + redeploys `smackerel-ml`. The live "domain extraction now < 30s"
outcome is therefore **PENDING that redeploy** — it cannot be, and is not, claimed here. No build,
no deploy, no host mutation, no push performed in this repo.
