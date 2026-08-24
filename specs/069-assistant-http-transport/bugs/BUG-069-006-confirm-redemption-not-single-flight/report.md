# Report: BUG-069-006 - Atomic confirm redemption

**Packet status:** in_progress
**Workflow mode:** bugfix-fastlane
**Phase performed:** bug discovery, documentation, and root-cause analysis
**Phase NOT performed:** implementation, test authoring, test execution

## Summary

This packet documents and root-causes `SEC-069-005-1`, a MEDIUM OWASP A04
time-of-check-to-time-of-use defect raised by `bubbles.security` during
BUG-069-005 and recorded in that packet's `## Discovered Issues` table dated
2026-08-23.

`confirm.Machine` redeems a confirm card with a read-modify-write assembled from
three independent `Store` calls with no transaction, no row lock, no advisory
lock, no mutex, and no compare-and-swap. Two callers whose `Load` calls both
complete before either `Persist` will both pass the ownership check and both
authorise the gated action, so it executes twice.

Every evidence claim carried in from the origin finding was independently
re-verified against the current tree in this session. Four confirmed as stated;
one confirmed with a type-name correction and turned out to be stronger than
reported. Two additional defects in the same mechanism were found during
verification and are recorded in `## Discovered Issues`.

No product or test source was modified by this phase. No DoD item is checked. No
scope status was advanced. No certification field was written.

**Change Boundary status.** The four implementation files and three test files
named in `scopes.md` under `Allowed file families` were read but not modified.
Zero files under `Excluded surfaces` were touched: no artifact of BUG-069-004 or
BUG-069-005 was changed, and nothing under `.github/bubbles/` was changed.

## Test Evidence

**No test suite was executed for this packet, and none is claimed.** The test
lane holds a `flock` suite lock while a `git push` pre-push validation runs; a
concurrent invocation exits 73 (lock held). Running it would have produced a
lock-contention exit, not a behavioural result, and recording that as test
evidence would be worthless.

Consequently:

- `## Discovered Issues` carries no row asserting any test outcome.
- The DoD item "The concurrent redemption test is recorded FAILING against the
  unfixed implementation before any fix lands" is unchecked, and is the first
  execution obligation of the implement phase.
- No claim anywhere in this packet asserts that a test passed or failed.

The evidence below is **static verification** performed in this session. Each
block is tagged with its claim source. `executed` means the command was run in
this session and the output is its real output. Nothing here is `interpreted` or
`not-run`.

### Claim 1 - the redemption triple is not atomic

**Claim Source:** executed

```
$ grep -nE 'func \(m \*Machine\) Confirm|m\.store\.Load|ConfirmRef != in\.ConfirmRef|Single-flight: clear pending|m\.store\.Persist' internal/assistant/confirm/machine.go
128:	conv, _, err := m.store.Load(ctx, in.UserID, in.Transport)
167:	err := m.store.Persist(ctxSpan, conv)
201:func (m *Machine) Confirm(ctx context.Context, in ConfirmInput, now time.Time) (ConfirmResult, error) {
205:	conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)
209:	if !ok || conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != in.ConfirmRef {
213:	// Single-flight: clear pending BEFORE writing the audit row so a
217:	if err := m.store.Persist(ctx, conv); err != nil {
261:	conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)
265:	if !ok || conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != in.ConfirmRef {
271:	if err := m.store.Persist(ctx, conv); err != nil {
319:		conv, ok, err := m.store.Load(ctx, e.UserID, e.Transport)
330:		if err := m.store.Persist(ctx, conv); err != nil {
```

**CONFIRMED.** Load 205, check 209, comment 213, Persist 217 - exactly the line
numbers the origin finding cited. The output also exposes the same shape at
261/265/271 (`Discard`) and 319/330 (`SweepTimeouts`), which the origin finding
did not mention; see `## Discovered Issues` row DIS-069-006-1.

### Claim 2 - no locking primitive on the production path

**Claim Source:** executed

```
$ grep -rnE 'FOR UPDATE|pg_advisory|BeginTx|\.Begin\(|sync\.Mutex|sync\.RWMutex|singleflight' internal/assistant/confirm/ internal/assistant/context/
internal/assistant/confirm/machine_test.go:25:	mu   sync.Mutex
internal/assistant/confirm/machine_test.go:75:	mu   sync.Mutex
internal/assistant/context/gauge_refresher_test.go:35:	mu     sync.Mutex
internal/assistant/context/gauge_refresher_test.go:79:	mu    sync.Mutex
grep_rc=0
```

**CONFIRMED.** Four matches, every one in a `_test.go` file. Zero in non-test
source.

### Claim 3 - the single-flight comment overstates the guarantee

**Claim Source:** executed. Read from `internal/assistant/confirm/machine.go`
lines 212-215:

```go
	pending := *conv.PendingConfirm
	// Single-flight: clear pending BEFORE writing the audit row so a
	// racing callback hits ErrPendingNotFound.
	conv.PendingConfirm = nil
```

**CONFIRMED.** The comment reads exactly as reported. The ordering it describes
is real and does defeat a race against the audit write. It does not defeat a
second goroutine whose `Load` preceded this `Persist`, which is the vulnerable
interval.

### Claim 4 - the store write cannot detect a lost race

**Claim Source:** executed. Read from `internal/assistant/context/pg_store.go`,
`func (s *PgStore) Persist` at line 99:

```sql
INSERT INTO assistant_conversations
    (user_id, transport, working_context, pending_confirm, pending_disambig, pending_clarify, last_activity_at, schema_version, legacy_retirement_notices)
VALUES
    ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7, $8, '[]'::jsonb)
ON CONFLICT (user_id, transport) DO UPDATE
    SET working_context  = EXCLUDED.working_context,
        pending_confirm  = EXCLUDED.pending_confirm,
        pending_disambig = EXCLUDED.pending_disambig,
        pending_clarify  = EXCLUDED.pending_clarify,
        last_activity_at = EXCLUDED.last_activity_at,
        schema_version   = EXCLUDED.schema_version
```

executed through:

```go
	if _, err := s.pool.Exec(ctx, q, ...); err != nil {
```

and the table definition from
`internal/db/migrations/041_assistant_conversations.sql`:

```
$ grep -n 'assistant_conversations' -A 22 internal/db/migrations/041_assistant_conversations.sql
21:CREATE TABLE IF NOT EXISTS assistant_conversations (
22-    user_id           TEXT        NOT NULL,
23-    transport         TEXT        NOT NULL,
24-    working_context   JSONB       NOT NULL DEFAULT '{}'::jsonb,
25-    pending_confirm   JSONB,
26-    pending_disambig  JSONB,
27-    last_activity_at  TIMESTAMPTZ NOT NULL,
28-    schema_version    INTEGER     NOT NULL DEFAULT 1,
29-    PRIMARY KEY (user_id, transport)
30-);
```

**CONFIRMED, with one correction and one strengthening.**

- *Correction:* the type is `PgStore`, not `PGStore` as the origin finding wrote.
  Substance unaffected.
- *As claimed:* unconditional upsert, no `confirm_ref` predicate, no `WHERE` on
  the `DO UPDATE`, no optimistic-concurrency version column. `schema_version` is
  a JSONB shape version - lines 17-19 of that migration state it is bumped only
  by migrations that change the JSONB shape - so it is not a lock token.
- *Stronger than claimed:* the `pgconn.CommandTag` is discarded via `_`. Rows
  affected is not merely unchecked, it is unobservable at this seam. Adding a
  predicate to this statement alone would be silently useless, because the
  losing writer would match zero rows and still return `nil`. This drives the
  fix design in `design.md`.

### Claim 5 - the HTTP dedup key cannot close the gap

**Claim Source:** executed. Read from `internal/assistant/httpadapter/dedup.go`
lines 20-24 and 75-80:

```go
type turnDedupKey struct {
	userDigest [sha256.Size]byte
	transport  string
	messageID  string
}

func (c *turnResponseCache) begin(userID, messageID string, fingerprint [sha256.Size]byte) (*turnDedupLease, error) {
	key := turnDedupKey{
		userDigest: sha256.Sum256([]byte(userID)),
		transport:  TransportName,
		messageID:  messageID,
	}
```

**CONFIRMED.** The key is `{userDigest, transport, messageID}`. `confirm_ref` is
absent. Two POSTs sharing a `confirm_ref` with different `transport_message_id`
values are distinct keys and both receive owner leases.

Additionally verified: `entries map[turnDedupKey]*turnDedupEntry` guarded by a
`sync.Mutex` - the cache is in-process. BUG-069-004's `design.md` states this
explicitly, describing it as following the "established process-local FIFO
transport cache" pattern.

### Scope discipline - the existing sequential test is sound

**Claim Source:** executed

```
$ grep -nE 'go func|sync\.WaitGroup|errgroup' tests/e2e/assistant/http_confirm_test.go
rc=1 (1 == NONE)
```

**CONFIRMED.** `tests/e2e/assistant/http_confirm_test.go` contains no
concurrency primitive. `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce`
issues three strictly ordered turns. Its sequential-replay assertion is sound and
is not challenged by this packet.

One line of that test strengthens this bug. At line 94:

```go
	turn3Req.TransportMessageID = "e2e-scope3-confirm-3-" + timestamp()
```

Turn 3 reuses the `confirm_ref` under a **different** `transport_message_id` -
the exact request shape described in this packet. The existing test therefore
already demonstrates that such a request passes the dedup cache and reaches
`Machine.Confirm`. Sequentially it is refused only because the pending row is
already cleared. Concurrently, it would not be.

### Deployment replica posture

**Claim Source:** executed

```
$ grep -rn 'replicas' docker-compose*.yml deploy/compose.deploy.yml
replicas_rc=1 (1 == none set anywhere)

$ sed -n '138,215p' deploy/compose.deploy.yml | grep -nE 'deploy:|replicas|resources|limits'
3:    restart: unless-stopped
60:    deploy:
61:      resources:
62:        limits:
```

**CONFIRMED single-replica today.** No `replicas` directive exists in any compose
file; every `deploy:` block carries only `resources: limits:`. This is why
`design.md` records that an in-process mutex would be sufficient *today* and
still rejects it: its failure mode on scale-out is silent.

### Verification claims that could not be made

- **Live concurrent reproduction:** not performed. Requires the test lane, which
  is lock-held. This is the implement phase's first execution obligation and is
  an unchecked DoD item in `scopes.md`, not an open question.
- **Whether the race is observable at current production latencies:** not
  measured. The window spans two Postgres round trips plus JSON unmarshalling of
  four columns, so it is not vanishingly small, but no timing was taken and none
  is asserted.

## Discovered Issues

| Date | Issue | Disposition | Reference |
|---|---|---|---|
| 2026-08-23 | DIS-069-006-1: the TOCTOU shape is not confined to `Confirm`. `Machine.Discard` (Load 261, check 265, Persist 271) and `Machine.SweepTimeouts` (Load 319, check 323, Persist 330) are structurally identical. `SweepTimeouts` carries the comment `// Already resolved by a racing confirm/discard — skip.`, anticipating races while relying on the same non-atomic check. A `Confirm` racing a `SweepTimeouts` can write both a `confirmed` and a `discarded_timeout` audit row for one reference, which is audit corruption rather than double execution | Absorbed into this packet's scope rather than routed elsewhere, because the three paths need the same store method and a partially-atomic redemption path is not a certifiable state. Covered by scenario SCN-BUG069006-003 and by the DoD item "A confirm racing the timeout sweep produces exactly one terminal audit row" | `internal/assistant/confirm/machine.go` `Discard`, `SweepTimeouts`; `specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/scopes.md` Scope 1 |
| 2026-08-23 | DIS-069-006-2: `PgStore.Persist` discards the `pgconn.CommandTag` (`if _, err := s.pool.Exec(...)`), so rows-affected is unobservable at that seam. A conditional-write fix that only adds a `WHERE` predicate to the existing statement would be silently useless - the losing writer would match zero rows and still return `nil` | Absorbed into this packet's fix design as a binding implementation constraint: the fix must introduce a seam returning the write outcome, not merely add a predicate. Recorded in `design.md` under "Why no other layer catches it" and enforced by the DoD item requiring `PgStore` to read `CommandTag.RowsAffected()` | `internal/assistant/context/pg_store.go` `Persist`; `specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/design.md` |
| 2026-08-23 | DIS-069-006-3: `SEC-069-005-2` observed that pending references are predictable timestamps, harmless today only because lookup is keyed on the authenticated `(UserID, Transport)` pair rather than on the reference alone. A conditional-clear keyed on `confirm_ref` must not weaken that property | Constrains this packet's own fix: the new predicate is applied in addition to `user_id` and `transport`, never in place of them, so no lookup becomes reference-only. Recorded in `design.md` under "Security And Privacy". The underlying `SEC-069-005-2` finding remains owned by its origin packet and is not modified here | `specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/design.md`; origin row in `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/report.md` |

## Relationship To BUG-069-004

`specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/bug.md` and
`design.md` were both read in full before this packet was authored, to test
whether this finding belongs inside that packet instead.

It does not. BUG-069-004 dedups on `transport_message_id` and its own
`bug.md` commits to collapsing concurrent retries - but only for one key. This
defect involves two requests that differ precisely in that field while sharing a
`confirm_ref`, so BUG-069-004's mechanism cannot catch it even when fully
correct, and making it catch it would break the idempotency cache's actual
contract. The fix also lands in different packages
(`internal/assistant/confirm/`, `internal/assistant/context/`) than
BUG-069-004's committed surface (`internal/assistant/httpadapter/`), and demands
cross-process coordination where BUG-069-004's design deliberately chose a
process-local cache. Full comparison table in `bug.md` under
"Relationship To BUG-069-004".

Both packets alter the duplicate-execution posture of the same endpoint, so they
warrant coordinated sequencing at delivery. That is a routing concern carried in
`state.json` `routing.nextRequiredOwner`, not grounds to merge them.

## Completion Statement

This packet is **not complete** and makes no completion claim.

Completed in this phase: the packet's six required artifacts plus
`uservalidation.md` and `scenario-manifest.json` were authored; every evidence
claim inherited from `SEC-069-005-1` was independently re-verified against the
current tree with the results recorded above; two additional defects in the same
mechanism were found and recorded in `## Discovered Issues` with dispositions;
the relationship to BUG-069-004 was determined by reading that packet rather
than by assumption.

Not performed in this phase, by explicit instruction and by lane availability:
no fix was implemented, no test was authored, and no test suite was executed.
Every DoD item in `scopes.md` is unchecked and every one of them is honest -
none of the work they describe has been done.

`status` is `in_progress` and `certification.status` is `in_progress`. The
packet is owned by `bubbles.implement` per `state.json`
`routing.nextRequiredOwner`, whose first execution obligation is the unchecked
DoD item requiring the concurrent test to be recorded FAILING against the unfixed
implementation before any fix lands.

## Implementation Phase

**Phase:** implement
**Agent:** bubbles.implement
**Claim Source:** executed
**Commit:** `61b8be79` - `fix(BUG-069-006): make confirm redemption atomic via a store-arbitrated CAS`

The `## Completion Statement` above records the bug-discovery phase. This
section records the implement phase that followed it. Every DoD item in
`scopes.md` remains unchecked.

### Concurrent Regression Tests: RED Then GREEN

Command:

```bash
SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go --go-run 'TestMachineConfirm_ConcurrentRedemptionExecutesOnce|TestMachineConfirm_RacingSweepProducesOneTerminalOutcome'
```

RED, against the unfixed `machine.go` with all other files present so the
package compiled. Exit code 1:

```
--- FAIL: TestMachineConfirm_ConcurrentRedemptionExecutesOnce (0.00s)
    machine_concurrency_test.go:115: winners: got 2 want 1 — the gated action would execute 2 times
    machine_concurrency_test.go:118: losers receiving ErrPendingNotFound: got 0 want 1
    machine_concurrency_test.go:123: confirmed audit rows for "cr-1": got 2 want 1
--- FAIL: TestMachineConfirm_RacingSweepProducesOneTerminalOutcome (0.00s)
    machine_concurrency_test.go:164: terminal audit rows for "cr-1": got 2 (confirmed=1 discarded_timeout=1) want exactly 1
FAIL    github.com/smackerel/smackerel/internal/assistant/confirm       0.060s
```

GREEN, with the fix applied. Exit code 0:

```
ok      github.com/smackerel/smackerel/internal/assistant/confirm       0.050s
```

### Supporting Lanes

| Lane | Exit code | Result |
|---|---|---|
| Full Go unit suite | 0 | 149 packages ok, 0 failures |
| `./smackerel.sh format --check` | 0 | `78 files already formatted` |
| `./smackerel.sh lint` | 0 | `Web validation passed` |

### The Fix

The fix adds `Store.ClearPendingConfirm(ctx, userID, transport, confirmRef, now)
(bool, error)`, evaluated as ONE atomic operation. All three call sites,
`Confirm`, `Discard`, and `SweepTimeouts`, branch on `won`, so exactly one
caller can redeem a given `confirmRef` and every other caller takes the
loser path.

`PgStore` implements it as a single conditional `UPDATE` and returns
`tag.RowsAffected() == 1`. That return is load-bearing: `Persist` discards its
`CommandTag` via `_`, so a bare conditional `WHERE` would have silently
no-opped and reported success to every racer. Reading the affected-row count is
what makes the database, rather than the caller, the arbiter of the race.

### The Interface Addition Broke Four Store Implementations

Adding the method to `Store` broke four implementations. Three of them were test
doubles that the first pass missed: `memContextStore`, `memStore`, and
`captureStore`. `./smackerel.sh check` is config-only and never compiles test
files, so those three breakages stayed invisible until the test lane ran. A
green `check` is not evidence that the package builds under `go test`.

### Why `SMACKEREL_SKIP_HOST_PREFLIGHT=1` Was Set

The disk preflight refuses with `C: 32 GB free, required 40 GB`. That reading is
of the Windows volume backing the vhdx; the WSL ext4 filesystem the lane
actually uses has 474 GB free and passes the same threshold.

The variable is defined by `smackerel.sh` itself, not invented for this run, and
setting it is justified for this lane only. `run_go_tooling` performs
`docker run --rm` against an already-present `golang:1.25.10-bookworm` image
with the Go caches in ext4-backed named volumes. That path builds no image and
pulls nothing, so the preflight's disk threshold guards a cost this lane does
not incur. The justification does NOT generalize to the build or e2e lanes,
which do build images and therefore do need the check.
