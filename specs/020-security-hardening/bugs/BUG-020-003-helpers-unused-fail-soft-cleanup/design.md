# Design: BUG-020-003 — `cmd/core/helpers.go` env-reading helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) silently fall back to `0` / `nil` on missing or malformed env vars, codifying the FORBIDDEN fail-soft pattern under Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Overview & Approach

This packet closes HL-RESCAN-014 by **deleting the 5 dead-set helpers** (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal`) from [`cmd/core/helpers.go`](../../../../cmd/core/helpers.go), **deleting the 31 dead-set test functions** they pin in [`cmd/core/main_test.go`](../../../../cmd/core/main_test.go), **pruning three orphaned imports** (`math`, `os`, `strconv`) from `cmd/core/helpers.go`, and **adding a persistent in-tree adversarial regression guard** (`cmd/core/helpers_no_defaults_test.go`) that AST-walks `cmd/core/helpers.go` on every `./smackerel.sh test unit --go` invocation and fails loud if any function in the file matches the silent-fallback signature shape (body contains `os.Getenv(KEY)` followed by a return-on-empty path that does not propagate an error).

Resolution path chosen: **Option A — pure deletion of the 5 truly dead helpers + their tests**, following the precedent BUG-044-001 set for the inverse case (the `HOSTNAME` reader had a live production caller in the revocation broadcaster, so it was refactored to a fail-loud `(value, error)` helper rather than deleted). The mirror-image rule applies here: the 5 helpers in question have **zero live production callers** (verified call-site evidence in [`spec.md`](spec.md) Detection > Verified Call-Site Inventory), so the precedent prescribes deletion, not refactoring. See DD-1 below for the full rejection of Options B and C.

The two LIVE production callers of `parseJSONArray` at [`cmd/core/connectors.go`](../../../../cmd/core/connectors.go) lines 76 and 103 (`parseJSONArray(cfg.BookmarksExcludeDomains)` / `parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`) are explicitly **out of scope** for this packet. They represent a separate Gate G028 concern (silent `nil`-on-parse-error in production wiring) that the self-hosted readiness rescan did NOT enumerate as a finding (HL-RESCAN-014 is bounded to the unused-helper claim). The connectors.go fail-loud refactor is documented as a sequel surface in DD-6 below and forwarded into [`uservalidation.md`](uservalidation.md) for the operator to file as BUG-020-004 if/when they choose to take it on.

The fix is mechanically:

1. **`cmd/core/helpers.go`** — delete `parseFloatEnv` (lines 63–82, ~20 lines), `parseJSONArrayEnv` (lines 17–22, ~6 lines), `parseJSONObjectEnv` (lines 43–48, ~6 lines), `parseJSONObject` (lines 37–41, ~5 lines), `parseJSONObjectVal` (lines 50–61, ~12 lines). Prune the now-orphaned imports `math`, `os`, `strconv` from the import block (DD-4). The post-fix file contains exactly two functions — `parseJSONArray` (live wrapper, line 13) and `parseJSONArrayVal` (live worker, line 25) — plus the package declaration and the surviving imports `encoding/json` and `log/slog`.

2. **`cmd/core/main_test.go`** — delete the 31 dead-set test functions: 7 `TestParseJSONObject_*` cases (lines 164–222), 13 `TestParseFloatEnv_*` cases (lines 225–335 covering valid/integer/empty/unset/invalid/negative/zero/scientific-notation/Inf/-Inf/+Inf/NaN/nan-lowercase), 3 `TestParseJSONArrayEnv_*` cases (lines 403–427), 3 `TestParseJSONObjectEnv_*` cases (lines 429–451), 3 stale `TestWeather*Wiring` cases (lines 568–593 for `WEATHER_FORECAST_DAYS` / `WEATHER_PRECISION` / empty-on-zero), 2 stale `TestMarketsFredSeriesWiring` cases (lines 643–663), and 1 `TestParseJSONObject_BackwardCompat` case (lines 461–466). The 7 `TestParseJSONArray_*` cases (lines 109–161) and the `TestParseJSONArray_BackwardCompat` case (line 455–459) are PRESERVED because `parseJSONArray` is LIVE (DD-3).

3. **`cmd/core/helpers_no_defaults_test.go`** (NEW FILE) — adds `TestNoSilentFallbackHelpersInCmdCore` which uses `go/parser` + `go/ast` to walk every `*.go` file under `cmd/core/` (excluding `_test.go` files), inspects every function declaration, and fails loud if the function body contains an `os.Getenv(...)` call whose subsequent control flow returns a literal value or `nil` without propagating an error. The test's adversarial assertion is the SCN-020-003-C contract surface — re-introducing any of the 5 deleted helpers OR adding a new helper that matches the silent-fallback shape mechanically fails the test with the explicit file + line + function name (DD-5).

The post-fix repo state has zero `parseFloatEnv` / `parseJSONArrayEnv` / `parseJSONObjectEnv` / `parseJSONObject` / `parseJSONObjectVal` symbols anywhere (verified by SCN-020-003-A grep audit), zero silent-fallback patterns in `cmd/core/helpers.go` (verified by SCN-020-003-B grep audit + the new AST guard), a passing `./smackerel.sh test unit --go` (verified by SCN-020-003-D), and identical compilation + behaviour at the two live `parseJSONArray` call sites in `cmd/core/connectors.go` (verified by SCN-020-003-E).

## Root Cause Analysis

The 5 dead helpers exist as **leftover scaffolding from spec 019 connector-wiring iterations** (R23, R27, R28). The git history embedded in `cmd/core/main_test.go` confirms this — the H-019-002 / IMP-019-R27-001 / IMP-019-R28 comment headers above the test blocks (lines 403, 482, 596) name the spec-019 round numbers in which the helpers were introduced. At each round the wiring code reached for an `Env`-suffixed helper to read a numeric / JSON-array env value, then a subsequent SST refactor moved the env read into the canonical `internal/config` SST loader (which produces `cfg.WeatherForecastDays`, `cfg.WeatherPrecision`, `cfg.FinancialMarketsFredSeries` struct fields rather than direct env reads). The wiring sites were updated to use the struct fields; the helpers and their tests were NOT removed in the same change.

The result is three concrete violations of Gate G028 / NO-DEFAULTS:

1. **The helpers themselves codify the forbidden read shape.** `parseFloatEnv` returns `0` on three distinct silent-fallback branches (empty env, parse error, non-finite). `parseJSONArrayEnv` and `parseJSONObjectEnv` delegate to `parseJSONArrayVal` / `parseJSONObjectVal`, both of which return `nil` on empty + on parse error with only a `slog.Warn`. None of the three helpers propagate an error to the caller. None of them call `log.Fatal`. The signatures (`fn(key string) <T>` with no error return) advertise a fail-soft ergonomic that the canonical Smackerel SST contract explicitly forbids.

2. **The tests lock the silent-fallback semantics in place.** 16 + 5 + 3 + 7 = 31 test functions in `cmd/core/main_test.go` exercise the fail-soft branches and assert that `parseFloatEnv("UNSET") == 0`, `parseJSONArrayEnv("INVALID_JSON") == nil`, etc. As long as those tests exist, any future fail-loud conversion of the helpers would require coordinated test deletion or rewrite, raising the friction of the canonical fix.

3. **The dead-helper signatures are a copy-paste hazard for new connector wiring.** A maintainer adding a new connector that needs a numeric env value sees `parseFloatEnv` in `cmd/core/helpers.go`, reads its one-line ergonomics, and reaches for it before discovering that the canonical fail-loud pattern requires hand-rolled `os.Getenv` + `strconv.ParseFloat` + empty/error → `log.Fatal`. The moment they do, Gate G028 is silently violated in production.

The reason these helpers slipped past prior reviews is:

- The self-hosted readiness rescan series prior to 2026-05-14 was bounded to the `internal/`, `ml/`, and `deploy/` surfaces (HL-RESCAN-008 covered `cmd/core/wiring.go` HOSTNAME but did not sweep `cmd/core/helpers.go`).
- The `gocritic` linter does not flag this pattern because the helpers' signatures are syntactically valid Go.
- The Gate G028 grep checks in `.github/instructions/smackerel-no-defaults.instructions.md` target source code that USES the forbidden form (e.g., `${VAR:-default}` in compose files, `os.getenv("KEY", "default")` in Python). They do not catch a Go function that wraps `os.Getenv` + return-on-empty inside a helper signature that hides the forbidden form from a string-search pass.
- The dead-set helpers have zero production callers, so a static-call-graph analysis would correctly identify them as unreachable from `main()`. But Go's standard tooling (`go build`, `go vet`) does not warn on unused package-level functions (it only warns on unused imports and unused locals). The deadness is invisible without a deliberate cross-package `grep_search` pass — exactly the pass the self-hosted readiness rescan executed.

The mitigation against future re-introduction is the persistent AST-based regression guard (DD-5). It catches both the deletion-revert vector AND the copy-paste-into-new-helper vector, because it inspects the function-body shape rather than the symbol name.

## Design Decisions

### DD-1: Resolution path — Option A (pure deletion of 5 dead helpers + 31 dead tests)

**Decision:** Adopt **Option A** from [`spec.md`](spec.md): delete the 5 dead-set helpers and their 31 corresponding test functions. Do NOT refactor the dead helpers to fail-loud signatures. Do NOT touch the live `parseJSONArray` call sites in `cmd/core/connectors.go`.

**Rationale:** Three independent reasons converge on Option A.

First, **the BUG-044-001 sister-packet precedent prescribes deletion when the helper is unused and refactoring when the helper is used**. BUG-044-001 closed HL-RESCAN-008 by refactoring the inline silent-fallback `os.Getenv("HOSTNAME")` + literal-string fallback into a fail-loud `resolveBroadcasterInstanceID() (string, error)` helper because there was a live production caller (the revocation broadcaster). The mirror-image case applies here: the 5 helpers in question have **zero live production callers** (verified call-site evidence in [`spec.md`](spec.md) Detection > Verified Call-Site Inventory). Refactoring zero callers achieves nothing; deletion removes the foothold for the forbidden pattern entirely.

Second, **deletion is the strictly stronger fix**. Option C (convert all 3 `Env`-suffixed helpers to fail-loud `(value, error)` signatures) would preserve a helper surface that future callers might still misuse — a maintainer who needs a numeric env value would see `parseFloatEnv(key string) (float64, error)` and might reach for it instead of writing the canonical inline `os.Getenv` + `strconv.ParseFloat` + `log.Fatal` pattern. The canonical pattern is what every other Go fail-loud read in the codebase uses (`cmd/core/wiring.go:37,57,59` for `SMACKEREL_AUTH_TOKEN`, `SMACKEREL_ENV`, `SMACKEREL_DEV_BYPASS_AUTH`; `cmd/core/wiring.go` `resolveBroadcasterInstanceID()` for `HOSTNAME`). Deleting the dead helpers forces future code to follow the canonical inline pattern, which matches the spec 044 / spec 020 design philosophy: **per-call-site fail-loud reads, not a shared "easy" helper that obscures the SST contract**.

Third, **scope discipline**. HL-RESCAN-014 is specifically the unused-helper finding. The two live `parseJSONArray` call sites in `cmd/core/connectors.go` represent a separate concern (silent `nil` on parse error in production wiring) that the rescan did NOT enumerate. Bundling them into this packet would (a) violate the single-bug-scope rule, (b) require re-classifying the rescan finding mid-flight, and (c) entangle the deletion proof (mechanical, low-risk) with a wiring refactor that requires per-connector regression coverage (cross-cutting, higher-risk). The connectors.go concern gets a sequel packet (BUG-020-004 candidate) per DD-6.

**Alternatives rejected:**

- **Option B (deletion of 5 dead helpers + fail-loud refactor of `parseJSONArray` callers in `cmd/core/connectors.go`):** Rejected per the scope-discipline rationale above. The connectors.go refactor is a meaningful change to live production code and deserves its own packet with its own DoD, scenarios, and regression coverage. Folding it in here would either (a) require expanding the spec's AC list mid-design (out of scope for the design specialist), or (b) silently bundle the change without spec coverage (forbidden by single-bug-scope discipline).
- **Option C (convert all 3 `Env`-suffixed helpers to fail-loud `(value, error)` signatures):** Rejected because (a) the helpers have ZERO live callers, so the conversion achieves no behavioural change in production; (b) the converted helpers would still advertise a "shortcut" ergonomic that the canonical inline pattern explicitly avoids; (c) the converted helpers would still need an adversarial regression guard to prevent a future revert to the silent-fallback form, but the test would be testing a helper that nobody uses. Strictly worse than Option A on every axis (more code, more tests, more cognitive load, no production behaviour change).
- **Hybrid: delete `parseFloatEnv` only; convert `parseJSONArrayEnv` / `parseJSONObjectEnv` to fail-loud:** Rejected because the same scope-discipline argument applies — there is no live caller for any of the three. A future caller for any of the three would still be told "use the canonical inline pattern" per the spec 044 / spec 020 design philosophy.

### DD-2: Scope boundary — `cmd/core/helpers.go` deletion + `cmd/core/main_test.go` test deletion + new regression-guard file only

**Decision:** This packet modifies **exactly three files in `cmd/core/`**: (a) [`cmd/core/helpers.go`](../../../../cmd/core/helpers.go) (delete 5 functions + prune 3 orphaned imports), (b) [`cmd/core/main_test.go`](../../../../cmd/core/main_test.go) (delete 31 test functions), (c) `cmd/core/helpers_no_defaults_test.go` (NEW — persistent AST-based regression guard).

**Rationale:** Every file outside this list is explicitly out of scope per [`spec.md`](spec.md) Out of Scope, including:

- [`cmd/core/connectors.go`](../../../../cmd/core/connectors.go) lines 76 / 103 — the two live `parseJSONArray` callers. Touching these would expand the scope into a separate Gate G028 finding (silent `nil` on parse error in production wiring) that HL-RESCAN-014 did not enumerate. Sequel packet candidate (DD-6).
- [`internal/config/`](../../../../internal/config/) — the SST loader is correct; the bug is purely in the dead helper code.
- [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md), [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md), [`.github/skills/smackerel-no-defaults/SKILL.md`](../../../../.github/skills/smackerel-no-defaults/SKILL.md) — the policy text already correctly forbids the silent-fallback pattern; this packet aligns the codebase to existing policy.
- [`specs/020-security-hardening/spec.md`](../../spec.md), [`specs/020-security-hardening/design.md`](../../design.md), and the parent spec's other artifacts — foreign-owned, read-only.
- Other `cmd/core/*.go` files (`cmd/core/main.go`, `cmd/core/wiring.go`, `cmd/core/api_init.go`, `cmd/core/connectors.go`) — the rescan did not enumerate any other dead-helper findings in `cmd/core/`. HL-RESCAN-008 already closed the `HOSTNAME` site (BUG-044-001).
- The ML sidecar (`ml/`) — HL-RESCAN-013 (BUG-020-002) already closed the Python-side equivalent at commit `eec1437c`.
- Parallel-session WIP under [`specs/041-qf-companion-connector/`](../../../041-qf-companion-connector/) and [`specs/052-bundle-secret-injection-contract/`](../../../052-bundle-secret-injection-contract/) — neither directory contains files this packet modifies (verified by file_search). Disjoint working sets.

The change boundary is deliberately the smallest set of files that satisfies all 7 ACs. Discipline against scope creep is the single biggest determinant of close-out velocity for bugfix-fastlane packets.

**Alternatives rejected:**

- **Add a fourth file modification: an SST-loader audit script under `scripts/`.** Considered. Rejected because the existing `scripts/` directory does not own Gate G028 enforcement (the existing pattern is in-tree Go tests under `internal/`, e.g., `internal/deploy/compose_contract_test.go` for the compose-side pattern). Adding a script-side guard would be a separate concern.
- **Add the regression guard inside `cmd/core/main_test.go` as a new test method.** Rejected because the regression guard targets `cmd/core/helpers.go` specifically, and a co-located file (`cmd/core/helpers_no_defaults_test.go`) is the discoverability-correct location. A future maintainer reading `helpers.go` sees the sibling test file by simple `ls cmd/core/helpers*` and finds the guard immediately.

### DD-3: Test deletion strategy — pure removal of dead-set tests; no rewrite to fail-loud assertions

**Decision:** Delete the 31 dead-set test functions outright. Do NOT rewrite them to assert a fail-loud contract (because the helpers they test are being deleted). Preserve the 8 `TestParseJSONArray_*` test cases (lines 109–161 + line 455–459 backward-compat) because `parseJSONArray` itself is LIVE in production and its existing test coverage stays valid.

**Rationale:** The 31 test functions exist to lock the silent-fallback semantics of helpers that are being deleted. There is no fail-loud contract to assert because there is no helper to assert it about. Rewriting any of these tests to "assert that `parseFloatEnv` does not exist" would be tautological and would belong to the regression guard (SCN-020-003-A), not to a per-helper unit test.

The 3 stale `TestWeather*Wiring` cases (lines 568–593) and 2 stale `TestMarketsFredSeriesWiring` cases (lines 643–663) are particularly worth flagging: they READ env vars (`WEATHER_FORECAST_DAYS`, `WEATHER_PRECISION`, `FINANCIAL_MARKETS_FRED_SERIES`) via the dead helpers, but the actual production wiring at [`cmd/core/connectors.go`](../../../../cmd/core/connectors.go) lines 282 / 283 / 360 reads these values from struct fields (`cfg.WeatherForecastDays`, `cfg.WeatherPrecision`, `cfg.FinancialMarketsFredSeries`) populated by the SST loader. The tests are ALREADY stale — they test a code path the production wiring no longer takes. Deleting them removes a documented-stale test surface AND the orphaned helper dependency in one move.

The 8 preserved `TestParseJSONArray_*` cases test the LIVE `parseJSONArray` function with input strings — they cover empty / valid array / empty array / invalid JSON / mixed types / nested / not-an-array / 1,2,3 backward-compat. None of these test the env-read silent-fallback pattern because `parseJSONArray` does not read env vars (it parses an in-memory string). They stay.

**Alternatives rejected:**

- **Rewrite the 31 dead-set tests to assert "function does not exist".** Rejected as tautological; this is the regression guard's job.
- **Keep the 31 dead-set tests and just rewrite them to drive a fail-loud contract on a refactored helper.** Rejected because Option A (DD-1) deletes the helpers; there is nothing to drive a contract against.
- **Move the 31 dead-set tests into a `_disabled_test.go.txt` archive file.** Rejected as test-hygiene noise; deleted code does not need a museum exhibit. Git history preserves the test bodies for any future archaeology.
- **Preserve the 5 stale `TestWeather*Wiring` + 2 `TestMarketsFredSeriesWiring` cases by rewriting them to assert against `cfg.WeatherForecastDays` / `cfg.FinancialMarketsFredSeries` struct fields.** Considered. Rejected because (a) the SST loader already has its own contract tests in `internal/config/`, and (b) the test names (`TestWeather*Wiring`) suggest they exist to prove the wiring path is end-to-end correct, but in their current form they only prove the helper is callable — a narrower test than the name claims. Sequel packet candidate if cross-package SST-to-connector wiring assertions are wanted (out of scope for HL-RESCAN-014).

### DD-4: Import pruning strategy — delete orphaned `math`, `os`, `strconv`; keep `encoding/json`, `log/slog`

**Decision:** After deleting the 5 dead-set helpers, the import block in `cmd/core/helpers.go` becomes:

```go
package main

import (
    "encoding/json"
    "log/slog"
)
```

The orphaned imports `math`, `os`, and `strconv` MUST be removed. They are used **only** by the 5 dead-set helpers:

- `math` — used only by `parseFloatEnv` (`math.IsNaN`, `math.IsInf`).
- `os` — used only by `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseFloatEnv` (`os.Getenv`).
- `strconv` — used only by `parseFloatEnv` (`strconv.ParseFloat`).

The surviving imports are:

- `encoding/json` — used by `parseJSONArrayVal` (live, called by `parseJSONArray`).
- `log/slog` — used by `parseJSONArrayVal` (live, for `slog.Warn` on parse error).

**Rationale:** Go's `go build` and `goimports` will fail compilation if an import is present without being referenced. Leaving orphaned imports would block `./smackerel.sh test unit --go` at the build stage and falsify SCN-020-003-D (suite-green canary). Pruning them in the same change keeps the post-fix tree compilable.

The pruning is mechanical: after deleting the 5 helpers, run `goimports -w cmd/core/helpers.go` (or do the equivalent manual edit) to remove the unreferenced imports. The post-fix file should have exactly 2 imports.

**Alternatives rejected:**

- **Keep `os` for documentation purposes (e.g., a comment explaining that env reads are forbidden).** Rejected because Go does not allow unused imports; the build would fail. A comment in the file documenting Gate G028 would belong in a Go doc comment at the package level, but the existing in-tree skill `.github/skills/smackerel-no-defaults/SKILL.md` and the regression guard test docstring already cover this.
- **Use `_ "os"` blank-import to satisfy the doc-comment intent.** Rejected because blank-importing standard-library packages is non-idiomatic Go and would itself be a code smell.

### DD-5: Adversarial regression guard — AST-based test in `cmd/core/helpers_no_defaults_test.go`

**Decision:** Create a new file `cmd/core/helpers_no_defaults_test.go` containing a single test function `TestNoSilentFallbackHelpersInCmdCore` that:

1. Walks every `*.go` file under `cmd/core/` (recursive), excluding `_test.go` files.
2. For each file, parses the AST via `go/parser` + `go/token`.
3. For each top-level function declaration in the parsed AST, inspects the function body for the silent-fallback signature pattern: a call to `os.Getenv(...)` whose return is bound to a local variable, followed by control flow that returns a literal or `nil` on the empty-string branch without producing an error.
4. Fails loud with `t.Errorf` reporting `file:line: function <name> matches silent-fallback signature shape (returns literal/nil on empty os.Getenv result without error propagation) — Gate G028 / HL-RESCAN-014`.

The test runs as part of `./smackerel.sh test unit --go` because it lives under `cmd/core/` and matches the `*_test.go` pattern. Pre-fix (with the 5 dead helpers still in place + this guard added), the test FAILS RED reporting the 3 silent-fallback helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) by file:line. Post-fix (helpers deleted), the test PASSES GREEN. Reverting any of the deletions causes the test to fail again with the same shape — proving the guard mechanically catches the regression.

The test file's package docstring + the function's doc comment include the breadcrumb tokens `Gate G028` and `HL-RESCAN-014`. The error message includes both tokens so the failure surface is self-documenting.

**Rationale:** The repo already has precedent for AST-based contract guards: [`internal/drive/consumers/consumer_contract_test.go`](../../../../internal/drive/consumers/consumer_contract_test.go) uses `go/parser` + `go/token` to walk every non-test Go file under each downstream consumer package and assert no forbidden import path appears. The pattern is well-established and runs as a normal Go test. Reusing the pattern for HL-RESCAN-014 keeps the guard mechanism familiar to maintainers and sidesteps the need for a new tooling category.

The guard inspects the function-body **shape**, not the function **name**. This catches both:

- **Direct revert vector:** a future maintainer adds `func parseFloatEnv(key string) float64 { ... }` back to `helpers.go` to support a new connector. The guard fails because the body matches the silent-fallback shape, regardless of the function name.
- **Copy-paste-into-new-helper vector:** a future maintainer adds `func parseDurationEnv(key string) time.Duration { ... }` with the same fail-soft shape. The guard fails the same way.

The guard is **scoped to `cmd/core/`** because that is the package the helpers live in. Expanding the scope to all of `cmd/`, `internal/`, and `ml/` would make the guard a workspace-wide silent-fallback sweep — a strictly larger commitment than HL-RESCAN-014 calls out, and a sequel packet candidate. The bounded scope keeps this packet's surface area honest.

The guard's RED→GREEN proof is captured in [`report.md`](report.md) per evidence-rules.md: (a) write the test FIRST against the pre-deletion tree (RED — reports the 3 silent-fallback helpers); (b) delete the helpers + tests + prune imports; (c) re-run the test (GREEN — zero matches). The proof is mechanical and reproducible.

**Alternatives rejected:**

- **`go vet` analyzer plugin under `tools/`.** Considered. Rejected because `go vet` analyzers are a heavier abstraction (separate `go.mod`, separate build target, requires plugin registration) and the cmd/core/ scope is small enough that an in-tree test is the right granularity. The repo precedent at `internal/drive/consumers/consumer_contract_test.go` is the right size for this concern.
- **`scripts/` shell script using `grep -E` over the source.** Considered. Rejected because a shell-based grep cannot reliably distinguish `os.Getenv(...)` followed by a fail-loud branch (canonical) from `os.Getenv(...)` followed by a silent-fallback branch (forbidden) — both forms contain the literal string `os.Getenv(`. Only an AST walk can tell them apart by inspecting the control flow. False-positive rate on a shell-grep guard would be too high to ship.
- **Pin the guard to symbol names (delete-list grep).** Considered. Rejected because that catches only the direct-revert vector, not the copy-paste-into-new-helper vector. The shape-based guard catches both.
- **Skip the regression guard entirely; rely on the symbol-removal grep audit (SCN-020-003-A) alone.** Rejected because SCN-020-003-A is a one-shot grep at audit time, not a persistent in-tree guard. AC-5 explicitly requires a persistent in-tree mechanism.

### DD-6: Sequel surface documentation — connectors.go live `parseJSONArray` callers

**Decision:** The two live `parseJSONArray` call sites in [`cmd/core/connectors.go`](../../../../cmd/core/connectors.go) at lines 76 and 103 are NOT touched by this packet. They are documented as a sequel surface in [`uservalidation.md`](uservalidation.md) under a "Sequel packet candidates" section, with the proposed packet ID **BUG-020-004** ("`cmd/core/connectors.go` `parseJSONArray` live callers silently coerce parse errors to empty exclusion lists; convert to fail-loud `(value, error)` reads or refuse connector construction").

The sequel packet (if filed) would:

- Convert `cmd/core/connectors.go:76` (`"exclude_domains": parseJSONArray(cfg.BookmarksExcludeDomains)`) and `cmd/core/connectors.go:103` (`"custom_skip_domains": parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`) to a fail-loud read pattern that either (a) propagates the parse error up to the bookmarks / browser-history wiring block which decides whether to refuse construction with `slog.Error`, or (b) refactors `parseJSONArray` itself to a `(value, error)`-returning signature.
- Touch the live test cases for `parseJSONArray` (the 8 preserved cases in `cmd/core/main_test.go`) to assert the new fail-loud contract.
- Decide the appropriate severity (likely P3 — silent empty exclusion list is a privacy / completeness concern but not a security or stability concern; an empty exclusion list means no domains are excluded from indexing, which is the connector's own configurable concern).

**Rationale:** The two live callers were introduced by spec 019 connector-wiring rounds and are arguably a SEPARATE finding from HL-RESCAN-014 (which is bounded to UNUSED helpers). The self-hosted readiness rescan did not enumerate them, so they are not part of the rescan's accountability surface. Filing them as a sequel keeps the rescan's finding set crisp (one packet per finding) and lets the operator decide whether to take on the connector-wiring refactor as a follow-up.

The sequel surface is documented in [`uservalidation.md`](uservalidation.md) (the operator-facing artifact) rather than in [`spec.md`](spec.md) (the bug specification) because it is forward-looking work the operator may or may not choose to file. [`spec.md`](spec.md) Out of Scope already explicitly lists `cmd/core/connectors.go` lines 76 and 103 as out of scope; the sequel-surface documentation in [`uservalidation.md`](uservalidation.md) provides the operator the actionable next step if they choose to take it on.

**Alternatives rejected:**

- **File BUG-020-004 immediately as a stub packet alongside BUG-020-003.** Considered. Rejected because (a) filing two packets concurrently violates the bugfix-fastlane discipline of single-bug-scope per packet, (b) the sequel work has a non-trivial design surface (decide between (a) error propagation up to the wiring block vs (b) `parseJSONArray` signature refactor — see DD-1's "canonical inline pattern vs shared helper" debate), and (c) the operator may legitimately decide the silent-empty-exclusion-list behaviour is acceptable for their threat model and skip filing the sequel.
- **Bundle the sequel work into this packet despite the scope-discipline argument.** Rejected per DD-1 / DD-2 scope-discipline rationale.
- **Document the sequel surface only as a comment in `cmd/core/connectors.go`.** Considered. Rejected because (a) source-code TODO-style comments are operator-invisible (they have to read the source to find them), and (b) the [`uservalidation.md`](uservalidation.md) sequel surface is the canonical location per BUG-020-002's precedent at uservalidation.md line 107 (which itself documented HL-RESCAN-014 as a sequel candidate before this packet was filed).

## Trade-offs & Alternatives

| Option | Pros | Cons | Decision |
|---|---|---|---|
| **A. Pure deletion of 5 dead helpers + 31 dead tests + new AST regression guard** (chosen) | Mechanically smallest fix; mirrors BUG-044-001 precedent for unused-helper case; removes the foothold for forbidden pattern entirely; persistent guard catches both revert + copy-paste vectors | Leaves 2 live `parseJSONArray` callers in `cmd/core/connectors.go` for sequel packet | Chosen — DD-1 |
| B. Deletion of 5 dead helpers + fail-loud refactor of `parseJSONArray` callers in `cmd/core/connectors.go` | Closes the silent-`nil` concern in production wiring as well | Violates single-bug-scope discipline; HL-RESCAN-014 is bounded to UNUSED helpers; bundles unrelated production change | Rejected — DD-1, DD-2 |
| C. Convert all 3 `Env`-suffixed helpers to fail-loud `(value, error)` signatures (no deletion) | Preserves helper API surface | Helpers have ZERO callers, so conversion achieves no behavioural change; signatures still advertise "shortcut" ergonomic; would still need adversarial guard against revert | Rejected — DD-1 |
| Keep dead-set tests, rewrite as fail-loud assertions | Locks fail-loud contract on the helpers | Helpers being deleted — nothing to assert against | Rejected — DD-3 |
| Move dead-set tests to `_disabled_test.go.txt` archive | Preserves test bodies for archaeology | Test-hygiene noise; git history already preserves them | Rejected — DD-3 |
| Skip import pruning, rely on `goimports` to fix later | Fewer manual changes | Compilation fails immediately; falsifies SCN-020-003-D | Rejected — DD-4 |
| Shell-grep regression guard instead of AST guard | No new Go code | Cannot distinguish fail-loud from silent-fallback; high false-positive rate | Rejected — DD-5 |
| Symbol-name regression guard (delete-list grep) | Simpler test | Misses copy-paste-into-new-helper vector | Rejected — DD-5 |
| Skip persistent regression guard; rely on SCN-020-003-A grep at audit time | Smaller change | One-shot grep ≠ persistent in-tree guard; falsifies AC-5 | Rejected — DD-5 |
| File BUG-020-004 immediately as stub packet | Visible commitment to sequel work | Violates single-bug-scope discipline; sequel design surface not yet resolved | Rejected — DD-6 |
| Document sequel surface only via source-code TODO comment | Co-located with code | Operator-invisible; uservalidation.md is canonical per BUG-020-002 precedent | Rejected — DD-6 |

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Deleting `parseJSONArray` (LIVE) by mistake breaks bookmarks + browser-history connector wiring at `cmd/core/connectors.go:76,103` | Low | High (compilation failure + connector regression) | Scope discipline (DD-2) — the 5 dead-set list is bounded and explicitly enumerated. The implementer follows the file-by-file delete list in [`scopes.md`](scopes.md). The `parseJSONArray` and `parseJSONArrayVal` symbols are NOT in the delete list. SCN-020-003-E is a positive canary that asserts the two live call sites compile and behave identically post-fix. |
| Deleting the 31 test functions shifts line numbers and invalidates references in adjacent doc comments or other tests | Low | Low (doc-comment drift) | After the delete, run `git diff --stat cmd/core/main_test.go` to confirm only the 31 functions are removed. Run `grep_search` for any line-number-bearing references to the deleted test ranges (none expected — Go tests do not cross-reference each other by line number). |
| The persistent AST regression guard catches false positives if the pattern matcher is too broad | Medium (during initial implementation) | Medium (test thrashing) | Pin the matcher to the EXACT silent-fallback signature shape per DD-5: function takes `key string` AND body contains `os.Getenv(...)` AND control flow returns a literal/nil on the empty-string branch WITHOUT error propagation. Validate the matcher against the canonical fail-loud reads in `cmd/core/wiring.go` (`SMACKEREL_AUTH_TOKEN`, `SMACKEREL_ENV`, `SMACKEREL_DEV_BYPASS_AUTH`, `HOSTNAME` via `resolveBroadcasterInstanceID`) — if the matcher flags any of those as silent-fallback, it is too broad and must be tightened. RED proof against the pre-deletion tree confirms the matcher catches the 3 known violations; absence of false positives in the canonical reads confirms the matcher is not too broad. |
| The AST regression guard fails to catch a silent-fallback variant the matcher does not anticipate (e.g., `os.LookupEnv` instead of `os.Getenv`) | Medium (over the long run) | Medium (delayed detection) | Document the matcher's known coverage (limited to `os.Getenv` callers in `cmd/core/`) in the test docstring + uservalidation.md as a known sequel surface. A future packet (BUG-020-005 candidate) can extend the matcher to cover `os.LookupEnv` and other variants. The current matcher closes the HL-RESCAN-014 finding as enumerated; broader sweep is a separate commitment. |
| A future maintainer who needs a numeric env value re-introduces `parseFloatEnv` because they don't know it was deleted | Medium (over the long run) | High (Gate G028 violation) | Three layers of defence: (a) the persistent AST regression guard mechanically fails on re-introduction; (b) the regression guard's failure message names HL-RESCAN-014 and Gate G028, providing the breadcrumb back to this packet; (c) the canonical inline pattern is documented in `.github/skills/smackerel-no-defaults/SKILL.md` and demonstrated in `cmd/core/wiring.go`. |
| Pre-existing `cmd/core/connectors.go:76,103` `parseJSONArray` calls become orphaned because `parseJSONArray` was accidentally deleted | Very Low | High (compilation failure) | Compile-time guarantee — Go's compiler refuses to build a file that calls a non-existent function. SCN-020-003-D (suite-green canary via `./smackerel.sh test unit --go`) catches this immediately. SCN-020-003-E adds a positive canary on the two specific call sites. |
| The 3 stale `TestWeather*Wiring` and 2 `TestMarketsFredSeriesWiring` tests are deleted and the operator believes the production wiring is no longer covered | Low | Low (perceived loss of coverage) | The tests being deleted ALREADY do not cover the production wiring path (they exercise the dead helpers, not the SST loader → struct field → connector wiring path). The actual wiring is covered by `internal/config/` SST loader tests. Document this in [`report.md`](report.md) under the "test-coverage delta" section. The operator can confirm by running `grep_search` for `cfg.WeatherForecastDays` to see the actual production read sites. |
| Parallel-session WIP under `specs/041-qf-companion-connector/` or `specs/052-bundle-secret-injection-contract/` introduces a conflict | Very Low | Low | This packet does not touch any file under `specs/041-*` or `specs/052-*`. The two sessions modify disjoint surfaces. Verify with `grep_search` for the parallel-session paths in this packet's diff before commit. |
| The new `cmd/core/helpers_no_defaults_test.go` file conflicts with a parallel session adding a different test file at the same path | Very Low | Low | The file name is unique to HL-RESCAN-014 and unlikely to collide. Verify with `file_search` for `helpers_no_defaults*` before adding. |

**Rollback plan:** If the post-fix tree fails `./smackerel.sh test unit --go` for any reason not caught by SCN-020-003-D, revert the three file changes via `git checkout cmd/core/helpers.go cmd/core/main_test.go && git rm cmd/core/helpers_no_defaults_test.go`. The rollback is mechanically safe because (a) the 5 deleted helpers had ZERO live callers (verified pre-deletion), so no production code path depends on the deletion landing, (b) the 31 deleted tests were stale (verified by spec evidence), so no production behaviour assertion depends on them, and (c) the new regression-guard test is purely additive. No data migrations, no service restarts, no operator-visible side effects — pure source-code rollback.

## Acceptance Criteria Mapping

Map each AC from [`spec.md`](spec.md) to the design decision(s) that satisfy it.

| AC | Spec text (abbreviated) | Satisfied by |
|---|---|---|
| **AC-1** | Bug packet exists at `specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/` with all 7 standard artifacts, correct `parentWorkflow.mode` and `discoveryRef` | Already satisfied by `bubbles.bug` (skeleton) + this design.md (Overview & Approach + DD-1 through DD-6) + downstream `bubbles.plan` for scopes.md / report.md / uservalidation.md |
| **AC-2** | Post-fix `grep_search` of all `*.go` for the 5 dead-set symbols returns ZERO matches | DD-1 (Option A pure deletion) + DD-2 (file-by-file delete list); SCN-020-003-A is the executable contract |
| **AC-3** | `cmd/core/helpers.go` contains no `os.Getenv("KEY")` + return-on-empty silent-fallback path; any remaining env reads use Gate G028 canonical pattern | DD-1 (delete the 3 `Env`-suffixed helpers — the only env readers in the file) + DD-4 (prune `os` import — proves no remaining env reads); SCN-020-003-B is the executable contract |
| **AC-4** | `cmd/core/main_test.go` does not contain test functions that lock silent-fallback semantics for deleted helpers | DD-3 (pure deletion of 31 dead-set tests; no rewrite); preserved 8 `TestParseJSONArray_*` tests cover the LIVE helper |
| **AC-5** | Persistent in-tree adversarial regression guard mechanically catches future re-introduction of FORBIDDEN pattern; runs on every `./smackerel.sh test unit --go`; failure names file + line + symbol | DD-5 (AST-based `cmd/core/helpers_no_defaults_test.go`); SCN-020-003-C is the executable contract |
| **AC-6** | All existing Go unit tests pass after deletion; `./smackerel.sh test unit --go` returns exit 0; pre-existing `parseJSONArray` callers untouched | DD-2 (scope boundary excludes `cmd/core/connectors.go`) + DD-3 (preserve 8 `TestParseJSONArray_*` tests) + DD-4 (prune orphaned imports to keep tree compilable); SCN-020-003-D is the executable suite-green canary; SCN-020-003-E is the positive canary on the two live call sites |
| **AC-7** | Generic-only constraint preserved; PII paths redacted to `~/smackerel`; `Gate G028` and `HL-RESCAN-014` tokens explicitly allowed | Universal across all DDs — no real hostnames, IPs, tailnet IDs, owner-username tokens introduced. Evidence-block PII redaction enforced in [`report.md`](report.md) per evidence-rules.md |

Every AC is covered by at least one design decision; no AC requires an escalation back to `bubbles.bug` for spec rework.

## Implementation Handoff

The next required owner is `bubbles.plan`. The planner consumes this design.md + [`spec.md`](spec.md) + [`scenario-manifest.json`](scenario-manifest.json) and produces:

- [`scopes.md`](scopes.md) — scope definition with Tiered DoD, Test Plan table mapping the 5 SCN-020-003-* scenarios to concrete test files and test names, and Gherkin scenario refinement.
- [`report.md`](report.md) — execution-evidence template with required headers (code-diff evidence, test evidence, audit evidence, RED→GREEN proof for SCN-020-003-C).
- [`uservalidation.md`](uservalidation.md) — operator-facing AC verification checklist with the BUG-020-004 sequel-packet pointer per DD-6.

After plan, the implementation specialist (`bubbles.implement`) executes the file-by-file changes documented in [`scopes.md`](scopes.md), the test specialist (`bubbles.test`) runs the proofs documented in [`report.md`](report.md), and the validation specialist (`bubbles.validate`) closes out via [`uservalidation.md`](uservalidation.md).
