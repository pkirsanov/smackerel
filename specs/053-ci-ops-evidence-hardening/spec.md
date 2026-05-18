# Feature: 053 CI Ops Evidence Hardening

## Problem Statement

BUG-045-002 closed the chronic Smackerel `ci.yml` integration-job failure by routing CI through the canonical `./smackerel.sh test integration` path and by adding a topology contract guard. The close-out packet ended in `done_with_concerns` because five product-side planning concerns remained routed from BUG-045-002 into `bubbles.plan`:

- `TR-BUG-045-002-008`: next-iteration G068 DoD-Gherkin fidelity coverage beyond the 11 scenarios mapped at audit time.
- `TR-BUG-045-002-009`: regression E2E coverage expansion for the CI integration job, full-stack reproduction, and contract-test surface.
- `TR-BUG-045-002-010`: consumer trace planning for CI workflow consumers.
- `TR-BUG-045-002-011`: shared infrastructure blast-radius planning across the test stack, CI workflow, contract tests, and CLI wrappers.
- `TR-BUG-045-002-012`: change boundary containment and G040 wrapper disposition.

The risks are planning drift and evidence drift, not a current runtime outage. The prior packet already proved the CI fix with green CI and local live-stack evidence; this spec consolidates the remaining operational planning into one product-owned artifact so the next design and plan phases can decide what to evidence, what to close as already satisfied, and what to exclude as framework-owned.

`TR-BUG-045-002-014` is framework-owned. It is explicitly excluded from this Smackerel product spec except as an upstream-framework note because the requested fixes live in Bubbles framework guard behavior, not Smackerel runtime, CI topology, or product-owned planning artifacts.

## Source Evidence And Current Capability Map

| Source | Observed Fact | Planning Impact |
|--------|---------------|-----------------|
| BUG-045-002 `report.md` Evidence 1-5b | CI failed repeatedly because CI's service topology diverged from the local canonical integration runner. | This spec must not re-litigate the fixed root cause; it plans evidence hardening around the closed fix. |
| BUG-045-002 `report.md` validation and audit evidence | Fix HEAD and later main pushes showed successful CI integration runs, and local `./smackerel.sh test integration` passed with the previously failing tests. | Regression expansion must build on already passing proof and identify additional protection, not duplicate the original close-out. |
| BUG-045-002 `state.json` `transitionRequests[]` | TR-008 through TR-012 are open product-planning requests owned by `bubbles.plan`; TR-014 is open framework work owned by `bubbles.workflow`. | This spec covers TR-008..012 and excludes TR-014 from product scope. |
| BUG-045-002 `report.md` Follow-up Work section | Audit listed G068 fidelity, regression E2E expansion, consumer trace, shared-infrastructure blast radius, and change-boundary containment. | These become the five active planning surfaces in this consolidated spec. |
| BUG-045-002 audit close-out | Traceability guard already showed 11/11 mappings at audit entry after the trace-ID anchor approach. | The G068 work must first prove residual gaps exist. If none exist, the correct outcome is evidence-backed closure, not invented planning work. |

### Current Capabilities

| Capability | Existing Evidence | Status |
|------------|-------------------|--------|
| CI integration job uses canonical CLI path | BUG-045-002 close-out evidence and topology guard | Complete for the original bug fix |
| Build-time contract guard for CI integration topology | `ci_integration_topology_contract_test.go` evidence in BUG-045-002 | Complete for the original bug fix |
| Local full-stack reproduction of the integration path | BUG-045-002 Scope 3 evidence | Complete for the original bug fix |
| Residual G068 proof-or-close decision | TR-008 open | Missing as consolidated planning output |
| Regression E2E expansion plan | TR-009 open | Missing as consolidated planning output |
| Consumer trace inventory for CI workflow consumers | TR-010 open | Missing as consolidated planning output |
| Shared infrastructure blast-radius plan | TR-011 open | Missing as consolidated planning output |
| Change boundary and G040 wrapper disposition plan | TR-012 open | Missing as consolidated planning output |

## Outcome Contract

**Intent:** Create one source-grounded operations planning contract for the BUG-045-002 carry-forward product concerns so Smackerel can harden CI evidence, traceability, and planning boundaries without changing runtime code in the analyst phase.

**Success Signal:** A later `bubbles.plan` run can create scopes from this spec where every TR-008..012 item maps to concrete scenarios, requirements, evidence expectations, and disposition rules; TR-014 remains out of product scope; and TR-008 cannot generate work unless a current traceability run proves residual G068 gaps still exist.

**Hard Constraints:**

- The spec covers TR-008, TR-009, TR-010, TR-011, and TR-012 together as one consolidated planning unit.
- TR-014 remains framework-owned and must not become Smackerel product implementation or product-planning scope.
- No runtime, source, CI workflow, contract-test, or CLI wrapper file is changed by this analyst artifact.
- Every planning claim must trace to BUG-045-002 `report.md`, BUG-045-002 `state.json`, or a later executed validation artifact created by the owning phase.
- G068 work must begin with evidence. If the current artifacts show no residual G068 gap, the planned outcome is evidence-backed closure of TR-008.
- The related identifier `specs/054-artifact-output-summarization` is reserved as a separate idea and is not created by this spec.

**Failure Condition:** This feature fails if it invents residual gaps, splits the five product TRs into fragmented specs without a traceable reason, pulls TR-014 framework guard maintenance into Smackerel product scope, or plans source-code changes before design and scope owners define the evidence contract.

## Goals

- Consolidate TR-008 through TR-012 into one testable product-side ops planning specification.
- Define evidence-first acceptance behavior for residual G068 DoD-Gherkin fidelity concerns.
- Define the regression E2E planning surface for the CI integration job, full-stack reproduction path, and contract-test surface.
- Define the consumer trace planning surface for first-party consumers of the CI integration workflow and its evidence.
- Define shared-infrastructure blast-radius planning for the test stack, CI workflow, contract tests, and CLI wrappers.
- Define change-boundary containment and G040 wrapper disposition rules for the remaining BUG-045-002 planning surfaces.
- Preserve source and framework ownership boundaries.

## Non-Goals

- No Smackerel runtime implementation.
- No edits to `.github/workflows/ci.yml`, `internal/`, `cmd/`, `scripts/runtime/`, `docker-compose*`, or `deploy/` surfaces in this analyst phase.
- No creation of `design.md` or `scopes.md` by this agent; those are owned by `bubbles.design` and `bubbles.plan`.
- No creation of `specs/054-artifact-output-summarization` in this invocation.
- No Bubbles framework guard changes for TR-014 in this product repository.
- No reopening of the original BUG-045-002 CI failure unless new evidence proves the CI fix regressed.

## Actors And Personas

| Actor | Description | Key Goals | Permissions / Boundaries |
|-------|-------------|-----------|--------------------------|
| Smackerel Maintainer | Product maintainer responsible for keeping CI, planning artifacts, and evidence credible. | See which carry-forward concerns remain and which owner must act. | Can approve product planning direction; cannot edit framework-managed Bubbles files in this repo. |
| Bubbles Planner | Planning owner for scopes, DoD, Gherkin-to-test mapping, consumer sweeps, blast-radius planning, and change boundaries. | Convert this spec into executable scopes with clear evidence expectations. | Owns `scopes.md`; must not implement source changes. |
| Bubbles Designer | Design owner for technical planning boundaries, evidence architecture, and operational contracts. | Decide how planning outputs should be structured before scopes are authored. | Owns `design.md`; must keep product spec constraints intact. |
| Validator / Auditor | Certification owner that verifies whether planned evidence actually proves the claims. | Confirm closure criteria without accepting proxy or invented evidence. | Owns certification state and audit verdicts; must not author planning content. |
| CI Workflow Consumer | Any product-owned workflow, guard, doc, or operator path that relies on the CI integration job or its evidence shape. | Avoid stale assumptions when CI evidence surfaces change. | Must be inventoried by planning before changes are scoped. |
| Framework Maintainer | Upstream Bubbles owner for TR-014 state-transition-guard behavior. | Address guard false positives in the framework source of truth. | Owns framework changes outside this Smackerel product spec. |

## Use Cases

### UC-053-001: Prove Or Close Residual G068 Work

- **Actor:** Bubbles Planner
- **Preconditions:** BUG-045-002 TR-008 is open and prior audit evidence reported 11/11 mapped scenarios.
- **Main Flow:**
  1. Planner runs or references a current traceability check for the BUG-045-002 planning artifacts.
  2. Planner records whether any residual G068 gaps exist beyond the 11 mapped at audit time.
  3. If gaps exist, planner names each gap, the scenario, the missing DoD or Test Plan fidelity, and the evidence needed to close it.
  4. If no gaps exist, planner records evidence-backed closure for TR-008 without creating new work.
- **Alternative Flow:** If the traceability tool itself produces a framework-owned false positive, planner routes that false positive to the framework owner and does not convert it into Smackerel work.
- **Postconditions:** TR-008 has either a precise scopeable gap list or a closure-by-evidence disposition.

### UC-053-002: Plan Regression E2E Expansion Around Existing CI Proof

- **Actor:** Bubbles Planner
- **Preconditions:** BUG-045-002's contract guard, adversarial tests, and local Path-A reproduction already passed.
- **Main Flow:**
  1. Planner inventories the three expansion surfaces: CI integration job, full-stack reproduction path, and contract-test surface.
  2. Planner defines which regression E2E scenarios add new protection beyond existing guard and local repro evidence.
  3. Planner rejects duplicate rows that only restate existing BUG-045-002 proof.
  4. Planner requires every new regression row to have a named scenario, file/location, expected observable result, and evidence source.
- **Postconditions:** TR-009 has a concrete expansion plan that protects real gaps without bloating the suite.

### UC-053-003: Trace Consumers Of CI Workflow Evidence

- **Actor:** Bubbles Planner
- **Preconditions:** TR-010 is open and CI evidence shape can affect workflow consumers.
- **Main Flow:**
  1. Planner inventories product-owned consumers of CI integration evidence.
  2. Planner classifies each consumer as direct, indirect, observational, or documentation-facing.
  3. Planner records stale-reference scans and compatibility expectations for each consumer class.
  4. Planner identifies any consumer with no required change and records the evidence for no action.
- **Postconditions:** TR-010 has a consumer trace plan that prevents stale CI assumptions.

### UC-053-004: Bound Shared Infrastructure Blast Radius

- **Actor:** Bubbles Designer and Bubbles Planner
- **Preconditions:** TR-011 is open and the surfaces include shared test stack, CI workflow, contract tests, and CLI wrappers.
- **Main Flow:**
  1. Designer defines the shared infrastructure surfaces and their dependency direction.
  2. Planner records canary expectations for each protected surface.
  3. Planner defines broad validation only after independent canaries protect shared contracts.
  4. Planner records rollback or restore expectations for any scoped change.
- **Postconditions:** TR-011 has a blast-radius plan that prevents shared-infrastructure changes from hiding collateral damage.

### UC-053-005: Contain Change Boundaries And Resolve G040 Wrapper Disposition

- **Actor:** Bubbles Planner
- **Preconditions:** TR-012 is open and BUG-045-002 report content includes G040 skip-region wrappers around routed planning content.
- **Main Flow:**
  1. Planner defines allowed file families for each scoped change.
  2. Planner lists excluded surfaces that must remain untouched.
  3. Planner decides whether each G040 wrapper remains as historical source evidence, receives a cross-reference to this spec, or is removed by the owning artifact phase after evidence is captured.
  4. Planner requires proof that no excluded surface changed.
- **Postconditions:** TR-012 has a concrete boundary and wrapper disposition rule.

### UC-053-006: Preserve Framework Boundary For TR-014

- **Actor:** Smackerel Maintainer and Framework Maintainer
- **Preconditions:** TR-014 is open and points to Bubbles framework guard maintenance.
- **Main Flow:**
  1. Smackerel spec records TR-014 as excluded from product scope.
  2. Any reference to TR-014 is limited to an upstream-framework note.
  3. Product scopes do not modify `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, or installed framework files.
- **Postconditions:** TR-014 remains routed to upstream Bubbles workflow maintenance.

## Gherkin Acceptance Scenarios

```gherkin
Scenario: SCN-053-001 Residual G068 work is evidence-gated
  Given BUG-045-002 audit evidence reported 11 mapped scenarios at audit time
  When the planner evaluates TR-BUG-045-002-008
  Then the planner records current traceability evidence before creating any G068 work
  And if no residual G068 gaps exist, TR-BUG-045-002-008 is closed with evidence instead of invented scope
```

```gherkin
Scenario: SCN-053-002 Regression expansion adds protection beyond existing proof
  Given BUG-045-002 already has a passing topology guard, adversarial contract tests, and live local reproduction evidence
  When the planner evaluates TR-BUG-045-002-009
  Then the planner defines only regression E2E scenarios that protect gaps not already covered by the existing evidence
  And each scenario names its CI job, full-stack reproduction, or contract-test surface
```

```gherkin
Scenario: SCN-053-003 CI workflow consumers are inventoried before scope decisions
  Given CI evidence shape may be consumed by first-party workflows, guards, docs, or operator paths
  When the planner evaluates TR-BUG-045-002-010
  Then the planner lists direct and indirect consumers
  And every consumer receives a required-change, no-change-with-evidence, or owner-routed disposition
```

```gherkin
Scenario: SCN-053-004 Shared infrastructure blast radius is bounded before broad validation
  Given the affected surfaces include the test stack, CI workflow, contract tests, and CLI wrappers
  When the designer and planner evaluate TR-BUG-045-002-011
  Then the plan identifies protected contracts, canary checks, broad validation triggers, and rollback or restore expectations
```

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

## Functional Requirements

| ID | Requirement |
|----|-------------|
| FR-053-001 | The planning packet MUST cite BUG-045-002 `report.md` and `state.json` as source material for every TR-008..012 claim. |
| FR-053-002 | TR-008 planning MUST start with a current traceability result or equivalent evidence proving whether residual G068 gaps exist. |
| FR-053-003 | TR-008 planning MUST close with evidence, rather than create scope, when residual G068 gaps are absent. |
| FR-053-004 | TR-009 planning MUST cover all three named surfaces: CI integration job, full-stack reproduction path, and contract-test surface. |
| FR-053-005 | TR-009 planning MUST reject regression rows that only duplicate already passing BUG-045-002 evidence without adding a new protective claim. |
| FR-053-006 | TR-010 planning MUST inventory first-party consumers of the CI integration workflow and CI evidence shape before scope decisions are made. |
| FR-053-007 | TR-010 planning MUST assign every consumer a required-change, no-change-with-evidence, or owner-routed disposition. |
| FR-053-008 | TR-011 planning MUST identify protected shared infrastructure contracts for test stack lifecycle, CI workflow ordering, contract-test parsing, and CLI wrapper behavior. |
| FR-053-009 | TR-011 planning MUST require independent canary checks before broad validation when a protected shared infrastructure surface is changed. |
| FR-053-010 | TR-012 planning MUST list allowed file families and excluded surfaces for each planned change. |
| FR-053-011 | TR-012 planning MUST define the disposition for each BUG-045-002 G040 wrapper that contains product-side planning content. |
| FR-053-012 | The spec MUST record TR-014 as excluded from Smackerel product scope and limited to an upstream-framework note. |
| FR-053-013 | The spec MUST keep the five product transition requests consolidated under `specs/053-ci-ops-evidence-hardening`. |
| FR-053-014 | The spec MUST NOT create `specs/054-artifact-output-summarization`; it may only mention the identifier as a reserved related idea. |
| FR-053-015 | Later planning outputs MUST use evidence provenance tags and must distinguish executed evidence, interpreted evidence, and owner-routed evidence gaps. |

## Non-Functional Requirements

- **Auditability:** Every planned claim must be traceable to a TR ID, source artifact, scenario, and requirement.
- **Evidence Integrity:** Scope evidence must use raw terminal or tool output when execution is claimed; interpreted planning conclusions must be labeled as interpreted.
- **Source Boundary:** Analyst, design, and plan phases for this spec must not mutate runtime/source files.
- **Framework Boundary:** Product-owned work must not edit framework-managed Bubbles install artifacts in the Smackerel repository.
- **Minimal Blast Radius:** Planning must prefer narrow, explicit surfaces and canary evidence before broad-suite reliance.
- **Consumer Safety:** Any renamed, removed, or reclassified CI evidence surface must have consumer trace coverage before closure.
- **No Invented Work:** Absence of a residual gap after evidence review is a valid closure outcome.

## Product Principle Alignment

Smackerel product principles are surfaced for owner approval and are not yet ratified. This feature is governance and operations support rather than a direct end-user product feature, but it supports the product direction in these ways:

| Principle | Relevance |
|-----------|-----------|
| Principle 5 - One Graph, Many Views | Consolidating five related TRs into one spec prevents fragmented planning records and keeps evidence, scenarios, and successor decisions visible from one operational graph of truth. |
| Principle 8 - Trust Through Transparency | The spec requires source-qualified planning, provenance tags, explicit consumer trace, and no invented residual gaps. This mirrors the product trust requirement that users can trace why the system did what it did. |
| Principle 3 - Knowledge Breathes | The spec treats old skip-region planning notes as lifecycle-managed evidence: each receives closure, scoped planning, or upstream ownership rather than remaining stale text. |
| Constitution C7 - Single CLI Operations | Regression planning must preserve the canonical Smackerel CLI as the product-owned execution surface for integration evidence. |
| Constitution C9/C10 - Isolated Test Environments and Docker Lifecycle Safety | Shared-infrastructure blast-radius planning must protect test-stack lifecycle, container cleanup, and freshness evidence. |

## Success Metrics

| Metric | Target |
|--------|--------|
| TR coverage | 5/5 product TRs (008..012) mapped to scenarios and requirements. |
| Framework exclusion | TR-014 appears only as an upstream-framework note and is absent from product scope outputs. |
| G068 proof gate | TR-008 has current evidence proving residual gaps or evidence-backed closure with no invented scope. |
| Regression expansion quality | Every TR-009 regression scenario names one of: CI integration job, full-stack reproduction path, or contract-test surface, and adds protection beyond existing BUG-045-002 proof. |
| Consumer trace completeness | Every identified CI workflow consumer has a disposition. |
| Blast-radius completeness | Test stack, CI workflow, contract tests, and CLI wrappers each have protected-contract and canary expectations. |
| Change boundary completeness | Every planned scope lists allowed file families and excluded surfaces. |
| Consolidation | No separate 054 artifact is created for this work. |

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Invented G068 gaps | Creates unnecessary scope and undermines audit trust. | Require current traceability evidence before any TR-008 work is planned. |
| Scope creep into runtime implementation | Violates planning-only workflow and artifact ownership. | Keep this spec source-free and route design/scopes to owning agents. |
| Framework work leakage | Smackerel product scopes could attempt to edit installed Bubbles framework files. | Exclude TR-014 and cite Framework File Immutability. |
| Fragmented planning | Five separate specs would split evidence and decisions. | Keep TR-008..012 in this consolidated 053 spec. |
| Duplicate regression rows | Suite grows without new protective value. | Require each regression row to state the gap it protects beyond existing BUG-045-002 proof. |
| Incomplete consumer trace | Stale CI assumptions survive in docs, guards, or operator paths. | Require direct/indirect consumer inventory and disposition. |
| Shared-infrastructure collateral damage | CLI wrappers or contract tests change behavior outside the intended surface. | Require shared-infrastructure canaries and explicit change boundaries. |

## Traceability Matrix

| Transition Request | Business Scenario(s) | Functional Requirement(s) | Required Planning Output | Disposition Rule |
|--------------------|----------------------|---------------------------|--------------------------|------------------|
| TR-BUG-045-002-008 | SCN-053-001 | FR-053-001, FR-053-002, FR-053-003, FR-053-015 | Current G068 evidence; residual-gap list or closure-by-evidence record. | Prove residual gaps first; close with evidence if none exist. |
| TR-BUG-045-002-009 | SCN-053-002 | FR-053-001, FR-053-004, FR-053-005, FR-053-015 | Regression E2E expansion plan for CI job, full-stack repro, and contract-test surface. | Add only protection not already supplied by BUG-045-002 guard/repro evidence. |
| TR-BUG-045-002-010 | SCN-053-003 | FR-053-001, FR-053-006, FR-053-007, FR-053-015 | Consumer inventory and disposition table for CI workflow consumers. | Every consumer gets required-change, no-change-with-evidence, or owner-routed disposition. |
| TR-BUG-045-002-011 | SCN-053-004 | FR-053-001, FR-053-008, FR-053-009, FR-053-015 | Shared-infrastructure blast-radius plan covering test stack, CI workflow, contract tests, and CLI wrappers. | Canary expectations precede broad validation when protected surfaces change. |
| TR-BUG-045-002-012 | SCN-053-005 | FR-053-001, FR-053-010, FR-053-011, FR-053-015 | Change boundary and G040 wrapper disposition plan. | Every scope lists allowed file families, excluded surfaces, and wrapper disposition. |
| TR-BUG-045-002-014 | SCN-053-006 | FR-053-012 | Upstream-framework note only. | Excluded from Smackerel product scope. |
| Consolidated-spec decision | SCN-053-007 | FR-053-013, FR-053-014 | This spec folder only. | Do not create 054 in this invocation. |

## Upstream Framework Note: TR-BUG-045-002-014

TR-BUG-045-002-014 concerns Bubbles framework guard behavior: G061 transition request counting, rework queue counting, completed-scope line counting, and G022 phase provenance modeling. Those surfaces are framework-owned. Smackerel may retain references to TR-014 as context for why BUG-045-002 ended `done_with_concerns`, but this product spec must not plan edits to `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, or other installed framework files.

## Successor Owner Guidance

- `bubbles.design` should define the operational evidence architecture, decision records, and owner boundaries for TR-008..012.
- `bubbles.plan` should create scopes after design, preserving the no-invented-gaps rule for TR-008 and the consolidated spec boundary for TR-008..012.
- `bubbles.workflow` or the upstream Bubbles repository remains the owner for TR-014.
