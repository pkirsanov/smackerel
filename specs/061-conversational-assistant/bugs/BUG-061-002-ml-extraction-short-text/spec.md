# Spec: BUG-061-002 Expected Behavior

## Expected Behavior
`process_content()` in `ml/app/processor.py` MUST NOT raise / silently drop
the capture when the upstream LLM returns a JSON payload that omits
`artifact_type` or `title`. It MUST degrade gracefully:

1. When `title` is missing or empty → derive from `content[:100].strip()`,
   falling back to the literal string `"Untitled"` if the content is empty.
2. When `artifact_type` is missing or empty → derive from the supplied
   `content_type` if it is non-empty and not the placeholder
   `"generic"`; otherwise fall back to `"note"`.
3. Emit a structured `WARN` log naming the defaulted fields, the
   `source_id`, and the `processing_tier` so the silent-drop is no longer
   silent and on-call has a single grep target.
4. Return `success=True` with a well-formed `result` dict so the artifact
   is persisted by downstream NATS handlers.

## Acceptance Criteria
- A short-text input (e.g. `content="hi"`, `content_type="generic"`,
  `processing_tier="light"`) combined with an LLM payload missing both
  required fields returns `success=True` with `result.title == "hi"`
  and `result.artifact_type == "note"`.
- An empty-string input combined with the same partial payload returns
  `success=True` with `result.title == "Untitled"` and
  `result.artifact_type == "note"`.
- A payload missing only `artifact_type` (but supplying a real `title`)
  preserves the LLM-supplied `title` verbatim and derives
  `artifact_type` from the caller's `content_type` argument.
- A payload missing only `title` (but supplying a real `artifact_type`)
  preserves the LLM-supplied `artifact_type` verbatim and derives
  `title` from the caller's content.
- All other previously-passing `ml/tests/test_processor.py` cases stay
  green (no regression in retry / invalid-JSON / connection-failure /
  degraded-fallback paths).

## Non-Goals
- Changing the upstream LLM prompt itself (the `light` / `metadata`
  tier rules already permit partial payloads — the bug is on the
  consumer side).
- Removing the unavailable-LLM degraded-fallback branch (kept as-is).
- Re-introducing a hard `ValueError` for any other "missing field"
  case — `summary`, `key_ideas`, `entities`, etc. continue to use
  `setdefault` as before.
- Touching spec 061 main `scopes.md` / `report.md` / `state.json` —
  bug close-out lives entirely under this bug folder.
