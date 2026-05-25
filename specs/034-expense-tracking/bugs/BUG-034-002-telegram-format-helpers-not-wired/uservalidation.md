# User Validation: BUG-034-002 — Telegram expense format helpers and fix-flow state machine not wired

## What was fixed

Documented a real spec-vs-code drift inside spec 034 (Expense Tracking):
the Telegram T-001..T-011 wireframe helpers and the multi-turn fix-flow
state machine exist in `internal/telegram/expenses.go` and are
test-covered, but they are NOT reached from the production Telegram
dispatch in `internal/telegram/bot.go`. Spec 034 Scope 08 DoD and
design.md narrative claim otherwise.

This is an **artifact-only fix**:

- 6-artifact bug packet authored under
  `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
- one prose `**Production-wiring status note (BUG-034-002,
  2026-05-25):**` block inserted under `## Scope 08: Telegram Expense
  Commands` in `specs/034-expense-tracking/scopes.md`
- one `bubbles.simplify` entry appended to
  `specs/034-expense-tracking/state.json` `executionHistory[]`
- `BUG-034-002-telegram-format-helpers-not-wired` appended to
  `specs/034-expense-tracking/state.json` `resolvedBugs[]` with
  `status: validated`
- round-7 entry appended to `.specify/memory/sweep-2026-05-25-r10.json`
  `rounds[]`

No production code, no tests, no scenario-manifest entries, no Test Plan
rows, no DoD checkboxes were modified. The real wiring (or alternative
deletion) is owned by spec 034 Scope 15 (`Not started`, gated on spec
037 LLM-agent migration).

## How to validate

1. Open `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
   and confirm all 6 artifacts are present (`spec.md`, `design.md`,
   `scopes.md`, `state.json`, `uservalidation.md`, `report.md`).
2. Open `specs/034-expense-tracking/scopes.md`, jump to
   `## Scope 08: Telegram Expense Commands`, and confirm a
   `**Production-wiring status note (BUG-034-002, 2026-05-25):**`
   paragraph appears between the existing `**Replacement:**` note and
   the `### Gherkin Scenarios` heading.
3. Open `specs/034-expense-tracking/state.json` and confirm:
   - `resolvedBugs[]` contains an entry with
     `"bugId": "BUG-034-002-telegram-format-helpers-not-wired"` and
     `"status": "validated"`.
   - `executionHistory[]` contains the appended `bubbles.simplify` round-7
     entry.
4. Run the four-guard sweep and confirm all PASS:

   ```bash
   bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking
   bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired
   bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking
   bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired
   bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking
   ```

5. Confirm boundary with `git diff --name-only HEAD~1` (after the fix
   commits land): only `specs/034-expense-tracking/scopes.md`,
   `specs/034-expense-tracking/state.json`,
   `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/*`,
   and `.specify/memory/sweep-2026-05-25-r10.json` should appear.

## Expected outcome

All guards PASS. The parent spec 034 status remains `done`. The bug is
recorded as `validated` (real production wiring deferred to Scope 15).
The Scope 08 status note ensures future readers see the wiring gap
explicitly and route it to Scope 15 rather than re-validating Scope 08
as fully delivered.

## Evidence references

See [report.md](report.md) for the verbatim guard output (with PII
redacted) and the boundary-preserved `git diff --name-only` evidence.

## Notes

The orphan helpers and state store are intentionally retained, not
deleted. Removing them would invalidate the Scope 08 DoD evidence trail
and the existing SCN-034-048..058 unit test surface, and would conflict
with Scope 15's reserved final disposition (which may either wire them
or replace them with agent-driven scenario outputs).

## Checklist

- [x] All 6 standard bug artifacts exist under
      `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
      (`spec.md`, `design.md`, `scopes.md`, `state.json`,
      `uservalidation.md`, `report.md`).
- [x] Parent `specs/034-expense-tracking/scopes.md` carries the
      `**Production-wiring status note (BUG-034-002, 2026-05-25):**`
      prose block under `## Scope 08: Telegram Expense Commands`,
      between the `**Replacement:**` block and the `### Gherkin
      Scenarios` heading.
- [x] Parent `specs/034-expense-tracking/state.json` `resolvedBugs[]`
      contains the BUG-034-002 entry with `"status": "validated"`.
- [x] Parent `specs/034-expense-tracking/state.json`
      `executionHistory[]` contains the appended `bubbles.simplify`
      round-7 entry referencing `sweep-2026-05-25-r10`.
- [x] `.specify/memory/sweep-2026-05-25-r10.json` `rounds[]` contains
      one round-7 entry with `bugId:
      BUG-034-002-telegram-format-helpers-not-wired`.
- [x] Four-guard sweep PASSES (or carries only the documented
      baseline BLOCKs already accepted by BUG-034-001 precedent):
      state-transition-guard (bug folder + parent), artifact-lint
      (bug folder + parent), traceability-guard (parent, 100/100
      baseline preserved).
- [x] `git diff --name-only HEAD` for the staged commit shows ONLY the
      four allowed surfaces (bug folder, parent scopes.md, parent
      state.json, sweep ledger); zero production code, test code,
      scenario-manifest, or design.md changes.
- [x] Final disposition (wire-or-delete the Telegram format helpers and
      fix-flow state machine) is explicitly assigned to spec 034 Scope
      15, which is `Not started` and gated on spec 037 LLM-agent
      migration.
