# Scopes: 005 -- Phase 4: Expansion

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Scope: 01-maps-timeline-connector

**Status:** Not Started
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Takeout JSON parsed correctly | Unit | internal/connector/maps/takeout_test.go | SCN-005-001 |
| 2 | Trail qualified by distance/duration | Unit | internal/connector/maps/trail_test.go | SCN-005-002 |
| 3 | Regression E2E: maps import | E2E | tests/e2e/test_maps_import.sh | SCN-005-001 |

### Definition of Done
- [ ] Google Takeout JSON location history parsed
- [ ] Activities classified by type
- [ ] Routes stored as GeoJSON with distance, duration, elevation
- [ ] Opt-in enforced via privacy_consent table
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-browser-history-connector

**Status:** Not Started
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Deep reading triggers processing | Integration | internal/connector/browser/chrome_test.go | SCN-005-003 |
| 2 | Social media aggregated, no URLs | Unit | internal/connector/browser/qualifier_test.go | SCN-005-004 |
| 3 | Skip list domains produce zero artifacts | Unit | internal/connector/browser/qualifier_test.go | SCN-005-005 |
| 4 | Regression E2E: browser sync | E2E | tests/e2e/test_browser_sync.sh | SCN-005-003 |

### Definition of Done
- [ ] Chrome history SQLite parsed for dwell time and revisits
- [ ] Articles with >3 min dwell processed through full pipeline
- [ ] Social media stored as domain-level aggregates only
- [ ] Skip list enforced, no artifacts created for excluded domains
- [ ] Opt-in enforced via privacy_consent table
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-trip-dossier

**Status:** Not Started
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Trip detected from flight email | Integration | internal/intelligence/trips/detector_test.go | SCN-005-006 |
| 2 | Dossier assembles cross-source artifacts | Integration | internal/intelligence/trips/assembler_test.go | SCN-005-007 |
| 3 | Trip prep alert 5 days before | Integration | internal/intelligence/alerts/trips_test.go | SCN-005-008 |
| 4 | Regression E2E: trip dossier | E2E | tests/e2e/test_trip_dossier.sh | SCN-005-006 |

### Definition of Done
- [ ] Trip detected from flight/hotel confirmation emails
- [ ] Dossier aggregates artifacts across sources
- [ ] Trip prep alert delivered 5 days before departure
- [ ] Trip states: upcoming -> active -> completed
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-people-intelligence

**Status:** Not Started
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Cooling detected from interaction drop | Integration | internal/intelligence/people/radar_test.go | SCN-005-009 |
| 2 | Profile aggregates all interaction data | E2E | tests/e2e/test_people_profile.sh | SCN-005-010 |
| 3 | Meeting patterns detected | Unit | internal/intelligence/people/analyzer_test.go | SCN-005-011 |
| 4 | Regression E2E: people intelligence | E2E | tests/e2e/test_people_profile.sh | SCN-005-010 |

### Definition of Done
- [ ] Interaction frequency and trend calculated per person
- [ ] Relationship cooling detection with soft alert
- [ ] Person profile shows: email count, meetings, shared topics, commitments
- [ ] Meeting patterns detected from calendar data
- [ ] All analysis observational, no automated outreach
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 05-trail-journal

**Status:** Not Started
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
```

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Trail search returns filtered results | E2E | tests/e2e/test_trail_search.sh | SCN-005-012 |
| 2 | Linked captures shown on trail detail | Integration | internal/intelligence/trips/trail_test.go | SCN-005-013 |
| 3 | Regression E2E: trail journal | E2E | tests/e2e/test_trail_search.sh | SCN-005-012 |

### Definition of Done
- [ ] Trails searchable by type, location, date, distance
- [ ] Trail detail shows route, stats, weather, linked captures
- [ ] GeoJSON format for route data
- [ ] Scenario-specific E2E regression tests
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
