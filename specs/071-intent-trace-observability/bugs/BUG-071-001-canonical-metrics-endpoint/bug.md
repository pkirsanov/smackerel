# Bug: BUG-071-001 Canonical metrics endpoint missing from assistant E2E

## Summary

The assistant refusal/intent-trace join E2E requires a bespoke `SMACKEREL_CORE_METRICS_URL` that the canonical disposable-stack runner never supplies, even though `CORE_EXTERNAL_URL` already identifies the live core endpoint.

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

The E2E resolves the live metrics URL from the canonical core endpoint supplied by the repository runner, scrapes the real `/metrics` endpoint, and fails loudly if either required metric family is absent.

## Actual Behavior

`SMACKEREL_TEST_ENV_FILE` is set while `SMACKEREL_CORE_METRICS_URL` is empty, so the test fails before making a request.

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

The test invented a second metrics endpoint variable instead of consuming the canonical `CORE_EXTERNAL_URL` already injected by `smackerel.sh`. The runner and test therefore disagree on the live endpoint contract.

## Related

- Feature: `specs/071-intent-trace-observability/`
- Companion packet: `BUG-071-002-intent-replay-sst-wiring`
- Prior broad-run packet: `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/`
