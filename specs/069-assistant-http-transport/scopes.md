# Scopes: 069 Assistant HTTP Transport

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

1. **HTTP Adapter Contract and Wire Schema** — Implement the second concrete `TransportAdapter`, synchronous `POST /api/assistant/turn`, request/response schema validation, facade invocation, capture rendering hook, and golden schema pinning. `foundation:true`.
2. **Auth, Scope, and Limit Rejections** — Mount the route behind bearer auth, required scope middleware, body-size cap, rate limit, and explicit CORS config before facade invocation.
3. **Disambiguation and Confirmation Round Trips** — Prove callback-style turns over HTTP resolve pending disambiguation and confirmation state exactly like Telegram.
4. **Reset and Capture Rendering** — Prove reset clears pending web state and capture-as-fallback is invoked exactly once with the same acknowledgement shape.
5. **Transport Parity, Hint Neutrality, and Live E2E Suite** — Prove Telegram and HTTP share the same facade path, hints are telemetry-only, and the assistant E2E suite runs over HTTP without Telegram.

### New Types and Signatures

- `internal/assistant/httpadapter.HTTPAdapter` implementing the existing `contracts.TransportAdapter` contract.
- `POST /api/assistant/turn` request schema v1 with `schema_version`, `transport_message_id`, `kind`, `transport_hint`, text, confirm, disambiguation, and client context fields.
- Response schema v1 with transport echo, status, body, sources, confirm card, disambiguation prompt, error cause, capture route, trace, facade flag, and emitted timestamp.
- Error response schema for auth, scope, schema, body-size, rate-limit, and facade errors.
- Required SST keys under `assistant.transports.http.*`, including `required_scope` resolved against spec 060 scope grammar.

### Validation Checkpoints

- After Scope 1, unit and contract tests prove schema v1 is pinned and accepted turns invoke `Facade.Handle` exactly once. The HTTP-route e2e tests for spec 068 compiler scenarios SCN-068-A01, SCN-068-A02, SCN-068-A03, SCN-068-A04, SCN-068-A05, SCN-068-A06, SCN-068-A07, and SCN-068-A09 also land in this scope because the assistant HTTP ingress they exercise first becomes real here.
- After Scope 1c-bis, unit tests prove every `assistant.transports.http.*` SST key (`HTTPEnabled`, `HTTPSchemaVersion`, `HTTPBodySizeMaxBytes`, `HTTPRateLimitPerUserPerMinute`, `HTTPConversationTTL`, `HTTPRequiredScope`, `HTTPCORSAllowedOrigins`, `HTTPTransportHintAllowlist`) is required, typed, and fails loud on absent/empty values with no fallback defaults. This unblocks Scope 1d's `cfg.Assistant.HTTP*` consumption and Scope 2's middleware enforcement.
- After Scope 1d, an integration test against the live `cmd/core` wiring proves `POST /api/assistant/turn` returns HTTP 200 (not 503) for a valid bearer turn, because `wireAssistantFacade` constructs `*HTTPAdapter` via `httpadapter.NewHTTPAdapter` and calls `assistantHTTPHandler.SetAdapter` + `SetMiddleware` exactly once per process. This unblocks every live-stack e2e row in Scopes 1, 2, 3, 4, and 5 and resolves F074-04B-ASSISTANT-HTTP-LATE-BIND.
- After Scope 2, integration tests prove 401, 403, 413, and 429 reject before the facade and missing HTTP transport config fails loud.
- After Scope 3, HTTP E2E tests prove disambiguation and confirmation round trips preserve pending state and user identity.
- After Scope 4, HTTP E2E tests prove reset and capture rendering match the shared assistant response model.
- After Scope 5, parity and live-stack E2E tests prove no scenario branches on transport and no Telegram account is required.

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | HTTP Adapter Contract and Wire Schema | None | adapter package, route, schema, golden tests, spec 068 compiler HTTP e2e tests (A01-A07, A09) | unit, e2e-api, Regression E2E, cross-spec e2e | route calls facade once; schema v1 pinned; SCN-068-A01/A02/A03/A04/A05/A06/A07/A09 HTTP e2e land here | Not Started |
| 1c-bis | HTTP Transport SST Contract | 1 | `internal/config` AssistantConfig HTTP block, `config/smackerel.yaml` schema, generated env keys | unit | every `assistant.transports.http.*` SST key required, typed, fail-loud (NO-DEFAULTS) | Not Started |
| 1d | HTTPAdapter Construction and LateBound Binding (rework) | 1, 1c-bis | `cmd/core/wiring_assistant_facade.go`, `cmd/core/services.go`, `internal/assistant/httpadapter` constructor surface | integration, Regression E2E | `wireAssistantFacade` constructs `*HTTPAdapter` and calls `LateBoundHandler.SetAdapter`+`SetMiddleware`; `POST /api/assistant/turn` returns 200 not 503 against live wiring | Not Started |
| 2 | Auth, Scope, and Limit Rejections | 1, 1c-bis, 1d, specs/060 | auth middleware, scope claim, rate/body/CORS enforcement | integration, Regression E2E | 401/403/413/429 pre-facade rejection using SST values from 1c-bis | Not Started |
| 3 | Disambiguation and Confirmation Round Trips | 1, 2 | pending state, callback request kinds, confirmation gate | e2e-api, integration, Regression E2E | disambig/confirm parity with Telegram | Not Started |
| 4 | Reset and Capture Rendering | 1, 2, 3 | reset kind, capture path, acknowledgement shape | e2e-api, integration, Regression E2E | reset clears state; capture invoked once | Not Started |
| 5 | Transport Parity, Hint Neutrality, and Live E2E Suite | 1a, 1b, 1c, 2, 3, 4, specs/068 | parity tests, hint validation, assistant E2E suite | unit, integration, e2e-api, stress | no transport branching; HTTP drives live assistant suite | In Progress (live integration + e2e GREEN pending foreign blocker resolution) |

---

## Scope 1: HTTP Adapter Contract and Wire Schema

**Status:** Not Started  
**Depends On:** None  
**Tags:** foundation:true  
**Surfaces:** `internal/assistant/httpadapter/`, API router mount, request/response schema validation, golden contract tests, assistant facade dependency injection.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A01 — HTTP turn returns the same response Telegram would
  Given the HTTP transport adapter is enabled with a valid bearer token
  When the user POSTs { transport_message_id, kind: "text", text: "/ask what is the weather in barcelona" } to /api/assistant/turn
  Then the response is HTTP 200 with a JSON body matching the AssistantResponse schema
  And the response body contains the same scenario invocation result a Telegram /ask would have produced for the same compiled intent
  And the response sets Transport = "web" and TransportMessageID echoing the request

Scenario: SCN-069-A07 — Schema is pinned by a golden contract test
  Given the request and response wire schemas declared in this spec
  When the contract test runs
  Then any change to the JSON field names, types, or required fields fails the test unless schema_version is bumped
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A01 | Valid token and HTTP transport enabled | POST text turn | HTTP 200 response matches schema, echoes transport id, and calls shared facade once | e2e-api | `report.md#scope-1` |
| SCN-069-A07 | Golden schema fixtures exist | Run contract test | Field name/type/required-field drift fails unless schema version changes | unit | `report.md#scope-1` |

### Implementation Plan

- Add `internal/assistant/httpadapter` that translates HTTP request JSON to `contracts.AssistantMessage{Transport:"web"}` and renders `contracts.AssistantResponse` as response schema v1.
- Mount `POST /api/assistant/turn` under existing API routing without changing the capability-layer interface.
- Validate schema version, transport message id, message kind, callback refs, and response serialization.
- Ensure accepted turns invoke `Facade.Handle` exactly once.
- Add golden fixtures for request and response schemas.
- **Shared Infrastructure Impact Sweep:** API router, assistant facade, transport adapter registry, and response model are shared surfaces. Canary rows validate Telegram adapter behavior and existing facade tests before broad suite reruns.
- **Change Boundary:** allowed file families are HTTP adapter package, API route mount, schema/golden tests, and assistant HTTP E2E tests. Excluded surfaces are Telegram adapter internals, scenario logic, compiler implementation, and streaming transports.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Adapter translation | unit | SCN-069-A01 | `internal/assistant/httpadapter/adapter_test.go` | `TestHTTPAdapterTranslatesTextTurnToAssistantMessage` | `./smackerel.sh test unit` | No |
| Golden schema | unit | SCN-069-A07 | `internal/assistant/httpadapter/golden_contract_test.go` | `TestHTTPAssistantTurnGoldenContractV1` | `./smackerel.sh test unit` | No |
| Canary: Telegram adapter + facade unaffected | integration | SCN-069-A01, SCN-069-A07 | `tests/integration/assistant/http_adapter_canary_test.go` | `TestHTTPAdapterCanary_TelegramAdapterAndFacadeUnchanged` | `./smackerel.sh test integration` | Yes |
| Facade invocation | integration | SCN-069-A01 | `tests/integration/api/assistant_http_turn_test.go` | `TestAssistantHTTPTurnInvokesFacadeExactlyOnce` | `./smackerel.sh test integration` | Yes |
| Regression E2E: text turn | e2e-api | SCN-069-A01 | `tests/e2e/assistant/http_turn_test.go` | `TestAssistantHTTPE2E_TextTurnReturnsSchemaValidResponse` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: schema pin | e2e-api | SCN-069-A07 | `tests/e2e/assistant/http_turn_test.go` | `TestAssistantHTTPE2E_ResponseSchemaMatchesV1Contract` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: compiler malformed JSON over HTTP | e2e-api | SCN-068-A06 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 1) | `tests/e2e/assistant/intent_compiler_http_test.go` | `TestIntentCompilerE2E_MalformedJSONBlocksRoutingAndCaptures` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: operational bypass over live transport | e2e-api | SCN-068-A07 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 1) | `tests/e2e/assistant/intent_compiler_http_test.go` | `TestIntentCompilerE2E_OperationalCommandsBypassCompilerOverLiveTransport` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: weather compiles before route over HTTP | e2e-api | SCN-068-A01 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 2) | `tests/e2e/assistant/intent_compiler_http_test.go` | `TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: retrieval receives structured context over HTTP | e2e-api | SCN-068-A02 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 2) | `tests/e2e/assistant/intent_compiler_http_test.go` | `TestIntentCompilerE2E_RetrievalReceivesStructuredContext` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: read intents never route from raw text over HTTP | e2e-api | SCN-068-A01, SCN-068-A02 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 2) | `tests/e2e/assistant/intent_compiler_http_test.go` | `TestIntentCompilerE2E_ReadIntentsNeverRouteFromRawTextOnly` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: list write requires confirmation over HTTP | e2e-api | SCN-068-A03 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 3) | `tests/e2e/assistant/intent_side_effect_test.go` | `TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: annotation slots from compiled intent over HTTP | e2e-api | SCN-068-A04 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 3) | `tests/e2e/assistant/annotation_intent_test.go` | `TestAnnotationIntentE2E_SlotsComeFromCompiledIntent` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: write/state never bypass confirm gate over HTTP | e2e-api | SCN-068-A03, SCN-068-A04, SCN-068-A09 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 3) | `tests/e2e/assistant/intent_side_effect_test.go` | `TestIntentCompilerE2E_WriteAndStateMutationNeverBypassConfirmGate` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: Springfield clarification over HTTP | e2e-api | SCN-068-A05 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 4) | `tests/e2e/assistant/intent_clarify_test.go` | `TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation` | `./smackerel.sh test e2e` | Yes |
| Cross-spec e2e: ambiguous never routes weather over HTTP | e2e-api | SCN-068-A05 (authored in [specs/068](../068-structured-intent-compiler/scopes.md) Scope 4) | `tests/e2e/assistant/intent_clarify_test.go` | `TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup` | `./smackerel.sh test e2e` | Yes |

**Cross-spec ownership note:** SCN-068-A01, SCN-068-A02, SCN-068-A03, SCN-068-A04, SCN-068-A05, SCN-068-A06, SCN-068-A07, and SCN-068-A09 remain authored in [spec 068](../068-structured-intent-compiler/scopes.md) because they describe compiler behavior, not transport mechanics. Their HTTP-route e2e proof lives in this spec because the assistant HTTP ingress they exercise first exists here. Spec 068's manifest/test-plan record these as `deferredTests` pointing back to the same file paths and test IDs used above; the scenario IDs are preserved verbatim across both specs so traceability stays intact.

### Definition of Done

- [ ] `POST /api/assistant/turn` accepts schema v1 text turns and invokes the shared facade exactly once.
- [ ] Request and response schema v1 are pinned by golden contract tests.
- [ ] Response echoes `Transport="web"` and the client-supplied `transport_message_id`.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Change Boundary is respected and zero excluded file families are changed.
- [ ] Scenario-specific E2E regression coverage exists for SCN-069-A01 and SCN-069-A07.
- [ ] Cross-spec HTTP-route e2e coverage exists in `tests/e2e/assistant/intent_compiler_http_test.go`, `tests/e2e/assistant/intent_side_effect_test.go`, `tests/e2e/assistant/annotation_intent_test.go`, and `tests/e2e/assistant/intent_clarify_test.go` for spec 068 scenarios SCN-068-A01, SCN-068-A02, SCN-068-A03, SCN-068-A04, SCN-068-A05, SCN-068-A06, SCN-068-A07, and SCN-068-A09, with scenario IDs preserved verbatim from spec 068.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 1c-bis: HTTP Transport SST Contract

**Status:** Not Started  
**Depends On:** Scope 1  
**Tags:** foundation:true, contract  
**Surfaces:** `internal/config` (AssistantConfig HTTP block), `config/smackerel.yaml` schema additions, generated env keys under `assistant.transports.http.*`.

### Rationale

Scope 1d's wiring block reads `cfg.Assistant.HTTPEnabled` and the broader `cfg.Assistant.HTTP*` struct, and Scope 2's middleware enforces values pulled from the same struct. Those fields were originally folded into Scope 2's plan as "Validate explicit CORS origin config and conversation TTL config with fail-loud errors" plus the unit row `TestAssistantHTTPTransportConfigRequiresEverySSTKey`. Promoting the SST contract into its own foundation scope removes the SCOPE-1d → SCOPE-2 cycle: SCOPE-1d cannot construct `*HTTPAdapter` without the typed config, but SCOPE-2 cannot author its rejection middleware before SCOPE-1d binds. SCOPE-1c-bis lands the contract once so SCOPE-1d and SCOPE-2 both consume it.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A13 — HTTP transport SST keys are required and fail loud
  Given config/smackerel.yaml declares assistant.transports.http with HTTPEnabled, HTTPSchemaVersion, HTTPBodySizeMaxBytes, HTTPRateLimitPerUserPerMinute, HTTPConversationTTL, HTTPRequiredScope, HTTPCORSAllowedOrigins, and HTTPTransportHintAllowlist
  When config generation or assistant config load runs
  Then every key is required, typed, and validated (no defaults, no fallbacks)
  And omitting or empty-stringing any key fails loud with a named error citing the missing key
  And the loaded AssistantConfig exposes the typed HTTP block consumed by Scope 1d wiring and Scope 2 middleware
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A13 | Test fixtures with full and partial HTTP config blocks | Load AssistantConfig; remove each key in turn | Full block parses to typed struct; each removal fails loud naming the missing key | unit | `report.md#scope-1c-bis` |

### Implementation Plan

- Add typed `HTTPTransportConfig` (or equivalent) on `AssistantConfig` with fields `HTTPEnabled`, `HTTPSchemaVersion`, `HTTPBodySizeMaxBytes`, `HTTPRateLimitPerUserPerMinute`, `HTTPConversationTTL`, `HTTPRequiredScope`, `HTTPCORSAllowedOrigins`, `HTTPTransportHintAllowlist`.
- Extend `config/smackerel.yaml` schema and the config-generation pipeline so every key emits to `config/generated/{dev,test}.env` and the loader rejects missing/empty values with named errors per `smackerel-no-defaults` SST policy.
- Validate value shapes (numeric ranges, duration parsing, non-empty allowlists, scope spelling resolvable against spec 060) at load time; no runtime fallbacks.
- Do NOT touch route mount, adapter, middleware implementation, or wiring — those are SCOPE-1d and SCOPE-2 surfaces.
- **Shared Infrastructure Impact Sweep:** AssistantConfig is consumed by Telegram adapter wiring and existing assistant code paths. Canary: existing `internal/config` tests and Telegram adapter config tests remain GREEN after the new HTTP block lands.
- **Change Boundary:** allowed file families: `internal/config/*assistant*`, `config/smackerel.yaml`, config generation scripts, and the new SST contract test file. Excluded: `internal/assistant/httpadapter/`, `cmd/core/wiring*.go`, every API route mount, every middleware implementation.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Config fail-loud | unit | SCN-069-A13 | `internal/config/assistant_http_transport_test.go` | `TestAssistantHTTPTransportConfigRequiresEverySSTKey` | `./smackerel.sh test unit` | No |
| Canary: existing AssistantConfig | unit | SCN-069-A13 | `internal/config/assistant_test.go` (existing) | existing AssistantConfig tests remain GREEN | `./smackerel.sh test unit` | No |

### Definition of Done

- [ ] `AssistantConfig` exposes a typed `HTTPTransportConfig` (or equivalent) with all eight required HTTP keys.
- [ ] Every key is required at load time; omitting or empty-stringing any key fails loud with a named error.
- [ ] `config/smackerel.yaml` and generated env files include all eight keys with no fallback defaults.
- [ ] Value validation (numeric ranges, duration parsing, scope spelling, non-empty allowlists) runs at load time.
- [ ] Shared Infrastructure Impact Sweep canary tests pass (existing AssistantConfig + Telegram wiring tests remain GREEN).
- [ ] Change Boundary honored: no changes to httpadapter, wiring, route mount, or middleware.
- [ ] Scenario-specific regression coverage exists for SCN-069-A13.
- [ ] `./smackerel.sh test unit` and artifact lint pass for this spec.

---

## Scope 1d: HTTPAdapter Construction and LateBound Binding (Rework)

**Status:** Not Started  
**Depends On:** Scope 1, Scope 1c-bis  
**Tags:** rework, finding:F-069-ADAPTER-NOT-BOUND, resolves:F074-04B-ASSISTANT-HTTP-LATE-BIND  
**Surfaces:** `cmd/core/wiring_assistant_facade.go`, `cmd/core/services.go` (the `assistantHTTPHandler *httpadapter.LateBoundHandler` field), `internal/assistant/httpadapter` constructor surface.

### Rework Rationale (Finding F-069-ADAPTER-NOT-BOUND)

Scope 1 shipped the `*HTTPAdapter` type, the `LateBoundHandler` shim, the route mount, and the schema/golden tests. Production wiring in `cmd/core/wiring.go` constructs `httpadapter.NewLateBoundHandler()` and mounts it at `POST /api/assistant/turn`, but `cmd/core/wiring_assistant_facade.go::wireAssistantFacade` only constructs the Telegram `assistant_adapter` and **never** calls `httpadapter.NewHTTPAdapter` and never calls `svc.assistantHTTPHandler.SetAdapter` / `SetMiddleware`. Result: every live HTTP turn returns HTTP 503 because `LateBoundHandler` has no backing adapter. This breaks every live-stack e2e/integration row in Scopes 1–5 (the symptom that surfaced as F074-04B-ASSISTANT-HTTP-LATE-BIND under spec 074's test-infra triage). The fix is wiring-only inside this spec's owned surfaces; no schema, contract, or middleware change is required.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A12 — HTTP adapter is bound in production wiring
  Given cmd/core is started with assistant.enabled=true and assistant.transports.http.enabled=true
  And a valid bearer token with the required assistant-turn scope
  When the user POSTs a schema-v1 text turn to /api/assistant/turn
  Then the response is HTTP 200 with a schema-v1 AssistantResponse body
  And the response is NOT HTTP 503 ("assistant HTTP adapter not bound")
  And wireAssistantFacade has called LateBoundHandler.SetAdapter exactly once with a non-nil *HTTPAdapter
  And wireAssistantFacade has called LateBoundHandler.SetMiddleware exactly once with the auth/scope/limit middleware chain
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A12 | Live cmd/core wired with HTTP transport enabled | POST schema-v1 text turn with valid bearer | HTTP 200 schema-v1 response; LateBoundHandler bound exactly once | integration | `report.md#scope-1d` |

### Implementation Plan

- In `cmd/core/wiring_assistant_facade.go::wireAssistantFacade`, after the Telegram adapter section, add an HTTP transport bind block guarded by `cfg.Assistant.HTTPEnabled` (closed-vocabulary SST key landed by Scope 1c-bis; SCOPE-1d MUST NOT proceed before SCOPE-1c-bis is Done).
- Construct `*HTTPAdapter` via `httpadapter.NewHTTPAdapter(httpadapter.Options{...})` populating: `Facade: facade`, `Tracer: svc.assistantTracer`, `Now: time.Now`, `Config: cfg.Assistant.HTTP` (transport config struct), capture invoker, response-render hooks, and any other Options fields the Scope 1 constructor requires. Fail loud on any nil/missing dependency (G028/G029) — do NOT supply defaults.
- Call `svc.assistantHTTPHandler.SetAdapter(adapter)` exactly once.
- Call `svc.assistantHTTPHandler.SetMiddleware(<Scope 2 middleware chain>)` exactly once. When Scope 2 has not landed yet, set a fail-loud placeholder middleware that rejects every request with HTTP 500 + named error, so the wiring is structurally complete but cannot accidentally serve pre-auth traffic; Scope 2 replaces it.
- Log `"assistant HTTP adapter bound"` with structured fields mirroring the existing Telegram bind log.
- **Containment rule:** no changes to `internal/assistant/httpadapter/*` exported API, no changes to route paths, no changes to the request/response schema, no changes to `assistant_adapter` (Telegram). Wiring-only.
- **Shared Infrastructure Impact Sweep:** the bind site is shared by every HTTP transport scenario. Canary rows reuse `tests/integration/assistant/http_adapter_canary_test.go` (Scope 1) to prove Telegram adapter and facade behavior remain unchanged after the new bind block runs.
- **Change Boundary:** allowed file families: `cmd/core/wiring_assistant_facade.go`, the new integration test file, and (if a constructor surface gap is found) a single additive change to `internal/assistant/httpadapter` constructor wiring that does NOT change the exported request/response contract. Excluded: schema files, route mount, Telegram adapter, middleware implementations (Scope 2), every test under `tests/e2e/assistant/`.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Wiring integration | integration | SCN-069-A12 | `tests/integration/assistant/http_adapter_bind_test.go` | `TestAssistantHTTPAdapterIsBoundInProductionWiring_ReturnsHTTP200NotHTTP503` | `./smackerel.sh test integration` | Yes |
| Canary: Telegram bind unaffected | integration | SCN-069-A12 | `tests/integration/assistant/http_adapter_bind_test.go` | `TestAssistantHTTPAdapterBindLeavesTelegramAdapterAndFacadeUnchanged` | `./smackerel.sh test integration` | Yes |
| Regression E2E: turn returns 200 | e2e-api | SCN-069-A12, SCN-069-A01 | `tests/e2e/assistant/http_turn_test.go` | `TestAssistantHTTPE2E_TextTurnReturnsSchemaValidResponse` (re-asserts non-503 after bind) | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] `wireAssistantFacade` constructs `*HTTPAdapter` via `httpadapter.NewHTTPAdapter` with every required Option populated; nil/missing dependencies fail loud at startup.
- [ ] `svc.assistantHTTPHandler.SetAdapter(adapter)` is called exactly once during wiring.
- [ ] `svc.assistantHTTPHandler.SetMiddleware(...)` is called exactly once during wiring (placeholder until Scope 2 lands the real chain).
- [ ] `POST /api/assistant/turn` returns HTTP 200 (not 503) for a valid bearer schema-v1 turn against the live wiring, proven by `TestAssistantHTTPAdapterIsBoundInProductionWiring_ReturnsHTTP200NotHTTP503`.
- [ ] Telegram adapter binding and facade behavior remain unchanged (canary row passes).
- [ ] Containment rule honored: no change to schema, route mount, Telegram adapter, or middleware implementations.
- [ ] Finding F-069-ADAPTER-NOT-BOUND closed with linked evidence; spec 074 finding F074-04B-ASSISTANT-HTTP-LATE-BIND is recorded as auto-resolved by this scope in `report.md#scope-1d`.
- [ ] `./smackerel.sh test integration` and `./smackerel.sh test e2e` for the linked tests pass; artifact lint passes for this spec.

---

## Scope 2: Auth, Scope, and Limit Rejections

**Status:** Not Started  
**Depends On:** Scope 1, Scope 1c-bis, Scope 1d, specs/060-bearer-auth-scope-claim  
**Surfaces:** bearer auth group, `assistant.turn`/configured scope claim, body-size cap, rate limiter, CORS enforcement, error response schema. Consumes the typed HTTP transport config produced by Scope 1c-bis; does NOT own the SST contract.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A02 — Auth is mandatory
  Given no bearer token is provided
  When the user POSTs to /api/assistant/turn
  Then the response is HTTP 401
  And the facade is never invoked

Scenario: SCN-069-A10 — Rate limit and body-size cap from SST
  Given assistant.transports.http.rate_limit_per_user_per_minute and body_size_max_bytes are set
  When a user exceeds either limit
  Then the request is rejected with the standard 429 / 413 status
  And no facade invocation occurs
  And missing config keys fail loud at startup (NO-DEFAULTS)
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A02 | Request lacks bearer token | POST turn | HTTP 401 with safe error body and `facade_invoked=false` | integration | `report.md#scope-2` |
| SCN-069-A10 | Request exceeds configured limit | POST oversized or high-rate requests | HTTP 413/429 with safe error body and no facade invocation | integration | `report.md#scope-2` |

### Implementation Plan

- Mount the route behind existing bearer auth and spec 060 scope middleware using the implementation spelling selected in `assistant.transports.http.required_scope` (typed value supplied by Scope 1c-bis).
- Apply body-size cap and per-user rate limiter (values supplied by Scope 1c-bis) before JSON decode and facade invocation.
- Apply explicit CORS origin enforcement and conversation TTL using the typed values from Scope 1c-bis; this scope does NOT (re)validate the SST contract — that ownership is Scope 1c-bis.
- Return stable error bodies that never echo tokens or secret headers.
- **Shared Infrastructure Impact Sweep:** auth middleware, scope middleware, API router order, and rate limiter are shared surfaces. Canary rows validate existing auth endpoints and scope middleware before broad suite reruns.
- **Change Boundary:** allowed file families are route middleware wiring, HTTP transport config, rate/body limit tests, and error schema tests. Excluded surfaces are token minting logic and unrelated API routes.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Missing bearer | integration | SCN-069-A02 | `tests/integration/api/assistant_http_auth_test.go` | `TestAssistantHTTPAuth_MissingBearerReturns401BeforeFacade` | `./smackerel.sh test integration` | Yes |
| Missing scope | integration | SCN-069-A02 | `tests/integration/api/assistant_http_auth_test.go` | `TestAssistantHTTPAuth_MissingTurnScopeReturns403BeforeFacade` | `./smackerel.sh test integration` | Yes |
| Limit rejection | integration | SCN-069-A10 | `tests/integration/api/assistant_http_limits_test.go` | `TestAssistantHTTPLimitsRejectBeforeFacadeInvocation` | `./smackerel.sh test integration` | Yes |
| Regression E2E: pre-facade errors | e2e-api | SCN-069-A02, SCN-069-A10 | `tests/e2e/assistant/http_error_test.go` | `TestAssistantHTTPE2E_PreFacadeErrorsDoNotInvokeFacade` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] Missing or invalid bearer token returns 401 before facade invocation.
- [ ] Missing required assistant-turn scope returns 403 before facade invocation.
- [ ] Body-size and rate-limit rejections return 413/429 before facade invocation.
- [ ] HTTP transport config keys are consumed from the typed SST contract landed in Scope 1c-bis (this scope does not own the fail-loud unit test).
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Scenario-specific E2E regression coverage exists for SCN-069-A02 and SCN-069-A10.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 3: Disambiguation and Confirmation Round Trips

**Status:** Not Started  
**Depends On:** Scope 1, Scope 2  
**Surfaces:** HTTP request kinds `disambiguation` and `confirm`, pending assistant state, confirmation gate, response rendering.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A03 — Disambiguation prompt round-trips over HTTP
  Given a prior turn produced an AssistantResponse with a DisambiguationPrompt
  When the user POSTs { kind: "disambiguation", disambiguation_ref, disambiguation_choice: 2 }
  Then the facade resolves the choice exactly as Telegram would
  And the next response is the chosen scenario's invocation result

Scenario: SCN-069-A04 — Confirm prompt round-trips over HTTP
  Given a prior turn produced an AssistantResponse with a ConfirmCard
  When the user POSTs { kind: "confirm", confirm_ref, confirm_choice: "accept" }
  Then the side-effect-bearing action executes
  And the response carries the same post-confirm result Telegram would produce
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A03 | Pending disambiguation exists for web transport | POST choice turn | Choice resolves for the same user/transport and final response matches selected candidate | e2e-api | `report.md#scope-3` |
| SCN-069-A04 | Pending confirmation exists for web transport | POST accept turn | Gated action executes once and response carries post-confirm result | e2e-api | `report.md#scope-3` |

### Implementation Plan

- Validate callback request kinds and require matching refs/choices before facade invocation.
- Preserve pending state isolation by `(user_id, transport="web")` and reject stale or cross-user refs.
- Ensure confirmation acceptance executes side-effect-bearing actions only through the existing facade confirmation path.
- **Shared Infrastructure Impact Sweep:** pending state, confirmation refs, and disambiguation refs are shared assistant state. Canary rows validate Telegram callback behavior and existing facade pending-state tests.
- **Change Boundary:** allowed file families are HTTP adapter callback validation, assistant pending-state tests, and HTTP E2E fixtures. Excluded surfaces are domain action implementations and Telegram callback internals.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Disambiguation E2E | e2e-api | SCN-069-A03 | `tests/e2e/assistant/http_disambiguation_test.go` | `TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn` | `./smackerel.sh test e2e` | Yes |
| Confirm E2E | e2e-api | SCN-069-A04 | `tests/e2e/assistant/http_confirm_test.go` | `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` | `./smackerel.sh test e2e` | Yes |
| Pending-state isolation | integration | SCN-069-A03, SCN-069-A04 | `tests/integration/assistant/http_pending_state_test.go` | `TestAssistantHTTPPendingStateIsScopedByUserAndTransport` | `./smackerel.sh test integration` | Yes |
| Regression E2E: stale callback | e2e-api | SCN-069-A03, SCN-069-A04 | `tests/e2e/assistant/http_confirm_test.go` | `TestAssistantHTTPE2E_StaleCallbackRefDoesNotExecuteAction` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] HTTP disambiguation callback resolves pending choices exactly once for the same user and transport.
- [ ] HTTP confirmation callback executes side-effect-bearing actions only through the existing confirm gate.
- [ ] Stale and cross-user callback refs are rejected without action execution.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Scenario-specific E2E regression coverage exists for SCN-069-A03 and SCN-069-A04.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 4: Reset and Capture Rendering

**Status:** Not Started  
**Depends On:** Scope 1, Scope 2, Scope 3  
**Surfaces:** HTTP reset kind, assistant conversation state, capture route invocation, response acknowledgement rendering.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A05 — Reset clears pending state
  When the user POSTs { kind: "reset" }
  Then the facade drops any pending confirm/disambig state for (user, transport=web)
  And the response is the canonical reset acknowledgement

Scenario: SCN-069-A06 — Capture-as-fallback acknowledgement is identical to Telegram
  Given the facade returns AssistantResponse with CaptureRoute = true
  When the HTTP adapter renders the response
  Then the local capture path is invoked exactly once
  And the HTTP response body includes the same "saved-as-idea" acknowledgement shape Telegram emits
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A05 | Pending state exists for web transport | POST reset turn | Pending state clears and reset acknowledgement is returned | e2e-api | `report.md#scope-4` |
| SCN-069-A06 | Facade returns capture route | POST unknown/open-ended turn | Capture path invokes once and acknowledgement matches shared response shape | e2e-api | `report.md#scope-4` |

### Implementation Plan

- Map HTTP `kind: reset` to the existing facade reset behavior for `transport="web"`.
- Invoke capture path exactly once when `AssistantResponse.CaptureRoute` is true.
- Render capture acknowledgement through shared response fields, not transport-specific scenario text.
- **Shared Infrastructure Impact Sweep:** reset and capture are cross-transport assistant state surfaces. Canary rows validate Telegram reset/capture behavior and existing capture tests.
- **Change Boundary:** allowed file families are HTTP adapter reset/capture handling and tests. Excluded surfaces are capture pipeline internals, Telegram rendering internals, and unrelated artifact persistence.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Reset E2E | e2e-api | SCN-069-A05 | `tests/e2e/assistant/http_reset_test.go` | `TestAssistantHTTPE2E_ResetClearsWebPendingState` | `./smackerel.sh test e2e` | Yes |
| Capture E2E | e2e-api | SCN-069-A06 | `tests/e2e/assistant/http_capture_test.go` | `TestAssistantHTTPE2E_CaptureRouteInvokesCaptureOnceAndAcknowledges` | `./smackerel.sh test e2e` | Yes |
| Reset integration | integration | SCN-069-A05 | `tests/integration/assistant/http_pending_state_test.go` | `TestAssistantHTTPResetClearsOnlyWebTransportState` | `./smackerel.sh test integration` | Yes |
| Regression E2E: capture parity | e2e-api | SCN-069-A06 | `tests/e2e/assistant/http_capture_test.go` | `TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] HTTP reset clears pending state for the authenticated user and web transport only.
- [ ] Capture-as-fallback invokes the existing capture path exactly once.
- [ ] HTTP response body renders the shared capture acknowledgement shape.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Scenario-specific E2E regression coverage exists for SCN-069-A05 and SCN-069-A06.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 5: Transport Parity, Hint Neutrality, and Live E2E Suite

**Status:** In Progress (live integration + e2e GREEN pending foreign blocker resolution)  
**Depends On:** Scope 1, Scope 2, Scope 3, Scope 4, specs/068-structured-intent-compiler  
**Surfaces:** adapter registry, transport hint validation, parity guard, assistant live E2E suite, no transport branching policy.

### Gherkin Scenarios

```gherkin
Scenario: SCN-069-A08 — Telegram and HTTP share one facade instance
  Given Telegram and HTTP adapters are both registered in the same process
  When a turn arrives on each transport for the same user
  Then both invocations hit the same Facade.Handle code path
  And both record turns in the same assistant_conversations row family (keyed by (UserID, Transport))
  And no scenario or routing decision branches on transport name

Scenario: SCN-069-A09 — Transport hint reserved but generic
  Given the request includes transport_hint = "mobile" or "bridge"
  When the adapter translates the request
  Then transport_hint is recorded for telemetry only and does NOT alter scenario selection, tool allowlist, or response shape
  And an unknown transport_hint is rejected by the closed-vocabulary check

Scenario: SCN-069-A11 — E2E suite drives the live stack without Telegram
  Given the live test stack is up
  When tests/e2e/assistant/* runs
  Then weather, retrieval, recipe/list, open-knowledge, disambig, confirm, reset, capture-fallback, and intent-compiler clarify scenarios all pass against the HTTP route
  And no test in the suite requires a real Telegram account or the Telegram bot to be running
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-069-A08 | Telegram and HTTP adapters registered | Exercise one turn per transport | Same facade seam and row family are observed; scenario code does not inspect transport | integration | `report.md#scope-5` |
| SCN-069-A09 | Valid and unknown transport hints | POST turn with `mobile`, `bridge`, then invalid hint | Valid hints affect telemetry only; invalid hint rejects before facade | unit + e2e-api | `report.md#scope-5` |
| SCN-069-A11 | Live test stack is running | Run assistant E2E suite | Suite covers assistant flows over HTTP with no Telegram dependency | e2e-api | `report.md#scope-5` |

### Implementation Plan

- Register HTTP as a peer adapter and assert parity with Telegram at the facade boundary.
- Validate `transport_hint` against the configured allowlist and record it only as telemetry metadata.
- Add spec 067 guard coverage preventing scenario/facade/executor branches on `AssistantMessage.Transport`; adapter and audit layers are the only permitted inspectors.
- Retarget canonical assistant E2E coverage to HTTP while keeping existing Telegram coverage intact.
- **Consumer Impact Sweep:** consumers include spec 031 live-stack testing docs, spec 043 real-LLM E2E assumptions, spec 061 adapter contract, spec 067 guards, frontend contract consumers, and assistant E2E fixtures.
- **Change Boundary:** allowed file families are HTTP adapter parity tests, assistant E2E suite, transport hint validation, and docs owned by the relevant docs phase. Excluded surfaces are new frontend UI implementation and capability-layer contract changes.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Transport parity | integration | SCN-069-A08 | `tests/integration/assistant/transport_parity_test.go` | `TestAssistantTransportParity_TelegramAndHTTPUseSameFacadePath` | `./smackerel.sh test integration` | Yes |
| Hint validation | unit | SCN-069-A09 | `internal/assistant/httpadapter/transport_hint_test.go` | `TestTransportHintIsClosedVocabularyAndTelemetryOnly` | `./smackerel.sh test unit` | No |
| Transport branch guard | guard | SCN-069-A08 | `tests/integration/policy/transport_branch_guard_test.go` | `TestTransportBranchGuardRejectsScenarioTransportBranching` | `./smackerel.sh test integration` | Yes |
| Live assistant suite | e2e-api | SCN-069-A11 | `tests/e2e/assistant/http_live_stack_test.go` | `TestAssistantHTTPE2E_LiveStackWithoutTelegramCoversCanonicalFlows` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: hint neutrality | e2e-api | SCN-069-A09 | `tests/e2e/assistant/http_turn_test.go` | `TestAssistantHTTPE2E_TransportHintDoesNotChangeScenarioOrResponseShape` | `./smackerel.sh test e2e` | Yes |
| Stress smoke | stress | SCN-069-A11 | `tests/stress/assistant/http_turn_stress_test.go` | `TestAssistantHTTPStress_PerUserRateLimitAndConversationTTLRemainStable` | `./smackerel.sh test stress` | Yes |

### Definition of Done

- [x] Telegram and HTTP adapters invoke the same facade path and share the same conversation row family keyed by user and transport. **Phase:** implement. **Claim Source:** interpreted. **Evidence:** `report.md#scope-5--transport-parity-hint-neutrality-and-live-e2e-suite` — `tests/integration/assistant/transport_parity_test.go::TestAssistantTransportParity_TelegramAndHTTPUseSameFacadePath` exercises both transports through the same `countingParityFacade.Handle` seam and asserts `parityStore` partitions rows by the `(UserID, Transport)` tuple (telegram + web rows independent; `DeleteByKey` on telegram leaves web intact). Live execution blocked by the foreign untracked `internal/assistant/facade_intent_trace.go` (spec 071) — see `report.md#scope-5-uncertainty-declaration`.
- [x] `transport_hint` is closed-vocabulary telemetry only and cannot alter routing, tools, response shape, or side-effect behavior. **Phase:** implement. **Claim Source:** executed. **Evidence:** `report.md#scope-5--transport-parity-hint-neutrality-and-live-e2e-suite` — `TestTransportHintIsClosedVocabularyAndTelemetryOnly` PASS (4 sub-tests). Proves every allowed hint lands only in `TransportMetadata["transport_hint"]`; empty hint is accepted without populating metadata; unknown `carrier-pigeon` is rejected before facade with a stable error naming `transport_hint` + `allowlist`; adversarial assertion confirms the hint never leaks into `msg.Text` or `msg.Kind`. The wire-level enforcement lives in `(TurnRequest).Validate(cfg HTTPTransportConfig)` which rejects unknown hints before `Translate` returns an `AssistantMessage`.
- [x] Policy guard prevents scenario/facade/executor transport branching outside adapter and audit layers. **Phase:** implement. **Claim Source:** executed. **Evidence:** `report.md#scope-5--transport-parity-hint-neutrality-and-live-e2e-suite` — `internal/assistant/intent/policyguard/transport_branch.go` ships `ReportTransportBranchViolations` with a closed `AllowedTransportInspectors` allowlist (httpadapter/, telegram/, whatsapp/, contracts/, context/, confirm/, transportidentity/, capturefallback/, metrics/, audit.go, bridge.go, facade.go) and the canonical `TransportBranchViolation` phrase. Five unit tests PASS including the real-repo cleanliness check (`TestReportTransportBranchViolations_RealAssistantSubtreeIsClean` PASS — zero findings under `internal/assistant`). The integration mirror `tests/integration/policy/transport_branch_guard_test.go` is authored and structurally identical to `tests/integration/policy/intent_bypass_guard_test.go`; live integration run blocked by foreign untracked `internal/config/assistant_*` files — see `report.md#scope-5-uncertainty-declaration`.
- [ ] Canonical assistant E2E suite runs over HTTP against the live stack without Telegram account or bot dependency. **Phase:** implement. **Claim Source:** not-run. **Uncertainty Declaration:** 2026-06-01 wrapper attempt `./smackerel.sh test e2e --go-run '^TestAssistantHTTPE2E_'` aborted at `go-e2e-stack-start (exit=1)` after two consecutive `container smackerel-test-smackerel-core-1 is unhealthy` failures (full trace in `/tmp/s069-e2e.log`). The `e2e`-tagged image built successfully but the core container's healthcheck failed within the wrapper's start window. See `report.md#scope-5-rerun-attempt-2026-06-01`. The test file `tests/e2e/assistant/http_live_stack_test.go::TestAssistantHTTPE2E_LiveStackWithoutTelegramCoversCanonicalFlows` is `//go:build e2e` and `go vet -tags e2e` clean.
- [x] Consumer Impact Sweep proves specs 031, 043, 061, 067, assistant E2E fixtures, and frontend contract consumers are aligned. **Phase:** implement. **Claim Source:** interpreted. **Evidence:** spec 031 live-stack testing docs unchanged (HTTP adapter rides the existing `./smackerel.sh test e2e` wrapper without new live-stack flags); spec 043 real-LLM E2E assumptions preserved (defensive-skip pattern matches `tests/e2e/drive/helpers.go`); spec 061 adapter contract unchanged (`HTTPAdapter` implements `contracts.TransportAdapter` per SCOPE-1a canary); spec 067 guards unchanged and now strengthened by the new transport-branch guard which lives in the same `policyguard` package; assistant E2E fixtures extended (8 new files in `tests/e2e/assistant/`); frontend contract consumers unaffected (response schema v1 pinned by `golden_contract_test.go`, no new wire fields).
- [x] Scenario-specific E2E regression coverage exists for SCN-069-A08, SCN-069-A09, and SCN-069-A11. **Phase:** implement. **Claim Source:** executed. **Evidence:** `report.md#scope-5--transport-parity-hint-neutrality-and-live-e2e-suite` — SCN-069-A08 covered by `tests/integration/assistant/transport_parity_test.go` + `internal/assistant/intent/policyguard/transport_branch_realrepo_test.go`; SCN-069-A09 covered by `internal/assistant/httpadapter/transport_hint_test.go` + `tests/e2e/assistant/http_turn_test.go::TestAssistantHTTPE2E_TransportHintDoesNotChangeScenarioOrResponseShape` (plus pre-existing spec 073 `tests/e2e/assistant/transport_hint_parity_test.go` regression that covers the same invariant end-to-end); SCN-069-A11 covered by `tests/e2e/assistant/http_live_stack_test.go`. `go vet -tags e2e` clean.
- [ ] Broader E2E regression suite passes. **Phase:** implement. **Claim Source:** not-run. **Uncertainty Declaration:** 2026-06-01 wrapper attempt `./smackerel.sh test e2e --go-run '^TestAssistantHTTPE2E_'` aborted at `go-e2e-stack-start (exit=1)` with `container smackerel-test-smackerel-core-1 is unhealthy` — same wrapper-startup failure under the `e2e` build tag. Every `./smackerel.sh test e2e --go-run ...` invocation in this session would hit the same abort before any test executes. See `report.md#scope-5-rerun-attempt-2026-06-01`.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, `./smackerel.sh test stress`, and artifact lint pass for this spec. **Phase:** implement. **Claim Source:** partial. **Evidence:** Targeted runs GREEN — `go test ./internal/assistant/httpadapter/ -run TestTransportHintIsClosedVocabularyAndTelemetryOnly` PASS; `go test ./internal/assistant/intent/policyguard/ -run TestReportTransportBranchViolations -v` PASS (5 sub-tests including real-repo cleanliness); `go test -tags stress ./tests/stress/assistant/ -v` PASS (p95=40µs, far under 50ms budget); `go vet -tags e2e ./tests/e2e/assistant/` PASS; `go vet -tags stress ./tests/stress/assistant/` PASS. 2026-06-01 wrapper-driven `./smackerel.sh test e2e --go-run '^TestAssistantHTTPE2E_'` aborted at `go-e2e-stack-start (exit=1)` with `smackerel-test-smackerel-core-1 is unhealthy` — see `report.md#scope-5-rerun-attempt-2026-06-01`. Scope remains **In Progress** pending wrapper-driven e2e GREEN once the e2e-tagged core healthcheck issue is resolved (cross-spec runtime/test-infra concern, not spec 069 owned).
