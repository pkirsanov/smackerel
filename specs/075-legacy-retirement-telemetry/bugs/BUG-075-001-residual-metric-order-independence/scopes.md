# Scopes: BUG-075-001 Residual metric order independence

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Materialize residual telemetry through the live product path

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.test`
**Scope Kind:** live-test bugfix

### Gherkin Scenarios

```gherkin
Feature: Order-independent residual telemetry verification

  Scenario: Real retired-command usage materializes privacy-safe telemetry
    Given a clean disposable stack and an open retirement window
    When an authenticated retired-command turn is processed
    Then the residual metric exposes HELP and TYPE
    And at least one sample has only command and user_bucket labels
    And every non-anonymous user bucket is HMAC shaped

  Scenario: Telemetry absence cannot silently pass
    Given the real retired-command request succeeds
    And residual recording is disconnected
    When metrics are scraped
    Then the E2E fails because no residual sample exists
```

### Implementation Plan

1. Drive one unique retired-command turn through the existing live helper.
2. Require successful HTTP behavior before the scrape.
3. Require an actual sample and retain privacy assertions.
4. Run the test alone, in the assistant package, and through all impacted guards.

### Change Boundary

Allowed: `tests/e2e/assistant/legacy_privacy_e2e_test.go`, shared assistant E2E helpers when required, docs, and this packet.

Excluded: production metric declarations, policy behavior, database schema, deployment, release trains, and secrets.

### Implementation Files

- `tests/e2e/assistant/legacy_privacy_e2e_test.go`

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Isolated residual privacy regression | `e2e-api` | `tests/e2e/assistant/legacy_privacy_e2e_test.go` | Creates a real observation and requires a privacy-safe sample | `./smackerel.sh test e2e --go-package assistant --go-run '^TestLegacyResidualTelemetry_'` | Yes |
| Telemetry absence cannot silently pass | `e2e-api` | `tests/e2e/assistant/legacy_privacy_e2e_test.go` | The same regression requires at least one concrete sample after the real request; removing telemetry wiring fails directly | `./smackerel.sh test e2e --go-package assistant --go-run '^TestLegacyResidualTelemetry_'` | Yes |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Proves package-order independence | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Confirms report, notice, and neighboring assistant flows | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Impacted unit suite | `unit` | `internal/assistant/legacyretirement/` | Existing telemetry and privacy units remain green | `./smackerel.sh test unit --go --go-run 'LegacyRetirement|PrometheusResidual' --verbose` | No |
| Static quality | `lint` | changed files | Check, lint, and format | `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` | No |

### Definition of Done

- [ ] Root cause is confirmed by isolated failure and package-order control.
- [ ] Real retired-command usage materializes privacy-safe telemetry: a real request creates the tested observation and exact label shape.
- [ ] HELP, TYPE, and an actual sample are required.
- [ ] Exact privacy-safe labels and bucket shapes remain enforced.
- [ ] Telemetry absence cannot silently pass: removing the live precondition or telemetry wiring fails the regression because no sample exists.
- [ ] No sleep, fake scrape, direct test registration, or interception exists.
- [ ] Change Boundary contains every changed file and no excluded surface changes.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Check, lint, format, artifact, traceability, reality, and regression guards pass.
- [ ] Validate-owned certification records the strongest evidence-supported state.
