# Regression Permanence

Purpose: canonical source for regression permanence requirements.

## Rules
- Every changed or fixed behavior needs persistent scenario-specific regression coverage. This is UNIVERSAL.
- The CATEGORY of that coverage is proportionate to the scenario's behavior traits, per the authoritative matrix in [test-core.md](test-core.md) and [`bubbles/registry/proof-obligations.yaml`](../../bubbles/registry/proof-obligations.yaml) (IMP-047 S-D). This file previously required E2E for every change; that wording conflicted with the trait matrix and is retired in favour of it.
- Traits owing LIVE proof must have it: a browser-driven run on the current production route for user-visible UI, a real request and response for an API contract, a write-and-read round trip for mutable state, a live boundary for freshness, retry, queue, provider or dependency behavior, and stress or load for SLA claims. A synthetic test may complement an applicable live proof; it may never replace one.
- Pure logic, documentation, static metadata, and non-runtime configuration receive proportionate proof. Runtime configuration receives no documentation exemption: a configured value that changes what the running system does is exercised through startup or runtime behavior.
- A broad rerun of existing suites is not enough by itself.
- UI changes require user-visible assertions; API changes require consumer-visible behavior checks.
- Rename/removal work requires consumer-facing regression coverage, not just producer-surface checks.
- Bug-fix regressions must include at least one adversarial case that would fail if the bug were reintroduced; a tautological case that already satisfies the broken path is not protective coverage.
- During trait backfill, an existing E2E link remains valid regression evidence. Traits are backfilled conservatively and an unknown trait requires review rather than an exemption.

## Cross-Spec Regression (Gate G044)

The `bubbles.regression` agent (Steve French) enforces cross-feature regression prevention:

- **G044 (regression baseline):** Test baseline snapshot before/after implementation — any previously-passing test that now fails is a REGRESSION.
- **G044 (comprehensive regression — cross-spec phase):** Tests from DONE specs must be re-executed after changes to verify no cross-feature interference.
- **G044 (comprehensive regression — conflict detection phase):** New specs scanned for route collisions, shared table mutations, contradictory business rules, and API contract conflicts against existing specs. When the project declares a `domainModel:` SST (delivered by Gate G130; consistency-nudged by Gate G131), the "contradictory business rules" sweep gains a STRUCTURED target — a spec's declared state transition can be diffed against the shared `domainModel` state machine, instead of only grepping prose across specs.

## Enforcement

The `regression` phase runs after `test` and before `simplify` in all delivery modes:
```
implement → test → regression → simplify → stabilize → security → docs → ...
```

This ensures regressions are caught at the earliest possible point after code is verified.
