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

### Red-stage proof, captured before the fix

Scenario-first ordering for this packet. The failing capture is recorded here,
ahead of the post-fix result, so the sequence is legible in file order. The
full capture is in `## Implementation Phase` below.

**RED — the concurrency tests against the UNFIXED `machine.go`**, with every
other file of the change present so the package still compiled. Exit 1:

```
--- FAIL: TestMachineConfirm_ConcurrentRedemptionExecutesOnce (0.00s)
    machine_concurrency_test.go:115: winners: got 2 want 1 — the gated action would execute 2 times
    machine_concurrency_test.go:118: losers receiving ErrPendingNotFound: got 0 want 1
    machine_concurrency_test.go:123: confirmed audit rows for "cr-1": got 2 want 1
--- FAIL: TestMachineConfirm_RacingSweepProducesOneTerminalOutcome (0.00s)
    machine_concurrency_test.go:164: terminal audit rows for "cr-1": got 2 (confirmed=1 discarded_timeout=1) want exactly 1
FAIL    github.com/smackerel/smackerel/internal/assistant/confirm       0.060s
```

**GREEN — green-stage, the same two tests with the fix applied.** Exit 0:

```
ok      github.com/smackerel/smackerel/internal/assistant/confirm       0.050s
```

The RED run is what makes the fix falsifiable: `winners: got 2` is the
double-execution the packet was filed for, observed rather than argued.

### Superseded note on the original filing session

The original filing session executed no test suite and said so. That statement
described the filing session only and is no longer the state of this packet —
the implement phase has since run the lane above. The static verification that
follows was performed during filing and is retained because it is what located
the defect.

The blocks below are **static verification** from the filing session. Each
block is tagged with its claim source. `executed` means the command was run in
that session and the output is its real output. Nothing there is `interpreted`
or `not-run`.

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

| 2026-08-25 | DIS-069-006-4: `tests/e2e/assistant_regression/bs_004_notification_confirm.sh` is the confirm-flow shell regression suite, and it cannot validate this packet: it exits 77 with `SKIP_REASON: SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED`, and the e2e runner reports that skip as `FAIL: ... (exit=77)`. It has therefore never exercised the confirm flow it is named for. This is pre-existing and independent of this change — the fixture belongs to spec 061 SCOPE-04 — but it is the reason the "Broader E2E regression suite passes" DoD item cannot be closed here honestly rather than merely conveniently. It is also the same shape as BUG-069-005: a required suite whose skip is invisible unless someone runs it and reads the reason. | Recorded, not fixed. Authoring the notification-proposal fixture is spec 061 SCOPE-04 work and lies outside this packet's Change Boundary, which was just corrected and must not be widened to absorb it. Owner: spec 061 SCOPE-04. | `tests/e2e/assistant_regression/bs_004_notification_confirm.sh`; `./smackerel.sh test e2e --shell-run assistant_regression/bs_004_notification_confirm.sh` → exit 77 |

| 2026-08-25 | DIS-069-006-5: `scenario-manifest.json` was inaccurate in two ways at once, and the state-transition guard surfaced both. Its `SCN-BUG069006-002` entry linked `tests/e2e/assistant/http_confirm_concurrent_test.go`, a file that has never existed — the concurrent API-boundary test was added to the existing `tests/e2e/assistant/http_confirm_test.go` instead, so the manifest pointed at nothing. Separately, all four scenarios still carried `"status": "not_started"` although all four linked tests exist and pass with recorded evidence in this report. The manifest was authored during planning and never reconciled as the tests landed. | Corrected. `SCN-BUG069006-002` now links the real file, and all four statuses read `passed`, which matches the executed evidence: SCN-001 from the verbose unit run plus the integration PASS, SCN-002 and SCN-004 from the e2e PASS lines, SCN-003 from the verbose unit run. | `scenario-manifest.json`; guard Check 3C |
| 2026-08-25 | DIS-069-006-6: after that correction the guard still reports five `AMBIGUOUS-TITLE` findings, one per linked test, each saying "the title appears 2 times". No test function is defined twice — `grep -rn "func <name>"` returns exactly one hit for each. The cause is at `.github/bubbles/scripts/scenario-test-resolve.sh` line 330, `occurrences = body.count(title)`, a plain substring count over the whole file inside the branch its own comment labels a "Conservative literal scan". Go convention requires a doc comment to begin with the identifier it documents, so every one of these tests carries a line like `// TestMachineConfirm_ConcurrentRedemptionExecutesOnce covers SCN-BUG069006-001.` — and that line is occurrence two. | Recorded, not worked around. Deleting the doc comments would green the advisory by removing both a Go convention and the human-readable half of scenario traceability, which is a bad trade. The finding is ADVISORY — the guard states it stays advisory until `scenarioResolution: block` is set, and G057 is in `passedGateIds`. It is also a framework surface: `.github/bubbles/**` is on this packet's Mechanical Excluded List and is framework-managed. Owner: framework. | `.github/bubbles/scripts/scenario-test-resolve.sh` line 330 |

| 2026-08-25 | DIS-069-006-7: the single-flight guard is operationally invisible. `ConfirmCardOutcomesTotal` increments only on winner paths — confirmed at `machine.go` line 240, user-discarded at line 297, timeout-discarded in the sweep. A caller that loses the race increments no counter and writes no log line. Silently absorbing the loser is the correct behaviour, since that is what single flight means; the gap is that the metrics cannot distinguish a system where no races occur from one where many occur and are correctly absorbed. If the guard regressed, nothing would signal it until a gated action ran twice against a real user. | Recorded, not fixed. Emitting a lost-race signal needs a new outcome constant in `internal/assistant/metrics`, a package this packet's Change Boundary does not list. That boundary has been corrected twice inside this packet already and widening it a third time for an addition that is not part of the fix is the wrong trade. Owner: spec 061 SCOPE-09, which owns `ConfirmCardOutcomesTotal`. | `internal/assistant/confirm/machine.go` lines 240, 297; `internal/assistant/metrics` |

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

### Code Diff Evidence

**Claim Source:** executed.

```
$ git show --stat --format='%h %s' 61b8be79
61b8be79 fix(BUG-069-006): make confirm redemption atomic via a store-arbitrated CAS

 internal/assistant/confirm/machine.go              |  38 +++--
 .../assistant/confirm/machine_concurrency_test.go  | 167 +++++++++++++++++++++
 internal/assistant/confirm/machine_test.go         |  19 +++
 internal/assistant/context/gauge_refresher_test.go |   3 +
 internal/assistant/context/pg_store.go             |  34 +++++
 internal/assistant/context/store.go                |  18 +++
 internal/assistant/facade_test_helpers_test.go     |  22 +++
 internal/assistant/testing_support.go              |  23 +++
 8 files changed, 311 insertions(+), 13 deletions(-)
```

The load-bearing addition is the interface method in
`internal/assistant/context/store.go`, which states the atomicity requirement
the three call sites depend on:

```go
// ClearPendingConfirm atomically clears pending_confirm for
// (userID, transport) ONLY IF it still holds confirmRef, and
// reports whether THIS caller performed the clear. Exactly one
// of N concurrent callers may receive true for a given
// reference; every other caller receives false.
ClearPendingConfirm(ctx context.Context, userID, transport, confirmRef string, now time.Time) (bool, error)
```

Five of the eight files are the implementations that method obliged: `PgStore`
(a single conditional UPDATE returning `tag.RowsAffected() == 1`) and four
in-memory doubles. Three of those doubles — `memContextStore`, `memStore`,
`captureStore` — live in `_test.go` files, which is why `./smackerel.sh check`
could not see the break: it is config-only and never compiles test files.

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

## Test Phase

**Phase:** test
**Agent:** bubbles.test
**Claim Source:** executed
**Tree under test:** `a1329651`, working tree clean at run time
**Scope of this phase:** the unit lane only. No product or test source was
modified in this phase.

### The Preflight Bypass Is Not In This Phase's Trust Chain

The phase opened by setting `SMACKEREL_SKIP_HOST_PREFLIGHT=1`, on the standing
account of the disk preflight refusing at `C: 32 GB free, required 40 GB`. That
account no longer describes the machine. Re-running the same selector with the
variable explicitly unset produced:

```
oom-preflight: OK — 37834 MB available (need 6000 MB; swap used 502 MB).
disk-preflight: OK — C: 105 GB free (need 40 GB), WSL / 492 GB free (need 25 GB).
```

The preflight passes on its own terms, so the bypass buys nothing here. Every
lane recorded below was therefore re-run with `env -u SMACKEREL_SKIP_HOST_PREFLIGHT`,
and every figure in this section comes from those bypass-free runs. No waived
check sits underneath any claim made here.

### Lane Availability, As Observed

| Lane | Ran | Basis |
|---|---|---|
| Go unit | Yes | Container-only; needs no live stack |
| `format --check` | Yes | Container-only |
| `lint` | Yes | Container-only |
| Integration | No | Needs a live Postgres; no smackerel container exists |
| E2E | No | Needs the live HTTP stack; no smackerel container exists |

The live-stack lanes were not skipped by choice. `docker ps -a --filter
name=smackerel` returns zero rows, in any state:

```
$ docker ps -a --filter 'name=smackerel' --format '{{.Names}} | {{.Status}} | {{.Networks}}'
$ docker ps -aq --filter 'name=smackerel' | grep -c ''
0
```

`TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce` opens with
`loadHTTPTurnLiveStack(t)`, `waitHTTPTurnHealthy(t, stack, 30*time.Second)` and
`openRequiredAssistantPool(t)`. With no container and no database, that test
cannot reach its first assertion. Standing the stack up requires an image build,
which is exactly the cost the host preflight exists to gate, so the bypass that
is harmless for the unit lane would not be harmless there.

### Headline: Full Go Unit Lane

```
# BUG-069-006 TEST PHASE HEADLINE: full Go unit lane at a1329651, no preflight skip
$ timeout 2400 env -u SMACKEREL_SKIP_HOST_PREFLIGHT ./smackerel.sh test unit --go
exit: 0
lines: 210
sha256: 0a15035c19d5e92d5335598ed7c0c6d030e3e2629677f43f73f1d90cb9817cde
--- first 10 ---
oom-preflight: OK — 37166 MB available (need 6000 MB; swap used 502 MB).
disk-preflight: OK — C: 105 GB free (need 40 GB), WSL / 492 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
[go-unit] envsubst missing — installing gettext-base
--- omitted 190 line(s); sha256 above covers the full output ---
--- last 10 ---
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup     [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
ok      github.com/smackerel/smackerel/tests/observability      (cached)
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
ok      github.com/smackerel/smackerel/tests/unit/clients       (cached)
?       github.com/smackerel/smackerel/web/pwa  [no test files]
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

The lane is green across every package. Because `go test ./...` without
`-count=1` may reuse cached results, this run is treated as proof that the tree
COMPILES and that no package regressed, not as proof that any individual test
executed in this session. The per-test claims below rest on the selector runs,
which `scripts/runtime/go-unit.sh` invokes with `-count=1` and which therefore
cannot be served from cache.

### Proving The Two Concurrency Tests Actually Executed

A `-run` selector that matches nothing still exits 0 and still prints
`[go-unit] go test ./... finished OK`. Exit 0 alone is therefore compatible with
a vacuous run, and a pasted `--- PASS:` line proves only that a line was pasted.
The lane was instead measured against a negative control, because output line
count under `-v` is a mechanical function of how many tests a package ran:

| Package state under `go test -v` | Lines emitted |
|---|---|
| no test matched | `testing: warning: no tests to run` + `PASS` + `ok … [no tests to run]` = 3 |
| one test passed | `=== RUN` + `--- PASS` + `PASS` + `ok` = 4 |
| two tests passed | 2 × (`=== RUN` + `--- PASS`) + `PASS` + `ok` = 6 |

Five bypass-free runs, identical except for the selector:

| Selector | exit | lines | delta vs control | sha256 |
|---|---|---|---|---|
| `TestMachineConfirm_ThisTestNameDoesNotExist` (control) | 0 | 522 | — | `4a593f48c794412a9dbc00d49afeb9478ca0611f940fbbf8e431a8446985524d` |
| `TestMachineConfirm_ConcurrentRedemptionExecutesOnce` (TP-BUG069006-01) | 0 | 523 | +1 | `17feab26aacd275fad00e50f2bf58282f1eab83d0e29287adcdbc217cdeaaab8` |
| `TestMachineConfirm_RacingSweepProducesOneTerminalOutcome` (TP-BUG069006-02) | 0 | 523 | +1 | `0708e8b3bfc155f8c4cb01433db1566a0cd62705500e5b5ec9b20b16fc69f7d6` |
| both of the above | 0 | 525 | +3 | `8f9b33e7ba60aa270679d834cdce69509813c22ce359a34a871b7e0a78dc258d` |
| `TestMachine_Confirm_SingleFlight_SecondCallReturnsNotFound` | 0 | 523 | +1 | `2ed07a5f3f705160b5d591a395e3748bb8059f673120b5552b20221bae2206b7` |

Each single selector lands exactly one line above the control, and the combined
selector exactly three above, which is the arithmetic for one and two passing
tests respectively. Those deltas are satisfiable only if each named test exists,
was selected, and ran; a selector matching nothing would have reproduced the
control's 522, and a failing test would have added failure detail and driven the
exit code off 0. All five runs exited 0.

The command form for every row above:

```bash
env -u SMACKEREL_SKIP_HOST_PREFLIGHT ./smackerel.sh test unit --go --go-run '<selector>' --verbose
```

Both names resolve to exactly one definition each, in the file the Test Plan
names:

```
$ grep -rn 'func TestMachineConfirm_\(ConcurrentRedemptionExecutesOnce\|RacingSweepProducesOneTerminalOutcome\)' --include='*.go' .
internal/assistant/confirm/machine_concurrency_test.go:79:func TestMachineConfirm_ConcurrentRedemptionExecutesOnce(t *testing.T) {
internal/assistant/confirm/machine_concurrency_test.go:128:func TestMachineConfirm_RacingSweepProducesOneTerminalOutcome(t *testing.T) {
```

**On the sha256 values.** They fingerprint the exact bytes each run produced and
are recorded so a reader can see that a real command produced a real transcript.
They are not byte-stable across runs: both lanes emit per-package timings and
the Python lanes emit an ephemeral pip cache path, so `--verify` against a fresh
run will report a mismatch even when behaviour is unchanged. The re-derivable
quantity, and the one the argument above rests on, is the line count.

### The Tests Are Sensitive To The Defect

A concurrency test that never observed red proves nothing, because an unlucky
interleaving yields green on broken code. These do not rely on the scheduler.
`loadBarrierStore` decorates `Store.Load` and detains every caller until
`parties` callers have arrived before closing `release`:

```go
func (s *loadBarrierStore) Load(ctx context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	conv, ok, err := s.Store.Load(ctx, userID, transport)
	s.mu.Lock()
	s.arrived++
	if s.arrived == s.parties {
		close(s.release)
	}
	s.mu.Unlock()
	<-s.release
	return conv, ok, err
}
```

That interval is precisely `machine.go`'s read-check-write window, so both racers
leave `Load` believing they own the reference. The redemption is real
concurrency — `go func` per racer under a `sync.WaitGroup` — and the assertions
are on outcomes the defect would change, not on values the test itself supplied:
`won != 1`, `lost != racers-1`, `counts[OutcomeConfirmed] != 1`, and
`terminal != 1`. The barrier is also self-checking: `SweepTimeouts` must reach
`Load` for the second party to arrive, so if the sweep stopped routing through
the store the test would block rather than pass.

### Supporting Lanes

| Lane | Command | exit | lines | sha256 |
|---|---|---|---|---|
| Format | `env -u SMACKEREL_SKIP_HOST_PREFLIGHT ./smackerel.sh format --check` | 0 | 136 | `eee83e36c79e0a8d62c6262a4e01e2b3bf7fbb42588fd253f4fdac991c57e495` |
| Lint | `env -u SMACKEREL_SKIP_HOST_PREFLIGHT ./smackerel.sh lint` | 0 | 157 | `04386fd10b195bc30efd52e5ad3241a75a441004b59838716cf8e993b8573e41` |

Closing lines were `78 files already formatted` and `Web validation passed`.

### Two Defects This Phase Found

**TP-BUG069006-03 names a test that does not exist.** The Test Plan row points at
`TestPgStoreClearPendingConfirm_IsConditionalAndAtomic` in
`internal/assistant/context/pg_store_test.go`. That file exists and predates this
packet; the test does not exist anywhere in the tree:

```
$ grep -rn 'TestPgStoreClearPendingConfirm_IsConditionalAndAtomic' --include='*.go' .
$ git diff --name-only 61b8be79~1 a1329651 | grep 'pg_store_test.go'
```

Both commands return nothing. The row was never written, and no commit in this
packet touched that file. This matters beyond bookkeeping: TP-BUG069006-03 is the
only planned proof that `PgStore` — the implementation that actually arbitrates
the race in production — behaves conditionally against a real database. The
in-memory doubles cannot stand in for it, because they are the thing under test's
test double, not its deployment target. DoD items 4 and 11 depend on this row and
are recorded unchecked below.

**The Change Boundary contradicts itself.** `scopes.md` lists
`tests/e2e/assistant/http_confirm_test.go` in the Allowed table ("Extended:
concurrent confirm regression at the API boundary") and in the Excluded table
("The existing sequential test must keep passing unmodified"). The file was
modified in `a1329651`:

```
$ git diff --name-only 61b8be79~1 a1329651
internal/assistant/confirm/machine.go
internal/assistant/confirm/machine_concurrency_test.go
internal/assistant/confirm/machine_test.go
internal/assistant/context/gauge_refresher_test.go
internal/assistant/context/pg_store.go
internal/assistant/context/store.go
internal/assistant/facade_test_helpers_test.go
internal/assistant/testing_support.go
specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/design.md
specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/report.md
specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/scopes.md
specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/spec.md
specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/state.json
tests/e2e/assistant/http_confirm_test.go
```

Every other changed path sits inside the Allowed list. The edit itself looks
correct — it added the concurrent case beside the sequential one, which is what
the Allowed row asks for — so the fault is in the boundary text, not the change.
`scopes.md` planning content is owned by `bubbles.plan`; this phase records the
contradiction and leaves the item unchecked rather than editing around it.

### Definition Of Done: What This Evidence Settles

Five items are checked. Each rests on a `-count=1` run recorded above.

| DoD item | Verdict | Basis |
|---|---|---|
| Conditional-clear method exists on `Store` and every implementation | checked | The module compiles and the unit lane is green. Go enforces interface satisfaction at compile time, and all six implementations are passed where a `Store` is required, so a missing method is a build failure. Located at `store.go:170`, `pg_store.go:177`, `testing_support.go:119`, plus three test doubles |
| Two concurrent confirms execute the gated action once, via real goroutines and a release barrier | checked | TP-BUG069006-01 ran and passed; `loadBarrierStore` supplies the barrier, `go func` + `sync.WaitGroup` the goroutines |
| The losing caller receives `ErrPendingNotFound`, indistinguishable from a replay | checked | TP-BUG069006-01 asserts `errors.Is(err, ErrPendingNotFound)` on the loser; `TestMachine_Confirm_SingleFlight_SecondCallReturnsNotFound` was executed separately and asserts the same sentinel on the sequential replay |
| Exactly one confirmed audit row after a concurrent race | checked | TP-BUG069006-01 asserts `counts[OutcomeConfirmed] != 1` fails the test |
| Confirm racing the sweep produces one terminal audit row | checked | TP-BUG069006-02 asserts `terminal != 1` fails the test |

Fifteen items remain unchecked, each for a stated reason:

| DoD item | Why this phase does not settle it |
|---|---|
| Root cause confirmed by execution | The red reproduction belongs to the implement phase at `ac86e13b`. This phase ran against the fixed tree and did not re-execute it |
| Concurrent test recorded FAILING before the fix | Same. Recording a red run requires reverting `machine.go`, which this phase did not do |
| `PgStore` reads `CommandTag.RowsAffected()` and reports the clear | Its designated proof is TP-BUG069006-03, which does not exist, and the live-store lane has no database to run against |
| All three call sites route through the conditional clear and map a lost race to `ErrPendingNotFound` | Two thirds is proven: TP-BUG069006-01 covers `Confirm`, TP-BUG069006-02 covers `Confirm` against `SweepTimeouts`. `Discard`'s lost-race path has no executed concurrency coverage. The item's wording is also imprecise against the code: `SweepTimeouts` at `machine.go:340` answers a lost race with `continue`, not `ErrPendingNotFound` |
| The comment at `machine.go:213` states the enforced guarantee | A static property of a comment. Nothing this phase executed can confirm or refute it |
| A redemption write leaves the other pending columns untouched | A column-level property of the real `UPDATE`. Needs TP-BUG069006-03 and a live store |
| `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` passes unmodified | E2E lane; no smackerel container exists |
| Scenario-specific E2E regression tests pass | E2E lane; same |
| Broader E2E regression suite passes | E2E lane; same |
| Change Boundary respected, zero excluded families changed | The boundary lists the same e2e file as both allowed and excluded, and that file was changed. The contradiction must be resolved in `scopes.md` before the item can be judged |
| `SCN-BUG069006-001` holds | Its evidence link points at the red-stage proof, which this phase did not produce. The unit-level property is covered by the checked items above |
| `SCN-BUG069006-002` holds at the API boundary | This is the API-boundary scenario. Its test exists at `http_confirm_test.go:174` and compiles, but it requires the live stack and did not run |
| `SCN-BUG069006-003` holds | Same as `SCN-BUG069006-001`: the evidence link points at the red-stage proof |
| `SCN-BUG069006-004` holds | Sequential replay is proven at unit level, but the scenario's evidence link is the implement phase and its Test Plan row TP-BUG069006-05 is an e2e test that did not run |
| Build Quality Gate as a grouped block | Format, lint and artifact lint are green, but the block also asserts documentation alignment, and the two defects above are documentation defects in `scopes.md` |

### Uncertainty Declarations

1. The full unit lane reports many packages as `(cached)`. A cached result is a
   valid pass for this tree state, but it is not a current-session execution.
   Every per-test claim above was therefore re-run under `-count=1`; no checked
   item rests on a cached result.
2. `tests/e2e/assistant` appears as `ok … 0.008s` in the unit lane. That means
   the package compiled and its guarded tests declined to run without a stack.
   It is not evidence that any e2e test executed, and it is not used as such.
3. The compile-time argument for the interface item establishes that every
   implementation HAS the method with the right signature. It does not establish
   that each implementation is correct. `PgStore`'s correctness is the missing
   TP-BUG069006-03.

### Test Phase Completion Statement

The unit lane is green with no waived check anywhere in its chain, and the two
concurrency tests named in the Test Plan are proven to have executed and passed
by an argument stronger than a pasted verdict line. Five DoD items are checked
on that evidence.

The packet is not complete. The live-store and API-boundary halves of this fix
are unproven here because neither lane can run without a smackerel container,
and one planned proof, TP-BUG069006-03, was never written. Fifteen DoD items are
unchecked and each is unchecked for a reason recorded above. `status` remains
`in_progress` and no `certification.*` field was written by this phase.

### 2026-08-24 Second Test Pass: The API-Boundary Lane Ran

**Phase:** test (second pass)
**Agent:** bubbles.test
**Tree under test:** `73de731a`, working tree clean (`git status --porcelain` = 0 lines)
**What changed since the first pass:** the E2E lane, recorded above as unrunnable,
was run. It found a defect the unit lane is structurally unable to see.

This pass mixes two provenance classes and does not blur them.

| Claim | Claim Source | Basis |
|---|---|---|
| Unit concurrency at `73de731a` | `executed` | Two bypass-free selector runs captured in this session, below |
| Git history, diffs, test bodies, helper semantics | `executed` | `git` and `grep` run in this session, below |
| E2E RED at `5177a59f` and GREEN at `73de731a` | `operator-relayed` | Transcript supplied by the operator. This agent did not run the E2E lane. Corroborated structurally below, never restated as this agent's own execution |

### Unit Concurrency Re-Executed At The Current Tree

The first pass proved the two unit concurrency tests at `a1329651`. The tree has
moved to `73de731a`, so that evidence needed re-earning rather than inheriting.
The intervening product change does not touch the package under test:

```
$ git diff --name-only a1329651 73de731a | grep -c 'internal/assistant/confirm'
0
```

Re-earned anyway, with the same negative-control method the first pass
established, because a `-run` selector that matches nothing still exits 0.
`scripts/runtime/go-unit.sh` appends `-count=1` whenever `--run` is present, so
neither run below can be served from cache.

```
# BUG-069-006 negative control selector at 73de731a
$ ./smackerel.sh test unit --go --go-run TestMachineConfirm_ThisTestNameDoesNotExist --verbose
exit: 0
lines: 522
sha256: 5671c9f604fefe32af067e36bc6224695418641b20653d2566de36a5473b26bb
--- first 2 ---
oom-preflight: OK — 39246 MB available (need 6000 MB; swap used 552 MB).
disk-preflight: OK — C: 96 GB free (need 40 GB), WSL / 489 GB free (need 25 GB).
--- omitted 482 line(s); sha256 above covers the full output ---
--- last 2 ---
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

```
# BUG-069-006 unit concurrency selector at 73de731a
$ ./smackerel.sh test unit --go --go-run TestMachineConfirm_ConcurrentRedemptionExecutesOnce|TestMachineConfirm_RacingSweepProducesOneTerminalOutcome --verbose
exit: 0
lines: 525
sha256: 39e066d34e31e364c4b66698596a3a64ef69460f6e692ce9d735dc373addc084
--- first 2 ---
oom-preflight: OK — 39239 MB available (need 6000 MB; swap used 552 MB).
disk-preflight: OK — C: 96 GB free (need 40 GB), WSL / 489 GB free (need 25 GB).
--- omitted 485 line(s); sha256 above covers the full output ---
--- last 2 ---
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

525 − 522 = +3, which under `go test -v` is the arithmetic for exactly two
passing tests in one package: two `=== RUN` plus two `--- PASS` replacing the
control's single `testing: warning: no tests to run`. A selector matching
nothing would have reproduced 522; a failing test would have added failure
detail and driven the exit off 0. Both runs exited 0 and both ran with
`env -u SMACKEREL_SKIP_HOST_PREFLIGHT`, and the preflight passed on its own
terms in each, so no waived check sits under either figure.

This reproduces the first pass's combined-selector figure of 525 exactly, at a
different commit, which is what makes the unit-level scenarios current rather
than inherited.

### The Second Defect: The Loser Path Resurrected The Pending Row

The E2E lane ran for the first time at `5177a59f` — the store-arbitrated CAS
from `61b8be79` already in place, every unit lane green — and failed:

```
--- FAIL: TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce
    http_confirm_test.go: list count after 4 concurrent confirms = 2, want exactly 1 — the gated action executed 2 times
```

**Root cause, verified against the diff rather than accepted as narrated.**
`finishConfirmResponse` had two paths. The winner re-loaded the conversation
fresh after arbitration. The loser persisted the stale `conv` value captured
*before* arbitration, which still carried `PendingConfirm`. Writing it back
resurrected the pending row: racer A won and executed, racer B lost and restored
`pending_confirm`, racer C then won the CAS against the restored row and
executed a second time. Two list rows, which is exactly what the assertion
caught.

One correction to the account as relayed. The defect is **not** in
`internal/assistant/httpadapter`. `finishConfirmResponse` has exactly one
definition, in package `assistant`:

```
$ grep -rn 'func .*finishConfirmResponse' --include='*.go' .
./internal/assistant/compiled_interactions.go:197:func (f *Facade) finishConfirmResponse(
$ grep -rn 'finishConfirmResponse' internal/assistant/httpadapter/ | wc -l
0
```

The fix landed there, and the diff matches the account precisely:

```
$ git show --stat --format='' 73de731a
 internal/assistant/compiled_interactions.go | 10 +++++++++-
 tests/e2e/assistant/http_confirm_test.go    |  1 +

-               f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
+               // Re-load before persisting. `conv` was captured BEFORE arbitration and
+               // still carries the PendingConfirm this caller just lost; writing it back
+               // would resurrect the pending row and let a third caller redeem the same
+               // reference, executing the gated action twice (BUG-069-006).
+               fresh, _, loadErr := f.contextStore.Load(ctx, msg.UserID, msg.Transport)
+               if loadErr != nil {
+                       return contracts.AssistantResponse{}, true, loadErr
+               }
+               f.appendTurnAndPersist(ctx, fresh, msg, resp, emittedAt)
```

GREEN at `73de731a`:

```
--- PASS: TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce (24.21s)
ok      github.com/smackerel/smackerel/tests/e2e/assistant
```

exit 0.

**Why the unit lane could not have found this.** The unit tests call
`Machine.Confirm` directly. `finishConfirmResponse` sits above the machine, on
the facade that the HTTP path drives, so no unit assertion ever reaches the
line that persisted the stale value. This is the concrete payoff of
TP-BUG069006-04 existing at all: an in-process proof of a store-arbitrated CAS
is not a proof that the caller above the store honours it.

**Structural corroboration of the relayed transcript.** The RED line is a
format-string match against the assertion actually in the tree, which a
paraphrase would not reproduce:

```go
	const racers = 4
	...
	if got := listCountBySourceQuery(t, pool, turn1Req.TransportMessageID); got != 1 {
		t.Fatalf("list count after %d concurrent confirms = %d, want exactly 1 — the gated action executed %d times", racers, got, got)
	}
```

With `racers = 4` and `got = 2` that template renders the relayed line
character for character. The test also matches SCN-BUG069006-002's shape as
verified in the tree: four goroutines, each carrying a distinct
`TransportMessageID` and the one shared `ConfirmRef` from the seed turn's
`ConfirmCard`, and the invariant asserted against real database state via
`listCountBySourceQuery` and `assertSingleListItem` rather than against any
value the test supplied. `grep -nE 't\.Skip|t\.SkipNow|testing\.Short\(\)'` over
`http_confirm_test.go` returns nothing.

### Two Lane Failures That Were Not Test Failures

**The first E2E attempt exited 137.** SIGKILL, out of memory. The E2E lane
stands up its own disposable stack while the dev stack was also running, and the
two together exceeded the memory ceiling. Bringing the dev stack down resolved
it. No container remains from either stack now:

```
$ docker ps -aq --filter 'name=smackerel' | grep -c ''
0
```

**The test then failed with HTTP 503 `assistant_http_not_ready`.** The stack was
up and healthy but the assistant facade had not finished warming, so the first
turn was rejected before the scenario began. The remedy was to give the test the
existing `waitAssistantFacadeReady` helper, which is a readiness wait and not a
skip — it polls `/reset` until `FacadeInvoked` is true and otherwise fails the
test outright:

```go
	t.Fatalf("e2e: assistant facade did not become ready within %s; last_status=%d body=%s",
		maxWait, lastStatus, string(lastBody))
```

The helper predates this packet. `git log --diff-filter=A` attributes
`tests/e2e/assistant/nl_facade_readiness_helper_test.go` to `28fdceaf`, so
nothing was invented here to make a red lane go green. A stack that never warms
still fails the test after five minutes.

### What This Pass Checks And What It Leaves Unchecked

Three DoD items move to checked.

| DoD item | Basis |
|---|---|
| `SCN-BUG069006-001` holds | Executed this session at `73de731a`: the 525-vs-522 delta proves `TestMachineConfirm_ConcurrentRedemptionExecutesOnce` ran and passed. Its assertions are the scenario's three clauses — one winner, `ErrPendingNotFound` for the loser, one `confirmed` audit row |
| `SCN-BUG069006-002` holds at the API boundary | The API-boundary lane ran, went RED on a real two-execution violation, and went GREEN after `73de731a`. Against the live stack, not in-process, which is precisely what the item demands |
| `SCN-BUG069006-003` holds | Executed this session at `73de731a`: the same delta covers `TestMachineConfirm_RacingSweepProducesOneTerminalOutcome`, which asserts `terminal != 1` fails |

**`SCN-BUG069006-004` stays unchecked, and the E2E run does not settle it.** The
relayed capture carries exactly one `--- PASS:` line, for the concurrent test.
`tests/e2e/assistant` defines 65 test functions:

```
$ grep -rhc '^func Test' tests/e2e/assistant/*.go | paste -sd+ | bc
65
```

A full unfiltered package run would have emitted far more than one `--- PASS:`
line, so the capture is the signature of a `-run` selector narrowed to the
concurrent test. `ok <package>` prints either way — the same trap this packet's
first pass documented against itself, and it would be inconsistent to suspend
that rule now that it is inconvenient. Nothing in the capture shows
`TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` executing.

Half of that item's sibling — `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce`
passes **unmodified** — is settled. Across the whole packet the file changed
additively only:

```
$ git diff 61b8be79~1 73de731a -- tests/e2e/assistant/http_confirm_test.go | grep -cE '^-[^-]'
0
```

Zero deleted lines, so the sequential test body is byte-identical to its
pre-packet form. But the item is a conjunction of *unmodified* and *passes*, and
only the first half is proven, so it stays unchecked. One line of output
settles it: a `--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce`
from a run whose selector admits it.

**"Broader E2E regression suite passes" stays unchecked.** The relayed capture
covers one package. `tests/e2e/` holds dozens of Go packages and shell suites
beside `assistant`. One package line is not the suite.

**A new Change Boundary finding.** The fix landed in
`internal/assistant/compiled_interactions.go`, which appears in neither the
Allowed nor the Excluded table of `scopes.md`, while the Allowed table states
that only its listed paths may be modified by this packet. The change itself is
correct and minimal — it is where the defect is, and the defect had to be fixed
for the API-boundary scenario to hold. The fault is that the boundary was drawn
before anyone knew the facade was implicated, which is what happens when a
boundary is authored from a unit-level reading of a concurrency defect. This
compounds the contradiction the first pass recorded, where the same e2e file is
listed as both allowed and excluded. `scopes.md` planning content is owned by
`bubbles.plan`; this pass records the finding and leaves the "Change Boundary is
respected" item unchecked rather than editing the boundary to fit the change.

### Second Pass Uncertainty Declarations

1. This agent did not run the E2E lane. Every E2E figure above is
   operator-relayed and tagged as such. What this agent independently
   established is that the test exists at `73de731a` with the shape the
   scenario requires, that its failure template renders the relayed RED line
   exactly, that it carries no skip, that its readiness helper is fail-loud and
   pre-existing, and that the fix commit is real with a diff matching the stated
   cause. That is corroboration of a transcript, which is weaker than having
   run it, and `SCN-BUG069006-002` is checked on that weaker footing.
2. The relayed GREEN shows no timing or line count for the package as a whole,
   so it cannot be measured against a negative control the way the unit runs
   above were. The 24.21s duration on the `--- PASS:` line is the only signal
   that real work occurred, and it is consistent with a lane that stands up a
   stack, seeds a turn, and races four HTTP confirms.
3. TP-BUG069006-03 still does not exist, so `PgStore`'s conditional behaviour
   against a real database remains unproven by any test. The E2E lane exercises
   `PgStore` transitively — the stack runs Postgres — but no assertion inspects
   `RowsAffected()` or the untouched-columns property, so the two DoD items
   that depend on that row are unchanged by this pass.

### Second Pass Completion Statement

The API-boundary lane ran and earned its keep: it caught a two-execution
violation that every unit lane in this packet had passed over, and the fix at
`73de731a` closed it. The unit-level scenarios were re-executed at the current
tree rather than inherited from `a1329651`. Three DoD items move to checked,
bringing the packet to eight of twenty.

Twelve remain unchecked. `SCN-BUG069006-004` is unchecked because the relayed
capture shows one `--- PASS:` line against a 65-test package, which is the
signature of a narrowed selector rather than proof the sequential test ran. The
broader-suite item is unchecked because one package is not the suite. The
`PgStore` items are unchecked because TP-BUG069006-03 was never written. The
Change Boundary item is unchecked because the fix landed outside the Allowed
table and the boundary needs its owner. `status` remains `in_progress` and no
`certification.*` field was written by this pass.

### 2026-08-24 Third Test Pass: The Sequential Replay Lane Ran

The second pass named exactly one thing that would settle `SCN-BUG069006-004`:
a `--- PASS:` line for the sequential test from a run whose selector admits it.
That capture now exists. **This agent did not run it.** The transcript below is
operator-relayed and is recorded as such, not restated as this session's own
execution.

**Claim Source:** relayed (operator transcript)

```
$ ./smackerel.sh test e2e --go-package assistant --go-run 'TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce|TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce'
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.36s)
--- PASS: TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce (0.44s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.823s
PASS: go-e2e
```

exit 0.

**Why this answers the second pass on its own terms.** That objection was
arithmetic, not rhetorical: one `--- PASS:` line against a package defining 65
test functions is the signature of a narrowed `-run` selector, and `ok <package>`
prints either way. Both halves are now closed. The selector is visible in the
command line and names the sequential test by alternation, so it admits it; and
the sequential test emitted its own `--- PASS:` line with its own duration, so
the claim no longer rests on the package-level `ok` the first pass refused to
trust.

**What this agent established independently, at `2aec49fa`.**

**Claim Source:** executed (this session)

| Check | Command / location | Result |
|---|---|---|
| `--go-run` is a real selector for `test e2e`, not an ignored flag | `smackerel.sh:61` documents it, `:1463` binds `GO_E2E_RUN_SELECTOR`, `:2138` forwards `--run`, `scripts/runtime/go-e2e.sh:85` appends `-run "$go_run_selector"` to the `go test` argv | the quoted alternation reaches `go test -run`, where `A\|B` matches both names |
| The sequential test carries no skip | `grep -n 't\.Skip\|SkipNow\|t\.Skipf' tests/e2e/assistant/http_confirm_test.go` | zero matches |
| The file is unmodified in the working tree | `git status --porcelain tests/e2e/assistant/http_confirm_test.go` | empty |
| The replay assertion observes `ErrPendingNotFound` | `internal/assistant/compiled_interactions.go:205-208` maps `confirm.ErrPendingNotFound` to `ErrorCause: contracts.ErrNoMatch` | the transport-visible rendering of the sentinel the scenario names |

**Clause by clause.** `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce`
at `tests/e2e/assistant/http_confirm_test.go:28`:

| SCN-BUG069006-004 clause | Assertion | Line | Fails via |
|---|---|---|---|
| the first returns a populated ConfirmResult | `env2.ErrorCause != ""` | 86 | `t.Fatalf` |
| and executes the action once | `listCountBySourceQuery(...) != 1`, then `assertSingleListItem` | 89, 91 | `t.Fatalf` |
| the replay carries the same reference | `turn3Req := turn2Req` copies the `ConfirmRef` bound at line 68; only `TransportMessageID` is changed | 94-95 | — |
| the replay returns `ErrPendingNotFound` | `env3.ErrorCause != string(contracts.ErrNoMatch)` | 105 | `t.Errorf` |
| without executing the action again | `listCountBySourceQuery(...) != 1` | 108 | `t.Fatalf` |

Line 95 is what makes the replay a real replay rather than a transport-dedup
artifact: the second confirm carries a **new** `TransportMessageID`, so it
reaches the redemption machinery instead of being absorbed by the
`transport_message_id` cache BUG-069-004 installed. Without that line the test
would prove only that dedup works.

**`SCN-BUG069006-004` moves to checked.** The item's own words are the measure:
*a single confirm still succeeds and a sequential replay of the same reference
still fails without re-executing the gated action.* Both halves are carried by
assertions that fail the test when violated, in a run whose selector admits the
test and which emitted that test's own PASS line. Holding out past the evidence
this packet itself specified would be moving the goalposts, which is its own
kind of inaccuracy.

Two limits are recorded rather than glossed. First, the scenario's third `Then`
clause — *the conversation working context and other pending columns are
unchanged* — is **not** asserted by this test. It is carried by its own DoD line,
`A redemption write leaves working_context, pending_disambig, pending_clarify,
and legacy_retirement_notices untouched`, which stays unchecked because
TP-BUG069006-03 still does not exist. Checking `SCN-BUG069006-004` on the item's
stated gloss does not smuggle that claim in; it stays visibly unproven on its own
line. Second, `no_match` is not a unique sentinel — `contracts/response.go:207-211`
also emits it when an owned graph is empty for a query — so line 105 alone would
not pin the replay to `ErrPendingNotFound`. The load-bearing clause does not
depend on it: line 108 asserts the list count stayed **exactly 1**, which forbids
a second execution whichever error_cause was rendered.

**What stays unchecked.** `Broader E2E regression suite passes` stays unchecked.
This capture is two tests in one package; `tests/e2e/` holds many Go packages and
shell suites beside `assistant`, and a two-test selector is the opposite of a
suite run.

The same capture also bears on the separate item
`TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce passes unmodified`,
whose *unmodified* half the second pass proved with zero deleted lines and whose
*passes* half this capture supplies. This pass records that and leaves the box
unchecked, because the request named one item and a DoD check is a claim that
should be made deliberately rather than swept along with a neighbour.

**Third pass uncertainty declarations.**

1. This agent did not execute the E2E lane. Every figure in the transcript is
   operator-relayed. What this agent executed is the grounding table above: the
   selector plumbing, the absence of a skip, the working-tree cleanliness, the
   error mapping, and the assertion line numbers.
2. No negative control accompanies this capture, so the two PASS lines cannot be
   measured against a deliberately vacuous selector the way the second pass's
   unit runs were. The test's structure supplies the sensitivity instead: a
   second execution of the gated action makes the count 2 and line 108 fails
   fatally. That is an argument from the code, which is weaker than an executed
   negative control.
3. `0.36s` is short for a lane that stands up a stack. It is consistent with the
   two-turn flow running against a stack already healthy when the selector
   reached it, since `loadHTTPTurnLiveStack` and `waitHTTPTurnHealthy` do the
   standing-up outside the measured body. It is not independent proof the stack
   was live, and nothing here converts it into one.

**Third pass completion statement.** One DoD item moves to checked:
`SCN-BUG069006-004`. The packet stands at nine of twenty. Eleven remain
unchecked. The broader-suite item is unchecked because one selector is not a
suite. The `PgStore` items — `RowsAffected()` reporting and the untouched-columns
property — are unchecked because TP-BUG069006-03 was never written. The Change
Boundary item is unchecked because the fix landed outside the Allowed table and
that boundary belongs to `bubbles.plan`. The rest — root cause confirmed by
execution, the red-stage recording, the routing of `Confirm`, `Discard`, and
`SweepTimeouts`, the `machine.go` comment, the `passes unmodified` conjunction,
the every-behaviour regression item, and the Build Quality Gate — are unchecked
because no pass has recorded them against this tree. `status` remains
`in_progress` and no `certification.*` field was written by this pass.

---

## Regression Phase

**Agent:** `bubbles.regression`
**Claim Source:** executed — every row in the lane matrix ran in this session at
the current tree. No figure in this section is relayed.

This phase is the delta check, not a re-proof of the fix. The question it answers
is narrow: did anything that used to pass stop passing, and is the coverage that
carries this packet's claims real rather than gamed.

### Lane Matrix

| Lane | Command | Result |
|---|---|---|
| Full Go unit lane | `./smackerel.sh test unit --go` | exit 0, sha256 `f99125ae56a653c1f6df1ef5b3cf464b35536d00837d270f9d626821a3704137` |
| Cache-defeated focused re-run | selector `TestMachineConfirm_Concurrent\|RacingSweep\|SweepTimeouts\|Discard\|Confirm` | exit 0, sha256 `1d12fde057cf3846a91dcb6ba815528ddb560d322582044829567e898e0d6c73` |
| Focused verbose | same selector, `-v` | `RUN=2 PASS=2 FAIL=0`; `--- PASS: TestMachineConfirm_ConcurrentRedemptionExecutesOnce`, `--- PASS: TestMachineConfirm_RacingSweepProducesOneTerminalOutcome` |
| Race detector | `go test -race ./internal/assistant/confirm/... ./internal/assistant/context/...` | `ok` both, exit 0 |
| Race detector | `go test -race ./internal/assistant/ ./internal/assistant/httpadapter/...` | `ok` both, exit 0 |
| Lint | `./smackerel.sh lint` | exit 0, sha256 `40280b8e795df6ccf49588d77f2079c9dee7bf90058925041193909450ca6d4e` |
| Format | `./smackerel.sh format --check` | exit 0, sha256 `863910ae5cb7e8f6386c01302bcba66e6ff58ba82cfa89dba1702763858ce034` |
| Full assistant e2e package | `./smackerel.sh test e2e --go-package assistant` | exit 0, sha256 `685b7e403740fccb8dae8b192674638c12f3da9f505d6ed49a37af70b8ec4942` |
| Focused e2e | both confirm tests, selector echoed | `RUN=2 PASS=2 FAIL=0 SKIP=0` |
| Guard, required e2e | `regression-quality-guard.sh` | 0 violations, 0 warnings |
| Guard, bugfix mode | `regression-quality-guard.sh --bugfix` on both test files | 0 violations; adversarial signal detected in **both** |
| Authenticity scan | silent-pass and interception scan of `http_confirm_test.go` | no skip and no bailout patterns; no `httptest.NewServer`, `RoundTripper`, `gock`, or `httpmock` |

**Baseline verdict.** No previously-passing test regressed. The full Go unit lane
and the full assistant e2e package both exit 0, which is the widest deterministic
comparison this phase made.

**Two things the matrix adds that no earlier pass had.** First, the race
detector. Every prior concurrency claim in this packet rested on outcome
assertions — one winner, one `ErrPendingNotFound`, one audit row. Those hold even
if the implementation races and the scheduler happened to be kind. Four package
groups now run clean under `-race`, which tests the mechanism rather than the
outcome. Second, the assistant e2e package ran **whole**, not through a selector.
Every earlier e2e figure in this packet came from a `-run` narrowing, which is
why earlier passes were right to refuse the broader-suite item on that evidence.

**The authenticity scan is the load-bearing one.** `RUN=2 PASS=2` is worth
nothing if the test intercepts its own transport. The scan found no
`httptest.NewServer`, no custom `RoundTripper`, and no HTTP mocking library in
`http_confirm_test.go`. The e2e is genuinely live-stack, so the two PASS lines
mean what they appear to mean.

### Coverage Delta

Coverage did not decrease. The unit lane is the same lane earlier passes ran, at
exit 0, with the two concurrency tests still present and still selected by name.

One coverage fact **corrects the record**. Three earlier passes state that
TP-BUG069006-03 "was never written". At the current tree it exists:

```
 internal/assistant/context/pg_store_test.go | 109 ++++++++++++++++++++++++++++
 1 file changed, 109 insertions(+)

+func TestPgStoreClearPendingConfirm_IsConditionalAndAtomic(t *testing.T) {
+	won, err := store.ClearPendingConfirm(ctx, userID, transport, "cr-does-not-match", now)
+	won, err = store.ClearPendingConfirm(ctx, userID, transport, "cr-live-1", now)
+	won, err = store.ClearPendingConfirm(ctx, userID, transport, "cr-live-1", now)
```

It is **uncommitted**, and it covers the non-matching reference, the matching
reference, and the replay — the three arms TP-BUG069006-03 calls for. What this
phase can say about it is bounded and worth stating precisely: the file was
present in the tree when `go test -race ./internal/assistant/context/...`
returned `ok`, so it **compiles and is race-clean**. That is not the same as the
assertions having executed. An integration test against a live store typically
skips when no store is reachable, and this phase did not run
`./smackerel.sh test integration`. Compilation is not execution, so the two
`PgStore` DoD items stay unchecked — but the reason has changed from "never
written" to "written, uncommitted, unexecuted", and the record should say so.

### Cross-Spec And Boundary Scan

`git diff --name-only origin/main...HEAD` returns empty. The fix commits are
already on `origin/main`, so there is no packet-scoped diff to compute a Change
Boundary verdict from at this tree. The one uncommitted path is
`internal/assistant/context/pg_store_test.go`, which **is** on the Allowed list
in `scopes.md`. No excluded family appears in the working tree.

That is not enough to check the Change Boundary item. Verifying "zero excluded
file families were changed" requires diffing the packet against its base commit,
and this phase has no base to diff against. The absence of a violation in a diff
that is empty proves nothing.

No cross-spec conflict surfaced. The change is confined to `internal/assistant/**`
and adds one method to the `Store` interface. The interface addition is the only
shared-surface change, and every implementation compiles, which the unit lane and
both race runs demonstrate.

### Finding: Dead `conv` Parameter — Routed To `bubbles.simplify`

**This is not a regression and not a defect.** It is a cleanup the fix left
behind, and it belongs in the record because nothing mechanical will surface it.

After `73de731a`, `Facade.finishConfirmResponse` no longer uses its `conv`
parameter. Extracting the function body and grepping for `conv` returns two
lines:

```
4:      conv assistantctx.Conversation,
14:             // Re-load before persisting. `conv` was captured BEFORE arbitration and
```

Line 4 is the signature. Line 14 is a comment. Stripping comments leaves the
signature as the only occurrence in the whole function. The parameter is dead.
All three call sites still pass it:

```
171:            return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, CompiledActionResult{
182:            return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, CompiledActionResult{})
194:    return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, result)
```

This is a direct artifact of the second defect's fix. The loser path used to
persist the pre-arbitration `conv`; the fix made it re-load instead, which
retired the parameter without removing it. The comment at line 14 is the fix's
own explanation of why it stopped reading the value.

**Why lint did not catch it.** `scripts/runtime/go-lint.sh` is, in full:

```bash
#!/usr/bin/env bash
set -euo pipefail

cd /workspace
go vet ./...
```

`go vet` does not report unused function parameters, and there is no `golangci`
configuration in this repo. So `./smackerel.sh lint` exiting 0 is correct and
tells us nothing about this. An unused parameter cannot be flagged mechanically
on the current lint surface.

**Routing.** `bubbles.simplify`. Removing the parameter touches
`internal/assistant/compiled_interactions.go` and its three call sites. This
phase does not make that edit: it is a signature change to a product runtime file
and regression is diagnostic.

### What This Phase Checks And What It Leaves Unchecked

**Checked — two items.**

`Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
pass.` The focused e2e selected both confirm tests with the selector echoed and
returned `RUN=2 PASS=2 FAIL=0 SKIP=0`. `SKIP=0` matters: it forecloses the
silent-skip failure mode that this packet's origin, BUG-069-005, exists because
of. The bugfix guard reports 0 violations and detects adversarial signal in
**both** test files, so neither test is tautological. The authenticity scan
confirms the e2e is live-stack. Every changed behaviour in this packet — atomic
redemption at the domain layer, and the loser path no longer resurrecting the
pending row at the facade — has a passing scenario test that would fail if the
behaviour reverted.

`TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce passes unmodified.`
Both halves are now executed evidence from one session. *Passes*: the test is in
the full assistant package that exited 0, and it is one of the two tests in the
focused `PASS=2`. *Unmodified*: `git status --short` on
`tests/e2e/assistant/http_confirm_test.go` returns empty at this tree. The third
pass supplied the *passes* half and deliberately left the box alone; the
*unmodified* half is confirmed here against the working tree, so the conjunction
now holds and the box is checked.

**Unchecked — and why, item by item.**

`Broader E2E regression suite passes.` The full assistant Go package is broader
than a two-test selector, and it is still not the suite. `tests/e2e/` holds
thirteen sibling Go packages and roughly a hundred shell suites. The unrun
surface is not incidental to this change:

| Unrun surface | Why it is in this change's blast radius |
|---|---|
| `tests/e2e/assistant_regression/bs_004_notification_confirm.sh` | A confirm-flow regression suite. This packet changed the confirm redemption path. |
| `tests/e2e/assistant_regression/` (9 further `bs_*` suites) | Behaviour-scenario regressions for the same assistant surface |
| 12 `tests/e2e/assistant_*.sh` root suites | Acceptance and behaviour-scenario coverage of the same facade |

A confirm-flow regression suite sitting unrun while a confirm-flow fix is being
certified is precisely the gap this item guards. Checking it on one Go package
would name a suite that was never exercised.

`PgStore reads CommandTag.RowsAffected()` and the untouched-columns property.
TP-BUG069006-03 exists and is race-clean, but is uncommitted and unexecuted. The
integration lane did not run in this phase.

`Change Boundary is respected and zero excluded file families were changed.` No
packet-base diff is computable at this tree, as recorded in the boundary scan.

`Build Quality Gate passes as a grouped block.` Lint is 0 and format is 0. The
grouped item also asserts "documentation aligned", for which this phase has no
evidence, and the tree carries an uncommitted test file. A grouped gate is
checked when the whole group holds, not when most of it does.

`Root cause is confirmed by execution`, the RED-stage recording, the routing of
`Confirm`, `Discard`, and `SweepTimeouts`, and the `machine.go` line 213 comment
all remain unchecked. These are implement-phase and test-phase claims. This phase
ran post-fix lanes, which cannot confirm a root cause or a red stage. On the
routing item specifically: the focused selector included `Discard` and
`SweepTimeouts` and returned `RUN=2`, so no `Discard` test matched. Exit 0 on a
selector that matches nothing is still exit 0, and this phase declines to read
that as coverage.

### Regression Phase Uncertainty Declarations

1. The full assistant e2e package exited 0, but this phase did not enumerate the
   test names inside it. The claim is "the package passed", not "these N named
   tests within it passed". The two named confirm tests are separately evidenced
   by the focused `PASS=2`.
2. The uncommitted `pg_store_test.go` is proven to compile and to be race-clean.
   Whether its assertions executed is unknown, because an unreachable store would
   make it skip and this phase did not read a per-test verdict for it.
3. No baseline snapshot from before the fix exists for the assistant e2e package,
   so "no regression" for that package is grounded in exit 0 at the current tree
   rather than in a before-and-after comparison. The unit lane does have prior
   passes to compare against, and it is stable.

### Regression Phase Completion Statement

**Verdict: REGRESSION_FREE.** No previously-passing test now fails. No coverage
decreased. No cross-spec conflict or design contradiction surfaced. The race
detector is clean across four package groups, which is new evidence this packet
did not previously hold.

Two DoD items move to checked: the every-behaviour scenario-regression item and
the `passes unmodified` conjunction. The packet stands at eleven of twenty. One
finding is routed to `bubbles.simplify`: the dead `conv` parameter on
`finishConfirmResponse`, invisible to a `go vet`-only lint surface. One record
correction is entered: TP-BUG069006-03 exists as an uncommitted, unexecuted test
rather than being unwritten. `status` remains `in_progress`. No `certification.*`
field was written by this phase.

## Simplify Phase

Agent: `bubbles.simplify`. Two changes were made and three candidates were
examined and left alone. Both changes are inside the Change Boundary; the second
one is a correction *to* the Change Boundary, which the packet's own commit
history had already contradicted.

### Change 1 — the dead `conv` parameter

The regression phase routed one finding here: after the loser-path fix,
`Facade.finishConfirmResponse` still took a `conv assistantctx.Conversation`
parameter that nothing in its body read. That is invisible to this repo's lint
surface, because `scripts/runtime/go-lint.sh` is four lines ending in
`go vet ./...` with no golangci configuration, and `go vet` does not report
unused function parameters. Lint exiting 0 said nothing about it either way.

The parameter is removed, along with all three call sites at lines 171, 182 and
194. Removing it had a second-order effect worth stating plainly: `conv` then
became unread inside `handlePendingConfirm` too, since passing it onward was its
only remaining use.

`handlePendingConfirm` keeps the parameter, renamed to `_` and annotated. Full
removal was considered and rejected on a concrete ground rather than a stylistic
one: the sole caller is `internal/assistant/facade.go` line 634, and that file is
not listed in the Change Boundary. Editing it to delete one argument would widen
a boundary that this packet corrected two commits ago, to buy nothing the `_`
form does not already buy. The `_` form removes the dead binding, keeps the
compiler enforcing that nothing reads the snapshot, and records at the signature
why a snapshot arrives and is ignored.

That annotation is the part worth keeping regardless of form. The defect this
packet fixed was a stale pre-arbitration `Conversation` being written back; a
signature that silently accepted one and a body that silently ignored it is
precisely the shape that invites someone to start using it again.

### Change 2 — the Change Boundary omitted its own compile-forced ripple

Enumerating every implementation of the new interface method shows ten files, of
which five are not in the Change Boundary's allowed list:

```
$ grep -rln 'ClearPendingConfirm' --include='*.go' .
./internal/assistant/confirm/machine.go
./internal/assistant/confirm/machine_test.go
./internal/assistant/context/gauge_refresher_test.go
./internal/assistant/context/pg_store.go
./internal/assistant/context/pg_store_test.go
./internal/assistant/context/store.go
./internal/assistant/facade_test_helpers_test.go
./internal/assistant/testing_support.go
./tests/integration/assistant/confirmation_canary_test.go
./tests/integration/assistant/transport_parity_test.go
```

The five unlisted files each carry a test-local `Store` double. None of them was
a discretionary edit: adding a method to a Go interface stops every
implementation compiling until it gains the method. The set is fully determined
by the interface change the boundary already authorises in `store.go`.

Two of these five were the reason commit `00fcdf3b` existed at all — the
integration build had been broken since `61b8be79` because no test lane compiles
integration-tagged files, so nothing reported it.

`scopes.md` gains a `### Store-Interface Ripple Files` table naming all five with
that reason. This makes the boundary describe what the change actually forces.
The alternative was to leave an artifact that five commits already contradicted,
which is the worse outcome: a boundary nobody can trust stops constraining
anything.

### Examined and left unchanged

**`canaryStore` and `parityStore` near-duplication.** Both live in
`tests/integration/assistant`, both wrap `sync.Mutex` plus a
`map[string]Conversation`. Consolidation looked attractive until the difference
showed up: `parityStore` also carries `persist []string` and appends to it on
every write, because the transport-parity test asserts persist call ordering.
Merging them would push that recording machinery into the canary test, which
does not want it, or introduce an options-carrying shared base to hide it. Two
short honest doubles beat one indirect shared one.

**The six `ClearPendingConfirm` doubles.** They repeat a compare-then-clear under
one lock. Extracting a shared helper would create a test-support dependency
spanning `internal/assistant`, `internal/assistant/confirm`,
`internal/assistant/context` and `tests/integration/assistant`. The repo already
has one shared implementation, `InMemoryContextStore` in
`internal/assistant/testing_support.go`; the rest are deliberately local and
minimal so a test reads without a cross-package hop.

**The `PgStore` SQL.** The single conditional `UPDATE` is already the smallest
form that expresses the compare-and-clear atomically. There is nothing to remove
without losing the atomicity that is the whole point.

### Verification

The full Go unit suite, captured:

```
# BUG-069-006 simplify: full Go unit suite after dead-parameter removal
$ ./smackerel.sh test unit --go
exit: 0
lines: 210
sha256: 727e4a090dbb7d5474e73c759ae23e0fd3c2cdc3ab03b90ff7e3fab60cd5c6db
--- first 20 ---
oom-preflight: OK — 38540 MB available (need 6000 MB; swap used 1143 MB).
disk-preflight: OK — C: 96 GB free (need 40 GB), WSL / 470 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
[go-unit] envsubst missing — installing gettext-base
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 170 line(s); sha256 above covers the full output ---
--- last 20 ---
ok  	github.com/smackerel/smackerel/internal/testsupport/jssource	(cached)
ok  	github.com/smackerel/smackerel/internal/topics	(cached)
ok  	github.com/smackerel/smackerel/internal/web	(cached)
ok  	github.com/smackerel/smackerel/internal/web/admin	(cached)
ok  	github.com/smackerel/smackerel/internal/web/icons	(cached)
ok  	github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter	(cached)
ok  	github.com/smackerel/smackerel/tests/e2e/agent	(cached)
ok  	github.com/smackerel/smackerel/tests/e2e/assistant	(cached)
ok  	github.com/smackerel/smackerel/tests/eval/assistant	(cached)
ok  	github.com/smackerel/smackerel/tests/integration	(cached) [no tests to run]
?   	github.com/smackerel/smackerel/tests/integration/agent/routerwarmup	[no test files]
?   	github.com/smackerel/smackerel/tests/integration/drive/fixtures	[no test files]
?   	github.com/smackerel/smackerel/tests/integration/nslock	[no test files]
ok  	github.com/smackerel/smackerel/tests/observability	(cached)
ok  	github.com/smackerel/smackerel/tests/stress/readiness	(cached)
ok  	github.com/smackerel/smackerel/tests/unit/clients	(cached)
?   	github.com/smackerel/smackerel/web/pwa	[no test files]
ok  	github.com/smackerel/smackerel/web/pwa/tests	(cached)
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

Verify with `bash .github/bubbles/scripts/evidence-capture.sh --verify
727e4a090dbb7d5474e73c759ae23e0fd3c2cdc3ab03b90ff7e3fab60cd5c6db --
./smackerel.sh test unit --go`.

One honest note on that capture: the `ok` lines read `(cached)`. The fresh
compile happened on the first suite run after the edit, which returned exit 0
across 149 `ok` packages with zero `FAIL` lines; this capture is Go reusing that
result because nothing changed in between. The edit was compiled and the suite
passed against it — the cache marker records that the second invocation added no
new information, not that the first one was skipped.

The two concurrency tests re-run explicitly against the simplified code, verbose
so the selector match is visible rather than inferred. This matters here: a
`--go-run` selector that matches nothing also exits 0, so the exit code alone
would not have distinguished "both passed" from "neither ran".

```
$ ./smackerel.sh test unit --go --go-run 'TestMachineConfirm_Concurrent|TestMachineConfirm_RacingSweep' --verbose
=== RUN   TestMachineConfirm_ConcurrentRedemptionExecutesOnce
--- PASS: TestMachineConfirm_ConcurrentRedemptionExecutesOnce (0.00s)
=== RUN   TestMachineConfirm_RacingSweepProducesOneTerminalOutcome
--- PASS: TestMachineConfirm_RacingSweepProducesOneTerminalOutcome (0.00s)
exit: 0
```

Build Quality Gate commands, each run after the edit:

```
$ ./smackerel.sh lint
exit: 0
$ ./smackerel.sh format --check
exit: 0
```

### Code Diff Evidence

The simplified surface, reviewed with git diff against the working tree:

```
$ git diff --stat internal/assistant/compiled_interactions.go
 internal/assistant/compiled_interactions.go | 13 ++++++-------
 1 file changed, 6 insertions(+), 7 deletions(-)
```

### Outcome

Two changes landed, both inside the boundary. Three consolidation candidates were
examined and each was left alone for a stated reason rather than by omission. The
full unit suite, lint and format all exit 0 after the edit. No DoD item is
checked by this phase that its own evidence does not support, and no
`certification.*` field was written here.

## Stabilize Phase

Agent: `bubbles.stabilize`. Five operational questions were asked about the fix,
four of which resolve clean on measured evidence and one of which is a real gap
recorded with an owner. No product or test source was modified by this phase.

The framing worth stating up front: every prior phase asked whether the fix is
*correct*. None asked whether it is *operable* — whether the new SQL scans, how
long it holds a lock, whether a lost race can spin or lose work, whether a
database outage gets mistold to a user as something else, and whether any of it
is visible in production. Those are different questions and a passing test suite
answers none of them.

### 1. Index coverage — measured, not inferred

The new predicate combines two primary-key columns with a JSONB extraction:

```
UPDATE assistant_conversations
   SET pending_confirm = NULL, last_activity_at = $3
 WHERE user_id = $1 AND transport = $2
   AND pending_confirm ->> 'confirm_ref' = $4
```

A JSONB extraction in a `WHERE` clause is worth checking, because if it drove row
selection it would need its own expression index and would otherwise scan. It
does not drive selection here. The live schema:

```
$ docker exec smackerel-postgres-1 psql -U smackerel -d smackerel -c "\d assistant_conversations"
          Column           |           Type           | Nullable
---------------------------+--------------------------+----------
 user_id                   | text                     | not null
 transport                 | text                     | not null
 working_context           | jsonb                    | not null
 pending_confirm           | jsonb                    |
 pending_disambig          | jsonb                    |
 last_activity_at          | timestamp with time zone | not null
 schema_version            | integer                  | not null
 legacy_retirement_notices | jsonb                    | not null
 pending_clarify           | jsonb                    |
Indexes:
    "assistant_conversations_pkey" PRIMARY KEY, btree (user_id, transport)
    "idx_assistant_conversations_idle" btree (last_activity_at)
    "idx_assistant_conversations_pending_clarify" btree ((pending_clarify ->> 'emit_time'::text)) WHERE pending_clarify IS NOT NULL
```

`(user_id, transport)` is the primary key, so the pair identifies exactly one
row. Reasoning stops there only if you are willing to trust the planner without
asking it. It was asked, against a row that genuinely matches the predicate,
inside a transaction that is rolled back so the development database is left
byte-identical:

```
$ docker exec smackerel-postgres-1 psql -U smackerel -d smackerel -v ON_ERROR_STOP=1 -c "
BEGIN;
INSERT INTO assistant_conversations (user_id, transport, working_context, pending_confirm, last_activity_at, legacy_retirement_notices)
VALUES ('stabilize-probe-u1','http','{}'::jsonb, '{\"confirm_ref\":\"cr-probe-1\"}'::jsonb, now(), '{}'::jsonb);
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF)
UPDATE assistant_conversations SET pending_confirm = NULL, last_activity_at = now()
 WHERE user_id = 'stabilize-probe-u1' AND transport = 'http' AND pending_confirm ->> 'confirm_ref' = 'cr-probe-1';
ROLLBACK;"
INSERT 0 1
                                    QUERY PLAN
 Update on assistant_conversations (actual time=0.056..0.057 rows=0 loops=1)
   Buffers: shared hit=4
   ->  Index Scan using assistant_conversations_pkey on assistant_conversations (actual time=0.019..0.020 rows=1 loops=1)
         Index Cond: ((user_id = 'stabilize-probe-u1'::text) AND (transport = 'http'::text))
         Filter: ((pending_confirm ->> 'confirm_ref'::text) = 'cr-probe-1'::text)
         Buffers: shared hit=2
 Planning Time: 0.304 ms
 Execution Time: 0.141 ms
(10 rows)
ROLLBACK
```

Three things in that plan settle the question. It is an `Index Scan using
assistant_conversations_pkey`, not a sequential scan. The `Index Cond` carries
only the two primary-key columns while the JSONB extraction appears as a
`Filter`, which is the planner confirming the extraction is evaluated *after* the
index has already narrowed to one row rather than being used to find it. And
`Buffers: shared hit=2` with `Execution Time: 0.141 ms` puts a number on it.

The `rows=1` on the Index Scan matters more than it looks. An earlier probe
against a non-existent row produced the same plan shape with `rows=0`, which
proves the plan but not that the predicate can match. Seeding a real row and
seeing `rows=1` proves both.

The development database is unchanged, which the rollback and a direct count
confirm:

```
$ docker exec smackerel-postgres-1 psql -U smackerel -d smackerel -tAc "SELECT count(*) FROM assistant_conversations WHERE user_id='stabilize-probe-u1';"
0
```

Conclusion: no scan risk, and no expression index should be added. An index on
`pending_confirm ->> 'confirm_ref'` would add write amplification on every
conversation update while buying nothing the primary key does not already give.

### 2. Lock duration and contention — bounded, and strictly better than before

`ClearPendingConfirm` issues its statement through `s.pool.Exec`, not inside a
transaction the caller holds open:

```
	tag, err := s.pool.Exec(ctx, q, userID, transport, now.UTC(), confirmRef)
```

That makes it a single autocommit statement, so the row lock exists only for the
duration measured above. Concurrent redemptions of the same reference serialize
on that one row, and every caller after the winner observes zero rows affected,
because the winner has set `pending_confirm` to `NULL` and the extraction no
longer equals the reference.

Comparing against what it replaced is the useful part. The old shape was Load,
check, Persist as three separate statements with no lock held across them — the
window between the check and the write is exactly the defect. The new shape holds
a lock for less wall-clock time than any explicit transaction wrapping those
three statements would have, while being the thing that closes the window.

### 3. `SweepTimeouts` under load — no spin, no silent loss

The sweep iterates `expired []ExpiredPending`, a finite caller-supplied slice, so
`continue` advances the index. There is no retry loop, so a lost race cannot
spin. A store error is not swallowed:

```
		won, err := m.store.ClearPendingConfirm(ctx, e.UserID, e.Transport, e.ConfirmRef, now)
		if err != nil {
			return res, fmt.Errorf("confirm.SweepTimeouts: clear pending %s/%s: %w", e.UserID, e.Transport, err)
		}
		if !won {
			// Lost the race to a concurrent confirm/discard — that
			// caller owns the terminal audit row for this reference.
			continue
		}
```

Work is not dropped when the sweep loses. The racing confirm or discard owns the
terminal audit row, and that is asserted rather than assumed:
`TestMachineConfirm_RacingSweepProducesOneTerminalOutcome` requires exactly one
terminal audit row, which fails both if two are written and if none is.

### 4. Outage versus stale confirmation — the distinction holds end to end

This was the question with the worst failure mode. If a database outage were
mapped to `ErrPendingNotFound`, a user would be told their confirmation is stale
or already resolved when the truth is that the store is down. That is the exact
class of dishonesty the repository's Assistant Response Honesty rule exists to
prevent, and the fix introduces a new boolean that could easily have collapsed
it.

It does not collapse. `Confirm` and `Discard` both reserve `ErrPendingNotFound`
for the lost-race branch and wrap store failures instead:

```
	won, err := m.store.ClearPendingConfirm(ctx, in.UserID, in.Transport, in.ConfirmRef, now)
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("confirm.Confirm: clear pending confirm: %w", err)
	}
	if !won {
		return ConfirmResult{}, ErrPendingNotFound
	}
```

The response boundary preserves it. `finishConfirmResponse` selects the stale
message with `errors.Is(actionErr, confirm.ErrPendingNotFound)`, which cannot
match an error whose chain wraps a store failure rather than that sentinel, so an
outage falls through to the next branch and surfaces as a real error:

```
	if errors.Is(actionErr, confirm.ErrPendingNotFound) {
		... Body: "That confirmation is stale or already resolved." ...
	}
	if actionErr != nil {
		return contracts.AssistantResponse{}, true, actionErr
	}
```

A store outage therefore produces an error, not a false statement about the
user's confirmation.

### 5. Observability — one real gap, recorded

`ConfirmCardOutcomesTotal` is incremented on winner paths only: confirmed at
`machine.go` line 240, user-discarded at line 297, and timeout-discarded in the
sweep. A caller that *loses* the race increments nothing and logs nothing.

```
$ grep -nE 'log\.|slog|metric|Counter|Inc\(|Observe' internal/assistant/confirm/machine.go
18:	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
239:	// Spec 061 SCOPE-09 — confirm-card outcome metric.
240:	assistantmetrics.ConfirmCardOutcomesTotal.WithLabelValues(
242:		assistantmetrics.ConfirmOutcomeConfirmed,
244:	).Inc()
294:	// Spec 061 SCOPE-09 — confirm-card outcome metric +
297:	assistantmetrics.ConfirmCardOutcomesTotal.WithLabelValues(
299:		assistantmetrics.ConfirmOutcomeDiscardedUser,
301:	).Inc()
302:	assistantmetrics.CaptureFallbackTotal.WithLabelValues(
```

Silently absorbing the loser is the correct *behaviour* — that is what single
flight means. The gap is that it is operationally invisible: the metrics cannot
distinguish a system where no races occur from one where many occur and are being
correctly absorbed. If the guard ever regressed, nothing would signal it until a
gated action ran twice in production.

This is recorded as DIS-069-006-7 rather than fixed. Emitting it needs a new
outcome constant in `internal/assistant/metrics`, and that package is not on this
packet's Change Boundary, which was corrected twice already in this packet and
should not be widened a third time for an addition that is not part of the fix.

### Verification

No source was modified by this phase, so the suites are re-run as a statement
that the tree is unchanged rather than as proof of a new claim.

```
$ ./smackerel.sh test unit --go
exit: 0
$ ./smackerel.sh lint
exit: 0
$ ./smackerel.sh format --check
exit: 0
```

### Outcome

Four of five questions resolve clean on measured evidence: the update is a
primary-key index scan at 0.141 ms with the JSONB as a post-index filter, the row
lock is a single autocommit statement and shorter than the shape it replaced, the
sweep cannot spin or lose work, and an outage is never reported as a stale
confirmation. One real gap is recorded with an owner and not fixed here. No DoD
item is checked by this phase, and no `certification.*` field was written.

## Confirm E2E Raw Output

The regression phase recorded its lanes as a results table with sha256 hashes and
no raw command output. That is real evidence, but the state-transition guard's
Check 9 is right that a table is not command output, and two checked DoD items
pointed at anchors carrying only prose. Rather than repoint those items at a
weaker anchor, both lanes were re-executed here so the evidence is raw.

The dev stack is stopped first, because the e2e lane starts its own disposable
stack and running both at once exhausts memory:

```
$ ./smackerel.sh down
exit: 0
$ docker ps --format '{{.Names}}' | grep -c smackerel
0
```

Both confirm tests, run together against the live stack:

```
$ ./smackerel.sh test e2e --go-package assistant --go-run 'TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce|TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce'
go-e2e: applying package selector: assistant
go-e2e: applying -run selector: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce|TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce
=== RUN   TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.13s)
=== RUN   TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce (0.09s)
PASS
ok  	github.com/smackerel/smackerel/tests/e2e/assistant	0.260s
PASS: go-e2e
exit: 0
```

The runner echoes both selectors, which is what distinguishes two tests passing
from a selector that matched nothing and exited 0 anyway. Zero skips:

```
$ grep -c -- '--- SKIP' <captured e2e output>
0
```

That zero is the load-bearing number for this packet's lineage. BUG-069-005
exists because a required e2e skipped silently while the suite still reported
green, so a confirm-flow claim resting on anything other than an observed zero
skip count would repeat the defect the origin packet was opened to fix.

The `passes unmodified` half of the first item is a separate claim from the
`passes` half, and git settles it independently:

```
$ git status --porcelain tests/e2e/assistant/http_confirm_test.go
$ echo "exit: $?"
exit: 0
```

Empty output means the file carries no working-tree modification at the tree that
produced the PASS above. The pre-existing sequential test therefore passes in its
original form, which is what makes it a regression check on the fix rather than a
test quietly rewritten to accommodate it.
