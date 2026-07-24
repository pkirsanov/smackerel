# Bug: [BUG-004] Production Readiness Claims Drift From Runtime Truth

## Summary

Release, documentation, and specification rollups describe capabilities as delivered based on code/spec state while deployed activation and end-to-end paths may be disabled, empty, broken, fixture-only, degraded, stale, or unverified.

## Severity

High (S2): users and operators cannot distinguish implemented code from a live usable capability.

## Status And Provenance

Reported from operator-supplied current-session historical input and the specs 105/106 analyst review. **Claim Source:** interpreted. No docs publication, runtime query, source mutation, commit, push, or deployment occurred here.

## Reproduction Steps

1. Read current release/docs/spec capability rollups that infer delivered status from implementation or certification state.
2. Compare those claims with the reported production journeys: Graph routes 404, modern auth rejects the credential session, Search does not submit, Digest is false empty, Assistant is blank on rejection, Recommendations has zero providers, and other capabilities may be disabled/fixture-only/unverified.
3. Observe no canonical distinction among implemented, configured, activated, live-verified, degraded, and disabled.

## Expected Behavior

One capability ledger records independent implementation, configuration, activation, live-verification, degradation, and disablement dimensions with evidence provenance/freshness. Managed docs, release claims, status UI, and rollups derive claims from that ledger. A capability cannot be called live/ready solely because code or a spec is done.

## Actual Behavior

Documented readiness can outrun deployed behavior and remain optimistic after runtime evidence contradicts it.

## Outcome Contract

**Intent:** Make every readiness claim traceable to current runtime evidence while preserving truthful implementation history.

**Success Signal:** A canonical ledger classifies each capability across all six dimensions, invalidates stale/contradictory live claims, and mechanically blocks docs/release text that overstates the derived status.

**Hard Constraints:** Historical implementation/certification records are not rewritten; no secret/personal evidence in the ledger; live verification requires actual journey evidence; disabled/degraded/empty/fixture-only remain distinct; no production write is performed merely to prove readiness.

**Failure Condition:** Any doc/release/status surface can claim delivered/live/ready from code/spec state alone, stale evidence remains current, or runtime contradiction is hidden behind a single boolean.

## Impact And Dependencies

- Consumes product journey evidence from `BUG-102-001` when available.
- Must represent all product bug dependencies without waiting to build the schema; unresolved capabilities remain explicitly unverified/degraded/disabled.
- Supports specs 105/106 readiness truth without editing those concurrent analyst files.

## Ownership Boundary

`bubbles.design` owns ledger/derivation architecture, `bubbles.plan` owns delivery scopes, `bubbles.docs` owns managed-doc reconciliation, and `bubbles.ux` owns operator/user status presentation. Certified parent requirements remain unchanged.
