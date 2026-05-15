# Design: BUG-020-002 — `ml/app/auth.py` reads `SMACKEREL_AUTH_TOKEN` at module import using the silent-default `os.environ.get(KEY, "")` form, violating Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Approach

Three coordinated changes that together close the Gate G028 violation at the highest-priority site (`ml/app/auth.py:11`, the canonical auth dependency for every non-health route via `Depends(verify_auth)`) while keeping pytest collection working when invoked outside an env-file context:

1. **`ml/app/auth.py`** — convert the module-level `_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` to the canonical Python fail-loud pattern: `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in `try / except KeyError as exc → raise RuntimeError(...) from exc`. The RuntimeError message names the variable, names the fix path (`run ./smackerel.sh config generate`), and includes the breadcrumb tokens `Gate G028` and `HL-RESCAN-013`. An EMPTY string is still allowed at module-import time — it is the SST-sanctioned dev-mode auth-bypass signal and is honoured by `verify_auth()` in the same module + the production-vs-dev branching in `ml/app/main.py:_check_required_config` (which converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)`).
2. **`ml/tests/conftest.py`** (NEW FILE) — calls `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup. The conftest runs ONCE before any test module is collected, so test modules that import `app.auth` at module-import time (e.g., `ml/tests/test_main.py:19` does `from app.main import _check_required_config, health`, which transitively imports `app.auth` via `ml/app/main.py:12`'s `from .auth import verify_auth`) see `SMACKEREL_AUTH_TOKEN` already in `os.environ` and the new fail-loud read succeeds. The `setdefault` form preserves any externally-provided value (e.g., when `./smackerel.sh test python_unit` invokes pytest with the env file already loaded), so the conftest is a no-op in the sanctioned workflow.
3. **`ml/tests/test_auth_module_import_fail_loud.py`** (NEW FILE) — a persistent in-tree adversarial test that mechanically locks the fail-loud contract against future drift. It declares 3 test functions: (a) adversarial `test_module_import_raises_when_env_var_unset` (uses `monkeypatch.delenv` then asserts `pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN")` on a forced re-import via the `_reload_auth_module()` helper that deletes `app.auth` from `sys.modules` then re-imports); (b) positive `test_module_import_succeeds_with_empty_value`; (c) positive `test_module_import_succeeds_with_real_value`. The adversarial test's docstring explicitly documents the regression target — reverting `ml/app/auth.py` line 22 from `os.environ["SMACKEREL_AUTH_TOKEN"]` back to `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` would cause `test_module_import_raises_when_env_var_unset` to FAIL because the import would silently succeed and `pytest.raises` would not trigger.

The fix is mechanically the same shape as the Go-side equivalent at `cmd/core/wiring.go:37,57,59` — the difference is that the Go side already uses `os.Getenv("SMACKEREL_AUTH_TOKEN")` + `if token == "" && env == "production" { log.Fatal(...) }` correctly, but the Python ML sidecar's auth.py was importing the variable with a silent default. The fix brings auth.py into parity with the Go side at the module-import boundary.

## Design Decisions

### DD-1: Three-file change boundary (auth.py + conftest.py + new test file), main.py and nats_client.py scoped to a sequel packet

**Decision:** This packet modifies exactly three files in `ml/`: (a) `ml/app/auth.py` (the canonical auth dependency, line 11); (b) `ml/tests/conftest.py` (NEW, pre-seeds the env for pytest collection); (c) `ml/tests/test_auth_module_import_fail_loud.py` (NEW, adversarial). The two other Gate G028 violations on the same env var — `ml/app/main.py:146` and `ml/app/nats_client.py:180` — are scoped to sequel packets.

**Rationale:** The three sites have meaningfully different runtime risk profiles. `ml/app/auth.py:11` runs at module-import time, BEFORE any startup check has a chance to validate the env, and is imported by every consumer of the auth dependency — it is the canonical fail-loud point. `ml/app/main.py:146` runs INSIDE `_check_required_config()` which is the lifespan startup check; the existing branching at lines 147-150 converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)`, so the unset case would still flow through to the production branch and exit. The runtime behavior with `os.environ.get(KEY, "")` vs `os.environ[KEY]` at this site is functionally equivalent in the production branch (both result in `sys.exit(1)`); the silent default there is an audit-cleanliness improvement, not a runtime fix. `ml/app/nats_client.py:180` runs INSIDE `connect()` which is invoked AFTER `_check_required_config()` validates the env, so the silent default there is functionally redundant.

A second reason to scope the main.py + nats_client.py sites to sequel packets is that a parallel session has uncommitted cosmetic reformatting in `ml/app/nats_client.py` at lines 153 and 168 (multi-line `raise RuntimeError(...)` collapsed to single-line). Touching `ml/app/nats_client.py:180` in this packet would either (a) require staging the parallel session's WIP cosmetic edits alongside the fail-loud fix (mixing two unrelated changes), or (b) require selective `git add -p` per-hunk staging (brittle from automation). Scoping the nats_client.py site to a sequel packet to be filed once the parallel session merges keeps this packet's change boundary clean.

**Alternatives rejected:**
- Sweep all three sites in this packet: rejected per the parallel-session conflict rationale above + the change-boundary discipline guidance.
- Sweep only auth.py + main.py (scope nats_client.py to a sequel packet): considered. Rejected because the main.py change is functionally a no-op (the production branch already exits), so the audit-cleanliness benefit is minimal and the marginal review cost is non-zero. Sequel-packet treatment groups all three audit-cleanliness sites together for a single coherent follow-up.
- Sweep only auth.py (no conftest.py change, no new test): rejected because the source change without the conftest would break pytest collection in unsanctioned invocation contexts (e.g., `pytest ml/tests/test_main.py` from a clean shell), and the source change without the new adversarial test would have no mechanical regression guard.

### DD-2: Use `try / except KeyError → raise RuntimeError from exc` instead of `os.environ[KEY]` direct

**Decision:** Wrap the `os.environ[KEY]` read in `try / except KeyError as exc → raise RuntimeError(...) from exc` rather than letting the bare `KeyError` propagate.

**Rationale:** The bare `KeyError` from `os.environ["SMACKEREL_AUTH_TOKEN"]` produces the message `'SMACKEREL_AUTH_TOKEN'` (just the var name in single quotes, no context). That message is technically fail-loud but does not name the fix path or explain why the variable is required. Wrapping in `RuntimeError(...)` lets the message include: (a) the file path that raised (`ml/app/auth.py:`), (b) the var name (`SMACKEREL_AUTH_TOKEN`), (c) the fix path (`run ./smackerel.sh config generate`), (d) the dev-mode bypass disclaimer (empty value is allowed when `SMACKEREL_ENV=development|test`), (e) the breadcrumb tokens (`HL-RESCAN-013` and `Gate G028 fail-loud SST contract`). The `from exc` clause preserves the original `KeyError` in the traceback so the root cause is not hidden.

This pattern matches the canonical fail-loud message style used elsewhere in the codebase: `internal/auth/devtoken.go` log.Fatal messages, `scripts/lib/runtime.sh` named-var Compose error messages, and `internal/deploy/compose_contract_test.go` named-var assertions.

**Alternatives rejected:**
- Bare `os.environ[KEY]` (let KeyError propagate): rejected because the message is too terse and does not name the fix path.
- Wrap in `OSError` or `EnvironmentError`: rejected because `RuntimeError` is the conventional choice for "this should never happen at runtime under normal operation"; `OSError` would imply a file-system error which is misleading.
- Use `sys.exit(1)` with `print(...)`: rejected because sys.exit at module-import time is harder to debug than a RuntimeError (the import-time exception shows up cleanly in tracebacks).

### DD-3: Add `ml/tests/conftest.py` to pre-seed `SMACKEREL_AUTH_TOKEN` for pytest collection

**Decision:** Create `ml/tests/conftest.py` that calls `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup, BEFORE any test module is collected.

**Rationale:** Several test modules import from `app.main` or `app.auth` at module-import time. `ml/tests/test_main.py:19` does `from app.main import _check_required_config, health`, which transitively imports `app.auth` via `ml/app/main.py:12`'s `from .auth import verify_auth`. `ml/tests/test_embedder.py:232` does `from app.main import health` inside a function (so the import only happens when the test runs, not at collection). With the new fail-loud read in auth.py, an unsanctioned `pytest ml/tests/test_main.py` invocation from a clean shell (no env file loaded) would crash at pytest collection time with `RuntimeError: ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set...`.

The conftest pre-seeds `SMACKEREL_AUTH_TOKEN` to an empty string IF AND ONLY IF the variable is not already set (`setdefault`). An empty value is the SST-sanctioned dev-mode auth-bypass signal, so this preserves the same observable behaviour as the previous `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` default for unrelated tests, without re-introducing the silent default inside the production module itself. When the developer invokes pytest via `./smackerel.sh test python_unit` (which loads the env file), `SMACKEREL_AUTH_TOKEN` is already in `os.environ` and the conftest's `setdefault` is a no-op.

The conftest is at `ml/tests/conftest.py` (not `ml/conftest.py`) to scope it to the test directory only. A repository-wide `conftest.py` would risk affecting non-test contexts (e.g., production import via `uvicorn app.main:app`).

**Alternatives rejected:**
- Add `[tool.pytest.ini_options]` env settings to `ml/pyproject.toml`: considered. Rejected because pytest's built-in `env` config requires the `pytest-env` plugin (not in `ml/pyproject.toml` dependencies); adding the plugin to `dev = [...]` would expand the dependency surface.
- Pre-export `SMACKEREL_AUTH_TOKEN=""` in `scripts/runtime/python-unit.sh`: considered. Rejected because the runtime script is the sanctioned-workflow entrypoint; a developer who runs `pytest ml/tests/...` directly (bypassing the runtime script) would still see the crash. The conftest catches both invocation paths.
- Hard-fail at pytest collection if `SMACKEREL_AUTH_TOKEN` is unset (no conftest pre-seed): rejected because it would make `pytest ml/tests/test_main.py` from a clean shell harder to use, with no security benefit (an empty token is already the dev-mode bypass signal).

### DD-4: Adversarial test uses `monkeypatch.delenv` AFTER the conftest pre-seed

**Decision:** The adversarial test `test_module_import_raises_when_env_var_unset` uses `monkeypatch.delenv("SMACKEREL_AUTH_TOKEN", raising=False)` to clear the variable BEFORE forcing a re-import via `_reload_auth_module()`. The `raising=False` arg makes the call a no-op when the variable is already unset, so the test passes the test-setup invariant assertion `assert "SMACKEREL_AUTH_TOKEN" not in os.environ`.

**Rationale:** The conftest pre-seeds `SMACKEREL_AUTH_TOKEN=""`, so by the time the adversarial test runs, the variable IS in `os.environ`. The test must explicitly delete it to exercise the fail-loud path. `monkeypatch` is the right tool because it auto-restores the env after the test function returns (so subsequent tests are unaffected). The `_reload_auth_module()` helper (`del sys.modules["app.auth"]` + `importlib.import_module("app.auth")`) forces a fresh module-import-time read against the current `os.environ` state — without this, Python would return the already-cached `app.auth` module from `sys.modules` and the test would silently pass for the wrong reason.

The `pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN")` form asserts both the exception type AND that the variable name appears in the message. Reverting line 22 to `os.environ.get(KEY, "")` would silently succeed and `pytest.raises` would not trigger — the test would FAIL with `Failed: DID NOT RAISE <class 'RuntimeError'>`, naming the regression precisely.

**Alternatives rejected:**
- Use `os.environ.pop("SMACKEREL_AUTH_TOKEN", None)` directly without monkeypatch: rejected because the env mutation would persist across tests and pollute the test environment.
- Use `pytest.raises(RuntimeError)` without the `match` arg: rejected because it would pass even if the RuntimeError message did not name the variable, weakening the contract.
- Test the helper function `verify_auth()` directly with monkeypatch: rejected because that exercises the runtime path, not the module-import-time path. The whole point of HL-RESCAN-013 is to lock the import-time read; the runtime path was already covered by `ml/tests/test_auth.py`.

### DD-5: HL-RESCAN-013 attribution in source comment + test docstring

**Decision:** Mention `HL-RESCAN-013` in (a) the source-code comment block above the fail-loud read in `ml/app/auth.py`, (b) the RuntimeError message itself, and (c) the test file's package docstring + each adversarial test docstring.

**Rationale:** Same rationale as BUG-029-003 DD-6: the breadcrumb belongs in the meta-evidence (docstring + fail message + comment), not in the contract-surface assertion. A future maintainer hitting the RuntimeError message at runtime, or hitting the adversarial test failure in CI, sees the `HL-RESCAN-013` reference and can navigate to this bug packet for context. The breadcrumb in the source comment block above the read makes the rationale visible to anyone reading the file.

**Alternatives rejected:**
- Skip HL-RESCAN-013 attribution: rejected because future maintainers benefit from the breadcrumb back to the discovering finding.

### DD-6: Module-level `_AUTH_TOKEN` constant is preserved (not made dynamic)

**Decision:** The fix preserves the existing module-level `_AUTH_TOKEN` constant pattern. The variable is read ONCE at module-import time and cached for the lifetime of the process. The `verify_auth()` function reads `_AUTH_TOKEN` from module scope on every request.

**Rationale:** The existing pattern is correct for a stateless server — the auth token is set at process start (via the env file) and does not change during the process lifetime. Making `_AUTH_TOKEN` dynamic (e.g., `os.environ.get(KEY, "")` inside `verify_auth()` on every request) would introduce per-request env-lookup overhead with no security benefit (an attacker who can mutate `os.environ` already has process-level access). The existing `ml/tests/test_auth.py` tests rely on the module-level pattern via `importlib.reload(auth_mod)` to test different token values; preserving the pattern keeps those tests valid.

**Alternatives rejected:**
- Make `_AUTH_TOKEN` dynamic via per-request env lookup: rejected per rationale above.
- Wrap `_AUTH_TOKEN` in a lazy property: rejected as overengineered for a value that does not change at runtime.

## Trade-offs

| Option | Pros | Cons | Decision |
|---|---|---|---|
| Sweep all three sites in this packet | Single coherent fix for all Gate G028 violations on `SMACKEREL_AUTH_TOKEN` in `ml/` | Conflicts with parallel session's WIP in `ml/app/nats_client.py` | Rejected — scope to auth.py only |
| Use bare `os.environ[KEY]` (let KeyError propagate) | Simpler code | Terse error message; no fix-path guidance | Rejected — wrap in RuntimeError |
| `pyproject.toml` env settings instead of conftest.py | Declarative | Requires `pytest-env` plugin; expands deps | Rejected — use conftest.py |
| Hard-fail pytest collection if env unset (no conftest pre-seed) | Maximally strict | Breaks unsanctioned-but-legitimate `pytest ...` invocations | Rejected — pre-seed via conftest |
| Direct env mutation without monkeypatch | Slightly less code | Pollutes test env across tests | Rejected — use monkeypatch |
| Skip `pytest.raises(match=...)` | Slightly less code | Weaker contract assertion | Rejected — keep match arg |
| Skip HL-RESCAN-013 attribution | Slightly less verbose | No breadcrumb for future maintainers | Rejected — keep attribution |

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Existing `ml/tests/test_auth.py` tests break because the new fail-loud read trips on the test's `patch.dict("os.environ", {...})` not deleting `SMACKEREL_AUTH_TOKEN` | Low | High (regression) | The conftest pre-seeds `SMACKEREL_AUTH_TOKEN=""` BEFORE pytest collection, so the variable is always in `os.environ` when test modules are imported. The existing `patch.dict` form ADDS/OVERWRITES the key (it does not delete), so the new fail-loud read sees a non-missing variable in every existing test. Verified by running `pytest ml/tests/test_auth.py -v` against the post-fix state — all 10 tests PASS. |
| `ml/tests/test_main.py:19`'s `from app.main import _check_required_config, health` crashes at pytest collection because the transitive `from .auth import verify_auth` triggers the new fail-loud read | Medium (without the conftest fix) | High (regression) | The conftest pre-seeds `SMACKEREL_AUTH_TOKEN=""` BEFORE any test module is collected. Verified by running `pytest ml/tests/test_main.py -v` against the post-fix state — all 26 tests PASS. |
| `ml/tests/test_embedder.py:232`'s `from app.main import health` crashes when the test runs | Low | Medium | The conftest pre-seed is in place when the test runs, so the inner import succeeds. Verified indirectly by the broader test suite passing. |
| Production / docker-compose runtime crashes because `SMACKEREL_AUTH_TOKEN` is missing from the env file | Very Low | High (production outage) | `scripts/commands/config.sh` emits `SMACKEREL_AUTH_TOKEN=` unconditionally into every generated env file (verified at lines 483/495/499/502/504/1031). The runtime path that loads the env file (docker-compose `env_file:` directive on `smackerel-ml`) always carries the variable. Production users who set the variable explicitly via secret-injection see the explicit value; production users who forget see the empty string + `SMACKEREL_ENV=production` → `sys.exit(1)` from `_check_required_config()` lifespan startup. The new fail-loud read at module-import time is satisfied in every documented production path. |
| Future agent reverts the fail-loud form back to `os.environ.get(KEY, "")` thinking it is "simpler" | Medium (over the long run) | High (silent regression of Gate G028) | The new adversarial test `test_module_import_raises_when_env_var_unset` mechanically catches any such reversion. The test's failure message names the variable + the regression target, so the maintainer sees the breadcrumb back to HL-RESCAN-013 immediately. |
| Parallel session's cosmetic reformatting of `ml/app/nats_client.py` lands while this packet is in review and the merge introduces a conflict | Low | Low | This packet does not touch `ml/app/nats_client.py`. The two sessions modify disjoint files. |

## Implementation

See [scopes.md](scopes.md) for the per-scope DoD checklist with concrete file-paths, test-IDs, and gate evidence references.
