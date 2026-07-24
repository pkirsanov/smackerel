# Scopes: [BUG-073-006] Auth Rejection Leaves Blank Assistant Response

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> Reconciled by `bubbles.plan` on 2026-07-24 against the current [spec.md](spec.md)
> (10 scenarios, `ASST-UI-001`..`ASST-UI-012`) and [design.md](design.md). The prior
> single-scope shape predated `SCN-073-006-10` and under-declared the cross-packet
> dependencies on `BUG-070-001` (auth session) and `BUG-102-001` (fault-profile
> registry). This plan restores `SCN-073-006-10` and binds the assistant surface to
> `BUG-070-001`'s now-authoritative unified session.
>
> Realigned by `bubbles.plan` on 2026-07-24 to close independent-review finding **F1
> (MEDIUM)**: `SCOPE-01` no longer authors a parallel fault-profile registry. It now
> CONSUMES the `BUG-102-001`-owned production-inert, test-only, machine-readable
> fault-profile registry (its `SCOPE-01`/`SCN-102-001-12`) by immutable `stableId`,
> owns only the eight assistant-specific fault PROFILES layered on it, and re-declares
> none of the `BUG-102-001`-owned nine-field schema — matching [design.md](design.md)
> `## Fault Registry Consumption`. `SCOPE-02` (honest terminal states) is unchanged
> except where it references the registry.

## Execution Outline

### Phase Order

1. **Scope 01 — Consume BUG-102-001 Fault-Profile Registry (Assistant Fault Profiles) & Production-Inert Verification.**
   CONSUME the `BUG-102-001`-owned production-inert, test-only, machine-readable
   fault-profile registry (its `SCOPE-01`/`SCN-102-001-12`): run the eight
   assistant-journey fault profiles (`auth-rejection`, `scope-denial`, `rate-limit`,
   `provider-failure`, `server-failure`, `timeout`, `network-loss`, `invalid-envelope`)
   by their `BUG-102-001`-owned `stableId` at owned real-stack boundaries, and verify no
   consumer exposes a fault control in production. This scope authors NO registry and
   re-declares NO schema; it owns only the assistant-specific profile consumption. It
   answers the design's routed `bubbles.plan` realignment (see [design.md](design.md)
   `## Fault Registry Consumption` and `## Routed Design Questions`).
2. **Scope 02 — Exhaustive Honest Assistant Turn Terminal States.**
   The user-facing fix: one paired pending row per submission, exactly one honest
   terminal outcome, typed non-2xx/schema/timeout/network handling, deduplicated
   retry, privacy, and accessibility — validated on the real stack against Scope 01's
   fault boundaries and `BUG-070-001`'s unified session.

### New Types & Signatures (planning-level; owner design confirms shape)

- **Consumed fault-profile reference (BUG-102-001-owned schema):** the eight assistant
  faults are entries in the `BUG-102-001` registry, referenced by immutable `stableId`
  only. `BUG-102-001` owns the closed nine-field schema — `stableId`, `journey`,
  `setup`, `teardown`, `parallelism`, `expectedRequest`, `expectedResponseOrTermination`,
  `evidence`, `noFirstPartyInterception` — and this packet re-declares, renames, or forks
  none of it (`ASST-UI-011`, consume-only). Production build/config/route/UI/request
  schema carry NO fault control (`ASST-UI-012`).
- **Closed `TurnOutcome`:** `kind ∈ { answer, clarification, confirmation, refusal,
  capture, auth_401, access_403, rate_limited, request_rejected, provider_unavailable,
  server_error, timeout, network, schema_decode }` + `safeMessage, retryable,
  signInRequired, response, requestId` (design "Closed Turn Outcome"). Raw bodies /
  exception strings are never fields.
- **In-memory `AssistantTurn`:** one paired group keyed by `transport_message_id`
  (`pending → retrying → terminal`); memory-only, no persistence.
- **Cross-packet binding (BUG-070-001):** the assistant's same-origin session is the
  ONE unified claim-bound PASETO session accepted by legacy pages, `/api`, and `/v1`;
  the state-writing capture path carries the `MutationTrustGuard` anti-CSRF proof;
  typed CSRF/auth outcomes `origin_rejected | csrf_missing | csrf_stale | csrf_mismatch |
  accepted` and 401/403 map to honest `auth_401` / `access_403` / `request_rejected`
  renders — never a blank or success/capture render.

### Validation Checkpoints

- After Scope 01: `tests/e2e/assistant/http_error_test.go` + `tests/e2e/assistant/http_live_stack_test.go`
  prove every assistant fault profile CONSUMED from the `BUG-102-001` registry by
  `stableId` injects at an owned boundary in parallel with no first-party interception;
  `tests/integration/policy/no_defaults_go_guard_test.go` +
  `tests/config/assistant_config_generate_test.sh` prove production inertness and that no
  profile or nine-field schema is authored in this packet — BEFORE the honest-render fix
  is validated against those faults.
- After Scope 02: the browser canary (`web/pwa/tests/assistant_chat.spec.ts`,
  `web/pwa/tests/assistant_retry.spec.ts`, `web/pwa/tests/assistant_accessibility.spec.ts`)
  runs before the broad `tests/e2e/assistant_regression_e2e_test.sh`, and the adversarial
  pre-facade regression (`cmd/core/wiring_assistant_http_prefacade_regression_test.go`)
  fails if the old blank terminal returns.

## Scope Ordering Rationale

The bug's user-facing outcome (honest auth-rejection render) is Scope 02, but it is
sequenced AFTER Scope 01 because `SCN-073-006-10`/`ASST-UI-011` require the real-stack
faults to be driven by the `BUG-102-001`-owned, test-only, machine-readable
fault-profile registry consumed by `stableId` — not by first-party Playwright
interception, which the design and spec forbid for live scenarios. Consuming the owned
fault profiles first is design-faithful and gives Scope 02's live e2e-ui coverage a real
place to inject `401/403/429/5xx/timeout/network/invalid-envelope`. Scope 01 has no
intra-packet predecessor, so the DAG pickup rule ("lowest-numbered eligible") starts at
Scope 01; its cross-packet consumption of the `BUG-102-001` registry foundation is
tracked in `specDependsOn` and the Scope 01 Cross-Packet Binding note. Scope 01 then
unlocks Scope 02 (`dependsOn: SCOPE-01` + cross-packet `BUG-070-001`).

## Scope Summary

| # | Scope | Scenarios | Surfaces | Test rows / test DoD items | Depends On | Status |
|---|---|---|---|---|---|---|
| 01 | Consume BUG-102-001 Fault-Profile Registry (Assistant Fault Profiles) & Production-Inert Verification | SCN-073-006-10 | Test infra consuming the `BUG-102-001` registry + prod config/build guard | 4 / 4 | `BUG-102-001` (registry foundation, cross-packet) | Not Started |
| 02 | Exhaustive Honest Assistant Turn Terminal States | SCN-073-006-01..09 | Web PWA `/pwa/assistant.html` (desktop + mobile viewport), `POST /api/assistant/turn` transport | 13 / 13 | SCOPE-01 + `BUG-070-001` | Not Started |

## Scope 01: Consume BUG-102-001 Fault-Profile Registry (Assistant Fault Profiles) & Production-Inert Verification

**Status:** Not Started
**Priority:** P0
**Depends On:** `BUG-102-001` (`SCOPE-01`/`SCN-102-001-12` production-inert, test-only, machine-readable fault-profile registry foundation, consumed by `stableId`). No intra-packet predecessor; unlocks Scope 02's live-stack faults.

### Cross-Packet Binding: BUG-102-001 (fault-profile registry FOUNDATION owner)

Per capability-foundation-design the broadest consumer owns the shared foundation:
`BUG-102-001`
(`specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap`)
exercises the widest fault surface (every required product journey), so it OWNS the
shared, production-inert, test-only, machine-readable fault-profile registry — its
`SCOPE-01` / `SCN-102-001-12`, e.g. `config/acceptance/fault-profiles.v1.yaml` plus its
JSON Schema — including the closed nine-field profile schema (`stableId`, `journey`,
`setup`, `teardown`, `parallelism`, `expectedRequest`, `expectedResponseOrTermination`,
`evidence`, `noFirstPartyInterception`) and its "Consumer Contract (BUG-073-006 And
Other Journey Owners)". This scope is a PURE CONSUMER of that foundation:

- It references the eight assistant-journey faults by their immutable
  `BUG-102-001`-owned `stableId` only; it authors no registry, defines no profile
  inline, and re-declares/renames/forks none of the nine-field schema.
- It runs each referenced profile in this packet's disposable `env=test*` stack using the
  profile's `BUG-102-001`-owned `setup`/`teardown`/`parallelism`, and asserts its
  `expectedRequest`/`expectedResponseOrTermination` through real stack behavior with
  `noFirstPartyInterception` honored — the one-way "consume, never own" contract.
- A missing/unknown `stableId` fails and never falls back to an inline definition.
- `BUG-102-001` is read-only here; this packet edits none of its artifacts. The
  dependency is recorded in `state.json` `specDependsOn`. Design authority:
  [design.md](design.md) `## Fault Registry Consumption`.

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-006-10 Consumed BUG-102-001 fault profiles are honored and production-inert
  Given a disposable validate/e2e stack and the BUG-102-001-owned test-only machine-readable fault-profile registry
  When this packet runs the eight assistant fault profiles (auth-rejection, scope-denial, rate-limit, provider-failure, server-failure, timeout, network-loss, invalid-envelope) referenced by their BUG-102-001-owned stableId
  Then each profile uses its BUG-102-001-owned setup, teardown, and parallelism and produces its declared expectedRequest and expectedResponseOrTermination through real stack behavior without first-party request interception
  And this packet authors no parallel registry and re-declares no schema field
  And no consumer exposes a fault control in production configuration, routes, requests, or UI
```

### Implementation Plan

- CONSUME the `BUG-102-001`-owned production-inert, test-only, machine-readable
  fault-profile registry (its `SCOPE-01`/`SCN-102-001-12`, e.g.
  `config/acceptance/fault-profiles.v1.yaml` plus its JSON Schema). This packet does NOT
  create, author, edit, or fork that registry, and re-declares/renames none of its closed
  nine-field schema (`stableId`, `journey`, `setup`, `teardown`, `parallelism`,
  `expectedRequest`, `expectedResponseOrTermination`, `evidence`,
  `noFirstPartyInterception`).
- Reference the eight assistant-journey fault profiles by their immutable
  `BUG-102-001`-owned `stableId` only: auth-rejection→`auth_401`, scope-denial→`access_403`,
  rate-limit→`rate_limited`, provider-failure→`provider_unavailable`,
  server-failure→`server_error`, timeout→`timeout`, network-loss→`network`,
  invalid-envelope→`schema_decode`. A missing or unknown `stableId` fails and never falls
  back to an inline definition (`ASST-UI-011`, consume-only).
- Run each referenced profile in this packet's disposable `env=test*` stack using the
  profile's `BUG-102-001`-owned `setup`/`teardown`/`parallelism`, and assert its
  `expectedRequest`/`expectedResponseOrTermination` at the owned real-stack boundaries the
  design routes to `bubbles.plan`: router `bearerAuthMiddleware` 401, `auth.RequireScope`
  403, `httpadapter.PreFacadeChain` 429/body, facade/provider `unavailable` envelope, 5xx,
  client-deadline timeout, fetch-reject network loss, and a contract-invalid v1 envelope —
  each a REAL request/response, honoring `noFirstPartyInterception` (never a
  first-party-intercepted canned response).
- Verify the shared production-inertness invariant (`ASST-UI-012`): the generated production
  config, routes, request schema, and UI expose NO fault control; a production build that can
  select, inject, or trigger a fault FAILS acceptance.
- Bind the consumed `auth-rejection` profile (`stableId` → `auth_401`) to `BUG-070-001`'s
  unified claim-bound PASETO session semantics (missing / expired / revoked session), so
  Scope 02's `SCN-073-006-02` and `SCN-073-006-03` consume a truthful auth/CSRF rejection at
  an owned boundary.

### Shared Infrastructure Impact Sweep

- The `BUG-102-001`-owned fault-profile registry (CONSUMED here, not owned), this packet's
  disposable validate/e2e stack composition that runs the referenced profiles, the owned
  real-stack fault boundaries, parallel-run isolation per the profile's `BUG-102-001`-owned
  `parallelism`, and the production config/build guard. Contract surfaces: every
  live-category scenario in Scope 02 that consumes a referenced fault profile, and the
  upstream `BUG-102-001` registry whose stability this packet depends on.
- Canary before broad reruns: the fault-consumption boundary tests run before the broad
  assistant regression suite because they are shared high-fan-out test infrastructure.

### Change Boundary

- Allowed after owner design: this packet's consumption of the `BUG-102-001`-owned registry
  by `stableId`, the assistant-specific fault-profile consumption tests in disposable
  validate/e2e stacks, the production-inert config/build guard consumed as a shared
  invariant, and directly affected test docs.
- Excluded: authoring, editing, or forking the `BUG-102-001`-owned fault-profile registry or
  re-declaring/renaming its nine-field schema; any `BUG-102-001` artifact; production
  configuration/routes/UI/request schema gaining any fault control; facade business
  semantics; unrelated PWA pages; operator-owned deployment assets; spec 104; and specs
  105/106.

### Test Plan

| Test Type | Category | File | Scenario / Assertion | Command | Live System |
|---|---|---|---|---|---|
| E2E API | e2e-api | tests/e2e/assistant/http_error_test.go | SCN-073-006-10 each of the eight assistant fault profiles consumed from the `BUG-102-001` registry by `stableId` injects at an owned real-stack boundary using the profile's 102-owned setup/teardown; real request/response; no first-party interception | ./smackerel.sh test e2e | Yes |
| E2E API | e2e-api | tests/e2e/assistant/http_live_stack_test.go | Consumed profiles run in parallel per their `BUG-102-001`-owned parallelism on the disposable live stack; only allowed evidence emitted | ./smackerel.sh test e2e | Yes |
| Integration | integration | tests/integration/policy/no_defaults_go_guard_test.go | Production build/config/route/UI/request schema exposes NO fault control (shared consumer invariant); a prod build that can select/inject/trigger a fault fails acceptance | ./smackerel.sh test integration | Yes |
| Unit | unit | tests/config/assistant_config_generate_test.sh | Generated production config carries no fault-control keys; assistant faults are referenced by `BUG-102-001`-owned `stableId` only — this packet authors no inline profile and re-declares no schema field | ./smackerel.sh test unit | No |

### Adversarial Regression Contract

A production build or profile that can select, inject, or trigger a fault MUST fail
acceptance. A fault profile that satisfies its expected outcome via first-party request
interception (`page.route` / `context.route` / `intercept` / `msw` / `nock`) MUST fail — the
outcome must traverse the real transport at an owned boundary. A change that authors a
parallel registry in this packet, defines a profile inline, or re-declares/renames the
`BUG-102-001`-owned nine-field schema MUST fail this scope's intent — the eight assistant
faults are referenced by `BUG-102-001`-owned `stableId` only.

### Definition of Done - Tiered Validation

- [ ] SCN-073-006-10 — each of the eight assistant fault profiles (auth-rejection, scope-denial, rate-limit, provider-failure, server-failure, timeout, network-loss, invalid-envelope) is consumed from the `BUG-102-001` registry by its owned `stableId` and injects at an owned real-stack boundary using the profile's 102-owned setup/teardown with NO first-party interception. → Evidence: [report.md](report.md) (e2e-api tests/e2e/assistant/http_error_test.go)
- [ ] Consumed profiles run in parallel per their `BUG-102-001`-owned parallelism on the disposable live stack and emit only allowed evidence. → Evidence: [report.md](report.md) (e2e-api tests/e2e/assistant/http_live_stack_test.go)
- [ ] Production build/config/route/UI/request schema exposes NO fault control (shared consumer invariant `ASST-UI-012`); a prod build that can select/inject/trigger a fault fails acceptance. → Evidence: [report.md](report.md) (integration tests/integration/policy/no_defaults_go_guard_test.go)
- [ ] Generated production config carries no fault-control keys and the assistant faults are referenced by `BUG-102-001`-owned `stableId` only — this packet authors no inline profile and re-declares no schema field. → Evidence: [report.md](report.md) (unit tests/config/assistant_config_generate_test.sh)
- [ ] Consumption architecture confirmed by [design.md](design.md) `## Fault Registry Consumption`: `BUG-102-001` OWNS the registry (its `SCOPE-01`/`SCN-102-001-12`); this packet is a pure consumer — no parallel registry, no schema fork.
- [ ] The eight assistant fault conditions are addressed by immutable `BUG-102-001`-owned `stableId` only; a missing/unknown `stableId` fails and never falls back to an inline definition.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Build Quality Gate: zero warnings, lint/format clean, artifact lint clean, docs aligned.

## Scope 02: Exhaustive Honest Assistant Turn Terminal States

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 01 (consumed real-stack fault profiles from the `BUG-102-001` registry) and cross-packet `BUG-070-001` (unified claim-bound PASETO session; the assistant's auth-rejection rendering binds to it).

### Cross-Packet Binding: BUG-070-001 (authoritative auth contract)

`BUG-070-001` (`specs/070-web-username-password-login/bugs/BUG-070-001-production-credential-session-paseto-split`)
is the now-authoritative, non-conflicting auth foundation this scope binds to:

- The assistant's same-origin session IS `BUG-070-001`'s ONE claim-bound PASETO session,
  accepted uniformly by legacy pages, `/api`, and `/v1` (no surface-specific cookie some
  surfaces reject). This closes the class of drift that let the assistant's
  `POST /api/assistant/turn` session be rejected while the row rendered blank.
- The state-writing capture (`saved_as_idea`) path is a MUTATION and carries the
  server-validated, session-bound anti-CSRF proof enforced by `MutationTrustGuard`
  (Origin allowlist → proof present → double-submit cross-check → HMAC signature+binding;
  403 before any state change). Its typed outcomes `origin_rejected | csrf_missing |
  csrf_stale | csrf_mismatch | accepted` and a plain scope 403 all render as honest
  `access_403` / `request_rejected` failure rows (`ASST-UI-002`/`ASST-UI-003`), never blank.
- An auth-rejected assistant call (401 missing/expired/revoked unified session; 403 scope
  denial; 403 CSRF rejection) MUST surface a typed, honest failure UI — visible re-auth or
  access guidance — and MUST NOT render blank, "saved as an idea", or success styling
  (`ASST-UI-007`, `SCN-073-006-02`, `SCN-073-006-06`).
- Full production-login real-stack Playwright for the auth paths is sequenced AFTER
  `BUG-070-001` delivers the unified session; no bearer injection or first-party interception
  may substitute for real production browser trust.

Surface scope: the fix is the responsive Web PWA assistant (`/pwa/assistant.html` +
`web/pwa/assistant.js`) at desktop AND mobile/narrow viewport (320px / 200% zoom,
`SCN-073-006-09`). Per [design.md](design.md), native mobile render-descriptor consumers
retain their current renderer via the unchanged shared `assistant_turn_v1` contract; if
native-mobile honest-failure parity is required as a separate deliverable that is a design
expansion (see report.md finding-accounting → routed to `bubbles.design`).

### Gherkin Scenarios

```gherkin
Scenario: SCN-073-006-01 Successful turn remains complete
  Given an authenticated user on the unified BUG-070-001 session and a healthy Assistant
  When the user submits one message
  Then one pending turn becomes exactly one visible supported terminal outcome
  And exactly one real POST is issued

Scenario: SCN-073-006-02 Pre-facade auth rejection is visible
  Given the unified BUG-070-001 PASETO session is missing, expired, or revoked before the facade
  When the user submits a message
  Then the user message stays paired with an inline 401 re-authentication error
  And no blank Assistant row remains and no capture or success styling appears

Scenario: SCN-073-006-03 Non-2xx and schema failures are typed
  Given the server returns a 403 scope denial, 429, other 4xx, 5xx, a MutationTrustGuard CSRF rejection, or a malformed envelope
  When the client processes the response
  Then a distinct non-sensitive typed error and retry are visible

Scenario: SCN-073-006-04 Network and timeout preserve retry context
  Given the request loses connectivity or exceeds its timeout at an owned boundary
  When the request terminates
  Then the transcript and original safe input remain and retry is available without duplicate submission

Scenario: SCN-073-006-05 Retry is idempotent from the user's perspective
  Given a failed turn exposes a retry action
  When the user activates retry once or repeatedly during the pending state
  Then at most one request is active and one terminal outcome is appended with the same logical turn identity

Scenario: SCN-073-006-06 Failure never becomes capture or success
  Given a high-band Assistant turn is rejected before or after facade execution
  When the UI renders the outcome
  Then it shows neither the saved-as-an-idea acknowledgement nor success styling

Scenario: SCN-073-006-07 Empty transcript remains honest
  Given no message has been submitted
  When Assistant opens
  Then it shows an accessible initial invitation with no fabricated conversation

Scenario: SCN-073-006-08 Error states protect privacy
  Given an auth or server response carries internal diagnostic detail
  When the client renders and logs the failure
  Then credentials, tokens, full prompts, and raw internal bodies are absent from DOM, attributes, and logs

Scenario: SCN-073-006-09 Assistant failures are accessible and responsive
  Given a keyboard or screen-reader user at 320px and 200% zoom
  When pending, error, retry, or re-authentication renders
  Then status is announced, focus stays predictable, and controls do not overlap
```

### Implementation Plan

- Replace the single global `#assistant-response`/`#assistant-error` panel with a transcript
  of paired turn groups; each submission atomically creates one in-memory `AssistantTurn`
  (user row + immediately-visible pending Assistant row) keyed by `transport_message_id`.
- Build `client_context` as `{}` and run request validation INSIDE the guarded turn
  transition so a local contract regression becomes a terminal `schema_decode` row with zero
  network requests, never an unpaired blank row.
- Normalize transport: parse safe v1 envelopes on any status; classify outer middleware
  errors (`401` unified-session rejection, `403` scope/CSRF, `429`, other `4xx`, `5xx`),
  provider `unavailable`, `AbortError` timeout, fetch-reject network, and invalid/ID-mismatch
  envelope into the closed `TurnOutcome`. Raw bodies/exception strings never reach DOM or logs.
- Render into the turn-specific Assistant row; clear incompatible sources/choices/confirm
  controls on each transition; attach sources only after full schema validation.
- Retry reuses the exact validated request + `transport_message_id`, is single-flight, updates
  the same paired row through `retrying`, and never appends a duplicate user row; explicit
  re-authentication (separate-tab `/login?next=/assistant`) never auto-resubmits.
- Bind auth/CSRF rejection rendering to `BUG-070-001` per the Cross-Packet Binding section.

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected user-visible state | Test Type | Evidence |
|---|---|---|---|---|---|
| SCN-073-006-02 auth rejection | Missing/expired/revoked unified BUG-070-001 session | Submit message | User row stays paired; honest 401 re-auth row; `Sign in again`; no blank/capture/success | e2e-ui | report.md#scenario-contract-evidence |
| SCN-073-006-03 typed non-2xx/CSRF/schema | Real 403/429/4xx/5xx/CSRF/invalid envelope at owned boundary | Submit message | Distinct typed safe copy + retry per class | e2e-ui | report.md#scenario-contract-evidence |
| SCN-073-006-04 network/timeout | Owned network-loss and timeout boundaries | Submit; wait; retry | Distinct network vs timeout copy; transcript/input preserved; one retry | e2e-ui | report.md#scenario-contract-evidence |
| SCN-073-006-09 accessible narrow retry | Keyboard/screen reader at 320px / 200% zoom | Submit failed turn; activate retry | One announcement per transition; predictable focus; no overlap/horizontal scroll | e2e-ui | report.md#scenario-contract-evidence |

### Shared Infrastructure Impact Sweep

- Assistant transcript reducer, request identity (`transport_message_id`), unified-session
  auth handling, retry single-flight, source/citation rendering, capture/refusal semantics,
  service-worker/network behavior, accessibility announcements. Downstream contract surfaces:
  spec 106 Assistant journey and BUG-102-001 acceptance (blocked on this packet).
- Canary + rollback: the focused browser canary runs before broad assistant E2E; rollback is
  a narrow renderer/telemetry pointer-swap to the immediately preceding revision without
  touching facade, wire schema, database, auth middleware, or server dedup.

### Change Boundary

- Allowed after owner design: Assistant client transport/state/rendering (`web/pwa/assistant.html`,
  `web/pwa/assistant.js`), focused client/API tests, redacted browser-turn telemetry, and
  directly affected docs.
- Excluded: facade business semantics except contract consumption, `assistant_turn_v1` schema
  or its generated validators, native mobile render-descriptor renderer, unrelated PWA pages,
  production data, operator-owned deployment assets, spec 104, and specs 105/106.

### Test Plan

| Test Type | Category | File | Scenario / Assertion | Command | Live System |
|---|---|---|---|---|---|
| E2E UI | e2e-ui | web/pwa/tests/assistant_chat.spec.ts | SCN-073-006-01 one pending row resolves to one non-empty supported terminal; exactly one real POST | ./smackerel.sh test e2e-ui | Yes |
| E2E UI | e2e-ui | web/pwa/tests/assistant_retry.spec.ts | SCN-073-006-02 missing/expired/revoked BUG-070-001 unified PASETO session renders honest 401 re-auth row; user message stays paired; no blank | ./smackerel.sh test e2e-ui | Yes |
| E2E UI | e2e-ui | web/pwa/tests/assistant_retry.spec.ts | SCN-073-006-03 403 scope denial, 429, other 4xx, 5xx, MutationTrustGuard CSRF-403, and malformed envelope render distinct typed errors with retry | ./smackerel.sh test e2e-ui | Yes |
| E2E UI | e2e-ui | web/pwa/tests/assistant_retry.spec.ts | SCN-073-006-04 network and timeout are distinct, preserve transcript/input, and expose retry | ./smackerel.sh test e2e-ui | Yes |
| E2E API | e2e-api | tests/e2e/assistant/web_pwa_retry_e2e_test.go | SCN-073-006-05 retry reuses one logical turn identity; server dedup yields one result; no duplicate row | ./smackerel.sh test e2e | Yes |
| E2E UI | e2e-ui | web/pwa/tests/assistant_chat.spec.ts | SCN-073-006-06 high-band failure never shows saved-as-an-idea or success styling | ./smackerel.sh test e2e-ui | Yes |
| E2E UI | e2e-ui | web/pwa/tests/assistant_chat.spec.ts | SCN-073-006-07 empty transcript shows an accessible invitation and zero fabricated rows | ./smackerel.sh test e2e-ui | Yes |
| Unit | unit | web/pwa/tests/assistant_storage_guard_test.go | SCN-073-006-08 no token/transcript persistence and no raw internal detail in DOM/attributes/logs | ./smackerel.sh test unit --go | No |
| E2E UI | e2e-ui | web/pwa/tests/assistant_accessibility.spec.ts | SCN-073-006-09 one announcement per transition, predictable focus, no overlap at 320px/200% zoom | ./smackerel.sh test e2e-ui | Yes |
| Unit | unit | web/pwa/tests/assistant_robustness_guard_test.go | Closed TurnOutcome normalizer matrix incl. response-ID mismatch, invalid-capture combo, client_context {} schema | ./smackerel.sh test unit --go | No |
| Integration | integration | tests/integration/api/assistant_http_auth_test.go | Pre-facade 401 vs 403 vs MutationTrustGuard CSRF-403 typed contracts bind to the BUG-070-001 unified session | ./smackerel.sh test integration | Yes |
| Canary | functional | cmd/core/wiring_assistant_http_prefacade_regression_test.go | Adversarial pre-facade rejection stays distinguishable; old blank terminal node must fail | ./smackerel.sh test unit --go | No |
| Broader E2E Regression | e2e-api | tests/e2e/assistant_regression_e2e_test.sh | Existing answer/refusal/capture semantics remain distinct | ./smackerel.sh test e2e | Yes |

### Adversarial Regression Contract

Reject a real Assistant POST at the unified-session auth boundary BEFORE facade invocation
and assert the same submitted message receives a visible typed 401 re-auth row plus retry.
The test MUST fail if the old blank terminal node, a false `saved as an idea`/success
render, or a duplicate retry submission returns. `SCN-073-006-02`'s regression MUST NOT use
first-party interception and MUST NOT early-return on a `/login` redirect (assert
`not blank` and the re-auth control is visible).

### Definition of Done - Tiered Validation

- [ ] SCN-073-006-01 — one pending Assistant row resolves to exactly one non-empty supported terminal outcome with exactly one real POST. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_chat.spec.ts)
- [ ] SCN-073-006-02 — a missing/expired/revoked BUG-070-001 unified PASETO session renders an honest 401 re-authentication row; the user message stays paired; no blank/capture/success. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_retry.spec.ts)
- [ ] SCN-073-006-03 — 403 scope denial, 429, other 4xx, 5xx, MutationTrustGuard CSRF-403, and a malformed envelope each render a distinct typed error with retry. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_retry.spec.ts)
- [ ] SCN-073-006-04 — network and timeout are distinct, preserve transcript/input, and expose retry. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_retry.spec.ts)
- [ ] SCN-073-006-05 — retry reuses one logical turn identity and server dedup yields one result with no duplicate row. → Evidence: [report.md](report.md) (e2e-api tests/e2e/assistant/web_pwa_retry_e2e_test.go)
- [ ] SCN-073-006-06 — a high-band failure never shows the saved-as-an-idea acknowledgement or success styling. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_chat.spec.ts)
- [ ] SCN-073-006-07 — an empty transcript shows an accessible invitation and zero fabricated rows. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_chat.spec.ts)
- [ ] SCN-073-006-08 — no token or transcript persists and no raw internal detail appears in DOM/attributes/logs. → Evidence: [report.md](report.md) (unit web/pwa/tests/assistant_storage_guard_test.go)
- [ ] SCN-073-006-09 — one announcement per transition, predictable focus, and no overlap at 320px/200% zoom. → Evidence: [report.md](report.md) (e2e-ui web/pwa/tests/assistant_accessibility.spec.ts)
- [ ] Closed TurnOutcome normalizer matrix (response-ID mismatch, invalid-capture combo, client_context {} schema) passes. → Evidence: [report.md](report.md) (unit web/pwa/tests/assistant_robustness_guard_test.go)
- [ ] Pre-facade 401 vs 403 vs MutationTrustGuard CSRF-403 typed contracts bind to the BUG-070-001 unified session. → Evidence: [report.md](report.md) (integration tests/integration/api/assistant_http_auth_test.go)
- [ ] Adversarial pre-facade regression stays distinguishable and the old blank terminal node fails. → Evidence: [report.md](report.md) (canary cmd/core/wiring_assistant_http_prefacade_regression_test.go)
- [ ] Broader E2E regression keeps answer/refusal/capture semantics distinct. → Evidence: [report.md](report.md) (e2e-api tests/e2e/assistant_regression_e2e_test.sh)
- [ ] `bubbles.design` confirms the pre-facade client-state root cause and the closed exhaustive terminal-outcome model.
- [ ] Assistant binds to the BUG-070-001 unified claim-bound PASETO session: 401 / 403 / MutationTrustGuard CSRF-403 rejection renders an honest typed failure UI, never a blank or success/capture render.
- [ ] Retry preserves safe context and cannot duplicate active requests or transcript turns.
- [ ] Rollback/restore path for the shared Assistant renderer is documented and verified.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Build Quality Gate: zero warnings, lint/format clean, artifact lint clean, docs aligned.
