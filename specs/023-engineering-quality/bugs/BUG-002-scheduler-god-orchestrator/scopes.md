# Scopes: [BUG-002] Scheduler god-orchestrator — Job interface extraction

## Execution Outline

### Phase Order
1. **Scope 1 — Job interface + scheduler core refactor:** Create `job.go` with `Job` interface (4 methods), refactor `scheduler.go` to accept `[]Job`, remove per-group mutex fields and all domain imports. Expose `Done()`/`WaitGroup()` for digest job shutdown coordination.
2. **Scope 2 — Extract job files + update main.go:** Create 12 job files (one per logical job, alert delivery + production share `alerts_job.go`), each implementing `Job`. Move `deliverPendingAlerts` logic into `AlertDeliveryJob.Run()`. Update `cmd/core/main.go` constructor to build job slice. Migrate digest retry accessors.
3. **Scope 3 — Test migration + verification:** Update `scheduler_test.go` for new constructor/Start signatures. Add per-job unit tests for Name/Schedule/Timeout/Run. Verify full test suite passes.

### New Types & Signatures
```go
// job.go
type Job interface { Name() string; Schedule() string; Timeout() time.Duration; Run(ctx context.Context) error }

// scheduler.go (changed)
func New() *Scheduler
func (s *Scheduler) Start(_ context.Context, jobs []Job) error
func (s *Scheduler) Done() <-chan struct{}
func (s *Scheduler) WaitGroup() *sync.WaitGroup

// Per-job constructors (each in own file)
func NewDigestJob(cronExpr string, gen *digest.Generator, bot *telegram.Bot, done <-chan struct{}, wg *sync.WaitGroup) *DigestJob
func NewSynthesisJob(engine *intelligence.Engine) *SynthesisJob
func NewWeeklySynthesisJob(engine *intelligence.Engine, bot *telegram.Bot) *WeeklySynthesisJob
func NewMonthlyReportJob(engine *intelligence.Engine, bot *telegram.Bot) *MonthlyReportJob
func NewPreMeetingBriefsJob(engine *intelligence.Engine, bot *telegram.Bot) *PreMeetingBriefsJob
func NewAlertDeliveryJob(engine *intelligence.Engine, bot *telegram.Bot) *AlertDeliveryJob
func NewAlertProductionJob(engine *intelligence.Engine) *AlertProductionJob
func NewFrequentLookupsJob(engine *intelligence.Engine, bot *telegram.Bot) *FrequentLookupsJob
func NewSubscriptionDetectionJob(engine *intelligence.Engine) *SubscriptionDetectionJob
func NewResurfacingJob(engine *intelligence.Engine, bot *telegram.Bot) *ResurfacingJob
func NewTopicMomentumJob(lifecycle *topics.Lifecycle) *TopicMomentumJob
func NewRelationshipCoolingJob(engine *intelligence.Engine) *RelationshipCoolingJob
```

### Validation Checkpoints
- After Scope 1: `scheduler.go` compiles with new Job-based Start signature, ≤150 LOC, zero domain imports
- After Scope 2: All 14 files exist, `go build ./internal/scheduler/...` succeeds, `cmd/core/main.go` compiles with new constructor
- After Scope 3: `./smackerel.sh test unit` passes, `./smackerel.sh build` succeeds, `./smackerel.sh check` clean

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | Job interface + scheduler core | `internal/scheduler/scheduler.go`, `internal/scheduler/job.go` | Compilation, LOC ≤150, zero domain imports | [ ] Not started |
| 2 | Extract job files + update main.go | `internal/scheduler/*.go` (12 job files), `cmd/core/main.go` | Compilation, 12 job files exist, all implement Job, main.go compiles | [ ] Not started |
| 3 | Test migration + verification | `internal/scheduler/*_test.go`, `cmd/core/main_test.go` | All unit tests pass, full build green, E2E regression green | [ ] Not started |

---

## Scope 1: Job interface + scheduler core refactor
**Status:** [ ] Not started

### Gherkin Scenarios

```gherkin
Feature: Scheduler Job interface and registration loop

  Scenario: SCN-BUG002-01 — Job interface defines 4 required methods
    Given the file internal/scheduler/job.go exists
    When the Job interface is inspected
    Then it declares exactly Name() string, Schedule() string, Timeout() time.Duration, Run(ctx context.Context) error

  Scenario: SCN-BUG002-02 — Scheduler constructor takes no domain dependencies
    Given the scheduler package is compiled
    When New() is called
    Then it returns a *Scheduler without requiring digest, telegram, intelligence, or topics parameters

  Scenario: SCN-BUG002-03 — Start registers all jobs from the slice
    Given a Scheduler created with New()
    And a slice of 3 mock Jobs with valid cron schedules
    When Start(ctx, jobs) is called
    Then CronEntryCount() returns 3

  Scenario: SCN-BUG002-04 — Start rejects invalid cron schedule
    Given a Scheduler created with New()
    And a mock Job whose Schedule() returns "invalid-cron"
    When Start(ctx, jobs) is called
    Then it returns an error containing "register job"

  Scenario: SCN-BUG002-05 — Overlapping job execution is skipped via TryLock
    Given a Scheduler with a registered Job whose Run() takes 5 seconds
    When the cron fires twice within the 5-second window
    Then the second invocation is skipped with a "skipping overlapping job" log warning
    And Run() is NOT called concurrently for the same job

  Scenario: SCN-BUG002-06 — Different jobs run concurrently without blocking
    Given a Scheduler with 2 registered Jobs (JobA and JobB)
    And both have a 1-second cron schedule
    When both fire simultaneously
    Then both Run() methods execute concurrently (no shared mutex between different jobs)

  Scenario: SCN-BUG002-07 — scheduler.go has zero domain package imports
    Given the scheduler refactoring is complete
    When scheduler.go import block is inspected
    Then it does NOT import digest, intelligence, telegram, or topics packages

  Scenario: SCN-BUG002-08 — scheduler.go is ≤150 LOC
    Given the scheduler refactoring is complete
    When wc -l internal/scheduler/scheduler.go is run
    Then the count is ≤150

  Scenario: SCN-BUG002-09 — Scheduler exposes Done channel and WaitGroup for shutdown coordination
    Given a Scheduler created with New()
    When Done() and WaitGroup() are called
    Then Done() returns a non-nil channel
    And WaitGroup() returns a non-nil *sync.WaitGroup

  Scenario: SCN-BUG002-10 — FormatAlertMessage and AlertTypeIcons remain accessible
    Given job.go contains FormatAlertMessage and AlertTypeIcons
    When called with valid alert parameters
    Then behavior is identical to the pre-refactoring baseline
```

### Implementation Plan

1. **Create `internal/scheduler/job.go`:**
   - `Job` interface with 4 methods: `Name() string`, `Schedule() string`, `Timeout() time.Duration`, `Run(ctx context.Context) error`
   - Move `FormatAlertMessage` function and `AlertTypeIcons` map from `scheduler.go`
   - ~40 LOC

2. **Refactor `internal/scheduler/scheduler.go`:**
   - Remove all 12 per-group mutex fields (`muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`, `muAlertProd`, `muResurface`, `muLookups`, `muSubs`, `muRelCool`)
   - Remove domain struct fields (`digestGen`, `bot`, `engine`, `lifecycle`)
   - Remove `digestPendingRetry`, `digestPendingDate` fields (move to DigestJob in Scope 2)
   - Change `New()` — no parameters, returns `*Scheduler` with only cron, context, done channel, wg
   - Change `Start(_ context.Context, jobs []Job) error` — register from job slice with per-job mutex + TryLock + timeout wrapper
   - Add `Done() <-chan struct{}` and `WaitGroup() *sync.WaitGroup` accessors
   - Remove all inline cron callback closures
   - Remove `deliverPendingAlerts` method (moves to AlertDeliveryJob in Scope 2)
   - Remove all domain package imports
   - Target: ~120 LOC

3. **Temporarily comment or stub digest retry accessors** to allow compilation (full migration in Scope 2)

**Components touched:** `internal/scheduler/scheduler.go`, `internal/scheduler/job.go` (new)
**Consumer impact:** `cmd/core/main.go` will NOT compile until Scope 2 updates it — this is expected and gated.

### Test Plan

| Type | ID | Test | File | Description |
|------|----|------|------|-------------|
| Unit | T-BUG002-01 | `TestJobInterfaceExists` | `scheduler/job_test.go` | Verify interface compiles with mock implementation |
| Unit | T-BUG002-02 | `TestNew_NoArgs` | `scheduler/scheduler_test.go` | `New()` returns non-nil Scheduler with zero deps |
| Unit | T-BUG002-03 | `TestStart_RegistersAllJobs` | `scheduler/scheduler_test.go` | 3 mock jobs → CronEntryCount == 3 |
| Unit | T-BUG002-04 | `TestStart_RejectsInvalidCron` | `scheduler/scheduler_test.go` | Mock job with bad schedule → error |
| Unit | T-BUG002-05 | `TestOverlappingJobSkipped` | `scheduler/scheduler_test.go` | Slow mock job, second fire → skip log |
| Unit | T-BUG002-06 | `TestDifferentJobsConcurrent` | `scheduler/scheduler_test.go` | Two jobs fire simultaneously, both run |
| Unit | T-BUG002-07 | `TestSchedulerGo_NoDomainImports` | `scheduler/scheduler_test.go` | `go list -f '{{.Imports}}'` or file grep |
| Unit | T-BUG002-08 | `TestSchedulerGo_LOCTarget` | `scheduler/scheduler_test.go` | `wc -l` ≤ 150 |
| Unit | T-BUG002-09 | `TestDoneAndWaitGroup` | `scheduler/scheduler_test.go` | Done() non-nil, WaitGroup() non-nil |
| Unit | T-BUG002-10 | `TestFormatAlertMessage` | `scheduler/job_test.go` | Existing FormatAlertMessage tests continue to pass |

### Definition of Done

- [ ] `internal/scheduler/job.go` exists with `Job` interface (4 methods)
- [ ] `internal/scheduler/scheduler.go` is ≤150 LOC
- [ ] `scheduler.go` has zero imports from `digest`, `intelligence`, `telegram`, `topics`
- [ ] `New()` constructor takes no domain dependency parameters
- [ ] `Start()` accepts `[]Job` and registers via loop with per-job TryLock/timeout wrapper
- [ ] 12 per-group named mutex fields removed from Scheduler struct
- [ ] `Done()` and `WaitGroup()` accessors exposed
- [ ] `FormatAlertMessage` and `AlertTypeIcons` moved to `job.go`
- [ ] `go build ./internal/scheduler/...` compiles (job files not yet needed — interface + scheduler core only)
- [ ] All Scope 1 Gherkin scenarios verified

---

## Scope 2: Extract job files + update main.go
**Status:** [ ] Not started

### Gherkin Scenarios

```gherkin
Feature: Job file extraction and main.go wiring

  Scenario: SCN-BUG002-11 — All 12 job files exist in internal/scheduler/
    Given the job extraction is complete
    When the internal/scheduler/ directory is listed
    Then these files exist: digest_job.go, synthesis_job.go, weekly_job.go, monthly_job.go,
         briefs_job.go, alerts_job.go, lookups_job.go, subscriptions_job.go,
         resurfacing_job.go, topics_job.go, relationship_job.go, job.go, scheduler.go

  Scenario: SCN-BUG002-12 — Each job struct implements the Job interface
    Given all 12 job structs are defined
    When each is assigned to a variable of type Job
    Then compilation succeeds for all 12 structs

  Scenario: SCN-BUG002-13 — DigestJob holds retry state and shutdown coordination
    Given a DigestJob constructed with NewDigestJob(...)
    When DigestPendingRetry() is called before any retry
    Then it returns false
    When SetDigestPending(true, "2026-04-15") is called
    Then DigestPendingRetry() returns true and DigestPendingDate() returns "2026-04-15"

  Scenario: SCN-BUG002-14 — DigestJob background goroutine respects shutdown
    Given a DigestJob constructed with a done channel
    When the done channel is closed
    Then any in-progress background polling goroutine exits within 5 seconds

  Scenario: SCN-BUG002-15 — AlertDeliveryJob.Run delivers pending alerts
    Given an AlertDeliveryJob with a mock engine returning 2 pending alerts
    When Run(ctx) is called
    Then both alerts are delivered via bot.SendAlertMessage
    And both are marked delivered via engine.MarkAlertDelivered

  Scenario: SCN-BUG002-16 — Job constructors reject nil critical dependencies
    Given NewDigestJob is called with nil generator
    When the constructor executes
    Then it returns a DigestJob whose Run() returns an error (not a panic)

  Scenario: SCN-BUG002-17 — Each job returns correct Name, Schedule, and Timeout
    Given all 12 job constructors are called with valid deps
    When Name(), Schedule(), and Timeout() are called on each
    Then they return the values matching the cron entry table from design.md

  Scenario: SCN-BUG002-18 — main.go constructs job slice and passes to scheduler
    Given main.go uses buildServices() or equivalent
    When the scheduler start block is reached
    Then scheduler.Start() is called with a []Job slice of length 12

  Scenario: SCN-BUG002-19 — No sub-package created under internal/scheduler/
    Given the extraction is complete
    When internal/scheduler/ is inspected for subdirectories
    Then no subdirectories exist (no jobs/ sub-package)

  Scenario: SCN-BUG002-20 — All job files use package scheduler (not a sub-package)
    Given all 12 job files are created
    When the package declaration of each file is inspected
    Then every file declares "package scheduler"
```

### Implementation Plan

1. **Create 12 job files** in `internal/scheduler/` (all `package scheduler`):

   | File | Struct | Domain Imports | Est. LOC |
   |------|--------|---------------|----------|
   | `digest_job.go` | `DigestJob` | `digest`, `telegram` | ~100 |
   | `synthesis_job.go` | `SynthesisJob` | `intelligence` | ~30 |
   | `weekly_job.go` | `WeeklySynthesisJob` | `intelligence`, `telegram` | ~25 |
   | `monthly_job.go` | `MonthlyReportJob` | `intelligence`, `telegram` | ~25 |
   | `briefs_job.go` | `PreMeetingBriefsJob` | `intelligence`, `telegram` | ~25 |
   | `alerts_job.go` | `AlertDeliveryJob`, `AlertProductionJob` | `intelligence`, `telegram` | ~70 |
   | `lookups_job.go` | `FrequentLookupsJob` | `intelligence`, `telegram` | ~55 |
   | `subscriptions_job.go` | `SubscriptionDetectionJob` | `intelligence` | ~20 |
   | `resurfacing_job.go` | `ResurfacingJob` | `intelligence`, `telegram` | ~40 |
   | `topics_job.go` | `TopicMomentumJob` | `topics` | ~20 |
   | `relationship_job.go` | `RelationshipCoolingJob` | `intelligence` | ~20 |

2. **Migrate digest retry state** from Scheduler to DigestJob:
   - `DigestJob` holds `pendingRetry bool`, `pendingDate string`, guarded by its own `sync.Mutex`
   - Scheduler holds `*DigestJob` reference for backward-compatible accessor delegation (`DigestPendingRetry()`, `DigestPendingDate()`, `SetDigestPending()`)

3. **Migrate `deliverPendingAlerts`** from Scheduler method → `AlertDeliveryJob.Run()` body

4. **Update `cmd/core/main.go`:**
   - Change `scheduler.New(digestGen, tgBot, intEngine, topicLifecycle)` → `scheduler.New()`
   - Build `[]scheduler.Job` slice with 12 job constructors
   - Change `sched.Start(ctx, cfg.DigestCron)` → `sched.Start(ctx, jobs)`
   - Pass `sched.Done()` and `sched.WaitGroup()` to `NewDigestJob`

5. **Verify:** `go build ./internal/scheduler/...` and `go build ./cmd/core/...` succeed

**Components touched:** `internal/scheduler/*.go` (12 new files), `cmd/core/main.go`
**Change Boundary:** Only `internal/scheduler/` and `cmd/core/main.go`. No changes to `internal/digest/`, `internal/intelligence/`, `internal/telegram/`, `internal/topics/`.

### Test Plan

| Type | ID | Test | File | Description |
|------|----|------|------|-------------|
| Unit | T-BUG002-11 | `TestDigestJobInterface` | `scheduler/digest_job_test.go` | DigestJob implements Job interface |
| Unit | T-BUG002-12 | `TestDigestJobRetryState` | `scheduler/digest_job_test.go` | SetDigestPending/DigestPendingRetry/DigestPendingDate thread-safe |
| Unit | T-BUG002-13 | `TestDigestJobNilGuard` | `scheduler/digest_job_test.go` | NewDigestJob with nil gen → Run() returns error |
| Unit | T-BUG002-14 | `TestSynthesisJobInterface` | `scheduler/synthesis_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-15 | `TestWeeklyJobInterface` | `scheduler/weekly_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-16 | `TestMonthlyJobInterface` | `scheduler/monthly_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-17 | `TestBriefsJobInterface` | `scheduler/briefs_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-18 | `TestAlertDeliveryJobInterface` | `scheduler/alerts_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-19 | `TestAlertProductionJobInterface` | `scheduler/alerts_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-20 | `TestLookupsJobInterface` | `scheduler/lookups_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-21 | `TestSubscriptionsJobInterface` | `scheduler/subscriptions_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-22 | `TestResurfacingJobInterface` | `scheduler/resurfacing_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-23 | `TestTopicMomentumJobInterface` | `scheduler/topics_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-24 | `TestRelationshipCoolingJobInterface` | `scheduler/relationship_job_test.go` | Name/Schedule/Timeout correct |
| Unit | T-BUG002-25 | `TestAllJobsScheduleValues` | `scheduler/job_test.go` | All 12 jobs' Schedule() matches design.md cron table |
| Unit | T-BUG002-26 | `TestAllJobsTimeoutValues` | `scheduler/job_test.go` | All 12 jobs' Timeout() matches design.md timeout table |
| Unit | T-BUG002-27 | `TestNoSubdirectories` | `scheduler/scheduler_test.go` | `os.ReadDir` confirms no subdirs |
| Unit | T-BUG002-28 | `TestBuild_Scheduler` | `scheduler/scheduler_test.go` | `go build ./internal/scheduler/...` succeeds |
| Unit | T-BUG002-29 | `TestBuild_CmdCore` | `cmd/core/main_test.go` | `go build ./cmd/core/...` succeeds |

### Definition of Done

- [ ] 12 job files created in `internal/scheduler/` (all `package scheduler`, NO sub-package)
- [ ] Each job struct implements `Job` interface (Name, Schedule, Timeout, Run)
- [ ] `DigestJob` holds retry state with thread-safe accessors
- [ ] `DigestJob` receives `done <-chan struct{}` and `wg *sync.WaitGroup` for shutdown coordination
- [ ] `deliverPendingAlerts` logic moved entirely into `AlertDeliveryJob.Run()`
- [ ] `cmd/core/main.go` constructs `[]scheduler.Job` slice with 12 entries
- [ ] `cmd/core/main.go` calls `scheduler.New()` (no domain args) and `sched.Start(ctx, jobs)`
- [ ] `go build ./internal/scheduler/...` compiles cleanly
- [ ] `go build ./cmd/core/...` compiles cleanly
- [ ] No sub-package `internal/scheduler/jobs/` exists
- [ ] All job files declare `package scheduler`
- [ ] All Scope 2 Gherkin scenarios verified

---

## Scope 3: Test migration + full verification
**Status:** [ ] Not started

### Gherkin Scenarios

```gherkin
Feature: Test migration and full regression verification

  Scenario: SCN-BUG002-21 — Existing scheduler tests adapted to new signatures
    Given scheduler_test.go tests are updated for New() and Start(ctx, []Job)
    When ./smackerel.sh test unit is run
    Then all scheduler package tests pass

  Scenario: SCN-BUG002-22 — CronEntryCount reflects job slice length
    Given a Scheduler started with 12 jobs
    When CronEntryCount() is called
    Then it returns 12

  Scenario: SCN-BUG002-23 — Concurrent retry access is thread-safe
    Given a DigestJob
    When 100 goroutines concurrently call SetDigestPending and DigestPendingRetry
    Then no race condition occurs (pass with -race flag)

  Scenario: SCN-BUG002-24 — Concurrency guard skips same-job overlap
    Given a Scheduler with a slow mock Job (1-second Run)
    When the cron fires twice in rapid succession for the same job
    Then the first Run completes and the second is skipped

  Scenario: SCN-BUG002-25 — Full unit test suite passes
    Given all scheduler refactoring is complete
    When ./smackerel.sh test unit is run
    Then exit code is 0 with zero failures across all packages

  Scenario: SCN-BUG002-26 — Full build succeeds
    Given all refactoring is complete
    When ./smackerel.sh build is run
    Then it exits 0

  Scenario: SCN-BUG002-27 — Check and lint pass
    Given all refactoring is complete
    When ./smackerel.sh check is run
    Then it exits 0
```

### Implementation Plan

1. **Update `internal/scheduler/scheduler_test.go`:**
   - `TestNew` → call `New()` with no args
   - `TestStart_InvalidCron` → pass mock job with bad schedule
   - `TestStart_ValidCron` → pass mock jobs, verify CronEntryCount
   - `TestSCN002060_CronEntries` → pass all 12 real jobs, verify count == 12
   - `TestCronConcurrencyGuard_*` → rewrite to use mock jobs instead of named mutex fields
   - Move digest-specific retry tests to `digest_job_test.go`

2. **Add per-job test files** (one `_test.go` per job file) verifying:
   - Name/Schedule/Timeout return correct values
   - Run(ctx) with nil dependencies returns error (not panic)
   - Interface compliance (compile-time `var _ Job = (*StructName)(nil)`)

3. **Run full verification:**
   - `./smackerel.sh test unit`
   - `./smackerel.sh build`
   - `./smackerel.sh check`

**Components touched:** `internal/scheduler/*_test.go`, `cmd/core/main_test.go` (if signatures changed)

### Test Plan

| Type | ID | Test | File | Description |
|------|----|------|------|-------------|
| Unit | T-BUG002-30 | `TestNew_NoArgs` (migrated) | `scheduler/scheduler_test.go` | Updated for new constructor |
| Unit | T-BUG002-31 | `TestStart_InvalidCron` (migrated) | `scheduler/scheduler_test.go` | Mock job with bad schedule |
| Unit | T-BUG002-32 | `TestCronEntryCount_12Jobs` | `scheduler/scheduler_test.go` | 12 jobs → count == 12 |
| Unit | T-BUG002-33 | `TestConcurrencyGuard_SameJobSkipped` (migrated) | `scheduler/scheduler_test.go` | Slow mock job overlap test |
| Unit | T-BUG002-34 | `TestConcurrentRetryAccess_Race` | `scheduler/digest_job_test.go` | 100 goroutines, -race clean |
| Unit | T-BUG002-35 | `TestRetryFieldLifecycle` (migrated) | `scheduler/digest_job_test.go` | Set/clear/read cycle |
| Functional | T-BUG002-36 | `TestSchedulerStartStop_FullCycle` | `scheduler/scheduler_test.go` | Start with mock jobs, Stop, verify drain |
| E2E-API | T-BUG002-37 | Regression: full test suite | `./smackerel.sh test unit` | All packages pass |
| E2E-API | T-BUG002-38 | Regression: full build | `./smackerel.sh build` | Clean build |
| E2E-API | T-BUG002-39 | Regression: check + lint | `./smackerel.sh check` | Clean check |

### Definition of Done

- [ ] All existing scheduler tests migrated to new constructor/Start signatures
- [ ] Per-job test files exist for all 12 job structs
- [ ] Each job test verifies Name/Schedule/Timeout correctness
- [ ] Each job test verifies Run(ctx) with nil deps returns error (not panic)
- [ ] Each job struct has compile-time interface check (`var _ Job = (*Struct)(nil)`)
- [ ] Concurrent retry access test passes with `-race`
- [ ] Concurrency guard (TryLock overlap skip) tested with mock slow job
- [ ] `./smackerel.sh test unit` passes with 0 failures
- [ ] `./smackerel.sh build` exits 0
- [ ] `./smackerel.sh check` exits 0
- [ ] No `TODO`, `FIXME`, or stub code remains in scheduler package
- [ ] Total scheduler package LOC is ~590 across 14 files (same total, distributed)
- [ ] `scheduler.go` remains ≤150 LOC
