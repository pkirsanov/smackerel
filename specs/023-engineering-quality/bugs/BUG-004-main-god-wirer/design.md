# Bug Fix Design: [BUG-004] main.go god-wirer

## Design Brief

- **Current State:** `cmd/core/main.go` is 724 LOC with 15 connector import aliases, inline connector config parsing + registration for each connector, service construction mixed with lifecycle code, and 6 helper functions. Every new connector adds ~30 LOC to this one file.
- **Target State:** `main.go` is ~150 LOC containing only `main()`, `run()` (slim), signal handling, HTTP server start/stop, and shutdown orchestration. Connector wiring and service construction are in dedicated same-package files.
- **Patterns to Follow:** Same-package multi-file split (like `internal/intelligence/` and the planned scheduler refactoring). All files are `package main`.
- **Patterns to Avoid:** Do NOT use `init()` for connector registration — it hides wiring and makes dependency order unclear.
- **Resolved Decisions:** Three files: `main.go`, `connectors.go`, `services.go`. Helper functions go with their consumers. `shutdownAll`/`runWithTimeout` stay in `main.go`.
- **Open Questions:** None.

## Root Cause Analysis

### Investigation Summary
`main.go` at 724 LOC imports 15 connector packages and mixes 3 distinct responsibilities: connector wiring with config parsing, service construction, and server lifecycle. Every new connector adds ~30 LOC to the same file.

### Root Cause
No extraction was performed because each individual connector addition was small (~30 LOC). The cumulative effect is a 724-LOC file that changes with every connector, service, or config addition.

### Verified Source Analysis (cmd/core/main.go)

**Import block (lines 1-44):**
- 15 connector aliases: `alertsConnector`, `bookmarksConnector`, `browserConnector`, `caldavConnector`, `discordConnector`, `guesthostConnector`, `hospitableConnector`, `imapConnector`, `keepConnector`, `mapsConnector`, `marketsConnector`, `rssConnector`, `twitterConnector`, `weatherConnector`, `youtubeConnector`
- 14 internal packages: `api`, `auth`, `config`, `connector`, `db`, `digest`, `graph`, `intelligence`, `nats`, `pipeline`, `scheduler`, `telegram`, `topics`, `web`
- 9 stdlib packages: `context`, `encoding/json`, `fmt`, `log/slog`, `math`, `net/http`, `os`, `os/signal`, `strconv`, `syscall`, `time`

**`run()` function (lines 62-540) — 3 interleaved responsibilities:**

1. **Config + lifecycle** (~60 LOC): config loading, logging setup, signal handling, HTTP server, shutdown
2. **Service construction** (~80 LOC): DB + NATS connections, graph repos, registry, pipeline, search engine, digest generator, intelligence engine, topic lifecycle, supervisor, web handler, context handler, OAuth handler, Telegram bot, scheduler, API deps + router
3. **Connector wiring** (~330 LOC): 15 connector instantiations, per-connector config parsing from env vars, per-connector registration + auto-start blocks

**Helper functions (lines 540-724):**
- `shutdownAll` (~60 LOC): sequential shutdown in reverse-dependency order
- `runWithTimeout` (~25 LOC): timeout wrapper for shutdown steps
- `parseJSONArray`, `parseJSONArrayEnv`, `parseJSONArrayVal` (~30 LOC): JSON array parsing
- `parseJSONObject`, `parseJSONObjectEnv`, `parseJSONObjectVal` (~30 LOC): JSON object parsing
- `parseFloatEnv` (~15 LOC): float env var parsing

### Impact Analysis
- Affected components: `cmd/core/main.go` only (split into 3 files)
- Affected data: none
- Affected users: none
- Risk: low — pure file move within `cmd/core/` package `main`

## Fix Design

### Solution Approach
Extract connector wiring and service construction into separate files in the same `package main`. Functions stay in the same package, so no import changes for consumers (there are none — it's `package main`).

### Target File Layout

#### `main.go` (~150 LOC after split)

**Keeps:**
- `version`, `commitHash` vars
- `main()` function
- `run()` function — slim version that calls `buildServices()` and `registerConnectors()`:
  1. `config.Load()`
  2. Logging setup
  3. `svc, err := buildServices(ctx, cfg)` — single call
  4. `registerConnectors(ctx, cfg, svc)` — single call
  5. HTTP server construction + `ListenAndServe`
  6. Signal handling
  7. `shutdownAll(...)`
- `shutdownAll()` function
- `runWithTimeout()` function

**Imports (after split):**
```go
import (
    "context"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/smackerel/smackerel/internal/config"
    "github.com/smackerel/smackerel/internal/connector"
    "github.com/smackerel/smackerel/internal/db"
    smacknats "github.com/smackerel/smackerel/internal/nats"
    "github.com/smackerel/smackerel/internal/pipeline"
    "github.com/smackerel/smackerel/internal/scheduler"
    "github.com/smackerel/smackerel/internal/telegram"
)
```

Note: `main.go` still needs `connector`, `db`, `nats`, `pipeline`, `scheduler`, `telegram` for `shutdownAll()` parameter types. If `shutdownAll` takes a `Services` struct, these reduce to just `config`.

#### `services.go` (~200 LOC)

**Contains:**
- `Services` struct holding all constructed service instances
- `buildServices(ctx context.Context, cfg *config.Config) (*Services, error)` function

**`Services` struct:**
```go
type Services struct {
    DB              *db.Postgres
    NATS            *smacknats.Client
    Registry        *connector.Registry
    Supervisor      *connector.Supervisor
    ResultSub       *pipeline.ResultSubscriber
    Pipeline        *pipeline.Processor
    SearchEngine    *api.SearchEngine
    DigestGen       *digest.Generator
    IntEngine       *intelligence.Engine
    TopicLifecycle  *topics.Lifecycle
    WebHandler      *web.Handler
    OAuthHandler    *auth.OAuthHandler
    ContextHandler  *api.ContextHandler
    TokenStore      *auth.TokenStore
    TelegramBot     *telegram.Bot
    Scheduler       *scheduler.Scheduler
    Router          http.Handler
    ArtifactPub     *pipeline.RawArtifactPublisher
    HospitalityLink *graph.HospitalityLinker
}
```

**`buildServices` constructs (in order):**
1. PostgreSQL connection + migration
2. NATS connection + stream setup
3. Hospitality graph repos + linker + topic seeding
4. Connector registry
5. Result subscriber + start
6. Pipeline processor
7. Search engine
8. Digest generator
9. Intelligence engine
10. Topic lifecycle
11. Connector supervisor + artifact publisher wiring
12. OAuth token store + handler
13. Web handler
14. Context handler
15. Telegram bot (if configured)
16. Scheduler (with job construction — integrates with BUG-002 refactoring)
17. API deps + router

**Imports:**
```go
import (
    "context"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "time"

    "github.com/smackerel/smackerel/internal/api"
    "github.com/smackerel/smackerel/internal/auth"
    "github.com/smackerel/smackerel/internal/config"
    "github.com/smackerel/smackerel/internal/connector"
    "github.com/smackerel/smackerel/internal/db"
    "github.com/smackerel/smackerel/internal/digest"
    "github.com/smackerel/smackerel/internal/graph"
    "github.com/smackerel/smackerel/internal/intelligence"
    smacknats "github.com/smackerel/smackerel/internal/nats"
    "github.com/smackerel/smackerel/internal/pipeline"
    "github.com/smackerel/smackerel/internal/scheduler"
    "github.com/smackerel/smackerel/internal/telegram"
    "github.com/smackerel/smackerel/internal/topics"
    "github.com/smackerel/smackerel/internal/web"
)
```

#### `connectors.go` (~400 LOC)

**Contains:**
- `registerConnectors(ctx context.Context, cfg *config.Config, svc *Services) error` function
- All 15 connector instantiations + per-connector config blocks + registration + auto-start logic
- Helper functions: `parseJSONArray`, `parseJSONArrayEnv`, `parseJSONArrayVal`, `parseJSONObject`, `parseJSONObjectEnv`, `parseJSONObjectVal`, `parseFloatEnv`

**Connector wiring blocks moved here (in the order they appear in current main.go):**

| Connector | Constructor | Config Source | Auto-start Condition |
|-----------|------------|---------------|---------------------|
| `gmail` (IMAP) | `imapConnector.New("gmail")` | OAuth token store | `tokenStore.HasToken(ctx, "google")` |
| `google-calendar` (CalDAV) | `caldavConnector.New("google-calendar")` | OAuth token store | Same Google token |
| `youtube` | `youtubeConnector.New("youtube")` | OAuth token store | Same Google token |
| `rss` | `rssConnector.New("rss", nil)` | source_config | Always registered, not auto-started |
| `google-keep` | `keepConnector.New("google-keep")` | — | Always registered, not auto-started |
| `bookmarks` | `bookmarksConnector.NewConnectorWithPool("bookmarks", pg.Pool)` | `cfg.BookmarksEnabled`, `cfg.BookmarksImportDir` | File-based, started if enabled |
| `browser-history` | `browserConnector.New("browser-history")` | `cfg.BrowserHistoryPath` | File-based, started if path set |
| `google-maps-timeline` | `mapsConnector.New("google-maps-timeline")` | `cfg.MapsImportDir` + 12 env vars | File-based, started if dir set |
| `hospitable` | `hospitableConnector.New("hospitable")` | — | Always registered, not auto-started |
| `guesthost` | `guesthostConnector.New()` | — | Always registered, not auto-started |
| `discord` | `discordConnector.New("discord")` | `DISCORD_*` env vars | Token-based, started if enabled |
| `twitter` | `twitterConnector.New("twitter")` | `TWITTER_*` env vars | Token-based, started if enabled |
| `weather` | `weatherConnector.New("weather")` | `WEATHER_*` env vars | No auth, started if enabled |
| `gov-alerts` | `alertsConnector.New("gov-alerts")` | `GOV_ALERTS_*` env vars | API key, started if enabled |
| `financial-markets` | `marketsConnector.New("financial-markets")` | `FINANCIAL_MARKETS_*` env vars | API key, started if enabled |

**Imports:**
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "math"
    "os"
    "strconv"

    "github.com/smackerel/smackerel/internal/config"
    "github.com/smackerel/smackerel/internal/connector"
    alertsConnector "github.com/smackerel/smackerel/internal/connector/alerts"
    bookmarksConnector "github.com/smackerel/smackerel/internal/connector/bookmarks"
    browserConnector "github.com/smackerel/smackerel/internal/connector/browser"
    caldavConnector "github.com/smackerel/smackerel/internal/connector/caldav"
    discordConnector "github.com/smackerel/smackerel/internal/connector/discord"
    guesthostConnector "github.com/smackerel/smackerel/internal/connector/guesthost"
    hospitableConnector "github.com/smackerel/smackerel/internal/connector/hospitable"
    imapConnector "github.com/smackerel/smackerel/internal/connector/imap"
    keepConnector "github.com/smackerel/smackerel/internal/connector/keep"
    mapsConnector "github.com/smackerel/smackerel/internal/connector/maps"
    marketsConnector "github.com/smackerel/smackerel/internal/connector/markets"
    rssConnector "github.com/smackerel/smackerel/internal/connector/rss"
    twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
    weatherConnector "github.com/smackerel/smackerel/internal/connector/weather"
    youtubeConnector "github.com/smackerel/smackerel/internal/connector/youtube"
    smacknats "github.com/smackerel/smackerel/internal/nats"
)
```

### `shutdownAll` Adaptation

Current `shutdownAll` takes 8 individual parameters. After refactoring, simplify to take `*Services`:

**Before:**
```go
func shutdownAll(timeoutS int, sched *scheduler.Scheduler, srv *http.Server,
    tgBot *telegram.Bot, resultSub *pipeline.ResultSubscriber,
    supervisor *connector.Supervisor, nc *smacknats.Client, pg *db.Postgres)
```

**After:**
```go
func shutdownAll(timeoutS int, srv *http.Server, svc *Services)
```

Shutdown sequence stays identical (scheduler → HTTP → Telegram → result subscribers → connectors → NATS → DB), just reads fields from `svc.*`.

### main.go `run()` After Refactoring

```go
func run() error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("configuration error: %w", err)
    }

    // Logging setup (~15 LOC, stays inline)
    ...

    svc, err := buildServices(ctx, cfg)
    if err != nil {
        return err
    }

    if err := registerConnectors(ctx, cfg, svc); err != nil {
        return err
    }

    // Start scheduler
    if err := svc.Scheduler.Start(ctx, cfg.DigestCron); err != nil {
        slog.Warn("digest scheduler failed to start", "error", err)
    }

    srv := &http.Server{
        Addr:              ":" + cfg.Port,
        Handler:           svc.Router,
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      30 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    // Signal handling + shutdown (~25 LOC, stays inline)
    ...

    shutdownAll(cfg.ShutdownTimeoutS, srv, svc)
    return nil
}
```

### Test Impact Analysis

**`main_test.go` — Zero compilation breakage.** All tests are in `package main` and all functions/types stay in the same package. Specific impact:

| Test | Current Location | Impact |
|------|-----------------|--------|
| `TestAllConnectorsRegistered` | Directly creates connectors | **None** — imports connector packages directly, doesn't use `registerConnectors()` |
| `TestDuplicateRegistrationRejected` | Tests registry behavior | **None** |
| `TestParseJSONArray_*` (7 tests) | Tests `parseJSONArray` | **None** — function moves to `connectors.go` but stays in `package main` |
| `TestParseJSONObject_*` (4 tests) | Tests `parseJSONObject` | **None** |

**New tests to consider (optional):**
- `TestBuildServices_ConfigError`: Verify `buildServices` returns error on bad DB URL
- `TestRegisterConnectors_AllRegistered`: Verify `registerConnectors` registers all 15 connectors
- These are integration-level tests and may be better validated via `./smackerel.sh test e2e`

### Verification Checklist (for implementer)

1. After splitting, run `wc -l cmd/core/main.go` — must be ≤200 LOC
2. Run `wc -l cmd/core/connectors.go` — should be ~400 LOC
3. Run `wc -l cmd/core/services.go` — should be ~200 LOC
4. Run `./smackerel.sh test unit` — all tests must pass unchanged
5. Verify no connector imports in `main.go`: `grep "Connector" cmd/core/main.go` should return nothing
6. Verify total LOC is unchanged: `wc -l cmd/core/*.go` ≈ 724 + test LOC

### Interaction with BUG-002 (Scheduler Refactoring)

If BUG-002 is implemented first, the scheduler construction in `services.go` would use the new `scheduler.New()` + job slice pattern instead of the current `scheduler.New(digestGen, tgBot, intEngine, topicLifecycle)`. If BUG-004 is implemented first, use the current constructor — BUG-002 will update it later.

Either order works. No circular dependency between the two refactorings.

### Alternative Approaches Considered
1. **Connector registration via init()** — Rejected: hides wiring, makes dependency order unclear
2. **Plugin-style connector loading** — Rejected: over-engineered for a single-binary deployment
3. **Separate package for services** — Rejected: `buildServices` needs to return concrete types from many packages; a separate package would need to re-export all of them or use interfaces, adding unnecessary indirection
