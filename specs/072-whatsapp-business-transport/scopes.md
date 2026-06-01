# Scopes: 072 WhatsApp Business Webhook Adapter

## Execution Outline

### Phase Order

1. **Scope 1 — Webhook, Identity, And Fail-Loud Config Foundation:** add WhatsApp adapter construction, Meta signature verification, generic transport identity lookup, and required SST validation before any facade invocation.
2. **Scope 2 — Response Renderer And Capture Acknowledgement:** render `AssistantResponse` shapes into WhatsApp text/list/button families, preserve capture-as-fallback acknowledgement, and prohibit silent template wrapping.
3. **Scope 3 — Round-Trip Controls And Idempotent Retries:** drive disambiguation, confirm, reset, and Meta retry behavior through the shared facade exactly once per transport message id.
4. **Scope 4 — Independent Disable And Operator Status:** prove WhatsApp enablement can be turned off without affecting Telegram/HTTP and expose operator-visible health counters.

### New Types & Signatures

- `internal/whatsapp/assistant_adapter.Options{CloudClient, IdentityRegistry, Capture, Verify, MaxTextChars, RateLimitPerUserPerMinute, Tracer}`
- `type WebhookVerifier interface { Verify(rawBody []byte, signature string) error; VerifyChallenge(token string) error }`
- `type TransportIdentityRegistry interface { Resolve(ctx context.Context, transport string, externalSubjectHash string) (userID string, err error) }`
- `type CloudClient interface { SendText(ctx context.Context, to string, msg TextMessage) error; SendInteractive(ctx context.Context, to string, msg InteractiveMessage) error }`
- Route contract: `GET|POST /v1/assistant/transports/whatsapp/webhook`, with exact mounted path supplied by SST.
- Migration contract: `assistant_transport_identities(transport, external_subject_hash, external_subject_type, user_id, status, verified_at, metadata, schema_version)`.

### Validation Checkpoints

- After Scope 1, signed webhooks translate to canonical messages, bad signatures stop before the facade, and enabled-with-missing-secret config fails loud.
- After Scope 2, golden renderer tests prove every response shape either maps to a WhatsApp type or explicit text/error behavior without dropped replies.
- After Scope 3, integration/e2e rows prove controls and Meta retries preserve facade parity and idempotency.
- After Scope 4, disable/status rows prove WhatsApp can be removed independently while Telegram and HTTP remain healthy.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries.
- Scope 1 is `foundation:true` because design introduces the reusable `TransportIdentityRegistry` foundation and WhatsApp proves `TransportAdapter` neutrality.
- No source, config, test, or docs work is performed by this planning pass.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Webhook, Identity, And Fail-Loud Config Foundation | API route, WhatsApp adapter, identity registry, config | SCN-072-A01, SCN-072-A02, SCN-072-A06 | Not Started |
| 2 | Response Renderer And Capture Acknowledgement | WhatsApp renderer, capture hook, golden fixtures | SCN-072-A03, SCN-072-A04, SCN-072-A05, SCN-072-A09 | Not Started |
| 3 | Round-Trip Controls And Idempotent Retries | facade bridge, disambig/confirm/reset, idempotency | SCN-072-A08, SCN-072-A10 | Not Started |
| 4 | Independent Disable And Operator Status | transport registry, monitoring, runbook/status surface | SCN-072-A07 | Not Started |

---

## Scope 1: Webhook, Identity, And Fail-Loud Config Foundation

**Status:** Not Started  
**Depends On:** —  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-072-A01 — Inbound webhook becomes a canonical AssistantMessage
  Given the WhatsApp adapter is enabled with valid SST credentials
  When a signed WhatsApp text message webhook arrives
  Then the adapter verifies the signature and translates the payload into an AssistantMessage with Transport = "whatsapp"
  And AssistantMessage.TransportMessageID equals the WhatsApp message id

Scenario: SCN-072-A02 — Unsigned or wrongly signed webhooks are rejected
  Given a webhook arrives without a valid X-Hub-Signature header
  When the adapter processes it
  Then the request is rejected with the standard error response
  And the facade is never invoked

Scenario: SCN-072-A06 — SST credentials are required and fail loud when enabled
  Given assistant.transports.whatsapp.enabled = true and access_token is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key
  And no WhatsApp ingress is exposed
```

### Implementation Plan

- Add `internal/whatsapp/assistant_adapter` with constructor-required verifier, cloud client, identity registry, capture hook, and tracer dependencies.
- Add raw-body signature verification and GET challenge verification before any JSON translation or facade invocation.
- Add `internal/assistant/transportidentity` and `assistant_transport_identities` for hashed external subjects, with no raw phone persistence.
- Add fail-loud `assistant.transports.whatsapp.*` validation when enabled, including webhook path, secrets, API base/version, rate limit, max text length, and identity hash key.
- Mount the webhook route only when WhatsApp is enabled and all required config passes.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `TransportAdapter` registry | WhatsApp must not change the spec 061 adapter contract | TP-072-01 and import/contract tests |
| Public webhook ingress | Bad signatures must stop before facade | TP-072-02 integration canary with facade spy |
| Identity registry | Later transports may reuse hashed subject lookup | TP-072-03 migration/lookup canary |

### Change Boundary

- **Allowed file families:** `internal/whatsapp/**`, `internal/api/**` route binding, `internal/assistant/transportidentity/**`, `internal/config/**`, DB migrations, and planned tests.
- **Excluded surfaces:** `contracts.TransportAdapter` signature changes, scenario/router code, Telegram renderer behavior, HTTP transport semantics, ML sidecar.
- **Containment rule:** any need to change the adapter contract routes to spec 061 ownership before implementation continues.

### Impact-Aware Validation

No project impact map is configured. Because this scope touches a public ingress and shared transport registry, validation must include signature-rejection canaries before broad integration execution.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-072-01 | SCN-072-A01 | integration | `tests/integration/assistant/whatsapp_webhook_test.go` | Planned: signed text webhook becomes canonical WhatsApp AssistantMessage | `./smackerel.sh test integration` | Yes |
| TP-072-02 | SCN-072-A02 | unit | `internal/whatsapp/assistant_adapter/verify_test.go` | Planned: invalid Meta signatures reject before facade invocation | `./smackerel.sh test unit` | No |
| TP-072-03 | SCN-072-A01 | integration | `tests/integration/assistant/transport_identity_test.go` | Planned: WhatsApp phone subject hash resolves canonical user without raw phone persistence | `./smackerel.sh test integration` | Yes |
| TP-072-04 | SCN-072-A06 | unit | `internal/config/assistant_whatsapp_test.go` | Planned: enabled WhatsApp with missing access token fails loud | `./smackerel.sh test unit` | No |
| TP-072-05 | SCN-072-A02 | e2e-api | `tests/e2e/assistant/whatsapp_signature_e2e_test.go` | Planned regression: unsigned live webhook never reaches facade | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] WhatsApp adapter foundation, webhook verification, identity lookup, route mount, and fail-loud config satisfy SCN-072-A01, SCN-072-A02, and SCN-072-A06.
- [ ] TP-072-01 passes with evidence of canonical `AssistantMessage{Transport:"whatsapp"}` and WhatsApp message id idempotency field.
- [ ] TP-072-02 passes with evidence that invalid signatures stop before facade invocation.
- [ ] TP-072-03 passes with evidence that identity lookup uses hashed external subjects only.
- [ ] TP-072-04 passes with evidence of named NO-DEFAULTS failure for missing enabled credential.
- [ ] TP-072-05 passes against the live stack as the persistent signature-rejection regression.
- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not run implementation, build, lint, or test commands. Each unchecked item requires current-session execution evidence before completion.

---

## Scope 2: Response Renderer And Capture Acknowledgement

**Status:** Not Started  
**Depends On:** Scope 1  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-072-A03 — AssistantResponse renders to the correct WhatsApp message type
  Given a turn produces an AssistantResponse containing a disambiguation prompt with three choices
  When the adapter renders it
  Then the user receives a WhatsApp interactive-button message with three buttons
  And the button payloads carry disambiguation_ref and choice index so the next inbound turn round-trips

Scenario: SCN-072-A04 — Mapping is exhaustive with a text fallback
  Given an AssistantResponse shape that has no specific WhatsApp render
  When the adapter renders it
  Then the user receives a text message containing the response's text representation
  And the response is never silently dropped

Scenario: SCN-072-A05 — Capture-as-fallback acknowledgement is identical to Telegram and HTTP
  Given the facade returns AssistantResponse with CaptureRoute = true
  When the WhatsApp adapter renders the response
  Then the capture path is invoked exactly once
  And the WhatsApp reply contains the same "saved-as-idea" acknowledgement shape Telegram and HTTP emit

Scenario: SCN-072-A09 — No silent template wrapping for normal replies
  Given a reply to a user within the WhatsApp 24-hour customer-service window
  When the adapter renders the response
  Then a free-form text or interactive message is sent
  And a WhatsApp message template is NOT used
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected User-Visible Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-072-A03 | WhatsApp Business chat | disambiguation response with three choices | user sees assistant reply | three human-readable buttons appear; payload ids stay opaque | TP-072-06 |
| SCN-072-A05 | WhatsApp Business chat | capture route response | user sends fallback-eligible turn | canonical saved-as-idea acknowledgement appears | TP-072-08 |
| SCN-072-A09 | WhatsApp Business chat | in-window normal reply | user receives response | no message template is used for free-form response | TP-072-09 |

### Implementation Plan

- Implement pure renderer golden tests for text, sources, disambiguation buttons/list, confirm card, reset acknowledgement, capture acknowledgement, and unknown response shape fallback.
- Invoke the capture hook exactly once before rendering capture acknowledgement when `CaptureRoute == true`.
- Encode disambiguation refs/choice indexes and confirm payloads in opaque WhatsApp payload ids, never visible labels.
- Enforce template policy so normal in-window replies use free-form text or interactive messages only.
- Add render metrics for text/list/buttons/template-required/error without raw user text or phone numbers.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `AssistantResponse` renderer vocabulary | WhatsApp must render by shape, not scenario | TP-072-06 and TP-072-07 renderer golden tests |
| Capture hook | Capture-as-fallback stays transport-neutral | TP-072-08 integration canary |
| Template policy | Free-form replies are not silently wrapped | TP-072-09 unit guard |

### Change Boundary

- **Allowed file families:** `internal/whatsapp/assistant_adapter/render*.go`, WhatsApp render fixtures/tests, capture hook invocation in WhatsApp adapter only.
- **Excluded surfaces:** facade decision logic, capture policy internals owned by spec 074, Telegram/HTTP renderer copy, scenario implementations.
- **Containment rule:** response-shape gaps must be resolved in the shared contract, not by scenario-specific WhatsApp branches.

### Impact-Aware Validation

No configured impact map exists. Renderer changes require unit golden coverage plus live e2e-api confirmation that user-visible message families match the response shape.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-072-06 | SCN-072-A03 | unit | `internal/whatsapp/assistant_adapter/render_golden_test.go` | Planned: three-choice disambiguation renders as WhatsApp buttons | `./smackerel.sh test unit` | No |
| TP-072-07 | SCN-072-A04 | unit | `internal/whatsapp/assistant_adapter/render_golden_test.go` | Planned: unknown response shape renders text or explicit observable error, never drop | `./smackerel.sh test unit` | No |
| TP-072-08 | SCN-072-A05 | integration | `tests/integration/assistant/whatsapp_capture_test.go` | Planned: CaptureRoute invokes capture exactly once and sends canonical acknowledgement | `./smackerel.sh test integration` | Yes |
| TP-072-09 | SCN-072-A09 | unit | `internal/whatsapp/assistant_adapter/template_policy_test.go` | Planned: normal in-window replies do not use WhatsApp templates | `./smackerel.sh test unit` | No |
| TP-072-10 | SCN-072-A03 | e2e-api | `tests/e2e/assistant/whatsapp_render_e2e_test.go` | Planned regression: live disambiguation response sends expected WhatsApp interactive type | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Renderer, capture acknowledgement, template policy, and render metrics satisfy SCN-072-A03, SCN-072-A04, SCN-072-A05, and SCN-072-A09.
- [ ] TP-072-06 through TP-072-10 pass with evidence and without client-visible scenario-specific branching.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute renderer, integration, e2e, or quality commands.

---

## Scope 3: Round-Trip Controls And Idempotent Retries

**Status:** Not Started  
**Depends On:** Scope 2  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-072-A08 — Disambiguation / confirm / reset round-trip identically
  Given a prior WhatsApp turn produced a confirm prompt
  When the user taps "accept"
  Then the facade resolves the confirm exactly as the Telegram and HTTP paths would
  And the post-confirm response is rendered to WhatsApp text or interactive type per the mapping table

Scenario: SCN-072-A10 — Idempotency on Meta retries
  Given Meta retries the same webhook with the same WhatsApp message id
  When the adapter processes both deliveries
  Then the facade observes exactly one turn for that TransportMessageID
  And no duplicate scenario invocation or capture occurs
```

### Implementation Plan

- Translate WhatsApp interactive reply payload ids back to canonical disambiguation, confirm, and reset request shapes.
- Reuse facade idempotency by setting `TransportMessageID` to the Meta message id for every accepted inbound message.
- Add duplicate-delivery handling that returns success to Meta while avoiding a second facade/capture/scenario invocation.
- Add cross-transport parity fixtures that drive WhatsApp, Telegram, and HTTP through the same facade scenario where available.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Facade idempotency | One `TransportMessageID` means one turn | TP-072-12 Meta retry canary |
| Confirm/disambiguation controls | WhatsApp payloads must round-trip to canonical shapes | TP-072-11 e2e row |
| Capture path | Duplicate retries must not duplicate capture | TP-072-13 integration row |

### Change Boundary

- **Allowed file families:** WhatsApp inbound translator, idempotency integration tests, facade test harness fixtures.
- **Excluded surfaces:** core scenario implementations, `AssistantMessage` struct changes, Telegram/HTTP behavior changes except comparison tests.
- **Containment rule:** retry handling cannot mint new transport ids or call facade twice for the same Meta id.

### Impact-Aware Validation

No project impact map is configured. Idempotency touches shared conversation state, so run the retry canary before broad E2E validation.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-072-11 | SCN-072-A08 | e2e-api | `tests/e2e/assistant/whatsapp_roundtrip_test.go` | Planned regression: confirm/disambiguation/reset round-trip through shared facade | `./smackerel.sh test e2e` | Yes |
| TP-072-12 | SCN-072-A10 | integration | `tests/integration/assistant/whatsapp_idempotency_test.go` | Planned: duplicate Meta delivery invokes facade once | `./smackerel.sh test integration` | Yes |
| TP-072-13 | SCN-072-A10 | e2e-api | `tests/e2e/assistant/whatsapp_retry_dedup_e2e_test.go` | Planned regression: duplicate webhook does not duplicate scenario or capture | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] WhatsApp interactive reply translation, facade parity, and Meta retry idempotency satisfy SCN-072-A08 and SCN-072-A10.
- [ ] TP-072-11, TP-072-12, and TP-072-13 pass with evidence.
- [ ] Shared Infrastructure Impact Sweep confirms no duplicate facade/capture calls for retried Meta message ids.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.

---

## Scope 4: Independent Disable And Operator Status

**Status:** Not Started  
**Depends On:** Scope 3  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-072-A07 — Disabling WhatsApp leaves Telegram and HTTP unaffected
  Given Telegram, HTTP, and WhatsApp are all registered and operating
  When the operator sets assistant.transports.whatsapp.enabled = false and restarts
  Then the WhatsApp ingress is removed
  And Telegram and HTTP continue to serve user turns with no regressions
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | Operator Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-072-A07 | WhatsApp Transport Status | all transports registered | restart with WhatsApp disabled | status shows WhatsApp disabled while Telegram and HTTP health remains healthy | TP-072-14 |

### Implementation Plan

- Gate route mount and adapter registration on explicit `assistant.transports.whatsapp.enabled`.
- Ensure disabled state removes only WhatsApp ingress and leaves Telegram/HTTP facade registration intact.
- Add health/metrics counters for enabled state, credential readiness, signature rejections, idempotent retries, render family counts, and last send status.
- Add operator status query/dashboard hooks without adding a mutating config UI.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Transport registry | Disabling one adapter must not unregister others | TP-072-14 integration row |
| Operator metrics/status | Status must distinguish disabled from credential error | TP-072-15 monitoring row |
| Runtime config | Missing secrets fail loud only when enabled | TP-072-16 config regression |

### Change Boundary

- **Allowed file families:** transport registration/wiring, config validation tests, WhatsApp status metrics, monitoring tests.
- **Excluded surfaces:** user-chat render copy, facade scenario behavior, credential values, deploy-specific hostnames or endpoints.
- **Containment rule:** no operator-coupled environment values may be added to this repo.

### Impact-Aware Validation

No project impact map is configured. Runtime registration changes require integration coverage for each affected transport and live e2e-api smoke coverage.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-072-14 | SCN-072-A07 | integration | `tests/integration/assistant/transport_disable_test.go` | Planned: disabling WhatsApp leaves Telegram and HTTP healthy | `./smackerel.sh test integration` | Yes |
| TP-072-15 | SCN-072-A07 | integration | `tests/integration/monitoring/whatsapp_transport_status_test.go` | Planned: status metrics distinguish disabled, credential-ready, and rejection counts | `./smackerel.sh test integration` | Yes |
| TP-072-16 | SCN-072-A07 | e2e-api | `tests/e2e/assistant/transport_disable_e2e_test.go` | Planned regression: live Telegram/HTTP turns still work when WhatsApp ingress is absent | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Independent disable behavior and operator status satisfy SCN-072-A07.
- [ ] TP-072-14 through TP-072-16 pass with evidence.
- [ ] Change boundary is respected and no deploy-specific target values are introduced.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.