# Design: BUG-013-002 — Data race in `TestFetchActivityContextCancelledBetweenPages` page counter

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Current Truth (objective baseline before this fix)

Established from the codebase at HEAD `c5e16160` (clean checkout of the
fix target file):

1. **The racing variable** — `internal/connector/guesthost/regression_test.go`,
   `TestFetchActivityContextCancelledBetweenPages`:
   ```go
   page := 0
   srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
       page++                                   // WRITE — server goroutine (line 677)
       ...
       {ID: "e" + strings.Repeat("x", page), ...},
       Cursor:  "cursor-page-" + strings.Repeat("x", page),
   }))
   ...
   go func() {
       for page < 1 {                           // READ — spin-wait goroutine (line 695)
           // spin-wait for first page to complete
       }
       cancel()
   }()
   _, err := client.FetchActivity(ctx, "", "", 10)
   ```
   `page` is a plain `int`. `net/http/httptest.Server` invokes the
   handler on its own goroutine per request; `FetchActivity` (running on
   the test's main goroutine) triggers that handler. The `go func()`
   cancellation goroutine reads `page` concurrently. There is no mutex,
   atomic, or channel mediating the write and the read.

2. **The race detector confirms it** — `go test -race -run '^TestFetchActivityContextCancelledBetweenPages$'`:
   ```
   WARNING: DATA RACE
   Write at 0x...c00041af48 by goroutine 108:
         regression_test.go:677            // page++
   Previous read at 0x...c00041af48 by goroutine 103:
         regression_test.go:695            // for page < 1
   Goroutine 103 (running) created at:
         regression_test.go:694            // go func() { ... }()
   FAIL    github.com/smackerel/smackerel/internal/connector/guesthost  0.119s
   ```

3. **The logic is correct without `-race`** —
   `go test -run '^TestFetchActivityContextCancelledBetweenPages$'` →
   `ok 0.066s`. The cancellation timing works in practice; only the
   memory-model synchronization is missing.

4. **The canonical unit surface does not use `-race`** —
   `scripts/runtime/go-unit.sh:66-68`:
   ```bash
   echo "[go-unit] starting go test ./..."
   go test "${go_test_args[@]}" ./...
   ```
   No `-race` flag. So the defect is invisible to `./smackerel.sh test
   unit --go` but breaks every direct `go test -race` invocation against
   the package.

5. **Prior acknowledgement / deferral** —
   `specs/022-operational-resilience/bugs/BUG-022-003-connector-429-retry-after-handling/scopes.md:184`
   records the same race as a "pre-existing -race-only flake … unrelated
   to this bug", confirmed against an unmodified earlier HEAD. It was
   correctly out of scope there; it is in scope for a regression sweep on
   spec 013 itself.

Conclusion: the defect is a missing-synchronization data race on a single
`int` counter used purely for test timing. The production `Client` and
`FetchActivity` paths are not involved.

## Root Cause

A test-local `int` is shared between two goroutines (the `httptest`
handler and the cancellation spin-wait) without any synchronization
primitive. The Go race detector flags the concurrent write/read; the Go
memory model makes the program's behavior technically undefined even
though the observed assertion currently passes.

## Solution

Replace the plain `int` with `sync/atomic.Int32` and route every access
through atomic operations. This is the smallest change that establishes a
happens-before relationship between the handler's increment and the
spin-wait's read, removing the data race while preserving the test's
control flow and assertions verbatim.

### Before → After

| Site | Before | After |
|------|--------|-------|
| Declaration | `page := 0` | `var page atomic.Int32` |
| Handler increment | `page++` then use `page` | `p := int(page.Add(1))` then use `p` |
| Spin-wait read | `for page < 1 {` | `for page.Load() < 1 {` |

`page.Add(1)` returns the new value atomically, so the handler captures a
local `p` once per request and uses it for both `strings.Repeat` calls —
this also removes the double-read of `page` inside a single handler
invocation. `page.Load()` gives the spin-wait goroutine a synchronized
read.

### Why atomic, not a mutex or channel

- **Atomic** is the canonical Go idiom for a single integer counter
  shared across goroutines. It is lock-free, zero-allocation, and the
  minimal diff (one import, one declaration, one increment, one read).
- **A `sync.Mutex`** would require wrapping both the increment and the
  read in `Lock`/`Unlock`, adding more surface than the defect warrants.
- **A channel handoff** would restructure the handler and the spin-wait
  goroutine — a larger refactor than needed and out of scope per the bug
  Non-Goals.

The atomic fix preserves the test's intent (H-013-R2-001: context
cancellation between pagination pages) exactly; only the memory
synchronization changes.

## Alternatives Considered

1. **Delete the test.** Rejected — the test provides real value
   (regression coverage for the between-pages cancellation check); the
   defect is in its synchronization, not its purpose.
2. **Skip under `-race` (`testing.Short()` / build tag).** Rejected —
   hiding a fixable data race behind a skip is exactly the kind of
   masking that lets a *real* future race in the same package go
   unnoticed. The fix is trivial; skip is not justified.
3. **Convert the spin-wait to a `sync.WaitGroup` / channel signal.**
   Rejected as scope creep — see "Why atomic" above.

## Blast Radius

- **One file changed:** `internal/connector/guesthost/regression_test.go`
  (test only).
- **No production code, no other test, no scenario manifest, no planning
  truth** (`spec.md`/`design.md`/`scopes.md` of parent spec 013) is
  touched. The H-013-R2-001 scenario semantics are unchanged.
- **No new dependency** — `sync/atomic` is in the standard library.
