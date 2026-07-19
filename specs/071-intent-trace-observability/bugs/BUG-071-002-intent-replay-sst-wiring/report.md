# Report: BUG-071-002 Intent replay SST wiring

## Summary

Both replay scenarios were reproduced against the disposable stack. The configured capability is lost before runtime because config compilation and aggregate loading omit the intent-trace block.

## Completion Statement

The packet is active. RED evidence is captured; implementation, specialist verification, and validate-owned certification remain unset.

## Test Evidence

### RED: Replay capability is false

**Executed:** YES (current session)
**Command:** `cd ~/smackerel-assistant-environment-residuals-20260719 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '<seven-test residual selector>'`
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects
    intent_replay_test.go:176: replay-intent exit=1, want 0
        stdout:

        stderr:
        smackerel-core assistant replay-intent: assistant.intent_trace.replay_enabled is false
        exit status 5
--- FAIL: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects (0.64s)
=== RUN   TestIntentReplayE2E_UnknownTraceIDExits2
    intent_replay_test.go:213: missing trace exit=1, want 2
        stdout:

        stderr:
        smackerel-core assistant replay-intent: assistant.intent_trace.replay_enabled is false
        exit status 5
--- FAIL: TestIntentReplayE2E_UnknownTraceIDExits2 (0.48s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      1.282s
FAIL: go-e2e (exit=1)
```

## Invocation Audit

No subagent invocation tool is available in this runtime. No specialist phase or certification is claimed.

### GREEN: SST wiring and replay

Concrete test files: `internal/config/assistant_intent_trace_wiring_contract_test.go` and `tests/e2e/assistant/intent_replay_test.go`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<exact six-test selector>'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects
--- PASS: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects (9.88s)
=== RUN   TestIntentReplayE2E_UnknownTraceIDExits2
--- PASS: TestIntentReplayE2E_UnknownTraceIDExits2 (4.68s)
=== RUN   TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics
--- PASS: TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      43.319s
PASS: go-e2e
Network smackerel-test_default Removed
```
