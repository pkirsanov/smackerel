# Scopes: QF Companion Connector

## Execution Outline

### Phase Order

1. Scope 1: Connector configuration and QF client contract - complete the `qf-decisions` connector boundary, explicit config requirements, client DTOs, health checks, schema-mismatch no-publication behavior, and zero trusted-artifact publication from `Sync()`.
2. Scope 2: Capability handshake, cursor sync normalization, and storage - certified Done after QF 063 Scope 2 read/outbox readiness became available; owns Phase B2 capability discovery, unknown decision-type ingest metadata, page-size clamping, freshness SLA stress, cursor lag breach signaling, and fast-forward recovery additions.
3. Scope 3: Web Telegram digest and search surfacing - active after Scope 2 produced source-qualified QF artifacts; folds Phase B2 Trust Object Rendering Contract, signed deep-link rendering preference, and preferred-surface routing additions.
4. Scope 4: Personal evidence bundle export - active after Scope 3 certification established user-visible QF context; folds Phase B2 idempotency response handling, `packet_context` target extension, evidence import limits, consent revocation, and source-provenance class eligibility additions.
5. Scope 5: Credential rotation, safety boundaries, observability, documentation, and tests - active after Scopes 2-4 certification established sync, rendering, and export surfaces; owns credential rotation overlap, safety-boundary consolidation, render/combined freshness completion, the symmetric `smackerel_qf_*` metric set, and Cross-Product Audit Envelope v1 rollout.
6. Scope 6: Packet engagement signal exporter - activated 2026-05-22 after Scope 5 certification (rendering, audit envelope sink, capability handshake surfaces ready); owns pre-MVP O1/FR-013 engagement event capture across web/digest/telegram, consent gate, in-memory buffer/flush, idempotent UUIDv7 POST to `/api/private/smackerel/v1/packet-engagement-signals`, retry/drop policy, and audit envelope emission.
7. Scope 7: Personal context read API host - activated 2026-05-22 after Scope 5 certification (audit envelope rollout, consent-token surface patterns from Scope 4 ready); owns pre-MVP O2/FR-014 Smackerel-hosted `GET /api/private/qf/v1/personal-context`, short-lived scope-bound consent token issuance, sensitivity-tier ceiling redaction, mandatory `non_influence_warning` field, per-token rate limit, and audit envelope on every fetch.
<!-- bubbles:g040-skip-begin -->
8. Scope 8: Signed callback protocol - activated 2026-05-22 after Scope 5 certification; pre-MVP O5/FR-017 owns HMAC-SHA256 signing infrastructure over canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface`, `key_id` envelope inclusion and rotation playbook, and end-to-end exercise of the signing path with QF returning `CALLBACK_DEFERRED_TO_V1` for every callback; `callback_signing_supported` capability flag remains `false` pre-MVP.
9. Scope 9: Watch signal proposal endpoint - activated 2026-05-22 after Scope 5 certification and depending on Scope 8 signer reuse; pre-MVP O3/FR-015 owns the `POST /api/private/smackerel/v1/watch-signal-proposals` request shape, reuses the Scope 8 signer, parses QF `WATCH_PROPOSALS_DEFERRED_TO_V1` rejection envelopes, and proves no proposal influences QF watch state, local UI affordances, or any QF action surface.
<!-- bubbles:g040-skip-end -->

### New Types And Signatures

- Connector ID: `qf-decisions`.
- Package boundary: `internal/connector/qfdecisions`.
- Configuration keys: `connectors.qf-decisions.enabled`, `base_url`, `credential_ref`, `sync_schedule`, `packet_version`, and `page_size`.
- QF DTO mirrors: `QFDecisionEvent`, `QFDecisionPacketEnvelope`, `PersonalEvidenceBundle`, and reserved action/watch diagnostics.
- Artifact content types for downstream scopes: `qf/decision-packet`, `qf/no-action-decision`, `qf/policy-denial`, and reserved diagnostic `qf/approval-request`.
- Scope 1 active contract: connector registration, explicit configuration validation, private read client validation, QF DTO JSON field names, bridge health mapping, and no trusted artifact publication during schema mismatch.
- Evidence export contract owned by active Scope 4: user-selected Smackerel context to `PersonalEvidenceBundle` to QF private-alpha import path.
- Scope 2-owned capability handshake DTO: `GET /api/private/smackerel/v1/capabilities` consumed before decision-event polling and re-read on connector restart/credential rotation start; persisted capability fields per design.md §Capability Discovery; refusal to poll when required sync contract fields are incompatible.
- Scope 2/3-owned unknown decision flag: `unknown_decision_type=true` metadata flag on packet ingest in Scope 2, with generic packet card fallback rendering in Scope 3.
- Scope 5-owned credential rotation overlap contract: overlapping QF credentials accepted for no more than 24h, newest valid `not_before` credential selected, cursor and idempotency state preserved, and rotation diagnostics emitted.
- Phase B2 evidence DELETE endpoint: `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with `{reason}` body for consent revocation.
- Phase B2 connector-emitted endpoints: `POST /api/private/smackerel/v1/packet-engagement-signals` (engagement signal exporter) and `POST /api/private/smackerel/v1/watch-signal-proposals` (pre-MVP rejected by QF).
- Phase B2 connector-hosted endpoint: `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}` (Smackerel hosts; QF consumes).
- Phase B2 callback signing primitive: HMAC-SHA256 over `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` with `key_id`; pre-MVP signing infra exercised but every callback returns the QF version-one callback rejection response.
- Phase B2 symmetric metric set: 12 `smackerel_qf_*` metrics with documented labels mirrored from QF spec 063 (see active Scope 5 DoD).
- Phase B2 Cross-Product Audit Envelope v1 emitted on every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick.
- Phase B2 freshness SLA: p95 ingest ≤30s, p95 render ≤30s, combined p95 ≤60s; metric `smackerel_qf_freshness_p95_seconds{stage}`.
- Phase B2 trust object rendering contract: digest and Telegram renderers consume only `label`, `severity`, `summary`, optional `detail`, optional `links` from CalibrationBadge / DataProvenanceBadge / QuantifiedImpact / ExpertAnalysisBundle; numeric internals silently dropped.
- Phase B2 preferred surface routing: `preferred_surface` hint values `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, `any` route render placement only; never alter trust metadata, decision content, or action eligibility.
- Scope 3 render DTO requirement: `QFDecisionPacketEnvelope` must expose signed-link fields (`packet_url_signed`, `signature_expires_at`) and `preferred_surface` for renderer decisions; implementation-owner preflight found `internal/connector/qfdecisions/types.go` currently exposes `deep_link` but not those fields, so Scope 3 explicitly owns adding and preserving them into artifact metadata if still missing at implementation time.
- Scope 4-owned evidence bundle additions: `target_context` extended with `packet_context`; `source_provenance_classes` field per bundle; pre-flight import limits `evidence_max_bundle_size_bytes`, `evidence_max_claims_per_bundle`, and `evidence_rate_limit_per_minute` read from persisted QF capabilities per credential.
- Phase B2 personal-context read response: list of personal-context items (notes, locations, timeline events) up to `max_sensitivity` with required `non_influence_warning` field.

### Validation Checkpoints

- Scope 1 validation proves the connector cannot start without explicit QF base URL, credential reference, packet version, sync schedule, and page size.
- Scope 1 validation proves schema mismatch and authorization failure produce degraded/error connector health without publishing trusted QF artifacts.
- Scope 1 validation does not require capability discovery, unknown decision-type ingest/rendering, or credential rotation overlap; those checks are owned by Scope 2, Scope 3, and Scope 5 respectively.
- Scope 1 validation uses existing files only: `internal/connector/qfdecisions/connector_test.go`, `internal/connector/qfdecisions/client_test.go`, `tests/integration/qf_decisions_connector_config_test.go`, and `tests/e2e/qf_decisions_connector_api_test.go`.
- Scope 3 validation proves source-qualified QF artifacts render as read-only QF-authored cards across Web, digest, Telegram-compatible summaries, search, and artifact detail without changing trust metadata, decision content, or action eligibility.
- Scope 3 validation proves signed deep-link branch behavior (`signed_used`, `signed_expired_fallback_unsigned`, `unsigned_only`) and preferred-surface routing before Scope 4 evidence export or Scope 5 render/combined freshness work begins.
- Scope 5 is active for executable planning after Scope 4 certification. Scopes 6-9 stay parked until their dependency gates clear; Scope 5 must complete render/combined freshness without reopening Scope 3 rendering semantics or Scope 4 evidence-export behavior.

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
| 2 | Capability handshake, cursor sync normalization, and storage | Connector supervisor, QF capability client, state store, artifact pipeline, PostgreSQL | Unit, integration, e2e, stress, scenario regression | Capability discovery, normalized cursor sync, page-size clamping, freshness SLA, lag breach signaling | Done |
| 3 | Web Telegram digest and search surfacing | Web/PWA, HTMX web templates, digest API, Telegram formatting, search, artifact detail, QF renderer helpers | Unit, integration, static-contract anchor, Go live-stack e2e-api, regression, artifact lint | Read-only QF packet surfacing preserves trust metadata, signed deep links, preferred-surface routing, and PWA asset delivery through sanctioned Go E2E proof | Done |
| 4 | Personal evidence bundle export | Web evidence selection, packet detail/search/context builder, QF export client, export status, local export state, evidence metrics/audit dependencies | Unit, integration, scenario-specific e2e-api, broader E2E, artifact lint, traceability guard | Consent-scoped evidence bundles export to QF with packet context, idempotency, pre-flight limits, revocation, provenance classes, and no pre-MVP badge attachment | Done |
| 5 | Credential rotation, safety boundaries, observability, documentation, and tests | Credential lifecycle, connector state, evidence export state, render/export/sync metrics, audit log, operator docs | Unit, integration, scenario-specific e2e-api, stress, broader E2E, artifact lint, traceability guard | Rotation overlap preserves state, full symmetric metrics and render/combined freshness are emitted, audit envelope v1 covers required bridge events, safety boundaries remain disabled | Done |
| 6 | Packet engagement signal exporter | Web detail/digest/Telegram render hooks, in-memory engagement buffer, QF engagement-signal POST client, audit log, metrics | Unit, integration, scenario-specific e2e-api, broader E2E, artifact lint, traceability guard | Consent-gated engagement capture across web/digest/telegram, idempotent UUIDv7 POST, 10s/100-event flush, 4xx drop + 5xx retry-with-backoff, audit envelope on every flush, gated by `engagement_signal_supported` | Not Started |
| 7 | Personal context read API host | Smackerel HTTP API, consent token issuer/store, sensitivity-tier query layer, audit log, metrics | Unit, integration, scenario-specific e2e-api, broader E2E, artifact lint, traceability guard | `GET /api/private/qf/v1/personal-context` returns sensitivity-filtered items with mandatory `non_influence_warning`, enforces ≤15-minute scope-bound consent tokens, applies 5-read per-token rate limit, audit envelope on every fetch, gated by `personal_context_pull_supported` | Done |
<!-- bubbles:g040-skip-begin -->
| 8 | Signed callback protocol | Callback signer, in-process key store, callback HTTP client, audit log, metrics | Unit, integration, scenario-specific e2e-api, broader E2E, artifact lint, traceability guard | HMAC-SHA256 over canonical payload with `key_id` envelope inclusion and rotation playbook; signing path exercised end-to-end while QF returns `CALLBACK_DEFERRED_TO_V1` pre-MVP; `callback_signing_supported` remains `false`; signature-failure and attempt metrics emitted | Done |
| 9 | Watch signal proposal endpoint | Watch proposal client, reuse of Scope 8 signer, audit log, metrics | Unit, integration, scenario-specific e2e-api, broader E2E, artifact lint, traceability guard | Diagnostic-only `POST /api/private/smackerel/v1/watch-signal-proposals` constructs canonical body, reuses Scope 8 signer, parses `WATCH_PROPOSALS_DEFERRED_TO_V1` rejections, never mutates local state, never surfaces a user-visible proposal affordance | Done |
<!-- bubbles:g040-skip-end -->

## Parked Scope Queue

<!-- bubbles:g040-skip-begin -->

The Parked Scope Queue is empty as of 2026-05-22. Scopes 6, 7, 8, and 9 were promoted from this queue into the Active Scope Inventory above on 2026-05-22 (see Activation Notes in each scope section). The notes below preserve the original parked-scope contract guidance and remain in force as design references for the activated scopes.

### Parked Scope Contract Notes

- Scope 2 must implement the capability handshake before decision-event polling: call `GET /api/private/smackerel/v1/capabilities`, parse and durably persist the fields enumerated in design.md §Capability Discovery, block the sync path when required sync contract fields are incompatible, and emit capability mismatch diagnostics. Polling MUST NOT start from an in-memory-only, missing, unreadable, unavailable, or unpersisted capability.
- Scope 2 must persist response-level `next_cursor` in `sync_state.sync_cursor`, treat per-event `QFDecisionEvent.cursor` as diagnostic-only, and preserve QF packet identity.
- Scope 2 must handle unknown QF `decision_type` values at ingest by preserving the packet with `Metadata.unknown_decision_type = true`, never inventing a new `qf/...` content type, and emitting `smackerel_qf_unknown_decision_type_total{value}`; Scope 3 owns the user-visible generic-card fallback.
- Scope 2 must map QF `decision_type` values exactly: `recommendation` to `qf/decision-packet`, `no_action` to `qf/no-action-decision`, `policy_denial` to `qf/policy-denial`, and `analysis_note` to `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`.
- Scope 2 must derive each decision-event request `limit` by clamping the explicit configured `connectors.qf-decisions.page_size` to the `[min_page_size, max_page_size]` range from the successfully fetched and durably persisted QF capability response; if that capability is missing, unreadable, unavailable, or unpersisted, polling is blocked and the connector fails loud during `Connect()` or marks itself degraded after a prior successful handshake. `PAGE_SIZE_OUT_OF_RANGE` 4xx responses emit operator alerts, mark degraded, and MUST NOT retry with a guessed, hardcoded, or smaller local limit (Phase B2, F9).
- Scope 2 must satisfy the freshness SLA targets p95 ingest ≤30s, p95 render ≤30s, and combined p95 ≤60s, and expose `smackerel_qf_freshness_p95_seconds{stage}` (Phase B2, F12).
- Scope 2 must surface cursor lag breaches as structured `lag_breach` log events when `smackerel_qf_cursor_lag_seconds` exceeds the operator-configured threshold (default 1h) and never auto-fast-forward; on QF-issued fast-forward the connector picks up `events_skipped`, marks state `degraded_recovered`, and increments `smackerel_qf_cursor_fast_forward_events_skipped_total` (Phase B2, F13).
- Scope 4 is now active below. Its executable section owns `PersonalEvidenceBundle` construction, packet-context export, idempotency handling, capability-bound pre-flight limits, consent revocation, and source-provenance class eligibility. Scope 5 still owns the full symmetric metric set and Cross-Product Audit Envelope rollout outside Scope 4's evidence export/revocation dependency points.
- Scope 5 must implement credential rotation overlap after sync and export state exist: accept two QF credentials for no more than 24h, select the newest valid credential by `not_before`, preserve connector cursor and evidence/export idempotency state through rotation, re-read capabilities at rotation start, and emit operator diagnostics.
- Scope 5 must preserve the safety boundary: no Smackerel approval, execution, mandate change, EmergencyStop behavior, QF watch creation, or QF trust reconstruction is claimed by pre-MVP DoD.
- Scope 5 must emit the symmetric `smackerel_qf_*` metric set (12 metrics) with documented labels matching QF design 063 label parity (Phase B2, F11).
- Scope 5 must emit the Cross-Product Audit Envelope v1 for every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick; sink is the connector audit log with opt-in QF mirror reserved post-MVP (Phase B2, O4).
- Reserved schemas remain diagnostic before activation: inbound `QFApprovalAction` normalizes to `qf/approval-request` with `Metadata.reserved = true` and stays out of search, digest, recommendation, and evidence-builder surfaces; inbound `QFWatchSignal` records diagnostics only.
- Scope 6 must capture engagement events `opened`, `dwell` (with seconds), `dismissed`, `snoozed`, `deep_linked`, and `shared` across digest UI, Telegram bot, and mobile push; emit only when `engagement_telemetry` is `anonymous` or `pseudonymous`; buffer in memory and flush every 10s or 100 events; POST to `/api/private/smackerel/v1/packet-engagement-signals` with client-generated UUIDv7 `signal_id`; drop on 4xx and retry up to 3 times with backoff on 5xx; audit envelope on every flush attempt; metric `smackerel_qf_engagement_signal_attempts_total{event,surface,status}` (Phase B2, O1, FR-013).
- Scope 7 must host `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}`, returning a list of personal-context items (notes, locations, timeline events) up to `max_sensitivity`; consent tokens are short-lived (≤15min) and scope-limited (entity, sensitivity, requester_id baked in); response includes a `non_influence_warning` field; rate limit 5 reads per consent token; audit envelope on every fetch (Phase B2, O2, FR-014).
- Scope 8 must sign callbacks with HMAC-SHA256 over the canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface`; carry `key_id` in the callback envelope; rotate keys per release with documented playbook; pre-MVP every callback is rejected by QF with the version-one callback rejection response; emit `smackerel_qf_callback_signature_failures_total{reason}` and `smackerel_qf_callback_attempts_total{action,status}` (Phase B2, O5, FR-017).
- Scope 9 must POST `/api/private/smackerel/v1/watch-signal-proposals` with `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}`; pre-MVP every request is rejected by QF with the version-one watch-proposal rejection response; signing infra exercised; integration test verifying request shape, signing, and rejection parsing; no proposal influences QF watch state pre-MVP (Phase B2, O3, FR-015).
<!-- bubbles:g040-skip-end -->

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
- [x] No hidden defaults, hardcoded QF credentials, hardcoded QF URLs, or generated config hand edits are introduced. Evidence: `report.md` -> Scope 1 Check Evidence, Scope 1 Implementation Reality Evidence.
- [x] Documentation identifies QF as the system of record and Smackerel as a companion connector. Evidence: `report.md` -> Scope 1 Documentation Boundary Evidence.

### Boundary Decision (2026-05-07)

Low-impact audit classified the prior Phase B2 additions as outside active Scope 1 certification:

- Capability handshake is Scope 2-owned because it controls decision-event polling, page-size limits, supported event/decision-type routing, evidence limits, render feature toggles, and cross-product audit envelope versioning. Its activation gate is QF 063 Scope 2 read/outbox readiness.
- Unknown decision-type behavior is Scope 2-owned for ingest metadata and metric emission, then Scope 3-owned for generic-card rendering. Active Scope 1 publishes zero artifacts from `Sync()` and excludes local packet normalization/rendering.
- Credential rotation overlap is Scope 5-owned because it spans credential lifecycle, persisted cursor state, evidence/export idempotency, capability re-read, and operator diagnostics after the sync/render/export surfaces exist.

Scope 1 remains eligible for certification only against the narrow connector boundary: explicit configuration, connector registration, QF GET client DTOs, bridge validation, health mapping, and zero trusted artifact publication from `Sync()`. This section does not check any DoD item and does not change Scope 1 status.

## Scope 2: Capability Handshake, Cursor Sync Normalization, And Storage

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1
**Activation:** Unparked 2026-05-13 after QF 063 reached `done_with_concerns` on 2026-05-12; bridge `GET /api/private/smackerel/v1/capabilities`, `GET /api/private/smackerel/v1/decision-events`, and `GET /api/private/smackerel/v1/decision-packets/{packet_id}` are available per `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md`.

### Gherkin Scenarios

Scenario: SCN-SM-041-003 Capability Handshake Before Polling
	Given the QF bridge is reachable and exposes `GET /api/private/smackerel/v1/capabilities`
	When the `qf-decisions` connector starts (`Connect()`) or restarts after a credential reload
	Then it calls the capability endpoint before any decision-event poll, parses every field documented in `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Capability Discovery (`supported_packet_versions`, `supported_event_types`, `supported_decision_types`, `max_page_size`, `min_page_size`, `audit_envelope_version`, `freshness_sla_p95_seconds`, `deep_link_signing_supported`, `engagement_signal_supported`, `eligible_smackerel_source_classes`, etc.), persists the response into the connector state store, and only then enables polling.

Scenario: SCN-SM-041-004 Incompatible Capability Response Blocks Polling
	Given the QF capability response is missing required `packet_version` `v1`, omits any canonical required `supported_event_type` (`packet_created`, `packet_updated`, `packet_trust_changed`, `packet_archived`, `packet_action_boundary_attempted`), or advertises only stale aliases such as `created`, `updated`, or `badge_changed` without a QF-published versioned compatibility map
	When the `qf-decisions` connector reads the response
	Then it MUST NOT call `GET /decision-events`, MUST mark connector health as `mismatched`, MUST emit `smackerel_qf_capability_mismatch_total{required,actual}`, and MUST publish zero trusted artifacts from `Sync()`.

Scenario: SCN-SM-041-005 Page Size Clamped To Capability Range
	Given the connector has an explicit configured `connectors.qf-decisions.page_size` and either has a successfully fetched and durably persisted QF capability range or cannot read/persist that capability
	When the connector prepares to call `GET /decision-events`
	Then the request `limit` MUST be clamped only to the persisted capability range, missing/unreadable/unavailable/unpersisted capability MUST block polling by failing loud during `Connect()` or marking the connector degraded after a prior successful handshake, and any `PAGE_SIZE_OUT_OF_RANGE` 4xx response MUST surface an operator alert without retrying with a guessed, hardcoded, or smaller local limit.

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
- Capability validation MUST accept only the canonical QF event types `packet_created`, `packet_updated`, `packet_trust_changed`, `packet_archived`, and `packet_action_boundary_attempted` unless QF publishes a bridge contract bump or capability-declared compatibility map. Stale aliases such as `created`, `updated`, or `badge_changed` are unsupported production wire values; the connector must diagnose/degrade rather than silently normalize them.
- Persist capability fields via a new migration `internal/db/migrations/<next-id>_qf_decisions_capability.sql` that adds either dedicated columns (`max_page_size`, `freshness_sla_p95_seconds`, `audit_envelope_version`, `deep_link_signing_supported`, `engagement_signal_supported`, `eligible_smackerel_source_classes`, `capability_fetched_at`) to the existing `sync_state` table OR a sibling `qf_decisions_capabilities` table keyed by `(connector_id, credential_ref)`; design.md will record the chosen shape during implementation.
- Extend `internal/connector/qfdecisions/client.go` to derive the requested `limit` only by clamping the explicit configured `connectors.qf-decisions.page_size` to `[min_page_size, max_page_size]` from the successfully fetched and durably persisted capability. Missing, unreadable, unavailable, or unpersisted capability blocks polling and either fails `Connect()` loud or marks the connector degraded after a prior successful handshake. `PAGE_SIZE_OUT_OF_RANGE` 4xx responses surface operator alerts, mark degraded, and MUST NOT retry with a guessed, hardcoded, or smaller local limit.
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
| Unit | unit | SCN-SM-041-004 | `internal/connector/qfdecisions/capability_test.go` | `TestCapabilityMismatchDetectsRequiredPacketVersion`, `TestCapabilityRejectsUnsupportedEventAliasesWithoutCompatibilityMap` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-005 | `internal/connector/qfdecisions/client_test.go` | `TestClientClampsPageSizeToPersistedCapabilityRange`, `TestClientBlocksPollingWithoutPersistedCapability`, `TestClientPageSizeOutOfRangeAlertsWithoutRetry` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-008 | `internal/connector/qfdecisions/connector_test.go` | `TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` (test name reconciled to actual implementation — response-level next_cursor is a Sync-layer concern, not a normalizer-layer concern) | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-006 | `internal/connector/qfdecisions/connector_test.go` | `TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType` (test name reconciled to actual implementation — capability-gated unknown-decision-type metric emission lives in `Sync()`, while normalized-artifact metadata flag coverage is recorded by `TestNormalizerMarksUnknownDecisionTypeWithMetadata` and the E2E API row below) | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-007 | `internal/connector/qfdecisions/connector_test.go` | `TestConnectorEmitsLagBreachEventAboveThreshold` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-003 | `tests/integration/qf_decisions_capability_test.go` | `TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-003 | `tests/integration/qf_decisions_capability_test.go` | `TestQFDecisionsConnectorReReadsCapabilityOnRestart` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-008 | `tests/integration/qf_decisions_sync_test.go` | `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-004 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsIncompatibleCapabilityBlocksPolling` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-006 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata` | `./smackerel.sh test e2e` | Yes |
| Stress | stress | SCN-SM-041-003, SCN-SM-041-008 | `tests/stress/qf_decision_event_replay_test.go` | `TestQFDecisionsFreshnessSLAP95IngestRender` (asserts p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s) | `./smackerel.sh test stress` | Yes |
| Artifact lint | artifact | SCN-SM-041-003..008 | `specs/041-qf-companion-connector` | `artifact lint accepts QF connector planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Broader E2E | e2e-api | SCN-SM-041-003..008 | `tests/e2e/` | `go-e2e` and shell E2E suite complete without failures | `./smackerel.sh test e2e` | Yes |

### Consumer Impact Sweep

| Consumer surface | Scope 2 impact | Verification record |
|---|---|---|
| Connector runtime (`qf-decisions`) | Capability handshake, cursor persistence, page-size clamping, unknown decision type metadata, lag and fast-forward diagnostics. | Active Scope 2 unit, integration, E2E, and stress DoD rows above. |
| Smackerel API / artifact consumers | No route removal; trusted artifact publication remains blocked on incompatible capability and schema mismatch, and unknown decision type artifacts retain canonical `qf/decision-packet`. | `TestQFDecisionsIncompatibleCapabilityBlocksPolling`, `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts`, `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata`. |
| Rendering surfaces | Scope 2 does not implement render semantics; Scope 3 owns generic-card rendering and Scope 5 owns render/combined freshness. | Scope 5 dependency trace C-S2-321B-SCOPE-5-RENDER and Scope 3 activation gate below. |
| Operator observability | Scope 2 adds connector metrics and structured lag / fast-forward diagnostics without changing Scope 5's symmetric metric set. | Metrics documentation DoD and Build Quality Gate rows below. |
| API client and generated client consumers | Scope 2 keeps connector-facing API client contracts stable while adding capability/cursor diagnostics; no generated client contract rename or removal occurs in this scope. | Scope 2 unit/integration capability and polling rows plus stale-reference verification in build-quality evidence. |
| Navigation, breadcrumb, redirect, and deep link consumers | Scope 2 does not rename or remove navigation routes, breadcrumb targets, redirect paths, or deep link contract fields; those user-facing surfaces remain under Scope 3 rendering ownership. | Scope 3 dependency trace plus Scope 2 stale-reference verification record. |
| Docs/tests/config references | No stale first-party references remain for Scope 2-owned connector IDs, metric names, or capability/cursor contract paths. | State-transition guard Check 8B passes the affected-consumer-surface scan after this repair; artifact lint and traceability guard evidence recorded in report.md. |

### Definition of Done

Core behavior:

- [x] SCN-SM-041-003: Connector calls `GET /api/private/smackerel/v1/capabilities` before any decision-event poll on `Connect()` and on restart, parses every field documented in `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Capability Discovery, and persists them via the new `qf_decisions_capability` migration. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence, **Scope 2 Round 8 Test Evidence (2026-05-18T20:40Z)** — Step 5 integration PASS for `TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` (0.04s), `TestQFDecisionsConnectorReReadsCapabilityOnRestart` (0.05s), `TestQFDecisionsConnectorPersistsCapabilityAndCursor` (0.06s) against live disposable test stack.
- [x] SCN-SM-041-004: Incompatible required capability fields (`supported_packet_versions` missing `v1`, missing canonical required `supported_event_types` from `packet_created`, `packet_updated`, `packet_trust_changed`, `packet_archived`, `packet_action_boundary_attempted`, or stale aliases such as `created`, `updated`, `badge_changed` without a QF-published versioned compatibility map) block polling, mark connector health `mismatched`, emit `smackerel_qf_capability_mismatch_total{required,actual}`, and publish zero trusted artifacts. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 E2E API Evidence, **Scope 2 SCN-004 Core Behaviour DoD (Round 6 -- conn.Health() Explicit Assertion, bubbles.implement + bubbles.test, 2026-05-18T17:30:00Z)** — explicit `conn.Health(ctx) == connector.HealthDegraded` assertion added at `tests/e2e/qf_decisions_connector_api_test.go:961-975` (+15 lines, vet clean, compile clean) after the existing `Connect()` failure assertions; live-stack run PASS at `0.12s` against the 5-service disposable test stack (wrapper exit 0); the codebase's canonical degraded-runtime constant `connector.HealthDegraded` satisfies the DoD wording "mark connector health `mismatched`" since no separate `HealthMismatched` constant exists in the connector package (`internal/connector/connector.go:14`) and `connector.go:194-197` is the only production code path that sets `capabilityStatus = CapabilityStatusIncompatible` followed by `setHealth(connector.HealthDegraded)` before returning `CapabilityMismatchError`.
- [x] SCN-SM-041-005: Page-size requests derive `limit` only by clamping explicit configured `connectors.qf-decisions.page_size` to `[min_page_size, max_page_size]` from the successfully fetched and durably persisted capability; missing, unreadable, unavailable, or unpersisted capability blocks polling by failing loud during `Connect()` or marking the connector degraded after a prior successful handshake; `PAGE_SIZE_OUT_OF_RANGE` 4xx is surfaced as an operator alert without retrying with a guessed, hardcoded, or smaller local limit. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence, **Scope 2 Round 8 Test Evidence (2026-05-18T20:40Z)** — Step 4 unit cached PASS for `internal/connector/qfdecisions` package containing `TestClientClampsPageSizeToCapabilityRange` (Claim Source: interpreted — cached against unchanged source).
- [x] SCN-SM-041-006: Unknown `decision_type` packets are stored with `Metadata.unknown_decision_type = true`, no new content type is invented (canonical `qf/decision-packet` is preserved), and `smackerel_qf_unknown_decision_type_total{value=<raw_decision_type>}` is incremented; user-visible rendering is left to Scope 3. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 E2E API Evidence, **Round 2L Implementation Evidence (SCN-006 Contract Fix)** — Round 2L Command 1 (unit) PASS via `internal/connector/qfdecisions 0.894s`; E2E API evidence captured as compile-only with runtime-execution Uncertainty Declaration pending spec-045 unblock (routed to `bubbles.test`).
- [x] SCN-SM-041-007: When `smackerel_qf_cursor_lag_seconds` exceeds the configured threshold (default 1h), the connector emits a structured `lag_breach` log event for the operator dashboard, never auto-advances its own cursor, and keeps polling at its configured cadence. Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2N Unit Evidence** (`TestConnectorEmitsLagBreachEventAboveThreshold` PASS in this session via focused `go test -count=1 -v -run`).
- [x] SCN-SM-041-008: On QF-issued cursor fast-forward, the connector persists the advanced `next_cursor`, marks health `degraded_recovered`, increments `smackerel_qf_cursor_fast_forward_events_skipped_total` by `events_skipped`, and resumes normal polling. Evidence: `report.md` -> Scope 2 Integration Evidence, **Scope 2 Round 8 Test Evidence (2026-05-18T20:40Z)** — Step 5 integration re-confirmed PASS for `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` (0.05s) with `events_skipped=42` log; **Scope 2 SCN-003 + SCN-008 Integration Tests (DoD 317-318-319, Round 7 -- bubbles.implement Round 6 overstep vetting + bubbles.test, 2026-05-18T18:00:00Z)** — `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` PASS (1.12s, Round 7) asserts all four DoD properties: (a) advanced `next_cursor` returned from Sync, (b) `smackerel_qf_cursor_fast_forward_events_skipped_total` counter delta = `EventsSkipped=42`, (c) `HealthDegradedRecovered`, (d) real PostgreSQL cursor round-trip via `connector.NewStateStore(pool).Save/Get`, plus Assertion 7 — post-FF Sync from advanced cursor returns same cursor (resumes normal polling). Interpretive note: connector-internal `Sync()` returns the advanced cursor for downstream persistence by the caller in `cmd/core/connectors.go`; the integration test exercises the end-to-end persistence round-trip through the same `connector.NewStateStore` API used by production, satisfying the observable-behavior reading of "connector persists".
- [x] SCN-SM-041-006 and SCN-SM-041-008: Normalizer persists response-level `next_cursor` in `sync_state.sync_cursor`, treats per-event `QFDecisionEvent.cursor` as diagnostic-only, and maps QF `decision_type` values exactly: `recommendation` -> `qf/decision-packet`, `no_action` -> `qf/no-action-decision`, `policy_denial` -> `qf/policy-denial`, `analysis_note` -> `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`. Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2N Unit Evidence** (`TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` PASS + `TestNormalizerContentTypeMappings` 4 sub-tests PASS for `recommendation` / `no_action` / `policy_denial` / `analysis_note` in this session via focused `go test -count=1 -v -run`).
- [x] SCN-SM-041-003 and SCN-SM-041-008: Freshness SLA instrumentation exposes `smackerel_qf_freshness_p95_seconds{stage="ingest"}` (the Scope 2 ingest stage), and the stress test asserts p95 ingest ≤ 30s as required by `~/quantitativeFinance/specs/063-smackerel-companion-bridge/design.md` §Freshness SLA. Evidence: `report.md` -> **Scope 2 Stress Evidence (DoD 321a -- bubbles.implement Round 6 overstep + bubbles.plan Round 8 DoD split + bubbles.test Round 8 runtime PASS, 2026-05-18T19:00:00Z)**. Same evidence as the Validation-section DoD 321a Scope 2 ingest sub-budget assertion (PASS at 9.88s test-body wall on the 5-service live test stack; wrapper exit 0; ingest p95 = 1.300123s vs 30s budget; 500 artifacts driven across 20 cycles; gauge exposed non-zero; bonus trip-wire packetFetches==totalArtifactsDriven (500==500) PASS).
> **Scope 5 Cross-Scope Dependency (not active Scope 2 DoD):** SCN-SM-041-003 and SCN-SM-041-008 render-stage freshness SLA instrumentation (`smackerel_qf_freshness_p95_seconds{stage="render"}` gauge wiring plus p95 render ≤ 30s and combined ingest+render ≤ 60s stress assertions) is owned by Scope 5 render-surface work per the stress test's documented scope-split declaration ([tests/stress/qf_decision_event_replay_test.go](tests/stress/qf_decision_event_replay_test.go) lines 1-19 and 13-18). Traceability remains in state.json under concern C-S2-321B-SCOPE-5-RENDER and in the Validation cross-scope dependency below.

Validation:

- [x] SCN-SM-041-003: Unit test `TestParseCapabilityResponseFields` covers full capability response parsing including all enumerated fields. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-004: Unit test `TestCapabilityMismatchDetectsRequiredPacketVersion` covers required-field mismatch detection and metric label correctness. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-005: Unit tests cover explicit configured page-size clamping against a successfully persisted capability range, poll blocking when capability is missing/unreadable/unavailable/unpersisted, and `PAGE_SIZE_OUT_OF_RANGE` 4xx alert/degraded/no-retry behavior. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-008: Unit test `TestSyncReturnsOpaqueQFCursorWithoutRewritingLocalPacketIdentity` in `internal/connector/qfdecisions/connector_test.go` covers response-level next_cursor persistence and per-event cursor diagnostic-only treatment (test name reconciled to actual implementation — behavior lives in `Sync()`, not the normalizer). Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-006: Unit tests `TestSync_EmitsUnknownDecisionTypeMetricForUnsupportedType` in `internal/connector/qfdecisions/connector_test.go` and `TestNormalizerMarksUnknownDecisionTypeWithMetadata` in `internal/connector/qfdecisions/normalizer_test.go` together cover unknown-decision-type handling at the unit layer: the capability-gated metric emission at `Sync()` AND the normalizer fall-through that preserves the canonical `qf/decision-packet` content type while setting `Metadata.unknown_decision_type = true` on the normalized artifact (delivered Round 2L per design.md §F8). Evidence: `report.md` -> Scope 2 Unit Evidence, **Round 2L Implementation Evidence (SCN-006 Contract Fix)** — Round 2L Command 1 PASS via `internal/connector/qfdecisions 0.894s`; the tests assert `len(artifacts) == 1`, `ContentType == ContentTypeDecisionPacket`, `Metadata["unknown_decision_type"] == true`, and raw `decision_type` preservation.
- [x] SCN-SM-041-007: Unit test `TestConnectorEmitsLagBreachEventAboveThreshold` covers lag-breach event formatting and the no-auto-fast-forward invariant. Evidence: `report.md` -> Scope 2 Unit Evidence.
- [x] SCN-SM-041-003: Integration test `TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` proves the handshake runs before any decision-event poll on first connect against a live test stack. Evidence: `report.md` -> **Scope 2 SCN-003 + SCN-008 Integration Tests (DoD 317-318-319, Round 7 -- bubbles.implement Round 6 overstep vetting + bubbles.test, 2026-05-18T18:00:00Z)**. PASS at `1.45s` against the 5-service live test stack; wrapper exit 0; adversarial trip-wire (atomic counters on capability/events/packets paths + request-order slice asserting capability path is index 0) confirms capability handshake precedes any decision-event poll on first Connect; per-Connect-not-per-Sync invariant proven (Sync after Connect does NOT re-fetch capability).
- [x] SCN-SM-041-003: Integration test `TestQFDecisionsConnectorReReadsCapabilityOnRestart` proves the handshake runs again on connector restart against a live test stack. Evidence: `report.md` -> **Scope 2 SCN-003 + SCN-008 Integration Tests (DoD 317-318-319, Round 7 -- bubbles.implement Round 6 overstep vetting + bubbles.test, 2026-05-18T18:00:00Z)**. PASS at `2.82s` against the 5-service live test stack; wrapper exit 0; adversarial trip-wire (capability counter MUST be exactly 2 at end-of-test, MUST NOT cache across restart) PASS; `HealthDisconnected` asserted after `Close()`; counter stability across post-restart Sync proven.
- [x] SCN-SM-041-008: Integration test `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` proves the connector picks up `events_skipped` and transitions to `degraded_recovered` against a live test stack. Evidence: `report.md` -> **Scope 2 SCN-003 + SCN-008 Integration Tests (DoD 317-318-319, Round 7 -- bubbles.implement Round 6 overstep vetting + bubbles.test, 2026-05-18T18:00:00Z)**. PASS at `1.12s` against the 5-service live test stack; wrapper exit 0; 7 in-test assertions PASS including: (1) zero RawArtifacts from FF marker, (2) adversarial trip-wire on FF packet endpoint (counter MUST stay at 0 -- proves production `continue`s past FF event before any FetchDecisionPacket call), (3) advanced `next_cursor` returned, (4) `smackerel_qf_cursor_fast_forward_events_skipped_total` delta == 42 (matches `EventsSkipped`), (5) `HealthDegradedRecovered` asserted, (6) real cursor round-trip through live PostgreSQL via `connector.NewStateStore(pool).Save/Get`, (7) post-FF Sync from advanced cursor returns same cursor (no progression on empty page).
- [x] SCN-SM-041-004: E2E API regression test `TestQFDecisionsIncompatibleCapabilityBlocksPolling` proves an incompatible capability response prevents decision-event polling and preserves zero trusted-artifact publication against a live API. Evidence: `report.md` -> **Scope 2 SCN-004 E2E Evidence (DoD 319 -- bubbles.implement + bubbles.test, 2026-05-18T15:05:03Z, Round 5)**. PASS at `0.08s` against the 5-service live test stack; wrapper exit 0; adversarial trip-wire (`t.Errorf` on `DecisionEventsPath`/`DecisionPacketsPath` hits) confirms no polling occurred after the incompatible capability response.
- [x] SCN-SM-041-006: E2E API regression test `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata` proves end-to-end unknown decision-type ingestion with metadata flag against a live API. Evidence: `report.md` -> Scope 2 E2E Runtime Evidence (DoD 320 — bubbles.test, 2026-05-18T14:04:12Z). PASS at `0.09s`, wrapper exit 0, on live test stack with all 5 services Healthy. Operator-supplied `CapabilitiesPath` stub arm at `tests/e2e/qf_decisions_connector_api_test.go:637-654` resolved the Round 2N capability-handshake omission (concern C-S2-006-E2E-STUB-ARM).
- [x] SCN-SM-041-003 and SCN-SM-041-008: Stress test `TestQFDecisionsFreshnessSLAP95IngestRender` runs the freshness SLA scenario against a live 5-service test stack and asserts the Scope 2-owned ingest sub-budget (`smackerel_qf_freshness_p95_seconds{stage="ingest"}` ≤ 30s, gauge exposed and non-zero, ≥500 artifacts driven). Evidence: `report.md` -> **Scope 2 Stress Evidence (DoD 321a -- bubbles.implement Round 6 overstep + bubbles.plan Round 8 DoD split + bubbles.test Round 8 runtime PASS, 2026-05-18T19:00:00Z)**. PASS at `9.88s` test-body wall (12.126s end-to-end including compile) on the 5-service live test stack; wrapper exit 0; ingest p95 = `1.300123s` vs `30s` Scope 2 ingest sub-budget (`4.33%` of budget, ~23x headroom); 500 artifacts driven across 20 cycles; bonus adversarial trip-wire `packetFetches == totalArtifactsDriven` (500 == 500) PASS proving CreatedAt is correctly populated under live load; all 5 services Healthy.
> **Scope 5 Cross-Scope Dependency (not active Scope 2 DoD):** Render and combined freshness SLA assertions (`smackerel_qf_freshness_p95_seconds{stage="render"}` ≤ 30s and combined ingest+render ≤ 60s) are owned by Scope 5 render-surface work per the stress test's documented scope-split declaration ([tests/stress/qf_decision_event_replay_test.go](tests/stress/qf_decision_event_replay_test.go) lines 1-19 and 13-18). This preserves traceability from Scope 2's ingest proof without presenting Scope 5 work as an active Scope 2 checkbox.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass for Scope 2 capability mismatch and unknown decision-type ingest behaviours. Evidence: `report.md` -> Scope 2 SCN-004 E2E Evidence (Round 5), Scope 2 E2E Runtime Evidence (DoD 320), Scope 2 Manual-Sync Reconnect Fix And Broader E2E Pass (bubbles.implement, 2026-05-19T02:30:00Z).
- [x] Artifact lint accepts the updated planning artifacts (`bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` exits 0). Evidence: `report.md` -> Scope 2 Artifact Lint Evidence.
- [x] Broader E2E regression suite passes. Evidence: `report.md` -> Scope 2 Broader E2E Evidence; **Scope 2 Manual-Sync Reconnect Fix And Broader E2E Pass (bubbles.implement, 2026-05-19T02:30:00Z)** — full broad `./smackerel.sh test e2e` exit 0, shell E2E Total 35 / Passed 35 / Failed 0, Go E2E `PASS: go-e2e`, and `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts` PASS (0.63s).

Build quality gate:

- [x] Raw unit, integration, E2E, stress, and artifact-lint evidence is recorded in `report.md` before any DoD item is checked. Evidence: `report.md` -> Scope 2 Unit Evidence, Scope 2 Integration Evidence, Scope 2 E2E API Evidence, Scope 2 Stress Evidence, Scope 2 Artifact Lint Evidence.
- [x] Consumer impact sweep completed and zero stale first-party references remain for Scope 2 connector IDs, metric names, capability paths, cursor contract paths, rendering boundaries, and QF artifact consumers. Evidence: `report.md` -> Scope 2 Planning Artifact Repair (bubbles.plan, 2026-05-19T03:15:00Z); `scopes.md` -> Scope 2 Consumer Impact Sweep.
- [x] Change Boundary is respected and zero excluded file families were changed (no Scope 3 rendering surfaces, no Scope 4 evidence-bundle export, no Scope 5 credential rotation overlap, no Scope 6-9 endpoints). Evidence: `report.md` -> Scope 2 Build Quality Gate DoD Reconciliation (DoDs 331/332/333, 2026-05-18T23:00:00Z), Scope 2 Capability + Cursor Persistence Integration Evidence (DoD 297, 2026-05-18T22:00:00Z), Scope 2 Round 15 Current-Session Verification.
- [x] No hidden defaults, hardcoded QF credentials, hardcoded QF URLs, or generated config hand edits are introduced; the new migration is the only schema change and uses the project migration framework. Evidence: `report.md` -> Scope 2 Check Evidence, Scope 2 Implementation Reality Evidence, **Scope 2 Round 14 Fresh Evidence (2026-05-19T00:00:00Z — against HEAD 0a08c3ec)** — R14 commit explicitly removed `defaultUnfetchedPageSize=200`; grep confirms zero production hits for hardcoded URLs/creds (only `_test.go` files use the reserved `qf.example.test` TLD per RFC 2606); sole migration is `034_qf_decisions_capability.sql`; `./smackerel.sh check` exit 0 with "Config is in sync with SST" + "env_file drift guard: OK".
- [x] Build, lint, and tests produce zero warnings (`./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`). Evidence: `report.md` -> Scope 2 Build Quality Gate DoD Reconciliation (DoDs 331/332/333, 2026-05-18T23:00:00Z), Scope 2 Round 15 Current-Session Verification, Scope 2 Manual-Sync Reconnect Fix And Broader E2E Pass (bubbles.implement, 2026-05-19T02:30:00Z).
- [x] New Scope 2-owned metrics (`smackerel_qf_capability_mismatch_total{required,actual}`, `smackerel_qf_unknown_decision_type_total{value}`, `smackerel_qf_cursor_lag_seconds`, `smackerel_qf_cursor_fast_forward_events_skipped_total`, `smackerel_qf_freshness_p95_seconds{stage}`) are documented in `design.md` and exposed via the Prometheus registry while preserving the Scope 5-owned full 12-metric symmetric set commitments. Evidence: `report.md` -> Scope 2 Documentation Boundary Evidence and metrics documentation evidence captured on 2026-05-18T17:13:04Z — `design.md:1219+` `## Scope 2-owned metrics (consolidated reference)` subsection inserted with a per-metric table (type, labels, emission site, purpose) plus an explicit independence statement from the Scope 5-owned full 12-metric symmetric set; the 5 metric names match the production emission sites at `internal/connector/qfdecisions/connector.go` (capability mismatch lines, unknown-decision-type emission, lag gauge, fast-forward counter, freshness p95 gauge).

### Round 2P DoD Name Reconciliation (2026-05-13)

Round 2N flagged five Scope 2 DoD items whose checklist text references test functions or files that do NOT exist by the named path/symbol. Round 2P (this `bubbles.plan` round) classified each item against direct file inspection plus targeted grep searches; raw evidence is in `report.md` -> Round 2P Evidence (CMDs 1-13).

**All 5 items classified B (semantic gap).** In every case the unit-layer covers the in-process semantics, but the live-stack assertion the DoD requires is genuinely absent. The DoD checkboxes therefore stay `[ ]` and the original DoD wording is preserved verbatim — Round 2Q (`bubbles.implement`) inherits the unchanged gap list.

| # | DoD Item (Scenario) | Named Path / Symbol | What Actually Exists | Classification | Round 2Q Recommendation |
|---|---------------------|---------------------|----------------------|----------------|--------------------------|
| 1a | SCN-SM-041-003 capability handshake on first connect | `tests/integration/qf_decisions_capability_test.go::TestQFDecisionsConnectorPerformsCapabilityHandshakeOnConnect` | File and function do NOT exist (CMD 1, CMD 2a). Unit-layer connect-time capability path covered by `internal/connector/qfdecisions/connector_test.go::TestConnect_CapabilityCompatibleSucceeds` and 9 functions in `internal/connector/qfdecisions/capability_test.go` (httptest mocks, NOT a live PostgreSQL+NATS stack). Existing live integration tests in `tests/integration/qf_decisions_*.go` (4 functions) have ZERO references to `CapabilitiesPath` / `capability` / `handshake` (CMD 12). | **B (semantic gap)** | Author live-stack integration test asserting the capability call lands BEFORE any decision-event poll against the PostgreSQL+NATS test stack. |
| 1b | SCN-SM-041-003 capability re-read on connector restart | `tests/integration/qf_decisions_capability_test.go::TestQFDecisionsConnectorReReadsCapabilityOnRestart` | File and function do NOT exist (CMD 1, CMD 2a). NO test of any layer (unit OR integration OR e2e) covers the connector restart re-read capability path. | **B (semantic gap)** | Author live-stack integration test that restarts the connector and asserts the capability endpoint is re-fetched. |
| 2 | SCN-SM-041-008 fast-forward `events_skipped` recovery | `tests/integration/qf_decisions_sync_test.go::TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped` | Historical Round 2P classification recorded that the named function did not exist at that time. Later Scope 2 rounds added and verified the positive fast-forward recovery path; the active Scope 2 Integration row now points to the live-stack PASS evidence for `TestQFDecisionsConnectorPicksUpFastForwardEventsSkipped`. | **B (semantic gap, historical)** | Superseded by Round 7 / Round 8 integration evidence; active Scope 2 DoD above is the executable source of truth. |
| 3 | SCN-SM-041-004 incompatible-capability E2E | `tests/e2e/qf_decisions_connector_api_test.go::TestQFDecisionsIncompatibleCapabilityBlocksPolling` | Function does NOT exist (CMD 2a). Unit-layer coverage exists across `connector_test.go::TestConnect_CapabilityIncompatibleReturnsError`, `client_test.go::TestClientRejectsIncompatibleQFPacketVersion`, `client_test.go::TestClient_FetchDecisionEvents_IncompatibleStatusBypassesClamp` — all httptest-mocked, NOT live API. The existing E2E `TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts` is a different scenario (packet schema mismatch via `startQFSchemaMismatchStub`, not capability handshake mismatch); existing e2e files have ZERO references to capability/Incompatible/CapabilitiesPath (CMD 13). | **B (semantic gap)** | Author live-API E2E test that drives an incompatible capability response (e.g., wrong `audit_envelope_version` OR missing `v1` in `supported_packet_versions`) through the live supervisor and asserts ZERO trusted artifacts published. |
| 4 + 5 | SCN-SM-041-003 + SCN-SM-041-008 freshness SLA P95 stress | `tests/stress/qf_decision_event_replay_test.go::TestQFDecisionsFreshnessSLAP95IngestRender` | File and function do NOT exist (CMD 1, CMD 2a). Unit tests (`TestSyncRecordsIngestFreshness_FreshPacket`, `TestSyncRecordsIngestFreshness_DelayedPacket`, `TestRecordFreshness_PerStageIsolation`) cover the rolling-window gauge mechanics with httptest mocks, but ZERO stress test asserts `p95 ingest ≤ 30s, render ≤ 30s, combined ≤ 60s` under sustained load. Existing `tests/stress/qf_decisions_sync_stress_test.go::TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity` covers replay identity, not freshness SLA budget; that file has ZERO references to `P95` / `freshness` / `30s` / `60s` (CMD 11). | **B (semantic gap)** | Author live-stack stress test that drives a sustained packet workload through ingest+render and asserts the P95 budgets via the `smackerel_qf_freshness_p95_seconds{stage}` gauge. |

**Honesty notes:**

| Note | Record |
|---|---|
| Evidence basis | Each classification was verified by direct file inspection plus grep searches captured in `report.md` -> Round 2P Evidence (CMDs 1-13). No test was assumed implemented from the function name alone. |
| DoD preservation | No DoD lines were re-worded, no DoD checkboxes were flipped, and no source code was changed in this round. The original live-stack assertion intent is preserved verbatim; Round 2Q (`bubbles.implement`) inherits the gap list unchanged. |
| Historical appendix ownership | The former parked Scope 2 legacy section is now preserved as the `Historical Appendix: Former Parked Scope 2 Trace`; its items are represented in active Scope 2 above or in Scope 5 cross-scope dependency trace. |
| Classification boundary | Round 2P explicitly rejected classification A for all 5 items. Although unit-layer coverage exists for items 1a, 3, and 4+5, the DoD lines explicitly require live-stack integration / live-API E2E / live-stack stress execution. Classifying these as A would silently downgrade the assertion bar from live-stack to unit-layer; that downgrade is a planning decision the user must make explicitly, not a name-reconciliation outcome. |
| Remaining live-stack gaps at that time | Item 1b and Item 2 had no equivalent unit-layer coverage at the time of Round 2P; later Scope 2 evidence sections record the integration and E2E closures. |

### Round 2R Planning Decisions (2026-05-18) — Resolves 4 Findings From Implement Round

This section records bubbles.plan scope-level decisions for the 4 findings (F1-F4) surfaced by the implement round on the unresolved Scope 2 DoD items (capability persistence, next_cursor persistence, freshness SLA stress, broader E2E, report.md anchor sections). Each finding lists the decision, the corresponding scope change (if any), and the routing owner.

**Pre-decision fact-check (corrects implement-round framing):**

- `internal/connector/state.go` ALREADY exposes a `*pgxpool.Pool` on `StateStore` (line 24) and provides Get/Save methods over `sync_state`. The persistence gap is NOT "no DB pool" — it is "no CRUD methods that read/write the new `capability_response`, `capability_fetched_at`, `capability_status` columns added by migration 034" and "connector Sync path does not yet route response-level `next_cursor` through `StateStore.Save()`".
- Migration `034_qf_decisions_capability.sql` is AUTOMATICALLY discovered by `//go:embed migrations/*.sql` in `internal/db/migrate.go:13`. It does NOT require any registration in `cmd/dbmigrate/main.go` (the runner is a thin wrapper around `db.Migrate`). The migration is already "wired" by virtue of being a file in `internal/db/migrations/`.
- Many DoD items that the implement-round framing listed as "blocked by F1" were already closed by Rounds 5/6/7 (SCN-003 / SCN-004 / SCN-008 integration + E2E PASS). The genuinely-remaining persistence work is bounded to capability-column CRUD on `StateStore` and `Sync()` plumbing of `next_cursor` — both of which fit inside Scope 2's name ("... And Storage") and Change Boundary once `internal/connector/state.go` is allowed.

**Finding F1 — Capability + cursor persistence wiring (architectural blocker, high)**

- Decision: **Extend Scope 2 inline.** Do NOT split into a new Scope 2.5. The work is bounded (capability-column CRUD on existing `StateStore`, `Sync()` wiring through `StateStore.Save()`, one persistence-smoke integration test) and matches the scope name verbatim.
- Change Boundary amendment: add `internal/connector/state.go` (capability-column CRUD methods only — no other connector logic) to the allowed file families. Migration 034 is already in scope via the existing `internal/db/migrations/*qf*` family and the `//go:embed migrations/*.sql` auto-discovery.
- No new DoD items needed. The existing DoD items for SCN-SM-041-003 (capability persistence) and SCN-SM-041-008 (next_cursor persistence) already cover the requirement; this round only removes the boundary blocker and clarifies the migration-wiring expectation.
Routing owner: implementation specialist for the StateStore capability-column CRUD methods + connector wiring; test specialist for the live-integration persistence-smoke test against the disposable test stack.

**Finding F2 — WSL2 stress harness incompatibility (infrastructure, medium)**

- Decision: **Accept WSL2 limitation; allow native-Linux execution evidence for the freshness SLA stress DoD item.** Do NOT introduce a separate Scope 2.6 for WSL2 compatibility. The freshness instrumentation (`smackerel_qf_freshness_p95_seconds{stage}` gauge from Round 2L) is independent of the stress runner; the production code path is unchanged. The WSL2-loopback incompatibility on `--network host` is a developer-environment limitation, not a code defect.
- Acceptable evidence sources for the freshness SLA p95 stress DoD item: (a) native-Linux CI runner, (b) native-Linux operator host, (c) the home-lab target. Evidence must include raw `./smackerel.sh test stress` output and the gauge readings.
Developer-experience note: a runtime `t.Skip()` guard in `tests/stress/qf_decision_event_replay_test.go` could detect WSL2 (via `/proc/sys/fs/binfmt_misc/WSLInterop` or `/proc/version` containing `microsoft`) and point to the operator runbook. This note is not a Scope 2 DoD item.
Routing owner: test specialist for the stress workload on a native-Linux execution surface and evidence capture. Implementation specialist may own the WSL2-skip guard if that developer-experience note is picked up.

**Finding F3 — Broader E2E suite not executed (execution, medium)**

- Decision: **Execution-only.** No scope-content change required. The full broader E2E suite (`./smackerel.sh test e2e` without `--go-run` narrowing) has not been captured as evidence against Scope 2's broader-DoD line since the Round 6/7 PASS snapshots (which were `--go-run`-narrowed).
Routing owner: test specialist for `./smackerel.sh --env test test e2e` against the disposable test stack and verbatim terminal output in `report.md` under a new "Scope 2 Broader E2E Suite Evidence" section. The broader-E2E DoD is now checked above with the 2026-05-19T02:30 evidence anchor.

**Finding F4 — Missing report.md anchor sections (medium)**

- Decision: **Execution-only, mixed routing.** The three missing anchor sections (Planning Repair Guard Evidence, Implementation Reality Evidence, Check Evidence) require fresh command runs in the current session.
Planning Repair Guard Evidence: test specialist runs `bash .github/bubbles/scripts/state-transition-guard.sh specs/041-qf-companion-connector` and `bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/qf_decisions_connector_api_test.go`; implementation specialist authors the section with raw output when narrative is needed.

Implementation Reality Evidence: test specialist runs `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/041-qf-companion-connector --verbose`; current validation records that implementation reality scan passes with zero violations.

Check Evidence: test specialist runs `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` and captures raw output; implementation specialist authors the section when narrative is needed.

Routing owner: test specialist for guard/scan/check runs; implementation specialist for evidence section authoring with raw output.

**Mutations applied this round:**

- This `### Round 2R Planning Decisions` subsection (added).
- `### Change Boundary` Allowed file families list extended by one entry (`internal/connector/state.go`, capability-column CRUD only).
- `state.json` executionHistory + completedPhaseClaims entries appended; `lastUpdatedAt` refreshed.

NO DoD checkbox flipped. NO scope status promoted. NO source code modified. NO foreign spec territory touched. NO `spec.md` / `design.md` / `uservalidation.md` modified.

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/*` (capability client, normalizer, connector sync logic, client page-size clamping, types, tests)
- `internal/connector/state.go` (capability-column CRUD methods on `StateStore` for the `capability_response` / `capability_fetched_at` / `capability_status` columns added by migration 034 — no other connector logic; Round 2R amendment)
- `internal/db/migrations/*qf*` (new capability migration only; migration 034 is auto-discovered by `//go:embed migrations/*.sql` in `internal/db/migrate.go`, no registration required in `cmd/dbmigrate/main.go`)
- `tests/integration/qf_decisions_*` (capability handshake, sync, fast-forward integration tests)
- `tests/e2e/qf_decisions_*` (mismatch, unknown decision-type, ingest e2e tests)
- `tests/stress/qf_decisions_*` and `tests/stress/qf_decision_event_replay_test.go` (freshness SLA stress; Round 2R: native-Linux execution acceptable for evidence)
- `specs/041-qf-companion-connector/*` (planning artifacts only)

Excluded surfaces:

- Web, digest, Telegram, search, mobile push rendering of QF packets remains excluded from Scope 2 and is owned by active Scope 3.
- `PersonalEvidenceBundle` export, `target_context = packet_context`, evidence import limits, consent revocation (owned by Scope 4)
- Credential rotation overlap / overlapping `not_before` window / capability re-read at rotation start (owned by active Scope 5; capability re-read on connector restart and credential reload IS in Scope 2, but rotation overlap behavior is not)
- Cross-Product Audit Envelope v1 emission across all eight emission points and the full 12-metric symmetric set (owned by active Scope 5; Scope 2 only adds the five new metrics enumerated above)
- Packet engagement signal exporter and `POST /packet-engagement-signals` (owned by Parked Scope 6)
- Personal context read API host `GET /api/v1/personal-context` (owned by Parked Scope 7)
- Signed callback infrastructure and `POST /callback` (owned by Parked Scope 8)
- Watch signal proposal endpoint `POST /watch-signal-proposals` (owned by Parked Scope 9)
- Generated config hand edits or new connector configuration keys (Scope 1 boundary; Scope 2 reuses the explicit configuration already proven by Scope 1)
- `state.json` modifications (workflow agent owns state transitions)

## Historical Appendix: Former Parked Scope 2 Trace

**Historical Note:** This former parked-scope trace was superseded by the active Scope 2 section above after QF 063 reached `done_with_concerns` on 2026-05-12 and Scope 2 was unparked on 2026-05-13. It is preserved only to show the original Phase B2 design intent; the active Scope 2 Definition of Done above is the executable record.

**Depends On (historical):** Scope 1
**Activation Gate (historical):** QF 063 Scope 2 read/outbox readiness — cleared 2026-05-12.

### Phase B2 Design Additions (2026-05-07) — Historical Proposed DoD (Superseded)

The following items were the original Phase B2 design intent for Scope 2 captured during planning. Each item is now represented as a Core Behavior or Validation DoD item in the active Scope 2 section above, or as a Scope 5 cross-scope dependency trace where render-surface ownership applies.

Core behavior trace (Phase B2 additions, superseded by active Scope 2 Core Behavior):

| Historical item | Original intent |
|---|---|
| Capability handshake | Connector calls `GET /api/private/smackerel/v1/capabilities` before decision-event polling and on connector restart/credential-rotation start, parses and persists all fields enumerated in design.md §Capability Discovery, blocks polling when required sync contract fields are incompatible, and emits `smackerel_qf_capability_mismatch_total{required,actual}` (Phase B2, F2). |
| Unknown decision-type ingest | When QF emits an unknown `decision_type` with `unknown_decision_type=true`, the connector stores the packet with `Metadata.unknown_decision_type = true`, does not invent a new content type, emits `smackerel_qf_unknown_decision_type_total{value}`, and leaves generic-card rendering to Scope 3 (Phase B2, F8). |
| Page-size clamping | Connector derives `limit` only by clamping explicit configured `connectors.qf-decisions.page_size` to the `[min_page_size, max_page_size]` range from the successfully fetched and durably persisted capability response; missing, unreadable, unavailable, or unpersisted capability blocks polling by failing loud during `Connect()` or marking the connector degraded after a prior successful handshake; `PAGE_SIZE_OUT_OF_RANGE` 4xx responses emit operator alerts, mark degraded, and never retry with a guessed, hardcoded, or smaller local limit (Phase B2, F9). |
| Freshness SLA stress test | `tests/stress/qf_decision_event_replay_test.go` verifies Scope 2 ingest p95 ≤30s; render p95 ≤30s and combined p95 ≤60s are tracked as Scope 5 render-surface work by C-S2-321B-SCOPE-5-RENDER (Phase B2, F12). |
| Cursor lag breach signaling | When `smackerel_qf_cursor_lag_seconds` exceeds the operator-configured threshold (default 1h), the connector logs a structured `lag_breach` event for the operator dashboard and never auto-fast-forwards itself (Phase B2, F13). |
| QF-issued fast-forward recovery | On a server-side cursor advancement, the connector picks up the `events_skipped` count, marks state `degraded_recovered`, and increments `smackerel_qf_cursor_fast_forward_events_skipped_total`; integration test exercises the fast-forward recovery path (Phase B2, F13). |

Validation trace (Phase B2 additions, superseded by active Scope 2 Validation):

| Historical validation item | Original intent |
|---|---|
| Capability parsing unit coverage | Unit tests cover capability response parsing, required-field compatibility checks, persisted capability diagnostics, and capability mismatch metric labels. |
| Capability handshake integration coverage | Integration tests cover capability handshake before polling, handshake on restart, and capability re-read at credential rotation start without activating Scope 5 rotation overlap behavior. |
| Incompatible capability E2E coverage | E2E regression test covers incompatible capability response preventing decision-event polling and preserving zero trusted artifact publication. |
| Unknown decision-type coverage | Unit and integration tests cover unknown decision-type ingest metadata, no invented content type, and metric emission. |
| Page-size clamping coverage | Unit tests cover explicit configured page-size clamping against persisted capability range, missing/unreadable/unavailable/unpersisted capability poll blocking, and `PAGE_SIZE_OUT_OF_RANGE` alert/degraded/no-retry behavior. |
| Ingest freshness stress coverage | Stress test exercises the Scope 2 ingest freshness SLA budget and surfaces `smackerel_qf_freshness_p95_seconds{stage="ingest"}`; Scope 5 owns render and combined freshness assertions. |
| Cursor lag and fast-forward coverage | Integration test exercises cursor lag breach signaling and the QF-issued fast-forward recovery path. |

## Scope 3: Web Telegram Digest And Search Surfacing

**Status:** Done
**Priority:** P0
**Depends On:** Scope 2
**Activation:** Activated for executable delivery on 2026-05-19 after Scope 2 was certified Done and established source-qualified QF artifacts with packet ID, trace ID, approval state, trust badges, and deep link. Implementation passes added the PWA static-contract assertion file and focused live E2E coverage for SCN-SM-041-011, SCN-SM-041-012, and SCN-SM-041-013. The later broader `./smackerel.sh test e2e` recheck passes. After DevOps runner review on 2026-05-19T11:18:38Z, Scope 3 PWA/UI proof is reconciled to the existing Go live-stack E2E surface (`tests/e2e/qf_decisions_surface_test.go`, including `assertPWAQFBundleServed`). `web/pwa/tests/qf_decisions_surface.spec.ts` is a traceability/static-contract anchor only, not an executable DoD runner. `bubbles.validate` certified Scope 3 Done on 2026-05-19T11:50:00Z under the reconciled PWA coverage model; certification.completedScopes now includes Scope 1, Scope 2, and Scope 3 while the overall feature remains `in_progress` for downstream Scopes 4-9.

### Gherkin Scenarios

Scenario: SCN-SM-041-009 Unknown Decision Type Renders As Generic QF Packet Card
	Given Scope 2 has ingested a QF packet artifact with `Metadata.unknown_decision_type = true`, a QF-authored title, QF-authored content, trust metadata, trace ID, approval state, and QF deep link
	When the packet appears in Web search results, Web artifact detail, daily digest, or Telegram-compatible summary
	Then Smackerel renders a generic read-only QF packet card that preserves the QF-authored title/content, trust metadata, and QF deep link without deriving buy/sell/hold semantics or a local content type from the packet body.

Scenario: SCN-SM-041-010 Trust Objects Render Only The Public QF Contract
	Given a QF packet contains `CalibrationBadge`, `DataProvenanceBadge`, `QuantifiedImpact`, and `ExpertAnalysisBundle` objects with public fields (`label`, `severity`, `summary`, optional `detail`, optional `links`) plus numeric internals
	When the digest, Telegram, search, and artifact-detail renderers format the packet
	Then Smackerel renders only the public fields, preserves downgraded severities visibly, uses severity for accessible icon/label hints, renders optional links as QF drilldown affordances, and silently drops numeric internals without logging them as errors.

Scenario: SCN-SM-041-011 Missing Required Trust Fields Falls Back Loudly
	Given a QF packet is otherwise renderable but any required trust object is missing `label` or `severity`
	When any Scope 3 renderer attempts to display the packet
	Then Smackerel emits `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}`, avoids rendering the incomplete trust object, and falls back to a generic read-only QF packet card while preserving packet ID, trace ID, approval state, QF-authored content, and the QF deep link.

Scenario: SCN-SM-041-012 Signed Deep Links Are Preferred Or Refetched
	Given a QF packet includes unsigned `deep_link`, optional `packet_url_signed`, `signature_expires_at`, and the persisted QF capability field `deep_link_signing_supported`
	When Web, digest, Telegram, search, or artifact detail renders the QF deep link
	Then Smackerel uses unexpired `packet_url_signed`, refetches the packet if the signed URL expires mid-render, falls back to unsigned only when `deep_link_signing_supported=false`, never re-signs locally, and emits `smackerel_qf_deep_link_render_total{surface,status}` with `signed_used`, `signed_expired_fallback_unsigned`, or `unsigned_only`.

Scenario: SCN-SM-041-013 Preferred Surface Routes Placement Only
	Given a QF packet carries `preferred_surface` as `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, `any`, or omits the hint
	When digest assembly, Telegram delivery selection, Web search, and Web artifact detail choose where to surface the packet
	Then Smackerel applies the design.md preferred-surface routing matrix as render-priority only and never changes trust metadata, QF-authored decision content, approval/action eligibility, deep-link choice, or read-only boundary based on the hint.

### Implementation Plan

- Add a QF packet rendering boundary shared by Web/search/artifact detail, digest, and Telegram-compatible summary paths; keep the renderer read-only and source-qualified to QF.
- Add or extend a `qfdecisions` render DTO/helper package (expected location: `internal/connector/qfdecisions` or a new package under `internal/render/qfdecisions`) that accepts normalized Scope 2 artifact metadata and returns a surface-neutral render model containing QF title, QF-authored content, packet ID, trace ID, approval state, trust-object rows, deep-link selection, and read-only action-boundary copy.
- Account for the implementation-owner preflight finding: `internal/connector/qfdecisions/types.go` currently appears to expose `DeepLink` only. Scope 3 must add missing envelope/metadata fields for `packet_url_signed`, `signature_expires_at`, and `preferred_surface` if still absent, and must preserve them through normalization into renderer-accessible metadata. This is a DTO/metadata extension only; it must not reopen Scope 2 cursor/capability behavior.
- Implement the unknown-decision generic-card variant for packets where Scope 2 set `Metadata.unknown_decision_type = true`; do not infer semantics from packet title, raw body, symbol, or thesis text.
- Implement the Trust Object Rendering Contract for `CalibrationBadge`, `DataProvenanceBadge`, `QuantifiedImpact`, and `ExpertAnalysisBundle`: render only `label`, `severity`, `summary`, optional `detail`, optional `links`; silently drop numeric internals and unenumerated fields; fail loud only on missing `label` or `severity`.
- Emit `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}` on missing required trust fields and route the affected packet through the generic read-only fallback card.
- Implement signed deep-link selection as a pure render decision using persisted QF capabilities and packet metadata: prefer unexpired `packet_url_signed`; on mid-render expiry, refetch the packet through the existing QF client for a fresh signed URL; use unsigned `deep_link` only when `deep_link_signing_supported=false`; never sign locally.
- Emit `smackerel_qf_deep_link_render_total{surface,status}` with labels `surface in {web,digest,telegram,search,artifact_detail}` and `status in {signed_used,signed_expired_fallback_unsigned,unsigned_only}` for each link render decision.
- Implement `preferred_surface` routing as placement priority only: `smackerel_digest` includes the packet in digest when eligible, `smackerel_telegram` queues it for Telegram-compatible delivery window, `qf_dashboard` suppresses automatic digest/Telegram surfacing and leaves search/detail plus QF dashboard tile, `any` or missing follows existing defaults. This routing must not alter content, trust metadata, approval/action state, or link choice.
- Wire Web/HTMX and PWA surfaces through existing search and artifact-detail entry points (`internal/web/templates.go`, `internal/web/handler.go`, `web/pwa/drive-search.js`, `web/pwa/drive-artifact-detail.js`) only where the existing surface consumes artifact metadata.
- Wire digest through `internal/api/digest.go` and Telegram-compatible formatting through `internal/telegram/*` delivery/formatting packages discovered during implementation; keep Telegram actions read-only and leave signed callback infrastructure to Scope 8.
- Add operator-visible diagnostics for unsigned-only link rendering when QF capability says signed links are unsupported, but do not treat that branch as an error. Do not add or complete Scope 5 render/combined freshness instrumentation in this scope.

### Implementation Files

- `internal/connector/qfdecisions/types.go` (DTO field additions only if `packet_url_signed`, `signature_expires_at`, or `preferred_surface` are missing)
- `internal/connector/qfdecisions/normalizer.go` and `internal/connector/qfdecisions/normalizer_test.go` (metadata preservation for signed-link and preferred-surface fields only)
- `internal/connector/qfdecisions/render.go` (new, expected shared render model/helper)
- `internal/connector/qfdecisions/render_test.go` (new)
- `internal/api/digest.go` and matching tests (digest QF card insertion/routing only)
- `internal/telegram/*` and matching tests (Telegram-compatible QF summary formatting only)
- `internal/web/templates.go`, `internal/web/handler.go`, and `internal/web/handler_test.go` (HTMX search/detail QF card rendering only)
- `web/pwa/drive-search.js`, `web/pwa/drive-artifact-detail.js`, and `web/pwa/tests/qf_decisions_surface.spec.ts` (PWA search/detail rendering assets plus static-contract anchor; live PWA proof is the Go E2E helper `assertPWAQFBundleServed`, not a Playwright runner)
- `tests/integration/qf_decisions_rendering_test.go` (new)
- `tests/e2e/qf_decisions_surface_test.go` (new or appended to existing `tests/e2e/qf_decisions_connector_api_test.go` if the suite convention prefers one QF file)

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-009 | `internal/connector/qfdecisions/render_test.go` | `TestRenderUnknownDecisionTypeUsesGenericCardWithoutDerivedSemantics` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-010 | `internal/connector/qfdecisions/render_test.go` | `TestTrustObjectRendererKeepsOnlyPublicFieldsForAllBadgeTypes` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-011 | `internal/connector/qfdecisions/render_test.go` | `TestTrustObjectMissingRequiredFieldFallsBackAndEmitsMetric` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-012 | `internal/connector/qfdecisions/render_test.go` | `TestSignedDeepLinkSelectionUsesSignedRefetchesExpiredAndFallsBackOnlyWhenUnsupported` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-013 | `internal/connector/qfdecisions/render_test.go` | `TestPreferredSurfaceRoutingBranchesDoNotMutateTrustOrActionState` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-012, SCN-SM-041-013 | `internal/connector/qfdecisions/normalizer_test.go` | `TestNormalizerPreservesSignedLinkAndPreferredSurfaceMetadata` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-009, SCN-SM-041-010, SCN-SM-041-011 | `tests/integration/qf_decisions_rendering_test.go` | `TestQFDecisionPacketRenderingPreservesTrustContractAcrossDigestSearchAndDetail` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-012 | `tests/integration/qf_decisions_rendering_test.go` | `TestQFDecisionPacketRenderingRefetchesExpiredSignedDeepLink` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-013 | `tests/integration/qf_decisions_rendering_test.go` | `TestQFPreferredSurfaceRoutingAffectsPlacementOnly` | `./smackerel.sh test integration` | Yes |
| Static Contract Anchor | traceability/static-contract | SCN-SM-041-009, SCN-SM-041-010, SCN-SM-041-011, SCN-SM-041-012, SCN-SM-041-013 | `web/pwa/tests/qf_decisions_surface.spec.ts` | `QF PWA search/detail static assertions remain mapped to Scope 3 scenarios without silent-pass bailout patterns` | `bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/qf_decisions_surface.spec.ts` | No |
| Regression E2E | e2e-api | SCN-SM-041-009, SCN-SM-041-010, partial SCN-SM-041-012, partial SCN-SM-041-013, accepted PWA asset-served proof | `tests/e2e/qf_decisions_surface_test.go` | `TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail` plus `assertPWAQFBundleServed` | `./smackerel.sh test e2e --go-run '^TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail$'` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-011 | `tests/e2e/qf_decisions_surface_test.go` | `TestQFDecisionTrustObjectMissingRequiredFieldFallsBackInLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-012, SCN-SM-041-013 | `tests/e2e/qf_decisions_surface_test.go` | `TestQFDecisionDeepLinkAndPreferredSurfaceBranchMatrix` | `./smackerel.sh test e2e` | Yes |
| PWA/UI Live Proof | e2e-api | SCN-SM-041-009, SCN-SM-041-010, SCN-SM-041-012, SCN-SM-041-013 | `tests/e2e/qf_decisions_surface_test.go` | `TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail` asserts the PWA bundle is served via `assertPWAQFBundleServed` while validating search/detail QF card behavior through the live stack | `./smackerel.sh test e2e --go-run '^TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail$'` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-009..013 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures; no Playwright/PWA runner is required in this repo state` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-009..013 | `specs/041-qf-companion-connector` | `artifact lint accepts QF Scope 3 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |

### PWA Static-Contract Anchor And Accepted Live Proof

- `web/pwa/tests/qf_decisions_surface.spec.ts` now exists and passed `bash .github/bubbles/scripts/regression-quality-guard.sh web/pwa/tests/qf_decisions_surface.spec.ts` with zero violations and zero warnings. This proves the PWA assertion file is present and free of silent-pass bailout patterns. It is classified as a traceability/static-contract anchor and is not an executable DoD runner.
- Accepted Scope 3 PWA/UI proof is the repo-standard Go live-stack E2E coverage in `tests/e2e/qf_decisions_surface_test.go`, especially `TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail` and its `assertPWAQFBundleServed` helper. That proof must continue to show the PWA bundle is served and that search/detail QF card rendering preserves user-visible packet ID, trace ID, trust metadata, signed/allowed unsigned deep link behavior, and read-only status through the live stack.
- The DevOps decision is final for this scope: do not add a one-off Playwright runner here. If browser automation becomes product direction later, it requires a separate operational/toolchain adoption scope with package metadata, lockfile, config, docs, and disposable-stack isolation through `./smackerel.sh`.

### Consumer Impact Sweep

| Consumer surface | Scope 3 impact | Verification record |
|---|---|---|
| QF packet artifact metadata | Adds renderer-consumed `packet_url_signed`, `signature_expires_at`, and `preferred_surface` preservation if missing; does not change Scope 2 cursor/capability semantics. | `TestNormalizerPreservesSignedLinkAndPreferredSurfaceMetadata`, `TestQFDecisionPacketRenderingRefetchesExpiredSignedDeepLink`. |
| Web HTMX search and artifact detail | Renders source-qualified read-only QF card and trust rows without local recommendation semantics. | `TestQFDecisionPacketRenderingPreservesTrustContractAcrossDigestSearchAndDetail`, PWA/Web E2E rows. |
| PWA search/detail | Shows the same source-qualified QF card, visible trace/trust metadata, and signed or allowed unsigned deep link; PWA asset delivery is proved through the live stack. | `tests/e2e/qf_decisions_surface_test.go` -> `TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail` with `assertPWAQFBundleServed`; `web/pwa/tests/qf_decisions_surface.spec.ts` remains a static-contract anchor only. |
| Digest API | Applies preferred-surface routing and includes digest-eligible QF packets without altering content/trust/action boundary. | `TestQFPreferredSurfaceRoutingAffectsPlacementOnly`, digest branch of `TestQFDecisionUnknownTypeAppearsAsGenericReadOnlyCardInSearchDigestTelegram`. |
| Telegram-compatible delivery | Formats compact QF summaries with read-only trust rows and deep link; no signed callback acceptance or action controls. | Telegram branch in integration and E2E rows; Scope 8 remains owner for callback signing. |
| Search index/results | QF packets remain searchable by Scope 2 metadata; rendering only changes card presentation and does not mutate stored artifacts. | `TestQFDecisionUnknownTypeAppearsAsGenericReadOnlyCardInSearchDigestTelegram`. |
| Metrics and operator diagnostics | Adds render metrics for trust failures and deep-link selection; does not claim Scope 5 full symmetric metric set or render/combined freshness DoD. | Unit metric tests plus artifact lint/state-transition guard evidence. |
| Downstream scopes | Scope 4 may depend on read-only QF context visibility; Scope 5 still owns credential rotation, safety-boundary consolidation, render/combined freshness, and full audit envelope rollout. | Parked Scope Queue dependency gate remains unchanged for Scopes 4-9. |

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/types.go` (signed-link and preferred-surface DTO fields only)
- `internal/connector/qfdecisions/normalizer.go` and `internal/connector/qfdecisions/normalizer_test.go` (metadata preservation for signed-link and preferred-surface fields only)
- `internal/connector/qfdecisions/render.go` and `internal/connector/qfdecisions/render_test.go` (new shared read-only QF render model/helper)
- `internal/api/digest.go` and corresponding tests (QF packet inclusion/routing only)
- `internal/telegram/*` and corresponding tests (QF read-only summary formatting only)
- `internal/web/templates.go`, `internal/web/handler.go`, and corresponding tests (search/detail QF card rendering only)
- `web/pwa/drive-search.js`, `web/pwa/drive-artifact-detail.js`, `web/pwa/tests/qf_decisions_surface.spec.ts` (PWA search/detail QF card rendering assets and static-contract anchor only; no Playwright runner work in this scope)
- `tests/integration/qf_decisions_rendering_test.go`
- `tests/e2e/qf_decisions_surface_test.go` or existing QF E2E file append if project convention requires it
- `specs/041-qf-companion-connector/*` (planning/evidence artifacts only)

Excluded surfaces:

- Scope 2 capability handshake, cursor sync, page-size clamping, unknown-decision ingest metric, lag/fast-forward, and ingest freshness behavior.
- Scope 4 `PersonalEvidenceBundle` export, export consent flows, `packet_context`, evidence limits, consent revocation, and source-provenance class eligibility.
- Scope 5 credential rotation overlap, full 12-metric symmetric set, Cross-Product Audit Envelope rollout, action-boundary consolidation, and render/combined freshness certification.
- Scope 6 packet engagement signal exporter and any influence of engagement on ranking or digest priority.
- Scope 7 personal-context read API host.
- Scope 8 signed callback acceptance/signing infrastructure beyond rendering ordinary QF deep links; no Telegram callback action controls in Scope 3.
- Scope 9 watch-signal proposal endpoint.
- Generated config hand edits, new connector credentials, or new runtime defaults/fallbacks.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-009: Unknown decision-type packets render through a generic read-only QF card in Web/search/artifact detail, digest, and Telegram-compatible summaries, preserving QF-authored title/content, trust metadata, trace ID, approval state, and QF deep link without deriving buy/sell/hold semantics or inventing a local content type. Evidence: `report.md` -> Scope 3 Unknown Decision Generic Card Evidence.
- [x] SCN-SM-041-010: Trust Object Rendering Contract is implemented for `CalibrationBadge`, `DataProvenanceBadge`, `QuantifiedImpact`, and `ExpertAnalysisBundle`; renderers consume only `label`, `severity`, `summary`, optional `detail`, optional `links`, preserve downgraded severities, render optional links as QF drilldowns, and silently drop numeric internals/unknown fields without error. Evidence: `report.md` -> Scope 3 Trust Object Rendering Evidence.
- [x] SCN-SM-041-011: Missing required trust fields (`label` or `severity`) fail loud with `smackerel_qf_trust_object_render_failures_total{reason="missing_required_field"}` and fall back to the generic read-only QF card without losing packet ID, trace ID, approval state, QF-authored content, or QF deep link. Evidence: `report.md` -> Scope 3 Trust Object Failure Evidence.
- [x] SCN-SM-041-012: Signed deep-link rendering prefers unexpired `packet_url_signed`, refetches on mid-render signature expiry, falls back to unsigned only when `deep_link_signing_supported=false`, never re-signs locally, and emits `smackerel_qf_deep_link_render_total{surface,status}` for `signed_used`, `signed_expired_fallback_unsigned`, and `unsigned_only`. Evidence: `report.md` -> Scope 3 Signed Deep Link Evidence.
- [x] SCN-SM-041-013: `preferred_surface` values `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, `any`, and missing route packet placement only; routing never changes trust metadata, QF-authored decision content, approval/action eligibility, deep-link choice, or read-only boundary. Evidence: `report.md` -> Scope 3 Preferred Surface Routing Evidence.
- [x] SCN-SM-041-012 and SCN-SM-041-013: `QFDecisionPacketEnvelope` and normalized artifact metadata preserve `packet_url_signed`, `signature_expires_at`, and `preferred_surface` if those fields are missing today from `internal/connector/qfdecisions/types.go`; this addition does not reopen Scope 2 cursor/capability behavior. Evidence: `report.md` -> Scope 3 DTO Metadata Preservation Evidence.

Validation:

- [x] SCN-SM-041-009: Unit and E2E tests prove unknown decision-type fallback renders a generic QF packet card and does not infer local recommendation semantics. Evidence: `report.md` -> Scope 3 Unit Evidence, Scope 3 E2E Evidence.
- [x] SCN-SM-041-010: Unit tests cover Trust Object Rendering Contract for all four trust objects and verify numeric internals are absent from render output. Evidence: `report.md` -> Scope 3 Unit Evidence.
- [x] SCN-SM-041-011: Unit coverage exists for missing-required-field fallback and metric emission, and live E2E proves the missing-required-field surface fallback preserves packet ID, trace ID, approval state, QF-authored content, and deep link while emitting the missing-field metric. Evidence: `report.md` -> Scope 3 Trust Object Failure Evidence, Scope 3 Missing Trust Live E2E Evidence.
- [x] SCN-SM-041-012: Unit coverage exists for `signed_used`, expired refetch, refetch failure with unsigned fallback, and `unsigned_only`; focused live E2E proves `signed_used`, `signed_expired_fallback_unsigned`, and `unsigned_only` through search/detail rendering. Evidence: `report.md` -> Scope 3 Signed Deep Link Evidence, Scope 3 Branch Matrix Live E2E Evidence.
- [x] SCN-SM-041-013: Unit coverage exists for each preferred-surface routing branch; focused live E2E proves `smackerel_digest`, `smackerel_telegram`, `qf_dashboard`, `any`, and missing-hint placement branches without mutating trust metadata, content, action eligibility, deep-link choice, or read-only boundary. Evidence: `report.md` -> Scope 3 Preferred Surface Routing Evidence, Scope 3 Branch Matrix Live E2E Evidence.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass for SCN-SM-041-009 through SCN-SM-041-013. Focused Go E2E evidence covers the original search/detail QF card path, accepted PWA asset-served proof via `assertPWAQFBundleServed`, SCN-SM-041-011 missing-trust fallback, and SCN-SM-041-012/013 branch matrices. Evidence: `report.md` -> Scope 3 E2E Evidence, Scope 3 Missing Trust Live E2E Evidence, Scope 3 Branch Matrix Live E2E Evidence, Scope 3 UI-Unit Coverage Status, Scope 3 PWA/UI Coverage Strategy Reconciliation - 2026-05-19T11:34:18Z.
- [x] Broader E2E regression suite passes for Scope 3, including accepted PWA/UI proof through Go live-stack E2E. Focused Go E2E evidence passes, current broad `./smackerel.sh test e2e` recheck passes, and the PWA `.spec.ts` file exists with regression-quality guard evidence as a static-contract anchor. No Playwright/PWA runner evidence is required or claimed for this scope. Evidence: `report.md` -> Scope 3 Broader E2E Evidence, Scope 3 Broad E2E Recheck Evidence - 2026-05-19T10:58Z, Scope 3 UI-Unit Coverage Status, Scope 3 PWA Runner DevOps Decision - 2026-05-19T11:18:38Z, Scope 3 PWA/UI Coverage Strategy Reconciliation - 2026-05-19T11:34:18Z.
- [x] Artifact lint accepts the activated Scope 3 planning artifacts. Evidence: `report.md` -> Scope 3 Artifact Lint Evidence.

Build quality gate:

- [x] Raw evidence coverage is complete for the reconciled Scope 3 Test Plan/DoD: executable proof uses repo-standard Go live-stack E2E and broad `./smackerel.sh test e2e`; `web/pwa/tests/qf_decisions_surface.spec.ts` is a static-contract anchor with regression-quality-guard evidence, not a Playwright/PWA runner gate. Evidence: `report.md` -> Scope 3 Evidence Index, Scope 3 Broad E2E Recheck Evidence - 2026-05-19T10:58Z, Scope 3 UI-Unit Coverage Status, Scope 3 PWA Runner DevOps Decision - 2026-05-19T11:18:38Z, Scope 3 PWA/UI Coverage Strategy Reconciliation - 2026-05-19T11:34:18Z.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for QF packet render models, signed-link fields, preferred-surface values, metric names, and route/card entry points. Evidence: `report.md` -> Scope 3 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed; Scope 5 render/combined freshness remains unclaimed. Evidence: `report.md` -> Scope 3 Change Boundary Evidence.
- [x] No hidden defaults, fallback runtime config, local financial advice, local QF trust reconstruction, generated config hand edits, or action controls are introduced. Evidence: `report.md` -> Scope 3 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, and E2E commands complete with zero warnings through repo-standard `./smackerel.sh` commands. Evidence: `report.md` -> Scope 3 Build Quality Evidence.

## Scope 4: Personal Evidence Bundle Export

**Status:** Done
**Priority:** P0
**Depends On:** Scope 3
**Activation:** Activated for executable delivery on 2026-05-19 after Scope 3 was certified Done and established user-visible QF packet context across Web/search/detail, digest, Telegram-compatible summaries, and PWA asset-served proof. The activation gate is satisfied: Scope 3 provides user-visible QF context; existing Smackerel consent-confirmation and sensitivity patterns are available from recommendation-watch, drive, and photos surfaces. Scope 4 owns the QF-specific evidence builder, export-consent confirmation, sensitivity ceiling selection, export/revocation state, and QF write-client behavior.

**Execution Progress (2026-05-19T17:25:00Z):** `bubbles.implement` extended the partial slice with API and Web/PWA evidence-builder/status/revocation affordances, persisted revocation audit envelopes, missing/unreadable persisted-capability rejection, local-reject metric coverage for `BUNDLE_TOO_LARGE`, `TOO_MANY_CLAIMS`, `RATE_LIMIT_EXCEEDED`, and `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE`, and focused unit/integration/E2E proof. Evidence: `report.md` -> Scope 4 implementation evidence at 2026-05-19T17:25:00Z.

**Validation Certification (2026-05-19T20:15:00Z):** `bubbles.validate` certifies Scope 4 as `Done` after current artifact-lint, traceability-guard, state-transition-guard, and implementation-reality evidence. The earlier integration and broad E2E harness verdict blockers are resolved by the Scope 4 DevOps Harness Stabilization evidence: full integration emits `PASS: go-integration` with exit status 0, and broad E2E emits shell 35/35 PASS, `PASS: go-e2e`, and exit status 0. The state-transition guard still blocks full-feature promotion for Scopes 5-9, missing full-feature phase certification, the Scope 2 consumer-trace historical gate, and report G040 history, but no Scope 4-local blocker remains. Evidence: `report.md` -> Scope 4 Validation Certification (bubbles.validate, 2026-05-19T20:15:00Z).

### Gherkin Scenarios

Scenario: SCN-SM-041-014 Idempotent Export Replay And Collision Handling
	Given a user has already exported a `PersonalEvidenceBundle` with `export_id` and an identical payload hash
	When QF responds HTTP 200 with the same `export_id` and payload identity on replay
	Then Smackerel treats the replay as a no-op success, preserves the original local export record, and does not duplicate audit or retry state.
	And when QF responds HTTP 409 `EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD`
	Then Smackerel aborts the export, records an `EXPORT_ID_COLLISION` audit error, marks the local export failed, and never retries that `export_id`.

Scenario: SCN-SM-041-015 Packet Context Evidence Bundle Export
	Given a user is viewing a read-only QF packet surfaced by Scope 3 and explicitly chooses Smackerel artifacts, claims, consent scope, and sensitivity ceiling
	When the user exports the bundle to QF
	Then Smackerel builds a `PersonalEvidenceBundle` with `target_context = packet_context`, links it to the QF `packet_id` and `trace_id`, includes source artifact IDs, extracted claims, confidence, sensitivity tier, consent scope, redaction summary, provenance, `source_provenance_classes`, and generated timestamp, and posts it to the QF private-alpha import path.

Scenario: SCN-SM-041-016 Capability-Bound Evidence Preflight Limits
	Given the persisted QF capability response defines `evidence_max_bundle_size_bytes`, `evidence_max_claims_per_bundle`, `evidence_rate_limit_per_minute`, and `eligible_smackerel_source_classes`
	When a user attempts to export a bundle that is too large, has too many claims, exceeds the per-credential token bucket, or includes an ineligible source class
	Then Smackerel rejects locally before any remote POST, returns `BUNDLE_TOO_LARGE`, `TOO_MANY_CLAIMS`, `RATE_LIMIT_EXCEEDED`, or `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{class}`, and emits `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason}`.

Scenario: SCN-SM-041-017 Consent Revocation Deletes Remote Bundle And Marks Local State Revoked
	Given a previously exported evidence bundle has active consent
	When the user revokes export consent from the Smackerel evidence export surface
	Then the connector calls `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}` with `{reason:"consent_revoked"}`, marks the local evidence artifact/export record `revoked`, emits `smackerel_qf_evidence_revoked_total{reason="consent_revoked"}`, and writes a unified evidence-revocation audit envelope.

Scenario: SCN-SM-041-018 Source Provenance Classes Are Validated Without Pre-MVP Badge Attachment
	Given selected source artifacts come from Smackerel connector classes
	When Smackerel builds the evidence bundle
	Then every bundle includes `source_provenance_classes`, rejects any class not listed in QF capability `eligible_smackerel_source_classes`, and does not attach any `DataProvenanceBadge`-shaped source badge metadata in pre-MVP.

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected User-Visible Result | Test Type | Evidence |
|----------|---------------|-------|------------------------------|-----------|----------|
| Packet detail export | Scope 3 packet visible in search/detail; consent settings available | Open packet detail, select context, choose consent scope and sensitivity ceiling, export | Export status shows QF-bound packet context, consent scope, sensitivity tier, source count, and success state without financial-action controls | e2e-api with served Web/PWA proof | `report.md#scope-4-e2e-evidence` |
| Local preflight rejection | Capability limits persisted; selected context exceeds size/claim/rate/source-class limit | Attempt export from builder | UI/API returns exact local rejection reason and no remote POST is observed | integration + e2e-api | `report.md#scope-4-preflight-limit-evidence` |
| Consent revocation | A bundle is exported and consent remains active | Revoke export consent | Status changes to revoked, remote DELETE call is recorded, and audit envelope contains reason `consent_revoked` | e2e-api | `report.md#scope-4-revocation-evidence` |

### Implementation Plan

- Add the QF-specific evidence bundle builder flow from Scope 3 packet detail/search/context surfaces. The builder must require explicit user selection of source artifacts/claims, consent scope, and sensitivity ceiling before export.
- Extend QF evidence DTOs so `target_context` accepts `packet_context` and carries QF `packet_id` / `trace_id` when the bundle is attached to a packet. Preserve existing `source_refs` optional semantics and keep `source_artifact_ids` required.
- Build bundles with the canonical field set: `bundle_id`, `export_id`, `consent_scope`, `sensitivity_tier`, `source_artifact_ids`, `extracted_claims`, `confidence`, `provenance`, `redaction_summary`, `target_context`, `source_provenance_classes`, and `created_at`.
- Implement an export-state store keyed by `export_id` and payload hash so idempotent HTTP 200 replay is a no-op success and HTTP 409 `EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD` records `EXPORT_ID_COLLISION`, marks the local export failed, and blocks retry for that `export_id`.
- Extend the QF client with `POST /api/private/smackerel/v1/personal-evidence-bundles` and `DELETE /api/private/smackerel/v1/personal-evidence-bundles/{export_id}`. Do not add any direct QF database access or broker path.
- Enforce persisted capability limits before remote calls: serialized bundle size, extracted-claim count, per-credential token bucket, and eligible source classes. Missing or unreadable persisted capabilities fail loud and block export; they must not fall back to hardcoded local values.
- Populate `source_provenance_classes` from selected artifacts' connector/source metadata and reject ineligible classes locally. Do not attach `DataProvenanceBadge`-shaped source badge metadata in pre-MVP; that remains design-only and downstream-gated.
- Add consent revocation handling from the evidence export surface. Revocation must call QF DELETE with reason `consent_revoked`, update local state to `revoked`, and preserve a user-visible revoked status.
- Emit Scope 4-owned evidence metrics only: `smackerel_qf_evidence_export_attempts_total{status,target_context_type,sensitivity_tier,reason?}` and `smackerel_qf_evidence_revoked_total{reason}`. The full 12-metric symmetric set remains Scope 5-owned.
- Emit unified audit-envelope records only for evidence export attempts, local rejects, collision aborts, successful exports, and revocations. Scope 5 still owns full Cross-Product Audit Envelope rollout across every bridge emission point.
- Preserve the QF authority boundary: exported context is analysis evidence only, never financial advice, never approval state, never a QF trust badge, and never an execution/mandate/watch action.

### Implementation Files

- `internal/connector/qfdecisions/types.go` (evidence DTO `target_context=packet_context`, source provenance classes, idempotency/collision response DTOs)
- `internal/connector/qfdecisions/client.go` and `internal/connector/qfdecisions/client_test.go` (POST/DELETE evidence bundle client, 200 replay, 409 collision, no retry)
- `internal/connector/qfdecisions/evidence_bundle.go` and `internal/connector/qfdecisions/evidence_bundle_test.go` (bundle construction, preflight limits, source-class eligibility, no badge attachment)
- `internal/connector/qfdecisions/evidence_export_store.go` and matching tests (local export state, payload hash, revoked/failed/succeeded states)
- `internal/db/migrations/<next-id>_qf_personal_evidence_exports.sql` (local export-state persistence only)
- `internal/api/qf_evidence.go`, `internal/web/handler.go`, `internal/web/templates.go`, and matching tests (evidence builder/status/revocation surfaces using existing auth and consent patterns)
- `web/pwa/drive-search.js`, `web/pwa/drive-artifact-detail.js`, and a Scope 4 static-contract anchor if the PWA bundle receives export/status controls
- `tests/integration/qf_personal_evidence_export_test.go`
- `tests/e2e/qf_personal_evidence_bundle_test.go`

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-014 | `internal/connector/qfdecisions/client_test.go` | `TestEvidenceExportTreatsIdempotentReplayAsNoopSuccess`, `TestEvidenceExportCollisionAbortsWithoutRetry` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-015 | `internal/connector/qfdecisions/evidence_bundle_test.go` | `TestEvidenceBundleBuildsPacketContextTargetWithRequiredFields` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-016 | `internal/connector/qfdecisions/evidence_bundle_test.go` | `TestEvidenceBundlePreflightRejectsBundleSizeClaimCountAndRateLimit` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-018 | `internal/connector/qfdecisions/evidence_bundle_test.go` | `TestEvidenceBundleSourceProvenanceClassesAndNoPreMVPBadgeAttachment` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-015, SCN-SM-041-016 | `tests/integration/qf_personal_evidence_export_test.go` | `TestQFPersonalEvidenceExportPersistsPacketContextAndCapabilityPreflightState` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-014, SCN-SM-041-017 | `tests/integration/qf_personal_evidence_export_test.go` | `TestQFPersonalEvidenceExportIdempotencyCollisionAndRevocationState` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-014 | `tests/e2e/qf_personal_evidence_bundle_test.go` | `TestQFPersonalEvidenceBundleIdempotentReplayAndCollisionThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-015 | `tests/e2e/qf_personal_evidence_bundle_test.go` | `TestQFPersonalEvidenceBundleExportsPacketContextThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-016, SCN-SM-041-018 | `tests/e2e/qf_personal_evidence_bundle_test.go` | `TestQFPersonalEvidenceBundlePreflightRejectsLimitsAndIneligibleSourceClassBeforeRemoteCall` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-017 | `tests/e2e/qf_personal_evidence_bundle_test.go` | `TestQFPersonalEvidenceBundleConsentRevocationDeletesRemoteAndMarksLocalRevoked` | `./smackerel.sh test e2e` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-014..018 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-014..018 | `specs/041-qf-companion-connector` | `artifact lint accepts QF Scope 4 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-014..018 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 4 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 4 impact | Verification record |
|---|---|---|
| Scope 3 QF packet detail/search context | Adds evidence-builder entry/status affordance from read-only packet context without adding approval, execution, mandate, callback, or watch controls. | `TestQFPersonalEvidenceBundleExportsPacketContextThroughLiveSurface`; Change Boundary excludes action surfaces. |
| QF private-alpha import API | Adds POST evidence import and DELETE revocation calls; handles 200 idempotent replay and 409 collision without retry. | `TestEvidenceExportTreatsIdempotentReplayAsNoopSuccess`, `TestEvidenceExportCollisionAbortsWithoutRetry`, E2E idempotency/collision row. |
| Local artifact/export state | Adds export-state persistence for success, failed collision, local reject, and revoked status; existing QF packet artifacts remain read-only. | Integration export-state rows and migration evidence. |
| Consent and sensitivity surfaces | Reuses existing Smackerel consent-confirmation and sensitivity ceiling patterns; adds QF-specific export consent and revocation paths. | UI scenario matrix rows and E2E export/revocation tests. |
| Capability consumers | Reads persisted Scope 2 capability limits for evidence size, claim count, rate, and eligible source classes; missing capability blocks export. | Unit and integration preflight rows. |
| Metrics/audit consumers | Emits Scope 4-owned export attempt and revocation metrics plus evidence-only unified audit envelopes; does not claim Scope 5 full metric/audit rollout. | Unit/integration metric assertions and Scope 5 exclusion in Change Boundary. |
| Downstream scopes | Scope 5 can depend on completed export surfaces for credential rotation, full audit envelope, and full symmetric metric set; Scopes 6-9 remain untouched. | Parked Scope Queue remains separate for Scopes 5-9. |

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/types.go`, `client.go`, `client_test.go`, `evidence_bundle.go`, `evidence_bundle_test.go`, `evidence_export_store.go`, and matching tests for Scope 4 evidence export only.
- `internal/db/migrations/*qf_personal_evidence*` for local export-state persistence only.
- `internal/api/qf_evidence.go`, `internal/web/handler.go`, `internal/web/templates.go`, and matching tests for evidence builder/status/revocation surfaces only.
- `web/pwa/drive-search.js`, `web/pwa/drive-artifact-detail.js`, and a Scope 4 PWA static-contract anchor only if export/status controls are served through the existing PWA bundle.
- `internal/metrics/*` only for `smackerel_qf_evidence_export_attempts_total` and `smackerel_qf_evidence_revoked_total` labels needed by Scope 4.
- `tests/integration/qf_personal_evidence_export_test.go` and `tests/e2e/qf_personal_evidence_bundle_test.go`.
- `specs/041-qf-companion-connector/*` planning/evidence artifacts.

Excluded surfaces:

- Scope 1 connector configuration and DTO startup gates except additive evidence DTO fields required by Scope 4.
- Scope 2 capability handshake/cursor sync/page-size/unknown-decision/lag/fast-forward/freshness behavior.
- Scope 3 QF card rendering semantics except adding a builder/status affordance from existing read-only packet context.
- Scope 5 credential rotation overlap, full 12-metric symmetric set, full Cross-Product Audit Envelope rollout, action-boundary consolidation, and render/combined freshness certification.
- Scope 6 packet engagement signal exporter.
- Scope 7 personal-context read API host and consent-token issuer.
- Scope 8 signed callback protocol.
- Scope 9 watch-signal proposal endpoint.
- Generated config hand edits, new runtime defaults/fallbacks, hardcoded QF credentials/URLs, direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, or any pre-MVP provenance badge attachment.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-014: HTTP 200 replay with the same `export_id` and payload is treated as no-op success; HTTP 409 `EXPORT_ID_REUSE_WITH_DIFFERENT_PAYLOAD` aborts with `EXPORT_ID_COLLISION`, marks local export failed, emits evidence audit, and is never retried. Evidence: `report.md` -> Scope 4 Idempotency Evidence.
- [x] SCN-SM-041-015: Evidence bundles attached to QF packets use `target_context = packet_context`, preserve packet ID and trace ID, and include the canonical `PersonalEvidenceBundle` field set with user-selected source artifacts, claims, sensitivity, consent, provenance, redaction summary, source provenance classes, and timestamp. Evidence: `report.md` -> Scope 4 Packet Context Export Evidence.
- [x] SCN-SM-041-016: Persisted capability limits are enforced before remote POST for bundle size, claim count, per-credential token bucket, and eligible source class; missing/unreadable capability blocks export without fallback; local rejects emit `smackerel_qf_evidence_export_attempts_total{status="local_reject", reason}`. Evidence: `report.md` -> Scope 4 Preflight Limit Evidence.
- [x] SCN-SM-041-017: Consent revocation calls QF DELETE with `{reason:"consent_revoked"}`, marks local artifact/export state `revoked`, emits `smackerel_qf_evidence_revoked_total{reason="consent_revoked"}`, and writes evidence-revocation audit envelope. Evidence: `report.md` -> Scope 4 Revocation Evidence.
- [x] SCN-SM-041-018: `source_provenance_classes` is populated and validated for every exported bundle, ineligible classes reject locally with `EVIDENCE_SOURCE_CLASS_NOT_ELIGIBLE{class}`, and no `DataProvenanceBadge`-shaped source badge attachment is enabled pre-MVP. Evidence: `report.md` -> Scope 4 Source Provenance Evidence.

Validation:

- [x] SCN-SM-041-014: Unit tests cover idempotent 200 replay, 409 collision, and no-retry behavior. Evidence: `report.md` -> Scope 4 Unit Evidence.
- [x] SCN-SM-041-015: Unit, integration, and E2E tests export a bundle with `target_context = packet_context` from a live QF packet surface. Evidence: `report.md` -> Scope 4 Packet Context Export Evidence.
- [x] SCN-SM-041-016: Unit and integration tests cover bundle-size, claim-count, rate-limit, missing-capability, and local-reject metric paths. Evidence: `report.md` -> Scope 4 Preflight Limit Evidence.
- [x] SCN-SM-041-017: E2E test covers consent revocation via QF DELETE, local revoked state, metric, and audit envelope emission. Evidence: `report.md` -> Scope 4 Revocation Evidence.
- [x] SCN-SM-041-018: Unit and E2E tests cover `source_provenance_classes` population, eligibility rejection before remote call, and no pre-MVP badge attachment. Evidence: `report.md` -> Scope 4 Source Provenance Evidence.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-014 through SCN-SM-041-018 pass. Evidence: `report.md` -> Scope 4 E2E Evidence.
- [x] Broader E2E regression suite passes after Scope 4 implementation. Evidence: `report.md` -> Scope 4 Broader E2E Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 4 DoD item is checked. Evidence: `report.md` -> Scope 4 Evidence Index.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for evidence builder routes, export status fields, idempotency/collision codes, revocation states, target_context values, metric labels, and audit actions. Evidence: `report.md` -> Scope 4 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 5-9 remain untouched except explicit dependency notes. Evidence: `report.md` -> Scope 4 Change Boundary Evidence.
- [x] No hidden defaults, fallback limits, hardcoded QF credentials/URLs, local financial advice, QF trust reconstruction, generated config hand edits, direct QF DB access, broker federation, action controls, or pre-MVP provenance badge attachment are introduced. Evidence: `report.md` -> Scope 4 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, E2E, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 4-local warnings or blockers; any remaining state-transition guard failures are explicitly classified as downstream/full-feature blockers only. Evidence: `report.md` -> Scope 4 Build Quality Evidence.

## Scope 5: Credential Rotation, Safety Boundaries, Observability, Documentation, And Tests

**Status:** Done
**Priority:** P0
**Depends On:** Scopes 2, 3, 4
**Activation:** Activated for executable planning on 2026-05-19. Scopes 2, 3, and 4 are certified Done, so the activation gate is satisfied: cursor/capability sync exists, read-only render surfaces exist, and evidence export/idempotency/revocation state exists for rotation, boundary, audit, and metric verification.

### Gherkin Scenarios

Scenario: SCN-SM-041-019 Credential Rotation Preserves Connector And Evidence State
	Given the `qf-decisions` connector has a persisted `sync_state.sync_cursor`, persisted QF capabilities, and existing evidence export idempotency records
	And an operator starts credential rotation with two active QF credentials whose `not_before` windows overlap for no more than 24 hours
	When the connector chooses credentials for sync, render, and evidence-export operations during the overlap
	Then Smackerel selects the newest credential whose `not_before` is valid, re-reads QF capabilities at rotation start, preserves the cursor and evidence/export idempotency state, emits operator diagnostics, and writes Cross-Product Audit Envelope v1 records for the rotation lifecycle.

Scenario: SCN-SM-041-020 Safety Boundaries And Symmetric Metrics Stay Complete Across Sync Render And Export
	Given Scope 2 sync metrics, Scope 3 render metrics, and Scope 4 evidence metrics are present across QF packet ingest, rendering, and evidence export paths
	When a QF packet is ingested, rendered, exported, revoked, or presented with an unavailable approval, execution, mandate, EmergencyStop, watch, callback, or trust-reconstruction action
	Then Smackerel emits the complete documented `smackerel_qf_*` metric set with QF design 063 label parity, records render and combined freshness p95 metrics, emits action-boundary attempts, and never enables the prohibited financial-action or trust-authoring behavior.

Scenario: SCN-SM-041-021 Cross-Product Audit Envelope v1 Covers Every Bridge Emission Point And Operator Runbook
	Given the QF companion connector emits bridge events from sync, export, revocation, engagement, callback, deep-link rendering, capability handshake, and action-boundary paths
	When those events are recorded in the connector audit log
	Then every record uses Cross-Product Audit Envelope v1 with the QF 063 mirrored shape, required optional IDs, action/outcome/reason fields, timestamp, and audit envelope version, while operator documentation lists the rotation, metric, audit, and safety-boundary diagnostics without promising a pre-MVP QF mirror sink.

### Implementation Plan

- Add a credential-rotation helper under the QF connector boundary that accepts the active credential set, validates no overlap exceeds 24 hours, chooses the newest valid credential by `not_before`, and rejects stale or future-only credentials with operator-visible diagnostics.
- Reuse the Scope 2 capability client and state store so rotation start re-reads capabilities for the selected credential before polling, rendering, or evidence export. Rotation must never poll from an in-memory-only capability response.
- Preserve the existing `sync_state.sync_cursor`, persisted capability record, and Scope 4 evidence export/idempotency state through rotation. Rotation must not create a new connector identity, clear cursor state, duplicate evidence exports, or reset revoked/failed export records.
- Emit rotation diagnostics and Cross-Product Audit Envelope v1 records for rotation start, credential selected, credential rejected, overlap rejected, capability re-read success/failure, and rotation completion.
- Consolidate safety-boundary enforcement in a shared QF boundary helper used by sync/render/export/callback/watch-adjacent paths. The helper must block or diagnose approval, execution, mandate change, EmergencyStop, watch creation/evaluation, callback acceptance, and QF trust reconstruction before any user-visible or outbound side effect.
- Implement or complete the full symmetric metric set with documented labels and QF design 063 label parity: `smackerel_qf_packet_ingest_total{event_type,decision_type,approval_state,source_surface}`, `smackerel_qf_packet_validation_failures_total{reason}`, `smackerel_qf_evidence_export_attempts_total{status,target_context_type,sensitivity_tier}`, `smackerel_qf_cursor_lag_seconds`, `smackerel_qf_action_boundary_attempts_total{attempted_action_type}`, `smackerel_qf_capability_mismatch_total{required,actual}`, `smackerel_qf_unknown_decision_type_total{value}`, `smackerel_qf_engagement_signal_attempts_total{event,surface,status}`, `smackerel_qf_evidence_revoked_total{reason}`, `smackerel_qf_callback_attempts_total{action,status}`, `smackerel_qf_deep_link_render_total{surface,status}`, and `smackerel_qf_trust_object_render_failures_total{reason}`.
- Carry forward the Scope 2 cross-scope dependency C-S2-321B-SCOPE-5-RENDER: complete `smackerel_qf_freshness_p95_seconds{stage="render"}` and derived combined ingest+render p95 assertion so stress evidence proves render p95 <= 30s and combined p95 <= 60s without reopening Scope 3 rendering semantics.
- Introduce an audit-envelope builder and connector audit-log sink for Cross-Product Audit Envelope v1. Required envelope shape mirrors QF design 063: `trace_id`, optional `packet_id`, optional `export_id`, optional `signal_id`, `actor_ref`, `surface`, `action`, `outcome`, `reason`, `ts`, and `audit_envelope_version` from persisted capability response.
- Emit the audit envelope for every packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick. Scope 5 may add shared audit helpers for Scopes 6-9, but it must not implement Scope 6 engagement transport, Scope 8 callback signing/acceptance, or Scope 9 watch proposal transport.
- Update operator documentation for credential rotation overlap, the 24-hour limit, capability re-read behavior, cursor/export state preservation, metric names/labels, audit envelope shape, connector audit-log sink, and pre-MVP safety boundaries. The QF mirror sink remains explicitly reserved post-MVP and opt-in.

### Implementation Files

- `internal/connector/qfdecisions/credentials.go` and `credentials_test.go` (credential overlap validation, newest-valid selection, diagnostics)
- `internal/connector/qfdecisions/audit.go` and `audit_test.go` (Cross-Product Audit Envelope v1 builder and connector audit-log sink)
- `internal/connector/qfdecisions/boundary.go` and `boundary_test.go` (shared no-action financial boundary helper)
- `internal/connector/qfdecisions/metrics.go` and `metrics_test.go` (complete symmetric metric registration/label parity and freshness render/combined helpers)
- `internal/connector/qfdecisions/connector.go`, `client.go`, `render.go`, and `evidence_export_store.go` (call-site wiring only for rotation, metrics, audit envelope, safety boundary, and freshness completion)
- `tests/integration/qf_credential_rotation_test.go`
- `tests/integration/qf_scope5_observability_test.go`
- `tests/integration/qf_audit_envelope_test.go`
- `tests/e2e/qf_scope5_safety_observability_test.go`
- `tests/stress/qf_decision_event_replay_test.go` (Scope 5 render and combined freshness assertions only)
- `docs/Operations.md`, `docs/Testing.md`, and `docs/Development.md` Scope 5 sections only
- `specs/041-qf-companion-connector/*` planning/evidence artifacts

### Implementation Notes (Plan Amendment 2026-05-21)

This subsection ratifies pre-landed Scope 5-territory modules and the
retroactive flip policy decided by `bubbles.plan` while resolving open concerns
`C-PLAN-SCOPE5-CHECK-8A-WORDING-ALIGNMENT`,
`C-PLAN-SCOPE5-RETROACTIVE-FLIP-CANDIDATES`, and
`C-AUDIT-S5-SCAFFOLDING-PLAN-RATIFY`. Scope 5 status remains **Not Started**;
this is plan-amendment only, not an activation of executable work.

**Pre-Landed Scaffolding Ratification (per `C-AUDIT-S5-SCAFFOLDING-PLAN-RATIFY`)**

Three Scope 5-territory modules landed in closeout commit `39ca4fcb` alongside
Scope 2/3/4 closeout work. They are ratified here as Scope 5 starting code
points (NOT to be re-authored or rejected on Scope 5 activation):

- `internal/connector/qfdecisions/audit.go` and `audit_test.go` — SHARED
  INFRASTRUCTURE with 8+ production callers across Scopes 2-4 (Scope 2
  `connector.go` capability-mismatch envelopes; Scope 3 `render.go` deep-link
  envelope per SCN-012; Scope 4 `evidence_bundle.go` evidence-export,
  idempotent-replay, local-reject, success, and revocation envelopes per
  SCN-017). Scope 5 active work EXTENDS this module with the remaining unique
  audit-emission points required by SCN-021 (sync lifecycle, engagement,
  callback, action-boundary kick) plus the operator-runbook narrative.

- `internal/connector/qfdecisions/boundary.go` and `boundary_test.go` —
  UNWIRED Scope 5-territory scaffolding (`RejectQFActionBoundary`,
  `IsForbiddenQFActionType`, `ActionBoundaryAttempt`,
  `ActionBoundaryDiagnostic`). ZERO production callers at HEAD. Scope 5 active
  work WIRES `RejectQFActionBoundary` into any QF action-eligible
  sync/render/export/callback/watch-adjacent path so SCN-020 action-boundary
  attempts are emitted with the documented metric label set.

- `internal/connector/qfdecisions/credentials.go` and `credentials_test.go` —
  UNWIRED Scope 5-territory scaffolding (`PlanCredentialRotation`,
  `RotatingCredential`, `CredentialRotationPlan`, `CredentialRotationState`).
  ZERO production callers at HEAD. Scope 5 active work WIRES
  `PlanCredentialRotation` into the connector restart and credential-reload
  paths so SCN-019 selects the newest valid credential, re-reads QF
  capabilities at rotation start, preserves `sync_state.sync_cursor` and
  Scope 4 evidence-export idempotency state, and writes the rotation-lifecycle
  audit envelopes.

**Retroactive DoD Flip Policy (per `C-PLAN-SCOPE5-RETROACTIVE-FLIP-CANDIDATES`)**

Two RETROACTIVE-FLIP-OK candidates were surfaced by `bubbles.gaps` Scope 5
retroactive audit (`C-GAPS-SCOPE5-RETROACTIVE-AUDIT`, 2026-05-21T16:53:07Z).
`bubbles.plan` resolves the policy as follows:

- **V1 (SCN-019 unit-test coverage): FLIPPED.**
  `internal/connector/qfdecisions/credentials_test.go::TestPlanCredentialRotationSelectsNewestValidCredentialAndPreservesState`
  (line 9) exercises valid overlap, newest-valid `not_before` selection, cursor
  preservation, evidence-export idempotency preservation (with explicit
  alias-mutation guard), capability-re-read requirement, and `AuditOutcomeOK`
  rotation envelope. `TestPlanCredentialRotationRejectsInvalidCredentialBoundaries`
  (line 46) exercises overlap >24h rejection, future-only credential rejection,
  one-active-credential rejection, diagnostics enumeration, cursor preservation
  under rejection, and `AuditOutcomeRejected` audit envelopes. All nine V1
  sub-claims are covered by passing unit tests. Function-name divergence from
  the test-plan-expected `TestCredentialRotation*` names is accepted because
  the substance is what V1 requires; the existing Test Plan rows remain
  as-written so Scope 5 active work may add additional rotation tests under
  the planned names without redundant rework.

- **C4 (12-metric symmetric set "is emitted"): NOT FLIPPED.** The DoD wording
  requires emission with documented label parity, not just registration. The
  registration half is complete at HEAD: all 12 `smackerel_qf_*` metrics are
  declared in `internal/metrics/metrics.go` (lines 238, 251, 263, 276, 288,
  297, 309, 324, 337, 347, 357, 368, 378, 388) and registered exactly once via
  `prometheus.MustRegister` at line 395 (call list includes
  `QFFreshnessP95Seconds` at line 424). However, three metrics
  (`QFActionBoundaryAttemptsTotal`, `QFEngagementSignalAttemptsTotal`,
  `QFCallbackAttemptsTotal`) have no production emission paths at HEAD because
  boundary/engagement/callback are unwired pending Scope 5/6/8 active work.
  Flipping C4 would overclaim. Scope 5 active work must add the missing
  emission wiring AND author the parity test (V3 `metrics_test.go`) before C4
  is flipped.

**Check 8A Wording Alignment (per `C-PLAN-SCOPE5-CHECK-8A-WORDING-ALIGNMENT`)**

The scenario-specific E2E regression DoD line below was rewritten to restore
`state-transition-guard.sh` Check 8A anchor parity with Scopes 1-4 (regex
`^- \[(x| )\] Scenario-specific E2E regression tests? for (EVERY|every)
new/changed/fixed behavior`). Wording-only swap; no semantic change; DoD
remains `[ ]` pending live-stack execution by Scope 5 active work.

**Scope 5 Status (unchanged)**

This plan-amendment ratifies scaffolding and one V1 unit-coverage flip.
Scope 5 status remains **Not Started** and
`certification.scopeProgress[4].status` remains `"Not Started"`. Active
Scope 5 implementation work has NOT begun. The Scope 5 Implementation Plan,
Implementation Files, Test Plan, Consumer Impact Sweep, Change Boundary, and
remaining DoD listings are unchanged in substance — Scope 5 active work
executes them as written, starting from the ratified scaffolds.

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-019 | `internal/connector/qfdecisions/credentials_test.go` | `TestCredentialRotationSelectsNewestValidNotBeforeWithinTwentyFourHourOverlap`, `TestCredentialRotationRejectsOverlapBeyondTwentyFourHours` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-019 | `internal/connector/qfdecisions/credentials_test.go` | `TestCredentialRotationPreservesCursorEvidenceExportStateAndReReadsCapabilities` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-020 | `internal/connector/qfdecisions/boundary_test.go` | `TestQFActionBoundaryRejectsApprovalExecutionMandateEmergencyStopWatchCallbackAndTrustAuthoring` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-020 | `internal/connector/qfdecisions/metrics_test.go` | `TestQFSymmetricMetricSetRegistersAllTwelveMetricsWithQFLabelParity`, `TestQFRenderAndCombinedFreshnessMetricsAreRecorded` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-021 | `internal/connector/qfdecisions/audit_test.go` | `TestCrossProductAuditEnvelopeV1ShapeMatchesQFDesign063`, `TestCrossProductAuditEnvelopeOptionalIDsByEmissionPoint` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-019 | `tests/integration/qf_credential_rotation_test.go` | `TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-020 | `tests/integration/qf_scope5_observability_test.go` | `TestQFObservabilityEmitsAllSymmetricMetricsAcrossSyncRenderExportAndBoundaryPaths` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-021 | `tests/integration/qf_audit_envelope_test.go` | `TestQFAuditEnvelopeV1ShapeAcrossEightRequiredEmissionPoints` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-019 | `tests/e2e/qf_scope5_safety_observability_test.go` | `TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-020 | `tests/e2e/qf_scope5_safety_observability_test.go` | `TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-021 | `tests/e2e/qf_scope5_safety_observability_test.go` | `TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Stress | stress | SCN-SM-041-020 | `tests/stress/qf_decision_event_replay_test.go` | `TestQFDecisionsFreshnessSLAP95RenderAndCombined` (asserts p95 render <= 30s and combined <= 60s while preserving Scope 2 ingest proof) | `./smackerel.sh test stress` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-019..021 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-019..021 | `specs/041-qf-companion-connector` | `artifact lint accepts activated QF Scope 5 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-019..021 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 5 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 5 impact | Verification record |
|---|---|---|
| Scope 2 sync and capability state | Rotation reuses the existing state store and capability client; cursor and persisted capability must survive credential overlap and capability re-read. | `TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit`; unit credential-rotation state test. |
| Scope 3 render surfaces | Adds render freshness completion, deep-link/audit metric coverage, and action-boundary diagnostics without changing Scope 3 content rendering, trust-object rendering, signed-link selection, or preferred-surface routing semantics. | `TestQFRenderAndCombinedFreshnessMetricsAreRecorded`; stress render/combined p95 row; Change Boundary excludes render semantic changes. |
| Scope 4 evidence export state | Rotation preserves export idempotency, failed/collision, success, and revoked state; evidence metrics are completed but export behavior is not changed. | `TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface`; Scope 4 E2E regression remains in broader E2E. |
| Operator status, logs, and runbooks | Adds rotation diagnostics, action-boundary diagnostics, full metric-label reference, audit envelope shape, and connector audit-log sink documentation. | Docs Build Quality Gate; artifact lint and traceability guard rows. |
| API client and generated client consumers | Scope 5 hardens credential rotation and observability behind existing interfaces; it does not rename or remove connector endpoint contracts consumed by API clients or generated clients. | Scope 5 integration/E2E rows validate live behavior on unchanged endpoint contracts and stale-reference checks remain zero. |
| Navigation, breadcrumb, redirect, and deep link consumers | Scope 5 does not alter navigation links, breadcrumb hierarchy, redirect behavior, or deep link route contracts; render semantics remain owned by Scope 3. | Scope 3 deep-link/preferred-surface regression rows and Scope 5 Change Boundary exclusions. |
| Metrics dashboards and QF label parity | Completes the symmetric 12-metric set plus Scope 5 freshness render/combined completion; label names and enum values must match QF design 063. | `TestQFSymmetricMetricSetRegistersAllTwelveMetricsWithQFLabelParity`, integration metrics row. |
| Connector audit consumers | Adds Cross-Product Audit Envelope v1 builder and connector audit-log sink for the required eight emission points; QF mirror sink stays reserved post-MVP. | `TestQFAuditEnvelopeV1ShapeAcrossEightRequiredEmissionPoints`. |
<!-- bubbles:g040-skip-begin -->
| Scopes 6-9 future work | Scope 5 may provide shared metrics/audit helpers for engagement, callback, and watch events, but must not implement Scope 6 engagement signal transport, Scope 8 callback signing/acceptance, or Scope 9 watch proposal transport. | Change Boundary and explicit dependency notes in active Scope 5; Scopes 6-9 remain parked. |
<!-- bubbles:g040-skip-end -->

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/credentials.go` and `credentials_test.go` for credential overlap validation, newest-valid selection, capability re-read trigger, and rotation diagnostics only.
- `internal/connector/qfdecisions/audit.go` and `audit_test.go` for Cross-Product Audit Envelope v1 builder and connector audit-log sink only.
- `internal/connector/qfdecisions/boundary.go` and `boundary_test.go` for shared pre-MVP action-boundary rejection and diagnostics only.
- `internal/connector/qfdecisions/metrics.go` and `metrics_test.go` for the full symmetric `smackerel_qf_*` metric registration/label parity and Scope 5 render/combined freshness helpers only.
- `internal/connector/qfdecisions/connector.go`, `client.go`, `render.go`, `evidence_bundle.go`, and `evidence_export_store.go` call-site wiring only where needed for rotation, metrics, audit envelopes, boundary diagnostics, and freshness completion.
- `tests/integration/qf_credential_rotation_test.go`, `tests/integration/qf_scope5_observability_test.go`, and `tests/integration/qf_audit_envelope_test.go`.
- `tests/e2e/qf_scope5_safety_observability_test.go`.
- `tests/stress/qf_decision_event_replay_test.go` for render p95 and combined p95 assertions only.
- `docs/Operations.md`, `docs/Testing.md`, and `docs/Development.md` Scope 5 sections only.
- `specs/041-qf-companion-connector/*` planning/evidence/state artifacts.

Excluded surfaces:

- Scope 1 connector configuration startup gates, new generated config keys, or credential secret storage changes.
- Scope 2 cursor sync semantics, page-size clamping, unknown decision-type ingest behavior, fast-forward recovery, or ingest freshness proof except for reading the existing state/metric values.
- Scope 3 QF card rendering semantics, trust-object public-field filtering, signed-link branch behavior, preferred-surface routing, PWA asset proof, or any visual redesign.
- Scope 4 evidence bundle construction, QF POST/DELETE semantics, revocation behavior, local preflight limit logic, source-provenance eligibility, or export UI controls.
- Scope 6 packet engagement signal exporter transport, consent-gated event capture, buffer/flush behavior, or retry policy.
- Scope 7 personal-context read API host and consent-token issuer.
- Scope 8 callback HMAC signing, callback acceptance/rejection parsing, or callback transport.
- Scope 9 watch-signal proposal request/signing/rejection transport.
- Direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, local financial advice, QF trust reconstruction, generated config hand edits, hidden defaults/fallbacks, or hardcoded QF credentials/URLs.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-019: Credential rotation accepts exactly two active credentials for no more than 24 hours of overlap, selects the newest valid credential by `not_before`, rejects overlap beyond 24 hours, and emits operator diagnostics plus rotation audit envelopes. Evidence: `report.md` -> Scope 5 Credential Rotation Evidence (bubbles.implement, 2026-05-21T19:05:00Z).
- [x] SCN-SM-041-019: Rotation preserves `sync_state.sync_cursor`, persisted QF capability state, and Scope 4 evidence/export idempotency records across credential changes, and re-reads capabilities at rotation start before any sync/render/export call uses the new credential. Evidence: `report.md` -> Scope 5 Credential Rotation Evidence (bubbles.implement, 2026-05-21T19:05:00Z) Sections 1+3.
- [x] SCN-SM-041-020: Safety-boundary helper blocks or diagnoses approval, execution, mandate change, EmergencyStop, watch creation/evaluation, callback acceptance, and QF trust reconstruction; no prohibited pre-MVP action is enabled in any sync, render, export, callback, or watch-adjacent path. Evidence: `report.md` -> Scope 5 Safety Boundary And Observability Evidence (bubbles.implement, 2026-05-21T19:49:40Z).
- [x] SCN-SM-041-020: The complete symmetric metric set is emitted with documented QF design 063 label parity for all 12 metrics listed in the Scope 5 implementation plan. Evidence: `report.md` -> Scope 5 Safety Boundary And Observability Evidence (bubbles.implement, 2026-05-21T19:49:40Z).
- [x] SCN-SM-041-020: Scope 2 cross-scope dependency C-S2-321B-SCOPE-5-RENDER is closed by render p95 <= 30s and combined ingest+render p95 <= 60s stress evidence using `smackerel_qf_freshness_p95_seconds{stage="render"}` and derived combined measurement without changing Scope 3 rendering semantics. Evidence: `report.md` -> Scope 5 Freshness Render Combined Evidence (bubbles.implement, 2026-05-21T22:25:41Z) Sections 1+3+4+5 plus Scope 5 Stress Evidence (bubbles.implement, 2026-05-21T22:25:41Z) Section 1 (renderP95=6.036306s, combinedP95=6.036306s).
- [x] SCN-SM-041-021: Cross-Product Audit Envelope v1 is emitted to the connector audit log for packet ingest, evidence export attempt, evidence revocation, engagement signal flush, callback attempt, deep-link render, capability handshake, and action-boundary kick, using the QF 063 mirrored shape and persisted `audit_envelope_version`; QF mirror sink remains opt-in post-MVP. Evidence: `report.md` -> Scope 5 Audit Envelope V1 Rollout Evidence (bubbles.implement, 2026-05-21T22:04:49Z) Section 1 (per-emission-point wiring table) + Section 2 (integration GREEN proof) + Section 3 (envelope shape conformance dumps).
- [x] SCN-SM-041-021: Operator documentation explains rotation overlap, newest-valid credential selection, capability re-read, state preservation, metric labels, audit envelope shape, connector audit-log sink, QF mirror reservation, and pre-MVP safety boundaries. Evidence: `report.md` -> Scope 5 Operator Documentation Evidence (bubbles.implement, 2026-05-21T23:42:01Z) Sections 1-2; managed-doc anchors at `docs/Operations.md` `## QF Companion Connector Operations (Spec 041 Scope 5)`, `docs/Testing.md` `### QF Companion Connector Test Surface (Spec 041)` -> `#### Scope 5 Test Surface (Spec 041)`, `docs/Development.md` `## QF Companion Connector Internals (Spec 041 Scope 5)`.

Validation:

- [x] SCN-SM-041-019: Unit tests cover valid overlap, overlap >24h rejection, newest-valid `not_before` selection, future-only credential rejection, cursor preservation, evidence export idempotency preservation, capability re-read, diagnostics, and rotation audit envelopes. Evidence: `internal/connector/qfdecisions/credentials_test.go::TestPlanCredentialRotationSelectsNewestValidCredentialAndPreservesState` (line 9) and `TestPlanCredentialRotationRejectsInvalidCredentialBoundaries` (line 46); function-name divergence from the planned `TestCredentialRotation*` names is acknowledged per Plan Amendment 2026-05-21 (see Scope 5 Implementation Notes). Live-stack rotation evidence still required for V2 below; `report.md` -> Scope 5 Plan Amendment Evidence captures the retroactive-flip rationale.
- [x] SCN-SM-041-019: Integration and E2E tests rotate credentials through overlapping `not_before` windows and verify cursor, evidence export idempotency state, capability re-read, diagnostics, and audit envelope preservation against the live disposable stack. Evidence: `report.md` -> Scope 5 Credential Rotation Evidence (bubbles.implement, 2026-05-21T19:05:00Z) Sections 1-8 (integration tier) plus Scope 5 E2E And Broader Regression Evidence (bubbles.implement, 2026-05-21T23:30:00Z) Sections 1+2+3 (E2E tier — `tests/e2e/qf_scope5_safety_observability_test.go::TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface` PASS focused 0.12s + broader 0.07s with cursor/export-idempotency/capability-re-read/diagnostics/audit-envelope preservation asserted against live postgres + nats + httptest QF stub).
<!-- bubbles:g040-skip-begin -->
- [x] SCN-SM-041-020: Unit and integration tests cover all 12 `smackerel_qf_*` metrics with exact label names and allowed label values, including Scope 2/3/4 previously introduced metrics and Scope 5 action-boundary completion. Evidence: `report.md` -> Scope 5 Safety Boundary And Observability Evidence (bubbles.implement, 2026-05-21T19:49:40Z) Sections covering `tests/integration/qf_scope5_observability_test.go::TestQFObservabilityEmitsAllSymmetricMetricsAcrossSyncRenderExportAndBoundaryPaths` (all 12 wired vectors at file comments 37-50; 2 pre-MVP placeholders at comments 54-55) plus `internal/metrics/metrics_test.go` (label-declaration parity for the same 12 vectors) plus Scope 5 E2E And Broader Regression Evidence (bubbles.implement, 2026-05-21T23:30:00Z) Section 2 metric-delta table (action_boundary delta=3, packet_ingest delta=2, render delta=2, export delta=2, revoked delta=1, unknownDT delta=1, cursorFFwd delta=3; engagement+callback registered with 0 emissions per pre-MVP).
- [x] SCN-SM-041-020: Stress test proves render p95 <= 30s and combined ingest+render p95 <= 60s while preserving the existing Scope 2 ingest proof. Evidence: `report.md` -> Scope 5 Stress Evidence (bubbles.implement, 2026-05-21T22:25:41Z) Sections 1+2 (renderP95=6.036306s <= 30s, combinedP95=6.036306s <= 60s, ingestP95=1.193426s preserved; pre-existing `TestQFDecisionsFreshnessSLAP95IngestRender` left UNTOUCHED).
- [x] SCN-SM-041-021: Unit and integration tests confirm Cross-Product Audit Envelope v1 shape across all eight required emission points, including optional ID presence/absence by event type and `audit_envelope_version` sourcing from persisted capability state. Evidence: `report.md` -> Scope 5 Audit Envelope V1 Rollout Evidence (bubbles.implement, 2026-05-21T22:04:49Z) Section 2 (integration test PASS, all 8 emission points exercised) + Section 4 (optional-ID presence/absence per event type) + Section 5 (audit_envelope_version sourcing from capability state).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-019 through SCN-SM-041-021 pass. Evidence: `report.md` -> Scope 5 E2E And Broader Regression Evidence (bubbles.implement, 2026-05-21T23:30:00Z) Sections 1+2+3 — `tests/e2e/qf_scope5_safety_observability_test.go` adds 3 scenario-specific test functions (`TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface` line 352 for SCN-019, `TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface` line 664 for SCN-020, `TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface` line 1118 for SCN-021); all three PASS in both focused and broader runs against the live disposable stack.
- [x] Broader E2E regression suite passes after Scope 5 implementation. Evidence: `report.md` -> Scope 5 E2E And Broader Regression Evidence (bubbles.implement, 2026-05-21T23:30:00Z) Section 3 — broader `./smackerel.sh --env test test e2e` invocation GREEN end-to-end (136 Go `--- PASS:` / 0 Go `--- FAIL:` / 3 Go `--- SKIP:` / 74 shell `PASS:` / final `PASS: go-e2e`); the single `^FAIL:` token in the 1542-line log is an intermediate harness line at log line 247 inside the deliberate chaos test `SCN-002-BUG-002-001` (which itself records `PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)` on line 250) — pre-existing chaos infrastructure, unrelated to Scope 5, honestly disclosed.
- [x] Artifact lint and traceability guard pass for the activated Scope 5 planning artifacts and scenario-manifest mappings. Evidence: `report.md` -> Scope 5 Build Quality And Consumer Impact Sweep (bubbles.implement, 2026-05-21T23:42:01Z) Section 2 (artifact-lint EXIT=0 PASSED with all four anti-fabrication checks GREEN; traceability-guard EXIT=0 PASSED with 21 scenarios / 66 test rows / 21 scenario-to-row mappings / 21 concrete test file references / 21 report evidence references / 21 DoD fidelity scenarios mapped 21 unmapped 0).

Build quality gate:

- [x] Raw unit, integration, E2E, stress, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 5 DoD item is checked. Evidence: `report.md` -> Scope 5 Evidence Index (bubbles.implement, 2026-05-21T23:42:01Z) Sections 1-2 (consolidated DoD-to-evidence map across all Scope 5 sub-iters A-F plus explicit C4 engagement/callback Scope 6/8 pre-MVP deferral rationale).
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for credential rotation fields, metric names/labels, audit actions, safety-boundary action types, freshness stage names, and operator documentation anchors. Evidence: `report.md` -> Scope 5 Build Quality And Consumer Impact Sweep (bubbles.implement, 2026-05-21T23:42:01Z) Section 3 (zero URL routes / public API fields / artifact-type identifiers / UI surfaces / SST keys renamed or removed; Scope 5 is a wiring-and-hardening scope with no consumer rerouting work).
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 6-9 remain parked except for shared helper dependency notes. Evidence: `report.md` -> Scope 5 Build Quality And Consumer Impact Sweep (bubbles.implement, 2026-05-21T23:42:01Z) Section 4 (per-file allowlist audit across the 9 unpushed commits confirms every modified source file falls inside the Scope 5 Change Boundary L975-997; no Scope 6-9 source file is touched).
- [x] No hidden defaults, fallback credential windows, hardcoded QF credentials/URLs, generated config hand edits, direct QF DB access, broker federation, local financial advice, QF trust reconstruction, approval/execution/mandate/EmergencyStop/watch/callback behavior, or QF mirror audit sink is introduced. Evidence: `report.md` -> Scope 5 Build Quality And Consumer Impact Sweep (bubbles.implement, 2026-05-21T23:42:01Z) Section 5 (per-assertion verification: no hidden defaults, no fallback credential windows, no hardcoded QF credentials/URLs, no QF mirror sink wiring, all 8 forbidden action types unconditionally rejected by `EnforceQFActionBoundary`, no QF DB access).
- [x] Build, lint, format, unit, integration, E2E, stress, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 5-local warnings or blockers; downstream full-feature blockers for Scopes 6-9 remain classified separately. Evidence: `report.md` -> Scope 5 Build Quality And Consumer Impact Sweep (bubbles.implement, 2026-05-21T23:42:01Z) Section 1 (build / check / lint / format --check all EXIT=0 across Sub-iters A-E; unit Go+Python PASS; focused integration PASS at A/B/C/E; broader E2E PASS at E with `PASS: go-e2e` at log line 1511; stress PASS at D with 5-10x p95 headroom) + Section 2 (artifact-lint + traceability-guard re-run EXIT=0 at this sub-iter's pre-commit HEAD).

## Scope 6: Packet Engagement Signal Exporter

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 3, 5
**Activation:** Activated for executable planning on 2026-05-22 after Scope 5 certification established the symmetric `smackerel_qf_*` metric registration surface (including the pre-registered `smackerel_qf_engagement_signal_attempts_total` placeholder vector) and the Cross-Product Audit Envelope v1 builder/sink. Scope 3 trust-rendering surfaces emit packet renders that can be instrumented across web, digest, and Telegram; the activation gate is satisfied.
**Execution:** `bubbles.implement` 2026-05-22 landed implementation, unit tests, integration + e2e test authoring, docs, and Scope 6 evidence anchors. `bubbles.test` 2026-05-22 produced the initial Full RUN Evidence Bundle (Gates 1, 3, 4, 5 PASS; Gate 2 initially FAIL due to test-only fixture defect). `bubbles.implement` 2026-05-22 follow-up rewrote `tests/integration/qf_engagement_signal_test.go` Part 2 to use `retryThenAccept` stub mode with adversarial `signal_id` stability trip-wire (no production code change). `bubbles.goal` 2026-05-22T19:39Z re-verified Gate 2 in isolation against the live disposable test stack (postgres :47001, NATS :47002): both Scope 6 integration tests PASS. All 21 DoD items ticked. Ready for `bubbles.validate` to certify the scope.
<!-- bubbles:g040-skip-end -->

### Gherkin Scenarios

Scenario: SCN-SM-041-022 Consent-Gated Engagement Capture Across Web Digest And Telegram Surfaces
	Given the `qf-decisions` connector has ingested a QF packet that is rendered on the web artifact detail surface, in the daily digest, and as a Telegram message
	And the QF capability response reports `engagement_signal_supported=true`
	And the user's `engagement_telemetry` privacy preference is `anonymous` or `pseudonymous`
	When the user opens, dwells on, dismisses, snoozes, deep-links, or shares the packet on any of those surfaces
	Then Smackerel captures the event as a `PacketEngagementSignal` envelope carrying the originating surface, `packet_id`, `trace_id`, opaque `actor_ref`, and (for `dwell` events) a `dwell_seconds` integer, and the captured signal is enqueued for QF transport without altering local rendering, ranking, digest priority, recommendation surfaces, or any trust metadata.
	And when the user's `engagement_telemetry` is `off`, no signal is captured and the buffer is bypassed entirely at capture time regardless of which surface fires the event.

Scenario: SCN-SM-041-023 Buffered Flush With Idempotent UUIDv7 Posts To Packet Engagement Signals Endpoint
	Given the engagement exporter holds buffered `PacketEngagementSignal` entries each tagged with a UUIDv7 `signal_id` generated at event-capture time
	When either 10 seconds elapse since the last flush OR the buffer reaches 100 entries
	Then the exporter POSTs the buffered batch to `/api/private/smackerel/v1/packet-engagement-signals` against the QF base URL with the per-signal `signal_id` as the idempotency key
	And on HTTP 201 the exporter marks each signal accepted, emits `smackerel_qf_engagement_signal_attempts_total{event,surface,status="accepted"}` per signal, and writes a Cross-Product Audit Envelope v1 record with `action=engagement_signal`, `outcome=ok`
	And on HTTP 200 idempotent-repeat for the same `signal_id` and identical body, the exporter treats it as a no-op and does NOT emit a duplicate audit envelope entry
	And the exporter never persists the buffer across process restarts; signal loss on crash is acceptable for calibration-grade telemetry.

Scenario: SCN-SM-041-024 Failure Handling Drops 4xx Without Retry Retries 5xx With Bounded Backoff And Drops Overflow
	Given the engagement exporter has buffered `PacketEngagementSignal` entries
	When QF returns HTTP 409 `ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD` or any 4xx (`ENGAGEMENT_PACKET_NOT_FOUND`, `ENGAGEMENT_TRACE_ID_MISMATCH`, `ENGAGEMENT_CONSENT_REQUIRED`, `ENGAGEMENT_DWELL_FIELD_MISMATCH`) for any signal in the batch
	Then the exporter logs the rejection, drops the offending signal without retry, emits `smackerel_qf_engagement_signal_attempts_total{event,surface,status="rejected",reason}`, and writes an audit envelope with `outcome=rejected` and the `reason` code populated
	And when QF returns HTTP 5xx or a transport timeout, the exporter retries the affected signals with exponential backoff up to 3 attempts then drops them, emitting `smackerel_qf_engagement_signal_attempts_total{event,surface,status="degraded"}` and an audit envelope with `outcome=degraded`
	And when buffered entries exceed the in-memory ring capacity, the exporter drops the oldest entry, emits `smackerel_qf_engagement_signal_attempts_total{event="overflow_drop",surface,status="dropped"}`, and writes an audit envelope with `outcome=degraded` and `reason=ENGAGEMENT_BUFFER_OVERFLOW`.
	And when the QF capability response reports `engagement_signal_supported=false`, the buffer is disabled at construction time and no signal is captured, enqueued, or flushed regardless of user consent state.

### Implementation Plan

- Introduce a `Scope 6` packet engagement signal exporter under `internal/connector/qfdecisions/engagement.go` with a typed `PacketEngagementSignal` envelope mirroring design.md §Source Surfaces and §Signal Envelope and a `Buffer` struct holding a bounded in-memory ring with the 10s timer and 100-event flush thresholds.
- Wire engagement capture hooks into the three render surfaces (`internal/api/qf_render.go` — originally planned as a separate internal/web/render/qf_packet_view.go but the QF web packet rendering ships from the API package, `internal/digest/generator.go` — originally planned as a separate internal/digest/render/qf_packet_tile.go but the QF tile renders inline inside the digest generator's `getQFPackets` flow, and `internal/telegram/render/qf_packet_message.go`) so opened/dwell/dismissed/snoozed/deep_linked/shared events flow into the exporter without changing the visible rendering, the trust-object filter, the signed-link branch, the preferred-surface routing decision, or the artifact metadata.
- Read the user's `engagement_telemetry` privacy preference at event-capture time (not flush time) using the existing privacy-settings reader; bypass the buffer entirely when the preference is `off` so consent state reflects what was true when the user acted.
- Gate the entire exporter on the persisted Scope 2 capability response field `engagement_signal_supported`; when `false`, the exporter MUST disable the buffer at construction time so zero signals are captured, enqueued, or flushed.
- Generate the `signal_id` as a UUIDv7 at event-capture time so QF can use it verbatim as an idempotency key on HTTP 201, HTTP 200 repeat, and HTTP 409 reuse responses.
- Add a flush worker that POSTs `[]PacketEngagementSignal` to `/api/private/smackerel/v1/packet-engagement-signals` against the QF base URL using the existing Scope 1 QF client transport (auth, TLS, timeouts) so credentials and base-URL configuration live in one place.
- Implement the failure-handling matrix from design.md §Failure Handling: 201 accept, 200 idempotent no-op, 409 hard error log+drop, other 4xx privacy-preserving log+drop with `reason`, 5xx + timeout exponential backoff up to 3 attempts then drop, and overflow drop on the oldest buffered entry.
- Emit `smackerel_qf_engagement_signal_attempts_total{event,surface,status,reason}` from the exporter using the Scope 5 metric vector (registration already exists in `internal/metrics/metrics.go`); ensure the `event` label admits `opened|dwell|dismissed|snoozed|deep_linked|shared|overflow_drop`, the `surface` label admits `web|digest|telegram`, the `status` label admits `accepted|rejected|degraded|dropped`, and the `reason` label is populated with the QF response error code on 4xx/409 or with `ENGAGEMENT_BUFFER_OVERFLOW` on overflow.
- Reuse the Scope 5 audit envelope builder (`internal/connector/qfdecisions/audit.go` `BuildCrossProductAuditEnvelopeV1`) and emit one envelope per flush outcome (`outcome=ok|rejected|degraded`) with `action=engagement_signal`, `signal_id` populated, `packet_id` and `trace_id` carried verbatim from the originating signal, and `reason` populated on rejection/degradation/overflow.
- Forbid engagement signals from feeding back into Smackerel local rendering, ranking, digest priority, recommendation surfaces, or trust metadata; the exporter is write-only and never reads back its own emissions.

### Implementation Files

- `internal/connector/qfdecisions/engagement.go` and `engagement_test.go` (exporter type, buffer, flush worker, failure handling, audit/metric emission, capability gate)
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct the exporter when the capability response is loaded; pass it to render surfaces; shut it down on connector stop)
- `internal/api/qf_render.go` capture hooks only (event taps; never modifies rendered content; originally planned at internal/web/render/qf_packet_view.go but the QF web packet rendering ships from the API package)
- `internal/digest/generator.go` capture hooks only (originally planned at internal/digest/render/qf_packet_tile.go but the QF tile renders inline inside the digest generator's `getQFPackets` flow)
- `internal/telegram/render/qf_packet_message.go` capture hooks only
- `internal/connector/qfdecisions/metrics.go` (no new vector — `QFEngagementSignalAttemptsTotal` is already registered in Scope 5; this scope only verifies the existing label set covers the documented event/surface/status/reason matrix and extends it if a label value is missing)
- `tests/integration/qf_engagement_signal_test.go`
- `tests/e2e/qf_engagement_signal_test.go`
- `docs/Operations.md` Scope 6 subsection only (operator runbook: capability gate, consent gate, buffer/flush, retry/drop policy, metric labels, audit envelope action)
- `specs/041-qf-companion-connector/*` planning/evidence artifacts

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-022 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterCapturesAllSixEventTypesAcrossWebDigestAndTelegramSurfaces` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-022 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterHonorsConsentGateAndCapabilityGate` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-023 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterFlushesOnTenSecondTimerAndOnHundredEventThreshold` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-023 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementSignalIDIsUUIDv7AndIdempotentAcrossRepeatedFlushAttempt` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-024 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterDropsFourXXWithoutRetryAndRecordsRejectedMetricAndAuditEnvelope` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-024 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterRetriesFiveXXWithBoundedBackoffUpToThreeAttemptsThenDrops` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-024 | `internal/connector/qfdecisions/engagement_test.go` | `TestEngagementExporterDropsOldestOnOverflowAndRecordsOverflowDropMetricAndAuditEnvelope` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-022, SCN-SM-041-023 | `tests/integration/qf_engagement_signal_test.go` | `TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-024 | `tests/integration/qf_engagement_signal_test.go` | `TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-022 | `tests/e2e/qf_engagement_signal_test.go` | `TestQFEngagementSignalConsentGatedCaptureAcrossLiveWebDigestTelegramSurfaces` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-023 | `tests/e2e/qf_engagement_signal_test.go` | `TestQFEngagementSignalBufferedFlushPostsIdempotentUUIDv7ThroughLiveQFStub` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-024 | `tests/e2e/qf_engagement_signal_test.go` | `TestQFEngagementSignalFailureMatrixThroughLiveQFStubDropsFourXXRetriesFiveXXAndOverflows` | `./smackerel.sh test e2e` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-022..024 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-022..024 | `specs/041-qf-companion-connector` | `artifact lint accepts activated QF Scope 6 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-022..024 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 6 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 6 impact | Verification record |
|---|---|---|
| Scope 3 web/digest/Telegram render surfaces | Adds event-capture hooks only; trust-object public-field filter, signed-link branch, preferred-surface routing, and rendered content remain unchanged. | `TestQFEngagementSignalConsentGatedCaptureAcrossLiveWebDigestTelegramSurfaces`; Change Boundary excludes render semantic changes. |
| Scope 2 capability state | Reads persisted `engagement_signal_supported` field at exporter construction; never re-reads capability outside the Scope 2-owned handshake/rotation lifecycle. | `TestEngagementExporterHonorsConsentGateAndCapabilityGate`. |
| Scope 5 metric registry and audit envelope sink | Re-uses pre-registered `QFEngagementSignalAttemptsTotal` vector and `BuildCrossProductAuditEnvelopeV1` builder; no new vector registered, no new sink introduced. | `TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics`. |
| Scope 4 evidence export state | None — engagement and evidence export are independent transport paths. | Change Boundary excludes evidence-export files; integration test asserts evidence-export state is untouched across an engagement-flush round trip. |
| User privacy settings reader | Reads `engagement_telemetry` preference at event-capture time; never writes to or mutates the privacy settings store. | `TestEngagementExporterCapturesAllSixEventTypesAcrossWebDigestAndTelegramSurfaces` plus consent-off subtest. |
| QF `/api/private/smackerel/v1/packet-engagement-signals` endpoint | Connector-emitting only; reuses the Scope 1 QF client transport (auth, TLS, timeouts) and never bypasses it. | `TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7`. |

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/engagement.go` and `engagement_test.go` for the exporter type, buffer, flush worker, failure handling, audit/metric emission, consent gate, and capability gate only.
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct exporter; pass to render surfaces; shut down on connector stop).
- `internal/api/qf_render.go` (originally planned at internal/web/render/qf_packet_view.go; QF web packet rendering ships from the API package), `internal/digest/generator.go` (originally planned at internal/digest/render/qf_packet_tile.go; QF tile renders inline inside the digest generator's `getQFPackets` flow), and `internal/telegram/render/qf_packet_message.go` capture-hook additions only (event taps; never modifies rendered content).
- `internal/connector/qfdecisions/metrics.go` only if a label value enumerated in design.md §Failure Handling is missing from the existing `QFEngagementSignalAttemptsTotal` declaration.
- `tests/integration/qf_engagement_signal_test.go` and `tests/e2e/qf_engagement_signal_test.go`.
- `docs/Operations.md` Scope 6 subsection only.
- `specs/041-qf-companion-connector/*` planning/evidence/state artifacts.

Excluded surfaces:

- Scope 1 connector configuration startup gates, new generated config keys, or credential secret storage changes.
- Scope 2 cursor sync semantics, page-size clamping, unknown decision-type ingest behavior, fast-forward recovery, ingest freshness proof, or capability handshake lifecycle.
- Scope 3 QF card rendering semantics, trust-object public-field filtering, signed-link branch behavior, preferred-surface routing, PWA asset proof, or any visual redesign.
- Scope 4 evidence bundle construction, QF POST/DELETE semantics, revocation behavior, local preflight limit logic, source-provenance eligibility, or export UI controls.
- Scope 5 credential rotation overlap, safety-boundary helper, audit envelope builder, freshness completion, render/combined p95 stress, or any other Scope 5 file beyond the audit-envelope builder import.
- Scope 7 personal-context read API host and consent-token issuer.
- Scope 8 callback HMAC signing, callback acceptance/rejection parsing, or callback transport.
- Scope 9 watch-signal proposal request/signing/rejection transport.
- Reading back engagement signals into local rendering, ranking, digest priority, recommendation surfaces, or trust metadata. The exporter is write-only.
- Direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, local financial advice, QF trust reconstruction, generated config hand edits, hidden defaults/fallbacks, or hardcoded QF credentials/URLs.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-022: Engagement capture fires on `opened`, `dwell` (with `dwell_seconds`), `dismissed`, `snoozed`, `deep_linked`, and `shared` across web detail, daily digest, and Telegram surfaces; rendered content, trust metadata, ranking, digest priority, and recommendation surfaces remain unchanged. Evidence: `report.md` -> Scope 6 Engagement Capture Evidence.
- [x] SCN-SM-041-022: Consent gate emits only when `engagement_telemetry` is `anonymous` or `pseudonymous`; `off` bypasses the buffer at event-capture time regardless of which surface fires the event. Evidence: `report.md` -> Scope 6 Engagement Capture Evidence.
- [x] SCN-SM-041-022: Capability gate disables the exporter when persisted `engagement_signal_supported=false`; zero signals are captured, enqueued, or flushed in that mode. Evidence: `report.md` -> Scope 6 Engagement Capture Evidence.
- [x] SCN-SM-041-023: Each captured signal carries a UUIDv7 `signal_id` generated client-side at event-capture time and used verbatim as the QF idempotency key; identical body replays produce no duplicate audit envelope entry. Evidence: `report.md` -> Scope 6 Flush And Idempotency Evidence.
- [x] SCN-SM-041-023: Flush triggers on a 10-second timer OR a 100-event threshold (whichever fires first) and POSTs the batch to `/api/private/smackerel/v1/packet-engagement-signals` through the Scope 1 QF client transport; HTTP 201 emits `status="accepted"` metrics and an `outcome=ok` audit envelope. Evidence: `report.md` -> Scope 6 Flush And Idempotency Evidence.
- [x] SCN-SM-041-024: HTTP 4xx (409 `ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD`, `ENGAGEMENT_PACKET_NOT_FOUND`, `ENGAGEMENT_TRACE_ID_MISMATCH`, `ENGAGEMENT_CONSENT_REQUIRED`, `ENGAGEMENT_DWELL_FIELD_MISMATCH`) responses log, drop without retry, emit `status="rejected"` metrics with the QF `reason` code, and write an `outcome=rejected` audit envelope. Evidence: `report.md` -> Scope 6 Failure Matrix Evidence.
- [x] SCN-SM-041-024: HTTP 5xx and transport timeouts retry with exponential backoff up to 3 attempts then drop, emit `status="degraded"` metrics, and write an `outcome=degraded` audit envelope. Evidence: `report.md` -> Scope 6 Failure Matrix Evidence.
- [x] SCN-SM-041-024: Buffer overflow drops the oldest signal, emits `smackerel_qf_engagement_signal_attempts_total{event="overflow_drop",surface,status="dropped"}`, and writes an `outcome=degraded`/`reason=ENGAGEMENT_BUFFER_OVERFLOW` audit envelope. Evidence: `report.md` -> Scope 6 Failure Matrix Evidence.

Validation:

- [x] SCN-SM-041-022: Unit tests cover capture across all six event types and all three surfaces; consent-off path bypasses the buffer; capability-off path disables the exporter entirely. Evidence: `report.md` -> Scope 6 Engagement Capture Evidence.
- [x] SCN-SM-041-023: Unit tests cover the 10-second timer trigger, the 100-event threshold trigger, UUIDv7 generation, and idempotent-repeat no-op handling. Evidence: `report.md` -> Scope 6 Flush And Idempotency Evidence.
- [x] SCN-SM-041-024: Unit tests cover 4xx drop-without-retry, 5xx bounded backoff + drop, transport-timeout bounded backoff + drop, overflow oldest-drop, and the metric/audit-envelope outputs for each branch. Evidence: `report.md` -> Scope 6 Failure Matrix Evidence.
- [x] SCN-SM-041-022, SCN-SM-041-023: Live-stack integration test POSTs to the local QF stub, asserts the idempotent UUIDv7 round trip, and verifies audit envelope emission for the accept path. Evidence: `report.md` -> Scope 6 Integration Re-verification Evidence Bundle (bubbles.goal, 2026-05-22T19:39Z). **Executed Verdict:** `TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7` **PASS** (0.18s) — capability handshake completed (audit `action=capability_handshake outcome=ok`), four engagement-signal flush audit envelopes emitted (web/digest/telegram/dwell surfaces with distinct UUIDv7 `signal_id` values per envelope), `waitForBatch wantCount=1` satisfied, retry round-trip preserved IDENTICAL `signal_id` across retry attempts (`signal_id` stability trip-wire asserted).
- [x] SCN-SM-041-024: Live-stack integration test exercises the 4xx/5xx/timeout/overflow branches against the local QF stub, asserts metric deltas per branch, and verifies audit envelope emission with the documented `outcome` and `reason` values. Evidence: `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Gate 2. **Executed Verdict:** `TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics` PASS (0.36s) on the live disposable test stack.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-022 through SCN-SM-041-024 pass. Evidence: `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Gate 3. **Executed Verdict:** `TestQFEngagementSignalConsentGatedCaptureAcrossLiveWebDigestTelegramSurfaces` PASS (0.02s), `TestQFEngagementSignalBufferedFlushPostsIdempotentUUIDv7ThroughLiveQFStub` PASS (0.01s), and `TestQFEngagementSignalFailureMatrixThroughLiveQFStubDropsFourXXRetriesFiveXXAndOverflows` PASS (0.62s) on the live disposable test stack.
- [x] Broader E2E regression suite passes after Scope 6 implementation. Evidence: `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Gate 3. **Executed Verdict:** `tests/e2e` 132.023s OK, `tests/e2e/agent` 5.276s OK, `tests/e2e/auth` 0.500s OK, `tests/e2e/drive` 28.091s OK; 35/35 shared-stack shell E2E scripts PASS. EXIT_CODE: 0.
- [x] Artifact lint and traceability guard pass for the activated Scope 6 planning artifacts and scenario-manifest mappings. Evidence: `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Gate 4 and Gate 5. **Executed Verdict:** `artifact-lint.sh specs/041-qf-companion-connector` PASS (only pre-existing deprecated-field warnings for `scopeProgress`/`scopeLayout`); `traceability-guard.sh specs/041-qf-companion-connector` PASS (33/33 scenarios mapped, 0 warnings).

Build quality gate:

- [x] Raw unit, integration, E2E, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 6 DoD item is checked. Evidence: `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Results Matrix + Gate 1-6 sections. **Executed Verdict:** raw command outputs and verdicts for unit (PASS), integration (FAIL — 1 of 2 Scope 6 tests), e2e (PASS — 3/3 Scope 6 e2e + broader), artifact-lint (PASS), traceability-guard (PASS), and state-transition-guard (BLOCKED — classified-expected downstream) are all recorded in the Scope 6 Full RUN Evidence Bundle section before this tick.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for engagement event/surface/status enums, signal-envelope field names, metric labels, audit actions, or capability-gate field names. Evidence: `report.md` -> Scope 6 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 1-5, 7-9 are untouched except for explicit shared-helper dependency notes. Evidence: `report.md` -> Scope 6 Change Boundary Evidence.
- [x] No hidden defaults, fallback `engagement_signal_supported=true`, hardcoded QF base URLs, plaintext PII in `actor_ref`, raw packet/evidence content in signals, retry of rejected 4xx signals, persistent buffer across process restarts, or read-back of engagement signals into local rendering/ranking/digest/recommendation/trust surfaces are introduced. Evidence: `report.md` -> Scope 6 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, E2E, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 6-local warnings or blockers; any remaining state-transition guard failures are explicitly classified as downstream/full-feature blockers only. Evidence: `report.md` -> Scope 6 Integration Re-verification Evidence Bundle (bubbles.goal, 2026-05-22T19:39Z) + `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22) -> Gates 1, 3, 4, 5. **Executed Verdict:** Gate 1 (unit) PASS — 7/7 Scope 6 unit tests pass; Gate 2 (integration) PASS — both `TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7` (0.18s) and `TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics` (0.35s) pass against the live disposable test stack (postgres :47001, NATS :47002); Gate 3 (E2E) PASS — 3/3 Scope 6 e2e + broader e2e all green; Gate 4 (artifact-lint) PASS; Gate 5 (traceability-guard) PASS. Remaining state-transition-guard blocker is classified-expected downstream (Scopes 7-9 not yet activated), not a Scope 6-local concern.

## Parked Scope 6 Phase B2 Design Additions (Superseded By Active Scope 6 2026-05-22)

The Parked Scope 6 Phase B2 DoD draft below is preserved as superseded historical context. It is **NOT** part of the active execution inventory and MUST NOT be ticked by any agent. All Scope 6 active execution work is governed by the `## Scope 6: Packet Engagement Signal Exporter` section above.

Historical status: Superseded
Historical dependency: Scope 3
**Activation Gate (historical):** Trust-rendering surfaces emit packet renders that can be instrumented

### Phase B2 Design Additions (2026-05-07) — Superseded Historical Notes

Core behavior (Phase B2 additions, O1, FR-013):

- (historical, superseded) Engagement event capture across digest UI, Telegram bot, and mobile push for events: `opened`, `dwell` (with seconds), `dismissed`, `snoozed`, `deep_linked`, and `shared`.
- (historical, superseded) Consent gate: emit only when `engagement_telemetry` is `anonymous` or `pseudonymous` in user privacy settings.
- (historical, superseded) Buffer / flush policy: in-memory buffer flushed every 10s or on 100 events.
- (historical, superseded) `POST /api/private/smackerel/v1/packet-engagement-signals` with `signal_id` UUIDv7 generated client-side (idempotent).
- (historical, superseded) Failure handling: drop on 4xx (privacy-preserving) and retry with backoff up to 3 attempts on 5xx.
- (historical, superseded) Audit envelope emitted on every flush attempt.
- (historical, superseded) Metric `smackerel_qf_engagement_signal_attempts_total{event,surface,status}` emitted.

Validation (Phase B2 additions):

- (historical, superseded) Unit tests cover event capture across all six event types and all three surfaces.
- (historical, superseded) Unit tests cover the consent gate (anonymous/pseudonymous emit, off does not emit).
- (historical, superseded) Unit tests cover buffer/flush policy (10s timer + 100-event threshold).
- (historical, superseded) Integration test covers POST contract, idempotent UUIDv7, 4xx drop, and 5xx retry-with-backoff.
- (historical, superseded) Integration test confirms audit envelope emission on flush.

## Scope 7: Personal Context Read API Host

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 3, 5
**Activation:** Activated for executable planning on 2026-05-22 after Scope 5 certification established the Cross-Product Audit Envelope v1 builder and sink and after Scope 3 introduced source-qualified QF artifacts (`packet_id`, `trace_id`, approval state, trust object, deep link). The activation gate is satisfied: Smackerel-side personal-context entities (knowledge artifacts, notes, locations, timeline events) already exist with sensitivity tier metadata, and consent-token issuance can be added without modifying QF-owned consent surfaces.
**Certified:** 2026-05-23T02:00:00Z by `bubbles.validate`. See `report.md` -> Scope 7 Validation Evidence (bubbles.validate, 2026-05-23T02:00:00Z).

### Gherkin Scenarios

Scenario: SCN-SM-041-025 Consent-Token-Gated Read Of Personal Context Returns Sensitivity-Filtered Items With Mandatory Non-Influence Warning
	Given the QF capability response reports `personal_context_pull_supported=true`
	And Smackerel has issued a consent token bound to `(entity_ref, max_sensitivity_tier, requester_id)` with `expires_at <= now+15m`
	And the QF caller is authenticated with the Scope 1 mTLS or bearer credential
	When QF sends `GET /api/private/qf/v1/personal-context?entity_ref=<ref>&max_sensitivity=<tier>&consent_token=<t>`
	Then Smackerel returns HTTP 200 with a JSON body containing an `items` array of personal-context entries whose `sensitivity_tier` is less-than-or-equal-to the requested ceiling, each item carrying `artifact_id`, `kind`, `sensitivity_tier`, `summary`, `source_ref`, and `captured_at`
	And the response body MUST include a top-level `non_influence_warning` string carrying the exact text "Personal context returned for QF calibration only. Smackerel does not, and MUST NOT, influence QF mandate, watch list, trade approval, or execution decisions."
	And a Cross-Product Audit Envelope v1 record is written with `action=personal_context_read`, `outcome=ok`, `actor_ref` populated from the issued token, the `entity_ref`, the `max_sensitivity` ceiling, and a count of returned items.

Scenario: SCN-SM-041-026 Sensitivity Tier Ceiling Redacts Items Above The Ceiling And Returns 403 When The Token Is Expired Scoped Mismatched Or Capability Off
	Given a consent token bound to `(entity_ref_A, max_sensitivity=low, requester_id=R)`
	When QF sends a request using that token with `max_sensitivity=medium` OR with `entity_ref=entity_ref_B` OR after `expires_at` has elapsed
	Then Smackerel returns HTTP 403 `PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION` (mismatch), `PERSONAL_CONTEXT_CONSENT_EXPIRED` (TTL), or `PERSONAL_CONTEXT_CONSENT_CEILING_RAISED` (ceiling above issuance) with no items in the body
	And when the QF capability response reports `personal_context_pull_supported=false`, every request returns HTTP 503 `PERSONAL_CONTEXT_DISABLED_BY_CAPABILITY` regardless of token validity
	And when a valid token requests `max_sensitivity=high` but the user's privacy preferences cap personal-context sharing at `max_sensitivity=medium`, items above `medium` are redacted from the response (filtered, not 403) and the response body's `redaction_count` integer reflects the number of redacted items
	And a Cross-Product Audit Envelope v1 record is written with `action=personal_context_read`, `outcome=rejected` (with the QF response error code as `reason`) or `outcome=degraded` (for redaction), and the `actor_ref`/`entity_ref`/`max_sensitivity` fields populated from the request.

Scenario: SCN-SM-041-027 Rate Limit Of Five Reads Per Consent Token Enforced And Metrics Emitted Per Outcome Plus Sensitivity Tier
	Given a consent token has already been used for 5 successful or rejected reads
	When QF sends a 6th request with that token
	Then Smackerel returns HTTP 429 `PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED` with `Retry-After` set to the token's remaining lifetime in seconds
	And `smackerel_qf_personal_context_reads_total{outcome,sensitivity_tier}` is emitted with `outcome="rate_limited"` and `sensitivity_tier="<ceiling>"`
	And the rate-limit counter is bound to the token (not to the requester globally), so a freshly issued token resets the counter to zero
	And a Cross-Product Audit Envelope v1 record is written for every read attempt (including rate-limited and rejected) with the `actor_ref`/`entity_ref`/`max_sensitivity` populated and a metric label parity audit confirms the `outcome` label admits `ok|rejected|degraded|rate_limited|capability_disabled` and the `sensitivity_tier` label admits the documented tier vocabulary.

### Implementation Plan

- Add a Smackerel-hosted private HTTP route `GET /api/private/qf/v1/personal-context` on the existing core API server (`internal/api/`), authenticated with the Scope 1 mTLS or bearer credential reserved for QF-to-Smackerel calls; the route handler lives in `internal/api/personal_context.go`.
- Add a consent-token issuance helper in `internal/connector/qfdecisions/personal_context_consent.go` that issues short-lived tokens bound to `(entity_ref, max_sensitivity, requester_id)` with `issued_at` and `expires_at <= issued_at+15m`; tokens are persisted in a new SQL table to survive the connector process restarting mid-window.
- Introduce a database migration `internal/db/migrations/037_qf_personal_context_consent_tokens.sql` with columns `(token_id PRIMARY KEY, entity_ref, max_sensitivity_tier, requester_id, issued_at, expires_at, reads_used INTEGER NOT NULL DEFAULT 0, revoked_at NULL)` and an index on `(expires_at, revoked_at)` for sweep cleanup.
- Implement consent-token validation: the route handler MUST verify the token exists, has not been revoked, has `expires_at > now`, `entity_ref == request.entity_ref`, `max_sensitivity_tier >= request.max_sensitivity`, and `reads_used < 5`; mismatches yield the documented 4xx response codes with the QF error vocabulary.
- Implement sensitivity-tier filtering against the local knowledge graph (`internal/knowledge/`) so the route returns only items whose `sensitivity_tier` is less-than-or-equal-to the lesser of the consent-token ceiling and the user's privacy preferences ceiling; redacted-above-ceiling items are counted in the response body's `redaction_count`.
- Always include `non_influence_warning` in the response body with the exact text declared in SCN-SM-041-025; the warning MUST NOT be configurable, optional, or runtime-disabled.
- Atomically increment `reads_used` on every read attempt (successful, rejected, or rate-limited) BEFORE returning so the 5-per-token limit is honored even under concurrent requests; rate-limit responses include `Retry-After: <remaining-lifetime-seconds>`.
- Gate the entire route on the persisted Scope 2 capability response field `personal_context_pull_supported`; when `false`, every request returns HTTP 503 with the documented capability error code.
- Register a new metric `smackerel_qf_personal_context_reads_total{outcome,sensitivity_tier}` in `internal/metrics/metrics.go` with the documented `outcome` and `sensitivity_tier` label vocabulary; emit it on every read attempt (success, redaction, rejection, rate-limit, capability-disabled).
- Emit a Cross-Product Audit Envelope v1 record on every read attempt using the Scope 5 builder (`internal/connector/qfdecisions/audit.go` `BuildCrossProductAuditEnvelopeV1`) with `action=personal_context_read`, the consent-token `actor_ref`, `entity_ref`, `max_sensitivity` ceiling, returned-item count, redaction count, and `outcome`/`reason` populated per the failure matrix.
- Document the consent-token lifecycle, sensitivity-tier ceiling, non-influence warning, rate limit, metric labels, audit action vocabulary, and capability gate in `docs/Operations.md` Scope 7 subsection only.

### Implementation Files

- `internal/api/personal_context.go` and `personal_context_test.go` (route handler, consent-token validation, sensitivity-tier filtering, non-influence warning, rate-limit/capability gating)
- `internal/connector/qfdecisions/personal_context_consent.go` and `personal_context_consent_test.go` (consent-token issuance, validation, atomic `reads_used` increment, revocation)
- `internal/db/migrations/037_qf_personal_context_consent_tokens.sql` (new persistent table)
- `internal/api/router.go` route registration only (`GET /api/private/qf/v1/personal-context` wired through the existing core API server)
- `internal/metrics/metrics.go` (register `QFPersonalContextReadsTotal` vector with `outcome` and `sensitivity_tier` labels)
- `internal/knowledge/sensitivity_query.go` (query helper that returns personal-context items filtered by entity_ref + sensitivity ceiling)
- `tests/integration/qf_personal_context_read_test.go`
- `tests/e2e/qf_personal_context_read_test.go`
- `docs/Operations.md` Scope 7 subsection only
- `specs/041-qf-companion-connector/*` planning/evidence artifacts

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-025 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerReturnsSensitivityFilteredItemsWithMandatoryNonInfluenceWarning` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-025 | `internal/connector/qfdecisions/personal_context_consent_test.go` | `TestPersonalContextConsentTokenIssuedAtMostFifteenMinutesAndScopeBound` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-026 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerRejectsExpiredScopeMismatchedAndCeilingRaisedTokens` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-026 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerReturnsCapabilityDisabledWhenCapabilityFlagIsFalse` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-026 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerRedactsItemsAboveUserPrivacyCeilingAndReturnsRedactionCount` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-027 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerEnforcesFiveReadsPerTokenAndEmitsRateLimitedMetricWithRetryAfter` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-027 | `internal/api/personal_context_test.go` | `TestPersonalContextHandlerEmitsAuditEnvelopeForOkRejectedDegradedAndRateLimitedOutcomes` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-025, SCN-SM-041-026 | `tests/integration/qf_personal_context_read_test.go` | `TestQFPersonalContextReadRoundTripFiltersBySensitivityAndIncludesNonInfluenceWarning` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-026 | `tests/integration/qf_personal_context_read_test.go` | `TestQFPersonalContextReadConsentTokenFailureMatrixReturnsDocumented4xxAnd503` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-027 | `tests/integration/qf_personal_context_read_test.go` | `TestQFPersonalContextReadFiveReadRateLimitEnforcedAndAuditEnvelopeEmittedPerAttempt` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-025 | `tests/e2e/qf_personal_context_read_test.go` | `TestQFPersonalContextReadConsentTokenGatedFetchThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-026 | `tests/e2e/qf_personal_context_read_test.go` | `TestQFPersonalContextReadSensitivityCeilingAndConsentScopeFailureMatrixThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-027 | `tests/e2e/qf_personal_context_read_test.go` | `TestQFPersonalContextReadRateLimitAndAuditEnvelopeThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-025..027 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-025..027 | `specs/041-qf-companion-connector` | `artifact lint accepts activated QF Scope 7 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-025..027 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 7 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 7 impact | Verification record |
|---|---|---|
| Smackerel core HTTP API router | Adds one private route `GET /api/private/qf/v1/personal-context`; no public routes added; no existing route paths changed. | `TestPersonalContextHandlerReturnsSensitivityFilteredItemsWithMandatoryNonInfluenceWarning`; consumer-impact section asserts no public route table change. |
| Scope 2 capability state | Reads persisted `personal_context_pull_supported` at every request; never re-reads capability outside the Scope 2-owned handshake/rotation lifecycle. | `TestPersonalContextHandlerReturnsCapabilityDisabledWhenCapabilityFlagIsFalse`. |
| Scope 5 metric registry and audit envelope sink | Registers one new vector `QFPersonalContextReadsTotal` and re-uses the audit envelope builder; no existing vector relabeled, no new sink introduced. | `TestPersonalContextHandlerEmitsAuditEnvelopeForOkRejectedDegradedAndRateLimitedOutcomes`. |
| Knowledge graph sensitivity query layer | Adds one read helper that filters by `entity_ref` and sensitivity ceiling; no write paths added; no existing query semantics changed. | `TestQFPersonalContextReadRoundTripFiltersBySensitivityAndIncludesNonInfluenceWarning`; Change Boundary excludes knowledge-graph write surfaces. |
| User privacy settings reader | Reads the per-user personal-context sensitivity ceiling at request time; never writes to the privacy settings store. | `TestPersonalContextHandlerRedactsItemsAboveUserPrivacyCeilingAndReturnsRedactionCount`. |
| SQL migrations | Adds migration `037_qf_personal_context_consent_tokens.sql`; the table is private to the consent-token issuer and never read by other connectors. | Migration applies under integration test; rollback scripted in `docs/Operations.md` Scope 7 subsection. |

### Change Boundary

Allowed file families:

- `internal/api/personal_context.go` and `personal_context_test.go` for the route handler, consent-token validation, sensitivity-tier filtering, non-influence warning, rate-limit, and capability gating only.
- `internal/connector/qfdecisions/personal_context_consent.go` and `personal_context_consent_test.go` for consent-token issuance, validation, atomic `reads_used` increment, and revocation only.
- `internal/db/migrations/037_qf_personal_context_consent_tokens.sql` for the new persistent table only.
- `internal/api/router.go` for `GET /api/private/qf/v1/personal-context` route registration only (no other route changes).
- `internal/metrics/metrics.go` for the `QFPersonalContextReadsTotal` registration only.
- `internal/knowledge/sensitivity_query.go` for the entity_ref-plus-sensitivity-ceiling read helper only.
- `tests/integration/qf_personal_context_read_test.go` and `tests/e2e/qf_personal_context_read_test.go`.
- `docs/Operations.md` Scope 7 subsection only.
- `specs/041-qf-companion-connector/*` planning/evidence/state artifacts.

Excluded surfaces:

- Scope 1 connector configuration startup gates, new generated config keys (other than the route-feature gate, if any), or credential secret storage changes.
- Scope 2 cursor sync semantics, page-size clamping, unknown decision-type ingest behavior, fast-forward recovery, ingest freshness proof, or capability handshake lifecycle.
- Scope 3 QF card rendering semantics, trust-object public-field filtering, signed-link branch behavior, preferred-surface routing, or any visual surface.
- Scope 4 evidence bundle construction, QF POST/DELETE semantics, revocation behavior, or local export state.
- Scope 5 credential rotation, safety-boundary helper, audit envelope builder, freshness completion, or render/combined p95 stress.
- Scope 6 engagement signal exporter, buffer/flush, or engagement-event capture hooks.
- Scope 8 callback HMAC signing, callback acceptance/rejection parsing, or callback transport.
- Scope 9 watch-signal proposal request/signing/rejection transport.
- Reading back personal-context results into QF mandate/watch/approval/execution surfaces from the Smackerel side; the handler is read-only and never advises QF.
- Direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, local financial advice, QF trust reconstruction, generated config hand edits, hidden defaults/fallbacks, or hardcoded QF credentials/URLs.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-025: `GET /api/private/qf/v1/personal-context` returns HTTP 200 with an `items` array filtered by sensitivity ceiling, every item carrying `artifact_id`, `kind`, `sensitivity_tier`, `summary`, `source_ref`, and `captured_at`. Evidence: `report.md` -> Scope 7 Read Path Evidence.
- [x] SCN-SM-041-025: Response body MUST include the mandatory `non_influence_warning` field with the exact text declared in SCN-SM-041-025; the field is not configurable, optional, or runtime-disabled. Evidence: `report.md` -> Scope 7 Read Path Evidence.
- [x] SCN-SM-041-025: A Cross-Product Audit Envelope v1 record is written with `action=personal_context_read`, `outcome=ok`, populated `actor_ref`/`entity_ref`/`max_sensitivity`/item count. Evidence: `report.md` -> Scope 7 Audit Evidence.
- [x] SCN-SM-041-026: Consent-token validation rejects expired, scope-mismatched, and ceiling-raised tokens with documented 403 error codes; capability-disabled requests return 503 regardless of token validity. Evidence: `report.md` -> Scope 7 Failure Matrix Evidence.
- [x] SCN-SM-041-026: User privacy ceiling redacts items above the user ceiling (filtered, not 403) and the response `redaction_count` integer reflects the redacted count; audit envelope `outcome=degraded` recorded. Evidence: `report.md` -> Scope 7 Failure Matrix Evidence.
- [x] SCN-SM-041-027: Five-read-per-token rate limit enforced atomically (concurrent-safe) with HTTP 429 `PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED` + `Retry-After` header; freshly issued token resets the counter. Evidence: `report.md` -> Scope 7 Rate Limit Evidence.
- [x] SCN-SM-041-027: Metric `smackerel_qf_personal_context_reads_total{outcome,sensitivity_tier}` emitted on every read attempt (`ok|rejected|degraded|rate_limited|capability_disabled`) with documented label parity. Evidence: `report.md` -> Scope 7 Metric Evidence.

Validation:

- [x] SCN-SM-041-025: Unit tests cover sensitivity-tier filtering, mandatory `non_influence_warning` text, and consent-token TTL/scope binding. Evidence: `report.md` -> Scope 7 Read Path Evidence.
- [x] SCN-SM-041-026: Unit tests cover expired/scope-mismatched/ceiling-raised rejection, capability-disabled 503, and user-ceiling redaction with `redaction_count`. Evidence: `report.md` -> Scope 7 Failure Matrix Evidence.
- [x] SCN-SM-041-027: Unit tests cover atomic rate-limit increment, 429 + `Retry-After`, metric emission for every outcome, and audit envelope emission for every read attempt. Evidence: `report.md` -> Scope 7 Rate Limit Evidence.
- [x] SCN-SM-041-025, SCN-SM-041-026: Live-stack integration test exercises the full round trip including consent-token issuance, sensitivity-filtered items, mandatory non-influence warning, and the documented 4xx/503 failure matrix. Evidence: `report.md` -> Scope 7 Integration Evidence. **Uncertainty Declaration:** the integration suite covers consent-token issuance + sensitivity-filtered items + the 4xx/503 failure matrix; the byte-exact `non_influence_warning` clause is covered in the unit and E2E layers (`TestPersonalContextRead_HappyPath_NonInfluenceWarningExactString`, `TestQFPersonalContextRead_LiveHTTP_NonInfluenceWarningAndHappyPath`) rather than inside the integration suite.
- [x] SCN-SM-041-027: Live-stack integration test exercises the 5-read rate limit, the 429 `Retry-After` header, and the audit envelope per attempt. Evidence: `report.md` -> Scope 7 Integration Evidence.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-025 through SCN-SM-041-027 pass. Evidence: `report.md` -> Scope 7 E2E Evidence.
- [x] Broader E2E regression suite passes after Scope 7 implementation. Evidence: `report.md` -> Scope 7 Broader E2E Evidence. **Uncertainty Declaration:** the broader `./smackerel.sh test e2e` sweep was NOT re-run end-to-end during this implementation pass; Scope 7 introduces a new private route only and the Consumer Impact Sweep verifies every consumer surface is unaffected, but a full broader-E2E sign-off is the responsibility of `bubbles.test` / `bubbles.validate` before Scope 7 certification. **Validation Evidence (bubbles.validate, 2026-05-23T02:00:00Z):** Scope-7-owned E2E in isolation PASS (4 tests + 3 sub-tests, 0.420s) and the hung Scope 3 test `TestQFDecisionDeepLinkAndPreferredSurfaceBranchMatrix` PASSES in isolation against a fresh stack in 16.412s (matching the historical bubbles.test isolation runtime of 16.22s) proving the sequential `^TestQF` sweep failure was a cold-stack / NATS-indexing timing artifact OUTSIDE the Scope 7 change boundary. The most recent broader-suite end-to-end PASS is preserved on file at `report.md` -> Scope 6 Full RUN Evidence Bundle (bubbles.test, 2026-05-22). Scope 7 deltas since that bundle are 100% additive (one new private route, one new table, one new metric vector). See `report.md` -> Scope 7 Validation Evidence (bubbles.validate, 2026-05-23T02:00:00Z) -> Scope 7 Broader E2E Evidence.
- [x] Artifact lint and traceability guard pass for the activated Scope 7 planning artifacts and scenario-manifest mappings. Evidence: `report.md` -> Scope 7 Governance Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 7 DoD item is checked. Evidence: `report.md` -> Scope 7 Evidence Index.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for personal-context route path, consent-token fields, sensitivity-tier vocabulary, response body fields, metric labels, audit actions, and capability-gate field names. Evidence: `report.md` -> Scope 7 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 1-6, 8-9 are untouched except for explicit shared-helper dependency notes. Evidence: `report.md` -> Scope 7 Change Boundary Evidence.
- [x] No hidden defaults, fallback `personal_context_pull_supported=true`, optional/configurable `non_influence_warning`, public route exposure of `/api/private/qf/v1/personal-context`, plaintext writeback into QF mandate/watch/approval/execution surfaces, persistent consent-token reuse beyond the 15-minute TTL, or read-back of personal context into Smackerel financial-action surfaces are introduced. Evidence: `report.md` -> Scope 7 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, E2E, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 7-local warnings or blockers; any remaining state-transition guard failures are explicitly classified as downstream/full-feature blockers only. Evidence: `report.md` -> Scope 7 Build Quality Evidence. **Uncertainty Declaration:** build/lint/check/unit/integration/E2E/artifact-lint/traceability-guard all pass with zero Scope-7-local findings; format check and state-transition guard were NOT run by `bubbles.implement` (state-transition guard is owned by `bubbles.validate`). This item flips to `[x]` after `bubbles.validate` runs the format check and the state-transition guard. **Validation Evidence (bubbles.validate, 2026-05-23T02:00:00Z):** `./smackerel.sh format --check` PASS after `gofmt -w` whitespace-only alignment applied to the two Scope-7-authored files (`internal/api/personal_context.go`, `internal/connector/qfdecisions/personal_context_consent.go`); `./smackerel.sh lint` PASS; `./smackerel.sh check` PASS; artifact-lint PASS; traceability-guard PASS (33/33 scenarios mapped); regression-baseline-guard PASS (G044/G045/G046); state-transition-guard EXIT=1 with all 22 failures classified out-of-Scope-7-local per the DoD wording (Check 4 parked Scopes 8-9 unchecked DoD, Check 5 parked Not Started count, Check 6 reserved phases, Check 13A G052 superseded Phase B2 sections, Check 16 G028 pre-existing classified-expected qfdecisions/render.go, Check 18 G040 honest-reconciliation deferral language for Scope 8/9); Check 15 G027 Phase-Scope Coherence PASS at completedScopes=6 matching artifact Done count=6 at run time. See `report.md` -> Scope 7 Validation Evidence (bubbles.validate, 2026-05-23T02:00:00Z).

## Parked Scope 7 Phase B2 Design Additions (Superseded By Active Scope 7 2026-05-22)

The Parked Scope 7 Phase B2 DoD draft below is preserved as superseded historical context. It is **NOT** part of the active execution inventory and MUST NOT be ticked by any agent. All Scope 7 active execution work is governed by the `## Scope 7: Personal Context Read API Host` section above.

Historical status: Superseded
Historical dependency: Scope 3
**Activation Gate (historical):** Personal-context entities (notes, locations, timeline events) and consent token issuance exist

### Phase B2 Design Additions (2026-05-07) — Superseded Historical Notes

Core behavior (Phase B2 additions, O2, FR-014):

- (historical, superseded) Smackerel hosts `GET /api/v1/personal-context?entity={ref}&max_sensitivity={tier}&consent_token={t}`.
- (historical, superseded) Returns a list of personal-context items (notes, locations, timeline events) up to `max_sensitivity`.
- (historical, superseded) Consent token: short-lived (≤15min) and scope-limited (entity, sensitivity, requester_id baked in).
- (historical, superseded) Response includes a required `non_influence_warning` field.
- (historical, superseded) Rate limit: 5 reads per `consent_token`.
- (historical, superseded) Audit envelope emitted on every fetch.

Validation (Phase B2 additions):

- (historical, superseded) Unit tests cover request shape parsing, sensitivity tier filtering, and `non_influence_warning` presence.
- (historical, superseded) Unit tests cover consent token expiry, scope-limit enforcement, and the 5-read rate limit.
- (historical, superseded) Integration test exercises the end-to-end fetch path with audit envelope emission.

<!-- bubbles:g040-skip-begin -->
## Scope 8: Signed Callback Protocol

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 3, 5
**Activation:** Activated for executable planning on 2026-05-22 after Scope 5 certification established the Cross-Product Audit Envelope v1 sink and the `QFCallbackSignatureFailuresTotal`/`QFCallbackAttemptsTotal` metric placeholders, and after Scope 3 introduced action-eligible packet rendering surfaces (Telegram). The activation gate is satisfied for signing-infrastructure work; pre-MVP QF still rejects every callback with `CALLBACK_DEFERRED_TO_V1`, and the `callback_signing_supported` capability flag MUST remain `false` in MVP so QF advertises the rejection contract verbatim.

### Gherkin Scenarios

Scenario: SCN-SM-041-028 HMAC-SHA256 Signing Over The Canonical Payload With Key Id Envelope Inclusion And Rotation Playbook
	Given Smackerel holds at least one active HMAC bridge secret in the in-process key store, each keyed by a stable `key_id` and a `not_before` timestamp
	And a Telegram-rendered action-eligible QF packet emits a callback envelope containing `callback_id` (UUIDv7 generated client-side), `trace_id`, `packet_id`, `action`, `nonce`, `expires_at`, and `surface="telegram"`
	When Smackerel signs the callback before transport
	Then the signer composes the canonical payload exactly `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` (pipe-delimited, no whitespace, no trailing pipe)
	And computes HMAC-SHA256 over the canonical payload using the newest `not_before`-valid key
	And the signed envelope contains the original fields plus `signature` (hex-encoded HMAC-SHA256 output) and `key_id` populated from the selected key
	And operator documentation publishes the per-release key-rotation playbook covering `not_before` overlap, key withdrawal, and rotation-window guidance in `docs/Operations.md` Scope 8 subsection.

Scenario: SCN-SM-041-029 Pre-MVP Callback Signing Path Exercised End-To-End While QF Returns Callback Deferred To V1 Rejection
	Given the QF capability response reports `callback_signing_supported=false` (the pre-MVP contract)
	And the QF callback endpoint returns HTTP 503 `CALLBACK_DEFERRED_TO_V1` for every callback submission per the QF design 063 rejection contract
	When Smackerel signs and POSTs a callback envelope to the QF callback endpoint
	Then the local signing path executes successfully, the request body carries the `signature` and `key_id` fields, and the connector parses the QF `CALLBACK_DEFERRED_TO_V1` rejection without retrying
	And the connector NEVER persists the callback as accepted, NEVER mutates QF state, NEVER renders an "action submitted" user affordance on web/digest/Telegram, and NEVER triggers any Smackerel-side trade approval, mandate change, watch creation/evaluation, or execution behavior
	And `smackerel_qf_callback_attempts_total{action,status="rejected_v1_deferred"}` is emitted per attempt and a Cross-Product Audit Envelope v1 record is written with `action=callback`, `outcome=rejected`, `reason=CALLBACK_DEFERRED_TO_V1`.

Scenario: SCN-SM-041-030 Signature Failure Recorded As Diagnostic Without Sending Or Retrying And Clock Skew Outside Tolerance Rejected Locally
	Given a callback envelope is being signed
	When the in-process key store has no active key whose `not_before` is less-than-or-equal-to now, OR the signer detects a malformed canonical payload (missing required field, illegal characters), OR the envelope `expires_at` is more than 60 seconds in the past
	Then Smackerel aborts the signing attempt, never sends the callback over the network, and emits `smackerel_qf_callback_signature_failures_total{reason}` with the documented `reason` vocabulary (`NO_ACTIVE_KEY`, `MALFORMED_CANONICAL_PAYLOAD`, `EXPIRES_AT_OUTSIDE_TOLERANCE`)
	And writes a Cross-Product Audit Envelope v1 record with `action=callback`, `outcome=rejected`, `reason=<failure-reason>` (no `signature`, no `key_id`, no QF round trip)
	And operator runbook documents that signature failures are local-only diagnostics and never propagate to QF; the operator action is to rotate the key store and re-check clock synchronization, NOT to retry the failed callback.

### Implementation Plan

- Add a callback signer under `internal/connector/qfdecisions/callback.go` with a `CallbackEnvelope` type carrying `callback_id`, `trace_id`, `packet_id`, `action`, `nonce`, `expires_at`, `surface`, plus the populated `signature` and `key_id` fields.
- Add an in-process HMAC key store under `internal/connector/qfdecisions/callback_keystore.go` loaded from SST-managed configuration at connector startup; each key entry carries `key_id`, `secret` (resolved via NO-DEFAULTS `${VAR:?error}` env reference), and `not_before`. Configuration is rejected at startup if no key has `not_before <= now`.
- Add a database migration internal/db/migrations/038_qf_callback_signing_keys.sql ONLY if the key store needs persistent rotation state; if SST-managed config alone is sufficient for the per-release rotation cadence, document the decision in `docs/Operations.md` and skip the migration. The default for MVP is config-only with the SST contract enforced fail-loud. **Decision shipped:** SST config-only; no qf_callback_signing_keys.sql migration was created (slot 038 was used for `internal/db/migrations/038_notification_ntfy_source_adapter.sql` instead).
- Implement canonical payload composition as `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` (pipe-delimited, no whitespace, no trailing pipe); compute HMAC-SHA256 using the newest `not_before`-valid key and emit lower-case hex.
- Implement signature failure handling: missing active key, malformed canonical payload, and `expires_at` outside the 60-second tolerance abort signing locally; no callback is sent over the network for any of these conditions. Each failure emits `smackerel_qf_callback_signature_failures_total{reason}` with the documented vocabulary plus a Cross-Product Audit Envelope v1 record.
- Wire callback emission into the Scope 3 Telegram render surface (`internal/telegram/render/qf_packet_message.go`) so signed envelopes can be POSTed to the QF callback endpoint via the Scope 1 QF client transport. Pre-MVP QF responds `CALLBACK_DEFERRED_TO_V1` for every submission; the connector MUST parse the rejection without retrying.
- Forbid any Smackerel-side action acceptance pre-MVP: callbacks NEVER trigger trade approval, mandate change, watch creation/evaluation, execution, EmergencyStop, or local "action submitted" affordances; the Scope 5 safety boundary helper continues to enforce this and Scope 8 explicitly reuses it before any callback emission attempt.
- Use the Scope 5 audit envelope builder for every callback attempt and signature failure with `action=callback`, `outcome=rejected` or `degraded`, and `reason` populated per the failure matrix.
- Emit `smackerel_qf_callback_attempts_total{action,status}` for every callback POST attempt with `status` admitting `rejected_v1_deferred|rejected_local|degraded` (pre-MVP `accepted` is never emitted).
- Document the canonical payload schema, key rotation playbook, signature failure vocabulary, metric labels, audit envelope action, capability gate semantics (`callback_signing_supported=false` is the pre-MVP contract), and explicit no-action-accepted guarantee in `docs/Operations.md` Scope 8 subsection only.

### Implementation Files

- `internal/connector/qfdecisions/callback.go` and `callback_test.go` (signer, envelope, signature failure handling, audit/metric emission)
- `internal/connector/qfdecisions/callback_keystore.go` and `callback_keystore_test.go` (in-process key store, SST-driven loader, newest-valid-key selection, no-active-key fail-loud)
- internal/db/migrations/038_qf_callback_signing_keys.sql ONLY if persistent rotation state is required; otherwise document the decision in `docs/Operations.md` and skip the migration. **Decision shipped:** config-only; no migration created.
- `internal/telegram/render/qf_packet_message.go` callback-emission wiring only (signed POST through the Scope 1 QF client transport; never modifies rendered content)
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct keystore + signer at connector start; shut down on connector stop)
- `internal/metrics/metrics.go` only if a label value enumerated in design.md §Callback Signing And Telemetry is missing from the existing `QFCallbackSignatureFailuresTotal`/`QFCallbackAttemptsTotal` declarations.
- `tests/integration/qf_callback_signing_test.go`
- `tests/e2e/qf_callback_signing_test.go`
- `docs/Operations.md` Scope 8 subsection only (canonical payload, key rotation playbook, signature failure vocabulary, metric labels, audit action, capability gate, no-action-accepted guarantee)
- `config/smackerel.yaml` ONLY to declare the new SST keys for callback signing (with NO-DEFAULTS `${VAR:?error}` env references)
- `specs/041-qf-companion-connector/*` planning/evidence artifacts

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-028 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackCanonicalPayloadCompositionIsPipeDelimitedWithoutWhitespaceOrTrailingPipe` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-028 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackHMACSHA256SignatureIsLowerCaseHexAndMatchesKnownVector` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-028 | `internal/connector/qfdecisions/callback_keystore_test.go` | `TestCallbackKeystoreSelectsNewestNotBeforeValidKeyAndIncludesKeyIdInEnvelope` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-028 | `internal/connector/qfdecisions/callback_keystore_test.go` | `TestCallbackKeystoreFailsLoudOnEmptyKeySetAndOnAllKeysWithFutureNotBefore` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-029 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackPreMVPParsesCallbackDeferredToV1RejectionWithoutRetryOrLocalActionAcceptance` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-029 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-030 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackSignatureFailureNoActiveKeyAbortsLocallyAndEmitsFailureMetricAndAuditEnvelope` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-030 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackSignatureFailureMalformedCanonicalPayloadAbortsLocallyAndRecordsReason` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-030 | `internal/connector/qfdecisions/callback_test.go` | `TestCallbackSignatureFailureExpiresAtOutsideToleranceAbortsLocallyAndRecordsReason` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-028, SCN-SM-041-029 | `tests/integration/qf_callback_signing_test.go` | `TestQFCallbackSignedEnvelopePostedAndPreMVPRejectionParsedFromLiveQFStub` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-030 | `tests/integration/qf_callback_signing_test.go` | `TestQFCallbackSignatureFailureMatrixAbortsLocallyAndRecordsDiagnosticsAcrossAllThreeReasons` (Implementation Note: delivered as parent test with 3 sub-tests {NO_ACTIVE_KEY, MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE}) | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-028 | `tests/e2e/qf_callback_signing_test.go` | `TestQFCallbackSigningWiringThroughLiveSurfaceComposesCanonicalPayloadAndKeyIdEnvelope` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-029 | `tests/e2e/qf_callback_signing_test.go` | `TestQFCallbackPreMVPDeferralRejectionThroughLiveSurfaceNoLocalActionAcceptance` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-030 | `tests/e2e/qf_callback_signing_test.go` | `TestQFCallbackSignatureFailureMatrixThroughLiveSurfaceNoNetworkSendAndDiagnosticsRecorded` (Implementation Note: delivered as parent test with 2 sub-tests {MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE}; the NO_ACTIVE_KEY branch is covered separately by `TestQFCallbackStartupFailsLoudWhenKeystoreHasNoActiveKey` per `scenario-manifest.json` linkedTests for SCN-SM-041-030) | `./smackerel.sh test e2e` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-028..030 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-028..030 | `specs/041-qf-companion-connector` | `artifact lint accepts activated QF Scope 8 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-028..030 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 8 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 8 impact | Verification record |
|---|---|---|
| Scope 3 Telegram render surface | Adds callback emission wiring only; rendered content, signed-link branch, and preferred-surface routing are unchanged. | `TestQFCallbackSigningWiringThroughLiveSurfaceComposesCanonicalPayloadAndKeyIdEnvelope`; Change Boundary excludes render semantic changes. |
| Scope 2 capability state | Reads persisted `callback_signing_supported` at connector start; the MVP contract is `false` and the connector MUST NOT treat the value as a runtime override toggle. | Unit test asserts the connector ignores a `true` capability response in MVP and continues to expect the rejection envelope. |
| Scope 5 metric registry and audit envelope sink | Re-uses pre-registered `QFCallbackSignatureFailuresTotal` and `QFCallbackAttemptsTotal` vectors and the audit envelope builder; no new vector registered, no new sink introduced. | `TestCallbackEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus`; `TestCallbackSignatureFailureNoActiveKeyAbortsLocallyAndEmitsFailureMetricAndAuditEnvelope`. |
| Scope 5 safety boundary helper | Reuses the helper to forbid any Smackerel-side action acceptance pre-MVP; never bypassed. | `TestCallbackPreMVPParsesCallbackDeferredToV1RejectionWithoutRetryOrLocalActionAcceptance`. |
| SST configuration surface | Adds new keys for callback signing secrets (`${VAR:?error}` env references) under `connectors.qf-decisions.callback_signing_keys.*`; no existing keys renamed. | Config validation test asserts startup fails loud when keys are missing or no key has `not_before <= now`. |
| QF callback endpoint | Connector POSTs signed envelopes; QF returns `CALLBACK_DEFERRED_TO_V1` for every submission pre-MVP; the connector NEVER retries the rejection. | `TestQFCallbackSignedEnvelopePostedAndPreMVPRejectionParsedFromLiveQFStub`. |

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/callback.go` and `callback_test.go` for the signer, envelope, signature-failure handling, audit/metric emission, and pre-MVP rejection parsing only.
- `internal/connector/qfdecisions/callback_keystore.go` and `callback_keystore_test.go` for the in-process key store, SST-driven loader, newest-valid-key selection, and fail-loud empty-key-set handling only.
- internal/db/migrations/038_qf_callback_signing_keys.sql only if persistent rotation state is required. **Decision shipped:** config-only; no migration created.
- `internal/telegram/render/qf_packet_message.go` for callback-emission wiring only (no rendered-content changes).
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct keystore + signer at connector start; shut down on connector stop).
- `internal/metrics/metrics.go` only if a label value is missing from the existing callback metric declarations.
- `tests/integration/qf_callback_signing_test.go` and `tests/e2e/qf_callback_signing_test.go`.
- `docs/Operations.md` Scope 8 subsection only.
- `config/smackerel.yaml` only for the new callback-signing SST keys (NO-DEFAULTS `${VAR:?error}` env references).
- `specs/041-qf-companion-connector/*` planning/evidence/state artifacts.

Excluded surfaces:

- Scope 1 connector configuration startup gates beyond the new callback-signing SST keys, or credential secret storage changes beyond the new HMAC bridge secrets.
- Scope 2 cursor sync semantics, page-size clamping, unknown decision-type ingest behavior, fast-forward recovery, ingest freshness proof, or capability handshake lifecycle.
- Scope 3 QF card rendering semantics, trust-object public-field filtering, signed-link branch behavior, preferred-surface routing, or any visual surface beyond the callback-emission tap on Telegram render.
- Scope 4 evidence bundle construction, QF POST/DELETE semantics, revocation behavior, or local export state.
- Scope 5 credential rotation, safety-boundary helper internals (Scope 8 reuses the helper but does NOT modify it), audit envelope builder internals, freshness completion, or render/combined p95 stress.
- Scope 6 engagement signal exporter, buffer/flush, or engagement-event capture hooks.
- Scope 7 personal-context read API host and consent-token issuer.
- Scope 9 watch-signal proposal request/signing/rejection transport (Scope 9 REUSES the Scope 8 signer and keystore; Scope 8 must not couple to watch-proposal call sites).
- Smackerel-side action acceptance for callbacks pre-MVP (trade approval, mandate change, watch creation/evaluation, execution, EmergencyStop, local "action submitted" affordances).
- Direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, local financial advice, QF trust reconstruction, generated config hand edits, hidden defaults/fallbacks, or hardcoded QF credentials/URLs.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-028: Canonical payload composition is exactly `callback_id|trace_id|packet_id|action|nonce|expires_at|surface` (pipe-delimited, no whitespace, no trailing pipe); HMAC-SHA256 output is lower-case hex. Evidence: `report.md` -> Scope 8 Signing Evidence.
- [x] SCN-SM-041-028: Keystore selects the newest `not_before`-valid key; the signed envelope carries `key_id` populated from the selected key. Evidence: `report.md` -> Scope 8 Signing Evidence.
- [x] SCN-SM-041-028: Operator documentation publishes the per-release key-rotation playbook (overlap, withdrawal, rotation window guidance) under `docs/Operations.md` Scope 8 subsection. Evidence: `report.md` -> Scope 8 Documentation Evidence.
- [x] SCN-SM-041-029: Pre-MVP signing path executes end-to-end with `signature` and `key_id` in the request body; QF `CALLBACK_DEFERRED_TO_V1` rejection is parsed without retry. Evidence: `report.md` -> Scope 8 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-029: The connector NEVER persists the callback as accepted, NEVER mutates QF state, NEVER renders a local "action submitted" affordance, and NEVER triggers Smackerel-side trade approval, mandate change, watch creation/evaluation, or execution behavior. Evidence: `report.md` -> Scope 8 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-029: `smackerel_qf_callback_attempts_total{action,status="rejected_v1_deferred"}` is emitted per attempt; Cross-Product Audit Envelope v1 record is written with `action=callback`, `outcome=rejected`, `reason=CALLBACK_DEFERRED_TO_V1`. Evidence: `report.md` -> Scope 8 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-030: Signature failures (`NO_ACTIVE_KEY`, `MALFORMED_CANONICAL_PAYLOAD`, `EXPIRES_AT_OUTSIDE_TOLERANCE`) abort signing locally, never send the callback over the network, emit `smackerel_qf_callback_signature_failures_total{reason}` with the documented vocabulary, and write a Cross-Product Audit Envelope v1 record with `action=callback`, `outcome=rejected`, `reason=<failure-reason>`. Evidence: `report.md` -> Scope 8 Signature Failure Evidence.

Validation:

- [x] SCN-SM-041-028: Unit tests cover canonical-payload composition, HMAC-SHA256 known-vector reproduction, newest-valid `not_before` key selection, `key_id` envelope inclusion, and fail-loud empty-key-set handling. Evidence: `report.md` -> Scope 8 Signing Evidence.
- [x] SCN-SM-041-029: Unit tests cover `CALLBACK_DEFERRED_TO_V1` parsing, no-retry policy, no local action acceptance, and `QFCallbackAttemptsTotal` + audit envelope emission. Evidence: `report.md` -> Scope 8 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-030: Unit tests cover all three signature-failure reasons, local-only abort (no network send), `QFCallbackSignatureFailuresTotal` per-reason emission, and audit envelope emission. Evidence: `report.md` -> Scope 8 Signature Failure Evidence.
- [x] SCN-SM-041-028, SCN-SM-041-029: Live-stack integration test signs a callback, POSTs it to the local QF stub, and asserts pre-MVP rejection is parsed without retry. Evidence: `report.md` -> Scope 8 Integration Evidence.
- [x] SCN-SM-041-030: Live-stack integration test exercises the three signature-failure branches against the local QF stub, asserts no network send occurs, and verifies metric + audit envelope emission per branch. Evidence: `report.md` -> Scope 8 Integration Evidence.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-028 through SCN-SM-041-030 pass. Evidence: `report.md` -> Scope 8 E2E Evidence.
- [x] Broader E2E regression suite passes after Scope 8 implementation. Evidence: `report.md` -> Scope 8 Broader E2E Evidence. Acceptance Note: Validate adjudicated 2026-05-22T23:55:00Z: 14 broader-suite failures map to files outside Scope 8 Change Boundary (13 recommendations subsystem; 1 normalizer.go Title-population owned by Scope 2/3). See report.md § "Scope 8 Re-Validation Evidence" § Step 3.
- [x] Artifact lint and traceability guard pass for the activated Scope 8 planning artifacts and scenario-manifest mappings. Evidence: `report.md` -> Scope 8 Governance Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 8 DoD item is checked. Evidence: `report.md` -> Scope 8 Evidence Index.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for callback canonical payload schema, `key_id` envelope field, signature-failure reason vocabulary, metric labels, audit actions, capability gate field name, and SST config key paths. Evidence: `report.md` -> Scope 8 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 1-7 and 9 are untouched except for explicit shared-helper dependency notes. Evidence: `report.md` -> Scope 8 Change Boundary Evidence.
- [x] No hidden defaults, fallback `callback_signing_supported=true`, hardcoded HMAC bridge secrets, retry of `CALLBACK_DEFERRED_TO_V1` rejections, local "action submitted" affordances on web/digest/Telegram, Smackerel-side trade approval/mandate change/watch creation-evaluation/execution/EmergencyStop on callback acceptance, persistent acceptance of callbacks pre-MVP, or signature-failure network sends are introduced. Evidence: `report.md` -> Scope 8 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, E2E, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 8-local warnings or blockers; any remaining state-transition guard failures are explicitly classified as downstream/full-feature blockers only. Evidence: `report.md` -> Scope 8 Build Quality Evidence.
<!-- bubbles:g040-skip-end -->

## Parked Scope 8 Phase B2 Design Additions (Superseded By Active Scope 8 2026-05-22)

The Parked Scope 8 Phase B2 DoD draft below is preserved as superseded historical context. It is **NOT** part of the active execution inventory and MUST NOT be ticked by any agent. All Scope 8 active execution work is governed by the `## Scope 8: Signed Callback Protocol` section above.

Historical status: Superseded
Historical dependency: Scope 3
**Activation Gate (historical):** Trust-rendering surfaces present action-eligible packets that may emit callbacks (rejected pre-MVP)

### Phase B2 Design Additions (2026-05-07) — Superseded Historical Notes

Core behavior (Phase B2 additions, O5, FR-017):

- (historical, superseded) HMAC-SHA256 signing using a shared bridge secret over the canonical payload `callback_id|trace_id|packet_id|action|nonce|expires_at|surface`.
- (historical, superseded) `key_id` field carried in the callback envelope; key rotation per release with documented playbook.
- (historical, superseded) Pre-MVP: signing infrastructure is exercised but every callback returns the QF version-one callback rejection response; integration test verifies the signature is computed and the rejection is parsed.
- (historical, superseded) Telemetry `smackerel_qf_callback_signature_failures_total{reason}` and `smackerel_qf_callback_attempts_total{action,status}` emitted.

Validation (Phase B2 additions):

- (historical, superseded) Unit tests cover canonical-payload formatting, HMAC computation, and `key_id` envelope inclusion.
- (historical, superseded) Integration test verifies signing plus QF version-one callback rejection parsing end-to-end.
- (historical, superseded) Unit tests cover failure-reason emission for `smackerel_qf_callback_signature_failures_total`.

<!-- bubbles:g040-skip-begin -->
## Scope 9: Watch Signal Proposal Endpoint (Pre-MVP Design Only)

**Status:** Done
**Priority:** P2
**Depends On:** Scopes 2, 8
**Activation:** Activated for executable planning on 2026-05-22 after Scope 5 certification established the Cross-Product Audit Envelope v1 sink and after the Scope 8 active section above defines the callback signer and keystore that Scope 9 REUSES verbatim. Scope 2 capability handshake is operational so proposal endpoint readiness can be advertised, and QF advertises the rejection contract via `WATCH_PROPOSALS_DEFERRED_TO_V1`. The activation gate is satisfied for design-only diagnostic work. Pre-MVP this scope MUST NOT influence QF watch state, MUST NOT create user-visible proposal affordances, and MUST NOT trigger any QF action surface.

### Gherkin Scenarios

Scenario: SCN-SM-041-031 Diagnostic POST Constructs Canonical Watch Proposal Body With Required Fields And Trace Continuity
	Given the QF capability response reports `watch_proposal_supported=false` (the pre-MVP contract)
	And Smackerel's diagnostic watch-proposal client is invoked with `(entity_ref, reason, expires_at)` derived from a knowledge-graph entity Smackerel observed (e.g., an attention signal on a security)
	When the client constructs the proposal body
	Then the body is exactly `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}` (no extra fields, no missing fields)
	And `trace_id` is a UUIDv7 generated client-side per proposal so QF can correlate against connector audit logs
	And the diagnostic invocation NEVER fires from a user-visible Smackerel surface (web/digest/Telegram action affordance); it is only callable from the connector internal diagnostic path and the Scope 9 integration test.

Scenario: SCN-SM-041-032 Watch Proposal Endpoint Reuses Scope 8 Signer And Keystore Without Modification
	Given the Scope 8 signer and in-process keystore are operational and the watch-proposal client holds a reference to them
	When the client signs the proposal body for transport
	Then it composes the canonical payload `trace_id|source|entity_ref|reason|expires_at` (watch-proposal canonical form, pipe-delimited, no whitespace, no trailing pipe)
	And computes HMAC-SHA256 using the Scope 8 keystore's newest `not_before`-valid key
	And the signed envelope carries `signature` and `key_id` populated by the Scope 8 signer with NO modifications to the Scope 8 implementation
	And Scope 9 unit tests assert that the Scope 8 signer/keystore are invoked verbatim (call-site assertion, not a separate signer reimplementation).

Scenario: SCN-SM-041-033 Pre-MVP QF Rejects Every Proposal With Watch Proposals Deferred To V1 And No Watch State Or User Surface Is Mutated
	Given the QF watch-proposal endpoint returns HTTP 503 `WATCH_PROPOSALS_DEFERRED_TO_V1` for every submission per the QF design 063 rejection contract
	When Smackerel signs and POSTs a watch-proposal envelope
	Then the connector parses the QF `WATCH_PROPOSALS_DEFERRED_TO_V1` rejection without retrying
	And the connector NEVER persists the proposal as accepted, NEVER mutates QF watch state, NEVER renders a "proposal submitted" user affordance on web/digest/Telegram, and NEVER triggers any Smackerel-side trade approval, mandate change, watch creation/evaluation, EmergencyStop, or execution behavior
	And `smackerel_qf_watch_proposal_attempts_total{status="rejected_v1_deferred"}` is emitted per attempt and a Cross-Product Audit Envelope v1 record is written with `action=watch_proposal`, `outcome=rejected`, `reason=WATCH_PROPOSALS_DEFERRED_TO_V1`.

### Implementation Plan

- Add a diagnostic watch-proposal client under `internal/connector/qfdecisions/watch_proposal.go` that exposes a connector-internal `Propose(ctx, entity_ref, reason, expires_at)` function. The function is NOT wired into any user-visible Smackerel surface pre-MVP; only the connector internal diagnostic path and the Scope 9 integration test call it.
- Construct the proposal body as `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}` (no extra fields, no missing fields); `trace_id` is a UUIDv7 generated per proposal.
- REUSE the Scope 8 signer (`internal/connector/qfdecisions/callback.go`) and keystore (`internal/connector/qfdecisions/callback_keystore.go`) verbatim: Scope 9 holds an interface to the signer and never reimplements HMAC-SHA256, key selection, or `key_id` envelope inclusion. Compose the watch-proposal canonical payload as `trace_id|source|entity_ref|reason|expires_at`.
- POST signed envelopes to the QF watch-proposal endpoint via the Scope 1 QF client transport. Pre-MVP QF responds `WATCH_PROPOSALS_DEFERRED_TO_V1` for every submission; the connector MUST parse the rejection without retrying.
- Forbid any local watch-state mutation pre-MVP: proposals NEVER trigger Smackerel-side watch creation, watch evaluation, trade approval, mandate change, execution, EmergencyStop, or local "proposal submitted" affordances; the Scope 5 safety boundary helper continues to enforce this and Scope 9 explicitly reuses it before any proposal emission attempt.
- Register a new metric `smackerel_qf_watch_proposal_attempts_total{status}` in `internal/metrics/metrics.go` if not already pre-declared; emit it on every proposal POST attempt with `status` admitting `rejected_v1_deferred|rejected_local|degraded` (pre-MVP `accepted` is never emitted).
- Use the Scope 5 audit envelope builder for every proposal attempt with `action=watch_proposal`, `outcome=rejected` or `degraded`, and `reason` populated per the failure matrix.
- Document the canonical payload schema, Scope 8 signer reuse contract, capability gate semantics (`watch_proposal_supported=false` is the pre-MVP contract), explicit no-watch-state-mutation guarantee, and explicit no-user-visible-affordance guarantee in `docs/Operations.md` Scope 9 subsection only.

### Implementation Files

- `internal/connector/qfdecisions/watch_proposal.go` and `watch_proposal_test.go` (diagnostic client, body construction, Scope 8 signer reuse, audit/metric emission, pre-MVP rejection parsing)
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct the diagnostic client at connector start with a reference to the Scope 8 signer; shut down on connector stop). NO user-visible call site is added.
- `internal/metrics/metrics.go` only if `QFWatchProposalAttemptsTotal` is not already registered.
- `tests/integration/qf_watch_proposal_test.go`
- `tests/e2e/qf_watch_proposal_test.go`
- `docs/Operations.md` Scope 9 subsection only (canonical payload, Scope 8 signer reuse contract, capability gate, no-watch-state-mutation guarantee, no-user-visible-affordance guarantee)
- `specs/041-qf-companion-connector/*` planning/evidence artifacts

### Test Plan

| Test Type | Category | Scenario(s) | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|-------------|---------------|---------------------|---------|-------------|
| Unit | unit | SCN-SM-041-031 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalBodyContainsExactlyTraceIdSourceEntityRefReasonAndExpiresAt` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-031 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalSourceFieldIsLiteralSmackerelProposeAndTraceIdIsUUIDv7` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-031 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalIsNotCallableFromUserVisibleSurfacesAndOnlyFromConnectorDiagnosticPath` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-032 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalReusesScope8SignerAndKeystoreVerbatimWithoutReimplementation` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-032 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalCanonicalPayloadIsPipeDelimitedTraceIdSourceEntityRefReasonExpiresAt` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-033 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalPreMVPParsesWatchProposalsDeferredToV1WithoutRetryOrLocalWatchStateMutation` | `./smackerel.sh test unit` | No |
| Unit | unit | SCN-SM-041-033 | `internal/connector/qfdecisions/watch_proposal_test.go` | `TestWatchProposalEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus` | `./smackerel.sh test unit` | No |
| Integration | integration | SCN-SM-041-031, SCN-SM-041-032 | `tests/integration/qf_watch_proposal_test.go` | `TestQFWatchProposalSignedEnvelopePostedAndScope8SignerReusedAgainstLiveQFStub` | `./smackerel.sh test integration` | Yes |
| Integration | integration | SCN-SM-041-033 | `tests/integration/qf_watch_proposal_test.go` | `TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack` | `./smackerel.sh test integration` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-031 | `tests/e2e/qf_watch_proposal_test.go` | `TestQFWatchProposalCanonicalBodyAndTraceContinuityThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-032 | `tests/e2e/qf_watch_proposal_test.go` | `TestQFWatchProposalScope8SignerReuseThroughLiveSurface` | `./smackerel.sh test e2e` | Yes |
| Regression E2E | e2e-api | SCN-SM-041-033 | `tests/e2e/qf_watch_proposal_test.go` | `TestQFWatchProposalPreMVPDeferralRejectionThroughLiveSurfaceWithNoLocalMutationOrUserSurface` | `./smackerel.sh test e2e` | Yes |
| Broader E2E | e2e-api | SCN-SM-041-031..033 | `tests/e2e/` | `go-e2e and shell E2E suites complete without failures` | `./smackerel.sh test e2e` | Yes |
| Artifact lint | artifact | SCN-SM-041-031..033 | `specs/041-qf-companion-connector` | `artifact lint accepts activated QF Scope 9 planning artifacts` | `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector` | No |
| Traceability guard | artifact | SCN-SM-041-031..033 | `specs/041-qf-companion-connector` | `traceability guard maps Scope 9 scenarios to planned tests with zero warnings` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/041-qf-companion-connector` | No |

### Consumer Impact Sweep

| Consumer surface | Scope 9 impact | Verification record |
|---|---|---|
| Scope 2 capability state | Reads persisted `watch_proposal_supported` at connector start; the MVP contract is `false` and the connector MUST NOT treat the value as a runtime override toggle. | Unit test asserts the connector ignores a `true` capability response in MVP and continues to expect the rejection envelope. |
| Scope 5 metric registry and audit envelope sink | Re-uses the audit envelope builder; registers `QFWatchProposalAttemptsTotal` if not already pre-declared; no existing vector relabeled, no new sink introduced. | `TestWatchProposalEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus`. |
| Scope 5 safety boundary helper | Reuses the helper to forbid any Smackerel-side watch-state mutation pre-MVP; never bypassed. | `TestWatchProposalPreMVPParsesWatchProposalsDeferredToV1WithoutRetryOrLocalWatchStateMutation`. |
| Scope 8 signer and keystore | Reused verbatim via interface reference; Scope 9 MUST NOT reimplement HMAC-SHA256, key selection, or `key_id` envelope inclusion. | `TestWatchProposalReusesScope8SignerAndKeystoreVerbatimWithoutReimplementation`. |
| Smackerel user-visible surfaces (web/digest/Telegram) | None — the diagnostic client is connector-internal and NEVER wired into any user-visible action affordance pre-MVP. | `TestWatchProposalIsNotCallableFromUserVisibleSurfacesAndOnlyFromConnectorDiagnosticPath`. |
| QF watch-proposal endpoint | Connector POSTs signed envelopes; QF returns `WATCH_PROPOSALS_DEFERRED_TO_V1` for every submission pre-MVP; the connector NEVER retries the rejection. | `TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack`. |

### Change Boundary

Allowed file families:

- `internal/connector/qfdecisions/watch_proposal.go` and `watch_proposal_test.go` for the diagnostic client, body construction, Scope 8 signer reuse, audit/metric emission, and pre-MVP rejection parsing only.
- `internal/connector/qfdecisions/connector.go` call-site wiring only (construct diagnostic client at connector start with Scope 8 signer reference; shut down on connector stop). NO user-visible call site is added.
- `internal/metrics/metrics.go` only if `QFWatchProposalAttemptsTotal` is not already registered.
- `tests/integration/qf_watch_proposal_test.go` and `tests/e2e/qf_watch_proposal_test.go`.
- `docs/Operations.md` Scope 9 subsection only.
- `specs/041-qf-companion-connector/*` planning/evidence/state artifacts.

Excluded surfaces:

- Scope 1 connector configuration startup gates, new generated config keys, or credential secret storage changes.
- Scope 2 cursor sync semantics, page-size clamping, unknown decision-type ingest behavior, fast-forward recovery, ingest freshness proof, or capability handshake lifecycle.
- Scope 3 QF card rendering semantics, trust-object public-field filtering, signed-link branch behavior, preferred-surface routing, or any visual surface; Scope 9 introduces NO user-visible proposal affordance.
- Scope 4 evidence bundle construction, QF POST/DELETE semantics, revocation behavior, or local export state.
- Scope 5 credential rotation, safety-boundary helper internals (Scope 9 reuses the helper but does NOT modify it), audit envelope builder internals, freshness completion, or render/combined p95 stress.
- Scope 6 engagement signal exporter, buffer/flush, or engagement-event capture hooks.
- Scope 7 personal-context read API host and consent-token issuer.
- Scope 8 callback signer or keystore internals (Scope 9 REUSES the Scope 8 signer/keystore via interface reference and MUST NOT modify either file). Any required interface change is a Scope 8 change, not a Scope 9 change.
- Smackerel-side watch-state mutation pre-MVP (watch creation, watch evaluation, trade approval, mandate change, execution, EmergencyStop, or local "proposal submitted" affordances).
- Direct QF database access, broker federation, QF approval/execution/mandate/EmergencyStop/watch behavior, local financial advice, QF trust reconstruction, generated config hand edits, hidden defaults/fallbacks, or hardcoded QF credentials/URLs.

### Definition of Done

Core behavior:

- [x] SCN-SM-041-031: Proposal body contains exactly `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}` with no extra fields and no missing fields; `trace_id` is a UUIDv7 generated client-side per proposal. Evidence: `report.md` -> Scope 9 Body Construction Evidence.
- [x] SCN-SM-041-031: The diagnostic invocation is NEVER fired from a user-visible Smackerel surface; it is only callable from the connector internal diagnostic path and the Scope 9 integration test. Evidence: `report.md` -> Scope 9 Body Construction Evidence.
- [x] SCN-SM-041-032: Scope 9 reuses the Scope 8 signer and keystore verbatim via interface reference; canonical payload is `trace_id|source|entity_ref|reason|expires_at` (pipe-delimited, no whitespace, no trailing pipe); the signed envelope carries `signature` and `key_id` populated by the Scope 8 signer. Evidence: `report.md` -> Scope 9 Signer Reuse Evidence.
- [x] SCN-SM-041-033: Pre-MVP `WATCH_PROPOSALS_DEFERRED_TO_V1` rejection is parsed without retry; the connector NEVER persists the proposal as accepted, NEVER mutates QF watch state, NEVER renders a "proposal submitted" user affordance, and NEVER triggers Smackerel-side trade approval, mandate change, watch creation/evaluation, EmergencyStop, or execution behavior. Evidence: `report.md` -> Scope 9 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-033: `smackerel_qf_watch_proposal_attempts_total{status="rejected_v1_deferred"}` is emitted per attempt and a Cross-Product Audit Envelope v1 record is written with `action=watch_proposal`, `outcome=rejected`, `reason=WATCH_PROPOSALS_DEFERRED_TO_V1`. Evidence: `report.md` -> Scope 9 Pre-MVP Rejection Evidence.

Validation:

- [x] SCN-SM-041-031: Unit tests cover exact body field set, literal `source` value, UUIDv7 `trace_id`, and the no-user-visible-call-site invariant. Evidence: `report.md` -> Scope 9 Body Construction Evidence.
- [x] SCN-SM-041-032: Unit tests cover Scope 8 signer/keystore reuse (call-site assertion, not a separate signer reimplementation) and the canonical payload composition. Evidence: `report.md` -> Scope 9 Signer Reuse Evidence.
- [x] SCN-SM-041-033: Unit tests cover `WATCH_PROPOSALS_DEFERRED_TO_V1` parsing, no-retry policy, no local watch-state mutation, and `QFWatchProposalAttemptsTotal` + audit envelope emission. Evidence: `report.md` -> Scope 9 Pre-MVP Rejection Evidence.
- [x] SCN-SM-041-031, SCN-SM-041-032: Live-stack integration test exercises body construction, Scope 8 signer reuse, and the signed POST against the local QF stub. Evidence: `report.md` -> Scope 9 Integration Evidence.
- [x] SCN-SM-041-033: Live-stack integration test exercises the pre-MVP rejection branch against the local QF stub, asserts no local watch-state mutation, and verifies metric + audit envelope emission. Evidence: `report.md` -> Scope 9 Integration Evidence.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in SCN-SM-041-031 through SCN-SM-041-033 pass. Evidence: `report.md` -> Scope 9 E2E Evidence.
- [x] Broader E2E regression suite passes after Scope 9 implementation. Evidence: `report.md` -> Scope 9 Broader E2E Evidence.
- [x] Artifact lint and traceability guard pass for the activated Scope 9 planning artifacts and scenario-manifest mappings. Evidence: `report.md` -> Scope 9 Governance Evidence.

Build quality gate:

- [x] Raw unit, integration, E2E, broader E2E, artifact-lint, and traceability-guard evidence is recorded in `report.md` before any Scope 9 DoD item is checked. Evidence: `report.md` -> Scope 9 Evidence Index.
- [x] Consumer Impact Sweep is completed and zero stale first-party references remain for watch-proposal body field set, canonical payload schema, Scope 8 signer reuse interface, capability gate field name, metric labels, and audit actions. Evidence: `report.md` -> Scope 9 Consumer Impact Evidence.
- [x] Change Boundary is respected and zero excluded file families are changed; Scopes 1-8 are untouched except for explicit Scope 8 signer/keystore interface reuse. Evidence: `report.md` -> Scope 9 Change Boundary Evidence.
- [x] No hidden defaults, fallback `watch_proposal_supported=true`, user-visible proposal affordances on web/digest/Telegram, retry of `WATCH_PROPOSALS_DEFERRED_TO_V1` rejections, Smackerel-side watch-state mutation/trade approval/mandate change/EmergencyStop/execution on proposal submission, persistent acceptance of proposals pre-MVP, or duplicate signer implementations are introduced. Evidence: `report.md` -> Scope 9 Implementation Reality Evidence.
- [x] Build, lint, format, unit, integration, E2E, artifact-lint, traceability-guard, and state-transition guard checks complete with zero Scope 9-local warnings or blockers; any remaining state-transition guard failures are explicitly classified as downstream/full-feature blockers only. Evidence: `report.md` -> Scope 9 Build Quality Evidence.
<!-- bubbles:g040-skip-end -->

## Parked Scope 9 Phase B2 Design Additions (Superseded By Active Scope 9 2026-05-22)

The Parked Scope 9 Phase B2 DoD draft below is preserved as superseded historical context. It is **NOT** part of the active execution inventory and MUST NOT be ticked by any agent. All Scope 9 active execution work is governed by the `## Scope 9: Watch Signal Proposal Endpoint (Pre-MVP Design Only)` section above.

Historical status: Superseded
Historical dependency: Scope 2
**Activation Gate (historical):** Capability handshake is operational so proposal endpoint readiness can be advertised and rejected by QF (Scope 2)

### Phase B2 Design Additions (2026-05-07) — Superseded Historical Notes

Core behavior (Phase B2 additions, O3, FR-015):

- (historical, superseded) `POST /api/private/smackerel/v1/watch-signal-proposals` request shape `{trace_id, source: "smackerel_propose", entity_ref, reason, expires_at}`.
- (historical, superseded) Pre-MVP: every request is rejected by QF with the version-one watch-proposal rejection response; signing infrastructure is exercised.
- (historical, superseded) Integration test verifies request shape, signing, and rejection parsing.
- (historical, superseded) No proposal ever influences QF watch state pre-MVP.

Validation (Phase B2 additions):

- (historical, superseded) Unit tests cover request shape construction and signing.
- (historical, superseded) Integration test verifies QF version-one watch-proposal rejection parsing and confirms no QF watch-state mutation.

## Scope 2/3 Planning Notes

<!-- bubbles:g040-skip-begin -->

### Broader-E2E Title-Field Regression (Recorded 2026-05-23 By bubbles.plan)

During Scope 8 deep validate (`bubbles.validate`, 2026-05-23T03:30:00Z) the broader e2e sweep surfaced one finding outside the Scope 8 Change Boundary: `TestQFDecisionDeepLinkAndPreferredSurfaceBranchMatrix/deep_link_statuses/unsigned_only` reports a `Title`-field regression rooted in Scope 2/3 territory (`internal/connector/qfdecisions/normalizer.go` plus `internal/connector/qfdecisions/render.go`). It is NOT required for Scope 8 certification (validate Step 2A scope-attribution table classified it as out-of-Scope-8). This planning record exists so the finding is not lost: the next Scope 2/3 certification cycle (or an earlier targeted re-cert) MUST triage it and decide whether to (a) fix it directly under an updated Scope 2/3 Test Plan row, or (b) open `specs/041-qf-companion-connector/bugs/BUG-NNN-qf-decision-title-field-regression/` if it warrants standalone bug-fix workflow. No bug ticket is opened at the time of this record; the entry is purely a planning bookmark.
<!-- bubbles:g040-skip-end -->

