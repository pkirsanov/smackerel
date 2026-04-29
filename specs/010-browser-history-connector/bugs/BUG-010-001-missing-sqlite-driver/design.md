# BUG-010-001 — Fix design (skeleton)

> **Status:** SKELETON — root-cause analysis is complete from triage; final
> fix-design decisions (driver choice mechanism, fixture strategy) to be
> locked by `bubbles.design` when fix work is dispatched.

---

## Root cause analysis

### Investigation summary

- `internal/connector/browser/browser.go:111` calls `sql.Open("sqlite3", dbPath+"?mode=ro")`.
- `grep -rn 'mattn/go-sqlite3\|modernc.org/sqlite\|glebarez' go.mod go.sum internal/ cmd/` returns zero matches.
- `database/sql` requires a driver to be registered (typically via blank import) before `sql.Open` can resolve a driver name. With no driver registered, `sql.Drivers()` returns `[]` and any `sql.Open("sqlite3", ...)` returns `sql: unknown driver "sqlite3" (forgotten import?)`.
- The defect is masked because:
  1. Unit tests (`internal/connector/browser/*_test.go`) never call `sql.Open` — they exercise the parsing/aggregation/dwell-tier logic with synthetic in-memory `HistoryEntry` slices.
  2. Integration tests (`tests/integration/browser_history_test.go`) gate on a fixture that does not exist (`data/browser-history/History/` is an empty directory), and the helper `requireFixture()` calls `t.Skip(...)` rather than failing — so the missing driver is never reached.
  3. Live-stack E2E was previously blocked by separate stack issues (now resolved per parent uservalidation), so the missing driver was never surfaced there either.

### Root cause (precise)

The browser-history connector's design (`specs/010-browser-history-connector/design.md` line 45) explicitly specifies "CGo dependency: `database/sql` + `go-sqlite3` for reading Chrome's SQLite". The `database/sql` portion was wired up; the SQLite driver dependency was never added to `go.mod`, never blank-imported, and therefore never registered with `database/sql`.

Net: a contract gap between design and implementation that was hidden by skip-on-missing-fixture test gating.

### Impact analysis

- **Affected components:** `internal/connector/browser/` (browser-history connector). No other connector uses SQLite today, so blast radius is limited to this connector.
- **Affected data:** None at rest — the connector cannot read any Chrome history data, so no malformed data has been ingested.
- **Affected users:** Anyone enabling the browser-history connector. The connector is gated `disabled` by default in `config/smackerel.yaml`, so production exposure is zero until enabled.
- **Affected certifications:** Parent feature `010-browser-history-connector` cannot be marked done. Two uservalidation items remain unchecked.

## Fix design

### Solution approach (preferred)

1. **Add `modernc.org/sqlite` to `go.mod`** via `go get modernc.org/sqlite@latest`.
2. **Create `internal/connector/browser/sqlite_driver.go`** containing:
   - A blank import of `modernc.org/sqlite` (registers driver name `"sqlite"`).
   - An `init()` that calls `sql.Register("sqlite3", &sqlite.Driver{})` (or `&sqliteDriver{}` per the package's exported driver type) to alias the same driver under the name `"sqlite3"` that the production code at `browser.go:111` expects. This avoids touching production call sites.
3. **Generate a deterministic Chrome-shaped SQLite fixture** at `data/browser-history/History/History`:
   - Either commit a small (<50 KB) seeded fixture with a documented seed script under `scripts/` (e.g., `scripts/seed_browser_history_fixture.go` or `.py`), OR
   - Generate the fixture in `tests/integration/browser_history_test.go` setup using the now-registered `"sqlite3"` driver (preferred because it avoids committing binary files and proves the driver works end-to-end inside the same test run).
   - Schema must match Chrome's `urls` and `visits` tables minimally:
     - `urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT, ...)`
     - `visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, visit_duration INTEGER)`
   - Seed rows MUST cover the scenarios the 6 integration tests assert (initial import, incremental cursor, full pipeline, social-media aggregation, repeat-visit escalation, privacy gate).
4. **Add an adversarial driver-presence sentinel test** (e.g., `tests/integration/browser_history_driver_test.go::TestSQLiteDriverRegistered`) that:
   - Asserts `slices.Contains(sql.Drivers(), "sqlite3")` is true.
   - Calls `sql.Open("sqlite3", ":memory:")` and asserts `err == nil` and the returned `*sql.DB` is non-nil.
   - Has NO `t.Skip` or bailout. Removing the blank import in step 2 must make this test fail.
5. **Verify the 6 currently-skipped integration tests now execute and pass** on the live integration stack.

### Alternative approaches considered

1. **`github.com/mattn/go-sqlite3` (CGO).** Rejected unless the pure-Go option is blocked: forces CGO into the Go core runtime build, regresses the CGO-free posture used by `./smackerel.sh build`, and complicates Docker image builds.
2. **Migrate `sql.Open("sqlite3", ...)` to `sql.Open("sqlite", ...)` and skip the alias.** Smaller diff but changes the production call site for a contract that the design doc names `sqlite3`. Rejected to keep the production code change boundary minimal — the alias is a single `sql.Register` line.
3. **Commit a real Chrome `History` file as fixture.** Rejected: privacy risk, opaque binary in version control, hard to maintain. Deterministic seeded fixture is preferred.
4. **Lift the skip gate without adding a driver.** Rejected — would make the 6 tests fail loudly, but does not fix the actual defect (no driver). Tests would fail at `sql.Open` instead of skipping.

### Open questions for `bubbles.design` to resolve before implementation

- **Q1.** Confirm `modernc.org/sqlite` is acceptable under the repo's dependency policy (no GPL conflict, acceptable supply-chain provenance). If not, fall back to `mattn/go-sqlite3` and accept the CGO change.
- **Q2.** Confirm whether the alias `sql.Register("sqlite3", &sqlite.Driver{})` is the chosen mechanism (preferred) vs. migrating the production call site to `"sqlite"` (smaller dependency surface but larger production diff).
- **Q3.** Confirm fixture strategy: commit a seeded SQLite file vs. generate in test setup. Recommendation: generate in test setup (no binary in git, proves driver round-trip).
- **Q4.** Confirm whether the driver registration file should live in `internal/connector/browser/` (scoped to the only consumer) or in a shared `internal/db/sqlite/` package (future-proofing). Recommendation: keep in `internal/connector/browser/` until a second consumer appears, per YAGNI.

### Files expected to change

| File | Change | Owner |
|------|--------|-------|
| `go.mod` | `+ require modernc.org/sqlite vX.Y.Z` (and indirect deps) | `bubbles.implement` |
| `go.sum` | Updated checksums | `bubbles.implement` (via `go mod tidy`) |
| `internal/connector/browser/sqlite_driver.go` | New file: blank import + `sql.Register("sqlite3", ...)` | `bubbles.implement` |
| `tests/integration/browser_history_test.go` | Optional: add `setupChromeFixture(t)` helper that creates the deterministic SQLite file in a temp dir or in `data/browser-history/History/`; remove or invert the `t.Skip` once the helper is wired in | `bubbles.implement` |
| `tests/integration/browser_history_driver_test.go` | New file: adversarial driver-presence sentinel | `bubbles.implement` |
| `data/browser-history/History/History` | Committed deterministic SQLite fixture (only if Q3 resolves to "commit"); otherwise no change here | `bubbles.implement` |
| `scripts/seed_browser_history_fixture.*` | Optional seed script if fixture is committed | `bubbles.implement` |

### Regression test design

| Test | Type | Pre-fix expectation | Post-fix expectation |
|------|------|---------------------|----------------------|
| `TestSQLiteDriverRegistered` (new) | integration | FAIL (`sql.Drivers()` empty, `sql.Open` returns unknown-driver error) | PASS |
| `TestBrowserHistorySync_InitialImport` | integration | SKIP | PASS |
| `TestBrowserHistorySync_IncrementalCursor` | integration | SKIP | PASS |
| `TestBrowserHistorySync_FullPipelineFlow` | integration | SKIP | PASS |
| `TestBrowserHistorySync_SocialMediaAggregation` | integration | SKIP | PASS |
| `TestBrowserHistorySync_RepeatVisitEscalation` | integration | SKIP | PASS |
| `TestBrowserHistorySync_FullPipeline_WithAggregationAndPrivacy` | integration | SKIP | PASS |

The fix is incomplete unless ALL rows above flip to PASS post-fix, AND the parent uservalidation checklist re-certifies green.
