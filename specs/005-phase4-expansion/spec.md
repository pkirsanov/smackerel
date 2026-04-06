# Feature: 005 — Phase 4: Expansion (Maps + Browser + Trips + People Intelligence)

> **Parent Spec:** [specs/001-smackerel-mvp](../001-smackerel-mvp/spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Phase:** 4 of 5
> **Depends On:** Phase 2 (Passive Ingestion)
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Phases 1-3 cover the digital knowledge layer — emails, articles, videos, calendar. But a significant portion of a person's life happens in the physical world: places visited, routes driven, hikes taken, trips planned. And the People graph from Phase 2 only knows names and email addresses — it doesn't understand relationship dynamics.

Phase 4 expands Smackerel's awareness into physical space (location history, trip assembly, trail journals) and social depth (interaction patterns, relationship cooling, gift-list memory). This is where the system starts to feel like it truly knows your life, not just your reading list.

---

## Outcome Contract

**Intent:** Smackerel passively tracks the user's location history and browser activity, automatically assembles trip dossiers from scattered artifacts (flight emails, hotel confirmations, saved restaurants, calendar events), records hiking/driving routes as searchable trail journals, and builds deep people intelligence showing interaction patterns, relationship health, and personal context.

**Success Signal:** The user has a Berlin trip coming up in 5 days. Without asking, Smackerel delivers a complete dossier: flight details (from email), hotel confirmation (from email), 3 saved restaurants (from active captures), a walking tour article (from browser history), weather forecast, and the colleague they're meeting (from calendar). Separately, the system notices the user hasn't interacted with Alex in 6 weeks — they used to communicate weekly.

**Hard Constraints:**
- Location data is opt-in — never enabled by default
- Browser history is opt-in — never enabled by default
- Location URLs stored only for intentional content (articles, places); navigation/social URLs stored as domain-level aggregates only
- No face recognition in photos (metadata only)
- People intelligence is observational — never automated outreach
- Trip dossiers assemble from existing artifacts — no external booking or reservation systems

**Failure Condition:** If trip dossiers miss obvious components (flight confirmation email exists but doesn't appear in the dossier), or if location data is enabled by default without explicit opt-in, or if people intelligence generates false relationship alerts — this phase has failed.

---

## Goals

1. Implement Google Maps Timeline connector for location history, routes, and place memory
2. Implement browser history connector (opt-in) for deep-interest signal detection
3. Build trip dossier auto-assembly from cross-source artifacts (email, calendar, captures, maps)
4. Build trail/route journal for hiking, cycling, and driving routes
5. Build people intelligence: interaction frequency, relationship radar, context aggregation
6. Link location context to captured artifacts (where was the user when they saved this?)

---

## Non-Goals

- Real-time location tracking or geofencing
- Face recognition in photos
- Social media integration
- Automated outreach or message sending
- Navigation or routing functionality
- Hotel/restaurant booking integration
- Photo content analysis beyond metadata (Phase 5 consideration)
- Podcast/audiobook connectors (post-MVP)

---

## Requirements

### R-401: Google Maps Timeline Connector
- **API:** Google Maps Timeline export (Takeout) or Location History API
- **Schedule:** Daily at 2 AM (configurable)
- **Opt-in required:** Not enabled by default; user must explicitly enable in Settings
- **Scope:** Location history, saved places, routes, timeline activities
- **Source Qualifiers:**
  - Activity type: driving, walking, cycling, transit, flight
  - Duration at location
  - Frequency of visits to a location
  - Starred/saved places
- **Processing per activity:**
  - Activity type classification (drive, walk, cycle, transit, flight)
  - Route/path (polyline) with start/end locations
  - Duration and distance (calculated from polyline)
  - Elevation data for trails (elevation API)
  - Weather conditions at the time (optional, via weather API)
  - Nearby captures: link to artifacts captured during this time/location window
- **Dedup:** Date + location cluster hash
- **Privacy:** Route data stored locally; no external location sharing

### R-402: Browser History Connector (Opt-in)
- **Source:** Browser extension or Chrome history API
- **Schedule:** Every 4 hours (configurable)
- **Opt-in required:** Not enabled by default; explicit consent with privacy explanation
- **Scope:** Visited URLs with dwell time
- **Source Qualifiers:**
  - Dwell time: >3 min = intentional reading (trigger processing)
  - Repeat visits to same URL = deep interest signal
  - Bookmarks → full processing
  - Domain categorization (news, reference, shopping, social, internal tools)
- **Processing Rules:**
  - Dwell >3 min: fetch content via readability, extract summary, tag topics
  - Frequent revisits: detect deep-interest topics, boost topic momentum
  - Bookmarks: full pipeline processing
  - Skip: social media feeds, search result pages, internal tools (configurable skip-list)
- **Privacy:**
  - Full URLs stored only for articles/content pages
  - Navigation/social URLs stored as domain-level aggregates only (e.g., "twitter.com: 45 min, 12 visits")
  - User can view and delete any stored URL
- **Dedup:** URL + date

### R-403: Trip Dossier Auto-Assembly
- **Trip Detection:**
  - Flight confirmation email detected (airline + confirmation code + dates)
  - Hotel/accommodation booking email detected
  - Calendar event with travel location
  - User explicitly creates a trip: "Berlin trip May 12-18"
- **Dossier Assembly:**
  - Aggregate all trip-related artifacts: flights, hotels, restaurants, activities, articles, maps routes
  - Cross-reference by: destination city/country, date range, related People
  - Structure:
    ```
    ✈️ Flights: carrier, route, dates, confirmation
    🏨 Accommodation: name, dates, confirmation, address
    🍽️ Restaurants: saved places with notes and sources
    🏛️ Activities: articles, recommendations, saved places
    📍 Routes: any planned or suggested routes
    👤 People: contacts meeting at destination (from calendar)
    🌤️ Weather: typical conditions for dates/destination
    ```
- **Proactive Surfacing:**
  - 5 days before trip: deliver dossier via configured channel
  - Include restaurant recommendations saved months earlier that match the destination
  - Include articles about the destination saved at any time
- **Trip States:** upcoming → active → completed
- **Post-Trip:** Link Maps timeline activities to the trip (routes, places visited)

### R-404: Trail/Route Journal
- **Source:** Maps Timeline activities of type: walking (long), cycling, hiking
- **Qualification:** Walking >2 km or >30 min, cycling >5 km, driving routes explicitly saved
- **Recorded Per Trail:**
  - Route polyline with GPS coordinates
  - Start/end locations with names
  - Total distance, duration
  - Elevation gain/loss (for hiking/cycling)
  - Date and time
  - Weather conditions at the time
  - Photos taken during the route (metadata only — timestamps + location)
  - Nearby artifacts captured during the trail time window
- **Searchable:**
  - "Show me all hikes this year" → list with distances, dates, map links
  - "Trails near Lisbon" → location-filtered routes
  - "My longest bike ride" → sorted by distance
  - "That hike where I saw the waterfall" → search captures linked to trails

### R-405: People Intelligence
- **Data Sources:**
  - Email interaction frequency and recency (from Gmail connector)
  - Calendar meeting frequency and patterns (from Calendar connector)
  - Artifact mentions (entity extraction across all artifacts)
  - Explicit user notes about people
  - Recommendations made by people ("Sarah recommended...")
- **Per-Person Profile:**
  - Name, aliases, email, organization
  - Interaction timeline: email count by month, meeting count by month
  - Interaction trend: increasing / stable / decreasing / lapsed
  - Shared topics: topics where both user and person have artifacts
  - Recommendations: things this person recommended
  - Pending items: things the user owes them or they owe the user
  - Last interaction date and type (email/meeting/mention)
  - Important dates: detected from email context (birthdays, anniversaries — if mentioned)
  - Personal notes: user-added context ("Met at conference", "Likes Italian food")
- **Relationship Radar:**
  - Detect significant interaction frequency changes
  - Alert when a regular contact goes quiet: "You haven't interacted with Alex in 6 weeks — you used to talk weekly. Reach out?"
  - Track relationship patterns: "You interact with David most before quarterly reviews"
  - Gift list: track when someone mentions wanting something ("Alex wanted the Ottolenghi cookbook in March") → surface before their birthday if known
- **Privacy:** All analysis is observational. No automated outreach. User can delete any person's data.

### R-406: Location-Aware Captures
- When a capture happens and the user's device provides location:
  - Tag the artifact with location context: {lat, lng, name}
  - Link to nearby place entities
  - Link to active trail/route if timing overlaps
- When user visits a location near a previously saved place:
  - Optional: "You saved a coffee shop 200m from here — you've been here 3 times and never went" (configurable, opt-in)

### R-407: Source Privacy Controls
- Settings page must clearly distinguish opt-in sources (Maps, Browser) from standard sources
- Privacy explanation shown before enabling: what data is collected, how it's stored, what's shared
- Per-source data deletion: "Delete all browser history data" removes all artifacts from that source
- Domain-level skip list for browser history (user-configurable)
- Location data granularity control: full precision / city-level / disabled

---

## User Scenarios (Gherkin)

### Trip Dossier

```gherkin
Scenario: SC-E01 Automatic trip detection from email
  Given the system processes an email with a Lufthansa flight confirmation LH456 to Berlin on May 12
  And processes a Booking.com hotel confirmation for Memmo Berlin, May 12-18
  When the trip detection engine runs
  Then it creates a "Berlin Trip" entity with dates May 12-18
  And links the flight and hotel artifacts to the trip
  And the trip state is "upcoming"

Scenario: SC-E02 Trip dossier aggregation
  Given a "Berlin Trip" exists for May 12-18
  And the user saved 2 restaurant recommendations tagged with "Berlin" 3 months ago
  And the user saved an article "Best Walking Tours in Berlin" from browser history
  And a calendar event shows "Lunch with Hans" on May 14 in Berlin
  When the dossier assembly runs
  Then the dossier includes: flight, hotel, 2 restaurants, walking tour article, meeting with Hans
  And Hans is linked as a trip contact

Scenario: SC-E03 Trip dossier proactive delivery
  Given the Berlin trip is 5 days away
  When the trip prep alert fires
  Then the user receives the full dossier via their configured channel
  And the dossier includes weather forecast for Berlin in May

Scenario: SC-E04 Post-trip route linking
  Given the Berlin trip is completed (May 18 has passed)
  And Maps Timeline shows 3 walking routes in Berlin during May 12-18
  When the Maps connector syncs
  Then the 3 routes are linked to the Berlin trip
  And the trip state transitions to "completed"
  And the routes are searchable as part of the trip

Scenario: SC-E05 Trip from explicit user creation
  Given the user types "Trip: Lisbon, June 1-7" via the capture channel
  When the system processes this input
  Then it creates a "Lisbon Trip" entity with dates
  And begins aggregating any existing Lisbon-tagged artifacts
```

### Trail Journal

```gherkin
Scenario: SC-E06 Hike recorded from Maps Timeline
  Given the Maps Timeline shows a walking activity on March 15
  And the route is 8.5 km, duration 2:30, elevation gain 450m
  When the Maps connector processes this activity
  Then it creates a trail entry with route polyline, distance, duration, elevation
  And records weather conditions for that date/location
  And links any artifacts captured during the hike time window

Scenario: SC-E07 Search trails by criteria
  Given the user has recorded 12 trails this year
  When the user searches "all hikes this year"
  Then the system returns all 12 trails sorted by date
  And each result shows: date, location, distance, duration, elevation

Scenario: SC-E08 Search trails by location
  Given the user has done 3 hikes near Sintra, Portugal
  When the user searches "trails near Sintra"
  Then the system returns the 3 Sintra trails
  And includes route details and dates

Scenario: SC-E09 Trail with linked captures
  Given the user took photos and saved a note during a hike
  When viewing the trail entry
  Then the linked photos (metadata) and note appear as part of the trail record
```

### Browser History

```gherkin
Scenario: SC-E10 Deep reading detection
  Given the user visits an article about "Microservices Patterns" and stays for 12 minutes
  When the browser history sync processes this visit
  Then the article is fetched and processed through the full pipeline
  And linked to the "microservices" topic
  And the topic's momentum score is updated

Scenario: SC-E11 Frequent revisit detection
  Given the user has visited the Go documentation page 8 times this month
  When the browser history sync detects this pattern
  Then the system notes "Go programming" as a deep-interest signal
  And boosts the Go topic momentum

Scenario: SC-E12 Social media aggregation only
  Given the user spent 45 minutes on twitter.com across 12 visits today
  When the browser history sync processes this
  Then it stores only domain-level aggregate: "twitter.com: 45 min, 12 visits"
  And does NOT store individual tweet URLs or content

Scenario: SC-E13 Skip list enforcement
  Given the user has configured "internal-tool.company.com" in the skip list
  When the browser history sync encounters visits to that domain
  Then all visits are skipped entirely
  And no artifacts are created
```

### People Intelligence

```gherkin
Scenario: SC-E14 Relationship cooling detection
  Given the user used to interact with Alex weekly (emails + meetings)
  And the last interaction with Alex was 6 weeks ago
  When the people intelligence engine detects this change
  Then it sends a soft prompt: "You haven't interacted with Alex in 6 weeks — you used to talk weekly. Reach out?"
  And the prompt respects the max 2 alerts/day limit

Scenario: SC-E15 Gift list from conversation capture
  Given Sarah mentioned in an email "I've been wanting that Ottolenghi cookbook"
  And the system processed this email and linked it to Sarah's People entity
  When Sarah's birthday approaches (if known)
  Then the system surfaces: "Sarah mentioned wanting the Ottolenghi cookbook in March"

Scenario: SC-E16 Person interaction summary
  Given the user searches "person David Kim"
  When the system retrieves David's People profile
  Then it shows: email count (by month), meeting count, shared topics, pending commitments
  And interaction trend: "Increasing — 2x more emails this month vs. last"
  And last 5 artifacts that mention David

Scenario: SC-E17 Meeting pattern detection
  Given the user has a recurring weekly 1:1 with Sarah
  And a monthly team sync with 8 attendees
  When the calendar connector detects these patterns
  Then Sarah's profile shows "Weekly 1:1, ongoing"
  And the team sync participants are all linked as team contacts

Scenario: SC-E18 Commute pattern detection
  Given the Maps Timeline shows the user drives to the same location every weekday
  And on Tuesdays the commute is consistently 20 minutes longer
  When the location intelligence engine detects this pattern
  Then it notes: "Your Tuesday commute averages 20 min longer for the past month"
  And surfaces this in the weekly synthesis under PATTERNS NOTICED
```

### Location-Aware Captures

```gherkin
Scenario: SC-E19 Capture with location context
  Given the user captures an article while sitting in a coffee shop in Lisbon
  And the device provides location data
  When the artifact is stored
  Then it is tagged with the coffee shop location
  And linked to any "Lisbon" trip or place entities

Scenario: SC-E20 Nearby saved place notification
  Given the user saved a coffee shop "Fabrica Coffee Roasters" in Lisbon
  And the user has been within 200m of this location 3 times (from Maps Timeline)
  When the proximity detection runs
  Then it optionally notes: "You saved Fabrica Coffee Roasters — you've been nearby 3 times"
  And this is a configurable, opt-in notification
```

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-E01 | Trip auto-detected from flight confirmation email | SC-E01 | Integration |
| AC-E02 | Trip dossier aggregates artifacts from email, captures, browser, calendar | SC-E02 | E2E |
| AC-E03 | Trip dossier delivered 5 days before departure | SC-E03 | E2E |
| AC-E04 | Post-trip routes linked from Maps Timeline | SC-E04 | Integration |
| AC-E05 | Explicit trip creation via text input | SC-E05 | Integration |
| AC-E06 | Hike recorded with full route, distance, elevation, weather | SC-E06 | Integration |
| AC-E07 | Trail search returns all trails sorted by date | SC-E07 | E2E |
| AC-E08 | Location-filtered trail search works | SC-E08 | E2E |
| AC-E09 | Captures during a hike are linked to the trail entry | SC-E09 | Integration |
| AC-E10 | Deep reading (>3 min dwell) triggers full processing | SC-E10 | Integration |
| AC-E11 | Frequent revisits boost topic momentum | SC-E11 | Integration |
| AC-E12 | Social media stored as domain aggregate only, no individual URLs | SC-E12 | Unit |
| AC-E13 | Skip list domains produce zero artifacts | SC-E13 | Unit |
| AC-E14 | Relationship cooling detected and alert sent | SC-E14 | Integration |
| AC-E15 | Gift list items surfaced near person's birthday | SC-E15 | Integration |
| AC-E16 | Person search returns full interaction profile | SC-E16 | E2E |
| AC-E17 | Meeting patterns detected and shown on People profile | SC-E17 | Integration |
| AC-E18 | Commute pattern changes detected and reported | SC-E18 | Integration |
| AC-E19 | Captures tagged with location context | SC-E19 | Integration |
| AC-E20 | Nearby saved place detection works (opt-in only) | SC-E20 | Integration |

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Enable Maps connector | Self-Hoster | Settings → Sources | 1. Click "Enable Maps" 2. Read privacy notice 3. Confirm opt-in 4. Connect | Maps syncing, clear opt-in recorded | Settings |
| Enable browser history | Self-Hoster | Settings → Sources | 1. Click "Enable Browser" 2. Read privacy notice 3. Configure skip list 4. Confirm | Browser syncing with skip list active | Settings |
| View trip dossier | Solo User | Web UI → Trips | 1. Click upcoming trip | Full dossier: flights, hotel, restaurants, activities, weather, contacts | Trip dossier view |
| Browse trails | Solo User | Web UI → Trails | 1. View trail list | All trails with date, location, distance, elevation | Trail browser |
| View trail detail | Solo User | Trails → click trail | 1. Click trail | Route map, stats, weather, linked captures | Trail detail |
| View person profile | Solo User | Web UI → People → person | 1. Click person | Interaction timeline, shared topics, pending items, recommendations | Person profile |
| Configure skip list | Self-Hoster | Settings → Browser | 1. Add domains to skip | Listed domains produce no artifacts | Settings |
| Delete source data | Self-Hoster | Settings → Source → Delete | 1. Click "Delete all data from source" 2. Confirm | All artifacts from that source removed | Settings |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| Maps sync latency | < 24 hours from activity to processed | Daily sync at 2 AM is sufficient |
| Browser sync latency | < 4 hours from visit to processed | 4-hour poll cycle |
| Trip dossier assembly | < 30 sec to aggregate all linked artifacts | Fast response when user opens dossier |
| Trail route rendering | Route polyline stored as standard GeoJSON | Compatible with any mapping library |
| People profile load time | < 2 sec including interaction timeline | Acceptable UX for profile views |
| Opt-in enforcement | Maps and Browser NEVER auto-enabled | Privacy-by-default is non-negotiable |
| Location data granularity | Configurable: full / city / disabled | User controls precision |
| Browser URL privacy | Social/navigation URLs as domain aggregates only | Never store individual social media URLs |
