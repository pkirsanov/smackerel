# BUG-061-007 — Design: deterministic `/weather` fast-path

## Root cause (code-path trace)

```
facade.Handle
  Step 2  LookupShortcut("/weather <us-zip>") -> shortcutScenarioID = "weather_query"
  Step 4  route -> BandHigh -> scenarioID = "weather_query"
  BandHigh: result = executor.Run(...)          # LLM tool-call loop
    qwen3:30b returns NO tool call, NO final
    -> OutcomeProviderError (llm_returned_no_tool_calls_and_no_final)
  weather source-assembler: result.Outcome != OK -> empty Sources
  provenance gate: weather_query is requires_provenance, Sources empty
    -> resp.CaptureRoute = true
    -> resp.Body = "saved as an idea — i'll surface it later."   # the masked answer
```

The scenario's `direct_output_from_tool: weather_lookup` only short-circuits *after* the
model emits the tool call; it does nothing when the model emits nothing.

## Fix

Add a **deterministic `/weather` fast-path** (Step 3.9) that runs BEFORE routing for an
explicit `/weather <location>` shortcut and dispatches the weather tool directly — no LLM,
no provenance gate.

### Seam (keeps the generic facade decoupled from the concrete tool)

- `internal/agent/tools/weather/tool.go` — extract the tool handler's core into an exported
  `LookupForecast(ctx, location, window) (json.RawMessage, error)` that reuses the same
  provider + cache + attribution invariants. `handleWeatherLookup` now unmarshals and
  delegates to it (the tool contract is unchanged).
- `internal/assistant/facade.go` — new `weatherLookup func(ctx, location) (json.RawMessage, error)`
  seam + `WithWeatherLookup(...)`. When wired, Step 3.9 fires for
  `shortcutScenarioID == "weather_query"`:
  - empty location → `StatusUnavailable` / `ErrSlotMissing` honest prompt (provider not called);
  - lookup error → `StatusUnavailable` / `ErrProviderUnavailable` honest line (NOT capture);
  - success → `StatusAnswered`, Body = `forecast_line`, one `SourceExternalProvider` Source
    (mirrors `weather.NewFacadeAssembler`, so attribution stays honest without the gate).
- `cmd/core/wiring_assistant_facade.go` — wire
  `facade.WithWeatherLookup(func(ctx, loc){ return weather.LookupForecast(ctx, loc, weather.WindowNow) })`.

### Why the explicit command is safe to dispatch deterministically

For `/weather <location>` the location is unambiguous (the stripped tail), so no LLM is
needed to decide the tool or extract the argument. NL weather ("what's the weather in
Paris?") is unaffected and still flows through the LLM path.

### Honest-error contract (removes the BS-006 masking for the explicit command)

The fast-path fully owns its response for BOTH success and failure, so a provider error is
surfaced honestly instead of being rewritten to "saved as an idea" by the provenance gate.

## Backward compatibility

The fast-path is gated on `f.weatherLookup != nil`. Facade tests that do not wire the seam
keep the prior LLM-routed `/weather` behavior — no existing test changes.

## Test plan

Adversarial unit tests in `internal/assistant/facade_weather_shortcut_test.go` wire the
executor stub to REPRODUCE the pre-fix failure (provider-error) and assert the fast-path
bypasses it. Plus the refactored weather tool tests confirm the handler contract is intact.
