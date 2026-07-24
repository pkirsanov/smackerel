# Slice 05: Sources, Config, And Durable Operations

**Status:** Not Started
**Depends On:** 01
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 6, 8, 12 (plus kernel rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An operator inspects source health/citations/confidence, refreshes/verifies safely, sees every required Card capability declared value-safe and fail-loud, views the schedule, overlaps a manual and scheduled run through durable operation keys/leases, and recovers typed dependency failures without duplicate logical work.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

Accepted only when the same request path proves in-band: claim-bound operator authz with a wrong-role denial; Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated source/config/operation mutation (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); closed typed errors with prior-state preservation and safe partial-failure truth; immutable audit per outcome; PostgreSQL read-back with refused/failed no-mutation proof; keyboard/screen-reader semantics; 320px/200%-zoom reflow and non-color-only state; content-free validate-plane traces/metrics.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-06 Source degradation preserves provenance truth
	Given multiple sources with citations confidence and approved media
	When one source changes shape or times out during refresh/verify
	Then remaining sources cannot fabricate full confidence, provenance/citations are retained, unsafe URLs fail before fetch, and the partial failure renders as a truthful typed state

Scenario: SCN-083-002-08 Missing required config fails loud safely
	Given the Card required-capability list and value-safe availability contract
	When a required runtime value is empty or an unsafe default/fixture is attempted
	Then startup or the exact operation fails loud without emitting secret output and an optional missing dependency disables only its specific operation

Scenario: SCN-083-002-12 Schedule and manual triggers deduplicate
	Given the scheduler and a manual trigger for the same operation
	When a double click or a concurrent scheduled run occurs
	Then a durable operation key/lease deduplicates to one effective run/event, a disabled trigger is visibly unavailable, and retry/recovery is audited
```

## Implementation Plan

1. Consume owner/version/receipt + operation contract; bind cookie-authenticated source/config/operation mutations to `MutationTrustGuard`; scheduled runs use a `system` actor.
2. Expose source inventory/availability, durable refresh, citations/confidence/disagreement, approved-source card-media refresh, operator verify/reject; enforce SSRF-safe URL policy (HTTPS, approved host, bounded redirects, no loopback/private/link-local/metadata).
3. Implement the SST `required_capabilities` list and value-safe availability projection; malformed required dependency refuses startup; optional dependency disables its exact operation; `/api/card-capability` and Sources UI report presence/validity/safe-cause only, never values.
4. Implement PostgreSQL operation key + queue/lease/recovery + `202` status + retry + audit; scheduler and manual paths share one pipeline; concurrency/double-click cannot duplicate a run/event.
5. Augment `/cards/admin` (Sources/config/schedules/manual runs) with in-flow typed states, disabled-trigger visibility, and keyboard/screen-reader parity.

## Migration And Rollback

Consume Migration D (operations, sources, immutable audit). Operation/audit backfill is conservative; leases recover without duplicate logical work; source/media provenance retained; rollback uses the captured snapshot.

## Consumer Impact Sweep

Trace source/config/operation store/service/API/web routes, scheduler bootstrap, source/Calendar adapters, Sources/admin templates, `/api/card-capability`, navigation, audit, docs, metrics/alerts, and existing admin Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` source/config/operation models/store/service/API/web/tests, additive Migration D, Card SST/generator entries. Excluded: optimization versions, portability, shared scheduler/auth internals beyond consuming `MutationTrustGuard`/one pipeline, CCManager, spec 079/106.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Source partial failure | Multiple sources, one times out | Refresh, verify | Truthful partial state; provenance retained; no fabricated confidence | `e2e-ui` |
| Fail-loud config | Required capability value empty | Start/operate | Fail-loud, no secret output; optional missing disables exact operation | `unit` |
| Trigger dedupe | Scheduler + manual for same op | Double click / concurrent run | One effective run/event; disabled trigger unavailable | `e2e-ui` |
| Forged-CSRF adversary | Forged Origin/token on operator mutation | Submit mutation | `MutationTrustGuard` typed refusal; no state change | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD05-TP01 | Config fail-loud unit | `unit` | `internal/cardrewards/config_test.go` | SCN-083-002-08 | Required-capability validation, value-safe projection, unsafe-default/fixture refusal, no secret output | `./smackerel.sh test unit` | No |
| CARD05-TP02 | Source degradation integration | `integration` | `internal/cardrewards/reconcile_integration_test.go` | SCN-083-002-06 | Partial source failure retains provenance, no fabricated confidence, SSRF-unsafe URL refused before fetch | `./smackerel.sh test integration` | Yes |
| CARD05-TP03 | Operation dedupe stress | `stress` | `internal/cardrewards/operation_stress_test.go` | SCN-083-002-12 | Concurrent scheduler + manual double-click deduplicate to one effective run/event via durable key/lease | `./smackerel.sh test stress` | Yes |
| CARD05-TP04 | Sources E2E happy | `e2e-api` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-06 | `TestSourceHealthRefreshVerifyAndCapabilityProjection` through real router/service/store | `./smackerel.sh test e2e` | Yes |
| CARD05-TP05 | Trigger dedupe E2E UI | `e2e-ui` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-12 | `SCN-083-002-12 schedule and manual triggers deduplicate` live, no interception | `./smackerel.sh test e2e-ui` | Yes |
| CARD05-TP06 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-06 | `TestRegressionSourceNoFabricatedConfidenceAndForgedCSRFFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD05-TP07 | Sources live Playwright | `e2e-ui` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-06 | Sources/admin keyboard/screen-reader + 320px reflow kernel, no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-06 source health/refresh/verify with retained provenance/citations, no fabricated confidence, SSRF-safe URLs, truthful partial-failure state.
- [ ] SCN-083-002-08 required capabilities value-safe and fail-loud; unsafe default/fixture refused; optional missing dependency disables only its operation; no secret output.
- [ ] SCN-083-002-12 durable operation key/lease dedupe across scheduler + manual; disabled trigger visibly unavailable; retry/recovery audited.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF typed states, operator-role denial, immutable audit, PostgreSQL read-back, keyboard/screen-reader + 320px parity.
- [ ] Migration D, rollback, consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 7 Rows / 7 Items

- [ ] CARD05-TP01 config fail-loud unit evidence is recorded.
- [ ] CARD05-TP02 source-degradation PostgreSQL integration evidence is recorded.
- [ ] CARD05-TP03 operation-dedupe stress evidence is recorded.
- [ ] CARD05-TP04 sources E2E happy-path evidence is recorded.
- [ ] CARD05-TP05 trigger-dedupe live E2E UI evidence is recorded.
- [ ] CARD05-TP06 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD05-TP07 live no-interception Sources Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration D compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
