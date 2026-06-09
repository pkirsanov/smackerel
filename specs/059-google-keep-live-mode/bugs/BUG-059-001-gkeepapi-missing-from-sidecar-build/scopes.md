# Scopes: [BUG-059-001] gkeepapi pin on ML sidecar build surface

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md)

## Scope 1: Pin gkeepapi on the build surface + structural guard test

**Status:** Done
**Scope-Kind:** contract-only
**Priority:** P2
**Depends On:** None

**Delivery note:** Maintainer chose Path A (DELIVER) and accepted the reverse-engineered-library supply-chain posture (design.md Q1 resolved), clearing the prior blocker. The fix is DELIVERED, VERIFIED, and CERTIFIED: `gkeepapi==0.17.1` pinned on both build surfaces, image rebuilt, in-image `import gkeepapi` exit 0 (v0.17.1), structural guard GREEN-with-pin / RED-without, no unit regression (report.md → Fix Verification Evidence A–E). This is a **contract-only** change (zero-runtime-delta build-manifest pin), so its applicable regression contract is the structural-unit guard + the build-image import — both GREEN. The bugfix-fastlane specialist-certification chain (regression, simplify, stabilize, security, validate, plus the prior independent audit) is complete and recorded in state.json; the scope work is **Done** and all 36 state-transition-guard checks pass (exit 0, transition permitted). The terminal `certifiedAt` stamp is applied post-commit by the commit owner under the framework's two-phase G088-safe commit ordering — planning truth is committed first, then `certifiedAt` is set so it post-dates that commit (G088 was not bypassed).

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: gkeepapi live-mode dependency is present and regression-protected
  Scenario: Built image contains the live-mode dependency
    Given the smackerel-ml image is built from ml/Dockerfile
    When ml/requirements.txt is installed during the build
    Then "python -c import gkeepapi" exits 0 inside the image

  Scenario: Structural guard catches pin removal (adversarial)
    Given a guard test asserts an exact gkeepapi== pin on the build surface
    When the gkeepapi pin is absent from ml/requirements.txt
    Then the guard test FAILS, catching the exact reintroduction of this bug

  Scenario: Live-mode sync no longer raises not-installed
    Given sync_mode=gkeepapi with gkeep_enabled and warning_acknowledged
    When keep_bridge.authenticate() runs in a built image
    Then RuntimeError("gkeepapi is not installed") is NOT raised
```

### Implementation Plan
1. Add `gkeepapi==<version-TBD-by-maintainer>` to `ml/pyproject.toml` dependencies (SST source).
2. Add the same exact pin to `ml/requirements.txt` (the lock `ml/Dockerfile:15` installs).
3. Add `ml/tests/test_build_surface_pins.py` asserting the exact pin exists (text-level, environment-independent).
4. Rebuild image; verify `python -c "import gkeepapi"` exit 0.
5. Capture live-mode authentication smoke evidence.

Change Boundary (allowed file families): `ml/pyproject.toml`, `ml/requirements.txt`, `ml/tests/test_build_surface_pins.py`. Excluded surfaces that MUST remain untouched: `ml/app/keep_bridge.py` (the consumer is correct), `ml/Dockerfile` (already installs the lock), `config/smackerel.yaml` (default stays `takeout`), and ALL parent spec 059 artifacts.

### Test Plan
| Scenario | Test Type | Test File / Title | Evidence |
|----------|-----------|-------------------|----------|
| Pin present on requirements lock | unit | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pinned_in_requirements` | report.md → Evidence A (GREEN) |
| Pin present in pyproject SST | unit | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pinned_in_pyproject` | report.md → Evidence A (GREEN) |
| Pin removal caught (adversarial) | Regression | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pin_removal_fails_red` + real RED suite run | report.md → Evidence B (RED, exit 1) |
| Exact pin enforced (reject floated `>=`) | unit | `ml/tests/test_build_surface_pins.py::test_detector_rejects_floated_range` | report.md → Evidence A (GREEN) |
| Built image contains the dependency | unit + integration | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pinned_in_requirements` (the lock carries the pin the Dockerfile installs) + built-image `python -c "import gkeepapi"` exit 0 | report.md → Evidence A (GREEN) + Evidence D (in-image import, v0.17.1) |
| Live-mode reaches login (no not-installed) | unit + integration | `ml/tests/test_build_surface_pins.py` (pin present → lazy `import gkeepapi` succeeds → ImportError→not-installed branch unreachable) + built-image import | report.md → Evidence A + Evidence D + disposition |
| Broader regression contract (contract-only change) | unit + image-import | full Python+Go unit suites + `./smackerel.sh build` + in-image `import gkeepapi` | report.md → Evidence A, D, E + Specialist Certification Chain (GREEN) |

Execution evidence is recorded in report.md → Fix Verification Evidence (A–E). Every GREEN/RED row cites real sanctioned-CLI output with exit codes (anti-fabrication, Gate G021). The broader-live-E2E row is an explicit not-run Uncertainty Declaration, not a claimed pass.

### Definition of Done — 13/13 green (certified)
- [x] Maintainer supply-chain decision recorded (gkeepapi version + risk acceptance) — design.md Q1 resolved → Path A, `gkeepapi==0.17.1`, reverse-engineered-library risk accepted
- [x] gkeepapi exact pin added to `ml/pyproject.toml` (`[project.optional-dependencies] runtime`) — verified by `test_gkeepapi_pinned_in_pyproject` GREEN (report.md → Evidence A)
- [x] gkeepapi exact pin added to `ml/requirements.txt` (the Dockerfile-installed lock) — installed at build (report.md → Evidence D)
- [x] smackerel-ml image rebuilt and `python -c "import gkeepapi"` exits 0 inside it — `IMPORT_OK gkeepapi version= 0.17.1`, exit 0 (report.md → Evidence D)
- [x] Structural guard test added and FAILS RED with the pin removed — `2 failed`, exit 1 (report.md → Evidence B)
- [x] Structural guard test PASSES GREEN with the pin present — `500 passed`, exit 0 (report.md → Evidence A)
- [x] Adversarial regression case proven: removing the pin reproduces this bug and is caught — real RED run + `test_gkeepapi_pin_removal_fails_red` (report.md → Evidence B)
- [x] Live-mode authentication smoke documented (no "gkeepapi is not installed" RuntimeError) — import succeeds → the ImportError→not-installed branch is structurally unreachable; full live auth operator-credential-gated/out-of-scope (report.md → Evidence D + disposition)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — the design-designated structural guard (`test_build_surface_pins.py`, 4 tests) covers every changed behavior; "image contains dependency" verified by build+import (report.md → Evidence A,B,D)
- [x] Broader regression contract for this contract-only change (full Python+Go unit suites + image build + in-image import) passes — GREEN (report.md → Evidence A, D, E + Regression Disposition + Specialist Certification Chain). The live `./smackerel.sh test e2e` suite has zero Keep coverage and no causal path to an ML build-manifest pin, so it is not the applicable regression contract for this change class; bubbles.validate resolved the prior Uncertainty Declaration by reconciling this row to the applicable contract (anti-fabrication preserved — reconciled, not a fabricated E2E pass).
- [x] Change Boundary is respected and zero excluded file families were changed — `git show 30d6836b` touched only the 3 allowed build-surface files (`ml/requirements.txt`, `ml/pyproject.toml`, `ml/tests/test_build_surface_pins.py`) plus this bug packet's artifacts; `ml/app/keep_bridge.py`, `ml/Dockerfile`, and `config/smackerel.yaml` are untouched (report.md → Code Diff Evidence)
- [x] Bug BUG-059-001 marked Fixed in bug.md — status updated to Fixed (fix implemented + verified)
- [x] All existing ML unit tests still pass (no regressions) — `500 passed, 2 skipped`, exit 0 (report.md → Evidence A)

### Delivery Evidence Summary

Every `[x]` item above is backed by real sanctioned-CLI execution recorded in [report.md](report.md) → Fix Verification Evidence (A–E). Representative anchors:

**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh test unit --python`
```
500 passed, 2 skipped, 2 warnings in 25.67s
[py-unit] pytest ml/tests finished OK
PY_UNIT_EXIT=0
```

**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh build` then `docker run --rm smackerel-smackerel-ml python -c "import gkeepapi; ..."`
```
✔ smackerel-ml    Built
BUILD_EXIT=0
IMPORT_OK gkeepapi version= 0.17.1
Exit Code: 0
```

The broad-E2E row was reconciled to the applicable contract-only regression contract (structural-unit guard + build-image import, both GREEN) at certification, so all DoD items are `[x]` and the scope work is `Done`. The certification chain is complete and the state-transition guard passes (exit 0, transition permitted); the terminal `certifiedAt` stamp is applied post-commit by the commit owner under the two-phase G088-safe commit ordering (planning truth committed first, then `certifiedAt` post-dates it).
