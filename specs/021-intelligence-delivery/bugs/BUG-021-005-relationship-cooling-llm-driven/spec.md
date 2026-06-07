# Spec: BUG-021-005 — relationship-cooling judgment must be LLM-driven

## Expected Behavior

Whether a relationship is "cooling" MUST be decided by the LLM per situation —
comparing the current silence against that contact's own historical cadence —
NOT by hardcoded numeric thresholds in Go. The Go core retrieves candidates and
their signals; the LLM judges. Only OPERATIONAL bounds (throughput cap,
confidence safety gate, anti-spam window) remain, and those are SST-configured
and fail loud.

## Actual Behavior

`ProduceRelationshipCoolingAlerts` decided cooling with inline SQL magic numbers
(`> 30` days silent, `>= 4` interactions in a fixed 90-180 day window,
`LIMIT 10`). `BUG-021-004` extracted those into Go constants and added a lock
test cementing them. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (LLM judges):** the cooling decision flows through the
   `relationship_cooling_evaluate` scenario via the agent bridge; no Go code
   contains a hardcoded "cooling" threshold (silence days, interaction count,
   window).
2. **AC-2 (signals, not thresholds):** the producer query retrieves candidate
   signals (days since last, total interactions, span, typical cadence) and
   orders most-dormant-first; the only numbers it carries are operational
   ($dedup_window, $candidate_cap).
3. **AC-3 (evaluator tested):** `BridgeCoolingEvaluator` is unit-tested with a
   scripted bridge runner — parses the decision, routes to the correct
   scenario, forwards candidate signals, never leaks the internal PersonID, and
   errors on every failure path.
4. **AC-4 (no magic-number fallback):** when the evaluator is not wired, cooling
   production is SKIPPED (no hardcoded heuristic runs).
5. **AC-5 (operational bounds as SST):** `intelligence.relationship_cooling.*`
   keys are fail-loud SST; missing/invalid values are rejected naming the key.
6. **AC-6 (BUG-021-004 reversed):** the constants, the query builder, and the
   `TestRelationshipCoolingHeuristic_MatchesDocumentedContract` lock test are
   removed.

## Out of Scope

- Live-LLM behavioral validation of the scenario (covered by the live-stack
  tier; the unit tier mocks the bridge).
- Other hardcoded business thresholds in the digest/recommendation layers
  (tracked separately — see the sweep follow-up).
- Changing the weekly schedule or the alert delivery path.

## Cross-References

- Bug detail + the operational/business boundary: `bug.md`
- Architecture: `docs/smackerel.md` §3.6
- Superseded: `../BUG-021-004-relationship-cooling-heuristic-untested/`
