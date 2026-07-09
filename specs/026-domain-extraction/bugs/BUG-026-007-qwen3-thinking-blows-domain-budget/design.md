# Design: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

## Root Cause Analysis

`qwen3:30b-a3b` is a **thinking** model: by default it emits a hidden `<think>…</think>`
chain-of-thought block before its answer. On the smackerel ML sidecar's **structured-JSON
extraction** calls (where the model output is a machine-parsed JSON object), that reasoning block
is pure latency. Live on evo-x2 (both models warm, identical recipe task) it costs **~113s** vs
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

### Mechanism: `/no_think` control token (not top-level `think=False`)

The user's live test used ollama's native top-level `"think": false` and it worked (13s vs 113s).
But that hit ollama's native API directly. Through **litellm**, the two candidate mechanisms differ
by route:

| Mechanism | `ollama_chat/` (`/api/chat`) route | legacy `ollama/` (`/api/generate`) route |
|-----------|------------------------------------|------------------------------------------|
| top-level `think=False` param | forwarded to request top level (like `keep_alive`) | **buried under `options` → Ollama ignores it** (same trap `ollama_keepalive.py` documents) |
| `/no_think` in messages | model sees it (system/user text) | model sees it (flattened into the prompt) |

In-scope call sites use **both** routes:

- `ollama_chat/`: `domain`, `synthesis` (extract + crosssource), `processor`, `card_categories`.
- legacy `ollama/` (`{provider}/{model}`): `nats_client` search-rerank, `drive_classify`.

A top-level `think=False` would be a **silent no-op** on the two legacy-routed calls. Therefore the
**robust, route-agnostic choice is `/no_think` injected into the request messages** — which is
also:

- **version-agnostic** (no dependence on a specific litellm passthrough),
- a **documented no-op on non-qwen models** (`gemma3:4b` in dev/test ignores it), and
- **already proven in this repo** — `nats_client.py::_handle_generate_digest` prepends `/no_think`
  to its prompt to suppress reasoning on the digest path.

A unit test proves the request actually carries `/no_think` for each in-scope call site.

### New module `ml/app/ollama_thinking.py`

Mirrors `ollama_keepalive.py` exactly (fail-loud SST resolver, no default):

```python
def resolve_structured_extraction_thinking() -> bool:
    """True = keep qwen thinking; False = disable (inject /no_think). Fail-loud."""
    raw = os.environ.get("ML_STRUCTURED_EXTRACTION_THINKING", "").strip().lower()
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise RuntimeError("ML_STRUCTURED_EXTRACTION_THINKING is required and must be 'true' or 'false' …")

def apply_structured_extraction_thinking(messages, provider) -> list[dict]:
    """No-op for non-ollama providers and when thinking is enabled; else inject /no_think."""
    if provider != "ollama":
        return messages
    if resolve_structured_extraction_thinking():
        return messages
    return _inject_no_think(messages)   # prefer system message, else first user message
```

`_inject_no_think` copies the messages (no mutation of caller state), prepends `/no_think` to the
system message if present, else to the first user message, and is idempotent (won't double-inject).

### Call-site integration (7 sites)

Each in-scope site adjusts its request messages immediately before `litellm.acompletion`:

```python
completion_kwargs["messages"] = apply_structured_extraction_thinking(completion_kwargs["messages"], provider)
# or, for the inline-arg sites:
messages = apply_structured_extraction_thinking([...], provider)
```

### SST wiring (fail-loud, no default)

- `config/smackerel.yaml` → `services.ml.structured_extraction_thinking: false`.
- `scripts/commands/config.sh` → `ML_STRUCTURED_EXTRACTION_THINKING="$(required_value services.ml.structured_extraction_thinking)"` (read) + `ML_STRUCTURED_EXTRACTION_THINKING=${ML_STRUCTURED_EXTRACTION_THINKING}` (emit), next to `ml_ollama_keep_alive`.
- `ml/app/main.py::_check_required_config` → validate ollama-conditionally (mirrors `ML_OLLAMA_KEEP_ALIVE`): required + must be `true`/`false`, else `sys.exit(1)`.
- `ml/tests/conftest.py` → `os.environ.setdefault("ML_STRUCTURED_EXTRACTION_THINKING", "false")` (developer-ergonomic seed, same pattern as `ML_OLLAMA_KEEP_ALIVE`; NOT a production default).

## Affected Files

| File | Change |
|------|--------|
| `ml/app/ollama_thinking.py` | NEW — resolver + injector |
| `ml/app/domain.py` | inject at `_do_domain_extract` completion_kwargs |
| `ml/app/synthesis.py` | inject at `handle_extract` + `handle_crosssource` |
| `ml/app/processor.py` | inject at `process_content` completion_kwargs |
| `ml/app/nats_client.py` | inject at `_handle_search_rerank` |
| `ml/app/card_categories.py` | inject at `extract_card_categories` |
| `ml/app/drive_classify.py` | inject at `classify_drive_file` |
| `ml/app/main.py` | validate `ML_STRUCTURED_EXTRACTION_THINKING` (ollama-conditional) |
| `config/smackerel.yaml` | add `services.ml.structured_extraction_thinking: false` |
| `scripts/commands/config.sh` | read + emit `ML_STRUCTURED_EXTRACTION_THINKING` |
| `ml/tests/conftest.py` | setdefault seed |
| `ml/tests/test_ollama_thinking.py` | NEW — resolver + per-call-site + agent-boundary tests |
| `ml/tests/test_main.py` | add key to ollama config tests + adversarial required/invalid tests |

## Scope Boundary (deliberately unchanged)

`agent.py` (reasoning path), `main.py::_warmup_domain_model`, `nats_client::_handle_generate_digest`
(already `/no_think`), `routes/chat.py`. A dedicated test asserts the agent path composes requests
WITHOUT `/no_think` even when SST=`false`.

## Regression Test Design

- **Resolver:** returns `True`/`False`; raises on unset / blank / invalid (adversarial no-default).
- **Injector:** injects into system message; injects into user-only shape; no-op when SST=`true`;
  no-op for non-ollama provider; idempotent.
- **Per-call-site (adversarial):** SST=`false` ⇒ captured `litellm.acompletion` messages contain
  `/no_think`; SST=`true` ⇒ they do NOT (this second assertion FAILS if the fix hard-wires the
  token on, and the first FAILS if the fix is reverted).
- **Scope boundary:** agent path composes messages WITHOUT `/no_think` under SST=`false`.
- **Startup:** `_check_required_config` fails on missing (ollama) / invalid value.
