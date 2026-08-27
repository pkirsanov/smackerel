# BUG-061-007 — Scopes

Status: in_progress

One scope: make the explicit `/weather` command deterministic so it renders the forecast
(or an honest unavailable line) and never the capture-as-fallback acknowledgement.

---

## Scope 1: Deterministic `/weather` shortcut fast-path

**Status:** Done (implemented + unit-verified; live-stack validation pending deploy)

**Depends on:** none

### Gherkin scenarios

```gherkin
Scenario: SCN-061-007-01 — a valid location returns the forecast (never "saved as an idea")
Scenario: SCN-061-007-02 — a provider failure is reported honestly (never "saved as an idea")
Scenario: SCN-061-007-03 — a bare /weather asks for a location (provider not called)
```

### Implementation plan

- `internal/agent/tools/weather/tool.go`: extract exported `LookupForecast`; delegate the
  handler to it (tool contract unchanged).
- `internal/assistant/facade.go`: `weatherLookup` seam + `WithWeatherLookup`; Step 3.9
  fast-path + `handleWeatherShortcut` (success → forecast + provider Source; error/empty →
  honest line; never capture-as-fallback).
- `cmd/core/wiring_assistant_facade.go`: wire the seam to `weather.LookupForecast(…, WindowNow)`.

### Test Plan

| Test Type | Category | File | Description | Command | Live System |
|-----------|----------|------|-------------|---------|-------------|
| Unit (adversarial) | `unit` | `internal/assistant/facade_weather_shortcut_test.go` | `TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor` — renders forecast + provider Source; executor invoked 0 times; body ≠ "saved as an idea" | `./smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut'` | No |
| Unit (adversarial) | `unit` | `internal/assistant/facade_weather_shortcut_test.go` | `TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea` — provider error → StatusUnavailable/ErrProviderUnavailable; body ≠ "saved as an idea"; CaptureRoute=false | `./smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut'` | No |
| Unit (adversarial) | `unit` | `internal/assistant/facade_weather_shortcut_test.go` | `TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup` — bare `/weather` → slot_missing; provider called 0 times | `./smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut'` | No |
| Unit (regression) | `unit` | `internal/agent/tools/weather/tool_test.go` | Refactored handler preserves the weather_lookup contract (empty-location / provider-error / windows / not-configured) | `./smackerel.sh test unit --go --go-run 'TestWeatherLookup|TestHandleWeatherLookup'` | No |
| Regression E2E (scenario-specific) | `e2e-api` | `tests/e2e/assistant/weather_shortcut_bug061007_e2e_test.go` | `TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured` — drives `/weather Paris` over the REAL HTTP ingress against the running stack and binds SCN-061-007-01/02/03 end to end: terminal `answered`, empty `error_cause`, `capture_route=false`, no "saved as an idea" body, and a real forecast reading. Carries a non-vacuity control turn. | `./smackerel.sh test e2e --go-package assistant --go-run 'TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured'` | Yes |
| Regression E2E (broader suite) | `e2e-api` | `tests/e2e/assistant/` (whole package) | The assistant e2e package still passes with the new file added, so the scenario test did not destabilise neighbouring live-stack coverage. | `./smackerel.sh test e2e --go-package assistant` | Yes |

### Consumer Impact Sweep

The fix extracted an exported symbol (`weather.LookupForecast`) out of the tool handler so the
facade could call it directly. Nothing was deleted and no route, endpoint or URL changed, so
the sweep is small and is enumerated rather than asserted:

| Consumer surface | Reference | State after the change |
|---|---|---|
| `internal/agent/tools/weather/tool.go` | the `weather_lookup` tool handler | Retained; now delegates to `LookupForecast`. The tool contract (name, schema, error strings) is unchanged, so the LLM tool path is unaffected. |
| `cmd/core/wiring_assistant_facade.go` | `WithWeatherLookup(weather.LookupForecast, WindowNow)` | New first-party caller of the extracted symbol; this is the wiring that makes the fast-path live. |
| `internal/assistant/facade.go` | `f.weatherLookup` seam | New internal consumer; nil-guarded, so an unwired build falls back to the pre-existing routed path rather than panicking. |
| Generated API client / deep link / navigation / breadcrumb / redirect surfaces | none | Not applicable: no route, endpoint, URL, slug or link identifier was renamed, moved or removed, so no client, deep link, navigation entry, breadcrumb or redirect references a stale identifier. |

The sweep for stale-reference risk was run over the repository rather than reasoned about.
`grep -rn 'LookupForecast' --include='*.go'` resolves to exactly three first-party files: the
definition and the handler's delegation in `internal/agent/tools/weather/tool.go` (lines 285
and 271), the live wiring call in `cmd/core/wiring_assistant_facade.go:242`, and two comment
references in `internal/assistant/facade.go` (206, 332) which name the symbol the injected
seam is wired to rather than calling it directly. The pre-existing `handleWeatherLookup` tool
entry point still resolves, so there is no dangling first-party reference to a removed name.

### Definition of Done

- [x] SCN-061-007-01 — an explicit `/weather <location>` carrying a real location answers with the forecast line itself, attributed to the provider that produced it, and the reply is never the "saved as an idea" capture acknowledgement. Verified at the facade weather-lookup seam (`unit`): body equals the forecast line, status is answered, body is asserted unequal to the product's own `captureFallbackAcknowledgement` constant, `CaptureRoute` is false, exactly one external-provider source is attached, and the LLM executor is invoked 0 times — so the masking path this bug reported cannot be reached. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] SCN-061-007-02 — when the weather provider genuinely fails, the explicit `/weather` turn says so honestly instead of claiming the request was saved. Verified at the facade weather-lookup seam (`unit`): with the lookup stub returning an error the status is unavailable with error cause `provider_unavailable`, the body is asserted unequal to `captureFallbackAcknowledgement`, `CaptureRoute` is false, and the executor is invoked 0 times — the pre-fix provider error is reproduced and is no longer rewritten into a capture acknowledgement. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] SCN-061-007-03 — a bare `/weather` with no location asks the user for one and does not call the weather provider at all. Verified at the facade weather-lookup seam (`unit`): the lookup stub records 0 calls, the status is unavailable with error cause `slot_missing`, and the body is asserted unequal to `captureFallbackAcknowledgement`, so an empty turn is neither sent upstream nor reported as a saved idea. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] Implementation behavior is complete — explicit `/weather` dispatches the weather tool directly; success renders the forecast + provider Source; error/empty renders an honest line; never the capture-as-fallback body. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass (`unit`) — the 3 adversarial fast-path tests GREEN (`internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Adversarial regression — each test wires the executor stub to reproduce the pre-fix provider-error and asserts the fast-path bypasses it (executor invoked 0 times) and never emits "saved as an idea"; `regression-quality-guard --bugfix` PASS. **Claim Source:** executed. Evidence: [report.md](report.md) → "Regression quality".
- [x] No regression — refactored weather tool tests + existing weather facade integration + backward-compat `/weather` executor-path test all GREEN (`internal/agent/tools/weather ok`, `internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — `go test ./...` (compile + vet, filtered) clean; zero warnings; `./smackerel.sh check` (config + scenario-lint) OK. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — `TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured` in `tests/e2e/assistant/weather_shortcut_bug061007_e2e_test.go` drives `/weather Paris` over the real HTTP ingress against the running stack and binds this bug's scenarios end to end. **Why it is not vacuous:** the e2e stack runs without a usable model, so every turn that reaches the model path returns `unavailable` / `provider_unavailable`; the weather fast-path is the one text turn that can reach a terminal `answered` precisely because it runs before the model. A regression that removed the fast-path would send `/weather` down that same path and fail the first subtest. The included control turn asserts a generic turn does NOT return forecast-shaped content, so the weather assertions bind something specific rather than a universal property. **Claim Source:** executed this session — 4/4 subtests PASS, `ok github.com/smackerel/smackerel/tests/e2e/assistant 0.339s`, exit 0. Evidence: [report.md](report.md) → "Certifying window — 2026-08-27".
- [x] Broader E2E regression suite passes — the whole `tests/e2e/assistant` package was executed with the new file present and reported `ok`, so adding this scenario test did not destabilise neighbouring live-stack coverage. **Claim Source:** executed this session. Evidence: [report.md](report.md) → "Certifying window — 2026-08-27".
- [x] Consumer impact sweep complete — zero stale first-party references remain. The change extracted `weather.LookupForecast` and deleted nothing; `grep -rn 'LookupForecast' --include='*.go'` resolves to exactly the three first-party files enumerated in the Consumer Impact Sweep table above, and no route, endpoint, URL, slug, deep link, navigation entry, breadcrumb or redirect identifier was renamed or removed, so no generated API client or link surface points at a stale name. **Claim Source:** executed. Evidence: [report.md](report.md) → "Certifying window — 2026-08-27".
- [x] Live-stack validation — on the running self-hosted bot `/weather <location>` returns a forecast (or an honest unavailable line), never "saved as an idea". **Claim Source:** the ephemeral e2e stack now proves the behaviour mechanically (see the scenario-specific E2E item above, executed this session). The self-hosted behavioural turn itself is the operator's, accepted on the recorded decision in [uservalidation.md](uservalidation.md) → `## Human Acceptance Record`; the agent did not perform it and does not claim to have. Deployment facts remain as recorded: running core digest `sha256:44ed9984…` matches the fix build, healthy, Telegram assistant adapter wired and bound. Evidence: [report.md](report.md) → "Certifying window — 2026-08-27".
