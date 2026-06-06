# BUG-036-002 — G068 fuzzy-tokenizer regression on spec 036 Done scopes

> **Parent feature:** [../../spec.md](../../spec.md) | [../../scopes.md](../../scopes.md) | [../../report.md](../../report.md)
> **Spec:** [spec.md](spec.md) | **Design:** [design.md](design.md) | **Scopes:** [scopes.md](scopes.md) | **Report:** [report.md](report.md)
> **Workflow Mode:** bugfix-fastlane (artifact-only fidelity playbook)
> **Discovered:** 2026-06-06 (stochastic-quality-sweep Round 12, `reconcile-to-doc`)

## Symptom

`bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`
returns `RESULT: FAILED (13 failures, 0 warnings)` on the certified-`done`
spec 036. All 13 failures are Gate G068 (Gherkin → DoD content fidelity):
12 Done-scope Gherkin scenarios have no faithful DoD item, plus the aggregate
summary line.

This diverges from the last recorded state (`state.json` executionHistory
Iter 11, 2026-05-09: `trace-guard ... RESULT: PASSED ... 56/56 DoD fidelity`).

## Affected scenarios (12)

| Scope | Scenarios |
|------:|-----------|
| 01 | SCN-036-003, SCN-036-005 |
| 02 | SCN-036-017 |
| 04 | SCN-036-030, SCN-036-037 |
| 05 | SCN-036-041 |
| 06 | SCN-036-044 |
| 07 | SCN-036-048, SCN-036-050 |
| 08 | SCN-036-053, SCN-036-054, SCN-036-055 |

## Root cause

The guard's `scenario_matches_dod` matches a scenario to a DoD item by
trace-ID first, then by fuzzy word overlap. The in-script v3.8.0 "G068
false-positive fix" tightened the tokenizer (`significant_words` min length
4→3; trimmed stop-word exclusion list). That shifted fuzzy scoring and flipped
12 previously fuzzy-passing, un-prefixed Done-scope scenarios to G068-unmapped.
BUG-036-001 had only prefixed the scenarios that were unmapped at its time, so
these 12 (which passed via fuzzy matching back then) never received a trace-ID
prefix and regressed when the tokenizer changed.

## Fix (artifact-only)

Tag each of the 12 covering DoD bullets in `specs/036-meal-planning/scopes.md`
with its `Scenario SCN-036-NNN (...)` trace ID (claim text preserved verbatim).
Reconcile 8 stale `_Not started._` per-scope stubs in
`specs/036-meal-planning/report.md`. No production code changed.

## Out of scope

- Production code under `internal/`, `cmd/`, `ml/`, `web/`.
- Spec 036 `status` / ceiling / scope-status semantics.
- Parked Scopes 09–15 (deferred to spec 037) — NOT force-closed.
- Framework scripts under `.github/bubbles/`.
