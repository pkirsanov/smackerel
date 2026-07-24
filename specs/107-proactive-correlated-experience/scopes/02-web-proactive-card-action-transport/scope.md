# SCOPE-02: Web Proactive Card & Authenticated Action Transport

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-01

## Outcome

Render one `ProactiveCardModel` as a spec-106 Pending-action-row on the web PWA —
title, a plain-language "why am I seeing this" provenance line derived from its
real producer and cause, an `Available`/`Degraded` availability badge, one-tap
`[Act][Snooze][Dismiss]` (≥44×44px), and `[Why ▾]` — and route the web action as
an authenticated same-origin `{nudgeRef, action}` mutation through the foundation
`NudgeAck` path, composed over spec-106's `AuthenticatedRequestAdapter` and
`MutationFeedbackPresenter`, with no bearer token in JavaScript and no new
surfacing path.

## Requirements And Scenarios

- FR-107-003, FR-107-004, FR-107-005, FR-107-023, FR-107-029, NFR-107-006
- SCN-107-003

```gherkin
Scenario: SCN-107-003 Nudge card carries provenance and one-tap actions
  Given a controller-permitted card produced by a real intelligence producer
  When the card renders on the web cockpit
  Then it shows a why-am-I-seeing-this provenance line derived from its producer and cause
  And it offers one-tap act, snooze, and dismiss
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Provenance + one-tap actions | Disposable stack; one controller-permitted card from a real producer; valid scoped session | Render the card on the web surface; read the provenance line; act/snooze/dismiss | A producer-derived "why am I seeing this" line and separately focusable one-tap act/snooze/dismiss; action rides the same-origin authenticated mutation and flips the card to its terminal form with one `status` announcement | e2e-ui |
| Same-origin transport | Valid HttpOnly cookie session | Trigger act from JS; inspect the request | `credentials: "same-origin"`, `auth.RequireScope`-gated, strict CORS/CSP/Origin; body carries only `{nudgeRef, action}`; no bearer in JS; no `content_key` client-side | integration |
| Shell canary | Existing authenticated assistant/shell session | Navigate the existing shell after the card renders | Existing spec-106 shell navigation and the assistant surface stay green; the card adds no nav destination | e2e-ui |

## Implementation Plan

1. Render the `ProactiveCardModel` (from SCOPE-01) as a spec-106 Pending-action-row: title, the producer-derived provenance line (spec-106 Evidence/provenance row; missing evidence is textually explicit), an availability badge (text+shape, never color alone), one-tap `[Act][Snooze][Dismiss]` as separately focusable named actions (the whole card is not one hidden button), and `[Why ▾]`.
2. Compose the web action over spec-106's `AuthenticatedRequestAdapter` + `MutationFeedbackPresenter` (pending → success/`already-handled`/error): a same-origin mutation riding the existing HttpOnly `auth_token` cookie (`internal/api/pwa.go`) with `credentials: "same-origin"`, gated by `auth.RequireScope`, protected by the product's strict same-origin CORS default (`internal/api/router.go`), CSP `script-src 'self'`, and Origin/Referer discipline.
3. Send only `{nudgeRef, action}` (`action ∈ act|snooze|dismiss`); the server resolves the `NudgeRef` → `(content_key, principal, action)` and calls the one `NudgeAck` path (SCOPE-01). The `content_key` is never sent by the client; no bearer token is placed in or read from JavaScript; no token or card content enters durable client storage.
4. Map the mutation outcome onto spec-106 tokens through `HonestStatePresenter`: `acted`/`snoozed` terminal forms in place, `already-handled` when a stale ref is tapped, `error` for a failed mutation — never a fabricated success and never a normal card for a failure.
5. Expose the stable spec-106 `data-*` DOM contract on the card region with closed, content-free token values (no node label, `content_key`, query, or provenance text in a test hook).
6. Update API/architecture/testing documentation through the docs owner during implementation; add no nav destination and do not modify `specs/106-*` (coordination note only).

## Shared Infrastructure Impact Sweep

- **Protected contracts:** the spec-106 shell/session, `AuthenticatedRequestAdapter`, `MutationFeedbackPresenter`, `data-*` DOM contract, and the product's HttpOnly cookie + strict CORS/CSP/Origin discipline; the SCOPE-01 `NudgeRef`/`NudgeAck` path.
- **Independent canaries:** existing authenticated shell navigation and the assistant surface stay green; existing state-mutating requests keep their Origin/Referer discipline; the service worker never caches `/api/*`.
- **Rollback:** the card and its action endpoint are additive; disabling them restores the shell with no proactive body; no session, CORS, or CSP contract is mutated.

## Change Boundary

**Allowed during execution:** the web proactive-card component, the thin
`{nudgeRef, action}` composition endpoint, card-region `data-*` hooks, and
tests/docs named by this scope.  
**Excluded:** editing `specs/106-*` or the shell/session/CORS/CSP owners;
introducing a second budget/dedupe/suppression path; the Telegram/WhatsApp
renderings (SCOPE-03); the cockpit composition (SCOPE-04); any client cache of
card/provenance data.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-003-U | Unit | `unit` | SCN-107-003 | `web/pwa/tests/proactive_card_model_test.ts` - `SCN-107-003 provenance line and three named actions render` | `./smackerel.sh test unit` | No |
| T107-003-I | Integration | `integration` | SCN-107-003 | `tests/integration/proactive/web_action_transport_test.go` - `SCN-107-003 same-origin authenticated {nudgeRef,action} mutation` | `./smackerel.sh test integration` | Yes |
| T107-003-A | E2E API regression | `e2e-api` | SCN-107-003 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-003 web nudge action API acknowledges through controller` | `./smackerel.sh test e2e` | Yes |
| T107-003-W | E2E UI regression | `e2e-ui` | SCN-107-003 | `web/pwa/tests/proactive-card.spec.ts` - `SCN-107-003 card shows provenance and one-tap act/snooze/dismiss` | `./smackerel.sh test e2e-ui` | Yes |
| T107-02-TRANSPORT | Integration | `integration` | SCN-107-003 | `tests/integration/proactive/web_action_transport_test.go` - `web action carries no bearer in JS and no content_key client-side` | `./smackerel.sh test integration` | Yes |
| T107-02-CANARY | Shared-shell canary | `e2e-ui` | SCN-107-003 | `web/pwa/tests/assistant_intents_dashboard.spec.ts` - `proactive card preserves authenticated shell and assistant navigation` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-003 Nudge card carries provenance and one-tap actions: the web card renders a producer-derived "why am I seeing this" provenance line and offers separately focusable one-tap act, snooze, and dismiss.
- [ ] The web action is an authenticated same-origin `{nudgeRef, action}` mutation composed over spec-106; it carries no bearer in JS, no `content_key` client-side, and introduces no second budget or surfacing path.
- [ ] Card outcomes map onto spec-106 honest-state tokens (`acted`/`snoozed`/`already-handled`/`error`) and never fabricate success or render a failure as a normal card.
- [ ] The card exposes the stable spec-106 `data-*` contract with closed, content-free token values and adds no navigation destination; `specs/106-*` is not modified.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-003-U passes with current-session evidence in `report.md#t107-003-u`.
- [ ] T107-003-I passes against the disposable stack with current-session evidence in `report.md#t107-003-i`.
- [ ] T107-003-A passes through production HTTP routes with current-session evidence in `report.md#t107-003-a`.
- [ ] T107-003-W passes without interception and proves provenance + one-tap actions in `report.md#t107-003-w`.
- [ ] T107-02-TRANSPORT proves no bearer in JS and no client-side content_key in `report.md#t107-02-transport`.
- [ ] T107-02-CANARY independently proves the shell/assistant navigation stays green in `report.md#t107-02-canary`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation, API documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. `specs/106-*` is a compose-over
dependency and is not modified.
