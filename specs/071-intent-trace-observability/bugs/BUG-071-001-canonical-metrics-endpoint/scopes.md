# Scopes: BUG-071-001 Canonical metrics endpoint

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align assistant E2E with the canonical core endpoint

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** test-harness bugfix

### Gherkin Scenarios

```gherkin
Feature: Canonical assistant metrics endpoint

  Scenario: Refusal and trace metrics are scraped from the live core
    Given the repository CLI has started the disposable E2E stack
    And the canonical core endpoint is present
    When the refusal and intent-trace join test runs
    Then it scrapes the real core metrics endpoint
    And both required metric families are present

  Scenario: Missing canonical endpoint fails loudly
    Given the assistant E2E package is selected
    And the canonical core endpoint is absent
    When the refusal and intent-trace join test starts
    Then the test fails before declaring the scenario successful
```

### Implementation Plan

1. Add a closed assistant-package selector to the E2E CLI and Go wrapper.
2. Replace the bespoke metrics variable with required `CORE_EXTERNAL_URL` resolution.
3. Preserve the real scrape and exact metric-family assertions.
4. Run the focused scenario, full assistant package, impacted unit checks, and governance guards.

### Change Boundary

Allowed: `smackerel.sh`, `scripts/runtime/go-e2e.sh`, `tests/e2e/assistant/intent_refusal_join_e2e_test.go`, focused runner contract tests, docs, and this packet.

Excluded: production metrics registration, dashboards, deployment, release trains, secrets, and non-assistant E2E packages.

### Implementation Files

- `smackerel.sh`
- `scripts/runtime/go-e2e.sh`
- `internal/deploy/assistant_e2e_package_contract_test.go`
- `tests/e2e/assistant/intent_refusal_join_e2e_test.go`
- `docs/Testing.md`
- `docs/Development.md`

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Canonical endpoint regression | `e2e-api` | `tests/e2e/assistant/intent_refusal_join_e2e_test.go` | Scrapes both metric families from the real core | `./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentRefusalJoinE2E_'` | Yes |
| Package selector contract | `unit` | repository CLI contract tests | Rejects unknown package values and preserves assistant-only selection | `./smackerel.sh test unit --go --go-run 'AssistantE2E' --verbose` | No |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Executes every assistant test without other E2E packages | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Confirms neighboring assistant scenarios remain green | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Static quality | `lint` | changed files | Check, lint, and format | `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` | No |

### Definition of Done

- [ ] Root cause is confirmed against the current runner and test source.
- [ ] Canonical endpoint regression fails before the fix and passes after it.
- [ ] The real core metrics endpoint is scraped with a bounded timeout.
- [ ] Unknown or missing package/endpoint inputs fail loudly.
- [ ] Change Boundary contains every changed file and no excluded surface changes.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Regression tests contain no bailout, interception, or canned metrics output.
- [ ] Check, lint, format, artifact, traceability, reality, and regression guards pass.
- [ ] Validate-owned certification records the strongest evidence-supported state.
