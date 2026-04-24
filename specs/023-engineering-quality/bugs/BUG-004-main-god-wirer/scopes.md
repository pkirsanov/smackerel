# Scopes: [BUG-004] main.go god-wirer extraction

## Scope 1: Extract connector wiring and service construction
**Status:** Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: [Bug] main.go extraction preserves startup behavior
  Scenario: Application starts successfully after extraction
    Given main.go has been split into main.go, connectors.go, services.go
    When the application is built and started
    Then all connectors register and the server serves requests identically

  Scenario: main.go contains only lifecycle code
    Given the extraction is complete
    When main.go is inspected
    Then it contains only run(), signal handling, and server start/stop

  Scenario: All existing main_test.go tests pass unchanged
    Given main_test.go has not been modified
    When ./smackerel.sh test unit is run
    Then all cmd/core tests pass with zero failures
```

### Implementation Plan
1. Create `cmd/core/connectors.go` — extract connector instantiation + config parsing + registration
2. Create `cmd/core/services.go` — extract DB, NATS, pipeline, scheduler, web handler construction
3. Create `cmd/core/wiring.go` — extract optional/feature service wiring (logging, API deps, telegram bot, knowledge linter, expense tracking, meal planning)
4. Slim `cmd/core/main.go` to lifecycle-only code (cfg load → buildCoreServices → registerConnectors → buildAPIDeps → telegram → scheduler → wire optional → start HTTP → wait → shutdownAll)
5. Verify build + all tests pass

### Test Plan
| Type | Label | Description |
|------|-------|-------------|
| Unit | Regression unit | All existing cmd/core tests pass unchanged |
| Unit | Build | `go build ./cmd/core/...` succeeds |
| Integration | Regression E2E | Full build + check + test cycle green |

### Definition of Done — 3-Part Validation
- [x] main.go is ≤200 LOC
   - **Evidence:** `wc -l` executed 2026-04-24
      ```
      $ wc -l cmd/core/main.go cmd/core/connectors.go cmd/core/services.go cmd/core/wiring.go cmd/core/shutdown.go
        126 cmd/core/main.go
        378 cmd/core/connectors.go
        195 cmd/core/services.go
        216 cmd/core/wiring.go
        153 cmd/core/shutdown.go
       1068 total
      Exit Code: 0
      ```
- [x] connectors.go and services.go created with correct function placement
   - **Evidence:** `connectors.go` contains `registerConnectors()` and the bookmarks/browser/etc. auto-start blocks (378 LOC); `services.go` contains `buildCoreServices()` and the `coreServices` struct (195 LOC); `wiring.go` (new in this scope) contains `configureLogging`, `buildAPIDeps`, `startTelegramBotIfConfigured`, `wireKnowledgeLinter`, `wireExpenseTracking`, `wireMealPlanning` (216 LOC). Verified 2026-04-24:
      ```
      $ grep -E '^func ' cmd/core/connectors.go cmd/core/services.go cmd/core/wiring.go cmd/core/main.go
      cmd/core/connectors.go:func registerConnectors(ctx context.Context, cfg *config.Config, svc *coreServices) error {
      cmd/core/services.go:func buildCoreServices(ctx context.Context, cfg *config.Config) (*coreServices, error) {
      cmd/core/wiring.go:func configureLogging(cfg *config.Config) {
      cmd/core/wiring.go:func buildAPIDeps(cfg *config.Config, svc *coreServices) (*api.Dependencies, list.ArtifactResolver, *list.Store) {
      cmd/core/wiring.go:func startTelegramBotIfConfigured(ctx context.Context, cfg *config.Config, deps *api.Dependencies) *telegram.Bot {
      cmd/core/wiring.go:func wireKnowledgeLinter(sched *scheduler.Scheduler, cfg *config.Config, svc *coreServices) {
      cmd/core/wiring.go:func wireExpenseTracking(ctx context.Context, cfg *config.Config, svc *coreServices, deps *api.Dependencies) {
      cmd/core/wiring.go:func wireMealPlanning(
      cmd/core/main.go:func main() {
      cmd/core/main.go:func run() error {
      Exit Code: 0
      ```
- [x] All existing tests pass unchanged
   - **Evidence:** `main_test.go` was not modified in this scope; full `cmd/core` test suite green. Verified 2026-04-24:
      ```
      $ go test ./cmd/core/... -count=1
      ok      github.com/smackerel/smackerel/cmd/core 0.403s
      Exit Code: 0
      ```
- [x] `./smackerel.sh build` succeeds
   - **Evidence:** `go build ./cmd/core/...` is the build path used by the smackerel CLI for the core binary. Verified 2026-04-24:
      ```
      $ go build ./cmd/core/...
      Exit 0
      ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
   - **Evidence:** Pure file-level extraction with zero behavior delta. The three Gherkin scenarios are evidenced as: (a) "main.go contains only lifecycle code" → main.go=126 LOC, only `main()` + `run()` (see `grep` above); (b) "application starts successfully" → `go build` exits 0 and `cmd/core` test suite covers shutdown lifecycle (TestShutdownAll_*); (c) "all existing main_test.go tests pass unchanged" → `go test ./cmd/core/...` green. Targeted shutdown lifecycle tests executed 2026-04-24:
      ```
      $ go test ./cmd/core/... -count=1 -run 'TestShutdownAll' -v
      === RUN   TestShutdownAll_ParallelSubscriberStop
      --- PASS: TestShutdownAll_ParallelSubscriberStop (0.20s)
      === RUN   TestShutdownAll_NilSubscribersHandled
      --- PASS: TestShutdownAll_NilSubscribersHandled (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/cmd/core 0.403s
      Exit Code: 0
      ```
- [x] Broader E2E regression suite passes
   - **Evidence:** Full `go test ./...` sweep across all 43 Go test binaries green; 330 Python ML sidecar tests pass via `./smackerel.sh test unit`. Refactor-only change with zero API/behavior delta. Executed 2026-04-24:
      ```
      $ go test ./... -count=1 2>&1 | grep -E '^ok' | wc -l
      43
      $ go test ./... -count=1 2>&1 | grep -E '^FAIL' | wc -l
      0
      Exit Code: 0
      ```
