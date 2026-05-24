# Design: BUG-017-002 — Trace-guard G068 regression on SCN-GA-NWS-002

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [017 spec](../../spec.md) | [017 scopes](../../scopes.md) | [017 report](../../report.md)
> **Date:** May 24, 2026
> **Workflow Mode:** bugfix-fastlane (sweep-2026-05-23-r30 Round 12)

---

## Root Cause

`bubbles` framework upgrade on 2026-05-12 (commit `3037eb8c`) tightened `.github/bubbles/scripts/traceability-guard.sh` Gate G068:

- `significant_words()` length floor lowered 4 → 3 (so `NWS`, `API`, `CSV` count).
- Stop-word case list trimmed (so `severity`, `event`, `classification` count).
- `scenario_matches_dod()` fuzzy path requires `score >= 3 && score >= ceil(word_count/2)`.

For scenario `SCN-GA-NWS-002 NWS severity and event classification`:

- Significant words after normalize: `nws`, `severity`, `event`, `classification` (4 words; stop word `and` dropped).
- Required score: `max(ceil(4/2), 3) = 3`.

Scope 03 DoD bullets at HEAD `90554aca`:

| Bullet | Significant-word overlap with SCN-GA-NWS-002 | Score | Pass? |
|---|---|---|---|
| `Severity mapped from NWS categories to CAP standard` | `severity`, `nws` | 2 | NO (needs ≥3) |
| `Event types classified (tornado, hurricane, flood, winter storm, heat, etc.)` | `event` (`classified` ≠ `classification` after normalize) | 1 | NO |
| `17 unit tests pass …` | none | 0 | NO |

No bullet contained the trace ID `SCN-GA-NWS-002`, so the trace-ID fast path also failed. Result: Gate G068 reports `12/13 mapped, 1 unmapped`, RESULT FAILED.

The pre-regression baseline (BUG-017-001 closed 2026-04-29) passed under the older fuzzy matcher because (a) the word-length floor was 4 (so 3-letter `NWS` was stripped, reducing word_count and making the relative threshold easier to satisfy with smaller score), and (b) the stop list excluded several domain words.

## Fix Approach (artifact-only)

Insert a single new scenario-prefix DoD bullet in Scope 03 of `specs/017-gov-alerts-connector/scopes.md` between the existing `Event types classified ...` bullet and the `17 unit tests pass …` bullet:

```
- [x] Scenario "SCN-GA-NWS-002 NWS severity and event classification": NWS severity (Extreme/Severe/Moderate/Minor) and event_type (tornado, winter_storm, heat, etc.) are classified together for each NWS alert per the SCN-GA-NWS-002 example table
  > Evidence: `alerts.go::mapNWSSeverity()` + `alerts.go::classifyNWSEventType()` jointly satisfy SCN-GA-NWS-002; TestMapNWSSeverity (Extreme→extreme, Severe→severe, Moderate→moderate, Minor→minor, Unknown→minor) and TestClassifyNWSEventType (tornado, winter_storm, heat, hurricane, flood, thunderstorm, cold, fire) PASS in `internal/connector/alerts/alerts_test.go`
```

The new bullet satisfies G068 via the trace-ID fast path (`SCN-GA-NWS-002` literal in bullet text) and is **also** redundantly significant-word-faithful: `nws`, `severity`, `event`, `classification` all appear in the bullet text. Existing bullets are preserved unchanged. No DoD claim is weakened or rewritten.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." This fix:

- **Adds** one new bullet that faithfully captures the SCN-GA-NWS-002 behavior the Gherkin describes (severity classification + event-type classification together).
- **Preserves** all existing bullets (`Severity mapped from NWS categories to CAP standard`, `Event types classified (tornado, hurricane, flood, winter storm, heat, etc.)`, `17 unit tests pass …`) byte-identical.
- **Does not edit** any Gherkin scenario in the spec.
- **Does not edit** any production code, test code, scenario-manifest.json, or report.md (beyond a single cross-reference line added to log this BUG-017-002 close-out).

The behavior the Gherkin describes (joint severity + event-type classification for NWS alerts) is the behavior the production code implements (`mapNWSSeverity` + `classifyNWSEventType`); the only thing being fixed is the documentation linkage under the v3.8.0 G068 matcher.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix (HEAD `90554aca`) the guard reports `RESULT: FAILED (2 failures)` with `12/13 mapped`. Post-fix the guard reports `RESULT: PASSED (0 warnings)` with `13/13 mapped`. The before/after guard runs are captured in `report.md` under "Validation Evidence".

Underlying behavior tests are also re-run (`go test ./internal/connector/alerts/`) to confirm zero runtime regression from the simplify pass (R2) interacts with the documentation-only fix here: 175 tests still PASS, race-detector subset still clean.
