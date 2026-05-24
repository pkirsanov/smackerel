# Smackerel API Reference

This managed reference documents the current operator-facing HTTP API contracts that are maintained as published integration truth. The API is served by `smackerel-core` through the Chi router in `internal/api/router.go`.

## Overview

The Notification Intelligence Handler Service from spec 054 adds authenticated operator endpoints under `/api/notifications`. The handler is source-neutral: source adapters submit events through the notification service contract, the core stores raw input before normalized records, and downstream classification, incident correlation, decisions, suppressions, approvals, and output delivery stay independent of any concrete source.

Spec 055 adds the concrete ntfy source adapter. The adapter-owned API routes are still mounted under `/api/notifications/sources/{source_instance_id}/ntfy` and remain authenticated. They expose ntfy source health detail, webhook ingest, reconnect state recording, dead-letter inspection, and replay-through-source-sink controls. They do not dispatch output directly; accepted ntfy events and the first accepted dead-letter replay enter the same `SourceEventSink` path as every other notification source, while repeated replay requests return the existing accepted attempt without another source-sink side effect.

## Authentication And Authorization

All `/api/notifications/*` routes are mounted inside the authenticated `/api` group and pass through `bearerAuthMiddleware`. Callers must provide an authenticated bearer context using the same auth contract as the rest of the protected API. Development and test environments may use the shared `SMACKEREL_AUTH_TOKEN` path when per-user auth is disabled by SST; production-class deployments use the per-user bearer-auth configuration documented in [Operations.md](Operations.md#per-user-bearer-authentication-spec-044).

The notification endpoints return JSON. Request bodies that create or acknowledge notification records must use `Content-Type: application/json`.

## Endpoints Or Contracts

### Notification Intelligence Summary

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/status` | `NotificationHandlers.Status` | `StatusSummary` |
| `GET` | `/api/notifications/summary` | `NotificationHandlers.Summary` | `{"summary": StatusSummary, "message": string}` |

`StatusSummary` fields:

| Field | Type | Meaning |
|-------|------|---------|
| `source_count` | integer | Count of configured notification source instances. |
| `open_incident_count` | integer | Count of notification incidents with no `resolved_at`. |
| `pending_approvals` | integer | Count of processing decisions with `decision_type = "approval_request"`. |
| `queued_deliveries` | integer | Count of output delivery attempts with `status = "queued"`. |

### Source Health

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/sources` | `NotificationHandlers.ListSources` | `{"sources": NotificationSourceStatus[]}` |

`NotificationSourceStatus` fields:

| Field | Type | Meaning |
|-------|------|---------|
| `source_type` | string | Stable adapter type, such as `manual`, `webhook`, `queue`, or a concrete adapter type owned by its own spec. |
| `source_instance_id` | string | Unique configured source instance identity. |
| `source_form` | string | One of `stream`, `webhook`, `polling`, `queue`, `file_drop`, `api_pull`, or `manual`. |
| `enabled` | boolean | Whether the source instance is enabled in configuration or created as an authenticated manual source. |
| `config_hash` | string | Redacted configuration identity used for drift/audit checks. |
| `secret_ref_names` | string array | Secret reference names only. Secret values are never returned. |
| `redacted_metadata` | object | Non-secret source metadata safe for operator display. |
| `health_state` | string | `connected`, `disconnected`, or `degraded`. |
| `last_event_at` | timestamp or null | Last event timestamp reported by the source. |
| `last_successful_check_at` | timestamp or null | Last successful source check timestamp. |
| `retry_count` | integer | Source retry count. |
| `last_error_kind` | string | Redacted error category when health is not connected. |
| `last_error_redacted` | string | Operator-safe error text. |
| `health_observed_at` | timestamp or null | Time the latest health report was observed. |

### ntfy Source Adapter

The ntfy source adapter is a concrete source implementation for spec 055. Its routes are authenticated and require the target `source_instance_id` to resolve to a registered source with `source_type = "ntfy"`. A non-ntfy source at the same path returns `404 ntfy_source_not_found`.

| Method | Path | Handler | Success |
|--------|------|---------|---------|
| `GET` | `/api/notifications/sources/{source_instance_id}/ntfy` | `NotificationHandlers.GetNtfySourceDetail` | `200 OK` |
| `POST` | `/api/notifications/sources/{source_instance_id}/ntfy/webhook` | `NotificationHandlers.ReceiveNtfyWebhook` | `202 Accepted` |
| `POST` | `/api/notifications/sources/{source_instance_id}/ntfy/reconnect` | `NotificationHandlers.ReconnectNtfySource` | `202 Accepted` |
| `GET` | `/api/notifications/sources/{source_instance_id}/ntfy/dead-letters` | `NotificationHandlers.ListNtfyDeadLetters` | `200 OK` |
| `GET` | `/api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}` | `NotificationHandlers.GetNtfyDeadLetter` | `200 OK` |
| `POST` | `/api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}/replay` | `NotificationHandlers.ReplayNtfyDeadLetter` | `202 Accepted` |

#### ntfy Detail

`GET /api/notifications/sources/{source_instance_id}/ntfy` returns:

| Field | Type | Meaning |
|-------|------|---------|
| `source` | object | Current `notification.SourceStatus` for the ntfy source instance. This embeds the source config and latest health report. |
| `topics` | array | `ntfy.SubscriptionState` records for configured topics. Fields include `SourceInstanceID`, `Topic`, `SourceForm`, `TransportMode`, `SubscriptionState`, `LastNtfyEventID`, `LastEventAt`, `LastOpenAt`, `LastKeepaliveAt`, `LastSuccessfulCheckAt`, `LagSeconds`, `PossibleGap`, `RetryCount`, `RetryBudget`, `LastErrorKind`, `LastErrorRedacted`, `RedactionState`, `CreatedAt`, and `UpdatedAt`. |
| `last_accepted_event` | object or null | Latest normalized notification for the source, with `notification_id`, `raw_event_id`, `source_event_id`, `topic`, `raw_stored`, `normalized`, and `title_preview`. |
| `source_output_boundary` | string | Operator-visible reminder that ntfy source events enter through `SourceEventSink` and output dispatch remains core-owned. |

#### ntfy Webhook Ingest

`POST /api/notifications/sources/{source_instance_id}/ntfy/webhook` accepts raw ntfy JSON for sources configured with `source_form = "webhook"` and `transport_mode = "webhook"`. The body is read with the source's configured `max_payload_bytes` limit. Empty, malformed, oversize, unknown-topic, or unsupported payloads are rejected; malformed payloads are also recorded as ntfy dead letters when the webhook receiver is running.

Success response:

```json
{"source_instance_id":"ntfy-local-webhook","accepted":true,"transport_mode":"webhook"}
```

Important error codes:

| Code | Status | Meaning |
|------|--------|---------|
| `invalid_ntfy_source_metadata` | `400` | Source status metadata could not be reconstructed into a valid ntfy config. |
| `invalid_ntfy_webhook_source` | `400` | The source is not configured as a webhook source. |
| `invalid_ntfy_webhook_payload` | `400` or `413` | Body is empty, malformed, not configured ntfy JSON, or exceeds the configured payload ceiling. |
| `ntfy_webhook_receiver_unavailable` | `503` | The runtime webhook receiver is not registered or not running. |
| `ntfy_webhook_rejected` | `400` | The adapter rejected a valid JSON payload, for example because the topic is not configured. |

#### ntfy Reconnect

`POST /api/notifications/sources/{source_instance_id}/ntfy/reconnect` records a reconnecting topic state for each configured topic and writes a degraded source-health report through the source health store. It does not create a notification.

Response:

```json
{"source_instance_id":"ntfy-local-webhook","state":"reconnecting","created_notification":false}
```

The route requires the ntfy operational store. If the store is unavailable, the API returns `503 ntfy_operational_store_unavailable`.

#### ntfy Dead Letters

`GET /api/notifications/sources/{source_instance_id}/ntfy/dead-letters` lists adapter-owned dead-letter records for that source instance.

Query parameters:

| Parameter | Required | Meaning |
|-----------|----------|---------|
| `limit` | no | Positive integer from `1` to `200`; omitted uses the current handler value of `50`. Invalid values return `400 invalid_ntfy_dead_letter_limit`. |
| `cursor` | no | Dead-letter ID returned as `next_cursor` from the previous page. |

Response:

```json
{"dead_letters":[...],"next_cursor":""}
```

Dead-letter responses are encoded through the redacted `ntfyDeadLetterResponse` DTO, not by serializing the internal `ntfy.DeadLetterRecord`. Operator APIs never return raw payload bytes, `RawPayload`, `raw_payload_bytes`, or internal payload reference fields. Replayable records may retain raw bytes internally so replay can reconstruct a source envelope, but list/detail API responses expose only safe metadata: `id`, `source_instance_id`, `topic`, `source_event_id`, `event_type`, `observed_at`, `payload_hash`, `payload_size_bytes`, `source_raw_event_id`, `safe_payload_preview`, `cause_kind`, `cause_redacted`, `replay_eligible`, `replay_status`, `attempt_count`, `last_attempt_at`, `redaction_state`, `created_at`, and `updated_at`. Secret values, raw credential-shaped fields, and unredacted payload fragments must not appear in payload previews or causes.

`GET /api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}` returns one record as:

```json
{"dead_letter":{}}
```

Missing records return `404 ntfy_dead_letter_not_found`.

#### ntfy Dead-Letter Replay

`POST /api/notifications/sources/{source_instance_id}/ntfy/dead-letters/{dead_letter_id}/replay` requires an explicit confirmation value:

```json
{"confirmation":"replay_through_source_sink"}
```

The replay service reconstructs an eligible ntfy source envelope and calls `SourceEventSink.SubmitSourceEvent` for the first accepted replay. If the same dead letter is replayed again after a successful replay, the API returns the existing accepted attempt with `already_replayed=true` and the original raw event ID; it does not call the source sink again, create another raw event, create another normalized notification, or send output directly.

Success response:

```json
{"attempt":{},"source_output_boundary":"replay submitted only through SourceEventSink"}
```

Replay attempt records are encoded from `ntfy.ReplayAttemptRecord`. The current fields are `ID`, `DeadLetterID`, `SourceInstanceID`, `IdempotencyKey`, `ActorKind`, `ActorRef`, `Status`, `RawEventID`, `SinkStatus`, `ErrorKind`, `ErrorRedacted`, `AttemptedAt`, and optional `already_replayed`. The `already_replayed` flag is omitted on the first accepted replay and set to `true` on idempotent repeat responses.

Replay errors:

| Code | Status | Meaning |
|------|--------|---------|
| `invalid_ntfy_replay_request` | `400` | Body is missing or invalid. |
| `invalid_ntfy_replay_confirmation` | `400` | Confirmation value is not `replay_through_source_sink`. |
| `ntfy_dead_letter_not_found` | `404` | Dead-letter record does not exist for the source instance. |
| `ntfy_replay_failed` | `400` | Record is not replay eligible, cannot be mapped, or the sink rejects the replay. |

### Manual Ingest

Manual ingest is an authenticated operator path for source-neutral intake. It creates a `manual` source instance when needed and then uses the same raw-event, normalization, classification, incident, decision, and output pipeline as adapter-submitted events.

| Method | Path | Handler | Success |
|--------|------|---------|---------|
| `POST` | `/api/notifications/manual-ingest` | `NotificationHandlers.ManualIngest` | `201 Created` |

Request fields:

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `source_type` | string | yes | Operator-defined source type for the manual event. |
| `source_instance_id` | string | yes | Stable source instance identity. |
| `title` | string | no | Normalization hint for notification title. |
| `body` | string | yes | Raw text payload and normalization body hint. |
| `severity` | string | no | `info`, `low`, `medium`, `high`, `critical`, or `unknown`. |
| `subject` | string | no | Subject, component, topic, or affected entity. |
| `service` | string | no | Service/component name when known. |
| `domain` | string | no | `ops`, `finance`, `travel`, `personal`, `system`, or `unknown`. |
| `intent` | string | no | `routine`, `investigate`, `outage`, `recovery`, `mitigation`, `approval`, or `unknown`. |
| `delivery_metadata` | object | no | Non-secret delivery metadata. If absent, the handler records authenticated operator metadata. |
| `source_specific_fields` | object | no | Source-specific audit fields. Core policy consumes normalized fields, not source-specific branches. |

Success response fields:

| Field | Type | Meaning |
|-------|------|---------|
| `receipt` | object | Ingest receipt encoded from `notification.IngestReceipt`; current fields are `SourceType`, `SourceInstanceID`, `SourceForm`, `RawEventID`, `Accepted`, and `Status`. |
| `notification_id` | string | ID of the normalized notification. |
| `incident_id` | string | ID of the correlated incident. |
| `decision_id` | string | ID of the processing decision. |

### Event History

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/events` | `NotificationHandlers.ListEvents` | `{"events": NormalizedNotification[]}` |
| `GET` | `/api/notifications/events/{event_id}` | `NotificationHandlers.GetEvent` | `EventDetail` |

`events` returns the newest normalized notifications. `EventDetail` contains the current audit chain for one notification:

| Field | Meaning |
|-------|---------|
| `Notification` | Normalized notification record with raw-event reference, source identity, title/body, severity/domain/intent, tags, redaction state, and normalization state. |
| `RawEvent` | Raw source-event record, including source event identity origin (`source` or `handler_derived`), raw payload reference/hash, delivery metadata, source-specific fields, and redaction state. |
| `Classification` | Latest severity/domain/intent classification with confidence, rationale, uncertainty, and classifier version when present. |
| `Decision` | Latest processing decision with decision type, reason codes, threshold inputs, risk assessment, and rationale when present. |
| `Incident` | Correlated incident when the decision links to one. |

### Incidents

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/incidents` | `NotificationHandlers.ListIncidents` | `{"incidents": Incident[]}` |
| `GET` | `/api/notifications/incidents/{incident_id}` | `NotificationHandlers.GetIncident` | `Incident` |
| `POST` | `/api/notifications/incidents/{incident_id}/snooze` | `NotificationHandlers.SnoozeIncident` | `202 Accepted` |

Incident records expose the current incident state, severity/domain/intent summary, subject/service, risk level, first and last event timestamps, persistence count, source instance IDs, state rationale, redaction state, and resolution timestamp when present.

The snooze endpoint acknowledges the operator action with:

```json
{"status":"recorded","incident_id":"<incident_id>"}
```

### Suppressions And Quiet Windows

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/suppressions` | `NotificationHandlers.ListSuppressions` | `{"suppressions": Suppression[]}` |
| `GET` | `/api/notifications/quiet-windows` | `NotificationHandlers.ListQuietWindows` | `{"quiet_windows": []}` when no quiet-window records are active |

Suppression records include notification, incident, or source scope; suppression kind such as `dedupe`, `maintenance`, `cooldown`, `user_preference`, `reaction_loop`, `policy`, or `quiet_window`; operator-safe reason; start timestamp; optional expiry; and creation timestamp.

### Approvals

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/approvals/{approval_id}` | `NotificationHandlers.GetApproval` | `200 OK` |
| `POST` | `/api/notifications/approvals/{approval_id}/decisions` | `NotificationHandlers.RecordApprovalDecision` | `202 Accepted` |

Current approval inspection response:

```json
{"approval_id":"<approval_id>","status":"inspectable"}
```

Current approval decision acknowledgement:

```json
{"approval_id":"<approval_id>","status":"recorded"}
```

The core decision engine selects `approval_request` for high-blast-radius risk, refuses destructive automatic actions, and records the decision rationale in the notification decision audit chain.

### Output Delivery

| Method | Path | Handler | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/outputs` | `NotificationHandlers.ListOutputs` | `{"outputs": DeliveryAttempt[]}` |

Delivery attempts include decision ID, incident ID, approval request ID when present, output channel, destination reference, payload hash, redaction state, status (`queued`, `sent`, `failed`, `withheld`, or `retry_exhausted`), redacted error information, attempted timestamp, and completion timestamp.

## Error Behavior

Notification API handlers use the shared API error envelope:

```json
{
  "error": {
    "code": "notification_event_not_found",
    "message": "notification event not found"
  }
}
```

Common status codes:

| Status | When |
|--------|------|
| `400` | Invalid JSON body, missing required ingest fields, invalid source config, or pipeline validation failure. |
| `401` | Missing or invalid authenticated bearer context. |
| `404` | Event, incident, source, or ntfy dead-letter ID is not found. |
| `413` | ntfy webhook payload exceeds the configured source payload ceiling. |
| `500` | Store, status, incident, suppression, output, summary, or ntfy operational query failed. |
| `503` | ntfy webhook receiver or ntfy operational store is unavailable. |

Error messages are redacted and must not include secret values, raw bearer tokens, passwords, API keys, or unredacted source payload fragments.

## Change Notes

| Date | Change |
|------|--------|
| 2026-05-24 | Corrected spec 055 ntfy dead-letter response documentation to match the redacted operator API contract: raw payload bytes are never returned; operators receive payload hash/size, replay status, redacted cause/category, safe preview, topic/event identifiers, and timestamps only. |
| 2026-05-24 | Added spec 055 ntfy source adapter API documentation for source detail, webhook ingest, reconnect, dead-letter list/detail pagination, and replay-through-source-sink controls. |
| 2026-05-22 | Added spec 054 source-neutral Notification Intelligence Handler API: authenticated `/api/notifications/*` operator endpoints for source health, manual ingest, event history, incidents, suppressions, quiet windows, approvals, summaries, and output delivery. ntfy-specific adapter behavior remains owned by spec 055. |
