# BUG-021-008: expertise tier & growth must be LLM-driven, not hardcoded score/threshold heuristics

**Status:** Resolved (LLM-driven expertise classification via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (continuation of BUG-021-007)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/expertise.go`, `internal/intelligence/expertise_eval.go` (new), `config/prompt_contracts/expertise-classify-v1.yaml` (new)

## Summary

After BUG-021-007 made resurfacing worthiness LLM-driven, the expertise map
(`GET /api/expertise`, R-501) still decided "how expert is the user in this
topic, and is the topic growing?" with hardcoded heuristics:

- **`computeDepthScore`**: a fixed weighted-sum
  `capture*0.3 + sources*15 + depth_ratio*20 + engagement*0.1 + density*10`.
- **`assignTier`**: numeric tier boundaries — expert `>100 captures && >90 score`,
  deep `>50 && >60`, intermediate `>20 && >30`, foundation `>5 && >10`.
- **`computeTrajectory`**: fixed velocity cutoffs — accelerating `>1.5`,
  steady `>=0.7`, decelerating `>=0.3`, else stopped.

These answer the same domain question the product architecture says must be
LLM-driven (docs/smackerel.md §3.6). Genuine expertise is not a fixed weighted
sum: 30 deeply-connected, frequently-revisited captures from many sources can
represent more real expertise than 200 shallow ones. A fixed formula and fixed
boundaries cannot weigh volume, diversity, depth, revisitation and
interconnection the way the situation demands.

## Mechanism (the old, hardcoded path)

`GenerateExpertiseMap` computed, per topic, `depthScore = computeDepthScore(te)`,
then `tier = assignTier(captureCount, depthScore)` and
`growth = computeTrajectory(recent30d, avgMonthly)`. The weighted sum and the
numeric boundaries were the entire expertise judgment. Lock tests
(`TestComputeDepthScore`, `TestAssignTier`, `TestComputeTrajectory`, and their
exact-boundary variants) cemented those magic numbers in place.

## Fix (delivered — LLM-driven, reuses the BUG-021-007 pattern, batch variant)

1. **New scenario** `config/prompt_contracts/expertise-classify-v1.yaml`
   (`expertise_classify`): input = `{data_days, topics:[{ref, topic_name,
   capture_count, source_diversity, depth_ratio, engagement,
   connection_density, recent_captures_30d, avg_monthly_captures}]}`; output =
   `{classifications:[{ref, tier, growth, confidence}]}`. The system prompt
   instructs the LLM to weigh the signals together and RELATIVE to the user's
   whole graph — no fixed weighted sum, no numeric tier/velocity threshold.
2. **New evaluator** `internal/intelligence/expertise_eval.go`:
   `ExpertiseEvaluator` interface + `BridgeExpertiseEvaluator` (mockable),
   `ExpertiseSignals` / `ExpertiseClassification`, the `ExpertiseConfig` bundle,
   and a `noop_expertise_classify` tool for the loader contract.
3. **`GenerateExpertiseMap` reworked**: it now gathers each topic's
   deterministic signals (capped by the operational `max_topics`) and makes ONE
   batched `expertise_classify` call so the LLM classifies every topic
   comparatively in a single request (this is an on-demand HTTP endpoint, so a
   per-topic loop would mean up to 100 sequential LLM calls). Classifications
   map back by `ref`. `computeDepthScore`, `assignTier`, `computeTrajectory` and
   the `DepthScore` field are deleted. When the evaluator is not wired, the
   endpoint **fails loud** — there is no hardcoded tier fallback.
4. **Operational bounds → SST** (fail-loud): `intelligence.expertise.{max_topics,
   maturity_days, blind_spot_min_mentions, blind_spot_max_captures,
   blind_spot_limit}` — a per-request topic cap, a data-sufficiency floor, and
   the blind-spot gap-detection bounds. None of these decide expertise; the LLM
   does.

## Operational vs business boundary

Per docs/smackerel.md §3.6 + constitution C8: **business reasoning → LLM**;
**operational limits → SST config (fail-loud)**. The tier/growth JUDGMENT is the
LLM's. The remaining numbers (topic cap, maturity floor, blind-spot gap bounds)
bound the job — they do not decide expertise.

## Relationship to BUG-021-005/006/007

Same directive, same pattern. Cooling, alert-timing, and resurfacing used a
per-candidate evaluator; expertise uses a BATCH evaluator (one call for the
whole topic set) because the expertise map is generated as a unit on demand and
the LLM benefits from reasoning across the user's topics comparatively.

## Note on superseded spec-006 tests

The expertise feature originated in `specs/006-phase5-advanced/`, whose
`scenario-manifest.json` / `report.md` reference `TestComputeDepthScore`,
`TestAssignTier`, and `TestComputeTrajectory`. Those lock tests are removed by
this bug (they pinned the magic numbers being deleted). Spec 006 is a certified
historical spec and is NOT edited; this packet records the supersession. The
LLM-driven decision contract is now covered by `expertise_eval_test.go`.

## Cross-References

- Scenario: `config/prompt_contracts/expertise-classify-v1.yaml`
- Evaluator: `internal/intelligence/expertise_eval.go`
- Map: `internal/intelligence/expertise.go`
- Wiring: `cmd/core/wiring_cooling.go` (`wireExpertiseEvaluator`)
- SST loader: `internal/config/expertise.go`
- Sibling (resurface): `../BUG-021-007-resurface-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
