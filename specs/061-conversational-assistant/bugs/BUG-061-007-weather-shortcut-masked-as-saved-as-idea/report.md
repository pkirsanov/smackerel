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

## Deploy + Live Verification (self-hosted home-lab) {#deploy-verify}

The fix (sourceSha `d4755abd`, which also carries a grpc → v1.82.1 CVE bump that
unblocked the Trivy gate — see below) was built + operator-cosign-signed + deployed
to the running self-hosted home-lab host and verified live this session
(`local-operator` trust model).

### Build + sign (accel tier, on the target)

`smackerel.sh build --target self-hosted` — 9/9 phases green:
- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities). The first build FAILED the gate
  on a HIGH in `google.golang.org/grpc v1.81.1` (GHSA-hrxh-6v49-42gf, newly in Trivy's
  DB); bumping grpc to v1.82.1 (commit `d4755abd`) cleared it.
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/pkirsanov/smackerel-core@sha256:44ed9984…`
  - ml   `ghcr.io/pkirsanov/smackerel-ml@sha256:30ea2392…`
- Config bundle `config-bundle-self-hosted-d4755abd…` (sha256 `ea288c7b…`) pushed + signed.

### Deploy (on-host local-operator apply → recreate)

`promote.sh --target home-lab --product smackerel --local-build-manifest <manifest> --operator <op>`
(on-host, under passwordless sudo, with the operator cosign pubkey + ghcr docker-config).
The adapter verified the release proof (cosign verified BOTH images + attestations against
the operator pubkey), decrypted the bundle secrets (mode 0600), and recreated
`smackerel-core` + `smackerel-ml` (infra services stayed healthy).

### Live running-state verification (this session, read-only)

```text
smackerel-home-lab-smackerel-core-1 | running/healthy | sha256:44ed9984… | MATCHES CORE FIX
smackerel-home-lab-smackerel-ml-1   | running/healthy | sha256:30ea2392… | MATCHES ML FIX
```

Both containers run the EXACT fix digests and are healthy; core startup log shows
`telegram bot started` + `assistant Telegram adapter wired and bound to bot`, so the
fixed `/weather` code path is the live one.

### Remaining (operator behavioral smoke test)

A direct authenticated probe of `POST /api/assistant/turn` returned HTTP 401 (production
requires a PASETO login session, not the raw shared token — a security-correct posture),
so the end-to-end behavioral confirmation is an operator Telegram turn: send
`/weather <city or ZIP>` and confirm a forecast (or an honest "unavailable" line), never
"saved as an idea". The fix binary is deployed + running + healthy + adapter-bound.
