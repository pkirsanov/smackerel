# Spec: Expected behavior for expense DB-iteration error handling

## Context

This bug spec governs the row-iteration error handling of the expense read paths
across three packages: `internal/api/expenses.go` (HTTP), `internal/digest/expenses.go`
(daily digest), and `internal/intelligence/expenses.go` (suggestion generation).

The binding convention is the one already used ~20× elsewhere in the codebase:

```go
for rows.Next() {
    var x T
    if err := rows.Scan(...); err != nil {
        return <zero>, fmt.Errorf("scan <ctx>: %w", err)
    }
    out = append(out, x)
}
if err := rows.Err(); err != nil {
    return <zero>, fmt.Errorf("iterate <ctx>: %w", err)
}
```

## Acceptance criteria

1. **List summary** (`GET /api/expenses`): a `Scan` error or a non-nil `rows.Err()`
   on the currency-summary read causes the handler to return `HTTP 500`
   (`QUERY_FAILED`), never a truncated `200`.
2. **List data** (`GET /api/expenses`): a `Scan` error or non-nil `rows.Err()` on the
   data/pagination read causes the handler to return `HTTP 500`, never a truncated
   `200`.
3. **Export distinct-currency** (`GET /api/expenses/export`): a `Scan` error or
   non-nil `rows.Err()` on the pre-stream currency read causes the handler to return
   `HTTP 500` before any CSV byte is produced.
4. **Export CSV stream** (`GET /api/expenses/export`): a `Scan`/`Unmarshal` error or
   non-nil `rows.Err()` mid-stream aborts the response (interrupted download via
   `http.ErrAbortHandler`) rather than flushing a complete-looking, silently
   truncated CSV as `200`.
5. **Digest** (`ExpenseDigestSection.Assemble`): a `Scan` error or non-nil
   `rows.Err()` on any of the 6 digest reads returns an error from `Assemble`. The
   caller (`internal/digest/generator.go`) already logs a warning and omits the
   expense section on error — so the user sees no expense section rather than a
   partially-summed one.
6. **Suggestions** (`ExpenseClassifier.GenerateSuggestions`): a `Scan` error or
   non-nil `rows.Err()` on the candidate read returns the error rather than
   processing a truncated candidate set.
7. **Success path unchanged**: when no DB error occurs, every endpoint and function
   produces byte-for-byte identical output to the pre-fix behavior.

## Negative / regression criteria

1. An adversarial unit test MUST inject a mid-stream `rows.Err()` into the extracted
   expense row-collection helpers and assert the helper returns a non-nil error
   wrapping the injected error. The test MUST FAIL if the `rows.Err()` check is
   removed (proven by reintroducing the defect and capturing the RED run).
2. An adversarial unit test MUST inject a per-row `Scan` error and assert the helper
   returns a non-nil error (proving the `continue`→propagate change).
3. An adversarial unit test MUST inject an invalid JSON row into the export decode
   helper and assert it returns a non-nil error (proving the `Unmarshal`
   `continue`→propagate change).
4. A happy-path test MUST assert the helper still consults `rows.Err()` on the
   success path (so a clean termination is distinguishable from a silent
   truncation), mirroring `internal/list/harden_test.go`.

## Product Principle Alignment

- **Principle 8 — Trust Through Transparency**: financial output (expense totals,
  tax-export CSV) MUST NOT be silently truncated and presented as complete. This
  bug fix is a direct enforcement of P8 on the expense read paths.

## Out of scope

- Abstracting `ExpenseHandler.Pool` to an interface for full handler-level HTTP-500
  injection (the repo's accepted unit-tier pattern tests the extracted helper).
- Changing the SQL queries, schema, or success-path output shape.
