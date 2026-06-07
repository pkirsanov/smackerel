# BUG-021-005: relationship-cooling decision must be LLM-driven, not hardcoded magic-number thresholds

**Status:** Resolved (LLM-driven cooling judgment via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Supersedes:** `BUG-021-004` (which extracted the thresholds into Go constants + a lock test — the WRONG direction; this bug removes both)
**Affected surface:** `internal/intelligence/alert_producers.go`, `internal/intelligence/cooling.go` (new), `config/prompt_contracts/relationship-cooling-evaluate-v1.yaml` (new)

## Summary

The relationship-cooling producer decided "is this relationship cooling?" with
hardcoded SQL magic numbers (`> 30 days` silent, `>= 4` interactions in a fixed
`[90, 180]`-day window, `LIMIT 10`). That is **domain reasoning encoded as fixed
thresholds in Go** — which the product architecture explicitly forbids:

> docs/smackerel.md §3.6: *"Hardcoded rule chains, regex-based intent routers,
> keyword-based classification trees... are NOT an acceptable target
> architecture. Domain reasoning — classification, intent routing,
> multi-step workflows — is LLM-driven, not rule-driven. The LLM is
> responsible for decisions (when to invoke which capability, in what order,
> with what arguments)."*

Whether 45 days of silence means "cooling" is entirely situational: alarming for
a weekly contact, routine for a quarterly one. A fixed threshold cannot capture
that — only per-situation LLM judgment can.

`BUG-021-004` made this worse by extracting the magic numbers into Go constants
and adding a **lock test that cemented them**. This bug reverses that and moves
the judgment to the LLM, following the established `annotation_classify`
precedent (which already replaced a hardcoded `interactionMap` literal with an
LLM scenario).

## Mechanism (the old, hardcoded path)

`ProduceRelationshipCoolingAlerts` ran one inline query whose `HAVING` clause
encoded the entire cooling judgment as constants:
`EXTRACT(DAY ...) > 30 AND COUNT(...) FILTER (... INTERVAL '180 days' ... '90 days') >= 4 ... LIMIT 10`.
Every contact that passed the fixed thresholds got an alert; nuance (this
person's own cadence, a recently-rekindled contact, sparse history) was
invisible.

## Fix (delivered — LLM-driven)

1. **New scenario** `config/prompt_contracts/relationship-cooling-evaluate-v1.yaml`
   (`relationship_cooling_evaluate`): input = a candidate's interaction signals
   (days since last, total interactions, relationship span, typical cadence);
   output = `{is_cooling, confidence, rationale}`. The system prompt instructs
   the LLM to judge cooling **per situation** — compare the current silence
   against the person's OWN cadence; a naturally infrequent contact is not
   cooling; sparse history → low confidence.
2. **New evaluator** `internal/intelligence/cooling.go`: `CoolingEvaluator`
   interface + `BridgeCoolingEvaluator` (routes to the scenario via the agent
   bridge, mockable in tests), plus a `noop_relationship_cooling` tool to
   satisfy the loader's `allowed_tools` contract.
3. **Producer rework**: the SQL now only RETRIEVES candidates + signals (pure
   data — `typical_gap_days = span/(interactions-1)` is arithmetic, not a
   threshold), ordered most-dormant-first. The LLM decides cooling per
   candidate; the Go side just gates on an operational confidence floor and
   creates the alert (with the LLM's rationale as the body). No hardcoded
   "cooling" threshold remains. When the evaluator is not wired, cooling
   production is skipped — there is **no magic-number fallback**.
4. **Operational bounds → SST** (not business thresholds, per the documented
   boundary): `intelligence.relationship_cooling.{max_candidates,
   confidence_floor, dedup_window_days}` — a throughput cap, a
   decision-confidence safety gate, and an anti-spam re-alert window. Fail-loud
   (Gate G028), operator-tunable.
5. **Removed** `BUG-021-004`'s constants + `relationshipCoolingAlertQuery()`
   builder + `TestRelationshipCoolingHeuristic_MatchesDocumentedContract` lock
   test.

## Operational vs business boundary (why some numbers remain, as SST)

Per docs/smackerel.md §3.6 + the constitution C8 SST policy, the line is:
**business reasoning → LLM**; **operational limits → SST config (fail-loud)**.
The cooling JUDGMENT is now the LLM's. The remaining numbers
(`max_candidates`, `confidence_floor`, `dedup_window_days`) are operational
throughput / safety / anti-spam knobs — they do not decide whether a
relationship is cooling; they bound the job and gate model confidence. They are
SST-configured and fail loud, satisfying NO-DEFAULTS.

## Cross-References

- Scenario: `config/prompt_contracts/relationship-cooling-evaluate-v1.yaml`
- Evaluator: `internal/intelligence/cooling.go`
- Producer: `internal/intelligence/alert_producers.go` (`ProduceRelationshipCoolingAlerts`)
- Wiring: `cmd/core/wiring_cooling.go`
- SST loader: `internal/config/relationship_cooling.go`
- Precedent: `internal/annotation/classifier_bridge.go` (annotation_classify)
- Superseded: `../BUG-021-004-relationship-cooling-heuristic-untested/`
- Architecture: `docs/smackerel.md` §3.6 (LLM Agent + Tools Pattern)
