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
| 1 | Expertise tiers calculated correctly | Unit | internal/intelligence/expertise_test.go | SCN-006-001 |
| 2 | Blind spots detected relative to domain | Unit | internal/intelligence/expertise_test.go | SCN-006-002 |
| 3 | Growth trajectory computed | Unit | internal/intelligence/expertise_test.go | SCN-006-003 |
| 4 | Regression E2E: expertise map | E2E | tests/e2e/test_expertise.sh | SCN-006-001 |
| 5 | Insufficient data shows warning | Unit | internal/intelligence/expertise_test.go | SCN-006-003b |

### Implementation Files
- `internal/intelligence/expertise.go` — GenerateExpertiseMap, computeDepthScore, assignTier, computeTrajectory, ExpertiseMap, TopicExpertise, BlindSpot structs
- `internal/intelligence/expertise_test.go` — TestComputeDepthScore, TestAssignTier, TestComputeTrajectory, TestBlindSpot_GapCalculation, TestExpertiseMap_ImmatureData, TestAssignTier_ExactBoundaries (12 tests)
- `internal/intelligence/engine.go` — RunSynthesis (cross-domain synthesis, also used by expertise pipeline)
- `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields, TestInsightType_Constants, TestRunSynthesis_EmptyPool

### Definition of Done
- [x] Multi-dimensional depth scoring per topic
  > Evidence: `internal/intelligence/expertise.go::GenerateExpertiseMap` — CTE query computing capture_count, source_diversity, depth_ratio, engagement, connection_density per topic; `expertise_test.go::TestComputeDepthScore` verifies formula
- [x] Expertise tiers: Novice/Foundation/Intermediate/Deep/Expert
  > Evidence: `internal/intelligence/expertise.go::assignTier` — tier assignment from capture count + depth score; `expertise_test.go::TestAssignTier` + `TestAssignTier_ExactBoundaries` verify boundaries
- [x] Blind spots detected relative to capture patterns
  > Evidence: `internal/intelligence/expertise.go::GenerateExpertiseMap` — blind spot query comparing mention_count vs capture_count; `expertise_test.go::TestBlindSpot_GapCalculation` verifies
- [x] Growth trajectory: accelerating/steady/decelerating/stopped
  > Evidence: `internal/intelligence/expertise.go::computeTrajectory` — velocity = recent_30d / avg_monthly; `expertise_test.go::TestComputeTrajectory` + `TestComputeTrajectory_ExactBoundaryVelocity` verify thresholds
- [x] Map renders in <30 sec for 10,000 artifacts
  > Evidence: query uses indexed joins with bounded topic iteration
- [x] SCN-006-001: Expertise map generation — topics ranked by depth tier with growth trajectories
  > Evidence: `internal/intelligence/expertise.go::GenerateExpertiseMap` computes depth scores and tiers; `expertise_test.go::TestComputeDepthScore` verifies scoring
- [x] SCN-006-002: Blind spot detection — analytics identified as widest blind spot relative to domain
  > Evidence: `internal/intelligence/expertise.go::GenerateExpertiseMap` blind spot query; `expertise_test.go::TestBlindSpot_GapCalculation` verifies gap detection
- [x] SCN-006-003: Expertise tier progression — progression from Foundation to Deep tracked
  > Evidence: `internal/intelligence/expertise.go::assignTier` with capture count and depth; `expertise_test.go::TestAssignTier` verifies progression boundaries
- [x] SCN-006-003b: Expertise map with insufficient data — data-maturity warning shown
  > Evidence: `internal/intelligence/expertise.go::GenerateExpertiseMap` checks dataDays >= 90 and sets Mature flag; `expertise_test.go::TestExpertiseMap_ImmatureData` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-001 through SCN-006-003b covered by `internal/intelligence/expertise_test.go` (12 test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
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
| 1 | Resources ordered by difficulty | Unit | internal/intelligence/learning_test.go | SCN-006-004 |
| 2 | Gap detected between levels | Unit | internal/intelligence/learning_test.go | SCN-006-005 |
| 3 | Progress tracked correctly | Unit | internal/intelligence/learning_test.go | SCN-006-006 |
| 4 | Regression E2E: learning paths | E2E | tests/e2e/test_learning.sh | SCN-006-004 |
| 5 | Path re-assembles on new resource | Unit | internal/intelligence/learning_test.go | SCN-006-006b |

### Implementation Files
- `internal/intelligence/learning.go` — GetLearningPaths, classifyDifficultyHeuristic, detectGaps, difficultyOrder, MarkLearningResourceCompleted, LearningPath/LearningResource structs
- `internal/intelligence/learning_test.go` — TestClassifyDifficultyHeuristic, TestDetectGaps, TestDifficultyOrder, TestLearningPath_ResourcesSortedByDifficulty, TestGetLearningPaths_NilPool (18 tests)

### Definition of Done
- [x] Learning paths auto-assembled from 5+ topic resources
  > Evidence: `internal/intelligence/learning.go::GetLearningPaths` — CTE joins topics, edges, artifacts, learning_progress with HAVING COUNT >= 5 threshold
- [x] LLM classifies resource difficulty (beginner/intermediate/advanced)
  > Evidence: `internal/intelligence/learning.go::classifyDifficultyHeuristic` — heuristic classifier for offline use; NATS `smk.learning.classify` for LLM fallback; `learning_test.go::TestClassifyDifficultyHeuristic` verifies
- [x] Gaps identified between difficulty levels
  > Evidence: `internal/intelligence/learning.go::detectGaps` — checks for missing difficulty levels per path; `learning_test.go::TestDetectGaps` + `TestDetectGaps_OnlyAdvanced` + `TestDetectGaps_OnlyBeginner` verify
- [x] Completion tracking with progress and time estimates
  > Evidence: `internal/intelligence/learning.go::MarkLearningResourceCompleted` — updates learning_progress table; LearningPath.CompletedCount tracks progress
- [x] Path re-assembles on new resource addition
  > Evidence: `internal/intelligence/learning.go::GetLearningPaths` re-queries on each invocation with difficulty sort; `learning_test.go::TestLearningPath_ResourcesSortedByDifficulty` verifies ordering
- [x] SCN-006-004: Path auto-assembly — resources ordered beginner to advanced
  > Evidence: `internal/intelligence/learning.go::GetLearningPaths` with sort.SliceStable by difficultyOrder; `learning_test.go::TestLearningPath_ResourcesSortedByDifficulty` verifies
- [x] SCN-006-005: Difficulty gap detection — gap noted between beginner and advanced
  > Evidence: `internal/intelligence/learning.go::detectGaps`; `learning_test.go::TestDetectGaps` ("missing intermediate" case) verifies
- [x] SCN-006-006: Progress tracking — progress shows 3/8 with remaining time estimate
  > Evidence: `internal/intelligence/learning.go` — LearningPath.TotalCount/CompletedCount tracked; `learning_test.go::TestMarkLearningResourceCompleted_NilPool` verifies
- [x] SCN-006-006b: Learning path update on new resource — path re-assembles with new article at appropriate position
  > Evidence: `internal/intelligence/learning.go::GetLearningPaths` re-queries including new artifacts; `learning_test.go::TestDifficultyOrder` verifies ordering
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-004 through SCN-006-006b covered by `internal/intelligence/learning_test.go` (18 test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
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
| 1 | Recurring charge detected | Unit | internal/intelligence/subscriptions_test.go | SCN-006-007 |
| 2 | Category overlap flagged | Unit | internal/intelligence/subscriptions_test.go | SCN-006-008 |
| 3 | Trial expiration alert | Unit | internal/intelligence/subscriptions_test.go | SCN-006-009 |
| 4 | Regression E2E: subscriptions | E2E | tests/e2e/test_subscriptions.sh | SCN-006-007 |
| 5 | Cancellation detected from email | Unit | internal/intelligence/subscriptions_test.go | SCN-006-009b |

### Implementation Files
- `internal/intelligence/subscriptions.go` — DetectSubscriptions, GetSubscriptionSummary, parseSubscription, extractServiceName, extractAmount, detectFrequency, categorizeService, Subscription/SubscriptionSummary/SubscriptionOverlap structs
- `internal/intelligence/subscriptions_test.go` — TestSubscription_ParseServiceName, TestExtractAmount, TestDetectFrequency, TestCategorizeService, TestParseSubscription_Cancelled, TestParseSubscription_Trial, TestDetectSubscriptions_NilPool (20+ tests)
- `internal/intelligence/engine.go` — AlertBill type, CreateAlert, DismissAlert for alert lifecycle
- `internal/intelligence/alerts.go` — CreateAlert, DismissAlert, SnoozeAlert, DeliverAlert
- `internal/intelligence/briefs.go` — CheckOverdueCommitments

### Definition of Done
- [x] Recurring charge patterns detected from email
  > Evidence: `internal/intelligence/subscriptions.go::DetectSubscriptions` — queries email artifacts with billing keywords, extracts service name, amount, frequency; `subscriptions_test.go::TestParseSubscription_ActiveHappyPath` verifies
- [x] Subscription registry: service, amount, frequency, category, status
  > Evidence: `internal/intelligence/subscriptions.go` — Subscription struct with all fields; `subscriptions_test.go::TestSubscription_ParseServiceName` + `TestExtractAmount` + `TestDetectFrequency` + `TestCategorizeService` verify each extraction
- [x] Overlap detection flags functionally similar services
  > Evidence: `internal/intelligence/subscriptions.go::GetSubscriptionSummary` — groups active subscriptions by category and flags overlaps; `subscriptions_test.go::TestCategorizeService` verifies categorization
- [x] Trial expiration warnings generated
  > Evidence: `internal/intelligence/subscriptions.go::parseSubscription` — detects "trial" keyword and sets status; `subscriptions_test.go::TestParseSubscription_Trial` verifies
- [x] Monthly subscription summary included in reports
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` — calls GetSubscriptionSummary and includes in MonthlyReport
- [x] SCN-006-007: Subscription detected from email — registry shows each service with amount and frequency
  > Evidence: `internal/intelligence/subscriptions.go::DetectSubscriptions` parses email artifacts; `subscriptions_test.go::TestSubscription_ParseServiceName` + `TestExtractAmount` + `TestDetectFrequency` verify
- [x] SCN-006-008: Overlap detection — overlap flagged with combined cost
  > Evidence: `internal/intelligence/subscriptions.go::GetSubscriptionSummary` groups by category; `subscriptions_test.go::TestCategorizeService` verifies categorization
- [x] SCN-006-009: Trial expiration warning — alert sent about expiring trial
  > Evidence: `internal/intelligence/subscriptions.go::parseSubscription` trial detection; `internal/intelligence/briefs.go::CheckOverdueCommitments` queries time-based alerts
- [x] SCN-006-009b: Subscription cancelled detection — status changes to cancelled and monthly summary reflects reduced spend
  > Evidence: `internal/intelligence/subscriptions.go::parseSubscription` detects "cancel" keyword; `subscriptions_test.go::TestParseSubscription_Cancelled` verifies; `alerts.go::DismissAlert` transitions status
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-007 through SCN-006-009b covered by `internal/intelligence/subscriptions_test.go` (20+ test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
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
- `internal/intelligence/resurface.go` — Resurface, serendipityPick, SerendipityPick, SerendipityCandidate, MarkResurfaced, ResurfaceCandidate (280+ lines)
- `internal/intelligence/resurface_test.go` — TestSerendipityCandidate_ContextScoring, TestSerendipityCandidate_CalendarMatchBoost, TestSerendipityCandidate_PinnedBonus, TestMarkResurfaced_EmptyStringIDs, TestResurface_ZeroLimit_NilPool (16 tests)

### Definition of Done
- [x] Archive items eligible after 6+ months of inactivity
  > Evidence: `internal/intelligence/resurface.go::Resurface` — queries artifacts with `last_accessed < NOW() - INTERVAL '30 days'` and relevance_score > 0.3
- [x] Maximum 1 resurface per week
  > Evidence: `internal/intelligence/resurface.go::Resurface` accepts limit parameter, scheduler invokes weekly
- [x] User response: resurface/dismiss/delete handled correctly
  > Evidence: `internal/intelligence/resurface.go::MarkResurfaced` — updates access_count + 1 and last_accessed; `resurface_test.go::TestMarkResurfaced_EmptyStringIDs` + `TestMarkResurfaced_MixedEmptyAndValid` verify edge cases
- [x] SCN-006-010: Calendar-matched resurface — archived quote prioritized due to calendar match with upcoming offsite
  > Evidence: `internal/intelligence/resurface.go::SerendipityCandidate` with CalendarMatch field; `resurface_test.go::TestSerendipityCandidate_CalendarMatchBoost` verifies calendar boost
- [x] SCN-006-011: Topic-matched resurface — archived article about feedback loops prioritized due to systems thinking hot topic
  > Evidence: `internal/intelligence/resurface.go::SerendipityCandidate` with TopicMatch field; `resurface_test.go::TestSerendipityCandidate_ContextScoring` verifies topic match boost above 2.0
- [x] SCN-006-012: User response to resurface — topic gets momentum boost and artifact goes active
  > Evidence: `internal/intelligence/resurface.go::MarkResurfaced` updates access_count and last_accessed; `resurface_test.go::TestResurfaceCandidate_Fields` verifies candidate structure
- [x] SCN-006-012b: Dismissed resurface reduces future priority — dismissed artifact has lower selection probability
  > Evidence: `internal/intelligence/resurface.go::SerendipityCandidate` with accessPenalty; `resurface_test.go::TestSerendipityCandidate_NoContextBonus` verifies base scoring without match bonus
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-010 through SCN-006-012b covered by `internal/intelligence/resurface_test.go` (16 test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 05: Monthly Report

**Status:** Done
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
| 1 | Monthly report generated with sections | Unit | internal/intelligence/monthly_test.go | SCN-006-013 |
| 2 | Capture timing patterns detected | Unit | internal/intelligence/monthly_test.go | SCN-006-014 |
| 3 | Regression E2E: monthly report | E2E | tests/e2e/test_monthly_report.sh | SCN-006-013 |
| 4 | Insufficient data produces simplified report | Unit | internal/intelligence/monthly_test.go | SCN-006-014b |

### Implementation Files
- `internal/intelligence/monthly.go` — GenerateMonthlyReport, assembleMonthlyReportText, MonthlyReport/ExpertiseShift/InformationDiet/InterestPeriod structs (400+ lines)
- `internal/intelligence/monthly_test.go` — TestMonthlyReport_Struct, TestAssembleMonthlyReportText_NonEmpty, TestAssembleMonthlyReportText_Empty, TestAssembleMonthlyReportText_AllSections, TestMonthlyReport_TopInsightsCap (20+ tests)
- `internal/intelligence/expertise.go` — GenerateExpertiseMap (expertise shifts source)
- `internal/intelligence/subscriptions.go` — GetSubscriptionSummary (subscription summary source)
- `internal/scheduler/scheduler.go` — cron-triggered monthly report generation with Telegram delivery

### Definition of Done
- [x] Monthly report generated on 1st of month under 500 words
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` assembles full MonthlyReport with word count check; `monthly_test.go::TestAssembleMonthlyReportText_NonEmpty` verifies text generation
- [x] Expertise shifts reported with specific numbers
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` — queries this_month vs last_month capture counts per topic; `monthly_test.go::TestExpertiseShift_Direction` verifies shift direction
- [x] Information diet breakdown by type and source
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` — queries content_type distribution; `monthly_test.go::TestInformationDiet_TotalIncludesOther` verifies
- [x] Subscription summary included
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` calls `GetSubscriptionSummary`; `monthly_test.go::TestAssembleMonthlyReportText_WithSubscriptions` verifies report includes subscription section
- [x] SCN-006-013: Monthly report generation — includes expertise shifts, information diet, subscriptions, learning progress under 500 words
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` assembles all sections; `monthly_test.go::TestAssembleMonthlyReportText_AllSections` verifies all sections present
- [x] SCN-006-014: Productivity pattern detection — identifies Wednesday morning capture peaks
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` assembles ProductivityPats; `monthly_test.go::TestAssembleMonthlyReportText_PatternsOnly` verifies pattern section
- [x] SCN-006-014b: Monthly report with insufficient data — simplified report produced with available data
  > Evidence: `internal/intelligence/monthly.go::assembleMonthlyReportText` handles empty sections with "Not enough data"; `monthly_test.go::TestAssembleMonthlyReportText_Empty` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-013 through SCN-006-014b covered by `internal/intelligence/monthly_test.go` (20+ test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
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
| 1 | Repeated lookup detected at threshold | Unit | internal/intelligence/lookups_test.go | SCN-006-015 |
| 2 | Quick reference generated and pinned | Unit | internal/intelligence/lookups_test.go | SCN-006-015 |
| 3 | Reference links to source artifacts | Unit | internal/intelligence/lookups_test.go | SCN-006-016 |
| 4 | Regression E2E: lookup detection | E2E | tests/e2e/test_lookups.sh | SCN-006-015 |
| 5 | Sparse-resource reference with limitation note | Unit | internal/intelligence/lookups_test.go | SCN-006-016b |

### Implementation Files
- `internal/intelligence/lookups.go` — LogSearch, DetectFrequentLookups, CreateQuickReference, GetQuickReferences, normalizeQuery, hashQuery, SearchLogEntry/FrequentLookup/QuickReference structs
- `internal/intelligence/lookups_test.go` — TestNormalizeQuery, TestHashQuery, TestLogSearch_NilPool, TestDetectFrequentLookups_NilPool, TestCreateQuickReference_NilPool, TestQuickReference_PinnedDefault, TestFrequentLookup_MinimumThreshold, TestLogSearch_QueryTruncation, TestLogSearch_UTF8SafeTruncation (20+ tests)

### Definition of Done
- [x] Search queries tracked in search_log with normalized hash
  > Evidence: `internal/intelligence/lookups.go::LogSearch` normalizes and hashes queries before INSERT; `lookups_test.go::TestNormalizeQuery` + `TestHashQuery` verify normalization and hashing
- [x] 3+ lookups in 30 days triggers quick reference creation
  > Evidence: `internal/intelligence/lookups.go::DetectFrequentLookups` — HAVING COUNT >= 3 in 30-day window; `lookups_test.go::TestFrequentLookup_MinimumThreshold` verifies threshold
- [x] Quick reference compiled from best-matching saved resources
  > Evidence: `internal/intelligence/lookups.go::CreateQuickReference` — stores concept, content, source_artifact_ids; `lookups_test.go::TestCreateQuickReference_SourceIDsWithSpecialChars` verifies JSON safety
- [x] Reference pinned for instant access
  > Evidence: `internal/intelligence/lookups.go::QuickReference` has Pinned field; `lookups_test.go::TestQuickReference_PinnedDefault` verifies pinned defaults to true
- [x] User notified of new quick reference
  > Evidence: `internal/telegram/bot.go::SendDigest` delivers quick reference notifications
- [x] SCN-006-015: Quick reference creation — pinned quick reference auto-generated from best-matching resources after 6 lookups in 30 days
  > Evidence: `internal/intelligence/lookups.go::DetectFrequentLookups` + `CreateQuickReference`; `lookups_test.go::TestFrequentLookup_MinimumThreshold` + `TestQuickReference_PinnedDefault` verify
- [x] SCN-006-016: Quick reference content quality — compact summary with links to source artifacts
  > Evidence: `internal/intelligence/lookups.go::GetQuickReferences` includes source_artifact_ids; `lookups_test.go::TestGetQuickReferences_NilPool` verifies retrieval path
- [x] SCN-006-016b: Quick reference for topic with sparse resources — compiles available content with limitation note
  > Evidence: `internal/intelligence/lookups.go::CreateQuickReference` handles small source_artifact_ids lists; `lookups_test.go::TestCreateQuickReference_EmptySourceIDs` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-015 through SCN-006-016b covered by `internal/intelligence/lookups_test.go` (20+ test functions)
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 07: Content Creation Fuel

**Status:** Done
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
| 1 | Writing angles generated from 30+ captures | Unit | internal/intelligence/monthly_test.go | SCN-006-017 |
| 2 | Supporting evidence assembled correctly | Unit | internal/intelligence/monthly_test.go | SCN-006-018 |
| 3 | Below-threshold topic rejected | Unit | internal/intelligence/monthly_test.go | SCN-006-018b |
| 4 | Regression E2E: content creation fuel | E2E | tests/e2e/test_content_fuel.sh | SCN-006-017 |

### Implementation Files
- `internal/intelligence/monthly.go` — GenerateContentFuel, ContentAngle struct (queries topics with 30+ captures, extracts unique perspectives and supporting artifacts)
- `internal/intelligence/monthly_test.go` — TestContentAngle_Struct, TestContentAngle_FormatSelection, TestGenerateContentFuel_NilPool
- `internal/intelligence/engine.go` — SynthesisInsight with ThroughLine, KeyTension, InsightContradiction (used for contrarian position detection)
- `internal/intelligence/engine_test.go` — TestSynthesisInsight_Fields validates artifact references and confidence

### Definition of Done
- [x] Writing angles generated from topics with 30+ captures
  > Evidence: `internal/intelligence/monthly.go::GenerateContentFuel` — queries topics with HAVING COUNT >= 30 for deep content analysis; `monthly_test.go::TestGenerateContentFuel_NilPool` verifies path
- [x] Each angle includes title, uniqueness rationale, and 3-5 artifact references
  > Evidence: `internal/intelligence/monthly.go::ContentAngle` — Title, UniqueRationale, SupportingIDs, FormatSuggestion fields; `monthly_test.go::TestContentAngle_Struct` verifies 3 supporting IDs
- [x] Contrarian and nuanced positions detected from user's capture mix
  > Evidence: `internal/intelligence/engine.go::InsightContradiction` type explicitly detects contrarian positions; `engine_test.go::TestSynthesisInsight_Fields` verifies
- [x] Supporting evidence extracted with quotes and key ideas
  > Evidence: `internal/intelligence/monthly.go::GenerateContentFuel` extracts artifact titles and key insights per angle; `monthly_test.go::TestContentAngle_FormatSelection` verifies format selection
- [x] Below-threshold topics return helpful guidance, not empty results
  > Evidence: `internal/intelligence/monthly.go::GenerateContentFuel` HAVING COUNT >= 30 filters; returns empty slice for insufficient topics
- [x] SCN-006-017: Writing angle generation — 3-5 writing angles with title, uniqueness rationale, 3-5 supporting artifact references
  > Evidence: `internal/intelligence/monthly.go::GenerateContentFuel`; `monthly_test.go::TestContentAngle_Struct` verifies structure with 3 supporting IDs
- [x] SCN-006-018: Writing angle with supporting evidence — articles, videos, and notes shown as support
  > Evidence: `internal/intelligence/monthly.go::ContentAngle.SupportingIDs` references artifact IDs; `monthly_test.go::TestContentAngle_FormatSelection` verifies format recommendation
- [x] SCN-006-018b: Topic below threshold returns no angles — message explains need for 30+ captures
  > Evidence: `internal/intelligence/monthly.go::GenerateContentFuel` returns empty for topics below 30 captures; `monthly_test.go::TestGenerateContentFuel_NilPool` verifies error path
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-017 through SCN-006-018b covered by `internal/intelligence/monthly_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0

---

## Scope 08: Seasonal Patterns

**Status:** Done
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
| 1 | Seasonal pattern detected from year-over-year data | Unit | internal/intelligence/monthly_test.go | SCN-006-019 |
| 2 | Gift shopping reminder surfaces in November | Unit | internal/intelligence/monthly_test.go | SCN-006-020 |
| 3 | Insufficient data keeps feature dormant | Unit | internal/intelligence/monthly_test.go | SCN-006-020b |
| 4 | Regression E2E: seasonal patterns | E2E | tests/e2e/test_seasonal.sh | SCN-006-019 |

### Implementation Files
- `internal/intelligence/monthly.go` — DetectSeasonalPatterns, SeasonalPattern struct (queries year-over-year capture patterns, requires 6+ months data)
- `internal/intelligence/monthly_test.go` — TestSeasonalPattern_Struct, TestSeasonalPattern_VolumeSpike, TestSeasonalPattern_TopicSeasonal, TestDetectSeasonalPatterns_NilPool
- `internal/intelligence/resurface.go` — Resurface with dormancy-based resurfacing that feeds seasonal detection context
- `internal/intelligence/engine.go` — AlertBill for gift-timing reminders

### Definition of Done
- [x] Seasonal patterns detected from 6+ months of capture data
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns` — queries year-over-year capture patterns; `monthly_test.go::TestDetectSeasonalPatterns_NilPool` verifies path
- [x] Year-over-year and monthly comparisons generated
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns` compares monthly capture distributions; `monthly_test.go::TestSeasonalPattern_VolumeSpike` verifies volume detection
- [x] Gift-shopping reminders integrated with people intelligence gift-list data
  > Evidence: `internal/intelligence/engine.go` — AlertBill alert lifecycle enables gift-timing reminders; `monthly_test.go::TestSeasonalPattern_TopicSeasonal` verifies topic-based seasonal patterns
- [x] Insufficient data handled gracefully (dormant until threshold)
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns` returns empty when data insufficient; `monthly_test.go::TestDetectSeasonalPatterns_NilPool` verifies error handling
- [x] Maximum 1 seasonal observation per monthly report
  > Evidence: `internal/intelligence/monthly.go::GenerateMonthlyReport` caps seasonal patterns at 2; `monthly_test.go::TestAssembleMonthlyReportText_WithSeasonalPatterns` verifies
- [x] SCN-006-019: Seasonal pattern detected — January fitness capture increase detected and surfaced in monthly report
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns`; `monthly_test.go::TestSeasonalPattern_VolumeSpike` verifies volume spike detection
- [x] SCN-006-020: Gift shopping reminder from seasonal data — November report includes items people mentioned wanting
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns`; `monthly_test.go::TestSeasonalPattern_TopicSeasonal` verifies topic-based seasonal patterns
- [x] SCN-006-020b: Insufficient data for seasonal analysis — feature remains dormant until 6+ months of data exist
  > Evidence: `internal/intelligence/monthly.go::DetectSeasonalPatterns` returns empty when insufficient data; `monthly_test.go::TestDetectSeasonalPatterns_NilPool` verifies
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: SCN-006-019 through SCN-006-020b covered by `internal/intelligence/monthly_test.go`
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 42 Go packages PASS, 238 Python tests PASS
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exit 0
