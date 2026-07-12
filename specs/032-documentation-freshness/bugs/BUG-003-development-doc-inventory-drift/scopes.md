# Bug Scopes — BUG-003: `docs/Development.md` inventory drift with no freshness guard

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Document the drifted package and add a fail-loud doc-freshness guard

**Status:** Done
**Priority:** P1
**Depends On:** None
**Scope-Kind:** contract-only

### Use Cases

```gherkin
Feature: docs/Development.md inventories stay mechanically in sync with the codebase
  Scenario: SCN-032-D12 — Every internal/ Go package is documented and the guard enforces it
    Given internal/scopesdriftguard/ exists on disk but is absent from the docs/Development.md Go Packages table
    And the doc-freshness guard test internal/docfreshness/doc_freshness_test.go is present
    When the Go unit suite runs the guard before the documentation is corrected
    Then TestDocFreshness_AllInternalPackagesDocumented fails listing the undocumented package(s)
    And after the Go Packages table documents every internal/ package the same test passes

  Scenario: SCN-032-D13 — Every migration and prompt contract is documented and the guard enforces it
    Given internal/db/migrations/*.sql and config/prompt_contracts/*.yaml are enumerable on disk
    When the doc-freshness guard runs
    Then TestDocFreshness_AllMigrationsDocumented asserts all 38 migration files appear in docs/Development.md
    And TestDocFreshness_AllPromptContractsDocumented asserts all 21 prompt contracts appear in docs/Development.md

  Scenario: SCN-032-D14 — The guard is not tautological and detects an undocumented item
    Given a synthetic repo whose docs/Development.md documents none of its inventories
    When the adversarial test scans the synthetic package, migration, and contract
    Then each scan reports exactly its synthetic missing item
    And a documented item is reported present, proving the guard passes only when the inventory is documented
```

### Implementation Files

| File | Action |
|------|--------|
| [internal/docfreshness/doc_freshness_test.go](../../../../internal/docfreshness/doc_freshness_test.go) | NEW test-only package — fail-loud contract guard for the three docs/Development.md inventories plus an adversarial anti-tautology case |
| [docs/Development.md](../../../../docs/Development.md) | Go Packages table — add `internal/scopesdriftguard/` and `internal/docfreshness/` rows (no removals) |

### Change Boundary

This scope adds one test-only Go file and two documentation rows. The boundary is narrow.

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `internal/docfreshness/doc_freshness_test.go` (new test-only file) | Any runtime `.go` (non-test), `.py`, `.sql`, `.proto`, `.toml` |
| `docs/Development.md` (Go Packages table insertion only) | `config/`, `docker-compose*.yml`, `Dockerfile`, `ml/Dockerfile` |
| Bug-packet artifacts under `specs/032-documentation-freshness/bugs/BUG-003-development-doc-inventory-drift/` | `.github/bubbles/**`, `.github/workflows/**`, `.specify/memory/` |
| Parent `specs/032-documentation-freshness/report.md` (evidence breadcrumb only) | `scripts/commands/build-self-hosted.sh` (in-progress external work) and all other `scripts/` source |
| | Parent `spec.md` / `design.md` / `scopes.md` (no planning-truth edit) |

### Consumer Impact Sweep

| Consumer surface | Impact | Action taken |
|------------------|--------|--------------|
| `./smackerel.sh test unit --go` + CI `go test ./...` | Gains 4 fast file-reading tests in `internal/docfreshness`; no runtime deps, no network, no DB. | None — verified passing on this tree (`ok internal/docfreshness`). |
| `docs/Development.md` readers / deep links | Two additive Go Packages rows; existing anchors and rows unchanged. | None — additive only. |
| Future package/migration/contract authors | Adding one without documenting it now fails the Go unit suite + CI. | Intended new control; documented in the guard's doc comment and the new table row. |
| `internal/scopesdriftguard/` | Unrelated guard package, not modified. | None. |

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOCFRESH-001 | go-contract | `internal/docfreshness/doc_freshness_test.go` | SCN-032-D12 | `TestDocFreshness_AllInternalPackagesDocumented` — every `internal/` package appears in `docs/Development.md` (34 on disk, 0 undocumented after fix) |
| T-DOCFRESH-002 | go-contract | `internal/docfreshness/doc_freshness_test.go` | SCN-032-D13 | `TestDocFreshness_AllMigrationsDocumented` (38/38) + `TestDocFreshness_AllPromptContractsDocumented` (21/21) |
| T-DOCFRESH-003 | go-contract regression (adversarial) | `internal/docfreshness/doc_freshness_test.go` | SCN-032-D14 | `TestDocFreshness_AdversarialUndocumentedItemsDetected` — synthetic undocumented package/migration/contract each detected; documented item reported present |

### Definition of Done

- [x] Every `internal/` Go package is documented in `docs/Development.md` and the guard enforces it. [SCN-032-D12: Every internal/ Go package is documented and the guard enforces it]
   → Evidence: `internal/scopesdriftguard/` + `internal/docfreshness/` rows added to the Go Packages table; `TestDocFreshness_AllInternalPackagesDocumented` reports `34 packages on disk, 0 undocumented` (was `2 undocumented: docfreshness, scopesdriftguard` before the rows). See report.md §Before/After.
- [x] Every migration and prompt contract is documented and the guard enforces it. [SCN-032-D13: Every migration and prompt contract is documented and the guard enforces it]
   → Evidence: `TestDocFreshness_AllMigrationsDocumented` reports `38 migration files on disk, 0 undocumented`; `TestDocFreshness_AllPromptContractsDocumented` reports `21 contracts on disk, 0 undocumented`. See report.md §After Fix.
- [x] The doc-freshness guard is fail-loud and not tautological. [SCN-032-D14: The guard is not tautological and detects an undocumented item]
   → Evidence: `TestDocFreshness_AdversarialUndocumentedItemsDetected` passes — it asserts the scans report exactly the synthetic missing items and that the green path passes once documented. See report.md §After Fix.
- [x] Targeted tests pass — T-DOCFRESH-001, T-DOCFRESH-002, T-DOCFRESH-003.
   → Evidence: `./smackerel.sh test unit --go --go-run 'TestDocFreshness'` → `ok github.com/smackerel/smackerel/internal/docfreshness`; per-test logs captured in report.md §After Fix.
- [x] Scenario-specific regression tests for EVERY new/changed/fixed behavior — the adversarial contract test T-DOCFRESH-003 is a persistent regression that fails if the guard is ever neutered or the bug reintroduced (an undocumented package added without a doc row).
   → Evidence: `TestDocFreshness_AdversarialUndocumentedItemsDetected` builds a synthetic repo and asserts detection; re-running the Go unit suite reproduces the pass condition. See report.md §Regression Evidence.
- [x] Broader regression suite passes — full `go test ./...` compiled clean and `go vet ./...` clean (runtime byte-identical; only a new test file + doc rows were added).
   → Evidence: `./smackerel.sh lint` → exit 0 (`go vet ./...` clean, ruff `All checks passed!`, web validation passed); `./smackerel.sh format --check` → exit 0. See report.md §Audit Evidence.
- [x] Consumer impact sweep complete and zero stale first-party references remain.
   → Evidence: see Consumer Impact Sweep above; the only consumers are the Go unit suite (passing) and `docs/Development.md` readers (additive rows). `git status --short -- '*.go'` lists only `internal/docfreshness/doc_freshness_test.go`.
- [x] Change boundary respected: only the allowed surfaces were modified; excluded surfaces are untouched.
   → Evidence: `git status --short` shows the only non-framework working-tree changes are `internal/docfreshness/doc_freshness_test.go` (new), `docs/Development.md`, the parent `report.md` breadcrumb, and this bug packet. No runtime `.go`/`.py`/`.sql`, no `config/`, no `.github/bubbles/**`, no `build-self-hosted.sh`.
- [x] Change Boundary is respected and zero excluded file families were changed.
   → Evidence: same as prior line; `git status --short -- '*.py' '*.sql' config/ docker-compose.yml docker-compose.prod.yml Dockerfile ml/Dockerfile` returns zero bug-related matches.
