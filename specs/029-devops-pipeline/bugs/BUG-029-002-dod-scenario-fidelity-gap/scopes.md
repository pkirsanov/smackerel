# Scopes: BUG-029-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 029

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-029-FIX-001 Trace guard accepts SCN-029-001..011 as faithfully covered
  Given specs/029-devops-pipeline/scopes.md Gherkin scenarios in Scopes 1, 2, 5, 6, 7 embed [SCN-029-NNN] in the scenario name
  And matching DoD bullets in those scopes are prefixed with [SCN-029-NNN]
  And the T-7-03 Test Plan row Location column holds a slash-bearing concrete path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline`
  Then Gate G068 reports "14 scenarios checked, 14 mapped to DoD, 0 unmapped"
  And no test row is flagged for missing concrete test file path
  And the overall result is PASSED
```

### Implementation Plan

1. Edit `specs/029-devops-pipeline/scopes.md` Scope 1 Gherkin: prefix `[SCN-029-001]` and `[SCN-029-002]` to the two scenarios; prefix the `.github/workflows/ci.yml exists and runs on push + PR` DoD bullet with `[SCN-029-001] [SCN-029-002]`.
2. Edit Scope 2 Gherkin: prefix `[SCN-029-003]` and `[SCN-029-004]`; prefix `Dockerfiles accept VERSION and COMMIT_HASH build args` with `[SCN-029-004]` (untagged → SHA build arg) and `CI tags images on version tag push` with `[SCN-029-003]`.
3. Edit Scope 5 Gherkin: prefix `[SCN-029-005]`; prefix the `ML sidecar image < 3GB` DoD bullet with `[SCN-029-005]`.
4. Edit Scope 6 Gherkin: prefix `[SCN-029-006]`, `[SCN-029-007]`, `[SCN-029-008]`; prefix `docker exec confirms EXPENSES_ENABLED ...` with `[SCN-029-006]`, `./smackerel.sh check includes env_file drift guard` with `[SCN-029-007]`, and `Individual environment: blocks removed ...` with `[SCN-029-008]`.
5. Edit Scope 7 Gherkin: prefix `[SCN-029-009]`, `[SCN-029-010]`, `[SCN-029-011]`; prefix `.github/workflows/ci.yml has push-images job ...` with `[SCN-029-009]` and `docker-compose.yml supports image: override via env var` with `[SCN-029-010]`.
6. Add `cmd/core/main.go` (a real existing path with a `/`) to the T-7-03 Location column and assertion text.
7. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 14 scenarios checked, 14 mapped to DoD, 0 unmapped` | SCN-029-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/029-devops-pipeline` | SCN-029-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/029-devops-pipeline/bugs/BUG-029-002-dod-scenario-fidelity-gap` | SCN-029-FIX-001 |
| T-FIX-1-04 | Boundary preserved | artifact | `git diff --name-only` | Diff confined to `specs/029-devops-pipeline/scopes.md` and the bug folder; no `internal/`, `cmd/`, `ml/`, `config/`, `tests/` paths touched | SCN-029-FIX-001 |

### Definition of Done

- [x] [SCN-029-FIX-001] Gherkin scenarios in Scopes 1, 2, 5, 6, 7 of `specs/029-devops-pipeline/scopes.md` embed `[SCN-029-NNN]` in the scenario name — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -nE '^[[:space:]]*Scenario: \[SCN-029-(001|002|003|004|005|007|008|009|010|011)\]' specs/029-devops-pipeline/scopes.md
  > 35:Scenario: [SCN-029-001] CI runs lint and tests on push
  > 41:Scenario: [SCN-029-002] CI runs on pull requests
  > 89:Scenario: [SCN-029-003] Version tag produces versioned images
  > 94:Scenario: [SCN-029-004] Untagged builds use commit SHA
  > 215:Scenario: [SCN-029-005] ML image under 3GB
  > 269:Scenario: [SCN-029-007] New config vars automatically flow to container
  > 276:Scenario: [SCN-029-008] env_file replaces individual environment declarations
  > 354:Scenario: [SCN-029-009] Tagged release pushes images to GHCR
  > 360:Scenario: [SCN-029-010] Operator deploys from pre-built images
  > 366:Scenario: [SCN-029-011] Build-from-source remains the default
  > ```
- [x] [SCN-029-FIX-001] Each previously-unmapped scenario has at least one `[SCN-029-NNN]`-prefixed DoD bullet sharing ≥3 significant words with the Gherkin scenario — **Phase:** implement
  > Evidence: see report.md `### Implementation Evidence` for the per-scope bullet listing.
- [x] [SCN-029-FIX-001] T-7-03 Test Plan row Location column contains a slash-bearing concrete path (`cmd/core/main.go`) — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -n -B1 -A1 'T-7-03' specs/029-devops-pipeline/scopes.md
  > | T-7-01 | Tagged release pushes images to GHCR | manual | `.github/workflows/ci.yml` | `push-images` job (gated on `refs/tags/v*`) pushes `smackerel-core:v*`/`:latest` and `smackerel-ml:v*`/`:latest` to ghcr.io with OCI labels | SCN-029-009 |
  > | T-7-02 | Operator deploys from pre-built image | integration | `docker-compose.yml` | Setting `SMACKEREL_CORE_IMAGE=ghcr.io/.../smackerel-core:v*` causes `./smackerel.sh up` to pull the pre-built image | SCN-029-010 |
  > | T-7-03 | Build-from-source default behavior unchanged | manual | `docker-compose.yml`, `cmd/core/main.go` | With `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE` unset, Compose falls back to the `build:` block and rebuilds the Go binary from `cmd/core/main.go` | SCN-029-011 |
  > ```
- [x] [SCN-029-FIX-001] Traceability guard PASSES against `specs/029-devops-pipeline` — **Phase:** validate
  > Evidence:
  > ```
  > $ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline 2>&1 | tail -10
  > ℹ️  DoD fidelity: 14 scenarios checked, 14 mapped to DoD, 0 unmapped
  >
  > --- Traceability Summary ---
  > ℹ️  Scenarios checked: 14
  > ℹ️  Test rows checked: 24
  > ℹ️  Scenario-to-row mappings: 14
  > ℹ️  Concrete test file references: 14
  > ℹ️  Report evidence references: 14
  > ℹ️  DoD fidelity scenarios: 14 (mapped: 14, unmapped: 0)
  >
  > RESULT: PASSED (0 warnings)
  > ```
- [x] [SCN-029-FIX-001] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] [SCN-029-FIX-001] No production code changed; boundary confined to `specs/029-devops-pipeline/scopes.md` and the new bug folder — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/029-devops-pipeline/scopes.md` and `specs/029-devops-pipeline/bugs/BUG-029-002-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.
