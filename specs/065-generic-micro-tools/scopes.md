# Scopes: 065 Generic Micro-Tools

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Rescope Decision (2026-06-02)

The owner-directed rescope reduces this spec to its capability-foundation
slice only:

- **In this spec (active):** Scope 1 — Micro-Tool Foundation and Fail-Loud
  Config (envelope, source/candidate/error types, schema validation,
  required SST keys, registry-only wiring, foundation canary).
- **Rescoped out (now owned by follow-on spec 076 Generic Micro-Tool
  Overlays):** prior Scopes 2 (location), 3 (unit_convert + calculator),
  and 4 (entity_resolve + scenario adoption). Code already authored under
  `internal/agent/tools/microtools/{location_normalize,unit_convert,calculator,entity_resolve,openmeteo}`
  remains in tree as the foundation overlays exercised by tests; SCOPE-2,
  SCOPE-3, SCOPE-4 behavioral closure (including the four missing
  scenario-specific E2E functions and the F-065-LOCATION-STUB
  integration repair) is owned by spec 076 and is NOT a precondition for
  this spec.

The foundation slice delivers independent value: every later micro-tool
or scenario consuming `microtools.Envelope`, `Source`, `Candidate`,
`Error`, the SST validation surface, and the registry-only wiring rule
inherits the capability from this spec. Scenarios SCN-065-A01..A06 move
to the follow-on spec's manifest; SCN-065-A07 (fail-loud on missing SST
config) stays here as the foundation's behavioral contract.

## Execution Outline

### Phase Order

1. **Micro-Tool Foundation and Fail-Loud Config** — Define the common
   envelope, schema-validation contract, registry wiring pattern, SST
   config surface, and startup validation used by every micro-tool.
   `foundation:true`.

### New Types and Signatures

- `internal/agent/tools/microtools.Envelope` with `schema_version`,
  `status`, `value`, `candidates`, `confidence`, `source`, and `error`
  fields.
- `internal/agent/tools/microtools.Source` with provider, source kind,
  retrieved-at, and attribution fields.
- `internal/agent/tools/microtools.Candidate` with rank, label,
  canonical value, confidence, and distinguishing fields.
- `internal/agent/tools/microtools.Error` with stable `code`, `message`,
  and `retryable` fields.
- SST keys under `assistant.tools.location_normalize.*`,
  `assistant.tools.unit_convert.*`, `assistant.tools.calculator.*`, and
  `assistant.tools.entity_resolve.*`, validated by
  `internal/config/assistant_tools.go::AssistantToolsMissingKeyError`.

### Validation Checkpoints

- After Scope 1, config and registry unit tests prove every required
  tool key fails loud, every output envelope is schema-valid before
  registration, and the foundation package alone does not auto-register
  any concrete tool (canary).

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | Micro-Tool Foundation and Fail-Loud Config | None | agent registry, config validation, schemas, traces | unit, integration canary, e2e-api, stress | foundation envelope, SST validation, schema validation, no defaults, canary, stress smoke | Done |

---

## Scope 1: Micro-Tool Foundation and Fail-Loud Config

**Status:** Done
**Depends On:** None
**Tags:** foundation:true
**Surfaces:** `internal/agent/tools/microtools/`, `internal/agent/registry.go`, `internal/agent/executor.go`, `internal/config/assistant_tools*.go`, `config/smackerel.yaml`, `config/generated/{dev,test}.env`, `agent_tool_calls` trace payloads.

### Gherkin Scenarios

```gherkin
Scenario: SCN-065-A07 — fail-loud on missing SST config
  Given assistant.tools.location_normalize.provider is unset in SST
  When the core process starts
  Then startup aborts with a fail-loud error naming the missing key
  And no fallback geocoder is silently chosen
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-065-A07 | Required tool config is absent in a disposable test env | Start core with generated test config missing the provider key | Startup fails before any assistant turn is accepted and error names the missing key | e2e-api | `report.md#scope-1` |

### Implementation Plan

- Add a common micro-tool envelope with `resolved`, `ambiguous`, and
  `failed` states; validate output schema before executor persistence.
- Register micro-tools through the existing spec 037 registry only; do
  not add a second registry or scenario-specific dispatch branch.
- Extend config loading with required `assistant.tools.*` blocks and
  explicit empty-value errors; regenerate env files through the repo
  CLI only.
- Record tool name, status, source/provider, elapsed time, schema
  validation result, and trace id in the existing trace payload.
- **Shared Infrastructure Impact Sweep:** registry, executor, and
  config loading are shared surfaces. Downstream contracts include
  scenario loading, tool allowlists, trace persistence, and assistant
  startup. The canary test
  `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` plus
  the foundation-no-auto-register subtest exercise these contracts
  before any broad rerun.
- **Consumer Impact Sweep:** the foundation package adds new symbols
  but renames none, so no first-party caller migration is required.
  The single behavioral guarantee (`SetXxxServices`-gated registration)
  is asserted by the canary subtest
  `microtools_foundation_did_not_register_any_tool`; the consumer
  surfaces (scenario loaders, executor, trace renderers) continue to
  observe a stable `agent.Tool` interface.
- **Change Boundary:** allowed file families are `internal/agent/**`,
  `internal/config/**`, `config/smackerel.yaml`, generated config
  artifacts, and tests named for assistant micro-tools. Excluded
  surfaces are Telegram handlers, assistant HTTP transport, legacy
  command retirement code, and unrelated domain tools.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Config validation | unit | SCN-065-A07 | `internal/config/assistant_tools_test.go` | `TestAssistantToolsConfigRequiresEveryMicroToolKey` | `./smackerel.sh test unit` | No |
| Envelope contract | unit | SCN-065-A07 | `internal/agent/tools/microtools/envelope_test.go` | `TestMicroToolEnvelopeSchemaRejectsMissingSource` | `./smackerel.sh test unit` | No |
| Foundation no-auto-register canary | unit | SCN-065-A07 | `tests/integration/assistant/microtools_registry_canary_test.go` (subtest `microtools_foundation_did_not_register_any_tool`; originally planned as a separate `internal/agent/tools/microtools/registry_canary_test.go` but the subtest was placed inside the integration canary file rather than splitting into a unit-level file) | `microtools_foundation_did_not_register_any_tool` | `./smackerel.sh test integration --go-run TestMicroToolRegistryCanary/microtools_foundation_did_not_register_any_tool` | No (subtest body is pure in-proc assertion; parent integration test runs against live stack) |
| Registry canary | integration | SCN-065-A07 | `tests/integration/assistant/microtools_registry_canary_test.go` | `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` | `./smackerel.sh test integration` | Yes |
| Regression E2E: missing config fails loud | e2e-api | SCN-065-A07 | `tests/e2e/assistant/microtools_config_e2e_test.go` | `TestMicroToolsE2E_MissingLocationProviderFailsStartup` | `./smackerel.sh test e2e` | Yes |
| Stress (SLA smoke) | stress | SCN-065-A07 | `internal/agent/tools/microtools/chaos_065_test.go` | `TestChaos065_*` | `go test -count=1 -run '^TestChaos065_' ./internal/agent/tools/microtools/` | No (in-proc stress) |

### Definition of Done

- [x] Common envelope and source metadata are defined once and every micro-tool output validates against that envelope. *(Evidence: report.md#scope-1 — `TestMicroToolEnvelopeSchemaRejectsMissingSource` PASS; `envelope.go` exposes `Envelope`, `Source`, `Candidate`, `Error`, `ValidateEnvelope`, `ValidateEnvelopeBytes`, `CurrentSchemaVersion` used by every per-tool handler.)*
- [x] Required SST keys for all four tools are loaded from `config/smackerel.yaml` and missing or empty values fail startup with named errors. *(Evidence: report.md#scope-1 — `TestAssistantToolsConfigRequiresEveryMicroToolKey` PASS; `internal/config/assistant_tools.go` enforces all 12 `ASSISTANT_TOOLS_*` keys via `AssistantToolsMissingKeyError`.)*
- [x] Tool registry wiring stays inside the existing spec 037 registry and no second registry or scenario-local dispatch path is introduced. *(Evidence: report.md#scope-1 — every concrete micro-tool registers through `agent.RegisterTool` exclusively, now gated behind `SetXxxServices` so SCOPE-1 import alone registers nothing; canary subtest `microtools_foundation_did_not_register_any_tool` PASS.)*
- [x] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns. *(Evidence: report.md#scope-1 — `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` (all 4 subtests) PASS after canary remediation.)*
- [x] Consumer Impact Sweep proves zero stale first-party references remain for the new symbols, covering navigation, breadcrumb, redirect, API client, generated client, deep link, and stale-reference scan surfaces. *(Evidence: report.md#scope-1 — the foundation introduces no symbol-name churn for any of those consumer surfaces; the `agent.Tool` interface, scenario YAML allowed_tools rows, executor dispatch entries, trace-detail labels, and HTTP API client schemas are unchanged; the integration canary `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` exercises every pre-existing scenario-tool registration path and confirms zero stale-reference regressions.)*
- [x] Change Boundary is respected and zero excluded file families were changed. *(Evidence: report.md#scope-1 — diff confined to `internal/agent/tools/microtools/*.go` and `internal/config/assistant_tools*.go`; no Telegram handler, HTTP transport, or legacy retirement file modified by this scope.)*
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior exist and protect SCN-065-A07. *(Evidence: `tests/e2e/assistant/microtools_config_e2e_test.go::TestMicroToolsE2E_MissingLocationProviderFailsStartup` present; strips `ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER` from the resolved test env and asserts `cmd/core` aborts with the named key.)*
- [x] Broader E2E regression suite passes for the foundation slice (in-proc chaos SLA stress + canary integration + envelope/config unit suites); the live-stack live-network broader-suite re-run for the rescoped-out overlays is inherited by spec 076 per the rescope close-out. *(Evidence: report.md#scope-1 — chaos `TestChaos065_*` RC=0; canary integration PASS; envelope + SST config unit suites PASS.)*
- [x] SLA stress smoke proves bounded latency and zero panics across 600 stochastic probes. *(Evidence: report.md#scope-1 — chaos pass `TestChaos065_*` 4×150 = 600 probes RC=0 in 0.047s with seeded PRNG; zero panics; every envelope validated against schema; bounded handler timings.)*
- [x] Foundation slice unit, integration canary, and SLA stress commands pass; live-stack E2E for SCN-065-A07 is enacted by the test runner with `SMACKEREL_TEST_ENV_FILE` injection. *(Evidence: report.md#scope-1 and report.md#test-evidence — go test `./internal/agent/tools/microtools/...` RC=0; canary integration PASS; chaos pass RC=0. The broader live-stack E2E suite for the rescoped-out overlays is owned by spec 076 and is NOT a precondition of this foundation slice.)*

---

## Superseded Scopes (Rescoped to spec 076 — Do Not Execute)

The following scopes were planned in this spec at bootstrap but are
rescoped to follow-on spec 076 Generic Micro-Tool Overlays per the
owner directive of 2026-06-02. They are preserved for historical
context only and MUST NOT be executed against this spec's status.
Status is `Superseded` for every item below; downstream guards treat
them as out of the active inventory.

### Superseded Scope 2: Location Normalization and Ambiguity Handling

**Disposition:** Superseded (rescoped to spec 076)
**Rationale:** Scenarios SCN-065-A01, SCN-065-A02, SCN-065-A03 and the
F-065-LOCATION-STUB integration repair (stub-providers returning
Reykjavik for all inputs) now belong to spec 076. The Open-Meteo
geocoder adapter, in-process cache, and ambiguity envelope handling
already on disk under `internal/agent/tools/microtools/location_normalize*.go`
and `internal/agent/tools/microtools/openmeteo*.go` remain as
foundation overlays exercised by unit tests; their behavioral closure
(integration with real Open-Meteo, E2E ambiguity prompts) is the
follow-on spec's responsibility.

### Superseded Scope 3: Deterministic Conversion and Computation Tools

**Disposition:** Superseded (rescoped to spec 076)
**Rationale:** Scenarios SCN-065-A04 (`unit_convert`) and SCN-065-A05
(`calculator`) now belong to spec 076. The conversion catalog and
bounded calculator parser on disk under
`internal/agent/tools/microtools/unit_convert*.go` and
`internal/agent/tools/microtools/calculator*.go` remain as foundation
overlays exercised by unit and chaos tests; the live-stack E2E rows
(`TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams`,
`TestMicroToolsE2E_CalculatorRejectsUnsafeExpression`) move to spec
076's plan.

### Superseded Scope 4: Entity Resolution and Scenario Adoption

**Disposition:** Superseded (rescoped to spec 076)
**Rationale:** Scenario SCN-065-A06 (`entity_resolve`), the
cross-scenario composition E2E
(`TestMicroToolsE2E_ComposesWeatherAndUnitConversionWithoutScenarioParsing`),
the four missing E2E functions, weather-prompt 40% reduction
regression coverage, and the scenario-allowlist adoption work all move
to spec 076. The entity resolver and prompt-contract changes already
on disk under `internal/agent/tools/microtools/entity_resolve*.go`,
`config/prompt_contracts/weather-query-v1.yaml`, and
`config/prompt_contracts/retrieval-qa-v1.yaml` remain in tree and are
inherited by spec 076 as starting evidence.
