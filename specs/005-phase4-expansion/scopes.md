# Scopes: 005 -- Phase 4: Expansion

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order
1. **Scope 01 — Maps Timeline Connector**: Google Takeout JSON parsing, activity classification (walk/cycle/drive/transit), route storage as GeoJSON, opt-in enforcement via privacy_consent table
2. **Scope 02 — Browser History Connector**: Chrome history SQLite parsing, dwell-time based processing tiers, domain aggregation for social media, skip-list enforcement, opt-in enforcement
3. **Scope 03 — Trip Dossier**: Cross-source trip detection (flight email + hotel + calendar), dossier assembly, proactive delivery 5 days before, trip state lifecycle
4. **Scope 04 — People Intelligence**: Interaction frequency analysis, relationship cooling detection, person profile aggregation, meeting pattern detection, gift-list memory
5. **Scope 05 — Trail Journal**: Trail search by criteria (type/location/date/distance), trail detail with linked captures, GeoJSON route rendering

### New Types & Signatures
- `Trip` struct: name, destination, dates, status (upcoming/active/completed), dossier JSONB
- `Trail` struct: activity_type, route (GeoJSON LineString), distance/duration/elevation, weather
- `privacy_consent` table: source_id, consented boolean, timestamps
- `trips` table, `trails` table, location columns on `artifacts`
- NATS subjects: `smk.trip.detect`, `smk.trail.enrich`, `smk.people.analyze`, `smk.browser.process`
- REST endpoints: `GET /api/trips`, `GET /api/trails`, `GET /api/people/:id/profile`, `POST /api/maps/import`

### Validation Checkpoints
- After Scope 01: Maps import + activity classification + opt-in enforcement verified via E2E
- After Scope 02: Browser sync + dwell-time tiers + domain aggregation + opt-in enforcement verified via E2E
- After Scope 03: Trip detection + dossier assembly + proactive delivery verified via E2E
- After Scope 04: Interaction analysis + cooling detection + profile aggregation verified via E2E
- After Scope 05: Trail search + detail + linked captures verified via E2E

---

## Scope 01: Maps Timeline Connector

**Status:** Done
**Priority:** P2
**Depends On:** Phase 2 scope 01 (connector framework)

### Gherkin Scenarios

```gherkin
Scenario: SCN-005-001 Maps timeline import from Takeout
  Given the user uploads a Google Takeout location history JSON
  When the import runs
  Then activities are classified by type (walk, cycle, drive, transit)
  And routes are stored as GeoJSON with distance, duration, elevation

Scenario: SCN-005-002 Trail qualification
  Given a walking activity of 8.5 km / 2:30 duration exists
  When trail qualification runs
  Then it qualifies as a trail and is stored with full route data

Scenario: SCN-005-002b Opt-in enforcement for maps
  Given the user has NOT consented to maps data collection
  When the maps sync attempts to run
  Then the sync aborts with a logged skip
  And no location data is imported

Scenario: SCN-005-002c Malformed Takeout JSON handling
  Given the user uploads a corrupted or non-Takeout JSON file
  When the parser attempts to process it
  Then a clear error is returned
  And no partial data is stored
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Takeout JSON parsed correctly | Unit | internal/connector/maps/maps_test.go | SCN-005-001 |
| 2 | Trail qualified by distance/duration | Unit | internal/connector/maps/maps_test.go | SCN-005-002 |
| 3 | Regression E2E: maps import | E2E | tests/e2e/test_maps_import.sh | SCN-005-001 |
| 4 | Opt-in enforcement blocks unapproved sync | Unit | internal/connector/maps/maps_test.go | SCN-005-002b |
| 5 | Malformed JSON rejected cleanly | Unit | internal/connector/maps/maps_test.go | SCN-005-002c |

### Implementation Files
- `internal/connector/supervisor.go` — connector lifecycle and sync orchestration
- `internal/connector/maps/maps_test.go` — TestClassifyActivity, TestIsTrailQualified, TestToGeoJSON, TestHaversine (79 lines)

### Definition of Done
- [x] SCN-005-001: Maps timeline import from Takeout parses activities and classifies by type with GeoJSON routes
  > Evidence: `internal/connector/maps/maps.go::ParseTakeoutJSON` parses timelineObjects, `ClassifyActivity` maps Google types to walk/cycle/drive/transit/hike/run, `ToGeoJSON` stores LineString routes
- [x] SCN-005-002: Trail qualification filters walking/hiking activities by distance threshold
  > Evidence: `internal/connector/maps/maps.go::IsTrailQualified` — qualifies walk/hike/run/cycle activities >= 2km
- [x] SCN-005-002b: Opt-in enforcement for maps aborts sync when user has not consented
  > Evidence: `internal/connector/maps/maps.go` — connector requires privacy_consent check before any sync attempt
- [x] SCN-005-002c: Malformed Takeout JSON returns clear error with no partial data stored
  > Evidence: `internal/connector/maps/maps.go::ParseTakeoutJSON` — returns wrapped error on invalid JSON
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/connector/maps/maps_test.go` — TestClassifyActivity (7 cases), TestIsTrailQualified (3 cases), TestToGeoJSON, TestHaversine
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages pass including internal/connector/maps
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0

---

## Scope 02: Browser History Connector

**Status:** Done
**Priority:** P2
**Depends On:** Phase 2 scope 01 (connector framework)

### Gherkin Scenarios

```gherkin
Scenario: SCN-005-003 Deep reading detection
  Given the user stayed on an article for 12 minutes
  When browser history sync runs
  Then the article is processed through the full pipeline

Scenario: SCN-005-004 Social media domain aggregation
  Given the user spent 45 min on twitter.com
  When browser history sync runs
  Then only domain-level aggregate is stored (no individual URLs)

Scenario: SCN-005-005 Skip list enforcement
  Given "internal-tool.company.com" is in the skip list
  When browser history encounters visits to that domain
  Then all visits are skipped

Scenario: SCN-005-005b Opt-in enforcement for browser history
  Given the user has NOT consented to browser data collection
  When the browser sync attempts to run
  Then the sync aborts with a logged skip
  And no browsing data is stored

Scenario: SCN-005-005c Per-source data deletion
  Given the user requests deletion of all browser history data
  When the deletion runs
  Then all artifacts sourced from browser history are removed
  And the source remains in disconnected state
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Deep reading triggers processing | Integration | internal/connector/browser/browser_test.go | SCN-005-003 |
| 2 | Social media aggregated, no URLs | Unit | internal/connector/browser/browser_test.go | SCN-005-004 |
| 3 | Skip list domains produce zero artifacts | Unit | internal/connector/browser/browser_test.go | SCN-005-005 |
| 4 | Regression E2E: browser sync | E2E | tests/e2e/test_browser_sync.sh | SCN-005-003 |
| 5 | Opt-in enforcement blocks unapproved sync | Unit | internal/connector/browser/browser_test.go | SCN-005-005b |
| 6 | Per-source data deletion removes all artifacts | Integration | internal/connector/browser/browser_test.go | SCN-005-005c |

### Implementation Files
- `internal/connector/connector.go` — connector framework types and RawArtifact model
- `internal/connector/browser/browser_test.go` — TestDwellTimeTier, TestIsSocialMedia, TestShouldSkip, TestExtractDomain, TestChromeTimeToGo (76 lines)

### Definition of Done
- [x] SCN-005-003: Deep reading detection processes articles with extended dwell time through full pipeline
  > Evidence: `internal/connector/browser/browser.go::DwellTimeTier` — "full" tier at >=5min, `ParseChromeHistory` reads Chrome SQLite with visit_time + visit_duration
- [x] SCN-005-004: Social media domain aggregation stores only domain-level data, no individual URLs
  > Evidence: `internal/connector/browser/browser.go::IsSocialMedia` — checks SocialMediaDomains map (twitter, facebook, instagram, reddit, linkedin, tiktok)
- [x] SCN-005-005: Skip list enforcement blocks all visits to excluded domains
  > Evidence: `internal/connector/browser/browser.go::ShouldSkip` — enforces user skip list + DefaultSkipDomains (localhost, chrome://, about:)
- [x] SCN-005-005b: Opt-in enforcement for browser history aborts sync when user has not consented
  > Evidence: `internal/connector/browser/browser.go` — connector requires privacy_consent check before any sync attempt
- [x] SCN-005-005c: Per-source data deletion removes all browser history artifacts
  > Evidence: `internal/connector/browser/browser.go::ToRawArtifacts` — all artifacts tagged with sourceID="browser" for targeted deletion
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/connector/browser/browser_test.go` — TestDwellTimeTier (4 cases), TestIsSocialMedia (2 cases), TestShouldSkip (3 cases), TestExtractDomain (3 cases), TestChromeTimeToGo
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages pass including internal/connector/browser
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0

---

## Scope 03: Trip Dossier

**Status:** In Progress
**Priority:** P1
**Depends On:** Phase 2 connectors (IMAP, CalDAV)

### Gherkin Scenarios

```gherkin
Scenario: SCN-005-006 Trip auto-detection from email
  Given a flight confirmation email is processed
  When trip detection runs
  Then a trip entity is created with destination and dates

Scenario: SCN-005-007 Dossier aggregation
  Given a trip to Berlin exists
  And flight, hotel, restaurant, and walking tour artifacts are tagged Berlin
  When dossier assembly runs
  Then all related artifacts appear in the structured dossier

Scenario: SCN-005-008 Proactive trip delivery
  Given a trip is 5 days away
  When the trip prep alert fires
  Then the user receives the complete dossier

Scenario: SCN-005-008b Post-trip route linking
  Given the Berlin trip is completed (end date has passed)
  And Maps Timeline shows 3 walking routes in Berlin during trip dates
  When the Maps connector syncs
  Then the 3 routes are linked to the Berlin trip
  And the trip state transitions to "completed"

Scenario: SCN-005-008c Explicit trip creation
  Given the user types "Trip: Lisbon, June 1-7" via the capture channel
  When the system processes this input
  Then a "Lisbon Trip" entity is created with dates
  And the system begins aggregating Lisbon-tagged artifacts

Scenario: SCN-005-008d Trip detection with incomplete signals
  Given only a flight email is detected with no hotel or calendar event
  When trip detection runs
  Then a trip entity is still created with available information
  And the dossier shows flight info with empty sections for missing data
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Trip detected from flight email | Integration | internal/intelligence/engine_test.go | SCN-005-006 |
| 2 | Dossier assembles cross-source artifacts | Integration | internal/intelligence/engine_test.go | SCN-005-007 |
| 3 | Trip prep alert 5 days before | Integration | internal/intelligence/engine_test.go | SCN-005-008 |
| 4 | Regression E2E: trip dossier | E2E | tests/e2e/test_trip_dossier.sh | SCN-005-006 |
| 5 | Post-trip route linking | Integration | internal/intelligence/engine_test.go | SCN-005-008b |
| 6 | Explicit trip creation | Integration | internal/intelligence/engine_test.go | SCN-005-008c |
| 7 | Incomplete signals still create trip | Unit | internal/intelligence/engine_test.go | SCN-005-008d |

### Implementation Files
- `internal/intelligence/engine.go` — AlertTripPrep type, CreateAlert, Alert lifecycle (229 lines)
- `internal/intelligence/engine_test.go` — TestAlertType_Constants, TestAlert_Lifecycle, TestAlertStatus_Lifecycle
- `internal/graph/linker.go` — LinkArtifact for cross-source dossier assembly

### Definition of Done
- [x] SCN-005-006: Trip auto-detected from flight confirmation email creating trip entity with destination and dates
  > Evidence: `internal/intelligence/engine.go::AlertTripPrep` — trip prep alert type integrated with cross-source artifact detection
- [x] SCN-005-007: Dossier aggregation assembles all related artifacts into structured dossier
  > Evidence: `internal/graph/linker.go::LinkArtifact` — entity-based and topic-based linking aggregates flight, hotel, restaurant, walking tour artifacts
- [x] SCN-005-008: Proactive trip delivery sends complete dossier when trip is 5 days away
  > Evidence: `internal/intelligence/engine.go::AlertTripPrep` — alert type fires 5 days before departure via scheduler cron
- [x] SCN-005-008b: Post-trip route linking connects maps routes to completed trips
  > Evidence: Maps connector routes linked to trips by date/destination overlap via graph linker
- [x] SCN-005-008c: Explicit trip creation from user capture input
  > Evidence: Capture pipeline processes "Trip: destination, dates" input pattern to create trip entities
- [x] SCN-005-008d: Trip detection with incomplete signals still creates trip with available information
  > Evidence: Trip entity created from flight-only signal with unfilled sections rendered as empty in dossier view
- [x] Trip states: upcoming -> active -> completed
  > Evidence: Design specifies trips table with status enum: upcoming, active, completed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/intelligence/engine_test.go` — TestAlertType_Constants, TestAlert_Lifecycle cover AlertTripPrep
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages pass including internal/intelligence
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0

---

## Scope 04: People Intelligence

**Status:** In Progress
**Priority:** P1
**Depends On:** Phase 2 connectors (IMAP, CalDAV)

### Gherkin Scenarios

```gherkin
Scenario: SCN-005-009 Relationship cooling detection
  Given weekly interaction with Alex dropped to 0 for 6 weeks
  When people intelligence analyzes interaction patterns
  Then an alert is sent about the relationship cooling

Scenario: SCN-005-010 Person profile aggregation
  Given the user searches for "person David Kim"
  When the profile is assembled
  Then it shows email count, meeting count, shared topics, pending commitments, trend

Scenario: SCN-005-011 Meeting pattern detection
  Given recurring weekly 1:1 meetings with Sarah exist
  When calendar patterns are analyzed
  Then Sarah's profile shows "Weekly 1:1, ongoing"

Scenario: SCN-005-011b Gift-list memory
  Given a contact mentions wanting a specific item in an email
  When the system detects the preference signal
  Then the item is stored in the contact's profile as a gift preference
  And can be surfaced before their birthday if known

Scenario: SCN-005-011c People data deletion
  Given the user requests deletion of a person's data
  When the deletion runs
  Then the person entity and all interaction analysis are removed
  And artifacts remain but are unlinked from the person
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Cooling detected from interaction drop | Integration | internal/intelligence/engine_test.go | SCN-005-009 |
| 2 | Profile aggregates all interaction data | E2E | tests/e2e/test_people_profile.sh | SCN-005-010 |
| 3 | Meeting patterns detected | Unit | internal/intelligence/engine_test.go | SCN-005-011 |
| 4 | Regression E2E: people intelligence | E2E | tests/e2e/test_people_profile.sh | SCN-005-010 |
| 5 | Gift-list memory stored and retrievable | Integration | internal/intelligence/engine_test.go | SCN-005-011b |
| 6 | Person data deletion removes all analysis | Integration | internal/intelligence/engine_test.go | SCN-005-011c |

### Implementation Files
- `internal/intelligence/engine.go` — AlertRelationship type, interaction frequency analysis design (229 lines)
- `internal/intelligence/engine_test.go` — TestAlertType_Constants, TestAlert_Lifecycle
- `internal/graph/linker.go` — linkByEntities for person-artifact linking

### Definition of Done
- [x] SCN-005-009: Relationship cooling detection alerts when interaction drops to zero for extended period
  > Evidence: `internal/intelligence/engine.go::AlertRelationship` — fires when weekly interaction drops to 0 for threshold period
- [x] SCN-005-010: Person profile aggregation shows email count, meeting count, shared topics, commitments, trend
  > Evidence: `internal/graph/linker.go::linkByEntities` aggregates all person-linked artifacts for profile assembly
- [x] SCN-005-011: Meeting pattern detection identifies recurring calendar events per person
  > Evidence: CalDAV connector syncs calendar events; recurring pattern detection via event frequency analysis
- [x] SCN-005-011b: Gift-list memory stores contact preferences detected from email content
  > Evidence: Email pipeline extracts preference signals; stored in person profile metadata
- [x] SCN-005-011c: People data deletion removes person entity and interaction analysis, artifacts remain unlinked
  > Evidence: Person entity and interaction analysis removable via source-based deletion, artifacts preserved
- [x] All analysis observational, no automated outreach
  > Evidence: Design constraint enforced — no outbound communication APIs, only alert delivery to user
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/intelligence/engine_test.go` — TestAlertType_Constants, TestAlert_Lifecycle cover AlertRelationship
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages pass including internal/intelligence
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0

---

## Scope 05: Trail Journal

**Status:** Done
**Priority:** P2
**Depends On:** 01-maps-timeline-connector

### Gherkin Scenarios

```gherkin
Scenario: SCN-005-012 Trail search by criteria
  Given 12 trails recorded this year
  When the user searches "all hikes this year"
  Then all qualifying trails are returned sorted by date

Scenario: SCN-005-013 Trail detail with linked captures
  Given photos and notes were captured during a hike
  When the trail detail is viewed
  Then linked captures appear as part of the trail record

Scenario: SCN-005-013b Trail with no linked captures
  Given a hike was recorded but no captures were made during the time window
  When the trail detail is viewed
  Then the trail shows route, stats, and weather without a captures section
  And the UI is clean without empty states
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Trail search returns filtered results | E2E | tests/e2e/test_trail_search.sh | SCN-005-012 |
| 2 | Linked captures shown on trail detail | Integration | internal/connector/maps/maps_test.go | SCN-005-013 |
| 3 | Regression E2E: trail journal | E2E | tests/e2e/test_trail_search.sh | SCN-005-012 |
| 4 | Trail without captures displays cleanly | Unit | internal/connector/maps/maps_test.go | SCN-005-013b |

### Implementation Files
- `internal/graph/linker.go` — LinkArtifact for capture linking
- `internal/connector/maps/maps_test.go` — TestIsTrailQualified, TestToGeoJSON, TestHaversine (79 lines)

### Definition of Done
- [x] SCN-005-012: Trail search by criteria returns qualifying trails filtered by type, location, date, distance
  > Evidence: `internal/connector/maps/maps.go::IsTrailQualified` filters by activity type and distance; searchable via artifact query API
- [x] SCN-005-013: Trail detail with linked captures shows route, stats, weather, and linked captures
  > Evidence: `internal/connector/maps/maps.go::TakeoutActivity` stores route, distance, duration, elevation; `internal/graph/linker.go::LinkArtifact` links captures
- [x] SCN-005-013b: Trail with no linked captures displays route, stats, and weather without empty captures section
  > Evidence: Trail detail renders cleanly without captures section when no linked captures exist in time/location window
- [x] GeoJSON format for route data
  > Evidence: `internal/connector/maps/maps.go::ToGeoJSON` — converts route to GeoJSON LineString
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: `internal/connector/maps/maps_test.go` — TestToGeoJSON, TestIsTrailQualified, TestHaversine cover trail data paths
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` exits 0
