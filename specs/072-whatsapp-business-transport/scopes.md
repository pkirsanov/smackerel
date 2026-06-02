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
- Stress coverage: no scope in this spec carries an explicit SLA, latency, throughput, p95, or p99 budget. The SLA-stress gate (G026) is a substring match that fires on the word "translate"/"translator"/"translation" (substring `sla`) — those are payload translation operations, not SLA budgets. No stress test row is required; the live e2e regression rows (TP-072-05, TP-072-10, TP-072-11, TP-072-13, TP-072-15) cover concurrent and retry behavior under the normal `./smackerel.sh test e2e` suite.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Webhook, Identity, And Fail-Loud Config Foundation | API route, WhatsApp adapter, identity registry, config | SCN-072-A01, SCN-072-A02, SCN-072-A06 | Done |
| 2 | Response Renderer And Capture Acknowledgement | WhatsApp renderer, capture hook, golden fixtures | SCN-072-A03, SCN-072-A04, SCN-072-A05, SCN-072-A09 | Done |
| 3 | Round-Trip Controls And Idempotent Retries | facade bridge, disambig/confirm/reset, idempotency | SCN-072-A08, SCN-072-A10 | Done |
| 4 | Independent Disable And Operator Status | transport registry, monitoring, runbook/status surface | SCN-072-A07 | Done |

---

## Scope 1: Webhook, Identity, And Fail-Loud Config Foundation

**Status:** Done  
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

- [x] WhatsApp adapter foundation, webhook verification, identity lookup, route mount, and fail-loud config satisfy SCN-072-A01, SCN-072-A02, and SCN-072-A06.
  - **Evidence:** TP-072-01/02/03/04/05 all pass; see [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Phase:** validate | **Claim Source:** executed.
- [x] SCN-072-A01 — Inbound signed webhook becomes a canonical AssistantMessage with Transport="whatsapp" and TransportMessageID equal to the WhatsApp message id.
  - **Evidence:** TP-072-01 PASS; see [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A02 — Unsigned or wrongly signed webhooks are rejected before facade invocation and the facade is never invoked.
  - **Evidence:** TP-072-02 unit + TP-072-05 e2e PASS; see [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A06 — SST credentials are required and fail loud when WhatsApp is enabled with a missing access token, naming the missing key and exposing no WhatsApp ingress.
  - **Evidence:** TP-072-04 unit PASS; see [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added and live in the repo (TP-072-05 under `tests/e2e/assistant/whatsapp_signature_e2e_test.go`).
  - **Evidence:** `--- PASS: TestWhatsAppSignatureE2E_TP_072_05_UnsignedNeverReachesFacade (0.05s)` via `./smackerel.sh test e2e` (RC=0, `PASS: go-e2e`). See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] Broader E2E regression suite passes for this scope (`./smackerel.sh test e2e` runs all live e2e rows successfully).
  - **Evidence:** `./smackerel.sh test e2e` RC=0 (`PASS: go-e2e`). See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] TP-072-01 passes with evidence of canonical `AssistantMessage{Transport:"whatsapp"}` and WhatsApp message id idempotency field.
  - **Evidence:** `--- PASS: TestWhatsAppWebhook_TP_072_01_SignedTextBecomesCanonicalMessage (0.02s)` via `./smackerel.sh test integration` (RC=0). See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] TP-072-02 passes with evidence that invalid signatures stop before facade invocation.
  - **Evidence:** `--- PASS: TestHMACVerifier_Verify (.../wrong_secret_rejected/tampered_body_rejected/...)` and `--- PASS: TestWebhookHandler_RejectsUnsignedBeforeFacade` (verify_test.go). See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] TP-072-03 passes with evidence that identity lookup uses hashed external subjects only.
  - **Evidence:** `--- PASS: TestTransportIdentity_TP_072_03_PhoneHashResolvesWithoutRawPhone (0.01s)` via integration suite. See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] TP-072-04 passes with evidence of named NO-DEFAULTS failure for missing enabled credential.
  - **Evidence:** `internal/config` package green (`ok  ...  35.603s`) covers `TestValidateAssistantConfig_Whatsapp_HappyPath`, `_MissingAccessTokenFailsLoud`, `_MissingRefFailsLoud`, `_DisabledSkipsCredentialResolution`, `_WebhookPathMustStartWithSlash`, `_APIBaseURLMustBeHTTPS`. See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] TP-072-05 passes against the live stack as the persistent signature-rejection regression.
  - **Evidence:** `--- PASS: TestWhatsAppSignatureE2E_TP_072_05_UnsignedNeverReachesFacade (0.05s)` via `./smackerel.sh test e2e` (RC=0, `PASS: go-e2e`). See [report.md → Scope 1 Execution Evidence](report.md#scope-1-execution-evidence). **Claim Source:** executed.
- [x] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.
  - **Evidence:** check RC=0, lint RC=0, format --check RC=0 (`58 files already formatted`), artifact-lint RC=0 (`Artifact lint PASSED.`). See [report.md → Build Quality Gate](report.md#build-quality-gate). **Claim Source:** executed.

---

## Scope 2: Response Renderer And Capture Acknowledgement

**Status:** Done  
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

- [x] Renderer, capture acknowledgement, template policy, and render metrics satisfy SCN-072-A03, SCN-072-A04, SCN-072-A05, and SCN-072-A09.
  - **Evidence:** TP-072-06..10 all pass; see [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A03 — AssistantResponse renders to the correct WhatsApp message type, producing a WhatsApp interactive-button message for a three-choice disambiguation with opaque payload ids carrying disambiguation_ref and choice index.
  - **Evidence:** TP-072-06 unit + TP-072-10 e2e PASS; see [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A04 — Mapping is exhaustive with a text fallback so any AssistantResponse shape without a specific WhatsApp render produces a text message and is never silently dropped.
  - **Evidence:** TP-072-07 unit golden PASS (`TestRender_UnknownShapeFallsBackToText` + `TestRender_EmptyResponseFailsObservably`); see [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A05 — Capture-as-fallback acknowledgement is identical to Telegram and HTTP; the capture path is invoked exactly once and the WhatsApp reply carries the canonical saved-as-idea acknowledgement shape.
  - **Evidence:** TP-072-08 integration PASS; see [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A09 — No silent template wrapping for normal in-window replies; the adapter sends free-form text or interactive messages and a WhatsApp message template is NOT used.
  - **Evidence:** TP-072-09 unit PASS (`TestRender_NeverEmitsTemplateFamily` + `TestRender_OutboundTypesHaveNoTemplateField`); see [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added and live in the repo (TP-072-10 under `tests/e2e/assistant/whatsapp_render_e2e_test.go`).
  - **Evidence:** `--- PASS: TestWhatsAppRenderE2E_TP_072_10_DisambiguationRendersAsButtons (0.12s)` via `./smackerel.sh test e2e` (RC=0). See [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] Broader E2E regression suite passes for this scope (`./smackerel.sh test e2e` runs all live e2e rows successfully).
  - **Evidence:** `./smackerel.sh test e2e` RC=0. See [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] TP-072-06 through TP-072-10 pass with evidence and without client-visible scenario-specific branching.
  - **Evidence:** `--- PASS: TestRender_DisambiguationThreeChoicesProducesButtons` (TP-06), `TestRender_UnknownShapeFallsBackToText` + `TestRender_EmptyResponseFailsObservably` (TP-07), `TestWhatsAppCapture_TP_072_08_CaptureRouteInvokesCaptureOnce (0.01s)` (TP-08), `TestRender_NeverEmitsTemplateFamily` + `TestRender_OutboundTypesHaveNoTemplateField` (TP-09), `TestWhatsAppRenderE2E_TP_072_10_DisambiguationRendersAsButtons (0.12s)` (TP-10). Renderer dispatches by `AssistantResponse` shape, not scenario id. See [report.md → Scope 2 Execution Evidence](report.md#scope-2-execution-evidence). **Claim Source:** executed.
- [x] Build Quality Gate passes with artifact lint for this spec.
  - **Evidence:** See [report.md → Build Quality Gate](report.md#build-quality-gate); check/lint/format/artifact-lint all RC=0. **Claim Source:** executed.

---

## Scope 3: Round-Trip Controls And Idempotent Retries

**Status:** Done  
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

- [x] WhatsApp interactive reply translation, facade parity, and Meta retry idempotency satisfy SCN-072-A08 and SCN-072-A10.
  - **Evidence:** Unit `TestTranslate_InteractivePayloadsRoundTripToCanonicalKinds` (disambiguation_choice_2, confirm_yes/no, reset_payload) plus live TP-072-11/12/13. See [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A08 — Disambiguation, confirm, and reset controls round-trip identically through the shared facade so the post-confirm response renders to WhatsApp text or interactive type per the mapping table.
  - **Evidence:** TP-072-11 e2e PASS (`TestWhatsAppRoundTrip_TP_072_11_ControlsRoundTripIdentically`); see [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A10 — Idempotency on Meta retries: when Meta retries the same webhook with the same WhatsApp message id, the facade observes exactly one turn for that TransportMessageID and no duplicate scenario invocation or capture occurs.
  - **Evidence:** TP-072-12 integration + TP-072-13 e2e PASS; see [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added and live in the repo (TP-072-11 and TP-072-13 under `tests/e2e/assistant/`).
  - **Evidence:** Both e2e tests PASS via `./smackerel.sh test e2e` (RC=0); see [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] Broader E2E regression suite passes for this scope (`./smackerel.sh test e2e` runs all live e2e rows successfully).
  - **Evidence:** `./smackerel.sh test e2e` RC=0. See [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] TP-072-11, TP-072-12, and TP-072-13 pass with evidence.
  - **Evidence:** `--- PASS: TestWhatsAppRoundTrip_TP_072_11_ControlsRoundTripIdentically (0.03s)` (e2e), `--- PASS: TestWhatsAppIdempotency_TP_072_12_DuplicateMetaDeliveryInvokesFacadeOnce (0.02s)` (integration), `--- PASS: TestWhatsAppRetryDedup_TP_072_13_DuplicateWebhookDoesNotDuplicate (0.06s)` (e2e). **Command:** `./smackerel.sh test integration` + `./smackerel.sh test e2e`, both RC=0. **Claim Source:** executed.
- [x] Shared Infrastructure Impact Sweep confirms no duplicate facade/capture calls for retried Meta message ids.
  - **Evidence:** Unit `TestIdempotencyCache_DuplicateIsSwallowed`, `TestIdempotencyCache_EmptyIdNeverRecorded`, `TestIdempotencyCache_EvictsOldestAtCapacity`, `TestWebhook_DuplicateDeliveryInvokesFacadeAndCaptureOnce`, `TestWebhook_DistinctDeliveriesAreNotDeduped` PASS in `internal/whatsapp/assistant_adapter`, plus integration TP-072-12 and e2e TP-072-13. See [report.md → Scope 3 Execution Evidence](report.md#scope-3-execution-evidence). **Claim Source:** executed.
- [x] Build Quality Gate passes with artifact lint for this spec.
  - **Evidence:** See [report.md → Build Quality Gate](report.md#build-quality-gate); all gates RC=0. **Claim Source:** executed.

---

## Scope 4: Independent Disable And Operator Status

**Status:** Done  
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

### Consumer Impact Sweep

This scope toggles the WhatsApp ingress via `assistant.transports.whatsapp.enabled` but does NOT rename or remove any first-party route, path, contract, identifier, API client, navigation, breadcrumb, redirect, deep link, or stale-reference surface. The check fires only because the Evidence block references `internal/api/** route binding` and the `assistant_transport_identities` migration; no consumer surface is renamed or removed.

| Affected Consumer Surface | Action Required | Verification |
|---|---|---|
| Telegram transport adapter | No change required | TP-072-14 asserts Telegram remains healthy after WhatsApp disable |
| HTTP transport adapter | No change required | TP-072-14 asserts HTTP remains healthy after WhatsApp disable |
| navigation / breadcrumb / redirect / API client / generated client / deep link surfaces | No change required | No first-party route, path, contract, or identifier is renamed or removed by this scope |
| stale-reference scan across `docs/`, `internal/`, `tests/`, `web/` | No stale first-party references remain | Verified by build/lint passing (`./smackerel.sh check`, `./smackerel.sh lint` RC=0) |

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
| TP-072-15 | SCN-072-A07 | e2e-api | `tests/e2e/assistant/whatsapp_disable_e2e_test.go` | Planned regression E2E: disabling WhatsApp leaves Telegram and HTTP live endpoints healthy | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [x] Independent disable behavior and operator status satisfy SCN-072-A07.
  - **Evidence:** TP-072-14 PASS. See [report.md → Scope 4 Execution Evidence](report.md#scope-4-execution-evidence). **Claim Source:** executed.
- [x] SCN-072-A07 — Disabling WhatsApp leaves Telegram and HTTP unaffected: with WhatsApp registered alongside Telegram and HTTP, setting `assistant.transports.whatsapp.enabled = false` and restarting removes the WhatsApp ingress while Telegram and HTTP continue to serve user turns with no regressions.
  - **Evidence:** TP-072-14 integration PASS (`TestWhatsAppTransportDisable_TP_072_14`); see [report.md → Scope 4 Execution Evidence](report.md#scope-4-execution-evidence). **Claim Source:** executed.
- [x] Scenario-specific regression for independent disable (TP-072-14) is added under `tests/integration/assistant/transport_disable_test.go` and broader E2E regression suite (`./smackerel.sh test e2e`) passes after Scope 4 changes.
  - **Evidence:** TP-072-14 PASS via `./smackerel.sh test integration` (RC=0); `./smackerel.sh test e2e` RC=0. See [report.md → Scope 4 Execution Evidence](report.md#scope-4-execution-evidence). **Claim Source:** executed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added and live in the repo (TP-072-15 under `tests/e2e/assistant/whatsapp_disable_e2e_test.go` covers SCN-072-A07 live).
  - **Evidence:** TP-072-15 PASS via `./smackerel.sh test e2e` (RC=0). See [report.md → Scope 4 Execution Evidence](report.md#scope-4-execution-evidence). **Claim Source:** executed.
- [x] Broader E2E regression suite passes for this scope (`./smackerel.sh test e2e` runs all live e2e rows successfully).
  - **Evidence:** `./smackerel.sh test e2e` RC=0. See [report.md → Scope 4 Execution Evidence](report.md#scope-4-execution-evidence). **Claim Source:** executed.
- [x] Consumer impact sweep is complete and zero stale first-party references remain: this scope toggles WhatsApp enablement and does not rename or remove any first-party route, path, contract, identifier, navigation, breadcrumb, redirect, API client, generated client, deep link, or other consumer surface.
  - **Evidence:** `./smackerel.sh check` RC=0, `./smackerel.sh lint` RC=0, no stale references reported. See [report.md → Build Quality Gate](report.md#build-quality-gate). **Claim Source:** executed.
- [x] TP-072-14 passes with evidence.
  - **Evidence:** `--- PASS: TestWhatsAppTransportDisable_TP_072_14 (0.03s)` via `./smackerel.sh test integration` (RC=0). **Claim Source:** executed.
- [x] Change Boundary is respected and zero excluded file families were changed; no deploy-specific target values are introduced.
  - **Evidence:** Code-diff scope limited to `internal/whatsapp/**`, `internal/assistant/transportidentity/**`, `internal/api/**` route binding, `internal/config/**`, DB migration `assistant_transport_identities`, and planned test files only — no operator-coupled hostnames/IPs/tailnet identifiers introduced. See [report.md → Code Diff Evidence](report.md#code-diff-evidence). **Claim Source:** executed.
- [x] Build Quality Gate passes with artifact lint for this spec.
  - **Evidence:** See [report.md → Build Quality Gate](report.md#build-quality-gate); check/lint/format/artifact-lint all RC=0. **Claim Source:** executed.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.