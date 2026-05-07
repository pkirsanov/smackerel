# Design: QF Companion Connector

## Design Brief

### Current State

Smackerel already has a passive connector framework in `internal/connector`, including `Connector`, `ConnectorConfig`, `RawArtifact`, `StateStore`, `Registry`, and `Supervisor`. The docs also define the QF Decisions connector boundary: QF packets are authoritative external artifacts, not Smackerel-local recommendations.

QuantitativeFinance is adding a pre-MVP private-alpha read/import bridge in `quantitativeFinance/specs/063-smackerel-companion-bridge`. That bridge provides read-only QF decision events and packet envelopes, plus a reverse import path for consent-scoped `PersonalEvidenceBundle`s.

### Target State

Smackerel adds a standard passive connector named `qf-decisions`. It syncs QF decision events by cursor, normalizes packet envelopes into source-qualified artifacts, surfaces them read-only in Web, Telegram, digest, and search, and exports selected Smackerel context back to QF as evidence bundles.

Smackerel remains a companion, memory, attention, and context surface. It does not provide financial advice, does not upgrade QF trust metadata, does not mutate QF approval state, and does not execute trades.

### Patterns To Follow

- `internal/connector/connector.go` for connector interface, config shape, `RawArtifact`, and health states.
- `internal/connector/state.go` for persisted cursor and error state in PostgreSQL.
- `internal/connector/supervisor.go` for sync loop, backoff, panic recovery, publisher handoff, and sync interval handling.
- Existing connector package pattern such as `internal/connector/guesthost`, including config validation, HTTP client separation, normalization, and health transitions.
- Smackerel docs in `docs/Connector_Development.md`, `docs/Testing.md`, and `docs/Operations.md` for QF connector boundary and operations.
- Existing PWA/web connector status and detail patterns from `web/pwa/connectors.html`, `web/pwa/connector-detail.html`, and `web/pwa/drive-artifact-detail.html` for health, diagnostics, artifact metadata, tabbed detail, and accessible status messaging.
- Existing Telegram and digest delivery patterns from `internal/telegram/bot.go` and `internal/api/digest.go`, preserving compact channel-safe summaries and server-owned digest retrieval.

### Patterns To Avoid

- No direct QF database access.
- No Kafka, NATS, or broker federation with QF in pre-MVP.
- No local recommendation artifact type for QF packets.
- No prompt contract may rewrite QF thesis, approval state, badges, or trust labels.
- No approval, mandate, EmergencyStop, paper execution, or live execution controls in pre-MVP.

### Resolved Decisions

- Default connector ID is `qf-decisions`.
- Package location is `internal/connector/qfdecisions`.
- QF packet artifacts use source-qualified content types such as `qf/decision-packet`, `qf/no-action-decision`, and `qf/policy-denial`.
- `RawArtifact.SourceID` is `qf-decisions` and stable packet identity is the QF `packet_id`.
- Required metadata includes QF packet ID, intent ID, scenario ID, trace ID, approval state, deep link, `CalibrationBadge`, and `DataProvenanceBadge`.
- Web surfaces include QF connector status, QF packet search/digest card, QF packet artifact detail, and a PersonalEvidenceBundle builder.
- Telegram renders QF packets as compact read-only summaries with QF source, trust labels, trace, and deep link.
- Schema mismatch and missing-trust states produce diagnostics and degraded connector health rather than trusted packet cards.

### Open Questions

- Exact QF private-alpha base path and credential type once QF implementation chooses HTTP polling or an outbox-backed read projection.
- Whether digest ranking treats QF packets as a dedicated section or source-qualified items inside existing finance/context sections.
- Whether the first Web implementation lands in existing HTMX routes, PWA pages, or both surfaces at once.

## Purpose And Scope

This design defines the Smackerel side of the QF companion integration. It covers connector architecture, sync lifecycle, cursor behavior, health states, artifact mapping, surfacing, evidence bundle export, prompt boundaries, action gating, operations, testing, and release mapping.

The connector is pre-MVP and read-only for QF decisions. It can export personal evidence to QF only with explicit consent and source provenance.

## Connector Architecture

The connector follows the existing Smackerel connector lifecycle.

```text
config/smackerel.yaml
    -> config generation
    -> ConnectorConfig
    -> qfdecisions.Connect()
    -> Supervisor.StartConnector("qf-decisions")
    -> qfdecisions.Sync(cursor)
    -> []connector.RawArtifact + new cursor
    -> ArtifactPublisher.PublishRawArtifact()
    -> PostgreSQL + processing/search/digest pipeline
```

### Package Layout

| File | Purpose |
|------|---------|
| `internal/connector/qfdecisions/connector.go` | Implements `connector.Connector`, health transitions, sync lifecycle. |
| `internal/connector/qfdecisions/client.go` | QF HTTP client for decision event list, packet fetch, and evidence export. |
| `internal/connector/qfdecisions/types.go` | QF bridge DTOs mirrored from the QF design. |
| `internal/connector/qfdecisions/normalizer.go` | Converts QF packet envelopes into `connector.RawArtifact`. |
| `internal/connector/qfdecisions/evidence_bundle.go` | Builds and validates `PersonalEvidenceBundle` exports. |
| `internal/connector/qfdecisions/*_test.go` | Unit and regression coverage for validation, sync, normalization, health, and evidence export. |

No source files are changed by this design; the layout records the intended implementation boundary.

### Configuration

`config/smackerel.yaml` owns connector configuration. When enabled, all required values must be explicit.

| Key | Required When Enabled | Rule |
|-----|-----------------------|------|
| `connectors.qf-decisions.enabled` | yes | Starts or stops the connector. |
| `connectors.qf-decisions.base_url` | yes | QF private-alpha base URL. Must be valid `http` or `https`. |
| `connectors.qf-decisions.credential_ref` | yes | Reference to generated secret or environment-backed credential. No inline production secret. |
| `connectors.qf-decisions.sync_schedule` | yes | Explicit schedule consumed by the supervisor. |
| `connectors.qf-decisions.packet_version` | yes | QF bridge contract version requested during sync. |
| `connectors.qf-decisions.page_size` | yes | Explicit page size sent to QF. Missing or invalid values fail connection. |

The connector must fail `Connect()` for missing base URL, missing credential reference, missing contract version, invalid URL, or invalid page size.

## Sync Lifecycle

### Connect

`Connect(ctx, cfg)` validates configuration, constructs the QF client, verifies the QF bridge health/schema endpoint if available, and transitions health from `disconnected` to `healthy` only after validation succeeds.

Required checks:

- `base_url` is syntactically valid and has no trailing slash in the client.
- Credential reference resolves through generated configuration or runtime environment.
- Requested packet contract version is non-empty.
- Page size is explicit and within QF's accepted limit.
- QF bridge responds with compatible schema metadata if the health/schema endpoint is present.

### Sync

`Sync(ctx, cursor)` receives the opaque QF cursor stored in `sync_state.sync_cursor` and returns QF artifacts plus the next QF cursor.

Sync steps:

1. Set health to `syncing`.
2. Call QF decision event list with cursor, page size, and packet version.
3. For each event, fetch the full packet envelope when the event list does not inline it.
4. Validate packet IDs, trace ID, badge objects, approval state, and deep link.
5. Normalize valid packet envelopes into `RawArtifact`s.
6. Preserve degraded diagnostics for invalid packets without producing actionable packet cards.
7. Return the QF `next_cursor` unmodified.
8. Restore health to `healthy` when no blocking validation or transport errors occurred.

The connector must not advance local approval state or produce action commands during sync.

### Cursor Semantics

| Cursor Rule | Design |
|-------------|--------|
| Storage | Use existing `sync_state.sync_cursor` for source ID `qf-decisions`. |
| Format | Opaque QF-issued string. Smackerel stores and returns it exactly. |
| First sync | Empty cursor asks QF for the credential-scoped first page. |
| Replay | Operator may clear only the `qf-decisions` cursor. Packet identity stays stable through `packet_id`. |
| Deduplication | `RawArtifact.SourceRef` is the QF `packet_id`; `event_id` is stored in metadata. |
| Updates | Later QF events for the same `packet_id` update or supersede the source-qualified packet view without inventing a new QF packet ID. |

## Health States

The connector uses existing `connector.HealthStatus` values.

| Health | QF Connector Meaning |
|--------|----------------------|
| `disconnected` | Connector has not connected or has been closed. |
| `healthy` | Last sync succeeded and required trust metadata was present. |
| `syncing` | Sync cycle is currently polling QF and normalizing packets. |
| `degraded` | QF reachable, but some packets failed schema/trust validation or QF reports a compatible warning state. |
| `failing` | Repeated sync errors approach supervisor circuit-breaker thresholds. |
| `error` | Config, auth, schema, or transport failure prevents usable sync. |

Degraded packet behavior is intentionally conservative: Smackerel can record diagnostics, but it must not render a degraded packet as an actionable or fully trusted decision card.

## QF Artifact Mapping

### RawArtifact Mapping

| RawArtifact Field | QF Mapping |
|-------------------|------------|
| `SourceID` | `qf-decisions` |
| `SourceRef` | QF `packet_id` exactly |
| `ContentType` | `qf/decision-packet`, `qf/no-action-decision`, `qf/policy-denial`, or reserved `qf/approval-request` diagnostic only |
| `Title` | QF-authored short title derived from packet thesis or decision type, without changing meaning |
| `RawContent` | Canonical serialized QF envelope received from QF |
| `URL` | QF `deep_link` exactly |
| `Metadata.packet_id` | QF `packet_id` exactly |
| `Metadata.intent_id` | QF `intent_id` exactly |
| `Metadata.scenario_id` | QF `scenario_id` exactly |
| `Metadata.trace_id` | QF `trace_id` exactly |
| `Metadata.approval_state` | QF display-only approval state exactly |
| `Metadata.calibration_badge` | QF `CalibrationBadge` object exactly |
| `Metadata.data_provenance_badge` | QF `DataProvenanceBadge` object exactly |
| `Metadata.event_id` | QF event ID for provenance |
| `Metadata.packet_version` | QF bridge contract version |
| `CapturedAt` | QF packet `created_at` or event `created_at` as defined by the envelope |

### Content Types

| Content Type | Meaning | QF `decision_type` Source | Rendering Rule |
|--------------|---------|---------------------------|----------------|
| `qf/decision-packet` | QF-authored recommendation packet (or analysis note when subtype set) | `recommendation`, and `analysis_note` (with `Metadata.decision_subtype = "analysis_note"`) | Read-only card with badges, trace, approval state, and QF link. Analysis-note variants render with the same trust metadata but should be visually distinguishable from recommendations using the subtype metadata. |
| `qf/no-action-decision` | QF-authored decision not to act | `no_action` | Read-only card emphasizing QF no-action rationale. |
| `qf/policy-denial` | QF-authored policy denial or blocked decision | `policy_denial` | Read-only card with denial reason and trust metadata. |
| `qf/approval-request` | Reserved diagnostic content type tied to inbound `QFApprovalAction` | None — `QFApprovalAction` is not a decision packet | Not rendered as an action in pre-MVP. Inbound action objects are recorded as diagnostics only. |

This content-type binding is the cross-repo contract with `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` ("Decision Type To Smackerel Content Type Mapping"). Smackerel MUST NOT invent additional `qf/...` content types pre-MVP.

### Required Validation

The normalizer rejects packet-card rendering unless all required QF metadata is present:

- `packet_id`
- `intent_id`
- `scenario_id`
- `trace_id`
- `approval_state`
- `deep_link`
- `CalibrationBadge`
- `DataProvenanceBadge`
- `packet_version`

Rejected packets are counted and surfaced through connector health. The connector may retain a diagnostic raw artifact only if it is clearly marked as non-actionable and excluded from recommendation surfaces.

## Web, Telegram, Digest, And Search Surfacing

### Web

Web surfaces render QF packets as external authoritative artifacts:

- Source label: `QuantitativeFinance`.
- Connector label: `qf-decisions`.
- QF packet ID, trace ID, approval state, calibration badge, provenance badge, and deep link visible.
- No approve, execute, mandate, or EmergencyStop controls in pre-MVP.
- Degraded packet cards show diagnostic state only and do not present a decision as trusted.

### Telegram

Telegram summaries are compact and channel-safe:

- Include QF source label, thesis excerpt, approval state, badge status text, trace ID, and QF deep link.
- Do not include action buttons for approval or execution in pre-MVP.
- Do not phrase Smackerel summary as buy/sell/hold advice unless that exact wording comes from QF packet content.

### Digest

Digest integration treats QF packets as QF-authored items:

- High-priority packets can appear in a QF section or source-qualified finance section.
- Digest text may summarize packet context, but must preserve QF trust labels and link to the authoritative QF packet.
- Quiet/sensitivity rules still apply to Smackerel delivery, but they do not rewrite QF packet state.

### Search

QF packets are searchable by:

- `packet_id`
- `trace_id`
- `intent_id`
- `scenario_id`
- symbols and entities from QF metadata
- thesis and why-now text
- related Smackerel concepts connected through source artifacts

Search results must distinguish QF-authored packet content from Smackerel-derived context.

## UI Surface And Component Design

Smackerel renders QF packets as source-qualified, read-only artifacts. UI components may summarize for space, but must keep QF-authored thesis, approval state, trust badges, packet IDs, trace IDs, and deep links visibly distinct from Smackerel context.

### QF Connector Status

Routes: `/settings` and `/status`

| Component | Responsibility | Data Source | Required States |
|-----------|----------------|-------------|-----------------|
| `QFConnectorStatusPanel` | Shows connector ID, enabled state, health, last sync, cursor lag, packet version, schema compatibility, and required metadata checklist. | Connector supervisor health plus `qf-decisions` state store. | not configured, syncing, healthy, degraded, error. |
| `QFConnectorDiagnosticsTable` | Lists auth failures, schema mismatch, validation failures, packet ID, trace ID, event time, and reason. | Connector diagnostic records and health transition metadata. | empty diagnostics, expandable rows, error rows. |
| `QFConnectorActions` | Sync now, view diagnostics, and disable connector through existing config-owned workflow. | Existing connector control path. | disabled while syncing, credential-safe errors, no credential display. |

Rules:

- Sync Now triggers the normal connector supervisor pattern and cannot expose credential material.
- Disable connector stops new syncs but does not delete existing synced artifacts.
- Schema/version mismatch marks health degraded or error and suppresses trusted packet rendering.
- Mobile diagnostics render as cards keyed by packet ID; desktop keeps a table-style diagnostic view.
- Health uses text and status icons; `aria-live` status text announces sync results.

### QF Packet Search And Digest Card

Routes: `/`, `/digest`

| Component | Responsibility | Data Source | Required States |
|-----------|----------------|-------------|-----------------|
| `QFPacketResultCard` | Search/result card with source label, connector label, QF title, approval state, calibration badge text, provenance badge text, trace ID, thesis excerpt, and actions. | Stored QF packet artifact metadata and raw envelope. | normal, loading, degraded diagnostic, no match. |
| `QFDigestItem` | Compact daily digest entry preserving QF source, approval state, trust summary, why-now excerpt, and QF deep link. | Digest assembly from stored QF artifact. | included, omitted by sensitivity/quiet policy, degraded diagnostic omitted from advice surfaces. |
| `QFPacketActions` | Open detail, open in QF, build evidence bundle. | Artifact metadata and evidence-builder route state. | open only, build bundle, no approval/execution. |

Rules:

- Thesis excerpts are QF-authored excerpts, not rewritten Smackerel recommendations.
- Open in QF uses the `deep_link` exactly as supplied by QF.
- Build evidence bundle starts a context-selection flow and cannot treat the QF packet itself as Smackerel advice.
- A degraded card shows source and diagnostic reason, but hides thesis excerpts and action affordances that could imply trust.
- Badges and source labels are text-visible and do not rely on color.

### QF Packet Artifact Detail

Route: `/artifact/{id}`

| Component | Responsibility | Data Source | Required States |
|-----------|----------------|-------------|-----------------|
| `QFPacketTrustHeader` | Displays QF source, content type, packet ID, trace ID, intent ID, scenario ID, approval state, and badges. | Artifact metadata. | normal, degraded, missing metadata diagnostic. |
| `QFPacketContentPanel` | Shows QF-authored thesis, why-now, and quantified impact summary. | Raw QF envelope. | normal, hidden when validation failed. |
| `CompanionContextSelector` | Lets the user select related Smackerel artifacts, concepts, entities, and market/news context for bundle building. | Existing search/graph/context results. | empty related context, loading, selectable, error without hiding QF trust metadata. |
| `ActionBoundaryNotice` | Explains that approval, execution, mandate changes, and EmergencyStop remain QF-owned release-gated actions. | Static policy text plus QF support status when available. | always visible for QF packets in pre-MVP. |

Rules:

- Related context checkboxes include source title, content type, sensitivity, and selection purpose in labels.
- Degraded packet detail suppresses evidence-building entry points and renders diagnostic metadata only.
- QF deep-link text includes packet title or ID.
- The action-boundary notice is in reading order before any action-like controls.

### Personal Evidence Bundle Builder

Route: `/evidence-bundles/new`

| Component | Responsibility | Data Source | Required States |
|-----------|----------------|-------------|-----------------|
| `SelectedSourceList` | Lists selected artifacts/concepts/entities, supports source removal, and opens read-only previews. | User-selected Smackerel context. | empty, selected, source unavailable. |
| `BundleMetadataForm` | Requires consent scope and sensitivity tier and displays confidence and redaction summary. | User input plus bundle generation result. | missing consent, missing sensitivity, valid metadata. |
| `ExtractedClaimsTable` | Shows claim, source IDs, related symbol/entity, and confidence. | Bundle extraction result. | extracting, valid claims, missing source references. |
| `BundleExportStatus` | Validates bundle, exports to QF, and records QF import status. | QF import response and local export record. | validation failed, exporting, accepted, rejected. |

Rules:

- Export is unavailable until validation confirms source IDs, claims, consent, sensitivity, provenance, and redaction summary.
- Success state shows bundle ID, QF import status, and source count.
- Export failure keeps the draft visible so the user can correct metadata or retry.
- Mobile renders source selection, metadata, claims, and export as a single-column stepper.
- Validation summary focuses the first invalid section and announces status via `aria-live`.

### Telegram QF Packet Summary

Surface: Telegram digest/message output

| Element | Required Content |
|---------|------------------|
| Source line | `QuantitativeFinance · qf-decisions` |
| Title | QF packet title or packet ID when title is unavailable. |
| Trust block | Display-only approval state, calibration badge text, provenance badge text, and shortened trace ID. |
| Body | QF-authored why-now excerpt within Telegram length constraints. |
| Link | QF deep link. |
| Boundary notice | Read-only in Smackerel; actions happen in QF. |

Rules:

- No Telegram approval, execution, mandate, watch, or EmergencyStop buttons appear in pre-MVP.
- Degraded packet messages use diagnostic wording and link to connector status rather than packet detail.
- Long IDs are visually shortened only when the full values remain available in Web detail.
- Text order is source, title, trust, trace, excerpt, link, boundary notice.

## UX Contract Reconciliation Matrix

| Spec UX Contract | Active Design Binding | Non-Negotiable Rule |
|------------------|-----------------------|---------------------|
| QF Connector Status | `/settings`, `/status`, `QFConnectorStatusPanel`, `QFConnectorDiagnosticsTable`, and `QFConnectorActions`. | Auth failures, schema/version mismatch, cursor lag, validation failures, and missing trust metadata are surfaced as connector health or diagnostics; credential material is never displayed. |
| QF Packet Search And Digest Card | Search route `/`, digest route `/digest`, `QFPacketResultCard`, `QFDigestItem`, and `QFPacketActions`. | Cards show QF source, packet ID, trace ID, display-only approval state, trust badges, and QF deep link; Smackerel does not rewrite QF thesis or present local financial advice. |
| QF Packet Artifact Detail | `/artifact/{id}`, `QFPacketTrustHeader`, `QFPacketContentPanel`, `CompanionContextSelector`, and `ActionBoundaryNotice`. | Detail view keeps QF-authored content and Smackerel companion context separate, suppresses evidence-building for invalid packets, and shows action-boundary language before any action-like control. |
| Personal Evidence Bundle Builder | `/evidence-bundles/new`, `SelectedSourceList`, `BundleMetadataForm`, `ExtractedClaimsTable`, and `BundleExportStatus`. | Export requires explicit consent scope, sensitivity tier, source artifact IDs, claims, provenance, redaction summary, and target context before a QF import request is sent. |
| Telegram QF Packet Summary | Telegram digest/message formatter and QF packet summary renderer. | Telegram output is compact but still carries QF source, display-only approval state, badge text, trace context, deep link, and read-only boundary; no approval, execution, mandate, watch, or EmergencyStop buttons appear. |
| Sync and rendering flow | `qf-decisions` connector sync, packet validation, artifact normalization, search/digest/detail/Telegram renderers. | Valid packets become source-qualified read-only artifacts; schema mismatch or missing required metadata becomes diagnostic/degraded state and cannot render as a trusted packet card. |
| Evidence export flow | Bundle builder, QF export client, export status record, and local source artifact references. | Smackerel exports compact evidence bundles with provenance and redaction, never raw personal dumps or QF decision/action instructions. |
| Schema and action-boundary flow | Connector health diagnostics, degraded packet behavior, `ActionBoundaryNotice`, and safety gating. | Unknown packet versions and unsupported action classes are blocked, logged, and routed to diagnostics without QF action calls or local approval/execution behavior. |

The UX sections in `spec.md` are active design input for Smackerel surfaces. Implementation can reuse existing HTMX/PWA templates or introduce dedicated QF components, but it must preserve the routes, channel states, accessibility expectations, connector diagnostics, export validation, and read-only financial authority boundary described above.

## Technical BDD Scenarios

### TSC-SM-041-001: QF Packet Sync And Read-Only Surfacing

Given the `qf-decisions` connector is configured with explicit base URL, credential reference, packet version, page size, and sync schedule  
When the connector polls QF and receives a packet envelope containing packet ID, intent ID, scenario ID, trace ID, approval state, deep link, `CalibrationBadge`, and `DataProvenanceBadge`  
Then Smackerel stores a `qf/decision-packet` artifact with the same metadata  
And search, digest, artifact detail, and Telegram surfaces show the packet read-only with QF source labels.

### TSC-SM-041-002: Schema Mismatch Produces Diagnostic Only

Given QF returns an unknown packet version or omits required trust metadata  
When the connector syncs the event  
Then Smackerel marks the connector degraded or error with packet ID, trace ID when safe, and reason  
And no trusted packet card, digest item, Telegram decision summary, or evidence-builder entry point renders for that packet.

### TSC-SM-041-003: Evidence Bundle Export

Given the user selects Smackerel artifacts, concepts, entities, or market/news context around a QF packet or analysis context  
When the user chooses consent scope and sensitivity tier and validates the bundle  
Then Smackerel generates a `PersonalEvidenceBundle` with source artifact IDs, related symbols/entities, extracted claims, confidence, provenance, redaction summary, and created timestamp  
And exports it to QF only after validation passes.

### TSC-SM-041-004: Action Boundary

Given a QF packet is visible in Smackerel pre-MVP  
When the user searches, opens detail, reads digest, or receives Telegram output  
Then approve, execute, mandate, watch, and EmergencyStop controls are absent or disabled  
And the UI directs supported action completion to the QF deep link.

## Access And Authorization Matrix

| Surface / Operation | Smackerel User | QF Connector | Personal Evidence Curator | Future Advisor/Operator | Public |
|---------------------|----------------|--------------|---------------------------|--------------------------|--------|
| Configure `qf-decisions` | Yes, through config-owned workflow | No | No | Tenant-scoped in later release | No |
| Sync QF decision events | No direct UI call | Read-only credential-scoped sync | No | Tenant/service credential in later release | No |
| View QF packet cards/details | Yes, if artifact is in local scope | No UI access | Yes, if same user scope | Tenant/client scoped in later release | No |
| Build evidence bundle | Yes | No | Yes | Tenant/client scoped in later release | No |
| Export evidence bundle to QF | Yes, after explicit consent | Uses configured credential for transport | Yes | Tenant/client scoped in later release | No |
| Approval/execution/mandate/EmergencyStop | No pre-MVP mutation | Never | Never | QF-owned release-gated behavior | No |

## Evidence Bundle Export Design

`PersonalEvidenceBundle` exports selected Smackerel context to QF. The export is user-initiated and consent-scoped.

### Bundle Sources

A bundle can include references to:

- Smackerel artifacts selected by the user.
- Concept/entity pages.
- Market/news context already captured by Smackerel.
- Notes, bookmarks, messages, or research trails related to a symbol or topic.
- Cross-source connections that cite underlying artifact IDs.

### Bundle Fields

| Field | Rule |
|-------|------|
| `bundle_id` | Smackerel-generated ID. Required. |
| `export_id` | Idempotency key for QF import. Required. |
| `source_artifact_ids` | Required list of Smackerel artifact IDs. Canonical source-reference set; QF stores it verbatim. |
| `source_refs` | Optional source-qualified external references (URL, message-id, mailbox-id, RSS GUID, etc.) when the underlying source has them. Absent when no external IDs apply. |
| `related_symbols` | Optional. Symbol list extracted from selected context. |
| `related_entities` | Optional. Entities extracted from selected context. |
| `extracted_claims` | Required. Claims tied to source artifact IDs and confidence. |
| `confidence` | Bundle-level confidence derived from source coverage and extraction quality. |
| `sensitivity_tier` | Required. Explicit sensitivity classification (`personal`, `sensitive`, `restricted`). |
| `consent_scope` | Required. Explicit purpose, target QF context, and revocation/expiry reference. |
| `provenance` | Required. Smackerel generation lineage and source-processing references. |
| `redaction_summary` | Required. What raw personal material was summarized or omitted. |
| `target_context` | Required. QF analysis/run/Rhai/guided-analysis attachment target identifier. Pre-MVP values are `guided_analysis`, `rhai_run`, `saved_result`, or `analysis_context` per the QF bridge contract. Must be selected before validation can pass. |
| `created_at` | Required. RFC3339 timestamp. |

This required-field set MUST stay aligned with the QF acceptance criteria in `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` so that a Smackerel-generated bundle that passes local validation also passes QF import validation.

### Export Flow

```text
User selects Smackerel context
    -> export dialog shows target QF context and sensitivity
    -> user grants explicit consent scope
    -> Smackerel builds PersonalEvidenceBundle
    -> Smackerel validates required source and provenance fields
    -> qf-decisions client posts bundle to QF import endpoint
    -> QF returns accepted or rejected status
    -> Smackerel records export status and QF import reference
```

Smackerel must not export a raw personal data dump. The bundle is a compact evidence object with source references and claims.

## Prompt And Synthesis Boundary

Smackerel prompt contracts can assist with summarization and cross-linking, but they cannot become QF decision authors.

| Prompt Use | Allowed |
|------------|---------|
| Summarize QF packet for digest length | Yes, only if QF thesis, approval state, and trust metadata remain intact. |
| Cross-link QF packet to personal context | Yes, with source citations and clear Smackerel context labeling. |
| Build evidence bundle claims from selected artifacts | Yes, with source artifact IDs and confidence. |
| Rewrite QF thesis or why-now | No. |
| Upgrade or soften QF trust badges | No. |
| Hide QF downgrade or stale state | No. |
| Generate buy/sell/hold advice from Smackerel context | No. |
| Emit approval or execution instructions | No. |

Any synthesized text must make the boundary visible: QF-authored decision content is separate from Smackerel-authored context notes.

## Safety And Action Gating

Pre-MVP gating rules:

1. Packet cards are read-only.
2. Action controls for approval, mandate change, EmergencyStop, paper execution, and live execution are absent or disabled with phase-boundary copy.
3. The connector does not call QF action endpoints.
4. Reserved action objects are treated as diagnostics only unless QF officially exposes a supported action in a later release.
5. Smackerel recommendations and watches must not use QF packets as authorization to trade.

If a QF packet includes action-like content, Smackerel renders it as QF-authored text and links back to QF for any supported workflow.

### Reserved Schemas (Not Implemented Pre-MVP)

The QF bridge contract defines two reserved schemas that this connector MUST recognize but MUST NOT exercise pre-MVP. They exist for forward compatibility only and are mirrored from `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`.

| Reserved Schema | Pre-MVP Behavior In Smackerel |
|-----------------|-------------------------------|
| `QFApprovalAction` (action_id, packet_id, trace_id, action_type, actor_ref, reason) | The connector does not construct or send this object. The Web/Telegram/digest surfaces do not expose any control that would generate it. If diagnostic tooling encounters it (for example a hand-crafted test fixture), it is normalized into a `qf/approval-request` content-type artifact with `Metadata.reserved = true` and is excluded from search, digest, recommendation surfaces, and the evidence builder. |
| `QFWatchSignal` (watch_id, signal_id, packet_id, trace_id, signal_type, state) | The connector does not subscribe to, evaluate, or surface watch signals pre-MVP. Any inbound watch signal payload is logged as a diagnostic only and does not alter connector state, packet state, digest content, or Telegram delivery. |

These schemas MUST stay out of the active rendering, action, and delivery code paths until QF exposes a supported endpoint for them in a later release. Treating either schema as actionable pre-MVP is a contract violation.

## Degraded Packet Behavior

A packet is degraded when QF is reachable but the packet cannot safely render as a trusted decision artifact.

| Condition | Behavior |
|-----------|----------|
| Missing badge object | Mark connector degraded; do not render trusted packet card. |
| Unknown packet version | Store diagnostic metadata; do not render actionable card. |
| Trace ID missing or malformed | Mark packet invalid for companion rendering. |
| Deep link missing | Mark packet invalid for companion rendering. |
| Approval state missing | Mark packet invalid for companion rendering. |
| QF marks packet stale or downgraded | Render the QF state exactly; do not upgrade or hide it. |

Degraded diagnostics can appear in connector status surfaces for the operator, but not as financial advice or user-action prompts.

## Data And Storage Decisions

Smackerel stores QF packets through the existing artifact pipeline and PostgreSQL-backed sync state.

| Data | Storage Path |
|------|--------------|
| Connector cursor | Existing `sync_state` row for `source_id = qf-decisions`. |
| Raw QF envelope | Existing artifact raw content path via `RawArtifact.RawContent`. |
| QF metadata | Existing artifact metadata map with exact QF IDs and badge objects. |
| Search vectors | Existing processing pipeline, excluding any transformation that changes QF-authored decision fields. |
| Evidence export record | Existing artifact/export tracking pattern or a dedicated PostgreSQL table owned by implementation planning. |

No embedded database, direct QF database connection, or file-based persistence is introduced.

## Security And Privacy

| Area | Design |
|------|--------|
| Credentials | Stored through Smackerel config generation and runtime secret handling. No production secret literals. |
| Authorization | Connector credential scope is enforced by QF. Smackerel does not broaden scope locally. |
| Personal context | Evidence export requires explicit consent, sensitivity tier, source artifact IDs, and provenance. |
| Redaction | Bundle generation records what was omitted or summarized. |
| Audit | Sync, validation failure, degraded packet, evidence export, and QF import status are logged with packet/export IDs. |
| Revocation | Disabling connector stops new syncs; v2 tenant revocation must also clear access through cursors, digest queues, and evidence exports. |

## Operations

Operational signals:

| Signal | Meaning |
|--------|---------|
| Connector health | `healthy`, `syncing`, `degraded`, `failing`, `error`, or `disconnected`. |
| Cursor lag | Difference between latest QF event and `qf-decisions` stored cursor. |
| Packet validation failures | Count by missing ID, missing badge, missing trace, unknown version, or missing link. |
| Artifact publication failures | Failures returned by `ArtifactPublisher`. |
| Evidence export attempts | Count by accepted/rejected status and sensitivity tier. |
| Boundary violations | Any attempted approval/execution/action path visible in Smackerel pre-MVP. |

Operator procedures:

- Rotate QF credential through `config/smackerel.yaml` and config generation.
- Reset only the `qf-decisions` cursor when replaying QF packets.
- Disable the connector if Smackerel displays QF packets without required trust metadata or action controls appear.
- Use packet ID and trace ID for cross-repo incident correlation.

## Testing Strategy

| Test Type | Coverage |
|-----------|----------|
| Unit | Config validation, client request construction, cursor handling, normalizer required-field validation, badge preservation, degraded packet classification, evidence bundle serialization. |
| Integration | Sync against a QF-compatible test read surface, persist cursor through `StateStore`, publish `RawArtifact`s, preserve QF IDs and badge objects. |
| E2E API | Ingest a QF packet, retrieve it through recent/search/detail APIs, and export a consent-scoped evidence bundle to QF. |
| E2E UI | Web, Telegram, and digest surfaces show QF source, packet ID, trace ID, badges, approval state, deep link, and no execution controls. |
| Stress | Repeated sync pages and packet updates do not duplicate `packet_id`, lose cursor state, or emit action prompts. |
| Regression | Missing badges, unknown packet version, stale cursor replay, local synthesis rewrite attempt, and missing consent export all fail safely. |

Scenario-to-test mapping:

| Scenario | Test Type | Assertion |
|----------|-----------|-----------|
| BS-001 / TSC-SM-041-001 QF packet in search/digest/detail/Telegram | Integration + E2E API + E2E UI | Rendered items preserve QF packet ID, intent ID, scenario ID, trace ID, approval state, badges, source labels, and QF deep link. |
| BS-002 / TSC-SM-041-003 Evidence bundle export | Unit + Integration + E2E API + E2E UI | Bundle includes source IDs, claims, symbols/entities, sensitivity, confidence, consent scope, provenance, redaction summary, and QF import status. |
| BS-003 / TSC-SM-041-004 Action boundary | E2E UI + E2E API | Approval/execution/mandate/watch/EmergencyStop controls are absent or disabled, boundary notice is visible, and no QF action request is sent. |
| BS-004 / TSC-SM-041-002 Schema mismatch | Unit + Integration + E2E UI | Connector health becomes degraded/error, diagnostics show packet/version reason, and no trusted packet card, digest item, Telegram decision summary, or evidence-builder entry point renders. |

Adversarial cases:

- Packet missing `CalibrationBadge` or `DataProvenanceBadge`.
- Packet with valid thesis but wrong or missing `trace_id`.
- Packet update for an existing `packet_id` after cursor replay.
- Smackerel synthesis attempt that changes QF thesis or approval state.
- Evidence export without consent or source artifact IDs.

## Release Mapping

| Release | Smackerel Behavior |
|---------|--------------------|
| Pre-MVP | Implement `qf-decisions` as read-only connector, render packet cards, support evidence bundle export, disable all decision actions. |
| MVP | Harden connector auth and entitlements; allow only QF-official limited actions if QF exposes them. |
| v1.0 | Promote QF packets into delivery and watch workflows for paper/analysis scenarios while QF owns watch evaluation. |
| v2.0 | Add tenant-aware service credentials, revocation, retention, metering, and audit behavior. |
| v3.0 | Certify Smackerel as an external companion surface after QF parity gates for safety, traceability, and policy enforcement. |

## Alternatives Considered

| Alternative | Decision | Rationale |
|-------------|----------|-----------|
| Smackerel reads QF database directly | Rejected | Bypasses QF validation and authority boundary. |
| Kafka/NATS federation with QF | Rejected for pre-MVP | Adds cross-project runtime coupling before the contract is proven. |
| Treat QF packets as Smackerel recommendations | Rejected | Would blur financial-advice authority and trust provenance. |
| Prompt-generated QF badges | Rejected | Trust badges must be QF-owned and exact. |
| Raw personal data export | Rejected | Reverse flow must be a consent-scoped evidence bundle with references and redaction summary. |

## Explicit Non-Goals

- No financial advice generated by Smackerel.
- No trade approval, mandate change, paper execution, live execution, or EmergencyStop from Smackerel in pre-MVP.
- No direct database connection to QF.
- No broker federation with QF.
- No local rewrite, upgrade, downgrade, or hiding of QF trust metadata.
- No QF tenant/client companion behavior before QF v2 release gates.
- No certified external-surface parity claim before QF v3 release gates.

## Open Questions

1. The exact QF private-alpha endpoint names and schema version string once QF implementation chooses polling or an outbox-backed read projection.
2. Whether QF packet cards are introduced as a dedicated Web component or rendered through the existing artifact detail template with a source-qualified QF panel.
3. Whether digest ranking uses a dedicated QF section or source-qualified inclusion in existing finance/context sections.

## Reconciliation Notes (2026-05-03)

This design has been re-reviewed against `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` to verify cross-repo contract alignment. The following design-only reconciliations were applied in this pass; no spec, scope, report, uservalidation, or runtime code was modified.

| Drift Found | Resolution In This Design |
|-------------|---------------------------|
| `PersonalEvidenceBundle` Bundle Fields table did not list `target_context`, although QF's import validator and TSC-QF-063-003 reject the import if it is missing. The Bundle Builder UI section already referenced "Target QF packet/context" but the contract field was undeclared. | Added `target_context` (Required) to the Bundle Fields table, with the same allowed values as the QF bridge contract (`guided_analysis`, `rhai_run`, `saved_result`, `analysis_context`). |
| `source_refs` was not present in the Bundle Fields table even though QF's contract names it as an optional field. | Added `source_refs` (Optional) and clarified that `source_artifact_ids` is the canonical required reference set. |
| `qf/...` content type table did not bind QF's `decision_type=analysis_note`, leaving the connector's normalization path undefined for that decision type. | Added the `analysis_note` mapping: it normalizes to `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`, matching the QF design's "Decision Type To Smackerel Content Type Mapping" subsection. |
| Reserved schemas `QFApprovalAction` and `QFWatchSignal` were referenced indirectly in the safety/action gating section but were never named explicitly as reserved schemas with pre-MVP handling rules. | Added a dedicated "Reserved Schemas (Not Implemented Pre-MVP)" subsection under Safety And Action Gating that names both schemas, lists their fields, and pins their pre-MVP handling: never constructed, never sent, never surfaced as actionable, normalized to diagnostic artifacts only if encountered. |
| `qf/approval-request` content type description previously hinted it was a "reserved or diagnostic object" without clarifying that it ties to inbound `QFApprovalAction` and is not a `decision_type` value. | Reworded the row to make the binding explicit and to note that `QFApprovalAction` is not a decision packet. |

Cross-design checks confirmed (no change required):

- All trust metadata fields (`packet_id`, `intent_id`, `scenario_id`, `trace_id`, `approval_state`, `deep_link`, `CalibrationBadge`, `DataProvenanceBadge`, `packet_version`) are preserved exactly through the connector and are required for trusted rendering.
- `RawArtifact.SourceRef` continues to use the QF `packet_id` for stable identity; `event_id` is preserved in metadata for provenance.
- Cursor semantics: response-level QF `next_cursor` is what the connector persists; the per-event `cursor` field is treated as diagnostic-only metadata.
- Action boundary is enforced consistently with the QF design: approval, execution, mandate change, watch, and EmergencyStop are absent or disabled, and the connector never calls QF action endpoints.
- Schema/version mismatch handling continues to mark the connector degraded or error and to suppress trusted packet cards, digest items, Telegram decision summaries, and evidence-builder entry points.
- Evidence export validation requires consent scope, sensitivity tier, source artifact IDs, claims, provenance, redaction summary, and target context before any QF import call.
- Health states, observability signals, and authorization matrix remain consistent with the cross-repo bridge contract.

No runtime source, scopes.md, report.md, uservalidation.md, or certification fields were modified by this reconciliation pass.
