# Scopes: 006 -- Phase 5: Advanced Intelligence

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Scope: 01-expertise-mapping

**Status:** Not Started
**Priority:** P1
**Depends On:** Phase 3 complete (3+ months data required)

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-001 Expertise map generation
  Given 500+ artifacts across 25 topics exist
  When the user requests their expertise map
  Then topics are ranked by depth tier with growth trajectories

Scenario: SCN-006-002 Blind spot detection
  Given 150 captures about product management but only 8 about analytics
  When the expertise map analyzes patterns
  Then analytics is identified as the widest blind spot

Scenario: SCN-006-003 Expertise tier progression
  Given a topic grew from 15 to 55 captures over 3 months
  When the expertise map updates
  Then the topic shows progression from Foundation to Deep
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Expertise tiers calculated correctly | Unit | internal/intelligence/expertise/mapper_test.go | SCN-006-001 |
| 2 | Blind spots detected relative to domain | Integration | internal/intelligence/expertise/blind_spots_test.go | SCN-006-002 |
| 3 | Growth trajectory computed | Unit | internal/intelligence/expertise/trajectory_test.go | SCN-006-003 |
| 4 | Regression E2E: expertise map | E2E | tests/e2e/test_expertise.sh | SCN-006-001 |

### Definition of Done
- [ ] Multi-dimensional depth scoring per topic
- [ ] Expertise tiers: Novice/Foundation/Intermediate/Deep/Expert
- [ ] Blind spots detected relative to capture patterns
- [ ] Growth trajectory: accelerating/steady/decelerating/stopped
- [ ] Map renders in <30 sec for 10,000 artifacts
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-learning-paths

**Status:** Not Started
**Priority:** P1
**Depends On:** 01-expertise-mapping

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-004 Path auto-assembly
  Given 8 TypeScript resources saved at varying difficulty levels
  When the learning path assembles
  Then resources are ordered beginner -> intermediate -> advanced

Scenario: SCN-006-005 Difficulty gap detection
  Given beginner and advanced Rust resources exist but no intermediate
  When the learning path assembles
  Then a gap is noted between beginner and advanced

Scenario: SCN-006-006 Progress tracking
  Given a learning path with 8 resources
  When the user marks 3 as completed
  Then progress shows 3/8 with remaining time estimate
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Resources ordered by difficulty | Integration | internal/intelligence/learning/path_test.go | SCN-006-004 |
| 2 | Gap detected between levels | Unit | internal/intelligence/learning/path_test.go | SCN-006-005 |
| 3 | Progress tracked correctly | Unit | internal/intelligence/learning/progress_test.go | SCN-006-006 |
| 4 | Regression E2E: learning paths | E2E | tests/e2e/test_learning.sh | SCN-006-004 |

### Definition of Done
- [ ] Learning paths auto-assembled from 5+ topic resources
- [ ] LLM classifies resource difficulty (beginner/intermediate/advanced)
- [ ] Gaps identified between difficulty levels
- [ ] Completion tracking with progress and time estimates
- [ ] Path re-assembles on new resource addition
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-subscription-tracker

**Status:** Not Started
**Priority:** P2
**Depends On:** Phase 2 scope 02 (IMAP connector)

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-007 Subscription detected from email
  Given recurring charge emails from Netflix, Spotify exist
  When subscription detection runs
  Then a registry shows each service with amount and frequency

Scenario: SCN-006-008 Overlap detection
  Given 3 writing tool subscriptions active
  When overlap analysis runs
  Then the overlap is flagged with combined cost

Scenario: SCN-006-009 Trial expiration warning
  Given a trial started 12 days ago with 14-day limit
  When 2 days remain
  Then an alert is sent about the expiring trial
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Recurring charge detected | Integration | internal/intelligence/finance/subscriptions_test.go | SCN-006-007 |
| 2 | Category overlap flagged | Unit | internal/intelligence/finance/overlap_test.go | SCN-006-008 |
| 3 | Trial expiration alert | Unit | internal/intelligence/finance/subscriptions_test.go | SCN-006-009 |
| 4 | Regression E2E: subscriptions | E2E | tests/e2e/test_subscriptions.sh | SCN-006-007 |

### Definition of Done
- [ ] Recurring charge patterns detected from email
- [ ] Subscription registry: service, amount, frequency, category, status
- [ ] Overlap detection flags functionally similar services
- [ ] Trial expiration warnings generated
- [ ] Monthly subscription summary included in reports
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-serendipity-engine

**Status:** Not Started
**Priority:** P1
**Depends On:** Phase 3 scope 05 (weekly synthesis)

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-010 Calendar-matched resurface
  Given an archived quote matches an upcoming offsite event
  When the serendipity engine selects the weekly item
  Then the quote is prioritized due to calendar match

Scenario: SCN-006-011 Topic-matched resurface
  Given "systems thinking" is currently hot
  And an archived article about feedback loops exists
  When the serendipity engine runs
  Then the article is prioritized due to topic match

Scenario: SCN-006-012 User response to resurface
  Given the user receives a serendipity resurface
  When they choose "resurface"
  Then the topic gets a momentum boost and the artifact goes active
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Calendar-matched item prioritized | Unit | internal/intelligence/serendipity/calendar_match_test.go | SCN-006-010 |
| 2 | Topic-matched item prioritized | Unit | internal/intelligence/serendipity/topic_match_test.go | SCN-006-011 |
| 3 | Resurface boosts momentum | Unit | internal/intelligence/serendipity/engine_test.go | SCN-006-012 |
| 4 | Regression E2E: serendipity | E2E | tests/e2e/test_serendipity.sh | SCN-006-010 |

### Definition of Done
- [ ] Archive items eligible after 6+ months of inactivity
- [ ] Calendar event affinity boosts selection probability
- [ ] Hot topic affinity boosts selection probability
- [ ] Maximum 1 resurface per week
- [ ] User response: resurface/dismiss/delete handled correctly
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 05-monthly-report

**Status:** Not Started
**Priority:** P2
**Depends On:** 01-expertise-mapping, 03-subscription-tracker

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-013 Monthly report generation
  Given 3+ months of data exist and it is the 1st of the month
  When the monthly report generates
  Then it includes: expertise shifts, information diet, subscriptions, learning progress
  And is under 500 words

Scenario: SCN-006-014 Productivity pattern detection
  Given capture timestamps show Wednesday morning peaks
  When the report analyzes patterns
  Then it identifies: "Your idea-dense windows are Wednesday mornings"
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Monthly report generated with sections | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |
| 2 | Capture timing patterns detected | Unit | internal/intelligence/meta/patterns_test.go | SCN-006-014 |
| 3 | Regression E2E: monthly report | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |

### Definition of Done
- [ ] Monthly report generated on 1st of month under 500 words
- [ ] Expertise shifts reported with specific numbers
- [ ] Information diet breakdown by type and source
- [ ] Subscription summary included
- [ ] Productivity patterns identified from timestamps
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 06-repeated-lookup-detection

**Status:** Not Started
**Priority:** P2
**Depends On:** Phase 1 scope 05 (semantic search)

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-015 Quick reference creation
  Given the user searched "TypeScript generics" 6 times in 30 days
  When the detector flags this
  Then a pinned quick reference is auto-generated from best-matching resources

Scenario: SCN-006-016 Quick reference content quality
  Given a quick reference for "Python decorators" exists
  When the user views it
  Then it shows a compact summary with links to source artifacts
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Repeated lookup detected at threshold | Unit | internal/intelligence/meta/lookups_test.go | SCN-006-015 |
| 2 | Quick reference generated and pinned | Integration | internal/intelligence/meta/lookups_test.go | SCN-006-015 |
| 3 | Reference links to source artifacts | Unit | internal/intelligence/meta/lookups_test.go | SCN-006-016 |
| 4 | Regression E2E: lookup detection | E2E | tests/e2e/test_lookups.sh | SCN-006-015 |

### Definition of Done
- [ ] Search queries tracked in search_log with normalized hash
- [ ] 3+ lookups in 30 days triggers quick reference creation
- [ ] Quick reference compiled from best-matching saved resources
- [ ] Reference pinned for instant access
- [ ] User notified of new quick reference
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
