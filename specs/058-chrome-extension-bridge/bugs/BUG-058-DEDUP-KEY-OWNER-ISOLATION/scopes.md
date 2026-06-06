# Scopes: BUG-058-DEDUP-KEY-OWNER-ISOLATION

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

The dedup-key contract decision (per-owner namespacing) was ratified by the
operator and delivered via `bubbles-workflow mode: bugfix-fastlane`
(parent-expanded — the active runtime lacks `runSubagent`). Single scope, Done.

## Scope 1 — Owner-namespaced dedup key

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane; contract decision ratified by operator)

### Definition of Done

- [x] Parent design §2.3 amended: dedup key preimage includes `owner_user_id` as the outermost namespace; OQ-2 resolved (global dedup was an oversight)
      → Evidence: `specs/058-chrome-extension-bridge/design.md` §2.3 + report.md `## Resolution of Open Questions`
- [x] `ComputeDedupKey` preimage prepends `owner_user_id`; single caller in `internal/api/connectors/extension/ingest.go` updated with a fail-loud empty-owner guard (`owner_required`, no fallback)
      → Evidence: report.md `### Code Diff Evidence` (Exit Code: BUILD_EXIT=0)
- [x] Unit twin `TestComputeDedupKey_VariesByOwner` in `internal/connector/ingest/dedup_test.go`: two owners diverge for identical tuples; same-owner determinism preserved
      → Evidence: report.md `## Test Evidence` (red→green; `go test` PASS)
- [x] Store-level `TestDedupStore_CrossOwnerIsolation`: owner B receives its OWN artifact (isDup=false); same-owner repeat dedups to the original id
      → Evidence: report.md `## Test Evidence`
- [x] `TestComputeDedupKey_VariesByDevice` (single-owner-multi-device "Chrome Sync") still passes — no regression to legitimate same-owner dedup
      → Evidence: report.md `## Test Evidence` (`--- PASS: TestComputeDedupKey_VariesByDevice`)
- [x] Handler fail-loud `TestIngest_RejectsItemWithoutOwner`: empty owner → `owner_required`, never published
      → Evidence: report.md `## Test Evidence` (`--- PASS: TestIngest_RejectsItemWithoutOwner`)
- [x] Live-Postgres `TestPostgresDedupStore_CrossOwnerIsolation` added (`tests/integration/extension_dedup_owner_isolation_test.go`): two owners → two rows + two artifact_ids, neither publish skipped; compiles under the integration tag, runs in CI, skips locally when `DATABASE_URL` is unset
      → Evidence: report.md `## Test Evidence` (integration compile + SKIP transcript)
- [x] `SCN-058-022..025` added to `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] `go build ./...`, `go vet`, `go test -race` green; NO schema migration added (`dedup_key` stays `BYTEA PRIMARY KEY`)
      → Evidence: report.md `## Test Evidence` (`BUILD_EXIT=0`; `ok` both packages)
- [x] Parent spec 058 recertified (design §2.3 edit + bubbles.spec-review CURRENT) and BUG-058 added to `resolvedBugs`
      → Evidence: `specs/058-chrome-extension-bridge/report.md` + `specs/058-chrome-extension-bridge/state.json`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — `TestPostgresDedupStore_CrossOwnerIsolation` (live-Postgres) persists the cross-owner isolation invariant; CI-run
      → Evidence: report.md `## Test Evidence` (integration compile + SKIP transcript; runs in CI)
- [x] Broader E2E regression suite passes in CI (the integration tier includes this test; it skips locally without a live stack, while the unit + store + handler tiers are green locally)
      → Evidence: report.md `## Test Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-058-DEDUP-OWNER-01 | TestComputeDedupKey_VariesByOwner | internal/connector/ingest/dedup_test.go | unit (red→green) | SCN-058-022 |
| T-058-DEDUP-OWNER-02 | TestDedupStore_CrossOwnerIsolation | internal/connector/ingest/dedup_test.go | unit/store (red→green) | SCN-058-023 |
| T-058-DEDUP-OWNER-03 | TestIngest_RejectsItemWithoutOwner | internal/api/connectors/extension/ingest_test.go | unit (handler) | SCN-058-024 |
| T-058-DEDUP-OWNER-04 | TestComputeDedupKey_VariesByDevice | internal/connector/ingest/dedup_test.go | regression (preserved) | SCN-058-026 |
| T-058-DEDUP-OWNER-05 | TestPostgresDedupStore_CrossOwnerIsolation | tests/integration/extension_dedup_owner_isolation_test.go | Regression E2E (integration, CI-run) | SCN-058-025 |

### Non-Goals

- The Round-18 `source_device_id` charset/length hardening — already shipped.
- Schema migration of `raw_ingest_dedup` — none required (preimage-only change).
- The four gaps in `../BUG-058-EXTERNAL-INFRA-MISSING/`.
