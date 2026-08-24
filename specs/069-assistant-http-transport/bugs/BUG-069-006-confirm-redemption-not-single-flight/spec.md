# Spec: BUG-069-006 - Atomic confirm redemption

**Parent spec:** 069-assistant-http-transport
**Bug:** BUG-069-006 - confirm redemption is not single-flight under concurrency
**Origin finding:** `SEC-069-005-1` (MEDIUM, OWASP A04 Insecure Design, TOCTOU)

## Problem Statement

A confirm card authorises exactly one execution of a gated action. The current
implementation enforces that only when redemption attempts are sequential.
Redemption is a read-modify-write spread across three independent store calls
with nothing making it atomic, so two overlapping attempts on one `confirm_ref`
can both pass the ownership check and both authorise execution.

## Expected Behavior

### EB-1 - Redemption is atomic

`confirm.Machine.Confirm` succeeds for at most one caller per `confirm_ref`,
regardless of how many callers attempt it and regardless of their interleaving.
The check that the pending row still holds the supplied `confirm_ref` and the
write that clears it MUST be one indivisible operation against the shared store.

### EB-2 - The loser is refused, and is refused identically

A caller that loses the race receives `confirm.ErrPendingNotFound`. The refusal
is byte-identical to the refusal a caller receives when arriving after a
completed sequential redemption. A client MUST NOT be able to distinguish "you
lost a race" from "this was already redeemed", because both mean the same thing:
the action is not yours to trigger.

### EB-3 - Exactly one side effect

The gated action executes exactly once. Exactly one `assistant_proposal` audit
row with `Outcome=confirmed` is written for that `confirm_ref`. The losing
caller produces no side effect and no audit row.

### EB-4 - Coordination survives more than one process

The mechanism guaranteeing EB-1 MUST remain correct when more than one
`smackerel-core` process serves turns concurrently. A guarantee that holds only
within a single process is not sufficient, because its failure mode on scale-out
is silent: no test fails, no error is logged, and the action simply runs twice.

### EB-5 - Discard and timeout sweep share the guarantee

`Machine.Discard` and `Machine.SweepTimeouts` perform the same three-call
read-modify-write on the same column and MUST become atomic under the same
mechanism. Specifically, a `Confirm` racing a `SweepTimeouts` MUST produce
exactly one terminal audit row, never both a `confirmed` and a
`discarded_timeout` row for one `confirm_ref`.

### EB-6 - Sequential behaviour is unchanged

Every currently-passing behaviour is preserved exactly:

- A valid sequential confirm executes the action and returns a populated
  `ConfirmResult`.
- A sequential replay of a redeemed reference returns `ErrPendingNotFound`.
- A confirm for a reference with no pending row returns `ErrPendingNotFound`.
- A confirm whose `confirm_ref` does not match the live pending row returns
  `ErrPendingNotFound`.
- `Machine.Propose` continues to replace any existing pending confirm on
  `(user_id, transport)`.
- The `assistant_conversations` columns not involved in redemption -
  `working_context`, `pending_disambig`, `pending_clarify`,
  `legacy_retirement_notices` - are unaffected by a redemption write. The last
  of these matters: `PgStore.Persist` deliberately leaves
  `legacy_retirement_notices` untouched on the UPDATE branch so the
  legacyretirement ledger writer stays its sole owner. Any new write path MUST
  preserve that ownership.

### EB-7 - The comment matches the code

The single-flight comment at `internal/assistant/confirm/machine.go` line 213
MUST describe what the implementation actually enforces. A comment asserting a
guarantee the code does not provide is itself a defect: it is what allowed this
gap to pass review.

## Acceptance Criteria

| ID | Criterion | Proven by |
|---|---|---|
| AC-1 | Two concurrent confirms of one `confirm_ref` produce exactly one execution | Concurrent regression test spawning real goroutines |
| AC-2 | The losing caller receives `ErrPendingNotFound` | Same test, asserting the error identity of the loser |
| AC-3 | Exactly one `confirmed` audit row exists for the reference after the race | Same test, counting audit rows |
| AC-4 | The concurrent test fails against the pre-fix implementation | Adversarial proof recorded before the fix lands |
| AC-5 | Coordination holds across processes, not only goroutines | Mechanism review plus a test exercising two independent store handles |
| AC-6 | `Discard` and `SweepTimeouts` are atomic under the same mechanism | Concurrent tests for each path |
| AC-7 | A `Confirm` racing a `SweepTimeouts` yields one terminal audit row | Concurrent cross-path test |
| AC-8 | All EB-6 sequential behaviours still pass | Existing suites, unchanged |
| AC-9 | A redemption write does not disturb the other pending columns or `legacy_retirement_notices` | Column-level assertion |

## Non-Goals

- Changing the HTTP turn dedup key. That cache is correctly keyed on
  `transport_message_id` for its own purpose; see
  `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/design.md`.
- Altering the sequential replay contract, which is already correct and proven by
  `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce`.
- Introducing a distributed lock service. Postgres is already the shared
  authority for this row and is sufficient.
- Changing the confirm-card user experience, the proposal flow, or the audit row
  schema.

## Adversarial Requirement

A regression test for AC-1 MUST spawn genuine concurrency - `go func` with
`sync.WaitGroup`, or an errgroup - and MUST be shown to fail against the
unfixed implementation. A third sequential turn does not prove AC-1 and MUST NOT
be accepted as evidence for it: the existing sequential test already passes
today, against the very code this packet reports as defective, which is precisely
why it cannot serve as proof.

### Single-Capability Justification

This packet introduces no reusable capability and no second provider, so the
capability-foundation shape does not apply to it. It repairs ONE behaviour of
ONE existing capability: the redemption of a pending confirmation reference in
`internal/assistant/confirm`.

The trigger words that make the gate look here — "implementation", "store" —
refer to the pre-existing `assistantctx.Store` interface, which this packet
extends by one method rather than founding. There is exactly one production
implementation of that method, `PgStore`; the other implementations are
in-memory test doubles, which are fixtures rather than variants and carry no
variation axis. Nothing here is selected at runtime, configured per
environment, or swapped per tenant.

A Domain Capability Model would therefore describe a capability of one member
with no axis of variation, which states nothing a reader could act on. The
proportionate record is this justification.
