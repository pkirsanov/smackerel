# Bug: BUG-017-002 — Trace-guard G068 regression on SCN-GA-NWS-002 after framework v3.8.0 upgrade

## Classification

- **Type:** Artifact-only documentation/traceability bug (framework-upgrade regression)
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 017 — Government Alerts Connector
- **Workflow Mode:** bugfix-fastlane (sweep-2026-05-23-r30 Round 12 — regression-to-doc parent-expanded child workflow)
- **Status:** Fixed (artifact-only)

## Problem Statement

After the May 12, 2026 framework upgrade (commit `3037eb8c chore: upgrade Bubbles framework + prune stale workflow-runs`), `traceability-guard.sh` v3.8.0 tightened Gate G068 (Gherkin → DoD Content Fidelity):

- Significant-word length floor lowered from 4 → 3 chars (so 3-letter domain words like `NWS`, `API`, `DoD` now count).
- Stop-word list trimmed (so domain-relevant words like `severity`, `event`, `classification` count).

Under the new rule, scenario `SCN-GA-NWS-002 NWS severity and event classification` (significant words: `NWS`, `severity`, `event`, `classification` = 4 words; threshold = max(ceil(4/2), 3) = 3) was no longer matched by any existing Scope 03 DoD bullet:

- Bullet "Severity mapped from NWS categories to CAP standard" matches only `severity` + `NWS` = score 2 (FAIL).
- Bullet "Event types classified (tornado, hurricane, flood, winter storm, heat, etc.)" matches only `event` = score 1 (FAIL); `classified` does not normalize to `classification`.

BUG-017-001 (closed 2026-04-29 with PASS) had added trace-ID bullets for 12 of 13 scenarios but coalesced the SCN-GA-NWS-002 evidence across two non-prefixed bullets, which satisfied the old fuzzy matcher but not the v3.8.0 matcher.

## Reproduction (Pre-fix, HEAD `90554aca`)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector 2>&1 | tail -10
❌ Scope 03: NWS Weather Alerts Source Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-GA-NWS-002 NWS severity and event classification
ℹ️  DoD fidelity: 13 scenarios checked, 12 mapped to DoD, 1 unmapped
❌ DoD content fidelity gap: 1 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 13
ℹ️  Test rows checked: 13
ℹ️  DoD fidelity scenarios: 13 (mapped: 12, unmapped: 1)

RESULT: FAILED (2 failures, 0 warnings)
```

## Gap Analysis

| Scenario | Behavior delivered? | Tests pass? | Concrete test file(s) | Concrete source | DoD gap |
|---|---|---|---|---|---|
| SCN-GA-NWS-002 | Yes — NWS severity → CAP standard; event types classified | Yes — `TestMapNWSSeverity`, `TestClassifyNWSEventType` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::mapNWSSeverity`, `classifyNWSEventType` | Existing bullets describe behavior but lack scenario-ID prefix and lack significant-word overlap ≥3 required by G068 v3.8.0 |

**Disposition:** Delivered-but-undocumented under stricter rule — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/017-gov-alerts-connector/scopes.md` Scope 03 DoD contains a bullet explicitly prefixed `Scenario "SCN-GA-NWS-002 NWS severity and event classification":` that satisfies the trace-ID match path of G068
- [x] Existing Scope 03 DoD bullets (severity mapping, event classification, 17 unit tests) are preserved unchanged
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector` returns `RESULT: PASSED (0 warnings)` with `13/13 mapped`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector` PASSES
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression` PASSES
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` is modified
- [x] Underlying behavior tests still pass (`go test ./internal/connector/alerts/`)
