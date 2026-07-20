# User Validation: BUG-071-002 Intent replay SST wiring

## Checklist

- [x] The replay CLI honors the explicit `assistant.intent_trace.replay_enabled` SST value.
- [x] A known trace replays read-only with matching route and tool calls.
- [x] An unknown trace retains the canonical not-found exit code and vocabulary.
