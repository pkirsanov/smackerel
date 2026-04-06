# Scopes: 006 -- Phase 5: Advanced Intelligence

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order
1. **Scope 01 — Expertise Mapping**: Multi-dimensional depth scoring per topic, expertise tiers (Novice→Expert), blind-spot detection, growth trajectories
2. **Scope 02 — Learning Paths**: Auto-assembly from 5+ topic resources, LLM difficulty classification, gap detection, progress tracking
3. **Scope 03 — Subscription Tracker**: Email pattern matching for recurring charges, registry with categories, overlap detection, trial expiration warnings
4. **Scope 04 — Serendipity Engine**: Archive item selection with context affinity (calendar/topic/person), weighted random, user response handling
5. **Scope 05 — Monthly Report**: Self-knowledge report with expertise shifts, information diet, subscription summary, learning progress, patterns
6. **Scope 06 — Repeated Lookup Detection**: Search query frequency tracking, 3+ lookups in 30 days triggers quick-reference auto-generation
7. **Scope 07 — Content Creation Fuel**: Writing angle generation from 30+ capture topics, supporting artifact collection, contrarian position detection
8. **Scope 08 — Seasonal Patterns**: Year-over-year capture behavior analysis, seasonal recommendations, 6+ months data required

### New Types & Signatures
- `ExpertiseMap` struct: topics with depth_score, tier, growth_trajectory, blind_spots
- `LearningPath` struct: topic, ordered resources with difficulty, estimated time, gaps
- `Subscription` struct: service_name, amount, frequency, category, status
- `QuickReference` struct: concept, compiled content, source_artifact_ids
- `ContentAngle` struct: title, uniqueness_rationale, supporting_artifacts, format_suggestion
- Tables: `subscriptions`, `learning_progress`, `quick_references`, `search_log`
- NATS subjects: `smk.learning.classify`, `smk.content.analyze`, `smk.monthly.generate`, `smk.quickref.generate`, `smk.seasonal.analyze`
- REST endpoints: `GET /api/expertise`, `GET /api/learning-paths`, `GET /api/subscriptions`, `GET /api/content-fuel`

### Validation Checkpoints
- After Scope 01: Expertise scoring + tier assignment + blind spots verified via E2E
- After Scope 02: Learning path assembly + difficulty ordering + gap detection verified via E2E
- After Scope 03: Subscription detection + overlap flagging verified via E2E
- After Scope 04: Serendipity selection + context matching + user response verified via E2E
- After Scope 05: Monthly report with all sections verified via E2E
- After Scope 06: Lookup detection + quick reference generation verified via E2E
- After Scope 07: Writing angle generation + evidence collection verified via E2E
- After Scope 08: Seasonal pattern detection + recommendations verified via E2E

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

Scenario: SCN-006-003b Expertise map with insufficient data
  Given the system has been running for less than 90 days
  When the user requests their expertise map
  Then a message explains that 90+ days of data are needed for meaningful results
  And whatever partial data exists is shown with a data-maturity warning
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Expertise tiers calculated correctly | Unit | internal/intelligence/expertise/mapper_test.go | SCN-006-001 |
| 2 | Blind spots detected relative to domain | Integration | internal/intelligence/expertise/blind_spots_test.go | SCN-006-002 |
| 3 | Growth trajectory computed | Unit | internal/intelligence/expertise/trajectory_test.go | SCN-006-003 |
| 4 | Regression E2E: expertise map | E2E | tests/e2e/test_expertise.sh | SCN-006-001 |
| 5 | Insufficient data shows warning | Unit | internal/intelligence/expertise/mapper_test.go | SCN-006-003b |

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

Scenario: SCN-006-006b Learning path update on new resource
  Given a TypeScript learning path exists with 8 resources
  When the user captures a new TypeScript article at intermediate level
  Then the path re-assembles with 9 resources
  And the new article is inserted at the appropriate difficulty position
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Resources ordered by difficulty | Integration | internal/intelligence/learning/path_test.go | SCN-006-004 |
| 2 | Gap detected between levels | Unit | internal/intelligence/learning/path_test.go | SCN-006-005 |
| 3 | Progress tracked correctly | Unit | internal/intelligence/learning/progress_test.go | SCN-006-006 |
| 4 | Regression E2E: learning paths | E2E | tests/e2e/test_learning.sh | SCN-006-004 |
| 5 | Path re-assembles on new resource | Integration | internal/intelligence/learning/path_test.go | SCN-006-006b |

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

Scenario: SCN-006-009b Subscription cancelled detection
  Given a cancellation confirmation email is processed
  When subscription detection runs
  Then the subscription status changes to "cancelled"
  And the monthly summary reflects the reduced spend
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Recurring charge detected | Integration | internal/intelligence/finance/subscriptions_test.go | SCN-006-007 |
| 2 | Category overlap flagged | Unit | internal/intelligence/finance/overlap_test.go | SCN-006-008 |
| 3 | Trial expiration alert | Unit | internal/intelligence/finance/subscriptions_test.go | SCN-006-009 |
| 4 | Regression E2E: subscriptions | E2E | tests/e2e/test_subscriptions.sh | SCN-006-007 |
| 5 | Cancellation detected from email | Integration | internal/intelligence/finance/subscriptions_test.go | SCN-006-009b |

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

Scenario: SCN-006-012b Dismissed resurface reduces future priority
  Given the user dismisses a serendipity resurface
  When the serendipity engine selects the next weekly item
  Then the dismissed artifact has lower selection probability
  And it stays archived
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Calendar-matched item prioritized | Unit | internal/intelligence/serendipity/calendar_match_test.go | SCN-006-010 |
| 2 | Topic-matched item prioritized | Unit | internal/intelligence/serendipity/topic_match_test.go | SCN-006-011 |
| 3 | Resurface boosts momentum | Unit | internal/intelligence/serendipity/engine_test.go | SCN-006-012 |
| 4 | Regression E2E: serendipity | E2E | tests/e2e/test_serendipity.sh | SCN-006-010 |
| 5 | Dismissed items deprioritized | Unit | internal/intelligence/serendipity/engine_test.go | SCN-006-012b |

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

Scenario: SCN-006-014b Monthly report with insufficient data
  Given less than 3 months of data exist
  When the monthly report attempts to generate
  Then it produces a simplified report with available data
  And notes which sections require more data history
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Monthly report generated with sections | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |
| 2 | Capture timing patterns detected | Unit | internal/intelligence/meta/patterns_test.go | SCN-006-014 |
| 3 | Regression E2E: monthly report | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |
| 4 | Insufficient data produces simplified report | Unit | internal/intelligence/meta/monthly_test.go | SCN-006-014b |

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

Scenario: SCN-006-016b Quick reference for topic with sparse resources
  Given the user searched "Kotlin coroutines" 4 times
  But only 1 relevant artifact exists
  When the quick reference is generated
  Then it compiles what is available from the single artifact
  And notes: "Limited sources — save more resources about this topic for a richer reference"
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Repeated lookup detected at threshold | Unit | internal/intelligence/meta/lookups_test.go | SCN-006-015 |
| 2 | Quick reference generated and pinned | Integration | internal/intelligence/meta/lookups_test.go | SCN-006-015 |
| 3 | Reference links to source artifacts | Unit | internal/intelligence/meta/lookups_test.go | SCN-006-016 |
| 4 | Regression E2E: lookup detection | E2E | tests/e2e/test_lookups.sh | SCN-006-015 |
| 5 | Sparse-resource reference with limitation note | Unit | internal/intelligence/meta/lookups_test.go | SCN-006-016b |

### Definition of Done
- [ ] Search queries tracked in search_log with normalized hash
- [ ] 3+ lookups in 30 days triggers quick reference creation
- [ ] Quick reference compiled from best-matching saved resources
- [ ] Reference pinned for instant access
- [ ] User notified of new quick reference
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 07-content-creation-fuel

**Status:** Not Started
**Priority:** P2
**Depends On:** 01-expertise-mapping

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-017 Writing angle generation
  Given the user has 35+ captures about "remote work" with diverse perspectives
  When the content creation fuel analysis runs
  Then it generates 3-5 writing angles
  And each angle has: title, uniqueness rationale, 3-5 supporting artifact references
  And at least one angle reflects a contrarian or nuanced position

Scenario: SCN-006-018 Writing angle with supporting evidence
  Given the user explores the angle "The Hidden Cost of Async Communication"
  When the evidence is assembled
  Then 4 specific articles, 2 videos, and 1 personal note are shown as support
  And extracted quotes/key ideas from each source are included

Scenario: SCN-006-018b Topic below threshold returns no angles
  Given the user has only 10 captures about "gardening"
  When the content creation fuel analysis runs for gardening
  Then no angles are generated
  And a message explains: "Need 30+ captures for meaningful writing angles"
```

### Implementation Plan
- Trigger: on-demand or when topic crosses 30-capture threshold
- Query all artifacts for the topic, extract positions, perspectives, quotes
- Publish to NATS `smk.content.analyze` for LLM analysis
- LLM identifies unique perspectives, contrarian views, original insights
- Map supporting evidence per position (3-5 specific references per angle)
- Generate 3-5 writing angle suggestions with format recommendations
- Store as content_fuel entries linked to topic

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Writing angles generated from 30+ captures | Integration | internal/intelligence/content/fuel_test.go | SCN-006-017 |
| 2 | Supporting evidence assembled correctly | Integration | internal/intelligence/content/evidence_test.go | SCN-006-018 |
| 3 | Below-threshold topic rejected | Unit | internal/intelligence/content/fuel_test.go | SCN-006-018b |
| 4 | Regression E2E: content creation fuel | E2E | tests/e2e/test_content_fuel.sh | SCN-006-017 |

### Definition of Done
- [ ] Writing angles generated from topics with 30+ captures
- [ ] Each angle includes title, uniqueness rationale, and 3-5 artifact references
- [ ] Contrarian and nuanced positions detected from user's capture mix
- [ ] Supporting evidence extracted with quotes and key ideas
- [ ] Below-threshold topics return helpful guidance, not empty results
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 08-seasonal-patterns

**Status:** Not Started
**Priority:** P2
**Depends On:** 01-expertise-mapping, 05-monthly-report

### Gherkin Scenarios

```gherkin
Scenario: SCN-006-019 Seasonal pattern detected
  Given 12+ months of capture data exist
  And the user increased fitness captures every January
  When the seasonal pattern analyzer runs
  Then it detects: "January: you typically increase fitness-related captures"
  And surfaces this in the monthly report when the season approaches

Scenario: SCN-006-020 Gift shopping reminder from seasonal data
  Given last December the user started gift shopping on Dec 15
  And 12 items people mentioned wanting have been tracked this year
  When November arrives
  Then the monthly report includes: "Last year you started gift shopping Dec 15. Here are items people mentioned wanting."

Scenario: SCN-006-020b Insufficient data for seasonal analysis
  Given the system has only 4 months of data
  When the seasonal analyzer runs
  Then no seasonal patterns are reported
  And the feature remains dormant until 6+ months of data exist
```

### Implementation Plan
- Requires 6+ months of data; gracefully dormant before threshold
- Publish capture patterns by month to NATS `smk.seasonal.analyze` for LLM analysis
- Compare current month patterns to same month in prior periods
- Detect: capture volume changes, topic distribution shifts, behavioral triggers
- Gift-list integration: cross-reference R-405 gift preferences with seasonal timing
- Surface patterns in monthly report when seasonal context applies
- Maximum 1 seasonal observation per monthly report

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Seasonal pattern detected from year-over-year data | Integration | internal/intelligence/meta/patterns_test.go | SCN-006-019 |
| 2 | Gift shopping reminder surfaces in November | Unit | internal/intelligence/meta/patterns_test.go | SCN-006-020 |
| 3 | Insufficient data keeps feature dormant | Unit | internal/intelligence/meta/patterns_test.go | SCN-006-020b |
| 4 | Regression E2E: seasonal patterns | E2E | tests/e2e/test_seasonal.sh | SCN-006-019 |

### Definition of Done
- [ ] Seasonal patterns detected from 6+ months of capture data
- [ ] Year-over-year and monthly comparisons generated
- [ ] Gift-shopping reminders integrated with people intelligence gift-list data
- [ ] Insufficient data handled gracefully (dormant until threshold)
- [ ] Maximum 1 seasonal observation per monthly report
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
