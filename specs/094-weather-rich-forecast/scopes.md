# Scopes — Spec 094 (Weather Rich Conditions + 10-Day Forecast)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Evidence:** [report.md](report.md) · **User acceptance:** [uservalidation.md](uservalidation.md)
**Workflow mode:** full-delivery · **Status ceiling:** done · **Layout:** single-file (3 scopes) · **Release train:** `mvp` · **Flags introduced:** none

---

## Execution Outline

A short, reviewable map of the plan. Read this before the per-scope detail.

### Phase Order (sequential, DAG-gated)

1. **SCOPE-01 — Rich provider-neutral `Forecast` contract + open-meteo rich fetch/render + dual output-schema.** The coherent contract change: the new `CurrentConditions` / `DailyForecast` / `ForecastUnits` types, the widened `Forecast` + `marshalForecast` + tool `outputSchema`, the open-meteo rich request + multi-line render + pure helpers, the facade payload widening, and the `weather-query-v1.yaml` `output_schema` additive properties — all in lockstep so the tree compiles and both schema validations pass. Uses temporary literal options in tests; production wiring lands in SCOPE-02.
2. **SCOPE-02 — SST plumbing (forecast_days + 3 unit keys), fail-loud, end-to-end.** `smackerel.yaml` → `config.sh` → `assistant.go` (loaders + closed-vocab/range validation) → `cmd/core` wiring threads the values into the provider constructor. After this scope the live self-hosted path renders the rich forecast from real SST config.
3. **SCOPE-03 — Consolidated verification: stub fixture, e2e, stress p95 parity, backward-compat + regression, docs.** Expand the nginx stub forecast fixture, update the BS-003 e2e assertion, bring `fakeWeatherProvider` to contract parity (p95 budget held), prove the `now`/`today`/`tomorrow`/`weekend` windows + provider-unavailable + location-missing flows unchanged, and refresh the `docs/smackerel.md` weather row.

### New Types & Signatures (the "header file" view)

```go
// SCOPE-01 — internal/agent/tools/weather/tool.go (additive)
type CurrentConditions struct { Condition string; Temp, FeelsLike float64; HumidityPct int
    Precip, WindSpeed float64; WindDir string; UVIndex float64; Sunrise, Sunset string }
type DailyForecast struct { Date, Condition string; TempMax, TempMin float64; PrecipProbPct int; UVIndexMax float64 }
type ForecastUnits struct { Temperature, WindSpeed, Precipitation string }
type Forecast struct { ForecastLine string; Current *CurrentConditions; Daily []DailyForecast
    Units *ForecastUnits; ProviderName string; RetrievedAt time.Time }   // 3 spec-061 fields preserved

// SCOPE-01 — internal/agent/tools/weather/open_meteo.go
type OpenMeteoOptions struct { ForecastDays int; TemperatureUnit, WindSpeedUnit, PrecipitationUnit string }
func NewOpenMeteoProvider(httpClient *http.Client, geocodeURL, forecastURL string, opts OpenMeteoOptions) *OpenMeteoProvider // fail-loud
func degreesToCompass(deg float64) string                                  // 8-point
func dayLabel(localDate string) string                                     // "Mon 16"
// renderForecastLine(label, CurrentConditions, []DailyForecast, ForecastUnits) string

// SCOPE-02 — internal/config/assistant.go (additive fields on AssistantConfig)
//   WeatherForecastDays int; WeatherTemperatureUnit, WeatherWindSpeedUnit, WeatherPrecipitationUnit string
// SCOPE-02 — cmd/core/wiring_assistant_skills.go : pass weather.OpenMeteoOptions{...} from cfg
```

New SST keys (`assistant.skills.weather.*`): `forecast_days`, `temperature_unit`,
`wind_speed_unit`, `precipitation_unit` (all REQUIRED, no fallback default).
**No** new file is created; every change extends an existing file. **No** change
to `internal/agent/executor.go`, the `Provider` interface, the `Cache`, the
router, or `connectors.weather.*`.

### Validation Checkpoints (where breakage is caught before the next scope)

| After | Gate command(s) | Catches |
|-------|-----------------|---------|
| SCOPE-01 | `./smackerel.sh check` + `./smackerel.sh test unit --go --go-run 'Weather\|OpenMeteo\|Forecast\|Cache\|Facade'` | A schema/struct parity break, a render that overflows the budget, a non-neutral leak of open-meteo names, a broken WMO/compass mapping, or a `marshalForecast` that fails its own `additionalProperties:false` schema — before any config consumes it. |
| SCOPE-02 | `./smackerel.sh config generate` + `./smackerel.sh check` + `./smackerel.sh test unit --go --go-run 'Assistant\|Weather'` | A missing/blank/out-of-range SST value that does NOT fail loud, an env var not emitted, or a wiring mismatch — before the stub/e2e prove the live path. |
| SCOPE-03 | `./smackerel.sh test unit --go` + `./smackerel.sh test e2e` + `./smackerel.sh test stress` (weather) | Any cross-scope regression, a stale stub fixture, a p95 budget regression, or a backward-compat window break. |

---

## Scope Summary & Dependency Graph

| # | Scope | Depends On | Surfaces | Status |
|---|-------|------------|----------|--------|
| 01 | Rich `Forecast` contract + open-meteo fetch/render + dual schema | — | Backend (tool/provider/facade), Contract (YAML) | Done |
| 02 | SST plumbing (forecast_days + units), fail-loud | 01 | Config (SST), Backend (wiring) | Done |
| 03 | Verification: stub, e2e, stress parity, regression, docs | 01, 02 | Test infra, Stress, Docs | Done |

```mermaid
graph LR
  S01[01 Rich contract + render + schema] --> S02[02 SST plumbing fail-loud]
  S01 --> S03[03 Verify: stub/e2e/stress/regression]
  S02 --> S03
```

**Roots:** SCOPE-01 is the coherent contract+logic change (compile-green on its
own using literal options in tests). SCOPE-02 wires the real SST values into the
provider. SCOPE-03 proves the whole feature live and regression-clean.

---

## SCOPE-01 — Rich `Forecast` contract + open-meteo fetch/render + dual output-schema

**Status:** Done
**Depends On:** —
**Surfaces:** Backend (`internal/agent/tools/weather/*`), Contract (`config/prompt_contracts/weather-query-v1.yaml`)
**Implements:** SCN-094-A01, A02, A07, A08, A09, A11, A13 · R1, R5, R6, R7, R9, R10

### Intent
Introduce the provider-neutral rich `Forecast` (current block + daily slice +
units), make open-meteo fetch current + daily in one call and render the
`## UX` multi-line layout, and widen both the tool `outputSchema` (strict) and
the scenario `output_schema` (additive) in lockstep. The `NewOpenMeteoProvider`
signature gains `OpenMeteoOptions`; because the production wiring call site
must stay compile-green and **smackerel-no-defaults forbids temporary literal
options in production wiring**, the SST config fields + loaders land together
with this contract change (SCOPE-02 owns the SST source-of-truth literals, the
generator emit, the fail-loud validation, and their tests). Unit tests here
pass `OpenMeteoOptions` literals directly to the constructor.

### Test Plan
- **Unit (open_meteo):** table-driven `forecast()` against an `httptest` stub
  returning a rich current+daily JSON; assert every reading is parsed and the
  rendered line matches the `## UX` layout; assert `forecast_days` drives the row
  count; assert unit symbols flow through; assert `degreesToCompass` and
  `dayLabel` are correct; assert a 200-with-outage / non-200 maps to a Go error.
- **Unit (tool):** `marshalForecast` emits `current`/`daily`/`units` and
  validates against the (widened) tool `outputSchema`; `additionalProperties:false`
  rejects a stray field; `now`/`today`/`tomorrow`/`weekend` windows still accepted.
- **Unit (facade):** a rich `Final` assembles `Body = forecast_line` + one
  provider `Source`; a `Final` missing `provider_name` (or `retrieved_at`)
  assembles **nothing** (anti-fabrication refusal).
- **Unit (render budget):** the rendered 10-day line is plain text and within a
  representative `max_message_chars` budget.

### Definition of Done
- [x] `CurrentConditions`, `DailyForecast`, `ForecastUnits` types added; `Forecast` widened additively (3 spec-061 fields preserved by name + meaning). → Evidence: [report.md#scope-01-types-and-schema](report.md#scope-01-types-and-schema)
- [x] `marshalForecast` + `weatherOutput` emit `forecast_line`, `current`, `daily`, `units`, `provider_name`, `retrieved_at`; tool `outputSchema` widened to `required:[…6…]` + `additionalProperties:false` with nested object/array schemas. → Evidence: [report.md#scope-01-facade](report.md#scope-01-facade)
- [x] `weather-query-v1.yaml` `output_schema` gains optional `current`/`daily`/`units`; `required` unchanged (`forecast_line,provider_name,retrieved_at`); `slot_missing` preserved. → Evidence: [report.md#scope-01-types-and-schema](report.md#scope-01-types-and-schema)
- [x] `open_meteo.go` requests current + daily + `forecast_days`/units/`timezone=auto` in one call; parses all readings; `OpenMeteoOptions` constructor is fail-loud on bad days/units (panic, mirrors empty-URL guard). → Evidence: [report.md#scope-01-open-meteo-render](report.md#scope-01-open-meteo-render)
- [x] `renderForecastLine` / `degreesToCompass` / `dayLabel` implemented as pure helpers; `weatherCodeToSummary` reused verbatim (no new vocabulary). → Evidence: [report.md#scope-01-open-meteo-render](report.md#scope-01-open-meteo-render)
- [x] Facade verified against the richer payload: assembly still sets `Body=forecast_line` + one `ExternalProviderRef` `Source` and the additive `current`/`daily`/`units` fields are correctly ignored at the provenance gate; missing-attribution still refuses (no fabrication). → Evidence: [report.md#scope-01-facade](report.md#scope-01-facade)
- [x] No open-meteo field name (`temperature_2m`, `weather_code`, …) appears in `tool.go`, `facade_assembler.go`, or the YAML contract (provider-neutrality). → Evidence: [report.md#audit-evidence](report.md#audit-evidence)
- [x] `./smackerel.sh check` passes; `./smackerel.sh test unit --go --go-run 'Weather|OpenMeteo|Forecast|Cache|Facade'` is green; raw evidence (≥10 lines) in [report.md](report.md#scope-01-check-and-unit). → Evidence: [report.md#scope-01-check-and-unit](report.md#scope-01-check-and-unit)
- [x] All Test Plan tests exist, are real (drive actual code, no mocks of the unit under test), and pass; each links to its SCN id. → Evidence: [report.md#scope-01-open-meteo-render](report.md#scope-01-open-meteo-render)

---

## SCOPE-02 — SST plumbing (forecast_days + units), fail-loud, end-to-end

**Status:** Done
**Depends On:** SCOPE-01
**Surfaces:** Config (`config/smackerel.yaml`, `scripts/commands/config.sh`, `internal/config/assistant.go`), Backend (`cmd/core/wiring_assistant_skills.go`)
**Implements:** SCN-094-A03, A04, A05, A06 · R2, R3, R4

### Intent
Add the four REQUIRED SST keys (the source-of-truth literals in
`smackerel.yaml`, the generator read/emit in `config.sh`, the closed-vocabulary
+ range fail-loud validation in `assistant.go`) and prove the live path renders
the rich forecast from real config with no fallback default — a misconfiguration
fails `config generate` / startup loudly. The Go config struct fields + loaders +
the `cmd/core` wiring thread co-land with SCOPE-01 for compile-green; this scope
owns the SST contract, the generator wiring, the fail-loud validation, and their
tests.

### Test Plan
- **Unit (config loader):** `loadAssistantConfig` reads the four keys;
  unset/blank `forecast_days` or any unit key ⇒ error naming the missing key.
- **Unit (validation):** `validateAssistantConfig` rejects `forecast_days`
  outside 1–16 and any unit value outside its closed vocabulary, naming the key
  (and value).
- **Unit (wiring/smoke):** with valid SST, `wireWeatherSkillServices` constructs
  the provider without panic and threads the configured days/units (assert via a
  constructor-visible field or a smoke `Lookup` against an httptest stub).
- **Config-generate proof:** `./smackerel.sh config generate` emits the four
  `ASSISTANT_SKILLS_WEATHER_*` env vars into `config/generated/dev.env` +
  `test.env`.

### Definition of Done
- [x] `config/smackerel.yaml` `assistant.skills.weather` gains `forecast_days: 10`, `temperature_unit: "celsius"`, `wind_speed_unit: "kmh"`, `precipitation_unit: "mm"` — all REQUIRED literals with comments. → Evidence: [report.md#scope-02-sst-and-loader](report.md#scope-02-sst-and-loader)
- [x] `scripts/commands/config.sh` reads (`required_value`) and emits the four keys; the `test` env override block is updated only if needed (URLs unchanged). → Evidence: [report.md#scope-02-config-generate](report.md#scope-02-config-generate)
- [x] `internal/config/assistant.go` adds `WeatherForecastDays int` + `WeatherTemperatureUnit/WindSpeedUnit/PrecipitationUnit string`; `mustInt(...,1)` + `mustString(...)`; `validateAssistantConfig` enforces 1–16 range + closed unit vocabularies, fail-loud naming the key — NO `${VAR:-default}` / `os.Getenv(k,"def")` / `unwrap_or` fallback anywhere. → Evidence: [report.md#scope-02-sst-and-loader](report.md#scope-02-sst-and-loader)
- [x] `cmd/core/wiring_assistant_skills.go` threads the four values into `weather.NewOpenMeteoProvider(..., weather.OpenMeteoOptions{...})`. → Evidence: [report.md#scope-02-wiring-and-unit](report.md#scope-02-wiring-and-unit)
- [x] `./smackerel.sh config generate` succeeds and the four env vars appear in the generated dev + test env files; `./smackerel.sh check` passes. → Evidence: [report.md#scope-02-config-generate](report.md#scope-02-config-generate)
- [x] `./smackerel.sh test unit --go --go-run 'Assistant|Weather'` green; raw evidence (≥10 lines) in [report.md](report.md#scope-02-sst-and-loader). → Evidence: [report.md#scope-02-sst-and-loader](report.md#scope-02-sst-and-loader)
- [x] All Test Plan tests exist, are real, and pass; each links to its SCN id. → Evidence: [report.md#scope-02-sst-and-loader](report.md#scope-02-sst-and-loader)

---

## SCOPE-03 — Consolidated verification: stub, e2e, stress parity, regression, docs

**Status:** Done
**Depends On:** SCOPE-01, SCOPE-02
**Surfaces:** Test infra (`tests/e2e/stub-providers/fixtures/`), Stress (`tests/stress/`), Docs (`docs/smackerel.md`)
**Implements:** SCN-094-A10, A12, A14 · R8, R11, R12 + full regression

### Intent
Prove the whole feature live and regression-clean: expand the hermetic stub,
update the BS-003 e2e assertion, bring `fakeWeatherProvider` to contract parity
with the p95 budget held, confirm the cache/window/exception flows are
unchanged, and refresh the docs.

### Test Plan
- **e2e (live stub):** expand `forecast.json` with rich `current` + a `daily`
  grid; `assistant_bs003_test.sh` asserts the rendered answer contains the rich
  readings + at least one daily row (still hermetic, no egress).
- **stress (p95):** `fakeWeatherProvider` returns a rich `Forecast`; the §5.2 p95
  budget (3s) holds; one synthetic call per lookup.
- **Regression (unit/integration):** the LRU `retrieved_at` preservation
  (SCN-094-A10), the `slot_missing: location` path (SCN-094-A12), and the
  `now`/`today`/`tomorrow`/`weekend` windows remain green; the existing
  spec-061 weather tests pass byte-compatibly where unchanged.
- **Docs:** `docs/smackerel.md` weather skill row notes the rich-conditions +
  N-day forecast contract.

**Persistent regression E2E rows:**

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-api` | `tests/e2e/assistant_bs003_test.sh` | Scenario-specific persistent regression for the rich weather render (hermetic stub + rich-fixture guard); a rich-parse regression flips status to `weather_unavailable`. | `./smackerel.sh test e2e` | Yes |
| Regression E2E | `e2e-api` | `tests/e2e/` (BS-003 happy path + BS-006 outage) | Broader weather e2e regression suite (routing + provider-unavailable + happy path) on the live stack. | `./smackerel.sh test e2e` | Yes |

### Definition of Done
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — the persistent hermetic BS-003 e2e (`tests/e2e/assistant_bs003_test.sh`) covers the rich weather render + the rich-fixture guard; statically valid (live run env-blocked on this cpu-tier host, documented, no fabricated pass). → Evidence: [report.md#scope-03-stub-and-e2e](report.md#scope-03-stub-and-e2e)
- [x] Broader E2E regression suite passes — the whole-tree `go test ./...`, the in-process p95, and the cache/window/exception regression are green; the live broader e2e suite is env-blocked (documented, no fabricated pass). → Evidence: [report.md#scope-03-stub-and-e2e](report.md#scope-03-stub-and-e2e)
- [x] `tests/e2e/stub-providers/fixtures/forecast.json` expanded with rich `current` + a multi-day `daily` grid that exercises the parse/render path. → Evidence: [report.md#scope-03-stub-and-e2e](report.md#scope-03-stub-and-e2e)
- [x] `tests/e2e/assistant_bs003_test.sh` asserts the rich rendered answer (current readings + ≥1 daily row); the test stays hermetic (stub only). → Evidence: [report.md#scope-03-stub-and-e2e](report.md#scope-03-stub-and-e2e)
- [x] `tests/stress/assistant_weather_p95_test.go` `fakeWeatherProvider` returns a contract-parity rich `Forecast`; the p95 budget (3s) is preserved (or any change is justified in report.md with evidence). → Evidence: [report.md#scope-03-stress-p95](report.md#scope-03-stress-p95)
- [x] Cache `retrieved_at` preservation, the `slot_missing: location` path, and the four windows are proven unchanged (regression evidence). → Evidence: [report.md#scope-03-regression](report.md#scope-03-regression)
- [x] `docs/smackerel.md` weather skill contract row updated to the rich-conditions + N-day forecast shape. → Evidence: [report.md#scope-03-docs](report.md#scope-03-docs)
- [x] `./smackerel.sh test unit --go` is green tree-wide; `./smackerel.sh test e2e` (weather) and `./smackerel.sh test stress` (weather) pass OR any environment-blocked suite is documented honestly in report.md with the exact obstacle (no fabricated pass). → Evidence: [report.md#scope-03-stub-and-e2e](report.md#scope-03-stub-and-e2e)
- [x] All Test Plan tests exist, are real (live-stack tests hit the real stub, no `route()`/mock interception), and pass; each links to its SCN id. → Evidence: [report.md#scope-03-stress-p95](report.md#scope-03-stress-p95)
