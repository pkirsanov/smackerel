# User Validation: BUG-013-001 — HospitalityLinker counter idempotency

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Scope of Externally Observable Change

This fix has **no externally observable functional change** under
nominal first-delivery operation. Operators see:

- The same hospitality digest content.
- The same `STAYED_AT`, `EXPENSE_AT`, `INVOICED_AT`, `REPORTED_AT`,
  and `DURING_STAY` edges in the knowledge graph.
- The same counter values on `guests.total_stays`,
  `guests.total_spend`, `properties.total_bookings`,
  `properties.total_revenue`, `properties.issue_count` after a single
  successful delivery.

The change is in the **invariant under redelivery**:

- **Before** — a process restart, AckWait expiry, or Nak retry would
  silently inflate the counters by one apply per redelivery. Operators
  could not see this drift without ground-truth reconciliation against
  the append-only `artifacts` table.
- **After** — the counters land exactly once per `(artifact_id, op_kind)`
  regardless of how many times NATS delivers the same payload.

## Acceptance Surface

| Surface | Validation step | Expected outcome |
|---------|-----------------|------------------|
| Operator-facing hospitality counters | Send the same GuestHost booking through ingestion → observe one row in `guests` with `total_stays=1`; restart the core process while the message is in-flight (or simulate via the integration test) → observe `total_stays` stays at 1 | Counters do not inflate on redelivery |
| Hospitality digest | View the daily digest after a redelivery event | Same digest content; no inflated property/guest stats |
| Edges | Query the knowledge graph for `STAYED_AT(guest, property)` and `EXPENSE_AT(invoice, property)` edges | Single edge per (guest, property) and (invoice, property) regardless of redelivery — same as before this fix |
| Dedup ledger | `SELECT artifact_id, op_kind, applied_at FROM hospitality_counter_applications ORDER BY applied_at DESC LIMIT 20;` | New operator-visible table; each row records the first-time apply of a counter mutation. Useful for reconciliation if operators ever need to audit counter values vs the append-only `artifacts` table. |
| Migration | `SELECT version, applied_at FROM schema_migrations WHERE version = '039_hospitality_counter_applications' ;` | Single row recorded once on the first process start after deployment. Migration is `IF NOT EXISTS` and idempotent. |

## Validation Steps

1. Deploy the change to a test environment with the persistent
   PostgreSQL test database.
2. Verify migration 039 ran exactly once:
   ```sql
   SELECT version, applied_at FROM schema_migrations
   WHERE version = '039_hospitality_counter_applications';
   ```
3. Send a GuestHost booking artifact through the ingestion pipeline.
4. Confirm one row each in `guests` (with `total_stays=1`) and
   `properties` (with `total_bookings=1`).
5. Confirm two rows in `hospitality_counter_applications`:
   `(artifact_id, "guest_stay_increment")` and
   `(artifact_id, "property_booking_increment")`.
6. Simulate a redelivery by re-publishing the same `artifacts.processed`
   payload (or, equivalently, run the integration test
   `TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery`).
7. Confirm:
   - `guests.total_stays` is still 1 (not 2).
   - `properties.total_bookings` is still 1 (not 2).
   - `hospitality_counter_applications` still has the same two rows
     (no INSERT happened on redelivery because of `ON CONFLICT DO NOTHING`).
   - The hospitality linker emitted a `slog.Debug` line noting the
     dedup (visible in container logs at `LOG_LEVEL=debug`).

## Operator Visibility / Observability

- **`slog.Debug` on dedup hit** — every redelivery emits a structured
  log line `"hospitality linker: guest stay counter already applied
  (NATS redelivery dedup)"` (or the property/task equivalent). Visible
  at `LOG_LEVEL=debug` for operators who want to audit redelivery
  frequency. Not visible at default INFO level (low-noise).
- **`slog.Warn` on claim failure** — if the dedup INSERT itself fails
  (infrastructure error), a Warn-level log line is emitted with
  `artifact_id`, `op_kind`, and the error. Visible at default LOG_LEVEL.
  Same severity as existing linker errors (which already log at Warn
  per the pipeline convention).
- **No new metric** — this fix is correctness-only. If operational
  data later shows redelivery is a recurring concern, a counter metric
  on `hospitality_counter_applications` row count vs `artifacts` row
  count is a small follow-up (out of scope here).

## Backfill / Historical Drift

This fix does NOT backfill any drift that may have occurred prior to
deployment. If operators suspect historical counter drift, the
reconciliation procedure is:

1. Iterate the append-only `artifacts` table for hospitality artifact
   types (`booking`, `expense`, `invoice`, `review`, `inspection`,
   `task`).
2. Re-derive the expected counter values from the artifact set.
3. Compare against the persisted `guests` / `properties` counters.
4. Update counters in-place if drift is observed.

This is operator-driven, not automatic. The append-only `artifacts`
table preserves the source events for as long as the artifact retention
window allows.

## Checklist

- [x] No public API change.
- [x] No NATS subject rename or schema breaking change.
- [x] Migration is forward-only and idempotent (`IF NOT EXISTS`).
- [x] Existing `guests` / `properties` schema unchanged.
- [x] Existing repo method signatures unchanged.
- [x] Existing edge writers unchanged.
- [x] Dedup ledger is operator-queryable (and `applied_at`-indexed for
      future TTL pruning if needed).
- [x] Three integration tests cover the positive path (idempotency),
      the task-delta path, and the adversarial inverse (independent
      accumulation must not be over-blocked by the dedup ledger).
- [x] `go test -count=1 -race` clean.
- [x] `./smackerel.sh build` and `./smackerel.sh check` clean.

## Sign-off

**Validation owner:** bubbles.workflow (parent-expanded stabilize-to-doc child of stochastic-quality-sweep R5, sweep `sweep-2026-05-25-r10`)
**Status:** Done
**Date:** 2026-05-25
