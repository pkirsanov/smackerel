# Scopes: 055 Notification Source ntfy Adapter

## Planning Status

Implementation status: behaviorally complete, with final promotion blocked only for artifact certification reconciliation. Current implementation, test, docs, stabilization, regression, security, chaos, audit, validate, and spec-review evidence is recorded in `report.md`, child bug evidence, `spec-review.md`, and `state.json`. Scopes 1-9 are active, checked, and marked Done in this planning artifact. Parent top-level and certification state intentionally remain `blocked` until validate owns final `certifiedAt`, certified phase metadata, done-mode artifact-lint, traceability guard, and state-transition guard promotion reruns after this governance pass.

Spec 054 dependency lock:

| Contract | Planning requirement |
|----------|----------------------|
| `notification.SourceAdapter` | ntfy implements lifecycle only; no classification, correlation, decisioning, action, approval, or output dispatch lives in the adapter. |
| `notification.SourceEventSink` | Every accepted ntfy message enters Smackerel through `SubmitSourceEvent`. |
| Raw-before-normalized pipeline | Raw ntfy JSON must be preserved before normalized notification creation. |
| Source health model | ntfy connection, auth, lag, retry, and dead-letter pressure reduce to connected, degraded, or disconnected with redacted errors. |
| Source status API | ntfy appears through existing source status plus adapter-owned authenticated detail controls. |
| Core boundary guard | Core notification production code remains free of ntfy-specific package dependencies and policy branches. |

## Execution Outline

### Phase Order

1. Scope 1, Config and Source Identity: add explicit ntfy source configuration, secret-reference validation, source registration, disconnected health for invalid enabled sources, and source-list UI visibility.
2. Scope 2, Event Mapping and Core Ingest: add ntfy parser, mapper, lifecycle handling, raw JSON preservation, source-specific field preservation, and source-detail proof of last accepted event.
3. Scope 3, Reconnect, Lag, and Health: add bounded stream/webhook health transitions, retry budget handling, lag/gap recording, reconnect API, and troubleshooting UI.
4. Scope 4, Dead-Letter and Replay: add dead-letter persistence, retry-to-DLQ behavior, replay-through-sink controls, and DLQ/replay UI.
5. Scope 5, Provenance, Loop, and Boundary Guards: add multi-topic and multi-instance provenance, loop metadata preservation, no-output-coupling guards, and cross-topic operator traceability.
6. Scope 6, Release Hardening and Documentation: complete cross-scope regression, stress, static guards, docs, and Bubbles validation gates without certifying implementation until execution evidence exists.
7. Scope 7, Production Webhook Receiver and Route: certify the focused production webhook route and receiver behavior already evidenced by unit and e2e API tests.
8. Scope 8, Runtime Adapter Startup from NTFY_SOURCES_JSON: certify the focused runtime startup path already evidenced by config generation, unit, and integration tests.
9. Scope 9, Focused Webhook Regression and Quality Gates: certify the focused source-neutral dispatch, stress, lint, and format evidence already recorded for the webhook/runtime slice.

### New Types And Signatures

| Planned surface | Signature or contract |
|-----------------|-----------------------|
| Adapter package | `internal/notification/source/ntfy` |
| Adapter type | `type Adapter struct { ... }` implementing `notification.SourceAdapter` |
| Config type | `type Config struct { SourceInstanceID string; SourceForm notification.SourceForm; TransportMode string; Topics []TopicConfig; Auth AuthConfig; Reconnect ReconnectConfig; Lag LagConfig; DeadLetter DeadLetterConfig; RedactedMetadata map[string]string }` |
| Event type | `type Event struct { ID string; Time *time.Time; EventType string; Topic string; Title string; Message string; Priority string; Tags []string; Click string; Icon string; Markdown string; Attachment map[string]any; Actions []map[string]any; Unknown map[string]any; Raw []byte }` |
| Mapper | `func MapEvent(ctx context.Context, cfg Config, event Event, observedAt time.Time) (notification.SourceEventEnvelope, error)` |
| Store tables | `notification_ntfy_subscription_states`, `notification_ntfy_dead_letters`, `notification_ntfy_replay_attempts` |
| Detail API | `GET /api/notifications/sources/{source_instance_id}/ntfy` |
| Webhook API | `POST /api/notifications/sources/{source_instance_id}/ntfy/webhook` |
| Reconnect API | `POST /api/notifications/sources/{source_instance_id}/ntfy/reconnect` |
| Dead-letter APIs | `GET /api/notifications/sources/{source_instance_id}/ntfy/dead-letters`, `GET /api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}` |
| Replay API | `POST /api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}/replay` |
| UI routes | `/notifications/sources`, `/notifications/sources/{source_instance_id}`, `/notifications/sources/{source_instance_id}/dead-letters`, replay confirmation, troubleshooting panel |

### Validation Checkpoints

| Checkpoint | Runs after | Purpose |
|------------|------------|---------|
| Config contract checkpoint | Scope 1 | Proves enabled sources require explicit identity, topics, endpoint, auth mode, secret references or explicit `auth_mode=none`, and redacted health. |
| Ingest checkpoint | Scope 2 | Proves a valid ntfy message creates raw and normalized records through spec 054 and lifecycle events create no notification. |
| Health checkpoint | Scope 3 | Proves degraded/disconnected transitions and recovery are based on real checks or accepted events. |
| DLQ checkpoint | Scope 4 | Proves invalid or unaccepted events dead-letter safely and replay goes only through `SourceEventSink`. |
| Boundary checkpoint | Scope 5 | Proves multiple topics/instances keep provenance, loop metadata reaches core loop guard, and adapter cannot call output channels. |
| Release checkpoint | Scope 6 | Runs unit, integration, e2e, stress, lint, format, artifact lint, traceability guard, and static scans before any validation owner can certify. |
| Webhook route checkpoint | Scope 7 | Proves the production webhook route dispatches configured receiver traffic and rejects malformed or unconfigured cases. |
| Runtime startup checkpoint | Scope 8 | Proves generated `NTFY_SOURCES_JSON` is consumed by runtime startup and malformed adapter config fails loud. |
| Focused regression checkpoint | Scope 9 | Proves concurrent webhook burst, lint, and format evidence for the focused runtime slice without promoting full feature certification. |

## Scope Ordering Rationale

The plan starts with source identity because every later behavior must attach to an unambiguous spec 054 source instance. It then delivers the first complete ingest path before adding operational failure handling. Dead-letter and replay follow reconnect because replay needs accepted source identity, bounded retry policy, and source health semantics. Provenance and boundary hardening are last among implementation scopes because they exercise the completed adapter behavior across multiple instances and guard against cross-boundary regressions. Release hardening closes documentation and validation without fabricating execution.

## Scope Inventory

| Scope | Name | Surfaces | Scenario IDs | Required tests | Status |
|-------|------|----------|--------------|----------------|--------|
| 1 | Config and Source Identity | config, source registry, source health API, source list UI | SCN-055-001, SCN-055-014 | unit, integration, e2e-api, e2e-ui, stress, static | [x] Done |
| 2 | Event Mapping and Core Ingest | adapter parser/mapper, source sink, raw/normalized pipeline, source detail UI | SCN-055-002, SCN-055-003, SCN-055-004, SCN-055-005 | unit, integration, e2e-api, e2e-ui, stress, static | [x] Done |
| 3 | Reconnect, Lag, and Health | transport lifecycle, topic state, reconnect API, troubleshooting UI | SCN-055-006, SCN-055-007 | unit, integration, e2e-api, e2e-ui, stress | [x] Done |
| 4 | Dead-Letter and Replay | DLQ store, replay attempts, DLQ APIs, replay UI | SCN-055-008, SCN-055-009 | unit, integration, e2e-api, e2e-ui, stress, static | [x] Done |
| 5 | Provenance, Loop, and Boundary Guards | multi-topic, multi-instance, loop metadata, no-output guard | SCN-055-010, SCN-055-011, SCN-055-012, SCN-055-013 | unit, integration, e2e-api, e2e-ui, stress, static | [x] Done |
| 6 | Release Hardening and Documentation | docs, operations, regression suite, artifact gates | All spec 055 scenarios | unit, integration, e2e-api, e2e-ui, stress, lint, format, artifact, traceability | [x] Done |
| 7 | Production Webhook Receiver and Route | webhook receiver, API route, malformed/adversarial route handling | SCN-055-015 | unit, e2e-api, regression | [x] Done |
| 8 | Runtime Adapter Startup from NTFY_SOURCES_JSON | config generation, runtime wiring, adapter startup, fail-loud malformed config | SCN-055-016 | unit, integration, static | [x] Done |
| 9 | Focused Webhook Regression and Quality Gates | source sink dispatch, webhook burst, lint, format | SCN-055-017 | stress, lint, format, regression | [x] Done |

## Scope 1: Config And Source Identity

**Status:** Done

Depends On: spec 054 source instance, source health, source registry, and `/api/notifications/sources` contracts are present and unchanged.

### Outcome

Operators can declare enabled ntfy source instances through explicit SST configuration. Valid instances register as `source_type=ntfy`; invalid enabled instances fail loud, accept zero events, and report disconnected redacted health. Secret values never appear in source status, logs, UI, or adapter-owned records.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-001 Enabled ntfy source requires explicit configuration
  Given spec 054 notification intelligence is enabled
  And an ntfy source instance is enabled
  When the instance is missing an explicit source instance ID, transport mode, topic set, endpoint identity, or required secret reference
  Then the adapter refuses to run that source instance
  And source health is reported as disconnected with redacted error details
  And no fallback topic, endpoint, credential, or output channel is used
```

```gherkin
Scenario: SCN-055-014 ntfy auth failure never exposes credential values
  Given an ntfy source instance uses a secret-managed credential reference
  When ntfy authentication fails
  Then source health reports a redacted authentication error category
  And logs, source status, dead-letter records, and operator APIs contain no credential value
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Config/SST | Add `notification_sources.ntfy.instances` schema to `config/smackerel.yaml` and config generator with explicit instance ID, enabled flag, source form, transport mode, endpoint ref, topic list, auth mode, secret reference names, reconnect, lag, dead-letter, and redacted metadata. |
| No-auth refinement | Refine spec 054 source config validation to allow zero secret references only when `auth_mode=none` is explicitly present in redacted metadata. Credential-backed modes still require secret reference names. |
| Source registration | Reconcile enabled ntfy source instances into `notification_source_instances` with `source_type=ntfy`, explicit `source_instance_id`, exact `source_form`, `secret_ref_names`, config hash, and redacted display metadata. |
| Health | Invalid enabled instances emit disconnected redacted health with cause categories such as `missing_endpoint`, `missing_topics`, `credential_ref_missing`, or `auth_failed`. |
| API | `GET /api/notifications/sources` includes ntfy rows through the existing source-neutral response shape; no ntfy-specific core endpoint is introduced in this scope. |
| UI | Existing Notification Sources surface shows ntfy rows, explicit state text, topic count, auth mode label, config hash label, secret reference names only, and a disabled or redacted detail state for invalid instances. |
| Auth and redaction | Secret values remain in the secret-management path. Logs, source status, redacted metadata, exports, and UI responses contain only reference names and redacted categories. |
| Observability | Add bounded config validation metrics/logs using source instance ID and error kind only. |
| Change Boundary | Allowed: config schema/generator, source config validator refinement, source registry bootstrap, source health projection, source list UI. Excluded: ntfy message mapping, DLQ, replay, Telegram/output dispatch, core classifier/correlator/decisioning. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Invalid enabled ntfy source row | Generated config has enabled source missing a required field | Open `/notifications/sources` | Row shows `ntfy`, disconnected state, redacted missing-config category, no secret values | e2e-ui | `TestNtfySourceListShowsInvalidConfigWithoutSecrets` |
| Auth failure redaction | Source has credential reference and simulated auth failure | Open source list and export redacted snapshot | UI/API show secret reference name and `auth_failed`; credential value absent | e2e-ui | `TestNtfySourceAuthFailureRedactedInUI` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S1-UNIT | Unit | `unit` | `internal/notification/source/ntfy/config_test.go` | SCN-055-001, SCN-055-014 | `TestNtfyConfigValidationRequiresExplicitEnabledInstanceFieldsAndSecretReferences` | `./smackerel.sh test unit` | No |
| TP-055-S1-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/config_integration_test.go` | SCN-055-001 | `TestNtfyInvalidEnabledInstanceRegistersDisconnectedHealthAndAcceptsNoEvents` | `./smackerel.sh test integration` | Yes |
| TP-055-S1-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-001, SCN-055-014 | `TestNotificationSourcesStatusShowsConnectedDisconnectedAndDegradedSources` | `./smackerel.sh test e2e` | Yes |
| TP-055-S1-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | SCN-055-001, SCN-055-014 | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S1-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-001 | `TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords` | `./smackerel.sh test stress` | Yes |
| TP-055-S1-STATIC | Static | `static` | `internal/notification/source/ntfy/config_test.go` | SCN-055-001, SCN-055-014 | `TestNtfyAuthFailureReportsOnlyRedactedCredentialCategories` | `./smackerel.sh test unit` | No |
| TP-055-S1-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-001 | `TestNotificationSourcesRejectDuplicateInstanceIdsBeforeProcessing` | `./smackerel.sh test e2e` | Yes |

### Scope 1 Coverage Route Notes

Historical route note: this scope formerly required `bubbles.test` evidence for source-status API redaction, disconnected-health UI, config-validation burst, no-secret exposure, and invalid-config fallback prevention. The active DoD below is now checked and mapped to recorded report evidence; no Scope 1 implementation gap is active.

### Definition of Done

- [x] SCN-055-001 enabled ntfy source explicit configuration behavior is validated: missing source instance ID, transport mode, topic set, endpoint identity, or required secret reference refuses the source, reports disconnected redacted health, accepts zero events, and uses no fallback topic, endpoint, credential, or output channel. Evidence: `report.md#implementation-evidence-2026-05-24-remaining-ntfy-health-gap-closure`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.
- [x] SCN-055-014 ntfy auth failure redaction behavior is validated: source health, source status, dead-letter records, operator APIs, and operator UI expose only redacted authentication categories and no credential values. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#current-focused-unit-and-static-evidence`.
- [x] TP-055-S1-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-STATIC passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S1-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#implementation-evidence-2026-05-24-remaining-ntfy-health-gap-closure`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Config docs and source status API docs describe ntfy source identity, auth reference handling, explicit `auth_mode=none`, redacted health, and no defaults. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Change Boundary is respected for this scope: Scope 1 owns config/source identity and redacted health only; ntfy message mapping, DLQ, and replay are owned by later scopes in this same feature, and final static/runtime guards prove no output-channel code is introduced through the config/source-identity path. Evidence: `report.md#regression-phase-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.

## Scope 2: Event Mapping And Core Ingest

**Status:** Done

Depends On: Scope 1 Done and spec 054 raw-before-normalized `SubmitSourceEvent` behavior remains unchanged.

### Outcome

A valid ntfy message event is parsed and mapped into a `SourceEventEnvelope`, accepted through the spec 054 sink, stored as raw JSON before normalization, and surfaced in operator detail. ntfy lifecycle events update source health only and never create user notifications.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-002 ntfy message enters the spec 054 raw and normalized pipeline
  Given an enabled ntfy source instance subscribes to the configured topic `self-hosted-alerts`
  When ntfy emits a valid JSON message event with id, time, topic, title, message, priority, and tags
  Then the adapter submits one `SourceEventEnvelope` with `source_type` set to `ntfy`
  And the core stores the raw ntfy JSON before normalization
  And the core creates a normalized notification linked to the raw event
```

```gherkin
Scenario: SCN-055-003 ntfy fields are preserved without becoming core policy branches
  Given an ntfy message includes priority, tags, attachment metadata, actions, and unknown safe fields
  When the adapter submits the event to the source sink
  Then recognized and safe unknown ntfy fields are preserved in delivery metadata or source-specific fields
  And mapping hints use only normalized fields understood by spec 054
  And core classification and decisioning do not branch on ntfy-only field names
```

```gherkin
Scenario: SCN-055-004 ntfy priority and tags provide hints but not final authority
  Given an ntfy event has high priority and operational tags
  When the adapter maps the event into the core source envelope
  Then source priority and tags are preserved with provenance
  And the core classifier produces the final severity, domain, and intent decision
  And the classification rationale records the signals used
```

```gherkin
Scenario: SCN-055-005 ntfy keepalive and open events update source health without creating notifications
  Given an ntfy source stream emits open or keepalive lifecycle events
  When the adapter observes those events
  Then the adapter updates connected health or last successful check time
  And no normalized user notification is created for lifecycle-only events
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Adapter package | Create `internal/notification/source/ntfy` with parser, mapper, adapter shell, and source sink integration. |
| Parser | Parse ntfy JSON message, open, keepalive, poll request, priority, tags, attachments, actions, markdown, click, icon, unknown safe fields, and raw bytes. |
| Mapper | Map message events to `SourceEventEnvelope` with `SourceType=ntfy`, configured source instance ID, source form, source event ID, event timestamp, raw JSON bytes, delivery metadata, source-specific fields, mapping hints, and loop metadata if present. |
| Core contract | Call only `SourceEventSink.SubmitSourceEvent` for accepted messages. Do not write normalized records, incidents, decisions, suppressions, approvals, actions, or outputs directly. |
| Lifecycle | Open, keepalive, and poll-request events update source/topic health and do not call `SubmitSourceEvent`. |
| UI/API | Extend ntfy source detail response to show last accepted event proof: source event ID, raw event ID, normalized notification ID, topic, `raw_stored=true`, `normalized=true`, and redacted title preview. |
| Observability | Add bounded mapper duration and event status metrics/logs/traces. |
| Change Boundary | Allowed: parser, mapper, adapter package, source detail projection, tests. Excluded: reconnect loops beyond event health updates, DLQ persistence, replay, output dispatch, core ntfy branches. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Last accepted event proof | Valid ntfy message is accepted through sink | Open ntfy source detail | Detail shows topic, source event ID, raw stored yes, normalized yes, redacted title preview | e2e-ui | `TestNtfySourceDetailShowsAcceptedRawAndNormalizedEvent` |
| Lifecycle-only health | Stream emits open/keepalive only | Open source detail and event history | Health updates; event history has no lifecycle-created normalized notification | e2e-ui | `TestNtfyLifecycleEventsUpdateHealthWithoutNotification` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S2-UNIT | Unit | `unit` | `internal/notification/source/ntfy/mapper_test.go` | SCN-055-002, SCN-055-003, SCN-055-004, SCN-055-005 | `TestNtfyMapperPreservesRawFieldsAndSeparatesLifecycleEvents` | `./smackerel.sh test unit` | No |
| TP-055-S2-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/adapter_integration_test.go` | SCN-055-002, SCN-055-005 | `TestNtfyMessageAcceptedThroughSourceSinkCreatesRawAndNormalizedRecords` | `./smackerel.sh test integration` | Yes |
| TP-055-S2-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-002, SCN-055-003, SCN-055-004, SCN-055-005 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |
| TP-055-S2-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | SCN-055-002, SCN-055-005 | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S2-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-002, SCN-055-003 | `TestNtfyWebhookBurstUsesRuntimeReceiverWithoutDuplicateRejection` | `./smackerel.sh test stress` | Yes |
| TP-055-S2-STATIC | Static | `static` | `internal/notification/no_ntfy_core_dependency_test.go` | SCN-055-003, SCN-055-004 | `TestCoreNotificationPackageHasNoNtfySpecificProductionDependency` | `./smackerel.sh test unit` | No |
| TP-055-S2-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-002, SCN-055-012 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] SCN-055-002 ntfy message raw and normalized pipeline behavior is validated: a valid message submits one `SourceEventEnvelope`, stores original raw ntfy JSON before normalization, and creates a linked normalized notification through spec 054. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-003 ntfy field preservation without core branching is validated: recognized and safe unknown fields are preserved, mapping hints use normalized fields, and core classification/decisioning do not branch on ntfy-only field names. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-004 ntfy priority and tags hint behavior is validated: source priority and tags are preserved with provenance, while the core classifier produces final severity, domain, and intent with rationale. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-005 ntfy lifecycle health behavior is validated: open and keepalive events update connected health or last successful check time without creating normalized user notifications. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-STATIC passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S2-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Source detail API/UI docs describe raw/normalized proof, lifecycle handling, source-specific fields, mapping hints, and core classifier authority. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] No core notification production file imports ntfy adapter package or branches on ntfy-only fields. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.

## Scope 3: Reconnect, Lag, And Health

**Status:** Done

Depends On: Scope 2 Done and accepted events/lifecycle checks already update source health from real observations.

### Outcome

ntfy stream/webhook connectivity failures become bounded degraded or disconnected health, lag and possible gaps are observable per topic, reconnects are operator-controllable without synthetic notifications, and connected health returns only after real source checks or accepted events.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-006 transient ntfy connection loss becomes degraded health and then recovers
  Given an ntfy source instance is connected and has accepted events
  When the ntfy connection drops and reconnect succeeds within bounded retry policy
  Then source health becomes degraded while retrying
  And retry count and last redacted error are visible through source health
  And source health returns to connected only after a successful source check or accepted event
```

```gherkin
Scenario: SCN-055-007 exhausted reconnect budget becomes disconnected health
  Given an ntfy source instance cannot reconnect after bounded retry attempts
  When the retry budget is exhausted
  Then source health becomes disconnected
  And last event time, last successful check time, retry count, and redacted error category remain inspectable
  And the adapter does not fabricate a connected state
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Store | Add `notification_ntfy_subscription_states` migration and store methods for one row per `(source_instance_id, topic)`. |
| Reconnect | Implement bounded retry budget, explicit backoff values from SST, keepalive timeout handling, source close handling, and no unbounded retry loops. |
| Lag/gap | Compute lag from latest event/open/keepalive/check timestamps, write per-topic `lag_seconds`, and set `possible_gap=true` when continuity cannot be proven after reconnect. |
| API | Add `GET /api/notifications/sources/{source_instance_id}/ntfy` detail and `POST /api/notifications/sources/{source_instance_id}/ntfy/reconnect`. |
| UI | Add ntfy source detail topic list, reconnect action, retry count, lag, possible gap indicator, redacted error, and troubleshooting panel. |
| Observability | Add reconnect, lifecycle, and lag metrics/logs/traces with bounded labels. |
| Auth | Reconnect API uses existing notification endpoint auth and source instance ownership checks. |
| Change Boundary | Allowed: topic state store, reconnect lifecycle, detail/reconnect APIs, UI health panels. Excluded: dead-letter persistence, replay, output delivery, core policy changes. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Degraded while reconnecting | Source was connected, then stream drops | Open source detail during retry | State is degraded, retry count visible, lag shown, error redacted | e2e-ui | `TestNtfySourceDetailShowsDegradedReconnectState` |
| Disconnected after retry budget | Stream cannot reconnect | Open troubleshooting panel | State is disconnected, retry budget exhausted, last check/event and error category visible | e2e-ui | `TestNtfyTroubleshootingShowsDisconnectedRetryExhaustion` |
| Reconnect control boundary | Operator clicks reconnect | Confirm health state and event history | State becomes reconnecting/degraded; no synthetic notification appears | e2e-ui | `TestNtfyReconnectControlDoesNotCreateNotification` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S3-UNIT | Unit | `unit` | `internal/notification/source/ntfy/health_test.go` | SCN-055-006, SCN-055-007 | `TestNtfyHealthTransitionsUseRealChecksAndRetryBudget` | `./smackerel.sh test unit` | No |
| TP-055-S3-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/reconnect_integration_test.go` | SCN-055-006, SCN-055-007 | `TestNtfyReconnectLagAndGapPersistInTopicState` | `./smackerel.sh test integration` | Yes |
| TP-055-S3-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-006, SCN-055-007 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` plus source status coverage mapped in recorded report evidence | `./smackerel.sh test e2e` | Yes |
| TP-055-S3-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | SCN-055-006, SCN-055-007 | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S3-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-006, SCN-055-007 | `TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords` | `./smackerel.sh test stress` | Yes |
| TP-055-S3-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-006 | `TestNotificationSourcesStatusShowsConnectedDisconnectedAndDegradedSources` | `./smackerel.sh test e2e` | Yes |

### Scope 3 Coverage Route Notes

Historical route note: this scope formerly required `bubbles.test` recovered-health proof from a real source check or accepted event. The active DoD below is now checked and mapped to recorded report evidence; no Scope 3 implementation gap is active.

### Definition of Done

- [x] SCN-055-006 transient ntfy connection loss behavior is validated: health becomes degraded while retrying, retry count and redacted error are visible, and health returns to connected only after a real source check or accepted event. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-007 exhausted reconnect budget behavior is validated: health becomes disconnected, last event/check times and retry count remain inspectable, redacted error category is preserved, and connected health is not fabricated. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S3-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Operations docs describe reconnect, lag, possible gap, retry budget exhaustion, and troubleshooting without exposing endpoints or credentials. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Reconnect and lag implementation has no unbounded retry loop, unbounded queue, unbounded sleep, or fabricated connected state. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.

## Scope 4: Dead-Letter And Replay

**Status:** Done

Depends On: Scope 3 Done and bounded retry/health semantics are available.

### Outcome

Malformed, unsupported, oversize, redaction-failed, topic-not-configured, sink-unavailable, and sink-rejected ntfy events are safely dead-lettered with provenance and redacted cause. Eligible records can be replayed only through `SourceEventSink`, with idempotency and no direct output dispatch.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-008 malformed ntfy JSON is dead-lettered without successful ingest
  Given an ntfy source instance receives malformed JSON or an unsupported event shape
  When the adapter cannot safely parse the event
  Then a dead-letter record captures source identity, topic when known, observed time, payload hash or safe payload reference, and redacted reason
  And the event is not reported as accepted by the core pipeline
  And source health reflects degraded status when dead-letter pressure crosses policy thresholds
```

```gherkin
Scenario: SCN-055-009 core sink failure is retried and then dead-lettered
  Given an ntfy event is valid
  And the spec 054 source sink is temporarily unavailable
  When adapter submission fails
  Then the adapter retries within bounded policy
  And if acceptance still fails, the event is dead-lettered with redacted cause
  And the adapter never dispatches the event directly to an output channel as a workaround
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Store | Add `notification_ntfy_dead_letters` and `notification_ntfy_replay_attempts` migrations and store methods. |
| Dead-letter | Persist source instance ID, topic, source event ID, event type, observed time, payload hash, safe payload reference, cause kind, redacted cause, replay eligibility, replay status, attempt count, and redaction state. |
| Retry | Retry sink-unavailable failures within explicit dead-letter retry budget, then dead-letter when acceptance cannot be proven. |
| Replay | Implement replay service that reconstructs eligible envelopes exactly, calls `SourceEventSink.SubmitSourceEvent`, records replay attempts, and relies on spec 054 idempotency. |
| API | Add dead-letter list/detail/replay endpoints with explicit positive `limit`, cursor support, source type checks, source instance checks, and confirmation value `replay_through_source_sink`. |
| UI | Add ntfy Dead-Letter Queue and Replay Confirmation flows with safe payload preview, replay eligibility text, and explicit source-sink-only warning. |
| Security | Redact payload previews, URL query secrets, auth headers, server error bodies, and dead-letter causes. Store raw bytes only when replay requires and redaction marks the payload safe. |
| Observability | Add dead-letter and replay metrics/logs/traces with bounded labels. |
| Change Boundary | Allowed: DLQ/replay store, API, UI, replay service. Excluded: output channel dispatch, core classifier/correlator changes, automatic attachment fetch/action execution. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Malformed event DLQ | Malformed JSON is received | Open DLQ | Queue shows redacted cause, source provenance, payload hash, not replay eligible | e2e-ui | `TestNtfyDeadLetterQueueShowsMalformedEventWithoutRawSecrets` |
| Replay-eligible sink failure | Sink-unavailable event is dead-lettered safely | Open replay confirmation | Confirmation says replay through source sink, not output delivery; idempotency key visible as hash | e2e-ui | `TestNtfyReplayConfirmationStatesSourceSinkBoundary` |
| Replay result | Confirm replay | Return to record | Replay attempt status shown; raw event link appears only after sink acceptance | e2e-ui | `TestNtfyReplayResultShowsSinkReceiptWithoutDispatchingOutput` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S4-UNIT | Unit | `unit` | `internal/notification/source/ntfy/dead_letter_test.go` | SCN-055-008, SCN-055-009 | `TestNtfyDeadLetterRedactsCausesAndComputesReplayEligibility` | `./smackerel.sh test unit` | No |
| TP-055-S4-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/replay_integration_test.go` | SCN-055-008, SCN-055-009 | `TestNtfySinkFailureRetriesDeadLettersAndReplaysThroughSourceSink` | `./smackerel.sh test integration` | Yes |
| TP-055-S4-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-008, SCN-055-009 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |
| TP-055-S4-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | SCN-055-008, SCN-055-009 | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S4-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-008, SCN-055-009 | `TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords` | `./smackerel.sh test stress` | Yes |
| TP-055-S4-STATIC | Static | `static` | `internal/notification/source/ntfy/no_output_coupling_test.go` | SCN-055-009 | `TestNtfyAdapterHasNoOutputChannelImports` | `./smackerel.sh test unit` | No |
| TP-055-S4-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-009 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] SCN-055-008 malformed or unsupported ntfy event dead-letter behavior is validated: the adapter records source identity, topic when known, observed time, payload hash or safe reference, redacted reason, no accepted ingest, and degraded pressure when thresholds are crossed. Evidence: `report.md#implementation-evidence-2026-05-24-remaining-ntfy-health-gap-closure`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`, `report.md#security-closure-verification-2026-05-24-sec-055-001-and-sec-055-002`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.
- [x] SCN-055-009 core sink failure retry and dead-letter behavior is validated: valid events retry within bounded policy, dead-letter with redacted cause after acceptance cannot be proven, and no output channel dispatch occurs as a workaround. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`.
- [x] TP-055-S4-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S4-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`.
- [x] TP-055-S4-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`.
- [x] TP-055-S4-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S4-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`.
- [x] TP-055-S4-STATIC passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S4-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-remediation-evidence-2026-05-24-sec-055-001`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#implementation-evidence-2026-05-24-remaining-ntfy-health-gap-closure`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#security-closure-verification-2026-05-24-sec-055-001-and-sec-055-002`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] API docs describe dead-letter list/detail/replay endpoints, explicit `limit`, replay confirmation, replay eligibility, idempotency, and redaction behavior. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Replay code proves it reconstructs source envelopes and calls `SubmitSourceEvent`; no output-channel code is imported or invoked. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.

## Scope 5: Provenance, Loop, And Boundary Guards

**Status:** Done

Depends On: Scope 4 Done and replay/source identity behaviors are already available.

### Outcome

Multiple ntfy topics and source instances remain distinguishable, overlapping topics and duplicate ntfy event IDs do not collapse provenance, loop metadata passes through for spec 054 loop guard evaluation, and static/runtime tests prove the adapter cannot become ntfy-to-output forwarding.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-010 multiple topics preserve exact topic provenance
  Given one ntfy source instance subscribes to multiple explicitly configured topics
  When messages arrive from different topics
  Then every envelope preserves the exact ntfy topic in delivery metadata
  And normalized notifications keep the same source instance ID
  And incident correlation may relate events without losing per-topic provenance
```

```gherkin
Scenario: SCN-055-011 multiple ntfy source instances remain distinct
  Given two ntfy source instances subscribe to overlapping topics
  When both receive an event with the same source event ID
  Then the events remain distinct by source instance ID and topic metadata
  And the core can correlate them only through explicit incident correlation logic
```

```gherkin
Scenario: SCN-055-012 ntfy adapter cannot become ntfy-to-Telegram forwarding
  Given an ntfy message qualifies for user-facing escalation
  When the adapter submits the event to the core sink
  Then the adapter performs no Telegram call and no output-channel dispatch
  And the spec 054 decision/output dispatcher decides whether and where to notify the user
```

```gherkin
Scenario: SCN-055-013 ntfy-originated loop metadata is preserved for the core loop guard
  Given an ntfy event contains metadata tying it to a recent Smackerel output or action result
  When the adapter maps the event
  Then loop metadata is included in the source envelope
  And the spec 054 loop guard decides whether to suppress actionable re-entry
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Multi-topic | Preserve exact topic in delivery metadata and source-specific fields for every envelope and topic state row. |
| Multi-instance | Include source instance ID in source event identity, store rows, replay idempotency keys, UI filters, API responses, logs, metrics, and trace attributes. |
| Loop metadata | Extract Smackerel origin IDs, incident IDs, decision IDs, loop guard keys, output trace refs, and source-origin metadata when present; pass through without adapter suppression decisions. |
| Output boundary | Add static import guard and runtime assertion tests proving ntfy adapter packages do not import Telegram, output dispatcher, dashboard delivery, digest, webhook output, or ntfy reply delivery. |
| API/UI | Add filters and detail links that let operators inspect topic-level and source-instance provenance from source detail, DLQ, event history, and incident detail. |
| Consumer Impact Sweep | Verify navigation links, API clients, route names, UI filters, docs, tests, and event history deep links use source instance ID plus topic where needed. |
| Change Boundary | Allowed: provenance fields, loop metadata mapping, static guards, filters, source/detail/event links. Excluded: changing core incident correlation logic except through existing source-neutral inputs. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Multi-topic filter | One source has two configured topics | Open source detail, select each topic, open events | Event history filters by source instance and selected topic; incident detail retains topic provenance | e2e-ui | `TestNtfyMultiTopicEventFiltersPreserveTopicProvenance` |
| Multi-instance overlap | Two instances share a topic and event ID | Open source list and details | Rows and events remain distinct by instance and topic | e2e-ui | `TestNtfyOverlappingInstancesRemainDistinctInUI` |
| Boundary copy | Message qualifies for escalation | Inspect adapter and output views | Adapter detail shows source ingest only; output attempt appears only in core output surface when policy creates one | e2e-ui | `TestNtfyAdapterViewDoesNotClaimOutputDispatch` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S5-UNIT | Unit | `unit` | `internal/notification/source/ntfy/provenance_test.go` | SCN-055-010, SCN-055-011, SCN-055-013 | `TestNtfyProvenanceAndLoopMetadataArePreservedInEnvelope` | `./smackerel.sh test unit` | No |
| TP-055-S5-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/provenance_integration_test.go` | SCN-055-010, SCN-055-011 | `TestNtfyMultiTopicAndMultiInstanceEventsDoNotCollapseIdentity` | `./smackerel.sh test integration` | Yes |
| TP-055-S5-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-010, SCN-055-011, SCN-055-012, SCN-055-013 | `TestNotificationFullPipelinePreservesAuditAndBlocksPolicyBypass` | `./smackerel.sh test e2e` | Yes |
| TP-055-S5-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | SCN-055-010, SCN-055-011, SCN-055-012 | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S5-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-010, SCN-055-011 | `TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords` | `./smackerel.sh test stress` | Yes |
| TP-055-S5-STATIC | Static | `static` | `internal/notification/source/ntfy/no_output_coupling_test.go`, `internal/notification/no_ntfy_core_dependency_test.go` | SCN-055-012 | `TestNtfyAdapterHasNoOutputChannelImports` and `TestCoreNotificationPackageHasNoNtfySpecificProductionDependency` | `./smackerel.sh test unit` | No |
| TP-055-S5-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-010, SCN-055-012 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |

### Scope 5 Coverage Route Notes

Historical route note: this scope formerly required `bubbles.test` proof for multi-topic and multi-instance provenance separation, loop metadata, and no-output boundary behavior. The active DoD below is now checked and mapped to recorded report evidence; no Scope 5 implementation gap is active.

### Definition of Done

- [x] SCN-055-010 multiple topic provenance behavior is validated: every envelope preserves exact ntfy topic, normalized notifications keep the source instance ID, and incident correlation does not erase per-topic provenance. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-011 multiple source instance identity behavior is validated: overlapping topics and duplicate source event IDs remain distinct by source instance ID and topic metadata, with correlation only through explicit core incident logic. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-012 no ntfy-to-output forwarding behavior is validated: the adapter performs no Telegram call or output-channel dispatch, and only spec 054 decision/output dispatchers can notify users. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-013 loop metadata pass-through behavior is validated: Smackerel origin, incident, decision, loop guard, or output trace metadata enters the source envelope for spec 054 loop guard evaluation. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-STATIC passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S5-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Consumer Impact Sweep evidence proves route filters, navigation, API clients, event history links, docs, and tests use source instance plus topic where required. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#docs-publication-evidence-2026-05-24`.
- [x] Static guards prove ntfy adapter has no output-channel imports and core notification production code has no ntfy branches. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.

## Scope 6: Release Hardening And Documentation

**Status:** Done

Depends On: Scope 5 Done and every scenario-specific implementation scope has passing raw evidence.

### Outcome

Spec 055 is ready for validation-owner certification: docs match the implemented behavior, every scenario has durable regression coverage, stress and static guards pass, and Bubbles artifact gates pass without marking implementation or certification complete inside the planning-owned artifacts.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-REL-001 Spec 055 remains source-neutral after all implementation scopes
  Given all ntfy adapter scopes have been implemented
  When release validation runs the complete regression and static guard suite
  Then every ntfy event path still enters through the spec 054 source sink
  And no core notification production code contains ntfy policy branches
  And no ntfy adapter code imports or dispatches output channels
```

```gherkin
Scenario: SCN-055-REL-002 Operator documentation matches implemented source behavior
  Given ntfy config, source health, detail, DLQ, replay, and troubleshooting surfaces are implemented
  When an operator follows the published docs
  Then the docs describe explicit config, secret references, source/output boundary, health states, DLQ, replay, and rollback accurately
  And the docs do not include hardcoded operator topology, credentials, or fallback config forms
```

### Implementation Plan

| Area | Planned work |
|------|--------------|
| Docs | Update `docs/API.md`, `docs/Operations.md`, `docs/Testing.md`, and any source notification docs to describe ntfy source config, status, detail, DLQ, replay, troubleshooting, metrics, rollback, and source/output boundary. |
| Regression | Ensure each SCN-055 scenario has at least one persistent e2e-api or e2e-ui regression test and at least one lower-level test. |
| Stress | Run burst, duplicate, malformed, sink-failure, reconnect-churn, multi-topic, and replay stress coverage. |
| Static scans | Run no-default config checks, no secret leakage checks, no output import checks, no core ntfy dependency checks, and no stale first-party references. |
| Bubbles gates | Run artifact lint and traceability guard for spec 055; implementation validation owner later runs state transition guard only after all scopes have evidence. |
| Rollback | Document config-first rollback: disable source instance in SST and regenerate config; historical records remain auditable. |
| Change Boundary | Allowed: docs, final test wiring, static guard hardening. Excluded: feature behavior changes unless they are required to fix failed release validation, in which case return to the owning implementation scope. |

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected visible result | Test type | Planned test |
|----------|---------------|-------|-------------------------|-----------|--------------|
| Full operator walk-through | All prior UI surfaces implemented | Source list to detail to DLQ to replay to troubleshooting | Every surface preserves source/output boundary and redacted status text | e2e-ui | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` |
| Regression suite no silent pass | E2E files exist | Run regression quality guard | Guard reports no bailout patterns in required ntfy e2e tests | static | `RegressionQualityGuardForNtfyE2E` |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Planned test title | Command | Live System |
|----|-----------|----------|---------------|------------------|--------------------|---------|-------------|
| TP-055-S6-UNIT | Unit | `unit` | `internal/notification/source/ntfy/...` plus `internal/notification/no_ntfy_core_dependency_test.go` | All SCN-055 scenarios | Full ntfy unit suite via `./smackerel.sh test unit` (covers `TestNtfyConfigValidationRequiresExplicitEnabledInstanceFieldsAndSecretReferences`, `TestNtfyAuthFailureReportsOnlyRedactedCredentialCategories`, `TestNtfyMapperPreservesRawFieldsAndSeparatesLifecycleEvents`, `TestNtfyHealthTransitionsUseRealChecksAndRetryBudget`, `TestNtfyDeadLetterRedactsCausesAndComputesReplayEligibility`, `TestNtfySinkFailureRetriesWithinBudgetBeforeDeadLetter`, `TestNtfyProvenanceAndLoopMetadataArePreservedInEnvelope`, `TestNtfyAdapterHasNoOutputChannelImports`, `TestNtfyAdapterStartRequiresTransportClientAndStopsCleanly`, `TestNtfyStartConfiguredAdaptersReadsJSONAndStartsStreamAndWebhook`, `TestNtfyStartConfiguredAdaptersFailsLoudForMalformedConfig`, `TestCoreNotificationPackageHasNoNtfySpecificProductionDependency`) | `./smackerel.sh test unit` | No |
| TP-055-S6-INTEGRATION | Integration | `integration` | `internal/notification/source/ntfy/...` plus `tests/integration/notification_ntfy_runtime_test.go` | All SCN-055 scenarios | Full ntfy integration suite via `./smackerel.sh test integration` (covers `TestNtfyInvalidEnabledInstanceRegistersDisconnectedHealthAndAcceptsNoEvents`, `TestNtfyMessageAcceptedThroughSourceSinkCreatesRawAndNormalizedRecords`, `TestNtfyReconnectLagAndGapPersistInTopicState`, `TestNtfySinkFailureRetriesDeadLettersAndReplaysThroughSourceSink`, `TestNtfyMultiTopicAndMultiInstanceEventsDoNotCollapseIdentity`, `TestNtfyRuntimeStartsConfiguredWebhookAdapterAndSubmitsObservedMessages`) | `./smackerel.sh test integration` | Yes |
| TP-055-S6-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | All SCN-055 scenarios | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `./smackerel.sh test e2e` | Yes |
| TP-055-S6-E2E-UI | E2E UI | `e2e-ui` | `tests/e2e/notification_ntfy_source_ui_test.go` | All UI scenarios | `TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting` | `./smackerel.sh test e2e` | Yes |
| TP-055-S6-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | All operational scenarios | `TestNtfyWebhookBurstUsesRuntimeReceiverWithoutDuplicateRejection` and `TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords` | `./smackerel.sh test stress` | Yes |
| TP-055-S6-LINT | Lint | `lint` | repository | All scopes | `SmackerelLintNoWarningsForNtfyAdapter` | `./smackerel.sh lint` | No |
| TP-055-S6-FORMAT | Format | `format` | repository | All scopes | `SmackerelFormatCheckForNtfyAdapter` | `./smackerel.sh format --check` | No |
| TP-055-S6-ARTIFACT | Artifact | `artifact` | `specs/055-notification-source-ntfy-adapter` | Planning artifacts | `ArtifactLintSpec055` | `bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter` | No |
| TP-055-S6-TRACE | Traceability | `artifact` | `specs/055-notification-source-ntfy-adapter` | Scenario manifest and scopes | `TraceabilityGuardSpec055` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter` | No |
| TP-055-S6-REGRESSION-GUARD | Static | `static` | `tests/e2e/notification_ntfy_source_api_test.go`, `tests/e2e/notification_ntfy_source_ui_test.go` | Regression E2E rows | `RegressionQualityGuardForNtfyE2E` | `timeout 600 bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/notification_ntfy_source_api_test.go tests/e2e/notification_ntfy_source_ui_test.go` | No |

### Definition of Done

- [x] SCN-055-REL-001 source-neutral release behavior is validated: all ntfy event paths still enter through spec 054 source sink, core notification production code has no ntfy policy branches, and ntfy adapter code has no output-channel imports or dispatches. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] SCN-055-REL-002 operator documentation behavior is validated: docs accurately describe explicit config, secret references, source/output boundary, health states, DLQ, replay, rollback, and contain no credential values or operator-specific topology. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-E2E-UI passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] TP-055-S6-LINT passes with zero warnings and raw evidence recorded in `report.md`. Evidence: `report.md#current-lint-evidence`.
- [x] TP-055-S6-FORMAT passes with raw evidence recorded in `report.md`. Evidence: `report.md#current-format-evidence`.
- [x] TP-055-S6-ARTIFACT passes with raw evidence recorded in `report.md`. Evidence: `report.md#current-artifact-lint-evidence`.
- [x] TP-055-S6-TRACE passes with raw evidence recorded in `report.md`. Evidence: `report.md#current-traceability-guard-evidence`.
- [x] TP-055-S6-REGRESSION-GUARD passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#implementation-evidence-2026-05-24-remaining-ntfy-health-gap-closure`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `report.md#chaos-closure-verification-2026-05-24-bug-chaos-20260524-001`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`.
- [x] Broader E2E regression suite passes with raw evidence recorded in `report.md`. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] Docs are updated and cite implemented API routes, config shape, health states, DLQ, replay, troubleshooting, observability, rollback, and source/output-channel boundary. Evidence: `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] No source status, logs, docs, config examples, UI text, or test fixtures expose credential values or operator-specific topology. Evidence: `report.md#security-closure-verification-2026-05-24-sec-055-001-and-sec-055-002`, `report.md#final-audit-evidence-2026-05-24-spec-055-ntfy-adapter`, `report.md#docs-publication-evidence-2026-05-24`, `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`.
- [x] State remains `in_progress` until a validation owner certifies all scopes with raw evidence; planning owner does not mark certification complete. Evidence: `report.md#test-owner-evidence-closure-2026-05-24-scope-1-6-exact-dod`, `state.json`.

## Scope 7: Production Webhook Receiver And Route

**Status:** Done

Depends On: Scope 1 source identity planning and the already-recorded implementation evidence in `report.md#implementation-gap-evidence-2026-05-24-production-webhook-and-runtime-startup`.

### Outcome

The production ntfy webhook route is mounted in the authenticated notifications API group, dispatches configured webhook traffic through the registered runtime receiver, and rejects malformed or unconfigured requests without bypassing source-neutral ingestion.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-015 production webhook route dispatches configured receiver traffic and rejects malformed cases
  Given an enabled webhook-form ntfy source instance is configured from generated runtime config
  When a valid ntfy webhook payload is posted to the production route for that source instance
  Then the route dispatches the payload to the registered ntfy receiver
  And malformed payloads or unregistered source instances are rejected without creating accepted source events
```

### Implementation Plan

| Area | Completed work |
|------|----------------|
| API route | Added `POST /api/notifications/sources/{source_instance_id}/ntfy/webhook` inside the existing bearer-authenticated notifications API group. |
| Receiver dispatch | Routed production webhook requests into the registered ntfy webhook receiver for configured source instances. |
| Malformed/adversarial handling | Rejected malformed JSON, unconfigured topics, missing receiver registration, and invalid receiver paths through focused unit/e2e coverage. |
| Boundary | Preserved the source-neutral sink boundary; the route does not dispatch Telegram/output notifications directly. |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Executed test title | Command | Live System | Evidence |
|----|-----------|----------|---------------|------------------|---------------------|---------|-------------|----------|
| TP-055-S7-UNIT | Unit | `unit` | `internal/api/notifications_ntfy_test.go` | SCN-055-015 | `TestNtfyProductionWebhookRouteDispatchesReceiverAndRejectsMalformedCases` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test unit --go --go-run 'TestNtfy|TestValidate_DBMaxConns_Missing' --verbose` | No | `report.md#focused-unit-evidence` |
| TP-055-S7-E2E-API | E2E API | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-015 | `TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test e2e --go-run 'TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload'` | Yes | `report.md#focused-e2e-api-evidence` |
| TP-055-S7-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-015 | `Regression: production ntfy webhook route rejects malformed payloads and unconfigured sources` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test e2e --go-run 'TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload'` | Yes | `report.md#focused-e2e-api-evidence` |

### Definition of Done

- [x] SCN-055-015 production webhook route behavior is validated: valid webhook payloads dispatch to the registered receiver, malformed payloads and unregistered source instances are rejected, and no accepted source event is created for rejected input. Evidence: `report.md#focused-unit-evidence`, `report.md#focused-e2e-api-evidence`.
- [x] TP-055-S7-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-unit-evidence`.
- [x] TP-055-S7-E2E-API passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] TP-055-S7-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Broader E2E regression suite passes for the focused webhook route slice with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Change Boundary is respected for this focused slice: route/receiver code is evidenced, while broader DLQ, replay, UI, and final certification remain outside this scope. Evidence: `report.md#focused-implementation-outcome`.

## Scope 8: Runtime Adapter Startup From NTFY_SOURCES_JSON

**Status:** Done

Depends On: Scope 7 Done and existing implementation evidence for generated config and runtime adapter startup.

### Outcome

Generated `NTFY_SOURCES_JSON` is consumed by runtime startup, enabled ntfy adapters are constructed and started with source-neutral notification services, webhook receiver registration is passed into API handlers, and malformed adapter config fails loud.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-016 runtime startup reads generated ntfy source config and starts configured adapters
  Given generated test environment config contains an enabled ntfy source definition
  When the core runtime starts configured ntfy adapters
  Then stream and webhook adapters are constructed from `NTFY_SOURCES_JSON`
  And malformed ntfy runtime config fails loud
  And observed webhook messages are submitted through the source-neutral sink
```

### Implementation Plan

| Area | Completed work |
|------|----------------|
| Generated config | Added generated `NTFY_SOURCES_JSON` for test runtime with explicit source instance, webhook endpoint, topics, auth mode, retry, lag, dead-letter, and config hash values. |
| Runtime wiring | Parsed generated JSON in core wiring, bootstrapped source rows, constructed enabled ntfy adapters, and started them with source-neutral notification service dependencies. |
| Webhook registry | Passed the runtime webhook registry to API handlers so production webhook routes can resolve configured receivers. |
| Fail-loud config | Added malformed runtime config coverage to prevent silent startup with invalid ntfy source JSON. |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Executed test title | Command | Live System | Evidence |
|----|-----------|----------|---------------|------------------|---------------------|---------|-------------|----------|
| TP-055-S8-CONFIG | Static | `static` | `config/generated/test.env` | SCN-055-016 | `NTFY_SOURCES_JSON is generated for test runtime` | `TERM=dumb NO_COLOR=1 ./smackerel.sh config generate --env test` and `grep -n NTFY_SOURCES_JSON ~/smackerel/config/generated/test.env` | No | `report.md#config-generation-evidence` |
| TP-055-S8-UNIT | Unit | `unit` | `internal/notification/source/ntfy/runtime.go`, `internal/notification/source/ntfy/config_json.go` | SCN-055-016 | `TestNtfyStartConfiguredAdaptersReadsJSONAndStartsStreamAndWebhook`, `TestNtfyStartConfiguredAdaptersFailsLoudForMalformedConfig` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test unit --go --go-run 'TestNtfy|TestValidate_DBMaxConns_Missing' --verbose` | No | `report.md#focused-unit-evidence` |
| TP-055-S8-INTEGRATION | Integration | `integration` | `tests/integration/notification_ntfy_runtime_test.go` | SCN-055-016 | `TestNtfyRuntimeStartsConfiguredWebhookAdapterAndSubmitsObservedMessages` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test integration --go-run 'TestNtfyRuntimeStartsConfiguredWebhookAdapterAndSubmitsObservedMessages'` | Yes | `report.md#focused-integration-evidence` |
| TP-055-S8-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-016 | `Regression: runtime configured webhook adapter remains reachable through production route` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test e2e --go-run 'TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload'` | Yes | `report.md#focused-e2e-api-evidence` |

### Definition of Done

- [x] SCN-055-016 runtime startup behavior is validated: generated `NTFY_SOURCES_JSON` is present, stream and webhook adapters start from it, malformed config fails loud, and observed webhook messages submit through the source-neutral sink. Evidence: `report.md#config-generation-evidence`, `report.md#focused-unit-evidence`, `report.md#focused-integration-evidence`.
- [x] TP-055-S8-CONFIG passes with raw evidence recorded in `report.md`. Evidence: `report.md#config-generation-evidence`.
- [x] TP-055-S8-UNIT passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-unit-evidence`.
- [x] TP-055-S8-INTEGRATION passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-integration-evidence`.
- [x] TP-055-S8-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Broader E2E regression suite passes for the focused runtime startup slice with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Runtime adapter startup remains fail-loud and source-neutral; no fallback topic, endpoint, credential, or output channel is evidenced. Evidence: `report.md#focused-implementation-outcome`.

## Scope 9: Focused Webhook Regression And Quality Gates

**Status:** Done

Depends On: Scope 8 Done and focused unit/integration/e2e validation already recorded.

### Outcome

The focused webhook/runtime slice has stress, lint, format, and code-diff evidence. Concurrent webhook delivery uses the runtime receiver without duplicate rejection, and quality gates for the touched source/config/runtime files passed without claiming full feature certification.

### Gherkin Scenarios

```gherkin
Scenario: SCN-055-017 focused webhook runtime slice passes stress and quality gates without promoting full certification
  Given the production webhook route and runtime startup path are implemented
  When focused stress, lint, format, and code-diff checks are reviewed
  Then webhook burst traffic uses the runtime receiver without duplicate rejection
  And lint and format pass for the repository command surface
  And only the evidenced focused slice is marked complete, leaving broader feature certification unpromoted
```

### Implementation Plan

| Area | Completed work |
|------|----------------|
| Stress | Exercised concurrent webhook burst through the runtime receiver without duplicate rejection. |
| Lint and format | Ran repo-standard lint and format checks for the focused implementation pass. |
| Code delta evidence | Captured git-backed path evidence for ntfy source, API, runtime wiring, config, scripts, and focused tests. |
| Certification boundary | Mapped only existing evidence to Scope 7-9 DoD and left validation-owned final status/certified phase promotion untouched. |

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Executed test title | Command | Live System | Evidence |
|----|-----------|----------|---------------|------------------|---------------------|---------|-------------|----------|
| TP-055-S9-STRESS | Stress | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-055-017 | `TestNtfyWebhookBurstUsesRuntimeReceiverWithoutDuplicateRejection` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test stress --go-run 'TestNtfyWebhookBurstUsesRuntimeReceiverWithoutDuplicateRejection'` | Yes | `report.md#focused-stress-evidence` |
| TP-055-S9-LINT | Lint | `lint` | repository | SCN-055-017 | `SmackerelLintFocusedNtfyRuntimeSlice` | `TERM=dumb NO_COLOR=1 ./smackerel.sh lint` | No | `report.md#lint-evidence` |
| TP-055-S9-FORMAT | Format | `format` | repository | SCN-055-017 | `SmackerelFormatFocusedNtfyRuntimeSlice` | `TERM=dumb NO_COLOR=1 ./smackerel.sh format --check` | No | `report.md#format-evidence` |
| TP-055-S9-REGRESSION | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-055-017 | `Regression: webhook runtime route remains source-neutral under focused e2e flow` | `TERM=dumb NO_COLOR=1 ./smackerel.sh test e2e --go-run 'TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload'` | Yes | `report.md#focused-e2e-api-evidence` |

### Definition of Done

- [x] SCN-055-017 focused webhook runtime quality behavior is validated: webhook burst traffic uses the runtime receiver without duplicate rejection, lint and format pass, and broader feature certification remains unpromoted. Evidence: `report.md#focused-stress-evidence`, `report.md#lint-evidence`, `report.md#format-evidence`, `report.md#code-diff-evidence`.
- [x] TP-055-S9-STRESS passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-stress-evidence`.
- [x] TP-055-S9-LINT passes with raw evidence recorded in `report.md`. Evidence: `report.md#lint-evidence`.
- [x] TP-055-S9-FORMAT passes with raw evidence recorded in `report.md`. Evidence: `report.md#format-evidence`.
- [x] TP-055-S9-REGRESSION passes with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Broader E2E regression suite passes for the focused webhook/runtime slice with raw evidence recorded in `report.md`. Evidence: `report.md#focused-e2e-api-evidence`.
- [x] Code Diff Evidence exists for touched runtime/source/config/test paths and excludes unrelated BUG-020/security WIP paths. Evidence: `report.md#code-diff-evidence`.

## Superseded Scopes (Do Not Execute)

No prior active scopes existed when this plan was created.
