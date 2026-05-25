# Design: BUG-013-001 — HospitalityLinker counter idempotency

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Current Truth (objective baseline before this fix)

Established from the codebase at HEAD `0de1a3cca8ecb5c9dacc6e57da6161ac216da367`:

1. **HospitalityLinker counter call-sites** — `internal/graph/hospitality_linker.go`:
   - `linkBooking` (lines 148-180) calls `IncrementStay(guest.ID, meta.Revenue)`
     and `IncrementBookings(property.ID, meta.Revenue)` unconditionally.
   - `linkTask` (lines 207-227) computes `delta = 1` (or `-1` when
     `meta.Status == "completed"`) and calls
     `UpdateIssueCount(property.ID, delta)` unconditionally.
   - Edge writes via `createEdgeWithMetadata` already use
     `ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE`
     and are idempotent.

2. **Counter SQL is non-idempotent** — `internal/db/guest_repo.go::IncrementStay`:
   ```sql
   UPDATE guests
   SET total_stays = total_stays + 1,
       total_spend = total_spend + $2,
       last_stay_at = NOW(),
       first_stay_at = COALESCE(first_stay_at, NOW()),
       updated_at = NOW()
   WHERE id = $1
   ```
   and `internal/db/property_repo.go::IncrementBookings`:
   ```sql
   UPDATE properties
   SET total_bookings = total_bookings + 1,
       total_revenue = total_revenue + $2,
       updated_at = NOW()
   WHERE id = $1
   ```
   and `UpdateIssueCount`:
   ```sql
   UPDATE properties
   SET issue_count = GREATEST(0, issue_count + $2),
       updated_at = NOW()
   WHERE id = $1
   ```

3. **Linker entry point** — `internal/pipeline/processor.go::HandleProcessedResult`
   line 701 invokes `HospitalityLinker.LinkArtifact` after the base
   `Linker.LinkArtifact` on line 686. Linker errors at either line are
   only logged at `Warn` and do not propagate (so a failed link cannot
   stop the rest of the pipeline, including the eventual `msg.Ack()`).

4. **NATS at-least-once delivery** — `internal/pipeline/subscriber.go::40-260`:
   - `AckPolicy=AckExplicitPolicy`
   - `AckWait=30 * time.Second`
   - `MaxDeliver>1` (default ≥ 5)
   - Success path invokes `HandleProcessedResult` (line ~227), then
     publishes synthesis + domain-extraction requests, then calls
     `msg.Ack()` (line ~258).
   - Failure path (`handleDeliveryFailure`) Naks for retry up to
     `MaxDeliver` before publishing to `deadletter.artifacts.processed`.

5. **Schema baseline** — `internal/db/migrations/001_initial_schema.sql`:
   - `guests (id, email, source, total_stays INT, total_spend REAL CHECK >= 0, ...)`
   - `properties (id, external_id, source, total_bookings INT,
     total_revenue REAL CHECK >= 0, issue_count INT CHECK >= 0, ...)`
   - No existing dedup table for (artifact, op).

6. **Latest migration** is `038_notification_ntfy_source_adapter.sql`;
   `039_*` is the next available numerical prefix. The migration loader
   in `internal/db/migrate.go` uses `//go:embed migrations/*.sql` plus
   a `schema_migrations` table guarded by an advisory lock — strictly
   ordered, applied once.

Conclusion: edges are idempotent today; counters are not. NATS
at-least-once guarantees that redelivery is a normal, expected event,
not an exceptional one. Counter drift is therefore a latent stability
defect that surfaces on any of three concrete redelivery causes (AckWait,
crash, Nak).

## Solution

### Data structure

A dedicated ledger keyed on `(artifact_id, op_kind)`:

```sql
CREATE TABLE IF NOT EXISTS hospitality_counter_applications (
    artifact_id TEXT NOT NULL,
    op_kind     TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (artifact_id, op_kind),
    CHECK (op_kind IN (
        'guest_stay_increment',
        'property_booking_increment',
        'property_issue_delta'
    ))
);
CREATE INDEX IF NOT EXISTS idx_hospitality_counter_applications_applied_at
    ON hospitality_counter_applications (applied_at);
```

Why a dedicated table (not, e.g., a `hospitality_counter_applied_at`
column on `artifacts`):

- **Granularity.** A booking artifact triggers TWO independent
  mutations (guest stay + property booking). A task artifact triggers
  ONE (property issue delta). Future hospitality artifact types could
  add more. A column on `artifacts` would have to expand into a JSONB
  set or three booleans; the dedicated table is the canonical SQL way
  to model an N-to-M apply ledger.
- **Cleanup.** The `applied_at` index allows operational TTL-style
  pruning if the table grows beyond the artifact retention window. Not
  enforced by code; available to ops.
- **Auditability.** The (artifact_id, op_kind) pairs are queryable for
  reconciliation against the `artifacts` table.

### Helper signature

```go
// internal/graph/hospitality_linker.go

const (
    hospitalityOpKindGuestStayIncrement      = "guest_stay_increment"
    hospitalityOpKindPropertyBookingIncrement = "property_booking_increment"
    hospitalityOpKindPropertyIssueDelta       = "property_issue_delta"
)

// tryClaimCounterApplication attempts to claim a one-time counter mutation
// against hospitality_counter_applications. Returns (true, nil) when this is
// the first time (artifactID, opKind) has been seen and the caller MUST
// proceed with the counter mutation. Returns (false, nil) when the row
// already exists (NATS redelivery; counter MUST be skipped). Returns
// (false, err) on infrastructure errors — callers MUST NOT apply the counter
// in this case because we cannot tell whether it has already been applied.
func (l *HospitalityLinker) tryClaimCounterApplication(ctx context.Context, artifactID, opKind string) (bool, error)
```

Implementation:

```go
var inserted bool
err := l.pool.QueryRow(ctx, `
    WITH ins AS (
        INSERT INTO hospitality_counter_applications (artifact_id, op_kind)
        VALUES ($1, $2)
        ON CONFLICT (artifact_id, op_kind) DO NOTHING
        RETURNING 1
    )
    SELECT EXISTS (SELECT 1 FROM ins)
`, artifactID, opKind).Scan(&inserted)
```

The CTE-with-EXISTS pattern is necessary because pgx returns
`pgx.ErrNoRows` for `INSERT ... ON CONFLICT DO NOTHING RETURNING` when
the conflict path fires (no row returned to the caller). Wrapping the
INSERT in a CTE and selecting `EXISTS (SELECT 1 FROM ins)` returns a
boolean unconditionally — `true` when the INSERT actually wrote a row,
`false` when the conflict path swallowed it. This is the canonical pgx
idiom for "did the upsert insert anything?" without needing
`RowsAffected` (which is also fine but adds a separate exec/query
round-trip).

### Call-site changes

`linkBooking` (full body, post-fix):

```go
if guest != nil && property != nil {
    if err := l.createEdge(ctx, "guest", guest.ID, "property", property.ID, "STAYED_AT", 1.0); err == nil {
        count++
    }

    // Guest stay counter
    claimed, claimErr := l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindGuestStayIncrement)
    if claimErr != nil {
        slog.Warn("hospitality linker: claim guest stay idempotency failed", "artifact_id", artifactID, "guest_id", guest.ID, "error", claimErr)
    } else if claimed {
        if err := l.guestRepo.IncrementStay(ctx, guest.ID, meta.Revenue); err != nil {
            slog.Warn("hospitality linker: increment guest stay failed", "guest_id", guest.ID, "error", err)
        }
    } else {
        slog.Debug("hospitality linker: guest stay counter already applied (NATS redelivery dedup)", "artifact_id", artifactID, "guest_id", guest.ID)
    }

    // Property booking counter
    claimed, claimErr = l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindPropertyBookingIncrement)
    if claimErr != nil {
        slog.Warn("hospitality linker: claim property booking idempotency failed", "artifact_id", artifactID, "property_id", property.ID, "error", claimErr)
    } else if claimed {
        if err := l.propertyRepo.IncrementBookings(ctx, property.ID, meta.Revenue); err != nil {
            slog.Warn("hospitality linker: increment property bookings failed", "property_id", property.ID, "error", err)
        }
    } else {
        slog.Debug("hospitality linker: property booking counter already applied (NATS redelivery dedup)", "artifact_id", artifactID, "property_id", property.ID)
    }
}
```

`linkTask` gates the existing `UpdateIssueCount` call with the same
pattern using `hospitalityOpKindPropertyIssueDelta`. The same `meta.Status`
parse logic computes the same delta on every call because `content_raw`
is the immutable post-ingestion payload — the second call recomputes
the same value, and the claim ensures it is NOT applied a second time.

### Fail-open vs fail-closed policy

The claim helper returns `(false, err)` on infrastructure error. The
linker treats this as **fail-closed for the counter** (do not apply)
but **fail-open for the rest of the pipeline** (continue with edges,
log a Warn, return). Rationale:

- The pipeline's existing convention is to log linker errors at Warn
  and not propagate (`processor.go:686/701`). Maintaining that
  convention.
- Skipping a counter apply on infrastructure-error is the safer side
  of the bet — the alternative (apply anyway) is exactly the drift we
  are fixing. A missed apply produces an under-count, which is
  observable through reconciliation against `artifacts` and is bounded
  by the redelivery semantics (the next successful delivery's claim
  will land normally because the conflict path is keyed on
  `(artifact_id, op_kind)` — a missed apply does NOT block future
  artifacts).

### Why not transactional?

A full transaction wrapping the claim + counter UPDATE would buy:
- Atomicity (claim and counter land together)
- Rollback on counter failure

It would cost:
- A new SQL round-trip pattern (BEGIN / claim / UPDATE / COMMIT)
- Hold open the pgx pool connection longer

The single-statement claim is sufficient because:
1. The claim is the gate; if it succeeds, the counter is owed.
2. A claim that succeeds followed by a counter UPDATE that fails leaves
   the dedup ledger consistent (the row says "applied", the counter
   says "no") — the operational cost is one missing apply, which is
   bounded and observable. Critically, the next redelivery will NOT
   re-claim and will NOT re-apply — the counter stays under-counted by
   exactly one. This is acceptable.
3. The base linker pattern (`Linker.LinkArtifact`) uses the same
   non-transactional, log-and-continue style.

If operational data later shows missed applies are a recurring problem,
the transaction can be added without an API change to the helper.

### Schema rationale (op_kind whitelist via CHECK)

The `CHECK (op_kind IN (...))` constraint:
- Prevents typos at the call site from creating ghost ledger entries
  (e.g., `"guest_stay_inc"` vs `"guest_stay_increment"`).
- Anchors the table semantics to the three current op kinds; adding a
  new op_kind requires a follow-up migration that extends the CHECK,
  which forces a deliberate review.
- Costs nothing on hot-path INSERT (simple IN check).

### What's NOT touched

- `Linker` base linker — unchanged.
- `Processor.HandleProcessedResult` — unchanged.
- `Subscriber` NATS configuration — unchanged.
- `GuestRepository.IncrementStay`, `PropertyRepository.IncrementBookings`,
  `PropertyRepository.UpdateIssueCount` signatures and SQL — unchanged.
- `createEdge` / `createEdgeWithMetadata` — unchanged (already
  idempotent).
- `Annotation`, `interactionMap`, schema for `guests` / `properties`
  themselves — unchanged.

## Threats / risks

| Risk | Mitigation |
|------|-----------|
| Dedup table grows unbounded as artifact count grows | `applied_at` index allows operational TTL pruning; spec 022 operational-resilience can pick this up if/when it becomes an issue. Not a correctness concern. |
| `op_kind` CHECK locks the schema to 3 op kinds | Adding a 4th op kind requires a follow-up migration to extend the CHECK. Acceptable cost for the typo safety it provides. |
| Counter under-apply on claim-then-UPDATE-fail | Bounded by redelivery semantics. Operational observability via the dedup ledger row + counter reconciliation. |
| Test stack DATABASE_URL not exported in CI | All three regression tests `t.Fatal` with an actionable message (precedent BUG-027-002). No silent skips. |

## Test surface

The new integration test
`tests/integration/guesthost_counter_idempotency_test.go` covers:

1. **First-delivery counter apply** — implicit in every test setUp.
2. **Booking-path redelivery idempotency** —
   `TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery`
   (two redeliveries → counters still 1; third redelivery →
   counters still 1).
3. **Task-path redelivery idempotency** —
   `TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery`
   (three deliveries → issue_count still 1).
4. **Dedup does NOT over-block legitimate accumulation** —
   `TestHospitalityLinker_DifferentArtifactsApplyIndependently`
   (3 distinct artifacts × 2 deliveries each → counters = 3, not 1
   and not 6).

Coverage rationale:
- (2) and (3) are the direct positive proofs of the fix.
- (4) is the adversarial inverse — a fix that "just doesn't apply
  counters" would pass (2) and (3) but fail (4). This test catches a
  regression where the dedup ledger is mis-keyed (e.g., uses
  `(op_kind)` instead of `(artifact_id, op_kind)`) and would over-block
  legitimate accumulation.

## Migration safety

`039_hospitality_counter_applications.sql` is `CREATE TABLE IF NOT EXISTS`
+ `CREATE INDEX IF NOT EXISTS` + CHECK constraint. It is forward-only
and idempotent (re-running is a no-op). It does NOT backfill any
existing drift in `guests` / `properties` — that is a separate
operational concern called out in Non-Goals.
