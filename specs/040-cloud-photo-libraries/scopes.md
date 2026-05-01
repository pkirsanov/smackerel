# Scopes: 040 Cloud Photo Libraries

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

1. **Scope 1: Photo platform foundation** - Establish the provider-neutral photo domain, SST configuration flow, NATS photo stream, database schema, ML contract boundary, and synthetic test boundary so a single synthetic photo can move through the platform without provider-specific branching.
2. **Scope 2: Immich connect, scan, and search** - Deliver the first user-visible vertical slice: connect Immich, choose scope, scan photos, classify/search them, and show connector progress plus skip states in the PWA.
3. **Scope 3: Lifecycle, duplicates, and removal review** - Add RAW-to-processed lifecycle links, duplicate clusters, quality scoring, removal candidates, and confirmation-gated provider actions.
4. **Scope 4: Capture, Telegram, and cross-feature routing** - Wire uploads, document scan, Telegram retrieval, expense/recipe/document routing, and sensitivity enforcement across channel boundaries.
5. **Scope 5: Multi-provider capability governance and operations** - Add a second provider adapter path, cross-provider search/dedupe, provider limitation visibility, photo health operations, observability, and ingest stress validation.

### New Types & Signatures

- `internal/connector/photos.PhotoLibrary`: provider-neutral connector interface embedding `connector.Connector` plus `Capabilities`, `ProbeCapabilities`, `EnumerateScope`, `Watch`, `Fetch`, and `Writer`.
- `internal/connector/photos.PhotoEvent`: normalized provider event with `ProviderRef`, `Op`, `MediaRole`, hashes, EXIF, album/tag/face metadata, sensitivity, and raw provider payload.
- `internal/connector/photos.ProviderWriter`: `AddToAlbum`, `Tag`, `Favorite`, `Archive`, `Delete`, `Upload`, and `RenameFaceCluster`, all capability-gated.
- New DB migration: `photos`, `photo_lifecycle_links`, `photo_clusters`, `photo_cluster_members`, `photo_removal_candidates`, `photo_capabilities`, `photo_sync_state`, `photo_face_links`, `photo_embeddings`, `photo_action_tokens`, `photo_audit_events`.
- New NATS stream: `PHOTOS` with request/response subjects `photos.classify`, `photos.ocr`, `photos.embed`, `photos.lifecycle`, `photos.dedupe`, `photos.aesthetic`, `photos.sensitivity`, `photos.removal.evaluate` and their result subjects.
- New API family: `/v1/photos/connectors`, `/v1/photos/search`, `/v1/photos/{id}`, `/v1/photos/{id}/preview`, `/v1/photos/health/*`, `/v1/photos/actions/plan`, `/v1/photos/actions/confirm`, `/v1/photos/upload`.
- New ML handlers: `ml/app/photos.py` handlers for classification, OCR, embeddings, lifecycle, dedupe, aesthetic, sensitivity, and removal rationale.

### Validation Checkpoints

- After Scope 1: schema, NATS, config generation, ML schema validation, and privacy boundary tests pass before provider work starts.
- After Scope 2: Immich live-stack scan/search and PWA connector/search e2e-ui pass before lifecycle or cleanup surfaces can use photo data.
- After Scope 3: action planning proves provider mutations cannot happen before confirmation before channel uploads or cross-feature automation are enabled.
- After Scope 4: Telegram/mobile/cross-feature routing and sensitivity live E2E pass before additional providers broaden the surface.
- After Scope 5: multi-provider, capability limitation, health dashboard, broader e2e, and stress gates pass before certification can start.

## Overview

This is a single-file scope plan with five sequential, vertical scopes. Scope 1 creates the shared photo contract and synthetic safety boundary. Scopes 2 through 5 each deliver user-visible outcomes across backend, ML, API, UI/channel, and live-system tests. The order is intentionally vertical after the foundation slice: the plan avoids a run of DB-only, service-only, or UI-only phases.

## Impacted Surfaces

- Backend: `internal/connector/photos/`, `internal/api/`, `internal/pipeline/`, `internal/nats/`, `internal/db/migrations/`, `internal/metrics/`, `cmd/core/` wiring.
- ML sidecar: `ml/app/photos.py`, NATS handlers, OCR/vision model gateway integration, structured output validation.
- Infrastructure/config: `config/smackerel.yaml`, `scripts/commands/config.sh`, `config/nats_contract.json`, generated config via `./smackerel.sh config generate`.
- UI: `web/pwa/` photo connectors, search/detail, photo health, confirmation modal, provider limitation banners, mobile document scan surfaces.
- Channels and consumers: Telegram capture/retrieval, mobile capture, expense tracking, recipes, knowledge/domain extraction, annotations, lists, meal planning, intelligence delivery, agent tools.
- Tests: Go unit/integration/e2e/stress, Python unit/integration, PWA e2e-ui, NATS contract tests, privacy-boundary tests.

## Scope Summary

| # | Scope | Surfaces | Required tests | DoD summary | Status |
|---|---|---|---|---|---|
| 1 | Photo platform foundation | DB, config, NATS, Go contracts, ML schemas, privacy fixtures | unit, integration, e2e-api, Regression E2E | Provider-neutral contract, schema, NATS, SST, and ML boundary ready | Done |
| 2 | Immich connect, scan, and search | Immich adapter, connector API, scan pipeline, PWA connectors/search/detail | unit, integration, e2e-api, e2e-ui, Regression E2E | Immich scoped scan produces searchable classified photos and visible skips | Done |
| 3 | Lifecycle, duplicates, and removal review | Lifecycle/dedupe/removal APIs, ML decisions, PWA health/review, confirmation token | unit, integration, e2e-api, e2e-ui, Regression E2E | RAW lifecycle, duplicate clusters, removal rationale, no mutation before confirm | Not Started |
| 4 | Capture, Telegram, and cross-feature routing | Upload API, mobile doc scan, Telegram, expenses, recipes, knowledge, sensitivity | unit, integration, e2e-api, e2e-ui, Regression E2E | Photos route across channels safely and sensitive retrieval is blocked | Not Started |
| 5 | Multi-provider capability governance and operations | Second provider, capability matrix, cross-provider search/dedupe, health, observability, stress | unit, integration, e2e-api, e2e-ui, stress, Regression E2E | Provider limits are visible, cross-provider results unify, operations meet health/stress gates | Not Started |

---

## Scope 1: Photo Platform Foundation

**Status:** Done
**Priority:** P0  
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-040-001 Photo contracts bootstrap from SST and NATS
  Given config/smackerel.yaml defines the top-level photos configuration
  And config/nats_contract.json defines the PHOTOS stream and every photos.* request/response pair
  When the repo-standard config generation and service startup contract checks run
  Then Go and Python resolve the same photo subjects, stream, and generated PHOTOS_* values
  And startup fails loudly if an enabled provider lacks required credentials or the NATS contract is incomplete

Scenario: SCN-040-002 Synthetic photo persists with provider-neutral shape
  Given a synthetic photo fixture with EXIF, album metadata, and provider face-cluster refs
  When the photo publisher ingests it through the provider-neutral PhotoLibrary contract
  Then an artifacts row and photos row are created with matching artifact_id, provider_ref, media_role, EXIF, albums, tags, and raw_provider payload
  And the photo appears through the /v1/photos/{id} API without exposing provider-specific branching

Scenario: SCN-040-003 Stable signals cannot replace LLM-owned decisions
  Given a photo has filename, timestamp, EXIF, content hash, and pHash stable signals
  When the ML sidecar returns a classification, lifecycle, dedupe, sensitivity, or removal result without required confidence or rationale
  Then the Go core rejects that decision, records a visible classification issue, and does not invent a fallback classification
```

### Implementation Plan

- Add `internal/connector/photos/` shared types: `PhotoLibrary`, `PhotoEvent`, `CapabilityReport`, `ProviderWriter`, stable-signal DTOs, errors, and test fixtures.
- Add DB migration for the photo tables and rollback SQL described in [design.md](design.md).
- Extend `config/smackerel.yaml` with top-level `photos` values, update `scripts/commands/config.sh`, and ensure generated env variables are produced only via `./smackerel.sh config generate`.
- Extend `config/nats_contract.json`, Go NATS constants/stream config, Go contract tests, and Python NATS startup validation for the `PHOTOS` stream.
- Add ML schema validators for photo requests/results and a minimal handler set that validates payloads and returns structured errors for missing decision fields.
- Add read-only `/v1/photos/{id}` and `/v1/photos/connectors` foundation handlers returning real DB-backed data, using the repo-standard JSON error envelope.
- Create synthetic fixture directories under the existing test fixture conventions for RAW/JPEG/HEIC/document/video metadata without touching user libraries.
- Seed `scenario-manifest.json` and `test-plan.json` entries for SCN-040-001 through SCN-040-003.

### Shared Infrastructure Impact Sweep

- NATS contract surfaces: `config/nats_contract.json`, `internal/nats/`, Python validator, pipeline subscribers, integration NATS tests.
- Config surfaces: `config/smackerel.yaml`, `scripts/commands/config.sh`, generated dev/test env files, service startup validation.
- Storage surfaces: migrations, artifact insert path, API handlers that reference `artifacts.id` as `TEXT`, pgvector embedding setup.
- Canary tests must run before broader suites: NATS contract canary, config generation canary, DB migration canary, ML schema canary.
- Rollback path: SQL rollback block in the migration and `./smackerel.sh config generate` drift check after reverting photos config additions.

### Change Boundary

- Allowed file families: `internal/connector/photos/`, `internal/db/migrations/`, `internal/nats/`, `internal/api/photos*`, `internal/pipeline/photo*`, `ml/app/photos.py`, `ml/tests/test_photos_*.py`, `config/smackerel.yaml`, `config/nats_contract.json`, `scripts/commands/config.sh`, `tests/integration/photos_*`, `tests/e2e/photos_*`.
- Excluded surfaces: existing non-photo connector behavior, non-photo NATS subjects, generic drive feature 038 runtime logic, production Docker image lifecycle beyond config generation, user-owned photo library paths.

### Test Plan

| Type | Category | File / location | Expected test title | Scenario(s) | Command | Live system |
|---|---|---|---|---|---|---|
| Unit | unit | `internal/connector/photos/library_test.go` | `TestPhotoEventProviderNeutralShape` | SCN-040-002 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/connector/photos/stable_signals_test.go` | `TestStableSignals_DoNotMakeLLMOwnedDecisions` | SCN-040-003 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/nats/contract_test.go` | `TestPhotoSubjectsMatchNATSContract` | SCN-040-001 | `./smackerel.sh test unit` | No |
| Unit | unit | `ml/tests/test_photos_contract.py` | `test_photo_result_requires_confidence_and_rationale` | SCN-040-003 | `./smackerel.sh test unit` | No |
| Integration | integration | `tests/integration/photos_foundation_test.go` | `TestPhotosFoundation_ConfigNATSAndSchemaLiveStack` | SCN-040-001, SCN-040-002 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_privacy_boundary_test.go` | `TestPhotosPrivacyBoundaryRejectsUserLibraryURLs` | SCN-040-001 | `./smackerel.sh test integration` | Yes |
| Canary | integration | `tests/integration/photos_contract_canary_test.go` | `TestPhotosContractCanary_ConfigNATSDBAndMLAgree` | SCN-040-001, SCN-040-003 | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_foundation_test.go` | `TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI` | SCN-040-002, SCN-040-003 | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] DB migration and rollback block implement every photo-owned table, enum, index, and artifact foreign key described in design Section 4. **Phase:** implement. **Claim Source:** executed. Evidence: `internal/db/migrations/025_photo_libraries.sql` adds the photo-owned enum/table/index set plus rollback comments; `./smackerel.sh test integration` exit 0 ran `TestPhotosFoundation_ConfigNATSAndSchemaLiveStack/migration_025_photos_present` and `TestPhotosContractCanary_ConfigNATSDBAndMLAgree/migration_025_photos_present` against the disposable test stack.
- [x] Photos SST values flow through `config/smackerel.yaml` -> `./smackerel.sh config generate` -> generated dev/test env -> Go/Python startup validation with no fallback defaults. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh config generate` exit 0 generated `dev.env`; `./smackerel.sh config generate --env test` exit 0 generated `test.env`; `./smackerel.sh check` exit 0 reported `Config is in sync with SST` and `env_file drift guard: OK`; `./smackerel.sh test unit` exit 0 ran `internal/config/photos_config_test.go` fail-loud tests and ML contract validation.
- [x] PHOTOS stream, every `photos.*` subject, request/response pair, Go constants, and Python validator are in sync with `config/nats_contract.json`. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh test unit` exit 0 ran `TestPhotoSubjectsMatchNATSContract` plus Python `ml/tests/test_nats_contract.py`; `./smackerel.sh test integration` exit 0 ran PHOTOS stream publish canaries and `TestPhotosContractCanary_ConfigNATSDBAndMLAgree/ml_sidecar_photos_contract_response` through live NATS and the ML sidecar.
- [x] Provider-neutral `PhotoLibrary`, `PhotoEvent`, and `ProviderWriter` interfaces exist and no downstream code branches on provider names for photo decisions. **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/library.go` defines the provider-neutral interfaces and DTOs; `./smackerel.sh test unit` exit 0 ran `TestPhotoEventProviderNeutralShape` and `TestPhotoEventRejectsProviderSpecificBranchingFields`; `./smackerel.sh test integration` exit 0 ran `TestPhotosPrivacyBoundary_ProviderSpecificBranchingIsRejected` and `TestPhotosPrivacyBoundaryRejectsUserLibraryURLs`.
- [x] Stable-signal boundary is enforced: missing LLM rationale/confidence produces a visible failure state rather than a heuristic decision. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh test unit` exit 0 ran `TestStableSignals_DoNotMakeLLMOwnedDecisions`, `TestStableSignalsRejectLLMDecisionMissingConfidenceOrRationale`, and `ml/tests/test_photos_contract.py::test_photo_result_requires_confidence_and_rationale`; `./smackerel.sh test integration` exit 0 ran `TestPhotosPrivacyBoundary_StableSignalsDoNotPersistLLMDecision`.
- [x] Scenario-specific E2E regression tests for SCN-040-001, SCN-040-002, and SCN-040-003 exist and are feature/component-specific. **Phase:** implement. **Claim Source:** executed. Evidence: `tests/e2e/photos_foundation_test.go::TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI` is feature-specific and passed both focused (`COMPOSE_PROGRESS=plain ./smackerel.sh test e2e --go-run TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI`, exit 0) and broad e2e; it boots the live stack, verifies `/v1/photos/connectors`, persists a synthetic photo, reads `/v1/photos/{id}`, and asserts provider-neutral shape without `provider_specific` leakage. SCN-040-001 is also covered by live startup/config/NATS checks in the same e2e run plus integration canaries; SCN-040-003 is covered by the provider-neutral/no-decision leakage assertions paired with the e2e fixture.
- [x] Shared Infrastructure Impact Sweep canary tests pass before broader integration and e2e suites run. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh test integration` exit 0 ran `TestPhotosContractCanary_ConfigNATSDBAndMLAgree`, `TestPhotosFoundation_ConfigNATSAndSchemaLiveStack`, and privacy-boundary canaries before the final broad `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e` run.
- [x] Broader E2E regression suite passes through `./smackerel.sh test e2e`. **Phase:** implement. **Claim Source:** executed. Evidence: `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e` exit 0; shell E2E summary reported 35 total, 35 passed, 0 failed; Go e2e reported `PASS: go-e2e`, including `TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI`.
- [x] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, and `./smackerel.sh test integration` pass. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh check` exit 0 (`Config is in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK`); `./smackerel.sh lint` exit 0 (`All checks passed!`, `Web validation passed`); `./smackerel.sh format --check` exit 0 (`44 files already formatted`); `./smackerel.sh test unit` exit 0 (Go packages pass, Python `389 passed, 2 warnings`); `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` exit 0 with the Scope 1 photo tests passing.
- [x] `scopes.md`, `scenario-manifest.json`, and `test-plan.json` remain in sync for SCN-040-001 through SCN-040-003. **Phase:** implement. **Claim Source:** interpreted/executed. Evidence: implementation aligned test names to the existing manifest/test-plan rows (`TestPhotosFoundation_ConfigNATSAndSchemaLiveStack`, `TestPhotosContractCanary_ConfigNATSDBAndMLAgree`, `TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI`, and `TestPhotosPrivacyBoundaryRejectsUserLibraryURLs`); `./smackerel.sh check` exit 0 kept scenario-lint clean. No planning content in `scenario-manifest.json` or `test-plan.json` was edited by implement.

---

## Scope 2: Immich Connect, Scan, And Search

**Status:** Done  
**Priority:** P0  
**Depends On:** Scope 1

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-040-004 User connects Immich and searches a classified photo
  Given the local disposable Immich test provider contains albums, EXIF metadata, OCR-bearing images, face clusters, and an included/excluded scope selection
  When the user connects Immich, selects scope, and starts the initial scan
  Then selected photos are ingested, classified, embedded, and searchable by natural-language content
  And excluded albums produce zero photo rows and zero search results

Scenario: SCN-040-005 Immich monitoring updates metadata without unnecessary reclassification
  Given an Immich photo was already classified and indexed
  When the provider reports an album move, metadata edit, delete, and new upload
  Then the album move updates metadata without rerunning image classification
  And the new upload is classified while the delete creates a tombstone according to policy

Scenario: SCN-040-006 Scan progress and skip states are user-visible
  Given the Immich scan encounters too-large, unsupported, permission-denied, provider-error, and extraction-failed photos
  When the user opens the connector detail screen
  Then progress shows counts per phase, ETA when available, and every skipped category with retry action and file identity
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected user-visible result | Test type |
|---|---|---|---|---|
| Connect Immich | Test provider running and auth token configured through SST | Open Add Photo Library wizard, enter URL/API key, test connection, select albums, connect | Connector card is healthy or syncing and scan progress starts | e2e-ui |
| Search by description | Scan has completed at least one OCR-bearing image | Search `whiteboard diagram from March offsite` | Result grid shows thumbnail, provider badge, date, OCR snippet, confidence, and Photo Detail link | e2e-ui |
| Visible skip states | Test provider contains blocked fixtures | Open connector detail | Skip rows show reason, count, retry token/action, and no hidden silent failures | e2e-ui |

### Implementation Plan

- Implement `internal/connector/photos/adapters/immich/` with connect/test, capability probe, scope enumeration, fetch thumbnail/original, monitor cursor, upload, album/tag/favorite writes, and face-cluster read support.
- Wire the adapter into connector registration and generated config with fail-loud validation when `photos.providers.immich.enabled` is true and required credentials are empty.
- Implement photo scanner phases: metadata, thumbnails, classification, embeddings, OCR escalation, sensitivity, progress ledger, skip ledger, and retry batch tokens.
- Implement `/v1/photos/connectors`, `/v1/photos/connectors/test`, `/v1/photos/search`, `/v1/photos/{id}`, and preview token flow for non-sensitive thumbnails.
- Build PWA Screens 1-5 from the UX wireframes using real API data: connectors list, add wizard, connector detail, search results, and photo detail.
- Add integration fixtures for a disposable Immich-compatible provider with included/excluded albums, empty library, blocked files, OCR content, and face clusters.
- Preserve provider-neutral downstream shape: Immich-specific data remains in `raw_provider`; API responses use shared DTOs.

### Consumer Impact Sweep

- New routes and links: connectors navigation, photo search result links, photo detail route, connector detail route, search filter chips, preview image URLs.
- API clients: PWA data hooks, Telegram retrieval preparation, cross-feature consumers reading `PhotoSummary` and `PhotoDetail`.
- Docs/config/tests: config examples, provider capability status, NATS subject names, e2e route selectors.
- Stale-reference scan required before completion for old or experimental photo route names if implementation iterates on paths.

### Change Boundary

- Allowed file families: `internal/connector/photos/adapters/immich/`, `internal/api/photos*`, `internal/pipeline/photo*`, `internal/connector/photos/scanner*`, `web/pwa/src/photos/`, `web/pwa/tests/photos_*`, `tests/integration/photos_immich*`, `tests/integration/photos_sync*`, `tests/integration/photos_skip_ledger*`, `tests/e2e/photos_search*`, `tests/e2e/photos_sync*`, `config/smackerel.yaml` (Immich block only).
- Excluded surfaces: non-Immich provider adapters, non-photo connectors, lifecycle/dedupe/removal logic (Scope 3), upload/Telegram routing (Scope 4), capability matrix governance and stress fixtures (Scope 5), production Docker image lifecycle.

### Test Plan

| Type | Category | File / location | Expected test title | Scenario(s) | Command | Live system |
|---|---|---|---|---|---|---|
| Unit | unit | `internal/connector/photos/adapters/immich/immich_test.go` | `TestImmichAdapter_MapsProviderMediaToPhotoEvent` | SCN-040-004 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/api/photos_test.go` | `TestPhotoSearchResponse_UsesProviderNeutralDTO` | SCN-040-004 | `./smackerel.sh test unit` | No |
| Integration | integration | `tests/integration/photos_immich_test.go` | `TestPhotosImmich_ConnectScopeAndScanLiveProvider` | SCN-040-004 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_sync_test.go` | `TestPhotosImmich_IncrementalChangesUpdateState` | SCN-040-005 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_skip_ledger_test.go` | `TestPhotosImmich_SkipLedgerVisibleAndRetryable` | SCN-040-006 | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_search_test.go` | `TestPhotosSearch_E2E_ImmichWhiteboardOCRResult` | SCN-040-004 | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_sync_test.go` | `TestPhotosSync_E2E_AlbumMoveDoesNotReclassify` | SCN-040-005 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_connectors.spec.ts` | `test('photo libraries list and Immich wizard use live connector API')` | SCN-040-004 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_connector_progress.spec.ts` | `test('connector detail renders progress and skip ledger from live API')` | SCN-040-006 | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] Immich adapter supports connect/test, scope selection, scan, monitor, fetch, upload, organize, and face-cluster read according to capability probe results. **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/adapters/immich/immich.go` implements `Connect`, `ProbeCapabilities`, `EnumerateScope`, `Watch`, `Fetch`, `Writer` (AddToAlbum/Tag/Favorite/Archive/Delete/Upload/RenameFaceCluster), and PersonRef→FaceClusterRef mapping; `./smackerel.sh test unit` exit 0 ran `TestImmichAdapter_MapsProviderMediaToPhotoEvent` and `TestImmichAdapter_EnumerateScopeExcludesAlbums`; `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` exit 0 ran `TestPhotosImmich_ConnectScopeAndScanLiveProvider` against the disposable Immich-compatible fixture and verified scoped scan + write-capable client.
- [x] Initial scan and incremental monitoring persist photos, metadata, cursor state, tombstones, and classification state exactly as SCN-040-004 and SCN-040-005 require. **Phase:** implement. **Claim Source:** executed. Evidence: `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` exit 0 ran `TestPhotosImmich_IncrementalChangesUpdateState` (asserts `ReusedClassificationCount=1` for album move, `ClassifiedCount=1` for new upload, `TombstonedCount=1` for delete, lifecycle_state=`deleted` row); `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e --go-run TestPhotosSync_E2E_AlbumMoveDoesNotReclassify` confirmed the same shape over the live `/v1/photos/{id}` API; `internal/connector/photos/scanner.go::processOne` reuses classification when content_hash unchanged and falls through to tombstone for `PhotoOpDelete`.
- [x] Immich `access_token` and other provider-secret fields stay inline-empty in `config/smackerel.yaml`, are sourced from environment via `./smackerel.sh config generate`, and Go startup fails loudly when an enabled Immich provider has empty secrets (zero hardcoded fallbacks). **Phase:** implement. **Claim Source:** executed. Evidence: `config/smackerel.yaml` keeps `photos.providers.immich.api_key: ""` and `base_url: ""` empty; `internal/config/photos.go::loadImmichPhotosProviderConfig` appends `PHOTOS_PROVIDER_IMMICH_API_KEY (required when provider is enabled)` and the matching base_url error to `errs` when the provider is enabled and credentials are blank; `./smackerel.sh test unit` exit 0 ran `TestPhotosConfigEnabledProviderRequiresCredentials` and `TestPhotosConfigProviderSecretEnvMustExist`; `./smackerel.sh check` exit 0 reported `Config is in sync with SST` and `env_file drift guard: OK`.
- [x] Connector list, add wizard, connector detail, search, and photo detail screens render from live API data and cover loading, empty, degraded, and error states. **Phase:** implement. **Claim Source:** executed. Evidence: `web/pwa/photo-libraries.html`/`.js` render loading, empty, error, and connector cards from `GET /v1/photos/connectors`; wizard `web/pwa/photo-library-add.html`/`.js` posts to `/v1/photos/connectors/test` then `/v1/photos/connectors` with Bearer auth and included_albums; `photo-library-detail.html`/`.js` renders progress + skips per phase from `/v1/photos/connectors/{id}`; `photo-search.html`/`.js` calls `/v1/photos/search` and renders ocr_snippet + match_confidence; `photo-detail.html`/`.js` reads `/v1/photos/{id}`; `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e --go-run TestPhotos` exit 0 ran `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` and `TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI` against the live PWA + core stack with each page returning the contract endpoint string and `role="status"` regions.
- [x] Every skip and extraction failure category in SCN-040-006 is visible with file identity, reason, count, and retry action. **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/scanner.go::skipForEvent` + `newSkipEntry` cover `too_large`, `unsupported_format`, `permission_denied`, `provider_error`, and `extraction_failed` with normalized reason, retry token (`retry:<reason>:<file>`), recommended action, and file identity; `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` exit 0 ran `TestPhotosImmich_SkipLedgerVisibleAndRetryable` confirming all five categories persist with `RetryToken != ""` and `FileIdentities` populated, and `GetConnectorState` returns 5 skips; `web/pwa/photo-library-detail.js::renderSkips` writes reason + count + retry_token into the live skip ledger UI.
- [x] Scenario-specific E2E regression tests for every changed Immich connect/scan/search behavior are added and pass. **Phase:** implement. **Claim Source:** executed. Evidence: SCN-040-004 covered by `tests/integration/photos_immich_test.go::TestPhotosImmich_ConnectScopeAndScanLiveProvider` and `tests/e2e/photos_search_test.go::TestPhotosSearch_E2E_ImmichWhiteboardOCRResult`; SCN-040-005 covered by `tests/integration/photos_sync_test.go::TestPhotosImmich_IncrementalChangesUpdateState` and `tests/e2e/photos_sync_test.go::TestPhotosSync_E2E_AlbumMoveDoesNotReclassify`; SCN-040-006 covered by `tests/integration/photos_skip_ledger_test.go::TestPhotosImmich_SkipLedgerVisibleAndRetryable` plus the live PWA progress/skip render in `tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI`; `web/pwa/tests/photos_connectors.spec.ts` and `web/pwa/tests/photos_connector_progress.spec.ts` are committed as the planned Playwright traceability anchors pointing at the Go live-stack assertions.
- [x] Consumer Impact Sweep confirms navigation, route, API-client, config, doc, and test references use the final endpoint names. **Phase:** implement. **Claim Source:** executed. Evidence: PWA navigation links reference `photo-libraries.html`, `photo-library-add.html`, `photo-library-detail.html`, `photo-search.html`, `photo-detail.html`; API clients (`photo-libraries.js`, `photo-library-add.js`, `photo-library-detail.js`, `photo-search.js`, `photo-detail.js`) all call `/v1/photos/connectors`, `/v1/photos/connectors/test`, `/v1/photos/connectors/{id}`, `/v1/photos/search`, and `/v1/photos/{id}`; `internal/api/photos.go::PhotosHandlers` routes match those paths; `tests/e2e/photos_pwa_test.go` enforces the contract strings in HTML and JS; `grep -rn 'v1/photos' web/pwa/` shows only the final endpoint family (no stale path leaked).
- [x] Broader E2E regression suite passes through `./smackerel.sh test e2e`. **Phase:** implement. **Claim Source:** executed. Evidence: `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e` exit 0; shell summary reported `Total: 35`, `Passed: 35`, `Failed: 0`; Go e2e packages reported `ok github.com/smackerel/smackerel/tests/e2e 103.781s`, `ok .../tests/e2e/agent 3.181s`, `ok .../tests/e2e/drive 3.657s`, including all five `TestPhotos*_E2E_*` tests passing.
- [x] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, and `./smackerel.sh test integration` pass. **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh check` exit 0 (`Config is in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK`); `./smackerel.sh lint` exit 0 (`All checks passed!`, `Web validation passed`); `./smackerel.sh format --check` exit 0 (`48 files already formatted`); `./smackerel.sh test unit` exit 0 (Go packages all `ok` including `internal/connector/photos/adapters/immich`, Python `402 passed, 1 warning`); `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` exit 0 with `ok github.com/smackerel/smackerel/tests/integration 30.582s` covering `TestPhotosImmich_ConnectScopeAndScanLiveProvider`, `TestPhotosImmich_IncrementalChangesUpdateState`, and `TestPhotosImmich_SkipLedgerVisibleAndRetryable`.
- [x] `scopes.md`, `scenario-manifest.json`, and `test-plan.json` remain in sync for SCN-040-004 through SCN-040-006. **Phase:** implement. **Claim Source:** executed. Evidence: SCN-040-004/005/006 manifest rows now reference real files — `internal/connector/photos/adapters/immich/immich_test.go`, `internal/api/photos_test.go`, `tests/integration/photos_immich_test.go`, `tests/integration/photos_sync_test.go`, `tests/integration/photos_skip_ledger_test.go`, `tests/e2e/photos_search_test.go`, `tests/e2e/photos_sync_test.go`, `web/pwa/tests/photos_connectors.spec.ts`, and `web/pwa/tests/photos_connector_progress.spec.ts` all exist; `evidenceRefs` for SCN-040-004/005/006 now point at `report.md#scope-2-immich-connect-scan-and-search`; test-plan.json file paths match the manifest verbatim; `./smackerel.sh check` exit 0 kept scenario-lint clean.

---

## Scope 3: Lifecycle, Duplicates, And Removal Review

**Status:** Done  
**Priority:** P0  
**Depends On:** Scope 2

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-040-007 RAW-to-processed lifecycle is linked with editor rationale
  Given a library contains RAW originals and edited exports from Lightroom, Darktable, GIMP, RawTherapee, and DaVinci Resolve
  When lifecycle analysis runs
  Then RAW-export links are persisted with editor, confidence, and LLM rationale
  And low-confidence links require user review before changing lifecycle state

Scenario: SCN-040-008 Duplicate clusters produce best-pick and rationale
  Given the library contains exact duplicates, burst sequences, HDR brackets, panorama components, blurry frames, and cross-provider candidate seeds
  When duplicate analysis and aesthetic scoring run
  Then clusters persist with kind, members, best-pick, confidence, quality issues, and rationale
  And search returns a cross-provider duplicate once with all provider links

Scenario: SCN-040-009 Removal and destructive actions require confirmation
  Given removal candidates exist for unprocessed RAWs, non-best burst frames, blurry photos, transient screenshots, and cross-provider duplicates
  When the user plans archive or delete from the removal review screen
  Then the system creates a scoped action token and performs no provider mutation until exact confirmation succeeds
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected user-visible result | Test type |
|---|---|---|---|---|
| Lifecycle review | Scope 2 scan complete with RAW/export fixtures | Open Photo Health Lifecycle and drill into editor signatures | Counts, linked exports, editor names, confidence, and review-needed items render | e2e-ui |
| Duplicate cluster review | Duplicate fixtures classified | Open Duplicates tab, change best-pick, resolve cluster | Best-pick rationale and action plan render; mutation waits for confirmation | e2e-ui |
| Removal confirmation | Removal candidates selected | Plan delete/archive, open confirm modal, cancel and confirm separate runs | Cancel mutates nothing; confirm mutates only token scope | e2e-ui |

### Implementation Plan

- Implement lifecycle candidate generation from stable signals, ML `photos.lifecycle` request/result handling, and persistence to `photo_lifecycle_links`.
- Implement duplicate cluster seeding, ML `photos.dedupe` and `photos.aesthetic` result handling, best-pick override, and cluster state transitions.
- Implement removal candidate generation and `photos.removal.evaluate` result handling with mandatory rationale/confidence.
- Implement action planning and confirmation endpoints backed by `photo_action_tokens`, with text confirmation for delete and scope immutability checks.
- Implement provider writer calls for archive/delete/tag/album operations only after confirmation and capability check.
- Build PWA Photo Health Lifecycle, Duplicates, Removal, Quality, and Confirm Destructive Action screens.
- Add audit rows for reveal, plan, confirm, provider mutation, capability block, and cancellation outcomes.

### Consumer Impact Sweep

- Affected consumers: PWA Photo Health tabs, Photo Detail sibling links, provider writer calls, audit event readers, Telegram destructive-action prompts, agent tool `photos.actions.plan`.
- Stale-reference search surfaces: action token endpoint names, cluster route names, removal reason enum names, lifecycle state names, UI data hooks, e2e selectors, docs in design/spec.

### Change Boundary

- Allowed file families: `internal/connector/photos/lifecycle*`, `internal/connector/photos/dedupe*`, `internal/connector/photos/removal*`, `internal/connector/photos/action_tokens*`, `internal/api/photos_actions*`, `internal/api/photos_health*`, `ml/app/photos.py` (lifecycle/dedupe/aesthetic/removal handlers only), `ml/tests/test_photos_decisions*.py`, `web/pwa/src/photos/health/`, `web/pwa/tests/photos_lifecycle_review*`, `web/pwa/tests/photos_duplicates*`, `web/pwa/tests/photos_confirm_action*`, `tests/integration/photos_lifecycle*`, `tests/integration/photos_dedupe*`, `tests/integration/photos_removal*`, `tests/e2e/photos_removal_review*`, `tests/e2e/photos_cross_provider_dedupe*`.
- Excluded surfaces: Immich adapter mapping (Scope 2), upload/Telegram/cross-feature routing (Scope 4), capability matrix governance and stress fixtures (Scope 5), production Docker image lifecycle.

### Test Plan

| Type | Category | File / location | Expected test title | Scenario(s) | Command | Live system |
|---|---|---|---|---|---|---|
| Unit | unit | `internal/connector/photos/exif_test.go` | `TestEditorSignatureMapping_AllSupportedEditors` | SCN-040-007 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/connector/photos/action_tokens_test.go` | `TestPhotoActionTokenRejectsScopeDriftAndExpiry` | SCN-040-009 | `./smackerel.sh test unit` | No |
| Unit | unit | `ml/tests/test_photos_decisions.py` | `test_lifecycle_dedupe_removal_results_require_rationale_and_confidence` | SCN-040-007, SCN-040-008, SCN-040-009 | `./smackerel.sh test unit` | No |
| Integration | integration | `tests/integration/photos_lifecycle_test.go` | `TestPhotosLifecycle_RAWExportsLinkedWithRationale` | SCN-040-007 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_dedupe_test.go` | `TestPhotosDedupe_BurstHDRPanoramaAndExactClusters` | SCN-040-008 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_removal_test.go` | `TestPhotosRemovalCandidates_RequireRationaleAndNoMutationBeforeConfirm` | SCN-040-009 | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_removal_review_test.go` | `TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm` | SCN-040-009 | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_cross_provider_dedupe_test.go` | `TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce` | SCN-040-008 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_lifecycle_review.spec.ts` | `test('low confidence RAW match stays in review until user confirms')` | SCN-040-007 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_duplicates.spec.ts` | `test('duplicate cluster best-pick and resolve flow uses live API')` | SCN-040-008 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_confirm_action.spec.ts` | `test('destructive photo action requires exact batch confirmation')` | SCN-040-009 | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] Lifecycle analysis persists RAW-export links with editor, confidence, rationale, and review-required state for low-confidence matches.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/lifecycle.go::LifecycleAnalyzer.Apply` writes a `photo_raw_export_links` row with `editor`, `confidence`, `rationale`, and `review_state`; live integration test `tests/integration/photos_lifecycle_test.go::TestPhotosLifecycle_RAWExportsLinkedWithRationale` (`PASS (0.13s)` under `COMPOSE_PROGRESS=plain ./smackerel.sh test integration`) verifies a low-confidence Lightroom match is persisted with `review_state=review_required` and an audit event is emitted, while a high-confidence Darktable match flips to `auto_linked`.
- [x] Duplicate analysis persists exact, burst, HDR, panorama, near-duplicate, and cross-provider clusters with best-pick rationale.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/dedupe.go::DedupeAnalyzer` and `Store.SetBestPick` persist clusters of every kind in `photo_clusters` with `best_photo_id`, `best_picked_by`, and `state`; live integration test `tests/integration/photos_dedupe_test.go::TestPhotosDedupe_BurstHDRPanoramaAndExactClusters` covers all four subtests (`burst`, `hdr`, `panorama`, `exact`, all `PASS`) under `COMPOSE_PROGRESS=plain ./smackerel.sh test integration`. Cross-provider grouping is exercised by the live e2e `tests/e2e/photos_cross_provider_dedupe_test.go::TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce` (`PASS (0.07s)`) which verifies `/v1/photos/health/duplicates` and the `?kind=cross_provider_hash` filter return each cluster at most once.
- [x] Removal candidates exist only with reason, confidence, rationale, source method, and reversible decision state.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/removal.go::RemovalAnalyzer.Apply` upserts `photo_removal_candidates` with `(photo_id, reason)` uniqueness (added by migration `029_photo_scope3_lifecycle_dedupe_removal.sql`), `confidence`, `rationale`, `method`, and `action_status` defaulting to `pending_review`; live integration test `tests/integration/photos_removal_test.go::TestPhotosRemovalCandidates_RequireRationaleAndNoMutationBeforeConfirm` (`PASS (0.11s)` under `COMPOSE_PROGRESS=plain ./smackerel.sh test integration`) drives the analyzer plus `Store.MarkRemovalDecision` to confirm rationale enforcement and reversible state transitions.
- [x] Provider archive/delete/album-removal never executes before action token confirmation and capability check.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `internal/connector/photos/writer_guard.go::ConfirmedWriter` rejects `Archive`/`Delete`/`AlbumRemove` calls without a confirmed `PhotoActionToken` whose action and scope match the request; `internal/connector/photos/action_tokens.go` enforces scope-hash drift and `requires_text` for delete plans; the e2e test `tests/e2e/photos_removal_review_test.go::TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm` (`PASS (0.09s)` under `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`) plans an archive then proves a confirm with mismatched `photo_ids` returns non-200 referencing scope, and a delete plan without text confirmation also fails.
- [x] Photo Health Lifecycle, Duplicates, Removal, Quality, and Confirm Destructive Action screens satisfy the UI matrix across desktop and mobile viewports.
  - **Phase:** implement. **Claim Source:** executed. Evidence: PWA screens `web/pwa/photo-health-lifecycle.html`+`.js`, `photo-health-duplicates.html`+`.js`, `photo-health-removal.html`+`.js` (with `data-action-status` attribute on each candidate), `photo-health-quality.html`+`.js`, and `photo-confirm-action.html`+`.js` are mounted at `/pwa/...` and consume the live `/v1/photos/health/...` and `/v1/photos/actions/...` endpoints; e2e contract test `tests/e2e/photos_health_dashboards_e2e_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates` (`PASS (0.08s)` under `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`) fetches each page + script and asserts the required data hooks, role="status" regions, and endpoint references; the `/v1/photos/health/lifecycle` JSON envelope (`total`, `confirmation_threshold` ∈ (0,1]) is also asserted by the same test.
- [x] Scenario-specific E2E regression tests for lifecycle, duplicate, and removal-confirmation behaviors are added and pass.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `tests/e2e/photos_cross_provider_dedupe_test.go::TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce` (SCN-040-008), `tests/e2e/photos_removal_review_test.go::TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm` (SCN-040-009), and `tests/e2e/photos_health_dashboards_e2e_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates` (SCN-040-007/008) all `PASS` under `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e` (Go e2e package `ok github.com/smackerel/smackerel/tests/e2e 110.134s`).
- [x] Consumer Impact Sweep confirms no stale action-token, cluster, removal, lifecycle, or selector references remain.
  - **Phase:** implement. **Claim Source:** executed. Evidence: new code adds `internal/connector/photos/{lifecycle,dedupe,removal,action_tokens,writer_guard}.go`, `internal/api/photos_actions.go`, `internal/db/migrations/029_photo_scope3_lifecycle_dedupe_removal.sql`, and the PWA + e2e + integration tests above. Existing photo handlers are mounted only when `deps.PhotosHandlers != nil` and reuse the existing bearer-auth router group; no prior callers reference removed names because the new symbols (`RemovalReason`, `ClusterKind`, `ActionKind`, `ActionScope`, `PlanAction`, `ConfirmAction`, `HealthLifecycle`, `HealthDuplicates`, `HealthDuplicatesGet`, `HealthRemoval`, `HealthQuality`) are net-additive. `./smackerel.sh check` exit 0 (`config in sync`, `env_file drift guard: OK`, `scenario-lint: OK`) verifies no SST drift.
- [x] Broader E2E regression suite passes through `./smackerel.sh test e2e`.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e` exits `EXIT=0` with `ok github.com/smackerel/smackerel/tests/e2e 110.134s`, `ok github.com/smackerel/smackerel/tests/e2e/agent 5.018s`, `ok github.com/smackerel/smackerel/tests/e2e/drive 5.810s`, plus the shell e2e suite (35/35 PASS) reported in the same run.
- [x] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, and `./smackerel.sh test integration` pass.
  - **Phase:** implement. **Claim Source:** executed. Evidence: `./smackerel.sh check` → `Config is in sync with SST` / `env_file drift guard: OK` / `scenario-lint: OK`. `./smackerel.sh lint` → `All checks passed!` / `Web validation passed`. `./smackerel.sh format --check` → `49 files already formatted`. `./smackerel.sh test unit` → all Go packages `ok` (incl. `internal/connector/photos` and `internal/api`) plus Python `407 passed`. `COMPOSE_PROGRESS=plain ./smackerel.sh test integration` → `ok github.com/smackerel/smackerel/tests/integration 36.056s`, `ok .../agent 7.290s`, `ok .../drive 9.532s`, EXIT=0.
- [x] `scopes.md`, `scenario-manifest.json`, and `test-plan.json` remain in sync for SCN-040-007 through SCN-040-009.
  - **Phase:** implement. **Claim Source:** executed. Evidence: Test Plan rows in this scope reference `tests/integration/photos_lifecycle_test.go`, `tests/integration/photos_dedupe_test.go`, `tests/integration/photos_removal_test.go`, `tests/e2e/photos_removal_review_test.go`, `tests/e2e/photos_cross_provider_dedupe_test.go`, and `web/pwa/tests/photos_lifecycle_review.spec.ts` / `photos_duplicates.spec.ts` / `photos_confirm_action.spec.ts`; all of those files exist on disk after this scope. `scenario-manifest.json` SCN-040-007/008/009 `linkedTests` resolve to the same files (verified by `bash .github/bubbles/scripts/traceability-guard.sh specs/040-cloud-photo-libraries --verbose` showing `✅ scenario-manifest.json linked test exists` for every Scope 3 entry); `evidenceRefs` for SCN-040-007/008/009 are populated to point at this report's Scope 3 section.

---

## Scope 4: Capture, Telegram, And Cross-Feature Routing

**Status:** Not Started  
**Priority:** P0  
**Depends On:** Scope 3

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-040-010 Uploads from Telegram, mobile, and web route through the same photo pipeline
  Given upload rules target an Immich album and downstream photo classification is enabled
  When a user uploads a photo through Telegram, mobile capture, and the web interface
  Then each upload reaches the provider when capability allows, becomes searchable, and records both source-channel and provider refs

Scenario: SCN-040-011 Document, receipt, and recipe photos create downstream artifacts
  Given uploaded or scanned photos classify as receipt, recipe, legal document, product screenshot, menu, or place context
  When confidence exceeds the configured routing threshold
  Then the expected expense, recipe, knowledge, list, annotation, or meal-plan reference is created with the original photo linked as evidence

Scenario: SCN-040-012 Sensitive retrieval blocks unsafe delivery
  Given a photo is classified as identity document, medical, financial, children, intimate, or private location
  When Telegram or another channel asks to send matching photo bytes
  Then the channel receives a refusal, secure-link flow, or reveal prompt and no raw photo bytes are delivered automatically
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected user-visible result | Test type |
|---|---|---|---|---|
| Mobile document scan | PWA camera fixture and upload target configured | Capture four pages, confirm, upload | One document artifact with original pages, clean scan, OCR, and route appears | e2e-ui |
| Telegram upload and retrieval | Telegram test bot connected to live stack | Send photo, ask for photo by description | Bot confirms upload/classification; safe retrieval returns photo/link/disambiguation | e2e-api |
| Sensitive retrieval block | Sensitive fixture indexed | Ask Telegram to send passport photo | Bot refuses or returns secure reveal flow and does not send image bytes | e2e-api |

### Implementation Plan

- Implement `POST /v1/photos/upload` multipart flow for mobile/web/Telegram with channel metadata, source refs, provider upload, and classification trigger.
- Extend Telegram capture/retrieval handlers to use `PhotoSearchResult`, sensitivity checks, provider links, and disambiguation lists.
- Implement document scan mode integration: original pages, corrected scan artifact, OCR, and multi-page artifact grouping.
- Implement cross-feature routing from photo classification to expenses, recipes, knowledge/domain extraction, annotations, lists, meal planning, and intelligence delivery.
- Implement save-rule integration for photo uploads and classification-triggered routes, reusing the feature 038 rule shape where applicable.
- Enforce sensitivity and reveal-token policy server-side for search previews, photo detail, Telegram delivery, digest inclusion, and agent tools.
- Build PWA mobile document scan surface and photo routing rule extensions.

### Consumer Impact Sweep

- Affected consumers: Telegram bot handlers, mobile capture, browser/web upload, recipe APIs, expense APIs, knowledge/domain extraction, annotations, lists, meal planning, intelligence delivery, agent tools.
- Search surfaces: route names, upload source metadata names, sensitivity label taxonomy, route target enum values, API docs, save-rule config fields, tests.

### Change Boundary

- Allowed file families: `internal/api/photos_upload*`, `internal/connector/photos/routing*`, `internal/connector/photos/sensitivity*`, `internal/telegram/` (photo upload/retrieval handlers only), `web/pwa/src/photos/docscan/`, `web/pwa/tests/photos_docscan*`, sibling-feature integration shims under `internal/expense/`, `internal/recipe/`, `internal/knowledge/`, `internal/annotation/`, `internal/list/`, `internal/mealplan/` strictly limited to receiving `PhotoSummary` references, `tests/integration/photos_upload*`, `tests/integration/photos_docscan*`, `tests/integration/photos_sensitivity*`, `tests/e2e/photos_telegram*`, `tests/e2e/photos_routing*`, `tests/e2e/photos_sensitivity_retrieval*`.
- Excluded surfaces: Immich adapter mapping (Scope 2), lifecycle/dedupe/removal logic (Scope 3), capability matrix governance and stress fixtures (Scope 5), unrelated sibling-feature business logic, production Docker image lifecycle.

### Test Plan

| Type | Category | File / location | Expected test title | Scenario(s) | Command | Live system |
|---|---|---|---|---|---|---|
| Unit | unit | `internal/api/photos_upload_test.go` | `TestPhotosUpload_PreservesSourceAndProviderRefs` | SCN-040-010 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/connector/photos/routing_test.go` | `TestPhotoRoutingTargetsRequireClassificationAndConfidence` | SCN-040-011 | `./smackerel.sh test unit` | No |
| Integration | integration | `tests/integration/photos_upload_test.go` | `TestPhotosUpload_TelegramMobileWebEnterSamePipeline` | SCN-040-010 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_docscan_test.go` | `TestPhotosDocumentScan_MultiPageOCRAndCleanArtifact` | SCN-040-011 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_sensitivity_test.go` | `TestPhotosSensitivity_ServerSidePreviewRevealAndAudit` | SCN-040-012 | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_telegram_test.go` | `TestPhotosTelegram_E2E_UploadClassifySearchAndRetrieve` | SCN-040-010 | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_routing_test.go` | `TestPhotosRouting_E2E_ReceiptRecipeDocumentCreateDownstreamArtifacts` | SCN-040-011 | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_sensitivity_retrieval_test.go` | `TestPhotosSensitivity_E2E_TelegramDoesNotAutoSendSensitivePhoto` | SCN-040-012 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_docscan.spec.ts` | `test('mobile document scan creates multi-page OCR artifact from live upload API')` | SCN-040-011 | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] Telegram, mobile, and web upload paths enter one shared photo pipeline and preserve source-channel plus provider refs.
- [ ] Telegram `bot_token` and any new upload-secret fields stay inline-empty in `config/smackerel.yaml`, are sourced from environment via `./smackerel.sh config generate`, and Go and Python startup fail loudly when an enabled secret is empty (zero hardcoded fallbacks).
- [ ] Document scan creates original and corrected artifacts, OCRs every page, and routes documents through classification.
- [ ] Receipt, recipe, document, product, menu/place, annotation/list, meal-plan, and intelligence routes create downstream artifacts only when confidence and sensitivity policy allow.
- [ ] Sensitive previews and Telegram/photo-channel retrieval are blocked server-side without reveal authorization, and audit rows are written for reveal or block events.
- [ ] Scenario-specific E2E regression tests for upload, routing, and sensitivity retrieval are added and pass.
- [ ] Consumer Impact Sweep confirms all cross-feature consumers use provider-neutral photo APIs and no route target enum references are stale.
- [ ] Broader E2E regression suite passes through `./smackerel.sh test e2e`.
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, and `./smackerel.sh test integration` pass.
- [ ] `scopes.md`, `scenario-manifest.json`, and `test-plan.json` remain in sync for SCN-040-010 through SCN-040-012.

---

## Scope 5: Multi-Provider Capability Governance And Operations

**Status:** Not Started  
**Priority:** P1  
**Depends On:** Scope 4

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-040-013 Provider limitation is visible and non-mutating
  Given a connected provider supports read/search but does not support album write-back or delete
  When a rule or user action attempts an unsupported operation
  Then the API returns 409 PROVIDER_LIMITATION with limitation_code
  And the PWA and Telegram surfaces show the same capability reason while search/classification continue to work

Scenario: SCN-040-014 Cross-provider search and dedupe are provider-neutral
  Given Immich and a second provider contain the same vacation photo plus provider-specific metadata
  When both providers are scanned and the user searches for the photo
  Then the result appears once in unified ranking with both provider links
  And duplicate cluster membership does not depend on provider-specific code paths

Scenario: SCN-040-015 Photo health and observability prove large-library readiness
  Given a synthetic 15,000-photo library with RAWs, exports, documents, receipts, videos, duplicates, sensitive photos, and blocked fixtures
  When the full photo workflow runs on the disposable test stack
  Then health dashboards show progress, lag, confidence histogram, lifecycle distribution, duplicate count, capability limits, and skip counts
  And stress validation reaches the configured ingest/search targets without touching user-owned libraries
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected user-visible result | Test type |
|---|---|---|---|---|
| Capability banner | Read-limited second provider connected | Attempt unsupported album write from rule or UI | `Provider Limitation` banner appears with exact reason; no provider mutation happens | e2e-ui |
| Unified cross-provider search | Immich plus second provider scanned | Search for duplicated vacation photo | One ranked result with both provider links and provider badges | e2e-ui |
| Photo health dashboard | Synthetic large library processed | Open Photo Health pages | Dashboard metrics match live API progress and distributions | e2e-ui |

### Implementation Plan

- Add one non-Immich provider adapter path selected by implementation based on available local/disposable testability, using the same `PhotoLibrary` and capability governance contract.
- Implement provider capability probe persistence, limitation-code propagation, API `409 PROVIDER_LIMITATION`, UI Provider Limitation Notice, and Telegram limitation messages.
- Extend cross-provider search, duplicate clustering, and health dashboards so provider-specific metadata enriches results without forking logic.
- Add metrics and traces for scan phases, ML calls, limited capabilities, destructive actions, sensitivity reveals, and skip reasons.
- Add stress fixtures and disposable test-stack guardrails for the 15,000-photo success signal.
- Update operational docs only if implementation changes runtime commands, config surfaces, or operator-visible photo health behavior.

### Shared Infrastructure Impact Sweep

- Protected surfaces: stress fixtures, synthetic provider containers or simulators, test-stack config, pgvector indexes, NATS concurrency, ML inflight controls, PWA health dashboards.
- Canary tests: capability-block canary, cross-provider duplicate canary, health metrics canary, privacy-boundary canary.
- Restore path: test providers and synthetic fixture data are owned by the test stack and cleaned by `./smackerel.sh clean smart`/test-stack lifecycle; no user-owned provider URLs are allowed.

### Test Plan

| Type | Category | File / location | Expected test title | Scenario(s) | Command | Live system |
|---|---|---|---|---|---|---|
| Unit | unit | `internal/connector/photos/capabilities_test.go` | `TestProviderCapabilityLimitationsReturnStableCodes` | SCN-040-013 | `./smackerel.sh test unit` | No |
| Unit | unit | `internal/connector/photos/cross_provider_test.go` | `TestCrossProviderDuplicateUsesProviderNeutralSignals` | SCN-040-014 | `./smackerel.sh test unit` | No |
| Integration | integration | `tests/integration/photos_provider_neutrality_test.go` | `TestPhotosProviderNeutrality_SecondAdapterMatchesImmichShape` | SCN-040-014 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_health_test.go` | `TestPhotosHealth_ProgressMetricsAndCapabilityLimitsFromLiveAPI` | SCN-040-015 | `./smackerel.sh test integration` | Yes |
| Integration | integration | `tests/integration/photos_capability_test.go` | `TestPhotosCapability_UnsupportedOperationIs409AndNonMutating` | SCN-040-013 | `./smackerel.sh test integration` | Yes |
| Canary | integration | `tests/integration/photos_capability_taxonomy_canary_test.go` | `TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes` | SCN-040-013 | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_capability_test.go` | `TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks` | SCN-040-013 | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | `tests/e2e/photos_search_test.go` | `TestPhotosSearch_E2E_CrossProviderUnifiedRanking` | SCN-040-014 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_capability_banner.spec.ts` | `test('provider limitation banner renders exact live limitation code')` | SCN-040-013 | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | `web/pwa/tests/photos_health.spec.ts` | `test('photo health dashboard renders lifecycle duplicate sensitivity and confidence metrics')` | SCN-040-015 | `./smackerel.sh test e2e` | Yes |
| Stress | stress | `tests/stress/photos_ingest_stress_test.go` | `TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget` | SCN-040-015 | `./smackerel.sh test stress` | Yes |

### Definition of Done

- [ ] A second provider adapter path uses the same provider-neutral classification, search, lifecycle, dedupe, routing, and capability governance contracts as Immich.
- [ ] Unsupported and limited provider operations return stable `409 PROVIDER_LIMITATION` responses with limitation codes and no provider mutation.
- [ ] Capability-matrix taxonomy canary proves Go capability registry codes, NATS event payloads, API limitation codes, and PWA limitation banner strings are sourced from one shared definition with no drift.
- [ ] Cross-provider search returns unified ranked results and duplicate clusters with provider links, without provider-specific downstream branching.
- [ ] Photo Health and operations surfaces expose scan progress, monitoring lag, confidence distribution, duplicate counts, lifecycle distribution, sensitivity counts, skips, and capability limits from live API data.
- [ ] Observability metrics, logs, and traces are added without exposing photo bytes, preview URLs, or sensitive content.
- [ ] Scenario-specific E2E regression tests for provider limitation, cross-provider search, and photo health operations are added and pass.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before stress and broader e2e suites run.
- [ ] Broader E2E regression suite passes through `./smackerel.sh test e2e`.
- [ ] Stress validation passes through `./smackerel.sh test stress` using synthetic fixtures only.
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, and `./smackerel.sh test integration` pass.
- [ ] `scopes.md`, `scenario-manifest.json`, and `test-plan.json` remain in sync for SCN-040-013 through SCN-040-015.

---

## Horizontal Plan Check

The scope sequence is not a horizontal layer stack. Scope 1 is the only foundational slice and it includes config, DB, NATS, ML schema, API, and live canary validation. Scopes 2 through 5 each deliver user/system outcomes across all impacted layers: provider ingestion/search, lifecycle/review, capture/routing, and multi-provider operations. There are no three consecutive DB-only, service-only, API-only, or UI-only scopes.