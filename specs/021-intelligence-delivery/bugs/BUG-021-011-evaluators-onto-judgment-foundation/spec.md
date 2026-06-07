# Spec: BUG-021-011 — migrate the four prior evaluators onto agent.InvokeJudgment

## Expected Behavior

The four pre-existing LLM-judgment evaluators (cooling, alert timing, resurface,
expertise) MUST route their bridge transport through the single reusable
`agent.InvokeJudgment[T]` primitive, with no per-evaluator copy of the
marshal/invoke/validate/decode plumbing, no private runner interface, and no
redundant per-evaluator "unavailable" sentinel. Runtime behaviour MUST be
identical (a behaviour-preserving refactor).

## Actual Behavior

Each of the four evaluators carried its own `xBridgeRunner` interface, its own
`ErrXEvaluatorUnavailable` sentinel, and its own ~30-line transport body — four
copies of the same logic. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (single transport):** each `BridgeXEvaluator` method body is a single
   `agent.InvokeJudgment[T]` call (plus the nil-receiver guard, and expertise's
   batch unwrap); no `json.Marshal`/`Invoke`/`Unmarshal` plumbing remains in the
   four evaluator methods.
2. **AC-2 (shared runner type):** each `BridgeXEvaluator.Runner` is
   `agent.JudgmentRunner`; the four private `xBridgeRunner` interfaces are
   removed.
3. **AC-3 (single sentinel):** the four `ErrXEvaluatorUnavailable` vars are
   removed; the evaluators yield `agent.ErrJudgmentUnavailable` on the
   nil-receiver / unavailable path.
4. **AC-4 (behaviour preserved):** every existing evaluator unit test passes —
   parse, scenario routing, signal forwarding, internal-field non-leak,
   nil-receiver/nil-runner (now asserting `agent.ErrJudgmentUnavailable`), and
   all error paths.
5. **AC-5 (clean build):** the now-unused `fmt` import is dropped from each
   evaluator file; `go build ./...`, `go vet`, and the affected packages are
   green; no dangling references to the removed identifiers remain.

## Out of Scope

- Any change to scenario YAML, SST config, wiring, or producer logic (transport
  refactor only).
- Live-LLM behavioral validation (live-stack tier).
- The hospitality evaluator (already on the foundation via BUG-021-010).

## Cross-References

- Bug detail: `bug.md`
- Foundation: `internal/agent/judgment.go`
- Origin: `../BUG-021-010-judgment-foundation-hospitality/`
