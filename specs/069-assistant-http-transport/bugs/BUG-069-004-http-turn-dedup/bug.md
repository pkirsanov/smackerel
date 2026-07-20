# BUG-069-004 - HTTP assistant retries execute the same turn more than once

**Status:** Confirmed - reproduction and fix in progress
**Severity:** Critical - retried assistant actions can duplicate facade execution
**Spec:** 069-assistant-http-transport
**Discovered:** 2026-07-19 during spec-073 synthesis broad E2E closeout

## Summary

`POST /api/assistant/turn` validates and echoes `transport_message_id` but does
not deduplicate accepted requests. Two authenticated requests for the same
user, HTTP transport, message ID, and body both invoke the facade and return
different `trace.assistant_turn_id` values. This violates spec 069 Hard
Constraint 4 and spec 073 SCN-073-A03.

The original PWA retry test hid this defect by using `/reset`, whose repeated
execution is an idempotent state clear and whose response has no invocation
trace. Replacing the fixture RED-first with deterministic
`/weather in barcelona` exposed duplicate execution.

## Reproduction

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWARetryE2E_'
```

Observed for one identical message ID:

```text
first  assistant_turn_id="trace_20260719T164001.274269423_6"
second assistant_turn_id="trace_20260719T164009.110344202_8"
```

The distinct-ID adversary passed with two different trace IDs, proving the
same-ID failure is not a fixed-trace or assertion artifact.

## Expected Behavior

- The idempotency key is scoped to authenticated identity, canonical HTTP
  transport, and `transport_message_id`.
- One accepted logical turn invokes the facade and capture path at most once.
- A retry receives the original logical response, including the same assistant
  turn ID, body, status, controls, and emitted timestamp.
- Per-HTTP-request metadata such as request ID remains current to the retry.
- A different user may reuse the same opaque message ID without receiving or
  affecting another user's response.
- Concurrent retries collapse onto one in-flight execution.
- The cache is bounded and expires entries using explicit SST-derived timing;
  it must not retain auth tokens or raw identity values.

## Actual Behavior

Every accepted request calls `a.facade.Handle`, regardless of whether the same
authenticated identity and transport message ID were already processed.

## Impact

A client retry after timeout can duplicate model/tool execution, conversation
turns, captures, confirmation side effects, cost, and audit entries. Cross-user
privacy would also be at risk if a future cache keyed only on message ID.
