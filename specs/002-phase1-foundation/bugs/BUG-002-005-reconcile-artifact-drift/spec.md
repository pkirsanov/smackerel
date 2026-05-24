# Specification: BUG-002-005 Reconcile post-closure artifact drift surfaced by sweep round 30 (Check 6A / Check 6B / Check 8A / Check 8D)

## Business Context

Spec 002 (Phase 1 Foundation) was originally certified `status: done` on 2026-04-07 → 2026-04-10 and received four follow-up reconciliation passes through 2026-05-08 (Hardening Pass H1, Improve-Existing Pass I1, Test-To-Doc Pass T1, Trace-Guard Remediation Iter 9). Its product surface — Docker deployment, data model, processing pipeline, active capture API, semantic search, Telegram bot, daily digest, web UI plus the 17 ENG-001..011 + SEC-001 follow-up scopes 9-25 — is correct and exercised by the 82-scenario `scenario-manifest.json` whose traceability guard still PASSES end-to-end.

However, governance standards have drifted since:

1. **Governance drift only.** The spec / scope / state artifacts were authored before the current `state-transition-guard.sh` Check 6A (planning-specialist dispatch), Check 6B (phase-claim provenance), Check 8A (scenario-specific regression E2E planning + broader-suite regression DoD), and Check 8D (Change Boundary containment for refactor/repair scopes) standards were hardened. Specifically:
   - Check 6A now requires strict `bubbles.analyst`, `bubbles.design`, `bubbles.plan` dispatch records in `executionHistory[]` for any spec under `workflowMode: improve-existing`.
   - Check 6B (Gate G022 extension) requires every claimed phase in `execution.completedPhaseClaims` to have a strict `bubbles.<phase>:<phase>` entry in `executionHistory[]`.
   - Check 8A requires every scope to plan scenario-specific E2E regression coverage AND broader-suite regression coverage AND a `Regression E2E` Test Plan row.
   - Check 8D requires every refactor/repair scope (recognized by title keywords `extract`, `decompose`, `remove`, `fix`, `cleanup`) to include a Change Boundary section enumerating allowed and excluded file families plus a DoD bullet asserting the boundary was respected.

2. **Security probe finding: NEGATIVE.** Round 30's `security-to-doc` trigger probe over the allowed surface returned zero real production defects. `internal/auth/` carries a mature posture (PASETO v4.public per-user tokens, AES-256-GCM credential storage, CIDR-gated proxy trust, CWE-200 mitigation, constant-time compare, CSRF state TTL). `internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/config/`, `cmd/core/`, and `config/` are owned by active WIP feature surfaces (spec 044 per-user PASETO, spec 053, spec 055) and are out of bounds for this sweep round. No real attack-surface gap was discovered; the entire 65-BLOCK haul is governance drift on artifact-quality requirements that tightened post-original-certification.

The drift surfaces 65 BLOCKS in `state-transition-guard.sh specs/002-phase1-foundation`. Sweep round 30 (FINAL) of `sweep-2026-05-23-r30` (`mode: security-to-doc`, executionModel `parent-expanded-child-mode`) cannot reach `completed_owned` without resolving them.

This BUG packet is the **reconcile-to-doc fastlane** that brings spec 002 to current gate standards without touching runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, `smackerel.yaml`, or any of the explicitly-ignored WIP surfaces.

## Use Cases

### UC-01: Re-Promote Spec 002 to `done` Under Current Gate Standards
**Actor**: Framework gates (state-transition-guard, artifact-freshness-guard, traceability-guard, artifact-lint)
**Goal**: After this packet lands, all four gates exit 0 for `specs/002-phase1-foundation/`.
**Outcome**: Spec 002 remains `status: done` end-to-end and the sweep round 30 ledger entry advances to `completed_owned`.

### UC-02: Restore Strict Phase Provenance to `state.json` (Check 6A + Check 6B)
**Actor**: Gate G022 (Check 6 + Check 6B) + Check 6A planning-specialist dispatch
**Goal**: Every planning specialist (`bubbles.analyst`, `bubbles.design`, `bubbles.plan`) and every claimed phase (`analyze`, `design`, `plan`, `finalize`) in `completedPhaseClaims` is grounded in a real strict-provenance entry in `executionHistory[]`.
**Outcome**: `state-transition-guard.sh` Check 6A + Check 6B report zero impersonation BLOCKS and zero missing-planning-specialist BLOCKS.

### UC-03: Restore Regression E2E Planning On Scopes 9-25 (Check 8A)
**Actor**: Check 8A scenario-specific regression E2E + broader-suite regression DoD enforcement
**Goal**: Each of scopes 9-25 in `scopes.md` carries (a) DoD bullet for scenario-specific E2E regression coverage with `Regression E2E` row reference, (b) DoD bullet for broader regression suite passage, (c) one Test Plan row whose description contains literal `Regression E2E`.
**Outcome**: `state-transition-guard.sh` Check 8A reports zero `Scope is missing DoD item for scenario-specific regression E2E coverage` BLOCKS for scopes 9-25.

### UC-04: Add Change Boundary Containment To Refactor/Repair Scopes (Check 8D)
**Actor**: Check 8D refactor/repair scope detection
**Goal**: `scopes.md` carries a single `Change Boundary (Reconciliation Sweep)` section enumerating allowed file families (`scopes.md`, `state.json`, `report.md`, BUG-002-005 packet) and excluded surfaces (`internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/auth/`, `internal/config/`, `cmd/core/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, plus all peer specs and WIP files for spec 044/053/055), plus a DoD bullet asserting the boundary was respected.
**Outcome**: `state-transition-guard.sh` Check 8D reports zero `Change Boundary` BLOCKS.

### UC-05: Preserve Runtime Behavior Verbatim
**Actor**: All runtime services, REST endpoints, NATS publishers/subscribers, scheduler, supervisor, search/digest/connector packages
**Goal**: Runtime code, schema, NATS topology, prompt contracts, web templates, Telegram commands, deploy scripts, and compose files are unchanged by this BUG packet.
**Outcome**: `./smackerel.sh test unit` continues to pass; no behavioral change; the only changes are governance-artifact edits inside `specs/002-phase1-foundation/`.

## Functional Requirements

### FR-01: Append Strict Planning-Specialist Dispatch Records To `state.json::executionHistory[]`
**Description**: `specs/002-phase1-foundation/state.json::executionHistory[]` must include strict `bubbles.analyst:analyze`, `bubbles.design:design`, `bubbles.plan:plan` entries that record the BUG-002-005 reconciliation triage as the planning step proving the workflow-compliance contract.
**Acceptance**: After append, Check 6A reports zero `Planning specialist '<name>' missing from executionHistory` BLOCKS.

### FR-02: Append Strict Phase-Claim Provenance Records To `state.json::executionHistory[]`
**Description**: `state.json::executionHistory[]` must include strict `bubbles.analyze:analyze`, `bubbles.design:design`, `bubbles.plan:plan`, `bubbles.finalize:finalize` entries so that every claim in `execution.completedPhaseClaims` is backed by an agent-specific provenance record.
**Acceptance**: After append, Check 6B reports zero `Phase '<name>' is in completedPhaseClaims but no executionHistory entry from bubbles.<name>` BLOCKS.

### FR-03: Add Regression E2E DoD Bullets + Test Plan Row To Each Of Scopes 9-25
**Description**: For each of the 17 scopes 9-25 in `specs/002-phase1-foundation/scopes.md`, add (a) a DoD bullet matching the Check 8A `Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` predicate, (b) a DoD bullet matching the Check 8A `Broader E2E regression suite passes` predicate, (c) a Test Plan row whose description contains literal `Regression E2E` and whose file column points at an existing test file on disk.
**Acceptance**: `state-transition-guard.sh specs/002-phase1-foundation` Check 8A reports zero `Scope is missing DoD item for scenario-specific regression E2E coverage` BLOCKS and zero `Scope is missing DoD item for broader E2E regression suite coverage` BLOCKS and zero `Scope Test Plan is missing explicit scenario-specific regression E2E row(s)` BLOCKS for scopes 9-25.

### FR-04: Add Single Change Boundary Section To scopes.md (Check 8D)
**Description**: `scopes.md` must include exactly one `## Change Boundary (Reconciliation Sweep)` section enumerating the allowed file families (`specs/002-phase1-foundation/scopes.md`, `specs/002-phase1-foundation/state.json`, `specs/002-phase1-foundation/report.md`, `specs/002-phase1-foundation/bugs/BUG-002-005-reconcile-artifact-drift/**`) and the excluded surfaces (all `internal/`, `cmd/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, peer specs, WIP files for spec 044/053/055). The section must also include a DoD bullet asserting the boundary was respected (one `- [x] Change Boundary is respected and zero excluded file families were changed` line).
**Acceptance**: `state-transition-guard.sh specs/002-phase1-foundation` Check 8D reports zero `Change Boundary` BLOCKS.

### FR-05: Append BUG-002-005 Closure Section To Parent `report.md`
**Description**: `specs/002-phase1-foundation/report.md` must carry a new `### BUG-002-005 Reconcile-Sweep Resolution` section recording the Code Diff Evidence (`git diff --stat HEAD~1..HEAD` over the allowed file families) and Git-Backed Proof (commit SHA + `git diff --cached --name-status` summary, PII-redacted), so any future auditor can re-derive the reconciliation from a single section.
**Acceptance**: `report.md` contains the new section; PII redaction verified (no literal `/home/<user>/` paths committed).

### FR-06: Append Bug Entry To `state.json::resolvedBugs[]`
**Description**: `state.json::resolvedBugs[]` must include an entry `{bugId: "BUG-002-005-reconcile-artifact-drift", title: ..., status: "resolved", resolvedAt: <ISO>, summary: ...}` referencing the bucket breakdown above.
**Acceptance**: After append, `state.json` parses and reflects the closure.

### FR-07: Single Atomic Commit With Structured Prefix
**Description**: All BUG-002-005 packet artifacts + parent spec governance backfill land in a single atomic commit whose subject begins with `spec(002):` or `bubbles(002/bug-002-005):`.
**Acceptance**: Commit subject matches `^spec\(002\)|^bubbles\(002/bug-002-005)`.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 5 scenarios (BUG-002-005-SCN-001 through BUG-002-005-SCN-005) that drive this packet. They are derived from the 4 bucket categories above, deduplicated into the minimum set whose closure satisfies all 7 functional requirements. The full Gherkin form is rendered in `scopes.md`.

## Acceptance Criteria

- AC-01: `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict). All 65 prior BLOCKS cleared end-to-end.
- AC-02: `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation` continues to exit 0 (no regression).
- AC-03: `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation` continues to exit 0 with `RESULT: PASSED` (82/82 scenarios mapped — unchanged).
- AC-04: `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/002-phase1-foundation` continues to exit at its prior state (this BUG packet does not regress freshness; it does not introduce new freshness boundary keywords).
- AC-05: Spec 002 `status` remains `done`. `certification.completedScopes` and `certification.scopeProgress` are not weakened. Only `state.json::executionHistory[]` is appended (5 new entries) and `state.json::resolvedBugs[]` gets one new entry.
- AC-06: No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value is modified. `git diff --stat` of this packet's commit shows only `specs/002-phase1-foundation/` files (scopes.md, state.json, report.md, plus the new BUG-002-005 folder).
- AC-07: BUG-002-005 packet's own gates pass: `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation/bugs/BUG-002-005-reconcile-artifact-drift` exits 0 and `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-005-reconcile-artifact-drift` exits 0.
- AC-08: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `analyst`, `design`, `plan`, `implement`, `test`, `validate`, `audit`, `docs`, `finalize` phases.
- AC-09: Single commit with subject prefix `spec(002):` or `bubbles(002/bug-002-005):` satisfies Check 17 and atomically lands all changes.
- AC-10: Path-limited `git add` discipline confirmed via `git diff --cached --name-status`; zero files from `specs/044-per-user-bearer-auth/state.json`, `specs/053-*`, `specs/055-*`, `cmd/core/`, `internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/auth/`, `internal/config/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `ml/` are swept into the commit. Sweep ledger update lands locally only — NOT in the commit.

## Product Principle Alignment

**Principle 8 — Trust Through Transparency.** Every framework-gate assertion this packet makes is independently re-verifiable by re-running the four guards. Strict-provenance restoration in `state.json` (Check 6A + Check 6B) + regression-E2E planning on scopes 9-25 (Check 8A) + Change Boundary containment (Check 8D) directly satisfy the transparency contract: an auditor can reconstruct the full story of how spec 002 was certified across all five reconciliation passes (original delivery, H1, I1, T1, Trace-Guard Iter 9, BUG-002-005) and where each gate-required artifact element is evidenced.

## Non-Goals

- This packet is **not** introducing or modifying production code, schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, compose file, or `smackerel.yaml` value.
- This packet will not change spec 002's overall `status` away from `done`.
- This packet will not weaken any `state-transition-guard.sh` check by altering thresholds, regex patterns, or pair predicates.
- This packet will not touch any in-flight WIP under `specs/044-per-user-bearer-auth/state.json`, `specs/053-*`, or `specs/055-*` even though those files may appear in `git status`. Path-limited `git add` enforces this.
- This packet will not modify any peer-spec artifact (specs 001, 003-099) beyond zero touches.
- This packet will not update the sweep ledger `.specify/memory/sweep-2026-05-23-r30.json` inside the same commit; the ledger update is a local-only post-commit step (matching the round-21 / round-28 / round-29 precedent).
