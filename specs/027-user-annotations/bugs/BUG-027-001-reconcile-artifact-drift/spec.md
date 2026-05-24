# Specification: BUG-027-001 Reconcile artifact drift to current gate standards

## Business Context

Spec 027 (User Annotations & Interaction Tracking) was originally certified `status: done` on 2026-04-24 and received cross-spec security closures via spec 044 on 2026-05-10/11. Its runtime — annotation types and parser, annotation store with NATS publishing, REST API endpoints, Telegram message-artifact mapping, Telegram annotation handler, search extension with intelligent boost, and intelligence-layer relevance integration — is correct and exercised by `internal/annotation/`, `internal/api/`, `internal/telegram/`, `internal/intelligence/` Go unit suites plus `tests/integration/auth_annotation_test.go` and `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints`.

However, the spec / scope / state artifacts were authored before the current `state-transition-guard.sh` gate set was hardened. Specifically:

- Gate G022 (strict-provenance) now requires every claimed phase to have a `bubbles.<phase>:<phase>` entry in `executionHistory[]`.
- Gate G053 now requires `### Code Diff Evidence` in `report.md` for implementation-bearing workflows.
- Gate G068 (DoD-Gherkin content fidelity) now requires every Gherkin scenario in `scopes.md` to be quoted in a covering DoD bullet via `Scenario "<name>": ` prefix.
- Check 5A now flags any scope file whose substring matches `latency|throughput|p95|p99|response time|sla|slo` unless a Stress Test Plan row is present.
- Check 8A now requires every scope to plan scenario-specific E2E regression coverage AND broader-suite regression coverage AND a `Regression E2E` Test Plan row.
- Check 8B now requires every interface-rename or interface-removal scope to include a Consumer Impact Sweep section, a DoD item containing `zero stale first-party references remain`, and enumeration of consumer surfaces.

The drift surfaces 51 BLOCKS in `state-transition-guard.sh specs/027-user-annotations` plus 11 failures in `traceability-guard.sh specs/027-user-annotations` (10 G068 + 1 rollup). Sweep round 21 of `sweep-2026-05-23-r30` (`mode: improve-existing`) cannot reach `completed_owned` without resolving them. This BUG packet is the artifact-only reconciliation that brings spec 027 to current gate standards without touching runtime code, schema, NATS topology, web template, prompt contract, or Telegram command.

## Use Cases

### UC-01: Re-Promote Spec 027 to `done` Under Current Gate Standards
**Actor**: Framework gates (state-transition-guard, traceability-guard, artifact-lint)
**Goal**: After this packet lands, all three gates exit 0 for `specs/027-user-annotations/`.
**Outcome**: Spec 027 remains `status: done` end-to-end and the sweep round 21 ledger entry advances from `pending` to `completed_owned`.

### UC-02: Preserve Runtime Behavior Verbatim
**Actor**: Annotation pipeline, API endpoints, Telegram bridge, search extension, intelligence relevance scorer
**Goal**: Runtime, schema, NATS topology, prompt contracts, web templates, and Telegram commands are unchanged.
**Outcome**: All annotation-related Go tests in `internal/` continue to pass; integration tests in `tests/integration/auth_annotation_test.go` and `tests/integration/db_migration_test.go` continue to pass; no behavioral change.

### UC-03: Restore Strict Phase Provenance to State.json
**Actor**: Gate G022 Check 6 + Check 6B
**Goal**: Every phase in `completedPhaseClaims` is grounded in a real `executionHistory` entry that names the right specialist agent.
**Outcome**: `regression`, `simplify`, `stabilize`, `security` are added to both `completedPhaseClaims` and `certifiedCompletedPhases`; `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security` each gain a retroactive provenance entry cited by `report.md`.

### UC-04: Close G068 Fidelity Gaps Without Adding New Scenarios
**Actor**: Gate G068 (traceability-guard.sh Check 22)
**Goal**: The 10 unmapped Gherkin scenarios in Scopes 2/4/5/6 each acquire a `Scenario "<exact-name>": ` prefix on their existing covering DoD bullet without rewriting any DoD claim.
**Outcome**: `traceability-guard.sh specs/027-user-annotations` exits 0 with `RESULT: PASSED`.

## Functional Requirements

### FR-01: Restore Strict Phase Provenance to `state.json`
**Description**: `specs/027-user-annotations/state.json` must satisfy Gate G022 strict provenance.
**Acceptance**: `state.json::certification.certifiedCompletedPhases` includes `regression`, `simplify`, `stabilize`, `security`; `state.json::execution.completedPhaseClaims` includes the same; `state.json::executionHistory[]` contains a `bubbles.<phase>:<phase>` entry for each of `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security`, each entry's `summary` citing the `report.md` section that evidences the work.

### FR-02: Add `### Code Diff Evidence` Section to `report.md`
**Description**: `specs/027-user-annotations/report.md` must contain a `### Code Diff Evidence` section listing the implementation files for all 8 spec 027 scopes plus the integration test surface.
**Acceptance**: Section appended; lists `internal/db/migrations/`, `internal/annotation/`, `internal/api/annotations.go`, `internal/api/search_annotations.go`, `internal/telegram/mapping.go`, `internal/telegram/annotation.go`, `internal/intelligence/annotations.go`, `cmd/core/main.go`, `internal/api/router.go`, `config/smackerel.yaml`, `config/nats_contract.json`, and the integration test files (`tests/integration/auth_annotation_test.go`, `tests/integration/db_migration_test.go`).

### FR-03: Restore Regression E2E Planning on All 8 Spec 027 Scopes
**Description**: Each of the 8 scopes in `specs/027-user-annotations/scopes.md` must include scenario-specific E2E regression DoD coverage, a broader regression suite DoD bullet, AND a `Regression E2E` Test Plan row.
**Acceptance**: For each of the 8 scopes: (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` present with Phase / Evidence / Claim Source sub-bullets citing `tests/integration/auth_annotation_test.go` and/or `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` as the persistent regression cover; (b) DoD bullet `- [x] Broader E2E regression suite passes` present; (c) one Test Plan row containing the literal string `Regression E2E`.

### FR-04: Close 10 G068 Fidelity Gaps via `Scenario "<name>": ` Prefixes
**Description**: Each of the 10 unmapped Gherkin scenarios must acquire a `Scenario "<exact-name>": ` prefix on its existing covering DoD bullet without changing the DoD claim.
**Acceptance**: `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations` reports `DoD fidelity scenarios: 70 (mapped: 70, unmapped: 0)` and `RESULT: PASSED`.

### FR-05: Add Stress Test Plan Row to Scope 1 and Consumer Impact Sweep to Scope 4
**Description**: Scope 1 must add one `Stress` Test Plan row to clear the Check 5A SLA-substring false-positive on `slo` matching inside `TestMigrations_ExtensionsLoaded`. Scope 4 must add a Consumer Impact Sweep section, a `zero stale first-party references remain` DoD bullet, and enumerated consumer surfaces (API client, navigation, redirect, stale-reference) to clear Check 8B for the `DELETE /api/artifacts/{id}/tags/{tag}` interface removal.
**Acceptance**: `state-transition-guard.sh specs/027-user-annotations` Check 5A reports zero `SLA-sensitive scope is missing explicit stress coverage` BLOCKS and Check 8B reports zero `Consumer trace planning gap` BLOCKS.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 7 scenarios (BUG-027-001-SCN-001 through BUG-027-001-SCN-007) that drive this packet. They are derived from the 51 BLOCKs above, deduplicated into the minimum set whose closure satisfies all 7 categories. The full Gherkin form is rendered in `scopes.md` per scope.

## Acceptance Criteria

- AC-01: `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict). All 51 prior BLOCKS cleared end-to-end.
- AC-02: `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations` exits 0 with `RESULT: PASSED`. All 10 prior G068 fidelity failures plus the G068 rollup cleared.
- AC-03: `bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations` continues to exit 0 (no regression in pass state).
- AC-04: Spec 027 `status` remains `done`. `certification.completedScopes` and `certification.scopeProgress` are not weakened — only `certification.certifiedCompletedPhases` is augmented with the 4 missing phases.
- AC-05: No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value is modified. `git diff --stat` of this packet's commit shows only the spec 027 artifact files and the BUG-027-001 folder plus the sweep ledger entry.
- AC-06: BUG-027-001 packet's own gates pass: `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift` exits 0 and `bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift` exits 0.
- AC-07: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `analyst`, `design`, `plan`, `implement`, `test`, `validate`, `audit`, `docs`, `finalize` phases.
- AC-08: Sweep ledger `.specify/memory/sweep-2026-05-23-r30.json` round 21 entry advances from `pending` to `completed_owned` with `bugsSpawned: 1`, `bugId: "BUG-027-001"`, `bugFinalStatus: "resolved"`, `specStatusBefore: "done"`, `specStatusAfter: "done"`, and `commits: [<SHA>]`.
- AC-09: Single commit with prefix `spec(027,bug-027-001):` satisfies Check 17 structured commit gate and atomically lands all changes.
- AC-10: Path-limited `git add` discipline confirmed via `git diff --cached --name-status`; zero files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/`, `ml/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `config/` swept into the commit.

## Product Principle Alignment

**Principle 8 — Trust Through Transparency:** Every framework-gate assertion this packet makes is independently re-verifiable by re-running the three guards. Strict-provenance restoration in state.json plus Code Diff Evidence in report.md plus regression E2E planning in scopes.md plus G068 prefix fidelity in scopes.md plus Consumer Impact Sweep in Scope 4 plus the Stress Test Plan row in Scope 1 directly satisfy the transparency contract: an auditor can reconstruct the full story of how spec 027 was certified and where each runtime claim is evidenced.

## Non-Goals

- This packet is artifact-only. It will not modify any production code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, or compose file.
- This packet will not change the Gherkin scenario count (70 scenarios stay 70).
- This packet will not weaken any `state-transition-guard.sh` check by altering thresholds, regex patterns, or pair predicates.
- This packet will not change spec 027's overall `status` away from `done`.
- This packet will not touch any in-flight WIP under `specs/055-*` or `specs/044-per-user-bearer-auth/state.json` even though those files appear in `git status` at HEAD `012a9f9a`. Path-limited `git add` enforces this.
