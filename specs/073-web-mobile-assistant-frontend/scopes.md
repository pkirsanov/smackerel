# Scopes: 073 Web/Mobile Assistant Frontend Client

## Execution Outline

### Phase Order

1. **Scope 1 — Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config:** generate web and shared-mobile models from spec 069, define one mobile foundation for iPhone/iOS plus Android, enforce transport hints, and add config/auth storage canaries.
2. **Scope 2 — Web Chat Vertical Slice:** build `/assistant` composer, same-origin authenticated turn POST, web retry with stable `transport_message_id`, response live region, and keyboard accessibility.
3. **Scope 3 — Shared Mobile Chat Vertical Slice:** add the planned shared mobile assistant codebase, package iPhone/iOS and Android through thin adapters, prove idempotent retry on both platforms, and validate VoiceOver plus TalkBack.
4. **Scope 4 — Cross-Surface Response Controls, Capture, And Parity:** prove disambiguation, confirm, citations, capture acknowledgement, no scenario branching, and renderer parity across web, iPhone/iOS, and Android.

### New Types & Signatures

- `AssistantTurnClient.send(request: AssistantTurnRequest, retryToken: TransportMessageID) -> AssistantTurnResponse`
- `AssistantResponseRenderer.render(response: AssistantResponseV1) -> RenderDescriptor`
- `AssistantRetryState{transport_message_id, original_request_body, retry_count, status}`
- `AssistantMobilePlatformAdapter{platform, secureSessionHandoff, accessibilityBridge, insets, navigationShell}`
- Web planned files: `web/pwa/assistant.html`, `web/pwa/assistant.js`, generated assistant-turn model artifacts under `web/pwa/generated/`.
- Shared mobile planned boundary: `clients/mobile/assistant/` with shared generated models, state machine, renderer core, and thin `platform/ios/` plus `platform/android/` adapters after the implementation owner selects the compliant shared-mobile runtime.
- Config keys: `web.assistant.enabled`, `web.assistant.backend_base_url`, `web.assistant.schema_version`, `mobile.assistant.enabled`, `mobile.assistant.backend_base_url`, `mobile.assistant.schema_version`, `mobile.assistant.platforms`, `mobile.assistant.auth_mode`.

### Validation Checkpoints

- After Scope 1, schema generation, renderer canary, config, transport-hint, and storage-boundary tests must fail before web or mobile UI depends on incompatible generated types.
- After Scope 2, web E2E must prove an authenticated live turn, retry idempotency, and keyboard/live-region behavior.
- After Scope 3, shared-mobile validation must prove one codebase feeds both iPhone/iOS and Android adapters, retries reuse the same id on both platforms, and VoiceOver/TalkBack labels are equivalent.
- After Scope 4, web/iPhone/iOS/Android fixture and live-path tests must prove response controls, capture acknowledgement, citations, and fallback rendering derive only from response shape.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` sections.
- Scope 1 is `foundation:true` because `AssistantSchemaModels`, `AssistantResponseRenderer`, `AssistantRetryState`, `AssistantA11yContract`, and the shared mobile platform-adapter boundary are reused by web, iPhone/iOS, and Android.
- Planned test files are handoff targets. A `TBD:` file/location means the implementation/test owner must create or select the concrete path after the shared-mobile runtime is chosen; it is not a claim that the file exists today.
- No source, test, config, ML, docs, or runtime files are modified by this planning pass.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config | schema/codegen, shared renderer canary, config, auth carrier boundaries, platform declaration | SCN-073-A02, SCN-073-A08, SCN-073-A11 | Not Started |
| 2 | Web Chat Vertical Slice | web/PWA UI, same-origin POST, retry, web a11y | SCN-073-A01, SCN-073-A03, SCN-073-A09 | Not Started |
| 3 | Shared Mobile Chat Vertical Slice | shared mobile core, iPhone/iOS adapter, Android adapter, mobile retry, VoiceOver/TalkBack | SCN-073-A02, SCN-073-A03, SCN-073-A10, SCN-073-A11 | Not Started |
| 4 | Cross-Surface Response Controls, Capture, And Parity | renderer, disambig, confirm, capture ack, citations, parity fixtures | SCN-073-A04, SCN-073-A05, SCN-073-A06, SCN-073-A07, SCN-073-A08 | Not Started |

---

## Scope 1: Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config

**Status:** Not Started  
**Depends On:** —  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-A02 — Shared mobile client uses generated types from the golden schema
  Given the shared mobile client is built for iPhone/iOS and Android with types generated from the spec 069 golden schema
  When the wire schema is changed in a way that breaks compatibility
  Then the shared mobile build fails at codegen before either mobile platform ships
  And no incompatible iPhone/iOS or Android client ships to users

Scenario: SCN-073-A08 — Closed-vocabulary transport_hint is honored
  Given the web client sends transport_hint = "web" and the shared mobile client sends transport_hint = "mobile" on iPhone/iOS and Android
  When the server processes all three clients
  Then the server-side closed-vocabulary check accepts both values
  And neither value alters scenario selection or response shape

Scenario: SCN-073-A11 — Missing backend base URL fails loud at build/start time
  Given the SST-derived backend base URL is unset for a client build
  When the client is built or initialized
  Then the build or initialization fails loud with a NO-DEFAULTS error naming the missing key
```

### Implementation Plan

- Generate web runtime validators and shared-mobile models from the spec 069 golden schema without local schema extensions.
- Establish the planned shared-mobile boundary under `clients/mobile/assistant/`: shared generated models, turn/retry state machine, response-renderer core, a11y descriptor, and thin platform adapters for iPhone/iOS and Android.
- Add a generated schema model/renderer canary proving the same golden `AssistantResponse` fixture is accepted by web, shared mobile core, iPhone/iOS adapter projection, and Android adapter projection.
- Add client config validation for required web and mobile keys, including `mobile.assistant.platforms` containing both `ios` and `android`, and schema version equality with spec 069 `v1`.
- Enforce `transport_hint` values `web` and `mobile` as telemetry-only values accepted by the existing HTTP transport.
- Establish auth carrier boundaries: web uses existing auth infrastructure; shared mobile core receives only an authorized request capability; platform adapters own secure session handoff and never expose bearer/session material to renderer/core code.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Generated client schema | Web and shared mobile consume the same spec 069 contract | TP-073-01 and TP-073-02 codegen canaries |
| Generated renderer canary | Web, iPhone/iOS, and Android render descriptors stay shape-compatible | TP-073-03 shared fixture canary |
| Shared mobile codebase boundary | One mobile foundation feeds both iPhone/iOS and Android adapters | TP-073-04 platform-set canary |
| Auth/session carrier | Client work must not introduce sensitive client storage in web or shared mobile renderer/core | TP-073-06 storage guard |
| HTTP transport hint | `web`/`mobile` are accepted and telemetry-only across web, iPhone/iOS, and Android | TP-073-05 integration canary |

### Change Boundary

- **Allowed file families:** schema generation scripts/artifacts for assistant turn v1, generated web assistant models, planned `clients/mobile/assistant/**` foundation/adapters after runtime selection, client config validation, targeted integration/unit tests.
- **Excluded surfaces:** new auth primitives, bearer/session storage in localStorage/sessionStorage/IndexedDB/service worker cache/shared mobile renderer state/logs/copy buffers, server route forks, scenario-specific UI logic, separate mobile business implementations.
- **Containment rule:** if implementation cannot keep one shared mobile renderer/state core for iPhone/iOS and Android, stop and route a design amendment instead of splitting behavior.

### Impact-Aware Validation

No project impact map is configured. Because this scope touches generated client schema, platform declarations, and sensitive auth boundaries, validation must include codegen failure tests, platform-set canaries, config fail-loud tests, storage-pattern guards, and a renderer fixture canary before any UI E2E is treated as meaningful.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-01 | SCN-073-A02 | unit/build | `TBD: clients/mobile/assistant shared schema generation test` | Planned: incompatible schema change fails shared mobile codegen for iPhone/iOS and Android | `./smackerel.sh test unit` | No |
| TP-073-02 | SCN-073-A02 | unit/build | `TBD: web/pwa generated assistant schema test` | Planned: web generated validator rejects incompatible schema drift | `./smackerel.sh test unit` | No |
| TP-073-03 | SCN-073-A02 | unit | `TBD: shared assistant renderer fixture canary` (inputs + golden descriptors under `tests/fixtures/assistant_response_v1/`) | Planned: web, shared-mobile core, iOS adapter projection, and Android adapter projection each produce a render descriptor conforming to `render-descriptor-v1.json` (see design.md § Render-Descriptor JSON Schema) and deep-equal the per-fixture golden descriptor for `text_only`, `with_sources`, `disambiguation`, `confirm_accept_decline`, `capture_acknowledgement`, `error_retry`, and `unknown_shape` | `./smackerel.sh test unit` | No |
| TP-073-04 | SCN-073-A02 | unit/build | `TBD: clients/mobile/assistant platform declaration test` | Planned: shared mobile build declares both ios and android targets from one codebase | `./smackerel.sh test unit` | No |
| TP-073-05 | SCN-073-A08 | integration | `TBD: assistant HTTP transport hint integration test` | Planned: web and mobile transport hints are accepted and telemetry-only for web, iPhone/iOS, and Android clients | `./smackerel.sh test integration` | Yes |
| TP-073-06 | SCN-073-A11 | unit | `TBD: web and shared mobile sensitive storage guard tests` | Planned: no web or shared mobile renderer/core path persists bearer/session material | `./smackerel.sh test unit` | No |
| TP-073-07 | SCN-073-A11 | unit/build | `TBD: web and mobile client config fail-loud tests` | Planned: missing backend URL/schema/platform/auth config fails without fallback defaults | `./smackerel.sh test unit` | No |
| TP-073-08 | SCN-073-A08 | e2e-api | `TBD: live assistant transport hint e2e test` | Planned regression: live web/mobile hints do not alter server response shape | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Shared generated schema, one shared mobile foundation, platform declarations, config validation, transport hints, renderer canary, and auth storage boundaries satisfy SCN-073-A02, SCN-073-A08, and SCN-073-A11.
- [ ] TP-073-01 through TP-073-08 pass with current-session evidence.
- [ ] Storage guard proves no sensitive auth/session material is persisted in forbidden web stores or shared mobile renderer/core surfaces.
- [ ] Mobile foundation guard proves iPhone/iOS and Android are produced from one shared mobile codebase with platform adapters only for OS-specific concerns.
- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not run implementation, build, lint, or runtime test commands. Each unchecked item requires current-session execution evidence before completion.

---

## Scope 2: Web Chat Vertical Slice

**Status:** Not Started  
**Depends On:** Scope 1  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-A01 — Web client sends an authenticated turn and renders the response
  Given the web client has a valid same-origin HttpOnly session cookie established by the spec 070 web login flow with the assistant.turn scope
  When the user types a NL message and submits it
  Then the client POSTs to /api/assistant/turn with credentials: "same-origin" and a fresh transport_message_id
  And the response body is rendered: text, citations, and any disambig/confirm/capture controls

Scenario: SCN-073-A03 — Transient network failure retries with the same transport_message_id
  Given the web client POSTs a turn and the request times out
  When the user retries on web
  Then the retry uses the SAME transport_message_id
  And the server returns the same response (deduped)

Scenario: SCN-073-A09 — Web client meets accessibility floor
  Given a screen reader is active on the web client
  When the user submits a turn and the response arrives
  Then the response area announces via an ARIA live region
  And keyboard navigation reaches the composer, send button, disambig choices, and confirm buttons in a sensible order
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-073-A01 | Web Assistant Chat | authenticated web session with assistant.turn scope | submit natural-language text | same-origin POST succeeds and the served PWA markup renders response body/sources/controls per the schema fixture | TP-073-09 |
| SCN-073-A03 | Web Assistant Chat | first POST times out | user (Go test simulation) chooses retry | retry reuses original `transport_message_id`; server observes one deduped turn; no duplicate transcript row in rendered HTML | TP-073-10 |
| SCN-073-A09 | Web Assistant Chat | served PWA assistant route | inspect rendered HTML/ARIA | `aria-live`/`role=status` response region, labelled composer, deterministic tab/focus order across composer/send/disambig/confirm/retry controls are present | TP-073-11 |

### Implementation Plan

- Add the planned web assistant route under `web/pwa/assistant.html` and `web/pwa/assistant.js` using existing PWA style and static serving patterns.
- Implement composer-first screen, transcript rows, same-origin `fetch('/api/assistant/turn', { credentials: 'same-origin' })` carrying the spec 070 HttpOnly session cookie (ratified 2026-06-01), and generated request/response validation. The web client never reads or stores bearer material; no JS-visible bearer fallback is permitted.
- Generate stable `transport_message_id` per submitted web turn and preserve it across retry attempts until the user edits the turn.
- Add ARIA live region, keyboard focus order, error card focus, and source/detail affordances.
- Add no-storage guard for bearer/session material in browser storage APIs.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Static PWA shell | Existing PWA pages and service worker must remain healthy | TP-073-09 Go-driven PWA e2e canary |
| Auth/session middleware | Same-origin assistant calls must not expose bearer tokens to browser storage or logs | TP-073-12 storage/auth guard |
| Retry/idempotency | Web retry must reuse transport id | TP-073-10 timeout retry row |
| Renderer foundation | Web projection must consume shared schema-driven render descriptors | TP-073-03 remains prerequisite |

### Change Boundary

- **Allowed file families:** planned `web/pwa/assistant.html`, planned `web/pwa/assistant.js`, generated assistant schema artifact, web assistant tests, existing style only for necessary assistant selectors.
- **Excluded surfaces:** unrelated PWA pages, service worker cache behavior unless needed for route inclusion, server auth rewrites outside existing middleware, localStorage/sessionStorage/IndexedDB token persistence, mobile-specific implementation.
- **Containment rule:** web client renders by schema response shape only and cannot branch on scenario id, action class, platform, or `transport_hint`.

### Impact-Aware Validation

No project impact map is configured. UI work requires Go-driven PWA e2e against the live assistant HTTP endpoint, static-HTML keyboard/ARIA assertions, retry regression with adversarial id-reuse sub-test, and storage guard coverage before scope completion. Driver-based screen-reader announcement validation (Playwright/axe) is deferred to a separate foundation spec — see design.md Alternatives.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-09 | SCN-073-A01 | e2e-api | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (planned) + `web/pwa/tests/assistant_chat.spec.ts` stub | Planned regression: Go e2e fetches the served PWA assistant route, asserts composer/transcript/source markup, posts an authenticated turn, and asserts response body/sources/controls render per the schema fixture | `./smackerel.sh test e2e` | Yes |
| TP-073-10 | SCN-073-A03 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` (planned) + `web/pwa/tests/assistant_retry.spec.ts` stub | Planned regression: web timeout retry reuses the same `transport_message_id` (server observes one deduped turn); adversarial sub-test fails if the retry mints a fresh id | `./smackerel.sh test e2e` | Yes |
| TP-073-11 | SCN-073-A09 | e2e-api | `tests/e2e/assistant/web_pwa_accessibility_e2e_test.go` (planned) + `web/pwa/tests/assistant_accessibility.spec.ts` stub | Planned: served PWA markup contains `aria-live`/`role=status` response region, labelled composer, and deterministic tab/focus order across composer, send, disambig choices, confirm, and retry controls (DOM + `tabindex` analysis). Driver-based announcement validation deferred to future browser-driver foundation spec | `./smackerel.sh test e2e` | Yes |
| TP-073-12 | SCN-073-A01 | unit | `TBD: web assistant auth storage guard test` | Planned: assistant web client does not read/write bearer tokens in browser storage | `./smackerel.sh test unit` | No |

### Definition of Done — Tiered Validation

- [ ] Web chat composer, authenticated POST, response render, retry state, ARIA live region, and keyboard workflow satisfy SCN-073-A01, SCN-073-A03, and SCN-073-A09.
- [ ] TP-073-09 through TP-073-12 pass with current-session evidence.
- [ ] UI text and controls fit mobile/desktop browser layouts without overlap or scenario-specific branching.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not run implementation or validation commands.

---

## Scope 3: Shared Mobile Chat Vertical Slice

**Status:** Not Started  
**Depends On:** Scope 2  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-A02 — Shared mobile client uses generated types from the golden schema
  Given the shared mobile client is built for iPhone/iOS and Android with types generated from the spec 069 golden schema
  When the wire schema is changed in a way that breaks compatibility
  Then the shared mobile build fails at codegen before either mobile platform ships
  And no incompatible iPhone/iOS or Android client ships to users

Scenario: SCN-073-A03 — Transient network failure retries with the same transport_message_id
  Given the shared mobile client POSTs a turn from iPhone/iOS and Android and the request times out
  When the user retries on each platform
  Then each retry uses the SAME transport_message_id for that platform's original turn
  And the server returns the same response (deduped)

Scenario: SCN-073-A10 — Shared mobile client meets VoiceOver and TalkBack accessibility floor
  Given VoiceOver is enabled on iPhone/iOS and TalkBack is enabled on Android
  When the user submits a turn and the response arrives
  Then the response renders with semantic labels readable by both mobile assistive technologies
  And interactive controls (disambig list, confirm buttons) are focusable and announce their purpose on both mobile platforms

Scenario: SCN-073-A11 — Missing backend base URL fails loud at build/start time
  Given the SST-derived mobile backend base URL is unset for a shared mobile build
  When the mobile client is built or initialized for iPhone/iOS or Android
  Then the build or initialization fails loud with a NO-DEFAULTS error naming the missing key
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-073-A02 | Shared mobile build | selected shared-mobile runtime with generated models | build iPhone/iOS and Android targets | one codebase produces both platform clients and schema drift fails before shipping | TP-073-13 |
| SCN-073-A03 | Shared Mobile Assistant Chat | timeout/5xx on mobile turn | retry from iPhone/iOS and Android | both platforms reuse the original `transport_message_id` | TP-073-14 |
| SCN-073-A10 | Shared Mobile Assistant Chat | VoiceOver and TalkBack enabled in platform validation | submit turn and move focus through response | roles, body, consequence, controls, citations, saved-as-idea, errors, and retry announce equivalently | TP-073-15, TP-073-16 |
| SCN-073-A11 | Shared mobile initialization | missing mobile backend/config key | initialize/build iPhone/iOS and Android clients | fail-loud config error names the missing key and no fallback URL is used | TP-073-17 |

### Implementation Plan

- Select or introduce one owner-approved shared-mobile runtime only if it preserves the `clients/mobile/assistant/` boundary and one shared business UI foundation for iPhone/iOS plus Android.
- Implement shared mobile generated models, request construction, transcript state, idempotent retry state, response-to-render-descriptor mapping, and composer shell.
- Add iPhone/iOS and Android platform adapters limited to secure session handoff, VoiceOver/TalkBack bridge labels, safe areas/insets, navigation shell, system gestures, and packaging.
- Prove both platforms send `transport_hint = "mobile"`, preserve `transport_message_id` across retry, and do not expose bearer/session material to renderer/core code.
- Add accessibility descriptors shared by both mobile adapters and platform checks for VoiceOver and TalkBack reading order.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Shared mobile renderer/state core | iPhone/iOS and Android must receive the same assistant behavior | TP-073-13 platform parity canary |
| Mobile auth adapter boundary | Platform adapters handle secure session handoff; shared core never sees raw storage primitives | TP-073-18 storage guard |
| Mobile retry/idempotency | Both platforms retry with their original turn id | TP-073-14 retry parity row |
| Mobile accessibility descriptors | VoiceOver and TalkBack labels/read order remain semantically equivalent | TP-073-15 and TP-073-16 |

### Change Boundary

- **Allowed file families:** planned `clients/mobile/assistant/**`, shared mobile runtime/build metadata if selected by implementation, mobile platform adapter tests, shared mobile unit/e2e tests.
- **Excluded surfaces:** separate mobile business/UI codebases, server route changes, new auth primitives, durable bearer/session storage in shared renderer/core code, platform-specific scenario/action/transport-hint branching.
- **Containment rule:** platform-specific code may adapt OS shell and accessibility bridge mechanics only; assistant semantics, schema interpretation, retry behavior, and response controls remain shared.

### Impact-Aware Validation

No project impact map is configured. Shared mobile work requires build canaries, iPhone/iOS and Android parity checks, VoiceOver/TalkBack accessibility validation, idempotent retry regression on both platforms, and sensitive storage guards.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-13 | SCN-073-A02 | unit/build | `TBD: shared mobile platform parity build test` | Planned: one shared mobile codebase builds or validates ios and android targets from the same renderer/state core | `./smackerel.sh test unit` | No |
| TP-073-14 | SCN-073-A03 | e2e-ui | `TBD: shared mobile retry parity e2e test` | Planned regression: iPhone/iOS and Android timeout retries reuse each platform's original transport_message_id | `./smackerel.sh test e2e` | Yes |
| TP-073-15 | SCN-073-A10 | e2e-ui | `TBD: iPhone/iOS VoiceOver assistant accessibility test` | Planned regression: VoiceOver announces response roles, controls, citations, capture acknowledgement, retry/offline, and session errors | `./smackerel.sh test e2e` | Yes |
| TP-073-16 | SCN-073-A10 | e2e-ui | `TBD: Android TalkBack assistant accessibility test` | Planned regression: TalkBack announces response roles, controls, citations, capture acknowledgement, retry/offline, and session errors | `./smackerel.sh test e2e` | Yes |
| TP-073-17 | SCN-073-A11 | unit/build | `TBD: shared mobile fail-loud config test` | Planned: missing mobile backend URL/schema/platform/auth config fails for iPhone/iOS and Android without defaults | `./smackerel.sh test unit` | No |
| TP-073-18 | SCN-073-A10 | unit | `TBD: shared mobile sensitive storage guard test` | Planned: shared mobile renderer/core stores no bearer/session material in durable client storage | `./smackerel.sh test unit` | No |

### Definition of Done — Tiered Validation

- [ ] Shared mobile codebase, generated models, iPhone/iOS adapter, Android adapter, idempotent retry, secure session boundary, semantic labels, focus order, and parity with shared fixtures satisfy SCN-073-A02, SCN-073-A03, SCN-073-A10, and SCN-073-A11.
- [ ] TP-073-13 through TP-073-18 pass with current-session evidence.
- [ ] Mobile parity proof shows iPhone/iOS and Android use one shared renderer/state core and differ only in permitted platform-adapter concerns.
- [ ] VoiceOver and TalkBack validation covers composer, choices, confirms, citations, capture acknowledgement, retry/offline errors, and session errors.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute mobile build, instrumentation, runtime, or quality commands.

---

## Scope 4: Cross-Surface Response Controls, Capture, And Parity

**Status:** Not Started  
**Depends On:** Scope 3  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-A04 — Disambiguation prompt renders and round-trips on web and mobile
  Given a turn returns an AssistantResponse with a disambiguation prompt
  When the user picks choice 2 on web, iPhone/iOS, and Android in separate sessions
  Then every client POSTs kind = "disambiguation" with the same disambiguation_ref shape
  And the eventual response on every client matches the chosen scenario's invocation result

Scenario: SCN-073-A05 — Confirm card renders identically and round-trips
  Given a turn returns an AssistantResponse with a confirm card
  When the user accepts the action
  Then the side-effect-bearing path executes server-side
  And web, iPhone/iOS, and Android render the post-confirm result with identical structure

Scenario: SCN-073-A06 — Capture-as-fallback acknowledgement is identical to other transports
  Given the server returns AssistantResponse with CaptureRoute = true
  When the client renders the response
  Then the "saved-as-idea" acknowledgement appears with the same shape and copy as Telegram, HTTP-test, and WhatsApp
  And no client-side capture logic exists; the server's response alone drives the UI

Scenario: SCN-073-A07 — No client-side scenario logic exists
  Given the response shape does not include a recognized control variant
  When the client renders it
  Then the client falls back to rendering the text body
  And the client does NOT branch on scenario id, action class, or transport_hint to decide affordances

Scenario: SCN-073-A08 — Closed-vocabulary transport_hint is honored
  Given the web client sends transport_hint = "web" and the shared mobile client sends transport_hint = "mobile" on iPhone/iOS and Android
  When the server processes all three clients
  Then the server-side closed-vocabulary check accepts both values
  And neither value alters scenario selection or response shape
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-073-A04 | Web + shared mobile response controls | disambiguation response fixture and live route | choose option 2 on web, iPhone/iOS, and Android | every client posts the same ref/index shape and renders matching result | TP-073-19 |
| SCN-073-A05 | Web + shared mobile confirm card | confirm response fixture and live route | accept action on each surface | server owns side effect; all clients render identical result structure | TP-073-20 |
| SCN-073-A06 | Web + shared mobile capture ack | capture response fixture | render response | canonical saved-as-idea shape and copy matches other transports and all three client surfaces | TP-073-21 |
| SCN-073-A07 | Shared renderer | unknown response variant | render response | text fallback appears without scenario/action/transport branching | TP-073-22 |
| SCN-073-A08 | Web + shared mobile transport hints | valid web, iPhone/iOS, and Android sessions | submit equivalent turns | hints remain closed-vocabulary telemetry only and do not alter affordances | TP-073-23 |

### Implementation Plan

- Create shared response fixtures for text, citations, disambiguation, confirm, reset, capture acknowledgement, error, and unknown shape.
- Implement web projection and shared mobile renderer/adapters against generated schema models using the same fixture expectations.
- Add source/citation detail panel or sheet behavior and copy-safe metadata display across web, iPhone/iOS, and Android.
- Add guard/static scan that fails on client-side branching over scenario id, action class, platform, or transport hint.
- Add cross-transport acknowledgement comparison against Telegram/HTTP/WhatsApp expected capture shape and web/iPhone/iOS/Android client projections.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Shared renderer fixtures | Web, iPhone/iOS, and Android must interpret the same schema semantics | TP-073-19 through TP-073-22 |
| Capture acknowledgement shape | Spec 074 canonical shape must remain transport-neutral | TP-073-21 cross-transport regression |
| No-branch guard | Client renderers must not encode scenario, action, platform, or transport-hint logic | TP-073-22 guard row |
| Response detail/source surface | Citations and copy-safe metadata must stay equivalent across web and mobile | TP-073-24 source-detail parity row |

### Change Boundary

- **Allowed file families:** shared assistant response fixtures, web assistant renderer files, shared mobile renderer/core/adapters, renderer tests, static scan/guard tests.
- **Excluded surfaces:** server scenario outcomes, capture policy internals, transport-specific server adapters, schema field additions, platform-specific business logic forks.
- **Containment rule:** if a response shape cannot support UX, route to spec 069/schema ownership rather than adding client-only logic.

### Consumer Impact Sweep

| Consumer | Stale-Reference Search Surface | Required Proof |
|---|---|---|
| Web client | generated model imports, response renderer, PWA route/tests | No references to old mobile-only plan paths or scenario-specific branches |
| Shared mobile client | shared core, iPhone/iOS adapter, Android adapter, build metadata/tests | iPhone/iOS and Android parity rows remain tied to one codebase |
| Cross-spec references | specs 069/072/074/075 plan-owned artifacts | Old 073 slug/title and mobile single-platform wording are absent or intentionally historical |
| Tests/fixtures | assistant response fixtures, e2e-ui/e2e-api rows, integration parity rows | Every changed behavior has persistent regression E2E planning |

### Impact-Aware Validation

No project impact map is configured. Renderer work must use unit/functional fixture tests plus e2e-ui rows for web, iPhone/iOS, and Android paths once all clients exist.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-19 | SCN-073-A04 | e2e-ui | `TBD: web and shared mobile disambiguation parity e2e test` | Planned regression: disambiguation choice round-trips by ref/index on web, iPhone/iOS, and Android | `./smackerel.sh test e2e` | Yes |
| TP-073-20 | SCN-073-A05 | e2e-ui | `TBD: web and shared mobile confirm parity e2e test` | Planned regression: confirm accept executes server-side and renders equivalent result structure on all three surfaces | `./smackerel.sh test e2e` | Yes |
| TP-073-21 | SCN-073-A06 | integration | `TBD: cross-transport capture acknowledgement parity test` | Planned: capture acknowledgement shape/copy matches Telegram, HTTP-test, WhatsApp, web, iPhone/iOS, and Android fixtures | `./smackerel.sh test integration` | Yes |
| TP-073-22 | SCN-073-A07 | unit | `TBD: assistant renderer no-branch guard test` | Planned: renderer has no scenario/action/platform/transport-hint branching and falls back to text | `./smackerel.sh test unit` | No |
| TP-073-23 | SCN-073-A08 | e2e-api | `TBD: live client transport hint parity e2e test` | Planned regression: web and mobile transport hints do not alter visible response shape on web, iPhone/iOS, or Android | `./smackerel.sh test e2e` | Yes |
| TP-073-24 | SCN-073-A04 | e2e-ui | `TBD: web and shared mobile source detail parity e2e test` | Planned regression: source detail opens, closes, returns focus, and exposes copy-safe metadata across web, iPhone/iOS, and Android | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Shared response renderer, citations/detail, disambiguation, confirm, capture acknowledgement, closed transport hints, and no-branch guard satisfy SCN-073-A04 through SCN-073-A08.
- [ ] TP-073-19 through TP-073-24 pass with current-session evidence.
- [ ] Consumer impact sweep confirms any schema/control rename updates web, shared mobile, iPhone/iOS adapter, Android adapter, tests, fixtures, generated clients, and response detail surfaces together.
- [ ] Web/mobile parity proof covers equivalent visible choices and actions on web, iPhone/iOS, and Android.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or UI validation commands.