# Bug: [BUG-004-004] Synthesis Persistence And Health Are Not Truthful

## Summary

`RunSynthesis` constructs synthesis structs and the scheduler logs only a count; `synthesis_insights` and `weekly_synthesis` remain empty, while health maps a never-run job to up.

## Severity

Critical (S1): the product reports a healthy intelligence capability that produces no durable user-readable output.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No scheduler run, SQL query, health probe, or source execution occurred in this planning-only invocation.

## Reproduction Steps

1. Trigger or wait for the synthesis scheduler path that invokes `RunSynthesis`.
2. Observe that synthesis structs are constructed and only a count is logged.
3. Query `synthesis_insights` and `weekly_synthesis`; observe zero persisted rows.
4. Inspect capability/job health before any successful run; observe never-run mapped to up.

## Expected Behavior

Each eligible synthesis run transactionally persists source-cited insights and a weekly synthesis, is idempotent for the same source/window, applies retention/lifecycle, exposes an authorized read surface, and reports never-run, running, current, stale, partial, and failed health truthfully with actionable alerts.

## Actual Behavior

Ephemeral structs and a count log substitute for durable output, and health claims availability without proof of any successful run.

## Outcome Contract

**Intent:** Turn synthesis execution into durable, explainable, idempotent output and make health reflect actual persisted run state.

**Success Signal:** A live-stack run writes source-linked insight and weekly rows in one transaction; rerun does not duplicate; authorized reads return them; never-run/stale/failed states are visible and alertable.

**Hard Constraints:** PostgreSQL is authoritative; citations/provenance precede persistence; no partial transaction; no fabricated health; retention/lifecycle is explicit; telemetry excludes personal content.

**Failure Condition:** Synthesis can report success/up with zero durable rows, duplicate a window, persist uncited output, partially commit, hide stale/failure, or lack an authorized read path.

## Impact And Dependencies

- Blocks truthful product journey acceptance in `BUG-102-001-product-journey-acceptance-gap`.
- Affects Digest/intelligence readiness consumed by the coherent product experience.
- Must preserve source-qualified processing and trust-through-transparency product principles.

## Root Cause Ownership

`bubbles.design` must confirm transaction ownership, persistence models, scheduler/run identity, lifecycle/retention, read API/UI, and health/alert derivation before implementation.
