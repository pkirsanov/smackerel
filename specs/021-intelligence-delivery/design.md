# Design: 021 Intelligence Delivery

> **Design Successor Note (2026-05-31).** This spec stays `done`. Intelligence
> generation, alert sweep, scheduler wiring, search-log capture, and health
> reporting in this design remain active. The "no new Telegram bot commands"
> boundary is superseded: user-facing delivery and interaction now flow through
> the assistant facade in [spec 061](../061-conversational-assistant/design.md),
> the structured intent compiler in [spec 068](../068-structured-intent-compiler/design.md),
> and the transport adapter in [spec 069](../069-assistant-http-transport/design.md).
> [Spec 066](../066-legacy-keyword-surface-retirement/design.md) owns retirement
> of legacy keyword commands. This spec still owns the producer side (what gets
> delivered and when); the assistant stack owns the user-input channel.

## Design Brief

**Current State:** The intelligence engine (`internal/intelligence/`) has complete data-model and generation capabilities: synthesis, alerting, people intelligence, subscription detection, frequent-lookup analysis, and reporting. The scheduler (`internal/scheduler/scheduler.go`) already wires daily synthesis (2 AM), resurfacing (8 AM), pre-meeting briefs (every 5 min), weekly synthesis (Sunday 4 PM), monthly report (1st at 3 AM), subscription detection (Monday 3 AM), and frequent lookup detection (4 AM daily). However, four critical delivery paths are broken or missing.

**Target State:** Wire the last-mile delivery gaps so that every intelligence insight Smackerel produces actually reaches the user. Specifically: (1) sweep pending alerts and deliver via Telegram, (2) produce alerts for the 4 orphaned alert types, (3) log search queries to feed the frequent-lookup pipeline, (4) report intelligence freshness in the health endpoint.

**Patterns to Follow:**
- Cron job registration in `scheduler.Start()` — all existing jobs follow the same pattern: `s.cron.AddFunc(expr, func() { ctx, cancel := ...; defer cancel(); s.engine.Method(ctx); ... })` ([scheduler.go](internal/scheduler/scheduler.go))
- Alert creation via `engine.CreateAlert()` with deduplication guards in the SQL query (see `CheckOverdueCommitments` in [engine.go](internal/intelligence/engine.go#L265))
- Telegram delivery via `s.bot.SendDigest(text)` — used by digest, resurfacing, weekly synthesis, monthly report, and pre-meeting briefs
- Health service status pattern in [health.go](internal/api/health.go) — `map[string]ServiceStatus` with `Status` field

**Patterns to Avoid:**
- Direct `bot.SendDigest()` inline in the alert delivery loop without marking delivered first — if the bot call succeeds but the status update fails, the alert may be re-delivered. Mark delivered only on success, leave as pending on failure (retry-safe).
- Fire-and-forget goroutines for alert delivery (as used by digest polling). Alert delivery should be synchronous within the cron callback since `GetPendingAlerts()` already enforces the 2/day cap.

**Resolved Decisions:**
- No new database tables or schema changes — all required tables (`alerts`, `subscriptions`, `trips`, `search_log`, `people`, `artifacts`) already exist
- No new services — all work is wiring within existing Go packages
- Alert delivery uses `bot.SendDigest()` — the same method used by all other Telegram delivery paths
- A `MarkAlertDelivered()` method is needed on `Engine` (simple UPDATE status + delivered_at)
- Health freshness queries `synthesis_insights.created_at` (the table already stores timestamps)

**Open Questions:**
- None — all required methods and data exist; this is purely wiring work.

## Overview

This feature wires four missing intelligence delivery paths with minimal changes across three packages:

| Gap | Package | Change Type |
|-----|---------|-------------|
| Alert delivery sweep | `internal/scheduler` | New cron job |
| 4 alert producers | `internal/intelligence` | 4 new methods, 4 new cron jobs in scheduler |
| Search query logging | `internal/api` | 1 line added to search handler |
| Intelligence freshness | `internal/api` | ~10 lines in health handler |

No new packages, no new dependencies, no schema changes.

## Architecture

### Component Interaction

```
scheduler.Start()
  ├── existing: RunSynthesis (2 AM)
  ├── existing: CheckOverdueCommitments (2 AM)
  ├── existing: Resurface (8 AM)
  ├── existing: GeneratePreMeetingBriefs (*/5 min)
  ├── existing: GenerateWeeklySynthesis (Sun 4 PM)
  ├── existing: GenerateMonthlyReport (1st 3 AM)
  ├── existing: DetectSubscriptions (Mon 3 AM)
  ├── existing: DetectFrequentLookups (4 AM)
  ├── NEW: DeliverPendingAlerts (*/15 min)
  ├── NEW: ProduceBillAlerts (6 AM daily)
  ├── NEW: ProduceTripPrepAlerts (6 AM daily)
  ├── NEW: ProduceReturnWindowAlerts (6 AM daily)
  └── NEW: ProduceRelationshipCoolingAlerts (Mon 7 AM weekly)
```

All new cron jobs follow the identical pattern already established by the 8 existing jobs in `scheduler.Start()`.

### Data Flow

```
[Alert Producers]  →  CreateAlert()  →  alerts table (status=pending)
                                              ↓
[DeliverPendingAlerts]  →  GetPendingAlerts()  →  bot.SendDigest()  →  MarkAlertDelivered()
                                                                              ↓
                                                                   alerts table (status=delivered)

[SearchHandler]  →  LogSearch()  →  search_log table
                                        ↓
[DetectFrequentLookups]  →  FrequentLookup results (existing, already scheduled)

[HealthHandler]  →  query MAX(created_at) FROM synthesis_insights  →  stale/up status
```

## New Methods on `intelligence.Engine`

### `MarkAlertDelivered(ctx, alertID) error`

Simple UPDATE to set `status = 'delivered'` and `delivered_at = NOW()` for a given alert ID. Required because no such method currently exists — `GetPendingAlerts()` reads alerts but nothing marks them delivered after Telegram send.

```sql
UPDATE alerts SET status = 'delivered', delivered_at = NOW()
WHERE id = $1 AND status IN ('pending', 'snoozed')
```

### `ProduceBillAlerts(ctx) error`

Queries `subscriptions` table for active subscriptions with estimated next billing date within 3 days. Calls `CreateAlert()` for each, with deduplication:

```sql
SELECT id, service_name, amount, currency, billing_freq
FROM subscriptions
WHERE status = 'active'
  AND billing_freq IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM alerts
    WHERE alert_type = 'bill'
      AND artifact_id = subscriptions.id
      AND status IN ('pending', 'delivered')
      AND created_at > NOW() - INTERVAL '30 days'
  )
LIMIT 20
```

Next billing date estimation: For `monthly` frequency, check if current day-of-month is within 3 days of `first_seen` day-of-month. For `annual`, check if current month+day is within 3 days of `first_seen` month+day. This is a heuristic — subscriptions don't store explicit billing dates, so the first-seen date is the best anchor.

Alert fields:
- `AlertType`: `bill`
- `Title`: `"Upcoming charge: {service_name} ({amount} {currency})"`
- `Priority`: 2
- `ArtifactID`: subscription ID (for dedup)

### `ProduceTripPrepAlerts(ctx) error`

Queries `trips` table for upcoming trips with departure within 5 days:

```sql
SELECT id, name, destination, start_date
FROM trips
WHERE status = 'upcoming'
  AND start_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
  AND NOT EXISTS (
    SELECT 1 FROM alerts
    WHERE alert_type = 'trip_prep'
      AND artifact_id = trips.id
      AND status IN ('pending', 'delivered')
  )
LIMIT 10
```

Alert fields:
- `AlertType`: `trip_prep`
- `Title`: `"Trip prep: {destination} in {days} days"`
- `Priority`: 2
- `ArtifactID`: trip ID (for dedup)

### `ProduceReturnWindowAlerts(ctx) error`

Queries `artifacts` table for purchase/receipt artifacts with return deadline metadata expiring within 5 days:

```sql
SELECT id, title, metadata->>'return_deadline' AS return_deadline
FROM artifacts
WHERE metadata->>'return_deadline' IS NOT NULL
  AND (metadata->>'return_deadline')::date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
  AND NOT EXISTS (
    SELECT 1 FROM alerts
    WHERE alert_type = 'return_window'
      AND artifact_id = artifacts.id
      AND status IN ('pending', 'delivered')
  )
LIMIT 10
```

Alert fields:
- `AlertType`: `return_window`
- `Title`: `"Return window closing: {title}"`
- `Priority`: 1 (time-sensitive)
- `ArtifactID`: artifact ID (for dedup)

### `ProduceRelationshipCoolingAlerts(ctx) error`

Queries the `people` table joined with interaction frequency data for contacts whose communication has dropped:

```sql
SELECT p.id, p.name,
       EXTRACT(DAY FROM NOW() - MAX(a.created_at))::int AS days_since,
       COUNT(DISTINCT a.id) FILTER (WHERE a.created_at > NOW() - INTERVAL '90 days') AS recent_count,
       COUNT(DISTINCT a.id) FILTER (WHERE a.created_at BETWEEN NOW() - INTERVAL '180 days' AND NOW() - INTERVAL '90 days') AS prior_count
FROM people p
JOIN edges e ON e.dst_id = p.id AND e.dst_type = 'person'
JOIN artifacts a ON a.id = e.src_id
GROUP BY p.id, p.name
HAVING EXTRACT(DAY FROM NOW() - MAX(a.created_at)) > 30
   AND COUNT(DISTINCT a.id) FILTER (WHERE a.created_at BETWEEN NOW() - INTERVAL '180 days' AND NOW() - INTERVAL '90 days') >= 4
LIMIT 10
```

The `prior_count >= 4` in 90 days ≈ roughly 1/week interaction rate. The `days_since > 30` confirms the drop-off.

Deduplication: skip if a `relationship_cooling` alert for the same person exists in `pending`/`delivered` state within the last 30 days (use `artifact_id = person.id`).

Alert fields:
- `AlertType`: `relationship_cooling`
- `Title`: `"Reconnect with {name}? Last contact {days} days ago"`
- `Priority`: 3
- `ArtifactID`: person ID (for dedup)

### `GetLastSynthesisTime(ctx) (time.Time, error)`

Simple query for health freshness:

```sql
SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights
```

Returns the timestamp of the most recent synthesis insight. If no synthesis has ever run, returns epoch (which will always be stale).

## Scheduler Wiring

All new jobs are added in `scheduler.Start()` inside the existing `if s.engine != nil` block, following the identical pattern of existing jobs.

### Alert Delivery Sweep — Every 15 Minutes

```
Cron: */15 * * * *
Timeout: 1 minute
Flow:
  1. engine.GetPendingAlerts(ctx) → []Alert
  2. For each alert:
     a. Format message: icon + title + body
     b. bot.SendDigest(formatted)
     c. engine.MarkAlertDelivered(ctx, alert.ID)
  3. If SendDigest or MarkAlertDelivered fails for one alert, log warning,
     skip to next alert (don't abort entire sweep)
```

Alert type icons for formatting:

| Type | Icon |
|------|------|
| `bill` | 💰 |
| `return_window` | 📦 |
| `trip_prep` | ✈️ |
| `relationship_cooling` | 👋 |
| `commitment_overdue` | ⏰ |
| `meeting_brief` | 📋 |

Format: `"{icon} {title}\n{body}"`

### Bill Alert Producer — Daily at 6 AM

```
Cron: 0 6 * * *
Timeout: 2 minutes
Flow: engine.ProduceBillAlerts(ctx)
```

Runs after subscription detection (Monday 3 AM) has populated the subscriptions table. Daily at 6 AM ensures timely alerting regardless of which day the billing date falls on.

### Trip Prep Alert Producer — Daily at 6 AM

```
Cron: 0 6 * * *
Timeout: 2 minutes
Flow: engine.ProduceTripPrepAlerts(ctx)
```

Same schedule as bills — batch all alert production together.

### Return Window Alert Producer — Daily at 6 AM

```
Cron: 0 6 * * *
Timeout: 2 minutes
Flow: engine.ProduceReturnWindowAlerts(ctx)
```

### Relationship Cooling Alert Producer — Weekly Monday at 7 AM

```
Cron: 0 7 * * 1
Timeout: 2 minutes
Flow: engine.ProduceRelationshipCoolingAlerts(ctx)
```

Weekly because relationship cooling is a slow signal (30+ day threshold). Running daily would waste cycles with no new results.

## Search Handler Change

In `internal/api/search.go`, the `SearchHandler` method must call `LogSearch()` after obtaining results. This is a single addition after the successful search path, before writing the JSON response.

Location: After the `results, totalCandidates, searchMode, err := engine.Search(...)` call and error handling, before `writeJSON(w, http.StatusOK, resp)`.

```go
// Log search for frequency tracking (non-blocking — failures don't affect response)
if d.IntelligenceEngine != nil {
    topResultID := ""
    if len(results) > 0 {
        topResultID = results[0].ArtifactID
    }
    if err := d.IntelligenceEngine.LogSearch(r.Context(), req.Query, len(results), topResultID); err != nil {
        slog.Warn("search logging failed", "error", err, "query", req.Query)
    }
}
```

Key design choice: `LogSearch()` failure is logged but does not affect the search response (R-021-006, SCN-021-011). The call is synchronous but fast (single INSERT).

## Health Handler Change

In `internal/api/health.go`, the intelligence service status block must additionally check synthesis freshness.

Replace the current simple pool-nil check:

```go
// Current (pool-nil only)
if d.IntelligenceEngine.Pool != nil {
    services["intelligence"] = ServiceStatus{Status: "up"}
} else {
    services["intelligence"] = ServiceStatus{Status: "down"}
}
```

With freshness-aware check:

```go
if d.IntelligenceEngine != nil {
    if d.IntelligenceEngine.Pool == nil {
        services["intelligence"] = ServiceStatus{Status: "down"}
    } else {
        lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
        if err != nil {
            slog.Warn("intelligence freshness check failed", "error", err)
            services["intelligence"] = ServiceStatus{Status: "up"}
        } else if time.Since(lastSynthesis) > 48*time.Hour {
            services["intelligence"] = ServiceStatus{Status: "stale"}
        } else {
            services["intelligence"] = ServiceStatus{Status: "up"}
        }
    }
}
```

The `stale` status must contribute to overall `degraded`:

```go
if svc.Status == "down" || svc.Status == "stale" {
    overall = "degraded"
}
```

## Security/Compliance

No new external surfaces, authentication paths, or data exposure:
- All new cron jobs run internally with the same `context.Background()` pattern as existing jobs
- `LogSearch()` is called from within an already-authenticated search handler (auth middleware at router level)
- Alert delivery uses the existing `bot.SendDigest()` which sends to pre-configured allowed chat IDs only
- No new API endpoints are added

## Observability

All new code paths use structured `slog` logging consistent with existing patterns:

| Event | Log Level | Fields |
|-------|-----------|--------|
| Alert delivery sweep start | Info | — |
| Alert delivered | Info | `alert_id`, `alert_type` |
| Alert delivery failed | Warn | `alert_id`, `error` |
| No pending alerts | (silent) | — |
| Bill alert produced | Info | `subscription_id`, `service_name` |
| Trip prep alert produced | Info | `trip_id`, `destination` |
| Return window alert produced | Info | `artifact_id`, `title` |
| Relationship cooling alert produced | Info | `person_id`, `name` |
| Search logged | (silent) | — |
| Search logging failed | Warn | `error`, `query` |
| Intelligence freshness check failed | Warn | `error` |

## Testing Strategy

| Requirement | Test Type | Approach |
|-------------|-----------|----------|
| R-021-001: Alert delivery sweep | Unit | Mock `GetPendingAlerts()` returning alerts → verify `MarkAlertDelivered()` called for each, verify `bot.SendDigest()` called with formatted message |
| R-021-001: Delivery cap | Unit | Return empty from `GetPendingAlerts()` when cap reached → verify no delivery attempted |
| R-021-002: Bill alerts | Unit | Seed subscriptions in test DB → run `ProduceBillAlerts()` → verify alerts created with correct type, dedup on re-run |
| R-021-003: Trip prep alerts | Unit | Seed trips in test DB → run `ProduceTripPrepAlerts()` → verify alerts created, dedup on re-run |
| R-021-004: Return window alerts | Unit | Seed artifacts with `return_deadline` metadata → run `ProduceReturnWindowAlerts()` → verify alerts with priority 1 |
| R-021-005: Relationship cooling | Unit | Seed people with interaction history → run `ProduceRelationshipCoolingAlerts()` → verify alerts, dedup on re-run |
| R-021-006: LogSearch | Unit | Call search handler with valid query → verify `LogSearch()` was called with correct args |
| R-021-006: LogSearch failure | Unit | Set engine pool to nil → call search → verify response returned normally, warning logged |
| R-021-007: Health freshness | Unit | Set last synthesis to 50h ago → call health → verify `stale` status and `degraded` overall |
| R-021-007: Health healthy | Unit | Set last synthesis to 12h ago → call health → verify `up` status |
| All delivery paths | Integration | Full stack: seed data → trigger cron → verify alerts in DB + delivery status |
| End-to-end | E2E | Create subscription → wait for alert sweep → verify Telegram delivery |

## Risks & Open Questions

**Resolved risks:**
- **Billing date estimation is heuristic:** Subscriptions don't store explicit next-billing-date. Using `first_seen` day-of-month as anchor is imprecise (billing dates can shift). This is acceptable for v1 — if a charge is missed by 1-2 days, the next daily run catches it. No user-facing failure.
- **Return window depends on metadata quality:** `return_deadline` in artifact metadata is populated by the extraction pipeline. If extraction misses the date, no alert is produced. This is an upstream data quality issue, not a delivery gap.

**No open questions remain** — all methods, tables, and delivery infrastructure exist. This feature is pure wiring.

## Unified Surfacing Controller

> **Added:** 2026-06-03 (release-planning MVP M1a). Companion to the new
> `## Cross-Surface Surfacing Budget` section in [spec.md](spec.md). This
> section describes the architecture of the "Next Smackerel" prioritizer.
> Scope 4 (`next-smackerel-prioritizer`) owns the implementation; this
> dispatch reopens spec 021 via `improve-existing` and leaves scope 4 at
> `Not Started` so a follow-up implementation dispatch picks it up.

### Problem the controller solves

Today, every surfacing path enforces its own ceiling in isolation:

- `internal/intelligence/alerts.go` `GetPendingAlerts()` enforces a 2/day
  alert cap.
- `internal/scheduler/scheduler.go` runs digest, resurfacing, weekly
  synthesis, monthly report on independent cron schedules.
- `internal/notification/decision.go` `DecisionEngine` evaluates each
  inbound notification independently and emits a `ProcessingDecision`.
- `internal/telegram/` delivers each outbound message as it arrives.

There is NO component that knows the SUM of nudges already delivered to a
user across all of these surfaces today. A user can therefore receive 2
alerts + 1 digest + 1 weekly synthesis + 1 trip-prep ping + 1 Telegram
broadcast all on the same Sunday afternoon and exceed any reasonable
interruption budget while every individual channel still believes it is
"under its cap".

### Architecture

The Unified Surfacing Controller is a single in-process decision point in
a new package (proposed: `internal/intelligence/surfacing`) that sits
upstream of EVERY user-visible dispatch path. Producers do not call
dispatchers directly anymore; they propose surfacing events to the
controller, and the controller decides whether each event is permitted,
deduped, suppressed, or escalated.

```
[ Producers ]                                  [ Channels ]
 alerts.GetPendingAlerts ─┐                  ┌─► telegram.SendDigest
 digest assembler ────────┤                  ├─► web push
 resurfacing job ─────────┤   ┌───────────┐  ├─► ntfy
 weekly synthesis ────────┼──►│  Unified  │──┼─► email-out
 monthly report ──────────┤   │ Surfacing │  └─► (future channels)
 trip prep producer ──────┤   │ Controller│
 relationship cooling ────┘   └───────────┘
                                    ▲
                                    │ reads decision input from
                                    │ internal/notification/decision.go
                                    │ (spec 054) + annotation feedback
                                    │ from spec 027
```

### Decision pipeline

For every candidate surfacing event the controller runs:

1. **Normalize.** Convert the producer-specific payload to a
   `SurfacingCandidate` with: `userID`, `contentKey` (artifact id + insight
   kind), `priority`, `producer`, `proposedChannels`, `timeCritical`,
   `enrichmentRefs` (from `DecisionEngine.Decide` output).
2. **Dedupe.** Reject candidates whose `contentKey` was already delivered
   today or is currently in flight on any other channel.
3. **Suppress.** Reject candidates whose `contentKey` was acknowledged,
   dismissed, or marked "not useful" by the user (annotation signal from
   spec 027) within the configured suppression window.
4. **Budget check.** Increment-and-check the per-user global counter.
   Reject when the daily budget is exhausted UNLESS step 5 escalates.
5. **Urgent escalation.** If `priority == 1` AND `timeCritical == true`
   AND the most recent urgent candidate for this user went undelivered
   today, permit dispatch and record `budget_overrides_total{reason=
   "urgent_escalation"}`. Per-channel safety nets (e.g., alert 2/day cap)
   still apply.
6. **Emit.** Return a `SurfacingDecision` to the producer with one of:
   `permit` (with bound channel set), `deduped`, `suppressed`,
   `deferred-budget-exhausted`, `escalated`. The producer then calls the
   appropriate dispatcher only when the decision is `permit` or
   `escalated`.

### Configuration (SST, no defaults)

All knobs live in `config/smackerel.yaml` under a new `surfacing:` key and
flow through the existing config generator. Missing values fail loud at
startup per the NO-DEFAULTS policy:

| Key | Meaning |
|-----|---------|
| `surfacing.daily_nudge_budget` | Max nudges/day per user across all channels (proposed MVP value: 5) |
| `surfacing.suppression_window_hours` | How long an ack/dismissal suppresses follow-ups for the same `contentKey` |
| `surfacing.urgent_escalation_enabled` | Whether priority-1 + timeCritical may exceed the daily budget |
| `surfacing.dedupe_window_hours` | How long `contentKey` dedupe applies (defaults to 24 explicitly) |

### Metrics (Prometheus)

The controller exposes the SLOs declared in [spec.md](spec.md) `##
Cross-Surface Surfacing Budget` via new counter/gauge families wired in
`internal/metrics/metrics.go`:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_surfacing_nudges_delivered_total` | counter | `channel`, `priority` | Per-channel delivery count for SLO 1 |
| `smackerel_surfacing_acted_on_total` | counter | `channel` | Numerator for acted-on rate (SLO 2) |
| `smackerel_surfacing_false_positive_total` | counter | `channel` | Numerator for false-positive ceiling (SLO 3) |
| `smackerel_surfacing_dedupe_total` | counter | `reason` (`same_day`, `in_flight`) | Suppression visibility |
| `smackerel_surfacing_suppression_total` | counter | `reason` (`acknowledged`, `dismissed`, `not_useful`) | SLO 4 evidence |
| `smackerel_surfacing_budget_overrides_total` | counter | `reason` (`urgent_escalation`) | SLO 5 audit trail |
| `smackerel_surfacing_budget_remaining` | gauge | `user_id` (hashed) | Operator visibility into per-user budget headroom |

### Integration touchpoints

| Existing module | Required change (Scope 4 will own) |
|-----------------|------------------------------------|
| `internal/intelligence/alerts.go` (alert delivery sweep added in Scope 1) | Replace direct `bot.SendDigest()` call with `controller.Propose(...)` and dispatch only on `permit`/`escalated` |
| `internal/scheduler/scheduler.go` (digest, resurfacing, weekly synthesis, monthly report, pre-meeting briefs) | Wrap each Telegram delivery call with a controller proposal |
| `internal/notification/decision.go` `DecisionEngine` | Treated as upstream input — the controller reads `DecisionEvaluation.Record()` for enrichment context; no change to its public API required |
| `internal/telegram/`, `internal/notification/` (push/email-out adapters) | No change to dispatcher logic — they remain dumb channels; the controller's `permit` decision is what unlocks them |
| `internal/annotation/` (spec 027) | Surface ack/dismiss/"not useful" signals through a small read interface the controller can poll for suppression checks |

### Coordination with adjacent specs

- **Spec 025 (synthesis)** — provides the content quality / relevance
  scoring the controller can use as a tiebreaker when budget headroom is
  partial.
- **Spec 054 (notification intelligence)** — the `DecisionEngine` in
  `internal/notification/decision.go` remains the per-event policy
  evaluator; the controller is the per-user budget enforcer. They compose
  rather than overlap.
- **Spec 027 (annotations)** — the canonical source for user-feedback
  signals that drive the suppression-on-acknowledged behavior.

### Non-goals for this dispatch

- No code changes in this dispatch. Spec/design/scope/manifest additions
  only.
- No new database schema in this design. The controller can begin with
  in-memory per-user counters and migrate to a `surfacing_events` table
  when persistence is needed; that decision belongs to the Scope 4
  implementation dispatch.
- No changes to existing per-channel safety nets (alert 2/day cap, etc.).
  Those remain as defense-in-depth beneath the unified budget.
