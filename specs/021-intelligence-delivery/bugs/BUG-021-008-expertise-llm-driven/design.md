# Design: BUG-021-008

## Problem

`GenerateExpertiseMap` decided tier + growth with a hardcoded weighted score
(`computeDepthScore`) and numeric boundaries (`assignTier`, `computeTrajectory`).
Per docs/smackerel.md §3.6, that domain reasoning must be LLM-driven. See
`bug.md`.

## Architecture (reuses the BUG-021-007 pattern; BATCH variant)

```
GET /api/expertise → ExpertiseHandler
  → Engine.GenerateExpertiseMap(ctx)
      → fail loud if no evaluator wired (no hardcoded tier fallback)
      → data-maturity check vs operational maturity_days (SST)
      → topic-dimension query (Go: capped by operational max_topics — no decision)
      → gather []ExpertiseSignals (ref = position; TopicID internal, json:"-")
      → ONE batched call:
          → ExpertiseEvaluator.ClassifyExpertise(ctx, dataDays, signals)
              → BridgeExpertiseEvaluator → agent.Bridge.Invoke
                  → expertise_classify scenario (LLM judges every topic)
              ← []ExpertiseClassification{ref, tier, growth, confidence}
      → map classifications back to topics by ref
      → detectBlindSpots (operational gap-detection bounds, SST)
```

### Why batch, not per-candidate

Cooling / alert-timing / resurfacing evaluate small candidate sets one item at a
time. The expertise map is generated as a UNIT on an on-demand HTTP request over
up to `max_topics` topics; a per-topic loop would mean up to 100 sequential LLM
calls per request. A single batched call keeps the endpoint responsive AND lets
the model reason comparatively (relative expertise across the user's graph).

### Components

1. **Scenario** `expertise-classify-v1.yaml` — one batched scenario; input is an
   array of per-topic signals + `data_days`; output is an array of
   `{ref, tier, growth, confidence}`. One no-op tool.

2. **`internal/intelligence/expertise_eval.go`** — `ExpertiseSignals` (public
   signals + `ref`; internal `TopicID` via `json:"-"`), `ExpertiseClassification`,
   `ExpertiseEvaluator` + `BridgeExpertiseEvaluator` (mockable runner),
   `ExpertiseConfig` (evaluator + operational bounds), the internal
   `expertiseRequest`/`expertiseResponse` envelopes. `init()` registers the
   no-op tool.

3. **`expertise.go` rework** — `GenerateExpertiseMap` fails loud without an
   evaluator; reads `max_topics` (query LIMIT) and `maturity_days` (maturity
   flag) from SST; gathers signals; makes one batched call; maps results back by
   `ref` (guarding out-of-range refs). `computeDepthScore`, `assignTier`,
   `computeTrajectory`, and the `DepthScore` field are deleted.
   `detectBlindSpots` takes the operational gap bounds.

4. **`cmd/core/wiring_cooling.go`** — `wireExpertiseEvaluator` builds
   `BridgeExpertiseEvaluator{Runner: bridge}` + `ExpertiseConfig` from
   `LoadExpertiseConfig()` and calls `engine.SetExpertiseConfig`. Nil bridge ⇒
   no-op (endpoint fails loud when invoked).

5. **SST** — `intelligence.expertise.{max_topics, maturity_days,
   blind_spot_min_mentions, blind_spot_max_captures, blind_spot_limit}` in
   smackerel.yaml + config.sh + `internal/config/expertise.go` (fail-loud
   loader).

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "How expert is the user? Is the topic growing?" | LLM (scenario) | Domain reasoning; situational, comparative |
| Per-topic signals (counts, ratios) | Go | Inputs, not thresholds |
| `max_topics` | SST | Per-request throughput cap |
| `maturity_days` | SST | Data-sufficiency floor |
| `blind_spot_*` bounds | SST | Mechanical gap-detection bounds |

No expertise threshold or weighted formula remains in Go. The operational bounds
are SST and fail loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **Evaluator** (`expertise_eval_test.go`): scripted runner proves batch parse,
  scenario routing, public-signal forwarding, internal-TopicID non-leak, `ref`
  correlation, empty-input short-circuit, and all error paths.
- **SST loader** (`config/expertise_test.go`): populate + fail-loud + range.
- The topic-dimension + blind-spot DB queries are covered by the live-stack
  integration tier.
- The former `computeDepthScore`/`assignTier`/`computeTrajectory` lock tests are
  removed (they pinned the deleted magic numbers).

## Blast Radius

- New: scenario YAML, `expertise_eval.go` (+ test), `config/expertise.go`
  (+ test).
- Modified: `expertise.go` (GenerateExpertiseMap reworked; 3 functions +
  `DepthScore` field removed; `detectBlindSpots` parameterized), `engine.go`
  (expertise field + setter), `expertise_test.go` (lock tests removed),
  `config/smackerel.yaml`, `scripts/commands/config.sh`,
  `cmd/core/wiring_cooling.go` (+ wiring fn), `cmd/core/main.go` (wiring call).
- No schema migration. The `depth_score` JSON field is dropped from the API
  response (no consumer reads it; confirmed by repo-wide search).

## Alternatives Considered

- **Per-topic LLM call.** Rejected: up to 100 sequential calls on a synchronous
  endpoint; batch is responsive and enables comparative reasoning.
- **Keep `computeDepthScore` as an input signal to the LLM.** Rejected: it is an
  arbitrary weighted sum; sending the raw signals lets the LLM weigh them itself.
- **Confidence-floor gate (as in cooling/resurface).** Not applicable:
  classification is descriptive (every topic gets a tier), not a gated binary
  action; `confidence` is recorded for transparency only.
