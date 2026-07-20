# Bug: BUG-071-001 Canonical metrics endpoint missing from assistant E2E

## Summary

The assistant refusal/intent-trace join E2E requires a bespoke `SMACKEREL_CORE_METRICS_URL` that the canonical disposable-stack runner never supplies. After resolving the real canonical endpoint, the scrape also proves both joined CounterVec families need closed-vocabulary zero-series initialization to be visible before their first event.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Required assistant package E2E is red
- [ ] Low - Minor issue, cosmetic

## Status

- [ ] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start from commit `7ca186217c007a24075b2273275a22434d89fc44` in an isolated worktree.
2. Confirm no `smackerel-test` resources exist.
3. Run the focused live test through `./smackerel.sh test e2e --go-run`.
4. Observe a partial-environment failure before the metrics scrape executes.

## Expected Behavior

The E2E resolves the live metrics URL from the canonical core endpoint supplied by the repository runner, scrapes the real `/metrics` endpoint, and finds both required metric families even before their first event.

## Actual Behavior

`SMACKEREL_TEST_ENV_FILE` is set while `SMACKEREL_CORE_METRICS_URL` is empty, so the test initially fails before making a request. Once the canonical endpoint is used, empty label vectors are absent from exposition until a child series is materialized.

## Environment

- Service: Smackerel assistant E2E
- Version: `7ca186217c007a24075b2273275a22434d89fc44`
- Platform: Linux, repository-managed disposable Docker stack

## Error Output

```text
=== RUN   TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics
    intent_refusal_join_e2e_test.go:77: e2e: partial test env - SMACKEREL_TEST_ENV_FILE="/workspace/config/generated/test.env" SMACKEREL_CORE_METRICS_URL="" (must be both set or both unset)
--- FAIL: TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics (0.00s)
```

## Root Cause

Two coupled defects were masked by the first failure: the test invented a second endpoint variable instead of consuming `CORE_EXTERNAL_URL`, and the two joined metrics did not materialize any closed-vocabulary child series at zero. The fix uses the canonical endpoint and initializes valid zero counters without recording synthetic events.

## Related

- Feature: `specs/071-intent-trace-observability/`
- Companion packet: `BUG-071-002-intent-replay-sst-wiring`
- Prior broad-run packet: `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/`
