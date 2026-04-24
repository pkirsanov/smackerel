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

### Definition of Done

- [x] Test stack isolation via SST config (`environments.test` in `config/smackerel.yaml`): ports 47001-47004, volumes `smackerel-test-*`, project `smackerel-test` — **Phase:** implement
  Evidence: `config/smackerel.yaml` environments.test section
  ```
  $ grep -nE '^  test:|smackerel-test|47001|47004' config/smackerel.yaml | head -10
  ```
- [x] `./smackerel.sh test integration` starts test stack, runs shell health check, then runs Go integration tests with `--network host` and SST-derived env vars — **Phase:** implement
  Evidence: `scripts/commands/test.sh` integration target
  ```
  $ grep -nE 'test integration|integration_test|--network host' scripts/commands/test.sh | head -5
  ```
- [x] Test cleanup via `t.Cleanup()` and `cleanupArtifact`/`cleanupList`/`cleanupAnnotation` helpers in `tests/integration/helpers_test.go` — **Phase:** implement
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

### Definition of Done

- [x] All consolidated migrations verified: `TestMigrations_SchemaVersionCount` checks >= 3 (001, 018, 019), `TestMigrations_AllTablesExist` verifies 12 tables, `TestMigrations_ExtensionsLoaded` verifies vector + pg_trgm in `tests/integration/db_migration_test.go` — **Phase:** implement
  Evidence: `tests/integration/db_migration_test.go` (305 lines)
  ```
  $ wc -l tests/integration/db_migration_test.go
  305 tests/integration/db_migration_test.go
  $ grep -nE 'func TestMigrations_(AllTablesExist|SchemaVersionCount|ExtensionsLoaded)' tests/integration/db_migration_test.go
  15:func TestMigrations_AllTablesExist(t *testing.T) {
  115:func TestMigrations_ExtensionsLoaded(t *testing.T) {
  135:func TestMigrations_SchemaVersionCount(t *testing.T) {
  ```
- [x] Schema DDL resilience tested: `TestMigrations_TableDropAndRecreate` drops lists/list_items, verifies other tables unaffected, recreates via fresh DDL in `tests/integration/db_migration_test.go` — **Phase:** implement
  Evidence: `tests/integration/db_migration_test.go:162`
  ```
  $ grep -nE 'func TestMigrations_TableDropAndRecreate' tests/integration/db_migration_test.go
  162:func TestMigrations_TableDropAndRecreate(t *testing.T) {
  ```
- [x] Table/column/index checks: `TestMigrations_ArtifactsColumns` (21 columns), `TestMigrations_IndexesExist` (11 indexes), `TestMigrations_AnnotationsConstraints` (chk_rating_range) — **Phase:** implement
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

### Definition of Done

- [x] 11 streams verified (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER) via `TestNATS_EnsureStreams` in `tests/integration/nats_stream_test.go` — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` (401 lines)
  ```
  $ wc -l tests/integration/nats_stream_test.go
  401 tests/integration/nats_stream_test.go
  $ grep -nE 'func TestNATS_(EnsureStreams|PublishSubscribe|ConsumerReplay)' tests/integration/nats_stream_test.go | head -10
  ```
- [x] Publish + subscribe roundtrip verified on ARTIFACTS and DOMAIN streams: `TestNATS_PublishSubscribe_Artifacts`, `TestNATS_PublishSubscribe_Domain` — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` PubSub test functions
  ```
  $ grep -nE 'TestNATS_PublishSubscribe' tests/integration/nats_stream_test.go
  ```
- [x] Consumer replay after simulated crash: `TestNATS_ConsumerReplay_NakRedeliver` — Nak + wait + fetch redelivered message on DEADLETTER stream — **Phase:** implement
  Evidence: `tests/integration/nats_stream_test.go` ConsumerReplay test
  ```
  $ grep -nE 'TestNATS_ConsumerReplay_NakRedeliver|DEADLETTER' tests/integration/nats_stream_test.go | head -5
  ```

---

## Scope 4: Artifact CRUD + Vector Search

**Status:** Done
**Priority:** P1
**Depends On:** Scope 2

### Definition of Done

- [x] Insert artifact with embedding → pgvector similarity search → find result: `TestArtifact_InsertAndVectorSearch` + `TestArtifact_VectorSimilarityDifferentEmbeddings` in `tests/integration/artifact_crud_test.go` — **Phase:** implement
  Evidence: `tests/integration/artifact_crud_test.go:20,401`
  ```
  $ grep -nE 'func TestArtifact_(InsertAndVectorSearch|VectorSimilarityDifferentEmbeddings)' tests/integration/artifact_crud_test.go
  20:func TestArtifact_InsertAndVectorSearch(t *testing.T) {
  401:func TestArtifact_VectorSimilarityDifferentEmbeddings(t *testing.T) {
  ```
- [x] Annotation CRUD: create rating/interaction/tag, query history, refresh materialized view, verify summary: `TestAnnotation_CRUD` — **Phase:** implement
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
- [x] Domain data JSONB containment query verified: `TestArtifact_DomainDataContainmentQuery` (positive + negative cases) — **Phase:** implement
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

### Definition of Done

- [x] Text capture → processing verified end-to-end: `TestE2E_CaptureProcessSearch` in `tests/e2e/capture_process_search_test.go` — POST /api/capture → poll /api/artifact/{id} → POST /api/search — **Phase:** implement
  Evidence: `tests/e2e/capture_process_search_test.go` (166 lines)
  ```
  $ wc -l tests/e2e/capture_process_search_test.go
  166 tests/e2e/capture_process_search_test.go
  $ grep -nE 'func TestE2E_CaptureProcessSearch|/api/capture|/api/search' tests/e2e/capture_process_search_test.go | head -5
  ```
- [x] Search returns captured artifact: test verifies artifact_id appears in search results after processing — **Phase:** implement
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

### Definition of Done

- [x] `WaitForMLReady` implemented in `internal/api/ml_readiness.go` — polls ML /health every 500ms until healthy or timeout — **Phase:** implement
  Evidence: `internal/api/ml_readiness.go` (52 lines)
  ```
  $ wc -l internal/api/ml_readiness.go
  52 internal/api/ml_readiness.go
  $ grep -nE 'func WaitForMLReady|500.*Millisecond|/health' internal/api/ml_readiness.go | head -5
  ```
- [x] Configurable timeout: SST path `services.ml.readiness_timeout_s` → config gen → `ML_READINESS_TIMEOUT_S` env var → `config.MLReadinessTimeoutS` → `buildCoreServices` — **Phase:** implement
  Evidence: SST flow through config.go and services.go
  ```
  $ grep -nE 'MLReadinessTimeoutS|ML_READINESS_TIMEOUT_S' internal/config/config.go cmd/core/services.go | head -5
  ```
- [x] Falls back to text mode on timeout: sets `mlHealthy=false` + `mlHealthAt=now` so `isMLHealthy()` returns false until TTL expires — **Phase:** implement
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
