# BUG-021-004: relationship-cooling heuristic was untested inline SQL, and deviates from the spec's "≥ 1/week" shorthand

**Status:** Resolved for the untested-heuristic concern (constants + lock test); threshold-direction surfaced to owner (DI-021-004)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Stochastic Quality Sweep Round 9 (parent: stochastic-quality-sweep) — `regression`, parent-expanded
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/alert_producers.go` (`ProduceRelationshipCoolingAlerts`)

## Summary

The relationship-cooling alert producer encodes its detection heuristic as
inline SQL magic numbers with **no test**, and the heuristic **deviates from the
spec's shorthand**:

- **Spec 021 R-021-005 / UC-005** require alerting on a contact "previously in
  regular contact (previously ≥ 1/week frequency)" who has gone silent > 30
  days.
- **The shipped SQL** fires when a person has `> 30` days since last contact
  AND `≥ 4` distinct interactions in the prior window `[90, 180]` days ago
  (3–6 months back). `≥ 4 in 90 days ≈ 1 per 22 days` — materially LOOSER than
  the spec's "≥ 1/week".

Two distinct problems: (1) the heuristic was unguarded by any test, so a
threshold change would drift silently; (2) the code's threshold and the spec's
shorthand disagree, and the correct reconciliation direction is a product
decision.

## Mechanism (verified by code reading at repo HEAD)

`ProduceRelationshipCoolingAlerts` ran an inline query with the literals `> 30`,
`INTERVAL '180 days'`, `INTERVAL '90 days'`, `>= 4`, dedup `30 days`, `LIMIT 10`.
The existing `alert_producers_test.go` tests only cover pure-Go helper math
(clampDay, billing dates); the cooling SQL had no unit or integration coverage,
and there is no live-DB producer harness in the repo.

## Impact / Severity rationale (Medium)

- **Untested invariant:** any future edit to the cooling thresholds (window,
  count, silence) would pass CI silently — the auditor's core complaint.
- **Spec/code drift:** the alert fires for less-regular contacts than the spec's
  "≥ 1/week" wording implies (more cooling alerts, not fewer). On a single-user
  system this favors surfacing, but it is undocumented drift from the spec.
- **No correctness/security risk:** the producer is read-only detection +
  `CreateAlert`; dedup and the per-run cap are intact.

## Fix (delivered — engineering concern)

1. **Extract the heuristic into named, documented constants**
   (`coolingSilenceMinDays`, `coolingPriorWindowStartDays`,
   `coolingPriorWindowEndDays`, `coolingMinPriorInteractions`,
   `coolingDedupWindowDays`, `coolingMaxAlertsPerRun`) and a testable
   `relationshipCoolingAlertQuery()` builder. **The produced SQL is byte-for-byte
   identical** to the prior inline literal — **no runtime behavior change**.
2. **Lock test** `TestRelationshipCoolingHeuristic_MatchesDocumentedContract`
   asserts the produced query embeds the documented threshold fragments
   (independently hardcoded as the contract) and that the constants back it — so
   any threshold change now fails CI, forcing a conscious, spec-reconciled
   decision.

## Surfaced for owner decision (DI-021-004 — NOT changed unilaterally)

The spec/code threshold disagreement is a **product decision** and was NOT
resolved by silently editing the spec OR changing the alert behavior:

- **Option A (keep shipped):** document the spec's threshold as `≥ 4 distinct
  interactions in the prior 90-day window` (the shipped, more-surfacing value).
  No runtime change. The lock test already encodes this.
- **Option B (honor "≥ 1/week"):** tighten `coolingMinPriorInteractions` toward
  a weekly cadence (fewer, stricter cooling alerts). This is a ONE-line change
  to the constant + the test fragment, and would change the user's live alert
  frequency.

This fix deliberately did NOT alter the running alert behavior and did NOT
rewrite the parent spec's requirement text. The owner picks the direction; the
parent spec 021 `R-021-005` / `UC-005` wording should then be refreshed by the
spec owner to match (left untouched here to avoid an unauthorized requirement
change on a certified spec).

## Cross-References

- Producer + constants: `internal/intelligence/alert_producers.go`
- Lock test: `internal/intelligence/alert_producers_test.go`
- Parent spec: `../../spec.md` (R-021-005, UC-005)
