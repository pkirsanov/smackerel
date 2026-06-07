# Design: BUG-021-009

## Problem

`DetectSeasonalPatterns` decided seasonal significance with hardcoded ratio
thresholds (`< 0.7`, `> 1.5`) and a "≥5 captures = seasonal" claim, and its ML
`seasonal.analyze` enrichment was dead due to a contract mismatch. Per
docs/smackerel.md §3.6, that significance judgment must be LLM-driven. See
`bug.md`.

## Architecture (Option A — extend the existing ML/NATS path)

```
GenerateMonthlyReport
  → Engine.DetectSeasonalPatterns(ctx)
      → skip if seasonal config not wired (no hardcoded fallback)
      → data-sufficiency floor vs operational MinDataDays (SST)
      → gather signals (Go: this-month vs last-year volume; topic candidates
        above operational floor, capped) — NO significance decision
      → skip if no NATS client (ML unavailable; no ratio fallback)
      → NATS Request → smk.seasonal.analyze (ML sidecar)
          → handle_seasonal_analyze JUDGES significance (LLM)
          ← { observations: [{pattern, month, observation, actionable}] }
      → map observations → SeasonalPattern, capped at MaxObservations
```

### Why the ML path, not agent.Bridge

Seasonal already had an ML touchpoint (`seasonal.analyze`). BUG-021-005/006/007/008
used `agent.Bridge` because those producers had no existing ML path. Reusing the
existing `seasonal.analyze` path keeps the feature coherent and avoids a second,
redundant LLM integration for the same concern.

### Components

1. **`internal/intelligence/monthly.go`** — `SeasonalConfig` (operational bounds
   struct, no evaluator); `DetectSeasonalPatterns` reworked to gather signals and
   delegate significance to the ML path; the `0.7`/`1.5` ratio decision and the
   `topic_seasonal` "≥5" claim removed; results capped at `MaxObservations`. The
   caller-side `> 2` cap in `GenerateMonthlyReport` is removed (the cap now lives
   in the detector).

2. **`ml/app/intelligence.py`** — `handle_seasonal_analyze` reworked to a new
   coherent contract: input = raw signals `{current_month, data_days,
   this_month_count, last_year_same_month_count, topic_candidates:[{name,count}]}`;
   output = `{observations:[{pattern, month, observation, actionable}], success}`.
   The prompt instructs the LLM to judge significance per situation (no fixed
   percentage). No LLM configured ⇒ empty observations (no magic-number
   fallback).

3. **`cmd/core/wiring_cooling.go`** — `wireSeasonalConfig` builds
   `intelligence.SeasonalConfig` from `config.LoadSeasonalConfig()` and calls
   `engine.SetSeasonalConfig`. Unlike the bridge evaluators it needs no bridge
   (significance is judged in the ML sidecar over NATS).

4. **SST** — `intelligence.seasonal.{min_data_days, topic_min_captures,
   topic_candidate_limit, max_observations}` in smackerel.yaml + config.sh +
   `internal/config/seasonal.go` (fail-loud loader).

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "Is this YoY change / topic seasonally meaningful?" | LLM (ML `seasonal.analyze`) | Domain reasoning; situational |
| YoY counts, topic counts | Go | Inputs, not thresholds |
| `min_data_days` | SST | Data-sufficiency floor (6+ months) |
| `topic_min_captures` / `topic_candidate_limit` | SST | Candidate floor / cap |
| `max_observations` | SST | Observation cap |

No seasonal-significance threshold remains in Go. The operational bounds are SST
and fail loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **SST loader** (`config/seasonal_test.go`): populate + fail-loud + range.
- **ML handler** (`ml/tests/test_intelligence_handlers.py::TestSeasonalAnalyze`):
  no-LLM → empty observations; no-signals → empty; response shape has the
  `observations` key. (The live-LLM judgment is exercised by the live-stack
  tier.)
- **Go skip/no-fallback**: `TestDetectSeasonalPatterns_NilPool` (nil pool errors
  first); the new nil-config and nil-NATS skip paths degrade gracefully. The DB
  signal queries are covered by the live-stack integration tier.

## Blast Radius

- New: `internal/config/seasonal.go` (+ test).
- Modified: `internal/intelligence/monthly.go` (DetectSeasonalPatterns reworked
  + SeasonalConfig; caller cap removed), `internal/intelligence/engine.go`
  (seasonal field + setter), `ml/app/intelligence.py` (handler reworked),
  `ml/tests/test_intelligence_handlers.py` (tests reworked),
  `config/smackerel.yaml`, `scripts/commands/config.sh`,
  `cmd/core/wiring_cooling.go` (+ wiring fn), `cmd/core/main.go` (wiring call).
- No schema migration. The seasonal NATS subject/response names are unchanged
  (`seasonal.analyze` / `seasonal.analyzed`); only the payload shapes are made
  coherent on both ends.

## Alternatives Considered

- **A new `agent.Bridge` seasonal scenario.** Rejected: redundant with the
  existing `seasonal.analyze` ML path; Option A reuses it.
- **Keep the Go `0.7`/`1.5` as a fallback when the LLM is unavailable.**
  Rejected: the directive is "no const limits"; consistent with the prior four
  conversions, no-LLM ⇒ skip.
- **Keep `topic_seasonal` heuristic in Go.** Rejected: "≥5 captures this month"
  is not evidence of seasonality; the LLM judges topic candidates instead.
