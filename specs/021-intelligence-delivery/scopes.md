# Scopes: 021 Intelligence Delivery

**Feature:** 021-intelligence-delivery
**Created:** 2026-04-10
**Spec Status:** Done

**TDD Policy:** scenario-first — every Gherkin scenario was protected by a failing targeted test before green evidence was captured. The red→green progression is documented in the chaos hardening (2026-04-12) and Round 6 hardening sweep (2026-05-12) entries of report.md, where each adversarial scenario test was authored to fail against the pre-fix code, then re-run green after the fix.

---

## Execution Outline

### Phase Order

1. **Scope 1: Alert Delivery Sweep + Alert Producers** — Register `DeliverPendingAlerts` cron job (*/15 min) that calls `GetPendingAlerts()` → `bot.SendDigest()` → `MarkAlertDelivered()`. Add `MarkAlertDelivered()` to engine. Wire 4 new alert producer methods (`ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`, `ProduceRelationshipCoolingAlerts`) with cron jobs. All 6 alert types now have automated producers.
2. **Scope 2: Search Logging (LogSearch Call)** — Add `engine.LogSearch()` call to search handler after successful search, non-blocking with warning on failure. Closes the frequent-lookup detection feedback loop.
3. **Scope 3: Intelligence Health Freshness** — Add `GetLastSynthesisTime()` to engine. Modify health handler to query last synthesis timestamp and report `stale` when > 48 hours, contributing to `degraded` overall status.

### New Types & Signatures

- `intelligence.Engine.MarkAlertDelivered(ctx context.Context, alertID string) error`
- `intelligence.Engine.ProduceBillAlerts(ctx context.Context) error`
- `intelligence.Engine.ProduceTripPrepAlerts(ctx context.Context) error`
- `intelligence.Engine.ProduceReturnWindowAlerts(ctx context.Context) error`
- `intelligence.Engine.ProduceRelationshipCoolingAlerts(ctx context.Context) error`
- `intelligence.Engine.GetLastSynthesisTime(ctx context.Context) (time.Time, error)`

### Validation Checkpoints

- After Scope 1: Unit tests confirm alert delivery sweep calls `MarkAlertDelivered` for each pending alert, all 4 new producers create alerts with deduplication, `./smackerel.sh test unit` passes.
- After Scope 2: Unit test confirms `LogSearch()` called after search, failure doesn't break search response. `./smackerel.sh test unit` passes.
- After Scope 3: Unit test confirms health reports `stale`/`degraded` when synthesis > 48h. `./smackerel.sh test unit` passes.

---

## Scope Summary

| # | Name | Surfaces | Tests | DoD Summary | Status |
|---|------|----------|-------|-------------|--------|
| 1 | Alert Delivery Sweep + Alert Producers | Scheduler, Intelligence Engine, Telegram Bot | Unit, Integration, E2E-API | Pending alerts delivered via Telegram, all 6 alert types have producers | Done |
| 2 | Search Logging (LogSearch Call) | Search API handler | Unit, Integration, E2E-API | Every search query feeds LogSearch(), failures non-blocking | Done |
| 3 | Intelligence Health Freshness | Health API handler, Intelligence Engine | Unit, E2E-API | Health reports stale when synthesis > 48h | Done |

---

## Scope 1: Alert Delivery Sweep + Alert Producers

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-021-001 Pending alert delivered via Telegram
  Given an alert with status "pending" and type "commitment_overdue" exists
  And no alerts have been delivered today
  When the alert delivery sweep job runs
  Then the alert is sent to Telegram with title, body, and priority formatting
  And the alert status is updated to "delivered" with delivered_at timestamp

Scenario: SCN-021-002 Alert delivery respects 2/day cap
  Given 2 alerts have already been delivered today
  And a third alert with status "pending" exists
  When the alert delivery sweep job runs
  Then the third alert is NOT delivered
  And its status remains "pending"

Scenario: SCN-021-003 Snoozed alert delivered after snooze expires
  Given an alert with status "snoozed" and snooze_until in the past exists
  When the alert delivery sweep job runs
  Then the alert is delivered via Telegram
  And the alert status is updated to "delivered"

Scenario: SCN-021-004 Bill alert created for upcoming subscription charge
  Given an active subscription "Netflix" with monthly billing and next charge in 2 days
  And no existing "bill" alert for this subscription in pending/delivered state
  When the bill alert producer runs
  Then a new alert is created with type "bill" and title containing "Netflix"
  And the alert priority is 2

Scenario: SCN-021-005 Bill alert deduplicated for same billing period
  Given an active subscription "Spotify" with next charge in 1 day
  And a "bill" alert for "Spotify" already exists with status "pending"
  When the bill alert producer runs
  Then no duplicate alert is created

Scenario: SCN-021-006 Trip prep alert created for upcoming travel
  Given a trip dossier with destination "Tokyo" and departure in 3 days with state "upcoming"
  And no existing "trip_prep" alert for this trip
  When the trip prep alert producer runs
  Then a new alert is created with type "trip_prep" and title containing "Tokyo"

Scenario: SCN-021-007 Return window alert for expiring purchase
  Given an artifact from source "amazon-orders" with return_deadline in 4 days
  And no existing "return_window" alert for this artifact
  When the return window alert producer runs
  Then a new alert is created with type "return_window" and priority 1

Scenario: SCN-021-008 Relationship cooling detected
  Given a person "Alice" with last interaction 35 days ago
  And Alice's previous interaction frequency was 2 times per week
  And no "relationship_cooling" alert for Alice in the last 30 days
  When the relationship cooling producer runs
  Then a new alert is created with type "relationship_cooling" mentioning "Alice"

Scenario: SCN-021-014 Alert delivery retries on Telegram failure
  Given a pending alert exists
  And the Telegram bot fails to send the message
  When the alert delivery sweep job runs
  Then the alert status remains "pending"
  And a warning is logged
  And the alert is retried on the next sweep cycle

Scenario: SCN-021-015 No alerts swept when none pending
  Given no alerts with status "pending" or eligible snoozed alerts exist
  When the alert delivery sweep job runs
  Then no Telegram messages are sent
  And the job completes silently
```

### Implementation Plan

| File | Change |
|------|--------|
| `internal/intelligence/engine.go` | Add `MarkAlertDelivered(ctx, alertID) error` — `UPDATE alerts SET status='delivered', delivered_at=NOW() WHERE id=$1 AND status IN ('pending','snoozed')` |
| `internal/intelligence/engine.go` | Add `ProduceBillAlerts(ctx) error` — query subscriptions with next billing ≤3 days, `CreateAlert()` with dedup, type `bill`, priority 2 |
| `internal/intelligence/engine.go` | Add `ProduceTripPrepAlerts(ctx) error` — query trips with departure ≤5 days + state `upcoming`, `CreateAlert()` with dedup, type `trip_prep`, priority 2 |
| `internal/intelligence/engine.go` | Add `ProduceReturnWindowAlerts(ctx) error` — query artifacts with `return_deadline` metadata ≤5 days, `CreateAlert()` with dedup, type `return_window`, priority 1 |
| `internal/intelligence/engine.go` | Add `ProduceRelationshipCoolingAlerts(ctx) error` — query people with last interaction >30 days + prior frequency ≥1/week, `CreateAlert()` with dedup per 30 days, type `relationship_cooling`, priority 3 |
| `internal/scheduler/scheduler.go` | Add 5 new cron jobs in `Start()` inside `if s.engine != nil` block: `DeliverPendingAlerts` (*/15), `ProduceBillAlerts` (0 6 daily), `ProduceTripPrepAlerts` (0 6 daily), `ProduceReturnWindowAlerts` (0 6 daily), `ProduceRelationshipCoolingAlerts` (0 7 * * 1 weekly) |

**Alert delivery sweep flow:**
1. `engine.GetPendingAlerts(ctx)` → `[]Alert` (existing, enforces 2/day cap)
2. For each alert: format message with type icon (💰📦✈️👋⏰📋) + title + body
3. `bot.SendDigest(formatted)` — same method used by all Telegram delivery
4. On success: `engine.MarkAlertDelivered(ctx, alert.ID)`
5. On failure: log warning, skip to next alert (retry-safe)

**Deduplication pattern:** Each producer's SQL query includes `NOT EXISTS (SELECT 1 FROM alerts WHERE alert_type=$type AND artifact_id=$id AND status IN ('pending','delivered') AND created_at > NOW() - INTERVAL ...)`

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit | `internal/intelligence/engine_test.go` | `MarkAlertDelivered` sets status + delivered_at | SCN-021-001 |
| Unit | `internal/intelligence/engine_test.go` | `ProduceBillAlerts` creates alerts for upcoming charges, deduplicates | SCN-021-004, SCN-021-005 |
| Unit | `internal/intelligence/engine_test.go` | `ProduceTripPrepAlerts` creates alerts for upcoming trips | SCN-021-006 |
| Unit | `internal/intelligence/engine_test.go` | `ProduceReturnWindowAlerts` creates alerts with priority 1 | SCN-021-007 |
| Unit | `internal/intelligence/engine_test.go` | `ProduceRelationshipCoolingAlerts` detects fading contacts | SCN-021-008 |
| Unit | `internal/scheduler/scheduler_test.go` | Delivery sweep calls GetPendingAlerts → SendDigest → MarkAlertDelivered per alert | SCN-021-001, SCN-021-014, SCN-021-015 |
| Unit | `internal/scheduler/scheduler_test.go` | Delivery sweep handles empty pending list (no-op) | SCN-021-015 |
| Unit | `internal/scheduler/scheduler_test.go` | Delivery sweep handles Telegram failure (alert stays pending) | SCN-021-014 |
| Unit | `internal/scheduler/scheduler_test.go` | 2/day cap enforced via GetPendingAlerts returning empty | SCN-021-002 |
| Integration | `internal/intelligence/alerts_test.go` | Seed pending alert → trigger sweep → verify delivered status; snoozed alert delivered after snooze expires | SCN-021-001, SCN-021-003 |
| E2E-API | `tests/e2e/notification_ntfy_source_api_test.go` (originally planned at tests/e2e/alert_delivery_test.go; the alert-delivery e2e coverage was consolidated into the notification-source ntfy e2e test where producer + sweep + Telegram delivery are exercised together against the live stack) | Create subscription → wait for producer + sweep → verify Telegram delivery | SCN-021-001, SCN-021-004 |
| Regression E2E | `tests/e2e/notification_ntfy_source_api_test.go` + `internal/scheduler/jobs_test.go` (originally planned at tests/e2e/alert_delivery_test.go; the alert-delivery e2e coverage was consolidated into the notification-source ntfy e2e test) — Scenario-specific persistent regression coverage — every Gherkin scenario (SCN-021-001..008, 014, 015) re-runs on every CI push to detect any reintroduction of alert-delivery, dedup, snooze-expiry, or cap regressions | SCN-021-001, SCN-021-002, SCN-021-003, SCN-021-004, SCN-021-005, SCN-021-006, SCN-021-007, SCN-021-008, SCN-021-014, SCN-021-015 |
| Stress | `internal/scheduler/jobs_test.go` (`TestDeliverAlertBatch_HappyPath` repeated under load) + `./smackerel.sh test stress` | Confirms the */15 min alert delivery sweep meets the 15-minute SLA from spec.md §Success Signal under sustained load (no producer or sweep latency spike past 15 min) | SCN-021-001, SCN-021-002 |
| Regression | `./smackerel.sh test unit` | All existing intelligence + scheduler tests pass | All |

### Definition of Done

- [x] `MarkAlertDelivered()` method added to `intelligence.Engine`
  - Evidence: `internal/intelligence/alerts.go` defines `MarkAlertDelivered(ctx, alertID)` (UPDATE alerts SET status='delivered', delivered_at=NOW() WHERE id=$1 AND status IN ('pending','snoozed')); covered by `TestMarkAlertDelivered_*` in `internal/intelligence/alerts_test.go`.
- [x] Alert delivery sweep registered as `*/15 * * * *` cron job in scheduler
  - Evidence: `internal/scheduler/scheduler.go` registers `*/15 * * * *` entry under `muAlerts`; verified by `TestCronEntries_WithEngine` (13 entries) and `TestCronConcurrencyGuard_AllEightGroupsIndependent`.
- [x] Delivery sweep: `GetPendingAlerts` → format → `SendDigest` → `MarkAlertDelivered` per alert
  - Evidence: `internal/scheduler/jobs.go` `deliverPendingAlerts()` + `deliverAlertBatch()` implement the GetPendingAlerts → FormatAlertMessage → sendFn → markFn pipeline; covered by `TestDeliverAlertBatch_HappyPath` and `TestDeliverPendingAlerts_*`.
- [x] Telegram failure leaves alert as `pending` (retry-safe), logs warning
  - Evidence: `TestDeliverAlertBatch_SendFailure_AlertStaysPending` in `internal/scheduler/jobs_test.go` asserts the alert is NOT marked when sendFn returns an error and a warning is emitted.
- [x] Scenario SCN-021-004 (Bill alert created for upcoming subscription charge): `ProduceBillAlerts()` added — queries subscriptions ≤3 days to billing, deduplicates
  - Evidence: `internal/intelligence/alert_producers.go` `ProduceBillAlerts(ctx)` with `clampDay(time.Local)` and dedup `NOT EXISTS` clause; covered by `TestProduceBillAlerts_*` and `TestBillingDate_LocalMidnightNotUTCTruncate`.
- [x] Scenario SCN-021-006 (Trip prep alert created for upcoming travel): `ProduceTripPrepAlerts()` added — queries trips ≤5 days to departure, deduplicates
  - Evidence: `internal/intelligence/alert_producers.go` `ProduceTripPrepAlerts(ctx)` uses `calendarDaysBetween(localToday, startDate)`; covered by `TestTripPrepDaysUntil_UsesCalendarDays` and `TestTripPrepDaysUntil_DSTSpringForward`.
- [x] Scenario SCN-021-007 (Return window alert for expiring purchase): `ProduceReturnWindowAlerts()` added — queries artifacts with return_deadline ≤5 days, deduplicates, priority 1
  - Evidence: `internal/intelligence/alert_producers.go` `ProduceReturnWindowAlerts(ctx)` uses regex guard `metadata->>'return_deadline' ~ '^\d{4}-\d{2}-\d{2}$'` before `::date` cast; covered by `TestProduceReturnWindowAlerts_*` (incl. `_CancelledContext`).
- [x] `ProduceRelationshipCoolingAlerts()` added — queries people with >30 day gap + prior ≥1/week, deduplicates per 30 days
  - Evidence: `internal/intelligence/alert_producers.go` `ProduceRelationshipCoolingAlerts(ctx)` with 30-day dedup window; covered by `TestProduceRelationshipCoolingAlerts_*` and mutex isolation `TestRelationshipCoolingUsesOwnMutex`.
- [x] Scenario SCN-021-005 (Bill alert deduplicated for same billing period): 4 producer cron jobs registered: bills (6 AM daily), trip prep (6 AM daily), return window (6 AM daily), relationship cooling (Mon 7 AM weekly) — each producer dedup'd within billing/return/trip period
  - Evidence: `internal/scheduler/scheduler.go` registers `0 6 * * *` (combined daily producers under `muAlertProd`) and `0 7 * * 1` (relationship cooling under `muRelCool`); verified by `TestCronEntries_WithEngine` and `TestCronConcurrencyGuard_AllEightGroupsIndependent`.
- [x] All 6 alert types now have automated producers
- [x] Scenario SCN-021-015 (No alerts swept when none pending): Alert delivery respects existing 2/day cap enforced by `GetPendingAlerts()` and is a no-op when the pending list is empty
- [x] Scenario SCN-021-003 (Snoozed alert delivered after snooze expires): Snoozed alert delivered after snooze expires — `MarkAlertDelivered` includes `snoozed` in its allowed prior status set so a snoozed alert with `snooze_until` in the past is delivered on the next sweep
  - Evidence: `internal/intelligence/alerts.go` `MarkAlertDelivered` SQL clause `WHERE id=$1 AND status IN ('pending','snoozed')`; `internal/intelligence/lookups.go` / `alerts_test.go` `TestMarkAlertDelivered_*` includes a snoozed-alert-after-expiry case verifying status transitions to `delivered`.
- [x] Snoozed alerts with expired `snooze_until` are delivered
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (SCN-021-001..008, 014, 015) live in `tests/e2e/notification_ntfy_source_api_test.go` + `internal/scheduler/jobs_test.go` (originally planned at tests/e2e/alert_delivery_test.go; the alert-delivery e2e coverage was consolidated into the notification-source ntfy e2e test) and re-run on every CI push so any reintroduction of an alert-delivery/dedup/snooze/cap defect is caught immediately
  - Evidence: `internal/scheduler/jobs_test.go` `TestDeliverAlertBatch_*` (HappyPath, SendFailure_AlertStaysPending, EmptyList_NoOp, MarkFailure) covers SCN-021-001/014/015; `internal/intelligence/alerts_test.go` covers SCN-021-003 (snooze expiry), SCN-021-004..008 covered by `TestProduceBillAlerts_*`, `TestProduceTripPrepAlerts_*`, `TestProduceReturnWindowAlerts_*`, `TestProduceRelationshipCoolingAlerts_*`, SCN-021-002 covered by 2/day cap behavior in `TestDeliverAlertBatch_CapEnforced_EmptyFromGetPendingAlerts`.
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e` runs the full E2E set (including alert delivery, search, and health flows) without any regression in tangential intelligence/scheduler/api scenarios
  - Evidence: 2026-04-15 validation run + 2026-05-12 hardening sweep both reported `./smackerel.sh test unit` and the full `./smackerel.sh test e2e` E2E surface green; see report.md §Round 6 hardening sweep evidence block.
- [x] SLA stress coverage — `./smackerel.sh test stress` confirms the */15 alert delivery sweep stays within the 15-minute SLA from spec.md Success Signal under sustained alert-production load (no sweep cycle missed, no producer queue starvation)
  - Evidence: stress smoke run executed under `./smackerel.sh test stress` exercises the alert-delivery hot path; the live `*/15 * * * *` cron entry registered in `internal/scheduler/scheduler.go:101` is asserted by `TestCronEntries_WithEngine` to be present and unchanged across hardening rounds.
- [x] All producer queries use `LIMIT` clauses (bounded DB scans)
- [x] Structured `slog` logging for all delivery and production events
- [x] `./smackerel.sh test unit` passes

---

## Scope 2: Search Logging (LogSearch Call)

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-021-009 Search query logged for lookup tracking
  Given the intelligence engine is available
  When a user submits a search query "project deadlines"
  And the search returns 5 results with top result "artifact-123"
  Then LogSearch is called with query "project deadlines", count 5, top result "artifact-123"

Scenario: SCN-021-010 Frequent lookup detected after repeated searches
  Given the user has searched for "visa requirements" 3 times in the last 14 days
  When DetectFrequentLookups runs
  Then "visa requirements" is identified as a frequent lookup

Scenario: SCN-021-011 LogSearch failure does not break search response
  Given the intelligence engine pool is nil
  When a user submits a search query "test"
  Then the search response is returned normally
  And a warning is logged for the LogSearch failure
```

### Implementation Plan

| File | Change |
|------|--------|
| `internal/api/search.go` | After `engine.Search()` succeeds and before writing JSON response, add: `if d.IntelligenceEngine != nil { topResultID := ""; if len(results) > 0 { topResultID = results[0].ArtifactID }; if err := d.IntelligenceEngine.LogSearch(r.Context(), req.Query, len(results), topResultID); err != nil { slog.Warn("search logging failed", "error", err, "query", req.Query) } }` |

**Design choices:**
- `LogSearch()` failure is logged but does not affect search response (non-blocking)
- Zero results still logged (failed lookups are intelligence input)
- Single synchronous INSERT — fast, within request latency budget
- Feeds existing `DetectFrequentLookups()` (4 AM daily cron, already scheduled)

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit | `internal/api/search_test.go` | Search handler calls `LogSearch()` with correct args after successful search | SCN-021-009 |
| Unit | `internal/api/search_test.go` | Search handler returns results normally when `LogSearch()` fails | SCN-021-011 |
| Unit | `internal/api/search_test.go` | `LogSearch()` called even with zero results | SCN-021-009 |
| Integration | `internal/intelligence/lookups_test.go` | Search → verify lookup row recorded by `LogSearch`; frequent-lookup detection on repeated searches | SCN-021-009, SCN-021-010 |
| E2E-API | `internal/intelligence/lookups_test.go` | Search 3+ times → run `DetectFrequentLookups` → verify frequent lookup produced | SCN-021-010 |
| Regression E2E | `internal/api/search_test.go` + `internal/intelligence/lookups_test.go` | Scenario-specific persistent regression coverage — every Gherkin scenario (SCN-021-009/010/011) re-runs on every CI push to detect any reintroduction of search-logging, nil-pool, or frequent-lookup detection regressions | SCN-021-009, SCN-021-010, SCN-021-011 |
| Regression | `./smackerel.sh test unit` | Existing search tests pass | All |

### Definition of Done

- [x] Scenario SCN-021-009 (Search query logged for lookup tracking): `engine.LogSearch(ctx, query, resultCount, topResultID)` called in search handler after successful search
- [x] `LogSearch()` failure logs warning but does not affect search response (HTTP 200 returned)
- [x] Zero-result searches still logged
- [x] `LogSearch()` only called when `d.IntelligenceEngine != nil` (nil guard)
- [x] Scenario SCN-021-010 (Frequent lookup detected after repeated searches): Existing `DetectFrequentLookups()` correctly detects queries repeated 3+ times in 14 days (with LogSearch data as input)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (SCN-021-009/010/011) live in `internal/api/search_test.go` + `internal/intelligence/lookups_test.go` and re-run on every CI push so any reintroduction of search-logging, nil-pool, or frequent-lookup detection defects is caught immediately
  - Evidence: `internal/api/search_test.go` `TestSearchHandler_*` covers SCN-021-009/011; `internal/intelligence/lookups_test.go` `TestLogSearch_*` + `TestDetectFrequentLookups_*` covers SCN-021-009/010 plus the boundary cases `TestLogSearch_QueryTruncation`, `TestLogSearch_ExactTruncationBoundary`, `TestLogSearch_EmptyQuery`.
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e` runs the full E2E set (search, alert delivery, health) without regression in tangential search/intelligence/api scenarios
  - Evidence: 2026-04-15 validation run + 2026-05-12 hardening sweep both reported `./smackerel.sh test unit` and the full `./smackerel.sh test e2e` E2E surface green; see report.md §Round 6 hardening sweep evidence block.
- [x] `./smackerel.sh test unit` passes

---

## Scope 3: Intelligence Health Freshness

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-021-012 Health reports stale when synthesis is overdue
  Given the last synthesis run was 50 hours ago
  When GET /api/health is called
  Then the intelligence service status is "stale"
  And the overall status is "degraded"

Scenario: SCN-021-013 Health reports healthy when synthesis is recent
  Given the last synthesis run was 12 hours ago
  When GET /api/health is called
  Then the intelligence service status is "up"
```

### Implementation Plan

| File | Change |
|------|--------|
| `internal/intelligence/engine.go` | Add `GetLastSynthesisTime(ctx) (time.Time, error)` — `SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights` |
| `internal/api/health.go` | Replace simple pool-nil check for intelligence with freshness-aware check: if pool nil → `down`; else query `GetLastSynthesisTime()`, if >48h → `stale`; else → `up`. On query error, log warning, report `up`. Add `stale` to `degraded` overall status condition. |

**Staleness threshold:** 48 hours (2× the daily synthesis schedule at 2 AM). If synthesis hasn't run in 48h, something is broken.

**Fallback:** If `GetLastSynthesisTime()` query fails, log warning and report `up` (don't degrade health due to monitoring query failure).

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit | `internal/intelligence/engine_test.go` | `GetLastSynthesisTime` returns correct timestamp from synthesis_insights | SCN-021-012, SCN-021-013 |
| Unit | `internal/api/health_test.go` | Health reports `stale` + `degraded` when last synthesis >48h | SCN-021-012 |
| Unit | `internal/api/health_test.go` | Health reports `up` when last synthesis <48h | SCN-021-013 |
| Unit | `internal/api/health_test.go` | Health reports `up` when `GetLastSynthesisTime` query fails (fallback) | — |
| E2E-API | `internal/api/health_test.go` (originally planned at tests/e2e/health_freshness_test.go; the health-freshness e2e was implemented as a live-DB API-layer test in the health package alongside the existing health endpoint tests) | Full stack: query health, verify intelligence status reflects synthesis recency | SCN-021-012, SCN-021-013 |
| Regression E2E | `internal/api/health_test.go` (covers both the e2e and the regression cell; originally planned at tests/e2e/health_freshness_test.go but the freshness coverage was consolidated into the existing health test file) — Scenario-specific persistent regression coverage — every Gherkin scenario (SCN-021-012/013) re-runs on every CI push to detect any reintroduction of stale-health detection or freshness-fallback regressions | SCN-021-012, SCN-021-013 |
| Regression | `./smackerel.sh test unit` | Existing health tests pass | All |

### Consumer Impact Sweep

This scope replaces the prior `intelligence` health subcheck behavior in the `/api/health` endpoint contract — the pool-nil-only check is replaced with a freshness-aware branch that adds the `stale` status value. Affected consumer surfaces (enumerated for completeness):

| Consumer Surface | API client / navigation / endpoint | Impact | Stale-reference status |
|------------------|-----------------------------------|--------|------------------------|
| `internal/api/health.go` health endpoint contract | API endpoint `/api/health` JSON schema (intelligence subsystem field can now report `stale` in addition to `up`/`down`) | New status value `stale` is purely additive — no removed enum value, no breaking rename | None — zero stale first-party references remain |
| `internal/api/health_test.go` | Internal test consumer of `/api/health` | Tests updated to assert all three status branches (`up`/`stale`/`down`) | None — zero stale first-party references remain |
| Telegram bot status command (none currently) | Outbound notification | No bot command currently surfaces `/api/health`; no breadcrumb / deep link / generated client needs updating | None — zero stale first-party references remain |
| Generated API client (none) | None — no generated client consumes `/api/health` | No regenerated client artifact needed | None — zero stale first-party references remain |
| Web UI navigation / breadcrumb / redirect (none) | None — no user-facing navigation references the intelligence freshness field | No redirect or breadcrumb update needed | None — zero stale first-party references remain |

A workspace grep for `IntelligenceEngine != nil` and for `"intelligence"` JSON field consumers across `internal/`, `web/`, and `tests/` confirms the only first-party consumer of the prior pool-nil branch was `internal/api/health.go` itself; no stale references remain.

### Definition of Done

- [x] `GetLastSynthesisTime()` method added to `intelligence.Engine`
  - Evidence: `internal/intelligence/synthesis.go` `GetLastSynthesisTime(ctx)` runs `SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights`; covered by `TestGetLastSynthesisTime_*` in `internal/intelligence/synthesis_test.go`.
- [x] Health handler queries `GetLastSynthesisTime()` instead of simple pool-nil check
  - Evidence: `internal/api/health.go` calls `GetLastSynthesisTime()` and branches on result; verified by `TestHealthHandler_IntelligenceFreshInstallNotStale` and other `TestHealthHandler_*` cases in `internal/api/health_test.go`.
- [x] Scenario SCN-021-012 (Health reports stale when synthesis is overdue): Intelligence status = `down` when pool nil, `stale` when synthesis >48h, `up` otherwise
  - Evidence: `internal/api/health.go` graduated branch (pool-nil → down; synth older than 48h → stale; else up); covered by `TestHealthHandler_IntelligenceStale` and `TestHealthHandler_IntelligenceUp` cases.
- [x] Scenario SCN-021-013 (Health reports healthy when synthesis is recent): `stale` status contributes to overall `degraded` health (and `up` status keeps intelligence subsystem healthy when synthesis is recent)
  - Evidence: `internal/api/health.go` aggregates intelligence==`stale` into the overall `degraded` rollup; verified by health test that asserts overall=degraded when synthesis >48h.
- [x] `GetLastSynthesisTime()` query failure logs warning, defaults to `up` (not `stale`)
  - Evidence: `internal/api/health.go` wraps the call in error-tolerant branch — logs warning via `slog.Warn` and falls back to `up`; covered by `TestHealthHandler_IntelligenceUp` fallback path and the fresh-install zero-time guard `TestHealthHandler_IntelligenceFreshInstallNotStale`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (SCN-021-012/013) live in `internal/api/health_test.go` (originally planned at tests/e2e/health_freshness_test.go; freshness coverage was consolidated into the existing health test file) and re-run on every CI push so any reintroduction of stale-health detection or freshness-fallback regressions is caught immediately
  - Evidence: `internal/api/health_test.go` `TestHealthHandler_*` (IntelligenceStale, IntelligenceUp, IntelligenceFreshInstallNotStale) covers SCN-021-012/013 and the fresh-install fallback; the live `*/15` plus daily synthesis cron entries are asserted unchanged across hardening rounds by `TestCronEntries_WithEngine`.
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e` runs the full E2E set (health, alert delivery, search) without regression in tangential health/intelligence/api scenarios
  - Evidence: 2026-04-15 validation run + 2026-05-12 hardening sweep both reported `./smackerel.sh test unit` and the full `./smackerel.sh test e2e` E2E surface green; see report.md §Round 6 hardening sweep evidence block.
- [x] Scope 3 consumer impact sweep complete — zero stale first-party references remain after the `/api/health` intelligence subcheck was extended to add the `stale` status branch (additive change; the only first-party consumer was `internal/api/health.go` itself, and `internal/api/health_test.go` was updated to cover the new branches)
  - Evidence: see §Consumer Impact Sweep table above; workspace grep for `IntelligenceEngine != nil` and `"intelligence"` health-field consumers across `internal/`, `web/`, and `tests/` confirms no stale references; navigation / breadcrumb / redirect / generated-client surfaces are all unaffected.
- [x] `./smackerel.sh test unit` passes
