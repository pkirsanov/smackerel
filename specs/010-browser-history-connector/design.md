# Design: 010 — Browser History Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, Keep, bookmarks, maps). A utility package already exists at `internal/connector/browser/browser.go` with Chrome SQLite parsing (`ParseChromeHistory`), dwell-time tiering (`DwellTimeTier`), social media detection (`IsSocialMedia`), skip filtering (`ShouldSkip`), and artifact conversion (`ToRawArtifacts`) — but these are standalone functions not wired into the connector framework.

### Target State

Add a `Connector` interface wrapper in `internal/connector/browser/` that turns the existing utility functions into a fully operational connector. The connector copies Chrome's locked History SQLite file to a temp location, queries visits newer than the cursor, applies skip/social-media/dwell-time classification, aggregates social media at domain level, detects repeat visits, and publishes tiered `RawArtifact` objects to the existing NATS pipeline. No new NATS streams, no new database migrations — the connector uses the existing artifact pipeline and `sync_state` table.

### Patterns to Follow

- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): struct with `id` + `health`, `New()` constructor, `Connect()` reads `SourceConfig`, `Sync()` sets health to syncing, iterates sources, filters by cursor, returns `[]RawArtifact` + latest cursor
- **Keep connector pattern** ([internal/connector/keep/keep.go](../../internal/connector/keep/keep.go)): config struct parsed from `ConnectorConfig.SourceConfig`, mutex-guarded health, sync metadata for health reporting, conditional registration in main.go
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)`
- **Existing browser utilities** ([internal/connector/browser/browser.go](../../internal/connector/browser/browser.go)): `ParseChromeHistory`, `DwellTimeTier`, `IsSocialMedia`, `ShouldSkip`, `ToRawArtifacts`, `chromeTimeToGo`, `extractDomain`
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull`, `TierStandard`, `TierLight`, `TierMetadata`

### Patterns to Avoid

- **Inventing new NATS streams** — the browser history connector uses the existing `artifacts.process` stream only; no Keep-style custom request/response subjects are needed since all logic is local Go code
- **Direct database queries from the connector** — dwell-time and visit data come from the Chrome SQLite file, not from Smackerel's PostgreSQL; the connector only touches PostgreSQL via StateStore for cursor persistence
- **Modifying existing browser.go** — the new connector.go wraps the existing functions; changes to browser.go are limited to exporting a new `ParseChromeHistorySince` variant that accepts a cursor parameter and removes the LIMIT 1000 cap

### Resolved Decisions

- Connector ID: `"browser-history"`
- Wrapper pattern: new `connector.go` wraps existing `ParseChromeHistory`, `DwellTimeTier`, `IsSocialMedia`, `ShouldSkip`, `ToRawArtifacts`
- File access: copy-then-read (Chrome locks its SQLite DB while running)
- Cursor: Chrome `visit_time` integer (microseconds since 1601-01-01) stored as string in StateStore
- Social media aggregation: domain-level daily summaries unless dwell ≥ 5min
- Privacy tiers: ≥5min full, ≥2min standard, ≥30s light, <30s metadata-only
- Opt-in: disabled by default in config
- No new NATS streams, no DB migration — uses existing artifact pipeline
- Registration in main.go: conditional on config enabled flag, no OAuth needed
- CGo dependency: `database/sql` + `go-sqlite3` for reading Chrome's SQLite
- Content extraction: high-dwell URLs fetched via `internal/extract` package

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────┐
│  Go Core Runtime                                │
│                                                 │
│  ┌───────────────────────────────────────────┐  │
│  │ internal/connector/browser/               │  │
│  │                                           │  │
│  │  browser.go    (existing utilities)       │  │
│  │  connector.go  (NEW — Connector iface)    │  │
│  │                                           │  │
│  │  Connect() ──► validate config            │  │
│  │                verify History file exists  │  │
│  │                                           │  │
│  │  Sync() ─────► copy History to tmp        │  │
│  │                ParseChromeHistorySince()   │  │
│  │                ShouldSkip() filter         │  │
│  │                IsSocialMedia() classify    │  │
│  │                DwellTimeTier() assign      │  │
│  │                repeat visit escalation     │  │
│  │                social media aggregation    │  │
│  │                privacy gate               │  │
│  │                ToRawArtifacts()            │  │
│  │                clean up tmp file          │  │
│  │                                           │  │
│  │  Health() ──► file accessible + sync meta │  │
│  │  Close() ───► cleanup, disconnected       │  │
│  └──────────────────┬────────────────────────┘  │
│                     │                           │
│          ┌──────────▼──────────┐                │
│          │  NATS JetStream     │                │
│          │  artifacts.process  │                │
│          └──────────┬──────────┘                │
│                     │                           │
│          ┌──────────▼──────────┐                │
│          │  Existing Pipeline  │                │
│          │  processor → dedup  │                │
│          │  → graph/linker     │                │
│          │  → topics/lifecycle │                │
│          └──────────┬──────────┘                │
│                     │                           │
│          ┌──────────▼──────────┐                │
│          │  PostgreSQL + pgvec │                │
│          │  artifacts, topics  │                │
│          │  edges, sync_state  │                │
│          └─────────────────────┘                │
└─────────────────────────────────────────────────┘

     Chrome Browser
     ┌────────────┐
     │ ~/.config/  │
     │ google-     │  copy-then-read
     │ chrome/     │◄──────────────── Connector copies
     │ Default/    │                  to /tmp, reads
     │ History     │                  copy, deletes tmp
     └────────────┘
```

### Data Flow

1. Scheduled sync fires (cron: `0 */4 * * *` default)
2. `Sync()` copies Chrome History SQLite file to temp directory
3. `ParseChromeHistorySince(tmpPath, cursor)` queries visits with `visit_time > cursor`, no row limit
4. `ShouldSkip(url, customSkipDomains)` filters out non-content URLs (chrome://, localhost, etc.)
5. `IsSocialMedia(domain)` splits entries into social-media vs. content tracks
6. Social media entries: aggregated at domain level per day; individual artifacts only for dwell ≥ 5min
7. Content entries: `DwellTimeTier(dwellTime)` assigns processing tier
8. Repeat visit detection: URLs seen ≥ 3 times in 7-day window get tier escalated by one level
9. Privacy gate: `metadata`-tier entries contribute to domain aggregates only — no individual artifact
10. `ToRawArtifacts()` converts qualifying entries to `[]RawArtifact`
11. Artifacts published to `artifacts.process` on NATS JetStream
12. Cursor advances to latest `visit_time` processed
13. Temp copy of History file deleted

---

## Component Design

### 1. `internal/connector/browser/connector.go` — Connector Interface (NEW)

Implements `connector.Connector`. Wraps existing utility functions from `browser.go`.

```go
package browser

import (
    "context"
    "fmt"
    "io"
    "log/slog"
    "os"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

// BrowserConfig holds parsed browser-history-specific configuration.
type BrowserConfig struct {
    HistoryPath             string
    AccessStrategy          string        // "copy" or "wal-read"
    InitialLookbackDays     int
    RepeatVisitWindow       time.Duration
    RepeatVisitThreshold    int
    ContentFetchTimeout     time.Duration
    ContentFetchConcurrency int
    ContentFetchDomainDelay time.Duration
    CustomSkipDomains       []string
    SocialMediaIndividualThreshold time.Duration
    // Dwell-time threshold overrides (zero means use defaults)
    DwellFullMin     time.Duration
    DwellStandardMin time.Duration
    DwellLightMin    time.Duration
}

// Connector implements the browser history connector.
type Connector struct {
    id     string
    health connector.HealthStatus
    mu     sync.RWMutex
    config BrowserConfig

    // Sync metadata for health reporting
    lastSyncTime       time.Time
    lastSyncCount      int
    lastSyncErrors     int
    lastSyncSkipped    int
    lastSyncByTier     map[string]int
    lastSyncFetchFails int
}

// New creates a new browser history connector.
func New(id string) *Connector {
    return &Connector{
        id:     id,
        health: connector.HealthDisconnected,
    }
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseBrowserConfig(config)
    if err != nil {
        return fmt.Errorf("parse browser config: %w", err)
    }

    // Validate History file exists and is readable
    if _, err := os.Stat(cfg.HistoryPath); os.IsNotExist(err) {
        c.mu.Lock()
        c.health = connector.HealthError
        c.mu.Unlock()
        return fmt.Errorf("chrome history file not found: %s", cfg.HistoryPath)
    }

    c.mu.Lock()
    c.config = cfg
    c.health = connector.HealthHealthy
    c.mu.Unlock()

    slog.Info("browser history connector connected",
        "history_path", cfg.HistoryPath,
        "access_strategy", cfg.AccessStrategy,
    )
    return nil
}
```

**`Sync(ctx, cursor)` method** (core logic):

```go
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()

    defer func() {
        c.mu.Lock()
        if c.lastSyncErrors > 0 {
            c.health = connector.HealthError
        } else {
            c.health = connector.HealthHealthy
        }
        c.mu.Unlock()
    }()

    // Step 1: Copy History file to temp location
    tmpPath, err := c.copyHistoryFile()
    if err != nil {
        // Retry once after 5 seconds (Chrome may be writing)
        time.Sleep(5 * time.Second)
        tmpPath, err = c.copyHistoryFile()
        if err != nil {
            return nil, cursor, fmt.Errorf("copy history file (after retry): %w", err)
        }
    }
    defer os.Remove(tmpPath)

    // Step 2: Parse entries since cursor
    var chromeTimeCursor int64
    if cursor != "" {
        chromeTimeCursor = parseCursorToChrome(cursor)
    } else {
        // Initial sync: lookback window
        lookback := time.Now().AddDate(0, 0, -c.config.InitialLookbackDays)
        chromeTimeCursor = goTimeToChrome(lookback)
    }

    entries, err := ParseChromeHistorySince(tmpPath, chromeTimeCursor)
    if err != nil {
        return nil, cursor, fmt.Errorf("parse chrome history: %w", err)
    }

    // Step 3–9: Filter, classify, aggregate, convert
    artifacts, newCursor, stats := c.processEntries(entries, chromeTimeCursor)

    // Record sync metadata
    c.mu.Lock()
    c.lastSyncTime = time.Now()
    c.lastSyncCount = len(artifacts)
    c.lastSyncSkipped = stats.skipped
    c.lastSyncByTier = stats.byTier
    c.lastSyncFetchFails = stats.fetchFails
    c.lastSyncErrors = stats.fetchFails
    c.mu.Unlock()

    return artifacts, newCursor, nil
}
```

**Key internal methods** (on the `Connector` struct):

- `copyHistoryFile() (string, error)` — copies `config.HistoryPath` to a temp file in `os.TempDir()`, returns temp file path. Uses `io.Copy` with a read-only source file handle. Returns descriptive error on failure (disk full, permission denied, file locked).

- `processEntries(entries []HistoryEntry, prevCursor int64) ([]connector.RawArtifact, string, syncStats)` — orchestrates the full filtering pipeline:
  1. Apply `ShouldSkip(url, config.CustomSkipDomains)` — count skipped by reason
  2. Split remaining into social-media (`IsSocialMedia(domain)`) vs. content tracks
  3. Assign `DwellTimeTier(dwellTime)` to content entries
  4. Run `detectRepeatVisits()` and escalate tiers where applicable
  5. Apply privacy gate: entries at `metadata` tier → domain aggregates only
  6. Aggregate social media entries per domain per day; except individual processing for dwell ≥ `SocialMediaIndividualThreshold`
  7. Call `ToRawArtifacts()` for qualifying content entries
  8. Build social-media aggregate artifacts
  9. Track the max `visit_time` as the new cursor
  10. Return combined artifacts, cursor string, and stats

- `detectRepeatVisits(entries []HistoryEntry) map[string]int` — counts URL occurrences within `config.RepeatVisitWindow`. Returns map of URL → visit count for URLs exceeding `config.RepeatVisitThreshold`.

- `escalateTier(currentTier string) string` — bumps tier one level: `metadata` → `light`, `light` → `standard`, `standard` → `full`, `full` → `full`.

- `buildSocialAggregate(domain string, entries []HistoryEntry, day time.Time) connector.RawArtifact` — creates a single `RawArtifact` with:
  - `SourceID`: `"browser-history"`
  - `SourceRef`: `"social-agg:{domain}:{day.Format("2006-01-02")}"`
  - `ContentType`: `"browsing/social-aggregate"`
  - `Title`: `"{count} visits to {domain} on {day}"`
  - `RawContent`: summary with total visits, total dwell, and top-dwell pages
  - `Metadata`: `{"domain", "total_visits", "total_dwell_seconds", "top_pages"}`
  - `CapturedAt`: end of the day

- `parseBrowserConfig(config ConnectorConfig) (BrowserConfig, error)` — extracts browser-history-specific fields from `ConnectorConfig.SourceConfig` with validation:
  - `history_path` is required and must be a non-empty string
  - `access_strategy` defaults to `"copy"` if not set
  - `initial_lookback_days` defaults to 30
  - All duration fields parsed from string (e.g., `"5m"`, `"7d"`)

**Health and Close:**

```go
func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *Connector) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.health = connector.HealthDisconnected
    slog.Info("browser history connector closed")
    return nil
}
```

### 2. Changes to `internal/connector/browser/browser.go` — New Cursor-Based Query

The existing `ParseChromeHistory` has a hardcoded `LIMIT 1000` and no cursor support. Add one new exported function:

```go
// ParseChromeHistorySince reads Chrome history entries with visit_time > cursor.
// Unlike ParseChromeHistory, this has no row limit and supports cursor-based
// incremental sync.
func ParseChromeHistorySince(dbPath string, chromeTimeCursor int64) ([]HistoryEntry, error) {
    db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
    if err != nil {
        return nil, fmt.Errorf("open Chrome history: %w", err)
    }
    defer db.Close()

    rows, err := db.Query(`
        SELECT u.url, u.title, v.visit_time, v.visit_duration
        FROM urls u
        JOIN visits v ON v.url = u.id
        WHERE v.visit_time > ?
        ORDER BY v.visit_time ASC
    `, chromeTimeCursor)
    if err != nil {
        return nil, fmt.Errorf("query history since cursor: %w", err)
    }
    defer rows.Close()

    var entries []HistoryEntry
    for rows.Next() {
        var e HistoryEntry
        var visitTime int64
        var duration int64
        if err := rows.Scan(&e.URL, &e.Title, &visitTime, &duration); err != nil {
            continue
        }
        e.VisitTime = chromeTimeToGo(visitTime)
        e.DwellTime = time.Duration(duration) * time.Microsecond
        e.Domain = extractDomain(e.URL)
        entries = append(entries, e)
    }

    return entries, nil
}
```

Also add two exported time conversion helpers for cursor management:

```go
// GoTimeToChrome converts a Go time.Time to Chrome's microseconds-since-1601 format.
func GoTimeToChrome(t time.Time) int64 {
    const chromeEpochDiff = 11644473600000000
    return t.UnixMicro() + chromeEpochDiff
}

// ChromeTimeToGo converts Chrome's microseconds-since-1601 to time.Time.
// Exports the existing chromeTimeToGo for use by the connector.
func ChromeTimeToGo(chromeTime int64) time.Time {
    return chromeTimeToGo(chromeTime)
}
```

### 3. Registration in `cmd/core/main.go`

The browser history connector is registered conditionally based on configuration. It does **not** require OAuth — the only credential is a local file path.

```go
import (
    browserConnector "github.com/smackerel/smackerel/internal/connector/browser"
)

// After existing connector registrations:
browserConn := browserConnector.New("browser-history")
registry.Register(browserConn)

// Browser history connector — no OAuth, opt-in via config
if cfg.Connectors.BrowserHistory.Enabled {
    browserCfg := connector.ConnectorConfig{
        AuthType:     "none",
        Enabled:      true,
        SourceConfig: map[string]interface{}{
            "history_path":             cfg.Connectors.BrowserHistory.Chrome.HistoryPath,
            "access_strategy":          cfg.Connectors.BrowserHistory.Chrome.AccessStrategy,
            "initial_lookback_days":    cfg.Connectors.BrowserHistory.Processing.InitialLookbackDays,
            "repeat_visit_window":      cfg.Connectors.BrowserHistory.Processing.RepeatVisitWindow,
            "repeat_visit_threshold":   cfg.Connectors.BrowserHistory.Processing.RepeatVisitThreshold,
            "content_fetch_timeout":    cfg.Connectors.BrowserHistory.Processing.ContentFetchTimeout,
            "content_fetch_concurrency": cfg.Connectors.BrowserHistory.Processing.ContentFetchConcurrency,
            "content_fetch_domain_delay": cfg.Connectors.BrowserHistory.Processing.ContentFetchDomainDelay,
            "custom_skip_domains":      cfg.Connectors.BrowserHistory.Skip.CustomDomains,
            "social_media_individual_threshold": cfg.Connectors.BrowserHistory.Skip.SocialMediaIndividualThreshold,
        },
        SyncSchedule: cfg.Connectors.BrowserHistory.SyncSchedule,
    }
    if err := browserConn.Connect(ctx, browserCfg); err != nil {
        slog.Warn("browser history connector failed to connect", "error", err)
    } else {
        supervisor.StartConnector(ctx, "browser-history")
        slog.Info("browser history connector started")
    }
}
```

---

## Configuration

Added to `config/smackerel.yaml` under `connectors:`:

```yaml
connectors:
  browser-history:
    enabled: false                    # Opt-in — disabled by default
    sync_schedule: "0 */4 * * *"     # Every 4 hours

    chrome:
      history_path: ""               # REQUIRED when enabled: path to Chrome History SQLite file
                                     # Linux:  ~/.config/google-chrome/Default/History
                                     # macOS:  ~/Library/Application Support/Google/Chrome/Default/History
      access_strategy: "copy"        # "copy" (safe default) or "wal-read" (fresher, advanced)

    processing:
      initial_lookback_days: 30
      dwell_time_thresholds:
        full_min: "5m"
        standard_min: "2m"
        light_min: "30s"
      repeat_visit_window: "7d"
      repeat_visit_threshold: 3
      content_fetch_timeout: "15s"
      content_fetch_concurrency: 5
      content_fetch_domain_delay: "1s"

    skip:
      custom_domains: []
      social_media_individual_threshold: "5m"

    privacy:
      store_full_urls_above_tier: "light"
      aggregate_only_below_tier: "metadata"
```

Config parsing in `internal/config/config.go` adds a `BrowserHistory` struct to the existing `Connectors` section. Validation:
- If `enabled: true`, `chrome.history_path` is required and non-empty
- `access_strategy` must be `"copy"` or `"wal-read"`
- Duration strings must parse via `time.ParseDuration` (with custom `"7d"` → `168h` expansion)

---

## Data Model

### No New Database Migration

The browser history connector uses the existing artifact and sync_state tables. No new tables or columns are required.

| Published Artifact Field | Value |
|---|---|
| `source_id` | `"browser-history"` |
| `source_ref` | URL for content entries; `"social-agg:{domain}:{date}"` for aggregates |
| `content_type` | `"url"` for content entries; `"browsing/social-aggregate"` for domain aggregates |
| `title` | Page title (from Chrome); or `"{count} visits to {domain}"` for aggregates |
| `raw_content` | URL for light/standard; extracted page text for full; summary for aggregates |
| `url` | Full URL (light tier and above); empty for metadata-tier and aggregates |
| `metadata.domain` | Extracted domain |
| `metadata.dwell_time` | Dwell duration in seconds |
| `metadata.tier` | Assigned processing tier (`full`, `standard`, `light`) |
| `metadata.repeat_visits` | Repeat visit count (if escalated) |
| `metadata.content_fetch_failed` | `true` if page content was not fetchable |
| `metadata.access_strategy` | `"copy"` or `"wal-read"` |
| `captured_at` | Visit timestamp from Chrome |

### Cursor Format

Cursor is stored as a string representation of the Chrome `visit_time` integer (microseconds since 1601-01-01) in the existing `sync_state` table via `StateStore.Save()`. Example: `"13350000000000000"`.

On initial sync (empty cursor), the connector computes a lookback cursor from `time.Now().AddDate(0, 0, -InitialLookbackDays)` converted to Chrome epoch.

### Dedup Strategy

- **Dedup key:** URL + visit date (date-granularity, not timestamp-granularity)
- Multiple visits to the same URL on the same day produce a single artifact with aggregated dwell time
- Cross-day visits to the same URL create separate artifacts linked via repeat visit detection
- Dedup checked against the existing pipeline `DedupChecker.Check(ctx, contentHash)` where `contentHash = sha256(url + visitDate)`

---

## Content Extraction for High-Dwell Pages

URLs assigned `full` or `standard` processing tiers have their page content fetched:

- Use `internal/extract` package for readability-based text extraction
- HTTP fetch with `config.ContentFetchTimeout` (default 15s)
- User-Agent: `"Smackerel/1.0 (personal knowledge agent)"`
- Rate limiting: max `config.ContentFetchConcurrency` (default 5) concurrent fetches, `config.ContentFetchDomainDelay` (default 1s) between requests to the same domain
- On failure (404, 500, timeout): create artifact with metadata only, set `metadata.content_fetch_failed = true`
- Content cached by URL hash within a single sync cycle to avoid re-fetching on same-day repeat visits

---

## Processing Tier Summary

| Dwell Time | Tier | URL Stored | Content Stored | Metadata Stored | Extraction |
|---|---|---|---|---|---|
| ≥ 5 min | `full` | Full URL | Extracted page text | Domain, dwell, visit time, title | Full extract + embed + graph link |
| ≥ 2 min | `standard` | Full URL | Extracted page text | Domain, dwell, visit time, title | Extract + embed |
| ≥ 30 sec | `light` | Full URL | Title only | Domain, dwell, visit time | Title embed only |
| < 30 sec | `metadata` | NOT stored individually | NOT stored | Domain-level aggregate only | None |

Repeat visit escalation (≥3 visits in 7 days) bumps tier by one level. Social media entries follow the same tiers but are aggregated at domain level unless individual dwell ≥ 5min.

---

## Social Media Domain Aggregation

Social media URLs (detected via `IsSocialMedia`) receive special handling:

1. All visits to a social media domain within a sync cycle are grouped by domain and date
2. A single `browsing/social-aggregate` artifact per domain per day is created containing:
   - Total visit count
   - Total dwell time
   - Top-dwell-time pages (up to 5) with their individual dwell times
3. **Exception:** Individual social media URLs with dwell ≥ `social_media_individual_threshold` (default 5min) are processed individually at `full` tier and excluded from the aggregate — sustained engagement on a single social page indicates intentional reading

---

## Repeat Visit Detection

Track URL visit frequency within `repeat_visit_window` (default 7 days):

1. During `processEntries`, build a frequency map of URL → visit count from current batch + historical count from artifact store
2. URLs with ≥ `repeat_visit_threshold` (default 3) visits get tier escalated by one level
3. Escalation is applied after dwell-time tiering but before pipeline publication
4. Store `"repeat_visits": N` in artifact metadata
5. Only content URLs participate — social media and skipped URLs excluded
6. The repeat visit window query uses the sync_state cursor to bound the lookback efficiently

---

## Error Handling and Resilience

| Error Scenario | Behavior |
|---|---|
| History file not found | `Connect()` fails with descriptive error; health = `error` |
| History file locked (copy fails) | Retry once after 5s; if still fails, skip sync cycle; health = `error` |
| SQLite parse error | Return error from `Sync()`; health = `error`; do not partially process |
| Content fetch failure (per-URL) | Create metadata-only artifact; set `content_fetch_failed: true`; continue |
| Network unavailable (all fetches fail) | All high-dwell URLs get metadata-only artifacts; flag for retry; health = `healthy` |
| Disk full during temp copy | Return error from `Sync()`; health = `error` |
| Corrupted / missing cursor | Fall back to lookback-window sync with dedup protection |
| Temp file cleanup | `defer os.Remove(tmpPath)` in `Sync()`; startup housekeeping clears orphaned tmp files |

---

## Security & Privacy

- **Read-only access:** The connector never writes to Chrome's History database. The temp copy is opened with `?mode=ro` SQLite flag.
- **Path validation:** `config.HistoryPath` is validated to prevent path traversal (must be an absolute path, no `..` components)
- **Content fetching:** Respects `robots.txt`, uses honest User-Agent header, rate-limits requests
- **Privacy tiers:** Short-dwell casual browsing stored as domain-level aggregates only — no individual URL trail persisted
- **Local only:** No browsing data sent to external services. Content extraction and embedding use local Ollama.
- **Temp file cleanup:** Temp copies are deleted immediately after parsing via `defer os.Remove`

---

## Observability

Structured log entries per sync cycle:

```json
{
  "msg": "browser history sync complete",
  "urls_processed": 150,
  "urls_skipped": 45,
  "tier_full": 12,
  "tier_standard": 28,
  "tier_light": 60,
  "tier_metadata": 50,
  "social_aggregates": 3,
  "content_fetches_ok": 35,
  "content_fetches_failed": 5,
  "repeat_escalations": 8,
  "duration_ms": 12500,
  "cursor_advanced": true
}
```

Health endpoint includes: last sync timestamp, URLs processed by tier, skip counts, content fetch stats, History file accessibility, access strategy.

---

## Testing & Validation Strategy

| Test Type | What It Validates | Location |
|---|---|---|
| Unit — `parseBrowserConfig` | Config parsing, defaults, validation errors | `internal/connector/browser/connector_test.go` |
| Unit — `processEntries` | Skip filtering, tier assignment, social aggregation, repeat detection, privacy gate | `internal/connector/browser/connector_test.go` |
| Unit — `ParseChromeHistorySince` | Cursor-based query, Chrome time conversion, no row limit | `internal/connector/browser/browser_test.go` |
| Unit — `copyHistoryFile` | Temp file creation, cleanup, error cases | `internal/connector/browser/connector_test.go` |
| Unit — `buildSocialAggregate` | Aggregate artifact structure, content type, metadata | `internal/connector/browser/connector_test.go` |
| Unit — `detectRepeatVisits` | Frequency counting, threshold, window bounds | `internal/connector/browser/connector_test.go` |
| Unit — `escalateTier` | Each tier transition, full stays full | `internal/connector/browser/connector_test.go` |
| Integration | Full Sync() with a real SQLite test fixture, StateStore cursor persistence | `tests/integration/` |
| E2E | Connector registered, sync runs, artifacts appear in search results | `tests/e2e/` |

Unit tests use a pre-built Chrome History SQLite fixture with known entries covering all tiers, social media, skip URLs, and repeat visits.

---

## Alternatives Considered

### File Access Strategy

| Option | Pros | Cons | Decision |
|---|---|---|---|
| Copy-then-read | No lock contention, works universally | Slightly stale (seconds), temp disk usage | **Chosen — default** |
| WAL-mode read | Fresher data, no temp file | Platform-dependent edge cases with Chrome's WAL | Available as config option |
| inotify/fswatch | Real-time detection | Complexity, platform differences, unnecessary for 4h poll | Rejected |

### Cursor Format

| Option | Pros | Cons | Decision |
|---|---|---|---|
| Chrome visit_time integer | Native to the query, no conversion needed in SQL | Opaque to humans | **Chosen** |
| RFC3339 timestamp | Human-readable | Requires conversion on every query, precision loss | Rejected |

### Social Media Handling

| Option | Pros | Cons | Decision |
|---|---|---|---|
| Skip entirely | Simplest | Loses social reading behavior signal | Rejected |
| Process individually | Consistent with other URLs | Floods knowledge graph with social noise | Rejected |
| Domain aggregation with exceptions | Captures behavior signal without noise | Slightly more complex | **Chosen** |

---

## Files Changed Summary

| File | Change Type | Description |
|---|---|---|
| `internal/connector/browser/connector.go` | **New** | `Connector` struct implementing `connector.Connector` interface |
| `internal/connector/browser/browser.go` | **Modified** | Add `ParseChromeHistorySince`, `GoTimeToChrome`, `ChromeTimeToGo` |
| `cmd/core/main.go` | **Modified** | Register browser-history connector, conditional connect |
| `internal/config/config.go` | **Modified** | Add `BrowserHistory` config struct to `Connectors` |
| `config/smackerel.yaml` | **Modified** | Add `browser-history` connector config block |
| `internal/connector/browser/connector_test.go` | **New** | Unit tests for connector logic |
