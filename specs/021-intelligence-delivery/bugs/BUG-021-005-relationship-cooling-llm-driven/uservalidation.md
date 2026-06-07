# User Validation: BUG-021-005

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — the cooling decision flows through the `relationship_cooling_evaluate` LLM scenario; no hardcoded "cooling" threshold remains in Go.
- [x] AC-2 — the producer retrieves candidate signals (no business threshold) and orders most-dormant-first; only operational params ($dedup_window, $candidate_cap) are carried.
- [x] AC-3 — `BridgeCoolingEvaluator` is unit-tested (parse, scenario routing, signal forwarding, PersonID non-leak, all error paths).
- [x] AC-4 — when the evaluator is not wired, cooling production is skipped (no magic-number fallback).
- [x] AC-5 — `intelligence.relationship_cooling.*` are fail-loud SST keys (missing/invalid rejected naming the key).
- [x] AC-6 — BUG-021-004's constants, query builder, and lock test are removed.

## Notes

This directly implements the owner's directive: the cooling JUDGMENT is now the
LLM's, decided per situation against each contact's own cadence — not a fixed
threshold. The boundary the product docs draw (docs/smackerel.md §3.6 +
constitution C8) is honored: business reasoning → LLM; operational limits
(throughput cap, confidence safety gate, anti-spam window) → fail-loud SST
config. Supersedes BUG-021-004, which had cemented the magic numbers.

Other hardcoded business thresholds in the digest/recommendation layers
(resurface dormancy/relevance, hospitality sentiment/rating, expense windows,
price-drop %) are the same class of debt and are flagged for follow-up.
