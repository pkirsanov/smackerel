# User Validation: BUG-021-011

**Reported by:** Owner directive — "we need best solution long term" (completing the foundation from BUG-021-010)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — each `BridgeXEvaluator` method body is a single `agent.InvokeJudgment[T]` call (plus the nil-receiver guard; expertise unwraps `resp.Classifications`); no transport plumbing remains in the four methods.
- [x] AC-2 — each `BridgeXEvaluator.Runner` is `agent.JudgmentRunner`; the four private `xBridgeRunner` interfaces are removed.
- [x] AC-3 — the four `ErrXEvaluatorUnavailable` sentinels are removed; the evaluators yield `agent.ErrJudgmentUnavailable` on the nil-receiver / unavailable path.
- [x] AC-4 — every existing evaluator unit test passes (parse, routing, signal forwarding, internal-field non-leak, nil paths, error paths).
- [x] AC-5 — the unused `fmt` import is dropped from each evaluator file; build/vet/tests green; no dangling references remain.

## Notes

This completes the "best solution long term" the owner asked for: BUG-021-010
built the reusable `agent.InvokeJudgment[T]` primitive and proved it on a new
consumer (hospitality); this bug retrofits the four pre-existing consumers
(cooling, alert timing, resurface, expertise) onto it. The codebase now holds
exactly ONE judgment transport and zero duplicated plumbing.

It is a behaviour-preserving refactor — no scenario, config, wiring, or producer
change — guarded entirely by the existing evaluator unit tests, which assert
every transport path and pass unchanged in intent. Net ~135 fewer lines.

The six LLM-driven business judgments delivered across BUG-021-005..010
(cooling, alert timing, resurface, expertise, seasonal, hospitality) all now sit
on a uniform foundation: business reasoning → LLM via one primitive; operational
limits → fail-loud SST.
