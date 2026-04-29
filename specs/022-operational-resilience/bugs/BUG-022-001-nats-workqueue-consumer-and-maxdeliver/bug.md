# BUG-022-001: NATS Workqueue Consumer And MaxDeliver Regression

## Status

- **Current status:** Reopened / In Progress
- **Reopened at:** 2026-04-29T00:31:49Z
- **Owner feature:** `specs/022-operational-resilience`
- **Next owner:** `bubbles.implement`
- **Severity:** High
- **Workflow mode:** `bugfix-fastlane`
- **Duplicate policy:** This is the existing owner packet for the resurfaced regression. Do not create a duplicate bug packet.

## Summary

`./smackerel.sh test integration` is failing again on the shared NATS integration surface. The three failing tests exactly match the historical BUG-022-001 failure signatures:

- `TestNATS_PublishSubscribe_Artifacts`
- `TestNATS_PublishSubscribe_Domain`
- `TestNATS_Chaos_MaxDeliverExhaustion`

This blocks feature 039 validation because the shared integration command exits non-zero before downstream validation can claim a green live-stack baseline.

## Reproduction

Command:

```bash
./smackerel.sh test integration
```

Observed current failure evidence from the 2026-04-29 reopen run:

```text
=== RUN   TestNATS_PublishSubscribe_Artifacts
    nats_stream_test.go:92: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Artifacts (0.01s)
=== RUN   TestNATS_PublishSubscribe_Domain
    nats_stream_test.go:164: create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain (0.01s)
=== RUN   TestNATS_Chaos_MaxDeliverExhaustion
    nats_stream_test.go:369: expected 0 messages after MaxDeliver exhaustion, got 1 - dead-message path broken
    nats_stream_test.go:371: MaxDeliver=3 exhausted after 3 Naks, no further redelivery confirmed
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion (2.03s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration        18.426s
FAIL
Command exited with code 1
```

**Claim Source:** executed.

## Expected Behavior

- `TestNATS_PublishSubscribe_Artifacts` creates its consumer without `err_code=10100` and round-trips exactly one artifact message.
- `TestNATS_PublishSubscribe_Domain` creates its consumer without `err_code=10100` and round-trips exactly one domain extraction message.
- `TestNATS_Chaos_MaxDeliverExhaustion` observes exactly zero messages after `MaxDeliver=3` exhaustion.
- Full `./smackerel.sh test integration` exits 0.

## Actual Behavior

- ARTIFACTS and DOMAIN consumer creation currently collide with workqueue uniqueness constraints.
- The MaxDeliver chaos test currently receives one extra message after the expected terminal delivery boundary.
- Full `./smackerel.sh test integration` exits 1.

## Current Evidence And Ownership Notes

- The packet already existed at `specs/022-operational-resilience/bugs/BUG-022-001-nats-workqueue-consumer-and-maxdeliver` and was not duplicated.
- The packet was missing required `bug.md` and `scenario-manifest.json` artifacts; both were repaired during this reopen.
- Historical 2026-04-26 closure evidence is now superseded by the 2026-04-29 failing integration run.
- Current source inspection shows `tests/integration/nats_stream_test.go` has reverted or drifted back to exact workqueue filters for ARTIFACTS and DOMAIN plus wildcard `deadletter.>` MaxDeliver isolation. That source file was inspected but not modified by `bubbles.bug`.

## Suspected Root Cause

Historical BUG-022-001 analysis still appears applicable, but it must be re-confirmed by the implementation owner before changing code:

- Exact `FilterSubject` values on workqueue streams can collide with existing consumers and trigger `err_code=10100`.
- The MaxDeliver test can be contaminated by broad `deadletter.>` matching on a retained stream, causing stale or unrelated messages to be fetched after exhaustion.

## Impact

- Blocks feature 039 validation and any other workflow that requires `./smackerel.sh test integration` to pass.
- Leaves the parent operational-resilience certification state inconsistent unless reopened and routed.

## Next Required Owner

`bubbles.implement` should take this reopened packet next. Implementation can proceed after this artifact repair because `workflowMode: bugfix-fastlane` has `statusCeiling: done`, the packet now has the required bug-owned artifacts, and the current failure has executed evidence.
