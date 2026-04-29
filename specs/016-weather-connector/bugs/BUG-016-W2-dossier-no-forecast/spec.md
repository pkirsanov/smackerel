# Feature: [BUG-016-W2] Trip dossiers do not include destination forecasts

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Sibling Bug:** [BUG-016-W1-digest-no-weather](../BUG-016-W1-digest-no-weather/) (same parent contract, different consumer)
> **Date:** April 26, 2026
> **Status:** Draft — bugfix-fastlane

---

## Problem Statement

The parent feature `016-weather-connector` ships a working weather connector that produces `weather/current` and `weather/forecast` artifacts and serves a `weather.enrich.request` NATS subject. However, the **trip dossier assembler never consumes any of that forecast data**. As a result, parent spec.md Outcome Contract Success Signal #3 — "a trip dossier for an upcoming flight includes destination weather" — is unmet for every detected trip.

Verified failure (parent uservalidation.md Remediation Goal for BUG-016-W2, plus direct code inspection 2026-04-26):

- `internal/intelligence/people.go::TripDossier` (lines 16–29) declares fields for `FlightArtifacts`, `HotelArtifacts`, `PlaceArtifacts`, `RelatedCaptures`, `DossierText`, `GeneratedAt` — no `DestinationForecast` / `Weather` field.
- `assembleDossierText` (line 139) renders `Trip to <dest>`, flight count, lodging count, related-captures count — no forecast section.
- `DetectTripsFromEmail` (lines 31–104) issues no `weather.enrich.request`, performs no `weather/forecast` artifact query, and does not consult any geocode/lookup path.
- `grep -n 'weather\|Weather\|forecast' internal/intelligence/people.go` → 0 matches.
- The parent feature's Change Boundary excluded `internal/intelligence/`, so no scope of the parent feature ever planned the integration despite the spec promising it.

This bug closes the gap between the parent Outcome Contract and the dossier implementation. Daily-digest weather integration (Success Signal #1) is tracked separately and already shipping under `BUG-016-W1-digest-no-weather`.

---

## Outcome Contract

**Intent:** Wire `internal/intelligence/people.go::DetectTripsFromEmail` and `assembleDossierText` to consume forecast data so that, for any detected upcoming trip whose destination matches a forecast-able location, the rendered trip dossier text includes a destination forecast — without changing how the connector itself produces or stores weather artifacts and without inventing new schemas.

**Success Signal:** When (a) `DetectTripsFromEmail` produces a `TripDossier` whose `State == "upcoming"` and `Destination` is non-empty, (b) at least one fresh `weather/forecast` artifact exists in the database whose title references that destination within the dossier's lookback window, then the resulting `TripDossier` carries a populated `DestinationForecast *DossierForecast`, and the rendered `DossierText` contains a forecast section (e.g. `🌤️ Forecast: Berlin — Mon 8°C light rain / Tue 11°C cloudy / Wed 14°C clear`).

**Hard Constraints:**

- Read-only consumption of weather data — the dossier assembler MUST NOT mutate, delete, or republish weather artifacts.
- Boundary discipline — implementation changes are confined to `internal/intelligence/` and read-only consumption of existing exports from `internal/connector/weather/`. No edits inside the connector package, no schema changes to `weather/forecast` artifacts, no new NATS subjects.
- The dossier assembler MUST NOT crash, error out, or block when forecast data is unavailable (no destination match, geocode/lookup miss, query timeout, no fresh artifacts) — the dossier MUST render gracefully without a forecast section OR with an explicit `🌤️ Forecast unavailable` marker.
- Test environments MUST use ephemeral fixtures and clean up after themselves (no residual rows in `artifacts`).
- All changes MUST keep the existing `DetectTripsFromEmail` deterministic-ordering contract intact (the `sort.Slice` by destination at line 99).

**Failure Condition:** If, after this fix, an upcoming `TripDossier` whose `Destination` matches a fresh `weather/forecast` artifact in the database renders without a forecast section AND without an explicit "forecast unavailable" marker, the bug is NOT fixed.

---

## Goals

1. **Dossier forecast field** — Add a `DestinationForecast *DossierForecast` field to `TripDossier` and populate it inside `DetectTripsFromEmail` from existing `weather/forecast` artifacts.
2. **Graceful absence** — Whenever forecast data cannot be assembled for a destination, the dossier renders gracefully (no forecast section OR an explicit "forecast unavailable" marker) and `DetectTripsFromEmail` does not return an error.
3. **Rendered marker** — `assembleDossierText` emits a `🌤️ Forecast:` line when forecast data is present; otherwise emits no forecast section.
4. **Adversarial regression coverage** — Tests cover three cases: forecast present (must render), destination missing or geocode-fail (graceful — no crash, no forecast section), forecast-query-failure (graceful — explicit "forecast unavailable" marker OR clean skip).

---

## Non-Goals

- Implementing weather collection — the connector already does this.
- Daily-digest weather integration (covered by `BUG-016-W1-digest-no-weather`).
- New NATS subjects, schema changes, or new artifact types.
- Geocoding arbitrary free-text destinations through a third-party service — this fix uses the destination string already extracted by `extractDestination` and matches against existing forecast artifact titles. If the destination cannot be matched against an existing forecast artifact, the fix renders the graceful "unavailable" path.
- Backfilling old dossiers with forecast data.
- UI rendering changes outside the dossier text pipeline.

---

## Requirements

| # | Requirement |
|---|-------------|
| R1 | `TripDossier` exposes a `DestinationForecast *DossierForecast` field with JSON tag `destination_forecast,omitempty`. |
| R2 | `DetectTripsFromEmail` populates `dossier.DestinationForecast` for each `State == "upcoming"` dossier from fresh `weather/forecast` artifacts whose title references the dossier's `Destination`. |
| R3 | When no matching forecast artifact exists OR the destination is empty, `dossier.DestinationForecast` MUST remain `nil` and `DetectTripsFromEmail` MUST NOT return an error attributable to the missing forecast data. |
| R4 | When the forecast assembly path fails (DB error, decode failure, timeout), the failure is logged via `slog.Warn` with key `"forecast"` and the dossier still renders — same pattern as the existing `slog.Warn` calls in `DetectTripsFromEmail` / `GetPeopleIntelligence`. |
| R5 | `assembleDossierText` renders a `🌤️ Forecast:` line listing up to three day-summaries when `dossier.DestinationForecast` is present and non-empty; renders nothing forecast-related when nil. |
| R6 | A pre-fix HEAD build MUST FAIL the regression test that asserts the rendered dossier text contains `🌤️ Forecast` when seeded forecast fixtures match the destination — i.e. the test cannot be tautological. |
| R7 | All regression tests MUST be free of bailout patterns — no `if condition { return; }` early-exit paths that silently pass when the failure condition is hit. |

---

## User Scenarios (Gherkin)

```gherkin
Feature: Trip dossier includes destination forecast when forecast data exists

  Scenario: SCN-BUG016W2-001 Forecast present — dossier renders forecast section
    Given an upcoming TripDossier with Destination = "Berlin"
    And a fresh weather/forecast artifact exists with title referencing "Berlin"
    When DetectTripsFromEmail assembles the dossier
    Then dossier.DestinationForecast is non-nil
    And dossier.DestinationForecast.Days has at least one day
    And the rendered DossierText contains a "🌤️ Forecast" section that names the destination

  Scenario: SCN-BUG016W2-002 Destination missing or geocode-fail — dossier renders gracefully without crash
    Given an upcoming TripDossier whose Destination is empty OR cannot be matched to any forecast artifact
    When DetectTripsFromEmail assembles the dossier
    Then dossier.DestinationForecast is nil
    And DetectTripsFromEmail returns no error attributable to the missing forecast
    And the rendered DossierText contains no "🌤️ Forecast" section

  Scenario: SCN-BUG016W2-003 Forecast query failure — dossier emits with explicit unavailable marker OR skips section gracefully
    Given an upcoming TripDossier with a non-empty Destination
    And the weather/forecast artifact query fails or times out
    When DetectTripsFromEmail assembles the dossier
    Then dossier.DestinationForecast is nil
    And the failure is logged via slog.Warn with key "forecast"
    And DetectTripsFromEmail returns without error
    And the rendered DossierText either contains "🌤️ Forecast unavailable" OR contains no "🌤️ Forecast" section at all

  Scenario: SCN-BUG016W2-004 Adversarial — pre-fix HEAD fails the present-case assertion
    Given the pre-fix HEAD build of internal/intelligence/people.go
    And fresh weather/forecast artifacts seeded in the DB whose titles match the dossier destination
    When the regression test from SCN-BUG016W2-001 runs against that build
    Then the test FAILS because TripDossier has no DestinationForecast field AND assembleDossierText emits no forecast marker
```

---

## Acceptance Criteria

- [ ] All four scenarios above pass against the post-fix build.
- [ ] SCN-BUG016W2-004 fails against the pre-fix HEAD (recorded in `report.md`).
- [ ] Parent `uservalidation.md` item "Trip dossiers can include destination forecasts" can be re-run by `bubbles.validate` and verified PASS.
- [ ] Parent `spec.md` Outcome Contract Success Signal #3 ("a trip dossier for an upcoming flight includes destination weather") is met without amending the parent spec.
- [ ] No residual rows in the `artifacts` table after integration / e2e test runs.
