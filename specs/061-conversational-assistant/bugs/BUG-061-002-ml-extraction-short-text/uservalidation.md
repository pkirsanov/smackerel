# User Validation: BUG-061-002 ML extraction graceful-degrade

## Checklist

### [Bug Fix] BUG-061-002 Short / minimal text no longer silently dropped by ml/app/processor.py
- [x] **What:** `process_content()` returns `success=True` with derived `title` / `artifact_type` when the LLM emits a partial JSON payload (the common case for short captures and `processing_tier="light"`), instead of silently dropping the artifact via a swallowed `ValueError`.
  - **Steps:**
    1. `cd ml && python3 -m pytest tests/test_processor.py -v`
    2. Confirm the four BUG-061-002 cases (`test_missing_artifact_type_degrades_to_default`, `test_missing_title_degrades_to_default`, `test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop`, `test_bug_061_002_empty_content_derives_untitled`) all PASS.
    3. `./smackerel.sh test unit --python` → `464 passed`.
    4. `./smackerel.sh test unit --go` → all packages OK.
  - **Expected:** All four targeted tests PASS; full Python + Go unit suites stay green; no regressions.
  - **Verify:** terminal exit code 0 + `22 passed` / `464 passed` / `[go-unit] go test ./... finished OK`.
  - **Evidence:** report.md → "Post-Fix Regression Test SUCCESS Proof" and "Test Evidence — Full Suites"
  - **Notes:** Bug fix for BUG-061-002; touches `ml/app/processor.py` and `ml/tests/test_processor.py` only. Spec 061 main artifacts untouched.
