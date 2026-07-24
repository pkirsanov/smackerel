# SCOPE-106-15: Readiness And Acceptance Projection Integration

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Depends On:** SCOPE-106-14
**External Entry Gates:** After SCOPE-106-14 produces current real journey evidence, BUG-032-004 SCOPE-04 and BUG-102-001 SCOPE-04 must complete their owner projections. Neither packet may use a route, flag, health probe, or spec status as substitute evidence.

## Outcome

Settings, capability availability, client availability, and Admin Acceptance compose owner-produced readiness and product-journey projections under the shared shell. Spec 106 does not derive readiness, ingest evidence, run acceptance, deploy, or mutate a release pointer.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-004 Optional capability is represented honestly
  Given the readiness owner classifies an optional capability needs-setup degraded intentionally unavailable or available from current evidence
  When Settings and navigation render
  Then the exact four-label projection meaning age limitation and permitted action appear
  And no secret or operator-only evidence is exposed

Scenario: SCN-106-005 Enabled capability with no working provider is not ready
  Given an enabled capability lacks a usable provider route dependency journey or deployment fact
  When owner readiness resolves
  Then the product presents Unavailable or Needs setup according to requiredness
  And no action or claim promotes it from enablement alone

Scenario: SCN-106-013 Native client availability follows artifact truth
  Given no immutable Android artifact is pinned for the current release
  When client availability renders
  Then Android is Unavailable with no download or setup promise
  And signing or target details remain private
```

## UI Scenario Matrix

| UX ID | Exact Planned Test Title | File |
|---|---|---|
| UX-E2E-106-049 | `UX-E2E-106-049 Settings uses exactly Available Needs setup Degraded and Unavailable from owner readiness` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` |
| UX-E2E-106-051 | `UX-E2E-106-051 operator readiness dimensions and contradictions demote claims without optimistic ready copy` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` |
| UX-E2E-106-070 | `UX-E2E-106-070 missing immutable Android artifact produces Unavailable and no download action` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` |
| UX-E2E-106-071 | `UX-E2E-106-071 Admin Acceptance distinguishes accepted degraded failed blocked timeout mismatch and missing identity states` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` |

## Implementation Plan

1. Consume BUG-032 daily-user/operator readiness projections and BUG-102 immutable acceptance result projection through their owner APIs/contracts only after the real-journey entry gates close.
2. Render Settings appearance/account/capability sections and authorized Admin/Acceptance with shared tables/records, state bands, evidence age, limitation, action, release/schema/time/counts, and safe detail.
3. Keep daily-user output bounded to discoverable capability label, age, limitation, and permitted action; operator evidence remains authorization-gated and content-free.
4. Keep Admin/Acceptance read-only: no run, retry, identity input, policy edit, deploy, keep, rollback, secret, or target control appears.
5. Render native-client availability from immutable artifact owner truth; absent artifact means Unavailable and no action.
6. Preserve exact owner failure/contract states. Resolver/store failure creates no optimistic link; health-only success cannot promote acceptance or readiness.
7. Run BUG-032/BUG-102 owner regressions independently before spec-106 projection tests.

## Consumer Impact Sweep

Trace Settings/Status, server/PWA navigation, capability badges, Admin tools, acceptance projection, client availability, docs/release claim consumers, APIs, deep links, stable hooks, tests, and acceptance/readiness mappings. Presentation may redact but never recalculate.

## Change Boundary

**Allowed:** Settings/Admin/Acceptance/client-availability shared composition, responsive/a11y projection, focused cross-surface tests.

**Excluded:** readiness catalog/resolver/ledger/ingestion/snapshots, BUG-102 runner/manifest/verdict, native artifact production, docs publication, deployment/knb, foreign packets, spec 079, CCManager, and target details.

## Test Plan

| ID | Category | File/Location | Scenario | Exact Test Title | Command | Live |
|---|---|---|---|---|---|---|
| XP106-15-U | `unit` | `internal/web/coherent_readiness_acceptance_test.go` | SCN-106-004, 005, 013 | `TestReadinessAcceptanceAdaptersConsumeOwnerProjectionsWithoutRecalculationOrControls` | `./smackerel.sh test unit --go` | No |
| XP106-15-I | `integration` | `tests/integration/experience/readiness_acceptance_composition_test.go` | SCN-106-004, 005, 013 | `TestSettingsAdminAcceptanceAndClientAvailabilityShareOwnerTruthAndAuthorization` | `./smackerel.sh test integration` | Yes |
| XP106-15-A | `e2e-api` | `tests/e2e/readiness_acceptance_composition_e2e_test.go` | SCN-106-004, 005, 013 | `Readiness acceptance and client artifact projections preserve owner schema freshness and failure states` | `./smackerel.sh test e2e` | Yes |
| XP106-15-O | `functional` | BUG-032/BUG-102 evidence register | SCN-106-004, 005, 013 | `TestReadinessAcceptanceCompositionRequiresPostJourneyOwnerEvidence` | `./smackerel.sh check` | No |
| UX-E2E-106-049 | `e2e-ui` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` | SCN-106-004, 005 | `UX-E2E-106-049 Settings uses exactly Available Needs setup Degraded and Unavailable from owner readiness` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-051 | `e2e-ui` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` | SCN-106-005 | `UX-E2E-106-051 operator readiness dimensions and contradictions demote claims without optimistic ready copy` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-070 | `e2e-ui` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` | SCN-106-013 | `UX-E2E-106-070 missing immutable Android artifact produces Unavailable and no download action` | `./smackerel.sh test e2e-ui` | Yes |
| UX-E2E-106-071 | `e2e-ui` | `web/pwa/tests/coherent_readiness_acceptance.spec.ts` | SCN-106-014 | `UX-E2E-106-071 Admin Acceptance distinguishes accepted degraded failed blocked timeout mismatch and missing identity states` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-106-004, SCN-106-005, and SCN-106-013 render exact owner readiness, acceptance, and artifact truth without re-derivation, optimistic fallback, or secret/target exposure.
- [ ] Settings and Admin/Acceptance remain responsive, semantic, authorized, and read-only with exact evidence age/limitation/action states.
- [ ] BUG-032/BUG-102 evidence is produced only after SCOPE-106-14 real journeys and remains independently certified.

#### Test Evidence - 8 Rows / 8 Items

- [ ] XP106-15-U passes with evidence in `report.md#xp106-15-u`.
- [ ] XP106-15-I passes with evidence in `report.md#xp106-15-i`.
- [ ] XP106-15-A passes with evidence in `report.md#xp106-15-a`.
- [ ] XP106-15-O passes with evidence in `report.md#xp106-15-o`.
- [ ] UX-E2E-106-049 passes with evidence in `report.md#ux-e2e-106-049`.
- [ ] UX-E2E-106-051 passes with evidence in `report.md#ux-e2e-106-051`.
- [ ] UX-E2E-106-070 passes with evidence in `report.md#ux-e2e-106-070`.
- [ ] UX-E2E-106-071 passes with evidence in `report.md#ux-e2e-106-071`.

#### Build Quality Gate

- [ ] BUG-032/BUG-102 owner suites, post-journey evidence gate, auth/redaction/privacy, read-only controls, responsive/a11y/theme, consumer trace, no-interception, check, lint, format, artifact lint, traceability, and directly affected integration documentation checks pass with zero warnings.
