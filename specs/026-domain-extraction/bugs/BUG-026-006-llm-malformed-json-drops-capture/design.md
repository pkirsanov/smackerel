# Design: BUG-026-006 — Malformed / empty LLM JSON capture-preservation

## Root Cause Analysis

The ML sidecar's universal-processing path (`ml/app/processor.py::process_content`) parses the LLM's
response with a single strict `json.loads`. Three failure surfaces caused a malformed response to
silently DROP the user's capture instead of degrading gracefully:

1. **Single strict parse, no salvage.** `result = json.loads(result_text)` fails on any preamble /
   suffix or truncation. The retry loop caught only `(RateLimitError, ServiceUnavailableError,
   InternalServerError)` — never `json.JSONDecodeError` — so a malformed payload was never salvaged.

2. **The `json.JSONDecodeError` branch hard-dropped regardless of the SST gate.** Unlike the
   unavailable-LLM branch (which degrades gracefully when
   `ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=true`), the malformed-JSON branch returned
   `{"success": False, "error": "Invalid JSON from LLM: …"}` with no salvage and no SST-gated
   degraded fallback — so a truncated / prose-wrapped payload dropped the capture. Live prod core
   logs on `<deploy-host>` showed exactly this: repeated `ML processing failed … "Invalid JSON from
   LLM: Unterminated string"` under LIGHT host load (1.09/32).

3. **The truncation contributor: a hardcoded `max_tokens=2000`.** `"Unterminated string"` means the
   model's JSON was cut off before its closing quote/brace — classic output-token-budget exhaustion.
   The `max_tokens` was a hardcoded `2000` magic number, too small for the rich universal-processing
   schema on a verbose 26B model → truncation the parser then had to survive.

4. **The residual hole this session completes — an EMPTY / None content hard-dropped.** Some
   Ollama-served models return `content=None` on an overrun / aborted generation. `_parse_llm_json`
   fed that to `json.loads(None)`, which raises a `TypeError` — NOT a `json.JSONDecodeError`. The
   `except json.JSONDecodeError` degraded-fallback branch therefore did NOT catch it; it fell
   through to the generic `except Exception` handler, and because a `TypeError` is not an
   "unavailable-LLM" error, that handler returned `{"success": False, "error": "LLM processing
   failed"}` — a hard drop with an opaque error. This is the same malformed-response-drops-capture
   defect for the None/empty edge, on a path the earlier fix did not close.

## Fix Design

The fix keeps every unparseable LLM response — truncated, prose-wrapped, empty, or None — on ONE
capture-preserving path: the SST-gated `except json.JSONDecodeError` degraded fallback.

### Part A — tolerant JSON extraction (`_parse_llm_json`, landed 2026-07-08 + completed this session)

```python
def _parse_llm_json(text: str | None) -> Any:
    if text is None or not text.strip():
        # An EMPTY or None payload is as unrecoverable as a truncated one and
        # MUST route through the SAME except-JSONDecodeError degraded-fallback
        # branch. Raising a plain json.JSONDecodeError here — instead of letting
        # json.loads(None) raise a TypeError that would BYPASS that branch and
        # hard-drop the capture via the generic exception handler — keeps every
        # unparseable response on one capture-preserving path.
        raise json.JSONDecodeError("empty LLM payload", text or "", 0)
    try:
        return json.loads(text)                    # strict parse
    except json.JSONDecodeError:
        start = text.find("{"); end = text.rfind("}")
        if start != -1 and end > start:
            return json.loads(text[start : end + 1])  # salvage widest {…} span
        raise                                      # truncated -> re-raise
```

- The `None`/empty guard is the **completion this session** (redteam F2 residual). It converts the
  `TypeError` hard-drop into a `json.JSONDecodeError` so an empty/None payload lands on the
  capture-preserving branch, consistent with truncated and prose-wrapped payloads.
- The salvage span recovers a JSON object wrapped in a prose preamble/suffix; a truly truncated
  payload has no closing brace and re-raises, routing to the degraded fallback.

### Part B — SST-gated degraded fallback on the `json.JSONDecodeError` branch (landed 2026-07-08)

The `except json.JSONDecodeError` branch mirrors the unavailable-LLM branch: when
`_processing_degraded_fallback_enabled()` returns `True` it returns `success: True` with a low-signal
result (`topics:["degraded-fallback-malformed-json"]`, title from `content[:100]`,
`source_quality:"low"`), preserving the capture. When the gate is disabled / misconfigured it
returns the hard `{"success": False, "error": "Invalid JSON from LLM: …"}` — no silent success.

### Part C — SST-owned output-token budget (landed 2026-07-09, spec 102 SCOPE-102-03)

`max_tokens` reads `resolve_domain_output_token_budget()` (SST key
`services.ml.domain_output_token_budget` → `ML_DOMAIN_OUTPUT_TOKEN_BUDGET`, fail-loud, no default;
default raised `2000 → 4096`), replacing the hardcoded `2000` so the rich schema is less likely to
truncate mid-object in the first place.

## Affected Files

| File | Change |
|------|--------|
| `ml/app/processor.py` | `_parse_llm_json` None/empty guard (this session) — raise `json.JSONDecodeError` for `None`/whitespace instead of a `TypeError` hard-drop; existing tolerant salvage + SST-gated `except json.JSONDecodeError` degraded fallback (landed 2026-07-08); `max_tokens = resolve_domain_output_token_budget()` (landed 2026-07-09) |
| `ml/tests/test_processor.py` | `test_none_llm_content_uses_sst_gated_degraded_fallback` + `test_none_llm_content_hard_fails_when_fallback_disabled` (adversarial, this session); existing `test_malformed_json_uses_sst_gated_degraded_fallback`, `test_json_with_prose_wrapper_is_salvaged`, `test_malformed_json_hard_fails_when_fallback_disabled`, `test_output_budget_read_from_sst_not_hardcoded_spec102` |
| `config/smackerel.yaml` | `services.ml.processing_degraded_fallback_enabled` + `services.ml.domain_output_token_budget: 4096` (SST source, landed) — UNCHANGED this session |

## Scope Boundary (deliberately unchanged)

- The retry policy (only transient litellm errors are retried; malformed JSON is salvaged/degraded,
  not retried).
- The unavailable-LLM degraded-fallback branch (`_is_llm_unavailable_error`) — untouched.
- The BUG-061-002 missing-required-fields degradation (`title`/`artifact_type` defaulting) —
  untouched.
- Any build, deploy, host mutation, or push, and the live model-quality / latency measurement
  (routed to bubbles.devops, R-102-D, non-gating).

## Root Cause → Fix Traceability

| Root cause | Fix part | Proven by |
|------------|----------|-----------|
| Single strict parse, no salvage | Part A salvage span | `test_json_with_prose_wrapper_is_salvaged` |
| Malformed branch hard-drops regardless of SST | Part B SST-gated degraded fallback | `test_malformed_json_uses_sst_gated_degraded_fallback` / `..._hard_fails_when_fallback_disabled` |
| `None`/empty content hard-drops via TypeError | Part A None/empty guard (this session) | `test_none_llm_content_uses_sst_gated_degraded_fallback` / `..._hard_fails_when_fallback_disabled` |
| Hardcoded `max_tokens=2000` truncates the schema | Part C SST-owned budget | `test_output_budget_read_from_sst_not_hardcoded_spec102` |
| Model quality + 71–95s latency (truncation source) | routed to bubbles.devops (R-102-D) | committed live proof-of-record (bug.md), non-gating |
