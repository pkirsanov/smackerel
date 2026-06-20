# Design: 072 WhatsApp Business Webhook Adapter

Owner: `bubbles.design`
Workflow mode: `product-to-planning`
Status ceiling for this pass: `specs_hardened`
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 061 defines `TransportAdapter`, `AssistantMessage`, `AssistantResponse`, and `Facade.Handle`; Telegram is implemented in `internal/telegram/assistant_adapter/`, and spec 069 defines the HTTP adapter contract but has no source package yet. The closed transport vocabulary already includes `whatsapp`, and `AssistantMessage.TransportMessageID` exists for transport idempotency.

**Target State.** Add a WhatsApp Business Cloud API adapter that verifies Meta webhooks, maps verified phone identities to Smackerel users, translates inbound payloads to `AssistantMessage{Transport:"whatsapp"}`, calls the shared facade exactly once per accepted message, and renders `AssistantResponse` into WhatsApp text, interactive list, or interactive button messages.

**Patterns to Follow.** Mirror the Telegram adapter shape: thin translation/render shell, constructor-required dependencies, closed-vocabulary validation, pure renderer golden tests, OTel root span at translate, and no scenario-specific branches. Mount the webhook under Chi before facade invocation and use spec 044/060 identity and scope machinery for canonical users.

**Patterns to Avoid.** Do not alter `TransportAdapter` for WhatsApp. Do not let unsigned webhooks reach the facade. Do not store raw phone numbers in telemetry. Do not wrap normal in-window replies in WhatsApp templates. Do not disable WhatsApp by letting the adapter start with missing credentials.

**Resolved Decisions.** Package name is `internal/whatsapp/assistant_adapter`. The canonical product webhook path is `/v1/assistant/transports/whatsapp/webhook`, with the exact mounted path explicitly supplied by SST. Phone-number to user mapping uses a generic `assistant_transport_identities` table keyed by `(transport, external_subject_hash)`. V1 supports inbound text and interactive reply payloads, outbound text/list/buttons, and no outbound media.

**Open Questions.** Operator provisioning UX for phone-number mapping can be CLI or admin HTTP in a later planning decision. This design defines the storage and adapter contract either surface must call.

## Overview

WhatsApp is the second concrete human-messaging `TransportAdapter` after Telegram. It proves the spec 061 facade contract is not Telegram-specific while preserving capture-as-fallback, disambiguation, confirmation, reset, and idempotency semantics.

The adapter has three boundaries:

| Boundary | Contract |
|----------|----------|
| Inbound webhook | Meta signature verification and payload parsing before facade invocation |
| Identity mapping | WhatsApp sender phone id to canonical Smackerel `user_id` |
| Outbound renderer | Exhaustive `AssistantResponse` to WhatsApp message-family mapping |

## Architecture

```text
Meta WhatsApp webhook
  -> GET verification or POST message delivery
  -> signature verification / verify-token check
  -> idempotency check by WhatsApp message id
  -> identity lookup: transport=whatsapp + external_subject_hash
  -> Translate -> AssistantMessage
  -> shared Facade.Handle
  -> Render -> WhatsApp Cloud API send
  -> metrics / IntentTrace / structured logs
```

Component map:

| Component | Location | Responsibility |
|-----------|----------|----------------|
| Adapter | `internal/whatsapp/assistant_adapter` | Implements `contracts.TransportAdapter` |
| Webhook handlers | `internal/api` route binding to adapter | GET verify and POST delivery |
| Identity store | `internal/assistant/transportidentity` | Generic transport identity mapping |
| Renderer | `internal/whatsapp/assistant_adapter/render.go` | Pure response-to-message mapping |
| Cloud API client (deferred) | `CloudClient` interface in `internal/whatsapp/assistant_adapter`; concrete `internal/whatsapp/cloudapi` deferred to a future increment | Signed outbound HTTP calls to Meta (v1 ships the injection seam only) |
| Config loader | `internal/config/assistant.go` extension | Fail-loud WhatsApp SST validation |

## Capability Foundation

The reusable foundation is the existing `TransportAdapter` plus a new generic `TransportIdentityRegistry` for external human-message transports.

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| `contracts.TransportAdapter` | Translate, render, identity, start/stop | Telegram, HTTP, WhatsApp, later transports |
| `TransportIdentityRegistry` | Map external transport subject to canonical `user_id` | WhatsApp now; Signal/Matrix/RCS later |
| `AssistantResponseRenderer` | Pure transport renderer from response shape | WhatsApp renderer and golden tests |
| `WebhookVerifier` | Verify upstream webhook authenticity before translation | WhatsApp route |

Foundation-owned policies:

- Adapters do not choose scenarios or tools.
- `Facade.Handle` is invoked at most once per accepted transport message id.
- Identity mapping must resolve before facade invocation.
- CaptureRoute is honored before rendering the acknowledgement.
- Unknown render shapes produce a text rendering, not a dropped response.

### Variation Axes

| Axis | Values | Foundation-Owned? |
|------|--------|-------------------|
| Provider protocol | Telegram Bot API, HTTP JSON, WhatsApp Cloud API | No |
| Identity subject | chat id, authenticated web user, phone-number hash | Yes |
| Render family | text, list, buttons, template-required refusal | Partly |
| Enablement | enabled, disabled | Yes, via SST validation |
| Retry/idempotency | upstream retry, outbound retry, duplicate message id | Yes |

## Concrete Implementations

### WhatsApp Adapter

Package: `internal/whatsapp/assistant_adapter`.

Constructor options are all required when WhatsApp is enabled:

```go
type Options struct {
    CloudClient CloudClient
    IdentityRegistry TransportIdentityRegistry
    Capture CaptureFn
    Verify WebhookVerifier
    MaxTextChars int
    RateLimitPerUserPerMinute int
    Tracer *tracing.Tracer
}
```

The adapter methods behave as follows:

| Method | Behavior |
|--------|----------|
| `Name()` | returns `whatsapp` |
| `Identity()` | extracts sender phone id, HMACs it, resolves active user mapping |
| `Translate()` | converts WhatsApp text/interactive replies to `AssistantMessage` |
| `Render()` | sends WhatsApp text/list/button messages through Cloud API |
| `Start()` | binds facade and registers ready state only after config and webhook verifier are valid |
| `Stop()` | drains in-flight requests; no new webhook accepts after stop begins |

### WhatsApp Cloud API Client (deferred to a future increment)

> **Design reconciliation (2026-06-16):** This section was reconciled to the
> as-shipped v1. v1 ships the `CloudClient` interface plus its `cmd/core`
> injection seam; the concrete `internal/whatsapp/cloudapi` client is deferred
> to a future increment. Earlier wording presented `cloudapi` as a concrete
> shipped package.

v1 ships only the `CloudClient` interface — the narrow outbound seam declared in
`internal/whatsapp/assistant_adapter/adapter.go` — plus its `cmd/core` injection
point. Until a concrete client is wired, the adapter's phone-targeted render path
fails loud (`whatsapp_adapter: RenderToPhone called without configured
CloudClient`). The concrete `internal/whatsapp/cloudapi` package is a future
increment and does not yet exist on disk.

When built, the client will own outbound API URL construction and authorization
headers. It will receive the Meta API base URL and API version from required SST
keys, and will never build those values from hardcoded runtime fallbacks.

### Transport Identity Registry

Package: `internal/assistant/transportidentity`.

The registry is transport-neutral. WhatsApp uses `external_subject_type='phone_e164_hmac'` and never stores raw phone numbers.

## Data Model

### `assistant_transport_identities`

```sql
CREATE TABLE IF NOT EXISTS assistant_transport_identities (
    transport               TEXT        NOT NULL CHECK (transport IN ('telegram', 'whatsapp', 'web', 'mobile')),
    external_subject_hash   TEXT        NOT NULL,
    external_subject_type   TEXT        NOT NULL,
    user_id                 TEXT        NOT NULL REFERENCES auth_users(user_id),
    status                  TEXT        NOT NULL CHECK (status IN ('active', 'disabled')),
    verified_at             TIMESTAMPTZ NOT NULL,
    metadata                JSONB       NOT NULL,
    schema_version          INTEGER     NOT NULL,
    PRIMARY KEY (transport, external_subject_hash)
);

CREATE INDEX IF NOT EXISTS idx_assistant_transport_identities_user
    ON assistant_transport_identities (user_id, transport);
```

For WhatsApp, `external_subject_hash = HMAC-SHA256(assistant.transports.whatsapp.identity_hash_key, normalized_e164_phone)`. Raw phone numbers may appear in process memory during verification but are not persisted in this table, logs, metrics, or traces.

### Idempotency

Use the facade/conversation idempotency key `(user_id, transport, transport_message_id)` from spec 069. If implementation discovers that `assistant_conversations` does not yet persist per-message idempotency, planning must add the smallest durable idempotency table without changing the `AssistantMessage` contract.

## API/Contracts

### Webhook Verification

Endpoint: `GET /v1/assistant/transports/whatsapp/webhook`.

This path is the product contract; SST must explicitly provide the mounted path.

Query validation:

| Query | Required | Validation |
|-------|----------|------------|
| `hub.mode` | yes | must equal `subscribe` |
| `hub.verify_token` | yes | constant-time compare with `assistant.transports.whatsapp.webhook_verify_token` |
| `hub.challenge` | yes | echoed only after token match |

Success: `200 text/plain` with challenge. Failure: `403` with no facade invocation.

### Inbound Delivery

Endpoint: `POST /v1/assistant/transports/whatsapp/webhook`.

Headers:

| Header | Required | Validation |
|--------|----------|------------|
| `X-Hub-Signature-256` | yes | HMAC-SHA256 over raw body using app secret |
| `Content-Type` | yes | `application/json` |

Accepted inbound shapes:

| WhatsApp payload | AssistantMessage |
|------------------|------------------|
| text message | `KindText`, `Text=body` |
| interactive button reply | `KindConfirm` or `KindDisambiguation` from payload prefix |
| interactive list reply | `KindDisambiguation` from row id |
| unsupported media | `KindText` with safe unsupported-media text only if source policy allows; otherwise reject before facade |

Error responses:

| Condition | Status | Facade Invoked |
|-----------|--------|----------------|
| Missing or invalid signature | 403 | no |
| Unknown phone mapping | 403 | no |
| Duplicate message id | 200 | no new invocation |
| Unsupported schema | 400 | no |
| Facade/render failure | 500 | yes when facade started |

### Outbound Render Mapping

| AssistantResponse Shape | WhatsApp Message |
|-------------------------|------------------|
| plain body + sources | text message with source block within configured cap |
| disambiguation 1-3 choices | interactive buttons |
| disambiguation 4-10 choices | interactive list |
| disambiguation >10 choices | text body with numbered choices and typed reply instruction |
| confirm card | two interactive buttons with positive/negative labels |
| reset acknowledgement | text message |
| capture acknowledgement | canonical saved-as-idea text |
| unknown response shape | text rendering of `Body`; if body empty, fail render with observable error |

Template messages are used only when WhatsApp's customer-service window requires a template and a named template is explicitly selected by the operator-runbook flow. The renderer never silently wraps free-form replies in templates.

## UI/UX

WhatsApp UI is native chat UI. The adapter controls message families and payload ids, not screen layout.

Message rules:

- Choice labels are human-readable and fit WhatsApp limits.
- Payload ids carry only opaque refs and indexes; they do not expose user text.
- Confirm buttons use explicit labels from `ConfirmCard`.
- Capture-as-fallback acknowledgement uses the canonical saved-as-idea shape from spec 074.
- Operator status appears in monitoring, not inside user chat.

Operator status surface:

| Field | Meaning |
|-------|---------|
| enabled state | explicit SST enablement |
| credentials readiness | all required keys resolved |
| signature rejection count | webhook authenticity failures |
| idempotent retry count | duplicate Meta deliveries |
| last inbound/outbound | timestamps, no raw phone |
| rendered text/list/buttons/capture ack | renderer coverage counters |

## Security/Compliance

- Signature verification occurs over the raw request body before JSON translation.
- `webhook_verify_token`, `app_secret`, and `access_token` are secrets and are never logged.
- Phone numbers are normalized and HMACed before persistence or telemetry.
- Unknown phone mappings are refused before facade invocation.
- Outbound send errors do not expose token values, phone numbers, or raw user text.
- Side-effect-bearing assistant actions still require server-side confirmation; WhatsApp buttons only submit the confirm choice.
- No marketing, unsolicited push, or broadcast send path is introduced.

Authorization matrix:

| Endpoint | Public Meta | Enrolled User | Operator |
|----------|-------------|---------------|----------|
| `GET /v1/assistant/transports/whatsapp/webhook` | verify-token only | no | no |
| `POST /v1/assistant/transports/whatsapp/webhook` | signed delivery only | mapped after signature | no |
| Identity provisioning CLI/API | no | no | admin only |
| Transport status dashboard | no | no | operator read |

## Configuration And Migrations

Required SST keys when `assistant.transports.whatsapp.enabled = true`:

| Key | Validation |
|-----|------------|
| `assistant.transports.whatsapp.enabled` | strict bool |
| `assistant.transports.whatsapp.webhook_path` | non-empty path, must start `/` |
| `assistant.transports.whatsapp.phone_number_id` | non-empty |
| `assistant.transports.whatsapp.business_account_id` | non-empty |
| `assistant.transports.whatsapp.webhook_verify_token` | non-empty secret |
| `assistant.transports.whatsapp.app_secret` | non-empty secret |
| `assistant.transports.whatsapp.access_token` | non-empty secret |
| `assistant.transports.whatsapp.message_template_namespace` | non-empty when template sends are enabled by policy |
| `assistant.transports.whatsapp.identity_hash_key` | non-empty secret used for phone HMAC |
| `assistant.transports.whatsapp.api_base_url` | non-empty HTTPS URL |
| `assistant.transports.whatsapp.api_version` | non-empty closed value approved by config validation |
| `assistant.transports.whatsapp.rate_limit_per_user_per_minute` | integer `>= 1` |
| `assistant.transports.whatsapp.max_text_chars` | integer `>= 1` |

When `enabled=false`, the adapter is not mounted and no WhatsApp ingress is exposed. Telegram and HTTP registration continue independently.

Migration adds `assistant_transport_identities`. The table is generic and can be reused by later transport adapters.

## Observability

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_whatsapp_webhook_total` | `outcome` | signed, rejected, duplicate, schema failure |
| `smackerel_assistant_whatsapp_turns_total` | `kind,outcome` | translated turns and facade outcomes |
| `smackerel_assistant_whatsapp_render_total` | `message_type,outcome` | text/list/buttons/template-required/error |
| `smackerel_assistant_whatsapp_send_latency_seconds` | `outcome` | Cloud API send latency |
| `smackerel_assistant_whatsapp_identity_total` | `outcome` | mapping hit/miss/disabled |

Structured logs include request id, hashed user id when resolved, WhatsApp message id, render type, facade flag, status, and error code. They exclude raw phone numbers, secrets, raw text, and access tokens.

IntentTrace integration: every accepted webhook stamps `transport="whatsapp"`, `transport_message_id=<Meta message id>`, and any capture route outcome from spec 074.

## Testing Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-072-A01 | integration | `tests/integration/assistant/whatsapp_webhook_test.go` | signed text webhook becomes `AssistantMessage` |
| SCN-072-A02 | unit + integration | `internal/whatsapp/assistant_adapter/verify_test.go` | bad signatures reject before facade |
| SCN-072-A03 | unit | `internal/whatsapp/assistant_adapter/render_golden_test.go` | three choices render as buttons |
| SCN-072-A04 | unit | same | unknown shape renders text or explicit render error, never drop |
| SCN-072-A05 | integration | `tests/integration/assistant/whatsapp_capture_test.go` | CaptureRoute invokes capture once and sends ack |
| SCN-072-A06 | unit | `internal/config/assistant_whatsapp_test.go` | missing enabled credential fails startup |
| SCN-072-A07 | integration | `tests/integration/assistant/transport_disable_test.go` | disabling WhatsApp leaves Telegram/HTTP healthy |
| SCN-072-A08 | e2e-api | `tests/e2e/assistant/whatsapp_roundtrip_test.go` | confirm/disambig/reset round-trip through facade |
| SCN-072-A09 | unit | `internal/whatsapp/assistant_adapter/template_policy_test.go` | normal replies do not use templates |
| SCN-072-A10 | integration | `tests/integration/assistant/whatsapp_idempotency_test.go` | duplicate Meta delivery invokes facade once |

## Risks & Open Questions

| Risk | Mitigation |
|------|------------|
| Meta payload shape drift | golden fixtures for webhook payload variants and schema error tests |
| Identity mismatch from phone normalization | one normalization function, HMAC lookup tests, operator provisioning validation |
| Render limits truncate meaning | renderer budget tests preserve controls and source labels before body trimming |
| WhatsApp throttling prevents delivery | bounded Cloud API retry policy with final observable failure; user turn remains deduped |
| Adapter leaks business logic | import/lint guard forbids scenario/tool packages inside WhatsApp adapter |