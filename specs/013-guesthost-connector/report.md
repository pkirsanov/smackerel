# Report: 013 — GuestHost Connector & Hospitality Intelligence

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Feature 013 implements the GuestHost connector for Smackerel, adding hospitality-aware graph intelligence, domain-specific digests, and a context enrichment API. This report tracks execution evidence for each scope.

**Last reconciled:** 2026-04-12 by `bubbles.workflow` (reconcile-to-doc, triggered by validate)

### Build Evidence

```
$ ./smackerel.sh build
# Docker images build successfully (Go core + ML sidecar)
# Exit: 0 — All checks passed!
```

```
$ ./smackerel.sh test unit
# All 33 packages pass (including guesthost, graph, digest, api, db)
# Exit: 0
```

### Implementation Files Verified

| File | Exists | Lines |
|------|--------|-------|
| `internal/connector/guesthost/types.go` | Yes | Structs for all 9 types |
| `internal/connector/guesthost/client.go` | Yes | Client, NewClient, Validate, FetchActivity |
| `internal/connector/guesthost/connector.go` | Yes | Full Connector interface impl |
| `internal/connector/guesthost/normalizer.go` | Yes | NormalizeEvent for all 11 event types |
| `internal/graph/hospitality_linker.go` | Yes | HospitalityLinker, LinkArtifact, SeedHospitalityTopics |
| `internal/db/guest_repo.go` | Yes | GuestRepository with UpsertByEmail, FindByEmail, IncrementStay |
| `internal/db/property_repo.go` | Yes | PropertyRepository with UpsertByExternalID, IncrementBookings |
| `internal/db/migrations/011_add_guests_properties.sql` | Yes | CREATE TABLE guests, properties |
| `internal/digest/hospitality.go` | Yes | HospitalityDigestContext, AssembleHospitalityContext |
| `internal/api/context.go` | Yes | ContextHandler, HandleContextFor, build*Context |
| `config/smackerel.yaml` (guesthost section) | Yes | enabled, base_url, api_key, sync_schedule, event_types |

### Unit Test Files Verified

| File | Exists | Test Count |
|------|--------|------------|
| `internal/connector/guesthost/client_test.go` | Yes | 11 tests |
| `internal/connector/guesthost/connector_test.go` | Yes | 6 tests |
| `internal/connector/guesthost/normalizer_test.go` | Yes | 9 tests |
| `internal/digest/hospitality_test.go` | Yes | 18 tests |
| `internal/api/context_test.go` | Yes | 9 tests |
| **Total unit tests for 013** | | **53 tests** |

### Missing Test Files (Planned But Not Created)

| Planned File | Status | Planned Tests |
|-------------|--------|---------------|
| `internal/graph/hospitality_linker_test.go` | **MISSING** | T-3-06 through T-3-12 (7 tests) |
| `internal/db/guest_repo_test.go` | **MISSING** | T-3-01 through T-3-03 (3 tests) |
| `internal/db/property_repo_test.go` | **MISSING** | T-3-04, T-3-05 (2 tests) |
| `tests/integration/guesthost_test.go` | **MISSING** | T-1-12, T-1-13, T-2-15 through T-2-17 (5 tests) |
| `tests/integration/guesthost_graph_test.go` | **MISSING** | T-3-13 through T-3-16 (4 tests) |
| `tests/integration/guesthost_digest_test.go` | **MISSING** | T-4-11 through T-4-13 (3 tests) |
| `tests/integration/guesthost_context_test.go` | **MISSING** | T-5-13 through T-5-15 (3 tests) |
| `tests/e2e/guesthost_test.go` | **MISSING** | T-2-18, T-2-19, T-3-17, T-3-18, T-4-14, T-5-16, T-5-17 (7 tests) |
| `internal/intelligence/hospitality.go` | **MISSING** | Logic inlined in context.go (functional equivalent) |
| **Total missing planned tests** | | **34 tests** |

---

## Scope 01: GH Connector — API Client, Types & Config

### Test Evidence

- 11 unit tests in `client_test.go`: auth header, validate success/401/403, URL construction, hasMore pagination, retry on 429, max retries 429, retry on 500, empty cursor omits since, pagination flow
- 6 unit tests in `connector_test.go`: config validation
- All pass via `./smackerel.sh test unit` (cached, exit 0)
- **Coverage:** Scenarios SCN-GH-001 through SCN-GH-007 covered by unit tests
- **Gap:** No integration test file (`tests/integration/guesthost_test.go`) — T-1-12, T-1-13 not implemented

### Status: Scope 1 implementation complete. Unit test coverage adequate for core scenarios.

---

## Scope 02: GH Connector — Implementation & Normalizer

### Test Evidence

- 6 unit tests in `connector_test.go`: connector ID, connect valid/invalid, sync no events, cursor advancement, health transitions
- 9 unit tests in `normalizer_test.go`: booking.created, review.received, message.received, task.created, expense.created, all event types, content hash consistency
- All pass via `./smackerel.sh test unit` (cached, exit 0)
- Connector registered in `cmd/core/main.go`
- **Coverage:** Scenarios SCN-GH-008 through SCN-GH-018 covered by unit tests
- **Gap:** No integration tests (T-2-15 through T-2-17), no e2e tests (T-2-18, T-2-19)

### Status: Scope 2 implementation complete. Unit test coverage adequate.

---

## Scope 03: Hospitality Graph Nodes & Linker

### Test Evidence

- `hospitality_linker.go`: LinkArtifact, linkBooking (STAYED_AT), linkReview (REVIEWED), linkTask/linkExpense (ISSUE_AT), linkMessage (DURING_STAY), SeedHospitalityTopics all implemented
- `guest_repo.go`: UpsertByEmail, FindByEmail, IncrementStay, UpdateSentiment implemented
- `property_repo.go`: UpsertByExternalID, FindByExternalID, IncrementBookings, UpdateTopics, UpdateIssueCount implemented
- `migrations/011_add_guests_properties.sql`: CREATE TABLE guests, properties with unique constraints
- Build compiles clean
- **CRITICAL GAP: ZERO unit tests for Scope 3 files.**
  - No `hospitality_linker_test.go` (7 planned tests: T-3-06 through T-3-12)
  - No `guest_repo_test.go` (3 planned tests: T-3-01 through T-3-03)
  - No `property_repo_test.go` (2 planned tests: T-3-04, T-3-05)
  - No integration tests (T-3-13 through T-3-16)
  - No e2e tests (T-3-17, T-3-18)
- **Implementation deviation:** 5 hospitality topics seeded (guest-experience, property-maintenance, revenue-management, booking-operations, guest-communication) vs 15 planned in spec

### Status: Implementation code exists but UNTESTED. Scope 3 cannot be considered "Done" without unit test coverage.

---

## Scope 04: Hospitality Digest

### Test Evidence

- 18 unit tests in `hospitality_test.go` covering: arrivals, departures, pending tasks, revenue snapshot, guest alerts, property alerts, empty day handling, no-connector-active behavior, format template, digest context assembly
- All pass via `./smackerel.sh test unit` (cached, exit 0)
- **Coverage:** Scenarios SCN-GH-029 through SCN-GH-036 covered by unit tests
- **Gap:** No integration tests (T-4-11 through T-4-13), no e2e test (T-4-14)
- **Implementation deviation:** Revenue snapshot computes week and month windows only (no 24h window). Per-channel breakdown not separately computed. DoD acknowledges this.

### Status: Scope 4 implementation complete. Unit test coverage exists but integration/e2e missing.

---

## Scope 05: Context Enrichment API

### Test Evidence

- 9 unit tests in `context_test.go` covering: guest context full, property context, booking context, guest not found (404), property not found (404), invalid entity type (400), include parameter filtering, hints generation
- All pass via `./smackerel.sh test unit` (cached, exit 0)
- Route registered in `router.go` with bearer auth middleware
- **Coverage:** Scenarios SCN-GH-037 through SCN-GH-046 covered by unit tests
- **Gap:** No integration tests (T-5-13 through T-5-15), no e2e tests (T-5-16, T-5-17)
- **Implementation deviation:** `internal/intelligence/hospitality.go` not created as a separate file — alert/hint logic inlined in `context.go` (functionally equivalent). Communication hints implement repeat_guest, vip, positive_reviewer rules; early-checkin and direct-booking-% rules NOT implemented. Config uses existing `runtime.auth_token` — no separate `intelligence.hospitality` or `context_api` config sections.

### Status: Scope 5 implementation complete with noted deviations. Unit test coverage exists but integration/e2e missing.

---

## Reconciliation Findings Summary

| # | Finding | Severity | Category |
|---|---------|----------|----------|
| F1 | report.md was completely stale ("Pending" for all scopes) | High | Docs |
| F2 | Zero integration test files exist (8 planned files, 20+ tests) | High | Test gap |
| F3 | Zero e2e test files exist (1 planned file, 7+ tests) | High | Test gap |
| F4 | Zero Scope 3 unit tests (12 planned across 3 files) | High | Test gap |
| F5 | state.json completedScopes was empty despite "Done" scopes | Medium | State drift |
| F6 | uservalidation.md claimed validation for unimplemented features | Medium | Docs |
| F7 | scenario-manifest.json references non-existent test files | Medium | Docs |
| F8 | 5 hospitality topics seeded vs 15 planned | Low | Implementation deviation |
| F9 | Revenue snapshot missing 24h window and per-channel breakdown | Low | Implementation deviation |
| F10 | Communication hints missing early-checkin and direct-booking-% rules | Low | Implementation deviation |
| F11 | No `intelligence.hospitality` or `context_api` config sections | Low | Implementation deviation |

---

## Hardening Findings (R27 — 2026-04-14)

| ID | Finding | Severity | CWE | Fix |
|----|---------|----------|-----|-----|
| H-013-001 | `truncateStr` byte-boundary truncation corrupts multi-byte UTF-8 in artifact titles | High | CWE-838 | Rune-aware truncation with `utf8.RuneStart` walk-back |
| H-013-002 | Non-string `event_types` config (YAML list syntax `[]interface{}`) silently ignored, causing all event types to be fetched instead of filtered set | Medium | SST | Handle `[]interface{}` by joining elements; warn on unrecognized types |
| H-013-003 | `Sync()` sets `HealthSyncing` before cursor validation; cursor parse error returns WITHOUT resetting health, leaving connector permanently stuck in `HealthSyncing` | Medium | — | Call `setHealth(HealthError)` on cursor error path |
| H-013-004 | `Client.baseOrigin` field computed in `NewClient()` but never referenced — dead code in auth/network layer | Low | — | Removed dead field |

### Hardening Evidence

```
$ ./smackerel.sh build
# Exit: 0

$ ./smackerel.sh test unit
# ok github.com/smackerel/smackerel/internal/connector/guesthost 0.315s
# All 33 packages pass
```

### New Regression Tests

| Test | File | Validates |
|------|------|-----------|
| TestTruncateStrUTF8SafeMultibyte | regression_test.go | H-013-001: 3-byte rune boundary respected |
| TestTruncateStrUTF8Safe4ByteChar | regression_test.go | H-013-001: 4-byte emoji boundary respected |
| TestTruncateStrShortStringUnchanged | regression_test.go | H-013-001: no-op when under limit |
| TestTruncateStrExactBoundary | regression_test.go | H-013-001: exact-length passthrough |
| TestTruncateStrTinyLimit | regression_test.go | H-013-001: limit ≤ 3 still valid UTF-8 |
| TestNormalizeEventUTF8TitleSafe | regression_test.go | H-013-001: end-to-end artifact title safety |
| TestSyncEventTypesAsSlice | regression_test.go | H-013-002: YAML list → CSV join |
| TestSyncEventTypesAsString | regression_test.go | H-013-002: string passthrough preserved |
| TestSyncBadCursorResetsHealth | regression_test.go | H-013-003: health not stuck at Syncing |
| TestNewClientNoBaseOriginField | regression_test.go | H-013-004: client still functional without dead field |

---

### Findings Requiring Follow-Up Work

**Must-fix (test coverage):**
- Create `internal/graph/hospitality_linker_test.go` with tests for LinkArtifact, edge creation, topic seeding
- Create `internal/db/guest_repo_test.go` with tests for UpsertByEmail, FindByEmail, IncrementStay
- Create `internal/db/property_repo_test.go` with tests for UpsertByExternalID, IncrementBookings

**Should-fix (integration/e2e):**
- Create integration test files under `tests/integration/`
- Create e2e test file `tests/e2e/guesthost_test.go`

**Nice-to-fix (implementation deviations):**
- Expand topic seeds from 5 to 15
- Add 24h revenue window and per-channel breakdown
- Add early-checkin and direct-booking-% hint rules
- Add `intelligence.hospitality` and `context_api` config sections

---

## Completion Statement

**NOT COMPLETE.** As of 2026-04-14 hardening pass:
- All 5 scopes have implementation code that builds and compiles
- 53 original unit tests + 10 hardening regression tests pass (63 total)
- 4 hardening findings fixed (H-013-001 through H-013-004)
- Scope 3 still has ZERO test coverage — linker, guest_repo, and property_repo are untested
- All integration and e2e test files from the test plan are missing (34 planned tests)
- Several acknowledged implementation deviations from the design doc
- Spec status should remain `in_progress` until test gaps are addressed
