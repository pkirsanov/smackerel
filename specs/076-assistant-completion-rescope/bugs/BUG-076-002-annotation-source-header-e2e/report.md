# Report: BUG-076-002 Annotation source header E2E

## Summary

The full assistant package exposed a missing test prerequisite: the annotation request omitted the source header required by the live API.

## Completion Statement

RED captured. Packet remains `in_progress`; specialist validation and certification are not claimed.

## Test Evidence

### RED

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<early assistant group>'`
**Exit Code:** 1
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestAnnotationClassifierWithShadowComparator
    annotation_classifier_e2e_test.go:82: POST annotation status = 400,
    body={"error":"X-Smackerel-Source header required"}
--- FAIL: TestAnnotationClassifierWithShadowComparator (39.58s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      42.546s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```

## Invocation Audit

No subagent invocation tool is available; no specialist completion is claimed.

### GREEN: Explicit API source

Concrete test file: `tests/e2e/assistant/annotation_classifier_e2e_test.go`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run 'TestAnnotationClassifierWithShadowComparator'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestAnnotationClassifierWithShadowComparator
--- PASS: TestAnnotationClassifierWithShadowComparator (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.212s
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```
