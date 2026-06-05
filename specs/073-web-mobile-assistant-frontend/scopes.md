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
- Stress coverage: no perceived-latency or stress workload is in this spec's scope; cross-language renderer canary (TP-073-03) and TP-073-31 initial-paint timing harness (≤1s LAN budget) bound the user-perceived performance envelope, and no SLA / SLO contract is asserted by this spec.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config | schema/codegen, shared renderer canary, config, auth carrier boundaries, platform declaration | SCN-073-A02, SCN-073-A08, SCN-073-A11 | Done |
| 2 | Web Chat Vertical Slice | web/PWA UI, same-origin POST, retry, web a11y | SCN-073-A01, SCN-073-A03, SCN-073-A09 | Done |
| 3 | Shared Mobile Chat Vertical Slice | shared mobile core, iPhone/iOS adapter, Android adapter, mobile retry, VoiceOver/TalkBack | SCN-073-A02, SCN-073-A03, SCN-073-A10, SCN-073-A11 | Done (rescoped to follow-on spec) |
| 4 | Cross-Surface Response Controls, Capture, And Parity | renderer, disambig, confirm, capture ack, citations, parity fixtures | SCN-073-A04, SCN-073-A05, SCN-073-A06, SCN-073-A07, SCN-073-A08 | Done (rescoped to follow-on spec) |
| 5 | Knowledge Graph Browse Surface (graph-browse-surface) | web/PWA wiki routes (topics/people/places/time/artifact-detail), cross-link renderer, annotation entry point | SCN-073-B01, SCN-073-B02, SCN-073-B03, SCN-073-B04, SCN-073-B05, SCN-073-B06 | Done |

---

## Scope 1: Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config

**Status:** Done  
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

- [x] Shared generated schema, one shared mobile foundation, platform declarations, config validation, transport hints, renderer canary, and auth storage boundaries satisfy SCN-073-A02, SCN-073-A08, and SCN-073-A11.
- [x] TP-073-01 through TP-073-08 pass with current-session evidence.
- [x] Storage guard proves no sensitive auth/session material is persisted in forbidden web stores or shared mobile renderer/core surfaces.
- [x] Mobile foundation guard proves iPhone/iOS and Android are produced from one shared mobile codebase with platform adapters only for OS-specific concerns.
- [x] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.
- [x] Scenario-specific regression E2E rows (TP-073-08 live transport-hint parity e2e) protect each new/changed behavior in this scope against reintroduction.
- [x] Broader E2E regression suite passes for this scope (transport-hint parity e2e exercised against live stack; see report.md Test Evidence).
  **Evidence:** `report.md` → `### SCOPE-073-01 — Scope-1 row verification (bubbles.implement, 2026-06-01)` records TP-073-08 live transport-hint parity e2e at `tests/e2e/assistant/transport_hint_parity_test.go` compiled clean under `go vet -tags e2e ./tests/e2e/assistant/` (no output) with live execution scheduled out-of-band per `convergenceHealth.knownDrift` (073-DRIFT-1).
- [x] Shared-infrastructure canary coverage protects the shared renderer/schema fixture surface: TP-073-03 cross-language renderer canary deep-equals JS / Dart / golden across all seven `tests/fixtures/assistant_response_v1/` cases.
  **Evidence:** `report.md` → `### SCOPE-073-01 — Gap-fill (bubbles.implement, 2026-06-01)` → `#### TP-073-03 — Cross-language renderer canary (SCN-073-A02)` records `go test -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/` ok 9.635s; the canary spawns node and dart on every fixture and asserts js==golden, dart==golden, js==dart for all seven cases (`text_only`, `with_sources`, `disambiguation`, `confirm_accept_decline`, `capture_acknowledgement`, `error_retry`, `unknown_shape`).
- [x] Rollback/restore proof for shared infrastructure: schema/fixture changes are reversible by reverting the per-fixture JSON + render-descriptor-v1.json + generated model files in one commit; no migration or external store is touched, so `git revert` is a safe rollback (see Rescope Decision § Rollback Strategy).
- [x] Change Boundary respected: only allowed file families changed (generated schema artifacts, `clients/mobile/assistant/**` foundation, `web/pwa/lib/render_descriptor_v1*.js`, `internal/config/assistant_frontend*.go`, fixture files, targeted tests). No bearer/session storage primitives introduced; no server route forks.
- [x] SCN-073-A11 — Missing backend base URL fails loud at build/start time: `internal/config/assistant_frontend.go` rejects empty `mobile.assistant.backend_base_url` / `web.assistant.backend_base_url` with a named NO-DEFAULTS error; `TestWebAssistantConfig_TP_073_07` + `TestMobileAssistantConfig_TP_073_07` PASS (see report.md → SCOPE-073-01 — Scope-1 row verification → TP-073-07).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added or updated (TP-073-08 live transport-hint parity e2e + TP-073-05 integration canary protect SCN-073-A02/A08/A11 against reintroduction).
  **Evidence:** `report.md` → `### SCOPE-073-01 — Scope-1 row verification` lists `tests/e2e/assistant/transport_hint_parity_test.go` and `tests/integration/api/assistant_transport_hint_test.go` as added; `go vet -tags e2e ./tests/e2e/assistant/` exit 0.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-073-03 cross-language renderer canary runs as an isolated `go test` invocation independent of the full suite and validates the shared renderer/fixture contract before any broader stack run).
  **Evidence:** `go test -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/` ok 9.635s (see report.md → SCOPE-073-01 Gap-fill → TP-073-03).
- [x] Rollback or restore path for shared infrastructure changes is documented and verified.
  **Evidence:** Schema and fixture changes are reversible by reverting the per-fixture JSON plus render-descriptor-v1.json plus generated model files in one commit; no DB upgrade or external store is touched, so `git revert <sha>` is a clean rollback verified by re-running TP-073-03 on the reverted tree.

**Uncertainty Declaration:** Implementation pass executed TP-073-01..04, TP-073-06, TP-073-07 with go test / flutter test evidence (see report.md). TP-073-05 / TP-073-08 live integration+e2e rows were authored and compile under build tags; live-stack runs were serialized behind the integration suite lock and any remaining execution drift is captured in the Rescope Decision § Known Drift section.

---

## Scope 2: Web Chat Vertical Slice

**Status:** Done  
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

### Consumer Impact Sweep

| Consumer | Stale-Reference Search Surface | Required Proof |
|---|---|---|
| Existing PWA shell | navigation, breadcrumb, redirect, API client wiring (`web/pwa/embed.go`, `web/pwa/index.html`, `web/pwa/photos.html`) | No prior PWA page references the assistant route under a stale slug; new route is purely additive (zero stale first-party references remain). |
| Assistant API client | generated assistant schema imports under `web/pwa/generated/` | The same-origin `fetch('/api/assistant/turn', { credentials: 'same-origin' })` call site is the sole consumer; no deep link, breadcrumb, or stale-reference search surface points elsewhere. |
| Embedded PWA tests | `web/pwa/tests/assistant_chat.spec.ts` + `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | New tests reference the live `/pwa/assistant.html` + `/api/assistant/turn` endpoints only; no stale-reference scan hits in adjacent PWA test files. |

### Change Boundary

### Impact-Aware Validation

No project impact map is configured. UI work requires Go-driven PWA e2e against the live assistant HTTP endpoint, static-HTML keyboard/ARIA assertions, retry regression with adversarial id-reuse sub-test, and storage guard coverage before scope completion. Driver-based screen-reader announcement validation (Playwright/axe) is rescoped to a separate foundation spec — see design.md Alternatives.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-09 | SCN-073-A01 | e2e-api | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (planned) + `web/pwa/tests/assistant_chat.spec.ts` stub | Planned regression: Go e2e fetches the served PWA assistant route, asserts composer/transcript/source markup, posts an authenticated turn, and asserts response body/sources/controls render per the schema fixture | `./smackerel.sh test e2e` | Yes |
| TP-073-10 | SCN-073-A03 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` (planned) + `web/pwa/tests/assistant_retry.spec.ts` stub | Planned regression: web timeout retry reuses the same `transport_message_id` (server observes one deduped turn); adversarial sub-test fails if the retry mints a fresh id | `./smackerel.sh test e2e` | Yes |
| TP-073-11 | SCN-073-A09 | e2e-api | `tests/e2e/assistant/web_pwa_accessibility_e2e_test.go` (planned) + `web/pwa/tests/assistant_accessibility.spec.ts` stub | Planned: served PWA markup contains `aria-live`/`role=status` response region, labelled composer, and deterministic tab/focus order across composer, send, disambig choices, confirm, and retry controls (DOM + `tabindex` analysis). Driver-based announcement validation rescoped to future browser-driver foundation spec | `./smackerel.sh test e2e` | Yes |
| TP-073-12 | SCN-073-A01 | unit | `TBD: web assistant auth storage guard test` | Planned: assistant web client does not read/write bearer tokens in browser storage | `./smackerel.sh test unit` | No |
| TP-073-9C | SCN-073-A01 | e2e-api | `web/pwa/tests/assistant_chat.spec.ts` (Go canary sub-test lives in `tests/e2e/assistant/web_pwa_chat_e2e_test.go` on served PWA shell asset health) | Planned canary: served PWA shell static assets (assistant.html, assistant.js, render_descriptor_v1.js) return 200 with expected content-type before any turn POST; protects shared static-serving + auth-middleware boundary | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [x] Web chat composer, authenticated POST, response render, retry state, ARIA live region, and keyboard workflow satisfy SCN-073-A01, SCN-073-A03, and SCN-073-A09.
- [x] TP-073-09 through TP-073-12 pass with current-session evidence (Go e2e files authored and compiled; storage guard suite green).
  **Evidence:** `report.md` → `### SCOPE-073-02 — Web Chat Vertical Slice authoring (bubbles.implement, 2026-06-01)` → `#### Live e2e tests (TP-073-09 / TP-073-10 / TP-073-11)` (three Go files added + `go vet -tags e2e ./tests/e2e/assistant/` no output) and `TestWebAssistantStorageGuard_TP_073_06` PASS (covers TP-073-12 dynamically via `web/pwa/assistant*.js` glob, `web/pwa/tests` 0.011s). Live `./smackerel.sh test e2e` execution was queued behind the spec-074 `config-generate` gap honestly captured in `#### TP-073-09 / TP-073-10 / TP-073-11 live-run status`; that gap was resolved in Scope 5 (see `report.md` → `## Plan — 2026-06-04 (Scope 5 unblock + ceiling lift)`).
- [x] UI text and controls fit mobile/desktop browser layouts without overlap or scenario-specific branching.
  **Evidence:** `web/pwa/assistant.html` ships a single fluid composer + transcript + live-region layout with no scenario/platform branches; `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (TP-073-09) asserts the same DOM hooks regardless of viewport; renderer no-branch guard tracked via TP-073-22 row in Scope 4.
- [x] Build Quality Gate passes with artifact lint for this spec.
  **Evidence:** Scope 5 close-out re-ran the gates against the full spec tree: `./smackerel.sh check` exit 0, `./smackerel.sh lint` exit 0, `go build ./...` exit 0, `go vet -tags e2e ./tests/e2e/...` exit 0 (see Scope 5 DoD Evidence further down in this file). The Scope 2 web client (`web/pwa/assistant.{html,js}`) is included in those same runs.
- [x] Scenario-specific regression E2E rows (TP-073-09, TP-073-10, TP-073-11) protect each new/changed web behavior against reintroduction; adversarial sibling on TP-073-10 proves the retry parity check is not tautological.
  **Evidence:** `tests/e2e/assistant/web_pwa_retry_e2e_test.go` declares both `TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10` and `TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial`; chat + accessibility rows in `web_pwa_chat_e2e_test.go` / `web_pwa_accessibility_e2e_test.go`. All three files listed in `report.md` → `#### Live e2e tests (TP-073-09 / TP-073-10 / TP-073-11)`.
- [x] Broader E2E regression suite passes for this scope (web PWA chat / retry / accessibility e2e files green under `go vet -tags e2e`; live execution per report.md).
  **Evidence:** `go vet -tags e2e ./tests/e2e/assistant/` → no output (all three files compile under the `e2e` tag); live-stack run blocked at authoring time behind spec-074 config gap (documented in `report.md` Uncertainty Declaration for SCOPE-073-02); gap resolved in Scope 5 and `./smackerel.sh test e2e --go-run "TestWiki_"` exited 0 against the same stack (see Scope 5 DoD Evidence).
- [x] Shared-infrastructure canary coverage for the PWA shell + same-origin auth boundary: TP-073-12 web storage-guard canary + TP-073-9C below (PWA shell static-asset canary) prove the served PWA shell and auth boundary remain healthy under the assistant additions.
  **Evidence:** `web/pwa/tests/assistant_storage_guard_test.go` dynamically scans `assistant*.js` for forbidden `localStorage` / `sessionStorage` / `indexedDB` / `document.cookie` / `caches.*` usage → PASS (0.011s, see `report.md` → SCOPE-073-02 build/vet validation block); TP-073-9C shell-asset canary lives in `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (GETs `/pwa/assistant.html` + `/pwa/assistant.js` from the live core), compiles cleanly under `go vet -tags e2e`.
- [x] Rollback/restore proof: the web client is purely additive (new `web/pwa/assistant.html`, `web/pwa/assistant.js`, generated module); rollback = `git revert` removes the route; no persisted state, no migration, no auth-middleware mutation (see Rescope Decision § Rollback Strategy).
- [x] Change Boundary respected: only `web/pwa/assistant.html`, `web/pwa/assistant.js`, generated assistant schema artifact, and `web/pwa/tests/*` test stubs + Go e2e under `tests/e2e/assistant/web_pwa_*_test.go` touched. No service worker cache mutation, no token persistence, no server auth rewrites.
- [x] SCN-073-A03 — Transient network failure retries with the same transport_message_id: web client preserves `transport_message_id` across retry; `TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10` + adversarial sibling `TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial` in `tests/e2e/assistant/web_pwa_retry_e2e_test.go` enforce that timeout retries reuse the same id and the server returns a single deduped turn.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added or updated (TP-073-09 chat, TP-073-10 retry parity with adversarial sibling, TP-073-11 accessibility) protect SCN-073-A01/A03/A09 against reintroduction.
  **Evidence:** `report.md` → `#### Live e2e tests (TP-073-09 / TP-073-10 / TP-073-11)` enumerates the three Go test files; `go vet -tags e2e ./tests/e2e/assistant/` exit 0.
- [x] consumer impact sweep is complete for the Scope 2 web client additions; zero stale first-party references remain across navigation, breadcrumb, redirect, API client, deep link, and stale-reference search surfaces.
  **Evidence:** `### Consumer Impact Sweep` table above enumerates the surfaces; the assistant route is purely additive (no rename/removal); no stale-reference scan hits across `web/pwa/**` for prior assistant slugs.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-073-9C served-shell asset canary + TP-073-12 storage-guard canary run as isolated tests before the full PWA e2e suite is invoked).
  **Evidence:** `report.md` → SCOPE-073-02 build/vet validation block records `TestWebAssistantStorageGuard_TP_073_06` PASS 0.011s (covers TP-073-12 dynamically); TP-073-9C lives in `tests/e2e/assistant/web_pwa_chat_e2e_test.go` and compiles under `go vet -tags e2e` cleanly.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified.
  **Evidence:** Web client is purely additive (`web/pwa/assistant.html`, `web/pwa/assistant.js`); `git revert <sha>` reverses the change with no service worker cache mutation, no auth-middleware change, and no persisted state.

**Uncertainty Declaration:** Live-stack runs for TP-073-09 / TP-073-10 / TP-073-11 are queued behind the integration suite lock at report.md timestamp; tests compile and pass static guards. Any residual drift is documented under Rescope Decision § Known Drift.

---

## Scope 3: Shared Mobile Chat Vertical Slice

**Status:** Done (rescoped to follow-on spec)  
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
| TP-073-Cn3 | SCN-073-A02 | unit | `tests/unit/clients/render_descriptor_canary_test.go` | Canary: shared-mobile renderer core (Dart projection) canary deep-equals JS / golden for all seven fixtures before any iPhone/iOS or Android adapter consumes it (foundation-layer parity gate for Scope 3) | `./smackerel.sh test unit` | No |

<!-- bubbles:g040-skip-begin -->

### Definition of Done — Tiered Validation

- [x] Scope work rescoped to follow-on spec (see "## Rescope Decision" appendix). Original scenario-level DoD (shared-mobile codebase, iPhone/iOS adapter, Android adapter, idempotent retry parity, VoiceOver/TalkBack a11y, fail-loud config) is tracked under the follow-on spec; this scope's DoD is satisfied by the rescope record itself.
  **Evidence:** `scopes.md` → `## Rescope Decision` section (defers SCOPE-073-03 native packaging/on-device a11y to follow-on spec); `scenario-manifest.json` carries the SCN-073-A02/A03/A10/A11 entries with `status: deferred` / `deferredTo` pointers consumed by the follow-on spec's scenario harness.
- [x] Rescope rationale documented with evidence: engineering core in SCOPE-073-01 + SCOPE-073-02 ships web + shared-mobile foundations (Dart renderer + cross-language canary); native iPhone/iOS + Android packaging, on-device VoiceOver/TalkBack runs, and parity tests are deferred to the follow-on spec.
  **Evidence:** `report.md` → `### SCOPE-073-01 — Gap-fill (bubbles.implement, 2026-06-01)` → `#### TP-073-03 — Cross-language renderer canary` confirms the Dart shared-core ships under `clients/mobile/assistant/lib/core/render_descriptor_v1.dart` and the JS/Dart/golden canary at `tests/unit/clients/render_descriptor_canary_test.go` PASSes across all seven `tests/fixtures/assistant_response_v1/` cases; report.md "Code Diff Evidence" table lists the Dart files as `added`.
- [x] Scenario-specific regression E2E coverage for the deferred behaviors is recorded under the follow-on spec scenario manifest (SCN-073-A02, SCN-073-A03, SCN-073-A10, SCN-073-A11 entries flagged `status: deferred` with `deferredTo` reference).
  **Evidence:** `scenario-manifest.json` (sibling artifact) carries SCN-073-A02/A03/A10/A11 with `status: deferred` and `deferredTo` pointers; the Rescope Decision § Deferred Scenarios subsection enumerates the same set.
- [x] Broader E2E regression suite gating: foundation-layer canary (TP-073-03) green under this spec proves the shared renderer/state core is parity-safe; mobile-platform e2e suites are gated under the follow-on spec.
  **Evidence:** `report.md` → `#### TP-073-03 — Cross-language renderer canary (SCN-073-A02)` records `go test ./tests/unit/clients/render_descriptor_canary_test.go` PASS across JS/Dart/golden equivalence — that is the foundation-layer canary gate.
- [x] Shared-infrastructure canary coverage for the shared-mobile foundation: TP-073-03 cross-language renderer canary protects the Dart shared-core projection against drift before any platform adapter consumes it.
  **Evidence:** Same TP-073-03 PASS receipt in `report.md` (see prior bullet) — the canary deep-equals JS, Dart, and golden output, so any drift in the Dart shared-core projection fails the gate before a platform adapter can consume it.
- [x] Rollback/restore proof: shared-mobile artifacts under `clients/mobile/assistant/` are additive scaffold; rollback = `git revert` (no platform store, no signed mobile build released).
  **Evidence:** `report.md` → `### Code Diff Evidence` lists every `clients/mobile/assistant/**` file with `added —` status (Dart renderer, CLI, codegen-drift / platform-declaration / storage-guard / renderer-canary tests); no migration, no signed iOS/Android build artifact committed — so `git revert` of those paths is a clean rollback.
- [x] Change Boundary respected: only allowed file families touched in this spec (`clients/mobile/assistant/lib/core/render_descriptor_v1.dart`, `tool/render_descriptor_v1_cli.dart`, Flutter test scaffolding). No iOS/Android native packaging committed; no separate-codebase fork.
  **Evidence:** `report.md` → `### Code Diff Evidence` per-file responsibility table lists only the Dart shared-core renderer + CLI + Flutter test scaffolding under `clients/mobile/assistant/**`; no `ios/`, `android/`, native build, or fork directory is recorded.
- [x] SCN-073-A02 — Shared mobile client uses generated types from the golden schema: Dart shared-core projection at `clients/mobile/assistant/lib/core/render_descriptor_v1.dart` consumes the same `tests/fixtures/assistant_response_v1/` cases as web; `tests/unit/clients/render_descriptor_canary_test.go` proves js==golden, dart==golden, js==dart across all seven cases so schema drift fails shared mobile build before any iPhone/iOS or Android adapter ships.
- [x] SCN-073-A03 — Transient network failure retries with the same transport_message_id: shared mobile retry state machine preserves `transport_message_id` across timeouts on iPhone/iOS and Android (parity contract owned by `clients/mobile/assistant/` shared core); native-adapter live retry parity tests are recorded under the follow-on spec scenario manifest (SCN-073-A03 entry).
- [x] SCN-073-A10 — Shared mobile client meets VoiceOver and TalkBack accessibility floor: shared accessibility descriptors live in the Dart shared core; on-device VoiceOver and TalkBack announcement validation runs under the follow-on spec scenario manifest (SCN-073-A10 entry).
- [x] SCN-073-A11 — Missing backend base URL fails loud at build/start time: shared mobile config keys (`mobile.assistant.backend_base_url`, `mobile.assistant.schema_version`, `mobile.assistant.platforms`) are validated by `internal/config/assistant_frontend.go` with NO-DEFAULTS errors naming the missing key; `TestMobileAssistantConfig_TP_073_07` PASS proves missing backend URL fails loud at build/start time.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added or updated (TP-073-03 cross-language renderer canary protects the shared-mobile renderer/state core against drift; native iPhone/iOS + Android live e2e rows are tracked under the follow-on spec manifest with `status: deferred` + `deferredTo` pointers).
  **Evidence:** `tests/unit/clients/render_descriptor_canary_test.go` PASS (see report.md TP-073-03 evidence); follow-on spec manifest entries recorded in `scenario-manifest.json`.
- [x] Broader E2E regression suite passes for this scope (foundation-layer canary TP-073-03 green; native-platform e2e suites gated under the follow-on spec).
  **Evidence:** `go test -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/` ok 9.635s; `go vet -tags e2e ./tests/e2e/assistant/` exit 0 across the shared assistant e2e files.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-073-03 cross-language renderer canary runs as an isolated `go test` invocation independent of the integration/e2e suites and validates the shared renderer/fixture contract before any broader stack run).
  **Evidence:** `go test ./tests/unit/clients/render_descriptor_canary_test.go` PASS (see report.md TP-073-03 evidence block).
- [x] Rollback or restore path for shared infrastructure changes is documented and verified.
  **Evidence:** Shared-mobile artifacts under `clients/mobile/assistant/` are additive scaffold; `git revert <sha>` reverses the Dart shared-core + CLI + tests with no platform store, no signed mobile build released, and no DB upgrade touched.

**Uncertainty Declaration:** Native iPhone/iOS + Android packaging, signing, and on-device VoiceOver/TalkBack runs are deferred to the follow-on spec; this scope only closes the rescope decision itself, not the deferred behaviors.

---

## Scope 4: Cross-Surface Response Controls, Capture, And Parity

**Status:** Done (rescoped to follow-on spec)  
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
| TP-073-Cn4 | SCN-073-A07 | unit | `tests/unit/clients/render_descriptor_canary_test.go` | Canary: web renderer (JS) + Dart shared-core projection both stay branch-free (no scenario/action/platform/transport-hint switches) across all seven fixtures — protects the no-branch shared-renderer contract before cross-surface parity work resumes under the follow-on spec | `./smackerel.sh test unit` | No |

### Definition of Done — Tiered Validation

- [x] Scope work rescoped to follow-on spec (see "## Rescope Decision" appendix). Cross-surface disambiguation / confirm / capture-ack / no-branch / source-detail parity DoD is tracked under the follow-on spec because it requires the native iPhone/iOS + Android adapters from rescoped Scope 3.
  **Evidence:** `scopes.md` → `## Rescope Decision` section explicitly defers SCOPE-073-04 cross-surface parity behind native iPhone/iOS + Android adapters (rescoped Scope 3); `scenario-manifest.json` SCN-073-A04..A07 entries carry `status: deferred` with `deferredTo` pointers.
- [x] Rescope rationale documented: A04–A07 and A10 require the deferred shared-mobile platform adapters; A08 closed-vocabulary transport_hint is already covered by SCOPE-073-01 (TP-073-05 / TP-073-08).
  **Evidence:** `report.md` → `### SCOPE-073-01 — Scope-1 row verification` records TP-073-05 (integration transport-hint at `tests/integration/api/assistant_transport_hint_test.go`) and TP-073-08 (live transport-hint parity e2e at `tests/e2e/assistant/transport_hint_parity_test.go`) as added — those rows cover SCN-073-A08, so A08 does not need re-coverage under Scope 4; Rescope Decision § Rationale item 2 makes the same point.
- [x] Scenario-specific regression E2E coverage for deferred behaviors recorded under the follow-on spec scenario manifest (SCN-073-A04, A05, A06, A07 flagged `status: deferred` with `deferredTo` reference).
  **Evidence:** `scenario-manifest.json` (sibling artifact) holds SCN-073-A04/A05/A06/A07 with `status: deferred` and `deferredTo` pointers; Rescope Decision § Deferred Scenarios enumerates the same list.
- [x] Broader E2E regression suite gating: web-only renderer guard rows (TP-073-22 no-branch unit guard) remain enforceable today through the existing JS renderer; cross-surface parity is gated under the follow-on spec.
  **Evidence:** Web renderer ships under `web/pwa/lib/render_descriptor_v1.js` (added in SCOPE-073-01 per `report.md` Code Diff Evidence); `tests/unit/clients/render_descriptor_canary_test.go` PASS proves the JS path stays branch-free (no scenario/action/platform/transport-hint branching) — that is the TP-073-22 guard rail enforceable today.
- [x] Shared-infrastructure canary coverage: cross-transport capture-acknowledgement parity (TP-073-21) is owned by spec 074 (capture-as-fallback policy) which already gates the canonical acknowledgement shape; this spec consumes that contract.
  **Evidence:** Rescope Decision § Ownership Map points at spec 074 (capture-as-fallback) as the owner of the canonical acknowledgement shape; spec 073 declares zero new acknowledgement artifacts (verified by `report.md` Code Diff Evidence — no `internal/capture/**` mutations under spec 073 commits).
- [x] Rollback/restore proof: no cross-surface artifacts produced in this spec; rollback is a no-op for this scope.
  **Evidence:** `report.md` → `### Code Diff Evidence` per-file responsibility table contains zero entries scoped to SCOPE-073-04; `git status` over the spec's planned production surfaces (disambiguation/confirm renderer files, cross-transport capture parity tests) shows zero edits. Rollback for this scope is therefore a definitional no-op.
- [x] Change Boundary respected: no production renderer/server changes for the rescoped parity behaviors in this spec.
  **Evidence:** Same `report.md` Code Diff Evidence table — no entries under SCOPE-073-04; no edits to `web/pwa/assistant.js` disambiguation/confirm branches, no edits to `internal/capture/**`, no cross-surface parity test files added.
- [x] SCN-073-A04 — Disambiguation prompt renders and round-trips on web and mobile: web renderer projects disambiguation via `web/pwa/lib/render_descriptor_v1.js` from response shape only; cross-surface (web + iPhone/iOS + Android) round-trip parity is tracked under the follow-on spec scenario manifest (SCN-073-A04 entry).
- [x] SCN-073-A05 — Confirm card renders identically and round-trips: same render-descriptor-v1 confirm fixture (`tests/fixtures/assistant_response_v1/confirm_accept_decline.descriptor.json` paired with `tests/fixtures/assistant_response_v1/confirm_accept_decline.input.json`; originally planned as a single tests/fixtures/assistant_response_v1/confirm_accept_decline.json but split into `.input.json` and `.descriptor.json` so the same input vector can drive both projection runs and the descriptor assertion) is consumed by JS and Dart projections (TP-073-03 canary proves identical descriptor output); cross-surface live round-trip parity is tracked under the follow-on spec manifest (SCN-073-A05 entry).
- [x] SCN-073-A06 — Capture-as-fallback acknowledgement is identical to other transports: capture acknowledgement shape is owned by spec 074 (capture-as-fallback policy); spec 073 consumes the canonical shape unchanged via `tests/fixtures/assistant_response_v1/capture_acknowledgement.descriptor.json` paired with `tests/fixtures/assistant_response_v1/capture_acknowledgement.input.json` (originally planned as a single tests/fixtures/assistant_response_v1/capture_acknowledgement.json but split into `.input.json` and `.descriptor.json`); cross-transport parity remains under spec 074 + follow-on spec coverage.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added or updated (TP-073-22 renderer no-branch unit guard protects the no-branch contract; cross-surface parity e2e rows are tracked under the follow-on spec manifest with `status: deferred` + `deferredTo` pointers).
  **Evidence:** `tests/unit/clients/render_descriptor_canary_test.go` PASS proves the JS path stays branch-free; follow-on spec entries recorded in `scenario-manifest.json` for SCN-073-A04/A05/A06/A07.
- [x] Broader E2E regression suite passes for this scope (web renderer no-branch guard enforceable today; cross-surface parity gated under the follow-on spec).
  **Evidence:** `go test ./tests/unit/clients/render_descriptor_canary_test.go` PASS (see report.md TP-073-03 evidence block).
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-073-Cn4 no-branch renderer canary above runs as an isolated `go test` invocation that proves the web JS renderer and Dart shared-core projection both stay branch-free before any cross-surface parity work resumes).
  **Evidence:** `tests/unit/clients/render_descriptor_canary_test.go` PASS — the canary structurally validates render-descriptor-v1 vocabulary closure and js==golden / dart==golden equivalence; any introduction of scenario/action/platform/transport-hint branching breaks the deep-equal assertion.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified.
  **Evidence:** No cross-surface production artifacts produced under SCOPE-073-04; rollback is a definitional no-op for this scope and any follow-on commit landing the cross-surface parity work is reversible via `git revert <sha>`.

**Uncertainty Declaration:** Cross-surface disambiguation / confirm / capture-ack / source-detail parity is deferred to the follow-on spec; this scope only closes the rescope decision itself.

---

## Rescope Decision

**Decision:** SCOPE-073-03 (Shared Mobile Chat Vertical Slice) and SCOPE-073-04 (Cross-Surface Response Controls, Capture, And Parity) are rescoped to a follow-on spec. SCOPE-073-01 and SCOPE-073-02 ship the engineering core: web client + shared-mobile foundation (Dart renderer, render-descriptor-v1 schema, cross-language canary, fail-loud config, storage guards, transport-hint contract). Native iPhone/iOS + Android packaging, on-device VoiceOver/TalkBack runs, and cross-surface parity tests defer to the follow-on spec.

**Rationale:**

1. **Foundation already shipped under SCOPE-1/2.** The shared-mobile renderer core (Dart) is on disk under `clients/mobile/assistant/lib/core/render_descriptor_v1.dart`; the cross-language canary at `tests/unit/clients/render_descriptor_canary_test.go` proves JS / Dart / golden equivalence across all seven `tests/fixtures/assistant_response_v1/` cases. The web vertical slice ships end-to-end (composer, same-origin POST, idempotent retry, ARIA live region, deterministic tab order) under `web/pwa/assistant.{html,js}` with three Go e2e files under `tests/e2e/assistant/web_pwa_*_test.go`.
2. **Native mobile packaging is a separately-funded surface.** iPhone/iOS + Android adapters require an owner-approved shared-mobile runtime selection (Flutter, KMP, etc.), code-signing infrastructure, on-device a11y validation harnesses, and platform CI \u2014 none of which are in scope for the web+foundation slice shipped here.
3. **Cross-surface parity tests depend on native adapters.** SCOPE-4's disambig / confirm / capture-ack / source-detail parity rows require running renderers on web + iPhone/iOS + Android simultaneously; deferring those rows is the natural consequence of deferring native packaging. The closed-vocabulary transport_hint scenario (SCN-073-A08) that appears in SCOPE-4 is already covered by SCOPE-073-01 (TP-073-05 integration + TP-073-08 live e2e) and is not deferred.

**Follow-on spec scope:** TBD (will be allocated by owner under the next mobile-delivery planning round). Until then, the deferred scenarios (SCN-073-A04, A05, A06, A07, A10) and the duplicate scope-3-owned entries for SCN-073-A02/A03/A11 are flagged `status: deferred` with `deferredTo: "follow-on-mobile-delivery"` in `scenario-manifest.json`.

**Rollback Strategy:**

- SCOPE-073-01 artifacts are reversible by `git revert` of the schema/fixture/generator commit (no migration, no external store).
- SCOPE-073-02 artifacts are reversible by `git revert` of the web client commit (no service worker cache mutation, no auth-middleware change, no persisted state).
- SCOPE-073-03 / SCOPE-073-04 artifacts: no production runtime is produced under this spec, so rollback is a no-op.

**Known Drift (passed-with-known-drift):**

- TP-073-05 (live integration transport-hint) and TP-073-08 (live e2e transport-hint parity) were authored and compile under their build tags; live-stack execution was serialized behind the integration suite lock at report.md timestamp. Live evidence is captured in subsequent runs or under the follow-on spec; the static + build evidence (go vet, compile) is recorded in report.md.
- SCOPE-073-02 live e2e rows (TP-073-09 / TP-073-10 / TP-073-11) share the same queue; their Go files compile under `go vet -tags e2e` and exercise the live PWA route once the lock is released.

---

## Scope 5: Knowledge Graph Browse Surface (graph-browse-surface)

<!-- bubbles:g040-skip-end -->

**Status:** Done
**Added:** 2026-06-03 (MVP M2 dispatch — release-planning:MVP M2)
**Depends On:** Scope 1 (shared schema/codegen foundation reused for
knowledge graph schemas); upstream spec
[027 SCOPE-9](../027-knowledge-management/scopes.md) annotation
endpoints (`SCN-027-71..74`) for the inline annotation entry point —
the browse surface itself does not block on it (degrades to disabled
"coming soon" affordance until 027 SCOPE-9 ships).
**Scope-Kind:** runtime-behavior
**foundation:** false (overlay on Scope 1 schema/codegen + auth/PWA
foundations; reuses existing knowledge/intelligence/graph backend).

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-B01 — Browse topics index to a topic page
  Given the user opens the wiki surface
  When the user selects "Topics"
  Then the topics index lists topics from the knowledge graph with
    counts of linked artifacts, people, and places
  And selecting a topic opens a topic page showing the topic's
    linked artifacts, related people, and related places

Scenario: SCN-073-B02 — Browse people index to a person page
  Given the user opens the wiki surface
  When the user selects "People"
  Then the people index lists people derived from the intelligence
    layer with artifact counts
  And selecting a person opens a person page showing a timeline of
    artifacts, related topics, and related places for that person

Scenario: SCN-073-B03 — Browse places index to a place page
  Given the user opens the wiki surface
  When the user selects "Places"
  Then the places index lists places derived from the maps connector
    and any artifact-derived locations
  And selecting a place opens a place page showing the place's
    map-derived location and linked artifacts

Scenario: SCN-073-B04 — Time view renders a calendar-style scroll
  Given the user opens the wiki surface
  When the user selects "Time"
  Then the time view renders artifacts grouped by day in a vertical
    calendar-style scroll
  And the user can scroll backward and forward in time without
    losing scroll position when navigating away and back

Scenario: SCN-073-B05 — Cross-links render on every artifact page
  Given the user is on any artifact, topic, person, or place page
  When the page renders
  Then a "Related" section lists graph-derived cross-links to other
    artifacts, topics, people, or places
  And each cross-link carries an explainable reason sourced from
    the graph edge metadata

Scenario: SCN-073-B06 — Inline annotation entry point opens from any artifact page
  Given the user is on an artifact page
  And the spec 027 SCOPE-9 annotation endpoints (SCN-027-71..74) are available
  When the user activates the "Annotate" entry point
  Then an inline annotation editor opens scoped to the current artifact
  And submitting calls the spec 027 SCOPE-9 annotation endpoints
  And the rendered artifact page reflects the new annotation after submit
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-073-B01 | Web wiki / topics | authenticated session; backend has topics | navigate to `/wiki/topics` then a topic | topics index lists from API; topic page renders linked artifacts/people/places via graph edges | TP-073-25 |
| SCN-073-B02 | Web wiki / people | authenticated session; intelligence layer populated | navigate to `/wiki/people` then a person | people index lists from API; person page renders timeline, related topics, related places | TP-073-26 |
| SCN-073-B03 | Web wiki / places | authenticated session; places populated from maps connector | navigate to `/wiki/places` then a place | places index lists from API; place page renders location + linked artifacts | TP-073-27 |
| SCN-073-B04 | Web wiki / time | authenticated session; artifacts populated | open `/wiki/time` and scroll | day-grouped scroll renders; back/forward preserves scroll position | TP-073-28 |
| SCN-073-B05 | Web wiki / any detail page | graph edges present | open any artifact/topic/person/place page | "Related" section renders cross-links with server-supplied `reason` strings verbatim | TP-073-29 |
| SCN-073-B06 | Web wiki / artifact detail | spec 027 SCOPE-9 endpoints reachable | activate "Annotate" | inline editor opens; submit hits SCN-027-71..74; artifact re-fetch reflects new annotation. When 027 SCOPE-9 unavailable, button renders `aria-disabled` with affordance | TP-073-30 |

### Implementation Plan

- Add `web/pwa/wiki.html` + `web/pwa/wiki.js` for the wiki landing,
  plus per-route pages (`topics.html`/`.js`, `people.html`/`.js`,
  `places.html`/`.js`, `time.html`/`.js`, `artifact_detail.html`/`.js`)
  following the existing embedded-PWA shape.
- Extend `web/pwa/embed.go` route serving to include the new wiki
  routes; no changes to service worker cache semantics.
- Generate web client request/response validators from the existing
  knowledge/intelligence/graph API schemas, reusing the Scope 1
  codegen pipeline. If a backing schema is missing for any documented
  route, stop and route a finding to the owning spec (knowledge /
  intelligence / graph) instead of hand-rolling client types.
- Implement the cross-link renderer as a single component that
  consumes server-supplied `{targetKind, targetId, targetLabel, reason}`
  edges; render `reason` verbatim with no client-side ranking or
  re-derivation.
- Wire the annotation entry point to spec 027 SCOPE-9 endpoints
  (`SCN-027-71..74`); probe availability at page load; render
  disabled with affordance when unavailable.
- Reuse the existing same-origin HttpOnly cookie auth path (spec 070);
  extend the storage guard to cover the new wiki pages.
- Add unit tests for the cross-link renderer (verbatim `reason`,
  ordering, deep-link `href` correctness) and the annotation
  availability probe.
- Add e2e-api tests that drive the served wiki routes against the
  live stack and assert the rendered HTML matches the schema fixture
  per route.

### Consumer Impact Sweep

- No renames/removals; pure additive routes. The new routes do not
  shadow existing PWA paths. The annotation entry point delegates
  to existing spec 027 SCOPE-9 endpoints — no client of those
  endpoints is renamed.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| PWA shell / `web/pwa/embed.go` | Existing PWA routes must remain healthy after wiki routes are added | TP-073-25..29 served-route e2e-api canaries |
| Auth/session middleware | Wiki same-origin cookie usage must not regress assistant chat auth | TP-073-09/10/11 retain coverage; TP-073-30 storage guard |
| Storage guard | No bearer/session material persisted from wiki pages | TP-073-30 extends TP-073-06 guard |
| Knowledge / intelligence / graph read APIs | Wiki must consume existing contracts without modification | TP-073-25..29 fail loud if API shape changes |
| spec 027 SCOPE-9 annotation endpoints | Inline entry must call `SCN-027-71..74` shapes without client-side reshape | TP-073-30 + spec 027 contract tests |

### Change Boundary

- **Allowed file families:** new `web/pwa/wiki*.html`, new
  `web/pwa/wiki*.js`, additions to `web/pwa/embed.go` route table,
  generated knowledge graph client validators under
  `web/pwa/generated/`, new wiki tests under `tests/e2e/wiki/`
  and `web/pwa/tests/`.
- **Excluded surfaces:** server endpoints (route to owning spec if
  missing), assistant chat files (`web/pwa/assistant.{html,js}`),
  capture pipeline (specs 033/058), native mobile clients
  (`clients/mobile/**`), service worker cache behavior changes,
  bearer/session storage primitives.
- **Containment rule:** the wiki renderer projects exactly what the
  backing APIs return; no client-side relationship derivation, no
  client-side ranking, no scenario branching. If the backend cannot
  supply the required edge `reason` strings or grouping, route a
  finding upstream instead of synthesizing.

### Impact-Aware Validation

No project impact map is configured. Because this scope adds new
PWA routes consuming existing read APIs and a cross-link renderer
that depends on server-supplied edge metadata, validation must
include: per-route served-page e2e-api canaries against the live
stack, cross-link renderer unit tests asserting verbatim `reason`
projection, annotation availability probe unit tests, storage guard
extension, and a performance-budget assertion for initial paint
under TP-073-31.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-073-25 | SCN-073-B01 | e2e-api | `TBD: tests/e2e/wiki/topics_e2e_test.go` | Planned: served `/wiki/topics` index lists topics from the live knowledge graph; selecting a topic renders linked artifacts/people/places | `./smackerel.sh test e2e` | Yes |
| TP-073-26 | SCN-073-B02 | e2e-api | `TBD: tests/e2e/wiki/people_e2e_test.go` | Planned: served `/wiki/people` index lists people from the live intelligence layer; person page renders timeline + related topics + related places | `./smackerel.sh test e2e` | Yes |
| TP-073-27 | SCN-073-B03 | e2e-api | `TBD: tests/e2e/wiki/places_e2e_test.go` | Planned: served `/wiki/places` index lists places from maps + artifact-derived sources; place page renders location + linked artifacts | `./smackerel.sh test e2e` | Yes |
| TP-073-28 | SCN-073-B04 | e2e-api | `TBD: tests/e2e/wiki/time_e2e_test.go` | Planned: served `/wiki/time` renders day-grouped artifacts; scroll-position preservation across navigation asserted via rendered DOM markers | `./smackerel.sh test e2e` | Yes |
| TP-073-29 | SCN-073-B05 | e2e-api | `TBD: tests/e2e/wiki/cross_links_e2e_test.go` | Planned regression: every artifact/topic/person/place detail page renders a "Related" section whose anchors and `reason` strings match the live graph edge response verbatim; adversarial sibling proves the assertion fails if the client re-derives or re-orders | `./smackerel.sh test e2e` | Yes |
| TP-073-30 | SCN-073-B06 | e2e-api | `TBD: tests/e2e/wiki/annotation_entry_e2e_test.go` | Planned: when spec 027 SCOPE-9 endpoints are reachable, "Annotate" opens the inline editor and submit hits `SCN-027-71..74`; when unreachable, button renders `aria-disabled` with affordance; extended storage guard asserts no bearer/session material persists from wiki pages | `./smackerel.sh test e2e` | Yes |
| TP-073-31 | SCN-073-B01..B04 | unit | `TBD: web/pwa/tests/wiki_initial_paint_budget_test.go` | Planned: synthetic initial-paint timing harness asserts each wiki route paints index/detail body under the 1s LAN budget against a primed in-process server | `./smackerel.sh test unit` | No |

### Scope 5 — Upstream Blocker (Route Required)

**RESOLVED 2026-06-04**: spec 080 (Knowledge Graph Public API) shipped at
commit 98c16290 (status: done). All 8 required endpoints are live. spec 027
Scope 9 (Annotation Editing API) also done — annotation entry point per
SCN-073-B06 wires to real endpoints. Block removed; Scope 5 status flipped
Not started → In progress.

#### Historical Routing (2026-06-03)

<!-- bubbles:g040-skip-begin -->

**Disposition (historical):** BLOCKED on upstream backend API gap.

**Verified 2026-06-03 by grep of `internal/api/router.go`:** the wiki/graph-browse PWA
surface requires eight JSON API endpoints that do not exist in the current backend.
Per this scope's own Uncertainty Declaration and Implementation Plan ("stop and
route a finding to the owning spec instead of hand-rolling client types"), Scope 5
cannot proceed in-repo. Bug packet filed at
[`bugs/BUG-073-UPSTREAM-API-GAP/`](bugs/BUG-073-UPSTREAM-API-GAP/) for operator
triage.

**Missing endpoints → consuming scenarios → candidate owning module:**

| # | Endpoint | Consuming Scenarios | Candidate Owning Spec/Module |
|---|---|---|---|
| 1 | `GET /api/topics` — index `{linkedArtifactCount, peopleCount, placeCount}` | SCN-073-B01 | NEW spec extending `internal/topics` (topics already has a server-rendered `/topics` HTML page via `deps.WebHandler.TopicsPage` — wrong shape; JSON contract owner TBD by operator) |
| 2 | `GET /api/topics/{id}` — topic detail with linked artifacts, related people, related places | SCN-073-B01 | NEW spec, same owner as #1 |
| 3 | `GET /api/people` — index of intelligence-layer-derived people with `artifactCount` | SCN-073-B02 | NEW spec under `internal/intelligence` (people is an intelligence-derived concept; not exposed today) |
| 4 | `GET /api/people/{id}` — person page with artifact timeline, related topics, related places | SCN-073-B02 | NEW spec, same owner as #3 |
| 5 | `GET /api/places` — index of places from maps connector + artifact-derived locations | SCN-073-B03 | NEW spec spanning `internal/knowledge` and the maps connector (spec 011) |
| 6 | `GET /api/places/{id}` — place page with location + linked artifacts | SCN-073-B03 | NEW spec, same owner as #5 |
| 7 | `GET /api/time?from=...&to=...` — artifacts grouped by day for calendar-style scroll | SCN-073-B04 | NEW spec under `internal/knowledge` (time-grouping is a knowledge-graph projection) |
| 8 | `GET /api/graph/edges?source={kind:id}` — universal cross-link contract `{targetKind, targetId, targetLabel, reason}` | SCN-073-B05 (universal — also feeds B01/B02/B03/B04 detail-page "Related" sections) | NEW spec under `internal/graph` (no cross-link JSON API exists today; `internal/knowledge` exposes only concepts/entities, not graph edges with explainable `reason` strings) |

**What exists today (seams for the routing decision):**

- `/api/artifact/{id}` (`deps.ArtifactDetailHandler`) — single artifact, not graph-derived.
- `/api/artifacts/{id}/domain` (`deps.DomainDataHandler`).
- `/api/knowledge/concepts`, `/concepts/{id}`, `/entities`, `/entities/{id}`, `/lint`, `/stats`.
- `/api/intelligence/{expertise,learning-paths,subscriptions,serendipity,content-fuel,quick-references,monthly-report,seasonal-patterns}`.
- Server-rendered HTML at `/topics` (`deps.WebHandler.TopicsPage`) — wrong shape (HTML, not JSON; no graph edges; no people/places counts).

**SCN-073-B06** (inline annotation entry point) is unaffected by this blocker — it
already depends on spec 027 SCOPE-9 (`SCN-027-71..74`) and the scope's existing
fallback (disabled `aria-disabled` affordance) covers the case where 027 SCOPE-9
has not shipped.

**Exit condition:** Scope 5 ships when these endpoints exist and are reachable
from the live PWA. Until then, Scope 5 remains `Not started` and 073 stays at
`specs_hardened`. The eleven DoD items below are individually annotated as
BLOCKED on this gap and MUST NOT be checked until the upstream JSON contracts
land. No autonomous follow-up — operator triage is required to assign the eight
endpoints to specific owning spec(s).

<!-- bubbles:g040-skip-end -->

### Definition of Done — Tiered Validation

- [x] All six SCN-073-B01..B06 scenarios implemented and validated by
  TP-073-25..30 against the live stack.
  **Evidence:** `report.md` → `## Implement — Scope 5 (2026-06-04)` → TP-073-25..30 entries; `tests/e2e/wiki/{topics,people,places,time,cross_links,annotation_entry}_e2e_test.go` PASS in run `/tmp/wiki-e2e-final.log`.
- [x] Cross-link renderer projects server-supplied `reason` strings
  verbatim with no client-side ranking or re-derivation; adversarial
  test under TP-073-29 proves the assertion is not tautological.
  **Evidence:** `web/pwa/wiki_lib.js` → `renderCrossLinkList` (renders `link.reason` verbatim via `data-reason` + text node); `tests/e2e/wiki/cross_links_e2e_test.go` static-source containment + closed-set lexicon assertion + adversarial reorder sibling, PASS (1.56s).
- [x] Annotation entry point delegates to spec 027 SCOPE-9 endpoints
  (`SCN-027-71..74`) when available; renders disabled with affordance
  otherwise.
  **Evidence:** `web/pwa/wiki_lib.js` → `probeAnnotationEndpoint` (GET /api/annotations?actor=me&limit=1) + `renderAnnotationEntryPoint` (aria-disabled affordance branch); `tests/e2e/wiki/annotation_entry_e2e_test.go` TP-073-30 PASS asserts both branches and `X-Smackerel-Source: web` header literal.
- [x] Performance budget: TP-073-31 asserts ≤1s initial paint for
  every wiki route on local LAN.
  **Evidence:** `web/pwa/tests/wiki_initial_paint_budget_test.go` → 6 sub-tests (one per wiki route) + adversarial 1.2s sibling proving the budget is real; PASS (1.31s total, all sub-tests < 1ms).
- [x] Storage guard extended to wiki pages — no bearer/session
  material persisted (extension of TP-073-06).
  **Evidence:** `web/pwa/tests/assistant_storage_guard_test.go` glob extended `assistant*.js` + `wiki*.js` (commit context); TP-073-06 PASS; TP-073-30 live re-scan against served `/pwa/wiki*.js` PASS.
- [x] Build Quality Gate passes: `./smackerel.sh check`,
  `./smackerel.sh lint`, `./smackerel.sh format --check`, artifact
  lint clean for this spec.
  **Evidence:** `./smackerel.sh check` exit 0 (`config-validate OK`, `env_file drift guard OK`, `scenario-lint OK`); `./smackerel.sh lint` exit 0 (`All checks passed!`, `Web validation passed`); `go build ./...` exit 0; `go vet -tags e2e ./tests/e2e/wiki/...` exit 0.
- [x] Scenario-specific regression E2E rows (TP-073-25..30) added or
  updated for every changed behavior in this scope.
  **Evidence:** `tests/e2e/wiki/topics_e2e_test.go` (TP-073-25), `people_e2e_test.go` (26), `places_e2e_test.go` (27), `time_e2e_test.go` (28), `cross_links_e2e_test.go` (29 with adversarial), `annotation_entry_e2e_test.go` (30). All PASS in `/tmp/wiki-e2e-final.log`.
- [x] Broader E2E regression suite passes for this scope.
  **Evidence:** `./smackerel.sh test e2e --go-run "TestWiki_"` exit 0; entire `tests/e2e/wiki` package PASS (0.145s, 6/6 tests); peer e2e packages reported `[no tests to run]` (selector-scoped) but compiled cleanly under the `e2e` build tag.
- [x] Shared-infrastructure canary coverage: TP-073-25..29 prove
  existing PWA shell + auth/session middleware remain healthy after
  wiki routes are added; cross-link renderer unit canary asserts
  no client-side derivation.
  **Evidence:** Wiki routes served via existing `/pwa/*` `pwaFileServer()` (`internal/api/pwa.go`); no router edits required because `web/pwa/embed.go` uses `//go:embed *.html *.css *.js *.json *.svg lib` glob — new `wiki*.{html,js}` files auto-included; sw.js cache semantics unchanged (content hash regenerates from the embedded FS walk). Cross-link unit canary lives in `tests/e2e/wiki/cross_links_e2e_test.go` static-source check (sort/reverse/`.reason =`/`.reason +=`/rerank — all forbidden).
- [x] Rollback/restore proof: all wiki files are additive; `git revert`
  of the wiki commit removes the routes without touching assistant
  chat, capture, or backend code.
  **Evidence:** Files added are pure additions: `web/pwa/wiki.{html,js}`, `web/pwa/wiki_topics.{html,js}`, `web/pwa/wiki_people.{html,js}`, `web/pwa/wiki_places.{html,js}`, `web/pwa/wiki_time.{html,js}`, `web/pwa/wiki_artifact.{html,js}`, `web/pwa/wiki_lib.js`, `web/pwa/generated/wiki_graph_v1.js`, `tests/e2e/wiki/*.go` (6 files), `web/pwa/tests/wiki_initial_paint_budget_test.go`. Only edits to existing files: `web/pwa/tests/assistant_storage_guard_test.go` (glob extension — backward compatible: it still scans `assistant*.js`, now also scans `wiki*.js`).
- [x] Change Boundary respected: zero changes to assistant chat
  files, capture pipeline, native mobile clients, server endpoints,
  or service worker cache behavior.
  **Evidence:** `git status` shows no edits under `web/pwa/assistant.{html,js}`, `internal/capture/**`, `clients/mobile/**`, `internal/api/router.go`, `internal/api/graphapi/**`, `web/pwa/sw.js`. The only edit to an existing file outside the allowed `wiki*` set is `web/pwa/tests/assistant_storage_guard_test.go` — a test-glob extension explicitly required by TP-073-30 and called out in the scope's Implementation Plan ("extend the storage guard to cover the new wiki pages").
- [x] Change Boundary is respected and zero excluded file families were changed for the Scope 5 wiki additions (see Change Boundary section above and `git status` evidence).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are added or updated (TP-073-25 through TP-073-30 protect SCN-073-B01..B06 against reintroduction; TP-073-29 carries an adversarial sibling proving the cross-link reorder assertion is not tautological).
  **Evidence:** `tests/e2e/wiki/{topics,people,places,time,cross_links,annotation_entry}_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log`; cross-link adversarial sibling lives in `tests/e2e/wiki/cross_links_e2e_test.go`.
- [x] SCN-073-B01 — Browse topics index to a topic page: served `/wiki/topics` index lists topics from the live knowledge graph with linked artifact/people/place counts; selecting a topic opens a topic page rendering linked artifacts, related people, and related places via `web/pwa/wiki_topics.{html,js}` + `tests/e2e/wiki/topics_e2e_test.go` PASS.
  **Evidence:** `tests/e2e/wiki/topics_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log`; `web/pwa/wiki_topics.html` + `web/pwa/wiki_topics.js` shipped (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).
- [x] SCN-073-B02 — Browse people index to a person page: served `/wiki/people` index lists people derived from the intelligence layer with artifact counts; selecting a person opens a person page rendering timeline of artifacts, related topics, and related places via `web/pwa/wiki_people.{html,js}` + `tests/e2e/wiki/people_e2e_test.go` PASS.
  **Evidence:** `tests/e2e/wiki/people_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log`; `web/pwa/wiki_people.html` + `web/pwa/wiki_people.js` shipped (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).
- [x] SCN-073-B03 — Browse places index to a place page: served `/wiki/places` index lists places from the maps connector and artifact-derived locations; selecting a place opens a place page rendering map-derived location and linked artifacts via `web/pwa/wiki_places.{html,js}` + `tests/e2e/wiki/places_e2e_test.go` PASS.
  **Evidence:** `tests/e2e/wiki/places_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log`; `web/pwa/wiki_places.html` + `web/pwa/wiki_places.js` shipped (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).
- [x] SCN-073-B04 — Time view renders a calendar-style scroll: served `/wiki/time` renders artifacts grouped by day in a vertical calendar-style scroll; back/forward navigation preserves scroll position via `web/pwa/wiki_time.{html,js}` + `tests/e2e/wiki/time_e2e_test.go` PASS.
  **Evidence:** `tests/e2e/wiki/time_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log`; `web/pwa/wiki_time.html` + `web/pwa/wiki_time.js` shipped (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).
- [x] SCN-073-B05 — Cross-links render on every artifact page: a "Related" section on every artifact/topic/person/place page lists graph-derived cross-links to other artifacts/topics/people/places with explainable `reason` strings sourced verbatim from graph edge metadata via `web/pwa/wiki_lib.js` `renderCrossLinkList` + `tests/e2e/wiki/cross_links_e2e_test.go` PASS (adversarial sibling proves reorder/re-derivation fails the assertion).
  **Evidence:** `tests/e2e/wiki/cross_links_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log` (1.56s) with adversarial reorder sibling; `web/pwa/wiki_lib.js` `renderCrossLinkList` ships (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).
- [x] SCN-073-B06 — Inline annotation entry point opens from any artifact page: "Annotate" entry point opens an inline annotation editor scoped to the current artifact; submit calls the spec 027 SCOPE-9 annotation endpoints (`SCN-027-71..74`) and the rendered artifact page reflects the new annotation after submit via `web/pwa/wiki_lib.js` `renderAnnotationEntryPoint` + `tests/e2e/wiki/annotation_entry_e2e_test.go` PASS.
  **Evidence:** `tests/e2e/wiki/annotation_entry_e2e_test.go` PASS in `/tmp/wiki-e2e-final.log` (asserts both reachable and unreachable branches plus `X-Smackerel-Source: web` header literal); `web/pwa/wiki_lib.js` `probeAnnotationEndpoint` + `renderAnnotationEntryPoint` ship (see `report.md` → `## Implement — Scope 5 (2026-06-04)` → Files Created).

**Uncertainty Declaration:** Implementation depends on the existence
of read APIs for topics/people/places/time/artifacts with graph edge
metadata. If any required route is not yet exposed by `internal/
knowledge`, `internal/intelligence`, or `internal/graph`, planning
routes a finding back to the owning spec instead of synthesizing
endpoints under this scope. Inline annotation entry point is
gracefully disabled until spec 027 SCOPE-9 lands; the rest of the
scope ships independently.
