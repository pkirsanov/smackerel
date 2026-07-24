# Scopes: [BUG-039-005] Enabled With Zero Providers Reports False Ready — SUPERSEDED

Links: [scopes/_index.md](scopes/_index.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> **SUPERSEDED — DO NOT EXECUTE.** This single-file draft was the pre-decomposition
> "initial bug-scope shape." The authoritative, active execution inventory is the
> eight dependency-ordered per-scope directory plan under
> [scopes/_index.md](scopes/_index.md) (`state.json.scopeLayout = per-scope-directory`,
> `artifacts.scopes = scopes/_index.md`). The content below is retained only as
> historical context and carries no active status, DoD, or evidence obligation.

## Superseded Scope (Do Not Execute): Provider-Backed Capability Availability

**Status:** Superseded (see [scopes/_index.md](scopes/_index.md))  
**Priority:** n/a  
**Depends On:** n/a

### Gherkin Scenarios

`SCN-039-005-01` through `SCN-039-005-09` in [spec.md](spec.md) cover healthy readiness, zero providers, unhealthy providers, production fixture exclusion, no match, watch refusal, auth/privacy, partial degradation, and accessible responsive state.

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|---|---|---|---|---|---|
| Healthy provider | Valid provider and session | Open/request recommendation | Ready action and sourced result | e2e-ui | Not recorded |
| Zero providers | Enabled; registry empty | Open/request/watch | Unavailable/refused; no inert watch | e2e-ui | Not recorded |
| Partial degradation | One healthy, one failing provider | Request | Verified degraded result with provenance | e2e-ui | Not recorded |
| Accessible narrow status | Keyboard/screen reader; mobile viewport | Inspect ready/unavailable/degraded/no-match/error | Perceivable state/action; no overlap | e2e-ui | Not recorded |

### Shared Infrastructure Impact Sweep

- Enablement config, provider registry, provider health, recommendation API/UI, watches, status/readiness, telemetry, docs/release claims.

### Change Boundary

- Allowed after owner design: recommendation config/registry/health/API/UI/watch/tests/docs.
- Excluded: Card Rewards optimizer, fixture promotion to production, production data, operator-owned deployment assets, spec 104, and specs 105/106.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Unit | `unit` | `internal/config/recommendations_validate_test.go`, `internal/api/recommendations_test.go` | Required/optional availability and typed response mapping | `./smackerel.sh test unit --go` | No |
| Integration | `integration` | `tests/integration/recommendation_provider_registry_test.go`, `recommendation_providers_test.go` | Empty/healthy/unhealthy/partial registry and production fixture exclusion | `./smackerel.sh test integration` | Yes |
| Watch Integration | `integration` | `tests/integration/recommendation_watches_test.go`, `recommendation_watch_audit_test.go` | No inert watch without provider; ready watch lifecycle remains auditable | `./smackerel.sh test integration` | Yes |
| Regression E2E API | `e2e-api` | `tests/e2e/recommendations_api_test.go`, `recommendations_providers_test.go` | Zero provider unavailable, healthy no-match, partial degradation, auth | `./smackerel.sh test e2e` | Yes |
| Regression E2E UI | `e2e-ui` | `tests/e2e/recommendations_web_test.go`, `recommendations_watches_web_test.go` | Visible availability/actions/results/watches without fake data | `./smackerel.sh test e2e` | Yes |
| Security/Privacy | `integration` | `tests/integration/recommendation_privacy_test.go` | Credentials/query/personal data absent from status/errors/telemetry | `./smackerel.sh test integration` | Yes |
| Stress | `stress` | `tests/stress/recommendations_test.go` | Provider-health churn and concurrent requests preserve bounded truthful readiness | `./smackerel.sh test stress` | Yes |
| Broader E2E Regression | `e2e-api` | `tests/e2e/recommendations_full_regression_test.go` | Existing feedback/why/constraints/consent flows remain intact | `./smackerel.sh test e2e` | Yes |

### Adversarial Regression Contract

Enable recommendations with an empty production registry and both Google/Yelp disabled; old code mounts false-ready routes, repaired code must refuse required mode or expose optional unavailable state with no action/watch. Attempt to register a fixture provider in production and assert it cannot make readiness true.

### Definition of Done - Tiered Validation

- [ ] `bubbles.design` confirms registry, requiredness, health, route/UI, and watch root causes.
- [ ] Readiness requires at least one configured healthy production provider.
- [ ] Required zero-provider mode fails loud or optional mode is unavailable with actions hidden/refused.
- [ ] Fixture providers cannot register or satisfy production readiness.
- [ ] Healthy no-match, partial degradation, zero provider, unhealthy provider, auth, timeout/quota, and error states are distinct.
- [ ] Watch creation/refresh cannot persist inert watches without a healthy provider.
- [ ] Independent provider-readiness canary passes before broad suite reruns.
- [ ] Rollback/restore and provider health recovery are documented and verified.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Pre-fix zero-provider/fixture adversarial regressions fail false-ready behavior.
- [ ] Post-fix live UI/API/watch/readiness regressions pass.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate passes with zero warnings, no defaults/fixtures, lint/format clean, artifact lint clean, and docs aligned.
