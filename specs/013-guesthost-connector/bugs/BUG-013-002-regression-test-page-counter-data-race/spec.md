# Bug: BUG-013-002 — Data race in `TestFetchActivityContextCancelledBetweenPages` page counter

Links: [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Classification

- **Type:** Test-quality / concurrency defect — an unsynchronized `int`
  counter in the spec 013 regression test
  (`internal/connector/guesthost/regression_test.go`) is written by the
  `httptest` handler goroutine and concurrently read by a spin-wait
  goroutine, producing a Go data race.
- **Severity:** LOW — test-harness only; no production code path is
  involved. The canonical unit surface (`./smackerel.sh test unit --go`,
  which runs `go test ./...` without `-race`, see
  `scripts/runtime/go-unit.sh:66-68`) is unaffected and stays green.
  However, `go test -race` (the race-mode quality command used repeatedly
  this session and in prior sweep rounds) **fails deterministically** for
  the entire `internal/connector/guesthost` package, blocking race-mode
  validation of spec 013's connector.
- **Parent Spec:** 013 — GuestHost Connector & Hospitality Intelligence
- **Workflow Mode:** regression-to-doc (parent-expanded child of
  stochastic-quality-sweep round 19, trigger `regression`, mapped mode
  `regression-to-doc`)
- **Status:** Open — discovered by regression R19 (sweep round 19 of 20)

## Problem Statement

`TestFetchActivityContextCancelledBetweenPages`
(`internal/connector/guesthost/regression_test.go`) verifies that
`Client.FetchActivity` returns a cancellation error when the context is
cancelled between pagination pages (scenario H-013-R2-001). To time the
cancellation, the test:

1. Declares a plain `page := 0` counter.
2. Increments it inside the `httptest` HTTP handler (`page++`) — this
   handler runs on the server's own goroutine.
3. Spin-waits on it from a separate cancellation goroutine
   (`for page < 1 { }`).

The `int` `page` is therefore **written by the handler goroutine** and
**read by the spin-wait goroutine** with no synchronization. Under the Go
race detector this is reported as a data race:

```
WARNING: DATA RACE
Write at 0x... by goroutine 108:
      internal/connector/guesthost/regression_test.go:677   // page++
Previous read at 0x... by goroutine 103:
      internal/connector/guesthost/regression_test.go:695   // for page < 1
```

Without `-race`, the assertion logic passes (the timing works out), so the
defect was masked on the canonical unit pipeline and carried forward as
tech debt. It was explicitly acknowledged and deferred in a different
bug's context:

> "guesthost has a pre-existing -race-only flake
> (TestFetchActivityContextCancelledBetweenPages) in
> regression_test.go:695 confirmed against unmodified HEAD c844addc via
> git stash; unrelated to this bug."
> — `specs/022-operational-resilience/bugs/BUG-022-003-.../scopes.md:184`

A regression sweep on spec 013 itself is the correct vehicle to close it:
the defect lives in spec 013's own regression suite and breaks race-mode
validation of spec 013's connector package.

## Impact

| Axis | Impact |
|------|--------|
| **Production** | None. The defect is confined to test code; no shipped code path reads or writes the racing variable. |
| **Canonical unit pipeline** | None. `./smackerel.sh test unit --go` runs `go test ./...` without `-race`; the assertion passes and the package is green. |
| **Race-mode validation** | `go test -race ./internal/connector/guesthost/...` FAILS deterministically. Any agent or operator running race-mode checks (as done this session and in prior rounds) sees spec 013's connector package fail, masking any *real* future race in the same package behind the noise. |
| **Correctness semantics** | A data race is undefined behavior under the Go memory model. Although the observed logic currently passes, the program is technically non-deterministic, and the spin-wait read of `page` is not guaranteed to observe the handler's write in a defined order. |
| **Cross-product impact** | None — spec 013 is internal to Smackerel; QF Companion is unaffected. |

## Acceptance Criteria

- [ ] `internal/connector/guesthost/regression_test.go` declares the
      `page` counter as a synchronized type (`sync/atomic.Int32`) so the
      handler write and the spin-wait read are race-free.
- [ ] The handler uses an atomic increment (`page.Add(1)`) and the
      spin-wait uses an atomic load (`page.Load()`).
- [ ] The test's externally observable behavior and assertions are
      unchanged: it still asserts that `FetchActivity` returns a
      cancellation error when the context is cancelled between pages
      (scenario H-013-R2-001 intent preserved).
- [ ] No production source file is modified.
- [ ] No other test in the package is modified.
- [ ] `gofmt -l` reports the file clean and `go vet ./internal/connector/guesthost/...` is clean.
- [ ] `go test -race -count=1 -run '^TestFetchActivityContextCancelledBetweenPages$' ./internal/connector/guesthost/` PASS (was FAIL pre-fix).
- [ ] `go test -race -count=1 ./internal/connector/guesthost/...` PASS (whole package green under race).
- [ ] `go test -count=1 ./internal/connector/guesthost/...` PASS with the
      stable 64-test count (no coverage decrease, no test removed).
- [ ] Cross-spec regression: `go test -race -count=1 ./internal/connector/... ./internal/graph/... ./internal/digest/...` PASS (no breakage in sibling connectors or the hospitality consumers).
- [ ] `artifact-lint.sh` PASS for this bug folder.

## Non-Goals

- Rewriting the test's spin-wait into a channel-based handoff. The
  busy-wait is a pre-existing style choice, not the defect; replacing the
  unsynchronized `int` with an atomic is the minimal, surgical fix that
  removes the data race while preserving the test's exact control flow.
- Wiring `-race` into the canonical `./smackerel.sh test unit` surface.
  That is a separate tooling decision outside this bug's scope.
- Backfilling `-race` audits of every other connector package (the R19
  cross-spec regression already confirmed they are race-clean).
