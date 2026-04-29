# Design: BUG-007-001 — Traceability gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [007 spec](../../spec.md) | [007 scopes](../../scopes.md) | [007 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 007 was extended with an extra Gherkin scenario `SCN-GK-030` ("Recently-archived note gets light tier despite recency") as part of a prior H-R2-001 hardening fix that moved the `IsArchived` check before the recency check in `internal/connector/keep/qualifiers.go::Evaluate` and added `TestQualifierRecentArchivedGetsLight` to `qualifiers_test.go`. The Gherkin scenario, the Test Plan row `T-3-08b`, the DoD bullet, and the supporting test were all added — but `specs/007-google-keep-connector/scenario-manifest.json` was not regenerated. It retained the original 29 entries (`SCN-007-001` … `SCN-007-029`) while scopes defined 30.

The traceability-guard's count check (`scenario-manifest.json covers only N scenarios but scopes define M`) compares raw counts, so the one-row gap caused a single guard failure on an otherwise fully traceable feature.

## Fix Approach (artifact-only)

Append a single 30th entry `SCN-007-030` to `specs/007-google-keep-connector/scenario-manifest.json`. The entry mirrors the structure of the existing 29 entries:

- `scenarioId: "SCN-007-030"`
- `name: "Recently-archived note gets light tier despite recency (SCN-GK-030)"`
- `scope: "03-source-qualifiers-processing-tiers"`
- `linkedTests`: `internal/connector/keep/qualifiers_test.go::TestQualifierRecentArchivedGetsLight` (already exists, already passing)
- `evidenceRefs`: same test plus source `internal/connector/keep/qualifiers.go::Evaluate`
- `linkedDoD`: phrasing that matches the existing Scope 3 DoD bullet

No other artifact is touched. No production code is changed. The boundary clause from the user prompt — "artifact-only preferred. No production code changes." — is honored.

## Why this is not "DoD rewriting"

The Gherkin scenario `SCN-GK-030`, the Test Plan row `T-3-08b`, the DoD bullet, the regression test, and the production behavior all already exist and are already PASS. The fix only registers the existing scenario in the manifest. No DoD bullet is added or modified, no scenario wording is changed.

## Regression Test

Because this fix is artifact-only and adds no executable code, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (1 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The underlying behavior test `TestQualifierRecentArchivedGetsLight` continues to pass and serves as the long-term regression for the SCN-GK-030 behavior.
