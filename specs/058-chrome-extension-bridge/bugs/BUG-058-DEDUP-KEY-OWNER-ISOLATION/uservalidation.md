# User Validation: BUG-058-DEDUP-KEY-OWNER-ISOLATION

The per-owner dedup-key fix is delivered via bugfix-fastlane (parent-expanded).
The checklist below reflects the genuinely-completed delivery: the unit +
store-level cross-tenant isolation proof executed locally (red→green), the
single-owner-multi-device case is preserved, and the live-Postgres assertion is
authored and CI-run. Full evidence is in [report.md](report.md).

## Checklist

- [x] Finding mechanism verified by code reading at repo HEAD (`internal/connector/ingest/dedup.go`, `internal/db/migrations/040_raw_ingest_dedup.sql`, `internal/api/connectors/extension/ingest.go`)
- [x] Round-18 confirming probe `TestComputeDedupKey_SeparatorInjectionResistance` ran and passed (keyer null-byte separator hygiene intact)
- [x] Bug packet (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json) filed
- [x] Cross-owner isolation fix implemented (`owner_user_id` folded FIRST into the dedup-key preimage); single caller updated with a fail-loud empty-owner guard
- [x] Cross-owner isolation proven locally at unit + store tier: `TestComputeDedupKey_VariesByOwner` + `TestDedupStore_CrossOwnerIsolation` genuinely fail before the preimage change and pass after (red→green in report.md)
- [x] Single-owner-multi-device ("Chrome Sync") preserved: `TestComputeDedupKey_VariesByDevice` still passes
- [x] Live-Postgres test `TestPostgresDedupStore_CrossOwnerIsolation` added (two owners → two rows + two artifact_ids); compiles under the integration tag and runs in CI (skips locally when `DATABASE_URL` is unset)
- [x] OQ-2 resolved: global dedup was an oversight (owner already stored in row; admin view owner-scoped) — key now namespaced per owner
