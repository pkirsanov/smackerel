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
- Required metadata includes QF packet ID, intent ID, scenario ID, trace ID, approval state, deep link, signed deep link (`packet_url_signed` + `signature_expires_at`), `CalibrationBadge`, and `DataProvenanceBadge`.
- Transport is the QF-internal PostgreSQL companion outbox projected behind the same `GET /api/private/smackerel/v1/decision-events` and `GET /api/private/smackerel/v1/decision-packets/{packet_id}` HTTP endpoints. Smackerel never reads QF PostgreSQL directly.
- Connector calls `GET /api/private/smackerel/v1/capabilities` on every `Connect()` and on credential rotation; capability mismatch refuses startup.
- Symmetric Smackerel-side metric set mirrors the QF-side metric set on shared label enums for cross-product dashboards (F11).
- Packet engagement signal exporter is active pre-MVP and consent-gated; signals are calibration-only and never influence local rendering, ranking, or trust metadata (O1, FR-013).
- Smackerel hosts a consent-gated `GET /api/private/qf/v1/personal-context` read endpoint for QF guided-analysis and Rhai workflows; responses carry a non-influence statement (O2, FR-014).
- Cross-product unified audit envelope is emitted for every bridge event (sync, validation, packet upsert, evidence import/revocation, engagement emit, capability handshake, callback signing, deep-link rendering decision, action-boundary diagnostic) (O4, FR-016).
- Callback signing infrastructure is implemented pre-MVP; callbacks are HMAC-signed but pre-MVP receivers reject with `CALLBACK_DEFERRED_TO_V1` so the signing path is exercised end-to-end (O5, FR-017).
- Source provenance classification is enumerated and validated pre-MVP, but bundle-side `DataProvenanceBadge`-shaped attachment is gated to v1.0 (O7, FR-018).
- Signed deep links are preferred over unsigned `packet_url` whenever the envelope carries `packet_url_signed` and the signature has not expired; Smackerel never re-signs links locally (O8, FR-019).
- Preferred surface hint is honored as render-priority only and never alters trust metadata, content, or action behavior (O9, FR-020).
- Web surfaces include QF connector status, QF packet search/digest card, QF packet artifact detail, and a PersonalEvidenceBundle builder.
- Telegram renders QF packets as compact read-only summaries with QF source, trust labels, trace, and deep link.
- Schema mismatch and missing-trust states produce diagnostics and degraded connector health rather than trusted packet cards.

### Open Questions

- Resolved 2026-05-07 — Transport is the PostgreSQL companion outbox projected behind the existing HTTP read endpoints; the outbox table is QF-internal. The connector continues to consume via HTTP polling.
- Whether digest ranking treats QF packets as a dedicated section or source-qualified items inside existing finance/context sections.
- Whether the first Web implementation lands in existing HTMX routes, PWA pages, or both surfaces at once.
- Engagement-signal consent UI lifecycle on the Smackerel side (when and how the user opts into surface-engagement reporting) — paired with QF OQ-04.
- Exact rendering placement for the consent affordance that mints personal-context read tokens for QF callers — paired with QF OQ-04.
- HMAC key rotation cadence and key-management ownership for both signed deep-link verification and callback signing — paired with QF OQ-06.

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

`Connect(ctx, cfg)` validates configuration, constructs the QF client, calls the capability handshake (see "Capability Discovery"), verifies the QF bridge health/schema endpoint if available, and transitions health from `disconnected` to `healthy` only after validation succeeds.

Required checks:

- `base_url` is syntactically valid and has no trailing slash in the client.
- Credential reference resolves through generated configuration or runtime environment.
- Requested packet contract version is non-empty.
- Page size is explicit, positive, and finite; the effective event-poll `limit` is derived only after the capability response has been fetched and persisted.
- Capability response is fetched, parsed, validated, and durably stored before `Connect()` returns success; supervised polling for `qf-decisions` starts only after this persisted capability is available.
- QF bridge responds with compatible schema metadata if the health/schema endpoint is present.

### Capability Discovery

`Connect()` MUST call `GET /api/private/smackerel/v1/capabilities` immediately after credential resolution and before the first decision-event poll. The connector MUST also re-fetch the capability response on every supervisor-initiated restart and on credential rotation start.

Required capability fields parsed and persisted:

| Field | Use |
|-------|-----|
| `supported_packet_versions` | Compared against the connector's compiled `packet_version`. Mismatch refuses startup. |
| `supported_event_types` | Canonical QF event-type allowlist for normalizer routing: `packet_created`, `packet_updated`, `packet_trust_changed`, `packet_archived`, `packet_action_boundary_attempted`. |
| `supported_decision_types` | Allowlist for content-type mapping. |
| `max_page_size` / `min_page_size` | Cursor pagination clamp `[min, max]`. |
| `supported_target_context_types` | Evidence builder limits `target_context_type` selection to this list. |
| `evidence_max_bundle_size_bytes` | Pre-flight bundle size guard for evidence export. |
| `evidence_max_claims_per_bundle` | Pre-flight claim count guard for evidence export. |
| `evidence_rate_limit_per_minute` | Token-bucket pacing for evidence exports. |
| `freshness_sla_p95_seconds` | Stress-test target for end-to-end render latency. |
| `audit_envelope_version` | Stamped on every audit-envelope record this connector emits. |
| `tenant_aware` | Pre-MVP MUST be `false`. |
| `preferred_surface_hint_supported` | Toggles render-routing logic for `preferred_surface`. |
| `engagement_signal_supported` | Gates the Packet Engagement Signal Exporter. |
| `personal_context_pull_supported` | Required before Smackerel advertises the personal-context read endpoint to QF. |
| `watch_signal_direction` | Pre-MVP MUST be `"qf_emit_only_pre_mvp"`. |
| `callback_signing_supported` | Pre-MVP MUST be `false`; signing infrastructure path is still exercised. |
| `deep_link_signing_supported` | When `true`, renderers MUST prefer `packet_url_signed` over the unsigned `packet_url`. |
| `no_action_emit_enabled` | Pre-MVP MUST be `false`; receiving a `no_action` packet while `false` is a contract violation. |
| `eligible_smackerel_source_classes` | Allowlist for `source_provenance_classes` entries on evidence bundles. |
| `credential_rotation_overlap_supported` | When `true`, two credentials may be active simultaneously for ≤24h. |

Behavior on incompatibility:

- Connector refuses to start; health transitions to `error` with reason `capability_mismatch`.
- `QFConnectorStatusPanel` surfaces the `incompatible` status and lists the exact mismatched fields.
- Emit `smackerel_qf_capability_mismatch_total{required, actual}` for each mismatched dimension.
- The supervisor does NOT retry the connect cycle until configuration or capability changes.

Re-check policy:

- Re-fetch on every supervisor-initiated restart and on credential rotation start.
- Persist the most recent capability response in connector state for diagnostic visibility on `/settings` and `/status`.
- Persisted capabilities also inform the operator-facing capability diff displayed in `QFConnectorStatusPanel`.
- Failure to persist the capability response is a failed handshake. The connector MUST NOT poll `decision-events` with an in-memory-only capability value.

### Event Type Vocabulary

The authoritative event vocabulary is the `QFDecisionEvent.event_type` enum in `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`. Smackerel's pre-MVP wire contract accepts only these exact QF values:

| QF `event_type` | Smackerel normalization behavior |
|-----------------|----------------------------------|
| `packet_created` | Fetch or consume the referenced packet envelope and create or upsert the source-qualified `RawArtifact` keyed by `packet_id`. |
| `packet_updated` | Re-fetch or consume the updated packet envelope and update the existing source-qualified packet view without changing the QF `packet_id`. |
| `packet_trust_changed` | Re-fetch or consume the packet envelope and refresh QF-owned trust metadata (`CalibrationBadge`, `DataProvenanceBadge`, approval state, signed link fields) without reconstructing trust locally. |
| `packet_archived` | Preserve the event as an archival diagnostic and remove the packet from trusted render queues only when QF supplies an explicit archived/tombstone state; never delete local provenance or cursor history based on the event type alone. |
| `packet_action_boundary_attempted` | Record a non-actionable diagnostic and audit-envelope entry; never create approval, execution, mandate, EmergencyStop, or watch controls. |

Short event aliases such as `created`, `updated`, or `badge_changed` are not part of the pre-MVP production wire contract. Local fixtures or historical state that use those names are stale and should be updated by the implementation/test owner rather than silently normalized. If QF later needs backwards-compatible aliases, QF must publish a new bridge contract version or capability-declared compatibility map; Smackerel may then preserve both `Metadata.event_type_original` and `Metadata.event_type_canonical`. Until that explicit versioned compatibility rule exists, unsupported `event_type` values produce connector diagnostics/degraded health, never advance trusted packet rendering, and must not be retried through an invented local mapping; cursor progression remains governed only by QF's response-level `next_cursor` after the diagnostic is persisted.

### Sync

`Sync(ctx, cursor)` receives the opaque QF cursor stored in `sync_state.sync_cursor` and returns QF artifacts plus the next QF cursor.

Sync steps:

1. Set health to `syncing`.
2. Call QF decision event list with cursor, packet version, and an explicit page size limit derived from the persisted capability response.
3. For each event, fetch the full packet envelope when the event list does not inline it.
4. Validate packet IDs, trace ID, badge objects, approval state, deep link, signed deep link, and signature expiry.
5. Normalize valid packet envelopes into `RawArtifact`s.
6. Preserve degraded diagnostics for invalid packets without producing actionable packet cards.
7. Return the QF `next_cursor` unmodified.
8. Restore health to `healthy` when no blocking validation or transport errors occurred.

The connector must not advance local approval state or produce action commands during sync.

### Page Size Handling (F9)

The connector sends `limit` for `GET /decision-events` only after the capability handshake has completed and the capability response has been durably persisted. The configured `connectors.qf-decisions.page_size` value is explicit configuration; missing, zero, negative, non-integer, or otherwise invalid values fail `Connect()` before any QF poll.

The effective request limit is computed by clamping the explicit configured `page_size` to the inclusive `[min_page_size, max_page_size]` range from the successfully persisted capability response. This clamp is a capability-bound calculation, not a hidden default. There is no fallback page size: if the capability response is missing, unavailable, unreadable, stale for the active credential, or not durably saved, the connector MUST NOT poll `decision-events`. If this happens during `Connect()`, startup fails loud with reason `capability_unavailable`; if it happens after a previously successful connect, the connector marks itself `degraded`, emits an operator-visible capability diagnostic, and waits for a successful re-handshake before polling again.

- A `PAGE_SIZE_OUT_OF_RANGE` 4xx response from QF MUST trigger a connector-level alert (`smackerel_qf_packet_validation_failures_total{reason="page_size_out_of_range"}`) and mark the connector `degraded`.
- The connector MUST NOT retry the same sync cycle with a guessed, smaller, or hardcoded limit after `PAGE_SIZE_OUT_OF_RANGE`. The operator must reconfigure the explicit `page_size` or wait for a refreshed capability response that makes the clamped value valid.
- `min_page_size`/`max_page_size` capability ranges that are missing, nonsensical, or impossible to apply fail the handshake with reason `capability_mismatch_page_size`; polling remains blocked until capability discovery succeeds.

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
- `packet_url_signed`
- `signature_expires_at`
- `CalibrationBadge`
- `DataProvenanceBadge`
- `packet_version`

Rejected packets are counted and surfaced through connector health. The connector may retain a diagnostic raw artifact only if it is clearly marked as non-actionable and excluded from recommendation surfaces.

### Trust Object Rendering

The digest, Telegram, search, and artifact-detail renderers consume only the public rendering surface defined by the QF design's Trust Object Rendering Contract. Smackerel MUST treat each trust object as opaque QF-owned metadata.

| Trust Object | Fields Smackerel MAY Render | Cardinality |
|--------------|-----------------------------|-------------|
| `CalibrationBadge` | `label`, `severity`, `summary`, optional `detail`, optional `links` | `label`/`severity`/`summary` required; `detail`/`links` optional |
| `DataProvenanceBadge` | `label`, `severity`, `summary`, optional `detail`, optional `links` | same |
| `QuantifiedImpact` | `label`, `severity`, `summary`, optional `detail`, optional `links` | same |
| `ExpertAnalysisBundle` | `label`, `severity`, `summary`, optional `detail`, optional `links` | same |

Rendering rules:

- Numeric internals, raw model coefficients, calibration distributions, intermediate computation fields, and any unenumerated field MUST be silently dropped (NOT errors). Dropping unknown internals keeps the renderer forward-compatible without leaking QF internals.
- `label` and `severity` are mandatory on every trust object. Missing either field on `CalibrationBadge` or `DataProvenanceBadge` fails packet rendering and emits `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}`. The packet is then routed through the degraded path.
- `severity` drives icon and `aria-label` only. Color is never the only signal.
- A `degraded`, `warning`, or `error` severity MUST remain visible. Smackerel MUST NOT upgrade severity, hide downgrade wording, or substitute alternate text.
- Optional `links` are rendered as text-visible affordances pointing back to the QF authoritative drilldown view.
- `QuantifiedImpact` and `ExpertAnalysisBundle` follow the same surface; their numeric internals (units, horizon vectors, model parameters) are NEVER reconstructed locally.

### Forward-Compatible decision_type Handling (F8)

When QF emits an unknown `decision_type` value, the QF envelope MUST carry a metadata flag `unknown_decision_type=true`. Smackerel behavior:

- Still ingest the event as a regular packet so the cursor advances cleanly.
- Set `Metadata.unknown_decision_type = true` on the resulting `RawArtifact`.
- Route rendering through a generic packet card variant; never invent a content type for the unknown value.
- Emit `smackerel_qf_unknown_decision_type_total{value}` for monitoring.
- Surface the packet on the connector diagnostics surface so operators can confirm the schema drift was intentional.
- NEVER reject a packet for unknown `decision_type` alone; trust metadata validation still applies.
- NEVER attempt to derive semantics for the unknown value from the packet body.
- Trust metadata, deep links, and badges still render under the rules above.

### no_action Decision Semantics (F8 / F17)

`no_action` is capability-gated by `no_action_emit_enabled` on the capability response.

| Capability State | Connector Behavior |
|------------------|--------------------|
| `no_action_emit_enabled = true` | A received `no_action` packet renders via the existing `qf/no-action-decision` content type with the standard read-only card emphasizing QF-authored no-action rationale. |
| `no_action_emit_enabled = false` | Receiving a `no_action` packet is a contract violation. Smackerel MUST drop the packet, increment `smackerel_qf_packet_validation_failures_total{reason="unexpected_no_action"}`, classify the diagnostic as `unknown_decision_type=true`, and surface an operator alert through `QFConnectorStatusPanel`. |

This protects the read pipeline against premature no-action rollouts and keeps Smackerel aligned with QF's pre-MVP gate.

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

### Signed Deep-Link Rendering (FR-019, O8)

Every QF deep link rendered in any Smackerel surface (Web, digest, Telegram, search, artifact detail) MUST follow this preference order:

1. If the envelope provides `packet_url_signed` AND `signature_expires_at` is in the future AND capability `deep_link_signing_supported = true`: render the signed URL exactly as supplied. Emit `smackerel_qf_deep_link_render_total{surface, status="signed_used"}`.
2. If `packet_url_signed` is present but `signature_expires_at` has passed: trigger a synchronous re-fetch of the packet via `GET /api/private/smackerel/v1/decision-packets/{packet_id}` to obtain a refreshed signature, then render the new signed URL. If the refresh fails, fall back to the unsigned `packet_url` and emit `smackerel_qf_deep_link_render_total{surface, status="signed_expired_fallback_unsigned"}`.
3. If capability `deep_link_signing_supported = false`: render the unsigned `packet_url`. Emit `smackerel_qf_deep_link_render_total{surface, status="unsigned_only"}`.

Hard rules:

- Smackerel MUST NOT downgrade a signed link to unsigned form on any rendering surface when both forms are available and the signed form is unexpired.
- Smackerel MUST NOT re-sign or re-issue links locally. Only QF mints signed companion deep links; the bridge HMAC secret lives on the QF side.
- An unsigned link rendered while capability advertises signing support MUST trigger an operator-visible diagnostic note in `QFConnectorStatusPanel` (never user-visible).
- Telegram messages, digest entries, web cards, and artifact detail panels all use the same precedence order.
- The signed URL MUST be rendered exactly; query parameters, fragment, and HMAC `sig` are opaque to Smackerel.
- Signed deep-link rendering decisions emit a unified audit envelope entry (`action=deep_link_render`, `outcome=ok|degraded`).

### Preferred Surface Routing (FR-020, O9)

The optional `preferred_surface` enum on `QFDecisionPacketEnvelope` is a render-priority hint only. Smackerel MUST honor it without altering trust metadata, packet content, badge severity, action affordances, or action-boundary copy.

| `preferred_surface` | Behavior |
|---------------------|----------|
| `qf_dashboard` | Show in search and artifact detail. Suppress from digest unless the user explicitly opens the packet from search. The digest tile entry, if shown, surfaces a "View in QF dashboard" affordance pointing at the (signed) deep link. |
| `smackerel_digest` | Include in the next eligible digest cycle. Default routing for `decision_type=analysis_note`. |
| `smackerel_telegram` | Prioritize for the next Telegram delivery window. (Telegram action affordances remain absent pre-MVP per FR-009.) |
| `any` or missing | Apply existing default behavior using the user's per-surface preferences and the default mapping below. |

Default mapping when `preferred_surface` is missing:

| `decision_type` | Default Surface |
|-----------------|-----------------|
| `analysis_note` | `smackerel_digest` |
| `recommendation` | `qf_dashboard` |
| `policy_denial` | `qf_dashboard` |
| `no_action` | `any` |

Hard rules:

- Routing decisions MUST NOT change content, badges, severity, deep-link rendering, or action affordances.
- A user's existing quiet/sensitivity policy still applies even when a surface is hinted; quiet policy wins over `preferred_surface`.
- A surface that has been disabled by the user (e.g. Telegram off) falls back to the next non-disabled surface in priority order, and the fallback is recorded as a unified audit envelope entry (`action=preferred_surface_fallback`, `outcome=ok`).
- When `preferred_surface_hint_supported = false` from the capability response, the connector ignores the hint entirely and uses the existing default behavior.

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
| `source_provenance_classes` | Optional. Per-source provenance classification (FR-018, O7). Each entry is `{source_artifact_id, source_provenance_class}` where `source_provenance_class` is one of `smackerel_markets`, `smackerel_weather`, `smackerel_news`, `smackerel_geopolitical`, `smackerel_other`, `external`. Pre-MVP design only — the field shape is emitted but bundle-side `DataProvenanceBadge`-shaped attachment is gated to v1.0. |
| `related_symbols` | Optional. Symbol list extracted from selected context. |
| `related_entities` | Optional. Entities extracted from selected context. |
| `extracted_claims` | Required. Claims tied to source artifact IDs and confidence. |
| `confidence` | Bundle-level confidence derived from source coverage and extraction quality. |
| `sensitivity_tier` | Required. Explicit sensitivity classification (`personal`, `sensitive`, `restricted`). |
| `consent_scope` | Required. Explicit purpose, target QF context, and revocation/expiry reference. |
| `provenance` | Required. Smackerel generation lineage and source-processing references. |
| `redaction_summary` | Required. What raw personal material was summarized or omitted. |
| `target_context` | Required. QF analysis/run/Rhai/guided-analysis attachment target identifier. Pre-MVP `target_context_type` values are `guided_analysis`, `rhai_run`, `saved_result`, `analysis_context`, or `packet_context` per the QF bridge contract. When `target_context_type=packet_context`, `target_context_ref` MUST be a `packet_id` visible to the connector credential. Must be selected before validation can pass. |
| `created_at` | Required. RFC3339 timestamp. |

This required-field set MUST stay aligned with the QF acceptance criteria in `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` so that a Smackerel-generated bundle that passes local validation also passes QF import validation.

### Source Provenance Classes (FR-018, O7)

Each entry in the optional `source_provenance_classes` array carries `{source_artifact_id, source_provenance_class}`. The class enum is:

- `smackerel_markets`
- `smackerel_weather`
- `smackerel_news`
- `smackerel_geopolitical`
- `smackerel_other`
- `external`

Pre-flight validation rules:

- Every `source_artifact_id` referenced in `source_provenance_classes` MUST also appear in the top-level `source_artifact_ids`. Mismatch is a local rejection with reason `EVIDENCE_PROVENANCE_REF_NOT_IN_BUNDLE`.
- Each declared `source_provenance_class` MUST appear in the capability response's `eligible_smackerel_source_classes` allowlist. Unknown classes are rejected locally with reason `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{class}`.
- `source_provenance_classes` is optional pre-MVP; bundles without it pass validation.

Pre-MVP scope:

- The connector emits the field shape so downstream evidence stores can mirror it, but Smackerel MUST NOT attach a `DataProvenanceBadge`-shaped block to selected source references on the bundle in pre-MVP. Badge-attachment activation is gated to v1.0 per the spec Release Mapping (FR-018) and the paired QF acceptance gate.
- Local validation still runs even though no badge attachment occurs, so the rule and connector-class enumeration are exercised end-to-end in evidence-export tests.
- The connector emits `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason="EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE"}` for class violations.

### Evidence Import Limits (F15)

The connector enforces capability-declared limits as a pre-flight check before any HTTP request to QF. Local rejection avoids round-trips for known-bad bundles and protects QF from accidental floods.

| Limit | Capability Field | Default (Pre-MVP) | Local Reject Reason Code |
|-------|------------------|-------------------|---------------------------|
| Bundle JSON serialized size | `evidence_max_bundle_size_bytes` | 524288 | `BUNDLE_TOO_LARGE` |
| Maximum claims in `extracted_claims` | `evidence_max_claims_per_bundle` | 50 | `TOO_MANY_CLAIMS` |
| Per-credential rate | `evidence_rate_limit_per_minute` | 10 / minute | `RATE_LIMIT_EXCEEDED` |

Implementation rules:

- Token-bucket rate limiter is keyed by connector credential ID; refill is one-minute fixed window.
- Bundle size is measured against the canonical serialized JSON about to be sent (not the pre-serialization Go struct).
- Local rejections emit `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason}` and a unified audit envelope entry (`action=evidence_import`, `outcome=rejected`, `reason` set to one of the local reason codes).
- The connector NEVER attempts to silently truncate, drop claims, or re-pack a bundle to fit limits. The originating workflow surfaces the limit violation to the user.
- If QF advertises tighter limits via capability response than the connector's defaults, the capability values win.
- Limits also apply when retrying after a 5xx; the limiter does not exempt retries.

### Export Flow

```text
User selects Smackerel context
    -> export dialog shows target QF context and sensitivity
    -> user grants explicit consent scope
    -> Smackerel builds PersonalEvidenceBundle
    -> Smackerel validates required source, provenance, size, and rate-limit fields locally
    -> qf-decisions client posts bundle to QF import endpoint
    -> QF returns accepted, rejected, or conflict status (see Idempotency Response Handling)
    -> Smackerel records export status and QF import reference
    -> Audit envelope entry recorded for every attempt
```

Smackerel must not export a raw personal data dump. The bundle is a compact evidence object with source references and claims.

### Idempotency Response Handling (F4)

`POST /api/private/smackerel/v1/personal-evidence-bundles` is idempotent on `export_id`. The connector handles the QF responses as follows:

| QF Response | Connector Behavior |
|-------------|--------------------|
| HTTP 201 with new attachment record | First successful import. Persist the export record with `status=ACCEPTED` and emit `smackerel_qf_evidence_export_attempts_total{status="accepted"}`. |
| HTTP 200 with the same `export_id` and matching attachment record | Treat as idempotent no-op success. Update the local export record's `last_observed_at` but do NOT emit a duplicate audit envelope entry. |
| HTTP 409 with `reason=EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD` | Hard error. Mark the local export record `EXPORT_ID_COLLISION`, emit `smackerel_qf_evidence_export_attempts_total{status="export_id_collision"}`, surface the conflict to the originating workflow, and abort the export. NEVER retry. |
| HTTP 409 with `reason=EXPORT_ID_PREVIOUSLY_REJECTED` | Mark the local export record `EXPORT_ID_PREVIOUSLY_REJECTED`. The user must mint a new bundle (new `export_id`) to retry. NEVER reuse the same `export_id`. |
| HTTP 4xx other than 409 | Treat as terminal local rejection per the relevant reason code. Do NOT retry. |
| HTTP 5xx | Retry with exponential backoff up to a bounded retry budget; after exhaustion mark `TRANSPORT_FAILED`. |

Connector retry policy: 200, 201, 4xx, and 409 responses are final. Only 5xx and transport timeouts are retryable. Audit envelopes record both retry attempts and final outcomes.

### Consent Revocation Path (F14, FR-014)

When a user revokes evidence-export consent through Smackerel UI or API:

1. Smackerel resolves the affected `export_id`(s) tied to the revoked consent scope.
2. For each `export_id`, the connector calls `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with body `{"reason": "consent_revoked"}` (or another caller-supplied reason such as `user_request` or `retention_policy`).
3. On HTTP 200/204 the local export record is marked `revoked` and the bundle is hidden from any local context-attached views.
4. On HTTP 404 (`EVIDENCE_EXPORT_ID_NOT_FOUND`) the local record is reconciled to `revoked_remote_missing` and an audit-envelope entry is recorded (`action=evidence_revoke`, `outcome=ok`, `reason=remote_missing`). No retry.
5. On HTTP 409 (`EVIDENCE_EXPORT_ID_ALREADY_REVOKED`) the local record is reconciled to `revoked` and a single audit entry is recorded.
6. On 5xx or transport failure the connector retries with exponential backoff and surfaces the pending revocation in connector status.

Audit and metrics:

- Every revocation attempt records a unified audit envelope (`action=evidence_revoke`, `outcome=ok|rejected`, `reason` populated on rejection or with `consent_revoked`).
- Emit `smackerel_qf_evidence_revoked_total{reason}`.
- The local evidence-export record retains the revocation reason and timestamp indefinitely so the user can verify revocation completion.
- After successful deletion, the same `export_id` MAY be re-used by a subsequent bundle export — QF's F4 idempotency check considers a deleted record absent.

## Packet Engagement Signal Exporter (FR-013, O1)

Smackerel emits `PacketEngagementSignal` events to QF whenever a user opens, dwells on, dismisses, snoozes, deep-links, or shares a QF packet rendered in any Smackerel surface. The exporter is consent-gated and never influences local rendering, ranking, digest priority, recommendation surfaces, or local trust metadata.

### Source Surfaces

| Surface | Captured Events | Notes |
|---------|-----------------|-------|
| Web (artifact detail, search card) | `opened`, `dwell`, `dismissed`, `snoozed`, `deep_linked`, `shared` | Dwell tracked while the artifact detail view is visible/focused. |
| Daily digest | `opened`, `dwell`, `dismissed`, `deep_linked`, `shared` | `opened` fires on tile expand or link click; `dwell` on visible-section duration. |
| Telegram bot | `opened`, `deep_linked`, `dismissed`, `snoozed` | `opened` corresponds to message-tap callbacks (signed callback envelope path); `dwell` is not captured on Telegram. |

### Signal Envelope (Smackerel-Side Construction)

| Field | Construction |
|-------|--------------|
| `signal_id` | UUIDv7 generated client-side at event capture; idempotency key for QF ingestion. |
| `packet_id` | Foreign key to a `QFDecisionPacketEnvelope` previously ingested by the connector. |
| `trace_id` | Copied verbatim from the referenced packet envelope; mismatch is a contract bug. |
| `engagement_event` | Enum: `opened`, `dwell`, `dismissed`, `snoozed`, `deep_linked`, `shared`. |
| `engagement_ts` | RFC3339 timestamp at the moment the user-visible event fired. |
| `surface` | Enum: `web`, `digest`, `telegram`. |
| `consent_scope` | Enum: `engagement_telemetry_anonymous` or `engagement_telemetry_pseudonymous`, derived from the user's privacy settings at event time. |
| `actor_ref` | Opaque, stable Smackerel-issued user token. NEVER plaintext PII (no email, no display name). |
| `dwell_seconds` | Integer; REQUIRED when `engagement_event=dwell`; FORBIDDEN otherwise. |

### Consent Gate

- Emission is permitted ONLY when the user has set Smackerel's `engagement_telemetry` privacy preference to `anonymous` or `pseudonymous`.
- A user with `engagement_telemetry=off` produces zero signals; the buffer is bypassed entirely at capture time.
- Consent state is read at event-capture time (not flush time) so the signal reflects the consent in effect when the user acted.
- Emission MUST NOT include packet content, evidence content, or any PII beyond `actor_ref`.

### Buffer And Flush Policy

- In-memory ring buffer per Smackerel process.
- Flush trigger: every 10 seconds OR on 100 buffered events, whichever fires first.
- Buffer overflow: drop the oldest signal and emit `smackerel_qf_engagement_signal_attempts_total{event="overflow_drop"}`.
- Buffer is NOT persisted across process restarts; signal loss on crash is acceptable for calibration-grade telemetry.
- Endpoint: `POST /api/private/smackerel/v1/packet-engagement-signals` per QF design 063.

### Failure Handling

| QF Response | Connector Behavior |
|-------------|--------------------|
| HTTP 201 `{signal_id, received_at}` | Mark signal accepted; emit `smackerel_qf_engagement_signal_attempts_total{event, surface, status="accepted"}`. |
| HTTP 200 (idempotent repeat with same `signal_id` + identical body) | No-op; do NOT emit a duplicate audit envelope entry. |
| HTTP 409 `ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD` | Hard error; log and drop. NEVER retry the same `signal_id`. |
| HTTP 4xx (`ENGAGEMENT_PACKET_NOT_FOUND`, `ENGAGEMENT_TRACE_ID_MISMATCH`, `ENGAGEMENT_CONSENT_REQUIRED`, `ENGAGEMENT_DWELL_FIELD_MISMATCH`) | Log + drop (privacy-preserving — never retry rejected signals). Emit `smackerel_qf_engagement_signal_attempts_total{event, surface, status="rejected", reason}`. |
| HTTP 5xx | Retry with exponential backoff up to 3 attempts then drop. |
| Transport timeout | Same as 5xx (bounded retry then drop). |

Engagement-signal emission is fire-and-forget from the user-experience perspective: failures MUST NOT block UX, retry indefinitely, or surface user-visible errors.

### Audit And Non-Influence

- Every flush attempt (success, retry, rejection, overflow drop) records a unified audit envelope entry (`action=engagement_signal`, `outcome=ok|rejected|degraded`, `reason` populated on rejection).
- Engagement signals MUST NOT be read back into Smackerel's local rendering, ranking, digest priority, recommendation surfaces, or local trust metadata. The exporter is write-only.
- The capability flag `engagement_signal_supported` from the QF capability response gates the entire exporter; if `false`, the buffer is disabled and no signals are emitted.

## Personal Context Read API (FR-014, O2)

Smackerel hosts a consent-gated read endpoint that QF guided-analysis and Rhai workflows MAY call to obtain personal-context summaries for an entity. The endpoint is hosted on the Smackerel side; QF acts as the consumer.

### Endpoint Contract

```
GET /api/private/qf/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}
```

Query parameters:

| Parameter | Type | Required | Rule |
|-----------|------|----------|------|
| `entity` | string | yes | Entity reference (ticker, theme, actor, person, org, or topic). |
| `max_sensitivity` | string enum | yes | `personal`, `sensitive`, or `restricted`. The endpoint enforces this as a ceiling; items above the ceiling are redacted. |
| `consent_token` | string | yes | Smackerel-issued short-lived token (TTL ≤ 15 minutes) scoped to (`entity`, `max_sensitivity`, `requester_id`). |

### Response Shape

```
{
  "entity": "<echoed reference>",
  "items": [
    {
      "source_artifact_id": "<smackerel artifact ID>",
      "summary": "<short summary, NOT raw content>",
      "claims": [...],
      "provenance": {...},
      "sensitivity_tier": "personal|sensitive|restricted"
    }
  ],
  "non_influence_warning": "Personal-context responses are analysis-context only. They MUST NOT influence packet generation, trust badges, approval state, mandate state, execution eligibility, or any QF action endpoint."
}
```

Behavior:

- Validate the `consent_token` (signature, expiry, scope match). Invalid token returns HTTP 401 with `reason=PERSONAL_CONTEXT_TOKEN_INVALID`.
- Enforce the `max_sensitivity` ceiling: items above the ceiling are filtered out before response assembly.
- Return an EMPTY `items` array (HTTP 200) rather than an error when no consent or no matches exist; QF treats empty results as "no eligible context", not an error.
- The endpoint has NO write side effects on either side.
- The endpoint MUST NOT grant action authority to QF; the `non_influence_warning` field is required on every response.

### Consent Token Lifecycle

- Issued by Smackerel UI on explicit user action (the user authorizes a personal-context read for the requesting QF surface).
- Token TTL: ≤15 minutes. Token MUST NOT be persisted across browser sessions.
- Per-token rate limit: 5 reads per token. Exceeding returns HTTP 429 with `reason=PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED`.
- Token scope is baked in: token bound to `(entity, max_sensitivity, requester_id)`. Re-using a token for a different entity or higher sensitivity tier returns HTTP 401 `PERSONAL_CONTEXT_SCOPE_MISMATCH`.

### Audit And Non-Influence

- Every fetch (accepted, redacted, empty, rejected) records a unified audit envelope entry on the Smackerel side (`action=personal_context_read`, `outcome=ok|rejected|degraded`, `reason` populated on rejection or redaction).
- Smackerel emits `smackerel_qf_personal_context_reads_total{outcome, sensitivity_tier}`.
- Personal-context reads MUST NOT influence Smackerel's local rendering, ranking, or trust metadata.
- The endpoint is gated by capability response `personal_context_pull_supported=true`; if Smackerel's build does not support the endpoint, it MUST advertise `personal_context_pull_supported=false` and return HTTP 404.

## Cross-Product Audit Envelope (FR-016, O4)

Smackerel emits the unified audit envelope (defined in `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` → "Cross-Product Audit Envelope") for every cross-product bridge event. Both products write the same shape into their own audit stores so cross-product incident investigation produces a single greppable timeline.

### Envelope Shape (Mirrored From QF Design)

```
{
  "trace_id": "<required>",
  "packet_id": "<optional>",
  "export_id": "<optional>",
  "signal_id": "<optional>",
  "actor_ref": "<required, opaque>",
  "surface": "<required, enum: smackerel_web|smackerel_telegram|smackerel_digest|smackerel_connector>",
  "action": "<required, enum>",
  "outcome": "<required, enum: ok|rejected|degraded|conflict>",
  "reason": "<optional, string code>",
  "ts": "<required, RFC3339>",
  "envelope_version": "v1"
}
```

### Smackerel Coverage Rules

Smackerel MUST emit one envelope record for each of:

| Event | Action Value |
|-------|--------------|
| Packet ingest (after normalization) | `packet_ingest` |
| Packet validation failure (degraded path) | `packet_validation_failed` |
| Capability handshake (success or mismatch) | `capability_handshake` |
| Cursor fast-forward applied (after operator action on QF side) | `cursor_fast_forward` |
| Evidence export attempt (accept, reject, conflict, local reject) | `evidence_import` |
| Evidence revocation attempt | `evidence_revoke` |
| Engagement signal flush attempt | `engagement_signal` |
| Callback signing/send attempt (pre-MVP rejected) | `callback` |
| Signed deep-link rendering decision | `deep_link_render` |
| Action-boundary attempt diagnostic (e.g. inbound `QFApprovalAction` fixture) | `action_boundary_attempt` |
| Preferred-surface fallback decision | `preferred_surface_fallback` |
| Watch-proposal signing-path exercise (pre-MVP rejected) | `watch_proposal` |
| Personal-context read served | `personal_context_read` |

### Sink

- Connector-side audit log is the primary sink (PostgreSQL audit table or structured log shipper, per Smackerel runtime conventions).
- Opt-in mirror to QF audit endpoint is reserved for post-MVP; pre-MVP each side keeps its own envelope store.
- Envelopes MUST NOT contain credential material, raw evidence content, or plaintext personal data. `actor_ref` is opaque; `reason` is a code (e.g. `WATCH_PROPOSALS_DEFERRED_TO_V1`), never free-text user input.
- Envelope retention horizon ≥ 30 days post-cursor, aligned with QF outbox retention so incident investigation has full coverage.

## Callback Signing (FR-017, O5)

Smackerel implements the HMAC-SHA256 signing infrastructure pre-MVP so the signing path is exercised end-to-end. Pre-MVP receivers (QF) reject all incoming callbacks with `CALLBACK_DEFERRED_TO_V1`, but the signing infrastructure is real, tested, and ready for v1.0+ enablement.

### Signed Callback Payload

| Field | Rule |
|-------|------|
| `callback_id` | UUIDv7 generated by Smackerel at the moment a Telegram or Web actionable element renders. Idempotency key. |
| `trace_id` | Copied from the originating packet envelope. |
| `packet_id` | Packet the callback is acting on. |
| `action` | Pre-MVP enum: `noop`, `open` only. v1.0+ extends to `delivery_ack`, `no_action_capture`. v3.0 extends to `emergency_stop`. |
| `nonce` | Per-callback random nonce; included in the signed payload to prevent replay. |
| `expires_at` | RFC3339 expiry; pre-MVP TTL is 5 minutes from issuance. |
| `signature` | HMAC-SHA256 over canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` (UTF-8, pipe-separated). Signature output is **lower-case hex** (`hex.EncodeToString(HMAC-SHA256(...))`). |
| `surface` | Enum: `telegram` or `web`. |

> **Encoding reconciliation 2026-05-23:** Lower-case hex is the canonical wire
> format for the `signature` field. Earlier drafts of this section described
> the output as base64url-encoded; the Scope 8 implementation (see
> `internal/connector/qfdecisions/callback.go`'s `hex.EncodeToString(mac.Sum(nil))`)
> and the adversarial unit test
> `TestCallbackHMACSHA256SignatureIsLowerCaseHexAndMatchesKnownVector` codify
> lower-case hex as the chosen encoding. The two encodings are
> cryptographically equivalent; hex was selected for HMAC bridge signature
> idiom alignment, adversarial unit-test assertability against a known HMAC
> vector, and parity with `scopes.md` Scope 8 Implementation Plan + DoD
> wording. Receivers MUST validate the signature as lower-case hex; any other
> encoding is `CALLBACK_SIGNATURE_INVALID`.

### Key Management

- Bridge HMAC secret is rotated per release; pre-MVP key lifetime is the release window (no automated rotation pre-MVP).
- `key_id` field accompanies the signature so QF can identify which key signed it during rotation overlap.
- Smackerel MUST NOT introduce a second callback signing scheme; the same signing path is reused for any future callback class.

### Pre-MVP Behavior

- Every Telegram message tap or Web actionable element renders a signed callback payload.
- The connector POSTs the signed envelope to `POST /api/private/smackerel/v1/callback`.
- QF validates the signature, nonce, and expiry first; invalid signature returns `CALLBACK_SIGNATURE_INVALID` (HTTP 401), expired returns `CALLBACK_SIGNATURE_EXPIRED` (HTTP 401).
- Regardless of signature outcome, QF rejects the action with `CALLBACK_DEFERRED_TO_V1` (HTTP 503) pre-MVP.
- Smackerel MUST treat the rejection as expected, log it as a diagnostic, and NOT retry.

### Telemetry

- `smackerel_qf_callback_attempts_total{action, status="deferred"}` for every pre-MVP rejected callback.
- `smackerel_qf_callback_signature_failures_total{reason}` for any signing/verification failure observed locally (e.g. clock skew producing expired-at-issuance signatures).
- Every callback attempt records a unified audit envelope entry (`action=callback`, `outcome=rejected`, `reason=CALLBACK_DEFERRED_TO_V1` or signing reason).

### v1.0+ Path

- v1.0 enables `delivery_ack` and `no_action_capture` actions: the same signing/verification path is reused; only QF's response changes (accept rather than reject).
- v3.0 reuses the same protocol for `emergency_stop` attestation under a stricter authorization gate.
- The pre-MVP exercise of the signing path means v1.0 enablement is a capability-flag flip plus QF-side action handlers, NOT a new signing rollout.

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

The QF bridge contract defines reserved schemas that this connector MUST recognize but MUST NOT exercise pre-MVP. They exist for forward compatibility and signing-infrastructure exercise only and are mirrored from `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`.

| Reserved Schema | Pre-MVP Behavior In Smackerel |
|-----------------|-------------------------------|
| `QFApprovalAction` (action_id, packet_id, trace_id, action_type, actor_ref, reason) | The connector does not construct or send this object. The Web/Telegram/digest surfaces do not expose any control that would generate it. If diagnostic tooling encounters it (for example a hand-crafted test fixture), it is normalized into a `qf/approval-request` content-type artifact with `Metadata.reserved = true` and is excluded from search, digest, recommendation surfaces, and the evidence builder. |
| `QFWatchSignal` (watch_id, signal_id, packet_id, trace_id, signal_type, state, source) | Bidirectional schema with QF as the final authority for watch creation, evaluation, and `AttentionBudget` enforcement (FR-015, O3). Pre-MVP, Smackerel does not subscribe to, evaluate, or surface watch signals. The connector MUST NOT construct or send watch proposals (`source=smackerel_propose`); any user-visible "propose paper-mandate watch" affordance is absent pre-MVP. If diagnostic tooling crafts a `smackerel_propose` payload, the connector MAY exercise the `POST /api/private/smackerel/v1/watch-signal-proposals` signing path against QF, MUST log the QF rejection (`WATCH_PROPOSALS_DEFERRED_TO_V1`) as a diagnostic, and MUST NOT alter local state. v1.0 implementation path: surface a "propose paper-mandate watch" affordance grounded in observed local context patterns and emit each proposal as a `qf/watch-signal` with `source=smackerel_propose`; QF retains final accept/reject authority. |
| `SmackerelCallbackEnvelope` (callback_id, trace_id, packet_id, action, nonce, expires_at, signature, surface) | Reserved callback schema (FR-017, O5). Pre-MVP, Smackerel implements the HMAC-SHA256 signing path end-to-end so the signing infrastructure is exercised: every Telegram and Web actionable element renders as a signed callback payload, the connector POSTs to `POST /api/private/smackerel/v1/callback`, and QF returns `CALLBACK_DEFERRED_TO_V1`. The connector MUST treat the rejection as expected, NOT retry, and emit `smackerel_qf_callback_attempts_total{action,status="deferred"}`. Allowed `action` values pre-MVP are `noop` and `open` only; v1.0 enables `delivery_ack` and `no_action_capture`; v3.0 reuses the same protocol for `emergency_stop` attestation. The connector MUST NOT introduce a second callback signing scheme. |

These schemas MUST stay out of the active rendering, action, and delivery code paths until QF exposes a supported endpoint for them in a later release. Treating any of these schemas as actionable pre-MVP is a contract violation. The signing infrastructure for `QFWatchSignal` proposals and `SmackerelCallbackEnvelope` is exercised pre-MVP precisely so v1.0+ enablement is a one-line capability flip rather than a new signing rollout.

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
| Credential rotation overlap (F16) | The connector accepts overlapping credentials within an operator-controlled window of ≤ 24 hours. Configuration may declare both `current` and `next` credentials with `not_before` timestamps; for each outbound request the connector picks the credential with the latest `not_before` whose `not_before <= now`. Cursor state, idempotency keys (`export_id`, `signal_id`), and capability handshake state survive rotation unchanged. Rotation events emit a unified audit envelope entry (`action=credential_rotation`, `outcome=ok`). The connector emits `smackerel_qf_capability_handshake_total{outcome="ok"}` immediately after rotation to confirm the new credential negotiates a compatible capability set; mismatch fails closed and the connector reverts to the prior credential within the overlap window. |
| Authorization | Connector credential scope is enforced by QF. Smackerel does not broaden scope locally. |
| Personal context | Evidence export requires explicit consent, sensitivity tier, source artifact IDs, and provenance. The personal-context read endpoint (`/api/private/qf/v1/personal-context`) is gated by short-lived consent tokens and a `non_influence_warning` response field. |
| Redaction | Bundle generation records what was omitted or summarized. |
| Audit | Sync, validation failure, degraded packet, evidence export, evidence revocation, engagement signal flush, callback signing, deep-link rendering, capability handshake, cursor lag breach, fast-forward applied, and personal-context read are all logged with the unified Cross-Product Audit Envelope shape and include packet/export/signal IDs. |
| Callback signing infrastructure | HMAC-SHA256 signing exercised end-to-end pre-MVP; QF rejects with `CALLBACK_DEFERRED_TO_V1` until v1.0. Key rotation per release with `key_id`. |
| Revocation | Disabling connector stops new syncs; in-flight evidence exports may still be revoked via `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` even with the connector otherwise disabled. v2 tenant revocation must also clear access through cursors, digest queues, evidence exports, engagement signal buffers, and personal-context tokens. |

## Operations

Operational signals:

| Signal | Meaning |
|--------|---------|
| Connector health | `healthy`, `syncing`, `degraded`, `degraded_recovered`, `failing`, `error`, or `disconnected`. |
| Cursor lag | Difference between latest QF event and `qf-decisions` stored cursor. |
| Packet validation failures | Count by missing ID, missing badge, missing trace, unknown version, or missing link. |
| Artifact publication failures | Failures returned by `ArtifactPublisher`. |
| Evidence export attempts | Count by accepted/rejected status and sensitivity tier. |
| Boundary violations | Any attempted approval/execution/action path visible in Smackerel pre-MVP. |

### Symmetric Metric Set (Mirrors QF `qf_smackerel_*` Series)

Smackerel emits the following metric series; the names, label keys, and label-value enumerations are mirror images of the QF-side `qf_smackerel_*` series defined in `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`. Identical labels mean shared dashboards and alert rules can pivot between sides without translation.

| Metric | Type | Labels |
|--------|------|--------|
| `smackerel_qf_packet_ingest_total` | counter | `packet_type`, `decision_type`, `status` (`accepted`, `degraded`, `rejected`), `reason` |
| `smackerel_qf_packet_validation_failures_total` | counter | `reason` (`missing_calibration_badge`, `missing_provenance_badge`, `missing_trace_id`, `unknown_packet_envelope_version`, `unknown_decision_type`, `missing_packet_url`, `signed_url_expired`, `other`) |
| `smackerel_qf_capability_handshake_total` | counter | `outcome` (`ok`, `version_mismatch`, `mandatory_field_missing`, `transport_error`) |
| `smackerel_qf_cursor_lag_seconds` | gauge | none (single value per process) |
| `smackerel_qf_cursor_fast_forward_events_skipped_total` | counter | none |
| `smackerel_qf_freshness_p95_seconds` | gauge | `stage` (`ingest`, `render`, `total`) |
| `smackerel_qf_evidence_export_attempts_total` | counter | `status` (`accepted`, `rejected`, `local_reject`, `export_id_collision`, `transport_error`), `reason` |
| `smackerel_qf_evidence_revoked_total` | counter | `reason` |
| `smackerel_qf_engagement_signal_attempts_total` | counter | `event` (`opened`, `dwell`, `dismissed`, `snoozed`, `deep_linked`, `shared`, `overflow_drop`), `surface` (`web`, `digest`, `telegram`), `status` (`accepted`, `rejected`, `transport_error`), `reason` |
| `smackerel_qf_callback_attempts_total` | counter | `action` (`noop`, `open`), `status` (`deferred`, `signature_invalid`, `signature_expired`, `transport_error`) |
| `smackerel_qf_callback_signature_failures_total` | counter | `reason` |
| `smackerel_qf_personal_context_reads_total` | counter | `outcome` (`ok`, `rejected`, `degraded`), `sensitivity_tier` (`personal`, `sensitive`, `restricted`) |

Label-value vocabularies are frozen contract: any new value MUST be added on both sides simultaneously through a capability-version bump.

### Freshness SLA (F12)

Pre-MVP latency budgets, measured per-packet from QF-emit to Smackerel render:

| Stage | p95 Budget |
|-------|------------|
| Ingest (QF emit → Smackerel artifact published) | ≤ 30 seconds |
| Render (Smackerel artifact published → user-visible on web/digest/Telegram) | ≤ 30 seconds |
| Total (QF emit → user-visible) | ≤ 60 seconds |

Smackerel emits `smackerel_qf_freshness_p95_seconds{stage}` for `ingest`, `render`, and `total`. Breach of any budget for ≥ 5 consecutive minutes flips connector health to `degraded` and fires an alert with the offending stage. The SLA labels match QF's `qf_smackerel_freshness_p95_seconds` so dashboards can plot both sides side-by-side.

### Cursor Fast-Forward Consumer (F13)

Smackerel never fast-forwards its own cursor. The fast-forward authority lives entirely on the QF side via `POST /api/private/smackerel/v1/cursor:fast-forward` triggered by an operator using QF's `/admin/companion-bridge` UI.

Connector behavior:

- When `cursor_lag_seconds` exceeds the operator-configured breach threshold (default 600 seconds, configurable via `config/smackerel.yaml` under `connectors.qf_decisions.lag_breach_threshold_seconds`), Smackerel emits a structured log event `lag_breach{cursor_lag_seconds, threshold_seconds, last_observed_packet_id, last_observed_packet_ts}` AND fires an alert AND flips connector health to `degraded`.
- The connector MUST NOT call `POST /api/private/smackerel/v1/cursor:fast-forward` itself. The endpoint is operator-only.
- When the QF operator issues a fast-forward, the next Smackerel sync receives the new cursor as a server-side hint payload (`cursor_fast_forward_advice`) inside the next event-poll response. Smackerel applies the advised cursor, increments `smackerel_qf_cursor_fast_forward_events_skipped_total` by the count of skipped events, and flips connector health from `degraded` to `degraded_recovered`.
- Skipped packets are NEVER backfilled — they are gone from the connector's perspective. The skipped count is a permanent operational record.
- Audit envelope entry is recorded for both the lag-breach event (`action=cursor_lag_breach`, `outcome=degraded`) and the fast-forward applied event (`action=cursor_fast_forward`, `outcome=degraded`).

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

## Reconciliation Notes (2026-05-07)

This design has been re-reviewed against the updated `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` (Phase B1) and against new Smackerel-side requirements FR-013 through FR-020 added by Phase A2 in `spec.md`. The following design-only updates were applied to back the new requirements and to lock cross-repo symmetry on the eight aligned cross-product opportunities. No spec, scope, state, report, uservalidation, runtime, config, or operational-doc file was touched.

### Smackerel-Side Requirements Backed (FR-013 — FR-020)

| Requirement | Design Section That Backs It |
|-------------|------------------------------|
| FR-013 Packet Engagement Signal Exporter | "Packet Engagement Signal Exporter (FR-013, O1)" — surface coverage, signal envelope, consent gate, buffer/flush, failure handling, audit, write-only non-influence. |
| FR-014 Personal Context Read Endpoint Exposure | "Personal Context Read API (FR-014, O2)" — endpoint contract, response shape, consent token lifecycle, rate limit, non-influence warning, audit. |
| FR-015 Bidirectional Watch Signal (pre-MVP design only) | "Reserved Schemas (Not Implemented Pre-MVP)" QFWatchSignal row — bidirectional with `source` enum, pre-MVP rejection of `smackerel_propose` with `WATCH_PROPOSALS_DEFERRED_TO_V1`, v1.0 implementation path documented. |
| FR-016 Unified Cross-Product Audit Envelope | "Cross-Product Audit Envelope (FR-016, O4)" — envelope shape mirrored from QF, full Smackerel coverage rules table, sink. |
| FR-017 Signed Callback Protocol (pre-MVP design + signing infra) | "Callback Signing (FR-017, O5)" — payload shape, key management, pre-MVP behavior (signing exercised, QF rejects with `CALLBACK_DEFERRED_TO_V1`), telemetry, v1.0+ path. |
| FR-018 Smackerel Connector Class Evidence Eligibility | "Source Provenance Classes (FR-018, O7)" + Bundle Fields `source_provenance_classes` row — class enum, validation rules, capability allowlist gating, pre-MVP no badge attachment. |
| FR-019 Signed Deep-Link Rendering | "Signed Deep-Link Rendering (FR-019/O8)" — three-tier preference order, expiry handling, audit. |
| FR-020 Preferred Surface Hint Honoring | "Preferred Surface Routing (FR-020/O9)" — behavior matrix, default mapping, fallback audit. |

### Aligned Cross-Product Opportunities (O1 — O9, excluding O6 Streaming)

- O1 Engagement signal — backed by "Packet Engagement Signal Exporter".
- O2 Personal-context read — backed by "Personal Context Read API".
- O3 Watch lattice extension — backed via QFWatchSignal Reserved Schema row pre-MVP design only.
- O4 Unified audit envelope — backed by "Cross-Product Audit Envelope".
- O5 Signed callback protocol — backed by "Callback Signing".
- O6 Streaming — NOT adopted pre-MVP; HTTP polling against the QF PostgreSQL-backed companion outbox is the chosen transport.
- O7 Connector-class evidence eligibility — backed by "Source Provenance Classes".
- O8 Signed deep-link rendering — backed by "Signed Deep-Link Rendering".
- O9 Preferred surface hint — backed by "Preferred Surface Routing".

### F-Findings Closed By This Pass

| Finding | Closure In This Design |
|---------|------------------------|
| F2 Capability discovery missing | "Capability Discovery" subsection added under Sync Lifecycle: full capability field table, handshake on Connect, behavior on incompatibility (connector enters `degraded`, no syncs until rehandshake). |
| F4 Idempotency 200 vs 409 semantics undefined | "Idempotency Response Handling (F4)" subsection added under Evidence Bundle Export Design: explicit table for HTTP 200, 201, 409 (`EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD`, `EXPORT_ID_PREVIOUSLY_REJECTED`), 4xx, 5xx connector behavior. Engagement signal exporter applies the same 200/201/409 contract. |
| F6 Trust object rendering contract undefined | "Trust Object Rendering" subsection added: per-badge rendering contract for `CalibrationBadge`, `DataProvenanceBadge`, `QuantifiedImpact`, `ExpertAnalysisBundle` with label, severity, summary, detail, and links rules. |
| F8 Forward-compatible decision_type | "Forward-Compatible decision_type Handling (F8)" subsection added: unknown values normalize to `qf/decision-packet` with `unknown_decision_type=true`, never break ingestion, never appear as actionable. "no_action Decision Semantics (F8/F17)" added to make the no-action capability gate explicit. |
| F9 Page-size handling undefined | "Page Size Handling (F9)" subsection added under Sync Lifecycle: explicit configured `page_size`, no hidden fallback, capability-persisted `[min_page_size, max_page_size]` clamp, missing capability blocks polling, and `PAGE_SIZE_OUT_OF_RANGE` rejection marks degraded without retrying a guessed limit. |
| F11 Symmetric metric naming | "Symmetric Metric Set" subsection added under Operations: full table of `smackerel_qf_*` metrics with label keys and value vocabularies that mirror QF's `qf_smackerel_*` series exactly. |
| F12 Freshness SLA | "Freshness SLA (F12)" subsection added: p95 ingest ≤30s, render ≤30s, total ≤60s; 5-minute breach → `degraded`; emits `smackerel_qf_freshness_p95_seconds{stage}`. |
| F13 Cursor fast-forward consumer behavior | "Cursor Fast-Forward Consumer (F13)" subsection added: connector logs `lag_breach`, never fast-forwards itself, applies operator-issued fast-forward via in-band `cursor_fast_forward_advice`, increments skipped counter, flips to `degraded_recovered`. |
| F14 Consent revocation client | "Consent Revocation Path (F14, FR-014)" subsection added under Evidence Bundle Export Design: `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with reason, 200/204/404/409/5xx handling, audit, metric. |
| F15 Evidence import limits | "Evidence Import Limits (F15)" subsection added: capability-declared bundle size, claims, and per-credential rate limits with local-reject reasons (`BUNDLE_TOO_LARGE`, `TOO_MANY_CLAIMS`, `RATE_LIMIT_EXCEEDED`); never silently truncated. |
| F16 Credential rotation overlap | "Credential rotation overlap (F16)" row added to Security And Privacy table: ≤24h overlap, latest `not_before` wins, cursor/idempotency state survives, post-rotation handshake confirmed. |
| F17 no_action capability gate | Captured in "no_action Decision Semantics (F8/F17)" — emission gated by `no_action_emission_supported` capability flag; renders as advisory artifact with no actionable controls. |

### Cross-Repo Symmetry Confirmation

- Capability response field set matches the QF capability response shape exactly.
- Idempotency reason codes (`EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD`, `EXPORT_ID_PREVIOUSLY_REJECTED`, `EVIDENCE_EXPORT_ID_NOT_FOUND`, `EVIDENCE_EXPORT_ID_ALREADY_REVOKED`) match QF.
- Engagement signal envelope fields (`signal_id`, `packet_id`, `trace_id`, `engagement_event`, `engagement_ts`, `surface`, `consent_scope`, `actor_ref`, `dwell_seconds`) and reason codes (`ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD`, `ENGAGEMENT_PACKET_NOT_FOUND`, `ENGAGEMENT_TRACE_ID_MISMATCH`, `ENGAGEMENT_CONSENT_REQUIRED`, `ENGAGEMENT_DWELL_FIELD_MISMATCH`) match QF.
- Audit envelope shape (`trace_id`, `packet_id`, `export_id`, `signal_id`, `actor_ref`, `surface`, `action`, `outcome`, `reason`, `ts`, `envelope_version: "v1"`) is identical on both sides.
- Callback envelope canonical signing payload (`callback_id|trace_id|packet_id|action|nonce|expires_at|surface`) matches QF byte-for-byte.
- Signed deep-link canonical payload (`packet_id|trace_id|exp`) and field names (`packet_url_signed`, `signature_expires_at`) match QF.
- Personal-context endpoint contract (`GET /api/private/qf/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}`) matches the QF consumer specification.
- Reserved schemas (`QFApprovalAction`, `QFWatchSignal`, `SmackerelCallbackEnvelope`) are mirrored in both designs with identical pre-MVP rejection semantics.
- Symmetric metric label-value vocabularies match across `smackerel_qf_*` and `qf_smackerel_*` series.
- Source provenance class enum (`smackerel_markets`, `smackerel_weather`, `smackerel_news`, `smackerel_geopolitical`, `smackerel_other`, `external`) matches QF's `eligible_smackerel_source_classes` allowlist.
- Preferred surface enum (`web`, `digest`, `telegram`) and default mapping are identical on both sides.

### Endpoints Documented As Connector-Emitting Or Connector-Consuming

| Endpoint | Direction |
|----------|-----------|
| `POST /api/private/smackerel/v1/cursor:fast-forward` | Operator-only on QF; connector consumes the in-band `cursor_fast_forward_advice` payload (does NOT call this endpoint itself). |
| `GET /api/private/smackerel/v1/capabilities` | Connector-emitting at handshake. |
| Event-poll endpoint(s) under `/api/private/smackerel/v1/decision-events` and `/api/private/smackerel/v1/decision-packets/{id}` | Connector-emitting per Sync Lifecycle. |
| `POST /api/private/smackerel/v1/personal-evidence-bundles` | Connector-emitting (evidence import). |
| `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` | Connector-emitting (consent revocation). |
| `POST /api/private/smackerel/v1/packet-engagement-signals` | Connector-emitting (engagement signal exporter). |
| `POST /api/private/smackerel/v1/callback` | Connector-emitting (signed callback infra exercise; QF rejects pre-MVP). |
| `POST /api/private/smackerel/v1/watch-signal-proposals` | Connector-emitting only via diagnostic exercise; QF rejects pre-MVP. |
| `GET /api/private/qf/v1/personal-context` | Connector-CONSUMING (Smackerel hosts the endpoint; QF guided-analysis and Rhai workflows consume it). |

### Confirmation Of Zero Non-Design Mutations

This reconciliation pass touched ONLY `specs/041-qf-companion-connector/design.md`. The following files were verified untouched:

- `specs/041-qf-companion-connector/spec.md`
- `specs/041-qf-companion-connector/scopes.md`
- `specs/041-qf-companion-connector/state.json`
- `specs/041-qf-companion-connector/report.md`
- `specs/041-qf-companion-connector/uservalidation.md`
- All runtime source under `internal/`, `cmd/`, `ml/`
- All configuration under `config/`
- All operational documentation under `docs/`

No certification fields, scope DoD checkboxes, execution evidence, runtime code, configuration, or operational documentation were modified by this design-only reconciliation pass.

## Scope 2-owned metrics (consolidated reference)

This subsection consolidates the 5 metrics owned by Scope 2 (QF Decisions
connector core behaviour). It exists to satisfy the Scope 2 Build
Quality Gate DoD item "New Scope 2-owned metrics are documented in
`design.md` and exposed via the Prometheus registry without altering
the Scope 5-owned full 12-metric symmetric set commitments."

| Metric | Type | Labels | Emitted from | Purpose |
|--------|------|--------|--------------|---------|
| `smackerel_qf_capability_mismatch_total` | counter | `required`, `actual` | `internal/connector/qfdecisions/connector.go` `Connect()` capability handshake (cf. §Capability Discovery above and line 163) | One increment per mismatched dimension when the QF-side capability response is incompatible with the connector's required packet version / required event types. Drives operator-visible `mismatched` health state. |
| `smackerel_qf_unknown_decision_type_total` | counter | `value` (raw `decision_type` string from the QF event) | `internal/connector/qfdecisions/connector.go` `Sync()` decision-type normalization (cf. line 302) | One increment per unknown `decision_type` value observed; canonical `qf/decision-packet` content type is preserved and `Metadata.unknown_decision_type = true` is set on the persisted artifact. User-visible rendering belongs to Scope 3. |
| `smackerel_qf_cursor_lag_seconds` | gauge | (none) | `internal/connector/qfdecisions/connector.go` `Sync()` lag computation | Current observed cursor lag in seconds. Crossing the configured threshold (default 1h) emits a structured `lag_breach` event for the operator dashboard; the connector does NOT auto-fast-forward. |
| `smackerel_qf_cursor_fast_forward_events_skipped_total` | counter | (none) | `internal/connector/qfdecisions/connector.go` `Sync()` fast-forward handling | Incremented by `events_skipped` whenever the QF response carries a `fast_forward_recovered` event. Connector persists the advanced `next_cursor`, marks health `degraded_recovered`, and resumes polling. |
| `smackerel_qf_freshness_p95_seconds` | gauge | `stage` (`ingest`, `render`) | `internal/connector/qfdecisions/connector.go` instrumentation + Scope 3 render path | p95 freshness latency per stage. Stress scenario asserts p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s (see Freshness SLA references to `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Freshness SLA). |

These 5 metrics are scoped strictly to Scope 2 core-behaviour
observability. They are independent of the Scope 5-owned full
12-metric symmetric set commitment (which covers credential
rotation / liveness instrumentation) and do not modify, rename, or
re-label any Scope 5 metric. Adding a 6th metric to this list
requires a planning round.
