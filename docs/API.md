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

### Chrome Extension Bridge Ingestion

**Endpoint:** `POST /v1/connectors/extension/ingest`

The Chrome Extension Bridge (spec 058) streams live `chrome.bookmarks` and
`chrome.history` events into the canonical artifact pipeline. The endpoint
reuses the existing `ArtifactPublisher` so the same normalizers that handle
import-dir captures process extension events without a parallel pipeline.

| Aspect | Value |
|--------|-------|
| Auth | `Authorization: Bearer <PASETO>` (spec 044 per-user token) |
| Required scopes | `extension:bookmarks` AND `extension:history` (AND-semantics, spec 060) |
| Content-Type | `application/json` |
| Max items per batch | `extension.ingest.max_batch_items` (SST; default 256) |
| Max body bytes | `extension.ingest.max_body_bytes` (SST; default 1 MiB / 1048576) |
| Idempotency key | `metadata.client_event_id` (UUIDv7, per-item) |

**Request body** — JSON array of [`RawArtifact`](../internal/connector/connector.go)
items, each constrained to:

- `source_id` MUST equal `"browser-extension"`
- `content_type` MUST be one of `extension.ingest.accepted_content_types`
  (SST; default `["bookmark", "browser_history_visit"]`)
- `metadata.source_device_id` 1–32 chars matching `[a-z0-9-]`
- `metadata.client_event_id` UUIDv7
- `metadata.extension_version` and `metadata.privacy_filter_version` REQUIRED
- For `content_type == "bookmark"`: `metadata.bookmark_id`,
  `metadata.bookmark_folder_path`, `metadata.bookmark_event`
- For `content_type == "browser_history_visit"`:
  `metadata.dwell_estimate_seconds`, `metadata.transition_type`,
  `metadata.visit_started_at`

**Response** — HTTP 200 with one per-item outcome:

```json
{
  "items": [
    {"client_event_id": "...", "outcome": "accepted", "artifact_id": "..."},
    {"client_event_id": "...", "outcome": "deduped",  "artifact_id": "..."},
    {"client_event_id": "...", "outcome": "rejected", "error": "..."}
  ]
}
```

**Error matrix** (transport-level; whole request fails):

| Status | Code | When |
|--------|------|------|
| 401 | `unauthenticated` | Missing or invalid PASETO |
| 403 | `scope_required`  | Token lacks one or both required scopes |
| 413 | `body_too_large`  | Body exceeds `extension.ingest.max_body_bytes` |
| 422 | `batch_too_large` | Item count exceeds `extension.ingest.max_batch_items` |
| 400 | `bad_request`     | Malformed JSON or missing required metadata field |

The 403 body shape matches the spec 060 envelope:

```json
{"error": {"code": "scope_required", "message": "...", "required_scopes": ["extension:bookmarks", "extension:history"]}}
```

**Server-side dedup.** Per spec 058 §2.3, the handler computes
`SHA-256(url || \x00 || content_type || \x00 || source_device_id || \x00 || bucket)`
and resolves through `raw_ingest_dedup`. Bookmarks use bucket `0` (no
window); history visits use
`floor(captured_at_unix / dedup_window_seconds)` with the window clamped to
`[60, 86400]`. Collisions return the existing `artifact_id` with
`outcome == "deduped"` and increment `visit_count` + `last_seen_at` without
re-publishing.

### Chrome Extension Bridge Admin Devices View

**Endpoint:** `GET /v1/admin/extension/devices`

Read-only aggregation over `raw_ingest_dedup` (spec 058 design §3.2).
Returns one entry per `(owner_user_id, source_device_id)` pair that has
posted at least one extension artifact.

| Aspect | Value |
|--------|-------|
| Auth | `Authorization: Bearer <PASETO>` (spec 044) |
| Authorization | Admin caller (spec 044 callerIsAdmin gate); non-admin callers see only rows whose `owner_user_id` matches their session user id |

**Response:**

```json
{
  "devices": [
    {
      "owner_user_id": "u-alice",
      "source_device_id": "work-laptop",
      "first_seen_at": "2026-05-01T08:30:00Z",
      "last_seen_at":  "2026-05-28T11:42:13Z",
      "visit_count_30d": 412
    }
  ]
}
```

`visit_count_30d` sums `visit_count` over rows whose `last_seen_at` falls
within the trailing 30 days; older rows still contribute first/last seen
timestamps but a zero recent count.

### Corpus Read Surface — `corpus:read` (Spec 108)

Spec 108 gates the corpus-read surface on the `corpus:read` scope claim. The
gate is mounted on the **route manifest** in `internal/api/router.go`, not
inside handler bodies, so the requirement is auditable from the routing table
and a new corpus handler cannot ship ungated by omission.

> **Sixteen route groups, not eight.** The gated surface is **sixteen**
> route groups, ratified by spec 108 `spec.md` §18 decision 5 (F-108-ADJ-01)
> and shipped as a closed sixteen-value `route_group` label set in
> `internal/metrics/auth.go`. Some upstream planning prose still says
> "eight" — that text predates decision 5 and is stale. **Do not "correct"
> this table back to eight.** Eight was Tier A alone; Tier B was brought in
> scope because those endpoints compute over the same global corpus, and
> leaving them bearer-only would have made this a boundary with a documented
> hole.

The two tiers differ in documentation only. **Both carry the same grant and
the same denial shape**; the split is not a difference in authority.

#### Tier A — raw corpus retrieval (groups 1–8)

| # | Method | Path | `route_group` | Required scope |
|---|---|---|---|---|
| 1 | `POST` | `/api/search` | `search` | `corpus:read` |
| 2 | `GET` | `/api/digest` | `digest` | `corpus:read` |
| 3 | `GET` | `/api/recent` | `recent` | `corpus:read` |
| 4 | `GET` | `/api/artifact/{id}` | `artifact_detail` | `corpus:read` |
| 5 | `GET` | `/api/artifacts/{id}/domain` | `artifact_domain` | `corpus:read` |
| 6 | `GET` | `/api/export` | `export` | `corpus:read` |
| 7 | `POST` | `/api/context-for` | `context_for` | `corpus:read` |
| 8 | `GET` | `/api/knowledge/concepts`, `/concepts/{id}`, `/entities`, `/entities/{id}`, `/lint`, `/stats` | `knowledge` | `corpus:read` |

Group 8 is registered as one chi sub-router and the gate attaches to the
enclosing group, so all six knowledge endpoints inherit the requirement as a
unit and a seventh cannot be added ungated.

#### Tier B — corpus-derived Phase-5 intelligence (groups 9–16)

| # | Method | Path | `route_group` | Required scope |
|---|---|---|---|---|
| 9 | `GET` | `/api/expertise` | `expertise` | `corpus:read` |
| 10 | `GET` | `/api/learning-paths` | `learning_paths` | `corpus:read` |
| 11 | `GET` | `/api/subscriptions` | `subscriptions` | `corpus:read` |
| 12 | `GET` | `/api/serendipity` | `serendipity` | `corpus:read` |
| 13 | `GET` | `/api/content-fuel` | `content_fuel` | `corpus:read` |
| 14 | `GET` | `/api/quick-references` | `quick_references` | `corpus:read` |
| 15 | `GET` | `/api/monthly-report` | `monthly_report` | `corpus:read` |
| 16 | `GET` | `/api/seasonal-patterns` | `seasonal_patterns` | `corpus:read` |

Tier B is registered behind a `deps.IntelligenceEngine != nil` conditional.
That conditional is **enclosed by** the gated group, never the reverse.
Inverting it would register all eight endpoints **outside** the gate exactly
when the engine is configured — that is, whenever they actually serve
corpus-derived signal.

#### Routes deliberately NOT gated, and why

Omission from the gate is a decision, not an oversight. Each of these is
excluded for a stated reason:

| Route(s) | Method | Why it is not gated on `corpus:read` |
|---|---|---|
| `/api/capture` | `POST` | Write path. Ingest is not a corpus *read*; gating it would break capture for every daily user. |
| `/api/bookmarks/import` | `POST` | Write path, same reasoning. |
| `/api/assistant/turn` | `POST` | Already gated on the `assistant:turn` claim. Its corpus access is **mediated** by the assistant facade, not raw retrieval. |
| `/api/artifacts/{id}/annotations*`, `/api/annotations`, `/api/artifacts/{id}/tags/{tag}` | `POST`/`GET`/`DELETE` | Already gated on `annotation:edit`. Annotation bodies are user-authored content, not corpus content. |
| `/api/topics`, `/api/people`, `/api/places`, `/api/time`, `/api/graph/edges` | `GET` | Already gated on `knowledge-graph:read`, which daily users legitimately hold. Adding `corpus:read` here would **revoke** an existing grant rather than add a boundary. |
| `/api/internal/telegram-message-artifact` | `POST`/`GET` | Internal id↔id mapping. Returns no corpus content. |
| `/api/health`, `/readyz`, `/metrics` | `GET` | Unauthenticated by design; carry no corpus data. |

#### Denial envelope and its zero-leakage guarantee

Under ENFORCE, a principal without `corpus:read` receives the standard
`403 scope_required` envelope documented under
[Error Behavior](#403-scope_required-spec-060), with
`required` set to `["corpus:read"]`:

```text
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"scope_required","required":["corpus:read"]}
```

The zero-leakage guarantee is what makes this a boundary rather than an
existence oracle. The denial body MUST NOT contain, and does not contain:

- result counts, including a `0` or any "no results" phrasing
- artifact ids, titles, domains, or snippets
- any signal distinguishing a **resolvable** id from a **non-existent** one

Consequences that clients and integrators can rely on:

- **Identical shape across every route group.** `GET /api/artifact/{id}` for a
  non-existent id and for an existing id are byte-identical when denied.
- **No 403-versus-404 discrimination.** The gate runs **before** the handler,
  so a denied principal never learns whether the id resolves. A denied caller
  cannot enumerate the corpus by probing status codes.
- **A wildcard `*` in a token scope claim is NOT honored.** A wildcard-scoped
  token is denied exactly like an empty-scoped one.
- **No `WWW-Authenticate` challenge and no retry hint.** The remedy is an
  operator grant, which is a **token rotation** — not a client-side retry and
  not a re-auth. See
  [Operations.md → Corpus Grant Metrics And The OBSERVE → ENFORCE Rollout](Operations.md#corpus-grant-metrics-and-the-observe--enforce-rollout-spec-108).
- **`shared_token` and `bootstrap` sessions bypass the check**, per the
  documented `RequireScope` source switch. They never receive this `403`.

Under OBSERVE the gate denies **nothing**. Requests without `corpus:read` are
served normally and only increment
`smackerel_auth_corpus_grant_would_deny_total`. Client behavior is therefore
unchanged until the stage flips.

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

### 403 scope_required (Spec 060)

Routes that wrap their handler with `auth.RequireScope(...)` enforce the
PASETO `scope` claim introduced by spec 060. When the caller is authenticated
but the session's `Scopes` does NOT contain every required scope, the
middleware responds:

```text
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"scope_required","required":["<first-missing-scope>"]}
```

Semantics:

- The `required` field contains the FIRST missing required scope (not the
  full intersection diff). This keeps the label cardinality of the
  `auth_scope_rejected_total{required_scope,user_id}` counter bounded to the
  closed set declared at middleware construction time.
- The body shape is fixed (`{"error":"scope_required","required":[...]}`).
  Tooling MAY match on either the `error` string or the HTTP status; both
  are stable contract.
- This response is emitted ONLY when the request successfully authenticated
  (the bearer middleware short-circuits earlier with `401` for anonymous or
  invalid tokens). A client that receives `403 scope_required` MUST NOT
  retry without re-minting a scoped token via the operator CLI (spec 060
  Operations.md → Scoped Token Enrollment).
- For sessions whose `Source` is `SessionSourceSharedToken` or
  `SessionSourceBootstrap`, `RequireScope` short-circuits as a bypass and
  increments `auth_scope_check_bypassed_total{source}` instead. Those
  sessions never receive a `403 scope_required`.
- A misconfigured router (`RequireScope` mounted without
  `bearerAuthMiddleware` upstream) responds with `500 Internal Server Error`
  body `{"error":"middleware_misconfigured"}` and emits a structured ERROR
  log. No `auth_scope_rejected_total` increment occurs for the
  misconfiguration case — it is a wiring bug, not a scope rejection.

Metrics:

| Metric | Labels | Increments when |
|---|---|---|
| `smackerel_auth_scope_rejected_total` | `required_scope`, `user_id` | Per `403 scope_required` response. `required_scope` is the first-missing scope; cardinality bounded by the closed set of scopes wired at construction. |
| `smackerel_auth_scope_check_bypassed_total` | `source` | Per `RequireScope` invocation against a `shared_token` or `bootstrap` session. `source` is a closed set of two values. |

RequireScope endpoint wiring matrix:

| Route | Required Scope | Wired By |
|---|---|---|
| `POST /v1/connectors/extension/ingest` | `extension:bookmarks,history` | spec 058 implementation |
| The **sixteen** corpus route groups — see [Corpus Read Surface](#corpus-read-surface--corpusread-spec-108) for the per-endpoint table | `corpus:read` | spec 108 implementation (ENFORCE stage only) |

The corpus row is mounted **conditionally on the resolved stage**: under
OBSERVE the `RequireScope` half is not mounted at all and nothing is denied;
under ENFORCE it is mounted on the enclosing route group so one mount covers
all sixteen. The stage is resolved once at startup from
`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` and fails loudly when absent, empty,
or malformed.

Spec 060 ships the `RequireScope` primitive and the supporting CLI / docs
only. Endpoint wiring of `RequireScope(...)` on pre-existing routes is OUT
of scope for spec 060; consumer specs wire their own scope requirements as
part of the same change set that introduces the route (spec 058 wires the
extension ingest route above; spec 108 wires the corpus route groups).

## Change Notes

| Date | Change |
|------|--------|
| 2026-08-11 | Spec 108 — documented the corpus read surface: **sixteen** gated route groups (Tier A raw retrieval 1–8, Tier B corpus-derived Phase-5 intelligence 9–16) each requiring `corpus:read`, the routes deliberately left ungated and why, the `403 scope_required` denial envelope for `corpus:read` and its zero-leakage guarantee (no counts, no ids, no 403-vs-404 discrimination, wildcard scope not honored), and the stage-conditional `RequireScope` wiring row. The gated surface is **sixteen**, not eight — `spec.md` §18 decision 5 superseded the original eight-group scope; upstream planning prose still saying "eight" is stale. |
| 2026-06-02 | Spec 072 — added WhatsApp Business webhook endpoints (`GET`/`POST` at the configured `assistant.transports.whatsapp.webhook_path`, default `/v1/assistant/transports/whatsapp/webhook`). GET handles Meta hub-mode verification challenge using `WHATSAPP_WEBHOOK_VERIFY_TOKEN`. POST requires a valid `X-Hub-Signature-256` HMAC over the raw request body keyed by `WHATSAPP_APP_SECRET`; unsigned or tampered bodies are rejected before the assistant facade is invoked. Duplicate Meta deliveries with the same `TransportMessageID` are deduplicated and invoke the facade and capture-as-fallback path exactly once. Responses render as text, interactive buttons (≤3 choices), interactive list (>3 choices), or text fallback per the WhatsApp Cloud API render table; the `template` message family is never emitted from the assistant outbound path. The transport disables independently of Telegram and HTTP via `ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED=false`. |
| 2026-05-28 | Spec 060 — documented `403 scope_required` response shape (`{"error":"scope_required","required":[<first-missing>]}`), `auth_scope_rejected_total` / `auth_scope_check_bypassed_total` metrics, misconfigured-router `500 middleware_misconfigured` behavior, and the initial `RequireScope` endpoint wiring matrix (spec 058 extension ingest). |
| 2026-05-28 | Added spec 058 Chrome Extension Bridge ingestion endpoint (`POST /v1/connectors/extension/ingest`), per-item response shape, error matrix, and the admin devices read-only view (`GET /v1/admin/extension/devices`). |
| 2026-05-24 | Corrected spec 055 ntfy dead-letter response documentation to match the redacted operator API contract: raw payload bytes are never returned; operators receive payload hash/size, replay status, redacted cause/category, safe preview, topic/event identifiers, and timestamps only. |
| 2026-05-24 | Added spec 055 ntfy source adapter API documentation for source detail, webhook ingest, reconnect, dead-letter list/detail pagination, and replay-through-source-sink controls. |
| 2026-05-22 | Added spec 054 source-neutral Notification Intelligence Handler API: authenticated `/api/notifications/*` operator endpoints for source health, manual ingest, event history, incidents, suppressions, quiet windows, approvals, summaries, and output delivery. ntfy-specific adapter behavior remains owned by spec 055. |
