# User Validation: BUG-021-010

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" + "we need best solution long term, but also short term value"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `agent.InvokeJudgment[T]` exists and centralizes the marshal/invoke/validate/decode contract; unit-tested (routing, signal forwarding, `json:"-"` non-leak, nil-runner sentinel, all error paths).
- [x] AC-2 — the guest/property concern decision routes through `hospitality_concern_evaluate`; no hardcoded sentiment/rating/issue-count alert threshold remains in Go/SQL.
- [x] AC-3 — the digest gathers per-row signals within operational caps and sends them in ONE batched call; the internal guest Email is never sent (`json:"-"`).
- [x] AC-4 — when the evaluator is not wired, the digest produces no concern alerts (no threshold fallback); arrivals/departures/revenue/tasks are unaffected.
- [x] AC-5 — `digest.hospitality.*` are fail-loud SST keys (missing/invalid rejected naming the key).

## Notes

This packet answers the owner's "best solution long term, but also short term
value" directly:

- **Long term:** `agent.InvokeJudgment[T]` is the reusable judgment primitive
  the four prior conversions each hand-rolled. New judgments route through it;
  the existing four can migrate later (behaviour-preserving, test-guarded).
- **Short term:** hospitality guest/property concern alerts are now LLM-judged —
  a real behaviour change extending LLM judgment into the `digest` package for
  the first time, with all hardcoded thresholds removed.

The operational/business boundary is honored: business reasoning → LLM;
operational limits (per-digest guest/property candidate caps) → fail-loud SST
config. A nil evaluator degrades gracefully (no concern alerts, no hardcoded
fallback), and the rest of the digest is untouched.
