# Design: 023 Engineering Quality

## Design Brief

**Current State:** The Go core runtime is functional but has nine identified quality issues: a data race in `mlClient()` (health.go:136), three SST-violating `os.Getenv()` calls for connector paths (main.go:149,167,185), five `interface{}` fields on the Dependencies struct requiring runtime type assertions (health.go:23-27, router.go:56-95), a dead `checkAuth` method (capture.go:126-143), four intelligence handlers bypassing `writeJSON` (intelligence.go), two hardcoded health statuses for Ollama/Telegram (health.go:109,112), a request logger that logs every health probe (router.go:121-133), and a hardcoded 5-minute sync wait in the connector supervisor (supervisor.go last `time.After`).

**Target State:** All nine findings resolved with compile-time safety, SST compliance, race-free concurrency, live health probes, clean logs, and config-driven connector scheduling.

**Patterns to Follow:**
- `writeJSON`/`writeError` helper pattern in [capture.go](../../internal/api/capture.go) (line 152-157) for consistent JSON responses
- `DBHealthChecker`/`NATSHealthChecker` interface pattern in [health.go](../../internal/api/health.go) (lines 33-42) for typed dependency injection
- `checkMLSidecar()` probe pattern in [health.go](../../internal/api/health.go) (lines 142-165) for HTTP-based service health
- `config.Config` struct loading from env vars in [config.go](../../internal/config/config.go) for SST-compliant config
- `ConnectorConfig.SyncSchedule` field in [connector.go](../../internal/connector/connector.go) (line 82) for schedule configuration

**Patterns to Avoid:**
- `interface{}` fields on Dependencies (health.go:23-27) — forces runtime type assertions; use named interfaces
- Raw `os.Getenv()` in main.go for values that belong in `config.Config` — violates SST
- Lazy init without synchronization (health.go:136-139) — data race under concurrency
- Manual `json.NewEncoder(w).Encode()` in handlers (intelligence.go) — bypasses writeJSON's status/content-type

**Resolved Decisions:**
- Fix mlClient race via `sync.Once` on the Dependencies struct (not constructor init — preserves lazy allocation)
- Define five narrow interfaces (`Pipeliner`, `Searcher`, `DigestGenerator`, `WebUI`, `OAuthFlow`) to replace the five `interface{}` fields
- Ollama probe uses `GET {OLLAMA_URL}/api/tags` with 2s timeout (same pattern as checkMLSidecar)
- Telegram health via a `HealthChecker` interface on Dependencies, implemented by `telegram.Bot`
- Log exclusion via path check at top of `structuredLogger` (no new middleware)
- Connector sync interval parsed from `ConnectorConfig.SourceConfig["sync_interval"]` with fallback from `ConnectorConfig.SyncSchedule`

**Open Questions:** None — all design decisions resolved from spec requirements and existing codebase patterns.

---

## Architecture Overview

This is a surgical code quality feature — no new packages, no new services, no new dependencies. All changes are within the existing Go core runtime, touching:

- `internal/api/` — health.go, router.go, capture.go, intelligence.go
- `internal/config/` — config.go
- `internal/connector/` — supervisor.go
- `cmd/core/` — main.go

No data model changes. No API contract changes (health endpoint JSON shape preserved). No Docker/infra changes.

---

## Design: R-ENG-001 — Fix mlClient() Race Condition

### Problem

`Dependencies.mlClient()` at [health.go](../../internal/api/health.go#L136-L139) performs an unsynchronized nil check + assignment on `MLClient`:

```go
func (d *Dependencies) mlClient() *http.Client {
    if d.MLClient == nil {
        d.MLClient = &http.Client{Timeout: 2 * time.Second}
    }
    return d.MLClient
}
```

Under concurrent `GET /api/health` requests, multiple goroutines can read `nil`, each allocate a new client, and race on the pointer write.

### Solution

Add a `sync.Once` field to `Dependencies` and use it to guard initialization:

```go
type Dependencies struct {
    // ... existing fields ...
    mlClientOnce sync.Once
}

func (d *Dependencies) mlClient() *http.Client {
    d.mlClientOnce.Do(func() {
        if d.MLClient == nil {
            d.MLClient = &http.Client{Timeout: 2 * time.Second}
        }
    })
    return d.MLClient
}
```

**Why sync.Once over constructor init:** The current pattern intentionally lazily initializes the client. `sync.Once` preserves this behavior while making it safe. If `MLClient` is pre-set (e.g., in tests), the `Do` callback respects the existing value.

### Testing Strategy

- **Unit test:** Call `mlClient()` from 50 goroutines concurrently, verify no panic and same pointer returned.
- **Race detector:** `go test -race ./internal/api/...` must pass cleanly.

---

## Design: R-ENG-002 — Route Connector Env Vars Through Config

### Problem

Three connector paths in [main.go](../../cmd/core/main.go) use raw `os.Getenv()`:

| Line | Variable | Current Call |
|------|----------|-------------|
| ~149 | `BOOKMARKS_IMPORT_DIR` | `os.Getenv("BOOKMARKS_IMPORT_DIR")` |
| ~167 | `BROWSER_HISTORY_PATH` | `os.Getenv("BROWSER_HISTORY_PATH")` |
| ~185 | `MAPS_IMPORT_DIR` | `os.Getenv("MAPS_IMPORT_DIR")` |

These bypass `config.Config` and violate SST policy.

### Solution

#### Step 1: Add to smackerel.yaml

The values already exist in `smackerel.yaml` under each connector's config block:
- `connectors.bookmarks.import_dir` (line exists, currently `""`)
- `connectors.browser-history.chrome.history_path` (line exists, currently `""`)
- `connectors.google-maps-timeline.import_dir` (line exists, currently `""`)

These are already in the SST file. The problem is that `config.Config` doesn't read them, so `main.go` falls back to raw `os.Getenv()`.

#### Step 2: Add fields to config.Config

Add three optional connector path fields to the `Config` struct in [config.go](../../internal/config/config.go):

```go
type Config struct {
    // ... existing fields ...
    BookmarksImportDir string
    BrowserHistoryPath string
    MapsImportDir      string
}
```

In `Load()`, read from the generated env vars:

```go
cfg.BookmarksImportDir = os.Getenv("BOOKMARKS_IMPORT_DIR")
cfg.BrowserHistoryPath = os.Getenv("BROWSER_HISTORY_PATH")
cfg.MapsImportDir      = os.Getenv("MAPS_IMPORT_DIR")
```

These are NOT required vars — connectors are opt-in. Empty string means "connector not enabled for file-based import."

#### Step 3: Update config generation pipeline

Ensure `./smackerel.sh config generate` emits these three variables into `config/generated/dev.env` and `config/generated/test.env` from the YAML connector blocks.

#### Step 4: Update main.go

Replace:
```go
if importDir := os.Getenv("BOOKMARKS_IMPORT_DIR"); importDir != "" {
```
With:
```go
if cfg.BookmarksImportDir != "" {
```

Same pattern for `BrowserHistoryPath` and `MapsImportDir`.

### Testing Strategy

- **Unit test:** `config.Load()` test with env vars set, verify fields populated.
- **Negative test:** Verify empty env var → empty config field (not a startup failure).
- **Integration:** `./smackerel.sh config generate` produces the three vars in generated env files.

---

## Design: R-ENG-009 — Replace interface{} Dependencies With Typed Interfaces

### Problem

Five fields on `Dependencies` in [health.go](../../internal/api/health.go#L23-L27) are `interface{}`:

```go
Pipeline     interface{}
SearchEngine interface{}
DigestGen    interface{}
WebHandler   interface{}
OAuthHandler interface{}
```

This forces runtime type assertions in [router.go](../../internal/api/router.go#L56-L95) and handler code (e.g., [capture.go](../../internal/api/capture.go#L85), [digest.go](../../internal/api/digest.go#L12)):

```go
proc, ok := d.Pipeline.(*pipeline.Processor)
engine, ok := d.SearchEngine.(*SearchEngine)
gen, ok := d.DigestGen.(*digest.Generator)
wh := deps.WebHandler.(webRouter)
oh := deps.OAuthHandler.(oauthRouter)
```

### Solution: Define Five Narrow Interfaces

Define interfaces in `internal/api/` that capture only the methods each consumer actually calls:

```go
// Pipeliner processes capture requests through the ML pipeline.
type Pipeliner interface {
    Process(ctx context.Context, req *pipeline.ProcessRequest) (*pipeline.ProcessResult, error)
}

// Searcher handles semantic search operations.
type Searcher interface {
    Search(ctx context.Context, req SearchRequest) ([]SearchResult, int, string, error)
}

// DigestGenerator produces daily/weekly digests.
type DigestGenerator interface {
    GetLatest(ctx context.Context, date string) (*digest.Digest, error)
}

// WebUI serves the HTMX web interface routes.
// Note: 16 methods — original 7 plus 9 added by specs 009 (bookmarks), 019 (connector wiring), and 025 (knowledge layer).
type WebUI interface {
    SearchPage(w http.ResponseWriter, r *http.Request)
    SearchResults(w http.ResponseWriter, r *http.Request)
    ArtifactDetail(w http.ResponseWriter, r *http.Request)
    DigestPage(w http.ResponseWriter, r *http.Request)
    TopicsPage(w http.ResponseWriter, r *http.Request)
    SettingsPage(w http.ResponseWriter, r *http.Request)
    StatusPage(w http.ResponseWriter, r *http.Request)
    SyncConnectorHandler(w http.ResponseWriter, r *http.Request)
    BookmarkUploadHandler(w http.ResponseWriter, r *http.Request)
    KnowledgeDashboard(w http.ResponseWriter, r *http.Request)
    ConceptsList(w http.ResponseWriter, r *http.Request)
    ConceptDetail(w http.ResponseWriter, r *http.Request)
    EntitiesList(w http.ResponseWriter, r *http.Request)
    EntityDetail(w http.ResponseWriter, r *http.Request)
    LintReport(w http.ResponseWriter, r *http.Request)
    LintFindingDetail(w http.ResponseWriter, r *http.Request)
}

// OAuthFlow handles OAuth2 authorization flows and status.
type OAuthFlow interface {
    StartHandler(w http.ResponseWriter, r *http.Request)
    CallbackHandler(w http.ResponseWriter, r *http.Request)
    StatusHandler(w http.ResponseWriter, r *http.Request)
}
```

Update the `Dependencies` struct:

```go
type Dependencies struct {
    DB                 DBHealthChecker
    NATS               NATSHealthChecker
    IntelligenceEngine *intelligence.Engine
    StartTime          time.Time
    MLSidecarURL       string
    MLClient           *http.Client
    mlClientOnce       sync.Once
    Pipeline           Pipeliner
    SearchEngine       Searcher
    DigestGen          DigestGenerator
    WebHandler         WebUI
    OAuthHandler       OAuthFlow
    TelegramBot        TelegramHealthChecker  // R-S-002
    OllamaURL          string                 // R-S-001
    AuthToken          string
    Version            string
    CommitHash         string
}
```

### Impact on router.go

The `type oauthRouter interface` and `type webRouter interface` local definitions in [router.go](../../internal/api/router.go#L60-L90) become redundant. Replace all runtime type assertions with direct interface method calls:

```go
// Before:
if deps.OAuthHandler != nil {
    type oauthStatusRouter interface {
        StatusHandler(w http.ResponseWriter, r *http.Request)
    }
    oh := deps.OAuthHandler.(oauthStatusRouter)
    r.Get("/auth/status", oh.StatusHandler)
}

// After:
if deps.OAuthHandler != nil {
    r.Get("/auth/status", deps.OAuthHandler.StatusHandler)
}
```

### Impact on capture.go and digest.go

Replace runtime casts with direct calls:

```go
// Before (capture.go):
proc, ok := d.Pipeline.(*pipeline.Processor)
if !ok || proc == nil {

// After:
if d.Pipeline == nil {

// Before (digest.go):
gen, ok := d.DigestGen.(*digest.Generator)
if !ok || gen == nil {

// After:
if d.DigestGen == nil {
```

### Impact on main.go

The concrete types `*pipeline.Processor`, `*api.SearchEngine`, `*digest.Generator`, `*web.Handler`, and `*auth.OAuthHandler` must satisfy the new interfaces. This is verified at compile time via implicit Go interface satisfaction. No explicit assertions needed.

### Testing Strategy

- **Compile-time:** If a concrete type drops a method, `go build` fails — this is the primary safety gain.
- **Unit test:** Existing handler tests continue to work with mock implementations of the interfaces.
- **Race detector:** No new shared state.

---

## Design: R-ENG-010 — Remove Dead checkAuth Method

### Problem

`checkAuth` at [capture.go](../../internal/api/capture.go#L126-L143) is a 17-line method that duplicates `bearerAuthMiddleware` in [router.go](../../internal/api/router.go#L170) and is never called anywhere in the codebase.

### Solution

Delete lines 126-143 of [capture.go](../../internal/api/capture.go). No other changes needed — the method has zero callers.

### Testing Strategy

- **Compile:** `go build ./...` succeeds with the method removed.
- **Grep:** `grep -rn "checkAuth" internal/` returns zero results.

---

## Design: R-ENG-013 — Use writeJSON in Intelligence Handlers

### Problem

Four handlers in [intelligence.go](../../internal/api/intelligence.go) manually call `json.NewEncoder(w).Encode()` instead of using `writeJSON`:

```go
// ExpertiseHandler (line ~16-17):
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(expertiseMap)

// LearningPathsHandler (line ~27-28):
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(paths)

// SubscriptionsHandler (line ~38-39):
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(summary)

// SerendipityHandler (line ~49-50, ~54-55):
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(pick)
```

This skips `writeJSON`'s consistent status-code setting and error logging.

### Solution

Replace each manual encoding block with `writeJSON(w, http.StatusOK, ...)`:

```go
func ExpertiseHandler(engine *intelligence.Engine) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        expertiseMap, err := engine.GenerateExpertiseMap(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "expertise_error", "expertise map generation failed")
            return
        }
        writeJSON(w, http.StatusOK, expertiseMap)
    }
}
```

Same pattern for `LearningPathsHandler`, `SubscriptionsHandler`, and `SerendipityHandler`.

For the `SerendipityHandler` nil-pick case, replace the raw `w.Write([]byte(...))` with:

```go
writeJSON(w, http.StatusOK, map[string]string{
    "message": "No serendipity candidates available yet. Archive items need 6+ months of dormancy.",
})
```

### Testing Strategy

- **Unit test:** Call each handler, verify `Content-Type: application/json` header and valid JSON body with correct HTTP status.
- **Existing tests:** Any existing intelligence handler tests continue to pass.

---

## Design: R-S-001 — Real Ollama Health Probing

### Problem

Ollama status is hardcoded at [health.go](../../internal/api/health.go#L112):

```go
services["ollama"] = ServiceStatus{Status: "unavailable"}
```

### Solution

Add `OllamaURL` field to Dependencies. Probe `GET {OllamaURL}/api/tags` with a 2-second timeout, following the same pattern as `checkMLSidecar()`:

```go
func checkOllama(ctx context.Context, ollamaURL string, client *http.Client) ServiceStatus {
    if ollamaURL == "" {
        return ServiceStatus{Status: "not_configured"}
    }

    probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, ollamaURL+"/api/tags", nil)
    if err != nil {
        return ServiceStatus{Status: "down"}
    }

    resp, err := client.Do(req)
    if err != nil {
        return ServiceStatus{Status: "down"}
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusOK {
        return ServiceStatus{Status: "up"}
    }
    return ServiceStatus{Status: "down"}
}
```

In `HealthHandler`, replace the hardcoded line with:

```go
services["ollama"] = checkOllama(ctx, d.OllamaURL, d.mlClient())
```

The `OllamaURL` is set in `main.go` from `cfg.OllamaURL`, which already flows from `config.Config` (SST-compliant).

### Backward Compatibility

- Health response JSON shape unchanged — `services.ollama` already exists, only the `status` value changes from hardcoded `"unavailable"` to a live value.
- `"not_configured"` replaces `"unavailable"` when `OllamaURL` is empty — more accurate semantics.

### Testing Strategy

- **Unit test:** Mock HTTP server returning 200 from `/api/tags` → status "up". Server returning 500 → "down". No URL configured → "not_configured".
- **Integration:** Live stack with Ollama running → health reports "up".

---

## Design: R-S-002 — Wire Telegram Bot Health Into Dependencies

### Problem

Telegram bot status is hardcoded at [health.go](../../internal/api/health.go#L109):

```go
services["telegram_bot"] = ServiceStatus{Status: "disconnected"}
```

### Solution

Define a `TelegramHealthChecker` interface and add it to Dependencies:

```go
// TelegramHealthChecker checks Telegram bot connection health.
type TelegramHealthChecker interface {
    Healthy() bool
}
```

The `telegram.Bot` struct already holds a `*tgbotapi.BotAPI` field (`api`). Add a `Healthy()` method:

```go
func (b *Bot) Healthy() bool {
    return b != nil && b.api != nil
}
```

In `HealthHandler`, replace the hardcoded line:

```go
if d.TelegramBot != nil && d.TelegramBot.Healthy() {
    services["telegram_bot"] = ServiceStatus{Status: "connected"}
} else {
    services["telegram_bot"] = ServiceStatus{Status: "disconnected"}
}
```

In `main.go`, wire the bot into Dependencies:

```go
deps := &api.Dependencies{
    // ... existing fields ...
    TelegramBot: tgBot, // may be nil if not configured
    OllamaURL:   cfg.OllamaURL,
}
```

### Backward Compatibility

- `services.telegram_bot` field already exists in the response. Only the status value changes from always-"disconnected" to a live value. Same `ServiceStatus` shape.

### Testing Strategy

- **Unit test:** Dependencies with nil TelegramBot → "disconnected". Dependencies with mock Healthy()=true → "connected".

---

## Design: R-S-007 — Exclude /api/health From Request Logging

### Problem

`structuredLogger` middleware in [router.go](../../internal/api/router.go#L121-L133) logs every request including health probes. At 10-second intervals, this produces ~8,640 unnecessary log lines/day.

### Solution

Add a path check at the top of the middleware handler:

```go
func structuredLogger(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Skip logging for health check and heartbeat endpoints
        if r.URL.Path == "/api/health" || r.URL.Path == "/ping" {
            next.ServeHTTP(w, r)
            return
        }

        start := time.Now()
        reqID := middleware.GetReqID(r.Context())
        ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
        next.ServeHTTP(ww, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", ww.Status(),
            "duration_ms", time.Since(start).Milliseconds(),
            "request_id", reqID,
        )
    })
}
```

**Why path check over separate middleware:** The paths are fixed, well-known, and won't change. A simple early-return is the minimal change. No new middleware constructor, no regex, no config — just two string comparisons.

### Testing Strategy

- **Unit test:** Send request to `/api/health`, verify no log output. Send request to `/api/capture`, verify log output appears.
- **Integration:** Run health probes for 1 minute, verify zero health-check log lines.

---

## Design: R-S-014 — Per-Connector sync_schedule From Config

### Problem

The connector supervisor at [supervisor.go](../../internal/connector/supervisor.go) hardcodes a 5-minute wait at the end of each sync cycle:

```go
select {
case <-connCtx.Done():
    return
case <-time.After(5 * time.Minute):
}
```

Meanwhile, `smackerel.yaml` defines per-connector `sync_schedule` cron expressions (e.g., `"*/30 * * * *"` for bookmarks, `"0 */4 * * *"` for browser history).

### Solution

The `ConnectorConfig` struct already has a `SyncSchedule` field (cron expression). However, parsing full cron to derive "next run" delay requires a cron library. A simpler approach that fits the current architecture:

#### Approach: Parse sync_interval from SourceConfig

Add a `sync_interval` key to `SourceConfig` that the config generation pipeline converts from the cron expression to a duration string. The supervisor reads this:

```go
func getSyncInterval(cfg ConnectorConfig) time.Duration {
    // Check SourceConfig for explicit interval
    if interval, ok := cfg.SourceConfig["sync_interval"]; ok {
        if s, ok := interval.(string); ok {
            if d, err := time.ParseDuration(s); err == nil && d > 0 {
                return d
            }
        }
    }

    // Parse SyncSchedule cron expression for simple cases
    if cfg.SyncSchedule != "" {
        if d := parseSimplisticCronInterval(cfg.SyncSchedule); d > 0 {
            return d
        }
    }

    // Default: 5 minutes (matches current behavior as safe fallback)
    return 5 * time.Minute
}
```

For the initial implementation, `parseSimplisticCronInterval` handles the common `*/N * * * *` (every N minutes) and `0 */N * * *` (every N hours) patterns. This covers all currently-configured connectors without adding a cron dependency.

#### Supervisor Changes

The supervisor needs the connector's config to know its schedule. Pass it when starting:

```go
func (s *Supervisor) StartConnector(ctx context.Context, id string, cfg ConnectorConfig) {
```

Or: the supervisor asks the registry for the connector's config. Since connectors store their config on `Connect()`, add a method:

```go
// ConfigFor returns the ConnectorConfig used when the connector was connected.
func (r *Registry) ConfigFor(id string) (ConnectorConfig, bool)
```

In the sync loop, replace the hardcoded wait:

```go
// Wait for next cycle (connector-specific schedule)
interval := getSyncInterval(connCfg)
select {
case <-connCtx.Done():
    return
case <-time.After(interval):
}
```

#### SST Compliance

The interval is derived from `smackerel.yaml` → `ConnectorConfig.SyncSchedule` or `SourceConfig["sync_interval"]`. No hardcoded value for specific connectors. The 5-minute default is a code-level safe fallback for connectors that don't specify a schedule, which is acceptable since it matches the current behavior and is not a config value that should vary per environment.

### Testing Strategy

- **Unit test:** `getSyncInterval` with various inputs: valid duration string, `*/30 * * * *` → 30m, `0 */4 * * *` → 4h, empty → 5m default.
- **Integration:** Start RSS connector with `sync_schedule: "*/30 * * * *"`, verify supervisor waits ~30m between cycles (testable via log output timing).

---

## Security & Compliance

- **No new auth surfaces.** All changes are internal to existing authenticated endpoints or the unauthenticated health endpoint.
- **Race fix (R-ENG-001)** eliminates a potential undefined-behavior vector under concurrent access.
- **Dead code removal (R-ENG-010)** reduces attack surface marginally.
- **No new dependencies.** Only `sync` from stdlib is added.
- **SST compliance (R-ENG-002)** strengthens config governance by eliminating three raw `os.Getenv()` calls.

## Observability

- **Log reduction (R-S-007):** ~8,640 fewer log lines/day at 10s health probe interval.
- **Health accuracy (R-S-001, R-S-002):** Operators see real Ollama and Telegram status instead of hardcoded placeholders.
- **No new metrics/traces** — changes are within existing code paths.

## Testing Strategy Summary

| Requirement | Test Type | Key Assertion |
|-------------|-----------|---------------|
| R-ENG-001 mlClient race | Unit + race detector | 50 concurrent calls, no race, same pointer |
| R-ENG-002 SST connector vars | Unit | Config fields populated from env, main.go reads from config |
| R-ENG-009 Typed interfaces | Compile-time | `go build` catches missing methods; existing tests pass |
| R-ENG-010 Dead checkAuth | Compile + grep | Build succeeds, zero grep results for `checkAuth` |
| R-ENG-013 writeJSON intelligence | Unit | Correct Content-Type, status code, JSON body |
| R-S-001 Ollama probe | Unit (mock HTTP) | up/down/not_configured based on server response |
| R-S-002 Telegram health | Unit | connected/disconnected based on Healthy() |
| R-S-007 Log exclusion | Unit | /api/health produces no log; /api/capture does |
| R-S-014 Sync schedule | Unit | Interval parsing from cron/duration; default 5m |

All changed packages must pass `go test -race ./...`.

## Risks & Open Questions

No open questions remain. Key risks:

1. **Typed interfaces may require import cycle resolution.** `Pipeliner` references `pipeline.ProcessRequest`/`ProcessResult`. If this creates an import cycle between `api` and `pipeline`, extract request/response types into a shared `internal/types` package or define the interface in the `pipeline` package and import it into `api`. The current dependency direction (`api` → `pipeline`) already exists, so this is unlikely.

2. **Searcher interface shape.** The current `SearchEngine` in [search.go](../../internal/api/search.go) is a struct with a `Pool` field, and search handlers access `Pool` directly. The `Searcher` interface must abstract the search operation, which may require a small refactor of the search handler to call `d.SearchEngine.Search()` instead of `d.SearchEngine.(*SearchEngine).Pool.Query()`. This is a net improvement.

3. **Cron interval parsing.** The simplistic parser covers `*/N` and `0 */N` patterns. If a connector uses a complex cron expression (e.g., `0 7,19 * * *`), it falls back to 5 minutes. This is acceptable for the initial implementation; a cron library can be added later if needed.
