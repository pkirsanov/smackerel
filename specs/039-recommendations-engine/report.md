# Execution Reports: 039 Recommendations Engine

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

Planning artifacts were re-authored by `bubbles.plan` on 2026-04-27 into 6 vertical-slice scopes. Runtime implementation evidence has not been recorded; execution agents append evidence under the matching scope sections when each scope runs.

## Completion Statement

No scope is complete. The active execution inventory is defined in [scopes.md](scopes.md), with `scope-01-foundation-schema` in progress and scopes 02-06 not started. No certification has been issued.

## Test Evidence

No runtime tests have been executed during planning. Required commands and test files are listed per scope in [scopes.md](scopes.md) and the live-test contract lives in [scenario-manifest.json](scenario-manifest.json). Validation uses the repo CLI `./smackerel.sh`; command evidence is recorded only when execution phases run the planned tests.

## Per-Scope Evidence

Implementation agents append one block per scope using the template at the bottom of this file. Do not edit existing blocks once committed; add a new dated entry instead.

### scope-01-foundation-schema

### Scope: scope-01-foundation-schema — 2026-04-27 02:45

#### Summary

`bubbles.implement` added the Scope 1 foundation slice: recommendation schema migration, typed SST config, provider registry and test fixture, interface scaffolds under `internal/recommendation/**`, no-provider request persistence, authenticated recommendation request route, and `/status` provider-health rendering. The scope is **not complete** because full integration and e2e gates did not exit 0; evidence below records exactly what passed and what remains uncertain.

#### Decision Record

- Provider registry defaults to empty and fixture providers are build-tagged for integration/e2e only.
- Disabled providers may carry empty API-key secret fields per SST policy; enabled providers must fail loud if API keys are empty or missing.
- `POST /api/recommendations/requests` persists `agent_traces` + `recommendation_requests` and returns `status: "no_providers"` only while the registry is empty. Provider execution is left to Scope 2.
- A no-cache test-image build was required because cached test images did not initially embed the new migration/code.

#### Completion Statement (MANDATORY)

Checked DoD items in [scopes.md](scopes.md): recommendation package compile/lint, fail-loud recommendation config validation, SST config generation/check, no-provider API persistence, and check/lint gates.

Unchecked DoD items: migration up/down proof, `/status` live UI proof, scenario-specific e2e proof, broader e2e suite, and full integration suite. **Uncertainty Declaration:** full live-stack validation is blocked by a mix of in-scope test retry interruption and unrelated in-flight drive migration expectations; the feature must remain `In Progress`.

### Code Diff Evidence

Files added/modified for this scope include:

- `internal/db/migrations/022_recommendations.sql`
- `internal/recommendation/{provider,location,dedupe,graph,rank,policy,quality,store,tools}`
- `internal/config/recommendations.go`, `internal/config/config.go`, `internal/config/validate_test.go`, `internal/config/recommendations_validate_test.go`
- `internal/api/recommendations.go`, `internal/api/recommendations_test.go`, `internal/api/health.go`, `internal/api/router.go`
- `internal/web/handler.go`, `internal/web/templates.go`, `cmd/core/wiring.go`
- `config/smackerel.yaml`, `scripts/commands/config.sh`
- `tests/integration/recommendation_providers_test.go`, `tests/integration/recommendations_migration_test.go`, `tests/e2e/operator_status_test.go`

**Phase:** validate  
**Command:** `git diff --stat -- specs/039-recommendations-engine/state.json specs/039-recommendations-engine/report.md internal/db/migrations/022_recommendations.sql internal/api/recommendations.go internal/web/handler.go internal/web/templates.go cmd/core/wiring.go tests/e2e/operator_status_test.go tests/integration/recommendations_migration_test.go tests/integration/recommendation_providers_test.go`  
**Exit Code:** 0  
**Claim Source:** executed

```
git diff --stat -- specs/039-recommendations-engine/state.json specs/039-recommendations-engine/report.md internal/db/migrations/022_recommendations.sql internal/api/recommendations.go internal/web/handler.go internal/web/templates.go cmd/core/wiring.go tests/e2e/operator_status_test.go tests/integration/recommendations_migration_test.go tests/integration/recommendation_providers_test.go
cmd/core/wiring.go        | 17 +++++++++++
internal/web/handler.go   | 77 ++++++++++++++++++++++++++++++++++++++++++-----
internal/web/templates.go | 12 ++++++++
3 files changed, 99 insertions(+), 7 deletions(-)
```

**Phase:** validate  
**Command:** `git status --short specs/039-recommendations-engine/state.json specs/039-recommendations-engine/report.md internal/db/migrations/022_recommendations.sql internal/api/recommendations.go internal/web/handler.go internal/web/templates.go cmd/core/wiring.go tests/e2e/operator_status_test.go tests/integration/recommendations_migration_test.go tests/integration/recommendation_providers_test.go`  
**Exit Code:** 0  
**Claim Source:** executed

```
 M cmd/core/wiring.go
 M internal/web/handler.go
 M internal/web/templates.go
?? internal/api/recommendations.go
?? internal/db/migrations/022_recommendations.sql
?? specs/039-recommendations-engine/report.md
?? specs/039-recommendations-engine/state.json
?? tests/e2e/operator_status_test.go
?? tests/integration/recommendation_providers_test.go
?? tests/integration/recommendations_migration_test.go
```

#### Test Evidence (ALL TYPES REQUIRED)

**Phase:** implement  
**Command:** `./smackerel.sh config generate`  
**Exit Code:** 0  
**Claim Source:** executed

```
Generated /home/philipk/smackerel/config/generated/dev.env
Generated /home/philipk/smackerel/config/generated/nats.conf
```

**Phase:** implement  
**Command:** `./smackerel.sh --env test config generate`  
**Exit Code:** 0  
**Claim Source:** executed

```
Generated /home/philipk/smackerel/config/generated/test.env
Generated /home/philipk/smackerel/config/generated/nats.conf
```

**Phase:** implement  
**Command:** `./smackerel.sh test unit`  
**Exit Code:** 0  
**Claim Source:** executed

```
ok      github.com/smackerel/smackerel/internal/config  0.149s
ok      github.com/smackerel/smackerel/internal/api     (cached)
?       github.com/smackerel/smackerel/internal/recommendation  [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/provider (cached)
343 passed, 1 warning in 20.87s
```

**Phase:** implement  
**Command:** `./smackerel.sh check`  
**Exit Code:** 0  
**Claim Source:** executed

```
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** implement  
**Command:** `./smackerel.sh lint`  
**Exit Code:** 0  
**Claim Source:** executed

```
All checks passed!
=== Validating web manifests ===
	OK: web/pwa/manifest.json
	OK: PWA manifest has required fields
	OK: web/extension/manifest.json
	OK: Chrome extension manifest has required fields (MV3)
	OK: web/extension/manifest.firefox.json
	OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Checking extension version consistency ===
	OK: Extension versions match (1.0.0)
Web validation passed
```

**Phase:** implement  
**Command:** `./smackerel.sh format --check`  
**Exit Code:** 0  
**Claim Source:** executed

```
41 files already formatted
```

**Phase:** implement  
**Command:** `./smackerel.sh --env test --no-cache build`  
**Exit Code:** 0  
**Claim Source:** executed

```
[+] Building 36/36 FINISHED
smackerel-core  Built
smackerel-ml    Built
```

**Phase:** implement  
**Command:** `./smackerel.sh test integration`  
**Exit Code:** 1  
**Claim Source:** executed

```
PASS: post-command --volumes removed smackerel-test-postgres-data
PASS: core reached /api/health on consecutive runs over a retained postgres volume with re-applied initial migration
=== RUN   TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace
--- PASS: TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace (0.04s)
=== RUN   TestRecommendationMigration_UpDownRoundTripIsIdempotent
		recommendations_migration_test.go:55: read recommendation migration: open internal/db/migrations/022_recommendations.sql: no such file or directory
--- FAIL: TestRecommendationMigration_UpDownRoundTripIsIdempotent (0.01s)
```

After this run, `tests/integration/recommendations_migration_test.go` was corrected to read `../../internal/db/migrations/022_recommendations.sql`. A later full-suite run did not reach this test because the stack failed during NATS startup; an earlier full-suite run also exposed unrelated in-flight drive migration failures:

```
=== RUN   TestDriveMigration022_ExpiresAtAndOAuthStatesApplied
		drive_migration_apply_test.go:301: drive_connections.expires_at is missing — migration 022 did not apply
		drive_migration_apply_test.go:305: drive_oauth_states table is missing — migration 022 did not apply
--- FAIL: TestDriveMigration022_ExpiresAtAndOAuthStatesApplied (0.05s)
```

**Phase:** implement  
**Command:** `./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

```
=== SCN-002-001: Docker compose cold start ===
PASS: SCN-002-001 (status=degraded)
=== SCN-002-004: Data persistence across restarts ===
Restarting services...
dependency failed to start: container smackerel-test-nats-1 exited (1)
```

#### Uncertainty Declarations

- Migration up/down proof remains unchecked after the final test-path correction because the later integration retry stopped before the recommendation package executed.
- `/status` live UI proof remains unchecked because e2e stopped in an earlier persistence/restart scenario before `tests/e2e/operator_status_test.go` ran.
- Full integration is blocked by unrelated in-flight drive migration expectations for `TestDriveMigration022_ExpiresAtAndOAuthStatesApplied` plus intermittent test-stack NATS startup failure. This is not treated as Scope 1 complete evidence.

#### Scenario Contract Evidence

- `SCN-039-002`: partially proven by `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` PASS in `./smackerel.sh test integration` and unit API coverage. UI half remains unchecked.
- `SCN-039-003`: proven by `internal/config/recommendations_validate_test.go` in `./smackerel.sh test unit`.
- `SCN-039-001`: implementation exists, but live up/down proof remains unchecked after the final test-path correction.

#### Coverage Report

No coverage mode was run. Unit and integration/e2e evidence above is command-execution evidence only.

#### Lint/Quality

Final `./smackerel.sh check`, `./smackerel.sh lint`, and `./smackerel.sh format --check` all exited 0 after the final source patch.

#### Governance Evidence

**Phase:** implement  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine`  
**Exit Code:** 1  
**Claim Source:** executed

```
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template tokens in scopes.md
✅ No unfilled evidence template tokens in report.md
✅ No repo-CLI bypass detected in report.md command evidence
❌ Top-level status 'in_progress' does not match certification.status 'not_started'
Artifact lint FAILED with 1 issue(s).
```

This remaining artifact-lint failure is validate-owned state metadata. `bubbles.implement` did not edit `certification.status` or other certification fields.

**Phase:** implement  
**Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/039-recommendations-engine`  
**Exit Code:** 0  
**Claim Source:** executed

```
ℹ️  Checking traceability for Scope 1: scope-01-foundation-schema
✅ Scope 1: scope-01-foundation-schema scenario mapped to Test Plan row: SCN-039-001 Migration applies and rolls back cleanly
✅ Scope 1: scope-01-foundation-schema scenario maps to concrete test file: tests/integration/recommendations_migration_test.go
✅ Scope 1: scope-01-foundation-schema scenario mapped to Test Plan row: SCN-039-002 Provider registry is empty by default
✅ Scope 1: scope-01-foundation-schema scenario maps to concrete test file: tests/integration/recommendation_providers_test.go
✅ Scope 1: scope-01-foundation-schema scenario mapped to Test Plan row: SCN-039-003 Config validation fails loud on missing required keys
✅ Scope 1: scope-01-foundation-schema scenario maps to concrete test file: internal/config/recommendations_validate_test.go
ℹ️  DoD fidelity: 30 scenarios checked, 30 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
```

#### Spot-Check Recommendations

Keep Scope 1 in progress. The next owner should resolve the migration-number collision with in-flight drive work (feature 038 expects a migration 022) and stabilize the test-stack NATS restart issue before attempting to mark live-stack DoD complete.

#### Validation Summary

Implementation code is present and the unit/build/SST/no-provider API evidence is green. Live-stack certification is incomplete; do not promote Scope 1 to Done.

### Scope: scope-01-foundation-schema — 2026-04-27 04:40 UTC — DevOps E2E Harness

#### Summary

`bubbles.devops` fixed the e2e harness sequencing blocker that caused `./smackerel.sh test e2e` to fail in lifecycle shell tests before Go e2e tests could execute. The public CLI now runs the operator-status Go canary before broad package e2e tests, uses the existing shared-stack shell runner instead of manually invoking each shell scenario, collects phase failures before returning the final exit code, and runs lifecycle scenarios last. The required 039 operator-status e2e proof now executes and passes, but the full e2e command still exits 1 because of unrelated broad Go e2e failures and two shared shell scenario failures.

#### Decision Record

- Kept the change inside the e2e harness boundary: CLI dispatch, shared runner selection, Go e2e runner ordering, and lifecycle test cleanup/waiting.
- Preserved lifecycle scenarios instead of removing or skipping them; they now run after Go e2e and shared-stack shell phases.
- Used existing `tests/e2e/lib/helpers.sh` cleanup and health-wait behavior for lifecycle tests so explicit disposable test volumes are cleaned consistently.
- Added a canary-first Go e2e run for `TestOperatorStatus_RecommendationProvidersEmptyByDefault` so broad package hangs cannot mask the 039 status-page proof.

#### Completion Statement (MANDATORY)

Checked DevOps-owned item: `./smackerel.sh test e2e` no longer fails before Go e2e tests run, and `TestOperatorStatus_RecommendationProvidersEmptyByDefault` executes and passes. Unchecked Scope 1 item: broader `./smackerel.sh test e2e` still exits 1, so Scope 1 remains `In Progress` and must not be certified Done.

#### Code Diff Evidence

Files changed by DevOps harness work:

- `smackerel.sh`
- `scripts/runtime/go-e2e.sh`
- `tests/e2e/run_all.sh`
- `tests/e2e/test_compose_start.sh`
- `tests/e2e/test_persistence.sh`
- `tests/e2e/test_config_fail.sh`

#### Shared Infrastructure Impact Sweep

- **Boundary:** e2e execution order and cleanup only; no product recommendation behavior, runtime config source, generated config, Compose service definitions, or persistent dev volumes were changed.
- **Resource classification:** affected resources are disposable `smackerel-test` containers/networks/volumes created by the repo CLI test stack.
- **Canary coverage:** `TestOperatorStatus_RecommendationProvidersEmptyByDefault` now runs before broad Go e2e; lifecycle canary `test_persistence` now passes restart/persistence after the Go phase.
- **Rollback note:** revert the DevOps changes in the files listed above to restore the previous direct shell-test enumeration and original lifecycle script cleanup behavior.

#### Test Evidence (ALL TYPES REQUIRED)

**Phase:** devops  
**Command:** `timeout 1800 ./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

```
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.07s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.088s
=== RUN   TestBrowserHistory_E2E_InitialSyncProducesArtifacts
	browser_history_e2e_test.go:114: search returned 405:
--- FAIL: TestBrowserHistory_E2E_InitialSyncProducesArtifacts (0.10s)
=== RUN   TestE2E_CaptureProcessSearch
	capture_process_search_test.go:104: artifact not processed within 60s timeout — pipeline may be broken
--- FAIL: TestE2E_CaptureProcessSearch (60.20s)
=== RUN   TestE2E_DomainExtraction
	domain_e2e_test.go:121: domain extraction not completed within 90s timeout — last domain_status= (pipeline or ML sidecar may not support domain extraction)
--- FAIL: TestE2E_DomainExtraction (90.24s)
=== RUN   TestKnowledgeStore_TablesExist
	knowledge_store_test.go:26: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
--- FAIL: TestKnowledgeStore_TablesExist (0.05s)
=== RUN   TestKnowledgeSynthesis_PipelineRoundTrip
	knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.22s)
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.04s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        198.139s
```

**Phase:** devops  
**Command:** `timeout 1800 ./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

```
== Phase 1: Shared-stack tests (30 tests) ==
Services healthy after 6s
PASS: test_capture_pipeline
PASS: test_voice_pipeline
PASS: test_llm_failure_e2e
PASS: test_capture_api
PASS: test_capture_errors
PASS: test_voice_capture_api
PASS: test_knowledge_graph
PASS: test_graph_entities
PASS: test_search
PASS: test_search_filters
PASS: test_search_empty
PASS: test_telegram
PASS: test_telegram_auth
PASS: test_telegram_voice
PASS: test_telegram_format
PASS: test_digest
PASS: test_digest_quiet
FAIL: test_digest_telegram (exit=1)
PASS: test_web_ui
PASS: test_web_detail
PASS: test_web_settings
PASS: test_connector_framework
PASS: test_imap_sync
PASS: test_caldav_sync
PASS: test_youtube_sync
PASS: test_bookmark_import
FAIL: test_topic_lifecycle (exit=1)
PASS: test_settings_connectors
PASS: test_maps_import
PASS: test_browser_sync
Total:  30
Passed: 28
Failed: 2
```

**Phase:** devops  
**Command:** `timeout 1800 ./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

```
== Phase 2: Lifecycle tests (3 tests) ==
--- Running: test_compose_start ---
PASS: SCN-002-001 (status=degraded)
--- Running: test_persistence ---
=== SCN-002-004: Data persistence across restarts ===
Insert verified (count=1)
Stopping services (preserving volumes)...
Restarting services...
Waiting for services to be healthy (max 120s)...
Services healthy after 4s
PASS: SCN-002-004 (data persisted, count=1)
--- Running: test_config_fail ---
Process exited with code 1 (expected non-zero)
PASS: SCN-002-044 (exit=1, named 3 missing variables)
Total:  3
Passed: 3
Failed: 0
Command exited with code 1
```

#### Uncertainty Declarations

- Full `./smackerel.sh test e2e` remains failed after the harness fix. The remaining failures are not in the e2e lifecycle harness: browser-history search 405s, capture/domain processing timeouts, knowledge stats 500, knowledge synthesis external URL extraction 422, digest Telegram delivery tracking, and duplicate topic seeding.
- The 039 `/status` recommendation-provider proof is no longer blocked: the exact test executed and passed twice in the final full e2e run.

#### Scenario Contract Evidence

- `SCN-039-002`: live `/status` half is now proven by `TestOperatorStatus_RecommendationProvidersEmptyByDefault` PASS in `./smackerel.sh test e2e`.
- Broader e2e suite remains failed, so Scope 1 DoD remains incomplete.

#### Coverage Report

No coverage mode was run for the DevOps harness change.

#### Lint/Quality

No separate lint command was run after the DevOps harness change. The changed shell paths were exercised by `./smackerel.sh test e2e`; the command still exits 1 due non-harness e2e failures listed above.

#### Spot-Check Recommendations

Route remaining full-e2e failures to product implementation/stabilization owners. DevOps should keep ownership only if a remaining failure is traced back to test-stack lifecycle, cleanup, generated config, or CI dispatch.

#### Validation Summary

The DevOps-owned lifecycle sequencing blocker is fixed: Go e2e runs before lifecycle shell tests, the 039 operator-status canary passes, and `test_persistence` passes after restart. Full e2e is still red for unrelated product/test-data failures, so Scope 1 must remain In Progress.

### Scope: scope-01-foundation-schema - 2026-04-29 00:05 UTC - Validation Reassessment

#### Summary

`bubbles.validate` reassessed feature 039 after BUG-039-002 and BUG-031-003 were certified. The old Scope 1 `/status` and broad e2e blockers are cleared in the current run: `./smackerel.sh test e2e` exits 0, the shell e2e phase reports 34/34 passed, and `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passes. Scope 1 is still not promotable because the full integration command exits 1 and the plan/state gates still have blocking findings.

#### Outcome Contract Verification (G070)

| Field | Declared | Evidence | Status |
|-------|----------|----------|--------|
| Intent | Ranked, sourced, personalized recommendations with visible why/source detail | Scope 1 only proves foundation schema, empty provider registry, no-provider persistence, config validation, and operator status. Full recommendation behavior is not implemented in completed evidence. | Not satisfied feature-wide |
| Success Signal | Telegram ramen recommendations within 10s, proactive price-drop watch, and inspectable provenance | No evidence demonstrates delivered recommendations, watches, or provenance inspection. Scope 1 evidence is foundational only. | Not satisfied |
| Hard Constraints | Multi-provider aggregation, graph reranking, source/personal citations, suppression, watch controls, precision policy, provider outage handling, opt-in monitoring, disclosure/safety/read-only behavior | Scope 1 evidence preserves fail-loud config and empty-provider/no-fabricated-result behavior, but most hard constraints are assigned to later scopes. | Partially preserved, not complete |
| Failure Condition | Feature fails if recommendation outputs violate graph, source, watch, precision, consent, safety, repeat, or cost/effort constraints | Current foundation state does not deliver real recommendations yet; failure condition is not triggered by delivered recommendation behavior, but outcome is not achieved. | Not triggered, not complete |

#### Completion Statement (MANDATORY)

Scope 1 remains `In Progress`. Current validation supports unblocking the stale e2e concern in prior evidence, but does not support promotion to `Done` because `./smackerel.sh test integration` fails and Scope 1 still has unchecked DoD owned by planning/execution artifacts. `certification.completedScopes` and `certification.certifiedCompletedPhases` remain empty.

### Code Diff Evidence

Validation-owned artifact changes for this reassessment:

- `specs/039-recommendations-engine/state.json`: synchronized `certification.status` from `not_started` to `in_progress`, recorded non-certified scope progress, and updated `lastUpdatedAt`.
- `specs/039-recommendations-engine/report.md`: appended this validation reassessment block.

Runtime implementation evidence was recorded in the earlier Scope 1 code-diff block with non-artifact paths including `internal/db/migrations/022_recommendations.sql`, `internal/api/recommendations.go`, `internal/web/handler.go`, `internal/web/templates.go`, `cmd/core/wiring.go`, and `tests/e2e/operator_status_test.go`. No runtime source files were changed by this validate pass.

**Phase:** validate  
**Command:** `git status --short specs/039-recommendations-engine/state.json specs/039-recommendations-engine/report.md internal/db/migrations/022_recommendations.sql internal/api/recommendations.go internal/web/handler.go internal/web/templates.go cmd/core/wiring.go tests/e2e/operator_status_test.go tests/integration/recommendations_migration_test.go tests/integration/recommendation_providers_test.go`  
**Exit Code:** 0  
**Claim Source:** executed

```
 M cmd/core/wiring.go
 M internal/web/handler.go
 M internal/web/templates.go
?? internal/api/recommendations.go
?? internal/db/migrations/022_recommendations.sql
?? specs/039-recommendations-engine/report.md
?? specs/039-recommendations-engine/state.json
?? tests/e2e/operator_status_test.go
?? tests/integration/recommendation_providers_test.go
?? tests/integration/recommendations_migration_test.go
```

#### Test Evidence (ALL TYPES REQUIRED)

**Phase:** validate  
**Command:** `timeout 180 ./smackerel.sh check`  
**Exit Code:** 0  
**Claim Source:** executed

```
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** validate  
**Command:** `timeout 600 ./smackerel.sh format --check`  
**Exit Code:** 0  
**Claim Source:** executed

```
42 files already formatted
```

**Phase:** validate  
**Command:** `timeout 900 ./smackerel.sh lint`  
**Exit Code:** 0  
**Claim Source:** executed

```
All checks passed!
=== Validating web manifests ===
OK: web/pwa/manifest.json
OK: Chrome extension manifest has required fields (MV3)
OK: Firefox extension manifest has required fields (MV2 + gecko)
Web validation passed
```

**Phase:** validate  
**Command:** `timeout 1200 ./smackerel.sh build`  
**Exit Code:** 0  
**Claim Source:** executed

```
smackerel-core  Built
smackerel-ml    Built
```

**Phase:** validate  
**Command:** `timeout 900 ./smackerel.sh test unit --go`  
**Exit Code:** 0  
**Claim Source:** executed

```
Go unit packages passed, including internal/config, internal/api, and internal/recommendation/provider.
```

**Phase:** validate  
**Command:** `timeout 1200 ./smackerel.sh test unit`  
**Exit Code:** 0  
**Claim Source:** executed

```
Go unit packages passed.
Python unit tests: 352 passed, 2 warnings in 32.17s.
```

**Phase:** validate  
**Command:** `timeout 3600 ./smackerel.sh test e2e`  
**Exit Code:** 0  
**Claim Source:** executed  
**Interpretation:** The stale BUG-039-002 and BUG-031-003 live-stack blockers are resolved in the current run. Scope 1's broad e2e command requirement is now green, although Scope 1 still cannot promote while integration remains red and plan-owned checkboxes remain unchecked.

```
Shell e2e phase: Total: 34, Passed: 34, Failed: 0
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault
PASS
Go e2e packages passed.
```

**Phase:** validate  
**Command:** `timeout 1800 ./smackerel.sh test integration`  
**Exit Code:** 1  
**Claim Source:** executed  
**Interpretation:** 039-specific integration coverage passes in the current run, but the command fails because shared NATS integration tests fail and the suite contains skips. This directly blocks the Scope 1 DoD requiring `./smackerel.sh test unit` and `./smackerel.sh test integration` to pass with no skips.

```
=== RUN   TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace
--- PASS: TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace
=== RUN   TestRecommendationMigration_UpDownRoundTripIsIdempotent
--- PASS: TestRecommendationMigration_UpDownRoundTripIsIdempotent
--- FAIL: TestNATS_PublishSubscribe_Artifacts
nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain
nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion
expected exhausted message to be removed from delivery stream, got 1 message(s)
Browser-history fixture integration tests skipped: 6
GuestHost integration stub tests skipped: multiple stub-mode tests
FAIL
```

**Phase:** validate  
**Command:** `timeout 900 ./smackerel.sh test stress`  
**Exit Code:** 0  
**Claim Source:** executed

```
Health stress: 25/25 passed.
Search stress: 1100 artifacts, 10 queries, average time 1585ms, threshold 3000ms, failures 0.
Search stress test passed: all queries completed under 3000ms with 1100 artifacts.
```

#### Governance Evidence

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine`  
**Exit Code:** 1 before state reconciliation; 0 after validate-owned state reconciliation  
**Claim Source:** executed  
**Interpretation:** The previously blocking state drift is resolved by this validate pass.

```
Before reconciliation:
Top-level status 'in_progress' does not match certification.status 'not_started'
Artifact lint FAILED with 1 issue(s).

After reconciliation:
Top-level status matches certification.status
Artifact lint PASSED.
```

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/039-recommendations-engine`  
**Exit Code:** 0  
**Claim Source:** executed

```
DoD fidelity: 30 scenarios checked, 30 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
```

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/039-recommendations-engine`  
**Exit Code:** 0  
**Claim Source:** executed

```
RESULT: PASS (0 failures, 0 warnings)
```

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/implementation-reality-scan.sh specs/039-recommendations-engine --verbose`  
**Exit Code:** 0  
**Claim Source:** executed

```
Files scanned: 12
Violations: 0
Warning: files resolved from design.md fallback; scopes should reference implementation files directly.
```

**Phase:** validate  
**Command:** `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/039-recommendations-engine`  
**Exit Code:** 1  
**Claim Source:** executed  
**Interpretation:** The validate-owned status mismatch and G053 code-diff evidence gap are resolved, but promotion remains blocked by plan/execution gates. Final transition verdict: 32 failures, 3 warnings.

```
Top-level status matches certification.status (in_progress)
scenario-manifest.json is missing requiredTestType entries for one or more scenarios (Gate G057)
scenario-manifest.json is missing linkedTests entries (Gate G057)
Scenario-first TDD evidence is recorded in the scope/report artifacts (G060 pass)
DoD items total: 77 (checked: 5, unchecked: 72)
Resolved scopes: total=6, Done=0, In Progress=1, Not Started=5, Blocked=0
Required phase 'implement' NOT in execution/certification phase records
Required phase 'test' NOT in execution/certification phase records
Required phase 'regression' NOT in execution/certification phase records
Implementation delta evidence recorded with git-backed proof and non-artifact file paths (G053 pass)
Scope/report artifacts contain G036/G040 wording hits
DoD-Gherkin content fidelity gap for SCN-039-002 (Gate G068)
TRANSITION BLOCKED: 32 failure(s), 3 warning(s)
```

#### Scenario Contract Evidence

- `SCN-039-001`: current run proves the recommendation migration up/down round trip test passes inside `./smackerel.sh test integration`, but the full integration command exits 1.
- `SCN-039-002`: current run proves the no-provider API persistence integration test and the operator-status e2e test pass; full Scope 1 promotion remains blocked by other gates.
- `SCN-039-003`: current run proves fail-loud recommendation config validation through `./smackerel.sh test unit`.

#### Remaining Blockers

1. Runtime blocker: `./smackerel.sh test integration` exits 1 due shared NATS test failures and skip-bearing integration suites.
2. Plan-owned blocker: `scenario-manifest.json` lacks `requiredTestType` and `linkedTests` entries required by G057.
3. Plan-owned blocker: scope artifacts still show 72 unchecked DoD items, including Scope 1 items whose evidence is now green but whose checkboxes are plan/execution-owned.
4. Process blocker: required full-delivery specialist phase records are absent from execution/certification state.
5. Artifact wording blocker: the current state-transition guard still reports G036/G040 wording hits in scope/report artifacts.

#### Ownership Routing Summary

| Finding | Owner Required | Reason | Re-validation Needed |
|---------|----------------|--------|----------------------|
| Shared NATS integration failures and integration skips | `bubbles.bug` / `bubbles.implement` | Runtime command required by Scope 1 exits 1 and cannot be waived by validation. | yes |
| Scenario manifest schema and stale Scope 1 DoD checkboxes | `bubbles.plan` after runtime blocker is fixed | These are plan-owned artifacts and validate cannot edit scopes.md structure or DoD status. | yes |
| Missing full-delivery phase records | `bubbles.workflow` | Full-delivery progression requires the recorded owner phases before certification. | yes |

#### Validation Summary

Do not certify feature 039 done and do not promote Scope 1. Current validation unblocks the old e2e concern but leaves feature 039 in `in_progress`; the precise first blocker for full-delivery progression is the failing integration command.

### Scope: scope-01-foundation-schema - 2026-04-29 06:30 UTC - Validation Reassessment After BUG-022 Certification

#### Summary

`bubbles.validate` reassessed feature 039 after BUG-022-001 was certified. The prior runtime blocker is cleared in this validation run: build, format, lint, check, unit, integration, e2e, and stress commands all exited 0. Feature 039 is still not certifiable because the state-transition guard exits 1 on planning/control-plane metadata, scope completion, phase-record, and G036/G040/G068 gates.

#### Outcome Contract Verification (G070)

| Field | Declared | Evidence | Status |
|-------|----------|----------|--------|
| Intent | Ranked, sourced, personalized recommendations with visible why/source detail | Runtime evidence currently proves Scope 1 foundation behavior only: migration, empty provider registry, no-provider persistence, config validation, and operator status. | Not satisfied feature-wide |
| Success Signal | Telegram ramen recommendations, proactive price-drop watch, and inspectable provenance | No delivered recommendation, watch alert, or full provenance inspection evidence exists yet. | Not satisfied |
| Hard Constraints | Multi-provider aggregation, graph reranking, citations, suppression, watch controls, precision policy, outage handling, consent, disclosure, safety, and read-only provider behavior | Scope 1 preserves empty-provider/no-fabricated-result and fail-loud config behavior; later constraints remain planned in Scopes 2-6. | Partially preserved, not complete |
| Failure Condition | Output violates graph/source/watch/precision/consent/safety/repeat/cost constraints | No real recommendation output is delivered yet, so the failure condition is not triggered by shipped behavior. | Not triggered, not complete |

#### Completion Statement (MANDATORY)

No scope was certified. `certification.completedScopes` and `certification.certifiedCompletedPhases` remain empty. The correct progression result is `route_required` to `bubbles.plan` because the runtime blocker is gone and the first actionable gate failures are plan-owned metadata and scope-artifact corrections.

#### Test Evidence (ALL TYPES REQUIRED)

| Check | Command | Exit Code | Key Evidence |
|-------|---------|-----------|--------------|
| Check | `timeout 180 ./smackerel.sh check` | 0 | Config in sync; env drift guard OK; scenario-lint OK |
| Build | `timeout 1200 ./smackerel.sh build` | 0 | `smackerel-core` and `smackerel-ml` built |
| Format | `timeout 600 ./smackerel.sh format --check` | 0 | `42 files already formatted` |
| Lint | `timeout 900 ./smackerel.sh lint` | 0 | `All checks passed!`; web validation passed |
| Unit | `timeout 1200 ./smackerel.sh test unit` | 0 | Go unit packages passed; Python unit: `352 passed, 2 warnings in 22.46s` |
| Integration | `timeout 1800 ./smackerel.sh test integration` | 0 | NATS tests passed; `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` passed; `TestRecommendationMigration_UpDownRoundTripIsIdempotent` passed |
| E2E | `timeout 3600 ./smackerel.sh test e2e` | 0 | Shell E2E: 34 total, 34 passed; `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passed; Go E2E packages passed |
| Stress | `timeout 900 ./smackerel.sh test stress` | 0 | Health stress 25/25 passed; search stress 1100 artifacts, 10 queries, 0 failures |

Raw command signals from the current run:

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
smackerel-core  Built
smackerel-ml    Built
42 files already formatted
All checks passed!
352 passed, 2 warnings in 22.46s
--- PASS: TestNATS_PublishSubscribe_Artifacts
--- PASS: TestNATS_PublishSubscribe_Domain
--- PASS: TestNATS_Chaos_MaxDeliverExhaustion
--- PASS: TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace
--- PASS: TestRecommendationMigration_UpDownRoundTripIsIdempotent
Shell E2E Test Results: Total: 34, Passed: 34, Failed: 0
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault
Search stress test passed: all queries completed under 3000ms with 1100 artifacts
```

#### Governance Evidence

| Gate | Command | Exit Code | Result |
|------|---------|-----------|--------|
| Artifact lint | `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine` | 0 | Passed |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/039-recommendations-engine` | 0 | Passed |
| Artifact freshness | `timeout 600 bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/039-recommendations-engine` | 0 | Passed |
| Implementation reality | `timeout 600 bash .github/bubbles/scripts/implementation-reality-scan.sh specs/039-recommendations-engine --verbose` | 0 | Passed with one warning: scopes should reference implementation files directly |
| State-transition guard | `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/039-recommendations-engine` | 1 | Blocked: 32 failures, 3 warnings |

State-transition guard blocking output from the current run:

```text
scenario-manifest.json is missing requiredTestType entries for one or more scenarios (Gate G057)
scenario-manifest.json is missing linkedTests entries (Gate G057)
DoD items total: 77 (checked: 5, unchecked: 72)
Resolved scopes: total=6, Done=0, In Progress=1, Not Started=5, Blocked=0
Required phase 'implement' NOT in execution/certification phase records
Required phase 'test' NOT in execution/certification phase records
Required phase 'regression' NOT in execution/certification phase records
Required phase 'simplify' NOT in execution/certification phase records
Required phase 'stabilize' NOT in execution/certification phase records
Required phase 'security' NOT in execution/certification phase records
Required phase 'docs' NOT in execution/certification phase records
Required phase 'validate' NOT in execution/certification phase records
Required phase 'audit' NOT in execution/certification phase records
Required phase 'chaos' NOT in execution/certification phase records
6 regression E2E planning requirement(s) missing
2 consumer-trace planning requirement(s) missing
2 change-boundary containment requirement(s) missing
DoD-Gherkin content fidelity gap in Scope 1: SCN-039-002 Provider registry is empty by default
TRANSITION BLOCKED: 32 failure(s), 3 warning(s)
```

#### Scenario Contract Evidence

- `SCN-039-001`: current integration run passes `TestRecommendationMigration_UpDownRoundTripIsIdempotent`.
- `SCN-039-002`: current integration run passes `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace`, and current e2e run passes `TestOperatorStatus_RecommendationProvidersEmptyByDefault`.
- `SCN-039-003`: current unit run passes the recommendation config validation package.

#### Open Findings

1. `scenario-manifest.json` has 30 scenario entries and zero `requiredTestType` / `linkedTests` fields. Owner: `bubbles.plan`.
2. `scopes.md` has 72 unchecked DoD items: Scope 1 has 5 unchecked items, and Scopes 2-6 are unimplemented and unchecked. Owner: `bubbles.plan` for metadata corrections; execution owners must not self-certify.
3. Scope statuses remain 0 Done, 1 In Progress, 5 Not Started. Owner: `bubbles.workflow` for sequencing after plan repair.
4. Full-delivery phase records are missing for implement, test, regression, simplify, stabilize, security, docs, validate, audit, and chaos. Owner: `bubbles.workflow`.
5. State-transition guard still reports G036/G040 wording in scope/report artifacts. Owner: `bubbles.plan` for plan-owned scope wording; evidence owners only for evidence wording they authored.
6. State-transition guard reports missing scenario-specific regression E2E DoD wording in all six scopes, consumer-trace planning gaps for Scope 1, change-boundary DoD/format gaps, and G068 for SCN-039-002. Owner: `bubbles.plan`.

#### Owner Packet

Route to `bubbles.plan` before implementation continues. Required plan-owned repairs:

- Add `requiredTestType` and `linkedTests` to all 30 entries in `scenario-manifest.json`, preserving the existing `SCN-039-*` IDs and live-test expectations.
- Repair scope DoD wording so scenario-specific regression E2E coverage is mechanically recognized in every scope.
- Repair Scope 1 DoD fidelity for `SCN-039-002` so both `/status` zero-provider rendering and API `no_providers` persistence remain explicit.
- Add or reformat consumer-impact and change-boundary DoD items so the guard recognizes the intended planning controls.
- Remove or replace G036/G040-triggering wording in plan-owned scope text while preserving SST truth and Scope 1 behavior.

#### Validation Summary

Runtime commands exited 0. Certification remains blocked by state-transition guard failures. Do not promote Scope 1 or feature 039.

### Plan Repair: control-plane metadata — 2026-04-29 06:43

#### Summary

`bubbles.plan` repaired plan-owned state-transition blockers only. No source code, tests, runtime config, certification-owned state, or scope status fields were changed. Scope 1 remains `In Progress`; scopes 02-06 remain `Not Started` and still require implementation/provenance through the full-delivery workflow.

#### Files Changed

- `specs/039-recommendations-engine/scenario-manifest.json`
- `specs/039-recommendations-engine/scopes.md`
- `specs/039-recommendations-engine/report.md`
- `specs/039-recommendations-engine/state.json`

#### Metadata Repairs

- Added `requiredTestType`, `regressionRequired`, and `linkedTests` fields to all 30 scenario-manifest entries.
- Linked only existing Scope 1 executable tests in `linkedTests`; not-started scopes retain empty `linkedTests` arrays until their implementation scopes create the files.
- Added exact scenario-specific E2E regression DoD wording across all six scopes.
- Added Scope 1 consumer-impact sweep planning and the guard-recognized DoD item for zero stale first-party references.
- Added guard-recognized Change Boundary containment DoD wording and allowed/excluded surface labels.
- Added an explicit SCN-039-002 DoD item covering both `/status` zero-provider rendering and API `no_providers` persistence.
- Replaced G036/G040-triggering wording while preserving SST truth around empty-string secret fields.

#### Governance Evidence

| Gate | Command | Exit Code | Result |
|------|---------|-----------|--------|
| Artifact lint | `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine` | 0 | Passed |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/039-recommendations-engine` | 0 | Passed |
| State-transition guard | `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/039-recommendations-engine` | 1 | Blocked by remaining completion/execution gates |

State-transition guard plan-owned checks now pass:

```text
✅ PASS: scenario-manifest.json records required live test types
✅ PASS: scenario-manifest.json records linkedTests
✅ PASS: scenario-manifest.json marks 30 regression-protected scenario contract(s)
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 1: scope-01-foundation-schema
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 2: scope-02-reactive-place-recommendation
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 3: scope-03-feedback-suppression-why
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 4: scope-04-watches-and-scheduler
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 5: scope-05-policy-quality-and-trip-dossier
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: Scope 6: scope-06-observability-stress-and-cutover
✅ PASS: Scope includes Consumer Impact Sweep section: Scope 1: scope-01-foundation-schema
✅ PASS: Scope DoD includes consumer impact sweep completion item: Scope 1: scope-01-foundation-schema
✅ PASS: Scope lists affected consumer surfaces for rename/removal work: Scope 1: scope-01-foundation-schema
✅ PASS: Scope includes Change Boundary section: scopes.md
✅ PASS: Scope DoD includes change-boundary containment item: scopes.md
✅ PASS: Scope enumerates allowed and excluded surfaces for the change boundary: scopes.md
✅ PASS: Zero deferral language found in scope and report artifacts (Gate G040)
✅ PASS: All 30 Gherkin scenarios have faithful DoD items (Gate G068)
```

Remaining state-transition blockers are not plan-owned metadata repairs:

```text
🔴 BLOCK: Resolved scope artifacts have 80 UNCHECKED DoD items — ALL must be [x] for 'done'
🔴 BLOCK: Resolved scope artifacts have 5 scope(s) still marked 'Not Started' — ALL scopes must be Done
🔴 BLOCK: Required phase 'implement' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'test' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'docs' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'validate' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'audit' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'chaos' NOT in execution/certification phase records (Gate G022 violation)
🔴 TRANSITION BLOCKED: 13 failure(s), 4 warning(s)
```

#### Completion Statement

Plan-owned state-transition metadata repairs are complete. Feature 039 is not complete or certifiable. Next owner is `bubbles.workflow` to route execution through implementation, test, regression, simplify, stabilize, security, docs, validate, audit, and chaos phases without promoting scopes ahead of evidence.

### Scope: scope-01-foundation-schema - 2026-04-29 07:20 UTC - Implement Reconciliation

#### Summary

`bubbles.implement` refreshed Scope 1 against the current runtime after the prior integration, e2e, and plan metadata blockers were cleared. No product source code was changed in this pass. Scope 1 evidence now covers the recommendation migration round trip, empty-provider API persistence, `/status` zero-provider UI, fail-loud recommendation config, SST generation, check/lint/format, unit, integration, and broad e2e commands.

#### Completion Statement

Scope 1 is marked `Done` in [scopes.md](scopes.md). This pass did not write validation-owned certification fields; `certification.completedScopes` and `certification.certifiedCompletedPhases` remain owned by `bubbles.validate`.

#### Files Changed In This Pass

- `specs/039-recommendations-engine/scopes.md`
- `specs/039-recommendations-engine/report.md`

Runtime implementation files were already present before this implement reconciliation. The current Scope 1-owned file set remains within the planned DB, recommendation, config, API, web, test, and SST surfaces.

#### Test Evidence

**Phase:** implement  
**Command:** `./smackerel.sh config generate`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Generated /home/philipk/smackerel/config/generated/dev.env
Generated /home/philipk/smackerel/config/generated/nats.conf
```

**Phase:** implement  
**Command:** `./smackerel.sh --env test config generate`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Generated /home/philipk/smackerel/config/generated/test.env
Generated /home/philipk/smackerel/config/generated/nats.conf
```

**Phase:** implement  
**Command:** `./smackerel.sh check`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** implement  
**Command:** `./smackerel.sh format --check`  
**Exit Code:** 0  
**Claim Source:** executed

```text
42 files already formatted
```

**Phase:** implement  
**Command:** `./smackerel.sh lint`  
**Exit Code:** 0  
**Claim Source:** executed

```text
All checks passed!
=== Validating web manifests ===
	OK: web/pwa/manifest.json
	OK: PWA manifest has required fields
	OK: web/extension/manifest.json
	OK: Chrome extension manifest has required fields (MV3)
	OK: web/extension/manifest.firefox.json
	OK: Firefox extension manifest has required fields (MV2 + gecko)
Web validation passed
```

**Phase:** implement  
**Command:** `./smackerel.sh test unit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/config  0.132s
?       github.com/smackerel/smackerel/internal/recommendation  [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/provider (cached)
?       github.com/smackerel/smackerel/internal/recommendation/store    [no test files]
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
352 passed, 2 warnings in 22.30s
```

**Phase:** implement  
**Command:** `./smackerel.sh test integration`  
**Exit Code:** 0  
**Claim Source:** executed

```text
=== RUN   TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace
--- PASS: TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace (0.09s)
=== RUN   TestRecommendationMigration_UpDownRoundTripIsIdempotent
--- PASS: TestRecommendationMigration_UpDownRoundTripIsIdempotent (0.78s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        21.887s
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  3.705s
PASS
ok      github.com/smackerel/smackerel/tests/integration/drive  2.009s
```

**Phase:** implement  
**Command:** `./smackerel.sh test e2e --go-run 'TestOperatorStatus_RecommendationProvidersEmptyByDefault$'`  
**Exit Code:** 0  
**Claim Source:** executed

```text
go-e2e: applying -run selector: TestOperatorStatus_RecommendationProvidersEmptyByDefault$
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.11s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.123s
PASS: go-e2e
```

**Phase:** implement  
**Command:** `./smackerel.sh test e2e`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Shell E2E Test Results
PASS: test_compose_start.sh
PASS: test_persistence.sh
PASS: test_postgres_readiness_gate.sh
PASS: test_config_fail.sh
PASS: test_capture_pipeline.sh
PASS: test_web_settings.sh
PASS: test_browser_sync.sh
Total:  34
Passed: 34
Failed: 0
=== RUN   TestOperatorStatus_RecommendationProvidersEmptyByDefault
--- PASS: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.05s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        113.442s
PASS: go-e2e
```

#### Consumer And Boundary Evidence

**Phase:** implement  
**Command:** `grep -RInE "/api/recommendations/requests|/status|recommendation provider|no_providers" internal cmd tests config/smackerel.yaml scripts/commands/config.sh`  
**Exit Code:** 0  
**Claim Source:** interpreted  
**Interpretation:** The sweep found active route mounts, handlers, templates, and tests for the additive `/status` provider block and `/api/recommendations/requests` no-provider path. No stale first-party route reference was identified in the inspected first-party surfaces.

```text
internal/api/router.go:166:                     r.Get("/status", deps.WebHandler.StatusPage)
internal/api/recommendations.go:64:// CreateRequest handles POST /api/recommendations/requests. Scope 1 persists
internal/api/recommendations.go:100:            Status:                     "no_providers",
internal/web/handler.go:419:// StatusPage handles GET /status.
internal/web/templates.go:184:    <p class="meta">0 recommendation providers configured</p>
internal/web/handler_test.go:143:       if !containsString(rendered, "0 recommendation providers configured") {
tests/e2e/operator_status_test.go:15:   resp, err := apiGet(cfg, "/status")
tests/e2e/operator_status_test.go:30:   if !strings.Contains(html, "0 recommendation providers configured") {
tests/integration/recommendation_providers_test.go:31:  req := httptest.NewRequest(http.MethodPost, "/api/recommendations/requests", bytes.NewBufferString(...))
tests/integration/recommendation_providers_test.go:46:  if resp.Status != "no_providers" {
```

**Phase:** implement  
**Command:** `git status --short -- specs/039-recommendations-engine/scopes.md specs/039-recommendations-engine/report.md specs/039-recommendations-engine/state.json specs/039-recommendations-engine/scenario-manifest.json internal/db/migrations/022_recommendations.sql internal/recommendation internal/config/config.go internal/config/recommendations.go internal/config/recommendations_validate_test.go internal/config/validate_test.go internal/api/recommendations.go internal/api/recommendations_test.go internal/api/router.go tests/integration/recommendation_providers_test.go tests/integration/recommendations_migration_test.go tests/e2e/operator_status_test.go internal/web/handler.go internal/web/templates.go cmd/core/services.go scripts/commands/config.sh config/smackerel.yaml`  
**Exit Code:** 0  
**Claim Source:** executed

```text
 M cmd/core/services.go
 M config/smackerel.yaml
 M internal/api/router.go
 M internal/config/config.go
 M internal/config/validate_test.go
 M internal/web/handler.go
 M internal/web/templates.go
 M scripts/commands/config.sh
?? internal/api/recommendations.go
?? internal/api/recommendations_test.go
?? internal/config/recommendations.go
?? internal/config/recommendations_validate_test.go
?? internal/db/migrations/022_recommendations.sql
?? internal/recommendation/
?? tests/e2e/operator_status_test.go
?? tests/integration/recommendation_providers_test.go
?? tests/integration/recommendations_migration_test.go
```

**Phase:** implement  
**Command:** `git status --short -- internal/connector internal/digest internal/intelligence internal/scheduler internal/telegram`  
**Exit Code:** 0  
**Claim Source:** interpreted  
**Interpretation:** The shared worktree contains excluded-family edits that are unrelated to Scope 1 and were not modified by this implement reconciliation. Scope 1-owned file changes remain inside the planned allowed surfaces listed above.

```text
 M internal/digest/generator.go
 M internal/intelligence/people.go
 M internal/scheduler/jobs.go
 M internal/scheduler/jobs_test.go
 M internal/telegram/bot.go
 M internal/telegram/bot_test.go
 M internal/telegram/forward.go
?? internal/connector/browser/sqlite_driver.go
?? internal/digest/weather.go
?? internal/digest/weather_test.go
?? internal/intelligence/people_forecast.go
?? internal/intelligence/people_forecast_test.go
?? internal/telegram/forward_single_flush_test.go
```

#### Scenario Contract Evidence

- `SCN-039-001`: `TestRecommendationMigration_UpDownRoundTripIsIdempotent` passed in `./smackerel.sh test integration`.
- `SCN-039-002`: `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` passed in integration; `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passed in focused and broad e2e.
- `SCN-039-003`: `internal/config/recommendations_validate_test.go` passed as part of `./smackerel.sh test unit`.

#### Tier 2 Implement Checks

| Check | Result | Evidence |
|-------|--------|----------|
| I1 Scope DoD evidence updated inline | Pass | Scope 1 DoD entries in [scopes.md](scopes.md) now carry implement evidence with `Phase` and `Claim Source` tags |
| I2 Required tests pass | Pass | `check`, `lint`, `format --check`, `test unit`, `test integration`, focused e2e, and broad e2e all exited 0 |
| I3 Docs synchronized | Pass | No managed docs changed in this implement pass; Scope 1 behavior is captured in the feature artifacts and tests |
| I4 Scope state coherent | Pass | [scopes.md](scopes.md) marks Scope 1 `Done`; certification fields remain validate-owned |
| I5 No new policy violations | Pass | SST generation/check passed; no generated config was hand-edited; no product source edits were made in this pass |

#### Governance Check Evidence

**Phase:** implement  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine`  
**Exit Code:** 0  
**Claim Source:** executed

```text
Artifact lint PASSED.
```

**Phase:** implement  
**Command:** `timeout 600 bash .github/bubbles/scripts/state-transition-guard.sh specs/039-recommendations-engine`  
**Exit Code:** 1  
**Claim Source:** executed

```text
TRANSITION BLOCKED: 14 failure(s), 4 warning(s)
state.json status MUST NOT be set to 'done'.
```

**Interpretation:** The evidence-format issue for Scope 1 checked DoD items is cleared: Check 9 reports all 13 checked DoD items have evidence blocks. Current transition blockers are feature-wide completion and certification gates: Scopes 2-6 are `Not Started`, 72 non-Scope-1 DoD items are unchecked, `certification.completedScopes` is validate-owned and still empty, and full-delivery specialist phase records are absent. This implement pass records Scope 1 evidence in `scopes.md` and `report.md` without writing validation-owned certification fields or self-certifying the feature.

### scope-02-reactive-place-recommendation

Pending implementation. Evidence to be appended by `bubbles.implement`.

### scope-03-feedback-suppression-why

Pending implementation. Evidence to be appended by `bubbles.implement`.

### scope-04-watches-and-scheduler

Pending implementation. Evidence to be appended by `bubbles.implement`.

### scope-05-policy-quality-and-trip-dossier

Pending implementation. Evidence to be appended by `bubbles.implement`.

### scope-06-observability-stress-and-cutover

Pending implementation. Evidence to be appended by `bubbles.implement`.

## Evidence Block Template

```
### Scope: <scope-name> — <YYYY-MM-DD HH:MM>

#### Summary
<one paragraph>

#### Decision Record
<key decisions, alternatives considered, rationale>

#### Completion Statement (MANDATORY)
<which DoD items are [x] with evidence; which remain [ ] with uncertainty declarations>

#### Code Diff Evidence
<file list + ±LOC summary>

#### Test Evidence (ALL TYPES REQUIRED)
**Phase:** <phase-name>
**Command:** <exact command executed>
**Exit Code:** <actual exit code>
**Claim Source:** <executed | interpreted | not-run>
<raw output, ≥10 lines>

#### Uncertainty Declarations (if any DoD items remain [ ])

#### Scenario Contract Evidence
<scenarioIds covered with linked test ids; reference scenario-manifest.json>

#### Coverage Report

#### Lint/Quality

#### Spot-Check Recommendations

#### Validation Summary
```
