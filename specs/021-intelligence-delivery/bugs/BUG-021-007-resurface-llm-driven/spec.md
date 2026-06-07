# Spec: BUG-021-007 — resurfacing worthiness must be LLM-driven

## Expected Behavior

Whether a dormant artifact is genuinely worth resurfacing to the user MUST be
decided by the LLM per situation — not by a hardcoded dormancy + relevance
window. The Go core retrieves dormant candidate artifacts and their signals
within an operational dormancy-retrieval floor; the LLM judges each. Only
OPERATIONAL bounds (dormancy floor, throughput cap, confidence safety gate)
remain, SST-configured and fail-loud. Serendipity (random rediscovery) is a
separate strategy and is intentionally not judged.

## Actual Behavior

`Engine.Resurface` Strategy 1 selected dormant artifacts with
`last_accessed < NOW() - INTERVAL '30 days'` AND `relevance_score > 0.3` and
surfaced them verbatim. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (LLM judges):** the dormancy strategy routes the worthiness decision
   through the `resurface_evaluate` scenario via the agent bridge; no Go code
   contains a hardcoded dormancy or relevance worthiness threshold.
2. **AC-2 (signals, not thresholds):** the strategy retrieves candidates within
   the operational `min_dormancy_days` floor and passes `{title, days_dormant,
   relevance_score, access_count}` to the LLM; the only numbers carried are
   operational ($min_dormancy_days, $max_candidates).
3. **AC-3 (evaluator tested):** `BridgeResurfaceEvaluator` is unit-tested with a
   scripted bridge runner — parses the decision, routes to the correct
   scenario, forwards public signals, never leaks the internal ArtifactID, and
   errors on every failure path.
4. **AC-4 (no window fallback):** when the evaluator is not wired, the dormancy
   strategy SKIPS (no hardcoded window runs); serendipity still fills the
   digest.
5. **AC-5 (operational bounds as SST):** `intelligence.resurface.*` keys are
   fail-loud SST; missing/invalid values are rejected naming the key.

## Out of Scope

- The serendipity strategy (intentionally non-deterministic random
  rediscovery, not a worthiness threshold).
- Live-LLM behavioral validation (live-stack tier).
- The cooling and alert-timing producers (already converted in BUG-021-005 /
  BUG-021-006).

## Cross-References

- Bug detail + the operational/business boundary: `bug.md`
- Sibling (alert-timing): `../BUG-021-006-alert-timing-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
