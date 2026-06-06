# Scopes: BUG-049-003 — Twitter API alert metric name + runbook docs drift

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope BUG-049-003-S1: Alert metric correction + operator runbook coverage

**Status:** Done
**Priority:** P0
**Depends On:** None
**Scope-Kind:** contract-only

> Scope-Kind rationale: this scope produces a data-only correction on two
> committed product surfaces (`config/prometheus/alerts.yml` and
> `docs/Operations.md`). There is no service binary or runtime code path
> to test live. The framework contract tests
> `TestMonitoringAlertsContract_LiveFile` and
> `TestMonitoringDocsContract_LiveFile` ARE the contract-layer regression
> mechanism for both scenarios; the full 32-sub-test monitoring +
> hardening suite re-runs as part of T-BUG-049-003-003 to prove no
> collateral regression.

### Gherkin Scenarios

```gherkin
Scenario: SCN-049-B005 Every alert metric name resolves to an emitted runtime metric
  Given the live config/prometheus/alerts.yml
  And   the live internal/metrics/*.go + ml/app/metrics.py runtime metric set
  When  TestMonitoringAlertsContract_LiveFile walks every alert's expr
  Then  every metric referenced is present in the runtime-emitted set
  And   the test passes with no "NOT emitted by the live runtime" violations.

Scenario: SCN-049-B006 Every alert in alerts.yml is named in docs/Operations.md Alert Runbook
  Given the live config/prometheus/alerts.yml
  And   the live docs/Operations.md Alert Runbook section
  When  TestMonitoringDocsContract_LiveFile cross-references every alert name
  Then  each alert name appears at least once in docs/Operations.md
  And   the test passes with no "not mentioned in docs/Operations.md" violations.
```

### Implementation Plan

1. Edit `config/prometheus/alerts.yml`:
   - Comment line: rename `smackerel_connector_twitter_api_retries`
     → `smackerel_connector_twitter_api_retries_total`.
   - Expr line: rename
     `rate(smackerel_connector_twitter_api_retries{connector="twitter"}[5m])`
     → `rate(smackerel_connector_twitter_api_retries_total{connector="twitter"}[5m])`.
   - Description line: rename
     `smackerel_connector_twitter_api_retries is incrementing faster`
     → `smackerel_connector_twitter_api_retries_total is incrementing faster`.
2. Edit `docs/Operations.md` Alert Runbook table (lines 1512-1521): add
   two rows (one for `TwitterAPIRateLimitChronicExhaustion`, one for
   `TwitterAPIRetryStorm`) naming severity, backing metric, firing action.
3. Re-run `go test ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile' -count=1` → expect PASS.
4. Re-run `go test ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile' -count=1` → expect PASS.
5. Re-run `./smackerel.sh test unit --go --go-run 'TestMonitoring|TestComposeContract_LiveFile|TestComposeResourceContract|TestFilesystemContract'` → expect PASS end-to-end (5 monitoring contract files + 3 extended hardening contracts + adversarial sub-tests).
6. Re-run `artifact-lint.sh` and `state-transition-guard.sh` for the parent spec and the bug folder.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-BUG-049-003-001 | code regression (contract-only scenario-specific regression E2E) | `internal/deploy/monitoring_alerts_contract_test.go` | SCN-049-B005 | `TestMonitoringAlertsContract_LiveFile` PASSES with `TwitterAPIRetryStorm` resolving to the runtime metric. |
| T-BUG-049-003-002 | code regression (contract-only scenario-specific regression E2E) | `internal/deploy/monitoring_docs_contract_test.go` | SCN-049-B006 | `TestMonitoringDocsContract_LiveFile` PASSES with both Twitter alerts in the runbook. |
| T-BUG-049-003-003 | code regression (broader regression E2E) | `internal/deploy/monitoring_alerts_contract_test.go` + `internal/deploy/monitoring_docs_contract_test.go` + sibling `internal/deploy/monitoring_*_test.go` + extended hardening contracts | SCN-049-B005, SCN-049-B006 | All 32 monitoring + hardening sub-tests PASS via `./smackerel.sh test unit --go --go-run 'TestMonitoring\|TestComposeContract_LiveFile\|TestComposeResourceContract\|TestFilesystemContract'`. |
| T-BUG-049-003-004 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack` | both | Parent lint still PASSES. |
| T-BUG-049-003-005 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack` | both | Parent transition guard PASSES. |
| T-BUG-049-003-006 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift` | both | Bug folder lint PASSES. |
| T-BUG-049-003-007 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift` | both | Bug folder transition guard PASSES at terminal status. |
| T-BUG-049-003-008 | source citation | `internal/deploy/monitoring_alerts_contract_test.go` + `monitoring_docs_contract_test.go` | SCN-049-B005, SCN-049-B006 | Citation in report.md of `loadKnownEmittedMetrics` + `requiredAlertNames` extractors proves contract tests re-block any future drift. |

### Definition of Done

- [x] **SCN-049-B005:** `config/prometheus/alerts.yml`
      `TwitterAPIRetryStorm` rule's `expr:` references
      `smackerel_connector_twitter_api_retries_total` (with the `_total`
      suffix). The accompanying header comment and description text use
      the same corrected name.
      (Evidence: see [report.md](report.md) under `## config/prometheus/alerts.yml Diff` — three replacements documented; `### T-BUG-049-003-001` shows the contract test PASS.)
- [x] **SCN-049-B005 / T-BUG-049-003-001:** `go test
      ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile'
      -count=1` PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-001` — Exit Code: 0, Executed: YES, "--- PASS" line included.)
- [x] **SCN-049-B006:** `docs/Operations.md` Alert Runbook table contains
      a row for `TwitterAPIRateLimitChronicExhaustion` naming severity
      (warning), backing metric (`max_over_time(smackerel_connector_twitter_api_rate_limit_reset_seconds[15m])`),
      and a concrete firing action.
      (Evidence: see [report.md](report.md) under `## docs/Operations.md Diff` — new row documented; `### T-BUG-049-003-002` shows the contract test PASS.)
- [x] **SCN-049-B006:** `docs/Operations.md` Alert Runbook table contains
      a row for `TwitterAPIRetryStorm` naming severity (warning),
      backing metric (`rate(smackerel_connector_twitter_api_retries_total[5m])`),
      and a concrete firing action.
      (Evidence: see [report.md](report.md) under `## docs/Operations.md Diff`; `### T-BUG-049-003-002`.)
- [x] **SCN-049-B006 / T-BUG-049-003-002:** `go test
      ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile'
      -count=1` PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-002` — Exit Code: 0, Executed: YES, "--- PASS" line included.)
- [x] **SCN-049-B005 + SCN-049-B006 / T-BUG-049-003-003 (scenario-specific regression E2E coverage + broader E2E regression suite):**
      `./smackerel.sh test unit --go --go-run 'TestMonitoring|TestComposeContract_LiveFile|TestComposeResourceContract|TestFilesystemContract'`
      PASSES end-to-end across all 32 sub-tests (5 monitoring contract
      files + 3 extended hardening contracts). Since this is a
      contract-only scope with no runtime path, the contract-test layer
      IS the deepest applicable regression layer (per the Scope-Kind:
      contract-only opt-out documented at the top of this scope). Both
      scenario-specific and broader-suite regression coverage are
      satisfied by this gate run.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-003` — Exit Code: 0, Executed: YES, 32 PASS lines from `--- PASS:` entries.)
- [x] **Parent spec hygiene / T-BUG-049-003-004:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/049-monitoring-stack` PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-004` — Exit Code: 0, Executed: YES, "Artifact lint PASSED" line.)
- [x] **Parent spec hygiene / T-BUG-049-003-005:**
      `BUBBLES_AGENT_NAME=bubbles.validate bash
      .github/bubbles/scripts/state-transition-guard.sh
      specs/049-monitoring-stack` PASSES (`🟡 TRANSITION PERMITTED with 1 warning(s)` —
      the warning is the pre-existing test-plan-path notice).
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-005` — Exit Code: 0, Executed: YES, transition verdict captured.)
- [x] **Bug folder hygiene / T-BUG-049-003-006:** `bash
      .github/bubbles/scripts/artifact-lint.sh
      specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift`
      PASSES.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-006` — Exit Code: 0, Executed: YES, "Artifact lint PASSED" line.)
- [x] **Bug folder hygiene / T-BUG-049-003-007:**
      `BUBBLES_AGENT_NAME=bubbles.validate bash
      .github/bubbles/scripts/state-transition-guard.sh
      specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift`
      PASSES at terminal status `done`.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-007` — Exit Code: 0, Executed: YES, transition verdict captured.)
- [x] **SCN-049-B005 + SCN-049-B006 / T-BUG-049-003-008:** `report.md`
      cites `loadKnownEmittedMetrics` (alerts contract) and
      `requiredAlertNames` (docs contract) from the live test sources,
      proving the contract tests re-block any future metric-name or
      docs-runbook drift.
      (Evidence: see [report.md](report.md) section `### T-BUG-049-003-008` — includes source citations of both extractors.)
