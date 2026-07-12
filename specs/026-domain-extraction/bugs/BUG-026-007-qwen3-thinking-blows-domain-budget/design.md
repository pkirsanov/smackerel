# Design: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

## Root Cause Analysis

`qwen3:30b-a3b` is a **thinking** model: by default it emits a hidden `<think>…</think>`
chain-of-thought block before its answer. On the smackerel ML sidecar's **structured-JSON
extraction** calls (where the model output is a machine-parsed JSON object), that reasoning block
is pure latency. Live on <deploy-host> (both models warm, identical recipe task) it costs **~113s** vs
**~10s** with thinking disabled — same 9/9-ingredient accuracy either way.

`ml/app/domain.py` wraps `_do_domain_extract` in `asyncio.wait_for(..., timeout=30)`
(`DOMAIN_EXTRACTION_TIMEOUT = 30`). Because ~113s ≫ 30s, `handle_domain_extract` raises
`asyncio.TimeoutError` and returns `_degraded_domain_fallback(...)` (or a hard `success: False`).
**Net effect in prod:** the full qwen3 extraction is never delivered — every domain extraction
silently degrades. The sidecar never disabled thinking on these calls, so it inherited qwen3's
latency-maximizing default.

Sibling context: the redteam F2 finding has two latency components. The **cold-load** component
(~22-45s) was already fixed by the `keep_alive` change (`ml/app/ollama_keepalive.py`). This bug is
the **thinking** component (~113s), the remaining half.

## Fix Design

### Mechanism: native Ollama `think` field (CORRECTED — the `/no_think` token is ineffective)

The FIRST design chose the `/no_think` control token on the assumption that a top-level `think=`
param would be buried under `options` on the legacy `ollama/` route (as `keep_alive` is). **Both
premises were wrong:**

1. **qwen3 ignores `/no_think`.** qwen3's Ollama chat template honors ONLY the native `think`
   request field; a `/no_think` string in the messages is a no-op (proven live on <deploy-host>:
   `/no_think` → thinking STILL on, >150s, invalid-JSON output).
2. **litellm 1.84.0 forwards top-level `think=` on BOTH routes.** An empirical request-capture probe
   (report.md → Test Evidence) shows `think=False` lands at the request TOP LEVEL (`data["think"]`)
   for the `ollama_chat/` (`/api/chat`) AND the legacy `ollama/` (`/api/generate`) transforms
   (`litellm/llms/ollama/{chat,completion}/transformation.py`: `think = optional_params.pop("think",
   None); if think is not None: data["think"] = think`). Unlike `keep_alive` (buried under `options`
   on the generate route) and unlike `reasoning_effort` (mapped to `think=True` for
   `"low"|"medium"|"high"` — the wrong direction).

**Corrected mechanism:** set the native top-level `completion_kwargs["think"] = False` (SST-gated,
ollama-only) on every in-scope structured call. The two formerly-legacy routes (`nats_client`
search-rerank, `drive_classify`) are migrated to the `ollama_chat/` prefix for role fidelity,
`keep_alive` parity, and consistency with the other structured sites (`domain`, `synthesis`,
`processor`, `card_categories`).

| Route | in-scope sites | `think=False` forwarded? |
|-------|----------------|--------------------------|
| `ollama_chat/` (`/api/chat`) | domain, synthesis (extract + crosssource), processor, card_categories, + migrated search-rerank + drive-classify | yes, top level |

A unit test proves each in-scope call actually sends `think=False` to litellm; two more prove the
migrated routes now resolve to `ollama_chat/…`.

### New module `ml/app/ollama_thinking.py`

Mirrors `ollama_keepalive.py` exactly (fail-loud SST resolver, no default):

```python
def resolve_structured_extraction_thinking() -> bool:
    """True = keep qwen thinking; False = disable (native think=False). Fail-loud."""
    raw = os.environ.get("ML_STRUCTURED_EXTRACTION_THINKING", "").strip().lower()
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise RuntimeError("ML_STRUCTURED_EXTRACTION_THINKING is required and must be 'true' or 'false' …")

def apply_structured_extraction_thinking(completion_kwargs, provider) -> dict:
    """No-op for non-ollama providers and when thinking is enabled; else set native think=False."""
    if provider != "ollama":
        return completion_kwargs
    if resolve_structured_extraction_thinking():
        return completion_kwargs
    completion_kwargs["think"] = False   # top-level; litellm 1.84.0 forwards to /api/chat data.think
    return completion_kwargs
```

The mutator edits `completion_kwargs` in place and returns it. It sets ONLY the native `think` key
(no message mutation), so the `/no_think`-in-messages injector (`NO_THINK_DIRECTIVE` /
`_inject_no_think`) is removed.

### Call-site integration (7 sites)

Each in-scope site mutates its `completion_kwargs` immediately before `litellm.acompletion`:

```python
apply_structured_extraction_thinking(completion_kwargs, provider)   # sets think=False when SST disables thinking
```

The two formerly-legacy sites (`_handle_search_rerank`, `classify_drive_file`) additionally switch
`model` from `{provider}/{model}` to `ollama_chat/{model}` for ollama (and pass `api_base` from
`OLLAMA_URL`).

### SST wiring (fail-loud, no default)

- `config/smackerel.yaml` → `services.ml.structured_extraction_thinking: false`.
- `scripts/commands/config.sh` → `ML_STRUCTURED_EXTRACTION_THINKING="$(required_value services.ml.structured_extraction_thinking)"` (read) + `ML_STRUCTURED_EXTRACTION_THINKING=${ML_STRUCTURED_EXTRACTION_THINKING}` (emit), next to `ml_ollama_keep_alive`.
- `ml/app/main.py::_check_required_config` → validate ollama-conditionally (mirrors `ML_OLLAMA_KEEP_ALIVE`): required + must be `true`/`false`, else `sys.exit(1)`.
- `ml/tests/conftest.py` → `os.environ.setdefault("ML_STRUCTURED_EXTRACTION_THINKING", "false")` (developer-ergonomic seed, same pattern as `ML_OLLAMA_KEEP_ALIVE`; NOT a production default).

## Affected Files

| File | Change |
|------|--------|
| `ml/app/ollama_thinking.py` | REWORKED — resolver + native `think=False` kwargs mutator |
| `ml/app/domain.py` | set `think=False` at `_do_domain_extract` completion_kwargs |
| `ml/app/synthesis.py` | set at `handle_extract` + `handle_crosssource` |
| `ml/app/processor.py` | set at `process_content` completion_kwargs |
| `ml/app/nats_client.py` | migrate `_handle_search_rerank` legacy `ollama/` → `ollama_chat/` + set |
| `ml/app/card_categories.py` | set at `extract_card_categories` (already `ollama_chat/`) |
| `ml/app/drive_classify.py` | migrate `classify_drive_file` legacy `ollama/` → `ollama_chat/` (+ `api_base`) + set |
| `ml/app/main.py` | validate `ML_STRUCTURED_EXTRACTION_THINKING` (ollama-conditional) — UNCHANGED |
| `config/smackerel.yaml` | `services.ml.structured_extraction_thinking: false` — UNCHANGED |
| `scripts/commands/config.sh` | read + emit `ML_STRUCTURED_EXTRACTION_THINKING` — UNCHANGED |
| `ml/tests/conftest.py` | setdefault seed — UNCHANGED |
| `ml/tests/test_ollama_thinking.py` | REWORKED — resolver + native-`think` per-call-site + route-migration + agent-boundary tests |
| `ml/tests/test_main.py` | ollama config tests + adversarial required/invalid tests — UNCHANGED |

## Scope Boundary (deliberately unchanged)

`agent.py` (reasoning path), `main.py::_warmup_domain_model`, `nats_client::_handle_generate_digest`
(plain-text digest, its OWN inline `/no_think` — see the adjacent-finding note in report.md),
`routes/chat.py`. A dedicated test asserts the agent path does NOT carry `think=False` even when
SST=`false`.

## Regression Test Design

- **Resolver:** returns `True`/`False`; raises on unset / blank / invalid (adversarial no-default).
- **Mutator:** sets `think=False` when SST=`false`; no `think` key when SST=`true`; no `think` key
  for non-ollama provider; leaves other kwargs untouched.
- **Per-call-site (adversarial):** SST=`false` ⇒ captured `litellm.acompletion` kwargs carry
  `think=False`; SST=`true` ⇒ they do NOT (the second assertion FAILS if the fix hard-wires it on,
  the first FAILS if the fix is reverted).
- **Route migration (adversarial):** the two formerly-legacy calls (search-rerank, drive-classify)
  capture `model == ollama_chat/…`.
- **Scope boundary:** agent path does NOT carry `think=False` under SST=`false`.
- **Startup:** `_check_required_config` fails on missing (ollama) / invalid value.
