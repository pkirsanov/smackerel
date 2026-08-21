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

- [x] SCN-061-007-01 — an explicit `/weather <location>` carrying a real location answers with the forecast line itself, attributed to the provider that produced it, and the reply is never the "saved as an idea" capture acknowledgement. Verified at the facade weather-lookup seam (`unit`): body equals the forecast line, status is answered, body is asserted unequal to the product's own `captureFallbackAcknowledgement` constant, `CaptureRoute` is false, exactly one external-provider source is attached, and the LLM executor is invoked 0 times — so the masking path this bug reported cannot be reached. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] SCN-061-007-02 — when the weather provider genuinely fails, the explicit `/weather` turn says so honestly instead of claiming the request was saved. Verified at the facade weather-lookup seam (`unit`): with the lookup stub returning an error the status is unavailable with error cause `provider_unavailable`, the body is asserted unequal to `captureFallbackAcknowledgement`, `CaptureRoute` is false, and the executor is invoked 0 times — the pre-fix provider error is reproduced and is no longer rewritten into a capture acknowledgement. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] SCN-061-007-03 — a bare `/weather` with no location asks the user for one and does not call the weather provider at all. Verified at the facade weather-lookup seam (`unit`): the lookup stub records 0 calls, the status is unavailable with error cause `slot_missing`, and the body is asserted unequal to `captureFallbackAcknowledgement`, so an empty turn is neither sent upstream nor reported as a saved idea. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario Re-Verification (this session)".
- [x] Implementation behavior is complete — explicit `/weather` dispatches the weather tool directly; success renders the forecast + provider Source; error/empty renders an honest line; never the capture-as-fallback body. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass (`unit`) — the 3 adversarial fast-path tests GREEN (`internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Adversarial regression — each test wires the executor stub to reproduce the pre-fix provider-error and asserts the fast-path bypasses it (executor invoked 0 times) and never emits "saved as an idea"; `regression-quality-guard --bugfix` PASS. **Claim Source:** executed. Evidence: [report.md](report.md) → "Regression quality".
- [x] No regression — refactored weather tool tests + existing weather facade integration + backward-compat `/weather` executor-path test all GREEN (`internal/agent/tools/weather ok`, `internal/assistant ok`). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — `go test ./...` (compile + vet, filtered) clean; zero warnings; `./smackerel.sh check` (config + scenario-lint) OK. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot `/weather <location>` returns a forecast (or an honest unavailable line), never "saved as an idea". **Claim Source:** deployed + running + infra-verified this session (running core digest `sha256:44ed9984…` matches the fix build; healthy; Telegram assistant adapter wired and bound — see [report.md](report.md) → "Deploy + Live Verification"); behavioral confirmation pending the operator's Telegram smoke test (a human turn the agent cannot perform; an authenticated HTTP probe returned 401 because prod requires a PASETO session).
