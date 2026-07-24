# Scope 01: Provider Contract And Migration Foundation

**Status:** Not Started
**Depends On:** -
**Tags:** foundation:true
**Scope-Kind:** runtime-behavior

## Outcome

Production has a typed provider boundary, explicit fail-loud declarations, fixture-class rejection, additive evidence schema, and a reversible migration checkpoint before any readiness consumer changes.

## Gherkin Scenarios

```gherkin
Scenario: SCN-039-005-04 Fixture provider cannot satisfy production
	Given a registry is constructed for production
	When a provider descriptor declares fixture class
	Then registration is rejected before inventory or readiness evaluation
	And no fixture result or healthy count can be produced

Scenario: SCN-039-005-15 Provider foundation migrates and configures without guessing
	Given recommendation requiredness, health bounds, provider metadata, and historical evidence need migration
	When configuration validation, additive migration, conservative backfill, and rollback checks run
	Then missing or invalid required values fail with value-safe codes and no fallback
	And historical evidence is retained without invented precision or sensitive output
```

## Implementation Plan

1. Extend the existing provider interface with `Descriptor`, `ProviderClass`, category declarations, and runtime-class-aware registry construction; retain build-tag fixture isolation.
2. Add required SST fields for requiredness, probe timeout, and maximum health age through the canonical config generator with explicit validation and value-safe errors.
3. Land additive nullable migration A for provider runtime inventory plus additive request/watch evidence columns needed by later scopes; preserve all existing rows.
4. Add deterministic backfill classification that labels unknown historical outcomes conservatively and refuses ambiguous provider metadata.
5. Add check constraints only after backfill verification; keep the application rollback path able to ignore additive columns.
6. Emit bounded registry/config/migration telemetry without secrets or unbounded labels.

## Shared Infrastructure Impact Sweep

- Protected surfaces: provider interface, runtime registry constructors, config generation, migration runner, and shared recommendation test wiring.
- Downstream contracts: reactive engine provider iteration, API provider projection, e2e registry build tags, provider health tests, status templates, and recommendation metrics.
- Canary: existing e2e fixture registry still constructs only in the e2e runtime class while production rejects the same descriptor.
- Restore: revert application readers first while additive columns remain; no evidence column or historical provider row is dropped.

## Consumer Impact Sweep

Search and classify every provider implementation and registry constructor. Replace ID-prefix class inference and direct cardinality readiness assumptions only where the new typed contract owns them; preserve provider IDs and compatibility response fields.

## Change Boundary

Allowed: recommendation config/generator, provider interface/registry/build-tag wiring, recommendation migrations, focused provider/config/migration tests. Excluded: request UI, watch commands, scheduler behavior, Card Rewards, shared authentication implementation, deployment overlays, release-train bundles, and unrelated specs.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| REC01-TP01 | Unit | `unit` | `internal/recommendation/provider/runtime_registry_test.go` | SCN-039-005-04 | `TestProductionRegistryRejectsFixtureDescriptor` and duplicate/class mismatch cases | `./smackerel.sh test unit --go` | No |
| REC01-TP02 | Config unit | `unit` | `internal/config/recommendations_validate_test.go` | SCN-039-005-15 | `TestRecommendationsRequiredProviderConfigFailsLoudWithoutValues` | `./smackerel.sh test unit --go` | No |
| REC01-TP03 | Migration integration | `integration` | `tests/integration/recommendations_migration_test.go` | SCN-039-005-15 | Additive upgrade, conservative backfill, constraints, application rollback, and retained evidence against ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| REC01-TP04 | Adversarial E2E API | `e2e-api` | `tests/e2e/recommendations_providers_test.go` | SCN-039-005-04 | `TestRegressionProductionFixtureOnlyRegistryRefusesStartup` fails before the repair and passes after it | `./smackerel.sh test e2e` | Yes |
| REC01-TP05 | Registry compatibility E2E API | `e2e-api` | `tests/e2e/recommendations_full_regression_test.go` | SCN-039-005-04 | `TestE2ERegistryBuildTagsPreserveFixtureOnlyValidateRuntime` exercises real production/e2e wiring, not a mocked registry | `./smackerel.sh test e2e` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-039-005-04 Fixture provider cannot satisfy production: production registration rejects fixture class before inventory/readiness and serves no fixture result.
- [ ] SCN-039-005-15 Provider foundation migrates and configures without guessing: fail-loud config, conservative evidence migration, value safety, and rollback are complete.
- [ ] Typed descriptors and runtime-class registration structurally prevent fixture readiness in production.
- [ ] Required configuration is fail-loud and all provider/config outputs are value-safe.
- [ ] Migration A, conservative backfill, constraint order, backup checkpoint, and application-first rollback are executable and preserve evidence.
- [ ] Shared registry canary proves e2e fixtures remain isolated without weakening production rejection.
- [ ] Consumer and change-boundary sweeps show zero collateral edits or stale class inference.

#### Test Evidence - 5 Rows / 5 Items

- [ ] REC01-TP01 unit evidence is recorded.
- [ ] REC01-TP02 config-unit evidence is recorded.
- [ ] REC01-TP03 migration-integration evidence is recorded.
- [ ] REC01-TP04 adversarial red-to-green E2E API evidence is recorded.
- [ ] REC01-TP05 registry compatibility E2E API evidence is recorded.

#### Build Quality Gate

- [ ] Focused checks, broader Go tests, lint, format check, migration lint, artifact lint, traceability, documentation alignment, zero warnings, and zero forbidden defaults/fixtures all pass with current-session evidence.