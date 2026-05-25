# Bug: BUG-034-002 — Telegram expense format helpers and fix-flow state machine not wired in production

## Classification

- **Type:** Artifact-only documentation / spec-vs-code drift bug
- **Severity:** MEDIUM (no runtime regression — the production bot still serves
  `/expense` query and export via minimal inline reply text and serves photos
  via the unified `/v1/photos/upload` path; what is missing is the polished
  T-001..T-005 OCR-confirmation flow and the T-008..T-009 multi-turn fix-flow
  state machine the spec design promises. Scope 08 DoD evidence and design.md
  describe these as wired; the production code only references them from tests.)
- **Parent Spec:** 034 — Expense Tracking
- **Workflow Mode:** simplify-to-doc (artifact-only closure within
  stochastic-quality-sweep R7 / sweep-2026-05-25-r10)
- **Status:** Fixed (artifact-only documentation closure; real production
  wiring deferred to Scope 15 of spec 034, which is gated on spec 037
  `037-llm-agent-tools` and currently `Not started`)

## Problem Statement

A `bubbles.simplify` probe against spec 034 surfaces a large body of orphan
code in `internal/telegram/expenses.go` (~150 LOC across exported format
helpers, intent-detection helpers, an in-memory conversation state store,
and regex/pattern globals). Every one of these symbols is referenced ONLY
from `internal/telegram/expenses_test.go`. None of them are reachable from
the production Telegram dispatch path in `internal/telegram/bot.go`.

The drift between spec contract and production code:

1. **Spec design.md (line 690) claims:** "Photo messages: If a photo is
   received and OCR produces text, the receipt extraction pipeline runs.
   The Telegram handler listens for the `artifacts.processed` NATS response
   and sends the T-001/T-002/T-003/T-004 confirmation format."
   **Production code:** No `artifacts.processed` subscriber exists in
   `internal/telegram/`. Photos are routed to `handlePhotoUpload`
   (`internal/telegram/photo_upload.go`) which posts to the unified
   `/v1/photos/upload` endpoint and replies with the freshly minted photo
   id only — never invoking `FormatExpenseConfirmation`, `FormatOCRFailure`,
   `FormatPartialExtraction`, or `FormatAmountMissing`.

2. **Spec design.md (line 712) claims:** "All Telegram responses follow the
   UX spec (T-001 through T-011). The format functions live in
   `internal/telegram/expenses.go`."
   **Production code:** The two wired production handlers,
   `handleExpenseQuery` (`internal/telegram/bot.go:1188-1245`) and
   `handleExpenseExport` (`internal/telegram/bot.go:1248-1289`), build
   their reply text via a local `strings.Builder` with a minimal inline
   format. Neither calls `FormatExpenseList` (T-006) nor
   `FormatExpenseCSVMessage` (T-007).

3. **Spec scopes.md Scope 08 DoD claims:** "Conversation state management
   with TTL for multi-turn fix flow and amount prompts"; "Fix flow presents
   fields, accepts corrections, and terminates on 'done'"; "Suggestion
   accept/dismiss works via natural language chat commands".
   **Production code:** The bot.go priority-7 expense router at
   `internal/telegram/bot.go:438-443` only checks `isExpenseQuery(text)`
   and `isExpenseExport(text)`. It NEVER checks `isExpenseEntry`,
   `isSuggestionAccept`, or `isSuggestionDismiss`. The `expenseStateStore`
   methods `Get`, `Set`, and `Clear` are never called from production —
   only `newExpenseStateStore` (line 147) and `StartCleanup` (line 163)
   are invoked. The TTL machinery runs against an empty map forever.

The functions, helpers, and state store exist; their tests exist and pass;
but the production message dispatch in `internal/telegram/bot.go` does not
reach them.

## Reproduction (probe evidence)

```
$ grep -RIn 'FormatExpense\|FormatOCRFailure\|FormatPartialExtraction\|FormatAmountMissing\|FormatExpenseList\|FormatExpenseCSVMessage\|FormatFixPrompt\|FormatFieldUpdated' internal/ | grep -v 'expenses_test.go\|expenses.go:'
# (no production callers — only test-file references)

$ grep -RIn 'isExpenseEntry\|isSuggestionAccept\|isSuggestionDismiss' internal/ | grep -v 'expenses_test.go\|expenses.go:'
# (no production callers — only test-file references)

$ grep -RIn 'expenseStates\.\(Get\|Set\|Clear\)' internal/
# (no matches in production — only StartCleanup is called from bot.go:163)

$ grep -RIn 'artifacts\.processed' internal/telegram/
# (no NATS subscriber in internal/telegram/ — design.md L690 contract unmet)
```

The production wiring evidence:

```
$ sed -n '438,443p' internal/telegram/bot.go
# priority-7 expense router checks ONLY isExpenseQuery + isExpenseExport

$ sed -n '1188,1245p' internal/telegram/bot.go
# handleExpenseQuery builds reply via local strings.Builder, not FormatExpenseList

$ sed -n '1248,1289p' internal/telegram/bot.go
# handleExpenseExport sends CSV document directly, not via FormatExpenseCSVMessage
```

## Gap Analysis

**Affected dead-in-production symbols in `internal/telegram/expenses.go`:**

| Symbol | Kind | Lines | Production callers | Test callers |
|--------|------|-------|--------------------|--------------|
| `FormatExpenseConfirmation` | exported func | 159 | 0 | `expenses_test.go:25,347,428` |
| `FormatOCRFailure` | exported func | 181 | 0 | `expenses_test.go:49` |
| `FormatPartialExtraction` | exported func | 186 | 0 | `expenses_test.go:60` |
| `FormatAmountMissing` | exported func | 198 | 0 | `expenses_test.go:80` |
| `FormatExpenseList` | exported func | 203 | 0 | `expenses_test.go:104,125,389` |
| `FormatExpenseCSVMessage` | exported func | 234 | 0 | `expenses_test.go:133,147` |
| `FormatFixPrompt` | exported func | 243 | 0 | `expenses_test.go:155` |
| `FormatFieldUpdated` | exported func | 262 | 0 | `expenses_test.go:178` |
| `isExpenseEntry` | helper | 123 | 0 | `expenses_test.go:217,220,223` |
| `isSuggestionAccept` | helper | 133 | 0 | `expenses_test.go:229,232` |
| `isSuggestionDismiss` | helper | 139 | 0 | `expenses_test.go:238,241` |
| `expenseStateStore.Get/Set/Clear` | methods | 269-329 | 0 | `expenses_test.go:269,288-329` |
| `expenseAmountReplyPattern` | regexp | 97 | 0 | `expenses_test.go:259` |
| `expenseEntryAmountRe` | regexp | 120 | 0 | (used only by `isExpenseEntry`) |
| `truncateVendor` | helper | 152 | 0 (transitively) | (used only by `Format*` family) |
| `maxTelegramVendorLen` | const | 149 | 0 (transitively) | (used only by `truncateVendor`) |

**Live in production** (retained without changes):
- `isExpenseQuery` (line 105) → `internal/telegram/bot.go:438`
- `isExpenseExport` (line 118) → `internal/telegram/bot.go:442,1179`
- `newExpenseStateStore` (line 271) → `internal/telegram/bot.go:147`
- `expenseStateStore.StartCleanup` (line 281) → `internal/telegram/bot.go:163`
- `expenseStateStore.Stop` (line 296) → tests only (kept as a graceful-shutdown
  hook for the goroutine started by `StartCleanup`)

**Disposition:** the finding is real spec-vs-code drift, NOT a simplify
opportunity to delete:

1. The unwired symbols are explicitly listed in Scope 08 DoD evidence
   (`scopes.md:992,1006`) as "implemented".
2. Spec 034 Scope 15 (`Not started`, depends on spec 037 LLM agent + tools)
   is the migration that owns final disposition: it will EITHER replace
   the format helpers with agent-driven scenario outputs OR explicitly
   wire the existing helpers into an `expense.intent_route-v1` consumer.
3. Deleting the helpers now would invalidate the Scope 08 DoD evidence
   trail and the SCN-034-048..058 test surface.
4. Wiring the helpers now would re-design the photo-upload path and the
   NATS receipt-extraction subscriber — that is feature work, not a
   simplify trigger response.

The correct disposition is to **document the gap explicitly** under
spec 034 so that:

- Future agents reading Scope 08 DoD know the helpers are tested but
  unwired in production.
- The Scope 15 migration carries an explicit obligation either to wire
  or to delete the orphan code.
- The Scope 08 status row is no longer misleading.

## Acceptance Criteria

- [x] `specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired/`
      contains the 6 standard bug artifacts (`spec.md`, `design.md`,
      `scopes.md`, `state.json`, `uservalidation.md`, `report.md`)
- [x] `specs/034-expense-tracking/scopes.md` Scope 08 carries a new
      `**Production-wiring status note (BUG-034-002, 2026-05-25):**` block
      under the existing `**Replacement:**` block, documenting which
      Scope 08 DoD claims correspond to symbols that exist as tested code
      but are not reached from `internal/telegram/bot.go`
- [x] `specs/034-expense-tracking/state.json` `resolvedBugs[]` lists
      `BUG-034-002-telegram-format-helpers-not-wired` with
      `status: validated`
- [x] `specs/034-expense-tracking/state.json` `executionHistory[]` appends
      one entry from `bubbles.simplify` documenting the
      stochastic-quality-sweep R7 simplify-to-doc round
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking`
      returns EXIT=0 (no new BLOCKs introduced)
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired`
      returns EXIT=0
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
      PASSES
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-002-telegram-format-helpers-not-wired`
      PASSES
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking`
      PASSES (unchanged from baseline; this fix adds no new scenarios and
      does not alter Test Plan tables or scenario-manifest entries)
- [x] No production code changed (boundary preserved — only
      `specs/034-expense-tracking/scopes.md`,
      `specs/034-expense-tracking/state.json`, and the new bug folder)
