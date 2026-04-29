# Design: BUG-017-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [017 spec](../../spec.md) | [017 scopes](../../scopes.md) | [017 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 017 was authored before Gate G068 (Gherkin → DoD Content Fidelity) and the per-scope Test Plan requirement were tightened in the trace guard. Two related artifact-level defects accumulated:

1. **No `### Test Plan` table on any of the 6 scopes.** `extract_test_rows` in `traceability-guard.sh` matches `^### Test Plan` and pipes through three `grep` filters; with no rows the pipeline returns non-zero and (under `set -euo pipefail`) the script aborts before reaching Gate G068. Once Test Plan tables are present, the per-scope traceability pass also requires each scenario to map to a row whose trace ID or fuzzy-word overlap matches the scenario, and the row must point at an existing file path on disk.
2. **DoD bullets did not embed `SCN-GA-NNN` trace IDs.** The DoD wording accurately described the delivered behavior (proximity filter, lifecycle dedup, USGS GeoJSON parsing, NWS CAP fields, multi-source aggregation, source-specific severity, NATS notification, travel radius doubling) but did not embed the trace ID the guard's content-fidelity matcher uses as its first criterion.
3. **report.md never named `tests/integration/weather_alerts_test.go` or `tests/e2e/weather_alerts_e2e_test.go`.** Scope 06's Test Plan resolves to those live-stack files (and to `internal/connector/alerts/alerts_test.go` for the in-process proxies). The `report_mentions_path` check in the guard requires every concrete test file from a Test Plan row to appear in `report.md` by exact relative path. The parent report referenced `internal/connector/alerts/alerts_test.go` extensively but never named the integration/e2e files.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. Boundary clause from the user prompt is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts, all confined to documentation:

1. **`### Test Plan` table on every scope** in `specs/017-gov-alerts-connector/scopes.md`. Each row carries the `SCN-GA-*` trace ID, names a concrete test file path that exists on disk, and a one-line assertion summary. Rows reference `internal/connector/alerts/alerts_test.go` for unit coverage and `tests/integration/weather_alerts_test.go` / `tests/e2e/weather_alerts_e2e_test.go` for the live-stack proxies for Scope 06 SCN-GA-NOTIF-001/002.

2. **Trace-ID-bearing DoD bullets** appended to each scope's existing DoD list: one new bullet per `SCN-GA-*` scenario, each citing the scenario ID by name, naming the concrete test file path, and quoting the raw `go test` PASS output. The new bullets *preserve* the original DoD claims (each scenario's behavior is genuinely delivered and tested) and only *add* the trace ID + raw evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited.

3. **report.md cross-reference section** documenting BUG-017-001, naming `internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, and `tests/e2e/weather_alerts_e2e_test.go` by full relative path, and recording the per-scenario classification and raw guard before/after output.

4. **Scenario manifest is left as-is**: `scenario-manifest.json` already covers all 13 `SCN-GA-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`, and the manifest cross-check passes pre-fix.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — proximity filtering, alert lifecycle, severity classification, multi-source sync, source-specific parsing, NATS notification, and travel radius doubling are all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix the guard exited silently after the manifest cross-check (test_rows empty under pipefail) and Gate G068 was never reached; post-Test-Plan-insertion it then returned `RESULT: FAILED (1 failures)` for the missing `tests/e2e/weather_alerts_e2e_test.go` reference; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence".
