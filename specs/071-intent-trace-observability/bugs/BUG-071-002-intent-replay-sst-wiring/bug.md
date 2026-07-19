# Bug: BUG-071-002 Intent replay SST is ignored

## Summary

`assistant.intent_trace.replay_enabled` is explicitly true in the Smackerel SST, but the generator emits none of the intent-trace keys and aggregate config loading never calls the intent-trace loader. Both replay E2E scenarios therefore observe the zero-value false capability.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Configured operator capability is unavailable
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status

- [ ] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Generate the canonical test config through the repository CLI.
2. Run the two intent replay E2E tests on the disposable stack.
3. Observe both subprocesses stop at `assistant.intent_trace.replay_enabled is false`.

## Expected Behavior

Every required `assistant.intent_trace.*` key is compiled into generated env, aggregate config loading validates all five values, and the replay CLI honors the explicit SST boolean. Missing or invalid values abort config loading.

## Actual Behavior

The generated env omits all five keys and `loadAssistantConfig` does not invoke `loadIntentTraceConfig`, leaving `ReplayEnabled` false.

## Environment

- Service: `smackerel-core assistant replay-intent`
- Version: `7ca186217c007a24075b2273275a22434d89fc44`
- Platform: Linux, repository-managed disposable Docker stack

## Error Output

```text
smackerel-core assistant replay-intent: assistant.intent_trace.replay_enabled is false
exit status 5
```

## Root Cause

The spec-071 typed loader was implemented and tested in isolation but omitted from both config compilation and aggregate runtime loading. Unit tests exercised only the detached helper and could not detect the missing consumers.

## Related

- Feature: `specs/071-intent-trace-observability/`
- Companion packet: `BUG-071-001-canonical-metrics-endpoint`
