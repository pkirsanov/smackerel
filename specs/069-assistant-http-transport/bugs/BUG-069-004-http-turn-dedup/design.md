# Design: BUG-069-004 - Bounded auth-scoped HTTP response replay

## Root Cause Analysis

`httpadapter.TurnRequest.Validate` requires `transport_message_id`, and
`Translate` copies it into `contracts.AssistantMessage`. The capability contract
explicitly says the value is adapter-owned for idempotency. However,
`HTTPAdapter.ServeHTTP` calls `a.facade.Handle` unconditionally and only echoes
the ID in `RenderJSON`. There is no lookup, in-flight collapse, or response
replay path.

Live deterministic RED used `/weather in barcelona`. The same authenticated
identity and message ID returned trace IDs ending `_6` and `_8`, proving two
facade executions. Different IDs returned distinct `_10` and `_11` traces and
the adversarial test passed.

## Fix Design

Add a private bounded response replay cache owned by `HTTPAdapter`, following
the established process-local FIFO transport cache in
`internal/whatsapp/assistant_adapter/idempotency.go`, with HTTP-specific
in-flight coordination:

1. Build a key from SHA-256 authenticated user identity digest, canonical
   transport, and transport message ID. Never retain the token or raw user ID.
2. Build a semantic request fingerprint from the decoded v1 request.
3. On the first key, install an in-flight entry and let that request own facade
   execution. Concurrent duplicates wait on the entry's completion channel or
   their own request cancellation.
4. Cache the completed HTTP status plus `TurnResponse`. Replay copies the
   cached response and replaces only `trace.request_id` with the current HTTP
   request ID; logical response/trace/emitted fields remain original.
5. Cache accepted-path 500 outcomes too, preventing a retry from duplicating
   unknown partial side effects. Validation/auth/limit errors occur before the
   cache and remain uncached.
6. Reject a same-key/different-fingerprint collision before facade invocation.
7. Expire completed entries using required
   `HTTPTransportConfig.ConversationTTL`; cap retained entries with a named
   transport safety constant matching the repository's bounded-cache pattern.
   Prefer evicting completed oldest entries; never strand in-flight waiters.

## Security And Privacy

- Cache keys retain only a one-way user digest plus opaque message ID.
- Cache values necessarily retain the response briefly to replay it; retention
  is bounded by the explicitly configured conversation TTL and capacity.
- No Authorization header, bearer/session token, request cookie, or raw user ID
  enters the cache.
- Cross-user and payload-collision adversaries are mandatory.

## Test State Isolation

The live E2E uses the SST shared identity. A reusable helper snapshots the exact
`assistant_conversations(user_id, transport)` row, deletes only that key for a
clean baseline, and restores all columns in cleanup. An adversarial test proves
an unrelated row is unchanged. No table-wide mutation is permitted.

## Rollback

Revert cache wiring/tests. No schema migration or deploy/config change is
required because the implementation uses existing required HTTP conversation
TTL configuration.
