<!-- Inactive duplicate content retained only because the patch delete operation did not physically remove prior duplicated text. Active design begins at the final "# Design: CI Ops Evidence Hardening" heading below.

## Design Brief

### Current State

BUG-045-002 is closed as `done_with_concerns`, not because the CI integration fix is unproven, but because five product-owned planning concerns remain routed from the bug packet into a Smackerel planning packet. The predecessor evidence shows the original chronic CI failure was fixed by routing the GitHub Actions integration job through the canonical `./smackerel.sh test integration` path, adding a topology contract guard, proving local full-stack reproduction, and observing green CI runs on main.

The remaining product transition requests are TR-BUG-045-002-008 through TR-BUG-045-002-012. TR-BUG-045-002-014 is framework-owned and remains outside this Smackerel product packet except as a boundary note.

### Target State

This feature becomes the single product-side planning contract for those five carry-forward concerns. It defines how to prove or close the G068 fidelity item, how to plan regression expansion without duplicating already-green BUG-045-002 proof, how to inventory consumers, how to bound shared-infrastructure blast radius, and how to decide G040 wrapper disposition.

The target is not implementation. The target is a traceable evidence architecture that lets `bubbles.plan`, `bubbles.harden`, `bubbles.validate`, and `bubbles.audit` act without inventing gaps, mutating runtime surfaces, or pulling framework work into Smackerel.

### Patterns To Follow

- Keep the consolidated feature folder at `specs/053-ci-ops-evidence-hardening` as the current product planning surface.
- Preserve the BUG-045-002 trace-ID anchor approach for G068: literal scenario IDs in DoD mappings are the durable link pattern.
- Use the predecessor bug packet as source evidence: `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md` and `state.json`.
- Treat Bubbles evidence provenance tags exactly as defined by `evidence-rules.md`: `executed`, `interpreted`, and `not-run`.
- Treat shared-infrastructure changes as protected surfaces requiring canary contracts before broad validation.

### Patterns To Avoid

- Do not reopen the original BUG-045-002 root-cause design unless new evidence proves the CI fix regressed.
- Do not create `specs/054-artifact-output-summarization` in this packet; that identifier remains a reserved related idea.
- Do not amend framework-managed files under `.github/bubbles/`, `.github/agents/bubbles_shared/`, `.github/instructions/bubbles-*`, or `.github/skills/bubbles-*` from this product repo.
- Do not rewrite predecessor Gherkin or DoD text just to satisfy a string-matching gate; use source-grounded trace IDs and owner-scoped planning.
- Do not plan broad source changes before a boundary record names the allowed file families and excluded surfaces.

### Resolved Decisions

- TR-008 through TR-012 stay in one consolidated spec because their evidence, consumers, blast-radius, and wrapper decisions share the same predecessor packet.
- TR-008 is proof-gated: current traceability evidence comes before any new G068 work is planned.
- Regression expansion must add new protection beyond the existing topology guard, adversarial contract tests, and local full-stack reproduction.
- Consumer trace, blast-radius, boundary, and wrapper decisions are artifact records first, not runtime changes.
- TR-014 remains framework-owned.

### Open Questions

- None block design completion. The current traceability result for TR-008 must be produced by the owning planning or validation phase before closure decisions are made.

## Purpose And Scope

This design defines the artifact architecture for an ops evidence-hardening packet. The packet exists to turn BUG-045-002 carry-forward concerns into explicit planning records, scope boundaries, and verification expectations without editing runtime code, CI workflow code, deploy adapters, framework install artifacts, or test source in the design phase.

The design covers:

- TR-BUG-045-002-008: G068 proof-or-close fidelity work.
- TR-BUG-045-002-009: regression E2E expansion planning.
- TR-BUG-045-002-010: CI workflow consumer trace planning.
- TR-BUG-045-002-011: shared-infrastructure blast-radius planning.
- TR-BUG-045-002-012: change boundary and G040 wrapper disposition planning.

The design excludes:

- TR-BUG-045-002-014 framework guard maintenance.
- Runtime/source implementation.
- CI workflow refactoring.
- New framework script, agent, instruction, or skill edits in the Smackerel repo.
- Creation of `specs/054-artifact-output-summarization`.

## Current Truth

| Source | Current Truth | Design Consequence |
|--------|---------------|--------------------|
| BUG-045-002 `state.json` top-level status | The predecessor packet is `done_with_concerns`. | This packet preserves the prior close-out while resolving the product-owned concerns separately. |
| BUG-045-002 `transitionRequests[]` | TR-008..012 are open, owned by `bubbles.plan`, and point to G068, regression expansion, consumer trace, blast radius, and boundary/wrapper planning. | The design maps each product TR to a specific record type and scope architecture. |
| BUG-045-002 `certification.concerns[]` | Six low-severity concerns were recorded; five are product planning concerns, one is framework guard maintenance. | Product work covers only the five Smackerel concerns and records TR-014 as a framework boundary. |
| BUG-045-002 report Evidence 1-5b | The chronic CI failure came from local-vs-CI topology drift. | The design does not re-litigate root cause; it hardens evidence around the already-fixed path. |
| BUG-045-002 validation and audit evidence | Path A, topology guard, adversarial sub-tests, local full-stack reproduction, and green CI evidence were already recorded. | Regression expansion must avoid duplicate proof and name only additional protective scenarios. |
| BUG-045-002 G068 history | Traceability failed with 11 unmapped scenarios, then the plan re-entry resolved the packet through trace-ID anchors and guard output later showed 11/11 mapped at audit entry. | TR-008 starts with a current proof step. No residual gap may be invented from stale pre-anchor output. |
| BUG-045-002 G040 wrappers | Report sections preserved routed planning content inside skip regions. | Wrapper disposition must be explicit: retain as history, retain with cross-reference, or remove only by owning artifact phase after coverage is proven. |
| Smackerel Bubbles governance | Framework-managed files are upstream-owned and immutable in downstream repos. | TR-014 is not product design scope; product artifacts may only record the boundary and route. |

## Architecture Overview

The feature is an artifact-only control plane. Its active objects are planning records, not runtime services.

```text
BUG-045-002 report/state
        |
        v
053 spec.md requirements and scenarios
        |
        v
053 design.md record model and boundaries
        |
        v
053 scopes.md planned by bubbles.plan
        |
        v
harden/docs/validate/audit evidence produced by owning phases
```

The architecture has four layers:

| Layer | Responsibility | Owned By |
|-------|----------------|----------|
| Source-truth layer | Predecessor BUG-045-002 report/state and current 053 spec. | `bubbles.analyst`, predecessor packet owners |
| Design layer | Record model, boundaries, phase ownership, verification strategy. | `bubbles.design` |
| Planning layer | Five scopes, Gherkin-to-test mapping, DoD, evidence slots. | `bubbles.plan` |
| Certification layer | Artifact lint, traceability guard, no-source-delta proof, audit interpretation. | `bubbles.validate`, `bubbles.audit` |

No runtime component is introduced. No database table, API endpoint, service route, Docker resource, UI surface, or deployment target changes.

## Design Decisions

### DD-053-001: One Consolidated Spec

TR-008 through TR-012 stay in `specs/053-ci-ops-evidence-hardening` because all five requests depend on the same predecessor packet and the same evidence boundary. Splitting them would create duplicate source-surface mappings and increase the chance that G040 wrapper disposition, consumer trace, and blast-radius decisions drift apart.

`specs/054-artifact-output-summarization` is not created here. If that idea is pursued, it must begin as a separate owner-approved feature with its own source evidence.

### DD-053-002: Proof-Before-Work For G068

TR-008 must begin with a current traceability result against the relevant artifacts. The first valid outcome is not automatically new scope; it is one of:

| Outcome | Meaning | Required Record |
|---------|---------|-----------------|
| `residual-gap-found` | Current traceability evidence shows a specific unmapped or weakly mapped scenario. | G068 residual-gap record with scenario ID, missing claim, owning artifact, and required evidence. |
| `closed-by-current-proof` | Current traceability evidence shows the scenario set is fully mapped. | Closure-by-evidence record citing command output and predecessor source surface. |
| `owner-routed-tool-issue` | The traceability tool produces a framework-owned false positive. | Framework-boundary route record; no Smackerel source work. |

This prevents stale pre-anchor evidence from becoming invented product work.

### DD-053-003: Regression Expansion Boundaries

TR-009 expansion has three allowed surfaces:

| Surface | Allowed Planning Question | Excluded Duplicate |
|---------|---------------------------|--------------------|
| CI integration job | What scenario proves the job still routes through canonical integration behavior as an observable CI contract? | Repeating only that BUG-045-002 already had a green CI run. |
| Full-stack reproduction path | What live-stack scenario proves the predecessor failure class remains covered outside GitHub Actions? | Repeating the exact BUG-045-002 local reproduction without a new protective claim. |
| Contract-test surface | What contract or adversarial check protects against topology drift or evidence-shape drift? | Re-listing the existing topology guard and three adversarial tests without a new gap. |

Every planned regression row must name the new failure mode it protects. If no new protective claim exists, the row is not added.

### DD-053-004: Consumer Trace Model

Consumer trace is planned before any evidence-shape change. A consumer is any first-party artifact that reads, references, asserts, documents, or operationally depends on the CI integration job, its logs, its command shape, or its evidence output.

Consumer classes:

| Class | Definition | Examples |
|-------|------------|----------|
| `direct` | Reads or executes the CI integration workflow or canonical command. | CI workflow, repo CLI, integration test runner. |
| `indirect` | Validates or parses the workflow/evidence shape. | Contract tests, traceability records, artifact lint references. |
| `observational` | Uses CI results as evidence without executing the job. | Report sections, validation records, certification concerns. |
| `documentation-facing` | Describes the user-facing command or operational evidence surface. | README, Testing, Operations, agents memory. |

Every consumer receives exactly one disposition: `required-change`, `no-change-with-evidence`, or `owner-routed`.

### DD-053-005: Shared Infrastructure Blast-Radius Model

Shared infrastructure surfaces are protected. A protected surface is any artifact whose change could affect multiple tests, workflows, evidence gates, or operator commands.

Protected surfaces for this packet:

| Surface | Protected Contract | Canary Requirement |
|---------|--------------------|--------------------|
| Test stack lifecycle | Test stack starts, reports health, runs integration suites, and tears down through `./smackerel.sh`. | A narrow command/evidence canary that proves the lifecycle contract before broad suite reliance. |
| CI workflow ordering | CI uses the canonical command and preserves upload/failure-step semantics. | Structural guard or workflow-evidence canary. |
| Contract-test parsing | Tests parse the intended workflow and reject known-bad topologies. | Adversarial guard cases for each rejected pattern. |
| CLI wrappers | User-facing command remains `./smackerel.sh test integration`; wrapper helpers remain internal. | CLI command evidence or no-change-with-evidence disposition. |

Broad validation may be planned only after the canary for the changed protected surface is named.

### DD-053-006: Change Boundary Model

Each scope must carry a boundary record with:

- Allowed artifact families.
- Excluded runtime/source/framework surfaces.
- Allowed change type.
- Required no-source-delta proof.
- Owner responsible for resolving boundary drift.

The default allowed surfaces for this feature are product planning artifacts under `specs/053-ci-ops-evidence-hardening`. Runtime and source paths are observational unless a later owner creates a separate implementation scope with explicit user approval and governance coverage.

### DD-053-007: G040 Wrapper Disposition Model

G040 wrapper content in the predecessor report is historical evidence until an owning artifact phase records a disposition. Valid dispositions:

| Disposition | Meaning | Required Proof |
|-------------|---------|----------------|
| `historical-retain` | The wrapper remains because it preserves predecessor audit history. | Current 053 records cite and map the content. |
| `cross-reference-retain` | The wrapper remains and receives or is paired with a reference to the 053 record that now owns the concern. | Source-surface mapping plus no-source-delta proof for excluded files. |
| `owner-remove` | The owning artifact phase removes wrapper content because all contained planning claims are represented in 053 and audit accepts no evidence loss. | Wrapper disposition record, mapped scope, and audit confirmation. |

Design does not remove or edit predecessor wrappers. It defines the model that later owners must use.

### DD-053-008: Framework Boundary

TR-014 remains outside Smackerel product scope. Product artifacts may mention it only to preserve routing truth and to avoid accidental edits to framework-managed install artifacts. Any framework guard repair belongs in the canonical Bubbles repository first, then downstream Smackerel receives it through the standard upgrade path.

## Data And Artifact Model

### TR Matrix

The TR matrix is the root index for the packet.

| Field | Required | Description |
|-------|----------|-------------|
| `trId` | Yes | Transition request ID, e.g. `TR-BUG-045-002-008`. |
| `sourceArtifact` | Yes | Predecessor artifact path and section or state field. |
| `sourceClaim` | Yes | The exact source-grounded concern. |
| `scenarioIds` | Yes | Current 053 scenarios that cover the TR. |
| `requirementIds` | Yes | Current 053 FRs that cover the TR. |
| `plannedRecordType` | Yes | One of G068, regression, consumer, blast-radius, boundary, wrapper. |
| `disposition` | Yes | `planned`, `closed-by-current-proof`, `owner-routed`, or `excluded-framework`. |
| `evidenceExpectation` | Yes | Command/tool/artifact evidence expected by the owning phase. |

Initial TR matrix:

| TR | Planned Record Type | Required First Disposition |
|----|---------------------|----------------------------|
| TR-BUG-045-002-008 | G068 proof-or-close | Current traceability proof before scope creation. |
| TR-BUG-045-002-009 | Regression expansion | New protective scenario or no-addition rationale. |
| TR-BUG-045-002-010 | Consumer inventory | Every consumer classified and dispositioned. |
| TR-BUG-045-002-011 | Blast-radius | Protected contracts and canaries named. |
| TR-BUG-045-002-012 | Boundary and wrapper | Allowed/excluded surfaces and wrapper disposition named. |
| TR-BUG-045-002-014 | Framework boundary | Excluded from product scope. |

### Source-Surface Matrix

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Stable local ID, e.g. `SRC-053-BUG-REPORT-G040`. |
| `path` | Yes | Artifact or source path. |
| `surfaceKind` | Yes | `source-truth`, `observed`, `protected`, `excluded`, or `framework-owned`. |
| `relationship` | Yes | How the surface relates to a TR or scenario. |
| `allowedAction` | Yes | `cite-only`, `plan-record`, `artifact-edit-by-owner`, `no-edit`, or `framework-route`. |
| `proofRequired` | Yes | Evidence needed to prove the action stayed within boundary. |

Required source surfaces:

| Surface | Kind | Allowed Action |
|---------|------|----------------|
| BUG-045-002 `report.md` | `source-truth` | `cite-only` unless a later owner records wrapper disposition. |
| BUG-045-002 `state.json` | `source-truth` | `cite-only` for TR and concern truth. |
| `specs/053-ci-ops-evidence-hardening/spec.md` | `source-truth` | `plan-record` source for design and scopes. |
| `specs/053-ci-ops-evidence-hardening/design.md` | `source-truth` | Current design record. |
| `.github/workflows/ci.yml` | `observed` / `protected` | No design-phase edits; may be cited as observed subject. |
| `internal/deploy/*` contract tests | `observed` / `protected` | No design-phase edits; may be cited as prior proof. |
| `.github/bubbles/*` and `.github/agents/bubbles_shared/*` | `framework-owned` | Product no-edit; route to framework owner. |

### Evidence Provenance Categories

This design uses the Bubbles canonical provenance taxonomy and adds planning overlays.

| Category | Meaning | Allowed Completion Use |
|----------|---------|------------------------|
| `executed` | Current-session command output directly proves the claim. | Can support checked DoD when owned by the phase. |
| `interpreted` | Evidence exists, but the conclusion requires explanation. | Must include an interpretation and remains audit-sensitive. |
| `not-run` | No command was executed for the claim. | Cannot close DoD; must remain uncertain. |
| `predecessor-source` | Historical source from BUG-045-002 report/state. | Can ground planning claims, not current execution claims. |
| `owner-routed` | Evidence or fix belongs to another owner. | Must name owner and boundary; cannot close product work as executed. |
| `closure-by-proof` | Current executed evidence proves no residual work exists. | Valid for TR-008 only when traceability output supports it. |

### Consumer Inventory Record

| Field | Required | Description |
|-------|----------|-------------|
| `consumerId` | Yes | Stable ID, e.g. `CON-053-CI-WORKFLOW`. |
| `pathOrSurface` | Yes | Path, artifact, command, or docs surface. |
| `consumerClass` | Yes | `direct`, `indirect`, `observational`, or `documentation-facing`. |
| `consumedSignal` | Yes | CI job name, command string, evidence section, path, or status field consumed. |
| `staleRisk` | Yes | What could break if the consumed signal changes. |
| `disposition` | Yes | `required-change`, `no-change-with-evidence`, or `owner-routed`. |
| `evidenceRef` | Yes | Artifact, command, or scan that supports the disposition. |
| `owner` | Yes | Phase or repo owner responsible for the disposition. |

### Blast-Radius Record

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Protected surface ID. |
| `protectedContract` | Yes | Contract that must remain true. |
| `dependentSurfaces` | Yes | Consumers or workflows that depend on it. |
| `canaryCheck` | Yes | Narrow validation that proves the protected contract. |
| `broadValidationTrigger` | Yes | When broad validation is justified. |
| `rollbackOrRestore` | Yes | How the artifact returns to prior state if the canary fails. |
| `evidenceExpectation` | Yes | Raw output or artifact proof expected. |

### Boundary Record

| Field | Required | Description |
|-------|----------|-------------|
| `scopeId` | Yes | Scope planned by `bubbles.plan`. |
| `allowedFileFamilies` | Yes | Paths allowed for that scope. |
| `excludedSurfaces` | Yes | Paths explicitly protected from edits. |
| `allowedChangeType` | Yes | `artifact-only`, `observational-scan`, `docs-cross-reference`, or `source-change-by-approved-scope`. |
| `noSourceDeltaProof` | Yes | Git status/diff evidence proving excluded surfaces did not change. |
| `owner` | Yes | Phase owner responsible for maintaining boundary. |

### Wrapper Disposition Record

| Field | Required | Description |
|-------|----------|-------------|
| `wrapperId` | Yes | Stable ID for each G040 region. |
| `predecessorLocation` | Yes | BUG-045-002 report section or line anchor description. |
| `containedClaim` | Yes | Planning concern preserved inside the wrapper. |
| `mappedTrId` | Yes | TR-008, TR-010, TR-011, or TR-012 as applicable. |
| `mappedScopeId` | Yes | Scope planned in 053. |
| `disposition` | Yes | `historical-retain`, `cross-reference-retain`, or `owner-remove`. |
| `crossReferenceRequired` | Yes | Whether the predecessor artifact needs an explicit 053 pointer. |
| `evidenceExpectation` | Yes | Proof that no evidence was lost or misrouted. |

## Workflow And Phase Ownership

| Phase / Agent | Responsibility In This Packet | Must Not Do |
|---------------|-------------------------------|-------------|
| `bubbles.analyst` | Owns `spec.md`, source-grounded requirements, actors, use cases, scenarios, FRs, product principle alignment. | Must not author `design.md` or `scopes.md`. |
| `bubbles.design` | Owns this `design.md`, record model, boundaries, phase ownership, verification strategy, alternatives, risks. | Must not implement source changes, create scopes, or certify completion. |
| `bubbles.plan` | Owns five-scope architecture in `scopes.md`, Test Plan rows, DoD, scenario-to-test mapping, and uncertainty declarations. | Must not claim executed evidence without running commands. |
| `bubbles.harden` | May diagnose planning-hardening gaps after scopes exist and route foreign-owned artifact changes to the owner. | Must not directly rewrite `scopes.md` planning content or source code. |
| `bubbles.docs` | May update managed docs only if the plan proves user-facing documentation changed. | Must not document internal-only topology as a user-facing contract. |
| `bubbles.validate` | Runs artifact lint, traceability guard where applicable, source-surface mapping checks, and no-source-delta proof. | Must not set final `done` state without satisfying ownership and evidence gates. |
| `bubbles.audit` | Independently reviews evidence provenance, no-invented-gap decisions, consumer trace completeness, and boundary compliance. | Must not fill missing planning artifacts directly. |
| `bubbles.workflow` / finalize | Records final route or outcome after owner phases are complete. | Must not route TR-014 into Smackerel product implementation. |

## Scope Architecture

The scope architecture is intentionally five scopes, aligned to the grill recommendation to keep TR-008 through TR-012 together while still isolating evidence surfaces.

| Scope | Primary TR | Design Intent | Key Records |
|-------|------------|---------------|-------------|
| Scope 1: G068 Proof-Or-Close | TR-008 | Execute or reference current traceability evidence and decide whether residual G068 work exists. | TR matrix rows, G068 residual-gap or closure-by-proof records, evidence provenance categories. |
| Scope 2: Regression Expansion Boundaries | TR-009 | Define only regression E2E or contract rows that add protection beyond BUG-045-002 proof. | Regression surface records, source-surface matrix, evidence expectation records. |
| Scope 3: Consumer Trace Inventory | TR-010 | Inventory direct, indirect, observational, and documentation-facing consumers of CI integration evidence. | Consumer inventory records and dispositions. |
| Scope 4: Shared Infrastructure Blast Radius | TR-011 | Identify protected contracts for test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrappers. | Blast-radius records, canary checks, broad-validation triggers. |
| Scope 5: Boundary And Wrapper Disposition | TR-012 plus TR-014 boundary | Define allowed/excluded surfaces, no-source-delta proof, wrapper disposition, and framework boundary. | Boundary records, wrapper disposition records, framework-boundary record. |

Scope ordering should preserve dependency flow: Scope 1 informs whether G068 work exists; Scope 2 uses that truth to avoid duplicate regression rows; Scope 3 identifies consumers before any evidence-shape decision; Scope 4 protects shared contracts; Scope 5 records final containment and wrapper disposition across the prior four scopes.

## API, UI, And Runtime Contracts

This design introduces no runtime APIs, no protobuf schemas, no database tables, no UI screens, and no service routes. The only contracts are artifact contracts:

| Contract | Producer | Consumer | Compatibility Rule |
|----------|----------|----------|--------------------|
| TR matrix | `bubbles.plan` | validate/audit/workflow | Every TR-008..012 has one current disposition. |
| Source-surface matrix | `bubbles.plan` | all later owner phases | Every claim references a named source surface. |
| Evidence provenance | all owner phases | validate/audit | Every evidence block uses `Claim Source`. |
| Consumer inventory | `bubbles.plan` | harden/validate/audit | Every consumer receives a disposition. |
| Blast-radius record | `bubbles.plan` | harden/validate/audit | Protected shared surfaces have canaries before broad validation. |
| Boundary record | `bubbles.plan` | all later owner phases | File changes stay inside the allowed family. |
| Wrapper disposition | `bubbles.plan` | docs/validate/audit | G040 wrappers are retained, cross-referenced, removed by owner, or routed. |

## Security And Compliance

Spec 053 is artifact-only and introduces no new data handling path. Security and compliance controls are governance controls:

- No secret values belong in the design or later planning artifacts.
- No environment-specific hostnames, real IPs, or operator-private topology should be added.
- Evidence captured later should avoid tokens and should use repo-relative paths where possible.
- No workflow, hook, or validation bypass is authorized by this packet.
- Framework-managed files must not be edited in the downstream Smackerel repo.

## Configuration And Migrations

No configuration values, generated env files, Docker Compose files, migrations, or deployment manifests change under this design.

If a later scope proves that a configuration or runtime file must change, that scope must expand its boundary explicitly, record the source evidence that requires the change, and route implementation to the correct owner. Until that happens, spec 053 is constrained to planning artifacts.

## Observability And Failure Handling

The feature observes planning and CI-evidence health rather than runtime health. Failure handling is expressed as dispositions:

| Failure Mode | Detection | Handling |
|--------------|-----------|----------|
| Residual G068 gap exists. | Traceability guard names unmapped scenario(s). | Scope exact gap rows with scenario and DoD/test closure expectation. |
| No residual G068 gap exists. | Traceability guard passes current scenario set. | Close TR-008 by evidence without creating artificial work. |
| Regression row duplicates existing proof. | Row maps only to already-passing BUG-045-002 guard/repro evidence. | Reject row from active scope. |
| Consumer cannot be classified. | Consumer inventory lacks class or disposition. | Keep scope incomplete until classified. |
| Protected infrastructure lacks canary. | Blast-radius record missing canary. | Do not authorize source changes. |
| Excluded surface changes. | No-source-delta proof shows source/runtime/framework file changes. | Block closure and route to owner for boundary correction. |
| Framework-owned issue appears in product scope. | TR-014 or framework files appear as product work. | Remove from product scope and route to upstream Bubbles. |

## Testing And Validation Strategy

This design does not define runtime tests because no runtime behavior changes. It defines validation requirements for the planning packet.

| Validation Target | Command Or Evidence Type | Owner | Expected Result |
|-------------------|--------------------------|-------|-----------------|
| Artifact structure | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | validate/audit | Exits 0 after required owner artifacts exist. |
| G068 mapping | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` when scopes exist | plan/validate | Exits 0 or records exact residual gaps. |
| Source-surface mapping | Read-only scan of planned source surfaces and references | plan/validate | Every matrix row references an existing artifact or owner-routed framework surface. |
| No-source-delta proof | `git diff --name-status` from the Smackerel repo root | validate/audit | Changed files are limited to allowed spec 053 artifacts for planning-only phases. |
| Consumer trace | Stale-reference scans selected by `bubbles.plan` | validate/audit | Every consumer has a disposition and no stale reference remains unaccounted. |
| Blast-radius canaries | Scope-defined canary commands | validate/audit | Canary passes before broad validation is accepted. |
| Boundary and wrapper disposition | Boundary table plus wrapper disposition table | audit | Every wrapper and file family has an owner and closure rule. |

Evidence expectations:

- Executed validations require raw terminal output in the owning evidence artifact.
- Interpreted design conclusions must include an interpretation line when recorded as evidence.
- `not-run` evidence cannot close a checked DoD item.
- Read-only file inspection is useful for planning, but it is not a substitute for executing required validation scripts.

## Change Boundaries

### Design-Phase Boundary

Allowed design-phase artifacts:

- `specs/053-ci-ops-evidence-hardening/design.md`
- `specs/053-ci-ops-evidence-hardening/state.json` execution fields and `artifacts.design`
- `specs/053-ci-ops-evidence-hardening/report.md` design-phase note

Excluded design-phase surfaces:

- Runtime/source files.
- CI workflow files.
- Test source files.
- Docker/Compose/deploy files.
- Managed docs outside the feature folder.
- Framework-managed Bubbles files.
- `specs/054-artifact-output-summarization`.

### Successor Planning Boundary

`bubbles.plan` may create `scopes.md` and planning-owned record tables under the 053 folder. It may not edit runtime/source files as part of planning.

### Successor Evidence Boundary

`bubbles.validate` and `bubbles.audit` may append evidence to report artifacts they own. They may not rewrite planning structures owned by `bubbles.plan` or final certification fields owned by another phase.

## Framework Boundary

TR-BUG-045-002-014 names framework guard behavior, including G061 transition request/rework queue counting and G022 phase provenance modeling. These are upstream Bubbles framework issues. In Smackerel:

- The product packet records TR-014 as excluded.
- Product scopes must not edit `.github/bubbles/scripts/state-transition-guard.sh` or any installed framework artifact.
- Product evidence may cite guard behavior only as a route boundary.
- Actual guard fixes must be made in the canonical Bubbles repo and installed downstream through the framework upgrade path.

## Alternatives Considered

| Alternative | Decision | Rationale |
|-------------|----------|-----------|
| Amend BUG-045-002 directly | Rejected | The predecessor packet is a closed evidence record. Amending it would blur the historical close-out and risk re-litigating a fix already proven by CI, local reproduction, and contract guards. |
| Amend spec 031 directly | Rejected for this packet | Spec 031 live-stack-testing principles may constrain regression planning, but this packet is about BUG-045-002 carry-forward evidence records, not global test doctrine. |
| Amend spec 023 directly | Rejected for this packet | Spec 023 adversarial-regression doctrine already informs TR-009. Changing the doctrine is not required to plan product evidence hardening here. |
| Split TR-008..012 into separate specs | Rejected | The records share source truth, consumer surfaces, wrapper disposition, and boundary proof. Splitting would fragment auditability. |
| Create `specs/054-artifact-output-summarization` now | Rejected | The user explicitly reserved that identifier as a later idea. This packet must not create it. |
| Treat TR-008 as automatic new work | Rejected | Current traceability proof may show no residual gaps. Inventing work would violate the no-invented-gap requirement. |
| Pull TR-014 into Smackerel product scope | Rejected | Framework-managed files are upstream-owned and immutable in downstream product repos. |

## Risks And Mitigations

| Risk | Mitigation |
|------|------------|
| Stale G068 evidence becomes invented work. | Scope 1 starts with current traceability evidence and supports closure-by-proof. |
| Regression expansion duplicates existing proof. | Scope 2 requires every row to name a new protected failure mode. |
| Consumer trace misses indirect evidence consumers. | Scope 3 requires direct, indirect, observational, and documentation-facing classes. |
| Shared-infrastructure changes validate only themselves. | Scope 4 requires downstream canaries for protected contracts. |
| G040 wrappers linger without ownership. | Scope 5 requires a wrapper disposition record for each predecessor region. |
| Framework work leaks into Smackerel. | TR-014 is excluded and framework-owned paths are listed as product no-edit. |
| Planning packet accidentally changes source. | Boundary records and no-source-delta proof are required before validation. |

## Open Questions

No design-blocking questions remain. The first unresolved factual decision is Scope 1's current traceability result for TR-008; it belongs to the planning or validation owner that executes the guard and records the output.

## Design Completion Statement

This design creates the planning architecture for spec 053 and leaves the packet in `in_progress`. It does not mark scopes or the spec done, does not create `scopes.md`, does not create spec 054, and does not authorize source or framework file changes. The next required owner is `bubbles.plan`.# Design: 053 CI Ops Evidence Hardening

## Design Brief

### Current State

BUG-045-002 is closed as `done_with_concerns`, not because the CI integration fix is unproven, but because five product-owned planning concerns remain routed from the bug packet into a successor Smackerel planning packet. The predecessor evidence shows the original chronic CI failure was fixed by routing the GitHub Actions integration job through the canonical `./smackerel.sh test integration` path, adding a topology contract guard, proving local full-stack reproduction, and observing green CI runs on main.

The remaining product transition requests are TR-BUG-045-002-008 through TR-BUG-045-002-012. TR-BUG-045-002-014 is framework-owned and remains outside this Smackerel product packet except as a boundary note.

### Target State

This feature becomes the single product-side planning contract for those five carry-forward concerns. It defines how to prove or close the G068 fidelity item, how to plan regression expansion without duplicating already-green BUG-045-002 proof, how to inventory consumers, how to bound shared-infrastructure blast radius, and how to decide G040 wrapper disposition.

The target is not implementation. The target is a traceable evidence architecture that lets `bubbles.plan`, `bubbles.harden`, `bubbles.validate`, and `bubbles.audit` act without inventing gaps, mutating runtime surfaces, or pulling framework work into Smackerel.

### Patterns To Follow

- Keep the consolidated feature folder at `specs/053-ci-ops-evidence-hardening` as the current product planning surface.
- Preserve the BUG-045-002 trace-ID anchor approach for G068: literal scenario IDs in DoD mappings are the durable link pattern.
- Use the predecessor bug packet as source evidence: `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md` and `state.json`.
- Treat Bubbles evidence provenance tags exactly as defined by `evidence-rules.md`: `executed`, `interpreted`, and `not-run`.
- Treat shared-infrastructure changes as protected surfaces requiring canary contracts before broad validation.

### Patterns To Avoid

- Do not reopen the original BUG-045-002 root-cause design unless new evidence proves the CI fix regressed.
- Do not create `specs/054-artifact-output-summarization` in this packet; that identifier remains a reserved related idea.
- Do not amend framework-managed files under `.github/bubbles/`, `.github/agents/bubbles_shared/`, `.github/instructions/bubbles-*`, or `.github/skills/bubbles-*` from this product repo.
- Do not rewrite predecessor Gherkin or DoD text just to satisfy a string-matching gate; use source-grounded trace IDs and owner-scoped planning.
- Do not plan broad source changes before a boundary record names the allowed file families and excluded surfaces.

### Resolved Decisions

- TR-008 through TR-012 stay in one consolidated spec because their evidence, consumers, blast-radius, and wrapper decisions share the same predecessor packet.
- TR-008 is proof-gated: current traceability evidence comes before any new G068 work is planned.
- Regression expansion must add new protection beyond the existing topology guard, adversarial contract tests, and local full-stack reproduction.
- Consumer trace, blast-radius, boundary, and wrapper decisions are artifact records first, not runtime changes.
- TR-014 remains framework-owned.

### Open Questions

- None block design completion. The current traceability result for TR-008 must be produced by the owning planning or validation phase before closure decisions are made.

## Purpose And Scope

This design defines the artifact architecture for an ops evidence-hardening packet. The packet exists to turn BUG-045-002 carry-forward concerns into explicit planning records, scope boundaries, and verification expectations without editing runtime code, CI workflow code, deploy adapters, framework install artifacts, or test source in the design phase.

The design covers:

- TR-BUG-045-002-008: G068 proof-or-close fidelity work.
- TR-BUG-045-002-009: regression E2E expansion planning.
- TR-BUG-045-002-010: CI workflow consumer trace planning.
- TR-BUG-045-002-011: shared-infrastructure blast-radius planning.
- TR-BUG-045-002-012: change boundary and G040 wrapper disposition planning.

The design excludes:

- TR-BUG-045-002-014 framework guard maintenance.
- Runtime/source implementation.
- CI workflow refactoring.
- New framework script, agent, instruction, or skill edits in the Smackerel repo.
- Creation of `specs/054-artifact-output-summarization`.

## Current Truth

| Source | Current Truth | Design Consequence |
|--------|---------------|--------------------|
| BUG-045-002 `state.json` top-level status | The predecessor packet is `done_with_concerns`. | This packet preserves the prior close-out while resolving product-owned planning concerns separately. |
| BUG-045-002 `transitionRequests[]` | TR-008..012 are open, owned by `bubbles.plan`, and point to G068, regression expansion, consumer trace, blast radius, and boundary/wrapper planning. | The design maps each product TR to a specific record type and scope architecture. |
| BUG-045-002 `certification.concerns[]` | Six low-severity concerns were recorded; five are product planning concerns, one is framework guard maintenance. | Product work covers only the five Smackerel concerns and records TR-014 as a framework boundary. |
| BUG-045-002 report Evidence 1-5b | The chronic CI failure came from local-vs-CI topology drift. | The design does not re-litigate root cause; it hardens evidence around the already-fixed path. |
| BUG-045-002 validation and audit evidence | Path A, topology guard, adversarial sub-tests, local full-stack reproduction, and green CI evidence were already recorded. | Regression expansion must avoid duplicate proof and name only additional protective scenarios. |
| BUG-045-002 G068 history | Traceability failed with 11 unmapped scenarios, then the plan re-entry resolved the packet through trace-ID anchors and guard output later showed 11/11 mapped at audit entry. | TR-008 starts with a current proof step. No residual gap may be invented from stale pre-anchor output. |
| BUG-045-002 G040 wrappers | Report sections preserved routed planning content inside skip regions. | Wrapper disposition must be explicit: retain as history, retain with cross-reference, or remove only by owning artifact phase after coverage is proven. |
| Smackerel Bubbles governance | Framework-managed files are upstream-owned and immutable in downstream repos. | TR-014 is not product design scope; product artifacts may only record the boundary and route. |

## Architecture Overview

The feature is an artifact-only control plane. Its active objects are planning records, not runtime services.

```text
BUG-045-002 report/state
        |
        v
053 spec.md requirements and scenarios
        |
        v
053 design.md record model and boundaries
        |
        v
053 scopes.md planned by bubbles.plan
        |
        v
harden/docs/validate/audit evidence produced by owning phases
```

The architecture has four layers:

| Layer | Responsibility | Owned By |
|-------|----------------|----------|
| Source-truth layer | Predecessor BUG-045-002 report/state and current 053 spec. | `bubbles.analyst`, predecessor packet owners |
| Design layer | Record model, boundaries, phase ownership, verification strategy. | `bubbles.design` |
| Planning layer | Five scopes, Gherkin-to-test mapping, DoD, evidence slots. | `bubbles.plan` |
| Certification layer | Artifact lint, traceability guard, no-source-delta proof, audit interpretation. | `bubbles.validate`, `bubbles.audit` |

No runtime component is introduced. No database table, API endpoint, service route, Docker resource, UI surface, or deployment target changes.

## Design Decisions

### DD-053-001: One Consolidated Spec

TR-008 through TR-012 stay in `specs/053-ci-ops-evidence-hardening` because all five requests depend on the same predecessor packet and the same evidence boundary. Splitting them would create duplicate source-surface mappings and increase the chance that G040 wrapper disposition, consumer trace, and blast-radius decisions drift apart.

`specs/054-artifact-output-summarization` is not created here. If that idea is pursued, it must begin as a separate owner-approved feature with its own source evidence.

### DD-053-002: Proof-Before-Work For G068

TR-008 must begin with a current traceability result against the relevant artifacts. The first valid outcome is not automatically new scope; it is one of:

| Outcome | Meaning | Required Record |
|---------|---------|-----------------|
| `residual-gap-found` | Current traceability evidence shows a specific unmapped or weakly mapped scenario. | G068 residual-gap record with scenario ID, missing claim, owning artifact, and required evidence. |
| `closed-by-current-proof` | Current traceability evidence shows the scenario set is fully mapped. | Closure-by-evidence record citing command output and predecessor source surface. |
| `owner-routed-tool-issue` | The traceability tool produces a framework-owned false positive. | Framework-boundary route record; no Smackerel source work. |

This prevents stale pre-anchor evidence from becoming invented product work.

### DD-053-003: Regression Expansion Boundaries

TR-009 expansion has three allowed surfaces:

| Surface | Allowed Planning Question | Excluded Duplicate |
|---------|---------------------------|--------------------|
| CI integration job | What scenario proves the job still routes through canonical integration behavior as an observable CI contract? | Repeating only that BUG-045-002 already had a green CI run. |
| Full-stack reproduction path | What live-stack scenario proves the predecessor failure class remains covered outside GitHub Actions? | Repeating the exact BUG-045-002 local reproduction without a new protective claim. |
| Contract-test surface | What contract or adversarial check protects against topology drift or evidence-shape drift? | Re-listing the existing topology guard and three adversarial tests without a new gap. |

Every planned regression row must name the new failure mode it protects. If no new protective claim exists, the row is not added.

### DD-053-004: Consumer Trace Model

Consumer trace is planned before any evidence-shape change. A consumer is any first-party artifact that reads, references, asserts, documents, or operationally depends on the CI integration job, its logs, its command shape, or its evidence output.

Consumer classes:

| Class | Definition | Examples |
|-------|------------|----------|
| `direct` | Reads or executes the CI integration workflow or canonical command. | CI workflow, repo CLI, integration test runner. |
| `indirect` | Validates or parses the workflow/evidence shape. | Contract tests, traceability records, artifact lint references. |
| `observational` | Uses CI results as evidence without executing the job. | Report sections, validation records, certification concerns. |
| `documentation-facing` | Describes the user-facing command or operational evidence surface. | README, Testing, Operations, agents memory. |

Every consumer receives exactly one disposition: `required-change`, `no-change-with-evidence`, or `owner-routed`.

### DD-053-005: Shared Infrastructure Blast-Radius Model

Shared infrastructure surfaces are protected. A protected surface is any artifact whose change could affect multiple tests, workflows, evidence gates, or operator commands.

Protected surfaces for this packet:

| Surface | Protected Contract | Canary Requirement |
|---------|--------------------|--------------------|
| Test stack lifecycle | Test stack starts, reports health, runs integration suites, and tears down through `./smackerel.sh`. | A narrow command/evidence canary that proves the lifecycle contract before broad suite reliance. |
| CI workflow ordering | CI uses the canonical command and preserves upload/failure-step semantics. | Structural guard or workflow-evidence canary. |
| Contract-test parsing | Tests parse the intended workflow and reject known-bad topologies. | Adversarial guard cases for each rejected pattern. |
| CLI wrappers | User-facing command remains `./smackerel.sh test integration`; wrapper helpers remain internal. | CLI command evidence or no-change-with-evidence disposition. |

Broad validation may be planned only after the canary for the changed protected surface is named.

### DD-053-006: Change Boundary Model

Each scope must carry a boundary record with:

- Allowed artifact families.
- Excluded runtime/source/framework surfaces.
- Allowed change type.
- Required no-source-delta proof.
- Owner responsible for resolving boundary drift.

The default allowed surfaces for this feature are product planning artifacts under `specs/053-ci-ops-evidence-hardening`. Runtime and source paths are observational unless a later owner creates a separate implementation scope with explicit user approval and governance coverage.

### DD-053-007: G040 Wrapper Disposition Model

G040 wrapper content in the predecessor report is historical evidence until an owning artifact phase records a disposition. Valid dispositions:

| Disposition | Meaning | Required Proof |
|-------------|---------|----------------|
| `historical-retain` | The wrapper remains because it preserves predecessor audit history. | Current 053 records cite and map the content. |
| `cross-reference-retain` | The wrapper remains and receives or is paired with a reference to the 053 record that now owns the concern. | Source-surface mapping plus no-source-delta proof for excluded files. |
| `owner-remove` | The owning artifact phase removes wrapper content because all contained planning claims are represented in 053 and audit accepts no evidence loss. | Wrapper disposition record, mapped scope, and audit confirmation. |

Design does not remove or edit predecessor wrappers. It defines the model that later owners must use.

### DD-053-008: Framework Boundary

TR-014 remains outside Smackerel product scope. Product artifacts may mention it only to preserve routing truth and to avoid accidental edits to framework-managed install artifacts. Any framework guard repair belongs in the canonical Bubbles repository first, then downstream Smackerel receives it through the standard upgrade path.

## Data And Artifact Model

### TR Matrix

The TR matrix is the root index for the packet.

| Field | Required | Description |
|-------|----------|-------------|
| `trId` | Yes | Transition request ID, e.g. `TR-BUG-045-002-008`. |
| `sourceArtifact` | Yes | Predecessor artifact path and section or state field. |
| `sourceClaim` | Yes | The exact source-grounded concern. |
| `scenarioIds` | Yes | Current 053 scenarios that cover the TR. |
| `requirementIds` | Yes | Current 053 FRs that cover the TR. |
| `plannedRecordType` | Yes | One of G068, regression, consumer, blast-radius, boundary, wrapper. |
| `disposition` | Yes | `planned`, `closed-by-current-proof`, `owner-routed`, or `excluded-framework`. |
| `evidenceExpectation` | Yes | Command/tool/artifact evidence expected by the owning phase. |

Initial TR matrix:

| TR | Planned Record Type | Required First Disposition |
|----|---------------------|----------------------------|
| TR-BUG-045-002-008 | G068 proof-or-close | Current traceability proof before scope creation. |
| TR-BUG-045-002-009 | Regression expansion | New protective scenario or no-addition rationale. |
| TR-BUG-045-002-010 | Consumer inventory | Every consumer classified and dispositioned. |
| TR-BUG-045-002-011 | Blast-radius | Protected contracts and canaries named. |
| TR-BUG-045-002-012 | Boundary and wrapper | Allowed/excluded surfaces and wrapper disposition named. |
| TR-BUG-045-002-014 | Framework boundary | Excluded from product scope. |

### Source-Surface Matrix

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Stable local ID, e.g. `SRC-053-BUG-REPORT-G040`. |
| `path` | Yes | Artifact or source path. |
| `surfaceKind` | Yes | `source-truth`, `observed`, `protected`, `excluded`, or `framework-owned`. |
| `relationship` | Yes | How the surface relates to a TR or scenario. |
| `allowedAction` | Yes | `cite-only`, `plan-record`, `artifact-edit-by-owner`, `no-edit`, or `framework-route`. |
| `proofRequired` | Yes | Evidence needed to prove the action stayed within boundary. |

Required source surfaces:

| Surface | Kind | Allowed Action |
|---------|------|----------------|
| BUG-045-002 `report.md` | `source-truth` | `cite-only` unless a later owner records wrapper disposition. |
| BUG-045-002 `state.json` | `source-truth` | `cite-only` for TR and concern truth. |
| `specs/053-ci-ops-evidence-hardening/spec.md` | `source-truth` | `plan-record` source for design and scopes. |
| `specs/053-ci-ops-evidence-hardening/design.md` | `source-truth` | Current design record. |
| `.github/workflows/ci.yml` | `observed` / `protected` | No design-phase edits; may be cited as observed subject. |
| `internal/deploy/*` contract tests | `observed` / `protected` | No design-phase edits; may be cited as prior proof. |
| `.github/bubbles/*` and `.github/agents/bubbles_shared/*` | `framework-owned` | Product no-edit; route to framework owner. |

### Evidence Provenance Categories

This design uses the Bubbles canonical provenance taxonomy and adds planning overlays.

| Category | Meaning | Allowed Completion Use |
|----------|---------|------------------------|
Inactive duplicate heading: Design: 053 CI Ops Evidence Hardening

## Design Brief

### Current State

BUG-045-002 is closed as `done_with_concerns`, not because the CI integration fix is unproven, but because five product-owned planning concerns remain routed from the bug packet into a successor Smackerel planning packet. The predecessor evidence shows the original chronic CI failure was fixed by routing the GitHub Actions integration job through the canonical `./smackerel.sh test integration` path, adding a topology contract guard, proving local full-stack reproduction, and observing green CI runs on main.

The remaining product transition requests are TR-BUG-045-002-008 through TR-BUG-045-002-012. TR-BUG-045-002-014 is framework-owned and remains outside this Smackerel product packet except as a boundary note.

### Target State

This feature becomes the single product-side planning contract for those five carry-forward concerns. It defines how to prove or close the G068 fidelity item, how to plan regression expansion without duplicating already-green BUG-045-002 proof, how to inventory consumers, how to bound shared-infrastructure blast radius, and how to decide G040 wrapper disposition.

The target is not implementation. The target is a traceable evidence architecture that lets `bubbles.plan`, `bubbles.harden`, `bubbles.validate`, and `bubbles.audit` act without inventing gaps, mutating runtime surfaces, or pulling framework work into Smackerel.

### Patterns To Follow

- Keep the consolidated feature folder at `specs/053-ci-ops-evidence-hardening` as the current product planning surface.
- Preserve the BUG-045-002 trace-ID anchor approach for G068: literal scenario IDs in DoD mappings are the durable link pattern.
- Use the predecessor bug packet as source evidence: `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md` and `state.json`.
- Treat Bubbles evidence provenance tags exactly as defined by `evidence-rules.md`: `executed`, `interpreted`, and `not-run`.
- Treat shared-infrastructure changes as protected surfaces requiring canary contracts before broad validation.

### Patterns To Avoid

- Do not reopen the original BUG-045-002 root-cause design unless new evidence proves the CI fix regressed.
- Do not create `specs/054-artifact-output-summarization` in this packet; that identifier remains a reserved related idea.
- Do not amend framework-managed files under `.github/bubbles/`, `.github/agents/bubbles_shared/`, `.github/instructions/bubbles-*`, or `.github/skills/bubbles-*` from this product repo.
- Do not rewrite predecessor Gherkin or DoD text just to satisfy a string-matching gate; use source-grounded trace IDs and owner-scoped planning.
- Do not plan broad source changes before a boundary record names the allowed file families and excluded surfaces.

### Resolved Decisions

- TR-008 through TR-012 stay in one consolidated spec because their evidence, consumers, blast-radius, and wrapper decisions share the same predecessor packet.
- TR-008 is proof-gated: current traceability evidence comes before any new G068 work is planned.
- Regression expansion must add new protection beyond the existing topology guard, adversarial contract tests, and local full-stack reproduction.
- Consumer trace, blast-radius, boundary, and wrapper decisions are artifact records first, not runtime changes.
- TR-014 remains framework-owned.

### Open Questions

- None block design completion. The current traceability result for TR-008 must be produced by the owning planning or validation phase before closure decisions are made.

## Purpose And Scope

This design defines the artifact architecture for an ops evidence-hardening packet. The packet exists to turn BUG-045-002 carry-forward concerns into explicit planning records, scope boundaries, and verification expectations without editing runtime code, CI workflow code, deploy adapters, framework install artifacts, or test source in the design phase.

The design covers:

- TR-BUG-045-002-008: G068 proof-or-close fidelity work.
- TR-BUG-045-002-009: regression E2E expansion planning.
- TR-BUG-045-002-010: CI workflow consumer trace planning.
- TR-BUG-045-002-011: shared-infrastructure blast-radius planning.
- TR-BUG-045-002-012: change boundary and G040 wrapper disposition planning.

The design excludes:

- TR-BUG-045-002-014 framework guard maintenance.
- Runtime/source implementation.
- CI workflow refactoring.
- New framework script, agent, instruction, or skill edits in the Smackerel repo.
- Creation of `specs/054-artifact-output-summarization`.

## Current Truth

| Source | Current Truth | Design Consequence |
|--------|---------------|--------------------|
| BUG-045-002 `state.json` top-level status | The predecessor packet is `done_with_concerns`. | This packet must preserve the prior close-out while resolving the product-owned concerns separately. |
| BUG-045-002 `transitionRequests[]` | TR-008..012 are open, owned by `bubbles.plan`, and point to G068, regression expansion, consumer trace, blast radius, and boundary/wrapper planning. | The design must map each product TR to a specific record type and scope architecture. |
| BUG-045-002 `certification.concerns[]` | Six low-severity concerns were recorded; five are product planning concerns, one is framework guard maintenance. | Product work covers only the five Smackerel concerns and records TR-014 as a framework boundary. |
| BUG-045-002 report Evidence 1-5b | The chronic CI failure came from local-vs-CI topology drift. | The current design must not re-litigate root cause; it hardens evidence around the already-fixed path. |
| BUG-045-002 validation and audit evidence | Path A, topology guard, adversarial sub-tests, local full-stack reproduction, and green CI evidence were already recorded. | Regression expansion must avoid duplicate proof and name only additional protective scenarios. |
| BUG-045-002 G068 history | Traceability failed with 11 unmapped scenarios, then the plan re-entry resolved the packet through trace-ID anchors and guard output later showed 11/11 mapped at audit entry. | TR-008 must start with a current proof step. No residual gap may be invented from stale pre-anchor output. |
| BUG-045-002 G040 wrappers | Report sections preserved routed planning content inside skip regions. | Wrapper disposition must be explicit: retain as history, retain with cross-reference, or remove only by owning artifact phase after coverage is proven. |
| Smackerel Bubbles governance | Framework-managed files are upstream-owned and immutable in downstream repos. | TR-014 is not product design scope; product artifacts may only record the boundary and route. |

## Architecture Overview

The feature is an artifact-only control plane. Its active objects are planning records, not runtime services.

```text
BUG-045-002 report/state
        |
        v
053 spec.md requirements and scenarios
        |
        v
053 design.md record model and boundaries
        |
        v
053 scopes.md planned by bubbles.plan
        |
        v
harden/docs/validate/audit evidence produced by owning phases
```

The architecture has four layers:

| Layer | Responsibility | Owned By |
|-------|----------------|----------|
| Source-truth layer | Predecessor BUG-045-002 report/state and current 053 spec. | `bubbles.analyst`, predecessor packet owners |
| Design layer | Record model, boundaries, phase ownership, verification strategy. | `bubbles.design` |
| Planning layer | Five scopes, Gherkin-to-test mapping, DoD, evidence slots. | `bubbles.plan` |
| Certification layer | Artifact lint, traceability guard, no-source-delta proof, audit interpretation. | `bubbles.validate`, `bubbles.audit` |

No runtime component is introduced. No database table, API endpoint, service route, Docker resource, or deployment target changes.

## Design Decisions

### DD-053-001: One Consolidated Spec

TR-008 through TR-012 stay in `specs/053-ci-ops-evidence-hardening` because all five requests depend on the same predecessor packet and the same evidence boundary. Splitting them would create duplicate source-surface mappings and increase the chance that G040 wrapper disposition, consumer trace, and blast-radius decisions drift apart.

`specs/054-artifact-output-summarization` is not created here. If that idea is pursued, it must begin as a separate owner-approved feature with its own source evidence.

### DD-053-002: Proof-Before-Work For G068

TR-008 must begin with a current traceability result against the relevant artifacts. The first valid outcome is not automatically new scope; it is one of:

| Outcome | Meaning | Required Record |
|---------|---------|-----------------|
| `residual-gap-found` | Current traceability evidence shows a specific unmapped or weakly mapped scenario. | G068 residual-gap record with scenario ID, missing claim, owning artifact, and required evidence. |
| `closed-by-current-proof` | Current traceability evidence shows the scenario set is fully mapped. | Closure-by-evidence record citing command output and predecessor source surface. |
| `owner-routed-tool-issue` | The traceability tool produces a framework-owned false positive. | Framework-boundary route record; no Smackerel source work. |

This prevents stale pre-anchor evidence from becoming invented product work.

### DD-053-003: Regression Expansion Boundaries

TR-009 expansion has three allowed surfaces:

| Surface | Allowed Planning Question | Excluded Duplicate |
|---------|---------------------------|--------------------|
| CI integration job | What scenario proves the job still routes through canonical integration behavior as an observable CI contract? | Repeating only that BUG-045-002 already had a green CI run. |
| Full-stack reproduction path | What live-stack scenario proves the predecessor failure class remains covered outside GitHub Actions? | Repeating the exact BUG-045-002 local reproduction without a new protective claim. |
| Contract-test surface | What contract or adversarial check protects against topology drift or evidence-shape drift? | Re-listing the existing topology guard and three adversarial tests without a new gap. |

Every planned regression row must name the new failure mode it protects. If no new protective claim exists, the row is not added.

### DD-053-004: Consumer Trace Model

Consumer trace is planned before any evidence-shape change. A consumer is any first-party artifact that reads, references, asserts, documents, or operationally depends on the CI integration job, its logs, its command shape, or its evidence output.

Consumer classes:

| Class | Definition | Examples |
|-------|------------|----------|
| `direct` | Reads or executes the CI integration workflow or canonical command. | CI workflow, repo CLI, integration test runner. |
| `indirect` | Validates or parses the workflow/evidence shape. | Contract tests, traceability records, artifact lint references. |
| `observational` | Uses CI results as evidence without executing the job. | Report sections, validation records, certification concerns. |
| `documentation-facing` | Describes the user-facing command or operational evidence surface. | README, Testing, Operations, agents memory. |

Every consumer receives exactly one disposition: `required-change`, `no-change-with-evidence`, or `owner-routed`.

### DD-053-005: Shared Infrastructure Blast-Radius Model

Shared infrastructure surfaces are protected. A protected surface is any artifact whose change could affect multiple tests, workflows, evidence gates, or operator commands.

Protected surfaces for this packet:

| Surface | Protected Contract | Canary Requirement |
|---------|--------------------|--------------------|
| Test stack lifecycle | Test stack starts, reports health, runs integration suites, and tears down through `./smackerel.sh`. | A narrow command/evidence canary that proves the lifecycle contract before broad suite reliance. |
| CI workflow ordering | CI uses the canonical command and preserves upload/failure-step semantics. | Structural guard or workflow-evidence canary. |
| Contract-test parsing | Tests parse the intended workflow and reject known-bad topologies. | Adversarial guard cases for each rejected pattern. |
| CLI wrappers | User-facing command remains `./smackerel.sh test integration`; wrapper helpers remain internal. | CLI command evidence or no-change-with-evidence disposition. |

Broad validation may be planned only after the canary for the changed protected surface is named.

### DD-053-006: Change Boundary Model

Each scope must carry a boundary record with:

- Allowed artifact families.
- Excluded runtime/source/framework surfaces.
- Allowed change type.
- Required no-source-delta proof.
- Owner responsible for resolving boundary drift.

The default allowed surfaces for this feature are product planning artifacts under `specs/053-ci-ops-evidence-hardening`. Runtime and source paths are observational unless a later owner creates a separate implementation scope with explicit user approval and governance coverage.

### DD-053-007: G040 Wrapper Disposition Model

G040 wrapper content in the predecessor report is historical evidence until an owning artifact phase records a disposition. Valid dispositions:

| Disposition | Meaning | Required Proof |
|-------------|---------|----------------|
| `historical-retain` | The wrapper remains because it preserves predecessor audit history. | Current 053 records cite and map the content. |
| `cross-reference-retain` | The wrapper remains and receives or is paired with a reference to the 053 record that now owns the concern. | Source-surface mapping plus no-source-delta proof for excluded files. |
| `owner-remove` | The owning artifact phase removes wrapper content because all contained planning claims are represented in 053 and audit accepts no evidence loss. | Wrapper disposition record, mapped scope, and audit confirmation. |

Design does not remove or edit predecessor wrappers. It defines the model that later owners must use.

### DD-053-008: Framework Boundary

TR-014 remains outside Smackerel product scope. Product artifacts may mention it only to preserve routing truth and to avoid accidental edits to framework-managed install artifacts. Any framework guard repair belongs in the canonical Bubbles repository first, then downstream Smackerel receives it through the standard upgrade path.

## Data And Artifact Model

### TR Matrix

The TR matrix is the root index for the packet.

| Field | Required | Description |
|-------|----------|-------------|
| `trId` | Yes | Transition request ID, e.g. `TR-BUG-045-002-008`. |
| `sourceArtifact` | Yes | Predecessor artifact path and section or state field. |
| `sourceClaim` | Yes | The exact source-grounded concern. |
| `scenarioIds` | Yes | Current 053 scenarios that cover the TR. |
| `requirementIds` | Yes | Current 053 FRs that cover the TR. |
| `plannedRecordType` | Yes | One of G068, regression, consumer, blast-radius, boundary, wrapper. |
| `disposition` | Yes | `planned`, `closed-by-current-proof`, `owner-routed`, or `excluded-framework`. |
| `evidenceExpectation` | Yes | Command/tool/artifact evidence expected by the owning phase. |

Initial TR matrix:

| TR | Planned Record Type | Required First Disposition |
|----|---------------------|----------------------------|
| TR-BUG-045-002-008 | G068 proof-or-close | Current traceability proof before scope creation. |
| TR-BUG-045-002-009 | Regression expansion | New protective scenario or no-addition rationale. |
| TR-BUG-045-002-010 | Consumer inventory | Every consumer classified and dispositioned. |
| TR-BUG-045-002-011 | Blast-radius | Protected contracts and canaries named. |
| TR-BUG-045-002-012 | Boundary and wrapper | Allowed/excluded surfaces and wrapper disposition named. |
| TR-BUG-045-002-014 | Framework boundary | Excluded from product scope. |

### Source-Surface Matrix

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Stable local ID, e.g. `SRC-053-BUG-REPORT-G040`. |
| `path` | Yes | Artifact or source path. |
| `surfaceKind` | Yes | `source-truth`, `observed`, `protected`, `excluded`, or `framework-owned`. |
| `relationship` | Yes | How the surface relates to a TR or scenario. |
| `allowedAction` | Yes | `cite-only`, `plan-record`, `artifact-edit-by-owner`, `no-edit`, or `framework-route`. |
| `proofRequired` | Yes | Evidence needed to prove the action stayed within boundary. |

Required source surfaces:

| Surface | Kind | Allowed Action |
|---------|------|----------------|
| BUG-045-002 `report.md` | `source-truth` | `cite-only` unless a later owner records wrapper disposition. |
| BUG-045-002 `state.json` | `source-truth` | `cite-only` for TR and concern truth. |
| `specs/053-ci-ops-evidence-hardening/spec.md` | `source-truth` | `plan-record` source for design and scopes. |
| `specs/053-ci-ops-evidence-hardening/design.md` | `source-truth` | Current design record. |
| `.github/workflows/ci.yml` | `observed` / `protected` | No design-phase edits; may be cited as observed subject. |
| `internal/deploy/*` contract tests | `observed` / `protected` | No design-phase edits; may be cited as prior proof. |
| `.github/bubbles/*` and `.github/agents/bubbles_shared/*` | `framework-owned` | Product no-edit; route to framework owner. |

### Evidence Provenance Categories

This design uses the Bubbles canonical provenance taxonomy and adds planning overlays.

| Category | Meaning | Allowed Completion Use |
|----------|---------|------------------------|
| `executed` | Current-session command output directly proves the claim. | Can support checked DoD when owned by the phase. |
| `interpreted` | Evidence exists, but the conclusion requires explanation. | Must include an interpretation and remains audit-sensitive. |
| `not-run` | No command was executed for the claim. | Cannot close DoD; must remain uncertain. |
| `predecessor-source` | Historical source from BUG-045-002 report/state. | Can ground planning claims, not current execution claims. |
| `owner-routed` | Evidence or fix belongs to another owner. | Must name owner and boundary; cannot close product work as executed. |
| `closure-by-proof` | Current executed evidence proves no residual work exists. | Valid for TR-008 only when traceability output supports it. |

### Consumer Inventory Record

| Field | Required | Description |
|-------|----------|-------------|
| `consumerId` | Yes | Stable ID, e.g. `CON-053-CI-WORKFLOW`. |
| `pathOrSurface` | Yes | Path, artifact, command, or docs surface. |
| `consumerClass` | Yes | `direct`, `indirect`, `observational`, or `documentation-facing`. |
| `consumedSignal` | Yes | CI job name, command string, evidence section, path, or status field consumed. |
| `staleRisk` | Yes | What could break if the consumed signal changes. |
| `disposition` | Yes | `required-change`, `no-change-with-evidence`, or `owner-routed`. |
| `evidenceRef` | Yes | Artifact, command, or scan that supports the disposition. |
| `owner` | Yes | Phase or repo owner responsible for the disposition. |

### Blast-Radius Record

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Protected surface ID. |
| `protectedContract` | Yes | Contract that must remain true. |
| `dependentSurfaces` | Yes | Consumers or workflows that depend on it. |
| `canaryCheck` | Yes | Narrow validation that proves the protected contract. |
| `broadValidationTrigger` | Yes | When broad validation is justified. |
| `rollbackOrRestore` | Yes | How the artifact returns to prior state if the canary fails. |
| `evidenceExpectation` | Yes | Raw output or artifact proof expected. |

### Boundary Record

| Field | Required | Description |
|-------|----------|-------------|
| `scopeId` | Yes | Scope planned by `bubbles.plan`. |
| `allowedFileFamilies` | Yes | Paths allowed for that scope. |
| `excludedSurfaces` | Yes | Paths explicitly protected from edits. |
| `allowedChangeType` | Yes | `artifact-only`, `observational-scan`, `docs-cross-reference`, or `source-change-by-approved-scope`. |
| `noSourceDeltaProof` | Yes | Git status/diff evidence proving excluded surfaces did not change. |
| `owner` | Yes | Phase owner responsible for maintaining boundary. |

### Wrapper Disposition Record

| Field | Required | Description |
|-------|----------|-------------|
| `wrapperId` | Yes | Stable ID for each G040 region. |
| `predecessorLocation` | Yes | BUG-045-002 report section or line anchor description. |
| `containedClaim` | Yes | Planning concern preserved inside the wrapper. |
| `mappedTrId` | Yes | TR-008, TR-010, TR-011, or TR-012 as applicable. |
| `mappedScopeId` | Yes | Scope planned in 053. |
| `disposition` | Yes | `historical-retain`, `cross-reference-retain`, or `owner-remove`. |
| `crossReferenceRequired` | Yes | Whether the predecessor artifact needs an explicit 053 pointer. |
| `evidenceExpectation` | Yes | Proof that no evidence was lost or misrouted. |

## Workflow And Phase Ownership

| Phase / Agent | Responsibility In This Packet | Must Not Do |
|---------------|-------------------------------|-------------|
| `bubbles.analyst` | Owns `spec.md`, source-grounded requirements, actors, use cases, scenarios, FRs, product principle alignment. | Must not author `design.md` or `scopes.md`. |
| `bubbles.design` | Owns this `design.md`, record model, boundaries, phase ownership, verification strategy, alternatives, risks. | Must not implement source changes, create scopes, or certify completion. |
| `bubbles.plan` | Owns five-scope architecture in `scopes.md`, Test Plan rows, DoD, scenario-to-test mapping, and uncertainty declarations. | Must not claim executed evidence without running commands. |
| `bubbles.harden` | May diagnose planning-hardening gaps after scopes exist and route foreign-owned artifact changes to the owner. | Must not directly rewrite `scopes.md` planning content or source code. |
| `bubbles.docs` | May update managed docs only if the plan proves user-facing documentation changed. | Must not document internal-only topology as a user-facing contract. |
| `bubbles.validate` | Runs artifact lint, traceability guard where applicable, source-surface mapping checks, and no-source-delta proof. | Must not set final `done` state without satisfying ownership and evidence gates. |
| `bubbles.audit` | Independently reviews evidence provenance, no-invented-gap decisions, consumer trace completeness, and boundary compliance. | Must not fill missing planning artifacts directly. |
| `bubbles.workflow` / finalize | Records final route or outcome after owner phases are complete. | Must not route TR-014 into Smackerel product implementation. |

## Scope Architecture

The scope architecture is intentionally five scopes, aligned to the grill recommendation to keep TR-008 through TR-012 together while still isolating evidence surfaces.

| Scope | Primary TR | Design Intent | Key Records |
|-------|------------|---------------|-------------|
| Scope 1: G068 Proof-Or-Close | TR-008 | Execute or reference current traceability evidence and decide whether residual G068 work exists. | TR matrix rows, G068 residual-gap or closure-by-proof records, evidence provenance categories. |
| Scope 2: Regression Expansion Boundaries | TR-009 | Define only regression E2E or contract rows that add protection beyond BUG-045-002 proof. | Regression surface records, source-surface matrix, evidence expectation records. |
| Scope 3: Consumer Trace Inventory | TR-010 | Inventory direct, indirect, observational, and documentation-facing consumers of CI integration evidence. | Consumer inventory records and dispositions. |
| Scope 4: Shared Infrastructure Blast Radius | TR-011 | Identify protected contracts for test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrappers. | Blast-radius records, canary checks, broad-validation triggers. |
| Scope 5: Boundary And Wrapper Disposition | TR-012 plus TR-014 boundary | Define allowed/excluded surfaces, no-source-delta proof, wrapper disposition, and framework boundary. | Boundary records, wrapper disposition records, framework-boundary record. |

Scope ordering should preserve dependency flow: Scope 1 informs whether G068 work exists; Scope 2 uses that truth to avoid duplicate regression rows; Scope 3 identifies consumers before any evidence-shape decision; Scope 4 protects shared contracts; Scope 5 records final containment and wrapper disposition across the prior four scopes.

## Verification Strategy

### Artifact Lint

`artifact-lint.sh` is required for the feature folder after design and again after planning. A design-phase run verifies the artifact set and provenance tags available at this phase. Later runs verify scope/DoD/report structure after `bubbles.plan` creates `scopes.md`.

Expected evidence:

- Command executed from the Smackerel repo root.
- Exit code recorded.
- Raw output captured by the owning validation or audit phase when used as evidence.
- Any failure routed to the artifact owner rather than patched by the wrong phase.

### Traceability Guard

Traceability guard is required when Scope 1 evaluates TR-008 and after `bubbles.plan` creates the scenario/DoD mapping for this feature.

Decision rule:

| Guard Result | Planning Result |
|--------------|-----------------|
| Exit 0, all current scenarios mapped | TR-008 may be closed by current proof. |
| Exit nonzero with specific unmapped 053 scenario | Plan a residual-gap record for that scenario. |
| Exit nonzero due framework-owned false positive | Route to framework owner and do not create Smackerel source work. |

### Source-Surface Mapping

Each scope must produce or cite a source-surface matrix row for every predecessor claim it uses. The matrix must distinguish:

- Historical predecessor evidence.
- Current executed evidence.
- Observed source surfaces that are not edited.
- Framework-owned surfaces that are product no-edit.

### No-Source-Delta Proof

Because this packet is planning-only unless a later owner explicitly expands scope, validation must prove runtime/source surfaces did not change.

Minimum proof expectation:

| Protected Family | Expected Result |
|------------------|-----------------|
| `.github/workflows/ci.yml` | No diff unless an explicitly approved source-change scope exists. |
| `internal/`, `cmd/`, `scripts/runtime/`, `web/`, `ml/` | No diff for this planning packet. |
| `docker-compose*.yml`, `deploy/` | No diff for this planning packet. |
| `.github/bubbles/`, `.github/agents/bubbles_shared/` | No downstream edits. |
| `specs/054-artifact-output-summarization` | Path absent. |

### Evidence Expectations

- Planning conclusions may use `interpreted` provenance only when they explain the interpretation.
- Closure of TR-008 must use current executed traceability evidence, not only predecessor narrative.
- Consumer trace scans must record what was searched and how each consumer disposition was assigned.
- Blast-radius canaries must prove downstream contracts, not the changed fixture or wrapper alone.
- Boundary proof must show excluded surfaces stayed unchanged.

## Change Boundaries

### Design-Phase Boundary

Allowed design-phase artifacts:

- `specs/053-ci-ops-evidence-hardening/design.md`
- `specs/053-ci-ops-evidence-hardening/state.json` execution fields and `artifacts.design`
- `specs/053-ci-ops-evidence-hardening/report.md` design-phase note

Excluded design-phase surfaces:

- Runtime/source files.
- CI workflow files.
- Test source files.
- Docker/Compose/deploy files.
- Managed docs outside the feature folder.
- Framework-managed Bubbles files.
- `specs/054-artifact-output-summarization`.

### Successor Planning Boundary

`bubbles.plan` may create `scopes.md` and planning-owned record tables under the 053 folder. It may not edit runtime/source files as part of planning.

### Successor Evidence Boundary

`bubbles.validate` and `bubbles.audit` may append evidence to report artifacts they own. They may not rewrite planning structures owned by `bubbles.plan` or final certification fields owned by another phase.

## Framework Boundary

TR-BUG-045-002-014 names framework guard behavior, including G061 transition request/rework queue counting and G022 phase provenance modeling. These are upstream Bubbles framework issues. In Smackerel:

- The product packet records TR-014 as excluded.
- Product scopes must not edit `.github/bubbles/scripts/state-transition-guard.sh` or any installed framework artifact.
- Product evidence may cite guard behavior only as a route boundary.
- Actual guard fixes must be made in the canonical Bubbles repo and installed downstream through the framework upgrade path.

## Alternatives Considered

| Alternative | Decision | Rationale |
|-------------|----------|-----------|
| Amend BUG-045-002 directly | Rejected | The predecessor packet is a closed evidence record. Amending it would blur the historical close-out and risk re-litigating a fix already proven by CI, local reproduction, and contract guards. |
| Amend spec 031 directly | Rejected for this packet | Spec 031 live-stack-testing principles may constrain regression planning, but this packet is about BUG-045-002 carry-forward evidence records, not global test doctrine. |
| Amend spec 023 directly | Rejected for this packet | Spec 023 adversarial-regression doctrine already informs TR-009. Changing the doctrine is not required to plan product evidence hardening here. |
| Split TR-008..012 into separate specs | Rejected | The records share source truth, consumer surfaces, wrapper disposition, and boundary proof. Splitting would fragment auditability. |
| Create `specs/054-artifact-output-summarization` now | Rejected | The user explicitly reserved that identifier as a later idea. This packet must not create it. |
| Treat TR-008 as automatic new work | Rejected | Current traceability proof may show no residual gaps. Inventing work would violate the no-invented-gap requirement. |
| Pull TR-014 into Smackerel product scope | Rejected | Framework-managed files are upstream-owned and immutable in downstream product repos. |

## Risks And Mitigations

| Risk | Mitigation |
|------|------------|
| Stale G068 evidence becomes invented work. | Scope 1 starts with current traceability evidence and supports closure-by-proof. |
| Regression expansion duplicates existing proof. | Scope 2 requires every row to name a new protected failure mode. |
| Consumer trace misses indirect evidence consumers. | Scope 3 requires direct, indirect, observational, and documentation-facing classes. |
| Shared-infrastructure changes validate only themselves. | Scope 4 requires downstream canaries for protected contracts. |
| G040 wrappers linger without ownership. | Scope 5 requires a wrapper disposition record for each predecessor region. |
| Framework work leaks into Smackerel. | TR-014 is excluded and framework-owned paths are listed as product no-edit. |
| Planning packet accidentally changes source. | Boundary records and no-source-delta proof are required before validation. |

## Open Questions

No design-blocking questions remain. The first unresolved factual decision is Scope 1's current traceability result for TR-008; it belongs to the planning or validation owner that executes the guard and records the output.
-->
# Design: CI Ops Evidence Hardening

## Design Brief

### Current State

BUG-045-002 is closed as `done_with_concerns`: the chronic CI integration job failure was fixed by routing CI through the canonical `./smackerel.sh test integration` path and protecting that topology with `internal/deploy/ci_integration_topology_contract_test.go`. The predecessor packet still carries five product-owned planning transition requests, `TR-BUG-045-002-008` through `TR-BUG-045-002-012`, plus one framework-owned request, `TR-BUG-045-002-014`.

Spec 053 exists to consolidate only the five product-owned planning requests into one ops evidence-hardening packet. It must not reopen the original CI outage, and it must not create `specs/054-artifact-output-summarization`.

### Target State

The target design is a planning architecture that lets `bubbles.plan` produce five evidence-driven scopes without touching runtime or source files. The packet defines the records, matrices, proof gates, owner boundaries, and verification expectations needed to decide what to evidence, what to close by proof, and what to route away from Smackerel product scope.

### Patterns To Follow

- Use the consolidated transition-request model already present in `specs/053-ci-ops-evidence-hardening/spec.md` and the prior transition records in `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/state.json`.
- Preserve the trace-ID anchor approach described in BUG-045-002 `report.md` for G068 fidelity: anchor planning rows to scenario IDs instead of rewriting Gherkin or DoD prose to satisfy fuzzy matching.
- Use source-qualified evidence categories from `.github/agents/bubbles_shared/evidence-rules.md`: `executed`, `interpreted`, and `not-run`.
- Use consumer-impact and shared-infrastructure planning rules from `.github/agents/bubbles_shared/consumer-trace.md`, `.github/agents/bubbles_shared/test-fidelity.md`, and `.github/agents/bubbles_shared/critical-requirements.md`.

### Patterns To Avoid

- Do not amend BUG-045-002 directly as the active work surface; it is an audited predecessor packet and should remain a historical source of truth.
- Do not patch `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, `.github/prompts/`, or other framework-managed install artifacts in this downstream repository.
- Do not plan runtime/source changes before proof exists that a source change is required; this packet is planning-only under `spec-scope-hardening`.
- Do not invent residual G068 gaps. Absence of a residual gap after a current traceability check is a valid closure outcome for TR-008.

### Resolved Decisions

- TR-008 through TR-012 stay in one consolidated spec, `specs/053-ci-ops-evidence-hardening`.
- TR-014 remains framework-owned and appears here only as a boundary note.
- TR-008 uses a proof-before-work gate: current traceability evidence is required before any residual G068 work is scoped.
- Regression expansion must add protective value beyond the already-passing topology guard, three adversarial sub-tests, and local Path-A reproduction from BUG-045-002.
- The design uses records and matrices rather than code changes: TR matrix, source-surface matrix, provenance categories, consumer inventory, blast-radius records, boundary records, and wrapper disposition records.

### Open Questions

- No blocking design questions remain. `bubbles.plan` must still populate concrete scope rows, Test Plan paths, and evidence commands from the models defined below.

## Purpose And Scope

This design defines the technical planning architecture for Smackerel spec 053. It converts five BUG-045-002 product carry-forward transition requests into one coherent packet of planning surfaces:

| Surface | Transition Request | Design Outcome |
|---------|--------------------|----------------|
| G068 proof-or-close | `TR-BUG-045-002-008` | Current traceability evidence determines whether residual work exists. |
| Regression expansion | `TR-BUG-045-002-009` | New regression rows must protect CI integration job, full-stack reproduction, or contract-test surfaces beyond existing proof. |
| Consumer trace | `TR-BUG-045-002-010` | First-party CI workflow consumers are inventoried and dispositioned. |
| Shared infrastructure blast radius | `TR-BUG-045-002-011` | Protected shared surfaces get canaries and restore expectations before broad validation. |
| Change boundary and G040 disposition | `TR-BUG-045-002-012` | Allowed file families, excluded surfaces, and wrapper handling are explicit. |

This packet is planning-only. It creates a design and hands the next step to `bubbles.plan`; no runtime, CI workflow, test, deploy, source, or framework-managed file is part of the active design output.

## Current Truth

### Source Packet Status

BUG-045-002 reached `done_with_concerns`, not a bare `done`, because the original CI failure was fixed and verified while low-severity planning concerns remained. The source packet's state records:

- `TR-BUG-045-002-008` through `TR-BUG-045-002-012` are open and owned by `bubbles.plan`.
- `TR-BUG-045-002-014` is open and owned by `bubbles.workflow` for framework guard maintenance.
- `certification.concerns[]` mirrors the five product planning requests plus TR-014.
- `certification.knownDrift[]` notes that the product items are routed planning work, not evidence that the original CI failure remains broken.

### Fixed Baseline From BUG-045-002

The design treats the following as already proven by BUG-045-002 and therefore not subject to relitigation in spec 053:

| Fixed Fact | Source In BUG-045-002 | Design Implication |
|------------|------------------------|--------------------|
| CI integration uses the canonical CLI path. | CI workflow refactor and validation evidence. | Regression expansion must not duplicate the same proof as a new claim. |
| The topology guard exists and passed. | `internal/deploy/ci_integration_topology_contract_test.go` evidence. | Any added contract surface must complement, not replace, this guard. |
| Three adversarial topology sub-tests passed. | Scope 2 regression evidence. | New regression rows need a distinct failure mode. |
| Local Path-A integration reproduction passed. | Scope 3 reproduction evidence. | Full-stack reproduction expansion must describe additional observation value. |
| Traceability guard showed 11/11 mappings after the trace-ID anchor plan re-entry. | Report and state audit references. | TR-008 must first prove a residual G068 gap exists before planning new work. |
| `done_with_concerns` was caused by routed planning concerns, not by a live outage. | Finalize verdict and certification concerns. | Spec 053 stays in planning scope and does not reopen the bug. |

### Product Boundary

Smackerel owns the planning artifacts under `specs/053-ci-ops-evidence-hardening/`. The product packet may inspect product-owned CI workflow, runtime CLI, docs, existing specs, and test artifacts for planning evidence, but this design does not authorize editing those surfaces.

### Framework Boundary

TR-014 concerns state-transition-guard behavior, including G061 counting and G022 phase-provenance modeling. Those scripts and shared docs are framework-managed install artifacts in this downstream repo. The only valid Smackerel action is to record the framework boundary and route TR-014 to upstream Bubbles work.

## Architecture Overview

Spec 053 uses a record-oriented planning architecture. Each scope produced later by `bubbles.plan` must consume the same canonical source facts and emit a structured disposition rather than free-form prose.

```text
BUG-045-002 report/state
        |
        v
Spec 053 spec.md requirements
        |
        v
Spec 053 design.md record models
        |
        v
Spec 053 scopes.md planning rows
        |
        v
Later evidence from owner phases
```

The design has four control principles:

1. Source-qualified claims: every planning claim points to BUG-045-002 `report.md`, BUG-045-002 `state.json`, spec 053 `spec.md`, or later executed evidence.
2. Proof before work: TR-008 and any other suspected gap starts with evidence, not assumption.
3. Matrix-first planning: TRs, surfaces, consumers, infrastructure, boundaries, and wrappers are captured as records that can be checked by reviewers.
4. No source mutation by design/plan: planning artifacts define evidence expectations; implementation or hardening owners act only if a later scope explicitly authorizes code changes.

## Design Decisions

### DD-053-001: One Consolidated Product Spec

TR-008 through TR-012 remain in `specs/053-ci-ops-evidence-hardening`. Splitting them into separate feature folders would fragment the shared evidence context: the same BUG-045-002 finalization packet, the same G040 wrappers, the same CI workflow evidence, and the same framework boundary inform all five requests.

The consolidated spec avoids cross-spec drift by requiring one TR matrix and one source-surface matrix. `specs/054-artifact-output-summarization` remains uncreated and reserved as a separate idea.

### DD-053-002: G068 Proof-Before-Work Gate

TR-008 cannot become new scope content until a current traceability result proves residual G068 gaps. The planner must record one of two dispositions:

| Disposition | Required Evidence | Planning Result |
|-------------|-------------------|-----------------|
| `residual-gap-list` | Current traceability output names unmapped or weakly mapped scenarios. | Plan exact gap rows with scenario ID, affected artifact, missing fidelity claim, and closure evidence. |
| `closure-by-evidence` | Current traceability output confirms no residual G068 gap for the current scenario set. | Close TR-008 in the planning packet without creating artificial work. |

This preserves the BUG-045-002 trace-ID anchor decision and prevents prose churn that rewrites DoD items merely to satisfy a string-matching gate.

### DD-053-003: Regression Expansion Boundaries

TR-009 covers only protective regression expansion beyond existing BUG-045-002 evidence. The allowed regression surfaces are:

- CI integration job evidence shape.
- Full-stack reproduction path evidence shape.
- Contract-test surface around CI topology and sibling workflow contracts.

A candidate regression row is rejected if its only claim is that the already-existing topology guard or Path-A reproduction still passes. A candidate row is accepted only when it names a distinct failure mode, an observable result, and the evidence source that would fail if that mode returns.

### DD-053-004: Consumer Trace Model

TR-010 uses a consumer inventory record. Consumers are not limited to code imports. For this packet, a consumer is any first-party surface that depends on the CI integration job, the canonical `./smackerel.sh test integration` command, the topology guard, the evidence shape, or the BUG-045-002 carry-forward decision.

Consumer classes:

| Class | Definition | Example Surface Types |
|-------|------------|-----------------------|
| `direct` | Executes or enforces the CI integration path. | Workflow job, runtime CLI wrapper, contract test. |
| `indirect` | Infers CI health or topology from another artifact. | Specs, state cross-references, managed docs, validation scripts. |
| `observational` | Reads CI evidence but does not execute the path. | Reports, audit packets, runbook references. |
| `owner-routed` | Belongs to a different owner or upstream framework. | Bubbles guard behavior for TR-014. |

Each consumer receives one of three dispositions: `required-change`, `no-change-with-evidence`, or `owner-routed`.

### DD-053-005: Shared Infrastructure Blast-Radius Model

TR-011 treats test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrapper behavior as protected shared infrastructure. Any later scope that authorizes a change to one protected surface must first define:

- the dependent contracts likely to cascade silently;
- a canary check that validates those contracts independently;
- the broad validation trigger that runs after the canary passes;
- a restore path for artifact or source deltas.

The canary must assert downstream behavior, not just the modified fixture or wrapper itself.

### DD-053-006: Change Boundary Model

TR-012 requires every scope to declare allowed file families and excluded surfaces before any owner phase acts. For planning-only scopes, allowed files are limited to spec 053 artifacts unless the planner explicitly defines a later execution scope with a different boundary.

The boundary record must include a no-source-delta proof expectation. At planning close-out, the proof should show that only spec 053 artifacts changed. If a later scope authorizes source changes, that scope's boundary must list every allowed source path and every excluded sibling surface.

### DD-053-007: G040 Wrapper Disposition Model

BUG-045-002 used `<!-- bubbles:g040-skip-begin -->` / `<!-- bubbles:g040-skip-end -->` wrappers around routed planning content in the report. Spec 053 does not remove those wrappers during design. Later planning must choose a disposition for each wrapper that contains product-owned planning content:

| Disposition | Meaning | Required Evidence |
|-------------|---------|-------------------|
| `retain-historical` | Wrapper remains in BUG-045-002 as historical audit context. | Cross-reference to spec 053 scope/disposition. |
| `retain-with-cross-reference` | Wrapper remains and points to the current spec 053 planning record. | Evidence that the target planning record exists. |
| `remove-after-capture` | Wrapper is removed by the owning artifact phase after content is captured elsewhere. | Diff proof plus artifact-lint/trace checks showing no loss of required evidence. |
| `owner-routed` | Wrapper content belongs to framework or another owner. | Owner and routing record. |

Design and plan phases for spec 053 should prefer `retain-historical` or `retain-with-cross-reference` unless a later owner has explicit authorization to edit BUG-045-002.

### DD-053-008: G040 Wrapper Disposition Does Not Equal Runtime Work

Wrapper decisions are artifact lifecycle decisions. They do not imply CI, runtime, test, CLI, or deploy changes. Any scope that tries to convert wrapper cleanup into source work must be rejected unless the source need is independently proven and explicitly planned.

### DD-053-009: Framework Boundary For TR-014

TR-014 is not represented as a product scope. Product artifacts may mention it only to preserve routing truth. Edits to `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, `.github/bubbles/workflows.yaml`, `.github/prompts/`, and `.github/skills/bubbles-*` must be handled upstream in the canonical Bubbles repository.

## Data And Artifact Model

### TR Matrix Record

Each transition request row in `scopes.md` should carry this shape:

| Field | Required | Description |
|-------|----------|-------------|
| `transitionRequestId` | Yes | One of TR-008..TR-012. TR-014 appears only in a framework boundary table. |
| `sourceArtifact` | Yes | BUG-045-002 `report.md` section, BUG-045-002 `state.json` field, or spec 053 requirement. |
| `sourceClaim` | Yes | The exact claim being planned. |
| `scenarioIds` | Yes | SCN-053 IDs covered by the row. |
| `requirementIds` | Yes | FR-053 IDs covered by the row. |
| `planningDisposition` | Yes | `scope-required`, `closure-by-evidence`, `no-change-with-evidence`, or `owner-routed`. |
| `evidenceExpected` | Yes | Evidence category and command or artifact expected later. |
| `owner` | Yes | Owning phase or agent for the next action. |

### Source-Surface Matrix

The source-surface matrix prevents ambiguous references to “CI evidence” by naming the exact surface and its product/framework ownership.

| Surface ID | Surface | Owner | Mutable In Spec 053? | Evidence Role |
|------------|---------|-------|----------------------|---------------|
| `SS-053-001` | BUG-045-002 `report.md` | Historical bug packet | No by design phase | Source truth and wrapper inventory. |
| `SS-053-002` | BUG-045-002 `state.json` | Historical bug packet | No by design phase | TR and concern source truth. |
| `SS-053-003` | Spec 053 `spec.md` | `bubbles.analyst` | No by design phase | Business requirements. |
| `SS-053-004` | Spec 053 `design.md` | `bubbles.design` | Yes | Technical planning architecture. |
| `SS-053-005` | Spec 053 `scopes.md` | `bubbles.plan` | Not yet present | Scope and DoD plan. |
| `SS-053-006` | `.github/workflows/ci.yml` | Product source | No in this packet | Observed CI topology surface only. |
| `SS-053-007` | `internal/deploy/*ci*topology*` tests | Product source | No in this packet | Existing contract-test reference only. |
| `SS-053-008` | `scripts/runtime/go-*.sh` wrappers and `./smackerel.sh` | Product runtime CLI surface | No in this packet | Existing CLI contract reference only. |
| `SS-053-009` | `.github/bubbles/scripts/*` and shared Bubbles docs | Framework install artifacts | No | TR-014 boundary only. |

### Evidence Provenance Categories

Spec 053 uses the Bubbles evidence taxonomy without inventing new proof classes.

| Category | Use In This Packet | Completion Meaning |
|----------|--------------------|--------------------|
| `executed` | Artifact-lint, traceability guard, source-surface scans, no-source-delta proof, and any later tests. | Raw command output directly proves the claim. |
| `interpreted` | Design, source mapping, owner routing, and model decisions. | Reasoning is documented; audit must review interpretation. |
| `not-run` | Any verification that cannot be executed in the owning phase. | Cannot close a checked DoD item; must remain an uncertainty until resolved. |

### Consumer Inventory Record

The consumer inventory table produced by `bubbles.plan` should use this structure:

| Field | Required | Description |
|-------|----------|-------------|
| `consumerId` | Yes | Stable ID such as `CON-053-001`. |
| `consumerName` | Yes | Human-readable surface name. |
| `consumerClass` | Yes | `direct`, `indirect`, `observational`, or `owner-routed`. |
| `sourceSurfaceIds` | Yes | One or more source surfaces consumed. |
| `dependencyClaim` | Yes | What assumption the consumer makes. |
| `staleReferenceCheck` | Yes | Read-only scan or artifact proof expected later. |
| `disposition` | Yes | `required-change`, `no-change-with-evidence`, or `owner-routed`. |
| `evidenceExpected` | Yes | Executed or interpreted evidence required for closure. |

Minimum consumers to consider:

- GitHub Actions CI integration job.
- Canonical `./smackerel.sh test integration` command docs and wrappers.
- Existing CI topology contract test.
- BUG-045-002 state/report references.
- Managed docs that mention `./smackerel.sh test integration`.
- Spec 031 live-stack-testing contract if cited by scope content.
- Spec 023 adversarial-regression precedent if cited by regression rows.
- Framework guard behavior as owner-routed, not product-owned.

### Blast-Radius Record

The blast-radius record should be created for every protected shared surface.

| Field | Required | Description |
|-------|----------|-------------|
| `surfaceId` | Yes | Source surface or newly planned surface. |
| `protectedContract` | Yes | Contract that must not drift. |
| `dependencyDirection` | Yes | Upstream/downstream relationship. |
| `canaryEvidence` | Yes | Narrow check that proves the contract before broad validation. |
| `broadValidation` | Yes | Broader validation command or artifact required after canary success. |
| `restorePath` | Yes | How to remove or revert the change if the surface is modified later. |
| `excludedSurfaces` | Yes | Sibling surfaces that must remain unchanged. |

Protected surfaces for TR-011:

- Test stack lifecycle: config generation, test Compose project, health/status, teardown.
- CI workflow ordering: build dependency, stack up/status/test/down, artifact upload, failure gate.
- Contract-test parsing: workflow YAML parsers and adversarial fixtures.
- CLI wrappers: `./smackerel.sh` command dispatch and `scripts/runtime/go-*.sh` wrappers.

### Boundary Record

Each scope must carry a boundary record.

| Field | Required | Description |
|-------|----------|-------------|
| `scopeId` | Yes | Planned scope identifier. |
| `allowedArtifacts` | Yes | Files the scope may modify. |
| `excludedArtifacts` | Yes | Files or families the scope must not modify. |
| `owner` | Yes | Agent that may perform the modifications. |
| `noSourceDeltaProof` | Yes | Expected proof that excluded source surfaces did not change. |
| `expansionRule` | Yes | What evidence and owner approval would be required to expand the boundary. |

For the design and plan phases, allowed artifacts should be limited to `specs/053-ci-ops-evidence-hardening/design.md`, `state.json` execution fields, `report.md` phase notes, and later `scopes.md`/planner-owned files. Runtime/source families are excluded.

### Wrapper Disposition Record

Each BUG-045-002 G040 wrapper relevant to product carry-forward content should receive one row.

| Field | Required | Description |
|-------|----------|-------------|
| `wrapperId` | Yes | Stable ID such as `G040-053-001`. |
| `sourceArtifact` | Yes | BUG-045-002 report section or line range reference in prose. |
| `containedClaim` | Yes | Planning concern isolated by the wrapper. |
| `transitionRequestId` | Yes | TR-008..TR-012, or `TR-014` when framework-owned. |
| `disposition` | Yes | `retain-historical`, `retain-with-cross-reference`, `remove-after-capture`, or `owner-routed`. |
| `owner` | Yes | Agent allowed to act. |
| `evidenceExpected` | Yes | Artifact proof needed to close the disposition. |

## Workflow And Phase Ownership

### Analyst

`bubbles.analyst` owns the business requirements already present in `spec.md`: actor definitions, use cases, Gherkin scenarios, functional requirements, success metrics, risk framing, and the traceability matrix.

### Design

`bubbles.design` owns this file. Design defines the record models, planning architecture, source boundaries, framework boundary, and verification strategy. Design may update `state.json.execution` and append a design-phase note to `report.md`; it must not edit certification state or create scopes.

### Plan

`bubbles.plan` is the next required owner. Plan creates `scopes.md` and any planner-owned machine-readable plan artifacts if the repo workflow requires them. It must instantiate the five scope architectures below, preserve the consolidated spec boundary, and maintain Test Plan / Gherkin / DoD parity.

### Harden

`bubbles.harden` may classify evidence-hardening findings after planning exists. It may not invent scopes or edit planner-owned structures. Any finding that requires plan changes routes back to `bubbles.plan`.

### Docs

`bubbles.docs` acts only if a planned scope proves managed docs must change. Design does not pre-authorize doc edits because BUG-045-002 already found the user-facing canonical CLI documented.

### Validate

`bubbles.validate` verifies artifact lint, traceability, source-surface mapping, no-source-delta proof, and any planned evidence outputs after scopes exist. It owns certification state; design and plan do not.

### Audit

`bubbles.audit` independently checks the final planning packet for source-grounded claims, provenance tags, no invented gaps, no source-boundary violations, and correct framework routing.

### Finalize

Finalization remains workflow-owned. It may set final status only after the owning validation and audit phases complete.

## Scope Architecture

`bubbles.plan` should produce five scopes aligned to the product TR set. Each scope should include scenario IDs, Test Plan rows, DoD items, and the record outputs named here.

### Scope 1: G068 Proof-Or-Close Matrix

**TR:** `TR-BUG-045-002-008`

**Purpose:** Determine whether residual G068 work exists after the BUG-045-002 trace-ID anchor fix.

**Required Records:** TR matrix row, source-surface row, evidence provenance category, proof disposition.

**Validation Shape:** Run traceability guard against the current planning artifact set when scopes exist. If no residual gap exists, record `closure-by-evidence`; if gaps exist, list each gap with scenario ID and exact closure expectation.

### Scope 2: Regression Expansion Boundary Plan

**TR:** `TR-BUG-045-002-009`

**Purpose:** Define regression expansion only where it protects a failure mode not already covered by the topology guard, adversarial tests, or local Path-A reproduction.

**Required Records:** TR matrix row, regression candidate table, evidence provenance category, duplicate-rejection rationale.

**Validation Shape:** Every proposed row names one surface (`ci-integration-job`, `full-stack-reproduction`, or `contract-test-surface`), one distinct failure mode, and one observable assertion that would fail if the failure mode returns.

### Scope 3: Consumer Trace Inventory

**TR:** `TR-BUG-045-002-010`

**Purpose:** Inventory first-party consumers of CI workflow evidence and canonical integration-test behavior.

**Required Records:** Consumer inventory records and source-surface matrix entries.

**Validation Shape:** Stale-reference scans and owner dispositions for direct, indirect, observational, and owner-routed consumers. Each consumer receives `required-change`, `no-change-with-evidence`, or `owner-routed`.

### Scope 4: Shared Infrastructure Blast-Radius Plan

**TR:** `TR-BUG-045-002-011`

**Purpose:** Protect shared test stack, CI workflow ordering, contract-test parsing, and CLI wrappers from collateral changes.

**Required Records:** Blast-radius records and canary definitions.

**Validation Shape:** Each protected surface defines a contract, canary, broad validation trigger, restore path, and excluded sibling surfaces.

### Scope 5: Boundary And G040 Wrapper Disposition

**TR:** `TR-BUG-045-002-012`

**Purpose:** Define file-family boundaries and lifecycle treatment for BUG-045-002 G040 wrappers containing product planning content.

**Required Records:** Boundary records and wrapper disposition records.

**Validation Shape:** Boundary proof shows only authorized artifacts changed. Wrapper rows record retain/cross-reference/remove/owner-routed disposition. TR-014 appears only as framework-owned context.

## API And Contract Considerations

This feature does not introduce runtime APIs, HTTP contracts, protobuf schemas, database tables, queues, services, or UI routes. The relevant contracts are planning and evidence contracts:

| Contract | Producer | Consumer | Compatibility Rule |
|----------|----------|----------|--------------------|
| TR matrix | `bubbles.plan` | validate/audit/workflow | Every TR-008..012 has one current disposition. |
| Source-surface matrix | `bubbles.plan` | all later phases | Every claim references a named source surface. |
| Evidence provenance | all owner phases | validate/audit | Every evidence block uses `Claim Source`. |
| Consumer inventory | `bubbles.plan` | harden/validate/audit | Every consumer receives a disposition. |
| Blast-radius record | `bubbles.plan` | implement/harden/validate | Protected shared surfaces have canaries before broad validation. |
| Boundary record | `bubbles.plan` | all execution phases | File changes stay inside the allowed family. |
| Wrapper disposition | `bubbles.plan` | docs/validate/audit | G040 wrappers are retained, cross-referenced, removed by owner, or routed. |

## Security, Compliance, And Privacy

Spec 053 is artifact-only and introduces no new data handling path. Security concerns are about governance integrity:

- No secret values should be introduced into planning artifacts.
- No environment-specific hostnames, real IPs, or operator-private topology should be added.
- Evidence captured later should use repo-relative paths where possible.
- Framework-managed files must not be patched in this downstream repo.
- Any CI or GitHub Actions evidence captured later must avoid tokens and must not instruct bypassing hooks or validation.

## Configuration And Migration Strategy

No configuration values, generated env files, Docker Compose files, migrations, or deployment manifests change under this design.

If a later scope discovers that a configuration or runtime file truly must change, that scope must expand its boundary explicitly, record the source evidence that requires the change, and route implementation to the correct owner. Until that happens, spec 053 is constrained to planning artifacts.

## Observability And Failure Handling

The feature observes planning and CI-evidence health rather than runtime health. Failure handling is expressed as dispositions:

| Failure Mode | Detection | Handling |
|--------------|-----------|----------|
| Residual G068 gap exists. | Traceability guard names unmapped scenario(s). | Scope exact gap rows with scenario and DoD/test closure expectation. |
| No residual G068 gap exists. | Traceability guard passes current scenario set. | Close TR-008 by evidence without creating new work. |
| Regression row duplicates existing proof. | Row maps only to already-passing BUG-045-002 guard/repro evidence. | Reject row from active scope. |
| Consumer cannot be classified. | Consumer inventory lacks class or disposition. | Keep scope incomplete until classified. |
| Protected infrastructure lacks canary. | Blast-radius record missing canary. | Do not authorize source changes. |
| Excluded surface changes. | No-source-delta proof shows source/runtime/framework file changes. | Block closure and route to owner for boundary correction. |
| Framework-owned issue appears in product scope. | TR-014 or framework files appear as product work. | Remove from product scope and route to upstream Bubbles. |

## Testing And Validation Strategy

This design does not define runtime tests because no runtime behavior changes. It defines validation requirements for the planning packet.

| Validation Target | Command Or Evidence Type | Owner | Expected Result |
|-------------------|--------------------------|-------|-----------------|
| Artifact structure | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | validate/audit | Exits 0 after plan artifacts exist. |
| G068 mapping | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` when scopes exist | plan/validate | Exits 0 or records exact residual gaps. |
| Source-surface mapping | Read-only scan of planned source surfaces and references | plan/validate | Every matrix row references an existing artifact or owner-routed framework surface. |
| No-source-delta proof | `git diff --name-status` from the Smackerel repo root | validate/audit | Changed files are limited to allowed spec 053 artifacts for planning-only phases. |
| Consumer trace | Stale-reference scans selected by `bubbles.plan` | validate/audit | Every consumer has a disposition and no stale reference remains unaccounted. |
| Blast-radius canaries | Scope-defined canary commands | validate/audit | Canary passes before broad validation is accepted. |
| Boundary and wrapper disposition | Boundary table plus wrapper disposition table | audit | Every wrapper and file family has an owner and closure rule. |

Evidence expectations:

- Executed validations require raw terminal output in the owning evidence artifact.
- Interpreted design conclusions must include an interpretation line when recorded as evidence.
- `not-run` evidence cannot close a checked DoD item.
- Read-only file inspection is useful for planning, but it is not a substitute for executing required validation scripts.

## Alternatives Considered

### Amend BUG-045-002 Directly

Rejected. BUG-045-002 is the audited source packet and should remain stable as historical evidence. Amending it directly would mix current planning with prior close-out evidence, increasing the chance of confusing the original fix proof with the successor planning records.

### Amend Spec 031 Directly

Rejected. Spec 031 is a live-stack-testing contract source that BUG-045-002 used as design precedent. The carry-forward items here are not changes to the live-stack-testing doctrine; they are evidence-hardening planning concerns around one CI incident packet.

### Amend Spec 023 Directly

Rejected. Spec 023 is an adversarial-regression precedent. TR-009 may cite that principle, but the concrete regression expansion belongs in spec 053 because it is scoped to BUG-045-002 CI evidence, not a general rewrite of regression policy.

### Create Five Separate Specs

Rejected. The five product TRs share the same source packet, source surfaces, G040 wrapper context, and framework boundary. Splitting them would require duplicate source matrices and would make no-invented-gap review harder.

### Create `specs/054-artifact-output-summarization`

Rejected for this invocation. The identifier remains uncreated and separate from the five BUG-045-002 product TRs consolidated here.

### Patch Framework Guard Files In Smackerel

Rejected. TR-014 is framework-owned. Downstream framework-managed files are install artifacts and must not be edited directly in Smackerel.

## Explicit Change Boundaries

### Allowed During Design Phase

- `specs/053-ci-ops-evidence-hardening/design.md`
- `specs/053-ci-ops-evidence-hardening/state.json` execution fields and design execution-history entry
- `specs/053-ci-ops-evidence-hardening/report.md` design-phase note

### Expected During Plan Phase

- `specs/053-ci-ops-evidence-hardening/scopes.md`
- Planner-owned scenario/test-plan artifacts if the current Bubbles workflow requires them
- `state.json` execution fields for plan phase
- `report.md` plan-phase note
- `uservalidation.md` only if `bubbles.plan` owns and updates acceptance checklist content

### Excluded Unless A Later Scope Explicitly Expands The Boundary

- `.github/workflows/ci.yml`
- `internal/`
- `cmd/`
- `scripts/runtime/`
- `docker-compose*.yml`
- `deploy/`
- `config/`
- managed docs outside the spec folder

### Always Excluded In This Product Spec

- `.github/bubbles/scripts/`
- `.github/agents/bubbles_shared/`
- `.github/agents/bubbles.*.agent.md`
- `.github/prompts/bubbles.*.prompt.md`
- `.github/bubbles/workflows.yaml`
- `.github/instructions/bubbles-*.instructions.md`
- `.github/skills/bubbles-*/`

## Framework Boundary

TR-014 remains a framework concern. Spec 053 records the boundary so product owners do not accidentally absorb the work. Any true fix for G061 counting, G022 phase provenance, or state-transition-guard false positives must be made in the canonical Bubbles repository and then installed into downstream repos through the supported upgrade path.

The product packet may include a row that says `TR-BUG-045-002-014 -> owner-routed -> bubbles.workflow`, but it must not create a Smackerel scope that edits framework install artifacts.

## Open Questions And Decision Path

| Question | Blocking? | Decision Path |
|----------|-----------|---------------|
| Which exact stale-reference scans should be used for consumer trace? | No | `bubbles.plan` selects scan commands from the consumer inventory records. |
| Should any BUG-045-002 G040 wrapper be removed rather than retained with a cross-reference? | No | Scope 5 records wrapper disposition; removal requires owning artifact-phase authorization and evidence. |
| Does TR-008 have a residual G068 gap after current planning artifacts exist? | Yes for TR-008 closure | Scope 1 runs traceability guard after scopes exist and records residual-gap-list or closure-by-evidence. |
| Is any runtime/source change needed? | No at design time | A later scope must prove need and expand the boundary before implementation. |

## Design Completion Statement

This design creates the planning architecture for spec 053 and leaves the packet in `in_progress`. It does not mark scopes or the spec done, does not create `scopes.md`, does not create spec 054, and does not authorize source or framework file changes. The next required owner is `bubbles.plan`.