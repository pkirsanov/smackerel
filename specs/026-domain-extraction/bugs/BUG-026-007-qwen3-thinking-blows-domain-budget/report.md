# Report: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

### Summary

qwen3's default thinking-mode adds a hidden `<think>` reasoning block (>150s live on the shared
<deploy-host> ollama daemon) before its answer on the ML sidecar's structured-JSON extraction calls —
blowing `DOMAIN_EXTRACTION_TIMEOUT = 30` (silent degrade) AND prefixing other structured callers
with a `<think>` block that trips `LLM returned invalid JSON` (the F2 invalid-JSON failure). Fixed
by an SST-gated, fail-loud thinking-disable (`ML_STRUCTURED_EXTRACTION_THINKING` /
`services.ml.structured_extraction_thinking`) that sets the NATIVE Ollama `think=False` request
field on each structured-extraction `litellm.acompletion` call, while leaving the agent reasoning
path thinking-ON.

### ⚠️ Mechanism Correction (this report supersedes the first fix)

**The FIRST fix was INEFFECTIVE and has been replaced.** It injected the qwen `/no_think` control
token into the request messages. Measured live on <deploy-host> (shared daemon, warm qwen3), qwen3's Ollama
chat template **ignores** the `/no_think` text directive — thinking stayed ON (>150s), and the
resulting `<think>`-prefixed output produced the live `ERROR smackerel-ml.synthesis LLM returned
invalid JSON: Expecting value: line 1 column 1`. qwen3 honors ONLY the native `think` request field:

| Mechanism (live <deploy-host>, shared daemon, warm qwen3) | Thinking | Wall / compute | JSON |
|-----------------------------------------------------|----------|----------------|------|
| `/no_think` in prompt messages (the FIRST fix)      | **STILL ON** | >150s | invalid (`<think>` prefix) |
| native `think=False` (ollama `/api/chat` top-level) | **OFF** | trivial prompt `load=0.1s prompt_eval=0.1s gen=0.9s eval_tok=6` (~1s compute) vs `think=True`=119s | valid |

The 30–60s wall-times seen earlier were daemon queueing CAUSED by the ml's own thinking-ON calls
saturating the shared daemon; fixing the mechanism removes that self-inflicted saturation.

### Root Cause

See [design.md](design.md) → "Root Cause Analysis" and the live <deploy-host> evidence in [bug.md](bug.md).
qwen3 (thinking model) defaults to a hidden `<think>` block before its JSON; on the extraction path
that is pure latency (>150s vs ~1s compute), exceeding the 30s domain budget → `asyncio.TimeoutError`
→ degraded fallback, and tripping invalid-JSON on the non-budgeted structured callers.

### Mechanism Decision — native `think`, verified against the pinned litellm

The mechanism is the **native top-level `think=False` kwarg** on `litellm.acompletion`. Verified
against the sidecar-pinned `litellm==1.84.0` (`ml/requirements.txt`) with an empirical
request-capture probe (a local HTTP server impersonating Ollama, capturing the exact JSON body
litellm builds — full output under "Test Evidence"):

- `litellm.completion(model="ollama_chat/qwen3…", think=False)` → request body `"think": false` at
  the **TOP LEVEL** of `/api/chat`. ✅ forwarded.
- `extra_body={"think": False}` → identical top-level result. ✅ (both forms work; top-level `think=`
  chosen as the simpler form, matching the operator's live `litellm.acompletion(..., think=False)`).
- `reasoning_effort="low"` → maps to `think=True` (WRONG direction — `"low"|"medium"|"high"` are
  truthy). Not used.
- The legacy `ollama/` (`/api/generate`) transform ALSO forwards `think` top-level in 1.84.0 — but
  `keep_alive` is buried under `options` there, so the two formerly-legacy routes (search-rerank,
  drive-classify) are still migrated to `ollama_chat/` for role fidelity + `keep_alive` parity +
  consistency with the other structured sites.

Source: `litellm/llms/ollama/{chat,completion}/transformation.py` →
`think = optional_params.pop("think", None); if think is not None: data["think"] = think`.

### Changes

| File | Change |
|------|--------|
| `ml/app/ollama_thinking.py` | REWORKED — `apply_structured_extraction_thinking(completion_kwargs, provider)` now sets native `completion_kwargs["think"] = False` (was: `/no_think` message injector). Fail-loud `resolve_structured_extraction_thinking()` kept as-is; `NO_THINK_DIRECTIVE` / `_inject_no_think` removed. |
| `ml/app/domain.py` | mutate `completion_kwargs` (`think=False`) at `_do_domain_extract` (the 30s-budget path) |
| `ml/app/synthesis.py` | mutate at `handle_extract` + `handle_crosssource` |
| `ml/app/processor.py` | mutate at `process_content` |
| `ml/app/card_categories.py` | build kwargs + mutate at `extract_card_categories` (already `ollama_chat/`) |
| `ml/app/nats_client.py` | `_handle_search_rerank` migrated legacy `ollama/` → `ollama_chat/` + mutate |
| `ml/app/drive_classify.py` | `classify_drive_file` migrated legacy `ollama/` → `ollama_chat/` (+ `api_base` from `OLLAMA_URL`, `import os`) + mutate |
| `ml/tests/test_ollama_thinking.py` | REWORKED — assert native `think=False` per call site + `ollama_chat/` route for the two migrated calls; `/no_think`-in-messages assertions removed |

SST wiring UNCHANGED (switch semantics identical — `false` = thinking disabled): `config/smackerel.yaml`
`services.ml.structured_extraction_thinking`, `scripts/commands/config.sh` emit, `ml/app/main.py`
`_check_required_config`, `ml/tests/conftest.py` seed, `ml/tests/test_main.py` config tests.
`ml/app/agent.py` UNCHANGED (reasoning path keeps thinking).

### Tests Reworked

| Test | Type | Asserts |
|------|------|---------|
| `test_resolve_*` (returns true/false, fail-loud unset/blank/invalid) | unit | resolver contract (UNCHANGED) |
| `test_apply_sets_native_think_false_when_disabled` | unit (adversarial) | mutator sets `think=False`, returns same dict, adds NO `/no_think` |
| `test_apply_is_noop_when_thinking_enabled` | unit (adversarial) | no `think` key when SST=true |
| `test_apply_is_noop_for_non_ollama_provider` | unit | no `think` key; resolver not consulted |
| `test_apply_does_not_disturb_other_kwargs` | unit | only `think` added; messages/temperature/keep_alive untouched |
| `test_domain_extract_disables_thinking_when_sst_false` | unit (adversarial) | domain request carries `think=False` |
| `test_domain_extract_keeps_thinking_when_enabled` | unit (adversarial) | NO `think` key when SST=true |
| `test_process_content_disables_thinking_when_sst_false` | unit (adversarial) | processor request carries `think=False` |
| `test_synthesis_extract_disables_thinking_when_sst_false` | unit (adversarial) | synthesis extract carries `think=False` |
| `test_synthesis_crosssource_disables_thinking_when_sst_false` | unit (adversarial) | crosssource carries `think=False` |
| `test_search_rerank_disables_thinking_and_uses_ollama_chat` | unit (adversarial) | rerank carries `think=False` AND `model == ollama_chat/…` (route migration) |
| `test_card_categories_disables_thinking_when_sst_false` | unit (adversarial) | card-categories carries `think=False` AND `model == ollama_chat/…` |
| `test_drive_classify_disables_thinking_and_uses_ollama_chat` | unit (adversarial) | drive-classify carries `think=False` AND `model == ollama_chat/…` (route migration) |
| `test_agent_path_keeps_thinking_even_when_disabled` | unit (scope boundary) | agent request does NOT carry `think=False` |
| `test_check_required_config_*_structured_extraction_thinking` | unit | fail-loud required/invalid (UNCHANGED) |

## Test Evidence

> Captured from ACTUAL `./smackerel.sh test unit --python` runs (Docker `pip install -e ./ml[dev]`
> installs the real `litellm==1.84.0`, then `pytest ml/tests`) + an isolated litellm probe. Claim
> Source tags per `evidence-rules.md`.

### litellm 1.84.0 `think`-forwarding probe (mechanism verification)

**Claim Source:** executed — isolated Python 3.12 venv (matching the sidecar `python:3.12-slim`),
`pip install litellm==1.84.0`, a local HTTP server impersonating Ollama capturing the request body:

```
=== ollama_chat/ (/api/chat) ===
[chat top-level think=False]  path=/api/chat  top_level_think=False  options_think='<absent>'
[chat extra_body think=False] path=/api/chat  top_level_think=False  options_think='<absent>'
[chat reasoning_effort=low]   path=/api/chat  top_level_think=True   options_think='<absent>'
[chat baseline (no think)]    path=/api/chat  top_level_think='<absent>'
=== legacy ollama/ (/api/generate) ===
[gen top-level think=False]   path=/api/generate  top_level_think=False  options_think='<absent>'
[gen extra_body think=False]  path=/api/generate  top_level_think=False  options_think='<absent>'
```

Conclusion: a top-level `think=False` kwarg IS forwarded to the Ollama request TOP LEVEL by litellm
1.84.0 (both routes); `reasoning_effort` is the wrong lever (maps `"low"`→`think=True`).

### Pre-Fix / adversarial (MUST FAIL) — RED

**Claim Source:** executed — with the native `think=False` mutator temporarily neutralized in
`ollama_thinking.py` (restored immediately after), the 9 mechanism / per-call-site tests fail. The
route-migration `model == ollama_chat/…` assertions still hold (migration lives in the call sites),
so the migrated-route tests fail ONLY on the `think` assertion — proving they detect the mechanism:

```
>       assert _think_disabled(captured), captured
E        +  where False = _think_disabled({'model': 'ollama_chat/qwen3:30b-a3b', ...})
...
FAILED ml/tests/test_ollama_thinking.py::test_apply_sets_native_think_false_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_apply_does_not_disturb_other_kwargs
FAILED ml/tests/test_ollama_thinking.py::test_domain_extract_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_process_content_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_extract_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_crosssource_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_search_rerank_disables_thinking_and_uses_ollama_chat
FAILED ml/tests/test_ollama_thinking.py::test_card_categories_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_drive_classify_disables_thinking_and_uses_ollama_chat
9 failed, 547 passed, 2 skipped in 7.25s
```

### Post-Fix (MUST PASS) — GREEN

**Claim Source:** executed — native `think=False` mutator restored; full ml unit suite green:

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
......................................................                   [100%]
556 passed, 2 skipped in 12.38s
[py-unit] pytest ml/tests finished OK
```

### Full ml unit suite (no regressions)

**Claim Source:** executed — same GREEN run: `556 passed, 2 skipped`. The 9 reworked
`test_ollama_thinking.py` tests pass and no sibling test (`test_drive_classify`, `test_nats_client`,
`test_processor`, `test_synthesis`, `test_card_categories`, `test_main`) regressed under the
`ollama_chat/` route migration.

### Bailout scan (no silent-pass patterns in the regression tests)

**Claim Source:** executed — the reworked tests assert directly on `captured["think"] is False` and
`captured["model"] == "ollama_chat/…"`. The only `return` hits are the mock `_capture` side-effects
and the `_think_disabled` / `_domain_data` helpers (returning the fake litellm response / a bool /
fixture data) — NOT test-body bailouts. No `pytest.skip` / `assert True` / conditional early-return
short-circuits an assertion.

## Redeploy / Live-Verification Note (anti-fabrication)

This is a **code change to `smackerel-ml`**. It takes effect only after the orchestrator rebuilds +
signs + redeploys `smackerel-ml` on <deploy-host>. The live "domain+synthesis fast + valid JSON" outcome is
therefore **PENDING that redeploy** and is NOT claimed here. No build, deploy, host mutation, or push
was performed in this repo — local commit only.

### Adjacent finding (out of scope, noted honestly)

`ml/app/nats_client.py::_handle_generate_digest` (the plain-text digest path) still uses its OWN
inline `/no_think` prompt prefix. If it runs on qwen3, that token is likely ALSO ineffective (same
root cause). It is OUT of scope for BUG-026-007 (plain-text digest, not a structured-JSON extraction
call) and is left unchanged; flagged here for a follow-up bug rather than silently expanded into.

### Completion Statement

Mechanism corrected to the native Ollama `think` field (verified forwarded by litellm 1.84.0). Code
+ reworked unit tests authored and passing locally (RED 9-fail → GREEN 556-pass). Live verification
pending orchestrator redeploy of `smackerel-ml`.
