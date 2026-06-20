# Design: Root cause analysis and fix design

## Root cause

The expense read paths predate (or diverged from) the repo's row-iteration
convention. Each loop was written as:

```go
for rows.Next() {
    var x T
    if err := rows.Scan(...); err != nil {
        continue          // ← silently drops the failing row
    }
    out = append(out, x)
}
// ← no rows.Err() check; a mid-iteration transport/protocol error is invisible
```

pgx (v5) reports two distinct failure modes that this pattern ignores:

1. **Per-row `Scan` failure** — a column decode error (e.g. corrupt JSONB, type
   mismatch). The pre-fix code `continue`s, silently dropping that financial row.
2. **Mid-iteration cursor failure** — a transport drop, server-side cancellation
   (`statement_timeout`), or protocol error. `rows.Next()` returns `false` and the
   error is *only* available via `rows.Err()`. The pre-fix code never calls it, so
   the loop terminates as if the result set ended normally.

Both modes turn a partial read into an apparently-complete one. For the API List and
Export handlers the truncated result is serialized and returned as `HTTP 200`. For
the digest, a partially-summed total is rendered. This is the P8 violation.

### Why this deviates from the established convention

`rows.Err()` is checked ~20× across the codebase. Representative sibling
(`internal/list/store.go` `GetList`):

```go
for rows.Next() {
    var item ListItem
    if err := rows.Scan(...); err != nil {
        return nil, fmt.Errorf("scan list item for list %s: %w", listID, err)
    }
    items = append(items, item)
}
if err := rows.Err(); err != nil {
    return nil, fmt.Errorf("iterate list items for list %s: %w", listID, err)
}
```

The expense loops are the outliers. The fix brings them into line with the rest of
the repo.

## Fix design

### Shared approach (all 11 loops)

Apply the established convention uniformly: `Scan` error → `return`/propagate;
add `if err := rows.Err(); err != nil { return ... }` after each loop. The
**success path is unchanged** — these checks only fire on an actual DB error.

### `internal/api/expenses.go` — extract testable helpers (mirrors `internal/list`)

`ExpenseHandler.Pool` is the concrete `*pgxpool.Pool`, so the handler cannot be
unit-tested with a fake pool without a larger refactor. The repo's accepted
unit-tier pattern (`internal/list/generator.go` + `internal/list/harden_test.go`)
extracts the row-collection logic behind a minimal `rowScanner` interface
(`Next() / Scan(...) / Err()`) and unit-tests it with an in-memory fake. We follow
that exactly:

- New package-level `rowScanner` interface.
- Hoist the two `List` local types to package level: `expenseCurrencySummary`,
  `expenseListItem` (JSON tags preserved verbatim → identical response shape).
- New `exportExpenseRow` type for the decoded tax-export row.
- New helpers:
  - `scanExpenseCurrencySummaries(rows rowScanner) ([]expenseCurrencySummary, int, error)`
  - `scanExpenseListItems(rows rowScanner) ([]expenseListItem, error)`
  - `decodeExportExpenseRow(rows rowScanner) (exportExpenseRow, error)`
- `List` calls the two collection helpers; on error → `writeExpenseError(500)`.
- The Export **distinct-currency** loop runs *before* any CSV byte, so on error it
  returns a clean `HTTP 500`.
- The Export **CSV stream** loop runs after headers/`csv.Writer` buffering has begun.
  A clean `500` is no longer possible once bytes may have flushed, so on a decode or
  `rows.Err()` error we `slog.Error(...)` and `panic(http.ErrAbortHandler)`. This is
  the documented Go idiom for aborting an in-progress response: net/http closes the
  connection so the client sees an **interrupted download**, never a complete-looking
  truncated tax CSV. (No `http.ErrAbortHandler` precedent existed in the repo; it is
  introduced here with an explanatory comment.)

### `internal/digest/expenses.go` — propagate to caller (caller already soft-degrades)

`ExpenseDigestSection.Assemble` returns `(*ExpenseDigestContext, error)`. The caller
`internal/digest/generator.go` (L160) already does:

```go
expCtx, expErr := g.ExpenseSection.Assemble(ctx)
if expErr != nil {
    slog.Warn("failed to assemble expense digest context", "error", expErr)
} else if !expCtx.IsEmpty() {
    digestCtx.Expenses = expCtx
}
```

So returning an error from any of the 6 digest loops causes the **expense section to
be omitted with a warning** — other digest sections (hospitality, knowledge, QF) are
unaffected. This is the correct P8 behavior: omit rather than render a
partially-summed total. All 6 loops get the uniform `Scan`→`return` +
`rows.Err()`→`return` treatment.

### `internal/intelligence/expenses.go` — propagate from `GenerateSuggestions`

`GenerateSuggestions(ctx) (int, error)` already returns errors on query failure; the
candidate loop gets the uniform `Scan`→`return 0, err` + `rows.Err()`→`return 0, err`
treatment.

## Affected files (fix scope)

- `internal/api/expenses.go` — helpers + 4 loops + Export abort path
- `internal/digest/expenses.go` — 6 loops
- `internal/intelligence/expenses.go` — 1 loop
- `internal/api/expenses_rowserr_test.go` (new) — adversarial regression for the
  extracted helpers

## Regression test design (adversarial — non-tautological)

New file `internal/api/expenses_rowserr_test.go` with a `fakeExpenseRows` `rowScanner`
(holds `[][]any` column values, `scanErrAt`, `scanErr`, `finalErr`, `errCalls`),
mirroring `internal/list/harden_test.go`:

| Test | Injection | Asserts |
|------|-----------|---------|
| `..._PropagatesRowsErr` | valid rows + `finalErr` set | helper returns non-nil error wrapping `finalErr`; nil result; `errCalls > 0` |
| `..._PropagatesScanError` | `scanErrAt` set | helper returns non-nil error wrapping `scanErr` |
| `..._HappyPathChecksRowsErr` | valid rows, `finalErr` nil | no error; correct result; `errCalls > 0` (Err consulted on success too) |
| export decode `..._PropagatesScanError` | `scanErrAt=0` | non-nil error ("scan expense export row") |
| export decode `..._PropagatesUnmarshalError` | invalid JSON row | non-nil error ("unmarshal expense export row") |
| export decode `..._HappyPath` | valid JSON | decodes `exp` correctly |

**Adversarial proof of non-tautology**: after the suite is green, temporarily delete
the `rows.Err()` check from `scanExpenseCurrencySummaries`, run the suite → capture
the RED failure, then restore → capture GREEN. This proves the test fails iff the bug
is reintroduced (captured in `report.md`).

## Risk

Low. Success-path output is byte-for-byte unchanged. The only behavioral change is on
an actual mid-stream DB error, where the new behavior (error/abort/omit) is strictly
more correct than the old behavior (silent truncation). The Export `panic(http.ErrAbortHandler)`
is recovered by net/http and is the standard streaming-abort idiom.
