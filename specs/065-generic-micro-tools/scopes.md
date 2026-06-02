# Scopes: 065 Generic Micro-Tools

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

1. **Micro-Tool Foundation and Fail-Loud Config** — Define the common envelope, schema-validation contract, registry wiring pattern, SST config surface, and startup validation used by every micro-tool. `foundation:true`.
2. **Location Normalization and Ambiguity Handling** — Implement `location_normalize` with provider attribution, bounded candidates, cache policy, and assistant clarification behavior.
3. **Deterministic Conversion and Computation Tools** — Implement `unit_convert` and `calculator` as no-egress micro-tools with strict parsers and source-qualified outputs.
4. **Entity Resolution and Scenario Adoption** — Implement `entity_resolve`, wire micro-tools through scenario allowlists, reduce weather prompt normalization burden, and prove cross-scenario tool composition.

### New Types and Signatures

- `internal/agent/tools/microtools.Envelope` with `schema_version`, `status`, `value`, `candidates`, `confidence`, `source`, and `error` fields.
- `internal/agent/tools/microtools.Source` with provider, source kind, retrieved-at, and attribution fields.
- `internal/agent/tools/microtools.Candidate` with rank, label, canonical value, confidence, and distinguishing fields.
- `internal/agent/tools/microtools.LocationProvider` selected only from required SST keys.
- `internal/agent/tools/microtools.EntityResolver` wrapping user-scoped graph/search primitives.
- SST keys under `assistant.tools.location_normalize.*`, `assistant.tools.unit_convert.*`, `assistant.tools.calculator.*`, and `assistant.tools.entity_resolve.*`.

### Validation Checkpoints

- After Scope 1, config and registry unit tests prove every required tool key fails loud and every output envelope is schema-valid before registration.
- After Scope 2, integration tests prove `palm springs ca`, `sf`, and ambiguous `springfield` behavior against the configured provider and assistant clarification path.
- After Scope 3, deterministic unit and HTTP E2E tests prove conversion and calculator behavior, including adversarial parser rejection.
- After Scope 4, live HTTP E2E tests prove the agent composes micro-tools without scenario-local normalization code and that weather prompt size is reduced by the required threshold.

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | Micro-Tool Foundation and Fail-Loud Config | None | agent registry, config validation, schemas, traces | unit, integration, Regression E2E | foundation envelope, SST validation, schema validation, no defaults | Not Started |
| 2 | Location Normalization and Ambiguity Handling | 1 | microtool provider, cache, assistant clarification | unit, integration, e2e-api, Regression E2E | canonical locations, ambiguity envelope, provider source | Not Started |
| 3 | Deterministic Conversion and Computation Tools | 1 | unit conversion catalog, calculator parser | unit, e2e-api, Regression E2E | conversion source, pure math, rejected identifiers | Not Started |
| 4 | Entity Resolution and Scenario Adoption | 1, 2, 3 | graph resolver, scenario YAML, weather prompt, HTTP E2E | unit, integration, e2e-api, Regression E2E | user-scoped entity resolution, prompt reduction, cross-scenario composition | Not Started |

---

## Scope 1: Micro-Tool Foundation and Fail-Loud Config

**Status:** Not Started  
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

- Add a common micro-tool envelope with `resolved`, `ambiguous`, and `failed` states; validate output schema before executor persistence.
- Register micro-tools through the existing spec 037 registry only; do not add a second registry or scenario-specific dispatch branch.
- Extend config loading with required `assistant.tools.*` blocks and explicit empty-value errors; regenerate env files through the repo CLI only.
- Record tool name, status, source/provider, elapsed time, schema validation result, and trace id in the existing trace payload.
- **Shared Infrastructure Impact Sweep:** registry, executor, and config loading are shared surfaces. Downstream contracts include scenario loading, tool allowlists, trace persistence, and assistant startup. Canary rows validate existing tool registration and executor schema retry behavior before broad suite reruns.
- **Change Boundary:** allowed file families are `internal/agent/**`, `internal/config/**`, `config/smackerel.yaml`, generated config artifacts, and tests named for assistant micro-tools. Excluded surfaces are Telegram handlers, assistant HTTP transport, legacy command retirement code, and unrelated domain tools.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Config validation | unit | SCN-065-A07 | `internal/config/assistant_tools_test.go` | `TestAssistantToolsConfigRequiresEveryMicroToolKey` | `./smackerel.sh test unit` | No |
| Registry contract | unit | SCN-065-A07 | `internal/agent/tools/microtools/envelope_test.go` | `TestMicroToolEnvelopeSchemaRejectsMissingSource` | `./smackerel.sh test unit` | No |
| Canary: existing executor schema loop | integration | SCN-065-A07 | `tests/integration/assistant/microtools_registry_canary_test.go` | `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` | `./smackerel.sh test integration` | Yes |
| Regression E2E: missing config fails loud | e2e-api | SCN-065-A07 | `tests/e2e/assistant/microtools_config_e2e_test.go` | `TestMicroToolsE2E_MissingLocationProviderFailsStartup` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] Common envelope and source metadata are defined once and every micro-tool output validates against that envelope.
- [ ] Required SST keys for all four tools are loaded from `config/smackerel.yaml` and missing or empty values fail startup with named errors.
- [ ] Tool registry wiring stays inside the existing spec 037 registry and no second registry or scenario-local dispatch path is introduced.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Change Boundary is respected and zero excluded file families are changed.
- [ ] Scenario-specific E2E regression coverage exists for SCN-065-A07.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 2: Location Normalization and Ambiguity Handling

**Status:** Not Started  
**Depends On:** Scope 1  
**Surfaces:** `internal/agent/tools/microtools/location_normalize*.go`, provider client, in-process cache, assistant clarification response, trace detail rendering fields.

### Gherkin Scenarios

```gherkin
Scenario: SCN-065-A01 — location_normalize resolves abbreviated US state
  Given the assistant is configured with location_normalize backed by open-meteo geocoding
  When the agent calls location_normalize with input "palm springs ca"
  Then the tool returns a canonical location { name, country, admin1, lat, lon } with admin1 = "California"
  And the call succeeds without retry within the per-tool timeout

Scenario: SCN-065-A02 — location_normalize resolves common city nickname
  Given the assistant is configured with location_normalize
  When the agent calls location_normalize with input "sf"
  Then the tool returns a canonical location with name containing "San Francisco" and admin1 = "California"

Scenario: SCN-065-A03 — location_normalize returns ambiguous-result envelope for borderline input
  Given the assistant is configured with location_normalize
  When the agent calls location_normalize with input "springfield"
  Then the tool returns status = "ambiguous" with a ranked candidate list (<=5)
  And the agent loop surfaces a spec 061 disambiguation prompt rather than guessing
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-065-A03 | Ambiguous provider result | Send `springfield weather tomorrow` through assistant HTTP | User sees ranked candidates with source/provider badge and no guessed city | e2e-api | `report.md#scope-2` |
| SCN-065-A01/2 | Resolved provider result | Send weather turns using `palm springs ca` and `sf` | User sees final answer without micro-tool jargon; trace contains provider/source details | e2e-api | `report.md#scope-2` |

### Implementation Plan

- Implement `LocationProvider` and Open-Meteo geocoding adapter selected by required SST provider key.
- Normalize provider responses into canonical location values with source/provider attribution and stable ambiguity ranking.
- Keep cache local to `location_normalize`, keyed by provider, normalized input, and config version.
- Surface ambiguous envelopes through the existing assistant disambiguation prompt; no renderer invents candidates.
- **Consumer Impact Sweep:** weather scenario prompt and trace-detail UI consume this tool. Update prompt contract, source/provider badges, operator trace rows, and tests that assert old prompt-side normalization text.
- **Change Boundary:** allowed file families are micro-tool location files, weather scenario YAML, assistant trace DTOs, and assistant E2E tests. Excluded surfaces are `internal/api/domain_intent.go`, Telegram legacy alias code, and non-weather domain handlers.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Provider mapping | unit | SCN-065-A01 | `internal/agent/tools/microtools/location_normalize_test.go` | `TestLocationNormalizeMapsOpenMeteoPalmSpringsCA` | `./smackerel.sh test unit` | No |
| Nickname mapping | unit | SCN-065-A02 | `internal/agent/tools/microtools/location_normalize_test.go` | `TestLocationNormalizeMapsSFToSanFrancisco` | `./smackerel.sh test unit` | No |
| Ambiguity envelope | unit | SCN-065-A03 | `internal/agent/tools/microtools/location_normalize_test.go` | `TestLocationNormalizeReturnsAmbiguousEnvelopeForSpringfield` | `./smackerel.sh test unit` | No |
| Provider integration | integration | SCN-065-A01, SCN-065-A02 | `tests/integration/assistant/microtools_location_test.go` | `TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations` | `./smackerel.sh test integration` | Yes |
| Regression E2E: ambiguity prompt | e2e-api | SCN-065-A03 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_SpringfieldProducesClarificationCandidates` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: weather normalization | e2e-api | SCN-065-A01, SCN-065-A02 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_WeatherUsesLocationNormalizeBeforeLookup` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] `location_normalize` resolves `palm springs ca` and `sf` into canonical location envelopes with source/provider attribution.
- [ ] Ambiguous input returns a bounded ranked candidate list and routes to spec 061 clarification without guessing.
- [x] Weather prompt contract instructs the agent to call `location_normalize` and removes prompt-side provider quirks by the required size threshold. *(Evidence: report.md#scope-2 — prompt block 1764→721 bytes = 59% reduction; allowed_tools lists location_normalize; TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent PASS.)*
- [ ] Consumer Impact Sweep proves no first-party test, prompt, or trace view still depends on prompt-side location dictionaries.
- [ ] Scenario-specific E2E regression coverage exists for SCN-065-A01, SCN-065-A02, and SCN-065-A03.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 3: Deterministic Conversion and Computation Tools

**Status:** Not Started  
**Depends On:** Scope 1  
**Surfaces:** `internal/agent/tools/microtools/unit_convert*.go`, conversion catalog, `calculator*.go`, parser tests, assistant HTTP E2E.

### Gherkin Scenarios

```gherkin
Scenario: SCN-065-A04 — unit_convert performs canonical conversion
  Given the assistant is configured with unit_convert
  When the agent calls unit_convert with { value: 3, from: "cup", to: "g", substance: "flour" }
  Then the tool returns a numeric result with explicit precision and source attribution

Scenario: SCN-065-A05 — calculator evaluates pure math expression
  Given the assistant is configured with calculator
  When the agent calls calculator with expression "(15 * 1.08875) + 12"
  Then the tool returns the numeric result with at most 6 significant digits
  And refuses any expression containing identifiers or function calls outside its allowlist
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-065-A04 | Unit conversion requested in an assistant turn | Send `convert 3 cups of flour to grams` through HTTP | Assistant responds with grams, precision, and source information | e2e-api | `report.md#scope-3` |
| SCN-065-A05 | Arithmetic requested in an assistant turn | Send a pure arithmetic expression and an adversarial identifier expression | Pure math succeeds; identifier/function expression is refused | e2e-api | `report.md#scope-3` |

### Implementation Plan

- Implement deterministic volume, mass, and substance-aware conversion catalog with catalog version in source metadata.
- Return ambiguous or failed envelopes when substance density is required but unavailable; never invent a density.
- Implement calculator with a dedicated numeric parser and bounded grammar; no shell, eval, SQL, imports, network, identifiers, or host functions.
- Wire both tools through scenario allowlists and trace rows.
- **Change Boundary:** allowed file families are micro-tool deterministic files, conversion catalog fixtures, and assistant E2E tests. Excluded surfaces are weather provider code, entity resolver, Telegram command retirement, and config outside required tool keys.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Conversion correctness | unit | SCN-065-A04 | `internal/agent/tools/microtools/unit_convert_test.go` | `TestUnitConvert_FlourCupsToGramsWithSource` | `./smackerel.sh test unit` | No |
| Conversion ambiguity | unit | SCN-065-A04 | `internal/agent/tools/microtools/unit_convert_test.go` | `TestUnitConvert_VolumeToMassRequiresSubstanceDensity` | `./smackerel.sh test unit` | No |
| Calculator correctness | unit | SCN-065-A05 | `internal/agent/tools/microtools/calculator_test.go` | `TestCalculator_EvaluatesBoundedArithmetic` | `./smackerel.sh test unit` | No |
| Calculator adversarial reject | unit | SCN-065-A05 | `internal/agent/tools/microtools/calculator_test.go` | `TestCalculator_RejectsIdentifiersFunctionsAndNonFiniteValues` | `./smackerel.sh test unit` | No |
| Regression E2E: unit conversion | e2e-api | SCN-065-A04 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: calculator safety | e2e-api | SCN-065-A05 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_CalculatorRejectsUnsafeExpression` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] `unit_convert` returns numeric values with precision and source metadata for supported conversions.
- [ ] Substance-aware conversion fails or asks for clarification when required density data is absent.
- [ ] `calculator` evaluates only the approved grammar and rejects identifiers, functions, imports, assignment, non-finite values, and external effects.
- [ ] Scenario-specific E2E regression coverage exists for SCN-065-A04 and SCN-065-A05.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 4: Entity Resolution and Scenario Adoption

**Status:** Not Started  
**Depends On:** Scope 1, Scope 2, Scope 3  
**Surfaces:** `internal/agent/tools/microtools/entity_resolve*.go`, graph/search client integration, scenario YAML allowlists, weather prompt contract, assistant HTTP E2E, trace detail rows.

### Gherkin Scenarios

```gherkin
Scenario: SCN-065-A06 — entity_resolve maps colloquial domain terms
  Given the assistant is configured with entity_resolve over the user knowledge graph
  When the agent calls entity_resolve with { input: "the lease", scope: "documents" }
  Then the tool returns the top-ranked artifact reference and a confidence score
  And returns status = "ambiguous" when the top score is below the configured floor
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-065-A06 | User graph has multiple document candidates | Ask for `the lease` through the assistant HTTP surface | Assistant either resolves to the user-scoped artifact with confidence or asks for a concrete disambiguation choice | e2e-api | `report.md#scope-4` |
| SCN-065-A01/A02/A04 | Micro-tools are allowlisted by scenarios | Ask weather and conversion turns through HTTP | Responses are correct and traces show micro-tool calls rather than scenario-local parsing | e2e-api | `report.md#scope-4` |

### Implementation Plan

- Implement `entity_resolve` against authenticated user graph/search primitives with exact, recent-context, relation, then vector ranking.
- Enforce user-scoped queries and reject cross-user artifacts in unit and integration tests.
- Add micro-tools to relevant scenario `allowed_tools` lists and remove prompt-side normalization text where the tool now owns the behavior.
- Keep `internal/api/domain_intent.go` deletion in spec 066; this scope only provides the reusable resolver and scenario adoption needed by that removal.
- **Consumer Impact Sweep:** scenario YAML, tool registry docs, trace views, tests for weather prompt size, and spec 066 consumers that rely on `entity_resolve` are inventoried. No direct deletion of legacy parsers occurs in this spec.
- **Change Boundary:** allowed file families are micro-tool resolver files, scenario YAML under `config/prompt_contracts/`, trace DTOs, and assistant E2E tests. Excluded surfaces are legacy Telegram command handlers and `/find` HTTP API deletion.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Resolver ranking | unit | SCN-065-A06 | `internal/agent/tools/microtools/entity_resolve_test.go` | `TestEntityResolveRanksExactRecentRelationThenVectorCandidates` | `./smackerel.sh test unit` | No |
| User isolation | integration | SCN-065-A06 | `tests/integration/assistant/entity_resolve_test.go` | `TestEntityResolveIntegration_UserScopedGraphCandidatesOnly` | `./smackerel.sh test integration` | Yes |
| Ambiguity floor | integration | SCN-065-A06 | `tests/integration/assistant/entity_resolve_test.go` | `TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous` | `./smackerel.sh test integration` | Yes |
| Prompt-size regression | unit | SCN-065-A01, SCN-065-A02 | `tests/integration/assistant/microtools_prompt_contract_test.go` | `TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent` | `./smackerel.sh test integration` | Yes |
| Regression E2E: entity resolve | e2e-api | SCN-065-A06 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_EntityResolveClarifiesLowConfidenceLease` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: composed tools | e2e-api | SCN-065-A01, SCN-065-A02, SCN-065-A04 | `tests/e2e/assistant/microtools_http_test.go` | `TestMicroToolsE2E_ComposesWeatherAndUnitConversionWithoutScenarioParsing` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] `entity_resolve` returns user-scoped ranked artifact references with confidence and ambiguity behavior. *(Evidence: report.md#scope-4 — TestEntityResolveIntegration_UserScopedGraphCandidatesOnly + TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous PASS; cmd/core wiring constructs searchEngineEntityResolver and calls SetEntityResolveServices.)*
- [x] Scenario `allowed_tools` lists include the relevant micro-tools and prompt-side normalization text is removed where the tool owns the behavior. *(Evidence: report.md#scope-4 — weather-query-v1.yaml + retrieval-qa-v1.yaml updated; prompt-contract test verifies.)*
- [x] Weather prompt size reduction meets the Success Signal threshold and is protected by a regression test. *(Evidence: report.md#scope-4 — 59% reduction (>=40% threshold); TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent regression PASS.)*
- [ ] Consumer Impact Sweep proves spec 066 can consume `entity_resolve` without relying on regex intent parsing.
- [ ] Scenario-specific E2E regression coverage exists for SCN-065-A06 and for cross-scenario micro-tool composition.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.
