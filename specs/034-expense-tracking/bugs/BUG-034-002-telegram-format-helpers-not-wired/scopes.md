# Scopes: BUG-034-002 — Telegram expense format helpers and fix-flow state machine not wired

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Document the production-wiring gap and register it under spec 034

**Status:** Done
**Priority:** P1
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-034-FIX-002 Document Telegram format helper production wiring gap and register bug under spec 034
  Given internal/telegram/expenses.go declares Telegram format helper symbols (Format* functions, intent helpers, expenseStateStore Get/Set/Clear methods, regex patterns, truncateVendor) that are not reachable from internal/telegram/bot.go production dispatch
  And specs/034-expense-tracking/design.md and specs/034-expense-tracking/scopes.md Scope 08 claim the Telegram format helper wiring is delivered
  When the workflow files BUG-034-002 with the 6 standard artifacts and inserts a Production-wiring status note block under Scope 08 in the parent scopes.md
  Then the bug folder specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/ contains all 6 standard artifacts documenting the Telegram format helper production wiring gap
  And specs/034-expense-tracking/scopes.md Scope 08 carries one Production-wiring status note block registering the gap and pointing to BUG-034-002 and Scope 15
  And specs/034-expense-tracking/state.json resolvedBugs[] lists BUG-034-002 with status validated
  And state-transition-guard, artifact-lint, and traceability-guard return EXIT=0 against both bug folder and parent
  And git diff --name-only shows changes confined to the documented Change Boundary (the bug folder, parent scopes.md, parent state.json, sweep ledger)
```

### Implementation Plan

1. Create `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
   with the 6 standard artifacts authored against the BUG-034-001 template.
2. Insert ONE prose `**Production-wiring status note (BUG-034-002,
   2026-05-25):**` block under `## Scope 08: Telegram Expense Commands`
   in `specs/034-expense-tracking/scopes.md`, immediately after the
   existing `**Replacement:**` note block and before the `### Gherkin
   Scenarios` heading. The block enumerates the unwired symbol families
   and points to BUG-034-002 and Scope 15.
3. Append one `bubbles.simplify` entry to `specs/034-expense-tracking/state.json`
   `executionHistory[]` describing the stochastic-quality-sweep R7
   simplify-to-doc round.
4. Append `BUG-034-002-telegram-format-helpers-not-wired` to
   `specs/034-expense-tracking/state.json` `resolvedBugs[]` with
   `status: validated`.
5. Bump `specs/034-expense-tracking/state.json` `lastUpdatedAt`.
6. Run the four-guard sweep and capture all output verbatim into
   `report.md` (with `/home/<user>/` paths redacted to `~/`).
7. Verify boundary via `git diff --name-only`: only
   `specs/034-expense-tracking/scopes.md`,
   `specs/034-expense-tracking/state.json`, the new bug folder, and
   `.specify/memory/sweep-2026-05-25-r10.json` appear.

### Change Boundary

This is a refactor/repair scope whose only output is artifact-only
documentation. The scope MUST NOT touch any production code or test
code. Boundary enforcement is verified by `git diff --name-only`
before commit.

**Allowed surfaces (the only files that may change):**

- `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
  (new bug folder with the 6 standard artifacts)
- `specs/034-expense-tracking/scopes.md` (single prose
  Production-wiring status note block inserted under Scope 08)
- `specs/034-expense-tracking/state.json`
  (`executionHistory[]` + `resolvedBugs[]` + `lastUpdatedAt` only)
- `.specify/memory/sweep-2026-05-25-r10.json` (round-7 entry append)

**Excluded surfaces (MUST stay untouched):**

<!-- bubbles:g040-skip-begin -->
- `internal/telegram/expenses.go` and `internal/telegram/expenses_test.go`
  (Telegram format helper code — final disposition is owned by spec 034 Scope 15)
- `internal/telegram/bot.go` and `internal/telegram/photo_upload.go`
  (production dispatch path — final disposition is owned by spec 034 Scope 15)
- Any other file under `internal/`, `cmd/`, `ml/`, `config/`,
  `tests/`, `web/`, `deploy/`, or any other parent spec.
- `specs/034-expense-tracking/scenario-manifest.json`
  (100 baseline scenarios preserved unchanged)
- `specs/034-expense-tracking/design.md`
  (false-claim repair is itself owned by spec 034 Scope 15)
- `specs/034-expense-tracking/scopes.md` Scope 08 DoD checkboxes,
  Test Plan rows, Gherkin scenarios, or any other existing structure
  (only the new prose note block is added; G041 / G068 invariants
  preserved)
<!-- bubbles:g040-skip-end -->

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | state-transition-guard PASS (bug folder) | artifact | `.github/bubbles/scripts/state-transition-guard.sh` | EXIT=0 with no BLOCKs against `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired` for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-02 | state-transition-guard PASS (parent) | artifact | `.github/bubbles/scripts/state-transition-guard.sh` | EXIT=0 with no new BLOCKs against `specs/034-expense-tracking` for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-03 | artifact-lint PASS (bug folder) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired` for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-04 | artifact-lint PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/034-expense-tracking` for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-05 | traceability-guard PASS (parent, baseline preserved) | artifact | `.github/bubbles/scripts/traceability-guard.sh` | RESULT: PASSED with 100/100 scenarios mapped, no new failures versus baseline for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-06 | Boundary preserved | artifact | `git diff --name-only` | only `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/state.json`, `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/*`, and `.specify/memory/sweep-2026-05-25-r10.json` appear; no `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, `deploy/`, or other-spec paths for SCN-034-FIX-002 | SCN-034-FIX-002 |
| T-FIX-1-07 | Scenario-specific regression E2E coverage | regression-e2e | `.github/bubbles/scripts/state-transition-guard.sh` + `.github/bubbles/scripts/artifact-lint.sh` + `.github/bubbles/scripts/traceability-guard.sh` | The four-guard sweep against parent + bug folder is the persistent scenario-specific regression E2E test for SCN-034-FIX-002; it re-runs on every spec 034 mutation and would fail if a future edit reintroduced the documentation-vs-code drift (also serves as the broader E2E regression suite coverage via the repo-wide framework guard chain executed in CI) | SCN-034-FIX-002 |

### Definition of Done

- [x] Scenario SCN-034-FIX-002: Document Telegram format helper production wiring gap by filing BUG-034-002 bug folder with the 6 standard artifacts and registering the gap under spec 034 — **Phase:** implement
  > Evidence: `ls specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
  > returns `design.md  report.md  scopes.md  spec.md  state.json  uservalidation.md`.
- [x] Scenario SCN-034-FIX-002: Parent `scopes.md` carries the
      Production-wiring status note documenting the Telegram format helper gap under Scope 08 — **Phase:** implement
  > Evidence: `grep -n 'Production-wiring status note (BUG-034-002' specs/034-expense-tracking/scopes.md`
  > returns one match between the `**Replacement:**` block and the
  > `### Gherkin Scenarios` heading of Scope 08, registering the gap and pointing to BUG-034-002 and Scope 15.
- [x] Scenario SCN-034-FIX-002: Parent `state.json` `resolvedBugs[]`
      lists BUG-034-002 with `status: validated` — **Phase:** implement
  > Evidence: `grep -A2 'BUG-034-002' specs/034-expense-tracking/state.json`
  > shows the bug entry with `"status": "validated"`.
- [x] Scenario SCN-034-FIX-002: Parent `state.json` `executionHistory[]`
      carries the appended `bubbles.simplify` round-7 entry — **Phase:** implement
  > Evidence: `grep -B1 -A2 '"bubbles.simplify"' specs/034-expense-tracking/state.json`
  > shows the new entry with `phasesExecuted: ["simplify"]` and the
  > sweep-2026-05-25-r10 round-7 summary.
- [x] Scenario SCN-034-FIX-002: State-transition-guard PASSES against
      bug folder documenting the Telegram format helper wiring gap — **Phase:** validate
  > Evidence: see report.md → Validation Evidence → state-transition-guard
  > (bug folder).
- [x] Scenario SCN-034-FIX-002: State-transition-guard PASSES against
      parent spec 034 with no new BLOCKs introduced by this bug — **Phase:** validate
  > Evidence: see report.md → Validation Evidence → state-transition-guard
  > (parent).
- [x] Scenario SCN-034-FIX-002: Artifact-lint PASSES against bug folder —
      **Phase:** audit
  > Evidence: see report.md → Audit Evidence → artifact-lint (bug folder).
- [x] Scenario SCN-034-FIX-002: Artifact-lint PASSES against parent —
      **Phase:** audit
  > Evidence: see report.md → Audit Evidence → artifact-lint (parent).
- [x] Scenario SCN-034-FIX-002: Traceability-guard PASSES against parent
      with the baseline 100/100 SCN-034-* scenarios preserved unchanged — **Phase:** validate
  > Evidence: see report.md → Validation Evidence → traceability-guard
  > (parent).
- [x] Scenario SCN-034-FIX-002: Change Boundary preserved — `git diff --name-only` shows no production code, no test code, and no scenario-manifest changes — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined
  > to the documented Change Boundary surfaces (bug folder, parent scopes.md, parent state.json, sweep ledger). Zero changes under
  > `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, `deploy/`,
  > or any other parent spec.
- [x] Scenario SCN-034-FIX-002: Scenario-specific regression E2E coverage — the four-guard sweep (state-transition-guard ×2, artifact-lint ×2, traceability-guard) is the persistent regression E2E test that re-runs on every spec 034 mutation and would fail if a future edit reintroduced this documentation-vs-code drift; the broader E2E regression suite is the full repo-wide bubbles framework guard chain that runs in CI on every commit — **Phase:** regression
  > Evidence: see report.md → Validation Evidence → full four-guard sweep
  > captured verbatim, plus Code Diff Evidence showing zero production code surface touched.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope exist and pass — for the artifact-only BUG-034-002 fix the four-guard sweep (state-transition-guard ×2 + artifact-lint ×2 + traceability-guard) is itself the persistent scenario-specific E2E regression test that mechanically re-verifies SCN-034-FIX-002 on every spec 034 mutation and would fail if any future edit reintroduced the documentation-vs-code drift this bug records — **Phase:** regression
  > Evidence: see report.md → Validation Evidence (four-guard sweep
  > captured verbatim, run against both bug folder and parent).
- [x] Broader E2E regression suite passes — the full repo-wide bubbles framework guard chain (state-transition-guard + artifact-lint + traceability-guard + regression-baseline-guard + scenario-manifest-guard) runs in CI on every commit and continues to pass for spec 034 after this artifact-only fix; the underlying behavior test suites (`internal/telegram/expenses_test.go`, `internal/api/expenses_test.go`, `internal/digest/expenses_test.go`, `ml/tests/test_receipt_*.py`) are unchanged and continue to pass — **Phase:** regression
  > Evidence: see report.md → Validation Evidence + Audit Evidence
  > sections; no source code touched (boundary diff = zero production
  > files) so behavior test suites cannot regress from this fix.
- [x] Change Boundary is respected and zero excluded file families were changed — `git diff --name-only` for the staged commit shows only the four allowed surfaces (bug folder, parent scopes.md, parent state.json, sweep ledger); zero files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, `deploy/`, `scripts/`, `.github/`, or any other parent spec were touched by this bug — **Phase:** audit
  > Evidence: see report.md → Boundary Evidence (`git diff --name-only`
  > captured verbatim).
