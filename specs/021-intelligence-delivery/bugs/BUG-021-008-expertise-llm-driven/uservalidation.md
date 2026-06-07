# User Validation: BUG-021-008

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `GenerateExpertiseMap` routes the tier + growth decision through the `expertise_classify` LLM scenario; no hardcoded depth-score formula or tier/velocity threshold remains in Go.
- [x] AC-2 — the map gathers per-topic signals and sends them (plus `data_days`) to the LLM in ONE batched call; the only Go-side numbers are operational ($max_topics, blind-spot bounds).
- [x] AC-3 — `BridgeExpertiseEvaluator` is unit-tested (batch parse, routing, signal forwarding, internal-TopicID non-leak, `ref` correlation, empty-input short-circuit, all error paths).
- [x] AC-4 — when the evaluator is not wired, the expertise endpoint fails loud (no hardcoded tier emitted).
- [x] AC-5 — `intelligence.expertise.*` are fail-loud SST keys (missing/invalid rejected naming the key).

## Notes

Continues the directive from BUG-021-005/006/007: the "how expert is the user,
and is the topic growing?" JUDGMENT is now the LLM's, decided per situation and
comparatively across the user's whole graph (30 deeply-connected, frequently
revisited captures from many sources can outrank 200 shallow ones — a fixed
weighted sum cannot capture that). The operational/business boundary is honored:
business reasoning → LLM; operational limits (topic cap, data-sufficiency floor,
blind-spot gap bounds) → fail-loud SST config.

This is a BATCH evaluator (one call for the whole topic set) rather than the
per-candidate evaluators used for cooling / alert-timing / resurfacing, because
the expertise map is generated as a unit on an on-demand HTTP request and the
model benefits from reasoning comparatively. The blind-spot detection remains a
mechanical gap metric; only its bounds moved to SST.
