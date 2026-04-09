# Feature: 017 — Government Alerts Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 5.10 Source Priority Matrix (Environmental Alerts, v3)

---

## Problem Statement

Natural disasters, severe weather, and public safety events are the most immediately actionable knowledge a person can receive — yet personal knowledge systems treat them as someone else's problem. Smackerel already knows where you live, where you travel, and where your maps timeline places you. Without government alert ingestion, this location-aware intelligence has a critical blind spot for **safety-relevant events**.

Without a government alerts connector, Smackerel has concrete gaps:

1. **Earthquake awareness is delayed.** A magnitude 4.5 earthquake hits 30 miles from the user's home. The USGS publishes this information within 2 minutes. But without alert ingestion, Smackerel can't include it in the digest, can't annotate the Maps timeline, and can't proactively warn the user.
2. **Tsunami warnings miss travelers.** A user has a flight to Honolulu next week (detected via calendar). A tsunami advisory is issued for Hawaii. Without alert ingestion, the trip dossier can't include this critical safety information.
3. **Severe weather blinds trip planning.** A tornado watch is issued for the user's destination city. The knowledge graph has the travel itinerary but no mechanism to inject severe weather into pre-departure context.
4. **Historical disaster context is lost.** "Was there an earthquake when I was in LA last October?" is unanswerable without historical alert data linked to the Maps timeline.
5. **Wildfire and air quality affect daily life.** Active wildfires near the user's location or poor air quality are public safety signals that should appear in the daily digest alongside weather. Without a connector, these are invisible.

Government alerts are classified as v3 priority in the source priority matrix (section 5.10): "Weather, earthquake, tsunami — contextual safety alerts." This spec activates that capability using freely available government data feeds.

---

## Outcome Contract

**Intent:** Aggregate government safety alerts — earthquakes, tsunamis, severe weather, wildfires, volcanic activity, and air quality — from official free data feeds, filter by relevance to the user's locations and travel plans, and inject them into the knowledge graph as time-sensitive artifacts that power proactive alerts, enrich daily digests, and annotate the Maps timeline with historical disaster context.

**Success Signal:** A user configures their home location (San Francisco) and has a trip to Tokyo next month. Within the first sync cycle: (1) a M3.2 earthquake 50 miles away appears in the daily digest, (2) a winter storm warning for the Sierra Nevada (150 miles away) is noted as potentially affecting weekend plans, (3) the Tokyo trip dossier includes recent seismic activity for Japan, and (4) when an AQI spike occurs due to a distant wildfire, a notification is sent before the next scheduled digest.

**Hard Constraints:**
- All data sources are FREE government APIs — no paid subscriptions, no API keys for core functionality
- Read-only consumption — never submits user data to government APIs beyond location coordinates for filtering
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Location coordinates sent to APIs MUST be rounded to reduce precision for privacy
- Alert severity classification MUST follow the official Common Alerting Protocol (CAP) severity levels
- Expired alerts MUST be retained for historical queries but excluded from active alerting
- All data stored locally — no cloud persistence

**Failure Condition:** If the connector is enabled with a home location, and within 24 hours: a USGS earthquake within 100 km is not captured, a NWS severe thunderstorm warning for the user's county is not delivered, or expired alerts clutter active notifications alongside current ones — the connector has failed.

---

## Goals

1. **Multi-source alert aggregation** — Ingest alerts from USGS (earthquakes, volcanoes), NWS (severe weather), NOAA (tsunamis), and GDACS (global disasters) into a unified alert stream
2. **Location-aware filtering** — Only process alerts relevant to the user's configured locations, upcoming travel destinations, and dynamic positions from the Maps connector
3. **Severity-driven processing** — Classify alerts by severity (extreme, severe, moderate, minor, unknown) following CAP standards, with tier assignment based on severity and proximity
4. **Alert lifecycle management** — Track alert state (active, updated, cancelled, expired) with proper transitions and expiration handling
5. **Proactive delivery** — Route high-severity alerts to the notification system for immediate user delivery, bypassing the normal digest cycle
6. **Cross-connector enrichment** — Enrich trip dossiers with destination alerts, annotate Maps timeline with historical disaster data, and include relevant alerts in daily digests
7. **Historical query support** — Maintain alert history for temporal queries ("were there any earthquakes near me last year?")
8. **Free-tier sustainability** — All government data feeds are free; the connector must stay within any rate limits

---

## Non-Goals

- **Emergency management system** — This is NOT a FEMA/911 replacement. It is a knowledge enrichment connector for personal awareness.
- **Replace official alert apps** — Users should keep FEMA app, Earthquake Early Warning, and local alert systems. This connector adds knowledge graph context, not primary safety alerting.
- **Real-time early warning** — The connector polls at intervals (5-15 minutes). It does NOT provide earthquake early warning (seconds before shaking) or tornado siren-level immediacy.
- **International alert aggregation** — Primary focus is US government feeds (USGS, NWS, NOAA). GDACS provides basic global coverage; per-country alert systems are future scope.
- **Alert response or action** — No evacuation routing, no emergency contact notifications, no shelter-in-place guidance.
- **Environmental monitoring** — No continuous air quality monitoring, no radiation tracking, no water quality. Only alert-driven data.
- **Insurance or damage assessment** — No property damage estimation, no insurance claim support.
- **Crowd-sourced reports** — No integration with citizen science (Did You Feel It?, social media reports).

---

## Data Source Strategy

### Available Government Data Feeds

All sources listed below are **free, publicly accessible, and require no API key** (except AirNow which requires free key registration).

| Source | Agency | Data | Format | Auth | Polling | Coverage |
|--------|--------|------|--------|------|---------|----------|
| **USGS Earthquake** | U.S. Geological Survey | Real-time earthquake events | GeoJSON | None | 5 min | Global |
| **NWS Alerts** | National Weather Service | Severe weather alerts | JSON-LD / CAP | None | 10 min | US |
| **NOAA Tsunami** | NOAA Tsunami Warning Centers | Tsunami alerts | Atom/RSS | None | 5 min | Pacific & Atlantic |
| **GDACS** | UN OCHA | Global disaster alerts | RSS/XML | None | 15 min | Global |
| **AirNow** | EPA | Air quality index | JSON | Free key | 30 min | US |
| **USGS Volcano** | USGS Volcano Hazards | Volcanic activity | JSON | None | 30 min | US + territories |
| **InciWeb** | NIFC | Active wildfire incidents | RSS | None | 30 min | US |

### Recommendation: Tiered Source Strategy

**Tier 1 (Always enabled):**
- USGS Earthquake API — global coverage, GeoJSON, excellent data quality
- NWS Alert API — US severe weather, JSON-LD with CAP format

**Tier 2 (Enabled by default if user has US location):**
- AirNow — requires free API key registration
- USGS Volcano — US volcanic activity
- InciWeb — US wildfires

**Tier 3 (Opt-in for global coverage):**
- NOAA Tsunami — Pacific and Atlantic tsunami alerts
- GDACS — global disaster aggregation

### USGS Earthquake API Detail

```
Endpoint: https://earthquake.usgs.gov/fdsnws/event/1/query
Method: GET
Parameters:
  format=geojson
  starttime=2026-04-08T00:00:00
  endtime=2026-04-09T00:00:00
  minmagnitude=2.5
  latitude=37.7749&longitude=-122.4194&maxradiuskm=200
  orderby=time
Rate limit: None stated (be respectful — max 1 request/5 min per filter)
```

Response includes: magnitude, location, depth, time, tsunami flag, alert level, felt reports.

### NWS Alert API Detail

```
Endpoint: https://api.weather.gov/alerts/active
Method: GET
Parameters:
  point=37.7749,-122.4194  (or area=CA for state-level)
  severity=extreme,severe,moderate
  status=actual
Headers: User-Agent required (identify your application)
Rate limit: None stated (be respectful)
```

Response follows the Common Alerting Protocol (CAP) with: event type, severity, certainty, urgency, headline, description, instruction, effective/expires times, affected zones.

---

## Requirements

### R-001: Connector Interface Compliance

The Government Alerts connector MUST implement the standard `Connector` interface:

- `ID()` returns `"gov-alerts"`
- `Connect()` validates configuration (at least one location configured, at least one source enabled), tests reachability of enabled APIs, sets health to `healthy`
- `Sync()` polls all enabled sources for alerts within configured location radii, deduplicates, returns `[]RawArtifact` and new cursor
- `Health()` reports per-source API reachability status
- `Close()` releases resources

### R-002: Location Configuration

The connector MUST support multiple location sources:

| Source | Configuration | Description |
|--------|--------------|-------------|
| **Static locations** | Configured in `smackerel.yaml` | Home, work, family locations |
| **Travel destinations** | Auto-detected from Calendar/Maps | Upcoming trip destinations |
| **Dynamic position** | From Maps connector | User's current location area |

```yaml
# config/smackerel.yaml
connectors:
  gov-alerts:
    enabled: false
    locations:
      - name: "Home"
        latitude: 0.0   # REQUIRED when enabled
        longitude: 0.0   # REQUIRED when enabled
        radius_km: 150
      - name: "Work"
        latitude: 0.0
        longitude: 0.0
        radius_km: 50
    travel_alert_days_ahead: 14  # How far ahead to check travel destinations
    sources:
      earthquake: true
      weather: true
      tsunami: false
      volcano: true
      wildfire: true
      air_quality: false  # Requires AirNow API key
      global_disasters: false  # GDACS
    air_quality_api_key: ""  # Required for air_quality source
    min_earthquake_magnitude: 2.5
    polling_intervals:
      earthquake: "5m"
      weather: "10m"
      tsunami: "5m"
      volcano: "30m"
      wildfire: "30m"
      air_quality: "30m"
      global_disasters: "15m"
```

### R-003: Alert Type Classification

All alerts MUST be classified using standardized content types:

| Alert Type | Source(s) | Artifact ContentType |
|------------|-----------|---------------------|
| Earthquake | USGS | `alert/earthquake` |
| Tsunami | NOAA | `alert/tsunami` |
| Severe weather (tornado, hurricane, flood, winter storm, heat, etc.) | NWS | `alert/weather` |
| Wildfire | InciWeb | `alert/wildfire` |
| Air quality | AirNow | `alert/air-quality` |
| Volcanic activity | USGS Volcano | `alert/volcano` |
| Global disaster (multi-type) | GDACS | `alert/disaster` |

### R-004: Severity Classification

Alerts MUST carry CAP-compliant severity levels:

| CAP Severity | Examples | Processing Tier | Proactive Delivery? |
|--------------|----------|----------------|---------------------|
| **Extreme** | Tsunami warning, tornado emergency, M7+ earthquake nearby | `full` | YES — immediate notification |
| **Severe** | Tornado warning, hurricane warning, M5+ earthquake within 100km | `full` | YES — immediate notification |
| **Moderate** | Tornado watch, flood watch, M3+ earthquake within 50km, AQI>150 | `standard` | YES — next digest or sooner |
| **Minor** | Frost advisory, small craft advisory, M2.5-3.0 earthquake | `light` | No — digest only |
| **Unknown** | Unclassified alerts | `light` | No — digest only |

### R-005: Metadata Preservation

Each alert MUST carry:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `alert_id` | Source-specific ID | `string` | Dedup key |
| `source` | API source name | `string` | Source tracking (usgs, nws, noaa, etc.) |
| `event_type` | Alert event type | `string` | Classification (earthquake, tornado watch, etc.) |
| `severity` | CAP severity level | `string` | Severity classification |
| `certainty` | CAP certainty | `string` | Observed, likely, possible, unlikely |
| `urgency` | CAP urgency | `string` | Immediate, expected, future, past |
| `headline` | Alert headline | `string` | Brief human-readable summary |
| `description` | Full description | `string` | Detailed alert text |
| `instruction` | Safety instructions | `string` | Recommended actions |
| `effective` | Alert start time | `string` (ISO 8601) | Active period start |
| `expires` | Alert end time | `string` (ISO 8601) | Active period end |
| `status` | Alert status | `string` | active, updated, cancelled, expired |
| `latitude` | Event latitude | `float` | Event location |
| `longitude` | Event longitude | `float` | Event location |
| `distance_km` | Distance to user | `float` | Proximity to nearest configured location |
| `nearest_location` | Closest user location | `string` | Which configured location is nearest |
| `magnitude` | Earthquake magnitude | `float` | For earthquake alerts |
| `depth_km` | Earthquake depth | `float` | For earthquake alerts |
| `aqi_value` | Air quality index | `int` | For air quality alerts |
| `affected_zones` | NWS zone codes | `[]string` | For weather alerts |

### R-006: Dedup Strategy

- **Dedup key:** Source + alert_id (each agency assigns unique identifiers)
  - USGS: earthquake event ID (e.g., `us7000abcd`)
  - NWS: alert ID (e.g., `urn:oid:2.49.0.1.840.0.abc`)
  - GDACS: event ID (e.g., `EQ1234`)
- On each sync, compare incoming alerts against previously stored alerts
- If an alert is updated (same ID, new content), update the artifact content and set status to `updated`
- If an alert is cancelled, set status to `cancelled` and mark as expired
- Expired alerts (past `expires` timestamp) transition to `expired` status
- Historical alerts are retained for queries but excluded from active notification

### R-007: Alert Lifecycle

```
States: active → updated → expired
                → cancelled

Transitions:
- New alert from API → active
- Same alert_id with changed content → updated (preserve history)
- Same alert_id with "Cancel" status → cancelled
- Current time past expires timestamp → expired
- Expired/cancelled alerts → retained in store, excluded from active notifications
```

### R-008: Proximity Calculation

For each alert, calculate distance to all configured user locations:

- Use Haversine formula for great-circle distance
- Filter alerts outside the configured radius for each location
- Include distance and nearest location name in metadata
- For area-based alerts (NWS zones), use zone centroid for distance calculation
- For travel destinations, use destination coordinates with expanded radius (2x normal)

### R-009: Proactive Delivery

High-severity alerts bypass the normal digest cycle:

- Extreme and Severe alerts → publish to notification subject on NATS for immediate delivery
- Moderate alerts → include in next digest generation cycle
- Minor alerts → include in daily digest only
- Notification subject: `alerts.notify` (new NATS subject for time-sensitive alerts)
- Notification payload includes alert headline, severity, distance, and recommended actions

---

## Business Scenarios

### BS-001: Nearby Earthquake Alert

A M4.2 earthquake occurs 35 miles from the user's home. The USGS API reports it within 2 minutes. At the next 5-minute poll cycle, the connector ingests the event as a `full` tier artifact with severity "moderate" (M4+ within 50km). The daily digest includes: "🌍 M4.2 earthquake — 35 mi from Home, 12:47 PM today."

### BS-002: Travel Destination Warning

The user has a flight to Miami in 5 days (detected from calendar). A hurricane watch is issued for South Florida. The NWS alert connector picks this up at the next 10-minute poll. The alert is classified as "severe" and triggers a proactive notification: "⚠️ Hurricane Watch issued for Miami area — you have travel planned in 5 days."

### BS-003: Historical Earthquake Query

The user asks: "Were there any earthquakes when I was in LA last October?" The knowledge graph has Maps timeline data showing the user was in LA Oct 10-15. The connector's historical alert data shows a M3.1 earthquake on Oct 12. The search returns: "M3.1 earthquake, 22 mi from your location in LA, Oct 12."

### BS-004: Wildfire Air Quality Impact

A wildfire 80 miles from the user produces smoke that pushes AQI to 175 (Unhealthy). The air quality source detects the spike. The daily digest includes: "⚠️ Air Quality Unhealthy (AQI 175) — wildfire smoke from [incident name]."

### BS-005: Multi-Source Alert Correlation

A M7.2 earthquake occurs in the Pacific. Within minutes: USGS reports the earthquake, NOAA issues a tsunami advisory for the Pacific coast, NWS issues a coastal flood watch. The connector ingests all three as linked artifacts (same event, different alert types) and delivers a combined notification.

---

## Gherkin Scenarios

```gherkin
Scenario: SCN-GA-001 Ingest USGS earthquake within configured radius
  Given the user has location "Home" at latitude 37.77, longitude -122.42, radius 200km
  And the USGS API returns an earthquake:
    | Field | Value |
    | id | us7000test |
    | magnitude | 4.2 |
    | latitude | 37.50 |
    | longitude | -122.10 |
    | time | 2026-04-09T12:47:00Z |
    | depth | 8.5 |
  When the connector polls USGS
  Then a RawArtifact is created with content_type "alert/earthquake"
  And metadata["magnitude"] is 4.2
  And metadata["distance_km"] is approximately 40
  And metadata["nearest_location"] is "Home"
  And metadata["severity"] is "moderate"
  And processing_tier is "standard"

Scenario: SCN-GA-002 Filter out distant earthquakes
  Given the user has location "Home" at latitude 37.77, longitude -122.42, radius 200km
  And the USGS API returns an earthquake at latitude 20.0, longitude -155.0 (Hawaii)
  When the connector evaluates proximity
  Then the alert is filtered out (distance > 200km from all configured locations)
  And no artifact is created

Scenario: SCN-GA-003 NWS severe weather alert with proactive delivery
  Given the user has location "Home" in NWS zone CAZ006
  And NWS issues a Tornado Warning for zone CAZ006 with severity "Extreme"
  When the connector polls NWS
  Then a RawArtifact is created with content_type "alert/weather"
  And metadata["severity"] is "extreme"
  And processing_tier is "full"
  And the alert is published to "alerts.notify" for immediate notification

Scenario: SCN-GA-004 Alert lifecycle: active → updated → expired
  Given an active alert with id "urn:oid:test123" effective until 18:00
  When NWS updates the alert with extended expiration to 21:00
  Then the alert status transitions to "updated"
  And the expires field is updated to 21:00
  When the current time passes 21:00
  Then the alert status transitions to "expired"
  And the alert is excluded from active notifications
  And the alert is retained in the store for historical queries

Scenario: SCN-GA-005 Travel destination alert enrichment
  Given the user has a calendar event "Flight to Miami" in 5 days
  And NWS issues a Hurricane Watch for Miami-Dade County
  When the connector evaluates travel destinations
  Then the alert is matched to the upcoming travel destination
  And metadata["nearest_location"] is "Travel: Miami"
  And the alert triggers proactive notification

Scenario: SCN-GA-006 Dedup across consecutive polls
  Given the connector previously ingested earthquake "us7000test" at 12:47
  When the next poll returns the same earthquake "us7000test"
  Then no duplicate artifact is created
  And the existing artifact is not reprocessed

Scenario: SCN-GA-007 Multi-source correlation for same event
  Given a major Pacific earthquake triggers:
    | Source | Alert Type |
    | USGS | earthquake M7.2 |
    | NOAA | tsunami advisory |
    | NWS | coastal flood watch |
  When the connector ingests all three alerts
  Then 3 separate RawArtifacts are created
  And all 3 share temporal and geographic proximity metadata
  And the knowledge graph can link them as related events
```
