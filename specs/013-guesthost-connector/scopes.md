# Scopes: 013 — GuestHost Connector & Hospitality Intelligence

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:**
- `internal/connector/guesthost/` (new directory: `types.go`, `client.go`, `connector.go`, `normalizer.go`)
- `internal/graph/hospitality_linker.go` (new file — extends graph layer)
- `internal/digest/hospitality.go` (new file — extends digest layer)
- `internal/api/context.go` (new file — new API endpoint)
- `internal/intelligence/hospitality.go` (new file — alert/hint engine)
- `internal/db/` (new migration files for `guests` and `properties` tables, new repository files)
- `config/smackerel.yaml` (add `connectors.guesthost`, `intelligence.hospitality`, `context_api` sections)
- `cmd/core/main.go` (register GH connector, wire hospitality linker, register context API route)
- `tests/` (new integration and e2e test files)

**Excluded surfaces:** No changes to existing connector implementations (RSS, IMAP, CalDAV, YouTube, Browser, Bookmarks, Keep, Maps, Hospitable). No changes to existing NATS stream configurations. No changes to the ML sidecar (it processes hospitality artifacts through the existing pipeline). No changes to existing search API or web handlers (except adding the new context route). Module 5 (Hospitable MCP mode) is out of scope — it extends spec 012 and will be handled there.

### Phase Order

1. **Scope 1: GH Connector — API Client, Types & Config** — Define GH activity feed response structs, build the HTTP client with Bearer token auth, pagination (`hasMore`), rate-limit retry, and event-type filtering. Add `connectors.guesthost` config section to `smackerel.yaml`. Client is testable in isolation after this scope.
2. **Scope 2: GH Connector — Implementation & Normalizer** — Implement the `Connector` interface wrapping the API client, build normalizers for all 11 event types (booking, guest, review, message, task, expense, property), add cursor management (RFC3339 timestamps), register in `cmd/core/main.go`. Full sync lifecycle works after this scope.
3. **Scope 3: Hospitality Graph Nodes & Linker** — Create `guests` and `properties` database tables via migration, implement repository layer, build the `HospitalityLinker` that upserts guest/property nodes and creates hospitality edge types (`STAYED_AT`, `REVIEWED`, `MANAGED_BY`, `ISSUE_AT`, `DURING_STAY`), seed hospitality topics. Graph intelligence layer works after this scope.
4. **Scope 4: Hospitality Digest** — Extend the digest generator with `HospitalityDigestContext` assembly (arrivals, departures, pending tasks, revenue snapshot, guest alerts, property alerts), build the hospitality prompt template, wire active-connector detection. Domain-specific daily briefings work after this scope.
5. **Scope 5: Context Enrichment API** — Add `POST /api/context-for` endpoint with guest/property/booking entity resolution, rule-based communication hints generation, sentiment trajectory computation, alert checking, and API key authentication. External systems can query Smackerel intelligence after this scope.

### New Types & Signatures

```go
// internal/connector/guesthost/types.go
type ActivityEvent struct { ID, Type, Timestamp string; EntityID string; Data json.RawMessage }
type ActivityFeedResponse struct { Events []ActivityEvent; Cursor string; HasMore bool }
type BookingData struct { PropertyID, PropertyName, GuestID, GuestEmail, GuestName, CheckIn, CheckOut, Source, Status string; TotalPrice float64 }
type ReviewData struct { PropertyID, PropertyName, GuestEmail, GuestName, Rating, Text, HostResponse string }
type MessageData struct { PropertyID, PropertyName, GuestEmail, GuestName, SenderRole, Body string; BookingID string }
type TaskData struct { PropertyID, PropertyName, Title, Description, Status, Category string }
type ExpenseData struct { PropertyID, PropertyName, Category, Description string; Amount float64 }
type GuestData struct { Email, Name string }
type PropertyData struct { ID, Name string }

// internal/connector/guesthost/client.go
type Client struct { baseURL, apiKey string; httpClient *http.Client }
func NewClient(baseURL, apiKey string) *Client
func (c *Client) Validate(ctx context.Context) error
func (c *Client) FetchActivity(ctx context.Context, since string, types string, limit int) (*ActivityFeedResponse, error)

// internal/connector/guesthost/connector.go
type Connector struct { id string; config connector.ConnectorConfig; client *Client; health connector.HealthStatus }
func New() *Connector
func (c *Connector) ID() string
func (c *Connector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
func (c *Connector) Health(ctx context.Context) connector.HealthStatus
func (c *Connector) Close() error

// internal/connector/guesthost/normalizer.go
func NormalizeEvent(event ActivityEvent) (connector.RawArtifact, error)

// internal/db/guest_repo.go
type GuestNode struct { ID uuid.UUID; Email, Name, Source string; TotalStays int; TotalSpend decimal.Decimal; ... }
type GuestRepository interface { UpsertByEmail(ctx, email, name, source string) (*GuestNode, error); FindByEmail(ctx, email string) (*GuestNode, error); IncrementStay(ctx, id uuid.UUID, spend decimal.Decimal) error; UpdateSentiment(ctx, id uuid.UUID, score float64) error }

// internal/db/property_repo.go
type PropertyNode struct { ID uuid.UUID; ExternalID, Source, Name string; TotalBookings int; TotalRevenue decimal.Decimal; ... }
type PropertyRepository interface { UpsertByExternalID(ctx, externalID, source, name string) (*PropertyNode, error); IncrementBookings(ctx, id uuid.UUID, revenue decimal.Decimal) error; UpdateTopics(ctx, id uuid.UUID, topics []string) error; UpdateIssueCount(ctx, id uuid.UUID, delta int) error }

// internal/graph/hospitality_linker.go
type HospitalityLinker struct { guestRepo db.GuestRepository; propertyRepo db.PropertyRepository; edgeRepo db.EdgeRepository; standardLinker *Linker }
func NewHospitalityLinker(...) *HospitalityLinker
func (l *HospitalityLinker) LinkArtifact(ctx context.Context, artifactID uuid.UUID) error

// internal/digest/hospitality.go
type HospitalityDigestContext struct { TodayArrivals, TodayDepartures []GuestStay; PendingTasks []Task; Revenue RevenueSnap; GuestAlerts []GuestAlert; PropertyAlerts []PropAlert }
func AssembleHospitalityContext(ctx context.Context, ...) (*HospitalityDigestContext, error)

// internal/api/context.go
type ContextRequest struct { EntityType, EntityID string; Include []string }
type ContextHandler struct { guestRepo db.GuestRepository; propertyRepo db.PropertyRepository; artifactRepo db.ArtifactRepository; ... }
func (h *ContextHandler) HandleContextFor(w http.ResponseWriter, r *http.Request)
```

### Validation Checkpoints

- **After Scope 1:** Unit tests validate API client request construction (correct Bearer auth header, URL formation with `since`/`types`/`limit` params, `hasMore` pagination loop). Integration test with mock HTTP server confirms multi-page activity feed fetches work end-to-end.
- **After Scope 2:** Unit tests validate normalizer output for all 11 event types with correct content types, titles, and metadata. Integration test with mock API confirms full `Sync()` produces correct `RawArtifact`s and advances cursor. E2E test confirms connector registration and first-sync pipeline.
- **After Scope 3:** Migration applies cleanly. Repository CRUD for guests/properties works. Hospitality linker creates correct graph nodes and edges from artifact metadata. Topic seeds are created on first sync. Integration tests confirm full link lifecycle from artifact → guest node → property node → edges.
- **After Scope 4:** Digest generator detects active hospitality connectors and assembles `HospitalityDigestContext`. Arrivals, departures, tasks, revenue snapshot, guest/property alerts all populated correctly. Empty-day handling omits sections. Integration test confirms end-to-end digest assembly.
- **After Scope 5:** Context API responds to guest/property/booking requests with correct data. Communication hints generated deterministically. Unknown entities return 404. API key auth works. E2E test confirms GH-to-Smackerel context flow.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|---|---|---|---|---|
| 1 | GH Connector: API Client, Types & Config | Go core, Config | 10 unit + 2 integration | Client builds correct requests, paginates hasMore, retries 429, config validates | Not started |
| 2 | GH Connector: Implementation & Normalizer | Go core, Config | 14 unit + 3 integration + 2 e2e | Connector lifecycle, normalizer maps all 11 event types, cursor management | Not started |
| 3 | Hospitality Graph Nodes & Linker | Go core, DB migration | 12 unit + 4 integration + 2 e2e | Guest/property tables, hospitality linker, edge types, topic seeds | Not started |
| 4 | Hospitality Digest | Go core | 10 unit + 3 integration + 1 e2e | Arrivals/departures/tasks/revenue/alerts in digest, empty-day handling | Not started |
| 5 | Context Enrichment API | Go core, API | 12 unit + 3 integration + 2 e2e | POST /api/context-for, guest/property/booking responses, communication hints | Not started |

---

## Scope 01: GH Connector — API Client, Types & Config

**Status:** Not started
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Define Go structs for the GuestHost activity feed API responses (`ActivityEvent`, `ActivityFeedResponse`, `BookingData`, `ReviewData`, `MessageData`, `TaskData`, `ExpenseData`, `GuestData`, `PropertyData`). Build the HTTP API client (`Client`) that handles Bearer token authentication (`tkn_xxx`), request construction for `GET /api/v1/activity?since={cursor}&types={csv}&limit=100`, `hasMore`-based pagination (loop within a single call until `hasMore=false`), rate limit detection with exponential backoff, and error classification. Add the `connectors.guesthost` configuration section to `smackerel.yaml` and implement config parsing with validation. After this scope, the API client is independently testable against a mock HTTP server.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GH-001 API client authenticates with GuestHost tenant API key
  Given a GuestHost API client configured with base URL and API key
  When Validate() is called
  Then the client sends GET request to the health endpoint with "Authorization: Bearer {api_key}" header
  And on 200 response, Validate() returns nil
  And on 401 response, Validate() returns an error containing "unauthorized"
  And on 403 response, Validate() returns an error containing "forbidden"

Scenario: SCN-GH-002 API client fetches activity feed with cursor and type filter
  Given a configured GuestHost API client
  When FetchActivity() is called with since="2026-04-01T00:00:00Z", types="booking.created,review.received", limit=100
  Then the request URL is "{base_url}/api/v1/activity?since=2026-04-01T00:00:00Z&types=booking.created,review.received&limit=100"
  And the Authorization header is set
  And the response is parsed into ActivityFeedResponse

Scenario: SCN-GH-003 API client handles hasMore pagination
  Given the GuestHost API returns hasMore=true on the first page with 100 events
  And hasMore=false on the second page with 50 events
  When FetchActivity() is called
  Then the client fetches both pages using the returned cursor
  And returns all 150 events in a single ActivityFeedResponse
  And the final cursor is from the last page

Scenario: SCN-GH-004 API client retries on rate limit (429)
  Given the GuestHost API returns 429 on the first request
  When FetchActivity() is called
  Then the client waits and retries with exponential backoff
  And on success after retry, returns the activity data
  And on 3 consecutive 429s, returns a rate limit error

Scenario: SCN-GH-005 API client handles server errors with retry
  Given the GuestHost API returns 500 on the first request
  When FetchActivity() is called
  Then the client retries with exponential backoff up to 3 times
  And on persistent 500, returns a server error

Scenario: SCN-GH-006 Config validation rejects invalid settings
  Given a smackerel.yaml with connectors.guesthost configured
  When api_key is empty and enabled is true
  Then config parsing returns an error containing "api_key"
  When base_url is empty and enabled is true
  Then config parsing returns an error containing "base_url"
  When sync_schedule is invalid cron
  Then config parsing returns a validation error

Scenario: SCN-GH-007 API client omits since param on first sync
  Given a client called with an empty cursor (first sync)
  When FetchActivity() is called with since=""
  Then the request URL omits the since parameter entirely
  And GH returns oldest events first
```

**Mapped Requirements:** FR-001, FR-004, FR-005, FR-012

### Implementation Plan

**Files created:**
- `internal/connector/guesthost/types.go` — All API response structs: `ActivityEvent`, `ActivityFeedResponse`, `BookingData`, `ReviewData`, `MessageData`, `TaskData`, `ExpenseData`, `GuestData`, `PropertyData`
- `internal/connector/guesthost/client.go` — `Client` struct, `NewClient()`, `Validate()`, `FetchActivity()`, `doRequest()`, `doGet()`

**Files modified:**
- `config/smackerel.yaml` — Add `connectors.guesthost` section with `enabled`, `base_url`, `api_key`, `sync_schedule`, `event_types`

**Components touched:**
- `Client.doRequest()`: builds URL, sets `Authorization: Bearer {api_key}`, sets `Content-Type`, handles timeout
- `Client.doGet()`: calls `doRequest`, reads body, checks status code, retries on 429/5xx via existing `internal/connector/backoff.go`
- `Client.FetchActivity()`: builds URL with query params, loops on `hasMore=true`, accumulates events, returns combined result with final cursor
- `Client.Validate()`: `doGet` on GH health endpoint, checks for 200
- Config parsing: extracts from `ConnectorConfig.SourceConfig`, validates required fields when enabled

**Consumer Impact Sweep:** No existing surfaces modified. Config section is additive. Types are package-internal.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-1-01 | TestClientAuthHeader | unit | `internal/connector/guesthost/client_test.go` | Request contains `Authorization: Bearer {api_key}` | SCN-GH-001 |
| T-1-02 | TestClientValidateSuccess | unit | `internal/connector/guesthost/client_test.go` | 200 response → nil error | SCN-GH-001 |
| T-1-03 | TestClientValidateUnauthorized | unit | `internal/connector/guesthost/client_test.go` | 401 response → error with "unauthorized" | SCN-GH-001 |
| T-1-04 | TestClientValidateForbidden | unit | `internal/connector/guesthost/client_test.go` | 403 response → error with "forbidden" | SCN-GH-001 |
| T-1-05 | TestFetchActivityURLConstruction | unit | `internal/connector/guesthost/client_test.go` | Correct base path, since, types, limit params | SCN-GH-002 |
| T-1-06 | TestFetchActivityHasMorePagination | unit | `internal/connector/guesthost/client_test.go` | hasMore=true → fetches next page; hasMore=false → stops | SCN-GH-003 |
| T-1-07 | TestClientRetryOn429 | unit | `internal/connector/guesthost/client_test.go` | First 429 then 200 → success after retry | SCN-GH-004 |
| T-1-08 | TestClientMaxRetriesOn429 | unit | `internal/connector/guesthost/client_test.go` | 3 consecutive 429s → rate limit error | SCN-GH-004 |
| T-1-09 | TestClientRetryOnServerError | unit | `internal/connector/guesthost/client_test.go` | 500 then 200 → success after retry | SCN-GH-005 |
| T-1-10 | TestConfigValidation | unit | `internal/connector/guesthost/connector_test.go` | Empty api_key + enabled → error; empty base_url → error | SCN-GH-006 |
| T-1-11 | TestFetchActivityEmptyCursorOmitsSince | unit | `internal/connector/guesthost/client_test.go` | Empty since → URL lacks since param | SCN-GH-007 |
| T-1-12 | TestFetchActivityFullPaginationFlow | integration | `tests/integration/guesthost_test.go` | Mock HTTP server with 2 pages → all events collected, final cursor correct | SCN-GH-003 |
| T-1-13 | TestClientRateLimitRecovery | integration | `tests/integration/guesthost_test.go` | Mock server returns 429 then 200 → client recovers | SCN-GH-004 |

### Definition of Done

#### Core Items

- [ ] `internal/connector/guesthost/types.go` created with `ActivityEvent`, `ActivityFeedResponse`, `BookingData`, `ReviewData`, `MessageData`, `TaskData`, `ExpenseData`, `GuestData`, `PropertyData` structs
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] `internal/connector/guesthost/client.go` created with `Client`, `NewClient()`, `Validate()`, `FetchActivity()`
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] `Client` sends `Authorization: Bearer {api_key}` header on every request
  > Verify: T-1-01 TestClientAuthHeader PASS
- [ ] `Validate()` distinguishes 200 (success), 401 (unauthorized), 403 (forbidden)
  > Verify: T-1-02, T-1-03, T-1-04 PASS
- [ ] `FetchActivity()` constructs correct URL with `since`, `types`, `limit` query params
  > Verify: T-1-05 TestFetchActivityURLConstruction PASS
- [ ] `FetchActivity()` loops on `hasMore=true`, accumulates events, returns combined result
  > Verify: T-1-06 TestFetchActivityHasMorePagination PASS
- [ ] Empty cursor omits `since` parameter (first sync fetches oldest events)
  > Verify: T-1-11 TestFetchActivityEmptyCursorOmitsSince PASS
- [ ] Rate limit (429) triggers exponential backoff with max 3 retries via existing `backoff.go`
  > Verify: T-1-07, T-1-08 PASS
- [ ] Server errors (5xx) trigger exponential backoff with max 3 retries
  > Verify: T-1-09 TestClientRetryOnServerError PASS
- [ ] `config/smackerel.yaml` has `connectors.guesthost` section with `enabled`, `base_url`, `api_key`, `sync_schedule`, `event_types`
  > Verify: Config section present in YAML
- [ ] Config parsing validates required fields when enabled, returns clear errors
  > Verify: T-1-10 TestConfigValidation PASS

#### Build Quality Gate

- [ ] All unit tests pass → `./smackerel.sh test unit`
- [ ] Lint passes with zero warnings → `./smackerel.sh lint`
- [ ] Format check passes → `./smackerel.sh format --check`
- [ ] No TODO/FIXME/STUB markers in new files

---

## Scope 02: GH Connector — Implementation & Normalizer

**Status:** Not started
**Priority:** P0
**Dependencies:** Scope 1 (API Client, Types & Config)

### Description

Implement the `Connector` interface in `connector.go` wrapping the API client into the standard connector lifecycle (Connect validates API key, Sync orchestrates paginated activity feed fetch and normalization, Health reports status, Close cleans up). Build normalizers in `normalizer.go` for all 11 GH event types (booking.created/updated/cancelled, guest.created/updated, review.received, message.received, task.created/completed, expense.created, property.updated), mapping each to a `RawArtifact` with correct content type, processing tier, title format, and structured hospitality metadata (property_id, guest_email, booking_id, checkin/checkout dates, revenue, booking_source). Implement cursor management using the last event timestamp. Register the connector in `cmd/core/main.go`. After this scope, the full sync lifecycle works: connect → sync → normalize → return artifacts + updated cursor.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GH-008 Connector implements standard lifecycle
  Given the GuestHost connector is registered in the connector registry
  When Connect() is called with valid config containing a valid API key
  Then the client validates the key against the GH health endpoint
  And Health() returns "healthy"
  And ID() returns "guesthost"
  When Sync() is called with an empty cursor
  Then it fetches the full activity feed (oldest first)
  And returns RawArtifacts with correct content types and metadata
  And a new cursor set to the timestamp of the last event
  When Close() is called
  Then Health() returns "disconnected"

Scenario: SCN-GH-009 Normalizer maps booking.created to booking artifact
  Given a booking.created event for guest "Sarah" at "Beach House" checking in Apr 15-18 via direct booking
  When NormalizeEvent() processes it
  Then SourceID equals "guesthost"
  And SourceRef equals the event ID
  And ContentType equals "booking"
  And Title equals "Beach House — Sarah — Apr 15-Apr 18"
  And processing tier is "standard"
  And Metadata contains property_id, property_name, guest_email, guest_name, checkin_date, checkout_date, booking_source="direct", revenue

Scenario: SCN-GH-010 Normalizer maps review.received to review artifact
  Given a review.received event with 5-star rating from "John" at "Mountain Cabin"
  When NormalizeEvent() processes it
  Then ContentType equals "review"
  And Title equals "Mountain Cabin — 5★ review from John"
  And processing tier is "full"
  And RawContent contains the review text
  And Metadata contains property_id, guest_email, edge hint data

Scenario: SCN-GH-011 Normalizer maps message.received to guest_message artifact
  Given a message.received event from guest "Sarah" about "Beach House"
  When NormalizeEvent() processes it
  Then ContentType equals "guest_message"
  And Title equals "Beach House — Message from Sarah"
  And processing tier is "full"
  And Metadata contains property_id, guest_email, booking_id

Scenario: SCN-GH-012 Normalizer maps task.created to task artifact
  Given a task.created event "Deep clean before next guest" at "Beach House"
  When NormalizeEvent() processes it
  Then ContentType equals "task"
  And Title equals "Beach House — Task: Deep clean before next guest"
  And processing tier is "standard"
  And Metadata contains property_id

Scenario: SCN-GH-013 Normalizer maps expense.created to financial artifact
  Given an expense.created event for "Plumber — $350" at "Beach House"
  When NormalizeEvent() processes it
  Then ContentType equals "financial"
  And Title equals "Beach House — Expense: Plumber $350.00"
  And processing tier is "standard"
  And Metadata contains property_id, revenue (as negative for expenses)

Scenario: SCN-GH-014 Incremental sync advances cursor
  Given a previous sync returned cursor "2026-04-01T12:00:00Z"
  When Sync() is called with that cursor
  Then FetchActivity() is called with since="2026-04-01T12:00:00Z"
  And only events after that timestamp are returned
  And the new cursor is the timestamp of the last returned event

Scenario: SCN-GH-015 Sync handles no new events gracefully
  Given the activity feed returns zero events
  When Sync() completes
  Then zero artifacts are returned
  And the cursor is unchanged
  And Health() remains "healthy"

Scenario: SCN-GH-016 Event type filter restricts sync
  Given event_types configured as "booking.created,review.received"
  When Sync() runs
  Then FetchActivity() includes types="booking.created,review.received"
  And only matching event types are returned

Scenario: SCN-GH-017 Content hash dedup prevents re-ingestion
  Given a booking.created event was already ingested in a previous sync
  When the same event appears again (by SourceRef)
  Then DedupChecker rejects it
  And the artifact is not re-published to NATS

Scenario: SCN-GH-018 Normalizer maps all remaining event types
  Given events of type guest.created, guest.updated, booking.updated, booking.cancelled, task.completed, property.updated
  When NormalizeEvent() processes each
  Then each produces the correct ContentType, Title, processing tier, and Metadata per the design mapping table
```

**Mapped Requirements:** FR-001, FR-002, FR-003, FR-004, FR-005

### Implementation Plan

**Files created:**
- `internal/connector/guesthost/connector.go` — `Connector` struct implementing `connector.Connector`, config extraction, `New()`, `Connect()`, `Sync()`, `Health()`, `Close()`
- `internal/connector/guesthost/normalizer.go` — `NormalizeEvent()` with per-event-type dispatch, `normalizeBooking()`, `normalizeReview()`, `normalizeMessage()`, `normalizeTask()`, `normalizeExpense()`, `normalizeGuest()`, `normalizeProperty()`

**Files modified:**
- `cmd/core/main.go` — Register `guesthost.New()` in the connector registry

**Components touched:**
- `Connector.Connect()`: parse config, create `Client`, call `client.Validate()`, set health
- `Connector.Sync()`: parse cursor (RFC3339 timestamp or empty) → call `FetchActivity()` → normalize each event → build content hash for dedup → return artifacts + new cursor
- `NormalizeEvent()`: switch on `event.Type`, unmarshal `event.Data` into typed struct, map to `RawArtifact` with correct ContentType, Title, ProcessingTier, Metadata
- Cursor: last event's `Timestamp` field (RFC3339 string)
- Dedup: `SourceID="guesthost"`, `SourceRef=event.ID`, content hash = SHA-256 of `event.Type + event.EntityID + event.Timestamp`
- Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
- Registration follows Keep/Hospitable pattern: `New()` → `registry.Register()`

**Consumer Impact Sweep:** Adding new connector to registry. No existing surfaces renamed or removed. Registration in main.go is additive.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-2-01 | TestConnectorID | unit | `internal/connector/guesthost/connector_test.go` | `ID()` returns `"guesthost"` | SCN-GH-008 |
| T-2-02 | TestConnectValidConfig | unit | `internal/connector/guesthost/connector_test.go` | Valid config + valid key → health is `healthy` | SCN-GH-008 |
| T-2-03 | TestConnectInvalidKey | unit | `internal/connector/guesthost/connector_test.go` | Invalid key → health is `error`, Connect returns error | SCN-GH-008 |
| T-2-04 | TestNormalizeBookingCreated | unit | `internal/connector/guesthost/normalizer_test.go` | booking.created → correct SourceRef, ContentType, Title, Metadata, tier | SCN-GH-009 |
| T-2-05 | TestNormalizeReviewReceived | unit | `internal/connector/guesthost/normalizer_test.go` | review.received → star rating in title, full tier | SCN-GH-010 |
| T-2-06 | TestNormalizeMessageReceived | unit | `internal/connector/guesthost/normalizer_test.go` | message.received → correct content type, body in content, full tier | SCN-GH-011 |
| T-2-07 | TestNormalizeTaskCreated | unit | `internal/connector/guesthost/normalizer_test.go` | task.created → correct title, standard tier | SCN-GH-012 |
| T-2-08 | TestNormalizeExpenseCreated | unit | `internal/connector/guesthost/normalizer_test.go` | expense.created → correct title with amount, standard tier | SCN-GH-013 |
| T-2-09 | TestNormalizeAllEventTypes | unit | `internal/connector/guesthost/normalizer_test.go` | All 11 event types produce correct ContentType and tier | SCN-GH-018 |
| T-2-10 | TestCursorAdvancement | unit | `internal/connector/guesthost/connector_test.go` | After sync, cursor advances to last event timestamp | SCN-GH-014 |
| T-2-11 | TestSyncNoNewEvents | unit | `internal/connector/guesthost/connector_test.go` | Empty response → zero artifacts, cursor unchanged, health healthy | SCN-GH-015 |
| T-2-12 | TestEventTypeFilter | unit | `internal/connector/guesthost/connector_test.go` | Configured types passed as CSV query param | SCN-GH-016 |
| T-2-13 | TestContentHashDedup | unit | `internal/connector/guesthost/normalizer_test.go` | Same event produces same content hash | SCN-GH-017 |
| T-2-14 | TestHealthTransitions | unit | `internal/connector/guesthost/connector_test.go` | Disconnected→healthy→syncing→healthy→disconnected | SCN-GH-008 |
| T-2-15 | TestSyncFullLifecycle | integration | `tests/integration/guesthost_test.go` | Mock API → Connect → Sync → correct artifacts + cursor | SCN-GH-008 |
| T-2-16 | TestSyncIncrementalCursor | integration | `tests/integration/guesthost_test.go` | Two syncs → second only fetches events after cursor | SCN-GH-014 |
| T-2-17 | TestSyncWithEventTypeFilter | integration | `tests/integration/guesthost_test.go` | Configured types → only matching events returned | SCN-GH-016 |
| T-2-18 | E2E: GH connector registration | e2e | `tests/e2e/guesthost_test.go` | Registry contains "guesthost" after startup | SCN-GH-008 |
| T-2-19 | E2E: Full sync pipeline | e2e | `tests/e2e/guesthost_test.go` | Mock API → sync → artifacts in DB with correct content types and metadata | SCN-GH-008 thru SCN-GH-018 |

### Definition of Done

#### Core Items

- [ ] `internal/connector/guesthost/connector.go` created with full `Connector` implementation
  > Verify: `var _ connector.Connector = (*Connector)(nil)` compiles
- [ ] `internal/connector/guesthost/normalizer.go` created with `NormalizeEvent()` handling all 11 event types
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] Connector registered in `cmd/core/main.go` following Keep/Hospitable pattern
  > Verify: grep for `guesthost.New()` in main.go
- [ ] `Connect()` validates API key via GH health endpoint, sets health correctly
  > Verify: T-2-02, T-2-03 PASS
- [ ] `Sync()` fetches activity feed, normalizes events, returns artifacts + cursor
  > Verify: T-2-15 TestSyncFullLifecycle PASS
- [ ] Normalizer maps all 11 event types to correct ContentType, Title, tier, and metadata
  > Verify: T-2-04 thru T-2-09 PASS
- [ ] Metadata includes all FR-003 hospitality fields (property_id, guest_email, booking_id, checkin/checkout, revenue, booking_source)
  > Verify: T-2-04 TestNormalizeBookingCreated checks all metadata fields
- [ ] Cursor advances to last event timestamp on each sync
  > Verify: T-2-10, T-2-16 PASS
- [ ] Empty sync returns zero artifacts, unchanged cursor, healthy status
  > Verify: T-2-11 TestSyncNoNewEvents PASS
- [ ] Event type filter correctly restricts fetched events
  > Verify: T-2-12 TestEventTypeFilter PASS
- [ ] Content hash enables dedup across syncs and sources
  > Verify: T-2-13 TestContentHashDedup PASS
- [ ] Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
  > Verify: T-2-14 TestHealthTransitions PASS
- [ ] E2E: connector registration and full sync pipeline work end-to-end
  > Verify: T-2-18, T-2-19 PASS
- [ ] Regression: Scope 1 tests still pass
  > Verify: T-1-* all PASS

#### Build Quality Gate

- [ ] All unit tests pass → `./smackerel.sh test unit`
- [ ] Lint passes with zero warnings → `./smackerel.sh lint`
- [ ] Format check passes → `./smackerel.sh format --check`
- [ ] No TODO/FIXME/STUB markers in new files
- [ ] Consumer impact sweep: zero stale references after connector addition

---

## Scope 03: Hospitality Graph Nodes & Linker

**Status:** Not started
**Priority:** P0
**Dependencies:** Scope 2 (GH Connector — Implementation & Normalizer)

### Description

Create the `guests` and `properties` database tables via a new migration, implement repository layers for both (`GuestRepository`, `PropertyRepository`), build the `HospitalityLinker` that extends the existing `graph.Linker` to upsert guest/property nodes from artifact metadata and create hospitality-specific edge types (`STAYED_AT`, `REVIEWED`, `MANAGED_BY`, `ISSUE_AT`, `DURING_STAY`). Seed common hospitality topics on first sync. After this scope, every synced GH artifact automatically creates or updates guest/property graph nodes and establishes hospitality edges that enable cross-domain artifact linking.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GH-019 Guest node created from booking artifact
  Given a booking artifact with guest_email="sarah@example.com" and guest_name="Sarah"
  When the HospitalityLinker processes the artifact
  Then a guest node is upserted with email="sarah@example.com", name="Sarah", source="guesthost"
  And total_stays is incremented by 1
  And total_spend is increased by the booking revenue
  And last_seen is updated to the booking date

Scenario: SCN-GH-020 Returning guest detected and tagged
  Given a guest node exists with email="sarah@example.com" and total_stays=2
  When a new booking artifact for the same email is processed
  Then total_stays becomes 3
  And tags include "returning"
  And total_spend is updated

Scenario: SCN-GH-021 Property node created from booking artifact
  Given a booking artifact with property_id="prop-123" and property_name="Beach House"
  When the HospitalityLinker processes the artifact
  Then a property node is upserted with external_id="prop-123", source="guesthost", name="Beach House"
  And total_bookings is incremented by 1
  And total_revenue is increased by the booking revenue

Scenario: SCN-GH-022 STAYED_AT edge created between guest and property
  Given a booking artifact linking guest "sarah@example.com" to property "Beach House"
  When the HospitalityLinker processes the artifact
  Then a STAYED_AT edge is created from the guest node to the property node
  And the edge metadata includes check-in and check-out dates

Scenario: SCN-GH-023 REVIEWED edge created from review artifact
  Given a review artifact from guest "john@example.com" for property "Mountain Cabin"
  When the HospitalityLinker processes the artifact
  Then a REVIEWED edge is created from the guest node to the property node
  And property node avg_rating is updated

Scenario: SCN-GH-024 ISSUE_AT edge created from negative task/review
  Given a task artifact with category "maintenance" for property "Beach House"
  When the HospitalityLinker processes the artifact
  Then an ISSUE_AT edge is created from the task artifact to the property node
  And property node issue_count is incremented

Scenario: SCN-GH-025 DURING_STAY edge links temporal artifacts to bookings
  Given a booking artifact for "Sarah" at "Beach House" from Apr 5-8
  And a voice memo artifact captured on Apr 6 with property context
  When the HospitalityLinker processes both artifacts
  Then a DURING_STAY edge is created from the voice memo to the booking artifact
  Because the voice memo's CapturedAt falls within the booking's check-in/check-out window

Scenario: SCN-GH-026 Hospitality topic seeds created on first sync
  Given no hospitality topics exist in the database
  When the first GH connector sync completes
  Then 15 hospitality topics are seeded in "emerging" state
  And subsequent syncs do not re-seed

Scenario: SCN-GH-027 Guest node merges data from multiple sources
  Given a guest "sarah@example.com" has bookings from both guesthost and hospitable connectors
  When artifacts from both sources are processed
  Then a single guest node exists with source="both"
  And total_stays aggregates stays from all sources
  And total_spend aggregates spend from all sources

Scenario: SCN-GH-028 Property metrics update from review artifact
  Given a property "Beach House" with avg_rating=4.5 from 10 reviews
  When a new 5-star review is processed
  Then avg_rating is recalculated to include the new review
  And total_bookings is not incremented (review ≠ booking)
```

**Mapped Requirements:** FR-006, FR-007, FR-008, FR-011

### Implementation Plan

**Files created:**
- `internal/db/migrations/NNNN_add_guests_properties.up.sql` — `CREATE TABLE guests (...)`, `CREATE TABLE properties (...)`
- `internal/db/migrations/NNNN_add_guests_properties.down.sql` — `DROP TABLE guests`, `DROP TABLE properties`
- `internal/db/guest_repo.go` — `GuestRepository` interface and PostgreSQL implementation: `UpsertByEmail`, `FindByEmail`, `IncrementStay`, `UpdateSentiment`, `UpdateTags`, `FindAll`
- `internal/db/property_repo.go` — `PropertyRepository` interface and PostgreSQL implementation: `UpsertByExternalID`, `FindByExternalID`, `IncrementBookings`, `UpdateRating`, `UpdateTopics`, `UpdateIssueCount`, `FindAll`
- `internal/graph/hospitality_linker.go` — `HospitalityLinker` struct: `LinkArtifact()`, `upsertGuestNode()`, `upsertPropertyNode()`, `linkGuestToProperty()`, `linkDuringStay()`, `seedTopics()`

**Files modified:**
- `cmd/core/main.go` — Wire `HospitalityLinker` into the pipeline, call `seedTopics()` on first sync detection

**Components touched:**
- `HospitalityLinker.LinkArtifact()`: extracts guest_email + property_id from artifact metadata, upserts nodes, creates edges by type
- `upsertGuestNode()`: find-or-create by email, update metrics based on artifact type (booking→increment stays/spend, review→update rating, message→update sentiment)
- `upsertPropertyNode()`: find-or-create by external_id+source, update metrics
- `linkGuestToProperty()`: create STAYED_AT (booking), REVIEWED (review), or generic edge
- `linkDuringStay()`: query bookings overlapping artifact timestamp, create edge if match found
- `seedTopics()`: check if hospitality topics exist, create 15 seeds in "emerging" state if not
- Edge creation: uses existing `edges` table with new type strings

**Consumer Impact Sweep:** New tables are additive. Linker is a new pipeline hook — no existing linker code is modified. Topic seeding uses existing topic creation infrastructure.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-3-01 | TestGuestUpsertCreate | unit | `internal/db/guest_repo_test.go` | New email → guest created with initial metrics | SCN-GH-019 |
| T-3-02 | TestGuestUpsertUpdate | unit | `internal/db/guest_repo_test.go` | Existing email → metrics updated, not duplicated | SCN-GH-020 |
| T-3-03 | TestGuestReturningTag | unit | `internal/db/guest_repo_test.go` | total_stays > 1 → "returning" tag added | SCN-GH-020 |
| T-3-04 | TestPropertyUpsertCreate | unit | `internal/db/property_repo_test.go` | New external_id → property created | SCN-GH-021 |
| T-3-05 | TestPropertyUpsertUpdate | unit | `internal/db/property_repo_test.go` | Existing property → metrics updated | SCN-GH-021 |
| T-3-06 | TestLinkerCreatesStayedAtEdge | unit | `internal/graph/hospitality_linker_test.go` | Booking artifact → STAYED_AT edge created | SCN-GH-022 |
| T-3-07 | TestLinkerCreatesReviewedEdge | unit | `internal/graph/hospitality_linker_test.go` | Review artifact → REVIEWED edge created | SCN-GH-023 |
| T-3-08 | TestLinkerCreatesIssueAtEdge | unit | `internal/graph/hospitality_linker_test.go` | Task artifact → ISSUE_AT edge, issue_count incremented | SCN-GH-024 |
| T-3-09 | TestLinkerCreatesDuringStayEdge | unit | `internal/graph/hospitality_linker_test.go` | Artifact within stay window → DURING_STAY edge | SCN-GH-025 |
| T-3-10 | TestLinkerNoDuringStayOutsideWindow | unit | `internal/graph/hospitality_linker_test.go` | Artifact outside stay window → no DURING_STAY edge | SCN-GH-025 |
| T-3-11 | TestTopicSeedingFirstSync | unit | `internal/graph/hospitality_linker_test.go` | First sync → 15 topics seeded | SCN-GH-026 |
| T-3-12 | TestTopicSeedingIdempotent | unit | `internal/graph/hospitality_linker_test.go` | Second call → no duplicate topics | SCN-GH-026 |
| T-3-13 | TestGuestMultiSourceMerge | integration | `tests/integration/guesthost_graph_test.go` | Artifacts from both sources → single guest node with source="both" | SCN-GH-027 |
| T-3-14 | TestPropertyRatingUpdate | integration | `tests/integration/guesthost_graph_test.go` | Review artifact → avg_rating recalculated | SCN-GH-028 |
| T-3-15 | TestLinkerFullPipeline | integration | `tests/integration/guesthost_graph_test.go` | Booking + review + task → guest node + property node + all edge types | SCN-GH-019 thru SCN-GH-024 |
| T-3-16 | TestDuringStayTemporalLinking | integration | `tests/integration/guesthost_graph_test.go` | Booking + artifact within window → DURING_STAY edge in DB | SCN-GH-025 |
| T-3-17 | E2E: Graph nodes created from GH sync | e2e | `tests/e2e/guesthost_test.go` | Full sync → guest and property nodes in DB with correct metrics | SCN-GH-019, SCN-GH-021 |
| T-3-18 | E2E: Hospitality edges in graph | e2e | `tests/e2e/guesthost_test.go` | Full sync → STAYED_AT, REVIEWED, ISSUE_AT edges in DB | SCN-GH-022 thru SCN-GH-024 |

### Definition of Done

#### Core Items

- [ ] Migration creates `guests` table with email unique constraint, all columns per design
  > Verify: Migration applies cleanly, `./smackerel.sh check` passes
- [ ] Migration creates `properties` table with (external_id, source) unique constraint, all columns per design
  > Verify: Migration applies cleanly, `./smackerel.sh check` passes
- [ ] `GuestRepository` implements UpsertByEmail, FindByEmail, IncrementStay, UpdateSentiment, UpdateTags
  > Verify: T-3-01, T-3-02, T-3-03 PASS
- [ ] `PropertyRepository` implements UpsertByExternalID, FindByExternalID, IncrementBookings, UpdateRating, UpdateTopics, UpdateIssueCount
  > Verify: T-3-04, T-3-05 PASS
- [ ] `HospitalityLinker.LinkArtifact()` upserts guest node from booking/review/message artifacts
  > Verify: T-3-06, T-3-07 PASS
- [ ] `HospitalityLinker.LinkArtifact()` upserts property node from all property-tagged artifacts
  > Verify: T-3-06 PASS
- [ ] STAYED_AT edge created from booking artifacts (guest → property)
  > Verify: T-3-06 PASS
- [ ] REVIEWED edge created from review artifacts (guest → property)
  > Verify: T-3-07 PASS
- [ ] ISSUE_AT edge created from task/negative-review artifacts (artifact → property)
  > Verify: T-3-08 PASS
- [ ] DURING_STAY edge created for artifacts within a booking's check-in/check-out window
  > Verify: T-3-09, T-3-10, T-3-16 PASS
- [ ] Guest node tags include "returning" when total_stays > 1
  > Verify: T-3-03 PASS
- [ ] Guest node source set to "both" when data comes from GH and Hospitable
  > Verify: T-3-13 PASS
- [ ] 15 hospitality topics seeded on first sync, idempotent on subsequent syncs
  > Verify: T-3-11, T-3-12 PASS
- [ ] E2E: full sync creates graph nodes and edges end-to-end
  > Verify: T-3-17, T-3-18 PASS
- [ ] Regression: Scope 1 + Scope 2 tests still pass
  > Verify: T-1-*, T-2-* all PASS

#### Build Quality Gate

- [ ] All unit tests pass → `./smackerel.sh test unit`
- [ ] All integration tests pass → `./smackerel.sh test integration`
- [ ] Lint passes with zero warnings → `./smackerel.sh lint`
- [ ] Format check passes → `./smackerel.sh format --check`
- [ ] No TODO/FIXME/STUB markers in new files

---

## Scope 04: Hospitality Digest

**Status:** Not started
**Priority:** P1
**Dependencies:** Scope 3 (Hospitality Graph Nodes & Linker)

### Description

Extend the existing digest generator (`internal/digest/generator.go`) with hospitality-specific sections. Build the `HospitalityDigestContext` assembly that queries guest and property graph nodes, artifact database, and booking data to produce: today's arrivals (with returning-guest detection), today's departures, pending tasks across properties, revenue snapshot (24h/7d/30d by channel and property), guest alerts (VIP arrivals, complaint history), and property alerts (rising issue topics). Create the hospitality prompt template for the ML sidecar. Wire active-connector detection so hospitality sections are only included when GH or Hospitable connectors are active. After this scope, hosts receive domain-specific daily briefings.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GH-029 Digest includes today's arrivals
  Given a booking with check-in date = today for guest "Sarah" at "Beach House"
  And Sarah is a returning guest (total_stays=3)
  When the digest generator runs
  Then the hospitality context includes Sarah in TodayArrivals
  And IsReturning is true with PastStays=2
  And the generated digest mentions today's arrival with returning-guest context

Scenario: SCN-GH-030 Digest includes today's departures
  Given a booking with check-out date = today for guest "John" at "Mountain Cabin"
  When the digest generator runs
  Then the hospitality context includes John in TodayDepartures
  And the generated digest mentions the departure

Scenario: SCN-GH-031 Digest includes pending tasks
  Given 3 open tasks across 2 properties
  When the digest generator runs
  Then PendingTasks contains all 3 tasks with property names
  And the generated digest lists pending tasks

Scenario: SCN-GH-032 Digest includes revenue snapshot
  Given bookings totaling $1,200 (direct) and $800 (Airbnb) in the last 7 days
  When the digest generator runs
  Then RevenueSnapshot.Last7d equals $2,000
  And BySource contains "direct": $1,200 and "airbnb": $800
  And the generated digest shows revenue breakdown by channel

Scenario: SCN-GH-033 Digest includes guest alerts for returning guests arriving
  Given a returning guest (3 stays, previous complaint about WiFi) is arriving today
  When the digest generator runs
  Then GuestAlerts includes an entry for this guest
  And the alert notes returning status and previous complaint
  And the generated digest highlights the guest intelligence

Scenario: SCN-GH-034 Digest includes property alerts for rising issues
  Given property "Beach House" has topic "cleaning-quality" in "active" state with momentum 8.2
  When the digest generator runs
  Then PropertyAlerts includes "Beach House" with "cleaning-quality" trending up
  And the generated digest mentions the operational concern

Scenario: SCN-GH-035 Empty day omits hospitality sections
  Given no arrivals, no departures, no pending tasks, no new reviews today
  When the digest generator runs
  Then TodayArrivals, TodayDepartures, PendingTasks sections are omitted (not shown as empty)
  And RevenueSnapshot still shows trailing windows if bookings exist in the period
  And the digest is not blank — standard knowledge sections (hot topics, action items) are preserved

Scenario: SCN-GH-036 No hospitality connectors active generates standard digest
  Given neither GH connector nor Hospitable connector is enabled
  When the digest generator runs
  Then no hospitality sections are assembled
  And the standard digest (action items, overnight artifacts, hot topics) is generated as before
```

**Mapped Requirements:** FR-009

### Implementation Plan

**Files created:**
- `internal/digest/hospitality.go` — `HospitalityDigestContext` struct, `AssembleHospitalityContext()`, `queryArrivals()`, `queryDepartures()`, `queryPendingTasks()`, `computeRevenueSnapshot()`, `buildGuestAlerts()`, `buildPropertyAlerts()`

**Files modified:**
- `internal/digest/generator.go` — Extend `Generate()` to detect active hospitality connectors and include hospitality context in digest payload
- `cmd/core/main.go` — Wire hospitality digest dependencies (guest/property repos)

**Components touched:**
- `AssembleHospitalityContext()`: queries booking artifacts for today's arrivals/departures, open task artifacts, revenue aggregation from financial/booking artifacts, guest nodes for alerts, property nodes for topic alerts
- `queryArrivals()`: SELECT booking artifacts WHERE checkin_date = today, JOIN guest node for returning-guest detection
- `queryDepartures()`: SELECT booking artifacts WHERE checkout_date = today
- `queryPendingTasks()`: SELECT task artifacts WHERE status != "completed"
- `computeRevenueSnapshot()`: Aggregate revenue from booking artifacts in 24h/7d/30d windows, group by source and property
- `buildGuestAlerts()`: Check arriving guests for returning status, complaint history, VIP tags
- `buildPropertyAlerts()`: Check property nodes for active topics with rising momentum
- Active connector detection: check connector registry for healthy GH or Hospitable connector
- Digest prompt template: new hospitality template string with sections for operations, guest intelligence, revenue, tasks, topics

**Consumer Impact Sweep:** Extends existing digest generator — no existing API or delivery channels changed. Hospitality sections are additive when connectors are active. Standard digest is preserved when no hospitality connectors are active.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-4-01 | TestQueryArrivalsToday | unit | `internal/digest/hospitality_test.go` | Booking with checkin=today → included in arrivals | SCN-GH-029 |
| T-4-02 | TestQueryArrivalsReturningGuest | unit | `internal/digest/hospitality_test.go` | Guest with total_stays>1 → IsReturning=true, PastStays correct | SCN-GH-029 |
| T-4-03 | TestQueryDeparturesToday | unit | `internal/digest/hospitality_test.go` | Booking with checkout=today → included in departures | SCN-GH-030 |
| T-4-04 | TestQueryPendingTasks | unit | `internal/digest/hospitality_test.go` | Open tasks → included; completed tasks → excluded | SCN-GH-031 |
| T-4-05 | TestRevenueSnapshotBySource | unit | `internal/digest/hospitality_test.go` | Revenue aggregated correctly by channel and time window | SCN-GH-032 |
| T-4-06 | TestRevenueSnapshotByProperty | unit | `internal/digest/hospitality_test.go` | Revenue aggregated correctly by property | SCN-GH-032 |
| T-4-07 | TestGuestAlertsReturningWithComplaint | unit | `internal/digest/hospitality_test.go` | Returning guest with prior complaint → alert generated | SCN-GH-033 |
| T-4-08 | TestPropertyAlertsRisingTopic | unit | `internal/digest/hospitality_test.go` | Active topic with high momentum → property alert | SCN-GH-034 |
| T-4-09 | TestEmptyDayOmitsSections | unit | `internal/digest/hospitality_test.go` | No arrivals/departures → sections omitted, not empty | SCN-GH-035 |
| T-4-10 | TestNoConnectorsActiveSkipsHospitality | unit | `internal/digest/hospitality_test.go` | No active hospitality connectors → standard digest only | SCN-GH-036 |
| T-4-11 | TestAssembleHospitalityContextFull | integration | `tests/integration/guesthost_digest_test.go` | Seeded bookings/tasks/reviews → full HospitalityDigestContext assembled | SCN-GH-029 thru SCN-GH-034 |
| T-4-12 | TestDigestGeneratorWithHospitality | integration | `tests/integration/guesthost_digest_test.go` | Active GH connector + seeded data → digest includes hospitality sections | SCN-GH-029 |
| T-4-13 | TestDigestGeneratorWithoutHospitality | integration | `tests/integration/guesthost_digest_test.go` | No active connectors → standard digest, no hospitality sections | SCN-GH-036 |
| T-4-14 | E2E: Hospitality digest end-to-end | e2e | `tests/e2e/guesthost_test.go` | Full sync + digest run → hospitality sections in output | SCN-GH-029 thru SCN-GH-035 |

### Definition of Done

#### Core Items

- [ ] `internal/digest/hospitality.go` created with `HospitalityDigestContext`, `AssembleHospitalityContext()`, and all query functions
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] Today's arrivals assembled with returning-guest detection
  > Verify: T-4-01, T-4-02 PASS
- [ ] Today's departures assembled correctly
  > Verify: T-4-03 PASS
- [ ] Pending tasks queried across all properties
  > Verify: T-4-04 PASS
- [ ] Revenue snapshot computed for 24h/7d/30d windows, broken down by channel and property
  > Verify: T-4-05, T-4-06 PASS
- [ ] Guest alerts generated for returning guests with complaint history
  > Verify: T-4-07 PASS
- [ ] Property alerts generated for properties with rising issue topics
  > Verify: T-4-08 PASS
- [ ] Empty day omits hospitality sections (not shown as empty)
  > Verify: T-4-09 PASS
- [ ] No hospitality connectors active → standard digest preserved
  > Verify: T-4-10 PASS
- [ ] Digest generator extended to detect active hospitality connectors and include context
  > Verify: T-4-12, T-4-13 PASS
- [ ] Hospitality prompt template created for ML sidecar
  > Verify: Template string present in code, used in digest generation path
- [ ] E2E: full pipeline from sync → digest includes hospitality sections
  > Verify: T-4-14 PASS
- [ ] Regression: Scope 1 + Scope 2 + Scope 3 tests still pass
  > Verify: T-1-*, T-2-*, T-3-* all PASS

#### Build Quality Gate

- [ ] All unit tests pass → `./smackerel.sh test unit`
- [ ] All integration tests pass → `./smackerel.sh test integration`
- [ ] Lint passes with zero warnings → `./smackerel.sh lint`
- [ ] Format check passes → `./smackerel.sh format --check`
- [ ] No TODO/FIXME/STUB markers in new files

---

## Scope 05: Context Enrichment API

**Status:** Not started
**Priority:** P1
**Dependencies:** Scope 3 (Hospitality Graph Nodes & Linker)

### Description

Add the `POST /api/context-for` endpoint to the API router. Implement the `ContextHandler` that resolves guest (by email), property (by external ID), and booking (by booking ID) entities, assembling rich context responses including: stay history, sentiment trajectory, active topics, alerts, and rule-based communication hints. Implement API key authentication for external callers (GuestHost). Communication hints are entirely rule-based (deterministic, no LLM) for speed and predictability. Add `intelligence.hospitality` and `context_api` config sections. After this scope, GuestHost or any external system can query Smackerel for AI-ready guest/property/booking intelligence.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GH-037 Guest context returns full history and hints
  Given a guest "sarah@example.com" with 3 stays, $2,840 total spend, and a past WiFi complaint
  When POST /api/context-for with entity_type="guest", entity_id="sarah@example.com", include=["history","sentiment","topics","alerts","communication_hints"]
  Then the response includes: totalStays=3, totalSpend=2840, property breakdown, channel breakdown
  And sentiment trajectory with recent message sentiment scores
  And active topics for this guest
  And alerts for any overdue commitments
  And communication hints including "Returning guest (3rd stay) — acknowledge loyalty"

Scenario: SCN-GH-038 Property context returns performance and operational hints
  Given property "Beach House" with 47 bookings, $38,500 revenue, and "cleaning-quality" topic active
  When POST /api/context-for with entity_type="property", entity_id="gh_property_uuid"
  Then the response includes: totalBookings=47, totalRevenue=38500, avgRating, revenueByChannel
  And activeTopics with "cleaning-quality" and its momentum score
  And recentIssues from task and review artifacts
  And operationalHints including "Cleaning complaints trending up"

Scenario: SCN-GH-039 Booking context returns booking details with guest context
  Given a booking for "Sarah" at "Beach House" from Apr 15-18
  When POST /api/context-for with entity_type="booking", entity_id="booking_uuid"
  Then the response includes: booking details (dates, property, guest, source, revenue)
  And linked guest context (abbreviated — stay history, tags)
  And in-stay artifacts (messages, tasks created during the booking period)

Scenario: SCN-GH-040 Unknown guest returns 404
  Given no guest node exists for "unknown@example.com"
  When POST /api/context-for with entity_type="guest", entity_id="unknown@example.com"
  Then the response is HTTP 404 with {"error": "guest_not_found"}

Scenario: SCN-GH-041 Unknown property returns 404
  Given no property node exists for "nonexistent_property_id"
  When POST /api/context-for with entity_type="property", entity_id="nonexistent_property_id"
  Then the response is HTTP 404 with {"error": "property_not_found"}

Scenario: SCN-GH-042 Communication hints are rule-based and deterministic
  Given a returning guest who books direct 67% of the time and previously requested early check-in
  When communication hints are generated
  Then hints include "Returning guest (N stays) — acknowledge loyalty"
  And hints include "Previously requested early check-in — proactively offer"
  And hints include "Books direct 67% of the time — high-value direct guest"
  And no LLM call is made during hint generation

Scenario: SCN-GH-043 API key authentication required
  Given the context API is configured with api_key="smk_test_key"
  When a request is made without Authorization header
  Then the response is HTTP 401 with {"error": "unauthorized"}
  When a request is made with an invalid API key
  Then the response is HTTP 401 with {"error": "unauthorized"}
  When a request is made with the correct API key
  Then the request is processed normally

Scenario: SCN-GH-044 Include parameter controls response sections
  Given a valid guest context request
  When include=["history"] (only history requested)
  Then the response includes the history section
  And sentiment, topics, alerts, and communication_hints sections are omitted

Scenario: SCN-GH-045 Invalid entity_type returns 400
  Given a context request with entity_type="unknown"
  When the request is processed
  Then the response is HTTP 400 with {"error": "invalid_entity_type", "valid_types": ["guest","property","booking"]}

Scenario: SCN-GH-046 Context API disabled returns 404 for all requests
  Given context_api.enabled is false in config
  When any POST /api/context-for request is made
  Then the response is HTTP 404 (endpoint not registered)
```

**Mapped Requirements:** FR-010, FR-012, NFR-001 (< 500ms p95), NFR-004

### Implementation Plan

**Files created:**
- `internal/api/context.go` — `ContextHandler` struct, `HandleContextFor()`, `buildGuestContext()`, `buildPropertyContext()`, `buildBookingContext()`, `computeSentimentTrajectory()`, `generateCommunicationHints()`, `checkAlerts()`
- `internal/intelligence/hospitality.go` — `AlertEngine` struct, `CheckAlerts()`, `CheckCommitments()` (rule-based alert generation)

**Files modified:**
- `internal/api/` (router file) — Register `POST /api/context-for` route with API key middleware
- `config/smackerel.yaml` — Add `intelligence.hospitality` and `context_api` sections
- `cmd/core/main.go` — Wire `ContextHandler` with repos and intelligence engine

**Components touched:**
- `ContextHandler.HandleContextFor()`: parse request, validate entity_type, dispatch to build*Context methods
- `buildGuestContext()`: query guest node by email, query artifacts by guest_email metadata, compute sentiment trajectory, check alerts, generate hints
- `buildPropertyContext()`: query property node by external_id, query active topics, recent issues from artifacts, generate operational hints
- `buildBookingContext()`: query booking artifact by ID, resolve linked guest, query in-stay artifacts
- `computeSentimentTrajectory()`: average sentiment scores from recent message/review artifacts with time weighting
- `generateCommunicationHints()`: rule-based logic per design (returning guest, early check-in preference, direct booking %, overdue commitments)
- `checkAlerts()`: query for overdue commitments (promises in messages not yet fulfilled), rising complaint topics
- API key middleware: compare `Authorization: Bearer {key}` against configured `context_api.api_key`
- `include` parameter filtering: only populate requested sections

**Consumer Impact Sweep:** New API route — no existing routes modified. Config sections are additive. No existing behavior changed.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-5-01 | TestBuildGuestContextFull | unit | `internal/api/context_test.go` | All sections populated for guest with history | SCN-GH-037 |
| T-5-02 | TestBuildGuestContextHistory | unit | `internal/api/context_test.go` | Stay count, spend, property breakdown correct | SCN-GH-037 |
| T-5-03 | TestBuildPropertyContextFull | unit | `internal/api/context_test.go` | Performance metrics, active topics, issues, hints populated | SCN-GH-038 |
| T-5-04 | TestBuildBookingContext | unit | `internal/api/context_test.go` | Booking details, linked guest, in-stay artifacts | SCN-GH-039 |
| T-5-05 | TestGuestNotFound | unit | `internal/api/context_test.go` | Unknown email → 404 with guest_not_found | SCN-GH-040 |
| T-5-06 | TestPropertyNotFound | unit | `internal/api/context_test.go` | Unknown property → 404 with property_not_found | SCN-GH-041 |
| T-5-07 | TestCommunicationHintsReturning | unit | `internal/api/context_test.go` | Returning guest → "acknowledge loyalty" hint | SCN-GH-042 |
| T-5-08 | TestCommunicationHintsDirectBooker | unit | `internal/api/context_test.go` | >50% direct → "high-value direct guest" hint | SCN-GH-042 |
| T-5-09 | TestCommunicationHintsEarlyCheckin | unit | `internal/api/context_test.go` | Early-checkin topic → proactive offer hint | SCN-GH-042 |
| T-5-10 | TestAPIKeyAuthRequired | unit | `internal/api/context_test.go` | No header → 401; wrong key → 401; correct key → 200 | SCN-GH-043 |
| T-5-11 | TestIncludeParameterFilters | unit | `internal/api/context_test.go` | include=["history"] → only history section in response | SCN-GH-044 |
| T-5-12 | TestInvalidEntityType | unit | `internal/api/context_test.go` | entity_type="unknown" → 400 with valid_types list | SCN-GH-045 |
| T-5-13 | TestContextAPIFullGuestFlow | integration | `tests/integration/guesthost_context_test.go` | Seeded guest + artifacts → full context response correct | SCN-GH-037 |
| T-5-14 | TestContextAPIFullPropertyFlow | integration | `tests/integration/guesthost_context_test.go` | Seeded property + artifacts → full context response correct | SCN-GH-038 |
| T-5-15 | TestSentimentTrajectoryComputation | integration | `tests/integration/guesthost_context_test.go` | Multiple message artifacts → correct sentiment trajectory | SCN-GH-037 |
| T-5-16 | E2E: Guest context from synced data | e2e | `tests/e2e/guesthost_test.go` | Full sync → POST /api/context-for guest → correct response | SCN-GH-037 |
| T-5-17 | E2E: Property context from synced data | e2e | `tests/e2e/guesthost_test.go` | Full sync → POST /api/context-for property → correct response | SCN-GH-038 |

### Definition of Done

#### Core Items

- [ ] `internal/api/context.go` created with `ContextHandler`, `HandleContextFor()`, and all build*Context methods
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] `internal/intelligence/hospitality.go` created with `AlertEngine`, `CheckAlerts()`
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] `POST /api/context-for` route registered with API key middleware
  > Verify: Route registered in router
- [ ] Guest context returns: profile, history (stays, spend, properties, channels), sentiment, topics, alerts, communication hints
  > Verify: T-5-01, T-5-02 PASS
- [ ] Property context returns: performance (bookings, revenue, rating, revenue by channel), active topics, recent issues, operational hints
  > Verify: T-5-03 PASS
- [ ] Booking context returns: booking details, linked guest context, in-stay artifacts
  > Verify: T-5-04 PASS
- [ ] Unknown guest → HTTP 404 `{"error": "guest_not_found"}`
  > Verify: T-5-05 PASS
- [ ] Unknown property → HTTP 404 `{"error": "property_not_found"}`
  > Verify: T-5-06 PASS
- [ ] Communication hints are rule-based: returning-guest, early-checkin, direct-booking-%, overdue-commitments
  > Verify: T-5-07, T-5-08, T-5-09 PASS
- [ ] API key authentication enforced — missing/invalid key → 401
  > Verify: T-5-10 PASS
- [ ] `include` parameter controls response sections — omitted sections excluded
  > Verify: T-5-11 PASS
- [ ] Invalid entity_type → HTTP 400 with valid_types list
  > Verify: T-5-12 PASS
- [ ] Context API response time < 500ms p95 (NFR-001)
  > Verify: Integration test latency assertion
- [ ] `config/smackerel.yaml` has `intelligence.hospitality` and `context_api` sections
  > Verify: Config sections present
- [ ] E2E: full pipeline from sync → context API returns correct data
  > Verify: T-5-16, T-5-17 PASS
- [ ] Regression: Scope 1 + Scope 2 + Scope 3 tests still pass
  > Verify: T-1-*, T-2-*, T-3-* all PASS

#### Build Quality Gate

- [ ] All unit tests pass → `./smackerel.sh test unit`
- [ ] All integration tests pass → `./smackerel.sh test integration`
- [ ] All e2e tests pass → `./smackerel.sh test e2e`
- [ ] Lint passes with zero warnings → `./smackerel.sh lint`
- [ ] Format check passes → `./smackerel.sh format --check`
- [ ] No TODO/FIXME/STUB markers in new files
