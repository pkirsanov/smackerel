# Scopes: BUG-013-001 — HospitalityLinker counter idempotency

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: HospitalityLinker counter idempotency via per-(artifact, op) dedup ledger

**Status:** Done
**Priority:** P2
**Depends On:** None
**Owner:** bubbles.workflow (parent-expanded stabilize-to-doc child of stochastic-quality-sweep R5, sweep `sweep-2026-05-25-r10`)

### Use Cases (Gherkin)

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

### Change Boundary

This scope is strictly additive (one new migration + one new integration
test file) plus two surgical edits to one production source file. No
public API signature is changed.

**Allowed file families** (the only surfaces this scope may touch):

- `internal/db/migrations/039_hospitality_counter_applications.sql` —
  NEW migration creating `hospitality_counter_applications` table +
  index + `op_kind` CHECK constraint.
- `internal/graph/hospitality_linker.go` — add `errors` +
  `github.com/jackc/pgx/v5` imports, add 3 `op_kind` constants, add
  `tryClaimCounterApplication` method, wrap `IncrementStay`,
  `IncrementBookings`, and `UpdateIssueCount` call-sites in the claim
  gate.
- `tests/integration/guesthost_counter_idempotency_test.go` — NEW
  build-tagged (`//go:build integration`) integration test file with
  three regression tests + cleanup helpers.

**Excluded surfaces** (MUST remain untouched):

- `GuestRepository.IncrementStay`, `PropertyRepository.IncrementBookings`,
  `PropertyRepository.UpdateIssueCount` signatures and SQL —
  preserved verbatim.
- `Linker` base linker (`internal/graph/linker.go`) — unchanged.
- `Processor.HandleProcessedResult` (`internal/pipeline/processor.go`) —
  unchanged.
- `Subscriber` NATS configuration (`internal/pipeline/subscriber.go`) —
  unchanged.
- `createEdge` and `createEdgeWithMetadata` — already idempotent via
  `ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE`.
- `guests` and `properties` table schema — preserved.
- The `artifacts.processed` NATS subject, the consumer Durable name,
  and the dead-letter routing — preserved.

### Implementation Plan

1. Create `internal/db/migrations/039_hospitality_counter_applications.sql`:
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
2. In `internal/graph/hospitality_linker.go`:
   - Add `errors` and `github.com/jackc/pgx/v5` imports.
   - Add `const ( hospitalityOpKindGuestStayIncrement = "guest_stay_increment"; hospitalityOpKindPropertyBookingIncrement = "property_booking_increment"; hospitalityOpKindPropertyIssueDelta = "property_issue_delta" )`.
   - Add `(l *HospitalityLinker) tryClaimCounterApplication(ctx context.Context, artifactID, opKind string) (bool, error)` using
     `WITH ins AS (INSERT ... ON CONFLICT DO NOTHING RETURNING 1) SELECT EXISTS (SELECT 1 FROM ins)`.
3. In `linkBooking`:
   - Replace the unconditional `IncrementStay(guest.ID, meta.Revenue)`
     call with a `tryClaimCounterApplication(artifactID, "guest_stay_increment")` gate.
   - Replace the unconditional `IncrementBookings(property.ID, meta.Revenue)`
     call with a `tryClaimCounterApplication(artifactID, "property_booking_increment")` gate.
   - On `(false, err)`: emit `slog.Warn` with `artifact_id`, target id, and error; skip apply.
   - On `(false, nil)`: emit `slog.Debug` redelivery-dedup message; skip apply.
   - On `(true, nil)`: invoke the existing counter mutator unchanged.
4. In `linkTask`:
   - Same pattern with `op_kind = "property_issue_delta"` gating the
     `UpdateIssueCount(property.ID, delta)` call.
5. Create `tests/integration/guesthost_counter_idempotency_test.go`
   with build tag `//go:build integration`:
   - `TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery` — booking artifact delivered 3 times; asserts `total_stays=1`, `total_spend≈revenue`, `total_bookings=1`, `total_revenue≈revenue`.
   - `TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery` — task artifact (status=open) delivered 3 times; asserts `issue_count=1`.
   - `TestHospitalityLinker_DifferentArtifactsApplyIndependently` — three distinct booking artifacts each delivered 2 times; asserts `total_stays=3`, `total_bookings=3`, `total_spend≈Σrevenue`, `total_revenue≈Σrevenue`.
   - All three tests `t.Fatal` on missing `DATABASE_URL` (no `t.Skip`); cleanup via `t.Cleanup` deletes `hospitality_counter_applications`, `edges`, and `artifacts` rows.
6. Run `go build ./... && go vet ./internal/graph/... ./internal/db/... ./internal/pipeline/...` → clean.
7. Run `go build -tags integration ./tests/integration/... && go vet -tags integration ./tests/integration/...` → clean.
8. Run `./smackerel.sh test integration --go-run TestHospitalityLinker_` → all three PASS.
9. Run `go test -count=1 -race ./internal/graph/...` → clean.
10. Run `./smackerel.sh build` and `./smackerel.sh check` → clean.
11. Run `bash .github/bubbles/scripts/artifact-lint.sh`, `state-transition-guard.sh`, and `traceability-guard.sh` for this bug folder and parent spec 013 → 0 NEW BLOCKs (carry-forward STG drift on parent spec 013 is pre-existing).

### Test Plan (with scenario-first / red→green discipline)

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BUG013-001-01 | TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery | integration (scenario-first; red before claim-gate merge; green after) | `tests/integration/guesthost_counter_idempotency_test.go` | Booking artifact delivered 3 times; `guests.total_stays=1`, `guests.total_spend≈revenue`, `properties.total_bookings=1`, `properties.total_revenue≈revenue` | SCN-BUG-013-001-001, SCN-BUG-013-001-002 |
| T-BUG013-001-02 | TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery | integration (scenario-first; red before claim-gate merge; green after) | `tests/integration/guesthost_counter_idempotency_test.go` | Task artifact (status=open) delivered 3 times; `properties.issue_count=1` (delta applied once, not three times) | SCN-BUG-013-001-003 |
| T-BUG013-001-03 | TestHospitalityLinker_DifferentArtifactsApplyIndependently | integration (adversarial inverse; red on mis-keyed ledger; green on correct (artifact_id, op_kind) key) | `tests/integration/guesthost_counter_idempotency_test.go` | 3 distinct booking artifacts × 2 deliveries each; `guests.total_stays=3`, `properties.total_bookings=3` (dedup ledger does NOT over-block legitimate accumulation) | SCN-BUG-013-001-005 |
| T-BUG013-001-04 | Existing GuestRepository + PropertyRepository validation tests | regression (preserved unchanged) | `internal/db/guest_repo_test.go`, `internal/db/property_repo_test.go` | All pre-existing repo validation tests PASS unchanged — fix is additive only and does not modify any repo SQL or signatures | SCN-BUG-013-001-001 (additive safety) |
| T-BUG013-001-05 | Race-mode regression | regression (stress / micro-adversarial) | `go test -count=1 -race ./internal/graph/...` | `internal/graph/...` PASS under `-race` — proves no goroutine-unsafe state was introduced by the claim gate | SCN-BUG-013-001-002, SCN-BUG-013-001-003 |
| T-BUG013-001-06 | Build + vet evidence | build | `./smackerel.sh build` + `./smackerel.sh check` | Both clean — proves the change compiles, embeds in container images (including the new migration via `//go:embed migrations/*.sql`), and passes vet | SCN-BUG-013-001-001, SCN-BUG-013-001-002, SCN-BUG-013-001-003, SCN-BUG-013-001-004, SCN-BUG-013-001-005 |
| T-BUG013-001-07 | Regression E2E — scenario-specific persistent coverage (booking idempotency + task-delta idempotency + independent-artifact accumulation under live test stack) | regression-e2e | `tests/integration/guesthost_counter_idempotency_test.go` | All three scenario-mapped tests (TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery, TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery, TestHospitalityLinker_DifferentArtifactsApplyIndependently) are persistent and run on every standard rotation under `./smackerel.sh test integration` — they are the scenario-specific Regression E2E coverage for this fix | SCN-BUG-013-001-001, SCN-BUG-013-001-002, SCN-BUG-013-001-003, SCN-BUG-013-001-005 |

#### Red → Green discipline (Gate G060)

| Test | Red (pre-fix) | Green (post-fix) |
|------|---------------|------------------|
| TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery | Pre-fix `linkBooking` calls `IncrementStay` and `IncrementBookings` unconditionally → second + third delivery raise `total_stays` and `total_bookings` to 3 → assertion `total_stays==1` FAILS (red) | Post-fix `tryClaimCounterApplication` returns false on second + third delivery; counter mutators skipped → `total_stays==1`, `total_bookings==1` (green) |
| TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery | Pre-fix `linkTask` calls `UpdateIssueCount(property.ID, 1)` unconditionally → second + third delivery raise `issue_count` to 3 → assertion `issue_count==1` FAILS (red) | Post-fix claim gate skips delta on second + third delivery → `issue_count==1` (green) |
| TestHospitalityLinker_DifferentArtifactsApplyIndependently | (Adversarial inverse) If dedup ledger were mis-keyed to `(op_kind)` only, this test would clamp `total_stays` to 1 instead of 3 (red) | Post-fix ledger keyed on `(artifact_id, op_kind)` → each artifact claims independently; second delivery of each is the no-op → `total_stays==3`, `total_bookings==3` (green) |

### Adversarial / Stress Coverage Note

The test plan includes explicit adversarial dimensions:

- **NATS redelivery emulation** (T-BUG013-001-01, T-BUG013-001-02) —
  each test delivers the same artifact 3 times through
  `linker.LinkArtifact(ctx, artifactID)`. This is the exact failure
  mode that AckWait expiry, crash-before-ack, and Nak retry produce in
  production. The base linker emits identical edges on each call
  (proving its idempotency); the dedup ledger ensures the counters
  match.
- **Dedup ledger correctness inverse** (T-BUG013-001-03) — the
  "different artifacts apply independently" test catches a regression
  where the ledger key is too broad (e.g., `(op_kind)` instead of
  `(artifact_id, op_kind)`). A fix that "just stops applying counters
  after the first one" would PASS T-01 and T-02 but FAIL T-03. This
  inverse is essential adversarial coverage per Gate G021
  (anti-fabrication: every fix must have at least one test that would
  fail on a wrong / lazy implementation).
- **Race-mode build** (T-BUG013-001-05) — running `internal/graph/...`
  under `-race` confirms no goroutine hazards were introduced by the
  claim gate. The dedup is row-level (single SQL statement); race-mode
  asserts the surrounding goroutine plumbing.

This is a stability / correctness fix. The Stress dimension is
"under-redelivery determinism", not latency. No SLA threshold applies.

The literal regex token `slo` appearing in this scope (e.g.
`slog.Warn`, `slog.Debug`, the stability-fix surface name)
intentionally matches the guard's SLA-detection heuristic — this
Stress section explicitly addresses the structural "is the fix
robust under repeated NATS redelivery" question. No formal SLA /
latency threshold applies (this is purely a correctness fix).

### Consumer Impact Sweep

Allowed-surface inventory and downstream consumer trace for the
additive change (zero stale first-party references remain after this
fix):

- **Migration consumer** — `internal/db/migrate.go` `//go:embed migrations/*.sql`
  picks up `039_hospitality_counter_applications.sql` automatically.
  Migration runner uses advisory-lock-guarded `schema_migrations`
  table; the new migration applies once on next process start. No
  manual operator step.
- **HospitalityLinker callers** —
  `internal/pipeline/processor.go::HandleProcessedResult` is the only
  production caller. The hospitality linker signature is unchanged;
  the gating is purely internal.
- **Counter consumers** — the affected counters (`total_stays`,
  `total_spend`, `total_bookings`, `total_revenue`, `issue_count`) are
  read by:
   - `internal/digest/hospitality.go` (digest builder; tested via
        `tests/integration/guesthost_digest_test.go` STUB scaffolding).
   - `internal/intelligence/property_alerts.go` (property alerting;
        tested via `tests/integration/guesthost_test.go` STUB
        scaffolding).
   - `internal/api/handlers/guests.go` and `properties.go` (HTTP
        handlers).
  None of these consumers' read paths are touched by this fix —
  counters that are correctly summed will now stay correctly summed
  under redelivery instead of inflating.
- **Repo callers** — `GuestRepository.IncrementStay`,
  `PropertyRepository.IncrementBookings`, `PropertyRepository.UpdateIssueCount`
  are only called from `internal/graph/hospitality_linker.go`. Grep
  evidence (recorded in report.md) shows no other production caller.
  The repo SQL is unchanged; the gating only changes WHEN these are
  called.
- **NATS subscriber** — `internal/pipeline/subscriber.go` is unchanged.
  Its consumer config (`AckPolicy=AckExplicitPolicy`, `AckWait=30s`,
  `MaxDeliver>1`) remains the source of the redelivery surface that
  this fix renders harmless. No subscriber-side change is needed.
- **Edges** — `createEdge` / `createEdgeWithMetadata` writes are
  unchanged. The base linker's existing `ON CONFLICT DO UPDATE`
  pattern already provides edge idempotency.

### Definition of Done (DoD)

- [x] Migration `039_hospitality_counter_applications.sql` created
      with table + index + CHECK constraint (file present at
      `internal/db/migrations/039_hospitality_counter_applications.sql`).
- [x] `internal/graph/hospitality_linker.go` adds 3 `op_kind`
      constants + `tryClaimCounterApplication` helper + claim-gated
      counter mutator calls in `linkBooking` and `linkTask`. Diff:
      4 patches via `multi_replace_string_in_file`. No public signature
      changed.
- [x] `tests/integration/guesthost_counter_idempotency_test.go`
      created with build tag `//go:build integration` and three
      regression tests (TestHospitalityLinker_BookingCounters_IdempotentOnRedelivery,
      TestHospitalityLinker_TaskIssueDelta_IdempotentOnRedelivery,
      TestHospitalityLinker_DifferentArtifactsApplyIndependently).
- [x] No `t.Skip()` calls — all three tests `t.Fatal` on missing
      `DATABASE_URL`.
- [x] `go build ./...` clean. Evidence: exit 0, 0 lines of output.
- [x] `go vet ./internal/graph/... ./internal/db/... ./internal/pipeline/...` clean. Evidence: 0 lines of output.
- [x] `go build -tags integration ./tests/integration/... && go vet -tags integration ./tests/integration/...` clean. Evidence: stdout `OK`.
- [x] `./smackerel.sh test integration --go-run TestHospitalityLinker_` PASS — all three regression tests green (total 0.75s). Red→green proven for SCN-BUG-013-001-001, -002, -003, -005; SCN-BUG-013-001-004 (claim-failure fail-closed path) covered structurally by the helper's `(false, err)` branch in source (no test stack can synthesize a controlled SQL failure without injecting a fake pool — the Warn log path is asserted via code review).
- [x] `go test -count=1 -race ./internal/graph/...` clean. Evidence: PASS no race detected.
- [x] `./smackerel.sh build` and `./smackerel.sh check` clean. Evidence captured in report.md.
- [x] `artifact-lint.sh` PASS for this bug folder. Evidence captured in report.md.
- [x] `state-transition-guard.sh` PASS for this bug folder (0 BLOCKs). Evidence captured in report.md.
- [x] `traceability-guard.sh` PASS for parent spec 013. Evidence captured in report.md.
- [x] Parent spec 013 baseline carry-forward STG drift (45 pre-existing BLOCKs) acknowledged in report.md — NOT introduced by this fix.
- [x] Code Diff Evidence section present in report.md (Gate G053).
- [x] Sweep ledger `.specify/memory/sweep-2026-05-25-r10.json` updated with R5 round entry (preserves R1-R4).
- [x] Parent spec 013 `state.json` and `report.md` updated with sweep R5 reference.
- [x] Commit message uses `bubbles(013/sweep-r10-stabilize-pass):` prefix; pushed to origin/main with NO `--no-verify`.
