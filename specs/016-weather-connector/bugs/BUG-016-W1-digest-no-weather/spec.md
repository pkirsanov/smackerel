# Feature: [BUG-016-W1] Daily digest does not include weather data

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Date:** April 26, 2026
> **Status:** Draft — bugfix-fastlane

---

## Problem Statement

The parent feature `016-weather-connector` ships a working weather connector that produces `weather/current` and `weather/forecast` artifacts and serves a `weather.enrich.request` NATS subject. However, the **daily digest generator never consumes any of that weather data**. As a result, parent spec.md Outcome Contract Success Signal #1 — "the daily digest includes current conditions and a 3-day forecast" — is unmet whenever a user has a configured home location and the connector is healthy.

Verified failure (uservalidation.md, 2026-04-26):

- `grep -r 'weather\|Weather\|forecast' internal/digest/` → 0 matches.
- `internal/digest/generator.go::DigestContext` (lines 30–43) declares fields for `ActionItems`, `OvernightArtifacts`, `HotTopics`, `Hospitality`, `KnowledgeHealth`, `Expenses` — no `Weather` field.
- `Generate()` performs no weather artifact query and emits no `weather.enrich.request`.
- `scopes.md` "Change Boundary" line 13 explicitly excluded digest handlers, so no scope of the parent feature ever planned the integration despite the spec promising it.

This bug closes the gap between the parent Outcome Contract and the digest implementation. Trip dossier integration (Success Signal #3) is tracked separately under `BUG-016-W2-dossier-no-forecast`.

---

## Outcome Contract

**Intent:** Wire `internal/digest/generator.go` to consume weather data so that a healthy weather connector with a configured home location materialises in the rendered daily digest text — current conditions plus a multi-day forecast — without changing how the connector itself produces or stores weather artifacts.

**Success Signal:** When (a) `config/smackerel.yaml` has at least one configured home location, (b) the weather connector has produced fresh `weather/current` and `weather/forecast` artifacts within the digest's lookback window, and (c) `Generator.Generate(ctx)` runs, then the resulting `DigestContext` carries a populated `Weather *WeatherDigestContext`, the rendered digest text contains a weather section (e.g. `🌤️ Weather: ~22°C, mostly sunny`) plus a 3-day forecast block, and `digest_context.weather` is consumed by the `digest-assembly-v1` ML prompt contract.

**Hard Constraints:**

- Read-only consumption of weather data — the digest generator MUST NOT mutate, delete, or republish weather artifacts.
- Boundary discipline — implementation changes are confined to `internal/digest/`, `config/prompt_contracts/digest-assembly-v1.yaml`, and at most a thin read-only consumer of existing exports from `internal/connector/weather/`. No edits inside the connector package, no schema changes to `weather/current` / `weather/forecast` artifacts, no new NATS subjects.
- The digest generator MUST NOT crash, error out, or block when weather data is unavailable (no home location configured, connector disabled, query times out, no fresh artifacts) — the digest MUST render without a weather section in those cases.
- Test environments MUST use ephemeral fixtures and clean up after themselves (no residual rows in `artifacts`).
- All changes MUST keep the existing digest fallback path (NATS publish failure → `storeFallbackDigest`) functional.

**Failure Condition:** If, after this fix, a user with a configured home location and a healthy weather connector receives a daily digest with no weather section while the database contains fresh `weather/current` / `weather/forecast` artifacts within the lookback window, the bug is NOT fixed.

---

## Goals

1. **Digest weather context** — Add a `Weather *WeatherDigestContext` field to `DigestContext` and populate it inside `Generator.Generate(ctx)` from existing weather artifacts.
2. **Graceful absence** — Whenever weather data cannot be assembled, the digest renders without a weather section and without raising an error.
3. **Prompt contract update** — Extend `config/prompt_contracts/digest-assembly-v1.yaml` so the ML side renders the weather section when `digest_context.weather` is present.
4. **Adversarial regression coverage** — Tests cover three cases: weather present, weather absent (no configured home / no artifacts), and weather query failure / timeout.

---

## Non-Goals

- Implementing weather collection — the connector already does this.
- Trip dossier weather (covered by `BUG-016-W2-dossier-no-forecast`).
- New NATS subjects, schema changes, or new artifact types.
- UI rendering changes outside the digest pipeline.
- Backfilling old digests with weather data.

---

## Requirements

| # | Requirement |
|---|-------------|
| R1 | `DigestContext` exposes a `Weather *WeatherDigestContext` field (`omitempty`). |
| R2 | `Generator.Generate(ctx)` populates `digestCtx.Weather` from fresh `weather/current` and `weather/forecast` artifacts when a home location is configured. |
| R3 | When no home location is configured OR no fresh weather artifacts exist, `digestCtx.Weather` MUST remain `nil` and `Generate()` MUST NOT return an error attributable to the missing weather data. |
| R4 | When the weather assembly path fails (DB error, decode failure, timeout), the failure is logged via `slog.Warn` and the digest still renders — same pattern as `getPendingActionItems`, `getOvernightArtifacts`, `getHotTopics`. |
| R5 | The `digest-assembly-v1` prompt contract documents the `weather` payload and instructs the LLM to render a weather section only when `digest_context.weather` is present. |
| R6 | The "quiet day" branch is updated so a digest that contains ONLY weather is NOT classified as quiet. |
| R7 | Regression tests include at least one adversarial case where the broken pre-fix behaviour (`Weather` field absent / never populated) would still pass — i.e. tests assert the rendered digest text contains the weather marker, not just that a struct exists. |

---

## User Scenarios (Gherkin)

```gherkin
Feature: Daily digest includes weather data when configured

  Scenario: SCN-BUG016W1-001 Weather present — digest renders weather section
    Given config/smackerel.yaml has a configured home location
    And the weather connector has produced a fresh weather/current artifact for that location within the last 6 hours
    And the weather connector has produced fresh weather/forecast artifacts covering the next 3 days
    When the digest generator assembles the DigestContext for today
    Then DigestContext.Weather is non-nil and carries the current conditions plus the 3-day forecast
    And the rendered digest text contains a weather section with temperature and conditions

  Scenario: SCN-BUG016W1-002 Weather absent (no home location) — digest renders gracefully
    Given config/smackerel.yaml has no configured home location
    When the digest generator assembles the DigestContext for today
    Then DigestContext.Weather is nil
    And Generate() returns without error
    And the rendered digest text contains no weather section

  Scenario: SCN-BUG016W1-003 Weather query failure — digest renders gracefully with no weather section
    Given config/smackerel.yaml has a configured home location
    And the weather artifact query fails or times out
    When the digest generator assembles the DigestContext for today
    Then DigestContext.Weather is nil
    And the failure is logged via slog.Warn with a "weather" key
    And Generate() returns without error
    And the rendered digest text contains no weather section

  Scenario: SCN-BUG016W1-004 Adversarial — pre-fix code path is detected
    Given a digest assembled by the pre-fix Generate() (no Weather field, no weather query)
    When the regression test runs against that build
    Then the test fails because the rendered digest contains no weather section despite fresh weather artifacts being present in the database
```

---

## Acceptance Criteria

- [ ] All four scenarios above pass against the post-fix build.
- [ ] SCN-BUG016W1-004 fails against the pre-fix HEAD (recorded in `report.md`).
- [ ] Parent uservalidation.md item "Daily digest can include weather data" can be re-run by `bubbles.validate` and verified PASS.
- [ ] Parent spec.md Outcome Contract Success Signal #1 ("the daily digest includes current conditions and a 3-day forecast") is met without amending the parent spec.
