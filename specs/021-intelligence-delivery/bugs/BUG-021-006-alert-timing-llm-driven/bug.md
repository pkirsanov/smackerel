# BUG-021-006: bill/trip/return alert timing must be LLM-driven, not hardcoded N-day windows

**Status:** Resolved (LLM-driven alert-timing judgment via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (continuation of BUG-021-005)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/alert_producers.go` (bill/trip/return producers), `internal/intelligence/alert_timing.go` (new), `config/prompt_contracts/alert-timing-evaluate-v1.yaml` (new)

## Summary

After BUG-021-005 made relationship-cooling LLM-driven, the three sibling alert
producers still decided "when should I alert the user?" with hardcoded N-day
windows:

- **Bill** (`ProduceBillAlerts`): `daysUntilBilling > 3` — alert only within 3 days.
- **Trip prep** (`ProduceTripPrepAlerts`): `start_date ... + INTERVAL '5 days'` — alert only within 5 days.
- **Return window** (`ProduceReturnWindowAlerts`): `return_deadline ... + INTERVAL '5 days'` — alert only within 5 days.

These answer the same domain question as cooling — and the same one the product
architecture says must be LLM-driven (docs/smackerel.md §3.6). Whether NOW is a
good time to remind depends entirely on the event: a large annual charge
deserves earlier notice than a small monthly one; an international multi-day
trip needs more lead time than a local overnight; a high-value return window is
more urgent than a trivial one. A fixed window cannot capture that.

## Mechanism (the old, hardcoded path)

Each producer used a hardcoded window as both the candidate FILTER and the
alert DECISION:
- bill: SQL fetched active subscriptions, Go computed `daysUntilBilling` and
  dropped anything `> 3` days out.
- trip / return: the SQL `BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'`
  was the entire timing decision.

## Fix (delivered — LLM-driven, reuses the BUG-021-005 pattern)

1. **New scenario** `config/prompt_contracts/alert-timing-evaluate-v1.yaml`
   (`alert_timing_evaluate`): input = `{alert_kind, subject, days_until_event,
   detail}`; output = `{should_alert, confidence, rationale}`. The system prompt
   instructs the LLM to judge alert timing per situation, per kind (bill / trip
   / return), with no fixed window.
2. **New evaluator** `internal/intelligence/alert_timing.go`:
   `AlertTimingEvaluator` interface + `BridgeAlertTimingEvaluator` (mockable),
   `AlertTimingCandidate` / `AlertTimingDecision`, the pure
   `alertTimingShouldSurface` gate, a shared `evaluateAndCreateTimedAlert`
   helper, and a `noop_alert_timing` tool for the loader contract.
3. **All three producers reworked**: each now retrieves candidates within an
   OPERATIONAL `lookahead_days` horizon (a generous candidate-retrieval window,
   not a decision), computes `days_until_event` and a `detail` string, and lets
   the LLM decide per candidate whether to alert NOW; the Go side gates only on
   the operational confidence floor. When the evaluator is not wired, the
   producers skip — there is **no hardcoded window fallback**.
4. **Operational bounds → SST** (fail-loud): `intelligence.alert_timing.{lookahead_days,
   max_candidates, confidence_floor}` — a candidate-retrieval horizon, a
   throughput cap, and a decision-confidence safety gate. None of these decide
   the alert timing; the LLM does.

## Operational vs business boundary

Per docs/smackerel.md §3.6 + constitution C8: **business reasoning → LLM**;
**operational limits → SST config (fail-loud)**. The "alert now?" JUDGMENT is
the LLM's. The remaining numbers (lookahead horizon, candidate cap, confidence
floor) bound the job and gate model confidence — they do not decide timing.

## Relationship to BUG-021-005

Same directive, same pattern, same file. BUG-021-005 converted the cooling
producer; this converts the three timing-based producers in the same
`alert_producers.go`. The two remaining producers in that file
(`commitment_overdue`, `meeting_brief`) are event-driven (overdue = already
past; meeting brief = fixed pre-meeting lead) and are not threshold-judged here;
they can follow the same pattern later if desired.

## Cross-References

- Scenario: `config/prompt_contracts/alert-timing-evaluate-v1.yaml`
- Evaluator: `internal/intelligence/alert_timing.go`
- Producers: `internal/intelligence/alert_producers.go`
- Wiring: `cmd/core/wiring_cooling.go` (`wireAlertTimingEvaluator`)
- SST loader: `internal/config/alert_timing.go`
- Sibling (cooling): `../BUG-021-005-relationship-cooling-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
