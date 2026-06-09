# Scopes: [BUG-059-001] gkeepapi pin on ML sidecar build surface

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md)

## Scope 1: Pin gkeepapi on the build surface + structural guard test

**Status:** In Progress
**Priority:** P2
**Depends On:** None

**Delivery note:** Maintainer chose Path A (DELIVER) and accepted the reverse-engineered-library supply-chain risk (design.md Q1 resolved), clearing the prior blocker. The fix is DELIVERED + VERIFIED: `gkeepapi==0.17.1` pinned on both build surfaces, image rebuilt, in-image `import gkeepapi` exit 0 (v0.17.1), structural guard GREEN-with-pin / RED-without, no unit regression (report.md → Fix Verification Evidence A–E). The scope stays **In Progress** (not Done) because ONE DoD row — the broader LIVE E2E suite — is an explicit not-run Uncertainty Declaration (deliberate: zero-runtime-delta build-manifest change; design designated structural-unit regression), and the bugfix-fastlane specialist-certification chain (regression/simplify/stabilize/security/audit) is owned downstream by separation of duties (anti-fabrication, Gate G021).

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
| Broader live E2E suite | e2e | `./smackerel.sh test e2e` | NOT RUN — explicit UD (report.md → Regression Disposition) |

Execution evidence is recorded in report.md → Fix Verification Evidence (A–E). Every GREEN/RED row cites real sanctioned-CLI output with exit codes (anti-fabrication, Gate G021). The broader-live-E2E row is an explicit not-run Uncertainty Declaration, not a claimed pass.

### Definition of Done — DELIVERED + VERIFIED (11/12 green; 1 explicit not-run UD)
- [x] Maintainer supply-chain decision recorded (gkeepapi version + risk acceptance) — design.md Q1 resolved → Path A, `gkeepapi==0.17.1`, reverse-engineered-library risk accepted
- [x] gkeepapi exact pin added to `ml/pyproject.toml` (`[project.optional-dependencies] runtime`) — verified by `test_gkeepapi_pinned_in_pyproject` GREEN (report.md → Evidence A)
- [x] gkeepapi exact pin added to `ml/requirements.txt` (the Dockerfile-installed lock) — installed at build (report.md → Evidence D)
- [x] smackerel-ml image rebuilt and `python -c "import gkeepapi"` exits 0 inside it — `IMPORT_OK gkeepapi version= 0.17.1`, exit 0 (report.md → Evidence D)
- [x] Structural guard test added and FAILS RED with the pin removed — `2 failed`, exit 1 (report.md → Evidence B)
- [x] Structural guard test PASSES GREEN with the pin present — `500 passed`, exit 0 (report.md → Evidence A)
- [x] Adversarial regression case proven: removing the pin reproduces this bug and is caught — real RED run + `test_gkeepapi_pin_removal_fails_red` (report.md → Evidence B)
- [x] Live-mode authentication smoke documented (no "gkeepapi is not installed" RuntimeError) — import succeeds → the ImportError→not-installed branch is structurally unreachable; full live auth operator-credential-gated/out-of-scope (report.md → Evidence D + disposition)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — the design-designated structural guard (`test_build_surface_pins.py`, 4 tests) covers every changed behavior; "image contains dependency" verified by build+import (report.md → Evidence A,B,D)
- [ ] Broader E2E regression suite passes — **NOT RUN (explicit Uncertainty Declaration, Gate G021):** zero-runtime-delta build-manifest change; design designated structural-unit + build-image-import as this bug's regression contract; proportionate broader regression (full Go+Python unit suites + build+import) is GREEN (report.md → Evidence A,D,E + Regression Disposition). Drives the packet to `in_progress`, not a forced `done`. **Audit-reviewed (bubbles.audit 2026-06-09):** independently confirmed `tests/e2e/` has ZERO Keep-live coverage (grep `keep_bridge|gkeepapi|keep.sync|sync_mode` → no matches) and the suite is a Go-core/live-stack runner with no causal path to an ML build-manifest pin; the `scenario-manifest.json` behavior contract has 3 scenarios, none e2e, all verified. This row is therefore a **non-applicable broad-regression gate for this change class** (distinct from BUG-056-002's genuinely-applicable migration row) — an audit-reviewed UD that **bubbles.validate MAY resolve at final certification**. It is NOT the binding blocker; the binding blockers are the incomplete specialist chain (G022) + validate-owned certification. The row stays `[ ]` because audit will not fabricate a green `[x]` for a suite that genuinely did not run. See report.md → Audit Findings A3.
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

The single `[ ]` item (broader live E2E suite) is an explicit not-run Uncertainty Declaration (report.md → Regression Disposition), which is why the packet is `in_progress`, not a forced `done`.
