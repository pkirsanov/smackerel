# Scopes: 083 Card Rewards Companion

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

> **Delivery in progress (`full-delivery`).** Scopes 01–02 are **Done**
> (implemented + validated with real evidence in [report.md](report.md) →
> "Delivery — Scope 01" / "Delivery — Scope 02"). Scopes 03–11 remain **Not
> Started**; their DoD items are unchecked. Scenario IDs use the
> `SCN-083-<LETTER><NN>` convention (letter = scope) so they match the default
> artifact-lint pattern `SCN-[0-9]{3}-[A-Z][0-9]{2}`.

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Config SST & Migration Schema | P0 | — | Config, PostgreSQL | Done |
| 02 | Card Domain Store, Types & CRUD API | P0 | 01 | Go Core, PostgreSQL, REST API | Done |
| 03 | Data Migration: CCManager JSON → PostgreSQL | P0 | 02 | Go Core, one-time importer | Not Started |
| 04 | Card-Rewards Source Connector | P1 | 01, 02 | `internal/connector/cardrewards` | Not Started |
| 05 | LLM Category Extraction (replaces regex) | P0 | 04 | `internal/cardrewards/extract` (Go orchestrator + schema-validate) + `ml/app/card_categories.py` (sidecar model call, C2) | Not Started |
| 06 | Multi-Source Reconciliation & Lifecycle | P1 | 05 | `internal/cardrewards/reconcile`, PostgreSQL | Done |
| 07 | Optimizer & Monthly Recommendation Generation | P1 | 02, 06 | `internal/cardrewards/optimize`, REST API | Done |
| 08 | CalDAV Calendar Delivery | P1 | 07 | `internal/cardrewards/calendar`, CalDAV | Not Started |
| 09 | Scheduler Jobs & Manual Triggers | P1 | 04, 05, 06, 07, 08 | `internal/scheduler` | Not Started |
| 10 | Web UI — Wallet, Offers, Selections, Bonuses, Categories | P1 | 02 | `internal/web` (Go templates), e2e-ui | Not Started |
| 11 | Web UI — Dashboard, Recommendations, Rotating Verify, Report, Admin | P1 | 06, 07, 09, 10 | `internal/web` + `internal/web/admin`, e2e-ui | Not Started |

## Dependency Graph

```
01-config-migration ──▶ 02-store-crud ──┬──▶ 03-data-migration
                                        │
                                        ├──▶ 04-connector ──▶ 05-llm-extract ──▶ 06-reconcile-lifecycle
                                        │                                              │
                                        ├──────────────────────────────────────────────┤
                                        │                                              ▼
                                        ├──▶ 07-optimizer-recommend ──▶ 08-caldav-delivery
                                        │            │                       │
                                        │            └───────────────────────┤
                                        │                                    ▼
                                        │          04,05,06,07,08 ──▶ 09-scheduler-jobs
                                        │                                    │
                                        └──▶ 10-web-ui-crud ─────────────────┤
                                                                             ▼
                                                       06,07,09,10 ──▶ 11-web-ui-dashboard-admin
```

---

## Scope 01: Config SST & Migration Schema

**Status:** Done
**Priority:** P0
**Depends On:** None
**Spec Refs:** FR-CR-001, FR-CR-020, NFR-CR-002, design §2, §10

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-A01 — card_rewards config section parsed from smackerel.yaml
  Given config/smackerel.yaml contains a card_rewards section with enabled, scrape_cron,
        monthly_recommend_cron, calendar_sync, fetch_timeout_seconds, extraction, sources,
        and tracked_categories
  When the Go config loader parses the configuration
  Then CardRewardsConfig is populated with all fields matching the YAML values

Scenario: SCN-083-A02 — config generate emits CARD_REWARDS_* env vars
  Given the card_rewards section exists in smackerel.yaml
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env and config/generated/test.env contain CARD_REWARDS_ENABLED,
       CARD_REWARDS_SCRAPE_CRON, CARD_REWARDS_MONTHLY_RECOMMEND_CRON, CARD_REWARDS_CALENDAR_SYNC,
       CARD_REWARDS_EXTRACTION_MODEL, CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD, and the
       source/tracked-category vars

Scenario: SCN-083-A03 — fail-loud on missing required config when enabled
  Given CARD_REWARDS_ENABLED=true but CARD_REWARDS_EXTRACTION_MODEL is unset/empty
  When the service starts
  Then it exits with a fatal error naming the missing variable (no in-source default)

Scenario: SCN-083-A04 — empty sources list rejected when enabled
  Given card_rewards.enabled is true and card_rewards.sources is empty
  When the config is validated
  Then validation fails loudly identifying sources as required-non-empty

Scenario: SCN-083-A05 — migration 057 creates all card-rewards tables
  Given the database has migrations up to 056
  When migration 057_card_rewards.sql is applied
  Then tables card_catalog, user_cards, card_offers, card_selections, signup_bonuses,
       rotating_category_observations, rotating_categories, category_aliases,
       card_recommendations, and card_runs exist with their CHECK constraints, FKs, and indexes

Scenario: SCN-083-A06 — rotating_categories enforces lifecycle + uniqueness constraints
  Given migration 057 is applied
  Then rotating_categories has CHECK lifecycle_state IN (upcoming, active, expired)
   And a UNIQUE (card_catalog_id, period_label) constraint
   And a NOT NULL needs_verification column defaulting to false

Scenario: SCN-083-A07 — disabled feature parses without requiring extraction config
  Given card_rewards.enabled is false
  When the config loader parses the configuration
  Then no extraction/source fields are required and the service starts normally
```

### Implementation Plan

- Create `internal/db/migrations/057_card_rewards.sql` (10 tables per design §2 with CHECK/FK/UNIQUE/indexes).
- Add `card_rewards:` to `config/smackerel.yaml` and `connectors.card-rewards`.
- Add `CardRewardsConfig` struct + fail-loud validation in `internal/config/config.go`.
- Emit `CARD_REWARDS_*` env vars in `scripts/commands/config.sh`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-01-01 | Unit | `internal/config/validate_test.go` | SCN-083-A01, A07 | Parse CardRewardsConfig (enabled + disabled) |
| T-01-02 | Integration | `scripts/commands/config.sh` test | SCN-083-A02 | config generate emits CARD_REWARDS_* vars |
| T-01-03 | Unit | `internal/config/validate_test.go` | SCN-083-A03, A04 | Fail-loud on missing/empty required config |
| T-01-04 | Integration | `tests/integration/db_migration_test.go` | SCN-083-A05, A06 | Migration 057 creates tables/constraints/indexes |

### Definition of Done

- [x] Implementation behavior complete: `card_rewards` config section + `connectors.card-rewards` exist; `CardRewardsConfig` parses with fail-loud validation; migration 057 creates all 10 tables with constraints/indexes — Evidence: [report.md](report.md) → "Delivery — Scope 01" (Files created/changed; A05/A06 migration test PASS)
- [x] Scenario tests pass for SCN-083-A01 and SCN-083-A07 (config parse, enabled + disabled) — unit — Evidence: [report.md](report.md) → "Evidence — SCN-083-A01/A03/A04/A07" (`TestLoadCardRewardsConfig_PopulatesWhenEnabled`, `…_DisabledParsesWithoutRequiringConfig` PASS; `ok internal/config`)
- [x] Scenario tests pass for SCN-083-A02 (config generate emits CARD_REWARDS_* env vars) — integration — Evidence: [report.md](report.md) → "Evidence — SCN-083-A02" (13 `CARD_REWARDS_*` vars in dev.env + test.env)
- [x] Scenario tests pass for SCN-083-A03 and SCN-083-A04 (fail-loud on missing/empty required config) — unit — Evidence: [report.md](report.md) → "Evidence — SCN-083-A01/A03/A04/A07" (`…_FailLoudOnMissingRequired` 7 subtests, `…_EmptySourcesRejected` 4 subtests, `…_EmptyTrackedCategoriesRejected` PASS)
- [x] Scenario tests pass for SCN-083-A05 and SCN-083-A06 (migration 057 tables/constraints/indexes) — integration (live PG) — Evidence: [report.md](report.md) → "Evidence — SCN-083-A05/A06" (`--- PASS: TestCardRewardsMigration_AppliesCleanly`; `INTEGRATION_EXIT=0`)
- [x] Build Quality Gate: `./smackerel.sh build`, `check`, `lint`, `format --check` clean (zero warnings); no `${VAR:-default}` fallbacks introduced (`smackerel-no-defaults`); artifact-lint clean; docs aligned — Evidence: [report.md](report.md) → "Evidence — Build Quality Gate" (FORMAT_EXIT=0, CHECK_EXIT=0, LINT_EXIT=0; ARTIFACT_LINT_EXIT=0; images built in integration run)

---

## Scope 02: Card Domain Store, Types & CRUD API

**Status:** Done
**Priority:** P0
**Depends On:** 01
**Spec Refs:** FR-CR-001..006, FR-CR-016 (API), NFR-CR-002, NFR-CR-006, design §2, §6 (resolve)

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-B01 — create and read a user card
  Given the card_catalog contains "citi-custom-cash"
  When a user card referencing citi-custom-cash is created via the store
  Then it is persisted in user_cards and readable with its nickname, note, and active flag

Scenario: SCN-083-B02 — card name resolution returns catalog candidates
  Given the catalog contains Citi Custom Cash with aliases ["citi custom cash", "custom cash"]
  When the resolver is given the free text "custom cash"
  Then it returns citi-custom-cash as the top candidate

Scenario: SCN-083-B03 — ambiguous resolution returns multiple candidates
  Given two catalog cards share an alias token
  When the resolver is given that ambiguous text
  Then it returns more than one candidate for disambiguation

Scenario: SCN-083-B04 — custom (non-catalog) card creation
  Given a description matching no catalog card
  When a custom card is created with source="manual"
  Then a card_catalog row with source="manual" and a user_cards row are created

Scenario: SCN-083-B05 — offer with shared limit group persists
  Given a user card exists
  When an offer with shared_limit_group="amex-dining-pool" and a date window is created
  Then it is persisted and queryable by user card and by shared_limit_group

Scenario: SCN-083-B06 — tiered selection persists with tier and period
  Given a user holds US Bank Cash+ (selectable, tiered)
  When a tier-1 selection for "Utilities" in period "Q3 2026" is saved
  Then card_selections stores tier=1, period_label="Q3 2026", category="Utilities"

Scenario: SCN-083-B07 — removing a card cascades to its offers/selections/bonuses
  Given a user card with offers, selections, and a signup bonus
  When the user card is deleted
  Then its offers, selections, and bonuses are removed (ON DELETE CASCADE)

Scenario: SCN-083-B08 — CRUD REST endpoints round-trip card data
  Given the card-rewards API is mounted
  When a client POSTs a card, GETs it, PUTs an edit, and DELETEs it
  Then each call returns the expected status and the final GET reflects deletion
```

### Implementation Plan

- `internal/cardrewards/types.go`, `store.go` (pgx CRUD for catalog, user_cards, offers, selections, bonuses, category_aliases).
- `internal/cardrewards/resolve.go` (alias/token resolution; replaces `card_resolver.py`).
- REST endpoints under the existing API router for card CRUD (consumed by Web UI + available to PWA).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-02-01 | Unit | `internal/cardrewards/resolve_test.go` | SCN-083-B02, B03 | Resolution: top candidate + ambiguity |
| T-02-02 | Integration | `internal/cardrewards/store_test.go` | SCN-083-B01, B04, B05, B06 | Store CRUD (cards, custom, offers, tiered selections) — live PG |
| T-02-03 | Integration | `internal/cardrewards/store_test.go` | SCN-083-B07 | Cascade delete of dependent rows — live PG |
| T-02-04 | E2E API | `tests/e2e/cardrewards_api_test.go` | SCN-083-B08 | CRUD endpoints round-trip — live stack |

### Definition of Done

- [x] Implementation behavior complete: domain types + PostgreSQL store with full CRUD; card resolver; REST CRUD endpoints mounted behind existing auth — Evidence: [report.md](report.md) → "Delivery — Scope 02" (Files created/changed: types.go/store.go/resolve.go/service.go + internal/api/cardrewards.go mounted in the bearer-auth group; handler mounts on pg-pool presence)
- [x] Scenario tests pass for SCN-083-B02 and SCN-083-B03 (resolution top candidate + ambiguity) — unit — Evidence: [report.md](report.md) → "Evidence — SCN-083-B02/B03" (`TestResolveCard_TopCandidate_B02`, `…_Ambiguous_B03`, `…_SharedExactAlias_B03` + 4 boundary/adversarial PASS; `ok internal/cardrewards`)
- [x] Scenario tests pass for SCN-083-B01, B04, B05, B06 (store CRUD incl. custom card, shared-limit offer, tiered selection) — integration (live PG) — Evidence: [report.md](report.md) → "Evidence — SCN-083-B01/B04/B05/B06/B07" (`--- PASS: TestCardRewardsStore_CreateReadUserCard_B01`/`…_CreateCustomCard_B04`/`…_SharedLimitOffer_B05`/`…_TieredSelection_B06`; `PASS: go-integration`)
- [x] Scenario tests pass for SCN-083-B07 (cascade delete) — integration (live PG) — Evidence: [report.md](report.md) → "Evidence — SCN-083-B01/B04/B05/B06/B07" (`--- PASS: TestCardRewardsStore_CascadeDelete_B07`; offers/selections/bonuses all removed by ON DELETE CASCADE)
- [x] Scenario tests pass for SCN-083-B08 (CRUD REST endpoints round-trip) — e2e-api (live stack) — Evidence: [report.md](report.md) → "Evidence — SCN-083-B08" (`--- PASS: TestCardRewardsAPICRUDRoundTrip_B08`; POST→201, GET→200, PUT→200, DELETE→204, GET→404 CARD_NOT_FOUND on the live stack)
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); no internal mocks in tests (real test DB); artifact-lint clean; docs aligned — Evidence: [report.md](report.md) → "Evidence — Build Quality Gate (Scope 02)" (CHECK_EXIT=0, LINT_EXIT=0 "All checks passed!", FORMAT_CHECK_EXIT=0; store/e2e tests use the real disposable Postgres + live stack, zero internal mocks)

---

## Scope 03: Data Migration — CCManager JSON → PostgreSQL

**Status:** Done
**Priority:** P0
**Depends On:** 02
**Spec Refs:** FR-CR-017, UC-007, design §11

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-C01 — import seeds the card catalog from cards-database.json
  Given CCManager/data/cards-database.json with ~21 cards
  When the one-time importer runs against an empty database
  Then card_catalog contains a row per card with type, benefits, aliases, and source="seed"

Scenario: SCN-083-C02 — import is idempotent
  Given the importer has already run once
  When it runs again against the same JSON
  Then no duplicate rows are created (upsert on natural keys)

Scenario: SCN-083-C03 — imported rotating categories are marked manual_override
  Given rotating-categories.json with historical quarters
  When the importer seeds rotating_categories
  Then each row has manual_override=true and source semantics so the first LLM refresh does not clobber it

Scenario: SCN-083-C04 — partial/missing JSON file does not abort the import
  Given user-offers.json is missing but other files are present
  When the importer runs
  Then it imports the available files, logs the skipped file, and completes without aborting

Scenario: SCN-083-C05 — category aliases imported from config.json
  Given config.json categories with starred, priority, built_in, and equivalents
  When the importer runs
  Then category_aliases rows reflect canonical names, equivalents, starred, and priority

Scenario: SCN-083-C06 — a migration run is recorded in card_runs
  Given the importer runs
  Then a card_runs row with run_type="migration" and a success/partial status is written
```

### Implementation Plan

- `cmd/cardrewards-import/main.go` (or `./smackerel.sh cardrewards import`) reading CCManager JSON at a configured path; idempotent upserts per design §11 mapping table.
- Records a `card_runs` migration row; logs skipped/partial files.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-03-01 | Integration | `internal/cardrewards/import_test.go` | SCN-083-C01, C05 | Catalog + category aliases imported — live PG |
| T-03-02 | Integration | `internal/cardrewards/import_test.go` | SCN-083-C02 | Idempotent re-run (no duplicates) — live PG |
| T-03-03 | Integration | `internal/cardrewards/import_test.go` | SCN-083-C03 | Imported rotating categories flagged manual_override — live PG |
| T-03-04 | Integration | `internal/cardrewards/import_test.go` | SCN-083-C04, C06 | Partial-file tolerance + migration run logged — live PG |

### Definition of Done

- [x] Implementation behavior complete: one-time idempotent importer maps all CCManager JSON files to PostgreSQL tables per design §11; logs skipped files; records a migration run → Evidence: report.md "Delivery — Scope 03" (Files + Decisions + SCN-083-C01..C06 integration block; `internal/cardrewards/import.go` `RunImport`)
- [x] Scenario tests pass for SCN-083-C01 and SCN-083-C05 (catalog + category alias import) — integration (live PG) → Evidence: report.md SCN-083-C01..C06 block — `TestCardRewardsImport_CatalogAndAliases_C01_C05 PASS` (catalog 7, aliases 8, row counts)
- [x] Scenario tests pass for SCN-083-C02 (idempotent re-run) — integration (live PG) → Evidence: report.md SCN-083-C01..C06 block — `TestCardRewardsImport_Idempotent_C02 PASS` (zero new data rows on re-run; migration audit +1)
- [x] Scenario tests pass for SCN-083-C03 (rotating categories marked manual_override) — integration (live PG) → Evidence: report.md SCN-083-C01..C06 block — `TestCardRewardsImport_RotatingManualOverride_C03 PASS` (discover-it Q1_2026 known value; manual_override=true)
- [x] Scenario tests pass for SCN-083-C04 and SCN-083-C06 (partial-file tolerance + run logged) — integration (live PG) → Evidence: report.md SCN-083-C01..C06 block — `TestCardRewardsImport_PartialFileToleranceAndRunLogged_C04_C06 PASS` (missing file skipped, run_type=migration logged)
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); real test DB (no mocks); artifact-lint clean; docs aligned → Evidence: report.md "Evidence — Build Quality Gate (Scope 03)" (CONFIG_GENERATE/FORMAT_CHECK/CHECK/LINT all exit 0) + 16 transform unit tests + 4 live-PG integration tests

---

## Scope 04: Card-Rewards Source Connector

**Status:** Done
**Priority:** P1
**Depends On:** 01, 02
**Spec Refs:** FR-CR-007 (fetch half), FR-CR-008, Principle 4, design §3

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-D01 — connector implements the Connector interface
  Given the card-rewards connector is registered
  Then it satisfies connector.Connector (ID, Connect, Sync, Health, Close) and ID() returns "card-rewards"

Scenario: SCN-083-D02 — Sync emits one source-attributed RawArtifact per source+card hint
  Given two configured sources and two card hints
  When Sync runs against fixture source content
  Then it emits RawArtifacts whose Metadata carries source_name, source_url, and issuer_hint

Scenario: SCN-083-D03 — connector does not parse categories (no regex)
  Given a fetched source page
  When Sync runs
  Then the RawArtifact RawContent holds the page text and no category parsing/regex is applied in the connector

Scenario: SCN-083-D04 — fetch timeout degrades only that source
  Given one source is slow beyond fetch_timeout_seconds and another responds
  When Sync runs
  Then the slow source is skipped (recorded as failed) and the healthy source still emits an artifact

Scenario: SCN-083-D05 — connector health reflects consecutive errors
  Given N consecutive fetch failures
  When Health is queried
  Then it returns degraded/failing per connector.HealthFromErrorCount thresholds

Scenario: SCN-083-D06 — cursor advances to last successful fetch
  Given a successful Sync
  Then the returned cursor encodes the last successful fetch timestamp
```

### Implementation Plan

- `internal/connector/cardrewards/connector.go` implementing the interface; fetch-only (no regex), source-attributed `RawArtifact` emission; register in `internal/connector/registry.go`; config via `connectors.card-rewards`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-04-01 | Unit | `internal/connector/cardrewards/connector_test.go` | SCN-083-D01, D06 | Interface compliance + cursor |
| T-04-02 | Unit | `internal/connector/cardrewards/connector_test.go` | SCN-083-D02, D03 | Source-attributed emission; no parsing in connector |
| T-04-03 | Unit | `internal/connector/cardrewards/connector_test.go` | SCN-083-D04, D05 | Timeout isolation + health thresholds |

### Definition of Done

- [x] Implementation behavior complete: `card-rewards` connector fetches configured sources read-only and emits source-attributed RawArtifacts; registered; no category parsing in the connector → Evidence: report.md "Delivery — Scope 04" (Files + Decisions; `internal/connector/cardrewards/connector.go` fetch-only, no `regexp`; registered + auto-start gated in `cmd/core/connectors.go`; SCN-083-D01..D06 unit block all PASS)
- [x] Scenario tests pass for SCN-083-D01 and SCN-083-D06 (interface compliance + cursor) — unit → Evidence: report.md SCN-083-D01..D06 unit block — `TestConnector_ImplementsInterfaceAndID_D01 PASS` (compile-time `var _ connector.Connector = New()` + `ID()=="card-rewards"`) + `TestSync_CursorEncodesLastSuccessfulFetch_D06 PASS` (cursor RFC3339Nano within [before,after])
- [x] Scenario tests pass for SCN-083-D02 and SCN-083-D03 (source-attributed emission; no regex) — unit → Evidence: report.md SCN-083-D01..D06 unit block — `TestSync_EmitsSourceAttributedArtifactPerSource_D02 PASS` (2 sources→2 artifacts; Metadata source_name/source_url/issuer_hint) + `TestSync_NoCategoryParsingRawContentVerbatim_D03 PASS` (RawContent verbatim; exactly 3 provenance keys; no parsed category/rate keys)
- [x] Scenario tests pass for SCN-083-D04 and SCN-083-D05 (timeout isolation + health) — unit → Evidence: report.md SCN-083-D01..D06 unit block — `TestSync_SlowSourceDegradesOnlyThatSource_D04 PASS` (slow source recorded failed via per-source deadline; fast source still emits; LastSyncStats=1/1; degraded) + `TestHealth_ReflectsConsecutiveErrors_D05 PASS` (1-4→healthy, 5→degraded, 10→failing via HealthFromErrorCount)
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); connector-metadata-preservation honored (Principle 4); artifact-lint clean; docs aligned → Evidence: report.md "Evidence — Build Quality Gate (Scope 04)" (check: config in sync + scenario-lint OK; `format --check`: 63 files already formatted; lint: All checks passed! + Web validation passed; `go test ./... finished OK`) + report.md "Gate: artifact-lint — Scope 04"

---

## Scope 05: LLM Category Extraction (replaces regex)

**Status:** In Progress — 7 of 8 DoD items complete with real evidence. The one remaining item (SCN-083-E08 *successful* live Ollama inference round-trip) is **blocked-needs-live-Ollama**: the disposable-stack Ollama serves no pulled LLM model in this environment (litellm `APIConnectionError`), so a successful inference cannot run here. The E08 audit-run persistence + the real orchestrator→sidecar HTTP fail-loud contract ARE proven live (see report.md). Satisfiable on the <home-lab-host> ops node.
**Priority:** P0
**Depends On:** 04
**Spec Refs:** FR-CR-007, FR-CR-010, NFR-CR-001, NFR-CR-003, NFR-CR-008, §17.2 (Constitution C2), design §4

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-E01 — valid extraction returns a schema-conformant record
  Given a source observation for Discover Q3 2026
  When the extractor calls the LLM and validates the response
  Then it stores a rotating_category_observations row with categories, dates, confidence, and verbatim evidence

Scenario: SCN-083-E02 — malformed JSON is discarded, not stored (adversarial)
  Given the LLM returns non-JSON / partial JSON for an observation
  When the extractor validates the response
  Then nothing is stored, the response is logged, and the run is marked partial

Scenario: SCN-083-E03 — no silent fallback on extraction failure (adversarial vs regex bug)
  Given an existing rotating_categories record and an extraction that fails to validate
  When the refresh runs
  Then the existing record is preserved and flagged needs_verification — it is NOT overwritten with stale or placeholder data

Scenario: SCN-083-E04 — low confidence flags needs_verification (adversarial)
  Given the LLM returns a valid record with confidence below the configured threshold
  When the extractor stores the observation
  Then downstream reconciliation flags needs_verification=true

Scenario: SCN-083-E05 — unknown card id is skipped, not mismapped (adversarial)
  Given a source references a card id not in card_catalog
  When the extractor processes it
  Then the observation is skipped with an audit note and no known card is mismapped

Scenario: SCN-083-E06 — page content treated as data, not instructions (injection defense)
  Given a source page embeds "ignore previous instructions" text
  When the extractor builds the prompt
  Then the system prompt treats page content as data and the embedded instruction is not followed

Scenario: SCN-083-E07 — extraction provenance retained
  Given a valid extraction
  Then the observation row stores source_name, source_url, and source_evidence (Principle 4)

Scenario: SCN-083-E08 — extraction run is audited
  Given an extraction batch runs
  Then a card_runs row with run_type="extract" records sources_attempted/succeeded and categories_extracted
```

### Implementation Plan

- `ml/app/card_categories.py` (NEW ML-sidecar route — Constitution C2 model-gateway boundary, sibling of `drive_classify.py` / `intelligence.py`): owns the Ollama call, the prompt (page content treated as data — injection defense), and the first strict-JSON pass; exposes `POST /extract-card-categories` with Bearer auth like the other sidecar routes.
- `internal/cardrewards/extract.go` (Go ORCHESTRATOR — NO direct Ollama client, NFR-CR-001/008): sends cleaned page text + candidate card/issuer to the sidecar over the existing Go↔sidecar HTTP contract (pattern: `internal/agent/embedder/sidecar`), validates the response with `santhosh-tekuri/jsonschema` (defense-in-depth, §17.2), applies confidence handling + unknown-card skip, and writes observations + `card_runs`. Provide a deterministic schema-fixture seam for Go tests; spec 043 Ollama test infra for live sidecar runs.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-05-01 | Unit | `internal/cardrewards/extract_test.go` | SCN-083-E01, E07 | Valid extraction + provenance stored |
| T-05-02 | Unit | `internal/cardrewards/extract_test.go` | SCN-083-E02, E03 | Malformed → discard, no silent overwrite (adversarial) |
| T-05-03 | Unit | `internal/cardrewards/extract_test.go` | SCN-083-E04, E05 | Low confidence flag + unknown-card skip (adversarial) |
| T-05-04 | Unit | `internal/cardrewards/extract_test.go` | SCN-083-E06 | Prompt-injection defense (content as data) |
| T-05-05 | Integration | `tests/integration/cardrewards_extract_test.go` | SCN-083-E08 | Extraction run audited; live PG + sidecar→Ollama (spec 043) |
| T-05-06 | Unit (Python) | `ml/tests/test_card_categories.py` | SCN-083-E01, E06 | Sidecar route returns strict-schema JSON; page content treated as data (injection defense) |

### Definition of Done

- [x] Constitution C2 boundary honored: the model-gateway call lives in `ml/app/card_categories.py` (Python sidecar); `internal/cardrewards/extract.go` contains NO direct model-backend client and only orchestrates + schema-validates the sidecar response (NFR-CR-001/008) — verified by a grep for ollama / `/api/generate` / `/api/chat` URLs under `internal/cardrewards/` returning zero hits → Evidence: report.md "Gate: Constitution C2 boundary grep" (`C2_GREP_EXIT=1`, zero matches)
- [x] Implementation behavior complete: strict-schema LLM extractor replaces regex; validates before storage; confidence/unknown-card/override handling; injection-safe prompt; writes observations + extract run → Evidence: report.md "Delivery — Scope 05" (Files: `extract.go` orchestrator + `card_categories.py` sidecar route; `store.go` `PersistExtractionRun`; the live-PG integration block proves observations + `extract` run are written)
- [x] Scenario tests pass for SCN-083-E01 and SCN-083-E07 (valid extraction + provenance) — unit → Evidence: report.md "SCN-083-E01..E07 (unit)" — `TestValidateExtraction_ValidRecordWithProvenance_E01_E07 PASS` + live-PG `TestExtractorLivePG_StoresObservationWithProvenance_E01_E07 PASS` (provenance source_name/url/evidence persisted)
- [x] Adversarial scenario tests pass for SCN-083-E02 and SCN-083-E03 (malformed discarded; no silent fallback) — unit; each uses input that would PASS the old silent-fallback path but MUST fail-loud to verification → Evidence: report.md — `TestValidateExtraction_MalformedAndInvalidDiscarded_E02_E03 PASS` (8 discard subtests) + live-PG `TestExtractorLivePG_MalformedDiscardedNoOverwrite_E02_E03 PASS` (existing record categories/confidence/manual_override UNCHANGED; only needs_verification flipped true)
- [x] Adversarial scenario tests pass for SCN-083-E04 and SCN-083-E05 (low-confidence flag; unknown-card skip) — unit → Evidence: report.md — `TestValidateExtraction_LowConfidenceFlagged_E04 PASS` + `TestValidateExtraction_UnknownCardSkipped_E05 PASS` + live-PG `TestExtractorLivePG_LowConfidenceStored_E04` / `TestExtractorLivePG_UnknownCardSkippedNoMismap_E05` PASS (no mismap onto co-resident known card)
- [x] Scenario test passes for SCN-083-E06 (prompt-injection defense) — unit → Evidence: report.md — Go `TestExtractRequest_PageContentIsDataNotInstructions_E06` + `TestValidateExtraction_RejectsCardOrPeriodMismatch_E06` PASS, and Python `test_card_categories.py::test_build_messages_treats_page_content_as_data_E06` (injected text only in the untrusted PAGE_CONTENT data block; system prompt declares it untrusted + forbids following it)
- [ ] Scenario test passes for SCN-083-E08 (extraction run audited) — integration (live PG + Ollama per spec 043) → **BLOCKED-NEEDS-LIVE-OLLAMA (honest).** `TestCardRewardsExtractLiveStackAudited_E08` RUNS + PASSES against the real ml sidecar (proves the orchestrator→`/extract-card-categories` HTTP contract, fail-loud handling, and a persisted `extract` audit run), and the audit-run persistence is independently PROVEN on live PG by `TestExtractorLivePG_ExtractionRunAudited_E08 PASS`. The remaining gap is a SUCCESSFUL sidecar→Ollama inference: here litellm returns `APIConnectionError` (disposable-stack Ollama has no pulled LLM model; the `integration` lane runs no `ollama-test-pull`). Satisfiable on the <home-lab-host> ops node. See report.md "SCN-083-E08 (live PG + real ml sidecar round-trip)".
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); §17.2 strict-schema contract honored; artifact-lint clean; docs aligned → Evidence: report.md "Build Quality Gate (Scope 05)" (`CHECK_EXIT=0`; `LINT_EXIT=0` All checks passed! + Web validation passed; `FORMAT_RECHECK_EXIT=0`) + connector-count + doc-freshness contracts green + artifact-lint (report.md "Gate: artifact-lint — Scope 05")

---

## Scope 06: Multi-Source Reconciliation & Category Lifecycle

**Status:** Done
**Priority:** P1
**Depends On:** 05
**Spec Refs:** FR-CR-009, FR-CR-011, FR-CR-012, Principle 3, design §5

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-F01 — agreeing sources reconcile to a high-confidence record
  Given two source observations agree on Discover Q3 2026 categories
  When the reconciler merges them
  Then rotating_categories has the agreed categories, source_count=2, and needs_verification=false

Scenario: SCN-083-F02 — disagreeing sources flag needs_verification (adversarial)
  Given two source observations disagree on the category set for the same card+period
  When the reconciler merges them
  Then the record is flagged needs_verification=true and both observations are retained for audit

Scenario: SCN-083-F03 — manual override is never overwritten
  Given a rotating_categories record with manual_override=true
  When a new extraction observation arrives
  Then the record's categories are unchanged and the observation is recorded for audit only

Scenario: SCN-083-F04 — upcoming → active transition by date
  Given a record whose period_start is now in the past and period_end is in the future
  When the daily lifecycle pass runs
  Then lifecycle_state becomes "active"

Scenario: SCN-083-F05 — active → expired transition by date
  Given a record whose period_end is in the past
  When the daily lifecycle pass runs
  Then lifecycle_state becomes "expired" and it is excluded from current recommendations

Scenario: SCN-083-F06 — re-enrollment window opening raises a pending action
  Given a selectable card whose next enrollment window opens today
  When the lifecycle pass runs
  Then a pending re-enrollment action is recorded for the dashboard

Scenario: SCN-083-F07 — reconciliation upsert is idempotent
  Given the reconciler runs twice on the same observations
  Then exactly one rotating_categories row exists per (card, period)
```

### Implementation Plan

- `internal/cardrewards/reconcile.go`: merge per (card, period), override protection, confidence aggregation, `needs_verification` rules, date-driven lifecycle transitions, idempotent upsert; emits `card_runs` reconcile rows.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-06-01 | Unit | `internal/cardrewards/reconcile_test.go` | SCN-083-F01, F02 | Agreement vs disagreement (adversarial) |
| T-06-02 | Unit | `internal/cardrewards/reconcile_test.go` | SCN-083-F03 | Manual override protection |
| T-06-03 | Unit | `internal/cardrewards/reconcile_test.go` | SCN-083-F04, F05 | Lifecycle date transitions |
| T-06-04 | Integration | `internal/cardrewards/reconcile_test.go` | SCN-083-F06, F07 | Re-enrollment pending action + idempotent upsert — live PG |

### Definition of Done

- [x] Implementation behavior complete: reconciler merges multi-source observations with confidence + needs_verification, protects manual overrides, advances lifecycle by date, raises re-enrollment actions, upserts idempotently → Evidence: report.md "Delivery — Scope 06" (Files verified: `reconcile.go` `Reconciler` + PURE `mergeObservations`/`deriveLifecycle` + `Reconcile`/`AdvanceLifecycle`; `store.go` upsert/lifecycle/pending-reenrollment methods; all SCN-083-F01..F07 unit + live-PG blocks PASS; no implementation bug found so source was NOT modified)
- [x] Scenario tests pass for SCN-083-F01 and SCN-083-F02 (agreement; disagreement → verify) — unit (F02 adversarial) → Evidence: report.md "SCN-083-F01/F02/F03/F04/F05 ... (reconcile unit tests)" — `TestReconcile_MergeAgreement_F01 PASS` (agreed set, source_count=2, needs_verification=false, confidence=0.90) + `TestReconcile_MergeDisagreement_F02 PASS` (ADVERSARIAL: disagreement→needs_verification=true, conservative confidence 0.88, source_count=1; `REGRESSION` guards fail if silently reconciled as agreement)
- [x] Scenario test passes for SCN-083-F03 (manual override never overwritten) — unit → Evidence: report.md unit block `TestReconcile_ManualOverrideNeverOverwritten_F03 PASS` (ADVERSARIAL: high-confidence 0.99 disagreeing observation does NOT overwrite `manual_override=true`; categories stay `[Gym Memberships]`, confidence 1.0, needs_verification false) + live `TestReconcileLivePG_ManualOverrideNotRewritten_F03 PASS` (overrides_protected=1, reconciled=0; observation retained for audit)
- [x] Scenario tests pass for SCN-083-F04 and SCN-083-F05 (upcoming→active→expired) — unit → Evidence: report.md unit block `TestReconcile_LifecycleByDate_F04_F05 PASS` (6 subtests: upcoming / F04 active / F05 expired + both boundary days + undated→unknown) + live `TestReconcileLivePG_LifecycleTransitions_F04_F05 PASS` (upcoming→active & active→expired transitions logged; expired EXCLUDED from `ListActiveRotatingCategories` via `REGRESSION` guard)
- [x] Scenario tests pass for SCN-083-F06 and SCN-083-F07 (re-enrollment action + idempotent upsert) — integration (live PG) → Evidence: report.md "SCN-083-F06/F07 (+ live F02/F03/F04/F05) on live disposable Postgres" — `TestReconcileLivePG_PendingReEnrollment_F06 PASS` (only the window-opening-today & not-enrolled selection surfaced; future / already-enrolled NOT; count=1 via both `AdvanceLifecycle` and `ListPendingReEnrollments`) + `TestReconcileLivePG_IdempotentUpsert_F07 PASS` (reconcile twice → `CountRotatingCategoriesByCardPeriod`==1 + stable row id; `REGRESSION` guard); `INTEG_EXIT=0`; disposable stack fully torn down (ephemeral isolation)
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); Principle 3 lifecycle honored; artifact-lint clean; docs aligned → Evidence: report.md "Evidence — Build Quality Gate (Scope 06)" (`CHECK_EXIT=0` config in sync + drift guard + scenario-lint OK; `LINT_EXIT=0` golangci-lint All checks passed! + Web validation passed; `FORMAT_EXIT=0` 65 files already formatted; `ARTIFACT_LINT_EXIT=0` Artifact lint PASSED)

---

## Scope 07: Optimizer & Monthly Recommendation Generation

**Status:** Done
**Priority:** P1
**Depends On:** 02, 06
**Spec Refs:** FR-CR-013, FR-CR-014, Principle 8 (reasons), design §6

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-G01 — base-rate optimization picks the highest fixed rate
  Given two owned cards with different base rates for "Groceries"
  When the optimizer evaluates "Groceries"
  Then it picks the card with the higher effective base rate and records a reason

Scenario: SCN-083-G02 — active rotating category beats base rate
  Given a card has an active rotating 5% category matching "Restaurants" and another card has 3% base
  When the optimizer evaluates "Restaurants"
  Then it picks the rotating-category card and the reason cites the rotating benefit

Scenario: SCN-083-G03 — expired rotating category is ignored
  Given a rotating category for the queried category is expired
  When the optimizer evaluates that category
  Then the expired benefit is not used

Scenario: SCN-083-G04 — shared/combined limit pool respected
  Given two offers in the same shared_limit_group with a combined limit
  When the optimizer evaluates categories covered by that pool
  Then the combined limit is treated as one pool, not double-counted

Scenario: SCN-083-G05 — category equivalents normalize before matching
  Given the user queries "eating out" and category_aliases maps it to "Dining"
  When the optimizer evaluates the query
  Then it matches Dining benefits

Scenario: SCN-083-G06 — monthly recommendations generated per tracked category
  Given tracked categories and current card data
  When recommendation generation runs for period 2026-06
  Then one card_recommendations row per tracked category is written with rate and reason

Scenario: SCN-083-G07 — starred override beats optimizer output
  Given a starred_override recommendation exists for a category
  When recommendations are regenerated
  Then the starred override is preserved over the optimizer's pick

Scenario: SCN-083-G08 — recommendation/report endpoints return current data
  Given recommendations exist for the period
  When a client GETs the recommendations and report endpoints
  Then they return the current period's recommendations and the optimization report
```

### Implementation Plan

- `internal/cardrewards/optimize.go` + `recommend.go`: effective-rate computation (base/rotating/offer/selection, shared-limit pools, ties), equivalents normalization, per-period recommendation upsert honoring starred overrides, reasons recorded; REST recommendation + report endpoints.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-07-01 | Unit | `internal/cardrewards/optimize_test.go` | SCN-083-G01, G02, G03 | Base vs rotating; expired ignored |
| T-07-02 | Unit | `internal/cardrewards/optimize_test.go` | SCN-083-G04, G05 | Shared-limit pool; equivalents |
| T-07-03 | Integration | `internal/cardrewards/recommend_test.go` | SCN-083-G06, G07 | Per-category generation + starred override — live PG |
| T-07-04 | E2E API | `tests/e2e/cardrewards_api_test.go` | SCN-083-G08 | Recommendation/report endpoints — live stack |

### Definition of Done

- [x] Implementation behavior complete: optimizer computes best card from base/rotating/offer/selection with shared-limit pools, ties, and equivalents; monthly recommendation generation honors starred overrides; reasons recorded; REST endpoints mounted — Evidence: [report.md](report.md) → "Evidence — DoD 1: Implementation behavior complete" (no source rewritten this run; behavior proven by G01–G08; reasons asserted by G01 "records a reason" + G02 "reason cites the rotating benefit")
- [x] Scenario tests pass for SCN-083-G01, G02, G03 (base vs rotating; expired ignored) — unit — Evidence: [report.md](report.md) → "Evidence — DoD 2 (SCN-083-G01, G02, G03) + DoD 3" (`TestOptimize_BaseRateHighestWins_G01`/`…_ActiveRotatingBeatsBase_G02`/`…_ExpiredRotatingIgnored_G03` PASS; `ok internal/cardrewards`; UNIT_EXIT=0)
- [x] Scenario tests pass for SCN-083-G04 and SCN-083-G05 (shared-limit pool; equivalents) — unit — Evidence: [report.md](report.md) → "Evidence — DoD 2 … + DoD 3 (SCN-083-G04, G05)" (`TestOptimize_SharedLimitPoolNotDoubleCounted_G04`/`…_EquivalentsNormalizeBeforeMatching_G05` PASS; UNIT_EXIT=0)
- [x] Scenario tests pass for SCN-083-G06 and SCN-083-G07 (per-category generation + starred override) — integration (live PG) — Evidence: [report.md](report.md) → "Evidence — DoD 4: SCN-083-G06 + adversarial SCN-083-G07" (`TestRecommendLivePG_PerCategoryGeneration_G06`/`…_StarredOverridePreserved_G07` PASS on live PG; INTEG_EXIT=0; disposable stack torn down)
- [x] Scenario test passes for SCN-083-G08 (recommendation/report endpoints) — e2e-api (live stack) — Evidence: [report.md](report.md) → "Evidence — DoD 5: SCN-083-G08" (`TestCardRewardsRecommendationsE2E_G08` PASS; first run + post-gofmt re-run E2E_RERUN_EXIT=0; generate→GET recommendations+report on the live stack)
- [x] Build Quality Gate: build/check/lint/format clean (zero warnings); reasons recorded for explainability (Principle 8); artifact-lint clean; docs aligned — Evidence: [report.md](report.md) → "Evidence — DoD 6: Build Quality Gate" + "Gate: artifact-lint — Scope 07" (CHECK_EXIT=0, LINT_EXIT=0, FORMAT_RECHECK_EXIT=0; artifact-lint recorded there)

---

## Scope 08: CalDAV Calendar Delivery

**Status:** Not Started
**Priority:** P1
**Depends On:** 07
**Spec Refs:** FR-CR-015, UC-005, design §7

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-H01 — monthly recommendation creates a CalDAV event
  Given a card_recommendations row for 2026-06 / "Restaurants"
  When the calendar bridge syncs
  Then a CalDAV event with a stable UID smackerel-cardrec-2026-06-restaurants is written

Scenario: SCN-083-H02 — re-sync updates the same event (no duplicate)
  Given a recommendation already synced with a UID
  When the recommendation rate changes and the bridge re-syncs
  Then the existing event is updated (same UID), not duplicated

Scenario: SCN-083-H03 — re-enrollment reminder event created
  Given a selectable card with a due re-enrollment window
  When the bridge syncs
  Then a CalDAV event smackerel-cardreenroll-<user_card_id>-<period> is written

Scenario: SCN-083-H04 — calendar_sync disabled skips writes but keeps Web UI data
  Given card_rewards.calendar_sync is false
  When recommendation generation runs
  Then no CalDAV event is written and recommendations remain visible in the Web UI

Scenario: SCN-083-H05 — deleting a recommendation cleans up its event
  Given a recommendation with a calendar_event_uid
  When the recommendation is removed
  Then the corresponding CalDAV event is deleted

Scenario: SCN-083-H06 — calendar sync run is audited
  Given the bridge syncs
  Then a card_runs row with run_type="calendar_sync" records events_written
```

### Implementation Plan

- `internal/cardrewards/calendar.go`: `CardCalendarBridge` over the shared CalDAV client (reuse `internal/mealplan` `CalDAVClient` shape + `internal/connector/caldav` credentials); stable UID scheme; update-not-duplicate; cleanup on delete; `card_runs` calendar_sync rows.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-08-01 | Unit | `internal/cardrewards/calendar_test.go` | SCN-083-H01, H02 | Stable UID create + update-not-duplicate (fake CalDAVClient) |
| T-08-02 | Unit | `internal/cardrewards/calendar_test.go` | SCN-083-H03, H05 | Re-enrollment event + cleanup on delete |
| T-08-03 | Unit | `internal/cardrewards/calendar_test.go` | SCN-083-H04 | calendar_sync disabled skips writes |
| T-08-04 | Integration | `internal/cardrewards/calendar_test.go` | SCN-083-H06 | Calendar sync run audited — live PG |

### Definition of Done

- [ ] Implementation behavior complete: CalendarBridge writes/updates/deletes CalDAV events with stable UIDs (reusing the mealplan pattern + caldav credentials); disabled-sync path keeps Web UI data; runs audited
- [ ] Scenario tests pass for SCN-083-H01 and SCN-083-H02 (stable UID create + update-not-duplicate) — unit
- [ ] Scenario tests pass for SCN-083-H03 and SCN-083-H05 (re-enrollment event + cleanup) — unit
- [ ] Scenario test passes for SCN-083-H04 (calendar_sync disabled) — unit
- [ ] Scenario test passes for SCN-083-H06 (calendar sync run audited) — integration (live PG)
- [ ] Build Quality Gate: build/check/lint/format clean (zero warnings); the CalDAVClient mock is an external-boundary fake (calendar server), not an internal-component mock; artifact-lint clean; docs aligned

---

## Scope 09: Scheduler Jobs & Manual Triggers

**Status:** Not Started
**Priority:** P1
**Depends On:** 04, 05, 06, 07, 08
**Spec Refs:** FR-CR-018, FR-CR-019, NFR-CR-005, design §8

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-I01 — daily refresh job registered on configured cron
  Given card_rewards.scrape_cron is set
  When the scheduler starts
  Then a card_rewards_refresh job is registered on that cron

Scenario: SCN-083-I02 — monthly recommend job registered on configured cron
  Given card_rewards.monthly_recommend_cron is set
  When the scheduler starts
  Then a card_rewards_recommend job is registered on that cron

Scenario: SCN-083-I03 — daily refresh runs the full pipeline
  Given the daily job fires
  When it executes
  Then it triggers connector sync → extract → reconcile → lifecycle and writes a card_runs row

Scenario: SCN-083-I04 — monthly job optimizes, recommends, and syncs calendar
  Given the monthly job fires
  When it executes
  Then it runs optimize → recommend → calendar sync and writes a card_runs row

Scenario: SCN-083-I05 — manual "scrape now" trigger reuses the refresh code path
  Given an operator triggers scrape-now
  When it executes
  Then it runs the same refresh pipeline with trigger="manual"

Scenario: SCN-083-I06 — re-running a job is idempotent
  Given the refresh job runs twice on the same source data
  Then rotating_categories rows are upserted (no duplicates) and calendar events are updated not duplicated
```

### Implementation Plan

- `internal/scheduler/cardrewards.go`: register `card_rewards_refresh` + `card_rewards_recommend` via `registration.go`; manual triggers call the same pipelines with `trigger="manual"`; idempotent.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-09-01 | Unit | `internal/scheduler/cardrewards_test.go` | SCN-083-I01, I02 | Jobs registered on configured crons |
| T-09-02 | Integration | `internal/scheduler/cardrewards_test.go` | SCN-083-I03, I04 | Full daily + monthly pipelines run + audited — live PG |
| T-09-03 | Integration | `internal/scheduler/cardrewards_test.go` | SCN-083-I05, I06 | Manual trigger reuse + idempotency — live PG |

### Definition of Done

- [ ] Implementation behavior complete: daily refresh and monthly recommend jobs registered on configured crons; manual triggers reuse the same pipelines; idempotent
- [ ] Scenario tests pass for SCN-083-I01 and SCN-083-I02 (jobs registered on crons) — unit
- [ ] Scenario tests pass for SCN-083-I03 and SCN-083-I04 (full daily + monthly pipelines audited) — integration (live PG)
- [ ] Scenario tests pass for SCN-083-I05 and SCN-083-I06 (manual trigger reuse + idempotency) — integration (live PG)
- [ ] Build Quality Gate: build/check/lint/format clean (zero warnings); manual + scheduled paths share code (NFR-CR-005); artifact-lint clean; docs aligned

---

## Scope 10: Web UI — Wallet, Offers, Selections, Bonuses, Categories

**Status:** Not Started
**Priority:** P1
**Depends On:** 02
**Spec Refs:** FR-CR-016, NFR-CR-006, UC-001/003/004, design §9

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-J01 — wallet page lists owned cards
  Given the user holds cards
  When the user opens /cards/wallet
  Then the page renders each card with its nickname, type, note, and active state

Scenario: SCN-083-J02 — add card via discovery
  Given the catalog contains Citi Custom Cash
  When the user types "custom cash", selects the candidate, and confirms
  Then a user card is created and shown on the wallet page

Scenario: SCN-083-J03 — add custom (non-catalog) card
  Given a description matching no catalog card
  When the user completes the add-custom flow
  Then a custom card is created and shown

Scenario: SCN-083-J04 — edit a card and add a note
  Given an owned card
  When the user edits it and saves a per-card note
  Then the changes persist and re-render on reload

Scenario: SCN-083-J05 — toggle card activation
  Given an active card
  When the user toggles activation off
  Then the card shows inactive and is excluded from optimization

Scenario: SCN-083-J06 — add and edit an offer with a shared limit group
  Given an owned card
  When the user adds an offer with a shared limit group and later edits it
  Then the offer persists with the shared_limit_group and edits round-trip

Scenario: SCN-083-J07 — tiered selection save
  Given a tiered selectable card
  When the user saves tier-1 and tier-2 categories for the period
  Then selections persist and the tiered view re-renders them

Scenario: SCN-083-J08 — manage category names, equivalents, and starred
  Given the categories page
  When the user adds an equivalent and stars a category
  Then category_aliases reflects the change and the dashboard ordering updates
```

### Implementation Plan

- `internal/web/cardrewards.go` + `cardrewards_templates.go`: chi routes + embedded `html/template` pages for wallet (list/add-discovery/add-custom/confirm/edit/note/remove/toggle), offers (list/add/edit/remove/toggle), selections (list/add/edit/tiered), bonuses (list/add/progress), categories (names/equivalents/starred/priority). Design-token styling; behind existing auth/CSP.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-10-01 | E2E UI | `tests/e2e-ui/cardrewards_wallet.spec.ts` | SCN-083-J01..J05 | Wallet list/add-discovery/add-custom/edit-note/toggle — live stack |
| T-10-02 | E2E UI | `tests/e2e-ui/cardrewards_offers_selections.spec.ts` | SCN-083-J06, J07 | Offer shared-limit; tiered selection — live stack |
| T-10-03 | E2E UI | `tests/e2e-ui/cardrewards_categories.spec.ts` | SCN-083-J08 | Category equivalents + starring — live stack |

### Definition of Done

- [ ] Implementation behavior complete: server-rendered wallet/offers/selections/bonuses/categories pages with full CRUD parity; behind existing auth/CSP; design tokens (no hardcoded colors)
- [ ] Scenario tests pass for SCN-083-J01..J05 (wallet CRUD incl. discovery/custom/note/toggle) — e2e-ui (live stack, no request interception)
- [ ] Scenario tests pass for SCN-083-J06 and SCN-083-J07 (offer shared-limit; tiered selection) — e2e-ui (live stack)
- [ ] Scenario test passes for SCN-083-J08 (category management) — e2e-ui (live stack)
- [ ] Build Quality Gate: build/check/lint/format clean (zero warnings); e2e-ui tests hit the real stack with no `page.route`/`intercept`; Docker bundle freshness verified for UI; artifact-lint clean; docs aligned

---

## Scope 11: Web UI — Dashboard, Recommendations, Rotating Verify, Report, Admin

**Status:** Not Started
**Priority:** P1
**Depends On:** 06, 07, 09, 10
**Spec Refs:** FR-CR-016, FR-CR-019, UC-002/005/006/007, Principle 8, design §9

### Gherkin Scenarios

```gherkin
Scenario: SCN-083-K01 — dashboard shows current categories, recommendations, and pending actions
  Given active rotating categories, this month's recommendations, and pending actions exist
  When the user opens /cards
  Then the dashboard renders all three, including any needs_verification and re-enrollment alerts

Scenario: SCN-083-K02 — recommendations page supports view/add/edit/star
  Given recommendations exist for the period
  When the user adds a category, edits one, and stars another
  Then the changes persist and re-render

Scenario: SCN-083-K03 — starred override is honored on regeneration from the UI
  Given the user starred-overrides a recommendation
  When recommendations are regenerated
  Then the override is preserved in the UI

Scenario: SCN-083-K04 — rotating verify page shows confidence and needs_verification badge
  Given a reconciled record flagged needs_verification
  When the user opens /cards/rotating
  Then the record shows its confidence, the needs_verification badge, and its source citation

Scenario: SCN-083-K05 — manual verify/override clears the flag
  Given a needs_verification record
  When the user edits/confirms it (manual override)
  Then needs_verification is cleared, manual_override is set, and future extraction does not overwrite it

Scenario: SCN-083-K06 — report page renders the full optimization report
  Given current card data and recommendations
  When the user opens /cards/report
  Then the optimization report renders best-card-per-category with reasons

Scenario: SCN-083-K07 — admin page triggers scrape-now and shows run history
  Given the admin page
  When the operator clicks "scrape now"
  Then the refresh pipeline runs and a new run appears in the run-history log

Scenario: SCN-083-K08 — admin page triggers sync-calendar-now
  Given the admin page and calendar_sync enabled
  When the operator clicks "sync calendar now"
  Then the calendar bridge runs and the run is logged with events_written
```

### Implementation Plan

- `internal/web/cardrewards.go` (dashboard, recommendations, rotating verify, report) + `internal/web/admin/` (run history + manual triggers wired to scheduler manual triggers). Confidence/needs_verification badges; source citations (Principle 8). Optional read-only PWA recommendation card (Open Question 4).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-11-01 | E2E UI | `tests/e2e-ui/cardrewards_dashboard.spec.ts` | SCN-083-K01, K06 | Dashboard + report render — live stack |
| T-11-02 | E2E UI | `tests/e2e-ui/cardrewards_recommendations.spec.ts` | SCN-083-K02, K03 | Recommendations view/add/edit/star + override — live stack |
| T-11-03 | E2E UI | `tests/e2e-ui/cardrewards_rotating_verify.spec.ts` | SCN-083-K04, K05 | Verify badge + manual override clears flag — live stack |
| T-11-04 | E2E UI | `tests/e2e-ui/cardrewards_admin.spec.ts` | SCN-083-K07, K08 | Admin scrape-now + sync-now + run history — live stack |

### Definition of Done

- [ ] Implementation behavior complete: dashboard, recommendations, rotating-verify, report, and admin pages render with confidence/needs_verification badges and source citations; admin triggers wired to scheduler manual triggers
- [ ] Scenario tests pass for SCN-083-K01 and SCN-083-K06 (dashboard + report) — e2e-ui (live stack)
- [ ] Scenario tests pass for SCN-083-K02 and SCN-083-K03 (recommendations CRUD + override) — e2e-ui (live stack)
- [ ] Scenario tests pass for SCN-083-K04 and SCN-083-K05 (verify badge + manual override clears flag) — e2e-ui (live stack)
- [ ] Scenario tests pass for SCN-083-K07 and SCN-083-K08 (admin scrape-now/sync-now + run history) — e2e-ui (live stack)
- [ ] Build Quality Gate: build/check/lint/format clean (zero warnings); e2e-ui hits real stack (no interception); Docker bundle freshness verified; artifact-lint clean; docs aligned

---

## Shared Planning Expectations

- **Test environment isolation:** every live-stack test (integration, e2e-api,
  e2e-ui, stress) uses ephemeral storage and the spec-043 test Ollama. No
  internal-component mocks; the only permitted fakes are external boundaries
  (the CalDAV server in Scope 08 unit tests).
- **Adversarial regressions (Scopes 05, 06):** each adversarial test must use
  input that the OLD regex silent-fallback path would have "passed" (by serving
  stale/placeholder data) but that the new contract MUST fail-loud to
  `needs_verification`. No tautological tests.
- **Config SST / no-defaults:** no scope may introduce `${VAR:-default}`
  fallbacks; all tunables come from `config/smackerel.yaml`.
- **Release train:** the `card_rewards` flag bundle edit
  (`config/feature-flags.mvp.yaml` default-ON; others default-OFF) is performed
  during delivery by `bubbles.train`, not in any scope here.
- **Delivery gating:** this plan is `specs_hardened`. Promotion to delivery
  (implementation) is a separate `full-delivery` (or equivalent) workflow.
