# Design: BUG-021-007

## Problem

`Engine.Resurface` Strategy 1 decided "worth resurfacing?" with a hardcoded
dormancy + relevance window (`INTERVAL '30 days'`, `relevance_score > 0.3`).
Per docs/smackerel.md §3.6, that domain reasoning must be LLM-driven. See
`bug.md`.

## Architecture (reuses the BUG-021-006 alert-timing pattern)

```
scheduler daily 8AM digest job / weekly synthesis
  → Engine.Resurface(ctx, limit)
      → Strategy 1 (dormancy):
          → gatherResurfaceCandidates (Go: retrieve dormant artifacts within
            $min_dormancy_days floor, capped by $max_candidates — no decision)
          → for each candidate:
              → ResurfaceEvaluator.EvaluateResurface
                  → BridgeResurfaceEvaluator → agent.Bridge.Invoke
                      → resurface_evaluate scenario (LLM judges)
                  ← ResurfaceDecision{worth_resurfacing, confidence, reason}
              → resurfaceShouldSurface: gate on confidence_floor → candidate
      → Strategy 2 (serendipity): unchanged — random rediscovery, not judged
```

### Components

1. **Scenario** `resurface-evaluate-v1.yaml` — one scenario for the dormancy
   decision; input carries the dormant artifact's signals; output
   `{worth_resurfacing, confidence, reason}`. One no-op tool.

2. **`internal/intelligence/resurface_eval.go`** — `ResurfaceSignals` (public
   signals + internal-only ArtifactID via `json:"-"`), `ResurfaceDecision`,
   `ResurfaceEvaluator` + `BridgeResurfaceEvaluator` (mockable runner),
   `ResurfaceConfig` (evaluator + operational bounds), and the pure
   `resurfaceShouldSurface` gate. `init()` registers the no-op tool.

3. **Strategy 1 rework** (`resurface.go`):
   - new `gatherResurfaceCandidates` helper retrieves dormant artifacts within
     the operational `$min_dormancy_days` floor
     (`COALESCE(last_accessed, created_at) < NOW() - make_interval(days => $1)`),
     ordered oldest-first, capped by `$max_candidates` — replacing the hardcoded
     window; it returns `ResurfaceSignals`, not decisions;
   - `Resurface` loops the candidates (honoring `limit` and ctx cancellation),
     calls `EvaluateResurface` per candidate, gates via `resurfaceShouldSurface`,
     and builds `ResurfaceCandidate` with the LLM's reason (falling back to a
     neutral templated string only for the human-facing copy, never the
     decision);
   - graceful skip + `slog.Warn` when the evaluator is nil.

4. **`cmd/core/wiring_cooling.go`** — `wireResurfaceEvaluator` builds
   `BridgeResurfaceEvaluator{Runner: bridge}` + `ResurfaceConfig` from
   `LoadResurfaceConfig()` and calls `engine.SetResurfaceConfig`. Nil bridge ⇒
   no-op (dormancy disabled, serendipity unaffected).

5. **SST** — `intelligence.resurface.{min_dormancy_days, max_candidates,
   confidence_floor}` in smackerel.yaml + config.sh +
   `internal/config/resurface.go` (fail-loud loader).

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "Is this dormant artifact worth resurfacing?" | LLM (scenario) | Domain reasoning; situational |
| `days_dormant` (calendar arithmetic) | Go | A signal, not a threshold |
| `min_dormancy_days` | SST | Candidate-retrieval floor (exclude fresh items) |
| `max_candidates` | SST | Throughput cap |
| `confidence_floor` | SST | Decision-confidence safety gate |

No worthiness threshold remains in Go. The operational bounds are SST and fail
loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **Evaluator** (`resurface_eval_test.go`): scripted runner proves parse,
  scenario routing, public-signal forwarding, internal-field (ArtifactID)
  non-leak, and all error paths.
- **Pure helper**: `resurfaceShouldSurface`.
- **SST loader** (`config/resurface_test.go`): populate + fail-loud + range.
- The dormancy DB query (`gatherResurfaceCandidates`) is covered by the
  live-stack integration tier.

## Blast Radius

- New: scenario YAML, `resurface_eval.go` (+ test), `config/resurface.go`
  (+ test).
- Modified: `resurface.go` (Strategy 1 reworked + `gatherResurfaceCandidates`
  helper), `engine.go` (resurface field + setter), `config/smackerel.yaml`,
  `scripts/commands/config.sh`, `cmd/core/wiring_cooling.go` (+ wiring fn),
  `cmd/core/main.go` (wiring call).
- No schema migration. No change to serendipity, to `MarkResurfaced`, or to the
  weekly/daily schedule.

## Alternatives Considered

- **Also LLM-judge serendipity.** Rejected: serendipity is deliberate random
  rediscovery (its value IS the randomness); judging it would defeat its
  purpose. It carries no worthiness threshold to remove.
- **Keep the dormancy floor as a hardcoded constant.** Rejected: it is
  operational, so it belongs in SST (and the directive is "no const limits").
- **Per-candidate vs batched LLM call.** Per-candidate mirrors BUG-021-006 and
  the small candidate sets (daily limit 5, weekly limit 1); batching is a future
  optimization.
