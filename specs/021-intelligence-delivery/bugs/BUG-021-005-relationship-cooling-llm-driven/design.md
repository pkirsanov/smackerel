# Design: BUG-021-005

## Problem

`ProduceRelationshipCoolingAlerts` encoded the cooling judgment as hardcoded SQL
magic numbers; `BUG-021-004` cemented them as Go constants + a lock test. The
product architecture (docs/smackerel.md §3.6) requires domain reasoning to be
LLM-driven. See `bug.md`.

## Architecture (follows the annotation_classify precedent)

```
scheduler weekly job
  → Engine.ProduceRelationshipCoolingAlerts
      → candidateCoolingQuery (Go: retrieve candidates + signals, no thresholds)
      → for each candidate:
          → CoolingEvaluator.EvaluateCooling
              → BridgeCoolingEvaluator → agent.Bridge.Invoke
                  → relationship_cooling_evaluate scenario (LLM judges)
              ← CoolingDecision{is_cooling, confidence, rationale}
          → coolingShouldSurface(decision, confidence_floor)  [operational gate]
          → Engine.CreateAlert (rationale as body)
```

### Components

1. **Scenario** `relationship-cooling-evaluate-v1.yaml` — input_schema (name,
   days_since_last_interaction, total_interactions, relationship_span_days,
   typical_gap_days); output_schema (is_cooling, confidence, rationale);
   system prompt instructs per-situation judgment against the contact's own
   cadence. One no-op tool (`noop_relationship_cooling`) to satisfy the loader.

2. **`internal/intelligence/cooling.go`** — `CoolingCandidate`,
   `CoolingDecision`, `CoolingEvaluator` interface, `BridgeCoolingEvaluator`
   (mockable `coolingBridgeRunner`), `CoolingConfig` (evaluator + operational
   bounds), and pure helpers `coolingTypicalGapDays` / `coolingShouldSurface`.
   `init()` registers the no-op tool (the package is imported by cmd/core).

3. **Producer rework** — `candidateCoolingQuery` retrieves the most-dormant
   candidates with their signals (parameterized only by the operational
   `$dedup_window_days` and `$max_candidates`); the loop collects candidates
   (closing the row cursor before any LLM call), evaluates each, and gates on
   the operational confidence floor. Graceful skip when the evaluator is nil.

4. **`cmd/core/wiring_cooling.go`** — builds `BridgeCoolingEvaluator{Runner:
   bridge}` + `CoolingConfig` from `LoadRelationshipCoolingConfig()` and calls
   `engine.SetCoolingConfig`. Nil bridge ⇒ no-op (cooling disabled).

5. **SST** — `intelligence.relationship_cooling.{max_candidates,
   confidence_floor, dedup_window_days}` in smackerel.yaml + config.sh +
   `internal/config/relationship_cooling.go` (fail-loud loader).

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "Is this relationship cooling?" | LLM (scenario) | Domain reasoning; situational |
| `typical_gap_days` (span/(n-1)) | Go (arithmetic) | A signal, not a threshold |
| `max_candidates` | SST | Throughput cap |
| `confidence_floor` | SST | Decision-confidence safety gate |
| `dedup_window_days` | SST | Anti-spam re-alert window |

No business threshold remains in Go. The operational bounds are SST-configured
and fail loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **Evaluator** (`cooling_test.go`): scripted `coolingBridgeRunner` proves
  parse, scenario routing, signal forwarding, PersonID non-leak, and all error
  paths — the LLM-driven core, testable without a live LLM.
- **Pure helpers**: `coolingTypicalGapDays`, `coolingShouldSurface`.
- **SST loader** (`relationship_cooling_test.go`): populate + fail-loud +
  range-rejection.
- The producer's DB query is covered by the live-stack integration tier (no
  producer DB harness exists in-repo; the query is a straightforward candidate
  retrieval).

## Blast Radius

- New: scenario YAML, `cooling.go`, `cooling_test.go`,
  `relationship_cooling.go`, `relationship_cooling_test.go`,
  `wiring_cooling.go`.
- Modified: `alert_producers.go` (producer rework), `engine.go` (cooling field
  + setter), `alert_producers_test.go` (removed lock test), `config/smackerel.yaml`,
  `scripts/commands/config.sh`, `cmd/core/main.go` (wiring call).
- No schema migration. No change to the alert delivery path or schedule.

## Alternatives Considered

- **Keep BUG-021-004's constants (operator picks the value).** Rejected by
  owner directive + docs/smackerel.md §3.6 — domain reasoning must be LLM-driven.
- **Lower-level LLMDriver.Turn instead of the agent bridge.** Rejected: the
  agent bridge + scenario contract is the established pattern (annotation,
  recommendation watch); it gives schema validation + tracing for free.
- **One batched LLM call for all candidates.** Deferred: per-candidate mirrors
  the annotation precedent, is simpler to schema, and the weekly candidate set
  is small; batching is a future optimization.
