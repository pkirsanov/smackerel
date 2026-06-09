# Bug Fix Design: [BUG-059-001] gkeepapi missing from ML sidecar build surfaces

## Root Cause Analysis

### Investigation Summary
DEVOPS-059-A probed the spec 059 live-mode runtime against its build surfaces. Verified evidence (captured in report.md → Diagnostic Evidence):
- `grep -rn gkeepapi ml/requirements.txt ml/pyproject.toml ml/Dockerfile` → exit 1 (absent from all three).
- Consumer uses it: `ml/app/keep_bridge.py:72` `import gkeepapi` (lazy, inside `authenticate()`), `:74` `keep = gkeepapi.Keep()`, `:82` `raise RuntimeError("gkeepapi is not installed...")`.
- `ml/Dockerfile:14-15` installs only `ml/requirements.txt`.
- Unit suite is blind: `ml/tests/test_keep.py:91` `bridge._keep_session = MagicMock()` and `:316` `patch.object(bridge, "authenticate", ...)` pre-empt the lazy import, so `:72` never runs under test.

### Root Cause
The live-mode consumer was implemented with a lazy `import gkeepapi`, but the dependency was never added to the build manifest (`ml/pyproject.toml` source or `ml/requirements.txt` lock). Because the import is lazy and the unit suite mocks the session, the gap is invisible to both the build and the test suite. The parent spec's own recovery runbook (`design.md:327`, `scopes.md:526`) assumes a pin already ships ("bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild"), confirming the pin was always intended but never landed.

### Impact Analysis
- Affected components: smackerel-ml sidecar live-mode path (`keep_bridge.authenticate`).
- Affected data: none (fail-loud before any sync; no partial writes).
- Affected users: only operators who enable live mode (triple opt-in). Default `takeout` users are unaffected.
- Blast radius: a single dependency line on one build surface.

## Fix Design

### Solution Approach
1. Add an exact pin `gkeepapi==<version-TBD-by-maintainer>` to `ml/pyproject.toml` `dependencies` (the SST source) AND to `ml/requirements.txt` (the generated lock that `ml/Dockerfile:15` installs). No `ml/Dockerfile` change is required because it already installs the lock; the gap is purely the missing line.
2. Add a STRUCTURAL guard test (e.g. `ml/tests/test_build_surface_pins.py`) that reads `ml/requirements.txt` (and/or `ml/pyproject.toml`) and asserts an exact `gkeepapi==` pin is present. This makes the mock-based suite able to catch pin removal — the test does NOT import `gkeepapi`; it asserts on the build-manifest text, so it runs without the library and fails RED if the pin is absent.
3. Rebuild the image and verify `python -c "import gkeepapi"` exits 0 inside it.
4. Document a live-mode smoke (authentication reaches past the lazy import) as evidence.

### Why a structural guard test (not an import test)
An `import gkeepapi` test would itself require the library to be installed in the test environment, coupling test execution to the very dependency under question and re-introducing the same blind spot in reverse. A text-level assertion on the build manifest is environment-independent, deterministic, and fails precisely when the pin is removed — the adversarial regression case required by the bug contract.

### Alternative Approaches Considered
1. Add `gkeepapi` only to `ml/Dockerfile` via an inline `pip install` — REJECTED: bypasses the SST build manifest (`pyproject.toml`/`requirements.txt`) and violates the single-source-of-truth dependency contract.
2. Make the import non-lazy at module load — REJECTED: would crash the whole sidecar at import time for default `takeout` users who never use live mode; the lazy import is correct, the missing pin is the bug.
3. Replace `gkeepapi` with an official Google Keep API — REJECTED: no official public Google Keep API exists; this is precisely why the reverse-engineered client is used.

## Open Questions (MAINTAINER DECISION REQUIRED BEFORE DELIVERY)
- **Q1 — supply-chain ownership (BLOCKING):** Which `gkeepapi` version to pin, and acceptance of the reverse-engineered-library supply-chain risk, is a maintainer decision required before delivery. `gkeepapi` is an UNOFFICIAL, REVERSE-ENGINEERED Google Keep library that Google actively breaks; pinning it as a production dependency and shipping it in the image is a deliberate supply-chain/security posture choice, not a sweep-round drive-by. The fix is intentionally DEFERRED until this decision is made and recorded.
- **Q2 — pin freshness policy:** Because Google periodically breaks `gkeepapi`, the maintainer should decide a pin-review cadence (e.g. tie it to the spec 059 drift-circuit-breaker recovery runbook) so the pin is bumped deliberately rather than floated.

## Testing Strategy
- unit (structural guard): assert an exact `gkeepapi==` pin on the build surface; RED when removed, GREEN when present.
- integration / e2e (deferred to delivery pass): built-image `python -c "import gkeepapi"` exit 0; live-mode authentication smoke reaching past the lazy import without `RuntimeError`.
- regression: the structural guard IS the adversarial regression — removing the pin (the exact reintroduction of this bug) makes it fail.
