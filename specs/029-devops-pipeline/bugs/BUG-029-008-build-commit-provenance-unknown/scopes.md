# Scopes: BUG-029-008 — Git-derive `SMACKEREL_COMMIT` for local-operator builds

> **Plan Status:** Fixed and certified `done` (2026-07-19). The git-working-tree derivation of
> `SMACKEREL_COMMIT` in `scripts/commands/config.sh` (plus the committed config-generator regression
> test) is shipped in `0f2fb517` and git-verified present at HEAD `1bfd18a0f357`; all DoD items are
> complete with fresh current-session evidence; the full bugfix-fastlane specialist pipeline executed
> and `state-transition-guard` passes all gates.
>
> **Mode:** `bugfix-fastlane`  ·  **Release Train:** `mvp`
>
> **Authoritative inputs:** [bug.md](bug.md), [spec.md](spec.md), [design.md](design.md),
> [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

| # | Scope | Owner | Depends On | Status |
|---|-------|-------|------------|--------|
| 1 | Git-derive `SMACKEREL_COMMIT` for local-operator builds (fail-safe, CI-precedence-preserving) | `bubbles.implement` | none | Done |

## Scope 1: Git-derive `SMACKEREL_COMMIT` for local-operator builds (fail-safe, CI-precedence-preserving)

**Scope-Kind:** contract-only

**Status:** Done

**Owner:** `bubbles.implement`

**Depends On:** none

> Contract-only: the in-repo deliverable is a config-generator behavior change proven by a
> config-generator regression test that runs the generator twice against the live
> `config/smackerel.yaml` and asserts the emitted `SMACKEREL_COMMIT`. The "running images become
> self-identifying" property is a build/deploy characteristic; its live evidence is the committed
> redteam observation (bug.md), and the fresh post-rebuild `docker inspect` re-check is owned by
> `bubbles.devops` as a non-gating operational step. This scope declares no runtime-behavior E2E
> rows (there is no in-repo live E2E surface for a config-generate value; a full self-hosted image
> build is explicitly excluded by the finding).

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: locally-built images carry the real source SHA (BUG-029-008)

  Scenario: Local-operator build derives the real source SHA when CI has not exported it
    Given SMACKEREL_COMMIT is unset (the local-operator / <deploy-host> build path, no CI env)
    And config generate runs inside a git checkout
    When scripts/commands/config.sh resolves build metadata into config/generated/dev.env
    Then dev.env carries a real 12-hex git SHA (optionally -dirty), never the literal "unknown"

  Scenario: CI / shell-env SMACKEREL_COMMIT export precedence is preserved
    Given SMACKEREL_COMMIT=<sentinel> is exported before config generate runs
    When scripts/commands/config.sh resolves build metadata into config/generated/dev.env
    Then dev.env carries the exported sentinel verbatim (the git-derivation arm runs ONLY when unset)
```

> Two secondary code behaviors — the `-dirty` suffix on a dirty tree (FR-03) and the `unknown`
> fallback outside a git checkout (FR-04) — are documented design behaviors (design.md → DD-3/DD-4),
> covered respectively by Sub-test 1's `(-dirty)?` regex tolerance and the resolver's `else` fallback
> branch. They are NOT declared as separate Gherkin scenarios because the automated regression does
> not force a dirty tree or a non-git directory; their presence is asserted read-only via Code Diff
> Evidence, keeping every declared scenario mapped to a concrete asserting sub-test.

### Implementation Files

- `scripts/commands/config.sh` — the git-derivation arm (`git -C "$REPO_ROOT" rev-parse --short=12
  HEAD`, `-dirty` suffix on a dirty tree, `unknown` fallback outside a git checkout), gated by
  `[[ -z "${SMACKEREL_COMMIT+set}" ]]` so a CI export wins. Shipped in `0f2fb517`, present at HEAD.
- `scripts/commands/config_build_commit_provenance_test.sh` — the config-generator regression
  (Sub-test 1 unset ⇒ real SHA; Sub-test 2 exported ⇒ preserved). Shipped in `0f2fb517`.
- `internal/config/sst_loader_build_commit_provenance_test.go` — the Go driver
  `TestSSTLoader_BuildCommitProvenance_BUG029008` (runs the shell test under
  `./smackerel.sh test unit --go`). Shipped in `0f2fb517`.

The already-correct downstream contract (`docker-compose.yml` `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}`,
`Dockerfile` / `ml/Dockerfile` `ARG COMMIT_HASH` → `LABEL org.opencontainers.image.revision` +
`-ldflags -X main.commitHash`) is unchanged and documented with git-backed proof in report.md →
"Code Diff Evidence".

### Change Boundary

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| files in this bug directory (`specs/029-devops-pipeline/bugs/BUG-029-008-build-commit-provenance-unknown/`) | any source file (`scripts/commands/config.sh`, the two test files) — verified READ-ONLY, NOT re-changed |
| — | the already-correct `docker-compose.yml` / `Dockerfile` / `ml/Dockerfile` wiring |
| — | the parent spec 029 artifacts (`state.json`, `report.md`, `spec.md`, `design.md`, `scopes.md`) |
| — | `internal/config/release_trains_contract_test.go` (pre-existing, unrelated gofmt finding — outside this boundary) |
| — | any build, deploy, host mutation, or push; any unrelated spec / bug / deploy config |

### Test Plan

| ID | Scenario | Category | Location / Command Surface | Required Assertion |
|----|----------|----------|----------------------------|--------------------|
| `TP-1` | Local-operator build derives real SHA | `unit` adversarial | `internal/config/sst_loader_build_commit_provenance_test.go::TestSSTLoader_BuildCommitProvenance_BUG029008` (Sub-test 1) via `./smackerel.sh test unit --go` | unset ⇒ `dev.env::SMACKEREL_COMMIT` matches `^[0-9a-f]{12}(-dirty)?$`, never `unknown` |
| `TP-2` | CI precedence preserved | `unit` adversarial | same Go driver (Sub-test 2) via `./smackerel.sh test unit --go` | exported sentinel `cafef00dba11` ⇒ preserved verbatim in `dev.env` |
| `TP-3` | Adversarial regression quality | `guard` | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix scripts/commands/config_build_commit_provenance_test.sh` | adversarial signal detected, 0 violations, no silent-pass bailout |
| `TP-4` | Config generator integrity | `functional` | `./smackerel.sh check` | config in sync with SST, env-file drift OK (exit 0) |
| `TP-5` | Running images self-identify | build/deploy (proof-of-record) | committed redteam observation (bug.md) + `bubbles.devops` post-rebuild `docker inspect` re-check | after rebuild+redeploy, `org.opencontainers.image.revision` = the source SHA (non-gating) |

**Test Plan ↔ DoD parity:** `TP-1`..`TP-5` map to the scenario / regression / integrity / live DoD
items below.

### Definition of Done

- [x] Local-operator build derives the real source SHA when CI has not exported it (SCN-001 / `TP-1`,
  adversarial) — unset ⇒ `dev.env::SMACKEREL_COMMIT` is a real 12-hex SHA, never `unknown`.
  - Evidence: report.md → "Test Evidence" GREEN (Sub-test 1 emitted `1bfd18a0f357` = HEAD SHA) + "Code Diff Evidence" (git-derivation arm at `config.sh:2213`).
- [x] CI / shell-env export precedence is preserved (SCN-002 / `TP-2`, adversarial) — an exported
  `SMACKEREL_COMMIT` sentinel wins verbatim.
  - Evidence: report.md → "Test Evidence" GREEN (Sub-test 2 emitted `cafef00dba11`) + "Scenario-First TDD" (the arm is gated by `[[ -z "${SMACKEREL_COMMIT+set}" ]]`).
- [x] A locally-modified build never claims a clean SHA (FR-03, design-backed code behavior) — dirty
  tree ⇒ `-dirty` suffix.
  - Evidence: design.md → DD-3/DD-4 + report.md → "Code Diff Evidence" (`config.sh:2214-2215` dirty arm) + Sub-test 1 regex `^[0-9a-f]{12}(-dirty)?$` tolerates the suffix.
- [x] A non-git source (tarball) still generates (FR-04, design-backed code behavior) — `git
  rev-parse` failure ⇒ `unknown` fallback, config generation not blocked.
  - Evidence: report.md → "Code Diff Evidence" (`config.sh:2220-2221` `else SMACKEREL_COMMIT="unknown"`).
- [x] Static regression proven FRESH this session — `./smackerel.sh test unit --go --go-run
  TestSSTLoader_BuildCommitProvenance_BUG029008` passes GREEN with exit 0 (`TP-1`+`TP-2`).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Provenance Unit Lane" (`--- PASS`, `ok internal/config`, `UNIT_GO_EXIT=0`).
- [x] Pre-fix adversarial regression FAILS (RED) and post-fix PASSES (GREEN).
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (RED prior-session: `SMACKEREL_COMMIT is not a real git SHA — actual: 'unknown'` with the arm stashed → GREEN this session: `1bfd18a0f357`).
- [x] Adversarial regression contains no silent-pass bailout patterns and carries an adversarial
  signal (`TP-3`).
  - Evidence: report.md → "Fresh Adversarial Regression Guard" (adversarial signal detected, 0 violations / 0 warnings, `RQG_EXIT=0`).
- [x] Root cause confirmed and documented (bug.md + design.md).
  - Evidence: report.md → "Root Cause" + design.md → "Root Cause Analysis" + the live redteam observation in bug.md.
- [x] Fix present at HEAD, git-verified, downstream wiring intact.
  - Evidence: report.md → "Code Diff Evidence" (`0f2fb517`; git-derivation arm at `config.sh:2213-2221`; compose `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}` at lines 83/180; Dockerfile ARG/LABEL/ldflags at lines 16/20/22/41/44).
- [x] All existing tests pass (no regressions) — the `internal/config` package is GREEN.
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Provenance Unit Lane" (`ok github.com/smackerel/smackerel/internal/config`).
- [x] Config generator integrity clean (`TP-4`) — `./smackerel.sh check` exit 0.
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Check" (`CHECK_RC=0`, config in sync with SST, env-file drift OK).
- [x] Change Boundary is respected and zero excluded file families were changed — the source fix is
  verified read-only (NOT re-changed); the certification packet touches ONLY the BUG-029-008 bug
  folder.
  - Evidence: report.md → "Audit Evidence" (`git status --short` clean before/after; packet mutations confined to the bug folder) + "Code Diff Evidence" (source fix in `0f2fb517`, untouched here).
- [x] Bug marked as Fixed in bug.md.
  - Evidence: bug.md Status line flipped to FIXED & VERIFIED at certification; state.json `status` = `done` and `certification.status` = `done`.
- [x] Live "running images self-identify" outcome certified on the source fix + the static regression;
  the fresh rebuild + signed redeploy + `docker inspect` re-check is owned by `bubbles.devops` as a
  non-gating step (`TP-5`, `redeployRequired: true`).
  - Evidence: bug.md redteam observation (live `revision=unknown` pre-fix) + report.md → "Redeploy / Live-Verification Note" (mechanism unit-proven; running-image restamp routed to bubbles.devops non-gating).
- [x] Build Quality Gate: `check` and `lint` clean of any BUG-029-008 delta; the full eight-phase
  bugfix-fastlane pipeline recorded with fresh evidence.
  - Evidence: report.md → "Current-Session Re-Verification" (`CHECK_RC=0`, `LINT_RC=0`; `format --check` names ONLY the pre-existing unrelated `internal/config/release_trains_contract_test.go`, outside this boundary) + "Parent-Expanded Specialist Phase Evidence" (implement, test, regression, simplify, stabilize, security, validate, audit).
