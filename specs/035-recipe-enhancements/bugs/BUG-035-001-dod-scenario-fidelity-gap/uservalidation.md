# User Validation Checklist — BUG-035-001 (DoD scenario fidelity gap, full close-out)

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json) | [state.json](state.json)

## Checklist

- [x] Baseline checklist initialized for this bug
- [x] **Trace guard PASSED:** `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` exits 0 with `RESULT: PASSED (0 warnings)` and shows `DoD fidelity scenarios: 50 (mapped: 50, unmapped: 0)`. Evidence: [report.md → Test Evidence A](report.md#a-trace-guard).
- [x] **Parent spec artifact lint PASSED:** `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements` exits 0 with `Artifact lint PASSED.`. Evidence: [report.md → Test Evidence B](report.md#b-parent-spec-artifact-lint).
- [x] **Bug folder artifact lint PASSED:** `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-001-dod-scenario-fidelity-gap` exits 0 with `Artifact lint PASSED.`. Evidence: [report.md → Test Evidence E](report.md#e-bug-folder-artifact-lint).
- [x] **Regression baseline guard PASSED:** `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/035-recipe-enhancements --verbose` exits 0 with `Regression baseline guard: PASSED`. Evidence: [report.md → Test Evidence C](report.md#c-regression-baseline-guard).
- [x] **Repo CLI check PASSED:** `./smackerel.sh check` exits 0 with `Config is in sync with SST` and `scenario-lint: OK`. Evidence: [report.md → Test Evidence D](report.md#d-repo-cli-check).
- [x] **No production code changed:** `git status --porcelain` lists only the parent `specs/035-recipe-enhancements/scopes.md` and the 6 bug-folder artifacts. No test files, no sibling spec artifacts, no framework files (`.github/bubbles/`, `.github/agents/`, `.github/instructions/`, `.github/skills/`), no user WIP files (`.github/workflows/build.yml`, `deploy/contract.yaml`, `deploy/self-hosted/manifest.yaml`, `deploy/self-hosted/params.yaml`) modified. Evidence: [report.md → Validation Phase](report.md#validation-phase--2026-05-08).
- [x] **Original DoD intent preserved for parked scopes:** Every parked Phase B scope's Gherkin scenarios, Implementation Plan, Test Plan, and DoD bullets remain verbatim under `## Parked Scope NN: <Name>` headings. The parking reclassification only renamed the headings and added inventory/contract documentation; no scope body content was deleted, weakened, or rewritten. `bubbles.plan` re-promotes them when each dependency gate clears.

Unchecked items indicate a user-reported regression. All entries default to `[x]` because validation just confirmed the working state via the lint/guard chain run-throughs in `report.md`.
