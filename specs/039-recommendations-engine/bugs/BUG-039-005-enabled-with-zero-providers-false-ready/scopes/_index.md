# Recommendation Readiness Scopes

Links: [spec.md](../spec.md) | [design.md](../design.md) | [report.md](../report.md) | [uservalidation.md](../uservalidation.md)

## Execution Outline

### Phase Order

1. **Provider contract and migration foundation** - establish typed production/fixture descriptors, explicit provider inventory, additive schema, fail-loud SST, and rollback checkpoints.
2. **Google Places production adapter** - wire and validate the real adapter contract against a protocol-compatible provider service.
3. **Yelp production adapter** - wire and validate the independent real adapter contract, including partial-coverage behavior.
4. **Availability and startup truth** - compute category/operation readiness from configuration, registration, class, and fresh health; enforce required versus optional behavior.
5. **Request outcomes and evidence** - gate requests before persistence and preserve distinct results, no-match, filtered-empty, degraded, and typed failure outcomes.
6. **Watch and scheduler eligibility** - prevent inert watch creation or synthetic runs while preserving safe pause, silence, delete, and existing-watch visibility.
7. **Shared API and accessible UI projections** - make request, watch, provider-status, and compatibility routes consume one availability contract with safe auth/privacy behavior.
8. **Rollout and cross-surface regression** - execute migration/backfill/rollback checks, provider-compatible live validation, stress, traceability, and the complete no-interception browser matrix.

### New Types And Signatures

- `provider.Descriptor{ID, DisplayName, Class, Categories}` and runtime-class-aware `provider.Registry`.
- `availability.Inventory`, `availability.Service`, `availability.Gate`, and immutable `AvailabilitySnapshot`.
- `ProviderExecutionError`, `ProviderEvidence`, `CapabilityState`, `AvailabilityCause`, `OutcomeClass`, and `SafeErrorClass`.
- `GET /api/recommendations/availability?category=<category>&operation=<operation>[&view=operator]`.
- Additive provider runtime, request outcome, and watch-run evidence columns described by design migrations A-C.
- Required SST keys: `RECOMMENDATIONS_REQUIRED`, provider health maximum age, and provider health timeout.

### Validation Checkpoints

- Scope 01 blocks adapter work until migration, configuration, registry-class, and rollback contract checks pass.
- Scopes 02-03 block readiness claims until both real adapter implementations pass independent protocol-compatible integration and typed-failure checks.
- Scope 04 blocks request/watch consumers until required and optional startup/availability behavior passes adversarial zero-provider tests.
- Scopes 05-07 each require scenario-specific live API and Playwright regression proof before the next consumer is changed.
- Scope 08 runs the complete migration, stress, security/privacy, accessibility, artifact-lint, and traceability gates before implementation can be certified.

## Dependency Graph

| # | Scope | Spec Scenarios | Depends On | Surfaces | Status |
|---|---|---|---|---|---|
| 01 | [Provider contract and migration foundation](01-provider-contract-foundation/scope.md) | SCN-039-005-02, 04, 07 | - | config, provider registry, PostgreSQL, security | Not Started |
| 02 | [Google Places production adapter](02-google-places-adapter/scope.md) | SCN-039-005-01, 03, 05 | 01 | provider adapter, health, attribution | Not Started |
| 03 | [Yelp production adapter](03-yelp-adapter/scope.md) | SCN-039-005-01, 03, 08 | 02 | provider adapter, partial coverage | Not Started |
| 04 | [Availability and startup truth](04-availability-startup-truth/scope.md) | SCN-039-005-01, 02, 03, 08, 10 | 03 | availability, startup, metrics | Not Started |
| 05 | [Request outcomes and evidence](05-request-outcomes-evidence/scope.md) | SCN-039-005-01, 03, 05, 07, 08 | 04 | request API, engine, evidence store | Not Started |
| 06 | [Watch and scheduler eligibility](06-watch-scheduler-eligibility/scope.md) | SCN-039-005-02, 03, 06, 07 | 05 | watches, scheduler, persistence | Not Started |
| 07 | [Shared API and accessible UI projections](07-shared-api-ui-projections/scope.md) | SCN-039-005-01 through 10 | 06 | request UI, watch UI, status, auth | Not Started |
| 08 | [Rollout and cross-surface regression](08-rollout-cross-surface-regression/scope.md) | SCN-039-005-01 through 10 | 07 | migrations, rollback, live validation, stress | Not Started |

## Planning Invariants

- Scope execution is strictly ordered; a scope starts only after every listed dependency is Done with evidence.
- Production readiness requires configured, registered, category-compatible, fresh, healthy, non-fixture provider capacity.
- A real provider-compatible validate stack supplies live behavior; Playwright never intercepts first-party recommendation traffic.
- Required zero-provider configuration refuses startup. Optional zero-provider operation remains an isolated unavailable capability.
- Unavailable, degraded, no-match, filtered-empty, authentication, quota, timeout, malformed-response, and provider-error outcomes remain mutually distinguishable.
- PostgreSQL remains authoritative; a pre-execution refusal creates no request, trace, watch, or watch-run business row.
- Every schema change is additive, backfilled conservatively, constraint-verified, and paired with an application-first rollback checkpoint.
- Fixture providers remain build- and type-isolated and cannot register in a production runtime class.
- Provider evidence, logs, traces, status, and metrics never expose credentials, raw queries, precise locations, personal data, or raw upstream bodies.

## Change Boundary

**Allowed file families:** recommendation config/generation, provider registry/adapters, recommendation availability/engine/store, recommendation request/watch API and web handlers/templates, recommendation migrations, recommendation-specific tests, and directly affected managed documentation.

**Excluded file families:** Card Rewards, Assistant, knowledge graph, notification delivery, unrelated provider/model registries, deployment overlays, release-train configuration, spec 079, and every other spec or bug packet.

## Impact And Trace Planning

- `.github/bubbles-project.yaml` declares observability posture `wired` but defines only `core.health`; no recommendation-specific workflow exists. Recommendation scopes therefore do not mislabel evidence as `core.health`.
- Each runtime scope still plans bounded recommendation metrics/log/span assertions. A recommendation trace workflow must be added by its owning design/config process before any `observabilityWorkflow` tag is introduced.
- The repository has no `testImpact` map, so each scope uses scenario-first focused checks followed by the canonical broader unit, integration, e2e-api, e2e-ui, and stress gates stated in its plan.

## Consumer Impact Sweep

The shared availability cutover must update recommendation request handlers, reactive engine, watch commands, scheduler evaluator, provider compatibility API, request/watch/status templates, navigation/deep links, authorization projections, metrics/alerts, managed docs, and all live API/browser tests. Searches for direct `RecommendationsEnabled`, `Registry.Len()`, empty-registry `no_eligible`, and fixture-prefix readiness checks must reach zero outside explicitly historical or test assertions.

### Definition of Done

The packet reaches implementation completion only when Scopes 01-08 are Done in dependency order, every scope's Test Plan row has its matching evidence-backed DoD item, all scenario-manifest contracts have real live-system evidence, migration and rollback checkpoints pass, both production adapters pass provider-compatible validation, and the final artifact-lint, traceability, regression, security, accessibility, stress, and validation gates pass without warnings or unresolved findings.