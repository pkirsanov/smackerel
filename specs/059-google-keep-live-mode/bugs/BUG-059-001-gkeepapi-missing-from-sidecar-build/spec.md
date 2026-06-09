# Spec: [BUG-059-001] gkeepapi pin present on the ML sidecar build surface

## Problem Statement
Google Keep live mode (spec 059) is the headline LIVE capability, but the `gkeepapi` library it imports at runtime is not present in any built `smackerel-ml` image. Operators who complete the three required live-mode opt-ins hit a hard `RuntimeError("gkeepapi is not installed")` instead of a working sync. The capability is non-deployable as shipped.

## Outcome Contract
**Intent:** A built `smackerel-ml` image MUST contain a pinned `gkeepapi` so live mode reaches the real Google Keep authentication path, and the mocked unit suite MUST be able to catch removal of the pin.
**Success Signal:** With the pin in place and live mode enabled (three opt-ins), `authenticate()` in `ml/app/keep_bridge.py` proceeds past `import gkeepapi` (line 72) without `ImportError`; and a structural guard test FAILS if the pin is removed from the build surface.
**Hard Constraints:** SST default stays `sync_mode: takeout`; live mode stays triple-opt-in and fail-loud; no secrets added to any build surface; the pin is an explicit exact `==` version (no floating range for a reverse-engineered library).
**Failure Condition:** Any built image where `python -c "import gkeepapi"` fails, OR a mocked suite that still passes after the pin is deleted.

## Goals
- The `gkeepapi` dependency is declared on the ML build surface with an exact pinned version.
- A structural guard test asserts the pin's presence so the mock-based suite catches regression.
- The live-mode authentication path is exercisable in a real (non-mocked) build.

## Non-Goals
- Changing the default `sync_mode` (stays `takeout`).
- Relaxing the triple-opt-in live-mode gate.
- Implementing the fix in this packet (deferred — supply-chain decision pending).
- Replacing `gkeepapi` with an official Google Keep API (no official public API exists).

## Requirements
- R1: `gkeepapi==<version>` present in `ml/pyproject.toml` dependencies AND `ml/requirements.txt` (the lock `ml/Dockerfile:15` installs).
- R2: A structural guard test asserts the pin exists on the build surface and fails if it is removed.
- R3: A built image satisfies `python -c "import gkeepapi"` exit 0.
- R4: Live-mode smoke evidence documents authentication reaching past the lazy import without the not-installed `RuntimeError`.

## User Scenarios (Gherkin)
```gherkin
Scenario: Built image contains the live-mode dependency
  Given the smackerel-ml image is built from ml/Dockerfile
  When the build surface (ml/requirements.txt) is installed
  Then gkeepapi is importable inside the image (python -c "import gkeepapi" exits 0)

Scenario: Mocked suite catches pin removal
  Given a structural guard test asserts gkeepapi is pinned on the build surface
  When the gkeepapi pin is removed from ml/requirements.txt
  Then the guard test FAILS (red), proving the regression is caught

Scenario: Live-mode sync no longer raises the not-installed error
  Given sync_mode=gkeepapi with gkeep_enabled and warning_acknowledged
  When keep_bridge.authenticate() runs against a built image
  Then the "gkeepapi is not installed" RuntimeError is NOT raised
```

## Acceptance Criteria
- AC-1 (R1): grep of `ml/requirements.txt` and `ml/pyproject.toml` both show an exact `gkeepapi==<version>` pin.
- AC-2 (R3): `python -c "import gkeepapi"` exits 0 inside a freshly built image.
- AC-3 (R2): the structural guard test fails when the pin is removed and passes when present (RED→GREEN).
- AC-4 (R4): live-mode smoke evidence recorded; no `gkeepapi is not installed` `RuntimeError`.
- AC-5 (negative/adversarial): removing the pin causes both the guard test AND an in-image import check to fail.
