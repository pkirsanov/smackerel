# Feature: 021 Intelligence Delivery

## Problem Statement

Smackerel's intelligence engine (`internal/intelligence/`) contains a comprehensive set of synthesis, alerting, people intelligence, expertise tracking, learning paths, subscription detection, frequent-lookup analysis, monthly reporting, and content resurfacing capabilities. These features are well-implemented and tested at the data-model and generation level.

However, the system under-delivers intelligence to the user. Several intelligence outputs are generated but never surfaced via Telegram or any delivery channel, and multiple alert types have no automated production pipeline. The result is a system that ingests and processes correctly but keeps its intelligence locked inside the database.

**Specific gaps identified by codebase audit:**

1. **Alert delivery pipeline missing (PRD-007):** `GetPendingAlerts()` exists and enforces a 2/day delivery cap, but no scheduled job calls it to sweep pending alerts and deliver them via Telegram. Alerts created by `CheckOverdueCommitments()` and `GeneratePreMeetingBriefs()` accumulate in the DB with status `pending` and are never sent to the user.

2. **4 of 6 alert types have no producer (PRD-007):** The alert data model defines 6 types (`bill`, `return_window`, `trip_prep`, `relationship_cooling`, `commitment_overdue`, `meeting_brief`). Only `commitment_overdue` and `meeting_brief` have automated producers. The remaining 4 types have zero automated detection pipelines feeding `CreateAlert()`:
   - `bill` — `DetectSubscriptions()` runs weekly but never creates bill-due alerts for upcoming charges
   - `return_window` — no detection pipeline for purchase return deadlines
   - `trip_prep` — `DetectTripsFromEmail()` runs but never creates trip preparation alerts
   - `relationship_cooling` — no detection pipeline for fading contact frequency

3. **LogSearch() never called (PRD-017):** The search handler (`internal/api/search.go`) returns results but never calls `engine.LogSearch()`, breaking the entire frequent-lookup detection pipeline. `DetectFrequentLookups()` runs daily but has no input data because searches are never logged.

4. **Daily synthesis results not surfaced (PRD-003):** `RunSynthesis()` at 2 AM generates `SynthesisInsight` records (cross-domain through-lines, contradictions, patterns) but these are stored silently. They are only aggregated indirectly if the weekly synthesis narrative references them.

5. **Intelligence engine health gap (S-003):** The health endpoint checks whether the intelligence engine's DB pool is non-nil, but does not verify that scheduled intelligence jobs have executed recently. A healthy-looking engine with stale intelligence (e.g., last synthesis > 48 hours ago) reports no degradation.

## Outcome Contract

**Intent:** Every intelligence insight Smackerel generates reaches the user through Telegram at the right time — alerts within minutes, weekly narratives on Sunday afternoon, monthly reports on the 1st, and search-driven lookups feeding back into the intelligence loop.

**Success Signal:** (1) Pending alerts created by any producer are delivered via Telegram within the next scheduled sweep cycle (≤15 minutes). (2) All 6 alert types have automated producers generating alerts from real data. (3) Every search query feeds LogSearch(), and DetectFrequentLookups() produces results when the same query recurs 3+ times in 14 days. (4) The health endpoint reports degraded when intelligence jobs are stale.

**Hard Constraints:**
- Alert delivery respects the existing 2/day cap enforced by `GetPendingAlerts()`
- Alert priority ordering (1=high first) is preserved
- Snoozed alerts are respected and only delivered after snooze_until
- All scheduling uses the existing `internal/scheduler` cron infrastructure
- All Telegram delivery uses the existing `bot.SendDigest()` / `bot.SendMessage()` methods
- No new services or database schema changes required — all methods already exist

**Failure Condition:** Intelligence continues to accumulate in the database without reaching the user. Alerts created by producers sit at status `pending` indefinitely. Users must manually query the system to discover intelligence rather than receiving it proactively.

## Goals

- G1: Wire alert delivery pipeline — scheduled job that calls `GetPendingAlerts()` and delivers via Telegram
- G2: Wire subscription detection → bill alert production
- G3: Wire trip detection → trip_prep alert production
- G4: Implement return_window alert detection from purchase/order artifacts
- G5: Implement relationship_cooling alert detection from contact frequency decay
- G6: Add `LogSearch()` call to the search handler
- G7: Add intelligence freshness to health degraded check

## Non-Goals

- Changing the intelligence engine algorithms (synthesis, resurfacing, expertise, learning)
- Adding new alert types beyond the 6 already defined
- Building a web UI for intelligence delivery (Telegram is the delivery channel)
- Modifying the weekly synthesis or monthly report generation logic
- Changing the 2/day alert delivery cap
- Adding new Telegram bot commands or interactive elements

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Smackerel User | The single self-hosted user who captures information and receives intelligence | Receive proactive intelligence without manual queries; be reminded of upcoming deadlines, trips, and cooling relationships | Full system access |
| Scheduler (System) | Cron-based job runner that triggers intelligence pipelines | Execute scheduled jobs reliably; respect timing contracts | Internal system actor |
| Intelligence Engine (System) | Orchestrates detection, scoring, and alert creation | Detect conditions, create alerts, generate reports | Internal system actor — reads artifacts, writes alerts/insights |
| Telegram Bot (System) | Delivers formatted intelligence to the user | Send messages within rate limits and formatting constraints | Outbound message delivery |

## Use Cases

### UC-001: Alert Delivery Pipeline
- **Actor:** Scheduler, Intelligence Engine, Telegram Bot
- **Preconditions:** At least one alert exists with status `pending` or `snoozed` with `snooze_until <= NOW()`
- **Main Flow:**
  1. Scheduler triggers alert delivery job (every 15 minutes)
  2. Engine calls `GetPendingAlerts()` — returns up to 2 alerts respecting daily cap
  3. For each alert, Telegram Bot sends formatted message with type icon, title, body, and priority
  4. Engine marks alert as `delivered` with `delivered_at` timestamp
- **Alternative Flows:**
  - No pending alerts → job completes silently
  - Telegram delivery fails → alert remains `pending`, retried next cycle
  - Daily cap reached (2 already delivered today) → `GetPendingAlerts()` returns empty, job completes
- **Postconditions:** Delivered alerts have status `delivered` and `delivered_at` set

### UC-002: Subscription Bill Alert Production
- **Actor:** Intelligence Engine
- **Preconditions:** `DetectSubscriptions()` has identified active subscriptions with billing frequency
- **Main Flow:**
  1. Scheduled job runs subscription alert check (daily)
  2. Engine queries active subscriptions with next billing date within 3 days
  3. For each upcoming charge, Engine calls `CreateAlert()` with type `bill`, title including service name and amount, priority 2
  4. Deduplication: skip if a `bill` alert for the same subscription already exists in `pending`/`delivered` state within the last billing period
- **Postconditions:** Bill alerts exist for upcoming subscription charges

### UC-003: Trip Preparation Alert Production
- **Actor:** Intelligence Engine
- **Preconditions:** `DetectTripsFromEmail()` has identified upcoming trips
- **Main Flow:**
  1. Scheduled job runs trip alert check (daily)
  2. Engine queries trip dossiers with departure_date within 5 days and state `upcoming`
  3. For each upcoming trip, Engine calls `CreateAlert()` with type `trip_prep`, title including destination, priority 2
  4. Deduplication: skip if a `trip_prep` alert for the same trip already exists in `pending`/`delivered` state
- **Postconditions:** Trip prep alerts exist for upcoming travel

### UC-004: Return Window Alert Production
- **Actor:** Intelligence Engine
- **Preconditions:** Purchase/order artifacts exist with return window metadata
- **Main Flow:**
  1. Scheduled job runs return window check (daily)
  2. Engine queries artifacts from purchase/receipt sources with return deadline within 5 days
  3. For each expiring return window, Engine calls `CreateAlert()` with type `return_window`, priority 1 (time-sensitive)
  4. Deduplication: skip if alert for same artifact already exists
- **Postconditions:** Return window alerts exist for expiring deadlines

### UC-005: Relationship Cooling Alert Production
- **Actor:** Intelligence Engine
- **Preconditions:** People entities exist with communication history
- **Main Flow:**
  1. Scheduled job runs relationship cooling check (weekly)
  2. Engine queries people with last interaction > 30 days and previous interaction frequency ≥ 1/week
  3. For each cooling relationship, Engine calls `CreateAlert()` with type `relationship_cooling`, priority 3
  4. Deduplication: skip if alert for same person exists in `pending`/`delivered` state within last 30 days
- **Postconditions:** Relationship cooling alerts exist for fading contacts

### UC-006: Search Query Logging
- **Actor:** Smackerel User, Intelligence Engine
- **Preconditions:** User submits a search query via POST /api/search
- **Main Flow:**
  1. Search handler processes query and obtains results
  2. Handler calls `engine.LogSearch(ctx, query, len(results), topResultID)` after successful search
  3. LogSearch records the query for frequency tracking
  4. `DetectFrequentLookups()` (daily 4 AM) detects queries repeated 3+ times in 14 days
- **Alternative Flows:**
  - LogSearch fails → log warning, do not fail the search response (non-blocking)
  - Zero results → still log the query (failed lookups are intelligence too)
- **Postconditions:** Search query is recorded; frequent lookups can be detected

### UC-007: Intelligence Freshness Health Check
- **Actor:** External monitoring, Smackerel User
- **Preconditions:** Health endpoint is queried
- **Main Flow:**
  1. Health handler checks intelligence engine pool (existing)
  2. Health handler additionally queries last synthesis timestamp
  3. If last synthesis > 48 hours ago, intelligence service status is `stale`
  4. `stale` intelligence contributes to overall `degraded` status
- **Postconditions:** Health response accurately reflects intelligence pipeline freshness

## Requirements

### R-021-001: Alert Delivery Sweep Job
The scheduler MUST register a cron job that runs every 15 minutes, calls `GetPendingAlerts()`, delivers each alert via Telegram, and marks them as `delivered`.

### R-021-002: Bill Alert Producer
A daily scheduled job MUST query active subscriptions with next billing date within 3 days and call `CreateAlert()` with type `bill` for each, with deduplication.

### R-021-003: Trip Prep Alert Producer
A daily scheduled job MUST query upcoming trips (departure within 5 days) and call `CreateAlert()` with type `trip_prep` for each, with deduplication.

### R-021-004: Return Window Alert Producer
A daily scheduled job MUST query artifacts with return window metadata expiring within 5 days and call `CreateAlert()` with type `return_window` for each, with deduplication.

### R-021-005: Relationship Cooling Alert Producer
A weekly scheduled job MUST query people with last interaction > 30 days (previously ≥ 1/week frequency) and call `CreateAlert()` with type `relationship_cooling` for each, with deduplication per 30-day window.

### R-021-006: Search Query Logging
The search handler MUST call `engine.LogSearch()` after every successful search execution, passing the query text, result count, and top result ID.

### R-021-007: Intelligence Freshness in Health Check
The health endpoint MUST check the timestamp of the last synthesis run and report intelligence as `stale` when older than 48 hours, contributing to overall `degraded` status.

## User Scenarios (Gherkin)

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

Scenario: SCN-021-012 Health reports stale when synthesis is overdue
  Given the last synthesis run was 50 hours ago
  When GET /api/health is called
  Then the intelligence service status is "stale"
  And the overall status is "degraded"

Scenario: SCN-021-013 Health reports healthy when synthesis is recent
  Given the last synthesis run was 12 hours ago
  When GET /api/health is called
  Then the intelligence service status is "up"

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

## Acceptance Criteria

- AC-1: Pending alerts are delivered via Telegram within 15 minutes of creation (maps to SCN-021-001)
- AC-2: The 2/day delivery cap is enforced — no more than 2 alerts delivered per calendar day (maps to SCN-021-002)
- AC-3: All 6 alert types have automated producers creating alerts from real data conditions (maps to SCN-021-004 through SCN-021-008)
- AC-4: Each alert producer deduplicates — no duplicate alerts for the same condition within the dedup window (maps to SCN-021-005)
- AC-5: Every search query is logged via LogSearch(), and the search response is unaffected by logging failures (maps to SCN-021-009, SCN-021-011)
- AC-6: DetectFrequentLookups returns results when the same query appears 3+ times in 14 days with LogSearch data (maps to SCN-021-010)
- AC-7: Health endpoint reports `stale`/`degraded` when last synthesis exceeds 48 hours (maps to SCN-021-012, SCN-021-013)
- AC-8: Snoozed alerts with expired snooze_until are picked up by the delivery sweep (maps to SCN-021-003)
- AC-9: Telegram delivery failures do not mark alerts as delivered; they are retried next cycle (maps to SCN-021-014)

## Competitive Analysis

| Feature | Smackerel (Current) | Smackerel (After 021) | Notion | Rewind.ai | Mem.ai |
|---------|--------------------|-----------------------|--------|-----------|--------|
| Proactive alerts | Data model only — no delivery | 6 alert types auto-produced and delivered | Manual reminders only | No proactive alerts | Limited AI suggestions |
| Relationship tracking | People entities built | Cooling alerts surfaced | No relationship intelligence | No people tracking | Basic contact mentions |
| Subscription awareness | Detection runs silently | Bill alerts before charges | No subscription tracking | No financial awareness | No subscription tracking |
| Travel preparation | Trip dossiers assembled | Trip prep alerts pre-departure | No travel intelligence | No travel context | No travel features |
| Search intelligence | Results returned | Queries feed lookup detection → intelligence loop | Search history exists | Semantic search | AI-powered search |
| Intelligence freshness | Binary up/down | Staleness detection in health | N/A (SaaS) | N/A (SaaS) | N/A (SaaS) |

## Improvement Proposals

### IP-001: Alert Delivery Pipeline ⭐ Competitive Edge
- **Impact:** High — unlocks ALL alert-based intelligence delivery
- **Effort:** S — single cron job calling existing `GetPendingAlerts()` + `bot.SendDigest()`
- **Competitive Advantage:** Transforms static intelligence DB into proactive delivery system
- **Actors Affected:** Smackerel User
- **Business Scenarios:** BS-001, BS-002, BS-003

### IP-002: Alert Producer Wiring
- **Impact:** High — activates 4 dormant alert types
- **Effort:** M — 4 new producer functions, each querying existing data and calling `CreateAlert()`
- **Competitive Advantage:** Anticipatory intelligence (bill reminders, trip prep, relationship maintenance) that no competitor offers at this depth
- **Actors Affected:** Smackerel User, Intelligence Engine
- **Business Scenarios:** BS-004 through BS-008

### IP-003: Search-Intelligence Feedback Loop
- **Impact:** Medium — one line of code unlocks frequent-lookup detection
- **Effort:** S — add `LogSearch()` call after search execution
- **Competitive Advantage:** Closes the intelligence loop — repeated searches surface knowledge gaps
- **Actors Affected:** Smackerel User
- **Business Scenarios:** BS-009

### IP-004: Intelligence Freshness Observability
- **Impact:** Low — operational reliability improvement
- **Effort:** S — query last synthesis timestamp in health handler
- **Competitive Advantage:** Self-hosted reliability — user knows when intelligence is stale
- **Actors Affected:** Smackerel User (health monitoring)

## Non-Functional Requirements

- **Latency:** Alert delivery sweep must complete within 30 seconds. Each alert producer must complete within 2 minutes.
- **Reliability:** Telegram delivery failures must not lose alerts. Failed deliveries retry on next sweep cycle.
- **Observability:** All scheduled jobs log start/completion/failure with structured slog. Alert delivery logs include alert ID, type, and delivery status.
- **Resource Usage:** Alert producers must use bounded queries (LIMIT clauses) to prevent unbounded DB scans.
- **Deduplication:** Every alert producer must prevent duplicate alerts for the same condition within the applicable dedup window.

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Alert delivery latency | ≤15 min from creation to Telegram delivery | Timestamp diff: alert.created_at → alert.delivered_at |
| Alert producer coverage | 6/6 alert types have automated producers | Count of alert types with non-zero production in 30-day window |
| Search logging coverage | 100% of search queries logged | LogSearch call count vs SearchHandler invocation count |
| Intelligence freshness accuracy | Health reports stale when synthesis > 48h | Health endpoint integration test with synthetic timestamps |
| Alert deduplication rate | 0 duplicate alerts per condition per dedup window | Query for duplicate alert_type + artifact_id combinations |
