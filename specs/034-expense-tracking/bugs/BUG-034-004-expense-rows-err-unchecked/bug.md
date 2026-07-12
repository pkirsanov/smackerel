# BUG-034-004: Expense DB-iteration loops swallow mid-stream errors — under-reported totals / truncated tax-export CSV returned as HTTP 200

**Status:** Done (engineering fix complete and unit-verified; central commit performed by the orchestrator)
**Fix summary:** All 11 loops hardened to the repo's `rows.Err()` convention; API List/Export `Scan`/`Unmarshal` `continue`→error propagation (List → HTTP 500, Export stream → `http.ErrAbortHandler`); digest/intelligence loops propagate errors. Adversarial regression in `internal/api/expenses_rowserr_test.go` (RED-on-reintroduction proven). See `report.md`.
**Severity:** Medium (financial-data correctness; silent truncation)
**Reported:** 2026-06-15 (stochastic-quality-sweep Round R10 harden pass)
**Reporter:** harden pass (code analysis) on `specs/034-expense-tracking`
**Parent spec:** `specs/034-expense-tracking/` (certified `done` 2026-06-06 — this is a genuine correctness defect, not grandfather debt)

## Summary

Every expense database-iteration loop iterates `for rows.Next()` but never checks
`rows.Err()` afterward, and the API List/Export loops additionally `continue`
silently when `Scan`/`Unmarshal` fails. pgx surfaces a mid-stream transport,
protocol, or decode failure through `rows.Err()` (and through `Scan` returning an
error on the failing row); without those checks the loop simply terminates early
and the handler treats the partial result set as complete.

Consequence for a financial feature: a mid-stream Postgres error during an expense
summary, list, or **tax-export** read yields an **under-reported expense total** or a
**silently-incomplete tax-export CSV**, returned to the caller as a successful
**HTTP 200**. The user cannot distinguish a truncated read from a complete one. This
violates **Product Principle 8 — trust-through-transparency** (no confidently-wrong
financial output).

This deviates from the codebase's own established convention: `rows.Err()` is checked
~20× in sibling files (`internal/digest/generator.go`, `internal/digest/hospitality.go`,
`internal/api/search.go`, `internal/api/drive_handlers.go`, `internal/list/store.go`,
`internal/list/generator.go`, `internal/cardrewards/store.go`). Those files always
`return` on a `Scan` error and always check `rows.Err()` after the loop. The expense
loops are the outliers.

## Affected loops (11 total — verified by harden pass)

| # | File | Loop (approx. line) | Defect |
|---|------|---------------------|--------|
| 1 | `internal/api/expenses.go` | List currency-summary `~L195` | no `rows.Err()`; `continue` on `Scan` error |
| 2 | `internal/api/expenses.go` | List data/pagination `~L240` | no `rows.Err()`; `continue` on `Scan` error |
| 3 | `internal/api/expenses.go` | Export distinct-currency `~L755` | no `rows.Err()`; `continue` on `Scan` error |
| 4 | `internal/api/expenses.go` | Export CSV stream `~L823` | no `rows.Err()`; `continue` on `Scan` (`~L827`) and `Unmarshal` (`~L831`) |
| 5 | `internal/digest/expenses.go` | summary (classification) `~L110` | no `rows.Err()`; `continue` on `Scan` error |
| 6 | `internal/digest/expenses.go` | currency totals `~L138` | no `rows.Err()`; `continue` on `Scan` error |
| 7 | `internal/digest/expenses.go` | needs-review `~L174` | no `rows.Err()`; `continue` on `Scan` error |
| 8 | `internal/digest/expenses.go` | suggestions `~L200` | no `rows.Err()`; `continue` on `Scan` error |
| 9 | `internal/digest/expenses.go` | missing-receipts `~L226` | no `rows.Err()`; `continue` on `Scan` error |
| 10 | `internal/digest/expenses.go` | unusual-charges `~L255` | no `rows.Err()`; `continue` on `Scan` error |
| 11 | `internal/intelligence/expenses.go` | uncategorized candidates `~L197` | no `rows.Err()`; `continue` on `Scan` error |

## Reproduction (code-analysis-backed — DB-error injection)

A live Postgres mid-stream failure (connection reset, server-side query
cancellation, protocol error, or a corrupt JSONB row) is hard to provoke
deterministically against a real DB, so reproduction is established two ways:

1. **Source inspection** — the pre-fix loops (captured verbatim in `report.md` →
   *Before Fix*) show the `continue`-on-error + missing-`rows.Err()` pattern.
2. **Unit-level injection** — an in-memory `rowScanner` fake (mirroring the repo's
   established `internal/list/harden_test.go` pattern) injects a mid-stream
   `rows.Err()` and a per-row `Scan` error. Against the pre-fix code path the
   collection would return a short slice with `nil` error (silent truncation);
   the regression test asserts the post-fix helpers instead return a non-nil
   error. See `report.md` → *Adversarial regression proof*.

### Concrete failure scenario (List summary)

1. Client calls `GET /api/expenses?from=2026-01-01&to=2026-12-31`.
2. The summary query streams 5 currency-group rows; after row 3 the Postgres
   connection drops (transport error). `summaryRows.Next()` returns `false`;
   `summaryRows.Err()` is now non-nil.
3. Pre-fix: the loop exits, `rows.Err()` is never consulted, and the handler
   returns `HTTP 200` with a `total_by_currency` summing only 3 of 5 groups and a
   `count` short by the missing rows.
4. The user sees an **under-reported expense total** presented as authoritative.

## Expected behavior

A mid-stream DB error during any expense read MUST surface as an error, never a
truncated success. Specifically:

- `GET /api/expenses` MUST return `HTTP 500` (not a truncated `200`) when the
  summary or data read fails mid-stream.
- `GET /api/expenses/export` MUST abort the response (interrupted download, not a
  complete-looking partial CSV) when a row decode or iteration fails mid-stream.
- The digest expense section MUST be omitted (caller already warns) rather than
  rendered with a partially-summed total.
- `GenerateSuggestions` MUST return the error rather than silently processing a
  truncated candidate set.

## Severity rationale

- **Financial-data correctness**: under-reported totals and incomplete tax-export
  CSVs are materially wrong outputs for a feature whose entire purpose is accurate
  expense accounting.
- **Silent**: returned as `HTTP 200` with no signal to the caller — the worst kind
  of correctness defect (P8 violation).
- **Bounded probability**: only manifests on an actual mid-stream DB error
  (transport drop, cancellation, corrupt row), which is uncommon but not
  impossible under load, failover, or `statement_timeout`. Hence **Medium**, not
  High.

## Out of scope

- Refactoring `ExpenseHandler.Pool` from the concrete `*pgxpool.Pool` to an
  interface (the repo's established unit-tier pattern tests an extracted
  `rowScanner` helper, not the whole handler — see `internal/list/harden_test.go`).
- Live self-hosted deploy verification (no deploy required; this is a unit-tier code
  fix). The consolidated commit + parent recertification is deferred to the
  end-of-sweep `bubbles.devops` pass per the sweep contract.
