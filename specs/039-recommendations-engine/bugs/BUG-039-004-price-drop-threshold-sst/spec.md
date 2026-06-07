# Spec: BUG-039-004 — price-drop watch threshold fallback must be fail-loud SST

## Expected Behavior

The price-drop watch threshold MUST resolve with the precedence trigger payload
→ user watch filter → OPERATIONAL SST default, with NO hardcoded in-source
fallback. The SST default
(`RECOMMENDATIONS_WATCHES_DEFAULT_PRICE_DROP_THRESHOLD_PCT`) is required and
fails loud (fraction in (0,1]) when missing or invalid.

## Actual Behavior

`gatherPriceDropCandidates` used a hardcoded `thresholdPct = 0.10` as the
final fallback when neither the trigger nor the user's watch filter supplied
`threshold_pct`. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (no literal):** no hardcoded price-drop threshold literal remains in
   `internal/recommendation/watch/`.
2. **AC-2 (SST default):** the fallback is sourced from
   `recommendations.watches.default_price_drop_threshold_pct` (fail-loud SST,
   range (0,1]); missing/invalid values are rejected naming the key.
3. **AC-3 (precedence preserved):** the resolution order is unchanged — trigger
   `threshold_pct` → watch filter `threshold_pct` → SST default — and is
   unit-tested via `resolvePriceDropThreshold`.
4. **AC-4 (configured passthrough):** the evaluator uses the CONFIGURED default
   (proven by a test that a different default flows through), not a baked-in
   `0.10`.
5. **AC-5 (no regression):** existing price-drop and watch tests stay green
   (none were coupled to the old literal).

## Out of Scope

- Re-architecting the price-drop model into an LLM judgment (the threshold is a
  user-config concept; only the missing-input default moves to SST).
- The other watch kinds (place/product/deal/event/content) — unaffected.

## Cross-References

- Bug detail + the operational-vs-business classification: `bug.md`
- Policy: `.github/instructions/smackerel-no-defaults.instructions.md`
