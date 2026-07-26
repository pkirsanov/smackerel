# Report: [BUG-039-005] Enabled With Zero Providers Reports False Ready

Links: [scopes/_index.md](scopes/_index.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with eight dependency-ordered per-scope plans: provider/config/migration foundation, independent Google Places and Yelp production adapters, availability/startup truth, request outcomes, watch/scheduler eligibility, shared API/UI projection, and rollout/regression closure.

The plan contains 49 exact Test Plan rows and 49 matching test-evidence DoD items. It requires provider-compatible protocol validation of real production adapters, fixture exclusion, adversarial red-to-green proof, migration/rollback, no-interception Playwright, stress, privacy/security, consumer tracing, and broader recommendation regression. No source, config, provider, test, runtime, production, commit, push, or deployment mutation is claimed.

## Completion Statement

Planning-owned artifacts are complete for implementation routing only if the final packet-local planning validators pass. Status remains `in_progress`; no implementation, behavior test, validation, audit, runtime readiness, or provider delivery is complete.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** recommendations enabled, production registry empty, Google/Yelp false, UI/routes mounted, and no providers/results/watches.
- **Evidence status:** no config, registry, HTTP, UI, or command output was captured here.

## Decision Record

- Readiness derives from configured, registered, category-compatible, fresh, healthy, non-fixture production providers, never enablement, routes, or registry count.
- Required mode fails startup while optional zero-provider mode keeps the product live and the recommendation capability unavailable.
- Google Places and Yelp are independent concrete overlays on one provider/availability foundation; each requires protocol-compatible live validation.
- Availability and execution outcome remain orthogonal so no-match, filtered-empty, partial coverage, refusal, and typed provider failure cannot collapse.
- Fixture providers are rejected structurally in production and never satisfy readiness or evidence.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declarations

- No before-fix command/browser reproduction was executed in this planning invocation.
- No after-fix behavior or provider validation exists.
- Exact provider health timeout, maximum age, and release-target requiredness values remain explicit operator/config inputs; the plan forbids invented defaults.
- Planned not-yet-authored test paths and titles are execution contracts, not claims that those files currently exist.

## Scenario Contract Evidence

The 2026-07-24T05:46Z independent-review spec revision added spec Gherkin scenario `SCN-039-005-10` (Disabled providers do not dilute readiness). This planning-hardening reconciliation integrated it into the availability denominator (SCOPE-04) and the daily-user/operator projection (SCOPE-07), and renumbered the plan-decomposition scenario "Provider foundation migrates" to `SCN-039-005-15` to free the canonical slot. [scenario-manifest.json](scenario-manifest.json) now carries fifteen contracts: ten spec scenarios (`SCN-039-005-01` through `SCN-039-005-10`) and five plan-decomposition scenarios (`SCN-039-005-11` through `SCN-039-005-15`), each assigned to its owning scope with concrete existing/planned regression targets. Evidence references remain empty until execution. [test-plan.json](test-plan.json) is the 49-row machine-readable handoff.

## Validation Summary

Planning-only artifact lint and traceability outcomes are recorded below after actual execution. No implementation validation or certification is requested.

## Audit Verdict

Not audited. No terminal verdict is claimed.

## Implementation — 2026-07-25 (bubbles.implement)

**Phase:** implement
**Claim Source:** executed (this invocation)

### Implemented

Added a self-contained, provider-backed readiness-DETERMINATION package that replaces the distributed false-ready inference (feature-enablement, route mounting, or `Registry.Len()` cardinality) with an honest denominator and a ready/not-ready gate. Provider health and eligibility are an INJECTED value input (`ProviderState`) — no live probe — so the gate logic is fully unit-testable without connectivity.

New files:

- `internal/recommendation/availability/service.go` — closed `CapabilityState`, `AvailabilityCause`, `Operation`, `ProviderClass`, `HealthStatus` enums; the injected `ProviderState` seam; `ProviderCounts`, `ProviderEvidence`, immutable `AvailabilitySnapshot` (with `MustRefuseStartup()` and `Ready()`); and the pure `Determine(Input) AvailabilitySnapshot` function.
- `internal/recommendation/availability/service_test.go` — real table-driven unit tests (no stack, no probe).

Determination logic:

- **Denominator** = providers that are operator-selected AND enabled AND fully configured AND production-class AND registered AND category-compatible for the evaluated category. **Numerator** = eligible AND `Health == healthy`. `ProviderReady = len(healthyEligible) > 0`.
- Disabled, unconfigured, fixture, unregistered, and category-irrelevant declarations are excluded BEFORE numerator/denominator computation, so they cannot dilute or flip readiness (REC-READY-011 / SCN-039-005-10). Stale and unknown health remain IN the denominator but are excluded from the numerator (freshness at the determination layer).
- One healthy eligible provider is sufficient; a second is not a completion dependency (REC-READY-012). A required capability that is not provider-ready must refuse startup via `MustRefuseStartup()`; an optional one reports `unavailable` in isolation.
- Precise, credential-free causes distinguish `zero_configured_providers`, `configured_adapter_missing`, `zero_category_providers`, `all_providers_unavailable`, `partial_provider_coverage` (degraded), and `provider_coverage_complete` (available). Ordering is deterministic by provider ID; evidence carries no secret, query, or upstream body.

The change boundary is the new `availability` package only. `provider.go`, `internal/api/*`, `internal/recommendation/reactive/*`, `internal/web/*`, and the SST config are intentionally NOT touched (see Deferred).

### Test Evidence — REC04-TP01 (availability unit)

**Command:** `./smackerel.sh test unit --go --go-run 'TestDetermineAvailability|TestReadinessDenominatorExcludes|TestAdversarialEnabledZeroEligible|TestOneHealthyEligibleProviderSuffices|TestOperationRequiresProvider' --verbose`
**Exit Code:** 0

```text
--- PASS: TestDetermineAvailability (0.00s)
    --- PASS: TestDetermineAvailability/enabled_zero_providers_optional_is_unavailable_not_ready (0.00s)
    --- PASS: TestDetermineAvailability/enabled_zero_providers_required_refuses_startup (0.00s)
    --- PASS: TestDetermineAvailability/one_healthy_eligible_provider_is_available_and_ready (0.00s)
    --- PASS: TestDetermineAvailability/all_eligible_providers_unhealthy_is_unavailable (0.00s)
    --- PASS: TestDetermineAvailability/stale_eligible_provider_is_in_denominator_but_unavailable (0.00s)
    --- PASS: TestDetermineAvailability/unknown-health_eligible_provider_is_in_denominator_but_unavailable (0.00s)
    --- PASS: TestDetermineAvailability/one_healthy_one_unhealthy_eligible_is_degraded_but_ready (0.00s)
    --- PASS: TestDetermineAvailability/configured_production_provider_without_adapter_is_unavailable (0.00s)
    --- PASS: TestDetermineAvailability/registered_provider_without_category_support_is_unavailable (0.00s)
    --- PASS: TestDetermineAvailability/only_fixture_provider_is_unavailable_never_ready (0.00s)
    --- PASS: TestDetermineAvailability/only_disabled_provider_is_unavailable_never_ready (0.00s)
    --- PASS: TestDetermineAvailability/provider-independent_operation_is_ready_without_a_provider (0.00s)
    --- PASS: TestDetermineAvailability/one_healthy_eligible_amid_excluded_declarations_stays_available (0.00s)
--- PASS: TestReadinessDenominatorExcludesDisabledUnconfiguredFixtureAndCategoryIrrelevantProviders (0.00s)
--- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/zero_providers (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/only_fixture (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/only_disabled (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/only_unconfigured (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/only_unregistered (0.00s)
    --- PASS: TestAdversarialEnabledZeroEligibleProvidersIsNotReady/only_category_irrelevant (0.00s)
--- PASS: TestOneHealthyEligibleProviderSufficesNoSecondRequired (0.00s)
--- PASS: TestOperationRequiresProvider (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/recommendation/availability    0.008s
UNIT_EXIT_RERUN=0
```

The adversarial subtests are the exact BUG-039-005 false-ready condition: an ENABLED capability with zero ELIGIBLE providers (zero / fixture-only / disabled-only / unconfigured-only / unregistered-only / category-irrelevant-only) never reports ready, and refuses startup when required. A naive `len(providers) > 0` gate would flip the fixture-only, disabled-only, and category-irrelevant cases to ready and fail these assertions.

### Build Quality

**Command:** `./smackerel.sh check`
**Exit Code:** 0

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

**Command:** `./smackerel.sh lint`
**Exit Code:** 0

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
=== Validating JS syntax ===
  OK: web/pwa/app.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

Go static analysis (golangci) reported no findings against the new package; a finding would have printed the `internal/recommendation/availability/service.go` path.

> **2026-07-26 wiring update:** the "intentionally NOT touched" boundary recorded
> in this 2026-07-25 section was accurate for that run. The subsequent wiring of
> the gate into `internal/api/recommendations.go` and the live proof are recorded
> below under **Runtime Wiring & Live Readiness Proof — 2026-07-26**, which
> supersedes the change-boundary statement here.

### Deferred (live / coordination-required)

- **Live provider HEALTH probe / real connectivity** — the gate consumes injected provider state; the live health-observation wiring (probe, freshness-window/max-age comparison against SST, timeout) is NOT implemented. Deferred (Docker/live).
- **Live-stack rows** REC04-TP02 (startup/config integration), REC04-TP03 / TP04 / TP05 (E2E API), REC04-TP06 (telemetry integration), and the live-integration run of REC04-TP07 — all require the ephemeral test stack. Not run; Docker was not brought up.
- **Provider foundation (SCOPE-01)** `provider.Descriptor` / `ProviderClass` and the **Google/Yelp production adapters (SCOPE-02/03)** remain not_started; the availability core deliberately decouples via a value seam so it is unit-verifiable ahead of them.
- **Consumer wiring** of the `AvailabilitySnapshot` into startup, request, watch, scheduler, and status surfaces (SCOPE-04 steps 5–6, SCOPE-05/06/07) requires editing the SST provider-config accessor (`config/smackerel.yaml` / `internal/config`) and consumer code — deferred as coordination-required to avoid concurrent-file collision and an un-unit-verifiable change.

### Guard Evidence

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh <packet>`
**Exit Code:** 0

```text
✅ Detected state.json status: blocked
...
✅ No repo-CLI bypass detected in scopes/07-shared-api-ui-projections/report.md command evidence
✅ No repo-CLI bypass detected in scopes/08-rollout-cross-surface-regression/report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

The state-transition guard is intentionally NOT run to `done`: a blocked packet with unchecked live/integration DoD items is expected to fail that guard, which is correct for this deferred-live slice.

## Runtime Wiring & Live Readiness Proof — 2026-07-26 (bubbles.implement)

**Phase:** implement

This session made **no** source change, ran **no** Docker, and ran **no** live
test. It reconciles the packet artifacts against the readiness-gate WIRING that
had already landed in the working tree (a prior implement run truncated before
updating the artifacts), and it verifies that wiring plus its durable live proof
by **inspecting the committed files this session**. The `MutationTrustGuard`
(AUTH-011, BUG-070-001) remains the owning CSRF/Origin guard; nothing here adds a
parallel guard.

### Wired Change — Availability Gate Now Drives The Live Readiness Verdict

**Claim Source:** executed (source `read_file` + `grep_search` this invocation) + interpreted.

Verified by reading the working tree this session:

- `internal/recommendation/availability/runtime.go` — new **live adapter**:
  `ProviderLister` (satisfied by `*provider.Registry` via `List()`), `Service`,
  `NewService(lister, enabled, validFor, now)`, and `Snapshot(ctx, category, op, required)`
  which maps each live provider into the value-type `ProviderState` and returns
  the pure `Determine(...)` verdict. Fixture class is resolved by the **TYPED**
  `IsFixtureProvider()` marker (never an ID prefix); a `nil`/empty registry yields
  zero providers → honest not-ready (`CauseZeroConfiguredProviders`).
- `internal/api/recommendations.go` (import line 15; `computeAvailabilityView` +
  `ListProviders` lines 387–470) — `computeAvailabilityView(ctx)` calls
  `recavailability.NewService(lister, h.cfg.Enabled, 0, time.Now).Snapshot(ctx, recommendation.CategoryPlace, recavailability.OperationRequest, false)`
  and projects the bounded, credential-free `availabilityView`; `ListProviders`
  embeds that `availability` block (`enabled`, `ready`, `state`, `cause`, `counts`)
  in every provider response. Readiness is now derived by the availability gate
  over the **REAL configured-provider registry**, not feature-enablement, route
  mounting, or `Registry.Len()`.
- `internal/recommendation/provider/fixture_integration.go` (line 96) —
  `func (p *FixtureProvider) IsFixtureProvider() bool { return true }`: the build-
  and type-isolated fixture marker the gate uses to exclude fixtures from the
  production readiness denominator.
- `internal/recommendation/availability/runtime_test.go` — unit coverage of the
  live adapter (`TestServiceSnapshotFromRegistry`, `TestServiceNilListerIsHonestNotReady`).
- `tests/integration/recommendation_availability_readiness_test.go` — the durable,
  re-runnable live proof (3 subtests) described next.

**Contract proven:** zero eligible providers → honest NOT-ready; ≥1 eligible
healthy production provider → ready; a second provider is not required.

### Live Integration Proof — REC04 Readiness Over The Real Registry

**Claim Source:** not-run this invocation. The three subtests were executed on the
disposable stack in the immediately-preceding implement run and torn down clean;
they were **NOT re-executed here** per the no-live-tests / no-Docker constraint.
The **durable, on-demand proof** is the committed test file
`tests/integration/recommendation_availability_readiness_test.go`, verified present
this session (via `read_file`) with exactly the three subtests and assertions
transcribed below.

Durable test file — `TestRecommendationAvailabilityReadiness_LiveStatusReflectsRealProviderState`
(driven through `api.NewRecommendationHandlers(...).ListProviders` on a real
`recprovider.Registry`, no interception), quoted from the committed source this
session:

```text
subtest 1  enabled zero providers is honest not ready
  registry := NewRegistry()            // zero providers
  assert availability.enabled == true
  assert availability.ready   == false // FALSE-READY regression would fail here
  assert availability.state   == "unavailable"
  assert availability.cause   == "zero_configured_providers"
  assert counts.eligible == 0 && counts.healthy_eligible == 0

subtest 2  enabled with only a healthy fixture is still not ready
  registry.Register(NewFixtureProvider("fixture_google_places", ...CategoryPlace))
  assert availability.ready   == false // fixtures MUST NOT dilute readiness
  assert availability.cause   == "zero_configured_providers"
  assert counts.fixtures == 1          // fixture observed but excluded
  assert counts.eligible == 0

subtest 3  enabled with one healthy production provider is ready
  registry.Register(readinessProductionProvider{google_places, CategoryPlace, Healthy})
  assert availability.ready   == true  // one eligible healthy provider suffices
  assert availability.state   == "available"
  assert counts.healthy_eligible == 1
```

**Command (preceding run):** `./smackerel.sh test integration`
**Reported result:** all three subtests PASS — `enabled_zero_providers_is_honest_not_ready`,
`enabled_with_only_a_healthy_fixture_is_still_not_ready`,
`enabled_with_one_healthy_production_provider_is_ready`; `ok tests/integration`;
`INTEGRATION_EXIT=0`.

**Teardown (preceding run):** `./smackerel.sh down` → `DOWN_EXIT=0`; zero
`smackerel` containers remain (disposable stack, ephemeral storage; nothing
persisted).

These three subtests are the exact BUG-039-005 contract on the real registry: an
ENABLED capability with zero eligible providers (empty or fixture-only) is honest
NOT-ready, and one healthy production provider is ready — never false-ready from
enablement or provider cardinality.

### Availability Unit — No Regression

**Claim Source:** durable file verified by inspection this session (`read_file`);
the pass/`ok` line is the reported result of the preceding `./smackerel.sh test unit --go`
run, not re-executed here.

`internal/recommendation/availability/runtime_test.go` (read this session)
unit-covers the live adapter with the same adversarial cases as the gate, on a
`stubLister` (no stack, no probe):

```text
TestServiceSnapshotFromRegistry
  empty registry enabled is not ready (the bug)        -> unavailable / zero_configured_providers / 0,0,0,0
  one healthy production provider is ready              -> available   / provider_coverage_complete / 1,1,1,0
  fixture-only registry is not ready                   -> unavailable / zero_configured_providers / 1,0,0,1
  healthy production plus healthy fixture stays ready   -> available (fixture excluded) / 2,1,1,1
  only-failing production provider is unavailable       -> unavailable / all_providers_unavailable / 1,1,0,0
  only-disabled production provider is excluded         -> unavailable / zero_configured_providers / 1,0,0,0
  healthy production for a different category not ready -> unavailable / zero_category_providers / 1,0,0,0
  one healthy plus one failing is degraded but ready    -> degraded / partial_provider_coverage / 2,2,1,0
TestServiceNilListerIsHonestNotReady
  nil lister enabled                                    -> ready=false / zero_configured_providers
```

**Reported result:** `./smackerel.sh test unit --go` shows `ok internal/recommendation/availability`
(no regression). `./smackerel.sh check` exit 0 and `./smackerel.sh lint` exit 0 were
likewise reported clean for this working tree (the raw check/lint output for the
availability slice is recorded in the 2026-07-25 section above).

### Scope Reconciliation

- SCOPE-04 Core Outcomes `SCN-039-005-01` (≥1 healthy → ready), `SCN-039-005-02`
  (enabled + zero → not-ready), and `SCN-039-005-10` (disabled/fixture excluded;
  one healthy suffices) are checked `[x]` in
  [scopes/04-availability-startup-truth/scope.md](scopes/04-availability-startup-truth/scope.md)
  with evidence links to this section.
- Left `[ ]` (honestly deferred): `SCN-039-005-03` (degraded/unhealthy **live**
  matrix), the non-scenario startup-authority / required-startup-abort /
  telemetry / canary-rollback-consumer core outcomes, and the planned live rows
  `REC04-TP02`, `REC04-TP03`, `REC04-TP04`, `REC04-TP05`, `REC04-TP06`,
  `REC04-TP07` (the landed live proof is the separate
  `tests/integration/recommendation_availability_readiness_test.go`, not those
  planned files). SCOPE-04 stays `in_progress`; the packet stays `blocked`.
