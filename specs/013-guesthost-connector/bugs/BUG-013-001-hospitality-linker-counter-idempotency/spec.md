# Bug: BUG-013-001 — HospitalityLinker counter mutations are not idempotent under NATS redelivery

## Classification

- **Type:** Stabilize / data-integrity defect — counters in `guests` and `properties` tables drift upward (or task delta downward) on NATS redelivery of the same `artifacts.processed` payload because `HospitalityLinker.LinkArtifact` applies counter mutations unconditionally on every invocation.
- **Severity:** MEDIUM — silent persisted counter drift; no error, no warning, no log line on the redundant apply. Operators see inflated `total_stays`, `total_spend`, `total_bookings`, `total_revenue` (or under-count for `issue_count` when `status="completed"` is redelivered) without any observability signal.
- **Parent Spec:** 013 — GuestHost Connector
- **Workflow Mode:** bugfix-fastlane (parent-expanded child of stochastic-quality-sweep round 5, trigger `stabilize`, mapped mode `stabilize-to-doc`)
- **Status:** Open — discovered by stabilize R5 (sweep `sweep-2026-05-25-r10`)

## Problem Statement

`internal/graph/hospitality_linker.go::LinkArtifact` is the hospitality-graph
extension to the standard linker pipeline. The pipeline calls it from
`internal/pipeline/processor.go::HandleProcessedResult` (line 701) after
the standard `Linker.LinkArtifact` (line 686). The standard linker's
edge writes are idempotent because
`internal/graph/linker.go::createEdgeWithMetadata` uses
`ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE`.

The hospitality linker reuses that same idempotent edge writer (its own
`createEdgeWithMetadata` delegates to the same SQL pattern), but it also
performs **per-event counter mutations** that are NOT idempotent:

```go
// linkBooking (pre-fix, internal/graph/hospitality_linker.go:148-180)
if err := l.guestRepo.IncrementStay(ctx, guest.ID, meta.Revenue); err != nil { ... }
if err := l.propertyRepo.IncrementBookings(ctx, property.ID, meta.Revenue); err != nil { ... }

// linkTask (pre-fix, internal/graph/hospitality_linker.go:207-227)
delta := 1
if meta.Status == "completed" {
    delta = -1
}
if err := l.propertyRepo.UpdateIssueCount(ctx, property.ID, delta); err != nil { ... }
```

`GuestRepository.IncrementStay` runs
`UPDATE guests SET total_stays = total_stays + 1, total_spend = total_spend + $2, ...`.
`PropertyRepository.IncrementBookings` runs
`UPDATE properties SET total_bookings = total_bookings + 1, total_revenue = total_revenue + $2, ...`.
`PropertyRepository.UpdateIssueCount` runs
`UPDATE properties SET issue_count = GREATEST(0, issue_count + $2), ...`.

None of these are idempotent at the row level — each invocation adds
to the running counter.

The `artifacts.processed` NATS consumer
(`internal/pipeline/subscriber.go::20-260`) is configured with:

```go
AckPolicy:  jetstream.AckExplicitPolicy
AckWait:    30 * time.Second
MaxDeliver: <DefaultMaxDeliver>  // > 1
```

and the success path acks only at the end of
`HandleProcessedResult` (line ~258 of `subscriber.go`, after the linker
calls, synthesis publish, and domain-extraction publish all succeed).
Any of the following redelivers the SAME `payload.ArtifactID` and drives
a second pass through `HospitalityLinker.LinkArtifact`:

1. **AckWait expiry.** DB latency, GC pause, or a slow synthesis publish
   pushes the handler past 30 s. NATS redelivers the message; the
   counters are applied a second time before the first ack arrives.
2. **Process crash between counter UPDATE and msg.Ack().** The counters
   already landed in PostgreSQL but the message is still in the
   un-acked window. On restart NATS redelivers; counters land a second
   time.
3. **Handler transient error producing a Nak.**
   `internal/pipeline/subscriber.go::handleDeliveryFailure` Naks the
   message for retry up to `MaxDeliver` before dead-lettering. The
   counter mutator may have succeeded in PostgreSQL while a later step
   (e.g., synthesis publish) failed — the retry then re-applies the
   counters.

The edges remain correct because `ON CONFLICT DO UPDATE` reduces the
re-write to a no-op. The counters drift silently.

## Impact

| Axis | Impact |
|------|--------|
| **Data integrity** | `guests.total_stays`, `guests.total_spend`, `properties.total_bookings`, `properties.total_revenue`, and `properties.issue_count` drift upward (or downward for issue_count when status=completed) on every NATS redelivery. The drift is unbounded — any process crash between the counter UPDATE and the NATS ack will inflate counters by one apply per restart. |
| **Observability** | The drift is silent. No counter, no warn log, no error. Operators see inflated aggregates with no signal that a redelivery occurred. |
| **Downstream impact** | Counters feed the hospitality digest, property rankings, the recommendation engine, and any operator dashboards. Inflated counters bias all downstream consumers and propagate further drift on rebuilds. |
| **Cross-product impact** | None — spec 013 is internal to Smackerel; QF Companion is unaffected. |
| **Recovery cost** | Without a per-(artifact, op) dedup ledger, the only way to back out drift is a full rebuild of derived state from the append-only `artifacts` table — operationally expensive. |
| **Severity** | MEDIUM. Counters are persisted operator-visible state. Silent corruption with no monitoring signal. Bounded only by `CHECK (total_spend >= 0)` style invariants which forbid runaway negative drift but allow unbounded positive drift. |

## Why this is "stabilize" not "harden" or "chaos"

Per `stabilize-to-doc` charter: reliability gap on an already-functional
surface that manifests under retry, redelivery, or burst dispatch — exactly
the failure mode here. The defect does not violate a security invariant
(rules out `harden`); it is not an external-failure stress probe (rules
out `chaos`); it is not a missing piece of ergonomics, symmetry, or API
consistency (rules out `improve`). It is a latent stability defect that
emerges only under at-least-once NATS redelivery, which is exactly what
the JetStream consumer protocol guarantees. This is the canonical
stabilize signature.

## Why prior rounds missed it

- **CHAOS R20 (2026-05-13)** — exercised cursor regression and future-skew
  defenses against the GuestHost connector loop (closed as
  CHAOS-013-R20-001 and CHAOS-013-R20-002 inside spec 013). The chaos
  probe stopped at the connector boundary — it did not exercise the
  post-ML hospitality-linker counter path.
- **Existing GuestHost integration tests** (`tests/integration/guesthost_*_test.go`)
  are STUBs (see baseline 45 STG BLOCKs); they do not exercise the
  counter idempotency path even when promoted to real tests.
- **The base `Linker` test suite** asserts edge idempotency via
  `ON CONFLICT DO UPDATE` semantics but does not assert counter
  idempotency, because counters are not part of the base linker's
  contract — only the hospitality-graph extension uses them.
- **No prior sweep round** targeted the GuestHost connector with the
  `stabilize` trigger.

R5 stabilize (this round) extends the lens to "what happens when the
same artifact_id is re-delivered through `HandleProcessedResult`" and
discovered the counter drift.

## Reproduction (pre-fix)

The integration test
`tests/integration/guesthost_counter_idempotency_test.go::TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery`
reproduces the defect deterministically. Pre-fix, the second
`linker.LinkArtifact(ctx, artifactID)` call would land the counters a
second time and the test assertion `guests.total_stays = 1` would fail
with `guests.total_stays = 2`. With the fix, the assertion holds.

The same fixture covers the task-status delta in
`TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery`, and the
"don't dedup over-block legitimate accumulation" surface in
`TestHospitalityLinker_DifferentArtifactsApplyIndependently`.

## Acceptance Criteria

- [ ] A new migration `internal/db/migrations/039_hospitality_counter_applications.sql`
      creates `hospitality_counter_applications (artifact_id TEXT, op_kind TEXT, applied_at TIMESTAMPTZ, PRIMARY KEY (artifact_id, op_kind))`
      with a CHECK constraint on `op_kind` (whitelist of three values:
      `guest_stay_increment`, `property_booking_increment`, `property_issue_delta`)
      and an index on `applied_at`.
- [ ] `internal/graph/hospitality_linker.go` adds three package-level
      `op_kind` constants and a new method
      `tryClaimCounterApplication(ctx, artifactID, opKind string) (bool, error)`
      that performs `INSERT ... ON CONFLICT DO NOTHING RETURNING applied_at`
      and returns `(true, nil)` on first claim, `(false, nil)` on conflict,
      and `(false, err)` on infrastructure error.
- [ ] `linkBooking` gates `guestRepo.IncrementStay` behind a claim with
      `op_kind = "guest_stay_increment"` and gates
      `propertyRepo.IncrementBookings` behind a claim with
      `op_kind = "property_booking_increment"`.
- [ ] `linkTask` gates `propertyRepo.UpdateIssueCount` behind a claim
      with `op_kind = "property_issue_delta"`.
- [ ] Fail-open policy on infrastructure error: if the claim helper
      returns `(false, err)` the counter is NOT applied (do not double-apply
      on uncertain claim state) and a `slog.Warn` is emitted with
      `artifact_id`, `op_kind`, and the error.
- [ ] No public signature on `HospitalityLinker`, `GuestRepository`, or
      `PropertyRepository` is changed.
- [ ] Edge writes via `createEdge` / `createEdgeWithMetadata` remain
      unchanged (already idempotent via `ON CONFLICT DO UPDATE`).
- [ ] A new build-tagged integration test
      `tests/integration/guesthost_counter_idempotency_test.go`
      (`//go:build integration`) provides three persistent regression
      tests:
   - `TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery` —
        asserts that two redeliveries of the same booking artifact land
        `guests.total_stays = 1`, `guests.total_spend ≈ revenue` (single
        apply), `properties.total_bookings = 1`,
        `properties.total_revenue ≈ revenue`, and that a third
        redelivery still produces the same row (durability across N
        redeliveries).
   - `TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery` —
        asserts that three deliveries of the same task artifact land
        `properties.issue_count = 1` (single delta apply).
   - `TestHospitalityLinker_DifferentArtifactsApplyIndependently` —
        asserts that three distinct booking artifacts for the same
        guest/property accumulate `total_stays = 3` and
        `total_bookings = 3` (dedup does not over-block).
- [ ] No `t.Skip()` calls in the new integration test — on missing
      `DATABASE_URL` it `t.Fatal`s with an actionable message
      (precedent: `tests/integration/intelligence_annotation_race_test.go`
      BUG-027-002).
- [ ] `go test -count=1 -race ./internal/graph/...` PASS — proves no
      goroutine hazards introduced in the linker.
- [ ] `go build ./...` clean and
      `go vet ./internal/graph/... ./internal/db/... ./internal/pipeline/...` clean.
- [ ] `./smackerel.sh build` and `./smackerel.sh check` clean.
- [ ] `./smackerel.sh test integration --go-run TestHospitalityLinker_`
      PASS against the live test stack (all three regression tests).
- [ ] `artifact-lint.sh`, `state-transition-guard.sh`, and
      `traceability-guard.sh` PASS for this bug folder.

## Non-Goals

- Backfilling or reconciling historical `total_stays`, `total_spend`,
  `total_bookings`, `total_revenue`, or `issue_count` values that may
  have drifted prior to this fix. The append-only `artifacts` table
  preserves the source events; backfill is a separate operational task
  if it becomes necessary.
- Promoting `LinkArtifact` to a transactional path (counter UPDATE +
  hospitality_counter_applications INSERT in a single tx). The single
  `INSERT ... ON CONFLICT DO NOTHING` claim is enough because:
    1. The claim is the gate; if it succeeds, the counter is owed and
       will be applied (any subsequent failure to apply is logged as a
       Warn but does not corrupt state because no row can be claimed
       twice).
    2. A claim followed by a counter UPDATE that crashes mid-flight
       still leaves the dedup ledger consistent: the ledger row says
       "applied", the counter says "no" — the operational cost is one
       missing apply, NOT a double apply. Counter drift downward is
       bounded by the same redelivery semantics that previously drove
       it upward and is observable through normal operator data
       reconciliation.
- Changing the public `Annotation` schema, the NATS subject name, the
  hospitality-graph node/edge schema, or any repo method signatures.
- Wider refactor of the linker's structured logging or metrics.
- Changes to `Linker` or `Pipeline.Processor` — neither's contract is
  modified; only `HospitalityLinker` gains the gating layer.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-BUG-013-001-001 Counter mutation is applied exactly once on first delivery
  Given a GuestHost booking artifact arrives on artifacts.processed for the first time
  When HospitalityLinker.LinkArtifact processes the artifact
  Then guests.total_stays is incremented by 1
    And guests.total_spend is incremented by the booking revenue
    And properties.total_bookings is incremented by 1
    And properties.total_revenue is incremented by the booking revenue
    And hospitality_counter_applications has rows (artifact_id, "guest_stay_increment")
        and (artifact_id, "property_booking_increment")

Scenario: SCN-BUG-013-001-002 NATS redelivery does not double-count booking counters
  Given a GuestHost booking artifact has already been linked once
  When NATS redelivers the same artifacts.processed payload
       (AckWait expiry, crash-before-ack, or handler-error Nak retry)
   And HospitalityLinker.LinkArtifact runs again for the same artifact_id
  Then guests.total_stays remains 1 (not 2)
    And guests.total_spend remains the original revenue (not 2× revenue)
    And properties.total_bookings remains 1 (not 2)
    And properties.total_revenue remains the original revenue (not 2× revenue)
    And edges remain a single STAYED_AT edge (already idempotent)

Scenario: SCN-BUG-013-001-003 Task issue delta is idempotent across redelivery
  Given a GuestHost task artifact with status="open" has already been linked
  When NATS redelivers the same task artifact through HospitalityLinker.LinkArtifact
  Then properties.issue_count remains at its post-first-delivery value
    And the delta is not re-applied

Scenario: SCN-BUG-013-001-004 Idempotency claim failure does not double-apply the counter
  Given the hospitality_counter_applications INSERT returns an infrastructure error
  When HospitalityLinker tries to apply a counter mutation
  Then the counter is NOT applied (fail-closed for the counter)
    And a structured warning is logged with artifact_id, op_kind, and error
    And the artifact's edges (STAYED_AT, EXPENSE_AT, etc.) are still created

Scenario: SCN-BUG-013-001-005 Independent artifacts accumulate counters as expected
  Given three distinct booking artifacts for the same guest and property arrive
  When HospitalityLinker.LinkArtifact processes each (with one redelivery each
       to exercise the dedup ledger)
  Then guests.total_stays equals 3 (not 1, not 6)
    And properties.total_bookings equals 3 (not 1, not 6)
    And hospitality_counter_applications has 6 rows
        (3 artifact_ids × 2 op_kinds)
```
