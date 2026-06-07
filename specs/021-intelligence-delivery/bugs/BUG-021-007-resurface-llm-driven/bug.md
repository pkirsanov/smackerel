# BUG-021-007: resurfacing worthiness must be LLM-driven, not a hardcoded dormancy/relevance window

**Status:** Resolved (LLM-driven resurfacing-worthiness judgment via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (continuation of BUG-021-006)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/resurface.go` (dormancy strategy), `internal/intelligence/resurface_eval.go` (new), `config/prompt_contracts/resurface-evaluate-v1.yaml` (new)

## Summary

After BUG-021-006 made bill/trip/return alert timing LLM-driven, the
digest resurfacing pipeline still decided "is this dormant artifact worth
bringing back to the user?" with a hardcoded dormancy + relevance window:

- **Dormancy** (`Engine.Resurface`, Strategy 1): a single SQL query selected
  artifacts with `last_accessed < NOW() - INTERVAL '30 days'` AND
  `relevance_score > 0.3`, then surfaced them verbatim.

This answers the same domain question as cooling and alert-timing — and the
same one the product architecture says must be LLM-driven
(docs/smackerel.md §3.6). Whether a dormant item is genuinely worth resurfacing
depends entirely on the item: a reference the user returned to many times
before it went quiet deserves resurfacing far more than something opened once
and abandoned; relevance `0.31` is not categorically "worth it" while `0.29` is
"not". A fixed `30 days` / `> 0.3` cutoff cannot capture that judgment.

## Mechanism (the old, hardcoded path)

`Engine.Resurface` Strategy 1 used the hardcoded window as BOTH the candidate
filter and the worthiness decision: the SQL `WHERE last_accessed < NOW() -
INTERVAL '30 days' AND relevance_score > 0.3 ORDER BY relevance_score DESC`
fully decided which dormant artifacts were "worth" resurfacing. Anything at
`29 days` or `relevance 0.30` was silently excluded; anything past the cutoff
was surfaced with a templated reason, regardless of whether resurfacing it
actually served the user.

## Fix (delivered — LLM-driven, reuses the BUG-021-006 pattern)

1. **New scenario** `config/prompt_contracts/resurface-evaluate-v1.yaml`
   (`resurface_evaluate`): input = `{title, days_dormant, relevance_score,
   access_count}`; output = `{worth_resurfacing, confidence, reason}`. The
   system prompt instructs the LLM to judge per situation whether a dormant
   artifact is genuinely worth resurfacing now, with no fixed dormancy or
   relevance cutoff.
2. **New evaluator** `internal/intelligence/resurface_eval.go`:
   `ResurfaceEvaluator` interface + `BridgeResurfaceEvaluator` (mockable),
   `ResurfaceSignals` / `ResurfaceDecision`, the pure `resurfaceShouldSurface`
   gate, the `ResurfaceConfig` bundle, and a `noop_resurface_evaluate` tool for
   the loader contract.
3. **Strategy 1 reworked**: `Engine.Resurface` now retrieves dormant CANDIDATES
   via the new `gatherResurfaceCandidates` helper (within an OPERATIONAL
   dormancy-retrieval floor + cap), and lets the LLM decide per candidate
   whether the artifact is worth resurfacing; the Go side gates only on the
   operational confidence floor. When the evaluator is not wired, the dormancy
   strategy skips — there is **no hardcoded window fallback** — and serendipity
   still fills the digest.
4. **Operational bounds → SST** (fail-loud): `intelligence.resurface.{min_dormancy_days,
   max_candidates, confidence_floor}` — a dormancy-retrieval floor (exclude
   freshly-accessed items), a throughput cap, and a decision-confidence safety
   gate. None of these decide worthiness; the LLM does.

## Operational vs business boundary

Per docs/smackerel.md §3.6 + constitution C8: **business reasoning → LLM**;
**operational limits → SST config (fail-loud)**. The "worth resurfacing?"
JUDGMENT is the LLM's. The remaining numbers (dormancy-retrieval floor,
candidate cap, confidence floor) bound the job and gate model confidence — they
do not decide worthiness.

## Relationship to BUG-021-006 / BUG-021-005

Same directive, same pattern. BUG-021-005 converted the cooling producer;
BUG-021-006 converted the three timing-based alert producers; this converts the
dormancy worthiness decision in the resurfacing pipeline. Strategy 2
(serendipity) is intentionally non-deterministic random rediscovery — it is NOT
a worthiness threshold, so it is deliberately left un-judged.

## Cross-References

- Scenario: `config/prompt_contracts/resurface-evaluate-v1.yaml`
- Evaluator: `internal/intelligence/resurface_eval.go`
- Strategy: `internal/intelligence/resurface.go`
- Wiring: `cmd/core/wiring_cooling.go` (`wireResurfaceEvaluator`)
- SST loader: `internal/config/resurface.go`
- Sibling (alert-timing): `../BUG-021-006-alert-timing-llm-driven/`
- Sibling (cooling): `../BUG-021-005-relationship-cooling-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
