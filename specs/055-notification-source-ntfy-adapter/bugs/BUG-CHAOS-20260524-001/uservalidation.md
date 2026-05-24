# User Validation: BUG-CHAOS-20260524-001

## Checklist

- [x] Replay controls remain visible only for eligible ntfy dead letters.
- [x] Replaying a dead letter once still routes through `SourceEventSink`.
- [x] Replaying or double-clicking the same dead letter repeatedly does not create duplicate notifications.
- [x] Replay responses remain redacted and do not expose raw payload bytes.
- [x] Malformed persisted ntfy redaction state fails loud instead of appearing as an empty redaction map.
