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
| Cloud Drives — Google Drive | `internal/drive/google` | OAuth2 (Google) | Google Drive REST API v3 | `038-cloud-drives-integration` |
| Cloud Photos — Immich | `internal/connector/photos/adapters/immich` | API key | Immich REST API | `040-cloud-photo-libraries` |
| Cloud Photos — PhotoPrism | `internal/connector/photos/adapters/photoprism` | API key | PhotoPrism REST API | `040-cloud-photo-libraries` |
| QF Decisions | `internal/connector/qfdecisions` | QF token / service credential | QuantitativeFinance decision-packet read surface | `041-qf-companion-connector` |

## QF Decisions Connector Boundary

The QF Decisions connector is a companion connector, not a markets connector and not a recommendation engine. Its job is to ingest QF-owned decision artifacts and preserve their authority boundary inside Smackerel.

| Concern | Requirement |
|---------|-------------|
| Connector ID | Use `qf-decisions` for the default connector instance |
| Artifact type | Emit QF packets as source-qualified artifacts such as `qf/decision-packet`, not as Smackerel-local recommendations |
| Required metadata | Preserve QF `packet_id`, `intent_id`, `scenario_id`, `trace_id`, `approval_state`, deep link, `CalibrationBadge`, and `DataProvenanceBadge` |
| Trust metadata | Never synthesize, upgrade, rewrite, or hide QF-provided trust badges |
| Actions | Pre-MVP connector exposes read-only packet surfacing and `PersonalEvidenceBundle` export only |
| Evidence bundle export | Include source artifact IDs, user consent, sensitivity classification, and provenance when exporting context back to QF |
| Failure behavior | If required packet IDs, trace IDs, or badges are missing, mark the artifact degraded and avoid action prompts |

## Cloud Drives Connector Boundary (Spec 038)

Cloud Drive providers (Google Drive in scope today) live under
`internal/drive/` and implement the `DriveProvider` interface in
`internal/drive/provider.go`, **not** the generic `connector.Connector`
interface above. The two surfaces are intentionally different because
Drive providers are bidirectional (read + save), folder-aware, and OAuth-redirect-driven.

| Concern | Requirement |
|---------|-------------|
| Provider package | One subpackage per provider under `internal/drive/<provider>/` (e.g., `internal/drive/google/`). |
| OAuth flow | Implement `BeginConnect` (issue auth URL + nonce row in `drive_oauth_states`) and `FinalizeConnect` (consume nonce, exchange code, persist `drive_connections`). The OAuth callback is `GET /v1/connectors/drive/oauth/callback`. |
| Credential storage | Persist the bearer (access) token plaintext as `bearer:<token>` in `drive_connections.credentials_ref` with the provider-supplied `expires_at`. Refresh tokens are intentionally not persisted in Scope 1; the dedicated credentials vault is deferred per spec 038 design.md §2.3 + decision-log A1. Until then, do not introduce a parallel secret path. |
| Scan + monitor | Drive providers MUST publish to NATS subjects on the `DRIVE` stream (`drive.scan.request.<provider>`, `drive.scan.progress.<provider>`, `drive.change.<provider>`, `drive.extract.request`, `drive.classify.request`). Cursor durability lives in `drive_cursors` with bounded-rescan fallback. |
| Save Rules engine | Provider writes flow through the rule engine (`internal/drive/rules/`) and the Save Service (`internal/drive/save/`). Direct provider writes from `internal/pipeline/` or `internal/telegram/` are forbidden. |
| Sensitivity + confirmation | Low-confidence and sensitive outcomes route through the Screen 11 confirmation surface (`/v1/drive/confirmations/{id}`) and Telegram numbered replies; both share one handler so the exactly-once contract holds across channels. |
| Configuration | All provider knobs (folder include/exclude, `max_depth`, sync interval, MIME allow-lists, sensitivity thresholds) live under `drive.providers.<id>` in `config/smackerel.yaml`; no Go-source literals. |

The Save Rules engine, retrieval service, scan loop, and tool registrations
work unchanged when a new provider is added (BS-008 acceptance criterion).

## Cloud Photo Libraries Connector Boundary (Spec 040)

Photo-library providers (Immich and PhotoPrism in scope today) live under
`internal/connector/photos/adapters/<provider>/` and implement the
provider-neutral `photolib.PhotoLibrary` contract in
`internal/connector/photos/library.go`. They are first-class providers
alongside cloud drives, but with their own data model (photo-specific
artifacts, dedupe clusters, lifecycle, sensitivity reveal).

| Concern | Requirement |
|---------|-------------|
| Provider package | One adapter per provider under `internal/connector/photos/adapters/<provider>/`. Both adapters share the same Go contract — capability probe, scope enumeration, scanner/monitor/skip-ledger, fetch, writers (AddToAlbum/Tag/Favorite/Archive/Delete/Upload/RenameFaceCluster). |
| Credential storage | Provider API keys live under `photos.providers.<provider>.access_token` in `config/smackerel.yaml`. Empty values fail-loud at startup. |
| Capability taxonomy | Unsupported / limited operations MUST use a `LimitationCode` constant from `internal/connector/photos/capability_taxonomy.go`. The same codes appear in API responses (`409 PROVIDER_LIMITATION`), Prometheus metrics (`smackerel_photos_capabilities_limited_total`), and the PWA Photo Health dashboard. The taxonomy canary integration test asserts those three surfaces stay in sync. |
| Cross-provider dedupe | Use `internal/connector/photos.SameCrossProviderDuplicate` (strict-hash equality, weak bytes+captured_at fallback) and the artifact-id reuse path in `store.go` so the same content from two providers maps to one canonical artifact. |
| Action confirmation | Destructive actions (archive, delete, album removal) MUST flow through `PhotoActionToken` mint/confirm (`/v1/photos/actions/plan` → `/v1/photos/actions/confirm`) with scope-hash drift checks; `ConfirmedWriter` wraps every `ProviderWriter` so a write cannot fire before confirmation. Delete also requires a text-confirmation step. |
| Sensitivity reveal | Sensitive previews are gated server-side at `/v1/photos/{id}/preview` (returns `403 sensitivity_requires_reveal` without a reveal token); single-use, actor-bound, TTL + hash-protected reveal tokens are minted via `/v1/photos/{id}/reveal`. Search results omit preview URLs and set `requires_reveal=true` for sensitive rows. |
| NATS surface | Photo-library providers publish to the `PHOTOS` stream (`photos.classify`, `photos.ocr`, `photos.embed`, `photos.lifecycle`, `photos.dedupe`, plus `.result`/`.classified`/`.embedded`/`.ocred` responses). |
| Capture + cross-feature routing | The unified `POST /v1/photos/upload` pipeline is shared by Telegram, the mobile PWA, and the web; cross-feature routing (`internal/connector/photos/routing.go`) classifies into `expense`, `recipe`, `document`, `knowledge`, `annotation`, `list`, `mealplan`, or `intelligence` and persists to `photo_routing_decisions` with `UNIQUE(photo_id, target)`. |

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
| Google Drive | `internal/drive/google/` | OAuth2 (Google) | `drive_cursors` (provider sync token) with bounded-rescan fallback |
| Immich (photos) | `internal/connector/photos/adapters/immich/` | API key | Timestamp/asset cursor in `photo_sync_state` |
| PhotoPrism (photos) | `internal/connector/photos/adapters/photoprism/` | API key | Timestamp/asset cursor in `photo_sync_state` |

The **bookmarks connector** (`internal/connector/bookmarks/connector.go`) is the simplest implementation and a good starting point for new generic `connector.Connector` connectors. For Drive providers, start from `internal/drive/google/` and the `DriveProvider` interface in `internal/drive/provider.go`. For photo-library providers, start from `internal/connector/photos/adapters/immich/` and the `photolib.PhotoLibrary` contract in `internal/connector/photos/library.go`.
