# Execution Reports

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Reconciliation Analysis — 2026-04-21 (reconcile-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep  
**Mode:** reconcile-to-doc  
**Verdict:** CLEAN — zero drift between claimed and implemented state

### Claimed-vs-Implemented State

| Surface | Claimed (state.json / scopes.md) | Actual (codebase) | Verdict |
|---------|----------------------------------|--------------------|---------|
| state.json status | `done`, 9 scopes certified | All files exist, all tests pass | MATCH |
| Scope 01 — Config & Pipeline | Done | `config/smackerel.yaml` expenses section (line 48), `scripts/commands/config.sh` emits 16 env vars, `internal/config/config.go` ExpenseConfig struct | MATCH |
| Scope 02 — Receipt Detection | Done | `ml/app/receipt_detection.py` heuristics, `config/prompt_contracts/receipt-extraction-v1.yaml`, `ml/app/synthesis.py` imports receipt_detection (line 15) | MATCH |
| Scope 03 — Data Model & Migration | Done | `internal/domain/expense.go` structs, `internal/db/migrations/019_expense_tracking.sql` (vendor_aliases, expense_suggestions, expense_suggestion_suppressions, indexes) | MATCH |
| Scope 04 — Classification Engine | Done | `internal/intelligence/expenses.go` ExpenseClassifier with 7-level rule priority chain | MATCH |
| Scope 05 — Vendor Normalization | Done | `internal/intelligence/expenses.go` VendorNormalizer with LRU cache, `internal/intelligence/vendor_seeds.go` pre-seeded aliases | MATCH |
| Scope 06 — Expense API | Done | `internal/api/expenses.go` (List, Get, Export, Correct, Classify, AcceptSuggestion, DismissSuggestion), `internal/api/router.go` line 128 registers routes | MATCH |
| Scope 07 — CSV Export | Done | `internal/api/expenses.go` Export method with standard and QuickBooks format | MATCH |
| Scope 08 — Telegram Commands | Done | `internal/telegram/expenses.go` all 11 formats, `internal/telegram/bot.go` routes expense commands (line 319) | MATCH |
| Scope 09 — Digest Integration | Done | `internal/digest/expenses.go` ExpenseDigestSection, integrated in `internal/digest/generator.go` | MATCH |

### Wiring Integration Check

| Wiring Point | Previously Noted Gap | Current State | Verdict |
|-------------|---------------------|---------------|---------|
| `cmd/core/main.go` route registration | "routes not in cmd/core/main.go" | Lines 161–175: `ExpenseHandler` wired when `cfg.ExpensesEnabled` | RESOLVED |
| `ml/app/synthesis.py` receipt import | "synthesis.py not importing receipt_detection" | Line 15: `from .receipt_detection import detect_receipt_content` | RESOLVED |
| `internal/telegram/bot.go` command routing | "expense commands not routed in telegram/bot.go" | Line 319: `case "expense":` dispatches to `handleExpenseCommand` | RESOLVED |
| `internal/db/migrations/` migration file | "no dedicated migration file" | `019_expense_tracking.sql` exists with all tables and indexes | RESOLVED |

### Test Verification

| Category | Result |
|----------|--------|
| Go unit tests | All packages OK (cached) |
| Python unit tests | 236 passed, 3 warnings (unrelated to expenses) |
| Config check | Config in sync with SST, env_file drift guard OK |

### State Metadata Cleanup

| Item | Before | After |
|------|--------|-------|
| `state.json` notes | Listed 4 stale wiring gaps | Updated to reflect all gaps resolved |
| `executionHistory[implement].summary` | Referenced wiring gaps as open | Updated to confirm all wiring complete |
| `lastUpdatedAt` | 2026-04-19 | 2026-04-21 |

---

## Regression Analysis — 2026-04-21 (regression-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep  
**Mode:** regression-to-doc  
**Verdict:** CLEAN — zero findings

### Test Baseline

| Category | Result |
|----------|--------|
| Go unit tests | All passing (40+ packages, cached) |
| Python unit tests | 236 passed, 3 warnings (unrelated to expenses) |
| Lint | All checks passed |
| Format | 33 files unchanged |
| Config check | Config in sync, env_file drift guard OK |

### Cross-Spec Conflict Scan

| Surface | Specs Checked | Verdict |
|---------|--------------|---------|
| Route collisions (`/api/expenses/*`) | 025, 026, 027, 028, 035, 036 | CLEAN — properly namespaced, no overlaps |
| Shared JSONB metadata (`metadata ? 'expense'`) | All metadata-writing specs | CLEAN — dedicated `expense` key, no cross-contamination |
| ML sidecar synthesis pipeline (`ml/app/synthesis.py`) | 025, 026 | CLEAN — receipt detection is additive second pass, standard extraction unaffected |
| Config namespace (`EXPENSES_*` env vars) | All config-consuming specs | CLEAN — no prefix collisions |
| Telegram commands (`/expense`) | 027, 028, 035, 036 | CLEAN — no command name conflicts |
| Digest sections (`ExpenseDigestSection`) | 012, 025 | CLEAN — separate conditional producer |
| Router registration (`internal/api/router.go`) | All API-registering specs | CLEAN — guarded by `if deps.ExpenseHandler != nil` |

### Design Contradiction Check

| Concern | Analysis | Verdict |
|---------|----------|---------|
| Metadata-first pattern (expenses as artifact metadata) | Consistent with project architecture — all features store domain data in `metadata` JSONB | CLEAN |
| Amounts as strings (no float arithmetic) | Design explicitly mandates `CAST(...AS NUMERIC)` for PostgreSQL aggregation only | CLEAN |
| Receipt heuristic false positives on recipe/meal content | Heuristic is intentionally broad (best-effort), LLM extraction is the quality gate | CLEAN by design |
| Config SST enforcement | All 16 expense env vars flow from `smackerel.yaml` → config generate → env vars → Go parsing | CLEAN |

### Coverage

No coverage decrease detected. Expense-specific test files:
- `internal/config/validate_test.go` — config parsing + fail-loud
- `internal/api/expenses_test.go` — API endpoint validation
- `internal/intelligence/expenses_test.go` — classification engine
- `internal/digest/expenses_test.go` — digest section assembly + word limit
- `internal/telegram/expenses_test.go` — Telegram formatting
- `internal/domain/expense_test.go` — domain model
- `ml/tests/test_receipt_detection.py` — heuristic rules
- `ml/tests/test_receipt_extraction.py` — extraction schema validation

---

<!-- Report entries will be added below as scopes are implemented -->
