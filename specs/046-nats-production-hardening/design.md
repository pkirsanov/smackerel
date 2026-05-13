# Design: NATS Production Hardening

## Current Truth

The review found that NATS behavior needs product-level hardening before the runtime can be treated as always-on. The NATS client should survive restarts, the server should have explicit storage and payload ceilings, and every stream should be bounded.

## Proposed Design

### Client Reconnect

- Update ML sidecar NATS connection configuration to request indefinite reconnect.
- Surface reconnect interval and max attempts through configuration where the library supports it.
- Add tests that interrupt and restore NATS in a disposable stack.

### Server Limits

- Add SST-backed fields for `max_payload`, `max_file_store`, and `max_mem_store`.
- Generate NATS runtime configuration from those fields.
- Add a config or compose contract test that fails on absent limits.

### Stream Caps

- Inventory every stream Smackerel creates.
- Set `MaxBytes` and retention policy for each stream.
- Add tests that inspect stream configuration after creation.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-046-001 | unit/config | Missing NATS limit values fail validation. |
| T-046-002 | integration | ML sidecar reconnects after NATS restart. |
| T-046-003 | integration | Stream configuration contains MaxBytes caps. |
| T-046-004 | stress | High message volume respects stream caps without unbounded disk growth. |
| T-046-005 | artifact | Artifact lint passes for this feature. |

## Risk Controls

- Reconnect tests must use disposable runtime state.
- Stream caps must be high enough for normal bursts and low enough to prevent disk exhaustion.
- NATS limits must be documented as operator-tunable SST values, not hidden defaults.
