# Design: Cloud Photo Libraries (Immich, Google Photos, Amazon Photos, …)

> Author: bubbles.design (mode: from-analysis). Inputs: [spec.md](spec.md) (analyst + UX). Owns ONLY this file. `scopes.md`, `report.md`, `uservalidation.md` are owned by `bubbles.plan`.

---

## Design Brief

**Current State.** The Smackerel core runtime already has a connector framework (`internal/connector/`), an artifact pipeline (`internal/pipeline/`), a versioned API surface under `/v1` (`internal/api/router.go`), a Python ML sidecar (`ml/app/`) wired through a shared NATS contract (`config/nats_contract.json`), and PostgreSQL with pgvector for canonical artifact + vector storage. There is no photo-native connector, no provider-capability model, no lifecycle/duplicate/quality intelligence, and no photo-specific NATS subjects. Feature 038 (generic cloud drives — Drive/Dropbox/OneDrive) is the sibling for plain-file storage; this feature owns dedicated photo libraries.

**Target State.** A provider-neutral `PhotoLibrary` connector family rooted on Immich (full-capability reference), with adapters for Google Photos, Amazon Photos, Apple iCloud Photos, Ente, and PhotoPrism. Every photo is canonicalized into the existing `artifacts` table plus a new `photos.*` schema that captures provider identity, media role, lifecycle state (`unprocessed|processed|published|archived|removal_candidate`), duplicate cluster membership, quality, sensitivity, and per-provider capabilities. Intelligence (caption, OCR, lifecycle pairing, duplicate best-pick, removal rationale, sensitivity, aesthetic/quality) runs in the Python ML sidecar via new `photos.*` NATS subjects. Heuristics are scoped to a small, named allow-list (FR-004A); everything else is LLM. Write-back to providers is gated by a runtime capability matrix and never silently degrades (FR-019). Destructive and low-confidence actions funnel through a single confirmation contract (FR-020).

**Patterns to Follow.**
- Connector lifecycle: `internal/connector/connector.go` (`Connector`, `RawArtifact`, `HealthFromErrorCount`, `ArtifactPublisher`).
- Subject contract: `config/nats_contract.json` is the SST for NATS subjects/streams; both Go and Python validate against it (`internal/nats/`, `ml/app/nats_contract.py`). New `photos.*` subjects MUST land here.
- API versioning + error style: `internal/api/router.go` mounts `/v1` routes behind bearer auth; `internal/api/capture.go` defines the repo-standard `{ "error": { "code", "message" } }` error envelope via `writeError` / `writeJSON`.
- Pipeline shape: `internal/pipeline/subscriber.go` (request/response with JetStream consumers, dead-letter handling) and `internal/pipeline/synthesis_subscriber.go` (multi-stage chained subjects).
- Connector implementations: `internal/connector/keep/`, `internal/connector/twitter/`, `internal/connector/maps/` for read-mostly, OAuth/token-based providers; `internal/connector/discord/` for streaming/event sources.
- Config SST: every value originates from `config/smackerel.yaml` and is regenerated into `config/generated/dev.env|test.env` via `./smackerel.sh config generate`. No hardcoded ports, URLs, hostnames, or fallbacks (instructions/bubbles-config-sst).
- ML sidecar service shape: `ml/app/main.py` registers handlers for NATS subjects and calls `intelligence.py` / `embedder.py` / `ocr.py`.

**Patterns to Avoid.**
- Per-provider connector packages that bake provider-specific decisions into the artifact pipeline. Photo intelligence MUST be provider-agnostic; provider adapters only translate raw provider responses into a normalized `PhotoEvent` and capability flags.
- Heuristic-first classification. Existing `internal/connector/maps/` and some legacy detection paths use pattern matching that is acceptable for stable signals (file extension, MIME, EXIF presence) but MUST NOT be extended to subject classification, lifecycle pairing, duplicate best-pick, or sensitivity. Those are LLM-only with rationale (FR-004, FR-004A).
- Silent skips. Several connectors today log a skip and move on. FR-014 forbids that for photos: every skipped or failed photo MUST be visible in the connector detail panel with reason and a `Retry batch` action.
- Hardcoded provider hostnames or `latest` Docker tags (config SST + Docker freshness instructions). All Immich URLs, API keys, OAuth client IDs come from `config/smackerel.yaml`.
- RFC 7807 problem details for this feature. The repo's active JSON API contract is `ErrorResponse`; photos MUST extend that shape with stable codes and optional metadata instead of introducing a second error-envelope style.

**Resolved Decisions.**
- Storage: extend canonical `artifacts` table with `photos`, `photo_lifecycle_links`, `photo_clusters`, `photo_cluster_members`, `photo_capabilities`, `photo_sync_state`, `photo_face_links`, `photo_removal_candidates`, and `photo_action_tokens`. `photos.artifact_id` is `TEXT` because `artifacts.id` is `TEXT` in `internal/db/migrations/001_initial_schema.sql`; new photo-owned IDs are UUIDs generated by the Go core, matching the newer drive migration style.
- Intelligence transport: new `photos.*` JetStream stream + subjects, owned by ML sidecar; never bypassed by core. The contract must update `config/nats_contract.json`, Go subject constants/`AllStreams`, request-response pair tests, and Python startup validation in one change.
- LLM models: caption + classification + lifecycle reasoning + sensitivity through the configured multimodal model in the LLM gateway. Cloud model use is allowed only when explicitly configured in `config/smackerel.yaml.llm`; aesthetic/quality model is the same multimodal model with a focused prompt and structured-output schema. No separate face model.
- Faces: Smackerel CONSUMES provider face clusters (Immich today; others when they expose them) and never trains face models locally. Privacy non-goal preserved.
- Provider write-back: handled by a `ProviderWriter` interface keyed by capability flags. If a capability is absent, the corresponding API endpoint returns `409 Provider Limitation` with a structured `limitation_code` so UI banners (Screen 15) render the same content everywhere.
- Destructive actions: a single `POST /v1/photos/actions/confirm` endpoint accepts a pre-issued `action_token` minted by any planning endpoint (lifecycle review, duplicate resolve, removal). The token carries the action scope, count, byte size, and confidence range; the confirmation modal in UI is the only mint-then-execute consumer.
- Sensitivity: classification is a coarse per-photo gate in `photos.sensitivity` (`none|sensitive|hidden`) plus multi-label reasons in `photos.sensitivity_labels` (`identity_document`, `medical`, `financial`, `children`, `intimate`, `private_location`). Retrieval pathways (search, Telegram delivery) MUST consult sensitivity before returning preview bytes; never auto-reveal.

**Open Questions** (mirrored to spec's Open Questions and called out in §16 below):
- O-040-D-01: Which Immich API version range should be accepted by the first adapter? Current design requires the adapter to record provider version and fail visibly outside the supported range.
- O-040-D-02: What is the long-term scale target for pgvector partitioning beyond large self-hosted libraries? Current design keeps one embedding table and requires an explicit partitioning decision before tens-of-millions scale.
- O-040-D-03: Which Apple/iCloud path is viable enough for a provider adapter: CloudKit web, export/import, or a local Photos library bridge? Current design treats Apple as read-limited until a provider capability probe proves otherwise.

---

## 1. Purpose & Scope

This design covers the technical realization of the Cloud Photo Libraries feature defined in [spec.md](spec.md), including:

- Provider abstraction across Immich (reference), Google Photos, Amazon Photos, Apple iCloud Photos, Ente, PhotoPrism.
- Bulk + incremental sync into the canonical artifact store.
- LLM-driven classification, OCR, lifecycle pairing, duplicate clustering, aesthetic/quality scoring, and sensitivity classification with stable-signal heuristic boundary (FR-004A).
- Cross-provider unified search and Photo Health (Lifecycle / Duplicates / Removal / Quality / People).
- Cross-feature routing into Recipe (035), Expense (034), Meal Planning (036), Knowledge (025), Annotations (027), Lists (028), Mobile capture (033), Telegram (008), Intelligence delivery (021), Agent tools (037), and Cloud Drives (038).
- Write-back governance with explicit capability visibility (FR-019) and destructive-action confirmation (FR-020).

Non-goals (per spec): editing photos, training face models, training NSFW models, building yet-another self-hosted gallery, large-scale collaborative sharing.

---

## 2. Architecture Overview

```text
                     ┌───────────────────────────────────────────────┐
                     │                Smackerel UI                    │
                     │   (PWA / Web / Browser ext / Telegram bot)     │
                     └───────────────┬───────────────────────────────┘
                                     │ HTTPS (REST + SSE)
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime (Chi)                          │
│                                                                        │
│  internal/connector/photos/   ← per-provider adapters (immich/, gphotos/,│
│   ┌───────┬───────────┬───────┴────────┬──────────┬──────────┐ amazon/,│
│   │immich │ gphotos   │ amazon         │ icloud   │ ente …   │ ente/, │
│   └───┬───┴───┬───────┴───┬────────────┴────┬─────┴────┬─────┘ icloud/)│
│       └───────┴───────────┴────────────────┬┘          │              │
│                                           ▼            │              │
│                  internal/connector/photos/library.go   │              │
│                  (PhotoLibrary, PhotoEvent, Capability) │              │
│                                           │            │              │
│       ┌───────────────────────────────────┼────────────┘              │
│       ▼                                   ▼                            │
│  pipeline/photo_publisher.go      photos/scanner.go                    │
│  (RawArtifact + provider state)   (bulk scan, cursor, retry batches)   │
│       │                                   │                            │
│       └────────────┬──────────────────────┘                            │
│                    ▼                                                   │
│              internal/nats (JetStream)                                 │
│   subjects: artifacts.process / search.embed                           │
│             photos.classify / photos.ocr / photos.embed                │
│             photos.lifecycle / photos.dedupe / photos.aesthetic        │
│             photos.sensitivity / photos.faces                          │
│       │                                   │                            │
│       │                                   ▼                            │
│       │                         ┌────────────────────────┐             │
│       │                         │   Python ML Sidecar    │             │
│       │                         │   ml/app/photos.py     │             │
│       │                         │   (caption, OCR, life- │             │
│       │                         │    cycle, dedupe, aest- │             │
│       │                         │    hetic, sensitivity) │             │
│       │                         │   uses LLM gateway →   │             │
│       │                         │   Ollama (local) /      │             │
│       │                         │   cloud (config-gated) │             │
│       │                         └──────────┬─────────────┘             │
│       │                                    │ ml_to_core results        │
│       ▼                                    ▼                            │
│  PostgreSQL + pgvector  ◀────── result subscribers (pipeline/)          │
│  (artifacts, photos.*, embeddings, capabilities, action_tokens)        │
└──────────────────────────────────────────────────────────────────────┘
```

Key boundaries:

- **Provider adapter** never makes intelligence decisions. It produces `PhotoEvent` records (create/update/delete) and a static-per-connection `Capability` set.
- **Scanner / publisher** publishes `RawArtifact` + `photos.classify` request and persists provider state.
- **ML sidecar** is the single owner of LLM calls and aesthetic/quality scoring. Core never calls Ollama directly for photos.
- **Result subscribers** in core write LLM outputs into `photos.*` tables and emit chained subjects (e.g., a successful `photos.classify` for a RAW triggers a `photos.lifecycle` request).

### 2.1 Go Core vs. Python ML Sidecar Ownership

| Concern | Go core owns | Python ML sidecar owns |
|---|---|---|
| Provider integration | Auth, scope selection, capability probing, bulk scan, monitor cursors, provider writes | None |
| Stable signals | MIME/type sniffing, byte size, EXIF parsing, pHash calculation, content hash, timestamp/window candidate generation | Consumes stable signals as evidence only |
| LLM decisions | Persists accepted results and blocks writes when decisions are missing or low-confidence | Caption, OCR interpretation, document type, routing class, lifecycle final judgment, duplicate best-pick, sensitivity labels, removal rationale |
| Search | Query parsing, authorization, sensitivity filtering, SQL/vector retrieval, response shaping | Embedding generation and optional rerank rationale |
| Failure handling | Skip ledger, retry tokens, dead-letter surfacing, provider limitation responses | Structured error payloads with retryable flag; no silent fallback classification |
| Destructive actions | Plan/confirm token lifecycle and provider writer invocation | Rationale/confidence for proposed actions only; never executes provider mutations |

The boundary is enforced through NATS schemas and tests: Go publishes stable-signal envelopes, Python returns structured decisions, and Go refuses to persist lifecycle/duplicate/removal state when the required `rationale` or `confidence` field is missing.

---

## 3. Provider Abstraction & Capability Matrix

### 3.1 Interfaces

```go
// internal/connector/photos/library.go
package photos

type Capability string
type CapabilityStatus string

const (
    CapRead         Capability = "read"           // list + fetch bytes/thumbnails + metadata
    CapMonitor      Capability = "monitor"        // incremental change feed (push or efficient poll)
    CapWriteAlbum   Capability = "write_album"    // create/update albums + assignments
    CapWriteTag     Capability = "write_tag"      // tags / keywords / labels
    CapWriteFavorite Capability = "write_favorite"
    CapArchive      Capability = "archive"        // move to recoverable archive
    CapDelete       Capability = "delete"         // provider trash/delete path; confirmation-gated
    CapUpload       Capability = "upload"         // push new photos in
    CapFacesRead    Capability = "faces_read"     // consume provider face clusters
    CapFacesWrite   Capability = "faces_write"    // rename/merge/split face clusters
    CapSensitivity  Capability = "sensitivity"    // provider-side hidden/private flags
)

type CapabilityEntry struct {
    Status         CapabilityStatus // "supported" | "limited" | "unsupported" | "unknown"
    LimitationCode string
}

type CapabilityReport struct {
    Provider        string
    ProviderVersion string
    Capabilities    map[Capability]CapabilityEntry
    DetectedAt      time.Time
}

type PhotoEvent struct {
    ProviderRef   string            // provider-native id
    Op            string            // "upsert" | "delete"
    ProviderMediaKind string         // provider-native "photo" | "video" | "live_photo" | ...
    MediaRole     string            // raw_original | camera_original | edited_export | ...
    ContentHash   string            // strong hash if provider exposes it
    Bytes         *int64            // nil if provider does not expose size
    BytesEstimated bool
    MIMEType      string
    Filename      string
    CapturedAt    time.Time
    UploadedAt    time.Time
    GeoLat, GeoLon *float64
    EXIF          map[string]any
    Albums        []string
    Tags          []string
    Faces         []FaceClusterRef  // provider-supplied
    Sensitivity   ProviderSensitivity
    RawProvider   map[string]any    // verbatim provider payload (audit + replay)
}

type PhotoLibrary interface {
    connector.Connector // ID/Connect/Sync/Health/Close
    Capabilities() CapabilityReport
    ProbeCapabilities(ctx context.Context, config connector.ConnectorConfig) (CapabilityReport, error)
    EnumerateScope(ctx context.Context, scope Scope) (<-chan PhotoEvent, <-chan error)
    Watch(ctx context.Context, cursor string) (<-chan PhotoEvent, <-chan error)
    Fetch(ctx context.Context, ref string, kind FetchKind) (io.ReadCloser, FetchMeta, error)
    Writer() ProviderWriter // may be a no-op writer when capabilities are read-only
}

type ProviderWriter interface {
    AddToAlbum(ctx context.Context, photo, album string) error
    Tag(ctx context.Context, photo string, tag string) error
    Favorite(ctx context.Context, photo string, on bool) error
    Archive(ctx context.Context, photo string) error
    Delete(ctx context.Context, photo string) error
    Upload(ctx context.Context, src io.Reader, meta UploadMeta) (string, error)
    RenameFaceCluster(ctx context.Context, cluster string, name string) error
}
```

### 3.2 Capability Matrix (initial; persisted in `photo_capabilities` and rendered to UI banner Screen 15)

| Provider | read | monitor | write_album | write_tag | write_favorite | archive | delete | upload | faces_read | faces_write | sensitivity |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Immich (self-hosted, v1.x) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Google Photos | ✓ | ✓ (poll) | ⚠ create-only via API; no mutate of user-created albums | ✗ | ✗ | ✗ | ✗ | ✓ (own-app albums only) | ✗ | ✗ | ✗ |
| Amazon Photos | ✓ | ⚠ poll-only | ✗ | ✗ | ✗ | ✗ | ✗ | ✓ | ✗ | ✗ | ✗ |
| Apple iCloud Photos | ✓ via iCloud Drive export / CloudKit web | ⚠ poll-only | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Ente | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ⚠ when exposed | ✗ | ✓ |
| PhotoPrism | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ⚠ partial | ✓ |

`⚠` means partial: capability is reported with a `limitation_code` (e.g., `gphotos.album.write.create_only`) that the API/UI surface must explain. `✗` means the capability is absent and the corresponding writer method MUST return `ErrCapabilityUnsupported` with the same code.

### 3.3 Capability Governance

Capabilities are runtime facts, not provider-name assumptions. Each provider adapter implements `ProbeCapabilities(ctx, connectorConfig) (CapabilityReport, error)` and stores the result in `photo_capabilities` during connection test, connection creation, startup health check, and any provider-version change.

```json
{
    "provider": "gphotos",
    "provider_version": "api-v1",
    "capabilities": {
        "read": { "status": "supported" },
        "monitor": { "status": "limited", "limitation_code": "gphotos.monitor.poll_only" },
        "write_album": { "status": "limited", "limitation_code": "gphotos.album.write.create_only" },
        "delete": { "status": "unsupported", "limitation_code": "gphotos.delete.unsupported" }
    },
    "detected_at": "rfc3339"
}
```

Rules:

- Core writer methods must check the stored capability status immediately before every provider mutation.
- `supported` means the provider operation can be executed as requested.
- `limited` means the operation is possible only within a named provider constraint; the API response must include `limitation_code` when a requested action falls outside that constraint.
- `unsupported` means the action is blocked with `409 PROVIDER_LIMITATION` and no provider call is attempted.
- `unknown` is allowed only before the first successful probe; any mutation attempted while `unknown` returns `409 PROVIDER_CAPABILITY_UNKNOWN`.
- Downstream features and UI components consume only the provider-neutral capability report. They must never branch directly on provider names.

### 3.4 Provider adapters (packages)

```
internal/connector/photos/
├── library.go             # interfaces + types
├── publisher.go           # PhotoEvent → artifact + photos.* row, NATS publish
├── scanner.go             # bulk scan, cursor management, retry batches, skip ledger
├── capabilities.go        # capability matrix + limitation codes
├── writer/                # one implementation per provider
│   ├── immich/
│   ├── gphotos/
│   ├── amazon/
│   ├── icloud/
│   ├── ente/
│   └── photoprism/
└── adapters/
    ├── immich/            # full reference adapter (read/monitor/write/upload/faces)
    ├── gphotos/           # OAuth2 + Library API + media items + albums
    ├── amazon/
    ├── icloud/
    ├── ente/              # SDK-based, encrypted
    └── photoprism/
```

---

## 4. Data Model (PostgreSQL + pgvector)

All new tables live in the existing default schema. Migrations follow the repo's existing pattern under `internal/db/migrations/`: one forward SQL file per logical change, with an explicit `-- ROLLBACK:` comment block at the top. The current `artifacts.id` column is `TEXT`, so every cross-feature foreign key to `artifacts` uses `TEXT`; photo-owned IDs are UUIDs generated by Go before insert, matching the newer drive-schema style.

### 4.1 Core photo tables

```sql
-- 040_photo_libraries.up.sql

CREATE TYPE photo_lifecycle_state AS ENUM (
    'unknown',
    'unprocessed',
    'processed',
    'published',
    'archived',
    'removal_candidate'
);

CREATE TYPE photo_media_role AS ENUM (
    'unknown',
    'raw_original',
    'camera_original',
    'edited_export',
    'derived_export',
    'video',
    'document_scan',
    'burst_member',
    'live_photo'
);

CREATE TYPE photo_sensitivity AS ENUM ('none', 'sensitive', 'hidden');

-- One row per logical photo per provider. The same physical content can have
-- multiple rows (one per provider) and is reconciled into clusters.
CREATE TABLE photos (
    id              UUID PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    connector_id    TEXT NOT NULL,
    provider        TEXT NOT NULL,                    -- 'immich' | 'gphotos' | ...
    provider_ref    TEXT NOT NULL,                    -- provider-native id
    provider_media_kind TEXT NOT NULL,                -- provider-native photo | video | live_photo | ...
    media_role      photo_media_role NOT NULL DEFAULT 'unknown',
    mime_type       TEXT NOT NULL,
    bytes           BIGINT,
    bytes_estimated BOOLEAN NOT NULL DEFAULT false,
    filename        TEXT,
    captured_at     TIMESTAMPTZ,
    uploaded_at     TIMESTAMPTZ,
    geo_lat         DOUBLE PRECISION,
    geo_lon         DOUBLE PRECISION,
    content_hash    TEXT,                             -- strong hash if provider exposes
    phash           BYTEA,                            -- 64-bit perceptual hash (heuristic seed)
    exif            JSONB NOT NULL DEFAULT '{}',
    albums          TEXT[] NOT NULL DEFAULT '{}',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    sensitivity     photo_sensitivity NOT NULL DEFAULT 'none',
    sensitivity_labels TEXT[] NOT NULL DEFAULT '{}',  -- identity_document | medical | financial | children | intimate | private_location
    sensitivity_src TEXT NOT NULL DEFAULT 'none',     -- llm | provider | user | rule | none
    lifecycle_state photo_lifecycle_state NOT NULL DEFAULT 'unknown',
    aesthetic_score REAL,                             -- [0,1], LLM-derived
    quality_issues  TEXT[] NOT NULL DEFAULT '{}',     -- ['blurry','underexposed',...]
    classification  JSONB NOT NULL DEFAULT '{}',      -- LLM caption + categories + confidence
    classification_confidence REAL,
    raw_provider    JSONB NOT NULL DEFAULT '{}',      -- verbatim provider payload (audit)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_ref)
);

CREATE INDEX photos_connector_idx        ON photos (connector_id);
CREATE INDEX photos_provider_idx         ON photos (provider);
CREATE INDEX photos_captured_at_idx      ON photos (captured_at);
CREATE INDEX photos_lifecycle_idx        ON photos (lifecycle_state);
CREATE INDEX photos_sensitivity_idx      ON photos (sensitivity);
CREATE INDEX photos_content_hash_idx     ON photos (content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX photos_phash_idx            ON photos USING hash (phash);
CREATE INDEX photos_albums_gin           ON photos USING gin (albums);
CREATE INDEX photos_classification_gin   ON photos USING gin (classification);
```

### 4.2 Lifecycle pairing

```sql
-- A directed edge between a RAW photo and one or more processed/edited exports.
CREATE TABLE photo_lifecycle_links (
    id              UUID PRIMARY KEY,
    raw_photo_id    UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    derived_photo_id UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    editor          TEXT,                             -- 'lightroom_classic' | 'darktable' | ...
    editor_version  TEXT,
    confidence      REAL NOT NULL,
    rationale       TEXT NOT NULL,                    -- LLM rationale (FR-016 / IP-001)
    method          TEXT NOT NULL,                    -- 'stable_signal' | 'llm'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (raw_photo_id, derived_photo_id)
);

CREATE INDEX photo_lifecycle_raw_idx     ON photo_lifecycle_links (raw_photo_id);
CREATE INDEX photo_lifecycle_derived_idx ON photo_lifecycle_links (derived_photo_id);
```

### 4.3 Duplicate clusters

```sql
CREATE TYPE photo_cluster_kind AS ENUM (
    'exact_hash',            -- stable signal (FR-004A allow-list)
    'cross_provider_hash',   -- stable signal
    'burst',                 -- LLM (seeded by EXIF burst id when present)
    'hdr',                   -- LLM
    'panorama_member',       -- LLM
    'near_duplicate'         -- LLM (pHash seed + LLM confirm)
);

CREATE TABLE photo_clusters (
    id              UUID PRIMARY KEY,
    kind            photo_cluster_kind NOT NULL,
    best_photo_id   UUID REFERENCES photos(id) ON DELETE SET NULL,
    best_picked_by  TEXT NOT NULL DEFAULT 'llm',      -- 'llm' | 'user' | 'rule'
    confidence      REAL NOT NULL,
    rationale       TEXT NOT NULL,                    -- LLM rationale (always populated)
    state           TEXT NOT NULL DEFAULT 'open',     -- open | resolved | snoozed
    snoozed_until   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE photo_cluster_members (
    cluster_id      UUID NOT NULL REFERENCES photo_clusters(id) ON DELETE CASCADE,
    photo_id        UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,                    -- 'best' | 'sibling'
    PRIMARY KEY (cluster_id, photo_id)
);

CREATE INDEX photo_clusters_state_idx ON photo_clusters (state);
CREATE INDEX photo_cluster_members_photo_idx ON photo_cluster_members (photo_id);
```

### 4.4 Removal candidates

```sql
CREATE TYPE photo_removal_reason AS ENUM (
    'unprocessed_raw',
    'burst_non_best',
    'blurry',
    'screenshot_transient',
    'cross_provider_duplicate',
    'user_marked'
);

CREATE TABLE photo_removal_candidates (
    id              UUID PRIMARY KEY,
    photo_id        UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    reason          photo_removal_reason NOT NULL,
    rationale       TEXT NOT NULL,                    -- mandatory; LLM or rule text
    confidence      REAL NOT NULL,
    method          TEXT NOT NULL,                    -- 'stable_signal' | 'llm'
    state           TEXT NOT NULL DEFAULT 'open',     -- open | kept | archived | deleted | exempted
    decided_at      TIMESTAMPTZ,
    decided_by      TEXT,                             -- authenticated owner/actor id
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (photo_id, reason)
);

CREATE INDEX photo_removal_state_idx  ON photo_removal_candidates (state);
CREATE INDEX photo_removal_reason_idx ON photo_removal_candidates (reason);
```

### 4.5 Capabilities, sync state, faces, embeddings

```sql
CREATE TABLE photo_capabilities (
    connector_id    TEXT PRIMARY KEY,
    provider        TEXT NOT NULL,
    capabilities    JSONB NOT NULL,                   -- { "write_album": "create_only", ... }
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE photo_sync_state (
    connector_id    TEXT PRIMARY KEY,
    cursor          TEXT,
    last_full_scan_at TIMESTAMPTZ,
    progress        JSONB NOT NULL DEFAULT '{}',      -- {metadata, thumbnails, classify, lifecycle, dedupe}
    skipped         JSONB NOT NULL DEFAULT '[]',      -- visible skip ledger (FR-014)
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE photo_face_links (
    id              UUID PRIMARY KEY,
    photo_id        UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    cluster_ref     TEXT NOT NULL,                    -- provider cluster id
    cluster_name    TEXT,
    provider        TEXT NOT NULL,
    UNIQUE (photo_id, provider, cluster_ref)
);

CREATE TABLE photo_embeddings (
    photo_id        UUID PRIMARY KEY REFERENCES photos(id) ON DELETE CASCADE,
    model_id        TEXT NOT NULL,
    modality        TEXT NOT NULL CHECK (modality IN ('image', 'thumbnail', 'ocr_text')),
    embedding       vector NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX photo_embeddings_vector_idx ON photo_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

### 4.6 Action tokens (FR-020)

```sql
CREATE TABLE photo_action_tokens (
    id              UUID PRIMARY KEY,
    user_id         TEXT NOT NULL,
    action          TEXT NOT NULL,                    -- 'archive' | 'delete' | 'mark_sensitive' | ...
    scope           JSONB NOT NULL,                   -- selector: photo ids / cluster ids / filter
    photo_count     INT NOT NULL,
    bytes_estimate  BIGINT,
    confidence_min  REAL,
    confidence_max  REAL,
    requires_text   BOOLEAN NOT NULL DEFAULT false,   -- delete requires "type DELETE"
    expires_at      TIMESTAMPTZ NOT NULL,
    consumed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX photo_action_tokens_user_idx ON photo_action_tokens (user_id);

CREATE TABLE photo_audit_events (
    id              UUID PRIMARY KEY,
    actor_id        TEXT NOT NULL,
    action          TEXT NOT NULL,                    -- reveal | route | archive | delete | tag | album_write | capability_block
    photo_id        UUID REFERENCES photos(id) ON DELETE SET NULL,
    connector_id    TEXT,
    provider        TEXT,
    outcome         TEXT NOT NULL,                    -- planned | confirmed | blocked | failed | revealed
    reason_code     TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX photo_audit_events_actor_idx ON photo_audit_events (actor_id, created_at DESC);
CREATE INDEX photo_audit_events_photo_idx ON photo_audit_events (photo_id, created_at DESC);
```

### 4.7 Rollback SQL

The migration file must include this rollback block as comments, matching the existing drive migration convention:

```sql
-- ROLLBACK:
--   DROP TABLE IF EXISTS photo_audit_events CASCADE;
--   DROP TABLE IF EXISTS photo_action_tokens CASCADE;
--   DROP TABLE IF EXISTS photo_embeddings CASCADE;
--   DROP TABLE IF EXISTS photo_face_links CASCADE;
--   DROP TABLE IF EXISTS photo_sync_state CASCADE;
--   DROP TABLE IF EXISTS photo_capabilities CASCADE;
--   DROP TABLE IF EXISTS photo_removal_candidates CASCADE;
--   DROP TABLE IF EXISTS photo_cluster_members CASCADE;
--   DROP TABLE IF EXISTS photo_clusters CASCADE;
--   DROP TABLE IF EXISTS photo_lifecycle_links CASCADE;
--   DROP TABLE IF EXISTS photos CASCADE;
--   DROP TYPE IF EXISTS photo_removal_reason;
--   DROP TYPE IF EXISTS photo_cluster_kind;
--   DROP TYPE IF EXISTS photo_sensitivity;
--   DROP TYPE IF EXISTS photo_media_role;
--   DROP TYPE IF EXISTS photo_lifecycle_state;
```

---

## 5. NATS Subjects (extends `config/nats_contract.json`)

Add a new `PHOTOS` stream and the following subjects. Both Go and Python sides validate against the contract on startup (existing pattern). This feature MUST update all four contract surfaces together: `config/nats_contract.json`, `internal/nats/client.go` constants + `AllStreams`, `internal/nats/contract_test.go` request/response pair coverage, and the Python startup validator in `ml/app/nats_contract.py`.

| Subject | Direction | Stream | Critical | Purpose |
|---|---|---|---|---|
| `photos.classify` | core_to_ml | `PHOTOS` | true | Caption + category + confidence + OCR trigger |
| `photos.classified` | ml_to_core | `PHOTOS` | false | Result of `photos.classify` |
| `photos.ocr` | core_to_ml | `PHOTOS` | false | OCR for text-bearing photos and document scans |
| `photos.ocred` | ml_to_core | `PHOTOS` | false | Result of `photos.ocr` |
| `photos.embed` | core_to_ml | `PHOTOS` | true | Multimodal embedding for unified search (FR-015) |
| `photos.embedded` | ml_to_core | `PHOTOS` | false | Result of `photos.embed` |
| `photos.lifecycle` | core_to_ml | `PHOTOS` | false | Pair RAW with derived exports + editor signature |
| `photos.lifecycle.result` | ml_to_core | `PHOTOS` | false | Lifecycle links + rationale |
| `photos.dedupe` | core_to_ml | `PHOTOS` | false | Cluster candidates (seed: pHash + EXIF burst id) |
| `photos.dedupe.result` | ml_to_core | `PHOTOS` | false | Cluster + best-pick + rationale |
| `photos.aesthetic` | core_to_ml | `PHOTOS` | false | Quality + aesthetic score + technical issues |
| `photos.aesthetic.result` | ml_to_core | `PHOTOS` | false | Score + issues |
| `photos.sensitivity` | core_to_ml | `PHOTOS` | false | Sensitivity classification |
| `photos.sensitivity.result` | ml_to_core | `PHOTOS` | false | Sensitivity + source |
| `photos.removal.evaluate` | core_to_ml | `PHOTOS` | false | Build removal-candidate rationale |
| `photos.removal.result` | ml_to_core | `PHOTOS` | false | Reason + rationale + confidence |

Contract JSON additions:

```json
{
    "streams": {
        "PHOTOS": { "subjects_pattern": "photos.>" }
    },
    "request_response_pairs": [
        { "request": "photos.classify", "response": "photos.classified" },
        { "request": "photos.ocr", "response": "photos.ocred" },
        { "request": "photos.embed", "response": "photos.embedded" },
        { "request": "photos.lifecycle", "response": "photos.lifecycle.result" },
        { "request": "photos.dedupe", "response": "photos.dedupe.result" },
        { "request": "photos.aesthetic", "response": "photos.aesthetic.result" },
        { "request": "photos.sensitivity", "response": "photos.sensitivity.result" },
        { "request": "photos.removal.evaluate", "response": "photos.removal.result" }
    ]
}
```

The Python sidecar validator must fail startup when the `PHOTOS` stream, any `photos.*` subject, or any pair above is absent or mapped to the wrong stream. The Go contract tests must fail when a new photo subject exists in the contract without a matching Go constant, or when a Go photo subject constant is missing from the contract.

Subject payload envelope (all requests/responses):

```json
{
    "request_id": "uuid",
    "photo_id": "uuid",
    "artifact_id": "artifact-text-id",
    "connector_id": "string",
    "provider": "immich|gphotos|amazon|icloud|ente|photoprism",
    "provider_ref": "string",
    "media_role": "raw_original|camera_original|edited_export|derived_export|video|document_scan|burst_member|live_photo|unknown",
    "stable_signals": {
        "mime_type": "image/heic",
        "content_hash": "sha256-or-provider-hash",
        "phash": "base64-64bit",
        "exif": {},
        "albums": [],
        "faces": []
    },
    "image_ref": {
        "kind": "thumbnail|preview|original",
        "uri": "internal-object-ref",
        "expires_at": "rfc3339"
    }
}
```

Response payloads MUST echo `request_id`, `photo_id`, `artifact_id`, and include either `result` or `{ "error": { "code", "message", "retryable" } }`. LLM-owned responses (`classified`, `lifecycle.result`, `dedupe.result`, `aesthetic.result`, `sensitivity.result`, `removal.result`) MUST include `confidence` and a non-empty `rationale` whenever they affect lifecycle, duplicate, removal, sensitivity, or routing state.

Pipeline chains (subscriber side):

1. `photos.classify` → `classified` triggers `photos.embed` and (if text-bearing) `photos.ocr`.
2. `photos.classified` for `media_role = raw_original` or `media_role = edited_export` triggers `photos.lifecycle`.
3. `photos.classified` triggers `photos.aesthetic` and `photos.sensitivity` in parallel.
4. New cluster candidates trigger `photos.dedupe`; `dedupe.result` may trigger `photos.removal.evaluate` for `burst_non_best` members.
5. Lifecycle results may trigger `photos.removal.evaluate` for `unprocessed_raw`.

---

## 6. Sync & Scan Pipeline

### 6.1 Bulk scan (UC-001)

```text
Scope confirmed
   │
   ▼
scanner.EnumerateScope()  ← provider adapter yields PhotoEvents
   │
   ▼ for each PhotoEvent
publisher.PublishPhotoEvent()
   ├── upsert into photos (+ artifacts row via existing ArtifactPublisher contract)
   ├── persist raw_provider payload
   ├── update photo_sync_state.progress
    └── publish photos.classify (if media_role is image/document/video-thumbnail classifiable)
                    │
                    ▼
            ML sidecar (ml/app/photos.py)
                    │
                    ▼
photos.classified  → core writes classification + triggers photos.embed,
                       photos.aesthetic, photos.sensitivity, optionally photos.ocr.
photos.lifecycle   → fires for `raw_original`, `edited_export`, or low-confidence lifecycle candidates
photos.dedupe      → fires when new pHash neighbors detected within scope
```

### 6.2 Incremental monitoring (UC-002, FR-003)

- Providers exposing change feeds (Immich, Ente, PhotoPrism): `Watch()` opens a streaming subscription with cursor.
- Providers without push (Google Photos, Amazon Photos, iCloud): adaptive poll using `library_modified_at` + delta tokens. Scheduler interval lives in `config/smackerel.yaml.photos.providers.<provider>.poll_interval_seconds`.
- Health: `HealthFromErrorCount` reused. Failing or degraded surfaces in UI Screen 3.

### 6.3 Skip ledger (FR-014, BS-024, BS-025)

Every skip writes a row into `photo_sync_state.skipped` with `{reason, count, last_seen_at, retry_token}`. The Connector Detail panel reads this directly. Empty libraries report `progress = {metadata: {done:0,total:0,empty:true}}` so UI prints "Library is empty — connector healthy" rather than 0%.

---

## 7. LLM-vs-Heuristic Boundary (FR-004 + FR-004A)

**Heuristics (allow-listed, stable signals only):**

| Signal | Use |
|---|---|
| File extension + MIME | media kind detection |
| Strong content hash (provider-supplied) | exact-hash duplicate cluster seeding |
| pHash 64-bit | near-duplicate cluster seeding (LLM confirms) |
| EXIF `BurstUUID`, `BurstID`, sequence numbers | burst cluster seeding (LLM confirms best-pick) |
| EXIF `Software` field with known editor signature | RAW→edited candidate seeding (LLM confirms) |
| EXIF `DateTimeOriginal` proximity (≤ 2s) within same album | RAW↔derived candidate pairing seed |
| Provider hidden/private flag | sensitivity floor of `sensitive` (LLM may upgrade to `hidden`) |

**LLM (mandatory for everything else):**

- Caption, categories, confidence (with structured output).
- OCR escalation decision and OCR consumption.
- Lifecycle pairing finalization (rationale + confidence).
- Duplicate best-pick selection with rationale per cluster.
- Aesthetic score and technical-issue tagging.
- Sensitivity classification and rationale.
- Removal-candidate rationale (every entry has one).

The Go core MUST NOT make any of the LLM-owned decisions, even as a fallback. A failed LLM call results in `state = unclassified` and a visible skip in the ledger; the photo is not silently classified by a heuristic.

---

## 8. API Surface (Go core, served by Chi)

All endpoints live under `/v1/photos/...` and are mounted behind the existing bearer-auth middleware, following the `/v1/agent/invoke` pattern in `internal/api/router.go`. JSON responses use the repo-standard API envelope from `internal/api/capture.go`, not RFC 7807. Sensitivity-gated endpoints honour `photos.sensitivity` and `photos.sensitivity_labels` server-side; the UI is not trusted to hide preview bytes.

### 8.0 Common schemas and error model

```json
// Error response: extends the existing ErrorResponse shape with optional metadata.
{
    "error": {
        "code": "PROVIDER_LIMITATION|SENSITIVITY_REQUIRES_REVEAL|ACTION_TOKEN_EXPIRED|INVALID_INPUT|NOT_FOUND|UNAUTHORIZED|ML_UNAVAILABLE|DB_UNAVAILABLE",
        "message": "human-readable message",
        "limitation_code": "gphotos.album.write.unsupported",
        "field": "scope.albums",
        "retryable": true,
        "action_token": "uuid"
    }
}
```

`limitation_code` appears only on `409 PROVIDER_LIMITATION`; `action_token` appears only when a mutation requires confirmation. All handlers MUST call the shared JSON writer style (`writeJSON` / `writeError` equivalent) and MUST avoid `http.Error` for new photo APIs so clients receive one error shape.

```json
// PhotoConnector
{
    "connector_id": "text",
    "provider": "immich|gphotos|amazon|icloud|ente|photoprism",
    "display_name": "Immich (self-hosted)",
    "status": "healthy|syncing|degraded|failing|error|disconnected",
    "capabilities": {
        "read": { "status": "supported" },
        "write_album": { "status": "limited|unsupported", "limitation_code": "gphotos.album.write.create_only" }
    },
    "scope": { "libraries": [], "included_albums": [], "excluded_albums": [], "use_faces": true },
    "progress": { "metadata": { "done": 0, "total": 0, "empty": false }, "classify": {}, "lifecycle": {}, "dedupe": {} },
    "skips": [{ "reason": "too_large|permission_denied|provider_5xx|unsupported_format|private_scope", "count": 0, "last_seen_at": "rfc3339", "retry_token": "opaque" }],
    "last_sync_at": "rfc3339",
    "monitoring_lag_seconds": 0
}

// PhotoSummary in search results
{
    "photo_id": "uuid",
    "artifact_id": "text",
    "provider": "immich",
    "provider_ref": "text",
    "filename": "IMG_4821.HEIC",
    "captured_at": "rfc3339",
    "albums": [],
    "location_label": "Alfama, Lisbon",
    "media_role": "camera_original",
    "lifecycle_state": "processed",
    "sensitivity": "none|sensitive|hidden",
    "sensitivity_labels": [],
    "match_confidence": 0.93,
    "classification": { "caption": "whiteboard diagram", "primary_category": "document/whiteboard", "document_type": "whiteboard", "ocr_snippet": "Q2 OKR" },
    "preview": { "available": true, "requires_reveal": false, "url": "/v1/photos/{id}/preview?size=thumb" }
}

// PhotoDetail extends PhotoSummary
{
    "photo": "PhotoSummary",
    "exif": {},
    "provider_metadata": { "faces": [], "places": [], "events": [], "sharing_state": {} },
    "ocr_text": "string",
    "lifecycle_links": [{ "raw_photo_id": "uuid", "derived_photo_id": "uuid", "editor": "lightroom_classic", "confidence": 0.91, "rationale": "string", "state": "confirmed|pending_confirmation|rejected" }],
    "duplicate_clusters": [{ "cluster_id": "uuid", "kind": "burst|hdr|panorama_member|near_duplicate|exact_hash|cross_provider_hash", "best_photo_id": "uuid", "confidence": 0.88, "rationale": "string" }],
    "routes": [{ "target": "expense|recipe|knowledge|list|annotation", "artifact_id": "text", "confidence": 0.86, "state": "created|needs_review|blocked" }]
}

// ActionPlan
{
    "action_token": "uuid",
    "action": "archive|delete|mark_sensitive|album_remove|favorite|tag|upload",
    "photo_count": 42,
    "bytes_estimate": 1181116006,
    "confidence_range": { "min": 0.71, "max": 0.94 },
    "requires_text": false,
    "expires_at": "rfc3339"
}
```

### 8.1 Connectors

| Method + Path | Body | 200 | Errors | Notes |
|---|---|---|---|---|
| `GET /v1/photos/connectors` | — | `{ connectors: [PhotoConnector] }` | 401 | UI Screen 1 |
| `POST /v1/photos/connectors` | `{ provider, config, scope }` | `{ connector_id }` | 400, 409 (capability incompat) | Screen 2 wizard step 4 |
| `POST /v1/photos/connectors/test` | `{ provider, config }` | `{ ok, capabilities, library_summary }` | 400, 502 (provider error) | Screen 2 step 2 |
| `GET /v1/photos/connectors/{id}` | — | `PhotoConnectorDetail` | 401, 404 | Screen 3 |
| `PATCH /v1/photos/connectors/{id}` | partial | `PhotoConnectorDetail` | 400, 404, 409 | scope/schedule edits |
| `POST /v1/photos/connectors/{id}/sync:pause` | — | 204 | 404 | |
| `POST /v1/photos/connectors/{id}/sync:resume` | — | 204 | 404 | |
| `POST /v1/photos/connectors/{id}/sync:full-rescan` | `{ action_token }` | 202 | 401, 409 | requires confirm token (FR-020) |
| `POST /v1/photos/connectors/{id}/sync:retry-batch` | `{ batch_token }` | 202 | 404 | from skip ledger |
| `DELETE /v1/photos/connectors/{id}` | `{ action_token }` | 204 | 401, 409 | confirmation required |

### 8.2 Photos

| Method + Path | Body | 200 | Errors | Notes |
|---|---|---|---|---|
| `GET /v1/photos/search` | query: `q, filters[]` | `PhotoSearchResult` | 400 | Screen 4 (FR-015) |
| `GET /v1/photos/{id}` | — | `PhotoDetail` | 401, 403 (sensitive), 404 | Screen 5 |
| `GET /v1/photos/{id}/preview` | query: `size, reveal_token` | image bytes | 401, 403 | sensitive requires `reveal_token` |
| `POST /v1/photos/{id}/reclassify` | — | 202 | | re-issues `photos.classify` |
| `POST /v1/photos/{id}/sensitivity` | `{ value, source: "user" }` | `PhotoDetail` | 400 | overrides LLM |
| `POST /v1/photos/{id}/route` | `{ target: "recipe" \| "expense" \| ... }` | `{ artifact_id }` | 400 | cross-feature routing (FR-007) |

### 8.3 Photo health

| Method + Path | Body | 200 | Notes |
|---|---|---|---|
| `GET /v1/photos/health/lifecycle` | — | `LifecycleSummary` | Screen 6 |
| `GET /v1/photos/health/duplicates` | filters | `[ClusterSummary]` | Screen 7 |
| `GET /v1/photos/health/duplicates/{cluster_id}` | — | `Cluster` | Screen 7 detail |
| `POST /v1/photos/health/duplicates/{cluster_id}/best-pick` | `{ photo_id }` | `Cluster` | user override |
| `POST /v1/photos/health/duplicates/{cluster_id}/resolve` | `{ action: "archive_non_best"\|"keep_all"\|"defer", action_token? }` | `Cluster` | destructive actions require token |
| `GET /v1/photos/health/removal` | filters | `RemovalQueue` | Screen 8 |
| `POST /v1/photos/health/removal/decide` | `{ photo_ids[], decision: "keep"\|"archive"\|"delete"\|"exempt", action_token }` | `RemovalQueue` | Screen 8 bulk |
| `GET /v1/photos/health/quality` | — | `QualitySummary` | Screen 9 |

### 8.4 Action tokens (FR-020)

| Method + Path | Body | 200 |
|---|---|---|
| `POST /v1/photos/actions/plan` | `{ action, scope }` | `{ action_token, photo_count, bytes_estimate, confidence_range, requires_text }` |
| `POST /v1/photos/actions/confirm` | `{ action_token, text_confirmation? }` | execution result |

The plan endpoint never mutates state. The confirm endpoint validates the token (matches the user, scope unchanged, not expired, not already consumed) before invoking the underlying provider writer.

### 8.5 Cross-feature

| Path | Used by |
|---|---|
| `GET /v1/photos/by-source/{kind}/{ref}` | Recipe (035), Expense (034), Knowledge (025) |
| `POST /v1/photos/upload` (multipart) | Mobile capture (033), Telegram (008), Browser ext |

---

## 9. UI Component Mapping (from inline wireframes)

| Wireframe (spec.md) | Route | Top-level component | Data hooks |
|---|---|---|---|
| Connectors list | `/connectors/photo-libraries` | `<PhotoConnectorsList>` | `GET /v1/photos/connectors` |
| Add Photo Library wizard | `/connectors/photo-libraries/new` | `<AddPhotoLibraryWizard>` | `POST .../test`, `POST /connectors` |
| Connector Detail | `/connectors/photo-libraries/:id` | `<PhotoConnectorDetail>` | `GET /v1/photos/connectors/:id` (SSE for progress) |
| Photo Search | `/search?q=…` | `<PhotoSearch>` (extends global search) | `GET /v1/photos/search` |
| Photo Detail | `/photos/:id` | `<PhotoDetail>` | `GET /v1/photos/:id` |
| Photo Health Lifecycle | `/photo-health/lifecycle` | `<HealthLifecycle>` | `GET .../health/lifecycle` |
| Photo Health Duplicates | `/photo-health/duplicates` | `<HealthDuplicates>` | `GET .../health/duplicates` |
| Photo Health Removal | `/photo-health/removal` | `<HealthRemoval>` | `GET .../health/removal` |
| Photo Health Quality | `/photo-health/quality` | `<HealthQuality>` | `GET .../health/quality` |
| People (face clusters) | `/people` | `<PeopleClusters>` | provider-backed; read-only by default |
| Save Rules | `/save-rules?domain=photos` | `<SaveRulesPhotos>` (extends 038) | shared rules engine |
| Mobile Doc Scan | PWA route | `<DocumentScanCapture>` | `POST /v1/photos/upload` |
| Telegram patterns | bot handler | `cmd/core/cmd_telegram_*` (extends 008) | shared photo APIs |
| Confirm Destructive Action | modal | `<ConfirmDestructiveAction>` | `POST /actions/plan` then `confirm` |
| Provider Limitation Notice | banner | `<ProviderLimitationBanner>` | `photo_capabilities` |

State management: existing app convention (no new framework). All sensitive previews are fetched only after the user explicitly clicks `Reveal`, which mints a short-lived `reveal_token` server-side.

---

## 10. Cross-Feature Integration Contracts

| Feature | Direction | Contract |
|---|---|---|
| 008 Telegram | photos→telegram | `POST /v1/photos/upload` for inbound; `GET /v1/photos/search` + sensitivity check for outbound; ack message includes classification + target |
| 033 Mobile capture | photos←mobile | `POST /v1/photos/upload` with `mode=document` triggers OCR + provider routing per Save Rule |
| 034 Expenses | photos→expense | When `classification.primary_category in {receipt, invoice}`, expose `POST /v1/photos/{id}/route?target=expense` |
| 035 Recipes | photos→recipe | When `classification.primary_category in {recipe_card, food_dish}`, route to recipe candidate |
| 036 Meal planning | photos→meal | Read-only photo refs from recipe enrichment |
| 021 Intelligence delivery | photos→digest | Photo-of-the-week / RAW backlog summaries |
| 025 Knowledge synthesis | photos→knowledge | Geo + people facts join knowledge graph |
| 026 Domain extraction | photos→knowledge | OCR text feeds the existing extractor |
| 027 Annotations | bidirectional | Annotations can target a `photo` artifact |
| 028 Lists | bidirectional | "Add to list" action stores `photo_id` |
| 037 Agent tools | photos→agent | LLM agent gets `photos.search` + `photos.detail` + `photos.actions.plan` (never `confirm`) |
| 038 Cloud drives | adjacent | 040 owns dedicated photo libraries; 038 owns generic file storage; cross-references documented in spec §Cross-Feature Integration |

---

## 11. Sensitivity Model (BS-016, FR-013)

- Every photo carries `sensitivity ∈ {none, sensitive, hidden}`.
- Every photo also carries zero or more `sensitivity_labels`: `identity_document`, `medical`, `financial`, `children`, `intimate`, `private_location`. Labels are the reason taxonomy required by FR-013; the coarse gate is the enforcement level.
- Sources: `provider` (provider hidden/private flag), `llm` (sensitivity classifier), `user` (manual override), `rule` (Save Rule).
- Resolution rule: `max(provider, llm, user, rule)` where ordering is `none < sensitive < hidden`. User override wins as a hard override regardless of others. Provider hidden/private flags set a minimum of `sensitive`; identity documents and intimate content set `hidden` unless the user explicitly lowers the gate.
- Retrieval guard: `GET /v1/photos/{id}/preview` returns `403 SensitivityRequiresReveal` unless a fresh `reveal_token` is supplied (60s TTL, scoped to user + photo).
- Search results: sensitive thumbnails are rendered as blurred redacted tiles client-side; the API returns metadata only (no preview URL) for sensitive photos until reveal.
- Telegram: outbound retrieval that matches a sensitive photo prompts the user with count + categories; never auto-delivers (UI Screen 13).
- Audit: every reveal, provider mutation, provider-capability block, destructive action plan, and destructive action confirmation writes a row to `photo_audit_events`.

---

## 12. Security & Compliance

- **Secrets:** All provider credentials live as encrypted connection secrets in the connector store. SST: `config/smackerel.yaml.photos.providers.<provider>.{client_id,client_secret,api_key}` are empty values in dev; production runs with environment overrides per repo policy. `config/generated/*.env` is gitignored.
- **Auth:** API uses the runtime bearer token for the Web/PWA + a per-token scope for the agent (`photos:read`, `photos:write`, `photos:destructive`).
- **Authorization matrix:**

| Endpoint family | Owner | Self-Hoster Admin | Family User | Agent (read scope) | Agent (write scope) | Public |
|---|---|---|---|---|---|---|
| `GET /connectors`, `GET /photos`, `GET /search`, `GET /health/*` | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ |
| `POST /connectors`, `PATCH /connectors`, `DELETE /connectors` | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ |
| `POST /photos/{id}/reclassify`, `POST /photos/{id}/sensitivity` | ✓ | ✓ | ✓ (own scope) | ✗ | ✗ | ✗ |
| `POST /photos/{id}/route` | ✓ | ✓ | ✓ | ✗ | ✓ | ✗ |
| `POST /photos/upload` | ✓ | ✓ | ✓ | ✗ | ✓ | ✗ |
| `POST /actions/plan` | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ |
| `POST /actions/confirm` | ✓ | ✓ | ✓ | ✗ | ✗ | ✗ |
| `GET /photos/{id}/preview` (sensitive) | ✓ + reveal | ✓ + reveal | ✓ + reveal | ✗ | ✗ | ✗ |

- **Privacy:** No face model training. No biometric storage. Provider face cluster ids and display names only.
- **Test boundary (FR-018):** automated tests use synthetic photo fixtures. Integration/e2e tests use a local Immich-compatible container/service inside the disposable test Compose project; recorded provider payloads are allowed only in unit/functional tests for parser and capability-probe logic. Tests MUST NOT call live cloud providers or the user's real photo library. Disposable Compose project name comes from generated test config, not a hardcoded Compose invocation.
- **Destructive actions (FR-020):** never one-step. Plan→confirm with token, scope, byte estimate, confidence range; delete additionally requires text confirmation.

---

## 13. Configuration (SST in `config/smackerel.yaml`)

Add a top-level `photos` section, following the sibling `drive` feature's config shape. Human edits happen only in `config/smackerel.yaml`; `scripts/commands/config.sh` must parse these keys with `required_value` / `required_json_value`, emit `PHOTOS_*` variables into `config/generated/dev.env` and `config/generated/test.env`, and the Go/Python runtime must fail startup if an enabled required key is empty or invalid. Generated config is never hand-edited.

```yaml
photos:
    enabled: true
    scan:
        parallelism: 4
        batch_size: 100
        max_file_size_bytes: 209715200
    monitor:
        cursor_invalidation_rescan_max_items: 5000
    policy:
        lifecycle_confirmation_threshold: 0.75
        duplicate_confirmation_threshold: 0.70
        routing_confidence_threshold: 0.70
        sensitivity_reveal_ttl_seconds: 60
        archive_action_token_ttl_seconds: 600
        delete_action_token_ttl_seconds: 120
        telegram_max_inline_size_bytes: 5242880
    intelligence:
        classify_model: ""
        embed_model: ""
        sensitivity_model: ""
        aesthetic_model: ""
        ocr_model: ""
        max_inflight_per_connector: 8
    providers:
        immich:
            enabled: false
            base_url: ""
            api_key: ""
            poll_interval_seconds: 600
            tls_skip_verify: false
            supported_api_versions: []
        gphotos:
            enabled: false
            client_id: ""
            client_secret: ""
            poll_interval_seconds: 1800
        amazon:
            enabled: false
            client_id: ""
            client_secret: ""
            poll_interval_seconds: 1800
        icloud:
            enabled: false
            username: ""
            app_password: ""
            poll_interval_seconds: 3600
        ente:
            enabled: false
            account: ""
            token: ""
            poll_interval_seconds: 900
        photoprism:
            enabled: false
            base_url: ""
            username: ""
            password: ""
            poll_interval_seconds: 600
```

Generated env emits `PHOTOS_*` variables. Required examples include `PHOTOS_ENABLED`, `PHOTOS_SCAN_PARALLELISM`, `PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD`, `PHOTOS_PROVIDER_IMMICH_BASE_URL`, `PHOTOS_PROVIDER_IMMICH_API_KEY`, and `PHOTOS_INTELLIGENCE_CLASSIFY_MODEL`. Provider credentials remain empty in the SST until explicitly configured, but enabling a provider with empty required credentials is a config-generation or startup error. No runtime code may supply fallback URLs, ports, provider names, model IDs, or thresholds.

---

## 14. Observability & Failure Modes

Reuse existing `internal/metrics/`. Add Prometheus metrics:

- `photos_scan_total{connector,provider,phase}` (phase ∈ metadata|thumbnails|classify|lifecycle|dedupe)
- `photos_scan_skipped_total{connector,provider,reason}`
- `photos_llm_calls_total{subject,outcome}` and `photos_llm_latency_seconds{subject}`
- `photos_capabilities_limited_total{connector,provider,capability}`
- `photos_destructive_actions_total{action,outcome}` (outcome ∈ planned|confirmed|cancelled|failed)
- `photos_sensitivity_reveals_total{user_role}`

Logs: structured (`slog`), include `connector_id`, `provider`, `photo_id`, and never the photo bytes/preview URL. Sensitive previews never appear in logs.

Traces: existing OTel instrumentation extended with `photos.classify`, `photos.dedupe`, `photos.lifecycle`, `photos.removal.evaluate` spans. Each span carries `confidence` and `method = stable_signal|llm`.

Failure modes:

| Failure | Behaviour |
|---|---|
| Provider 5xx during scan | retry with existing `connector.backoff`; after threshold, write to skip ledger and surface in UI |
| LLM gateway unavailable | NATS message becomes negatively-acked, retried up to N times, then dead-lettered; photo state remains `unclassified`; UI shows "classification queued" |
| Capability missing for requested action | API returns `409 Provider Limitation` with `limitation_code`; UI banner Screen 15 |
| Destructive action token expired | API returns `410 ActionTokenExpired`; UI re-plans |
| Sensitive reveal without token | `403 SensitivityRequiresReveal` |

---

## 15. Testing Strategy (mapped to `./smackerel.sh test {unit,integration,e2e,stress}`)

Test planning follows `bubbles-test-integrity`:

- `unit` tests execute real Go/Python functions. Provider HTTP payload recordings are allowed only in unit/functional parser tests and cannot satisfy integration/e2e rows.
- `integration`, `e2e-api`, `e2e-ui`, and `stress` tests hit the real disposable stack: Go core, Python ML sidecar, Postgres+pgvector, NATS, Ollama or configured local vision/OCR model, and a local Immich-compatible test provider loaded with synthetic RAW/JPEG/HEIC/video/document fixtures.
- Live-stack tests MUST NOT use request interception or in-process provider response substitution (`route()`, `context.route()`, `cy.intercept`, `msw`, `nock`, `wiremock`, or equivalents). A test using those patterns is not allowed to satisfy a live-stack scenario.
- Tests must assert user-visible behavior and persisted state. Status-only assertions are insufficient unless paired with concrete response fields, DB state, or provider-visible write effects.

### 15.1 Functional Requirement Coverage

| Requirement | Test type | Location | Required assertion |
|---|---|---|---|
| FR-001 PhotoLibrary interface | unit (Go) | `internal/connector/photos/*_test.go` | all adapters expose one provider-neutral contract and identical `PhotoEvent` shape |
| FR-002 Immich connect & scope | integration | `tests/integration/photos_immich_test.go` | local Immich connector authenticates, selected albums are scanned, excluded albums produce no photos |
| FR-003 Bulk + incremental | integration | `tests/integration/photos_sync_test.go` | initial scan persists cursor; new upload, edit, album move, and delete update state |
| FR-004 Multimodal LLM | unit + integration | `ml/tests/test_photos_intelligence.py`, `tests/integration/photos_intelligence_test.go` | structured LLM outputs include category, confidence, OCR decision, sensitivity labels, and rationale where required |
| FR-004A Stable-signal boundary | unit (Go + Python) | `internal/connector/photos/stable_signals_test.go`, `ml/tests/test_photos_boundary.py` | core prepares only allow-listed signals; final classification/lifecycle/dedupe/sensitivity decisions require ML/user output |
| FR-005 Editing lifecycle | integration | `tests/integration/photos_lifecycle_test.go` | RAW + Lightroom/Darktable/GIMP/RawTherapee exports link with editor and rationale; missing RAW yields `processed` with source-not-found state |
| FR-006 Duplicate clusters | integration | `tests/integration/photos_dedupe_test.go` | burst, HDR, panorama, exact, near, and cross-provider duplicate clusters persist with best-pick rationale |
| FR-007 Cross-feature routing | e2e-api | `tests/e2e/photos_routing_test.go` | receipt, recipe, document, product, and place photos create/attach the expected downstream artifacts |
| FR-008 Upload from devices | integration + e2e-api | `tests/integration/photos_upload_test.go`, `tests/e2e/photos_telegram_test.go` | mobile/web/Telegram uploads flow through provider upload, classification, and search |
| FR-009 Document scan | integration | `tests/integration/photos_docscan_test.go` | perspective-corrected multi-page scan stores original + clean artifact and OCR from every page |
| FR-010 Telegram retrieval | e2e-api | `tests/e2e/photos_telegram_test.go` | natural-language query returns photo/link/disambiguation and blocks sensitive auto-delivery |
| FR-011 Albums/tags/favorites | integration | `tests/integration/photos_organization_test.go` | write-capable provider reflects album/tag/favorite changes; limited provider returns `409 PROVIDER_LIMITATION` |
| FR-012 EXIF preservation | unit + integration | `internal/connector/photos/exif_test.go`, `tests/integration/photos_metadata_test.go` | camera/lens/GPS/software/timestamps/provider metadata round-trip into API responses |
| FR-013 Sensitivity | integration + e2e-api | `tests/integration/photos_sensitivity_test.go`, `tests/e2e/photos_sensitivity_retrieval_test.go` | coarse gate + labels enforce preview reveal, Telegram policy, digest exclusion, and audit rows |
| FR-014 Visible skip states | integration | `tests/integration/photos_skip_ledger_test.go` | too-large, unsupported, permission-denied, quota, provider error, and extraction failure skips are visible with retry tokens |
| FR-015 Cross-provider search | e2e-api | `tests/e2e/photos_search_test.go` | Immich + second provider results appear in one ranked response with provider links and no provider tabs |
| FR-016 Removal candidates | integration + e2e-api | `tests/integration/photos_removal_test.go`, `tests/e2e/photos_removal_review_test.go` | every removal candidate has reason/rationale/confidence and cannot mutate provider state before confirmation |
| FR-017 Health & progress | integration + e2e-ui | `tests/integration/photos_health_test.go`, `web/pwa/tests/photos_health.spec.ts` | progress, monitoring lag, confidence histogram, duplicate count, lifecycle distribution, and capability limits render from real API data |
| FR-018 Privacy test boundary | unit + integration guard | `tests/integration/photos_privacy_boundary_test.go` | test config rejects non-disposable provider URLs and never points at user-owned libraries |
| FR-019 Capability visibility | unit + e2e-api + e2e-ui | `internal/connector/photos/capabilities_test.go`, `tests/e2e/photos_capability_test.go`, `web/pwa/tests/photos_capability_banner.spec.ts` | unsupported/limited operations return structured `409` and render Screen 15 provider limitation banners |
| FR-020 Confirmation | e2e-api + e2e-ui | `tests/e2e/photos_destructive_confirm_test.go`, `web/pwa/tests/photos_confirm_action.spec.ts` | action planning is non-mutating; confirm token scope/expiry/text confirmation are enforced |

### 15.2 Business Scenario Coverage

| Scenario | Test type | Location | Required assertion |
|---|---|---|---|
| BS-001 Bulk library ingest | e2e-api + stress | `tests/e2e/photos_bulk_ingest_test.go`, `tests/stress/photos_ingest_stress_test.go` | synthetic 15k-photo library becomes searchable with metadata/classification confidence visible |
| BS-002 OCR document search | integration + e2e-api | `tests/integration/photos_ocr_test.go`, `tests/e2e/photos_search_test.go` | OCR text persists and query returns the expected whiteboard/menu/note photo |
| BS-003 RAW lifecycle | integration | `tests/integration/photos_lifecycle_test.go` | 150 RAW-export links, 350 unprocessed RAWs, editor tag recorded |
| BS-004 Unprocessed RAW candidates | e2e-api | `tests/e2e/photos_removal_review_test.go` | old unprocessed RAWs appear with exact rationale and batch action plan |
| BS-005 Editor signatures | unit + integration | `internal/connector/photos/exif_test.go`, `tests/integration/photos_lifecycle_test.go` | Lightroom, Darktable, GIMP, RawTherapee, DaVinci Resolve signatures map to editor ids |
| BS-006 Burst best-pick | integration + e2e-ui | `tests/integration/photos_dedupe_test.go`, `web/pwa/tests/photos_duplicates.spec.ts` | 10 burst frames form one cluster with one best-pick and nine archive/removal scores |
| BS-007 HDR brackets | integration | `tests/integration/photos_dedupe_test.go` | bracket set groups with HDR output identified when present |
| BS-008 Cross-provider duplicate | e2e-api | `tests/e2e/photos_cross_provider_dedupe_test.go` | identical Immich + second-provider photo returns once with both provider links |
| BS-009 Receipt routing | e2e-api | `tests/e2e/photos_routing_test.go` | expense artifact contains vendor/date/total/currency and original photo evidence |
| BS-010 Recipe routing | e2e-api | `tests/e2e/photos_routing_test.go` | recipe artifact contains name/ingredients/steps and original photo link |
| BS-011 Food/place context | integration + e2e-api | `tests/integration/photos_metadata_test.go`, `tests/e2e/photos_routing_test.go` | GPS/place context and cuisine tag join the knowledge/digest surfaces |
| BS-012 Telegram upload | e2e-api | `tests/e2e/photos_telegram_test.go` | Telegram photo reaches target Immich album, classification, and search within configured window |
| BS-013 Telegram retrieval | e2e-api | `tests/e2e/photos_telegram_test.go` | query sends safe match with extracted serial number caption |
| BS-014 Panorama components | integration | `tests/integration/photos_dedupe_test.go` | components group with stitched result and component removal candidates |
| BS-015 Transient screenshot | integration | `tests/integration/photos_removal_test.go` | expired booking screenshot is flagged `transient_expired` with rationale |
| BS-016 Sensitive retrieval block | e2e-api | `tests/e2e/photos_sensitivity_retrieval_test.go` | passport match produces secure-link/refusal flow and no chat photo bytes |
| BS-017 Multi-page scan | integration | `tests/integration/photos_docscan_test.go` | four pages create one legal document artifact with OCR from all pages |
| BS-018 Provider-agnostic classification | integration | `tests/integration/photos_provider_neutrality_test.go` | Immich and second adapter publish identical artifact/classification/routing shapes |
| BS-019 Blurry detection | integration | `tests/integration/photos_quality_test.go` | blurry burst frames have lower scores and removal candidates with rationale |
| BS-020 Metadata graph | integration + e2e-api | `tests/integration/photos_metadata_graph_test.go`, `tests/e2e/photos_search_test.go` | EXIF camera/lens/GPS facts are queryable through knowledge/search |
| BS-021 Album changes | integration | `tests/integration/photos_sync_test.go` | album move updates metadata/context without re-running image classification |
| BS-022 Face clusters | integration + e2e-api | `tests/integration/photos_faces_test.go`, `tests/e2e/photos_people_search_test.go` | provider face cluster refs return person-filtered results |
| BS-023 Video classification | integration | `tests/integration/photos_video_test.go` | video duration/resolution/codec/thumbnail classification persist; no frame timeline rows are created |
| BS-024 Empty library | integration + e2e-ui | `tests/integration/photos_empty_library_test.go`, `web/pwa/tests/photos_connector_empty.spec.ts` | connector healthy, zero artifacts, subsequent upload flows |
| BS-025 Progress visibility | integration + e2e-ui | `tests/integration/photos_health_test.go`, `web/pwa/tests/photos_connector_progress.spec.ts` | N/total per phase, ETA, skipped reasons, and capability status render from API |
| BS-026 Meal-plan photo reference | e2e-api | `tests/e2e/photos_mealplan_test.go` | meal plan references extracted recipe text and original photo |
| BS-027 Export without RAW | integration | `tests/integration/photos_lifecycle_test.go` | edited export has editor detected and source-not-found lifecycle link state |
| BS-028 Health dashboard | e2e-ui | `web/pwa/tests/photos_health.spec.ts` | lifecycle, duplicate, sensitive, document, receipt, recipe, and confidence metrics render |
| BS-029 Low-confidence RAW match | integration + e2e-ui | `tests/integration/photos_low_confidence_test.go`, `web/pwa/tests/photos_lifecycle_review.spec.ts` | pending confirmation blocks processed state until approve/reject |
| BS-030 Capability gap | e2e-api + e2e-ui | `tests/e2e/photos_capability_test.go`, `web/pwa/tests/photos_capability_banner.spec.ts` | album write is blocked with visible provider-capability reason while search still works |
| BS-031 Destructive approval | e2e-api + e2e-ui | `tests/e2e/photos_destructive_confirm_test.go`, `web/pwa/tests/photos_confirm_action.spec.ts` | no archive/delete/album removal occurs before exact batch confirmation |
| BS-032 Stable signals boundary | unit + integration | `internal/connector/photos/stable_signals_test.go`, `tests/integration/photos_boundary_test.go` | similar names/timestamps seed review only; LLM/user judgment rejects different subjects |

Bug-fix regression scopes for this feature MUST include adversarial cases, such as a stale action token, a sensitive match that attempts auto-delivery, a provider limitation that would otherwise no-op, or an LLM result missing rationale. These cases must fail if the bug is reintroduced.

---

## 16. Open Questions and Resolved Analyst Questions

### 16.1 Open Questions

1. **O-040-D-01 Immich API version range** — Which Immich server/API version range is the first adapter required to support? Current design records provider version during capability probe and returns a visible incompatibility error outside the supported range.
2. **O-040-D-02 Apple/iCloud provider path** — Which path is viable enough for Apple/iCloud: CloudKit web, export/import, or local Photos library bridge? Current design treats Apple as read-limited until a capability probe proves otherwise.
3. **O-040-D-03 Vector scale threshold** — At what photo volume should `photo_embeddings` be partitioned by connector/provider? Current design keeps one table in Postgres+pgvector for normal self-hosted scale and requires an explicit migration before tens-of-millions scale.

### 16.2 Resolved During Design Reconciliation

| Analyst question | Design decision |
|---|---|
| Google Photos API restrictions | Minimum adapter scope is read + poll monitor + upload into app-owned albums where allowed; unsupported writes return `409 PROVIDER_LIMITATION`. |
| Amazon Photos API limitations | Treat Amazon as capability-probed read/import where available; no provider mutation is attempted unless a supported API is proven by the adapter. |
| LLM classification cost | Use staged processing: metadata/thumbnail first, full-resolution only when needed for OCR, lifecycle, duplicate, sensitivity, or low-confidence review; concurrency comes from `photos.intelligence.max_inflight_per_connector`. |
| Perceptual hash algorithm | Use 64-bit pHash as the near-duplicate seed, exact hash for byte-identical matches, and LLM confirmation for final cluster membership and best-pick. |
| RAW-export matching confidence | Stable signals seed candidates; configured thresholds (`photos.policy.lifecycle_confirmation_threshold`) decide whether LLM-confirmed links apply directly or enter pending confirmation. |
| Cross-provider face mapping | Consume provider face clusters only; cross-provider person grouping is label/cluster-reference based unless the user explicitly merges clusters. No local face model training. |
| Deletion propagation policy | Prefer provider archive/trash where supported; delete requires action token plus text confirmation. Unsupported delete/archive is blocked with capability reason. |
| Video lifecycle tracking | Videos use `media_role = video`, thumbnail classification, and metadata search. Frame-level editing timeline analysis is not part of this photo lifecycle model. |
| Multi-routing policy | A photo may create multiple route candidates. High-confidence receipt/recipe/document routes can create target artifacts; ambiguous or sensitive multi-routes enter annotation/review instead of picking a single route. |
| Local directory scanning | A local directory provider can implement the same `PhotoLibrary` interface later; the core model and tests remain provider-neutral. |
| HEIF/HEIC and ProRAW | HEIF/HEIC and DNG/ProRAW are first-class media roles through MIME/EXIF parsing; unsupported decoder paths write visible skip reasons, not silent drops. |

---

## 17. Alternatives & Tradeoffs

| Alternative | Why rejected |
|---|---|
| Train local face/NSFW models | Conflicts with non-goals and privacy posture; provider face clusters + LLM sensitivity classifier are sufficient |
| Heuristic-first lifecycle/dedupe | Fails the LLM mandate; produces brittle results across editor versions and cameras; rationale-less decisions can't satisfy FR-016 |
| Per-provider Postgres schemas | Breaks cross-provider unified search and Photo Health rollups |
| Bypass NATS and call ML sidecar via HTTP | Loses retry/dead-letter semantics that other pipelines rely on; breaks parity with existing pipeline design |
| One global "delete" toggle without action tokens | Cannot satisfy FR-020 confirmation contract |

---

## 18. FR / BS / UC Coverage

Every functional requirement in `spec.md` is covered by an architecture element and a row in §15.1. Every business scenario BS-001 through BS-032 is covered by a concrete test row in §15.2 with an assertion that checks real behavior, not only status codes. Every use case is covered by a wireframe + API pair (UC-001..UC-005, UC-010..UC-013) or a cross-feature contract in §10 (UC-006..UC-009). The planning artifact authored later by `bubbles.plan` will convert these rows into scope-level scenario IDs, Gherkin, Test Plan rows, and DoD items.

---

## 19. Rollout Plan (high level — `bubbles.plan` will turn this into scopes)

1. Foundations: data model, capability matrix, provider interface, NATS subjects, ML sidecar handlers, and provider-neutral contracts.
2. Immich reference adapter: connect, scope, scan, monitor, write-back, faces-read.
3. Intelligence pipeline: classify → embed → ocr → aesthetic → sensitivity, with skip ledger.
4. Lifecycle pairing + duplicate clusters + removal candidates with rationale.
5. UI: Connectors list, Add wizard (Immich), Connector detail, Photo Search, Photo Detail.
6. Photo Health surfaces (Lifecycle / Duplicates / Removal / Quality).
7. Confirm Destructive Action contract end-to-end + Provider Limitation Notice.
8. Cross-feature wiring (Telegram, Mobile capture, Recipe, Expense, Knowledge, Lists, Annotations, Agent tools, Intelligence delivery).
9. Additional providers (Google Photos, Amazon Photos, Apple iCloud, Ente, PhotoPrism) one at a time, each gated by capability matrix.
10. Stress + observability hardening.
