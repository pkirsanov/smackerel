# Scopes: BUG-076-002 Annotation source header E2E

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Supply the canonical annotation source prerequisite

**Status:** In Progress
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

- [ ] Root cause confirmed with current RED.
- [ ] Generated header name is required without fallback.
- [ ] API annotation reaches the shadow comparator: live annotation returns 201 and the `api` counter advances.
- [ ] Missing prerequisite cannot silently pass.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Change boundary and quality guards pass.
- [ ] Validate-owned certification remains authoritative.
