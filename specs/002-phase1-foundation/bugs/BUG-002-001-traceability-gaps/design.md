# Design: BUG-002-001 — Traceability gaps in spec 002

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [002 spec](../../spec.md) | [002 scopes](../../scopes.md) | [002 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

Three independent root causes accumulated under one trace-guard failure surface:

1. **Manifest never re-generated.** `scenario-manifest.json` was generated on 2026-04-07 covering only the original 44 scenarios. Scopes 9–25 (improvement, system-review, gap-closure) were added later (2026-04-09 through 2026-04-12) and added 38 new Gherkin scenarios (`SCN-002-045`–`SCN-002-082`). The manifest was never updated. Gates G057/G059 caught this as `manifest covers only 44 scenarios but scopes define 82`.

2. **Report evidence references never recorded.** When scopes 14, 15, 18, and 19 were appended to `scopes.md` and the underlying test files were written under `internal/scheduler/`, `internal/auth/`, and `internal/connector/`, the evidence sections in `report.md` for these scopes (or any scope) never literally cited the test file paths. The traceability-guard's `report_mentions_path` is a `grep -Fq` literal-substring check; without the path string anywhere in `report.md` the scope fails even though the test file exists and passes. The check fires once per scenario in the affected scope (so scope 15 with 4 scenarios fires 4 times, etc.), giving the 10 raw failures.

3. **Test Plan row missing for SCN-002-080.** Scope 24 was added with two Gherkin scenarios but only one Test Plan row covering `SCN-002-079`. `SCN-002-080` (graceful OCR fallback on failure) was implemented (try/except in `_handle_artifact_process`) and exercised (`test_returns_empty_on_exception` in `ml/tests/test_ocr.py`) but never had a Test Plan row listing the existing test file path.

## Fix Approach (artifact-only)

Boundary clause from the user prompt: "artifact-only preferred. No production code changes." The gap analysis confirmed every behavior is delivered and tested, so no production change is justified.

The fix has three parts, all confined to `specs/002-phase1-foundation/*` and the bug folder:

1. **Extend `scenario-manifest.json`** to cover all 82 scenarios. The 38 new entries (`SCN-002-045`–`SCN-002-082`) follow the existing schema (`scenarioId`, `scope`, `requiredTestType`, `linkedTests`, `evidenceRefs`, `linkedDoD`) and point to test functions already on disk, taken directly from the Test Plan tables for scopes 9–25 in `scopes.md`.

2. **Append a "BUG-002-001 — Traceability Gaps" cross-reference section** to `report.md`. The section (a) classifies each of the 12 failures, (b) lists the concrete test files literally so `report_mentions_path` succeeds for scopes 14, 15, 18, 19, and (c) records raw before/after `traceability-guard` output. The same approach worked for BUG-009-001 and BUG-026-001.

3. **Insert one Test Plan row for SCN-002-080** at the top of the Scope 24 Test Plan table in `scopes.md`, mapping the scenario to `ml/tests/test_ocr.py::TestExtractTextTesseract::test_returns_empty_on_exception` (an in-process proxy that exercises the OCR-failure fallback path the scenario describes). Row position matters because the trace guard takes the first row whose trace ID matches the scenario.

## Why this is not "DoD rewriting"

Every Gherkin scenario flagged by Gate G068 was already authored against the delivered behavior — there are no DoD rewrites here. The fix only fills in linkage gaps (manifest entries, report file-path references, and one Test Plan row) so the guard's path/ID matchers can resolve. No DoD bullet was deleted or weakened, no Gherkin was edited, and no production code was touched.

## Regression Test

Because this is an artifact-only fix, the regression test is the traceability-guard itself. Pre-fix: `RESULT: FAILED (12 failures, 0 warnings)`. Post-fix: `RESULT: PASSED (0 warnings)`. The full guard output is captured inline in `report.md` under "Validation Evidence" with a `**Claim Source:** executed.` provenance tag.
