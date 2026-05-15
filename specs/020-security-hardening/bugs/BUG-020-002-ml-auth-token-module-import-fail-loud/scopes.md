# Scopes: BUG-020-002 — `ml/app/auth.py` reads `SMACKEREL_AUTH_TOKEN` at module import using the silent-default `os.environ.get(KEY, "")` form, violating Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Scope 1: Convert the module-import-time read of `SMACKEREL_AUTH_TOKEN` in `ml/app/auth.py` to the canonical Python fail-loud pattern, pre-seed the env via `ml/tests/conftest.py` so pytest collection still works, and add a persistent in-tree adversarial test that mechanically locks the contract against future drift

**Status:** Done

**Files:**
- [ml/app/auth.py](../../../../ml/app/auth.py) (line 11 silent-default read converted to `os.environ[KEY]` wrapped in `try / except KeyError → raise RuntimeError(...) from exc`; +21 / -1 lines including the leading 12-line context comment block)
- [ml/tests/conftest.py](../../../../ml/tests/conftest.py) (NEW, 28 lines; `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup)
- [ml/tests/test_auth_module_import_fail_loud.py](../../../../ml/tests/test_auth_module_import_fail_loud.py) (NEW, 87 lines; 1 adversarial test + 2 positive tests + 1 reload helper)

### Use Cases

```gherkin
Feature: ml/app/auth.py reads SMACKEREL_AUTH_TOKEN at module import using the canonical Python fail-loud pattern (Gate G028)
  Scenario: SCN-020-002-A — Module import raises RuntimeError when env var is unset
    Given the SMACKEREL_AUTH_TOKEN env var is not present in os.environ
    When a fresh `import app.auth` is forced (via `del sys.modules["app.auth"]` then `importlib.import_module("app.auth")`)
    Then the import raises a RuntimeError whose message names "SMACKEREL_AUTH_TOKEN"
    And the message names the fix path "run ./smackerel.sh config generate"
    And the message includes the breadcrumb tokens "Gate G028" and "HL-RESCAN-013"

  Scenario: SCN-020-002-B — Module import succeeds when env var is empty
    Given SMACKEREL_AUTH_TOKEN is set to an empty string in os.environ
    When a fresh `import app.auth` is forced
    Then the import succeeds and the module-level _AUTH_TOKEN equals ""
    And the dev-mode auth-bypass behavior of verify_auth() is preserved

  Scenario: SCN-020-002-C — Module import succeeds when env var holds a real token value
    Given SMACKEREL_AUTH_TOKEN is set to "test-secret-real-value" in os.environ
    When a fresh `import app.auth` is forced
    Then the import succeeds and the module-level _AUTH_TOKEN equals "test-secret-real-value"

  Scenario: SCN-020-002-D — pytest conftest pre-seeds env var before collection
    Given a developer runs `pytest ml/tests/test_main.py` from a clean shell with no env file loaded
    When pytest starts and loads ml/tests/conftest.py before collecting any test module
    Then conftest calls os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")
    And the subsequent `from app.main import _check_required_config, health` import in test_main.py:19 succeeds (the transitive `from .auth import verify_auth` finds SMACKEREL_AUTH_TOKEN in os.environ)

  Scenario: SCN-020-002-E — Existing test suites pass unchanged after the fix
    Given the post-fix ml/app/auth.py + ml/tests/conftest.py + ml/tests/test_auth_module_import_fail_loud.py
    When `pytest ml/tests/test_auth.py ml/tests/test_main.py ml/tests/test_startup_warning.py -v` runs
    Then test_auth.py reports 10 passed
    And test_main.py reports 26 passed
    And test_startup_warning.py reports 3 passed
```

### Implementation Plan

1. **`ml/app/auth.py` line 11 — convert silent-default read to canonical Python fail-loud pattern:** replace `_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` with `try: _AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` / `except KeyError as exc: raise RuntimeError(...) from exc`. Lead with a 12-line context comment block explaining (a) the HL-RESCAN-013 / Gate G028 chain, (b) the canonical Python fail-loud pattern citation from `.github/copilot-instructions.md` Secrets Management table, (c) why an empty string value is still allowed (dev-mode auth-bypass signal), (d) where the production-vs-dev branching lives (`ml/app/main.py:_check_required_config` lifespan startup check). The RuntimeError message names the file path, the variable, the fix path (`run ./smackerel.sh config generate`), the dev-mode disclaimer, and the breadcrumb tokens (`Gate G028` and `HL-RESCAN-013`).
2. **`ml/tests/conftest.py` (NEW FILE) — pre-seed SMACKEREL_AUTH_TOKEN for pytest collection:** declare a 22-line module docstring documenting the HL-RESCAN-013 / Gate G028 rationale + the import chain that would otherwise break (test_main.py:19 → app.main:12 → app.auth) + the `setdefault` rationale (preserves externally-provided values, never silently overrides). Body is a single `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` call.
3. **`ml/tests/test_auth_module_import_fail_loud.py` (NEW FILE) — adversarial regression test:** declare a module docstring naming the regression target. Define a `_reload_auth_module()` helper that does `del sys.modules["app.auth"]` then `importlib.import_module("app.auth")`. Define 3 test functions: (a) `test_module_import_raises_when_env_var_unset` uses `monkeypatch.delenv("SMACKEREL_AUTH_TOKEN", raising=False)` then `pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN")` on the reload; (b) `test_module_import_succeeds_with_empty_value` uses `monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")` then asserts `auth_mod._AUTH_TOKEN == ""`; (c) `test_module_import_succeeds_with_real_value` uses `monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "test-secret-real-value")` then asserts `auth_mod._AUTH_TOKEN == "test-secret-real-value"`. Each test docstring documents the exact contract assertion and the regression-target line in `ml/app/auth.py`.
4. **RED→GREEN proof (scenario-first TDD):** before committing, capture (a) the new adversarial test PASSes against the post-fix `ml/app/auth.py`, (b) the existing `ml/tests/test_auth.py` (10 tests), `ml/tests/test_main.py` (26 tests), `ml/tests/test_startup_warning.py` (3 tests) suites PASS unchanged (canary), and (c) module-import-time RED proof via `python -c "import os; os.environ.pop('SMACKEREL_AUTH_TOKEN', None); from app import auth"` against the post-fix `auth.py` — exits non-zero with the named-var traceback. Reverting line 22 of `ml/app/auth.py` from `os.environ["SMACKEREL_AUTH_TOKEN"]` back to `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` would cause `test_module_import_raises_when_env_var_unset` to FAIL with `Failed: DID NOT RAISE <class 'RuntimeError'>`.
5. **Confine the change boundary:** the only files modified are `ml/app/auth.py`, the new `ml/tests/conftest.py`, the new `ml/tests/test_auth_module_import_fail_loud.py`, and the seven BUG-020-002 packet artifacts under `specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/`. No `ml/app/main.py`, no `ml/app/nats_client.py`, no `scripts/commands/config.sh`, no `cmd/core/...`, no `internal/auth/...`, no `Dockerfile`, no `ml/Dockerfile`, no `docker-compose.yml`, no `deploy/compose.deploy.yml`, no `.github/workflows/*`, no foreign-owned `specs/**` content.

### Test Plan

- **Targeted adversarial suite:** `pytest ml/tests/test_auth_module_import_fail_loud.py -v` runs all 3 new tests — every one PASS in <1s wall-clock. Captures the new contract.
- **Targeted canary suites:** `pytest ml/tests/test_auth.py -v` (10 pre-existing tests) PASS unchanged. `pytest ml/tests/test_main.py -v` (26 pre-existing tests) PASS unchanged. `pytest ml/tests/test_startup_warning.py -v` (3 pre-existing tests) PASS unchanged.
- **RED proof (module-import-time fail-loud):** `python -c "import os; os.environ.pop('SMACKEREL_AUTH_TOKEN', None); from app import auth"` against the post-fix `auth.py` exits non-zero with `RuntimeError: ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set...` traceback that names the variable + the fix path (`run ./smackerel.sh config generate`) + the breadcrumbs (`Gate G028`, `HL-RESCAN-013`).
- **GREEN proof (sanctioned-workflow ergonomics):** `pytest ml/tests/` from a clean shell after the conftest pre-seed is in place — pytest collects all test modules without crashing, even though the developer did not explicitly source the env file.
- **RED→GREEN proof of the adversarial test (scenario-first TDD):** temporarily revert line 22 of `ml/app/auth.py` from `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` to `_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")`. Re-run `pytest ml/tests/test_auth_module_import_fail_loud.py::test_module_import_raises_when_env_var_unset -v`. Observe FAIL with `Failed: DID NOT RAISE <class 'RuntimeError'>`. Restore via `replace_string_in_file`. Re-run. Observe PASS.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-020-002-A: Module import raises RuntimeError when env var unset | unit (Python module-import contract) | ml/tests/test_auth_module_import_fail_loud.py | test_module_import_raises_when_env_var_unset | YES — fails RED if line 22 is reverted to `os.environ.get(KEY, "")` form | Persistent in-tree adversarial test that runs on every `./smackerel.sh test python_unit` invocation. |
| SCN-020-002-B: Module import succeeds when env var is empty | unit (Python module-import contract) | ml/tests/test_auth_module_import_fail_loud.py | test_module_import_succeeds_with_empty_value | NO (positive contract) | Same as above. |
| SCN-020-002-C: Module import succeeds when env var holds a real token value | unit (Python module-import contract) | ml/tests/test_auth_module_import_fail_loud.py | test_module_import_succeeds_with_real_value | NO (positive contract) | Same as above. |
| SCN-020-002-D: pytest conftest pre-seeds env var before collection | unit (Python module-import contract) | ml/tests/conftest.py + every other test module | (proven by every other test module continuing to import cleanly) | NO (positive infrastructure contract) | Same as above. |
| SCN-020-002-E: Existing test suites pass unchanged after the fix | unit (Python regression canary) | ml/tests/test_auth.py + ml/tests/test_main.py + ml/tests/test_startup_warning.py | (entire pre-existing suites) | NO (positive canary) | Pre-existing tests; preserved unchanged. |

### Definition of Done

- [x] `ml/app/auth.py` line 22 reads `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in `try / except KeyError as exc → raise RuntimeError(...) from exc`. The canonical Python fail-loud pattern matches `.github/copilot-instructions.md` Secrets Management table requirements. [SCN-020-002-A]
   → Evidence: `grep -nE 'os\.environ\["SMACKEREL_AUTH_TOKEN"\]' ml/app/auth.py` returns one hit at the line, and `grep -nE 'except KeyError as exc' ml/app/auth.py` returns one hit. See report.md > Code Diff Evidence.
- [x] The RuntimeError message in `ml/app/auth.py` names the variable `SMACKEREL_AUTH_TOKEN`, names the fix path `run ./smackerel.sh config generate`, and includes the breadcrumb tokens `Gate G028` and `HL-RESCAN-013`. [SCN-020-002-A]
   → Evidence: `grep -n 'SMACKEREL_AUTH_TOKEN must be set\|HL-RESCAN-013\|Gate G028' ml/app/auth.py` returns the message lines. See report.md > Code Diff Evidence.
- [x] `ml/app/auth.py` retains the dev-mode auth-bypass behavior: when SMACKEREL_AUTH_TOKEN is set to an empty string in os.environ, the module-import succeeds and `_AUTH_TOKEN == ""`. The `verify_auth()` function returns immediately without checking headers when `_AUTH_TOKEN` is empty (preserves the contract documented at the function's docstring). [SCN-020-002-B]
   → Evidence: see SCN-020-002-B test PASS in report.md > Test Evidence > Targeted adversarial suite.
- [x] `ml/tests/conftest.py` exists at the canonical pytest discovery path and calls `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup, BEFORE any test module is collected. The `setdefault` form preserves any externally-provided value, so the conftest is a no-op when the developer invokes pytest via `./smackerel.sh test python_unit` (which loads the env file). [SCN-020-002-D]
   → Evidence: `grep -n 'setdefault' ml/tests/conftest.py` returns one hit. The other test files import-cleanly under pytest collection. See report.md > Code Diff Evidence + > Test Evidence.
- [x] `ml/tests/test_auth_module_import_fail_loud.py` exists and declares 3 test functions: `test_module_import_raises_when_env_var_unset` (adversarial), `test_module_import_succeeds_with_empty_value`, `test_module_import_succeeds_with_real_value`. [SCN-020-002-A, B, C]
   → Evidence: `grep -nE '^def test_' ml/tests/test_auth_module_import_fail_loud.py` returns 3 hits. See report.md > Code Diff Evidence.
- [x] The adversarial test `test_module_import_raises_when_env_var_unset` uses `monkeypatch.delenv("SMACKEREL_AUTH_TOKEN", raising=False)` to clear the env var, then calls a `_reload_auth_module()` helper that does `del sys.modules["app.auth"]` followed by `importlib.import_module("app.auth")`, then asserts via `pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN")`. The test docstring explicitly documents the regression target — reverting `ml/app/auth.py` line 22 to `os.environ.get(KEY, "")` would cause this test to FAIL because the import would silently succeed and `pytest.raises` would not trigger. [SCN-020-002-A]
   → Evidence: `grep -n 'monkeypatch.delenv\|pytest.raises\|_reload_auth_module' ml/tests/test_auth_module_import_fail_loud.py` returns the expected hits. See report.md > Code Diff Evidence.
- [x] `pytest ml/tests/test_auth_module_import_fail_loud.py -v` PASSes — all 3 new tests PASS in <1s wall-clock. [SCN-020-002-A, B, C]
   → Evidence: see report.md > Test Evidence > Targeted adversarial suite. Captures the test runner output with `3 passed`.
- [x] HL-RESCAN-013 attribution is present in either the `ml/app/auth.py` source comment block or the test file docstring (and in fact both). [SCN-020-002-A]
   → Evidence: `grep -n 'HL-RESCAN-013' ml/app/auth.py ml/tests/test_auth_module_import_fail_loud.py` returns multiple hits. See report.md > Code Diff Evidence.
- [x] RED proof captured (scenario-first TDD): temporarily reverting line 22 of `ml/app/auth.py` to `os.environ.get(KEY, "")` form causes `test_module_import_raises_when_env_var_unset` to FAIL with `Failed: DID NOT RAISE <class 'RuntimeError'>`. Restoration returns the test to PASS. [SCN-020-002-A]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Module-import-time RED proof captured: `python -c "import os; os.environ.pop('SMACKEREL_AUTH_TOKEN', None); from app import auth"` against the post-fix `auth.py` exits non-zero with a RuntimeError traceback that names the variable + the fix path + the breadcrumbs. [SCN-020-002-A]
   → Evidence: see report.md > Validation Evidence > Module-import RED proof.
- [x] `pytest ml/tests/test_auth.py -v` PASSes unchanged — all 10 pre-existing tests PASS. [SCN-020-002-E]
   → Evidence: see report.md > Test Evidence > Targeted canary suites > test_auth.py.
- [x] `pytest ml/tests/test_main.py -v` PASSes unchanged — all 26 pre-existing tests PASS. The conftest pre-seed of `SMACKEREL_AUTH_TOKEN` is what allows the transitive `app.auth` import in `test_main.py:19`'s `from app.main import _check_required_config, health` to succeed. [SCN-020-002-D, E]
   → Evidence: see report.md > Test Evidence > Targeted canary suites > test_main.py.
- [x] `pytest ml/tests/test_startup_warning.py -v` PASSes unchanged — all 3 pre-existing tests PASS. [SCN-020-002-E]
   → Evidence: see report.md > Test Evidence > Targeted canary suites > test_startup_warning.py.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — persistent in-tree `ml/tests/test_auth_module_import_fail_loud.py` (3 tests) runs on every `./smackerel.sh test python_unit` invocation (CI + developer pre-push). The module-import-time fail-loud contract is a Python-import invariant; the regression suite IS the pytest suite itself. [SCN-020-002-A, B, C]
   → Evidence: see report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `ml/tests/` pytest suite plus the four targeted suites named above PASS, including the new conftest applying cleanly. [SCN-020-002-D, E]
   → Evidence: see report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. The `ml/tests/test_auth.py` suite is the canonical canary for the auth-module API surface; it runs on every pytest invocation and PASSes unchanged. [Spec 020 auth contract canary]
   → Evidence: targeted `pytest ml/tests/test_auth.py -v` runs the canary suite; all 10 tests PASS unchanged. The new adversarial test is purely additive against a different file (`test_auth_module_import_fail_loud.py`), so the canary cannot regress as a side effect.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single `git revert` of the BUG-020-002 commit. The change is bounded to (a) one source-line conversion in `ml/app/auth.py` (revert restores the silent-default form), (b) one new conftest file (revert removes it), (c) one new adversarial test file (revert removes it). No production runtime Go code, no live deploy compose file, no schema migration, no SST source change. Verified by the RED→GREEN proof step which temporarily reverts the source change, observes expected FAIL output, then restores.
- [x] Consumer impact sweep complete and zero stale first-party references remain. [Consumer Impact Sweep]
   → Evidence: see Consumer Impact Sweep section below.
- [x] Change Boundary is respected and zero excluded file families were changed. [Change Boundary]
   → Evidence: see Change Boundary section below; `git diff --stat` reports exactly one source file modified (`ml/app/auth.py`) plus two new test-infrastructure files (`ml/tests/conftest.py`, `ml/tests/test_auth_module_import_fail_loud.py`) plus the seven BUG-020-002 packet artifacts. Every file family in the Excluded surfaces list is bit-identical to HEAD.
- [x] Stress coverage assessment (Gate G026): explicit stress/load coverage is NOT REQUIRED for this fix. The change is a single module-import-time read pattern + a pytest collection seed + 3 new pytest unit tests; there is no latency, throughput, p95/p99, response-time, sla, or slo dimension that the change can move. The new tests run in <1s wall-clock; no daemon, no concurrency, no sustained load. This DoD line documents the assessment for the Gate G026 lint. [Broader regression]
   → Evidence: the new tests run in <1s (see Validation Evidence > Targeted adversarial suite). No stress dimension applies.

### Shared Infrastructure Impact Sweep

`ml/app/auth.py` is the **canonical auth dependency** for the ML sidecar — every non-health route uses `Depends(verify_auth)`. The module-level `_AUTH_TOKEN` constant is read ONCE at import time and consumed on every request. The BUG-020-002 fix has the following blast radius:

- **Direct downstream consumers:** every consumer of `verify_auth` (every non-health route on `smackerel-ml`). Pre-fix: silent fallback to empty string; post-fix: import raises RuntimeError if `SMACKEREL_AUTH_TOKEN` is unset, succeeds if empty (dev-mode bypass preserved) or non-empty (production token honored). The empty + non-empty paths are bit-identical to pre-fix behavior; only the unset path now fails fast instead of silently coercing.
- **Indirect downstream consumers:** every test module that imports `app.main` or `app.auth` at module-import time (e.g., `ml/tests/test_main.py:19`). Pre-fix: pytest collection silently succeeded with empty token; post-fix: pytest collection succeeds because `ml/tests/conftest.py` pre-seeds `SMACKEREL_AUTH_TOKEN=""` before any test module loads.
- **Operator-side fan-out:** none. The ML sidecar reads `SMACKEREL_AUTH_TOKEN` from its env file (`docker-compose env_file:` directive on `smackerel-ml`); operators inject the variable via the env file or via secret-injection. The fix does not change the operator interface; it only changes the failure mode when the variable is missing.
- **Adapter-side fan-out:** none. Same reason as above.
- **Test infrastructure (canary surface):** `ml/tests/test_auth.py` (10 tests) PASSES unchanged — the existing tests use `with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": auth_token})` which ADDS/OVERWRITES the key (it does not delete), so the new fail-loud read sees a non-missing variable in every existing test. `ml/tests/test_main.py` (26 tests) PASSES unchanged — the conftest pre-seed handles the import-time read. `ml/tests/test_startup_warning.py` (3 tests) PASSES unchanged.
- **Generated-artifact contract:** none. `scripts/commands/config.sh` already emits `SMACKEREL_AUTH_TOKEN=` unconditionally into every generated env file (verified at lines 483/495/499/502/504/1031). The fix does not require a new SST emission or a config schema change.
- **Bootstrap contract for downstream specs:** spec 040 (MIT-040-S-004 — production-mode auth-token fail-fast in `_check_required_config`) is unaffected — the existing branching at `ml/app/main.py:147-150` continues to convert empty + production to `sys.exit(1)`. Spec 020 (this fix's parent) gains a module-import-time adversarial that locks the canonical Python fail-loud pattern against future drift on `ml/app/auth.py`.
- **Rollback or restore plan:** see the corresponding DoD item — single `git revert`; no live-file or runtime-behavior mismatch possible because the fix only adds new failure modes for previously-silent misconfiguration paths and does not remove any existing functionality.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix is a single module-import-time read + a pytest collection seed + 3 new unit tests; no daemon state, no shared cache, no cross-process ordering concern.

### Consumer Impact Sweep

This bug fix does **not** rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol, deep link, breadcrumb, navigation entry, or generated client. The change is bounded to:

- **`ml/app/auth.py`:** the module-level `_AUTH_TOKEN` constant is preserved (still module-scope, still read once at import). Only the read pattern changes (silent default → fail-loud try/except). The `verify_auth` function signature is bit-identical to HEAD. No public symbol removed, no signature changed.
- **`ml/tests/conftest.py`:** purely additive new file. pytest auto-discovers it via the standard collection path.
- **`ml/tests/test_auth_module_import_fail_loud.py`:** purely additive new test file. Test functions are not importable across packages.
- **No public API change.** No HTTP route, no NATS subject, no CLI flag, no env-var name, no config-key, no URL path, no breadcrumb, no redirect surface, no generated client regeneration.
- **Affected consumer surfaces enumerated:** the consumers of the new fail-loud read are (a) every test module that imports `app.auth` at collection time (e.g., `test_main.py`); (b) every runtime entrypoint that imports `app.auth` (uvicorn loading `app.main:app` → loading `app.auth`). All consumer surfaces are explicitly named and validated by the test plan. No documentation or operator runbook references the new conftest by name.
- **Cross-package consumer surface:** zero. The new test file's helpers (`_reload_auth_module`) are not imported by any other test.
- **Stale-reference scan:** zero stale first-party references remain. The pre-existing `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` form did not have any test that referenced it by name, so there is nothing to update.

### Change Boundary

**Allowed file families (this fix may modify):**

- `ml/app/auth.py` — the canonical auth dependency (line 11 silent-default read converted to canonical Python fail-loud pattern)
- `ml/tests/conftest.py` — NEW pytest collection seed (pre-seeds SMACKEREL_AUTH_TOKEN to empty string)
- `ml/tests/test_auth_module_import_fail_loud.py` — NEW persistent in-tree adversarial test
- `specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `ml/app/main.py` — sequel packet (existing `_check_required_config()` lifespan branching at lines 147-150 already converts empty + production to `sys.exit(1)`; the silent default at line 146 is an audit-cleanliness improvement, not a runtime fix)
- `ml/app/nats_client.py` — sequel packet (parallel session has uncommitted cosmetic reformatting in this file at lines 153 and 168; touching line 180 here would mix two unrelated changes)
- `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_ocr.py`, `ml/tests/test_main.py` — bit-identical to HEAD (parallel session's cosmetic WIP is not part of this packet)
- `cmd/core/wiring.go` — bit-identical to HEAD (Go-side equivalent fail-loud read is already correct)
- `internal/auth/devtoken.go` — bit-identical to HEAD (Go-side runtime auth dev-token surface)
- `scripts/commands/config.sh` — bit-identical to HEAD (already emits SMACKEREL_AUTH_TOKEN= unconditionally)
- `config/smackerel.yaml` — bit-identical to HEAD (no SST source change required)
- `docker-compose.yml`, `deploy/compose.deploy.yml`, `Dockerfile`, `ml/Dockerfile` — bit-identical to HEAD (no infra change)
- `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/scopes.md`, `specs/020-security-hardening/state.json`, `specs/020-security-hardening/uservalidation.md`, `specs/020-security-hardening/report.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- `specs/040-*/**` (any spec 040 directory) — foreign-owned (different parent spec; the MIT-040-S-004 production-mode auth fail-fast contract is preserved unchanged)
- `.github/workflows/*` — bit-identical to HEAD (the existing `python-unit` job picks up the new test automatically)
- `scripts/lib/runtime.sh`, `scripts/runtime/python-unit.sh` — bit-identical to HEAD (no runtime invocation pattern change required)
- Any other `specs/**` directory — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-020-002-A: Module import raises RuntimeError when env var unset | test_module_import_raises_when_env_var_unset | ml/tests/test_auth_module_import_fail_loud.py | unit (Python module-import contract) | YES — fails RED if line 22 is reverted |
| SCN-020-002-B: Module import succeeds when env var is empty | test_module_import_succeeds_with_empty_value | ml/tests/test_auth_module_import_fail_loud.py | unit (Python module-import contract) | NO (positive contract) |
| SCN-020-002-C: Module import succeeds when env var holds a real token value | test_module_import_succeeds_with_real_value | ml/tests/test_auth_module_import_fail_loud.py | unit (Python module-import contract) | NO (positive contract) |
| SCN-020-002-D: pytest conftest pre-seeds env var before collection | (proven by every other test module continuing to import cleanly) | ml/tests/conftest.py | unit (pytest collection contract) | NO (positive infrastructure contract) |
| SCN-020-002-E: Existing test suites pass unchanged after the fix | test_auth.py (10) + test_main.py (26) + test_startup_warning.py (3) | ml/tests/test_auth.py + ml/tests/test_main.py + ml/tests/test_startup_warning.py | unit (Python regression canary) | NO (positive canary) |
| Canary: ml/tests/test_auth.py contract preserved | (entire pre-existing suite) | ml/tests/test_auth.py | unit (Python regression canary) | NO (positive canary) |
| Canary: spec 040 MIT-040-S-004 production-mode fail-fast preserved | test_check_required_config_* (entire pre-existing suite in test_main.py) | ml/tests/test_main.py | unit (Python regression canary) | NO (positive canary) |
