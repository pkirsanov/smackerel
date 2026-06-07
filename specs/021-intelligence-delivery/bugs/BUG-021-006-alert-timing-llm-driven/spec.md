# Spec: BUG-021-006 — bill/trip/return alert timing must be LLM-driven

## Expected Behavior

Whether NOW is a good time to alert about an upcoming bill, trip, or return
deadline MUST be decided by the LLM per situation — not by a hardcoded N-day
window. The Go core retrieves candidate events within an operational lookahead
horizon; the LLM judges each. Only OPERATIONAL bounds (horizon, throughput cap,
confidence safety gate) remain, SST-configured and fail-loud.

## Actual Behavior

`ProduceBillAlerts` used `daysUntilBilling > 3`; `ProduceTripPrepAlerts` and
`ProduceReturnWindowAlerts` used `INTERVAL '5 days'`. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (LLM judges):** all three producers route the alert-timing decision
   through the `alert_timing_evaluate` scenario via the agent bridge; no Go code
   contains a hardcoded alert-timing window.
2. **AC-2 (signals, not thresholds):** each producer retrieves candidates within
   the operational `lookahead_days` horizon and passes `{alert_kind, subject,
   days_until_event, detail}` to the LLM; the only numbers carried are
   operational ($lookahead, $cap).
3. **AC-3 (evaluator tested):** `BridgeAlertTimingEvaluator` is unit-tested with
   a scripted bridge runner — parses the decision, routes to the correct
   scenario, forwards public signals, never leaks the internal ArtifactID /
   AlertType / Priority, and errors on every failure path.
4. **AC-4 (no window fallback):** when the evaluator is not wired, all three
   producers SKIP (no hardcoded window runs).
5. **AC-5 (operational bounds as SST):** `intelligence.alert_timing.*` keys are
   fail-loud SST; missing/invalid values are rejected naming the key.

## Out of Scope

- The `commitment_overdue` and `meeting_brief` producers (event-driven, not
  threshold-judged here).
- Live-LLM behavioral validation (live-stack tier).
- The relationship-cooling producer (already converted in BUG-021-005).

## Cross-References

- Bug detail + the operational/business boundary: `bug.md`
- Sibling (cooling): `../BUG-021-005-relationship-cooling-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
