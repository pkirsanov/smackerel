# BUG-021-011: migrate the four prior LLM-judgment evaluators onto the agent.InvokeJudgment foundation

**Status:** Resolved (behaviour-preserving DRY refactor via bugfix-fastlane — see report.md)
**Severity:** Low (code-health / maintainability; zero behaviour change)
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "we need best solution long term" (completing the foundation introduced in BUG-021-010)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/{cooling,alert_timing,resurface_eval,expertise_eval}.go` (+ their tests)

## Summary

BUG-021-010 introduced `agent.InvokeJudgment[T]` — the single reusable
`marshal → invoke → validate → decode` transport for LLM-driven business
judgments — and made the hospitality evaluator its first consumer. The four
earlier evaluators (relationship cooling, alert timing, resurfacing worthiness,
expertise classification) still each carried their own hand-rolled copy of that
plumbing plus a redundant per-evaluator runner interface and "unavailable"
sentinel error.

This bug completes the convergence: all four migrate onto `InvokeJudgment`,
leaving exactly ONE judgment transport in the codebase and ZERO duplication.
It is a pure behaviour-preserving refactor — every existing evaluator unit test
(parse, scenario routing, signal forwarding, internal-field non-leak,
nil-receiver/nil-runner, all error paths) passes unchanged in intent.

## Mechanism (the duplication being removed)

Each evaluator declared:

- a private `xBridgeRunner interface { Invoke(...) }` — identical across all four;
- an `ErrXEvaluatorUnavailable` sentinel — identical purpose across all four;
- a ~30-line method body: `json.Marshal` → build `IntentEnvelope` → `Invoke` →
  nil-result check → outcome check → empty-final check → `json.Unmarshal` —
  identical shape across all four.

That is four copies of the same transport, each a place the same bug could be
introduced or fixed inconsistently.

## Fix (delivered — behaviour-preserving)

For each of the four evaluators:

1. **Runner field** retyped from the private `xBridgeRunner` to the shared
   `agent.JudgmentRunner` (which `*agent.Bridge` and the test scripted runners
   already satisfy — same `Invoke` signature).
2. **Sentinel removed**: `ErrXEvaluatorUnavailable` deleted; the nil-receiver
   guard and the unavailable path now both yield `agent.ErrJudgmentUnavailable`,
   consistent with the hospitality evaluator.
3. **Body replaced** with a single `agent.InvokeJudgment[T](ctx, b.Runner,
   source, scenarioID, signals)` call (expertise wraps it to return
   `resp.Classifications` from its batched `expertiseResponse`). The nil-receiver
   guard stays (a method on a nil `*BridgeXEvaluator` cannot dereference
   `b.Runner`).
4. **Imports**: the now-unused `fmt` import is dropped from each file (`json` /
   `errors` remain — used by each evaluator's `init()` no-op tool registration).
5. **Tests**: the four `nil-receiver`/`nil-runner` assertions switch from
   `ErrXEvaluatorUnavailable` to `agent.ErrJudgmentUnavailable`; one stale
   comment referencing the removed `coolingBridgeRunner` is corrected.

Net: **135 fewer lines** across the eight files (179 deletions, 44 insertions),
with identical runtime behaviour.

## Why this is safe

- The four `xBridgeRunner` interfaces and the `*agent.Bridge` runner have the
  same `Invoke` signature as `agent.JudgmentRunner`, so production wiring
  (`BridgeXEvaluator{Runner: bridge}`) and the test scripted runners compile and
  behave identically.
- `agent.InvokeJudgment` performs the exact same validation sequence
  (nil-runner/nil-result → `ErrJudgmentUnavailable`; non-OK outcome / empty final
  / bad JSON → error) the hand-rolled bodies did; the per-evaluator tests assert
  every one of those paths and pass.
- The sentinels were referenced only inside each evaluator file and its test (no
  producers, no wiring), so removing them is contained.

## Relationship to BUG-021-010

BUG-021-010 built the foundation and proved it on a new consumer (hospitality);
this bug retrofits the four pre-existing consumers onto it. Together they leave
the codebase with one judgment primitive and no duplicated transport.

## Cross-References

- Foundation: `internal/agent/judgment.go` (`InvokeJudgment`, `JudgmentRunner`, `ErrJudgmentUnavailable`)
- Migrated evaluators: `internal/intelligence/{cooling,alert_timing,resurface_eval,expertise_eval}.go`
- Origin: `../BUG-021-010-judgment-foundation-hospitality/`
- Architecture: `docs/smackerel.md` §3.6
