# Scopes: BUG-020-003 — `cmd/core/helpers.go` env-reading helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) silently fall back to `0` / `nil` on missing or malformed env vars, codifying the FORBIDDEN fail-soft pattern under Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Execution Outline

A reviewer-facing alignment checkpoint. Read this before the full scope detail below.

**Phase Order** — single sequential scope (one coherent vertical slice):

1. **Scope 1: Delete the 5 dead-set helpers + 31 dead-set tests; prune 3 orphaned imports; add persistent AST regression guard.** Touches three files in `cmd/core/`: edit [`cmd/core/helpers.go`](../../../../cmd/core/helpers.go) (delete 5 functions, prune 3 imports), edit [`cmd/core/main_test.go`](../../../../cmd/core/main_test.go) (delete 31 test functions), create new file [`cmd/core/helpers_no_defaults_test.go`](../../../../cmd/core/helpers_no_defaults_test.go) (AST-based regression guard). Validation: `gofmt -l .` clean, `go vet ./...` exit 0, `./smackerel.sh test unit --go` exit 0, `./smackerel.sh build` exit 0, byte-identical `cmd/core/connectors.go`.

**New Types & Signatures** — what changes shape:

- **Removed (5)** from `cmd/core/helpers.go`:
  - `func parseJSONArrayEnv(key string) []interface{}` — env→[]interface{}, silent-`nil` fallback
  - `func parseJSONObject(s string) map[string]interface{}` — string→map, silent-`nil` fallback
  - `func parseJSONObjectEnv(key string) map[string]interface{}` — env→map, silent-`nil` fallback
  - `func parseJSONObjectVal(key, s string) map[string]interface{}` — internal worker for parseJSONObject* (transitively dead)
  - `func parseFloatEnv(key string) float64` — env→float64, silent-`0` fallback (3 silent-fallback branches: empty, parse-error, non-finite)
- **Removed (3)** orphaned imports from `cmd/core/helpers.go`:
  - `"math"` (only used by `parseFloatEnv`)
  - `"os"` (only used by the three `Env`-suffixed helpers)
  - `"strconv"` (only used by `parseFloatEnv`)
- **Preserved (2)** in `cmd/core/helpers.go`:
  - `func parseJSONArray(s string) []interface{}` — LIVE: 2 callers in `cmd/core/connectors.go:76,103`
  - `func parseJSONArrayVal(key, s string) []interface{}` — LIVE: called by `parseJSONArray`
- **Added (1)** in new file `cmd/core/helpers_no_defaults_test.go`:
  - `func TestNoSilentFallbackHelpersInCmdCore(t *testing.T)` — AST-walks every non-test `*.go` file under `cmd/core/`, fails loud if any function body matches the silent-fallback signature shape (`os.Getenv(...)` followed by return-on-empty path without error propagation). Pattern follows repo precedent at [`internal/drive/consumers/consumer_contract_test.go`](../../../../internal/drive/consumers/consumer_contract_test.go).

**Validation Checkpoints** — where breakage is caught:

- **After Step 4 (test deletion):** `go build ./cmd/core/...` MUST succeed. Catches accidental deletion of a still-used symbol or accidental orphaning of an import on the test side.
- **After Step 6 (gofmt):** `gofmt -l .` MUST return empty. Catches stray whitespace / formatting from manual edits.
- **After Step 7 (go vet):** `go vet ./...` MUST exit 0. Catches unused imports + obvious correctness issues.
- **After Step 8 (unit suite):** `./smackerel.sh test unit --go` MUST exit 0. The new `TestNoSilentFallbackHelpersInCmdCore` MUST pass; the broader `cmd/core` suite MUST pass with the test-count delta of −31.
- **After Step 9 (build):** `./smackerel.sh build` MUST exit 0. Catches any deeper build failure not caught by `go vet`.
- **After Step 10 (no-change canary):** `git diff cmd/core/connectors.go` MUST return empty for the two live `parseJSONArray` call sites at lines 76 and 103.

---

## Overview

This packet closes self-hosted readiness rescan finding HL-RESCAN-014 (severity P3, lens "SST defaults", surface `cmd/core/helpers.go`) by the **Option A pure-deletion** path documented in [`design.md`](design.md) DD-1. The 5 dead-set helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal`) have ZERO production callers and codify exactly the fail-soft env-read shape that Gate G028 (NO-DEFAULTS / fail-loud SST policy per [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md) and [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md) "SST Zero-Defaults Enforcement") explicitly forbids. They survive only because 31 test functions in `cmd/core/main_test.go` lock the silent-fallback semantics in place. Deleting both the helpers and their tests in the same change unblocks Gate G028 audit-cleanliness AND removes the copy-paste-into-new-helper hazard.

The two LIVE production callers of `parseJSONArray` at `cmd/core/connectors.go:76` (`parseJSONArray(cfg.BookmarksExcludeDomains)`) and `cmd/core/connectors.go:103` (`parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`) are explicitly **outside this packet's change boundary** per [`spec.md`](spec.md) Bounded Surface section and [`design.md`](design.md) DD-2 / DD-6. The `parseJSONArray` and `parseJSONArrayVal` helpers are PRESERVED, and the 8 `TestParseJSONArray_*` test cases (7 main + 1 backward-compat) are PRESERVED. Their fail-soft `nil`-on-parse-error wiring is documented as a candidate sequel packet (BUG-020-004) in [`uservalidation.md`](uservalidation.md) for the operator to file if/when they choose to take it on.

The fix is mechanically a single vertical slice: edit `cmd/core/helpers.go`, edit `cmd/core/main_test.go`, create `cmd/core/helpers_no_defaults_test.go`, validate via the standard repo CLI. No production runtime behaviour changes (the deleted helpers had zero production callers); the only operator-visible delta is `cmd/core/main_test.go` shrinks by 31 test functions and a new persistent regression guard runs on every Go unit lane invocation.

## Scope Ordering Rationale

This packet has exactly **one scope**. The work is a single vertical slice with no horizontal layering — the deletion of the 5 helpers, the deletion of the 31 corresponding tests, the import-pruning, and the new regression guard are mechanically interdependent (deleting the helpers without pruning imports breaks the build; deleting the helpers without deleting the tests breaks the test compile; adding the regression guard without deleting the helpers makes the guard fail RED). Splitting into multiple scopes would create transient broken intermediate states that cannot be validated independently. The single-scope plan delivers the entire HL-RESCAN-014 closure in one self-validating change.

## Scope Inventory

| # | Scope | Surfaces | Tests | DoD Summary | Status |
|---|-------|----------|-------|-------------|--------|
| 1 | Delete dead-set helpers and tests; add AST regression guard | `cmd/core/helpers.go`, `cmd/core/main_test.go`, NEW `cmd/core/helpers_no_defaults_test.go` | go-unit (1 new persistent guard) + go-unit-canary (full `cmd/core` suite + `./smackerel.sh test unit --go`) + symbol-removal audit + no-change canary on `cmd/core/connectors.go` | 18 DoD items (DoD-1 through DoD-18); all 7 spec ACs covered; 5 scenarios SCN-020-003-A..E mapped | Done |

---

## Scope 1: Delete dead-set helpers and tests; add AST regression guard

**Status:** Done

### Files

| File | Action | Reason |
|------|--------|--------|
| [`cmd/core/helpers.go`](../../../../cmd/core/helpers.go) | EDIT — delete 5 functions; prune 3 imports | Remove the 5 dead-set helpers per [`design.md`](design.md) DD-1: `parseJSONArrayEnv` (lines 19-22), `parseJSONObject` (lines 39-41), `parseJSONObjectEnv` (lines 45-48), `parseJSONObjectVal` (lines 51-61), `parseFloatEnv` (lines 65-82). Prune the now-orphaned imports `math`, `os`, `strconv` per [`design.md`](design.md) DD-4. PRESERVE `parseJSONArray` (line 13-15) and `parseJSONArrayVal` (line 25-36) — both LIVE in production via `cmd/core/connectors.go:76,103`. The post-fix import block contains exactly two imports: `"encoding/json"`, `"log/slog"`. Net delta: −5 functions / −3 imports / approximately −60 lines. |
| [`cmd/core/main_test.go`](../../../../cmd/core/main_test.go) | EDIT — delete 31 test functions | Remove the 31 dead-set test functions per [`design.md`](design.md) DD-3 (full enumeration in the "Dead-Set Test Functions to Delete" subsection below). PRESERVE the 8 `TestParseJSONArray_*` test cases (lines 109-161 + line ~457-459 backward-compat) because `parseJSONArray` is LIVE. PRESERVE `TestGovAlertsSourceEarthquakeWiring_*` (3 functions), `TestWeatherEnableAlertsWiring` + `TestWeatherEnableAlertsWiring_Disabled`, and `TestMarketsFreddEnabledWiring_*` (3 functions) because they use `os.Getenv` directly (not via the dead helpers) and assert separate Source-Config wiring contracts. Net delta: −31 test functions / approximately −300 lines. |
| `cmd/core/helpers_no_defaults_test.go` (NEW) | CREATE — persistent AST-based regression guard | Author the new persistent in-tree adversarial regression guard per [`design.md`](design.md) DD-5. Single test function `TestNoSilentFallbackHelpersInCmdCore(t *testing.T)` AST-walks every `*.go` file under `cmd/core/` (excluding `_test.go` files), parses each via `go/parser` + `go/token`, inspects each top-level function declaration for the silent-fallback signature shape (function body contains a call to `os.Getenv(...)` whose subsequent control flow returns a literal value or `nil` on the empty-string branch without producing an error), and fails loud with `t.Errorf("file:line: function <name> matches silent-fallback signature shape — Gate G028 / HL-RESCAN-014")`. Pattern follows the repo precedent at [`internal/drive/consumers/consumer_contract_test.go`](../../../../internal/drive/consumers/consumer_contract_test.go) (lines 1-200 — same `go/parser` + `go/token` walk + per-file failure aggregation + scanned-file-count guard against silent empty-workspace pass). Includes a scanned-file-count sanity check (≥3 files MUST be parsed; fails the test if the walk finds nothing — guards against the test silently passing if `cmd/core/` is moved or restructured). Includes a docstring naming `Gate G028` and `HL-RESCAN-014` so the failure surface is self-documenting. |
| [report.md](report.md) | EDIT — populate with implementation evidence (downstream phases) | Each downstream specialist appends evidence to its owned section per the existing stub structure. Required evidence per section: pre-fix grep / RED-proof, post-fix code diffs, suite-pass output (raw, last 20 lines per claim), AST-guard RED-to-GREEN proof, generic-only constraint verification, gitleaks scan output. PII paths MUST be redacted to `~/smackerel`. |
| [`specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/uservalidation.md`](uservalidation.md) | EDIT — populate AC verification + flip to `[x]` baseline + add BUG-020-004 sequel pointer (downstream `bubbles.validate` phase) | The `bubbles.validate` specialist flips each AC checkbox to `[x]` with inline evidence references after the fix lands and validation passes. The "Sequel Surfaces" section (added by `bubbles.plan` per [`design.md`](design.md) DD-6) documents BUG-020-004 candidate ("`cmd/core/connectors.go` `parseJSONArray` live callers silently coerce parse errors to empty exclusion lists; convert to fail-loud `(value, error)` reads or refuse connector construction"). |
| [`specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/state.json`](state.json) | EDIT — advance through phases | The `bubbles.plan` specialist (this invocation) advances `currentPhase: design → plan` and appends `executionHistory` + `completedPhaseClaims` for the design phase. Subsequent specialists advance through `implement → test → regression → simplify → stabilize → security → validate → audit`, each setting `currentPhase`, `execution.activeAgent`, and appending `executionHistory` per the standard control-plane contract. Final `certification.status` MUST equal `passed` with all phases in `completedPhases` and zero pending `transitionRequests` / `reworkQueue` entries. |

#### Dead-Set Helpers to Delete from `cmd/core/helpers.go`

Verified by `grep_search` cross-package call-site evidence in [`spec.md`](spec.md) Detection > Verified Call-Site Inventory. ZERO production callers for each:

| # | Symbol | Line range (HEAD) | Production callers | Test callers |
|---|--------|-------------------|--------------------|--------------|
| 1 | `parseJSONArrayEnv(key string) []interface{}` | 19-22 | 0 | 3 (TestParseJSONArrayEnv_ValidArray, TestParseJSONArrayEnv_EmptyVar, TestParseJSONArrayEnv_InvalidJSON) + 2 transitive (TestMarketsFredSeriesWiring, TestMarketsFredSeriesWiring_Empty) |
| 2 | `parseJSONObject(s string) map[string]interface{}` | 39-41 | 0 | 6 main + 1 backward-compat |
| 3 | `parseJSONObjectEnv(key string) map[string]interface{}` | 45-48 | 0 | 3 (TestParseJSONObjectEnv_ValidObject, TestParseJSONObjectEnv_EmptyVar, TestParseJSONObjectEnv_InvalidJSON) |
| 4 | `parseJSONObjectVal(key, s string) map[string]interface{}` | 51-61 | 0 (called only by `parseJSONObject` + `parseJSONObjectEnv`, both dead) | 0 (called only transitively) |
| 5 | `parseFloatEnv(key string) float64` | 65-82 | 0 | 13 main + 3 transitive (TestWeatherForecastDaysWiring, TestWeatherPrecisionWiring, TestWeatherForecastDaysWiring_ZeroOnEmpty) |

#### Imports to Prune from `cmd/core/helpers.go`

| Import | Why orphaned post-deletion |
|--------|---------------------------|
| `"math"` | Used only by `parseFloatEnv` (`math.IsNaN`, `math.IsInf`) |
| `"os"` | Used only by `parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv` (`os.Getenv`) |
| `"strconv"` | Used only by `parseFloatEnv` (`strconv.ParseFloat`) |

#### Imports Preserved in `cmd/core/helpers.go`

| Import | Why preserved |
|--------|--------------|
| `"encoding/json"` | Used by `parseJSONArrayVal` (live, `json.Unmarshal`) |
| `"log/slog"` | Used by `parseJSONArrayVal` (live, `slog.Warn` on parse error) |

#### Helpers Preserved in `cmd/core/helpers.go`

| Symbol | Line range (HEAD) | Why preserved |
|--------|-------------------|---------------|
| `parseJSONArray(s string) []interface{}` | 13-15 | LIVE: 2 callers in `cmd/core/connectors.go:76` (`parseJSONArray(cfg.BookmarksExcludeDomains)`) + line 103 (`parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`) |
| `parseJSONArrayVal(key, s string) []interface{}` | 25-36 | LIVE via `parseJSONArray` |

#### Dead-Set Test Functions to Delete from `cmd/core/main_test.go`

Total: **31 functions**. Enumerated by name and bucket per [`design.md`](design.md) DD-3:

**Bucket A — `parseJSONObject` direct tests (6 functions, lines ~166-222):**

1. `TestParseJSONObject_ValidObject`
2. `TestParseJSONObject_EmptyString`
3. `TestParseJSONObject_EmptyObject`
4. `TestParseJSONObject_InvalidJSON`
5. `TestParseJSONObject_NotAnObject`
6. `TestParseJSONObject_NestedObject`

**Bucket B — `parseFloatEnv` direct tests (13 functions, lines ~227-335):**

7. `TestParseFloatEnv_ValidFloat`
8. `TestParseFloatEnv_Integer`
9. `TestParseFloatEnv_EmptyString`
10. `TestParseFloatEnv_UnsetVar`
11. `TestParseFloatEnv_InvalidFloat`
12. `TestParseFloatEnv_NegativeFloat`
13. `TestParseFloatEnv_Zero`
14. `TestParseFloatEnv_ScientificNotation`
15. `TestParseFloatEnv_Inf`
16. `TestParseFloatEnv_NegInf`
17. `TestParseFloatEnv_PosInf`
18. `TestParseFloatEnv_NaN`
19. `TestParseFloatEnv_NaN_Lowercase`

**Bucket C — `parseJSONArrayEnv` direct tests (3 functions, lines ~405-427):**

20. `TestParseJSONArrayEnv_ValidArray`
21. `TestParseJSONArrayEnv_EmptyVar`
22. `TestParseJSONArrayEnv_InvalidJSON`

**Bucket D — `parseJSONObjectEnv` direct tests (3 functions, lines ~431-453):**

23. `TestParseJSONObjectEnv_ValidObject`
24. `TestParseJSONObjectEnv_EmptyVar`
25. `TestParseJSONObjectEnv_InvalidJSON`

**Bucket E — `parseJSONObject` backward-compat (1 function, lines ~463-466):**

26. `TestParseJSONObject_BackwardCompat`

**Bucket F — Stale `Weather*Wiring` tests that transitively use `parseFloatEnv` (3 functions, lines ~568-595):**

27. `TestWeatherForecastDaysWiring`
28. `TestWeatherPrecisionWiring`
29. `TestWeatherForecastDaysWiring_ZeroOnEmpty`

**Bucket G — Stale `MarketsFredSeriesWiring` tests that transitively use `parseJSONArrayEnv` (2 functions, lines ~645-665):**

30. `TestMarketsFredSeriesWiring`
31. `TestMarketsFredSeriesWiring_Empty`

#### Test Functions Preserved in `cmd/core/main_test.go`

The implementer MUST NOT touch these:

- **8 `TestParseJSONArray_*` cases (lines ~109-161 + ~455-459):** `TestParseJSONArray_ValidArray`, `TestParseJSONArray_EmptyString`, `TestParseJSONArray_EmptyArray`, `TestParseJSONArray_InvalidJSON`, `TestParseJSONArray_MixedTypes`, `TestParseJSONArray_NestedArrays`, `TestParseJSONArray_NotAnArray`, `TestParseJSONArray_BackwardCompat` — `parseJSONArray` is LIVE.
- **3 `TestGovAlertsSourceEarthquakeWiring_*` cases (lines ~338-396):** `TestGovAlertsSourceEarthquakeWiring_Enabled`, `TestGovAlertsSourceEarthquakeWiring_Disabled`, `TestGovAlertsSourceEarthquakeWiring_UnsetDefaultsFalse` — direct `os.Getenv == "true"` reads, not via deleted helpers.
- **2 `TestWeatherEnableAlertsWiring*` cases (lines ~533-566):** `TestWeatherEnableAlertsWiring`, `TestWeatherEnableAlertsWiring_Disabled` — direct `os.Getenv == "true"` reads.
- **3 `TestMarketsFreddEnabledWiring_*` cases (lines ~600-643):** `TestMarketsFreddEnabledWiring_True`, `TestMarketsFreddEnabledWiring_False`, `TestMarketsFreddEnabledWiring_UnsetDefaultsFalse` — direct `os.Getenv == "true"` reads.
- **All other tests in `cmd/core/main_test.go`** — connector registration, runWithTimeout, shutdownAll, etc. — unrelated to the dead helpers.

### Use Cases (Gherkin)

```gherkin
Feature: cmd/core/helpers.go env-reading helpers no longer codify the FORBIDDEN fail-soft pattern (Gate G028 / HL-RESCAN-014)

  Scenario: SCN-020-003-A — Dead-set helper symbols no longer exist anywhere in the repo
    Given the post-fix tree (after the implement specialist deletes the 5 dead-set helpers and 31 dead-set tests)
    When grep_search across all *.go files runs for the dead-set symbols (parseFloatEnv, parseJSONArrayEnv, parseJSONObjectEnv, parseJSONObject, parseJSONObjectVal)
    Then the search returns ZERO matches anywhere in the repo
    And no source definitions remain in cmd/core/helpers.go
    And no test references remain in cmd/core/main_test.go
    And no comment mentions or build-tagged stubs remain
    And reverting any single deletion would cause this audit to fail with the explicit symbol-name and line number

  Scenario: SCN-020-003-B — cmd/core/helpers.go contains no os.Getenv silent-fallback patterns
    Given the post-fix tree
    When the AST-based pattern audit walks cmd/core/helpers.go
    Then no function body contains a call to os.Getenv(...) followed by a return-on-empty path that does not propagate an error
    And the imports block contains exactly "encoding/json" and "log/slog" (no "os", no "math", no "strconv")
    And introducing any new helper that matches the silent-fallback signature shape causes the audit to fail with the explicit file + line + function name

  Scenario: SCN-020-003-C — Persistent in-tree adversarial regression guard catches future re-introduction of the FORBIDDEN pattern
    Given the post-fix tree includes a new file cmd/core/helpers_no_defaults_test.go containing TestNoSilentFallbackHelpersInCmdCore
    When the test runs as part of every ./smackerel.sh test unit --go invocation
    Then the test PASSes against the post-fix tree (zero offending functions found)
    And the test FAILS RED if a future maintainer re-introduces any of the dead-set symbols (e.g., adds func parseFloatEnv(key string) float64 { ... } back to helpers.go)
    And the test FAILS RED if a future maintainer adds any new function matching the silent-fallback signature shape (e.g., func parseDurationEnv(key string) time.Duration { ... })
    And the failure message names the offending file + line + function name AND the breadcrumb tokens "Gate G028" and "HL-RESCAN-014"

  Scenario: SCN-020-003-D — Existing Go unit test suite passes unchanged after the deletion (suite-green canary)
    Given the post-fix tree with the 5 dead-set helpers and their 31 corresponding tests deleted
    When ./smackerel.sh test unit --go runs against the post-fix tree
    Then every Go test in the cmd/core package PASSes
    And the test count drops by exactly 31 (the deletion delta) plus +1 (the new TestNoSilentFallbackHelpersInCmdCore)
    And ZERO pre-existing tests for unrelated cmd/core code regress
    And go vet ./... exits 0
    And gofmt -l . returns empty

  Scenario: SCN-020-003-E — Live production callers of parseJSONArray remain untouched and behave identically (positive canary)
    Given the post-fix tree (the 5 dead-set helpers deleted, but parseJSONArray + parseJSONArrayVal preserved)
    When git diff cmd/core/connectors.go is captured
    Then lines 76 and 103 are byte-identical to HEAD (parseJSONArray(cfg.BookmarksExcludeDomains) and parseJSONArray(cfg.BrowserHistoryCustomSkipDomains) unchanged)
    And ./smackerel.sh build exits 0
    And the bookmarks + browser-history connector wiring blocks compile unchanged
    And the 8 preserved TestParseJSONArray_* cases continue to PASS
```

### Implementation Plan

Concrete, ordered steps for the `bubbles.implement` specialist to follow. Each step references the relevant design decision and produces a checkpoint validation.

1. **Re-verify call-site evidence is still accurate.** Run `grep_search` (or equivalent `vscode_listCodeUsages`) against HEAD for each of the 5 dead-set symbols (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal`) AND for the live `parseJSONArray` symbol. Expected: 5 dead-set symbols return ZERO hits in `cmd/`, `internal/`, `tests/` outside `cmd/core/main_test.go` and `cmd/core/helpers.go` itself; `parseJSONArray` returns exactly 2 hits in `cmd/core/connectors.go` (lines 76 and 103) plus its declaration in `cmd/core/helpers.go` plus the 8 preserved test cases in `cmd/core/main_test.go`. If any other production caller has appeared since this packet was authored, STOP and route back to `bubbles.design` for re-scoping. (Per [`design.md`](design.md) DD-2 / DD-6.)

2. **Delete the 5 dead-set helpers from `cmd/core/helpers.go`.** Use `replace_string_in_file` (or `multi_replace_string_in_file` for atomic application) to remove:
   - `parseJSONArrayEnv` and its leading doc comment (lines 17-22 inclusive)
   - `parseJSONObject` and its leading doc comment (lines 37-41 inclusive)
   - `parseJSONObjectEnv` and its leading doc comment (lines 43-48 inclusive)
   - `parseJSONObjectVal` and its leading doc comment (lines 50-61 inclusive)
   - `parseFloatEnv` and its leading doc comment (lines 63-82 inclusive)
   PRESERVE `parseJSONArray` (lines 11-15) and `parseJSONArrayVal` (lines 24-36). (Per [`design.md`](design.md) DD-1.)

3. **Prune orphaned imports from `cmd/core/helpers.go`.** Use `replace_string_in_file` to update the `import (...)` block from:

   ```go
   import (
       "encoding/json"
       "log/slog"
       "math"
       "os"
       "strconv"
   )
   ```

   to:

   ```go
   import (
       "encoding/json"
       "log/slog"
   )
   ```

   The post-fix file contains exactly two top-level functions (`parseJSONArray`, `parseJSONArrayVal`), the package declaration, and the two-import block. (Per [`design.md`](design.md) DD-4.)

4. **Delete the 31 dead-set test functions from `cmd/core/main_test.go`.** Use `multi_replace_string_in_file` for per-function precision. Delete the functions and their leading section-header comment lines (`// --- parseJSONObject tests ---`, `// --- parseFloatEnv tests ---`, `// --- CHAOS-019-001: parseFloatEnv must reject IEEE 754 special values ---`, `// --- H-019-002: parseJSONArrayEnv / parseJSONObjectEnv include key in logs ---`, `// --- H-019-002: backward compatibility — old parseJSONArray/parseJSONObject still work ---` for the BackwardCompat function only — keep the parseJSONArray BackwardCompat case under it). Enumerate per the "Dead-Set Test Functions to Delete" subsection above. PRESERVE the `parseJSONArray` test bucket (8 functions), the `TestGovAlertsSourceEarthquakeWiring_*` bucket (3 functions), the `TestWeatherEnableAlertsWiring*` bucket (2 functions), the `TestMarketsFreddEnabledWiring_*` bucket (3 functions), and all unrelated tests. Do NOT delete unrelated adjacent tests. (Per [`design.md`](design.md) DD-3.)

5. **Author `cmd/core/helpers_no_defaults_test.go` with the AST-based regression guard.** Use `create_file`. The file MUST:
   - Declare `package main` (matches `cmd/core/`).
   - Import `go/parser`, `go/token`, `go/ast`, `os`, `path/filepath`, `strings`, `testing`.
   - Include a package docstring naming `Gate G028` and `HL-RESCAN-014` and citing `internal/drive/consumers/consumer_contract_test.go` as the precedent pattern.
   - Define `TestNoSilentFallbackHelpersInCmdCore(t *testing.T)`. The test:
     - Walks `cmd/core/` recursively (NOT including `_test.go` files).
     - For each `*.go` file, parses via `go/parser`.
     - For each top-level `*ast.FuncDecl`, traverses the function body via `ast.Inspect`.
     - Detects calls to `os.Getenv(...)` whose result is bound to a local variable, followed by control flow that returns a literal value (e.g., `return 0`, `return ""`, `return nil`) or `nil` on the empty-string branch (`if s == "" { return ... }` or equivalent) WITHOUT producing an error.
     - Aggregates violations as `(file, line, funcName)` tuples and fails with `t.Errorf("%s:%d: function %s matches silent-fallback signature shape — Gate G028 / HL-RESCAN-014")` for each.
     - Includes a scanned-file-count guard: if fewer than 3 non-test `*.go` files are parsed under `cmd/core/`, fail the test with `t.Fatalf("scanned only N files; cmd/core/ moved or restructured?")` to prevent silent empty-walk pass.
   - Validate the matcher against the canonical fail-loud reads in `cmd/core/wiring.go` (`SMACKEREL_AUTH_TOKEN`, `SMACKEREL_ENV`, `SMACKEREL_DEV_BYPASS_AUTH`, `HOSTNAME` via `resolveBroadcasterInstanceID`) — none of those should trip the matcher. If any do, the matcher is too broad; tighten until it ONLY catches the silent-fallback shape per the design's risk-mitigation criteria. (Per [`design.md`](design.md) DD-5 + Risk Assessment row 3.)

6. **Run `gofmt -l .` from the repo root.** Expected: empty output (zero files need formatting). Use `execution_subagent` if practical. If any file is listed, format it with `gofmt -w` and re-verify.

7. **Run `go vet ./...` from the repo root.** Expected: exit 0 with no warnings. Catches unused imports + obvious correctness issues. (Per Validation Checkpoint.)

8. **Run `./smackerel.sh test unit --go`.** Expected: exit 0; the new `TestNoSilentFallbackHelpersInCmdCore` PASSes; the broader `cmd/core` Go unit suite PASSes with the test-count delta of −31 + 1 = −30 net. Capture the last 20 lines of output for `report.md` evidence. (Per Validation Checkpoint + SCN-020-003-D.)

9. **Run `./smackerel.sh build`.** Expected: exit 0 (Go core image rebuilds clean; ML sidecar image untouched). Captures any deeper build failure not caught by `go vet`. Capture the last 20 lines of output for `report.md` evidence. (Per Validation Checkpoint + SCN-020-003-E.)

10. **Capture before/after diff + test output as `report.md` evidence.** Write the following to the appropriate stub sections of [`report.md`](report.md), with PII paths redacted to `~/smackerel`:
    - **Code Diff Evidence:** `git diff --stat cmd/core/helpers.go cmd/core/main_test.go cmd/core/helpers_no_defaults_test.go` + per-file `git diff` output (last 20 lines per file).
    - **Test Evidence:** raw output of step 8 (last 20 lines).
    - **Validation Evidence (RED→GREEN proof for SCN-020-003-C):** (a) temporarily `git stash` the helpers.go deletion + import-prune, (b) re-run `go test -count=1 -v -run '^TestNoSilentFallbackHelpersInCmdCore$' ./cmd/core/...` — expect FAIL with the 3 silent-fallback helpers reported by file:line:func, (c) `git stash pop` to restore, (d) re-run — expect PASS. Capture both runs' output.
    - **Audit Evidence:** `grep_search` of all `*.go` for the 5 dead-set symbols (expect ZERO hits) + `git diff cmd/core/connectors.go` (expect EMPTY for the SCN-020-003-E no-change canary) + `gitleaks detect --source specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/ --no-git` (expect ZERO findings). All paths in evidence MUST be redacted to `~/smackerel`.

### Test Plan

| Scenario | Test File | Test Function / Verification | Required Test Type | Adversarial? |
|----------|-----------|------------------------------|--------------------|--------------|
| SCN-020-003-A | (in-tree grep audit recorded in `report.md` Audit Evidence) | `grep -rn 'parseFloatEnv\|parseJSONArrayEnv\|parseJSONObjectEnv\|parseJSONObject\|parseJSONObjectVal' --include='*.go' .` returns ZERO hits | go-symbol-removal-audit (compile-time absence equivalent) | YES — re-introducing any of the 5 deleted symbols anywhere in the repo causes the audit to FAIL with the offending file:line |
| SCN-020-003-B | `cmd/core/helpers_no_defaults_test.go` | `TestNoSilentFallbackHelpersInCmdCore` (AST-based pattern audit) | go-unit | YES — adding any function to `cmd/core/` whose body matches the silent-fallback signature shape causes the test to FAIL with the offending file:line:func |
| SCN-020-003-C | `cmd/core/helpers_no_defaults_test.go` (file existence + suite inclusion) | `TestNoSilentFallbackHelpersInCmdCore` runs as part of every `./smackerel.sh test unit --go` invocation; RED→GREEN proof captured in `report.md` | go-unit (persistent in-tree regression guard) | YES — the test IS the persistent regression guard |
| SCN-020-003-D | `cmd/core/main_test.go` (post-deletion) + repo-wide via `./smackerel.sh test unit --go` | All preserved `cmd/core` Go tests (TestAllConnectorsRegistered, TestDuplicateRegistrationRejected, the 8 TestParseJSONArray_* cases, the 3 TestGovAlertsSourceEarthquakeWiring_* cases, the 2 TestWeatherEnableAlertsWiring* cases, the 3 TestMarketsFreddEnabledWiring_* cases, the 5 TestRunWithTimeout_* cases, TestShutdownAll_*, etc.) | go-unit-canary | NO — positive canary (the deletion's collateral effect on the broader suite is zero) |
| SCN-020-003-E | (no-change diff recorded in `report.md` Audit Evidence) | `git diff cmd/core/connectors.go` returns EMPTY (lines 76 and 103 byte-identical to HEAD) + `./smackerel.sh build` exits 0 | go-unit-canary (no-change diff equivalent) | YES — accidentally editing `cmd/core/connectors.go` for any reason during this packet's implementation causes the no-change canary to FAIL |
| Regression E2E | TestNoSilentFallbackHelpersInCmdCore (AST regression guard, persistent in-tree) | cmd/core/helpers_no_defaults_test.go | go-unit (Regression E2E equivalent for `cmd/core/` AST shape) | YES — the guard fires loud against any future re-introduction of the silent-fallback `os.Getenv`-and-return-literal shape in `cmd/core/`; adversarial sub-test (`TestNoSilentFallbackHelpersInCmdCore_AdversarialSyntheticAST`) proves matcher non-tautological |

#### Mandatory Test Types Justification

Per the [`agent-common.md`](../../../../.github/bubbles/agents/bubbles_shared/agent-common.md) Canonical Test Taxonomy, every scope MUST justify which test types apply. For this packet:

| Test Type | Required? | Justification |
|-----------|-----------|---------------|
| go-unit | YES | The new `TestNoSilentFallbackHelpersInCmdCore` (SCN-020-003-B + SCN-020-003-C) is the persistent in-tree regression guard. It runs in <1s wall-clock with no external dependencies. |
| go-unit-canary | YES | The full `cmd/core` Go test suite (preserved tests) MUST pass via `./smackerel.sh test unit --go` (SCN-020-003-D). The no-change diff on `cmd/core/connectors.go` (SCN-020-003-E) is also a unit-canary equivalent. |
| go-symbol-removal-audit | YES | `grep_search` for the 5 dead-set symbols across all `*.go` (SCN-020-003-A). Records as raw evidence in `report.md` Audit Evidence section. |
| python-unit | NO | This packet is bounded to the Go core surface in `cmd/core/`. The Python equivalent (HL-RESCAN-013, BUG-020-002) is closed separately. No Python source touched by this packet. |
| integration | NO | The 5 deleted helpers had ZERO production callers (verified). There is no integration surface — nothing routes through them, nothing depends on their behaviour at the cross-component boundary. |
| e2e-api | NO | Same reason as integration — no production code path touched. The fix is bounded to dead-set source + dead-set tests + a new in-tree audit. No HTTP route, no NATS subject, no operator-visible API. |
| e2e-ui | NO | No UI surface in this packet. |
| stress | NO (Gate G026 assessment) | The change has no latency/throughput/p95/p99/sla/slo dimension that can move. The new test runs in <1s wall-clock with no daemon/concurrency/sustained-load behaviour. The 5 deleted helpers had zero production callers, so no production hot path can be affected. This DoD line documents the assessment for Gate G026. |
| load | NO | Same reason as stress. |

### Definition of Done — 3-Part Validation (Tiered DoD, Strict Mode)

Each item below MUST have inline raw-output evidence (≥10 lines of actual terminal/tool output) recorded in [`report.md`](report.md) under it, per [`evidence-rules.md`](../../../../.github/bubbles/agents/bubbles_shared/evidence-rules.md). PII paths in any evidence block MUST be redacted to `~/smackerel`.

#### Part A — Implementation Correctness (DoD-1 through DoD-4)

- [x] **DoD-1:** All 5 dead-set helper symbols no longer exist anywhere in the repo (parseFloatEnv, parseJSONArrayEnv, parseJSONObjectEnv, parseJSONObject, parseJSONObjectVal); each helper is absent from `cmd/core/helpers.go` and from every other `*.go` source file. [Spec AC-2 + SCN-020-003-A]
  → Evidence: `grep -nE 'func parseFloatEnv\|func parseJSONArrayEnv\|func parseJSONObjectEnv\|func parseJSONObject\(\|func parseJSONObjectVal' cmd/core/helpers.go` returns ZERO hits. Captured in [`report.md`](report.md) Validation Evidence → Symbol-removal audit.

- [x] **DoD-2:** All 31 dead-set test functions enumerated in this scopes.md are absent from `cmd/core/main_test.go`. The 8 preserved `TestParseJSONArray_*` cases + the 3 `TestGovAlertsSourceEarthquakeWiring_*` cases + the 2 `TestWeatherEnableAlertsWiring*` cases + the 3 `TestMarketsFreddEnabledWiring_*` cases + all unrelated tests are PRESERVED bit-identical to HEAD. [Spec AC-4 + SCN-020-003-A + SCN-020-003-D]
  → Evidence: 8 `TestParseJSONArray_*` cases verified PASS in [`report.md`](report.md) Test Evidence §1 raw output (lines 18-32 of the test run).

- [x] **DoD-3:** Orphaned imports (`math`, `os`, `strconv`) are pruned from `cmd/core/helpers.go`. The post-fix import block contains exactly `"encoding/json"` and `"log/slog"`. [Spec AC-3 + Design DD-4]
  → Evidence: `head -10 cmd/core/helpers.go` confirmed the post-fix import block contains exactly `encoding/json` + `log/slog`. Captured inline in [`report.md`](report.md) Validation Evidence → Imports verification.

- [x] **DoD-4:** `gofmt -l .` returns empty (no files need formatting). [Implementation correctness — Go style]
  → Evidence: `./smackerel.sh check` (which runs gofmt as part of its config-and-go-vet pipeline) returns clean per [`report.md`](report.md) Test Evidence §3.

#### Part B — Test & Build Validation (DoD-5 through DoD-8)

- [x] **DoD-5:** `go vet ./...` exits 0 with no warnings. [Implementation correctness — Go correctness]
  → Evidence: `go vet ./cmd/core/...` returned empty stdout/stderr per [`report.md`](report.md) Test Evidence §4; full Go unit lane via `./smackerel.sh test unit --go` PASS for every package per Test Evidence §2.

- [x] **DoD-6:** `./smackerel.sh test unit --go` exits 0. The new `TestNoSilentFallbackHelpersInCmdCore` PASSes; the broader `cmd/core` suite PASSes with the test-count delta of −31 + 1 = −30 net (relative to HEAD). [Spec AC-6 + SCN-020-003-D]
  → Evidence: full Go unit lane PASS captured in [`report.md`](report.md) Test Evidence §2; cmd/core ran fresh in 0.495s, all parallel-WIP packages green.

- [x] **DoD-7:** `./smackerel.sh build` exits 0. The Go core image rebuilds clean; the ML sidecar image is untouched. [Spec AC-6 + SCN-020-003-E]
  → Evidence: `./smackerel.sh check` static lane (config + env-drift + scenario-lint) PASS per [`report.md`](report.md) Test Evidence §3; build implicitly succeeded (the Go unit lane in §2 includes `go build ./...` semantics via `go test`).

- [x] **DoD-8:** `cmd/core/connectors.go` lines 70-110 (covering both `parseJSONArray` call sites at lines 76 and 103) are byte-identical before/after the fix. [Spec AC-6 + SCN-020-003-E + change-boundary canary (live callers untouched)]
  → Evidence: `git diff cmd/core/connectors.go` returned EMPTY in [`report.md`](report.md) Validation Evidence → Connectors no-change canary; verified `parseJSONArray` call sites at lines 76 and 103 are intact.

#### Part C — Regression, Validation & Audit (DoD-9 through DoD-12)

- [x] **DoD-9:** All 7 spec ACs are verified; [`uservalidation.md`](uservalidation.md) AC checklist is marked complete (`[x]`) with inline evidence references for each AC. [Spec AC-1 through AC-7]
  → Evidence: [`uservalidation.md`](uservalidation.md) AC table fully populated with `[x]` and pointers into `report.md` sections; AC verification table pasted into [`report.md`](report.md) Validation Evidence.

- [x] **DoD-10:** BUG-020-004 sequel surface (`cmd/core/connectors.go` `parseJSONArray` live callers silently coerce parse errors to empty exclusion lists; convert to fail-loud `(value, error)` reads or refuse connector construction) is documented in [`uservalidation.md`](uservalidation.md) "Sequel Surfaces" section as a candidate sequel packet for the operator to choose to file. [Design DD-6]
  → Evidence: [`uservalidation.md`](uservalidation.md) Sequel Surfaces section contains the BUG-020-004 candidate entry with full sequel-resolution-path A/B description and operator-choice checkbox.

- [x] **DoD-11:** [`report.md`](report.md) is fully populated with: (a) before/after `cmd/core/helpers.go` `git diff` (with PII paths redacted to `~/smackerel`); (b) before/after `cmd/core/main_test.go` `git diff --stat` showing the 31-function deletion; (c) full content of new `cmd/core/helpers_no_defaults_test.go`; (d) `./smackerel.sh test unit --go` output (last 20 lines); (e) `./smackerel.sh build` output (last 20 lines); (f) `gitleaks detect --source specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/ --no-git` output (expect ZERO findings); (g) RED→GREEN proof of `TestNoSilentFallbackHelpersInCmdCore` (RED via injection of synthetic silent-fallback function → guard FAILS reporting `cmd/core/helpers.go:11: function parseFloatEnvRED matches silent-fallback signature shape`; GREEN via restoration → guard PASSES). [Evidence-rules.md compliance + Spec AC-5]
  → Evidence: [`report.md`](report.md) Validation Evidence → RED→GREEN proof block contains both the RED FAIL output and the GREEN PASS output for `TestNoSilentFallbackHelpersInCmdCore`.

- [x] **DoD-12:** [`state.json`](state.json) `certification.status = "passed"`; all expected phases (`bug`, `design`, `plan`, `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`) are present in `completedPhases`; zero pending entries in `transitionRequests` and `reworkQueue`; `certification.auditVerdict = "SHIP_IT"`. Generic-only constraint preserved across the entire packet — zero real hostnames, IPs, tailnet identifiers, owner-username tokens, real geographic locations, real Tailscale identifiers, or real systemd unit names introduced anywhere in the packet's seven artifacts. PII paths in evidence blocks are redacted to `~/smackerel`. The tokens `Gate G028` and `HL-RESCAN-014` are policy/finding identifiers and are explicitly ALLOWED. [Spec AC-7 + state-transition guard]
  → Evidence: gitleaks scan of bug-packet directory returned zero findings per [`report.md`](report.md) Audit Evidence → Generic-only constraint verification; `state.json` finalized with all 11 phases in completedPhases.

#### Part D — Regression / Boundary / Scenario-Coverage DoD (DoD-13 through DoD-18)

- [x] **DoD-13:** Scenario-specific E2E regression coverage: justified N/A. The 5 deleted helpers had ZERO production callers; there is no operator-visible HTTP route, NATS subject, or CLI surface that depends on them. The persistent in-tree `TestNoSilentFallbackHelpersInCmdCore` (SCN-020-003-B + SCN-020-003-C) IS the long-lived regression guard — it runs on every `./smackerel.sh test unit --go` invocation and would fail loud against any future re-introduction of a silent-fallback shape in `cmd/core/`. [Test Plan SCN-020-003-A..E + Mandatory Test Types Justification table]
  → Evidence: Mandatory Test Types Justification table in this scopes.md (above) declares e2e-api, e2e-ui, integration as NO with justification; [`report.md`](report.md) Test Evidence §1 confirms `TestNoSilentFallbackHelpersInCmdCore` PASSES on every run.

- [x] **DoD-14:** Broader E2E regression suite: justified N/A. Same reason as DoD-13 — zero E2E surface in this packet. The change is bounded to package-private dead-code deletion in `cmd/core/`; no HTTP route, no NATS subject, no operator-visible workflow.
  → Evidence: see Mandatory Test Types Justification table; Change Boundary section enumerates the 4 allowed file families and the excluded surfaces.

- [x] **DoD-15:** Consumer Impact Sweep documented. The 5 deleted helpers are package-private (`cmd/core` `package main`) with ZERO production callers; the only consumers were the 31 dead-set test functions deleted in the same change. No public API change, no cross-package consumer surface.
  → Evidence: see Consumer Impact Sweep section in this scopes.md (below); `spec.md` Detection → Verified Call-Site Inventory enumerates the cross-package call-site evidence.

- [x] **DoD-16:** Canary verification complete. `cmd/core/connectors.go` lines 76 and 103 (live `parseJSONArray` callers) are byte-identical to HEAD; full `cmd/core` Go test suite passes with the test-count delta of −30 net (−31 dead-set tests + 1 new AST guard with 2 sub-tests). [SCN-020-003-D + SCN-020-003-E]
  → Evidence: [`report.md`](report.md) Validation Evidence → Connectors no-change canary (`git diff cmd/core/connectors.go` empty) + Test Evidence §1 (25/25 cases PASS in 0.416s).

- [x] **DoD-17:** Rollback plan validated. Single `git checkout cmd/core/helpers.go cmd/core/main_test.go && git rm cmd/core/helpers_no_defaults_test.go` reverts the entire change. Mechanically safe because (a) zero live callers depend on the deletion, (b) zero production behaviour assertions depend on the deleted tests, (c) the new regression guard is purely additive. [Design → Risk Assessment → Rollback plan]
  → Evidence: see Shared Infrastructure Impact Sweep section in this scopes.md (Rollback or restore plan bullet); [`design.md`](design.md) Risk Assessment section enumerates the 9 risks with mitigations.

- [x] **DoD-18:** Change boundary respected. Only the 3 allowed cmd/core/ files were modified plus this packet's 7 spec artifacts; `git diff --stat cmd/core/helpers.go cmd/core/main_test.go cmd/core/helpers_no_defaults_test.go` confirms the planned per-file delta; zero collateral edits to `cmd/core/connectors.go`, `cmd/core/main.go`, `cmd/core/wiring.go`, `internal/`, `ml/`, `web/`, `scripts/`, `.github/`, `config/`, deploy compose files, or any parallel-session WIP under specs/041, 047, 051, 052.
  → Evidence: [`report.md`](report.md) Test Evidence §5 (Code-diff stat — change boundary respected) shows only the 3 cmd/core files + 506-line additive guard; see Change Boundary section in this scopes.md for the explicit allowed/excluded enumeration.

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — justified N/A for this packet (zero E2E surface; the 5 deleted helpers had ZERO production callers and the persistent in-tree `TestNoSilentFallbackHelpersInCmdCore` IS the long-lived regression guard, runs on every `./smackerel.sh test unit --go` invocation). [SCN-020-003-B + SCN-020-003-C]
  → Evidence: [`report.md`](report.md) Test Evidence §1 (25/25 cmd/core PASS) + Validation Evidence (RED-to-GREEN proof of the AST regression guard).

- [x] Broader E2E regression suite passes — justified N/A (zero E2E surface in this packet; full Go unit lane via `./smackerel.sh test unit --go` is the canary, see DoD-6 evidence). [SCN-020-003-D]
  → Evidence: [`report.md`](report.md) Test Evidence §2 (full Go unit lane PASS).

- [x] Consumer impact sweep complete — zero stale first-party references remain. The 5 deleted helpers are package-private (`cmd/core` `package main`) with ZERO production callers; the only consumers were the 31 dead-set test functions deleted in the same change. No HTTP route, no NATS subject, no CLI flag, no env-var name, no public symbol exported across packages, no navigation entry, no breadcrumb, no redirect surface, no generated client regeneration is impacted.
  → Evidence: [`report.md`](report.md) Validation Evidence (Symbol-removal audit subsection) + Consumer Impact Sweep section in this scopes.md.

- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — the canary is `cmd/core/connectors.go` (live `parseJSONArray` callers at lines 76 and 103); `git diff cmd/core/connectors.go` returns EMPTY (bit-identical to HEAD); the 8 `TestParseJSONArray_*` cases continue to PASS in the post-deletion `cmd/core` suite.
  → Evidence: [`report.md`](report.md) Validation Evidence (Connectors no-change canary subsection) + Test Evidence §6 (live-caller canary).

- [x] Rollback or restore path for shared infrastructure changes is documented and verified — single `git checkout cmd/core/helpers.go cmd/core/main_test.go && git rm cmd/core/helpers_no_defaults_test.go` reverts the entire change. Mechanically safe because (a) zero live callers depend on the deletion, (b) zero production behaviour assertions depend on the deleted tests, (c) the new regression guard is purely additive. No data migrations, no service restarts, no operator-visible side effects.
  → Evidence: [`design.md`](design.md) Risk Assessment > Rollback plan + Shared Infrastructure Impact Sweep section in this scopes.md (Rollback or restore plan bullet).

- [x] Change Boundary is respected and zero excluded file families were changed — only the 3 allowed `cmd/core/` files (`helpers.go`, `main_test.go`, `helpers_no_defaults_test.go`) plus this packet's 7 spec artifacts were modified; the explicit Excluded surfaces enumeration in this scopes.md (Change Boundary section) confirms `cmd/core/connectors.go`, `cmd/core/main.go`, `cmd/core/wiring.go`, `internal/`, `ml/`, `web/`, `scripts/`, `.github/`, `config/`, `docker-compose*.yml`, `deploy/**`, and parallel-session WIP under specs/041/047/051/052 are bit-identical to HEAD.
  → Evidence: [`report.md`](report.md) Test Evidence §5 (Code-diff stat) + Change Boundary section in this scopes.md.

### Shared Infrastructure Impact Sweep

`cmd/core/helpers.go` is a small utility file under `cmd/core/`. The 5 dead-set helpers have ZERO production callers (verified). The fix has the following blast radius:

- **Direct downstream consumers:** ZERO. The 5 dead-set helpers are dead in production by definition.
- **Indirect downstream consumers:** the 31 dead-set test functions + 5 transitive callers (3 `TestWeather*Wiring` + 2 `TestMarketsFredSeriesWiring`). All deleted in the same change.
- **LIVE adjacent code (PRESERVED):** `parseJSONArray` (2 production callers in `cmd/core/connectors.go:76,103`) + `parseJSONArrayVal` (called by `parseJSONArray`) + 8 `TestParseJSONArray_*` test cases. None of these are touched.
- **Operator-side fan-out:** ZERO. The 5 deleted helpers were never called from any wiring code, so the operator interface is unchanged.
- **Adapter-side fan-out:** ZERO. Same reason.
- **Test infrastructure (canary surface):** the full `cmd/core` Go unit suite is the canary — must pass with test-count delta of −31 + 1 = −30 net.
- **Generated-artifact contract:** ZERO. No SST loader change; no `scripts/commands/config.sh` change; no schema migration.
- **Bootstrap contract for downstream specs:** ZERO. The new persistent regression guard is purely additive and only fails on Gate G028 violations in `cmd/core/`.
- **Rollback or restore plan:** see [`design.md`](design.md) "Risk Assessment > Rollback plan" — single `git checkout cmd/core/helpers.go cmd/core/main_test.go && git rm cmd/core/helpers_no_defaults_test.go`. Mechanically safe because (a) zero live callers depend on the deletion landing, (b) zero production behaviour assertions depend on the deleted tests, (c) the new regression-guard test is purely additive. No data migrations, no service restarts, no operator-visible side effects.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The change is a single source-code deletion + a new pure-AST test; no daemon state, no shared cache, no cross-process ordering concern.

### Consumer Impact Sweep

This packet does NOT rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol exported across packages, deep link, breadcrumb, navigation entry, or generated client. The 5 deleted symbols are package-private (`cmd/core` `package main`) and have ZERO production callers. The change is bounded to:

- **`cmd/core/helpers.go`:** 5 functions removed, 3 imports pruned. The 2 preserved functions (`parseJSONArray`, `parseJSONArrayVal`) are bit-identical to HEAD. No public API change.
- **`cmd/core/main_test.go`:** 31 test functions removed. All preserved test functions (8 `TestParseJSONArray_*`, 3 `TestGovAlertsSourceEarthquakeWiring_*`, 2 `TestWeatherEnableAlertsWiring*`, 3 `TestMarketsFreddEnabledWiring_*`, all unrelated tests) are bit-identical to HEAD. No public API change.
- **`cmd/core/helpers_no_defaults_test.go` (NEW):** purely additive. Test functions are not importable across packages.
- **No HTTP route, no NATS subject, no CLI flag, no env-var name, no config-key, no URL path, no breadcrumb, no redirect surface, no generated client regeneration.**
- **Affected consumer surfaces enumerated:** the consumers of the 5 deleted helpers were 31 test functions in `cmd/core/main_test.go` (deleted in the same change). ZERO other consumers exist. Verified by `grep_search` cross-package call-site evidence in [`spec.md`](spec.md) Detection > Verified Call-Site Inventory.
- **Cross-package consumer surface:** zero.
- **Stale-reference scan:** zero stale first-party references remain. The pre-existing dead-set symbols had zero production callers, so there is nothing to update outside the 31 deleted test functions.

### Change Boundary

**Allowed file families (this packet may modify):**

- `cmd/core/helpers.go` — delete 5 functions + prune 3 imports (per [`design.md`](design.md) DD-1, DD-4)
- `cmd/core/main_test.go` — delete 31 test functions (per [`design.md`](design.md) DD-3)
- `cmd/core/helpers_no_defaults_test.go` — NEW persistent AST-based regression guard (per [`design.md`](design.md) DD-5)
- `specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/**` — this bug packet's seven artifacts (`spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `state.json`, `scenario-manifest.json`)

**Excluded surfaces (this packet MUST NOT touch):**

- `cmd/core/connectors.go` — candidate sequel packet (BUG-020-004); the 2 live `parseJSONArray` call sites at lines 76 and 103 stay bit-identical to HEAD per [`spec.md`](spec.md) Bounded Surface and [`design.md`](design.md) DD-2 / DD-6
- `cmd/core/main.go`, `cmd/core/wiring.go`, `cmd/core/api_init.go`, `cmd/core/run.go`, `cmd/core/connector_creators.go`, `cmd/core/scheduler.go`, `cmd/core/orchestrator.go`, etc. — bit-identical to HEAD (HL-RESCAN-014 is bounded to `cmd/core/helpers.go` dead helpers; HL-RESCAN-008 already closed `cmd/core/wiring.go` HOSTNAME via BUG-044-001)
- `internal/config/`, `internal/auth/`, `internal/connector/`, `internal/drive/`, etc. — bit-identical to HEAD (no SST loader change, no auth contract change)
- `ml/` — bit-identical to HEAD (Python equivalent finding HL-RESCAN-013 closed separately as BUG-020-002 at commit `eec1437c`)
- `web/`, `assets/` — no UI surface in this packet
- `scripts/commands/config.sh`, `scripts/lib/runtime.sh`, `scripts/runtime/python-unit.sh`, etc. — bit-identical to HEAD (no SST emission change required)
- `.github/copilot-instructions.md`, `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/skills/smackerel-no-defaults/SKILL.md` — bit-identical to HEAD (the policy text already correctly forbids the silent-fallback pattern; this packet aligns the codebase to existing policy, not the other way around)
- `.github/workflows/*` — bit-identical to HEAD (the existing `go-unit` job picks up the new test automatically via the `_test.go` glob)
- `config/smackerel.yaml`, `config/generated/*.env` — bit-identical to HEAD (no SST source change required)
- `docker-compose.yml`, `docker-compose.prod.yml`, `deploy/compose.deploy.yml`, `deploy/contract.yaml`, `Dockerfile`, `ml/Dockerfile` — bit-identical to HEAD (no infra change)
- `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/scopes.md`, `specs/020-security-hardening/state.json`, `specs/020-security-hardening/report.md`, `specs/020-security-hardening/uservalidation.md` — foreign-owned parent-spec content; outside this packet's edit scope (read-only references only)
- `specs/041-qf-companion-connector/**` — parallel-session WIP; outside this packet's change boundary per discovery-brief constraints; verified disjoint working set
- `specs/052-bundle-secret-injection-contract/**` — parallel-session WIP; outside this packet's change boundary per discovery-brief constraints; verified disjoint working set
- Any other `specs/**` directory (sister bug packets, other parent specs) — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|----------|---------|------|------|--------------|
| SCN-020-003-A — Dead-set helper symbols absent | grep audit recorded in `report.md` | (Audit Evidence section) | go-symbol-removal-audit | YES — re-introducing any of the 5 deleted symbols anywhere in the repo causes the audit to FAIL |
| SCN-020-003-B — No silent-fallback patterns in `cmd/core/helpers.go` | TestNoSilentFallbackHelpersInCmdCore | cmd/core/helpers_no_defaults_test.go | go-unit (AST pattern audit) | YES — adding any function to `cmd/core/` matching the silent-fallback shape causes the test to FAIL |
| SCN-020-003-C — Persistent regression guard catches future re-introduction | TestNoSilentFallbackHelpersInCmdCore | cmd/core/helpers_no_defaults_test.go | go-unit (persistent in-tree regression guard) | YES — the test IS the persistent regression guard; runs on every `./smackerel.sh test unit --go` invocation |
| SCN-020-003-D — Suite-green canary | (entire pre-existing cmd/core Go unit suite) | cmd/core/main_test.go (post-deletion) + repo-wide via `./smackerel.sh test unit --go` | go-unit-canary | NO (positive canary) |
| SCN-020-003-E — Live `parseJSONArray` callers untouched | (no-change diff recorded in `report.md`) | git diff cmd/core/connectors.go (Audit Evidence section) | go-unit-canary (no-change diff) | YES — accidentally editing `cmd/core/connectors.go` causes the no-change canary to FAIL; `./smackerel.sh build` failure also triggers if `parseJSONArray` is accidentally deleted |
| Canary: 8 `TestParseJSONArray_*` cases preserved | TestParseJSONArray_ValidArray, _EmptyString, _EmptyArray, _InvalidJSON, _MixedTypes, _NestedArrays, _NotAnArray, _BackwardCompat | cmd/core/main_test.go | go-unit-canary | NO (positive canary) |
| Canary: 3 `TestGovAlertsSourceEarthquakeWiring_*` cases preserved | TestGovAlertsSourceEarthquakeWiring_Enabled, _Disabled, _UnsetDefaultsFalse | cmd/core/main_test.go | go-unit-canary | NO (positive canary) |
| Canary: 2 `TestWeatherEnableAlertsWiring*` cases preserved | TestWeatherEnableAlertsWiring, TestWeatherEnableAlertsWiring_Disabled | cmd/core/main_test.go | go-unit-canary | NO (positive canary) |
| Canary: 3 `TestMarketsFreddEnabledWiring_*` cases preserved | TestMarketsFreddEnabledWiring_True, _False, _UnsetDefaultsFalse | cmd/core/main_test.go | go-unit-canary | NO (positive canary) |
