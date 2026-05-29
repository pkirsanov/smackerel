# Design: BUG-061-002 ML extraction graceful-degrade for missing required fields

## Root Cause
`ml/app/processor.py` (pre-fix line 175-178):

```python
required_fields = ["artifact_type", "title"]
for field in required_fields:
    if field not in result:
        raise ValueError(f"Missing required field: {field}")
```

This contract is violated by the prompt's own tier rules and by realistic
short-input behaviour:

- `processing_tier="light"` only requests `title, summary, topics, sentiment`
  — `artifact_type` is intentionally absent.
- `processing_tier="metadata"` only requests `title, artifact_type` — fine
  in principle, but `title` is still LLM-discretionary on minimal input.
- Short / low-signal inputs (`"hi"`, single emoji, URL-only captures)
  routinely cause every tier to omit one or both fields.

When the `ValueError` fires it is caught by the broad
`except Exception` block (pre-fix line 196), which:

1. Discards the precise error message and substitutes a generic
   `"LLM processing failed"`.
2. Bypasses the degraded-fallback branch because
   `_is_llm_unavailable_error` looks for network keywords (`connection`,
   `connect`, `refused`, `timeout`) and a `ValueError` matches none of
   them.

Net effect: every short-text capture becomes a silent ingestion drop
with no actionable diagnostic.

## Fix Design
Replace the hard `required_fields` check with a setdefault-style
graceful-degrade block, mirroring the existing unavailable-LLM
fallback shape, and emit a structured WARN log naming the defaulted
fields:

```python
defaulted_fields: list[str] = []
if "title" not in result or not str(result.get("title") or "").strip():
    result["title"] = content[:100].strip() or "Untitled"
    defaulted_fields.append("title")
if "artifact_type" not in result or not str(result.get("artifact_type") or "").strip():
    result["artifact_type"] = content_type if content_type and content_type != "generic" else "note"
    defaulted_fields.append("artifact_type")
if defaulted_fields:
    logger.warning(
        "LLM result missing required fields %s for source_id=%s tier=%s; "
        "derived defaults from content/content_type (BUG-061-002)",
        defaulted_fields,
        source_id,
        processing_tier,
    )
```

Rationale:

- Mirrors the existing `_processing_degraded_fallback` branch's
  derivation rules so the same artifact_type/title contract holds
  whether the LLM is partially-responsive or fully unavailable.
- Uses `setdefault`-equivalent semantics for these two fields, matching
  how every other optional field is already populated immediately below
  this block.
- The empty-string guard (`not str(result.get(field) or "").strip()`)
  also covers the (rarer) case where the LLM emits the key with a
  literal `null` or empty string — previously this passed the
  `field not in result` check but produced a useless artifact.
- The WARN log gives on-call a single grep target
  (`BUG-061-002`) to spot prompt-quality regressions.

## Affected Files
| File | Change |
|------|--------|
| `ml/app/processor.py` | Replace the `required_fields` loop with the setdefault/derive block above (lines 175-178 pre-fix; lines 175-200 post-fix). |
| `ml/tests/test_processor.py` | Re-classify the two pre-existing `test_missing_*_returns_error` tests as `test_missing_*_degrades_to_default` and add two new adversarial `test_bug_061_002_*` cases (short-text-partial-payload and empty-content-partial-payload). |

## Regression Test Design
- **`test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop`** — the canonical adversarial case. Input `content="hi"`, `content_type="generic"`, `processing_tier="light"`, LLM payload `{"summary": "...", "topics": [...], "sentiment": "..."}` (no `artifact_type`, no `title`). Asserts the EXACT shape that the broken code path could NEVER produce: `success=True`, `result.title == "hi"`, `result.artifact_type == "note"`, `result.summary` / `result.topics` preserved.
- **`test_bug_061_002_empty_content_derives_untitled`** — proves the `"Untitled"` fallback for the empty-content edge case.
- **`test_missing_artifact_type_degrades_to_default`** / **`test_missing_title_degrades_to_default`** — the original two tests, rewritten to assert the new contract (success + derived values) instead of the old hard-fail contract.
- Pre-fix verification: all four tests fail with `assert False is True` and `ValueError: Missing required field: ...` logged from `processor.py:178`. Captured in `report.md`.

## Non-Goals
- No NATS-handler or pipeline-level changes — the fix is entirely contained in `process_content`.
- No live-stack E2E required — the broken contract is a deterministic in-process unit-level invariant; the unit suite is the right gate. Spec 061 BS-002 live-stack work continues independently.
