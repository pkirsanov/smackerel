# Scopes: BUG-021-004

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).

## Scope 1 — Make the cooling heuristic explicit and test-guarded

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] The cooling thresholds are named, documented package constants (`coolingSilenceMinDays`, `coolingPriorWindowStartDays`, `coolingPriorWindowEndDays`, `coolingMinPriorInteractions`, `coolingDedupWindowDays`, `coolingMaxAlertsPerRun`)
      → Evidence: report.md `### Code Diff Evidence`
- [x] `relationshipCoolingAlertQuery()` builds the SQL from the constants; the producer calls it; the produced SQL is byte-for-byte the prior inline literal (no behavior change)
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0)
- [x] `TestRelationshipCoolingHeuristic_MatchesDocumentedContract` asserts the produced query embeds the documented threshold fragments and the constants back them
      → Evidence: report.md `## Test Evidence` (PASS)
- [x] Adversarial drift: changing a threshold makes the lock test FAIL (fragment + constant assertions)
      → Evidence: report.md `## Test Evidence` (`REVERT_RC=1` drift run)
- [x] The spec/code threshold disagreement is surfaced as DI-021-004 (routed to owner); the running alert behavior and the parent spec requirement text are NOT changed
      → Evidence: bug.md `## Surfaced for owner decision`; state.json `discoveredIssues`
- [x] `go build ./internal/intelligence/...`, full intelligence package green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-COOLING-01` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage for the heuristic — the lock test persists the threshold contract and fails on drift (proven adversarially)
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression suite passes — the full `internal/intelligence` package runs green with the lock test included
      → Evidence: report.md `### Validation Evidence` (`ok ... internal/intelligence`)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-COOLING-01 | TestRelationshipCoolingHeuristic_MatchesDocumentedContract | internal/intelligence/alert_producers_test.go | source-contract (regression) | SCN-021-COOLING-01 |

### Non-Goals

- Changing the running alert behavior (Option B — owner decision, DI-021-004).
- Editing the parent spec 021 requirement text (owner follow-up).
- A live-Postgres producer integration test (no harness; out of scope).
