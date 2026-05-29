# Bug: [BUG-061-002] ML LLM extraction fails on short / minimal text — ValueError at ml/app/processor.py:178

## Summary
`process_content()` in `ml/app/processor.py` raises `ValueError("Missing required field: artifact_type")` (line 178) whenever the upstream LLM returns a JSON payload that omits `artifact_type` or `title`. This is the *expected* LLM behaviour for very short or low-signal inputs (e.g. `"hi"`, single-word capture, a stray emoji), and also legitimately occurs for `processing_tier="light"` where the prompt itself only asks for `title, summary, topics, sentiment`. The exception is swallowed by the outer `except Exception`, surfacing only as an opaque `{"success": false, "error": "LLM processing failed"}` — the entire ingestion of that artifact silently fails with no actionable signal.

## Severity
- [ ] Critical
- [x] High — silently drops valid user captures with no observable error and no recovery
- [ ] Medium
- [ ] Low

## Status
- [ ] Reported
- [x] Confirmed
- [ ] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

## Reproduction Steps
1. Invoke `process_content(content="hi", content_type="generic", processing_tier="light", ...)` against an LLM that returns a partial JSON payload such as `{"summary": "A brief greeting.", "topics": ["greeting"], "sentiment": "neutral"}` (i.e. no `artifact_type`, no `title`).
2. Observe the function returning `{"success": False, "error": "LLM processing failed"}` and an internal `ValueError: Missing required field: artifact_type` logged at `ml/app/processor.py:178`.

## Expected Behavior
Short / minimal text input MUST NOT cause a hard failure. The processor SHOULD degrade gracefully by:
- Deriving `title` from the first ~100 characters of the input content (mirroring the existing degraded-fallback branch).
- Deriving `artifact_type` from the supplied `content_type` (or `"note"` when generic/unset).
- Emitting a structured WARN log indicating which fields were defaulted.
- Returning `success=True` with a real, persistable result so the artifact is captured rather than silently dropped.

## Actual Behavior (Before Fix — Verbatim Repro)
```
LLM processing failed
Traceback (most recent call last):
  File "~/smackerel/ml/app/processor.py", line 178, in process_content
    raise ValueError(f"Missing required field: {field}")
ValueError: Missing required field: artifact_type
REPRO_RESULT: {
  "success": false,
  "error": "LLM processing failed"
}
```
(Captured via in-process repro with `app.processor.litellm.acompletion` patched to return the partial payload above. Full transcript in `report.md` → "Bug Reproduction — Before Fix".)

## Environment
- Service: smackerel-ml (FastAPI sidecar)
- Code: `ml/app/processor.py` line 178 (`raise ValueError(f"Missing required field: {field}")`)
- Affected callers: every NATS-driven processing handler that funnels through `process_content` (intelligence + extraction pipelines).
- Surfaced during: spec 061 SCOPE-06 BS-002 live-stack debugging (rounds 49-59, commits `e2540aeb..d4111aa5`); deferred for an independent bubbles.bug invocation per operator routing in Round 56.

## Root Cause
The validation block at `ml/app/processor.py` lines 175-178 enforces `artifact_type` and `title` as hard requirements:

```python
required_fields = ["artifact_type", "title"]
for field in required_fields:
    if field not in result:
        raise ValueError(f"Missing required field: {field}")
```

This contract is incompatible with three of the prompt's own tier rules:
- `processing_tier="light"` only requests `title, summary, topics, sentiment` — `artifact_type` is intentionally absent.
- `processing_tier="metadata"` only requests `title, artifact_type` — fine, but `title` is still LLM-discretionary.
- Short / low-signal inputs (single tokens, emoji, URL-only captures) cause every tier to omit one or both fields in practice.

The raised `ValueError` is then caught by the broad `except Exception` block (line 196), which:
1. Discards the precise error message (`"Missing required field: artifact_type"`) and substitutes a generic `"LLM processing failed"`.
2. Bypasses the degraded-fallback branch because `_is_llm_unavailable_error` looks for network keywords and a `ValueError` matches none of them.

The net effect: every short-text capture becomes a silent ingestion drop with no actionable diagnostic.

## Related
- Feature: `specs/061-conversational-assistant/`
- Surfaced in: spec 061 SCOPE-06 BS-002 (rounds 49-59, `state.json.execution.executionHistory[-1]` summary)
- Sibling bug: `BUG-061-001-bs001-webhook-cold-start-timing` (Round 11)
- No related PRs (in-tree fix)
