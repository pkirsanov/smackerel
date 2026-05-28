# Bug Fix Design: [BUG-020-008] `parseIntEnv` silent SST defaults

> **STATUS:** Initial design authored by `bubbles.bug` during Phase 2 documentation. The implementing agent (`bubbles.design` via `runSubagent` in Phase 3) MUST review and refine before code edits begin.

## Root Cause Analysis

### Investigation Summary
Code-review pass surfaced finding H-1 (P0). The helper `parseIntEnv(key, defaultVal int) int` at `internal/config/config.go` L1777–1789 is the only int-from-env helper in `Load()` and is the textbook silent-default anti-pattern that `.github/instructions/smackerel-no-defaults.instructions.md` exists to ban:

```go
func parseIntEnv(key string, defaultVal int) int {
    s := os.Getenv(key)
    if s == "" {
        return defaultVal              // silent empty-string fallback
    }
    v, err := strconv.Atoi(s)
    if err != nil {
        return defaultVal              // silent parse-error fallback
    }
    return v
}
```

Eight `Load()` call-sites pass `0` as the silent fallback for SST-required int values (see bug.md → Error Output). None of the 8 keys are registered with the existing `Config.requiredVars()` / `Validate()` missing-key collector, so a typo or missing env produces a runtime `0` with no boot-time signal — exactly the failure mode the NO-DEFAULTS regime exists to prevent.

The string-valued SST fields in the same `Load()` block already use direct `os.Getenv(...)` and are validated by `Validate()`; the int fields were the only ones routed through a fallback helper.

### Root Cause
A helper that pre-dates the NO-DEFAULTS regime (or was written before the SST contract was hardened) was kept in place when the 8 int fields were added. The fallback semantics of `parseIntEnv` directly contradict the binding policy in `.github/copilot-instructions.md` → SST Zero-Defaults Enforcement (Go column: `os.Getenv("KEY")` + empty check → fatal). Gap is in source code, not in compose / docs / deploy.

### Impact Analysis
- **Affected components:** `internal/config/config.go` only.
- **Affected data:** none directly, but downstream behavior depends on each consumer:
  - `BookmarksMinURLLength == 0` → bookmark URL filter degenerates (every URL passes the min-length check).
  - `BrowserHistoryInitialLookbackDays == 0` → no history is initially ingested OR all-of-time is ingested, depending on consumer semantics.
  - `BrowserHistoryRepeatVisitThreshold == 0` → every visit is "repeat" OR no visit is "repeat", depending on consumer.
  - `BrowserHistoryContentFetchConcurrency == 0` → either no concurrency or unbounded — both wrong.
  - `QFDecisionsPacketVersion == 0` → cross-product (QF) packet version is invalid; downstream consumers reject or mis-route.
  - `QFDecisionsPageSize == 0` → page-size of 0 either disables pagination or breaks loops.
  - `HospitableInitialLookbackDays == 0`, `HospitablePageSize == 0` → same shape: silently broken connector.
- **Affected users:** any operator who misspells an env name, omits a key from a generated env file, or rolls back `config/smackerel.yaml` past a key addition. Failure is silent at boot and surfaces as a wrong-behavior bug later.
- **Affected runtime:** boot path of `cmd/core` via `internal/config.Load()` → `Config.Validate()`.

## Fix Design

### Solution Approach
Three coordinated edits inside `internal/config/config.go`:

1. **Replace `parseIntEnv` with a fail-loud helper.** Introduce `mustParseIntEnv(key string) (int, error)` (or equivalent) that returns `(0, error)` on empty and `(0, error)` on `strconv.Atoi` failure, with the error message naming the env key and (for parse failures) the offending value. Delete the old `parseIntEnv` after migration.
2. **Migrate the 8 call-sites.** Each of the 8 lines becomes a two-step pattern: read into a local int, accumulate the error into the `requiredVars()` missing-keys collector (so `Validate()` returns a single consolidated error). The exact mechanical shape (collect-then-validate vs. early-return on first error) MUST match the existing pattern used by the other required SST string fields — read `Config.Validate()` and `Config.requiredVars()` before choosing.
3. **Register the 8 keys with `requiredVars()`.** Whichever required-key set `Validate()` consults, the 8 new int keys MUST be members. The consolidated error message MUST list every missing key in one pass.

The implementing agent MUST NOT take shortcuts:
- DO NOT use `mustParseIntEnv` and `log.Fatal` inline at each call-site — that bypasses the consolidated `Validate()` error and forces N reboots to discover N missing keys.
- DO NOT add a per-key default in `config/smackerel.yaml` to "make the test pass" — that hides the missing-key signal.
- DO NOT mark fields as optional by changing the spec — these 8 fields are SST-required per their connectors / cross-product contract.

### Affected Files
| File | Change |
|------|--------|
| `internal/config/config.go` | Replace `parseIntEnv` with `mustParseIntEnv` (or equivalent); migrate 8 call-sites; register 8 keys with `requiredVars()` / `Validate()` |
| `internal/config/config_test.go` (or new sibling test file) | Add the 4 unit tests required by spec.md Acceptance Criteria #2–#4 |
| `config/smackerel.yaml` | If any of the 8 keys is missing, add it with an explicit non-empty int value (per EB-5) |

### Alternative Approaches Considered
1. **Keep `parseIntEnv` but log a warning when defaultVal is returned.** Rejected — warnings are easily missed in dev and lost in prod logs; the NO-DEFAULTS regime is fail-loud, not fail-noisy. Same anti-pattern, lower volume.
2. **Make the 8 fields genuinely optional with documented defaults.** Rejected — every one of the 8 is SST-required by its consumer (bookmarks filter min length, browser-history lookback window, QF cross-product packet version, hospitable connector page size). A real "0" value is meaningless for all 8.
3. **Move the helper out of `config.go` into a shared `envutil` package.** Rejected as part of this fix — orthogonal refactor; do not expand the change boundary. If `envutil` is desired later, file a separate scope.
4. **Use struct tags + reflection to enforce required ints.** Rejected — large mechanism for a small problem; the existing `Validate()` / `requiredVars()` pattern already exists and is the obvious extension point.

### Regression Test Design
Four unit tests in `internal/config/config_test.go` (or a sibling file):

1. **`TestValidate_MissingSingleIntKey_FailsLoud`** — table-driven over the 8 keys; for each key, unset that env var (leave the other 7 set to valid ints), call `Config.Validate()`, assert the returned error contains the missing key name.
2. **`TestValidate_MissingAllIntKeys_ConsolidatedError`** — unset all 8 simultaneously, call `Validate()`, assert the single returned error contains all 8 key names.
3. **`TestValidate_UnparseableIntKey_FailsLoud`** — table-driven over the 8 keys; set each to `"abc"`, call `Validate()`, assert the error names both the key and the offending value.
4. **`TestValidate_AllIntKeysValid_NoError`** — set all 8 to valid non-zero ints, assert `Validate()` returns nil (no false-positive failures).

**Adversarial requirement (NON-NEGOTIABLE):** The "unset key" test cases MUST physically `os.Unsetenv` the key in the test setup. A tautological test that calls `Validate()` against an already-valid env is forbidden — it would pass even if the bug were reintroduced.

**Pre-fix evidence:** Tests #1–#3 above MUST be authored AND executed BEFORE the helper migration. They MUST FAIL against `main` because today `parseIntEnv` returns `0` instead of failing loud, and the 8 keys are absent from `requiredVars()`. This is the Red phase of the TDD requirement.

### Rollback Plan
Revert the `internal/config/config.go` and `internal/config/config_test.go` edits. The bug returns immediately on revert; no data migration; no schema change.

## Verification Plan
- `grep -nE "parseIntEnv\\(.*,\\s*0\\)" internal/config/config.go` returns zero matches.
- `./smackerel.sh build` passes.
- `./smackerel.sh test unit` passes, and the 4 new tests are in the test output.
- `./smackerel.sh config generate` produces dev / test env files that include all 8 keys with non-empty, parseable int values.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults` passes.
