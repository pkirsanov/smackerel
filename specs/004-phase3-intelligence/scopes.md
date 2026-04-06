# Scopes: 004 -- Phase 3: Intelligence

Links: [spec.md](spec.md) | [design.md](design.md)

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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Cross-domain cluster detected | Integration | internal/intelligence/synthesis/engine_test.go | SCN-004-001 |
| 2 | Surface overlap rejected | Unit | internal/intelligence/synthesis/analyzer_test.go | SCN-004-002 |
| 3 | Contradiction detected | Integration | internal/intelligence/synthesis/contradiction_test.go | SCN-004-003 |
| 4 | Regression E2E: synthesis quality | E2E | tests/e2e/test_synthesis.sh | SCN-004-001 |

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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | User promise detected from email | Integration | internal/intelligence/commitments/detector_test.go | SCN-004-004 |
| 2 | Contact promise detected | Integration | internal/intelligence/commitments/detector_test.go | SCN-004-005 |
| 3 | Follow-up triggers resolve prompt | Integration | internal/intelligence/commitments/resolver_test.go | SCN-004-006 |
| 4 | Overdue alert generated | Unit | internal/intelligence/alerts/commitments_test.go | SCN-004-007 |
| 5 | Regression E2E: commitment lifecycle | E2E | tests/e2e/test_commitments.sh | SCN-004-004 |

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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full context brief generated | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |
| 2 | New contact gets minimal brief | Unit | internal/intelligence/alerts/premeeting_test.go | SCN-004-009 |
| 3 | Duplicate briefs prevented | Unit | internal/intelligence/alerts/premeeting_test.go | SCN-004-010 |
| 4 | Regression E2E: brief timing | E2E | tests/e2e/test_premeeting.sh | SCN-004-008 |

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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Bill reminder generated on schedule | Integration | internal/intelligence/alerts/bills_test.go | SCN-004-011 |
| 2 | Alerts batched to max 2/day | Unit | internal/intelligence/alerts/manager_test.go | SCN-004-012 |
| 3 | Dismissed alert not re-sent | Unit | internal/intelligence/alerts/manager_test.go | SCN-004-013 |
| 4 | Regression E2E: alert delivery | E2E | tests/e2e/test_alerts.sh | SCN-004-011 |

### Definition of Done
- [ ] Bill reminders generated 3 days before due date
- [ ] Return window alerts generated 4 days before closing
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Full synthesis with all sections | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |
| 2 | Quiet week skips empty sections | Unit | internal/intelligence/digest/weekly_test.go | SCN-004-015 |
| 3 | Calendar-matched serendipity prioritized | Unit | internal/intelligence/digest/serendipity_test.go | SCN-004-016 |
| 4 | Regression E2E: weekly synthesis | E2E | tests/e2e/test_weekly_synthesis.sh | SCN-004-014 |

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
