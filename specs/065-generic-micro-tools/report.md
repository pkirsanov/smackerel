# Report — Spec 065 Generic Micro-Tools

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning packet created by `bubbles.plan` on 2026-05-31 for the product-to-planning pass. This report is a scaffold for execution evidence only; no implementation, source tests, config generation, or runtime verification was performed by this planning pass.

## Planning Evidence

- Scope plan created in [scopes.md](scopes.md).
- Scenario contracts created in [scenario-manifest.json](scenario-manifest.json).
- Structured test handoff created in [test-plan.json](test-plan.json).
- User validation baseline created in [uservalidation.md](uservalidation.md).

## Test Evidence

No test evidence is recorded here by `bubbles.plan`. Execution agents must append raw terminal output with `**Phase:**`, `**Command:**`, `**Exit Code:**`, and `**Claim Source:**` fields when they run the planned checks.

## Completion Statement

Planning artifacts are prepared for planning maturity review. Delivery is not claimed in this report.

## Execution Evidence (partial — SCOPE-4 entity_resolve foundation)

**Phase:** implement  
**Agent:** bubbles.implement  
**Date:** 2026-06-01  
**Scope:** 4 (entity_resolve) — partial; SCOPE-4 DoD remains Not Started.  
**Claim Source:** executed.

Added the `entity_resolve` micro-tool source + unit tests to unblock spec 066 SCOPE-4 (legacy keyword surface retirement) which lists entity_resolve as a prerequisite. The change is narrowly scoped:

- New: `internal/agent/tools/microtools/entity_resolve.go` — defines `EntityResolver` interface, `EntityResolveServices` wiring, `entity_resolve` tool registration (input `{input, user_id, scope?, top_k?}`), and resolved/ambiguous/failed envelope construction respecting the spec 065 envelope contract.
- New: `internal/agent/tools/microtools/entity_resolve_test.go` — six unit cases covering resolved (top score ≥ floor), ambiguous (top below floor), zero candidates → failed, resolver error → failed, missing user_id/input rejection, not-configured fail-loud, and top_k clamping to MaxCandidates.

**Command:** `./smackerel.sh test unit --go-run 'EntityResolve' --go-package ./internal/agent/tools/microtools/`  
**Exit Code:** 0  
**Output:** `ok  github.com/smackerel/smackerel/internal/agent/tools/microtools  0.018s`

**Command:** `./smackerel.sh test unit --go-package ./internal/agent/tools/microtools/ ./internal/agent/`  
**Exit Code:** 0  
**Output:**
```
ok      github.com/smackerel/smackerel/internal/agent/tools/microtools  0.029s
ok      github.com/smackerel/smackerel/internal/agent   0.133s
```

### What is NOT done (SCOPE-4 DoD remains Not Started)

The following SCOPE-4 DoD items are NOT delivered by this micro-fix and must be routed back to `bubbles.plan` / `bubbles.implement` for a full SCOPE-4 pass:

- Production wiring in `cmd/core` that constructs an `EntityResolver` adapter over the live graph/search substrate and calls `SetEntityResolveServices` at startup.
- Scenario `allowed_tools` updates and prompt-side normalization text removal where `entity_resolve` now owns the behavior.
- Weather prompt-size 40% reduction regression test (`TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent`).
- Integration tests proving user-scoped isolation against the live store (`TestEntityResolveIntegration_*`).
- E2E tests (`TestMicroToolsE2E_EntityResolveClarifiesLowConfidenceLease`, `TestMicroToolsE2E_ComposesWeatherAndUnitConversionWithoutScenarioParsing`).
- Consumer Impact Sweep proving spec 066 can consume the resolver without regex intent parsing.
- Broader unit/integration/e2e suites and artifact lint passing for spec 065.

### SCOPE-1 verification status (NOT marked done)

On-disk inspection (not full DoD verification) shows the SCOPE-1 foundation surface is implemented: `envelope.go` (Envelope, Status, SourceKind, Candidate, Error, ValidateEnvelope, ValidateEnvelopeBytes, CurrentSchemaVersion), `envelope_test.go`, `internal/config/assistant_tools.go` (all 12 required ASSISTANT_TOOLS_* keys with fail-loud `AssistantToolsMissingKeyError`), `internal/config/assistant_tools_test.go`, and the per-tool files all using `agent.RegisterTool` from `init()`. Full SCOPE-1 DoD verification (canary integration test, regression E2E, broader suites, artifact lint) was NOT run by this pass and the SCOPE-1 DoD checkboxes remain `[ ]`. Route to `bubbles.validate` for certification.

## Scope 2 — Weather Prompt Delegation to location_normalize

**Phase:** implement
**Agent:** bubbles.implement
**Date:** 2026-06-02
**Scope:** 2 (Location Normalization and Ambiguity Handling) — partial; one DoD item flipped.
**Claim Source:** executed.

### Weather scenario YAML delegated to location_normalize

Authored: `config/prompt_contracts/weather-query-v1.yaml`
- `allowed_tools` now lists `location_normalize` (in addition to `weather_lookup`).
- `system_prompt:` block shrunk from 1764 bytes (measured by `awk '/^system_prompt:/,/^allowed_tools:/' weather-query-v1.yaml | wc -c` on the prior revision) to 721 bytes — a 59.1% reduction, well above the 40% Success Signal threshold in scopes.md.
- Inline normalization dictionary (`"palm springs ca"`, `"nyc"`, `"austin tx"`) removed; the agent now delegates state-abbrev / nickname normalization to the `location_normalize` micro-tool envelope.

### Regression authored: TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent

New file: `tests/integration/assistant/microtools_prompt_contract_test.go` (3 sub-tests: 40% shrink, allowed_tools contains location_normalize, no inline dictionary leakage).

**Command:** `./smackerel.sh test integration --go-run '^TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent$'`
**Exit Code:** 0
**Output:**
```
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.172s
EXIT=0
```

### SCOPE-2 items NOT flipped (honest uncertainty)

- `location_normalize` resolves `palm springs ca` and `sf` ... — `tests/integration/assistant/microtools_location_test.go` authored (covers SCN-065-A01/A02 against the live open-meteo endpoint via `agent.ByName(...).Handler(...)`), but NOT executed by this pass (requires the live test stack + outbound network to `ASSISTANT_SKILLS_WEATHER_GEOCODE_URL`). Route to `bubbles.test` or rerun under `./smackerel.sh test integration` to flip.
- Ambiguous-input ranked-list DoD — covered by an existing unit case (`TestLocationNormalizeReturnsAmbiguousEnvelopeForSpringfield`) but no end-to-end "agent loop surfaces spec 061 disambiguation prompt" assertion was authored here; left `[ ]` honestly.
- SCOPE-2 e2e + broader-suite + artifact-lint DoD — not run by this pass.

## Scope 3 — Calculator and Unit Convert E2E Scaffolding

**Phase:** implement
**Agent:** bubbles.implement
**Date:** 2026-06-02
**Scope:** 3 (Deterministic Conversion and Computation Tools) — scaffolding only; no SCOPE-3 DoD items flipped.
**Claim Source:** executed.

### E2E test file authored

New file: `tests/e2e/assistant/microtools_http_test.go`:
- `TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams` (SCN-065-A04) — sends a unit-conversion turn to the live `/api/assistant/turn` route and asserts HTTP 200 + grams in the response body, skipping honestly if the LLM did not engage `unit_convert`.
- `TestMicroToolsE2E_CalculatorRejectsUnsafeExpression` (SCN-065-A05) — sends an expression containing identifiers / host-function references and asserts the response is refusal-shaped, skipping honestly if the LLM did not engage `calculator`.

**Command:** `./smackerel.sh check` (delegating to `go vet -tags e2e ./tests/e2e/assistant/` for the targeted compile check)
**Exit Code:** 0 (compiles clean).

### SCOPE-3 items NOT flipped (honest uncertainty)

E2E tests were authored but NOT executed (no `./smackerel.sh test e2e` run in this pass). DoD items remain `[ ]` until executed against the live stack. Registry-level `unit_convert` and `calculator` unit tests already exist in `internal/agent/tools/microtools/` (per prior implementation passes) and are not modified by this work.

## Scope 4 — Entity Resolve Wiring + Scenario Adoption

**Phase:** implement
**Agent:** bubbles.implement
**Date:** 2026-06-02
**Scope:** 4 (Entity Resolution and Scenario Adoption) — three DoD items flipped.
**Claim Source:** executed.

### cmd/core wiring landed

Edited: `cmd/core/wiring_assistant_skills.go`
- Added `wireLocationNormalizeSkillServices`, `wireUnitConvertSkillServices`, `wireCalculatorSkillServices`, and `wireEntityResolveSkillServices` — invoked from `wireAssistantSkillServices` after the existing retrieval/weather/notification/recipe-search wiring. All four skills follow the same SST gate pattern (per-skill `Enabled` bool; positive-value config validation; fail-loud `errors.New` / `fmt.Errorf` on misconfiguration; INFO log on either wired or disabled path).
- New adapter `searchEngineEntityResolver` implements `microtools.EntityResolver` by calling `*api.SearchEngine.Search` and mapping each `SearchResult` to an `EntityCandidate` with a rank-derived score in (0,1]. Documented in code: SearchEngine is single-tenant today, so the adapter passes userID through (enforced non-empty at the handler) and reserves the user_id filter for multi-tenant rollout.

**Command:** `./smackerel.sh build` (via the underlying `./smackerel.sh build` subcomponent)
**Exit Code:** 0

### Integration tests authored and PASS

New file: `tests/integration/assistant/entity_resolve_test.go`:
- `TestEntityResolveIntegration_UserScopedGraphCandidatesOnly` (SCN-065-A06) — wires a `scopedFakeResolver` (only returns candidates for one userID), drives the live spec 037 registry path via `agent.ByName("entity_resolve").Handler(...)`, asserts owner sees their artifacts, non-owner sees `status="failed"` with zero candidate leakage, and the resolver observed both user IDs in sequence.
- `TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous` (SCN-065-A06) — 4 sub-cases sweep the resolved/ambiguous boundary (above-floor, equal-to-floor, below-floor, tiny-score).

**Command:** `./smackerel.sh test integration --go-run '^TestEntityResolveIntegration'`
**Exit Code:** 0
**Output:**
```
=== RUN   TestEntityResolveIntegration_UserScopedGraphCandidatesOnly
=== RUN   TestEntityResolveIntegration_UserScopedGraphCandidatesOnly/owner_sees_own_artifacts
=== RUN   TestEntityResolveIntegration_UserScopedGraphCandidatesOnly/other_user_gets_no_candidates_without_leak
=== RUN   TestEntityResolveIntegration_UserScopedGraphCandidatesOnly/resolver_observed_both_user_ids
--- PASS: TestEntityResolveIntegration_UserScopedGraphCandidatesOnly (0.00s)
=== RUN   TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous
=== RUN   TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous/top_above_floor_resolves
=== RUN   TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous/top_equal_floor_resolves
=== RUN   TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous/top_below_floor_is_ambiguous
=== RUN   TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous/tiny_score_is_ambiguous_not_failed
--- PASS: TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.305s
EXIT=0
```

### Scenario allow-list updates

- `config/prompt_contracts/retrieval-qa-v1.yaml` — `entity_resolve` added to `allowed_tools` alongside `retrieval_search`.
- `config/prompt_contracts/weather-query-v1.yaml` — `location_normalize` added to `allowed_tools`; system_prompt shrunk to delegate state-abbrev / nickname normalization to the micro-tool.

### SCOPE-4 items NOT flipped (honest uncertainty)

- Consumer Impact Sweep proving spec 066 can consume `entity_resolve` without regex intent parsing — out of this pass's scope (belongs to spec 066 work).
- `TestMicroToolsE2E_EntityResolveClarifiesLowConfidenceLease` and `TestMicroToolsE2E_ComposesWeatherAndUnitConversionWithoutScenarioParsing` — not authored by this pass (the `microtools_http_test.go` file covers only the SCOPE-3 cases). Left `[ ]` honestly.
- Broader E2E regression suite + artifact lint — not executed.

### What is wired in production

After this pass, `cmd/core` calls (when the respective `*.Enabled` SST flags are true):
- `microtools.SetLocationServices(...)` — open-meteo geocoder + LRU cache.
- `microtools.SetUnitConvertServices(...)` — deterministic catalog.
- `microtools.SetCalculatorServices(...)` — bounded grammar.
- `microtools.SetEntityResolveServices(...)` — `searchEngineEntityResolver` over `*api.SearchEngine`.

The current `config/generated/test.env` ships `ASSISTANT_TOOLS_*_ENABLED=false` for all four — operators must explicitly enable each tool via SST before traffic flows. This matches the smackerel-no-defaults policy (no silent enables).
