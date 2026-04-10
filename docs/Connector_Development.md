# Connector Development Guide

How to build and integrate a new passive data connector into Smackerel.

## Current Connector Inventory

| Connector | Package | Auth | Data Source | Spec |
|-----------|---------|------|-------------|------|
| Gmail | `internal/connector/imap` | OAuth2 (Google) | Gmail REST API | — |
| Google Calendar | `internal/connector/caldav` | OAuth2 (Google) | Calendar API v3 | — |
| YouTube | `internal/connector/youtube` | OAuth2 (Google) | Data API v3 | — |
| RSS / Atom | `internal/connector/rss` | None | Feed URLs | — |
| Bookmarks | `internal/connector/bookmarks` | None (file import) | Chrome JSON, Netscape HTML | `009-bookmarks-connector` |
| Browser History | `internal/connector/browser` | None (file-based) | Chrome SQLite | `010-browser-history-connector` |
| Google Keep | `internal/connector/keep` | OAuth2 / app password | Takeout JSON or gkeepapi | `007-google-keep-connector` |
| Google Maps | `internal/connector/maps` | None (file import) | Takeout location history | `011-maps-connector` |
| Hospitable | `internal/connector/hospitable` | API token | Hospitable REST API | `012-hospitable-connector` |
| Discord | `internal/connector/discord` | Bot token | Discord REST API | `014-discord-connector` |
| Twitter / X | `internal/connector/twitter` | Bearer token (optional) | Data archive + API v2 | `015-twitter-connector` |
| Weather | `internal/connector/weather` | None | Open-Meteo API | `016-weather-connector` |
| Government Alerts | `internal/connector/alerts` | None | USGS Earthquake API | `017-gov-alerts-connector` |
| Financial Markets | `internal/connector/markets` | Finnhub API key | Finnhub + CoinGecko | `018-financial-markets-connector` |

## Connector Interface

Every connector implements the `connector.Connector` interface defined in `internal/connector/connector.go`:

```go
type Connector interface {
    // ID returns the unique identifier for this connector instance.
    ID() string

    // Connect initializes the connector with the given configuration.
    Connect(ctx context.Context, config ConnectorConfig) error

    // Sync fetches new items since the last cursor position.
    // Returns the fetched items, a new cursor, and any error.
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)

    // Health returns the current health status of the connector.
    Health(ctx context.Context) HealthStatus

    // Close shuts down the connector and releases resources.
    Close() error
}
```

### Health States

Connectors report one of six health states:

| Status | Meaning |
|--------|---------|
| `healthy` | Connected and last sync succeeded |
| `syncing` | Currently running a sync cycle |
| `degraded` | Partially functional (some resources unavailable) |
| `failing` | Repeated sync failures, approaching circuit breaker |
| `error` | Last sync failed or configuration is invalid |
| `disconnected` | Not yet initialized (pre-`Connect`) |

### ConnectorConfig

Configuration is passed to `Connect()` as a `ConnectorConfig` struct:

```go
type ConnectorConfig struct {
    AuthType       string                 // oauth2, api_key, token, none
    Credentials    map[string]string      // type-specific credentials
    SyncSchedule   string                 // cron expression
    Enabled        bool
    ProcessingTier string                 // full, standard, light, metadata
    Qualifiers     map[string]interface{}
    SourceConfig   map[string]interface{} // connector-specific settings
}
```

### RawArtifact

`Sync()` returns a slice of `RawArtifact` — the normalized output format:

```go
type RawArtifact struct {
    SourceID    string                 // connector ID (e.g., "bookmarks")
    SourceRef   string                 // unique reference within the source
    ContentType string                 // MIME type or artifact type
    Title       string
    RawContent  string
    URL         string                 // optional
    Metadata    map[string]interface{} // source-specific metadata
    CapturedAt  time.Time
}
```

## Adding a New Connector

### 1. Create the package

Create a new directory under `internal/connector/<name>/` with at least a `connector.go` file:

```go
package myconnector

import (
    "context"
    "github.com/smackerel/smackerel/internal/connector"
)

// Compile-time interface check.
var _ connector.Connector = (*MyConnector)(nil)

type MyConnector struct {
    id     string
    health connector.HealthStatus
}

func New(id string) *MyConnector {
    return &MyConnector{id: id, health: connector.HealthDisconnected}
}

func (c *MyConnector) ID() string { return c.id }

func (c *MyConnector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error {
    // Validate config, initialize clients, set health to Healthy
    c.health = connector.HealthHealthy
    return nil
}

func (c *MyConnector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.health = connector.HealthSyncing
    defer func() { c.health = connector.HealthHealthy }()

    // Fetch items since cursor, return new cursor
    return nil, cursor, nil
}

func (c *MyConnector) Health(ctx context.Context) connector.HealthStatus {
    return c.health
}

func (c *MyConnector) Close() error {
    return nil
}
```

Use `var _ connector.Connector = (*MyConnector)(nil)` for the compile-time interface check — every connector in the repo follows this pattern.

### 2. Register in main.go

In `cmd/core/main.go`, import the package and register with the supervisor:

```go
import myConnector "github.com/smackerel/smackerel/internal/connector/myconnector"

// In run():
myConn := myConnector.New("my-source")
registry.Register(myConn)
```

If the connector needs auto-start (no OAuth flow required), add startup logic similar to the bookmarks or browser-history connectors — read config from environment variables, call `Connect()`, then `supervisor.StartConnector()`.

### 3. Add config to smackerel.yaml

Add a section under `connectors:` in `config/smackerel.yaml`:

```yaml
connectors:
  my-source:
    enabled: true
    sync_schedule: "*/5 * * * *"
    source_config:
      # connector-specific settings
```

Then regenerate: `./smackerel.sh config generate`.

## The Sync Loop

The supervisor (`internal/connector/supervisor.go`) manages every connector's lifecycle:

1. **Startup** — `StartConnector()` spawns a goroutine for the connector
2. **Load cursor** — reads the last cursor from the `sync_state` table via `StateStore`
3. **Call Sync()** — the connector fetches items since the cursor and returns a new cursor
4. **Save state** — on success, the supervisor writes the new cursor and item count back to `sync_state`
5. **Wait** — sleeps for 5 minutes (the default sync interval), then repeats from step 2
6. **Error handling** — on failure, exponential backoff kicks in (1s → 2s → 4s → 8s → 16s, max 5 retries per cycle). After max retries, waits 60 seconds and resets.

### Cursor Pattern

Cursors are opaque strings — each connector decides the format:

- **Timestamp cursor** — most Google API connectors use ISO 8601 timestamps or sync tokens
- **Page token** — YouTube connector uses API page tokens
- **File list cursor** — bookmarks connector JSON-encodes a list of processed file names
- **Offset** — simple integer offset for paginated APIs

The supervisor stores and retrieves cursors via the `StateStore` (backed by the `sync_state` PostgreSQL table). Connectors receive the last cursor in `Sync()` and return the new one.

### Incremental Sync

Always sync incrementally. The first sync (empty cursor) should fetch historical data up to a configurable lookback window. Subsequent syncs should only fetch items newer than the cursor. This keeps sync cycles fast and avoids reprocessing.

## Error Handling and Health Transitions

### Within Sync()

- Set health to `syncing` at the start, restore to `healthy` or `error` on exit
- Return errors rather than panicking — the supervisor handles retry logic
- For partial failures (some items processed, some failed), return the successfully processed items with an updated cursor, and log the failures. The next cycle will retry from the new cursor position.

### Supervisor Recovery

The supervisor wraps each connector in `runWithRecovery()`:

- **Panic recovery** — if a connector panics, the supervisor catches it, waits 5 seconds, and restarts
- **Circuit breaker** — 5 panics within a 10-minute window disables the connector permanently until manual restart
- **Graceful shutdown** — `StopAll()` cancels all connector contexts and prevents new starts

### Backoff

The `Backoff` struct (`internal/connector/backoff.go`) provides exponential backoff with ±25% jitter:

- Base delay: 1 second
- Max delay: 16 seconds
- Max retries: 5 per cycle
- Resets on successful sync

## State Store

Sync state is persisted in the `sync_state` PostgreSQL table via `StateStore` (`internal/connector/state.go`):

| Column | Purpose |
|--------|---------|
| `source_id` | Connector ID (primary key) |
| `enabled` | Whether the connector is active |
| `sync_cursor` | Opaque cursor string |
| `items_synced` | Cumulative count of synced items |
| `errors_count` | Current error count |
| `last_error` | Most recent error message |
| `last_sync` | Timestamp of last sync attempt |

Use `StateStore.Get()` to read state and `StateStore.Save()` to persist after a successful sync. `RecordError()` increments error count without overwriting the cursor.

## Testing Patterns

### Unit Test Structure

Each connector package should have a `*_test.go` file. Follow the pattern in `internal/connector/connector_test.go`:

```go
package myconnector

import (
    "context"
    "testing"

    "github.com/smackerel/smackerel/internal/connector"
)

func TestMyConnector_Interface(t *testing.T) {
    // Compile-time check is sufficient, but this documents the intent
    var _ connector.Connector = New("test")
}

func TestMyConnector_Connect(t *testing.T) {
    c := New("test")
    err := c.Connect(context.Background(), connector.ConnectorConfig{
        AuthType: "none",
        Enabled:  true,
        SourceConfig: map[string]interface{}{
            // test-specific config
        },
    })
    if err != nil {
        t.Fatalf("connect: %v", err)
    }
    if c.Health(context.Background()) != connector.HealthHealthy {
        t.Error("expected healthy after connect")
    }
}

func TestMyConnector_Sync_EmptyCursor(t *testing.T) {
    c := New("test")
    // Setup test fixtures...
    c.Connect(context.Background(), connector.ConnectorConfig{...})

    items, cursor, err := c.Sync(context.Background(), "")
    if err != nil {
        t.Fatalf("sync: %v", err)
    }
    // Assert items and cursor
}

func TestMyConnector_Sync_IncrementalCursor(t *testing.T) {
    // Verify that a non-empty cursor produces only newer items
}

func TestMyConnector_Health_Transitions(t *testing.T) {
    c := New("test")
    if c.Health(context.Background()) != connector.HealthDisconnected {
        t.Error("expected disconnected before connect")
    }
    // Connect, verify healthy; trigger error, verify error state
}
```

### What to Test

- **Interface compliance** — compile-time `var _ connector.Connector = ...` check
- **Connect validation** — missing config, invalid paths, bad credentials
- **Sync with empty cursor** — initial full sync behavior
- **Sync with existing cursor** — incremental sync only returns new items
- **Health state transitions** — disconnected → healthy → syncing → healthy/error
- **Error cases** — network failures, malformed data, permission errors
- **Close** — resources are released, health transitions to disconnected

### Running Tests

```bash
./smackerel.sh test unit
```

## Existing Connectors

For reference, the codebase has these connector implementations:

| Connector | Package | Auth | Cursor Type |
|-----------|---------|------|-------------|
| Gmail (IMAP) | `internal/connector/imap/` | OAuth2 (Google) | Timestamp / sync token |
| Google Calendar (CalDAV) | `internal/connector/caldav/` | OAuth2 (Google) | Sync token |
| YouTube | `internal/connector/youtube/` | OAuth2 (Google) | Page token |
| RSS / Atom | `internal/connector/rss/` | None | Item GUIDs |
| Bookmarks | `internal/connector/bookmarks/` | None (file-based) | Processed file list |
| Browser History | `internal/connector/browser/` | None (file-based) | Timestamp |
| Google Keep | `internal/connector/keep/` | None (Takeout) / gkeepapi | Timestamp |
| Google Maps Timeline | `internal/connector/maps/` | None (Takeout file) | Processed file list |
| Hospitable | `internal/connector/hospitable/` | API token | Timestamp |

The **bookmarks connector** (`internal/connector/bookmarks/connector.go`) is the simplest implementation and a good starting point for new connectors.
