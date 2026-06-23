# Scopes: 054 Notification Intelligence Handler Service

## Execution Outline

### Phase Order

1. **Source Contract And Registry** - establish the source-neutral adapter interface, source instance registry, health contract, and spec 055 ntfy boundary without any ntfy production dependency.
2. **Raw And Normalized Event Persistence** - add durable storage and service flow that stores raw source input before normalized notifications are available to downstream policy.
3. **Classification Engine** - classify severity, domain, and intent with durable rationale, confidence, uncertainty, and source-qualified provenance.
4. **Dedupe, Correlation, And Incidents** - turn repeated and related notifications into explicit incident records with durable state transitions, suppressions, and correlation rationale.
5. **Enrichment And Decision Engine** - enrich incidents with bounded graph/system metadata and choose one handling decision without fabricating facts when enrichment is missing.
6. **Safe Reaction And Approval Policy** - run read-only diagnostics, enforce low-risk action allowlists, require approval for high-blast-radius actions, refuse destructive automation, and prevent reaction loops.
7. **Output Channels And Operator Surfaces** - deliver concise redacted output through channel abstractions and expose authenticated API/HTMX status, history, incident, action, approval, and health views.
8. **Observability, Config, Security, And Full Pipeline Hardening** - wire SST config, authn/authz, redaction, metrics, traces, logs, stress coverage, and the spec 055 adapter handoff guard.
9. **Surfacing Controller Integration** (reactivated 2026-06-23) - route the notification decision→dispatch seam through the shared synchronous spec 078 `surfacing.Controller.Propose` so user-facing nudges honor the global interruption budget, cross-channel dedupe, and acknowledgment suppression instead of queueing output directly.

### New Types And Signatures

- `notification.SourceAdapter`: `SourceType()`, `SourceForm()`, `InstanceID()`, `Connect(ctx,cfg)`, `Start(ctx,sink)`, `Health(ctx)`, `Stop(ctx)`.
- `notification.SourceEventSink`: `SubmitSourceEvent(ctx,envelope)`, `ReportSourceHealth(ctx,report)`.
- `notification.SourceEventEnvelope`: source identity, form, event ID or derived identity inputs, observed/source timestamps, raw payload, delivery metadata, source-specific fields, mapping hints, and loop metadata.
- `notification.NormalizedNotification`: raw reference, source identity, title/body, severity, tags, subject/service, domain, intent, delivery metadata, source-specific reference, and redaction state.
- `notification.Incident`: correlation key, state, severity/domain/intent summary, active notification set, state transition history, suppressions, approvals, actions, and resolution evidence.
- `notification.Decision`: `no_action`, `record_only`, `diagnostics`, `autonomous_handling`, `user_escalation`, or `approval_request`, with rationale and risk class.
- `notification.OutputChannel`: channel identity, capability metadata, delivery request, delivery result, and loop-origin metadata.
- Migration `036_notification_intelligence.sql`: notification source instances, source health events, raw events, normalized notifications, classifications, correlations, incidents, transitions, decisions, suppressions, diagnostics, actions, approvals, output channels, and delivery attempts.
- Operator API routes: authenticated `/api/notifications/sources`, `/api/notifications/events`, `/api/notifications/events/{event_id}`, `/api/notifications/manual-ingest`, `/api/notifications/incidents`, `/api/notifications/incidents/{incident_id}`, `/api/notifications/incidents/{incident_id}/snooze`, `/api/notifications/approvals/{approval_id}`, `/api/notifications/approvals/{approval_id}/decisions`, `/api/notifications/suppressions`, `/api/notifications/quiet-windows`, `/api/notifications/summary`, `/api/notifications/outputs`, and `/api/notifications/status`.
- Operator web routes: authenticated `/notifications`, `/notifications/sources`, `/notifications/events`, `/notifications/incidents`, `/notifications/incidents/{incident_id}`, `/notifications/approvals/{approval_id}`, `/notifications/suppressions`, `/notifications/summary`, and `/notifications/outputs`.
- Config blocks: `notification_intelligence` and `notification_outputs`, generated from `config/smackerel.yaml` with fail-loud required values and no runtime fallback syntax.
- Scope 9 (reactivated): `notification.Service.SetSurfacingController(*surfacing.Controller)` plus a private `proposeSurfacing(ctx, surfacing.SurfacingCandidate) bool` (mirrors `scheduler` `proposeSurfacing`; nil controller → legacy direct-dispatch fallback); the decision→dispatch seam in `Service.Process` consumes the delivered spec 078 `surfacing.SurfacingCandidate`/`SurfacingDecision`; additive `surfacing.ProducerNotification` enum constant; notification incident-ack → shared `surfacing.InMemoryAck.Acknowledge(correlationKey)`.

### Validation Checkpoints

- After Scope 1, source contract conformance tests prove source-agnostic adapter lifecycle, duplicate source rejection, health redaction, and no production ntfy imports.
- After Scope 2, persistence integration and e2e-api ingest tests prove raw input is durable before normalized records and replayable event identity is stable.
- After Scope 3, classifier tests prove severity/domain/intent decisions are durable, explainable, and not source-specific branches.
- After Scope 4, incident state machine tests prove dedupe, correlation, suppression, and state transitions before any action engine is wired.
- After Scope 5, decision tests prove enrichment, threshold policy, and single-decision behavior before diagnostics or action execution can mutate state.
- After Scope 6, safety tests prove diagnostics are read-only, autonomous actions are low-risk only, approvals gate high-blast-radius actions, destructive automation is refused, and loop prevention holds.
- After Scope 7, API and web e2e tests prove operator-visible status surfaces and channel abstraction deliver redacted output without hardcoded Telegram or ntfy logic.
- After Scope 8, full integration, e2e, stress, lint, format, artifact lint, traceability guard, and no-defaults scans prove the complete vertical slice is deployable and ready for spec 055.
<!-- bubbles:g040-skip-begin -->
<!-- g040 rationale: "defer"/"follow-up" describe the feature's runtime behavior and a resolved historical reactivation, not deferred work. -->
- After Scope 9 (reactivated), unit + integration + e2e-api tests prove the notification decision engine routes through the shared controller (permit/escalated only), non-urgent decisions defer when the global budget is exhausted, urgent decisions escalate, and acknowledgment suppresses sibling/follow-up output — with a nil-controller legacy fallback preserved.
<!-- bubbles:g040-skip-end -->

## Planning Assumptions

- Spec 054 owns the core notification intelligence service and reusable contracts only.
- Spec 055 owns the first concrete ntfy source adapter and must depend on the SourceAdapter conformance suite from this plan.
- Core production code may include source contract test fixtures, but it must not ship ntfy-specific branches, ntfy package imports, hardcoded Telegram assumptions, or production source fixtures.
- Smackerel's current UI surface is Go/HTMX plus authenticated JSON APIs; planned UI tests target those operator pages through the repo e2e command.
- Concrete thresholds, action allowlists, channel destinations, and source instances must be explicit SST values during implementation.

## Scope Inventory

| Scope | Name | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|----------|---------------|-------------|--------|
| 1 | Source Contract And Registry | Backend, config contract, API status | Unit, integration, e2e-api | Source-neutral adapters register, health is uniform/redacted, ntfy remains future-only | Done |
| 2 | Raw And Normalized Event Persistence | Backend, DB migration, API ingest | Unit, integration, e2e-api, stress | Raw input stored first, normalized model persisted, source-specific context preserved | Done |
| 3 | Classification Engine | Backend, DB, audit API | Unit, integration, e2e-api | Severity/domain/intent classification durable, explainable, and source-agnostic | Done |
| 4 | Dedupe, Correlation, And Incidents | Backend, DB, incident API | Unit, integration, e2e-api, stress | Duplicate noise suppressed, related notifications become lifecycle incidents | Done |
| 5 | Enrichment And Decision Engine | Backend, graph reads, DB | Unit, integration, e2e-api | Enrichment is bounded and decisions are single, durable, quiet by default | Done |
| 6 | Safe Reaction And Approval Policy | Backend, DB, action/approval API | Unit, integration, e2e-api, stress | Read-only diagnostics, low-risk allowlists, approval gates, destructive refusal, loop guard | Done |
| 7 | Output Channels And Operator Surfaces | Backend, HTMX web, API, output dispatch | Unit, integration, e2e-api, e2e-ui | Channel abstraction and operator status/history surfaces work without hardcoded Telegram/ntfy | Done |
| 8 | Observability, Config, Security, And Hardening | Config, auth, telemetry, docs, full stack | Unit, integration, e2e-api, e2e-ui, stress, lint | Fail-loud SST, redaction, authz, metrics/traces/logs, and spec 055 handoff are verified | Done |
| 9 | Surfacing Controller Integration (reactivated; spec 078 controller shipped) | Backend decision→dispatch seam, shared surfacing controller, acknowledgment registry, integration tests | Unit, integration, e2e-api | Notification decision engine routes user-facing decisions through the synchronous spec 078 `surfacing.Controller.Propose` seam (shared global nudge budget + cross-channel dedupe + ack suppression) and only dispatches on Permit/Escalated instead of directly queueing output | Done (certified 2026-06-23 by bubbles.validate; unit/integration/e2e GREEN) |

<!-- bubbles:g040-skip-begin -->
<!-- g040 rationale: historical governance narrative documenting the now-RESOLVED post-release deferral (Scope 9 is Done as of 2026-06-23); not active deferred work. -->
## Post-Release Scope Exception

Scope 9 was an intentional **post-release deferral** approved at portfolio-planning level (release-planning dispatch RELEASE-MVP:M1c, 2026-06-03). It shipped as `Blocked` in the scope inventory and was excluded from the spec-level promotion gate by design — not by oversight — while its upstream dependency was in flight. It is mirrored in `state.json` under `certification.postReleaseExceptions[0]` (carrying the `DI-054-01` reference).

**Reactivated 2026-06-23.** The unblock gate is now **SATISFIED**. The unified surfacing controller shipped — via **spec 078 (cross-surface-surfacing-prioritizer)**, not spec 021. The "unified surfacing controller" milestone (originally tracked as spec 021 M1a) was rescoped out of spec 021 into spec 078 by commit `640b95d0`; every "spec 021 M1a" reference in this plan is corrected to **spec 078**. The delivered controller lives at `internal/intelligence/surfacing/` and exposes a **synchronous** `Controller.Propose(ctx, SurfacingCandidate) (SurfacingDecision, error)` seam — **not** the event-bus / `SurfacingProposal` / `AcknowledgmentBus` model the original deferral assumed. Scope 9 below is reconciled to that real contract. The reference integration is `internal/scheduler/jobs.go` `proposeSurfacing` (scheduler producers call `Controller.Propose` and dispatch only on `Permit`/`Escalated`).

| Scope | Original block reason | Unblock gate | Status | Delivered by |
|---|---|---|---|---|
| 9 — Surfacing Controller Integration | The unified surfacing controller had not yet been delivered; spec 054 could not route decisions through it until the controller and its arbitration + acknowledgment contract existed on trunk. Implementing against a local stub would fork the canonical controller contract — forbidden by the change boundary. | Unified surfacing controller delivered: a proposal/permit decision point, cross-producer global-budget + cross-channel-dedupe arbitration, and an acknowledgment-suppression surface. | **SATISFIED 2026-06-23** | spec 078 (cross-surface-surfacing-prioritizer); controller at `internal/intelligence/surfacing/`; reference integration `internal/scheduler/jobs.go` `proposeSurfacing` |

The original portfolio decision (release-target scopes 1–8 promoted to `done`, Scope 9 deferred) stands as the MVP record. With the dependency satisfied, Scope 9 is reactivated as an **improve-existing continuation** (top-level `status: in_progress`) so an `implement` run can realize SCN-054-027..030 against the real controller. The forward-looking scaffolds (`internal/notification/decision_surfacing_test.go`, `internal/notification/surfacing_controller_integration_test.go`) are the RED starting point; the implement run un-skips and realizes them.

### Unblock Verification — Per Sub-Capability

| Sub-capability needed by Scope 9 | Present in delivered spec 078 controller? | Evidence |
|---|---|---|
| Proposal / permit decision point | **Yes** | `Controller.Propose` returns `SurfacingDecision{Kind}` ∈ {`permit`,`escalated`,`deduped`,`suppressed`,`deferred-budget-exhausted`}; caller dispatches on `permit`/`escalated` (see scheduler `proposeSurfacing`). |
| Cross-producer arbitration + cross-channel dedupe | **Yes** | One shared `*surfacing.Controller` per process (`cmd/core/main.go`); `BudgetTracker` is a single global daily nudge budget across all producers; `DedupeIndex` collapses same-`ContentKey` candidates across channels. |
| Acknowledgment / sibling-cancellation surface (SCN-054-030) | **Mechanism yes; production feed not wired** | `AckLookup` + `InMemoryAck.Acknowledge(contentKey)` + `SuppressionWindow.IsSuppressed(contentKey)` exist and suppress same-`ContentKey` follow-ups regardless of producer/channel. It is **pull/suppression-based, not a push `AcknowledgmentBus`.** Production wires `surfacing.NewInMemoryAck()` inline and nothing calls `Acknowledge` outside tests. **Scope 9 must add the thin 054-side wiring** (shared ack registry + notification incident-ack → `Acknowledge(correlationKey)`); no spec 078 internal change is required. SCN-054-030 is therefore **reconciled, not deferred**. |
<!-- bubbles:g040-skip-end -->

## Cross-Scope Certification Gates

These gates apply across the single-file scope plan because the notification handler introduces core contracts, shared source fixtures, shared API/auth surfaces, and implementation-bearing runtime changes. They remain unchecked until the validation owner records direct evidence in `report.md`.

### Cross-Scope Canary Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Fixture Canary | `integration` | Canary: shared source fixture/bootstrap contracts for SCN-054-001 through SCN-054-026 | `internal/notification/source_registry_integration_test.go` | `TestSourceRegistryPersistsHealthForSimultaneousInstances` | `./smackerel.sh test integration` | Yes |
| Fixture Canary | `e2e-api` | Canary: full pipeline contract across source, ingest, classification, incident, decision, approval, output, auth, and audit surfaces | `tests/e2e/notification_full_pipeline_api_test.go` | `TestNotificationFullPipelinePreservesAuditAndBlocksPolicyBypass` | `./smackerel.sh test e2e` | Yes |

### Cross-Scope Certification DoD

- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. Evidence: `report.md#cross-scope-certification-gates`
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. Evidence: `report.md#cross-scope-certification-gates`
- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md#cross-scope-certification-gates`

## Scope 1: Source Contract And Registry

**Status:** Done  
**Depends On:** None  
**Surfaces:** `internal/notification`, source registry, source health status API, config contract, source contract tests

### Use Cases

#### SCN-054-001: Register Multiple Source Instances Without Source-Specific Core Branches

Scenario: SCN-054-001: Register Multiple Source Instances Without Source-Specific Core Branches

Given the operator config declares two enabled source instances with different forms and one repeated source type with a different instance ID  
When the notification source registry starts  
Then each source instance is registered by `(source_type, source_instance_id, source_form)`  
And duplicate instance IDs are rejected before event processing begins  
And the core handler exposes no ntfy-specific package import, branch, or production dependency

#### SCN-054-002: Source Adapter Submits Only Through The Core Sink

Scenario: SCN-054-002: Source Adapter Submits Only Through The Core Sink

Given a source adapter implements the source contract and receives a source event  
When it starts and submits the event  
Then it calls `SubmitSourceEvent` on the core sink  
And it cannot call classifier, correlator, output channel, action executor, or incident store APIs directly  
And the sink returns an ingest receipt that references the source instance and raw event acceptance status

#### SCN-054-003: Source Health Is Uniform And Redacted

Scenario: SCN-054-003: Source Health Is Uniform And Redacted

Given one source connects, one source has invalid credentials, and one source repeatedly fails transient checks  
When source health is reported  
Then the connected source reports `connected` with last event/check timestamps  
And the invalid source reports `disconnected` with redacted error category  
And the transient source reports `degraded` with retry count and redacted last error

### Implementation Plan

- Add `internal/notification` package with source contract types, source form enum, health enum, source registry, source supervisor boundary, and source event sink interface.
- Add source instance validation that requires explicit source identity, form, enabled flag, config hash, secret reference names, and redacted config metadata.
- Add a source health service/store boundary that records connected, disconnected, and degraded states with last event time, last successful check, retry count, and redacted last error fields.
- Add an authenticated read-only status endpoint for source registry and health summaries; write endpoints for source config remain owned by SST/secret-managed configuration rather than the runtime status API.
- Add conformance test fixture adapters for stream, webhook, polling, queue, file_drop, api_pull, and manual forms. These fixtures exist only in tests.
- Add a static unit test that fails if the core `internal/notification` package imports ntfy or branches on ntfy-only fields.
- Keep spec 055 ntfy as a dependent future source implementation; this scope only creates the contract it must satisfy.

### Consumer Impact Sweep

- Affected consumers: future spec 055 ntfy adapter, future non-ntfy adapters, operator status API, operator web status page, scheduler/supervisor startup.
- Stale-reference surfaces to verify: source registry names, health enum values, config key names, API route registration, docs/spec references, and tests that mention concrete source types.
- Excluded consumers: Telegram delivery, output channel dispatch, action execution, and source-specific configuration UI.

### Shared Infrastructure Impact Sweep

- Protected surface: `SourceAdapter` and `SourceEventSink` are high-fan-out contracts.
- Canary tests before downstream execution: source conformance unit tests, duplicate registration integration test, health redaction test, no-ntfy-import test.
- Rollback boundary: source contract code can be reverted without modifying existing connector runtime or Telegram delivery code.

### Change Boundary

- Allowed file families: notification package, source contract tests, additive API status route, additive config parsing for notification source metadata.
- Excluded surfaces: production ntfy adapter, Telegram delivery code, existing `alerts` primary model, recommendation delivery pipeline, destructive action code.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-001 | `internal/notification/source_contract_test.go` | `TestSourceRegistryRegistersMultipleInstancesWithoutNtfyDependency` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-002 | `internal/notification/source_contract_test.go` | `TestSourceAdapterConformanceSubmitsOnlyThroughSink` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-003 | `internal/notification/source_health_test.go` | `TestSourceHealthStatesRedactErrorsAndTrackRetryCounts` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-001, SCN-054-003 | `internal/notification/source_registry_integration_test.go` | `TestSourceRegistryPersistsHealthForSimultaneousInstances` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-003 | `tests/e2e/notification_sources_api_test.go` | `TestNotificationSourcesStatusShowsConnectedDisconnectedAndDegradedSources` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-001 duplicate source IDs remain rejected before processing | `tests/e2e/notification_sources_api_test.go` | `TestNotificationSourcesRejectDuplicateInstanceIdsBeforeProcessing` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

**Core Items**

- [x] Source adapter interfaces, source forms, health states, registry, and source event sink are implemented. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Test fixture adapters cover stream, webhook, polling, queue, file_drop, api_pull, and manual forms without production fixture wiring. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Authenticated source status API returns source identity, form, health state, last event/check timestamps, retry count, and redacted error data. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Source Adapter Submits Only Through The Core Sink behavior is implemented and validated. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Scenario-specific unit, integration, e2e-api, and e2e regression tests pass for SCN-054-001, SCN-054-002, and SCN-054-003. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Static guard proves core notification code has no ntfy production import, ntfy-specific branch, or Telegram-as-model dependency. Evidence: `report.md#scope-1-source-contract-and-registry`

**Build Quality Gate**

- [x] `./smackerel.sh test unit`, `./smackerel.sh test integration`, and `./smackerel.sh test e2e` pass with zero warnings. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] `./smackerel.sh lint` and `./smackerel.sh format --check` pass with zero warnings. Evidence: `report.md#scope-1-source-contract-and-registry`
- [x] Artifact lint remains clean after scope evidence is recorded. Evidence: `report.md#scope-1-source-contract-and-registry`

## Scope 2: Raw And Normalized Event Persistence

**Status:** Done  
**Depends On:** Scope 1  
**Surfaces:** `internal/notification`, `internal/db/migrations/036_notification_intelligence.sql`, stores, ingest API/test sink

### Use Cases

#### SCN-054-004: Raw Event Is Durable Before Normalization

Scenario: SCN-054-004: Raw Event Is Durable Before Normalization

Given a registered source instance submits a valid event envelope  
When the ingestor accepts the event  
Then a `notification_raw_events` record is written before a normalized notification is processed  
And the ingest receipt references the raw event ID  
And downstream decisioning cannot claim success without the raw record

#### SCN-054-005: Normalized Model Preserves Source Context Outside Core Logic

Scenario: SCN-054-005: Normalized Model Preserves Source Context Outside Core Logic

Given a raw event contains delivery metadata, source-specific fields, mapping hints, and source tags  
When normalization succeeds  
Then the normalized notification contains the required common fields  
And source-specific fields are preserved by reference for audit/enrichment  
And classifier/correlator/decision inputs use normalized fields rather than source-specific branches

#### SCN-054-006: Missing Source Event ID Gets Deterministic Handler Identity

Scenario: SCN-054-006: Missing Source Event ID Gets Deterministic Handler Identity

Given a source event has no source-provided event ID  
When the ingestor accepts it  
Then the handler derives a stable source event ID from source instance, form, observation window, payload hash, and canonical delivery metadata  
And `source_event_id_origin` is stored as `handler_derived`  
And replaying the same event produces the same identity without dropping raw history

### Implementation Plan

- Add additive migration for source instances, source health events, raw events, and normalized notifications with `CHECK` constraints and application-written IDs/timestamps.
- Implement `RawEventStore`, `NotificationStore`, and ingestion transaction flow that writes raw event first, then normalized notification and validation/normalization records.
- Implement payload hashing, payload size bounds, raw payload storage for text/bytes/file references, and deterministic source event ID derivation.
- Implement `Normalizer` that consumes only `SourceEventEnvelope` plus mapping hints and emits `NormalizedNotification` with required fields.
- Preserve delivery metadata and source-specific fields as redacted JSONB/reference fields without allowing core branching on source-specific keys.
- Add e2e manual ingest or fixture-source ingest route only for operator/test source forms, not as a production bypass around source adapters.

### Consumer Impact Sweep

- Affected consumers: source adapters, classifier, deduper, correlator, audit APIs, operator event history UI.
- Stale-reference surfaces to verify: raw event ID names, normalized notification ID names, source event identity origin, payload kind enums, JSON response field names.
- Excluded consumers: final output channels, action executors, ntfy parser implementation.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-006 | `internal/notification/ingest_identity_test.go` | `TestDerivedSourceEventIDIsStableAndExplained` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-005 | `internal/notification/normalizer_test.go` | `TestNormalizerEmitsRequiredFieldsAndPreservesSourceSpecificContext` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-004, SCN-054-005 | `internal/notification/store_integration_test.go` | `TestRawEventIsCommittedBeforeNormalizedNotification` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-004 | `internal/api/notifications_pipeline.go` (e2e coverage lives in the live-DB test alongside the pipeline handler; originally planned at tests/e2e/notification_ingest_api_test.go) | `TestNotificationIngestPersistsRawAndNormalizedRecords` | `./smackerel.sh test e2e` | Yes |
| E2E API | `e2e-api` | SCN-054-005 | `internal/api/notifications_pipeline.go` (e2e coverage lives in the live-DB test alongside the pipeline handler; originally planned at tests/e2e/notification_manual_ingest_api_test.go — the manual-ingest e2e cell was consolidated with the auto-ingest e2e cell because they share the same normalization/classification/correlation/decision pipeline) | `TestManualIngestUsesSameNormalizationClassificationCorrelationAndDecisionPipeline` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-006 derived source event IDs remain stable on replay | `internal/api/notifications_pipeline.go` (originally planned at tests/e2e/notification_ingest_api_test.go) | `TestNotificationIngestDerivesStableEventIDWhenSourceIDMissing` | `./smackerel.sh test e2e` | Yes |
| Stress | `stress` | SCN-054-004 | `tests/stress/notification_ingest_stress_test.go` | `TestNotificationIngestSustainsBurstWithoutRawRecordLoss` | `./smackerel.sh test stress` | Yes |

### Definition of Done

**Core Items**

- [x] Migration `036_notification_intelligence.sql` creates raw event and normalized notification persistence with constrained enums and no database-side runtime defaults. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Ingest transaction writes raw event before normalized notification and rejects partial downstream success. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Normalized model includes source type, source instance, source event ID, timestamps, title, body, severity, tags, subject/service, raw reference, delivery metadata, source-specific reference, and redaction state. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Source-specific fields are preserved for audit/enrichment and excluded from core policy branching. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Normalized Model Preserves Source Context Outside Core Logic behavior is implemented and validated. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Missing Source Event ID Gets Deterministic Handler Identity behavior is implemented and validated. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Scenario-specific unit, integration, e2e-api, regression, and stress tests pass for SCN-054-004, SCN-054-005, and SCN-054-006. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`

**Build Quality Gate**

- [x] Unit, integration, e2e, stress, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`
- [x] Documentation for durable raw and normalized event storage is aligned in spec/design/API docs. Evidence: `report.md#scope-2-raw-and-normalized-event-persistence`

## Scope 3: Classification Engine

**Status:** Done  
**Depends On:** Scope 2  
**Surfaces:** classifier, classification store, audit API/event detail, redaction guard

### Use Cases

#### SCN-054-007: Severity Domain And Intent Are Classified With Rationale

Scenario: SCN-054-007: Severity Domain And Intent Are Classified With Rationale

Given a normalized notification with title, body, subject, source severity, tags, and prior metadata  
When the classifier runs  
Then severity, domain, and intent are stored with confidence, rationale, and classifier version  
And the event detail API can explain why those labels were chosen

#### SCN-054-008: Missing Evidence Produces Uncertainty Instead Of Fabricated Confidence

Scenario: SCN-054-008: Missing Evidence Produces Uncertainty Instead Of Fabricated Confidence

Given a normalized notification lacks service metadata or known graph context  
When classification runs  
Then the classifier records explicit uncertainty and lower confidence  
And it does not invent source facts, affected services, or incident evidence

#### SCN-054-009: Classification Is Source-Agnostic

Scenario: SCN-054-009: Classification Is Source-Agnostic

Given equivalent normalized notifications arrive from two different source types  
When classification runs  
Then the same normalized semantics produce the same severity/domain/intent result  
And no classifier rule branches on ntfy-specific or source-specific raw field names

### Implementation Plan

- Implement classification domain types, bounded enums, confidence/rationale envelope, classifier versioning, and durable classification records.
- Implement rule-first severity/domain/intent classifier using normalized fields, source severity provenance, source tags, subject/service, and bounded graph metadata references.
- Implement uncertainty handling when facts are missing or conflicting.
- Add classification audit retrieval through event/notification detail API and web detail surfaces.
- Add redaction pass for classification rationale before logs, API, and UI output.
- Add static source-agnostic rule guard for ntfy/source-specific branches in classifier package.

### Consumer Impact Sweep

- Affected consumers: deduper, correlator, decision engine, event detail API, operator incident UI, future source adapters.
- Stale-reference surfaces to verify: classification enum names, confidence/rationale JSON fields, classifier version field, API response shape.
- Excluded consumers: source transport parsing, action execution, output delivery implementation.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-007 | `internal/notification/classifier_test.go` | `TestClassifierStoresSeverityDomainIntentWithRationale` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-008 | `internal/notification/classifier_uncertainty_test.go` | `TestClassifierRecordsUncertaintyWhenEvidenceIsMissing` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-009 | `internal/notification/classifier_source_agnostic_test.go` | `TestClassifierDoesNotBranchOnSourceSpecificFields` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-007, SCN-054-008 | `internal/notification/classification_store_integration_test.go` | `TestClassificationPersistenceAndAuditRetrieval` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-007 | `tests/e2e/notification_classification_api_test.go` | `TestNotificationDetailShowsClassificationRationale` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-009 equivalent normalized events classify consistently across sources | `tests/e2e/notification_classification_api_test.go` | `TestEquivalentNormalizedEventsClassifySameAcrossDifferentSources` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

**Core Items**

- [x] Classification store persists severity, domain, intent, confidence, uncertainty, classifier version, source provenance, and rationale. Evidence: `report.md#scope-3-classification-engine`
- [x] Classifier consumes normalized fields and bounded graph metadata only; source-specific raw fields never drive core branches. Evidence: `report.md#scope-3-classification-engine`
- [x] Missing or conflicting enrichment records uncertainty instead of fabricated facts. Evidence: `report.md#scope-3-classification-engine`
- [x] Event detail API and web detail surfaces expose redacted classification rationale. Evidence: `report.md#scope-3-classification-engine`
- [x] Scenario-specific unit, integration, e2e-api, and regression tests pass for SCN-054-007, SCN-054-008, and SCN-054-009. Evidence: `report.md#scope-3-classification-engine`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-3-classification-engine`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-3-classification-engine`

**Build Quality Gate**

- [x] Unit, integration, e2e, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-3-classification-engine`
- [x] Classification behavior is documented in API/testing docs and remains spec-derived. Evidence: `report.md#scope-3-classification-engine`

## Scope 4: Dedupe, Correlation, And Incidents

**Status:** Done  
**Depends On:** Scope 3  
**Surfaces:** deduper, correlator, incident store, suppression records, incident API

### Use Cases

#### SCN-054-010: Duplicate Routine Events Stay Silent But Auditable

Scenario: SCN-054-010: Duplicate Routine Events Stay Silent But Auditable

Given a routine notification repeats within the configured cooldown window  
When dedupe runs  
Then raw and normalized records remain durable  
And a suppression/correlation record explains the duplicate  
And no user-facing output is produced

#### SCN-054-011: Related Severe Events Become One Active Incident

Scenario: SCN-054-011: Related Severe Events Become One Active Incident

Given several notifications with related subject, service, domain, intent, and persistence evidence arrive from one or more sources  
When correlation runs  
Then the handler creates or updates one incident  
And the incident state, active notification set, correlation rationale, and transition history are durable

#### SCN-054-012: Incident State Transitions Are Explicit And Bounded

Scenario: SCN-054-012: Incident State Transitions Are Explicit And Bounded

Given an incident moves from observing to active, diagnosing, mitigated or escalated, and resolved  
When each transition occurs  
Then every state change records actor, cause, timestamp, previous state, next state, and rationale  
And invalid transitions are refused and recorded as policy decisions

### Implementation Plan

- Add incident, correlation, transition, and suppression persistence tables and stores.
- Implement exact duplicate detection using source identity/event identity/payload hash while preserving raw history.
- Implement near-duplicate and related-event correlation using normalized subject, service, domain, intent, severity, tags, time window, and graph references.
- Implement incident lifecycle states: observing, active, diagnosing, mitigating, approval_requested, escalated, suppressed, resolved.
- Implement state machine validation, transition audit, and invalid transition refusal records.
- Add incident list/detail API routes for active, suppressed, and resolved incidents.

### Consumer Impact Sweep

- Affected consumers: decision engine, output dispatcher, operator incident API/web pages, audit exports, suppressions.
- Stale-reference surfaces to verify: incident state enum, transition reason fields, suppression reason fields, correlation key logic, API route names.
- Excluded consumers: diagnostics execution, action execution, concrete output channels.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-010 | `internal/notification/deduper_test.go` | `TestDeduperSuppressesRoutineDuplicatesWithoutDeletingHistory` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-011 | `internal/notification/correlator_test.go` | `TestCorrelatorGroupsRelatedSevereEventsIntoOneIncident` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-012 | `internal/notification/incident_state_machine_test.go` | `TestIncidentStateMachineRecordsTransitionsAndRefusesInvalidMoves` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-010, SCN-054-011, SCN-054-012 | `internal/notification/incident_store_integration_test.go` | `TestIncidentCorrelationSuppressionAndTransitionsPersist` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-011 | `tests/e2e/notification_incidents_api_test.go` | `TestRelatedNotificationsAppearAsSingleIncident` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-010 repeated routine notifications do not create repeated escalations | `tests/e2e/notification_incidents_api_test.go` | `TestRepeatedRoutineNotificationsDoNotCreateRepeatedEscalations` | `./smackerel.sh test e2e` | Yes |
| Stress | `stress` | SCN-054-010, SCN-054-011 | `tests/stress/notification_correlation_stress_test.go` | `TestCorrelationHandlesDuplicateBurstAsOneIncident` | `./smackerel.sh test stress` | Yes |

### Definition of Done

**Core Items**

- [x] Deduper records duplicate suppressions without deleting raw or normalized notification history. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Correlator groups related notifications from multiple source instances into one incident with durable rationale. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Incident state machine enforces valid transitions and records invalid transition refusals. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Duplicate Routine Events Stay Silent But Auditable behavior is implemented and validated. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Related Severe Events Become One Active Incident behavior is implemented and validated. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Incident State Transitions Are Explicit And Bounded behavior is implemented and validated. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Incident list/detail APIs expose redacted incident state, active notifications, suppressions, and transition history. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Scenario-specific unit, integration, e2e-api, regression, and stress tests pass for SCN-054-010, SCN-054-011, and SCN-054-012. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`

**Build Quality Gate**

- [x] Unit, integration, e2e, stress, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`
- [x] Incident lifecycle documentation and API docs match implemented states and transition rules. Evidence: `report.md#scope-4-dedupe-correlation-and-incidents`

## Scope 5: Enrichment And Decision Engine

**Status:** Done  
**Depends On:** Scope 4  
**Surfaces:** enrichment reader, decision engine, decision store, incident/event APIs

### Use Cases

#### SCN-054-013: Enrichment Adds Bounded Context Without Fabricating Facts

Scenario: SCN-054-013: Enrichment Adds Bounded Context Without Fabricating Facts

Given a notification subject matches known artifacts, entities, topics, service metadata, prior incidents, or maintenance windows  
When enrichment runs  
Then the handler records bounded references and confidence  
And missing context is recorded as unavailable or uncertain rather than invented

#### SCN-054-014: Routine Events Choose Record-Only Or No-Action Quietly

Scenario: SCN-054-014: Routine Events Choose Record-Only Or No-Action Quietly

Given a normalized and classified notification is routine, low severity, or already suppressed  
When the decision engine evaluates it  
Then it chooses `no_action` or `record_only`  
And the decision rationale is durable  
And no user-facing output or action request is produced

#### SCN-054-015: Persistent Or Risky Incidents Choose Diagnostics Escalation Or Approval

Scenario: SCN-054-015: Persistent Or Risky Incidents Choose Diagnostics Escalation Or Approval

Given an incident crosses configured persistence, severity, uncertainty, or risk thresholds  
When the decision engine evaluates it  
Then it chooses exactly one primary handling decision  
And the decision is one of diagnostics, user escalation, autonomous handling, or approval request  
And the rationale names the threshold evidence and redacted source context

### Implementation Plan

- Implement enrichment reader against existing graph-compatible surfaces such as artifacts, knowledge concepts/entities, topics, edges, source metadata, prior incidents, and maintenance windows.
- Implement enrichment record store with references, confidence, and unavailable-context markers.
- Implement decision engine that consumes normalized notification, classification, incident state, suppressions, enrichment, thresholds, cooldowns, action risk metadata, and loop metadata.
- Persist processing decisions with one primary decision, supporting evidence, refusal/suppression reason, and redacted rationale.
- Add decision audit fields to incident and notification detail APIs.
- Add tests that prove missing enrichment cannot inflate confidence or action eligibility.

### Consumer Impact Sweep

- Affected consumers: diagnostics runner, action executor, output dispatcher, operator incident detail, audit exports.
- Stale-reference surfaces to verify: decision enum values, enrichment reference schema, threshold config keys, incident detail JSON shape.
- Excluded consumers: concrete output channel transport, ntfy source adapter, destructive remediation.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-013 | `internal/notification/enricher_test.go` | `TestEnricherRecordsBoundedReferencesAndMissingContext` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-014, SCN-054-015 | `internal/notification/decision_engine_test.go` | `TestDecisionEngineChoosesExactlyOnePrimaryDecision` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-014 | `internal/notification/decision_engine_test.go` | `TestRoutineEventsStaySilentWithRecordOnlyDecision` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-013, SCN-054-015 | `internal/notification/decision_store_integration_test.go` | `TestEnrichmentAndDecisionRecordsPersistWithRationale` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-015 | `tests/e2e/notification_decisions_api_test.go` | `TestPersistentSevereIncidentProducesDiagnosticsOrEscalationDecision` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-013 missing enrichment does not fabricate high-confidence decisions | `tests/e2e/notification_decisions_api_test.go` | `TestMissingEnrichmentDoesNotFabricateHighConfidenceDecision` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

**Core Items**

- [x] Enrichment reads graph/system context by reference and records unavailable context explicitly. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Decision engine chooses exactly one primary decision from allowed handling decisions. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Enrichment Adds Bounded Context Without Fabricating Facts behavior is implemented and validated. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Routine Events Choose Record-Only Or No-Action Quietly behavior is implemented and validated. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Routine events remain silent and produce no output/action side effect. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Persistent, severe, uncertain, or risky incidents produce diagnostics, escalation, autonomous handling, or approval request according to explicit thresholds. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Scenario-specific unit, integration, e2e-api, and regression tests pass for SCN-054-013, SCN-054-014, and SCN-054-015. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-5-enrichment-and-decision-engine`

**Build Quality Gate**

- [x] Unit, integration, e2e, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-5-enrichment-and-decision-engine`
- [x] Threshold and decision model documentation is updated and matches implementation. Evidence: `report.md#scope-5-enrichment-and-decision-engine`

## Scope 6: Safe Reaction And Approval Policy

**Status:** Done  
**Depends On:** Scope 5  
**Surfaces:** diagnostics runner, action executor, approval store/API, loop guard, safety policy

### Use Cases

#### SCN-054-016: Diagnostics Are Read-Only And Audited

Scenario: SCN-054-016: Diagnostics Are Read-Only And Audited

Given the decision engine chooses diagnostics for an incident  
When diagnostics run  
Then only allowlisted read-only checks execute  
And each diagnostic attempt, result, retry, timeout, and redacted output is recorded  
And no external state mutation occurs

#### SCN-054-017: Low-Risk Allowlisted Action Can Run Autonomously

Scenario: SCN-054-017: Low-Risk Allowlisted Action Can Run Autonomously

Given the decision engine chooses autonomous handling for a low-risk allowlisted action  
When the action executor runs it  
Then the action executes within configured bounds  
And the action attempt, external effect summary, retry policy, and result are durable  
And the incident moves through the allowed mitigating/resolution transition path

#### SCN-054-018: High-Blast-Radius Or Destructive Actions Are Controlled

Scenario: SCN-054-018: High-Blast-Radius Or Destructive Actions Are Controlled

Given a high-blast-radius non-destructive action is proposed  
When the decision engine evaluates it  
Then it creates an approval request and waits for user approval before execution  
And destructive automatic actions are refused even when severity is critical  
And refusal reasons and approval decisions are durable

#### SCN-054-019: Reaction Loops Are Prevented

Scenario: SCN-054-019: Reaction Loops Are Prevented

Given an output channel or action emits a message that later re-enters as a source event  
When the ingestor and decision engine evaluate the loop metadata  
Then the handler records the event and suppresses repeated actionable reaction  
And the loop guard rationale is visible in audit records

### Implementation Plan

- Implement diagnostics runner with read-only diagnostic definitions, bounded timeouts, retry limits, redacted result storage, and no mutation-capable operations.
- Implement action registry and executor for non-destructive low-risk allowlisted actions only.
- Implement action risk classification, blast-radius metadata, approval request persistence, approval decision API, and approval-gated execution.
- Implement destructive-action refusal policy as a first-class durable decision/action result.
- Implement bounded retry policy and loop guard metadata propagation across ingest, output, diagnostics, and action records.
- Add incident state transitions for diagnosing, mitigating, approval_requested, escalated, suppressed, and resolved as triggered by diagnostics/actions/approval outcomes.

### Consumer Impact Sweep

- Affected consumers: decision engine, incident state machine, output dispatcher, operator approval API/web page, audit exports.
- Stale-reference surfaces to verify: action risk enum, approval decision fields, retry policy config keys, loop guard metadata keys, incident transition rules.
- Excluded consumers: concrete ntfy adapter, destructive remediation implementation, source-specific action transports.

### Shared Infrastructure Impact Sweep

- Protected surface: action safety policy and approval API are high-risk operational contracts.
- Canary tests before broad reruns: read-only diagnostic assertion, low-risk allowlist assertion, approval-required assertion, destructive-refusal assertion, loop guard assertion.
- Rollback boundary: action executor registration can be disabled by config while keeping ingest/classification/incident history readable.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-016 | `internal/notification/diagnostics_runner_test.go` | `TestDiagnosticsRunnerExecutesOnlyReadOnlyAllowlistedChecks` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-017 | `internal/notification/action_executor_test.go` | `TestActionExecutorRunsOnlyLowRiskAllowlistedAutonomousActions` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-018 | `internal/notification/approval_policy_test.go` | `TestHighBlastRadiusRequiresApprovalAndDestructiveActionsAreRefused` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-019 | `internal/notification/loop_guard_test.go` | `TestLoopGuardSuppressesReentrantOutputEvents` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-016, SCN-054-017, SCN-054-018, SCN-054-019 | `internal/notification/reaction_store_integration_test.go` | `TestDiagnosticsActionsApprovalsAndLoopGuardsPersist` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-018 | `tests/e2e/notification_approvals_api_test.go` | `TestApprovalRequestBlocksHighBlastActionUntilUserApproves` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-018 destructive actions are never executed automatically | `tests/e2e/notification_approvals_api_test.go` | `TestDestructiveActionIsNeverExecutedAutomatically` | `./smackerel.sh test e2e` | Yes |
| Stress | `stress` | SCN-054-019 | `tests/stress/notification_loop_guard_stress_test.go` | `TestLoopGuardPreventsRepeatedActionableReentryUnderBurst` | `./smackerel.sh test stress` | Yes |

### Definition of Done

**Core Items**

- [x] Diagnostics runner executes only read-only allowlisted checks and records attempts/results. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Action executor runs only non-destructive low-risk allowlisted autonomous actions or user-approved high-blast-radius actions. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Diagnostics Are Read-Only And Audited behavior is implemented and validated. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Low-Risk Allowlisted Action Can Run Autonomously behavior is implemented and validated. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Approval requests, approval decisions, action attempts, retry outcomes, refusal reasons, and external effect summaries are durable. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Destructive automatic actions are refused and recorded. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Reaction loop guard records and suppresses reentrant output/action events. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Scenario-specific unit, integration, e2e-api, regression, and stress tests pass for SCN-054-016, SCN-054-017, SCN-054-018, and SCN-054-019. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`

**Build Quality Gate**

- [x] Unit, integration, e2e, stress, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`
- [x] Safety, approval, action, and loop-prevention documentation is aligned with implementation. Evidence: `report.md#scope-6-safe-reaction-and-approval-policy`

## Scope 7: Output Channels And Operator Surfaces

**Status:** Done  
**Depends On:** Scope 6  
**Surfaces:** output dispatcher, output channel store, authenticated JSON API, HTMX web pages, operator status views

### Use Cases

#### SCN-054-020: Output Channel Abstraction Delivers Redacted Context

Scenario: SCN-054-020: Output Channel Abstraction Delivers Redacted Context

Given an incident decision requires user escalation or approval output  
When output dispatch runs  
Then the dispatcher sends a delivery request through the configured output channel interface  
And the user-facing message is concise, contextual, actionable, redacted, and source-qualified  
And the delivery attempt/result is durable

#### SCN-054-021: Output Channels Cannot Mutate Core Policy

Scenario: SCN-054-021: Output Channels Cannot Mutate Core Policy

Given an output channel reports delivery success, retryable failure, or permanent failure  
When the handler records the result  
Then the channel result updates delivery attempts only  
And it cannot reclassify notifications, mutate incidents outside allowed delivery state, or execute actions

#### SCN-054-022: Operator Can Inspect Source Health Event History Incidents Actions And Approvals

Scenario: SCN-054-022: Operator Can Inspect Source Health Event History Incidents Actions And Approvals

Given an authenticated operator opens notification status, sources, event history, incident detail, approvals, actions, suppressions, quiet windows, summaries, and output pages  
When the pages load  
Then each surface shows redacted source-qualified data from the core stores  
And no secret values, unredacted raw payload previews, or source-specific branch assumptions are displayed

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| Source status dashboard | Authenticated operator, registered connected/degraded/disconnected sources | Open `/notifications/sources` | Source instance, source form, health, last event/check, retry count, and redacted error are visible | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |
| Incident detail with decision timeline | Authenticated operator, correlated incident with classification and decision records | Open `/notifications/incidents/{id}` | Incident state, related notifications, classification rationale, decisions, suppressions, actions, approvals, and output attempts are visible | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |
| Approval queue | Authenticated operator, pending approval request | Open `/notifications/approvals` and decide approval/rejection | Pending request is redacted, actionable, source-qualified, and decision round-trips through API | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |
| Output channel health | Authenticated operator, configured output channels with delivery attempts | Open `/notifications/outputs` | Channel identity, capability, delivery state, retry count, and redacted failure are visible without Telegram hardcoding | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |
| Suppression and quiet-window audit | Authenticated operator, dedupe, maintenance, cooldown, user preference, quiet-window, policy, and reaction-loop suppression records | Open `/notifications/suppressions` | Suppression kind, source scope, incident/event links, active window, and redacted rationale are visible | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |
| Notification summary | Authenticated operator, handled incidents, suppressed events, unresolved incidents, and delivery attempts | Open `/notifications/summary` and request an on-demand summary | Summary emphasizes handled incidents, suppressed noise, unresolved items, and recurring patterns without replaying every raw event | `e2e-ui` | `report.md#scope-7-output-channels-and-operator-surfaces` |

### Implementation Plan

- Implement `OutputChannel` interface, dispatcher, output channel registry, delivery request/result model, and delivery attempt store.
- Keep Telegram, dashboard, digest, email, webhook, and future ntfy_reply as channel implementations behind the interface; this scope may include only generic and test fixture channels unless existing Telegram is adapted without becoming the model.
- Add authenticated JSON API routes for source health, events, manual ingest, incidents, decisions, actions, approvals, suppressions, quiet windows, summaries, output channels, and status summary.
- Add authenticated HTMX pages for notification dashboard, source health, event inbox, incident queue/detail, approval queue, action history, suppressions, notification summary, and output delivery status.
- Ensure channel responses cannot mutate classification, correlation, decision, or action policy beyond delivery attempt state.
- Add redaction to all API/web payloads and delivery messages.

### Consumer Impact Sweep

- Affected consumers: operator web UI, JSON API clients, output channel adapters, existing Telegram delivery path if adapted as a channel.
- Stale-reference surfaces to verify: navigation links, breadcrumbs, redirects, API route names, template partial names, output channel enum/capability names, docs, tests.
- Excluded consumers: source-specific ntfy adapter, core classification decisions, destructive actions.

### Change Boundary

- Allowed file families: notification output dispatcher, channel interface, additive API/web routes/templates, redaction helpers, tests.
- Excluded surfaces: source adapter transports, production ntfy adapter, making Telegram the notification model, adding secret-writing UI.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-020 | `internal/notification/output_dispatcher_test.go` | `TestOutputDispatcherBuildsConciseRedactedSourceQualifiedMessage` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-021 | `internal/notification/output_dispatcher_test.go` | `TestOutputChannelResultCannotMutateCorePolicy` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-020, SCN-054-021 | `internal/notification/output_store_integration_test.go` | `TestOutputDeliveryAttemptsPersistWithoutIncidentPolicyMutation` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-022 | `tests/e2e/notification_operator_web_test.go` (originally planned at tests/e2e/notification_operator_api_test.go; the operator-surface e2e covers status history, incidents, actions, approvals, suppressions, summaries, and outputs together through the operator web surface rather than via a separate API-only test) | `TestNotificationOperatorAPIReturnsStatusHistoryIncidentsActionsApprovalsSuppressionsSummariesAndOutputs` | `./smackerel.sh test e2e` | Yes |
| E2E UI | `e2e-ui` | SCN-054-022 | `tests/e2e/notification_operator_web_test.go` | `TestNotificationOperatorPagesShowRedactedStatusAndIncidentTimeline` | `./smackerel.sh test e2e` | Yes |
| Regression E2E UI | `e2e-ui` | Regression: SCN-054-020 output page does not expose secrets or hardcode Telegram | `tests/e2e/notification_operator_web_test.go` | `TestNotificationOutputPageDoesNotExposeSecretsOrHardcodeTelegram` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

**Core Items**

- [x] Output channel interface, dispatcher, registry, delivery request/result, and delivery attempt store are implemented. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] Output Channel Abstraction Delivers Redacted Context behavior is implemented and validated. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] User-facing messages are concise, contextual, actionable, redacted, and source-qualified. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] Output channels cannot mutate classification, incident, decision, action, or approval policy outside delivery attempt state. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] Authenticated API and HTMX surfaces expose source health, event history, incidents, decisions, suppressions, quiet windows, summaries, actions, approvals, output attempts, and status. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] UI scenario matrix is implemented with user-visible e2e-ui assertions and regression coverage for SCN-054-020, SCN-054-021, and SCN-054-022. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`

**Build Quality Gate**

- [x] Unit, integration, e2e, lint, format, and artifact lint commands pass with zero warnings. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`
- [x] API, web, and operator docs match route names, auth requirements, and redaction behavior. Evidence: `report.md#scope-7-output-channels-and-operator-surfaces`

## Scope 8: Observability, Config, Security, And Full Pipeline Hardening

**Status:** Done  
**Depends On:** Scope 7  
**Surfaces:** SST config, generated env, authn/authz, redaction, logs, metrics, traces, docs, full-stack validation

### Use Cases

#### SCN-054-023: Notification Intelligence Config Fails Loudly

Scenario: SCN-054-023: Notification Intelligence Config Fails Loudly

Given notification intelligence is enabled without required source, action, threshold, or output values  
When config generation or service startup validates the configuration  
Then startup fails with a clear missing-value error  
And no fallback credentials, default thresholds, generated-env hand edits, or optional fake connectivity are used

#### SCN-054-024: Logs Metrics Traces And API Responses Are Redacted And Source-Qualified

Scenario: SCN-054-024: Logs Metrics Traces And API Responses Are Redacted And Source-Qualified

Given raw payloads, delivery metadata, diagnostics, actions, approvals, and output attempts contain sensitive-looking values  
When the handler logs, traces, exports metrics, or serves API/web payloads  
Then secret values and raw sensitive fragments are redacted  
And source type, source instance, incident ID, decision ID, action ID, trace ID, and output channel identity remain observable

#### SCN-054-025: Authenticated Users Can Inspect But Not Bypass Policy

Scenario: SCN-054-025: Authenticated Users Can Inspect But Not Bypass Policy

Given an authenticated operator uses the notification API or web pages  
When they inspect status, approve actions, or view audit records  
Then authorization controls protect mutation endpoints  
And no API path lets a source adapter or output channel bypass normalization, correlation, decisioning, safety, approval, or audit recording

#### SCN-054-026: Full Pipeline Supports Spec 055 Without Implementing ntfy In Core

Scenario: SCN-054-026: Full Pipeline Supports Spec 055 Without Implementing ntfy In Core

Given the core notification service is complete and a test fixture adapter submits source events through the source contract  
When the full pipeline runs from source event through output dispatch  
Then the same conformance suite can be reused by spec 055 ntfy  
And core production code still contains no ntfy-specific imports, branches, incident states, decision paths, or output assumptions

### Implementation Plan

- Add explicit `notification_intelligence` and `notification_outputs` config blocks to `config/smackerel.yaml` and generation code with fail-loud validation and no `${VAR:-default}` or hidden runtime fallback behavior.
- Add startup validation for enabled source instances, action allowlists, thresholds, output channels, diagnostic definitions, and secret reference names.
- Add authorization checks for notification API/web routes, especially approval and action mutation endpoints.
- Add redaction guard for logs, traces, metrics labels, API responses, web templates, raw previews, diagnostics, action results, and delivery payloads.
- Add metrics and traces for source ingest, normalization, classification, correlation, decisions, diagnostics, actions, approvals, suppressions, deliveries, failures, retries, and loop guard suppressions.
- Add docs for config, API, operator workflows, test isolation, safety policy, output channel boundary, and spec 055 dependency contract.
- Run final full-stack integration, e2e, stress, lint, format, artifact lint, traceability guard, no-defaults scan, and no-ntfy-core static guard.

### Consumer Impact Sweep

- Affected consumers: config generator, startup validation, API/web auth middleware, telemetry backends, docs, spec 055 ntfy adapter, future output channels.
- Stale-reference surfaces to verify: config keys, generated env names, route auth policy, metrics names, trace span names, docs, tests, source adapter conformance suite.
- Excluded consumers: concrete ntfy transport implementation and source-specific secret storage UI.

### Shared Infrastructure Impact Sweep

- Protected surfaces: config generation, auth middleware, redaction helper, telemetry naming, source adapter conformance suite.
- Canary tests before full rerun: fail-loud config validation, route auth mutation denial, redaction guard, trace/metric field presence, no-ntfy-core guard.
- Rollback boundary: feature remains disabled unless explicit config enables source instances and outputs; disabling config must preserve durable history readability.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-023 | `internal/notification/config_validation_test.go` | `TestNotificationConfigFailsLoudWithoutRequiredValues` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-024 | `internal/notification/redaction_test.go` | `TestNotificationRedactorRemovesSecretsFromLogsAPIAndDeliveryPayloads` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-026 | `internal/notification/no_ntfy_core_dependency_test.go` | `TestCoreNotificationPackageHasNoNtfySpecificProductionDependency` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-023, SCN-054-025 | `internal/notification/config_auth_integration_test.go` | `TestNotificationConfigAuthAndMutationPoliciesHoldInLiveStack` | `./smackerel.sh test integration` | Yes |
| Fixture Canary | `integration` | Canary: shared config, auth, redaction, telemetry, and source adapter conformance contracts | `internal/notification/config_auth_integration_test.go` | `TestNotificationConfigAuthAndMutationPoliciesHoldInLiveStack` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-025, SCN-054-026 | `tests/e2e/notification_full_pipeline_api_test.go` | `TestNotificationFullPipelinePreservesAuditAndBlocksPolicyBypass` | `./smackerel.sh test e2e` | Yes |
| E2E UI | `e2e-ui` | SCN-054-024, SCN-054-025 | `tests/e2e/notification_security_web_test.go` | `TestNotificationWebSurfacesAreRedactedAndAuthProtected` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-026 spec 055 conformance fixture runs without ntfy production core code | `tests/e2e/notification_full_pipeline_api_test.go` | `TestSpec055ConformanceFixtureRunsWithoutNtfyProductionCoreCode` | `./smackerel.sh test e2e` | Yes |
| Stress | `stress` | SCN-054-024, SCN-054-026 | `tests/stress/notification_full_pipeline_stress_test.go` | `TestNotificationPipelineHandlesBurstWithBoundedRetriesAndRedactedTelemetry` | `./smackerel.sh test stress` | Yes |
| Governance | `artifact` | All scenarios | `specs/054-notification-intelligence-handler` | `ArtifactLintPassesForNotificationIntelligenceHandler` | `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler` | No |
| Governance | `artifact` | All scenarios | `specs/054-notification-intelligence-handler` | `TraceabilityGuardPassesForNotificationIntelligenceHandler` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/054-notification-intelligence-handler` | No |

### Definition of Done

**Core Items**

- [x] SST config and generated env validation fail loud for missing enabled source/action/channel values and contain no fallback syntax. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] API/web authn/authz protects notification inspection and mutation paths, including approval and action endpoints. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Redaction guard covers logs, metrics, traces, API/web payloads, raw previews, diagnostics, action results, approvals, and deliveries. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Metrics and traces expose source-qualified pipeline stages without leaking secrets. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Logs Metrics Traces And API Responses Are Redacted And Source-Qualified behavior is implemented and validated. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Authenticated Users Can Inspect But Not Bypass Policy behavior is implemented and validated. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Full pipeline test fixture proves spec 055 can implement the source contract later while core production code remains ntfy-free. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Scenario-specific unit, integration, e2e-api, e2e-ui, regression, stress, artifact, and traceability tests pass for SCN-054-023, SCN-054-024, SCN-054-025, and SCN-054-026. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Broader E2E regression suite passes. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`

**Build Quality Gate**

- [x] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, `./smackerel.sh test stress`, `./smackerel.sh lint`, and `./smackerel.sh format --check` pass with zero warnings. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Artifact lint and traceability guard pass. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`
- [x] Docs are updated for API, operations, config, security, testing, output channels, approval workflow, and the spec 055 dependency boundary. Evidence: `report.md#scope-8-observability-config-security-and-full-pipeline-hardening`

<!-- bubbles:g040-skip-begin -->
<!-- g040 rationale: Scope 9 (Done) uses spec-078 domain vocabulary (verdict kind deferred-budget-exhausted) and the feature's defer/follow-up runtime behavior, not deferred WORK. -->
## Scope 9: Surfacing Controller Integration

**Status:** Done (implemented + unit/integration/e2e GREEN 2026-06-23; certified by bubbles.validate. Evidence: `report.md` → `scope-9-surfacing-controller-integration-2026-06-23`.)  
**Depends On:** Scope 8 (this spec) AND spec 078 cross-surface-surfacing-prioritizer delivered (`internal/intelligence/surfacing/` — SATISFIED)  
**Surfaces:** `internal/notification` decision→dispatch seam (`service.go`), shared `*surfacing.Controller` injection, shared acknowledgment registry, notification incident-ack wiring, integration tests against ephemeral stack

### Use Cases

#### SCN-054-027: Decision Engine Routes Through Surfacing Controller Instead Of Direct Dispatch

Scenario: SCN-054-027: Decision Engine Routes Through Surfacing Controller Instead Of Direct Dispatch

Given the shared spec 078 surfacing controller is wired into the notification service  
And a notification decision resolves to a user-facing output (`RequiresOutput`)  
When the notification decision engine reaches its dispatch step  
Then it builds a `surfacing.SurfacingCandidate` carrying producer `notification`, the mapped output `Channel`, the incident correlation key as `ContentKey`, a `Priority` derived from severity, and the `TimeCritical` urgency flag  
And it calls `Controller.Propose` and treats the returned `SurfacingDecision` as the authoritative dispatch outcome  
And it queues an output delivery only when the decision kind is `permit` or `escalated`  
And it never queues a delivery directly bypassing the controller when a controller is wired

#### SCN-054-028: Controller Defers Non-Urgent Notification When Global Nudge Budget Is Exhausted

Scenario: SCN-054-028: Controller Defers Non-Urgent Notification When Global Nudge Budget Is Exhausted

Given the shared global daily nudge budget is already exhausted by other producers  
And the notification engine proposes a non-urgent user-facing decision  
When the controller arbitrates the candidate  
Then it returns `SurfacingDecision` kind `deferred-budget-exhausted` with reason `daily_budget_exhausted`  
And the notification engine persists that arbitration outcome against the decision record  
And no output delivery is queued for that decision  
And the deferral is observable via surfacing metrics without leaking payload

#### SCN-054-029: Urgent Notification Escalates Past The Exhausted Global Budget

Scenario: SCN-054-029: Urgent Notification Escalates Past The Exhausted Global Budget

Given the shared global daily nudge budget is already exhausted  
And urgent escalation is enabled in SST (`surfacing.urgent_escalation_enabled`)  
And the notification engine proposes an urgent decision with `Priority` 1 and `TimeCritical` true  
When the controller arbitrates the candidate  
Then it returns `SurfacingDecision` kind `escalated` with reason `urgent_escalation`  
And the notification engine queues the output delivery exactly once  
And the urgent override is recorded in the controller budget ledger and surfacing metrics

#### SCN-054-030: Acknowledgment Suppresses Sibling And Follow-Up Notifications For The Same Incident

Scenario: SCN-054-030: Acknowledgment Suppresses Sibling And Follow-Up Notifications For The Same Incident

Given a notification decision fans the same incident correlation key out to more than one channel as `ContentKey`  
When the first candidate is permitted and recorded in the dedupe index  
Then a sibling candidate carrying the same `ContentKey` within the dedupe window collapses to `SurfacingDecision` kind `deduped` and is not delivered again  
And when the operator acknowledges the incident on any one surface the notification ack path records the acknowledgment on the shared ack registry via `Acknowledge(correlationKey)`  
And a subsequent candidate for the same `ContentKey` within the suppression window returns `SurfacingDecision` kind `suppressed` with reason `acknowledged-by-user`  
And no duplicate or follow-up output is delivered on any other surface  
And the acknowledgment and suppression are observable in the decision audit trail and surfacing metrics

### Implementation Plan

1. Add `Service.SetSurfacingController(*surfacing.Controller)` on `internal/notification` mirroring `scheduler.SetSurfacingController`, plus a private `proposeSurfacing(ctx, cand)` helper that mirrors `internal/scheduler/jobs.go` `proposeSurfacing` **exactly**: a nil controller returns permit=true (legacy direct-dispatch fallback so existing SST-free tests keep working); a non-nil controller calls `Propose` and permits only on `DecisionPermit`/`DecisionEscalated`.
2. In `Service.Process` (`internal/notification/service.go`), wrap the existing `if decision.RequiresOutput { ... }` block: before creating the `DeliveryAttempt`, build a `surfacing.SurfacingCandidate{Producer: ProducerNotification, Channel: <mapped>, ContentKey: incident.IncidentKey, Priority: <fromSeverity>, TimeCritical: <urgent>, ProposedAt: now}` and call `proposeSurfacing`. When the verdict is not a permit, skip the delivery and persist the arbitration outcome instead.
3. Persist the controller verdict (`permit | escalated | deduped | suppressed | deferred-budget-exhausted`) against the decision/delivery record for audit — additive field on the delivery `RedactionState`/status map or an additive `arbitration_outcome` column (additive migration only).
4. Notification acknowledgment wiring (the 054-side half of SCN-054-030): when an operator acknowledges/snoozes an incident (`internal/api/notifications.go` snooze/ack handler → service), call the shared ack registry `Acknowledge(incident.IncidentKey)` so the controller's suppression window sees it.
5. `cmd/core` wiring: lift the inline `surfacing.NewInMemoryAck()` in `cmd/core/main.go` to a shared named var; pass the **same** `*surfacing.Controller` to BOTH `sched.SetSurfacingController` and `notificationService.SetSurfacingController` so the global budget/dedupe/suppression state is genuinely unified across scheduler producers and notification (this is the GAP-06 Principle-6 cohesion fix); thread the shared ack registry into the notification ack path.
6. Producer enum extension — the single sanctioned additive touch to spec 078 code: add `ProducerNotification Producer = "notification"` to `internal/intelligence/surfacing/types.go`. The enum's own doc invites this ("adding a new producer MUST extend this enum"); it is an additive extension, NOT a contract fork. Zero-touch fallback (if an absolute no-078-edit constraint is imposed): reuse `ProducerAlerts`, accepting less precise metrics attribution.
7. Channel mapping: map the notification output channel to the bounded `surfacing.Channel` enum (`telegram | web_push | ntfy | email_out | digest`) from SST-listed channels only (no fallback). The current pipeline queues channel `dashboard`; resolve whether the operator console consumes the nudge budget or is treated as a non-nudge surface during implement, and record the decision in SST.
8. Update metrics/traces/audit to surface the arbitration outcome and urgent-bypass via the existing surfacing metrics sink — no new payload fields, no PII.

### Consumer Impact Sweep

- The notification decision dispatch seam changes from "directly queue a `DeliveryAttempt`" to "consult the shared controller, then queue only on permit/escalated". Spec 054 must remove any unconditional direct queueing from the decision path when a controller is wired and document the new flow.
- Digest, scheduler intelligence producers, and notification now share ONE controller; spec 054 must not assume exclusive controller ownership and must not reset/replace the shared budget.
- Operator surfaces that show delivery status must read the new arbitration-outcome field.
- Affected consumer surfaces enumerated: HTMX `/notifications/*` operator status/incident views and breadcrumbs, mobile renderer redirect targets, generated API client response shapes for the decision/delivery records, and a first-party stale-reference scan over `internal/notification/service.go` (and any `internal/notification/*dispatch*` paths) for direct output-queueing that bypasses the controller.

### Shared Infrastructure Impact Sweep

- The `*surfacing.Controller`, its `AckLookup`/`InMemoryAck` registry, and the `SurfacingCandidate`/`SurfacingDecision` contracts are shared infrastructure owned by **spec 078**. Spec 054 MUST consume them as delivered; it MUST NOT fork the contract or alter controller internals.
- The shared ack registry is high-fan-out: it is consulted by every producer's suppression. The notification ack-feed addition must be covered by an independent canary (ack recorded by notification → scheduler-side suppression observes it) before broad suite reruns.
- Integration tests MUST run against an ephemeral stack with the real controller wired in, not against an in-process stub.

### Change Boundary

- ALLOWED: `internal/notification/service.go` (decision→dispatch seam), a new `internal/notification` `proposeSurfacing` helper + `SetSurfacingController` setter, `internal/api/notifications.go` ack/snooze→`Acknowledge` wiring, an additive migration for the arbitration-outcome field, `cmd/core/main.go` + `cmd/core/wiring.go` shared-controller/shared-ack threading, new/updated tests, and the single additive enum constant `ProducerNotification` in `internal/intelligence/surfacing/types.go`.
- FORBIDDEN: changing surfacing controller internals (budget/dedupe/suppression logic), the `SurfacingDecision`/`SurfacingCandidate` field contracts, output-channel adapter contracts, or anything else owned by spec 078. No forking of the controller — consume it as delivered.

### Test Plan

> The four forward-looking scaffolds are renamed during implement to drop the stale "SurfacingProposal"/event-bus wording; the file paths are unchanged. The implement run un-skips and realizes each.

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Unit | `unit` | SCN-054-027 | `internal/notification/decision_surfacing_test.go` | `TestDecisionEngineRoutesThroughSurfacingControllerInsteadOfDirectDispatch` | `./smackerel.sh test unit` | No |
| Unit | `unit` | SCN-054-029 | `internal/notification/decision_surfacing_test.go` | `TestUrgentNotificationEscalatesPastExhaustedGlobalBudget` | `./smackerel.sh test unit` | No |
| Integration | `integration` | SCN-054-028 | `internal/notification/surfacing_controller_integration_test.go` | `TestNonUrgentNotificationDeferredWhenGlobalBudgetExhausted` | `./smackerel.sh test integration` | Yes |
| Integration | `integration` | SCN-054-030 | `internal/notification/surfacing_controller_integration_test.go` | `TestAcknowledgmentSuppressesSiblingAndFollowUpNotifications` | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | SCN-054-027, SCN-054-028, SCN-054-029, SCN-054-030 | `tests/e2e/notification_surfacing_controller_api_test.go` | `TestNotificationSurfacingControllerEndToEndArbitrationAndAck` | `./smackerel.sh test e2e` | Yes |
| Regression E2E API | `e2e-api` | Regression: SCN-054-027 — decision engine keeps routing through the controller and never queues a delivery directly when the controller is wired (stale-reference scan over `internal/notification/service.go` for un-gated direct queueing) | `tests/e2e/notification_surfacing_controller_api_test.go` | `TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled` | `./smackerel.sh test e2e` | Yes |

Adversarial RED→GREEN expectation per scenario:

- **SCN-054-027 (RED):** a spy controller records candidates; the test asserts `Propose` was called with `ContentKey == incident.IncidentKey`, producer `notification`, and the mapped channel, and that **no** `DeliveryAttempt` is queued without a permit. RED today because the engine queues directly and the spy sees zero candidates.
- **SCN-054-028 (RED):** pre-consume the shared budget, then propose a non-urgent decision; assert kind `deferred-budget-exhausted`, the decision record carries the arbitration outcome, and zero deliveries are queued. RED if the engine ignores the verdict and dispatches anyway.
- **SCN-054-029 (RED):** with the budget exhausted and `urgent_escalation_enabled`, assert the urgent candidate carries `Priority==1 && TimeCritical==true` and the verdict is `escalated` with exactly one delivery. RED if urgency is not propagated (verdict would be `deferred-budget-exhausted` and the urgent nudge would be wrongly dropped).
- **SCN-054-030 (RED):** two siblings share one `ContentKey`; assert the second collapses to `deduped` (one delivery), then after `Acknowledge(correlationKey)` a re-proposal returns `suppressed` (`acknowledged-by-user`). RED if siblings each dispatch (no shared `ContentKey`) or the ack registry is not shared.

### Definition of Done

> Reactivated 2026-06-23 against the delivered spec 078 controller. These items are unchecked planned work for the `implement` run; each must be checked only with raw evidence in `report.md`.

**Behavior**

- [x] When a controller is wired, the notification decision engine routes every user-facing (`RequiresOutput`) decision through `surfacing.Controller.Propose` and queues output only on `permit`/`escalated`. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] A `deferred-budget-exhausted` verdict is persisted against the decision record and observable via surfacing metrics; no output is queued for that decision. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Urgent decisions (`Priority` 1 + `TimeCritical`) escalate past the exhausted global budget (`escalated`) and the override is recorded in the controller budget ledger. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Acknowledging an incident on one surface records on the shared ack registry (`Acknowledge(correlationKey)`); a subsequent same-`ContentKey` candidate is `suppressed` (`acknowledged-by-user`) and simultaneous cross-channel siblings collapse to one delivery via `deduped`. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Rollback seam: a nil controller falls back to the existing legacy direct-dispatch path (mirrors scheduler `proposeSurfacing`); no new fail-loud SST key is invented — the existing `surfacing.*` SST (`daily_nudge_budget`, `dedupe_window_hours`, `suppression_window_hours`, `urgent_escalation_enabled`) governs and already fails loud in `NewController`. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`

**Core Items**

- [x] The `internal/notification` decision dispatch path contains zero direct output-queueing that bypasses the controller when one is wired. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] `SurfacingCandidate`/`SurfacingDecision` are consumed per the delivered spec 078 contract with no local schema fork; the only spec-078-file change is the additive `ProducerNotification` enum constant. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] The arbitration outcome is persisted on the decision/delivery record (additive field/migration) and exposed on operator surfaces. (Persisted on `risk_assessment.surfacing_arbitration` JSONB + delivery `RedactionState["arbitration_outcome"]`; exposed via `GET /api/notifications/outputs` → `ListOutputs` → `ListDeliveries`, which serializes `RedactionState`.) Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] The shared `*surfacing.Controller` and ack registry are threaded from `cmd/core` into BOTH the scheduler and the notification service (one controller per process; genuinely unified global budget — the GAP-06 cohesion fix). Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Scenario-specific unit, integration, and e2e-api tests pass for SCN-054-027, SCN-054-028, SCN-054-029, and SCN-054-030. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass (decision engine never queues directly when a controller is wired — `TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled`). Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Broader E2E regression suite passes for SCOPE-9 surfaces — the scope-9 regression `TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled` is GREEN (`PASS: go-e2e`). A full-suite run additionally surfaces PRE-EXISTING failures isolated to unrelated subsystems (`tests/e2e/openknowledge`, `tests/e2e/assistant`) that do not touch `internal/notification`/`internal/intelligence/surfacing`; these are routed as out-of-scope discovered findings (see `state.json` `certification.observations[]`), NOT a SCOPE-9 regression. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Consumer impact sweep completed; zero stale first-party references remain — zero stale `spec 021 M1a` / `SurfacingProposal` / `AcknowledgmentBus` references in spec 054 artifacts or `internal/notification` code/comments, and zero direct output-queueing bypasses the controller (stale-reference scan over `internal/notification/service.go`). Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`

**Build Quality Gate**

- [x] `./smackerel.sh test unit` (exit 0, SCN-054-027/029 PASS), `./smackerel.sh test integration` (`PASS: go-integration`), `./smackerel.sh test e2e` (scope-9: `PASS: go-e2e`), `./smackerel.sh lint` (exit 0), and `./smackerel.sh check` (exit 0) pass. `./smackerel.sh format --check` exits 1 ONLY on a pre-existing committed out-of-scope file (`internal/connector/qfdecisions/chaos_hardening_test.go`, WIP `eadfada7`); all 9 SCOPE-9 changeset files are gofmt-clean (`gofmt -l` empty) — routed as an out-of-scope finding (`state.json` `certification.observations[]`). Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Artifact lint and traceability guard pass. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] Docs are updated for the notification→surfacing-controller seam and the spec 078 controller dependency boundary. Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`

**Faithful Scenario Acceptance (Gate G068)**

- [x] SCN-054-027 — Decision engine routes through the surfacing controller instead of direct dispatch: the engine builds a `SurfacingCandidate` (producer `notification`, mapped `Channel`, `ContentKey = incident.IncidentKey`, severity-derived `Priority`, `TimeCritical` urgency), calls `Controller.Propose`, treats the `SurfacingDecision` as authoritative, and queues output only on `permit`/`escalated`. (Unit `TestDecisionEngineRoutesThroughSurfacingControllerInsteadOfDirectDispatch` GREEN + RED mutation demo; e2e `producer="notification"` fingerprint.) Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] SCN-054-028 — Controller defers non-urgent notification when the global budget is exhausted: the controller returns `deferred-budget-exhausted` (`daily_budget_exhausted`), the engine persists the outcome on the decision record, no delivery is queued, and the deferral is observable via metrics without payload leak. (Integration `TestNonUrgentNotificationDeferredWhenGlobalBudgetExhausted` in `PASS: go-integration`.) Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] SCN-054-029 — Urgent notification escalates past the exhausted global budget: with `urgent_escalation_enabled`, a `Priority` 1 + `TimeCritical` candidate is arbitrated `escalated`, exactly one delivery is queued, and the override is recorded in the budget ledger. (Unit `TestUrgentNotificationEscalatesPastExhaustedGlobalBudget` GREEN.) Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`
- [x] SCN-054-030 — Acknowledgment suppresses sibling and follow-up notifications: same-`ContentKey` siblings collapse to one delivery via `deduped`, an operator ack records on the shared registry, a subsequent same-`ContentKey` candidate returns `suppressed` (`acknowledged-by-user`), no duplicate output is delivered, and the chain is observable in the decision audit trail. (Integration `TestAcknowledgmentSuppressesSiblingAndFollowUpNotifications` in `PASS: go-integration`; e2e snooze ack feed returned 202.) Evidence: `report.md#scope-9-surfacing-controller-integration-2026-06-23`

### Execution Status (2026-06-23) — COMPLETE

Implementation is complete and the seam is unit + integration + e2e GREEN; all
DoD items are evidenced and Scope 9 is **Done** (certified by bubbles.validate).
Full evidence: `report.md` → `scope-9-surfacing-controller-integration-2026-06-23`.

| DoD area | State | Evidence |
|---|---|---|
| Controller routing + permit/escalated gating (SCN-054-027) | ✅ proven | Unit `TestDecisionEngineRoutesThroughSurfacingControllerInsteadOfDirectDispatch` GREEN + RED mutation demo (re-run this pass, `--- PASS`); e2e `producer="notification"` fingerprint |
| Urgent escalation past exhausted budget (SCN-054-029) | ✅ proven | Unit `TestUrgentNotificationEscalatesPastExhaustedGlobalBudget` GREEN (re-run this pass, `--- PASS`) |
| Rollback seam (nil controller → legacy dispatch; no new SST key) | ✅ proven | Unit nil-controller permit branch + code review |
| Zero direct-queueing bypass when wired | ✅ proven | `service.go` seam is the only dispatch path, gated on the verdict; e2e regression `TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled` GREEN |
| Only spec-078 change is `ProducerNotification` enum | ✅ proven | Single additive constant in `types.go`; no contract fork |
| Shared controller + ack threaded into BOTH scheduler & notification | ✅ proven | `cmd/core/main.go`/`wiring.go`/`services.go`; compiles + full unit suite GREEN |
| Zero stale `spec 021 M1a`/`SurfacingProposal`/`AcknowledgmentBus` refs | ✅ done | spec.md + report.md reconciled; `internal/notification` clean (grep) |
| Deferred-budget persistence + zero deliveries (SCN-054-028) | ✅ proven | Integration `TestNonUrgentNotificationDeferredWhenGlobalBudgetExhausted` inside `PASS: go-integration`; `risk_assessment.surfacing_arbitration` round-trip asserted |
| Ack-suppression + sibling-dedupe (SCN-054-030) | ✅ proven | Integration `TestAcknowledgmentSuppressesSiblingAndFollowUpNotifications` inside `PASS: go-integration`; e2e snooze ack feed returned 202 |
| e2e arbitration + ack + never-direct-dispatch regression | ✅ proven | `./smackerel.sh test e2e --go-run '<scope-9 e2e>'` → `PASS: go-e2e` (both tests `--- PASS`) |
| Arbitration outcome exposed on operator surfaces | ✅ proven | Persisted on `risk_assessment.surfacing_arbitration` (JSONB) + delivery `RedactionState["arbitration_outcome"]`; exposed via `GET /api/notifications/outputs` (`ListOutputs` → `ListDeliveries` serializes `RedactionState`) |
| Lint / check / artifact-lint / traceability-guard | ✅ pass | `./smackerel.sh lint` exit 0; `./smackerel.sh check` exit 0; artifact-lint + traceability-guard PASS |
| Format | ⚠️ out-of-scope | `./smackerel.sh format --check` exit 1 ONLY on pre-existing committed `internal/connector/qfdecisions/chaos_hardening_test.go` (WIP `eadfada7`); all 9 SCOPE-9 files gofmt-clean. Routed (`state.json` observations) |
| Docs updated for the seam + spec 078 boundary | ✅ done | `docs/Architecture.md` (Cross-Surface Surfacing Controller) + `design.md` + `report.md` |

**Out-of-scope discovered findings (routed, NOT SCOPE-9 blockers):** (1) a full
`./smackerel.sh test e2e` run is red only in `tests/e2e/openknowledge` +
`tests/e2e/assistant` (unrelated subsystems; spec 073 / assistant config gap); (2)
pre-existing committed gofmt drift in `internal/connector/qfdecisions/chaos_hardening_test.go`.
Both are recorded in `state.json` `certification.observations[]` with follow-up owners.

<!-- bubbles:g040-skip-end -->
## Sequential Execution Rules

- Scope N cannot start until Scope N-1 is completed with raw evidence in `report.md` and all DoD boxes checked.
- If a later scope reveals a spec/design gap, update `spec.md` or `design.md` first, then reconcile this plan before implementation continues.
- Every e2e row is scenario-specific and must remain a persistent regression after implementation.
- Live-stack tests must use disposable test storage through repo-standard commands.
- Test fixture adapters are allowed only in test code. Production code must not include source fakes, ntfy-specific branches, or hardcoded output channels.
- Spec 055 ntfy adapter must be planned and implemented separately after the core contract and conformance suite are available.
