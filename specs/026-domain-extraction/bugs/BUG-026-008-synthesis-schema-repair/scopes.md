# Scopes: BUG-026-008 Bounded synthesis schema repair

## Scope 1: Repair one parsed schema-invalid synthesis response

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** backend contract bugfix

### Gherkin Scenarios

```gherkin
Feature: Bounded synthesis extraction schema repair

  Scenario: Missing required concepts is corrected once (BUG-026-008-SCN-001)
    Given the committed ingest synthesis schema requires concepts
    And the first LLM response is parsed JSON without concepts
    And the second response is valid against the same schema
    When handle_extract processes the artifact
    Then it returns success true
    And exactly two LLM calls occur
    And token usage is the sum of both calls

  Scenario: A second schema-invalid response is terminal (BUG-026-008-SCN-002)
    Given the first and repair responses both violate the extraction schema
    When handle_extract processes the artifact
    Then it returns success false with the final schema error
    And exactly two LLM calls occur

  Scenario: Malformed repair JSON is terminal (BUG-026-008-SCN-003)
    Given the first response is parsed but schema-invalid
    And the repair response is malformed JSON
    When handle_extract processes the artifact
    Then it returns success false with the repair JSON error class
    And no third call occurs

  Scenario: Repair LLM exception is terminal and content-free (BUG-026-008-SCN-004)
    Given the first response is parsed but schema-invalid
    And the repair LLM call raises an exception containing a sensitive marker
    When handle_extract processes the artifact
    Then it returns success false with the exception type only
    And neither logs nor result contain the marker or artifact content

  Scenario: Initially valid output remains a one-call path (BUG-026-008-SCN-005)
    Given the first response validates against the committed schema
    When handle_extract processes the artifact
    Then it returns success true after exactly one LLM call

  Scenario: Repair retains the structured extraction request profile (BUG-026-008-SCN-006)
    Given Ollama thinking, keep-alive, context window, temperature, response format, and token budget are configured
    And schema repair is required
    When both requests are captured at the external LLM boundary
    Then both carry the same configured profile and original artifact context
    And the trace ID is preserved only on the result contract, not model messages or logs

  Scenario: Required semantic content is never normalized (BUG-026-008-SCN-007)
    Given required concepts or claims are missing
    When handle_extract evaluates the parsed response
    Then it sends the corrective request rather than inserting empty semantic defaults
```

### Implementation Plan

1. Add the focused missing-`concepts` regression against the real `handle_extract` and committed prompt contract; execute it red before runtime changes.
2. Add the fail-loud SST retry-attempt key and generated/runtime validation contract.
3. Implement one profile-preserving corrective dispatch and accumulated accounting.
4. Add all terminal/adversarial cases and content-free observability assertions.
5. Run focused green, full Python unit, broader impacted suites, lint, format, and governance guards.
6. Route validate-owned certification, then commit and push without hook bypass.

### Implementation Files

- `ml/app/synthesis.py`
- `ml/app/main.py`
- `ml/app/metrics.py`
- `ml/tests/conftest.py`
- `ml/tests/test_synthesis.py`
- `ml/tests/test_main.py`
- `ml/tests/test_ollama_keepalive.py`
- `ml/tests/fixtures/card_rewards_missing_concepts.json`
- `internal/pipeline/synthesis_types.go`
- `internal/pipeline/synthesis_subscriber_test.go`
- `config/smackerel.yaml`
- `scripts/commands/config.sh`
- `docs/Development.md`

### Change Boundary

**Allowed surfaces:** the implementation files listed above and this BUG-026-008 packet.

**Excluded surfaces:** core synthesis status/acknowledgement logic, prompt-schema required fields,
BUG-026-006 malformed-JSON capture implementation, BUG-026-007 thinking/profile helpers,
deployment adapters/manifests, release-train bundles, secrets, and host configuration.

### Test Plan

| Test Type | ID | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|---|
| Pre-fix focused regression | `TP-BUG026008-000` | `unit` | `ml/tests/test_synthesis.py`, `ml/tests/fixtures/card_rewards_missing_concepts.json` | `test_handle_extract_repairs_missing_concepts_once` proves the original one-call permanent failure before the repair | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Missing required concepts is corrected once (BUG-026-008-SCN-001) | `TP-BUG026008-001` | `unit` | `ml/tests/test_synthesis.py`, `ml/tests/fixtures/card_rewards_missing_concepts.json` | `test_handle_extract_repairs_missing_concepts_once` drives actual `handle_extract` from missing concepts to one valid correction | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| A second schema-invalid response is terminal (BUG-026-008-SCN-002) | `TP-BUG026008-002` | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_fails_when_schema_repair_remains_invalid` proves two calls, final-validator classification, and no model-value leak | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Malformed repair JSON is terminal (BUG-026-008-SCN-003) | `TP-BUG026008-003` | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_fails_when_schema_repair_is_malformed_json` proves decode-class failure after two calls | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Repair LLM exception is terminal and content-free (BUG-026-008-SCN-004) | `TP-BUG026008-004` | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_schema_repair_exception_is_content_free` proves type-only error and content-free logs/result | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Initially valid output remains a one-call path (BUG-026-008-SCN-005) | `TP-BUG026008-005` | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_valid_first_response_remains_one_call` proves no unnecessary correction | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Repair retains the structured extraction request profile (BUG-026-008-SCN-006) | `TP-BUG026008-006A` | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_schema_repair_retains_profile_and_sums_tokens` proves profile/context/accounting retention at the external LLM boundary | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Go response contract for BUG-026-008-SCN-006 | `TP-BUG026008-006B` | `unit` | `internal/pipeline/synthesis_subscriber_test.go` | Focused Go response tests prove full-pipeline trace and payload retention | `./smackerel.sh test unit --go --go-run 'TestSynthesisExtractResponse_(FullPipelinePayload|FailureMarksFailed|SuccessMarksCompleted)' --verbose` | No |
| Required semantic content is never normalized (BUG-026-008-SCN-007) | `TP-BUG026008-007` | `unit` | `ml/tests/test_synthesis.py` | The missing-concepts test asserts a corrective request carrying the schema and anti-fabrication instruction instead of an empty required field | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Startup/config unit contract | `TP-BUG026008-008A` | `unit` | `ml/tests/test_main.py`, `ml/tests/test_synthesis.py` | Missing/invalid retry budget fails loud at startup and request time | `./smackerel.sh test unit --python` | No |
| Generated config/SST contract | `TP-BUG026008-008B` | `guard` | `config/smackerel.yaml`, `scripts/commands/config.sh` | Generated ML env remains in SST sync | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` | No |
| Regression E2E | `TP-BUG026008-009` | `e2e-api` | `tests/e2e/knowledge_synthesis_test.go` | `TestKnowledgeSynthesis_PipelineRoundTrip` exercises the scenario-level capture/synthesis workflow through the live disposable test stack | `./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip` | Yes; ephemeral stack |
| Broader E2E regression | `TP-BUG026008-010` | `e2e-api` | `tests/e2e/` | Existing capture, core status, and acknowledgement workflows remain green | `./smackerel.sh test e2e` | Yes; ephemeral stack |
| Impacted unit suite | `TP-BUG026008-011` | `unit` | `ml/tests/` | Full Python ML suite including malformed-JSON and qwen3 profile regressions | `./smackerel.sh test unit --python` | No |
| Lint | `TP-BUG026008-012A` | `lint` | Changed runtime/config/tests | Lint reports no warnings | `./smackerel.sh lint` | No |
| Format | `TP-BUG026008-012B` | `lint` | Changed runtime/config/tests | Format check reports no drift | `./smackerel.sh format --check` | No |
| Artifact lint | `TP-BUG026008-013A` | `artifact` | BUG-026-008 packet | Artifact structure and evidence anchors remain valid | `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair` | No |
| Traceability | `TP-BUG026008-013B` | `artifact` | BUG-026-008 packet | Gherkin, scenario manifest, tests, and DoD remain linked | `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair` | No |
| Implementation reality | `TP-BUG026008-013C` | `artifact` | BUG-026-008 packet | Referenced implementation files contain no stub/fake/default regressions | `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair --verbose` | No |
| Adversarial regression guard | `TP-BUG026008-013D` | `artifact` | `ml/tests/test_synthesis.py`, `ml/tests/test_main.py`, `ml/tests/test_ollama_keepalive.py` | Bugfix regressions contain adversarial signals and no bailout patterns | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_synthesis.py ml/tests/test_main.py ml/tests/test_ollama_keepalive.py` | No |
| State-transition guard | `TP-BUG026008-013E` | `artifact` | BUG-026-008 packet | Records exact remaining owner-routed findings | `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair` | No |

### Definition of Done

- [ ] Root cause and outcome contract are confirmed against the actual `handle_extract` branch.
- [x] `TP-BUG026008-000` - Pre-fix focused regression fails because missing `concepts` returns permanent failure after one call. Evidence: [report.md#red-bug-reproduction-before-fix](report.md#red-bug-reproduction-before-fix)
- [ ] `TP-BUG026008-008A` - Fail-loud startup/config unit contract permits exactly one schema-repair attempt.
- [ ] `TP-BUG026008-008B` - Generated ML environment remains in SST sync.
- [x] `TP-BUG026008-001` - Missing required concepts is corrected once: invalid-then-valid succeeds after exactly two calls and summed token usage. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-002` - A second schema-invalid response is terminal: exactly two calls return the second response's content-free validator/path class. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-003` - Malformed repair JSON is terminal: exactly two calls return the repair decode class and no model output. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-004` - Repair LLM exception is terminal and content-free: exactly two calls return the exception type without message, artifact, or trace leakage. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-005` - Initially valid output remains a one-call path with no corrective request. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-006A` - Repair retains the structured extraction request profile, original context, total token usage, and total processing duration. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-006B` - Focused Go response contract preserves the full-pipeline trace and payload. Evidence: [report.md#focused-go-response-contract](report.md#focused-go-response-contract)
- [x] `TP-BUG026008-007` - Required semantic content is never normalized: missing required concepts/claims trigger correction instead of fabricated empty defaults. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG026008-009`: `TestKnowledgeSynthesis_PipelineRoundTrip` through the live disposable stack).
- [ ] Broader E2E regression suite passes (`TP-BUG026008-010`).
- [x] `TP-BUG026008-011` - Full impacted Python suite passes, including BUG-026-006 malformed-JSON and BUG-026-007 thinking/token-profile regressions. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] `TP-BUG026008-012A` - Lint passes with no warnings in changed surfaces. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] `TP-BUG026008-012B` - Format validation passes for the active diff. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] `TP-BUG026008-013A` - Artifact lint passes. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] `TP-BUG026008-013B` - Traceability guard passes. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] `TP-BUG026008-013C` - Implementation-reality scan passes. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] `TP-BUG026008-013D` - Adversarial regression guards pass. Evidence: [report.md#adversarial-regression-guards](report.md#adversarial-regression-guards)
- [ ] `TP-BUG026008-013E` - State-transition guard records the exact remaining owner-routed findings.
- [ ] Security and audit review find no content/exception secret leakage, unbounded retry, config fallback, or change-boundary violation.
- [ ] Validate-owned certification records the strongest status supported by executed evidence.
