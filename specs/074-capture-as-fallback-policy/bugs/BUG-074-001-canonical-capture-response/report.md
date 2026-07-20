# Report: BUG-074-001 Canonical capture response

## Summary

Two live assistant tests expose the same production defect: successful fallback capture retains upstream refusal metadata.

## Completion Statement

RED captured. Packet remains `in_progress`; specialist validation and certification are not claimed.

## Test Evidence

### RED

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<early assistant group>'`
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround
    capture_fallback_trigger_e2e_test.go:117: body = "I don't have a sourced answer for that.";
    expected canonical 'saved as an idea' acknowledgement
--- FAIL: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.75s)
=== RUN   TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape
    http_capture_test.go:126: error_cause = "provider_unavailable" on capture fallback;
    want empty (capture is a normal status, not an error)
--- FAIL: TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape (0.64s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      42.546s
FAIL: go-e2e (exit=1)
```

## Invocation Audit

No subagent invocation tool is available; no specialist completion is claimed.

### GREEN: Canonical capture response

Concrete test files: `internal/assistant/facade_open_knowledge_no_ground_test.go`, `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`, and `tests/e2e/assistant/http_capture_test.go`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround|CaptureAcknowledgementMatchesTelegramShape'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround
--- PASS: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.08s)
=== RUN   TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape
--- PASS: TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape (0.05s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.212s
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```
