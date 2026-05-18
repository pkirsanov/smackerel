# Scopes: CI Ops Evidence Hardening

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

This spec is an artifact-only operations planning packet (per [design.md](design.md) → "Configuration And Migrations" and "Change Boundaries"). No runtime, source, CI workflow, deploy adapter, contract test, CLI wrapper, framework script, framework agent, framework instruction, or framework skill file changes inside any scope of this packet. Every scope produces planning records under `specs/053-ci-ops-evidence-hardening/` and read-only references to the predecessor evidence at `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/`.

Five sequential scopes, ordered per [design.md](design.md) → "Scope Architecture":

1. **Scope 1 (G068 Proof-Or-Close)** — Execute or reference a current traceability result for the BUG-045-002 artifacts and decide whether residual G068 work exists. Produce a TR matrix row plus exactly one of: a `residual-gap-found` record, a `closed-by-current-proof` record, or an `owner-routed-tool-issue` framework-boundary route. No invented gaps; closure by current proof is a valid outcome.
2. **Scope 2 (Regression Expansion Boundaries)** — Define only those regression scenarios that add protection beyond the BUG-045-002 topology guard, adversarial contract tests, and local full-stack reproduction proof. Every planned regression row must name its protected failure mode and its surface (CI integration job, full-stack reproduction path, or contract-test surface). Rows that duplicate existing proof are rejected.
3. **Scope 3 (Consumer Trace Inventory)** — Inventory direct, indirect, observational, and documentation-facing consumers of the CI integration workflow and its evidence shape. Every consumer receives exactly one disposition: `required-change`, `no-change-with-evidence`, or `owner-routed`. Stale-reference scans support each disposition.
4. **Scope 4 (Shared Infrastructure Blast Radius)** — Identify protected contracts for test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrappers. Each protected surface gets a canary check, a broad-validation trigger, and a rollback/restore expectation. Broad validation is not authorized for any change until that surface's canary is named.
5. **Scope 5 (Boundary And Wrapper Disposition)** — Define allowed/excluded artifact surfaces per scope, capture no-source-delta proof, record each BUG-045-002 G040 wrapper's disposition (`historical-retain`, `cross-reference-retain`, or `owner-remove`), and record the framework-boundary record routing TR-BUG-045-002-014 upstream to `bubbles.workflow` / the canonical Bubbles repository without editing any framework-managed install artifact in this Smackerel repo.

### New Types & Signatures

This packet introduces no runtime types, no schemas, no APIs, no DB tables, and no UI screens. It introduces **artifact-only record types** defined in [design.md](design.md) → "Data And Artifact Model":

- `TR Matrix Row` — fields: `trId`, `sourceArtifact`, `sourceClaim`, `scenarioIds`, `requirementIds`, `plannedRecordType`, `disposition`, `evidenceExpectation`.
- `Source-Surface Matrix Row` — fields: `surfaceId`, `path`, `surfaceKind`, `relationship`, `allowedAction`, `proofRequired`.
- `Evidence Provenance Tag` — values: `executed`, `interpreted`, `not-run`, `predecessor-source`, `owner-routed`, `closure-by-proof`.
- `Consumer Inventory Record` — fields: `consumerId`, `pathOrSurface`, `consumerClass`, `consumedSignal`, `staleRisk`, `disposition`, `evidenceRef`, `owner`.
- `Blast-Radius Record` — fields: `surfaceId`, `protectedContract`, `dependentSurfaces`, `canaryCheck`, `broadValidationTrigger`, `rollbackOrRestore`, `evidenceExpectation`.
- `Boundary Record` — fields: `scopeId`, `allowedFileFamilies`, `excludedSurfaces`, `allowedChangeType`, `noSourceDeltaProof`, `owner`.
- `Wrapper Disposition Record` — fields: `wrapperId`, `predecessorLocation`, `containedClaim`, `mappedTrId`, `mappedScopeId`, `disposition`, `crossReferenceRequired`, `evidenceExpectation`.
- `Framework-Boundary Record` — fields: `trId`, `routedOwner`, `frameworkArtifactPaths` (no-edit list), `productEvidenceCitationOnly` (true), `crossRepoFollowUp`.

### Validation Checkpoints

- After **Scope 1**: `traceability-guard.sh` evidence is captured AND TR-008 has exactly one of the three first-disposition outcomes recorded. Scope 2 cannot start until Scope 1's G068 truth is known, because invented residual-gap rows would silently land in Scope 2 regression planning.
- After **Scope 2**: Every regression row references one of the three allowed surfaces and names the new failure mode it protects beyond existing BUG-045-002 evidence. No duplicate rows. `artifact-lint.sh` exits 0 on the updated planning artifacts.
- After **Scope 3**: Every identified CI workflow consumer has a class and a disposition. No consumer is `unclassified` or missing. Stale-reference scans recorded for any signal slated to change.
- After **Scope 4**: Every protected shared-infrastructure surface (test stack lifecycle, CI workflow ordering, contract-test parsing, CLI wrappers) has a named canary check and a broad-validation trigger. No protected surface lacks a canary.
- After **Scope 5**: Boundary records exist for every scope (1-5); no-source-delta proof command is named; every G040 wrapper in the BUG-045-002 report has a disposition; the framework-boundary record routes TR-014 upstream without naming a Smackerel framework-edit action.

---

## Cross-Scope Dependencies

| From | To | Dependency Reason |
|------|----|--------------------|
| Scope 1 | Scope 2 | Scope 1's G068 proof-or-close outcome controls whether any TR-008 residual-gap row is allowed to influence Scope 2 regression planning. If Scope 1 closes TR-008 by current proof, Scope 2 must not invent regression rows that re-prove the same closed surface. |
| Scope 3 | Scope 4 | Consumer inventory (direct, indirect, observational, documentation-facing) must precede any evidence-shape change in Scope 4's blast-radius planning. A protected-surface canary cannot be valid if an unknown consumer depends on the evidence shape it is meant to protect. |
| Scope 1, Scope 2, Scope 3, Scope 4 | Scope 5 | Scope 5 records the final containment: boundary records reference scope IDs 1-4; wrapper disposition records reference the TRs covered in scopes 1-4; framework-boundary record preserves TR-014 exclusion across all prior scopes. Scope 5 cannot finalize boundary records before the four upstream scopes have produced the records it must reference. |

The dependency direction is strict: a downstream scope cannot move from `Not Started` to `In Progress` until every upstream scope it depends on is `Done`.

---

## Scope 1: G068 Proof-Or-Close

**Status:** Not Started
**Primary TR:** TR-BUG-045-002-008
**Depends On:** None
**Owner phases:** `bubbles.plan` (record authorship); `bubbles.harden` (residual-gap routing if any); `bubbles.validate` (executes `traceability-guard.sh` and captures evidence); `bubbles.audit` (verifies the first-disposition outcome is one of the three valid options without invented work)

### Gherkin Scenarios

```gherkin
Scenario: SCN-053-001 Residual G068 work is evidence-gated
  Given BUG-045-002 audit evidence reported 11 mapped scenarios at audit time
  When the planner evaluates TR-BUG-045-002-008
  Then the planner records current traceability evidence before creating any G068 work
  And if no residual G068 gaps exist, TR-BUG-045-002-008 is closed with evidence instead of invented scope
```

### Implementation Plan (Artifact-Only)

1. Author the TR matrix entry for `TR-BUG-045-002-008` in scope-owned planning content (this scope's section). Fields per [design.md](design.md) → "TR Matrix": `sourceArtifact` (BUG-045-002 `report.md` + `state.json`), `sourceClaim` (G068 fidelity carry-forward), `scenarioIds` = `SCN-053-001`, `requirementIds` = `FR-053-001`, `FR-053-002`, `FR-053-003`, `FR-053-015`, `plannedRecordType` = `G068 proof-or-close`.
2. Reserve a slot for the current traceability evidence that the validation owner will capture by executing `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` once the planning artifacts exist. Plan must specify that the evidence is captured into [report.md](report.md) → "Scope 1 Execution Evidence" by the executing owner with provenance tag `executed`.
3. Author exactly one first-disposition record template per [design.md](design.md) → "DD-053-002: Proof-Before-Work For G068":
   - `residual-gap-found` template fields: `scenarioId`, `missingClaim`, `owningArtifact`, `requiredEvidence`.
   - `closed-by-current-proof` template fields: `commandOutputReference`, `predecessorSourceCitation`, `mappedScenarioCount`, `unmappedScenarioCount` (must be zero for this outcome).
   - `owner-routed-tool-issue` template fields: `frameworkArtifactPath`, `routedOwner`, `productNoActionReason`.
4. Record the absolute prohibition: a TR-008 residual-gap record may not be authored unless the captured traceability evidence in [report.md](report.md) shows a specific unmapped or weakly mapped scenario. Stale pre-anchor output from BUG-045-002 is not valid current evidence.

### Test Plan (Artifact Validation Only)

| ID | Validation Type | Command Or Evidence Source | Scenario | Assertion (cite design.md §) |
|----|-----------------|---------------------------|----------|------------------------------|
| V-053-S1-001 | Artifact-lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | SCN-053-001 | Exits 0 after Scope 1 planning content is authored (design.md → "Testing And Validation Strategy" row 1). |
| V-053-S1-002 | Traceability-guard (G068 proof) | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` | SCN-053-001 | Exit code is recorded into [report.md](report.md) Scope 1 evidence with provenance tag `executed`. Output drives selection of exactly one first-disposition outcome (design.md → "Testing And Validation Strategy" row 2 and DD-053-002). |
| V-053-S1-003 | Regression artifact-validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` re-run after disposition record is authored | SCN-053-001 | Same lint exits 0 after the first-disposition record is in place, providing persistent regression protection that the G068 record stays well-formed across later scope edits. |

### Definition of Done (Scope 1)

- [ ] **S1-D1.** TR matrix row for `TR-BUG-045-002-008` is authored in this scope's section with all required fields per design.md → "TR Matrix" populated: `trId`, `sourceArtifact`, `sourceClaim`, `scenarioIds`, `requirementIds`, `plannedRecordType`, `disposition`, `evidenceExpectation`.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 1: G068 Proof-Or-Close" Implementation Plan step 1 record block.
- [ ] **S1-D2.** Current traceability evidence is captured by the executing owner using `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening`. Raw terminal output (command, exit code, mapped/unmapped scenario counts) is recorded in [report.md](report.md) → "Scope 1 Execution Evidence" with provenance tag `executed`.
  - Evidence anchor: [report.md](report.md) → "Scope 1 Execution Evidence".
- [ ] **S1-D3.** Exactly one first-disposition outcome is recorded per DD-053-002: `residual-gap-found`, `closed-by-current-proof`, or `owner-routed-tool-issue`. The recorded outcome cites the V-053-S1-002 command output captured in S1-D2.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 1: G068 Proof-Or-Close" first-disposition record block populated per Implementation Plan step 3.
- [ ] **S1-D4. Scenario validator: SCN-053-001 Residual G068 work is evidence-gated** — Given the BUG-045-002 carry-forward concern TR-BUG-045-002-008, When the captured `traceability-guard.sh` evidence shows zero unmapped scenarios, Then the recorded outcome is `closed-by-current-proof` with no `residual-gap-found` rows authored; OR When the captured evidence names ≥1 specific unmapped scenario, Then a `residual-gap-found` record is authored citing the scenario ID, missing claim, owning artifact, and required evidence; OR When the unmapped condition is traceable to a framework guard false positive, Then an `owner-routed-tool-issue` record is authored routing to upstream Bubbles maintenance.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 1: G068 Proof-Or-Close" first-disposition record block + [report.md](report.md) → "Scope 1 Execution Evidence" command output.
- [ ] **S1-D5.** Artifact lint exits 0 after Scope 1 content is written: `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening`. Raw command + exit code captured in [report.md](report.md) → "Scope 1 Execution Evidence".
  - Evidence anchor: [report.md](report.md) → "Scope 1 Execution Evidence" artifact-lint run block.

---

## Scope 2: Regression Expansion Boundaries

**Status:** Not Started
**Primary TR:** TR-BUG-045-002-009
**Depends On:** Scope 1 (G068 truth controls whether residual gaps may inform regression rows)
**Owner phases:** `bubbles.plan` (record authorship); `bubbles.harden` (gap routing if Scope 1 surfaced gaps); `bubbles.validate` (re-runs `artifact-lint.sh` on the updated planning surface); `bubbles.audit` (verifies every regression row names a new failure mode beyond existing BUG-045-002 proof)

### Gherkin Scenarios

```gherkin
Scenario: SCN-053-002 Regression expansion adds protection beyond existing proof
  Given BUG-045-002 already has a passing topology guard, adversarial contract tests, and live local reproduction evidence
  When the planner evaluates TR-BUG-045-002-009
  Then the planner defines only regression E2E scenarios that protect gaps not already covered by the existing evidence
  And each scenario names its CI job, full-stack reproduction, or contract-test surface
```

### Implementation Plan (Artifact-Only)

1. Author the TR matrix entry for `TR-BUG-045-002-009` in this scope's section. Fields: `sourceArtifact` (BUG-045-002 `report.md` Evidence 1-5b and validation/audit evidence), `sourceClaim` (regression expansion carry-forward), `scenarioIds` = `SCN-053-002`, `requirementIds` = `FR-053-001`, `FR-053-004`, `FR-053-005`, `FR-053-015`, `plannedRecordType` = `regression expansion`.
2. Author a source-surface matrix subsection covering the three allowed expansion surfaces per [design.md](design.md) → "DD-053-003: Regression Expansion Boundaries":
   - `CI integration job` — surface kind `observed`/`protected`, allowed action `cite-only`/`plan-record`, source `.github/workflows/ci.yml` (no edit in this packet).
   - `Full-stack reproduction path` — surface kind `observed`, allowed action `plan-record` (canonical command `./smackerel.sh test integration`), no source edit.
   - `Contract-test surface` — surface kind `observed`/`protected`, allowed action `plan-record`, source `internal/deploy/ci_integration_topology_contract_test.go` (no edit in this packet).
3. Author one regression surface record table with rows that each carry: `regressionRowId`, `surface` (one of the three above), `newProtectedFailureMode`, `existingProofThisDoesNotDuplicate`, `evidenceExpectation`. Any row that cannot name a `newProtectedFailureMode` distinct from existing BUG-045-002 guard/adversarial/repro evidence is explicitly rejected and recorded as `rejected-duplicate`.
4. Carry the disposition forward into the TR matrix entry: `disposition` is `planned` if ≥1 valid row exists; `closed-by-current-proof` if every candidate row is rejected as duplicate; `owner-routed` for any candidate that turns out to belong to a different owner (e.g., framework guard).
5. Cross-reference Scope 1's first-disposition outcome: if Scope 1 closed TR-008 by current proof with zero unmapped scenarios, this scope must not author any regression row that re-proves the same surface that Scope 1 already proved closed.

### Test Plan (Artifact Validation Only)

| ID | Validation Type | Command Or Evidence Source | Scenario | Assertion (cite design.md §) |
|----|-----------------|---------------------------|----------|------------------------------|
| V-053-S2-001 | Artifact-lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | SCN-053-002 | Exits 0 after Scope 2 regression surface records are authored. |
| V-053-S2-002 | Traceability-guard (G068 mapping) | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` | SCN-053-002 | SCN-053-002 maps to ≥1 DoD item in this scope per G068 (design.md → "Testing And Validation Strategy" row 2). |
| V-053-S2-003 | Duplicate-row detection (artifact review) | Audit-time inspection of every regression-row record's `newProtectedFailureMode` field against the existing BUG-045-002 proof catalog | SCN-053-002 | Every row names a failure mode not already covered by BUG-045-002 topology guard, the three adversarial sub-tests, or the local full-stack reproduction. Rows missing a new protective claim are rejected per DD-053-003. |
| V-053-S2-004 | Regression artifact-validation | Re-run V-053-S2-001 and V-053-S2-002 after rows are authored | SCN-053-002 | Both validations exit 0 (or the traceability guard records named gaps that route back to Scope 1), providing persistent regression protection that the regression record set stays well-formed. |

### Definition of Done (Scope 2)

- [ ] **S2-D1.** TR matrix row for `TR-BUG-045-002-009` is authored in this scope's section with all required design.md → "TR Matrix" fields populated.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 2: Regression Expansion Boundaries" Implementation Plan step 1 record block.
- [ ] **S2-D2.** Source-surface matrix subsection covers all three allowed expansion surfaces (CI integration job, full-stack reproduction path, contract-test surface) per DD-053-003 with `surfaceKind` and `allowedAction` populated.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 2: Regression Expansion Boundaries" Implementation Plan step 2 source-surface subsection.
- [ ] **S2-D3.** Regression surface record table is authored where every row populates `regressionRowId`, `surface`, `newProtectedFailureMode`, `existingProofThisDoesNotDuplicate`, and `evidenceExpectation`. Rows that cannot name a new failure mode are recorded as `rejected-duplicate` and excluded from the active set per DD-053-003.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 2: Regression Expansion Boundaries" Implementation Plan step 3 record table.
- [ ] **S2-D4. Scenario validator: SCN-053-002 Regression expansion adds protection beyond existing proof** — Given BUG-045-002 has a passing topology guard, three adversarial contract sub-tests, and a passing local full-stack reproduction, When the regression row table is authored, Then every row's `surface` field is one of {CI integration job, full-stack reproduction path, contract-test surface} AND every row's `newProtectedFailureMode` field names a failure mode not already protected by the named existing BUG-045-002 proof AND every candidate row missing a new protective claim is recorded as `rejected-duplicate` instead of being added to the active set.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 2: Regression Expansion Boundaries" regression surface record table + Implementation Plan step 5 cross-reference block.
- [ ] **S2-D5.** TR matrix `disposition` for `TR-BUG-045-002-009` is set to one of {`planned`, `closed-by-current-proof`, `owner-routed`} consistent with the regression row table state per DD-053-003 and the Scope 1 outcome.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 2: Regression Expansion Boundaries" TR matrix row `disposition` field.
- [ ] **S2-D6.** `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` exits 0 after Scope 2 content is written. Raw command + exit code captured in [report.md](report.md) → "Scope 2 Execution Evidence".
  - Evidence anchor: [report.md](report.md) → "Scope 2 Execution Evidence" artifact-lint run block.

---

## Scope 3: Consumer Trace Inventory

**Status:** Not Started
**Primary TR:** TR-BUG-045-002-010
**Depends On:** None (must complete before Scope 4 blast-radius decisions)
**Owner phases:** `bubbles.plan` (record authorship); `bubbles.harden` (routing of `owner-routed` consumers); `bubbles.validate` (stale-reference scans named by this scope); `bubbles.audit` (verifies every CI workflow consumer has a class and a disposition with no `unclassified` entries)

### Gherkin Scenarios

```gherkin
Scenario: SCN-053-003 CI workflow consumers are inventoried before scope decisions
  Given CI evidence shape may be consumed by first-party workflows, guards, docs, or operator paths
  When the planner evaluates TR-BUG-045-002-010
  Then the planner lists direct and indirect consumers
  And every consumer receives a required-change, no-change-with-evidence, or owner-routed disposition
```

### Implementation Plan (Artifact-Only)

1. Author the TR matrix entry for `TR-BUG-045-002-010`. Fields: `sourceArtifact` (BUG-045-002 `report.md` + this packet's CI workflow citations), `sourceClaim` (CI workflow consumer trace carry-forward), `scenarioIds` = `SCN-053-003`, `requirementIds` = `FR-053-001`, `FR-053-006`, `FR-053-007`, `FR-053-015`, `plannedRecordType` = `consumer inventory`.
2. Author a consumer inventory table per [design.md](design.md) → "DD-053-004: Consumer Trace Model" and "Consumer Inventory Record". Every consumer of the CI integration job or its evidence shape (the GitHub Actions job, the canonical `./smackerel.sh test integration` command path, log/evidence sections, status fields, and documentation surfaces) gets one row populating all eight required fields: `consumerId`, `pathOrSurface`, `consumerClass`, `consumedSignal`, `staleRisk`, `disposition`, `evidenceRef`, `owner`.
3. Use the four consumer classes from DD-053-004 exactly: `direct`, `indirect`, `observational`, `documentation-facing`. Every row carries exactly one class. No row may be `unclassified`.
4. Assign exactly one disposition per row: `required-change`, `no-change-with-evidence`, or `owner-routed`. A `no-change-with-evidence` disposition must cite the read-only inspection or stale-reference scan that proves no change is required. An `owner-routed` disposition must name the routed owner.
5. For every consumer whose `consumedSignal` is at risk of changing in any later scope (especially Scope 4 shared-infrastructure work), author the stale-reference scan command the validation owner will run. The scan command itself is named here as a planning record; execution is the validation owner's responsibility, captured into [report.md](report.md) → "Scope 3 Execution Evidence".
6. Maintain explicit boundary: this scope authors planning records only. No first-party CI workflow file, docs file, contract test file, or CLI wrapper file is edited by Scope 3.

### Test Plan (Artifact Validation Only)

| ID | Validation Type | Command Or Evidence Source | Scenario | Assertion (cite design.md §) |
|----|-----------------|---------------------------|----------|------------------------------|
| V-053-S3-001 | Artifact-lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | SCN-053-003 | Exits 0 after consumer inventory is authored. |
| V-053-S3-002 | Disposition-completeness inspection | Review of consumer inventory rows against the design.md → "Consumer Inventory Record" required-fields list | SCN-053-003 | Every row populates all 8 required fields; every row has exactly one class from {`direct`, `indirect`, `observational`, `documentation-facing`}; every row has exactly one disposition from {`required-change`, `no-change-with-evidence`, `owner-routed`}; zero `unclassified` rows; zero rows with empty disposition. |
| V-053-S3-003 | Stale-reference scan plan | Inspection that every consumer with at-risk `consumedSignal` has a named stale-reference scan command for the executing owner | SCN-053-003 | Every at-risk consumer has a named scan command; absent at-risk consumers are explicitly recorded with `no-change-with-evidence` and the citation that proves no scan is required (design.md → "Testing And Validation Strategy" row 5). |
| V-053-S3-004 | Regression artifact-validation | Re-run V-053-S3-001 + V-053-S3-002 after edits | SCN-053-003 | Both validations exit 0, providing persistent regression protection that the consumer inventory stays complete and well-formed across later scope work. |

### Definition of Done (Scope 3)

- [ ] **S3-D1.** TR matrix row for `TR-BUG-045-002-010` is authored in this scope's section with all required design.md → "TR Matrix" fields populated.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" Implementation Plan step 1 record block.
- [ ] **S3-D2.** Consumer inventory table is authored where every row populates all 8 required fields per "Consumer Inventory Record": `consumerId`, `pathOrSurface`, `consumerClass`, `consumedSignal`, `staleRisk`, `disposition`, `evidenceRef`, `owner`.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" Implementation Plan step 2 consumer inventory table.
- [ ] **S3-D3.** Every consumer is classified into exactly one of the four DD-053-004 classes: `direct`, `indirect`, `observational`, `documentation-facing`. Zero rows are `unclassified`.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" consumer inventory table `consumerClass` column.
- [ ] **S3-D4.** Every consumer receives exactly one disposition: `required-change`, `no-change-with-evidence`, or `owner-routed`. `no-change-with-evidence` rows cite their proving artifact; `owner-routed` rows name the routed owner.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" consumer inventory table `disposition` + `evidenceRef` + `owner` columns.
- [ ] **S3-D5. Scenario validator: SCN-053-003 CI workflow consumers are inventoried before scope decisions** — Given CI evidence shape may be consumed by first-party workflows, guards, docs, or operator paths, When the consumer inventory table is authored, Then ≥1 consumer in each of the four required classes (direct, indirect, observational, documentation-facing) is enumerated where such a consumer exists AND every enumerated consumer has a single disposition from the three allowed values AND any consumer that cannot be confidently classified or dispositioned blocks Scope 3 from reaching `Done`.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" consumer inventory table + S3-D3 + S3-D4 anchors above.
- [ ] **S3-D6.** Stale-reference scan commands are named for every consumer whose `consumedSignal` is at risk of changing in any later scope. The named commands are recorded for the validation owner to execute and capture evidence into [report.md](report.md) → "Scope 3 Execution Evidence".
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 3: Consumer Trace Inventory" Implementation Plan step 5 stale-reference scan plan.
- [ ] **S3-D7.** `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` exits 0 after Scope 3 content is written. Raw command + exit code captured in [report.md](report.md) → "Scope 3 Execution Evidence".
  - Evidence anchor: [report.md](report.md) → "Scope 3 Execution Evidence" artifact-lint run block.

---

## Scope 4: Shared Infrastructure Blast Radius

**Status:** Not Started
**Primary TR:** TR-BUG-045-002-011
**Depends On:** Scope 3 (consumer inventory must precede evidence-shape decisions)
**Owner phases:** `bubbles.design` (confirms shared-infrastructure surface boundary if questions arise); `bubbles.plan` (record authorship); `bubbles.validate` (executes scope-defined canary commands); `bubbles.audit` (verifies every protected surface has a named canary check and broad-validation trigger)

### Gherkin Scenarios

```gherkin
Scenario: SCN-053-004 Shared infrastructure blast radius is bounded before broad validation
  Given the affected surfaces include the test stack, CI workflow, contract tests, and CLI wrappers
  When the designer and planner evaluate TR-BUG-045-002-011
  Then the plan identifies protected contracts, canary checks, broad validation triggers, and rollback or restore expectations
```

### Implementation Plan (Artifact-Only)

1. Author the TR matrix entry for `TR-BUG-045-002-011`. Fields: `sourceArtifact` (BUG-045-002 `report.md` + this packet's shared-infrastructure citations), `sourceClaim` (shared-infrastructure blast-radius carry-forward), `scenarioIds` = `SCN-053-004`, `requirementIds` = `FR-053-001`, `FR-053-008`, `FR-053-009`, `FR-053-015`, `plannedRecordType` = `blast-radius`.
2. Author one blast-radius record per protected surface per [design.md](design.md) → "DD-053-005: Shared Infrastructure Blast-Radius Model" and "Blast-Radius Record". The four required protected surfaces are:
   - `Test stack lifecycle` — protected contract: test stack starts, reports health, runs integration suites, and tears down through `./smackerel.sh`.
   - `CI workflow ordering` — protected contract: CI uses the canonical command and preserves upload/failure-step semantics.
   - `Contract-test parsing` — protected contract: tests parse the intended workflow and reject known-bad topologies.
   - `CLI wrappers` — protected contract: user-facing command remains `./smackerel.sh test integration`; wrapper helpers remain internal.
3. Each record populates all 7 required fields per "Blast-Radius Record": `surfaceId`, `protectedContract`, `dependentSurfaces`, `canaryCheck`, `broadValidationTrigger`, `rollbackOrRestore`, `evidenceExpectation`.
4. Cross-reference Scope 3 consumer inventory: every `dependentSurfaces` entry must trace back to a Scope 3 consumer row OR be explicitly noted as a non-consumer dependency with rationale. A blast-radius record cannot claim a dependent surface that Scope 3 did not inventory.
5. Define the broad-validation gating rule: a `broadValidationTrigger` is only honored after the surface's named `canaryCheck` is recorded and (when later executed by the validation owner) captured into [report.md](report.md) → "Scope 4 Execution Evidence". This is a planning rule that downstream scopes and bug packets must observe; it does not authorize any source-code change in this packet.
6. Maintain explicit boundary: this scope authors planning records only. No first-party shared-infrastructure file (CI workflow, contract test, CLI wrapper, test-stack lifecycle script) is edited by Scope 4.

### Test Plan (Artifact Validation Only)

| ID | Validation Type | Command Or Evidence Source | Scenario | Assertion (cite design.md §) |
|----|-----------------|---------------------------|----------|------------------------------|
| V-053-S4-001 | Artifact-lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | SCN-053-004 | Exits 0 after blast-radius records are authored. |
| V-053-S4-002 | Coverage-completeness inspection | Review of blast-radius records against the four required protected surfaces in DD-053-005 | SCN-053-004 | Every required protected surface (test stack lifecycle, CI workflow ordering, contract-test parsing, CLI wrappers) has exactly one blast-radius record with all 7 required fields populated. |
| V-053-S4-003 | Canary-presence inspection | Review of every blast-radius record's `canaryCheck` field | SCN-053-004 | Zero records have empty `canaryCheck`; the named canary is narrow enough to validate the protected contract without depending on the broader suite (design.md → "Testing And Validation Strategy" row 6). |
| V-053-S4-004 | Consumer-cross-reference inspection | Comparison of every `dependentSurfaces` entry against Scope 3 consumer inventory | SCN-053-004 | Every `dependentSurfaces` entry either traces to a Scope 3 consumer row or carries an explicit non-consumer dependency rationale. |
| V-053-S4-005 | Regression artifact-validation | Re-run V-053-S4-001 + V-053-S4-002 + V-053-S4-003 after edits | SCN-053-004 | All three validations exit 0, providing persistent regression protection that blast-radius coverage stays complete across later scope work. |

### Definition of Done (Scope 4)

- [ ] **S4-D1.** TR matrix row for `TR-BUG-045-002-011` is authored in this scope's section with all required design.md → "TR Matrix" fields populated.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 4: Shared Infrastructure Blast Radius" Implementation Plan step 1 record block.
- [ ] **S4-D2.** A blast-radius record exists for each of the four required protected surfaces per DD-053-005: test stack lifecycle, CI workflow ordering, contract-test parsing, CLI wrappers. Zero required surfaces are missing.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 4: Shared Infrastructure Blast Radius" Implementation Plan step 2 blast-radius record set.
- [ ] **S4-D3.** Every blast-radius record populates all 7 required "Blast-Radius Record" fields: `surfaceId`, `protectedContract`, `dependentSurfaces`, `canaryCheck`, `broadValidationTrigger`, `rollbackOrRestore`, `evidenceExpectation`.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 4: Shared Infrastructure Blast Radius" Implementation Plan step 3 record fields.
- [ ] **S4-D4.** Every `dependentSurfaces` entry traces back to a Scope 3 consumer row OR carries an explicit non-consumer dependency rationale referencing the source-truth citation.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 4: Shared Infrastructure Blast Radius" Implementation Plan step 4 cross-reference block + Scope 3 consumer inventory.
- [ ] **S4-D5. Scenario validator: SCN-053-004 Shared infrastructure blast radius is bounded before broad validation** — Given the affected surfaces are test stack, CI workflow, contract tests, and CLI wrappers, When the blast-radius record set is authored, Then each of those four surfaces has a named `canaryCheck` AND a named `broadValidationTrigger` AND a named `rollbackOrRestore` expectation AND the broad-validation gating rule is recorded such that `broadValidationTrigger` is only honored after the named `canaryCheck` is captured by the executing owner.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 4: Shared Infrastructure Blast Radius" blast-radius record set + Implementation Plan step 5 gating rule block.
- [ ] **S4-D6.** `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` exits 0 after Scope 4 content is written. Raw command + exit code captured in [report.md](report.md) → "Scope 4 Execution Evidence".
  - Evidence anchor: [report.md](report.md) → "Scope 4 Execution Evidence" artifact-lint run block.

---

## Scope 5: Boundary And Wrapper Disposition

**Status:** Not Started
**Primary TR:** TR-BUG-045-002-012 (plus framework-boundary record for TR-BUG-045-002-014)
**Depends On:** Scopes 1, 2, 3, 4 (final containment records reference all prior scope outputs)
**Owner phases:** `bubbles.plan` (record authorship); `bubbles.validate` (executes `git diff --name-status` for no-source-delta proof and stale-reference scans); `bubbles.audit` (verifies every G040 wrapper has a disposition; verifies the framework-boundary record routes TR-014 upstream without authoring a Smackerel framework-edit action)

### Gherkin Scenarios

```gherkin
Scenario: SCN-053-005 Change boundary and G040 wrapper disposition are explicit
  Given BUG-045-002 routed planning content through G040 skip-region wrappers
  When the planner evaluates TR-BUG-045-002-012
  Then the plan lists allowed file families, excluded surfaces, and a disposition for each wrapper
  And the plan requires proof that excluded surfaces remain unchanged
```

```gherkin
Scenario: SCN-053-006 Framework-owned TR-014 stays outside Smackerel product scope
  Given TR-BUG-045-002-014 is owned by Bubbles workflow/framework maintenance
  When this Smackerel product spec references TR-014
  Then the reference is limited to an upstream-framework note
  And no Smackerel product scope attempts to edit framework-managed files
```

```gherkin
Scenario: SCN-053-007 One consolidated spec covers the product planning set
  Given bubbles.grill recommended one consolidated spec for TR-008 through TR-012
  When this feature is planned
  Then TR-008, TR-009, TR-010, TR-011, and TR-012 map to this spec
  And `specs/054-artifact-output-summarization` is not created by this feature
```

### Implementation Plan (Artifact-Only)

1. Author the TR matrix entry for `TR-BUG-045-002-012`. Fields: `sourceArtifact` (BUG-045-002 `report.md` G040 wrapper regions + state.json), `sourceClaim` (boundary and wrapper disposition carry-forward), `scenarioIds` = `SCN-053-005`, `requirementIds` = `FR-053-001`, `FR-053-010`, `FR-053-011`, `FR-053-015`, `plannedRecordType` = `boundary and wrapper`.
2. Author one Boundary Record per scope (1, 2, 3, 4, 5) per [design.md](design.md) → "DD-053-006: Change Boundary Model" and "Boundary Record". Each record populates all 6 required fields: `scopeId`, `allowedFileFamilies`, `excludedSurfaces`, `allowedChangeType`, `noSourceDeltaProof`, `owner`.
3. The default `allowedFileFamilies` for every scope in this packet is `specs/053-ci-ops-evidence-hardening/**`. The default `excludedSurfaces` set covers: `internal/**`, `cmd/**`, `ml/**`, `web/**`, `scripts/runtime/**`, `scripts/commands/**`, `docker-compose*.yml`, `Dockerfile`, `deploy/**`, `.github/workflows/**`, `.github/bubbles/**`, `.github/agents/bubbles_shared/**`, `.github/instructions/bubbles-*.md`, `.github/skills/bubbles-*/**`, and the predecessor `specs/045-deploy-resource-filesystem-hardening/**` files (cite-only). `allowedChangeType` is `artifact-only` for every scope of this packet.
4. Name the `noSourceDeltaProof` command for the validation owner to execute and capture into [report.md](report.md) → "Scope 5 Execution Evidence": `git diff --name-status main...HEAD -- ':(exclude)specs/053-ci-ops-evidence-hardening/' ':(exclude)specs/053-ci-ops-evidence-hardening/**'` (or the equivalent against the baseline branch the validation owner selects). Expected output: zero lines outside `specs/053-ci-ops-evidence-hardening/`.
5. Author one Wrapper Disposition Record per G040 wrapper region present in the predecessor BUG-045-002 `report.md`. Each record populates all 8 required fields per "Wrapper Disposition Record": `wrapperId`, `predecessorLocation`, `containedClaim`, `mappedTrId`, `mappedScopeId`, `disposition`, `crossReferenceRequired`, `evidenceExpectation`. Valid dispositions per DD-053-007: `historical-retain`, `cross-reference-retain`, or `owner-remove`. Design does not remove or edit predecessor wrappers; this scope only records the model and the per-wrapper disposition for later owner execution.
6. Author the Framework-Boundary Record for `TR-BUG-045-002-014` per [design.md](design.md) → "DD-053-008: Framework Boundary". Fields: `trId` = `TR-BUG-045-002-014`, `routedOwner` = `bubbles.workflow` / upstream Bubbles framework repository, `frameworkArtifactPaths` (no-edit list) = `.github/bubbles/scripts/state-transition-guard.sh`, `.github/bubbles/scripts/**` (any installed framework script), `.github/agents/bubbles_shared/**`, `.github/instructions/bubbles-*.md`, `.github/skills/bubbles-*/**`, `productEvidenceCitationOnly` = `true`, `crossRepoFollowUp` = "framework guard repairs must land in the canonical Bubbles repository first; Smackerel receives them downstream through the standard framework upgrade path." This record MUST NOT author any Smackerel framework-edit action.
7. Author the consolidation record for SCN-053-007: a single-row record stating that TR-008, TR-009, TR-010, TR-011, and TR-012 all map to `specs/053-ci-ops-evidence-hardening/` and that `specs/054-artifact-output-summarization` is not created by this feature. The record cites FR-053-013 and FR-053-014.

### Test Plan (Artifact Validation Only)

| ID | Validation Type | Command Or Evidence Source | Scenario | Assertion (cite design.md §) |
|----|-----------------|---------------------------|----------|------------------------------|
| V-053-S5-001 | Artifact-lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` | SCN-053-005, SCN-053-006, SCN-053-007 | Exits 0 after Scope 5 boundary, wrapper, framework-boundary, and consolidation records are authored. |
| V-053-S5-002 | No-source-delta proof | `git diff --name-status main...HEAD -- ':(exclude)specs/053-ci-ops-evidence-hardening/' ':(exclude)specs/053-ci-ops-evidence-hardening/**'` (executed by validation owner) | SCN-053-005 | Output is empty (zero lines), proving no excluded surface changed across this packet. Raw command + output captured into [report.md](report.md) → "Scope 5 Execution Evidence" (design.md → "Testing And Validation Strategy" row 4). |
| V-053-S5-003 | Framework-no-edit inspection | Read-only inspection of the diff output from V-053-S5-002 plus an explicit grep of changed paths against the `frameworkArtifactPaths` no-edit list | SCN-053-006 | Zero changed paths fall inside `.github/bubbles/**`, `.github/agents/bubbles_shared/**`, `.github/instructions/bubbles-*.md`, or `.github/skills/bubbles-*/**`. |
| V-053-S5-004 | Consolidation inspection | Read-only inspection that `specs/054-artifact-output-summarization` does not exist after this feature lands | SCN-053-007 | `ls specs/ | grep -E '^054-'` returns no result OR is recorded with an explicit note showing that 054 was not created by this feature. |
| V-053-S5-005 | Traceability-guard (G068 mapping) | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` | SCN-053-005, SCN-053-006, SCN-053-007 | Each Scope 5 scenario maps to ≥1 DoD item in this scope per G068. |
| V-053-S5-006 | Regression artifact-validation | Re-run V-053-S5-001 + V-053-S5-002 + V-053-S5-005 after edits | SCN-053-005, SCN-053-006, SCN-053-007 | All three validations exit 0, providing persistent regression protection that boundary, wrapper, and framework-boundary records stay well-formed across later edits. |

### Definition of Done (Scope 5)

- [ ] **S5-D1.** TR matrix row for `TR-BUG-045-002-012` is authored in this scope's section with all required design.md → "TR Matrix" fields populated.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Implementation Plan step 1 record block.
- [ ] **S5-D2.** Boundary Records exist for every scope in this packet (1, 2, 3, 4, 5). Each record populates all 6 required "Boundary Record" fields: `scopeId`, `allowedFileFamilies`, `excludedSurfaces`, `allowedChangeType`, `noSourceDeltaProof`, `owner`. `allowedChangeType` is `artifact-only` for every scope of this packet.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Implementation Plan step 2 Boundary Record set.
- [ ] **S5-D3.** The `noSourceDeltaProof` command is named in every Boundary Record and the named command is executed by the validation owner with raw `git diff --name-status` output captured into [report.md](report.md) → "Scope 5 Execution Evidence". Output shows zero changes outside `specs/053-ci-ops-evidence-hardening/`.
  - Evidence anchor: [report.md](report.md) → "Scope 5 Execution Evidence" no-source-delta proof block.
- [ ] **S5-D4.** Wrapper Disposition Records exist for every G040 wrapper region present in the predecessor BUG-045-002 `report.md`. Each record populates all 8 required "Wrapper Disposition Record" fields with a disposition from {`historical-retain`, `cross-reference-retain`, `owner-remove`} per DD-053-007.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Implementation Plan step 5 Wrapper Disposition Record set.
- [ ] **S5-D5. Scenario validator: SCN-053-005 Change boundary and G040 wrapper disposition are explicit** — Given BUG-045-002 routed planning content through G040 skip-region wrappers, When the Scope 5 Boundary Record set and Wrapper Disposition Record set are authored, Then every scope has a Boundary Record listing allowed file families and excluded surfaces AND every G040 wrapper in the predecessor report has a disposition AND the `noSourceDeltaProof` command is named such that the validation owner can prove excluded surfaces remain unchanged.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Boundary Record set + Wrapper Disposition Record set + [report.md](report.md) → "Scope 5 Execution Evidence" no-source-delta proof block.
- [ ] **S5-D6. Scenario validator: SCN-053-006 Framework-owned TR-014 stays outside Smackerel product scope** — Given `TR-BUG-045-002-014` is owned by Bubbles workflow/framework maintenance, When the Framework-Boundary Record is authored, Then `routedOwner` names `bubbles.workflow` / upstream Bubbles framework repository AND `frameworkArtifactPaths` lists `.github/bubbles/**`, `.github/agents/bubbles_shared/**`, `.github/instructions/bubbles-*.md`, and `.github/skills/bubbles-*/**` as no-edit surfaces AND `productEvidenceCitationOnly` is `true` AND no Smackerel scope in this packet authors an edit to any path in the no-edit list.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Implementation Plan step 6 Framework-Boundary Record + [report.md](report.md) → "Scope 5 Execution Evidence" framework-no-edit inspection block (V-053-S5-003).
- [ ] **S5-D7. Scenario validator: SCN-053-007 One consolidated spec covers the product planning set** — Given bubbles.grill recommended one consolidated spec for TR-008 through TR-012, When the consolidation record is authored, Then the record explicitly states that TR-008, TR-009, TR-010, TR-011, and TR-012 all map to `specs/053-ci-ops-evidence-hardening/` AND the record explicitly states that `specs/054-artifact-output-summarization` is not created by this feature AND `ls specs/ | grep -E '^054-'` returns no result captured by the validation owner.
  - Evidence anchor: [scopes.md](scopes.md) → "Scope 5: Boundary And Wrapper Disposition" Implementation Plan step 7 consolidation record + [report.md](report.md) → "Scope 5 Execution Evidence" consolidation inspection block (V-053-S5-004).
- [ ] **S5-D8.** `bash .github/bubbles/scripts/artifact-lint.sh specs/053-ci-ops-evidence-hardening` exits 0 after Scope 5 content is written. Raw command + exit code captured in [report.md](report.md) → "Scope 5 Execution Evidence".
  - Evidence anchor: [report.md](report.md) → "Scope 5 Execution Evidence" artifact-lint run block.
- [ ] **S5-D9.** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/053-ci-ops-evidence-hardening` is executed by the validation owner after all five scopes are authored. Raw command + exit code + mapped/unmapped scenario counts captured in [report.md](report.md) → "Scope 5 Execution Evidence". Every SCN-053-001..007 maps to ≥1 DoD item in its owning scope per G068.
  - Evidence anchor: [report.md](report.md) → "Scope 5 Execution Evidence" traceability-guard run block.
