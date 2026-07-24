# Spec 106 Planning Report

Links: [scope index](scopes/_index.md) | [user validation](uservalidation.md) | [scenario manifest](scenario-manifest.json) | [test plan](test-plan.json)

## Summary

This root report records planning-owner validation only. Implementation, product tests, browser journeys, NFR measurements, owner dependency completion, readiness ingestion, production acceptance, deployment, certification, commit, and push have not been performed or claimed.

## Decision Record

The active plan uses 16 per-scope directories. Shared foundations precede renderer cutover; each domain surface composes only owner-delivered behavior; real disposable journeys precede BUG-032/BUG-102 projection consumption; and final acceptance emits a content-free owner handoff. The analyst-owned `spec.md` User Scenarios are the authoritative meanings for `SCN-106-001` through `SCN-106-022` where design-local technical prose reused those identifiers differently.

## Reconciliation Record (2026-07-24 harden)

This planning packet was reconciled against the 2026-07-24T05:46 spec revision during a spec-scope-hardening `harden` run by `bubbles.plan`. Status was set to `in_progress` (non-terminal); no product source, tests, certification, commit, push, or deployment was performed.

**Stale-evidence drift found (mtime table):** `spec.md` and `state.json` were revised at 2026-07-24T05:46:37Z to 22 analyst scenarios (SCN-106-001..022), 38 FRs, and 11 NFRs, while `design.md`, `report.md`, `scenario-manifest.json`, `test-plan.json`, `uservalidation.md`, `scopes/_index.md`, and all 16 per-scope files remained at the 2026-07-24T01:27:57Z generation (15 scenarios) — a 4h19m drift. `state.json.planningProvenance.validation` exit codes were `null` (packet never guard-validated).

**Changes made (planning artifacts only):**
- `scenario-manifest.json`: added SCN-106-016..022 mapped to existing scopes and already-planned tests — SCN-016→SCOPE-106-04 cross-family mutation canary; SCN-017/018/019→SCOPE-106-09 `UX-E2E-106-037/038/039`; SCN-020→SCOPE-106-15 `UX-E2E-106-071`; SCN-021→SCOPE-106-06 `UX-E2E-106-027`; SCN-022→SCOPE-106-12 `UX-E2E-106-052`. `scenarioAuthority` updated to "001 through 022". No new test file invented; every `plannedTest` stays `not-yet-authored`; `linkedTests` stay empty.
- `state.json`: `status` and `certification.status` → `in_progress`; `specScenarioCount` 15→22; validation exits recorded (0/0); `execution` set to bubbles.plan/harden; `domainOwnership.workRouteOwnership` resolution added; reconciliation `executionHistory` entry appended. Test Plan rows and DoD test items unchanged at 144==144.
- `scopes/_index.md`: scenario references 15→22; added the Work-Route Ownership And Partial-Supersession section.
- `scopes/09-work-route-composition/scope.md`: added the Work-Route Ownership section (prose only; Gherkin/Test-Plan/DoD unchanged, 7 rows / 7 items).
- `report.md` (this file): planning validation is no longer pending — this session's actual guard runs are recorded under Planning Validation.
- `test-plan.json`: unchanged — it is the per-scope machine handoff keyed by test id/scenarioId, already synchronized with the 144 unchanged Test Plan rows; it carries no analyst-scenario count.

**Work-route ownership resolution:** Spec 106 owns the route-free **Work navigation group** (null parent href) and the **Lists/Meals/Expenses browser routes and journeys** composed over the existing owner domain APIs (SCOPE-106-09; catalog registration/cutover in SCOPE-106-02/05). The Work grouping is covered by the declared partial-supersession of spec 100's fixed assistant-first inventory (`state.json.predecessorContractDisposition[100]`); the sealed artifacts of specs 100 and 092 are not edited; Cards (092, SCOPE-106-10) stays a separate top-level surface, not a Work leaf. Shell-reached Work mutations bind to BUG-070-001 `MutationTrustGuard` (403 before mutation), which spec 106 consumes.

**Chain / bindings:** BUG-102-001 produces immutable acceptance evidence → BUG-032-004 derives readiness → spec 106 renders it (SCOPE-106-15/16; `state.json.evidenceLifecycleRole`, `producesAcceptanceEvidence: false`). CSRF/Origin enforcement for shell mutations binds to BUG-070-001 `MutationTrustGuard`.

**Residuals routed:** `design.md` is stale (predates the 05:46 spec revision; does not name SCN-106-016..022 or the term `MutationTrustGuard`) but architecturally sufficient — it explicitly routes "absent browser-route ownership" to `bubbles.plan` and models Work as a route-free group. It was **not edited**; a design-refresh residual is routed to `bubbles.design`. Ten bug packets remain `in_progress` and spec 105 remains `not_started` — entry gates, not planning work.

## Completion Statement

Planning packet construction is subject to the validation evidence recorded below. All implementation DoD items remain unchecked and every scope remains Not Started.

## Test Evidence

No implementation, unit, integration, e2e-api, e2e-ui, stress, load, browser, or runtime evidence is recorded by the planning owner.

## Planning Validation

Both canonical guards were executed in this session (2026-07-24 harden). Both PASSED at baseline and again after reconciliation; this is the first guard validation of the packet.

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/106-coherent-product-experience` — exit 0.

```text
✅ No repo-CLI bypass detected in scopes/12-sources-activity-admin-projections/report.md command evidence
✅ No repo-CLI bypass detected in scopes/13-responsive-accessibility-hardening/report.md command evidence
✅ No repo-CLI bypass detected in scopes/14-disposable-product-journeys-nfr/report.md command evidence
✅ No repo-CLI bypass detected in scopes/15-readiness-acceptance-projection/report.md command evidence
✅ No repo-CLI bypass detected in scopes/16-final-acceptance-handoff/report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/106-coherent-product-experience` — exit 0.

```text
ℹ️  DoD fidelity: 29 scenarios checked, 29 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 29
ℹ️  Test rows checked: 160
ℹ️  Scenario-to-row mappings: 29
ℹ️  Concrete test file references: 29
ℹ️  Report evidence references: 29
ℹ️  DoD fidelity scenarios: 29 (mapped: 29, unmapped: 0)
ℹ️  Edge confidence (IMP-015 Scope B): declared=44 inferred=1 ambiguous=13

RESULT: PASSED (0 warnings)
TRACE_GUARD_EXIT=0
```

G057/G059 manifest coverage and G068 DoD fidelity (29/29 mapped, 0 unmapped) pass; scope-local traceability is clean. No residual trace exit 1. No evidence reference was fabricated; every planned test remains `not-yet-authored` and no Test Plan row or DoD item was added (144==144 preserved).

## Uncertainty Declarations

Implementation outcomes are intentionally unverified because execution has not begun. External owner dependencies and their observed planning statuses are recorded in `state.json` and the scope index.

## Scenario Contract Evidence

`scenario-manifest.json` contains planning-only `plannedTests` entries and no nonexistent test under `linkedTests`. `test-plan.json` is the machine-readable test handoff and must remain synchronized with every scope Test Plan table and DoD test item.

## Coverage Report

Planning coverage targets all 22 analyst scenarios, all 72 UX-E2E rows, every supported product surface, and NFR-106-001/002 stress/load proof. Coverage percentages are not claimed before tests exist and execute. SCN-106-016..022 are covered through `scenario-manifest.json` mappings to existing scopes and already-planned tests; no Test Plan row or DoD item was added (144==144 preserved).

## Lint/Quality

No planning validation result is claimed until current-session command output is recorded under Planning Validation.

## Spot-Check Recommendations

Harden must verify taxonomy completeness, exact Markdown/JSON parity, scenario semantics, owner-boundary non-duplication, and route realism before implementation pickup.

## Validation Summary

Planning reconciliation validated in this session: `artifact-lint.sh` exit 0 (PASSED) and `traceability-guard.sh` exit 0 (PASSED, 0 warnings, G068 DoD fidelity 29/29 mapped). G057/G059 manifest coverage and G068 DoD fidelity pass; no residual trace exit 1. Status remains `in_progress` (non-terminal); no certification, commit, push, or deployment is claimed.

## Audit Verdict

No audit or certification verdict is claimed.
