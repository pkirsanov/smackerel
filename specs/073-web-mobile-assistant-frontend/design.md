# Design: 073 Web/Mobile Assistant Frontend Client

Owner: `bubbles.design`
Workflow mode: `product-to-planning`
Status ceiling for this pass: `specs_hardened`
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 069 defines the synchronous `POST /api/assistant/turn` HTTP adapter and pins request/response JSON with `schema_version="v1"`, but the only concrete consumer today is test infrastructure. The repo has a real embedded PWA surface under [web/pwa](../../web/pwa), including Playwright-style tests, and no committed native or shared mobile client tree.

**Target State.** Add a minimal web assistant chat plus one shared mobile assistant codebase that produces iPhone/iOS and Android clients from the same mobile foundation. Web, iPhone/iOS, and Android consume generated models from the spec 069 schema, render `AssistantResponse` through one schema-driven response renderer, preserve `transport_message_id` across retries, and keep scenario decisions entirely server-side.

**Patterns to Follow.** Use the existing embedded PWA shape for the web page and tests, spec 044 auth/session requirements, spec 060 `assistant.turn` authorization, spec 069's `POST /api/assistant/turn` contract, and the assistant response vocabulary already used by transport renderers. Keep the first screen utilitarian: session gate when required, transcript, composer, response controls, retry state, and sources.

**Patterns to Avoid.** Do not add per-platform assistant routes, client-side scenario branching, local schema extensions, command menus as the primary UX, transport-hint-driven affordances, or separate iOS and Android business/UI codebases. Do not copy the existing PWA settings pattern from [web/pwa/app.js](../../web/pwa/app.js) that reads and writes auth material from browser storage.

**Resolved Decisions.** Web lives under `web/pwa/assistant.*` and posts to the existing endpoint using generated web models. Mobile introduces one new shared mobile codebase, provisionally `clients/mobile/assistant/`, with generated schema models, a shared turn/retry state machine, and a shared response-renderer core; iPhone/iOS and Android differences live only in thin platform adapters for secure session handoff, accessibility bridges, safe areas/insets, navigation shell, and packaging. Both mobile targets send `transport_hint="mobile"`; web sends `transport_hint="web"`; the value is telemetry only.

**Open Questions.** The concrete mobile runtime/toolchain is not committed in this repo yet, so implementation planning must select one owner-approved shared-mobile runtime that satisfies this architecture rather than treating a named tool as already present. If implementation finds that `/api/assistant/turn` only accepts Authorization headers, the auth owner must decide whether same-origin HttpOnly cookie validation is added for web or whether web uses a memory-only bearer handoff for the active page lifecycle.

## Purpose & Scope

This design reconciles spec 073 after the product requirement changed from a web plus single-platform mobile client to web plus a shared iOS+Android mobile foundation. It is planning-only and does not introduce source, test, config, docs, or plan-artifact edits.

The feature proves that the spec 069 wire contract is renderable by real human-facing clients without adding server routes or moving assistant scenario logic into clients. The required user actions are: send a natural-language turn, choose disambiguation, accept or cancel confirmation, reset pending assistant state, open sources, see capture-as-fallback acknowledgement, and retry transient failures with the same `transport_message_id`.

## Architecture Overview

```text
Spec 069 assistant_turn_v1 schema
  -> generated web schema models and validator
  -> generated shared-mobile schema models

Web assistant client
  -> web turn state
  -> schema-driven response renderer
  -> POST /api/assistant/turn with transport_hint="web"

Shared mobile assistant codebase
  -> shared mobile turn/retry state
  -> shared mobile response-renderer core
  -> iPhone/iOS platform adapter
  -> Android platform adapter
  -> POST /api/assistant/turn with transport_hint="mobile"
```

| Component | Planned Location | Responsibility |
|-----------|------------------|----------------|
| Web assistant page | `web/pwa/assistant.html` | Chat-first browser route served by the existing embedded PWA pipeline |
| Web assistant logic | `web/pwa/assistant.js` | Session gate, composer, idempotent retry, generated schema validation, web rendering projection |
| Web generated models | `web/pwa/generated/assistant_turn_v1.*` | Generated request/response types and runtime validation from spec 069 |
| Shared mobile codebase | `clients/mobile/assistant/` | One mobile foundation that builds iPhone/iOS and Android clients |
| Mobile shared core | `clients/mobile/assistant/core/` | Generated models, turn state machine, retry state, render descriptors, source/action semantics |
| Mobile platform adapters | `clients/mobile/assistant/platform/ios/`, `clients/mobile/assistant/platform/android/` | Secure session handoff, accessibility bridge labels, safe-area/system-inset handling, packaging shell |
| Cross-client fixtures | `tests/fixtures/assistant_response_v1/` | Golden response shapes used to prove web, iPhone/iOS, and Android renderer parity |
| Existing server route | `POST /api/assistant/turn` | Spec 069 HTTP adapter; no new route is designed here |

The mobile architecture intentionally chooses a schema-first shared-client foundation instead of a named mobile framework as current repo fact. A later implementation may select a specific runtime only if it preserves one shared mobile codebase and this adapter boundary. Splitting iPhone/iOS and Android into independent client implementations requires a future owner-approved design amendment documenting infeasibility and the smallest acceptable split.

## Capability Foundation

The reusable capability is `AssistantClientSurface`: a schema-driven client capability shared by web and the shared mobile codebase, with the mobile codebase producing iPhone/iOS and Android clients from one foundation.

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| `AssistantSchemaModels` | Generated request/response models pinned to spec 069 `schema_version="v1"` | Web, shared mobile, parity fixtures |
| `AssistantTurnClient` | Submit text, disambiguation, confirm, and reset turns to `POST /api/assistant/turn` | Web and shared mobile |
| `AssistantRetryState` | Retain the original request body and `transport_message_id` across timeout, 5xx, 429, and offline recovery | Web, iPhone/iOS, Android |
| `AssistantResponseRenderer` | Convert `AssistantResponse` shapes into render descriptors for body, sources, disambiguation, confirm, reset, capture acknowledgement, and error states | Web and shared mobile |
| `AssistantA11yContract` | Define reading order, labels, focus behavior, live/status announcements, and platform accessibility parity | Web ARIA, iPhone/iOS VoiceOver, Android TalkBack |
| `AssistantSecurityBoundary` | Keep auth/session material behind auth/platform adapters and out of renderer, transcript, logs, copy actions, and generated fixtures | Web, iPhone/iOS, Android |

Foundation-owned behavior:

- Render by response shape only, never by scenario id, action class, `transport_hint`, or platform.
- Preserve one `transport_message_id` for each attempted turn and reuse it for retries until the user edits the turn.
- Treat confirmation controls as server round-trips; clients never execute side effects locally.
- Break client builds or schema-validation tests when the spec 069 schema changes incompatibly.
- Render capture-as-fallback acknowledgement from the returned `AssistantResponse` shape and copy; clients do not decide to capture locally.
- Expose one shared mobile renderer/state core to both iPhone/iOS and Android platform adapters.

### Variation Axes

| Axis | Values | Foundation-Owned? | Notes |
|------|--------|-------------------|-------|
| Surface | web, mobile | Partly | Web has a PWA projection; mobile has one shared foundation. |
| Mobile platform adapter | iPhone/iOS, Android | No for shell details; yes for behavior parity | Adapters may handle OS APIs but not assistant semantics. |
| Transport hint | `web`, `mobile` | Yes | Closed vocabulary, telemetry only. |
| Auth carrier | same-origin web session, mobile auth adapter session | Yes | No renderer access to bearer/session material. |
| Response shape | body, sources, disambiguation, confirm, reset, capture acknowledgement, error | Yes | One response renderer vocabulary. |
| Accessibility bridge | ARIA/live region, VoiceOver, TalkBack | Partly | Labels/order are shared; platform bridge APIs vary. |
| Layout | desktop drawer, mobile sheet, stacked phone actions, tablet side sheet | No | Layout may differ, semantics must not. |

## Concrete Implementations

### Web Client

The web client is an embedded PWA page under `web/pwa/assistant.html` with logic in `web/pwa/assistant.js`. It uses the existing static asset embedding path and follows the repo's web test pattern under `web/pwa/tests/`.

Web requests use `POST /api/assistant/turn` with `transport_hint="web"`. The preferred auth path is same-origin authenticated requests that do not expose bearer material to JavaScript. If the existing route requires Authorization headers during implementation, the web client may hold a bearer value only in memory for the active page lifecycle through an auth-owner-approved handoff; it must not write auth/session material to browser storage, service worker cache, IndexedDB, logs, copy buffers, or accessibility labels.

### Shared Mobile Client

The mobile client is one codebase under `clients/mobile/assistant/`. The shared portion owns generated models, request construction, idempotent retry state, transcript state, response-to-render-descriptor mapping, and all assistant controls. iPhone/iOS and Android package the same shared behavior.

Platform adapters may contain only these concerns:

- Secure session handoff using the auth-owner-approved platform mechanism. Keychain-class or Keystore-class APIs may be used only behind the adapter and never exposed to shared renderer code.
- VoiceOver and TalkBack label bridging from shared accessibility descriptors.
- Safe area, status bar, keyboard, back gesture, and system navigation inset handling.
- Packaging metadata and platform shell navigation.

Platform adapters must not branch on assistant scenario id, action class, response status, source type, or `transport_hint` to decide visible assistant affordances. Any platform-specific limitation that would require separate iOS and Android business logic blocks completion until an owner-approved design amendment resolves it.

### Shared Schema And Renderer Core

Spec 069 remains the single source of truth for the request and response schema. Code generation emits web models and shared-mobile models from the same schema artifact. The renderer consumes generated `AssistantResponse` models and emits a render descriptor such as:

```json
{
  "message_role": "assistant",
  "body": "assistant response text",
  "sources": [],
  "controls": [
    {"kind": "disambiguation", "ref": "ref", "choices": []},
    {"kind": "confirm", "ref": "ref", "choices": ["accept", "decline"]}
  ],
  "status": "rendered",
  "capture_acknowledgement": false,
  "trace": {"visible_to_operator": false}
}
```

The descriptor is a client-side view model, not a new server contract. It exists to keep web, iPhone/iOS, and Android rendering semantics aligned while allowing platform-specific layout projection.

## Data Model And Storage

No new server table is required. Server-side assistant state remains owned by existing assistant conversation storage, auth/session state remains owned by specs 044/060, and trace/replay state remains outside this spec.

Client state is ephemeral UI state:

```json
{
  "draft_text": "string",
  "pending_turn": {
    "transport_message_id": "stable-client-id",
    "request_body": {},
    "retry_count": 0,
    "status": "pending|retrying|failed|offline"
  },
  "transcript": [
    {"role": "user", "text": "...", "transport_message_id": "stable-client-id"},
    {"role": "assistant", "schema_version": "v1", "response": {}}
  ]
}
```

Persistence rules:

- Transcript persistence is not part of this design.
- Draft text may be kept only as non-secret UI state and must be cleared when the user signs out.
- `pending_turn.request_body` may be retained in memory while retry is possible; if a platform persists queued requests for offline recovery, the queue must exclude bearer/session material and must be covered by plan-owned security tests before completion.
- `transport_message_id` is diagnostic/idempotency metadata, not user-facing required copy.

## API/Contracts

Endpoint: `POST /api/assistant/turn` from spec 069.

Request schema v1 is consumed unchanged:

```json
{
  "schema_version": "v1",
  "transport_message_id": "client-stable-id",
  "kind": "text",
  "transport_hint": "web",
  "text": "weather in Barcelona tomorrow",
  "confirm_ref": null,
  "confirm_choice": null,
  "disambiguation_ref": null,
  "disambiguation_choice": null,
  "client_context": {"conversation_id": "optional-client-thread-id"}
}
```

Web uses `transport_hint="web"`. iPhone/iOS and Android both use `transport_hint="mobile"`; platform identity, if needed for observability, belongs in redacted client telemetry metadata and must not affect server behavior or renderer affordances.

Follow-on disambiguation request:

```json
{
  "schema_version": "v1",
  "transport_message_id": "new-stable-id-for-choice-turn",
  "kind": "disambiguation",
  "transport_hint": "mobile",
  "text": "",
  "confirm_ref": null,
  "confirm_choice": null,
  "disambiguation_ref": "ref-from-response",
  "disambiguation_choice": 2,
  "client_context": {"conversation_id": "optional-client-thread-id"}
}
```

Follow-on confirm request:

```json
{
  "schema_version": "v1",
  "transport_message_id": "new-stable-id-for-confirm-turn",
  "kind": "confirm",
  "transport_hint": "web",
  "text": "",
  "confirm_ref": "ref-from-response",
  "confirm_choice": "accept",
  "disambiguation_ref": null,
  "disambiguation_choice": null,
  "client_context": {"conversation_id": "optional-client-thread-id"}
}
```

Response schema v1 is consumed unchanged:

```json
{
  "schema_version": "v1",
  "transport": "web",
  "transport_message_id": "client-stable-id",
  "status": "checking_weather",
  "body": "assistant response text",
  "sources": [],
  "sources_overflow_count": 0,
  "confirm_card": null,
  "disambiguation_prompt": null,
  "error_cause": "",
  "capture_route": false,
  "trace": {
    "assistant_turn_id": "turn-id",
    "agent_trace_id": "trace-id-or-null",
    "request_id": "http-request-id"
  },
  "facade_invoked": true,
  "emitted_at": "2026-05-31T00:00:00Z"
}
```

Error model and client behavior:

| Condition | HTTP Status | Client Behavior |
|-----------|-------------|-----------------|
| Missing or invalid auth | 401 | Show session-required state, disable submit, preserve unsubmitted draft |
| Missing `assistant.turn` scope | 403 | Show insufficient-scope state, disable submit, do not retry automatically |
| Invalid schema or unsupported client schema version | 400 | Show schema/version error and block submit until client is updated |
| Body too large | 413 | Keep draft editable and show size error |
| Rate limited | 429 | Show retry card; user retry reuses the same `transport_message_id` |
| Timeout before response | N/A | Show retry/offline state; retry reuses the original request body and id |
| 5xx after request accepted | 500 family | Show retry card; if server dedupes, replace error with returned response |

No client may mint a replacement `transport_message_id` for the same attempted turn because a retry happened. A new id is allowed only after the user edits the message or starts a distinct follow-on action.

Authorization matrix:

| Surface | Public | Authenticated User With `assistant.turn` | Operator/Devtools Context |
|---------|--------|------------------------------------------|---------------------------|
| Web assistant page | Session gate only | Own assistant chat | Optional trace affordance when separately authorized |
| Shared mobile assistant tab | No | Own assistant chat on iPhone/iOS and Android | No special assistant controls in v1 |
| `POST /api/assistant/turn` | No | Yes | Yes as a scoped user |
| Response detail and sources | No | Safe own response detail | Redacted trace metadata only when authorized |

## UI/UX Considerations

The first useful screen is the assistant chat, not a landing page. The session gate appears only when auth/session state is missing or invalid.

Shared component structure:

```text
AssistantChatScreen
  SessionGate
  Transcript
    UserMessage
    AssistantResponseCard
      BodyText
      StatusLine
      CitationTrigger
      DisambiguationControl
      ConfirmationControl
      CaptureAcknowledgement
      ErrorOrRetryCard
  Composer
  ResponseDetailPanelOrSheet
```

Renderer rules:

- `BodyText` renders `response.body` when present.
- `CitationTrigger` and source rows render only from `response.sources` and `sources_overflow_count`.
- `DisambiguationControl` renders only from `response.disambiguation_prompt`.
- `ConfirmationControl` renders only from `response.confirm_card`.
- `CaptureAcknowledgement` renders from the capture response shape returned by the server; clients do not infer capture from failed scenario matching.
- Unknown optional response fields are ignored after schema validation, but known required field mismatches fail loudly through generated validation/build checks.

Accessibility requirements:

- Web uses ordered message roles, keyboard-reachable composer/actions/sources, and an ARIA live region for new assistant responses and retry/error state changes.
- iPhone/iOS uses VoiceOver labels generated from the shared accessibility descriptor for role, body, consequence, choice ordinal, action label, source state, and retry status.
- Android uses TalkBack labels generated from the same descriptor and preserves the same reading order.
- Mobile platform adapters may adjust bridge mechanics, safe areas, and system navigation, but label text and focus order must remain semantically equivalent on iPhone/iOS and Android.
- No token, cookie, raw trace, or secret value appears in visible text, copied text, logs, or accessibility labels.

## Security & Compliance

- Spec 044 authenticated user context and spec 060 `assistant.turn` scope are required before submit is enabled.
- Web must not store auth/session material in `localStorage`, `sessionStorage`, IndexedDB, service worker cache, or other browser storage.
- Mobile shared code must not read or persist bearer/session material. Only platform auth adapters may perform auth-owner-approved secure session handoff, and renderer/core code receives only an authorized request capability.
- iPhone/iOS secure-session handling is an adapter detail, not a separate assistant implementation.
- Android secure-session handling is an adapter detail, not a separate assistant implementation.
- Copy/export actions exclude bearer tokens, auth cookies, raw trace payloads, raw prompts unless explicitly allowed by a separate export policy, and source bodies.
- Clients never execute side effects locally; they submit server-provided confirm choices to `/api/assistant/turn`.
- `transport_hint` cannot alter route selection, tool allowlist, permissions, response shape, or visible affordances.
- Client telemetry must use closed labels and redacted identifiers; full prompt text and source bodies are excluded.

## Configuration & Build

All client configuration is SST-managed and fail-loud. Missing values fail build or startup with named errors; no client guesses backend URL, schema version, transport hint, or auth mode.

Required design keys:

| Key | Validation |
|-----|------------|
| `web.assistant.enabled` | strict bool |
| `web.assistant.backend_base_url` | explicit same-origin marker or explicit non-empty URL from SST |
| `web.assistant.schema_version` | must equal spec 069 `v1` |
| `mobile.assistant.enabled` | strict bool |
| `mobile.assistant.backend_base_url` | explicit non-empty HTTPS URL from SST for mobile builds |
| `mobile.assistant.schema_version` | must equal spec 069 `v1` |
| `mobile.assistant.platforms` | explicit set containing both `ios` and `android` for this spec |
| `mobile.assistant.auth_mode` | explicit auth-owner-approved mode |

Build gates:

- Schema generation must run before web and mobile client builds.
- Incompatible schema changes must fail web and shared-mobile builds before runtime.
- Mobile build validation must prove the single shared mobile codebase includes both iPhone/iOS and Android targets.
- If a selected mobile runtime cannot share the renderer/state core across iPhone/iOS and Android, implementation must stop and route a design amendment before coding around the split.

## Observability & Failure Handling

Client observability is redacted and keyed by closed labels:

| Metric/Event | Labels | Meaning |
|--------------|--------|---------|
| `smackerel_assistant_client_turn_total` | `surface,platform,kind,outcome` | Turn submit/result for web, iPhone/iOS, and Android |
| `smackerel_assistant_client_retry_total` | `surface,platform,reason,reused_transport_message_id` | Idempotent retry behavior; `reused_transport_message_id` must be true for retries |
| `smackerel_assistant_client_render_total` | `surface,platform,response_shape` | Renderer coverage by schema shape |
| `smackerel_assistant_client_schema_validation_total` | `surface,platform,outcome` | Generated model validation/build outcomes |
| `smackerel_assistant_client_a11y_check_total` | `platform,assistive_tech,outcome` | ARIA, VoiceOver, and TalkBack checks |

Failure handling rules:

- Timeout, 429, and 5xx retry cards retain the original request body and `transport_message_id`.
- Offline recovery may disable retry until connectivity returns, but it must preserve draft text and pending id.
- Permission errors do not retry automatically.
- Missing config is a non-retryable fail-loud state naming the missing SST key.
- If a response arrives after a prior timeout, the client replaces the retry card with the deduped response rather than adding a duplicate assistant response.
- Logs exclude bearer tokens, cookies, full prompts, source bodies, and platform secure-storage details.

## Testing & Validation Strategy

| Scenario | Test Type | Planned Location | Assertion |
|----------|-----------|------------------|-----------|
| SCN-073-A01 | e2e-ui + e2e-api | `web/pwa/tests/assistant_chat.spec.ts` | Web sends an authenticated turn to `/api/assistant/turn` and renders body, sources, and controls |
| SCN-073-A02 | build/unit | `clients/mobile/assistant/tests/schema_generation.*` plus web schema test | Incompatible spec 069 schema changes fail generated web and shared-mobile models before either mobile target ships |
| SCN-073-A03 | unit + e2e-ui | `web/pwa/tests/assistant_retry.spec.ts`, `clients/mobile/assistant/tests/retry_parity.*` | Timeout and 5xx retries reuse the exact original `transport_message_id` on web, iPhone/iOS, and Android |
| SCN-073-A04 | e2e-ui + shared fixture | `tests/fixtures/assistant_response_v1/disambiguation.json`, web/mobile renderer tests | Disambiguation choices render and round-trip by schema refs on all three user surfaces |
| SCN-073-A05 | e2e-ui + shared fixture | confirm-card fixture and renderer tests | Confirm accept/cancel posts server-provided refs and never executes side effects client-side |
| SCN-073-A06 | e2e-ui + fixture parity | capture acknowledgement fixture and renderer tests | Saved-as-idea acknowledgement shape and copy match the server response on web, iPhone/iOS, and Android |
| SCN-073-A07 | unit/guard | renderer source scan and unknown-shape fixture | Renderer contains no scenario id, action class, or `transport_hint` branching for affordances |
| SCN-073-A08 | integration + client contract | spec 069 transport hint test plus client request tests | Web sends `web`; iPhone/iOS and Android send `mobile`; hints remain telemetry only |
| SCN-073-A09 | e2e-ui accessibility | `web/pwa/tests/assistant_accessibility.spec.ts` | Keyboard path and ARIA live/status announcements work for web chat, retry, sources, disambiguation, and confirm |
| SCN-073-A10 | mobile accessibility | `clients/mobile/assistant/tests/accessibility_ios.*`, `clients/mobile/assistant/tests/accessibility_android.*` | VoiceOver and TalkBack reach composer, choices, confirms, citations, capture acknowledgement, retry/offline, and session errors in equivalent order |
| SCN-073-A11 | unit/build | web and mobile config validation tests | Missing backend URL/schema/platform/auth config fails loud with the missing key name |

Additional validation requirements:

- Web, iPhone/iOS, and Android parity tests must use the same golden `AssistantResponse` fixtures.
- Mobile parity tests must prove one shared mobile renderer/state core feeds both platform adapters.
- Retry tests must include an adversarial case where the test fails if a retry mints a fresh `transport_message_id`.
- Source scans must reject client renderer decisions based on `scenario_id`, action class, or `transport_hint`.
- Accessibility validation must include both VoiceOver and TalkBack, not only generic mobile label checks.

## Alternatives & Tradeoffs

| Alternative | Decision | Rationale |
|-------------|----------|-----------|
| Separate iOS and Android client codebases | Rejected | Violates the spec's shared mobile codebase requirement and creates renderer drift risk. |
| Named mobile framework selected in design without repo support | Rejected | The repo has no committed mobile toolchain; the design should constrain architecture and let planning select tooling truthfully. |
| Web-only proof of spec 069 | Rejected | Does not satisfy the required iPhone/iOS and Android consumers or mobile parity tests. |
| Client-side scenario-specific UI branches | Rejected | Moves assistant behavior out of the server contract and breaks transport parity. |
| Store auth tokens in browser or shared mobile renderer state | Rejected | Violates the security boundary and risks exposing bearer/session material. |
| Server route per client | Rejected | Spec 069 establishes one HTTP route and closed `transport_hint` vocabulary. |

## Risks & Open Questions

| Item | Owner / Decision Path |
|------|-----------------------|
| Mobile runtime/toolchain is not committed | `/bubbles.plan` must choose a concrete shared-mobile implementation path or route back to design if no compliant option exists. |
| Plan-owned artifacts may still describe the prior single-platform mobile design | `/bubbles.plan` must reconcile scopes, test-plan, scenario-manifest, report template, and user validation against this design. |
| Web cookie support for `/api/assistant/turn` may be absent | Auth/API owner decides whether same-origin cookie auth is supported or whether web uses an in-memory bearer handoff. |
| Mobile auth persistence across process death may need a broader auth design | Auth owner must approve any OS secure-session adapter behavior before implementation persists or restores session capability. |
| Offline retry persistence can become sensitive if it stores full prompts | Planning must either keep retry state memory-only or add security tests for any persisted queue that excludes auth material. |

## Superseded Design Decisions

The prior active design treated Android as the only mobile implementation and described a platform-specific mobile module. That architecture is no longer active. The current design requires one shared mobile codebase that produces iPhone/iOS and Android clients, with platform-specific behavior restricted to adapters for secure session handoff, accessibility bridges, safe areas/insets, navigation shell, and packaging.
