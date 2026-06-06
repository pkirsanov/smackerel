# Scopes: BUG-058-DEDUP-KEY-OWNER-ISOLATION

This bug requires a planning-truth change (parent `design.md` §2.3) and is
routed to `bubbles.design`. The single scope below is `Not Started`; it is the
fix work to be executed AFTER `bubbles.design` ratifies the proposal in
`design.md`. The chaos round (Round 18) only discovered, verified, and routed
the finding — no fix code was written for this bug.

## Scope 1 — Owner-namespaced dedup key

**Status:** Not Started
**Owner chain:** bubbles.design -> bubbles.plan -> bubbles.implement -> bubbles.test -> bubbles.validate

### Definition of Done

- [ ] Parent design §2.3 "Resolved Decisions" amended: dedup key tuple includes `owner_user_id` (bubbles.design)
- [ ] OQ-2 resolved: confirm global dedup was not intentional, or close `wontfix` with a documented rationale (bubbles.design)
- [ ] `ComputeDedupKey` preimage prepends `owner_user_id`; single caller in `internal/api/connectors/extension/ingest.go` updated (bubbles.implement)
- [ ] Unit twin in `internal/connector/ingest/dedup_test.go`: `ComputeDedupKey(ownerA, ...) != ComputeDedupKey(ownerB, ...)` for identical tuples; same-owner determinism preserved (bubbles.test)
- [ ] `TestComputeDedupKey_VariesByDevice` (single-owner-multi-device) still passes — no regression (bubbles.test)
- [ ] Live-Postgres test alongside `tests/integration/extension_dedup_race_test.go`: two owners + same tuple resolve to two distinct rows and two distinct artifact_ids, neither publish skipped (bubbles.test)
- [ ] `SCN-058-022` added to `scenario-manifest.json` + parent Scope 2 DoD (bubbles.plan)
- [ ] Parent Scope 2 recertified (bubbles.validate)
