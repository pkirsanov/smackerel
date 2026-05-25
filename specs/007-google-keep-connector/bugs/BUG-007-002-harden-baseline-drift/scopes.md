# Scopes: BUG-007-002 — Harden baseline drift

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Close 13 state-transition-guard BLOCKs on spec 007 via additive artifact edits

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-FIX-007-002 State-transition-guard for spec 007 reports 0 BLOCKs
  Given specs/007-google-keep-connector/state.json.status is "done"
  And scopes.md contains additive Scenario Fidelity DoD items for SCN-GK-003, 005, 008, 010, 012, 019, 020, 024, 027, 030
  And report.md wraps the 2026-04-14 Improve-Existing Analysis Findings table and the Round-N Documentary Observations narrative with `<!-- bubbles:g040-skip-begin -->` / `<!-- bubbles:g040-skip-end -->` markers
  And at least one commit on the current branch carries the prefix `bubbles(007/bug-007-002-harden-baseline-drift)`
  When the workflow runs `bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector`
  Then the guard reports 0 BLOCKs
  And the guard does not emit "DoD-Gherkin content fidelity gap" for any SCN-GK-NNN
  And the guard does not emit "Report artifact contains N deferral language hit(s)"
  And the guard does not emit "full-delivery requires at least one structured commit message for spec 007"
```

### Implementation Plan

1. Append one Scenario Fidelity DoD item per failing scenario to the relevant scope in `specs/007-google-keep-connector/scopes.md`. Each item embeds `SCN-GK-NNN` + the full scenario title verbatim + an Evidence line citing the existing passing test.
2. Wrap the historical post-mortem narratives in `specs/007-google-keep-connector/report.md` with `<!-- bubbles:g040-skip-begin -->` / `<!-- bubbles:g040-skip-end -->` markers (per CHECK 18 escape hatch).
3. Run `bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector` and confirm 0 BLOCKs.
4. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and the bug folder.
5. Commit with prefix `bubbles(007/bug-007-002-harden-baseline-drift): ...` (resolves the commit-convention BLOCK).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-2-01 | state-transition-guard.sh PASS | artifact | `.github/bubbles/scripts/state-transition-guard.sh` | 0 BLOCKs against `specs/007-google-keep-connector` | SCN-GK-FIX-007-002 |
| T-FIX-2-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/007-google-keep-connector` | SCN-GK-FIX-007-002 |
| T-FIX-2-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/007-google-keep-connector/bugs/BUG-007-002-harden-baseline-drift` | SCN-GK-FIX-007-002 |
| T-FIX-2-04 | Locked scenarios unchanged | artifact | `git diff` | `state.json.certification.lockdownState.lockedScenarioIds` unchanged | SCN-GK-FIX-007-002 |
| T-FIX-2-05 | Underlying behavior tests still pass | unit | `internal/connector/keep/*_test.go`, `ml/tests/test_keep.py` | `./smackerel.sh test unit` exits 0 | SCN-GK-FIX-007-002 |

### Definition of Done

- [x] `specs/007-google-keep-connector/scopes.md` contains 10 new additive `SCN-GK-NNN` Scenario Fidelity DoD items (003, 005 in Scope 01; 008, 010 in Scope 02; 012, 030 in Scope 03; 019, 020 in Scope 04; 024 in Scope 05; 027 in Scope 06) — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -cE "^- \[x\] SCN-GK-(003|005|008|010|012|019|020|024|027|030)\b" specs/007-google-keep-connector/scopes.md
  > 10
  > ```
- [x] `specs/007-google-keep-connector/report.md` wraps both historical narratives with G040 skip-markers — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -cE "bubbles:g040-skip-(begin|end)" specs/007-google-keep-connector/report.md
  > 4
  > ```
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector` returns 0 BLOCKs after commit — **Phase:** validate
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/state-transition-guard.sh specs/007-google-keep-connector 2>&1 | grep -cE "^🔴 BLOCK"
  > 0
  > ```
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector` PASS — **Phase:** audit
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector >/dev/null && echo OK
  > OK
  > ```
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector/bugs/BUG-007-002-harden-baseline-drift` PASS — **Phase:** audit
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector/bugs/BUG-007-002-harden-baseline-drift >/dev/null && echo OK
  > OK
  > ```
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix — **Phase:** audit
  > Evidence:
  > ```
  > $ git diff --name-only HEAD~1 HEAD | grep -vE "^specs/007-google-keep-connector/" | grep -vE "^\.specify/memory/sweep-2026-05-24-r10\.json$" | wc -l
  > 0
  > ```
- [x] `state.json.certification.lockdownState.lockedScenarioIds` unchanged — **Phase:** audit
  > Evidence:
  > ```
  > $ git diff HEAD~1 HEAD -- specs/007-google-keep-connector/state.json | grep -E "lockedScenarioIds" | wc -l
  > 0
  > ```
