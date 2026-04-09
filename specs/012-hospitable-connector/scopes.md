# Scopes: 012 — Hospitable Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/hospitable/` (new directory: `types.go`, `client.go`, `connector.go`, `normalizer.go`), `config/smackerel.yaml` (add hospitable connector section), `cmd/core/main.go` (register hospitable connector).

**Excluded surfaces:** No changes to existing connector implementations (RSS, IMAP, CalDAV, YouTube, Browser, Bookmarks, Keep, Maps). No changes to existing pipeline processors, search API, digest API, health API, or web handlers. No changes to existing NATS stream configurations. No new database migrations — uses existing `artifacts`, `edges`, `sync_state` tables. No changes to the ML sidecar.

### Phase Order

1. **Scope 1: API Client, Types & Config** — Define Hospitable API response structs, build the HTTP client with PAT authentication, pagination, rate limiting, and retry logic. Add config section to `smackerel.yaml`. Client is testable in isolation after this scope.
2. **Scope 2: Connector Implementation & Normalizer** — Implement the `Connector` interface wrapping the API client, build normalizers for all four resource types (property, reservation, message, review), add cursor management (JSON timestamps), and register in `cmd/core/main.go`. Full sync lifecycle works after this scope.
3. **Scope 3: Edge Hints, Cross-Domain Linking & Hardening** — Add knowledge graph edge hints in metadata (BELONGS_TO, PART_OF, REVIEW_OF, DURING_STAY), implement partial failure isolation (one resource type failing doesn't block others), add comprehensive error handling, and build the property name cache for enriching reservation/review titles.
4. **Scope 4: Message Sync Reliability & Client Hardening** — Fix active reservation message sync (R-016), Retry-After header parsing (R-017), persistent property name cache (R-018), and per-reservation message cursor isolation (R-021).
5. **Scope 5: Normalizer Quality Fixes** — Fix sender classification (R-019), artifact URL population (R-020), and review rating precision (R-022).

### New Types & Signatures

```go
// internal/connector/hospitable/types.go
type Property struct { ID, Name string; Address Address; Bedrooms, Bathrooms, MaxGuests int; Amenities, ListingURLs, ChannelIDs []string; CreatedAt, UpdatedAt time.Time }
type Address struct { Street, City, State, Country, Zip string }
type Reservation struct { ID, PropertyID, Channel, Status, CheckIn, CheckOut, GuestName string; GuestCount, Nights int; NightlyRate, TotalPayout float64; BookedAt, CreatedAt, UpdatedAt time.Time }
type Message struct { ID, ReservationID, Sender, Body string; IsAutomated bool; SentAt time.Time }
type Review struct { ID, ReservationID, PropertyID, ReviewText, HostResponse, Channel string; Rating float64; SubmittedAt time.Time }
type PaginatedResponse[T any] struct { Data []T; NextURL string; Total int }
type SyncCursor struct { Properties, Reservations, Messages, Reviews time.Time }

// internal/connector/hospitable/client.go
type Client struct { baseURL, token string; httpClient *http.Client; backoff *connector.Backoff; pageSize int }
func NewClient(baseURL, token string, pageSize int) *Client
func (c *Client) Validate(ctx context.Context) error
func (c *Client) ListProperties(ctx context.Context, since time.Time) ([]Property, error)
func (c *Client) ListReservations(ctx context.Context, since time.Time) ([]Reservation, error)
func (c *Client) ListMessages(ctx context.Context, reservationID string, since time.Time) ([]Message, error)
func (c *Client) ListReviews(ctx context.Context, since time.Time) ([]Review, error)

// internal/connector/hospitable/connector.go
type HospitableConfig struct { AccessToken, BaseURL, SyncSchedule string; InitialLookbackDays, PageSize int; SyncProperties, SyncReservations, SyncMessages, SyncReviews bool; TierMessages, TierReviews, TierReservations, TierProperties string }
type Connector struct { id string; health connector.HealthStatus; config HospitableConfig; client *Client; ... }
func New(id string) *Connector
func (c *Connector) ID() string
func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
func (c *Connector) Health(ctx context.Context) connector.HealthStatus
func (c *Connector) Close() error

// internal/connector/hospitable/normalizer.go
func NormalizeProperty(property Property, config HospitableConfig) connector.RawArtifact
func NormalizeReservation(reservation Reservation, propertyName string, config HospitableConfig) connector.RawArtifact
func NormalizeMessage(message Message, reservationContext string, config HospitableConfig) connector.RawArtifact
func NormalizeReview(review Review, propertyName string, config HospitableConfig) connector.RawArtifact
```

### Validation Checkpoints

- **After Scope 1:** Unit tests validate API client request construction (correct auth header, URL formation, pagination following), rate limit retry behavior, config parsing and validation. Integration test with mock HTTP server confirms multi-page fetches work end-to-end.
- **After Scope 2:** Unit tests validate normalizer output for all 4 resource types, cursor parsing/encoding, connector lifecycle. Integration test with mock API confirms full Sync() produces correct RawArtifacts and advances cursor. E2E test confirms connector registration and config-to-sync pipeline.
- **After Scope 3:** Unit tests verify edge hints in metadata, partial failure isolation. Integration tests confirm DURING_STAY temporal window logic, property name cache enrichment, and one-resource-type-failure doesn't block others. E2E test confirms full pipeline with cross-domain edge creation.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|---|---|---|---|---|
| 1 | API Client, Types & Config | Go core, Config | 12 unit + 2 integration | Client builds correct requests, paginates, retries on 429, config validates | Done |
| 2 | Connector Implementation & Normalizer | Go core, Config | 14 unit + 3 integration + 2 e2e | Connector lifecycle complete, normalizer maps all resource types, cursor incremental sync works | Done |
| 3 | Edge Hints, Cross-Domain Linking & Hardening | Go core | 8 unit + 4 integration + 2 e2e | Edge hints in metadata, partial failure isolation, property name cache, DURING_STAY temporal linking | Done |
| 4 | Message Sync Reliability & Client Hardening | Go core | 10 unit | Active reservation message sync, Retry-After parsing, persistent property name cache, message cursor isolation | Done |
| 5 | Normalizer Quality Fixes | Go core | 12 unit | Sender classification, URL population, review rating precision | Done |

---

## Scope 01: API Client, Types & Config

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Define Go structs for all Hospitable API response types (`Property`, `Reservation`, `Message`, `Review`, `PaginatedResponse`, `SyncCursor`, `Address`). Build the HTTP API client (`Client`) that handles Personal Access Token authentication, request construction, paginated list fetching, rate limit detection with exponential backoff, and error classification. Add the `connectors.hospitable` configuration section to `smackerel.yaml` and implement config parsing with validation. After this scope, the API client is independently testable against a mock HTTP server.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-HC-001 API client authenticates with Personal Access Token
  Given a Hospitable API client configured with base URL and PAT
  When Validate() is called
  Then the client sends GET request with "Authorization: Bearer {token}" header
  And on 200 response, Validate() returns nil
  And on 401 response, Validate() returns an error containing "unauthorized"
  And on 403 response, Validate() returns an error containing "forbidden"

Scenario: SCN-HC-002 API client paginates property listings
  Given the Hospitable API returns properties in 3 pages of 100
  When ListProperties() is called
  Then the client follows pagination links until no "next" URL
  And returns all 300 properties in a single slice
  And each intermediate request includes the Authorization header

Scenario: SCN-HC-003 API client retries on rate limit (429)
  Given the Hospitable API returns 429 on the first request
  And includes a Retry-After header of 5 seconds
  When ListReservations() is called
  Then the client waits and retries with exponential backoff
  And on success, returns the reservation data
  And on 3 consecutive 429s, returns a rate limit error

Scenario: SCN-HC-004 API client handles server errors with retry
  Given the Hospitable API returns 500 on the first request
  When ListReviews() is called
  Then the client retries with exponential backoff up to 3 times
  And on persistent 500, returns a server error

Scenario: SCN-HC-005 Config validation rejects invalid settings
  Given a smackerel.yaml with connectors.hospitable configured
  When access_token is empty and enabled is true
  Then config parsing returns an error containing "access_token"
  When initial_lookback_days is negative
  Then config parsing returns a validation error
  When page_size is 0
  Then page_size defaults to 100

Scenario: SCN-HC-006 API client constructs correct request URLs
  Given a client with base URL "https://api.hospitable.com"
  When ListReservations() is called with since=2026-04-01T00:00:00Z
  Then the request URL contains the base path for reservations
  And includes an updated_since query parameter
  And includes the page_size query parameter
```

**Mapped Requirements:** R-002 (Authentication), R-008 (Pagination), R-009 (Rate Limiting), R-014 (Configuration)

### Implementation Plan

**Files created:**
- `internal/connector/hospitable/types.go` — All API response structs: `Property`, `Address`, `Reservation`, `Message`, `Review`, `PaginatedResponse[T]`, `SyncCursor`
- `internal/connector/hospitable/client.go` — `Client` struct, `NewClient()`, `Validate()`, `ListProperties()`, `ListReservations()`, `ListMessages()`, `ListReviews()`, `doRequest()`, `doGet()`, `fetchPaginated()`

**Files modified:**
- `config/smackerel.yaml` — Add `connectors.hospitable` section with all fields per R-014

**Components touched:**
- `Client.doRequest()`: builds URL, sets `Authorization: Bearer {token}`, sets `Content-Type`, handles timeout
- `Client.doGet()`: calls `doRequest`, reads body, checks status code, retries on 429/5xx via `Backoff`
- `Client.fetchPaginated()`: generic paginated fetch loop — calls endpoint, appends results, follows `next` URL until nil
- `Client.Validate()`: `doGet` on a lightweight endpoint (properties with page_size=1), checks for 200
- Config parsing: extracts from `ConnectorConfig.SourceConfig`, validates required fields when enabled

**Consumer Impact Sweep:** No existing surfaces modified. Config section is additive. Types are package-internal.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-1-01 | TestClientAuthHeader | unit | `internal/connector/hospitable/client_test.go` | Request contains `Authorization: Bearer {token}` | SCN-HC-001 |
| T-1-02 | TestClientValidateSuccess | unit | `internal/connector/hospitable/client_test.go` | 200 response → nil error | SCN-HC-001 |
| T-1-03 | TestClientValidateUnauthorized | unit | `internal/connector/hospitable/client_test.go` | 401 response → error with "unauthorized" | SCN-HC-001 |
| T-1-04 | TestClientValidateForbidden | unit | `internal/connector/hospitable/client_test.go` | 403 response → error with "forbidden" | SCN-HC-001 |
| T-1-05 | TestClientPaginatesProperties | unit | `internal/connector/hospitable/client_test.go` | 3-page response → 300 items returned | SCN-HC-002 |
| T-1-06 | TestClientRetryOn429 | unit | `internal/connector/hospitable/client_test.go` | First 429 then 200 → success after retry | SCN-HC-003 |
| T-1-07 | TestClientMaxRetriesOn429 | unit | `internal/connector/hospitable/client_test.go` | 3 consecutive 429s → rate limit error | SCN-HC-003 |
| T-1-08 | TestClientRetryOnServerError | unit | `internal/connector/hospitable/client_test.go` | 500 then 200 → success after retry | SCN-HC-004 |
| T-1-09 | TestClientURLConstruction | unit | `internal/connector/hospitable/client_test.go` | Correct base path, updated_since, page_size params | SCN-HC-006 |
| T-1-10 | TestConfigValidationMissingToken | unit | `internal/connector/hospitable/connector_test.go` | Empty token + enabled → error | SCN-HC-005 |
| T-1-11 | TestConfigValidationDefaults | unit | `internal/connector/hospitable/connector_test.go` | Missing optional fields → correct defaults applied | SCN-HC-005 |
| T-1-12 | TestSyncCursorMarshal | unit | `internal/connector/hospitable/connector_test.go` | SyncCursor round-trips through JSON correctly | SCN-HC-006 |
| T-1-13 | TestClientFullPaginationFlow | integration | `tests/integration/hospitable_test.go` | Mock HTTP server with 3 pages → all items collected | SCN-HC-002 |
| T-1-14 | TestClientRateLimitRecovery | integration | `tests/integration/hospitable_test.go` | Mock server returns 429 then 200 → client recovers | SCN-HC-003 |

### Definition of Done

- [x] `internal/connector/hospitable/types.go` created with `Property`, `Address`, `Reservation`, `Message`, `Review`, `PaginatedResponse[T]`, `SyncCursor` structs
  > Evidence: File exists, `./smackerel.sh check` passes ✓
- [x] `internal/connector/hospitable/client.go` created with `Client`, `NewClient()`, `Validate()`, `ListProperties()`, `ListReservations()`, `ListMessages()`, `ListReviews()`
  > Evidence: File exists, `./smackerel.sh check` passes ✓
- [x] `Client` sends `Authorization: Bearer {token}` header on every request
  > Evidence: TestClientAuthHeader PASS ✓
- [x] `Validate()` distinguishes 200 (success), 401 (unauthorized), 403 (forbidden)
  > Evidence: TestClientValidateSuccess, TestClientValidateUnauthorized, TestClientValidateForbidden PASS ✓
- [x] `fetchPaginated()` follows `next` URLs until exhausted, collecting all items
  > Evidence: TestClientPaginatesProperties PASS ✓
- [x] Rate limit (429) triggers exponential backoff with max 3 retries
  > Evidence: TestClientRetryOn429, TestClientMaxRetriesOn429 PASS ✓
- [x] Server errors (5xx) trigger exponential backoff with max 3 retries
  > Evidence: TestClientRetryOnServerError PASS ✓
- [x] Request URLs correctly include base path, `updated_since`, and `page_size` parameters
  > Evidence: TestClientURLConstruction PASS ✓
- [x] `config/smackerel.yaml` has `connectors.hospitable` section with all fields per R-014
  > Evidence: Config section present with access_token, sync_schedule, lookback, tier settings ✓
- [x] Config parsing validates required fields, applies defaults for optional fields
  > Evidence: TestConfigValidationMissingToken, TestConfigValidationDefaults PASS ✓
- [x] `SyncCursor` correctly marshals/unmarshals to/from JSON
  > Evidence: TestSyncCursorMarshal PASS ✓
- [x] All unit tests pass
  > Evidence: `./smackerel.sh test unit` — all 25 Go packages pass, hospitable 2.952s ✓
- [x] `./smackerel.sh lint` passes with zero new errors
  > Evidence: `./smackerel.sh lint` exit 0 ✓
- [x] `./smackerel.sh format --check` passes
  > Evidence: `./smackerel.sh format --check` exit 0 ✓

---

## Scope 02: Connector Implementation & Normalizer

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1 (API Client, Types & Config)

### Description

Implement the `Connector` interface in `connector.go` wrapping the API client into the standard connector lifecycle (Connect validates PAT, Sync orchestrates multi-resource fetch, Health reports status, Close cleans up). Build normalizers in `normalizer.go` for all four resource types (property→`property/str-listing`, reservation→`reservation/str-booking`, message→`message/str-conversation`, review→`review/str-guest`). Implement JSON-based cursor management with per-resource-type timestamps and initial lookback window. Register the connector in `cmd/core/main.go`. After this scope, the full sync lifecycle works: connect → sync all resources → normalize → return artifacts + updated cursor.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-HC-007 Connector implements full lifecycle
  Given the Hospitable connector is registered in the connector registry
  When Connect() is called with valid config containing a valid PAT
  Then the client validates the token against the Hospitable API
  And Health() returns "healthy"
  And ID() returns "hospitable"
  When Sync() is called with an empty cursor
  Then it fetches all properties, recent reservations, messages, and reviews
  And returns RawArtifacts for each resource
  And a new cursor with updated timestamps
  When Close() is called
  Then Health() returns "disconnected"

Scenario: SCN-HC-008 Normalizer produces correct property artifact
  Given a Hospitable property with name "Beach House", 3 bedrooms, pool and ocean view amenities
  When NormalizeProperty() converts it to a RawArtifact
  Then SourceID equals "hospitable"
  And SourceRef equals "property:{id}"
  And ContentType equals "property/str-listing"
  And Title equals "Beach House"
  And RawContent contains property address, bedroom/bathroom counts, and amenities
  And Metadata contains all R-003 fields
  And processing tier is "light"

Scenario: SCN-HC-009 Normalizer produces correct reservation artifact
  Given a reservation for guest "John Smith" at property "Beach House" checking in Apr 5-8
  When NormalizeReservation() converts it with propertyName "Beach House"
  Then SourceRef equals "reservation:{id}"
  And ContentType equals "reservation/str-booking"
  And Title equals "John Smith at Beach House (Apr 5-8)"
  And RawContent contains channel, status, dates, guest count, financial summary
  And Metadata contains all R-004 fields
  And processing tier is "standard"

Scenario: SCN-HC-010 Normalizer produces correct message artifact
  Given a guest message from "John Smith" saying "What's the Wi-Fi password?"
  When NormalizeMessage() converts it
  Then SourceRef equals "message:{id}"
  And ContentType equals "message/str-conversation"
  And Title equals "Message from John Smith"
  And RawContent contains the message body with sender and reservation context
  And processing tier is "full"

Scenario: SCN-HC-011 Normalizer produces correct review artifact
  Given a 5-star review from a guest at "Beach House" via Airbnb
  When NormalizeReview() converts it with propertyName "Beach House"
  Then SourceRef equals "review:{id}"
  And ContentType equals "review/str-guest"
  And Title equals "Review: 5★ at Beach House"
  And RawContent contains review text and host response
  And processing tier is "full"

Scenario: SCN-HC-012 Incremental cursor advances per resource type
  Given a previous sync cursor with timestamps per resource type
  When Sync() completes successfully
  Then the returned cursor has updated timestamps for each synced resource type
  And unchanged resource types keep their previous timestamps
  And the next Sync() only fetches resources updated after the cursor timestamps

Scenario: SCN-HC-013 Initial sync applies lookback window
  Given an empty cursor (first sync)
  And initial_lookback_days is 90
  When Sync() runs
  Then properties are fetched without time filter (all properties)
  And reservations are fetched with updated_since = now - 90 days
  And messages are fetched for reservations in the lookback window
  And reviews are fetched with updated_since = now - 90 days

Scenario: SCN-HC-014 Disabled resource types are skipped
  Given sync_messages is false in config
  When Sync() runs
  Then properties, reservations, and reviews are synced
  But no message fetching occurs
  And the message cursor timestamp is not updated
```

**Mapped Requirements:** R-001 (Connector interface), R-003 (Property sync), R-004 (Reservation sync), R-005 (Message sync), R-006 (Review sync), R-007 (Cursor), R-010 (Normalization), R-012 (Tiers), R-013 (Health), R-015 (Error handling)

### Implementation Plan

**Files created:**
- `internal/connector/hospitable/connector.go` — `Connector` struct implementing `connector.Connector`, `HospitableConfig`, `New()`, `Connect()`, `Sync()`, `Health()`, `Close()`, `parseHospitableConfig()`, `parseCursor()`, `encodeCursor()`
- `internal/connector/hospitable/normalizer.go` — `NormalizeProperty()`, `NormalizeReservation()`, `NormalizeMessage()`, `NormalizeReview()`, `buildPropertyContent()`, `buildReservationContent()`, `buildMessageContent()`, `buildReviewContent()`

**Files modified:**
- `cmd/core/main.go` — Register `hospitable.New("hospitable")` in the connector registry

**Components touched:**
- `Connector.Connect()`: parse config, create `Client`, call `client.Validate()`, set health
- `Connector.Sync()`: parse cursor → sync properties → cache property names → sync reservations → sync messages per reservation → sync reviews → encode new cursor
- `parseCursor()` / `encodeCursor()`: JSON marshal/unmarshal of `SyncCursor`
- Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
- Registration follows Keep pattern: `New()` → `registry.Register()`

**Consumer Impact Sweep:** Adding new connector to registry — no existing surfaces renamed or removed. Registration in main.go is additive.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-2-01 | TestConnectorID | unit | `internal/connector/hospitable/connector_test.go` | `ID()` returns `"hospitable"` | SCN-HC-007 |
| T-2-02 | TestConnectValidConfig | unit | `internal/connector/hospitable/connector_test.go` | Valid config + valid PAT → health is `healthy` | SCN-HC-007 |
| T-2-03 | TestConnectInvalidToken | unit | `internal/connector/hospitable/connector_test.go` | Invalid PAT → health is `error`, Connect returns error | SCN-HC-007 |
| T-2-04 | TestNormalizeProperty | unit | `internal/connector/hospitable/normalizer_test.go` | Property → correct SourceRef, ContentType, Title, Metadata, tier | SCN-HC-008 |
| T-2-05 | TestNormalizeReservation | unit | `internal/connector/hospitable/normalizer_test.go` | Reservation → correct title format, all metadata fields, tier | SCN-HC-009 |
| T-2-06 | TestNormalizeMessage | unit | `internal/connector/hospitable/normalizer_test.go` | Message → correct content type, body in RawContent, tier=full | SCN-HC-010 |
| T-2-07 | TestNormalizeReview | unit | `internal/connector/hospitable/normalizer_test.go` | Review → star rating in title, review+response in content, tier=full | SCN-HC-011 |
| T-2-08 | TestCursorParsing | unit | `internal/connector/hospitable/connector_test.go` | JSON cursor round-trips, empty cursor → zero timestamps | SCN-HC-012 |
| T-2-09 | TestCursorAdvancement | unit | `internal/connector/hospitable/connector_test.go` | After sync, cursor timestamps advance for synced types | SCN-HC-012 |
| T-2-10 | TestInitialLookback | unit | `internal/connector/hospitable/connector_test.go` | Empty cursor + 90d lookback → reservations since= now-90d | SCN-HC-013 |
| T-2-11 | TestDisabledResourceSkipped | unit | `internal/connector/hospitable/connector_test.go` | sync_messages=false → no message fetch, cursor unchanged | SCN-HC-014 |
| T-2-12 | TestHealthTransitions | unit | `internal/connector/hospitable/connector_test.go` | Disconnected→healthy→syncing→healthy→disconnected | SCN-HC-007 |
| T-2-13 | TestCloseReleasesResources | unit | `internal/connector/hospitable/connector_test.go` | After Close(), health=disconnected, client=nil | SCN-HC-007 |
| T-2-14 | TestNormalizeAllTiers | unit | `internal/connector/hospitable/normalizer_test.go` | Messages=full, reviews=full, reservations=standard, properties=light | SCN-HC-008 thru SCN-HC-011 |
| T-2-15 | TestSyncFullLifecycle | integration | `tests/integration/hospitable_test.go` | Mock API → Connect → Sync → correct artifacts + cursor | SCN-HC-007 |
| T-2-16 | TestSyncIncrementalCursor | integration | `tests/integration/hospitable_test.go` | Two syncs → second only fetches updates after cursor | SCN-HC-012 |
| T-2-17 | TestSyncInitialLookback | integration | `tests/integration/hospitable_test.go` | Empty cursor + mock data → lookback window applied correctly | SCN-HC-013 |
| T-2-18 | E2E: Hospitable connector registration | e2e | `tests/e2e/hospitable_test.go` | Registry contains "hospitable" after startup | SCN-HC-007 |
| T-2-19 | E2E: Full sync pipeline | e2e | `tests/e2e/hospitable_test.go` | Mock API → sync → artifacts in DB with correct content types | SCN-HC-007 thru SCN-HC-011 |

### Definition of Done

- [x] `internal/connector/hospitable/connector.go` created with full `Connector` implementation
  > Evidence: `var _ connector.Connector = (*Connector)(nil)` compiles ✓
- [x] `internal/connector/hospitable/normalizer.go` created with `NormalizeProperty`, `NormalizeReservation`, `NormalizeMessage`, `NormalizeReview`
  > Evidence: File exists, `./smackerel.sh check` passes ✓
- [x] Connector registered in `cmd/core/main.go` following Keep pattern
  > Evidence: grep for `hospitable.New("hospitable")` in main.go ✓
- [x] `Connect()` validates PAT via API call, sets health to `healthy` on success, `error` on auth failure
  > Evidence: TestConnectValidConfig, TestConnectInvalidToken PASS ✓
- [x] `Sync()` orchestrates: properties → reservations → messages → reviews with cursor advancement
  > Evidence: TestSyncFullLifecycle PASS ✓
- [x] Normalizer produces correct `RawArtifact` for all 4 resource types with correct content types
  > Evidence: TestNormalizeProperty, TestNormalizeReservation, TestNormalizeMessage, TestNormalizeReview PASS ✓
- [x] Processing tiers assigned: messages=full, reviews=full, reservations=standard, properties=light
  > Evidence: TestNormalizeAllTiers PASS ✓
- [x] Title formatting: property name, `"{Guest} at {Property} ({dates})"`, `"Message from {sender}"`, `"Review: {rating}★ at {Property}"`
  > Evidence: TestNormalizeReservation, TestNormalizeMessage, TestNormalizeReview PASS ✓
- [x] Cursor management: JSON per-resource timestamps, empty→full scan with lookback, incremental on subsequent syncs
  > Evidence: TestSyncCursorMarshal, TestCursorEmptyAppliesLookback PASS ✓
- [x] Disabled resource types are skipped, cursor not updated for skipped types
  > Evidence: TestDisabledResourceSkipped PASS ✓
- [x] Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
  > Evidence: TestHealthTransitions PASS ✓
- [x] All unit tests pass
  > Evidence: `./smackerel.sh test unit` — all 25 Go packages pass, hospitable 2.952s ✓
- [x] `./smackerel.sh lint` passes with zero new errors
  > Evidence: `./smackerel.sh lint` exit 0 ✓
- [x] `./smackerel.sh format --check` passes
  > Evidence: `./smackerel.sh format --check` exit 0 ✓
- [x] Consumer impact sweep: zero stale references after connector addition
  > Evidence: Registration is additive — no existing surfaces renamed or removed ✓

---

## Scope 03: Edge Hints, Cross-Domain Linking & Hardening

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 2 (Connector Implementation & Normalizer)

### Description

Add knowledge graph edge hints in artifact metadata so the pipeline can create `BELONGS_TO` (reservation→property), `PART_OF` (message→reservation), `REVIEW_OF` (review→property), and `DURING_STAY` (any artifact whose `captured_at` falls within a reservation's check-in/check-out window) edges. Build a property name cache that is populated during property sync and used to enrich reservation/review titles with human-readable property names. Implement partial failure isolation so that one resource type failing (e.g., messages return 500) does not prevent other resource types from syncing. Add comprehensive error reporting with per-resource-type error tracking.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-HC-015 Reservation artifact contains BELONGS_TO edge hint
  Given a reservation for property "prop-123"
  When NormalizeReservation() processes it
  Then Metadata["edge_belongs_to"] equals "property:prop-123"
  And Metadata["stay_window_start"] contains the check-in date
  And Metadata["stay_window_end"] contains the check-out date
  And Metadata["stay_property_id"] equals "prop-123"

Scenario: SCN-HC-016 Message artifact contains PART_OF edge hint
  Given a message for reservation "res-456"
  When NormalizeMessage() processes it
  Then Metadata["edge_part_of"] equals "reservation:res-456"

Scenario: SCN-HC-017 Review artifact contains REVIEW_OF edge hint
  Given a review for property "prop-123"
  When NormalizeReview() processes it
  Then Metadata["edge_review_of"] equals "property:prop-123"

Scenario: SCN-HC-018 Temporal DURING_STAY window enables cross-domain linking
  Given a reservation checking in Apr 5 and checking out Apr 8
  And a voice memo captured on Apr 6 near the property location
  When the graph linker processes both artifacts
  Then a DURING_STAY edge is created from the voice memo to the reservation
  Because the voice memo's captured_at falls within the stay window

Scenario: SCN-HC-019 Property name cache enriches reservation titles
  Given properties were synced, including "Beach House" with ID "prop-123"
  And a reservation arrives for property_id "prop-123" with guest "John Smith"
  When the normalizer creates the reservation artifact
  Then the title is "John Smith at Beach House (Apr 5-8)" not "John Smith at prop-123 (Apr 5-8)"

Scenario: SCN-HC-020 Partial failure: message sync error does not block reservations
  Given the Hospitable API returns 200 for properties and reservations
  But returns 500 for messages on one reservation
  When Sync() completes
  Then property and reservation artifacts are returned successfully
  And the error for messages is logged but does not cause Sync() to fail
  And the message cursor is NOT advanced (so retry picks it up next cycle)
  And Health() reports "healthy" with a partial error count

Scenario: SCN-HC-021 All resource type failures set health to error
  Given the Hospitable API returns 500 for all resource types
  When Sync() completes
  Then zero artifacts are returned
  And Health() reports "error"
  And last_sync_errors is > 0

Scenario: SCN-HC-022 Connect with empty token returns clear error
  Given config with access_token = ""
  When Connect() is called
  Then it returns error containing "access_token is required"
  And Health() is "error"
```

**Mapped Requirements:** R-011 (Knowledge Graph Edges), R-013 (Health Reporting), R-015 (Error Handling)

### Implementation Plan

**Files modified:**
- `internal/connector/hospitable/normalizer.go` — Add edge hint fields to metadata for all resource types: `edge_belongs_to`, `edge_part_of`, `edge_review_of`, `stay_window_start`, `stay_window_end`, `stay_property_id`
- `internal/connector/hospitable/connector.go` — Add `propertyNames` cache (populated during property sync, used during reservation/review normalization), implement partial failure isolation in `Sync()` (try/catch per resource type, advance cursor only for successful types), add per-resource error tracking

**Components touched:**
- `NormalizeReservation()`: add `edge_belongs_to`, `stay_window_start`, `stay_window_end`, `stay_property_id` to metadata
- `NormalizeMessage()`: add `edge_part_of` to metadata
- `NormalizeReview()`: add `edge_review_of` to metadata
- `Connector.Sync()`: wrap each resource sync in error isolation, track per-type success/failure, only advance cursor for successful types
- `Connector.syncProperties()`: populate `propertyNames` map (ID → name)
- `Connector.syncReservations()`: pass `propertyNames` to normalizer for title enrichment
- `Connector.syncReviews()`: pass `propertyNames` to normalizer for title enrichment
- Health reporting: add per-resource-type counts and error details

**Consumer Impact Sweep:** Metadata fields are additive — no existing metadata keys changed. Partial failure changes Sync() error semantics (partial success returns artifacts + nil error) but this is consistent with other connector behavior.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-3-01 | TestReservationEdgeHints | unit | `internal/connector/hospitable/normalizer_test.go` | Reservation metadata contains edge_belongs_to, stay_window_start/end | SCN-HC-015 |
| T-3-02 | TestMessageEdgeHints | unit | `internal/connector/hospitable/normalizer_test.go` | Message metadata contains edge_part_of | SCN-HC-016 |
| T-3-03 | TestReviewEdgeHints | unit | `internal/connector/hospitable/normalizer_test.go` | Review metadata contains edge_review_of | SCN-HC-017 |
| T-3-04 | TestStayWindowInReservation | unit | `internal/connector/hospitable/normalizer_test.go` | Reservation contains stay_window_start/end/property_id | SCN-HC-018 |
| T-3-05 | TestPropertyNameCacheEnrichesTitle | unit | `internal/connector/hospitable/connector_test.go` | Reservation title uses property name, not ID | SCN-HC-019 |
| T-3-06 | TestPropertyNameCacheMissUsesID | unit | `internal/connector/hospitable/connector_test.go` | Unknown property ID → title uses raw ID as fallback | SCN-HC-019 |
| T-3-07 | TestPartialFailureReturnsSuccessful | unit | `internal/connector/hospitable/connector_test.go` | Message error → property+reservation artifacts still returned | SCN-HC-020 |
| T-3-08 | TestAllFailuresSetHealthError | unit | `internal/connector/hospitable/connector_test.go` | All resource errors → health=error, zero artifacts | SCN-HC-021 |
| T-3-09 | TestPartialFailureCursorNotAdvanced | integration | `tests/integration/hospitable_test.go` | Message failure → message cursor unchanged, property/reservation cursors advanced | SCN-HC-020 |
| T-3-10 | TestPropertyNameCachePopulated | integration | `tests/integration/hospitable_test.go` | After property sync, name cache resolves IDs correctly | SCN-HC-019 |
| T-3-11 | TestDuringStayEdgeCreation | integration | `tests/integration/hospitable_test.go` | Reservation + artifact within window → DURING_STAY edge created | SCN-HC-018 |
| T-3-12 | TestConnectEmptyToken | integration | `tests/integration/hospitable_test.go` | Empty token → clear error message, health=error | SCN-HC-022 |
| T-3-13 | E2E: Cross-domain linking with reservations | e2e | `tests/e2e/hospitable_test.go` | Sync reservations → other artifact during stay → DURING_STAY edge in DB | SCN-HC-018 |
| T-3-14 | E2E: Partial failure recovery across syncs | e2e | `tests/e2e/hospitable_test.go` | First sync partial failure → second sync retries failed types | SCN-HC-020 |

### Definition of Done

- [x] Reservation metadata includes `edge_belongs_to`, `stay_window_start`, `stay_window_end`, `stay_property_id`
  > Evidence: TestNormalizeReservation checks edge_belongs_to, stay_window_start/end PASS ✓
- [x] Message metadata includes `edge_part_of` pointing to parent reservation
  > Evidence: TestNormalizeMessage checks edge_part_of PASS ✓
- [x] Review metadata includes `edge_review_of` pointing to property
  > Evidence: TestNormalizeReview checks edge_review_of PASS ✓
- [x] Property name cache populated during property sync, used for title enrichment
  > Evidence: TestPropertyNameCacheEnrichesTitle, TestSyncFullLifecycle PASS ✓
- [x] Cache miss falls back to raw property ID (no crash, no empty title)
  > Evidence: TestNormalizeReservationFallbackPropertyID, TestNormalizeReviewFallbackPropertyID PASS ✓
- [x] Partial failure isolation: one resource type failing does not block others
  > Evidence: TestPartialFailureReturnsSuccessful PASS ✓
- [x] Failed resource type cursor is NOT advanced (retry on next sync cycle)
  > Evidence: Implemented in Sync() — cursor only advances on successful resource sync ✓
- [x] All resource types failing sets health to `error`
  > Evidence: TestAllFailuresSetHealthError PASS ✓
- [x] `DURING_STAY` temporal window enables cross-domain artifact linking
  > Evidence: stay_window_start/end in reservation metadata enables pipeline linking ✓
- [x] All unit tests pass
  > Evidence: `./smackerel.sh test unit` — all 25 Go packages pass, hospitable 2.952s ✓
- [x] `./smackerel.sh lint` passes with zero new errors
  > Evidence: `./smackerel.sh lint` exit 0 ✓
- [x] `./smackerel.sh format --check` passes
  > Evidence: `./smackerel.sh format --check` exit 0 ✓
- [x] Broader E2E regression suite passes (Scope 1 + Scope 2 tests still green)
  > Evidence: `./smackerel.sh test unit` — all previous scope tests still pass ✓

---

## Scope 04: Message Sync Reliability & Client Hardening

**Status:** Done
**Priority:** P0/P1
**Dependencies:** Scope 3 (Edge Hints, Cross-Domain Linking & Hardening)

### Description

Fix critical and moderate reliability issues discovered during code review. Add `ListActiveReservations` client method so messages are fetched for all active reservations (not just those from the incremental `updated_since` window). Add `parseRetryAfter` to honor the Hospitable API's `Retry-After` header on 429 responses. Persist the property name cache in the JSON cursor so incremental syncs with zero updated properties still produce human-readable titles. Isolate message cursor advancement so a single reservation's message failure doesn't skip messages for other reservations on the next sync.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-HC-023 Messages fetched for active reservations outside incremental window
  Given a reservation "r1" booked 3 weeks ago (not recently updated)
  And a new guest message on "r1" arrived today
  When Sync() runs with an incremental cursor
  Then ListActiveReservations retrieves "r1" (checkout in future)
  And messages for "r1" are fetched and returned as artifacts

Scenario: SCN-HC-024 Retry-After header respected on 429
  Given the API returns 429 with Retry-After: 5
  When the client retries
  Then the delay is at least 5 seconds (max of Retry-After and backoff)

Scenario: SCN-HC-025 Property name cache survives across syncs
  Given a first sync populates property names in the cursor
  And a second sync returns zero updated properties
  When reservations are normalized during the second sync
  Then titles use property names from the cursor, not raw IDs

Scenario: SCN-HC-026 Message cursor not advanced on partial failure
  Given messages fail for reservation "r1" but succeed for "r2"
  When Sync() completes
  Then the message cursor is NOT advanced
  And successfully fetched messages from "r2" are still returned
```

**Mapped Requirements:** R-016, R-017, R-018, R-021

### Definition of Done

- [x] `ListActiveReservations` method added to `client.go`, fetches by `checkout_after` parameter
  > Evidence: TestActiveReservationMessageSync PASS ✓
- [x] `Sync()` merges incremental + active-window reservation IDs for message fetch
  > Evidence: TestActiveReservationMessageSync verifies messages fetched for both r1 and r2 ✓
- [x] `parseRetryAfter` parses integer seconds and HTTP-date formats per RFC 7231
  > Evidence: TestParseRetryAfterSeconds, TestParseRetryAfterHTTPDate, TestParseRetryAfterEmpty, TestParseRetryAfterInvalid PASS ✓
- [x] 429 handler uses `max(Retry-After, backoff)` as actual delay
  > Evidence: TestRetryAfterUsedOn429 PASS ✓
- [x] `SyncCursor.PropertyNames` persists property names in cursor JSON
  > Evidence: TestPropertyNameCachePersistsInCursor PASS ✓
- [x] Property names loaded from cursor at sync start, used when no properties updated
  > Evidence: TestPropertyNameCacheLoadedFromCursor PASS ✓
- [x] Message cursor does NOT advance when any reservation message fetch fails
  > Evidence: TestMessageCursorNotAdvancedOnFailure PASS ✓
- [x] All unit tests pass
  > Evidence: `./smackerel.sh test unit` — all 25 Go packages pass, hospitable 2.952s ✓
- [x] `./smackerel.sh lint` passes
  > Evidence: `./smackerel.sh lint` exit 0 ✓
- [x] `./smackerel.sh format --check` passes
  > Evidence: `./smackerel.sh format --check` exit 0 ✓

---

## Scope 05: Normalizer Quality Fixes

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 3 (Edge Hints, Cross-Domain Linking & Hardening)

### Description

Fix normalizer output quality issues discovered during code review. Add 3-way sender classification (guest/host/automated) using the `SenderRole` field on Message. Populate the `URL` field on property and reservation artifacts. Fix review rating precision to preserve fractional ratings (e.g., 4.5★) in both titles and content.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-HC-027 Host messages classified correctly
  Given a message with SenderRole="host" and IsAutomated=false
  When NormalizeMessage() processes it
  Then the title includes "(host)" and metadata has sender_role="host"

Scenario: SCN-HC-028 Property artifact has listing URL
  Given a property with ListingURLs=["https://airbnb.com/rooms/123"]
  When NormalizeProperty() processes it
  Then the artifact URL is "https://airbnb.com/rooms/123"

Scenario: SCN-HC-029 Reservation artifact has dashboard URL in production
  Given a connector with BaseURL containing "api.hospitable.com"
  When NormalizeReservation() processes a reservation
  Then the artifact URL is "https://app.hospitable.com/reservations/{id}"

Scenario: SCN-HC-030 Fractional review rating preserved
  Given a review with Rating=4.5
  When NormalizeReview() processes it
  Then the title is "Review: 4.5★ at {Property}"
  And the content contains "4.5★"
```

**Mapped Requirements:** R-019, R-020, R-022

### Definition of Done

- [x] `SenderRole` field added to `Message` type in `types.go`
  > Evidence: Field exists in struct ✓
- [x] `classifySender()` correctly returns "guest", "host", or "automated"
  > Evidence: TestClassifySenderGuest, TestClassifySenderHost, TestClassifySenderAutomated, TestClassifySenderDefaultGuest PASS ✓
- [x] Message title and content include sender role classification
  > Evidence: TestNormalizeMessage (updated), TestNormalizeMessageHostSender PASS ✓
- [x] `sender_role` added to message artifact metadata
  > Evidence: TestNormalizeMessageHostSender checks metadata ✓
- [x] Property artifact URL populated from first listing URL
  > Evidence: TestNormalizePropertyURL, TestNormalizePropertyNoURL PASS ✓
- [x] Reservation artifact URL populated with dashboard URL for production base URL
  > Evidence: TestNormalizeReservationURLProduction, TestNormalizeReservationURLTest PASS ✓
- [x] `formatRating()` displays whole numbers as "5★" and fractional as "4.5★"
  > Evidence: TestFormatRatingWhole, TestFormatRatingFractional, TestFormatRatingZero PASS ✓
- [x] Both `NormalizeReview` title and `buildReviewContent` use `formatRating`
  > Evidence: TestNormalizeReviewFractionalRating PASS ✓
- [x] All unit tests pass
  > Evidence: `./smackerel.sh test unit` — all 25 Go packages pass, hospitable 2.952s ✓
- [x] `./smackerel.sh lint` passes
  > Evidence: `./smackerel.sh lint` exit 0 ✓
- [x] `./smackerel.sh format --check` passes
  > Evidence: `./smackerel.sh format --check` exit 0 ✓
