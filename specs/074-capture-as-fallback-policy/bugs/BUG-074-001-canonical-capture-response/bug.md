# Bug: BUG-074-001 No-ground capture preserves error response fields

## Summary

After a successful open-knowledge no-ground capture, the facade retains the upstream refusal body and `provider_unavailable` error cause instead of returning the canonical saved-as-idea capture response.

## Severity

- [ ] Critical
- [x] High
- [ ] Medium
- [ ] Low

## Status

- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Run the two capture response tests against a clean disposable stack.
2. Observe `capture_route=true` and saved-as-idea status.
3. Observe the stale refusal body and error cause.

## Expected Behavior

A successful fallback capture returns the canonical normal response: saved-as-idea status, capture route true, canonical acknowledgement body, empty error cause, and no confirm/disambiguation payload.

## Actual Behavior

The successful persistence hook does not rewrite the upstream refusal envelope.

## Error Output

```text
body = "I don't have a sourced answer for that."; expected canonical 'saved as an idea' acknowledgement
error_cause = "provider_unavailable" on capture fallback; want empty
```

## Root Cause

The no-ground hook calls `runCaptureFallback` but canonicalizes only the failure branch. The success branch leaves `resp` untouched.

## Related

- Feature: `specs/074-capture-as-fallback-policy/`
