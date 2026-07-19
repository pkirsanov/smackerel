# Scopes: BUG-050-001 — Reflect the ML sidecar's real `model_loaded` end-to-end

> **Plan Status:** Fixed and certified `done` (2026-07-19). Core's `checkMLSidecar` now decodes the
> ML sidecar's `/health` body and REFLECTS its `status` + `model_loaded` (instead of hardcoding
> `up`/`true`), and the ML sidecar's own `/health` reads the embedder's live state via
> `is_model_loaded()` (instead of a frozen import-time snapshot). The fix is shipped in `f26dbdd9`
> and git-verified present at HEAD `d8871e2728d0`; all DoD items are complete with fresh
> current-session evidence (Go + Python unit lanes GREEN, regression guard `0 violations`,
> check/lint clean); the full bugfix-fastlane specialist pipeline executed and `state-transition-guard`
> passes all gates.
>
> **Mode:** `bugfix-fastlane`  ·  **Release Train:** `mvp`
>
> **Authoritative inputs:** [bug.md](bug.md), [spec.md](spec.md), [design.md](design.md),
> [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

| # | Scope | Owner | Depends On | Status |
|---|-------|-------|------------|--------|
| 1 | Reflect the ML sidecar's real `model_loaded` end-to-end (core decodes the body; sidecar reports live state) | `bubbles.implement` | none | Done |

## Scope 1: Reflect the ML sidecar's real `model_loaded` end-to-end (core decodes the body; sidecar reports live state)

**Scope-Kind:** contract-only

**Status:** Done

**Owner:** `bubbles.implement`

**Depends On:** none

> Contract-only: the in-repo deliverable is a probe/aggregate CONTRACT proven by contract-level unit
> tests, not a live E2E surface. The core contract is the ML `/health` JSON contract
> (`{status, model_loaded}`) → the `services.ml_sidecar` aggregate mapping in `checkMLSidecar`
> (`internal/api/health.go`), proven by `httptest`-server contract tests; the sidecar contract is the
> embedder-state read in the ML `/health` (`ml/app/main.py` + `ml/app/embedder.py`), proven by the
> monkeypatched-embedder `ml/tests/test_health_model_loaded.py` regression suite. The end-to-end "core
> `/api/health` reflects a RUNNING ML sidecar's real `model_loaded`" property is an aggregate/deploy
> characteristic with no in-repo live E2E surface (it requires a running core + ML sidecar + ollama
> stack); its live evidence is the committed redteam observation (bug.md), and the fresh full-stack
> re-check is owned by `bubbles.devops` as a non-gating operational step. This scope therefore
> declares no runtime-behavior E2E rows (v4.1.0 `contract-only` opt-out); every declared scenario maps
> to a concrete contract-level asserting test.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: core /api/health reflects the ML sidecar's real model_loaded (BUG-050-001)

  Scenario: Core reflects model_loaded:false instead of fabricating true
    Given the ML sidecar GET /health returns 200 with {"status":"up","model_loaded":false}
    When core's checkMLSidecar builds the services.ml_sidecar aggregate entry
    Then core reports model_loaded:false reflected from the body, never the fabricated true

  Scenario: Core reflects a self-declared degraded sidecar status
    Given the ML sidecar GET /health returns 200 with {"status":"degraded","model_loaded":false}
    When core's checkMLSidecar builds the aggregate entry
    Then core reports status:degraded reflected from the body, not overridden to up

  Scenario: Core does not fabricate a model_loaded claim on a reachable-but-unparseable body
    Given the ML sidecar returns 200 with an empty or unparseable body
    When core's checkMLSidecar builds the aggregate entry
    Then core reports status:up with NO model_loaded claim (nil), never a fabricated value

  Scenario: The ML sidecar /health reports the embedder's live model_loaded, not a stale import
    Given app.main was imported while embedder._model was None and the model loads later on first generate_embedding()
    When GET /health is served after the lazy load reassigns embedder._model
    Then the sidecar reports model_loaded:true via embedder.is_model_loaded(), never the frozen import-time false
```

> The healthy/`down` paths (`TestCheckMLSidecar_HealthyResponse`, `TestCheckMLSidecar_UnhealthyResponse`,
> `TestCheckMLSidecar_ConnectionRefused`) are documented preservation behaviors covered by their
> named tests; they are asserted as regression guards rather than as new Gherkin scenarios, keeping
> every declared scenario mapped to a concrete asserting adversarial/reflection test.

### Implementation Files

- `internal/api/health.go` — the new `mlHealthBody` type and the rewritten `checkMLSidecar`: a bounded
  `GET <ml>/health`, non-200/transport → `down`, 200 + parseable → reflect `status` + `model_loaded`,
  200 + unparseable → `up` with no claim. Shipped in `f26dbdd9`, present at HEAD.
- `internal/api/health_test.go` — the core regression tests: `TestCheckMLSidecar_ModelNotLoaded`
  (adversarial), `TestCheckMLSidecar_DegradedBody`, `TestCheckMLSidecar_ReachableUnparseableBody`, and
  the corrected `TestCheckMLSidecar_HealthyResponse`. Shipped in `f26dbdd9`.
- `ml/app/embedder.py` — `is_model_loaded()`, a call-time read of the module global `_model`. Shipped
  in `f26dbdd9`.
- `ml/app/main.py` — `/health` calls `is_model_loaded()` instead of `from .embedder import _model`.
  Shipped in `f26dbdd9`.
- `ml/tests/test_health_model_loaded.py` — the F8 stale-binding regression suite (3 tests). Shipped in
  `f26dbdd9`.

All five files are runtime source (not spec/docs), satisfying the G053/G093 delivery-delta via the
git-backed proof in report.md → "Code Diff Evidence".

### Change Boundary

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| files in this bug directory (`specs/050-ml-sidecar-health-isolation/bugs/BUG-050-001-ml-model-loaded-fabricated/`) | any source file (`internal/api/health.go`, `internal/api/health_test.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_health_model_loaded.py`) — verified READ-ONLY, NOT re-changed |
| — | the parent spec 050 artifacts (`state.json`, `report.md`, `spec.md`, `design.md`, `scopes.md`) |
| — | the sibling `BUG-050-002` bug folder (the `disconnected`-status degraded aggregate, F1, bundled in the same commit but a separate bug) |
| — | `internal/config/release_trains_contract_test.go` (pre-existing, unrelated gofmt finding — outside this boundary) |
| — | any build, deploy, host mutation, or push; any unrelated spec / bug / deploy config |

### Test Plan

| ID | Scenario | Category | Location / Command Surface | Required Assertion |
|----|----------|----------|----------------------------|--------------------|
| `TP-1` | Core reflects `model_loaded:false` | `unit` adversarial | `internal/api/health_test.go::TestCheckMLSidecar_ModelNotLoaded` via `./smackerel.sh test unit --go` | 200 `{"model_loaded":false}` ⇒ `*status.ModelLoaded == false` (never the fabricated `true`) |
| `TP-2` | Core reflects degraded status | `unit` | `internal/api/health_test.go::TestCheckMLSidecar_DegradedBody` via `./smackerel.sh test unit --go` | 200 `{"status":"degraded"}` ⇒ `status.Status == "degraded"` (not overridden to `up`) |
| `TP-3` | Core no-fabrication on unparseable body | `unit` | `internal/api/health_test.go::TestCheckMLSidecar_ReachableUnparseableBody` via `./smackerel.sh test unit --go` | 200 + empty body ⇒ `status.Status == "up"` AND `status.ModelLoaded == nil` |
| `TP-4` | Sidecar reports live `model_loaded` | `unit` adversarial | `ml/tests/test_health_model_loaded.py::test_health_model_loaded_tracks_embedder_not_stale_import` + `…_true_after_generate_embedding` via `./smackerel.sh test unit --python` | model loaded after import ⇒ `/health["model_loaded"] is True` (not the frozen import-time `False`) |
| `TP-5` | Healthy / down paths preserved | `unit` | `TestCheckMLSidecar_HealthyResponse` / `_UnhealthyResponse` / `_ConnectionRefused` via `./smackerel.sh test unit --go` | `{"model_loaded":true}` ⇒ `up`+`true`; non-200 / unreachable ⇒ `down` |
| `TP-6` | Adversarial regression quality | `guard` | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/health_test.go` | adversarial signal detected, 0 violations, no silent-pass bailout |
| `TP-7` | Go unit lane integrity | `unit` | `./smackerel.sh test unit --go --go-run 'TestCheckMLSidecar|TestHealthHandler_MLSidecar'` | `ok internal/api`, exit 0 |
| `TP-8` | Python unit lane integrity | `unit` | `./smackerel.sh test unit --python` | `ml/tests` GREEN (`622 passed, 2 skipped`), exit 0 |
| `TP-9` | Live aggregate reflects the running sidecar | e2e (proof-of-record) | committed redteam observation (bug.md) + `bubbles.devops` full-stack re-check | after redeploy, core `/api/health` `services.ml_sidecar.model_loaded` mirrors the sidecar's own `/health` (non-gating) |

**Test Plan ↔ DoD parity:** `TP-1`..`TP-9` map one-to-one to the scenario / preservation / lane /
regression / live DoD items below.

### Definition of Done

- [x] Core reflects `model_loaded:false` from the sidecar body (SCN-001 / `TP-1`, adversarial) — never
  the fabricated `true`.
  - Evidence: report.md → "Test Evidence" GREEN (`--- PASS: TestCheckMLSidecar_ModelNotLoaded`) + "Code Diff Evidence" (`checkMLSidecar` body decode at `internal/api/health.go`).
- [x] Core reflects a self-declared `degraded` sidecar status (SCN-002 / `TP-2`).
  - Evidence: report.md → "Test Evidence" GREEN (`--- PASS: TestCheckMLSidecar_DegradedBody`).
- [x] Core reports `up` with NO `model_loaded` claim on a reachable-but-unparseable body (SCN-003 /
  `TP-3`) — the anti-fabrication half of the fix.
  - Evidence: report.md → "Test Evidence" GREEN (`--- PASS: TestCheckMLSidecar_ReachableUnparseableBody`) + "Code Diff Evidence" (`json.Decode` err ⇒ `up` with no claim).
- [x] The ML sidecar `/health` reports the embedder's live `model_loaded`, not the frozen import-time
  snapshot (SCN-004 / `TP-4`, adversarial).
  - Evidence: report.md → "Test Evidence" Python GREEN (`test_health_model_loaded.py` PASS) + "Code Diff Evidence" (`is_model_loaded()` call-time read; `/health` calls it).
- [x] The healthy / `down` paths are preserved (`TP-5`) — `{"model_loaded":true}` ⇒ `up`+`true`;
  non-200 / unreachable ⇒ `down`.
  - Evidence: report.md → "Test Evidence" GREEN (`--- PASS: TestCheckMLSidecar_HealthyResponse`, `…_UnhealthyResponse`, `…_ConnectionRefused`).
- [x] Go unit lane proven FRESH this session — `./smackerel.sh test unit --go --go-run
  'TestCheckMLSidecar|TestHealthHandler_MLSidecar'` GREEN with exit 0 (`TP-7`).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Go Unit Lane" (`ok internal/api`, `UNIT_GO_EXIT=0`).
- [x] Python unit lane proven FRESH this session — `./smackerel.sh test unit --python` GREEN with
  exit 0 (`TP-8`).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Python Unit Lane" (`622 passed, 2 skipped`, `UNIT_PY_EXIT=0`).
- [x] Pre-fix adversarial regression FAILS (RED) and post-fix PASSES (GREEN).
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (Go RED prior-session `--- FAIL: TestCheckMLSidecar_ModelNotLoaded` with the fix stashed → GREEN this session).
- [x] Adversarial regression contains no silent-pass bailout patterns and carries an adversarial
  signal (`TP-6`).
  - Evidence: report.md → "Fresh Adversarial Regression Guard" (adversarial signal detected, 0 violations / 0 warnings, `RQG_EXIT=0`).
- [x] Root cause confirmed and documented (bug.md + design.md).
  - Evidence: report.md → "Root Cause" + design.md → "Root Cause Analysis" + the live redteam observation in bug.md.
- [x] Fix present at HEAD, git-verified across both surfaces.
  - Evidence: report.md → "Code Diff Evidence" (`f26dbdd9`; `checkMLSidecar` decode + `mlHealthBody` in `internal/api/health.go`; `is_model_loaded()` in `ml/app/embedder.py`; `/health` call in `ml/app/main.py`).
- [x] Config integrity + lint clean (`./smackerel.sh check` exit 0, `./smackerel.sh lint` exit 0).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Check / Fresh Lint" (`CHECK_EXIT=0`, `LINT_EXIT=0`, "All checks passed!").
- [x] Change Boundary is respected and zero excluded file families were changed — the source fix is
  verified read-only (NOT re-changed); the certification packet touches ONLY the BUG-050-001 bug
  folder.
  - Evidence: report.md → "Audit Evidence" (`git status --short` clean before/after; packet mutations confined to the bug folder) + "Code Diff Evidence" (source fix in `f26dbdd9`, untouched here).
- [x] Bug marked as Fixed in bug.md.
  - Evidence: bug.md Status line flipped to FIXED & VERIFIED at certification; state.json `status` = `done` and `certification.status` = `done`.
- [x] Live "core `/api/health` reflects the running ML sidecar's real `model_loaded`" outcome certified
  on the source fix + the Go/Python unit regressions; the fresh full-stack re-check (and signed
  redeploy) is owned by `bubbles.devops` as a non-gating operational step (`TP-9`, `redeployRequired: true`).
  - Evidence: bug.md redteam observation (live core claimed `true` while the sidecar reported `false`) + report.md → "Redeploy / Live-Verification Note" (mechanism unit-proven end to end; running-stack re-check routed to bubbles.devops non-gating).
- [x] Build Quality Gate: `check` and `lint` clean of any BUG-050-001 delta; the full eight-phase
  bugfix-fastlane pipeline recorded with fresh evidence.
  - Evidence: report.md → "Current-Session Re-Verification" (`CHECK_EXIT=0`, `LINT_EXIT=0`; `format --check` names ONLY the pre-existing unrelated `internal/config/release_trains_contract_test.go`, outside this boundary) + "Parent-Expanded Specialist Phase Evidence" (implement, test, regression, simplify, stabilize, security, validate, audit).
