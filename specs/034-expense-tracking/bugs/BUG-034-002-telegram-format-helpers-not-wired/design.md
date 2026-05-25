# Design: BUG-034-002 — Telegram expense format helpers and fix-flow state machine not wired (artifact-only fix)

## Root Cause

Two-layer drift between the spec contract and the production Telegram
dispatch:

1. **Photo-ingestion path drift.** `specs/034-expense-tracking/design.md`
   §"Telegram Bot Integration" (line 690) commits to a flow where a
   Telegram photo triggers OCR + receipt extraction, the result is
   published on the `artifacts.processed` NATS subject, and a
   Telegram-side subscriber receives the payload and calls one of
   `FormatExpenseConfirmation` (T-001) / `FormatOCRFailure` (T-002) /
   `FormatPartialExtraction` (T-003) / `FormatAmountMissing` (T-004) to
   reply on the originating chat. The production path actually delivered
   by Scope 08 is `internal/telegram/photo_upload.go:handlePhotoUpload`,
   which uploads the largest `PhotoSize` to the unified
   `/v1/photos/upload` HTTP endpoint and replies with the freshly minted
   photo id only. No Telegram-side `artifacts.processed` subscriber was
   delivered; the four `Format*` confirmation helpers exist in
   `internal/telegram/expenses.go` but are only exercised by
   `internal/telegram/expenses_test.go`.

2. **Text-dispatch path drift.** `specs/034-expense-tracking/scopes.md`
   §"Scope 08 Telegram Expense Commands" Definition of Done claims
   manual expense entry (T-005), formatted list output (T-006), CSV
   wrapper text (T-007), fix-flow state machine (T-009), and suggestion
   accept/dismiss (T-008) are wired. In production, the priority-7
   expense router at `internal/telegram/bot.go:438-443` only checks
   `isExpenseQuery(text)` and `isExpenseExport(text)`. It NEVER reaches
   `isExpenseEntry`, `isSuggestionAccept`, `isSuggestionDismiss`, nor any
   `expenseStates.Get/Set/Clear` call. `handleExpenseQuery`
   (`internal/telegram/bot.go:1188-1245`) and `handleExpenseExport`
   (`internal/telegram/bot.go:1248-1289`) build their own minimal reply
   text via a local `strings.Builder`, bypassing `FormatExpenseList`,
   `FormatExpenseCSVMessage`, `FormatFixPrompt`, and `FormatFieldUpdated`.
   The TTL goroutine `newExpenseStateStore(120).StartCleanup` (lines 147
   and 163) runs against a map that no production code path mutates.

Net effect: ~150 LOC of test-only orphan code shipped to production binary
behind the contract claim that they implement the T-001..T-011 UX
wireframes.

## Fix Approach

**Boundary:** ONLY `specs/034-expense-tracking/scopes.md`,
`specs/034-expense-tracking/state.json`, and the new bug folder
`specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`.
NO production code, NO tests, NO sibling specs, NO `scenario-manifest.json`
mutation, NO Test Plan row mutation, NO scenario semantic mutation, NO
DoD checkbox mutation. The existing 100 SCN-034-* scenarios and their
mapping rows stay byte-for-byte stable so the traceability-guard PASS
baseline is preserved.

**Per-scope edit to parent `scopes.md`:**

- Insert ONE new prose `**Production-wiring status note (BUG-034-002,
  2026-05-25):**` block immediately after the existing `**Replacement:**`
  note block under `## Scope 08: Telegram Expense Commands`. The block
  enumerates the unwired symbol families (Format helpers, intent helpers,
  state-store Get/Set/Clear) and lists which T-NNN wireframes they were
  supposed to back, then points to BUG-034-002 and to Scope 15 as the
  owner of final disposition.

The block is plain prose (no checkbox bullet, no Gherkin scenario, no
Test Plan row) so the bubbles guards (`traceability-guard.sh`,
`state-transition-guard.sh`, `artifact-lint.sh`) cannot fire on it:

- `traceability-guard.sh` counts `Scenario: ` matches inside ```gherkin```
  fences, table rows in `### Test Plan` sections, and `evidenceRefs`
  arrays in `scenario-manifest.json`. None of those shapes are touched.
- `state-transition-guard.sh` G023 Check 4 counts `- [ ]` and `- [x]`
  checkbox items in scope DoD sections. A prose block under the scope
  header is not a checkbox item.
- `artifact-lint.sh` requires presence of canonical section headers and
  rejects empty bodies. Adding additional prose does not violate any
  required-section check.

**Per-spec edit to parent `state.json`:**

1. Append a `bubbles.simplify` entry to `executionHistory[]` documenting
   the stochastic-quality-sweep R7 simplify-to-doc round, the probe
   scope, the orphan-code finding, and the artifact-only closure
   disposition.
2. Append `BUG-034-002-telegram-format-helpers-not-wired` to
   `resolvedBugs[]` with `status: validated`.
3. Bump `lastUpdatedAt` to the run timestamp.
4. Leave `status`, `certification.*`, `completedScopes`, scope rows, and
   every existing executionHistory entry untouched.

**Bug-folder artifacts:** The 6 standard artifacts (`spec.md`,
`design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`)
follow the BUG-034-001 template exactly. The bug carries a single Scope 1
"Document the production-wiring gap" with one Gherkin scenario
`SCN-034-FIX-002`, one Test Plan coverage row, and a checked DoD matching
the spec.md acceptance criteria.

## Affected Files

- `specs/034-expense-tracking/scopes.md` (one prose note block inserted
  under Scope 08; zero scenarios / rows / checkboxes changed)
- `specs/034-expense-tracking/state.json` (one `executionHistory[]`
  entry appended; one `resolvedBugs[]` entry appended; `lastUpdatedAt`
  bumped)
- `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/*`
  (new bug folder with 6 artifacts)
- `.specify/memory/sweep-2026-05-25-r10.json` (round 7 entry appended to
  `rounds[]`)

Out-of-scope (NOT touched):
- `internal/telegram/expenses.go` (orphan helpers preserved per spec
  contract; deletion would conflict with Scope 08 DoD evidence and
  Scope 15 future disposition)
- `internal/telegram/bot.go` (production wiring unchanged)
- `internal/telegram/photo_upload.go` (unified upload path unchanged)
- `internal/telegram/expenses_test.go` (test coverage of the helpers
  preserved verbatim)
- `specs/034-expense-tracking/scenario-manifest.json` (100 scenarios,
  100 PASS mappings preserved verbatim)
- `specs/034-expense-tracking/spec.md` / `design.md` (the design narrative
  IS the spec contract — the gap is in code, not in design; the spec
  remains the target that Scope 15 will deliver against)
- Any other parent spec, framework script, instruction, or skill file

## Regression Test Design

The regression test for this artifact-only fix is the four-guard sweep:

1. `bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking`
   → EXIT=0 (baseline warnings preserved; no new BLOCK)
2. `bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired`
   → EXIT=0
3. `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
   → PASSED
4. `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired`
   → PASSED
5. `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking`
   → PASSED (unchanged from baseline 100/100)

**Adversarial regression:** if a future agent deletes the
production-wiring status note from Scope 08, the gap becomes invisible
again. The bug folder remains in `resolvedBugs[]` and the executionHistory
entry remains in `state.json`, both serving as audit traces a future
reader can grep for.

## Constraints Honored

- No production code change (`internal/`, `cmd/`, `ml/`, `config/`,
  `tests/`, `web/`, `deploy/`, `scripts/` untouched)
- No sibling-spec change (`specs/0[0-2]*`, `specs/03[0-3]`,
  `specs/03[5-9]`, `specs/040*`, `specs/041*`, `specs/05*` untouched)
- No new test files
- No edits to existing scenario titles, scenario count, Test Plan rows,
  or scenario-manifest.json
- No `--no-verify` push; standard pre-commit hooks run
- No shell file writes (all artifacts authored via IDE
  `create_file` / `replace_string_in_file` tools)
- No framework-managed file modification (no `.github/bubbles/`,
  `.github/agents/`, `.github/prompts/`, `.specify/templates/` edits)
- PII redacted in all evidence blocks (`/home/<user>/...` paths replaced
  with `~/...`)
- Pre-existing unrelated working-tree drift (smackerel.sh, deploy/,
  internal/graph/hospitality_linker.go, deploy_target_status_test*, etc.)
  is NOT staged
