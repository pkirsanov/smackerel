# Specification: BUG-028-003 Reconcile artifact drift to current gate standards

## Business Context

Spec 028 (Actionable Lists & Resource Tracking) was originally certified `status: done` on 2026-04-19 and received subsequent bug-close iterations via BUG-028-001 (G068 DoD-Gherkin fidelity, 2026-04-27) and BUG-028-002 (compare-aggregator silent JSON swallow, parent-expanded harden-to-doc, 2026-05-12). Its runtime — DB migration consolidated into `001_initial_schema.sql` lines 545-588, list types in `internal/list/types.go`, list store CRUD in `internal/list/store.go`, recipe / reading / compare aggregators in `internal/list/{recipe,reading}_aggregator.go`, list generator in `internal/list/generator.go`, REST API endpoints in `internal/api/lists.go`, Telegram `/list` command in `internal/telegram/list.go`, intelligence integration in `internal/intelligence/lists.go`, plus the NATS `lists.created`/`lists.completed` events — is correct and exercised by `internal/list/{types,store,recipe_aggregator,reading_aggregator,generator,harden}_test.go` Go unit suites, `internal/api/lists_test.go`, `internal/telegram/list_test.go`, `internal/intelligence/lists_test.go`, plus `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` and `tests/integration/artifact_crud_test.go::TestList_Chaos_CascadeDeleteDuringConcurrentUpdates`.

However, the spec / scope / state artifacts were authored before the current `state-transition-guard.sh` gate set was hardened. Specifically:

- Gate G022 (strict-provenance) now requires every claimed phase to have a `bubbles.<phase>:<phase>` entry in `executionHistory[]`.
- Gate G053 now requires `### Code Diff Evidence` in `report.md` for implementation-bearing workflows.
- Check 5A now flags any scope file whose substring matches `latency|throughput|p95|p99|response time|sla|slo` unless a Stress Test Plan row is present.
- Check 8A now requires every scope to plan scenario-specific E2E regression coverage AND broader-suite regression coverage AND a `Regression E2E` Test Plan row.
- Check 17 now requires `full-delivery` specs to have at least one commit with prefix `^spec\(<NNN>\)|^bubbles\(<NNN>/`.

The drift surfaces 38 BLOCKS in `state-transition-guard.sh specs/028-actionable-lists`. Sweep round 22 of `sweep-2026-05-23-r30` (`mode: stochastic-quality-sweep`, trigger `harden`, mapped child workflow mode `harden-to-doc`) cannot reach `completed_owned` without resolving them. This BUG packet is the artifact-only reconciliation that brings spec 028 to current gate standards without touching runtime code, schema, NATS topology, web template, prompt contract, or Telegram command.

## Use Cases

### UC-01: Re-Promote Spec 028 to `done` Under Current Gate Standards
**Actor**: Framework gates (state-transition-guard, traceability-guard, artifact-lint)
**Goal**: After this packet lands, all three gates exit 0 for `specs/028-actionable-lists/`.
**Outcome**: Spec 028 remains `status: done` end-to-end and the sweep round 22 ledger entry advances from `pending` to `completed_owned`.

### UC-02: Preserve Runtime Behavior Verbatim
**Actor**: List pipeline (store, aggregators, generator), REST API, Telegram bridge, intelligence integration
**Goal**: Runtime, schema, NATS topology, prompt contracts, web templates, and Telegram commands are unchanged.
**Outcome**: All list-related Go tests in `internal/list/`, `internal/api/`, `internal/telegram/`, `internal/intelligence/` continue to pass; integration tests in `tests/integration/artifact_crud_test.go` continue to pass; no behavioral change.

### UC-03: Restore Strict Phase Provenance to State.json
**Actor**: Gate G022 Check 6 + Check 6B
**Goal**: Every phase in `completedPhaseClaims` is grounded in a real `executionHistory` entry that names the right specialist agent.
**Outcome**: `regression`, `simplify`, `stabilize`, `security` are added to both `completedPhaseClaims` and `certifiedCompletedPhases`; `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security` each gain a retroactive provenance entry cited by `report.md`.

### UC-04: Close Check 8A Regression E2E Planning On All 8 Spec 028 Scopes
**Actor**: Gate G016 / Check 8A regression E2E planning
**Goal**: Each of the 8 scopes acquires a scenario-specific regression DoD bullet, a broader-suite regression DoD bullet, and a `Regression E2E` Test Plan row without inventing test files that do not exist.
**Outcome**: `tests/integration/artifact_crud_test.go::{TestList_CreateAndUpdateStatus, TestList_Chaos_CascadeDeleteDuringConcurrentUpdates}` are cited as the persistent regression cover for the runtime claims, and `state-transition-guard.sh specs/028-actionable-lists` reports zero Check 8A BLOCKS.

## Functional Requirements

### FR-01: Restore Strict Phase Provenance to `state.json`
**Description**: `specs/028-actionable-lists/state.json` must satisfy Gate G022 strict provenance.
**Acceptance**: `state.json::certification.certifiedCompletedPhases` includes `regression`, `simplify`, `stabilize`, `security`; `state.json::execution.completedPhaseClaims` includes the same; `state.json::executionHistory[]` contains a `bubbles.<phase>:<phase>` entry for each of `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security`, each entry's `summary` citing the `report.md` section that evidences the work.

### FR-02: Add `### Code Diff Evidence` Section to `report.md`
**Description**: `specs/028-actionable-lists/report.md` must contain a `### Code Diff Evidence` section listing the implementation files for all 8 spec 028 scopes plus the integration test surface.
**Acceptance**: Section appended; lists `internal/db/migrations/001_initial_schema.sql` (lines 545-588), `internal/list/{types,store,recipe_aggregator,reading_aggregator,generator}.go`, `internal/api/lists.go`, `internal/telegram/list.go`, `internal/intelligence/lists.go`, `cmd/core/main.go`, `config/smackerel.yaml` (lists block), `config/nats_contract.json` (`lists.created`, `lists.completed`), and the integration test files.

### FR-03: Restore Regression E2E Planning on All 8 Spec 028 Scopes
**Description**: Each of the 8 scopes in `specs/028-actionable-lists/scopes.md` must include scenario-specific E2E regression DoD coverage, a broader regression suite DoD bullet, AND a `Regression E2E` Test Plan row.
**Acceptance**: For each of the 8 scopes: (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` present with Phase / Evidence / Claim Source sub-bullets citing `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` and/or `tests/integration/artifact_crud_test.go::TestList_Chaos_CascadeDeleteDuringConcurrentUpdates` as the persistent regression cover; (b) DoD bullet `- [x] Broader E2E regression suite passes` present; (c) one Test Plan row containing the literal string `Regression E2E`.

### FR-04: Add Stress Test Plan Row to Clear Check 5A SLA-Substring False-Positive
**Description**: Scope 5 (List Generator) must add one `Stress` Test Plan row to clear the Check 5A SLA-substring false-positive on `slo` matching inside `slog.Warn` at `scopes.md` line 389.
**Acceptance**: `state-transition-guard.sh specs/028-actionable-lists` Check 5A reports zero `SLA-sensitive scope is missing explicit stress coverage` BLOCKS.

### FR-05: Land Reconciliation Under Structured `spec(028,bug-028-003):` Commit Prefix
**Description**: The reconciliation commit must use prefix `spec(028,bug-028-003):` so Check 17 sees at least one `^spec\(028\)|^bubbles\(028/` commit in the spec's `git log`.
**Acceptance**: `git log --pretty='%h %s' -- specs/028-actionable-lists/ | grep -cE '^[0-9a-f]+ (spec\(028\)|bubbles\(028/)'` returns at least 1.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 6 scenarios (BUG-028-003-SCN-001 through BUG-028-003-SCN-006) that drive this packet. They are derived from the 38 BLOCKs above, deduplicated into the minimum set whose closure satisfies all 6 categories. The full Gherkin form is rendered in `scopes.md` per scope.

## Acceptance Criteria

- AC-01: `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict). All 38 prior BLOCKS cleared end-to-end.
- AC-02: `bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists` continues to exit 0 with `RESULT: PASSED` (no regression to the BUG-028-001 closure).
- AC-03: `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists` continues to exit 0.
- AC-04: Spec 028 `status` remains `done`. `certification.completedScopes` and `certification.scopeProgress` are not weakened — only `certification.certifiedCompletedPhases` is augmented with the 4 missing phases.
- AC-05: No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value is modified. `git diff --stat` of this packet's commit shows only the spec 028 artifact files and the BUG-028-003 folder.
- AC-06: BUG-028-003 packet's own gates pass: `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists/bugs/BUG-028-003-reconcile-artifact-drift` exits 0 and `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists/bugs/BUG-028-003-reconcile-artifact-drift` exits 0.
- AC-07: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`, `docs` phases (10 phases per bugfix-fastlane mode).
- AC-08: Sweep ledger `.specify/memory/sweep-2026-05-23-r30.json` round 22 entry advances from `pending` to `completed_owned` with `bugsSpawned: 1`, `bugId: "BUG-028-003"`, `bugFinalStatus: "resolved"`, `specStatusBefore: "done"`, `specStatusAfter: "done"`, and `commits: [<SHA>]` (parent owns this update; the bug commit itself does not modify the ledger).
- AC-09: Single commit with prefix `spec(028,bug-028-003):` satisfies Check 17 structured commit gate and atomically lands all changes.
- AC-10: Path-limited `git add` discipline confirmed via `git diff --cached --name-status`; zero files from `specs/055-*`, `specs/053-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/`, `ml/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `config/` swept into the commit.

## Product Principle Alignment

**Principle 8 — Trust Through Transparency:** Every framework-gate assertion this packet makes is independently re-verifiable by re-running the three guards. Strict-provenance restoration in state.json plus Code Diff Evidence in report.md plus regression E2E planning in scopes.md plus the Stress Test Plan row in Scope 5 directly satisfy the transparency contract: an auditor can reconstruct the full story of how spec 028 was certified and where each runtime claim is evidenced.

## Non-Goals

- This packet is artifact-only. It will not modify any production code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, or compose file.
- This packet will not change the Gherkin scenario count for spec 028 (34 scenarios stay 34; the 6 new scenarios are internal to this BUG packet and live in the bug's own manifest).
- This packet will not weaken any `state-transition-guard.sh` check by altering thresholds, regex patterns, or pair predicates.
- This packet will not change spec 028's overall `status` away from `done`.
- This packet will not touch any in-flight WIP under `specs/055-*`, `specs/053-*`, `specs/044-per-user-bearer-auth/state.json`, or any of the unrelated dirty paths in `cmd/`, `internal/`, `ml/`, `web/`, `docs/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, or `.github/bubbles/` that appear in `git status` at HEAD `42863de8`. Path-limited `git add` enforces this.
