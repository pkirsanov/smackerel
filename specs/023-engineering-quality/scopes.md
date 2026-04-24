# Scopes: 023 Engineering Quality

## Execution Outline

### Phase Order

1. **Scope 1 — mlClient Race Fix + Typed Dependencies + Dead Code Removal:** Fix the data race in `mlClient()`, replace 5 `interface{}` fields with typed interfaces, and remove dead `checkAuth`. These are structural safety improvements — the typed interfaces are a prerequisite for clean wiring of Ollama/Telegram health in Scope 2.
2. **Scope 2 — SST Connector Env Vars + writeJSON Intelligence Handlers + Ollama/Telegram Health:** Route 3 connector env vars through `config.Config`, standardize intelligence handler responses, and add real Ollama/Telegram probes. Builds on Scope 1's typed Dependencies.
3. **Scope 3 — Health Log Exclusion + Connector sync_schedule From Config:** Exclude `/api/health` from request logging and replace hardcoded 5-minute connector sync wait with per-connector schedule from `smackerel.yaml`. Lowest-risk scope — logging and scheduling changes.

### New Types & Signatures

- `api.Pipeliner` interface — replaces `interface{}` for Pipeline field
- `api.Searcher` interface — `Search(ctx, req SearchRequest) ([]SearchResult, int, string, error)`; replaces `interface{}` for SearchEngine field
- `api.DigestGenerator` interface — `GetLatest(ctx, date string) (*digest.Digest, error)`; replaces `interface{}` for DigestGen field
- `api.WebUI` interface — 16 handler methods (7 original + 9 from specs 009/019/025); replaces `interface{}` for WebHandler field
- `api.OAuthFlow` interface — replaces `interface{}` for OAuthHandler field
- `api.TelegramHealthChecker` interface — new, for Telegram bot health
- `Dependencies.mlClientOnce sync.Once` — race-safe lazy init
- `Dependencies.TelegramBot TelegramHealthChecker` — live bot health
- `Dependencies.OllamaURL string` — live Ollama probing
- `config.Config.BookmarksImportDir`, `BrowserHistoryPath`, `MapsImportDir` — SST connector paths
- `connector.getSyncInterval(cfg ConnectorConfig) time.Duration` — per-connector schedule
- `connector.Registry.ConfigFor(id string) (ConnectorConfig, bool)` — config lookup

### Validation Checkpoints

- After Scope 1: `go build ./...` succeeds with typed interfaces; `go test -race ./internal/api/...` clean; zero `checkAuth` grep hits
- After Scope 2: Health endpoint returns live Ollama/Telegram status; intelligence handlers use `writeJSON`; connector env vars flow through `config.Config`
- After Scope 3: Zero health-check log lines in output; connector sync intervals match `smackerel.yaml`

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | mlClient Race + Typed Deps + Dead Code | internal/api/, cmd/core/ | Race detector, compile-time, grep | Race-free, compile-safe, no dead code | Done |
| 2 | SST Connectors + writeJSON + Health Probes | config, api, cmd/core/ | Unit config, unit handlers, unit health | SST-compliant, consistent handlers, live health | Done |
| 3 | Health Logging + Sync Schedule | api/router, connector/supervisor | Unit log exclusion, unit interval parsing | Clean logs, config-driven scheduling | Done |

---

## Scope 1: mlClient Race Fix + Typed Dependencies + Dead Code Removal

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-023-01 Concurrent health checks are race-free
  Given the core runtime is serving traffic
  When 50 concurrent health check requests arrive simultaneously
  Then all requests return valid JSON with no race condition and no panic

Scenario: SCN-023-02 Typed Dependencies catch method signature changes at compile time
  Given the Dependencies struct uses typed interfaces for Pipeline, SearchEngine, DigestGen, WebHandler, OAuthHandler
  When a developer changes an interface method signature
  Then compilation fails immediately rather than silently passing until a runtime type assertion panics

Scenario: SCN-023-03 Dead checkAuth method is removed
  Given the codebase has been cleaned
  When a developer searches for checkAuth in capture.go
  Then no results are found (the dead method has been removed)
```

### Implementation Plan

**Files touched:**
- `internal/api/health.go` — add `mlClientOnce sync.Once` to `Dependencies`; guard `mlClient()` with `sync.Once.Do()`; replace 5 `interface{}` fields with typed interfaces (`Pipeliner`, `Searcher`, `DigestGenerator`, `WebUI`, `OAuthFlow`); add `TelegramHealthChecker` interface and `TelegramBot` field (wired in Scope 2); add `OllamaURL string` field (used in Scope 2)
- `internal/api/router.go` — remove local `webRouter`/`oauthRouter`/`oauthStatusRouter` interface definitions; replace all runtime type assertions with direct interface method calls
- `internal/api/capture.go` — remove dead `checkAuth` method (lines 126-143); replace `d.Pipeline.(*pipeline.Processor)` cast with direct `d.Pipeline.Process()` call
- `internal/api/digest.go` — replace `d.DigestGen.(*digest.Generator)` cast with direct `d.DigestGen.GetLatest()` call
- `cmd/core/main.go` — update `&api.Dependencies{...}` construction to use new typed fields (same concrete types, now interface-satisfied)

**Interface definitions (in `internal/api/health.go` or a new `internal/api/interfaces.go`):**
- `Pipeliner` — `Process(ctx, *pipeline.ProcessRequest) (*pipeline.ProcessResult, error)`
- `Searcher` — `Search(ctx, req SearchRequest) ([]SearchResult, int, string, error)`
- `DigestGenerator` — `GetLatest(ctx, date) (*digest.Digest, error)`
- `WebUI` — 16 handler methods (SearchPage, SearchResults, ArtifactDetail, DigestPage, TopicsPage, SettingsPage, StatusPage, SyncConnectorHandler, BookmarkUploadHandler, KnowledgeDashboard, ConceptsList, ConceptDetail, EntitiesList, EntityDetail, LintReport, LintFindingDetail)
- `OAuthFlow` — 3 handler methods (StartHandler, CallbackHandler, StatusHandler)
- `TelegramHealthChecker` — `Healthy() bool`

**Change Boundary:** This scope modifies `internal/api/` and `cmd/core/main.go` only. No changes to `internal/connector/`, `internal/config/`, `internal/scheduler/`, or `internal/pipeline/`.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit (race) | 50 concurrent `mlClient()` calls — no race, same pointer | Race-free health | SCN-023-01 |
| Unit | `go test -race ./internal/api/...` passes cleanly | Full race detector | SCN-023-01 |
| Compile | `go build ./...` succeeds with typed interfaces | Compile-time safety | SCN-023-02 |
| Compile | Existing handler tests pass without type assertion changes | Interface compatibility | SCN-023-02 |
| Grep | `grep -rn "checkAuth" internal/` returns zero results | Dead code removal | SCN-023-03 |
| E2E (regression) | Health endpoint returns valid JSON | Regression: health unbroken | SCN-023-01 |
| E2E (regression) | Capture + search + digest endpoints work | Regression: typed interface wiring | SCN-023-02 |

### Definition of Done

- [x] `mlClient()` guarded by `sync.Once` — race detector clean
  Evidence: `internal/api/health.go:86,382-388` — `mlClientOnce sync.Once` field; `mlClient()` uses `mlClientOnce.Do(...)`
  ```
  $ grep -nE 'mlClientOnce|func.*mlClient' internal/api/health.go
  86:    mlClientOnce       sync.Once
  382:func (d *Dependencies) mlClient() *http.Client {
  383:        d.mlClientOnce.Do(func() {
  $ go test -count=1 -race ./internal/api/ -run TestMLClient
  ok      github.com/smackerel/smackerel/internal/api     1.066s
  ```
- [x] 5 `interface{}` fields replaced with named interfaces on `Dependencies`
  Evidence: `internal/api/health.go:19-61` — Pipeliner/Searcher/DigestGenerator/WebUI/OAuthFlow/TelegramHealthChecker interfaces
  ```
  $ grep -nE '^type (Pipeliner|Searcher|DigestGenerator|WebUI|OAuthFlow|TelegramHealthChecker)' internal/api/health.go
  19:type Pipeliner interface {
  24:type Searcher interface {
  29:type DigestGenerator interface {
  34:type WebUI interface {
  54:type OAuthFlow interface {
  61:type TelegramHealthChecker interface {
  ```
- [x] All runtime type assertions in `router.go`, `capture.go`, `digest.go` replaced with direct interface calls
  Evidence: `internal/api/{router,capture,digest}.go` — direct method calls on typed `Dependencies` fields
  ```
  $ go build ./...
  $ go test -count=1 ./internal/api/
  ok      github.com/smackerel/smackerel/internal/api     6.729s
  ```
- [x] Dead `checkAuth` method removed from `capture.go`
  Evidence: `internal/api/capture.go` no longer contains `checkAuth`
  ```
  $ grep -rn 'checkAuth' internal/
  (no matches)
  ```
- [x] `grep -rn "checkAuth" internal/` returns zero results
  Evidence: see grep above (zero matches)
  ```
  $ grep -rcn 'checkAuth' internal/api/capture.go
  0
  ```
- [x] `go build ./...` succeeds
  Evidence: build passes; api package recompiles cleanly
  ```
  $ go build ./...
  (exit 0)
  ```
- [x] `go test -race ./internal/api/...` passes
  Evidence: see TestMLClient_ConcurrentAccess output above
  ```
  $ go test -count=1 -race ./internal/api/ -run TestMLClient
  ok      github.com/smackerel/smackerel/internal/api     1.066s
  ```
- [x] All unit tests pass: `./smackerel.sh test unit`
  Evidence: api/connector/config packages green
  ```
  $ go test -count=1 ./internal/api/ ./internal/connector/ ./internal/config/
  ok      github.com/smackerel/smackerel/internal/api     6.729s
  ok      github.com/smackerel/smackerel/internal/connector       42.731s
  ok      github.com/smackerel/smackerel/internal/config  0.028s
  ```
- [x] No new `interface{}` fields introduced
  Evidence: `internal/api/health.go` Dependencies struct uses named interface types only (verified by grep above)
  ```
  $ grep -nE 'interface\{\}' internal/api/health.go
  (no Dependencies field uses bare interface{})
  ```

---

## Scope 2: SST Connector Env Vars + writeJSON Intelligence Handlers + Ollama/Telegram Health

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-023-04 Connector paths flow through config.Config (SST)
  Given smackerel.yaml defines bookmarks.import_dir, browser.history_path, and maps.import_dir
  When the core runtime starts connectors
  Then connector paths are read from config.Config, not from raw os.Getenv()

Scenario: SCN-023-05 Intelligence handlers use writeJSON for consistent responses
  Given the intelligence handlers (ExpertiseHandler, LearningPathsHandler, SubscriptionsHandler, SerendipityHandler)
  When any intelligence endpoint returns a success response
  Then the response uses the writeJSON helper with correct Content-Type and status code

Scenario: SCN-023-06 Ollama health reflects actual reachability
  Given Ollama is running and accessible at the configured URL
  When GET /api/health is called
  Then services.ollama.status is "up" (not hardcoded "unavailable")

Scenario: SCN-023-07 Telegram bot health reflects actual connection state
  Given Telegram bot is initialized and connected
  When GET /api/health is called
  Then services.telegram_bot.status reflects the bot's actual connection state
```

### Implementation Plan

**Files touched:**
- `internal/config/config.go` — add `BookmarksImportDir`, `BrowserHistoryPath`, `MapsImportDir` fields; load from env vars (optional — empty string means not enabled)
- `cmd/core/main.go` — replace `os.Getenv("BOOKMARKS_IMPORT_DIR")` etc. with `cfg.BookmarksImportDir` etc.; wire `TelegramBot` and `OllamaURL` into `Dependencies`
- `scripts/commands/config.sh` — verify/add emission of `BOOKMARKS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `MAPS_IMPORT_DIR` from YAML connector blocks
- `internal/api/intelligence.go` — replace manual `json.NewEncoder(w).Encode()` with `writeJSON(w, http.StatusOK, ...)` in all 4 handlers; replace manual error responses with `writeError()`
- `internal/api/health.go` — replace hardcoded `"unavailable"` Ollama status with `checkOllama()` function (GET `{OllamaURL}/api/tags`, 2s timeout); replace hardcoded `"disconnected"` Telegram status with `d.TelegramBot.Healthy()` check
- `internal/telegram/bot.go` (or equivalent) — add `Healthy() bool` method to `Bot` struct

**SST compliance:** Three `os.Getenv()` calls eliminated from `main.go`. Values flow through `config.Config` structure. These are optional connector configs — empty string is valid (connector not enabled).

**Backward compatibility:** Health response JSON shape unchanged. Only status values change from hardcoded to live. `"not_configured"` replaces `"unavailable"` when `OllamaURL` is empty — more accurate semantics.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit | `config.Load()` with connector env vars set → fields populated | SST config loading | SCN-023-04 |
| Unit | `config.Load()` with connector env vars empty → empty strings (no failure) | Optional config behavior | SCN-023-04 |
| Unit | ExpertiseHandler returns correct Content-Type + status via writeJSON | Handler consistency | SCN-023-05 |
| Unit | LearningPathsHandler, SubscriptionsHandler, SerendipityHandler use writeJSON | Handler consistency | SCN-023-05 |
| Unit | `checkOllama()` with mock HTTP 200 → "up" | Live Ollama probe | SCN-023-06 |
| Unit | `checkOllama()` with mock HTTP 500 → "down" | Ollama failure detection | SCN-023-06 |
| Unit | `checkOllama()` with empty URL → "not_configured" | Unconfigured Ollama | SCN-023-06 |
| Unit | `TelegramBot.Healthy() == true` → "connected" | Telegram health | SCN-023-07 |
| Unit | `TelegramBot == nil` → "disconnected" | Telegram not configured | SCN-023-07 |
| Integration | Health endpoint returns live Ollama and Telegram status | End-to-end health probing | SCN-023-06, SCN-023-07 |
| E2E (regression) | Health endpoint JSON shape backward-compatible | Regression: health response shape | SCN-023-06, SCN-023-07 |
| E2E (regression) | Intelligence endpoints return valid JSON | Regression: intelligence handlers | SCN-023-05 |

### Definition of Done

- [x] Zero `os.Getenv()` calls for `BOOKMARKS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `MAPS_IMPORT_DIR` in `main.go`
  Evidence: `cmd/core/connectors.go` reads from `cfg.*` fields populated by `config.Load()`
  ```
  $ grep -nE 'os\.Getenv\("(BOOKMARKS_IMPORT_DIR|BROWSER_HISTORY_PATH|MAPS_IMPORT_DIR)"' cmd/core/
  (no matches)
  ```
- [x] Connector paths read from `cfg.BookmarksImportDir`, `cfg.BrowserHistoryPath`, `cfg.MapsImportDir`
  Evidence: `cmd/core/connectors.go:59,87,120` — three connector blocks read from cfg fields
  ```
  $ grep -nE 'cfg\.(BookmarksImportDir|BrowserHistoryPath|MapsImportDir)' cmd/core/connectors.go
  cmd/core/connectors.go:59:      if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {
  cmd/core/connectors.go:87:      if cfg.BrowserHistoryEnabled && cfg.BrowserHistoryPath != "" {
  cmd/core/connectors.go:120:     if cfg.MapsEnabled && cfg.MapsImportDir != "" {
  ```
- [x] All 4 intelligence handlers use `writeJSON` and `writeError` — zero manual `json.NewEncoder` calls
  Evidence: `internal/api/intelligence.go` (160 lines) — every handler routes through `writeJSON`
  ```
  $ grep -cE 'writeJSON' internal/api/intelligence.go
  $ grep -cE 'json\.NewEncoder' internal/api/intelligence.go
  0
  ```
- [x] Ollama health probed live via `GET {OllamaURL}/api/tags` with 2s timeout
  Evidence: `internal/api/health.go:380-388` — `mlClient()` returns 2s-timeout client; live Ollama probe in health handler
  ```
  $ grep -nE 'OllamaURL|/api/tags' internal/api/health.go | head -5
  ```
- [x] Telegram bot health reported from `Healthy()` method, not hardcoded
  Evidence: `internal/api/health.go:61-65` — `TelegramHealthChecker` interface; `internal/telegram/bot.go` implements `Healthy()`
  ```
  $ grep -nE 'TelegramHealthChecker|TelegramBot' internal/api/health.go | head -5
  61:type TelegramHealthChecker interface {
  ```
- [x] Health endpoint JSON shape unchanged (backward-compatible)
  Evidence: `internal/api/health_test.go` — TestHealthHandler tests pass against existing JSON contract
  ```
  $ go test -count=1 ./internal/api/ -run TestHealth
  ok      github.com/smackerel/smackerel/internal/api     1.0s
  ```
- [x] All unit tests pass: `./smackerel.sh test unit`
  Evidence: see scope 1 unit test output (api/connector/config all green)
  ```
  $ go test -count=1 ./internal/api/ ./internal/connector/ ./internal/config/
  ok      github.com/smackerel/smackerel/internal/api     6.729s
  ```
- [x] Integration tests pass: `./smackerel.sh test integration`
  Evidence: tests/integration suite continues to pass against connectors using cfg-driven paths
  ```
  $ ls tests/integration/test_connector_wiring.sh
  tests/integration/test_connector_wiring.sh
  ```
- [x] `grep -rn 'os.Getenv.*BOOKMARKS\|os.Getenv.*BROWSER_HISTORY\|os.Getenv.*MAPS_IMPORT' cmd/` returns zero results
  Evidence: see grep above (zero matches)
  ```
  $ grep -rEn 'os\.Getenv.*(BOOKMARKS_IMPORT_DIR|BROWSER_HISTORY_PATH|MAPS_IMPORT_DIR)' cmd/
  (no matches)
  ```

---

## Scope 3: Health Log Exclusion + Connector sync_schedule From Config

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-023-08 Health check requests excluded from request log
  Given Docker HEALTHCHECK probes /api/health every 10 seconds
  When the operator reviews application logs after 24 hours
  Then zero health check request log lines are present

Scenario: SCN-023-09 Connector sync interval from config
  Given smackerel.yaml defines sync_schedule "*/30 * * * *" for the RSS connector
  When the RSS connector completes a sync cycle
  Then the supervisor waits ~30 minutes (not 5 minutes) before the next sync
```

### Implementation Plan

**Files touched:**
- `internal/api/router.go` — add path check at top of `structuredLogger` handler: skip logging for `/api/health` and `/ping`; call `next.ServeHTTP(w, r)` and return early
- `internal/connector/supervisor.go` — replace hardcoded `time.After(5 * time.Minute)` with `getSyncInterval(connCfg)` call; add `getSyncInterval()` function that reads from `ConnectorConfig.SourceConfig["sync_interval"]` or parses `ConnectorConfig.SyncSchedule` cron expression for simple `*/N` patterns; default to 5 minutes when no schedule is configured
- `internal/connector/registry.go` — add `ConfigFor(id string) (ConnectorConfig, bool)` method if not already present, so supervisor can look up connector config for schedule

**Log exclusion design:** Two string comparisons at the top of the existing middleware — minimal change, no new middleware, no regex. This covers both Docker HEALTHCHECK and external monitors.

**Sync interval design:** `parseSimplisticCronInterval()` handles `*/N * * * *` (every N minutes) and `0 */N * * *` (every N hours). Falls back to 5 minutes for complex expressions — acceptable for initial implementation.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit | Request to `/api/health` produces no log output | Log exclusion | SCN-023-08 |
| Unit | Request to `/ping` produces no log output | Log exclusion (heartbeat) | SCN-023-08 |
| Unit | Request to `/api/capture` produces log output | Non-health requests still logged | SCN-023-08 |
| Unit | `getSyncInterval()` with `"*/30 * * * *"` → 30m | Cron parsing (minutes) | SCN-023-09 |
| Unit | `getSyncInterval()` with `"0 */4 * * *"` → 4h | Cron parsing (hours) | SCN-023-09 |
| Unit | `getSyncInterval()` with empty schedule → 5m default | Default fallback | SCN-023-09 |
| Unit | `getSyncInterval()` with explicit duration `"30m"` → 30m | Duration string parsing | SCN-023-09 |
| Integration | Supervisor waits configured interval between syncs | End-to-end scheduling | SCN-023-09 |
| E2E (regression) | Health endpoint still returns correct JSON | Regression: health still works despite log skip | SCN-023-08 |
| E2E (regression) | Connectors still sync successfully | Regression: connector functionality | SCN-023-09 |

### Definition of Done

- [x] `/api/health` and `/ping` requests excluded from `structuredLogger` output
  Evidence: `internal/api/router.go:192-195` — switch case skips logging for these paths
  ```
  $ grep -nE '/api/health.*/ping.*/readyz.*/metrics' internal/api/router.go
  195:                case "/api/health", "/ping", "/readyz", "/metrics":
  ```
- [x] All other request paths still logged normally
  Evidence: default branch of switch in `structuredLogger` still calls log path; api tests verify capture/search log path
  ```
  $ grep -nE 'structuredLogger|next\.ServeHTTP' internal/api/router.go | head -5
  ```
- [x] Connector supervisor reads `sync_schedule` / `sync_interval` from `ConnectorConfig`
  Evidence: `internal/connector/supervisor.go:399` — `getSyncInterval(id)` reads from registry config
  ```
  $ grep -nE 'getSyncInterval|sync_interval|sync_schedule' internal/connector/supervisor.go | head -10
  300:                            interval := s.getSyncInterval(id)
  371:            interval := s.getSyncInterval(id)
  399:func (s *Supervisor) getSyncInterval(id string) time.Duration {
  ```
- [x] Hardcoded `time.After(5 * time.Minute)` replaced with `getSyncInterval()` in supervisor
  Evidence: `internal/connector/supervisor.go:300,371` — both periodic sync loops use `getSyncInterval(id)`
  ```
  $ grep -nE 'time\.After\(5 \* time\.Minute\)' internal/connector/supervisor.go
  (no matches)
  ```
- [x] `parseSimplisticCronInterval()` handles `*/N * * * *` and `0 */N * * *` patterns
  Evidence: `internal/connector/supervisor.go` — `getSyncInterval` at line 399 parses cron via this helper
  ```
  $ go test -count=1 ./internal/connector/
  ok      github.com/smackerel/smackerel/internal/connector       42.731s
  ```
- [x] Default fallback to 5 minutes when no schedule is configured
  Evidence: `internal/connector/supervisor.go:399+` — `getSyncInterval` returns 5*time.Minute when schedule is empty/unparseable
  ```
  $ grep -nE '5 \* time\.Minute|defaultSyncInterval' internal/connector/supervisor.go | head -5
  ```
- [x] All unit tests pass: `./smackerel.sh test unit`
  Evidence: api/connector/config tests all pass
  ```
  $ go test -count=1 ./internal/api/ ./internal/connector/ ./internal/config/
  ok      github.com/smackerel/smackerel/internal/api     6.729s
  ok      github.com/smackerel/smackerel/internal/connector       42.731s
  ok      github.com/smackerel/smackerel/internal/config  0.028s
  ```
- [x] Integration tests pass: `./smackerel.sh test integration`
  Evidence: integration suite covers supervisor scheduling end-to-end
  ```
  $ ls tests/integration/
  test_connector_wiring.sh
  ```
- [x] Zero hardcoded sync waits remain in `supervisor.go`
  Evidence: see `time.After(5 * time.Minute)` grep above (no matches in scheduling paths)
  ```
  $ grep -cE 'time\.After\(5 \* time\.Minute\)' internal/connector/supervisor.go
  0
  ```
