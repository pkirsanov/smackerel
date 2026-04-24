# Execution Reports

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Extract scheduler methods to ≤150 LOC — Done

### Summary
Pragmatic file-level extraction of `internal/scheduler/scheduler.go`. The 213-LOC scheduler.go was reduced to 122 LOC by moving `Stop()` + `runGuarded()` to a new `lifecycle.go`, the digest retry-state accessors and `CronEntryCount()` to a new `state.go`, and the `MealPlanAutoCompleter` interface + `SetKnowledgeLinter`/`SetMealPlanAutoComplete` setters to a new `registration.go`. The previously inline 10-entry intelligence-engine cron registration block in `Start()` was factored into a tiny private helper `scheduleEngineJobs()` to keep `Start()` compact. All extracted methods retain their `*Scheduler` receivers; **no exported API signature changes**; no behavior change. Verified at HEAD on 2026-04-24.

### Pragmatic Plan Note
The original BUG-002 plan called for a full `Job` interface refactor that would change `scheduler.New(...)` to `scheduler.New()` and `Start(ctx, cronExpr)` to `Start(ctx, []Job)`. The parent agent's hard rules for this session forbid exported-API signature changes. This scope therefore delivers the ≤150 LOC target via file extraction while preserving every exported signature on `*Scheduler`. The Job-interface refactor remains available as a follow-up if the no-API-change rule is later relaxed.

### Completion Statement
All 6 DoD items in `scopes.md` are checked with inline `**Evidence:**` blocks captured this session from real terminal output. Scope 1 status promoted from `Not Started` to `Done`. State promoted from `in_progress` to `done`.

### Test Evidence

**Command:** `./smackerel.sh test unit` — full unit suite (Go + Python ML sidecar)

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/cmd/core 0.432s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        (cached)
ok      github.com/smackerel/smackerel/internal/agent   (cached)
ok      github.com/smackerel/smackerel/internal/annotation      (cached)
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/auth    (cached)
ok      github.com/smackerel/smackerel/internal/config  0.071s
ok      github.com/smackerel/smackerel/internal/connector       (cached)
... (all 41 Go packages ok)
ok      github.com/smackerel/smackerel/internal/scheduler       5.024s
330 passed, 2 warnings in 12.06s
Exit Code: 0
```

**Command:** `./smackerel.sh test unit` — scheduler package isolated (verbose run summary)

```
$ go test -count=1 -v ./internal/scheduler/... 2>&1 | grep -E '^(--- PASS|PASS|ok )' | tail -10
--- PASS: TestRelationshipCoolingUsesOwnMutex (0.00s)
--- PASS: TestDeliverPendingAlerts_NilBotShortCircuit (0.00s)
--- PASS: TestDeliverPendingAlerts_NilBotNilEngine (0.00s)
--- PASS: TestDeliverPendingAlerts_DetachedMarkContext (0.00s)
--- PASS: TestMeetingBriefDeliveredMarkMustBeDetached (0.00s)
--- PASS: TestStop_CronStopBounded (0.00s)
--- PASS: TestStop_WgWaitBounded (5.00s)
PASS
ok      github.com/smackerel/smackerel/internal/scheduler       5.026s
Exit Code: 0
```

### Validation Evidence

**Command:** `wc -l internal/scheduler/*.go` — confirms target scheduler.go ≤150 LOC met (122 LOC, 81% of cap)

```
$ wc -l internal/scheduler/scheduler.go internal/scheduler/lifecycle.go internal/scheduler/state.go internal/scheduler/registration.go internal/scheduler/jobs.go
  122 internal/scheduler/scheduler.go
   46 internal/scheduler/lifecycle.go
   28 internal/scheduler/state.go
   26 internal/scheduler/registration.go
  467 internal/scheduler/jobs.go
  689 total
Exit Code: 0
```

**Command:** `grep -E '^func ' internal/scheduler/{scheduler,lifecycle,state,registration}.go` — confirms function placement

```
$ grep -E '^func ' internal/scheduler/scheduler.go internal/scheduler/lifecycle.go internal/scheduler/state.go internal/scheduler/registration.go
internal/scheduler/scheduler.go:func New(digestGen *digest.Generator, bot *telegram.Bot, engine *intelligence.Engine, lifecycle *topics.Lifecycle) *Scheduler {
internal/scheduler/scheduler.go:func (s *Scheduler) Start(_ context.Context, cronExpr string) error {
internal/scheduler/scheduler.go:func (s *Scheduler) scheduleEngineJobs() {
internal/scheduler/lifecycle.go:func (s *Scheduler) Stop() {
internal/scheduler/lifecycle.go:func (s *Scheduler) runGuarded(mu *sync.Mutex, group, job string, fn func()) {
internal/scheduler/state.go:func (s *Scheduler) DigestPendingRetry() bool {
internal/scheduler/state.go:func (s *Scheduler) DigestPendingDate() string {
internal/scheduler/state.go:func (s *Scheduler) SetDigestPending(retry bool, date string) {
internal/scheduler/state.go:func (s *Scheduler) CronEntryCount() int {
internal/scheduler/registration.go:func (s *Scheduler) SetKnowledgeLinter(linter *knowledge.Linter, cronExpr string) {
internal/scheduler/registration.go:func (s *Scheduler) SetMealPlanAutoComplete(svc MealPlanAutoCompleter, cronExpr string) {
Exit Code: 0
```

**Command:** `grep -nE 'scheduler\.New|sched\.Start' cmd/core/*.go` — sole consumer's call sites are unchanged (proves no API signature change)

```
$ grep -nE 'scheduler\.New|sched\.Start' cmd/core/*.go
cmd/core/main.go:71:    sched := scheduler.New(svc.digestGen, tgBot, svc.intEngine, svc.topicLifecycle)
cmd/core/main.go:85:    if err := sched.Start(ctx, cfg.DigestCron); err != nil {
Exit Code: 0
```

**Command:** `./smackerel.sh format --check && ./smackerel.sh lint && go vet ./...`

```
$ ./smackerel.sh format --check
39 files left unchanged

$ ./smackerel.sh lint
All checks passed!
Web validation passed

$ go vet ./...
(no output)
Exit Code: 0
```

### Audit Evidence

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-002-scheduler-god-orchestrator`

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-002-scheduler-god-orchestrator
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ Detected state.json status: done
✅ Detected state.json workflowMode: bugfix-fastlane
✅ Required specialist phase 'implement' found in execution/certification phase records
✅ Required specialist phase 'test' found in execution/certification phase records
✅ Required specialist phase 'validate' found in execution/certification phase records
✅ Required specialist phase 'audit' found in execution/certification phase records
Artifact lint PASSED.
Exit Code: 0
```

## Re-Promotion Note (2026-04-24)

The earlier 2026-04-15 demotion to `in_progress` flagged that the prior `done` claim lacked evidence. This session captured real terminal output for every DoD item against the current HEAD. The original scopes.md called for a full `Job` interface refactor that would change exported `scheduler.New()` and `Start()` signatures; the parent session's hard rules forbid exported-API signature changes, so the scope was re-scoped to deliver the same ≤150 LOC target via file extraction (no Job interface, no constructor change, no behavior change). The 2026-04-24 promotion replaces the stub Pending content with command-backed evidence per the bugfix-fastlane workflow.
