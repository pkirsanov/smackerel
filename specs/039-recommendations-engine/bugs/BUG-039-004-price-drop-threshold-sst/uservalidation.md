# User Validation: BUG-039-004

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (autonomous open-items sweep)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — no hardcoded price-drop threshold literal remains in `internal/recommendation/watch/`.
- [x] AC-2 — the fallback is sourced from `recommendations.watches.default_price_drop_threshold_pct` (fail-loud SST, range (0,1]); missing/invalid rejected naming the key.
- [x] AC-3 — the resolution precedence (trigger → filter → SST default) is preserved and unit-tested via `resolvePriceDropThreshold`.
- [x] AC-4 — the evaluator uses the CONFIGURED default (proven by a passthrough test), not a baked-in `0.10`.
- [x] AC-5 — existing price-drop and watch tests stay green (none were coupled to the old literal).

## Notes

This completes the owner's "NO const limits" sweep: it is the one remaining
hardcoded operational constant outside the intelligence/digest engines that the
BUG-021-005..011 work converted.

Classification matters here: unlike the six intelligence/digest BUSINESS
judgments moved to the LLM, the price-drop threshold is a USER-config value
(`watch.Filters["threshold_pct"]`) with a trigger override; the `0.10` was only
the no-input OPERATIONAL fallback. Per the operational-vs-business boundary
applied across the entire sweep (business reasoning → LLM; operational limits →
fail-loud SST), the correct fix is a fail-loud SST default — not an LLM call.
The user/trigger threshold still takes precedence; only the missing-input default
moved out of Go.

No existing test was coupled to the old literal (the price-drop integration test
always supplies an explicit `threshold_pct: 0.15`), so this is a behaviour-
preserving fix for all current tests while removing the policy violation.
