# BUG-CHAOS-20260524-001: ntfy Dead-Letter Replay Burst Re-submits Through Source Sink

## Status

Fixed - chaos verified and G048 audit rework repaired; standalone promotion remains validation-owned.

## Severity

P2 - Medium. The replay audit row is idempotent, but the source-sink side effect is not. Repeated replay of one dead-letter can create duplicate raw and normalized notification records.

## Source

- Agent: bubbles.chaos
- Discovered at: 2026-05-24T18:53:23Z
- Seed: 20260524
- Feature: specs/055-notification-source-ntfy-adapter

## Symptom

A seeded ntfy chaos integration probe replayed the same replay-eligible dead-letter three times. The adapter/store coalesced the replay audit table to one `notification_ntfy_replay_attempts` row, but each replay still called `SourceEventSink.SubmitSourceEvent` and created a new raw event. The observed summary was `raw=8 dead_letters=5 replay_attempt_rows=1`, with three distinct replay raw event IDs.

## Reproduction

1. Create a replay-eligible ntfy dead-letter for an enabled webhook source.
2. Call `Store.ReplayDeadLetter` three times with the same `dead_letter_id` and actor reference.
3. Count `notification_ntfy_replay_attempts` for the dead-letter and `notification_raw_events` for the replayed source event.
4. Observe one replay-attempt row but multiple raw events created through the source sink.

Original chaos command that exposed the side effect:

```bash
TERM=dumb NO_COLOR=1 ./smackerel.sh test integration --go-run 'TestNtfyChaosResilienceSeed20260524'
```

The temporary chaos test file was removed after evidence capture; implementation must add a permanent regression test before fixing.

## Expected Behavior

Repeated replay for the same dead-letter must be idempotent before source-sink side effects. After an accepted replay, subsequent replay attempts for the same dead-letter should either return the existing accepted attempt or fail with an explicit already-replayed status, without creating another raw event or normalized notification.

## Actual Behavior

The idempotency key is applied when recording the replay attempt, after `ReplayDeadLetter` has already parsed, mapped, and submitted the event to `SourceEventSink`. The audit row is bounded, but the source-sink side effect is repeated.

## Impact

Repeated operator replay, browser double-submit, retry-after-timeout, or concurrent replay can duplicate notification evidence and incident correlation input. The duplicate events remain source-qualified, but the replay control is not side-effect idempotent.

## Audit Rework: RW-BUG-CHAOS-20260524-001-AUDIT-001

The audit follow-up found that `scanSubscriptionStates` and `scanDeadLetters` silently ignored malformed persisted `redaction_state` decode errors. The repair now propagates contextual decode errors and adds deterministic scanner regression coverage for both subscription-state and dead-letter rows.
