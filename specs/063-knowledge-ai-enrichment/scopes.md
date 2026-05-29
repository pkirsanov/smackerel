# Scopes — Spec 063 Knowledge AI Enrichment

> **Owner:** bubbles.plan. Authored 2026-05-29. Workflow mode `product-to-planning` (status ceiling `specs_hardened`).
> **Inputs (read-only):** [spec.md](spec.md), [design.md](design.md), [scenario-manifest.json](scenario-manifest.json).
> **Substrate boundary:** Per [spec.md §13](spec.md#13-routing-note) and [design.md §3](design.md#3-module-layout), NO scope may modify any file under `internal/agent/` (except additive `internal/agent/tools/enrichment/`), any file under `internal/assistant/` (substrate FROZEN), `internal/intelligence/synthesis.go` (spec 025 canonical), `internal/intelligence/alert_producers.go|briefs.go` (spec 021), `internal/extract/`, `ml/app/{domain,synthesis}.py` (spec 026), or `internal/notification/` (spec 054). Plan-phase routing decisions for OQ-PLAN-5 missing publishers are in the Routing section below.

---

## Execution Outline (alignment checkpoint)

### Phase order (13 scopes, sequential)

| # | Scope | Surface | SCN-mapping | Foundation? |
|---|-------|---------|-------------|-------------|
| 01 | SST keys + config validation | `config/smackerel.yaml`, `internal/config/enrichment.go` | infra | **foundation** |
| 02 | Migration 045 (enrichment_why + consolidation_candidates + enrichment_token_ledger + INFERRED edges index) | `internal/db/migrations/045_knowledge_enrichment.sql` | infra | **foundation** |
| 03 | `EnrichmentProducer` interface + foundation types + 7 architecture tests | `internal/knowledge/enrichment/{producer,evidence,provenance,refusal,scheduler,nats_subjects,architecture_test}.go` | (design §13 arch tests) | **foundation** |
| 04 | Token-budget ledger gate (80% soft / 100% hard / cheap-exceeds refuse) | `.../token_budget.go`, `internal/metrics/counters.go` (append) | UX §14.E | **foundation** |
| 05 | Refusal contract + min-sources gate + Prometheus counters | `.../refusal.go`, `internal/metrics/counters.go` | UX §14.F | **foundation** |
| 06 | `ResynthesisProducer` (R-1, R-2, R-3, R-11) + no-thinning guard | `.../resynthesis.go` + scenario YAML | SCN-063-001/002/003 | overlay |
| 07 | `RelationshipInferenceProducer` (R-4, R-5) + candidate-pair selector SQL | `.../relationship_inference.go` + scenario YAML | SCN-063-004/005 | overlay |
| 08 | `WhyAugmenterProducer` (R-6, R-7) | `.../why_augmenter.go` + scenario YAML | SCN-063-006/007 | overlay |
| 09 | `ConsolidationAnalyzerProducer` (R-8) + 90-day TTL retention | `.../consolidation_analyzer.go` + scenario YAML | SCN-063-008 | overlay |
| 10 | Reactive `knowledge_lookup` scenario + facade integration (R-9) + UX §14.B disclosure footer | `internal/agent/tools/enrichment/knowledgelookup/`, `config/prompt_contracts/enrichment-knowledge-lookup-v1.yaml`, `config/assistant/scenarios.yaml` (append row) | SCN-063-009/010 | overlay |
| 11 | Per-tick budget calibration (load test) — resolves OQ-PLAN-1 final values | `tests/load/enrichment_load_test.go`, SCOPE-01 SST values bumped per evidence | NFR-2 | overlay |
| 12 | Architecture-test CI wiring — resolves OQ-PLAN-3 (reuse spec 062 pattern: co-located `architecture_test.go` + adversarial sub-tests; NO new CI workflow) | `.github/workflows/ci.yml` already runs `./smackerel.sh test unit` which picks up arch tests | (design §13) | overlay |
| 13 | Docs (`docs/smackerel.md` Knowledge Enrichment section + operator runbook in `docs/Operations.md`) | docs only | — | overlay |

### New types & signatures introduced (header-only)

```go
// internal/knowledge/enrichment/producer.go
type EnrichmentProducer interface {
    Name() string
    Enqueue(ctx context.Context, trigger Trigger) ([]Job, error)
    RunJob(ctx context.Context, job Job) (Result, error)
    ApplyOutput(ctx context.Context, result Result) error
    DrainBacklog(ctx context.Context, budget int) (processed int, err error)
}
type Trigger struct { Kind TriggerKind; TargetID, TargetKind string; OccurredAt time.Time }
type Job struct { ProducerName, TargetKind, TargetID string; EvidenceSet EvidenceSet; EnqueuedAt time.Time }
type EvidenceSet struct { ArtifactIDs, ConceptPageIDs []string }
type Result struct { Job Job; Output *Output; Refusal *Refusal; Provenance ProvenanceRecord }
type ProvenanceRecord struct {
    PromptContractVersion, LLMRunID, ModelID string
    EvidenceArtifactIDs, EvidenceConceptPageIDs []string
    Confidence float64
}
type Refusal struct { Reason string } // closed vocabulary per design §2.2

// internal/knowledge/enrichment/token_budget.go
type Gate interface {
    Admit(ctx context.Context, surface string, estimatedTokens int) (Decision, error)
}
type Decision struct { Outcome DecisionOutcome; ModelID string } // PROCEED|DOWNGRADE|REFUSE

// internal/agent/tools/enrichment/knowledgelookup/facade_assembler.go
type Assembler struct{ /* ... */ }
func (a *Assembler) Assemble(ctx context.Context, raw map[string]any) (contracts.SourceAssembly, error)
```

### Validation checkpoints

- **After SCOPE-03:** all 7 architecture tests green (foundation surface validated before any overlay producer runs).
- **After SCOPE-05:** refusal contract + min-sources gate validated standalone — provides confidence that overlay producers cannot silently fabricate.
- **After SCOPE-10:** end-to-end reactive path (SCN-063-009/010) validated against live facade — proves the spec 061 provenance gate refuses empty `sources[]` from the new scenario.
- **After SCOPE-11:** per-tick budget values stamped into `config/smackerel.yaml` are operationally validated (no cost cliff on representative graph).

### Foundation-first ordering (Capability Foundation Design)

Per design §2 the `EnrichmentProducer` interface plus 4 background overlays + 1 reactive surface (5 concrete implementations). Foundation scopes SCOPE-01..SCOPE-05 MUST land before any overlay (SCOPE-06+). Each overlay `Depends On: SCOPE-05`. SCOPE-10 (reactive) additionally `Depends On: SCOPE-04` (token-budget gate is the §14.E downgrade-then-refuse contract).

---

## Open Question Resolutions (plan-owned)

| OQ | Resolution | Pointer |
|----|-----------|---------|
| OQ-PLAN-1 (per-tick budget) | Recommended starting values stamped into SCOPE-01 config. Empirical calibration deferred to SCOPE-11 load test; if load evidence shows different values, SCOPE-11 updates SCOPE-01 config in-place. Initial values: `resynthesis` (cadence 300s, per_tick 10, backlog_cap 500), `relationship_inference` (cadence 900s, per_tick 20, backlog_cap 200), `why_augmenter` (cadence 120s, per_tick 20, backlog_cap 300), `consolidation_analyzer` (cadence 600s, per_tick 5, backlog_cap 50), `queue.capacity` 200 per producer, `daily_token_budget` 200000. Justification: keeps a Twitter-archive-sized ingest (12k artifacts ≈ 1.2k concept pages) drainable within ~6 hours at per-tick=10/300s. | SCOPE-01, SCOPE-11 |
| OQ-PLAN-2 (`consolidation_candidates` retention) | **Persist with 90-day TTL + manual cleanup job.** UX §14.D commits "persisted-but-inert" — data must NOT vanish from under the user while still cold-stored. 90 days balances graph hygiene against the user's pull-only access pattern. Cleanup runs in scheduler, soft-deletes rows where `created_at < NOW() - 90d` AND `last_surfaced_at IS NULL`. SST key `enrichment.producers.consolidation_analyzer.retention_days` REQUIRED at startup. | SCOPE-09, SCOPE-01 |
| OQ-PLAN-3 (arch-test CI wiring) | **Reuse spec 062 pattern.** Spec 062 SCOPE-04 places architecture tests in `internal/intelligence/forward_looking/architecture_test.go` co-located with the foundation; CI picks them up automatically via `./smackerel.sh test unit --go`. Spec 063 mirrors: `internal/knowledge/enrichment/architecture_test.go` (foundation-owned, 7 tests + adversarial `t.Run("would_catch_regression", ...)` sub-tests). NO new CI workflow file. | SCOPE-03, SCOPE-12 |
| OQ-PLAN-4 (candidate-pair selector SQL) | **RESOLVED — existing `knowledge_entities` schema is sufficient.** Verified [001_initial_schema.sql:458-477](../../internal/db/migrations/001_initial_schema.sql) provides `knowledge_entities(id, name_normalized, mentions JSONB, related_concept_ids TEXT[])` and `edges` table is polymorphic. Candidate-pair selector SQL (drafted in SCOPE-07 implementation plan) joins on shared `knowledge_entities.id` via `mentions->>'artifact_id'` while LEFT-JOINing `edges` to filter pairs the heuristic clustering already linked. No route-required packet to spec 025. | SCOPE-07 |
| OQ-PLAN-5 (NATS publisher availability in foreign substrate) | **MIXED: 1 reusable, 3 route-required packets.** Verified [internal/nats/client.go:17-100](../../internal/nats/client.go): `SubjectArtifactsProcessed = "artifacts.processed"` is already published by spec 026 pipeline → spec 063 `resynthesis` subscribes (NO packet needed). Missing publishers for (a) `topic.edited|merged` (spec 025 substrate), (b) `intelligence.alert_emitted` (spec 021 substrate), (c) `intelligence.recommendation_emitted|brief_emitted` (spec 021 substrate). Three route-required packets queued (see Routing section). UNTIL those packets land, SCOPE-08/09 ship with **cron-only triggers** (poll for new alert/recommendation/brief/topic-edit rows since last successful tick) — design.md §4 already declares cron-drain is the catch-up path; cron-only is a degraded-but-correct fallback per P9 (Design For Restart). | SCOPE-08, SCOPE-09, Routing section |

---

## Routing (packets to other spec owners)

Three route-required packets surfaced by OQ-PLAN-5. None block spec 063 implementation (cron-only fallback per OQ-PLAN-5 above); landing them upgrades event-driven enqueue from "next tick after row insert" to "immediate".

| Packet | Owner spec | Substrate file to add publisher | Subject to add | Rationale |
|--------|-----------|--------------------------------|----------------|-----------|
| PKT-063-A | spec 025 (knowledge synthesis) | `internal/intelligence/synthesis.go` or sibling topic-mutation site | `topic.edited` / `topic.merged` | Enables event-driven `ConsolidationAnalyzerProducer.Enqueue` (R-8, SCN-063-008). Without it, cron poll picks up topic edits with up to `cadence_seconds` (600s) latency. |
| PKT-063-B | spec 021 (intelligence delivery) | `internal/intelligence/alert_producers.go` | `intelligence.alert_emitted` | Enables event-driven `WhyAugmenterProducer.Enqueue` for alerts (R-6, SCN-063-006). |
| PKT-063-C | spec 021 (intelligence delivery) | `internal/intelligence/briefs.go` + recommendation producer | `intelligence.recommendation_emitted` / `intelligence.brief_emitted` | Enables event-driven `WhyAugmenterProducer.Enqueue` for recommendations and briefs (R-6). |

Packets are documented here for spec 063's lifecycle; the orchestrator (bubbles.workflow) is the dispatcher when implementation begins.

---

## Scope 01: SST Keys + Config Validation

**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** None

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-CFG-01 — Missing enrichment.* key aborts startup
  Given config/smackerel.yaml omits enrichment.daily_token_budget
  When the runtime starts and config validation runs
  Then process exits non-zero with "[F063-SST-MISSING] missing or invalid required enrichment configuration: enrichment.daily_token_budget"

Use case: SCN-063-CFG-02 — All keys present resolves cleanly
  Given config/smackerel.yaml provides every enrichment.* key per design §10
  When the runtime starts
  Then no F063-SST-MISSING error is emitted and producers register
```

### Implementation Plan

- Append `enrichment:` block to `config/smackerel.yaml` per [design.md §10](design.md#10-sst-keys--final-fail-loud-required-set) verbatim, with initial values per OQ-PLAN-1 resolution above (zero literal fallbacks).
- New: `internal/config/enrichment.go` mirroring `internal/config/assistant.go` pattern — typed struct, per-field non-empty / range validation, fail-loud error formatting `[F063-SST-MISSING] ...`.
- Wire into existing master validator chain (`internal/config/validate.go`).
- Initial values stamped into SST (operator may override; SCOPE-11 may revise):
  ```yaml
  enrichment:
    global_enabled: true
    queue: { capacity: 200 }
    disclosure: { staleness_minutes: 60, backlog_threshold: 100 }
    daily_token_budget: 200000
    cap_reset_timezone: "Europe/Amsterdam"
    refusal: { min_sources_required: 2 }
    producers:
      resynthesis: { enabled: true, cadence_seconds: 300, per_tick_budget: 10, backlog_cap: 500, prompt_contract_version: "enrichment-resynthesis-v1", model_provider: "gemma3:4b", no_thinning_guard_enabled: true }
      relationship_inference: { enabled: true, cadence_seconds: 900, per_tick_budget: 20, backlog_cap: 200, confidence_floor: 0.70, candidate_selector_limit: 100, prompt_contract_version: "enrichment-relationship-inference-v1", model_provider: "deepseek-r1:7b" }
      why_augmenter: { enabled: true, cadence_seconds: 120, per_tick_budget: 20, backlog_cap: 300, confidence_floor: 0.50, prompt_contract_version: "enrichment-why-augmenter-v1", model_provider: "gemma3:4b" }
      consolidation_analyzer: { enabled: true, cadence_seconds: 600, per_tick_budget: 5, backlog_cap: 50, confidence_floor: 0.75, retention_days: 90, prompt_contract_version: "enrichment-consolidation-analyzer-v1", model_provider: "deepseek-r1:7b" }
    reactive:
      knowledge_lookup: { enabled: true, prompt_contract_version: "enrichment-knowledge-lookup-v1", model_provider_primary: "gemma3:4b", model_provider_fallback: "gemma3:4b", latency_budget_ms: 5000 }
  ```

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/config/enrichment_test.go` | Each required key absent → `[F063-SST-MISSING]` error citing field name | design §10 |
| unit adversarial | same | `daily_token_budget: 0` rejected; `confidence_floor: 1.5` rejected; `cap_reset_timezone: "invalid/tz"` rejected | smackerel-no-defaults |
| unit | same | All keys present → validator returns nil; struct populated with expected typed values | SCN-063-CFG-02 |
| Regression E2E | `tests/e2e/enrichment_config_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-063-CFG-01/02 — startup aborts on missing `enrichment.*` keys; all-keys-present resolves cleanly against the live stack config-validation path | SCN-063-CFG-01/02 |

### Definition of Done

- [ ] SCN-063-CFG-01 — Missing key aborts startup: verified by `enrichment_test.go::TestMissingKeyFailsLoud` per-field table
- [ ] SCN-063-CFG-02 — All keys present resolves cleanly: verified by `enrichment_test.go::TestAllKeysResolve`
- [ ] Zero `os.Getenv(..., "fallback")`, zero `${VAR:-default}`, zero `if cfg.X == 0 { cfg.X = N }` in `internal/config/enrichment.go`
- [ ] `enrichment:` block in `config/smackerel.yaml` matches design §10 key set 1:1 (lint check)
- [ ] Build Quality (build/lint/format/unit) green; evidence captured per [bubbles-evidence-capture](../../.github/skills/bubbles-evidence-capture/SKILL.md) (PII-redacted)
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_config_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping recorded in `scenario-manifest.json` (SCN-063-CFG-01/02)

---

## Scope 02: Migration 045 — Enrichment Tables + INFERRED Edges Index

**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** SCOPE-01

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-MIG-01 — Migration 045 applies cleanly
  Given a fresh test database
  When migration 045_knowledge_enrichment.sql runs
  Then enrichment_why, consolidation_candidates, enrichment_token_ledger tables exist
  And the partial index idx_edges_type_inferred is created
  And no existing rows in edges, concept_pages, alerts, or topics are mutated

Use case: SCN-063-MIG-02 — CHECK constraints reject invalid values
  Given the migration is applied
  When an INSERT into enrichment_why with parent_kind='bogus' is attempted
  Then the CHECK constraint rejects the row
  And similar rejections fire for confidence > 1.0, confidence < 0.0
```

### Implementation Plan

- New: `internal/db/migrations/045_knowledge_enrichment.sql` containing verbatim:
  - `enrichment_why` table per [design.md §6](design.md#6-resolves-oq-3-storage-shape-for-why-prose)
  - `consolidation_candidates` table (columns: `id TEXT PK`, `topic_pair_ids TEXT[]`, `decision TEXT CHECK (decision IN ('merge','keep_separate','needs_more_data'))`, `confidence REAL CHECK (0.0 <= confidence <= 1.0)`, `evidence_artifact_ids TEXT[]`, `prompt_contract_version TEXT NOT NULL`, `llm_run_id TEXT NOT NULL`, `model_id TEXT NOT NULL`, `last_surfaced_at TIMESTAMPTZ`, `superseded_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`)
  - `enrichment_token_ledger` table per [design.md §11](design.md#11-token-cost-budget-mechanism-ux-14e)
  - `CREATE INDEX IF NOT EXISTS idx_edges_type_inferred ON edges(edge_type) WHERE edge_type LIKE 'INFERRED_%';`
- All tables use `IF NOT EXISTS`; CHECK constraints inline.
- **NO** modifications to existing `edges`, `concept_pages`, alert/recommendation/brief tables (read-only substrate).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| integration | `internal/db/migrations/migration_045_test.go` | Migration applies; tables exist with expected columns | SCN-063-MIG-01 |
| integration adversarial | same | INSERTs violating each CHECK constraint rejected (parent_kind, decision, confidence range) | SCN-063-MIG-02 |
| integration | same | After migration, `SELECT COUNT(*) FROM edges` unchanged; existing concept_pages/alerts rows untouched (heuristic-untouched guarantee) | design §7 |
| Regression E2E | `tests/e2e/enrichment_migration_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-063-MIG-01/02 — live stack migration apply, table presence assertions, CHECK-constraint adversarial inserts, edges row-count snapshot | SCN-063-MIG-01/02 |

### Definition of Done

- [ ] SCN-063-MIG-01 — Migration applies cleanly: verified by `migration_045_test.go::TestApply`
- [ ] SCN-063-MIG-02 — CHECK constraints enforced: verified by adversarial sub-tests per constraint
- [ ] Partial index `idx_edges_type_inferred` exists and covers `INFERRED_%` filter
- [ ] Zero mutations to existing tables (verified by row-count snapshot before/after)
- [ ] Build Quality green; integration evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_migration_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json` (SCN-063-MIG-01/02)

---

## Scope 03: `EnrichmentProducer` Foundation + 7 Architecture Tests

**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** SCOPE-02

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-FOUND-01 — Producer interface contract holds
  Given a mock EnrichmentProducer registered with the scheduler
  When Enqueue, RunJob, ApplyOutput, DrainBacklog are invoked
  Then each method signature matches design §2.1 verbatim
  And EvidenceSet+ProvenanceRecord types are populated end-to-end

Use case: SCN-063-ARCH-01..07 — Architecture invariants from design §13
  Given the spec 063 source tree under internal/knowledge/enrichment/
  When the 7 architecture tests run
  Then NoFacadeMutation, NoAgentRuntimeMutation, NoDirectOllamaHTTP,
       NoHeuristicEdgeMutation, NoHeuristicSynthesisCall, NoNotificationCall,
       RefusalCopyConstants all pass
  And each carries a "would_catch_regression" adversarial sub-test
```

### Implementation Plan

- New: `internal/knowledge/enrichment/producer.go` (`EnrichmentProducer` interface + `Trigger`/`Job`/`Result`/`Refusal`/`ProvenanceRecord` types per design §2.1–§2.2).
- New: `internal/knowledge/enrichment/evidence.go` (`EvidenceSet` constructor + citation-set-violation check).
- New: `internal/knowledge/enrichment/provenance.go` (ULID minting for `LLMRunID`).
- New: `internal/knowledge/enrichment/scheduler.go` (per-tick driver — NATS subscribe + cron tick fan-out; producer registry).
- New: `internal/knowledge/enrichment/nats_subjects.go` (subject constants: `enrichment.trigger.artifact.{arrived,mutated,removed}`, `enrichment.trigger.intelligence.{alert,recommendation,brief}_emitted`, `enrichment.trigger.topic.{edited,merged}`).
- New: `internal/knowledge/enrichment/architecture_test.go` per design §13 with 7 tests, each containing `t.Run("would_catch_regression", ...)` per [bubbles-test-integrity](../../.github/skills/bubbles-test-integrity/SKILL.md).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `producer_test.go` | Interface satisfied by stub; types round-trip | SCN-063-FOUND-01 |
| unit | `architecture_test.go::TestNoFacadeMutation` | import-graph + git-blame scan rejects enrichment commit touching `internal/assistant/facade.go` | design §13 |
| unit | same | TestNoAgentRuntimeMutation — no spec-063 mutation of `internal/agent/{router,executor,registry,nats_driver,tracer}.go` | design §13 |
| unit | same | TestNoDirectOllamaHTTP — AST scan rejects `net/http` calls to `/api/generate` or `:11434` from `internal/knowledge/enrichment/` | design §13 |
| unit | same | TestNoHeuristicEdgeMutation — AST scan rejects `UPDATE edges`/`DELETE FROM edges` SQL literals where edge_type filter is not `INFERRED_%` | design §13 |
| unit | same | TestNoHeuristicSynthesisCall — only `resynthesis.go` may UPDATE concept_pages; all updates go through `applyOutputIfNotThinning` | design §13 |
| unit | same | TestNoNotificationCall — import-graph asserts `internal/knowledge/enrichment/` does NOT import `internal/notification/` | design §13 |
| unit | same | TestRefusalCopyConstants — golden-file comparison of `refusal.go` constants against UX §14.F canonical strings | design §13 |
| unit adversarial | each arch test | `t.Run("would_catch_regression", ...)` sub-test constructs the forbidden pattern in tempdir/in-memory fixture and asserts gate trips | design §13 |
| Regression E2E | `tests/e2e/enrichment_foundation_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-063-FOUND-01 + SCN-063-ARCH-01..07 — live stack boot proves the EnrichmentProducer registry wires the scheduler and all 7 architecture invariants remain green in the running binary | SCN-063-FOUND-01, SCN-063-ARCH-01..07 |

### Definition of Done

- [ ] SCN-063-FOUND-01 — interface contract: `producer_test.go::TestEnrichmentProducerInterface` green
- [ ] All 7 architecture tests pass on `./smackerel.sh test unit --go`
- [ ] Each architecture test carries a `would_catch_regression` adversarial sub-test that fails on the forbidden pattern
- [ ] `ProvenanceRecord` ULID minting verified deterministic
- [ ] SST zero-defaults: no inline producer names or subject strings outside `nats_subjects.go` constants
- [ ] Build Quality green; evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_foundation_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json` (SCN-063-FOUND-01, SCN-063-ARCH-01..07)

---

## Scope 04: Token-Budget Ledger Gate (UX §14.E)

**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** SCOPE-03

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-BUDGET-01 — Healthy state: PROCEED on primary model
  Given today's enrichment_token_ledger sum is 0.40 * daily_token_budget
  When Gate.Admit(surface, estimated_tokens=500) is called
  Then Decision.Outcome == PROCEED
  And Decision.ModelID == configured primary for the surface

Use case: SCN-063-BUDGET-02 — Soft cap at 80% triggers downgrade
  Given today's ledger sum is 0.85 * daily_token_budget
  When Gate.Admit is called
  Then Decision.Outcome == DOWNGRADE
  And Decision.ModelID == knowledge_lookup.model_provider_fallback

Use case: SCN-063-BUDGET-03 — Hard cap exhausted refuses
  Given remaining budget < estimated_tokens for cheap model
  When Gate.Admit is called
  Then Decision.Outcome == REFUSE
  And the caller records refusal_reason="daily_token_budget_exhausted"

Use case: SCN-063-BUDGET-04 — Calendar-day reset
  Given today's ledger has 1.0 * daily_token_budget consumed
  When the clock crosses midnight in enrichment.cap_reset_timezone
  Then Gate.Admit returns PROCEED for the first call on the new day
```

### Implementation Plan

- New: `internal/knowledge/enrichment/token_budget.go` implementing `Gate.Admit` per [design.md §11](design.md#11-token-cost-budget-mechanism-ux-14e) verbatim algorithm.
- Ledger writes use existing `pgxpool.Pool`; in-process 30s TTL cache for "today's used tokens" (disclosed in design §11) — cache invalidated on every successful `ApplyOutput`.
- Append `enrichment_token_*` Prometheus counters to `internal/metrics/counters.go`.
- Calendar-day boundary computed via `time.Now().In(loadTimezone(cfg.cap_reset_timezone))`.

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `token_budget_test.go` | 80% boundary fires DOWNGRADE; 100% fires REFUSE when cheap model also exceeds | SCN-063-BUDGET-01/02/03 |
| unit adversarial | same | Cache invalidation: a stale 30s cache MUST NOT permit a job that would push the live ledger past hard cap (test mutates ledger out-of-band and asserts next Admit reads fresh value after invalidation) | design §11 |
| integration | `token_budget_integration_test.go` | Calendar-day rollover in injected TZ → next-day ledger empty → PROCEED | SCN-063-BUDGET-04 |
| integration adversarial | same | Concurrent Admit calls under hard-cap pressure never over-admit (Postgres row-count assertion after 100 parallel Admit attempts) | NFR-2 |
| Regression E2E | `tests/e2e/enrichment_budget_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-063-BUDGET-01..04 — live stack budget Gate.Admit PROCEED/DOWNGRADE/REFUSE transitions + calendar-day rollover behavior | SCN-063-BUDGET-01..04 |

### Definition of Done

- [ ] SCN-063-BUDGET-01..03 — three Decision outcomes verified by table-driven test
- [ ] SCN-063-BUDGET-04 — TZ rollover verified by injected clock
- [ ] Adversarial: stale-cache regression test + concurrent-Admit regression test both pass
- [ ] Prometheus counters `enrichment_token_consumed_total`, `enrichment_budget_decision_total{outcome}` registered
- [ ] SST zero-defaults: `daily_token_budget` and `cap_reset_timezone` read from SST; 80%/100% thresholds are design-§11 documented constants
- [ ] Build Quality green; evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_budget_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 05: Refusal Contract + Min-Sources Gate + Counters (UX §14.F)

**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** SCOPE-04

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-REFUSE-01 — Min-sources gate refuses before LLM call
  Given a reactive query whose retrieval returns 1 source
  And enrichment.refusal.min_sources_required == 2
  When the gate evaluates the query
  Then the LLM is NOT invoked
  And the scenario returns empty cited_*_ids
  And the spec 061 provenance gate produces the canonical refusal body

Use case: SCN-063-REFUSE-02 — Closed-vocabulary refusal taxonomy
  Given each Refusal{Reason} value
  When recorded via refusal.Record()
  Then only the 5 closed values are accepted; any other value panics in tests
  And a Prometheus counter enrichment_refusal_total{producer,reason} increments
```

### Implementation Plan

- New: `internal/knowledge/enrichment/refusal.go` — closed-vocabulary `Reason` constants per design §2.2 + `Record(ctx, producer, reason)` that emits structured log + Prometheus counter.
- New: refusal copy string constants matching UX §14.F canonical text; architecture test `TestRefusalCopyConstants` from SCOPE-03 enforces parity.
- Min-sources gate function in `internal/agent/tools/enrichment/knowledgelookup/` (used by SCOPE-10) — cheap `COUNT(*)` SQL precondition; reads `enrichment.refusal.min_sources_required`.
- Append `enrichment_refusal_total{producer, reason}` counter to `internal/metrics/counters.go`.

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `refusal_test.go` | Closed vocabulary: only the 5 reasons accepted; unknown reason panics | SCN-063-REFUSE-02 |
| unit | same | `enrichment_refusal_total{producer,reason}` increments on Record() | NFR-3 |
| unit | same | Refusal copy constants match UX §14.F canonical strings byte-for-byte (golden file) | design §13 |
| integration | `min_sources_gate_test.go` | Sub-floor retrieval → LLM NOT invoked (assert via test-double LLM expecting zero calls); empty cited_*_ids returned | SCN-063-REFUSE-01 |
| Regression E2E | `tests/e2e/enrichment_refusal_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-063-REFUSE-01/02 — live stack min-sources gate skips LLM + closed-vocabulary refusal taxonomy + UX §14.F canonical copy parity | SCN-063-REFUSE-01/02 |

### Definition of Done

- [ ] SCN-063-REFUSE-01 — min-sources gate skips LLM: verified by `min_sources_gate_test.go` with test-double assertion
- [ ] SCN-063-REFUSE-02 — closed-vocabulary refusal taxonomy: verified by `refusal_test.go`
- [ ] Golden-file UX §14.F copy match (architecture test from SCOPE-03 wired)
- [ ] Prometheus counter increments verified
- [ ] SST zero-defaults: `min_sources_required` read from SST, no inline literal
- [ ] Build Quality green; evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_refusal_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 06: `ResynthesisProducer` (R-1/2/3/11)

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-05

### Use Cases (Gherkin) — SCN-063-001, 002, 003

See [spec.md §9](spec.md#9-user-scenarios-gherkin--representative-full-set-tracked-in-scenario-manifestjson-during-plan-phase) for verbatim text. Summary:
- **SCN-063-001** — new artifact ingest triggers re-synthesis; affected concept page advances with new evidence ID.
- **SCN-063-002** — no-thinning guard refuses strictly-thinner output; existing concept page preserved.
- **SCN-063-003** — backlog catch-up after 30-day offline drains oldest-first, bounded by per_tick_budget + backlog_cap; zero notifications.

### Implementation Plan

- New: `internal/knowledge/enrichment/resynthesis.go` implementing `EnrichmentProducer` for `Name() == "resynthesis"`.
- Subscribes to existing `SubjectArtifactsProcessed = "artifacts.processed"` per OQ-PLAN-5 resolution (no new publisher needed for resynthesis).
- `Enqueue`: `SELECT id FROM knowledge_concepts WHERE source_artifact_ids && ARRAY[new_artifact_id]` (uses existing gin index `idx_knowledge_concepts_source_artifacts`).
- `RunJob`: builds closed `EvidenceSet` from concept page's existing `source_artifact_ids` ∪ new artifact; invokes spec 037 executor with scenario `enrichment-resynthesis-v1`.
- `ApplyOutput`: **no-thinning guard** — counts new claims, summary tokens, cited artifact IDs; if strictly thinner, records `Refusal{Reason: "would_thin_existing_content"}` (per design §13 architecture test, all UPDATE concept_pages goes through `applyOutputIfNotThinning`).
- `DrainBacklog`: oldest-first by `triggers.OccurredAt`, bounded by `backlog_cap`.
- New scenario YAML: `config/prompt_contracts/enrichment-resynthesis-v1.yaml` per design §9 (input_schema = `{concept_page_id, current_summary, current_claims, evidence_artifact_ids}`; output_schema = `{summary, claims, cited_artifact_ids, confidence}`).

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| unit | `resynthesis_test.go::TestEnqueueOverlap` | Single new artifact → enqueues exactly the affected concept pages | SCN-063-001 |
| unit | `resynthesis_test.go::TestNoThinningGuard` | Output with fewer claims OR fewer cited IDs OR shorter summary → refused | SCN-063-002 |
| unit adversarial | same | Output with equal counts but DIFFERENT artifact IDs (re-attribution) → NOT refused (proves guard is not over-broad) | R-3 |
| integration | `resynthesis_integration_test.go` | Live Postgres + NATS: publish to `artifacts.processed`; observe `knowledge_concepts.updated_at` advance + new `source_artifact_ids` member | SCN-063-001 |
| integration | same | Backlog of 500 triggers drained oldest-first, capped at per_tick_budget per tick, total ≤ backlog_cap | SCN-063-003 |
| integration adversarial | same | 30-day offline simulation: zero `internal/notification/` calls observed during drain (proves R-13 + P6) | SCN-063-003 |
| e2e-api | `tests/e2e/enrichment_resynthesis_e2e_test.sh` | Live stack: insert artifact via API; poll until knowledge_concepts.updated_at advances; assert provenance triple present in row metadata | SCN-063-001 |
| Regression E2E | `tests/e2e/enrichment_resynthesis_e2e_test.sh` (extended with regression rows) + `tests/e2e/enrichment_resynthesis_regression_e2e_test.sh` (adversarial suite) | Regression: persistent scenario-specific E2E coverage for SCN-063-001/002/003 — re-ingest trigger, no-thinning guard refuses thinner output, backlog-drain bounded with zero notifications | SCN-063-001/002/003 |

### Definition of Done

- [ ] SCN-063-001 — verified by integration test `resynthesis_integration_test.go::TestEnqueueOnArtifactsProcessed`
- [ ] SCN-063-002 — verified by `resynthesis_test.go::TestNoThinningGuard` + adversarial sub-test
- [ ] SCN-063-003 — verified by integration backlog-drain test + zero-notification adversarial assertion
- [ ] `enrichment-resynthesis-v1.yaml` validated by `cmd/scenario-lint`
- [ ] SST zero-defaults: all behavior reads from SCOPE-01 config; no inline literals
- [ ] Architecture tests from SCOPE-03 (NoHeuristicEdgeMutation, NoHeuristicSynthesisCall) remain green
- [ ] Build Quality green; integration + e2e evidence captured (PII-redacted)
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_resynthesis_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 07: `RelationshipInferenceProducer` (R-4/5)

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-05

### Use Cases (Gherkin) — SCN-063-004, 005

### Implementation Plan

- New: `internal/knowledge/enrichment/relationship_inference.go`.
- **Cron-only** (per design §4 — no event source signals candidate-pair novelty).
- Candidate-pair selector SQL (per OQ-PLAN-4 resolution — existing schema sufficient):
  ```sql
  -- artifacts sharing >=1 knowledge_entity but NOT already co-clustered or already inferred
  SELECT a1.id, a2.id
    FROM knowledge_entities ke
    CROSS JOIN LATERAL jsonb_array_elements(ke.mentions) m1
    CROSS JOIN LATERAL jsonb_array_elements(ke.mentions) m2
    JOIN artifacts a1 ON a1.id = (m1->>'artifact_id')
    JOIN artifacts a2 ON a2.id = (m2->>'artifact_id')
   WHERE a1.id < a2.id
     AND NOT EXISTS (
       SELECT 1 FROM edges e1, edges e2
        WHERE e1.src_id = a1.id AND e2.src_id = a2.id
          AND e1.edge_type = 'BELONGS_TO' AND e2.edge_type = 'BELONGS_TO'
          AND e1.dst_id = e2.dst_id
     )
     AND NOT EXISTS (
       SELECT 1 FROM edges e
        WHERE e.edge_type LIKE 'INFERRED_%'
          AND ((e.src_id = a1.id AND e.dst_id = a2.id) OR (e.src_id = a2.id AND e.dst_id = a1.id))
     )
   LIMIT $1;  -- enrichment.producers.relationship_inference.candidate_selector_limit
  ```
- `RunJob`: invokes scenario `enrichment-relationship-inference-v1` with both artifact summaries; LLM returns `{edge_type ∈ {INFERRED_RELATED, INFERRED_COREFERENCE, INFERRED_TEMPORAL_SEQUENCE}, confidence, justification}`.
- `ApplyOutput`: confidence < floor → `Refusal{Reason: "below_confidence_floor"}`; else INSERT into `edges` with `metadata JSONB` per design §7.
- New scenario YAML: `config/prompt_contracts/enrichment-relationship-inference-v1.yaml`.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| unit | `relationship_inference_test.go` | Candidate-pair selector SQL builds correctly; LIMIT honored | design §7 |
| unit | same | Confidence below floor → refused, no edge persisted | SCN-063-005 |
| unit adversarial | same | LLM returns edge_type outside closed taxonomy → refused (proves output validation) | R-4 |
| integration | `relationship_inference_integration_test.go` | LLM=0.82 → edges row persisted with edge_type=INFERRED_COREFERENCE, metadata contains provenance triple | SCN-063-004 |
| integration adversarial | same | Heuristic-untouched: existing BELONGS_TO edges between candidate artifacts NEVER mutated (row-snapshot before/after) | design §7 |
| e2e-api | `tests/e2e/enrichment_relationship_inference_e2e_test.sh` | Live stack: seed two artifacts sharing entity, run producer tick, assert new INFERRED_COREFERENCE edge in graph | SCN-063-004 |
| Regression E2E | `tests/e2e/enrichment_relationship_inference_e2e_test.sh` (extended) + `tests/e2e/enrichment_relationship_inference_regression_e2e_test.sh` (adversarial suite) | Regression: persistent scenario-specific E2E coverage for SCN-063-004/005 — INFERRED edge persisted with provenance, below-floor refusal, BELONGS_TO heuristic-untouched | SCN-063-004/005 |

### Definition of Done

- [ ] SCN-063-004 — INFERRED edge persisted with provenance: verified by integration test
- [ ] SCN-063-005 — below-floor refused: verified by unit test
- [ ] Adversarial: heuristic-untouched row-snapshot test passes
- [ ] Architecture test NoHeuristicEdgeMutation remains green after producer runs
- [ ] `enrichment-relationship-inference-v1.yaml` validated by `cmd/scenario-lint`
- [ ] SST zero-defaults: confidence floor + candidate selector limit read from SCOPE-01 config
- [ ] Build Quality green; e2e evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_relationship_inference_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 08: `WhyAugmenterProducer` (R-6/7)

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-05

### Use Cases (Gherkin) — SCN-063-006, 007

### Implementation Plan

- New: `internal/knowledge/enrichment/why_augmenter.go`.
- **Trigger source v1: cron-only** (per OQ-PLAN-5 — `intelligence.alert_emitted` publisher missing in spec 021 substrate). `Enqueue` runs SQL `SELECT id, source_artifact_ids FROM alerts a LEFT JOIN enrichment_why w ON (w.parent_kind='alert' AND w.parent_id=a.id AND w.superseded_at IS NULL) WHERE w.id IS NULL AND array_length(a.source_artifact_ids, 1) > 0 LIMIT $1`. Same shape for recommendations/briefs.
- Once PKT-063-B/C land, an event-driven NATS subscription is added in a follow-up (NOT spec 063 v1 scope — would silently add publishers to foreign substrate).
- `RunJob`: invokes scenario `enrichment-why-augmenter-v1` with parent body + `source_artifact_ids`.
- `ApplyOutput`: cited IDs outside parent's `source_artifact_ids` → `Refusal{Reason: "evidence_set_violation"}`; empty cited → `Refusal{Reason: "insufficient_evidence"}`; else INSERT into `enrichment_why`.
- New scenario YAML: `config/prompt_contracts/enrichment-why-augmenter-v1.yaml`.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| unit | `why_augmenter_test.go` | Cited ID outside parent's source_artifact_ids → refused with evidence_set_violation | SCN-063-006 |
| unit | same | Empty source_artifact_ids on parent → refused with insufficient_evidence | SCN-063-007 |
| unit adversarial | same | LLM returns prose with phantom artifact_id never in graph → refused (proves citation-closed-set check) | R-7 |
| integration | `why_augmenter_integration_test.go` | After tick, parent alert row UNCHANGED; new enrichment_why row joined via LEFT JOIN visible | SCN-063-006 |
| integration adversarial | same | Parent alert row untouched (column-snapshot before/after) — proves spec 021 substrate read-only | spec.md §13 |
| e2e-api | `tests/e2e/enrichment_why_augmenter_e2e_test.sh` | Live stack: seed alert with sources, run tick, assert enrichment_why prose attached + citations ⊆ parent sources | SCN-063-006 |
| Regression E2E | `tests/e2e/enrichment_why_augmenter_e2e_test.sh` (extended) + `tests/e2e/enrichment_why_augmenter_regression_e2e_test.sh` (adversarial suite) | Regression: persistent scenario-specific E2E coverage for SCN-063-006/007 — citations ⊆ parent sources, empty-evidence refusal, parent alert row untouched (spec 021 read-only boundary) | SCN-063-006/007 |

### Definition of Done

- [ ] SCN-063-006 — prose attached + citations ⊆ parent: verified by integration test
- [ ] SCN-063-007 — empty-evidence refusal: verified by unit test
- [ ] Adversarial: parent-row-untouched test passes (proves spec 021 read-only boundary)
- [ ] `enrichment-why-augmenter-v1.yaml` validated by `cmd/scenario-lint`
- [ ] SST zero-defaults: per-tick budget + confidence floor read from SCOPE-01 config
- [ ] Cron-only fallback documented; PKT-063-B/C tracked as follow-up
- [ ] Build Quality green; e2e evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_why_augmenter_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 09: `ConsolidationAnalyzerProducer` (R-8) + 90-Day TTL

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-05

### Use Cases (Gherkin) — SCN-063-008

```gherkin
Use case: SCN-063-CONS-01 — Retention cleanup respects last_surfaced_at
  Given a consolidation_candidates row with created_at 100 days ago
  And last_surfaced_at IS NULL
  When the retention cleanup tick runs (retention_days=90)
  Then the row is soft-deleted (superseded_at set)

Use case: SCN-063-CONS-02 — Row pulled by user is retained past TTL
  Given a row with created_at 100 days ago
  And last_surfaced_at within the last 30 days
  When the cleanup tick runs
  Then the row is NOT soft-deleted
```

### Implementation Plan

- New: `internal/knowledge/enrichment/consolidation_analyzer.go`.
- **Trigger source v1: cron-only** (per OQ-PLAN-5 — `topic.edited|merged` publisher missing in spec 025 substrate). `Enqueue` polls a topic-edit audit table (existing `topics.updated_at` watermark) since last successful run.
- `RunJob`: invokes scenario `enrichment-consolidation-analyzer-v1` over union of affected topics' artifacts.
- `ApplyOutput`: confidence < floor (0.75) → refused; else INSERT into `consolidation_candidates`.
- Retention cleanup job in scheduler: soft-delete rows where `created_at < NOW() - cfg.retention_days * INTERVAL '1 day'` AND `last_surfaced_at IS NULL`.
- `last_surfaced_at` updated by reactive scenario in SCOPE-10 when the row is returned to the user.
- New scenario YAML: `config/prompt_contracts/enrichment-consolidation-analyzer-v1.yaml`.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| unit | `consolidation_analyzer_test.go` | Confidence 0.74 below 0.75 floor → refused | R-8 |
| unit | same | Output shape conforms to `{decision, confidence, evidence_artifact_ids}` per R-8 | SCN-063-008 |
| integration | `consolidation_analyzer_integration_test.go` | Topic merge → row persisted; topic edit → row persisted; NO notifications fired (R-13) | SCN-063-008 |
| integration | `retention_cleanup_test.go` | Row at 100d age + NULL last_surfaced_at → soft-deleted | SCN-063-CONS-01 |
| integration adversarial | same | Row at 100d age + last_surfaced_at 30d ago → preserved (proves retention honors UX §14.D inertness) | SCN-063-CONS-02 |
| e2e-api | `tests/e2e/enrichment_consolidation_e2e_test.sh` | Live stack: merge two topics, run tick, assert row visible in `consolidation_candidates` with valid provenance | SCN-063-008 |
| Regression E2E | `tests/e2e/enrichment_consolidation_e2e_test.sh` (extended) + `tests/e2e/enrichment_consolidation_regression_e2e_test.sh` (adversarial suite) | Regression: persistent scenario-specific E2E coverage for SCN-063-008 + SCN-063-CONS-01/02 — row persisted with provenance, retention TTL soft-deletes inert rows, last_surfaced_at preservation, zero-notification assertion | SCN-063-008, SCN-063-CONS-01/02 |

### Definition of Done

- [ ] SCN-063-008 — row persisted with structured suggestion: verified by integration test
- [ ] SCN-063-CONS-01/02 — retention policy verified by `retention_cleanup_test.go`
- [ ] Adversarial: zero-notification assertion during topic-merge integration test (proves R-13)
- [ ] `enrichment-consolidation-analyzer-v1.yaml` validated by `cmd/scenario-lint`
- [ ] SST zero-defaults: confidence floor + retention_days read from SCOPE-01 config
- [ ] Cron-only fallback documented; PKT-063-A tracked as follow-up
- [ ] Build Quality green; e2e evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_consolidation_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 10: Reactive `knowledge_lookup` Scenario + Facade Integration (R-9)

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-04, SCOPE-05

### Use Cases (Gherkin) — SCN-063-009, 010

### Implementation Plan

- New: `internal/agent/tools/enrichment/knowledgelookup/` package:
  - `tool.go` — registers `knowledge_lookup_search` tool via `init()` (spec 037 extension point); MUST NOT touch `internal/agent/{router,executor,registry}.go`.
  - `facade_assembler.go` — implements `contracts.SourceAssembler`; translates `{cited_concept_page_ids, cited_artifact_ids}` → `[]contracts.Source`; empty assembly ⇒ spec 061 provenance gate refuses.
  - `source_assembly.go` — translation helpers.
- New scenario YAML: `config/prompt_contracts/enrichment-knowledge-lookup-v1.yaml` with `requires_provenance: true`, `allowed_tools: [knowledge_lookup_search, retrieval_search]` (composes existing tool per design §8), `intent_examples` using synthesis verbs ("what do I know about", "summarize what I've learned", "tell me about").
- Append ONE row to `config/assistant/scenarios.yaml` registering `knowledge_lookup` with router. NO router code change.
- Min-sources gate from SCOPE-05 wired BEFORE LLM call (cheap SQL `COUNT(*)`).
- §14.B disclosure footer wiring: tool computes `(time_since_last_resynthesis_tick, current_backlog_depth)` from `enrichment_token_ledger` + scheduler state; appends footer to reply iff BOTH thresholds tripped per UX §14.B AND-gate.
- §14.E disclosed-downgrade footer wiring: budget Gate.Admit DOWNGRADE → footer appended.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| unit | `tool_test.go` | `init()` registers `knowledge_lookup_search`; spec 037 registry sees it | R-9 |
| unit | `facade_assembler_test.go` | Empty cited_*_ids → empty SourceAssembly → provenance gate refuses | SCN-063-010 |
| unit | `source_assembly_test.go` | Cited concept_page_ids + artifact_ids translate to `[]contracts.Source` with correct `kind` per ID prefix | SCN-063-009 |
| unit adversarial | `facade_assembler_test.go` | LLM returns answer citing artifact_id NOT in retrieved evidence set → assembler drops it → provenance gate refuses (proves closed-set enforcement) | SCN-063-009 |
| unit | `disclosure_footer_test.go` | Both AND-gate thresholds tripped → footer appended; only ONE tripped → footer absent | UX §14.B |
| integration | `knowledgelookup_integration_test.go` | Live facade + Postgres: query "what do I know about X" with seeded CP-X → reply cites CP-X + member artifact IDs | SCN-063-009 |
| integration | same | Query "what do I know about Y" with zero matches → canonical refusal body returned via spec 061 provenance gate | SCN-063-010 |
| e2e-api | `tests/e2e/enrichment_knowledge_lookup_e2e_test.sh` | Live stack: facade reactive query end-to-end; reply.sources non-empty; cited IDs resolvable in DB | SCN-063-009 |
| e2e-api | same | Empty-evidence query returns canonical refusal; capture-as-fallback offered | SCN-063-010 |
| Regression E2E | `tests/e2e/enrichment_knowledge_lookup_e2e_test.sh` (extended) + `tests/e2e/enrichment_knowledge_lookup_regression_e2e_test.sh` (adversarial suite) | Regression: persistent scenario-specific E2E coverage for SCN-063-009/010 — reactive answer cites CP + artifacts, phantom-citation drop, empty-evidence refusal via spec 061 provenance gate, UX §14.B AND-gate disclosure footer | SCN-063-009/010 |

### Definition of Done

- [ ] SCN-063-009 — reactive answer cites CP + artifact IDs: verified by e2e
- [ ] SCN-063-010 — empty-evidence refusal via spec 061 gate: verified by e2e
- [ ] Adversarial: phantom-citation drop test passes (proves closed evidence-set enforcement)
- [ ] Disclosure footer AND-gate test passes (both/only-one/neither combinations)
- [ ] `enrichment-knowledge-lookup-v1.yaml` validated by `cmd/scenario-lint`; `requires_provenance: true` set
- [ ] Architecture tests NoFacadeMutation, NoAgentRuntimeMutation green (only `internal/agent/tools/enrichment/...` added; no `internal/agent/{router,executor,registry,nats_driver,tracer}.go` mutation)
- [ ] SST zero-defaults: latency budget + model selection + min-sources read from SCOPE-01 config
- [ ] Reactive p95 < 5s budget verified by integration test wall-clock assertion (UX §14.G)
- [ ] Build Quality green; e2e evidence captured (PII-redacted)
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/enrichment_knowledge_lookup_regression_e2e_test.sh` and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 11: Per-Tick Budget Calibration (Load Test)

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-10

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-LOAD-01 — Per-tick budgets drain representative graph without cost cliff
  Given a seeded graph of 12000 artifacts across 1200 concept pages
  When all four background producers run for one hour at SCOPE-01 initial cadence
  Then total LLM token consumption < daily_token_budget * (1/24)
  And per-tick wall-clock < cadence_seconds (no overlap)
  And zero PROCEED->REFUSE budget transitions observed (operator does not hit hard cap on representative workload)
```

### Implementation Plan

- New: `tests/load/enrichment_load_test.go` seeding 12k artifacts + 1.2k concept pages (matches Twitter-archive ingestion shape).
- Run producers for one hour wall-clock against live ML sidecar; capture Prometheus counters.
- If observed token consumption / per-tick latency violates the assertions above, **update `config/smackerel.yaml` SCOPE-01 values in-place** with evidence-cited rationale; otherwise stamp the OQ-PLAN-1 values as final.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| stress/load | `tests/load/enrichment_load_test.go` | Hour-long run completes within budget assertions above | SCN-063-LOAD-01 |
| unit | same (table-driven) | Each producer's per_tick_budget * (3600 / cadence_seconds) ≤ daily_token_budget / 4 (rough fair-share guard) | NFR-2 |
| Regression E2E | `tests/load/enrichment_load_test.go` + `tests/e2e/enrichment_budget_regression_e2e_test.sh` (shared with SCOPE-04) | Regression: persistent scenario-specific E2E coverage for SCN-063-LOAD-01 — hour-long load run against representative graph stays under daily token budget and never triggers PROCEED→REFUSE budget transitions | SCN-063-LOAD-01 |

### Definition of Done

- [ ] SCN-063-LOAD-01 — hour-long load test green under representative graph
- [ ] If SCOPE-01 values revised, evidence block in report.md cites before/after token counts
- [ ] Prometheus dashboard query for `enrichment_token_consumed_total` validated against load run
- [ ] Build Quality green; load evidence captured
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/load/enrichment_load_test.go` (load suite) + `tests/e2e/enrichment_budget_regression_e2e_test.sh` (shared with SCOPE-04) and pass against the live stack
- [ ] Broader E2E regression suite passes (`./smackerel.sh test e2e`) after this scope ships
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 12: Architecture-Test CI Wiring

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-03
**Scope-Kind:** ci-config

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-CI-01 — Architecture tests run on every PR
  Given a PR touches internal/knowledge/enrichment/
  When ./smackerel.sh test unit --go runs in CI
  Then all 7 architecture tests execute
  And the run fails if any architecture invariant is violated
```

### Implementation Plan

- Per OQ-PLAN-3 resolution: **reuse spec 062 pattern** — architecture tests in `internal/knowledge/enrichment/architecture_test.go` (authored in SCOPE-03) are picked up automatically by the existing `./smackerel.sh test unit --go` invocation in CI.
- NO new CI workflow file. NO new pre-commit hook.
- This scope's only deliverable is **evidence** that CI runs the tests on a representative PR and that adversarial sub-tests fail when the forbidden pattern is introduced.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| ci-evidence | `.github/workflows/ci.yml` (read-only verify) | Existing `test unit` job includes spec 063 architecture tests in its output | SCN-063-CI-01 |
| adversarial | branch-fixture | A throwaway commit introducing `internal/notification` import in `internal/knowledge/enrichment/why_augmenter.go` triggers `TestNoNotificationCall` failure in CI run | design §13 |

### Definition of Done

- [ ] SCN-063-CI-01 — CI output evidence shows 7 architecture tests executed
- [ ] Adversarial: throwaway-commit evidence (or local `go test` run injecting forbidden import) proves gate trips
- [ ] No CI workflow file added (architecture-test-as-go-test discipline preserved)
- [ ] Build Quality green; CI evidence captured (PII-redacted)
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Scope 13: Documentation

**Status:** [ ] Not Started | **Foundation:** false | **Depends On:** SCOPE-10
**Scope-Kind:** docs-only

### Use Cases (Gherkin)

```gherkin
Use case: SCN-063-DOC-01 — Operator runbook covers enrichment lifecycle
  Given a new operator reads docs/Operations.md
  When they look up "Knowledge Enrichment" section
  Then they find: enable/disable per producer, budget tuning, refusal-rate troubleshooting, retention policy
```

### Implementation Plan

- Append "Knowledge Enrichment" section to [docs/smackerel.md](../../docs/smackerel.md) covering: capability surface (reactive + 4 background producers), provenance contract, principle alignment (P3/P5/P6/P8/P9), integration with spec 025/021/061/062.
- Append "Knowledge Enrichment" runbook section to [docs/Operations.md](../../docs/Operations.md) covering: SST keys (link to design §10), per-producer enable/disable, daily_token_budget tuning, refusal-rate alarms (Prometheus query examples), `consolidation_candidates` retention cleanup, manual force-drain procedure.
- Update [docs/Architecture.md](../../docs/Architecture.md) module diagram with `internal/knowledge/enrichment/` boundary.

### Test Plan

| Type | File | Assertion | SCN |
|------|------|-----------|-----|
| docs-content | manual review | Each Prometheus query example references real counter names from `internal/metrics/counters.go` | NFR-3 |
| docs-content | manual review | All internal markdown links resolve | SCN-063-DOC-01 |

### Definition of Done

- [ ] SCN-063-DOC-01 — operator runbook covers lifecycle: verified by manual review checklist
- [ ] No fabricated SST keys, Prometheus counter names, or CLI commands in docs (grep each against source)
- [ ] All new internal links resolve
- [ ] Build Quality (lint/format) green; evidence captured
- [ ] Scenario→test mapping in `scenario-manifest.json`

---

## Cross-Cutting Definition of Done (applies to every scope)

- [ ] PII-redaction (`/home/<user>` → `~`) applied to all evidence blocks before commit
- [ ] Repo-CLI commands only in evidence (no ad-hoc `go test ./...` or `pytest` per [terminal-discipline](../../.github/instructions/terminal-discipline.instructions.md))
- [ ] No `git push --no-verify` on commits containing source files
- [ ] state.json executionHistory entry appended; `lastUpdatedAt` bumped; no promotion past `specs_hardened` until DoD complete across all scopes
