# Spec 094 — Weather Rich Conditions + 10-Day Forecast

**Status:** in_progress
**Workflow mode:** full-delivery · **Status ceiling:** done
**Release train:** `mvp`
**Builds on:** [061-conversational-assistant](../061-conversational-assistant/spec.md) SCOPE-07 (the `weather_lookup` skill, the `weather-query-v1` contract, the open-meteo provider seam, and the provenance/anti-fabrication invariants this spec enhances)
**Relates to:** [065-generic-micro-tools](../065-generic-micro-tools/spec.md) (the `location_normalize` micro-tool that co-resolves a location for the same scenario), [069-assistant-http-transport](../069-assistant-http-transport/spec.md) + [073-web-mobile-assistant-frontend](../073-web-mobile-assistant-frontend/spec.md) (the non-Telegram surfaces that render the assistant `Body`), [016-weather-connector](../016-weather-connector/spec.md) (the SEPARATE background weather *connector* — NOT touched by this spec)

> **Operator directive (verbatim):** *"weather command should return all info, not just temperature - humidity, rains, sun, uv, wind, etc., also 10 days forecast"*

---

## Problem

Smackerel's conversational assistant exposes a `weather` skill (spec 061
SCOPE-07, package [`internal/agent/tools/weather/`](../../internal/agent/tools/weather/)).
Today the skill answers a weather question with a **single terse line**:

```
Barcelona, ES: clear, 18.0°C
```

The open-meteo provider ([`open_meteo.go`](../../internal/agent/tools/weather/open_meteo.go))
requests only `current=temperature_2m,weather_code` and renders
`"<label>: <summary>, <temp>°C"`. The `today` / `tomorrow` / `weekend`
forecast windows are accepted by the contract but currently **fall back to
current conditions** (`_ = window`) because no daily grid is fetched yet.

That answer is missing almost everything a person asks "what's the weather"
to learn:

- **No humidity.** The reading the operator explicitly named first.
- **No precipitation / rain.** Whether to carry an umbrella is the single most
  common reason to check.
- **No "sun".** Sunrise / sunset times — when it gets light and dark.
- **No UV.** Whether sun protection is needed.
- **No wind.** Speed and direction.
- **No feels-like.** Apparent temperature, which can differ from the raw number
  by many degrees in wind or humidity.
- **No multi-day outlook at all.** There is no way to see beyond "right now";
  the operator explicitly wants a **10-day forecast**.

The operator wants the `weather` command to **return all of it** — the full set
of current conditions **and** a 10-day forecast — in one answer, while keeping
the existing provider-neutral, provenance-attributed, anti-fabrication
guarantees that spec 061 established.

---

## Goals

1. **Rich current conditions.** A weather answer reports, for the resolved
   location: condition summary, temperature, **feels-like (apparent)**
   temperature, **humidity**, **precipitation**, **wind** (speed + direction),
   **UV**, and **sun** (sunrise + sunset) — not just temperature.
2. **10-day forecast.** The same answer includes a multi-day outlook —
   **10 days** by default — with, per day: the condition, the high/low
   temperatures, the precipitation outlook (probability), and the UV peak. The
   number of days is **operator-configurable via SST** (no hardcoded constant).
3. **One answer, every surface.** The rich answer renders coherently on every
   assistant transport — Telegram (MarkdownV2), HTTP/web, and mobile — staying
   **skimmable and phone-screen-fit** (Product Principle 7). The user-facing
   `Body` is the rendered multi-line text; a parallel **structured** payload
   (`current` + `daily`) is additionally exposed on the tool contract so a
   native frontend can render its own richer UI without a second round-trip.
4. **Provider-neutral.** All new readings live on the **provider-neutral**
   `Forecast` contract. open-meteo remains *one* provider behind the existing
   `Provider` interface; no open-meteo-specific shape leaks into the tool,
   facade, or contract layer.
5. **Provenance preserved.** Every answer keeps its `provider_name` +
   `retrieved_at` attribution and the spec 061 §5.2 cache-`retrieved_at`
   preservation invariant; the facade still **refuses to fabricate** attribution
   when any required field is missing (anti-fabrication / `requires_provenance`).
6. **Units from SST.** Temperature, wind-speed, and precipitation units are
   **operator-chosen SST values** with **no hardcoded fallback default**
   (smackerel-no-defaults). Missing unit config fails startup loudly.
7. **Zero regression.** The existing routing, the `now` / `today` / `tomorrow`
   / `weekend` windows, the LRU cache, the rate limit, the provider-unavailable
   (`BS-006`) capture path, and the location-missing (`slot_missing: location`)
   path all behave exactly as spec 061 shipped them.

---

## Non-Goals

- **A new weather provider.** open-meteo (keyless) remains the only concrete
  provider. The work keeps the provider seam clean so a second provider *could*
  be added, but adding one is out of scope.
- **Touching the spec 016 background weather *connector*.** `connectors.weather.*`
  (ingestion into the knowledge graph) is a different subsystem with its own
  `forecast_days`. This spec changes **only** `assistant.skills.weather.*` (the
  conversational command).
- **Severe-weather alerts / push notifications.** This is a pull/query answer
  (Principle 6 — invisible by default). Proactive alerting is spec 017's domain.
- **Hourly / minute-by-minute forecasts.** Current conditions + a daily 10-day
  grid is the contract. Sub-daily granularity is not requested.
- **Historical weather.** Past-date lookups are out of scope.
- **Per-user unit preferences.** Units are a single SST choice for the
  deployment, not a per-user setting.
- **Changing the assistant routing / scenario-selection layer.** The
  `weather_query` scenario, its intent examples, and `direct_output_from_tool`
  pass-through are unchanged except for the output-schema widening.

---

## Domain Capability Model

This spec **deepens** the single spec-061 capability — *answer a weather
question from an external provider with attribution* — by enriching its result
primitive. The domain primitive is the **Forecast**, and it gains structure.

### Primitive: Forecast (enriched)

| Field (conceptual) | Meaning | Source |
|--------------------|---------|--------|
| rendered line | The human-readable, multi-line, plain-text answer the user sees. Carries the full current block + the 10-day rows. | rendered from the two blocks below |
| current conditions | condition, temperature, feels-like, humidity, precipitation, wind speed + direction, UV | provider "current" + today's daily UV peak |
| daily outlook (N days) | per day: date, condition, temp high, temp low, precipitation probability, UV peak | provider "daily" grid |
| provider name | attribution — which provider answered | provider identity |
| retrieved at | attribution — wall-clock moment the provider responded; **preserved unchanged across cache hits** | provider response time (cache-preserved) |

### Sub-primitive: Current Conditions

A snapshot for "right now" at the resolved location: condition summary,
temperature, apparent (feels-like) temperature, relative humidity,
precipitation amount, wind speed, wind direction, and the UV peak for the day.

### Sub-primitive: Daily Forecast (one per forecast day)

A compact per-day outlook: the calendar date, condition summary, high and low
temperature, precipitation probability, and the day's UV peak. Sunrise / sunset
for the *current* day are surfaced in the current block ("sun").

### Lifecycle / invariants

- A Forecast is **read-only** and **attributed**: it always carries
  `provider_name` + `retrieved_at`. A Forecast missing any attribution field is
  **never** assembled into a user answer — the provenance gate refuses it and
  the answer routes to the `BS-006` capture path (anti-fabrication).
- A Forecast served from the LRU cache carries the **original** upstream
  `retrieved_at`, never the cache-hit wall clock (spec 061 §5.2).
- The rendered line is **plain text**; each transport adapter applies its own
  escaping (Telegram MarkdownV2 escaping is owned by the adapter, not the
  skill).

---

## Product Principle Alignment

This feature touches retrieval and output-shape principles. Per
[product-principles.instructions.md](../../.github/instructions/product-principles.instructions.md)
this section is **binding**.

- **Principle 2 — Vague In, Precise Out.** A vague "what's the weather in
  Barcelona" yields a precise, complete answer (all conditions + 10-day
  outlook). The user supplies a fuzzy location; the system resolves it and
  returns everything, rather than demanding the user ask for each reading
  separately.
- **Principle 6 — Invisible By Default, Felt Not Heard.** This is a **pull**
  (the user asked). No new proactive notification, no status-update prompt, no
  system-initiated message is introduced. The feature stays within the
  query/answer contract.
- **Principle 7 — Small, Frequent, Actionable Output.** A 10-day forecast risks
  becoming a wall of text. The design keeps it **phone-screen-fit and
  skimmable**: a compact current block plus **one short line per day**, ordered
  most-relevant-first (today at the top). Read time stays well under the digest
  budget; the Telegram per-message budget is honored by the existing
  `budgetTruncate`.
- **Principle 8 — Trust Through Transparency.** Every answer keeps its
  `provider_name` + `retrieved_at` attribution (the "as of …" / source line).
  The anti-fabrication invariant is strengthened, not weakened: richer data
  still refuses to render without complete provider attribution.

This feature does **not** initiate any financial action and carries no QF
companion packet (Principle 10 — not applicable).

---

## Release Train

**Target train:** `mvp` (the active self-hosted train; see
[config/release-trains.yaml](../../config/release-trains.yaml)).

This is an **enhancement of an already-shipped, always-on skill** consumed by
the self-hosted deployment. It introduces **no new feature flag** — the weather
skill is gated by the existing SST key `assistant.skills.weather.enabled`, which
is already part of the `mvp` runtime. `state.json.flagsIntroduced` is therefore
empty. Behavior on other trains is unchanged: the skill reads the same SST
contract regardless of train, and the new unit / forecast-days keys are REQUIRED
in `config/smackerel.yaml` (the single source of truth all trains derive from).

---

## Requirements (technology-agnostic)

### Functional

- **R1.** When the weather skill answers, the answer MUST include, for the
  resolved location: condition, temperature, feels-like temperature, humidity,
  precipitation, wind speed, wind direction, UV, sunrise, and sunset.
- **R2.** The answer MUST include a multi-day forecast of **N days** (default
  10, SST-configured), each day reporting condition, high temp, low temp,
  precipitation probability, and UV peak.
- **R3.** The number of forecast days MUST originate from SST config with **no
  hardcoded fallback**; an out-of-range or missing value MUST fail startup
  loudly.
- **R4.** Temperature, wind-speed, and precipitation **units** MUST originate
  from SST config with **no hardcoded fallback**; an unrecognized or missing
  unit MUST fail startup loudly.
- **R5.** The rendered answer MUST be plain text, multi-line, and fit within the
  Telegram per-message budget after the adapter's MarkdownV2 escaping; it MUST
  remain skimmable (compact current block + one line per day).
- **R6.** The tool's structured output MUST additionally expose machine-readable
  `current` and `daily` objects/arrays (additive to the existing schema) so a
  native frontend can render its own UI; the existing `forecast_line`,
  `provider_name`, `retrieved_at` fields MUST be preserved.
- **R7.** Attribution (`provider_name` + `retrieved_at`) MUST be present on every
  answer; the facade MUST refuse to assemble (and route to capture) when any
  required attribution field is absent.
- **R8.** A cache hit MUST return the original upstream `retrieved_at`, unchanged.

### Non-functional / governance

- **R9.** All new readings MUST live on the provider-neutral `Forecast`
  contract; no open-meteo-specific field names may appear in the tool, facade,
  or YAML contract layer.
- **R10.** The `weather-query-v1.yaml` `output_schema`, the tool's Go output
  marshalling, and the facade payload struct MUST stay in **lockstep** (parity
  enforced by `scenario-lint`).
- **R11.** The p95 latency budget for the weather skill (spec 061 §5.2: 3s) MUST
  be preserved; one upstream forecast call still returns both current + daily.
- **R12.** The provider-unavailable (`BS-006`) and location-missing
  (`slot_missing: location`) flows MUST behave exactly as spec 061 shipped them.

---

## Acceptance Criteria

- **AC-1.** A `weather in <city>` query returns an answer containing condition,
  temperature, feels-like, humidity, precipitation, wind (speed + direction),
  UV, sunrise, and sunset for the resolved location. *(R1)*
- **AC-2.** The same answer contains a 10-day forecast: 10 dated rows, each with
  condition, high/low temp, precipitation probability, and UV. *(R2)*
- **AC-3.** Changing `assistant.skills.weather.forecast_days` to `7` yields a
  7-day forecast; setting it to an out-of-range value fails `config generate` /
  startup loudly. *(R3)*
- **AC-4.** Removing or blanking a unit key (temperature/wind/precip) fails
  startup loudly; no silent default is substituted. *(R4)*
- **AC-5.** The rendered Telegram message stays within the configured
  `max_message_chars` budget and renders without MarkdownV2 parse errors. *(R5)*
- **AC-6.** The tool's JSON output validates against the widened
  `output_schema` (includes `current` + `daily`) and still carries
  `forecast_line`, `provider_name`, `retrieved_at`. *(R6, R10)*
- **AC-7.** A forecast response with a missing `provider_name` (or
  `retrieved_at`) is **not** assembled into an answer; the response routes to the
  capture path. *(R7)*
- **AC-8.** A second identical lookup within the cache TTL returns the original
  `retrieved_at`, byte-identical to the first. *(R8)*
- **AC-9.** When the provider returns an error/outage, the answer takes the
  `BS-006` provider-unavailable path (no fabricated readings). *(R12)*
- **AC-10.** A query with no resolvable location takes the `slot_missing:
  location` path, unchanged from spec 061. *(R12)*
- **AC-11.** The `now` / `today` / `tomorrow` / `weekend` windows continue to be
  accepted and answered (now enriched), with no contract break. *(R7, backward
  compat)*

---

## Behavioral Scenarios (Gherkin)

```gherkin
Feature: Rich weather conditions and 10-day forecast

  Background:
    Given the weather skill is enabled
    And the configured provider is open-meteo
    And forecast_days is 10

  # SCN-094-A01
  Scenario: Current conditions include all readings, not just temperature
    Given a user asks "weather in Barcelona"
    And the provider returns current conditions for Barcelona
    When the weather skill answers
    Then the answer includes the condition, temperature, and feels-like temperature
    And the answer includes humidity, precipitation, wind speed, and wind direction
    And the answer includes the UV index
    And the answer includes sunrise and sunset times

  # SCN-094-A02
  Scenario: The answer includes a 10-day forecast
    Given a user asks "weather in Barcelona"
    And the provider returns a 10-day daily grid
    When the weather skill answers
    Then the answer includes exactly 10 dated daily rows
    And each daily row includes a condition, a high and low temperature, a precipitation probability, and a UV peak

  # SCN-094-A03
  Scenario: Forecast-day count is driven by SST configuration
    Given forecast_days is configured to 7
    When the weather skill answers a location query
    Then the answer includes exactly 7 daily rows

  # SCN-094-A04
  Scenario: A missing or out-of-range forecast_days fails loudly at startup
    Given forecast_days is unset
    When configuration is loaded
    Then loading fails with an error naming the forecast_days key
    And no default value is substituted

  # SCN-094-A05
  Scenario: Units originate from SST with no silent default
    Given the temperature-unit key is unset
    When configuration is loaded
    Then loading fails with an error naming the temperature-unit key
    And no default unit is substituted

  # SCN-094-A06
  Scenario: An unrecognized unit value is rejected
    Given the wind-speed-unit key is set to an unrecognized value
    When configuration is loaded
    Then loading fails with an error naming the offending key and value

  # SCN-094-A07
  Scenario: The rendered answer is plain text and fits the transport budget
    Given a 10-day rich forecast is rendered
    When the Telegram adapter renders the answer
    Then the rendered message is within the configured per-message character budget
    And the message contains no unescaped MarkdownV2 control sequence

  # SCN-094-A08
  Scenario: The structured output carries machine-readable current and daily blocks
    Given the weather skill answers a location query
    When the tool output is validated against the output schema
    Then the output contains a structured current object
    And the output contains a structured daily array
    And the output still contains forecast_line, provider_name, and retrieved_at

  # SCN-094-A09
  Scenario: Attribution is mandatory — a forecast missing provider name is refused
    Given a forecast result whose provider_name is empty
    When the facade attempts to assemble the answer
    Then no source attribution is assembled
    And the response routes to the capture path rather than fabricating attribution

  # SCN-094-A10
  Scenario: A cache hit preserves the original retrieved-at timestamp
    Given a location was looked up and cached
    When the same location is looked up again within the cache TTL
    Then the answer's retrieved_at equals the original upstream retrieved_at
    And it is not the cache-hit wall-clock time

  # SCN-094-A11
  Scenario: Provider outage takes the unavailable path with no fabricated data
    Given the provider returns an outage error
    When the weather skill answers
    Then the answer takes the provider-unavailable path
    And no weather readings are fabricated

  # SCN-094-A12
  Scenario: A query with no resolvable location takes the location-missing path
    Given a weather query with no resolvable location
    When the weather skill processes it
    Then the response signals a missing location slot
    And behaves exactly as before this enhancement

  # SCN-094-A13
  Scenario: Backward-compatible forecast windows still answer
    Given a user asks "weather in Barcelona tomorrow"
    When the weather skill answers
    Then the tomorrow window is accepted
    And the answer is the enriched current + daily forecast with no contract break

  # SCN-094-A14
  Scenario: The p95 latency budget is preserved with a single upstream call
    Given a burst of weather lookups across several locations
    When the skill answers each
    Then the p95 latency stays within the configured budget
    And a single upstream forecast call returns both current and daily data
```

---

## Open Decisions (for `bubbles.design` to settle)

1. **(a) Output-shape strategy.** Rich rendered `forecast_line` only, vs. also
   exposing additive structured `current` + `daily`. *(Recommended: both — the
   rendered line is the binding user answer; the structured blocks are additive
   for the web/mobile frontend per Principle 8 + design-decision-#1.)*
2. **(b) Forecast-window vs forecast-days.** Whether "10-day" is a new
   `ForecastWindow` value, a separate `forecast_days` knob, or both. Must keep
   `now` / `today` / `tomorrow` / `weekend` working. *(Recommended:
   `forecast_days` SST knob drives the daily grid; the existing windows remain
   for the current-conditions emphasis.)*
3. **(c) UV "current" source.** open-meteo `current` has no `uv_index`; the
   daily grid has `uv_index_max`. Decide how "current UV" is surfaced.
   *(Recommended: today's `uv_index_max` as the current-block UV; per-day UV in
   the daily rows.)*
4. **(d) Unit SST keys + vocabularies.** Exact key names and allowed values for
   temperature / wind-speed / precipitation units. *(Recommended: closed
   vocabularies matching open-meteo's unit parameters, REQUIRED, fail-loud.)*
5. **(e) Exact rendered layout.** The per-day line format and the current-block
   line set (UX-owned). *(Settled in design `## UX`.)*
