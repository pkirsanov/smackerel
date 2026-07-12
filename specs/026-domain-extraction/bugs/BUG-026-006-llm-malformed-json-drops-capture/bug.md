# BUG-026-006 — Malformed/truncated LLM JSON hard-drops the capture

- **Severity:** HIGH (redteam **F2**)
- **Owning spec:** `026-domain-extraction` (owns the "Invalid JSON from LLM" ML-processing contract)
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on <deploy-host>
- **Status:** CODE-RESILIENCE PART FIXED IN-REPO + MODEL/OPS PART ROUTED — not pushed

## Summary

Live prod core logs showed repeated `ML processing failed … "Invalid JSON from LLM: Unterminated
string"`, `domain extraction exceeded 30s budget`, and synthesis `processing_ms: 71724/94792`
(71–95s) on `ollama/gemma4:26b`, **under LIGHT host load (1.09/32)** — so this is not the
documented host-contention gotcha. Two independent problems:

1. **Code resilience (in-repo, fixed):** the ML sidecar's `except json.JSONDecodeError` branch
   returned `{"success": False}` **regardless of the SST degraded-fallback gate**, so a malformed
   or truncated LLM payload **hard-dropped the capture** (unlike the unavailable-LLM branch, which
   degrades gracefully).
2. **Model/ops (routed):** `gemma4:26b` emits **truncated** JSON ("Unterminated string" = output
   cut off mid-string) and 71–95s latency under light load — a model-quality / output-budget /
   routing problem, not a code bug.

## Reproduction

**Redteam (live prod):** core logs — `Invalid JSON from LLM: Unterminated string`,
`domain extraction exceeded 30s budget`, `processing_ms: 71724`/`94792`, model `ollama/gemma4:26b`.

**In-repo static confirmation ([ml/app/processor.py](../../../../ml/app/processor.py)):**

- `result = json.loads(result_text)` — single strict parse; a preamble/suffix or truncation raises.
- Retry loop caught only `(RateLimitError, ServiceUnavailableError, InternalServerError)` — **not**
  `json.JSONDecodeError`.
- `except json.JSONDecodeError:` → `return {"success": False, "error": "Invalid JSON from LLM: …"}`
  with **no** salvage and **no** SST-gated degraded fallback (the generic `except Exception` branch
  *does* have the fallback). Net effect: malformed JSON ⇒ capture dropped.

## Root cause

- **"Unterminated string"** = the model's JSON was **truncated** before the closing quote/brace —
  classic output-token-budget exhaustion (`max_tokens=2000`, hardcoded) on a verbose 26B model
  filling the rich schema, and/or model quality.
- The parser was single-attempt strict, and the JSONDecodeError branch treated *any* unparseable
  output as an unrecoverable hard failure that **drops the user's capture**.

## Fix (in-repo — code resilience only)

[ml/app/processor.py](../../../../ml/app/processor.py):

1. `_parse_llm_json(text)` — strict parse, then **tolerant salvage** of the widest `{…}` span
   (recovers JSON wrapped in a prose preamble/suffix that `response_format=json_object` doesn't
   suppress for every Ollama model). Truncated payloads have no closing brace → re-raise.
2. The `except json.JSONDecodeError` branch now mirrors the unavailable-LLM branch: when the SST
   gate (`ML_PROCESSING_DEGRADED_FALLBACK_ENABLED`) is **enabled**, it **degrades gracefully**
   (capture preserved, `topics:["degraded-fallback-malformed-json"]`, logged — no longer silent)
   instead of hard-dropping. When the gate is **disabled/misconfigured**, the pre-existing hard
   error is preserved (no silent success).

[ml/tests/test_processor.py](../../../../ml/tests/test_processor.py):
`test_malformed_json_uses_sst_gated_degraded_fallback` (adversarial — truncated payload),
`test_json_with_prose_wrapper_is_salvaged` (adversarial — prose-wrapped JSON),
`test_malformed_json_hard_fails_when_fallback_disabled` (SST-gate integrity).

## Routed (model/ops — with evidence)

**Owner: bubbles.devops / model-selection ops.** The TRUNCATION + latency root cause is not a code
bug:

- `gemma4:26b` produces truncated JSON and 71–95s synthesis / >30s domain-extraction under light
  load.
- `max_tokens=2000` is **hardcoded** in `processor.py` and is likely too small for the rich schema
  on a verbose model → truncation.
- **Recommendation:** SST-own the output-token budget (raise it so the schema fits) and/or route
  domain-extraction + synthesis to a model with adequate output budget/latency; treat the 30s
  domain budget vs 71–95s observed latency as a model-selection SLO decision. Changing the model /
  token budget trades latency/cost and is an ops call, so it is **not** made here.

## Test evidence

**RED (pre-fix source, new tests) — `./smackerel.sh test unit --python` with fix stashed:**

```
FAILED ml/tests/test_processor.py::TestProcessContentErrors::test_malformed_json_uses_sst_gated_degraded_fallback
FAILED ml/tests/test_processor.py::TestProcessContentErrors::test_json_with_prose_wrapper_is_salvaged
2 failed, 518 passed, 2 skipped in 8.77s
___PY_RED_EXIT=1___
```

**GREEN (fix in place) — `./smackerel.sh test unit`:**

```
520 passed, 2 skipped in 9.27s
[py-unit] pytest ml/tests finished OK
___FULL_UNIT_EXIT=0___
```

## Redeploy note

The resilience fix is in the built `smackerel-ml` runtime; the running prod image is **unchanged**
until an operator-gated redeploy of `smackerel-ml`. The routed model/token-budget change is
separate and also requires an ops action. No push / redeploy performed here.
