# User Validation: BUG-013-002 — Data race in `TestFetchActivityContextCancelledBetweenPages` page counter

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Scope of Externally Observable Change

This fix has **no externally observable functional change**. It touches
only a test file's internal synchronization. Operators and end users see:

- The same GuestHost connector behavior — `Client.FetchActivity`,
  pagination, cursor handling, and normalization are all unchanged.
- The same shipped binaries — no production source file is modified, so
  the container images are byte-for-byte unaffected by this change.

The change is purely a **developer-experience / CI-quality** improvement:

- **Before** — running `go test -race` against
  `internal/connector/guesthost/...` reported a `DATA RACE` and failed,
  masking any *real* future race in the same package behind the known
  noise, and making race-mode validation of spec 013's connector
  impossible without a `git stash`.
- **After** — `go test -race` against the package passes cleanly, so
  race-mode validation is meaningful again.

## Acceptance Surface

| Surface | Validation step | Expected outcome |
|---------|-----------------|------------------|
| Race-mode package check | `go test -race -count=1 ./internal/connector/guesthost/...` | `ok` — no DATA RACE; was FAIL before the fix |
| Targeted test (race) | `go test -race -count=1 -run '^TestFetchActivityContextCancelledBetweenPages$' ./internal/connector/guesthost/` | `ok` — the cancellation test passes under the detector |
| Non-race stability | `go test -count=1 ./internal/connector/guesthost/...` | `ok` — 64 tests pass, identical to before the fix (no coverage decrease) |
| Cross-spec regression | `go test -race -count=1 ./internal/connector/... ./internal/graph/... ./internal/digest/...` | All green — no sibling-connector or hospitality-consumer breakage |
| Production behavior | Inspect `git diff` | Only `internal/connector/guesthost/regression_test.go` (a `_test.go` file) changed; zero production source delta |

## Validation Steps

1. Check out the working tree with the fix applied.
2. Run the targeted race check:
   ```bash
   go test -race -count=1 -run '^TestFetchActivityContextCancelledBetweenPages$' ./internal/connector/guesthost/
   ```
   Confirm `ok` and **no** `WARNING: DATA RACE`.
3. Run the whole package under race:
   ```bash
   go test -race -count=1 ./internal/connector/guesthost/...
   ```
   Confirm `ok`.
4. Confirm non-race stability and the 64-test count:
   ```bash
   go test -count=1 -v ./internal/connector/guesthost/... | grep -c '^--- PASS'
   ```
   Confirm `64`.
5. Confirm no production code changed:
   ```bash
   git diff --name-only | grep -v '_test.go'
   ```
   Confirm the guesthost connector has no non-test file in the output.

## Sign-off

- [x] No production behavior change — test-only fix.
- [x] Race-mode validation of the spec 013 connector package is restored.
- [x] No coverage decrease (stable 64-test count).
- [x] No cross-spec regression.
- [x] Evidence captured in [report.md](report.md) (Before Fix / After Fix, same session).
