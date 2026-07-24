# Slice 04: Optimization, History, And Reports

**Status:** Not Started
**Depends On:** 01, 02
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 5, 11 (plus kernel rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An owner generates, manually edits/reorders, compares, accepts, and restores immutable optimization versions, and inspects current/historical/exclusive report states bound to a version hash — with starred/manual historical choices never silently overwritten.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

Accepted only when the same request path proves in-band: claim-bound authz with a cross-user denial; Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated optimization/report mutation (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); closed typed errors with prior-state preservation; immutable audit per outcome; PostgreSQL read-back with refused/failed no-mutation proof; keyboard/screen-reader semantics; 320px/200%-zoom reflow and non-color-only state; content-free validate-plane traces/metrics.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-05 Historical optimization remains editable and versioned
	Given retained optimization periods with manual overrides and starred choices
	When the owner edits/reorders, regenerates, compares, accepts, and restores a prior version
	Then immutable versions and a current pointer are preserved, regeneration cannot overwrite starred/manual historical choices, an invalid period is refused, and each version reads back from PostgreSQL

Scenario: SCN-083-002-11 Report distinguishes no-data stale and failure
	Given a report bound to an immutable optimization version
	When cards are absent, no category matches, a version is stale, or a provider fails
	Then current/historical/no-cards/no-match/stale/degraded/unavailable/failed states remain exclusive and a stale result can never be presented as current
```

## Implementation Plan

1. Consume owner/version/receipt contract; bind cookie-authenticated optimization/report mutations to `MutationTrustGuard`.
2. Implement immutable `OptimizationVersion` entries, current pointer, copy-on-write manual edit/reorder/override, compare, accept, restore; regeneration appends a version and never mutates a starred historical choice.
3. Bind reports to a version hash with exclusive current/historical/no-cards/no-match/stale/degraded/unavailable/failed status; an export-safe view exposes recommendation, alternatives, rate/source/reason/limits.
4. Audit every generate/edit/accept/restore/report outcome; refuse invalid periods with a typed error and no mutation.
5. Augment `/cards/recommendations`, `/cards/rotating`, `/cards/report` with in-flow typed states and keyboard/screen-reader parity.

## Migration And Rollback

Consume Migration C (immutable optimization versions). Revision-one backfill and current pointers verify; rollback beyond the first multi-version write uses a database snapshot plus prior binary, never lossy flattening.

## Consumer Impact Sweep

Trace optimization/report store/service/API/web routes, version pointer/compare adapters, Optimize/report templates, deep links, navigation, audit, export-safe report view, docs, and existing recommendations/dashboard Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` optimization/version/report models/store/service/API/web/tests, additive Migration C. Excluded: source operations, portability, shared auth internals beyond consuming `MutationTrustGuard`, CCManager, spec 079/106.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Editable history | Retained versions with manual overrides | Edit, reorder, regenerate, compare, accept, restore | Immutable versions preserved; starred choice not overwritten | `e2e-ui` |
| Exclusive report states | Version with varying data/provider state | View report under no-data/stale/failure | Distinct exclusive states; stale never shown as current | `e2e-ui` |
| Forged-CSRF adversary | Forged Origin/token on optimization mutation | Submit mutation | `MutationTrustGuard` typed refusal; no state change | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD04-TP01 | Optimization unit | `unit` | `internal/cardrewards/service_optimize_test.go` | SCN-083-002-05 | Copy-on-write version chain, starred-choice protection, invalid-period refusal, report status derivation | `./smackerel.sh test unit` | No |
| CARD04-TP02 | Version integration | `integration` | `internal/cardrewards/store_test.go` | SCN-083-002-05 | Append-only version chain + current pointer + restore on ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| CARD04-TP03 | Editable history E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_recommendations.spec.ts` | SCN-083-002-05 | `SCN-083-002-05 historical optimization remains editable and versioned` live, no interception | `./smackerel.sh test e2e-ui` | Yes |
| CARD04-TP04 | Report states E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_dashboard.spec.ts` | SCN-083-002-11 | `SCN-083-002-11 report distinguishes no-data stale and failure` exclusive states, live | `./smackerel.sh test e2e-ui` | Yes |
| CARD04-TP05 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_recommendations.spec.ts` | SCN-083-002-05 | `TestRegressionRegenerationCannotOverwriteStarredAndForgedCSRFFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD04-TP06 | Report freshness integration | `integration` | `internal/cardrewards/store_test.go` | SCN-083-002-11 | Stale version cannot be presented as current; report bound to version hash | `./smackerel.sh test integration` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-05 immutable versions/current pointer/copy-on-write edit/compare/accept/restore; starred/manual historical choice never overwritten; invalid period refused.
- [ ] SCN-083-002-11 exclusive current/historical/no-cards/no-match/stale/degraded/unavailable/failed report states bound to a version hash; stale never shown as current.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF typed states, cross-user denial, immutable audit, PostgreSQL read-back, keyboard/screen-reader + 320px parity.
- [ ] Migration C, rollback, consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 6 Rows / 6 Items

- [ ] CARD04-TP01 optimization-unit evidence is recorded.
- [ ] CARD04-TP02 version-chain PostgreSQL integration evidence is recorded.
- [ ] CARD04-TP03 editable-history live E2E UI evidence is recorded.
- [ ] CARD04-TP04 exclusive-report-states live E2E UI evidence is recorded.
- [ ] CARD04-TP05 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD04-TP06 report-freshness integration evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration C compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
