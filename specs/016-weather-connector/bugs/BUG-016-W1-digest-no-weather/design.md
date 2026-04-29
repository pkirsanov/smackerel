# Bug Fix Design: [BUG-016-W1] Daily digest does not include weather data

> **Owner:** bubbles.design (to be expanded during root cause analysis phase)
> **Status:** Initial — populated by bubbles.bug at packet creation, to be deepened by bubbles.design before implementation

---

## Root Cause Analysis

### Investigation Summary

The parent feature `016-weather-connector` was scoped against a `Change Boundary` (parent `scopes.md` line 13) that explicitly excluded `internal/digest/` and other consumer packages. The connector was implemented against that boundary and is healthy: it persists `weather/current` and `weather/forecast` artifacts (`internal/connector/weather/weather.go:171,214`) and serves the `weather.enrich.request` NATS subject (`internal/connector/weather/enrich.go`). The digest generator was, by design, never modified.

When parent uservalidation re-evaluated the **Outcome Contract** rather than just the change boundary on 2026-04-26, the gap surfaced: the Outcome Contract Success Signal #1 promises digest weather, but no scope was ever planned to wire it. This is a classic boundary-vs-contract mismatch.

### Root Cause

`internal/digest/generator.go::Generator.Generate()` does not consume weather data of any kind:

- `DigestContext` (lines 30–43) has no `Weather` field.
- `Generate()` (lines 87–171) calls `getPendingActionItems`, `getOvernightArtifacts`, `getHotTopics`, plus optional `AssembleHospitalityContext`, `assembleKnowledgeHealthContext`, and `ExpenseSection.Assemble` — none of these touch weather.
- No `weather.enrich.request` is published, no `artifacts` table query filters by `content_type IN ('weather/current','weather/forecast')`.

The fix is purely additive on the digest side; the connector requires no changes.

### Impact Analysis

- **Affected components:** `internal/digest/` (generator, context types, optional new assembler file), `config/prompt_contracts/digest-assembly-v1.yaml`.
- **Read-only dependencies:** `internal/connector/weather/` exports (artifact ContentType constants, optionally a `WeatherDigestContext` DTO if exposed by the connector package — otherwise the digest package owns the DTO).
- **Affected data:** None at rest — only the assembly-time `DigestContext` payload changes shape (additive field, `omitempty`).
- **Affected users:** All digest consumers gain a weather section when a home location is configured.

---

## Fix Design

### Solution Approach — Option (a1): Artifact-query

Of the two options in parent `uservalidation.md` Remediation Goal:

- **(a1) Query recent `weather/current` + `weather/forecast` artifacts from PostgreSQL.**
- **(a2) Issue a `weather.enrich.request` over NATS for current+forecast.**

**Choose (a1).** Rationale:

1. **Pattern parity with existing `DigestContext` populators.** Every other sub-context in `Generate()` is assembled by direct DB query against `g.Pool` (e.g. `getPendingActionItems`, `getOvernightArtifacts`, `AssembleHospitalityContext`). `DigestContext` has never used NATS request/response for assembly.
2. **Lower failure surface.** No new request/response correlation, no timeout management beyond the existing `ctx`, no NATS reply subject management.
3. **Existing fresh data.** `weather/current` is refreshed by the connector's normal sync cadence; querying the latest artifact within a TTL (e.g. ≤ 6 hours for current, ≤ 24 hours for forecast) is sufficient and cheap.
4. **Reuses the connector contract instead of inventing one.** `enrich.go` is for *historical* enrichment of date+location pairs from other connectors (Maps); reusing it for the digest's "right now" context would mis-fit the contract.

### Implementation Sketch

Add a new file `internal/digest/weather.go` containing:

- `type WeatherDigestContext struct { Current *WeatherCurrent; Forecast []WeatherForecastDay; Location string }` — local DTO, JSON-tagged for the prompt contract.
- `func AssembleWeatherContext(ctx context.Context, pool *pgxpool.Pool, homeLat, homeLon float64, now time.Time) (*WeatherDigestContext, error)` — selects the most recent `weather/current` artifact for the home location within a TTL, plus the next 3 `weather/forecast` artifacts. Returns `(nil, nil)` when no home location is configured, no fresh artifacts exist, or the query fails after logging.
- `func (w *WeatherDigestContext) IsEmpty() bool` — mirrors the `Hospitality`/`Expenses` pattern.

In `internal/digest/generator.go`:

- Add `Weather *WeatherDigestContext \`json:"weather,omitempty"\`` to `DigestContext`.
- Read home location from config (already loaded elsewhere — to be confirmed in implement phase; if not loaded by `Generator`, accept it via a new `Generator.HomeLocation` field wired in `cmd/core/main.go`).
- After the existing sub-context assembly, call `AssembleWeatherContext`. On non-nil result, set `digestCtx.Weather`.
- Update the "quiet day" condition to include `digestCtx.Weather == nil`.
- Pattern: `slog.Warn("failed to assemble weather digest context", "error", err)` on failure, mirror existing handlers.

In `config/prompt_contracts/digest-assembly-v1.yaml`:

- Document the new `digest_context.weather` payload.
- Add an instruction segment: "If `digest_context.weather` is present, render a `🌤️ Weather:` line plus a 3-day forecast block; otherwise omit the section entirely."

### Alternative Approaches Considered

1. **(a2) NATS request/response** — Rejected (see Solution Approach §1–4 above).
2. **Embed weather in the existing hospitality/knowledge/expenses sub-contexts** — Rejected: weather is orthogonal to those domains; bundling violates separation of concerns and parent spec language.
3. **Defer integration and amend parent spec** — Rejected by parent uservalidation Remediation Goal (preferred option is (a)). Removing the Outcome Contract promise would be a documentation-only patch that hides a missing feature.
4. **Compute weather from a new connector method on demand** — Rejected: forces synchronous inter-package coupling for data the connector already persists.

### Affected Files

| File | Type of change |
|------|----------------|
| `internal/digest/generator.go` | Add `Weather` field; call assembler; update quiet-day check. |
| `internal/digest/weather.go` (new) | `WeatherDigestContext`, `AssembleWeatherContext`, `IsEmpty`. |
| `internal/digest/weather_test.go` (new) | Unit tests for the three Gherkin scenarios + adversarial. |
| `internal/digest/generator_test.go` | Add a Generate-level test asserting Weather is populated when fixtures exist and absent otherwise. |
| `config/prompt_contracts/digest-assembly-v1.yaml` | Document weather payload + render instruction. |
| `cmd/core/main.go` (only if home location is not already wired into `Generator`) | Wire `HomeLocation` into the generator from config. |

### Regression Test Design (failing-first)

Pre-fix failing test (must FAIL on current HEAD before any code change):

- **File:** `internal/digest/generator_test.go` — new test `TestGenerate_IncludesWeatherWhenConfigured`.
- **Setup:** seed `artifacts` with one `weather/current` row and three `weather/forecast` rows for a home location, in an ephemeral test schema.
- **Action:** call `Generator.Generate(ctx)`.
- **Assertion (adversarial):** `digestCtx.Weather` is non-nil AND the rendered digest text (via the fallback path which is deterministic) contains the substring `Weather`.
- **Expected pre-fix outcome:** FAIL — `DigestContext` has no `Weather` field, so the assertion cannot compile (or fails after a temporary field stub) and the rendered text contains no weather marker.

### Round-Trip Verification

Not applicable — this fix has no save/load symmetry. The connector writes weather artifacts and the digest reads them; the round trip is already covered by the connector's own `Sync` tests plus the new digest assembly test.

---

## Open Questions (to resolve in implement phase)

1. Is the home location already loaded into the `Generator` via config? If not, where should it be read — `cmd/core/main.go` wiring, or a `cfg.Runtime.HomeLocations` lookup at assembly time?
2. What is the freshness TTL for `weather/current` in the digest context — 6 hours (matches typical current-conditions cadence) or longer?
3. Does the `digest-assembly-v1` prompt contract require a schema-version bump, or is an additive optional field acceptable under the existing version?

These are documented for `bubbles.design` to expand and `bubbles.implement` to resolve before code changes land.
