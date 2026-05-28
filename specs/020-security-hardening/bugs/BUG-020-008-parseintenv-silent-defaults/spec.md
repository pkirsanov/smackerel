# Spec: [BUG-020-008] Required SST int config MUST fail loud on missing or unparseable env input

## Expected Behavior

### EB-1: `parseIntEnv` silent-default helper is removed or rewritten to fail loud
`internal/config/config.go` MUST NOT carry any helper that silently substitutes a fallback int value for a required SST env var. The current `parseIntEnv(key, defaultVal int) int` (L1777–1789) MUST either be deleted or replaced by a helper that returns an error (e.g., `mustParseIntEnv(key string) (int, error)`) so the caller is forced to surface missing or unparseable input through `Config.Validate()`.

### EB-2: All 8 SST int call-sites route through the `Validate()` chain
The following 8 env vars MUST be added to whatever required-key set `Config.requiredVars()` (or the equivalent missing-key collector used by `Validate()`) maintains, so a missing or empty value produces a single consolidated error naming every absent key:

| # | Env var | Field | Current line |
|---|---------|-------|--------------|
| 1 | `BOOKMARKS_MIN_URL_LENGTH` | `BookmarksMinURLLength` | L475 |
| 2 | `BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS` | `BrowserHistoryInitialLookbackDays` | L481 |
| 3 | `BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD` | `BrowserHistoryRepeatVisitThreshold` | L486 |
| 4 | `BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY` | `BrowserHistoryContentFetchConcurrency` | L488 |
| 5 | `QF_DECISIONS_PACKET_VERSION` | `QFDecisionsPacketVersion` | L568 |
| 6 | `QF_DECISIONS_PAGE_SIZE` | `QFDecisionsPageSize` | L569 |
| 7 | `HOSPITABLE_INITIAL_LOOKBACK_DAYS` | `HospitableInitialLookbackDays` | L576 |
| 8 | `HOSPITABLE_PAGE_SIZE` | `HospitablePageSize` | L577 |

### EB-3: Unparseable values also fail loud
A non-empty env value that fails `strconv.Atoi` MUST produce a clear `Validate()` error naming the offending key and the offending value. It MUST NOT silently fall back to `0` or any other implicit value.

### EB-4: Validate() error is consolidated
When multiple of the 8 keys are missing, `Validate()` MUST return a single error that names every missing key in one pass (matching the existing pattern for other required SST values). The operator MUST NOT have to reboot N times to discover N missing keys.

### EB-5: SST contract preserves the keys
`config/smackerel.yaml` and the `./smackerel.sh config generate` pipeline MUST continue to materialize all 8 keys into `config/generated/dev.env` and `config/generated/test.env`. If any of the 8 keys is currently absent from `smackerel.yaml`, it MUST be added with an explicit, fail-loud value (no silent default).

## Acceptance Criteria
1. `grep -nE "parseIntEnv\\(.*,\\s*0\\)" internal/config/config.go` returns zero matches after the fix.
2. A unit test asserts that unsetting any one of the 8 env vars and calling `Config.Validate()` returns an error whose message contains the missing key name.
3. A unit test asserts that unsetting all 8 simultaneously returns a single consolidated error that names all 8 keys.
4. A unit test asserts that setting any one of the 8 to a non-numeric value (e.g., `"abc"`) returns a `Validate()` error naming both the offending key and the offending value.
5. `./smackerel.sh build` passes after the change.
6. `./smackerel.sh test unit` passes after the change.
7. `./smackerel.sh config generate` produces `dev.env` and `test.env` that include all 8 keys with non-empty, parseable int values.
8. `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults` passes.
