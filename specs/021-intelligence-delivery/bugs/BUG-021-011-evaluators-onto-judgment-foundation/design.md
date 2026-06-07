# Design: BUG-021-011

## Problem

The four pre-existing LLM-judgment evaluators each carried a private runner
interface, a redundant "unavailable" sentinel, and a hand-rolled copy of the
marshal/invoke/validate/decode transport that BUG-021-010 centralized into
`agent.InvokeJudgment[T]`. See `bug.md`.

## Architecture

### Before (per evaluator, ×4)

```
type xBridgeRunner interface { Invoke(ctx, env) (*InvocationResult, *RoutingDecision) }
type BridgeXEvaluator struct { Runner xBridgeRunner }
var ErrXEvaluatorUnavailable = errors.New("...")

func (b *BridgeXEvaluator) EvaluateX(ctx, in) (Decision, error) {
    if b == nil || b.Runner == nil { return zero, ErrXEvaluatorUnavailable }
    structured, err := json.Marshal(in); ...           // ~30 lines of
    res, _ := b.Runner.Invoke(ctx, IntentEnvelope{...}) //   identical
    if res == nil { ... }; if res.Outcome != OK { ... } //   plumbing,
    if len(res.Final) == 0 { ... }                      //   four copies
    var d Decision; json.Unmarshal(res.Final, &d); ...
    return d, nil
}
```

### After (per evaluator, ×4)

```
type BridgeXEvaluator struct { Runner agent.JudgmentRunner }

func (b *BridgeXEvaluator) EvaluateX(ctx, in) (Decision, error) {
    if b == nil { return zero, agent.ErrJudgmentUnavailable }
    return agent.InvokeJudgment[Decision](ctx, b.Runner, source, scenarioID, in)
}
```

`InvokeJudgment` already performs the identical validation sequence, so the four
bodies collapse to one call each.

### Per-evaluator specifics

| Evaluator | Source | Scenario ID | Return shape |
|---|---|---|---|
| cooling | `scheduler` | `relationship_cooling_evaluate` | `CoolingDecision` |
| alert timing | `scheduler` | `alert_timing_evaluate` | `AlertTimingDecision` |
| resurface | `scheduler` | `resurface_evaluate` | `ResurfaceDecision` |
| expertise | `api` | `expertise_classify` | `expertiseResponse` → `.Classifications` |

Expertise is the only batch evaluator: it keeps its `expertiseRequest` /
`expertiseResponse` envelopes and its `len(topics) == 0` short-circuit, then
calls `InvokeJudgment[expertiseResponse]` and returns `resp.Classifications`.

### Nil-receiver guard

The `if b == nil` guard stays in each method: a method invoked on a nil
`*BridgeXEvaluator` must not dereference `b.Runner`. The nil-*Runner* case is
handled inside `InvokeJudgment` (returns `agent.ErrJudgmentUnavailable`), so both
paths yield the same sentinel.

### Imports

Each evaluator file drops `fmt` (its only uses were the removed `fmt.Errorf`
calls). `encoding/json` and `errors` remain — each file's `init()` registers a
no-op tool using `json.RawMessage` schemas and an `errors.New` handler.

## Test Strategy

No new tests. The existing evaluator unit tests are the regression guard — they
already assert every transport path:

- parse + scenario routing + signal forwarding + internal-field non-leak;
- nil-receiver / nil-runner → now `agent.ErrJudgmentUnavailable` (assertions
  updated);
- non-OK outcome / empty final / bad JSON → error.

All pass unchanged in intent. The `cmd/core` build (production wiring) and
`cmd/scenario-lint` continue to compile and pass.

## Blast Radius

- Modified: `internal/intelligence/{cooling,alert_timing,resurface_eval,expertise_eval}.go`
  (transport bodies, runner field types, sentinel removal, `fmt` import) and
  their four `_test.go` files (nil-receiver assertions + one stale comment).
- No scenario YAML, SST, wiring, producer, or schema change. No migration.
- Net 135 fewer lines (179 deletions, 44 insertions).

## Alternatives Considered

- **Alias the sentinels** (`var ErrXEvaluatorUnavailable = agent.ErrJudgmentUnavailable`)
  to avoid touching the tests. Rejected: it preserves four redundant names for
  one error — the opposite of the DRY goal — and diverges from the hospitality
  evaluator, which already uses `agent.ErrJudgmentUnavailable` directly.
- **Migrate in four separate packets.** Rejected: the change is mechanical and
  identical across the four; one packet keeps the convergence atomic and the
  evidence in one place.
- **Leave the four as-is.** Rejected: the owner asked for the best long-term
  solution; leaving four copies of the transport is the duplication the
  foundation exists to remove.
