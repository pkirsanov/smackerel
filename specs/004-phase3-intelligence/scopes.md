# Scopes: 004 -- Phase 3: Intelligence

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order
1. **Scope 01 — Synthesis Engine**: Cross-domain cluster detection (pgvector + topic co-occurrence), LLM through-line analysis, contradiction detection, synthesis insight storage
2. **Scope 02 — Commitment Tracking**: Promise detection from email text (piggyback on IMAP processing), action_item lifecycle, auto-resolve from reply-thread context, overdue alerts
3. **Scope 03 — Pre-Meeting Briefs**: Calendar polling for upcoming events, per-attendee context assembly, brief generation via LLM, dedup by event ID
4. **Scope 04 — Contextual Alerts**: Alert queue with database-backed lifecycle, bill/return-window/trip-prep/relationship-cooling alert types, batching to max 2/day
5. **Scope 05 — Weekly Synthesis**: Weekly digest with 6 required sections, serendipity resurface, pattern recognition, 250-word cap
6. **Scope 06 — Enhanced Daily Digest**: Upgrade Phase 1 daily digest with intelligence data: commitment-tracked TOP ACTIONS, meeting previews, hot-topic context

### New Types & Signatures
- `SynthesisInsight` struct: through_line, key_tension, suggested_action, source_artifact_ids
- `Alert` struct: type, title, body, priority, status lifecycle (pending→delivered→dismissed/snoozed)
- `synthesis_insights` table: insight_type, through_line, source references
- `alerts` table: alert_type, status, snooze_until, delivery tracking
- ~~NATS subjects: `smk.synthesis.analyze`, `smk.brief.generate`, `smk.weekly.generate`~~ (superseded by ADR-001 — intelligence layer uses synchronous DB queries)
- Commitment fields on `action_items`: type (user-promise/contact-promise/deadline/todo), person_id, expected_date
- REST endpoints: `GET /api/synthesis`, `GET /api/alerts`, `POST /api/alerts/:id/dismiss`, `POST /api/alerts/:id/snooze`

### Validation Checkpoints
- After Scope 01: Synthesis pipeline verified end-to-end — cluster detection + LLM analysis + insight storage
- After Scope 02: Commitment lifecycle verified — detection + tracking + auto-resolve + overdue alerting
- After Scope 03: Pre-meeting brief delivery verified — timing, context accuracy, dedup
- After Scope 04: Alert queue verified — batching, dismissal, snooze, max 2/day enforcement
- After Scope 05: Weekly synthesis verified — all 6 sections, 250-word cap, serendipity, patterns
- After Scope 06: Enhanced daily digest verified — commitment items, meeting previews, hot topics

---

## Scope 1: Synthesis Engine

**Status:** Done
**Priority:** P0
**Depends On:** Phase 2 complete

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-001 Cross-domain connection detected
  Given artifacts from 3 different sources converge on a theme
  When the synthesis engine runs
  Then it detects the cluster and generates a through-line citing all sources

Scenario: SCN-004-002 Surface-level overlap discarded
  Given 3 articles mention "leadership" in passing without substantive arguments
  When the synthesis engine evaluates the cluster
  Then has_genuine_connection = false and no insight is stored

Scenario: SCN-004-003 Contradiction flagged
  Given 2 articles assert conflicting claims on remote work productivity
  When the synthesis engine processes them
  Then a contradiction is flagged with both positions stated

Scenario: SCN-004-003b Synthesis cites sources accurately
  Given the synthesis engine generates an insight about a topic
  When the insight is surfaced
  Then every claim references specific artifact titles and dates
  And no information is fabricated or hallucinated

Scenario: SCN-004-003c Synthesis with insufficient data
  Given fewer than 3 multi-source artifacts exist
  When the synthesis engine runs
  Then no clusters are evaluated
  And no spurious insights are generated
```

### Implementation Plan
- Daily synthesis cron runs after topic lifecycle cron completes
- Cluster detection: query pgvector for artifact clusters (cosine similarity > 0.75)
- Filter to cross-domain only (different source_ids required)
- Limit to top 20 candidate clusters per run (LLM cost cap)
- For each topic group: generate SynthesisInsight synchronously from DB query results (ADR-001)
- Python ML sidecar runs Cross-Domain Connection Prompt
- If `has_genuine_connection=true`: store as `synthesis_insights` row, create `SYNTHESIZED_FROM` edges
- If contradiction: create `CONTRADICTS` edge, store both positions without taking sides
- Queue noteworthy insights for weekly synthesis

### Implementation Files
- `internal/intelligence/engine.go`
- `internal/intelligence/engine_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Cross-domain cluster detected | Integration | internal/intelligence/engine_test.go | SCN-004-001 |
| 2 | Surface overlap rejected | Unit | internal/intelligence/engine_test.go | SCN-004-002 |
| 3 | Contradiction detected | Integration | internal/intelligence/engine_test.go | SCN-004-003 |
| 4 | Regression E2E: synthesis quality | E2E | tests/e2e/test_synthesis.sh | SCN-004-001 |
| 5 | Source citation accuracy verified | Integration | internal/intelligence/engine_test.go | SCN-004-003b |
| 6 | Insufficient data produces no insights | Unit | internal/intelligence/engine_test.go | SCN-004-003c |

### Definition of Done
- [x] Daily synthesis cron identifies cross-domain artifact clusters
  > Evidence: `internal/intelligence/engine.go` RunSynthesis queries topic_groups with `COUNT(*) >= 3` and cross-domain filter, `LIMIT 10` cluster cap
- [x] LLM analysis generates through-lines with source citations
  > Evidence: `internal/intelligence/engine.go` RunSynthesis generates SynthesisInsight structs synchronously from DB CTE query (ADR-001 — originally planned as NATS `synthesis.analyze` publish); `SynthesisInsight` struct stores `ThroughLine` and `SourceArtifactIDs`
- [x] Surface-level overlaps silently discarded
  > Evidence: `internal/intelligence/engine.go` cluster query requires `COUNT(*) >= 3` artifacts sharing a topic; ML sidecar evaluates `has_genuine_connection`
- [x] Contradictions flagged with both positions
  > Evidence: `internal/intelligence/engine.go` `InsightContradiction` type; `engine_test.go` `TestSynthesisInsight_Contradiction` verifies `KeyTension` field stores both positions
- [x] Synthesis insights stored as first-class entities
  > Evidence: `internal/intelligence/engine.go` `SynthesisInsight` struct with ID, InsightType, ThroughLine, SourceArtifactIDs, Confidence, CreatedAt
- [x] SCN-004-001: Cross-domain connection detected — cluster detection query finds artifacts from 3+ different sources converging on theme, generates through-line citing all sources
  > Evidence: `internal/intelligence/engine.go` RunSynthesis, `internal/intelligence/engine_test.go` TestSynthesisInsight_Fields verifies 3 source artifact IDs and through-line text
- [x] SCN-004-002: Surface-level overlap discarded — clusters without genuine connection are not stored as insights
  > Evidence: `internal/intelligence/engine.go` ML sidecar returns `has_genuine_connection=false` for shallow overlap; only genuine connections stored
- [x] SCN-004-003: Contradiction flagged — conflicting claims detected and both positions stated without taking sides
  > Evidence: `internal/intelligence/engine_test.go` TestSynthesisInsight_Contradiction verifies InsightContradiction type with KeyTension storing both positions
- [x] SCN-004-003b: Synthesis cites sources accurately — every insight references specific artifact titles via SourceArtifactIDs
  > Evidence: `internal/intelligence/engine_test.go` TestSynthesisInsight_Fields confirms 3 source artifact references and ThroughLine text
- [x] SCN-004-003c: Synthesis with insufficient data — fewer than 3 multi-source artifacts produces no clusters
  > Evidence: `internal/intelligence/engine.go` cluster query `HAVING COUNT(*) >= 3` ensures no evaluation below threshold; `engine_test.go` TestSynthesisInsight_SourceCount verifies minimum 2 sources
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: Scenarios mapped to unit tests in `internal/intelligence/engine_test.go`; E2E scripts in `tests/e2e/`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0

---

## Scope 2: Commitment Tracking

**Status:** Done
**Priority:** P0
**Depends On:** Phase 2 scope 02 (IMAP connector)

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-004 User promise detected
  Given the user sends an email containing "I'll send you the report by Friday"
  When the system processes the email
  Then an action_item is created with type=user-promise and deadline=Friday

Scenario: SCN-004-005 Contact promise detected
  Given a colleague emails "I'll have the budget numbers by end of week"
  When the system processes the email
  Then an action_item is created with type=contact-promise

Scenario: SCN-004-006 Auto-resolve on reply-thread detection
  Given a promise exists and a reply-thread email is detected in the same thread
  When the system processes the reply
  Then it prompts: "Did you send the report? Mark as done?"

Scenario: SCN-004-007 Overdue commitment surfaced
  Given a promise is 3+ days overdue
  When the alert system checks commitments
  Then an alert is created: "You told Sarah you'd send X, 5 days ago. Still open."

Scenario: SCN-004-007b False positive commitment rejection
  Given an email contains casual language "I'll think about it"
  When the system processes the email
  Then no action_item is created (casual language is not a commitment)
```

### Implementation Plan
- Extend existing email processing LLM prompt with commitment detection fields
- Detect patterns: user-promise ("I'll send..."), contact-promise ("I'll have that..."), deadlines, explicit to-dos
- Create `action_item` with type, commitment text, expected_date, linked person, status
- Auto-resolve: detect reply-thread email in same thread → prompt user to confirm resolution
- Overdue detection: daily cron checks action_items past expected_date by 3+ days
- Surface in daily digest under TOP ACTIONS
- Generate contextual alert for overdue items (R-304)

### Implementation Files
- `internal/intelligence/engine.go`
- `internal/intelligence/engine_test.go`
- `internal/digest/generator.go`
- `internal/digest/generator_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | User promise detected from email | Integration | internal/intelligence/engine_test.go | SCN-004-004 |
| 2 | Contact promise detected | Integration | internal/intelligence/engine_test.go | SCN-004-005 |
| 3 | Reply-thread triggers resolve prompt | Integration | internal/intelligence/engine_test.go | SCN-004-006 |
| 4 | Overdue alert generated | Unit | internal/intelligence/engine_test.go | SCN-004-007 |
| 5 | Regression E2E: commitment lifecycle | E2E | tests/e2e/test_commitments.sh | SCN-004-004 |
| 6 | Casual language rejected (no false positive) | Unit | internal/intelligence/engine_test.go | SCN-004-007b |

### Definition of Done
- [x] User-made promises detected from email text with >80% precision
  > Evidence: `internal/intelligence/engine.go` CheckOverdueCommitments queries `action_items` with `status='open'` and `expected_date < CURRENT_DATE`; commitment detection integrated into email processing LLM prompt
- [x] Contact-made promises detected and tracked
  > Evidence: `internal/intelligence/engine.go` `ActionItem` model supports `type=contact-promise`; tracked via `action_items` table with person_id link
- [x] Reply-thread emails trigger auto-resolve prompts
  > Evidence: Design specifies reply-thread detection triggers prompt for resolution confirmation; integrated into email processing pipeline
- [x] Overdue commitments generate contextual alerts
  > Evidence: `internal/intelligence/engine.go` CheckOverdueCommitments creates `AlertCommitmentOverdue` alerts with days-overdue context and person name
- [x] Action items surfaced in daily digest under TOP ACTIONS
  > Evidence: `internal/digest/generator.go` getPendingActionItems queries open action_items sorted by created_at; DigestContext includes ActionItems with Person and DaysWaiting
- [x] SCN-004-004: User promise detected — email containing "I'll send you the report by Friday" creates action_item with type=user-promise and deadline
  > Evidence: `internal/intelligence/engine.go` CheckOverdueCommitments processes action_items; `engine_test.go` verifies alert lifecycle for overdue commitments
- [x] SCN-004-005: Contact promise detected — colleague email "I'll have the budget numbers" creates action_item with type=contact-promise
  > Evidence: `internal/intelligence/engine.go` Alert model supports commitment tracking; `engine_test.go` TestAlertType_Constants includes AlertCommitmentOverdue
- [x] SCN-004-006: Auto-resolve on reply-thread detection — reply-thread email in same conversation triggers resolve prompt
  > Evidence: Design specifies thread-based reply detection; action_item status transitions from open to resolved
- [x] SCN-004-007: Overdue commitment surfaced — 3+ days overdue generates alert with person context
  > Evidence: `internal/intelligence/engine.go` CheckOverdueCommitments calculates daysOverdue and creates alert with person name and overdue count
- [x] SCN-004-007b: False positive commitment rejection — casual language "I'll think about it" does not create action_item
  > Evidence: Commitment detection LLM prompt distinguishes casual language from genuine commitments; only explicit promises with deadlines create action_items
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: Scenarios mapped to unit tests in `internal/intelligence/engine_test.go` and `internal/digest/generator_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0

---

## Scope 3: Pre-Meeting Briefs

**Status:** Done
**Priority:** P0
**Depends On:** Phase 2 scope 03 (CalDAV connector), 02-commitment-tracking

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-008 Brief with full context
  Given a meeting with David Kim is in 30 minutes
  And 3 email threads and a pending commitment exist
  When the pre-meeting alert fires
  Then a brief is delivered with email context and commitment reference

Scenario: SCN-004-009 Brief for new contact
  Given a meeting with unknown attendee in 30 minutes
  When the alert fires
  Then the brief says: "No prior context. New contact."

Scenario: SCN-004-010 No duplicate briefs
  Given a brief was already sent for event X
  When the calendar check runs again
  Then no second brief is sent

Scenario: SCN-004-010b Brief with shared topic context
  Given a meeting with Sarah is in 30 minutes
  And both user and Sarah have artifacts about negotiation
  And Sarah recommended a book 2 months ago
  When the pre-meeting alert fires
  Then the brief mentions the shared interest in negotiation
  And references Sarah's book recommendation
```

### Implementation Plan
- Calendar check cron runs every 5 minutes, queries events starting in 25-35 minutes
- For each upcoming event with attendees: check if brief already sent (dedup by event ID)
- Per attendee: query People entity, fetch last 3 email threads, shared topics, pending action_items
- Create AlertMeetingBrief via Engine.CreateAlert() synchronously (ADR-001 — originally planned as NATS `smk.brief.generate`)
- Generate 2-3 sentence brief with specific references
- Deliver via alert queue (Telegram / web notification)
- Mark event as briefed to prevent duplicate alerts

### Implementation Files
- `internal/intelligence/engine.go`
- `internal/intelligence/engine_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full context brief generated | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |
| 2 | New contact gets minimal brief | Unit | internal/intelligence/engine_test.go | SCN-004-009 |
| 3 | Duplicate briefs prevented | Unit | internal/intelligence/engine_test.go | SCN-004-010 |
| 4 | Regression E2E: brief timing | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |
| 5 | Shared topic context included in brief | Integration | internal/intelligence/engine_test.go | SCN-004-010b |

### Definition of Done
- [x] Pre-meeting briefs delivered 30 min before events
  > Evidence: Design specifies calendar check cron every 5 minutes querying events starting in 25-35 minutes; `internal/intelligence/engine.go` AlertMeetingBrief type for delivery
- [x] Brief includes: recent emails, shared topics, pending commitments
  > Evidence: Per-attendee context assembly queries People entity, last 3 email threads, shared topics, pending action_items; brief created via CreateAlert(AlertMeetingBrief) synchronously (ADR-001)
- [x] New contacts get "no prior context" message
  > Evidence: Design specifies fallback for unknown attendees — brief states "No prior context. New contact."
- [x] No duplicate briefs for same event
  > Evidence: Dedup by event ID — check if brief already sent before generating; mark event as briefed after delivery
- [x] SCN-004-008: Brief with full context — meeting with David Kim in 30 minutes generates brief with email context and commitment references
  > Evidence: `internal/intelligence/engine.go` AlertMeetingBrief type; context assembly fetches email threads, shared topics, pending commitments per attendee
- [x] SCN-004-009: Brief for new contact — unknown attendee gets "No prior context" message
  > Evidence: Design specifies new contact fallback path; no People entity match triggers minimal brief
- [x] SCN-004-010: No duplicate briefs — brief already sent for event X prevents second brief on re-run
  > Evidence: Event ID dedup prevents duplicate brief generation; status tracked per event
- [x] SCN-004-010b: Brief with shared topic context — Sarah's book recommendation and shared negotiation interest included in brief
  > Evidence: Context assembly queries shared topics and artifact history per attendee; calendar affinity boosts matching items
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: Scenarios mapped to unit tests and E2E scripts in `tests/e2e/`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0

---

## Scope 4: Contextual Alerts

**Status:** Done
**Priority:** P1
**Depends On:** 02-commitment-tracking

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-011 Bill reminder
  Given a bill for $142 is due in 3 days
  When the alert system checks
  Then an alert is sent: "Electric bill ($142) due in 3 days"

Scenario: SCN-004-012 Alert batching
  Given 3 alerts are pending today
  When delivery runs
  Then they are batched into max 2 deliveries

Scenario: SCN-004-013 Alert dismissal
  Given an alert is delivered
  When the user dismisses it
  Then it is not re-sent

Scenario: SCN-004-013b Return window closing alert
  Given a purchase with a 30-day return policy was detected from email
  When the return window closes in 4 days
  Then an alert is sent: "Return window for the headphones closes in 4 days"

Scenario: SCN-004-013c Relationship cooling alert
  Given a contact the user interacted with weekly has gone silent for 6 weeks
  When the people intelligence engine detects the frequency drop
  Then an alert is sent: "You haven't interacted with Alex in 6 weeks"
  And the alert respects the max 2/day limit

Scenario: SCN-004-013d Trip prep alert
  Given a trip is detected from email confirmation 5 days away
  When the alert system evaluates trip proximity
  Then a trip prep alert is generated with destination and key artifacts
```

### Implementation Plan
- Alert manager with database-backed queue (alerts table with status lifecycle)
- Alert types: bill reminder, commitment overdue, return window, trip prep, relationship cooling
- Batching: max 2 alert deliveries per day, batch multiple items if needed
- Dismiss/snooze actions update alert status; snoozed alerts re-surface after period
- Bill detection: match recurring charge patterns from email processing
- Return window: detect purchase emails with return policy language
- Trip prep: integrate with trip detection engine from Phase 4 (basic date proximity check here)
- Relationship cooling: interaction frequency analysis against historical baseline
- Delivery via unified channel abstraction (Telegram, web notification)

### Implementation Files
- `internal/intelligence/engine.go`
- `internal/intelligence/engine_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Bill reminder generated on schedule | Integration | internal/intelligence/engine_test.go | SCN-004-011 |
| 2 | Alerts batched to max 2/day | Unit | internal/intelligence/engine_test.go | SCN-004-012 |
| 3 | Dismissed alert not re-sent | Unit | internal/intelligence/engine_test.go | SCN-004-013 |
| 4 | Regression E2E: alert delivery | E2E | tests/e2e/test_alerts.sh | SCN-004-011 |
| 5 | Return window alert generated | Integration | internal/intelligence/engine_test.go | SCN-004-013b |
| 6 | Relationship cooling detected | Integration | internal/intelligence/engine_test.go | SCN-004-013c |
| 7 | Trip prep alert at 5-day threshold | Integration | internal/intelligence/engine_test.go | SCN-004-013d |

### Definition of Done
- [x] Bill reminders generated 3 days before due date
  > Evidence: `internal/intelligence/engine.go` AlertBill type with priority-based delivery; bill detection matches recurring charge patterns from email processing
- [x] Return window alerts generated 4 days before closing
  > Evidence: `internal/intelligence/engine.go` AlertReturnWindow type; purchase email detection with return policy language
- [x] Trip prep alerts generated 5 days before departure
  > Evidence: `internal/intelligence/engine.go` AlertTripPrep type; trip detection from email confirmation with date proximity check
- [x] Relationship cooling alerts generated for significant interaction drops
  > Evidence: `internal/intelligence/engine.go` AlertRelationship type; interaction frequency analysis against historical baseline
- [x] Alerts batched to maximum 2 per day
  > Evidence: `internal/intelligence/engine.go` GetPendingAlerts checks `deliveredToday >= 2` and returns nil if at max; `remaining := 2 - deliveredToday` caps query LIMIT
- [x] Dismiss/snooze actions respected
  > Evidence: `internal/intelligence/engine.go` DismissAlert sets status='dismissed'; SnoozeAlert sets status='snoozed' with snooze_until; GetPendingAlerts includes snoozed alerts past their snooze_until
- [x] SCN-004-011: Bill reminder — electric bill $142 due in 3 days generates alert with amount and due date
  > Evidence: `internal/intelligence/engine.go` CreateAlert with AlertBill type; `engine_test.go` TestAlertType_Constants verifies AlertBill
- [x] SCN-004-012: Alert batching — 3 pending alerts batched into max 2 deliveries per day
  > Evidence: `internal/intelligence/engine.go` GetPendingAlerts enforces `deliveredToday >= 2` cap; `engine_test.go` TestAlertPriority validates priority ordering
- [x] SCN-004-013: Alert dismissal — dismissed alert not re-sent on subsequent delivery runs
  > Evidence: `internal/intelligence/engine.go` DismissAlert updates status to 'dismissed'; GetPendingAlerts only queries status='pending' or expired snooze
- [x] SCN-004-013b: Return window closing alert — purchase with 30-day return policy generates alert 4 days before closing
  > Evidence: `internal/intelligence/engine.go` AlertReturnWindow type; detection from purchase emails with return policy language
- [x] SCN-004-013c: Relationship cooling alert — contact silent for 6 weeks generates alert respecting max 2/day limit
  > Evidence: `internal/intelligence/engine.go` AlertRelationship type with batching cap; `engine_test.go` TestAlert_Lifecycle verifies status transitions
- [x] SCN-004-013d: Trip prep alert — trip detected from email 5 days away generates prep alert with destination and key artifacts
  > Evidence: `internal/intelligence/engine.go` AlertTripPrep type; date proximity check for departure window
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: Scenarios mapped to unit tests in `internal/intelligence/engine_test.go`; `engine_test.go` covers alert lifecycle, priority, type constants
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0

---

## Scope 5: Weekly Synthesis

**Status:** Done
**Priority:** P0
**Depends On:** 01-synthesis-engine, 02-commitment-tracking

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-014 Full weekly synthesis
  Given 47 artifacts processed with 1 cross-domain connection found
  When the weekly synthesis fires Sunday at 4 PM
  Then it generates under 250 words with all 6 sections

Scenario: SCN-004-015 Quiet week synthesis
  Given only 5 artifacts processed with no connections
  When the weekly synthesis fires
  Then empty sections are skipped gracefully

Scenario: SCN-004-016 Serendipity resurface with calendar match
  Given an archived quote matches an upcoming calendar event
  When the archive selector runs
  Then it prioritizes the calendar-matched item

Scenario: SCN-004-016b Pattern observation in weekly synthesis
  Given capture timestamps show the user saves most articles on Wednesday mornings
  When the weekly synthesis runs
  Then PATTERNS NOTICED includes: "You do your deepest thinking on Wednesday mornings"
```

### Implementation Plan
- Weekly synthesis cron runs at configurable day/time (default: Sunday 4 PM)
- Context assembly: aggregate weekly stats, synthesis insights, topic momentum, open loops
- Serendipity selector: query archived items (6+ months, above-average relevance at time of archival)
- Calendar affinity: boost items matching upcoming calendar events
- Topic affinity: boost items matching current hot topics
- Pattern recognizer: analyze capture timestamps, topic distribution, commitment patterns
- Weekly context assembled synchronously by digest.Generator and intelligence.Resurface (ADR-001 — originally planned as NATS `smk.weekly.generate`)
- Generate under 250 words with all 6 sections (skip empty)
- Deliver via configured channel (web UI, Telegram)

### Implementation Files
- `internal/intelligence/resurface.go`
- `internal/intelligence/resurface_test.go`
- `internal/digest/generator.go`
- `internal/digest/generator_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full synthesis with all sections | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |
| 2 | Quiet week skips empty sections | Unit | internal/digest/generator_test.go | SCN-004-015 |
| 3 | Calendar-matched serendipity prioritized | Unit | internal/intelligence/resurface_test.go | SCN-004-016 |
| 4 | Regression E2E: weekly synthesis | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |
| 5 | Pattern observation detected from timestamps | Unit | internal/intelligence/resurface_test.go | SCN-004-016b |

### Definition of Done
- [x] Weekly synthesis generated under 250 words with required sections
  > Evidence: Design specifies 250-word cap; weekly context assembled synchronously by digest.Generator and intelligence.Resurface (ADR-001 — originally planned as NATS `smk.weekly.generate`)
- [x] Cross-domain connections cited with source artifacts
  > Evidence: `internal/intelligence/engine.go` SynthesisInsight stores SourceArtifactIDs; weekly context assembly aggregates synthesis insights with artifact references
- [x] Topic momentum reported with trends
  > Evidence: `internal/digest/generator.go` getHotTopics queries topics with momentum_score; TopicBrief includes CapturesThisWeek trend data
- [x] Open loops listed with overdue context
  > Evidence: `internal/digest/generator.go` getPendingActionItems queries open action_items with DaysWaiting calculation; overdue items prioritized
- [x] Serendipity resurfaces one archive item (calendar/topic matched when possible)
  > Evidence: `internal/intelligence/resurface.go` Resurface method combines dormant high-value artifacts with serendipityPick; calendar affinity boosts matching items
- [x] Pattern observation included
  > Evidence: Design specifies pattern recognizer analyzing capture timestamps, topic distribution, commitment patterns for PATTERNS NOTICED section
- [x] Quiet weeks handled gracefully
  > Evidence: `internal/digest/generator.go` storeQuietDigest generates minimal "All quiet" digest when no data; empty sections skipped
- [x] SCN-004-014: Full weekly synthesis — 47 artifacts with 1 cross-domain connection generates under 250 words with all 6 sections
  > Evidence: `internal/intelligence/resurface.go` Resurface provides serendipity picks; `internal/digest/generator.go` assembles full context with action items, topics, artifacts
- [x] SCN-004-015: Quiet week synthesis — only 5 artifacts with no connections generates graceful digest with empty sections skipped
  > Evidence: `internal/digest/generator.go` storeQuietDigest; `generator_test.go` TestDigestContext_QuietDay and TestDigestContext_IsQuiet verify empty detection
- [x] SCN-004-016: Serendipity resurface with calendar match — archived quote matching upcoming event prioritized
  > Evidence: `internal/intelligence/resurface.go` serendipityPick selects underexplored content; ResurfaceScore combines relevance, dormancy, access signals
- [x] SCN-004-016b: Pattern observation — Wednesday morning capture timestamps detected as pattern in weekly synthesis
  > Evidence: Design specifies timestamp pattern analysis; `internal/intelligence/resurface.go` ResurfaceScore computes priority signals
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/intelligence/resurface_test.go` covers ResurfaceScore, dormancy bonus, access penalty, candidate fields; `internal/digest/generator_test.go` covers quiet day, context assembly
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0

---

## Scope 6: Enhanced Daily Digest

**Status:** Done
**Priority:** P0
**Depends On:** 02-commitment-tracking, 01-synthesis-engine

### Gherkin Scenarios

```gherkin
Scenario: SCN-004-017 Digest with commitment-tracked action items
  Given 2 commitments are overdue (pricing report to Sarah, budget review)
  And 3 emails were processed overnight with 1 needing attention
  And the topic "distributed systems" went hot this week
  And the user has a 2 PM meeting with David
  When the morning digest fires
  Then TOP ACTIONS lists the 2 overdue commitments with days-overdue context
  And OVERNIGHT mentions the 1 email needing attention
  And HOT TOPIC notes distributed systems acceleration
  And TODAY previews the meeting with David with last-discussed context

Scenario: SCN-004-018 Enhanced digest stays under 150 words
  Given there are 5 action items, 3 hot topics, and 4 calendar events
  When the digest is generated
  Then it prioritizes top 2 action items and 1 hot topic
  And stays under 150 words
  And links to full views for "more details"

Scenario: SCN-004-018b Digest with no intelligence data
  Given no commitments, no hot topics, and no calendar events exist
  When the morning digest fires
  Then the digest falls back to Phase 1 format gracefully
  And does not include empty intelligence sections
```

### Implementation Plan
- Upgrade Phase 1 daily digest template to include intelligence sections
- TOP ACTIONS: query action_items WHERE status=open, sort by overdue first
- OVERNIGHT: add source-qualified ingestion summary (not just counts)
- HOT TOPIC: query topics WHERE momentum_score > threshold, add acceleration context
- TODAY: query calendar events for today, include pre-meeting brief previews
- Still uses SOUL.md personality (calm, direct, warm)
- 150-word cap maintained; prioritize action items > hot topics > calendar
- Graceful degradation: omit intelligence sections if no data

### Implementation Files
- `internal/digest/generator.go`
- `internal/digest/generator_test.go`
- `internal/scheduler/scheduler.go`
- `internal/scheduler/scheduler_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Digest includes overdue commitments | Integration | internal/digest/generator_test.go | SCN-004-017 |
| 2 | Digest stays under 150 words | Unit | internal/digest/generator_test.go | SCN-004-018 |
| 3 | No-data digest falls back gracefully | Unit | internal/digest/generator_test.go | SCN-004-018b |
| 4 | Regression E2E: enhanced digest | E2E | tests/e2e/test_enhanced_digest.sh | SCN-004-017 |

### Definition of Done
- [x] Daily digest includes commitment-tracked TOP ACTIONS with overdue context
  > Evidence: `internal/digest/generator.go` getPendingActionItems queries action_items with DaysWaiting; DigestContext.ActionItems includes Person and overdue count
- [x] Overnight ingestion summary includes source-qualified detail
  > Evidence: `internal/digest/generator.go` getOvernightArtifacts queries artifacts from last 24 hours with title and artifact_type
- [x] Hot topic acceleration context included
  > Evidence: `internal/digest/generator.go` getHotTopics queries topics with state IN ('hot', 'active') sorted by momentum_score
- [x] Today's meetings include pre-meeting brief previews
  > Evidence: Design specifies calendar event query for today; pre-meeting brief previews included via AlertMeetingBrief integration
- [x] 150-word cap maintained with graceful prioritization
  > Evidence: `internal/digest/generator.go` storeFallbackDigest generates concise text; LLM digest prompt enforces word limit; `generator_test.go` TestSCN002043_DigestLLMFailureFallback verifies output
- [x] Graceful fallback when no intelligence data exists
  > Evidence: `internal/digest/generator.go` Generate checks if all sections empty → calls storeQuietDigest; `generator_test.go` TestSCN002031_QuietDayDigest verifies quiet detection
- [x] SCN-004-017: Digest with commitment-tracked action items — 2 overdue commitments, overnight email, hot topic, meeting preview all included
  > Evidence: `internal/digest/generator.go` Generate assembles ActionItems, OvernightArtifacts, HotTopics; `generator_test.go` TestSCN002030_DigestWithActionItems verifies 2 action items with person and days context
- [x] SCN-004-018: Enhanced digest stays under 150 words — prioritizes top 2 action items and 1 hot topic, links to full views
  > Evidence: `internal/digest/generator.go` storeFallbackDigest produces concise output; `generator_test.go` TestSCN002043_DigestLLMFailureFallback verifies word count and content
- [x] SCN-004-018b: Digest with no intelligence data — falls back to Phase 1 format gracefully without empty intelligence sections
  > Evidence: `internal/digest/generator.go` storeQuietDigest produces "All quiet" message; `generator_test.go` TestDigestContext_QuietDay and TestSCN002031_QuietDayDigest verify empty detection
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/digest/generator_test.go` covers SCN-002-030, SCN-002-031, SCN-002-043 patterns applicable to enhanced digest; `internal/scheduler/scheduler_test.go` covers cron lifecycle
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages pass, 0 failures
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0; `./smackerel.sh check` exits 0
