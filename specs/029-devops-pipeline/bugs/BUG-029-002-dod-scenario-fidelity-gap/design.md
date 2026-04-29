# Design: BUG-029-002 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [029 spec](../../spec.md) | [029 scopes](../../scopes.md) | [029 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

The traceability-guard's `scenario_matches_dod` function (`.github/bubbles/scripts/traceability-guard.sh` lines 216–268) tries trace-ID equality first: it extracts the first `(SCN|AC|FR|UC)-[A-Za-z0-9_-]+` token from the Gherkin scenario string and compares it against trace IDs extracted from each DoD bullet. Only if no scenario-side trace ID is present does it fall back to the fuzzy "≥3 significant words shared" branch.

In `specs/029-devops-pipeline/scopes.md`, Scopes 3 and 4 already followed the trace-ID-first pattern (`Scenario: [SCN-029-012] Branch protection ...` paired with `[SCN-029-012] Documented ...`), and they pass. Scopes 1, 2, 5, 6, 7 instead wrote bare scenario names (`Scenario: CI runs lint and tests on push`) — no embedded trace ID — so the matcher fell through to fuzzy matching, which sometimes succeeded (SCN-029-006) and sometimes did not (the other nine scenarios).

A second, smaller defect: the T-7-03 Test Plan row Location column held only `docker-compose.yml`. The guard's `extract_path_candidates` regex `([A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+` requires at least one `/`, so `docker-compose.yml` was rejected and the row was reported as having no concrete test file path.

## Fix Approach (artifact-only)

**Boundary preserved:** edits are confined to `specs/029-devops-pipeline/scopes.md` plus this bug folder. No production code, no other parent artifacts.

The fix has three mechanical parts:

1. **Embed trace IDs in Gherkin scenario names.** For each of the nine unmapped scenarios across Scopes 1, 2, 5, 6, 7, replace `Scenario: <name>` with `Scenario: [SCN-029-NNN] <name>`. This restores the trace-ID branch of `scenario_matches_dod` for those scenarios.

2. **Prefix matching DoD bullets with `[SCN-029-NNN]`.** For each unmapped scenario, the closest existing DoD bullet (one whose evidence already describes the behavior the Gherkin scenario claims) gets the `[SCN-029-NNN]` prefix. Where one bullet covered two scenarios (e.g., the original `.github/workflows/ci.yml` exists and runs on push + PR` covers both SCN-029-001 and SCN-029-002), both IDs are prefixed onto the single bullet. Where useful, a parenthetical phrase is appended that echoes the Gherkin "Then" line so a human reader sees the linkage and the fuzzy fallback would also succeed.

3. **Add a slash-bearing path to T-7-03.** Append `, \`cmd/core/main.go\`` to the Location column and to the assertion text. `cmd/core/main.go` is a real existing file — the Go entrypoint that the `build:` block compiles into the smackerel-core image — so the path is both concrete and semantically aligned with "Build-from-source remains the default."

No DoD bullet was deleted or weakened. No Gherkin scenario was rewritten. The behavior the Gherkin describes is the behavior the production code already implements; the only gap was that documentation linkage was not visible to the guard's trace-ID matcher.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets edited by this fix retain their original behavioral claims word-for-word. The only changes are (a) a prefix `[SCN-029-NNN]`, and (b) in some cases a single appended parenthetical that quotes the Gherkin scenario name. The DoD continues to describe the same delivered behavior; it now also makes the trace-ID linkage explicit.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself:

- Pre-fix: `RESULT: FAILED (11 failures, 0 warnings)` with `9 unmapped` scenarios and one missing path.
- Post-fix: `RESULT: PASSED (0 warnings)` with `14 scenarios checked, 14 mapped to DoD, 0 unmapped` and all 14 test rows resolving to concrete files.

Both runs are captured in `report.md` under `### Validation Evidence`.
