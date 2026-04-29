# Design: BUG-024-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [024 spec](../../spec.md) | [024 scopes](../../scopes.md) | [024 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 024 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the reconciled `docs/smackerel.md` content (PostgreSQL + pgvector storage diagrams and DDL, §3 architecture preservation, §19 phased plan with delivery markers, §22.7 15-connector inventory) but did not embed the `SCN-024-NN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for these four scenarios the DoD wording happened to fall below the threshold, so the gate failed.

Two ancillary problems accumulated under the same root: (1) `scenario-manifest.json` was never generated for spec 024 (G057/G059), and (2) the Manual Test Plan rows for SCN-024-03/04/05/06 described the verification action (e.g. "§3 mermaid diagrams unchanged") without including the `docs/smackerel.md` path token, so the concrete-test-file check could not resolve a path for those scenarios even though the file is the single deliverable of the entire spec.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only preferred. No production code changes" — is honored: gap analysis proved every behavior is delivered in `docs/smackerel.md`, so no production change is justified.

The fix has four parts:

1. **Trace-ID-bearing DoD bullets** added to parent `scopes.md`:
   - Scope 1 DoD gains two new bullets explicitly naming `SCN-024-02` and `SCN-024-03` with raw grep/awk evidence against `docs/smackerel.md` and source pointers (§8, §14, §3).
   - Scope 2 DoD gains two new bullets explicitly naming `SCN-024-05` and `SCN-024-06` with raw grep/find evidence against `docs/smackerel.md` and `internal/connector/` (§19, §22.7).

2. **Test Plan path token** added to existing Manual rows so the concrete-test-file check resolves `docs/smackerel.md` for SCN-024-03/04/05/06. The behavioral description is preserved verbatim — only the row text now begins with the path token.

3. **Scenario manifest** `specs/024-design-doc-reconciliation/scenario-manifest.json` is generated covering all 6 `SCN-024-*` scenarios. Each entry has `scenarioId`, `scope`, `requiredTestType`, `linkedTests` (with `file` + `function`), `evidenceRefs` (doc-section pointers + report cross-references), and `linkedDoD`.

4. **Report cross-reference** appended to `specs/024-design-doc-reconciliation/report.md` documenting the bug, the per-scenario classification, the pre-fix reproduction output, and the boundary statement.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — PostgreSQL storage, §3 preservation, §19 phased plan, §22.7 15-connector inventory are all genuinely delivered in the docs reconciliation) and only add the trace ID and raw verification evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the reconciled `docs/smackerel.md` carries; the only thing being fixed is the documentation linkage.

## Doc-only deferred-manual disposition

Spec 024 is a doc-only reconciliation spec; the entire deliverable is `docs/smackerel.md`. The trace-guard accepts `docs/smackerel.md` as a concrete test artifact (it exists, it is the audited evidence). No deferred-manual carve-out was needed: every scenario maps to a concrete grep/awk/find verification against `docs/smackerel.md` or `internal/connector/`. The user prompt's allowance for `deferred-manual with evidenceRefs` was not exercised because every scenario can be cleanly verified with a path-bearing Test Plan row plus a trace-ID-bearing DoD bullet.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (10 failures, 0 warnings)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence".
