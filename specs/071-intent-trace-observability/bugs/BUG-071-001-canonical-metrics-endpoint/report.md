# Report: BUG-071-001 Canonical metrics endpoint

## Summary

The current candidate was reproduced on the repository-managed disposable stack. The only spec-071 refusal-join failure occurs before HTTP because the test requires a noncanonical metrics variable.

## Completion Statement

The packet is active. RED evidence is captured; implementation, specialist verification, and validate-owned certification remain unset.

## Test Evidence

### RED: Canonical metrics endpoint contract

**Executed:** YES (current session)
**Command:** `cd ~/smackerel-assistant-environment-residuals-20260719 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '<seven-test residual selector>'`
**Exit Code:** 1
**Claim Source:** executed

```text
go-e2e: applying -run selector: <seven-test residual selector>
=== RUN   TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics
    intent_refusal_join_e2e_test.go:77: e2e: partial test env -
    SMACKEREL_TEST_ENV_FILE="/workspace/config/generated/test.env"
    SMACKEREL_CORE_METRICS_URL=""
    (must be both set or both unset)
--- FAIL: TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      1.282s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
```

## Invocation Audit

No subagent invocation tool is available in this runtime. No specialist phase or certification is claimed.

### GREEN: Live canonical metrics endpoint

Concrete test file: `tests/e2e/assistant/intent_refusal_join_e2e_test.go`

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<exact six-test selector>'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics
--- PASS: TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics (0.01s)
=== RUN   TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects
--- PASS: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects (9.88s)
=== RUN   TestIntentReplayE2E_UnknownTraceIDExits2
--- PASS: TestIntentReplayE2E_UnknownTraceIDExits2 (4.68s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      43.319s
PASS: go-e2e
Network smackerel-test_default Removed
```
