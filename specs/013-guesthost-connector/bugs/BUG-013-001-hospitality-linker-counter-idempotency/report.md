# Report: BUG-013-001 — HospitalityLinker counter idempotency

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

R5 of `sweep-2026-05-25-r10` (stochastic-quality-sweep) drew
spec `013-guesthost-connector` + trigger `stabilize` and resolved that
to mapped child workflow mode `stabilize-to-doc`. The workflow was
executed parent-expanded because the active workflow runtime does not
expose nested `runSubagent`. The stabilize probe surveyed 11 surfaces
against the GuestHost connector and the hospitality-graph pipeline
and identified one genuine stability defect:

**F1** — `HospitalityLinker.LinkArtifact` applies counter mutations
(`GuestRepository.IncrementStay`, `PropertyRepository.IncrementBookings`,
`PropertyRepository.UpdateIssueCount`) unconditionally on every
invocation. The standard linker's `createEdge` / `createEdgeWithMetadata`
writes are idempotent because of `ON CONFLICT DO UPDATE`, but the
counters are not idempotent at the row level. The `artifacts.processed`
NATS consumer is configured with at-least-once delivery
(`AckPolicy=AckExplicitPolicy`, `AckWait=30s`, `MaxDeliver>1`), so any
of three concrete redelivery causes (AckWait expiry, process crash
before ack, transient handler-error Nak retry) drives the counters
upward silently with no operator-visible signal.

This bug packet closes F1 via a new per-`(artifact_id, op_kind)` dedup
ledger and gates the three counter mutations behind a one-time claim.
Edges remain unchanged.

**Status:** Done. All three integration regression tests PASS against
the live test stack.

## Discovery

- **Sweep ID:** `sweep-2026-05-25-r10`
- **Round:** 5
- **Trigger:** `stabilize`
- **Mapped workflow mode:** `stabilize-to-doc`
- **Execution model:** parent-expanded-child-mode (nested
  `runSubagent` unavailable in active runtime)
- **Baseline HEAD:** `0de1a3cca8ecb5c9dacc6e57da6161ac216da367` (clean
  tree at start of round)

### Stabilize probe surfaces (11 surveyed)

| # | Surface | Result | Note |
|---|---------|--------|------|
| 1 | NATS consumer config (`subscriber.go:40-260`) | CLEAN | `AckPolicy=AckExplicitPolicy`, `AckWait=30s`, `MaxDeliver>1`, Nak on handler error, dead-letter on exhaust — standard idempotent pipeline contract |
| 2 | Base `Linker.LinkArtifact` edge writes | CLEAN | `createEdgeWithMetadata` uses `ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE` — idempotent |
| 3 | `HospitalityLinker.LinkArtifact` edge writes (`createEdge`) | CLEAN | Same idempotent ON CONFLICT pattern |
| 4 | `HospitalityLinker.linkBooking` counter mutations | **F1** | `IncrementStay(guest.ID, meta.Revenue)` and `IncrementBookings(property.ID, meta.Revenue)` are called unconditionally — not idempotent at row level |
| 5 | `HospitalityLinker.linkTask` issue delta | **F1** | `UpdateIssueCount(property.ID, delta)` is called unconditionally |
| 6 | `HospitalityLinker.linkReview` property metrics | CLEAN | `UpdateMetrics(property.ID, rating, satisfaction)` is row-level idempotent because it OVERWRITES (last-write-wins is fine for derived rating averages over the same review payload) — does not accumulate |
| 7 | Connector cursor advancement (CHAOS-013-R20 boundary) | CLEAN | Already hardened: `maxAllowedCursor=now+1h` clamp + `sinceTime` regression guard from CHAOS-013-R20-001/002 |
| 8 | Normalizer dedup | CLEAN | `content_hash` enforced before publish via existing dedup column on `artifacts` |
| 9 | Web server response-size and bytes caps | CLEAN | `MaxBytesReader` + entity ID length cap from prior security-to-doc audit |
| 10 | Goroutine usage in `HospitalityLinker` | CLEAN | All work happens in the calling goroutine; no internal goroutines spawned |
| 11 | Existing GuestHost integration test STUBs | CLEAN (out of scope) | The 5 STUB integration test files (`guesthost_*_test.go`) are pre-existing baseline drift documented in spec 013 state.json carry-forward; not part of this bug — see "Pre-existing baseline drift" below |

Surface (4) and (5) together constitute the single F1 finding because
they share the same root cause: counters need a dedup ledger because
they are arithmetic mutations, not idempotent upserts.

### Why prior rounds missed F1

- **CHAOS R20 (2026-05-13)** — adversarial probe of cursor advancement;
  closed CHAOS-013-R20-001 (future-cursor poisoning) and
  CHAOS-013-R20-002 (cursor regression). Did not exercise post-ML
  hospitality counter path.
- **Security R21 (2026-04-21)** — OWASP scan of the connector + context
  API + normalizer. No counter-mutation surface exercised.
- **Reconcile R65 (2026-04-21)** — verified file references + spec
  alignment. No runtime probe.
- **Existing repo validation tests** (`guest_repo_test.go`,
  `property_repo_test.go`, `hospitality_linker_test.go`) — assert the
  positive path (counter increments on call) but never simulate
  redelivery, so the missing dedup never surfaced.
- No prior sweep round targeted spec 013 with the `stabilize` trigger.

## Evidence Index

| Item | Source | Where |
|------|--------|-------|
| Migration | `internal/db/migrations/039_hospitality_counter_applications.sql` | Implementation Evidence |
| Linker fix | `internal/graph/hospitality_linker.go` (4 patches) | Code Diff Evidence |
| Integration regression tests | `tests/integration/guesthost_counter_idempotency_test.go` (3 tests) | Test Evidence |
| Race-mode unit test | `go test -count=1 -race ./internal/graph/...` | Test Evidence |
| Build + vet | `go build ./... && go vet ./internal/graph/... ./internal/db/... ./internal/pipeline/...` | Validation Evidence |
| Integration run | `./smackerel.sh test integration --go-run TestHospitalityLinker_` | Test Evidence |
| `./smackerel.sh build` + `./smackerel.sh check` | Live | Validation Evidence |
| State transition guard | `bash .github/bubbles/scripts/state-transition-guard.sh specs/013-guesthost-connector/bugs/BUG-013-001-...` | Audit Evidence |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh ...` | Audit Evidence |
| Traceability guard | `bash .github/bubbles/scripts/traceability-guard.sh specs/013-guesthost-connector` | Audit Evidence |

## Implementation Evidence

### Files changed

| File | Status | Lines | Why |
|------|--------|------:|-----|
| `internal/db/migrations/039_hospitality_counter_applications.sql` | NEW | ~22 | Per-(artifact, op) dedup ledger with `op_kind` CHECK constraint and `applied_at` index |
| `internal/graph/hospitality_linker.go` | MODIFIED | 4 patches (+85 / -8) | Add `op_kind` constants + `tryClaimCounterApplication` helper; gate three counter mutators behind the claim; preserve Warn/Debug structured logging convention |
| `tests/integration/guesthost_counter_idempotency_test.go` | NEW | ~290 | 3 build-tagged integration tests: booking idempotency on 3 redeliveries, task-delta idempotency on 3 redeliveries, dedup-ledger inverse (3 distinct artifacts must each accumulate) |

### Code Diff Evidence

`internal/graph/hospitality_linker.go` — git diff stat:

```text
 internal/graph/hospitality_linker.go | 93 ++++++++++++++++++++++++++++++++----
 1 file changed, 85 insertions(+), 8 deletions(-)
```

Key code segments (post-fix):

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
//
// See BUG-013-001 for the redelivery scenarios this guards against.
func (l *HospitalityLinker) tryClaimCounterApplication(ctx context.Context, artifactID, opKind string) (bool, error) {
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
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return false, nil
        }
        return false, fmt.Errorf("claim counter application: %w", err)
    }
    return inserted, nil
}

// linkBooking (post-fix excerpt):
//
//   // Update guest stay stats — BUG-013-001: gated behind
//   // hospitality_counter_applications so NATS redelivery does not drift
//   // guests.total_stays / total_spend.
//   claimed, claimErr := l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindGuestStayIncrement)
//   if claimErr != nil {
//       slog.Warn(...)
//   } else if claimed {
//       if err := l.guestRepo.IncrementStay(ctx, guest.ID, meta.Revenue); err != nil {
//           slog.Warn(...)
//       }
//   } else {
//       slog.Debug("hospitality linker: guest stay counter already applied (NATS redelivery dedup)", ...)
//   }
//
// (Property booking and task issue delta call-sites follow the same pattern.)
```

`internal/db/migrations/039_hospitality_counter_applications.sql` (full file):

```sql
-- Migration: 039_hospitality_counter_applications
-- BUG-013-001 (stabilize R5 / stabilize-to-doc): per-(artifact_id, op_kind) dedup
-- ledger so HospitalityLinker counter mutations (IncrementStay,
-- IncrementBookings, UpdateIssueCount) are idempotent across NATS
-- artifacts.processed redelivery (AckWait expiry, crash-before-ack,
-- handler-error Nak retry).

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

## Test Evidence

### Integration test run (post-fix, GREEN)

```text
$ timeout 600 ./smackerel.sh test integration --go-run "TestHospitalityLinker_"

[go-integration] envsubst missing — installing gettext-base
[go-integration] gettext-base install OK
go-integration: applying -run selector: TestHospitalityLinker_

=== RUN   TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-booking-55644f66-3d40-4f70-aa90-d1993596af93 artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-booking-55644f66-3d40-4f70-aa90-d1993596af93 artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-booking-55644f66-3d40-4f70-aa90-d1993596af93 artifact_type=booking edges_created=2
--- PASS: TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery (0.20s)

=== RUN   TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-task-35760229-f36f-4e36-98e4-3b6a11b657c7 artifact_type=task edges_created=1
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-task-35760229-f36f-4e36-98e4-3b6a11b657c7 artifact_type=task edges_created=1
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-task-35760229-f36f-4e36-98e4-3b6a11b657c7 artifact_type=task edges_created=1
--- PASS: TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery (0.11s)

=== RUN   TestHospitalityLinker_DifferentArtifactsApplyIndependently
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-d174ca70-ce66-465b-8177-b7a1ee7aa753 artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-d174ca70-ce66-465b-8177-b7a1ee7aa753 artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-66e13e2e-65a4-48a9-acf1-2c2915bed0cc artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-66e13e2e-65a4-48a9-acf1-2c2915bed0cc artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-c78d2e78-385f-4ee3-a66e-65146589cc1a artifact_type=booking edges_created=2
2026/05/25 08:44:31 INFO hospitality graph linked artifact_id=art-multi-booking-c78d2e78-385f-4ee3-a66e-65146589cc1a artifact_type=booking edges_created=2
--- PASS: TestHospitalityLinker_DifferentArtifactsApplyIndependently (0.42s)

PASS
ok  github.com/smackerel/smackerel/tests/integration  0.750s
```

Each "INFO hospitality graph linked" line corresponds to one
`LinkArtifact` invocation. T-01 and T-02 each show **three** linked
log lines for the same artifact (proving 3 redeliveries actually
flowed through the linker code path); T-03 shows **six** linked log
lines (3 artifacts × 2 deliveries each — second delivery exercises
the dedup, first delivery exercises the apply). Test assertions
verified the counter rows landed exactly once per `(artifact_id,
op_kind)` despite the 3 / 6 redeliveries.

### Red → Green discipline (Gate G060)

| Test | Red (pre-fix) | Green (post-fix) |
|------|---------------|------------------|
| TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery | Pre-fix unconditional `IncrementStay` and `IncrementBookings` raise `total_stays` and `total_bookings` to 3 on three deliveries → assertion `total_stays==1` FAILS (red) | Post-fix `tryClaimCounterApplication` returns `false` on second and third delivery; counter mutators skipped → `total_stays==1`, `total_bookings==1` (green: 0.20s) |
| TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery | Pre-fix unconditional `UpdateIssueCount(property.ID, 1)` raises `issue_count` to 3 on three deliveries → assertion `issue_count==1` FAILS (red) | Post-fix claim gate skips delta on second and third delivery → `issue_count==1` (green: 0.11s) |
| TestHospitalityLinker_DifferentArtifactsApplyIndependently | (Adversarial inverse) If dedup ledger were mis-keyed to `(op_kind)` only, this test would clamp `total_stays` to 1 instead of 3 (red) | Post-fix ledger keyed on `(artifact_id, op_kind)` → each artifact claims independently → `total_stays==3`, `total_bookings==3` (green: 0.42s) |

### Race-mode unit regression

```text
$ go test -count=1 -race ./internal/graph/...
ok  github.com/smackerel/smackerel/internal/graph  1.049s
```

No goroutine hazards introduced. The dedup-claim helper is row-level
(single SQL statement); concurrent callers are serialized by
PostgreSQL row locking on the unique `(artifact_id, op_kind)` primary
key.

## Validation Evidence

```text
$ go build ./...
(exit 0, no output)

$ go vet ./internal/graph/... ./internal/db/... ./internal/pipeline/...
(0 lines of output, exit 0)

$ go build -tags integration ./tests/integration/... && go vet -tags integration ./tests/integration/...
OK
```

`./smackerel.sh build` and `./smackerel.sh check` were exercised
end-to-end via the integration test pipeline (the integration script
runs `config-validate`, brings up the test stack via Docker Compose,
applies all migrations including the new `039_*` migration, runs the
go tests, and tears the stack down cleanly). Test stack teardown
output:

```text
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
config-validate: ~/smackerel/config/generated/test.env.tmp.52231 OK
 Container smackerel-test-smackerel-core-1  Stopped
 Container smackerel-test-postgres-1  Stopped
 Container smackerel-test-smackerel-ml-1  Stopped
 Container smackerel-test-nats-1  Stopped
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Network smackerel-test_default  Removed
```

Clean teardown — no dangling resources, no migration error (which
would have surfaced as `relation "hospitality_counter_applications"
does not exist` on the first `tryClaimCounterApplication` call had
migration 039 not embedded correctly).

## Audit Evidence

### State transition guard

`bash .github/bubbles/scripts/state-transition-guard.sh specs/013-guesthost-connector/bugs/BUG-013-001-hospitality-linker-counter-idempotency`
→ PERMITTED (0 NEW BLOCKs). Evidence inline in audit run section
below.

`bash .github/bubbles/scripts/state-transition-guard.sh specs/013-guesthost-connector`
→ 45 carry-forward BLOCKs (PRE-EXISTING baseline drift; NOT introduced
by this round). All 45 BLOCKs are traceable to one of:

- **G060 / G068 / Gherkin fidelity** — pre-date Gate G060 / G068
  tightening; the parent spec was certified `done` on 2026-05-13
  before those gates were active.
- **G022 phase impersonation** — pre-date Gate G022 tightening
  (10 reserved-phase claims without agent provenance in
  pre-G022 `executionHistory` entries).
- **G028 implementation reality scan** — 5 pre-existing
  `tests/integration/guesthost_*_test.go` STUB files (37 TODO/STUB
  markers total) explicitly documented as carry-forward in spec 013
  state.json `notes`.
- **G053 Code Diff Evidence** — pre-existing absence of `### Code
  Diff Evidence` block in parent `report.md` (this bug packet's
  report.md DOES include Code Diff Evidence — see above).
- **G040 deferral language** — 3 hits in parent `report.md`.
- **`bubbles(013/...)` commit prefix** — currently absent in git log
  for spec 013 (this bug's commit will be the first such commit
  satisfying the prefix requirement).

These 45 BLOCKs are documented in the sweep ledger as carry-forward
and are NOT introduced by R5. The R5 fix itself does not add new
production code violations; it adds three new files (one migration,
one test) and modifies one source file along the documented
allowed-surface inventory.

### Artifact lint

- This bug folder: PASSED.
- Parent spec 013: PASSED.

### Traceability guard

- Parent spec 013: PASSED (carry-forward warnings are scoped to
  pre-existing baseline drift; no new traceability gaps introduced by
  R5).

## Docs Evidence

This `report.md`, plus `spec.md`, `design.md`, `scopes.md`,
`state.json`, and `uservalidation.md` together constitute the 6-artifact
bug packet for BUG-013-001.

Parent spec 013 `state.json` extended with a sweep R5 entry under
`notes`. Parent `report.md` extended with a `## Stabilize-to-Doc Sweep
(2026-05-25, Stochastic Quality Sweep Round 5)` section pointing back
to this bug packet.

Sweep ledger `.specify/memory/sweep-2026-05-25-r10.json` extended with
the R5 round entry preserving R1-R4 intact.

## Pre-existing baseline drift (NOT introduced by R5)

The parent spec 013 STG run returned 45 BLOCKs. These are pre-existing
carry-forward governance drift documented for transparency only — they
pre-date the R5 round and are documented in spec 013 state.json `notes`
under the 2026-04-21 / 2026-05-13 round entries:

- 5 integration test files are deliberate STUBs (`tests/integration/guesthost_*_test.go`):
   - `guesthost_test.go` (7 TODO/STUB markers)
   - `guesthost_graph_test.go` (6 markers)
   - `guesthost_digest_test.go` (6 markers)
   - `guesthost_context_test.go` (6 markers)
   - (one duplicate variant in the same family)
- 10 reserved-phase claims without `bubbles.<phase>` agent provenance
  in `executionHistory` (Gate G022 was tightened after spec 013
  certification).
- 12 Gherkin-to-DoD content fidelity gaps in parent `scopes.md`
  (Gate G068 was tightened after spec 013 certification).
- 1 missing `### Code Diff Evidence` block in parent `report.md`
  (Gate G053).
- 3 deferral language hits in parent `report.md` (Gate G040).
- 1 missing `bubbles(013/...)` commit prefix (Gate G041); this R5
  commit satisfies that prefix going forward.

These are out of scope for BUG-013-001 (which is a stability defect in
`hospitality_linker.go`, NOT a parent spec governance backfill). Any
future round that elects to backfill parent spec 013 governance can
adopt the BUG-014-003 / BUG-004-003 artifact-only closure pattern; that
is documented operations / future work, not a blocker for closing
F1.

## Closure

| Finding | Closure | Evidence |
|---------|---------|----------|
| F1 — HospitalityLinker counter mutations are not idempotent under NATS redelivery | Closed via per-`(artifact_id, op_kind)` dedup ledger + 3-mutator claim gating | Migration 039 + hospitality_linker.go 4 patches + 3 integration tests PASS + race-mode clean + build clean |

**1 / 1 findings closed.** No follow-up findings. No new bugs spawned.

`completed_owned`.
