# Smackerel API Reference

This managed reference documents the current operator-facing HTTP API contracts that are maintained as published integration truth. The API is served by `smackerel-core` through the Chi router in `internal/api/router.go`.

## Overview

The Notification Intelligence Handler Service from spec 054 adds authenticated operator endpoints under `/api/notifications`. The handler is source-neutral: source adapters submit events through the notification service contract, the core stores raw input before normalized records, and downstream classification, incident correlation, decisions, suppressions, approvals, and output delivery stay independent of any concrete source.

Spec 055 owns ntfy-specific subscription and payload mapping. The spec 054 API does not expose ntfy-only fields or ntfy-specific actions.

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
| `404` | Event or incident ID is not found. |
| `500` | Store, status, incident, suppression, output, or summary query failed. |

Error messages are redacted and must not include secret values, raw bearer tokens, passwords, API keys, or unredacted source payload fragments.

## Change Notes

| Date | Change |
|------|--------|
| 2026-05-22 | Added spec 054 source-neutral Notification Intelligence Handler API: authenticated `/api/notifications/*` operator endpoints for source health, manual ingest, event history, incidents, suppressions, quiet windows, approvals, summaries, and output delivery. ntfy-specific adapter behavior remains owned by spec 055. |
