# Design: BUG-014-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [014 spec](../../spec.md) | [014 scopes](../../scopes.md) | [014 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 014 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately describe the delivered behavior (message classification, tier assignment, REST pagination, cursor advancement, connector lifecycle, gateway event capture, thread-disable config, SSRF rejection) but did not embed the `SCN-DC-NNN-NNN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for these eight scenarios the DoD wording fell below the threshold (the DoD spoke in implementation terms — "fetchChannelMessages() paginates with backfill_limit" — while the Gherkin scenario name says "Fetch message history with pagination"), so the gate fails.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has one part: **trace-ID-bearing DoD bullets** appended to each affected scope's DoD section in `specs/014-discord-connector/scopes.md`. Each new bullet:

1. Is checked `[x]`.
2. Restates the scenario name verbatim and embeds the `SCN-DC-NNN-NNN` ID, guaranteeing trace-ID-equality matching in the guard's first pass.
3. Cites the existing test functions (`TestClassifyMessage`, `TestAssignTier`, `TestFetchChannelMessages_*`, `TestSyncEndToEnd_*`, `TestConnect_*`, `TestEventPoller_*`, `TestSync_IncludeThreadsFalse_SkipsThreads`, `TestParseBotCommand_SSRFProtection` / `TestIsSafeURL`).
4. Cites the existing source functions (`classifyMessage`, `assignTier`, `fetchChannelMessages`, `Sync`, `Connect`/`Close`, `EventPoller`, `IncludeThreads` gate, `ParseBotCommand` / `isSafeURL`).

The scenario-manifest.json for spec 014 already exists and already lists all 13 scenarios with `linkedTests` and `evidenceRefs`, so no manifest change is required for this bug. The Test Plan rows in `scopes.md` already carry per-row `Scenario ID` columns, so no Test Plan rewrite is required.

Bullets added (Scope → Scenario):

| Scope | Scenario | New DoD bullet wording (head) |
|---|---|---|
| 01 | SCN-DC-NRM-001 | "Scenario SCN-DC-NRM-001 — Classify all message content types ... is delivered and tested" |
| 01 | SCN-DC-NRM-002 | "Scenario SCN-DC-NRM-002 — Assign processing tiers per R-007 ... is delivered and tested" |
| 02 | SCN-DC-REST-001 | "Scenario SCN-DC-REST-001 — Fetch message history with pagination ... is delivered and tested" |
| 02 | SCN-DC-REST-002 | "Scenario SCN-DC-REST-002 — Per-channel cursor advancement ... is delivered and tested" |
| 03 | SCN-DC-CONN-001 | "Scenario SCN-DC-CONN-001 — Connector lifecycle ... is delivered and tested" |
| 04 | SCN-DC-GW-001 | "Scenario SCN-DC-GW-001 — Real-time message capture ... is delivered and tested" |
| 05 | SCN-DC-THR-004 | "Scenario SCN-DC-THR-004 — Thread ingestion disabled via config ... is delivered and tested" |
| 06 | SCN-DC-CMD-003 | "Scenario SCN-DC-CMD-003 — Capture command with unsafe URL rejected ... is delivered and tested" |

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — message classification, tier assignment, REST pagination, cursor advancement, connector lifecycle, gateway capture, thread-disable, SSRF rejection are all genuinely delivered and tested) and only add the trace ID and named test/source references the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (9 failures, 0 warnings)` with `DoD fidelity: 13 scenarios checked, 5 mapped to DoD, 8 unmapped`. Post-fix it returns `RESULT: PASSED (0 warnings)` with `DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped`. The guard runs are captured in `report.md` under "Validation Evidence" and "Pre-fix Reproduction".
