# BUG-039-004: price-drop watch threshold has a hardcoded 0.10 fallback (NO-DEFAULTS violation)

**Status:** Resolved (hardcoded fallback → fail-loud SST via bugfix-fastlane — see report.md)
**Severity:** Medium (binding NO-DEFAULTS / smackerel-no-defaults policy violation)
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (autonomous open-items sweep)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/039-recommendations-engine/`
**Affected surface:** `internal/recommendation/watch/evaluator.go` (`gatherPriceDropCandidates`), `internal/config/recommendations.go`

## Summary

The price-drop watch evaluator resolved the threshold (the % drop that counts
as a "drop worth processing") with a three-level precedence whose last level was
a **hardcoded in-source literal**:

```go
thresholdPct := numericFromAny(trigger.Context["threshold_pct"])   // 1. trigger payload
if thresholdPct == 0 {
    thresholdPct = numericFromAny(watch.Filters["threshold_pct"])  // 2. user's watch filter
}
if thresholdPct == 0 {
    thresholdPct = 0.10                                            // 3. HARDCODED FALLBACK
}
```

Level 3 (`0.10`) is exactly the kind of fallback runtime value the binding
`smackerel-no-defaults` policy forbids and the owner's "NO const limits"
directive targets. When neither the trigger nor the user's watch supplied a
threshold, the engine silently used `0.10` baked into Go.

## Classification — operational default, not a business judgment (SST, not LLM)

Unlike the six intelligence/digest judgments converted to the LLM
(BUG-021-005..010), the price-drop threshold is a **user-configured value**
(`watch.Filters["threshold_pct"]`) with a trigger override
(`trigger.Context["threshold_pct"]`). The `0.10` is purely the
no-input OPERATIONAL fallback. Per the operational-vs-business boundary applied
across the whole sweep (business reasoning → LLM; operational limits → fail-loud
SST), the correct fix is to make this fallback a fail-loud SST operational
default — NOT to re-architect the user-config-first price-drop model into an LLM
call. The user/trigger threshold still takes precedence; only the missing-input
default moves out of Go.

## Fix (delivered — fail-loud SST)

1. **New SST key** `recommendations.watches.default_price_drop_threshold_pct`
   (`RECOMMENDATIONS_WATCHES_DEFAULT_PRICE_DROP_THRESHOLD_PCT`), loaded via the
   existing `parseUnitFloat` helper with an added `> 0` guard (fraction in
   (0,1]). No in-source default — missing/invalid fails loud at config load.
2. **Threaded through** `watch.Options.DefaultPriceDropThresholdPct` →
   `Evaluator.defaultPriceDropThresholdPct`, wired from
   `cfg.Recommendations.Watches.DefaultPriceDropThresholdPct` in cmd/core.
3. **Extracted** `resolvePriceDropThreshold(trigger, watch, defaultPct)` — the
   trigger → filter → SST-default precedence, now unit-testable without a DB.
   The `0.10` literal is gone; `gatherPriceDropCandidates` calls the helper with
   `e.defaultPriceDropThresholdPct`.

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| User's chosen drop threshold | user (`watch.Filters`) | Per-watch preference |
| Trigger-supplied threshold | trigger payload | Per-evaluation override |
| Fallback default when neither set | SST (`default_price_drop_threshold_pct`) | Operational default, fail-loud |

No price-drop threshold literal remains in Go. The fallback is SST and fails
loud (constitution C8 / smackerel-no-defaults).

## Why no test was coupled to the old literal

The only price-drop integration test (`tests/integration/recommendation_price_watches_test.go`)
sets `threshold_pct: 0.15` in BOTH the watch filter and the trigger context, so
it never exercised the `0.10` fallback. Non-price watch tests never enter
`gatherPriceDropCandidates`. Verified before editing; no behaviour change for
any existing test.

## Relationship to the BUG-021-005..011 sweep

This completes the owner's "NO const limits" sweep: it is the one remaining
hardcoded business/operational constant outside the intelligence/digest engines.
It follows the same operational-vs-business boundary (SST for the operational
default, as with every floor/cap/window in the sweep).

## Cross-References

- Evaluator: `internal/recommendation/watch/evaluator.go`
- SST loader: `internal/config/recommendations.go`
- Wiring: `cmd/core/wiring_recommendation_watches.go`
- Sweep origin: `../../021-intelligence-delivery/bugs/BUG-021-005-relationship-cooling-llm-driven/`
- Policy: `.github/instructions/smackerel-no-defaults.instructions.md`
