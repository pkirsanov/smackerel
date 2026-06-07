# User Validation: BUG-021-007

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — the dormancy strategy routes the worthiness decision through the `resurface_evaluate` LLM scenario; no hardcoded dormancy/relevance worthiness threshold remains in Go.
- [x] AC-2 — the strategy retrieves candidates within the operational `min_dormancy_days` floor and passes `{title, days_dormant, relevance_score, access_count}` to the LLM.
- [x] AC-3 — `BridgeResurfaceEvaluator` is unit-tested (parse, routing, signal forwarding, internal-field non-leak, all error paths).
- [x] AC-4 — when the evaluator is not wired, the dormancy strategy skips (no window fallback); serendipity still fills the digest.
- [x] AC-5 — `intelligence.resurface.*` are fail-loud SST keys (missing/invalid rejected naming the key).

## Notes

Continues the directive from BUG-021-005 / BUG-021-006: the "worth resurfacing?"
JUDGMENT for dormant artifacts is now the LLM's, decided per situation (a
reference returned to many times before it went quiet deserves resurfacing more
than something opened once and abandoned; a `0.30` relevance is not
categorically "not worth it"). The operational/business boundary is honored:
business reasoning → LLM; operational limits (dormancy-retrieval floor,
candidate cap, confidence safety gate) → fail-loud SST config.

The serendipity strategy (random rediscovery from underexplored topics) is
intentionally non-deterministic and carries no worthiness threshold, so it was
deliberately left un-judged.
