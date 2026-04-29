# Bug Fix Design: [BUG-016-W2] Trip dossiers do not include destination forecasts

> **Owner:** bubbles.design (to be expanded during root cause analysis phase)
> **Status:** Initial — populated by bubbles.bug at packet creation, to be deepened by bubbles.design before implementation

---

## Root Cause Analysis

### Investigation Summary

The parent feature `016-weather-connector` was scoped against a `Change Boundary` that explicitly excluded `internal/intelligence/` and other consumer packages. The connector was implemented against that boundary and is healthy: it persists `weather/current` and `weather/forecast` artifacts (`internal/connector/weather/weather.go`) and serves the `weather.enrich.request` NATS subject (`internal/connector/weather/enrich.go`). The trip dossier assembler in `internal/intelligence/people.go` was, by design, never modified.

When parent uservalidation re-evaluated the **Outcome Contract** rather than just the change boundary on 2026-04-26, the gap surfaced: Outcome Contract Success Signal #3 promises destination forecasts in trip dossiers, but no scope was ever planned to wire it. This is the same boundary-vs-contract mismatch as BUG-016-W1 and is fixed with the same architectural pattern.

### Root Cause

`internal/intelligence/people.go` does not consume forecast data of any kind:

- `TripDossier` (lines 16–29) has no `DestinationForecast` / `Weather` field.
- `DetectTripsFromEmail` (lines 31–104) calls `extractDestination`, `classifyTripState`, `assembleDossierText` — none of these touch weather.
- `assembleDossierText` (lines 139–152) emits flight / hotel / related-captures counts only.
- No `weather.enrich.request` is published, no `artifacts` table query filters by `artifact_type = 'weather/forecast'`.
- `grep -n 'weather\|Weather\|forecast' internal/intelligence/people.go` → 0 matches.

The fix is purely additive on the dossier side; the connector requires no changes.

### Impact Analysis

- **Affected components:** `internal/intelligence/` (`people.go` + new sibling file `people_forecast.go`, plus `people_test.go` extensions).
- **Read-only dependencies:** the existing `weather/forecast` artifact rows persisted by `internal/connector/weather/`. No package-level Go import of `internal/connector/weather` is required because the consumer queries by `artifact_type = 'weather/forecast'` and `source_id = 'weather'` string constants.
- **Affected data:** None at rest — only the assembly-time `TripDossier` payload changes shape (additive field, `omitempty`).
- **Affected users:** All upcoming-trip dossier consumers gain a forecast section when the destination matches a fresh forecast artifact.

---

## Fix Design

### Solution Approach — Option (a1): Artifact-query

Of the two options in parent `uservalidation.md` Remediation Goal for BUG-016-W2:

- **(a1) Query recent `weather/forecast` artifacts from PostgreSQL whose title references the destination.**
- **(a2) Issue a `weather.enrich.request` over NATS for the destination + departure date.**

**Choose (a1).** Rationale:

1. **Pattern parity with the sibling fix (BUG-016-W1).** That bug used the artifact-query path against the same `weather/forecast` rows; using the same path here gives the codebase one consistent forecast-consumption pattern instead of two.
2. **Pattern parity with `internal/intelligence/`.** Every existing assembler in this package (e.g. `GetPeopleIntelligence`, `DetectTripsFromEmail` itself) drives off direct DB queries against `e.Pool`. None use NATS request/response for assembly. Introducing NATS here would add the only request/response correlation in the package.
3. **Lower failure surface.** No new request/response correlation, no timeout management beyond the caller's `ctx`, no NATS reply-subject management.
4. **Existing fresh data.** The weather connector already persists `weather/forecast` for every monitored location on its normal sync cadence; querying the most recent forecast artifacts for a destination within a TTL is sufficient and cheap.
5. **Reuses the connector contract instead of inventing one.** `enrich.go` is for *historical date+location* enrichment driven by other connectors (Maps); reusing it for the dossier's "future destination forecast" mis-fits the contract. The dossier needs *forward-looking* forecast data the connector already persists.

**Acknowledged constraint:** Forecast artifacts are produced only for *monitored* locations. If a detected trip destination is not in the monitored set, no forecast artifact will exist, and the dossier renders the graceful "no forecast" path. This is acceptable: the parent spec contract is satisfied for any destination that *can* be forecast-matched, and is gracefully degraded for destinations that cannot — no crash, no error, no fabricated data.

### Implementation Sketch

Add a new file `internal/intelligence/people_forecast.go` containing:

- `type DossierForecast struct { Destination string; Days []DossierForecastDay; AssembledAt time.Time }` — local DTO, JSON-tagged for downstream serialisation.
- `type DossierForecastDay struct { Title string; Description string; CapturedAt time.Time }`.
- `func (e *Engine) assembleDestinationForecast(ctx context.Context, destination string, now time.Time) *DossierForecast` — selects up to N most recent `weather/forecast` artifacts whose title ILIKE matches the destination, within a TTL window. Returns `nil` when destination is empty, when `e.Pool` is nil, when no fresh artifacts match, or when the query fails after logging via `slog.Warn("failed to assemble dossier forecast", "forecast", ..., "error", err)`.
- `func (f *DossierForecast) IsEmpty() bool` — mirrors the `WeatherDigestContext.IsEmpty` pattern from BUG-016-W1.

In `internal/intelligence/people.go`:

- Add `DestinationForecast *DossierForecast \`json:"destination_forecast,omitempty"\`` to `TripDossier`.
- After the `for _, d := range tripMap` loop calls `assembleDossierText`, branch on `d.State == "upcoming"` and call `e.assembleDestinationForecast(ctx, d.Destination, time.Now())`. On non-nil result, set `d.DestinationForecast` BEFORE re-assembling `d.DossierText` so the rendered text includes the forecast section.
- Update `assembleDossierText` to render a `🌤️ Forecast: <destination> — <day1> / <day2> / <day3>` line (max 3 days) when `d.DestinationForecast != nil && !d.DestinationForecast.IsEmpty()`. Do NOT touch the existing flight / hotel / captures rendering order.
- Preserve the existing `sort.Slice` deterministic ordering at line 99.

### Alternative Approaches Considered

1. **(a2) NATS request/response** — Rejected (see Solution Approach §1–5 above).
2. **Geocode arbitrary destinations to lat/lon and call the connector's forecast path directly** — Rejected: introduces a third-party geocoding dependency, violates the read-only consumption constraint, and exceeds the parent uservalidation Remediation Goal which explicitly allows destination-string lookup.
3. **Defer integration and amend parent spec** — Rejected by parent uservalidation Remediation Goal (preferred option is (a)). Removing the Outcome Contract promise would be a documentation-only patch that hides a missing feature.
4. **Embed forecast data in `RelatedCaptures`** — Rejected: violates the dossier shape, conflates artifact ID lists with structured forecast data, and breaks downstream consumers that expect `RelatedCaptures` to be an artifact-ID slice.
5. **Compute forecasts synchronously during dossier assembly via a connector method call** — Rejected: forces synchronous inter-package coupling for data the connector already persists.

### Affected Files

| File | Type of change |
|------|----------------|
| `internal/intelligence/people.go` | Add `DestinationForecast` field; call assembler for `upcoming` dossiers; update `assembleDossierText` to render the forecast line. |
| `internal/intelligence/people_forecast.go` (new) | `DossierForecast`, `DossierForecastDay`, `assembleDestinationForecast`, `IsEmpty`. |
| `internal/intelligence/people_forecast_test.go` (new) | Unit tests for the four Gherkin scenarios — present, missing destination, query failure, adversarial rendering. |
| `internal/intelligence/people_test.go` | Extend with at least one assertion that `assembleDossierText` renders the forecast marker when `DestinationForecast` is set. |

### Regression Test Design (failing-first)

Pre-fix failing test (must FAIL on current HEAD before any code change):

- **File:** `internal/intelligence/people_forecast_test.go` — new test `TestAssembleDossierText_RendersForecastSection`.
- **Setup:** construct a `TripDossier{ Destination: "Berlin", State: "upcoming", DestinationForecast: &DossierForecast{...with non-empty Days} }`.
- **Action:** call `assembleDossierText(d)`.
- **Adversarial assertion:** rendered text contains the substring `🌤️ Forecast` AND contains the destination name `Berlin`. The fixture is constructed such that *if the broken pre-fix code path were preserved* (no `DestinationForecast` field, no forecast rendering branch), the assertion would fail because the substring is structurally absent from the renderer's output.
- **Expected pre-fix outcome:** FAIL — `TripDossier` has no `DestinationForecast` field, so the test file is uncompilable against pre-fix HEAD. This is a strict structural FAIL, equivalent to the pattern used for BUG-016-W1's `TestFormatWeatherFallback_RendersWeatherSection`.

A second adversarial test `TestAssembleDestinationForecast_NoMatchingArtifact` seeds the DB with a `weather/forecast` artifact whose title does NOT reference the destination and asserts the result is `nil` — this prevents a tautological "always returns non-nil" implementation from passing.

A third adversarial test `TestAssembleDestinationForecast_QueryFailure` injects a closed pool / cancelled context and asserts (a) `nil` return AND (b) no panic AND (c) the surrounding `DetectTripsFromEmail` call still returns no error.

### Round-Trip Verification

Not applicable — this fix has no save/load symmetry. The connector writes `weather/forecast` artifacts and the dossier assembler reads them; the round trip is already covered by the connector's own `Sync` tests plus the new dossier assembly tests.

---

## Open Questions (to resolve in implement phase)

1. What is the freshness TTL for `weather/forecast` in the dossier context — 24 hours (matches BUG-016-W1) or longer to accommodate trips detected far in advance?
2. Should the dossier render an explicit `🌤️ Forecast unavailable` marker on query failure, or silently skip the section? Spec scenario SCN-BUG016W2-003 accepts EITHER — pick one consistently.
3. Should `assembleDestinationForecast` also be called for `State == "active"` dossiers (trips currently underway), or only `"upcoming"`? Parent uservalidation Goal text uses "upcoming"; spec contract is permissive.

These are documented for `bubbles.design` to expand and `bubbles.implement` to resolve before code changes land.
