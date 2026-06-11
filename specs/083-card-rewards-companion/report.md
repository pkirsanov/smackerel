# Report: 083 Card Rewards Companion

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

This is a **planning-only** execution (`product-to-planning`, ceiling
`specs_hardened`). It produced the reviewable plan to absorb the standalone
**CCManager** credit-card rewards app into smackerel as a native **card
rewards** feature. NO implementation code, migration, config edit, or
feature-flag bundle edit was made.

Artifacts authored:

| Artifact | Content |
|----------|---------|
| `spec.md` | Problem, Outcome Contract, 7 use cases, 20 functional + 7 non-functional requirements, **Product Principle Alignment** (cites §16.8; confirms §1.6/Principle 10 NOT crossed), **Release Train** (mvp), **Offline/Host** note, 6 owner open questions |
| `design.md` | Current-Truth primitive map (verified paths), architecture diagram, **10-table PostgreSQL schema** (migration 057), source connector, **strict-schema LLM extraction replacing the regex scraper**, multi-source reconciliation + Principle-3 lifecycle, optimizer, **CalDAV delivery reusing `internal/mealplan` CalendarBridge**, rationalized **Web UI IA** (CCManager ~22 screens → 10 smackerel pages), config SST keys (fail-loud), JSON→PG migration design, LLM prompt/schema contract |
| `scopes.md` | **11 dependency-ordered scopes**, Gherkin `SCN-083-A01..K08`, Test Plan tables (unit/integration/e2e-api/e2e-ui), tiered checkbox DoD; data migration and LLM extraction are dedicated scopes with adversarial tests vs the regex failure modes; UI has 2 dedicated e2e-ui scopes |
| `state.json` | v3; `status=specs_hardened`; `workflowMode=product-to-planning`; `releaseTrain=mvp`; `flagsIntroduced=["card_rewards"]`; `planMaturityOnly=true`; honest parent-expanded execution record |
| `uservalidation.md` | Baseline acceptance checklist (planning decisions) |
| `report.md` | This file |

### Execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`product-to-planning` phases (analyze → design → plan → harden) were executed
in **parent-expanded** form by a single orchestrator agent that authored the
planning documents directly. No separate certified specialist sub-agents were
dispatched or claimed. This is recorded in `state.json.executionHistory`. Per
governance, parent-expansion is acceptable here because (a) this is a
planning-only ceiling producing reviewable documents (not code/test
certification), and (b) the execution model is disclosed, not fabricated.

### Post-plan refinement — cross-project tech consistency (2026-06-11)

After initial planning, an owner directive ("tech we're using must be
consistent across projects unless there is a very good reason for deviation")
triggered a consistency audit of the plan against smackerel's real dependency
baseline (`go.mod`) and Constitution C2. One genuine defect was found and
fixed, plus reinforcing clarifications:

| Change | File(s) | Reason |
|--------|---------|--------|
| **LLM model-gateway call moved to the Python ML sidecar** (new route `ml/app/card_categories.py`); Go `internal/cardrewards/extract.go` is now an orchestrator + schema-validator, NOT a direct Go→Ollama client | spec.md (NFR-CR-001, Hard Constraint), design.md (§4 + package layout + Current-Truth rows), scopes.md (Scope 05 plan/refs/DoD/test plan) | Constitution C2 reserves "model gateway work" for the Python sidecar (siblings: `drive_classify.py`, `intelligence.py`). The original §4 implied a Go-side Ollama call — a C2 deviation. |
| **New NFR-CR-008 (no new dependency/language/framework)** + a design **§0 Technology Consistency** matrix | spec.md, design.md | Pins reuse of `pgx`, `chi`+`html/template`, `robfig/cron`, `santhosh-tekuri/jsonschema`, `go-shiori/go-readability`+`net/http`, the ML sidecar, and the `internal/mealplan` CalDAV pattern — all already in `go.mod`/repo. |
| **Scope 05 C2-boundary DoD item** (grep proves no direct Ollama client under `internal/cardrewards/`) + a Python sidecar unit-test row (T-05-06) | scopes.md | Makes the consistency rule mechanically verifiable at delivery. |

Re-ran `bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion` → **exit 0** (only the pre-existing deprecated-`scopeProgress` WARN, untouched by this refinement). Ceiling unchanged (`specs_hardened`).

## Completion Statement

The planning deliverables for feature 083 are complete to the `specs_hardened`
ceiling. All reused smackerel primitives are cited by verified path. The plan
honors smackerel governance: Go-first (C2), PostgreSQL-only, config SST
no-defaults (Gate G028 / `smackerel-no-defaults`), connector-metadata
preservation (Principle 4), one-graph (Principle 5), transparency /
source-attribution (Principle 8), Knowledge-Breathes lifecycle (Principle 3),
and the BLOCKING product-principles enforcement file (§16.8 placement; §1.6 /
Principle 10 boundary not crossed). Status stops at `specs_hardened`; no
implementation occurred. Delivery is a separate, later workflow.

## Test Evidence

Planning-only: there is no implementation to test. The applicable gate is the
Bubbles **artifact lint**. Command, exit code, and raw output below.

### Gate: artifact-lint

Command: `bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion`

Result: exit 0. All required-artifact, DoD-checkbox-syntax, uservalidation
checklist, state.json v3 schema, status-ceiling, report-section, and
anti-fabrication checks pass. One non-blocking warning only: `state.json` uses
the deprecated-but-supported `scopeProgress` field (retained intentionally to
document all 11 scopes + dependencies for the reviewer). Zero failures. Raw
output:

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ uservalidation checklist has checked-by-default entries
✅ All checklist bullet items use checkbox syntax
✅ Detected state.json status: specs_hardened
✅ Detected state.json workflowMode: product-to-planning
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
✅ state.json v3 has recommended field: transitionRequests
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' (non-blocking; retained to document 11 scopes + deps)
✅ state.json planMaturityOnly=true is not claiming delivery-done status
✅ Workflow mode 'product-to-planning' permits current status 'specs_hardened' (ceiling: specs_hardened)
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence
✅ No mode-specific report gates configured for workflowMode 'product-to-planning'
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

---

## Delivery — Scope 01: Config SST & Migration Schema (2026-06-11)

> **Delivery started.** The `full-delivery` workflow began implementing the
> 11 dependency-ordered scopes. This section records **Scope 01** delivery with
> real, in-session terminal evidence. Scopes 02–11 remain **Not Started** (see
> Delivery Status below). Execution model for delivery is **parent-expanded**
> (the `runSubagent`/`agent` tool is unavailable in this runtime); disclosed in
> `state.json.executionHistory`, not fabricated.

### Files created / changed (Scope 01)

| File | Change |
|------|--------|
| `internal/db/migrations/057_card_rewards.sql` | NEW — 10 card-rewards tables (design §2) with CHECK/FK/UNIQUE constraints + indexes; `CREATE … IF NOT EXISTS` (self-idempotent, matches migration 056) |
| `internal/config/cardrewards.go` | NEW — `CardRewardsConfig` + `LoadCardRewardsConfig()` fail-loud loader (disabled→no error; enabled→names each missing/invalid key) |
| `internal/config/cardrewards_test.go` | NEW — unit tests: SCN-083-A01/A03/A04/A07 + confidence/cron/int/calendar-sync permutations |
| `internal/config/config.go` | `CardRewards CardRewardsConfig` field + `LoadCardRewardsConfig()` wired into `Load()` (startup fail-loud when enabled) |
| `config/smackerel.yaml` | NEW `card_rewards:` section (enabled:false dev placeholder, fail-loud-when-enabled) + `connectors.card-rewards` entry |
| `scripts/commands/config.sh` | Emit 13 `CARD_REWARDS_*` env vars (`yaml_get` + `yaml_get_json` for sources/tracked_categories) into the generated env file |
| `tests/integration/card_rewards_migration_test.go` | NEW — `//go:build integration` migration test (SCN-083-A05/A06) reusing the shared `testPool`/`tableExists` harness |

### Evidence — SCN-083-A02 (config generate emits CARD_REWARDS_*)

Command: `./smackerel.sh config generate` then `grep -n CARD_REWARDS config/generated/{dev,test}.env`

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.714384 OK
Generated ~/smackerel/config/generated/dev.env
=== dev.env ===
431:CARD_REWARDS_ENABLED=false
432:CARD_REWARDS_SCRAPE_CRON=0 6 * * *
433:CARD_REWARDS_MONTHLY_RECOMMEND_CRON=0 7 1 * *
434:CARD_REWARDS_CALENDAR_SYNC=false
435:CARD_REWARDS_CALENDAR_UID_PREFIX=smackerel-cardrec
436:CARD_REWARDS_FETCH_TIMEOUT_SECONDS=20
437:CARD_REWARDS_EXTRACTION_MODEL=
438:CARD_REWARDS_EXTRACTION_ENDPOINT=
439:CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD=0.0
440:CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD=0
441:CARD_REWARDS_SOURCES=[]
442:CARD_REWARDS_TRACKED_CATEGORIES=[]
=== test.env ===
431:CARD_REWARDS_ENABLED=false
432:CARD_REWARDS_SCRAPE_CRON=0 6 * * *
   … (identical 13 vars in test.env, lines 431–442) …
```

### Evidence — SCN-083-A01/A03/A04/A07 (config unit tests)

Command: `./smackerel.sh test unit --go --go-run CardRewards --verbose`

```text
[go-unit] applying -run selector: CardRewards
[go-unit] starting go test ./...
=== RUN   TestLoadCardRewardsConfig_PopulatesWhenEnabled
--- PASS: TestLoadCardRewardsConfig_PopulatesWhenEnabled (0.00s)
=== RUN   TestLoadCardRewardsConfig_DisabledParsesWithoutRequiringConfig
--- PASS: TestLoadCardRewardsConfig_DisabledParsesWithoutRequiringConfig (0.00s)
=== RUN   TestLoadCardRewardsConfig_UnsetEnabledTreatedAsDisabled
--- PASS: TestLoadCardRewardsConfig_UnsetEnabledTreatedAsDisabled (0.00s)
=== RUN   TestLoadCardRewardsConfig_FailLoudOnMissingRequired
    --- PASS: …/CARD_REWARDS_SCRAPE_CRON (0.00s)
    --- PASS: …/CARD_REWARDS_MONTHLY_RECOMMEND_CRON (0.00s)
    --- PASS: …/CARD_REWARDS_FETCH_TIMEOUT_SECONDS (0.00s)
    --- PASS: …/CARD_REWARDS_EXTRACTION_MODEL (0.00s)
    --- PASS: …/CARD_REWARDS_EXTRACTION_ENDPOINT (0.00s)
    --- PASS: …/CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD (0.00s)
    --- PASS: …/CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD (0.00s)
--- PASS: TestLoadCardRewardsConfig_FailLoudOnMissingRequired (0.00s)
=== RUN   TestLoadCardRewardsConfig_EmptySourcesRejected
    --- PASS: …/empty_array (0.00s)
    --- PASS: …/empty_string (0.00s)
    --- PASS: …/not_json (0.00s)
    --- PASS: …/missing_url (0.00s)
--- PASS: TestLoadCardRewardsConfig_EmptySourcesRejected (0.00s)
--- PASS: TestLoadCardRewardsConfig_EmptyTrackedCategoriesRejected (0.00s)
--- PASS: TestLoadCardRewardsConfig_RejectsBadConfidence (0.00s)
--- PASS: TestLoadCardRewardsConfig_RejectsBadCron (0.00s)
--- PASS: TestLoadCardRewardsConfig_RejectsNonPositiveInts (0.00s)
--- PASS: TestLoadCardRewardsConfig_CalendarSyncRequiresUIDPrefix (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.021s
```

### Evidence — SCN-083-A05/A06 (migration 057 on live disposable Postgres)

Command: `./smackerel.sh test integration --go-run CardRewardsMigration`

```text
 smackerel-core  Built
 smackerel-ml  Built
Preparing disposable test stack...
 Container smackerel-test-postgres-1  Healthy
 Container smackerel-test-smackerel-core-1  Healthy
go-integration: applying -run selector: CardRewardsMigration
=== RUN   TestCardRewardsMigration_AppliesCleanly
--- PASS: TestCardRewardsMigration_AppliesCleanly (0.19s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.338s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-postgres-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
INTEGRATION_EXIT=0
```

The test drops all 10 tables, applies `057_card_rewards.sql` from scratch
(asserts the 10 tables exist), asserts the `rotating_categories_lifecycle_check`
CHECK exists AND actually rejects an out-of-range `lifecycle_state='bogus'`
insert, asserts the `idx_rotating_card_period` UNIQUE index, asserts
`needs_verification` is `NOT NULL DEFAULT false`, asserts the summary indexes
(`idx_user_cards_active`, `idx_observations_card_period`,
`idx_recommendations_period`, `idx_runs_type_time`), and re-applies the
migration to prove `CREATE … IF NOT EXISTS` idempotency.

### Evidence — Build Quality Gate (Scope 01)

Commands: `./smackerel.sh format --check`, `./smackerel.sh check`, `./smackerel.sh lint`

```text
63 files already formatted
FORMAT_EXIT=0
config-validate: ~/smackerel/config/generated/dev.env.tmp.945780 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0
All checks passed!
=== Validating web manifests ===  (PWA + Chrome MV3 + Firefox MV2)  OK
=== Validating JS syntax ===  OK
=== Checking extension version consistency ===  OK
Web validation passed
LINT_EXIT=0
```

Docker images `smackerel-test-smackerel-core` and `smackerel-test-smackerel-ml`
built successfully during the integration run (build evidence above), and
`go test ./...` compiled every Go package during the unit run — together
proving the build is clean. `smackerel-no-defaults`: no `${VAR:-default}` /
`${VAR-default}` runtime fallback was introduced (the only matches are
documentation comments quoting the forbidden form); the config.sh additions use
the established `… 2>/dev/null) || VAR=""` generator pattern, and the Go loader
enforces fail-loud-when-enabled. artifact-lint: exit 0 (above).

## Delivery — Scope 02: Card Domain Store, Types & CRUD API (2026-06-11)

Delivered the card-rewards domain layer (`internal/cardrewards/`), the REST CRUD
API behind the existing bearer-auth group, and card-name resolution (replacing
CCManager's `card_resolver.py`). All eight Scope-02 scenarios (SCN-083-B01..B08)
validated with real in-session output: resolver unit tests, store integration
tests on a live disposable Postgres, and an e2e CRUD round-trip against the live
stack.

**Design decisions disclosed:**

- **JSON = snake_case.** The prompt mentioned "camelCase JSON", but the entire
  smackerel API surface (`connector.RawArtifact`, `mealplan`, `recommendations`,
  every handler DTO) uses snake_case, and `design.md`'s own extraction example
  (`card_id`, `period_label`) is snake_case. Honoring the HARD CONSISTENCY
  constraint ("reuse smackerel's stack verbatim") and avoiding a lone
  inconsistent island, snake_case was used. camelCase is the WanderAide/GuestHost
  convention, not smackerel's.
- **CRUD handler mounts when the Postgres pool is present**, NOT gated on
  `card_rewards.enabled`. Wallet/offers/selections/bonuses CRUD is data
  management and does not need the ingestion config (Ollama model/endpoint, cron,
  sources). This matches smackerel's `if deps.X != nil` mount idiom
  (knowledge/annotations/recommendations) and keeps the ingestion pipeline
  (connector/extraction/scheduler — scopes 04/05/09) separately gated on
  `card_rewards.enabled`. Dev default stays `enabled: false`; no config edit was
  made; the e2e runs cleanly with the feature's dev default unchanged.

**Two real bugs were caught by the live integration tests and fixed (not
worked around):**

1. `aliases TEXT[] NOT NULL` rejected a nil `[]string` (pgx encodes nil slice as
   SQL `NULL`, which violates the constraint when the column is named explicitly
   in the INSERT). Fixed by normalizing nil → `[]string{}` (`nonNilStrings`) in
   the catalog + category-alias inserts.
2. `user_cards.id` / offers / selections / bonuses are `UUID` columns; the store
   integration test initially used `"uc-"+prefix` string ids → `invalid input
   syntax for type uuid`. Fixed by generating real UUIDs (`uuid.NewString()`) for
   the UUID-typed PK columns in the test (catalog id stays a TEXT slug).

### Files created / changed (Scope 02)

| File | Change |
|------|--------|
| `internal/cardrewards/types.go` | NEW — domain types (CatalogCard, UserCard, Offer, Selection, SignupBonus, CategoryAlias) + card-type/rate-type/bonus-type enums & validators; snake_case JSON |
| `internal/cardrewards/resolve.go` | NEW — deterministic, dependency-free card-name resolver (exact alias/name → substring → token Jaccard), ranked candidates with MatchType; replaces `card_resolver.py` |
| `internal/cardrewards/store.go` | NEW — pgx CRUD for catalog, user_cards, offers, selections, signup_bonuses, category_aliases; transactional custom-card insert; `nonNilStrings` TEXT[] guard |
| `internal/cardrewards/service.go` | NEW — business logic, UUID generation, validation, sentinel errors (ErrValidation/ErrCatalogNotFound/ErrUserCardNotFound) |
| `internal/cardrewards/resolve_test.go` | NEW — unit tests (SCN-083-B02/B03 + boundary/adversarial) |
| `internal/cardrewards/store_test.go` | NEW — integration tests on live PG (SCN-083-B01/B04/B05/B06/B07), `//go:build integration` |
| `internal/api/cardrewards.go` | NEW — REST handler (relative-prefix routes, sentinel-error→HTTP mapping) mounted in the bearer-auth group |
| `internal/api/health.go` | EDIT — `Dependencies.CardRewardsHandler` field |
| `internal/api/router.go` | EDIT — mount card-rewards routes (`if deps.CardRewardsHandler != nil`) |
| `cmd/core/wiring.go` | EDIT — `wireCardRewardsHandler` (constructs store+service+handler when pg pool present); `cardrewards` import |
| `cmd/core/main.go` | EDIT — call `wireCardRewardsHandler(svc, deps)` before `api.NewRouter` (construction-order rule) |
| `scripts/runtime/go-integration.sh` | EDIT — add `./internal/cardrewards/...` to the integration package list |
| `tests/e2e/cardrewards_api_test.go` | NEW — e2e CRUD round-trip (SCN-083-B08), `//go:build e2e` |

### Evidence — SCN-083-B02/B03 (resolver unit tests)

Command: `./smackerel.sh test unit --go --go-run 'TestResolveCard' --verbose`
(compiled every Go package via `go test ./...`; all packages `ok`, then:)

```text
=== RUN   TestResolveCard_TopCandidate_B02
--- PASS: TestResolveCard_TopCandidate_B02 (0.00s)
=== RUN   TestResolveCard_NormalizationRobust_B02
--- PASS: TestResolveCard_NormalizationRobust_B02 (0.00s)
=== RUN   TestResolveCard_Ambiguous_B03
--- PASS: TestResolveCard_Ambiguous_B03 (0.00s)
=== RUN   TestResolveCard_SharedExactAlias_B03
--- PASS: TestResolveCard_SharedExactAlias_B03 (0.00s)
=== RUN   TestResolveCard_EmptyInput
--- PASS: TestResolveCard_EmptyInput (0.00s)
=== RUN   TestResolveCard_UnrelatedInputDropped
--- PASS: TestResolveCard_UnrelatedInputDropped (0.00s)
=== RUN   TestResolveCard_RankedAndDeduped
--- PASS: TestResolveCard_RankedAndDeduped (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.014s
```

### Evidence — SCN-083-B01/B04/B05/B06/B07 (store integration on live disposable Postgres)

Command: `./smackerel.sh test integration --go-run 'CardRewardsStore'` (live
test stack up + healthy; go-integration container with `DATABASE_URL` →
`postgres:…/smackerel`; `-tags integration`).

```text
go-integration: applying -run selector: CardRewardsStore
=== RUN   TestCardRewardsStore_CreateReadUserCard_B01
--- PASS: TestCardRewardsStore_CreateReadUserCard_B01 (0.04s)
=== RUN   TestCardRewardsStore_CreateCustomCard_B04
--- PASS: TestCardRewardsStore_CreateCustomCard_B04 (0.04s)
=== RUN   TestCardRewardsStore_SharedLimitOffer_B05
--- PASS: TestCardRewardsStore_SharedLimitOffer_B05 (0.04s)
=== RUN   TestCardRewardsStore_TieredSelection_B06
--- PASS: TestCardRewardsStore_TieredSelection_B06 (0.04s)
=== RUN   TestCardRewardsStore_CascadeDelete_B07
--- PASS: TestCardRewardsStore_CascadeDelete_B07 (0.07s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.240s
PASS: go-integration
```

(The first two integration runs FAILED loudly and surfaced the two real bugs
documented above — `null value in column "aliases" … violates not-null
constraint (SQLSTATE 23502)` and `invalid input syntax for type uuid … (SQLSTATE
22P02)` — which were fixed in source; the run above is post-fix. The tests are
genuine: they fail when the schema/store is wrong.)

### Evidence — SCN-083-B08 (e2e CRUD round-trip on the live stack)

Command: `./smackerel.sh test e2e --go-run 'CardRewardsAPICRUDRoundTrip'`
(fresh e2e core image built with the new `/api/cards` handler; live stack
healthy; bearer auth). The test POSTs a custom card → 201, GETs it → 200, PUTs
an edit (nickname/active) → 200, DELETEs → 204, then GETs again → 404
`CARD_NOT_FOUND`.

```text
go-e2e: applying -run selector: CardRewardsAPICRUDRoundTrip
=== RUN   TestCardRewardsAPICRUDRoundTrip_B08
--- PASS: TestCardRewardsAPICRUDRoundTrip_B08 (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.161s
```

### Evidence — Build Quality Gate (Scope 02)

Commands: `./smackerel.sh format --check`, `./smackerel.sh check`,
`./smackerel.sh lint` (after `./smackerel.sh format` aligned the two new const
blocks).

```text
=== CHECK ===
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
=== LINT ===
All checks passed!
=== Validating web manifests ===  (PWA + Chrome MV3 + Firefox MV2)  OK
=== Validating JS syntax ===  OK
=== Checking extension version consistency ===  OK
Web validation passed
LINT_EXIT=0
=== RE-CHECK (format) ===
63 files already formatted
FORMAT_CHECK_EXIT=0
```

`go test ./...` (unit run) compiled every Go package, and the integration + e2e
runs compiled with `-tags integration` and `-tags e2e` and rebuilt the core
image — together proving the build is clean across all tag sets. No
`${VAR:-default}` runtime fallback was introduced (`smackerel-no-defaults`); the
handler mounts on pool presence with no config edit.

### Code Diff Evidence (Scope 02)

Real `git status --short` + `git diff --stat` for the Scope 02 surface
(read-only git; autoCommit OFF — nothing committed this turn):

```text
 M cmd/core/main.go
 M cmd/core/wiring.go
 M internal/api/health.go
 M internal/api/router.go
 M scripts/runtime/go-integration.sh
?? internal/api/cardrewards.go
?? internal/cardrewards/
?? tests/e2e/cardrewards_api_test.go
---DIFFSTAT---
 cmd/core/main.go                  |  6 ++++++
 cmd/core/wiring.go                | 19 +++++++++++++++++++
 internal/api/health.go            |  5 +++++
 internal/api/router.go            |  9 +++++++++
 scripts/runtime/go-integration.sh |  2 +-
 5 files changed, 40 insertions(+), 1 deletion(-)
```

The new package `internal/cardrewards/` (types.go, resolve.go, store.go,
service.go + resolve_test.go, store_test.go), `internal/api/cardrewards.go`, and
`tests/e2e/cardrewards_api_test.go` are untracked new files; the 5 wiring/mount
files are the small modified deltas shown above (handler field + router mount +
wiring constructor + main call + integration-runner package add).

### Spec-level gate status — state-transition-guard (honest)

`bash .github/bubbles/scripts/state-transition-guard.sh
specs/083-card-rewards-companion` → **TRANSITION BLOCKED: 92 failure(s), 3
warning(s)**. This is the SPEC's `done`-promotion guard and it correctly blocks,
because the spec is mid-delivery (2 of 11 scopes Done) and was planned at the
`specs_hardened` ceiling. The blocks are spec-level `done`-gates, NOT defects in
Scope 02's delivered code/tests:

- 9 scopes still `Not Started`; 53 unchecked DoD items (scopes 03–11).
- `scenario-manifest.json` tracks 0 of 78 scenarios; requiredTestType /
  linkedTests / evidenceRefs unpopulated (G057) — a planning control-plane
  artifact to be filled as scopes land.
- 11 specialist pipeline phases (implement/test/regression/simplify/gaps/
  stabilize/security/validate/audit/chaos/docs) not yet in spec-level
  certification records (G022); phase-claim provenance shape (G022 ext).
- 7 e2e-ui Test-Plan files (scopes 10/11) do not exist yet (Check 8).
- 33 regression-E2E planning rows + change-boundary section (Checks 8A/8D) and
  17 DoD-Gherkin fidelity refinements (G068) are planning-template requirements
  spanning ALL scopes (including the already-Done Scope 01) — a `bubbles.plan` /
  `bubbles.harden` pass, not a Scope-02 code defect.
- `### Code Diff Evidence` (G053) — added above this turn.
- G040 deferral-language hits are false positives on legitimate adversarial
  scenario wording ("NOT overwritten with stale or placeholder data", Scope 05
  SCN-083-E03) and the Scope 01 "dev placeholder" config note — correct spec
  content, not deferred work.

Top-level `status` is intentionally NOT promoted to `done` (per the run
contract — 9 scopes remain). The per-scope delivery gates for Scope 02
(build/check/lint/format + artifact-lint, all GREEN; 8/8 scenarios validated on
real infra) are met; the guard's `done`-gate blocks are the expected,
honest signal that the SPEC is not yet complete.

### Delivery Status (honest)

| Scope | Status | Evidence |
|-------|--------|----------|
| 01 Config SST & Migration Schema | **Done** | "Delivery — Scope 01" section (A01–A07 all validated with real output; Build Quality Gate green) |
| 02 Card Domain Store, Types & CRUD API | **Done** | "Delivery — Scope 02" section (B01–B08 all validated with real output: resolve unit, store integration on live PG, e2e CRUD round-trip on live stack; Build Quality Gate green) |
| 03 Data Migration (CCManager JSON → PG) | **Done** | "Delivery — Scope 03" section (C01–C06 all validated on real infra: 16 transform unit tests + 4 live-PG integration tests for catalog/aliases/row-counts, rotating manual_override, partial-file tolerance + migration-run logged, and second-run idempotency; Build Quality Gate green) |
| 04 Card-Rewards Source Connector | **Done** | "Delivery — Scope 04" section (D01–D06 unit incl. SSRF guard, slow-source isolation, health thresholds, verbatim no-parse; Build Quality Gate green) |
| 05 LLM Category Extraction (sidecar + orchestrator) | In Progress | "Delivery — Scope 05" section (E01–E07 unit + live-PG green incl. adversarial malformed-discard-no-overwrite; **E08 successful-live-Ollama-inference blocked-needs-live-Ollama** — sidecar 503 on the disposable stack) |
| 06 Multi-Source Reconciliation & Lifecycle | **Done** | "Delivery — Scope 06" section (F01–F07: adversarial disagreement + manual-override-never-overwritten unit, live-PG idempotent-upsert/pending-re-enrollment/lifecycle-transitions; Build Quality Gate green) |
| 07 Optimizer & Monthly Recommendations | **Done** | "Delivery — Scope 07" section (G01–G08: optimize unit incl. adversarial expired-rotating-ignored + shared-limit-not-double-counted, live-PG per-category + starred-override-preserved, e2e generate/report; Build Quality Gate green) |
| 08 CalDAV Calendar Delivery | **Done** | "Delivery — Scope 08" section (H01–H06: stable-UID create + adversarial re-sync-updates-same-UID, disabled-sync-keeps-data, delete-cleanup, live-PG sync-run-audited; Build Quality Gate green) |
| 09 Scheduler Jobs & Manual Triggers | **Done** | "Delivery — Scope 09" section (I01–I06: jobs-on-configured-crons + adversarial no-swap unit, live-PG full daily/monthly pipelines audited, manual-reuse trigger="manual", adversarial re-run idempotency; Build Quality Gate green, artifact-lint exit 0) |
| 10 Web UI — Wallet/Offers/Selections/Bonuses/Categories | **Done** | "Delivery — Scope 10" section (J01–J08 all via live-stack Playwright e2e-ui [7 passed], no request interception, CSP-clean; 3 live-PG store-CRUD integration tests; Docker bundle freshness verified via fresh core image rebuild; Build Quality Gate green, artifact-lint exit 0) |
| 11 Web UI — Dashboard/Recommendations/Rotating-Verify/Report/Admin | Not Started | — |

**Why not all 11 in this run (honest blocker):** delivering and
validate-certifying 11 scopes — Go domain layer, a new Python ML-sidecar route,
a connector, scheduler jobs, a CalDAV bridge, a JSON→PG importer, and 10
server-rendered Web UI pages — requires live-Postgres integration, live-Ollama
extraction (spec-043 infra), Playwright e2e-ui across 7 spec files, and stress
evidence, with ≥10 lines of real terminal output per DoD item and zero
fabrication. That is not achievable in a single automated turn (no sub-agent
delegation; parent-expanded). This run delivered **Scope 01** (dependency root)
and **Scope 02** (domain store + CRUD API) in full with real evidence and
stopped honestly rather than fabricate scopes 03–11. The `card_rewards` feature
flag bundle edit (`config/feature-flags.mvp.yaml`) is intentionally NOT made —
it is `bubbles.train`-owned and routed during delivery per release-train
governance. Continuation: implement **Scope 03** (Data Migration: CCManager
JSON → PostgreSQL; depends on 02 ✓) next, then 04–11 in DAG order.

## Delivery — Scope 02 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` implement/test phases for Scope 02 were executed in
**parent-expanded** form by a single orchestrator agent — NO separate certified
specialist sub-agents (bubbles.implement/test/validate/audit) were dispatched or
claimed. This is disclosed in `state.json.executionHistory`, not fabricated. All
evidence above is from real in-session command execution (unit, live-PG
integration, live-stack e2e, format/check/lint). Only Scope 02 was delivered
this run; scopes 03–11 remain Not Started.

---

## Delivery — Scope 03: Data Migration (CCManager JSON → PostgreSQL) (2026-06-11)

> Scope 03 delivers the one-time, idempotent importer that absorbs the standalone
> CCManager `data/*.json` files into the 10-table PostgreSQL card-rewards schema
> (design §11). It reuses the Scope 02 pgx Store/types and the ResolveCard
> resolver — no duplicate persistence logic. All evidence below is from real
> in-session execution. autoCommit OFF — nothing committed.

### Files created / changed (Scope 03)

- `internal/cardrewards/import.go` (NEW) — CCManager file DTOs; pure
  unit-testable transforms (card-type mapping incl. tiered→user-selected,
  flat/store/hotel/airline→fixed, unknown→skip; dollars→cents; jsonb placement;
  category alias/equivalents flattening; lifecycle/run derivation; date
  parsing); `RunImport` orchestrator with idempotency + partial-file tolerance +
  per-table `ImportReport`.
- `internal/cardrewards/import_transform_test.go` (NEW) — 16 unit tests for the
  transforms (cents conversion, jsonb shaping, alias flattening, lifecycle
  derivation, multi-category offer expansion, boundary/unknown/missing-field
  cases).
- `internal/cardrewards/import_test.go` (NEW, `//go:build integration`) — 4
  live-PG tests: T-03-01 (C01/C05 catalog+aliases+row counts), T-03-03 (C03
  rotating manual_override + known discover-it Q1_2026 value), T-03-04 (C04
  partial-file tolerance + C06 migration run logged), T-03-02 (C02 idempotency).
- `internal/cardrewards/testdata/ccmanager/*` (NEW) — hermetic fixtures
  mirroring the real CCManager shapes (cards-database/config/rotating-categories/
  user-cards/user-offers/user-selections/pending-selections/run-history/
  latest-report + monthly-recommendations/2026-01.json), with intentional skip
  cases (unknown card type, unresolvable wallet name, orphan rotating card,
  unmappable run types).
- `internal/cardrewards/types.go` (M) — added `RotatingCategory`,
  `CardRecommendation`, `CardRun` domain types + lifecycle/run enum constants
  and `ValidLifecycleState`/`ValidRunType`.
- `internal/cardrewards/store.go` (M) — added importer idempotency helpers
  (`GetOrCreateUserCardByCatalog`, `InsertOfferIfAbsent`,
  `InsertSelectionIfAbsent`, `InsertSignupBonusIfAbsent`, `InsertRunIfAbsent`,
  `CountRunsByType`) and `UpsertRotatingCategory` / `UpsertRecommendation` /
  `CreateRun` / `ListRotatingCategoriesByCard` (using `INSERT … WHERE NOT EXISTS
  … IS NOT DISTINCT FROM` for nullable natural keys, `ON CONFLICT` where a
  usable unique key exists).
- `cmd/cardrewards-import/main.go` (NEW) — thin CLI binary (mirrors
  `cmd/dbmigrate`): resolves `--data-dir` flag → `CARD_REWARDS_IMPORT_DIR`
  env fail-loud, connects via `DATABASE_URL`, migrates, runs `RunImport`, prints
  the JSON report.
- `internal/config/cardrewards.go` (M) — added optional `ImportDataDir` field
  (read regardless of `enabled`; invocation-gated, not startup-gated).
- `config/smackerel.yaml` (M) — added `card_rewards.import_data_dir: ""` (empty
  placeholder; operator supplies per environment — No Env-Specific Content).
- `scripts/commands/config.sh` (M) — read + emit `CARD_REWARDS_IMPORT_DIR`
  (optional, mirrors the existing card_rewards var pattern).

### Decisions disclosed (faithful transforms, not silent fallbacks)

- **card_type normalization** (CHECK domain is rotating|fixed|user-selected):
  explicit total map — `tiered→user-selected` (tiers are a selection mechanism;
  `tiered_benefits` → `selectable_benefits` jsonb), `flat/store/hotel/airline→
  fixed`. An **unknown** type is **skipped + logged** (no guessing). The mystery
  fixture card proves this.
- **run_type / trigger**: CCManager-only run types (`user_change`,
  `github_sync`) are **skipped + logged** rather than violating the card_runs
  CHECK; only the mappable subset (`calendar_sync`, …) imports. Trigger
  `auto→scheduled`, `ui/manual→manual`.
- **Idempotency**: `ON CONFLICT` for catalog (id) / aliases (canonical) /
  rotating ((card,period)) / recommendations ((period,category)); `INSERT …
  WHERE NOT EXISTS (… IS NOT DISTINCT FROM …)` for user_cards / offers /
  selections / bonuses / historical runs (their natural keys have nullable
  columns where a UNIQUE constraint treats each NULL as distinct). The
  `migration` audit run is intentionally appended each invocation (Principle 8).
- **Rotating categories** seeded `manual_override=true`, `confidence=1` so the
  first live LLM extraction (Scope 05/06) augments rather than overwrites
  imported history (SCN-083-C03).
- **FK safety / partial tolerance**: rotating quarters for a card not in the
  catalog are skipped + logged (orphan-card fixture); a missing file imports the
  rest (C04).
- **Data dir is invocation-gated fail-loud**: `--data-dir` flag or
  `CARD_REWARDS_IMPORT_DIR`; never a committed real path (the SST value is an
  empty placeholder per No Env-Specific Content).

### Evidence — JSON→row transforms (unit; SCN-083-C01/C03/C05 mapping logic)

Command: `./smackerel.sh test unit --go --go-run '<transform+resolver regex>' --verbose`

```text
[go-unit] starting go test ./...
=== RUN   TestMapCardType
--- PASS: TestMapCardType (0.00s)
=== RUN   TestDollarsToCents
--- PASS: TestDollarsToCents (0.00s)
=== RUN   TestCentsPtr
--- PASS: TestCentsPtr (0.00s)
=== RUN   TestParseDate
--- PASS: TestParseDate (0.00s)
=== RUN   TestParsePeriodRange
--- PASS: TestParsePeriodRange (0.00s)
=== RUN   TestDeriveLifecycle
--- PASS: TestDeriveLifecycle (0.00s)
=== RUN   TestMapRunTypeAndTrigger
--- PASS: TestMapRunTypeAndTrigger (0.00s)
=== RUN   TestNormalizeOfferRateType
--- PASS: TestNormalizeOfferRateType (0.00s)
=== RUN   TestQuarterAndMonthLabel
--- PASS: TestQuarterAndMonthLabel (0.00s)
=== RUN   TestBuildCatalogCard_TypeMappingAndJSONBPlacement
--- PASS: TestBuildCatalogCard_TypeMappingAndJSONBPlacement (0.00s)
=== RUN   TestBuildCategoryAliases_Flattening
--- PASS: TestBuildCategoryAliases_Flattening (0.00s)
=== RUN   TestBuildRotatingCategory
--- PASS: TestBuildRotatingCategory (0.00s)
=== RUN   TestBuildSignupBonuses
--- PASS: TestBuildSignupBonuses (0.00s)
=== RUN   TestBuildOffers_MultiCategoryAndSharedLimit
--- PASS: TestBuildOffers_MultiCategoryAndSharedLimit (0.00s)
=== RUN   TestBuildHistoricalRun
--- PASS: TestBuildHistoricalRun (0.00s)
=== RUN   TestResolveCatalogID_ConfidenceFloor
--- PASS: TestResolveCatalogID_ConfidenceFloor (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.033s
[go-unit] go test ./... finished OK
UNIT_TRANSFORM_EXIT=0
```

### Evidence — SCN-083-C01..C06 (importer on live disposable Postgres)

Command: `./smackerel.sh test integration --go-run CardRewardsImport` (live test
stack up + healthy; go-integration container with `DATABASE_URL` →
`postgres:…/smackerel`; `-tags integration`). All four tests assert against a
real ephemeral database — catalog/alias presence + per-table row counts
(C01/C05), rotating `manual_override=true` with the known `discover-it Q1_2026`
= [Grocery Stores, Wholesale Clubs, Streaming] (C03), missing-file tolerance +
`run_type=migration` row logged (C04/C06), and a second-run zero-duplicate
idempotency check (C02).

```text
go-integration: applying -run selector: CardRewardsImport
=== RUN   TestCardRewardsImport_CatalogAndAliases_C01_C05
--- PASS: TestCardRewardsImport_CatalogAndAliases_C01_C05 (0.10s)
=== RUN   TestCardRewardsImport_RotatingManualOverride_C03
--- PASS: TestCardRewardsImport_RotatingManualOverride_C03 (0.06s)
=== RUN   TestCardRewardsImport_PartialFileToleranceAndRunLogged_C04_C06
--- PASS: TestCardRewardsImport_PartialFileToleranceAndRunLogged_C04_C06 (0.10s)
=== RUN   TestCardRewardsImport_Idempotent_C02
--- PASS: TestCardRewardsImport_Idempotent_C02 (0.11s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.390s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
INTEGRATION_EXIT=0
```

Row-count / key-field assertions exercised inside the passing tests:
- card_catalog: 6 seed cards + 1 manual (signify) present; `mystery-card`
  (unknown type) absent; report `card_catalog=7`.
- category_aliases: report `category_aliases=8`; `Dining` starred+built_in,
  priority 2, 2 equivalents; equivalents-only `gas` → [`fuel`].
- amazon offers = 3 (multi-category combo, shared-limit group, limit 100000¢);
  citi selections = 1; usbank tiered selections = 3 (tier set); citi bonuses = 2;
  recommendations 2026-01 = 2, 2025-12 = 1.
- discover-it rotating = 2; Q1_2026 categories = [Grocery Stores, Wholesale
  Clubs, Streaming], lifecycle active, limit 150000¢, `manual_override=true`,
  `confidence=1.0`; orphan-card skipped.
- C02 idempotency: all DB-scoped data counts identical run A vs run B; re-run
  reported 0 new user_cards/offers/selections/bonuses; calendar_sync historical
  run stable; `migration` audit rows = before+1 (exactly one appended per run).

### Evidence — Build Quality Gate (Scope 03)

Commands (all via `./smackerel.sh`): `config generate`, `format --check`,
`check`, `lint`.

```text
=== CONFIG GENERATE ===
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Generated ~/smackerel/config/generated/dev.env
CONFIG_GENERATE_EXIT=0
=== FORMAT --CHECK ===
FORMAT_CHECK_EXIT=0
=== CHECK ===
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
=== LINT ===
All checks passed!
Web validation passed
LINT_EXIT=0
```

`go test ./...` (unit) compiled every Go package including the new
`cmd/cardrewards-import` binary and the `internal/cardrewards` import code; the
integration run compiled with `-tags integration`. No `${VAR:-default}` runtime
fallback was introduced (`smackerel-no-defaults`); the new `import_data_dir` SST
key uses the established optional `|| VAR=""` generator pattern with a Go-side
invocation-time fail-loud.

### Code Diff Evidence (Scope 03)

Read-only `git status --short` for the Scope 03 surface (autoCommit OFF —
nothing committed this turn):

```text
 M config/smackerel.yaml
 M internal/cardrewards/store.go
 M internal/cardrewards/types.go
 M internal/config/cardrewards.go
 M scripts/commands/config.sh
?? cmd/cardrewards-import/
?? internal/cardrewards/import.go
?? internal/cardrewards/import_test.go
?? internal/cardrewards/import_transform_test.go
?? internal/cardrewards/testdata/
```

### Delivery — Scope 03 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` implement/test phases for Scope 03 were executed in
**parent-expanded** form by a single orchestrator agent — NO separate certified
specialist sub-agents (bubbles.implement/test/validate/audit) were dispatched or
claimed. Disclosed in `state.json.executionHistory`, not fabricated. Only Scope
03 was delivered this run (stopped before Scope 04 per the run contract); scopes
04–11 remain Not Started. Top-level `status` intentionally NOT promoted to
`done` (8 scopes remain). The `card_rewards` feature-flag bundle edit
(`config/feature-flags.mvp.yaml`) is intentionally NOT made — `bubbles.train`-owned.

---

## Delivery — Scope 04: Card-Rewards Source Connector (2026-06-11)

> Scope 04 delivers the fetch-only `card-rewards` source connector
> (FR-CR-007 fetch half, FR-CR-008, Principle 4). For each operator-configured
> source page it emits ONE source-attributed `connector.RawArtifact` carrying
> the verbatim page text + provenance metadata; it performs NO category parsing
> and imports NO `regexp` (extraction is Scope 05). Fetches are isolated
> per-source, health maps consecutive failures via the shared
> `connector.HealthFromErrorCount` thresholds, and the cursor encodes the last
> successful fetch timestamp. All evidence below is from real in-session
> execution. autoCommit OFF — nothing committed.

### Files created / changed (Scope 04)

- `internal/connector/cardrewards/connector.go` (NEW) — `Connector` implementing
  `connector.Connector` (ID/Connect/Sync/Health/Close); `ConnectorID =
  "card-rewards"` constant; `Source` type; fail-loud `parseSources` /
  `parseFetchTimeout` (no defaults); per-source `context.WithTimeout` isolation;
  `validateSourceURL` SSRF guard (scheme allowlist + private/loopback rejection)
  with an unexported `allowPrivateHosts` white-box test seam; `LastSyncStats()`
  observability accessor. No `regexp` import; `RawContent` is the verbatim body;
  `Metadata` carries only `source_name`/`source_url`/`issuer_hint`.
- `internal/connector/cardrewards/connector_test.go` (NEW) — white-box unit
  suite: the 6 Gherkin scenarios (D01–D06) plus fail-loud config, before-connect,
  close, SSRF, and generic-config-form parser tests (14 test funcs + 8 subtests).
- `cmd/core/connectors.go` (M, +36) — import `cardrewardsConnector`; instantiate
  `cardRewardsConn := cardrewardsConnector.New()`; add it to the registration
  slice (`svc.registry.Register`); add an auto-start block gated on
  `cfg.CardRewards.Enabled` that maps `cfg.CardRewards.Sources` →
  `[]cardrewards.Source` and passes `sources` + `fetch_timeout_seconds` via
  `ConnectorConfig.SourceConfig`.

### Decisions disclosed (honest)

- **`New()` hardcodes the ID** to `ConnectorID = "card-rewards"` (mirrors
  `guesthost.New()`), so SCN-083-D01 (`ID() == "card-rewards"`) can never break
  via a wiring typo.
- **Config is read from `ConnectorConfig.SourceConfig`** (the canonical
  framework pattern used by rss `feed_urls` / weather `locations`), not a bespoke
  setter — this keeps the supervisor re-Connect path working. `Connect` is
  fail-loud (Gate G028, `smackerel-no-defaults`): empty/missing sources or a
  non-positive `fetch_timeout_seconds` is an error; there is NO in-code default.
- **`Source` is declared locally** in the connector package (not imported from
  `internal/config`) to keep the connector self-contained and avoid any import
  cycle, matching rss/weather; the wiring converts `config.CardRewardsSource` →
  `cardrewards.Source`.
- **SSRF guard** (scheme allowlist + private/loopback/link-local/unspecified
  rejection) is defense-in-depth even though source URLs are operator SST config;
  an unexported `allowPrivateHosts` field (white-box test seam, default `false`)
  lets the unit tests fetch from loopback `httptest` servers while production
  stays SSRF-safe. Redirects are refused (`CheckRedirect`).
- **"Register in `internal/connector/registry.go`"** (Implementation Plan
  wording) is honored at the canonical registration site
  `cmd/core/registerConnectors` — `registry.go` is the generic `Registry` type;
  every connector (rss/weather/markets/qf-decisions/…) is instantiated and
  `svc.registry.Register`-ed there. The connector registers unconditionally and
  auto-starts only when `card_rewards.enabled` (dev default `false`, so dev boot
  is unchanged).
- **`cmd/core/main_test.go` curated 15-connector list NOT modified**: it builds
  its own hardcoded list and does NOT call `registerConnectors`; it already omits
  the 16th real connector (`qf-decisions`), so it is a curated subset sanity
  test, not an exhaustive registry mirror. Adding card-rewards there (and the
  pre-existing qf-decisions gap) is out of Scope 04; the run confirmed it still
  passes (`ok …/cmd/core`).

### Evidence — SCN-083-D01..D06 + edges (unit, live `go test` in Docker)

Command: `./smackerel.sh test unit --go --go-run '<D01..D06 + edge regex>'
--verbose`. `go test ./...` compiled every Go package (including the
`cmd/core` wiring — `ok …/cmd/core`), then ran the cardrewards suite. Loopback
(`127.0.0.1:*`) targets are httptest servers; the WARN lines are the connector
correctly recording per-source failures (slow-source timeout for D04; 10
consecutive HTTP 500s for D05).

```text
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core 0.289s [no tests to run]
ok      github.com/smackerel/smackerel/internal/cardrewards     0.024s [no tests to run]
ok      github.com/smackerel/smackerel/internal/config  0.097s [no tests to run]
ok      github.com/smackerel/smackerel/internal/connector       0.065s [no tests to run]
=== RUN   TestConnector_ImplementsInterfaceAndID_D01
--- PASS: TestConnector_ImplementsInterfaceAndID_D01 (0.00s)
=== RUN   TestSync_CursorEncodesLastSuccessfulFetch_D06
2026/06/11 15:42:46 INFO card-rewards connector connected id=card-rewards sources=1 fetch_timeout=5s
--- PASS: TestSync_CursorEncodesLastSuccessfulFetch_D06 (0.03s)
=== RUN   TestSync_EmitsSourceAttributedArtifactPerSource_D02
2026/06/11 15:42:46 INFO card-rewards connector connected id=card-rewards sources=2 fetch_timeout=5s
--- PASS: TestSync_EmitsSourceAttributedArtifactPerSource_D02 (0.02s)
=== RUN   TestSync_NoCategoryParsingRawContentVerbatim_D03
2026/06/11 15:42:46 INFO card-rewards connector connected id=card-rewards sources=1 fetch_timeout=5s
--- PASS: TestSync_NoCategoryParsingRawContentVerbatim_D03 (0.01s)
=== RUN   TestSync_SlowSourceDegradesOnlyThatSource_D04
2026/06/11 15:42:46 INFO card-rewards connector connected id=card-rewards sources=2 fetch_timeout=5s
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=slow-source url=http://127.0.0.1:35011 error="fetch: Get \"http://127.0.0.1:35011\": context deadline exceeded"
--- PASS: TestSync_SlowSourceDegradesOnlyThatSource_D04 (0.17s)
=== RUN   TestHealth_ReflectsConsecutiveErrors_D05
2026/06/11 15:42:46 INFO card-rewards connector connected id=card-rewards sources=1 fetch_timeout=5s
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=always-fails url=http://127.0.0.1:46375 error="unexpected HTTP status 500"
--- PASS: TestHealth_ReflectsConsecutiveErrors_D05 (0.02s)
=== RUN   TestSync_SuccessResetsConsecutiveErrors
--- PASS: TestSync_SuccessResetsConsecutiveErrors (0.01s)
=== RUN   TestSync_TotalFailureKeepsCursor
2026/06/11 15:42:46 WARN card-rewards source fetch failed source=down url=http://127.0.0.1:35395 error="unexpected HTTP status 502"
--- PASS: TestSync_TotalFailureKeepsCursor (0.01s)
=== RUN   TestConnect_FailsLoudOnInvalidConfig
--- PASS: TestConnect_FailsLoudOnInvalidConfig (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/nil_source_config (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/missing_sources (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/empty_sources (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/source_missing_url (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/source_missing_name (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/missing_timeout (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/zero_timeout (0.00s)
    --- PASS: TestConnect_FailsLoudOnInvalidConfig/negative_timeout (0.00s)
=== RUN   TestSync_BeforeConnectErrors
--- PASS: TestSync_BeforeConnectErrors (0.00s)
=== RUN   TestClose_SetsDisconnectedAndBlocksSync
--- PASS: TestClose_SetsDisconnectedAndBlocksSync (0.01s)
=== RUN   TestValidateSourceURL_SSRFGuard
--- PASS: TestValidateSourceURL_SSRFGuard (0.00s)
=== RUN   TestParseSources_AcceptsGenericConfigForms
--- PASS: TestParseSources_AcceptsGenericConfigForms (0.00s)
=== RUN   TestParseFetchTimeout_AcceptsNumericForms
--- PASS: TestParseFetchTimeout_AcceptsNumericForms (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/cardrewards   0.333s
[go-unit] go test ./... finished OK
```

Scenario → assertion mapping (all PASS above):
- **D01** `TestConnector_ImplementsInterfaceAndID_D01` — compile-time
  `var _ connector.Connector = New()` + `ID() == "card-rewards"`.
- **D06** `TestSync_CursorEncodesLastSuccessfulFetch_D06` — cursor parses as
  RFC3339Nano and falls within `[before, after]` the sync.
- **D02** `TestSync_EmitsSourceAttributedArtifactPerSource_D02` — 2 sources →
  2 artifacts; each `Metadata` carries matching `source_name`/`source_url`/
  `issuer_hint`; `SourceID == "card-rewards"`.
- **D03** `TestSync_NoCategoryParsingRawContentVerbatim_D03` — `RawContent ==`
  verbatim body; `Metadata` has exactly 3 provenance keys; no
  `categories`/`category`/`rate`/`cashback`/`rewards` parsed keys.
- **D04** `TestSync_SlowSourceDegradesOnlyThatSource_D04` — slow source trips its
  per-source deadline (the WARN line) and is recorded failed; the fast source
  still emits 1 artifact; `LastSyncStats() = (1,1)`; health `degraded`; cursor
  advances.
- **D05** `TestHealth_ReflectsConsecutiveErrors_D05` — 1–4 failures → `healthy`,
  5th → `degraded`, 10th → `failing` (the 10 WARN lines), via
  `connector.HealthFromErrorCount`.

### Evidence — Build Quality Gate (Scope 04)

`./smackerel.sh check` (host path redacted `~/` per No-PII), `format --check`,
`lint` — all green:

```text
=== ./smackerel.sh check ===
config-validate: ~/smackerel/config/generated/dev.env.tmp.391529 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK

=== ./smackerel.sh format --check ===
63 files already formatted

=== ./smackerel.sh lint ===
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

`go test ./...` (unit run) compiled every Go package including the new
`internal/connector/cardrewards` package and the `cmd/core` connector wiring.
No `${VAR:-default}` runtime fallback was introduced (`smackerel-no-defaults`);
`Connect` fail-loud rejects missing/empty sources and non-positive timeout.

### Gate: artifact-lint — Scope 04

Command: `bash .github/bubbles/scripts/artifact-lint.sh
specs/083-card-rewards-companion` → **Artifact lint PASSED**, `ARTIFACT_LINT_EXIT=0`.

```text
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: full-delivery
✅ state.json v3 has required field: status / execution / certification / policySnapshot
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' (pre-existing; non-blocking)
ℹ️  Workflow mode 'full-delivery' allows status 'done'; current status is 'in_progress'
✅ report.md contains section: Summary / Completion Statement / Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

The single ⚠️ (`scopeProgress` deprecation) is a pre-existing schema choice in
this spec's `state.json` (used by Scopes 01–03), not introduced by Scope 04; the
lint PASSES. The ℹ️ note is expected — the spec is mid-delivery (4 of 11 scopes
Done) so top-level `status` stays `in_progress`.

### Code Diff Evidence (Scope 04)

Read-only `git status --short` + `git diff --stat` for the Scope 04 surface
(autoCommit OFF — nothing committed this turn):

```text
 M cmd/core/connectors.go
?? internal/connector/cardrewards/
---DIFFSTAT---
 cmd/core/connectors.go | 36 ++++++++++++++++++++++++++++++++++++
 1 file changed, 36 insertions(+)
```

The new package `internal/connector/cardrewards/` (connector.go +
connector_test.go) is an untracked new directory; the only modified tracked file
is the +36-line connector wiring in `cmd/core/connectors.go`.

### Delivery — Scope 04 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` implement/test phases for Scope 04 were executed in
**parent-expanded** form by a single orchestrator agent — NO separate certified
specialist sub-agents (bubbles.implement/test/validate/audit) were dispatched or
claimed. Disclosed in `state.json.executionHistory`, not fabricated. Only Scope
04 was delivered this run (Scopes 05–11 NOT started, per the run contract).
Top-level `status` intentionally NOT promoted to `done` (7 scopes remain). The
`card_rewards` feature-flag bundle edit (`config/feature-flags.mvp.yaml`) is
intentionally NOT made — `bubbles.train`-owned. The working tree is left ready
for an orchestrator preservation commit (Scope 04 surface only).

---

## Delivery — Scope 05: LLM Category Extraction (replaces regex) (2026-06-11)

> Scope 05 replaces the CCManager regex scraper — and its silent fallback to
> stale / placeholder rotating categories — with strict-schema LLM extraction.
> Constitution C2 boundary: the model-gateway call lives ONLY in the Python ML
> sidecar route `POST /extract-card-categories`; the Go orchestrator
> `internal/cardrewards/extract.go` sends page text + candidate over the
> existing Go↔sidecar HTTP contract, validates the response with
> `santhosh-tekuri/jsonschema/v6` (defense-in-depth), applies confidence /
> unknown-card / card-period-echo handling, and persists observations + an
> `extract` audit run. The anti-silent-fallback contract is the whole point:
> a response that does not parse/validate, echoes the wrong card/period, or
> names an unknown card is DISCARDED or SKIPPED — never stored, never mismapped,
> and never used to overwrite an existing reconciled record.

### Files created / changed (Scope 05)

- `internal/cardrewards/extract.go` (NEW) — orchestrator: pure `validateExtraction`
  (schema + card/period echo + date checks + unknown-card skip + low-confidence
  flag), `Extractor.Run` (sidecar seam + store + `extract` audit run), the
  `SidecarExtractor` interface (deterministic test seam), and the production
  `HTTPSidecarExtractor` (POST `/extract-card-categories`, Bearer auth, fail-loud
  constructor). NO direct model-backend client (C2).
- `internal/cardrewards/types.go` (M) — `RotatingCategoryObservation` + `CardPeriodRef`.
- `internal/cardrewards/store.go` (M) — `GetRotatingCategory`,
  `ListObservationsByCardPeriod`, `scanObservation`, and the transactional
  `PersistExtractionRun` (FK-safe: run → observations → flag-existing, atomic;
  flagging only sets `needs_verification`, never rewrites categories).
- `internal/cardrewards/extract_test.go` (NEW) — unit T-05-01..04 (SCN-083-E01..E07
  pure validation + page-content-as-data + transport against httptest + fail-loud).
- `internal/cardrewards/extract_integration_test.go` (NEW, `//go:build integration`) —
  live-PG persistence for E01–E07 + audit run, via a deterministic fake of the
  EXTERNAL model-gateway boundary (no internal mocks; Store + DB are real).
- `tests/integration/cardrewards_extract_test.go` (NEW, `//go:build integration`) —
  T-05-05 / SCN-083-E08 live-stack round-trip (real ml sidecar), gated on sidecar
  reachability.
- `ml/app/card_categories.py` (NEW) — sidecar route + pure `build_extraction_messages`
  (prompt-injection defense: page content in a delimited DATA block, system prompt
  forbids following embedded instructions) + `parse_strict_response` (first
  strict-JSON pass). litellm imported lazily (dev test lane needs no litellm).
- `ml/app/main.py` (M) — mount the card-categories router under `verify_auth`.
- `ml/tests/test_card_categories.py` (NEW) — Python unit T-05-06 (SCN-083-E01 + E06 pure).
- Doc-drift reconciliation (card-rewards domain, pre-existing from scopes 01–04):
  `docs/smackerel.md` §22.7 connector inventory 16→17 (+card-rewards row);
  `docs/Development.md` passive-connector bullet 16→17, `internal/cardrewards/`
  Go-packages row, `057_card_rewards.sql` migrations row, `cardrewards/` connector
  sub-package; `internal/deploy/docs_connector_count_contract_test.go`
  adversarial `SmackerelMdHigh` fixture made self-adjusting (`runtime+1`).

### Decisions disclosed (honest)

1. **Target card+period echo.** `ExtractInput` carries the target `CardID` +
   `PeriodLabel`; the sidecar response MUST echo both. A mismatch (hallucination
   or prompt-injection trying to switch cards) is DISCARDED — never stored under
   the target. This is a Go-side mismap/injection defense complementing the
   sidecar's system-prompt defense (SCN-083-E06).
2. **`spend_limit` is whole dollars** on the page; the orchestrator converts ×100
   to `limit_cents` for the column.
3. **Pure decision vs persistence split.** `validateExtraction` is a pure function
   of (raw, input, knownCard, threshold) → unit-testable with NO DB and NO mocks
   (E01–E07). `Extractor.Run` wires it to a REAL `Store` (live PG) for the audited
   path (E08). Only the EXTERNAL model-gateway boundary is faked in integration
   tests — the Store and DB are always real (no-internal-mocks policy honored).
4. **`extract` run status.** `success` only when every input produced a stored
   observation; any discard/skip/sidecar-error → `partial` (matches SCN-083-E02).
5. **No new config keys / no cron wiring.** `MLSidecarURL` + `AuthToken` +
   `Extraction.ConfidenceThreshold` already exist (Scopes 01/02). Scheduler/cron
   wiring of the extractor is Scope 09, not Scope 05.

### Gate: Constitution C2 boundary grep (DoD item)

```
$ grep -rniE 'ollama|/api/generate|/api/chat' internal/cardrewards/ ; echo "C2_GREP_EXIT=$?"
C2_GREP_EXIT=1 (1 = no matches = PASS)
```

Zero hits — the model-gateway call lives ONLY in the Python sidecar
(`ml/app/card_categories.py`), never in the Go orchestrator package.

### Evidence — SCN-083-E01..E07 on live disposable Postgres (integration, fake external-sidecar seam)

`./smackerel.sh test integration --go-run 'ExtractorLivePG|CardRewardsExtractLiveStack'` →
full disposable stack came up Healthy, then:

```
=== RUN   TestExtractorLivePG_StoresObservationWithProvenance_E01_E07
--- PASS: TestExtractorLivePG_StoresObservationWithProvenance_E01_E07 (0.25s)
=== RUN   TestExtractorLivePG_MalformedDiscardedNoOverwrite_E02_E03
2026/06/11 ... WARN card-rewards extraction discarded (invalid) — flagging target for verification card_id=cr-int-...-discover-it period=Q3_2026 source="Doctor of Credit" reason="response is not valid JSON: invalid character 'D' looking for beginning of value"
--- PASS: TestExtractorLivePG_MalformedDiscardedNoOverwrite_E02_E03 (0.14s)
=== RUN   TestExtractorLivePG_LowConfidenceStored_E04
2026/06/11 ... INFO card-rewards extraction below confidence threshold — reconciler will flag card_id=cr-int-...-discover-it period=Q3_2026 source="Doctor of Credit"
--- PASS: TestExtractorLivePG_LowConfidenceStored_E04 (0.18s)
=== RUN   TestExtractorLivePG_UnknownCardSkippedNoMismap_E05
2026/06/11 ... INFO card-rewards extraction skipped unknown card — not mismapped card_id=cr-int-...-ghost-card source="Doctor of Credit" reason="card_id \"cr-int-...-ghost-card\" is not in card_catalog"
--- PASS: TestExtractorLivePG_UnknownCardSkippedNoMismap_E05 (0.10s)
=== RUN   TestExtractorLivePG_ExtractionRunAudited_E08
2026/06/11 ... WARN card-rewards extraction sidecar error — flagging target for verification card_id=cr-int-...-chase-freedom period=Q3_2026 source="Doctor of Credit" error="source page unreachable"
--- PASS: TestExtractorLivePG_ExtractionRunAudited_E08 (0.09s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.946s
PASS: go-integration
... (project-scoped test stack teardown: all containers + volumes + network removed) ...
INTEG_EXIT=0
```

The adversarial proof (SCN-083-E02/E03): `MalformedDiscardedNoOverwrite` seeds an
authoritative existing `rotating_categories` row (`manual_override=true`,
`confidence=1.0`, categories `[Grocery Stores, Streaming]`, `needs_verification=false`),
feeds garbage to the sidecar, and asserts against the real DB afterward that the
existing row's categories/confidence/manual_override are UNCHANGED and only
`needs_verification` flipped to `true` — the exact CCManager silent-fallback
failure mode, proven fixed. SCN-083-E05 asserts the unknown observation is
neither persisted nor mismapped onto the co-resident known card.

### Evidence — SCN-083-E01..E07 (unit; `extract_test.go`)

`./smackerel.sh test unit --go --go-run 'ValidateExtraction|ExtractRequest|HTTPSidecarExtractor|NewHTTPSidecarExtractor|ValidRunTrigger' --verbose`:

```
--- PASS: TestValidateExtraction_ValidRecordWithProvenance_E01_E07 (0.00s)
--- PASS: TestValidateExtraction_MalformedAndInvalidDiscarded_E02_E03 (0.00s)
    --- PASS: .../non-JSON garbage (0.00s)
    --- PASS: .../truncated_partial JSON (0.00s)
    --- PASS: .../empty categories array (old path would store stale/empty) (0.00s)
    --- PASS: .../missing categories key (0.00s)
    --- PASS: .../confidence out of range (0.00s)
    --- PASS: .../unexpected extra field (additionalProperties:false) (0.00s)
    --- PASS: .../unparseable period date (0.00s)
    --- PASS: .../period_end before period_start (0.00s)
--- PASS: TestValidateExtraction_LowConfidenceFlagged_E04 (0.00s)
--- PASS: TestValidateExtraction_UnknownCardSkipped_E05 (0.00s)
--- PASS: TestExtractRequest_PageContentIsDataNotInstructions_E06 (0.00s)
--- PASS: TestValidateExtraction_RejectsCardOrPeriodMismatch_E06 (0.00s)
    --- PASS: .../card_id mismatch (0.00s)
    --- PASS: .../period mismatch (0.00s)
--- PASS: TestNewHTTPSidecarExtractor_FailLoud (0.00s)  [empty_baseURL, empty_token, non-positive_timeout]
--- PASS: TestHTTPSidecarExtractor_Transport (0.13s)  [2xx returns raw body + bearer + /extract-card-categories path, non-2xx error, empty-body error]
--- PASS: TestValidRunTrigger (0.00s)
ok      github.com/smackerel/smackerel/internal/cardrewards     0.208s
UNIT_VERBOSE_EXIT=0
```

These are pure-function tests (no DB, no internal mocks): `validateExtraction`
proves the design §4 contract for every adversarial shape, and the 8 discard
subtests each use input the regex silent-fallback path would have accepted —
here each fails loud to verification with NO observation produced.

### Evidence — SCN-083-E01 + E06 (Python sidecar unit; `ml/tests/test_card_categories.py`)

`./smackerel.sh test unit --python`:

```
........................................................................ [ 98%]
.......                                                                  [100%]
509 passed, 2 skipped, 2 warnings in 18.90s
[py-unit] pytest ml/tests finished OK
PY_UNIT_EXIT=0
```

`test_card_categories.py` covers: `parse_strict_response` accepts a valid record
(E01) and rejects non-JSON / truncated / empty-categories / missing-key /
out-of-range-confidence / additionalProperties (the inputs the regex fallback
would have swallowed); `build_extraction_messages` places injected
"ignore previous instructions" text ONLY inside the untrusted PAGE_CONTENT data
block of the user message, never in the system instructions, and the system
prompt declares the block untrusted data + forbids following it (E06). The 2
warnings are pre-existing third-party (`fastapi/testclient` deprecation;
`test_nats_consumer_config` coroutine) — none from this scope.

### Evidence — SCN-083-E08 (live PG + real ml sidecar round-trip)

`./smackerel.sh test integration --go-run 'CardRewardsExtractLiveStack'` — the full
disposable stack came up Healthy (postgres, nats, ollama, ml sidecar, core); the test
gates on sidecar reachability (ML_SIDECAR_URL is injected by the integration runner) so
it RUNS the real orchestrator → sidecar round-trip:

```
=== RUN   TestCardRewardsExtractLiveStackAudited_E08
2026/06/11 ... WARN card-rewards extraction sidecar error — flagging target for verification card_id=discover-it period=Q3_2026 source="Live Stack Source" error="cardrewards sidecar: /extract-card-categories returned HTTP 502: {\"detail\":\"model gateway unreachable: APIConnectionError\"}"
    cardrewards_extract_test.go:144: SCN-083-E08 live extraction: stored=0 discarded=1 skipped=0 lowConfidence=0 flagged=0
--- PASS: TestCardRewardsExtractLiveStackAudited_E08 (0.09s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.222s
PASS: go-integration
... (project-scoped test stack teardown: all containers + volumes + network removed) ...
E08LIVE_EXIT=0
```

**HONEST status (blocked-needs-live-Ollama).** The test exercised the real
orchestrator → ml-sidecar HTTP round-trip end-to-end: the sidecar's
`/extract-card-categories` route returned a STRUCTURED HTTP 502
(`model gateway unreachable: APIConnectionError`) — which proves the route is
deployed in the rebuilt image (a 404 would mean otherwise) and that the
sidecar's strict error contract works. The orchestrator FAILED LOUD
(discarded, flagged the target `needs_verification`, persisted the `extract`
audit run) — never a silent fallback to a stale/placeholder category, which is
exactly the CCManager failure mode this scope replaces. The sidecar→Ollama
MODEL INFERENCE leg could NOT complete in THIS environment: litellm raised
`APIConnectionError` because the disposable-stack Ollama serves no pulled LLM
model (the `integration` lane does not run the spec-043 `ollama-test-pull`
step, and — confirmed empirically — does not forward `SMACKEREL_TEST_OLLAMA`
into the go-test container). A SUCCESSFUL live Ollama inference round-trip
therefore remains **blocked-needs-live-Ollama**; it is satisfiable on the
scenario's <home-lab-host> ops node (which has Ollama + the model). The audit-run
PERSISTENCE half of E08 is independently PROVEN on live Postgres by
`TestExtractorLivePG_ExtractionRunAudited_E08` (PASS, above).

### Evidence — Build Quality Gate (Scope 05)

```
$ ./smackerel.sh check
config-validate: .../test.env... OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0

$ ./smackerel.sh lint
All checks passed!
Web validation passed
LINT_EXIT=0

$ ./smackerel.sh format --check
65 files already formatted
FORMAT_RECHECK_EXIT=0
```

Connector-count contract + doc-freshness (card-rewards domain reconciliation):

```
--- PASS: TestConnectorCountContract_LiveFile (0.00s)   (all three surfaces agree on 17 connectors)
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.026s
--- PASS: TestDocFreshness_AllInternalPackagesDocumented (0.01s)
--- PASS: TestDocFreshness_AllMigrationsDocumented (0.00s)
--- PASS: TestDocFreshness_AllPromptContractsDocumented (0.00s)
ok      github.com/smackerel/smackerel/internal/docfreshness    0.028s
```

### Pre-existing, out-of-domain (NOT introduced by Scope 05)

`internal/scopesdriftguard` `TestScopesPathRefDrift_NonIncreasing` fails at 287
broken `specs/*/scopes.md` file references vs a ratchet ceiling of 270. The
breakdown is dominated by unrelated features: 034-expense (81), 035-recipe (62),
036-mealplan (41), 061-assistant (39), 063-enrichment (40). Spec 083 contributes
17 — all FORWARD references to files for unbuilt scopes 06–11 (reconcile.go,
optimize.go, calendar.go, scheduler/cardrewards.go, web/cardrewards.go, the
e2e-ui specs) that resolve as those scopes land. Scope 05 creating the five
Scope-05 files actually REDUCED 083's contribution (from ~22 to 17, total ~292→287).
The ratchet was already exceeded at the committed Scope-04 HEAD; this is a
pre-existing repo-wide drift, not a Scope-05 regression, and is NOT being
worked around by raising the ratchet.

### Gate: artifact-lint — Scope 05

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' (pre-existing from Scopes 01–04; non-blocking)
ℹ️  Workflow mode 'full-delivery' allows status 'done'; current status is 'in_progress'
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

### Delivery — Scope 05 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` implement/test phases for Scope 05 were executed in
**parent-expanded** form by a single orchestrator agent — NO separate certified
specialist sub-agents (bubbles.implement/test/validate/audit) were dispatched or
claimed. Disclosed in `state.json.executionHistory`, consistent with Scopes
01–04. Only Scope 05 was delivered this run (Scopes 06–11 NOT started). The
`card_rewards` feature-flag bundle edit (`config/feature-flags.mvp.yaml`) is
intentionally NOT made — `bubbles.train`-owned. NOT committed (autoCommit off):
the working tree is left ready for an orchestrator preservation commit (Scope 05
surface + the card-rewards doc-drift reconciliation + the self-adjusting
connector-count adversarial fixture). Top-level `status` intentionally NOT
promoted to `done`: Scope 05's E08 successful-live-Ollama-inference item is
blocked-needs-live-Ollama and 6 scopes (06–11) remain, so `currentScope` stays
5 and `completedScopes` is NOT extended with "5".

---

## Delivery — Scope 06: Multi-Source Reconciliation & Category Lifecycle (2026-06-11)

> Scope 06 was delivered by a prior implement pass that was cut off mid-run: the
> implementation existed on disk (`internal/cardrewards/reconcile.go` + the new
> Store/types methods) and the unit tests were reported passing, but the live-PG
> integration results (F06/F07) were never captured and the scope status was
> never flipped. This run is a **VERIFY + CERTIFY** pass: every Scope 06 test was
> re-run via the repo CLI with full raw evidence below; no implementation bug was
> found, so `reconcile.go`/`store.go`/`types.go` were NOT modified. All evidence
> is from real in-session execution. autoCommit OFF — nothing committed.

### Files verified (Scope 06)

- `internal/cardrewards/reconcile.go` (NEW, pre-existing on disk) — `Reconciler`
  with PURE `mergeObservations` (F01 agreement; F02 disagreement →
  `needs_verification`+conservative confidence; F03 manual-override protection;
  plurality selection deterministic) and PURE date-driven lifecycle via the
  shared `deriveLifecycle`; `Reconcile` (idempotent per-(card,period) upsert +
  `card_runs` reconcile audit row) and `AdvanceLifecycle` (upcoming→active→expired
  transitions + `ListPendingReEnrollments` for the dashboard). Owns no
  model/network access (Store + clock only).
- `internal/cardrewards/reconcile_test.go` (NEW) — T-06-01/02/03 unit tests for
  the PURE decisions (F01–F05 + UC-002 A2 low-confidence + no-observation edge).
  F02/F03 are ADVERSARIAL with explicit `REGRESSION` assertions that fail if the
  protection is removed (not tautological).
- `internal/cardrewards/reconcile_integration_test.go` (NEW, `//go:build
  integration`) — T-06-04 live-PG tests: F07 idempotent upsert (asserts exactly
  ONE `rotating_categories` row per (card,period) + stable id after running the
  reconciler twice), F06 re-enrollment pending action (window-opens-today &
  not-enrolled surfaced; future/already-enrolled NOT), plus live F04/F05
  transitions+exclusion, live F02 (both observations retained), live F03
  (override not rewritten + observation retained for audit).
- `internal/cardrewards/store.go` (M) — added `UpsertRotatingCategory`,
  `GetRotatingCategory`, `ListAllRotatingCategories`, `ListActiveRotatingCategories`,
  `UpdateRotatingLifecycle`, `ListPendingReEnrollments`,
  `CountRotatingCategoriesByCardPeriod` (idempotent upsert keyed on
  `(card_catalog_id, period_label)`; active-set excludes `expired`).
- `internal/cardrewards/types.go` (M) — added `PendingReEnrollment` +
  `CardPeriodRef` (and the `RotatingCategoryObservation` consumed by the merge).

### Evidence — SCN-083-F01/F02/F03/F04/F05 + UC-002 A2 (reconcile unit tests)

Command: `./smackerel.sh test unit --go --go-run 'TestReconcile_' --verbose`
(runs `go test -v -run TestReconcile_ -count=1 ./...`; only the cardrewards
pure-decision tests match — F02/F03 are adversarial with `REGRESSION`
assertions). Full raw output:

```text
[go-unit] applying -run selector: TestReconcile_
[go-unit] starting go test ./...
+ go test -v -run TestReconcile_ -count=1 ./...
=== RUN   TestReconcile_MergeAgreement_F01
--- PASS: TestReconcile_MergeAgreement_F01 (0.00s)
=== RUN   TestReconcile_MergeDisagreement_F02
--- PASS: TestReconcile_MergeDisagreement_F02 (0.00s)
=== RUN   TestReconcile_ManualOverrideNeverOverwritten_F03
--- PASS: TestReconcile_ManualOverrideNeverOverwritten_F03 (0.00s)
=== RUN   TestReconcile_LifecycleByDate_F04_F05
=== RUN   TestReconcile_LifecycleByDate_F04_F05/upcoming:_start_in_future
=== RUN   TestReconcile_LifecycleByDate_F04_F05/F04_active:_start_past,_end_future
=== RUN   TestReconcile_LifecycleByDate_F04_F05/F05_expired:_end_in_past
=== RUN   TestReconcile_LifecycleByDate_F04_F05/active_boundary:_end_==_today
=== RUN   TestReconcile_LifecycleByDate_F04_F05/active_boundary:_start_==_today
=== RUN   TestReconcile_LifecycleByDate_F04_F05/undated:_cannot_determine
--- PASS: TestReconcile_LifecycleByDate_F04_F05 (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/upcoming:_start_in_future (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/F04_active:_start_past,_end_future (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/F05_expired:_end_in_past (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/active_boundary:_end_==_today (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/active_boundary:_start_==_today (0.00s)
    --- PASS: TestReconcile_LifecycleByDate_F04_F05/undated:_cannot_determine (0.00s)
=== RUN   TestReconcile_LowConfidenceFlags_A2
--- PASS: TestReconcile_LowConfidenceFlags_A2 (0.00s)
=== RUN   TestReconcile_NoObservationsKeepsExisting
--- PASS: TestReconcile_NoObservationsKeepsExisting (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.044s
[go-unit] go test ./... finished OK
@@@UNIT_EXIT=0@@@
```

- **F01** (agreeing sources → agreed set, `source_count=2`, `needs_verification=false`,
  confidence = max 0.90): PASS.
- **F02 (adversarial)** — two distinct category sets MUST be flagged
  `needs_verification=true` with conservative lower confidence 0.88 and
  `source_count=1` (plurality only); `REGRESSION` guards fail if disagreement is
  silently reconciled as agreement: PASS.
- **F03 (adversarial)** — a high-confidence (0.99) DISAGREEING observation MUST
  NOT overwrite a `manual_override=true` record; categories stay
  `[Gym Memberships]`, confidence stays 1.0, `needs_verification` stays false:
  PASS.
- **F04/F05** — date-driven lifecycle (upcoming/active/expired incl. both
  boundary days + undated→unknown) all 6 subtests PASS; the end-to-end merge of a
  past-period observation reconciles to `expired`.

### Evidence — SCN-083-F06/F07 (+ live F02/F03/F04/F05) on live disposable Postgres

Command: `./smackerel.sh test integration --go-run 'ReconcileLivePG'` (live test
stack built + brought up healthy: postgres/nats/ollama/ml/core all Healthy;
go-integration container with `DATABASE_URL` → the disposable
`smackerel-test-postgres`; `-tags integration`; each test namespaces its catalog
ids with a per-test prefix). Only the 5 reconcile live-PG tests match the
selector; the disagreement/override WARN/INFO log lines are the reconciler's own
audit output. Full raw output (test block + teardown):

```text
go-integration: applying -run selector: ReconcileLivePG
=== RUN   TestReconcileLivePG_IdempotentUpsert_F07
--- PASS: TestReconcileLivePG_IdempotentUpsert_F07 (0.05s)
=== RUN   TestReconcileLivePG_PendingReEnrollment_F06
2026/06/11 18:50:26 INFO card-rewards lifecycle: pending re-enrollment actions surfaced for dashboard count=1
--- PASS: TestReconcileLivePG_PendingReEnrollment_F06 (0.03s)
=== RUN   TestReconcileLivePG_LifecycleTransitions_F04_F05
2026/06/11 18:50:26 INFO card-rewards lifecycle transition card_id=cr-int-20260611185026-248219806-discover-it period=P_active from=upcoming to=active
2026/06/11 18:50:26 INFO card-rewards lifecycle transition card_id=cr-int-20260611185026-248219806-discover-it period=P_expired from=active to=expired
2026/06/11 18:50:26 INFO card-rewards lifecycle: pending re-enrollment actions surfaced for dashboard count=1
--- PASS: TestReconcileLivePG_LifecycleTransitions_F04_F05 (0.04s)
=== RUN   TestReconcileLivePG_DisagreementRetainsBothObservations_F02
2026/06/11 18:50:26 WARN card-rewards reconcile: sources disagree — flagged needs_verification, both observations retained card_id=cr-int-20260611185026-294831726-discover-it period=Q3_2026
--- PASS: TestReconcileLivePG_DisagreementRetainsBothObservations_F02 (0.04s)
=== RUN   TestReconcileLivePG_ManualOverrideNotRewritten_F03
2026/06/11 18:50:26 INFO card-rewards reconcile: manual override protected — record unchanged, observations retained for audit card_id=cr-int-20260611185026-333647277-discover-it period=Q3_2026
--- PASS: TestReconcileLivePG_ManualOverrideNotRewritten_F03 (0.03s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.255s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
@@@INTEG_EXIT=0@@@
```

- **F07 (idempotent upsert)** — `TestReconcileLivePG_IdempotentUpsert_F07`
  reconciles the same two agreeing observations TWICE, then asserts
  `CountRotatingCategoriesByCardPeriod == 1` and the row id is unchanged across
  runs (`REGRESSION` guard: "expected exactly 1 rotating_categories row per
  (card,period)"); the agreed record is `source_count=2`,
  `needs_verification=false`, categories `[PayPal Restaurants]`: PASS.
- **F06 (re-enrollment pending action)** — `TestReconcileLivePG_PendingReEnrollment_F06`
  seeds 3 selections (window-opens-today + not-enrolled; future window;
  open-but-enrolled) and asserts `AdvanceLifecycle` + the dashboard read path
  (`ListPendingReEnrollments`) BOTH surface exactly 1 pending action (the
  Restaurants window opening today) with the resolved catalog name: PASS.
- **F04/F05 (live)** — `TestReconcileLivePG_LifecycleTransitions_F04_F05` seeds an
  `upcoming` record whose window is now current and an `active` record whose
  window has ended; after `AdvanceLifecycle` they become `active`/`expired`
  respectively, and the expired record is EXCLUDED from
  `ListActiveRotatingCategories` (`REGRESSION` guard): PASS.
- **F02 (live)** — disagreement persists `needs_verification=true` AND retains
  BOTH observations for audit: PASS.
- **F03 (live)** — a high-confidence disagreeing observation does NOT rewrite a
  manual-override row (`overrides_protected=1`, `reconciled=0`); the observation
  is retained for audit: PASS.

Teardown removed all ephemeral containers + the postgres volume + the network —
the disposable test stack was fully torn down (test-environment isolation
honored; no residual state).

### Evidence — Build Quality Gate (Scope 06)

All four gate commands run via the repo CLI on the working tree (which includes
the Scope 06 reconcile surface). No implementation bug was found by any test, so
`reconcile.go` / `store.go` / `types.go` were NOT modified during this
verify+certify pass.

`./smackerel.sh check` → `CHECK_EXIT=0`:

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.3340258 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
@@@CHECK_EXIT=0@@@
```

`./smackerel.sh lint` → `LINT_EXIT=0` (golangci-lint over the whole module incl.
the reconcile surface, then web validation):

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
@@@LINT_EXIT=0@@@
```

`./smackerel.sh format --check` → `FORMAT_EXIT=0`:

```text
65 files already formatted
@@@FORMAT_EXIT=0@@@
```

`bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion`
→ `ARTIFACT_LINT_EXIT=0`:

```text
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: full-delivery
✅ Top-level status matches certification.status
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
@@@ARTIFACT_LINT_EXIT=0@@@
```

(One non-blocking `⚠️ state.json uses deprecated field 'scopeProgress'` warning is
pre-existing for this spec and unchanged by Scope 06 — artifact lint still
PASSED with exit 0.)

### Code Diff Evidence (Scope 06)

Real `git status --short` + `git diff --stat` scoped to the cardrewards surface
(read-only git; autoCommit OFF — nothing committed this turn; the unrelated
concurrent BUG-064-002 / spec-084 working-tree changes were deliberately NOT
listed, touched, or staged):

```text
=== git status --short (cardrewards surface) ===
 M internal/cardrewards/store.go
 M internal/cardrewards/types.go
?? internal/cardrewards/reconcile.go
?? internal/cardrewards/reconcile_integration_test.go
?? internal/cardrewards/reconcile_test.go
=== git diff --stat (tracked cardrewards files) ===
 internal/cardrewards/store.go | 125 +++++++++++++++++++++++++++++++++++++++++
 internal/cardrewards/types.go |  17 ++++++
 2 files changed, 142 insertions(+)
=== reconcile files line counts ===
  382 internal/cardrewards/reconcile.go
  245 internal/cardrewards/reconcile_test.go
  382 internal/cardrewards/reconcile_integration_test.go
 1009 total
```

The Scope 06 surface is: `reconcile.go` (382 lines, NEW), `reconcile_test.go`
(245 lines, NEW unit), `reconcile_integration_test.go` (382 lines, NEW live-PG),
and the `store.go` (+125) / `types.go` (+17) additions. The working tree is left
ready for an orchestrator preservation commit of exactly this surface.

### Delivery — Scope 06 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` test/verify phase for Scope 06 was executed in **parent-expanded**
form by a single orchestrator agent — NO separate certified specialist
sub-agents (bubbles.implement/test/validate/audit) were dispatched or claimed.
Disclosed in `state.json.executionHistory`, consistent with Scopes 01–05. This
run did NOT write implementation code: the reconcile implementation already
existed on disk from a prior cut-off pass; this run RE-RAN every Scope 06 test
via the repo CLI, captured the previously-missing live-PG evidence (F06/F07),
confirmed all gates green, found NO implementation bug, and flipped Scope 06 →
Done. The unrelated concurrent BUG-064-002 / spec-084 work in the tree
(internal/assistant/*, internal/telegram/*, cmd/core/*, config/*, docs/*,
specs/064/084) was NOT read, modified, staged, or committed. Top-level `status`
stays `in_progress`: Scope 05 remains In Progress (E08 blocked-needs-live-Ollama)
and scopes 07–11 remain Not Started, so `completedScopes` becomes
["1","2","3","4","6"] and `currentScope` advances to 7. NOT committed (autoCommit
off) — the orchestrator owns the preservation checkpoint.

## Delivery — Scope 07: Optimizer & Monthly Recommendation Generation (2026-06-11)

> Scope 07 was delivered by a prior implement pass that was cut off **while the
> live-PG integration image build was still running**: the implementation
> (`internal/cardrewards/optimize.go`, `recommend.go`, the REST
> recommendation/generate/report endpoints in `internal/api/cardrewards.go`, plus
> the `service.go`/`store.go` additions) and the unit tests (G01–G05) already
> existed on disk, but the live-PG integration (G06/G07) and e2e-api (G08)
> evidence was never captured and the scope was never flipped. This run is a
> **VERIFY + CERTIFY** pass: every Scope 07 test was re-run via the repo CLI to
> completion, the previously-missing live-PG + live-stack evidence was captured,
> all gates confirmed green, and **NO implementation bug was found** — no
> implementation source was rewritten. The only working-tree write to a Scope 07
> file was `gofmt` whitespace on `tests/e2e/cardrewards_api_test.go` (the cut-off
> pass never formatted it); G08 was re-run after formatting and PASSED again.

### Files verified (Scope 07)

- `internal/cardrewards/optimize.go` (NEW) — effective-rate optimizer: base/rotating/offer/selection, shared-limit pools, ties, equivalents normalization, reasons recorded (Principle 8).
- `internal/cardrewards/optimize_test.go` (NEW) — unit G01–G05 + `NoBenefit_ReturnsNone` / `InactiveCardExcluded` edges.
- `internal/cardrewards/recommend.go` (NEW) — per-period recommendation generation honoring `starred_override`, audit run appended.
- `internal/cardrewards/recommend_test.go` (NEW, `//go:build integration`) — live-PG G06 + adversarial G07.
- `internal/api/cardrewards.go` (M) — REST `GET /api/card-recommendations`, `POST /api/card-recommendations/generate`, optimization-report endpoint.
- `internal/cardrewards/service.go`, `internal/cardrewards/store.go` (M) — recommender/optimizer wiring + recommendation upsert/list queries.
- `tests/e2e/cardrewards_api_test.go` (M) — e2e-api G08 (live stack); gofmt'd this run.

### Evidence — DoD 1: Implementation behavior complete (optimizer + recommender + REST + reasons)

No implementation source was rewritten this run; the behavior is proven by the G01–G08 scenario tests below (optimizer base/rotating/expired/shared-limit/equivalents; recommender per-category generation + starred-override preservation; REST endpoints; reasons recorded — asserted by G01 "records a reason" and G02 "reason cites the rotating benefit"). The only Scope 07 working-tree write was gofmt whitespace on the e2e test (no logic change):

```text
?? internal/cardrewards/optimize.go
?? internal/cardrewards/optimize_test.go
?? internal/cardrewards/recommend.go
?? internal/cardrewards/recommend_test.go
 M internal/api/cardrewards.go
 M internal/cardrewards/service.go
 M internal/cardrewards/store.go
 M tests/e2e/cardrewards_api_test.go
```

Behaviour cross-references: optimizer reasons + selection → DoD 2/3 block; per-category generation + starred override → DoD 4 block; REST endpoints round-trip → DoD 5 block.

### Evidence — DoD 2 (SCN-083-G01, G02, G03) + DoD 3 (SCN-083-G04, G05) — unit

Command: `./smackerel.sh test unit --go --go-run 'Optimize' --verbose`

```text
[go-unit] applying -run selector: Optimize
+ go test -v -run Optimize -count=1 ./...
=== RUN   TestOptimize_BaseRateHighestWins_G01
--- PASS: TestOptimize_BaseRateHighestWins_G01 (0.00s)
=== RUN   TestOptimize_ActiveRotatingBeatsBase_G02
--- PASS: TestOptimize_ActiveRotatingBeatsBase_G02 (0.00s)
=== RUN   TestOptimize_ExpiredRotatingIgnored_G03
--- PASS: TestOptimize_ExpiredRotatingIgnored_G03 (0.00s)
=== RUN   TestOptimize_SharedLimitPoolNotDoubleCounted_G04
--- PASS: TestOptimize_SharedLimitPoolNotDoubleCounted_G04 (0.00s)
=== RUN   TestOptimize_EquivalentsNormalizeBeforeMatching_G05
--- PASS: TestOptimize_EquivalentsNormalizeBeforeMatching_G05 (0.00s)
=== RUN   TestOptimize_NoBenefit_ReturnsNone
--- PASS: TestOptimize_NoBenefit_ReturnsNone (0.00s)
=== RUN   TestOptimize_InactiveCardExcluded
--- PASS: TestOptimize_InactiveCardExcluded (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.050s
[go-unit] go test ./... finished OK
@@@UNIT_EXIT=0@@@
```

G03 is the **adversarial** case (expired rotating benefit must NOT be used); G04 is the **shared-limit pool** case (combined limit counted once, not double-counted); G05 is the **equivalents** case (alias normalization before matching). All PASS.

### Evidence — DoD 4: SCN-083-G06 + adversarial SCN-083-G07 — integration (live PG)

Command: `./smackerel.sh test integration --go-run 'RecommendLivePG'` (disposable test stack, real PostgreSQL)

```text
 Container smackerel-test-postgres-1  Healthy
 Container smackerel-test-smackerel-core-1  Healthy
 Container smackerel-test-smackerel-ml-1  Healthy
go-integration: applying -run selector: RecommendLivePG
=== RUN   TestRecommendLivePG_PerCategoryGeneration_G06
--- PASS: TestRecommendLivePG_PerCategoryGeneration_G06 (0.05s)
=== RUN   TestRecommendLivePG_StarredOverridePreserved_G07
--- PASS: TestRecommendLivePG_StarredOverridePreserved_G07 (0.07s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.145s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-nats-data  Removing
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
@@@INTEG_EXIT=0@@@
```

G07 is the **adversarial** regression: a `starred_override` recommendation is PRESERVED over the optimizer's pick (the test fails with "starred_override was cleared — the manual override was overwritten (G07 regression)" if the override is overwritten). PASS. Disposable stack fully torn down (all containers + 3 volumes + network removed — ephemeral isolation).

### Evidence — DoD 5: SCN-083-G08 — e2e-api (live stack)

Command: `./smackerel.sh test e2e --go-run 'TestCardRewardsRecommendationsE2E_G08'` (live stack; core image rebuilt with the new REST endpoints). Run once before formatting and **re-run after the gofmt whitespace fix** so the certified file state is the one tested:

```text
 Container smackerel-test-smackerel-core-1  Healthy
 Container smackerel-test-smackerel-ml-1  Healthy
go-e2e: applying -run selector: TestCardRewardsRecommendationsE2E_G08
=== RUN   TestCardRewardsRecommendationsE2E_G08
--- PASS: TestCardRewardsRecommendationsE2E_G08 (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.515s
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
@@@E2E_RERUN_EXIT=0@@@
```

The G08 test declares a unique tracked category, adds a card with a 5% offer, `POST /api/card-recommendations/generate` for a unique period (`generated >= 1`), then `GET /api/card-recommendations?period=...` and the optimization-report endpoint, asserting they return the current period's data — real chi router, bearer auth, optimizer, recommender, and PostgreSQL (no mocks). First run `@@@E2E_EXIT=0@@@`; post-gofmt re-run `@@@E2E_RERUN_EXIT=0@@@` (shown above). Both PASS.

### Evidence — DoD 6: Build Quality Gate (check / lint / format / artifact-lint)

`./smackerel.sh check` →

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
@@@CHECK_EXIT=0@@@
```

`./smackerel.sh lint` →

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
@@@LINT_EXIT=0@@@
```

`./smackerel.sh format --check` first flagged the un-gofmt'd Scope 07 e2e test (the only unclean file in the whole tree — no BUG-064-002 file was listed), then `./smackerel.sh format` applied gofmt to that one file and `format --check` re-ran clean:

```text
tests/e2e/cardrewards_api_test.go
@@@FORMAT_EXIT=1@@@
65 files already formatted
@@@FORMAT_RECHECK_EXIT=0@@@
```

### Gate: artifact-lint — Scope 07

Command: `bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion`

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ Found DoD section in scopes.md
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Top-level status matches certification.status
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
@@@ARTIFACT_LINT_EXIT=0@@@
```

### Delivery — Scope 07 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` test/verify phase for Scope 07 was executed in **parent-expanded**
form by a single orchestrator agent — NO separate certified specialist sub-agents
(bubbles.implement/test/validate/audit) were dispatched or claimed. Disclosed in
`state.json.executionHistory`, consistent with Scopes 01–06. This run did NOT
write implementation code: the optimizer/recommender implementation and the REST
endpoints already existed on disk from the cut-off pass; this run RE-RAN every
Scope 07 test via the repo CLI, captured the previously-missing live-PG (G06/G07)
and e2e-api (G08) evidence, applied gofmt to the one unformatted e2e test file
(whitespace only; G08 re-verified after), confirmed all gates green, found NO
implementation bug, and flipped Scope 07 → Done. The unrelated concurrent
BUG-064-002 / spec-084 work in the tree (`internal/assistant/*`,
`internal/telegram/*`, `cmd/core/main.go`,
`cmd/core/wiring_assistant_openknowledge.go`, `config/*`, `docs/Operations.md`,
`specs/064`/`084`) was NOT read, modified, staged, or committed. Top-level
`status` stays `in_progress`: Scope 05 remains In Progress (E08
blocked-needs-live-Ollama) and scopes 08–11 remain Not Started, so
`completedScopes` becomes `["1","2","3","4","6","7"]` and `currentScope` advances
to 8. NOT committed (autoCommit off) — the orchestrator owns the preservation
checkpoint.

## Delivery — Scope 08: CalDAV Calendar Delivery (2026-06-11)

> Scope 08 was **implemented and tested in this run** (parent-expanded
> `full-delivery`, phases implement + test). It adds `internal/cardrewards/calendar.go`
> — a `CardCalendarBridge` over the shared `mealplan`-shaped `CalDAVClient`
> interface (design §7) — and its unit + live-PG integration tests. Google
> Calendar is the operator's PRIMARY card-rewards consumption surface (preserved
> from CCManager), so the bridge turns persisted monthly recommendations and open
> re-enrollment actions into stable-UID CalDAV events that UPDATE (never
> duplicate) on re-sync. The bridge reuses the existing `internal/connector/caldav`
> client + credentials at wire-time (Scope 09); no new Google Calendar client was
> invented.

### Files created / changed (Scope 08)

```text
?? internal/cardrewards/calendar.go
?? internal/cardrewards/calendar_test.go
?? internal/cardrewards/calendar_integration_test.go
```

- `internal/cardrewards/calendar.go` (NEW) — `CalDAVClient` interface (same shape as `mealplan.CalDAVClient`), `CardCalendarBridge`, stable UID scheme (`<prefix>-cardrec-<period>-<slug>` / `<prefix>-cardreenroll-<user_card_id>-<period>`), `SyncRecommendations` / `SyncReEnrollments` (input-taking, fake-client testable), `DeleteRecommendationEvent` (cleanup), and the store-driven `SyncPeriod` (persists `calendar_event_uid`, writes a `card_runs` `run_type="calendar_sync"` audit row).
- `internal/cardrewards/calendar_test.go` (NEW, unit, untagged) — H01–H05 + UID-scheme + non-month-period edge, against an in-memory FAKE of the EXTERNAL CalDAV server.
- `internal/cardrewards/calendar_integration_test.go` (NEW, `//go:build integration`) — H06 live-PG audit + UID round-trip + re-sync-no-duplicate.

Every other entry in `git status` (`cmd/core/*`, `internal/assistant/*`,
`internal/telegram/*`, `config/*`, `docs/Operations.md`, `specs/064-…/BUG-064-002/`,
`specs/084-…/`) is the pre-existing concurrent **BUG-064-002 / spec-084** change
set — NOT read, modified, or staged by this run.

### Evidence — DoD 1: Implementation behavior complete

`calendar.go` is new code delivered this run; the behavior is proven by the
H01–H06 scenario tests below: write a stable-UID event (H01), update-not-duplicate
on re-sync (H02, adversarial), re-enrollment reminder (H03), disabled-sync skips
writes but keeps the recommendation data (H04), delete cleans up the event (H05),
and the store-driven sync persists `calendar_event_uid` + writes the
`run_type="calendar_sync"` audit row recording `events_written` (H06, live PG).
Reuse of the `mealplan` CalDAVClient shape + the `caldav` credentials is per
design §7; scheduler/manual-trigger wiring is Scope 09's responsibility (scopes.md
Scope 09 `Depends On: …,08`).

### Evidence — DoD 2 (SCN-083-H01, H02) + DoD 3 (SCN-083-H03, H05) + DoD 4 (SCN-083-H04) — unit

Command: `./smackerel.sh test unit --go --go-run 'CardCalendarBridge' --verbose`

```text
[go-unit] applying -run selector: CardCalendarBridge
[go-unit] starting go test ./...
+ go test -v -run CardCalendarBridge -count=1 ./...
=== RUN   TestCardCalendarBridge_UIDSchemes
--- PASS: TestCardCalendarBridge_UIDSchemes (0.00s)
=== RUN   TestCardCalendarBridge_RecommendationEventStableUID_H01
--- PASS: TestCardCalendarBridge_RecommendationEventStableUID_H01 (0.00s)
=== RUN   TestCardCalendarBridge_ReSyncUpdatesSameUID_H02
--- PASS: TestCardCalendarBridge_ReSyncUpdatesSameUID_H02 (0.00s)
=== RUN   TestCardCalendarBridge_ReEnrollmentEvent_H03
--- PASS: TestCardCalendarBridge_ReEnrollmentEvent_H03 (0.00s)
=== RUN   TestCardCalendarBridge_DisabledSyncSkipsWritesKeepsData_H04
--- PASS: TestCardCalendarBridge_DisabledSyncSkipsWritesKeepsData_H04 (0.00s)
=== RUN   TestCardCalendarBridge_DeleteCleansUpEvent_H05
--- PASS: TestCardCalendarBridge_DeleteCleansUpEvent_H05 (0.00s)
=== RUN   TestCardCalendarBridge_NonMonthPeriodCountsFailed
2026/06/11 20:13:31 WARN card calendar: skipping recommendation with non-month period category=Restaurants period=Q3_2026 error="card calendar: recommendation period \"Q3_2026\" is not a YYYY-MM month label: parsing time \"Q3_2026\" as \"2006-01\": cannot parse \"Q3_2026\" as \"2006\""
--- PASS: TestCardCalendarBridge_NonMonthPeriodCountsFailed (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.018s
@@@UNIT_EXIT=0@@@
```

- **H01** (`…_RecommendationEventStableUID_H01`): a recommendation writes a CalDAV event under the STABLE UID `smackerel-cardrec-2026-06-restaurants`, summary `Restaurants: use Amex Gold (4%)`, category tag `smackerel-cardrewards`, props `X-SMACKEREL-CARDREC-ID`/`X-SMACKEREL-PERIOD`, start `2026-06-01 09:00 UTC`.
- **H02 (ADVERSARIAL)** (`…_ReSyncUpdatesSameUID_H02`): synced once at 3%, then re-synced at 5% — the fake server (keyed by UID, exactly like real CalDAV) shows `putCalls == 2` yet **exactly 1 event**, with the UPDATED `(5%)` summary. FAILS with "H02 REGRESSION: N events after re-sync, want exactly 1 (update, not duplicate)" if the UID were not stable.
- **H03** (`…_ReEnrollmentEvent_H03`): a pending re-enrollment writes a reminder under `smackerel-cardreenroll-uc-9-2026-Q3`.
- **H04** (`…_DisabledSyncSkipsWritesKeepsData_H04`): with `calendar_sync` disabled, `SyncRecommendations` returns `Skipped=true`, `EventsWritten=0`, a nil UID map, **zero** PutEvent calls (recommendations + re-enrollments), AND the recommendation data slice (the Web-UI data source) is left intact (2 rows, unchanged).
- **H05** (`…_DeleteCleansUpEvent_H05`): deleting a recommendation that carries a `calendar_event_uid` removes its event (`deleteCalls == 1`, 0 events left); a recommendation with no UID is a safe no-op.
- The non-month edge proves fail-loud per-event handling (no silent fallback to "now"); the `WARN` line above is that path firing.

### Evidence — DoD 5: SCN-083-H06 — integration (live PG)

Command: `./smackerel.sh test integration --go-run 'CardCalendarBridgeLivePG'` (disposable test stack, real PostgreSQL; only the EXTERNAL CalDAV server is faked)

```text
go-integration: applying -run selector: CardCalendarBridgeLivePG
=== RUN   TestCardCalendarBridgeLivePG_SyncRunAudited_H06
--- PASS: TestCardCalendarBridgeLivePG_SyncRunAudited_H06 (0.08s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.107s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
config-validate: ~/smackerel/config/generated/test.env.tmp OK
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
@@@INTEG_EXIT=0@@@
```

`SyncPeriod` reads the period's recommendations + open re-enrollments from live
PostgreSQL, writes events through the fake CalDAV boundary, and the test asserts:
(1) a `card_runs` row pinned by the returned `RunID` has `run_type="calendar_sync"`,
`status="success"`, and `events_written == res.EventsWritten` (SCN-083-H06);
(2) `res.EventsWritten >= 3` (2 carded recommendations + 1 re-enrollment);
(3) each synced recommendation persisted its stable `calendar_event_uid` (round-trip
via `GetRecommendation`), while the un-carded recommendation stored NO UID;
(4) a **re-sync** appends a SECOND `calendar_sync` run yet leaves the same single
event per UID (update-not-duplicate at the live-PG level, SCN-083-H02). Disposable
stack fully torn down — all containers + 3 volumes (`ollama-data`, `nats-data`,
`postgres-data`) + network removed (ephemeral isolation).

**External-boundary-fake confirmation:** `fakeCalDAVClient` models the EXTERNAL
calendar server (keyed by UID, just like a real CalDAV server). It is an
external-dependency fake explicitly allowed by the test-integrity policy; the
Store and PostgreSQL are REAL in H06, and NO internal component (Store, service,
business logic) is mocked.

### Evidence — DoD 6: Build Quality Gate (check / lint / format)

`./smackerel.sh check` →

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
@@@CHECK_EXIT=0@@@
```

`./smackerel.sh lint` →

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
@@@LINT_EXIT=0@@@
```

`./smackerel.sh format --check` (clean on the first try — the new `calendar*.go`
files were gofmt-correct as written; no BUG-064-002/spec-084 file required
formatting either):

```text
65 files already formatted
@@@FORMAT_EXIT=0@@@
```

No `${VAR:-default}` runtime config fallback was introduced (smackerel-no-defaults):
`calendar_sync` and `calendar_uid_prefix` are read from the Scope 01 fail-loud SST
loader and passed into the bridge constructor; the bridge invents no config
defaults. The only non-config "degradation" is the human-readable calendar summary
falling back to the stable card id when a card name is unexpectedly blank — a
display string, not a config value.

### Gate: artifact-lint — Scope 08

Command: `bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion`

```text
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: full-delivery
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' — see scope-workflow.md state.json canonical schema v2
ℹ️  Workflow mode 'full-delivery' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: Summary
✅ report.md contains section matching: Completion Statement
✅ report.md contains section matching: Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
@@@ARTIFACT_LINT_EXIT=0@@@
```

The `scopeProgress` deprecation is a non-blocking WARNING carried by this spec
since Scope 01 (every scope uses `scopeProgress`); it does not affect the PASS and
is out of Scope 08's remit. Top-level `status` correctly stays `in_progress`
(Scope 05 In Progress, scopes 09–11 Not Started).

### Delivery — Scope 08 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` implement + test phases for Scope 08 were executed in
**parent-expanded** form by a single orchestrator agent — NO separate certified
specialist sub-agents (bubbles.implement/test/validate/audit) were dispatched or
claimed. Disclosed in `state.json.executionHistory`, consistent with Scopes 01–07.
This run WROTE new implementation code (`internal/cardrewards/calendar.go` and its
two test files); it RAN every Scope 08 test (unit H01–H05 + the live-PG H06
integration) to completion via the repo CLI and captured the output above before
certifying. The unrelated concurrent **BUG-064-002 / spec-084** work in the tree
(`internal/assistant/*`, `internal/telegram/*`, `cmd/core/main.go`,
`cmd/core/wiring_assistant_openknowledge.go`,
`cmd/core/openknowledge_prompt_contract_test.go`,
`config/prompt_contracts/open_knowledge.yaml`, `config/smackerel.yaml`,
`docs/Operations.md`, `specs/064`/`084`) was NOT read, modified, staged, or
committed — the only working-tree additions this run are the three new
`internal/cardrewards/calendar*.go` files. No `card_rewards` feature-flag bundle
edit (`config/feature-flags.mvp.yaml`) — bubbles.train-owned. Top-level `status`
stays `in_progress`: Scope 05 remains In Progress (E08 blocked-needs-live-Ollama)
and scopes 09–11 remain Not Started, so `completedScopes` becomes
`["1","2","3","4","6","7","8"]` and `currentScope` advances to 9. NOT committed
(autoCommit off) — the orchestrator owns the preservation checkpoint.

## Delivery — Scope 09: Scheduler Jobs & Manual Triggers (2026-06-11)

> Scope 09 was **VERIFIED + CERTIFIED in this run** (parent-expanded, see the
> execution-model note below). A prior implement pass had completed the
> implementation + unit tests but was **cut off while the live-PG integration
> tests were running**, so it never captured them and never certified. This run
> did **NOT rewrite implementation code** — it re-ran the full Scope 09 test set
> via the repo CLI to completion, captured the previously-missing live-PG
> evidence (I03/I04/I05/I06), confirmed every gate green, and found **NO
> implementation bug**. The only working-tree write to a Scope 09 file was
> `gofmt -w` whitespace on the two un-formatted files the cut-off pass left
> behind (`cmd/core/wiring_cardrewards_scheduler.go`,
> `internal/scheduler/cardrewards_test.go`) — directly analogous to the Scope 07
> gofmt fix.

### Files created / changed (Scope 09)

| File | State | Purpose |
|------|-------|---------|
| `internal/cardrewards/pipeline.go` | new | Shared refresh/recommend code path: `RunDailyRefresh` (sync→extract→reconcile→lifecycle) + `RunMonthlyRecommend` (optimize→recommend→calendar sync); the SINGLE path both cron jobs AND manual triggers drive (NFR-CR-005); fail-loud `NewPipeline`; idempotent. |
| `internal/cardrewards/pipeline_integration_test.go` | new | Live-PG T-09-02/T-09-03 (`//go:build integration`): I03/I04/I05/I06 with real Store+DB; only the source website + ML sidecar (503, no model) are external-boundary fakes. |
| `internal/scheduler/cardrewards.go` | new | Registers `card_rewards_refresh` (scrape cron) + `card_rewards_recommend` (monthly cron) via the standard `AddFunc`+`runGuarded` pattern; `TriggerCardRewardsRefreshNow`/`TriggerCardRewardsRecommendNow` reuse the SAME pipeline methods with `trigger="manual"` under the same overlap guards. |
| `internal/scheduler/cardrewards_test.go` | new | Unit T-09-01: I01/I02 jobs-on-configured-crons (+ adversarial no-swap), scheduled→`scheduled` trigger, I05 manual-reuse wiring, error propagation, nil-pipeline + empty-cron fail-loud-skip. |
| `cmd/core/wiring_cardrewards_scheduler.go` | new | Constructs the pipeline from SST config + the connected connector and calls `SetCardRewardsJobs`; WARN-and-skips when disabled / pool-or-connector unavailable / cron empty (fail-loud, no default). |
| `internal/cardrewards/store.go` | modified (+29) | `ListObservationRefs` — drives reconcile off ALL stored observation refs (not just this run's), which is what makes a re-run idempotent (I06). |
| `internal/scheduler/scheduler.go` | modified (+13) | Scheduler struct fields (`cardRewardsPipeline`/`cardRewardsScrapeCron`/`cardRewardsRecommendCron`/`cardRewardsJobs` + two overlap mutexes) and the `scheduleCardRewardsJobs()` call in `Start()`. |
| `cmd/core/main.go` | modified (1 line) | The single call-site line `wireCardRewardsScheduler(cfg, svc, sched)` after `wireLegacyRetirementScheduler`. The concurrent spec-084 `WriteTimeout 1800→3600` hunk in this file is NOT mine and was left byte-identical (hash unchanged before/after — see DoD 5). |

### Evidence — DoD 1: Implementation behavior complete

Daily refresh + monthly recommend jobs register on their configured crons; manual
triggers reuse the SAME pipeline methods (only the trigger label differs); re-runs
are idempotent. Behavior is proven end-to-end by the I01–I06 scenarios below (unit
wiring + live-PG composite runs). No implementation code was rewritten this run;
the source on disk (`pipeline.go`, `scheduler/cardrewards.go`,
`wiring_cardrewards_scheduler.go`, the `ListObservationRefs` store method, the
`scheduler.go` wiring) is unchanged except gofmt whitespace on the two files noted
above. The shared-code-path guarantee (NFR-CR-005) is structural: both the cron
callbacks (`runCardRewardsRefreshJob`/`runCardRewardsRecommendJob`) and the manual
triggers (`TriggerCardRewardsRefreshNow`/`TriggerCardRewardsRecommendNow`) call the
identical `Pipeline.RunDailyRefresh` / `Pipeline.RunMonthlyRecommend` methods —
there is no second, manual-only path that could drift.

### Evidence — DoD 2: SCN-083-I01 + I02 (jobs registered on crons) — unit

**Command:** `./smackerel.sh test unit --go --go-run 'CardRewards' --verbose` — exit 0

```text
=== RUN   TestCardRewardsJobsRegisteredOnConfiguredCrons_I01_I02
2026/06/11 21:16:44 INFO card_rewards_refresh scheduled cron="0 6 * * *"
2026/06/11 21:16:44 INFO card_rewards_recommend scheduled cron="0 7 1 * *"
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsJobsRegisteredOnConfiguredCrons_I01_I02 (0.00s)
=== RUN   TestCardRewardsScheduledJobsUseScheduledTrigger
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsScheduledJobsUseScheduledTrigger (0.00s)
=== RUN   TestCardRewardsManualTriggersReuseSameMethodsWithManualTrigger_I05
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsManualTriggersReuseSameMethodsWithManualTrigger_I05 (0.00s)
=== RUN   TestCardRewardsManualTriggerPropagatesPipelineError
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsManualTriggerPropagatesPipelineError (0.00s)
=== RUN   TestCardRewardsJobsNotRegisteredWhenPipelineNil
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsJobsNotRegisteredWhenPipelineNil (0.00s)
=== RUN   TestCardRewardsEmptyCronSkipsThatJob
2026/06/11 21:16:44 INFO card_rewards_refresh scheduled cron="0 6 * * *"
2026/06/11 21:16:44 INFO scheduler stopped cleanly
--- PASS: TestCardRewardsEmptyCronSkipsThatJob (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/scheduler       0.037s
[go-unit] go test ./... finished OK
@@@UNIT_EXIT=0@@@
```

I01 (`card_rewards_refresh` on the scrape cron) and I02 (`card_rewards_recommend`
on the monthly cron) both register on EXACTLY their configured crons, with the
ADVERSARIAL no-swap assertions inside the test proving a regression that swapped
the crons would fail. The config package's `TestLoadCardRewardsConfig_*` (14
subtests, all PASS) also matched the `CardRewards` selector and is shown in the
full transcript — those belong to Scope 01 and are green here as a side effect.

### Evidence — DoD 3: SCN-083-I03 + I04 (full daily + monthly pipelines audited) — integration (live PG)

**Command:** `./smackerel.sh test integration --go-run 'CardRewardsPipelineLivePG'` — exit 0 (disposable test stack, live Postgres)

```text
=== RUN   TestCardRewardsPipelineLivePG_DailyRefreshFullPipeline_I03
2026/06/11 21:19:43 WARN card-rewards extraction sidecar error — flagging target for verification card_id=cr-int-...-discover-it period=Q3_2026 source=TestSource error="cardrewards sidecar: /extract-card-categories returned HTTP 503: {\"error\":\"no model available on disposable test stack\"}"
2026/06/11 21:19:43 INFO card-rewards daily refresh complete trigger=scheduled artifacts=1 extracted=0 reconciled=1 lifecycle_transitioned=0
--- PASS: TestCardRewardsPipelineLivePG_DailyRefreshFullPipeline_I03 (0.09s)
=== RUN   TestCardRewardsPipelineLivePG_MonthlyRecommendFullPipeline_I04
2026/06/11 21:19:43 INFO card-rewards monthly recommend complete trigger=scheduled period=2026-08 generated=1 preserved_override=0 calendar_events=1
--- PASS: TestCardRewardsPipelineLivePG_MonthlyRecommendFullPipeline_I04 (0.06s)
```

I03 runs the full daily pipeline on live PG and asserts the `card_runs` audit
trail: exactly 1 scheduled `scrape` run (status success), 1 `extract` run (status
**partial** — the sidecar genuinely returned HTTP 503 with no model, so it flagged
targets and **fabricated zero** observations: observation count unchanged), and 2
`reconcile` runs (reconcile + advance-lifecycle), with reconcile genuinely merging
the seeded observations into exactly 1 authoritative `rotating_categories` row.
I04 runs optimize→recommend→calendar-sync: 1 `card_recommendations` row pointing at
the wallet card (rate 4), 1 scheduled `optimize` run, 1 `calendar_sync` run, and a
CalDAV event under the stable recommendation UID persisted back onto the rec row.
(See the blocked-needs-live-Ollama note below: the live-LLM extract leg is the only
piece deferred; the seeded-observation path proves the wiring + audit + reconcile.)

### Evidence — DoD 4: SCN-083-I05 + I06 (manual trigger reuse + idempotency) — integration (live PG)

**Command:** same integration run (`CardRewardsPipelineLivePG`) — exit 0

```text
=== RUN   TestCardRewardsPipelineLivePG_ManualTriggerReusesRefresh_I05
2026/06/11 21:19:43 WARN card-rewards extraction sidecar error — flagging target for verification card_id=cr-int-...-discover-it period=Q3_2026 source=TestSource error="...HTTP 503..."
2026/06/11 21:19:43 WARN card-rewards extraction sidecar error — flagging target for verification card_id=cr-int-...-discover-it period=Q3_2026 source=TestSource error="...HTTP 503..."
2026/06/11 21:19:43 INFO card-rewards daily refresh complete trigger=manual artifacts=1 extracted=0 reconciled=2 lifecycle_transitioned=0
--- PASS: TestCardRewardsPipelineLivePG_ManualTriggerReusesRefresh_I05 (0.06s)
=== RUN   TestCardRewardsPipelineLivePG_ReRunIdempotent_I06
2026/06/11 21:19:43 INFO card-rewards daily refresh complete trigger=scheduled artifacts=1 extracted=0 reconciled=3 lifecycle_transitioned=0
2026/06/11 21:19:43 INFO card-rewards daily refresh complete trigger=scheduled artifacts=1 extracted=0 reconciled=3 lifecycle_transitioned=0
2026/06/11 21:19:43 INFO card-rewards monthly recommend complete trigger=scheduled period=2026-08 generated=2 preserved_override=0 calendar_events=2
2026/06/11 21:19:43 INFO card-rewards monthly recommend complete trigger=scheduled period=2026-08 generated=2 preserved_override=0 calendar_events=2
--- PASS: TestCardRewardsPipelineLivePG_ReRunIdempotent_I06 (0.12s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.352s
PASS: go-integration
@@@INTEG_EXIT=0@@@
```

I05 proves the manual "scrape now" trigger reuses the SAME `RunDailyRefresh` code
path with `trigger="manual"`: the `card_runs.trigger` column carries `manual`
(manual scrape-run delta = 1, scheduled scrape-run delta = 0 — it does NOT mislabel
as scheduled) and the manual refresh genuinely reconciled on live PG (1 rotating
row). I06 (**ADVERSARIAL idempotency**) re-runs both jobs: a second
`RunDailyRefresh` keeps exactly 1 `rotating_categories` row (a non-idempotent
reconcile would show 2 — the assertion FAILS if regressed), and a second
`RunMonthlyRecommend` keeps exactly 1 `card_recommendations` row while issuing a
fresh `PutEvent` against the SAME stable CalDAV UID (event UPDATED, not duplicated;
the test FAILS if `putCalls` does not increase OR if the row count grows). The
disposable stack was torn down cleanly on exit (all containers + 3 volumes
[postgres-data, nats-data, ollama-data] + network removed — ephemeral isolation).

### Evidence — DoD 5: Build Quality Gate (check / lint / format / artifact-lint)

**`./smackerel.sh check` — exit 0:**

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.1950139 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
@@@CHECK_EXIT=0@@@
```

**`./smackerel.sh lint` — exit 0 (golangci-lint + web validation):**

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: web/extension/manifest.json
  OK: web/extension/manifest.firefox.json
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
@@@LINT_EXIT=0@@@
```

**`./smackerel.sh format --check` — first flagged the two un-gofmt'd Scope 09 files (exit 1), then `./smackerel.sh format` applied `gofmt -w`, then re-check exit 0:**

```text
# ./smackerel.sh format --check   (first pass — un-formatted files from the cut-off implement pass)
cmd/core/wiring_cardrewards_scheduler.go
internal/scheduler/cardrewards_test.go
@@@FORMAT_EXIT=1@@@

# ./smackerel.sh format            (gofmt -w is silent for Go; ruff reports the Python files)
65 files left unchanged
@@@FORMAT_APPLY_EXIT=0@@@

# ./smackerel.sh format --check   (re-check — clean)
65 files already formatted
@@@FORMAT_RECHECK_EXIT=0@@@
```

`go-format.sh` apply mode is `gofmt -w cmd internal tests/*.go` (silent — it
rewrote only the two non-compliant Scope 09 files); the "65 files" counts are
`ruff` (Python) output and were always clean (my changes are Go-only). The shared
`cmd/core/main.go` was **NOT** touched by format — proven by an identical content
hash before and after:

```text
@@@MAINGO_SHA_BEFORE=dcee8972c1bb4f6543e3970fe127b716f0ff0b44@@@
@@@MAINGO_SHA_AFTER=dcee8972c1bb4f6543e3970fe127b716f0ff0b44@@@
```

**`bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion` — exit 0:**

```text
✅ Required artifact exists: spec.md / design.md / scopes.md / report.md / uservalidation.md / state.json
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ uservalidation checklist has checked-by-default entries
✅ Detected state.json status: in_progress    workflowMode: full-delivery
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' (pre-existing, carried from earlier scopes — non-blocking)
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md / report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
@@@ARTIFACT_LINT_EXIT=0@@@
```

NFR-CR-005 (manual + scheduled paths share code) is satisfied structurally and
proven by I05; zero warnings in build/check/lint; artifact-lint clean.

### Blocked-needs-live-Ollama note (NOT a Scope 09 blocker)

The I03 daily-refresh extract leg shows `extracted=0` because the disposable test
stack has **no live model** — a real `HTTPSidecarExtractor` round-trips to a local
httptest server returning HTTP 503, so the Scope 05 orchestrator records a
**partial** extract run and flags targets for verification, **fabricating nothing**.
This is the same deferred-live-Ollama state already recorded against **Scope 05's
E08** item. It is **NOT a Scope 09 blocker**: Scope 09's scenarios are about the
scheduler wiring, the composite-pipeline composition, the `card_runs` audit trail,
and idempotency — all of which are fully proven here via the **seeded-observation
path** (`PersistExtractionRun`), so reconcile→lifecycle→optimize→recommend→calendar
are genuinely exercised on live PG without requiring a live LLM. The only piece
that genuinely needs live Ollama is a model producing observations from page text,
which is Scope 05's remit, not Scope 09's. No live-LLM success was fabricated.

### Delivery — Scope 09 execution model (transparency)

The `runSubagent`/`agent` tool was **unavailable** in this runtime, so the
`full-delivery` test/verify phase for Scope 09 was executed in **parent-expanded**
form by a single orchestrator agent — NO separate certified specialist sub-agents
(bubbles.implement/test/validate/audit) were dispatched or claimed. Disclosed in
`state.json.executionHistory`, consistent with Scopes 01–08. This run did **NOT**
write implementation code; it re-ran the full Scope 09 test set via the repo CLI to
completion, captured the previously-missing live-PG evidence, confirmed all gates
green, and found **NO** implementation bug — the only working-tree write was
`gofmt -w` whitespace on `cmd/core/wiring_cardrewards_scheduler.go` and
`internal/scheduler/cardrewards_test.go`. The unrelated concurrent
**BUG-064-002 / spec-084** work in the tree (`internal/assistant/*`,
`internal/telegram/*`, `cmd/core/main.go` [spec-084 hunk only],
`cmd/core/wiring_assistant_openknowledge.go`,
`cmd/core/openknowledge_prompt_contract_test.go`,
`config/prompt_contracts/open_knowledge.yaml`, `config/smackerel.yaml`,
`docs/Operations.md`, `specs/064`/`084`) was NOT read, modified, staged, or
committed. No `card_rewards` feature-flag bundle edit (`config/feature-flags.mvp.yaml`)
— bubbles.train-owned. Top-level `status` stays `in_progress`: Scope 05 remains In
Progress (E08 blocked-needs-live-Ollama) and scopes 10–11 remain Not Started, so
`completedScopes` becomes `["1","2","3","4","6","7","8","9"]` and `currentScope`
advances to 10. NOT committed (autoCommit off) — the orchestrator owns the
preservation checkpoint (it will hunk-stage the single `main.go` card-rewards line
separately from the spec-084 hunk).

---

## Delivery — Scope 10: Web UI — Wallet, Offers, Selections, Bonuses, Categories (2026-06-12)

Scope 10 delivers the server-rendered card-rewards Web UI with full CRUD parity
to the standalone CCManager app (FR-CR-016) — the user-facing half of the
absorption that lets the operator retire CCManager. Pages follow the existing
`internal/web` Go `html/template` + go-chi paradigm (NFR-CR-006), behind the
existing `webAuthMiddleware` + global CSP, styled with the shared design-token
palette (`var(--…)`, no hardcoded colors), and use plain Post/Redirect/Get
forms (no client JS) so they are strictly CSP-clean.

### Execution model (transparency)

`runSubagent`/`agent` was **unavailable** in this runtime, so the `full-delivery`
implement+test phases for Scope 10 ran in **parent-expanded** form by a single
orchestrator agent — NO separate certified specialist sub-agents
(bubbles.implement/test/validate/audit) were dispatched or claimed. Disclosed in
`state.json.executionHistory`, consistent with Scopes 01–09. The unrelated
concurrent **BUG-064-002 / spec-084** work in the tree (`internal/assistant/*`,
`internal/telegram/*`, `cmd/core/main.go` [spec-084 WriteTimeout hunk only],
`cmd/core/wiring_assistant_openknowledge.go`,
`cmd/core/openknowledge_prompt_contract_test.go`,
`config/prompt_contracts/open_knowledge.yaml`, `config/smackerel.yaml`,
`docs/Operations.md`, `specs/064`/`084`) was NOT read, modified, staged, or
committed (verified by `git diff` of `main.go` + `config/smackerel.yaml` — both
contain ONLY spec-084 content). `main.go` was NOT touched: the web handler is
wired in `cmd/core/wiring.go::wireCardRewardsHandler` (which already runs before
`api.NewRouter`), reusing the same `*cardrewards.Service` as the REST handler.
No `card_rewards` feature-flag bundle edit (`config/feature-flags.mvp.yaml`) —
bubbles.train-owned.

### Files created / changed (Scope 10)

**Created:**
- `internal/web/cardrewards.go` — `CardRewardsWebHandler` (self-contained template set with a script-free design-token `head`/`foot`), `RegisterRoutes`, and all wallet/offers/selections/bonuses/categories page + action handlers (Post/Redirect/Get).
- `internal/web/cardrewards_templates.go` — `cardRewardsTemplates`: local `head`/`foot` (design tokens, no scripts), `cardrewards-nav`, and the 10 page templates with rich `data-*` hooks for assertion.
- `internal/cardrewards/store_crud_test.go` — `//go:build integration` live-PG tests for the new offer/selection/bonus Get/Update/Delete/List store methods.
- `web/pwa/tests/cardrewards_wallet.spec.ts` — e2e-ui J01–J05.
- `web/pwa/tests/cardrewards_offers_selections.spec.ts` — e2e-ui J06, J07.
- `web/pwa/tests/cardrewards_categories.spec.ts` — e2e-ui J08.

**Modified (additive only):**
- `internal/cardrewards/store.go` — `GetOffer`/`UpdateOffer`/`DeleteOffer`/`ListOffers`, `GetSelection`/`UpdateSelection`/`ListSelections`, `GetSignupBonus`/`UpdateSignupBonus`/`ListBonuses` (mirror existing pgx patterns).
- `internal/cardrewards/service.go` — `ErrOfferNotFound`/`ErrSelectionNotFound`/`ErrBonusNotFound` sentinels; `GetOffer`/`UpdateOffer`/`DeleteOffer`/`ListOffers`, `GetSelection`/`UpdateSelection`/`ListSelections`, `GetBonus`/`UpdateBonus`/`ListBonuses`, `ListCatalog` (validation + delegate).
- `internal/api/router.go` — `CardRewardsWebUI` interface + mount block behind `webAuthMiddleware` (mirrors the `AgentAdminUI` precedent; mounted only when non-nil so router unit tests are unaffected).
- `internal/api/health.go` — `Dependencies.CardRewardsWebHandler CardRewardsWebUI` field.
- `cmd/core/wiring.go` — `wireCardRewardsHandler` also builds `web.NewCardRewardsWebHandler(service)` and assigns `deps.CardRewardsWebHandler`.

### Path reconciliation note (Test Plan vs harness)

The scopes.md Test Plan names the specs `tests/e2e-ui/cardrewards_*.spec.ts`,
but the repo's actual e2e-ui harness (`./smackerel.sh test e2e-ui` →
`scripts/runtime/web-e2e-ui.sh`) discovers tests via `web/pwa/playwright.config.ts`
(`testDir: "tests"`, `testMatch: "**/*.spec.ts"`), i.e. only
`web/pwa/tests/**/*.spec.ts` are ever executed. The specs were therefore placed
in `web/pwa/tests/` so they are actually run by the sanctioned command; placing
them under `tests/e2e-ui/` would mean they are never discovered.

### Pre-existing out-of-scope finding (shared head ↔ CSP htmx mismatch)

The first e2e-ui run surfaced a **pre-existing** inconsistency: the shared
`internal/web` `head` template loads htmx from `https://unpkg.com/htmx.org@1.9.12`
(no trailing slash), but the global CSP `script-src` allow-lists
`https://unpkg.com/htmx.org@1.9.12/` (WITH trailing slash) — so the browser
refuses to load htmx on every page that uses the shared head (e.g. the
htmx-driven recommendations page). This is owned by the web/spec-044/057 area,
not Scope 10. Scope 10 stays scoped: the card-rewards pages use plain forms and
a **script-free** head, so they are CSP-clean without touching the shared head
or CSP. Recorded here as a finding for the owning area to address separately.

### Evidence — DoD 1: Implementation behavior complete

Server-rendered wallet/offers/selections/bonuses/categories pages with full CRUD
parity, behind `webAuthMiddleware` + global CSP, design tokens (no hardcoded
colors). Proven end-to-end by the 7 live-stack Playwright scenarios (DoD 2–4
below — every page renders, every CRUD mutation persists and re-renders) plus 3
live-PG store-CRUD integration tests. `./smackerel.sh check` (compile + SST
sync) exit 0:

```
config-validate: /home/.../config/generated/dev.env.tmp.3188712 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
@@@CHECK_EXIT=0@@@
```

New domain CRUD (offer/selection/bonus Get/Update/Delete/List) — live-PG
integration, `./smackerel.sh test integration --go-run CardRewardsStoreCRUD`:

```
go-integration: applying -run selector: CardRewardsStoreCRUD
=== RUN   TestCardRewardsStoreCRUD_OfferLifecycle_J06
--- PASS: TestCardRewardsStoreCRUD_OfferLifecycle_J06 (0.05s)
=== RUN   TestCardRewardsStoreCRUD_SelectionLifecycle_J07
--- PASS: TestCardRewardsStoreCRUD_SelectionLifecycle_J07 (0.04s)
=== RUN   TestCardRewardsStoreCRUD_BonusLifecycle
--- PASS: TestCardRewardsStoreCRUD_BonusLifecycle (0.04s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.172s
PASS: go-integration
@@@INTEG_EXIT=0@@@
```

The OfferLifecycle test asserts the J06 edit round-trips (title/category/rate/
activated change persisted via Get→Update→Get), ListOffers includes the row,
delete removes it, and a second delete of the missing row returns `(false, nil)`
(not an error). SelectionLifecycle asserts a tier-1→tier-2 edit round-trips.
BonusLifecycle asserts a spend-progress update meeting the requirement
round-trips with `met=true`. Stack torn down clean (ephemeral isolation).

### Evidence — DoD 2: SCN-083-J01..J05 (wallet CRUD incl. discovery/custom/note/toggle) — e2e-ui, live stack, no interception

`./smackerel.sh test e2e-ui cardrewards` — 7 scenarios, real headless Chromium
against the freshly-rebuilt disposable stack (`SMACKEREL_BASE_URL` = core), real
`/v1/web/login` auth, real `/api/cards` seeding. J01 (wallet lists nickname,
type, note, active) + J03 (add custom) are covered by scenario 2; J02 (discovery)
by scenario 4; J04 (edit + note persists on reload) by scenario 6; J05 (toggle
activation) by scenario 7:

```
Running 7 tests using 3 workers

  ✓  1 …s › SCN-083-J06 — add and edit an offer with a shared limit group (2.1s)
  ✓  2 …01 — add a custom card; wallet lists nickname, type, note, active (1.5s)
  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (1.9s)
  ✓  4 … Rewards Wallet › SCN-083-J02 — add a catalog card via discovery (790ms)
  ✓  5 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (852ms)
  ✓  6 …t › SCN-083-J04 — edit a card and add a note; persists on reload (796ms)
  ✓  7 …— Card Rewards Wallet › SCN-083-J05 — toggle card activation off (513ms)

  7 passed (5.2s)
@@@E2EUI_EXIT=0@@@
```

No-interception scan (DoD requirement — must be empty for the card-rewards specs):

```
# grep -rn 'page\.route|context\.route|intercept(|cy\.intercept|msw|nock|wiremock' web/pwa/tests/cardrewards_*.spec.ts
(no matches — scan clean)
```

Each scenario authenticates via the real login endpoint (adversarial guard:
`expect(page).toHaveURL(/\/cards\/wallet…/)` ensures the authed page is NOT
bounced to `/login`), asserts user-visible state (rendered nickname/type/note/
`data-card-status`, not just URL), reloads to prove persistence (J04), and ends
with `assertNoCSPViolations(page)` (CSP-clean). No silent-pass bailouts.

### Evidence — DoD 3: SCN-083-J06 (offer shared-limit) + SCN-083-J07 (tiered selection) — e2e-ui, live stack

From the same `7 passed` run above — scenario 1 (J06) and scenario 5 (J07):

```
  ✓  1 …s › SCN-083-J06 — add and edit an offer with a shared limit group (2.1s)
  ✓  5 …e 10 — Offers & Selections › SCN-083-J07 — tiered selection save (852ms)
```

J06 adds an offer with a `shared_limit_group`, asserts the
`data-shared-limit-group` tag renders, opens the edit form (asserting the title
AND the shared-limit-group are pre-filled = round-trip), edits title+rate,
reloads, and asserts the edited title + preserved group + new rate persist. J07
saves tier-1 + tier-2 categories in one submit, reloads, and asserts BOTH a
`data-selection-tier="1"` row and a `data-selection-tier="2"` row re-render with
the correct categories and period.

### Evidence — DoD 4: SCN-083-J08 (category management) — e2e-ui, live stack

From the same `7 passed` run — scenario 3 (J08):

```
  ✓  3 …s › SCN-083-J08 — manage category names, equivalents, and starred (1.9s)
```

J08 adds a starred category with one equivalent (asserts the row renders with the
equivalent + `data-starred="true"`), then re-submits the SAME canonical name with
a second equivalent (idempotent upsert), reloads, and asserts the row is NOT
duplicated (`toHaveCount(1)`) and reflects BOTH equivalents + starred — proving
the change lands in `category_aliases`. (Dashboard ordering itself is Scope 11.)

### Evidence — DoD 5: Build Quality Gate

**`./smackerel.sh lint` — Go (golangci-lint) + web TS validation, exit 0:**

```
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
@@@LINT_EXIT=0@@@
```

**`./smackerel.sh format --check` — flagged the new file, `./smackerel.sh format`
applied gofmt, re-check clean:**

```
# first pass
internal/web/cardrewards.go
@@@FORMAT_EXIT=1@@@
# after ./smackerel.sh format (gofmt -w) and a later self-contained-head edit:
65 files already formatted
@@@FORMAT_RECHECK_EXIT=0@@@
```

**Docker bundle freshness (UI is Docker-served; the binary embeds the Go
templates).** The e2e-ui lane (project `smackerel-test-e2e-ui`) does not
auto-rebuild on source change, so the stale core image from the first run was
explicitly removed to force a fresh build:

```
Untagged: smackerel-test-e2e-ui-smackerel-core:latest
Deleted: sha256:3b061689d5b3f31376c1dfd72d560f8cd827ca61cd628e4befce7405044fb980
@@@RM_EXIT=0@@@
CORE_IMAGE_ABSENT_CONFIRMED
```

The re-run then rebuilt the core image **fresh from current source** and the
container was recreated and became Healthy before the specs ran — proving the
served pages came from the new binary (a stale binary would 404 `/cards/*` and
fail the no-bailout assertions):

```
#14 [smackerel-core builder 6/7] COPY . .
#14 DONE 17.7s
#15 [smackerel-core builder 7/7] RUN ... go build ... -o /bin/smackerel-core ./cmd/core
#15 DONE 43.2s
#19 naming to docker.io/library/smackerel-test-e2e-ui-smackerel-core 0.5s done
 smackerel-core  Built
 Container smackerel-test-e2e-ui-smackerel-core-1  Started
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy
```

**Artifact lint — `bash .github/bubbles/scripts/artifact-lint.sh specs/083-card-rewards-companion`:**

```
✅ Required artifact exists: spec.md / design.md / uservalidation.md / state.json / scopes.md / report.md
✅ Found DoD section in scopes.md
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ Top-level status matches certification.status
⚠️  state.json uses deprecated field 'scopeProgress' (pre-existing across all scopes; non-blocking)
ℹ️  Workflow mode 'full-delivery' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: Summary / Completion Statement / Test Evidence

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
@@@ARTIFACT_LINT_EXIT=0@@@
```

### Completion Statement — Scope 10

All 5 DoD items are `[x]` with inline raw evidence above. Scope 10 is **Done**:
the card-rewards Web UI delivers full wallet/offers/selections/bonuses/categories
CRUD parity (J01–J08) proven on the live disposable stack with no request
interception, CSP-clean, Docker-bundle-fresh, and behind the existing auth/CSP.
Top-level spec status remains `in_progress` (Scope 05 In Progress —
blocked-needs-live-Ollama; Scope 11 Not Started). `completedScopes` =
`[1,2,3,4,6,7,8,9,10]`, `currentScope` = 11. Not committed (autoCommit off) —
the orchestrator owns the preservation checkpoint.


