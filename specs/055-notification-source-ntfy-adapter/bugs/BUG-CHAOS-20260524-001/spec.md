# Spec: Idempotent ntfy Dead-Letter Replay Burst

## Expected Behavior

ntfy dead-letter replay must be idempotent across repeated requests for the same dead-letter record. The first accepted replay may submit the reconstructed `SourceEventEnvelope` through `SourceEventSink`. Later replay attempts for the same dead-letter must not create additional raw events, normalized notifications, incidents, decisions, approvals, or delivery attempts from the same dead-letter.

Persisted ntfy `redaction_state` must also fail loud during store reconstruction. If a scanner receives malformed or non-object redaction state bytes for subscription state or dead-letter rows, it must return a contextual decode error instead of returning a record with an empty fabricated redaction map.

## Acceptance Criteria

- A replay-eligible dead-letter replayed once records one accepted replay attempt and one source-sink raw event.
- Replaying the same dead-letter again does not call `SourceEventSink.SubmitSourceEvent` a second time.
- Concurrent replay attempts for the same dead-letter serialize or converge to one source-sink side effect.
- Replay attempt audit remains available and redacted.
- The replay API response makes already-replayed state explicit without exposing raw payload bytes.
- Replays still go only through `SourceEventSink`; no output channel is called directly.
- Malformed persisted `redaction_state` decode failures are propagated with source/topic or dead-letter/source context.
- Regression coverage proves both subscription-state and dead-letter scanners reject malformed `redaction_state` bytes.

## Gherkin Scenario

```gherkin
Scenario: Replaying one ntfy dead-letter repeatedly does not duplicate source events
  Given an ntfy dead-letter is replay eligible
  And the dead-letter has not yet been replayed
  When an operator or retry path submits replay for the same dead-letter three times
  Then at most one replay attempt reaches SourceEventSink
  And at most one raw notification event is created for that dead-letter replay
  And later attempts return an already-replayed or existing-attempt result
  And no direct output-channel delivery is created by the ntfy adapter

Scenario: Malformed ntfy redaction state is not silently discarded
  Given persisted ntfy subscription-state or dead-letter row data contains malformed redaction_state bytes
  When the ntfy store scan helper reconstructs the row
  Then the helper returns a contextual decode error
  And no empty redaction map is fabricated for operator-facing state
```

## Non-Goals

- Do not bypass `SourceEventSink` for the first replay.
- Do not remove replay audit records.
- Do not expose raw payload bytes in operator APIs.
- Do not hide persisted redaction-state corruption by substituting empty redaction maps.
