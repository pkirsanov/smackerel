# Design: BUG-007-003 — keep_bridge silent exception swallow

> **Bug:** [bug.md](bug.md) | **Spec:** [spec.md](spec.md)
> **Parent:** [007 spec](../../spec.md) | [007 scopes](../../scopes.md)
> **Status:** Initial — root cause confirmed; implementation pending dispatch to `bubbles.implement`

---

## Root Cause

`serialize_note` was written defensively against the unofficial `gkeepapi` library, which can raise on any attribute access if the upstream Google Keep API mutates. The author wrapped each volatile attribute access in `try: ... except Exception: pass` to keep the sync resilient. The intent (resilience) is correct, but the implementation chose `pass` instead of `pass + log`, leaving zero observability for the only signal that would surface schema drift.

The 5 blocks in `serialize_note` (per `ml/app/keep_bridge.py` lines ~85–122):

| # | Lines | Attribute | Fallback |
|---|---|---|---|
| 1 | ~86–90  | `gnote.labels.all()` | `labels = []` |
| 2 | ~92–96  | `gnote.collaborators.all()` | `collaborators = []` |
| 3 | ~98–109 | `gnote.items` iteration | `list_items = []` |
| 4 | ~111–119 | `timestamps.updated` | `modified_usec = 0` |
| 5 | ~111–119 | `timestamps.created` (same `try` block as #4) | `created_usec = 0` |

Note on #4/#5: the timestamps live in a single shared `try` block in the current code. The fix can either (a) split them into two `try` blocks so each surface is independently observable, or (b) keep one block and log once with a combined context. Option (a) is preferred because it preserves the resilience semantics independently per-attribute (a failure on `updated` no longer hides a separate failure on `created`).

## Fix Approach

Minimal, additive change inside `serialize_note`:

1. For each of the 5 logical failure points, replace `except Exception: pass` with:
   ```python
   except Exception as exc:
       logger.warning(
           "serialize_note: <context> access failed: %s: %s",
           type(exc).__name__, exc,
       )
   ```
   Where `<context>` is one of: `labels`, `collaborators`, `list_items`, `timestamps.updated`, `timestamps.created`.

2. Split the current shared `timestamps.updated` / `timestamps.created` try block into two independent blocks so each failure is observable in isolation.

3. Preserve all existing fallback values and the no-raise contract. Do NOT change the returned dict shape.

### Affected Files

| File | Change |
|---|---|
| `ml/app/keep_bridge.py` | 5 `except Exception: pass` → 5 `except Exception as exc: logger.warning(...)`; split timestamps try block into two |
| `ml/tests/test_keep.py` | New regression test `test_serialize_note_logs_warning_on_attribute_failures` (and supporting fixtures) |

## Regression Test Design

**File:** `ml/tests/test_keep.py`
**Test:** `test_serialize_note_logs_warning_on_attribute_failures`

**Mechanism:** pytest `caplog` fixture captures log records at `WARNING` level for the `smackerel-ml.keep-bridge` logger.

**Fixture:** A `MagicMock`-based fake `gnote` where:
- `gnote.labels.all` raises `AttributeError("labels.all gone")`
- `gnote.collaborators.all` raises `RuntimeError("collaborators API changed")`
- accessing `gnote.items` raises `TypeError("items not iterable")` (via a `PropertyMock` or by setting `items` to an object whose `__iter__` raises)
- `gnote.timestamps.updated` raises `AttributeError("updated removed")`
- `gnote.timestamps.created` raises `AttributeError("created removed")`
- safe attributes return benign defaults: `id=""`, `title=""`, `text=""`, `pinned=False`, `archived=False`, `trashed=False`, `color=None`

**Assertions:**

```python
with caplog.at_level(logging.WARNING, logger="smackerel-ml.keep-bridge"):
    result = serialize_note(fake_gnote)

warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
assert len(warnings) == 5, f"expected 5 warning records, got {len(warnings)}: {[r.message for r in warnings]}"

contexts = {r.getMessage() for r in warnings}
assert any("labels" in m for m in contexts)
assert any("collaborators" in m for m in contexts)
assert any("list_items" in m for m in contexts)
assert any("timestamps.updated" in m for m in contexts)
assert any("timestamps.created" in m for m in contexts)

# Resilience preserved
assert result["labels"] == []
assert result["collaborators"] == []
assert result["list_items"] == []
assert result["modified_usec"] == 0
assert result["created_usec"] == 0
```

### Adversarial Case

The test uses inputs where **every** swallowed exception path triggers. With the current `pass`-only code, zero warnings are captured → assertion `len(warnings) == 5` fails. If the fix is reverted (any single `logger.warning` removed), the count drops below 5 and the test fails. This satisfies the bubbles adversarial regression requirement: the test cannot pass against any subset of the bug.

### Pre-fix Expectation

Running the new test against current `keep_bridge.py` MUST fail with `len(warnings) == 0` (or some count < 5 if any access happens to log elsewhere). This proves the bug exists before the fix lands.

## Why this is the right fix

- Preserves resilience (no behavior change for callers; sync does not abort on attribute drift).
- Surfaces schema drift in logs — the minimum observability bar.
- Smallest possible code change consistent with the "keep `pass` for resilience" constraint in the bug report.
- No new dependencies, no new metric infrastructure, no behavior change for the Go ingestion side.
