# Design: BUG-031-008 CI integration job stabilization

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [state.json](state.json)

## Overview

Four independent root causes keep the `integration` job red. Each is fixed in its
own scope; the change manifest is deliberately minimal and spec-083-free.

## Capability Posture

### Single-Implementation Justification

Each of the five clusters is delivered by exactly one mechanism against an
existing surface; there is no foundation-plus-concrete-implementations split and
no second provider/adapter/variant to select between, so a Capability Foundation
/ Concrete Implementations / Variation Axes decomposition is not applicable:

| Cluster | Single implementation | No variant because |
|---|---|---|
| C1 | docker-absence honest-skip guard in `cli_auth_passthrough_test.go` | the wrapper + in-container CLI are already correct on docker hosts |
| C2 | in-network `CORE_EXTERNAL_URL` override in the `smackerel.sh` runner | it mirrors the single existing `ML_SIDECAR_URL` override |
| C3a | corrected assertion in `microtools_registry_canary_test.go` | the registry itself is unchanged |
| C3b | one `allowed_tools` entry in `weather-query-v1.yaml` | additive restore of one already-registered tool |
| C4 | fallback-geocoder honest-skip guard in `microtools_location_test.go` | real-provider coverage is owned by spec 076 |

The "provider" / "connector" / "adapter" words elsewhere in this design refer to
the pre-existing open-meteo geocoder provider and the existing connector
passthrough arm; this packet neither introduces nor multiplies any such
capability.

## Reproduced Root Causes (AUTHORITATIVE — supersedes the initial hypotheses in "Root Cause Analysis" below)

The session reproduction (`./smackerel.sh test integration` at HEAD `28851e7a`, full
transcript in `report.md`) **overturned two of the initial hypotheses** and surfaced a
fifth, separate failing cluster. The reproduce-first gate is exactly why these are
corrected here. The fixes implemented are driven by the reproduced reality below, NOT the
initial `## Root Cause Analysis` prose (retained only as the pre-reproduction record).

| Cluster | Initial hypothesis | REPRODUCED root cause | Fix (implemented) |
|---|---|---|---|
| **C1** (`TestCLIAuthPassthrough_*`) | wrapper missing `-T` masks exit code | `docker is required`, exit 1 — the **containerized go-integration runner has no docker**, so the `auth)` arm's `require_docker` aborts before any compose exec; the wrapper + in-container CLI are correct | **Scope 1 (test):** skip `cli_auth_passthrough_test.go` when `docker` is not on PATH (honest env-gated skip, matching the repo pattern). Host-with-docker still runs it fully. |
| **C2** (`TestAssistantTransportHint_*`) | CI cold-start / ML readiness timeout | `core not healthy after 30s at http://127.0.0.1:<port>` — core **is** healthy; `CORE_EXTERNAL_URL` is the host mapping (`127.0.0.1:PORT`), **unreachable from inside the compose-network runner**. Reproduces LOCALLY, not CI-only | **Scope 2 (smackerel.sh):** runner now overrides `CORE_EXTERNAL_URL=http://smackerel-core:${core_container_port}` (in-network), mirroring the existing `ML_SIDECAR_URL=http://smackerel-ml:8081`. No timeout bump; no unhealthy core masked. |
| **C3a** (`TestMicroToolRegistryCanary`) | all four micro-tools now register | only `location_normalize` + `entity_resolve` register at import via `init()`; `unit_convert` + `calculator` register lazily on `Set*Services` (no `init()`) | **Scope 3 (test):** assert loc+entity ARE registered at import AND unit/calc are NOT (adversarial inverse) — matches shipped reality + spec-076 Scope-3 canary. |
| **C3b** (`TestWeatherPromptUses...`) | `location_normalize` dropped from weather `allowed_tools` | CONFIRMED — only the `allowed_tools_lists_location_normalize` subtest fails; shrink + no-dictionary subtests already PASS | **Scope 4 (config):** add `location_normalize` (`side_effect_class: external`) to `weather-query-v1.yaml` `allowed_tools`. |
| **C4** (`TestLocationNormalizeIntegration_*`) | maybe same as C3b, or a separate geocoder issue | SEPARATE — the test-stack geocode **stub returns "Reykjavík" for all inputs** (F-065-LOCATION-STUB); this is an orphaned spec-065 SCOPE-2 test (superseded → spec-076, which is done and covers location_normalize via `TestMicroToolOverlays_FullMatrix`) that requires a REAL open-meteo geocoder the stub cannot provide | **Scope 5 (test):** skip `TestLocationNormalizeIntegration` when the wired geocoder is the canned stub (probe returns Reykjavík); real-provider coverage is owned by spec-076. |

The four `## Root Cause Analysis` subsections below are the pre-reproduction hypotheses,
kept for the audit trail. Where they disagree with the table above, **the table wins**.

## Root Cause Analysis

### C1 — CLI auth/connector passthrough loses exit code in non-interactive contexts

`smackerel.sh` `auth)` (line ~693) and `connector)` (line ~709) arms invoke:

```bash
smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel-core auth "$@"
```

`smackerel_compose` (`scripts/lib/runtime.sh:92-97`) redirects stdin from `/dev/null`
for `exec` but does **not** pass `-T`:

```bash
if [[ "${1:-}" == "exec" ]]; then
  "${args[@]}" "$@" </dev/null
else
  "${args[@]}" "$@"
fi
```

Without `-T`, `docker compose exec` requests a pseudo-TTY. Under a non-interactive
caller — `cli_auth_passthrough_test.go` runs the wrapper via Go `exec.CommandContext`
+ `CombinedOutput()`, and the CI runner has no TTY — compose aborts with
`the input device is not a TTY` and exits **1**, *before* the in-container
`smackerel-core auth` binary runs. That binary (`cmd/core/cmd_auth.go::runAuthCommand`)
is correct: it returns **2** with `usage: smackerel auth ...` (no args) or
`smackerel auth: unknown subcommand %q ...` (bad subcommand). The test therefore sees
exit 1 + TTY error instead of exit 2 + banner, and fails both subtests.

This is a **pre-existing latent defect**: spec-060 Scope-3 added both the wrapper arm
and the test, but the wrapper never carried `-T`. It was harmless until the integration
job began running the test on `main`. (spec-021 only changed the `deploy)` arm's
double-shift — unrelated, as the operator noted.)

**Fix (Scope 1):** add `-T` to both the `auth)` and `connector)` exec invocations:

```bash
smackerel_compose "$TARGET_ENV" exec -T smackerel-core smackerel-core auth "$@"
```

`-T` disables pseudo-TTY allocation, so the in-container exit code (2) and the stderr
banners propagate verbatim through `docker compose exec` → the bash wrapper → the Go
test. The `</dev/null` redirect in `smackerel_compose` already neutralizes stdin, so
`-T` is purely additive and changes nothing for interactive operator use (an operator
running `./smackerel.sh auth enroll ...` from a terminal still gets correct behavior;
`-T` only suppresses TTY allocation, which the auth/connector CLIs do not need — they
read args + env, not interactive stdin).

### C2 — assistant transport-hint tests exhaust the 30s live-stack health wait

`TestAssistantTransportHint_*` call `waitLiveStackHealthy(t, stack, 30*time.Second)`
which polls `GET {CORE_EXTERNAL_URL}/api/health` every 2s for a 200. Both tests fail
at ~30.02s, i.e. `/api/health` never returned 200 within the in-test 30s budget. The
CI log showed "ML sidecar readiness timeout" + a model-envelope validation line.

The determination that gates the fix is **local-vs-CI**:

- **If the cluster PASSES locally** (this workstation runs a large GPU-accelerated model,
  warm) **but only fails in CI** (CI uses a small CPU model — `gemma3:4b` — with a
  ~4 GB Ollama + ~3 GB model cold pull), the root cause is **CI cold-start timing**:
  core's readiness (which depends on the ML sidecar) is not achieved within the in-test
  30s window even though the `Bring up test stack` step nominally completed. The fix is a
  **CI-stack readiness adjustment** (e.g. the in-test health budget is too tight for the
  CPU cold-start path, or the bring-up health gate must fully include core/ML readiness
  before tests start). This is NOT a blind timeout bump over an unhealthy core — it is
  matching the readiness budget to the proven cold-start cost.
- **If the cluster FAILS locally too**, core has a genuine readiness defect (e.g. a
  model-envelope misconfig — `LLM_MODEL` requiring more memory than `OLLAMA_MEMORY_LIMIT`)
  and the fix is to repair the stack/config so core actually becomes healthy.

The exact branch is determined by the session reproduction (recorded in `report.md`).
Per the operator constraint, C2 is the cluster most likely to require a real CI-resource
decision; if it cannot be honestly closed this session it is handed back with exact repro
and next owner rather than masked.

### C3a — micro-tools registry canary froze a stale spec-065 SCOPE-1 assumption

`microtools_registry_canary_test.go` subtest `microtools_foundation_did_not_register_any_tool`
asserts `location_normalize`/`unit_convert`/`calculator`/`entity_resolve` are **NOT**
registered, encoding the spec-065 SCOPE-1 "envelope-only foundation; concrete tools land
in SCOPE-2..4" assumption. Reality moved past it: each micro-tool self-registers at import
via `init(){ …RegisterOnce.Do(register…) }` → `agent.RegisterTool`:

- `internal/agent/tools/microtools/location_normalize.go:164` `init()` → `:167` `RegisterTool`
- `internal/agent/tools/microtools/unit_convert.go:55` `Do(registerUnitConvert)` → `:206` `RegisterTool`
- `internal/agent/tools/microtools/calculator.go:58` `Do(registerCalculator)` → `:112` `RegisterTool`
- `internal/agent/tools/microtools/entity_resolve.go:113` `Do(registerEntityResolve)` → `:186` `RegisterTool`

spec-065 SCOPE-2..4 were **Superseded → rescoped to spec-076** (see
`specs/065-generic-micro-tools/scopes.md` "Superseded Scopes"). spec-076 (status `done`)
Scope-3 ships its own `tool_registry_canary` (TP-076-03-06) that explicitly asserts
`agent.Has` for all four micro-tool names. So the authoritative owner already treats the
four tools as registered; the spec-065-era canary's "must NOT register" assertion is stale.

**Fix (Scope 3) — test fix (operator-classified):** replace the
`microtools_foundation_did_not_register_any_tool` subtest's "must NOT be registered"
assertion with a "MUST be registered" assertion (rename to reflect shipped reality),
keeping the still-valid `weather_lookup_still_registered`, `weather_lookup_schemas_still_compile`,
and `registry_still_lists_all_tools` subtests. Justified against spec-065 SCOPE-2..4
(superseded → spec-076) and spec-076 Scope-3's own canary.

### C3b — location_normalize dropped from the weather scenario allowed_tools

`config/prompt_contracts/weather-query-v1.yaml` `allowed_tools` lists only `weather_lookup`
(line ~39); `location_normalize` is absent. `microtools_prompt_contract_test.go` subtest
`allowed_tools_lists_location_normalize` (spec-065 SCOPE-4 contract) requires
`location_normalize` present.

git history of the file:
- `1f74d5c0` (spec-065 SCOPE-2/4 evidence) — **added** `location_normalize` + shrank the prompt.
- `4a883984` "spec 061: weather scenario calls weather_lookup directly (no location_normalize step)" — **removed** `location_normalize` from the weather scenario.
- `028845ab` "spec 061: shorter weather prompt" — most recent; prompt is the shrunk form.

`location_normalize` is a real, production-wired tool
(`cmd/core/wiring_assistant_skills.go:295` `SetLocationServices`) registered with
`SideEffectClass: external`. Per operator directive, this cluster is a **config
regression**: the spec-065-SCOPE-4-derived contract test encodes the desired state
(the weather scenario advertises `location_normalize`), and the fix is to restore the
config to match the test — not to mutate the test.

**Fix (Scope 4) — config fix:** add `location_normalize` (`side_effect_class: external`,
matching its registration) to `weather-query-v1.yaml` `allowed_tools`, alongside
`weather_lookup`. The `system_prompt` is already the shrunk form (no inline
state/nickname dictionary), so the `system_prompt_block_shrunk_by_at_least_40_percent`
and `prompt_no_longer_carries_inline_location_dictionary` subtests already pass — only
the allowlist assertion fails today. Restoring to `allowed_tools` re-advertises the tool
WITHOUT reverting spec-061's `direct_output_from_tool: weather_lookup` pass-through
optimization or re-bloating the prompt; the model is still instructed to call
`weather_lookup` and the executor still short-circuits on its result. The existing
`allowed_tools_lists_location_normalize` subtest is itself the adversarial regression
guard — it fails if `location_normalize` is ever dropped from the allowlist again.

**Transparency note:** spec-061 (commit `4a883984`) deliberately removed the
`location_normalize` *step* from the weather flow because `weather_lookup` geocodes
internally. Restoring `location_normalize` to `allowed_tools` only re-advertises tool
availability to the scenario; it does not re-introduce a mandatory normalization step and
does not change the shrunk prompt or the direct-output optimization. Scope 4 verifies no
broader weather/assistant integration regression results.

### TestLocationNormalizeIntegration (separate, defensive re-check)

`tests/integration/assistant/microtools_location_test.go::TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations`
wires its OWN open-meteo provider via `wireLiveOpenMeteoLocationProvider` and calls the
registered handler directly through `agent.ByName` — it is **independent** of the
weather-query-v1.yaml `allowed_tools` (C3b). It skips honestly when
`ASSISTANT_SKILLS_WEATHER_GEOCODE_URL` is unset. Scope 4 re-runs it to confirm it is not
a separate live open-meteo/geocoder regression; the reproduction result is recorded in
`report.md`.

## Change Manifest (allowed file families)

- `smackerel.sh` (C1: `auth)` + `connector)` exec arms only — add `-T`)
- `tests/integration/assistant/microtools_registry_canary_test.go` (C3a: stale subtest)
- `config/prompt_contracts/weather-query-v1.yaml` (C3b: `allowed_tools`)
- C2: scope-dependent — either `tests/integration/api/assistant_transport_hint_test.go`
  (in-test readiness budget) and/or `.github/workflows/ci.yml` (CI bring-up readiness)
  per the reproduction determination; recorded in Scope 2.
- This bug packet under `specs/031-live-stack-testing/bugs/BUG-031-008-integration-job-stabilization/`

## Excluded file families (HARD)

- spec-083: `specs/083-card-rewards-companion/**`, `internal/cardrewards/**`,
  `ml/app/card_categories.py`, `ml/app/main.py`, `ml/tests/test_card_categories.py`,
  `tests/integration/cardrewards_extract_test.go`,
  `internal/deploy/docs_connector_count_contract_test.go`, `docs/Development.md`,
  `docs/smackerel.md`.
- `.github/bubbles/**`, `.github/agents/**`, `.github/instructions/bubbles-*` (framework).
- Any other spec packet.

## Shared Infrastructure Impact Sweep

- **C1** touches a shared operator surface (`smackerel.sh` + `scripts/lib/runtime.sh`
  usage). `-T` is additive and value-safe; it does not change interactive operator
  behavior for `auth`/`connector` (which read args/env, not interactive stdin). The
  `connector)` arm gets the identical fix so the two passthrough arms stay consistent and
  a future connector passthrough test does not re-hit the same TTY trap.
- **C3a/C3b** touch the micro-tool registry canary + a scenario prompt-contract. The
  registry itself is unchanged (C3a is a test-expectation correction); C3b is additive to
  one scenario's `allowed_tools`. Scope 4 verifies the broader weather/assistant
  integration tests remain green so the additive allow-list entry does not perturb live
  tool-selection behavior.
- **C2** may touch CI bring-up readiness — the most sensitive surface; its fix is
  reproduction-gated and, if it requires a real CI-resource decision, is handed back
  rather than guessed.
