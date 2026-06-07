# Spec: BUG-021-004 — the relationship-cooling heuristic must be explicit and test-guarded

## Expected Behavior

The relationship-cooling detection thresholds MUST be explicit, documented, and
guarded by a test, so a threshold change cannot drift silently. The documented
contract MUST match the shipped runtime behavior, and any deviation from the
parent spec's "previously regular contact" wording MUST be surfaced (not
silently reconciled in either direction).

## Actual Behavior

The thresholds were inline SQL magic numbers with no test, and the code's
`≥ 4 interactions in 90 days` deviates (looser) from the spec's "≥ 1/week"
shorthand. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (explicit constants):** the heuristic thresholds are named package
   constants with a documenting comment; the query is built from them.
2. **AC-2 (no behavior change):** the produced SQL is identical to the prior
   inline literal — the running alert behavior is unchanged.
3. **AC-3 (lock test):** `TestRelationshipCoolingHeuristic_MatchesDocumentedContract`
   asserts the produced query embeds the documented threshold fragments; it
   fails if any threshold changes (proven by an adversarial drift run).
4. **AC-4 (surfaced, not decided):** the spec/code threshold disagreement is
   recorded as a discovered issue (DI-021-004) routed to the owner; the parent
   spec's requirement text is NOT rewritten here.

## Out of Scope

- Changing the running alert behavior (Option B — a product decision for the
  owner).
- Editing the parent spec 021 `R-021-005` / `UC-005` requirement wording (owner
  follow-up once the threshold direction is chosen).
- A live-Postgres integration test of the producer (no producer DB harness
  exists; the contract test locks the heuristic without one).

## Cross-References

- Bug detail + the owner decision (DI-021-004): `bug.md`
- Parent spec: `../../spec.md`
