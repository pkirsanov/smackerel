# Scopes: QF Companion Connector

## Execution Outline

### Phase Order

1. Scope 1: Connector configuration and QF client contract - add the `qf-decisions` connector boundary, config requirements, client DTOs, and health/schema checks.
2. Scope 2: Cursor sync normalization and storage - poll QF events, fetch packet envelopes, validate required trust metadata, and publish source-qualified artifacts.
3. Scope 3: Web Telegram digest and search surfacing - render QF packets read-only across Smackerel surfaces without approval, execution, mandate, watch, or advice controls.
4. Scope 4: Personal evidence bundle export - let users export selected Smackerel context to QF as consent-scoped `PersonalEvidenceBundle`s.
5. Scope 5: Safety boundaries observability docs and tests - prove action gates, connector diagnostics, release boundaries, and cross-repo compatibility.

### New Types And Signatures

- Connector ID: `qf-decisions`.
- Package boundary: `internal/connector/qfdecisions`.
- Configuration keys: `connectors.qf-decisions.enabled`, `base_url`, `credential_ref`, `sync_schedule`, `packet_version`, and `page_size`.
- QF DTO mirrors: `QFDecisionEvent`, `QFDecisionPacketEnvelope`, `PersonalEvidenceBundle`, and reserved action/watch diagnostics.
- Artifact content types: `qf/decision-packet`, `qf/no-action-decision`, `qf/policy-denial`, and reserved diagnostic `qf/approval-request`.
- Artifact identity: `RawArtifact.SourceID = qf-decisions`, `RawArtifact.SourceRef = packet_id`, QF trace and trust metadata preserved in artifact metadata.
- Evidence export path: user-selected Smackerel context to `PersonalEvidenceBundle` to QF private-alpha import path.

### Validation Checkpoints

- After Scope 1, config and client tests prove the connector cannot start without explicit QF base URL, credential reference, packet version, and page size.
- After Scope 2, integration and E2E API tests prove cursor sync stores QF packets without changing packet IDs, trace IDs, approval state, or trust badges.
- After Scope 3, E2E UI tests prove Web, Telegram, digest, and search render QF packets as QF-authored read-only artifacts with no action controls.
- After Scope 4, export tests prove evidence bundles include bundle/export IDs, `created_at`, source artifact IDs, claims, sensitivity, consent, provenance, redaction summary, target context, optional `source_refs` semantics, and QF import status.
- After Scope 5, safety, observability, documentation, regression, and artifact checks pass before any completion claim.

## Overview

This plan implements the Smackerel side of the pre-MVP QF companion integration. Smackerel acts as a passive connector, memory, attention, digest, search, Web, and Telegram surface. QF remains the system of record and financial decision authority.

The connector must never generate financial advice, approve trades, change mandates, execute, upgrade trust badges, hide downgraded QF metadata, or treat QF packets as Smackerel-local recommendations. Reverse flow is limited to user-initiated, consent-scoped `PersonalEvidenceBundle` export.

Post-MVP capabilities such as QF-supported approvals, standing watches, tenant-aware access, voice parity, EmergencyStop parity, and paper/live execution are release follow-up only. They are not part of pre-MVP DoD.

## Scope Inventory

| Scope | Name | Surfaces | Required Tests | DoD Summary | Status |
|-------|------|----------|----------------|-------------|--------|
| 1 | Connector configuration and QF client contract | Config generation, connector registry, QF client DTOs | Unit, integration, e2e-api regression | Connector starts only with explicit config and compatible QF contract | [x] Done |
| 2 | Cursor sync normalization and storage | Connector supervisor, state store, artifact pipeline, PostgreSQL | Unit, integration, e2e-api, stress | QF packets become source-qualified artifacts with metadata preserved | [ ] Not started |
| 3 | Web Telegram digest and search surfacing | Web, Telegram, digest, search, artifact detail | UI unit, e2e-ui, e2e-api regression | Packets render read-only with QF trust labels and no action controls | [ ] Not started |
| 4 | Personal evidence bundle export | Web evidence selection, bundle builder, QF export client, export status | Unit, integration, e2e-api, e2e-ui, security | User exports consent-scoped context bundles to QF with provenance | [ ] Not started |
| 5 | Safety observability docs and tests | Health diagnostics, logs, metrics, docs, release follow-up | Integration, e2e-api, e2e-ui, stress, artifact lint | Safety boundary, diagnostics, docs, and compatibility are verified | [ ] Not started |

## Scope 1: Connector Configuration And QF Client Contract

Status: [x] Done  
Depends On: None

### Business Scenarios

#### SCN-SM-041-001: Connector Starts With Explicit Configuration

Given a Smackerel operator enables `qf-decisions`  
When the connector starts  
Then it requires explicit QF base URL, credential reference, sync schedule, packet version, and page size from Smackerel configuration.

#### SCN-SM-041-002: Connector Rejects Missing Or Incompatible QF Contract

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
- Change Boundary: connector configuration, registry, client contract, and health checks only; no artifact publication or UI surfacing in this scope.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-001 | `internal/connector/qfdecisions/connector_test.go` | `validates qf-decisions connector configuration before connect` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-002 | `internal/connector/qfdecisions/client_test.go` | `rejects incompatible QF packet version during client validation` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-001, SCN-SM-041-002 | `tests/integration/qf_decisions_connector_config_test.go` | `registers qf-decisions and reports health for explicit config outcomes` | `./smackerel.sh test integration` | Yes |
| E2E API Regression | e2e-api | SCN-SM-041-002 | `tests/e2e/qf_decisions_connector_api_test.go` | `Regression: qf-decisions does not publish artifacts when QF schema is incompatible` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-001 | `specs/041-qf-companion-connector` | `artifact lint accepts QF connector planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |

### Definition of Done

Core behavior:

- [x] `qf-decisions` is registered as a passive connector with explicit configuration owned by `config/smackerel.yaml` and generated env output. Evidence: `report.md` → Scope 1 Integration Evidence, Scope 1 Check Evidence, Code Diff Evidence.
- [x] Connector startup fails for missing base URL, credential reference, packet version, sync schedule, page size, invalid URL, or invalid page size. Evidence: `report.md` → Scope 1 Unit Evidence, Scope 1 Integration Evidence.
- [x] QF client DTOs mirror QF spec 063 field names for packet IDs, trace IDs, approval state, badges, deep links, `decision_type`, and evidence bundles, including required `target_context` and optional `source_refs` semantics. Evidence: `report.md` → Code Diff Evidence, Scope 1 Unit Evidence, RED Proof Note.
- [x] The connector uses HTTP polling/read surface only; no direct QF database access, broker federation, or embedded credentials. Evidence: `report.md` → Scope 1 Unit Evidence, Scope 1 Implementation Reality Evidence.

Validation:

- [x] Unit tests cover configuration validation and QF client contract compatibility. Evidence: `report.md` → Scope 1 Unit Evidence.
- [x] Integration tests prove registry startup and health transitions for valid config, auth failure, and schema mismatch. Evidence: `report.md` → Scope 1 Integration Evidence.
- [x] E2E API regression test proves incompatible schema does not publish trusted packet artifacts. Evidence: `report.md` → Scope 1 E2E API Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, and artifact-lint evidence is recorded in `report.md` before DoD items are checked. Evidence: `report.md` → Scope 1 Unit Evidence, Scope 1 Integration Evidence, Scope 1 E2E API Evidence, Scope 1 Artifact Lint Evidence.
- [x] No fallback defaults, hardcoded QF credentials, hardcoded QF URLs, or generated config hand edits are introduced. Evidence: `report.md` → Scope 1 Check Evidence, Scope 1 Implementation Reality Evidence.
- [x] Documentation identifies QF as the system of record and Smackerel as a companion connector. Evidence: `report.md` → Scope 1 Documentation Boundary Evidence.

## Scope 2: Cursor Sync Normalization And Storage

Status: [ ] Not started  
Depends On: Scope 1

### Business Scenarios

#### SCN-SM-041-003: QF Packet Sync Preserves Identity And Trust

Given QF exposes a decision event and packet envelope  
When `qf-decisions` syncs by cursor  
Then Smackerel stores a source-qualified artifact with the exact QF packet ID, intent ID, scenario ID, trace ID, approval state, deep link, `CalibrationBadge`, and `DataProvenanceBadge`.

#### SCN-SM-041-004: Degraded Packet Does Not Become Trusted Card

Given QF sends an event whose packet is missing a trust badge, trace ID, approval state, deep link, or known packet version  
When Smackerel normalizes the packet  
Then the connector records diagnostics and health degradation without rendering a trusted or actionable packet card.

#### SCN-SM-041-005: Cursor Replay Does Not Duplicate QF Packet Identity

Given an operator replays or clears the `qf-decisions` cursor  
When Smackerel syncs QF packets again  
Then existing QF packet IDs remain stable and duplicate packet identities are not created.

### Implementation Plan

- Implement connector `Sync(ctx, cursor)` using the existing supervisor and state store.
- Poll QF decision events with opaque cursor, explicit page size, and requested packet version.
- Fetch packet envelopes when the event list does not inline them.
- Validate all required QF fields before producing trusted artifacts.
- Normalize valid packets into `RawArtifact` with `SourceID = qf-decisions`, `SourceRef = packet_id`, QF content types, raw envelope content, and exact metadata objects.
- Map QF `decision_type` to Smackerel content type per the cross-repo design table: `recommendation` → `qf/decision-packet`, `no_action` → `qf/no-action-decision`, `policy_denial` → `qf/policy-denial`, and `analysis_note` → `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"` preserved verbatim. Do NOT introduce additional `qf/...` content types pre-MVP.
- Persist the response-level `next_cursor` in `sync_state.sync_cursor` as the canonical advancement value. Treat per-event `QFDecisionEvent.cursor` as a diagnostic checkpoint for partial-page resumption only; never use it for normal advancement.
- Persist cursor state through existing PostgreSQL-backed `sync_state` behavior.
- Store degraded diagnostics without emitting a financial recommendation or trusted packet card.
- Change Boundary: connector sync, normalization, state, and artifact publication only; no Web/Telegram/digest/search UI changes yet.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-003, SCN-SM-041-004 | `internal/connector/qfdecisions/normalizer_test.go` | `preserves QF trust metadata and rejects incomplete packet envelopes` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-005 | `internal/connector/qfdecisions/connector_test.go` | `returns opaque QF cursor without rewriting local packet identity` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-003, SCN-SM-041-005 | `tests/integration/qf_decisions_sync_test.go` | `syncs QF packets through StateStore and ArtifactPublisher with stable packet IDs` | `./smackerel.sh test integration` | Yes |
| E2E API | e2e-api | SCN-SM-041-003, SCN-SM-041-004 | `tests/e2e/qf_decisions_connector_api_test.go` | `ingests QF packet and retrieves it through Smackerel recent search and detail APIs` | `./smackerel.sh test e2e` | Yes |
| Stress | stress | SCN-SM-041-005 | `tests/stress/qf_decisions_sync_stress_test.go` | `repeated QF cursor pages do not duplicate packet IDs or lose trace metadata` | `./smackerel.sh test stress` | Yes |

### Definition of Done

Core behavior:

- [ ] Sync persists the response-level `next_cursor` in `sync_state.sync_cursor` exactly; per-event `QFDecisionEvent.cursor` is treated as diagnostic-only and is not used for normal advancement.
- [ ] Valid QF packets are normalized into source-qualified Smackerel artifacts with exact QF IDs, trace ID, approval state, badges, and deep link.
- [ ] Content-type normalization matches the QF design's "Decision Type To Smackerel Content Type Mapping" table: `recommendation` → `qf/decision-packet`, `no_action` → `qf/no-action-decision`, `policy_denial` → `qf/policy-denial`, `analysis_note` → `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"` preserved; no other `qf/...` content type is introduced pre-MVP.
- [ ] Missing trust metadata, unknown packet version, missing trace, missing approval state, or missing deep link results in degraded diagnostics rather than trusted rendering.
- [ ] Cursor replay and packet updates preserve QF `packet_id` identity and do not create Smackerel-local recommendation identities.

Validation:

- [ ] Unit tests cover normalizer required-field validation, badge preservation, content type mapping, and cursor semantics.
- [ ] Integration tests prove state store, supervisor, and artifact publisher behavior against a QF-compatible live test surface.
- [ ] E2E API test proves ingested QF packets can be read through Smackerel APIs with metadata intact.
- [ ] Stress test proves repeated pages and updates do not duplicate packet IDs or lose trace correlation.

Build quality gate:

- [ ] Raw unit, integration, E2E, stress, and regression-quality guard evidence is recorded in `report.md`.
- [ ] No internal mocks are used to satisfy live integration or E2E DoD items.
- [ ] No cache, local file, or embedded database becomes the source of truth for QF packets.

## Scope 3: Web Telegram Digest And Search Surfacing

Status: [ ] Not started  
Depends On: Scope 2

### Business Scenarios

#### SCN-SM-041-006: User Views QF Packet As Read-Only Artifact

Given Smackerel has synced a valid QF packet  
When the user opens the packet from Web, digest, search, or Telegram  
Then the surface shows QF source, packet ID, trace ID, approval state, trust badges, and QF deep link without action controls.

#### SCN-SM-041-007: Smackerel Does Not Rewrite QF Decision Content

Given a QF packet has QF-authored thesis, why-now, approval state, and trust badges  
When Smackerel summarizes or displays the packet  
Then Smackerel keeps QF-authored decision text separate from Smackerel context notes and does not generate buy/sell/hold advice.

#### SCN-SM-041-008: Search Finds QF Packets Without Exposing Hidden Sensitive Context

Given QF packet metadata and related Smackerel concepts are indexed  
When the user searches by symbol, thesis, packet ID, trace ID, or related concept  
Then results distinguish QF-authored packets from Smackerel-derived context and do not expose hidden sensitive evidence.

### Implementation Plan

- Add read-only QF packet card rendering in Web or source-qualified artifact detail surface.
- Add Telegram-safe summary formatting with QF source label, thesis excerpt, badge status, approval state, trace ID, and QF deep link.
- Add digest inclusion that preserves QF labels and respects quiet/sensitivity policies without rewriting QF state.
- Add search indexing and result rendering for QF packet IDs, trace IDs, symbols, thesis, why-now, and related concepts.
- Ensure no approve, execute, mandate, EmergencyStop, paper execution, live execution, or QF watch action controls appear in pre-MVP surfaces.
- Keep Smackerel summaries labeled as context notes, not QF decision authorship.
- UI Scenario Matrix is covered by the scenarios above and the E2E UI test rows below.
- Change Boundary: presentation and read-only retrieval only; no QF action calls and no evidence export yet.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| UI Unit | ui-unit | SCN-SM-041-006, SCN-SM-041-007 | `web/src/**/QFPacketCard.test.tsx` or equivalent | `renders QF packet card with badges trace deep link and no action controls` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-008 | `tests/integration/qf_decisions_search_test.go` | `indexes QF packet metadata for source-qualified search results` | `./smackerel.sh test integration` | Yes |
| E2E API | e2e-api | SCN-SM-041-008 | `tests/e2e/qf_decisions_connector_api_test.go` | `search returns QF packet by packet ID trace ID symbol and concept with source boundary` | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | SCN-SM-041-006, SCN-SM-041-007 | `tests/e2e/qf_decisions_surfaces_test.go` | `shows QF packet read-only in Web digest and Telegram-compatible rendering` | `./smackerel.sh test e2e` | Yes |
| Regression | e2e-ui | SCN-SM-041-006, SCN-SM-041-007 | `tests/e2e/qf_decisions_surfaces_test.go` | `Regression: QF packet surfaces do not show approval execution mandate or EmergencyStop controls` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

Core behavior:

- [ ] Web, digest, Telegram-compatible summary, and search surfaces show QF packets as QF-authored read-only artifacts.
- [ ] Packet ID, trace ID, approval state, calibration badge, data-provenance badge, and QF deep link are visible wherever channel capacity allows.
- [ ] Smackerel summaries do not rewrite QF thesis, why-now, approval state, trust badge severity, stale state, or downgrade wording.
- [ ] Approval, execution, mandate, EmergencyStop, paper execution, live execution, and QF watch controls are absent or disabled with phase-boundary copy.
- [ ] Search distinguishes QF packet content from Smackerel context and does not expose hidden sensitive evidence.

Validation:

- [ ] UI unit tests cover QF packet card rendering and no-action controls.
- [ ] Integration and E2E API tests cover source-qualified search retrieval by packet ID, trace ID, symbol, thesis, and concept.
- [ ] E2E UI tests cover Web, digest, and Telegram-compatible rendering with user-visible assertions for trust metadata and disabled/absent actions.
- [ ] Regression E2E tests prove action controls do not appear and Smackerel does not present QF packets as local recommendations.

Build quality gate:

- [ ] Required E2E files pass regression-quality guard and live-stack authenticity scans.
- [ ] Docker bundle freshness is verified if a Docker-served Web bundle changes.
- [ ] Raw test and validation evidence is recorded in `report.md` before any DoD item is checked.

## Scope 4: Personal Evidence Bundle Export

Status: [ ] Not started  
Depends On: Scope 3

### Business Scenarios

#### SCN-SM-041-009: User Builds A Consent-Scoped Evidence Bundle

Given a user has Smackerel artifacts, concepts, entities, market/news context, notes, or research trails around a symbol or topic  
When the user selects context for QF export  
Then Smackerel builds a `PersonalEvidenceBundle` with bundle/export IDs, source artifact IDs, related symbols/entities, claims, confidence, sensitivity, consent scope, provenance, redaction summary, target context, optional source references, and timestamp.

#### SCN-SM-041-010: Evidence Export Records QF Import Outcome

Given the user confirms a target QF context and consent scope  
When Smackerel posts the bundle to QF  
Then Smackerel records the export ID, QF import status, rejection reason if any, and source artifact references.

#### SCN-SM-041-011: Missing Consent Or Source Artifact IDs Blocks Export

Given selected context lacks explicit consent scope, sensitivity tier, source artifact IDs, claims, provenance, redaction summary, target context, or created timestamp  
When the user attempts to export to QF  
Then Smackerel blocks or marks the export invalid before QF treats it as accepted evidence.

### Implementation Plan

- Add bundle builder for selected artifacts, concepts, entities, market/news context, notes, and cross-source connections.
- Require `bundle_id`, `export_id`, `consent_scope`, `sensitivity_tier`, `source_artifact_ids`, `extracted_claims`, `provenance`, `redaction_summary`, `target_context`, and `created_at`. `source_refs` is optional and is included only when the underlying source has external IDs (URL, message-id, mailbox-id, RSS GUID, etc.). Field set MUST stay aligned with QF spec 063 acceptance criteria so a Smackerel-locally-valid bundle also passes QF import validation.
- Use prompt contracts only to extract/cite claims with source IDs; do not generate financial advice or QF decision wording.
- Add export client call to QF private-alpha import path when configured.
- Persist export status, QF import reference, accepted/rejected status, and rejection reason.
- Add Web evidence export flow with user-visible consent, sensitivity, target context, and final status.
- Change Boundary: evidence bundle export only; no QF decision action, approval, mandate, watch, or execution request.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-009, SCN-SM-041-011 | `internal/connector/qfdecisions/evidence_bundle_test.go` | `builds PersonalEvidenceBundle with consent sensitivity target context provenance and source claims` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-010 | `tests/integration/qf_evidence_export_test.go` | `exports evidence bundle to QF and records accepted or rejected import status` | `./smackerel.sh test integration` | Yes |
| E2E API | e2e-api | SCN-SM-041-009, SCN-SM-041-010, SCN-SM-041-011 | `tests/e2e/qf_decisions_connector_api_test.go` | `exports consent-scoped evidence bundle and rejects missing-consent export` | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | SCN-SM-041-009, SCN-SM-041-010 | `tests/e2e/qf_evidence_export_test.go` | `user selects context grants consent and sees QF evidence export status` | `./smackerel.sh test e2e` | Yes |
| Security | security | SCN-SM-041-011 | `tests/security/qf_evidence_export_security_test.go` | `blocks evidence export without consent source artifact IDs or target context` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

Core behavior:

- [ ] Smackerel builds `PersonalEvidenceBundle`s with required fields `bundle_id`, `export_id`, `consent_scope`, `sensitivity_tier`, `source_artifact_ids`, `extracted_claims`, `provenance`, `redaction_summary`, `target_context`, and `created_at`; `source_refs` is optional and is included only when the source has external IDs.
- [ ] A Smackerel-locally-valid bundle passes QF import validation in spec 063 (field-set parity); divergence between repos is treated as a contract regression and blocks the scope.
- [ ] Evidence export is user-initiated and records export ID, QF import status, QF import reference, and rejection reason when applicable.
- [ ] Missing bundle/export ID, `created_at`, consent, sensitivity, source artifact IDs, claims, provenance, redaction summary, or target context blocks export or records rejection.
- [ ] Prompt-assisted extraction preserves source citations and never creates QF decision text, approval, execution, mandate, or financial advice.

Validation:

- [ ] Unit tests cover bundle field validation, source citation, consent handling, sensitivity handling, and missing-field rejection.
- [ ] Integration tests prove QF export status is persisted and linked to source artifacts.
- [ ] E2E API test proves accepted and rejected QF import paths with idempotent export IDs.
- [ ] E2E UI test proves the user-visible export flow shows consent, sensitivity, target context, and final QF status.
- [ ] Security test proves missing consent or cross-target export does not become accepted evidence.

Build quality gate:

- [ ] Raw test evidence and regression-quality guard output are recorded in `report.md`.
- [ ] No raw personal data dump path is introduced; bundles are compact evidence objects with references and redaction summary.
- [ ] Documentation explains evidence export as personal context only, not trading authority.

## Scope 5: Safety Boundaries Observability Documentation And Tests

Status: [ ] Not started  
Depends On: Scope 4

### Business Scenarios

#### SCN-SM-041-012: Operator Diagnoses QF Connector Health

Given QF auth fails, schema mismatches, cursor lag grows, or packets fail validation  
When the operator opens connector health/status surfaces  
Then Smackerel shows QF connector health, issue category, packet/export identifiers, and safe remediation guidance.

#### SCN-SM-041-013: Boundary Violation Is Prevented And Auditable

Given a user or future UI path attempts to approve, execute, change mandate, trigger EmergencyStop, or create a QF watch from Smackerel pre-MVP  
When the attempt reaches Smackerel safety gating  
Then the action is blocked, logged, and no QF action request is sent.

#### SCN-SM-041-014: Release Follow-Up Is Documented Outside Pre-MVP DoD

Given the connector is pre-MVP  
When reviewers inspect docs and tests  
Then MVP/v1/v2/v3 upgrades are documented as follow-up and no current DoD claims action parity, tenant certification, voice parity, or execution support.

### Implementation Plan

- Add connector health diagnostics for auth failure, schema mismatch, cursor lag, validation failures, artifact publication failures, evidence export status, and boundary violation attempts.
- Add metrics/logging with packet ID, trace ID, event ID, export ID, source ID, health state, and reason codes.
- Add safety gate tests proving no QF action request is emitted for approval, execution, mandate, EmergencyStop, or QF watch attempts.
- Implement reserved-schema handling per the design's "Reserved Schemas (Not Implemented Pre-MVP)" subsection: never construct or send `QFApprovalAction` or `QFWatchSignal`; if diagnostic tooling encounters `QFApprovalAction`, normalize it into a `qf/approval-request` artifact with `Metadata.reserved = true` and exclude it from search, digest, recommendation surfaces, and the evidence builder; treat any inbound `QFWatchSignal` payload as a diagnostic log only and never alter connector state, packet state, digest content, or Telegram delivery.
- Update connector operations docs, testing docs, and release notes with QF authority boundaries and post-MVP follow-up.
- Add cross-repo compatibility verification against QF spec 063 packet/evidence contracts.
- Run artifact lint and implementation reality scan before completion.
- Change Boundary: diagnostics, safety gates, docs, and tests only; no new QF capability expansion.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Integration | integration | SCN-SM-041-012 | `tests/integration/qf_decisions_health_test.go` | `reports QF connector auth schema cursor validation and export health states` | `./smackerel.sh test integration` | Yes |
| E2E API | e2e-api | SCN-SM-041-012, SCN-SM-041-013 | `tests/e2e/qf_decisions_connector_api_test.go` | `blocks QF action boundary attempts and exposes connector diagnostics` | `./smackerel.sh test e2e` | Yes |
| E2E UI | e2e-ui | SCN-SM-041-012, SCN-SM-041-013 | `tests/e2e/qf_decisions_surfaces_test.go` | `shows QF connector diagnostics and no approval execution mandate or EmergencyStop controls` | `./smackerel.sh test e2e` | Yes |
| Stress | stress | SCN-SM-041-012 | `tests/stress/qf_decisions_sync_stress_test.go` | `tracks QF cursor lag and validation failures under repeated sync pages` | `./smackerel.sh test stress` | Yes |
| Artifact lint | artifact | SCN-SM-041-014 | `specs/041-qf-companion-connector` | `artifact lint accepts completed QF companion connector evidence` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |

### Definition of Done

Core behavior:

- [ ] Connector health/status surfaces expose auth failures, schema mismatches, cursor lag, validation failures, artifact publication failures, evidence export outcomes, and boundary violation attempts.
- [ ] Safety gates block approval, execution, mandate, EmergencyStop, and QF watch behavior without sending QF action requests.
- [ ] Reserved schemas are honored per design: `QFApprovalAction` is never constructed or sent and any inbound action object is normalized into `qf/approval-request` with `Metadata.reserved = true` and excluded from search, digest, recommendation, and evidence-builder surfaces; `QFWatchSignal` payloads are recorded as diagnostic logs only and never alter connector, packet, digest, or Telegram state.
- [ ] Logs and metrics preserve packet ID, trace ID, event ID, export ID, source ID, health state, and reason codes where applicable.
- [ ] Documentation clearly states Smackerel is a companion/context surface and QF is the decision authority.
- [ ] MVP/v1/v2/v3 approvals, standing watches, tenant-aware access, voice, EmergencyStop parity, and execution remain release follow-up only.

Validation:

- [ ] Integration tests cover health diagnostics and metrics/log fields for auth, schema, cursor, validation, artifact, and export states.
- [ ] E2E API and E2E UI tests prove action-boundary attempts are blocked and no QF action request is sent.
- [ ] Stress tests prove cursor lag and validation metrics remain reliable under repeated sync pages.
- [ ] Artifact lint, implementation reality scan, and live-test authenticity scans pass before any scope status is changed to Done.

Build quality gate:

- [ ] Raw verification evidence is recorded in `report.md` with command, exit code, and untruncated output.
- [ ] Required E2E regression tests pass regression-quality guard and include adversarial cases for missing badges, unknown packet versions, missing consent, and action-boundary attempts.
- [ ] No code or documentation claims Smackerel can provide QF financial advice, approve trades, change mandates, execute, or certify external-surface parity in pre-MVP.

## Release Follow-Up Not In Pre-MVP DoD

- MVP: QF-authenticated connector hardening and only QF-official limited actions if QF exposes them.
- v1.0: QF-owned watch evaluation and delivery channel behavior for paper/analysis workflows.
- v2.0: tenant-aware service credentials, revocation, retention, metering, and audit policy.
- v3.0: certified external companion parity, voice/mobile parity, EmergencyStop parity, and execution workflows only where QF mandates allow.
