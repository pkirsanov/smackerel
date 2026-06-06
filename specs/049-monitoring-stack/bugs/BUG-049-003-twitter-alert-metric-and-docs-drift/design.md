# Design: BUG-049-003 — Twitter API alert metric name + runbook docs drift

## Current Truth

The live HEAD `a697e0db800bfefa8d027d846b7a82f63a9d530b` carries two real
contract-test failures on spec 049:

```text
--- FAIL: TestMonitoringAlertsContract_LiveFile (0.00s)
    monitoring_alerts_contract_test.go:212: live config/prometheus/alerts.yml
    violates spec 049 FR-049-003 alert contract: contract violation: alert
    "TwitterAPIRetryStorm" references metric
    "smackerel_connector_twitter_api_retries" which is NOT emitted by the
    live runtime (not found in internal/metrics/*.go or ml/app/metrics.py)
    — either add the instrumentation in the runtime or remove/correct the
    alert
--- FAIL: TestMonitoringDocsContract_LiveFile (0.00s)
    monitoring_docs_contract_test.go:99: live docs/Operations.md violates
    spec 049 FR-049-005(e) docs contract: contract violation: alert
    "TwitterAPIRateLimitChronicExhaustion" from config/prometheus/alerts.yml
    is not mentioned in docs/Operations.md — an on-call engineer who
    searches for the alert name MUST find the runbook row; add the alert
    to the Alert Runbook table
```

The root cause is commit `6912eb5e5` (2026-06-04, spec 056 Twitter API
connector) which:

1. Added the `smackerel-connector-twitter` rule group to `alerts.yml`
   with `TwitterAPIRateLimitChronicExhaustion` and `TwitterAPIRetryStorm`.
2. Used `smackerel_connector_twitter_api_retries` in the `TwitterAPIRetryStorm`
   expr without the Prometheus counter `_total` suffix. The live runtime
   in `internal/metrics/metrics.go:96` emits `smackerel_connector_twitter_api_retries_total`.
3. Did not add corresponding rows to the `docs/Operations.md` Alert
   Runbook table for either Twitter alert.

The `TwitterAPIRateLimitChronicExhaustion` rule references
`smackerel_connector_twitter_api_rate_limit_reset_seconds`, which DOES
exist in the runtime (line 107, a `Gauge`; gauges legitimately do not
get the `_total` suffix). That alert's metric reference is correct; only
the docs runbook row is missing.

## Proposed Design

### Edit 1 — `config/prometheus/alerts.yml`

Three text replacements, all on the spec-056-introduced block:

| Line | Before | After |
|------|--------|-------|
| 162 (comment) | `# smackerel_connector_twitter_api_retries{reason} (counter).` | `# smackerel_connector_twitter_api_retries_total{reason} (counter).` |
| 183 (expr) | `sum by (endpoint, reason) (rate(smackerel_connector_twitter_api_retries{connector="twitter"}[5m])) > 0.2` | `sum by (endpoint, reason) (rate(smackerel_connector_twitter_api_retries_total{connector="twitter"}[5m])) > 0.2` |
| 191 (description) | `smackerel_connector_twitter_api_retries is incrementing faster` | `smackerel_connector_twitter_api_retries_total is incrementing faster` |

### Edit 2 — `docs/Operations.md`

Append two rows to the Alert Runbook table (lines 1512-1521) for the
new Twitter alerts:

| Alert | Severity | Backing Metric | Firing Action |
|-------|----------|----------------|---------------|
| `TwitterAPIRateLimitChronicExhaustion` | warning | `max_over_time(smackerel_connector_twitter_api_rate_limit_reset_seconds[15m])` | Twitter API rate-limit reset window above 60s sustained for 30m — verify bearer-token tier, reduce SST `services.connectors.twitter.poll_seconds`, or narrow active source lists. |
| `TwitterAPIRetryStorm` | warning | `rate(smackerel_connector_twitter_api_retries_total[5m])` | Twitter retry rate ≥ 0.2/s for 10m — check connector logs for the failing endpoint/reason, verify Tailscale connectivity to `api.twitter.com`, inspect `smackerel_connector_twitter_api_requests_total{status}` for 5xx / 429 patterns. |

### No Other Changes

- No `internal/metrics/metrics.go` change — the runtime metric is
  correctly named with the `_total` suffix already.
- No spec 056 planning-truth change — spec 056 is shipped; the broken
  surfaces live in spec 049-owned files.
- No new test code — the existing
  `TestMonitoringAlertsContract_LiveFile` and
  `TestMonitoringDocsContract_LiveFile` are the regression mechanism.
  They are currently RED; once the data is fixed they go GREEN and
  remain GREEN against any future drift.

## Test Strategy

| Test ID | Type | Location | Purpose |
|---------|------|----------|---------|
| T-BUG-049-003-001 | go regression | `go test ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile' -count=1` | Alert contract PASSES with `TwitterAPIRetryStorm` resolving to the runtime metric. |
| T-BUG-049-003-002 | go regression | `go test ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile' -count=1` | Docs contract PASSES with both Twitter alerts named in the Alert Runbook table. |
| T-BUG-049-003-003 | go full regression | `./smackerel.sh test unit --go --go-run 'TestMonitoring\|TestComposeContract_LiveFile\|TestComposeResourceContract\|TestFilesystemContract'` | All 5 monitoring contract files + 3 extended hardening contracts PASS end-to-end. |
| T-BUG-049-003-004 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack` | Parent spec lint still PASSES. |
| T-BUG-049-003-005 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack` | Parent spec transition guard PASSES. |
| T-BUG-049-003-006 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift` | Bug folder lint PASSES. |
| T-BUG-049-003-007 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift` | Bug folder transition guard PASSES at terminal status. |
| T-BUG-049-003-008 (adversarial citation) | source | `internal/deploy/monitoring_alerts_contract_test.go::loadKnownEmittedMetrics` + `monitoring_docs_contract_test.go::requiredAlertNames` | Citation in `report.md` showing the contract test's adversarial branches that would re-block any future drift. |

## Risk Controls

- The fix is data-only (one YAML file + one Markdown file).
- The existing contract tests are the regression mechanism: they were
  RED before the fix, will be GREEN after, and re-trip RED on any
  future drift.
- The metric name correction goes from a non-existent name to an
  existing name; rule reload in Prometheus will start matching real
  series immediately.
- The docs entries are operator-facing prose; reviewable, reversible.
- No source-code build artifact changes; no risk of binary drift.

## Open Questions

- **OQ-BUG-049-003-A:** Should this fix also bump the firing thresholds
  (`>60` for 30m, `>0.2/s` for 10m)?
  **Resolution:** No. Threshold tuning is operator-overlay territory
  per spec 049's "Risk Controls" section ("thresholds are documented
  in the alert body so operators can tune via overlay without touching
  the rule file"). This bug fixes the contract drift only.
- **OQ-BUG-049-003-B:** Should this bug live under spec 056 instead?
  **Resolution:** No. Per artifact-ownership-routing "fix where the
  broken state lives": the alert file and Operations.md monitoring
  section are spec 049-owned product surfaces. Spec 056 shipped its
  amendment; the corrective fix lands in the contract-owning spec.

### Single-Implementation Justification

This bug is a single data-only correction across two product surfaces
(`config/prometheus/alerts.yml` and `docs/Operations.md`) that share one
logical owner: spec 049's static monitoring contract. There is no
second provider, no adapter, no strategy, no plugin to choose between.
The existing `internal/deploy/monitoring_alerts_contract_test.go` and
`internal/deploy/monitoring_docs_contract_test.go` are the single
implementation surface (Go static contract tests) and they already
enforce the invariant for every alert and every docs runbook row. No
additional implementation variant, plugin layer, or foundation/overlay
split is warranted.
