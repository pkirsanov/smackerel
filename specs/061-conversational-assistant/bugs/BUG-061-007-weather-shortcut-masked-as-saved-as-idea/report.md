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
0d5f9d51 docs(061): BUG-061-007 — record <deploy-target> deploy + live infra-verification evidence
fe571649 docs(061): BUG-061-007 — record <deploy-target> deploy + live infra-verification evidence
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

The `0d5f9d51` / `fe571649` subjects are quoted with the concrete deployment-target
name redacted to `<deploy-target>` per the product-deployment-boundary policy; the
commit SHAs are unaltered and remain the verifiable anchor, so `git show <sha>`
resolves the real subject.

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

## Regression phase (`bubbles.regression`) {#regression-phase}

**Claim Source:** every command below was executed in this session against `HEAD=063a17fb`
on a clean tree. Exit codes are the observed values, not expectations.

### 1. Host-preflight override — disclosed, not buried {#regression-preflight-override}

`./smackerel.sh test …` is gated by `host_resource_preflight`, which refuses on this host.
Run standalone, first-hand:

```text
$ disk-preflight.sh
  ┌─ disk-preflight: REFUSED — not enough free disk ──────────────────┐
  │  C: (backs the vhdx): 34     GB free   required: 40   GB
  │  WSL / (ext4)       : 495    GB free   required: 25   GB
  │  Current Docker footprint:
  │      Images          119   58   61.41GB   21GB (34%)
  │      Local Volumes   112   24   165.5GB   39.73GB (24%)
  │      Build Cache     638    0   27.05GB   14.6GB
  └────────────────────────────────────────────────────────────────────┘
DISK_PREFLIGHT_EXIT=1
```

Every `test` run in this phase therefore carried `SMACKEREL_SKIP_HOST_PREFLIGHT=1`. That is
the CLI's **own documented opt-out**, not an invented bypass — `smackerel.sh:715` reads
`Opt out with SMACKEREL_SKIP_HOST_PREFLIGHT=1`, and `smackerel.sh:726` implements it. The
same comment block scopes the guard to *"heavy (multi-minute, multi-GB) commands"* whose
failure modes are *"OOM-kill … when the shared WSL host is out of RAM"* and a
*"disk-full wedge … [when] a heavy build can fill the disk"*. `test unit --go` is neither: it
is a `docker run --rm` against an already-present image with warm caches, and it builds no
image, so the guard's premise does not hold for it. The override is recorded here rather
than silently applied.

The remedy the banner suggests first — `docker-safe-prune.sh --apply` — was deliberately
**NOT** run. The measured footprint above shows 14.6 GB reclaimable build cache and 39.73 GB
reclaimable volumes; that cache is shared with sibling repositories whose builds were active,
so reclaiming it to satisfy a guard that does not apply to this command would have destroyed
other work. Declining the suggested remedy is part of the finding, not an omission.

### 2. Test baseline — full Go unit suite {#regression-baseline}

```text
$ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core                       1.474s
ok      github.com/smackerel/smackerel/internal/agent/tools/weather   (cached)
ok      github.com/smackerel/smackerel/internal/api                   6.454s
ok      github.com/smackerel/smackerel/internal/assistant             (cached)
ok      github.com/smackerel/smackerel/internal/auth                  3.200s
ok      github.com/smackerel/smackerel/internal/config                45.504s
…
[go-unit] go test ./... finished OK
UNIT_GO_EXIT=0
```

Whole-module `go test ./...`: **exit 0**, zero `FAIL` lines, zero `--- FAIL` lines. No
previously-passing test regressed.

| Category | Command | Result | Status |
|---|---|---|---|
| `unit` (Go, whole module) | `test unit --go` | `finished OK`, exit 0, 0 FAIL | 🟢 CLEAN |
| `unit` (3 BUG-061-007 tests) | `test unit --go --go-run 'TestFacadeWeatherShortcut'` | exit 0 | 🟢 CLEAN |
| `integration` | see [§5](#regression-integration) | `PASS: go-integration` + `PASS: python-integration`, exit 0 | 🟢 CLEAN |

### 3. Mutation re-derivation — the tests are genuinely adversarial {#regression-mutation}

Coverage that cannot fail is not coverage. One mutant was re-derived **first-hand** this
session rather than inherited from a prior pass.

**Mutant M1 — relational-operator replacement on the fast-path guard** (`facade.go:1006`),
inverting `!= nil` to `== nil` so the fast path can never fire when the seam IS wired, which
is exactly the pre-fix behaviour:

```text
$ git diff -- internal/assistant/facade.go
index 139510ff..31174bc3 100644
-  if msg.Kind == contracts.KindText && shortcutScenarioID == "weather_query" && f.weatherLookup != nil {
+  if msg.Kind == contracts.KindText && shortcutScenarioID == "weather_query" && f.weatherLookup == nil {

$ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut'
--- FAIL: TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup (0.00s)
    facade_weather_shortcut_test.go:191: executor invoked 1 times; want 0
    facade_weather_shortcut_test.go:197: error_cause = "provider_unavailable"; want "slot_missing"
--- FAIL: TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea (0.00s)
    facade_weather_shortcut_test.go:152: executor invoked 1 times; want 0
--- FAIL: TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor (0.00s)
    facade_weather_shortcut_test.go:102: executor invoked 1 times; the /weather fast-path MUST bypass the LLM (want 0)
    facade_weather_shortcut_test.go:105: weather lookup called 0 times; want exactly 1
    facade_weather_shortcut_test.go:108: lookup location = ""; want "90210" (the stripped shortcut tail)
    facade_weather_shortcut_test.go:111: status = "unavailable"; want "answered"
    facade_weather_shortcut_test.go:114: body = "the service is unavailable right now — …"; want the forecast line "Beverly Hills, CA: clear, 22°C"
    facade_weather_shortcut_test.go:125: len(Sources) = 0; want 1 provider attribution source
FAIL    github.com/smackerel/smackerel/internal/assistant       0.282s
M1_MUTANT_EXIT=1
```

**M1 KILLED by all three tests.** Tree then restored and proven byte-identical — not merely
"reverted", but hash-equal to the pre-mutation blob:

```text
$ git hash-object internal/assistant/facade.go   # before mutation
139510ffc375b310e2dd8c4309afe7b07e085edb
$ git hash-object internal/assistant/facade.go   # after restore
139510ffc375b310e2dd8c4309afe7b07e085edb
$ git status --porcelain
PORCELAIN_LINES=0

$ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go --go-run 'TestFacadeWeatherShortcut'
ok      github.com/smackerel/smackerel/internal/assistant       0.262s
[go-unit] go test ./... finished OK
M1_RESTORED_EXIT=0
```

#### Honest nuance — which assertion actually discriminates in SCN-061-007-03

The mutant output above is more informative than a bare pass/fail, and it corrects a claim
that a coarser reading would have made. In `SCN-061-007-03`
(`_EmptyLocation_HonestPrompt_NoLookup`) the assertion `lookupCalls == 0` did **NOT** fire
under M1 — with the fast path disabled the lookup is never called *either*, so that line is
satisfied by both the fixed and the broken build and is **not individually discriminating**.
The two assertions that actually killed the mutant for that test are the ones printed above:
`executor invoked 1 times; want 0` (line 191) and `error_cause = "provider_unavailable"; want
"slot_missing"` (line 197).

A further first-hand correction: the capture-acknowledgement assertion did not fire for that
test either. Under M1 this unit harness produced `status=unavailable
error_cause=provider_unavailable` — visible in the emitted `assistant_turn` audit line — not
`saved_as_idea`, because no source-assembler is registered in the harness. So SCN-061-007-03
is adversarial via the executor-invocation and error-cause assertions **only**. Recorded as
measured rather than as assumed.

### 4. Cross-spec impact — no conflict with BUG-061-008 {#regression-cross-spec}

Both packets touch the same file (`internal/assistant/facade.go`) and the same scenario ID
(`weather_query`), which is the shape that usually indicates interference. Verified
first-hand that it does not, here:

```text
$ awk 'NR==36' internal/assistant/facade_execution_error_honesty_test.go
var requiresProvenanceScenarios = []string{"weather_query", "retrieval_qa", "recipe_search", "open_knowledge"}

$ awk 'NR==1006' internal/assistant/facade.go
  if msg.Kind == contracts.KindText && shortcutScenarioID == "weather_query" && f.weatherLookup != nil {
```

The two are **complementary, not overlapping**:

| | BUG-061-008 | BUG-061-007 (this packet) |
|---|---|---|
| Path exercised | `weather_query` + `OutcomeProviderError` through the **UNWIRED** seam | explicit `/weather` shortcut through the **WIRED** seam |
| Guard | `requiresProvenanceScenarios` sweep | `f.weatherLookup != nil` (`facade.go:1006`) |
| Reachable together? | No — the guard is mutually exclusive on `weatherLookup` nil-ness | |

The fast path also `return`s at `facade.go:1010`, **before**
`canonicalizeSuccessfulCaptureResponse` is ever reached; it sets neither `StatusSavedAsIdea`
nor `CaptureRoute`, and it audits `BandHigh` (`f.writeAudit(ctx, msg, BandHigh, …)` at
`facade.go:1009`). It therefore cannot regress the band-LOW-only capture-acknowledgement rule
that BUG-061-008 and the `INV-HB-REFUSAL` invariant protect. The whole-module suite in §2
passing with zero FAILs is the corroborating measurement: `facade_execution_error_honesty_test.go`
and `contracts/refusal_test.go` both ran and both passed.

### 5. Integration suite {#regression-integration}

The broader integration suite **was** executed for this packet, after the artifacts above had
already been written. Both halves passed:

```text
$ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test integration
--- PASS: TestAcceptanceGate_RoutingAccuracyAndCaptureFallback (0.00s)
--- PASS: TestClassify_WeatherSignal (0.00s)
    --- PASS: TestClassify_WeatherSignal/What's_the_weather_like_today? (0.00s)
    --- PASS: TestClassify_WeatherSignal/Will_it_rain_tomorrow? (0.00s)
    --- PASS: TestClassify_WeatherSignal/Forecast_for_Berlin_this_weekend? (0.00s)
--- PASS: TestRun_AdversarialFailureSurfaces (0.00s)
--- PASS: TestRun_AgainstShippedCorpus (0.00s)
ok      github.com/smackerel/smackerel/internal/cardrewards     3.187s
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.034s
PASS: go-integration
[py-integration] live integration pytest finished OK
PASS: python-integration
INTEGRATION_EXIT=0
```

Two details worth recording rather than glossing:

- The weather-routing acceptance tests (`TestClassify_WeatherSignal` and
  `TestAcceptanceGate_RoutingAccuracyAndCaptureFallback`) are the ones most exposed to this
  fix, since it changes what happens to a `weather_query` turn. Both passed, which is direct
  corroboration that the fast path did not disturb routing or the capture-fallback acceptance
  floor.
- The run stood up a **project-scoped ephemeral stack** (`smackerel-test-*`: postgres, nats,
  ollama, searxng, jaeger, stub-providers, core, ml) and tore it down completely on exit —
  every container removed, and the `smackerel-test-postgres-data`, `-nats-data` and
  `-ollama-data` volumes removed with it. No persistent dev or production state was touched.

### 6. Coverage delta

No test was deleted, skipped, or weakened in this phase — the only tree mutation was the
temporary mutant in §3, reverted to a byte-identical blob and proven so by hash. The
scenario-specific E2E gap for `SCN-061-007-01/02/03` is **unchanged and still open**; it is
recorded in `state.json` `certification.pendingGates` and is not closed by this phase. Per
`planning-checks.sh:72`, which text-matches the literal string `Regression E2E` in any Test
Plan row, no such row was added to a `unit`-category row: doing so would turn that gate green
while misdescribing the test category. That gate stays red honestly.

### Verdict

```text
🟢 REGRESSION_FREE

Test baseline (unit, whole module): exit 0, zero FAIL — no previously-passing test regressed
Integration suite: exit 0 — go-integration PASS, python-integration PASS
Mutation (M1, re-derived first-hand): KILLED by 3/3 tests; tree restored byte-identical
Cross-spec conflicts: 0 (BUG-061-008 verified complementary, not overlapping)
Design contradictions: 0
Coverage delta: 0 removed / 0 weakened / 0 skipped
Still open (NOT closed by this phase): scenario-specific E2E for SCN-061-007-01/02/03;
  operator live Telegram turn; five specialist phases (simplify, stabilize, security,
  audit, validate)
```

---

## Simplify phase (`bubbles.simplify`) {#simplify-phase}

**Claim Source:** every command below was executed in THIS session against
`<repo-root>`; repository binding `rb:vscode-6af2178e10192363b0e52a46fb5e0950:35`
(`PREFLIGHT_COMMITTED`, `actionable: true`, control revision 35).

### Review surface {#simplify-surface}

`git show --stat 2f35e26f` (exit 0) — 12 files, 754 insertions / 7 deletions; the four
non-artifact files are the fix surface already recorded in `affectedSurface`. Commit subject
is not quoted here: the pre-push deployment-boundary scanner does not strip code fences, and
the SHA is a sufficient anchor.

Files read in full for this pass:

| File | Region reviewed |
|---|---|
| `internal/assistant/facade.go` | Step 3.9 guard (`:1006`), `weatherLookup` field + `WithWeatherLookup` (`:204`–`:336`), `handleWeatherShortcut` (`:1492`–`:1544`) |
| `internal/assistant/compiled_interactions.go` | `handleResolvedCompiledWeather`, `compiledWeatherResponse`, `compiledWeatherFailure` (`:301`–`:430`) |
| `internal/agent/tools/weather/tool.go` | `handleWeatherLookup` + extracted `LookupForecast` (`:266`–`:310`) |
| `internal/assistant/facade_weather_shortcut_test.go` | all 3 tests, full file |
| `cmd/core/wiring_assistant_facade.go` | import block + seam wiring |

### Finding S-1 (ACCEPTED, fixed) — two test names promised more than their assertions bound

Both failure-path tests named an honest **user-visible body** in their title but asserted only
the structural status pair, never the body itself. An empty `Body` satisfied every assertion in
each test.

| Test | Name promises | Was bound | Gap |
|---|---|---|---|
| `..._ProviderError_HonestUnavailable_NotSavedAsIdea` | an honest *unavailable* line | `status`, `error_cause`, `body != capture-ack`, `!CaptureRoute` | body never asserted to be the fast-path failure line |
| `..._EmptyLocation_HonestPrompt_NoLookup` | a *prompt* asking for a location | `status`, `error_cause`, `body != capture-ack`, `lookupCalls == 0` | body never asserted to be a prompt |

This is the exact class this packet family has been bitten by, and it is not cosmetic here: the
defect under repair *is* a wrong user-visible body. A test that binds only `status` cannot see a
body regression.

The second row matters more than it looks. Under mutant **M1** (`f.weatherLookup != nil` →
`== nil`, re-derived in the regression phase) the executor path also terminates at
`status=unavailable` / `error_cause=provider_unavailable` — so for
`_ProviderError_HonestUnavailable_` the status pair is **non-discriminating**, and its only
discriminator was `executor.invocations == 0`. Binding the fast-path copy adds a genuine second
independent discriminator rather than restating the first.

**Fix applied** — assertions strengthened, no production code touched:

```go
// _ProviderError_HonestUnavailable_NotSavedAsIdea
if !strings.Contains(resp.Body, "90210") {
    t.Errorf("body = %q; want the fast-path failure line naming the requested location %q", resp.Body, "90210")
}

// _EmptyLocation_HonestPrompt_NoLookup
if !strings.Contains(resp.Body, "location") {
    t.Errorf("body = %q; want a prompt asking for a location", resp.Body)
}
```

Plus the `strings` import. Diff is 3 hunks, +12 lines, in one file.

Both new assertions are **non-vacuous by construction**: production emits
`"couldn't get the weather for %s right now — please try again."` (facade.go:1508) and
`"which location? try \`/weather <city or ZIP>\`."` (facade.go:1498), so an empty or
substituted body fails `strings.Contains`. **Honest limitation:** that non-vacuity is argued
from reading the production strings and from the suite passing — a mutant that blanks each body
was **not** executed this pass.

### Finding S-2 (REVIEWED, DECLINED) — apparent duplication between the two weather renderers

`handleWeatherShortcut` builds a `SourceExternalProvider` attribution block, and so does
`compiledWeatherResponse`. Extracting a shared renderer was considered and **rejected**: the
two differ on four axes that the caller must decide anyway.

| Axis | `handleWeatherShortcut` | `compiledWeatherResponse` |
|---|---|---|
| Input | `json.RawMessage` from the seam | typed `weather.Forecast` |
| Status | `StatusAnswered` | `StatusCheckingWeather` |
| `Source.ID` | `"weather-provider"` | `provider:retrievedAtUnixNano` |
| Strictness | forecast line required; attribution best-effort | line + provider + non-zero `RetrievedAt` all required, else error |
| Provenance gate | deliberately bypassed (this is the fix) | `provenance.Enforce` applied |

A shared helper would need all four parameterized — larger than the ~10 duplicated lines — and
would re-couple the fast path to the compiled path it exists to bypass. Declined as taste churn.

The duplication that **did** matter was already removed by the fix itself: `LookupForecast` is
an extraction, not a copy, and both `handleWeatherLookup` and the facade seam call it, so
provider + cache + `RetrievedAt` invariants have exactly one implementation.

### Observation S-3 (RECORDED, not changed) — best-effort attribution branch is untested

When `payload.ProviderName` is empty, `handleWeatherShortcut` returns `StatusAnswered` with a
forecast body and **zero** `Sources`, and the `RetrievedAt` parse error is discarded
(`retrievedAt, _ := time.Parse(...)`). No test covers that branch. It is not reachable through
the wired production path — `LookupForecast` stamps `ProviderName` from `svc.Provider.Name()` —
so it is a latent defensive branch, not a live defect. Left unchanged: touching production code
on a packet already verified `REGRESSION_FREE`, to harden an unreachable branch, is exactly the
churn this phase is told not to invent. Routed as an observation for `bubbles.plan`.

### Dead code / unused imports {#simplify-dead-code}

None found. Unused imports and unused local variables are **compile errors** in Go, so the
`go build`/`go vet` stage inside `go test ./...` returning exit 0 is positive evidence for the
whole module, not an assertion. No commented-out code, `TODO`, `FIXME`, or unreachable branch
was observed in the four fix-surface files.

### Verification {#simplify-verification}

**Host-preflight override — disclosed, not buried.** `SMACKEREL_SKIP_HOST_PREFLIGHT=1` was set
for the run below, the same documented opt-out (`smackerel.sh:715` documents it, `:726`
implements it) and for the same reason as the regression phase: `disk-preflight.sh` refuses on
this host (C: ~34 GB free vs a 40 GB threshold) and the guard is scoped by its own comment to
heavy multi-GB build commands — `test unit --go` builds no image. `docker-safe-prune.sh --apply`
was deliberately **not** run: ~14.6 GB of the reclaimable cache is shared with sibling
repositories whose builds were active.

```text
$ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go
exit: 0
lines: 207
sha256: 30bf91163102e3bad7c4e82a7d1aef503797350e61bdef6396af8b470de2fa92
--- last 20 (excerpt) ---
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/admin       (cached)
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter      (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/eval/assistant     (cached)
ok      github.com/smackerel/smackerel/tests/observability      (cached)
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
ok      github.com/smackerel/smackerel/tests/unit/clients       (cached)
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
[go-unit] go test ./... finished OK
```

Captured via `.github/bubbles/scripts/evidence-capture.sh`; the sha256 covers all 207 produced
lines and is re-derivable with `--verify`. `internal/assistant` was edited this pass, so it
could **not** serve from the test cache — it recompiled and re-ran, and the module-wide exit 0
therefore covers the three strengthened tests.

### Commands executed this phase

| # | Command | Exit | Result |
|---|---|---|---|
| 1 | `repository-binding-host-context.sh --session-log … --workspace-root <repo-root>` | 0 | `expectedControlRevision=34` |
| 2 | `repository-binding.sh preflight --request-class STRUCTURED` | 0 | `PREFLIGHT_COMMITTED` revision=35, `actionable: true` |
| 3 | `git show --stat 2f35e26f` | 0 | 12 files, +754/−7 — fix surface confirmed |
| 4 | `git status --porcelain \| wc -l` | 0 | `0` — clean tree before edits |
| 5 | `SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go` | 0 | `[go-unit] go test ./... finished OK` — zero FAIL, module-wide |

### Verdict

```text
🟢 SIMPLIFIED — one accepted finding, one declined, one recorded

Findings accepted + fixed : 1 (S-1 — 2 test names stronger than their assertions)
Findings declined         : 1 (S-2 — cross-renderer duplication; abstraction > duplication)
Observations recorded     : 1 (S-3 — untested defensive attribution branch, unreachable in prod)
Production code changed   : 0 files
Test code changed         : 1 file, 3 hunks, +12 lines
Dead code / unused imports removed : 0 (none present; proved by the Go compiler, exit 0)
Files deleted             : 0 (no deletion candidate arose; deletion-safety gate not triggered)
Verification              : test unit --go exit 0, zero FAIL

Still open (NOT closed by this phase): scenario-specific E2E for SCN-061-007-01/02/03;
  operator live Telegram turn; the 1 unchecked DoD item; four specialist phases
  (stabilize, security, audit, validate)
Deliberately not written: certification.certifiedCompletedPhases (bubbles.validate owns it);
  certification.pendingGates (foreign-owned; its stale G022 sentence is reported, not edited);
  uservalidation.md (G136); the unchecked live-stack DoD item
```

---

## Stabilize phase (`bubbles.stabilize`) {#stabilize-phase}

**Agent:** `bubbles.stabilize` · **Verdict:** 🟢 STABLE (0 stability defects in the
changed surface; 2 non-blocking follow-ups routed) · **Production code changed: 0 files**

### Surface reviewed

The four non-artifact files of fix commit `2f35e26f`, read in full, plus every
collaborator the new fast path reaches at runtime:

| File | Role in the fast path |
|---|---|
| `internal/assistant/facade.go` | Step 3.9 guard (`:1006`) + `handleWeatherShortcut` (`:1492`), rendering at `:1498`/`:1508`; seam field `:213`, `WithWeatherLookup` `:333` |
| `internal/agent/tools/weather/tool.go` | `LookupForecast` (extracted by the fix), `loadServices`, `servicesMu`, tool registration |
| `cmd/core/wiring_assistant_facade.go` | seam wiring (`:241`), adapter publication (`:409`) |
| `internal/agent/tools/weather/cache.go` | in-process LRU reached on every lookup |
| `internal/agent/tools/weather/open_meteo.go` | the two upstream round trips |
| `cmd/core/wiring_assistant_skills.go` | provider + HTTP client + cache construction |
| `internal/assistant/httpadapter/late_binding.go` | the publication barrier between wiring and request goroutines |

### 1. Goroutine / lifecycle — CLEAN

The fast path starts nothing that needs stopping. A whole-file scan of
`facade.go` for `go func` / `go <ident>(` / `time.After` / `time.NewTicker` /
`time.NewTimer` returns **zero matches**, so there is no goroutine or timer to
leak per turn anywhere in the facade, let alone on this path.

`handleWeatherShortcut` is a straight-line function: it trims the location,
calls the seam, unmarshals into a local struct, and returns a value. It opens
no store, holds no lock, registers no cleanup, and allocates nothing that
outlives the return.

On spans specifically: the fast path returns at `facade.go:1010` **before**
Step 4, so it never reaches `routeWithSpan` / `borderlineWithSpan` and creates
no span at all. A span leak requires a span that is started and never ended;
zero started is trivially zero leaked. (That the path is consequently
un-traced is an observability gap, recorded as an observation below — it is
not a leak and not a stability defect.)

### 2. Concurrency — CLEAN, and the verdict is **STATIC**

The premise in the review brief is correct and load-bearing: the assistant HTTP
ingress runs one goroutine per request into one shared `*Facade`, and
`wireAssistantFacade` runs in a **background goroutine while the listener is
already accepting** `POST /api/assistant/turn`
(`cmd/core/main.go:541,564`, `runAssistantFacadeWiringWithRetry`). So the
question of whether a request goroutine can observe a half-wired `Facade` is
real, not hypothetical. Four sub-checks:

**(a) `weatherLookup` is write-once at construction.** The only write site in
the repository is `facade.go:335` inside `WithWeatherLookup`, and the only
caller is `wiring_assistant_facade.go:241`, executed once. Confirmed by
enumerating all 7 references to the identifier. No turn-time write exists.

**(b) The closure captures nothing mutable.** The wired value is
`func(ctx, location) { return weather.LookupForecast(ctx, location, weather.WindowNow) }`
— a package-level function and a package-level constant. There is no captured
variable, so there is no per-turn shared cell to race on.

**(c) The write is published through a synchronizing operation.** This is the
part that actually makes (a) safe rather than merely plausible. In the wiring
goroutine, the plain write at `facade.go:335` is sequenced before
`svc.assistantHTTPHandler.SetAdapter(adapter)` at
`wiring_assistant_facade.go:409`. `SetAdapter` is
`h.adapter.Store(a)` over `adapter atomic.Pointer[HTTPAdapter]`
(`late_binding.go:33,42`), and `ServeHTTP` reads it back with the matching
`Load`. Under the Go memory model an atomic `Store` observed by an atomic
`Load` establishes happens-before, so any request goroutine that reaches the
adapter at all is guaranteed to observe the fully-wired `weatherLookup`. Before
that store, `LateBoundHandler` is fail-closed (503), so there is no window in
which a turn sees a nil seam that "should" have been set. The Telegram adapter
is wired at `:256`, also after `:241`, in the same goroutine, and a `go`
statement is itself a happens-before edge.

**(d) Shared state behind the seam is synchronized.** `weather.services` is
guarded by `servicesMu sync.RWMutex` and read only through `loadServices()`
(`tool.go:126-128,146-149`). The LRU is guarded by `mu sync.Mutex` held across
`Get`, `Put` and `Len` (`cache.go:24,70,95,118`). `handleWeatherShortcut`
writes no `Facade` field. The fast path therefore adds a second concurrent
*caller* of an already-synchronized cache, not a new unsynchronized one.

**Honest limitation — this verdict is STATIC.** It is derived from reading the
code and the memory-model rules above, not from a `-race` execution. The repo
CLI exposes no `--race` selector on `test unit`, and terminal discipline
forbids reaching around it with an ad-hoc `go test -race`. No race detector was
run this phase; that is stated plainly rather than implied away.

### 3. Resource usage — BOUNDED

| Bound | Value | Source |
|---|---|---|
| Per upstream HTTP request | `2 * time.Second` | `wiring_assistant_skills.go:148-149` — `&http.Client{Timeout: 2 * time.Second}` |
| Requests per lookup | 2, sequential (geocode → forecast) | `open_meteo.go:227-231`, `:300-304` |
| Effective ceiling per cold-cache turn | ≈ 4 s | the two bounds above |
| Retries | **none** | no retry/backoff construct exists in the weather package |
| Cache size | 128 entries + TTL | `wiring_assistant_skills.go` `cacheCapacity = 128`, `NewCache(ttl, 128)` |

`http.Client.Timeout` spans connection, redirects and **body read**, so the
unbounded-looking `json.NewDecoder(resp.Body).Decode(&f)` (`open_meteo.go:240`,
`:313`) cannot be held open by a slow or endless body — it is cut at 2 s. There
is no unbounded retry, no per-turn allocation that survives the turn, and the
cache is capacity-bounded, so repeated `/weather` traffic cannot grow memory
without limit.

Worth stating precisely, because it is easy to misread as a defect: the tool's
`PerCallTimeoutMs: 8000` is a property of the **registered agent tool**, applied
by the tool-call loop, and the fast path deliberately bypasses that loop — so
that budget does **not** apply here. It does not need to: the transport-layer
2 s client timeout is strictly tighter and is what actually bounds both paths.
The fast path is therefore bounded by the same mechanism as the tool path, not
by a weaker one.

### 4. Config / deployment reliability — HONEST, fails loud to the user

Two distinct degraded states, both checked:

**Weather skill disabled or misconfigured.** `wireWeatherSkillServices` returns
early when `WeatherEnabled=false` without calling `SetServices`, so
`loadServices()` returns `weather_tools_not_configured`
(`tool.go:146-158`). `LookupForecast` propagates it, and
`handleWeatherShortcut` renders `Status=StatusUnavailable`,
`ErrorCause=ErrProviderUnavailable`, body "couldn't get the weather for
`<location>` right now — please try again." That is the honest outcome: the
user is told weather is unavailable and is **never** told the question was
saved as an idea. This is exactly the masking the bug removed, and it holds on
the config-failure branch too, not just the provider-failure branch.

**Cause-token conflation is forced, not sloppy — non-finding.** A permanent
misconfiguration and a transient upstream outage both surface as
`provider_unavailable`. I checked whether a better token exists before writing
this up: the closed vocabulary in `contracts/response.go:194-229` is
`""`, `provider_unavailable`, `missing_scope`, `slot_missing`,
`internal_error`, `no_match`, `model_not_switchable`, `no_grounded_answer`.
There is no config-missing member, and `internal_error` would be actively
wrong (a disabled skill is an intended state, not an internal failure). So
`provider_unavailable` is the most honest available token. **Not recorded as a
finding.**

**Unwired seam.** `f.weatherLookup == nil` skips Step 3.9 and falls through to
the LLM tool-call path — i.e. the documented, backward-compatible pre-fix
behavior. In production the seam is wired unconditionally (not behind any SST
flag), and per §2(c) no turn can observe it unset, so there is no runtime
silent-degradation path today. The durability of that guarantee is the subject
of follow-up F-2.

### Findings

**Zero stability defects attributable to `2f35e26f` across all four domains.**
The two items below are recorded rather than fixed, and neither makes this
packet unstable.

**F-1 — ADJACENT (not caused by this fix): the raised tool budget cannot be
consumed, and its wiring comment is now factually wrong.**
`PerCallTimeoutMs` was raised `2000 → 8000` by commit `2084cbf5`
("fix(weather,telegram): /weather provider_unavailable + bot DNS-race silent
disable"). Provenance was checked before attributing it: `git show 2f35e26f --
internal/agent/tools/weather/tool.go` contains **no** `PerCallTimeoutMs` hunk,
and `git merge-base --is-ancestor 2084cbf5 2f35e26f` reports **not an
ancestor**, so that change is not in this fix's history at all. The comment
above the client construction (`wiring_assistant_skills.go:135-136`, written
by `0f36093f`) still asserts the client timeout "matches the tool's
PerCallTimeoutMs budget (2s, see …tool.go init())" — that sentence is now
false. Runtime arithmetic remains coherent (2 × 2 s = 4 s < 8 s), so nothing is
broken today; but `tool.go`'s stated rationale — "8s gives ~2x headroom over
the observed worst case" of "~2s per call" — is not achieved for a *single*
slow call, because the tighter 2 s per-request client bound fires first and
still yields `provider_error`. That is the very symptom `2084cbf5` set out to
eliminate. Owner: `bubbles.implement`. Severity: medium (stale doc + partially
ineffective remediation on an adjacent commit). Not this packet's defect and
explicitly **not** grounds for a fix cycle here.

**F-2 — DURABILITY (design property of this fix, not an instability): seam
removal is not test-protected.** `WithWeatherLookup` is nil-safe by *ignoring*
nil (`facade.go:334`), and the seam is optional by design. Deleting
`facade.WithWeatherLookup(...)` from `wireAssistantFacade` would compile
cleanly and leave all three fast-path unit tests **green**, because those tests
call `WithWeatherLookup(lookup)` directly on a locally constructed facade
(`facade_weather_shortcut_test.go:71`). A repo-wide grep confirms that is the
only `_test.go` reference to the identifier: there is **no** wiring-level
assertion that production actually installs the seam. The consequence is that
the pre-fix "saved as an idea" behavior could be silently restored in
production by a one-line deletion with a fully green suite. This is a
regression-durability gap, not a runtime defect — the wiring is present and
unconditional today, and mutation M1 was killed by all three tests. Recording
it rather than escalating: marking the packet UNSTABLE would route a fix cycle
onto code that works. Owner: `bubbles.test` (add a wiring-level guard), with
`bubbles.plan` if it warrants a DoD row.

**Observation (no owner, no action requested):** the fast path emits no OTel
span and no dedicated counter, so `/weather` shortcut turns are invisible in
the assistant's per-turn tracing that Step 4+ turns produce. It is audited
(`writeAudit(..., BandHigh, ...)` at `facade.go:1008`) and persisted, so it is
not unobservable — only less observable than the path it replaces.

### Commands executed (this phase)

| Command | Exit | Result |
|---|---|---|
| `repository-binding-host-context.sh --session-log <host-token> --workspace-root <repo-root>` | 0 | `expectedControlRevision=36`; control file under `/run/user/1000/bubbles` |
| `repository-binding.sh preflight --request-class STRUCTURED --repository-root <repo-root>` | 0 | `PREFLIGHT_COMMITTED revision=37 repository=smackerel actionable=true` |
| `git show --stat 2f35e26f` | 0 | 12 files / 754 insertions — fix surface confirmed (4 non-artifact files) |
| `git show 2f35e26f -- internal/agent/tools/weather/tool.go \| grep -i percall` | 0 | **empty** — fix did not touch `PerCallTimeoutMs` (F-1 attribution) |
| `git log -S"PerCallTimeoutMs: 8000" -- …/weather/tool.go` | 0 | `2084cbf5` — the commit that raised the budget |
| `git merge-base --is-ancestor 2084cbf5 2f35e26f` | 1 | not an ancestor — `2084cbf5` is outside this fix's history |
| `grep -nE "go func\|go [a-zA-Z_]+\(\|time.After\|time.NewTicker\|time.NewTimer" internal/assistant/facade.go` | 1 | **zero matches** — no goroutine/timer anywhere in the facade |
| `grep -n "sync.\|mu.\|atomic." internal/assistant/httpadapter/late_binding.go` | 0 | `atomic.Pointer[HTTPAdapter]`, `Store` at `:42` — the publication barrier |
| `grep -n "sync.\|func (c *Cache)" internal/agent/tools/weather/cache.go` | 0 | `mu sync.Mutex` held in `Get`/`Put`/`Len` |
| `grep -rn "WithWeatherLookup" --include='*_test.go' .` | 0 | single hit in the unit test; **no** wiring-level guard (F-2) |
| `grep -rn "Err… ErrorCause = " internal/assistant/contracts/*.go` | 0 | 8-member closed vocabulary; no config-missing token (cause conflation is forced) |

**Reused, not re-run** (cited per the phase brief, measured earlier this
packet): `test unit --go` exit 0; `test integration` exit 0; `artifact-lint`
exit 0; mutation M1 killed by all 3 fast-path tests with a byte-identical
restore.

### Not run — stated, not implied

- **No `-race` execution.** The repo CLI exposes no `--race` selector, and
  terminal discipline forbids an ad-hoc `go test -race` around it. The §2
  concurrency verdict is STATIC, derived from the atomic publication barrier
  and the write-once seam, and is labelled as such wherever it appears.
- **No load or soak run.** The resource bounds in §3 are read from the
  constructed `http.Client`, the two call sites and the cache constructor; they
  are not measured under sustained concurrent `/weather` traffic.
- **No re-run of the Go or integration suites.** This phase changed zero
  production and zero test files, so re-running them would re-measure an
  unchanged tree. The prior exit-0 results are cited, not re-claimed as fresh.
- **No live provider turn.** Whether upstream currently exceeds the 2 s
  per-request bound (the F-1 concern) was not measured against the real
  endpoint; F-1 is argued from the comments' own stated measurements, not from
  a new one.
- **`docker-safe-prune.sh --apply` deliberately not run** — the reclaimable
  cache is shared with sibling repositories.

### Host preflight disclosure

No command in this phase required the disk-preflight guard: the phase ran only
`git`, `grep` and file reads, and built no image. `SMACKEREL_SKIP_HOST_PREFLIGHT=1`
was therefore **not** used this phase. (It was used in the preceding simplify
phase and is disclosed at `#simplify-verification`.) Recorded so its absence
here reads as a true negative rather than an omission.

### Verdict

```
🟢 STABLE

Domains audited: goroutine/lifecycle, concurrency, resource usage,
                 config/deployment reliability
Stability defects in the changed surface: 0
Follow-ups recorded (non-blocking): 2
  F-1 adjacent  — timeout-budget incoherence + stale comment, owner bubbles.implement,
                  attributable to 2084cbf5 (proved not an ancestor of 2f35e26f)
  F-2 durability— seam removal not test-protected, owner bubbles.test
Observations: 1 (fast path emits no span/counter; audited, so not unobservable)
Production code changed: 0 files
Fix cycle needed: NO

Concurrency verdict is STATIC (no race detector was run; the CLI exposes no
--race selector and terminal discipline was not breached to obtain one).
```

---

## Security phase (`bubbles.security`) {#security-phase}

Diagnostic phase. Zero production files, zero test files and zero foreign
artifacts were changed. Every claim below cites a file and line read in this
session; where a conclusion rests on reading code rather than executing a
proof-of-concept, the claim is tagged `interpreted` rather than `executed`.

### Threat surface actually introduced by this fix {#security-surface}

The fix (`2f35e26f`, 4 non-artifact files) adds exactly one new data path:
an explicit `/weather <location>` reaches an outbound third-party HTTP call
without the LLM loop, and the provider's reply reaches a user-visible chat
body. That is the whole new surface, and it is what was reviewed.

| Trust boundary | Direction | Carrier | Reviewed at |
|---|---|---|---|
| user → outbound HTTP | user-controlled `location` string | `handleWeatherShortcut` → `weather.LookupForecast` → `OpenMeteoProvider.geocode` | `facade.go:1387-1400`, `open_meteo.go:198-232` |
| provider → user | `forecast_line`, `provider_name` | `handleWeatherShortcut` unmarshal → `AssistantResponse.Body` / `.Sources` | `facade.go:1408-1444` |
| user → user (reflection) | `location` echoed in the failure body | `fmt.Sprintf("couldn't get the weather for %s …", location)` | `facade.go:1402-1407` |
| facade → audit / persistence | whole turn | `appendTurnAndPersist`, `writeAudit` | `facade.go:1006-1010` |

### 1. Input handling — CLEAN (no injection, no SSRF) {#security-input}

**Claim Source: interpreted** (static read of the request-construction code;
no crafted-input PoC was executed).

The user string is never concatenated into a URL, a path, or a header. Both
outbound requests build their query through `net/url`:

```
internal/agent/tools/weather/open_meteo.go:223   q := url.Values{}
internal/agent/tools/weather/open_meteo.go:224   q.Set("name", loc)
internal/agent/tools/weather/open_meteo.go:227   req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.geocodeURL+"?"+q.Encode(), nil)
internal/agent/tools/weather/open_meteo.go:290   q := url.Values{}
internal/agent/tools/weather/open_meteo.go:300   req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.forecastURL+"?"+q.Encode(), nil)
```

`q.Encode()` percent-encodes the value, so `&`, `?`, `#`, `/`, CR and LF in
the location cannot add or split a parameter. That closes both parameter
injection and CR/LF request-splitting — and `net/http` additionally rejects
control characters in a request line, so splitting has two independent
barriers, not one.

**SSRF is structurally impossible on this path.** The user contributes only
the *value* of the `name` query parameter. The scheme, host and path come from
`p.geocodeURL` / `p.forecastURL`, which are injected at construction from SST
(`assistant.skills.weather.{geocode_url,forecast_url}`) and are validated
fail-loud — `NewOpenMeteoProvider` **panics** on an empty endpoint
(`open_meteo.go:113-125`). There is no user-reachable seam that rewrites the
host. A user cannot steer the request anywhere.

The location is `strings.TrimSpace`-normalised twice (`facade.go:1388`,
`tool.go` `LookupForecast`) and an empty tail short-circuits to
`ErrSlotMissing` before any network call — so the empty case never reaches
the provider at all.

### 2. Output handling — CLEAN on the deployed configuration {#security-output}

**Claim Source: interpreted** (static read of the renderer; no injected
provider payload was replayed through a live transport).

Two provider-controlled strings reach user-visible output: `forecast_line`
(into `resp.Body`) and `provider_name` (into `resp.Sources[0].Title` /
`ExternalProviderRef.ProviderName`). One user-controlled string —
`location` — is reflected into the failure body.

The Telegram renderer escapes before it sets a parse mode. The escape is
applied on *every* branch that carries a body:

| Branch | Escape call | Line |
|---|---|---|
| sourced answer (the `/weather` success path — `SourceExternalProvider` is non-artifact) | `escapeForMode(okOut, mode)` | `render_outbound.go:166` |
| default body | `escapeForMode(body, mode)` | `render_outbound.go:198` |
| model footer | `escapeForMode(footer, mode)` | `render_outbound.go:243` |

`escapeForMode` → `escapeMarkdownV2` replaces the full closed Telegram set
`_*[]()~`>#+-=|{}.!` (`render_outbound.go:320-355`), and the deployed mode is
`markdown_mode: "MarkdownV2"` (`config/smackerel.yaml:1312`). So on the
configuration this packet ships, provider-controlled text cannot open a
Markdown entity, forge a link, or spoof the attribution footer.

The **reflected `location` never reaches the Telegram wire at all**: a
`StatusUnavailable` response routes to `renderError`, which emits the
single-line `"<skill>: <cause>"` form and drops `resp.Body` entirely
(`render_outbound.go:175-180`, `:300-312`). The reflection surface on this
transport is therefore empty. (That the facade's honest body is discarded by
the renderer is a rendering-fidelity nuance, not a security defect — the
rendered `provider_unavailable` cause is still honest, and the behaviour is
pre-existing renderer code untouched by this fix.)

Two adjacent gaps were found in the *shared* renderer layer. Neither is
caused by this fix and neither is reachable on the deployed configuration;
both are recorded as follow-ups in `#security-findings`, not as packet
findings.

### 3. Secret hygiene — CLEAN {#security-secrets}

**Claim Source: executed** (grep over the whole non-test weather package).

```
$ grep -rniE 'api[_-]?key|token|secret|password|credential|auth' \
    internal/agent/tools/weather/*.go | grep -v '_test.go'
(no output)
```

There is no credential in the package to leak. The provider is key-less by
design — the package header states it plainly: *"Geocodes the requested
location via the open-meteo geocoding endpoint (a single hit, no API key
required)"* (`open_meteo.go:9-11`). No `Authorization` header is set on
either request (`open_meteo.go:227-231`, `:300-304`).

The error path was checked specifically for the credential-in-URL risk named
in the brief. It is closed by construction: `handleWeatherShortcut`
**discards `err`** and substitutes a fixed sentence —

```go
out, err := f.weatherLookup(ctx, location)
if err != nil {
    return contracts.AssistantResponse{
        Status:     contracts.StatusUnavailable,
        ErrorCause: contracts.ErrProviderUnavailable,
        Body:       fmt.Sprintf("couldn't get the weather for %s right now — please try again.", location),
        EmittedAt:  emittedAt,
    }
}
```

`err` is bound but never rendered, logged, or attached to the response, so no
upstream error string — and therefore no URL, no query string, no header
echo — can reach the user, the audit record, or a log line. `writeAudit`
receives the constructed `resp`, not the error. The fast path emits no span
and no log statement of its own (confirmed in the stabilize phase, recorded
at `#stabilize-findings` as the observability observation), so there is no
third sink either.

Presence-only check of the environment during this phase: no weather-provider
credential variable is defined, because none exists in the contract.

### 4. Auth / authorization — CLEAN, nothing security-relevant is skipped {#security-authz}

**Claim Source: executed** (step-marker enumeration) **+ interpreted**
(reasoning about what each skipped step does).

The brief's core question is whether returning early at Step 3.9 skips a check.
Enumerating the step markers in `Handle` settles it by ordering:

```
$ grep -nE '^\s+// --- Step' internal/assistant/facade.go
620:  // --- Step 1: /reset short-circuit ---
652:  // --- Step 1.5: pending disambiguation resolver ---
678:  // --- Step 1.6: legacy-retirement dispatch ---
734:  // --- Step 2: shortcut detection (text turns only) ---
755:  // --- Step 2.5: NL routing for /find + /rate ---
802:  // --- Step 3: reference resolution (text turns only) ---
823:  // --- Step 3.5: structured intent compilation ---
879:  // --- Step 3.55: clarification gate
926:  // --- Step 3.6: side-effect confirmation gate
980:  // --- Step 3.7: retrieval-strategy routing (additive) ---
993:  // --- Step 3.9: deterministic /weather shortcut fast-path ---
1013: // --- Step 4: build envelope + route ---
```

Every gate that could matter runs **before** 3.9, so the fast path cannot
bypass any of them:

- **Confirm gate.** `handlePendingConfirm` runs at `:634` and the side-effect
  confirmation gate at `:926/:936`. A confirm reply is consumed at Step 1 and
  never reaches 3.9; a side-effect intent needing confirmation returns its
  confirm card at 3.6. The gate is upstream, not skipped.
- **Disambiguation.** `resolvePendingDisambig` runs at `:674`, upstream.
- **Clarification.** Step 3.55 runs at `:879`, upstream.
- **Per-user scope.** The fast path still calls
  `f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)` and
  `f.writeAudit(ctx, msg, BandHigh, nil, nil, resp)` (`facade.go:1007-1008`),
  so the turn is persisted to the same per-user conversation and audited
  exactly as a routed turn is. Neither identity nor audit is skipped.
- **Rate limiting.** `grep -rniE 'ratelimit|rate_limit|limiter|quota|throttle'
  internal/assistant/*.go` (non-test) returned **no matches** — the facade has
  no rate limiter on *any* path. So there is no differential: the fast path
  skips nothing that the routed path enforces. (The absence of a facade-level
  limiter is repo-level and pre-existing; see follow-up SEC-3.)

What 3.9 genuinely skips is the LLM tool-call loop, the router/borderline
band computation and the capture-as-fallback gate. None is a security
control: they are *routing* machinery. The capture gate in particular was the
defect — masking an explicit weather command as "saved as an idea" — so
skipping it is the fix, and it is skipped only for `msg.Kind == KindText &&
shortcutScenarioID == "weather_query" && f.weatherLookup != nil`, an explicit
operator-typed command with a wired seam.

One authorization-shaped nuance was checked and cleared: the fast path does
**not** guard on `conv.PendingConfirm == nil` the way the compiled-weather
block at `:963` does. That is safe because a pending-confirm *reply* is
already consumed at `:634` before 3.9 is reached, and 3.9 fires only on a
literal `/weather` prefix, which is not a confirm reply. It does not execute
the pending action, so no confirmation is short-circuited.

### 5. Denial of service — BOUNDED, and not user-multipliable {#security-dos}

**Claim Source: interpreted** (static read of the retry loop; no load test
was run — resource bounds were measured by the stabilize phase, cited here,
not re-derived).

The brief asked specifically whether a location string can multiply round
trips. It cannot. The geocode retry list is capped at two entries by
construction, regardless of input:

```go
attempts := []string{loc}
if idx := strings.Index(loc, ","); idx > 0 {          // FIRST comma only
    head := strings.TrimSpace(loc[:idx])
    if head != "" && head != loc {
        attempts = append(attempts, head)             // at most ONE extra
    }
}
```
(`open_meteo.go:198-210`)

`strings.Index` returns the first comma only and exactly one extra attempt is
appended, so `"a,b,c,d,…"` with a thousand commas still yields **2** geocode
attempts. Worst case per turn is therefore **≤ 3 upstream requests**
(≤2 geocode + 1 forecast) — an honest refinement of the brief's "two
sequential round trips", and still a constant that does not grow with input
length. Each is bounded by the provider `http.Client` timeout and the shared
request `ctx`; the 128-entry TTL cache was verified by stabilize.

No unbounded allocation was found on the path: the response is decoded into
fixed structs (`forecastResp`, `geocodeResp`) with typed fields, the daily
slice is pre-sized from `len(f.Daily.Time)`, and the rendered body is
budget-truncated by the transport (`joinAndBudget` / `budgetTruncate`).

### Mechanical floor — G034 `security-gate.sh` {#security-gate}

Run against the whole repository, as the gate requires. **Real exit code: 1.**

```
$ bash .github/bubbles/scripts/security-gate.sh --repo-root <repo-root>
FINDING: inline-credentials: ./scripts/commands/config.sh:866
FINDING: inline-credentials: ./scripts/commands/config.sh:1070
FINDING: inline-credentials: ./scripts/commands/config.sh:1236
FINDING: inline-credentials: ./scripts/commands/config.sh:1542
FINDING: inline-credentials: ./scripts/commands/config.sh:1841
FINDING: inline-credentials: ./scripts/commands/config.sh:1851
FINDING: inline-credentials: ./scripts/commands/config.sh:2239
FINDING: inline-credentials: ./scripts/commands/config.sh:2240
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:56
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:111
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:118
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:122
[security-gate] FAIL — G034 findings: 12
SECURITY_GATE_EXIT=1
```

**Attribution — repo level, not this packet.** All 12 findings live in two
files, `scripts/commands/config.sh` and
`scripts/commands/config_secret_rejection_test.sh`. This packet's fix commit
touched neither: its non-artifact surface is
`cmd/core/wiring_assistant_facade.go`,
`internal/agent/tools/weather/tool.go`, `internal/assistant/facade.go` and
`internal/assistant/facade_weather_shortcut_test.go`. The finding set is
therefore pre-existing and independent of BUG-061-007.

It is reported, not suppressed, and no exemption was added. The exit code is
recorded as **1** rather than being re-run with a narrowed scope to
manufacture a 0 — a scoped re-run would have hidden a real repo-level signal.

Two honest observations about *what* the 12 matched, offered as context and
**not** as a dismissal (`Claim Source: interpreted` — read from the gate's own
output lines, not separately verified as safe):

- Eight matches in `config.sh` are the generator's `__SECRET_PLACEHOLDER__*__`
  sentinel tokens — the substitution mechanism itself, matched because the
  pattern sees `NAME="…"`.
- The four `config_secret_rejection_test.sh` matches are inside the test that
  *enforces* secret rejection: two are `grep -qE` patterns, one is a failure
  message, one is a pass message.
- One `config.sh` match (`:2240`) is a self-described webhook test fixture
  rather than a live credential. Its value is deliberately not reproduced in
  this artifact.

Whether the gate should exempt placeholder sentinels and its own rejection
test is a repo-level decision with a repo-level owner — recorded as SEC-4.
This phase does not make that call and did not touch the gate.

### Findings {#security-findings}

**Zero security defects attributable to this packet.** All four follow-ups
below are adjacent — pre-existing code or repo-level policy that this fix
neither introduced nor worsened. Per the phase brief they are recorded with
owners rather than used to mark the packet insecure.

| ID | Severity | OWASP | What | Why NOT a packet finding | Owner |
|---|---|---|---|---|---|
| SEC-1 | Medium (latent) | A03 | `escapeForMode` returns the string **unescaped** when `mode == HTML`, yet `renderOutbound` then sets `ParseMode = ModeHTML` (`render_outbound.go:320-325`, `:73-75`). In HTML mode no escaping is applied to any body. | Pre-existing shared-renderer behaviour affecting *every* response body, not the weather path. Unreachable on the deployed configuration: `config/smackerel.yaml:1312` pins `markdown_mode: "MarkdownV2"`, whose escaper is complete. | `bubbles.implement` (transport renderer) |
| SEC-2 | Low (latent) | A03 | The WhatsApp renderer consumes `resp.Body` raw at four sites (`internal/whatsapp/assistant_adapter/render.go:212,225,282,344`) with no escape helper and no parse-mode selector found in that package. | Pre-existing renderer, untouched by this fix; WhatsApp applies only inherent lightweight formatting, so the impact is display spoofing, not script execution. Needs its own review rather than an inline patch here. | `bubbles.implement` (WhatsApp transport) |
| SEC-3 | Low | A04 | No rate limiter exists anywhere in `internal/assistant` (non-test grep: zero matches), so a user can drive repeated outbound provider requests. | Repo-level and pre-existing on **both** paths — the pre-fix LLM route called the same tool. The fix adds no new outbound capability and *reduces* per-turn cost by removing the LLM loop. Bounded at ≤3 upstream requests per turn with a TTL cache. | `bubbles.plan` (assistant throughput policy) |
| SEC-4 | Informational | — | `security-gate.sh` exits 1 on 12 repo-level `inline-credentials` matches in the SST generator and its own secret-rejection test. | Files never touched by this packet; the gate's pattern set vs. placeholder sentinels is a repo-level policy decision. | repo owner / `bubbles.plan` |

### Commands executed (this phase) {#security-commands}

| # | Command | Exit | Result |
|---|---|---|---|
| 1 | `repository-binding-host-context.sh --session-log <host-token> --workspace-root <repo-root>` | 0 | `expectedControlRevision=37`; session control file resolved (presence-only) |
| 2 | `repository-binding.sh preflight --request-class STRUCTURED --repository-root <repo-root>` | 0 | `PREFLIGHT_COMMITTED revision=38 repository=smackerel actionable=true` |
| 3 | `git show --stat 2f35e26f` | 0 | 12 files, 754 insertions — 4 non-artifact files confirmed as the review surface |
| 4 | `git show 2f35e26f -- <3 production files>` | 0 | Full diff read; `handleWeatherShortcut` + `LookupForecast` + `WithWeatherLookup` seam |
| 5 | `grep -rnE 'url\.(Values\|QueryEscape\|PathEscape\|Parse)\|http.NewRequest\|Sprintf\(.*http\|baseURL\|Endpoint' internal/agent/tools/weather/*.go` | 0 | 4 hits, all `url.Values` + `q.Encode()`; **zero** string-concatenated user input |
| 6 | `grep -rniE 'api[_-]?key\|token\|secret\|password\|credential\|auth' internal/agent/tools/weather/*.go` | 1 | **No output** — no credential exists in the provider package |
| 7 | `grep -rnE 'ParseMode\s*=\|ParseMode:' internal/telegram/ --include='*.go'` | 0 | 5 sites; MarkdownV2 / HTML / cleared |
| 8 | `grep -rnE 'func .*(Escape\|escape)' internal/telegram/ internal/agent/tools/weather/` | 0 | `escapeForMode:320`, `escapeMarkdownV2:353` — escaping exists and precedes the parse-mode set |
| 9 | `grep -rnE '"html"\|"markdownv2"\|markdown_mode' internal/ config/` | 0 | `config/smackerel.yaml:1312 markdown_mode: "MarkdownV2"` — deployed mode has a complete escaper |
| 10 | `grep -rln 'contracts.AssistantResponse' internal/ (transports only)` | 0 | Two transports consume the body: Telegram and WhatsApp (motivates SEC-2) |
| 11 | `grep -nE '^\s+// --- Step' internal/assistant/facade.go` | 0 | Step 3.9 sits **after** every confirm / disambig / clarification gate — the authz answer |
| 12 | `grep -nE 'PendingConfirm\|PendingDisambig' internal/assistant/facade.go` | 0 | Resolvers at `:634` / `:674`, both upstream of `:993` |
| 13 | `grep -rniE 'ratelimit\|rate_limit\|limiter\|quota\|throttle' internal/assistant/*.go` | — | **No matches** — no limiter on either path, so no differential bypass (motivates SEC-3) |
| 14 | `bash .github/bubbles/scripts/security-gate.sh --repo-root <repo-root>` | **1** | 12 repo-level `inline-credentials` findings, all in two untouched files — see `#security-gate` |

### Not run — stated, not implied {#security-not-run}

- **No crafted-input proof-of-concept.** No `/weather` turn with an injection
  payload was driven against a live provider or a live transport. The input
  and output verdicts rest on reading the escaping and request-construction
  code and are tagged `interpreted` accordingly.
- **No SAST tool beyond the framework gate.** `gosec`/`semgrep` are not in the
  repo CLI surface and terminal discipline forbids an ad-hoc invocation.
- **No dependency CVE scan this phase.** `cargo/npm/pip`-style audits do not
  apply to a Go module here, and the repo's supply-chain gate (Trivy, 0 CVEs
  after the grpc v1.82.1 bump) is already recorded at `#build-check`; it is
  cited, not re-claimed as fresh.
- **No re-run of `test unit --go`, `test integration`, `artifact-lint` or
  `pii-scan` for the code baseline.** This phase changed zero production and
  zero test files, so re-running them would re-measure an unchanged tree. The
  prior exit-0 results are cited per the phase brief. (`artifact-lint` and
  `pii-scan` **were** run after this phase's artifact writes — see
  `#security-verification`.)
- **No `--race` run.** Unchanged from the stabilize phase; the CLI exposes no
  `--race` selector. Any concurrency statement here remains **STATIC**.
- **No live provider turn**, so no observation of a real upstream error string
  — the secret-hygiene verdict rests on `err` being provably discarded at
  `facade.go:1402-1407`, which is stronger than a single observed error would be.

### Host preflight disclosure {#security-preflight}

`SMACKEREL_SKIP_HOST_PREFLIGHT=1` was **not** used this phase. No build,
image or stack command ran — only `git`, `grep`, file reads and the framework
gate — so `disk-preflight.sh` was never reached. It would still refuse on this
host (~34 GB free against a 40 GB threshold) if a build command were run; the
documented opt-out remains the only sanctioned response and no shared cache
was pruned. Recorded so the absence reads as a true negative.

### Verdict {#security-verdict}

```
⚠️ FINDINGS

Axes reviewed: input handling, output handling, secret hygiene,
               auth/authorization, denial of service
Security defects attributable to THIS packet: 0
  input handling  — CLEAN (url.Values escaping; SSRF structurally impossible)
  output handling — CLEAN on the deployed MarkdownV2 configuration
  secret hygiene  — CLEAN (key-less provider; upstream err provably discarded)
  auth / authz    — CLEAN (every gate is upstream of Step 3.9; nothing skipped)
  denial of service — BOUNDED (<=3 upstream requests/turn, not user-multipliable)

Adjacent follow-ups recorded (non-blocking, none caused by this fix): 4
  SEC-1 Medium/latent  — escapeForMode no-ops in HTML mode, owner bubbles.implement
  SEC-2 Low/latent     — WhatsApp renderer consumes body raw, owner bubbles.implement
  SEC-3 Low            — no facade rate limiter on EITHER path, owner bubbles.plan
  SEC-4 Informational  — G034 gate exit 1, 12 repo-level findings, repo owner

Mechanical floor: security-gate.sh EXIT 1 — 12 findings, ALL repo-level,
ALL in scripts/commands/{config.sh,config_secret_rejection_test.sh}, files
this packet never touched. Reported, not suppressed; no exemption added.

Production code changed: 0 files
Fix cycle needed: NO

The verdict is FINDINGS rather than SECURE solely because the mandatory
mechanical floor exits non-zero at repo level. The packet's own changed
surface is clean on all five axes.
```

