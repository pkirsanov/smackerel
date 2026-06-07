# Design: BUG-021-006

## Problem

The bill / trip-prep / return-window producers decided "alert now?" with
hardcoded N-day windows (`> 3`, `INTERVAL '5 days'`). Per docs/smackerel.md
§3.6, that domain reasoning must be LLM-driven. See `bug.md`.

## Architecture (reuses the BUG-021-005 cooling pattern)

```
scheduler jobs (bill / trip / return)
  → Engine.Produce{Bill,TripPrep,ReturnWindow}Alerts
      → candidate query (Go: retrieve events within $lookahead_days, no decision)
      → for each candidate:
          → AlertTimingEvaluator.EvaluateAlertTiming
              → BridgeAlertTimingEvaluator → agent.Bridge.Invoke
                  → alert_timing_evaluate scenario (LLM judges)
              ← AlertTimingDecision{should_alert, confidence, rationale}
          → evaluateAndCreateTimedAlert: gate on confidence_floor, CreateAlert
```

### Components

1. **Scenario** `alert-timing-evaluate-v1.yaml` — one scenario for all three
   kinds; input carries `alert_kind` so the LLM applies kind-appropriate
   reasoning; output `{should_alert, confidence, rationale}`. One no-op tool.

2. **`internal/intelligence/alert_timing.go`** — `AlertKind`,
   `AlertTimingCandidate` (public signals + internal-only ArtifactID/AlertType/
   Priority via `json:"-"`), `AlertTimingDecision`, `AlertTimingEvaluator` +
   `BridgeAlertTimingEvaluator` (mockable runner), `AlertTimingConfig`
   (evaluator + operational bounds), the pure `alertTimingShouldSurface` gate,
   and the shared `evaluateAndCreateTimedAlert` helper (evaluate → gate →
   CreateAlert with the LLM rationale as body). `init()` registers the no-op
   tool.

3. **Producer rework** — each producer:
   - widens its SQL to retrieve candidates within the operational
     `$lookahead_days` horizon (replacing the hardcoded window), parameterized
     + capped by `$max_candidates`;
   - collects candidates (closing the row cursor before any LLM call);
   - composes a per-kind `detail` string (e.g. "annual USD 120.00 subscription",
     "trip to Tokyo", "return deadline for ...");
   - calls `evaluateAndCreateTimedAlert`.
   Graceful skip when the evaluator is nil.

4. **`cmd/core/wiring_cooling.go`** — `wireAlertTimingEvaluator` builds
   `BridgeAlertTimingEvaluator{Runner: bridge}` + `AlertTimingConfig` from
   `LoadAlertTimingConfig()` and calls `engine.SetAlertTimingConfig`. Nil bridge
   ⇒ no-op.

5. **SST** — `intelligence.alert_timing.{lookahead_days, max_candidates,
   confidence_floor}` in smackerel.yaml + config.sh +
   `internal/config/alert_timing.go` (fail-loud loader).

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "Alert about this event now?" | LLM (scenario) | Domain reasoning; situational |
| `days_until_event` (calendar arithmetic) | Go | A signal, not a threshold |
| `lookahead_days` | SST | Candidate-retrieval horizon |
| `max_candidates` | SST | Throughput cap |
| `confidence_floor` | SST | Decision-confidence safety gate |

No alert-timing threshold remains in Go. The operational bounds are SST and
fail loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **Evaluator** (`alert_timing_test.go`): scripted runner proves parse, scenario
  routing, public-signal forwarding, internal-field non-leak, and all error
  paths.
- **Pure helper**: `alertTimingShouldSurface`.
- **SST loader** (`config/alert_timing_test.go`): populate + fail-loud + range.
- Producer DB queries are covered by the live-stack integration tier.

## Blast Radius

- New: scenario YAML, `alert_timing.go` (+ test), `config/alert_timing.go`
  (+ test).
- Modified: `alert_producers.go` (3 producers reworked), `engine.go`
  (alertTiming field + setter), `config/smackerel.yaml`, `scripts/commands/config.sh`,
  `cmd/core/wiring_cooling.go` (+ wiring fn), `cmd/core/main.go` (wiring call).
- No schema migration. No change to alert delivery or the weekly/daily schedule.

## Alternatives Considered

- **Three separate scenarios (one per kind).** Rejected: it is one decision
  type ("alert now?"); `alert_kind` in the input lets one scenario reason
  per-kind, less duplication.
- **Keep the lookahead as a hardcoded constant.** Rejected: it is operational,
  so it belongs in SST (and the directive is "no const limits").
- **Per-candidate vs batched LLM call.** Per-candidate mirrors BUG-021-005 and
  the small candidate sets; batching is a future optimization.
