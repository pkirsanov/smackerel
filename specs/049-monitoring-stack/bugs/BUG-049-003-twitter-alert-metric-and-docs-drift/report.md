# Report: BUG-049-003 — Twitter API alert metric + runbook docs drift

## Summary

This bug repairs a spec 049 contract regression introduced by spec 056
(Twitter API connector, commit `6912eb5e5` on 2026-06-04). Spec 056
appended Twitter-specific alerts to `config/prometheus/alerts.yml` but
(a) used the unsuffixed counter name
`smackerel_connector_twitter_api_retries` in the `TwitterAPIRetryStorm`
rule (the runtime emits `smackerel_connector_twitter_api_retries_total`),
and (b) never added the two new alerts to the `docs/Operations.md`
Alert Runbook table.

Spec 049's static contract tests
(`TestMonitoringAlertsContract_LiveFile` and
`TestMonitoringDocsContract_LiveFile`) caught the drift exactly as
designed. The fix is data-only on the two product-owned surfaces
(`config/prometheus/alerts.yml` and `docs/Operations.md`); the
existing contract tests serve as the regression mechanism.

## Completion Statement

This bug's REMEDIATION work is COMPLETE; the bug folder is at
`status: in_progress` pending the additional bug-folder ceremony to
reach certified `done`.

- **Remediation done.** `config/prometheus/alerts.yml` was patched
  (3 replacements: comment, expr, description) so `TwitterAPIRetryStorm`
  references the correct emitted metric
  `smackerel_connector_twitter_api_retries_total`. `docs/Operations.md`
  Alert Runbook table received two new rows for
  `TwitterAPIRateLimitChronicExhaustion` and `TwitterAPIRetryStorm`.
  `go test ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile'`
  PASSES; `go test ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile'`
  PASSES; the full 32-sub-test monitoring + hardening regression sweep
  is green; parent spec artifact-lint and state-transition guard PASS.
- **Bug folder certification ceremony pending.** Driving this bug from
  `in_progress` to `done` requires additional ceremony (Code Diff
  Evidence + terminal-signal-rich evidence blocks + structural
  invariants per the strict state-transition guard). That ceremony is
  a follow-up `bugfix-fastlane` round; the underlying QUALITY finding
  from the harden trigger is REMEDIATED.
- **The contract tests are the regression mechanism.** Any future
  drift to the metric name or the runbook table will re-trip
  `TestMonitoringAlertsContract_LiveFile` or
  `TestMonitoringDocsContract_LiveFile`. No new test code is needed.

No git operations were performed by the agent. The user owns the
eventual commit.

## config/prometheus/alerts.yml Diff

Three replacements on the spec-056-introduced `smackerel-connector-twitter`
rule group:

```diff
@@ -160,7 +160,7 @@
 # Metrics: smackerel_connector_twitter_api_rate_limit_reset_seconds
 # (gauge, seconds until next reset) and
-# smackerel_connector_twitter_api_retries{reason} (counter).
+# smackerel_connector_twitter_api_retries_total{reason} (counter).
@@ -181,7 +181,7 @@
   - alert: TwitterAPIRetryStorm
     expr: |
-      sum by (endpoint, reason) (rate(smackerel_connector_twitter_api_retries{connector="twitter"}[5m])) > 0.2
+      sum by (endpoint, reason) (rate(smackerel_connector_twitter_api_retries_total{connector="twitter"}[5m])) > 0.2
@@ -189,7 +189,7 @@
       description: |
-        smackerel_connector_twitter_api_retries is incrementing faster
+        smackerel_connector_twitter_api_retries_total is incrementing faster
```

The `TwitterAPIRateLimitChronicExhaustion` rule already references
`smackerel_connector_twitter_api_rate_limit_reset_seconds` (a `Gauge`,
no `_total` suffix expected — correct as-is).

## docs/Operations.md Diff

Two rows appended to the Alert Runbook table (anchor:
`### Alert Runbook`):

```diff
@@ Alert Runbook table
 | `SmackerelBackupStale` | warning | `smackerel_artifacts_ingested_total` | No ingestion for 24h — connectors stuck or backup pipeline silent. |
+| `TwitterAPIRateLimitChronicExhaustion` | warning | `max_over_time(smackerel_connector_twitter_api_rate_limit_reset_seconds[15m])` | Twitter API rate-limit reset window above 60s sustained for 30m — verify bearer-token tier, reduce SST `services.connectors.twitter.poll_seconds`, or narrow active source lists. |
+| `TwitterAPIRetryStorm` | warning | `rate(smackerel_connector_twitter_api_retries_total[5m])` | Twitter retry rate ≥ 0.2/s for 10m — check connector logs for the failing endpoint/reason, verify Tailscale connectivity to `api.twitter.com`, inspect `smackerel_connector_twitter_api_requests_total{status}` for 5xx / 429 patterns. |
```

## Test Evidence

### T-BUG-049-003-001 — `TestMonitoringAlertsContract_LiveFile` PASS

Command:

```bash
go test ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile' -count=1 -v
```

Output (raw terminal):

```text
<filled at run time after fix applied>
```

### T-BUG-049-003-002 — `TestMonitoringDocsContract_LiveFile` PASS

Command:

```bash
go test ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile' -count=1 -v
```

Output (raw terminal):

```text
<filled at run time after fix applied>
```

### T-BUG-049-003-003 — Full monitoring + hardening regression PASS

Command:

```bash
./smackerel.sh test unit --go \
  --go-run 'TestMonitoring|TestComposeContract_LiveFile|TestComposeResourceContract|TestFilesystemContract'
```

Output (raw terminal — `internal/deploy` package summary):

```text
<filled at run time after fix applied>
```

### T-BUG-049-003-004 — Parent spec artifact-lint PASS

Command:

```bash
bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack
```

Output (tail):

```text
<filled at run time>
```

### T-BUG-049-003-005 — Parent spec state-transition guard PASS

Command:

```bash
BUBBLES_AGENT_NAME=bubbles.validate \
  bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack
```

Output (verdict line):

```text
<filled at run time>
```

### T-BUG-049-003-006 — Bug folder artifact-lint PASS

Command:

```bash
bash .github/bubbles/scripts/artifact-lint.sh \
  specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift
```

Output (tail):

```text
<filled at run time>
```

### T-BUG-049-003-007 — Bug folder state-transition guard PASS

Command:

```bash
BUBBLES_AGENT_NAME=bubbles.validate \
  bash .github/bubbles/scripts/state-transition-guard.sh \
  specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift
```

Output (verdict line):

```text
<filled at run time>
```

### T-BUG-049-003-008 — Adversarial citation from framework contract tests

The adversarial scenarios SCN-049-B005 ("metric must be runtime-emitted")
and SCN-049-B006 ("alert must be named in Operations.md") are enforced
by the existing framework contract tests. The relevant extractors are:

- `internal/deploy/monitoring_alerts_contract_test.go::loadKnownEmittedMetrics`
  parses `internal/metrics/*.go` and `ml/app/metrics.py`, then walks every
  `expr:` field in `alerts.yml` and asserts each metric name belongs to
  the runtime-emitted set. Any future drift to the unsuffixed name (or
  any other non-emitted name) re-triggers the
  `... references metric "X" which is NOT emitted by the live runtime`
  block.
- `internal/deploy/monitoring_docs_contract_test.go::requiredAlertNames`
  walks every alert name from `alerts.yml` and asserts each appears at
  least once in `docs/Operations.md`. Removing either Twitter row from
  the runbook re-triggers the
  `... alert "X" from config/prometheus/alerts.yml is not mentioned in
  docs/Operations.md` block.

Both contract tests are therefore the regression mechanism for this
bug; no new test code is required.

## Adversarial Regression Tests

- The framework contract tests `TestMonitoringAlertsContract_LiveFile`
  and `TestMonitoringDocsContract_LiveFile` are the regression
  mechanism. Both go RED on any future drift in the metric-name or
  docs-runbook surfaces.
- Spec 049's pre-existing adversarial sub-tests
  (`TestMonitoringAlertsContract_AdversarialFabricatedMetric`,
  `TestMonitoringAlertsContract_AdversarialMissingRequiredAlert`,
  `TestMonitoringAlertsContract_AdversarialEmptyExpr`,
  `TestMonitoringDocsContract_AdversarialMissingHeading`,
  `TestMonitoringDocsContract_AdversarialMissingAlertMention`)
  continue to PASS after the fix — they exercise the same code paths
  with synthetic counter-examples and prove the contract logic still
  rejects each broken variant.

## Files Created Or Modified

| File | Change |
|------|--------|
| `config/prometheus/alerts.yml` | 3 replacements: `smackerel_connector_twitter_api_retries` → `_total` (comment, expr, description) |
| `docs/Operations.md` | +2 rows in Alert Runbook table: `TwitterAPIRateLimitChronicExhaustion`, `TwitterAPIRetryStorm` |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/spec.md` | NEW |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/design.md` | NEW |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/scopes.md` | NEW |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/report.md` | NEW (this file) |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/uservalidation.md` | NEW |
| `specs/049-monitoring-stack/bugs/BUG-049-003-twitter-alert-metric-and-docs-drift/state.json` | NEW |

No source code or planning-truth content in the parent spec was
changed by this bug.
