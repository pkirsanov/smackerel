# Scopes: BUG-013-002 — Data race in `TestFetchActivityContextCancelledBetweenPages` page counter

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Make the regression-test page counter race-free via atomic access

**Status:** Done
**Priority:** P2
**Depends On:** None
**Owner:** bubbles.workflow (parent-expanded regression-to-doc child of stochastic-quality-sweep R19, round 19 of 20)

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BUG-013-002-001 Cancellation-between-pages test runs race-free under the detector
  Given the GuestHost regression test TestFetchActivityContextCancelledBetweenPages
    And the httptest handler goroutine increments the page counter
    And a separate spin-wait goroutine reads the page counter
  When the test runs under "go test -race"
  Then no data race is reported
    And the test passes

Scenario: SCN-BUG-013-002-002 Cancellation assertion intent is preserved
  Given the context is cancelled after the first pagination page completes
  When Client.FetchActivity is called against the paginating httptest server
  Then it returns a non-nil error whose message mentions cancellation or context
    And the test still asserts that exact behavior (unchanged from before the fix)

Scenario: SCN-BUG-013-002-003 Whole connector package is race-clean
  Given the fix replaces the unsynchronized int counter with sync/atomic.Int32
  When the full internal/connector/guesthost package runs under "go test -race"
  Then the package passes with no data race in any test

Scenario: SCN-BUG-013-002-004 No coverage decrease and no cross-spec breakage
  Given the fix touches only test synchronization, not assertions or production code
  When the package runs without -race
  Then the stable 64-test pass count is unchanged
    And the sibling connector packages, internal/graph, and internal/digest still pass under -race
```

### Change Boundary

This scope is a single surgical edit to one test file. No production
source, no other test, and no planning truth is touched.

**Allowed file family** (the only surface this scope may touch):

- `internal/connector/guesthost/regression_test.go` — add the
  `sync/atomic` import; change `page` from `int` to `atomic.Int32`;
  change the handler increment to `page.Add(1)` (captured as a local
  `p`); change the spin-wait read to `page.Load()`.

**Excluded surfaces** (MUST remain untouched):

- `internal/connector/guesthost/client.go`, `connector.go`,
  `normalizer.go`, `types.go` — production code, unchanged.
- Every other test function in `regression_test.go`,
  `client_test.go`, `connector_test.go`, `normalizer_test.go` —
  unchanged.
- `specs/013-guesthost-connector/spec.md`, `design.md`, `scopes.md`,
  `scenario-manifest.json` — planning truth, unchanged (no
  recertification triggered).
- The assertion logic of
  `TestFetchActivityContextCancelledBetweenPages` itself — the
  cancellation-error assertion is preserved verbatim.

### Implementation Plan

1. Add `"sync/atomic"` to the import block of
   `internal/connector/guesthost/regression_test.go` (alphabetically
   between `"strings"` and `"testing"`).
2. In `TestFetchActivityContextCancelledBetweenPages`:
   - Replace `page := 0` with `var page atomic.Int32` (plus an
     explanatory comment naming the data race being fixed).
   - In the handler, replace `page++` with `p := int(page.Add(1))` and
     use the local `p` in both `strings.Repeat("x", p)` call-sites
     (event ID and cursor).
   - In the spin-wait goroutine, replace `for page < 1 {` with
     `for page.Load() < 1 {`.
3. `gofmt -l` the file → no output.
4. `go vet ./internal/connector/guesthost/...` → clean.
5. `go test -race -count=1 -run '^TestFetchActivityContextCancelledBetweenPages$' ./internal/connector/guesthost/` → PASS (red→green proof).
6. `go test -race -count=1 ./internal/connector/guesthost/...` → PASS.
7. `go test -count=1 ./internal/connector/guesthost/...` → PASS with 64-test count.
8. `go test -race -count=1 ./internal/connector/... ./internal/graph/... ./internal/digest/...` → PASS (cross-spec regression).
9. `bash .github/bubbles/scripts/artifact-lint.sh specs/013-guesthost-connector/bugs/BUG-013-002-regression-test-page-counter-data-race` → PASSED.

### Test Plan (with red→green discipline)

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BUG013-002-01 | TestFetchActivityContextCancelledBetweenPages (under `-race`) | regression (red before fix, green after) | `internal/connector/guesthost/regression_test.go` | Runs under the race detector with no DATA RACE report; still asserts a cancellation/context error from FetchActivity | SCN-BUG-013-002-001, SCN-BUG-013-002-002 |
| T-BUG013-002-02 | Full guesthost package under `-race` | regression | `go test -race -count=1 ./internal/connector/guesthost/...` | Whole package green; no race in any test | SCN-BUG-013-002-003 |
| T-BUG013-002-03 | Full guesthost package without `-race` | regression (coverage stability) | `go test -count=1 ./internal/connector/guesthost/...` | Stable 64-test pass count; no test removed, no coverage decrease | SCN-BUG-013-002-004 |
| T-BUG013-002-04 | Cross-spec regression under `-race` | regression | `go test -race -count=1 ./internal/connector/... ./internal/graph/... ./internal/digest/...` | All sibling connectors + hospitality consumers (graph, digest) PASS; fix introduces no breakage | SCN-BUG-013-002-004 |

#### Red → Green discipline (Gate G060)

| Test | Red (pre-fix) | Green (post-fix) |
|------|---------------|------------------|
| TestFetchActivityContextCancelledBetweenPages under `-race` | Plain `int page` is written by the handler goroutine (`page++`, line 677) and read by the spin-wait goroutine (`for page < 1`, line 695) → race detector reports `WARNING: DATA RACE` and the package FAILS (`FAIL … 0.119s`) | `var page atomic.Int32` with `page.Add(1)` / `page.Load()` establishes happens-before → no race reported, test PASSES (`ok … 1.058s`) |

#### Adversarial dimension (Gate G021)

The regression guard is the race detector itself. The test genuinely
exercises concurrent access to `page` (handler goroutine writes,
spin-wait goroutine reads). If a future change reintroduces the
unsynchronized `int` (or otherwise drops the atomic), `go test -race`
deterministically FAILS again — proven by the captured pre-fix red run.
This is not a tautological regression: the pre-fix code genuinely fails
under `-race` and the post-fix code genuinely passes, both captured in
the same session (see report.md Before Fix / After Fix).

### Consumer Impact Sweep

- **Production callers of the racing variable:** none — `page` is a
  test-local variable; no shipped code references it.
- **Other tests in the package:** unchanged; the 64-test non-race count
  is identical before and after.
- **Sibling connector packages / hospitality consumers:** the R19
  cross-spec `-race` run (all `internal/connector/...`, `internal/graph`,
  `internal/digest`) is green, confirming the fix introduces no breakage
  beyond the single test file.
- **Planning truth / scenario manifest:** untouched; H-013-R2-001
  scenario semantics preserved, so no traceability or recertification
  impact on parent spec 013.

### Definition of Done (DoD)

- [x] `sync/atomic` import added; `page` changed to `atomic.Int32`;
      handler uses `page.Add(1)` (captured as `p`); spin-wait uses
      `page.Load()`. Diff: 3 hunks via `multi_replace_string_in_file`,
      one file. Evidence: report.md Code Diff Evidence.
- [x] No production source file modified. Evidence: `git status` shows
      only `internal/connector/guesthost/regression_test.go` changed
      among non-pre-existing files (report.md).
- [x] No other test function modified. Evidence: diff is confined to the
      import block and `TestFetchActivityContextCancelledBetweenPages`.
- [x] `gofmt -l internal/connector/guesthost/regression_test.go` clean
      (no output). Evidence: report.md (`FMT_RC=0`).
- [x] `go vet ./internal/connector/guesthost/...` clean
      (`VET_RC=0`). Evidence: report.md.
- [x] `go test -race -run '^TestFetchActivityContextCancelledBetweenPages$'`
      PASS (was FAIL pre-fix). Red→green captured in report.md.
- [x] `go test -race ./internal/connector/guesthost/...` PASS
      (whole package green under race). Evidence: report.md
      (`PKG_RACE_RC=0`).
- [x] `go test ./internal/connector/guesthost/...` PASS with 64-test
      stable count. Evidence: report.md.
- [x] Cross-spec `-race` regression
      (`./internal/connector/... ./internal/graph/... ./internal/digest/...`)
      PASS (`CROSS_RC=0`). Evidence: report.md.
- [x] Before Fix and After Fix reproduction captures present in
      report.md, same session, ≥10 lines each (Gate 0).
- [x] Code Diff Evidence section present in report.md (Gate G053).
- [x] `artifact-lint.sh` PASS for this bug folder. Evidence: report.md.
- [x] No commit/push performed in this round — changes left in the working
      tree for the parent batch. Evidence: report.md "Pre-existing baseline
      drift" section (git status enumeration).
