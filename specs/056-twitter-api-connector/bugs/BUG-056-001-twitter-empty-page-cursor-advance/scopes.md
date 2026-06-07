# Scopes: BUG-056-001

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).

## Scope 1 — Anchor the resume cursor to the last non-empty page

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `fetchEndpointPaginated` advances `lastNonEmptyToken` only when `len(body.Data) > 0`; the in-loop `cursor` still advances every page
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] Regression test `TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor` asserts the cursor stays anchored to `PAGE2_TOKEN` for an empty non-terminal page; red before the fix, green after
      → Evidence: report.md `## Test Evidence` (red `PAGE3_TOKEN` → green `ok`)
- [x] Adversarial re-RED: reverting the guard makes the new test FAIL ("advanced it to PAGE3_TOKEN")
      → Evidence: report.md `## Test Evidence` (`REVERT_RC=1`)
- [x] Existing pagination contracts preserved (`TestTwitterAPI_ReplayPagination`, `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor`, restart + bounds tests)
      → Evidence: report.md `## Test Evidence` (full package `ok`)
- [x] `go build ./internal/connector/twitter/...`, `go vet`, full package test green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-056-CURSOR-01` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage for the fixed behavior — the new test drives `fetchEndpointPaginated` through a sparse-page fixture and persists the anchored-cursor invariant; it fails if the guard regresses
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression suite passes — the full `internal/connector/twitter` package runs green with the new test included
      → Evidence: report.md `## Test Evidence` (`ok ... 4.052s`)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-056-CURSOR-01 | TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor | internal/connector/twitter/api_test.go | regression (red→green) | SCN-056-CURSOR-01 |
| T-056-CURSOR-02 | TestTwitterAPI_ReplayPagination (preserved) | internal/connector/twitter/api_test.go | regression (preserved) | SCN-056-CURSOR-02 |

### Non-Goals

- Rate-limit / dedup / cursor-restart behaviors (unchanged).
