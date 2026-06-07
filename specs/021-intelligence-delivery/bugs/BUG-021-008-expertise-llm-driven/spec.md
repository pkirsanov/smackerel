# Spec: BUG-021-008 — expertise tier & growth must be LLM-driven

## Expected Behavior

The expertise tier and growth trajectory for each topic MUST be decided by the
LLM per situation — not by a hardcoded weighted score or numeric tier/velocity
threshold. The Go core retrieves each topic's deterministic signals and makes a
single batched LLM classification call; the LLM assigns tier + growth per topic.
Only OPERATIONAL bounds (topic cap, data-sufficiency floor, blind-spot
gap-detection bounds) remain, SST-configured and fail-loud.

## Actual Behavior

`GenerateExpertiseMap` used `computeDepthScore` (a fixed weighted sum),
`assignTier` (numeric capture/score boundaries), and `computeTrajectory` (fixed
velocity cutoffs). See `bug.md`.

## Acceptance Criteria

1. **AC-1 (LLM judges):** `GenerateExpertiseMap` routes the tier + growth
   decision through the `expertise_classify` scenario via the agent bridge; no
   Go code contains a hardcoded depth-score formula or tier/velocity threshold.
2. **AC-2 (signals, not thresholds):** the map gathers `{ref, topic_name,
   capture_count, source_diversity, depth_ratio, engagement,
   connection_density, recent_captures_30d, avg_monthly_captures}` per topic and
   sends them (plus `data_days`) to the LLM in ONE batched call; the only
   numbers that bound the Go side are operational ($max_topics, blind-spot
   bounds).
3. **AC-3 (evaluator tested):** `BridgeExpertiseEvaluator` is unit-tested with a
   scripted bridge runner — parses the batch, routes to the correct scenario,
   forwards public signals, never leaks the internal TopicID, correlates by
   `ref`, and errors on every failure path.
4. **AC-4 (no tier fallback):** when the evaluator is not wired, the expertise
   endpoint fails loud (no hardcoded tier is emitted).
5. **AC-5 (operational bounds as SST):** `intelligence.expertise.*` keys are
   fail-loud SST; missing/invalid values are rejected naming the key.

## Out of Scope

- Live-LLM behavioral validation (live-stack tier).
- The cooling / alert-timing / resurfacing producers (already converted in
  BUG-021-005/006/007).
- Re-classifying the blind-spot detection itself as an LLM judgment (it remains
  a mechanical gap metric; only its bounds move to SST).

## Cross-References

- Bug detail + the operational/business boundary: `bug.md`
- Sibling (resurface): `../BUG-021-007-resurface-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
