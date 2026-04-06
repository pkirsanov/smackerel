# Scopes: 004 -- Phase 3: Intelligence

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order
1. **Scope 01 — Synthesis Engine**: Cross-domain cluster detection (pgvector + topic co-occurrence), LLM through-line analysis, contradiction detection, synthesis insight storage
2. **Scope 02 — Commitment Tracking**: Promise detection from email text (piggyback on IMAP processing), action_item lifecycle, auto-resolve from follow-ups, overdue alerts
3. **Scope 03 — Pre-Meeting Briefs**: Calendar polling for upcoming events, per-attendee context assembly, brief generation via LLM, dedup by event ID
4. **Scope 04 — Contextual Alerts**: Alert queue with database-backed lifecycle, bill/return-window/trip-prep/relationship-cooling alert types, batching to max 2/day
5. **Scope 05 — Weekly Synthesis**: Weekly digest with 6 required sections, serendipity resurface, pattern recognition, 250-word cap
6. **Scope 06 — Enhanced Daily Digest**: Upgrade Phase 1 daily digest with intelligence data: commitment-tracked TOP ACTIONS, meeting previews, hot-topic context

### New Types & Signatures
- `SynthesisInsight` struct: through_line, key_tension, suggested_action, source_artifact_ids
- `Alert` struct: type, title, body, priority, status lifecycle (pending→delivered→dismissed/snoozed)
- `synthesis_insights` table: insight_type, through_line, source references
- `alerts` table: alert_type, status, snooze_until, delivery tracking
- NATS subjects: `smk.synthesis.analyze`, `smk.brief.generate`, `smk.weekly.generate`
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

## Scope: 01-synthesis-engine

**Status:** Not Started
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
- For each cluster: publish to NATS `smk.synthesis.analyze` for LLM evaluation
- Python ML sidecar runs Cross-Domain Connection Prompt
- If `has_genuine_connection=true`: store as `synthesis_insights` row, create `SYNTHESIZED_FROM` edges
- If contradiction: create `CONTRADICTS` edge, store both positions without taking sides
- Queue noteworthy insights for weekly synthesis

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Cross-domain cluster detected | Integration | internal/intelligence/synthesis/engine_test.go | SCN-004-001 |
| 2 | Surface overlap rejected | Unit | internal/intelligence/synthesis/analyzer_test.go | SCN-004-002 |
| 3 | Contradiction detected | Integration | internal/intelligence/synthesis/contradiction_test.go | SCN-004-003 |
| 4 | Regression E2E: synthesis quality | E2E | tests/e2e/test_synthesis.sh | SCN-004-001 |
| 5 | Source citation accuracy verified | Integration | internal/intelligence/synthesis/analyzer_test.go | SCN-004-003b |
| 6 | Insufficient data produces no insights | Unit | internal/intelligence/synthesis/engine_test.go | SCN-004-003c |

### Definition of Done
- [ ] Daily synthesis cron identifies cross-domain artifact clusters
- [ ] LLM analysis generates through-lines with source citations
- [ ] Surface-level overlaps silently discarded
- [ ] Contradictions flagged with both positions
- [ ] Synthesis insights stored as first-class entities
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-commitment-tracking

**Status:** Not Started
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

Scenario: SCN-004-006 Auto-resolve on follow-up
  Given a promise exists and a follow-up email is detected in the same thread
  When the system processes the follow-up
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
- Auto-resolve: detect follow-up email in same thread → prompt user to confirm resolution
- Overdue detection: daily cron checks action_items past expected_date by 3+ days
- Surface in daily digest under TOP ACTIONS
- Generate contextual alert for overdue items (R-304)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | User promise detected from email | Integration | internal/intelligence/commitments/detector_test.go | SCN-004-004 |
| 2 | Contact promise detected | Integration | internal/intelligence/commitments/detector_test.go | SCN-004-005 |
| 3 | Follow-up triggers resolve prompt | Integration | internal/intelligence/commitments/resolver_test.go | SCN-004-006 |
| 4 | Overdue alert generated | Unit | internal/intelligence/alerts/commitments_test.go | SCN-004-007 |
| 5 | Regression E2E: commitment lifecycle | E2E | tests/e2e/test_commitments.sh | SCN-004-004 |
| 6 | Casual language rejected (no false positive) | Unit | internal/intelligence/commitments/detector_test.go | SCN-004-007b |

### Definition of Done
- [ ] User-made promises detected from email text with >80% precision
- [ ] Contact-made promises detected and tracked
- [ ] Follow-up emails trigger auto-resolve prompts
- [ ] Overdue commitments generate contextual alerts
- [ ] Action items surfaced in daily digest under TOP ACTIONS
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-pre-meeting-briefs

**Status:** Not Started
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
- Publish context to NATS `smk.brief.generate` for LLM summarization
- Generate 2-3 sentence brief with specific references
- Deliver via alert queue (Telegram / web notification)
- Mark event as briefed to prevent duplicate alerts

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full context brief generated | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |
| 2 | New contact gets minimal brief | Unit | internal/intelligence/alerts/premeeting_test.go | SCN-004-009 |
| 3 | Duplicate briefs prevented | Unit | internal/intelligence/alerts/premeeting_test.go | SCN-004-010 |
| 4 | Regression E2E: brief timing | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |
| 5 | Shared topic context included in brief | Integration | internal/intelligence/alerts/premeeting_test.go | SCN-004-010b |

### Definition of Done
- [ ] Pre-meeting briefs delivered 30 min before events
- [ ] Brief includes: recent emails, shared topics, pending commitments
- [ ] New contacts get "no prior context" message
- [ ] No duplicate briefs for same event
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-contextual-alerts

**Status:** Not Started
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

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Bill reminder generated on schedule | Integration | internal/intelligence/alerts/bills_test.go | SCN-004-011 |
| 2 | Alerts batched to max 2/day | Unit | internal/intelligence/alerts/manager_test.go | SCN-004-012 |
| 3 | Dismissed alert not re-sent | Unit | internal/intelligence/alerts/manager_test.go | SCN-004-013 |
| 4 | Regression E2E: alert delivery | E2E | tests/e2e/test_alerts.sh | SCN-004-011 |
| 5 | Return window alert generated | Integration | internal/intelligence/alerts/bills_test.go | SCN-004-013b |
| 6 | Relationship cooling detected | Integration | internal/intelligence/alerts/commitments_test.go | SCN-004-013c |
| 7 | Trip prep alert at 5-day threshold | Integration | internal/intelligence/alerts/trips_test.go | SCN-004-013d |

### Definition of Done
- [ ] Bill reminders generated 3 days before due date
- [ ] Return window alerts generated 4 days before closing
- [ ] Trip prep alerts generated 5 days before departure
- [ ] Relationship cooling alerts generated for significant interaction drops
- [ ] AlertS batched to maximum 2 per day
- [ ] Dismiss/snooze actions respected
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 05-weekly-synthesis

**Status:** Not Started
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
- Publish to NATS `smk.weekly.generate` for dedicated LLM call
- Generate under 250 words with all 6 sections (skip empty)
- Deliver via configured channel (web UI, Telegram, Slack)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full synthesis with all sections | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |
| 2 | Quiet week skips empty sections | Unit | internal/intelligence/digest/weekly_test.go | SCN-004-015 |
| 3 | Calendar-matched serendipity prioritized | Unit | internal/intelligence/digest/serendipity_test.go | SCN-004-016 |
| 4 | Regression E2E: weekly synthesis | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |
| 5 | Pattern observation detected from timestamps | Unit | internal/intelligence/digest/patterns_test.go | SCN-004-016b |

### Definition of Done
- [ ] Weekly synthesis generated under 250 words with required sections
- [ ] Cross-domain connections cited with source artifacts
- [ ] Topic momentum reported with trends
- [ ] Open loops listed with overdue context
- [ ] Serendipity resurfaces one archive item (calendar/topic matched when possible)
- [ ] Pattern observation included
- [ ] Quiet weeks handled gracefully
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 06-enhanced-daily-digest

**Status:** Not Started
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

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Digest includes overdue commitments | Integration | internal/intelligence/digest/daily_test.go | SCN-004-017 |
| 2 | Digest stays under 150 words | Unit | internal/intelligence/digest/daily_test.go | SCN-004-018 |
| 3 | No-data digest falls back gracefully | Unit | internal/intelligence/digest/daily_test.go | SCN-004-018b |
| 4 | Regression E2E: enhanced digest | E2E | tests/e2e/test_enhanced_digest.sh | SCN-004-017 |

### Definition of Done
- [ ] Daily digest includes commitment-tracked TOP ACTIONS with overdue context
- [ ] Overnight ingestion summary includes source-qualified detail
- [ ] Hot topic acceleration context included
- [ ] Today's meetings include pre-meeting brief previews
- [ ] 150-word cap maintained with graceful prioritization
- [ ] Graceful fallback when no intelligence data exists
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
