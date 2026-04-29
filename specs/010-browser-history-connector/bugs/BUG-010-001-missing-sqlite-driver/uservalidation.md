# BUG-010-001 User Validation Checklist

> **Scope:** Validation gates that must flip green before this bug can be
> certified done. Checked-by-default convention applies AFTER the fix is
> validated; until fix dispatch, items remain unchecked because the bug is
> still open.

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] **Driver presence sentinel passes**
  - **What:** `sql.Drivers()` contains `"sqlite3"` and `sql.Open("sqlite3", ":memory:")` returns no error.
  - **Steps:**
    1. Apply fix on a clean working tree.
    2. Run `go test -tags=integration -run TestSQLiteDriverRegistered ./tests/integration/ -v`.
  - **Expected:** `--- PASS: TestSQLiteDriverRegistered`.
  - **Verify:** `go test` output.
  - **Evidence:** report.md → Scope 1 → Post-fix GREEN — sentinel passes (`sql.Drivers() = [sqlite sqlite3]`).
  - **Notes:** Adversarial — must FAIL when blank import is removed. Pre-fix RED captured in report.md (`sql.Drivers() = []`).

- [x] **All 6 browser-history integration tests execute and pass**
  - **What:** No `--- SKIP` outcomes; all 6 tests run end-to-end against the deterministic SQLite fixture on the live integration stack.
  - **Steps:**
    1. Apply fix.
    2. Run `./smackerel.sh test integration` (or `go test -tags=integration -run TestBrowserHistory ./tests/integration/ -v`).
  - **Expected:** 6× `--- PASS`, 0× `--- SKIP`, 0× `--- FAIL`.
  - **Verify:** `go test` output captured in report.md.
  - **Evidence:** report.md → Scope 1 → Post-fix GREEN runs (standalone tag run + `./smackerel.sh test integration` excerpt). 6 PASS, 0 SKIP, 0 FAIL across both runs.
  - **Notes:** Removing the blank import must turn these into FAILs (not skips). Pre-fix evidence (6× SKIP signature) is preserved in the report.md "Pre-fix evidence" block.

- [ ] **Live-stack E2E exercises the Chrome-history Sync path**
  - **What:** A live-stack E2E test runs Sync() against the deterministic fixture, observes a RawArtifact published to NATS, and verifies the artifact propagates through the pipeline.
  - **Steps:**
    1. `./smackerel.sh up`
    2. `./smackerel.sh test e2e`
  - **Expected:** E2E suite includes a browser-history scenario that PASSES.
  - **Verify:** `./smackerel.sh test e2e` output.
  - **Evidence:** report.md#scope-1
  - **Notes:** Required to flip the parent feature uservalidation "E2E tests pass against live stack" item.

- [ ] **Parent feature uservalidation re-certified by `bubbles.validate`**
  - **What:** Both currently-failing items in `specs/010-browser-history-connector/uservalidation.md` flip from `[ ]` (with VERIFIED FAIL note) to `[x]` (with bubbles.validate sign-off).
  - **Steps:**
    1. After Scope 1 closes, invoke `bubbles.validate` against `specs/010-browser-history-connector/`.
  - **Expected:** "Integration tests pass against real SQLite fixture" → `[x]`; "E2E tests pass against live stack" → `[x]`.
  - **Verify:** `git diff specs/010-browser-history-connector/uservalidation.md`.
  - **Evidence:** report.md#scope-1
  - **Notes:** Closes the parent certification gate.

- [x] **Change Boundary respected**
  - **What:** `git diff --name-only` shows only files in the allowed Change Boundary list from [scopes.md](scopes.md).
  - **Steps:**
    1. After fix is committed, run `git diff --name-only main..HEAD`.
  - **Expected:** Only `go.mod`, `go.sum`, `internal/connector/browser/sqlite_driver.go` (new), `tests/integration/browser_history_test.go`, `tests/integration/browser_history_driver_test.go` (new), and optionally `data/browser-history/History/History` + `scripts/seed_browser_history_fixture.*`.
  - **Expected exclusions:** `internal/connector/browser/browser.go` semantic logic untouched (only acceptable change is none, OR — if `bubbles.design` resolves Q2 to "rename driver" — the single string `"sqlite3"` → `"sqlite"`).
  - **Verify:** `git diff --name-only` and per-file inspection.
  - **Evidence:** report.md → Scope 1 → Code Diff Evidence (`git status --short` shows exactly the 5 boundary files: `M go.mod`, `M go.sum`, `M tests/integration/browser_history_test.go`, `?? internal/connector/browser/sqlite_driver.go`, `?? tests/integration/browser_history_driver_test.go`). `internal/connector/browser/browser.go` is untouched.
  - **Notes:** Adversarial — any out-of-boundary file change must block certification.

- [x] **No silent-pass bailouts in regression tests**
  - **What:** `TestSQLiteDriverRegistered` contains no `t.Skip` or `if err != nil { return }` around `sql.Open`. The 6 `TestBrowserHistorySync_*` tests are NOT loosened to skip when the driver is missing.
  - **Steps:**
    1. `grep -nE 't\.Skip|return$' tests/integration/browser_history_driver_test.go`
    2. Inspect setup helpers in `tests/integration/browser_history_test.go` for new bailout patterns.
  - **Expected:** Driver sentinel has zero `t.Skip` calls; integration tests no longer skip on missing driver.
  - **Verify:** `grep` output + manual review.
  - **Evidence:** report.md → Scope 1 → Adversarial test source — `grep -nE 't\.Skip\(|if err != nil \{ return' tests/integration/browser_history_driver_test.go` returns exit=1 (zero matches). The previous silent-skip path in `requireFixture` was replaced with an on-demand seeder that calls `t.Fatalf` on any failure.
  - **Notes:** Enforces the adversarial regression contract from spec.md A5.

Unchecked items above represent open fix-validation gates. They flip to
`[x]` only after the implementation, test, and validate phases produce real
evidence in report.md.
