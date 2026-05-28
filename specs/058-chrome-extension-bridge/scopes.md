# Scopes: 058 Chrome Extension Bridge (Live Bookmarks + Browser History)

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

### Phase Order

1. **Server Ingest Endpoint + SST Config + Scope-Gated Mount** — Add `internal/config/extension.go` (SST-bound `ExtensionIngestConfig`, fail-loud validation, zero defaults), wire into `internal/config` loader, generate env, add `POST /v1/connectors/extension/ingest` handler in `internal/api/connectors/extension/ingest.go` mounted with `auth.RequireScope("extension:bookmarks", "extension:history")` (AND-semantics; per design §1.1 the required scope tuple). Handler decodes `[]connector.RawArtifact`, performs per-item validation (ContentType allowlist, SourceID, Metadata keys per §2.2), and on success hands each item to the existing `ArtifactPublisher.PublishRawArtifact`. Per-item outcomes `{accepted, deduped, rejected}` returned as HTTP 200 JSON body; transport errors (auth/body/batch) return 4xx per §3.1 error table.
2. **Server Dedup Table + Keyer + Upsert Path** — Forward migration `internal/db/migrations/NNN_create_raw_ingest_dedup.sql` creates `raw_ingest_dedup` (PK `dedup_key BYTEA`, plus owner/device index per §2.3). New `internal/connector/ingest/dedup.go` exports `ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte` (SHA-256 over canonical tuple) and `UpsertDedup(ctx, db, row) (artifactID string, deduped bool, err error)`. Handler from Scope 1 is retrofitted: on insert → publish (new artifact); on collision → increment `visit_count` + bump `last_seen_at`, return existing `artifact_id` with `outcome:"deduped"`. Bookmark bucket fixed at `0` (window bypassed per NC-5); history bucket is `floor(captured_at_unix / window_seconds)` with window resolved from per-request `Metadata.dedup_window_seconds` → SST `default_dedup_window_seconds`.
3. **Chrome MV3 Extension Skeleton + Background WAL + Options Page** — Create `extensions/chrome-bridge/` (TypeScript + esbuild) per design §4.1 module layout. Background service-worker wires `chrome.bookmarks.{onCreated,onChanged,onRemoved,onMoved}` and `chrome.history.{onVisited,onVisitRemoved}` listeners, applies privacy-filter (cap 64 patterns), dwell gate (history only), best-effort local dedup, IndexedDB WAL enqueue, drain loop with exponential backoff per §4.2, and `chrome.alarms` 1-min re-wake per §4.3. Options page persists `{base_url, bearer_token, source_device_id, dedup_window_seconds, dwell_threshold_seconds, privacy_allow_patterns, privacy_deny_patterns}` to `chrome.storage.local` with §4.4 validation; bearer token masked by default. Vitest covers privacy compile, dwell gate, queue, backoff, transport error mapping, and the §9.3 adversarial twins (mismatched device-id, corrupted IndexedDB row).
4. **Build/Release Wiring — `./smackerel.sh build --extension chrome-bridge` + CI Signed Zip + Build-Manifest Entry** — Add `scripts/commands/build-chrome-bridge.sh` (npm ci → esbuild → zip → emit `.sha256`) and a new `build --extension chrome-bridge` case in `smackerel.sh` that dispatches to it. Extend `.github/workflows/build.yml` with a `build-chrome-bridge` job that runs after the core image build on the same checkout/SHA, executes the build, signs the zip with `cosign sign-blob --yes` (keyless, Rekor), uploads zip + `.sha256` + `.sig` to the workflow run and to the GitHub Release for the SHA, and records the zip SHA-256 in `build-manifest-<sourceSha>.yaml` alongside the core/ML image digests. Existing `scripts/commands/package-extension.sh` (share-only extension) is left untouched.
5. **Operator Docs (Sideload + Devices View + Runbook)** — Add to `docs/Operations.md`: "Chrome Extension Bridge — Sideload Workflow" subsection covering download from GitHub Release → `cosign verify-blob` → `chrome://extensions` "Load unpacked" → options-page setup (base URL, paste PASETO from `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history`, set `source_device_id`); document offline revocation latency (OQ-DSN-2) and privacy-filter pattern cap (OQ-DSN-3). Add to `docs/API.md`: `POST /v1/connectors/extension/ingest` request/response shape (§3.1) and authorization matrix (§5.2), plus `GET /v1/admin/extension/devices` (§3.2). Add admin devices read-only view in `internal/api/admin/devices.go` + minimal `web/` page that lists `SELECT DISTINCT source_device_id ...` aggregations from `raw_ingest_dedup`.

### New Types and Signatures

- `internal/config/extension.go` (new) — `type ExtensionIngestConfig struct { Enabled bool; MaxBatchItems int; MaxBodyBytes int64; DefaultDedupWindowSeconds int; AcceptedContentTypes []string; RequiredTokenScope string }`; `func (c *ExtensionIngestConfig) Validate() error` — fail-loud on any zero/empty field per smackerel-no-defaults policy.
- `internal/api/connectors/extension/ingest.go` (new) — `func NewIngestHandler(cfg ExtensionIngestConfig, pub connector.ArtifactPublisher, dedup DedupStore) http.Handler`; per-item response struct `type IngestItemOutcome struct { ClientEventID string; Outcome string; ArtifactID string; Error string }`.
- `internal/connector/ingest/dedup.go` (new) — `func ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte`; `type DedupRow struct { Key []byte; OwnerUserID, SourceID, ContentType, SourceDeviceID, ArtifactID string; CapturedAt time.Time }`; `type DedupStore interface { Upsert(ctx context.Context, row DedupRow) (artifactID string, deduped bool, err error) }`; `func NewPostgresDedupStore(db *sql.DB) DedupStore`.
- `internal/db/migrations/NNN_create_raw_ingest_dedup.sql` (new) — table + index per design §2.3; rollback is `DROP TABLE raw_ingest_dedup;`.
- `internal/api/admin/devices.go` (new) — `func NewDevicesHandler(db *sql.DB) http.Handler`; response shape `{"devices": [{"source_device_id", "user_id", "first_seen_at", "last_seen_at", "visit_count_30d"}]}`.
- `extensions/chrome-bridge/` (new) — `manifest.json` (MV3, min Chrome version pinned, CSP `connect-src` restricted to operator base URL); `src/background/index.ts` event registration; `src/background/queue.ts` exports `enqueue(item)` / `drain()` / `markOutcome(clientEventID, outcome)`; `src/background/transport.ts` exports `postBatch(items): Promise<IngestItemOutcome[]>`; `src/common/schema.ts` mirrors `RawArtifact` + `Metadata` typings from design §2.1–§2.2.
- `scripts/commands/build-chrome-bridge.sh` (new) — pipeline per design §8.1; emits `dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip` + `.sha256`.
- `smackerel.sh` — new dispatch arm `build --extension chrome-bridge` forwarding to `scripts/commands/build-chrome-bridge.sh`; honors existing SST env loading; no fallback defaults.
- `.github/workflows/build.yml` — new `build-chrome-bridge` job per design §8.2.
- `config/smackerel.yaml` — new `extension.ingest.*` block per design §6.

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test unit --go-run TestExtensionIngestConfig_Validate` proves SST validation rejects every zero/empty field; `go test ./internal/api/connectors/extension -run 'TestIngest_RejectsBatchOver256|TestIngest_RejectsBodyOver1MiB|TestIngest_RejectsInvalidJSON|TestIngest_RejectsMissingScope|TestIngest_AcceptsValidBatch|TestIngest_PerItemRejection'` proves handler-level guarantees; integration test against the live test stack (router + `bearerAuthMiddleware` + `RequireScope("extension:bookmarks","extension:history")`) proves scope-gating end-to-end and rejects a legacy spec-044 token (BS-002 adversarial twin reuse).
- After Scope 2: `go test ./internal/connector/ingest -run 'TestComputeDedupKey_Deterministic|TestComputeDedupKey_VariesByDevice|TestComputeDedupKey_VariesByBucket|TestUpsertDedup_InsertPublishes|TestUpsertDedup_CollisionIncrementsVisitCount|TestUpsertDedup_BookmarkBucketAlwaysZero'` proves keyer + upsert; integration test (Postgres + NATS) proves: bookmark add → 1 publish; two history visits within window → 1 publish + `visit_count=2`; two history visits across window → 2 publishes; same URL across two `source_device_id` → 2 publishes (Chrome Sync case); revoked token → 401 within ≤ 60 s.
- After Scope 3: `cd extensions/chrome-bridge && npm test` (vitest) proves privacy compile (incl. cap-64 rejection), dwell gate, IndexedDB WAL persistence across simulated SW eviction, backoff curve, transport 401/403/413/422/5xx error mapping, and the §9.3 adversarial twins (mismatched device-id, corrupted IndexedDB row skipped without losing neighbors). E2E browser (Playwright + headless Chromium loading the unpacked extension) proves bookmark-add → artifact visible via `/v1/artifacts?source=browser-extension` within 60 s.
- After Scope 4: Two CI runs on the same git SHA produce byte-identical zips (recorded in `build-manifest-<sourceSha>.yaml`); `cosign verify-blob` against the released artifact succeeds; `./smackerel.sh build --extension chrome-bridge` succeeds locally with the same SHA-256 output.
- After Scope 5: `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/058-chrome-extension-bridge --verbose` proves doc changes are registered; manual review confirms Operations.md covers the sideload workflow and API.md documents the endpoint + authorization matrix; integration test against `/v1/admin/extension/devices` proves devices view returns the seeded `source_device_id` aggregation.

## Planning Assumptions

- Spec 060 Scopes 1 + 2 are SHIPPED; `auth.RequireScope(required ...string) func(http.Handler) http.Handler` is importable from `internal/auth` with AND-semantics, dev/test bypass pass-through, and 403 `scope_required` body shape. OQ-DSN-1 is resolved.
- Spec 060 Scope 3 (CLI `--scope` flags) is in flight; the operator-docs scope (058 Scope 5) MUST reference the `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history` form per design §8.3, even though enrollment tooling lands in spec 060.
- `connector.RawArtifact` JSON tags (§2.1) are stable; `ArtifactPublisher.PublishRawArtifact` is the canonical publication path and is reused unchanged (no parallel pipeline).
- The existing `web/extension/` share-only extension stays untouched; the bridge is a separate directory with its own toolchain per design §4 and §10 ("Extending web/extension/ instead of new directory — REJECTED").
- Wire scope tuple is `("extension:bookmarks", "extension:history")` per design §1.1 and §5.1; this binds the AND-semantics enforcement to two granular scopes rather than one combined string. Spec 060's `RegisteredScopeSurfaces = ["extension"]` covers the surface name; the two scope names share that surface.
- SST-zero-defaults: every new tunable (`extension.ingest.*`) is declared in `config/smackerel.yaml` and validated fail-loud; no `os.Getenv(..., default)` patterns; no `${VAR:-default}` in Compose or scripts.
- Build-Once Deploy-Many (G074): the extension zip is an immutable build artifact pinned by SHA-256 in `build-manifest-<sourceSha>.yaml`; CI does not push to the Chrome Web Store; the deploy adapter never rebuilds.
- Service-worker eviction is a known MV3 hazard; the queue is the durability boundary (IndexedDB), and `chrome.alarms` is the wake guarantee. Tests MUST simulate eviction by re-importing the background module against a persisted IndexedDB instance.
- Adversarial regression rows (G024) are mandatory: each dedup test has a mismatched-device-id twin; the scope-enforcement integration test has a non-extension-scope twin proving exact-match (not substring); the offline-queue test has a corrupted-row twin proving the drainer skips bad entries without losing neighbors.

## Scope Inventory

| Scope | Name | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|----------|---------------|-------------|--------|
| 1 | Server Ingest Endpoint + SST Config + Scope-Gated Mount | `internal/config/extension.go`, `internal/api/connectors/extension/ingest.go`, `config/smackerel.yaml`, router wiring | Unit (config validation, handler decode/limits), integration (live router + RequireScope) | Endpoint POST /v1/connectors/extension/ingest mounted with `auth.RequireScope("extension:bookmarks","extension:history")`; SST fail-loud; per-item outcomes returned; legacy spec-044 token rejected | Not started |
| 2 | Server Dedup Table + Keyer + Upsert Path | `internal/db/migrations/NNN_create_raw_ingest_dedup.sql`, `internal/connector/ingest/dedup.go`, handler retrofit | Unit (keyer, upsert), integration (Postgres + NATS, all §9.2 dedup scenarios) | Dedup table created; SHA-256 keyer deterministic; collision path increments visit_count without re-publish; bookmark bucket fixed at 0; Chrome Sync twin produces two artifacts | Not started |
| 3 | Chrome MV3 Extension Skeleton + Background WAL + Options Page | `extensions/chrome-bridge/` (manifest, background, options, src/common) | Vitest (privacy, dwell, queue, backoff, transport, §9.3 twins), E2E browser (Playwright) | Extension installs in headless Chromium; bookmark-add → artifact within 60 s; offline → queue persists across SW eviction; revoked token surfaces in badge | Not started |
| 4 | Build/Release Wiring (smackerel.sh + CI Signed Zip + Build-Manifest) | `scripts/commands/build-chrome-bridge.sh`, `smackerel.sh`, `.github/workflows/build.yml` | CI smoke (reproducibility: two SHAs identical), cosign verify | `./smackerel.sh build --extension chrome-bridge` emits versioned zip + `.sha256`; CI signs with cosign keyless; zip SHA-256 recorded in `build-manifest-<sourceSha>.yaml`; existing share-extension pipeline untouched | Not started |
| 5 | Operator Docs + Devices Admin View | `docs/Operations.md`, `docs/API.md`, `internal/api/admin/devices.go`, `web/` admin page | regression-baseline-guard; integration (devices view) | Operations.md documents sideload workflow + cosign verify + options-page setup + OQ-DSN-2/OQ-DSN-3 caveats; API.md documents endpoint + auth matrix; devices view returns aggregated `source_device_id` rows | Not started |

---

## Scope 1: Server Ingest Endpoint + SST Config + Scope-Gated Mount

**Status:** Not Started
**Priority:** P0
**Depends On:** spec 060 Scope 2 (`auth.RequireScope` exported — SHIPPED)
**Surfaces:** `internal/config/extension.go` (new), `internal/config` loader, `config/smackerel.yaml`, `internal/api/connectors/extension/ingest.go` (new), `internal/api/router.go` (mount), corresponding `_test.go`.

### Gherkin Scenarios

#### SCN-058-001: Valid scoped batch is accepted and published

```gherkin
Given a PASETO token with scopes ["extension:bookmarks","extension:history"]
And a body of 3 RawArtifact items (2 bookmarks, 1 browser_history_visit) under 1 MiB
When the operator POSTs /v1/connectors/extension/ingest
Then the response is HTTP 200
And the body contains 3 per-item outcomes with outcome="accepted"
And ArtifactPublisher.PublishRawArtifact was called exactly 3 times
And each returned artifact_id is non-empty
```

#### SCN-058-002: Token missing required scope is rejected with 403 scope_required

```gherkin
Given a legacy spec-044 PASETO token with Scopes=nil
When the operator POSTs /v1/connectors/extension/ingest with any body
Then the response is HTTP 403
And the body shape matches the spec 060 `scope_required` error envelope
And ArtifactPublisher.PublishRawArtifact is NEVER called
```

#### SCN-058-003: Body over 1 MiB is rejected with 413 body_too_large

```gherkin
Given a PASETO token with the required scopes
And a request body exceeding 1 MiB (SST: extension.ingest.max_body_bytes)
When the operator POSTs /v1/connectors/extension/ingest
Then the response is HTTP 413 with code "body_too_large"
And no items are decoded or published
```

#### SCN-058-004: Batch over 256 items is rejected with 422 batch_too_large

```gherkin
Given a PASETO token with the required scopes
And a body containing 257 RawArtifact items
When the operator POSTs /v1/connectors/extension/ingest
Then the response is HTTP 422 with code "batch_too_large"
And no items are published
```

#### SCN-058-005: SST loader fails loud on missing/zero ExtensionIngestConfig values

```gherkin
Given config/smackerel.yaml omits any one of {enabled, max_batch_items, max_body_bytes, default_dedup_window_seconds, accepted_content_types, required_token_scope}
When ExtensionIngestConfig.Validate() runs at core startup
Then Validate returns a non-nil error naming the missing field
And the smackerel-core binary exits non-zero before serving any request
```

### Implementation Plan

- `internal/config/extension.go`: declare `ExtensionIngestConfig` struct with yaml tags per design §6; `Validate()` returns wrapped error on any zero-valued field (`Enabled` MUST be explicitly set — use `*bool` or require non-nil yaml node; `len(AcceptedContentTypes) == 0` is an error; `RequiredTokenScope == ""` is an error). Plug into `internal/config/loader.go` so the loader calls `Validate()` and panics/exits non-zero on failure.
- `config/smackerel.yaml`: add the `extension.ingest.*` block from design §6 verbatim. Regenerate via `./smackerel.sh config generate` (committed `config/generated/dev.env`, `config/generated/test.env`).
- `internal/api/connectors/extension/ingest.go`: `NewIngestHandler` returns an `http.Handler` that:
  1. Enforces `Content-Length <= cfg.MaxBodyBytes` → 413 `body_too_large` before reading body (defense-in-depth: wrap body in `http.MaxBytesReader`).
  2. Decodes body via `json.Decoder` with `DisallowUnknownFields()` into `[]connector.RawArtifact`; 400 `invalid_json` on decode error.
  3. Rejects batches with `len(items) > cfg.MaxBatchItems` → 422 `batch_too_large`.
  4. Per item: validate `SourceID == "browser-extension"`, `ContentType` ∈ `cfg.AcceptedContentTypes`, required Metadata keys present per design §2.2; on validation failure emit per-item `outcome:"rejected"` with `error` code; on success call `pub.PublishRawArtifact(ctx, item)` and emit `outcome:"accepted"` with `artifact_id`.
  5. Returns HTTP 200 with `{"items":[...]}` body.
- Router wiring in `internal/api/router.go`: mount under `r.With(auth.RequireScope("extension:bookmarks", "extension:history"))` (AND-semantics — both scopes required). Mount happens inside the existing `bearerAuthMiddleware` group so `Session` is populated before the scope check.
- Scope 2 will retrofit the handler to call `DedupStore.Upsert` between validation and publish; this scope's handler may publish directly OR may take a `DedupStore` parameter whose Scope-1-stage implementation is a no-op pass-through that always returns `(newUUID, false, nil)`. The interface seam is declared in Scope 1; the Postgres implementation lands in Scope 2.

### Test Plan

| Scenario | Test type | File | Test name | Assertion |
|----------|-----------|------|-----------|-----------|
| SCN-058-005 | unit | `internal/config/extension_test.go` | `TestExtensionIngestConfig_Validate_RejectsEachMissingField` | All 6 fields each tested in isolation; error message names the offending field |
| SCN-058-005 | unit | `internal/config/extension_test.go` | `TestExtensionIngestConfig_Validate_AcceptsFullyPopulated` | Valid config returns nil |
| SCN-058-003 | unit | `internal/api/connectors/extension/ingest_test.go` | `TestIngest_RejectsBodyOver1MiB` | 1 MiB + 1 byte body → 413 `body_too_large`; publisher mock NEVER called |
| SCN-058-004 | unit | `internal/api/connectors/extension/ingest_test.go` | `TestIngest_RejectsBatchOver256` | 257-item batch → 422 `batch_too_large`; publisher mock NEVER called |
| SCN-058-001 | unit | `internal/api/connectors/extension/ingest_test.go` | `TestIngest_AcceptsValidBatch_AllAccepted` | 3-item batch (2 bookmark, 1 browser_history_visit) → 200 with 3 `accepted` outcomes; publisher mock called 3 times |
| (per-item rejection) | unit | `internal/api/connectors/extension/ingest_test.go` | `TestIngest_PerItemRejection_PreservesNeighbors` | Mixed batch with one invalid `ContentType` → 200 with mixed outcomes; only valid items published |
| SCN-058-001 | unit | `internal/api/connectors/extension/ingest_test.go` | `TestIngest_RejectsUnknownJSONField_DisallowUnknownFields` | Body with extra top-level key → 400 `invalid_json` (defense-in-depth) |
| SCN-058-002 | integration | `internal/api/connectors/extension/ingest_integration_test.go` | `TestIngest_LegacyToken_Rejected403ScopeRequired` | Live router + spec-044 legacy token (no `scope` claim) → 403 `scope_required`; counter `auth_scope_rejected_total{required_scope="extension:bookmarks"}` increments by 1 |
| SCN-058-001 | integration | `internal/api/connectors/extension/ingest_integration_test.go` | `TestIngest_ScopedToken_Accepted` | Live router + PASETO token with both required scopes → 200; artifact retrievable via `/v1/artifacts?source=browser-extension` |
| Regression E2E | e2e-api | `tests/e2e/extension_ingest_e2e_test.go` | `TestE2E_ExtensionIngest_BookmarkRoundtrip` | Full stack POST → publish → query; artifact present within 60 s p95 |
| Adversarial regression (BS-002 twin) | integration | `internal/api/connectors/extension/ingest_integration_test.go` | `TestIngest_PartialScopeMatch_Rejected` | Token with ONLY `extension:bookmarks` (not `extension:history`) → 403; proves AND-semantics, not subset-tolerant |

### Definition of Done

- [ ] `internal/config/extension.go` exists with `ExtensionIngestConfig` and fail-loud `Validate()`; all 6 fields covered by `TestExtensionIngestConfig_Validate_RejectsEachMissingField`.
- [ ] `config/smackerel.yaml` declares the `extension.ingest.*` block per design §6; `./smackerel.sh config generate` regenerates env files cleanly.
- [ ] `internal/api/connectors/extension/ingest.go` returns a handler that enforces `MaxBodyBytes` (413), `MaxBatchItems` (422), `DisallowUnknownFields` (400), per-item validation (200 with mixed outcomes), and calls `ArtifactPublisher.PublishRawArtifact` on success.
- [ ] Router mounts the handler under `auth.RequireScope("extension:bookmarks", "extension:history")` inside the existing `bearerAuthMiddleware` group; legacy spec-044 tokens are rejected with 403 `scope_required`.
- [ ] `DedupStore` interface seam exists in `internal/connector/ingest/dedup.go` with a no-op pass-through implementation; Scope 2 replaces with Postgres-backed impl.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (SCN-058-001 through SCN-058-005 mapped above).
- [ ] Adversarial regression: `TestIngest_PartialScopeMatch_Rejected` proves AND-semantics is exact, not substring/subset.
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`).
- [ ] Build Quality Gate: `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration` all clean; zero deferrals.

---

## Scope 2: Server Dedup Table + Keyer + Upsert Path

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1
**Surfaces:** `internal/db/migrations/NNN_create_raw_ingest_dedup.sql` (new), `internal/connector/ingest/dedup.go` (Postgres impl), `internal/api/connectors/extension/ingest.go` (retrofit).

### Shared Infrastructure Impact Sweep

The dedup keyer is a high-fan-out contract: every future ingestion path that wants server-authoritative dedup will key off `ComputeDedupKey`. Downstream contract surfaces:

- Wire schema for `Metadata.source_device_id` (extension is the first producer; future mobile-capture / share-extension flows will reuse this key tuple).
- The bookmark-bucket-fixed-at-0 rule is asymmetric with history bucketing; future content types added to `AcceptedContentTypes` MUST declare their bucketing rule in this same module to avoid silent divergence.
- Migration order is permanent; any rollback affects only this feature (no downstream readers in scope 058).

**Canary:** `TestComputeDedupKey_VariesByDevice` is the canary that proves the Chrome Sync case (same URL, different device) never collapses; it MUST run before any broader integration suite re-runs.

### Gherkin Scenarios

#### SCN-058-006: Two history visits within the dedup window collapse to one artifact

```gherkin
Given two browser_history_visit items with identical URL, content_type, and source_device_id
And both captured_at timestamps fall in the same floor(t/window) bucket
When the operator POSTs both in one batch (or two batches)
Then ArtifactPublisher.PublishRawArtifact is called exactly once
And the raw_ingest_dedup row for that key has visit_count = 2
And the second item's response outcome is "deduped" with the SAME artifact_id as the first
```

#### SCN-058-007: Two history visits across the dedup window produce two artifacts

```gherkin
Given two browser_history_visit items with identical URL/content_type/device
And the second captured_at falls in a different floor(t/window) bucket
When the operator POSTs them
Then ArtifactPublisher.PublishRawArtifact is called exactly twice
And two distinct raw_ingest_dedup rows exist with distinct artifact_ids
```

#### SCN-058-008: Chrome Sync — same URL across two source_device_id values produces two artifacts

```gherkin
Given two browser_history_visit items with identical URL/content_type/captured_at
And source_device_id values "laptop" and "work-desktop"
When the operator POSTs them
Then two distinct dedup_key rows exist
And ArtifactPublisher.PublishRawArtifact is called exactly twice
```

#### SCN-058-009: Bookmark dedup key uses bucket=0 regardless of timestamp

```gherkin
Given two bookmark items with identical url/content_type/source_device_id
And captured_at timestamps 24 hours apart
When the operator POSTs them
Then both items map to the SAME dedup_key
And ArtifactPublisher.PublishRawArtifact is called exactly once
And the second item's outcome is "deduped"
```

### Implementation Plan

- Forward migration `internal/db/migrations/NNN_create_raw_ingest_dedup.sql` per design §2.3; assign N during implementation.
- `internal/connector/ingest/dedup.go` (replace no-op from Scope 1):
  - `ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte` — `sha256.Sum256([]byte(url + "\x00" + contentType + "\x00" + deviceID + "\x00" + strconv.FormatInt(bucket, 10)))`; explicit null-byte separators prevent boundary-collision attacks.
  - `NewPostgresDedupStore(db *sql.DB).Upsert` — uses `INSERT ... ON CONFLICT (dedup_key) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, visit_count = raw_ingest_dedup.visit_count + 1 RETURNING artifact_id, (xmax = 0) AS inserted`; the `xmax = 0` trick distinguishes insert vs update.
- Handler retrofit: between per-item validation and publish, call `DedupStore.Upsert`; on `deduped=true`, skip publish and return `outcome:"deduped"` with the existing `artifact_id`; on `deduped=false`, call `PublishRawArtifact` and return `outcome:"accepted"`.
- Bucket computation:
  - Bookmarks: `bucket = 0` always.
  - History: `bucket = capturedAt.Unix() / windowSeconds` where `windowSeconds = item.Metadata["dedup_window_seconds"]` (clamped to `[60, 86400]`) or `cfg.DefaultDedupWindowSeconds` when absent.

### Test Plan

| Scenario | Test type | File | Test name | Assertion |
|----------|-----------|------|-----------|-----------|
| (keyer determinism) | unit | `internal/connector/ingest/dedup_test.go` | `TestComputeDedupKey_Deterministic` | Same input → identical output across 1000 invocations |
| SCN-058-008 (canary) | unit | `internal/connector/ingest/dedup_test.go` | `TestComputeDedupKey_VariesByDevice` | Same URL/content_type/bucket + different device → different keys |
| SCN-058-007 | unit | `internal/connector/ingest/dedup_test.go` | `TestComputeDedupKey_VariesByBucket` | Same URL/content_type/device + different bucket → different keys |
| SCN-058-006 | integration | `internal/connector/ingest/dedup_integration_test.go` | `TestUpsertDedup_CollisionIncrementsVisitCount` | Live Postgres: two upserts on same key → row has `visit_count=2`; second call returns existing `artifact_id` with `deduped=true` |
| SCN-058-009 | integration | `internal/connector/ingest/dedup_integration_test.go` | `TestUpsertDedup_BookmarkBucketAlwaysZero` | Two bookmark items 24 h apart → same dedup_key; publisher called once |
| SCN-058-006 + SCN-058-007 + SCN-058-008 | e2e-api | `tests/e2e/extension_dedup_e2e_test.go` | `TestE2E_ExtensionDedup_WindowAndDeviceMatrix` | Full stack: within-window collapse, across-window split, Chrome-Sync split all produce correct artifact counts |
| Regression E2E | e2e-api | `tests/e2e/extension_dedup_e2e_test.go` | `TestE2E_ExtensionDedup_Regression_BookmarkBypassesWindow` | Persistent regression guard: bookmark dedup MUST ignore window (would fail if bucket logic regressed to use timestamp for bookmarks) |
| Adversarial regression | unit | `internal/connector/ingest/dedup_test.go` | `TestComputeDedupKey_BoundaryCollisionResistance` | Inputs designed to collide if separator were absent (e.g. `("a", "bc", ...)` vs `("ab", "c", ...)`) produce different keys |

### Definition of Done

- [ ] Migration `NNN_create_raw_ingest_dedup.sql` exists and is reversible via `DROP TABLE`.
- [ ] `ComputeDedupKey` is deterministic, varies by device, varies by bucket, and is boundary-collision resistant.
- [ ] `NewPostgresDedupStore.Upsert` correctly distinguishes insert vs update via `xmax = 0` and returns existing `artifact_id` on collision.
- [ ] Handler retrofit calls `Upsert` between validation and publish; `outcome:"deduped"` is returned on collision and `PublishRawArtifact` is NOT called.
- [ ] Bookmark bucket is fixed at `0`; per-request `dedup_window_seconds` is clamped to `[60, 86400]` with fallback to SST `default_dedup_window_seconds`.
- [ ] Scenario-specific E2E regression tests for SCN-058-006 through SCN-058-009.
- [ ] Adversarial regression: `TestComputeDedupKey_BoundaryCollisionResistance` proves separator hygiene.
- [ ] Independent canary suite (`TestComputeDedupKey_VariesByDevice` + Chrome-Sync e2e row) passes before broad suite reruns.
- [ ] Rollback path documented (DROP TABLE) and verified by replaying migration down.
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate clean.

---

## Scope 3: Chrome MV3 Extension Skeleton + Background WAL + Options Page

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1 (endpoint exists for integration tests)
**Surfaces:** `extensions/chrome-bridge/` (entire directory: `manifest.json`, `package.json`, `tsconfig.json`, `esbuild.config.mjs`, `src/background/*`, `src/options/*`, `src/common/*`, `test/unit/*`).

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| Options page first-run | Extension freshly installed, `chrome.storage.local` empty | Open options page | Form renders empty; badge shows "SETUP"; bearer token field is masked | e2e-ui (Playwright) | report.md#scn-058-010 |
| Save options + immediate badge clear | Operator pastes base_url, bearer_token, source_device_id | Click Save | `chrome.storage.local` persists keys; badge clears; listeners activate | e2e-ui (Playwright) | report.md#scn-058-011 |
| Bookmark add → artifact within 60 s | Extension configured + scoped token | Add a bookmark in Chrome | Artifact visible via `/v1/artifacts?source=browser-extension` within 60 s | e2e-ui (Playwright + live stack) | report.md#scn-058-012 |
| Revoked token → badge surfaces error | Server-side revoke token via spec 044 | Add a bookmark | Next POST returns 401; badge shows "AUTH" | e2e-ui (Playwright) | report.md#scn-058-013 |

### Gherkin Scenarios

#### SCN-058-010: Privacy filter drops denied URL before enqueue

```gherkin
Given the operator has configured deny_pattern "^https://bank\\.example\\.com/"
When the user visits https://bank.example.com/account
Then the background service worker drops the event before enqueue
And no IndexedDB WAL row is written
And no network request leaves the browser
```

#### SCN-058-011: Dwell threshold filters out short visits

```gherkin
Given the operator dwell_threshold_seconds = 120
And the user opens a tab, navigates to https://example.com, closes after 30 s
When chrome.history.onVisited fires for that visit
Then no item is enqueued (dwell < threshold)
```

#### SCN-058-012: IndexedDB WAL persists across service-worker eviction

```gherkin
Given the queue contains 5 unsent items
And the background service worker is evicted
When the operator network reconnects and chrome.alarms re-wakes the worker
Then drain() reads all 5 items from IndexedDB
And POSTs them in a single batch
And rows whose response outcome is "accepted" or "deduped" are removed
```

#### SCN-058-013: Exponential backoff curve respects design §4.2

```gherkin
Given the server returns HTTP 503
When the drain loop encounters 7 consecutive failures
Then the retry intervals follow {1s, 2s, 5s, 15s, 60s, 5m, 30m} (with ±10% jitter tolerance)
And the 8th failure caps at 24 h and surfaces the dead-letter badge
```

#### SCN-058-014: Transport error mapping per design §3.1 table

```gherkin
Given the server returns one of {401, 403, 413, 422, 400}
When the drain loop processes the response
Then 401/403 mark all queued items terminal and surface the badge (no retry)
And 413/422/400 mark only the offending item rejected (no retry)
And 5xx + network errors mark items retryable
```

#### SCN-058-015: Corrupted IndexedDB row is skipped without losing neighbors

```gherkin
Given the WAL contains items [A, B (corrupted JSON), C, D]
When drain() iterates the queue
Then A, C, D are POSTed normally
And B is logged once + removed from the queue
And no exception propagates to the service worker top-level
```

### Implementation Plan

- Bootstrap `extensions/chrome-bridge/` with `package.json` (esbuild, vitest, fake-indexeddb, @types/chrome), `tsconfig.json` (target ES2022, module ES2022), `esbuild.config.mjs` (two entry points: `src/background/index.ts` → `dist/background.js`; `src/options/index.ts` → `dist/options/index.js`; copy `manifest.json`, `src/options/index.html`, `icons/`).
- `manifest.json` (MV3): `manifest_version: 3`, `permissions: ["bookmarks", "history", "storage", "alarms"]`, `host_permissions: []` (operator base URL added dynamically at install? NO — CSP `connect-src` must be set at manifest time; document that operator MUST edit manifest before sideload OR fall back to `host_permissions: ["<all_urls>"]` constrained by `connect-src`). Pin `minimum_chrome_version` per the latest stable Chrome supporting MV3 features used.
- `src/common/schema.ts`: mirror `RawArtifact` JSON shape + per-content-type `Metadata` typings; types are the source of truth on the extension side.
- `src/background/queue.ts`: IndexedDB wrapper using a single object store `wal` keyed by `client_event_id`; methods `enqueue(item)`, `peekBatch(maxItems, maxBytes)`, `markOutcome(clientEventID, outcome)`, `removeCorrupted(clientEventID)`.
- `src/background/transport.ts`: `postBatch(items)` returns `Promise<IngestItemOutcome[]>`; maps HTTP status to retryable vs terminal per design §3.1; never logs the bearer token.
- `src/background/privacy_filter.ts`: compiles operator allow/deny pattern arrays into `RegExp[]` once at startup; serialized form cached in `chrome.storage.local` to survive SW eviction; rejects pattern arrays over 64 entries at options-save time.
- `src/background/index.ts`: registers `chrome.bookmarks.{onCreated,onChanged,onRemoved,onMoved}` and `chrome.history.{onVisited,onVisitRemoved}` at top level; `chrome.alarms.create("smackerel-bridge-drain", { periodInMinutes: 1 })`; alarm handler calls `drain()`.
- `src/options/index.ts`: form with masked bearer-token field + "Reveal" toggle; on Save, validates all fields per design §4.4 before writing `chrome.storage.local`; sets badge `SETUP` when token or base URL missing.

### Test Plan

| Scenario | Test type | File | Test name | Assertion |
|----------|-----------|------|-----------|-----------|
| SCN-058-010 | unit (vitest) | `test/unit/privacy_filter.spec.ts` | `dropsDeniedURLBeforeEnqueue` | Pattern match returns drop; enqueue spy NEVER called |
| SCN-058-010 (cap) | unit (vitest) | `test/unit/privacy_filter.spec.ts` | `rejectsPatternArrayOver64` | Save with 65 patterns throws ValidationError |
| SCN-058-011 | unit (vitest) | `test/unit/dwell_gate.spec.ts` | `dropsVisitBelowThreshold` | dwell=30s, threshold=120s → dropped |
| SCN-058-012 | unit (vitest) | `test/unit/queue.spec.ts` | `persistsAcrossSWEviction` | fake-indexeddb persists; re-import module reads all 5 items |
| SCN-058-013 | unit (vitest) | `test/unit/backoff.spec.ts` | `followsDesignedCurve` | 7 failures → intervals match design §4.2 within ±10% jitter |
| SCN-058-014 | unit (vitest) | `test/unit/transport.spec.ts` | `mapsHTTPStatusToOutcome` | All 5 status families produce correct terminal/retryable classification |
| SCN-058-015 | unit (vitest) | `test/unit/queue.spec.ts` | `skipsCorruptedRow` | A,B(corrupted),C,D → A/C/D POSTed; B removed; no exception |
| Adversarial: mismatched device-id (§9.3 twin) | unit (vitest) | `test/unit/queue.spec.ts` | `dedupKeyTupleIncludesDeviceID` | Two items same URL different device → local dedup does NOT collapse (server-authoritative) |
| Bookmark roundtrip | e2e-ui | `test/e2e/bookmark_roundtrip.spec.ts` | `bookmarkAddVisibleWithin60s` | Playwright loads unpacked extension into headless Chromium against the live test stack; bookmark add → artifact visible in `/v1/artifacts?source=browser-extension` within 60 s |
| Regression E2E | e2e-ui | `test/e2e/bookmark_roundtrip.spec.ts` | `Regression_OfflineQueueFlushOnReconnect` | Persistent regression: simulate offline → add 3 bookmarks → bring online → all 3 artifacts present |
| Revoked token surfaces badge | e2e-ui | `test/e2e/auth_failure.spec.ts` | `revokedTokenSetsBadgeAUTH` | Revoke token via spec-044 admin endpoint → next POST → badge shows "AUTH" |

### Definition of Done

- [ ] `extensions/chrome-bridge/` directory exists with the full module layout from design §4.1.
- [ ] `manifest.json` is MV3 with the minimum permission set (`bookmarks`, `history`, `storage`, `alarms`); CSP `connect-src` is restrictive.
- [ ] Background service worker registers all listeners at top level; `chrome.alarms` 1-min wake guarantee in place.
- [ ] Privacy filter drops denied URLs BEFORE enqueue; pattern cap of 64 enforced at options-save.
- [ ] Dwell gate enforces operator threshold; history events below threshold are NOT enqueued.
- [ ] IndexedDB WAL persists across SW eviction (verified with fake-indexeddb).
- [ ] Backoff curve matches design §4.2; dead-letter badge surfaces after 24 h cap.
- [ ] Transport error mapping per design §3.1 table is fully covered by unit tests.
- [ ] Options page masks bearer token by default; Save validates per design §4.4.
- [ ] Scenario-specific E2E regression tests for SCN-058-010 through SCN-058-015.
- [ ] Adversarial regression: `skipsCorruptedRow` proves drainer resilience; `dedupKeyTupleIncludesDeviceID` proves device-id is part of the local dedup key.
- [ ] Bookmark roundtrip E2E (Playwright + live stack) passes within 60 s p95.
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: `cd extensions/chrome-bridge && npm test` clean; esbuild build produces working `dist/` output.

---

## Scope 4: Build/Release Wiring — `./smackerel.sh build --extension chrome-bridge` + CI Signed Zip + Build-Manifest Entry

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 3 (extension source exists to build)
**Surfaces:** `scripts/commands/build-chrome-bridge.sh` (new), `smackerel.sh` (dispatch arm), `.github/workflows/build.yml` (new job).

### Change Boundary

This is a narrow build-pipeline addition. Allowed file families:

- `scripts/commands/build-chrome-bridge.sh` (new).
- `smackerel.sh` — single new dispatch case for `build --extension chrome-bridge`.
- `.github/workflows/build.yml` — new `build-chrome-bridge` job appended after the core image build.
- `docs/Operations.md` — release-channel note (download from GitHub Release for the SHA) — overlaps with Scope 5; Scope 4 may add a stub paragraph that Scope 5 expands.

Excluded surfaces (MUST NOT change):

- `scripts/commands/package-extension.sh` (existing share-extension pipeline — design §8.1 mandates "left untouched").
- `web/extension/` (share-only extension).
- Any other `.github/workflows/*.yml` file.
- Any runtime Go or TypeScript source under `internal/`, `cmd/`, `extensions/chrome-bridge/src/`.

### Gherkin Scenarios

#### SCN-058-016: `./smackerel.sh build --extension chrome-bridge` produces a versioned signed zip

```gherkin
Given a clean working tree at git SHA <sha> with manifest.json version "0.1.0"
When the operator runs `./smackerel.sh build --extension chrome-bridge`
Then dist/extension/smackerel-chrome-bridge-0.1.0-<sha>.zip is created
And dist/extension/smackerel-chrome-bridge-0.1.0-<sha>.zip.sha256 contains its SHA-256 digest
And exit code is 0
```

#### SCN-058-017: Two CI runs on the same SHA produce byte-identical zips

```gherkin
Given two independent CI runs on the same git SHA <sha>
When `./smackerel.sh build --extension chrome-bridge` runs in each
Then both zips have identical SHA-256 digests
And both digests are recorded in build-manifest-<sha>.yaml
```

#### SCN-058-018: CI signs the zip with cosign keyless and publishes to GitHub Release

```gherkin
Given the build-chrome-bridge CI job has produced the zip
When the cosign sign-blob step runs
Then a .sig artifact is uploaded alongside the zip
And both the zip, .sha256, and .sig are attached to the GitHub Release for the SHA
And the zip SHA-256 is appended to build-manifest-<sha>.yaml under chrome_bridge_zip_sha256
```

### Implementation Plan

- `scripts/commands/build-chrome-bridge.sh`:
  1. `cd extensions/chrome-bridge`
  2. `npm ci` (lockfile MUST be committed)
  3. `node esbuild.config.mjs` (production mode, minify, sourcemap inline)
  4. Read `manifest.json` version + current git SHA (`git rev-parse --short=12 HEAD`)
  5. `zip -r -X dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip dist/extension/chrome-bridge/` (`-X` strips extra file attrs for byte-reproducibility)
  6. `sha256sum dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip > dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip.sha256`
- `smackerel.sh`: add a `build)` sub-dispatch arm that checks for `--extension chrome-bridge` and forwards to the script; preserve existing `build` behavior for the core image build.
- `.github/workflows/build.yml`: add `build-chrome-bridge` job with `needs: [build-core-image]`, runs on the same checkout/SHA; steps:
  1. Run the build script.
  2. `cosign sign-blob --yes --output-signature <zip>.sig <zip>` (OIDC identity from GitHub Actions; Rekor public log entry).
  3. Upload `<zip>`, `<zip>.sha256`, `<zip>.sig` as workflow artifacts.
  4. On `release` event: attach the three files to the GitHub Release for the tag.
  5. Append `chrome_bridge_zip_sha256: <hex>` to `build-manifest-<sourceSha>.yaml`.

### Test Plan

| Scenario | Test type | File | Test name | Assertion |
|----------|-----------|------|-----------|-----------|
| SCN-058-016 | functional | `scripts/commands/build-chrome-bridge_test.sh` | `script_produces_versioned_zip_and_sha256` | Run script in a clean fixture; assert both output files exist with expected names |
| SCN-058-017 | CI smoke | `.github/workflows/build.yml` (matrix: 2 runs on same SHA) | `build_chrome_bridge_reproducibility` | Both runs upload zips; CI step `sha256sum` proves byte-identical |
| SCN-058-018 | CI smoke | `.github/workflows/build.yml` | `build_chrome_bridge_signs_and_publishes` | `.sig` artifact present; `cosign verify-blob --signature <sig> --certificate-identity ... <zip>` succeeds in a verify step |
| Regression E2E | CI | `.github/workflows/build.yml` | `Regression_BuildManifestRecordsZipSHA256` | Persistent regression: `build-manifest-<sha>.yaml` contains the `chrome_bridge_zip_sha256` key with the matching value |
| Change Boundary | grep | `scripts/commands/package-extension.sh` (mtime check in CI) | `share_extension_pipeline_untouched` | `git log --since=<spec-058-merge-date> -- scripts/commands/package-extension.sh web/extension/` returns no commits in Scope 4 |

### Definition of Done

- [ ] `scripts/commands/build-chrome-bridge.sh` exists and produces a versioned zip + `.sha256` deterministically.
- [ ] `smackerel.sh build --extension chrome-bridge` dispatches to the script and exits 0 on success.
- [ ] `.github/workflows/build.yml` `build-chrome-bridge` job runs after the core image build, signs the zip with cosign keyless (Rekor), and uploads zip + `.sha256` + `.sig`.
- [ ] `build-manifest-<sourceSha>.yaml` contains `chrome_bridge_zip_sha256` matching the published artifact.
- [ ] Two CI runs on the same SHA produce byte-identical zips (verified by CI matrix smoke).
- [ ] `cosign verify-blob` against the released artifact succeeds with the expected certificate identity.
- [ ] Change Boundary respected: `scripts/commands/package-extension.sh` and `web/extension/` unchanged.
- [ ] Scenario-specific E2E/CI regression rows for SCN-058-016 through SCN-058-018.
- [ ] Persistent regression row `Regression_BuildManifestRecordsZipSHA256` proves build-manifest contract.
- [ ] Build Quality Gate clean.

---

## Scope 5: Operator Docs + Devices Admin View

**Status:** Not Started
**Priority:** P1
**Depends On:** Scopes 1, 2, 4
**Surfaces:** `docs/Operations.md`, `docs/API.md`, `internal/api/admin/devices.go` (new), `web/` admin page (minimal), `internal/api/router.go` (mount).

### Consumer Impact Sweep

Spec 058 introduces a new public API surface (`/v1/connectors/extension/ingest`) and a new admin surface (`/v1/admin/extension/devices`). Affected consumer-facing surfaces:

- `docs/API.md` — new endpoint sections + auth matrix (§5.2).
- `docs/Operations.md` — sideload runbook, cosign verify workflow, options-page setup, OQ-DSN-2/OQ-DSN-3 caveats.
- `web/` admin navigation — new "Extension Devices" link.
- Stale-reference scan: confirm no documentation references a hypothetical legacy `/v1/extensions/*` path (would indicate a fork of the API namespace).

### Gherkin Scenarios

#### SCN-058-019: Operator can sideload the extension following Operations.md

```gherkin
Given an operator with a fresh Chromium browser and the documented prerequisites
When they follow docs/Operations.md "Chrome Extension Bridge — Sideload Workflow" step-by-step
Then the extension loads without errors
And cosign verify-blob succeeds against the released artifact
And the options page accepts a PASETO minted via `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history`
```

#### SCN-058-020: `GET /v1/admin/extension/devices` aggregates seeded devices

```gherkin
Given raw_ingest_dedup contains rows for source_device_id values ["laptop", "work-desktop"]
And the calling token has the admin scope (existing spec 044 admin path)
When GET /v1/admin/extension/devices is called
Then the response contains 2 device entries
And each entry has first_seen_at, last_seen_at, visit_count_30d
```

#### SCN-058-021: API.md documents the endpoint + authorization matrix

```gherkin
Given docs/API.md contains a "Chrome Extension Bridge Ingestion" section
When a reviewer reads the section
Then the request body, max batch, max body, and per-item response shape match design §3.1
And the authorization matrix matches design §5.2
And the 403 scope_required response shape matches spec 060
```

### Implementation Plan

- `docs/Operations.md`: add "Chrome Extension Bridge — Sideload Workflow" subsection covering: download from GitHub Release for the running smackerel-core SHA; `cosign verify-blob --signature <sig> --certificate-identity '...' <zip>`; `chrome://extensions` → enable Developer Mode → Load Unpacked (extracted zip); options-page form completion (base URL, paste PASETO from `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history`, set `source_device_id`); operator-runbook caveats for OQ-DSN-2 (offline revocation latency — operator can also disable in `chrome://extensions`) and OQ-DSN-3 (pattern cap of 64).
- `docs/API.md`: add "Chrome Extension Bridge Ingestion" section with request/response shape per design §3.1, error-code table, and the per-endpoint authorization matrix from design §5.2; add `GET /v1/admin/extension/devices` documentation per design §3.2.
- `internal/api/admin/devices.go`: implement `NewDevicesHandler` returning the aggregated `SELECT DISTINCT source_device_id, MIN(first_seen_at), MAX(last_seen_at), SUM(visit_count) FILTER (WHERE last_seen_at > now() - interval '30 days') FROM raw_ingest_dedup WHERE source_id = 'browser-extension' [AND owner_user_id = ?] GROUP BY source_device_id`. Mount under the existing admin auth middleware; non-admin users see only their own devices.
- `web/` admin: add a minimal page (HTMX, consistent with existing admin pages) that fetches `/v1/admin/extension/devices` and renders a table.

### Test Plan

| Scenario | Test type | File | Test name | Assertion |
|----------|-----------|------|-----------|-----------|
| SCN-058-020 | integration | `internal/api/admin/devices_integration_test.go` | `TestDevicesHandler_AggregatesSeededDevices` | Seed 2 device rows → handler returns 2 entries with correct aggregates |
| SCN-058-020 | integration | `internal/api/admin/devices_integration_test.go` | `TestDevicesHandler_NonAdminSeesOnlyOwnDevices` | Non-admin caller filtered to their own `owner_user_id` |
| SCN-058-019 | docs review + functional | manual + `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/058-chrome-extension-bridge --verbose` | `docs_changes_registered` | Guard proves managed-doc changes are tracked |
| SCN-058-021 | grep | `tests/docs/api_md_extension_section_test.sh` | `api_md_has_extension_section_with_403_scope_required` | grep proves the section exists and references `scope_required` body shape |
| Regression E2E | e2e-api | `tests/e2e/admin_devices_e2e_test.go` | `Regression_AdminDevicesViewReturnsSeededDevices` | Persistent regression against the live test stack |
| Consumer-trace | grep | `tests/docs/no_legacy_extension_path_test.sh` | `no_v1_extensions_path_references` | grep proves no doc/source references `/v1/extensions/` (would indicate API namespace fork) |

### Definition of Done

- [ ] `docs/Operations.md` "Chrome Extension Bridge — Sideload Workflow" subsection exists and covers download, cosign verify, load-unpacked, options-page setup, and OQ-DSN-2/OQ-DSN-3 caveats.
- [ ] `docs/API.md` documents `POST /v1/connectors/extension/ingest` (request/response/errors per design §3.1) and `GET /v1/admin/extension/devices` (per design §3.2), plus the authorization matrix from §5.2.
- [ ] `internal/api/admin/devices.go` returns aggregated device entries; non-admin callers see only their own.
- [ ] Minimal `web/` admin page renders the devices table.
- [ ] `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/058-chrome-extension-bridge --verbose` passes (managed-doc changes registered).
- [ ] Consumer Impact Sweep complete: zero stale `/v1/extensions/*` references; admin navigation updated.
- [ ] Scenario-specific E2E regression tests for SCN-058-019 through SCN-058-021.
- [ ] Persistent regression: `Regression_AdminDevicesViewReturnsSeededDevices`.
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate clean.

---

## Cross-Spec Coordination Notes

- **Spec 044:** No further dependencies in 058; the per-user PASETO flow is consumed unchanged.
- **Spec 060:** Scopes 1 + 2 (foundation: `scope` claim + `auth.RequireScope`) are SHIPPED and consumed by 058 Scope 1. Spec 060 Scope 3 (CLI `--scope` flags) is referenced in 058 Scope 5 docs but is NOT a blocking dependency for 058 Scopes 1–4 (an operator can mint scoped tokens by editing config during the gap; doc note added in 058 Scope 5).
- **Spec 020 (security hardening):** The tailnet-edge bind pattern is preserved; the new endpoint inherits the existing host-bind contract for `smackerel-core`.

## Out of Scope (carried from spec.md)

- Firefox / Safari extensions.
- Mobile Chrome.
- Public Chrome Web Store distribution.
- Page content capture in the extension (server-side `content_fetch_*` remains the canonical surface).
- Per-extension token endpoints (NC-1 binds auth to spec 044).
