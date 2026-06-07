# User Validation: BUG-021-006

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — all three producers (bill, trip-prep, return-window) route the alert-timing decision through the `alert_timing_evaluate` LLM scenario; no hardcoded window remains in Go.
- [x] AC-2 — each producer retrieves candidates within the operational `lookahead_days` horizon and passes `{alert_kind, subject, days_until_event, detail}` to the LLM.
- [x] AC-3 — `BridgeAlertTimingEvaluator` is unit-tested (parse, routing, signal forwarding, internal-field non-leak, all error paths).
- [x] AC-4 — when the evaluator is not wired, all three producers skip (no window fallback).
- [x] AC-5 — `intelligence.alert_timing.*` are fail-loud SST keys (missing/invalid rejected naming the key).

## Notes

Continues the directive from BUG-021-005: the "alert now?" JUDGMENT for bills,
trips, and return deadlines is now the LLM's, decided per situation and per kind
(a large annual charge warrants earlier notice than a small monthly one; an
international trip needs more lead time than a local overnight). The
operational/business boundary is honored: business reasoning → LLM; operational
limits (lookahead horizon, candidate cap, confidence safety gate) → fail-loud
SST config.

The two remaining producers in alert_producers.go (`commitment_overdue`,
`meeting_brief`) are event-driven (not threshold-judged) and were left as-is.
