# User Validation: BUG-049-003 — Twitter API alert metric + runbook docs drift

## Checklist

- [x] The metric name typo `smackerel_connector_twitter_api_retries`
      (no `_total`) does not match the live runtime emission
      `smackerel_connector_twitter_api_retries_total` in
      `internal/metrics/metrics.go:96`.
- [x] The two Twitter alerts (`TwitterAPIRateLimitChronicExhaustion`
      and `TwitterAPIRetryStorm`) are missing from the
      `docs/Operations.md` Alert Runbook table.
- [x] The fix touches `config/prometheus/alerts.yml` and
      `docs/Operations.md` only — no source code, no spec planning
      truth, no spec 056 changes.
- [x] The existing `TestMonitoringAlertsContract_LiveFile` and
      `TestMonitoringDocsContract_LiveFile` contract tests are the
      regression mechanism (currently RED, will be GREEN after fix).
- [x] No `git commit` / `git push` actions are taken by the agent.
- [x] The bug folder ships with all six required artifacts.
