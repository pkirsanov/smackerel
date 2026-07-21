# Scopes: BUG-075-001 Residual metric order independence

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Materialize residual telemetry through the live product path

**Status:** Done
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

- [x] Root cause is confirmed by isolated failure and package-order control. → Evidence: [report.md](report.md) "Prior-Session Evidence" RED (`TestLegacyResidualTelemetry_` FAILs first-in-order: HELP line missing, metric not yet registered) + "Live E2E — isolated order-independence proof" (the SAME test PASSes at 19.55s cold-start as the first-and-only test) — the defect was package-order dependence and it is gone.
- [x] Real retired-command usage materializes privacy-safe telemetry: a real request creates the tested observation and exact label shape. → Evidence: [report.md](report.md) "Live E2E — isolated order-independence proof" — the test posts a real authenticated retired-command turn (unique `turnID`), asserts `200`, then requires a concrete `smackerel_legacy_command_residual_total` sample with the exact `{command,user_bucket}` labels and HMAC-shaped bucket (`LEG_A_EXIT=0`).
- [x] HELP, TYPE, and an actual sample are required. → Evidence: [report.md](report.md) "### Code Diff Evidence" (retains the HELP/TYPE registration checks and adds `sampleCount == 0 → t.Fatalf`) + "Live E2E — isolated order-independence proof" GREEN (a concrete sample was present after the real turn).
- [x] Exact privacy-safe labels and bucket shapes remain enforced. → Evidence: [report.md](report.md) "### Code Diff Evidence" — each sample must carry exactly `len(labels) == 2` = `command,user_bucket`, and every `user_bucket` must match `^[0-9a-f]{64}$` (HMAC-SHA256); enforced live in leg (a) GREEN.
- [x] Telemetry absence cannot silently pass: removing the live precondition or telemetry wiring fails the regression because no sample exists. → Evidence: [report.md](report.md) "### Code Diff Evidence" (the empty-bucket zero-sample allowance `if val == "" { continue }` is removed; `sampleCount == 0 → t.Fatalf`) + `regression-quality-guard.sh --bugfix` → "Adversarial signal detected" (`RQG_BUGFIX_EXIT=0`), a non-tautological guard.
- [x] No sleep, fake scrape, direct test registration, or interception exists. → Evidence: [report.md](report.md) "### Code Diff Evidence" (uses the real live helpers `loadLegacyRetirementNoticeLiveStack` / `postNoticeAssistantTurn` / `scrapeMetrics`; no `time.Sleep`, no registry mutation, no `httptest`/route interception) + `implementation-reality-scan.sh` → PASSED (`REALITY_EXIT=0`) + `regression-quality-guard.sh` → 0 violations.
- [x] Change Boundary contains every changed file and no excluded surface changes. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `git show 8ac848e1 --numstat` shows the only changed file is `tests/e2e/assistant/legacy_privacy_e2e_test.go` (+31/−6); no production metric declaration, policy, schema, deployment, release-train, or secret surface touched; working tree is packet-only.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [report.md](report.md) "Live E2E — isolated order-independence proof" — SCN-001 (real-usage materializes telemetry) + SCN-002 (telemetry absence fails directly) are both encoded in `TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly`, PASS on the live stack (`ok … 19.577s`, `LEG_A_EXIT=0`).
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "Broader E2E regression — full assistant package" — 40 PASS; the residual test + every neighboring legacy-retirement product flow GREEN. The only 2 failures are pre-existing, unrelated `buildvcs` (`error obtaining VCS status: exit 128`) environment failures in `intent_replay_test.go` — a different subsystem, outside this change boundary, not caused by this change (working tree packet-only), owned by concurrent spec069 deterministic-e2e work (G051 class; DI-075-001-01).
- [x] Check, lint, format, artifact, traceability, reality, and regression guards pass. → Evidence: [report.md](report.md) "Guards & Quality Gates" — `CHECK_EXIT=0`, `FORMAT_EXIT=0`, `LINT_EXIT=0`, `UNIT_EXIT=0` (29 PASS), `artifact-lint` exit 0, `traceability-guard` PASSED, `regression-quality` (standard + `--bugfix`) exit 0, `implementation-reality-scan` PASSED; `state-transition-guard` verdict PASS (`failedGateIds []`).
- [x] Validate-owned certification records the strongest evidence-supported state. → Evidence: [state.json](state.json) `certification.certifierAgent = bubbles.validate`, `certification.status = done`; the validate phase owns terminal certification (recorded in `execution.executionHistory`).
