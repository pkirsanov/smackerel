# Slice 03: Bonus And Calendar Lifecycle

**Status:** Not Started
**Depends On:** 01
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 4 (plus calendar-delivery aspect of row 12; kernel rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An owner creates/updates/completes/deletes a bonus, crosses a threshold exactly once, and delivers/updates/removes one stable Calendar event through a durable transactional outbox, with partial/retry truth exposed and no phantom re-appearance on rerun.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

Accepted only when the same request path proves in-band: claim-bound authz with a cross-user denial; Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated bonus/calendar mutation (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); closed typed errors with prior-state preservation; immutable audit for persisted/idempotent/conflict/refused/failed/partial/no-op outcomes; PostgreSQL read-back with refused/failed no-mutation proof; keyboard/screen-reader semantics; 320px/200%-zoom reflow and non-color-only state; content-free validate-plane traces/metrics.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-04 Bonus and Calendar lifecycle is idempotent
	Given a bonus with a spend threshold and a bound Calendar event
	When progress crosses the threshold, the bonus is completed and deleted, and the run is repeated
	Then the threshold crossing and completion are idempotent, one stable Calendar UID is created/updated/removed via the outbox, a removed bonus/event does not return on rerun, and every outcome including partial delivery is audited
```

## Implementation Plan

1. Consume owner/version/receipt contract; bind cookie-authenticated bonus/calendar mutations to `MutationTrustGuard`.
2. Implement bonus lifecycle (`active|met|expired|deleted`); progress atomically moves active→met; explicit complete is idempotent; deadline moves active→expired; delete removes visibility and enqueues Calendar removal.
3. Implement transactional Calendar outbox with a stable UID binding; external delivery is never represented as committed until its outbox/operation state says so; retries are idempotent.
4. Expose partial/retry truth as typed states; audit persisted/idempotent/failed/partial/no-op outcomes.
5. Augment `/cards/bonuses` with in-flow typed states, keyboard/screen-reader parity, and stable identity across reload.

## Migration And Rollback

Consume Migration B (Calendar outbox/binding). Stable Calendar UID preserved; application rollback before the first irreducible outbox write uses the captured snapshot; delivered external events reconcile through the outbox rather than lossy deletion.

## Consumer Impact Sweep

Trace bonus store/service/API/web routes, Calendar binding/outbox adapters, Today/bonus templates, deep links, navigation, audit, operations, docs, and existing bonuses Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` bonus/calendar models/store/service/API/web/tests, additive Migration B Calendar outbox. Excluded: offer/selection, optimization versions, source operations, shared auth internals beyond consuming `MutationTrustGuard`, CCManager, spec 079/106.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Bonus lifecycle | Bonus with threshold + bound event | Progress, complete, delete, rerun | Idempotent completion; event removed and does not return | `e2e-ui` |
| Partial delivery truth | Outbox delivery fails once | Retry | Typed partial/retry state; single stable UID; audited | `e2e-ui` |
| Forged-CSRF adversary | Forged Origin/token on bonus mutation | Submit mutation | `MutationTrustGuard` typed refusal; no state change | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD03-TP01 | Bonus unit | `unit` | `internal/cardrewards/service_bonus_test.go` | SCN-083-002-04 | Idempotent threshold crossing, lifecycle transitions, outbox state derivation | `./smackerel.sh test unit` | No |
| CARD03-TP02 | Bonus+Calendar integration | `integration` | `internal/cardrewards/calendar_test.go` | SCN-083-002-04 | Transactional outbox + stable UID create/update/remove and audit on ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| CARD03-TP03 | Bonus E2E happy | `e2e-api` | `web/pwa/tests/cardrewards_bonuses.spec.ts` | SCN-083-002-04 | `TestBonusLifecycleAndCalendarOutboxRoundTrip` through real router/service/store | `./smackerel.sh test e2e` | Yes |
| CARD03-TP04 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_bonuses.spec.ts` | SCN-083-002-04 | `TestRegressionBonusIdempotentAndRemovedEventNoReturnForgedCSRFFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD03-TP05 | Bonuses live Playwright | `e2e-ui` | `web/pwa/tests/cardrewards_bonuses.spec.ts` | SCN-083-002-04 | `SCN-083-002-04 Regression: bonus + calendar lifecycle` with no interception + a11y kernel | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-04 complete bonus lifecycle with idempotent threshold crossing, completion, and deletion.
- [ ] One stable Calendar UID created/updated/removed via a transactional outbox; removed bonus/event does not return on rerun; partial/retry truth is typed and audited.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF typed states, cross-user denial, immutable audit, PostgreSQL read-back, keyboard/screen-reader + 320px parity.
- [ ] Migration B Calendar outbox, rollback, consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 5 Rows / 5 Items

- [ ] CARD03-TP01 bonus-unit evidence is recorded.
- [ ] CARD03-TP02 bonus+Calendar PostgreSQL integration evidence is recorded.
- [ ] CARD03-TP03 bonus E2E happy-path evidence is recorded.
- [ ] CARD03-TP04 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD03-TP05 live no-interception bonuses Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration B compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
