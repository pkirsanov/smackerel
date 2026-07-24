# Slice 02: Benefits And Pending Lifecycle

**Status:** Not Started
**Depends On:** 01
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 2, 3, 10 (plus kernel rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An owner creates one multi-category offer bound to one typed shared limit pool, manages tiered/non-tiered selection sets across their lifecycle, and sees pending selection/re-enrollment causes appear, resolve, dismiss, and prove non-recurrence — with the optimizer counting the shared cap exactly once and PostgreSQL the sole authority.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

Accepted only when the same request path proves in-band: claim-bound authz with a cross-user denial; same-origin Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated offer/selection/pending mutation (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); closed typed read/mutation errors with prior-state preservation; immutable audit for each outcome; PostgreSQL read-back with refused/failed no-mutation proof; representative keyboard/screen-reader semantics; representative 320px/200%-zoom reflow and non-color-only state; content-free validate-plane traces/metrics.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-02 Multi-category shared-limit offer is optimized once
	Given an owner defines one offer targeting multiple categories with a single typed shared cap
	When the optimizer evaluates the offer and one category is later removed
	Then the shared cap is counted exactly once, the pool survives single-category removal, and no duplicate business meaning is created

Scenario: SCN-083-002-03 Selection lifecycle reaches pending and resolution
	Given tiered and non-tiered selection sets with enrollment and lock windows
	When windows expire and the owner re-enrolls, dismisses, or resolves
	Then a locked expired selection cannot remain active, deleting one tier cannot erase another, and each lifecycle state reads back from PostgreSQL

Scenario: SCN-083-002-10 Pending selections are actionable not guilt-inducing
	Given a persisted pending cause with reason and due period
	When the owner resolves or dismisses it and later refreshes
	Then the resolved cause cannot reappear without a genuinely new period/version and no global guilt counter is shown
```

## Implementation Plan

1. Consume owner/version/receipt contract and bind every cookie-authenticated offer/selection/pending mutation to `MutationTrustGuard` before store access.
2. Model one offer parent + category children + typed `LimitPool`; optimizer evaluates the pool cap once; single-category removal preserves the pool.
3. Implement `SelectionSet` aggregate with tier entries and closed lifecycle (`draft|enrolled|locked|expired|pending_reenrollment|resolved|dismissed|deleted`); resolve creates a new enrolled revision; dismiss closes only the current cause.
4. Persist `card_pending_actions` keyed by owner+kind+entity+due-period+source/entity version; resolve/dismiss prevents the same cause reappearing; a new period/version creates a new key; no unread counter.
5. Surface Benefits (`/cards/offers`, `/cards/selections`, `/cards/categories`) and Today pending inbox with in-flow typed states, keyboard/screen-reader parity, and audit for every outcome.

## Migration And Rollback

Consume Migration B (offers, selections, lifecycle, pending). Ambiguous legacy shared groups refuse merge; category/tier counts preserve identity; application rollback before the first irreducible multi-category write uses the captured snapshot, never lossy flattening.

## Consumer Impact Sweep

Trace offer/selection/pending store/service/API/web routes, optimizer pool accounting, Today/Benefits templates, deep links, navigation, audit, import/export records, docs, and existing offers/selections/dashboard Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` offer/selection/pending models/store/service/API/web/tests, additive Migration B, optimizer pool accounting. Excluded: bonus/calendar, optimization versions, source operations, shared auth internals beyond consuming `MutationTrustGuard`, CCManager, spec 079/106.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Multi-category shared cap | Offer over two categories, one pool | Optimize, remove one category | Cap counted once; pool survives removal | `e2e-ui` |
| Selection lifecycle | Tiered + non-tiered sets, windows | Enroll, expire, re-enroll, delete one tier | Locked-expired cannot stay active; other tier intact | `e2e-ui` |
| Pending resolve/dismiss | Persisted cause | Resolve/dismiss, refresh | Cause does not reappear without new key; no guilt counter | `e2e-ui` |
| Forged-CSRF adversary | Forged Origin/token on offer/selection mutation | Submit mutation | `MutationTrustGuard` typed refusal; no state change | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD02-TP01 | Offer/selection unit | `unit` | `internal/cardrewards/service_benefits_test.go` | SCN-083-002-02 | Shared-pool single-count accounting, selection transitions, pending cause-key derivation | `./smackerel.sh test unit` | No |
| CARD02-TP02 | Shared-cap integration | `integration` | `internal/cardrewards/store_test.go` | SCN-083-002-02 | Multi-category offer + single typed cap counted once; single-category removal preserves pool on ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| CARD02-TP03 | Selection lifecycle E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_offers_selections.spec.ts` | SCN-083-002-03 | `SCN-083-002-03 selection lifecycle reaches pending and resolution` live, no interception | `./smackerel.sh test e2e-ui` | Yes |
| CARD02-TP04 | Pending inbox E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_dashboard.spec.ts` | SCN-083-002-10 | `SCN-083-002-10 pending selection lifecycle` actionable, no guilt counter, live | `./smackerel.sh test e2e-ui` | Yes |
| CARD02-TP05 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_offers_selections.spec.ts` | SCN-083-002-02 | `TestRegressionSharedCapNoDoubleCountAndForgedCSRFFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD02-TP06 | Pending non-recurrence integration | `integration` | `internal/cardrewards/store_test.go` | SCN-083-002-10 | Resolved/dismissed cause cannot reappear without a new period/version key | `./smackerel.sh test integration` | Yes |
| CARD02-TP07 | Benefits live Playwright | `e2e-ui` | `web/pwa/tests/cardrewards_offers_selections.spec.ts` | SCN-083-002-03 | Benefits keyboard/screen-reader + 320px reflow kernel, no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-02 one offer spans multiple categories with one typed shared cap counted exactly once; single-category removal preserves the pool.
- [ ] SCN-083-002-03 complete canonical/tiered/non-tiered selection lifecycle; locked-expired cannot remain active; deleting one tier cannot erase another.
- [ ] SCN-083-002-10 first-class pending cause lifecycle with resolve/dismiss/non-recurrence and no guilt counter.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF typed states, cross-user denial, immutable audit, PostgreSQL read-back, keyboard/screen-reader + 320px parity.
- [ ] Migration B, rollback, consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 7 Rows / 7 Items

- [ ] CARD02-TP01 offer/selection unit evidence is recorded.
- [ ] CARD02-TP02 shared-cap single-count PostgreSQL integration evidence is recorded.
- [ ] CARD02-TP03 selection-lifecycle live E2E UI evidence is recorded.
- [ ] CARD02-TP04 pending-inbox live E2E UI evidence is recorded.
- [ ] CARD02-TP05 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD02-TP06 pending non-recurrence integration evidence is recorded.
- [ ] CARD02-TP07 live no-interception Benefits Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration B compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
