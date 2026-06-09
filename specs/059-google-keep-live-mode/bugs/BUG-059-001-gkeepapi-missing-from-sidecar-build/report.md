# Execution Report: [BUG-059-001] gkeepapi missing from ML sidecar build surfaces

## Status: OPEN / TRIAGED / DEFERRED (fix not implemented — supply-chain decision pending)

This report records the DIAGNOSTIC evidence that confirms the bug. It does NOT claim any fix or test execution — the fix is intentionally deferred to a deliberate delivery pass (see bug.md → Deferred Reason, design.md → Open Questions Q1). All DoD items in scopes.md remain unchecked (anti-fabrication, Gate G021).

## Summary
- **Bug:** `gkeepapi` live-mode runtime dependency is absent from all three ML sidecar build surfaces (`ml/requirements.txt`, `ml/pyproject.toml`, `ml/Dockerfile`); `ml/app/keep_bridge.py:72` imports it lazily and `:82` raises `RuntimeError("gkeepapi is not installed")` on the first live-mode sync.
- **Severity:** Medium (fail-safe — default `sync_mode: takeout`, three explicit opt-ins required to reach the failure, loud `RuntimeError`, no data loss / no security exposure).
- **Root cause:** Lazy `import gkeepapi` plus a mock-based unit suite hid the missing build-manifest pin from both the build and the tests.
- **Status:** Documented + triaged + DEFERRED (state.json `status: blocked`). Fix NOT implemented — pending maintainer supply-chain decision (design.md → Open Question Q1).
- **Scenarios validated:** none executed; this packet is diagnostic-evidence-only (fix deferred).

## Diagnostic Evidence (verified at HEAD 9638b065, 2026-06-07)

### Evidence 1 — gkeepapi absent from ALL build surfaces (root cause)
Command:
```
grep -rn gkeepapi ml/requirements.txt ml/pyproject.toml ml/Dockerfile; echo "exit=$?"
```
Output:
```
exit=1
```
No matches in any of the three build surfaces; exit 1 confirms absence.

### Evidence 2 — consumer code DOES import gkeepapi (lazy, inside authenticate())
Command:
```
grep -nE "import gkeepapi|gkeepapi\.Keep\(\)|gkeepapi is not installed" ml/app/keep_bridge.py
```
Output:
```
72:        import gkeepapi  # noqa: F811
74:        keep = gkeepapi.Keep()
82:        raise RuntimeError("gkeepapi is not installed. Install with: pip install gkeepapi")
```

### Evidence 3 — Dockerfile installs only requirements.txt (which lacks the pin)
Command:
```
grep -n "requirements.txt" ml/Dockerfile
```
Output:
```
14:COPY requirements.txt .
15:RUN pip install --no-cache-dir -r requirements.txt
```

### Evidence 4 — unit suite is structurally blind (mocks the session / patches authenticate)
Command:
```
grep -nE "MagicMock|_keep_session|patch.object" ml/tests/test_keep.py | head
```
Output:
```
6:from unittest.mock import MagicMock, patch
91:        bridge._keep_session = MagicMock()
308:        bridge._keep_session = mock_keep
316:            with patch.object(bridge, "authenticate", return_value=mock_keep):
```
The pre-seeded `_keep_session` / patched `authenticate` mean the lazy `import gkeepapi` at `keep_bridge.py:72` never executes under test — the suite cannot catch the missing dependency.

### Evidence 5 — SST default is takeout (fail-safe; live mode not active by default)
Command:
```
sed -n '357,361p' config/smackerel.yaml
```
Output:
```
  google-keep:
    enabled: false
    sync_mode: takeout # takeout, gkeepapi, or hybrid
    import_dir: "" # path to Google Takeout Keep export directory
    include_archived: false
```

### Evidence 6 — parent runbook assumes the pin already ships
Command:
```
sed -n '325,330p' specs/059-google-keep-live-mode/design.md
```
Output:
```
1. `./smackerel.sh logs` to inspect the `keep_protocol_drift_detected` event(s) and identify what changed (e.g., `gkeepapi` version, response shape).
2. If a library upgrade is needed, bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild.
3. Edit `config/smackerel.yaml` and change `drift_ack_token` to ANY new value.
```

## Consequence
In any built `smackerel-ml` image, the first real live-mode sync (`sync_mode ∈ {gkeepapi,hybrid}` + `gkeep_enabled:true` + `warning_acknowledged:true`) raises `RuntimeError("gkeepapi is not installed")` at `keep_bridge.py:82`. The spec 059 headline LIVE capability is non-deployable as shipped. The failure is LOUD and fail-safe (no data loss / no security exposure); default `takeout` users are unaffected.

## Test Evidence
NONE executed — fix deferred. No regression or verification test was run because the fix is intentionally deferred (see Completion Statement below). The delivery pass will populate this section with: the pre-fix RED structural guard test (`ml/tests/test_build_surface_pins.py`), the post-fix GREEN run, an in-image `python -c "import gkeepapi"` exit 0, and a live-mode authentication smoke. Recording any test result here now would be fabrication (Gate G021). The verified diagnostic evidence that confirms the bug is captured above under "## Diagnostic Evidence".

## Parent-Spec Non-Interference Evidence
Parent spec `059-google-keep-live-mode` status remains `done`; no parent artifact (spec.md / design.md / scopes.md / state.json / report.md / uservalidation.md / scenario-manifest.json) was modified. Parent artifact-lint count immediately before this bug folder was created: 5 (accepted known drift). After bug-folder creation the parent artifact-lint count is still 5 (delta = 0) — single-file layout does not recurse into `bugs/`.

## Completion Statement
This packet is TRACKED-WORK CREATION ONLY and is intentionally INCOMPLETE. The defect (DEVOPS-059-A) is documented with verified diagnostic evidence; the fix is DEFERRED to a deliberate delivery pass because pinning a reverse-engineered Google library as a production dependency is a maintainer supply-chain decision (design.md → Open Question Q1). state.json `status: blocked`, `certification.status: blocked`. All scopes.md DoD items are unchecked. No fix code was written, no tests were run, and the parent spec 059 stays `done` with its protected artifacts unchanged. This is explicitly NOT a completion/"done" claim.
