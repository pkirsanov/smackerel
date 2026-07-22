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

### Definition of Done

- [x] Implementation behavior is complete — explicit `/weather` dispatches the weather tool directly; success renders the forecast + provider Source; error/empty renders an honest line; never the capture-as-fallback body. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass (`unit`) — the 3 adversarial fast-path tests GREEN (`internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Adversarial regression — each test wires the executor stub to reproduce the pre-fix provider-error and asserts the fast-path bypasses it (executor invoked 0 times) and never emits "saved as an idea"; `regression-quality-guard --bugfix` PASS. **Claim Source:** executed. Evidence: [report.md](report.md) → "Regression quality".
- [x] No regression — refactored weather tool tests + existing weather facade integration + backward-compat `/weather` executor-path test all GREEN (`internal/agent/tools/weather ok`, `internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — `go test ./...` (compile + vet, filtered) clean; zero warnings; `./smackerel.sh check` (config + scenario-lint) OK. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot `/weather <location>` returns a forecast (or an honest unavailable line), never "saved as an idea". **Claim Source:** not-run (pending the home-lab deploy of the fixed SHA; owner bubbles.devops → operator smoke test).
