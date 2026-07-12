# Bug: BUG-020-002 — `ml/app/auth.py` reads `SMACKEREL_AUTH_TOKEN` at module import using the silent-default `os.environ.get(KEY, "")` form, violating Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Classification

- **Type:** Security defect — fail-loud SST contract gap on the ML sidecar auth token (operational hardening)
- **Severity:** P3 — LOW (the runtime contract for SMACKEREL_AUTH_TOKEN is already enforced once the lifespan startup check `_check_required_config()` runs in `ml/app/main.py:141-150`: production + empty value → `sys.exit(1)`. The pre-fix gap is at module-import time: `ml/app/auth.py` line 11 silently coerces an UNSET env var to an empty string, so a developer who imports `app.auth` outside the sanctioned `./smackerel.sh up` / docker-compose env_file path sees no error and silently gets dev-mode bypass behaviour. This masks real misconfiguration in unsanctioned import contexts and does not match the canonical Python fail-loud pattern documented in `.github/copilot-instructions.md` Secrets Management table — `os.environ["KEY"]` (raises KeyError) is REQUIRED, `os.getenv("KEY", "default")` is FORBIDDEN.)
- **Parent Spec:** 020 — Security Hardening (owns the SMACKEREL_AUTH_TOKEN contract; see `specs/020-security-hardening/spec.md` and the existing fail-loud branching in `ml/app/main.py:_check_required_config`)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 self-hosted readiness re-scan (finding HL-RESCAN-013)

## Problem Statement

The ML sidecar reads its auth token from `SMACKEREL_AUTH_TOKEN`. The Smackerel runtime contract per `.github/copilot-instructions.md` is:

> **SST Zero-Defaults Enforcement (NON-NEGOTIABLE)** — ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase.
>
> | Language | FORBIDDEN | REQUIRED |
> |----------|-----------|----------|
> | **Python** | `os.getenv("KEY", "default")` | `os.environ["KEY"]` (raises KeyError) |

The pre-fix `ml/app/auth.py` line 11 was:

```python
_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
```

This silently coerces an UNSET `SMACKEREL_AUTH_TOKEN` env var to an empty string at module-import time. The sanctioned developer workflow (`./smackerel.sh up` invokes Compose with `--env-file config/generated/dev.env`, which ALWAYS contains `SMACKEREL_AUTH_TOKEN=` because `scripts/commands/config.sh` emits the line unconditionally — empty for dev, auto-generated 48-hex for test) makes the silent fallback functionally invisible in the happy path. But:

1. **Unsanctioned import contexts** (a developer who runs `python -c "from app.auth import verify_auth"` without sourcing the env file, or any tooling that imports `app.auth` outside the docker-compose env_file path) silently get `_AUTH_TOKEN == ""` and the sidecar enters dev-mode auth bypass with no warning.
2. **Test-collection contexts** (pytest collecting `ml/tests/test_main.py`, which transitively imports `app.auth` via `from app.main import _check_required_config, health` at module-import time) historically also relied on the silent default to keep pytest collection from crashing, masking the gap.
3. **Code-search audit drift** (a future agent grepping for `os.environ.get` to enumerate Gate G028 violations would find this line and have to decide whether it is exempt — the silent-default form is forbidden by policy but the runtime branching elsewhere makes the runtime risk low).

The fix converts line 11 to the canonical Python fail-loud pattern (`os.environ[KEY]` wrapped in try/except KeyError → RuntimeError naming the variable + Gate G028 + HL-RESCAN-013 + the fix path `./smackerel.sh config generate`). An EMPTY string is still allowed: it is the SST-sanctioned dev-mode auth-bypass signal and is honoured by `verify_auth()` in the same module + the production-vs-dev branching in `ml/app/main.py:_check_required_config` (which converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)`). To keep pytest collection working when the developer invokes pytest outside an env-file context, a new `ml/tests/conftest.py` calls `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup BEFORE any test module is collected — this preserves the same observable behaviour as the previous silent default for unrelated tests, without re-introducing the silent default inside the production module itself. A new persistent in-tree adversarial test `ml/tests/test_auth_module_import_fail_loud.py` mechanically locks the fail-loud contract against future drift: it uses `monkeypatch.delenv` to clear the variable AFTER the conftest seed, then asserts that `import app.auth` raises RuntimeError naming the variable. Reverting the line 11 change would silently succeed and cause the adversarial test to FAIL.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | self-hosted readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-013 |
| Severity | P3 (the runtime branching in `ml/app/main.py:_check_required_config` already converts empty + production to `sys.exit(1)`; the gap is at module-import time and in unsanctioned import contexts) |
| Audit method | `grep -nE 'os\.environ\.get\("SMACKEREL_AUTH_TOKEN"' ml/` returned `ml/app/auth.py:11`, `ml/app/main.py:146`, `ml/app/nats_client.py:180`. Cross-referenced with `.github/copilot-instructions.md` Secrets Management table. The `ml/app/auth.py:11` site is the highest-priority violation because it runs at module-import time (before any startup check has a chance to validate the env), and the named file is the canonical auth dependency for every non-health route via `Depends(verify_auth)`. The `ml/app/main.py:146` site runs INSIDE `_check_required_config()` which is the lifespan startup check — the unset case would still flow through to the production branch and exit, so the runtime risk is lower; the silent default there is functionally equivalent in current behaviour but should be migrated as a sequel packet for code-search audit cleanliness. The `ml/app/nats_client.py:180` site runs INSIDE `connect()` which is invoked AFTER `_check_required_config()` validates the env; the silent default there is functionally redundant. The latter two sites are scoped to a sequel packet to keep this fix minimum-touch and to avoid colliding with a parallel session that has uncommitted changes in `ml/app/nats_client.py`. |

## Acceptance Criteria

- AC-1: `ml/app/auth.py` line 11 uses the canonical Python fail-loud pattern: `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in `try / except KeyError as exc → raise RuntimeError(...) from exc`. The RuntimeError message names the variable `SMACKEREL_AUTH_TOKEN`, names the fix path `run ./smackerel.sh config generate`, and includes the breadcrumb tokens `Gate G028` and `HL-RESCAN-013`.
- AC-2: `ml/app/auth.py` retains its existing dev-mode auth-bypass behaviour for an EMPTY string value: `_AUTH_TOKEN == ""` → `verify_auth()` returns immediately without checking headers (preserves the contract documented at `ml/app/auth.py` `verify_auth` docstring `"When SMACKEREL_AUTH_TOKEN is empty, all requests pass (dev mode)."`).
- AC-3: `ml/tests/conftest.py` exists, calls `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` at pytest startup, and is loaded by pytest BEFORE any test module is collected. The conftest preserves the same observable behaviour as the previous silent default for tests that invoke pytest outside an env-file context (e.g., `pytest ml/tests/test_main.py` from a clean shell).
- AC-4: A new persistent in-tree adversarial test `ml/tests/test_auth_module_import_fail_loud.py` exists. It declares 3 test functions: (a) adversarial `test_module_import_raises_when_env_var_unset` (uses `monkeypatch.delenv` then asserts `pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN")` on a forced re-import); (b) positive `test_module_import_succeeds_with_empty_value` (asserts `_AUTH_TOKEN == ""` after `monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")` + reload); (c) positive `test_module_import_succeeds_with_real_value` (asserts `_AUTH_TOKEN == "test-secret-real-value"` after `monkeypatch.setenv` + reload).
- AC-5: The adversarial test's docstring explicitly documents the regression target: reverting `ml/app/auth.py` line 22 (the `os.environ[KEY]` form) back to `os.environ.get(KEY, "")` MUST cause `test_module_import_raises_when_env_var_unset` to FAIL — the import would silently succeed and `pytest.raises` would not trigger.
- AC-6: All 3 new tests PASS via `pytest ml/tests/test_auth_module_import_fail_loud.py -v`. Pre-existing tests `ml/tests/test_auth.py` (10 tests), `ml/tests/test_main.py` (26 tests), and `ml/tests/test_startup_warning.py` (3 tests) PASS unchanged after the source + conftest changes.
- AC-7: HL-RESCAN-013 attribution is present in either the test docstring or the source-code comment block above the fail-loud read in `ml/app/auth.py`, so a future regression points back to this bug.
- AC-8: Module-import-time RED proof captured: a `python -c "import os; os.environ.pop('SMACKEREL_AUTH_TOKEN', None); from app import auth"` invocation against the post-fix `auth.py` exits non-zero with a `RuntimeError: ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set...` traceback.

## Out of Scope

- `ml/app/main.py` line 146 — the existing `_check_required_config()` lifespan check uses `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` and then converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)` at lines 147-150. The runtime risk of the silent default there is bounded by the production branch; converting it to the fail-loud form is an audit-cleanliness improvement. Scoped to a sequel packet to keep this fix minimum-touch.
- `ml/app/nats_client.py` line 180 — runs INSIDE `connect()` which is invoked AFTER `_check_required_config()` validates the env, so the silent default there is functionally redundant. A parallel session has uncommitted cosmetic reformatting in `ml/app/nats_client.py` at lines 153 and 168 (multi-line `raise RuntimeError(...)` collapsed to single-line); touching line 180 in this packet would conflict with that parallel session. Scoped to a sequel packet to be filed once the parallel session merges.
- Editing `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/scopes.md`, `specs/020-security-hardening/state.json`, `specs/020-security-hardening/uservalidation.md`, or `specs/020-security-hardening/report.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope.
- Modifying the SST emission of `SMACKEREL_AUTH_TOKEN` in `scripts/commands/config.sh` — already correct (lines 483/495/499/502/504/1031 emit the line unconditionally; empty for dev, auto-generated 48-hex for test). Verified by `grep -n SMACKEREL_AUTH_TOKEN= config/generated/dev.env config/generated/test.env`.
- Editing `cmd/core/wiring.go` — the Go-side equivalent fail-loud read (lines 37/57/59) is already correct (`os.Getenv("SMACKEREL_AUTH_TOKEN")` + empty-and-production → log.Fatal pattern). Outside the Python ML sidecar boundary.
- Editing `.github/workflows/ci.yml` — unrelated; the existing `python-unit` job picks up the new test automatically.
- Adding a separate fail-loud check in `verify_auth()` itself — the module-import-time read is the canonical fail-loud point. The runtime `verify_auth()` function preserves its existing dev-mode bypass on empty value.
