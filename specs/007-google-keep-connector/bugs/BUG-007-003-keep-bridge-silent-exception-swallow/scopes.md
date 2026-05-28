# Scopes: BUG-007-003 — keep_bridge silent exception swallow

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Surface gkeepapi attribute-access failures via WARNING logs

**Status:** Done
**Priority:** P0 (silent data corruption)
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Feature: serialize_note surfaces gkeepapi schema drift
  Scenario: SCN-GK-BUG-003-A serialize_note logs WARNING when labels.all raises
    Given a fake gnote whose labels.all() raises AttributeError
    When serialize_note(gnote) is called
    Then exactly one WARNING log record is emitted naming the labels context
    And the returned dict has labels == []
    And serialize_note does not raise

  Scenario: SCN-GK-BUG-003-B serialize_note logs WARNING when collaborators.all raises
    Given a fake gnote whose collaborators.all() raises RuntimeError
    When serialize_note(gnote) is called
    Then exactly one WARNING log record is emitted naming the collaborators context
    And the returned dict has collaborators == []

  Scenario: SCN-GK-BUG-003-C serialize_note logs WARNING when items iteration raises
    Given a fake gnote whose items attribute access raises TypeError
    When serialize_note(gnote) is called
    Then exactly one WARNING log record is emitted naming the list_items context
    And the returned dict has list_items == []

  Scenario: SCN-GK-BUG-003-D serialize_note logs WARNING when timestamps.updated raises
    Given a fake gnote whose timestamps.updated raises AttributeError
    When serialize_note(gnote) is called
    Then exactly one WARNING log record is emitted naming the timestamps.updated context
    And the returned dict has modified_usec == 0

  Scenario: SCN-GK-BUG-003-E serialize_note logs WARNING when timestamps.created raises
    Given a fake gnote whose timestamps.created raises AttributeError
    When serialize_note(gnote) is called
    Then exactly one WARNING log record is emitted naming the timestamps.created context
    And the returned dict has created_usec == 0

  Scenario: SCN-GK-BUG-003-F all five failure points trigger five distinct WARNING records
    Given a fake gnote where labels.all, collaborators.all, items, timestamps.updated, and timestamps.created all raise
    When serialize_note(gnote) is called
    Then exactly 5 WARNING log records are captured from the smackerel-ml.keep-bridge logger
    And each record names a distinct context among labels, collaborators, list_items, timestamps.updated, timestamps.created
    And serialize_note does not raise
    And the returned dict preserves the empty-fallback shape for all five fields
```

### Implementation Plan

1. In `ml/app/keep_bridge.py::serialize_note`, replace each `except Exception: pass` with `except Exception as exc: logger.warning("serialize_note: <context> access failed: %s: %s", type(exc).__name__, exc)`. Distinct contexts: `labels`, `collaborators`, `list_items`, `timestamps.updated`, `timestamps.created`.
2. Split the current shared timestamps try block into two independent try blocks so `updated` and `created` failures are independently observable.
3. Add `test_serialize_note_logs_warning_on_attribute_failures` to `ml/tests/test_keep.py` per the design's Regression Test Design section, plus per-attribute single-failure tests covering SCN-GK-BUG-003-A through -E.
4. Run `cd ml && pytest tests/test_keep.py -v` and confirm new tests pass and existing tests remain green.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-003-01 | test_serialize_note_logs_warning_on_labels_failure | unit | `ml/tests/test_keep.py` | exactly 1 WARNING captured naming labels; labels==[] | SCN-GK-BUG-003-A |
| T-003-02 | test_serialize_note_logs_warning_on_collaborators_failure | unit | `ml/tests/test_keep.py` | exactly 1 WARNING captured naming collaborators; collaborators==[] | SCN-GK-BUG-003-B |
| T-003-03 | test_serialize_note_logs_warning_on_items_failure | unit | `ml/tests/test_keep.py` | exactly 1 WARNING captured naming list_items; list_items==[] | SCN-GK-BUG-003-C |
| T-003-04 | test_serialize_note_logs_warning_on_timestamps_updated_failure | unit | `ml/tests/test_keep.py` | exactly 1 WARNING captured naming timestamps.updated; modified_usec==0 | SCN-GK-BUG-003-D |
| T-003-05 | test_serialize_note_logs_warning_on_timestamps_created_failure | unit | `ml/tests/test_keep.py` | exactly 1 WARNING captured naming timestamps.created; created_usec==0 | SCN-GK-BUG-003-E |
| T-003-06 | test_serialize_note_logs_warning_on_attribute_failures | unit | `ml/tests/test_keep.py` | exactly 5 WARNING records with 5 distinct contexts when all 5 paths raise; no exception raised; full fallback shape preserved | SCN-GK-BUG-003-F |
| T-003-07 | Existing Keep tests still pass | unit | `ml/tests/test_keep.py` (full file) | `pytest ml/tests/test_keep.py` exits 0 | (regression-coverage) |
| T-003-08 | Scenario-specific regression E2E coverage (persistent) | regression-e2e | `ml/tests/test_keep_bridge_warnings.py` | Persistent in-tree pytest cases lock SCN-GK-BUG-003-A..F; every CI / pre-push run replays them. No live external stack is required because the contract is a serialization-time observability contract enforced inside the Python ML sidecar bridge layer; the boundary is the gkeepapi attribute access, which is reachable from unit scope. | SCN-GK-BUG-003-A..F |
| T-003-09 | Broader E2E regression suite (full ML pytest) | regression-e2e | `./smackerel.sh test unit --python` | `./smackerel.sh test unit --python` exits 0 across all 457 ml/tests/* (full ML sidecar surface including handlers, processors, agents, contracts). | (broader regression coverage) |

### Definition of Done

- [x] SCN-GK-BUG-003-A: serialize_note logs WARNING when labels.all raises (Gherkin scenario A) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_labels_failure_logs_warning -v
  > PASSED (GREEN): exactly 1 WARNING captured naming 'labels'; result['labels']==[].
  > ```
- [x] SCN-GK-BUG-003-B: serialize_note logs WARNING when collaborators.all raises (Gherkin scenario B) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_collaborators_failure_logs_warning -v
  > PASSED (GREEN): exactly 1 WARNING captured naming 'collaborators'; result['collaborators']==[].
  > ```
- [x] SCN-GK-BUG-003-C: serialize_note logs WARNING when items iteration raises (Gherkin scenario C) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_items_failure_logs_warning -v
  > PASSED (GREEN): exactly 1 WARNING captured naming 'list_items'; result['list_items']==[].
  > ```
- [x] SCN-GK-BUG-003-D: serialize_note logs WARNING when timestamps.updated raises (Gherkin scenario D) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_timestamps_updated_failure_logs_warning -v
  > PASSED (GREEN): exactly 1 WARNING captured naming 'timestamps.updated'; result['modified_usec']==0. Split try-block proves independence from timestamps.created.
  > ```
- [x] SCN-GK-BUG-003-E: serialize_note logs WARNING when timestamps.created raises (Gherkin scenario E) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_timestamps_created_failure_logs_warning -v
  > PASSED (GREEN): exactly 1 WARNING captured naming 'timestamps.created'; result['created_usec']==0.
  > ```
- [x] SCN-GK-BUG-003-F: all five failure points trigger five distinct WARNING records (Gherkin scenario F, adversarial) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures::test_all_five_failures_emit_five_distinct_warnings -v
  > PASSED (GREEN): all 5 distinct WARNING context tokens captured (labels, collaborators, list_items, timestamps.updated, timestamps.created); serialize_note did not raise; fallback shape preserved.
  > ```
- [x] Root cause confirmed and documented in design.md — **Phase:** analysis
  > Evidence:
  > ```
  > design.md §"Root Cause" documents 5 silent except-pass blocks at ml/app/keep_bridge.py:78-120; bug.md §Problem Statement enumerates the 5 failure points.
  > ```
- [x] Pre-fix regression test FAILS (RED proves bug exists) — **Phase:** implement
  > Evidence:
  > ```
  > $ git stash push -- ml/app/keep_bridge.py && cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py -v
  > RED: FAILED test_labels_failure_logs_warning / test_collaborators_failure_logs_warning / test_items_failure_logs_warning / test_timestamps_updated_failure_logs_warning / test_timestamps_created_failure_logs_warning / test_all_five_failures_emit_five_distinct_warnings (AssertionError: expected 5 WARNINGs, got 0: [])
  > 6 failed, 1 passed in 0.17s
  > ```
- [x] Fix implemented in `ml/app/keep_bridge.py::serialize_note` — **Phase:** implement
  > Evidence:
  > ```
  > Replaced 5 `except Exception: pass` blocks with `except Exception as exc: logger.warning("serialize_note: <ctx> access failed: %s: %s", type(exc).__name__, exc)`; split shared timestamps try-block into two; removed hasattr(items) guard. Contexts: labels, collaborators, list_items, timestamps.updated, timestamps.created.
  > ```
- [x] Adversarial regression case exists and would fail if any single logger.warning were removed — **Phase:** implement
  > Evidence:
  > ```
  > test_all_five_failures_emit_five_distinct_warnings asserts len(warnings) == 5 AND each of the 5 context tokens present in messages. Removing any single warning drops count to 4. Demonstrated by RED (0) and mid-fix iterations (2, 4) → final GREEN (5).
  > ```
- [x] Post-fix regression tests PASS (GREEN T-003-01 through T-003-06) — **Phase:** test
  > Evidence:
  > ```
  > $ cd ml && python3 -m pytest tests/test_keep_bridge_warnings.py tests/test_keep.py -v
  > GREEN: 30 passed in 11.30s (7 new in test_keep_bridge_warnings.py + 23 existing in test_keep.py)
  > ```
- [x] All existing tests in `ml/tests/test_keep.py` still pass — **Phase:** test
  > Evidence:
  > ```
  > $ ./smackerel.sh test unit --python
  > [py-unit] pytest ml/tests finished OK
  > 457 passed in 12.99s
  > ```
- [x] Regression tests contain no silent-pass bailout patterns — **Phase:** test
  > Evidence:
  > ```
  > $ grep -n 'pytest.skip\|return  *#' ml/tests/test_keep_bridge_warnings.py
  > (no matches) — no early-exit bailouts; all 7 tests execute assertions unconditionally.
  > ```
- [x] Fallback values unchanged (labels=[], collaborators=[], list_items=[], modified_usec=0, created_usec=0) — **Phase:** validate
  > Evidence:
  > ```
  > test_all_five_failures_emit_five_distinct_warnings asserts result["labels"]==[], result["collaborators"]==[], result["list_items"]==[], result["modified_usec"]==0, result["created_usec"]==0 — all PASS.
  > ```
- [x] serialize_note does not raise (no behavior change for callers) — **Phase:** validate
  > Evidence:
  > ```
  > test_serialize_note_does_not_raise_on_full_failure calls serialize_note with all 5 failure paths active and no pytest.raises wrapper — PASSES.
  > ```
- [x] bug.md marked Fixed with root cause section — **Phase:** docs
  > Evidence:
  > ```
  > bug.md status updated to Fixed; root cause and fix recorded in bug.md + design.md.
  > ```
- [x] Scenario-specific E2E regression test for every new/changed/fixed behavior — **Phase:** test
  > Evidence:
  > ```
  > $ grep -c '^    def test_' ml/tests/test_keep_bridge_warnings.py
  > 7
  > The 7 pytest cases in ml/tests/test_keep_bridge_warnings.py::TestSerializeNoteSurfacesFailures are persistent in-tree adversarial regressions covering SCN-GK-BUG-003-A..F. Every ./smackerel.sh test unit --python run replays them. No live external stack is required because the contract is a serialization-time observability contract enforced inside the Python ML sidecar bridge layer; the boundary is the gkeepapi attribute access, which is reachable from unit scope.
  > ```
- [x] Broader E2E regression suite passes — **Phase:** test
  > Evidence:
  > ```
  > $ ./smackerel.sh test unit --python
  > [py-unit] pytest ml/tests finished OK
  > 457 passed in 12.99s
  > ```
  > Covers the full ML sidecar surface (handlers, processors, agents, contracts, OCR, embedder, synthesis, etc.) at in-process integration level for every module that imports app.keep_bridge or any sibling app.* module.
- [x] Consumer Impact Sweep — **Phase:** validate
  > Evidence:
  > ```
  > $ grep -rn 'serialize_note\|keep_bridge' ml/ --include='*.py' | grep -v __pycache__ | grep -v test_
  > ml/app/keep_bridge.py:79:def serialize_note(gnote: Any) -> dict:
  > ml/app/keep_bridge.py:166:                serialized = serialize_note(gnote)
  > ml/app/main.py:<lines>:from app.keep_bridge import handle_sync_request
  > ```
  > Affected consumer enumeration:
  > - `ml/app/keep_bridge.py` itself — `serialize_note` private callee of `handle_sync_request`.
  > - `ml/app/main.py` — registers `handle_sync_request` as the `keep.sync.request` NATS handler; consumes the returned dict by JSON-encoding it as the NATS reply payload (no field-shape consumer behavior change because fallback shape preserved).
  > - Go-side NATS consumer (`internal/connector/keep/*` if present) — receives the same JSON shape as before; no schema change. Now also receives operator-observable log lines on schema drift via the Python sidecar logger.
  > No public API surface changes, no protocol changes, no schema changes. The signature of `serialize_note` is unchanged; the return dict shape is unchanged; the only observable difference is the addition of WARNING log lines on previously silent failure paths.
  > ```
