# Design: Idempotent ntfy Dead-Letter Replay Burst Fix

## Suspected Root Cause

`internal/notification/source/ntfy/store.go::ReplayDeadLetter` reconstructs the dead-letter payload and calls `sink.SubmitSourceEvent` before the replay idempotency record is persisted through `recordReplayAttempt`. The `notification_ntfy_replay_attempts.idempotency_key` unique constraint coalesces the audit row, but it cannot prevent the already-executed source-sink side effect.

## Impacted Components

- `internal/notification/source/ntfy/store.go`
- `notification_ntfy_dead_letters`
- `notification_ntfy_replay_attempts`
- ntfy replay API path in `internal/api/notifications_ntfy.go`
- Integration and E2E replay tests for spec 055

## Fix Direction

Implement side-effect idempotency before calling `SourceEventSink`.

Candidate approach:

1. Begin a database transaction and lock the dead-letter row by ID and source instance.
2. If `replay_status = 'replayed'`, return a redacted already-replayed attempt/result without calling the sink.
3. If no accepted replay exists, create or claim the replay attempt idempotency key before the sink call.
4. Submit to `SourceEventSink` only for the winner of that claim.
5. Update dead-letter replay status and attempt metadata in the same transaction boundary where possible; when the sink call cannot be inside the transaction safely, persist an explicit in-progress/claimed state before side effect and finalize after the call.
6. Add a permanent regression test that fails against the current behavior by asserting repeated replay creates one raw event, not three.

## Audit Follow-Up: G048 Redaction-State Decode

### Root Cause

`scanSubscriptionStates` and `scanDeadLetters` decoded persisted `redaction_state` using `_ = json.Unmarshal(...)`. Any malformed JSON or incompatible state shape was discarded, and the scanner returned a record with an empty map. That hid persistence corruption from callers and could make operator state look safely redacted when the redaction metadata was actually unreadable.

### Fix Direction

1. Replace ignored unmarshal results with a shared decode helper.
2. Return contextual errors that identify the source/topic or dead-letter/source pair being reconstructed.
3. Reject nil/non-object redaction state rather than substituting an empty map.
4. Add deterministic unit regression coverage at the scanner seam for subscription states and dead letters.
5. Preserve the existing redacted API DTO and replay idempotency paths with focused unit, integration, E2E, and stress selectors.

## Risk Notes

- Avoid holding a database transaction across slow or unbounded sink work unless the sink path is demonstrably local and bounded.
- Preserve current redaction guarantees from SEC-055-001.
- Preserve the source/output boundary: the adapter must not dispatch output directly.
- The scanner regression should use malformed row bytes directly because the live PostgreSQL `JSONB` column rejects malformed JSON before it reaches the scanner.
