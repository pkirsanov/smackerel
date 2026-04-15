# Bug Fix Design: [BUG-002] Scheduler god-orchestrator

## Design Brief

- **Current State:** `internal/scheduler/scheduler.go` is 610 LOC with 12 cron callbacks (13 logical jobs) registered inline in `Start()`, 12 dedicated per-group mutexes, and direct imports of 4 domain packages (`digest`, `intelligence`, `telegram`, `topics`). Every new cron job requires editing this single file.
- **Target State:** Scheduler becomes a thin lifecycle manager (~150 LOC) that registers `Job` interface implementers from a slice. Each job is a standalone file in the same package with its own deps injected at construction.
- **Patterns to Follow:** The `intelligence` package already splits by domain (expertise.go, subscriptions.go, monthly.go, resurface.go, people.go, lookups.go, learning.go). Apply the same single-package multi-file convention.
- **Patterns to Avoid:** Do NOT create `internal/scheduler/jobs/` as a sub-package — Go requires every directory to be a separate package, which would force circular imports (scheduler ↔ jobs) or an interface-only abstraction package. Keep all files in `package scheduler`.
- **Resolved Decisions:** Job interface with 4 methods; scheduler owns per-job mutexes; each job gets deps via constructor injection; no sub-package.
- **Open Questions:** None.

## Root Cause Analysis

### Investigation Summary
Retro hotspot analysis (2026-04-15) identified `internal/scheduler/scheduler.go` as the #1 coupling hub: 22 changes in 30 days, co-changing with 6+ files across 4 packages. Every cron job is an inline closure with duplicated TryLock/timeout/error/delivery boilerplate.

### Root Cause
Procedural design — no job abstraction. Each of the 12 cron.AddFunc callbacks (representing 13 logical jobs) is a bespoke closure in `Start()` that directly calls domain methods, manages its own mutex, and handles delivery inline. The Scheduler struct imports and wires `digest.Generator`, `intelligence.Engine`, `telegram.Bot`, and `topics.Lifecycle` directly.

### Verified Source Analysis (scheduler.go)

**Struct fields (lines 19-55):**
- Domain deps: `digestGen *digest.Generator`, `bot *telegram.Bot`, `engine *intelligence.Engine`, `lifecycle *topics.Lifecycle`
- Retry state: `digestPendingRetry bool`, `digestPendingDate string` (guarded by `mu sync.Mutex`)
- Lifecycle: `cron *cron.Cron`, `baseCtx context.Context`, `baseCancel context.CancelFunc`, `done chan struct{}`, `wg sync.WaitGroup`, `stopOnce sync.Once`
- 12 per-group mutexes: `muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`, `muAlertProd`, `muResurface`, `muLookups`, `muSubs`, `muRelCool`

**Constructor signature (line 58):**
```go
func New(digestGen *digest.Generator, bot *telegram.Bot, engine *intelligence.Engine, lifecycle *topics.Lifecycle) *Scheduler
```

**Start method signature (line 72):**
```go
func (s *Scheduler) Start(_ context.Context, cronExpr string) error
```

### Complete Job Inventory (12 cron entries, 13 logical jobs)

| # | Cron Expression | Mutex | Timeout | Domain Calls | Delivery |
|---|----------------|-------|---------|-------------|----------|
| 1 | `cronExpr` (param) | `muDigest` | 2min | `digestGen.Generate`, `digestGen.GetLatest` | `bot.SendDigest` via background poller goroutine |
| 2 | `0 * * * *` | `muHourly` | 2min | `lifecycle.UpdateAllMomentum` | none |
| 3a | `0 2 * * *` | `muDaily` | 5min | `engine.RunSynthesis` | none |
| 3b | (same entry) | (same) | (same) | `engine.CheckOverdueCommitments` | none |
| 4 | `0 8 * * *` | `muResurface` | 2min | `engine.Resurface`, `engine.MarkResurfaced` | `bot.SendDigest` |
| 5 | `*/5 * * * *` | `muBriefs` | 1min | `engine.GeneratePreMeetingBriefs` | `bot.SendDigest` (per brief) |
| 6 | `0 16 * * 0` | `muWeekly` | 5min | `engine.GenerateWeeklySynthesis` | `bot.SendDigest` |
| 7 | `0 3 1 * *` | `muMonthly` | 5min | `engine.GenerateMonthlyReport` | `bot.SendDigest` |
| 8 | `0 3 * * 1` | `muSubs` | 2min | `engine.DetectSubscriptions` | none |
| 9 | `0 4 * * *` | `muLookups` | 2min | `engine.DetectFrequentLookups`, `engine.CreateQuickReference`, `engine.PurgeOldSearchLogs` | `bot.SendDigest` (per quick ref) |
| 10 | `*/15 * * * *` | `muAlerts` | 1min | `engine.GetPendingAlerts`, `engine.MarkAlertDelivered` | `bot.SendAlertMessage` via `deliverPendingAlerts` |
| 11 | `0 6 * * *` | `muAlertProd` | 5min | `engine.ProduceBillAlerts`, `engine.ProduceTripPrepAlerts`, `engine.ProduceReturnWindowAlerts` | none |
| 12 | `0 7 * * 1` | `muRelCool` | 2min | `engine.ProduceRelationshipCoolingAlerts` | none |

### Impact Analysis
- Affected components: `internal/scheduler/scheduler.go`, `cmd/core/main.go` (constructor call)
- Affected data: none (refactoring only)
- Affected users: none (internal structure)
- Blast radius: medium — scheduler constructor signature changes in `cmd/core/main.go`

## Fix Design

### Solution Approach
Extract a `Job` interface and move each cron callback to its own file in `internal/scheduler/` (same `package scheduler` — NOT a sub-package). The scheduler becomes a thin loop that registers jobs from a slice passed to its constructor. Each job struct holds its own dependencies via constructor injection. The scheduler owns per-job mutexes and wraps each job's `Run()` with the shared TryLock/timeout/error boilerplate.

### Critical Design Decision: No Sub-Package

The original design proposed `internal/scheduler/jobs/` as a sub-directory. This is **incorrect for Go** because:
1. Every Go directory is a separate package — `jobs/` would be `package jobs`
2. `package jobs` cannot import `package scheduler` (circular: scheduler imports jobs to register them)
3. Jobs would need a separate interface package, adding unnecessary indirection

**Correct approach:** All job files live in `internal/scheduler/` as `package scheduler`. This is the same pattern used by `internal/intelligence/` (engine.go, expertise.go, subscriptions.go, etc.).

### Job Interface (`internal/scheduler/job.go`)

```go
package scheduler

import (
    "context"
    "time"
)

// Job defines a schedulable unit of work.
type Job interface {
    Name() string
    Schedule() string
    Timeout() time.Duration
    Run(ctx context.Context) error
}
```

### Scheduler Registration Loop (new `Start` in `scheduler.go`)

```go
func (s *Scheduler) Start(_ context.Context, jobs []Job) error {
    for _, j := range jobs {
        j := j // capture loop var
        mu := &sync.Mutex{}
        _, err := s.cron.AddFunc(j.Schedule(), func() {
            if !mu.TryLock() {
                slog.Warn("skipping overlapping job", "job", j.Name())
                return
            }
            defer mu.Unlock()
            ctx, cancel := context.WithTimeout(s.baseCtx, j.Timeout())
            defer cancel()
            if err := j.Run(ctx); err != nil {
                slog.Error("job failed", "job", j.Name(), "error", err)
            }
        })
        if err != nil {
            return fmt.Errorf("register job %q: %w", j.Name(), err)
        }
    }
    s.cron.Start()
    slog.Info("scheduler started", "jobs", len(jobs))
    return nil
}
```

### New Constructor

```go
func New() *Scheduler {
    ctx, cancel := context.WithCancel(context.Background())
    return &Scheduler{
        cron:       cron.New(),
        baseCtx:    ctx,
        baseCancel: cancel,
        done:       make(chan struct{}),
    }
}
```

Domain dependencies are removed from the constructor — they move to job constructors.

### Target File Layout

| File | Content | Imports (domain) | Est. LOC |
|------|---------|-----------------|----------|
| `job.go` | `Job` interface, `FormatAlertMessage`, `AlertTypeIcons` | none | ~40 |
| `scheduler.go` | `Scheduler` struct (slim), `New`, `Start`, `Stop`, `CronEntryCount`, `DigestPendingRetry`/`Date`/`Set` accessors | `cron/v3` only | ~120 |
| `digest_job.go` | `DigestJob` struct + `NewDigestJob` constructor | `digest`, `telegram` | ~100 |
| `synthesis_job.go` | `SynthesisJob` (synthesis + overdue) | `intelligence` | ~30 |
| `weekly_job.go` | `WeeklySynthesisJob` | `intelligence`, `telegram` | ~25 |
| `monthly_job.go` | `MonthlyReportJob` | `intelligence`, `telegram` | ~25 |
| `briefs_job.go` | `PreMeetingBriefsJob` | `intelligence`, `telegram` | ~25 |
| `alerts_job.go` | `AlertDeliveryJob` + `AlertProductionJob` | `intelligence`, `telegram` | ~70 |
| `lookups_job.go` | `FrequentLookupsJob` | `intelligence`, `telegram` | ~55 |
| `subscriptions_job.go` | `SubscriptionDetectionJob` | `intelligence` | ~20 |
| `resurfacing_job.go` | `ResurfacingJob` | `intelligence`, `telegram` | ~40 |
| `topics_job.go` | `TopicMomentumJob` | `topics` | ~20 |
| `relationship_job.go` | `RelationshipCoolingJob` | `intelligence` | ~20 |

**Total: ~590 LOC across 14 files** (same total, distributed).

### Per-Job Constructor Signatures

```go
// digest_job.go
func NewDigestJob(cronExpr string, gen *digest.Generator, bot *telegram.Bot,
    done <-chan struct{}, wg *sync.WaitGroup) *DigestJob

// synthesis_job.go
func NewSynthesisJob(engine *intelligence.Engine) *SynthesisJob

// weekly_job.go
func NewWeeklySynthesisJob(engine *intelligence.Engine, bot *telegram.Bot) *WeeklySynthesisJob

// monthly_job.go
func NewMonthlyReportJob(engine *intelligence.Engine, bot *telegram.Bot) *MonthlyReportJob

// briefs_job.go
func NewPreMeetingBriefsJob(engine *intelligence.Engine, bot *telegram.Bot) *PreMeetingBriefsJob

// alerts_job.go
func NewAlertDeliveryJob(engine *intelligence.Engine, bot *telegram.Bot) *AlertDeliveryJob
func NewAlertProductionJob(engine *intelligence.Engine) *AlertProductionJob

// lookups_job.go
func NewFrequentLookupsJob(engine *intelligence.Engine, bot *telegram.Bot) *FrequentLookupsJob

// subscriptions_job.go
func NewSubscriptionDetectionJob(engine *intelligence.Engine) *SubscriptionDetectionJob

// resurfacing_job.go
func NewResurfacingJob(engine *intelligence.Engine, bot *telegram.Bot) *ResurfacingJob

// topics_job.go
func NewTopicMomentumJob(lifecycle *topics.Lifecycle) *TopicMomentumJob

// relationship_job.go
func NewRelationshipCoolingJob(engine *intelligence.Engine) *RelationshipCoolingJob
```

### Digest Job Special Handling

The digest job is the most complex — it has:
1. A background goroutine that polls for ML-processed digest results (60s timeout)
2. Pending retry state (`digestPendingRetry`, `digestPendingDate`) with thread-safe accessors
3. Shutdown coordination via `done <-chan struct{}` and `wg *sync.WaitGroup`

The `DigestJob` struct holds these:
```go
type DigestJob struct {
    cronExpr string
    gen      *digest.Generator
    bot      *telegram.Bot
    done     <-chan struct{}
    wg       *sync.WaitGroup
    mu       sync.Mutex // guards pending retry state
    pendingRetry bool
    pendingDate  string
}
```

The scheduler passes its `done` channel and `wg` to `NewDigestJob()`. The test accessors (`DigestPendingRetry()`, `DigestPendingDate()`, `SetDigestPending()`) remain on Scheduler but delegate to the DigestJob if present — or they move to DigestJob with Scheduler providing a `DigestJob()` accessor. Simplest: keep the accessors on Scheduler and have Scheduler hold a `*DigestJob` reference for backward compatibility with existing tests.

### `deliverPendingAlerts` Migration

The current `deliverPendingAlerts` method on Scheduler (~45 LOC) moves entirely into `AlertDeliveryJob.Run()`. It uses `FormatAlertMessage` (stays in `job.go`), `engine.GetPendingAlerts`, `bot.SendAlertMessage`, and `engine.MarkAlertDelivered`.

### main.go Constructor Change

**Before:**
```go
sched := scheduler.New(digestGen, tgBot, intEngine, topicLifecycle)
if err := sched.Start(ctx, cfg.DigestCron); err != nil { ... }
```

**After:**
```go
sched := scheduler.New()
jobs := []scheduler.Job{
    scheduler.NewDigestJob(cfg.DigestCron, digestGen, tgBot, sched.Done(), sched.WaitGroup()),
    scheduler.NewTopicMomentumJob(topicLifecycle),
    scheduler.NewSynthesisJob(intEngine),
    scheduler.NewResurfacingJob(intEngine, tgBot),
    scheduler.NewPreMeetingBriefsJob(intEngine, tgBot),
    scheduler.NewWeeklySynthesisJob(intEngine, tgBot),
    scheduler.NewMonthlyReportJob(intEngine, tgBot),
    scheduler.NewSubscriptionDetectionJob(intEngine),
    scheduler.NewFrequentLookupsJob(intEngine, tgBot),
    scheduler.NewAlertDeliveryJob(intEngine, tgBot),
    scheduler.NewAlertProductionJob(intEngine),
    scheduler.NewRelationshipCoolingJob(intEngine),
}
if err := sched.Start(ctx, jobs); err != nil { ... }
```

Scheduler exposes `Done() <-chan struct{}` and `WaitGroup() *sync.WaitGroup` for the digest job's shutdown coordination.

### Test Impact Analysis

**`scheduler_test.go` — Tests requiring changes:**

| Test | Current Behavior | Required Change |
|------|-----------------|----------------|
| `TestNew` | Checks `New(nil, nil, nil, nil)` return | Change to `New()` — no args |
| `TestStart_InvalidCron` | Calls `Start(nil, "invalid-cron-expression")` | Remove — invalid cron is now a job concern. Or test with a job that has an invalid schedule. |
| `TestStart_ValidCron` | Calls `Start(nil, "0 7 * * *")` | Change to `Start(nil, []Job{...})` with a mock job |
| `TestSCN002060_CronEntries` | Checks `CronEntryCount()` | Pass mock jobs, verify count equals len(jobs) |
| `TestCronEntries_WithEngine` | Checks ≥11 entries with engine | Pass all jobs, verify count equals 12 |
| `TestSCN002061_NilDigestGenGuard` | Checks nil digestGen on struct | Move to digest_job_test.go |
| `TestSCN002062_ConcurrentRetryAccess` | Accesses `SetDigestPending`/`DigestPendingRetry` | Keep if accessors stay on Scheduler; else move to digest_job_test.go |
| `TestSCN002063_RetryFieldLifecycle` | Same | Same |
| `TestSCN002058_MutexProtectsRetryFields` | Same | Same |
| `TestSCN002059_RetryClearsOnSuccess` | Same | Same |

**Tests that compile unchanged:**
| Test | Reason |
|------|--------|
| `TestFormatAlertMessage_*` | `FormatAlertMessage` stays in `package scheduler` (moved to job.go) |
| `TestCronConcurrencyGuard_SameGroupSkipped` | Mutexes are now scheduler-internal, but test can use a slow mock job to verify overlap skipping |
| `TestCronConcurrencyGuard_AllGroupsIndependent` | Needs rewrite — named mutex fields (`muDaily`, `muHourly`) no longer exist. Replace with behavioral test: multiple concurrent jobs from different groups run simultaneously |

**New tests to add:**
- `digest_job_test.go`: DigestJob constructor, retry state, nil guard
- Each job file should have a `_test.go` verifying Name()/Schedule()/Timeout() return correct values and Run() with nil deps returns error (not panic)

### Alternative Approaches Considered
1. **Keep inline, just reduce duplication with helper functions** — Rejected: reduces LOC but doesn't fix coupling (scheduler.go still imports all domain packages)
2. **Event-driven scheduler via NATS** — Rejected: over-engineered for local cron; adds latency and failure modes for no user benefit
3. **Sub-package `internal/scheduler/jobs/`** — Rejected: creates separate `package jobs` requiring either circular imports or an interface-only package. Go convention is same-package multi-file for this pattern.
