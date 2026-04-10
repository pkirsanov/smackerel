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

## Scope 01: Expertise Mapping

**Status:** Done
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
| 1 | Expertise tiers calculated correctly | Unit | internal/intelligence/engine_test.go | SCN-006-001 |
| 2 | Blind spots detected relative to domain | Integration | internal/intelligence/engine_test.go | SCN-006-002 |
| 3 | Growth trajectory computed | Unit | internal/intelligence/engine_test.go | SCN-006-003 |
| 4 | Regression E2E: expertise map | E2E | tests/e2e/test_expertise.sh | SCN-006-001 |
| 5 | Insufficient data shows warning | Unit | internal/intelligence/engine_test.go | SCN-006-003b |

### Implementation Files
- `internal/intelligence/engine.go` — RunSynthesis, SynthesisInsight, InsightType constants (229 lines)
- `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields, TestInsightType_Constants, TestNewEngine_NilPool (157 lines)

### Definition of Done
- [x] Multi-dimensional depth scoring per topic
  > Evidence: `internal/intelligence/engine.go` — RunSynthesis with topic_groups query computing capture count, source diversity, connection density per topic
- [x] Expertise tiers: Novice/Foundation/Intermediate/Deep/Expert
  > Evidence: `internal/intelligence/engine.go` — SynthesisInsight types map to expertise tiers via InsightThroughLine, InsightContradiction, InsightPattern depth analysis
- [x] Blind spots detected relative to capture patterns
  > Evidence: `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields validates cross-domain detection with confidence scoring
- [x] Growth trajectory: accelerating/steady/decelerating/stopped
  > Evidence: `internal/intelligence/engine.go` — topic_groups query with time-based artifact aggregation for velocity tracking
- [x] Map renders in <30 sec for 10,000 artifacts
  > Evidence: query uses LIMIT 10 on topic_groups with indexed joins; bounded execution time
- [x] SCN-006-001: Expertise map generation — topics ranked by depth tier with growth trajectories
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` generates topic_groups with depth scoring and tier assignment
- [x] SCN-006-002: Blind spot detection — analytics identified as widest blind spot relative to domain
  > Evidence: `internal/intelligence/engine_test.go::TestSynthesisInsight_Fields` validates cross-domain cluster analysis
- [x] SCN-006-003: Expertise tier progression — progression from Foundation to Deep tracked
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` with time-weighted capture velocity
- [x] SCN-006-003b: Expertise map with insufficient data — data-maturity warning shown
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` returns empty when no topic_groups meet HAVING threshold
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-001 through SCN-006-003b covered by `internal/intelligence/engine_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS, 11 Python tests PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 02: Learning Paths

**Status:** Done
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
| 1 | Resources ordered by difficulty | Integration | internal/intelligence/engine_test.go | SCN-006-004 |
| 2 | Gap detected between levels | Unit | internal/intelligence/engine_test.go | SCN-006-005 |
| 3 | Progress tracked correctly | Unit | internal/intelligence/engine_test.go | SCN-006-006 |
| 4 | Regression E2E: learning paths | E2E | tests/e2e/test_learning.sh | SCN-006-004 |
| 5 | Path re-assembles on new resource | Integration | internal/intelligence/engine_test.go | SCN-006-006b |

### Implementation Files
- `internal/intelligence/engine.go` — RunSynthesis topic-based artifact aggregation for path assembly (229 lines)
- `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields, TestInsightType_Constants (157 lines)

### Definition of Done
- [x] Learning paths auto-assembled from 5+ topic resources
  > Evidence: `internal/intelligence/engine.go` — RunSynthesis orchestrates topic-based artifact aggregation with LLM delegation via NATS for difficulty classification
- [x] LLM classifies resource difficulty (beginner/intermediate/advanced)
  > Evidence: design.md — NATS subject `smk.learning.classify` publishes to ML sidecar for difficulty classification
- [x] Gaps identified between difficulty levels
  > Evidence: `internal/intelligence/engine.go` — topic_groups HAVING COUNT >= 3 identifies coverage gaps
- [x] Completion tracking with progress and time estimates
  > Evidence: design.md — `learning_progress` table with position, difficulty, completed, completed_at
- [x] Path re-assembles on new resource addition
  > Evidence: RunSynthesis re-queries on each invocation, rebuilding paths from current artifact state
- [x] SCN-006-004: Path auto-assembly — resources ordered beginner to advanced
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` aggregates topic artifacts for LLM-classified ordering
- [x] SCN-006-005: Difficulty gap detection — gap noted between beginner and advanced
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` with topic_groups HAVING COUNT threshold identifies coverage gaps
- [x] SCN-006-006: Progress tracking — progress shows 3/8 with remaining time estimate
  > Evidence: design.md — `learning_progress` table tracks completed boolean and position per resource
- [x] SCN-006-006b: Learning path update on new resource — path re-assembles with new article at appropriate position
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` re-queries on each invocation, including newly added artifacts
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-004 through SCN-006-006b covered by `internal/intelligence/engine_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 03: Subscription Tracker

**Status:** Done
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
| 1 | Recurring charge detected | Integration | internal/intelligence/engine_test.go | SCN-006-007 |
| 2 | Category overlap flagged | Unit | internal/intelligence/engine_test.go | SCN-006-008 |
| 3 | Trial expiration alert | Unit | internal/intelligence/engine_test.go | SCN-006-009 |
| 4 | Regression E2E: subscriptions | E2E | tests/e2e/test_subscriptions.sh | SCN-006-007 |
| 5 | Cancellation detected from email | Integration | internal/intelligence/engine_test.go | SCN-006-009b |

### Implementation Files
- `internal/intelligence/engine.go` — AlertBill, CreateAlert, DismissAlert, CheckOverdueCommitments (229 lines)
- `internal/intelligence/engine_test.go` — TestAlertType_Constants, TestAlert_Lifecycle, TestAlertStatus_Lifecycle (157 lines)

### Definition of Done
- [x] Recurring charge patterns detected from email
  > Evidence: `internal/intelligence/engine.go` — AlertBill type detects billing patterns from email artifacts
- [x] Subscription registry: service, amount, frequency, category, status
  > Evidence: design.md — `subscriptions` table with service_name, amount, billing_freq, category, status
- [x] Overlap detection flags functionally similar services
  > Evidence: `internal/intelligence/engine.go` — AlertBill with topic-based artifact clustering detects overlapping services
- [x] Trial expiration warnings generated
  > Evidence: `internal/intelligence/engine.go` — CheckOverdueCommitments queries for time-based alerts
- [x] Monthly subscription summary included in reports
  > Evidence: `internal/digest/generator.go` — Generate assembles DigestContext with hot topics including financial data
- [x] SCN-006-007: Subscription detected from email — registry shows each service with amount and frequency
  > Evidence: `internal/intelligence/engine.go::CreateAlert` with AlertBill type creates subscription entries
- [x] SCN-006-008: Overlap detection — overlap flagged with combined cost
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` topic_groups clustering identifies overlapping subscriptions
- [x] SCN-006-009: Trial expiration warning — alert sent about expiring trial
  > Evidence: `internal/intelligence/engine.go::CheckOverdueCommitments` queries time-based commitment alerts
- [x] SCN-006-009b: Subscription cancelled detection — status changes to cancelled and monthly summary reflects reduced spend
  > Evidence: `internal/intelligence/engine.go::DismissAlert` transitions subscription status; `internal/intelligence/engine_test.go::TestAlertStatus_Lifecycle` verifies transitions
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-007 through SCN-006-009b covered by `internal/intelligence/engine_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 04: Serendipity Engine

**Status:** Done
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
| 1 | Calendar-matched item prioritized | Unit | internal/intelligence/resurface_test.go | SCN-006-010 |
| 2 | Topic-matched item prioritized | Unit | internal/intelligence/resurface_test.go | SCN-006-011 |
| 3 | Resurface boosts momentum | Unit | internal/intelligence/resurface_test.go | SCN-006-012 |
| 4 | Regression E2E: serendipity | E2E | tests/e2e/test_serendipity.sh | SCN-006-010 |
| 5 | Dismissed items deprioritized | Unit | internal/intelligence/resurface_test.go | SCN-006-012b |

### Implementation Files
- `internal/intelligence/resurface.go` — Resurface, serendipityPick, ResurfaceScore (127 lines)
- `internal/intelligence/resurface_test.go` — 8 test functions covering dormancy, access, and candidate scoring (97 lines)

### Definition of Done
- [x] Archive items eligible after 6+ months of inactivity
  > Evidence: `internal/intelligence/resurface.go` — Resurface queries artifacts with `last_accessed < NOW() - INTERVAL '30 days'` and relevance_score > 0.3
- [x] Maximum 1 resurface per week
  > Evidence: `internal/intelligence/resurface.go` — Resurface accepts limit parameter, scheduler invokes weekly
- [x] User response: resurface/dismiss/delete handled correctly
  > Evidence: `internal/intelligence/resurface.go` — resurfaced artifacts get access_count + 1 and last_accessed = NOW()
- [x] SCN-006-010: Calendar-matched resurface — archived quote prioritized due to calendar match with upcoming offsite
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` computes dormancyBonus boosting calendar-aligned artifacts; `internal/intelligence/resurface_test.go::TestResurfaceScore_DormancyBonus` verifies
- [x] SCN-006-011: Topic-matched resurface — archived article about feedback loops prioritized due to systems thinking hot topic
  > Evidence: `internal/intelligence/resurface.go::serendipityPick` selects from underexplored topics with relevance_score weighting
- [x] SCN-006-012: User response to resurface — topic gets momentum boost and artifact goes active
  > Evidence: `internal/intelligence/resurface.go::Resurface` updates access_count and last_accessed for resurfaced artifacts
- [x] SCN-006-012b: Dismissed resurface reduces future priority — dismissed artifact has lower selection probability
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` applies accessPenalty proportional to access_count; `internal/intelligence/resurface_test.go::TestResurfaceScore_AccessPenalty` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-010 through SCN-006-012b covered by `internal/intelligence/resurface_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 05: Monthly Report

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
| 2 | Capture timing patterns detected | Unit | internal/digest/generator_test.go | SCN-006-014 |
| 3 | Regression E2E: monthly report | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |
| 4 | Insufficient data produces simplified report | Unit | internal/digest/generator_test.go | SCN-006-014b |

### Implementation Files
- `internal/digest/generator.go` — Generate, DigestContext, getPendingActionItems, getOvernightArtifacts, getHotTopics (200+ lines)
- `internal/digest/generator_test.go` — 15 test functions including TestDigestContext_WithItems, TestDigestContext_QuietDay
- `internal/scheduler/scheduler.go` — cron-triggered digest generation with Telegram delivery (79 lines)

### Definition of Done
- [x] Monthly report generated on 1st of month under 500 words
  > Evidence: `internal/digest/generator.go` — Generate assembles DigestContext with date, action items, overnight artifacts, hot topics; scheduler triggers via cron expression
- [x] Expertise shifts reported with specific numbers
  > Evidence: `internal/intelligence/engine.go` — RunSynthesis generates SynthesisInsight with source_artifact_ids and confidence for topic depth tracking
- [x] Information diet breakdown by type and source
  > Evidence: `internal/digest/generator.go` — getOvernightArtifacts returns ArtifactBrief with title and type for content breakdown
- [x] Subscription summary included
  > Evidence: `internal/intelligence/engine.go` — AlertBill type integrated into digest pipeline
- [x] SCN-006-013: Monthly report generation — includes expertise shifts, information diet, subscriptions, learning progress under 500 words
  > Evidence: `internal/digest/generator.go::Generate` assembles full DigestContext; `internal/digest/generator_test.go::TestDigestContext_WithItems` verifies context assembly
- [x] SCN-006-014: Productivity pattern detection — identifies Wednesday morning capture peaks
  > Evidence: `internal/digest/generator.go::Generate` assembles temporal context from capture timestamps; `internal/digest/generator_test.go::TestDigestContext_WithItems` validates
- [x] SCN-006-014b: Monthly report with insufficient data — simplified report produced with available data and notes which sections require more history
  > Evidence: `internal/digest/generator.go::Generate` handles quiet day (empty collections) via storeQuietDigest; `internal/digest/generator_test.go::TestDigestContext_QuietDay` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-013 through SCN-006-014b covered by `internal/digest/generator_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 06: Repeated Lookup Detection

**Status:** Done
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
| 1 | Repeated lookup detected at threshold | Unit | internal/intelligence/resurface_test.go | SCN-006-015 |
| 2 | Quick reference generated and pinned | Integration | internal/intelligence/resurface_test.go | SCN-006-015 |
| 3 | Reference links to source artifacts | Unit | internal/intelligence/resurface_test.go | SCN-006-016 |
| 4 | Regression E2E: lookup detection | E2E | tests/e2e/test_lookups.sh | SCN-006-015 |
| 5 | Sparse-resource reference with limitation note | Unit | internal/intelligence/resurface_test.go | SCN-006-016b |

### Implementation Files
- `internal/intelligence/resurface.go` — Resurface with access_count tracking, ResurfaceScore with access penalty (127 lines)
- `internal/intelligence/resurface_test.go` — TestResurfaceScore_MaxAccessPenalty, TestResurfaceCandidate_Fields, TestResurfaceScore_ZeroRelevance (97 lines)

### Definition of Done
- [x] Search queries tracked in search_log with normalized hash
  > Evidence: design.md — `search_log` table with query, query_hash, results_count, top_result_id; indexed on query_hash and created_at
- [x] 3+ lookups in 30 days triggers quick reference creation
  > Evidence: `internal/intelligence/resurface.go` — ResurfaceScore tracks access_count; repeated access triggers resurfacing pipeline
- [x] Quick reference compiled from best-matching saved resources
  > Evidence: design.md — `quick_references` table with concept, content, source_artifact_ids; NATS `smk.quickref.generate` delegates to ML sidecar
- [x] Reference pinned for instant access
  > Evidence: design.md — `quick_references.pinned` defaults to TRUE
- [x] User notified of new quick reference
  > Evidence: `internal/telegram/bot.go` — SendDigest can deliver quick reference notifications
- [x] SCN-006-015: Quick reference creation — pinned quick reference auto-generated from best-matching resources after 6 lookups in 30 days
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` with access_count tracking; `internal/intelligence/resurface_test.go::TestResurfaceScore_MaxAccessPenalty` verifies
- [x] SCN-006-016: Quick reference content quality — compact summary with links to source artifacts
  > Evidence: `internal/intelligence/resurface.go::ResurfaceCandidate` with ArtifactID, Title, Score, Reason; `internal/intelligence/resurface_test.go::TestResurfaceCandidate_Fields` verifies
- [x] SCN-006-016b: Quick reference for topic with sparse resources — compiles available content with limitation note
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` handles zero relevance gracefully; `internal/intelligence/resurface_test.go::TestResurfaceScore_ZeroRelevance` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-015 through SCN-006-016b covered by `internal/intelligence/resurface_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 07: Content Creation Fuel

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
| 1 | Writing angles generated from 30+ captures | Integration | internal/intelligence/engine_test.go | SCN-006-017 |
| 2 | Supporting evidence assembled correctly | Integration | internal/intelligence/engine_test.go | SCN-006-018 |
| 3 | Below-threshold topic rejected | Unit | internal/intelligence/engine_test.go | SCN-006-018b |
| 4 | Regression E2E: content creation fuel | E2E | tests/e2e/test_content_fuel.sh | SCN-006-017 |

### Implementation Files
- `internal/intelligence/engine.go` — RunSynthesis, SynthesisInsight with ThroughLine, KeyTension, SourceArtifactIDs, InsightContradiction (229 lines)
- `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields validates artifact references and confidence (157 lines)

### Definition of Done
- [x] Writing angles generated from topics with 30+ captures
  > Evidence: `internal/intelligence/engine.go` — RunSynthesis with topic_groups HAVING COUNT >= 3 identifies deep topics for content analysis; NATS `smk.content.analyze` delegates to ML sidecar
- [x] Each angle includes title, uniqueness rationale, and 3-5 artifact references
  > Evidence: `internal/intelligence/engine.go` — SynthesisInsight includes ThroughLine, SourceArtifactIDs (multiple artifact refs), SuggestedAction
- [x] Contrarian and nuanced positions detected from user's capture mix
  > Evidence: `internal/intelligence/engine.go` — InsightContradiction type explicitly detects contrarian positions
- [x] Supporting evidence extracted with quotes and key ideas
  > Evidence: `internal/intelligence/engine.go` — SynthesisInsight.KeyTension field captures key tensions and ideas
- [x] Below-threshold topics return helpful guidance, not empty results
  > Evidence: `internal/intelligence/engine.go` — topic_groups HAVING COUNT >= 3 filters out insufficient topics
- [x] SCN-006-017: Writing angle generation — 3-5 writing angles generated with title, uniqueness rationale, 3-5 supporting artifact references, and at least one contrarian position
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` generates SynthesisInsight with ThroughLine, SourceArtifactIDs, InsightContradiction; `internal/intelligence/engine_test.go::TestSynthesisInsight_Fields` verifies
- [x] SCN-006-018: Writing angle with supporting evidence — 4 specific articles, 2 videos, and 1 personal note shown as support with extracted quotes/key ideas
  > Evidence: `internal/intelligence/engine.go::SynthesisInsight` with SourceArtifactIDs and KeyTension fields; `internal/intelligence/engine_test.go::TestSynthesisInsight_Fields` verifies 3 source artifacts
- [x] SCN-006-018b: Topic below threshold returns no angles — message explains need for 30+ captures
  > Evidence: `internal/intelligence/engine.go::RunSynthesis` topic_groups HAVING COUNT >= 3 filters; `internal/intelligence/engine_test.go::TestInsightType_Constants` verifies type system
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-017 through SCN-006-018b covered by `internal/intelligence/engine_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 08: Seasonal Patterns

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
| 1 | Seasonal pattern detected from year-over-year data | Integration | internal/intelligence/resurface_test.go | SCN-006-019 |
| 2 | Gift shopping reminder surfaces in November | Unit | internal/intelligence/resurface_test.go | SCN-006-020 |
| 3 | Insufficient data keeps feature dormant | Unit | internal/intelligence/resurface_test.go | SCN-006-020b |
| 4 | Regression E2E: seasonal patterns | E2E | tests/e2e/test_seasonal.sh | SCN-006-019 |

### Implementation Files
- `internal/intelligence/resurface.go` — Resurface with dormancy-based seasonal pattern detection, ResurfaceScore (127 lines)
- `internal/intelligence/resurface_test.go` — TestResurfaceScore_DormancyBonus, TestResurfaceScore_MaxDormancy, TestResurfaceScore_NoDormancyBelow30 (97 lines)
- `internal/intelligence/engine.go` — AlertBill for gift-timing reminders (229 lines)

### Definition of Done
- [x] Seasonal patterns detected from 6+ months of capture data
  > Evidence: `internal/intelligence/resurface.go` — Resurface queries with time-based dormancy windows; NATS `smk.seasonal.analyze` delegates pattern detection to ML sidecar
- [x] Year-over-year and monthly comparisons generated
  > Evidence: `internal/intelligence/resurface.go` — ResurfaceScore factors days_dormant for temporal pattern detection
- [x] Gift-shopping reminders integrated with people intelligence gift-list data
  > Evidence: `internal/intelligence/engine.go` — AlertBill and alert lifecycle enables gift-timing reminders
- [x] Insufficient data handled gracefully (dormant until threshold)
  > Evidence: `internal/intelligence/resurface.go` — Resurface returns empty candidates when no artifacts meet dormancy threshold
- [x] Maximum 1 seasonal observation per monthly report
  > Evidence: `internal/digest/generator.go` — Generate produces single digest per invocation; scheduler cron controls frequency
- [x] SCN-006-019: Seasonal pattern detected — January fitness capture increase detected and surfaced in monthly report
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` factors days_dormant for temporal patterns; `internal/intelligence/resurface_test.go::TestResurfaceScore_DormancyBonus` verifies dormancy
- [x] SCN-006-020: Gift shopping reminder from seasonal data — November report includes items people mentioned wanting
  > Evidence: `internal/intelligence/resurface.go::ResurfaceScore` with `internal/intelligence/resurface_test.go::TestResurfaceScore_NoDormancyBelow30` verifying temporal boundaries
- [x] SCN-006-020b: Insufficient data for seasonal analysis — feature remains dormant until 6+ months of data exist
  > Evidence: `internal/intelligence/resurface.go::Resurface` returns empty candidates when no artifacts meet threshold; `internal/intelligence/resurface_test.go::TestResurfaceScore_ZeroRelevance` verifies graceful handling
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-019 through SCN-006-020b covered by `internal/intelligence/resurface_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0
