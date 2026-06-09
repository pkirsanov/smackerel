# Bug: [BUG-059-001] gkeepapi live-mode dependency missing from ML sidecar build surfaces

## Summary
The `gkeepapi` runtime dependency required by Google Keep live mode is absent from every Python ML sidecar build surface (`ml/requirements.txt`, `ml/pyproject.toml`, `ml/Dockerfile`), so any built `smackerel-ml` image raises `RuntimeError("gkeepapi is not installed")` on the first real live-mode sync. The spec 059 headline LIVE capability is non-deployable as shipped.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

**Severity rationale (fail-SAFE, not urgent):** The shipped SST default is `sync_mode: takeout` (`config/smackerel.yaml:359`), the fixture/replay path — live mode is NOT active by default. Live mode requires THREE explicit opt-ins (`sync_mode ∈ {gkeepapi,hybrid}` + `gkeep_enabled:true` + `warning_acknowledged:true`). The failure is LOUD (`RuntimeError`), not silent: no data loss, no security hole, no credential leak. The workaround is to stay on `takeout` mode until the pin ships.

## Status
- [ ] Reported
- [x] Confirmed (reproduced via diagnostic evidence)
- [ ] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

**Triage state:** CERTIFIED via Path A (DELIVER). The maintainer accepted the reverse-engineered-library supply-chain posture and selected `gkeepapi==0.17.1` (design.md Q1 resolved), clearing the prior `blocked` state. Fix DELIVERED, VERIFIED, and CERTIFIED: pinned on `ml/pyproject.toml` + `ml/requirements.txt`, image rebuilt, in-image `import gkeepapi` exit 0 (v0.17.1), structural guard GREEN-with-pin / RED-without, no unit regression (report.md → Fix Verification Evidence). The bugfix-fastlane specialist-certification chain (regression/simplify/stabilize/security/validate + the prior independent audit) is complete and recorded and all 36 state-transition-guard checks pass (exit 0, transition permitted), so `Verified` is checked. The `Closed` box (terminal `done`) is applied post-commit by the commit owner under the two-phase G088-safe commit ordering — planning truth is committed first, then `certifiedAt` post-dates it; state.json currently records `status: in_progress` with `certifiedAt: null`.

## Reproduction Steps
1. Confirm the dependency is absent from all build surfaces:
   `grep -rn gkeepapi ml/requirements.txt ml/pyproject.toml ml/Dockerfile` → exit 1 (no matches).
2. Confirm the consumer code imports it: `ml/app/keep_bridge.py:72` `import gkeepapi`.
3. Build the sidecar image (`./smackerel.sh build`); `ml/Dockerfile:15` installs only `ml/requirements.txt`.
4. Enable live mode via the three explicit opt-ins and trigger a `keep.sync.request`.
5. Observe `RuntimeError("gkeepapi is not installed. Install with: pip install gkeepapi")` from `ml/app/keep_bridge.py:82`.

## Expected Behavior
A built `smackerel-ml` image contains the `gkeepapi` library so that, once an operator completes the three explicit live-mode opt-ins, `authenticate()` reaches the real Google Keep login path instead of raising `ImportError` → `RuntimeError`.

## Actual Behavior
`gkeepapi` is not installed in any built image. The lazy `import gkeepapi` at `ml/app/keep_bridge.py:72` raises `ImportError`, caught at `:81` and re-raised as `RuntimeError("gkeepapi is not installed...")` at `:82`.

## Environment
- Service: smackerel-ml (Python sidecar)
- Version: HEAD 9638b065
- Build surface: `ml/Dockerfile` (installs `ml/requirements.txt` at line 15)
- Platform: Docker image `smackerel-ml`

## Error Output
```
RuntimeError: gkeepapi is not installed. Install with: pip install gkeepapi
  raised at ml/app/keep_bridge.py:82 (authenticate)
```
(The dependency-absence root cause is proven by the grep evidence in report.md → Diagnostic Evidence. The live `RuntimeError` was NOT reproduced against a running stack here; reproducing it requires the three live-mode opt-ins and is deferred with the fix — see report.md → Fix Verification Evidence.)

## Root Cause
The consumer code (`ml/app/keep_bridge.py`) performs a LAZY `import gkeepapi` inside `authenticate()`, but the dependency was never added to the build manifest. Unit tests pass because `ml/tests/test_keep.py` pre-seeds `bridge._keep_session = MagicMock()` (`:91`) and patches `authenticate` (`:316`), so the lazy import line at `:72` never executes — the suite is structurally blind to the missing dependency. The parent spec's own recovery runbook (`specs/059-google-keep-live-mode/design.md:327`, echoed at `scopes.md:526`) documents "bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild", which presupposes a pin that was never added.

## Related
- Feature: `specs/059-google-keep-live-mode/` (parent — status `done`, untouched by this bug)
- Consumer: `ml/app/keep_bridge.py:72,74,82`
- Build surfaces: `ml/requirements.txt`, `ml/pyproject.toml`, `ml/Dockerfile:14-15`
- Parent design recovery step that assumes the pin: `specs/059-google-keep-live-mode/design.md:327`
- Parent scopes echo: `specs/059-google-keep-live-mode/scopes.md:526`
- Diagnostic origin: DEVOPS-059-A (stochastic-quality-sweep round 17/20)

## Resolution (Path A — DELIVER)
`gkeepapi` is an UNOFFICIAL, REVERSE-ENGINEERED Google Keep client that Google actively breaks. Pinning it as a production dependency was a deliberate supply-chain decision the maintainer consciously owned. The maintainer chose **Path A**: accept the supply-chain risk and pin `gkeepapi==0.17.1` (latest stable; `requires-python >=3.10`, satisfied by the `python:3.12-slim` builder). The exact pin is bumped deliberately (never floated) per the spec 059 drift-circuit-breaker runbook (design.md Q2). The existing consumer call `keep.login(email, password)` (`keep_bridge.py:74`) is API-compatible with 0.17.1 (`Keep.login(self, email, password, state=None, sync=True, device_id=None)`) — ZERO consumer-code change. Delivered surfaces: `ml/pyproject.toml` (`[project.optional-dependencies] runtime`) + `ml/requirements.txt` (the Dockerfile-installed lock) + `ml/tests/test_build_surface_pins.py` (structural guard). Priority was medium; deploy-blocking ONLY for operators running live mode. See report.md → Fix Verification Evidence and scopes.md DoD.
