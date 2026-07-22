# BUG-061-007 — Report

## Summary

The explicit `/weather <location>` slash command was routed through the LLM tool-call loop;
when the self-hosted model (`qwen3:30b`) failed to emit the `weather_lookup` tool call, the
provider-error was masked by the provenance gate as `saved as an idea — i'll surface it
later.`. The fix adds a deterministic `/weather` fast-path (Step 3.9) that dispatches the
weather tool directly for the explicit shortcut — rendering the forecast (with provider
attribution) on success, or an honest "unavailable" line on failure — bypassing the LLM loop
and the capture-as-fallback gate. Backward-compatible: the pre-existing LLM `/weather` path
is untouched when the seam is not wired.

## Completion Statement

Code-complete and unit-verified. One scope; three adversarial regression tests GREEN; the
weather tool handler refactor preserves its contract; both changed Go packages compile and
pass; adversarial regression-quality guard PASS. Live-stack validation on the running
self-hosted bot is pending the home-lab deploy of the fixed SHA.

## Root cause (code-path trace) {#repro-red}

Traced from the deployed `assistant_turn` audit for the `/weather <us-zip>` turn:

```text
scenario_id = weather_query   band = high   status = saved_as_idea
outcome = provider-error
outcome_detail = error=llm_returned_no_tool_calls_and_no_final
provider = ollama   model = ollama_chat/qwen3:30b-a3b
```

`weather_query` uses `direct_output_from_tool: weather_lookup`, which short-circuits only
AFTER the model emits the tool call. The local model emitted neither a tool call nor a final,
so the executor returned `OutcomeProviderError`; because `weather_query` is
`requires_provenance` and the assembler produced empty Sources, the provenance gate rewrote
the error to the capture-as-fallback body. Geocoding was never the cause (Open-Meteo resolves
the ZIP directly). The three adversarial tests below wire the executor stub to reproduce this
exact provider-error and assert the fast-path bypasses it.

## After Fix — unit evidence {#after-fix-unit-evidence}

Command (run through the repo CLI in the isolated Go container):

```text
$ ~/smackerel/smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut|TestWeatherLookup|TestHandleWeatherLookup|TestFacadeWeatherIntegration|TestFacade_BandHigh_StructuredContextPopulated|TestFacadeHighBandProvenanceGate'
[go-unit] applying -run selector: TestFacadeWeatherShortcut|TestWeatherLookup|TestHandleWeatherLookup|TestFacadeWeatherIntegration|TestFacade_BandHigh_StructuredContextPopulated|TestFacadeHighBandProvenanceGate
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core 0.259s [no tests to run]
ok      github.com/smackerel/smackerel/internal/agent   0.037s [no tests to run]
ok      github.com/smackerel/smackerel/internal/agent/tools/weather     0.055s
ok      github.com/smackerel/smackerel/internal/assistant       0.268s
```

- `internal/agent/tools/weather` ran its tests and passed (no `[no tests to run]` suffix) —
  the `LookupForecast` extraction + handler delegation preserves the weather_lookup contract
  (`TestWeatherLookup_NotConfigured`, `TestHandleWeatherLookup_EmptyLocation_Errors`,
  `TestWeatherLookup_ProviderError`, `TestHandleWeatherLookup_WindowsStillAccepted`, …).
- `internal/assistant` ran its tests and passed — the 3 new adversarial fast-path tests
  (`TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor`,
  `_ProviderError_HonestUnavailable_NotSavedAsIdea`,
  `_EmptyLocation_HonestPrompt_NoLookup`) plus the pre-existing weather integration tests
  (`TestFacadeWeatherIntegration_BS003/BS006`) and the backward-compat executor-path test
  (`TestFacade_BandHigh_StructuredContextPopulated_WeatherQuery`).
- The filtered `go test ./...` compiled the whole module (compile + vet) with zero FAILs.

## Regression quality {#regression-quality}

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/assistant/facade_weather_shortcut_test.go
  BUBBLES REGRESSION QUALITY GUARD
  Bugfix mode: true
ℹ️  Scanning internal/assistant/facade_weather_shortcut_test.go
✅ Adversarial signal detected in internal/assistant/facade_weather_shortcut_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
```

Each test is genuinely adversarial: the executor stub returns the pre-fix
`OutcomeProviderError` (the exact `llm_returned_no_tool_calls_and_no_final` failure), so if
the Step-3.9 fast-path were removed the executor would run, the provenance gate would mask
the error, and the `body != "saved as an idea"` + `executor.invocations == 0` assertions
would fail.

## Build check {#build-check}

```text
$ ~/smackerel/smackerel.sh check
config-validate: OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

## Test Evidence

See "After Fix — unit evidence" and "Regression quality" above. Command exit status: the CLI
printed `[go-unit] go test ./... finished OK` and returned success; the guard returned exit 0.

## Deploy + Live Verification

Pending the home-lab deploy of the fixed SHA (bubbles.devops), then an operator Telegram smoke
test: send `/weather <location>` and confirm a forecast (or an honest unavailable line), never
"saved as an idea".
