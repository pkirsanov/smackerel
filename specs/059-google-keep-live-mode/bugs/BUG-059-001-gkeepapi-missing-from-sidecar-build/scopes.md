# Scopes: [BUG-059-001] gkeepapi pin on ML sidecar build surface

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md)

## Scope 1: Pin gkeepapi on the build surface + structural guard test

**Status:** Blocked
**Priority:** P2
**Depends On:** None

**Blocked reason:** Maintainer supply-chain decision required before any fix code lands — which `gkeepapi` version to pin and acceptance of the reverse-engineered-library risk (see design.md → Open Questions Q1). This packet is tracked-work CREATION only; implementation is deferred to a deliberate delivery pass.

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
| Pin present on build surface | unit | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pinned` | report.md (after fix) |
| Pin removal caught (adversarial) | Regression E2E | `ml/tests/test_build_surface_pins.py::test_gkeepapi_pin_removal_fails_red` | report.md (after fix) |
| Image import works | integration | built-image `python -c "import gkeepapi"` exit 0 | report.md (after fix) |
| Live-mode auth smoke | e2e-api | `keep.sync.request` reaches auth without not-installed RuntimeError | report.md (after fix) |

All rows are PLANNED — the fix is deferred. No execution evidence is claimed in this packet (anti-fabrication, Gate G021).

### Definition of Done — NOT yet done (fix deferred pending maintainer decision)
- [ ] Maintainer supply-chain decision recorded (gkeepapi version + risk acceptance) — design.md Q1 resolved
- [ ] gkeepapi exact pin added to `ml/pyproject.toml` dependencies
- [ ] gkeepapi exact pin added to `ml/requirements.txt` (the Dockerfile-installed lock)
- [ ] smackerel-ml image rebuilt and `python -c "import gkeepapi"` exits 0 inside it
- [ ] Structural guard test added and FAILS RED with the pin removed
- [ ] Structural guard test PASSES GREEN with the pin present
- [ ] Adversarial regression case proven: removing the pin reproduces this bug and is caught
- [ ] Live-mode authentication smoke documented (no "gkeepapi is not installed" RuntimeError)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Bug BUG-059-001 marked Fixed in bug.md
- [ ] All existing ML unit tests still pass (no regressions)
