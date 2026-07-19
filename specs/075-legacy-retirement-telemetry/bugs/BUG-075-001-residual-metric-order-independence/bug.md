# Bug: BUG-075-001 Residual metric E2E depends on package order

## Summary

The residual telemetry privacy E2E scrapes an empty `CounterVec` before causing any real retired-command observation, so Prometheus emits no metric family and the test fails when it runs before another test materializes a sample.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Required live telemetry regression is order-dependent
- [ ] Low - Minor issue, cosmetic

## Status

- [ ] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start a clean disposable E2E stack.
2. Run the residual privacy E2E before any retired-command turn.
3. Observe the missing HELP line.
4. In the same package run, execute a real retired-command notice turn before the rolling report test.
5. Observe the rolling report test sees the metric and passes.

## Expected Behavior

The privacy E2E creates its own real residual observation through the live assistant route, then requires HELP, TYPE, a sample, the exact label set, and an HMAC-shaped bucket. It passes independently of package order.

## Actual Behavior

The test accepts zero samples but paradoxically requires the empty vector to appear in exposition. Another test's request can make it pass, creating order dependence.

## Environment

- Service: legacy retirement telemetry
- Version: `7ca186217c007a24075b2273275a22434d89fc44`
- Platform: Linux, repository-managed disposable Docker stack

## Error Output

```text
=== RUN   TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly
    legacy_privacy_e2e_test.go:76: /metrics is missing the HELP line for "smackerel_legacy_command_residual_total"; metric is not registered.
--- FAIL: TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly (0.01s)
```

## Root Cause

Prometheus `CounterVec` descriptors are registered at process initialization, but a vector with no label child produces no exposition family. The E2E never invokes the real recording path before scraping.

## Related

- Feature: `specs/075-legacy-retirement-telemetry/`
- Companion packet: `BUG-075-002-assistant-renderer-node-toolchain`
