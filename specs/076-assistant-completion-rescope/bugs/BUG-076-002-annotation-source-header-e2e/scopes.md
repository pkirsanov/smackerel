# Scopes: BUG-076-002 Annotation source header E2E

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Supply the canonical annotation source prerequisite

**Status:** Done
**Depends On:** none
**Owner:** `bubbles.test`

### Gherkin Scenarios

```gherkin
Feature: Annotation shadow E2E provenance
  Scenario: API annotation reaches the shadow comparator
    Given the generated test config names the annotation source header
    When the E2E posts an annotation with source api
    Then the API returns 201
    And the api shadow-comparator counter advances

  Scenario: Missing generated header name fails loudly
    Given the header-name config is absent
    When the E2E prepares the request
    Then the test fails before claiming comparator coverage
```

### Implementation Plan

1. Read the generated header name and reject empty config.
2. Set the explicit `api` source on the live annotation request.
3. Run focused and full assistant E2E plus guards.

### Change Boundary

Allowed: `tests/e2e/assistant/annotation_classifier_e2e_test.go` and this packet.

### Implementation Files

- `tests/e2e/assistant/annotation_classifier_e2e_test.go`

### Test Plan

| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Annotation source regression | `e2e-api` | `tests/e2e/assistant/annotation_classifier_e2e_test.go` | Real annotation and counter delta | `./smackerel.sh test e2e --go-package assistant --go-run '^TestAnnotationClassifier'` | Yes |
| Missing generated header name fails loudly | `e2e-api` | `tests/e2e/assistant/annotation_classifier_e2e_test.go` | The test fatals before request construction if the generated header name is absent; removing the request header produces the API's real HTTP 400 | `./smackerel.sh test e2e --go-package assistant --go-run '^TestAnnotationClassifier'` | Yes |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Full package | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Neighboring assistant flows | `./smackerel.sh test e2e --go-package assistant` | Yes |

### Definition of Done

- [x] Root cause confirmed with current RED. → Evidence: [report.md](report.md) "Bug Reproduction — RED (revert-reverify, current session)" — with the load-bearing `annReq.Header.Set(sourceHeader, "api")` line omitted, the focused live e2e FAILs with the exact `POST annotation status = 400, body={"error":"X-Smackerel-Source header required"}` (`--- FAIL`, `RED_E2E_EXIT=1`), reproducing the reported defect on the live API before the fix is restored byte-exact.
- [x] Generated header name is required without fallback. → Evidence: [report.md](report.md) "### Code Diff Evidence" — the fix reads `os.Getenv("ANNOTATIONS_SOURCE_HEADER_NAME")` and `t.Fatal`s when empty (no fallback / hardcoded alternate); the value is generated from `config/smackerel.yaml` `annotations.source_header_name` via `scripts/commands/config.sh` `required_value` (fail-loud) into `config/generated/test.env` (`ANNOTATIONS_SOURCE_HEADER_NAME=X-Smackerel-Source`).
- [x] API annotation reaches the shadow comparator: live annotation returns 201 and the `api` counter advances. → Evidence: [report.md](report.md) "Test Evidence — isolated GREEN" (`--- PASS: TestAnnotationClassifierWithShadowComparator`, `PASS: go-e2e`, exit 0) and "Broader E2E regression" (same test PASS at 13.67s cold-facade) — the test asserts HTTP 201 on the annotation POST and requires `smackerel_annotation_classifier_shadow_calls_total{channel="api"}` to advance, so a PASS proves both.
- [x] Missing prerequisite cannot silently pass. → Evidence: [report.md](report.md) "### Code Diff Evidence" (fail-loud `t.Fatal` when the generated header name is absent — SCN-002) + "Guards & Quality Gates" targeted API unit adversaries `TestCreateAnnotation_MissingSourceHeader_400` / `TestCreateAnnotation_UnknownSourceHeader_400` GREEN (`UNIT_EXIT=0`) + the "Bug Reproduction — RED" empirical proof that omitting the header fails on the live API (400). Adversarial coverage is placed in the owning API suite per [design.md](design.md) and passes RQG `--bugfix` (`RQG_BUGFIX_API_EXIT=0`).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [report.md](report.md) "Test Evidence" — SCN-001 (api-sourced annotation reaches the shadow comparator: 201 + `channel="api"` counter advance) is encoded in `TestAnnotationClassifierWithShadowComparator` and PASSes live; SCN-002 (missing generated header name fails loudly) is encoded as the fail-loud `t.Fatal` prerequisite in the same test and covered adversarially by the owning API suite. [scenario-manifest.json](scenario-manifest.json) maps both scenarios to the concrete test.
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "Broader E2E regression — full assistant package" — 32 PASS / 13 SKIP; the target test + every neighboring assistant flow GREEN. The only 2 failures are the pre-existing, unrelated `buildvcs` (`error obtaining VCS status: exit 128`) environment failures in `intent_replay_test.go` — a different subsystem, outside this change boundary, not caused by this change (working tree packet-only), owned by concurrent spec069/intent-replay work (G051 class; DI-076-002-01).
- [x] Change boundary and quality guards pass. → Evidence: [report.md](report.md) "### Code Diff Evidence" (the fix is `8ac848e1`, `+6/−0` in the single allowed file `tests/e2e/assistant/annotation_classifier_e2e_test.go`; working tree packet-only) + "Guards & Quality Gates" — `CHECK_EXIT=0`, `FORMAT_EXIT=0`, `LINT_EXIT=0`, `UNIT_EXIT=0`, `artifact-lint` exit 0, `traceability-guard` PASSED, `implementation-reality-scan` PASSED (`REALITY_EXIT=0`), `regression-quality` standard exit 0; `state-transition-guard` verdict PASS (`failedGateIds []`).
- [x] Validate-owned certification remains authoritative. → Evidence: [state.json](state.json) `certification.certifierAgent = bubbles.validate`, `certification.certificationReadiness = ready`; terminal `certification.status = done` + `certifiedAt` are stamped only by the validate-owned promote commit after the planning-truth commit (G088), and `execution.executionHistory` records the validate phase.
