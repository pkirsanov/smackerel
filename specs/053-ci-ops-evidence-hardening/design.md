# Design: CI Ops Evidence Hardening

## Design Brief

Spec 053 consolidates the Smackerel product-owned planning follow-up from BUG-045-002. The predecessor bug fixed the CI integration failure and closed as `done_with_concerns`; this spec does not reopen that outage. It defines an artifact-only planning packet for `TR-BUG-045-002-008` through `TR-BUG-045-002-012` and explicitly excludes framework-owned `TR-BUG-045-002-014`.

The design target is a record-driven plan that lets later Bubbles phases prove, close, or route each carry-forward concern without editing runtime, source, CI workflow, deployment, or framework-managed files.

## Current Truth

- BUG-045-002 verified the canonical CI integration path through `./smackerel.sh test integration`.
- The topology guard and adversarial topology sub-tests exist in product test surfaces and are treated as prior proof.
- The predecessor packet's remaining product concerns are planning concerns, not evidence that CI remains broken.
- `specs/054-artifact-output-summarization` is a reserved later idea and is not created by this feature.
- TR-014 belongs to upstream Bubbles framework maintenance, not this Smackerel product packet.

## Scope Boundary

Allowed files for this feature are limited to `specs/053-ci-ops-evidence-hardening/**` unless a later owner explicitly expands the boundary with evidence and approval. The current mode is `spec-scope-hardening`, so implementation and runtime changes are out of scope.

Excluded surfaces include:

- `.github/workflows/**`
- `.github/bubbles/**`
- `.github/agents/bubbles_shared/**`
- `.github/agents/bubbles.*.agent.md`
- `.github/prompts/bubbles.*.prompt.md`
- `.github/instructions/bubbles-*.instructions.md`
- `.github/skills/bubbles-*/**`
- `cmd/**`
- `internal/**`
- `ml/**`
- `scripts/**`
- `deploy/**`
- `docker-compose*.yml`
- `config/**`

## Design Decisions

### DD-053-001: Consolidated Product Packet

Keep `TR-BUG-045-002-008` through `TR-BUG-045-002-012` in one feature folder. They share the same source bug packet, evidence model, CI workflow context, G040 wrapper context, and framework boundary. Splitting them would duplicate source matrices and increase traceability drift.

### DD-053-002: Proof Before Residual Work

`TR-BUG-045-002-008` must start with current traceability evidence. If traceability shows residual gaps, scope the exact scenario IDs and missing claims. If traceability shows no residual gap, close the item by evidence instead of inventing work.

### DD-053-003: Regression Expansion Must Add Value

`TR-BUG-045-002-009` can only add regression planning that protects a failure mode not already covered by the BUG-045-002 topology guard, adversarial sub-tests, or local reproduction proof. Duplicate rows are rejected and recorded as such.

### DD-053-004: Consumer Trace Is Broader Than Imports

`TR-BUG-045-002-010` inventories any first-party surface that consumes the CI integration path, evidence shape, canonical command, topology guard, or carry-forward decision. Consumers may be direct, indirect, observational, or owner-routed.

### DD-053-005: Shared Infrastructure Needs Canary Boundaries

`TR-BUG-045-002-011` treats test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrapper behavior as protected shared surfaces. Every protected surface needs a canary check, broad validation trigger, restore path, and excluded sibling surfaces before any later source change can be authorized.

### DD-053-006: Boundary Proof Is Required

`TR-BUG-045-002-012` requires every scope to declare allowed file families and excluded surfaces. Closure needs a no-source-delta proof that distinguishes Spec 053 work from unrelated dirty worktree state.

### DD-053-007: G040 Wrappers Are Artifact Lifecycle Records

BUG-045-002 G040 wrappers that contain product planning content need wrapper disposition records. Valid dispositions are `retain-historical`, `retain-with-cross-reference`, `remove-after-capture`, and `owner-routed`. Wrapper disposition does not imply runtime or source work.

### DD-053-008: Framework Work Stays Upstream

TR-014 may be referenced only as an owner-routed framework boundary record. Smackerel must not edit downstream installed Bubbles framework artifacts for this product spec.

## Artifact Model

### Transition Request Matrix

Each TR row records:

| Field | Meaning |
|-------|---------|
| `trId` | Source transition request ID. |
| `sourceArtifact` | BUG-045-002 report/state or Spec 053 artifact. |
| `sourceClaim` | Claim being planned or closed. |
| `scenarioIds` | SCN-053 IDs covered by the row. |
| `requirementIds` | FR-053 IDs covered by the row. |
| `plannedRecordType` | Matrix, inventory, blast-radius, boundary, or wrapper record. |
| `disposition` | Current closure/routing state. |
| `evidenceExpectation` | Command or artifact evidence needed to close. |

### Consumer Inventory

Consumer records include `consumerId`, `pathOrSurface`, `consumerClass`, `consumedSignal`, `staleRisk`, `disposition`, `evidenceRef`, and `owner`.

Allowed consumer classes:

- `direct`
- `indirect`
- `observational`
- `documentation-facing`
- `owner-routed`

Allowed dispositions:

- `required-change`
- `no-change-with-evidence`
- `owner-routed`

### Blast-Radius Record

Blast-radius records include `surfaceId`, `protectedContract`, `dependentSurfaces`, `canaryCheck`, `broadValidationTrigger`, `rollbackOrRestore`, and `evidenceExpectation`.

### Boundary Record

Boundary records include `scopeId`, `allowedFileFamilies`, `excludedSurfaces`, `allowedChangeType`, `noSourceDeltaProof`, and `owner`.

### Wrapper Disposition Record

Wrapper records include `wrapperId`, `sourceArtifact`, `containedClaim`, `mappedTrId`, `disposition`, `owner`, `crossReferenceRequired`, and `evidenceExpectation`.

## Scope Architecture

| Scope | Transition Request | Required Planning Output |
|-------|--------------------|--------------------------|
| Scope 1: G068 Fidelity Proof-Or-Close | `TR-BUG-045-002-008` | TR matrix row and proof-or-close disposition. |
| Scope 2: Regression E2E Expansion Plan | `TR-BUG-045-002-009` | Source-surface matrix and duplicate-aware regression rows. |
| Scope 3: CI Consumer Trace Plan | `TR-BUG-045-002-010` | Consumer inventory and stale-reference scan plan. |
| Scope 4: Shared Infrastructure Blast-Radius Plan | `TR-BUG-045-002-011` | Blast-radius records with canaries and restore expectations. |
| Scope 5: Change Boundary + G040 Wrapper Disposition | `TR-BUG-045-002-012` | Boundary records, wrapper disposition records, framework boundary, and consolidation proof. |

## Validation Strategy

This feature has no runtime tests because it intentionally changes no runtime behavior. Validation is artifact-focused:

| Validation Target | Command Or Evidence | Expected Result |
|-------------------|---------------------|-----------------|
| Artifact structure | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | Exit 0. |
| Scenario-to-DoD fidelity | `bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` | All SCN-053 scenarios map to DoD. |
| Boundary proof | `git status --short --untracked-files=all` plus `git diff --name-status` inspections | Spec 053 work is separated from unrelated dirty state. |
| No framework edit | Read-only git status/diff inspection for framework paths | No downstream framework-managed files changed. |
| No Spec 054 creation | Read-only spec directory inspection | No `specs/054-*` directory exists. |

## Failure Handling

- If traceability has unmapped scenarios, Scope 1 remains incomplete until exact residual rows are authored.
- If regression rows duplicate prior proof, they are rejected rather than scoped as new work.
- If a consumer cannot be classified, Scope 3 remains incomplete.
- If a protected shared surface lacks a canary, Scope 4 remains incomplete.
- If no-source-delta proof includes unrelated dirty files outside the allowed family, Scope 5 remains blocked unless the proof is reframed by an owner with explicit evidence or the unrelated dirtiness is resolved.
- If framework-owned work appears in product scope, it is routed upstream and removed from Smackerel product closure.

## Alternatives Considered

### Amend BUG-045-002 Directly

Rejected. BUG-045-002 is the audited source packet and should remain stable as historical evidence.

### Amend Spec 031 Or Spec 023 Directly

Rejected. Those specs provide precedent but do not own this incident-specific evidence-hardening packet.

### Create Five Separate Specs

Rejected. The five product TRs share a source packet and should stay in one traceability surface.

### Create `specs/054-artifact-output-summarization`

Rejected for this invocation. Spec 054 remains reserved only.

### Patch Framework Files In Smackerel

Rejected. TR-014 is framework-owned and must be handled in the canonical Bubbles repository.

## Completion Statement

This design defines the Spec 053 planning architecture and boundaries. It does not authorize source, runtime, CI, deploy, or framework changes. Current scope status and evidence live in `scopes.md`, `report.md`, `state.json`, and `scenario-manifest.json`.
