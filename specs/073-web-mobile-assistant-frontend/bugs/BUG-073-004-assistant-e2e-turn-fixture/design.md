# Design: BUG-073-004 - Replace parity reset fixture and isolate shared HTTP state

## Root Cause Analysis

`internal/assistant/facade.go` handles `KindReset` before routing or invocation:
it deletes the exact conversation key and returns `"context reset."`. Because
no `agent.InvocationResult` exists, `httpadapter.RenderJSON` has no invocation
trace from which to populate `trace.assistant_turn_id`.

The affected E2E helpers instead construct `KindText` with `Text: "/reset"`.
The transport translates it as an HTTP message, and the facade recognizes the
reset command. The tests then incorrectly use that command response as proof of
normal facade parity and idempotency. Separately, `TransportName` is the
canonical HTTP adapter token `web`; the request's `transport_hint` is copied
only to `TransportMetadata` and intentionally cannot change the response
transport.

## Fix Design

1. Add one assistant E2E state-isolation helper that resolves the SST test
   identity, snapshots its exact `assistant_conversations` row for transport
   `web`, deletes only that key to establish a clean baseline, and restores the
   original row in `t.Cleanup`. An absent original row is restored as absent.
2. Use a unique, non-command ordinary text prompt that reaches the real facade.
   Keep the prompt equivalent across parity calls and restore the baseline
   between calls so transport hint is the only changed input.
3. Assert the canonical response transport remains `web` for both hints. Compare
   only non-volatile contract fields; do not add an expectation that the mobile
   hint rewrites the adapter identity.
4. Add an adversarial isolation test proving an unrelated conversation row is
   untouched and the target row's full JSON/state is restored exactly.

## Data Safety

The helper may address only the primary key `(user_id, transport)`. It may not
use unqualified `UPDATE`, `DELETE`, or table truncation. Snapshot and restore
must include all persisted columns so pending confirm/disambiguation/clarify
state and timestamps are not lost.

## Production Boundary

The expected implementation changes parity test code and test support only.
Production assistant semantics remain unchanged. The separately proven HTTP
dedup defect is owned by BUG-069-004.

## Rollback

Revert the test/helper changes. No schema, runtime config, deployment, or
persistent production state is involved.

### Single-Implementation Justification

This bugfix adds no new implementation, provider, or variant (Gate G094). It
corrects a stale E2E fixture (swapping the `/reset` short circuit for an
ordinary text turn) and adds one exact-key conversation-isolation test helper.
The production `internal/assistant/facade.go` and `internal/assistant/httpadapter`
remain a single, unchanged implementation; there are no alternative
implementations, no variation axes, and no foundation/overlay split. A
capability-foundation split therefore does not apply.
