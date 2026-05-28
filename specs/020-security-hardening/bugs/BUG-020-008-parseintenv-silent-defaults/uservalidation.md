# User Validation: [BUG-020-008] `parseIntEnv` silent SST defaults

## Checklist

### [Bug Fix] [BUG-020-008] Required SST int config fails loud on missing or unparseable env input
- [x] **What:** `internal/config/config.go` no longer silently substitutes `0` for missing or unparseable SST int env vars; all 8 keys (`BOOKMARKS_MIN_URL_LENGTH`, `BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS`, `BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD`, `BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY`, `QF_DECISIONS_PACKET_VERSION`, `QF_DECISIONS_PAGE_SIZE`, `HOSPITABLE_INITIAL_LOOKBACK_DAYS`, `HOSPITABLE_PAGE_SIZE`) fail loud via `Config.Validate()` with a consolidated missing-key error.
  - **Steps:**
    1. Inspect `internal/config/config.go` and confirm `parseIntEnv(key, defaultVal int) int` is GONE.
    2. Run `grep -nE "parseIntEnv\(.*,\s*0\)" internal/config/config.go` and confirm zero matches (exit 1).
    3. Inspect the 4 new unit tests in `internal/config/config_test.go` (or sibling): `TestValidate_MissingSingleIntKey_FailsLoud`, `TestValidate_MissingAllIntKeys_ConsolidatedError`, `TestValidate_UnparseableIntKey_FailsLoud`, `TestValidate_AllIntKeysValid_NoError` (or equivalent names).
    4. Run `./smackerel.sh test unit` and confirm exit 0 with the 4 new tests in the output.
    5. Run `./smackerel.sh build` and confirm exit 0.
    6. As an adversarial check, unset `BOOKMARKS_MIN_URL_LENGTH` (or any one of the 8) in your dev env and boot the core service; confirm it ABORTS at boot with a clear error naming the missing key.
  - **Expected:** A missing or typo'd SST int env var aborts boot with a consolidated `Validate()` error; the runtime never starts with a silent `0`. The 4 unit tests cover missing-single, missing-all, unparseable, and all-valid cases.
  - **Verify:** Steps 2 + 4 + 6 above. Step 2 is the structural guard; step 4 is the regression guard; step 6 is the live-stack guard.
  - **Evidence:** report.md → Post-Fix Regression Test (PENDING — will populate during Phase 5 implementation)
  - **Notes:** Code-review finding H-1 (P0 SST NO-DEFAULTS violation). Spec association: `specs/020-security-hardening/`. Workflow mode: `bugfix-fastlane` (NOT tdd.exempt — this IS a code change with real Red→Green tests).
