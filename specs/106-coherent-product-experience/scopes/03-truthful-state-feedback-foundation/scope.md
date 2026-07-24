# SCOPE-106-03: Truthful State And Feedback Foundation

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Tags:** foundation:true
**Depends On:** SCOPE-106-01, SCOPE-106-02

## Outcome

One renderer-neutral presentation foundation maps owner-defined availability, read, auth/access, privacy-clear, and mutation outcomes to truthful components without querying domain stores, parsing raw error strings, or converting failure into empty or success.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-004 Optional capability is represented honestly
  Given an owner reports optional configuration absent or an explicit unsupported policy
  When the shared presenter renders the capability
  Then it shows Needs setup or Unavailable with only the permitted action
  And it does not report a product outage or ready daily journey

Scenario: SCN-106-005 Enabled capability with no working provider is not ready
  Given a feature switch or route exists but its owner reports no usable provider route or dependency
  When availability and content state are projected
  Then availability is Unavailable and content cannot fabricate results or normal readiness

Scenario: SCN-106-010 Mutation reports authoritative outcome
  Given an authenticated user starts a state-changing command
  When the owner reports pending persisted idempotent conflict refused partial or failed
  Then duplicate submission is prevented and the exact terminal state is visible
  And success appears only after complete persistence and authoritative read-back
```

## Implementation Plan

1. Define `ExperienceStatePresenter` over closed owner outcome adapters for loading, ready, first-use empty, filtered empty, stale, degraded, needs setup, disabled, unauthorized, access denied, not found, and error. Unknown or contradictory combinations map to typed error.
2. Define `MutationFeedbackPresenter` for idle, pending, persisted, idempotent, conflict, refused, partial, and failed. Partial is never announced or styled as complete.
3. Define `AuthenticatedRequestAdapter` presentation only: unified 401 synchronously clears protected DOM, accessible labels, in-memory business state, pending work, and graph pixels before re-auth; 403 retains the valid session and shows access denied.
4. Build shared unframed state bands, field/error associations, status/alert semantics, retry/re-auth/config/return actions, pending control locking, success refresh, and stable focus behavior for server and PWA adapters.
5. Keep capability availability independent from page content and mutation state. Routes, flags, health, empty arrays, HTTP 200, or mounted handlers cannot create availability.
6. Enforce no raw stack/error/secret/personal content in DOM, accessibility tree, logs, metrics, traces, URLs, clipboard, storage, or test artifacts.
7. Provide domain adapter seams only; Search, Digest, Assistant, Graph, Cards, Recommendations, Sources, Activity, and readiness owners retain their typed outcomes and business rules.

## Shared Infrastructure Impact Sweep

Protected surfaces are shared auth/session presentation, request helpers, HTMX lifecycle behavior, PWA state clearing, common status/alert primitives, mutation locking, and test fixture state injection. Independent canaries cover a legacy authorized empty read, a PWA 401 privacy clear, a 403 operator denial, one native form mutation, one HTMX mutation, and one Card PRG mutation before broad adoption.

## Rollback

The state/presentation package and adapters roll back atomically. Rollback never restores failure-as-empty, optimistic success, raw errors, retained protected DOM after 401, or duplicate-submit behavior. If an owner adapter cannot be mapped safely, that surface renders typed Unavailable rather than a guessed state.

## Change Boundary

**Allowed:** shared experience state/presentation types and components, auth/access presentation adapters, common mutation feedback, safe focus/live-region helpers, focused tests.

**Excluded:** auth token issuance or middleware verification, domain state derivation/persistence, provider/route readiness logic, domain API schemas, shell cutover, foreign packets, deployment, spec 079, knb, CCManager, and managed claims.

## Test Plan

| ID | Test Type | Category | File/Location | Scenarios | Exact Test Title / Behavior | Command | Live System |
|---|---|---|---|---|---|---|---|
| XP106-03-U | Unit | `unit` | `internal/experience/state_presenter_test.go` | SCN-106-004, 005, 010 | `TestExperienceStateAvailabilityAndMutationAxesRemainClosedIndependentAndFailClosed` | `./smackerel.sh test unit --go` | No |
| XP106-03-I | Integration | `integration` | `tests/integration/experience/state_presenter_test.go` | SCN-106-004, 005, 010 | `TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess` | `./smackerel.sh test integration` | Yes |
| XP106-03-A | E2E API regression | `e2e-api` | `tests/e2e/experience_state_e2e_test.go` | SCN-106-004, 005, 010 | `Availability content auth and mutation outcomes remain structurally distinct through real routes` | `./smackerel.sh test e2e` | Yes |
| XP106-03-W | E2E UI regression | `e2e-ui` | `web/pwa/tests/coherent_states.spec.ts` | SCN-106-004, 005, 010 | `shared state bands show exact recovery and never collapse failure empty unavailable or success` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-03-P | Security/privacy regression | `integration` | `tests/integration/experience/privacy_clear_test.go` | SCN-106-004, 005, 010 | `TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail` | `./smackerel.sh test integration` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-004 optional state, SCN-106-005 unavailable dependency, and SCN-106-010 mutation feedback remain exact, independent, and owner-derived.
- [ ] Failure cannot become empty or success; route/flag/health cannot become availability; partial cannot become complete.
- [ ] Unified 401 privacy clear, 403 access denial, focus, announcements, pending locks, read-back, and recovery actions behave equivalently across renderers.
- [ ] Shared-state canaries, privacy checks, and rollback protect every high-fan-out consumer.

#### Test Evidence - 5 Rows / 5 Items

- [ ] XP106-03-U passes with current-session evidence in `report.md#xp106-03-u`.
- [ ] XP106-03-I passes against real owner outcomes in `report.md#xp106-03-i`.
- [ ] XP106-03-A passes through real routes in `report.md#xp106-03-a`.
- [ ] XP106-03-W passes without interception in `report.md#xp106-03-w`.
- [ ] XP106-03-P passes privacy-clear and redaction checks in `report.md#xp106-03-p`.

#### Build Quality Gate

- [ ] State exclusivity, privacy, auth/access semantics, no-raw-error, no-sensitive-storage, check, lint, format, artifact lint, traceability, canary, rollback, and directly affected security/testing documentation checks pass with zero warnings.
