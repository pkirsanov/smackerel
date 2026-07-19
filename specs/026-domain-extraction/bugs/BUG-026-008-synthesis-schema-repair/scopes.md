# Scopes: BUG-026-008 Bounded synthesis schema repair

## Scope 1: Repair one parsed schema-invalid synthesis response

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** backend contract bugfix

### Gherkin Scenarios

```gherkin
Feature: Bounded synthesis extraction schema repair

  Scenario: Missing required concepts is corrected once
    Given the committed ingest synthesis schema requires concepts
    And the first LLM response is parsed JSON without concepts
    And the second response is valid against the same schema
    When handle_extract processes the artifact
    Then it returns success true
    And exactly two LLM calls occur
    And token usage is the sum of both calls

  Scenario: A second schema-invalid response is terminal
    Given the first and repair responses both violate the extraction schema
    When handle_extract processes the artifact
    Then it returns success false with the final schema error
    And exactly two LLM calls occur

  Scenario: Malformed repair JSON is terminal
    Given the first response is parsed but schema-invalid
    And the repair response is malformed JSON
    When handle_extract processes the artifact
    Then it returns success false with the repair JSON error class
    And no third call occurs

  Scenario: Repair LLM exception is terminal and content-free
    Given the first response is parsed but schema-invalid
    And the repair LLM call raises an exception containing a sensitive marker
    When handle_extract processes the artifact
    Then it returns success false with the exception type only
    And neither logs nor result contain the marker or artifact content

  Scenario: Initially valid output remains a one-call path
    Given the first response validates against the committed schema
    When handle_extract processes the artifact
    Then it returns success true after exactly one LLM call

  Scenario: Repair retains the structured extraction request profile
    Given Ollama thinking, keep-alive, context window, temperature, response format, and token budget are configured
    And schema repair is required
    When both requests are captured at the external LLM boundary
    Then both carry the same configured profile and original artifact context
    And the trace ID is preserved only on the result contract, not model messages or logs

  Scenario: Required semantic content is never normalized
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

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Missing required concepts is corrected once | `unit` | `ml/tests/test_synthesis.py`, `ml/tests/fixtures/card_rewards_missing_concepts.json` | `test_handle_extract_repairs_missing_concepts_once` drives actual `handle_extract` from missing concepts to one valid correction | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| A second schema-invalid response is terminal | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_fails_when_schema_repair_remains_invalid` proves two calls, final-validator classification, and no model-value leak | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Malformed repair JSON is terminal | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_fails_when_schema_repair_is_malformed_json` proves decode-class failure after two calls | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Repair LLM exception is terminal and content-free | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_schema_repair_exception_is_content_free` proves type-only error and content-free logs/result | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Initially valid output remains a one-call path | `unit` | `ml/tests/test_synthesis.py` | `test_handle_extract_valid_first_response_remains_one_call` proves no unnecessary correction | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Repair retains the structured extraction request profile | `unit` | `ml/tests/test_synthesis.py`, `internal/pipeline/synthesis_subscriber_test.go` | `test_handle_extract_schema_repair_retains_profile_and_sums_tokens` plus `TestSynthesisExtractResponse_FullPipelinePayload` prove profile/context/trace/accounting retention | `./smackerel.sh test unit --python` and `./smackerel.sh test unit --go --go-run 'TestSynthesisExtractResponse_(FullPipelinePayload|FailureMarksFailed|SuccessMarksCompleted)' --verbose` | No; external LLM boundary controlled |
| Required semantic content is never normalized | `unit` | `ml/tests/test_synthesis.py` | The missing-concepts test asserts a corrective request carrying the schema and anti-fabrication instruction instead of an empty required field | `./smackerel.sh test unit --python` | No; external LLM boundary controlled |
| Startup/config contract | `unit` | `ml/tests/test_main.py`, `ml/tests/test_synthesis.py` | Missing/invalid retry budget fails loud at startup and request time; generated env remains in SST sync | `./smackerel.sh test unit --python` and `./smackerel.sh check` | No |
| Regression E2E | `e2e-api` | Existing synthesis/capture E2E lane | Scenario-level capture/synthesis workflow remains explicit through the live disposable test stack | `./smackerel.sh test e2e` | Yes; ephemeral stack |
| Broader E2E regression | `e2e-api` | Repository E2E suite | Existing capture, core status, and acknowledgement workflows remain green | `./smackerel.sh test e2e` | Yes; ephemeral stack |
| Impacted unit suite | `unit` | `ml/tests/` | Full Python ML suite including malformed-JSON and qwen3 profile regressions | `./smackerel.sh test unit --python` | No |
| Static quality | `lint` | Changed runtime/config/tests | Lint and format checks report no warnings | `./smackerel.sh lint` and `./smackerel.sh format --check` | No |
| Governance | `artifact` | Bug packet and changed implementation | Artifact, traceability, adversarial regression, and implementation-reality gates | Bubbles guard scripts through committed command surfaces | No |

### Definition of Done

- [ ] Root cause and outcome contract are confirmed against the actual `handle_extract` branch.
- [x] Pre-fix focused regression fails because missing `concepts` returns permanent failure after one call. Evidence: [report.md#red-bug-reproduction-before-fix](report.md#red-bug-reproduction-before-fix)
- [ ] Fail-loud SST and generated ML env contract permit exactly one schema-repair attempt.
- [x] Missing required concepts is corrected once: invalid-then-valid succeeds after exactly two calls and summed token usage. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] A second schema-invalid response is terminal: exactly two calls return the second response's content-free validator/path class. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Malformed repair JSON is terminal: exactly two calls return the repair decode class and no model output. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Repair LLM exception is terminal and content-free: exactly two calls return the exception type without message, artifact, or trace leakage. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Initially valid output remains a one-call path with no corrective request. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Repair retains the structured extraction request profile, original context, trace result contract, total token usage, and total processing duration. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Required semantic content is never normalized: missing required concepts/claims trigger correction instead of fabricated empty defaults. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [ ] Change Boundary is respected: every changed file is in the allowed list and zero excluded surfaces changed.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [x] Full impacted Python suite passes, including BUG-026-006 malformed-JSON and BUG-026-007 thinking/token-profile regressions. Evidence: [report.md#harness-and-category-repairs](report.md#harness-and-category-repairs)
- [x] Regression tests contain no silent-pass bailout or tautological bugfix patterns. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] Check, lint, and format validation pass with no warnings in changed surfaces. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [x] Artifact lint, traceability guard, implementation-reality scan, and state-transition checks pass at the permitted status. Evidence: [report.md#final-cheap-closeout-checks](report.md#final-cheap-closeout-checks)
- [ ] Security and audit review find no content/exception secret leakage, unbounded retry, config fallback, or change-boundary violation.
- [ ] Validate-owned certification records the strongest status supported by executed evidence.
