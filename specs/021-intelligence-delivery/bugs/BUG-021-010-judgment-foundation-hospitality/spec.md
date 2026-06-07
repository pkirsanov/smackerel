# Spec: BUG-021-010 — hospitality alerts LLM-driven on a reusable judgment foundation

## Expected Behavior

Whether a guest or property is a concern worth flagging to the host MUST be
decided by the LLM per situation — not by a hardcoded sentiment/rating/issue
threshold. The Go core gathers candidate signals within operational caps and a
single batched LLM call judges which warrant a host alert. The repeated
`marshal → invoke → validate → decode` plumbing is provided once by a reusable
`agent.InvokeJudgment[T]` primitive. Only OPERATIONAL bounds (candidate caps)
remain, SST-configured and fail-loud.

## Actual Behavior

`queryGuestAlerts` / `queryPropertyAlerts` decided alerts in SQL with
`sentiment_score < 0.3`, `avg_rating < 3.5`, `issue_count >= 5`, `total_stays > 1`.
Each prior LLM-judgment evaluator re-implemented its own bridge plumbing. See
`bug.md`.

## Acceptance Criteria

1. **AC-1 (reusable foundation):** `agent.InvokeJudgment[T]` exists and
   centralizes the marshal/invoke/validate/decode contract; it is unit-tested
   (routing, signal forwarding, `json:"-"` non-leak, nil-runner sentinel, all
   error paths).
2. **AC-2 (LLM judges hospitality):** the guest/property concern decision routes
   through the `hospitality_concern_evaluate` scenario via the bridge; no Go/SQL
   code contains a hardcoded sentiment/rating/issue-count alert threshold.
3. **AC-3 (signals, not thresholds):** the digest gathers `{ref, name,
   total_stays, sentiment_score, total_spend}` per guest and `{ref, name,
   issue_count, avg_rating}` per property within operational caps and sends them
   to the LLM in ONE batched call; the internal guest Email is never sent
   (`json:"-"`).
4. **AC-4 (no threshold fallback):** when the evaluator is not wired, the digest
   produces no concern alerts (no hardcoded threshold runs); arrivals,
   departures, revenue, and tasks are unaffected.
5. **AC-5 (operational bounds as SST):** `digest.hospitality.*` keys are
   fail-loud SST; missing/invalid values are rejected naming the key.

## Out of Scope

- Migrating the existing four `agent.Bridge` evaluators (cooling, alert-timing,
  resurface, expertise) onto `InvokeJudgment` — a behaviour-preserving follow-up.
- Live-LLM behavioral validation (live-stack tier).
- The arrivals/departures/revenue/tasks digest sections (unchanged).

## Cross-References

- Bug detail + the operational/business boundary: `bug.md`
- Foundation: `internal/agent/judgment.go`
- Sibling (seasonal): `../BUG-021-009-seasonal-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
