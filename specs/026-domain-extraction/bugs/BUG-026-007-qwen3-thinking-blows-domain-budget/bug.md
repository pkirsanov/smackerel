# BUG-026-007 — qwen3 default thinking-mode blows the 30s domain-extraction budget

- **Severity:** HIGH (redteam **F2**, latency half)
- **Owning spec:** `026-domain-extraction` (owns the domain-extraction 30s-budget ML-processing contract)
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on evo-x2 (both models WARM)
- **Status:** MECHANISM CORRECTED (native Ollama `think` field) + UNIT TESTS REWORKED IN-REPO — not pushed; live verification pending orchestrator redeploy. The FIRST fix (the `/no_think` prompt token) was INEFFECTIVE — see "⚠️ Mechanism Correction" below.

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
- This change removes the **thinking** latency on the structured-JSON extraction path — but ONLY
  via the native `think` field (see ⚠️ Mechanism Correction); the first fix's `/no_think` token did
  not.

## ⚠️ Mechanism Correction (native `think` field — the first fix was ineffective)

The FIRST fix injected the qwen `/no_think` control token into the request messages. **That is
INEFFECTIVE on qwen3**: qwen3's Ollama chat template ignores the `/no_think` text directive and
honors ONLY the native `think` request field. Measured live on evo-x2 (shared daemon, warm qwen3):

- `/no_think` in prompt → thinking **STILL ON** (>150s), and the `<think>`-prefixed output produced
  the live `ERROR smackerel-ml.synthesis LLM returned invalid JSON: Expecting value: line 1 column 1`
  (the F2 invalid-JSON failure).
- native `think=False` (ollama `/api/chat` top-level field) → thinking **OFF**: a trivial prompt
  reports `load=0.1s prompt_eval=0.1s gen=0.9s eval_tok=6` (~1s compute) vs `think=True` = 119s.
- **litellm forwards it:** `litellm.acompletion(..., think=False)` AND `extra_body={"think": False}`
  both produced thinking-OFF behavior (valid JSON, low `eval_tok`) for the `ollama_chat/` prefix.
  Verified against the pinned `litellm==1.84.0` with a request-capture probe (report.md → Test
  Evidence): a top-level `think=` lands at the request top level for both the `ollama_chat/` and
  legacy `ollama/` transforms. (The earlier 30–60s wall-times were daemon queueing CAUSED by the
  ml's own thinking-ON calls; fixing the mechanism removes that self-inflicted saturation.)

The fix now sets the native `think=False` kwarg on every in-scope structured-extraction call and
migrates the two legacy `ollama/` routes (search-rerank, drive-classify) to `ollama_chat/`.

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

**Mechanism: the native Ollama `think` request field** — `apply_structured_extraction_thinking`
sets `completion_kwargs["think"] = False` (SST-gated, ollama-only) on every in-scope structured
call. Rationale (corrected):

- qwen3's Ollama chat template honors ONLY the native `think` field; a `/no_think` string in the
  messages is IGNORED (the first fix's error — proven live on evo-x2, see ⚠️ Mechanism Correction).
- litellm 1.84.0 forwards a top-level `think=` kwarg to the Ollama request **top level**
  (`data["think"]`) for BOTH the `ollama_chat/` and legacy `ollama/` transforms — verified with a
  request-capture probe (`litellm/llms/ollama/{chat,completion}/transformation.py`). Unlike
  `keep_alive` (buried under `options` on the legacy generate route).
- The two formerly-legacy routes (`nats_client` search-rerank, `drive_classify`) are migrated to the
  `ollama_chat/` (`/api/chat`) prefix for role fidelity + `keep_alive` parity + consistency with the
  other structured sites (`domain`, `synthesis`, `processor`, `card_categories`).
- A no-op on non-qwen Ollama models (they ignore the field) and on non-ollama providers.

**SST wiring (mirrors `ml/app/ollama_keepalive.py` exactly — fail-loud, NO hardcoded default):**

- New resolver `ml/app/ollama_thinking.py::resolve_structured_extraction_thinking()` reading
  `ML_STRUCTURED_EXTRACTION_THINKING`; fail-loud `RuntimeError` on unset/empty/invalid.
- Reworked helper `apply_structured_extraction_thinking(completion_kwargs, provider)` — provider-
  gated to ollama, SST-gated, sets the native `completion_kwargs["think"] = False` only when
  thinking is DISABLED (was: a `/no_think` message injector).
- SST source key `services.ml.structured_extraction_thinking: false` added to
  `config/smackerel.yaml`, emitted by `scripts/commands/config.sh` next to `ml_ollama_keep_alive`,
  and validated at startup in `ml/app/main.py::_check_required_config` (ollama-conditional,
  true/false, `sys.exit(1)` on invalid — mirrors the keep_alive validation).
- Value semantics: `false` = disable thinking (the fix's posture, native `think=False`); `true` =
  keep thinking. Fail loud on any other value.

## Explicitly OUT of scope (unchanged)

- [ml/app/agent.py](../../../../ml/app/agent.py) `handle_invoke` — the agent reasoning/planning
  path (separate `AGENT_PROVIDER_*` model resolution). Thinking is VALUABLE there (quality >
  latency). Guarded by an explicit scope-boundary test.
- `ml/app/main.py::_warmup_domain_model` — best-effort warmup (`max_tokens=1`), not a structured
  extraction.
- `ml/app/nats_client.py::_handle_generate_digest` — plain-text digest, carries its OWN inline
  `/no_think` prefix (UNCHANGED here). NOTE (adjacent finding): if it runs on qwen3 that `/no_think`
  is likely ALSO ineffective — flagged for a follow-up bug, not fixed in this structured-extraction
  scope.
- `ml/app/routes/chat.py` — interactive assistant/chat surface.

## Test evidence

See [report.md](report.md) → "Test Evidence" for the RED (pre-fix) and GREEN (post-fix) runs.

## Redeploy note

This is a **code change to `smackerel-ml`**. The running prod image on evo-x2 is **unchanged** until
the orchestrator rebuilds + signs + redeploys `smackerel-ml`. The live "domain extraction now < 30s"
outcome is therefore **PENDING that redeploy** — it cannot be, and is not, claimed here. No build,
no deploy, no host mutation, no push performed in this repo.
