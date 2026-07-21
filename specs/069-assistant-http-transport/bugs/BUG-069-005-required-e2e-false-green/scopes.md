# Scopes: BUG-069-005 Required assistant E2E false-green

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

1. Complete the compiler SST/provider/core vertical path.
2. Compose compiler outcomes into persistent disambiguation and confirmation
   state.
3. Make all five protected E2E tests deterministic and strict, prove the
   installed product guards, and record the foreign framework guard gap for
   upstream ownership.

## Mechanical Allowed List

The following concrete product paths are the complete implementation path set
changed at revision `ebf419414e1d1f824c77acf14025b1dda418c2a7`. Packet planning,
state, and evidence artifacts are not product implementation files.

### Implementation Files

#### Scope 1 Paths

- `cmd/core/main.go`
- `cmd/core/wiring_assistant_facade.go`
- `config/smackerel.yaml`
- `docker-compose.yml`
- `internal/assistant/intent/http_transport.go`
- `internal/assistant/intent/http_transport_test.go`
- `internal/config/assistant.go`
- `internal/config/assistant_intent_compiler.go`
- `internal/config/assistant_intent_compiler_test.go`
- `internal/config/assistant_test.go`
- `internal/config/validate_test.go`
- `ml/app/main.py`
- `ml/app/routes/intent_compile.py`
- `ml/tests/conftest.py`
- `ml/tests/test_intent_compiler.py`
- `ml/tests/test_intent_compiler_provider_fixture.py`
- `ml/tests/test_main.py`
- `scripts/commands/config.sh`
- `tests/e2e/intent-compiler-provider/provider.py`
- `tests/e2e/stub-providers/nginx.conf`
- `tests/integration/assistant/bug069005_runtime_canary_test.go`

#### Scope 2 Paths

- `cmd/core/wiring_assistant_actions.go`
- `internal/agent/tools/microtools/location_normalize.go`
- `internal/agent/tools/microtools/location_normalize_openmeteo.go`
- `internal/agent/tools/microtools/location_normalize_openmeteo_test.go`
- `internal/agent/tools/weather/tool.go`
- `internal/assistant/compiled_interactions.go`
- `internal/assistant/context/store.go`
- `internal/assistant/facade.go`

#### Scope 3 Paths

- `tests/e2e/assistant/annotation_intent_test.go`
- `tests/e2e/assistant/http_confirm_test.go`
- `tests/e2e/assistant/http_disambiguation_test.go`
- `tests/e2e/assistant/intent_clarify_test.go`
- `tests/e2e/assistant/intent_side_effect_test.go`
- `tests/e2e/assistant/required_compiler_state_helpers_test.go`

## Mechanical Excluded List

- Parent Spec 069 `spec.md`, `design.md`, `scopes.md`, `state.json`,
  `scenario-manifest.json`, `test-plan.json`, `report.md`, or
  `uservalidation.md` during implementation; reconciliation is validate-owned
  after proof.
- Any other spec or bug packet.
- Public assistant HTTP v1 schema changes.
- Anonymous, shared-secret, or test-only product ingress.
- Runtime environment detection or hidden defaults.
- Release-train and feature-flag bundle files.
- Secrets, deployment, knb, target-host, or operator topology.
- Direct edits to Smackerel framework-managed Bubbles files.

## Scope Inventory

| Scope | Name | Depends On | Scenario IDs | Status |
|-------|------|------------|--------------|--------|
| 1 | Compiler SST, provider transport, and core wiring | none | SCN-BUG069005-001, SCN-BUG069005-002 | Done |
| 2 | Persistent disambiguation and confirmation state | Scope 1 | SCN-BUG069005-003, SCN-BUG069005-004, SCN-BUG069005-005 | Done |
| 3 | Strict required E2E and product guard proof | Scopes 1, 2 | all five | In Progress |

## Scope 1: Compiler SST, provider transport, and core wiring

**Status:** Done
**Priority:** P0
**Scope-Kind:** runtime-behavior
**Tags:** foundation-repair
**Depends On:** none

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG069005-001 - Annotation slots come from the live compiled intent
  Given the disposable test stack has the compiler enabled through the real ML route
  And the annotation input omits legacy annotation keywords
  When an authenticated user sends the annotation turn over POST /api/assistant/turn
  Then a persistent ConfirmCard is returned from compiled state-mutation slots
  And accepting it applies the compiled annotation values exactly once

Scenario: SCN-BUG069005-002 - Springfield ambiguity creates a persistent choice
  Given the compiler and location normalization capability are wired
  When an authenticated user asks for Springfield weather over HTTP
  Then the response contains a persistent DisambiguationPrompt with at least two choices
  And no weather lookup occurs before a choice is submitted
```

### Implementation Plan

1. Enforce fail-loud compiler/transport cross-field coherence and enable the
   compiler explicitly in disposable test config.
2. Implement the real ML `/assistant/intent/compile` route and production
   core-to-ML `intent.Transport` with existing auth/network policy.
3. Configure the test stack's external LLM dependency with deterministic
   compiler fixtures behind the same provider interface.
4. Construct `intent.NewLLMCompiler` and attach it exactly once through
   `Facade.WithIntentCompiler` in core wiring.
5. Add canaries that fail startup when HTTP is enabled but compiler/provider
   wiring is absent.

### Allowed Files

- `cmd/core/main.go`
- `cmd/core/wiring_assistant_facade.go`
- `config/smackerel.yaml`
- `docker-compose.yml`
- `internal/assistant/intent/http_transport.go`
- `internal/assistant/intent/http_transport_test.go`
- `internal/config/assistant.go`
- `internal/config/assistant_intent_compiler.go`
- `internal/config/assistant_intent_compiler_test.go`
- `internal/config/assistant_test.go`
- `internal/config/validate_test.go`
- `ml/app/main.py`
- `ml/app/routes/intent_compile.py`
- `ml/tests/conftest.py`
- `ml/tests/test_intent_compiler.py`
- `ml/tests/test_intent_compiler_provider_fixture.py`
- `ml/tests/test_main.py`
- `scripts/commands/config.sh`
- `tests/e2e/intent-compiler-provider/provider.py`
- `tests/e2e/stub-providers/nginx.conf`
- `tests/integration/assistant/bug069005_runtime_canary_test.go`

### Excluded Files

- assistant public schema
- confirmation/disambiguation persistence changes (Scope 2)
- release trains, flags, deployment, secrets
- parent Spec 069 artifacts

### Test Plan

| Row | Scenario | Category | File/Location | Expected Test Title | Command | Live System |
|-----|----------|----------|---------------|---------------------|---------|-------------|
| TP-BUG069005-04 | SCN-BUG069005-001 | Regression E2E API | `tests/e2e/assistant/annotation_intent_test.go` | `TestAnnotationIntentE2E_SlotsComeFromCompiledIntent` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069005-05 | SCN-BUG069005-002 | Regression E2E API | `tests/e2e/assistant/intent_clarify_test.go` | `TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069005-01 | SCN-BUG069005-001, SCN-BUG069005-002 | unit | `internal/config/assistant_intent_compiler_test.go` | `TestIntentCompilerRequiredWhenAssistantHTTPEnabled` | `./smackerel.sh test unit --go` | No |
| TP-BUG069005-02 | SCN-BUG069005-001, SCN-BUG069005-002 | functional | `ml/tests/test_intent_compiler.py` | `test_intent_compile_route_returns_schema_bound_fixture` | `./smackerel.sh test unit --python` | No |
| TP-BUG069005-03 | SCN-BUG069005-001, SCN-BUG069005-002 | integration | `tests/integration/assistant/intent_compiler_canary_test.go` | `TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] TP-BUG069005-01 proves an enabled HTTP assistant cannot start with the compiler disabled or unwired; no fallback raw-text path is selected.
   > **Phase:** implement
   > **Command:** `./smackerel.sh test unit --go`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > [go-unit] starting go test ./...
   > ok github.com/smackerel/smackerel/cmd/core
   > ok github.com/smackerel/smackerel/internal/assistant
   > ok github.com/smackerel/smackerel/internal/assistant/intent
   > ok github.com/smackerel/smackerel/internal/config
   > ok github.com/smackerel/smackerel/internal/assistant/httpadapter
   > ok github.com/smackerel/smackerel/internal/assistant/context
   > ok github.com/smackerel/smackerel/internal/assistant/confirm
   > ok github.com/smackerel/smackerel/internal/agent/tools/weather
   > [go-unit] go test ./... finished OK
   > ```
   > Evidence: [report.md#scope-1-go-and-config-unit-green](report.md#scope-1-go-and-config-unit-green)
- [x] TP-BUG069005-02 proves the real ML compiler route validates the request and returns schema-bound deterministic provider output without executing tools.
   > **Phase:** implement
   > **Command:** `./smackerel.sh test unit --python`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > [py-unit] pip install OK; starting unit-only pytest ml/tests
   > ........................................................................ [ 10%]
   > ........................................................................ [ 20%]
   > ........................................................................ [ 30%]
   > ........................................................................ [ 40%]
   > ........................................................................ [ 50%]
   > ........................................................................ [ 60%]
   > ........................................................................ [ 70%]
   > ........................................................................ [ 81%]
   > ........................................................................ [ 91%]
   > ............................................................... [100%]
   > 718 passed, 2 deselected in 12.72s
   > [py-unit] pytest ml/tests finished OK
   > ```
   > Evidence: [report.md#scope-1-python-compiler-route-green](report.md#scope-1-python-compiler-route-green)
- [x] TP-BUG069005-03 proves live core constructs one compiler transport/client and attaches it to the shared facade before HTTP adapter binding.
   > **Phase:** implement
   > **Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > go-integration: applying -run selector: ^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$
   > === RUN TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
   > --- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.07s)
   > === RUN TestCompilerInteractiveControlsPersistByUserAndTransport
   > --- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/integration/assistant 0.227s
   > PASS: go-integration
   > [py-integration] starting live integration pytest
   > 1 passed in 0.41s
   > [py-integration] live integration pytest finished OK
   > PASS: python-integration
   > ```
   > Evidence: [report.md#live-compiler-and-pending-state-integration-green](report.md#live-compiler-and-pending-state-integration-green)
- [x] TP-BUG069005-04 proves annotation behavior is driven by compiled slots using adversarial input that omits legacy keywords.
   > **Phase:** implement
   > **Command:** exact-five `./smackerel.sh test e2e --go-package assistant --go-run <anchored-selector>`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > === RUN TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.14s)
   > === RUN TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
   > === RUN TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.05s)
   > === RUN TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > === RUN TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#exact-five-required-green](report.md#exact-five-required-green)
- [x] TP-BUG069005-05 proves Springfield ambiguity reaches the compiler and returns at least two canonical choices before any weather lookup.
   > **Phase:** implement
   > **Command:** exact-five `./smackerel.sh test e2e --go-package assistant --go-run <anchored-selector>`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > Smackerel pre-flight resource check: OK
   > RAM available: 38794 MB (required >= 6000 MB)
   > Disk available: 676297 MB / 660.4 GB (required >= 15 GB)
   > Container smackerel-test-intent-compiler-provider-1 Healthy
   > Container smackerel-test-smackerel-core-1 Healthy
   > Container smackerel-test-smackerel-ml-1 Healthy
   > === RUN TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > === RUN TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.05s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#exact-five-required-green](report.md#exact-five-required-green)
- [x] SCN-BUG069005-001 - Annotation slots come from the live compiled intent: a persistent ConfirmCard is built from compiled state-mutation slots and acceptance applies those values exactly once.
   > **Phase:** implement
   > **Command:** exact-five E2E plus focused live integration selector
   > **Exit Code:** 0 for both
   > **Claim Source:** executed
   > ```text
   > === RUN TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.14s)
   > === RUN TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
   > === RUN TestCompilerInteractiveControlsPersistByUserAndTransport
   > --- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/integration/assistant 0.227s
   > PASS: go-integration
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > REQUIRED_FIVE_SKIP_CALLS=0
   > ```
   > Evidence: [report.md#exact-five-required-green](report.md#exact-five-required-green)
- [x] SCN-BUG069005-002 - Springfield ambiguity creates a persistent choice: the live response carries at least two persistent choices and no weather lookup occurs before selection.
   > **Phase:** implement
   > **Command:** exact-five E2E and live compiler canary
   > **Exit Code:** 0 for both
   > **Claim Source:** executed
   > ```text
   > === RUN TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
   > --- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.07s)
   > === RUN TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > === RUN TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.05s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/integration/assistant 0.227s
   > PASS: go-integration
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > Container smackerel-test-intent-compiler-provider-1 Removed
   > Network smackerel-test_default Removed
   > ```
   > Evidence: [report.md#live-compiler-and-pending-state-integration-green](report.md#live-compiler-and-pending-state-integration-green)
- [x] Change Boundary is respected and zero excluded file families are changed.
   > **Phase:** implement
   > **Command:** `git diff --check && git status --short`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > M cmd/core/main.go
   > M cmd/core/wiring_assistant_facade.go
   > M config/smackerel.yaml
   > M docker-compose.yml
   > M internal/agent/tools/microtools/location_normalize.go
   > M internal/agent/tools/microtools/location_normalize_openmeteo.go
   > M internal/agent/tools/weather/tool.go
   > M internal/assistant/context/store.go
   > M internal/assistant/facade.go
   > M internal/config/assistant.go
   > M internal/config/assistant_intent_compiler.go
   > M ml/app/main.py
   > M scripts/commands/config.sh
   > M tests/e2e/assistant/annotation_intent_test.go
   > M tests/e2e/assistant/intent_clarify_test.go
   > ```
   > Evidence: [report.md#build-quality-and-governance](report.md#build-quality-and-governance)
- [x] Build Quality Gate passes for touched config, Go, Python, and integration surfaces with zero warnings.
   > **Phase:** test
   > **Executed:** YES
   > **Claim Source:** executed
   > **Commands:** focused Go/Python units; focused live integration; protected weather E2E; `check`; `lint`; `format --check`; artifact, traceability, reality, and regression guards
   > **Exit Codes:** all 0
   > ```text
   > ok github.com/smackerel/smackerel/internal/assistant
   > 1 passed, 720 deselected
   > PASS: go-integration
   > PASS: python-integration
   > PASS: go-e2e
   > Config is in sync with SST
   > All checks passed!
   > Web validation passed
   > 78 files already formatted
   > Violations: 0
   > Warnings: 0
   > ```
   > Evidence: [report.md#weather-implementation-and-test-closeout](report.md#weather-implementation-and-test-closeout)

All items require current-session command evidence before checking.

## Test Continuation Evidence: Weather And Readiness (2026-07-21)

This section is additive corrective evidence only. It changes no checkbox,
scope status, or certification field. The earlier focused integration failure
is resolved; the exact required weather behavior is the remaining blocker.

### Corrective Live Canary Evidence

> **Phase:** test
> **Executed:** YES (current continuation)
> **Claim Source:** executed
> **Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
> **Exit Code:** 0
> ```text
> go-integration: applying -run selector: ^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$
> === RUN   TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
> --- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.09s)
> === RUN   TestCompilerInteractiveControlsPersistByUserAndTransport
> --- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.05s)
> PASS
> ok      github.com/smackerel/smackerel/tests/integration/assistant      0.246s
> PASS: go-integration
> [py-integration] pip install OK; starting live integration pytest
> .                                                                        [100%]
> 1 passed in 0.41s
> [py-integration] live integration pytest finished OK
> PASS: python-integration
> ```
> **Result:** PASS - 2 selected Go canaries passed with 0 skips; the live
> compiler assertion proves `external_lookup`, `weather_query`, and
> `slots.location.raw=Barcelona`. Evidence:
> [report.md#exact-two-live-integration-canaries](report.md#exact-two-live-integration-canaries).

### Remaining Exact Weather Blocker

> **Phase:** test
> **Executed:** YES (current continuation)
> **Claim Source:** executed
> **Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation$'`
> **Exit Code:** 1
> ```text
> go-e2e: applying package selector: assistant
> go-e2e: applying -run selector: ^TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation$
> === RUN   TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation
>     intent_compiler_http_test.go:111: deterministic weather fixture routed to capture fallback; envelope={SchemaVersion:v1 Transport:web Status:saved_as_idea Sources:[] ErrorCause: CaptureRoute:true FacadeInvoked:true}
> --- FAIL: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation (7.33s)
> FAIL
> FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      7.364s
> FAIL
> FAIL: go-e2e (exit=1)
> Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
> Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
> ```
> **Result:** FAIL - 0 passed, 1 failed, 0 skipped. The protected test now
> fails loudly, and its focused AST guard passes with zero skip-family calls.
> Production ownership is required because the normal compiled weather path
> does not consume the resolved location slot or assemble a sourced weather
> response. Evidence:
> [report.md#exact-weather-e2e---fail-loud-blocker](report.md#exact-weather-e2e---fail-loud-blocker),
> [report.md#production-owner-root-cause-after-falsification](report.md#production-owner-root-cause-after-falsification).

### Continuation DoD Disposition

| Surface | Disposition |
|---|---|
| TP-BUG069005-13, scenario-specific E2E, broader E2E, Build Quality, and certification | No new checkbox is supported for checking while the exact required weather test is red. |
| TP-BUG069005-11 | Remains unchecked because the product AST guard does not close the separately owned generic Bubbles guard finding. |
| Scope, certification, and delivery | Status remains `in_progress`; no commit or push is authorized by this continuation. |

## Scope 2: Persistent disambiguation and confirmation state

**Status:** Done
**Priority:** P0
**Scope-Kind:** runtime-behavior
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG069005-003 - Disambiguation choice resolves pending state
  Given a prior HTTP turn persisted a DisambiguationPrompt
  When the user submits one listed choice with the issued reference
  Then the same pending state is cleared exactly once
  And the selected candidate drives the resumed assistant turn

Scenario: SCN-BUG069005-004 - List write is not persisted before confirmation
  Given the compiler returns a validated list-write intent
  When an authenticated user asks to add milk to a shopping list
  Then the response contains a persistent ConfirmCard
  And the list is unchanged until the issued confirm reference is accepted

Scenario: SCN-BUG069005-005 - Confirm acceptance executes the gated action once
  Given a prior HTTP turn persisted a ConfirmCard
  When the user accepts the issued confirm reference
  Then the proposed action executes exactly once
  And replaying the same reference does not execute it again
```

### Implementation Plan

1. Map compiled ambiguity metadata through the read-only location resolver into
   the established persistent disambiguation proposal/response shape.
2. Resolve callback choices against authenticated `(user, transport, ref)`,
   clear pending state once, and resume with the selected structured value.
3. Map compiled write/external-write intents into `confirm.Machine.Propose`,
   storing only server-validated proposal payloads.
4. Return existing `ConfirmCard` fields; on accept, execute the persisted
   proposal once and preserve replay rejection.
5. Assert exact-row state transitions and business effects against disposable
   Postgres, with no table-wide cleanup.

### Allowed Files

- `cmd/core/wiring_assistant_actions.go`
- `internal/agent/tools/microtools/location_normalize.go`
- `internal/agent/tools/microtools/location_normalize_openmeteo.go`
- `internal/agent/tools/microtools/location_normalize_openmeteo_test.go`
- `internal/agent/tools/weather/tool.go`
- `internal/assistant/compiled_interactions.go`
- `internal/assistant/context/store.go`
- `internal/assistant/facade.go`

### Excluded Files

- new pending-state tables
- client-side persistence/cache
- new public response fields or schema version
- raw-text classifiers or transport-specific scenario branches
- parent spec artifacts and unrelated assistant domains

### Test Plan

| Row | Scenario | Category | File/Location | Expected Test Title | Command | Live System |
|-----|----------|----------|---------------|---------------------|---------|-------------|
| TP-BUG069005-07 | SCN-BUG069005-003 | Regression E2E API | `tests/e2e/assistant/http_disambiguation_test.go` | `TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069005-08 | SCN-BUG069005-004 | Regression E2E API | `tests/e2e/assistant/intent_side_effect_test.go` | `TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069005-09 | SCN-BUG069005-005 | Regression E2E API | `tests/e2e/assistant/http_confirm_test.go` | `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069005-06 | SCN-BUG069005-003, SCN-BUG069005-004, SCN-BUG069005-005 | integration | `tests/integration/assistant/http_pending_state_test.go` | `TestCompilerInteractiveControlsPersistByUserAndTransport` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] TP-BUG069005-06 proves compiled interactive controls use the existing durable row family, are isolated by user/transport, and clear exactly once.
   > **Phase:** implement
   > **Command:** focused `./smackerel.sh test integration --go-run <anchored-selector>`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > Container smackerel-test-postgres-1 Healthy
   > Container smackerel-test-smackerel-core-1 Healthy
   > Container smackerel-test-smackerel-ml-1 Healthy
   > Container smackerel-test-intent-compiler-provider-1 Healthy
   > === RUN TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
   > --- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.07s)
   > === RUN TestCompilerInteractiveControlsPersistByUserAndTransport
   > --- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/integration/assistant 0.227s
   > PASS: go-integration
   > 1 passed in 0.41s
   > PASS: python-integration
   > ```
   > Evidence: [report.md#live-compiler-and-pending-state-integration-green](report.md#live-compiler-and-pending-state-integration-green)
- [x] TP-BUG069005-07 proves an issued disambiguation choice resolves the same pending state and uses the selected candidate; stale/cross-user refs do not resolve.
   > **Phase:** implement
   > **Command:** exact-five E2E and full assistant E2E package
   > **Exit Code:** 0 for both
   > **Claim Source:** executed
   > ```text
   > === RUN TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.05s)
   > === RUN TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > --- PASS: TestWhatsAppRoundTrip_TP_072_11_ControlsRoundTripIdentically (0.00s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 59.863s
   > PASS: go-e2e
   > REQUIRED_FIVE_SKIP_CALLS=0
   > ```
   > Evidence: [report.md#full-assistant-e2e-package-green](report.md#full-assistant-e2e-package-green)
- [x] TP-BUG069005-08 proves no list row changes before confirmation and the accepted persisted proposal changes it exactly once.
   > **Phase:** implement
   > **Command:** exact-five E2E plus live pending-state integration
   > **Exit Code:** 0 for both
   > **Claim Source:** executed
   > ```text
   > === RUN TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
   > === RUN TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
   > === RUN TestCompilerInteractiveControlsPersistByUserAndTransport
   > --- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > PASS
   > ok github.com/smackerel/smackerel/tests/integration/assistant 0.227s
   > PASS: go-integration
   > Network smackerel-test_default Removed
   > ```
   > Evidence: [report.md#exact-five-required-green](report.md#exact-five-required-green)
- [x] TP-BUG069005-09 proves confirm accept executes the gated action once and replay executes zero additional actions.
   > **Phase:** implement
   > **Command:** exact-five `./smackerel.sh test e2e --go-package assistant --go-run <anchored-selector>`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > go-e2e: applying package selector: assistant
   > go-e2e: applying -run selector: exact-five
   > === RUN TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.14s)
   > === RUN TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
   > === RUN TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > PASS: go-e2e
   > REQUIRED_FIVE_SKIP_CALLS=0
   > Container smackerel-test-postgres-1 Removed
   > ```
   > Evidence: [report.md#exact-five-required-green](report.md#exact-five-required-green)
- [x] Valid confirm-required writes return `ConfirmCard` with `capture_route=false`; capture fallback remains reserved for its policy-owned failure/abandonment cases.
   > **Phase:** implement
   > **Command:** exact-five E2E and installed regression-quality guard
   > **Exit Code:** 0 for both
   > **Claim Source:** executed
   > ```text
   > === RUN TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.14s)
   > === RUN TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
   > === RUN TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.372s
   > REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
   > Files scanned: 5
   > Files with adversarial signals: 5
   > REQUIRED_FIVE_SKIP_CALLS=0
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#build-quality-and-governance](report.md#build-quality-and-governance)
- [x] Change Boundary is respected and zero excluded file families are changed.
   > **Phase:** implement
   > **Command:** `git diff --check && git status --short`
   > **Exit Code:** 0
   > **Claim Source:** executed
   > ```text
   > M internal/assistant/context/store.go
   > M internal/assistant/facade.go
   > M tests/e2e/assistant/http_confirm_test.go
   > M tests/e2e/assistant/http_disambiguation_test.go
   > M tests/e2e/assistant/intent_side_effect_test.go
   > ?? cmd/core/wiring_assistant_actions.go
   > ?? internal/assistant/compiled_interactions.go
   > ?? tests/e2e/assistant/required_compiler_state_helpers_test.go
   > ?? tests/integration/assistant/bug069005_runtime_canary_test.go
   > M specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/report.md
   > M specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/scopes.md
   > git diff --check: exit 0
   > no parent-spec, framework-managed, deployment, secret, release-train, or knb path listed
   > ```
   > Evidence: [report.md#build-quality-and-governance](report.md#build-quality-and-governance)
- [x] Build Quality Gate passes for state-machine, persistence, and HTTP tests with zero warnings.
   > **Phase:** test
   > **Executed:** YES
   > **Claim Source:** executed
   > **Commands:** focused live compiler/persistence canaries; protected weather E2E; full assistant E2E package; static and packet guards
   > **Exit Codes:** all 0
   > ```text
   > TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler PASS
   > TestCompilerInteractiveControlsPersistByUserAndTransport PASS
   > TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce PASS
   > TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn PASS
   > TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 102.493s
   > PASS: go-e2e
   > RESULT: PASSED (0 warnings)
   > Violations: 0
   > Warnings: 0
   > ```
   > Evidence: [report.md#weather-implementation-and-test-closeout](report.md#weather-implementation-and-test-closeout)

All items require current-session command evidence before checking.

## Scope 3: Strict required E2E and product guard proof

**Status:** In Progress
**Priority:** P0
**Scope-Kind:** tests-and-governance
**Depends On:** Scopes 1, 2

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG069005-006 - Required tests fail closed instead of skipping
   Given the five manifest-required assistant HTTP tests are selected through the repo CLI
   When any required compiler, disambiguation, or confirmation behavior is absent
   Then the responsible test fails rather than calling a Go skip-family method
   And a healthy run reports five passes and zero skips
```

### Implementation Plan

1. Remove every behavior-dependent `t.Skip`, `t.Skipf`, and `t.SkipNow` from
   the exact five required tests; absent behavior becomes a fatal assertion.
2. Run one exact repo-CLI selector proving five passes and zero skips.
3. Run the broader assistant E2E package and regression scans.
4. Record the generic Go skip-family detection gap as a non-product route to
   `bubbles.implement` in the canonical Bubbles source repository; product
   certification does not wait for propagation when the exact five execute
   five of five with zero skips and the installed product regression guards
   pass.
5. Run packet artifact lint and traceability, then route certification to
   `bubbles.validate`.

### Allowed Files

- `tests/e2e/assistant/annotation_intent_test.go`
- `tests/e2e/assistant/http_confirm_test.go`
- `tests/e2e/assistant/http_disambiguation_test.go`
- `tests/e2e/assistant/intent_clarify_test.go`
- `tests/e2e/assistant/intent_side_effect_test.go`
- `tests/e2e/assistant/required_compiler_state_helpers_test.go`

### Excluded Files

- direct Smackerel `.github/bubbles/**` edits
- test skips, broad selector reductions, or manifest weakening
- fake-live direct facade tests presented as E2E
- parent Spec 069 artifacts before validate-owned reconciliation
- unrelated product tests or runtime code

### Foreign Framework Routing Observation

- **Finding:** The generic Bubbles regression-quality guard does not yet
   identify Go `t.Skip`, `t.Skipf`, and `t.SkipNow` as required-test bailouts.
- **Owner:** `bubbles.implement` in the canonical Bubbles source repository
   (`<bubbles-repo>`), targeting
   `bubbles/scripts/regression-quality-guard.sh` and
   `bubbles/scripts/regression-quality-guard-selftest.sh`.
- **Product boundary:** Smackerel's installed `.github/bubbles/**` remains
   unchanged. This foreign framework observation is not product implementation
   work and is not a product-certification prerequisite when TP-BUG069005-10
   reports five of five passes with zero skips and TP-BUG069005-11 plus the
   existing product regression guards pass.
- **Claim:** This packet records the route only; it makes no claim that the
   upstream Bubbles gap is fixed.
- **Foreign verification reference:** TP-BUG069005-12 remains owned by the
   canonical Bubbles workflow: upstream
   `bubbles/scripts/regression-quality-guard-selftest.sh` must prove that a Go
   `t.Skipf` fixture exits non-zero with `REQUIRED_TEST_SKIP` during upstream
   Bubbles framework validation. This reference is not a Smackerel Test Plan
   row or product DoD item.

### Test Plan

| Row | Scenario | Category | File/Location | Expected Test Title | Command | Live System |
|-----|----------|----------|---------------|---------------------|---------|-------------|
| TP-BUG069005-10 | SCN-BUG069005-006 | Regression E2E API | `tests/e2e/assistant/http_confirm_test.go` | exact five manifest-required test identities selected together | `./smackerel.sh test e2e --go-run '<anchored exact-five selector>'` | Yes |
| TP-BUG069005-11 | SCN-BUG069005-006 | guard | `tests/e2e/assistant/http_confirm_test.go` | zero required-test skip-family violations across all five files | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix <five files>` | No |
| TP-BUG069005-13 | SCN-BUG069005-006 | Regression E2E API | `tests/e2e/assistant/http_live_stack_test.go` | broader assistant protected-scenario regression | `./smackerel.sh test e2e --go-run '<assistant protected selector>'` | Yes |

### Definition of Done

- [x] TP-BUG069005-10 reports exactly five required tests passed, zero failed, zero skipped, and package exit 0 through the repo CLI.
   > **Phase:** test
   > **Executed:** YES (current session)
   > **Claim Source:** executed
   > **Command:** exact-five CPU-tier repo-CLI selector
   > **Exit Code:** 0
   > ```text
   > === RUN   TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.11s)
   > === RUN   TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.05s)
   > === RUN   TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.04s)
   > === RUN   TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > === RUN   TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.03s)
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 0.302s
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#exact-five-protected-spec-069-e2e](report.md#exact-five-protected-spec-069-e2e)
- [x] SCN-BUG069005-006 - Required tests fail closed instead of skipping: absent compiler, disambiguation, or confirmation behavior fails the responsible test, and a healthy exact-five run reports five passes and zero skips.
   > **Phase:** test
   > **Executed:** YES (current session)
   > **Claim Source:** executed
   > **Commands:** exact-five CPU-tier repo-CLI selector; installed regression guard in normal and `--bugfix` modes; explicit protected-file Go skip-family scan
   > **Exit Codes:** 0, 0, 0, and grep exit 1 (zero skip-family matches)
   > ```text
   > --- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.11s)
   > --- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.05s)
   > --- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.04s)
   > --- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
   > --- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.03s)
   > PASS: go-e2e
   > REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
   > Files scanned: 5
   > Files with adversarial signals: 5
   > PROTECTED_SKIP_FAMILY_SCAN_EXIT=1
   > PRIMARY_LIVE_MOCK_SCAN_EXIT=1
   > exact-five selected skips: 0
   > ```
   > Evidence: [report.md#exact-five-protected-spec-069-e2e](report.md#exact-five-protected-spec-069-e2e), [report.md#packet-guards-and-test-fidelity](report.md#packet-guards-and-test-fidelity)
- [x] Pre-fix semantic RED and post-fix GREEN for the same exact five tests are both recorded with claim-source provenance.
   > **Phase:** test
   > **Executed:** RED not rerun; GREEN executed in current session
   > **Claim Source:** accepted `bubbles.test` handoff for RED; executed for GREEN
   > **Command:** exact-five CPU-tier repo-CLI selector (GREEN only)
   > **Exit Code:** 0 (GREEN)
   > ```text
   > ACCEPTED RED (not-run in this session):
   > TestAnnotationIntentE2E_SlotsComeFromCompiledIntent SKIP
   > TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce SKIP
   > TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn SKIP
   > TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation SKIP
   > TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence SKIP
   > 5 executed; 0 required behavior passed; 5 skipped; package exit 0
   > CURRENT GREEN (executed):
   > TestAnnotationIntentE2E_SlotsComeFromCompiledIntent PASS
   > TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce PASS
   > TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn PASS
   > TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation PASS
   > TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence PASS
   > 5 passed; 0 failed; 0 skipped; package exit 0
   > ```
   > Evidence: [report.md#before-fix---accepted-bubblestest-handoff](report.md#before-fix---accepted-bubblestest-handoff), [report.md#exact-five-protected-spec-069-e2e](report.md#exact-five-protected-spec-069-e2e)
- [x] TP-BUG069005-11 rejects every Go skip-family bailout in the protected files and reports zero violations after repair.
   > **Phase:** test
   > **Executed:** YES
   > **Claim Source:** executed
   > **Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix <five protected files>`
   > **Exit Code:** 0
   > ```text
   > Adversarial signal detected in annotation_intent_test.go
   > Adversarial signal detected in http_confirm_test.go
   > Adversarial signal detected in http_disambiguation_test.go
   > Adversarial signal detected in intent_clarify_test.go
   > Adversarial signal detected in intent_side_effect_test.go
   > REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
   > Files scanned: 5
   > Files with adversarial signals: 5
   > ```
   > Evidence: [report.md#static-and-governance-evidence](report.md#static-and-governance-evidence)
- [x] TP-BUG069005-13 passes with no adjacent assistant regression and no required skip.
   > **Phase:** test
   > **Executed:** YES
   > **Claim Source:** executed
   > **Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant`
   > **Exit Code:** 0
   > ```text
   > TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation PASS
   > TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence PASS
   > TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce PASS
   > TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn PASS
   > PASS
   > ok github.com/smackerel/smackerel/tests/e2e/assistant 102.493s
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#full-assistant-package-evidence](report.md#full-assistant-package-evidence)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass.
   > **Phase:** test
   > **Executed:** YES
   > **Claim Source:** executed
   > **Commands:** protected Barcelona E2E, focused live canaries, and full assistant package
   > **Exit Codes:** all 0
   > ```text
   > TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler PASS
   > TestCompilerInteractiveControlsPersistByUserAndTransport PASS
   > TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation PASS
   > TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall PASS
   > TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation PASS
   > TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup PASS
   > PASS: go-integration
   > PASS: python-integration
   > PASS: go-e2e
   > ```
   > Evidence: [report.md#weather-implementation-and-test-closeout](report.md#weather-implementation-and-test-closeout)
- [ ] Broader E2E regression suite passes.
   > **Uncertainty Declaration**
   > **What was attempted:** the complete assistant E2E package passed.
   > **What remains:** canonical all-package E2E must run after this branch is merged with current `main` and the topic-momentum fix.
   > **Why it remains unchecked:** package-local evidence is not presented as repository-wide evidence.
- [x] Artifact lint and traceability pass for this bug packet.
   > **Phase:** test
   > **Executed:** YES (current session)
   > **Claim Source:** executed
   > **Commands:** packet artifact lint; packet traceability guard
   > **Exit Codes:** 0, 0
   > ```text
   > Required artifact exists: spec.md
   > Required artifact exists: design.md
   > Required artifact exists: scopes.md
   > Required artifact exists: report.md
   > All checked DoD items in scopes.md have evidence blocks
   > No repo-CLI bypass detected in report.md command evidence
   > Artifact lint PASSED.
   > scenario-manifest.json covers 6 scenario contract(s)
   > All linked tests from scenario-manifest.json exist
   > Scenarios checked: 6
   > Test rows checked: 15
   > Scenario-to-row mappings: 6
   > DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
   > RESULT: PASSED (0 warnings)
   > ```
   > Evidence: [report.md#packet-guards-and-test-fidelity](report.md#packet-guards-and-test-fidelity)
- [ ] `bubbles.validate` reconciles certification and any parent Spec 069 invalidation only after executable evidence is complete.

All items require current-session command evidence before checking.
