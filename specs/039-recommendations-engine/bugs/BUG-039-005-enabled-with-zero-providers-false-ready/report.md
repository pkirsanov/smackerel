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
