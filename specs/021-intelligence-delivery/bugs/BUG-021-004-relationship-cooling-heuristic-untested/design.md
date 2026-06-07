# Design: BUG-021-004

## Problem

`ProduceRelationshipCoolingAlerts` encoded its heuristic as inline SQL magic
numbers with no test, and the threshold deviates from the spec's "≥ 1/week"
shorthand. See `bug.md` for the verified mechanism and the product-decision
boundary.

## Change (engineering concern only — no behavior change)

1. **Named constants.** Introduce documented package constants for the
   heuristic: `coolingSilenceMinDays = 30`, `coolingPriorWindowStartDays = 180`,
   `coolingPriorWindowEndDays = 90`, `coolingMinPriorInteractions = 4`,
   `coolingDedupWindowDays = 30`, `coolingMaxAlertsPerRun = 10`. The comment
   documents the operationalization of "previously regular contact" AND the
   surfaced deviation from "≥ 1/week".
2. **Testable builder.** `relationshipCoolingAlertQuery()` builds the SQL from
   the constants via `fmt.Sprintf` (integer constants only — no user input, no
   injection). The output is byte-for-byte the prior inline literal, so the
   producer's behavior is unchanged. `ProduceRelationshipCoolingAlerts` calls
   the builder.
3. **Lock test.** `TestRelationshipCoolingHeuristic_MatchesDocumentedContract`
   asserts the produced query contains the documented threshold fragments
   (independently hardcoded as the contract) and that the constants back them.

## Why a source-contract test (not a live-DB integration test)

The producer SQL needs a live Postgres to exercise behaviorally, and the repo
has no producer DB harness (the existing `alert_producers_test.go` covers only
pure-Go helper math). A live-DB seed test would require modeling the
people/edges/artifacts schema and risks incorrect seeds. The contract test
locks the heuristic thresholds (the auditor's core concern — "if the threshold
changes, no test will catch it") without a live DB, and is non-tautological: the
expected fragments are hardcoded independently of the constants, so a constant
change fails the test.

## Why the threshold direction is NOT decided here

Reconciling `≥ 4 in 90 days` (shipped) vs `≥ 1/week` (spec shorthand) changes
the user's live cooling-alert frequency — a product decision. This fix locks the
SHIPPED behavior and surfaces the choice as DI-021-004. Editing the parent spec
021 requirement text or the runtime threshold without owner direction would be
an unauthorized product/spec change.

## Blast Radius

- `internal/intelligence/alert_producers.go` — constants + builder + the
  producer now calls the builder (identical SQL).
- `internal/intelligence/alert_producers_test.go` — the lock test (+ `strings`
  import).
- No schema, no config, no behavior change.

## Alternatives Considered

- **Change the code to `≥ 1/week`.** Rejected here: alters the user's live alert
  behavior (a product decision) — surfaced as DI-021-004 Option B instead.
- **Edit the parent spec to `≥ 4 in 90 days`.** Rejected here: rewriting a
  certified spec's requirement to match delivery is the owner's call (Option A);
  left for owner-directed refresh.
- **Live-Postgres integration test.** Rejected: no producer DB harness; high
  seed-modeling risk; the contract test locks the heuristic without it.
