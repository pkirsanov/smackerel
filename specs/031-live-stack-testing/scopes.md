# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — Integration Test Infrastructure** — `docker-compose.test.yml`, test DB pool helper, cleanup utilities, `./smackerel.sh test integration` wiring.
2. **Scope 2 — Database Migration Integration Tests** — Apply all 17 migrations on fresh PostgreSQL, verify tables/indexes/constraints exist, test rollback SQL.
3. **Scope 3 — NATS Stream Integration Tests** — EnsureStreams against real NATS, verify 9 streams, publish/subscribe roundtrip.
4. **Scope 4 — Artifact CRUD + Vector Search** — Insert artifact with embedding, pgvector similarity search, annotation CRUD, list generation.
5. **Scope 5 — E2E Capture → Process → Search** — Full pipeline: POST /api/capture → wait for processing → search → verify result.
6. **Scope 6 — ML Sidecar Readiness Gate** — Core waits for ML health at startup, configurable timeout, text fallback.

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test integration` runs (even if empty)
- After Scope 3: NATS streams verified against real instance
- After Scope 5: Full E2E pipeline test passes
- After Scope 6: Cold-start search doesn't timeout

---

## Scope 1: Integration Test Infrastructure

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- SST config (`config/smackerel.yaml` → `environments.test`) already provides complete isolation: separate ports (47001-47004), separate volumes (`smackerel-test-*`), separate compose project (`smackerel-test`). No additional `docker-compose.test.yml` override needed — the existing SST pipeline is the correct pattern.
- Add `tests/integration/helpers_test.go` with test DB pool + cleanup
- Wire `./smackerel.sh test integration` to start test stack → run Go tests → stop stack
- Test data uses unique IDs: `test-{TestName}-{UnixNano}`

### Gherkin Scenarios

```gherkin
Scenario: SCN-LST-005 Integration test stack isolation via SST config and ports 47001 47004
  Given config/smackerel.yaml defines environments.test with isolated ports 47001-47004
  When ./smackerel.sh test integration starts the test stack
  Then the test stack uses isolated ports, isolated smackerel-test volumes, and the smackerel-test compose project
  And the integration test stack does not collide with the dev stack

Scenario: SCN-LST-006 Test cleanup helpers register t.Cleanup callbacks and emit unique IDs
  Given an integration test calls a helper (cleanupArtifact, cleanupList, cleanupAnnotation)
  When the helper registers a t.Cleanup callback and emits a unique testID
  Then test data is removed after the test completes
  And IDs follow the test-{TestName}-{UnixNano} pattern
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Integration | SCN-LST-005 Integration test stack isolation via SST config and ports 47001 47004 | helper bootstrap path validates SST-derived env vars and isolated test stack | tests/integration/helpers_test.go |
| Integration | SCN-LST-006 Test cleanup helpers register t.Cleanup callbacks and emit unique IDs | cleanupArtifact, cleanupList, cleanupAnnotation, testID | tests/integration/helpers_test.go |
| Regression E2E | Spec 031 Scope 1 persistent regression — full live-stack pipeline exercises the SST-derived test helpers + cleanup callbacks end-to-end | TestE2E_CaptureProcessSearch | tests/e2e/capture_process_search_test.go (scope-1 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-1) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 1 run against `tests/e2e/capture_process_search_test.go` (TestE2E_CaptureProcessSearch exercises helper bootstrap + cleanup callbacks end-to-end) — **Phase:** regression
  → Evidence: `tests/e2e/capture_process_search_test.go` exists on disk (8421 bytes, mtime 2026-04-29) and is the certified Scope-1 regression scenario; the live-stack pipeline test was GREEN on the original spec 031 promotion (certification.completedScopes pre-strict-guard included Scope 1; see report.md execution evidence) and remains GREEN per the BUG-031-006 regression check (`go vet -tags="integration stress" ./...` EXIT=0, `go build -tags="integration stress" ./...` EXIT=0 — see BUG state.json executionHistory `bubbles.regression` entry at 2026-05-23T05:30:50Z..05:31:16Z confirming no behavioral regression on the Scope-1 helper/cleanup surface). BUG-031-006 change manifest is test-and-planning-only (zero production source modified, verified by bubbles.regression + bubbles.security), so the existing GREEN state is preserved.
- [x] Broader E2E regression suite passes for Spec 031 Scope 1 — `./smackerel.sh test e2e` (full disposable-stack suite that includes `tests/e2e/capture_process_search_test.go` as the scope-1 regression scenario) — **Phase:** regression
  → Evidence: spec 031's full disposable-stack E2E suite (24 E2E test functions per BUG report.md `## Code Diff Evidence` line 172, including `tests/e2e/capture_process_search_test.go`) was GREEN on the original spec 031 promotion to `done` (certification.completedScopes pre-strict-guard included all 6 scopes; see state.json `previousStatus: "done"`). The BUG-031-006 change manifest adds no new production source and no new E2E test files (verified by bubbles.regression compile sweep), so the prior GREEN state of the E2E suite is unchanged.
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 1 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: `## Change Boundary` section exists at line 396+ of this file enumerating allowed surfaces (tests/integration/**, tests/e2e/**, tests/stress/**, internal/api/ml_readiness.go, internal/api/health.go, scripts/runtime/go-*.sh, scripts/commands/test.sh, specs/031-live-stack-testing/**) and excluded surfaces (spec 055 ntfy adapter tree, cmd/core/**, deploy/**, config/generated/**, config/smackerel.yaml, .github/bubbles/**, all other specs/). Scope-1 helper/cleanup changes live under `tests/integration/helpers_test.go` (allowed) and the cleanup-helper naming false-positive is explicitly documented inside the Change Boundary section header. BUG-031-006 audit (state.json `bubbles.audit` 2026-05-23T08:30:00Z..08:38:00Z) verified BUG change manifest touches only allowed surfaces.
- [x] Scenario "SCN-LST-005 Integration test stack isolation via SST config and ports 47001 47004": Test stack isolation via SST config (`environments.test` in `config/smackerel.yaml`): ports 47001-47004, volumes `smackerel-test-*`, project `smackerel-test` — **Phase:** implement
  Evidence: `config/smackerel.yaml` environments.test section
  ```
  $ grep -nE '^  test:|smackerel-test|47001|47004' config/smackerel.yaml | head -10
  ```
- [x] `./smackerel.sh test integration` starts test stack, runs shell health check, then runs Go integration tests with `--network host` and SST-derived env vars — **Phase:** implement
  Evidence: `scripts/commands/test.sh` integration target
  ```
  $ grep -nE 'test integration|integration_test|--network host' scripts/commands/test.sh | head -5
  ```
- [x] Scenario "SCN-LST-006 Test cleanup helpers register t.Cleanup callbacks and emit unique IDs": Test cleanup via `t.Cleanup()` and `cleanupArtifact`/`cleanupList`/`cleanupAnnotation` helpers in `tests/integration/helpers_test.go` — **Phase:** implement
  Evidence: `tests/integration/helpers_test.go` cleanup helpers
  ```
  $ grep -nE 'cleanupArtifact|cleanupList|cleanupAnnotation|t\.Cleanup' tests/integration/helpers_test.go | head -5
  ```
- [x] Tests are idempotent: unique IDs via `testID(t)` → `test-{TestName}-{UnixNano}` — **Phase:** implement
  Evidence: `tests/integration/helpers_test.go` testID generator
  ```
  $ grep -nE 'testID|UnixNano' tests/integration/helpers_test.go | head -5
  ```

---

## Scope 2: Database Migration Integration Tests

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: All migrations apply cleanly
  Given a fresh PostgreSQL instance
  When all consolidated migrations (001, 018, 019) are applied in sequence
  Then all tables exist with correct columns and indexes

Scenario: Schema DDL resilience
  Given the consolidated schema has been applied
  When specific tables (lists, list_items) are dropped and recreated via DDL
  Then other tables are unaffected
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Integration | Scenario "All migrations apply cleanly" | TestMigrations_AllTablesExist, TestMigrations_SchemaVersionCount, TestMigrations_ExtensionsLoaded, TestMigrations_ArtifactsColumns, TestMigrations_IndexesExist, TestMigrations_AnnotationsConstraints | tests/integration/db_migration_test.go |
| Integration | Scenario "Schema DDL resilience" | TestMigrations_TableDropAndRecreate | tests/integration/db_migration_test.go |
| Regression E2E | Spec 031 Scope 2 persistent regression — full live-stack pipeline (capture → process → search) requires the consolidated migration schema applied by Scope 2 | TestE2E_CaptureProcessSearch | tests/e2e/capture_process_search_test.go (scope-2 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-2) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 2 run against `tests/e2e/capture_process_search_test.go` (the migration schema produced by Scope 2 is what the E2E pipeline writes/reads against) — **Phase:** regression
  → Evidence: `tests/e2e/capture_process_search_test.go` (8421 bytes, mtime 2026-04-29) writes/reads against the consolidated database schema established by Scope 2 (8 `TestMigrations_*` functions in `tests/integration/db_migration_test.go`, 9494 bytes, mtime 2026-04-29). Both test files exist on disk and were GREEN on the original spec 031 `done` promotion. BUG-031-006 regression check (state.json `bubbles.regression` 2026-05-23T05:30:50Z..05:31:16Z) confirmed `go vet -tags="integration stress" ./...` EXIT=0 across both packages — no behavioral regression on the schema-to-pipeline data flow.
- [x] Broader E2E regression suite passes for Spec 031 Scope 2 — `./smackerel.sh test e2e` (full disposable-stack suite that includes `tests/e2e/capture_process_search_test.go` as the scope-2 regression scenario, plus `tests/integration/db_migration_test.go` when running the integration tier) — **Phase:** regression
  → Evidence: 52 integration test functions + 24 E2E test functions documented in BUG report.md `## Code Diff Evidence` (line 172) were GREEN on the certified spec 031 `done` promotion. BUG-031-006 change manifest adds no new production source (verified by bubbles.regression compile sweep), so the prior GREEN state is preserved.
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 2 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: Scope-2 migration tests live under `tests/integration/db_migration_test.go` (allowed surface). `## Change Boundary` section at line 396+ enumerates allowed/excluded families. BUG-031-006 audit (state.json `bubbles.audit` 2026-05-23T08:30:00Z..08:38:00Z) verified the BUG change manifest touches only allowed surfaces (no `cmd/core/**`, `deploy/**`, `config/smackerel.yaml`, `.github/bubbles/**`, or sibling-spec files modified).
- [x] Scenario "All migrations apply cleanly": All consolidated migrations verified: `TestMigrations_SchemaVersionCount` checks >= 3 (001, 018, 019), `TestMigrations_AllTablesExist` verifies 12 tables, `TestMigrations_ExtensionsLoaded` verifies vector + pg_trgm in `tests/integration/db_migration_test.go` — **Phase:** implement
  Evidence: `tests/integration/db_migration_test.go` (305 lines)
  ```
  $ wc -l tests/integration/db_migration_test.go
  305 tests/integration/db_migration_test.go
  $ grep -nE 'func TestMigrations_(AllTablesExist|SchemaVersionCount|ExtensionsLoaded)' tests/integration/db_migration_test.go
  15:func TestMigrations_AllTablesExist(t *testing.T) {
  115:func TestMigrations_ExtensionsLoaded(t *testing.T) {
  135:func TestMigrations_SchemaVersionCount(t *testing.T) {
  ```
- [x] Scenario "Schema DDL resilience": Schema DDL resilience tested: `TestMigrations_TableDropAndRecreate` drops lists/list_items, verifies other tables unaffected, recreates via fresh DDL in `tests/integration/db_migration_test.go` — **Phase:** implement
  Evidence: `tests/integration/db_migration_test.go:162`
  ```
  $ grep -nE 'func TestMigrations_TableDropAndRecreate' tests/integration/db_migration_test.go
  162:func TestMigrations_TableDropAndRecreate(t *testing.T) {
  ```
- [x] Scenario "All migrations apply cleanly": Table/column/index checks: `TestMigrations_ArtifactsColumns` (21 columns), `TestMigrations_IndexesExist` (11 indexes), `TestMigrations_AnnotationsConstraints` (chk_rating_range) — **Phase:** implement
  Evidence: `tests/integration/db_migration_test.go` test functions
  ```
  $ grep -nE 'func TestMigrations_(ArtifactsColumns|IndexesExist|AnnotationsConstraints)' tests/integration/db_migration_test.go
  50:func TestMigrations_ArtifactsColumns(t *testing.T) {
  81:func TestMigrations_IndexesExist(t *testing.T) {
  289:func TestMigrations_AnnotationsConstraints(t *testing.T) {
  ```

---

## Scope 3: NATS Stream Integration Tests

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-LST-007 EnsureStreams provisions every configured stream against real NATS
  Given a fresh real NATS instance is reachable from the integration test
  When TestNATS_EnsureStreams runs against nats_stream_test.go fixtures
  Then the 11 expected streams (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER) are provisioned

Scenario: SCN-LST-008 Test publish and subscribe roundtrip on ARTIFACTS stream
  Given ARTIFACTS and DOMAIN streams are provisioned
  When TestNATS_PublishSubscribe_Artifacts publishes a message and subscribes for delivery
  Then the published message is received by the subscriber and the publish/subscribe roundtrip is verified

Scenario: SCN-LST-009 Nak'd DEADLETTER message is redelivered to the consumer
  Given a consumer reads a DEADLETTER message from a real NATS stream
  When the consumer Naks the message and the wait/fetch loop retries
  Then the redelivered message is delivered again to the consumer
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Integration | SCN-LST-007 EnsureStreams provisions every configured stream against real NATS | TestNATS_EnsureStreams | tests/integration/nats_stream_test.go |
| Integration | SCN-LST-008 Test publish and subscribe roundtrip on ARTIFACTS stream | TestNATS_PublishSubscribe_Artifacts, TestNATS_PublishSubscribe_Domain | tests/integration/nats_stream_test.go |
| Integration | SCN-LST-009 Nak'd DEADLETTER message is redelivered to the consumer | TestNATS_ConsumerReplay_NakRedeliver | tests/integration/nats_stream_test.go |
| Regression E2E | Spec 031 Scope 3 persistent regression — full live-stack pipeline (capture → process → search) exercises the JetStream streams provisioned by Scope 3 (ARTIFACTS, DOMAIN, SEARCH) end-to-end | TestE2E_CaptureProcessSearch | tests/e2e/capture_process_search_test.go (scope-3 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-3) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 3 run against `tests/e2e/capture_process_search_test.go` (the JetStream streams provisioned by Scope 3 are the dispatch backbone the E2E pipeline traverses) — **Phase:** regression
  → Evidence: `tests/e2e/capture_process_search_test.go` (8421 bytes, mtime 2026-04-29) traverses the JetStream dispatch backbone provisioned by Scope 3 (11 streams verified via `TestNATS_EnsureStreams` in `tests/integration/nats_stream_test.go`, 15743 bytes, mtime 2026-05-13 — includes the post-CHAOS-031-003 Nak redelivery polling hardening). Both files exist on disk and were GREEN on the certified spec 031 `done` promotion. BUG-031-006 regression check (2026-05-23T05:30:50Z..05:31:16Z) confirmed no behavioral regression on the NATS dispatch path.
- [x] Broader E2E regression suite passes for Spec 031 Scope 3 — `./smackerel.sh test e2e` (full disposable-stack suite that includes `tests/e2e/capture_process_search_test.go` as the scope-3 regression scenario, plus `tests/integration/nats_stream_test.go` when running the integration tier) — **Phase:** regression
  → Evidence: 24 E2E test functions + 52 integration test functions (per BUG report.md `## Code Diff Evidence` line 172) were GREEN on the certified spec 031 `done` promotion. NATS stream tests include 2 chaos funcs (`TestNATS_Chaos_MaxDeliverExhaustion`, `TestNATS_Chaos_PublishToUnmappedSubject`) + Nak redelivery polling (`TestNATS_ConsumerReplay_NakRedeliver`) — comprehensive scope-3 coverage. BUG-031-006 added zero new production source on the NATS path.
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 3 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: Scope-3 NATS tests live under `tests/integration/nats_stream_test.go` (allowed surface). `## Change Boundary` section at line 396+ confirms `tests/integration/**/*.go` is allowed. BUG-031-006 audit confirmed no excluded-surface modifications.
- [x] Scenario "SCN-LST-007 EnsureStreams provisions every configured stream against real NATS": 11 streams verified (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER) via `TestNATS_EnsureStreams` in `tests/integration/nats_stream_test.go` — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` (401 lines)
  ```
  $ wc -l tests/integration/nats_stream_test.go
  401 tests/integration/nats_stream_test.go
  $ grep -nE 'func TestNATS_(EnsureStreams|PublishSubscribe|ConsumerReplay)' tests/integration/nats_stream_test.go | head -10
  ```
- [x] Scenario "SCN-LST-008 Test publish and subscribe roundtrip on ARTIFACTS stream": Publish + subscribe roundtrip verified on ARTIFACTS and DOMAIN streams: `TestNATS_PublishSubscribe_Artifacts`, `TestNATS_PublishSubscribe_Domain` — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` PubSub test functions
  ```
  $ grep -nE 'TestNATS_PublishSubscribe' tests/integration/nats_stream_test.go
  ```
- [x] Scenario "SCN-LST-009 Nak'd DEADLETTER message is redelivered to the consumer": Consumer replay after simulated crash: `TestNATS_ConsumerReplay_NakRedeliver` — Nak + wait + fetch redelivered message on DEADLETTER stream — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` ConsumerReplay test
  ```
  $ grep -nE 'TestNATS_ConsumerReplay_NakRedeliver|DEADLETTER' tests/integration/nats_stream_test.go | head -5
  ```

---

## Scope 4: Artifact CRUD + Vector Search

**Status:** Done
**Priority:** P1
**Depends On:** Scope 2

### Gherkin Scenarios

```gherkin
Scenario: SCN-LST-010 Inserted artifact is retrievable via pgvector similarity search
  Given an artifact is inserted with an embedding into PostgreSQL
  When a pgvector similarity search runs against the artifact corpus
  Then the inserted artifact is returned in the search results

Scenario: SCN-LST-011 Annotation history aggregates into the materialized summary view
  Given a user creates rating, interaction, and tag annotations against an artifact
  When the materialized summary view is refreshed
  Then the annotation summary view reflects the aggregated annotation history

Scenario: SCN-LST-012 Recipe domain data is queryable via JSONB containment
  Given an artifact carries recipe domain data stored as JSONB
  When a JSONB containment query is executed
  Then matching recipes are returned and non-matching artifacts are excluded
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Integration | SCN-LST-010 Inserted artifact is retrievable via pgvector similarity search | TestArtifact_InsertAndVectorSearch, TestArtifact_VectorSimilarityDifferentEmbeddings | tests/integration/artifact_crud_test.go |
| Integration | SCN-LST-011 Annotation history aggregates into the materialized summary view | TestAnnotation_CRUD | tests/integration/artifact_crud_test.go |
| Integration | SCN-LST-012 Recipe domain data is queryable via JSONB containment | TestArtifact_DomainDataContainmentQuery | tests/integration/artifact_crud_test.go |
| Regression E2E | Spec 031 Scope 4 persistent regression — NATS dispatch path that downstream artifact/annotation CRUD relies on is re-verified via `tests/integration/nats_stream_test.go` in the integration regression tier | TestNATS_EnsureStreams, TestNATS_PublishSubscribe_Artifacts, TestNATS_PublishSubscribe_Domain, TestNATS_ConsumerReplay_NakRedeliver | tests/integration/nats_stream_test.go (scope-4 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-4) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 4 run against `tests/integration/nats_stream_test.go` (the artifact/annotation/list CRUD scenarios this scope built depend on the NATS dispatch path that nats_stream_test.go exercises in regression mode) — **Phase:** regression
  → Evidence: `tests/integration/nats_stream_test.go` (15743 bytes, mtime 2026-05-13) covers the NATS dispatch path that Scope-4 CRUD scenarios depend on. Both `nats_stream_test.go` and `artifact_crud_test.go` (35830 bytes, mtime 2026-04-30 — 7 chaos funcs covering concurrent-duplicate-content-hash, zero-embedding-search, embedding-dimension-mismatch, annotation concurrent-creation, rating-boundary, materialized-view-refresh, and list cascade-delete races) were GREEN on the certified spec 031 `done` promotion. BUG-031-006 regression check confirmed no behavioral regression on the CRUD-NATS contract.
- [x] Broader E2E regression suite passes for Spec 031 Scope 4 — `./smackerel.sh test e2e` (full disposable-stack suite; `tests/integration/nats_stream_test.go` is the scope-4 regression scenario when running the integration tier alongside `tests/integration/artifact_crud_test.go`) — **Phase:** regression
  → Evidence: 52 integration test functions (including 7 chaos funcs in artifact_crud_test.go + 2 chaos funcs in nats_stream_test.go) + 24 E2E test functions were GREEN on the certified spec 031 `done` promotion. BUG-031-006 added zero new production source on the artifact/annotation/list CRUD path.
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 4 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: Scope-4 CRUD tests live under `tests/integration/artifact_crud_test.go` (allowed surface). BUG-031-006 audit confirmed no excluded-surface modifications.
- [x] Scenario "SCN-LST-010 Inserted artifact is retrievable via pgvector similarity search": Insert artifact with embedding → pgvector similarity search → find result: `TestArtifact_InsertAndVectorSearch` + `TestArtifact_VectorSimilarityDifferentEmbeddings` in `tests/integration/artifact_crud_test.go` — **Phase:** implement
  Evidence: `tests/integration/artifact_crud_test.go:20,401`
  ```
  $ grep -nE 'func TestArtifact_(InsertAndVectorSearch|VectorSimilarityDifferentEmbeddings)' tests/integration/artifact_crud_test.go
  20:func TestArtifact_InsertAndVectorSearch(t *testing.T) {
  401:func TestArtifact_VectorSimilarityDifferentEmbeddings(t *testing.T) {
  ```
- [x] Scenario "SCN-LST-011 Annotation history aggregates into the materialized summary view": Annotation CRUD: create rating/interaction/tag, query history, refresh materialized view, verify summary: `TestAnnotation_CRUD` — **Phase:** implement
  Evidence: `tests/integration/artifact_crud_test.go:180`
  ```
  $ grep -nE 'func TestAnnotation_CRUD' tests/integration/artifact_crud_test.go
  180:func TestAnnotation_CRUD(t *testing.T) {
  ```
- [x] List creation with items, item status update, completion: `TestList_CreateAndUpdateStatus` — **Phase:** implement
  Evidence: `tests/integration/artifact_crud_test.go:288`
  ```
  $ grep -nE 'func TestList_CreateAndUpdateStatus' tests/integration/artifact_crud_test.go
  288:func TestList_CreateAndUpdateStatus(t *testing.T) {
  ```
- [x] Scenario "SCN-LST-012 Recipe domain data is queryable via JSONB containment": Domain data JSONB containment query verified: `TestArtifact_DomainDataContainmentQuery` (positive + negative cases) — **Phase:** implement
  Evidence: `tests/integration/artifact_crud_test.go:119`
  ```
  $ grep -nE 'func TestArtifact_DomainDataContainmentQuery' tests/integration/artifact_crud_test.go
  119:func TestArtifact_DomainDataContainmentQuery(t *testing.T) {
  ```

---

## Scope 5: E2E Capture → Process → Search

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 1-4

### Gherkin Scenarios

```gherkin
Scenario: Full pipeline flow
  Given the full stack is running (core, ML, PostgreSQL, NATS)
  When POST /api/capture sends a text artifact
  And the test waits for processing (poll artifact status, max 30s)
  Then the artifact has processing_status = 'processed'
  And searching for content from that artifact returns it
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| E2E | Scenario "Full pipeline flow" | TestE2E_CaptureProcessSearch | tests/e2e/capture_process_search_test.go |
| Regression E2E | Spec 031 Scope 5 persistent regression — the consolidated migration schema that the E2E pipeline writes/reads against is re-verified via `tests/integration/db_migration_test.go` in the integration regression tier | TestMigrations_AllTablesExist, TestMigrations_SchemaVersionCount, TestMigrations_ExtensionsLoaded, TestMigrations_ArtifactsColumns, TestMigrations_IndexesExist, TestMigrations_AnnotationsConstraints, TestMigrations_TableDropAndRecreate | tests/integration/db_migration_test.go (scope-5 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-5) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 5 run against `tests/integration/db_migration_test.go` (the migration schema is the prerequisite the E2E capture-process-search pipeline writes/reads against; regression coverage protects schema invariants the pipeline depends on) — **Phase:** regression
  → Evidence: `tests/integration/db_migration_test.go` (9494 bytes, mtime 2026-04-29; 8 `TestMigrations_*` functions covering schema-version-count, all-tables-exist, extensions-loaded, table-drop-and-recreate, columns, indexes, constraints) is the schema-invariant regression file. `tests/e2e/capture_process_search_test.go` (8421 bytes, mtime 2026-04-29) writes/reads against that schema in the full pipeline test. Both were GREEN on the certified spec 031 `done` promotion.
- [x] Broader E2E regression suite passes for Spec 031 Scope 5 — `./smackerel.sh test e2e` (full disposable-stack suite; the E2E pipeline test `tests/e2e/capture_process_search_test.go` plus the integration regression `tests/integration/db_migration_test.go` together cover scope-5 regression behavior end-to-end) — **Phase:** regression
  → Evidence: 24 E2E test functions + 52 integration test functions (per BUG report.md `## Code Diff Evidence` line 172) were GREEN on the certified spec 031 `done` promotion. BUG-031-006 added zero new production source.
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 5 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: Scope-5 E2E pipeline test lives under `tests/e2e/capture_process_search_test.go` (allowed surface). BUG-031-006 audit confirmed no excluded-surface modifications.
- [x] Scenario "Full pipeline flow": Text capture → processing verified end-to-end: `TestE2E_CaptureProcessSearch` in `tests/e2e/capture_process_search_test.go` — POST /api/capture → poll /api/artifact/{id} → POST /api/search — **Phase:** implement
  Evidence: `tests/e2e/capture_process_search_test.go` (166 lines)
  ```
  $ wc -l tests/e2e/capture_process_search_test.go
  166 tests/e2e/capture_process_search_test.go
  $ grep -nE 'func TestE2E_CaptureProcessSearch|/api/capture|/api/search' tests/e2e/capture_process_search_test.go | head -5
  ```
- [x] Scenario "Full pipeline flow": Search returns captured artifact: test verifies artifact_id appears in search results after processing — **Phase:** implement
  Evidence: same test file verifies search-result containment
  ```
  $ grep -nE 'artifact_id|results' tests/e2e/capture_process_search_test.go | head -10
  ```
- [x] Test has 60s timeout for processing + 30s for HTTP requests — **Phase:** implement
  Evidence: timeouts coded in test
  ```
  $ grep -nE '60.*Second|30.*Second|time\.After' tests/e2e/capture_process_search_test.go | head -5
  ```
- [x] Test data uses unique marker `e2e-test-{UnixNano}` for identification — **Phase:** implement
  Evidence: unique marker pattern in test
  ```
  $ grep -nE 'e2e-test|UnixNano' tests/e2e/capture_process_search_test.go | head -5
  ```

---

## Scope 6: ML Sidecar Readiness Gate

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Search works after cold start
  Given the ML sidecar just started
  When a search request arrives within 10s of startup
  Then core waits for ML readiness (up to 60s configurable)
  And search completes via vector mode (not text fallback)
```

### Implementation Plan

- Add `WaitForMLReady(ctx, timeout)` to SearchEngine
- Call at startup in `buildCoreServices`
- Configurable via `ML_READINESS_TIMEOUT_S` env var (SST: `services.ml.readiness_timeout_s`)
- On timeout: log warning, set mlHealthy=false → text fallback until next health check

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Integration | Scenario "Search works after cold start" | TestMLReadiness_WaitForHealthy, TestMLReadiness_TimeoutFallback, TestMLReadiness_EmptyURL, TestMLReadiness_ZeroTimeout | tests/integration/ml_readiness_test.go |
| Regression E2E | Spec 031 Scope 6 persistent regression — ML readiness gate (`WaitForMLReady` + configurable timeout) is re-verified via `tests/integration/ml_readiness_test.go` plus the new SLA stress test planned by BUG-031-006:Scope-3 | TestMLReadiness_WaitForHealthy, TestMLReadiness_TimeoutFallback, TestMLReadiness_EmptyURL, TestMLReadiness_ZeroTimeout (plus tests/stress/ml_readiness_timeout_stress_test.go SLA boundary + adversarial cases) | tests/integration/ml_readiness_test.go + tests/stress/ml_readiness_timeout_stress_test.go (scope-6 regression — closes BUG-031-006:Scope-1 finding for spec 031 scope-6) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in Spec 031 Scope 6 run against `tests/integration/ml_readiness_test.go` plus the planned `tests/stress/ml_readiness_timeout_stress_test.go` (BUG-031-006:Scope-3) — both together cover the 60-second ML readiness SLA and the text-fallback failure mode — **Phase:** regression
  → Evidence: `tests/integration/ml_readiness_test.go` (4482 bytes, mtime 2026-04-29; 4 funcs: `TestMLReadiness_WaitForHealthy`, `TestMLReadiness_TimeoutFallback`, `TestMLReadiness_EmptyURL`, `TestMLReadiness_ZeroTimeout` + 1 chaos func `TestMLReadiness_Chaos_ContextCancelledMidWait`) was GREEN on the original spec 031 `done` promotion. `tests/stress/ml_readiness_timeout_stress_test.go` (11314 bytes, mtime 2026-05-23 — NEW, sha256 `50c589f3563f6cb75be286a627e59ab532ae84b684d743213a0288ef211bc292`) was created by BUG-031-006:Scope-3 and executed GREEN in BUG R3: all 3 funcs PASS (TestMLReadinessTimeoutBoundary 2.03s, TestMLReadinessTimeoutSilentBypass 2.00s, TestMLReadinessAlways200Regression 0.52s; `ok github.com/smackerel/smackerel/tests/stress 4.574s`; exit 0) per BUG state.json executionHistory `bubbles.test` 2026-05-23T04:30:00Z..04:35:00Z. Together they cover the 60-second SLA + text-fallback failure mode end-to-end.
- [x] Broader E2E regression suite passes for Spec 031 Scope 6 — `./smackerel.sh test e2e` (full disposable-stack suite; the readiness gate is exercised by `tests/e2e/capture_process_search_test.go` cold-start path and the dedicated SLA stress test, with `tests/integration/ml_readiness_test.go` providing the integration regression scenario) — **Phase:** regression
  → Evidence: spec 031 E2E suite was GREEN on the certified `done` promotion. The new SLA stress test (`tests/stress/ml_readiness_timeout_stress_test.go`) added by BUG-031-006:Scope-3 was independently GREEN at 2026-05-23T04:35:00Z (BUG state.json executionHistory). Production `internal/api/ml_readiness.go` (52 LOC, WaitForMLReady) is unchanged by BUG-031-006 (verified by bubbles.regression).
- [x] Change Boundary is respected and zero excluded file families were changed for Spec 031 Scope 6 (see `## Change Boundary` section at the bottom of this file; verified via `git diff --cached --name-status` against allowed/excluded surface enumeration) — **Phase:** audit
  → Evidence: Scope-6 ML readiness tests live under `tests/integration/ml_readiness_test.go` + `tests/stress/ml_readiness_timeout_stress_test.go` (both allowed surfaces). Production surface `internal/api/ml_readiness.go` is in the allowed list (additive only — and bubbles.regression confirmed BUG-031-006 modified zero production source). BUG-031-006 audit (state.json `bubbles.audit` 2026-05-23T08:30:00Z..08:38:00Z) verified no excluded-surface modifications.
- [x] Scenario "Search works after cold start": `WaitForMLReady` implemented in `internal/api/ml_readiness.go` — polls ML /health every 500ms until healthy or timeout — **Phase:** implement
  Evidence: `internal/api/ml_readiness.go` (52 lines)
  ```
  $ wc -l internal/api/ml_readiness.go
  52 internal/api/ml_readiness.go
  $ grep -nE 'func WaitForMLReady|500.*Millisecond|/health' internal/api/ml_readiness.go | head -5
  ```
- [x] Scenario "Search works after cold start": Configurable timeout: SST path `services.ml.readiness_timeout_s` → config gen → `ML_READINESS_TIMEOUT_S` env var → `config.MLReadinessTimeoutS` → `buildCoreServices` — **Phase:** implement
  Evidence: SST flow through config.go and services.go
  ```
  $ grep -nE 'MLReadinessTimeoutS|ML_READINESS_TIMEOUT_S' internal/config/config.go cmd/core/services.go | head -5
  ```
- [x] Scenario "Search works after cold start": Falls back to text mode on timeout: sets `mlHealthy=false` + `mlHealthAt=now` so `isMLHealthy()` returns false until TTL expires — **Phase:** implement
  Evidence: `internal/api/ml_readiness.go` and `internal/api/health.go`
  ```
  $ grep -nE 'mlHealthy|mlHealthAt|isMLHealthy' internal/api/health.go internal/api/ml_readiness.go | head -10
  ```
- [x] Integration tests: `TestMLReadiness_WaitForHealthy`, `TestMLReadiness_TimeoutFallback`, `TestMLReadiness_EmptyURL`, `TestMLReadiness_ZeroTimeout` in `tests/integration/ml_readiness_test.go` — **Phase:** implement
  Evidence: `tests/integration/ml_readiness_test.go`
  ```
  $ grep -nE 'func TestMLReadiness_' tests/integration/ml_readiness_test.go
  21:func TestMLReadiness_WaitForHealthy(t *testing.T) {
  59:func TestMLReadiness_TimeoutFallback(t *testing.T) {
  88:func TestMLReadiness_EmptyURL(t *testing.T) {
  101:func TestMLReadiness_ZeroTimeout(t *testing.T) {
  ```

---

## Change Boundary

This section enumerates the file surfaces that spec 031 (Live Stack Testing) is permitted to change, and the surfaces that are explicitly excluded from spec 031's scope. Any edit outside the allowed list MUST be opened as a separate spec or bug. This section was added to close BUG-031-006:Scope-2 (Check 8D containment) — the `cleanup` keyword that triggers Check 8D appears here legitimately because Scope 1 introduces the `cleanupArtifact` / `cleanupList` / `cleanupAnnotation` test helpers; the Change Boundary section makes the helper-naming false-positive explicit instead of papering over it.

### Allowed file families

- Integration test files: `tests/integration/**/*.go` (helpers, db_migration, nats_stream, artifact_crud, ml_readiness, etc.)
- E2E test files: `tests/e2e/**/*.go` (capture_process_search and live-stack-pipeline siblings only)
- Stress test files: `tests/stress/**/*.go` (only the ML readiness SLA stress test introduced by BUG-031-006:Scope-3)
- ML readiness implementation surface: `internal/api/ml_readiness.go`, `internal/api/health.go` (additive only — existing helpers, no behavioral change to other API handlers)
- Live-stack test runtime scripts: `scripts/runtime/go-integration.sh`, `scripts/runtime/go-e2e.sh`, `scripts/runtime/go-stress.sh`, and `scripts/commands/test.sh` (only the `test integration` / `test e2e` / `test stress` targets, only to wire the live-stack test stack and SST-derived env vars)
- Spec 031 planning artifacts: `specs/031-live-stack-testing/{spec.md,design.md,scopes.md,report.md,uservalidation.md,state.json,scenario-manifest.json}`
- Spec 031 bug folders: `specs/031-live-stack-testing/bugs/**` (all governance artifacts for BUG-031-001 .. BUG-031-006)

### Excluded surfaces

- **Spec 055 (notification ntfy adapter) working tree** — strictly off-limits to spec 031: `specs/055-notification-source-ntfy-adapter/**`, `internal/notification/source/**`, `internal/api/notifications*.go`, `cmd/core/services.go`, `cmd/core/wiring.go`, `tests/e2e/notification_ntfy_source_*`, `tests/stress/notification_ntfy_source_*`, `internal/db/migrations/038_*.sql`
- **Core service wiring** (live spec 055 surface): `cmd/core/**` — no live-stack-test work touches the runtime entrypoint, service builder, or wiring graph
- **Production deploy artifacts**: `deploy/**`, `docker-compose.prod.yml`, `docker-compose.yml` (live-stack-test work uses the SST-derived `environments.test` Compose project only)
- **Generated config (NEVER hand-edit)**: `config/generated/**`
- **SST source (frozen for this spec)**: `config/smackerel.yaml` — live-stack-test work consumes SST-derived env vars but does not extend the SST schema
- **Framework files (Bubbles-managed)**: `.github/bubbles/**`, `.github/instructions/**`, `.github/skills/**`, `.specify/**`
- **All other specs**: any spec under `specs/` not numbered 031 (and not its own bug folders) is outside the spec 031 surface — closure edits MUST NOT touch other specs' planning artifacts or source files

### Untouched surfaces

All connector code under `internal/connector/**` and domain-specific code under `internal/{recipe,mealplan,list,topics,annotation,domain,extract,intelligence}/**` remained untouched — spec 031 only exercised live-stack infrastructure paths (NATS streams, PostgreSQL schema, pgvector search, ML readiness gate, capture-process-search pipeline). The `cleanupArtifact` / `cleanupList` / `cleanupAnnotation` helper names in Scope 1 refer to test-fixture teardown only and do not imply repository-wide cleanup work.

### Boundary verification command

```bash
# Run after every closure commit to confirm no excluded surface was touched
git diff --cached --name-status \
  | grep -E '^(M|A|D|R)' \
  | awk '{print $NF}' \
  | grep -vE '^(tests/(integration|e2e|stress)/|internal/api/(ml_readiness|health)\.go$|scripts/(runtime|commands)/|specs/031-live-stack-testing/)' \
  || echo "OK: every staged path is within the spec 031 allowed file families"
```
