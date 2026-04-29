# Bug: BUG-010-002 — DoD scenario fidelity gap (SCN-BH-001/002/003/004/008)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 010 — Browser History Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 5 of the 11 Gherkin scenarios in the parent feature's `scopes.md` had no faithful matching DoD item:

- `SCN-BH-001` Initial sync imports history with dwell-time tiering
- `SCN-BH-002` Incremental sync processes only new visits
- `SCN-BH-003` Skip rules filter non-content URLs
- `SCN-BH-004` Chrome History file not found reports health error
- `SCN-BH-008` Repeat visits escalate processing tier

Gate G068's content-fidelity matcher requires a DoD bullet to either (a) carry the same `SCN-BH-NNN` trace ID as the Gherkin scenario, or (b) share enough significant words with the scenario name. The pre-existing DoD entries described the implemented behavior in implementation language (cursor management, Connect/Sync orchestration, health lifecycle, repeat visit detection, tier escalation) but did not embed the `SCN-BH-NNN` trace ID, and the fuzzy matcher's significant-word threshold was not satisfied for these five scenarios.

This is the same failure pattern as `specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap`.

## Reproduction (Pre-fix)

```
$ timeout 120 bash .github/bubbles/scripts/traceability-guard.sh specs/010-browser-history-connector 2>&1 | tail -10
❌ Scope 01: Connector Implementation, Config & Registration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-BH-001 Initial sync imports history with dwell-time tiering
❌ Scope 01: Connector Implementation, Config & Registration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-BH-002 Incremental sync processes only new visits
❌ Scope 01: Connector Implementation, Config & Registration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-BH-003 Skip rules filter non-content URLs
❌ Scope 01: Connector Implementation, Config & Registration Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-BH-004 Chrome History file not found reports health error
❌ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-BH-008 Repeat visits escalate processing tier
ℹ️  DoD fidelity: 11 scenarios checked, 6 mapped to DoD, 5 unmapped
❌ DoD content fidelity gap: 5 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
RESULT: FAILED (6 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each missing scenario the bug investigator searched the production code (`internal/connector/browser/connector.go`, `browser.go`) and the test files (`connector_test.go`, `browser_test.go`). All five behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gap is that DoD bullets did not embed the `SCN-BH-NNN` ID that the guard uses for fidelity matching.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-BH-001 | Yes — initial-cursor `Sync` copies the History file, parses entries via `ParseChromeHistorySince`, classifies each entry into `full`/`standard`/`light`/`metadata` tier via `DwellTimeTier`, returns `RawArtifact`s, and advances the cursor | Yes — `TestProcessEntries_DwellTimeTiering`, `TestSync_EmptyCursor_UsesLookback` PASS | `internal/connector/browser/connector_test.go` | `internal/connector/browser/connector.go::Connect`, `Sync`, `processEntries`; `browser.go::DwellTimeTier` |
| SCN-BH-002 | Yes — `ParseChromeHistorySince` issues a `WHERE visit_time > ?` cursor query with ASC ordering and `LIMIT 10000` per batch; the connector advances the cursor only past entries newer than the prior cursor | Yes — `TestParseChromeHistorySince_HasLimit`, `TestCursorConversion_RoundTrip`, `TestProcessEntries_CursorAdvances` PASS | `internal/connector/browser/connector_test.go`, `browser_test.go` | `internal/connector/browser/browser.go::ParseChromeHistorySince`; `connector.go::processEntries`, `parseCursorToChromeSafe` |
| SCN-BH-003 | Yes — `ShouldSkip` rejects `chrome://`, `chrome-extension://`, `about:`, `file://`, scheme-prefixed `localhost`, and configured `CustomSkipDomains`; only real content URLs reach the dwell-tier classifier | Yes — `TestProcessEntries_SkipFiltering`, `TestShouldSkip`, `TestShouldSkip_SchemePrefixedLocalhost` PASS | `internal/connector/browser/connector_test.go`, `browser_test.go` | `internal/connector/browser/browser.go::ShouldSkip`; `connector.go::processEntries` (skip-filter pass) |
| SCN-BH-004 | Yes — `Connect` validates the configured `history_path`, returns an error containing the path on missing/unreadable files, and `Health` transitions to `error` if the file disappears after a previously-successful Connect | Yes — `TestConnect_HistoryFileNotFound`, `TestConnect_HistoryFileNotReadable`, `TestHealth_FileDisappearsAfterConnect` PASS | `internal/connector/browser/connector_test.go` | `internal/connector/browser/connector.go::Connect`, `Health` |
| SCN-BH-008 | Yes — `detectRepeatVisits` counts URL frequency within `RepeatVisitWindow` and `escalateTier` raises a URL's processing tier (`light` → `standard`, `standard` → `full`) once visit count meets `RepeatVisitThreshold`; below-threshold and out-of-window visits are not escalated | Yes — `TestDetectRepeatVisits_TierEscalation`, `TestEscalateTier_AllTransitions`, `TestDetectRepeatVisits_BelowThreshold_NoEscalation`, `TestDetectRepeatVisits_RespectsWindow` PASS | `internal/connector/browser/connector_test.go` | `internal/connector/browser/connector.go::detectRepeatVisits`, `escalateTier`; `processEntries` (tier-escalation pass) |

**Disposition:** All five scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/010-browser-history-connector/scopes.md` has a DoD bullet that explicitly contains `SCN-BH-001` with raw `go test` evidence and a source-file pointer
- [x] Same for `SCN-BH-002`, `SCN-BH-003`, `SCN-BH-004`, `SCN-BH-008`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/010-browser-history-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/010-browser-history-connector` PASS with `DoD fidelity: 11 mapped, 0 unmapped`
- [x] No production code changed (boundary)
