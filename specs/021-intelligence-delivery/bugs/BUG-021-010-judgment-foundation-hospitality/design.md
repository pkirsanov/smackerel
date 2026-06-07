# Design: BUG-021-010

## Problem

The hospitality digest decided guest/property concern alerts with hardcoded SQL
thresholds, and each prior LLM-judgment evaluator re-implemented the same bridge
plumbing. The owner asked for the "best solution long term, but also short term
value." See `bug.md`.

## Architecture

### Long-term: the reusable judgment foundation

```
agent.InvokeJudgment[T](ctx, runner, source, scenarioID, signals) (T, error)
  → json.Marshal(signals)                 // json:"-" keeps internal keys out
  → runner.Invoke(IntentEnvelope{...})    // *agent.Bridge satisfies JudgmentRunner
  → validate: nil runner/result → ErrJudgmentUnavailable;
              non-OK / empty final / bad JSON → error
  → json.Unmarshal(res.Final) → T
```

One primitive, one validation path. Each evaluator keeps only its signal/decision
shapes + operational bounds. The four prior evaluators can migrate to it later
(behaviour-preserving; their tests guard it).

### Short-term: hospitality concern judgment on the foundation

```
daily digest → Generator.Generate
  → AssembleHospitalityContext(ctx, pool, eval, bounds)
      → arrivals / departures / tasks / revenue (unchanged)
      → assembleConcernAlerts:
          → gatherGuestSignals / gatherPropertySignals
            (candidates within operational caps — no threshold)
          → eval.EvaluateConcerns
              → BridgeHospitalityEvaluator → agent.InvokeJudgment
                  → hospitality_concern_evaluate scenario (LLM judges)
              ← HospitalityDecision{guest_alerts[], property_alerts[]}
          → map alerts back to GuestAlert/PropertyAlert by ref
      → nil eval ⇒ no concern alerts (no threshold fallback)
```

### Components

1. **`internal/agent/judgment.go`** — `InvokeJudgment[T]`, `JudgmentRunner`,
   `ErrJudgmentUnavailable`. Lives in `agent` because it depends only on
   `IntentEnvelope` / `InvocationResult` / `OutcomeOK`, and both `intelligence`
   and `digest` already import `agent` (no new coupling, no cycle).

2. **Scenario** `hospitality-concern-evaluate-v1.yaml` — one batched scenario
   for guests + properties; the LLM returns only the rows worth flagging. One
   no-op tool.

3. **`internal/digest/hospitality_eval.go`** — `GuestSignal` / `PropertySignal`
   (public signals + `ref`; internal Email via `json:"-"`),
   `ConcernJudgment` / `HospitalityDecision`, `HospitalityEvaluator` +
   `BridgeHospitalityEvaluator` (on `InvokeJudgment`), `HospitalityBounds`.
   `init()` registers `noop_hospitality_concern`.

4. **`hospitality.go` rework** — `gatherGuestSignals` / `gatherPropertySignals`
   replace the threshold queries; `assembleConcernAlerts` runs the LLM and maps
   results by `ref`; `AssembleHospitalityContext` takes the evaluator + bounds.

5. **`cmd/core/wiring_hospitality.go`** — `wireHospitalityEvaluator` builds
   `BridgeHospitalityEvaluator{Runner: bridge}` + bounds from
   `LoadHospitalityConfig()` and calls `SetHospitalityEvaluator`. Nil bridge ⇒
   no-op.

6. **`cmd/scenario-lint/main.go`** — blank import of `internal/digest` so
   `noop_hospitality_concern` is registered when the linter validates BS-010
   (the loader requires every allowed_tools entry to be in the registry).

7. **SST** — `digest.hospitality.{guest_candidate_limit, property_candidate_limit}`
   in smackerel.yaml + config.sh + `internal/config/hospitality.go`.

## Operational vs business boundary

| Concern | Owner | Why |
|---|---|---|
| "Is this guest/property a concern worth flagging?" | LLM (scenario) | Domain reasoning; situational |
| Per-row signals (counts, sentiment, rating) | Go | Inputs, not thresholds |
| `guest_candidate_limit` / `property_candidate_limit` | SST | Per-digest candidate caps |

No concern threshold remains in Go/SQL. The operational bounds are SST and fail
loud (constitution C8 / NO-DEFAULTS).

## Test Strategy

- **Foundation** (`agent/judgment_test.go`): routing, source, signal forwarding,
  `json:"-"` non-leak, nil-runner sentinel, all error paths.
- **Evaluator** (`digest/hospitality_eval_test.go`): batch parse, routing,
  guest/property signal forwarding, internal-Email non-leak, empty-input
  short-circuit, all error paths.
- **SST loader** (`config/hospitality_test.go`): populate + fail-loud + range.
- The candidate DB queries are covered by the live-stack integration tier; the
  existing 16 hospitality unit tests (IsEmpty, fallback formatting, struct) stay
  green.

## Blast Radius

- New: `agent/judgment.go` (+ test), scenario YAML, `digest/hospitality_eval.go`
  (+ test), `config/hospitality.go` (+ test), `cmd/core/wiring_hospitality.go`.
- Modified: `digest/hospitality.go` (queries reworked; AssembleHospitalityContext
  signature += eval, bounds), `digest/generator.go` (eval field + setter; call
  site), `cmd/core/main.go` (wiring call), `cmd/scenario-lint/main.go` (blank
  import), `config/smackerel.yaml`, `scripts/commands/config.sh`.
- No schema migration. `AssembleHospitalityContext` is exported but called only
  from `generator.go` (confirmed repo-wide); the signature change is contained.

## Alternatives Considered

- **Per-row LLM calls.** Rejected: the digest runs daily over capped candidate
  sets; one batched call is responsive and lets the LLM weigh the host's guests
  comparatively.
- **A digest-local copy of the bridge plumbing.** Rejected: that is the exact
  duplication this bug removes; `InvokeJudgment` is the convergence point.
- **Migrate the existing four evaluators in this packet.** Deferred: a
  behaviour-preserving refactor of certified, working code is out of scope here;
  it is a clean follow-up guarded by the existing unit tests.
- **Keep `repeat_guest` / `high_issue_count` as factual SQL alerts.** Rejected:
  both embed a cutoff (`>1`, `>=5`); folding their signals into the LLM input
  lets the model decide and removes all hardcoded hospitality thresholds.
