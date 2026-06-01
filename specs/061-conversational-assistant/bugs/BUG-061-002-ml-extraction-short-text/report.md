# Report: BUG-061-002 ML extraction graceful-degrade

## Summary
**Bug:** `ml/app/processor.py:178` raised `ValueError("Missing required field: artifact_type|title")` whenever the upstream LLM returned a partial JSON payload (the common case for short / low-signal inputs and for `processing_tier="light"`). The outer `except Exception` swallowed the precise message into an opaque `{"success": False, "error": "LLM processing failed"}` and bypassed the unavailable-LLM fallback (which keys on network error strings), silently dropping the entire capture.

**Severity:** High — silent ingestion drop with no actionable signal.

**Root cause:** Hard `required_fields = ["artifact_type", "title"]` check incompatible with the prompt's own `light`/`metadata` tier rules and with LLM behaviour on short inputs.

**Fix:** Replace the hard check with a setdefault/derive block that mirrors the existing unavailable-LLM fallback derivation rules (title ← `content[:100]` or `"Untitled"`; artifact_type ← `content_type` or `"note"`) and emits a structured WARN log naming the defaulted fields, the `source_id`, and the `processing_tier`.

### Completion Statement
All four regression / adversarial-regression tests for BUG-061-002 PASS post-fix and FAILED pre-fix. The full `ml/` unit suite (`./smackerel.sh test unit --python`, 464 tests) and the full Go unit suite (`./smackerel.sh test unit --go`) both pass with no regressions.

## Changes
| File | Change |
|------|--------|
| `ml/app/processor.py` | Replaced `required_fields` loop with setdefault/derive block + WARN log (+26/-5). |
| `ml/tests/test_processor.py` | Rewrote 2 existing tests to assert the new contract; added 2 new adversarial regression tests. |

## Tests Added
| Type | Test | Purpose |
|------|------|---------|
| Unit (regression) | `test_missing_artifact_type_degrades_to_default` | rewritten — asserts derive-from-content_type contract |
| Unit (regression) | `test_missing_title_degrades_to_default` | rewritten — asserts derive-from-content contract |
| Unit (adversarial regression) | `test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop` | canonical short-text repro shape |
| Unit (adversarial regression) | `test_bug_061_002_empty_content_derives_untitled` | empty-content `"Untitled"` fallback |

## Bug Reproduction — Before Fix
Captured via in-process repro with `app.processor.litellm.acompletion` patched to return a partial LLM payload (no `artifact_type`, no `title`), exactly mirroring what `gemma3:4b` and similar small models emit for short captures:

```
$ cd ml && python3 -c "...patch litellm.acompletion to return partial payload; call process_content(content='hi', content_type='generic', processing_tier='light', ...)"
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

**Observed contract violation:** every short / low-signal capture returns `success=False` with no `"result"` key. The downstream NATS handler treats this as a processing failure and the artifact is silently dropped from intelligence/extraction pipelines.

## Pre-Fix Regression Test FAILURE Proof
Stashed only `ml/app/processor.py` to isolate the test changes, then re-ran the four targeted tests:

```
$ git stash push -- ml/app/processor.py
Saved working directory and index state WIP on main: d4111aa5 spec(061): Round 59 — BS-002+BS-007 RED; Option 2 SST override incomplete (4 of 5 model env consumers still leak gemma3:4b); routed to plan
$ cd ml && python3 -m pytest tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop \
    tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_empty_content_derives_untitled \
    tests/test_processor.py::TestProcessContentErrors::test_missing_artifact_type_degrades_to_default \
    tests/test_processor.py::TestProcessContentErrors::test_missing_title_degrades_to_default -v
...
>       assert result["success"] is True
E       assert False is True

tests/test_processor.py:333: AssertionError
------------------------------ Captured log call -------------------------------
ERROR    smackerel-ml.processor:processor.py:206 LLM processing failed
Traceback (most recent call last):
  File "~/smackerel/ml/app/processor.py", line 178, in process_content
    raise ValueError(f"Missing required field: {field}")
ValueError: Missing required field: title
=========================== short test summary info ============================
FAILED tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop
FAILED tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_empty_content_derives_untitled
FAILED tests/test_processor.py::TestProcessContentErrors::test_missing_artifact_type_degrades_to_default
FAILED tests/test_processor.py::TestProcessContentErrors::test_missing_title_degrades_to_default
============================== 4 failed in 0.42s ===============================
$ git stash pop
```

All four pre-fix runs surface the exact stack trace from `processor.py:178`, proving the adversarial regression tests would catch a reintroduction of the bug.

## Post-Fix Regression Test SUCCESS Proof
```
$ cd ml && python3 -m pytest tests/test_processor.py -v
...
tests/test_processor.py::TestProcessContentErrors::test_missing_artifact_type_degrades_to_default PASSED [ 40%]
tests/test_processor.py::TestProcessContentErrors::test_missing_title_degrades_to_default PASSED [ 45%]
tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop PASSED [ 50%]
tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_empty_content_derives_untitled PASSED [ 54%]
...
============================== 22 passed in 0.16s ==============================
```

## Test Evidence — Full Suites

### Python Unit Suite (smackerel-ml)
```
$ ./smackerel.sh test unit --python
...
[py-unit] pip install OK; starting pytest ml/tests
........................................................................ [ 15%]
........................................................................ [ 31%]
........................................................................ [ 46%]
........................................................................ [ 62%]
........................................................................ [ 77%]
........................................................................ [ 93%]
................................                                         [100%]
464 passed, 1 warning in 14.01s
[py-unit] pytest ml/tests finished OK
PY_EXIT=0
```

### Go Unit Suite
```
$ ./smackerel.sh test unit --go
...
ok      github.com/smackerel/smackerel/internal/telegram        27.986s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      (cached)
ok      github.com/smackerel/smackerel/internal/telegram/render (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/eval/assistant     (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
ok      github.com/smackerel/smackerel/tests/observability      (cached)
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
[go-unit] go test ./... finished OK
GO_EXIT=0
```

### Silent-pass / Bailout Pattern Audit
```
$ grep -nE 'if .*: *return' ml/tests/test_processor.py | grep -i 'bug_061_002\|degrade' ; echo EXIT=$?
EXIT=1
```
Zero hits — no conditional-return early-exits in any new test; assertions are direct.

### Code Diff Evidence

Verbatim `git show` of the fix commit (PII-redacted; trimmed to the
processor.py hunk and the test-module summary):

```
$ git log --oneline -3 ml/app/processor.py
e7ca6c5b fix(ml): BUG-061-002 — graceful-degrade processor for partial LLM payloads
d1491aa1 chore(ml): clear pre-existing lint/format/test debt
e0b8ae89 fix(ml): bump LLM timeout 180→600 for gemma4:26b 4K-token digest gen

$ git show e7ca6c5b --stat | head
commit e7ca6c5bb79ae76e397382a06479af9e6679a34d
    fix(ml): BUG-061-002 — graceful-degrade processor for partial LLM payloads
 ml/app/processor.py        | 31 ++++++++++++++++++++++++++-----
 ml/tests/test_processor.py | ...
 2 files changed, ...

$ git show e7ca6c5b -- ml/app/processor.py
diff --git a/ml/app/processor.py b/ml/app/processor.py
@@ -171,7 +171,26 @@
-        # Validate required fields
-        required_fields = ["artifact_type", "title"]
-        for field in required_fields:
-            if field not in result:
-                raise ValueError(f"Missing required field: {field}")
+        # BUG-061-002: short / low-signal inputs (single tokens, emoji,
+        # URL-only captures) and the prompt's own "light" / "metadata"
+        # tier rules legitimately produce LLM payloads that omit
+        # `artifact_type` and/or `title`. Previously this raised a
+        # ValueError that the outer except-clause swallowed into an
+        # opaque "LLM processing failed" — silently dropping the
+        # capture. Degrade gracefully instead, mirroring the existing
+        # unavailable-LLM fallback shape, and log which fields were
+        # defaulted so the silent-drop is no longer silent.
+        defaulted_fields: list[str] = []
+        if "title" not in result or not str(result.get("title") or "").strip():
+            result["title"] = content[:100].strip() or "Untitled"
+            defaulted_fields.append("title")
+        if "artifact_type" not in result or not str(result.get("artifact_type") or "").strip():
+            result["artifact_type"] = content_type if content_type and content_type != "generic" else "note"
+            defaulted_fields.append("artifact_type")
+        if defaulted_fields:
+            logger.warning(
+                "LLM result missing required fields %s for source_id=%s tier=%s; "
+                "derived defaults from content/content_type (BUG-061-002)",
+                defaulted_fields,
+                source_id,
+                processing_tier,
+            )
```

The fix is contained entirely within `ml/app/processor.py` (function
`process_content`, +26/-5 lines). The companion test changes in
`ml/tests/test_processor.py` add the four BUG-061-002 regression
cases listed in "Tests Added" above.

## Claim Source
- Pre-fix repro stack trace: **executed** (in-process Python repro, PII-redacted)
- Pre-fix four-test failure: **executed** (`git stash` isolated processor.py; pytest re-ran the four targeted tests; output above is verbatim from terminal, PII-redacted)
- Post-fix processor.py test pass: **executed** (`pytest -v` against post-fix tree; output verbatim, PII-redacted)
- Full Python + Go unit suites: **executed** (`./smackerel.sh test unit --{python,go}`; output verbatim, PII-redacted)
- All evidence captured this session; no interpretation or paraphrasing.
