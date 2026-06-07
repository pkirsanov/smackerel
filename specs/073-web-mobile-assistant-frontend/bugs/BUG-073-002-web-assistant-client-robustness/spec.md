# Spec: BUG-073-002 — web assistant client must be single-flight, time-bounded, and show a busy state

## Expected Behavior

The web assistant chat client MUST:
1. Dispatch at most ONE turn at a time — a rapid double-submit MUST NOT create
   two overlapping turns with different `transport_message_id`s.
2. Bound each request with a client-side timeout, surfacing a retryable error
   if the endpoint hangs.
3. Show a visible busy affordance (disabled Send + `aria-busy`) while a turn is
   in flight.

## Actual Behavior

The submit handler dispatched without awaiting and `dispatchTurn` had no
re-entry guard (race); `postTurn` had no `AbortController` (no timeout); the
Send button was never disabled (no busy affordance). See `bug.md` → "Mechanism".

## Acceptance Criteria

1. **AC-1 (single-flight):** `dispatchTurn` and the submit handler early-return
   while a turn is `inFlight`.
2. **AC-2 (timeout):** `postTurn` builds an `AbortController`, passes `signal:`
   to `fetch`, aborts after `TURN_TIMEOUT_MS`, and surfaces a timeout error.
3. **AC-3 (busy affordance):** `setComposerBusy` disables the Send button and
   toggles `aria-busy` around the in-flight turn.
4. **AC-4 (no regression):** the storage guard and codegen-drift guards still
   pass; `node --check` is clean.
5. **AC-5 (locked):** a Go source-contract guard fails if any of the three
   mechanisms is removed (adversarial twin proves detection).

## Out of Scope

- The mobile Dart client (audited clean).
- Server-side turn dedup (unchanged; owned by the assistant transport).
- Browser interaction e2e (blocked by the auth-gated disposable stack; the
  source-contract guard is the runnable lock).

## Cross-References

- Bug detail + fix + testing rationale: `bug.md`
- Parent spec/design: `../../spec.md`, `../../design.md`
