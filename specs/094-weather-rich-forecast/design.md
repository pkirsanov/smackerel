# Design 094 — Weather Rich Conditions + 10-Day Forecast

**Owner:** `bubbles.design` · **Status:** design complete · **Builds on:** [spec.md](spec.md) (analyst BDD + the resolved UX in `## UX` below)

This document settles spec 094's five open decisions ((a) output shape, (b)
window-vs-days, (c) current-UV source, (d) unit SST keys, (e) rendered layout),
then specifies the provider-neutral contract, the open-meteo fetch/render, the
SST plumbing, the wiring, the security model, and the test strategy at
contract grade so `/bubbles.plan` and `/bubbles.implement` proceed with zero
ambiguity.

---

## Design Brief

**Current State.** Spec 061 SCOPE-07 ships a `weather_lookup` agent tool
([`internal/agent/tools/weather/`](../../internal/agent/tools/weather/)) behind a
provider-neutral `Provider` interface (open-meteo the only impl). The tool
returns a `Forecast{ForecastLine, ProviderName, RetrievedAt}`; the open-meteo
provider ([`open_meteo.go`](../../internal/agent/tools/weather/open_meteo.go))
requests only `current=temperature_2m,weather_code` and renders
`"<label>: <summary>, <temp>°C"`. The `today`/`tomorrow`/`weekend` windows
currently fall back to current conditions (`_ = window`). An LRU
[`Cache`](../../internal/agent/tools/weather/cache.go) keyed by
`(provider, location, window)` preserves the upstream `retrieved_at` across hits
(§5.2). A facade [`SourceAssembler`](../../internal/agent/tools/weather/facade_assembler.go)
sets `Body = forecast_line` and attaches one `ExternalProviderRef` source;
missing any of `forecast_line`/`provider_name`/`retrieved_at` ⇒ refuse to
assemble (the `requires_provenance: true` anti-fabrication gate routes to the
`BS-006` capture path). The skill is wired in
[`wiring_assistant_skills.go`](../../cmd/core/wiring_assistant_skills.go) from
`cfg.Assistant.Weather*` SST values, and the contract is
[`config/prompt_contracts/weather-query-v1.yaml`](../../config/prompt_contracts/weather-query-v1.yaml)
with `direct_output_from_tool: weather_lookup`.

**Target State.** The same skill returns a **rich multi-line** answer — current
condition, temperature, feels-like, humidity, precipitation, wind (speed +
8-point compass direction), UV, sunrise, sunset — **plus a 10-day daily
forecast** (config-driven count), one compact line per day. open-meteo fetches
current + daily in **one** call. The provider-neutral `Forecast` gains a
structured `Current` block, a `Daily` slice, and a `Units` descriptor; the
tool's JSON output is widened **additively** with `current` / `daily` / `units`
so the web/mobile frontend can render natively. `forecast_line` /
`provider_name` / `retrieved_at` and every provenance/cache invariant are
preserved. Forecast-day count and units come from new **REQUIRED** SST keys with
no fallback default.

**Patterns to Follow.**
- The provider-neutral seam: all new readings on `Forecast` (not on
  `OpenMeteoProvider`). open-meteo stays one adapter (BS-009).
- The constructor fail-loud nil/empty-guard style in
  [`NewOpenMeteoProvider`](../../internal/agent/tools/weather/open_meteo.go) and
  [`NewCache`](../../internal/agent/tools/weather/cache.go) — panic at
  construction on misconfiguration so a bad env file is caught at startup.
- The SST loader idiom in
  [`internal/config/assistant.go`](../../internal/config/assistant.go)
  (`mustString` / `mustInt` / closed-vocabulary validation in
  `validateAssistantConfig`) — every key REQUIRED, fail-loud (smackerel-no-defaults).
- The `config.sh` read-block / `test`-override / `.env`-emit triplet for
  `ASSISTANT_SKILLS_WEATHER_*` ([scripts/commands/config.sh](../../scripts/commands/config.sh)
  ~1398 / ~1658 / ~2222).
- The plain-text `Body` + adapter-owned MarkdownV2 escaping contract (spec 061
  spec.md §14.B.1; [render_outbound.go](../../internal/telegram/assistant_adapter/render_outbound.go)
  `escapeForMode` + `budgetTruncate`).
- The hermetic nginx stub fixture pattern
  ([tests/e2e/stub-providers/fixtures/forecast.json](../../tests/e2e/stub-providers/fixtures/forecast.json)).

**Patterns to Avoid.**
- Leaking open-meteo field names (`temperature_2m`, `weather_code`, …) past the
  provider into the tool/facade/YAML layer (breaks BS-009 provider-neutrality).
- A hardcoded forecast-day constant or a `${VAR:-default}` / `unwrap_or` unit
  fallback (violates smackerel-no-defaults / Gate G028).
- Pre-escaping MarkdownV2 inside the skill — escaping is the adapter's job; the
  skill emits plain text (double-escaping corrupts the message).
- Space-padded "columns" for the daily grid — Telegram's proportional font
  won't align them; use a compact delimiter format.
- A second upstream round-trip for daily data — open-meteo returns current +
  daily in one request, preserving the §5.2 p95 budget.

---

## Settled Open Decisions (binding)

### (a) Output shape — **rendered line + additive structured blocks.**
The binding user answer is the rich multi-line plain-text `forecast_line`
(every transport renders it). **Additionally**, the tool JSON exposes
machine-readable `current` (object), `daily` (array), and `units` (object) so
the spec-073 web/mobile frontend can render a native UI without a second
round-trip (Principle 8 transparency; spec 094 R6). This is purely additive:
`forecast_line` / `provider_name` / `retrieved_at` are unchanged in name and
meaning. *Rejected:* rendered-line-only (forecloses the frontend) and
structured-only (breaks Telegram/HTTP text consumers).

### (b) Window vs days — **a new SST `forecast_days` knob drives the daily grid; the existing windows are preserved.**
"10-day" is expressed by `assistant.skills.weather.forecast_days` (default 10,
range 1–16 — open-meteo's max). The `ForecastWindow` enum
(`now`/`today`/`tomorrow`/`weekend`) is **unchanged and still accepted**; every
window now returns the enriched current block + the daily grid. The window no
longer changes the *set* of data fetched (one rich call serves all), so the
backward-compatible windows keep working with no contract break (SCN-094-A13).
*Rejected:* adding a `ten_day` enum value (would fragment the cache key and the
window switch for no behavioral gain).

### (c) Current-UV source — **today's `uv_index_max` from the daily grid.**
open-meteo's `current` block has no `uv_index`; `uv_index_max` is a daily field.
The current block surfaces **today's** (`daily[0]`) `uv_index_max` as the
"current" UV (the day's peak — the actionable number for sun protection);
per-day UV peaks populate the daily rows. *Rejected:* an extra `hourly=uv_index`
fetch (more payload, sub-daily granularity the spec explicitly excludes).

### (d) Unit SST keys — **three REQUIRED keys with closed vocabularies, fail-loud.**
| SST key (`assistant.skills.weather.*`) | env var | allowed values | open-meteo param | display symbol |
|---|---|---|---|---|
| `temperature_unit` | `ASSISTANT_SKILLS_WEATHER_TEMPERATURE_UNIT` | `celsius`, `fahrenheit` | `temperature_unit` | `°C` / `°F` |
| `wind_speed_unit` | `ASSISTANT_SKILLS_WEATHER_WIND_SPEED_UNIT` | `kmh`, `ms`, `mph`, `kn` | `wind_speed_unit` | `km/h` / `m/s` / `mph` / `kn` |
| `precipitation_unit` | `ASSISTANT_SKILLS_WEATHER_PRECIPITATION_UNIT` | `mm`, `inch` | `precipitation_unit` | `mm` / `in` |
| `forecast_days` | `ASSISTANT_SKILLS_WEATHER_FORECAST_DAYS` | integer 1–16 | `forecast_days` | — |

All REQUIRED; an unset/blank/unrecognized value fails `loadAssistantConfig` /
`validateAssistantConfig` with the offending key (and value) named. No default
substitution. These are read at wiring time and threaded into the provider
constructor (provider-neutral inputs; open-meteo maps them to its query params).

### (e) Rendered layout — **see `## UX` below (binding).**

### Single-Implementation Justification

This feature uses **provider** language (the spec-061 `Provider` interface;
open-meteo) and so trips the capability-foundation proportionality triggers, but
it ships **exactly one** concrete implementation and deliberately does **not**
introduce a second. open-meteo is the sole provider; spec 094 Non-Goals
explicitly exclude "a new weather provider." The capability **foundation already
exists** (spec 061: the `Provider` interface + the provider-neutral `Forecast`
contract + the `Cache`); spec 094 only **enriches the shared contract** (adds
`Current`/`Daily`/`Units` to the provider-neutral `Forecast`) so the existing
seam still cleanly admits a future second provider **without** rework. There is
therefore no foundation-vs-implementations split to scope (no `foundation:true`
overlay), no variation axis beyond the single open-meteo adapter, and no new
abstraction is warranted (YAGNI — inventing a provider-strategy layer for one
provider would multiply response paths and risk the non-enumeration / provenance
invariants). If and when a second provider is added, *that* spec owns the
foundation/concrete-implementations/variation-axes decomposition.

---

## UX (binding — owned by the UX pass; lives here because `ux.md` is a forbidden sidecar)

> Non-UI surfaces (HTTP/web/mobile) consume the same `Body`. UX here defines the
> rendered text layout, the status/condition language, and the two exception
> flows (location-missing, provider-unavailable). Telegram MarkdownV2 escaping
> is applied by the adapter — the skill emits **plain text**.

### Rendered `forecast_line` (the user-facing answer)

Plain text, newline-separated, ordered most-relevant-first (Principle 7). For a
`celsius` / `kmh` / `mm` / `forecast_days=10` deployment:

```
Barcelona, ES — clear, 18°C (feels 17°C)
humidity 55% · wind 12 km/h NE · UV 4
precip 0.2 mm · sunrise 07:12 · sunset 21:25

next 10 days:
Mon 16: clear, 14–22°C, rain 10%, UV 5
Tue 17: partly cloudy, 13–21°C, rain 20%, UV 4
Wed 18: rain, 12–19°C, rain 80%, UV 3
… (one line per configured day)
```

**Layout contract:**
- **Line 1** — `<label> — <condition>, <temp>°<U> (feels <feels>°<U>)`.
- **Line 2** — `humidity <H>% · wind <W> <windUnit> <dir> · UV <uv>`.
- **Line 3** — `precip <P> <precipUnit> · sunrise <HH:MM> · sunset <HH:MM>`.
- **Blank line**, then `next <N> days:`.
- **N day rows** — `<Wkd> <DD>: <condition>, <min>–<max>°<U>, rain <prob>%, UV <uv>`.
- **Rounding:** temperatures, wind, humidity, precip-prob, UV → nearest integer;
  precipitation amount → 1 decimal. **Compass:** 8-point (`N NE E SE S SW W NW`)
  from wind-direction degrees. **Day label:** weekday-abbrev + day-of-month from
  the (local, `timezone=auto`) daily date.
- **Budget:** the adapter's `budgetTruncate` enforces `max_message_chars`; a
  10-day render is ≈ 14 lines / ≈ 700 chars pre-escape — comfortably within the
  4096 Telegram cap (SCN-094-A07).

### Condition language
Reuse the existing `weatherCodeToSummary` WMO mapping verbatim (`clear`,
`partly cloudy`, `fog`, `rain`, `snow`, `showers`, `thunderstorm`, `unknown`) —
terse, lowercase, Principle-7-aligned. No new vocabulary.

### Exception flows (unchanged from spec 061 — do not regress)
- **Location missing** → the scenario emits `slot_missing: location` (the
  weather_lookup tool is not called); the user sees the existing
  ask-for-location behavior. SCN-094-A12.
- **Provider unavailable / outage** → the tool returns a Go error; the facade
  assembles **no** source; the `requires_provenance` gate refuses and routes to
  the `BS-006` capture path. **No partial / fabricated readings are ever
  rendered** — a missing field means the whole answer is refused, not
  best-efforted. SCN-094-A09, SCN-094-A11.

---

## Provider-Neutral Contract (Go)

In [`tool.go`](../../internal/agent/tools/weather/tool.go) (provider-neutral —
no open-meteo names):

```go
// CurrentConditions is the provider-neutral "right now" snapshot.
// Numeric values are expressed in the units named by Forecast.Units.
type CurrentConditions struct {
    Condition   string  `json:"condition"`     // WMO summary, e.g. "clear"
    Temp        float64 `json:"temp"`
    FeelsLike   float64 `json:"feels_like"`
    HumidityPct int     `json:"humidity_pct"`
    Precip      float64 `json:"precip"`
    WindSpeed   float64 `json:"wind_speed"`
    WindDir     string  `json:"wind_dir"`      // 8-point compass, e.g. "NE"
    UVIndex     float64 `json:"uv_index"`      // today's peak
    Sunrise     string  `json:"sunrise"`       // local HH:MM
    Sunset      string  `json:"sunset"`        // local HH:MM
}

// DailyForecast is one provider-neutral daily-outlook row.
type DailyForecast struct {
    Date          string  `json:"date"`            // local YYYY-MM-DD
    Condition     string  `json:"condition"`
    TempMax       float64 `json:"temp_max"`
    TempMin       float64 `json:"temp_min"`
    PrecipProbPct int     `json:"precip_prob_pct"`
    UVIndexMax    float64 `json:"uv_index_max"`
}

// ForecastUnits is the self-describing unit descriptor (display symbols).
type ForecastUnits struct {
    Temperature   string `json:"temperature"`   // "°C" | "°F"
    WindSpeed     string `json:"wind_speed"`    // "km/h" | "m/s" | "mph" | "kn"
    Precipitation string `json:"precipitation"` // "mm" | "in"
}

// Forecast is the provider-neutral result. Current/Daily/Units are
// additive (spec 094); ForecastLine/ProviderName/RetrievedAt are
// preserved verbatim from spec 061.
type Forecast struct {
    ForecastLine string             `json:"forecast_line"`
    Current      *CurrentConditions `json:"current"`
    Daily        []DailyForecast    `json:"daily"`
    Units        *ForecastUnits     `json:"units"`
    ProviderName string             `json:"provider_name"`
    RetrievedAt  time.Time          `json:"retrieved_at"`
}
```

`marshalForecast` emits exactly: `forecast_line`, `current`, `daily`, `units`,
`provider_name`, `retrieved_at` (RFC3339). The `weatherOutput` marshal struct
mirrors this 1:1.

### Tool `OutputSchema` (tool.go — BS-005 return-schema, terminal on violation)
Widened to `required: [forecast_line, current, daily, units, provider_name,
retrieved_at]`, `additionalProperties: false`, with nested object schemas for
`current` (the 10 fields above), `daily.items` (the 6 fields), and `units` (the
3 fields). This is what the executor validates the tool result against at
[executor.go §3e](../../internal/agent/executor.go) (`return_schema_violation`).

### Scenario `output_schema` (weather-query-v1.yaml — direct-output validation)
`current` / `daily` / `units` are **added as optional properties** (documented
shapes). `required` stays `[forecast_line, provider_name, retrieved_at]` and
`slot_missing` is preserved, so the location-missing path is **unaffected**. The
executor validates the tool result against this at
[executor.go direct-output](../../internal/agent/executor.go); the tool's emitted
JSON (3 required + the 3 additive) validates because the scenario schema does not
set `additionalProperties: false`. **Rationale for the asymmetry:** the tool
schema is strict (the tool always emits all 6 on success); the scenario schema
is permissive (it must also admit the `slot_missing` final, which carries none of
the forecast fields).

---

## open-meteo Fetch + Render (open_meteo.go)

One forecast request:

```
current   = temperature_2m,relative_humidity_2m,apparent_temperature,
            precipitation,weather_code,wind_speed_10m,wind_direction_10m
daily     = weather_code,temperature_2m_max,temperature_2m_min,
            precipitation_probability_max,uv_index_max,sunrise,sunset
forecast_days   = <SST forecast_days>
temperature_unit / wind_speed_unit / precipitation_unit = <SST>
timezone  = auto   (local sunrise/sunset + local daily dates)
```

- The `OpenMeteoProvider` struct gains `forecastDays int` + `units providerUnits`
  (the open-meteo param strings + display symbols), set by the constructor.
- `NewOpenMeteoProvider(httpClient, geocodeURL, forecastURL, opts OpenMeteoOptions)`
  — `opts` carries `ForecastDays` + the three unit strings. **Fail-loud**: panic
  on `ForecastDays < 1 || > 16` or any empty/unrecognized unit (mirrors the
  existing empty-URL panic; a bad env file dies at startup, not first request).
- `forecast()` parses the widened response into `CurrentConditions` + `[]DailyForecast`
  + `ForecastUnits`, renders the layout from `## UX`, and returns the enriched
  `Forecast`. `RetrievedAt` is still `p.now().UTC()`; the §5.2 cache invariant is
  untouched (the cache stores/returns the whole `Forecast` verbatim).
- New pure helpers (unit-tested): `degreesToCompass(float64) string`,
  `renderForecastLine(label string, cur CurrentConditions, daily []DailyForecast, u ForecastUnits) string`,
  `dayLabel(localDate string) string`. `weatherCodeToSummary` is reused verbatim.

The geocode path, the comma-suffix retry, the `Provider` interface, the cache,
and the handler are **unchanged**.

---

## SST Plumbing (end-to-end, fail-loud)

| Layer | File | Change |
|---|---|---|
| Source of truth | [config/smackerel.yaml](../../config/smackerel.yaml) `assistant.skills.weather` | Add `forecast_days: 10`, `temperature_unit: "celsius"`, `wind_speed_unit: "kmh"`, `precipitation_unit: "mm"` — all REQUIRED literals. |
| Generator read | [scripts/commands/config.sh](../../scripts/commands/config.sh) ~1408 | `required_value assistant.skills.weather.{forecast_days,temperature_unit,wind_speed_unit,precipitation_unit}`. |
| Generator emit | config.sh ~2222 | Emit the four `ASSISTANT_SKILLS_WEATHER_*` lines into `<env>.env`. |
| Test override | config.sh ~1658 | (No URL change.) The four keys flow through unchanged in the `test` env (stub ignores params). |
| Loader | [internal/config/assistant.go](../../internal/config/assistant.go) | New fields `WeatherForecastDays int`, `WeatherTemperatureUnit/WindSpeedUnit/PrecipitationUnit string`; `mustInt(...,1)` + `mustString(...)`; closed-vocabulary + 1–16 range checks in `validateAssistantConfig`, fail-loud naming the key. |
| Wiring | [cmd/core/wiring_assistant_skills.go](../../cmd/core/wiring_assistant_skills.go) | Thread the four values into `weather.NewOpenMeteoProvider(..., weather.OpenMeteoOptions{...})`. |

No `connectors.weather.*` change (that is the separate spec-016 connector).

---

## Security & Anti-Fabrication

- **Provenance preserved.** The facade still refuses to assemble when
  `forecast_line`/`provider_name`/`retrieved_at` is missing; the richer payload
  does not relax this. A partial provider response (e.g. daily present, current
  absent) renders no answer — the gate refuses (SCN-094-A09).
- **No injection surface.** The rendered line is plain text; the Telegram
  adapter escapes it (MarkdownV2). Numeric fields are formatted with `%d` / `%.1f`
  (no user/provider string interpolated unescaped except the WMO summary and the
  geocoded label, both already in the spec-061 line and adapter-escaped).
- **SST fail-loud.** Units + forecast-days are REQUIRED; a misconfigured
  deployment dies at startup, never silently defaulting (Gate G028).
- **No new egress / no new secret / no new dependency.** Same open-meteo host,
  keyless; one request (current+daily) replaces one request (current). No new
  Go module.
- **Cache key unchanged** `(provider, location, window)` — the richer payload is
  stored as the cached `Forecast` value; `retrieved_at` preserved (SCN-094-A10).

---

## Test Strategy

| Scenario | Type | Test (package) |
|---|---|---|
| SCN-094-A01 current readings rendered | unit | `open_meteo_test.go::TestForecast_RichCurrent_AllReadingsRendered` |
| SCN-094-A02 10 daily rows | unit | `open_meteo_test.go::TestForecast_DailyGrid_TenRowsRendered` |
| SCN-094-A03 forecast_days drives count | unit | `open_meteo_test.go::TestForecast_ForecastDays_DrivesRowCount` |
| SCN-094-A04 forecast_days fail-loud | unit | `assistant_test.go::TestLoadAssistant_WeatherForecastDays_RequiredAndRanged` |
| SCN-094-A05 unit fail-loud | unit | `assistant_test.go::TestLoadAssistant_WeatherUnits_Required` |
| SCN-094-A06 unrecognized unit rejected | unit | `assistant_test.go::TestValidateAssistant_WeatherUnit_ClosedVocabulary` |
| SCN-094-A07 budget + plain text | unit | `render_outbound` golden + `open_meteo_test.go::TestRenderForecastLine_PlainText_WithinBudget` |
| SCN-094-A08 structured output schema | unit | `tool_test.go::TestMarshalForecast_StructuredOutput_ValidatesSchema` |
| SCN-094-A09 attribution mandatory | unit | `facade_assembler_test.go::TestFacade_MissingProvider_RefusesAssembly` (extend) |
| SCN-094-A10 cache retrieved_at preserved | unit | `cache_test.go` (existing, extended for rich Forecast) |
| SCN-094-A11 provider outage path | unit | `open_meteo_test.go` outage + facade refuse |
| SCN-094-A12 location-missing path | unit | existing slot_missing path (unchanged; regression) |
| SCN-094-A13 backward-compat windows | unit | `tool_test.go::TestHandleWeatherLookup_WindowsStillAccepted` |
| SCN-094-A14 p95 single call | stress | `tests/stress/assistant_weather_p95_test.go` (fakeWeatherProvider parity) |
| live happy path | e2e | `tests/e2e/assistant_bs003_test.sh` + stub fixture |

- **Unit** is the dominant layer (pure parse/render/marshal/config). The
  open-meteo response parsing is exercised with table-driven fixtures (no
  network) via the existing `httptest`-style stub used in `open_meteo_test.go`.
- **e2e** uses the in-tree nginx stub
  ([forecast.json](../../tests/e2e/stub-providers/fixtures/forecast.json)),
  expanded with the new `current` + `daily` fields, so the live-stack BS-003
  happy path exercises the rich render without network egress.
- **stress** keeps the §5.2 p95 budget (3s); `fakeWeatherProvider` is updated to
  return a rich `Forecast` so the contract shape matches; the budget is unchanged
  (still one synthetic call per lookup, cache-backed).
- **Adversarial/regression:** the `additionalProperties:false` tool schema would
  reject a stray field; the missing-provider facade test fails if anti-fabrication
  regresses; the unit fail-loud tests fail if a silent default is introduced; the
  backward-compat window test fails if an enum value is dropped.

---

## File-Touch Map

| # | File | Change | Scope |
|---|------|--------|-------|
| 1 | `internal/agent/tools/weather/tool.go` | `CurrentConditions`/`DailyForecast`/`ForecastUnits` types; widen `Forecast`, `weatherOutput`, `marshalForecast`, tool `outputSchema` | 01 |
| 2 | `internal/agent/tools/weather/open_meteo.go` | `OpenMeteoOptions`; widened request; rich parse; `renderForecastLine`/`degreesToCompass`/`dayLabel`; constructor fail-loud | 01 |
| 3 | `internal/agent/tools/weather/open_meteo_test.go` | rich parse/render/days/units/outage tests | 01 |
| 4 | `internal/agent/tools/weather/tool_test.go` | marshalling + windows-accepted tests | 01 |
| 5 | `internal/agent/tools/weather/facade_assembler.go` | unchanged gate logic (needs only the 3 attribution fields; additive fields ignored) + a comment noting the additive contract | 01 |
| 6 | `internal/agent/tools/weather/facade_assembler_test.go` | rich-payload assembly + missing-attribution refusal | 01 |
| 7 | `config/prompt_contracts/weather-query-v1.yaml` | `output_schema` += optional `current`/`daily`/`units` | 01 |
| 8 | `config/smackerel.yaml` | `assistant.skills.weather` += 4 REQUIRED keys | 02 |
| 9 | `scripts/commands/config.sh` | read + emit the 4 keys (test override unchanged) | 02 |
| 10 | `internal/config/assistant.go` | 4 new fields + loaders + closed-vocab/range validation | 02 |
| 11 | `internal/config/assistant_test.go` | loader + validation tests | 02 |
| 12 | `cmd/core/wiring_assistant_skills.go` | thread the 4 values into the provider constructor | 02 |
| 13 | `tests/e2e/stub-providers/fixtures/forecast.json` | += `current` (rich) + `daily` (grid) | 03 |
| 14 | `tests/e2e/assistant_bs003_test.sh` | assert rich render | 03 |
| 15 | `tests/stress/assistant_weather_p95_test.go` | `fakeWeatherProvider` rich `Forecast`; budget held | 03 |
| 16 | `docs/smackerel.md` (weather skill row) | note the rich-conditions + N-day forecast contract | 03 |

No change to `internal/agent/executor.go`, the `Provider` interface, the `Cache`,
the router, the rate limiter, or `connectors.weather.*`.
