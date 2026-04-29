# Report: BUG-010-002 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 5 of 11 Gherkin scenarios in `specs/010-browser-history-connector` had no faithful matching DoD item: `SCN-BH-001`, `SCN-BH-002`, `SCN-BH-003`, `SCN-BH-004`, `SCN-BH-008`. Investigation confirmed the gap is artifact-only — every scenario is fully delivered in production code (`internal/connector/browser/connector.go`, `browser.go`) and exercised by passing unit tests. The DoD bullets simply did not embed the `SCN-BH-NNN` trace IDs that the guard's content-fidelity matcher requires.

The fix added 5 trace-ID-bearing DoD bullets to `specs/010-browser-history-connector/scopes.md` (4 in Scope 01, 1 in Scope 02). No production code was modified; the boundary clause in the user prompt was honored. Same playbook as `specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap`.

## Completion Statement

All 7 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (5 unmapped scenarios, 6 failures) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. The 15 underlying behavior tests for the previously-flagged scenarios still pass with no regressions.

## Test Evidence

### Underlying behavior tests (regression-protection for the artifact fix)

> Phase agent: bubbles.test
> Executed: YES

```
$ go test -count=1 -v -run 'TestProcessEntries_DwellTimeTiering$|TestSync_EmptyCursor_UsesLookback$|TestParseChromeHistorySince_HasLimit$|TestCursorConversion_RoundTrip$|TestProcessEntries_CursorAdvances$|TestProcessEntries_SkipFiltering$|TestShouldSkip$|TestShouldSkip_SchemePrefixedLocalhost$|TestConnect_HistoryFileNotFound$|TestConnect_HistoryFileNotReadable$|TestHealth_FileDisappearsAfterConnect$|TestDetectRepeatVisits_TierEscalation$|TestEscalateTier_AllTransitions$|TestDetectRepeatVisits_BelowThreshold_NoEscalation$|TestDetectRepeatVisits_RespectsWindow$' ./internal/connector/browser/
=== RUN   TestShouldSkip
--- PASS: TestShouldSkip (0.00s)
=== RUN   TestShouldSkip_SchemePrefixedLocalhost
--- PASS: TestShouldSkip_SchemePrefixedLocalhost (0.00s)
=== RUN   TestParseChromeHistorySince_HasLimit
--- PASS: TestParseChromeHistorySince_HasLimit (0.00s)
=== RUN   TestProcessEntries_DwellTimeTiering
--- PASS: TestProcessEntries_DwellTimeTiering (0.00s)
=== RUN   TestProcessEntries_SkipFiltering
--- PASS: TestProcessEntries_SkipFiltering (0.00s)
=== RUN   TestConnect_HistoryFileNotFound
--- PASS: TestConnect_HistoryFileNotFound (0.00s)
=== RUN   TestCursorConversion_RoundTrip
--- PASS: TestCursorConversion_RoundTrip (0.00s)
=== RUN   TestSync_EmptyCursor_UsesLookback
--- PASS: TestSync_EmptyCursor_UsesLookback (0.00s)
=== RUN   TestProcessEntries_CursorAdvances
--- PASS: TestProcessEntries_CursorAdvances (0.00s)
=== RUN   TestDetectRepeatVisits_TierEscalation
--- PASS: TestDetectRepeatVisits_TierEscalation (0.00s)
=== RUN   TestEscalateTier_AllTransitions
--- PASS: TestEscalateTier_AllTransitions (0.00s)
=== RUN   TestDetectRepeatVisits_BelowThreshold_NoEscalation
--- PASS: TestDetectRepeatVisits_BelowThreshold_NoEscalation (0.00s)
=== RUN   TestDetectRepeatVisits_RespectsWindow
--- PASS: TestDetectRepeatVisits_RespectsWindow (0.00s)
=== RUN   TestConnect_HistoryFileNotReadable
--- PASS: TestConnect_HistoryFileNotReadable (0.01s)
=== RUN   TestHealth_FileDisappearsAfterConnect
2026/04/27 02:02:05 INFO browser history connector connected history_path=/tmp/TestHealth_FileDisappearsAfterConnect2494257420/001/History access_strategy=copy
2026/04/27 02:02:05 WARN chrome history file no longer accessible path=/tmp/TestHealth_FileDisappearsAfterConnect2494257420/001/History error="stat /tmp/TestHealth_FileDisappearsAfterConnect2494257420/001/History: no such file or directory"
--- PASS: TestHealth_FileDisappearsAfterConnect (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/browser       0.092s
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES

```
$ timeout 120 bash .github/bubbles/scripts/traceability-guard.sh specs/010-browser-history-connector 2>&1 | tail -20
✅ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate scenario maps to DoD item: SCN-BH-006 Social media visits are aggregated at domain level
✅ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate scenario maps to DoD item: SCN-BH-007 Long social media read gets individual processing
✅ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate scenario maps to DoD item: SCN-BH-008 Repeat visits escalate processing tier
✅ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate scenario maps to DoD item: SCN-BH-009 Metadata-tier entries produce only domain aggregates
✅ Scope 02: Social Media Aggregation, Repeat Visits & Privacy Gate scenario maps to DoD item: SCN-BH-010 Content fetch failure produces metadata-only artifact
ℹ️  DoD fidelity: 11 scenarios checked, 11 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 11
ℹ️  Test rows checked: 41
ℹ️  Scenario-to-row mappings: 11
ℹ️  Concrete test file references: 11
ℹ️  Report evidence references: 11
ℹ️  DoD fidelity scenarios: 11 (mapped: 11, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed. Pre-fix run on the same revision (with the unfixed artifacts) reported `RESULT: FAILED (6 failures)` including `DoD fidelity: 11 scenarios checked, 6 mapped to DoD, 5 unmapped` — see Section "Pre-fix Reproduction" below.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES

Artifact lint runs are recorded in `### Audit Evidence — Artifact Lint` after the closing finalization step. `git diff --name-only` confirms the change boundary:

```
$ git diff --name-only
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/design.md
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/report.md
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/scopes.md
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/spec.md
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/state.json
specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/uservalidation.md
specs/010-browser-history-connector/scopes.md
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path.

## Pre-fix Reproduction

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

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits).

## Audit Evidence — Artifact Lint

Captured during the validation/audit phase after the DoD bullets were appended.

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/010-browser-history-connector 2>&1 | tail -30
✅ Strict section 'Validation Evidence' includes Executed: YES
✅ Strict section 'Validation Evidence' includes command evidence
✅ Strict section 'Validation Evidence' includes phase agent marker 'bubbles.validate'
✅ Strict section 'Audit Evidence' includes Executed: YES
✅ Strict section 'Audit Evidence' includes command evidence
✅ Strict section 'Audit Evidence' includes phase agent marker 'bubbles.audit'
✅ Strict section 'Chaos Evidence' includes Executed: YES
✅ Strict section 'Chaos Evidence' includes command evidence
✅ Strict section 'Chaos Evidence' includes phase agent marker 'bubbles.chaos'
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
✅ All 20 evidence blocks in report.md contain legitimate terminal output
✅ No narrative summary phrases detected in report.md
✅ Required specialist phase 'implement' recorded in execution/certification phase records
✅ Required specialist phase 'test' recorded in execution/certification phase records
✅ Required specialist phase 'docs' recorded in execution/certification phase records
✅ Required specialist phase 'validate' recorded in execution/certification phase records
✅ Required specialist phase 'audit' recorded in execution/certification phase records
✅ Required specialist phase 'chaos' recorded in execution/certification phase records
✅ Spec-review phase recorded for 'full-delivery' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap 2>&1 | tail -30
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
✅ state.json v3 has recommended field: transitionRequests
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
✅ Workflow mode 'bugfix-fastlane' allows status 'done'
✅ All 1 scope(s) in scopes.md are marked Done
✅ Required specialist phase 'implement' found in execution/certification phase records
✅ Required specialist phase 'test' found in execution/certification phase records
✅ Required specialist phase 'validate' found in execution/certification phase records
✅ Required specialist phase 'audit' found in execution/certification phase records
✅ Phase-scope coherence verified (Gate G027)
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ workflowMode gate satisfied: ### Validation Evidence
✅ workflowMode gate satisfied: ### Audit Evidence
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
✅ All evidence blocks contain legitimate terminal output
✅ No narrative summary phrases detected in report.md

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Claim Source:** executed. Both lint runs return exit 0 with `Artifact lint PASSED.`
