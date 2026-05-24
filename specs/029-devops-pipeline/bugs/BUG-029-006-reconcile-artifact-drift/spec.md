# Spec: BUG-029-006 — Reconcile Spec 029 Artifact Drift To Current Gate Standards

**Parent Spec:** 029-devops-pipeline
**Discovered:** 2026-05-23 (stochastic sweep round 23, trigger=devops, mapped child workflow=devops-to-doc)
**Mode:** bugfix-fastlane (artifact reconciliation; zero runtime change)

## Use Cases

- **UC-01 — Sweep round closes 38 governance findings cleanly.** When the stochastic sweep parent dispatches `devops-to-doc` against `specs/029-devops-pipeline`, the orchestrator (a) probes the live devops surface, (b) finds zero functional drift, (c) creates this BUG packet to reconcile legacy artifact drift, and (d) brings `state-transition-guard.sh` to 0 BLOCKs without touching production code, tests, or runtime configuration.
- **UC-02 — Future audit can trace every claimed phase to an executionHistory entry.** When a reviewer cross-checks `state.json::completedPhaseClaims` against `executionHistory`, every claimed phase MUST have at least one matching `bubbles.<phase>` entry (G022/G022-extension).
- **UC-03 — Future audit can verify per-scope regression coverage.** Every scope's DoD MUST cite scenario-specific regression E2E tests and the broader integration suite (G016 / Check 8A), and every Test Plan MUST include a Regression E2E row.
- **UC-04 — Future audit can verify Build-Once Deploy-Many compliance claims via Code Diff Evidence.** The parent `report.md` MUST include a Code Diff Evidence + Git-Backed Proof section enumerating every implementation-bearing file touched (G053 / Check 13B).

## Functional Requirements

- **FR-01 — G016 closure.** Each of Scopes 1-7 in `specs/029-devops-pipeline/scopes.md` MUST contain (a) a `| Regression E2E | …` row in the Test Plan table referencing the BUG-029-006-SCN-001 scenario and concrete test functions, (b) a `Scenario-specific E2E regression tests …` DoD bullet, and (c) a `Broader E2E regression suite passes …` DoD bullet — totalling 21 G016 additions.
- **FR-02 — G026 closure (Scope 5).** Scope 5 (ML Sidecar Image Optimization) MUST contain a `### Consumer Impact Sweep` section, a `- [x] Consumer Impact Sweep complete: zero stale first-party references remain` DoD bullet, and explicit enumeration of affected consumer surfaces using at least one of the keywords `navigation|breadcrumb|redirect|API client|generated client|deep link|stale-reference`.
- **FR-03 — G022 + G022-extension closure.** `state.json::executionHistory` MUST include retroactive entries for `bubbles.bootstrap`, `bubbles.regression`, `bubbles.simplify`, `bubbles.stabilize`, and `bubbles.security` (parent-expanded-specialist execution model), and `completedPhaseClaims` + `certifiedCompletedPhases` MUST extend to cover all four canonical missing phases.
- **FR-04 — G053 closure.** `report.md` MUST gain a `### Code Diff Evidence` subsection and `### Git-Backed Proof` block enumerating every implementation-bearing file changed under spec 029 history.
- **FR-05 — G057 + G060 + G040 + Check 17 closure.** `scenario-manifest.json` MUST add `requiredTestType` to SCN-029-012, SCN-029-013, SCN-029-014, SCN-029-015 (G057). `scopes.md` and `report.md` MUST contain scenario-first TDD evidence markers covering red→green / scenario-first / tdd vocabulary (G060). `scopes.md` L316/L319 MUST be rewritten to remove `deferred to integration validation` language while preserving truthful evidence pointers (G040). The closure commit MUST use the structured prefix `bubbles(029/bug-029-006)` (Check 17).

## Acceptance Criteria

- **AC-01** — `bash .github/bubbles/scripts/state-transition-guard.sh specs/029-devops-pipeline` exits 0 with `BLOCKING ISSUES (Required for "done"): 0`.
- **AC-02** — `bash .github/bubbles/scripts/state-transition-guard.sh specs/029-devops-pipeline/bugs/BUG-029-006-reconcile-artifact-drift` exits 0 with `BLOCKING ISSUES (Required for "done"): 0`.
- **AC-03** — `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline` returns `PASSED`.
- **AC-04** — `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-006-reconcile-artifact-drift` returns `PASSED`.
- **AC-05** — `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline` returns `PASSED`.
- **AC-06** — `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline/bugs/BUG-029-006-reconcile-artifact-drift` returns `PASSED`.
- **AC-07** — All 7 scopes in `specs/029-devops-pipeline/scopes.md` cite scenario-specific regression E2E coverage + broader integration suite DoD bullets, and Test Plan tables include the Regression E2E row referencing BUG-029-006-SCN-001.
- **AC-08** — Scope 5 contains Consumer Impact Sweep section + DoD bullet + enumerated affected surfaces (keyword pattern matched).
- **AC-09** — `state.json::completedPhaseClaims` contains `regression`, `simplify`, `stabilize`, `security` (in addition to pre-existing `bootstrap`, `implement`, `test`, `validate`, `audit`, `docs`, `chaos`, `spec-review`), and `executionHistory` contains matching `bubbles.bootstrap` / `bubbles.regression` / `bubbles.simplify` / `bubbles.stabilize` / `bubbles.security` entries.
- **AC-10** — Closure commit is single, prefix `bubbles(029/bug-029-006)`, touches ONLY paths under `specs/029-devops-pipeline/` (no stray edits to spec 055, internal/, cmd/, scripts/, config/, .github/workflows/, deploy/, or any other spec folder's `state.json`).
