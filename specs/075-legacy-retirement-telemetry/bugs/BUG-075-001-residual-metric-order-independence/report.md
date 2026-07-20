# Report: BUG-075-001 Residual metric order independence

## Summary

The privacy test failed before any residual observation, while the rolling-report control passed after a retired-command request in the same package run. This isolates the defect to test precondition ordering.

## Completion Statement

The packet is active. RED and control evidence are captured; implementation, specialist verification, and validate-owned certification remain unset.

## Test Evidence

### RED and package-order control

**Executed:** YES (current session)
**Command:** `cd ~/smackerel-assistant-environment-residuals-20260719 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '<seven-test residual selector>'`
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly
    legacy_privacy_e2e_test.go:76: /metrics is missing the HELP line for
    "smackerel_legacy_command_residual_total"; metric is not registered.
    A regression that removed the init() in
    internal/assistant/legacyretirement/telemetry.go would trip this.
--- FAIL: TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly (0.01s)
=== RUN   TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody
--- FAIL: TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody (0.02s)
=== RUN   TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice
--- FAIL: TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice (0.07s)
=== RUN   TestLegacyRetirementReport_E2E_RollingSevenDay
--- PASS: TestLegacyRetirementReport_E2E_RollingSevenDay (0.02s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      1.282s
```

The two notice tests send live turns before failing at renderer invocation. Their requests materialize the metric, explaining why the later report control passes.

**Claim Source:** interpreted

## Invocation Audit

No subagent invocation tool is available in this runtime. No specialist phase or certification is claimed.

### GREEN: Order-independent real sample

Concrete test file: `tests/e2e/assistant/legacy_privacy_e2e_test.go`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<exact six-test selector>'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
=== RUN   TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly
--- PASS: TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly (25.18s)
=== RUN   TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody
--- PASS: TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody (1.10s)
=== RUN   TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice
--- PASS: TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice (2.40s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      43.319s
PASS: go-e2e
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```
