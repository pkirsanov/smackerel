# Specification: BUG-024-002 Reconcile artifact drift to current gate standards + close real §22.7 connector-inventory drift (15 → 16, QF Decisions)

## Business Context

Spec 024 (Design Document Reconciliation) was originally certified `status: done` on 2026-04-10 and received four follow-up reconciliation passes through 2026-04-22 plus a spec-review on 2026-04-23 and a cross-spec implement entry on 2026-05-08. Its product surface — `docs/smackerel.md` reconciled against the committed `internal/connector/` packages, `internal/db/migrations/`, `internal/api/router.go`, `cmd/core/main.go`, and the NATS / PostgreSQL stack — is correct and exercised by the grep/awk validation suite in `scopes.md` Test Plan + DoD evidence blocks.

However, two truths have drifted since:

1. **Real implementation drift.** Spec 041 (`status: done_with_concerns`) introduced `internal/connector/qfdecisions/` on 2026-05-22 (commits `39ca4fcb`, `c22151a5`, `43ce5096`). `cmd/core/connectors.go` now registers 16 connectors, but `docs/smackerel.md` §22.7 Committed Connector Inventory still claims `(15 connectors)` with rows 1–15 and §24-A architecture tree still claims `Connector plugins (15 committed)` with 15 leaves. The R-006 contract that spec 024 owns ("All connector lists in the document must account for the implemented connectors") is silently violated.
2. **Governance drift.** The spec / scope / state artifacts were authored before the current `state-transition-guard.sh`, `artifact-freshness-guard.sh`, Gate G053, Gate G060, Check 17, Check 8A, and Check 8B standards were hardened. Specifically:
   - Gate G022 (strict-provenance) now requires every claimed phase to have a `bubbles.<phase>:<phase>` entry in `executionHistory[]`.
   - Gate G022 (specialist completeness) requires `regression`, `simplify`, `stabilize`, `security` in `certifiedCompletedPhases`.
   - Gate G053 now requires `### Code Diff Evidence` in `report.md` for implementation-bearing workflows.
   - Gate G060 now requires red→green TDD evidence markers when `policySnapshot.tdd.mode` is `scenario-first`.
   - Check 5A flags any scope file whose substring matches `latency|throughput|p95|p99|response time|sla|slo` unless a Stress Test Plan row is present.
   - Check 8A requires every scope to plan scenario-specific E2E regression coverage AND broader-suite regression coverage AND a `Regression E2E` Test Plan row.
   - Check 8B requires every scope that touches shared fixture/bootstrap infrastructure to include a Shared Infrastructure Impact Sweep section, canary + rollback DoD items, a canary Test Plan row, and explicit enumeration of downstream consumer surfaces. `docs/smackerel.md` is the product/architecture truth document every other spec reads; Scope 2 of spec 024 owns that file and triggers Check 8B.
   - Check 17 requires at least one git commit whose subject prefix matches `^spec\(024\)|^bubbles\(024/`.
   - `artifact-freshness-guard.sh` Check 1 case-insensitively flags any heading containing `Superseded|Suppressed` as a boundary; subsequent active-section headings are reported as "active-looking heading after freshness boundary". `spec.md` line 123 (`### BS-005: Phased Plan References Superseded Technology`) and `design.md` bash-fenced comments at lines 512/515/518 (`# Zero unmarked OpenClaw references (§4 superseded header …)`) hit this substring trigger and cascade 19 sub-failures.

The drift surfaces 32 BLOCKS in `state-transition-guard.sh specs/024-design-doc-reconciliation` + 19 sub-failures in `artifact-freshness-guard.sh specs/024-design-doc-reconciliation` + 1 real connector-inventory drift in `docs/smackerel.md` §22.7 + §24-A. Sweep round 29 of `sweep-2026-05-23-r30` (`mode: reconcile-to-doc`) cannot reach `completed_owned` without resolving them.

This BUG packet is the **reconcile-to-doc fastlane** that brings spec 024 to current gate standards AND closes the real §22.7 drift without touching runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, `smackerel.yaml`, or any of the explicitly-ignored WIP surfaces (spec 055, `cmd/core/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`, `specs/044-per-user-bearer-auth/state.json`).

## Use Cases

### UC-01: Re-Promote Spec 024 to `done` Under Current Gate Standards
**Actor**: Framework gates (state-transition-guard, artifact-freshness-guard, traceability-guard, artifact-lint)
**Goal**: After this packet lands, all four gates exit 0 for `specs/024-design-doc-reconciliation/`.
**Outcome**: Spec 024 remains `status: done` end-to-end and the sweep round 29 ledger entry advances from `pending` to `completed_owned`.

### UC-02: Reconcile §22.7 Connector Inventory With Live Registry (15 → 16)
**Actor**: A reader of `docs/smackerel.md` (engineer, auditor, investor, downstream-spec author)
**Goal**: `docs/smackerel.md` §22.7 and §24-A accurately enumerate the 16 connectors registered in `cmd/core/connectors.go`, including QF Decisions (companion-mode, boundary-enforced per spec 041 — read-only ingestion, no financial advice generation).
**Outcome**: The R-006 contract spec 024 owns is restored. Downstream specs cannot accidentally re-introduce the 15-vs-16 drift because the inventory now exactly matches `find internal/connector -maxdepth 1 -mindepth 1 -type d` count.

### UC-03: Preserve Runtime Behavior Verbatim
**Actor**: Connector registry, every committed connector, all REST endpoints, NATS publishers, search/digest paths
**Goal**: Runtime code, schema, NATS topology, prompt contracts, web templates, Telegram commands, deploy scripts, and compose files are unchanged.
**Outcome**: `./smackerel.sh test unit` continues to pass; no behavioral change; the only `docs/smackerel.md` change is §22.7 (header + intro + row 16) and §24-A (tree count + leaf 16).

### UC-04: Restore Strict Phase Provenance to `state.json`
**Actor**: Gate G022 Check 6 + Check 6B
**Goal**: Every phase in `completedPhaseClaims` is grounded in a real `executionHistory` entry that names the right specialist agent, and `regression`/`simplify`/`stabilize`/`security` are added to both `completedPhaseClaims` and `certifiedCompletedPhases`.
**Outcome**: `state-transition-guard.sh` Check 6 + 6B report zero impersonation BLOCKS and zero missing-phase BLOCKS.

### UC-05: Clear Artifact-Freshness False Positives Without Weakening Detection
**Actor**: `artifact-freshness-guard.sh` Check 1
**Goal**: `spec.md` BS-005 heading and `design.md` bash-fenced comments are reworded to use `Outdated` / `historical` instead of `Superseded` so the substring detector does not cascade-flag 19 active-section headings.
**Outcome**: `artifact-freshness-guard.sh` exits 0 with `RESULT: PASSED`. The guard's detection logic is unchanged — only the false-positive substrings are corrected.

## Functional Requirements

### FR-01: Update `docs/smackerel.md` §22.7 Connector Inventory From 15 to 16
**Description**: §22.7 heading, intro line, and table must reflect the 16 committed connectors registered in `cmd/core/connectors.go`.
**Acceptance**: Line 2370 reads `### 22.7 Committed Connector Inventory (16 connectors)`. Line 2372 reads `All 16 connectors are implemented under \`internal/connector/\` in Go:`. The table includes row 16 for QF Decisions: `| 16 | QF Decisions | \`qfdecisions/\` | Companion | QF DecisionPacket ingestion as read-only companion (spec 041 — boundary: no financial advice generation) |`.

### FR-02: Update `docs/smackerel.md` §24-A Architecture Tree From 15 to 16
**Description**: §24-A architecture-tree connector-plugins block must reflect 16 committed connectors.
**Acceptance**: Line 2477 reads `│   ├── Connector plugins (16 committed)`. A new leaf `│   │   ├── QF Decisions (qfdecisions/)` is inserted alongside the existing 15 leaves, placed after `│   │   └── YouTube (youtube/)` (which is converted from terminal `└──` to `├──` since QF Decisions is now the new last committed leaf followed by the `Planned connectors:` block).

### FR-03: Restore Strict Phase Provenance to `state.json`
**Description**: `specs/024-design-doc-reconciliation/state.json` must satisfy Gate G022 strict provenance + specialist completeness.
**Acceptance**: `state.json::certification.certifiedCompletedPhases` includes `regression`, `simplify`, `stabilize`, `security`, `bootstrap`. `state.json::execution.completedPhaseClaims` includes the same. `state.json::executionHistory[]` contains a `bubbles.<phase>:<phase>` entry for each of `bootstrap`, `plan`, `test`, `validate`, `audit`, `chaos`, `docs`, `design`, `regression`, `simplify`, `stabilize`, `security`, each entry's `summary` citing the `report.md` section that evidences the work.

### FR-04: Add `### Code Diff Evidence` Section to `report.md`
**Description**: `specs/024-design-doc-reconciliation/report.md` must contain a `### Code Diff Evidence` section listing the design-doc file plus the validation surfaces.
**Acceptance**: Section appended; lists `docs/smackerel.md` (with the §22.7, §24-A, §2, §3, §4, §6, §7, §8, §14, §17, §18, §19, §21.3, §22, §24 edit groups originally landed) and the spec 024 artifact files themselves (since this is a docs-only feature whose "implementation" is the design-doc edits + spec artifact updates).

### FR-05: Restore Regression E2E Planning on Both Spec 024 Scopes
**Description**: Each of the 2 scopes in `specs/024-design-doc-reconciliation/scopes.md` must include scenario-specific E2E regression DoD coverage, a broader regression suite DoD bullet, AND a `Regression E2E` Test Plan row.
**Acceptance**: For each of the 2 scopes: (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` present with Phase / Evidence / Claim Source sub-bullets citing the grep/awk validation suite that proves §4/§8/§14 (Scope 1) and §19/§21.3/§22 (Scope 2) stayed reconciled; (b) DoD bullet `- [x] Broader E2E regression suite passes` present; (c) one Test Plan row containing the literal string `Regression E2E`.

### FR-06: Add Stress Test Plan Row to Clear Check 5A SLA-Substring False Positive
**Description**: Scope 1 must add one `Stress` Test Plan row to clear the Check 5A SLA-substring false-positive (matches `sla` inside Slack-reconciliation language). The spec 024 reconciliation does not make a real SLA / latency / throughput claim; the Stress row asserts that the design-doc reconciliation does not regress under repeated full-document grep/awk sweeps.
**Acceptance**: `state-transition-guard.sh specs/024-design-doc-reconciliation` Check 5A reports zero `SLA-sensitive scope is missing explicit stress coverage` BLOCKS.

### FR-07: Add Shared Infrastructure Impact Sweep + Canary + Rollback to Scope 2
**Description**: Scope 2 edits §19, §21.3, §22 of `docs/smackerel.md` — the canonical product/architecture truth document every other spec reads. Check 8B treats that as shared-fixture/bootstrap infrastructure. Scope 2 must add (a) a `Shared Infrastructure Impact Sweep` section enumerating downstream readers (every spec under `specs/`, every BUG packet, every sweep summary, the README, `docs/Architecture.md`, `docs/Deployment.md`, investor-facing docs, the design-doc reconciliation contract itself in `spec.md` R-006), (b) a canary DoD item asserting that the §22.7 row-16 + §24-A leaf-16 edits were preview-verified by re-rendering the table and tree before the edit landed, (c) a rollback DoD item asserting that the §22.7 + §24-A edits are atomic single-commit reversible via `git revert <SHA>`, (d) a canary Test Plan row, and (e) explicit enumeration of consumer surfaces in the Test Plan.
**Acceptance**: `state-transition-guard.sh specs/024-design-doc-reconciliation` Check 8B reports zero `shared-infrastructure planning requirement(s) missing` BLOCKS.

### FR-08: Add Red→Green TDD Evidence Markers (Gate G060)
**Description**: Because `policySnapshot.tdd.mode` is `scenario-first`, `scopes.md` and/or `report.md` must carry red→green TDD evidence markers. For a docs-only reconciliation feature, the analogous TDD pattern is: red = "grep pre-edit finds the drift", green = "grep post-edit finds zero drift". Each scope's `Scenario-First TDD Evidence` subsection records both states for at least one scenario.
**Acceptance**: `state-transition-guard.sh specs/024-design-doc-reconciliation` Gate G060 reports zero BLOCKS.

### FR-09: Clear Artifact-Freshness Substring False Positives in spec.md + design.md
**Description**: `spec.md` line 123 heading and `design.md` lines 512/515/518 bash-fenced comments must be reworded to use `Outdated` / `historical` instead of `Superseded` so the `artifact-freshness-guard.sh` Check 1 substring detector does not cascade-flag 19 active-section headings. The original semantic meaning (the §4 OpenClaw block in `docs/smackerel.md` is the superseded surface) is preserved by retaining `SUPERSEDED` in the design-doc itself, which is outside spec 024's artifact set.
**Acceptance**: `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 with `RESULT: PASSED`.

### FR-10: Single Atomic Commit With Structured Prefix Satisfies Check 17
**Description**: All BUG-024-002 packet artifacts + the §22.7 + §24-A edits in `docs/smackerel.md` + the parent spec 024 governance backfill land in a single atomic commit whose subject begins with `bubbles(024/bug-024-002):`.
**Acceptance**: `state-transition-guard.sh specs/024-design-doc-reconciliation` Check 17 reports zero `missing structured commit prefix` BLOCKS.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 8 scenarios (BUG-024-002-SCN-001 through BUG-024-002-SCN-008) that drive this packet. They are derived from the 32 + 19 + 1 findings above, deduplicated into the minimum set whose closure satisfies all 10 functional requirements. The full Gherkin form is rendered in `scopes.md`.

## Acceptance Criteria

- AC-01: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict). All 32 prior BLOCKS cleared end-to-end.
- AC-02: `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 with `RESULT: PASSED`. All 19 prior sub-failures cleared.
- AC-03: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` continues to exit 0 (no regression in pass state).
- AC-04: `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` continues to exit 0 with `RESULT: PASSED`.
- AC-05: `grep -nE "Connector plugins \(16 committed\)|Committed Connector Inventory \(16 connectors\)|All 16 connectors are implemented" docs/smackerel.md` returns exactly 3 hits at lines that now match the updated text; `grep -cE "qfdecisions|QF Decisions" docs/smackerel.md` returns at least 2 (one for the §22.7 row, one for the §24-A leaf).
- AC-06: `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l` continues to return `16` and matches the new `(16 connectors)` claim exactly (no live-vs-doc drift).
- AC-07: Spec 024 `status` remains `done`. `certification.completedScopes` and `certification.scopeProgress` are not weakened — only `certification.certifiedCompletedPhases` is augmented with the 5 missing phases (`regression`, `simplify`, `stabilize`, `security`, `bootstrap`).
- AC-08: No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value is modified. `git diff --stat` of this packet's commit shows only `docs/smackerel.md` + the spec 024 artifact files + the BUG-024-002 folder.
- AC-09: BUG-024-002 packet's own gates pass: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift` exits 0 and `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift` exits 0.
- AC-10: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `analyst`, `design`, `plan`, `implement`, `test`, `validate`, `audit`, `docs`, `finalize` phases.
- AC-11: Single commit with subject prefix `bubbles(024/bug-024-002):` satisfies Check 17 structured commit gate and atomically lands all changes.
- AC-12: Path-limited `git add` discipline confirmed via `git diff --cached --name-status`; zero files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/core/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `ml/` are swept into the commit. Sweep ledger update lands locally only — NOT in the commit.

## Product Principle Alignment

**Principle 4 — Source-Qualified Processing.** §22.7 inventory accuracy is a source-qualification contract: every connector named in the design doc must trace 1:1 to a registered `connector.Connector` implementation under `internal/connector/`. The 15-vs-16 drift quietly breaks that contract by making QF Decisions invisible in the product-truth document. FR-01 + FR-02 restore the contract.

**Principle 8 — Trust Through Transparency.** Every framework-gate assertion this packet makes is independently re-verifiable by re-running the four guards. Strict-provenance restoration in `state.json` + Code Diff Evidence in `report.md` + regression-E2E planning in `scopes.md` + Shared Infrastructure Impact Sweep on Scope 2 + Stress Test Plan row + red→green TDD markers + freshness-substring false-positive cleanup directly satisfy the transparency contract: an auditor can reconstruct the full story of how spec 024 was certified and where each runtime claim is evidenced.

**Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product).** The §22.7 row 16 explicitly carries the boundary text "QF DecisionPacket ingestion as read-only companion (spec 041 — boundary: no financial advice generation)". This preserves the cross-product boundary that spec 041 owns and that this reconciliation must not erode by paraphrasing the connector role.

## Non-Goals

- This packet is **not** introducing or modifying QF Decisions connector code, schema, packet shape, NATS topology, or boundary semantics. Spec 041 owns all of those; this packet only reconciles `docs/smackerel.md` to accurately describe what spec 041 already shipped.
- This packet will not modify any production code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, or compose file.
- This packet will not change spec 024's overall `status` away from `done`.
- This packet will not weaken any `state-transition-guard.sh` or `artifact-freshness-guard.sh` check by altering thresholds, regex patterns, or pair predicates.
- This packet will not touch any in-flight WIP under `specs/055-*` or `specs/044-per-user-bearer-auth/state.json` even though those files appear in `git status` at HEAD `d203d0b9`. Path-limited `git add` enforces this.
- This packet will not update the sweep ledger `.specify/memory/sweep-2026-05-23-r30.json` inside the same commit; the ledger update is a local-only post-commit step (matching the round-21 / round-28 precedent).
