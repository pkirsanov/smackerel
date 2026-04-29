# Design: 038 Cloud Drives Integration

> **Status:** Foundational design for the Cloud Drives capability committed
> to in [spec.md](./spec.md). Builds on the connector contract committed in
> [internal/connector/connector.go](../../internal/connector/connector.go),
> the ingestion pipeline in
> [internal/pipeline/ingest.go](../../internal/pipeline/ingest.go), the
> NATS contract in [config/nats_contract.json](../../config/nats_contract.json),
> the prompt-contract pattern in [config/prompt_contracts/](../../config/prompt_contracts/),
> and the agent + tool surface defined by
> [specs/037-llm-agent-tools/design.md](../037-llm-agent-tools/design.md).
> Downstream features 008 (Telegram), 021 (intelligence delivery), 026
> (domain extraction), 027 (annotations), 028 (lists), 033 (mobile capture),
> 034 (expenses), 035 (recipes), 036 (meal plan), and 037 (agent + tools)
> consume this surface; none of them re-implements drive provider logic,
> save-back routing, or rule evaluation.

---

## Design Brief

**Current State.** Smackerel's connectors today are read-only ingestion
sources that emit `RawArtifact` instances into the pipeline (see
`internal/connector/connector.go` `Connector` interface, `RawArtifact`
struct, and `Health` enum, plus `internal/pipeline/ingest.go` storage path).
Google Keep is the only Google-source precedent and runs as a pull-only
connector via Takeout/`gkeepapi`-style normalization. Telegram is a capture
source (feature 008) but cannot send files back to a user. Multi-format
extraction exists in `internal/extract/` for the formats Smackerel already
ingests (text, HTML, basic images via the ML sidecar). There is no
provider-neutral cloud-drive abstraction, no save-back service, no
folder-aware classification context, no rule engine that decides where an
artifact should be filed, and no concept of sensitivity tiers attached to
artifacts.

**Target State.** A provider-neutral cloud-drive surface (`internal/drive/`)
that exposes a `DriveProvider` Go interface, a first concrete
`GoogleDriveProvider`, a bulk + incremental scan loop, a bidirectional
**Save Service** with idempotent target-path resolution and
version/replace/skip semantics, a **Save Rules engine** that decides which
incoming artifacts are filed where, a **Retrieval Service** that lets
Telegram and other channels return drive files to a user under sensitivity
policy, and a **classification context** that enriches every drive artifact
with folder semantics, sharing, version chain, and sensitivity tier. A new
NATS stream `DRIVE` carries scan, change, save-request, save-result, and
health subjects between Go and the Python ML sidecar. All configuration
lives in [config/smackerel.yaml](../../config/smackerel.yaml) under a new
`drive:` block; secrets are sourced from approved secret storage; nothing
falls back to defaults.

**Patterns to Follow.**
- Connector contract from
  [internal/connector/connector.go](../../internal/connector/connector.go)
  (`Connector`, `RawArtifact`, `Health`, lifecycle hooks). The
  `DriveProvider` is an extension of this surface, not a replacement.
- Generated config + zero-defaults from
  [config/smackerel.yaml](../../config/smackerel.yaml) →
  `config/generated/*.env` pipeline. New keys MUST be declared in
  `smackerel.yaml`; no in-code fallbacks.
- NATS-mediated Go ↔ Python boundary from
  [config/nats_contract.json](../../config/nats_contract.json). One new
  stream `DRIVE`; both sides verify constants from the generated contract.
- Prompt contract YAML shape from
  [config/prompt_contracts/cross-source-connection-v1.yaml](../../config/prompt_contracts/cross-source-connection-v1.yaml)
  for the new classification + folder-context contracts.
- Per-package data ownership: `internal/drive/` owns provider, save service,
  retrieval service, rule engine, and drive-side schema. Cross-feature
  consumers (`internal/recipe/`, `internal/intelligence/`,
  `internal/mealplan/`, `internal/list/`, `internal/annotation/`,
  `internal/digest/`) consume drive artifacts via the existing artifact
  store; they never reach into provider APIs.
- Agent tool registration from
  [specs/037-llm-agent-tools/design.md](../037-llm-agent-tools/design.md).
  Drive read/save/retrieve operations are exposed as registered tools so the
  scenario agent (037) can drive flows like "save this receipt to Drive" or
  "send me the boarding pass" without bespoke regex routing in
  `internal/telegram/`.

**Patterns to Avoid.**
- Putting provider-specific HTTP calls into `internal/pipeline/` or
  `internal/telegram/`. All provider traffic goes through `DriveProvider`.
- Treating "save to Drive" as a one-off Telegram handler. Save requests are
  produced by any connector and routed through the Save Rules engine.
- Hardcoded folder names, MIME allow-lists, size caps, or sync intervals in
  Go source. Every value lives in `config/smackerel.yaml`.
- Storing provider OAuth refresh tokens anywhere outside approved secret
  storage; never in `config/smackerel.yaml`, never in PostgreSQL plaintext.
- Silent classification overrides. Every low-confidence routing decision
  goes through Screen 11 confirmation (FR-016).

**Resolved Decisions.**
- Go owns: provider interface, scan/change loop, save service, retrieval
  service, rule engine, sensitivity policy enforcement, drive schema, and
  all NATS publish/subscribe.
- Python ML sidecar owns: extraction for formats not handled in Go (PDF
  text-layer + OCR fallback, Office formats, audio transcription, image
  captioning) and LLM-driven classification + folder-context summarization.
  Python is stateless and never decides where a file is saved.
- New NATS stream `DRIVE` with subjects: `drive.scan.request.<provider>`,
  `drive.scan.progress.<provider>`, `drive.change.<provider>`,
  `drive.extract.request`, `drive.extract.result`,
  `drive.classify.request`, `drive.classify.result`,
  `drive.save.request.<provider>`, `drive.save.result.<provider>`,
  `drive.health.<provider>`. JetStream durable consumers per worker.
- Storage: existing `artifacts` table is extended via a sibling
  `drive_files` table keyed by `(provider_id, provider_file_id)` with a
  `version_chain` column (jsonb array of provider revision IDs) and a FK
  back to `artifacts.id` for the head version. Save requests live in
  `drive_save_requests` with an `idempotency_key` unique index built from
  `(rule_id, source_artifact_id, target_path)`.
- Sensitivity tiers: `none | financial | medical | identity`. Tier is
  attached to the artifact, not the file path; rules read it; retrieval and
  share enforcement reads it.
- Folder include/exclude rules use combined include and exclude paths with
  an integer `max_depth`. Evaluated server-side at scan time and at
  monitor time. No DSL.
- Change-history continuity: per-provider cursor stored in `drive_cursors`.
  When the provider invalidates its change marker, the connector falls back
  to a bounded rescan of the configured scope and re-emits only deltas
  computed against the current `drive_files` snapshot. (Resolves Open
  Question 8.)
- Sensitive files are never returned over Telegram as bytes. The retrieval
  service returns either a provider deep link (with a one-line policy note)
  or refuses, depending on the configured policy. (FR-014, BS-025.)
- Save-back idempotency: the rule engine computes a deterministic target
  path; the save service uses provider-supported conditional create
  semantics where available, otherwise it consults `drive_save_requests`
  before writing.
- Scenario agent integration: the drive package registers four tools with
  the registry from spec 037 — `drive_search`, `drive_get_file`,
  `drive_save_file`, `drive_list_rules` — each declared `read` /
  `external` / `external` / `read` respectively.

**Open Questions.**
- None blocking for design. The analyst-flagged questions are resolved in
  the active design sections and summarized in the final Open Questions
  resolution table; if provider implementation reveals a contradiction,
  route back to `bubbles.design` before planning changes.

---

## 1. Architecture Placement

```
┌──────────────────────────── Web (PWA) / Telegram ──────────────────────────┐
│ Screens 1–11   │   Telegram bot save/retrieve replies   │   Daily digest    │
└───────────┬───────────────────────┬───────────────────────────┬─────────────┘
            │ HTTP                  │ Telegram Bot API           │ delivery
            ▼                       ▼                            ▼
┌────────────────────── Go core (cmd/core) ─────────────────────────────────┐
│ internal/api/   internal/telegram/   internal/digest/                      │
│        │                │                 │                                │
│        ▼                ▼                 ▼                                │
│ ┌─────────────────────────── internal/drive/ ──────────────────────────┐  │
│ │ DriveProvider iface · GoogleDriveProvider · ScanLoop · ChangeMonitor │  │
│ │ SaveService · RetrievalService · RuleEngine · SensitivityPolicy      │  │
│ │ Schema: drive_files · drive_cursors · drive_save_requests · rules    │  │
│ │ Tool registrations (drive_search/get/save/list_rules) → spec 037     │  │
│ └────────────────────────────────┬─────────────────────────────────────┘  │
│        │                         │                                         │
│        ▼                         ▼                                         │
│ internal/connector/  internal/pipeline/  internal/extract/  internal/db/   │
│ (connector contract) (artifact storage)  (Go-side text)     (pgvector)     │
└────────────────────────────────┬─────────────────────────────────────────┘
                                 │ NATS (stream: DRIVE)
                                 ▼
┌─────────────────────── Python ML sidecar (ml/app) ───────────────────────┐
│ extract.py (PDF/Office/audio/image) · classify.py (LLM) · folder_ctx.py  │
│ Stateless workers; consume drive.extract.request, drive.classify.request │
└──────────────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
                       PostgreSQL + pgvector
                       (artifacts, drive_files, drive_cursors,
                        drive_save_requests, drive_rules, rule_audit)
```

**FR coverage in this layer:** FR-001 (provider model), FR-002 (Google
Drive concrete), FR-003 (scan/monitor), FR-004 (read/save), FR-007
(metadata), FR-017 (health surface). All other FRs are realized in the
sub-sections below.

---

## 2. Provider Model (FR-001, FR-002, IP-001)

### 2.1 Go interface

`internal/drive/provider.go` defines:

```go
type Provider interface {
    ID() string                          // "google", "dropbox", ...
    DisplayName() string                 // "Google Drive"
    Capabilities() Capabilities          // versions, sharing, change_history, ...

    // BeginConnect starts a provider-specific OAuth web flow. The caller is
    // expected to redirect the end user's browser to authURL. state is an
    // opaque server-issued nonce that MUST be persisted server-side
    // (drive_oauth_states, §3.4) bound to the user, accessMode, and scope
    // so the eventual provider callback can be authenticated and the
    // pending request reconstructed without trusting query parameters.
    BeginConnect(ctx context.Context, accessMode AccessMode, scope Scope) (authURL string, state string, err error)

    // FinalizeConnect exchanges the provider-issued authorization code for
    // access + refresh tokens, persists the connection (including
    // expires_at) into drive_connections, deletes the matching
    // drive_oauth_states row, and returns the new connection identifier.
    // Implementations MUST verify state against the persisted nonce and
    // MUST refuse codes whose state has expired or already been consumed.
    FinalizeConnect(ctx context.Context, state string, code string) (connectionID string, err error)

    Disconnect(ctx context.Context, connectionID string) error
    Scope(ctx context.Context, connectionID string) (Scope, error)
    SetScope(ctx context.Context, connectionID string, scope Scope) error
    ListFolder(ctx context.Context, connectionID string, folderID string, pageToken string) (items []FolderItem, nextPageToken string, err error)
    GetFile(ctx context.Context, connectionID string, providerFileID string) (FileBytes, error)
    PutFile(ctx context.Context, connectionID string, folderID string, title string, body FileBytes) (providerFileID string, err error)
    Changes(ctx context.Context, connectionID string, cursor string) (changes []Change, nextCursor string, err error)
    Health(ctx context.Context, connectionID string) (Health, error)
}

type Capabilities struct {
    SupportsVersions      bool
    SupportsSharing       bool
    SupportsChangeHistory bool
    MaxFileSizeBytes      int64
    SupportedMimeFilter   []string
}
```

The interface intentionally mirrors the existing
`internal/connector/connector.go` lifecycle (Connect, Health) so that drives
appear in the existing connector registry alongside Telegram, Keep, etc.
The `Connect` step is split into `BeginConnect` + `FinalizeConnect` because
an OAuth web flow cannot complete inside a single synchronous call: the
user must visit the provider's consent screen and the provider then
redirects to our callback with an authorization code. This split is the
standard OAuth web-flow shape and avoids the test-only programmatic
shortcut anti-pattern (see decision log under Open Questions).

### 2.2 Adding providers (IP-001, UC-008, BS-008)

A new provider is added by:
1. Implementing `DriveProvider` in `internal/drive/<provider>/`.
2. Registering it via `init()` in the package, which calls
   `drive.Register(driver)` — analogous to the agent tool registry pattern
   in spec 037 (decentralized registration, no central enum).
3. Declaring its capabilities and the smallest scope set in
   `config/smackerel.yaml` under `drive.providers.<id>`.
4. Adding the provider tile to Screen 2's selector via the existing PWA
   connector registry (no UI hard-coding).

The Save Rules engine, retrieval service, scan loop, and tool registrations
work unchanged when a new provider is added. (BS-008 acceptance criterion.)

### 2.3 Google Drive concrete provider (FR-002, BS-018)

Lives in `internal/drive/google/`. Uses the official Google Drive REST API
via the project's OAuth client. Refresh tokens are written into approved
secret storage at `Connect()` time and read by the provider on each call;
they never enter `config/smackerel.yaml`. Scope set is the smallest needed
for the user's chosen access mode (Screen 2 step 2).

---

## 3. Scan and Monitor Loop (FR-003, UC-001, UC-002, BS-001, BS-002)

### 3.1 Bulk scan

On `Connect()` the connector emits `drive.scan.request.<provider>` with the
chosen folder scope. The scan worker (Go, `internal/drive/scan/`) walks
`ListFolder` with provider-side paging, persists each file to `drive_files`
(keyed by provider+file_id), and enqueues `drive.extract.request` for
content beyond plain metadata. Progress is emitted on
`drive.scan.progress.<provider>` (one event per N files or per second,
whichever is first) and surfaced by Screen 3's progress bar.

### 3.2 Incremental monitor

After bulk scan completes, the change monitor polls `Changes(cursor)` on
the provider's published interval (configurable per provider). Each delta:
- New file → enqueue `drive.extract.request` and `drive.classify.request`.
- Modified file → bump `drive_files.version_chain`, enqueue extract+classify.
- Moved file → recompute folder context, enqueue
  `drive.classify.request` (Flow D, BS-006).
- Deleted/trashed → mark `drive_files.tombstoned_at`, keep searchable for
  the configured TTL, then prune (Screen 5 tombstone state).

### 3.3 Cursor durability (Open Question 8 → resolved)

`drive_cursors(provider_id, connection_id, cursor TEXT, valid_until TIMESTAMPTZ)`.
When the provider returns "cursor invalid", the monitor falls back to a
bounded rescan of in-scope folders, computes a delta against
`drive_files`, and re-issues only those deltas as change events. This makes
change-history continuity provider-independent.

### 3.4 Drive storage model (FR-007, FR-015, BS-007, BS-013, BS-017)

Drive-specific state is stored beside, not inside, the canonical artifact
record. `artifacts.id` remains the cross-feature identity; `drive_files`
preserves provider identity, folder context, and version metadata.

```sql
CREATE TABLE drive_connections (
  id UUID PRIMARY KEY,
  provider_id TEXT NOT NULL,
  owner_user_id UUID NOT NULL,
  account_label TEXT NOT NULL,
  access_mode TEXT NOT NULL CHECK (access_mode IN ('read_only', 'read_save')),
  status TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'failing', 'disconnected')),
  last_health_reason TEXT,
  scope JSONB NOT NULL DEFAULT '{}'::jsonb,
  credentials_ref TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NULL,        -- access-token expiry written by FinalizeConnect; NULL means "not yet known"
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (provider_id, owner_user_id, account_label)
);

-- Transient nonce store binding an in-flight OAuth web flow to the user,
-- access mode, and scope. A row is INSERTed by BeginConnect and DELETEd
-- by FinalizeConnect (or by the bounded sweeper after expires_at). The
-- table is intentionally a real Postgres table (not in-memory) so the
-- callback works across process restarts and across replicas, and so
-- the SCN-038-002 integration test can assert state was issued and
-- consumed atomically.
CREATE TABLE drive_oauth_states (
  state         TEXT PRIMARY KEY,             -- opaque server-issued nonce (>=128 bits, base64url)
  provider_id   TEXT NOT NULL,
  owner_user_id UUID NOT NULL,
  access_mode   TEXT NOT NULL CHECK (access_mode IN ('read_only', 'read_save')),
  scope         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ NOT NULL          -- short-lived (config-driven; default 10 minutes)
);
CREATE INDEX drive_oauth_states_expires_idx ON drive_oauth_states (expires_at);

CREATE TABLE drive_files (
  id UUID PRIMARY KEY,
  artifact_id UUID NOT NULL REFERENCES artifacts(id),
  connection_id UUID NOT NULL REFERENCES drive_connections(id),
  provider_file_id TEXT NOT NULL,
  provider_revision_id TEXT,
  provider_url TEXT NOT NULL,
  title TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  folder_path TEXT[] NOT NULL,
  provider_labels JSONB NOT NULL,
  owner_label TEXT NOT NULL,
  last_modified_by TEXT,
  sharing_state JSONB NOT NULL,
  sensitivity TEXT NOT NULL CHECK (sensitivity IN ('none', 'financial', 'medical', 'identity')),
  extraction_state TEXT NOT NULL CHECK (extraction_state IN ('pending', 'complete', 'partial', 'skipped', 'blocked')),
  skip_reason TEXT,
  tombstoned_at TIMESTAMPTZ,
  permission_lost_at TIMESTAMPTZ,
  version_chain JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (connection_id, provider_file_id)
);

CREATE INDEX drive_files_artifact_idx ON drive_files(artifact_id);
CREATE INDEX drive_files_folder_path_idx ON drive_files USING GIN(folder_path);
CREATE INDEX drive_files_sensitivity_idx ON drive_files(sensitivity);

CREATE TABLE drive_folders (
  id UUID PRIMARY KEY,
  connection_id UUID NOT NULL REFERENCES drive_connections(id),
  provider_folder_id TEXT NOT NULL,
  folder_path TEXT[] NOT NULL,
  folder_summary JSONB NOT NULL,
  summarized_at TIMESTAMPTZ NOT NULL,
  UNIQUE (connection_id, provider_folder_id)
);

CREATE TABLE drive_cursors (
  connection_id UUID PRIMARY KEY REFERENCES drive_connections(id),
  cursor TEXT NOT NULL,
  valid_until TIMESTAMPTZ,
  last_rescan_started_at TIMESTAMPTZ,
  last_rescan_completed_at TIMESTAMPTZ
);
```

Native Google Docs are represented as canonical `(connection_id,
provider_file_id)` rows. Each provider revision refreshes the extracted
snapshot and appends the provider revision ID to `version_chain`; the
provider URL stays stable for the current head while prior revisions remain
reachable through Screen 6's Versions tab when the provider exposes them.

---

## 4. Extraction and Classification (FR-005, FR-006, BS-007, BS-013, IP-005, IP-006)

### 4.1 Extraction request shape

Go publishes `drive.extract.request` with `{provider, file_id, mime,
size, sensitivity_hint}`. Python's `ml/app/extract.py` worker:
- Routes to the right extractor (PDF text → fast path; PDF image-only →
  OCR; Office → existing handler; audio → Whisper; image → caption + OCR).
- Honors `runtime.ml.processing_tier` (config), so large files can be
  sent to a low-priority subject.
- Emits `drive.extract.result` with extracted text, page map, and an
  enumerated `skip_reason` when a file cannot be processed (size,
  encryption, unsupported format).

Skip reasons feed Screen 4's grouping and FR-013.

### 4.2 Classification + folder context

Go publishes `drive.classify.request` with extracted text plus
**folder context**: ancestor folder names, sibling MIME mix, sharing
audience, and previously-assigned tags. Python's `classify.py` runs an LLM
call against a new prompt contract
`config/prompt_contracts/drive-classification-v1.yaml` (mirrors the recipe
contract shape) that returns `{topic, sensitivity, audience,
classification, confidence, evidence}`.

When confidence < `drive.classification.confidence_threshold` (config), Go
applies `drive.classification.low_confidence_action` — when set to `pause`,
it opens a confirmation interaction (Screen 11 web modal or Telegram
numbered reply) and only commits the classification once the user picks.
(FR-016, BS-015.)

### 4.3 Folder-as-taxonomy (FR-008, UC-007, Flow D)

A second prompt contract,
`config/prompt_contracts/drive-folder-context-v1.yaml`, summarizes a folder
once per change cycle into a stable JSON structure
`{topic, audience, sensitivity_prior, expected_classification}` stored on
`drive_folders`. `classify.py` reads the latest summary as part of its
prompt, so a folder rename or move triggers re-summarization which can in
turn trigger reclassification of contained files (Flow D).

---

## 5. Save Rules Engine (FR-009, FR-010, IP-002, IP-004, Screens 7+8)

### 5.1 Schema

```
drive_rules(
  id UUID PK,
  name TEXT,
  enabled BOOL,
  source_kinds TEXT[],            -- 'telegram','mobile','meal_plan',...
  classification TEXT,            -- 'receipt','recipe','manual',...
  sensitivity_in TEXT[],          -- ['none','financial']
  confidence_min NUMERIC,
  provider_id TEXT,
  target_folder_template TEXT,    -- 'Receipts/{year}/'
  on_missing_folder TEXT,         -- 'create' | 'fail'
  on_existing_file TEXT,          -- 'replace' | 'version' | 'skip'
  guardrails JSONB,               -- {never_link_share: true, require_confirm_below: 0.75}
  created_at, updated_at
);

drive_save_requests(
  id UUID PK,
  rule_id UUID FK,
  source_artifact_id UUID FK,
  target_path TEXT,
  idempotency_key TEXT UNIQUE,    -- hash(rule_id, source_artifact_id, target_path)
  status TEXT,                    -- pending|written|skipped|failed
  attempts INT,
  last_error TEXT,
  created_at, completed_at
);

drive_folder_resolutions(
  id UUID PK,
  connection_id UUID FK,
  provider_id TEXT,
  folder_path TEXT UNIQUE,
  provider_folder_id TEXT,
  created_by_request_id UUID FK,
  created_at, updated_at
);

drive_rule_audit(
  id BIGSERIAL PK,
  rule_id UUID,
  source_artifact_id UUID,
  outcome TEXT,                    -- 'matched','skipped','conflict','failed'
  reason TEXT,
  created_at
);
```

### 5.2 Evaluation

When any connector commits an artifact, the pipeline calls
`drive.RuleEngine.Evaluate(artifact)`. The engine:
1. Filters rules by `source_kinds` and `classification`.
2. Filters by `sensitivity_in`.
3. Filters by `confidence_min` (against the artifact's classification
   confidence).
4. If multiple rules match, all matching rules are recorded in
   `drive_rule_audit` with `outcome='conflict'` and the **first stable**
   match is executed. Conflicts are surfaced on Screen 7 (amber chip).
5. The matched rule's `target_folder_template` is rendered with the
   artifact's metadata (`{year}`, `{isoweek}`, `{topic}`); invalid tokens
   fail the save with a clear `last_error` and surface in Screen 7's row
   error counter.
6. Sensitivity guardrails (`never_link_share`, `require_confirm_below`)
   are applied before issuing `drive.save.request`.

### 5.3 Save service (FR-004, FR-015, BS-014)

`internal/drive/save/` consumes `drive.save.request.<provider>`:
- Computes deterministic `target_path`.
- Looks up `drive_save_requests` by `idempotency_key`. If `written`,
  no-op.
- Resolves the target folder through `drive_folder_resolutions` inside the
  same database transaction. A unique `(connection_id, folder_path)` insert
  plus provider-side conditional create ensures exactly one folder is
  created when concurrent saves target a missing folder (BS-016).
- Otherwise calls `provider.PutFile()` with the `on_existing_file` policy.
- On success, writes `drive_files` entry, links it to the source artifact
  in the artifact graph, and emits a delivery event to the originating
  connector (Telegram → Screen 9 reply).
- On failure, increments `attempts`, records `last_error`, and re-enqueues
  with exponential backoff bounded by config.

### 5.4 Test endpoint (Screen 8 "Test against")

`POST /api/v1/drive/rules/{id}/test` runs evaluation against a
caller-chosen artifact in dry-run mode and returns the would-be target
path, would-be outcome, and the resolved guardrail decisions. Never
writes to the provider.

---

## 6. Retrieval Service (FR-010, UC-005, BS-025, Screen 10)

`internal/drive/retrieve/` exposes a Go API:

```go
type RetrieveRequest struct {
    Channel        string   // "telegram"
    UserID         string
    Query          string
    AllowedClassif []string // optional narrow
}

type RetrieveCandidate struct {
    ArtifactID  string
    Title       string
    Folder      string
    Sensitivity string
    SizeBytes   int64
    Provider    string
    ProviderURL string
}

type RetrieveDelivery struct {
    Mode         string // "bytes" | "secure_link" | "provider_link" | "refused"
    URL          string // for link modes
    PolicyReason string // when refused or downgraded
}
```

Flow C in spec.md is implemented as:
1. `Search()` queries the artifact store + drive_files (pgvector + filter on
   `audience`/`sensitivity`/`folder`).
2. If 0 results → return refusal with a refine hint (Screen 10 no-match).
3. If 1 result → policy check: sensitivity restricts mode to
   `secure_link` or `refused`; size > `drive.telegram.max_inline_size_bytes`
   downgrades to `provider_link`; otherwise `bytes`.
4. If >1 results → return candidates; the channel adapter (Telegram bot)
   shows the disambiguation reply; user pick funnels back through step 3.

Refusal text is sourced from a single localized table; the bot does not
invent prose.

---

## 7. Cross-Feature Integration (FR-011, FR-012, IP-003, IP-004)

| Feature | Direction | Surface |
|---------|-----------|---------|
| 008 Telegram capture | save → drive (rules), drive → telegram (retrieval) | Save Service, Retrieval Service, registered tools `drive_save_file`, `drive_search`, `drive_get_file` |
| 021 Intelligence delivery | drive artifacts appear in daily digest with provider chip | Digest reads from artifact store; no special path |
| 026 Domain extraction | drive artifacts feed extraction queue when classified as recipe/receipt/etc. | `drive.classify.result` triggers `domain.extract.request` for the right domain |
| 027 Annotations | annotations attach to drive artifacts via `artifacts.id` (head version); version chain visible on Screen 6 Versions tab | No drive-side change; annotation package reads `version_chain` for diff context |
| 028 Lists | list items can be sourced from drive artifacts (e.g., shopping list from receipt) | List package consumes via artifact store |
| 033 Mobile capture | mobile-captured files routed by Save Rules same as Telegram | Save Rules engine `source_kinds` includes `mobile` |
| 034 Expenses | receipts saved to drive contribute to expense intelligence | `drive.classify.result.classification='receipt'` is consumed by `internal/intelligence/` via NATS |
| 035 Recipes | recipe PDFs/photos in drive populate recipe library | Same as 034 with `classification='recipe'` |
| 036 Meal plan | weekly meal plan synthesis is saved back to drive at user-selected folder | Meal-plan service issues a `drive.save.request` via the Save Service public Go API |
| 037 Agent + tools | scenario agent uses `drive_search`, `drive_get_file`, `drive_save_file`, `drive_list_rules` to drive flows | Tool registrations declared in `internal/drive/tools.go`; allowlists per scenario file |

This satisfies UC-006 (meal plan → drive), UC-004 (Telegram → drive),
UC-005 (drive → Telegram), and the Cross-Feature Integration Map in spec.md.

---

## 8. Sensitivity, Permission, and Privacy (FR-014, IP-006, BS-011, BS-017, BS-021, BS-022)

### 8.1 Sensitivity tiers

`none | financial | medical | identity`. Stored on `drive_files.sensitivity`
with the CHECK constraint declared in §3.4 — there is no `artifacts.sensitivity`
column today, and Scope 1 deliberately does not add one. Cross-feature search
that needs sensitivity-aware filtering joins against `drive_files`. A future
scope MAY introduce a generic `artifacts.sensitivity` column (with its own
migration) when non-drive surfaces require sensitivity at the artifact level;
that decision is out of scope here.

Tier is set by:
- LLM classification output (`classify.py`).
- Folder context output (FR-008, e.g. a folder named `Medical/` contributes
  a `medical` sensitivity prior).
- Explicit override by user (Screen 6 chip → policy edit).

Higher of the three wins.

### 8.2 Policy enforcement points

| Surface | Enforcement |
|---------|------------|
| Search results (Screen 5) | "Open in Drive ↗" requires confirm dialog when sensitivity ∈ {medical, financial, identity} and policy demands |
| Telegram retrieval (Screen 10) | Sensitivity restricts `Mode` to `secure_link` or `refused` per RetrievalService sensitivity policy (see §6); inline byte delivery is additionally bounded by `drive.telegram.max_inline_size_bytes` and `drive.telegram.max_link_files_per_reply` |
| Share suggestions / digests (021) | Sensitive artifacts excluded from any auto-shared digest unless user opted in per category |
| Save Rule editor (Screen 8) | `never_link_share` guardrail blocks rules from creating provider-side share links for matched artifacts |
| Provider-side share change (BS-011) | Change monitor detects sharing change → bumps sensitivity to at most the new sharing audience and re-evaluates rules |
| Permission loss (UC-011, BS-022) | Provider returns 403/404 → `drive_files.permission_lost_at` set; artifact remains queryable but bytes are unavailable; Screen 6 banner explains |

### 8.3 Privacy-preserving validation (FR-018)

Test fixtures live in `tests/integration/drive/fixtures/` and are
synthetic. The integration suite uses a disposable Compose project per
`./smackerel.sh test integration` rules — never the persistent dev drive.
End-to-end runs the real `GoogleDriveProvider` implementation against an
owned external-boundary fixture server with recorded Google Drive responses
and synthetic file bytes. That keeps provider code in the path while
keeping personal drives out of validation. Live sandbox contract tests run
through `./smackerel.sh test integration` with explicit test secret refs;
when those refs are absent the test reports blocked rather than passing.

---

## 9. Configuration (SST, FR-001, FR-002, FR-003, FR-014, NFR Configurability)

Authoritative SST surface; mirrored to [`config/smackerel.yaml`](../../config/smackerel.yaml)
and validated by [`internal/config/drive.go::loadDriveConfig`](../../internal/config/drive.go).
The 22 required keys below are emitted as `DRIVE_*` entries into
`config/generated/${TARGET_ENV}.env` by the SST generator. Every value is
REQUIRED at startup — the Go core fails loud and exits if any key is
missing or invalid. Angle-bracket annotations describe the required type
and constraint, not a runtime default.

```yaml
drive:
  enabled: <required boolean>                         # master enable flag for the drive subsystem
  classification:
    enabled: <required boolean>                       # enable LLM-driven classification of drive files
    confidence_threshold: <required decimal 0..1>     # min confidence to apply classification automatically
    low_confidence_action: <pause|skip|allow>         # action when confidence < confidence_threshold
  scan:
    parallelism: <required positive integer>          # concurrent provider scan workers
    batch_size: <required positive integer>           # files per provider page batch
  monitor:
    poll_interval_seconds: <required positive integer>                   # monitor cycle interval
    cursor_invalidation_rescan_max_files: <required positive integer>    # bounded rescan cap when cursor invalidates
  policy:
    sensitivity_default: <public|internal|sensitive|secret>  # default tier when classifier output is missing
    sensitivity_thresholds:
      public: <required decimal 0..1>                 # classifier confidence to label public
      internal: <required decimal 0..1>               # threshold for internal
      sensitive: <required decimal 0..1>              # threshold for sensitive
      secret: <required decimal 0..1>                 # threshold for secret
  telegram:
    max_inline_size_bytes: <required positive integer>     # ceiling for inline Telegram delivery
    max_link_files_per_reply: <required positive integer>  # max secure-link files per Telegram reply
  limits:
    max_file_size_bytes: <required positive integer>       # hard cap per drive file
  rate_limits:
    requests_per_minute: <required positive integer>       # provider request rate ceiling
  providers:
    google:
      oauth_client_id: <required string>             # empty placeholder allowed only when drive.enabled=false
      oauth_client_secret: <required string>         # empty placeholder allowed only when drive.enabled=false
      oauth_redirect_url: <required URL string>      # OAuth redirect callback URL
      scope_defaults: <required non-empty list of OAuth scope URLs>
```

Notes:

- When `drive.enabled=false`, `oauth_client_id` and `oauth_client_secret`
  MAY be empty placeholders for dev. When `drive.enabled=true`,
  `loadDriveConfig` enforces both as non-empty and exits otherwise.
- `scope_defaults` MUST be a non-empty JSON list when read from the env;
  the generator preserves quoted scope URIs as opaque list items.
- Per-connection OAuth refresh tokens are persisted by the connector
  layer (not via `config/smackerel.yaml`) and are out of scope for the
  SST schema above.
- NATS subjects and stream `DRIVE` are added to
  [config/nats_contract.json](../../config/nats_contract.json) and
  verified on both Go and Python startup (existing pattern).

---

## 10. API Surface (Web PWA → Go core)

| Method | Path | Purpose | Screen |
|--------|------|---------|--------|
| GET | `/api/v1/connectors` | List connectors with health and counters | 1 |
| POST | `/api/v1/connectors/drive/connect` | Begin add-drive flow. Body: `{provider, accessMode, scope}`. Returns `{authURL, state}`. Server INSERTs a `drive_oauth_states` row before responding. | 2 |
| GET | `/api/v1/connectors/drive/oauth/callback` | Provider redirect target. Query: `state`, `code`. Server calls the matching provider's `FinalizeConnect(state, code)`, persists the connection (including `expires_at`), deletes the `drive_oauth_states` row, and redirects the browser to Screen 3 with the new connection id. | 2 |
| POST | `/api/v1/connectors/drive/{id}/scope` | Persist folder scope | 2 |
| GET | `/api/v1/connectors/drive/{id}` | Connector detail + recent activity | 3 |
| POST | `/api/v1/connectors/drive/{id}/pause` `/resume` `/reconnect` `/disconnect` | Lifecycle | 3 |
| GET | `/api/v1/connectors/drive/{id}/skipped` | Skipped & blocked listing | 4 |
| POST | `/api/v1/connectors/drive/{id}/skipped/{file_id}/action` | Raise cap / exclude folder / retry | 4 |
| GET | `/api/v1/search?q=...&filters=...` | Drive-aware unified search | 5 |
| GET | `/api/v1/artifacts/{id}` | Artifact detail (preview/text/metadata/versions) | 6 |
| POST | `/api/v1/artifacts/{id}/save_copy` | Save copy to chosen folder | 6 |
| GET/POST/PUT/DELETE | `/api/v1/drive/rules` `/{id}` | Rule CRUD | 7, 8 |
| POST | `/api/v1/drive/rules/{id}/test` | Dry-run rule against artifact | 8 |
| GET | `/api/v1/drive/rules/audit` | Audit log | 7 |
| POST | `/api/v1/drive/confirmations/{id}` | Resolve a low-confidence confirmation | 11 |

All endpoints require the same auth surface as existing PWA routes; no
endpoint is anonymous. Sensitive operations (disconnect, share-link, open
sensitive in drive) require a re-confirmation modal client-side and a
`confirm: true` body field server-side.

Screen 11's Telegram fallback uses the existing Telegram bot router; the
selection is delivered to the same `/api/v1/drive/confirmations/{id}`
handler.

---

## 11. Failure Modes and Rollback

| Failure | Surface | Recovery |
|---------|---------|----------|
| Provider 401 (token expired) | Screen 1 status dot, Screen 3 banner | Connector marked degraded; refresh attempted; user prompted to reconnect |
| Provider rate limit | Scan/monitor backoff | Exponential backoff bounded by config; surfaced as `degraded` after threshold |
| Cursor invalidated | Monitor | Bounded rescan window from config; surfaced as a one-time progress bar on Screen 3 |
| Save target folder missing with `on_missing_folder=fail` | Telegram reply Screen 9 | "Create now" deep-link to rule editor |
| Save conflict (`on_existing_file=skip`) | `drive_save_requests.status='skipped'` | No retry; surfaced in audit log |
| Extract worker timeout | `drive_files.skip_reason='extract_timeout'` | Retried with exponential backoff up to config max; then surfaces in Screen 4 |
| Classify low confidence | `drive_files.classification_pending=true` | Screen 11 confirmation; on user pick, classification commits and feedback is recorded |
| Provider permanently revoked access | Screen 1 red dot; rules pointing at provider show `Reconnect` | Existing `drive_files` remain queryable; provider bytes unavailable until reconnection |
| Partial scan failure | Progress event with `partial=true` | Resume from last cursor; never restart from zero |

No design path performs a destructive action without an explicit user
gesture. There is no "auto-clean" of drive_files on connector disconnect;
artifacts remain queryable per the Outcome Contract.

---

## 12. Observability

- Metrics (Prometheus, existing scrape):
  `drive_scan_files_total{provider,connection,outcome}`,
  `drive_extract_duration_seconds_bucket{format}`,
  `drive_classify_confidence_bucket`,
  `drive_save_requests_total{provider,outcome}`,
  `drive_retrieval_total{channel,mode}`,
  `drive_provider_errors_total{provider,kind}`.
- Structured logs include `connection_id`, `provider`, `file_id`,
  `rule_id`, `idempotency_key` where applicable. No file bytes or
  extracted text are logged.
- Traces span `scan → extract → classify → rule → save` and
  `telegram_query → search → policy → deliver`. Existing trace helpers in
  [internal/agent/tracer.go](../../internal/agent/tracer.go) and
  [internal/metrics/trace.go](../../internal/metrics/trace.go) are reused.
- Screen 3 counter cards read directly from a small read model populated
  by these metrics so that UI numbers and metric numbers are reconciled
  by construction.

---

## 13. Test Strategy

| Layer | Where | What |
|-------|-------|------|
| Unit (Go) | `internal/drive/.../*_test.go` | Provider interface adherence; rule engine matching; idempotency key derivation; sensitivity policy decisions; cursor-invalidation rescan logic |
| Unit (Python) | `ml/tests/test_extract.py`, `ml/tests/test_classify.py` | Format routers; prompt contract input/output validation; folder-context summarization shape |
| Integration | `tests/integration/drive/` | Real `GoogleDriveProvider` against an owned fixture HTTP server plus real Go services, PostgreSQL, NATS, and ML-sidecar workers; covers BS-001..BS-009, BS-014..BS-019, BS-021, BS-022 |
| E2E | `tests/e2e/drive/` | Disposable Compose project; Telegram capture → rule evaluation → provider fixture write → search → Telegram retrieval (BS-004, BS-005, BS-014, BS-025); folder move triggers reclassification (BS-006); permission revocation surfaces correctly (BS-022) |
| Stress | `tests/stress/drive/` | 5,000-file/25 GB synthetic scan, monitor delta replay, save-back burst (NFR Performance) |

All tests are run via `./smackerel.sh test {unit,integration,e2e,stress}`.
No new ad-hoc commands.

Adversarial regression cases for bug fixes follow the standing rule in
`.github/copilot-instructions.md`: every regression must include a case
that would fail if the bug were reintroduced (e.g., reintroducing a
hardcoded MIME allow-list must break a unit test that asserts the list
comes from config).

---

## 14. FR / IP / Scenario Coverage Map

| ID | Where covered |
|----|---------------|
| FR-001 | §2.1, §2.2 |
| FR-002 | §2.3 |
| FR-003 | §3 |
| FR-004 | §5.3, §10 |
| FR-005 | §4.1 |
| FR-006 | §4.2 |
| FR-007 | §3, §4.3, schema in §5.1 |
| FR-008 | §4.3 |
| FR-009 | §5 |
| FR-010 | §6 |
| FR-011 | §7 |
| FR-012 | §7 |
| FR-013 | §3.2, §4.1, §11, Screen 4 |
| FR-014 | §6, §8 |
| FR-015 | §5.3, §11 |
| FR-016 | §4.2, Screen 11 endpoint in §10 |
| FR-017 | §3.1, §11, §12 |
| FR-018 | §8.3, §13 |
| IP-001 | §2.2 |
| IP-002 | §5 |
| IP-003 | §7 |
| IP-004 | §5.3, §7 |
| IP-005 | §4.1, §13 |
| IP-006 | §4.2, §8 |
| IP-007 | §3.2, §4.1, §11, Screen 4 |
| IP-008 | §3.4, §10, Screen 6 |
| IP-009 | §3.3, §8.2 (BS-001, BS-009, BS-019) |
| IP-010 | §6, §10, §12 |

### Scenario coverage

| Scenario | Design location |
|----------|-----------------|
| BS-001 | §3 bulk scan, §4 folder context, §13 integration |
| BS-002 | §4.1 PDF extraction, §7 recipe integration, §13 integration |
| BS-003 | §4.1 image OCR, §7 expense integration, §13 integration |
| BS-004 | §5 save rules, §5.3 save service, §13 E2E |
| BS-005 | §7 meal-plan production, §13 E2E |
| BS-006 | §3.2 moved-file delta, §4.3 folder-as-taxonomy, §13 E2E |
| BS-007 | §3.4 native Google Doc representation, §10 artifact detail |
| BS-008 | §2.2 provider registration |
| BS-009 | §4.1 skip reasons, §11 failure modes, Screen 4 |
| BS-010 | §3.4 owner/sharing columns, §6 audience filter, §8 policy |
| BS-011 | §8.2 provider-side share change |
| BS-012 | §4.1 encrypted extraction block, §11 failure modes |
| BS-013 | §3.4 version_chain, §10 Versions tab |
| BS-014 | §5.2 guardrails, §8 policy enforcement, §13 E2E |
| BS-015 | §4.2 low-confidence confirmation, §10 confirmation endpoint |
| BS-016 | §5.1 `drive_folder_resolutions`, §5.3 transactional folder creation |
| BS-017 | §3.2 tombstone handling, §3.4 tombstoned_at, §11 rollback |
| BS-018 | §2.3 Google connection, §3.1 empty scan behavior |
| BS-019 | §3.2 folder move handling, §3.3 bounded rescan, §13 integration |
| BS-020 | §11 provider outage, §12 provider error metrics |
| BS-021 | §2 provider-neutral model, §6 unified retrieval, §10 search API |
| BS-022 | §4.3 folder context, §8.1 sensitivity tiers, §7 expense/domain consumption |
| BS-023 | §4.1 audio extraction, §13 Python unit coverage |
| BS-024 | §4.1 OCR fallback, §13 Python unit coverage |
| BS-025 | §6 retrieval service, §10 Telegram confirmation path, §13 E2E |

---

## Open Questions

No blocking design questions remain. Analyst-flagged questions are resolved
as follows:

| Analyst question | Design resolution |
|------------------|-------------------|
| 1. Native Google Doc representation | Canonical `(connection_id, provider_file_id)` in §3.4, exported snapshot refreshed per provider revision. |
| 2. Sensitivity threshold and policy schema | Four-tier sensitivity model and enforcement matrix in §8; configured threshold in §9. |
| 3. Save-rule conflict resolution | First stable match executes, all matches audited as conflicts, Screen 7 surfaces comparison in §5.2. |
| 4. Folder include/exclude expressiveness | Include list + exclude list + `max_depth` modeled as per-connection scan-rule fields (see §3.4 schema); no custom DSL. Global SST in §9 governs subsystem-wide knobs only. |
| 5. Provider capability mapping | `Capabilities()` in §2.1 governs provider-specific features without leaking to downstream consumers. |
| 6. Encrypted PDF handling | Extraction-blocked state only in §4.1/§11; no password capture surface in this feature. |
| 7. Quota visibility surface | Screen 3 counters + Screen 4 grouped skipped/blocked list; metrics in §12. |
| 8. Change-history continuity | `drive_cursors` bounded rescan strategy in §3.3. |
| 9. Mobile capture and Drive precedence | Shared idempotency identity through `drive_save_requests` in §5.1/§5.3. |
| 10. Cross-feature test fixtures | Synthetic fixtures under `tests/integration/drive/fixtures/` and cross-feature integration suites in §13. |
| 11. Cross-channel retrieval delivery | `RetrieveDelivery.Mode` decision table in §6 and policy enforcement in §8. |

### Decision log — Scope 1 round 5 contract resolution

- **Blocker A (drive_connections.expires_at missing):** **Resolved A1 — additive
  column.** Migration `022_drive_connection_expires_at.sql` adds
  `expires_at TIMESTAMPTZ NULL` directly on `drive_connections`. Token
  expiry is a property of the active connection at the same row level as
  `status` and `scope`, matches the existing schema style, and avoids
  introducing a child credentials table whose abstraction value is not
  yet justified by a second provider. A future scope MAY indirect
  credentials through a `drive_credentials` child table once a non-Google
  provider's token model demands it; until then this single nullable
  column is the smallest correct change. Rationale for rejecting A2: a
  `drive_credentials` table or JSONB-keyed indirection adds surface
  (writes, reads, migrations, fixtures) without behavior, and would have
  to be rewritten anyway when the real second-provider contract lands.
- **Blocker B (`Connect` cannot drive an OAuth redirect):** **Resolved B1 —
  split into `BeginConnect` + `FinalizeConnect`.** `BeginConnect` issues
  the auth URL and persists a `drive_oauth_states` nonce; the provider
  redirects the user back to
  `GET /api/v1/connectors/drive/oauth/callback`, which calls
  `FinalizeConnect(state, code)` to exchange the code, persist the
  connection, and delete the nonce. This is the standard OAuth web-flow
  shape used by virtually every external integration. Rationale for
  rejecting B2 (one `Connect` plus an awkward finalizer): conflates two
  distinct lifecycle steps onto one method name and forces callers to
  branch on whether the return value is an auth URL or a connection id.
  Rationale for rejecting B3 (test-only programmatic code path): would
  require a fixture-only branch in production code, violating the
  project rule that production code paths exercise tests, not the other
  way around. The fixture in §8.3 instead serves the standard OAuth
  endpoints and is selected purely via the SST `oauth_base_url` /
  `api_base_url` indirection, so production `BeginConnect` /
  `FinalizeConnect` are the only code paths.
- **Migration:** `022_drive_connection_expires_at.sql` (additive
  `ALTER TABLE drive_connections ADD COLUMN expires_at TIMESTAMPTZ NULL;`)
  plus the new `drive_oauth_states` table. Rollback drops the column and
  the table.
- **Test contracts unchanged:** SCN-038-001/002/003 names and intent are
  preserved; only signatures and persisted columns change. Scope 1's
  scoped scenarios still hold.
