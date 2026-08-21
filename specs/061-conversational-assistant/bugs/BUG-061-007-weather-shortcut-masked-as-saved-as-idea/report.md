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
self-hosted bot is pending the `<target>` deploy of the fixed SHA.

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

### Code Diff Evidence

The fix commit of record on `main` is
`2e71d64ae1ebd968e2d7a72f1bfea7515f8dc35e` (authored 2026-07-22 19:06:33 +0000),
subject `fix(061): BUG-061-007 — deterministic /weather shortcut (no more "saved
as an idea")`.

Every subject below appears TWICE. Only one commit of each pair is reachable from
`HEAD`; the other is orphaned. The reachable one is the commit of record, and it
was selected by execution rather than by picking the first line of the log:

```text
$ git log --oneline --all --grep='BUG-061-007'
0d5f9d51 docs(061): BUG-061-007 — record home-lab deploy + live infra-verification evidence
fe571649 docs(061): BUG-061-007 — record home-lab deploy + live infra-verification evidence
f213a681 chore(deps): bump google.golang.org/grpc v1.81.1 -> v1.82.1 (GHSA-hrxh-6v49-42gf)
bcfbcb2c chore(deps): bump google.golang.org/grpc v1.81.1 -> v1.82.1 (GHSA-hrxh-6v49-42gf)
2f35e26f fix(061): BUG-061-007 — deterministic /weather shortcut (no more "saved as an idea")
2e71d64a fix(061): BUG-061-007 — deterministic /weather shortcut (no more "saved as an idea")

$ git merge-base --is-ancestor <sha> HEAD   # per candidate, exit 0 = reachable
2f35e26f : ORPHANED (not an ancestor of HEAD) : branches=[none]
2e71d64a : ANCESTOR-OF-HEAD : branches=[ ... * main   remotes/origin/main ]
f213a681 : ORPHANED (not an ancestor of HEAD) : branches=[none]
bcfbcb2c : ANCESTOR-OF-HEAD : branches=[ ... * main   remotes/origin/main ]
0d5f9d51 : ORPHANED (not an ancestor of HEAD) : branches=[none]
fe571649 : ANCESTOR-OF-HEAD : branches=[ ... * main   remotes/origin/main ]
d4755abd : ORPHANED (not an ancestor of HEAD) : branches=[none]
```

Per-file delta of the commit of record, as executed:

```text
$ git show --numstat --format='' 2e71d64a
12      0       cmd/core/wiring_assistant_facade.go
21      7       internal/agent/tools/weather/tool.go
105     0       internal/assistant/facade.go
202     0       internal/assistant/facade_weather_shortcut_test.go
59      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/bug.md
63      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/design.md
104     0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/report.md
9       0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/scenario-manifest.json
49      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/scopes.md
55      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/spec.md
59      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/state.json
16      0       specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea/uservalidation.md

$ git show --stat --format='' 2e71d64a   # trailing summary line
 12 files changed, 754 insertions(+), 7 deletions(-)
```

Non-artifact delta — the four files that carry the behavior change:

| File | + | − | Role in the fix |
|------|---|---|-----------------|
| `cmd/core/wiring_assistant_facade.go` | 12 | 0 | wires the `WithWeatherLookup` seam to `weather.LookupForecast(…, WindowNow)` |
| `internal/agent/tools/weather/tool.go` | 21 | 7 | extracts exported `LookupForecast`; the tool handler delegates to it (tool contract unchanged) |
| `internal/assistant/facade.go` | 105 | 0 | `weatherLookup` seam, `WithWeatherLookup`, Step 3.9 fast-path, `handleWeatherShortcut` |
| `internal/assistant/facade_weather_shortcut_test.go` | 202 | 0 | the three adversarial scenario tests |

The remaining 8 files in the commit are this packet's own artifacts under `specs/`,
which are planning surface and are NOT counted as behavior delta.

#### Traceability finding — the recorded `sourceSha` is orphaned {#code-diff-orphaned-sha}

`state.json` records `deployment.sourceSha: "d4755abd"`. That object exists but is
NOT an ancestor of `HEAD` and no branch contains it, so it is orphaned; the commit
of record for that same grpc bump is `bcfbcb2c`. The same holds for the fix itself
(`2f35e26f` orphaned, `2e71d64a` of record). The built artifact is unaffected,
because each orphaned commit and its on-`main` counterpart carry an IDENTICAL tree:

```text
$ git diff --stat 2f35e26f 2e71d64a
$ git diff --stat d4755abd bcfbcb2c
(both produced no output — the trees are identical)
```

So the running binary corresponds to the code on `main`; the defect is traceability
only. This section cites `2e71d64a` for that reason. The recorded `sourceSha` was
left as measured rather than silently rewritten, because this pass can prove tree
equivalence but cannot prove which object the build host actually consumed.

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

## Scenario Re-Verification (this session) {#scenario-reverification}

The three scenario tests were re-executed in the CURRENT session, with `--verbose`,
so that each scenario is bound by its OWN `--- PASS:` line instead of being inferred
from a package-level `ok`. That distinction is load-bearing here: under a `-run`
selector a package that matched nothing still prints `ok <pkg> <time> [no tests to
run]`, so a package-level `ok` cannot by itself separate "ran and passed" from
"matched nothing". An earlier non-verbose run in this session returned exit 0 but
left one of the three tests unaccounted for in the captured output, which is why the
verbose re-run was performed rather than treating exit 0 as sufficient.

```text
$ ./smackerel.sh test unit --go --go-run '^TestFacadeWeatherShortcut_' --verbose
COMBINED_EXIT=0
--- per-test markers ---
118:=== RUN   TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor
120:=== RUN   TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea
122:=== RUN   TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup
127:--- PASS: TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup (0.00s)
130:--- PASS: TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea (0.00s)
132:--- PASS: TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor (0.00s)
--- assistant package result ---
134:ok          github.com/smackerel/smackerel/internal/assistant       0.265s
--- any FAIL anywhere ---
0
```

The package line carries no `[no tests to run]` suffix, all three tests emit both a
`=== RUN` and a `--- PASS`, and the count of `FAIL`-shaped lines across the whole
run is 0.

**Claim Source:** executed. Scenario-to-assertion binding, read from
`internal/assistant/facade_weather_shortcut_test.go` at the cited line ranges:

| Scenario | Test | Assertions that bind the scenario's claim |
|----------|------|-------------------------------------------|
| SCN-061-007-01 | `TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor` | `resp.Body == forecastLine`; `resp.Status == StatusAnswered`; `resp.Body != captureFallbackAcknowledgement`; `resp.CaptureRoute == false`; exactly 1 `SourceExternalProvider` with `ProviderName == "open-meteo"`; `executor.invocations == 0`; lookup called once with the stripped tail `"90210"` |
| SCN-061-007-02 | `TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea` | lookup stub returns an error; `resp.Status == StatusUnavailable`; `resp.ErrorCause == ErrProviderUnavailable`; `resp.Body != captureFallbackAcknowledgement`; `resp.CaptureRoute == false`; `executor.invocations == 0` |
| SCN-061-007-03 | `TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup` | bare `/weather`; `lookupCalls == 0` (the provider is never called); `resp.Status == StatusUnavailable`; `resp.ErrorCause == ErrSlotMissing`; `resp.Body != captureFallbackAcknowledgement` |

`captureFallbackAcknowledgement` is the real product constant, not a test-local
string — `internal/assistant/facade.go:60` defines it as
`"saved as an idea — i'll surface it later."`, which is the exact body this bug
reported. Each test therefore asserts against the true masking string.

**Mechanism level, stated so the DoD items cannot be read as more than they are:**
these are `unit` tests exercising the facade through the injected `WithWeatherLookup`
seam with a stubbed provider. They bind the routing and rendering decision — which is
precisely where this bug lived — and they do NOT observe the deployed bot.

## E2E Coverage — measured absence, and why no row was written {#e2e-coverage-absence}

Check 8A of the transition guard reports three blocking items for this scope: a
missing scenario-specific regression E2E DoD item, a missing broader E2E regression
suite DoD item, and a missing scenario-specific regression E2E Test Plan row. All
three are CORRECT and are left standing.

No `e2e-api` or `e2e-ui` test binds SCN-061-007-01/02/03. This packet's coverage for
these scenarios is `unit`, which is exactly what `scenario-manifest.json` declares
(`"requiredTestType": "unit"` on all three), so the manifest does not overstate it
either.

The Test Plan row was NOT written, and the reason is mechanical rather than stylistic.
`.github/bubbles/scripts/guards/planning-checks.sh:72` decides that sub-check with:

```text
grep -Eiq '^\|.*Regression E2E' "$scope_path" || grep -Eiq '^\|.*e2e-(api|ui).*(\||`).*Regression:' "$scope_path"
```

The first alternative matches the literal text `Regression E2E` anywhere in a table
row. Every row in this scope's Test Plan has category `unit`, so typing that string
into one of them would flip the sub-check to PASS while describing those rows as a
test category they are not. That is a false green: the gate would report coverage
this packet does not have, and the misdescription would then be read downstream as
fact. The honest state is the red one.

The same disclosure applies to the two DoD sub-checks, and it is recorded here rather
than quietly used. Both are presence-matched with `^\- \[(x| )\] …`, so the checkbox
may be UNCHECKED and the sub-check still passes — adding either line would clear two
of the three Check 8A failures without any change in the packet's real coverage.
Neither line was added for that reason.

Closing this gap needs an `e2e` test that drives an explicit `/weather` turn against a
provider-enabled assistant stack and asserts the rendered body is the forecast (or the
honest unavailable line) and never the capture acknowledgement. That is a harness
change outside this bug's four-file fix surface; the owner is the assistant e2e
harness owner, not this packet.

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

## Deploy + Live Verification (`<target>`) {#deploy-verify}

The fix (sourceSha `d4755abd`, which also carries a grpc → v1.82.1 CVE bump that
unblocked the Trivy gate — see below) was built + operator-cosign-signed + deployed
to the running `<deploy-host>` and verified live this session
(`local-operator` trust model).

### Build + sign (knb-owned configured tier on `<deploy-host>`)

`smackerel.sh build --target <target>` — 9/9 phases green:
- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities). The first build FAILED the gate
  on a HIGH in `google.golang.org/grpc v1.81.1` (GHSA-hrxh-6v49-42gf, newly in Trivy's
  DB); bumping grpc to v1.82.1 (commit `d4755abd`) cleared it.
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/<operator>/smackerel-core@sha256:44ed9984…`
  - ml   `ghcr.io/<operator>/smackerel-ml@sha256:30ea2392…`
- Config bundle `config-bundle-<target>-d4755abd…` (sha256 `ea288c7b…`) pushed + signed.

### Deploy (on-host local-operator apply → recreate)

`<knb-repo>/scripts/deploy/promote.sh --target <target> --product smackerel --local-build-manifest <manifest> --operator <operator>`
(on-host, under passwordless sudo, with the operator cosign pubkey + ghcr docker-config).
The adapter verified the release proof (cosign verified BOTH images + attestations against
the operator pubkey), decrypted the bundle secrets (mode 0600), and recreated
`smackerel-core` + `smackerel-ml` (infra services stayed healthy).

### Live running-state verification (this session, read-only)

```text
smackerel-<target>-smackerel-core-1 | running/healthy | sha256:44ed9984… | MATCHES CORE FIX
smackerel-<target>-smackerel-ml-1   | running/healthy | sha256:30ea2392… | MATCHES ML FIX
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
