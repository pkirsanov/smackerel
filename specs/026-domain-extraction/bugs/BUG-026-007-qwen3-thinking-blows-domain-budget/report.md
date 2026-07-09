# Report: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

### Summary

qwen3's default thinking-mode added a ~113s hidden reasoning block to the ML sidecar's
structured-JSON extraction calls, blowing `DOMAIN_EXTRACTION_TIMEOUT = 30` and silently degrading
domain extraction in prod. Fixed by an SST-gated, fail-loud thinking-disable
(`ML_STRUCTURED_EXTRACTION_THINKING` / `services.ml.structured_extraction_thinking`) that injects
the qwen `/no_think` control token into the structured-extraction request messages, while leaving
the agent reasoning path thinking-ON.

### Root Cause

See [design.md](design.md) → "Root Cause Analysis" and the live evo-x2 latency table in
[bug.md](bug.md). qwen3 (thinking model) defaults to a hidden `<think>` block before its JSON;
on the extraction path that is pure latency (~113s vs ~10s, identical 9/9 accuracy), exceeding the
30s domain budget → `asyncio.TimeoutError` → degraded fallback.

### Mechanism Decision

`/no_think` control token injected into the request messages, **not** a top-level `think=False`
param — because the `nats_client` search-rerank and `drive_classify` calls route through litellm's
legacy `ollama/` (`/api/generate`) transform, which buries unknown top-level params under `options`
where Ollama never sees them (the same trap `ollama_keepalive.py` documents for `keep_alive`).
`/no_think` is route-agnostic, version-agnostic, a no-op on non-qwen models, and already used by
`nats_client.py::_handle_generate_digest` in this repo (proven precedent). See [design.md](design.md)
→ "Mechanism".

### Changes

| File | Change |
|------|--------|
| `ml/app/ollama_thinking.py` | NEW — `resolve_structured_extraction_thinking()` (fail-loud) + `apply_structured_extraction_thinking(messages, provider)` (`/no_think` injector) |
| `ml/app/domain.py` | inject at `_do_domain_extract` completion_kwargs (the 30s-budget path) |
| `ml/app/synthesis.py` | inject at `handle_extract` + `handle_crosssource` |
| `ml/app/processor.py` | inject at `process_content` completion_kwargs |
| `ml/app/nats_client.py` | inject at `_handle_search_rerank` |
| `ml/app/card_categories.py` | inject at `extract_card_categories` (`LLM_MODEL`, verified) |
| `ml/app/drive_classify.py` | inject at `classify_drive_file` (`LLM_MODEL` via NATS dispatch, verified) |
| `ml/app/main.py` | validate `ML_STRUCTURED_EXTRACTION_THINKING` (ollama-conditional, true/false, `sys.exit(1)`) |
| `config/smackerel.yaml` | add `services.ml.structured_extraction_thinking: false` |
| `scripts/commands/config.sh` | read + emit `ML_STRUCTURED_EXTRACTION_THINKING` |
| `ml/tests/conftest.py` | `setdefault("ML_STRUCTURED_EXTRACTION_THINKING", "false")` |
| `ml/tests/test_ollama_thinking.py` | NEW — resolver + per-call-site + agent-boundary tests |
| `ml/tests/test_main.py` | add key to ollama config tests + adversarial required/invalid tests |

### Tests Added

| Test | Type | Asserts |
|------|------|---------|
| `test_resolve_*` (returns true/false, fail-loud unset/blank/invalid) | unit | resolver contract |
| `test_apply_*` (system shape, user-only shape, SST=true no-op, non-ollama no-op, idempotent) | unit | injector contract |
| `test_domain_extract_injects_no_think_when_disabled` | unit (adversarial) | domain request carries `/no_think` |
| `test_domain_extract_keeps_thinking_when_enabled` | unit (adversarial) | domain request has NO `/no_think` when SST=true |
| `test_synthesis_extract_injects_no_think_when_disabled` | unit (adversarial) | synthesis extract request carries `/no_think` |
| `test_synthesis_crosssource_injects_no_think_when_disabled` | unit (adversarial) | crosssource request carries `/no_think` |
| `test_process_content_injects_no_think_when_disabled` | unit (adversarial) | processor request carries `/no_think` |
| `test_search_rerank_injects_no_think_when_disabled` | unit (adversarial) | rerank request carries `/no_think` |
| `test_card_categories_injects_no_think_when_disabled` | unit (adversarial) | card-categories request carries `/no_think` |
| `test_drive_classify_injects_no_think_when_disabled` | unit (adversarial) | drive-classify request carries `/no_think` |
| `test_agent_path_keeps_thinking_even_when_disabled` | unit (scope boundary) | agent request has NO `/no_think` |
| `test_check_required_config_requires_structured_extraction_thinking` | unit (adversarial) | fail-loud required-when-ollama |
| `test_check_required_config_rejects_invalid_structured_extraction_thinking` | unit | rejects non-true/false |

## Test Evidence

> Captured from ACTUAL `./smackerel.sh test unit --python` runs. Claim Source tags per
> `evidence-rules.md`.

### Pre-Fix Regression Test (MUST FAIL) — RED

**Claim Source:** executed

Tests authored, call sites NOT yet modified. The 7 per-call-site injection tests + 3 config tests
fail (the resolver / injector / agent-boundary / `keeps_thinking_when_enabled` tests already pass):

```
FAILED ml/tests/test_main.py::test_check_required_config_allows_ollama_without_api_key
FAILED ml/tests/test_main.py::test_check_required_config_requires_structured_extraction_thinking
FAILED ml/tests/test_main.py::test_check_required_config_rejects_invalid_structured_extraction_thinking
FAILED ml/tests/test_ollama_thinking.py::test_domain_extract_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_process_content_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_extract_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_crosssource_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_search_rerank_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_card_categories_injects_no_think_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_drive_classify_injects_no_think_when_disabled
10 failed, 548 passed, 2 skipped in 5.80s
```

Representative failure (the proven 30s-budget domain path — the request carries NO thinking-disable
before the fix):

```
>       assert _has_no_think(captured["messages"]), captured["messages"]
E       AssertionError: [{'role': 'system', 'content': 'You are a recipe extraction engine. Extract
E         structured recipe data from the provided c...pty string or zero).
E         '}, {'role': 'user', 'content': '
E         Content:
E         ---
E         Ingredients: flour. Instructions: bake.
E         ---'}]
E       assert False
E        +  where False = _has_no_think([{'role': 'system', 'content': 'You are a recipe extraction
E         engine. ...'}, {'role': 'user', 'content': '\nContent:\n---\nIngredients: flour. ...'}])
ml/tests/test_ollama_thinking.py:210: AssertionError
```

### Post-Fix Regression Test (MUST PASS) — GREEN

**Claim Source:** executed

Fix applied at all 7 in-scope call sites + `main.py` validation. The full ml unit suite is green
(the 10 previously-RED tests now pass; no other test changed):

```
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 12%]
....................................s................................... [ 25%]
........................................................................ [ 38%]
........................................................................ [ 51%]
........................................................................ [ 64%]
........................................................................ [ 77%]
........................................................................ [ 90%]
........................................................                 [100%]
558 passed, 2 skipped in 6.56s
[py-unit] pytest ml/tests finished OK
```

### Full ml unit suite (no regressions)

**Claim Source:** executed

Same run as GREEN above: `558 passed, 2 skipped` — the 548 pre-existing passing tests are unchanged
and the 10 new tests pass. No collateral regressions.

### SST wiring end-to-end (`./smackerel.sh config generate`)

**Claim Source:** executed

The new `required_value services.ml.structured_extraction_thinking` resolves and emits into the
generated env (no silent default). `config/generated/` is gitignored, so this adds no diff noise:

```
config-validate: .../config/generated/dev.env.tmp.63965 OK
Generated .../config/generated/dev.env

# config/generated/dev.env
388:ML_OLLAMA_KEEP_ALIVE=30m
389:ML_STRUCTURED_EXTRACTION_THINKING=false
```

### Bailout scan (no silent-pass patterns in the regression tests)

**Claim Source:** executed

Scan of `ml/tests/test_ollama_thinking.py` for silent-pass bailout patterns
(`pytest.skip` / `assert True` / test-body early `return` / `if …: return` / `pragma: no cover`):
the only `return` hits are inside the mock `_capture` side-effect functions and the `_has_no_think`
/ `_domain_data` helpers (returning the fake litellm response / a bool / fixture data) — NOT
test-body bailouts. Every test asserts directly on the forbidden behavior; none conditionally
short-circuits an assertion.

## Redeploy / Live-Verification Note

This is a **code change to `smackerel-ml`**. It takes effect only after the orchestrator rebuilds +
signs + redeploys `smackerel-ml` on evo-x2. The live "domain extraction now < 30s" outcome is
therefore **PENDING that redeploy** and is NOT claimed here (anti-fabrication). No build, deploy,
host mutation, or push was performed in this repo.

### Completion Statement

Code + unit tests authored and passing locally. Live verification pending orchestrator redeploy.
