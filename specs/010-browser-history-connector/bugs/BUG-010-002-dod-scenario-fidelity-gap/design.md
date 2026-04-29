# Design: BUG-010-002 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [010 spec](../../spec.md) | [010 scopes](../../scopes.md) | [010 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 010 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the delivered behavior (cursor advancement, Connect/Sync orchestration, health lifecycle, repeat visit detection, tier escalation) but did not embed the `SCN-BH-NNN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for these five scenarios the DoD wording happened to fall below the threshold (the scenario titles use words like "initial sync imports history with dwell-time tiering" while the closest DoD bullets use words like "Sync orchestrates: cursor parse → find new files → parse → filter → normalize → return artifacts + cursor").

Five scenarios were affected:

- **SCN-BH-001** (Scope 01): Existing DoD bullets describe the `Connector` interface contract and `Sync` orchestration in implementation language; none names the scenario.
- **SCN-BH-002** (Scope 01): Existing DoD bullets describe `ParseChromeHistorySince` LIMIT/ASC behavior and cursor codec but use the words "ParseChromeHistorySince added… cursor-based query, ASC order, LIMIT 10000 per batch" rather than echoing "incremental sync processes only new visits".
- **SCN-BH-003** (Scope 01): No DoD bullet describes skip filtering at all — the closest existing bullet is "Connect validates config: history_path non-empty and exists" which is unrelated.
- **SCN-BH-004** (Scope 01): The "Health lifecycle transitions verified" bullet covers the behavior but uses different vocabulary than the scenario name.
- **SCN-BH-008** (Scope 02): The "Repeat visit detection counts URL frequency within configurable window" and "Tier escalation applied for URLs exceeding repeat visit threshold" bullets cover the behavior but, again, the fuzzy match did not detect a 3-word overlap with "Repeat visits escalate processing tier".

The same root cause and same fix shape was used in `specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap`. This bug applies the identical playbook to spec 010.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only fix preferred. No production code changes unless scenario truly undelivered." — is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has two parts:

1. **Trace-ID-bearing DoD bullets** added to `scopes.md`:
   - Scope 01 DoD gains four bullets — one each for `SCN-BH-001`, `SCN-BH-002`, `SCN-BH-003`, `SCN-BH-004` — with raw `go test` output and a source-file pointer.
   - Scope 02 DoD gains one bullet for `SCN-BH-008` with raw `go test` output and a source-file pointer to `detectRepeatVisits`/`escalateTier`.

2. **Bug packet** at `specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap/` documenting the gap, the per-scenario classification, and the verification evidence.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — initial sync, cursor-based incremental sync, skip filtering, health-error reporting, and repeat-visit tier escalation are all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (6 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence".
