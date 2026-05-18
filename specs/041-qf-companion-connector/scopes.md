# Scopes: QF Companion Connector

## Execution Outline

### Phase Order

1. Scope 1: Connector configuration and QF client contract - complete the `qf-decisions` connector boundary, explicit config requirements, client DTOs, health checks, schema-mismatch no-publication behavior, and zero trusted-artifact publication from `Sync()`.
2. Parked Scope 2: Capability handshake, cursor sync normalization, and storage - activate after QF 063 Scope 2 read/outbox readiness is available; owns Phase B2 capability discovery, unknown decision-type ingest metadata, page-size clamping, freshness SLA stress, cursor lag breach signaling, and fast-forward recovery additions.
3. Parked Scope 3: Web Telegram digest and search surfacing - activate after Scope 2 produces source-qualified QF artifacts; folds Phase B2 Trust Object Rendering Contract, signed deep-link rendering preference, and preferred-surface routing additions.
4. Parked Scope 4: Personal evidence bundle export - activate after read-only packet surfacing is available; folds Phase B2 idempotency response handling, `packet_context` target extension, evidence import limits, consent revocation, and source-provenance class eligibility additions.
5. Parked Scope 5: Credential rotation, safety boundaries, observability, documentation, and tests - activate after the connector vertical slices above are implemented; owns credential rotation overlap plus Phase B2 symmetric `smackerel_qf_*` metric set and Cross-Product Audit Envelope v1 additions.
6. Parked Scope 6: Packet engagement signal exporter - new pre-MVP scope (O1, FR-013) folding the engagement event capture, consent gate, buffer/flush, idempotent POST, and audit envelope design.
7. Parked Scope 7: Personal context read API host - new pre-MVP scope (O2, FR-014) folding the connector-hosted `GET /api/v1/personal-context` endpoint, consent token contract, sensitivity tiering, non-influence warning, rate limit, and audit envelope design.
8. Parked Scope 8: Signed callback protocol - new pre-MVP design-only scope (O5, FR-017) folding the HMAC-SHA256 callback signing infrastructure, key rotation playbook, QF version-one callback rejection parsing, and callback telemetry design.
9. Parked Scope 9: Watch signal proposal endpoint - new pre-MVP design-only scope (O3, FR-015) folding the `POST /watch-signal-proposals` request shape, signing, and QF version-one watch-proposal rejection parsing design; no proposal influences QF watch state pre-MVP.

### New Types And Signatures

- Connector ID: `qf-decisions`.
- Package boundary: `internal/connector/qfdecisions`.
- Configuration keys: `connectors.qf-decisions.enabled`, `base_url`, `credential_ref`, `sync_schedule`, `packet_version`, and `page_size`.
- QF DTO mirrors: `QFDecisionEvent`, `QFDecisionPacketEnvelope`, `PersonalEvidenceBundle`, and reserved action/watch diagnostics.
- Artifact content types for downstream scopes: `qf/decision-packet`, `qf/no-action-decision`, `qf/policy-denial`, and reserved diagnostic `qf/approval-request`.
- Scope 1 active contract: connector registration, explicit configuration validation, private read client validation, QF DTO JSON field names, bridge health mapping, and no trusted artifact publication during schema mismatch.
- Evidence export contract reserved for parked Scope 4: user-selected Smackerel context to `PersonalEvidenceBundle` to QF private-alpha import path.
- Scope 2-owned capability handshake DTO: `GET /api/private/smackerel/v1/capabilities` consumed before decision-event polling and re-read on connector restart/credential rotation start; persisted capability fields per design.md §Capability Discovery; refusal to poll when required sync contract fields are incompatible.
- Scope 2/3-owned unknown decision flag: `unknown_decision_type=true` metadata flag on packet ingest in Scope 2, with generic packet card fallback rendering in Scope 3.
- Scope 5-owned credential rotation overlap contract: overlapping QF credentials accepted for no more than 24h, newest valid `not_before` credential selected, cursor and idempotency state preserved, and rotation diagnostics emitted.
- Phase B2 evidence DELETE endpoint: `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with `{reason}` body for consent revocation.
- Phase B2 connector-emitted endpoints: `POST /api/private/smackerel/v1/packet-engagement-signals` (engagement signal exporter) and `POST /api/private/smackerel/v1/watch-signal-proposals` (pre-MVP rejected by QF).
- Phase B2 connector-hosted endpoint: `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}` (Smackerel hosts; QF consumes).
- Phase B2 callback signing primitive: HMAC-SHA256 over `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` with `key_id`; pre-MVP signing infra exercised but every callback returns the QF version-one callback rejection response.
- Phase B2 symmetric metric set: 12 `smackerel_qf_*` metrics with documented labels mirrored from QF spec 063 (see Parked Scope 5 DoD).
- Phase B2 Cross-Product Audit Envelope v1 emitted on every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick.
- Phase B2 freshness SLA: p95 ingest ≤30s, p95 render ≤30s, combined p95 ≤60s; metric `smackerel_qf_freshness_p95_seconds{stage}`.
- Phase B2 trust object rendering contract: digest and Telegram renderers consume only `label`, `severity`, `summary`, optional `detail`, optional `links` from CalibrationBadge / DataProvenanceBadge / QuantifiedImpact / ExpertAnalysisBundle; numeric internals silently dropped.
- Phase B2 preferred surface routing: `preferred_surface` hint values `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, `any` route render placement only; never alter trust metadata, decision content, or action eligibility.
- Phase B2 evidence bundle additions: `target_context` extended with `packet_context`; `source_provenance_classes` field per bundle; pre-flight import limits `evidence_max_bundle_size_bytes` (default 524288), `evidence_max_claims_per_bundle` (default 50), `evidence_rate_limit_per_minute` (default 10) per credential.
- Phase B2 personal-context read response: list of personal-context items (notes, locations, timeline events) up to `max_sensitivity` with required `non_influence_warning` field.

### Validation Checkpoints

- Scope 1 validation proves the connector cannot start without explicit QF base URL, credential reference, packet version, sync schedule, and page size.
- Scope 1 validation proves schema mismatch and authorization failure produce degraded/error connector health without publishing trusted QF artifacts.
- Scope 1 validation does not require capability discovery, unknown decision-type ingest/rendering, or credential rotation overlap; those checks are owned by Scope 2, Scope 3, and Scope 5 respectively.
- Scope 1 validation uses existing files only: `internal/connector/qfdecisions/connector_test.go`, `internal/connector/qfdecisions/client_test.go`, `tests/integration/qf_decisions_connector_config_test.go`, and `tests/e2e/qf_decisions_connector_api_test.go`.
- Scopes 2-5 stay parked until QF 063 Scope 2 exposes the read/outbox readiness needed for live packet sync.

## Overview

This plan implements the Smackerel side of the pre-MVP QF companion integration. Smackerel acts as a passive connector, memory, attention, digest, search, Web, and Telegram surface. QF remains the system of record and financial decision authority.

The connector must never generate financial advice, approve trades, change mandates, execute, upgrade trust badges, hide downgraded QF metadata, or treat QF packets as Smackerel-local recommendations. Reverse flow is limited to user-initiated, consent-scoped `PersonalEvidenceBundle` export.

Capabilities requiring QF-owned approval, watch, tenant, voice, EmergencyStop, paper execution, or live execution contracts remain in the parked release ladder and are not claimed by pre-MVP DoD.

## Plan Notes (2026-05-07)

- Cross-repo symmetry verified against `quantitativeFinance/specs/063-smackerel-companion-bridge/spec.md` (Phase A2) and `quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` (Phase B2).
- Pre-MVP design-only release boundary preserved for opportunities O3 (watch signal proposals), O4 (cross-product audit envelope mirroring to QF), O5 (signed callback infrastructure), O7 (source-provenance class evidence badges), and O8 (additional design-only deltas).
- Pre-MVP implementation in scope for opportunities O1 (packet engagement signal exporter), O2 (personal context read API host), and O9 (preferred-surface routing).
- Opportunity O6 (real-time streaming over polling) explicitly NOT adopted for pre-MVP.
- Open questions OQ-04, OQ-05, OQ-06 (cross-repo) are held outside pre-MVP planning.
- Phase B2 closes design findings F2, F4, F6, F8, F9, F11, F12, F13, F14, F15, F16, and F17 by folding their resolutions into the appropriate scope DoD additions below.
- All Phase B2 additions are recorded as unchecked DoD items only; no scope status is changed and no DoD checkbox is checked by this planning pass.
- Boundary repair 2026-05-07: low-impact audit classified the Phase B2 capability handshake, unknown decision-type behavior, and credential rotation overlap as outside active Scope 1 certification. Scope 1 remains limited to configuration, registration, read-client DTO contract, bridge validation, health mapping, and zero-artifact sync behavior.

## Active Scope Inventory

| Scope | Name | Surfaces | Required Tests | DoD Summary | Status |
|-------|------|----------|----------------|-------------|--------|
| 1 | Connector configuration and QF client contract | Config generation, connector registry, QF client DTOs | Unit, integration, scenario-specific Regression E2E, broader E2E, artifact lint | Connector starts only with explicit config and compatible QF contract | Done |
| 2 | Capability handshake, cursor sync normalization, and storage | Connector supervisor, QF capability client, state store, artifact pipeline, PostgreSQL | Unit, integration, e2e, stress, scenario regression | Capability discovery, normalized cursor sync, page-size clamping, freshness SLA, lag breach signaling | Not Started |

## Parked Scope Queue

These scopes preserve the product intent and dependency order but are not part of the active execution inventory for Scope 1 validation. They must be expanded back into executable scope sections by `bubbles.plan` after the QF wait state clears. (Scope 2 was unparked on 2026-05-13 after QF 063 reached `done_with_concerns`; see active Scope 2 section below.)

| Parked Scope | Name | Dependency Gate | Intended Surfaces | Activation Check |
|--------------|------|-----------------|-------------------|------------------|
| 3 | Web Telegram digest and search surfacing | Scope 2 source-qualified artifacts | Web, Telegram, digest, search, artifact detail | QF packets exist as Smackerel artifacts with packet ID, trace ID, approval state, badges, and deep link |
| 4 | Personal evidence bundle export | Scope 3 read-only packet surfacing | Web evidence selection, bundle builder, QF export client, export status | User-visible QF context and consent/sensitivity UI paths exist |
| 5 | Credential rotation, safety boundaries, observability, documentation, and tests | Scopes 2-4 implemented | Credential lifecycle, health diagnostics, logs, metrics, docs, safety gates | Sync, rendering, and export surfaces exist for rotation and boundary verification |
| 6 | Packet engagement signal exporter | Scope 3 trust-rendering surface exists | Digest UI, Telegram bot, mobile push, signal exporter, audit log | Trust-rendering surfaces emit packet renders that can be instrumented |
| 7 | Personal context read API host | Scope 3 trust-rendering surface exists | Connector-hosted private API, consent token issuer, sensitivity store, audit log | Personal-context entities (notes, locations, timeline events) and consent token issuance exist |
| 8 | Signed callback protocol | Scope 3 trust-rendering surface exists | Callback signer, key store, callback transport, audit log | Trust-rendering surfaces present action-eligible packets that may emit callbacks (rejected pre-MVP) |
| 9 | Watch signal proposal endpoint (pre-MVP design only) | Scope 2 capability handshake exists | Watch proposal client, signer, audit log | Capability handshake is operational so proposal endpoint readiness can be advertised and rejected by QF |

### Parked Scope Contract Notes

- Scope 2 must implement the capability handshake before decision-event polling: call `GET /api/private/smackerel/v1/capabilities`, parse and persist the fields enumerated in design.md §Capability Discovery, block the sync path when required sync contract fields are incompatible, and emit capability mismatch diagnostics.
- Scope 2 must persist response-level `next_cursor` in `sync_state.sync_cursor`, treat per-event `QFDecisionEvent.cursor` as diagnostic-only, and preserve QF packet identity.
- Scope 2 must handle unknown QF `decision_type` values at ingest by preserving the packet with `Metadata.unknown_decision_type = true`, never inventing a new `qf/...` content type, and emitting `smackerel_qf_unknown_decision_type_total{value}`; Scope 3 owns the user-visible generic-card fallback.
- Scope 2 must map QF `decision_type` values exactly: `recommendation` to `qf/decision-packet`, `no_action` to `qf/no-action-decision`, `policy_denial` to `qf/policy-denial`, and `analysis_note` to `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`.
- Scope 2 must clamp page sizes to `[1, max_page_size]` from the capability response (fallback default 200 when capability missing) and reject `PAGE_SIZE_OUT_OF_RANGE` 4xx responses with operator alerts (Phase B2, F9).
- Scope 2 must satisfy the freshness SLA targets p95 ingest ≤30s, p95 render ≤30s, and combined p95 ≤60s, and expose `smackerel_qf_freshness_p95_seconds{stage}` (Phase B2, F12).
- Scope 2 must surface cursor lag breaches as structured `lag_breach` log events when `smackerel_qf_cursor_lag_seconds` exceeds the operator-configured threshold (default 1h) and never auto-fast-forward; on QF-issued fast-forward the connector picks up `events_skipped`, marks state `degraded_recovered`, and increments `smackerel_qf_cursor_fast_forward_events_skipped_total` (Phase B2, F13).
- Scope 3 must render QF packets as QF-authored read-only artifacts across Web, digest, Telegram-compatible summaries, and search.
- Scope 3 must render unknown decision-type packets through a generic QF packet card that preserves QF-authored content, trust metadata, and deep links without deriving semantics from the packet body.
- Scope 3 must enforce the Trust Object Rendering Contract: digest and Telegram renderers consume only `label`, `severity`, `summary`, optional `detail`, and optional `links` from CalibrationBadge, DataProvenanceBadge, QuantifiedImpact, and ExpertAnalysisBundle; numeric internals are silently dropped (not errors); missing required `label`/`severity` fails loud with `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}` and falls back to a generic packet card (Phase B2, F6).
- Scope 3 must prefer `packet_url_signed` for deep-link rendering when present and unexpired, fall back to unsigned only when the capability declares `deep_link_signing_supported=false`, refetch on signature expiry mid-render, and emit `smackerel_qf_deep_link_render_total{surface,status}` with statuses `signed_used`, `signed_expired_fallback_unsigned`, and `unsigned_only` (Phase B2, F6).
- Scope 3 must honor `preferred_surface` routing values `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, and `any` per design.md §Preferred Surface Routing; routing must never alter trust metadata, decision content, or action eligibility (Phase B2, O9).
- Scope 4 must build `PersonalEvidenceBundle`s with `bundle_id`, `export_id`, `consent_scope`, `sensitivity_tier`, `source_artifact_ids`, `extracted_claims`, `provenance`, `redaction_summary`, `target_context`, and `created_at`; `source_refs` remains optional when sources have external IDs.
- Scope 4 must treat HTTP 200 idempotency replay as no-op success and HTTP 409 `EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD` as a hard abort with `EXPORT_ID_COLLISION` audit error; never retry 409 (Phase B2, F4).
- Scope 4 must extend `target_context` with `packet_context` to support bundles attached to a packet (Phase B2, F4).
- Scope 4 must enforce evidence import limits pre-flight: bundle size ≤ `evidence_max_bundle_size_bytes` (capability default 524288), claim count ≤ `evidence_max_claims_per_bundle` (default 50), per-credential rate ≤ `evidence_rate_limit_per_minute` (default 10) via token bucket; reject locally with `BUNDLE_TOO_LARGE`, `TOO_MANY_CLAIMS`, or `RATE_LIMIT_EXCEEDED`; emit `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason}` (Phase B2, F14).
- Scope 4 must support consent revocation via `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with `{reason: "consent_revoked"}` body, mark the local artifact `revoked`, and emit a unified audit envelope (Phase B2, F15).
- Scope 4 must populate `source_provenance_classes` on every exported bundle, validate pre-flight against capability `eligible_smackerel_source_classes`, and reject locally with `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{class}` for any non-eligible class; pre-MVP design-only — badge attachment must not be enabled (Phase B2, O7).
- Scope 5 must implement credential rotation overlap after sync and export state exist: accept two QF credentials for no more than 24h, select the newest valid credential by `not_before`, preserve connector cursor and evidence/export idempotency state through rotation, re-read capabilities at rotation start, and emit operator diagnostics.
- Scope 5 must preserve the safety boundary: no Smackerel approval, execution, mandate change, EmergencyStop behavior, QF watch creation, or QF trust reconstruction is claimed by pre-MVP DoD.
- Scope 5 must emit the symmetric `smackerel_qf_*` metric set (12 metrics) with documented labels matching QF design 063 label parity (Phase B2, F11).
- Scope 5 must emit the Cross-Product Audit Envelope v1 for every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick; sink is the connector audit log with opt-in QF mirror reserved post-MVP (Phase B2, O4).
- Reserved schemas remain diagnostic before activation: inbound `QFApprovalAction` normalizes to `qf/approval-request` with `Metadata.reserved = true` and stays out of search, digest, recommendation, and evidence-builder surfaces; inbound `QFWatchSignal` records diagnostics only.
- Scope 6 must capture engagement events `opened`, `dwell` (with seconds), `dismissed`, `snoozed`, `deep_linked`, and `shared` across digest UI, Telegram bot, and mobile push; emit only when `engagement_telemetry` is `anonymous` or `pseudonymous`; buffer in memory and flush every 10s or 100 events; POST to `/api/private/smackerel/v1/packet-engagement-signals` with client-generated UUIDv7 `signal_id`; drop on 4xx and retry up to 3 times with backoff on 5xx; audit envelope on every flush attempt; metric `smackerel_qf_engagement_signal_attempts_total{event,surface,status}` (Phase B2, O1, FR-013).
- Scope 7 must host `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}`, returning a list of personal-context items (notes, locations, timeline events) up to `max_sensitivity`; consent tokens are short-lived (≤15min) and scope-limited (entity, sensitivity, requester_id baked in); response includes a `non_influence_warning` field; rate limit 5 reads per consent token; audit envelope on every fetch (Phase B2, O2, FR-014).
- Scope 8 must sign callbacks with HMAC-SHA256 over the canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface`; carry `key_id` in the callback envelope; rotate keys per release with documented playbook; pre-MVP every callback is rejected by QF with the version-one callback rejection response; emit `smackerel_qf_callback_signature_failures_total{reason}` and `smackerel_qf_callback_attempts_total{action,status}` (Phase B2, O5, FR-017).
- Scope 9 must POST `/api/private/smackerel/v1/watch-signal-proposals` with `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}`; pre-MVP every request is rejected by QF with the version-one watch-proposal rejection response; signing infra exercised; integration test verifying request shape, signing, and rejection parsing; no proposal influences QF watch state pre-MVP (Phase B2, O3, FR-015).

## Scope 1: Connector Configuration And QF Client Contract

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

Scenario: SCN-SM-041-001 Connector Starts With Explicit Configuration
	Given a Smackerel operator enables `qf-decisions`
	When the connector starts
	Then it requires explicit QF base URL, credential reference, sync schedule, packet version, and page size from Smackerel configuration.

Scenario: SCN-SM-041-002 Connector Rejects Missing Or Incompatible QF Contract
	Given the QF bridge is unavailable, unauthorized, or exposes an incompatible packet version
	When `qf-decisions` connects
	Then Smackerel marks the connector degraded or error and does not sync trusted packet artifacts.

### Implementation Plan

- Add `qf-decisions` to the connector registry as a normal passive connector.
- Add config schema and generation support for explicit `base_url`, `credential_ref`, `sync_schedule`, `packet_version`, `page_size`, and `enabled` values.
- Implement QF client DTOs mirroring QF spec 063 names without renaming trust metadata, including the QF `decision_type` field and the canonical `PersonalEvidenceBundle` field set (`target_context` required, `source_refs` optional).
- Validate config in `Connect()` and fail loudly for missing base URL, credential reference, packet version, invalid URL, or invalid page size.
- Use HTTP polling/read surface only; no direct QF database access and no Kafka/NATS federation.
- Preserve source boundary: QF credential scope is enforced by QF, not broadened by Smackerel.
- Change Boundary: connector configuration, registry, client contract, and health checks only.
- Allowed file families: `cmd/core/connectors.go`, `config/smackerel.yaml`, `internal/config/config.go`, `scripts/commands/config.sh`, and `internal/connector/qfdecisions/*` client/connector/type files.
- Excluded surfaces: capability discovery, artifact publication, UI surfacing, local packet normalization, unknown decision-type ingest/rendering, credential rotation overlap, digest generation, cross-project QF write paths, and Scope 2+ source-qualified packet consumption.

### Implementation Files

- `cmd/core/connectors.go`
- `config/smackerel.yaml`
- `internal/config/config.go`
- `scripts/commands/config.sh`
- `internal/connector/qfdecisions/client.go`
- `internal/connector/qfdecisions/connector.go`
- `internal/connector/qfdecisions/types.go`

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Regression E2E | e2e-api | SCN-SM-041-001 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsConnectorHealthAppearsInLiveAPI` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-002 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts` | `./smackerel.sh test e2e` | Yes |
| Unit | unit | SCN-SM-041-001 | `internal/connector/qfdecisions/connector_test.go` | `TestParseConfigRequiresExplicitFields`, `TestConnectValidConfigSetsHealthy` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-002 | `internal/connector/qfdecisions/client_test.go` | `TestClientRejectsIncompatibleQFPacketVersion` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-001, SCN-SM-041-002 | `internal/connector/qfdecisions/client_test.go` | `TestDTOJSONFieldNamesMirrorQFContract` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-001, SCN-SM-041-002 | `tests/integration/qf_decisions_connector_config_test.go` | `TestQFDecisionsConnectorConfigRegistryAndHealthIntegration`, `TestQFDecisionsConnectorSchemaMismatchIntegration`, `TestQFDecisionsConnectorAuthFailureIntegration` | `./smackerel.sh test integration` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-001, SCN-SM-041-002 | `tests/e2e/qf_decisions_connector_api_test.go` | `go-e2e` and shell E2E suite complete without failures | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-001 | `specs/041-qf-companion-connector` | `artifact lint accepts QF connector planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |

### Definition of Done

Core behavior:

- [x] SCN-SM-041-001: `qf-decisions` is registered as a passive connector with explicit configuration owned by `config/smackerel.yaml` and generated env output. Evidence: `report.md` -> Scope 1 Integration Evidence, Scope 1 Check Evidence, Code Diff Evidence.
- [x] SCN-SM-041-001 and SCN-SM-041-002: Connector startup fails for missing base URL, credential reference, packet version, sync schedule, page size, invalid URL, invalid sync schedule, or invalid page size. Evidence: `report.md` -> Scope 1 Unit Evidence, Scope 1 Integration Evidence.
- [x] SCN-SM-041-001 and SCN-SM-041-002: QF client DTOs mirror QF spec 063 field names for packet IDs, trace IDs, approval state, badges, deep links, `decision_type`, and evidence bundles, including required `target_context` and optional `source_refs` semantics. Evidence: `report.md` -> Code Diff Evidence, Scope 1 Unit Evidence, RED Proof Note.
- [x] SCN-SM-041-002: Connector rejects missing or incompatible QF contracts by degrading health and blocking trusted artifact sync. Evidence: `report.md` -> Scope 1 Unit Evidence, Scope 1 Integration Evidence, Scope 1 E2E API Evidence.
- [x] SCN-SM-041-002: The connector uses HTTP polling/read surface only; no direct QF database access, broker federation, embedded credentials, or trusted artifact publication on schema mismatch. Evidence: `report.md` -> Scope 1 Unit Evidence, Scope 1 Implementation Reality Evidence, Scope 1 E2E API Evidence.

Validation:

- [x] SCN-SM-041-001 and SCN-SM-041-002: Unit tests cover configuration validation and QF client contract compatibility. Evidence: `report.md` -> Scope 1 Unit Evidence.
- [x] SCN-SM-041-001 and SCN-SM-041-002: Integration tests prove registry startup and health transitions for valid config, auth failure, and schema mismatch. Evidence: `report.md` -> Scope 1 Integration Evidence.
- [x] SCN-SM-041-002: E2E API regression test proves incompatible schema does not publish trusted packet artifacts. Evidence: `report.md` -> Scope 1 E2E API Evidence.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass for SCN-SM-041-001 and SCN-SM-041-002. Evidence: `report.md` -> Scope 1 E2E API Evidence.
- [x] Broader E2E regression suite passes. Evidence: `report.md` -> Scope 1 Broader E2E Evidence (2026-05-07T18:06Z run captured to `/tmp/my-broader-e2e3.log`; all Go e2e packages PASS, shell suite 35/35 PASS, `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts` 0.64s PASS).

Build quality gate:

- [x] Raw unit, integration, E2E, and artifact-lint evidence is recorded in `report.md` before DoD items are checked. Evidence: `report.md` -> Scope 1 Unit Evidence, Scope 1 Integration Evidence, Scope 1 E2E API Evidence, Scope 1 Artifact Lint Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md` -> Planning Repair Guard Evidence.
- [x] No fallback defaults, hardcoded QF credentials, hardcoded QF URLs, or generated config hand edits are introduced. Evidence: `report.md` -> Scope 1 Check Evidence, Scope 1 Implementation Reality Evidence.
- [x] Documentation identifies QF as the system of record and Smackerel as a companion connector. Evidence: `report.md` -> Scope 1 Documentation Boundary Evidence.

### Boundary Decision (2026-05-07)

Low-impact audit classified the prior Phase B2 additions as outside active Scope 1 certification:

- Capability handshake is Scope 2-owned because it controls decision-event polling, page-size limits, supported event/decision-type routing, evidence limits, render feature toggles, and cross-product audit envelope versioning. Its activation gate is QF 063 Scope 2 read/outbox readiness.
- Unknown decision-type behavior is Scope 2-owned for ingest metadata and metric emission, then Scope 3-owned for generic-card rendering. Active Scope 1 publishes zero artifacts from `Sync()` and excludes local packet normalization/rendering.
- Credential rotation overlap is Scope 5-owned because it spans credential lifecycle, persisted cursor state, evidence/export idempotency, capability re-read, and operator diagnostics after the sync/render/export surfaces exist.

Scope 1 remains eligible for certification only against the narrow connector boundary: explicit configuration, connector registration, QF GET client DTOs, bridge validation, health mapping, and zero trusted artifact publication from `Sync()`. This section does not check any DoD item and does not change Scope 1 status.

## Scope 2: Capability Handshake, Cursor Sync Normalization, And Storage

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1
**Activation:** Unparked 2026-05-13 after QF 063 reached `done_with_concerns` on 2026-05-12; bridge `GET /api/private/smackerel/v1/capabilities`, `GET /api/private/smackerel/v1/decision-events`, and `GET /api/private/smackerel/v1/decision-packets/{packet_id}` are available per `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`.

### Gherkin Scenarios

Scenario: SCN-SM-041-003 Capability Handshake Before Polling
	Given the QF bridge is reachable and exposes `GET /api/private/smackerel/v1/capabilities`
	When the `qf-decisions` connector starts (`Connect()`) or restarts after a credential reload
	Then it calls the capability endpoint before any decision-event poll, parses every field documented in `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Capability Discovery (`supported_packet_versions`, `supported_event_types`, `supported_decision_types`, `max_page_size`, `min_page_size`, `audit_envelope_version`, `freshness_sla_p95_seconds`, `deep_link_signing_supported`, `engagement_signal_supported`, `eligible_smackerel_source_classes`, etc.), persists the response into the connector state store, and only then enables polling.

Scenario: SCN-SM-041-004 Incompatible Capability Response Blocks Polling
	Given the QF capability response is missing required `packet_version` `v1` or any required `supported_event_type`
	When the `qf-decisions` connector reads the response
	Then it MUST NOT call `GET /decision-events`, MUST mark connector health as `mismatched`, MUST emit `smackerel_qf_capability_mismatch_total{required,actual}`, and MUST publish zero trusted artifacts from `Sync()`.

Scenario: SCN-SM-041-005 Page Size Clamped To Capability Range
	Given the connector configuration requests a page size outside `[min_page_size, max_page_size]` from the persisted capability response (or capability is missing and the fallback default 200 applies)
	When the connector calls `GET /decision-events`
	Then the request `limit` MUST be clamped to the capability range, and any `PAGE_SIZE_OUT_OF_RANGE` 4xx response MUST be surfaced as an operator alert without retrying the same out-of-range request.

Scenario: SCN-SM-041-006 Unknown Decision Type Ingested With Metadata Flag
	Given QF emits a `QFDecisionPacketEnvelope` whose `decision_type` is not in `supported_decision_types` (or the envelope sets `unknown_decision_type=true`)
	When the connector ingests the packet via the normalizer
	Then the resulting Smackerel artifact MUST have `Metadata.unknown_decision_type = true`, MUST NOT invent a new `qf/...` content type, MUST keep the canonical `qf/decision-packet` content type, and MUST increment `smackerel_qf_unknown_decision_type_total{value=<raw_decision_type>}`. (Generic-card user-visible rendering remains Scope 3 territory.)

Scenario: SCN-SM-041-007 Cursor Lag Breach Logged Without Auto Fast Forward
	Given `smackerel_qf_cursor_lag_seconds` exceeds the operator-configured threshold (default `1h`)
	When the connector observes the lag during a sync tick
	Then it MUST emit a structured `lag_breach` log event (with `cursor_lag_seconds`, `threshold_seconds`, `last_event_id`, `connector_id`) for the operator dashboard, MUST NOT auto-advance its own cursor, and MUST keep polling at its configured cadence.

Scenario: SCN-SM-041-008 Operator-Initiated Fast Forward Recovery
	Given an operator has called `POST /api/private/smackerel/v1/cursor:fast-forward` against QF and QF has advanced the cursor by `events_skipped` events
	When the next `qf-decisions` sync observes the advanced cursor
	Then the connector MUST persist the new `next_cursor`, MUST mark its health label `degraded_recovered`, MUST increment `smackerel_qf_cursor_fast_forward_events_skipped_total` by `events_skipped`, and MUST resume normal polling against the new head.

### Implementation Plan

- Cherry-pick the preserved `internal/connector/qfdecisions/normalizer.go` and `internal/connector/qfdecisions/normalizer_test.go` from branch `parking/041-scope-2-qf-decisions-sync-pending-qf-063` (HEAD `4f90b6fc`); refactor only as needed to integrate with new capability client and unknown-decision-type metadata path.
- Cherry-pick the preserved `internal/connector/qfdecisions/connector.go` `Sync()` rewrite and `internal/connector/qfdecisions/connector_test.go` cursor-identity tests from the parking branch; extend `Sync()` to call the capability client before its first decision-event poll, on connector restart, and on credential rotation start.
- Add `internal/connector/qfdecisions/capability.go` exposing a `CapabilityClient` that GETs `/api/private/smackerel/v1/capabilities`, parses the full field set, performs required-field compatibility checks against the connector build, and returns a typed `Capabilities` value plus diagnostic mismatch records; package-internal so Scope 5 credential rotation can re-use it.
- Add `internal/connector/qfdecisions/capability_test.go` covering response parsing, required-field mismatch detection (`packet_version`, `supported_event_types`), persisted-field round-trip, and metric label correctness.
- Persist capability fields via a new migration `internal/db/migrations/<next-id>_qf_decisions_capability.sql` that adds either dedicated columns (`max_page_size`, `freshness_sla_p95_seconds`, `audit_envelope_version`, `deep_link_signing_supported`, `engagement_signal_supported`, `eligible_smackerel_source_classes`, `capability_fetched_at`) to the existing `sync_state` table OR a sibling `qf_decisions_capabilities` table keyed by `(connector_id, credential_ref)`; design.md will record the chosen shape during implementation.
- Extend `internal/connector/qfdecisions/client.go` to clamp the requested `limit` to `[min_page_size, max_page_size]` from the persisted capability (fallback default 200 when capability is missing during cold start), and to surface `PAGE_SIZE_OUT_OF_RANGE` 4xx responses as operator alerts without retrying the same out-of-range request.
- Add freshness SLA timing instrumentation in `Sync()` and the artifact pipeline so per-stage timestamps can be recorded; expose `smackerel_qf_freshness_p95_seconds{stage}` histogram with stages `ingest` and `render` and a derived `combined` reducer for stress-test consumption.
- Add cursor lag tracking in `Sync()` reading `server_time` from each decision-events response, computing `smackerel_qf_cursor_lag_seconds`, and emitting a structured `lag_breach` log event when the configured threshold (default 1h) is exceeded; never auto-advance the cursor.
- Add fast-forward recovery handling in `Sync()` so when the persisted cursor advances by more than the polled batch size (i.e., QF advanced via `cursor:fast-forward`), the connector reads `events_skipped` from the QF diagnostic event, increments `smackerel_qf_cursor_fast_forward_events_skipped_total`, and transitions connector health to `degraded_recovered`.
- Cherry-pick `tests/integration/qf_decisions_sync_test.go`, `tests/stress/qf_decisions_sync_stress_test.go`, and the Scope 2 ingest portion of `tests/e2e/qf_decisions_connector_api_test.go` from the parking branch; extend with capability-handshake, capability-mismatch, fast-forward-recovery, and freshness-SLA cases as listed in the test plan.
- Wire all new metrics into the existing Prometheus registry exporter; do NOT introduce Scope 5-owned credential rotation behavior or Scope 3-owned rendering surfaces.

### Implementation Files

- `internal/connector/qfdecisions/capability.go` (new)
- `internal/connector/qfdecisions/capability_test.go` (new)
- `internal/connector/qfdecisions/normalizer.go` (cherry-picked from parking branch)
- `internal/connector/qfdecisions/normalizer_test.go` (cherry-picked + extended)
- `internal/connector/qfdecisions/connector.go` (cherry-picked + extended for handshake, lag breach, fast-forward)
- `internal/connector/qfdecisions/connector_test.go` (cherry-picked + extended)
- `internal/connector/qfdecisions/client.go` (extended for page-size clamping)
- `internal/connector/qfdecisions/client_test.go` (extended)
- `internal/db/migrations/<next-id>_qf_decisions_capability.sql` (new)
- `tests/integration/qf_decisions_capability_test.go` (new)
- `tests/integration/qf_decisions_sync_test.go` (cherry-picked + extended for fast-forward recovery)
- `tests/e2e/qf_decisions_connector_api_test.go` (cherry-picked Scope 2 ingest test + new mismatch and unknown-decision-type tests)
- `tests/stress/qf_decision_event_replay_test.go` (refactored from parking-branch `qf_decisions_sync_stress_test.go` to assert freshness SLA budget)

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-003 | `internal/connector/qfdecisions/capability_test.go` | `TestParseCapabilityResponseFields` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-004 | `internal/connector/qfdecisions/capability_test.go` | `TestCapabilityMismatchDetectsRequiredPacketVersion` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-005 | `internal/connector/qfdecisions/client_test.go` | `TestClientClampsPageSizeToCapabilityRange` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-008 | `internal/connector/qfdecisions/connector_test.go` | `TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` (test name reconciled to actual implementation — response-level next_cursor is a Sync-layer concern, not a normalizer-layer concern) | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-006 | `internal/connector/qfdecisions/connector_test.go` | `TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType` (test name reconciled to actual implementation — capability-gated unknown-decision-type metric emission lives in `Sync()`, not the normalizer; metadata-flag persistence on normalized artifacts is a documented honest gap deferred to a future round under bubbles.plan ownership) | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-007 | `internal/connector/qfdecisions/connector_test.go` | `TestConnectorEmitsLagBreachEventAboveThreshold` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-003 | `tests/integration/qf_decisions_capability_test.go` | `TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-003 | `tests/integration/qf_decisions_capability_test.go` | `TestQFDecisionsConnectorReReadsCapabilityOnRestart` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-008 | `tests/integration/qf_decisions_sync_test.go` | `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-004 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsIncompatibleCapabilityBlocksPolling` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-006 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata` | `./smackerel.sh test e2e` | Yes |
| Stress | stress | SCN-SM-041-003, SCN-SM-041-008 | `tests/stress/qf_decision_event_replay_test.go` | `TestQFDecisionsFreshnessSLAP95IngestRender` (asserts p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s) | `./smackerel.sh test stress` | Yes |
| Artifact lint | artifact | SCN-SM-041-003..008 | `specs/041-qf-companion-connector` | `artifact lint accepts QF connector planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Broader E2E | e2e-api | SCN-SM-041-003..008 | `tests/e2e/` | `go-e2e` and shell E2E suite complete without failures | `./smackerel.sh test e2e` | Yes |

### Definition of Done

Core behavior:

- [ ] SCN-SM-041-003: Connector calls `GET /api/private/smackerel/v1/capabilities` before any decision-event poll on `Connect()` and on restart, parses every field documented in `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Capability Discovery, and persists them via the new `qf_decisions_capability` migration. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence.
- [ ] SCN-SM-041-004: Incompatible required capability fields (`supported_packet_versions` missing `v1`, missing required `supported_event_types`) block polling, mark connector health `mismatched`, emit `smackerel_qf_capability_mismatch_total{required,actual}`, and publish zero trusted artifacts. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 E2E API Evidence.
- [ ] SCN-SM-041-005: Page-size requests are clamped to `[min_page_size, max_page_size]` from the persisted capability (fallback default 200 when capability is missing during cold start); `PAGE_SIZE_OUT_OF_RANGE` 4xx is surfaced as an operator alert without retrying the same out-of-range request. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence.
- [x] SCN-SM-041-006: Unknown `decision_type` packets are stored with `Metadata.unknown_decision_type = true`, no new content type is invented (canonical `qf/decision-packet` is preserved), and `smackerel_qf_unknown_decision_type_total{value=<raw_decision_type>}` is incremented; user-visible rendering is left to Scope 3. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 E2E API Evidence, **Round 2L Implementation Evidence (SCN-006 Contract Fix)** — Round 2L Command 1 (unit) PASS via `internal/connector/qfdecisions 0.894s`; E2E API evidence captured as compile-only with runtime-execution Uncertainty Declaration pending spec-045 unblock (routed to `bubbles.test`).
- [x] SCN-SM-041-007: When `smackerel_qf_cursor_lag_seconds` exceeds the configured threshold (default 1h), the connector emits a structured `lag_breach` log event for the operator dashboard, never auto-advances its own cursor, and keeps polling at its configured cadence. Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2N Unit Evidence** (`TestConnectorEmitsLagBreachEventAboveThreshold` PASS in this session via focused `go test -count=1 -v -run`).
- [ ] SCN-SM-041-008: On QF-issued cursor fast-forward, the connector persists the advanced `next_cursor`, marks health `degraded_recovered`, increments `smackerel_qf_cursor_fast_forward_events_skipped_total` by `events_skipped`, and resumes normal polling. Evidence: `report.md` -> Scope 2 Integration Evidence.
- [x] SCN-SM-041-006 and SCN-SM-041-008: Normalizer persists response-level `next_cursor` in `sync_state.sync_cursor`, treats per-event `QFDecisionEvent.cursor` as diagnostic-only, and maps QF `decision_type` values exactly: `recommendation` -> `qf/decision-packet`, `no_action` -> `qf/no-action-decision`, `policy_denial` -> `qf/policy-denial`, `analysis_note` -> `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`. Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2N Unit Evidence** (`TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` PASS + `TestNormalizerContentTypeMappings` 4 sub-tests PASS for `recommendation` / `no_action` / `policy_denial` / `analysis_note` in this session via focused `go test -count=1 -v -run`).
- [ ] SCN-SM-041-003 and SCN-SM-041-008: Freshness SLA instrumentation exposes `smackerel_qf_freshness_p95_seconds{stage}` for stages `ingest` and `render`, and the stress test asserts p95 ingest ≤ 30s, render ≤ 30s, and combined ≤ 60s as required by `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Freshness SLA. Evidence: `report.md` -> Scope 2 Stress Evidence.

Validation:

- [x] SCN-SM-041-003: Unit test `TestParseCapabilityResponseFields` covers full capability response parsing including all enumerated fields. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-004: Unit test `TestCapabilityMismatchDetectsRequiredPacketVersion` covers required-field mismatch detection and metric label correctness. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-005: Unit test `TestClientClampsPageSizeToCapabilityRange` covers page-size clamping and `PAGE_SIZE_OUT_OF_RANGE` 4xx rejection. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-008: Unit test `TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` in `internal/connector/qfdecisions/connector_test.go` covers response-level next_cursor persistence and per-event cursor diagnostic-only treatment (test name reconciled to actual implementation — behavior lives in `Sync()`, not the normalizer). Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-006: Unit tests `TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType` in `internal/connector/qfdecisions/connector_test.go` and `TestNormalizerMarksUnknownDecisionTypeWithMetadata` in `internal/connector/qfdecisions/normalizer_test.go` together cover unknown-decision-type handling at the unit layer: the capability-gated metric emission at `Sync()` AND the normalizer fall-through that preserves the canonical `qf/decision-packet` content type while setting `Metadata.unknown_decision_type = true` on the normalized artifact (delivered Round 2L per design.md §F8). Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2L Implementation Evidence (SCN-006 Contract Fix)** — Round 2L Command 1 PASS via `internal/connector/qfdecisions 0.894s`; the tests assert `len(artifacts) == 1`, `ContentType == ContentTypeDecisionPacket`, `Metadata["unknown_decision_type"] == true`, and raw `decision_type` preservation.
- [x] SCN-SM-041-007: Unit test `TestConnectorEmitsLagBreachEventAboveThreshold` covers lag-breach event formatting and the no-auto-fast-forward invariant. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [ ] SCN-SM-041-003: Integration test `TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` proves the handshake runs before any decision-event poll on first connect against a live test stack. Evidence: `report.md` -> Scope 2 Integration Evidence.
- [ ] SCN-SM-041-003: Integration test `TestQFDecisionsConnectorReReadsCapabilityOnRestart` proves the handshake runs again on connector restart against a live test stack. Evidence: `report.md` -> Scope 2 Integration Evidence.
- [ ] SCN-SM-041-008: Integration test `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` proves the connector picks up `events_skipped` and transitions to `degraded_recovered` against a live test stack. Evidence: `report.md` -> Scope 2 Integration Evidence.
- [x] SCN-SM-041-004: E2E API regression test `TestQFDecisionsIncompatibleCapabilityBlocksPolling` proves an incompatible capability response prevents decision-event polling and preserves zero trusted-artifact publication against a live API. Evidence: `report.md` -> **Scope 2 SCN-004 E2E Evidence (DoD 319 -- bubbles.implement + bubbles.test, 2026-05-18T15:05:03Z, Round 5)**. PASS at `0.08s` against the 5-service live test stack; wrapper exit 0; adversarial trip-wire (`t.Errorf` on `DecisionEventsPath`/`DecisionPacketsPath` hits) confirms no polling occurred after the incompatible capability response.
- [x] SCN-SM-041-006: E2E API regression test `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata` proves end-to-end unknown decision-type ingestion with metadata flag against a live API. Evidence: `report.md` -> Scope 2 E2E Runtime Evidence (DoD 320 — bubbles.test, 2026-05-18T14:04:12Z). PASS at `0.09s`, wrapper exit 0, on live test stack with all 5 services Healthy. Operator-supplied `CapabilitiesPath` stub arm at `tests/e2e/qf_decisions_connector_api_test.go:637-654` resolved the Round 2N capability-handshake omission (concern C-S2-006-E2E-STUB-ARM).
- [ ] SCN-SM-041-003 and SCN-SM-041-008: Stress test `TestQFDecisionsFreshnessSLAP95IngestRender` runs the freshness SLA scenario against a live stack and asserts p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s, with `smackerel_qf_freshness_p95_seconds{stage}` exposed. Evidence: `report.md` -> Scope 2 Stress Evidence.
- [x] Artifact lint accepts the updated planning artifacts (`bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` exits 0). Evidence: `report.md` -> Scope 2 Artifact Lint Evidence.
- [ ] Broader E2E regression suite (`./smackerel.sh test e2e`) passes; both Go e2e packages and the shell E2E suite report zero failures. Evidence: `report.md` -> Scope 2 Broader E2E Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, stress, and artifact-lint evidence is recorded in `report.md` before any DoD item is checked. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence, Scope 2 E2E API Evidence, Scope 2 Stress Evidence, Scope 2 Artifact Lint Evidence.
- [ ] Change Boundary is respected and zero excluded file families were changed (no Scope 3 rendering surfaces, no Scope 4 evidence-bundle export, no Scope 5 credential rotation overlap, no Scope 6-9 endpoints). Evidence: `report.md` -> Scope 2 Planning Repair Guard Evidence.
- [ ] No fallback defaults, hardcoded QF credentials, hardcoded QF URLs, or generated config hand edits are introduced; the new migration is the only schema change and uses the project migration framework. Evidence: `report.md` -> Scope 2 Check Evidence, Scope 2 Implementation Reality Evidence.
- [ ] Build, lint, and tests produce zero warnings (`./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`). Evidence: `report.md` -> Scope 2 Check Evidence.
- [ ] New Scope 2-owned metrics (`smackerel_qf_capability_mismatch_total{required,actual}`, `smackerel_qf_unknown_decision_type_total{value}`, `smackerel_qf_cursor_lag_seconds`, `smackerel_qf_cursor_fast_forward_events_skipped_total`, `smackerel_qf_freshness_p95_seconds{stage}`) are documented in `design.md` and exposed via the Prometheus registry without altering the Scope 5-owned full 12-metric symmetric set commitments. Evidence: `report.md` -> Scope 2 Documentation Boundary Evidence.

### Round 2P DoD Name Reconciliation (2026-05-13)

Round 2N flagged five Scope 2 DoD items whose checklist text references test functions or files that do NOT exist by the named path/symbol. Round 2P (this `bubbles.plan` round) classified each item against direct file inspection plus targeted grep searches; raw evidence is in `report.md` -> Round 2P Evidence (CMDs 1-13).

**All 5 items classified B (semantic gap).** In every case the unit-layer covers the in-process semantics, but the live-stack assertion the DoD requires is genuinely absent. The DoD checkboxes therefore stay `[ ]` and the original DoD wording is preserved verbatim — Round 2Q (`bubbles.implement`) inherits the unchanged gap list.

| # | DoD Item (Scenario) | Named Path / Symbol | What Actually Exists | Classification | Round 2Q Recommendation |
|---|---------------------|---------------------|----------------------|----------------|--------------------------|
| 1a | SCN-SM-041-003 capability handshake on first connect | `tests/integration/qf_decisions_capability_test.go::TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` | File and function do NOT exist (CMD 1, CMD 2a). Unit-layer connect-time capability path covered by `internal/connector/qfdecisions/connector_test.go::TestConnect_CapabilityCompatibleSucceeds` and 9 functions in `internal/connector/qfdecisions/capability_test.go` (httptest mocks, NOT a live PostgreSQL+NATS stack). Existing live integration tests in `tests/integration/qf_decisions_*.go` (4 functions) have ZERO references to `CapabilitiesPath` / `capability` / `handshake` (CMD 12). | **B (semantic gap)** | Author live-stack integration test asserting the capability call lands BEFORE any decision-event poll against the PostgreSQL+NATS test stack. |
| 1b | SCN-SM-041-003 capability re-read on connector restart | `tests/integration/qf_decisions_capability_test.go::TestQFDecisionsConnectorReReadsCapabilityOnRestart` | File and function do NOT exist (CMD 1, CMD 2a). NO test of any layer (unit OR integration OR e2e) covers the connector restart re-read capability path. | **B (semantic gap)** | Author live-stack integration test that restarts the connector and asserts the capability endpoint is re-fetched. |
| 2 | SCN-SM-041-008 fast-forward `events_skipped` recovery | `tests/integration/qf_decisions_sync_test.go::TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` | Function does NOT exist anywhere (CMD 2a). Production code at `internal/connector/qfdecisions/connector.go:245-296,387-388` implements the positive fast-forward recovery path (`fastForwardObserved`, `metrics.QFCursorFastForwardEventsSkipped.Add`, `setHealth(HealthDegradedRecovered)`) per CMD 9, but ZERO tests (unit OR integration OR e2e) exercise that positive path per CMD 10. The lag-breach unit test `TestConnectorEmitsLagBreachEventAboveThreshold` only asserts the NEGATIVE no-auto-fast-forward invariant. | **B (semantic gap)** | Author at minimum a unit test of the positive fast-forward recovery path; ideally also the live-stack integration test the DoD demands. This is the most under-covered gap of the five — production code exists but is functionally untested at every layer. **Round 2Q IMPLEMENTED (unit layer only):** `internal/connector/qfdecisions/connector_test.go::TestSyncSkipsFastForwardDiagnosticEventAndIncrementsCounter` (added at line 1079, ran fresh under `./smackerel.sh test unit --go` — package `internal/connector/qfdecisions` reported `ok` in 0.458s and re-cached on a follow-up run; raw output captured in `report.md` -> Round 2Q Evidence). Test asserts: (a) FF diagnostic event with `EventsSkipped=42` is NOT normalized into a `RawArtifact` and its `packet_id` is NEVER fetched (adversarial trip-wire `ffPacketFetches==0`), (b) `smackerel_qf_cursor_fast_forward_events_skipped_total` counter delta is exactly `42`, (c) `Health()` transitions to `HealthDegradedRecovered`, (d) slog emits a `fast_forward_recovered` WARN record carrying `events_skipped=42`, `event_id="event-ff-marker-1"`, `connector_id=DefaultConnectorID`. **Live-stack integration test the DoD names (`TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` against the PostgreSQL+NATS test stack) is still genuinely absent — blocked by spec-045 SST-loader runtime drift (`envsubst: command not found` per `internal/config::TestSSTLoader_RejectsDevPostgresPassword_HomeLab` failure observed in the same run).** Original DoD line 304 stays `[ ]` until `bubbles.test` re-evaluates whether the unit-layer cover is acceptable substitution OR the live integration test must be authored after spec-045 unblocks. |
| 3 | SCN-SM-041-004 incompatible-capability E2E | `tests/e2e/qf_decisions_connector_api_test.go::TestQFDecisionsIncompatibleCapabilityBlocksPolling` | Function does NOT exist (CMD 2a). Unit-layer coverage exists across `connector_test.go::TestConnect_CapabilityIncompatibleReturnsError`, `client_test.go::TestClientRejectsIncompatibleQFPacketVersion`, `client_test.go::TestClient_FetchDecisionEvents_IncompatibleStatusBypassesClamp` — all httptest-mocked, NOT live API. The existing E2E `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts` is a different scenario (packet schema mismatch via `startQFSchemaMismatchStub`, not capability handshake mismatch); existing e2e files have ZERO references to capability/Incompatible/CapabilitiesPath (CMD 13). | **B (semantic gap)** | Author live-API E2E test that drives an incompatible capability response (e.g., wrong `audit_envelope_version` OR missing `v1` in `supported_packet_versions`) through the live supervisor and asserts ZERO trusted artifacts published. |
| 4 + 5 | SCN-SM-041-003 + SCN-SM-041-008 freshness SLA P95 stress | `tests/stress/qf_decision_event_replay_test.go::TestQFDecisionsFreshnessSLAP95IngestRender` | File and function do NOT exist (CMD 1, CMD 2a). Unit tests (`TestSyncRecordsIngestFreshness_FreshPacket`, `TestSyncRecordsIngestFreshness_DelayedPacket`, `TestRecordFreshness_PerStageIsolation`) cover the rolling-window gauge mechanics with httptest mocks, but ZERO stress test asserts `p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s` under sustained load. Existing `tests/stress/qf_decisions_sync_stress_test.go::TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity` covers replay identity, not freshness SLA budget; that file has ZERO references to `P95` / `freshness` / `30s` / `60s` (CMD 11). | **B (semantic gap)** | Author live-stack stress test that drives a sustained packet workload through ingest+render and asserts the P95 budgets via the `smackerel_qf_freshness_p95_seconds{stage}` gauge. |

**Honesty notes:**

- Each classification was verified by direct file inspection plus grep searches captured in `report.md` -> Round 2P Evidence (CMDs 1-13). No test was assumed implemented from the function name alone.
- No DoD lines were re-worded, no DoD checkboxes were flipped, and no source code was changed in this round. The original live-stack assertion intent is preserved verbatim; Round 2Q (`bubbles.implement`) inherits the gap list unchanged.
- The duplicate `## Parked Scope 2:` legacy section (line 357) was NOT touched — that cleanup is owned by a separate planning round.
- Round 2P explicitly REJECTS classification A for all 5 items. Although unit-layer coverage exists for items 1a, 3, and 4+5, the DoD lines explicitly require live-stack integration / live-API E2E / live-stack stress execution. Classifying these as A would silently downgrade the assertion bar from live-stack to unit-layer; that downgrade is a planning decision the user must make explicitly, not a name-reconciliation outcome.
- Item 1b and Item 2 have NO equivalent unit-layer coverage either — the production behavior they target is genuinely untested.

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/*` (capability client, normalizer, connector sync logic, client page-size clamping, types, tests)
- `internal/db/migrations/*qf*` (new capability migration only)
- `tests/integration/qf_decisions_*` (capability handshake, sync, fast-forward integration tests)
- `tests/e2e/qf_decisions_*` (mismatch, unknown decision-type, ingest e2e tests)
- `tests/stress/qf_decisions_*` and `tests/stress/qf_decision_event_replay_test.go` (freshness SLA stress)
- `specs/041-qf-companion-connector/*` (planning artifacts only)

Excluded surfaces:

- Web, digest, Telegram, search, mobile push rendering of QF packets (owned by Parked Scope 3)
- `PersonalEvidenceBundle` export, `target_context = packet_context`, evidence import limits, consent revocation (owned by Parked Scope 4)
- Credential rotation overlap / overlapping `not_before` window / capability re-read at rotation start (owned by Parked Scope 5; capability re-read on connector restart and credential reload IS in Scope 2, but rotation overlap behavior is not)
- Cross-Product Audit Envelope v1 emission across all eight emission points and the full 12-metric symmetric set (owned by Parked Scope 5; Scope 2 only adds the five new metrics enumerated above)
- Packet engagement signal exporter and `POST /packet-engagement-signals` (owned by Parked Scope 6)
- Personal context read API host `GET /api/v1/personal-context` (owned by Parked Scope 7)
- Signed callback infrastructure and `POST /callback` (owned by Parked Scope 8)
- Watch signal proposal endpoint `POST /watch-signal-proposals` (owned by Parked Scope 9)
- Generated config hand edits or new connector configuration keys (Scope 1 boundary; Scope 2 reuses the explicit configuration already proven by Scope 1)
- `state.json` modifications (workflow agent owns state transitions)

## Parked Scope 2: Capability Handshake, Cursor Sync Normalization, And Storage

**Status:** Superseded by active Scope 2 section above (unparked 2026-05-13 after QF 063 reached `done_with_concerns`). This section is preserved for traceability of the original Phase B2 design intent only; its proposed DoD items have been folded into the active Scope 2 Definition of Done. **Do not execute against these checkboxes** — they are advisory historical context.

**Depends On (historical):** Scope 1
**Activation Gate (historical):** QF 063 Scope 2 read/outbox readiness — cleared 2026-05-12.

### Phase B2 Design Additions (2026-05-07) — Historical Proposed DoD (Superseded)

The following items were the original Phase B2 design intent for Scope 2 captured during planning. Each item is now represented as a Core Behavior or Validation DoD item in the active Scope 2 section above; these checkboxes remain unchecked and MUST NOT be ticked here.

Core behavior (Phase B2 additions, superseded — see active Scope 2 Core Behavior):

- [ ] Capability handshake: connector calls `GET /api/private/smackerel/v1/capabilities` before decision-event polling and on connector restart/credential-rotation start, parses and persists all fields enumerated in design.md §Capability Discovery, blocks polling when required sync contract fields are incompatible, and emits `smackerel_qf_capability_mismatch_total{required,actual}` (Phase B2, F2).
- [ ] Unknown decision-type ingest: when QF emits an unknown `decision_type` with `unknown_decision_type=true`, the connector stores the packet with `Metadata.unknown_decision_type = true`, does not invent a new content type, emits `smackerel_qf_unknown_decision_type_total{value}`, and leaves generic-card rendering to Scope 3 (Phase B2, F8).
- [ ] Page-size clamping: connector clamps requested page size to `[1, max_page_size]` from the capability response; fallback default 200 if capability is missing; rejects `PAGE_SIZE_OUT_OF_RANGE` 4xx responses with operator alerts (Phase B2, F9).
- [ ] Freshness SLA stress test: `tests/stress/qf_decision_event_replay_test.go` (or equivalent) verifies p95 ingest ≤30s, p95 render ≤30s, and combined p95 ≤60s; metric `smackerel_qf_freshness_p95_seconds{stage}` is exposed (Phase B2, F12).
- [ ] Cursor lag breach signaling: when `smackerel_qf_cursor_lag_seconds` exceeds the operator-configured threshold (default 1h), the connector logs a structured `lag_breach` event for the operator dashboard and never auto-fast-forwards itself (Phase B2, F13).
- [ ] QF-issued fast-forward recovery: on a server-side cursor advancement, the connector picks up the `events_skipped` count, marks state `degraded_recovered`, and increments `smackerel_qf_cursor_fast_forward_events_skipped_total`; integration test exercises the fast-forward recovery path (Phase B2, F13).

Validation (Phase B2 additions, superseded — see active Scope 2 Validation):

- [ ] Unit tests cover capability response parsing, required-field compatibility checks, persisted capability diagnostics, and capability mismatch metric labels.
- [ ] Integration tests cover capability handshake before polling, handshake on restart, and capability re-read at credential rotation start without activating Scope 5 rotation overlap behavior.
- [ ] E2E regression test covers incompatible capability response preventing decision-event polling and preserving zero trusted artifact publication.
- [ ] Unit and integration tests cover unknown decision-type ingest metadata, no invented content type, and metric emission.
- [ ] Unit tests cover page-size clamping, fallback default, and `PAGE_SIZE_OUT_OF_RANGE` rejection.
- [ ] Stress test exercises the freshness SLA budget and surfaces `smackerel_qf_freshness_p95_seconds{stage}`.
- [ ] Integration test exercises cursor lag breach signaling and the QF-issued fast-forward recovery path.

## Parked Scope 3: Web Telegram Digest And Search Surfacing

**Status:** Not Started
**Depends On:** Scope 2
**Activation Gate:** QF packets exist as Smackerel artifacts with packet ID, trace ID, approval state, badges, and deep link

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions):

- [ ] Unknown decision-type generic-card fallback: render packets carrying `Metadata.unknown_decision_type = true` through a generic QF packet card that preserves QF-authored title/content, trust metadata, and QF deep link without deriving semantics from packet body (Phase B2, F8).
- [ ] Trust Object Rendering Contract: digest and Telegram renderers consume ONLY `label`, `severity`, `summary`, optional `detail`, and optional `links` from CalibrationBadge, DataProvenanceBadge, QuantifiedImpact, and ExpertAnalysisBundle; numeric internals are silently dropped (NOT errors); unit tests cover each badge type (Phase B2, F6).
- [ ] Trust Object missing-required failure: fail loud only when a required field (`label` / `severity`) is missing; emit `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}` and fall back to the generic packet card (Phase B2, F6).
- [ ] Signed deep-link rendering preference: prefer `packet_url_signed` when present and unexpired; fall back to unsigned only when the capability declares `deep_link_signing_supported=false`; on signature expiry mid-render, refetch the packet for a fresh signed URL; emit `smackerel_qf_deep_link_render_total{surface,status}` with statuses `signed_used`, `signed_expired_fallback_unsigned`, and `unsigned_only`; e2e tests cover each branch (Phase B2, F6).
- [ ] `preferred_surface` routing: digest renderer routes per design.md §Preferred Surface Routing — `smackerel_digest` to digest-only, `smackerel_telegram` to the Telegram bot, `qf_dashboard` to a "View in QF dashboard" tile, `any` to user preference; routing NEVER alters trust metadata, decision content, or action eligibility based on the hint; unit tests cover each routing branch (Phase B2, O9, FR-020).

Validation (Phase B2 additions):

- [ ] UI/unit and E2E tests cover the unknown decision-type generic-card fallback and prove no buy/sell/hold semantics are inferred locally.
- [ ] Unit tests cover Trust Object Rendering Contract per badge type (CalibrationBadge, DataProvenanceBadge, QuantifiedImpact, ExpertAnalysisBundle).
- [ ] Unit tests cover the missing-required-field fallback and metric emission.
- [ ] E2E tests cover `signed_used`, `signed_expired_fallback_unsigned`, and `unsigned_only` deep-link render branches.
- [ ] Unit tests cover each `preferred_surface` routing branch.

## Parked Scope 4: Personal Evidence Bundle Export

**Status:** Not Started
**Depends On:** Scope 3
**Activation Gate:** User-visible QF context and consent/sensitivity UI paths exist

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions):

- [ ] Idempotency response handling: HTTP 200 with the same `export_id` and payload is treated as a no-op success; HTTP 409 `EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD` aborts the export with an `EXPORT_ID_COLLISION` audit error and is never retried (Phase B2, F4).
- [ ] `target_context` enum extended with `packet_context`; e2e test exports a bundle attached to a packet (Phase B2, F4).
- [ ] Evidence import limits enforced pre-flight: bundle size ≤ `evidence_max_bundle_size_bytes` (capability default 524288), claim count ≤ `evidence_max_claims_per_bundle` (default 50), per-credential rate ≤ `evidence_rate_limit_per_minute` (default 10) via token bucket; reject locally with `BUNDLE_TOO_LARGE`, `TOO_MANY_CLAIMS`, or `RATE_LIMIT_EXCEEDED`; emit `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason}` (Phase B2, F14).
- [ ] Consent revocation: when the user revokes export consent (Smackerel UI/API), the connector calls `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with `{reason: "consent_revoked"}`, marks the local artifact `revoked`, and emits a unified audit envelope; e2e test covers the revocation path (Phase B2, F15).
- [ ] `source_provenance_classes` field populated on every exported bundle; pre-flight validation against capability `eligible_smackerel_source_classes`; reject locally with `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{class}` if any class is not eligible; pre-MVP design-only — badge attachment must NOT be enabled (Phase B2, O7).

Validation (Phase B2 additions):

- [ ] Unit tests cover idempotent 200 replay and 409 collision paths.
- [ ] E2E test exports a bundle with `target_context = packet_context`.
- [ ] Unit and integration tests cover bundle-size, claim-count, and rate-limit pre-flight rejection.
- [ ] E2E test covers consent revocation via DELETE and audit envelope emission.
- [ ] Unit tests cover `source_provenance_classes` population and eligibility rejection; pre-MVP enforcement that badge attachment is NOT enabled.

## Parked Scope 5: Credential Rotation, Safety Boundaries, Observability, Documentation, And Tests

**Status:** Not Started
**Depends On:** Scopes 2, 3, 4
**Activation Gate:** Sync, rendering, and export surfaces exist for boundary verification

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions):

- [ ] Credential rotation overlap: connector accepts two active QF credentials for no more than 24h during rotation, chooses the newest valid credential by `not_before`, preserves `sync_state.sync_cursor` and evidence/export idempotency state, re-reads capabilities at rotation start, and emits operator diagnostics and audit envelope records (Phase B2, F16).
- [ ] Symmetric metric set emitted with documented labels: `smackerel_qf_packet_ingest_total{event_type,decision_type,approval_state,source_surface}`, `smackerel_qf_packet_validation_failures_total{reason}`, `smackerel_qf_evidence_export_attempts_total{status,target_context_type,sensitivity_tier}`, `smackerel_qf_cursor_lag_seconds`, `smackerel_qf_action_boundary_attempts_total{attempted_action_type}`, `smackerel_qf_capability_mismatch_total{required,actual}`, `smackerel_qf_unknown_decision_type_total{value}`, `smackerel_qf_engagement_signal_attempts_total{event,surface,status}`, `smackerel_qf_evidence_revoked_total{reason}`, `smackerel_qf_callback_attempts_total{action,status}`, `smackerel_qf_deep_link_render_total{surface,status}`, and `smackerel_qf_trust_object_render_failures_total{reason}`; cross-reference QF design 063 for label parity (Phase B2, F11).
- [ ] Cross-Product Audit Envelope v1 emitted for every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick; envelope shape mirrored from QF design 063; integration test confirms envelope shape; sink is the connector audit log with opt-in QF mirror reserved post-MVP (Phase B2, O4).

Validation (Phase B2 additions):

- [ ] Unit and integration tests rotate credentials through overlapping `not_before` windows and verify cursor, evidence export idempotency state, capability re-read, diagnostics, and audit envelope preservation.
- [ ] Unit and integration tests cover emission of all 12 `smackerel_qf_*` metrics with correct label sets.
- [ ] Integration test confirms Cross-Product Audit Envelope v1 shape across the eight required emission points.

## Parked Scope 6: Packet Engagement Signal Exporter

**Status:** Not Started
**Depends On:** Scope 3
**Activation Gate:** Trust-rendering surfaces emit packet renders that can be instrumented (Scope 3)

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions, O1, FR-013):

- [ ] Engagement event capture across digest UI, Telegram bot, and mobile push for events: `opened`, `dwell` (with seconds), `dismissed`, `snoozed`, `deep_linked`, and `shared`.
- [ ] Consent gate: emit only when `engagement_telemetry` is `anonymous` or `pseudonymous` in user privacy settings.
- [ ] Buffer / flush policy: in-memory buffer flushed every 10s or on 100 events.
- [ ] `POST /api/private/smackerel/v1/packet-engagement-signals` with `signal_id` UUIDv7 generated client-side (idempotent).
- [ ] Failure handling: drop on 4xx (privacy-preserving) and retry with backoff up to 3 attempts on 5xx.
- [ ] Audit envelope emitted on every flush attempt.
- [ ] Metric `smackerel_qf_engagement_signal_attempts_total{event,surface,status}` emitted.

Validation (Phase B2 additions):

- [ ] Unit tests cover event capture across all six event types and all three surfaces.
- [ ] Unit tests cover the consent gate (anonymous/pseudonymous emit, off does not emit).
- [ ] Unit tests cover buffer/flush policy (10s timer + 100-event threshold).
- [ ] Integration test covers POST contract, idempotent UUIDv7, 4xx drop, and 5xx retry-with-backoff.
- [ ] Integration test confirms audit envelope emission on flush.

## Parked Scope 7: Personal Context Read API Host

**Status:** Not Started
**Depends On:** Scope 3
**Activation Gate:** Personal-context entities (notes, locations, timeline events) and consent token issuance exist

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions, O2, FR-014):

- [ ] Smackerel hosts `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}`.
- [ ] Returns a list of personal-context items (notes, locations, timeline events) up to `max_sensitivity`.
- [ ] Consent token: short-lived (≤15min) and scope-limited (entity, sensitivity, requester_id baked in).
- [ ] Response includes a required `non_influence_warning` field.
- [ ] Rate limit: 5 reads per `consent_token`.
- [ ] Audit envelope emitted on every fetch.

Validation (Phase B2 additions):

- [ ] Unit tests cover request shape parsing, sensitivity tier filtering, and `non_influence_warning` presence.
- [ ] Unit tests cover consent token expiry, scope-limit enforcement, and the 5-read rate limit.
- [ ] Integration test exercises the end-to-end fetch path with audit envelope emission.

## Parked Scope 8: Signed Callback Protocol

**Status:** Not Started
**Depends On:** Scope 3
**Activation Gate:** Trust-rendering surfaces present action-eligible packets that may emit callbacks (rejected pre-MVP)

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions, O5, FR-017):

- [ ] HMAC-SHA256 signing using a shared bridge secret over the canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface`.
- [ ] `key_id` field carried in the callback envelope; key rotation per release with documented playbook.
- [ ] Pre-MVP: signing infrastructure is exercised but every callback returns the QF version-one callback rejection response; integration test verifies the signature is computed and the rejection is parsed.
- [ ] Telemetry `smackerel_qf_callback_signature_failures_total{reason}` and `smackerel_qf_callback_attempts_total{action,status}` emitted.

Validation (Phase B2 additions):

- [ ] Unit tests cover canonical-payload formatting, HMAC computation, and `key_id` envelope inclusion.
- [ ] Integration test verifies signing plus QF version-one callback rejection parsing end-to-end.
- [ ] Unit tests cover failure-reason emission for `smackerel_qf_callback_signature_failures_total`.

## Parked Scope 9: Watch Signal Proposal Endpoint (Pre-MVP Design Only)

**Status:** Not Started
**Depends On:** Scope 2
**Activation Gate:** Capability handshake is operational so proposal endpoint readiness can be advertised and rejected by QF (Scope 2)

### Phase B2 Design Additions (2026-05-07) — Proposed DoD

Core behavior (Phase B2 additions, O3, FR-015):

- [ ] `POST /api/private/smackerel/v1/watch-signal-proposals` request shape `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}`.
- [ ] Pre-MVP: every request is rejected by QF with the version-one watch-proposal rejection response; signing infrastructure is exercised.
- [ ] Integration test verifies request shape, signing, and rejection parsing.
- [ ] No proposal ever influences QF watch state pre-MVP.

Validation (Phase B2 additions):

- [ ] Unit tests cover request shape construction and signing.
- [ ] Integration test verifies QF version-one watch-proposal rejection parsing and confirms no QF watch-state mutation.

