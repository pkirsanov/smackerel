# Bug: [BUG-020-008] `parseIntEnv` silently substitutes `0` for 8 required SST int config values

## Summary
`internal/config/config.go` defines `parseIntEnv(key, defaultVal int) int` (L1777–1789) which silently returns `defaultVal` when the env var is empty OR unparseable. Eight call-sites in `Load()` pass `0` as the silent fallback for required SST int config values (`BOOKMARKS_MIN_URL_LENGTH`, `BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS`, `BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD`, `BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY`, `QF_DECISIONS_PACKET_VERSION`, `QF_DECISIONS_PAGE_SIZE`, `HOSPITABLE_INITIAL_LOOKBACK_DAYS`, `HOSPITABLE_PAGE_SIZE`). A misnamed, missing, or typo'd env var produces a runtime `0` instead of failing loud at boot, directly violating `.github/instructions/smackerel-no-defaults.instructions.md` and the `## SST Zero-Defaults Enforcement (NON-NEGOTIABLE)` section of `.github/copilot-instructions.md` which explicitly lists `getEnv("KEY", "fallback")` as FORBIDDEN.

## Severity
- [x] Critical — silent SST defaults are exactly the failure mode the NO-DEFAULTS regime is built to prevent; a misnamed bookmarks/browser-history/QF/hospitable env var ships with `0` instead of aborting boot
- [ ] High
- [ ] Medium
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Read `internal/config/config.go` L1777–1789 — `parseIntEnv` returns `defaultVal` on both empty and unparseable input, with no `Validate()` registration.
2. Read L475, L481, L486, L488, L568, L569, L576, L577 — 8 call-sites pass `0` as the silent fallback for SST-required int values.
3. Adversarial repro: unset `BOOKMARKS_MIN_URL_LENGTH` (or misspell it as `BOOKMARK_MIN_URL_LENGTH`) and boot Smackerel. The runtime starts with `Config.BookmarksMinURLLength == 0` instead of aborting at boot with a consolidated missing-key error from `Validate()`.
4. Compare against the policy in `.github/instructions/smackerel-no-defaults.instructions.md` → "FORBIDDEN: `os.getenv("KEY", "default")`" and `getEnv("KEY", "fallback")` per the Go column of the SST Zero-Defaults table in `copilot-instructions.md`.

## Expected Behavior
- For each of the 8 SST int values, missing or unparseable env input MUST abort boot via the existing `Config.Validate()` chain with a consolidated missing-key error that names the offending env var(s).
- A helper that returns an int from an env var MUST either (a) return an error and force the caller to fail loud, or (b) be confined to values that are genuinely optional with a designed-default semantic (and no SST-required value qualifies).
- `Validate()` MUST surface all 8 keys in the same missing-keys batch it already produces for other required SST values, so an operator sees every missing key in one pass instead of N reboot cycles.

## Actual Behavior
- `parseIntEnv` silently substitutes `0` for empty or unparseable input.
- A missing or typo'd env name produces a runtime `0` with no boot abort and no log signal.
- Downstream behavior depends on whether each consumer treats `0` as "disabled," "unlimited," "first page," or "broken" — none of which are the operator's intent when the value is actually missing.

## Environment
- Repo: `smackerel` @ current `main`
- Affected source: `internal/config/config.go` (L1777–1789 helper; L475, L481, L486, L488, L568, L569, L576, L577 call-sites)
- Authoritative policy: `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement
- Spec association: `specs/020-security-hardening/` (SST / NO-DEFAULTS regime owner)
- Code-review finding: H-1 (P0 SST NO-DEFAULTS violation)

## Error Output
```
internal/config/config.go:1777  func parseIntEnv(key string, defaultVal int) int {
internal/config/config.go:1779      if s == "" {
internal/config/config.go:1780          return defaultVal           // SILENT DEFAULT — forbidden by NO-DEFAULTS regime
internal/config/config.go:1783      v, err := strconv.Atoi(s)
internal/config/config.go:1785          return defaultVal           // SILENT DEFAULT on parse error — forbidden
internal/config/config.go:475       BookmarksMinURLLength:                  parseIntEnv("BOOKMARKS_MIN_URL_LENGTH", 0),
internal/config/config.go:481       BrowserHistoryInitialLookbackDays:      parseIntEnv("BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS", 0),
internal/config/config.go:486       BrowserHistoryRepeatVisitThreshold:     parseIntEnv("BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD", 0),
internal/config/config.go:488       BrowserHistoryContentFetchConcurrency:  parseIntEnv("BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY", 0),
internal/config/config.go:568       QFDecisionsPacketVersion:               parseIntEnv("QF_DECISIONS_PACKET_VERSION", 0),
internal/config/config.go:569       QFDecisionsPageSize:                    parseIntEnv("QF_DECISIONS_PAGE_SIZE", 0),
internal/config/config.go:576       HospitableInitialLookbackDays:          parseIntEnv("HOSPITABLE_INITIAL_LOOKBACK_DAYS", 0),
internal/config/config.go:577       HospitablePageSize:                     parseIntEnv("HOSPITABLE_PAGE_SIZE", 0),
```

## Root Cause (filled after analysis)
See design.md.

## Related
- Feature: `specs/020-security-hardening/`
- Policy: `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement
- Code-review finding: H-1 (P0)
