# Scope 08: Rollout And Cross-Surface Regression

**Status:** Not Started
**Depends On:** 07
**Scope-Kind:** runtime-behavior

## Outcome

The complete provider-backed readiness change survives staged migration, rollback rehearsal, provider-compatible live execution, health churn, concurrency, privacy/security, accessibility, and broader recommendation/product regression before any readiness claim changes.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-14 Rollout preserves recommendation truth across every surface
	Given representative legacy rows and staged required optional healthy partial all-unhealthy fixture-only no-match filtered-empty typed-failure auth watch and accessibility states
	When migration backfill constraints rollback re-upgrade provider-compatible API browser scheduler persistence telemetry and stress workflows execute
	Then every surface reports the same authoritative state and evidence while core optional-capability isolation and historical evidence remain intact
	And no refused mutation fixture result false-ready claim secret stale outcome interception bailout or out-of-boundary change appears
```

## Implementation Plan

1. Execute migration A-C sequencing on representative legacy rows, including ambiguous historical empties, and verify backup/restore and application-first rollback checkpoints.
2. Run production adapters through provider-compatible external boundary services for healthy, no-match, auth, quota, timeout, malformed, generic failure, and mixed outcomes.
3. Exercise concurrent availability checks, provider health churn, request bursts, watch create/refresh races, and scheduler/manual overlap against the ephemeral validate stack.
4. Run complete API and no-interception Playwright scenario matrices plus existing Why, feedback, preferences, constraints, consent, Telegram, attribution, confidence, and product-shell regressions.
5. Verify bounded metrics/logs/traces and alert conditions without secret, query, location, user, watch, or request labels.
6. Run consumer stale-reference scans, implementation reality checks, artifact lint, traceability, source locking, security/privacy, accessibility, and managed documentation checks.

## Migration And Rollback Checkpoint

- Upgrade: nullable columns, conservative backfill, compatible readers, new writers, verification, then constraints.
- Application rollback: previous reader/writer ignores additive columns; no evidence columns or provider facts are dropped.
- Post-semantic-write rollback: preserve typed evidence and restore application only to a version proven to ignore unknown columns without rewriting outcomes.
- Required-mode activation: occurs only after a production adapter passes target-compatible health; optional mode remains unavailable until then.

## Consumer Impact Sweep

Recheck every request, watch, scheduler, provider API, status, navigation, deep link, metric, alert, doc claim, config key, and test reference. Zero active consumers may infer readiness from enablement, route mounting, raw registry cardinality, fixture ID prefixes, or an empty result list.

## Change Boundary

Allowed: recommendation-specific migration/rollout validation, live scenario and stress tests, managed recommendation docs/claims, direct consumer cleanup proven by the sweep. Excluded: deployment overlay values, release-train files, unrelated runtime remediation, Card Rewards, spec 079, and other bug packets.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC08-TP01 | Migration/rollback integration | `integration` | `tests/integration/recommendations_migration_test.go` | SCN-039-005-14 | Full staged upgrade, backfill, constraints, application rollback, evidence retention, and re-upgrade against ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| REC08-TP02 | Provider matrix E2E API | `e2e-api` | `tests/e2e/recommendations_providers_test.go`, `tests/e2e/recommendations_api_test.go` | SCN-039-005-14 | Real production adapters and protocol-compatible external services cover all ready/degraded/unavailable/outcome classes | `./smackerel.sh test e2e` | Yes |
| REC08-TP03 | Complete Playwright matrix | `e2e-ui` | `web/pwa/tests/recommendations_readiness.spec.ts`, `web/pwa/tests/recommendation_watches_readiness.spec.ts` | SCN-039-005-14 | All stable request/watch/status/auth/accessibility journeys run on live stack with no interception | `./smackerel.sh test e2e-ui` | Yes |
| REC08-TP04 | Broader recommendation regression | `e2e-api` | `tests/e2e/recommendations_full_regression_test.go` and existing recommendation e2e files | SCN-039-005-14 | Why, feedback, preferences, policy, constraints, consent, Telegram, attribution, confidence, and dossier contracts remain intact | `./smackerel.sh test e2e` | Yes |
| REC08-TP05 | Readiness concurrency stress | `stress` | `tests/stress/recommendations_test.go` | SCN-039-005-14 | Provider health churn, request bursts, watch races, and scheduler overlap preserve truthful bounded state and zero inert rows | `./smackerel.sh test stress` | Yes |
| REC08-TP06 | Security/privacy integration | `integration` | `tests/integration/recommendation_privacy_test.go`, `tests/integration/recommendation_metrics_test.go` | SCN-039-005-14 | Authz, CSRF, redaction, bounded labels, safe logs, and secret/query/location absence | `./smackerel.sh test integration` | Yes |
| REC08-TP07 | Consumer/reality regression | `functional` | recommendation source, config, docs, and test surfaces | SCN-039-005-14 | Stale-reference, fixture-production, default/fallback, reality, no-interception, artifact-lint, and traceability guards report zero findings | `./smackerel.sh lint` | No |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-14 Rollout preserves recommendation truth across every surface: migrations, adapters, runtime consumers, telemetry, stress, accessibility, and rollback remain coherent and value-safe.
- [ ] All nine scenario contracts are individually traceable through provider, availability, request, watch, scheduler, API/UI, persistence, and telemetry evidence.
- [ ] Both production adapters pass real protocol-compatible validation; fixture providers never enter production readiness or results.
- [ ] Migration/backfill/constraints/rollback and required-mode activation are rehearsed without evidence loss or optional-capability product outage.
- [ ] Every consumer uses one readiness contract and all explicit unavailable/degraded/no-match/filtered/auth/quota/timeout/error states remain distinct.
- [ ] Consumer and change-boundary sweeps report zero stale references or edits outside the packet boundary.

#### Test Evidence - 7 Rows / 7 Items

- [ ] REC08-TP01 migration/rollback integration evidence is recorded.
- [ ] REC08-TP02 provider-matrix E2E API evidence is recorded.
- [ ] REC08-TP03 complete live no-interception Playwright evidence is recorded.
- [ ] REC08-TP04 broader recommendation E2E regression evidence is recorded.
- [ ] REC08-TP05 readiness-concurrency stress evidence is recorded.
- [ ] REC08-TP06 security/privacy/telemetry integration evidence is recorded.
- [ ] REC08-TP07 consumer/reality/governance evidence is recorded.

#### Build Quality Gate

- [ ] Full repository-standard build/check/test/lint/format gates applicable to this packet, source locking, zero warnings, migration safety, bundle freshness, artifact lint, traceability, managed docs, and validate-owned certification all pass with current-session evidence.