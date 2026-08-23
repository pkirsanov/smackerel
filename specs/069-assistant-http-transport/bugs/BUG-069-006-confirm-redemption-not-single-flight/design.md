# Design: BUG-069-006 - Atomic confirm redemption

## Root Cause Analysis

### The mechanism

`confirm.Machine` treats "redeem this confirm card" as a read-modify-write over
`assistant_conversations.pending_confirm`, expressed as three independent calls
through the `assistantctx.Store` interface:

```go
// internal/assistant/confirm/machine.go, Confirm() at line 201
conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)          // 205  READ
if !ok || conv.PendingConfirm == nil ||
   conv.PendingConfirm.ConfirmRef != in.ConfirmRef {                 // 209  CHECK
	return ConfirmResult{}, ErrPendingNotFound
}
pending := *conv.PendingConfirm
// Single-flight: clear pending BEFORE writing the audit row so a
// racing callback hits ErrPendingNotFound.                          // 213
conv.PendingConfirm = nil
if err := m.store.Persist(ctx, conv); err != nil { ... }             // 217  WRITE
```

`Store` (`internal/assistant/context/store.go` line 143) exposes `Load`,
`Persist`, `DeleteByKey`, and `SweepIdle`. It has no transaction handle, no
conditional write, and no compare-and-swap. The interface offers the Machine no
way to express "clear this only if it still holds this reference", so the Machine
does the only thing available: read, decide in Go, write unconditionally.

The window between line 205 and line 217 spans a network round trip to Postgres,
JSON unmarshalling of four columns, and a second round trip. Two goroutines that
enter it together both read a live pending row, both find the reference matching,
and both proceed.

### Why the existing single-flight reasoning is incomplete

The comment at line 213 is not wrong about what it claims. Ordering the
`Persist` before the audit write does close a race - the one where a second
callback arrives after the row has been cleared. That second callback reads a
nil `PendingConfirm` and correctly returns `ErrPendingNotFound`.

The error is in the scope of the claim. It defends the interval *after* the
write. It says nothing about the interval *before* it, and the vulnerable
interval is the one before it. Two callers whose `Load` calls both complete
before either `Persist` never observe each other at all. Ordering two operations
within one goroutine cannot serialise two goroutines.

### Why no other layer catches it

**The persistence layer cannot detect the loss.** `PgStore.Persist`
(`internal/assistant/context/pg_store.go` line 99) is an unconditional upsert:

```sql
INSERT INTO assistant_conversations (...) VALUES (...)
ON CONFLICT (user_id, transport) DO UPDATE
    SET working_context = EXCLUDED.working_context,
        pending_confirm = EXCLUDED.pending_confirm, ...
```

There is no `WHERE` on the `DO UPDATE`, no `confirm_ref` predicate, and no
version column. Migration `internal/db/migrations/041_assistant_conversations.sql`
defines the table with primary key `(user_id, transport)`; its `schema_version`
column is documented in that migration's own header as a JSONB *shape* version
bumped only by shape-changing migrations, so it is not an optimistic-lock token.

There is also a seam problem that shapes the fix. The call site is:

```go
if _, err := s.pool.Exec(ctx, q, ...); err != nil {
```

The `pgconn.CommandTag` - the only thing that carries rows-affected - is
discarded. Adding a predicate to this statement would therefore be *silently
useless*: the losing writer would match zero rows and still return `nil`. Any
conditional-write fix must introduce a seam that returns the outcome, not merely
add a `WHERE` clause to the existing one. This is the single most important
implementation consequence in this document.

**The transport layer is keyed on the wrong field.**
`internal/assistant/httpadapter/dedup.go` lines 20-24 define
`turnDedupKey{userDigest, transport, messageID}`. `confirm_ref` is not part of
it. Two POSTs sharing a `confirm_ref` with different `transport_message_id`
values are different keys and both get owner leases. That is correct for an
idempotency cache - distinguishing distinct client messages is its job - but it
means the transport provides nothing here.

**No lock exists anywhere.** Searching
`FOR UPDATE|pg_advisory|BeginTx|\.Begin\(|sync\.Mutex|sync\.RWMutex|singleflight`
across `internal/assistant/confirm/` and `internal/assistant/context/` returns
matches only in `_test.go` files.

### Blast radius is wider than the reported finding

`Confirm` is one of three paths with an identical shape:

| Path | Load | Check | Persist |
|---|---|---|---|
| `Machine.Confirm` | 205 | 209 | 217 |
| `Machine.Discard` | 261 | 265 | 271 |
| `Machine.SweepTimeouts` | 319 | 323 | 330 |

`SweepTimeouts` carries the comment
`// Already resolved by a racing confirm/discard — skip.` - it anticipates
races and relies on the same non-atomic check to handle them. A `Confirm`
racing a `SweepTimeouts` can therefore write **both** a `confirmed` and a
`discarded_timeout` audit row for one reference. That is audit corruption, a
different failure than double execution, and it is why the fix must cover all
three paths rather than `Confirm` alone.

`Machine.Propose` is excluded. It replaces any existing pending confirm by
design (`machine.go` lines 121-123), so a last-writer-wins race there is the
specified behaviour, not a defect.

## Fix Design

### Option analysis

**Option A - conditional write (compare-and-swap).** Add a store method that
clears the pending row only when it still holds the expected reference, and
returns whether it did:

```sql
UPDATE assistant_conversations
   SET pending_confirm = NULL, last_activity_at = $3
 WHERE user_id = $1 AND transport = $2
   AND pending_confirm ->> 'confirm_ref' = $4
```

The JSON key is `confirm_ref`, confirmed from the struct tag at
`internal/assistant/context/store.go` line 72
(`ConfirmRef string \`json:"confirm_ref"\``).

Postgres evaluates the predicate and the write as one atomic statement under
`READ COMMITTED`. Of two concurrent executions, the second re-reads the row
after the first commits, finds the predicate false, and reports zero rows
affected. The Machine maps zero rows to `ErrPendingNotFound`.

- One round trip, no transaction management, no lock to leak.
- Correct across processes, because Postgres is the shared arbiter.
- Requires a new `Store` method returning `(bool, error)` or
  `(int64, error)`. It cannot ride on `Persist`, both because `Persist` is a
  whole-row upsert with different semantics and because `Persist` discards the
  `CommandTag`.
- Naturally leaves `working_context`, `pending_disambig`, `pending_clarify`, and
  `legacy_retirement_notices` untouched, satisfying EB-6 without extra care.
- Every `Store` implementation must gain the method, including
  `InMemoryContextStore` (`internal/assistant/testing_support.go` line 97),
  whose version must be genuinely atomic under its own mutex or unit tests will
  pass while production stays broken.

**Option B - row lock inside a transaction.** `BeginTx`, then
`SELECT ... FOR UPDATE` on the row, evaluate in Go, `UPDATE`, commit.

- Correct, and generalises to multi-statement work.
- Costs a transaction-shaped seam in `Store`, which currently has none.
  Introducing transaction lifetimes through this interface is a materially
  larger change than adding one method.
- Holds a row lock across application logic. Any slow path inside the
  transaction becomes lock hold time on a per-user row.
- Warranted only if redemption later needs several statements to be atomic
  together. It does not today.

**Option C - Postgres advisory lock** keyed on a hash of
`(user_id, confirm_ref)`.

- Correct across processes.
- Introduces a second, parallel locking regime alongside the row it protects,
  with its own leak-on-crash and key-collision considerations
  (`pg_advisory_xact_lock` mitigates the leak by scoping to a transaction, which
  reintroduces Option B's transaction seam).
- Protects a *name*, not the row. Any future writer that forgets to take the
  same lock silently bypasses it. Option A cannot be bypassed, because the
  guarantee lives in the statement that performs the write.

**Option D - in-process `sync.Mutex`. Rejected.** The reasoning matters, because
it would appear to work.

Verified: no compose file in this repository sets `replicas`
(`docker-compose.yml`, `docker-compose.prod.yml`, `deploy/compose.deploy.yml`
contain no such directive; every `deploy:` block carries only
`resources: limits:`). There is one `smackerel-core` container per project
today. **A process-local mutex would therefore be sufficient for correctness
right now.**

It is still the wrong answer:

1. Its failure mode on scale-out is silent. Adding a second replica reintroduces
   the defect with no failing test, no error, and no log line - the action simply
   executes twice again. A guarantee that evaporates without signal is worse than
   a visible constraint.
2. It puts the invariant in the wrong place. The shared state is the database
   row; the guarantee belongs where the row is written.
3. This product already carries one process-local coordination assumption -
   BUG-069-004's design commits to a "process-local FIFO transport cache". That
   is defensible for transport retry collapse, where the retry reaches the same
   process. It is not defensible for a redemption lock. Adding a second such
   assumption compounds a constraint that is currently undocumented as a
   deployment invariant.

If Option A is ever judged too invasive, the honest alternative is a mutex
**plus** a deployment invariant recorded where operators will see it, plus a
test that fails when a second replica is configured. A bare mutex with no such
guard would be a regression waiting on a config change.

### Selected approach

**Option A**, extended to all three redemption paths.

1. Add one conditional-clear method to `assistantctx.Store`, along the lines of
   `ClearPendingConfirm(ctx, userID, transport, confirmRef string, now time.Time) (bool, error)`,
   returning whether this caller performed the clear.
2. Implement it on `PgStore` as the single `UPDATE ... WHERE pending_confirm ->> 'confirm_ref' = $4`
   above, reading `CommandTag.RowsAffected()` - the value the current `Persist`
   discards - and returning `rows == 1`.
3. Implement it on `InMemoryContextStore` with genuine atomicity under its
   existing lock, so unit-level concurrency tests are meaningful rather than
   decorative.
4. Rewrite `Confirm`, `Discard`, and `SweepTimeouts` to call it in place of the
   `Persist`-after-Load pattern, and to return `ErrPendingNotFound` when it
   reports `false`. The `Load` stays, because the payload and scenario id are
   still needed to build the audit row; it simply stops being the authority on
   whether redemption may proceed.
5. Correct the comment at line 213 to state the guarantee the code now provides,
   per EB-7.

Ordering note: the audit write must remain after a successful clear. Only the
caller that observed `rows == 1` writes an audit row, which is what makes EB-3
and EB-7 hold together.

### Consequence for `Persist`

`Persist` is unchanged. It remains the whole-row upsert used by `Propose` and by
the facade's ordinary conversation writes, where last-writer-wins is correct.
The new method sits beside it for the one operation that needs a predicate. This
keeps the change additive and leaves BUG-069-004's surface untouched.

### Schema impact

None. No migration is required. The predicate reads the existing `pending_confirm`
JSONB column. An index is not required either: the primary key `(user_id, transport)`
already selects a single row, and the JSON predicate is evaluated on that row.

## Security And Privacy

- No new data is stored and no column is added.
- The `confirm_ref` is already persisted in `pending_confirm`; using it as a
  predicate exposes nothing new.
- The refusal path must not leak which reference *is* live. Returning the
  existing `ErrPendingNotFound` unchanged satisfies EB-2 and keeps the loser
  indistinguishable from any other non-owner.
- `SEC-069-005-2` observed that the disambiguation reference is a predictable
  nanosecond timestamp. Confirm references are looked up only within the
  conversation already loaded from the auth-derived `(UserID, Transport)` key,
  so predictability is not exploitable across users here. This fix must not
  introduce any lookup keyed on `confirm_ref` alone, since that would remove the
  property that currently makes predictability harmless. Recorded as a row in
  `report.md` `## Discovered Issues`.

## Test Design

The proving test must spawn real concurrency. Shape:

```go
var wg sync.WaitGroup
results := make([]error, 2)
start := make(chan struct{})
for i := 0; i < 2; i++ {
    wg.Add(1)
    go func(i int) {
        defer wg.Done()
        <-start                      // release both together
        _, results[i] = machine.Confirm(ctx, in, now)
    }(i)
}
close(start)
wg.Wait()
// exactly one nil, exactly one ErrPendingNotFound, exactly one side effect
```

The barrier channel matters: without it the goroutines may serialise by
scheduling accident and the test passes for the wrong reason.

**Adversarial requirement.** The test MUST be run against the unfixed
implementation and MUST fail there. A concurrency test that has never been seen
red proves nothing, because a race that does not reliably interleave produces a
green run on broken code. If the natural interleaving proves unreliable, the
implement phase should widen the window deliberately - a store decorator that
delays between `Load` and the clear - rather than accept an unproven green.

A third sequential turn is explicitly not acceptable evidence for AC-1.
`TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` already passes
against the defective code, which is exactly why it cannot demonstrate this fix.

## Rollback

Revert the new `Store` method, its two implementations, and the three call-site
changes. No migration to unwind, no configuration to restore, no deploy step. The
prior behaviour returns in full, including the defect.
