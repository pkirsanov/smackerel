# Design: 069 Assistant HTTP Transport

Owner: `bubbles.design`  
Workflow mode: `product-to-planning`  
Status ceiling for this pass: `specs_hardened`  
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 061 defines `TransportAdapter`, `AssistantMessage`, `AssistantResponse`, and `Facade.Handle`. Telegram is the only concrete adapter. Assistant E2E tests either call Go internals or depend on Telegram-shaped boundaries, so compiler, micro-tool, open-knowledge, disambiguation, confirmation, reset, and capture behavior lack a deterministic HTTP ingress.

**Target State.** Add one bearer-authenticated HTTP adapter and one route, `POST /api/assistant/turn`, that translates JSON into `AssistantMessage`, calls the same facade as Telegram, and renders `AssistantResponse` as versioned JSON. This becomes the canonical assistant live-stack E2E surface and the backend contract for later clients.

**Patterns to Follow.** Implement a second concrete adapter without changing the interface. Mount under the existing `/api` bearer-auth group and spec 060 scope middleware. Use existing conversation state keyed by `(UserID, Transport)`, existing source/provenance types, and golden JSON contract tests.

**Patterns to Avoid.** Do not create per-frontend routes, anonymous test ingress, shared-secret bypass, transport-specific scenario logic, a second facade, streaming in v1, or schema changes without a version bump. `transport_hint` must never alter route, tool allowlist, response shape, or side-effect behavior.

**Resolved Decisions.** V1 is synchronous `POST /api/assistant/turn`. Package name is `internal/assistant/httpadapter`. The adapter sets `Transport="web"`; `transport_hint` is telemetry metadata constrained to `web`, `mobile`, or `bridge`. Request `kind` reuses `contracts.MessageKind`. Wire schema is pinned as `schema_version="v1"`.

**Open Questions.** Scope syntax must be reconciled with spec 060 before implementation: spec 069 reserves the product label `assistant.turn`, while spec 060 currently owns the final scope-claim grammar. Planning must select the implementation spelling in `assistant.transports.http.required_scope`.

## Purpose And Scope

This design exposes the existing assistant capability over HTTP. It does not build a web/mobile/WhatsApp client, add streaming, or change the capability-layer contract.

## Architecture Overview

```text
HTTP client / E2E test
  -> POST /api/assistant/turn
  -> bearer auth + required assistant turn scope
  -> body-size, rate, CORS, schema validation
  -> httpadapter.Translate
  -> shared Facade.Handle
  -> httpadapter.RenderJSON
  -> HTTP response
```

Telegram and HTTP are peer adapters.

## Capability Foundation

The reusable foundation is the existing `TransportAdapter` contract.

```go
type HTTPAdapter struct {
    Facade contracts.Assistant
    Clock func() time.Time
    Config HTTPTransportConfig
}
```

Policies:

- Adapters translate and render only.
- `Facade.Handle` is invoked exactly once for accepted turns.
- Auth, scope, size, rate, and schema failures happen before facade invocation.
- CaptureRoute is honored by the adapter.
- Response schema is stable and versioned.

### Variation Axes

| Axis | Values | Enforcement |
|------|--------|-------------|
| Transport implementation | Telegram, HTTP | shared adapter interface |
| Client hint | web, mobile, bridge | closed vocabulary, telemetry only |
| Message kind | text, confirm, disambiguation, reset | existing `MessageKind` |
| Failure boundary | auth, scope, schema, limit, facade | HTTP status + `facade_invoked` |
| Conversation key | user + transport | existing table primary key |

## Concrete Implementations

### HTTP Adapter Package

Package: `internal/assistant/httpadapter`.

Responsibilities: decode/validate JSON, resolve authenticated user id, build `AssistantMessage{Transport:"web"}`, copy `transport_hint` into metadata, call the facade, invoke capture when `CaptureRoute=true`, and render JSON.

### API Route

Route: `POST /api/assistant/turn`.

Middleware order:

1. request id
2. body-size cap
3. bearer auth
4. required assistant turn scope
5. per-user rate limit
6. JSON schema validation
7. adapter/facade invocation

## Data Model

No new table is required. Existing `assistant_conversations` stores state for `transport="web"`. Existing `agent_traces` and assistant audit payloads record trace data. Idempotency is keyed by `(user_id, transport, transport_message_id)`; any durable dedupe table must be planned without changing the v1 wire schema.

## API Contracts

### Request Schema v1

```json
{
  "schema_version": "v1",
  "transport_message_id": "test-turn-001",
  "kind": "text",
  "transport_hint": "web",
  "text": "weather in palm springs ca tomorrow",
  "confirm_ref": null,
  "confirm_choice": null,
  "disambiguation_ref": null,
  "disambiguation_choice": null,
  "client_context": {"conversation_id": "optional-client-thread-id"}
}
```

Validation: schema version and transport message id are required; `kind` is `text`, `confirm`, `disambiguation`, or `reset`; callback kinds require their refs/choices; unknown transport hints reject before the facade.

### Response Schema v1

```json
{
  "schema_version": "v1",
  "transport": "web",
  "transport_message_id": "test-turn-001",
  "status": "checking_weather",
  "body": "assistant response text",
  "sources": [],
  "sources_overflow_count": 0,
  "confirm_card": null,
  "disambiguation_prompt": null,
  "error_cause": "",
  "capture_route": false,
  "trace": {"assistant_turn_id": "turn-id", "agent_trace_id": "trace-id-or-null", "request_id": "http-request-id"},
  "facade_invoked": true,
  "emitted_at": "2026-05-31T00:00:00Z"
}
```

### Error Responses

| Condition | Status | Code | Facade Invoked |
|-----------|--------|------|----------------|
| Missing/invalid bearer token | 401 | `auth_required` / `auth_invalid` | no |
| Missing required scope | 403 | `scope_required` | no |
| Invalid schema | 400 | `invalid_assistant_turn` | no |
| Body too large | 413 | `body_too_large` | no |
| Rate limit exceeded | 429 | `rate_limited` | no |
| Facade internal error | 500 | `assistant_turn_failed` | yes when invocation started |

## Configuration

Required SST keys:

| Key | Purpose |
|-----|---------|
| `assistant.transports.http.enabled` | strict bool |
| `assistant.transports.http.schema_version` | expected wire schema |
| `assistant.transports.http.body_size_max_bytes` | request cap |
| `assistant.transports.http.rate_limit_per_user_per_minute` | rate limit |
| `assistant.transports.http.cors_allowed_origins` | explicit origins |
| `assistant.transports.http.conversation_ttl_seconds` | state TTL |
| `assistant.transports.http.transport_hint_allowlist` | hint vocabulary |
| `assistant.transports.http.required_scope` | spec-060-approved scope label |

Missing config fails startup.

## Security And Compliance

- No anonymous route exists.
- Bearer auth and assistant turn scope are required before facade execution.
- CORS origins are explicit.
- Error responses never echo tokens or secret headers.
- Transport hints cannot alter routing, tools, or response shape.
- Side effects remain governed by spec 068 confirmation and policy gates.

## Observability And Failure Handling

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_http_turns_total` | `status,facade_invoked,transport_hint` | route outcomes |
| `smackerel_assistant_http_turn_latency_seconds` | `status` | adapter latency |
| `smackerel_assistant_http_rejections_total` | `code` | pre-facade rejection count |
| `smackerel_assistant_transport_parity_total` | `scenario,outcome` | parity observations |

Logs include request id, hashed user id, hint, kind, facade flag, status, error code, assistant turn id, and agent trace id. Bodies are redacted.

## Testing And Validation Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-069-A01 | e2e-api | `tests/e2e/assistant/http_turn_test.go` | HTTP text turn returns schema-valid response |
| SCN-069-A02 | integration | `tests/integration/api/assistant_http_auth_test.go` | missing bearer returns 401 and facade is not invoked |
| SCN-069-A03 | e2e-api | `tests/e2e/assistant/http_disambiguation_test.go` | disambiguation round-trips |
| SCN-069-A04 | e2e-api | `tests/e2e/assistant/http_confirm_test.go` | confirm accept executes gated action |
| SCN-069-A05 | e2e-api | `tests/e2e/assistant/http_reset_test.go` | reset clears web pending state |
| SCN-069-A06 | e2e-api | `tests/e2e/assistant/http_capture_test.go` | capture invoked once and acknowledged |
| SCN-069-A07 | unit | `internal/assistant/httpadapter/golden_contract_test.go` | golden schema pins v1 |
| SCN-069-A08 | integration | `tests/integration/assistant/transport_parity_test.go` | Telegram and HTTP use same facade path |
| SCN-069-A09 | unit | `internal/assistant/httpadapter/transport_hint_test.go` | hints are telemetry only |
| SCN-069-A10 | integration | `tests/integration/api/assistant_http_limits_test.go` | rate/body caps reject before facade |
| SCN-069-A11 | e2e-api | `tests/e2e/assistant/http_live_stack_test.go` | live stack drives assistant without Telegram |

## Alternatives And Tradeoffs

| Option | Decision | Rationale |
|--------|----------|-----------|
| Async POST plus GET | Rejected for v1 | synchronous contract is enough for bounded assistant turns |
| Per-frontend routes | Rejected | forks transport semantics |
| Anonymous local test route | Rejected | misses auth and scope behavior |
| Streaming v1 | Out of scope | requires a separate delivery contract |

## Risks And Open Questions

| Risk | Mitigation |
|------|------------|
| Scope spelling conflict | Planning finalizes `required_scope` against spec 060 |
| HTTP adapter drifts from Telegram | parity tests and spec 067 transport-branch guard |
| Tests overfit prose | assert schema, status, trace, and state changes |
| Client retries duplicate turns | client-supplied `transport_message_id` scoped by user/transport |
