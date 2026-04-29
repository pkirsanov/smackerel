# BUG-010-001 — Browser-history connector missing SQLite driver import

> **Parent feature:** [specs/010-browser-history-connector](../../)
> **Parent scope:** Browser-history connector certification
> **Filed by:** `bubbles.bug` (bugfix-fastlane, document-only mode)
> **Filed at:** 2026-04-26
> **Severity:** P1 / HIGH — blocks browser-history connector certification entirely
> **Status:** Fixed — validated 2026-04-26 by bubbles.validate (7 integration tests + sentinel PASS under live stack)

---

## Summary

`internal/connector/browser/browser.go:111` calls `sql.Open("sqlite3", dbPath+"?mode=ro")` to read Chrome's history database, but no SQLite driver is imported anywhere in the Go module. At runtime `sql.Drivers()` returns `[]` and any real `Sync()` against a Chrome `History` file fails immediately with:

```
sql: unknown driver "sqlite3" (forgotten import?)
```

This defect:

1. Makes the connector non-functional against any real Chrome `History` file.
2. Hides itself in the unit-test suite because the unit tests do not call `sql.Open`.
3. Causes all 6 integration tests in `tests/integration/browser_history_test.go` to be silently `SKIP`'d via `requireFixture()` because the fixture directory `data/browser-history/History/` is empty — even after the directory is populated, the tests would fail at `sql.Open` before reading any rows.
4. Blocks the last two unchecked items in [specs/010-browser-history-connector/uservalidation.md](../../uservalidation.md): "Integration tests pass against real SQLite fixture" and "E2E tests pass against live stack".

## Symptoms

### S1 — `sql.Drivers()` is empty at runtime

`grep -r 'sqlite3\|modernc.org/sqlite\|glebarez' go.mod` returns no matches. The module's `require` and indirect blocks contain no SQLite driver. At runtime `database/sql.Drivers()` returns `[]`.

### S2 — Integration tests skip universally

`tests/integration/browser_history_test.go` contains 6 integration tests:

1. `TestBrowserHistorySync_InitialImport` (line 57)
2. `TestBrowserHistorySync_IncrementalCursor` (line 115)
3. `TestBrowserHistorySync_FullPipelineFlow` (line 164)
4. `TestBrowserHistorySync_SocialMediaAggregation` (line 241)
5. `TestBrowserHistorySync_RepeatVisitEscalation` (line 297)
6. `TestBrowserHistorySync_FullPipeline_WithAggregationAndPrivacy` (line 345)

All 6 share the `requireFixture()` helper (line 45) which calls `t.Skip("integration: Chrome History test fixture not available")` when no usable SQLite fixture exists. The repo-local fixture directory `data/browser-history/History/` is empty, so all 6 are unconditionally skipped on any clean checkout.

### S3 — Even with a fixture, `Sync()` would fail

If the fixture were populated, `ParseChromeHistorySince()` (browser.go line 103–117) would still error at `sql.Open("sqlite3", dbPath+"?mode=ro")` with the unknown-driver message above, because no driver was ever registered under the name `"sqlite3"`.

### S4 — User validation gates remain red

`specs/010-browser-history-connector/uservalidation.md` records both unchecked items as "VERIFIED FAIL 2026-04-26 (bubbles.validate)" with the missing-driver root cause attached. The connector cannot be certified until this defect is resolved.

## Reproduction

### Prerequisite (S2 confirmation)

```bash
ls data/browser-history/History/
go test -tags=integration -run TestBrowserHistory ./tests/integration/ -v
```

Expected (verbatim signature, recorded in `report.md`): all 6 tests `--- SKIP` with reason `"integration: Chrome History test fixture not available"`.

### Direct driver presence probe (S1 + S3 confirmation)

A standalone probe in any Go file in the module:

```go
import "database/sql"
fmt.Println(sql.Drivers())                    // expect: []
_, err := sql.Open("sqlite3", ":memory:")
fmt.Println(err)                              // expect: sql: unknown driver "sqlite3" (forgotten import?)
```

Both assertions hold on `main` HEAD as of 2026-04-26.

## Expected behavior

1. The Go module imports a registered SQLite driver under the name `"sqlite3"`.
2. `sql.Drivers()` includes `"sqlite3"`.
3. `sql.Open("sqlite3", ":memory:")` returns no error.
4. With a populated fixture in `data/browser-history/History/`, the 6 integration tests in `tests/integration/browser_history_test.go` execute (no longer skip) and pass on the live integration stack.
5. The browser-history connector can complete a `Sync()` end-to-end against a real Chrome `History` file and publish `RawArtifact` events to NATS.
6. Both currently-failing user-validation items in [specs/010-browser-history-connector/uservalidation.md](../../uservalidation.md) flip to `[x]`.

## Actual behavior

1. No SQLite driver in `go.mod` / `go.sum`.
2. `sql.Drivers()` returns `[]`.
3. `sql.Open("sqlite3", ...)` returns `sql: unknown driver "sqlite3" (forgotten import?)`.
4. All 6 integration tests skip; none execute.
5. `Sync()` errors out before any rows are read.
6. Both user-validation items remain unchecked with documented failure evidence.

## Decision (driver choice)

**Preferred:** `modernc.org/sqlite` (pure-Go, no CGO required).

- Matches the repo's CGO-free posture (Go core runtime is built without CGO in the standard `./smackerel.sh build` flow).
- Driver name is `"sqlite"` by default; must be re-registered as `"sqlite3"` via a small init that calls `sql.Register("sqlite3", &sqlite.Driver{})` (or equivalent), OR the production `sql.Open` call sites must be migrated to use the `"sqlite"` driver name. The boundary owner (`bubbles.design`) will lock the exact mechanism.

**Fallback:** `github.com/mattn/go-sqlite3` (CGO).

- Registers itself as `"sqlite3"` automatically via blank import.
- Requires CGO — would force a build-system change and is rejected unless the pure-Go option is blocked.

The fix scope MUST add exactly one of these drivers, registered (directly or via `Register`) under the name `"sqlite3"` so that the existing `sql.Open("sqlite3", ...)` call site in `browser.go:111` is not modified.

## Boundary

Allowed surfaces:

- `go.mod`, `go.sum`
- `internal/connector/browser/` — new file `sqlite_driver.go` (or equivalent) for the blank-import + optional `Register` call
- `tests/integration/browser_history_test.go` — only if needed to add the adversarial driver-presence sentinel test or to relax the fixture-skip when an in-test fixture is generated
- `data/browser-history/History/` — committed deterministic Chrome-shaped SQLite fixture (or in-test seed code that creates the fixture deterministically)

Forbidden:

- Any change to `internal/connector/browser/browser.go` semantic logic (parsing, dwell-tier, skip rules, social aggregation, retry, copy-then-read).
- Any change to other connectors, the agent runtime, NATS wiring, or the pipeline.
- Any change that disables, skips, or weakens the assertions of the 6 existing integration tests.
- Any other production code outside the boundary above.

## Acceptance scenarios

```gherkin
Feature: BUG-010-001 SQLite driver is registered for the browser-history connector

  Scenario: BUG-010-001-A driver presence sentinel
    Given the smackerel Go module is built from HEAD
    When sql.Drivers() is queried at runtime
    Then "sqlite3" appears in the returned slice
    And sql.Open("sqlite3", ":memory:") returns a non-nil DB and a nil error

  Scenario: BUG-010-001-B integration tests stop skipping
    Given a deterministic Chrome-shaped SQLite fixture exists at data/browser-history/History/History
    When `go test -tags=integration -run TestBrowserHistory ./tests/integration/ -v` runs
    Then all 6 TestBrowserHistorySync_* tests execute (none SKIP)
    And all 6 PASS against the live integration stack

  Scenario: BUG-010-001-C real Sync() round-trip succeeds
    Given the browser-history connector is configured against the deterministic fixture
    When Sync() is invoked
    Then sql.Open succeeds with no "unknown driver" error
    And at least one RawArtifact is published to NATS
    And the connector reports Health() == healthy

  Scenario: BUG-010-001-D parent uservalidation gates flip green
    Given BUG-010-001-A, B, and C all pass
    When `bubbles.validate` re-runs the parent feature uservalidation checklist
    Then "Integration tests pass against real SQLite fixture" is checked [x]
    And "E2E tests pass against live stack" is checked [x] (or remains gated only by unrelated stack issues)
```

## Adversarial regression contract

Any fix MUST satisfy ALL of the following — the regression suite must encode them:

- **A1.** A driver-presence sentinel test asserts `sql.Open("sqlite3", ":memory:")` returns no error. This test MUST live in a package that depends transitively on the same import path as the production driver registration, so removal of the blank import in `internal/connector/browser/sqlite_driver.go` (or wherever the registration lives) makes the sentinel fail.
- **A2.** All 6 `TestBrowserHistorySync_*` tests MUST execute (no `--- SKIP`). The fix is incomplete if any of the 6 still skip post-fix.
- **A3.** The fix MUST NOT modify the assertions of the 6 existing integration tests. Test-isolation/skip-removal changes are allowed only to consume a deterministic fixture, not to lower the bar.
- **A4.** Pre-fix evidence (verbatim `sql.Drivers() == []` proof + verbatim "unknown driver \"sqlite3\"" error + verbatim 6× SKIP output) MUST be captured in `report.md` so a future regression can be detected by re-comparing against the same failure signature.
- **A5.** No silent-pass bailout patterns in the regression tests. The driver-presence sentinel MUST NOT contain `if err != nil { t.Skip(); return }` around the `sql.Open` call. The 6 integration tests MUST NOT be loosened to skip when the driver is missing — they must hard-fail.
- **A6.** If the synthetic fixture is generated in test setup code, the generation MUST be deterministic (fixed visit_time / visit_duration / url / title rows) so test assertions remain stable across runs.

## Out of scope

- Refactoring the browser-history connector beyond adding the SQLite driver and (optionally) the test fixture seeder.
- Adding additional Chrome history features (extensions, downloads, favicons, etc.).
- Updating other connectors that may also need SQLite in the future.
- Re-running the parent feature uservalidation checklist (deferred to `bubbles.validate` after this bug is closed).
