# BUG-069-006 - Confirm redemption is not single-flight under concurrency

**Status:** Confirmed by static analysis - not yet reproduced under live concurrency
**Severity:** Medium - a gated action can execute twice when two confirms race
**Spec:** 069-assistant-http-transport
**Discovered:** 2026-08-23 by `bubbles.security` during BUG-069-005
**Origin finding:** `SEC-069-005-1` (MEDIUM, OWASP A04 Insecure Design, TOCTOU)
**Class:** Time-of-check-to-time-of-use on confirm-card redemption

## Summary

`confirm.Machine` redeems a confirm card with a read-modify-write built from
three independent store calls. Nothing makes that triple atomic: there is no
transaction, no row lock, no advisory lock, no in-process mutex, and no
compare-and-swap. Two goroutines that reach `Machine.Confirm` with the same
`confirm_ref` before either has persisted the cleared pending row will both
observe the pending row as live, both pass the ownership check, and both return
a populated `ConfirmResult` to the action executor. The gated action then runs
twice.

The code asserts the opposite. `internal/assistant/confirm/machine.go` line 213
carries the comment:

```go
// Single-flight: clear pending BEFORE writing the audit row so a
// racing callback hits ErrPendingNotFound.
```

That ordering is real and it does defeat one race - a callback arriving after
the `Persist` but before the audit write. It does not defeat a second caller
that performed its `Load` before the first caller's `Persist`. The comment
therefore overstates the guarantee the implementation provides, which is how the
gap survived review.

## Scope Discipline - What This Packet Is Not

The **sequential** replay property is real, is proven, and is not challenged
here. `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` in
`tests/e2e/assistant/http_confirm_test.go` issues turn 1 (propose), turn 2
(confirm), then turn 3 (replay of the same reference) and asserts the gated
action executed exactly once. That test passes and its assertion is sound. A
replay is sequential by construction.

BUG-069-005's scenario is satisfied. Nothing in this packet reopens it, and no
artifact under `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/`
is modified by this packet.

What that test does not cover is **concurrency**. Verified by direct inspection:
`tests/e2e/assistant/http_confirm_test.go` contains zero occurrences of
`go func`, `sync.WaitGroup`, or `errgroup`. Its three turns are strictly
ordered. A sequential third turn cannot exercise an interleaving that only
exists when two `Load` calls both complete before either `Persist`.

One detail of that test is worth stating because it strengthens this bug rather
than weakening it. Turn 3 deliberately uses a **different**
`transport_message_id` while reusing the same `confirm_ref`
(`http_confirm_test.go` line 94). That is precisely the request shape described
below. The existing test therefore already demonstrates that such a request
reaches `Machine.Confirm` - the HTTP dedup cache does not intercept it. The only
thing that stops it re-executing is that, sequentially, the pending row has
already been cleared. Remove the sequencing and nothing stops it.

## Reproduction

Not yet reproduced under live concurrency. No test suite was executed for this
packet: the test lane holds a `flock` suite lock (exit 73 = held) while a
`git push` pre-push validation runs. The reproduction below is the design of
record for the implement phase, not a claim of execution.

Intended failing reproduction:

1. Propose a gated action so `assistant_conversations.pending_confirm` holds one
   live `confirm_ref` for `(user_id, transport)`.
2. Issue two confirms for that same `confirm_ref` concurrently, each carrying a
   distinct `transport_message_id`, spawned with `go func` plus `sync.WaitGroup`
   so both `Load` calls are in flight before either `Persist` lands.
3. Count the side effects of the gated action.

Expected on a correct implementation: exactly one execution; the losing caller
receives `confirm.ErrPendingNotFound`.

Expected today: both callers receive a populated `ConfirmResult` and the action
executes twice.

## Evidence

All five items below were verified in this session by reading the current tree.

### 1. The redemption triple is not atomic - CONFIRMED

`internal/assistant/confirm/machine.go`, `func (m *Machine) Confirm` at line 201:

| Line | Operation |
|---|---|
| 205 | `conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)` |
| 209 | `if !ok \|\| conv.PendingConfirm == nil \|\| conv.PendingConfirm.ConfirmRef != in.ConfirmRef` |
| 213 | the single-flight comment |
| 217 | `if err := m.store.Persist(ctx, conv); err != nil` |

Three separate store calls with no enclosing transaction and no lock. The window
between line 205 and line 217 is unguarded.

### 2. No locking primitive exists on the production path - CONFIRMED

```
grep -rE 'FOR UPDATE|pg_advisory|BeginTx|\.Begin\(|sync\.Mutex|sync\.RWMutex|singleflight' \
  internal/assistant/confirm/ internal/assistant/context/
```

Four matches, all in test files:

```
internal/assistant/confirm/machine_test.go:25:  mu   sync.Mutex
internal/assistant/confirm/machine_test.go:75:  mu   sync.Mutex
internal/assistant/context/gauge_refresher_test.go:35:  mu     sync.Mutex
internal/assistant/context/gauge_refresher_test.go:79:  mu    sync.Mutex
```

Zero matches in non-test source.

### 3. The store write cannot detect a lost race - CONFIRMED, with a correction

The origin finding named the type `PGStore`. The actual type is `PgStore`
(`internal/assistant/context/pg_store.go` line 99). The substance holds and is
in fact stronger than reported. `Persist` issues:

```sql
INSERT INTO assistant_conversations (...)
VALUES (...)
ON CONFLICT (user_id, transport) DO UPDATE
    SET working_context = EXCLUDED.working_context,
        pending_confirm = EXCLUDED.pending_confirm,
        ...
```

- Unconditional upsert keyed on `(user_id, transport)` only.
- No `confirm_ref` predicate and no `WHERE` clause on the `DO UPDATE`.
- No optimistic-concurrency version column. The table's `schema_version`
  (migration `internal/db/migrations/041_assistant_conversations.sql`) is a
  JSONB *shape* version; that migration's own header states it is bumped only by
  migrations that change the JSONB shape, so it cannot serve as a lock token.
- Executed through `s.pool.Exec` - no transaction.
- The `pgconn.CommandTag` return is discarded (`if _, err := s.pool.Exec(...)`),
  so rows-affected is not merely unchecked, it is unobservable at this seam.
  A conditional write must therefore plumb a new return value, not just add a
  predicate. This is a design consequence recorded in `design.md`.

`Load` (line ~54) uses `s.pool.QueryRow` with no `FOR UPDATE`.

### 4. The HTTP dedup key cannot close the gap - CONFIRMED

`internal/assistant/httpadapter/dedup.go` lines 20-24:

```go
type turnDedupKey struct {
	userDigest [sha256.Size]byte
	transport  string
	messageID  string
}
```

`begin()` (line 75) builds the key from `sha256.Sum256([]byte(userID))`, the
`TransportName` constant, and `messageID`. `confirm_ref` is absent. Two POSTs
that share one `confirm_ref` but carry different `transport_message_id` values
are two distinct keys, so both are granted owner leases and both invoke the
facade. This is correct behaviour for that cache - it is an idempotency cache
for transport retries, not a redemption lock - but it means the transport layer
provides no protection here.

### 5. Deployment is single-replica today - CONFIRMED, and it matters

```
grep -rn 'replicas' docker-compose*.yml deploy/compose.deploy.yml
```

returns nothing; every `deploy:` block carries only `resources: limits:`. There
is exactly one `smackerel-core` container per compose project today. The
consequence for fix selection is analysed in `design.md` - in short, a
process-local mutex would be sufficient *today* and would silently stop being
sufficient the moment a second replica is introduced, with no failing test to
signal it.

## Beyond The Original Finding

Two observations found while verifying, both recorded as rows in `report.md`
`## Discovered Issues` dated 2026-08-23 with dispositions.

**The same TOCTOU shape appears in all three redemption paths**, not only
`Confirm`. `Machine.Discard` (`Load` 261, check 265, `Persist` 271) and
`Machine.SweepTimeouts` (`Load` 319, check 323, `Persist` 330) are structurally
identical. `SweepTimeouts` even carries the comment
`// Already resolved by a racing confirm/discard — skip.`, acknowledging that
races occur while relying on the same non-atomic check to handle them. A
`Confirm` racing a `SweepTimeouts` can therefore write **both** a `confirmed`
and a `discarded_timeout` audit row for one `confirm_ref`, which is an
audit-integrity defect distinct from double execution. Any fix that makes only
`Confirm` atomic leaves this open, so the scope in `scopes.md` covers all three.

**The confirm machine is transport-agnostic.** `Machine.Confirm` is reached from
every transport, not only HTTP. A fix placed in the HTTP adapter would not
protect a race between two non-HTTP callbacks, nor a cross-transport race. This
is the second independent reason the fix belongs in the domain and persistence
layers.

## Relationship To BUG-069-004

`specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/`
("HTTP assistant retries execute the same turn more than once", Critical,
`in_progress`) was read in full before this packet was written. The two are the
same *defect family* - duplicate execution of an operation that must happen once
- and BUG-069-004 built the mechanism whose key shape leaves this gap open. They
are nonetheless different defects.

BUG-069-004's `bug.md` "Expected Behavior" explicitly includes
*"Concurrent retries collapse onto one in-flight execution."* So BUG-069-004 does
handle concurrency. It handles it **for one key**: same user, same transport,
same `transport_message_id`. This packet concerns two requests that differ in
exactly that field while sharing a `confirm_ref`.

| Dimension | BUG-069-004 | BUG-069-006 |
|---|---|---|
| Duplicate identified by | same `transport_message_id` | same `confirm_ref`, different `transport_message_id` |
| Layer | transport (`internal/assistant/httpadapter/`) | domain + persistence (`internal/assistant/confirm/`, `internal/assistant/context/`) |
| Coordination scope | process-local, per its own design | must be cross-process; the database is the shared authority |
| Transports protected | HTTP only | every transport reaching `confirm.Machine` |
| What is duplicated | a whole assistant turn | redemption of one gated action |
| Severity | Critical | Medium |

**Conclusion: this is correctly a separate packet, not extra scope on
BUG-069-004.** Four reasons:

1. **BUG-069-004's mechanism cannot close it even when fully correct.** Two
   different message ids are two different cache keys *by design*. Making them
   collide would break the idempotency cache's actual contract, which is to
   distinguish distinct client messages.
2. **The fix touches disjoint files.** BUG-069-004's design commits to
   `internal/assistant/httpadapter/`. This fix lands in
   `internal/assistant/confirm/machine.go` and
   `internal/assistant/context/pg_store.go`, neither of which is in
   BUG-069-004's Change Boundary.
3. **The coordination scopes are incompatible.** BUG-069-004's design states its
   cache follows the "established process-local FIFO transport cache" pattern.
   A process-local cache is the right answer for transport retry collapse and
   the wrong answer for a redemption lock that must hold across processes.
4. **Folding it in would widen a Critical in-flight packet** into two additional
   packages and a persistence-contract change, mid-implementation.

They should still be sequenced together at delivery, since both alter the
duplicate-execution posture of the same endpoint. That sequencing is a routing
concern recorded in `state.json` `routing.nextRequiredOwner`, not a reason to
merge the packets.

## Impact

- A gated action - a list write, an annotation, a scheduled notification -
  executes twice from one user confirmation.
- Duplicate `assistant_proposal` audit rows for one `confirm_ref`, so the audit
  trail misrepresents what the user authorised.
- A `Confirm` racing `SweepTimeouts` can record contradictory terminal outcomes
  (`confirmed` and `discarded_timeout`) for the same reference.
- Duplicate model and tool cost for the duplicated action.

Rated **Medium** rather than High because reaching it requires two confirms for
one reference to overlap within the `Load`-to-`Persist` window, and the confirm
control is user-driven. It is reachable without privilege: a double-tapped
confirm control, a client retry that mints a fresh `transport_message_id`, or
any deliberate concurrent submit.

## Expected Behavior

Two concurrent confirms of one `confirm_ref` result in exactly one execution of
the gated action. The losing caller receives `confirm.ErrPendingNotFound` and is
indistinguishable from a caller arriving after a completed sequential
redemption. Full contract in `spec.md`.
