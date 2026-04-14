# Bug: parseFloatEnv Accepts IEEE 754 Special Values (Inf, NaN)

**Bug ID:** BUG-019-001
**Severity:** High
**Found by:** bubbles.chaos (stochastic-quality-sweep R06)
**Feature:** 019-connector-wiring

---

## Problem

`parseFloatEnv` in `cmd/core/main.go` uses `strconv.ParseFloat` to convert environment variable strings to `float64`. Go's `strconv.ParseFloat` successfully parses `"Inf"`, `"-Inf"`, `"+Inf"`, and `"NaN"` without error, returning the corresponding IEEE 754 special values.

These non-finite values flow into connector `SourceConfig` maps and bypass downstream validation that uses ordinary comparison operators:

- `NaN > 0` → always `false`
- `NaN < 0` → always `false`
- `NaN >= threshold` → always `false`
- `+Inf > 0` → `true`, but `changePct >= +Inf` → always `false`

### Concrete Impact

| Env Var | Value | Effect |
|---------|-------|--------|
| `FINANCIAL_MARKETS_ALERT_THRESHOLD` | `NaN` | `classifyTier()` always returns `"light"` — all market alerts silently suppressed |
| `FINANCIAL_MARKETS_ALERT_THRESHOLD` | `Inf` | Same — no finite change exceeds infinity |
| `DISCORD_BACKFILL_LIMIT` | `Inf` | `int(+Inf)` is implementation-defined per Go spec — could be `MinInt64` or platform-dependent |
| `GOV_ALERTS_MIN_EARTHQUAKE_MAG` | `NaN` | Already guarded by alerts connector (has `math.IsNaN`/`math.IsInf` check) |
| All `MAPS_*` float vars | `NaN`/`Inf` | Semantically wrong distance/duration thresholds silently accepted |

### Root Cause

`parseFloatEnv` validates only for empty string and parse errors. `strconv.ParseFloat` considers `Inf`/`NaN` valid parses (returns nil error), so these values pass through silently.

---

## Fix

Add `math.IsNaN` and `math.IsInf` rejection to `parseFloatEnv` immediately after successful parse. Log a warning and return 0, consistent with the existing error-fallback behavior.

## Regression Test Cases

1. `parseFloatEnv` with env var set to `"Inf"` → must return 0
2. `parseFloatEnv` with env var set to `"-Inf"` → must return 0
3. `parseFloatEnv` with env var set to `"+Inf"` → must return 0
4. `parseFloatEnv` with env var set to `"NaN"` → must return 0
5. `parseFloatEnv` with env var set to `"nan"` (lowercase) → must return 0
6. `parseFloatEnv` with env var set to `"3.14"` → must still return 3.14 (no regression)
