# Scopes: BUG-061-002 ML extraction graceful-degrade for partial LLM payloads

## Scope 1: Replace hard `required_fields` check with setdefault/derive block

**Status:** [x] Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: ml/app/processor.process_content tolerates partial LLM payloads
  Scenario: Short text with partial payload no longer silently drops
    Given content="hi", content_type="generic", processing_tier="light"
    And the LLM returns {"summary": "...", "topics": [...], "sentiment": "..."}
    When process_content is invoked
    Then the result has success=True
    And result.result.title == "hi"
    And result.result.artifact_type == "note"
    And result.result.summary and topics are preserved verbatim

  Scenario: Empty content with partial payload derives "Untitled"
    Given content="", content_type="generic", processing_tier="light"
    And the LLM returns {"summary": "Empty input."}
    When process_content is invoked
    Then the result has success=True
    And result.result.title == "Untitled"
    And result.result.artifact_type == "note"

  Scenario: Missing artifact_type only — derive from content_type
    Given content_type="article"
    And the LLM returns {"title": "Missing artifact_type"}
    When process_content is invoked
    Then result.result.title == "Missing artifact_type" (preserved)
    And result.result.artifact_type == "article" (derived)

  Scenario: Missing title only — derive from content
    Given content="A meaningful capture about supper plans.", content_type="article"
    And the LLM returns {"artifact_type": "article"}
    When process_content is invoked
    Then result.result.artifact_type == "article" (preserved)
    And result.result.title == "A meaningful capture about supper plans." (derived)
```

### Implementation Plan
1. In `ml/app/processor.py`, replace the `required_fields` loop (pre-fix lines 175-178) with the setdefault/derive block specified in `design.md` → "Fix Design". Emit a structured WARN log naming the defaulted fields, the `source_id`, and the `processing_tier`.
2. In `ml/tests/test_processor.py`, rewrite `test_missing_required_field_returns_error` → `test_missing_artifact_type_degrades_to_default` and `test_missing_title_returns_error` → `test_missing_title_degrades_to_default` to assert the new contract.
3. Add two new adversarial tests: `test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop` and `test_bug_061_002_empty_content_derives_untitled`.

### Test Plan
| Type | Command | Purpose |
|------|---------|---------|
| Unit (Python, regression) | `python3 -m pytest ml/tests/test_processor.py -v` | proves the new contract on partial-payload inputs |
| Unit (Python, adversarial regression) | same | the two `test_bug_061_002_*` cases would fail if the bug were reintroduced |
| Unit (Python, full sidecar) | `./smackerel.sh test unit --python` | proves no other ml test regressed |
| Unit (Go) | `./smackerel.sh test unit --go` | sanity check; no Go code touched, so cached results are sufficient |

### Definition of Done — 3-Part Validation
- [x] Root cause confirmed and documented (design.md)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      Pre-fix processor.py:175-178:
          required_fields = ["artifact_type", "title"]
          for field in required_fields:
              if field not in result:
                  raise ValueError(f"Missing required field: {field}")

      Caught by `except Exception` at processor.py:196 which discards the
      precise message and returns {"success": False, "error": "LLM processing
      failed"}. _is_llm_unavailable_error matches only network keywords, so
      the unavailable-LLM fallback is bypassed.
      ```
- [x] Fix implemented (ml/app/processor.py setdefault/derive block + WARN log)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ git diff --stat ml/app/processor.py
       ml/app/processor.py | 31 ++++++++++++++++++++++++++-----
       1 file changed, 26 insertions(+), 5 deletions(-)
      ```
- [x] Pre-fix regression test FAILS
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ git stash push -- ml/app/processor.py
      Saved working directory and index state WIP on main: d4111aa5 ...
      $ python3 -m pytest tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop \
          tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_empty_content_derives_untitled \
          tests/test_processor.py::TestProcessContentErrors::test_missing_artifact_type_degrades_to_default \
          tests/test_processor.py::TestProcessContentErrors::test_missing_title_degrades_to_default -v
      ...
      >       assert result["success"] is True
      E       assert False is True
      tests/test_processor.py:333: AssertionError
      ------------------------------ Captured log call ------------------------------
      ERROR    smackerel-ml.processor:processor.py:206 LLM processing failed
      Traceback (most recent call last):
        File "~/smackerel/ml/app/processor.py", line 178, in process_content
          raise ValueError(f"Missing required field: {field}")
      ValueError: Missing required field: title
      ============================== 4 failed in 0.42s ===============================
      ```
- [x] Adversarial regression case exists and would fail if the bug returned
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      The four regression tests above are the adversarial set. The
      canonical adversarial property is asserted in
      test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop:

          assert result["success"] is True, (
              f"Pre-fix regression: short text caused silent drop. Got: {result!r}"
          )
          assert "result" in result, "Pre-fix regression: no 'result' key"
          assert result["result"]["title"] == "hi"
          assert result["result"]["artifact_type"] == "note"
          assert result["result"]["summary"] == "A brief greeting."
          assert result["result"]["topics"] == ["greeting"]

      The broken code path returned success=False with no "result" key, so
      every one of these assertions would re-fail if the validate-and-raise
      block were reintroduced. NOT tautological.
      ```
- [x] Post-fix regression test PASSES
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ python3 -m pytest tests/test_processor.py -v
      ...
      tests/test_processor.py::TestProcessContentErrors::test_missing_artifact_type_degrades_to_default PASSED [ 40%]
      tests/test_processor.py::TestProcessContentErrors::test_missing_title_degrades_to_default PASSED [ 45%]
      tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop PASSED [ 50%]
      tests/test_processor.py::TestProcessContentErrors::test_bug_061_002_empty_content_derives_untitled PASSED [ 54%]
      ...
      ============================== 22 passed in 0.16s ==============================
      ```
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nE 'if .*: *return' ml/tests/test_processor.py | grep -i 'bug_061_002\|degrade' ; echo EXIT=$?
      EXIT=1
      (zero hits — no conditional-return bailout in any new test.)
      ```
- [x] All existing tests pass (no regressions)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ ./smackerel.sh test unit --python
      ...
      464 passed, 1 warning in 14.01s
      [py-unit] pytest ml/tests finished OK
      PY_EXIT=0

      $ ./smackerel.sh test unit --go
      ...
      ok      github.com/smackerel/smackerel/internal/telegram        27.986s
      ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      (cached)
      ...
      [go-unit] go test ./... finished OK
      GO_EXIT=0
      ```
- [x] Bug marked as Fixed in bug.md

### Out-of-Scope (Deliberate)
- No live-stack E2E run — the broken contract is a deterministic in-process unit invariant; the unit suite is the correct gate. Spec 061 BS-002 live-stack work continues under spec 061 ownership.
