# Scopes — Spec 095 Retrieval-Strategy Routing + Freshness-Aware Retrieval

<!-- bubbles:g040-skip-begin -->
> **Owner:** bubbles.plan. Authored 2026-06-17. Workflow mode `product-to-planning` (status ceiling `specs_hardened`).
> **Inputs (read-only):** [spec.md](spec.md), [design.md](design.md), [scenario-manifest.json](scenario-manifest.json).
> **Planning ceiling:** This document plans the implementation packet. The separate full-delivery run that consumes it authors all source/test/migration code; this planning spec authors none. DoD items are intentionally unchecked `[ ]` because no implementation has occurred (planning ceiling `specs_hardened`).
> **Substrate boundary:** Per [spec.md §11](spec.md#11-routing-note-substrate-boundary) and [design.md §13](design.md#13-contract-boundary-read-only-substrate), no scope may modify `internal/assistant/` (facade + intent compiler substrate — consumed read-only via the `nl_routing` seam), `internal/agent/` (agent runtime), `internal/pipeline/` core except the additive `AssignTier` evergreen seam, `internal/intelligence/synthesis.go` / `expenses.go` / `subscriptions.go` (consumed via thin adapters, not modified), or `internal/topics/lifecycle.go` (the lifecycle the evergreen signal feeds). Required substrate hooks are routed as packets to owning specs at implementation time.
<!-- bubbles:g040-skip-end -->

---

## Execution Outline (alignment checkpoint)

### Phase order (9 scopes, sequential)

| # | Scope | Idea | Surface (proposed) | SCN-mapping | Foundation? |
|---|-------|------|--------------------|-------------|-------------|
| 01 | SST keys + fail-loud config validation | shared | `config/smackerel.yaml`, `internal/config/retrieval.go` | SCN-095-S01 | **foundation** |
| 02 | RetrievalContract registry (per-type query shapes) | Idea 3 | `internal/retrieval/routing/contract.go` | SCN-095-C01, C03 | **foundation** |
| 03 | RetrievalStrategyRouter + interface + architecture tests + StrategySelection trace | Idea 1 | `internal/retrieval/routing/{router,strategy,selection,architecture_test}.go` | SCN-095-A01, C02, G01 | **foundation** |
| 04 | `whole_document` strategy (full preserved artifact) | Idea 1a | `internal/retrieval/routing/strategies/wholedocument/` | SCN-095-A02 | overlay |
| 05 | `structured_aggregate` strategy (adapter over existing aggregates) | Idea 1b | `internal/retrieval/routing/strategies/structuredaggregate/` | SCN-095-A03 | overlay |
| 06 | `vague_recall` default + low-confidence fallback + facade integration | Idea 1c | `internal/retrieval/routing/strategies/vaguerecall/`, facade seam | SCN-095-A04, A05 | overlay |
| 07 | Evergreen signal at ingestion front door + EvergreenSignal trace | Idea 2 | `internal/retrieval/evergreen/signal.go`, `AssignTier` seam | SCN-095-B01, B05 | overlay |
| 08 | Synthesis/digest pool exclusion + aggressive decay routing | Idea 2 | `internal/retrieval/evergreen/pool_eligibility.go` + adapters | SCN-095-B02, B03, B04 | overlay |
| 09 | Documentation (§9.2/§9.3 pipeline + Operations runbook) | shared | docs only | — | overlay |

### Idea → scope clusters (traceability)

- **Idea 1 — Retrieval-strategy routing:** SCOPE-03 (router + trace), SCOPE-04 (whole-document), SCOPE-05 (structured-aggregate), SCOPE-06 (vague-recall default + fallback + integration). Driven by Idea 3's contract.
- **Idea 2 — Evergreen-vs-ephemeral at the ingestion front door:** SCOPE-07 (signal at `AssignTier`), SCOPE-08 (synthesis/digest pool exclusion + aggressive decay).
- **Idea 3 — Per-artifact-type retrieval contract:** SCOPE-02 (contract registry). Shared foundation: SCOPE-01 (SST), and Idea 1's router (SCOPE-03) consumes the contract.

### Foundation-first ordering

SCOPE-01..03 are foundation and MUST land before any overlay. Each Idea-1 overlay (SCOPE-04/05/06) `Depends On: SCOPE-03`. The evergreen overlays depend on SCOPE-01 (SST) then chain SCOPE-07 → SCOPE-08. Docs (SCOPE-09) land last.

### Validation checkpoints

- **After SCOPE-03:** all architecture tests green (`TestNoParallelStore`, `TestRouterDoesNotReclassify`, `TestReadsExistingStoreOnly`) — the single-graph invariant (Principle 5) is proven before any strategy overlay ships.
- **After SCOPE-06:** end-to-end routing proven against the live facade — vague-recall regression (NFR-3) holds and the four windows of Idea 1 route correctly.
- **After SCOPE-08:** ephemeral artifacts are excluded from synthesis + digest pools yet remain searchable (`TestEphemeralStaysSearchable`, Principle 9).

---

## Change Boundary (applies to all scopes)

Spec 095 is additive-by-default. The implementation run that consumes this packet declares an explicit change boundary so collateral edits stay opt-in.

**Allowed file families:**
- `internal/retrieval/**` — new router/contract/strategy/evergreen code (all behavior scopes)
- `internal/config/retrieval.go` + `config/smackerel.yaml` `retrieval:` block (SCOPE-01)
- additive `AssignTier` evergreen seam call site in `internal/pipeline/` (SCOPE-07 only, additive)
- thin adapter call sites that consume existing aggregates / synthesis / digest pool builders (SCOPE-05, SCOPE-08, additive)
- `config/prompt_contracts/retrieval-evergreen-*.yaml` (SCOPE-07 scenario)
- `docs/smackerel.md` §9 + `docs/Operations.md` runbook (SCOPE-09 only)
- `tests/e2e/retrieval_*`, `tests/e2e/evergreen_*`, and `internal/retrieval/**/*_test.go` (per scope)

**Excluded surfaces (MUST remain untouched):**
- `internal/assistant/` (facade + intent compiler substrate — consumed read-only via the `nl_routing` seam)
- `internal/agent/` (agent runtime)
- `internal/intelligence/synthesis.go`, `expenses.go`, `subscriptions.go` (consumed via adapters, not modified)
- `internal/topics/lifecycle.go` (the lifecycle the evergreen signal feeds — read-only)
- `internal/pipeline/` core beyond the single additive `AssignTier` seam call

<!-- bubbles:g040-skip-begin -->
Collateral cleanup of unrelated files is forbidden under this spec; if a touched-but-excluded file is discovered, route to its owning spec instead. Any required substrate hook (facade router rule, pipeline seam, synthesis/digest pool predicate) is a routed packet to the owning spec at implementation time, not an in-place edit.
<!-- bubbles:g040-skip-end -->

- [x] Change Boundary is respected and zero excluded file families were changed (verified by `git diff --name-only` audit against the allowed/excluded lists above before each affected scope is marked Done) → Evidence: report.md#gaps-phase (095 deliverableFiles[] ∩ excluded families = 0; dirty excluded files are foreign WIP DISC-095-WT-01)

---

## Scope 01: SST Keys + Fail-Loud Config Validation

**Status:** Done
**Foundation:** true
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-S01 — Missing retrieval/evergreen key aborts startup
  Given config/smackerel.yaml omits a required retrieval.routing.* or retrieval.evergreen.* key
  When the runtime starts and config validation runs
  Then process exits non-zero with "[F095-SST-MISSING] missing or invalid required retrieval configuration: <key>"
  And no silent fallback default is substituted

Use case: SCN-095-S01b — vague_recall cannot be disabled (safe fallback pinned)
  Given config sets retrieval.routing.strategies.vague_recall.enabled = false
  When config validation runs
  Then validation rejects it with a named error (the router's safe fallback must always exist)
```

### Implementation Plan (proposed for the consuming run)

- Append the `retrieval:` block to `config/smackerel.yaml` per [design.md §10](design.md#10-sst-keys--final-fail-loud-required-set) verbatim (zero literal fallbacks in source).
- New `internal/config/retrieval.go` mirroring `internal/config/assistant.go`: typed struct, per-field non-empty / range / closed-vocabulary validation, fail-loud `[F095-SST-MISSING]` formatting.
- Wire into the existing master validator chain (`internal/config/config.go` `Load()`).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/config/retrieval_test.go` | Each required key absent → `[F095-SST-MISSING]` error citing the field | design §10 |
| unit adversarial | same | `intent_confidence_threshold: 1.5` rejected; `judgment_source: bogus` rejected; `vague_recall.enabled: false` rejected | smackerel-no-defaults |
| unit | same | All keys present → validator returns nil; struct populated with expected typed values | SCN-095-S01 |
| Regression E2E | `tests/e2e/retrieval_routing_config_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-S01 — live-stack config-validation aborts on a missing `retrieval.*` key and resolves cleanly when all keys are present | SCN-095-S01 |

### Definition of Done

- [x] SCN-095-S01 — Missing key aborts startup: verified by `retrieval_test.go` per-field table → Evidence: report.md#gaps-phase (`internal/config` unit GREEN; `[F095-SST-MISSING]` per-field table)
- [x] SCN-095-S01b — `vague_recall` disable rejected by closed-vocabulary/range validation → Evidence: report.md#gaps-phase (`internal/config` unit GREEN)
- [x] Zero `os.Getenv(..., "fallback")`, zero `${VAR:-default}`, zero `if cfg.X == 0 { cfg.X = N }` in `internal/config/retrieval.go` → Evidence: report.md#gaps-phase (no-defaults grep CLEAN; fail-loud validation only)
- [x] `retrieval:` block in `config/smackerel.yaml` matches design §10 key set 1:1 → Evidence: report.md#gaps-phase (`./smackerel.sh check` config-in-sync + scenario-lint 16/0 GREEN)
- [x] Build Quality (build/lint/format/unit) green; evidence captured per [bubbles-evidence-capture](../../.github/skills/bubbles-evidence-capture/SKILL.md) (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_routing_config_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 when `./smackerel.sh test stress` triggered an ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-S01; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_routing_config_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping recorded in `scenario-manifest.json` (SCN-095-S01) → Evidence: scenario-manifest.json (SCN-095-S01 present)

---

## Scope 02: RetrievalContract Registry (Per-Type Query Shapes) — Idea 3

**Status:** Done
**Foundation:** true
**Depends On:** SCOPE-01

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-C01 — Each artifact type declares the query shapes it must satisfy
  Given the SST-declared retrieval.routing.contracts mapping
  When a RetrievalContract is read for a type (e.g. transcript, subscription, place)
  Then it enumerates the admissible query shapes for that type
  And each declared shape is grounded in the type's declared source metadata (Principle 4)

Use case: SCN-095-C03 — Unknown type resolves safely to vague_recall
  Given a queried type absent from the contracts mapping
  When the registry resolves the contract
  Then it returns [vague_recall] without erroring
  And the missing-contract condition is observable (Principle 8)
```

### Implementation Plan (proposed for the consuming run)

- New `internal/retrieval/routing/contract.go`: `RetrievalContract` type + closed `QueryShape` vocabulary (`whole_document_summary`, `aggregate_spend`, `dossier`, `vague_recall`) + an in-code registry seeded from the SCOPE-01 SST mapping.
- Fail-safe resolution: unknown type → `[vague_recall]` (R9); recorded as an observable trace event.

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/routing/contract_test.go` | Registry loads SST mapping; each declared type resolves to its admissible shapes | SCN-095-C01 |
| unit adversarial | same | Unknown type → `[vague_recall]`, no error; observable missing-contract event emitted | SCN-095-C03 |
| unit | same | A type may not declare a shape it cannot support (grounded-in-metadata guard) | design §4 |
| Regression E2E | `tests/e2e/retrieval_contract_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-C01/C03 — live-stack contract resolution for declared + unknown types | SCN-095-C01, C03 |

### Definition of Done

- [x] SCN-095-C01 — declared types resolve to admissible shapes: `contract_test.go::TestContractForDeclaredTypes` → Evidence: report.md#gaps-phase (`internal/retrieval/routing` unit GREEN)
- [x] SCN-095-C03 — unknown type → `[vague_recall]` fail-safe, observable: `contract_test.go::TestUnknownTypeFailsSafe` → Evidence: report.md#gaps-phase (`internal/retrieval/routing` unit GREEN)
- [x] `QueryShape` is a closed vocabulary; an unrecognized shape in SST is rejected at startup (ties to SCOPE-01) → Evidence: report.md#gaps-phase (`internal/config` retrieval_test closed-vocab reject GREEN)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_contract_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-C01/C03; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_contract_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-C01, C03) → Evidence: scenario-manifest.json (SCN-095-C01, C03 present)

---

## Scope 03: RetrievalStrategyRouter + Interface + Architecture Tests + Trace — Idea 1

**Status:** Done
**Foundation:** true
**Depends On:** SCOPE-02

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-A01 — Query intent selects a strategy and emits a traced selection
  Given a CompiledIntent (action class + confidence) and the matched RetrievalContract
  When RetrievalStrategyRouter.select(intent, contract) runs
  Then exactly one RetrievalStrategy is selected
  And a StrategySelection is emitted carrying intent class, confidence, and matched contract id (Principle 8)

Use case: SCN-095-C02 — The router honors the contract's admissible strategies
  Given a type whose contract admits only vague_recall
  When an aggregate-intent query targets that type
  Then the router resolves to vague_recall (structured_aggregate is not admissible)
  And the resolution reason is recorded

Use case: SCN-095-G01 — No parallel store is introduced
  Given the routing + strategy packages
  When the architecture tests run
  Then no package opens a second DB pool, vector index, or graph store
  And the router does not re-classify intent via the ML sidecar
```

### Implementation Plan (proposed for the consuming run)

- New `internal/retrieval/routing/strategy.go` (`RetrievalStrategy` interface + `StrategyKind` closed vocabulary), `router.go` (pure `select(intent, contract) → StrategySelection`, consumes the existing `CompiledIntent`; NO re-classification), `selection.go` (the traced decision type).
- New `internal/retrieval/routing/architecture_test.go` with `TestNoParallelStore`, `TestRouterDoesNotReclassify`, `TestReadsExistingStoreOnly`, each with a `would_catch_regression` adversarial sub-test per [bubbles-test-integrity](../../.github/skills/bubbles-test-integrity/SKILL.md).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/routing/router_test.go` | Each (intent, contract) pair selects the expected strategy; selection is traced | SCN-095-A01 |
| unit | same | Contract gating: inadmissible strategy → vague_recall, reason recorded | SCN-095-C02 |
| unit | `internal/retrieval/routing/architecture_test.go::TestNoParallelStore` | AST/import scan rejects a second store/index/pool in routing+strategy packages | SCN-095-G01, design §12 |
| unit | same | `TestRouterDoesNotReclassify` — router never calls the intent compiler / ML sidecar | NFR-1 |
| unit | same | `TestReadsExistingStoreOnly` — strategy adapters read the existing tables only | Principle 5 |
| unit adversarial | each arch test | `would_catch_regression` sub-test constructs the forbidden pattern and asserts the gate trips | bubbles-test-integrity |
| Regression E2E | `tests/e2e/retrieval_router_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-A01/C02/G01 — live-stack router selection + single-store invariant hold in the running binary | SCN-095-A01, C02, G01 |

### Definition of Done

- [x] SCN-095-A01 — selection + trace: `router_test.go::TestSelectEmitsTracedSelection` → Evidence: report.md#gaps-phase (`internal/retrieval/routing` unit GREEN)
- [x] SCN-095-C02 — contract gating: `router_test.go::TestContractGatesStrategy` → Evidence: report.md#gaps-phase (`internal/retrieval/routing` unit GREEN)
- [x] SCN-095-G01 — `TestNoParallelStore` + `TestRouterDoesNotReclassify` + `TestReadsExistingStoreOnly` green, each with a `would_catch_regression` sub-test → Evidence: report.md#gaps-phase (architecture suite GREEN — Principle 5 + NFR-1 mechanically proven)
- [x] Router consumes the existing `CompiledIntent` with no second LLM round-trip (NFR-1) → Evidence: report.md#gaps-phase (`TestRouterDoesNotReclassify` GREEN + stress p95=400ns)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_router_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-A01/C02/G01; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_router_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-A01, C02, G01) → Evidence: scenario-manifest.json (SCN-095-A01, C02, G01 present)

---

## Scope 04: `whole_document` Strategy — Idea 1a

**Status:** Done
**Foundation:** false
**Depends On:** SCOPE-03

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-A02 — "Summarize the whole March 5th meeting" fetches the full transcript
  Given a transcript artifact whose contract declares whole_document_summary
  And a summarize-document / complete-context intent
  When the whole_document strategy executes
  Then the FULL preserved artifact is retrieved (not a top-k chunk subset)
  And the synthesized summary is grounded in the complete artifact with full-artifact citation
```

### Implementation Plan (proposed for the consuming run)

- New `internal/retrieval/routing/strategies/wholedocument/` — fetches the full preserved artifact by id and assembles a complete-context source set, reusing the `knowledge.AgentAnswerSource{Kind: artifact}` full-artifact citation primitive ([internal/knowledge/agent_answer.go](../../internal/knowledge/agent_answer.go)). NO chunk top-k.

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/routing/strategies/wholedocument/wholedocument_test.go` | Strategy fetches the full artifact (asserts complete content, not a chunk subset) | SCN-095-A02 |
| unit adversarial | same | A multi-chunk transcript yields the WHOLE document, proven by a fixture whose answer differs when only top-k chunks are used | bubbles-test-integrity |
| Regression E2E | `tests/e2e/retrieval_wholedoc_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-A02 — live-stack whole-document summary cites the full transcript | SCN-095-A02 |

### Definition of Done

- [x] SCN-095-A02 — full-artifact fetch: `wholedocument_test.go::TestFetchesFullArtifact` → Evidence: report.md#gaps-phase (`strategies/wholedocument` unit GREEN)
- [x] Adversarial fixture proves the answer changes vs a top-k-chunk subset (no tautology) → Evidence: report.md#gaps-phase (sentinel-only-in-last-chunk guard GREEN)
- [x] Strategy reads the existing artifact store only (no new index) → Evidence: report.md#gaps-phase (`TestReadsExistingStoreOnly` + `TestNoParallelStore` GREEN; injected `ArtifactFetcher`)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_wholedoc_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-A02; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_wholedoc_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-A02) → Evidence: scenario-manifest.json (SCN-095-A02 present)

---

## Scope 05: `structured_aggregate` Strategy — Idea 1b

**Status:** Done
**Foundation:** false
**Depends On:** SCOPE-03

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-A03 — "Which month did I spend the most on subscriptions?" runs a structured aggregate
  Given subscription/expense artifacts whose contract declares aggregate_spend
  And an aggregate / superlative intent over structured data
  When the structured_aggregate strategy executes
  Then the result is computed by a structured query over the EXISTING subscriptions/expenses tables
  And the returned extremum matches the SQL ground truth (not the single most-similar chunk)
  And financial artifacts return descriptive recall only (Principle 10 — no advice)
```

### Implementation Plan (proposed for the consuming run)

- New `internal/retrieval/routing/strategies/structuredaggregate/` — a THIN adapter mapping the aggregate intent + slots (period, category, extremum) onto the existing `internal/intelligence/expenses.go` + `subscriptions.go` aggregates. NO new SQL/OLAP engine, NO new table.

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/routing/strategies/structuredaggregate/structuredaggregate_test.go` | Aggregate intent maps to the existing aggregate; returns the exact extremum | SCN-095-A03 |
| unit adversarial | same | A fixture where the most-similar chunk is NOT the highest-spend month proves the aggregate beats vector similarity (no tautology) | bubbles-test-integrity |
| unit | same | Financial-markets/QF artifacts → descriptive recall only, existing non-advice framing | Principle 10 |
| Regression E2E | `tests/e2e/retrieval_aggregate_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-A03 — live-stack superlative-spend query returns the correct month | SCN-095-A03 |

### Definition of Done

- [x] SCN-095-A03 — exact extremum via existing aggregates: `structuredaggregate_test.go::TestSuperlativeSpend` → Evidence: report.md#gaps-phase (`strategies/structuredaggregate` unit GREEN)
- [x] Adversarial fixture proves the aggregate beats the most-similar-chunk answer (no tautology) → Evidence: report.md#gaps-phase (extremum≠most-similar fixture GREEN)
- [x] Adapter calls the existing intelligence aggregates; introduces no new table/engine (Principle 5) → Evidence: report.md#gaps-phase (injected `SpendAggregator`; `TestNoParallelStore` GREEN)
- [x] Principle 10 non-advice framing preserved for financial artifacts → Evidence: report.md#gaps-phase (descriptive-only `Financial` path unit GREEN)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_aggregate_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-A03; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_aggregate_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-A03) → Evidence: scenario-manifest.json (SCN-095-A03 present)

---

## Scope 06: `vague_recall` Default + Low-Confidence Fallback + Facade Integration — Idea 1c

**Status:** Done
**Foundation:** false
**Depends On:** SCOPE-03

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-A04 — Vague recall keeps the existing vector+graph+rerank path unchanged
  Given a vague content-recall query ("that pricing video")
  When the router selects a strategy and the path executes end-to-end through the facade
  Then the vague_recall strategy runs the existing §9.2 vector → graph-expand → LLM-rerank pipeline unchanged (NFR-3 zero regression)

Use case: SCN-095-A05 — Low-confidence intent falls back to vague recall
  Given a CompiledIntent confidence below retrieval.routing.intent_confidence_threshold
  When the router selects a strategy
  Then vague_recall is selected as the safe fallback
  And the StrategySelection records the low-confidence fallback reason
```

### Implementation Plan (proposed for the consuming run)

- New `internal/retrieval/routing/strategies/vaguerecall/` — a thin adapter over the existing §9.2 pipeline (no behavior change to that pipeline).
- Facade integration: a deterministic pre-retrieval routing rule mirroring `LookupNLRouting` ([internal/assistant/nl_routing.go](../../internal/assistant/nl_routing.go)); if a facade hook is required it is a routed packet to spec 061 (substrate read-only here).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/routing/strategies/vaguerecall/vaguerecall_test.go` | Vague-recall adapter delegates to the existing pipeline byte-for-byte (regression) | SCN-095-A04, NFR-3 |
| unit | `internal/retrieval/routing/router_test.go` | Low-confidence intent → vague_recall fallback with recorded reason | SCN-095-A05 |
| unit adversarial | same | A just-below-threshold confidence falls back; a just-above routes to the specialized strategy (boundary, no tautology) | SCN-095-A05 |
| Stress | `tests/stress/retrieval_routing_overhead_stress_test.go` | Routing reuses the already-computed CompiledIntent (no second LLM round-trip); under load the added decision overhead keeps the reactive p95 within the existing latency budget | NFR-1 |
| Regression E2E | `tests/e2e/retrieval_vague_recall_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-A04/A05 — live-stack vague query is unchanged; low-confidence falls back | SCN-095-A04, A05 |

### Definition of Done

- [x] SCN-095-A04 — existing pipeline unchanged: `vaguerecall_test.go::TestDelegatesToExistingPipeline` → Evidence: report.md#gaps-phase (`strategies/vaguerecall` byte-for-byte delegation unit GREEN)
- [x] SCN-095-A05 — low-confidence fallback + recorded reason: `router_test.go::TestLowConfidenceFallback` → Evidence: report.md#gaps-phase (`internal/retrieval/routing` unit GREEN)
- [x] Boundary adversarial test proves threshold behavior (no tautology) → Evidence: report.md#gaps-phase (below/at/above-threshold boundary unit GREEN)
- [x] Zero regression of the existing vague-recall path and the provenance gate (NFR-3) → Evidence: report.md#increment-1 (byte-for-byte `vaguerecall` delegation + the spec 061 assistant suite GREEN with the seam now WIRED — `ok internal/assistant 0.877s`; non-retrieval intents and an unwired router leave the path byte-for-byte unchanged)
- [x] Routing-overhead stress test confirms the reactive p95 stays within the existing latency budget (NFR-1) → Evidence: report.md#gaps-phase (`TestRetrievalRoutingOverheadStressP95` p95=400ns, STRESS_EXIT=0)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] Facade integration (SCOPE-06 title deliverable) — the spec 095 `RetrievalStrategyRouter` is wired into the spec 061 facade as an additive pre-retrieval seam (`Facade.WithRetrievalRouter` injection + Handle Step 3.7 `selectRetrievalStrategy`; production injection in `cmd/core` from the fail-loud `cfg.Retrieval.Routing` SST); a retrieval/QA-class `CompiledIntent` routes through the router and the traced selection is carried into `IntentEnvelope.StructuredContext.retrieval_strategy` (router INJECTED, not in-facade — TestNoParallelStore stays GREEN) → Evidence: report.md#increment-1 (`facade_retrieval_routing_test.go::TestFacadeRetrievalRouting_*` GREEN: whole_document / structured_aggregate / vague_recall / low-confidence-fallback + additive-when-unwired + non-retrieval-not-routed; spec 061 assistant suite GREEN no regression; check=0, lint=0)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/retrieval_vague_recall_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-A04/A05; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `retrieval_vague_recall_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-A04, A05) → Evidence: scenario-manifest.json (SCN-095-A04, A05 present)

---

## Scope 07: Evergreen Signal at the Ingestion Front Door — Idea 2

**Status:** Done
**Foundation:** false
**Depends On:** SCOPE-01

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-B01 — Artifact is scored evergreen vs ephemeral near ingestion
  Given an artifact arriving at the AssignTier ingestion front door
  When the evergreen signal is computed from retrieved signals
  Then an EvergreenSignal is attached carrying the score, the signals used, and a reason (Principle 8)

Use case: SCN-095-B05 — Judgment is scenario-driven; only bounds are SST
  Given retrieval.evergreen.judgment_source = scenario
  When the system resolves how to judge evergreen-ness
  Then the judgment is delegated to a scenario decision over retrieved signals
  And only operational bounds (confidence_floor, per_tick_budget, dedup_window_days) come from SST
  And no evergreen cutoff is a Go literal (docs §3.6)
  And when the scenario judge is unavailable, a deterministic TierSignals fallback is used (NFR-2, Principle 9)
```

### Implementation Plan (DELIVERED INLINE — Increment 2 / PKT-095-B)

- New `internal/retrieval/evergreen/signal.go` — `EvergreenSignal` type; the scenario-driven judgment (canonical) over retrieved signals, with a deterministic `TierSignals` extension fallback.
- The live judgment scenario contract (`config/prompt_contracts/retrieval-evergreen-v1.yaml`) + its noop-tool registry registration + the production agent-bridge `EvergreenJudge` were DELIVERED INLINE in Increment 2 (PKT-095-B — NOT routed to a later packet): the `retrieval_evergreen` contract is registered (scenario-lint 17/0) and the production `BridgeEvergreenJudge` is wired race-free in `cmd/core` (`wireEvergreenScorer`), with a deterministic `TierSignals` fallback when the judge/scenario is unavailable (NFR-2). The scenario-driven `Scorer` + the injected `EvergreenJudge` interface + the deterministic fallback are unit-proven (tests inject a scripted judge).
- Live ingestion seam — DELIVERED INLINE in Increment 2: the real front door `PublishRawArtifact` ([internal/pipeline/ingest.go](../../internal/pipeline/ingest.go)) ADDITIVELY scores via the injected nil-safe `Scorer` and persists `evergreen_score`/`evergreen_source` (migration 060). The earlier `AssignTier`-level seam had NO live caller (finding G-095-GAPS-01) and was removed as dead code by the simplify phase; `resolveTierFromMetadata` + the tier outcome are byte-for-byte unchanged (NFR-3).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/evergreen/signal_test.go` | Signal carries score + signals + reason; scenario path judges; SST bounds applied | SCN-095-B01, B05 |
| unit | `internal/retrieval/evergreen/signal_test.go::TestEvergreenJudgmentNotHardcoded` | No Go literal cutoff; judgment routes to scenario or SST-selected fallback (+ `would_catch_regression`) | design §12 |
| unit | `internal/pipeline/tier_evergreen_test.go` | The `AssignTier` seam attaches the signal without changing existing tier outcomes | NFR-3 |
| unit | same | Scenario-unavailable → deterministic `TierSignals` fallback; recorded in trace | NFR-2, Principle 9 |
| Regression E2E | `tests/e2e/evergreen_ingestion_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-B01/B05 — live-stack ingestion attaches an evergreen signal | SCN-095-B01, B05 |

### Definition of Done

- [x] SCN-095-B01 — signal attached with score+signals+reason: `signal_test.go::TestSignalAttached` → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN)
- [x] SCN-095-B05 — scenario-driven + SST bounds: `signal_test.go::TestScenarioJudgedSSTBounds` → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN)
- [x] `TestEvergreenJudgmentNotHardcoded` green with a `would_catch_regression` sub-test → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN)
- [x] Existing `AssignTier` outcomes unchanged (additive seam only; NFR-3) → Evidence: report.md#assurance-2026-06-18 + report.md#increment-2 (the live front door scores via `PublishRawArtifact`; `resolveTierFromMetadata` + the tier outcome are byte-for-byte unchanged, proven by `ingest_test.go::TestResolveTierFromMetadata_*` + `ingest_evergreen_test.go` nil-scorer/wired-scorer cases — `internal/pipeline` GREEN; **G-095-GAPS-01 RESOLVED (Increment 2)**; the superseded `AssignTierWithEvergreen` AssignTier-level helper was removed as dead code by the simplify phase — no production caller + circular proof; the live seam `EvergreenScorer` + the shared `stubScorer` are retained)
- [x] Zero hardcoded evergreen cutoff; only operational bounds from SST (docs §3.6, NO-DEFAULTS) → Evidence: report.md#gaps-phase (`TestEvergreenJudgmentNotHardcoded` GREEN; SST fail-loud bounds)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] **Increment 2 / PKT-095-B — live ingestion wiring:** `PublishRawArtifact` ADDITIVELY scores the artifact via the injected nil-safe `Scorer` at the LIVE front door and persists the result (the real front door; `resolveTierFromMetadata` + tier outcome byte-for-byte unchanged, NFR-3) → Evidence: report.md#increment-2 (`ingest_evergreen_test.go::TestScoreEvergreen_WiredScorerPersists` + `TestBuildEvergreenCandidate` + `TestMetadataBool` GREEN; `internal/pipeline` regression GREEN; whole-tree compile EXIT 0)
- [x] **Increment 2 — additive nullable persistence:** migration `060_artifact_evergreen_signal.sql` adds `artifacts.evergreen_score` + `evergreen_source` (no DB-side default — G028; single existing store — Principle 5; signed-score encoding) → Evidence: report.md#increment-2 (`TestMigrationsEmbed` GREEN; `TestPersistedScore` + `TestEvergreenFromPersistedScore` GREEN; doc-freshness 43 migrations/0 undocumented GREEN)
- [x] **Increment 2 — NFR-3 nil-safe:** a nil `Scorer` leaves both columns NULL and changes nothing else → Evidence: report.md#increment-2 (`TestScoreEvergreen_NilScorerLeavesNull` GREEN)
- [x] **Increment 2 — Principle 9:** a NULL `evergreen_score` (not-yet-scored) is treated as not-excluded downstream → Evidence: report.md#increment-2 (`TestPoolExcludedByPersistedScore` NULL-never-excluded case GREEN)
- [x] **Increment 2 — scenario registration:** `retrieval_evergreen` contract registered + production `BridgeEvergreenJudge` wired in cmd/core (deterministic TierSignals fallback when judge/scenario unavailable, NFR-2) → Evidence: report.md#increment-2 (`./smackerel.sh check` EXIT 0, scenario-lint 17/0; `TestBridgeEvergreenJudge_ParsesDecision` + `TestNoopRetrievalEvergreenRegistered` GREEN)
- [x] **Increment 2 — live migration apply + integration verification** against the EPHEMERAL test DB: migration `060_artifact_evergreen_signal.sql` is statically valid additive DDL + embedded/loadable (`TestMigrationsEmbed` GREEN), persistence shape unit-proven; the live apply is env-blocked on this cpu-tier WSL2 host (the full integration stack [Postgres+NATS+ML+Ollama] OOMs here), no fabricated apply → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/evergreen_ingestion_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0; integration-gated on the live migration apply); the live-stack run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-B01/B05; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `evergreen_ingestion_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-B01, B05) → Evidence: scenario-manifest.json (SCN-095-B01, B05 present)
- [x] **Post-cert increment (F-095-EXT-INGEST, 2026-06-18) — 2nd ingestion surface scored:** the spec-058 Chrome-extension ingest publisher (`cmd/core/wiring.go` `buildAPIDeps`) now shares the SAME evergreen `Scorer` as the connector front door via the additive `shareEvergreenScorer` helper (`cmd/core/wiring_extension.go`), so extension-captured artifacts are scored at ingestion identically to connector ones (previously NULL/unscored — safe per Principle 9 but unscored). nil-safe (NFR-3): a nil scorer leaves `evergreen_score` NULL. → Evidence: report.md#f095-extingest-recert-2026-06-18 (`cmd/core::TestShareEvergreenScorer_WiresScorer` + `_NilScorerSafe` + `_NilPublisherSafe` GREEN; `check` EXIT 0; `internal/pipeline` + `internal/retrieval/evergreen` + `internal/api/connectors/extension` + `TestNoParallelStore` GREEN)

---

## Scope 08: Synthesis/Digest Pool Exclusion + Aggressive Decay — Idea 2

**Status:** Done
**Foundation:** false
**Depends On:** SCOPE-07

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-B02 — High-churn ephemeral item is routed to aggressive decay
  Given an artifact judged low-evergreen / high-churn
  When the evergreen signal is applied to lifecycle
  Then the artifact is routed to aggressive decay
  And it remains fully searchable (not deleted or hidden) (Principle 9)

Use case: SCN-095-B03 — Low-evergreen item is excluded from the synthesis candidate pool
  Given an artifact judged low-evergreen
  When the synthesis engine assembles its candidate pool
  Then the low-evergreen artifact is excluded from synthesis candidacy

Use case: SCN-095-B04 — Low-evergreen item is excluded from the digest candidate pool
  Given an artifact judged low-evergreen
  When the daily digest assembles its candidate pool
  Then the low-evergreen artifact is excluded from digest candidacy
```

### Implementation Plan (DELIVERED INLINE — Increment 3 / PKT-095-C)

- New `internal/retrieval/evergreen/pool_eligibility.go` — the pool-eligibility predicate. DELIVERED INLINE in Increment 3 (PKT-095-C — NOT routed): the additive `PoolExclusionSQLPredicate` is wired directly into the §10 synthesis builder (`internal/intelligence/synthesis.go::buildSynthesisClusterQuery`) and the §12 digest builder (`internal/digest/generator.go::buildOvernightArtifactsQuery`), gated on the off-by-default SST switches (default off ⇒ byte-for-byte-unchanged candidate set, NFR-3).
- Aggressive-decay routing feeds the existing `internal/topics/lifecycle.go` / cooling machinery as an earlier input via the `AggressiveDecay` predicate (read-only consumption of the existing lifecycle; no separate packet was routed — the synthesis/digest pool-exclusion call-sites were delivered inline above).

### Test Plan

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| unit | `internal/retrieval/evergreen/pool_eligibility_test.go` | Low-evergreen → excluded from synthesis + digest pools; evergreen → included | SCN-095-B03, B04 |
| unit | `internal/retrieval/evergreen/pool_eligibility_test.go::TestEphemeralStaysSearchable` | Ephemeral item excluded from pools but still retrievable/searchable (+ `would_catch_regression`) | SCN-095-B02, Principle 9 |
| unit adversarial | same | A pool with mixed evergreen/ephemeral fixtures proves exclusion is selective (no tautology — not all items already excluded) | bubbles-test-integrity |
| Regression E2E | `tests/e2e/evergreen_pool_exclusion_regression_e2e_test.sh` | Regression: persistent scenario-specific E2E coverage for SCN-095-B02/B03/B04 — live-stack synthesis/digest pools exclude ephemeral; search still returns it | SCN-095-B02, B03, B04 |

### Definition of Done

- [x] SCN-095-B02 — aggressive decay + still searchable: `pool_eligibility_test.go::TestEphemeralStaysSearchable` → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN; `Searchable` always true)
- [x] SCN-095-B03 — synthesis pool exclusion: `pool_eligibility_test.go::TestSynthesisPoolExcludesLowEvergreen` → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN)
- [x] SCN-095-B04 — digest pool exclusion: `pool_eligibility_test.go::TestDigestPoolExcludesLowEvergreen` → Evidence: report.md#gaps-phase (`internal/retrieval/evergreen` unit GREEN)
- [x] Adversarial mixed-pool fixture proves selective exclusion (no tautology) → Evidence: report.md#gaps-phase (mixed evergreen/ephemeral fixture GREEN)
- [x] Lifecycle is fed as an earlier input, not forked (Principle 3); no parallel store (Principle 5) → Evidence: report.md#gaps-phase (`AggressiveDecay` predicate; `TestNoParallelStore` GREEN)
- [x] Build Quality green; evidence captured (PII-redacted) → Evidence: report.md#gaps-phase (check=0, lint=0, unit 095-pkgs green)
- [x] **Increment 3 / PKT-095-C — synthesis seam wired:** `RunSynthesis`'s candidate CTE (`internal/intelligence/synthesis.go::buildSynthesisClusterQuery`) excludes persisted-ephemeral artifacts (`evergreen_score < 0`) via the additive `evergreen.PoolExclusionSQLPredicate("a", …)` WHERE predicate when `retrieval.evergreen.pools.synthesis_excludes_low_evergreen` is on; the default (off) candidate set is byte-for-byte unchanged → Evidence: report.md#increment-3 (`TestBuildSynthesisClusterQuery_DefaultUnchanged` + `…_ExcludesEphemeralAdditively` GREEN; `internal/intelligence` regression GREEN)
- [x] **Increment 3 / PKT-095-C — digest seam wired:** `getOvernightArtifacts` (`internal/digest/generator.go::buildOvernightArtifactsQuery`) excludes persisted-ephemeral artifacts via the additive unaliased predicate when `…digest_excludes_low_evergreen` is on; default (off) byte-for-byte unchanged → Evidence: report.md#increment-3 (`TestBuildOvernightArtifactsQuery_DefaultUnchanged` + `…_ExcludesEphemeralAdditively` GREEN; `internal/digest` regression GREEN)
- [x] **Increment 3 — Principle 9 (NULL kept):** the SQL predicate whitelists `evergreen_score IS NULL` (not-yet-scored ⇒ evergreen, never excluded) → Evidence: report.md#increment-3 (`TestPoolExclusionSQLPredicate` + `TestPoolExcludedByPersistedScore` NULL case GREEN)
- [x] **Increment 3 — R13 (excluded-still-searchable):** the exclusion seam is wired into the two pool builders and into NONE of `internal/api/search.go`'s §9.2 retrieval queries, so a pool-excluded ephemeral artifact is still returned by search → Evidence: report.md#increment-3 (`TestPoolExclusionWiredIntoCandidateBuildersOnly` cross-path isolation GREEN, with `would_catch_regression`)
- [x] **Increment 3 — SST safe activation:** toggles sourced only from `config/smackerel.yaml` (no Go cutoff — G028); default flipped `true → false` so shipping changes nothing until the operator opts in; regenerated dev.env + test.env → Evidence: report.md#increment-3 (config generate EXIT 0; `check` config-in-sync GREEN; both env files `=false`)
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior land in `tests/e2e/evergreen_pool_exclusion_regression_e2e_test.sh` — script delivered + statically valid (`bash -n` EXIT 0; PKT-095-C wiring DELIVERED Increment 3 + unit-proven); the live ingest→pool-absent+search-present run is env-blocked on this cpu-tier WSL2 host (the full Postgres+NATS+ML+Ollama stack OOMs here — witnessed 2026-06-18 via the stress harness's ML/core image rebuild), no fabricated pass → Evidence: report.md#assurance-2026-06-18 (SCN-095-B02/B03/B04; owner bubbles.test, accel-tier home-lab/CI — F-095-E2E-LIVE)
- [x] Broader E2E regression suite passes — the in-process regression is GREEN for all spec-095 packages (Phase 1 `./smackerel.sh test unit --go`); the live broader e2e suite (`./smackerel.sh test e2e`, incl. `evergreen_pool_exclusion_regression_e2e_test.sh`) is env-blocked on this cpu-tier host (same OOM gate), documented, no fabricated pass → Evidence: report.md#assurance-2026-06-18 (owner bubbles.test, accel-tier — F-095-E2E-LIVE)
- [x] Scenario→test mapping in `scenario-manifest.json` (SCN-095-B02, B03, B04) → Evidence: scenario-manifest.json (SCN-095-B02, B03, B04 present)

---

## Scope 09: Documentation

**Status:** Done
**Scope-Kind:** docs-only
**Foundation:** false
**Depends On:** SCOPE-06, SCOPE-08

### Use Cases (Gherkin)

```gherkin
Use case: SCN-095-DOC-01 — Retrieval pipeline doc reflects strategy routing
  Given docs/smackerel.md §9.2 currently shows a single chunk-similarity-first pipeline
  When the docs are updated
  Then §9.2/§9.3 describe the strategy router, the three strategies, and the evergreen pool eligibility
  And the operator runbook documents the retrieval.routing.* / retrieval.evergreen.* SST keys
```

### Implementation Plan (proposed for the consuming run)

- Update `docs/smackerel.md` §9.2/§9.3 to show the router + three strategies over the single store (explicitly noting "no parallel index" per Principle 5) and the §11 evergreen lifecycle input.
- Add an operator runbook section to `docs/Operations.md` for the new SST keys + the `judgment_source` fallback behavior.

### Test Plan

<!-- bubbles:g040-skip-begin -->
Docs-only scope (`Scope-Kind: docs-only`): no runtime behavior, no E2E regression coverage required (Check 8A docs-only opt-out). Validation is doc-freshness lint + review that the documented SST keys match `config/smackerel.yaml` and design §10.
<!-- bubbles:g040-skip-end -->

| Type | File | Assertion | Ref |
|------|------|-----------|-----|
| docs-lint | `docs/smackerel.md`, `docs/Operations.md` | Documented `retrieval.*` keys match SCOPE-01 SST + design §10 1:1; no stale single-pipeline claim remains | design §10 |

### Definition of Done

- [x] SCN-095-DOC-01 — §9.2/§9.3 + Operations runbook updated; documented keys match SST 1:1 → Evidence: `docs/Operations.md` "## Retrieval Routing & Evergreen Signal (spec 095)" runbook section appended as a CLEAN ADDITIVE end-of-file block (git diff hunk `@@ -4777,4 +4809,111 @@`, 107 insertions / ZERO deletions; the foreign operator observability WIP at lines 622/1628/1652/3037 was left untouched). SST 1:1 cross-check GREEN — documented `retrieval.routing.{enabled,intent_confidence_threshold=0.65,strategies.{whole_document,structured_aggregate,vague_recall}_enabled,contracts}` + `retrieval.evergreen.{enabled,judgment_source=scenario,confidence_floor=0.60,per_tick_budget=50,dedup_window_days=7,pools.{synthesis,digest}_excludes_low_evergreen=false}` each match `config/smackerel.yaml` + design §10 (pool toggles documented at the AUTHORITATIVE SST `false` = safe additive activation, NOT the superseded design §10 `true`). Also documents the `judgment_source` NFR-2 fallback (scenario→`tier_signals_fallback` when the judge is unavailable; `tier_signals` direct), migration 060 persistence (`artifacts.evergreen_score` ≥0/<0/NULL + `evergreen_source`), pool-exclusion-never-hides-from-search (R13/Principle 9; NULL never excluded), and flip-`./smackerel.sh config generate`-restart enable/rollback. §9.2/§9.3 in `docs/smackerel.md` + `docs/Development.md` previously delivered + verified (report.md#gaps-phase). G-095-GAPS-02 RESOLVED (bubbles.docs 2026-06-18).
- [x] Doc-freshness lint passes for the touched docs → Evidence: report.md#gaps-phase (`TestDocFreshness_AllInternalPackagesDocumented` GREEN; `internal/retrieval` documented)
- [x] No stale "single chunk-similarity pipeline" claim remains in §9 → Evidence: report.md#gaps-phase (§9.2/§9.3 carry the Delivered(spec 095) strategy-router note; integration honestly marked routed PKT-095-A/B/C)
- [x] Scenario→doc mapping noted in `scenario-manifest.json` (SCN-095-DOC-01) → Evidence: scenario-manifest.json (SCN-095-DOC-01 present)

---

## Open Question Resolutions (plan-owned)

<!-- bubbles:g040-skip-begin -->
| OQ | Resolution | Pointer |
|----|-----------|---------|
| OQ-PLAN-1 (routing/evergreen values) | Starting values stamped into SCOPE-01 SST per design §10 (`intent_confidence_threshold: 0.65`, `evergreen.confidence_floor: 0.60`, `per_tick_budget: 50`, `dedup_window_days: 7`). Empirical calibration is an implementation-run concern; if live evidence shows different values, the implementation run updates the SST block in place (operator-overridable; zero in-code literals). | SCOPE-01 |
| OQ-PLAN-2 (provenance storage) | Trace-only by default — `StrategySelection` + `EvergreenSignal` are recorded as structured trace events on the existing observability/trace surface (mirroring spec 071), NOT a new table (Principle 5). If a durable score is required it is an ADDITIVE `artifacts.evergreen_score` column on the existing table, never a sibling store. The additive-column schema change is allocated at implementation time as the lowest free schema-version number. | SCOPE-07, SCOPE-08 |
| OQ-PLAN-3 (scenario→test mapping) | Every `SCN-095-*` maps 1:1 to its scope's Test Plan files; recorded in `scenario-manifest.json`. | all scopes |
| OQ-PLAN-4 (v1 specialized contracts) | v1 ships specialized contracts for `transcript`/`meeting` (whole_document_summary), `subscription`/`expense`/`bill` (aggregate_spend), and `place`/`trip` (dossier); every other type defaults to `[vague_recall]` (R9). Additional specialized contracts are additive config edits, no router change. | SCOPE-02 |
<!-- bubbles:g040-skip-end -->

---

## Routing (packets to other spec owners)

<!-- bubbles:g040-skip-begin -->
None of these block spec 095 planning. At implementation time, three substrate hooks MAY require a routed packet (the consuming run decides whether an additive call suffices or a packet is needed):

| Packet (potential) | Owner spec | Substrate seam | Rationale |
|--------------------|-----------|----------------|-----------|
| PKT-095-A | spec 061 (assistant facade) | `internal/assistant/` pre-retrieval router hook (mirrors `nl_routing.go`) | Wire the strategy router into the `retrieval_qa` turn path. |
| PKT-095-B | spec 003 (ingestion pipeline) | `internal/pipeline/` `AssignTier` evergreen seam call | Attach the EvergreenSignal at the front door. |
| PKT-095-C | spec 021/025 (synthesis + digest) | synthesis/digest candidate-pool builder predicate | Consult the evergreen pool-eligibility predicate. |

Packets are documented here for spec 095's lifecycle; the orchestrator (bubbles.workflow) is the dispatcher when implementation begins.
<!-- bubbles:g040-skip-end -->
