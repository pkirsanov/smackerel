# Bug: BUG-007-003 — keep_bridge.serialize_note silently swallows gkeepapi exceptions

## Classification

- **Type:** Observability / silent-data-corruption bug
- **Severity:** HIGH (silent data corruption; no log, no metric, no alert)
- **Parent Spec:** 007 — Google Keep Connector
- **Workflow Mode:** bugfix-fastlane (ceiling: `done` — real Python code change)
- **Source:** Code-review finding H-3
- **Status:** Fixed

## Problem Statement

`ml/app/keep_bridge.py::serialize_note` contains 5 distinct `try: ... except Exception: pass` blocks (lines ~78–120) that silently swallow exceptions from `gkeepapi` attribute access:

1. `gnote.labels.all()` (labels list)
2. `gnote.collaborators.all()` (collaborators list)
3. `gnote.items` iteration (list_items)
4. `timestamps.updated` (modified_usec)
5. `timestamps.created` (created_usec)

When any of these raise (e.g., because `gkeepapi` changes attribute shape, deprecates a method, or the upstream API mutates), the note serializes with empty `labels`, empty `collaborators`, empty `list_items`, `modified_usec=0`, `created_usec=0`. The Go side ingests these as **legitimately empty** notes. There is no `logger.warning`, no metric increment, no alert. Schema drift in the unofficial Google Keep API would corrupt every synced note silently and indefinitely.

## Impact

- **Silent data corruption** of every Keep note after any upstream schema drift.
- **No observability surface** — operators cannot detect the failure mode in logs, metrics, or alerts.
- **Cursor poisoning** — if `timestamps.updated` raises for all notes, `modified_usec=0` for all entries, the cursor never advances and the connector ingests the same broken set forever.
- Violates Smackerel's fail-loud SST posture: exceptions on an external (and explicitly unofficial) API surface must be surfaced, not absorbed.

## Reproduction (Pre-fix, expected)

A unit test mocks a `gnote` whose attribute access raises (`labels.all()` raises, `collaborators.all()` raises, `items` access raises, `timestamps.updated` raises, `timestamps.created` raises) and asserts that `serialize_note(gnote)` emits a `WARNING`-level log line per failure via `caplog`.

Pre-fix expected outcome: test FAILS because no log lines are emitted.

```
$ cd ml && pytest tests/test_keep.py::test_serialize_note_logs_warning_on_attribute_failures -v
FAILED — assert 5 == 0 (no WARNING log records captured)
```

## Expected Fix

Keep the `pass` for resilience (do not change the empty-fallback values; do not raise), but add a single `logger.warning(...)` call per `except` block that surfaces the exception type and message. One log line per failure is sufficient to surface schema drift in monitoring.

Pattern:

```python
except Exception as exc:
    logger.warning("serialize_note: labels.all() failed: %s: %s", type(exc).__name__, exc)
```

Repeated per block with a distinct context tag (`labels`, `collaborators`, `list_items`, `timestamps.updated`, `timestamps.created`).

## Acceptance Criteria

- [ ] Each of the 5 `except Exception: pass` blocks in `serialize_note` emits a `logger.warning(...)` with the exception type and message before falling back to the empty default.
- [ ] Fallback values (`labels=[]`, `collaborators=[]`, `list_items=[]`, `modified_usec=0`, `created_usec=0`) are unchanged — resilience is preserved.
- [ ] No new exception is raised from `serialize_note` (no behavior change for callers).
- [ ] A regression test in `ml/tests/test_keep.py` mocks a `gnote` with failing attribute access and asserts via `caplog` that exactly one `WARNING` record per failed access is emitted.
- [ ] The regression test FAILS against the current code and PASSES after the fix.
- [ ] All existing Keep tests in `ml/tests/test_keep.py` continue to pass.
