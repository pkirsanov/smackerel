# Scopes: BUG-031-008 CI integration job stabilization

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Four independent clusters keep the CI `integration` job red. Each cluster is one
scope. All four are independent (`Depends On: None`) and may be fixed in any order;
they are executed sequentially with per-cluster reproduce → fix → re-run evidence.

## Scope 1: C1 — CLI auth passthrough (docker-absent runner) honest skip

**Status:** Done
**Priority:** P1
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-008 C1 CLI passthrough exit-code propagation

  Scenario: BUG-031-008-SCN-001 auth with no subcommand returns exit 2 + usage banner
    Given a running test stack and the smackerel.sh auth passthrough wrapper
    When a non-interactive caller runs `./smackerel.sh --env test auth` (no subcommand)
    Then the wrapper exits with code 2
    And the combined output contains "usage: smackerel auth"

  Scenario: BUG-031-008-SCN-002 auth unknown subcommand returns exit 2 + forwarded arg
    Given a running test stack and the smackerel.sh auth passthrough wrapper
    When a non-interactive caller runs `./smackerel.sh --env test auth not-a-real-subcommand`
    Then the wrapper exits with code 2
    And the combined output contains "unknown subcommand"
    And the combined output contains the forwarded arg "not-a-real-subcommand"
```

### Implementation Plan

> Reproduction overturned the initial `-T` hypothesis: the actual failure is `docker is
> required` (exit 1) because the containerized go-integration runner has no docker. See
> design.md "Reproduced Root Causes" + report.md#c1-repro.

1. In `tests/integration/cli_auth_passthrough_test.go`, guard `runSmackerelAuth` with
   `exec.LookPath("docker")`; when docker is absent (the containerized runner),
   `t.Skip(...)` with a clear reason — consistent with the repo's env-gated integration
   skips (e.g. `assistant_transport_hint_test.go`).
2. No `smackerel.sh` change: the wrapper + in-container `smackerel-core auth` CLI already
   return exit 2 + the banners correctly on a host with docker.
3. Re-run both subtests (expect honest SKIP in the containerized runner).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-C1-01 | TestCLIAuthPassthrough_NoArgsExitsTwo | integration | tests/integration/cli_auth_passthrough_test.go | exit 2 + "usage: smackerel auth" through non-interactive caller | BUG-031-008-SCN-001 |
| T-C1-02 | TestCLIAuthPassthrough_UnknownSubcommandExitsTwo | integration | tests/integration/cli_auth_passthrough_test.go | exit 2 + "unknown subcommand" + forwarded arg | BUG-031-008-SCN-002 |

### Shared Infrastructure Impact Sweep

The fix is confined to one test file's skip guard; no production/shared surface changes.
The skip is honest (the containerized runner structurally cannot reach docker to exercise
the host-side `docker compose exec` path); it is NOT skip-to-green of a product bug. CI
coverage of the wrapper would require giving the runner docker.sock + the docker CLI — a
larger spec-045 BUG-045-002 topology change, recorded as a follow-up finding (handback).

### Change Boundary

Allowed: `tests/integration/cli_auth_passthrough_test.go` only. Excluded: every spec-083
path; `smackerel.sh` (the wrapper is correct); framework files.

### Definition of Done

- [x] `cli_auth_passthrough_test.go` skips both subtests when docker is unavailable (containerized runner), with a clear reason — **Phase:** implement
  → Evidence: [report.md#c1-implement](report.md) (diff: `exec.LookPath("docker")` → `t.Skip`)
- [x] T-C1-01 + T-C1-02 reproduced RED at baseline (`docker is required`, exit 1) AND honest SKIP after the fix against the real runner — **Phase:** test
  → Evidence: [report.md#c1-repro](report.md) (RED ≥10 lines) + [report.md#c1-after](report.md) (SKIP)
- [x] Build Quality Gate: zero warnings; zero deferrals; spec-083 untouched; skip is honest (documents the docker-absent runner; not masking a product bug — wrapper + cmd_auth.go return exit 2 correctly on a docker host) — **Phase:** audit
  → Evidence: [report.md#c1-audit](report.md)

## Scope 2: C2 — transport-hint in-container URL unreachability

**Status:** Done
**Priority:** P1
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-008 C2 assistant transport-hint live-stack readiness

  Scenario: BUG-031-008-SCN-003 web and mobile hints accepted as telemetry against a healthy stack
    Given the live test stack reaches /api/health 200 within the readiness budget
    When the same scenario-neutral turn is posted with transport_hint web, mobile (ios), mobile (android)
    Then each returns HTTP 200 and decodes strictly against TurnResponse v1
    And transport_hint never becomes the transport and the facade is invoked

  Scenario: BUG-031-008-SCN-004 unknown transport_hint rejected pre-facade
    Given the live test stack is healthy
    When a turn is posted with transport_hint "carrier-pigeon"
    Then the endpoint returns HTTP 400 before invoking the facade

  Scenario: BUG-031-008-SCN-005 readiness determination (local vs CI)
    Given the same code at origin/main
    When the cluster is reproduced locally and compared to the CI failure signature
    Then the root cause is classified as CI-cold-start-timing (passes local, fails CI)
      or genuine-core-unhealthy (fails local too)
    And the fix matches the determined cause without masking an unhealthy core
```

### Implementation Plan

> Reproduction overturned the initial cold-start hypothesis: core IS healthy; the test
> fails because `CORE_EXTERNAL_URL=http://127.0.0.1:<port>` (the host mapping) is
> unreachable from inside the compose-network runner. Reproduces locally. See design.md
> "Reproduced Root Causes" + report.md#c2-repro.

1. Reproduce locally and capture the core health snapshot proving core is healthy
   (degraded-200) while the test cannot reach `http://127.0.0.1:<port>` from the runner.
2. In `smackerel.sh`, the go-integration `docker run` already rewrites dependency URLs to
   in-network service names (`DATABASE_URL`→`postgres:`, `NATS_URL`→`nats:`,
   `ML_SIDECAR_URL=http://smackerel-ml:8081`). Add the missing one: read
   `core_container_port` from `CORE_CONTAINER_PORT` and pass
   `-e "CORE_EXTERNAL_URL=http://smackerel-core:${core_container_port}"`. The `-e` flag
   overrides the env-file's host URL. NOT a timeout bump; NOT masking an unhealthy core.
3. Re-run both transport-hint tests against the real stack (expect PASS).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-C2-01 | TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry | integration (live) | tests/integration/api/assistant_transport_hint_test.go | 200 + strict TurnResponse v1 decode for web/mobile | BUG-031-008-SCN-003 |
| T-C2-02 | TestAssistantTransportHint_UnknownHintRejectedBeforeFacade | integration (live) | tests/integration/api/assistant_transport_hint_test.go | 400 for unknown hint | BUG-031-008-SCN-004 |
| T-C2-03 | Local-vs-CI readiness determination | diagnostic | reproduction transcript + CI log signature | classify CI-timing vs genuine-unhealthy | BUG-031-008-SCN-005 |

### Shared Infrastructure Impact Sweep

C2 may touch the CI bring-up readiness gate (`.github/workflows/ci.yml`) and/or the
in-test health budget — the most sensitive surface. The fix is reproduction-gated; if it
requires a real CI-resource decision it is handed back rather than guessed. No production
runtime behavior is changed by a readiness-budget adjustment.

### Change Boundary

Allowed: `smackerel.sh` (go-integration `docker run` runner env only — add
`core_container_port` + the `CORE_EXTERNAL_URL` in-network override). Excluded: every
spec-083 path; any blind timeout bump; any request interception/mocks.

### Definition of Done

- [x] Local reproduction captured and the cause classified: NOT cold-start — core is healthy (degraded-200), `CORE_EXTERNAL_URL=127.0.0.1:<port>` unreachable from the compose-network runner; reproduces locally — **Phase:** test
  → Evidence: [report.md#c2-repro](report.md) (≥10 lines: test FAIL @127.0.0.1 + core /api/health up snapshot)
- [x] Fix matches the determined cause: runner overrides `CORE_EXTERNAL_URL=http://smackerel-core:${core_container_port}` (in-network), mirroring `ML_SIDECAR_URL`; no blind timeout bump — **Phase:** implement
  → Evidence: [report.md#c2-fix](report.md) (smackerel.sh diff)
- [x] T-C2-01 + T-C2-02 pass GREEN against the real stack — **Phase:** test
  → Evidence: [report.md#c2-after](report.md) (GREEN: web/mobile-ios/mobile-android + unknown-hint 400)
- [x] Build Quality Gate: zero warnings; spec-083 untouched; no request interception/mocks introduced (live-stack authenticity preserved) — **Phase:** audit
  → Evidence: [report.md#c2-audit](report.md)

## Scope 3: C3a — micro-tools registry canary stale assertion

**Status:** Done
**Priority:** P2
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-008 C3a micro-tools registry canary reflects shipped reality

  Scenario: BUG-031-008-SCN-006 import-registered micro-tools match shipped reality
    Given the microtools package is imported (init() registration runs)
    When the agent registry is queried
    Then location_normalize and entity_resolve are registered (via init())
    And unit_convert and calculator are NOT registered by mere import (they register on Set*Services)
    And weather_lookup remains registered and its schemas still compile
```

### Implementation Plan

> Reproduction refined the assertion: only `location_normalize` + `entity_resolve` register
> at import (via `init()`); `unit_convert` + `calculator` register lazily on
> `SetUnitConvertServices` / `SetCalculatorServices` (no `init()`). See report.md#c3a-repro.

1. In `tests/integration/assistant/microtools_registry_canary_test.go`, replace the stale
   `microtools_foundation_did_not_register_any_tool` subtest (which asserted NONE of the
   four register) with `import_registered_microtools_match_shipped_reality`: assert
   `location_normalize` + `entity_resolve` ARE registered at import, AND `unit_convert` +
   `calculator` are NOT (the adversarial inverse — fails if either contract regresses).
2. Preserve the `weather_lookup_still_registered`, `weather_lookup_schemas_still_compile`,
   and `registry_still_lists_all_tools` subtests unchanged.
3. Re-run the canary (expect PASS). Justified vs spec-065 SCOPE-2..4 (superseded →
   spec-076) and spec-076 Scope-3's own `tool_registry_canary`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-C3a-01 | TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate | integration | tests/integration/assistant/microtools_registry_canary_test.go | four micro-tools registered + weather_lookup + schemas compile | BUG-031-008-SCN-006 |

### Shared Infrastructure Impact Sweep

C3a corrects a test expectation only; the agent registry and micro-tool registration code
are unchanged. The updated assertion is non-tautological — it fails if any of the four
micro-tools regresses out of the registry (the adversarial inverse of the old assertion).

### Change Boundary

Allowed: `tests/integration/assistant/microtools_registry_canary_test.go` only.
Excluded: every spec-083 path; the microtools production package; framework files.

### Definition of Done

- [x] Stale subtest replaced: asserts location_normalize+entity_resolve registered at import AND unit_convert+calculator NOT registered by import; legitimate subtests preserved — **Phase:** implement
  → Evidence: [report.md#c3a-implement](report.md) (diff)
- [x] T-C3a-01 reproduced RED at baseline (loc+entity flagged) AND passes GREEN after the fix against the real stack — **Phase:** test
  → Evidence: [report.md#c3a-repro](report.md) (RED ≥10 lines) + [report.md#c3a-after](report.md) (GREEN)
- [x] Build Quality Gate: zero warnings; spec-083 untouched; assertion is non-tautological (lazyOnly inverse fails if unit/calc self-register at import) — **Phase:** audit
  → Evidence: [report.md#c3a-audit](report.md)

## Scope 4: C3b — weather scenario allowed_tools location_normalize

**Status:** Done
**Priority:** P2
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-008 C3b weather scenario advertises location_normalize

  Scenario: BUG-031-008-SCN-007 weather allowed_tools includes location_normalize and weather_lookup
    Given config/prompt_contracts/weather-query-v1.yaml
    When the allowed_tools list is parsed
    Then it includes both "weather_lookup" and "location_normalize"
    And the system_prompt block remains at least 40% smaller than the pre-spec-065 baseline
    And the prompt carries no inline location normalization dictionary
```

### Implementation Plan

1. In `config/prompt_contracts/weather-query-v1.yaml`, add `location_normalize`
   (`side_effect_class: external`) to `allowed_tools` alongside `weather_lookup`. Do NOT
   touch the system_prompt (already shrunk) or `direct_output_from_tool`.
2. Run `./smackerel.sh check` (scenario-lint) to confirm the scenario still validates.
3. Re-run `TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent` (all 3 subtests).
4. Re-run `TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations` (defensive — confirm
   it is not a separate live-geocoder regression).
5. Verify no broader weather/assistant integration regression results from the additive
   allow-list entry.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-C3b-01 | TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent | integration | tests/integration/assistant/microtools_prompt_contract_test.go | allowed_tools includes location_normalize + weather_lookup; prompt ≥40% shrunk; no inline dictionary | BUG-031-008-SCN-007 |
| T-C3b-02 | scenario-lint (`./smackerel.sh check`) | gate | config/prompt_contracts | weather-query-v1 validates with the added tool | BUG-031-008-SCN-007 |

### Shared Infrastructure Impact Sweep

C3b is additive to one scenario's `allowed_tools` (`location_normalize` is already a
registered, production-wired tool). It does not revert spec-061's
`direct_output_from_tool: weather_lookup` optimization or re-bloat the prompt. The
existing `allowed_tools_lists_location_normalize` subtest is the adversarial regression
guard (fails if `location_normalize` is dropped again). Scope 4 verifies the broader
weather/assistant integration tests stay green so the additive entry does not perturb
live tool-selection behavior.

### Change Boundary

Allowed: `config/prompt_contracts/weather-query-v1.yaml` (`allowed_tools` only).
Excluded: every spec-083 path; the weather system_prompt; framework files.

### Definition of Done

- [x] `location_normalize` (`side_effect_class: external`) added to weather-query-v1.yaml allowed_tools; system_prompt + direct_output_from_tool untouched — **Phase:** implement
  → Evidence: [report.md#c3b-implement](report.md) (diff)
- [x] T-C3b-01 reproduced RED at baseline (only allowed_tools subtest failed) AND all 3 subtests pass GREEN after the fix — **Phase:** test
  → Evidence: [report.md#c3b-repro](report.md) (RED) + [report.md#c3b-after](report.md) (GREEN, all 3 subtests)
- [x] Build Quality Gate: scenario-lint green (stack bring-up validated weather-query-v1 with the added tool); zero warnings; spec-083 untouched; no broader weather/assistant integration regression (tests/integration/assistant PASS) — **Phase:** audit
  → Evidence: [report.md#c3b-lint](report.md) + [report.md#c3b-audit](report.md)

## Scope 5: C4 — TestLocationNormalizeIntegration stub-geocoder honest skip

**Status:** Done
**Priority:** P2
**Depends On:** None

> Surfaced by the reproduction as a SEPARATE failing cluster (the operator flagged it as a
> defensive re-check under C3b). C3b's yaml fix does NOT resolve it. See report.md#c4-repro.

### Gherkin Scenarios

```gherkin
Feature: BUG-031-008 C4 location_normalize integration honest skip against the stub geocoder

  Scenario: BUG-031-008-SCN-009 stub geocoder is detected and the test honestly skips
    Given the test-stack injects the canned stub geocoder (returns "Reykjavík" for all inputs)
    When TestLocationNormalizeIntegration probes the wired geocoder for a canonical location
    Then the test skips with a clear reason citing F-065-LOCATION-STUB and spec-076 ownership

  Scenario: BUG-031-008-SCN-010 a real open-meteo geocoder still exercises the assertions
    Given a host wires ASSISTANT_SKILLS_WEATHER_GEOCODE_URL to a real open-meteo endpoint
    When the probe returns a non-Reykjavík canonical location
    Then the test runs its palm springs / sf canonical-resolution assertions fully
```

### Implementation Plan

1. Reproduce: confirm TestLocationNormalizeIntegration fails because the test-stack
   external-provider stub (spec-061 §18.4) returns "Reykjavík" for every geocode input
   (F-065-LOCATION-STUB), unrelated to C3b's allowed_tools fix.
2. In `tests/integration/assistant/microtools_location_test.go`, add `skipIfStubGeocoder(t)`:
   probe the wired geocoder for "Tokyo"; if it returns Reykjavík (the canned stub),
   `t.Skip(...)` citing F-065-LOCATION-STUB + spec-076 (`TestMicroToolOverlays_FullMatrix`)
   ownership of the real-provider coverage. A real-geocoder host still runs fully.
3. Re-run (expect honest SKIP against the stub).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-C4-01 | TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations | integration (live) | tests/integration/assistant/microtools_location_test.go | honest SKIP against the canned stub; full palm springs / sf assertions against a real geocoder | BUG-031-008-SCN-009, BUG-031-008-SCN-010 |

### Shared Infrastructure Impact Sweep

C4 adds a stub-detection skip guard to one orphaned test (spec-065 SCOPE-2 superseded →
spec-076). No production/shared surface changes. The skip is honest (the canned stub
structurally cannot return real place names; real-provider coverage lives in spec-076's
TestMicroToolOverlays_FullMatrix) and non-masking (a real-geocoder host still runs the
assertions).

### Change Boundary

Allowed: `tests/integration/assistant/microtools_location_test.go` only. Excluded: every
spec-083 path; the stub-providers container; framework files.

### Definition of Done

- [x] `skipIfStubGeocoder` added: probes the wired geocoder and skips when it is the canned Reykjavík stub, citing F-065-LOCATION-STUB + spec-076 ownership — **Phase:** implement
  → Evidence: [report.md#c4-implement](report.md) (diff)
- [x] T-C4-01 reproduced RED at baseline (Reykjavík for palm springs/sf) AND honest SKIP after the fix against the stub-backed test stack — **Phase:** test
  → Evidence: [report.md#c4-repro](report.md) (RED ≥10 lines) + [report.md#c4-after](report.md) (SKIP)
- [x] Build Quality Gate: zero warnings; spec-083 untouched; skip is honest + non-masking (real-geocoder host still runs assertions); classified as SEPARATE from C3b — **Phase:** audit
  → Evidence: [report.md#c4-audit](report.md)
