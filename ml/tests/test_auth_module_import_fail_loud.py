"""HL-RESCAN-013 / Gate G028 — adversarial regression tests for the
module-import-time fail-loud read of SMACKEREL_AUTH_TOKEN in
`ml/app/auth.py`.

These tests prove that:

1. (adversarial) Importing `app.auth` with SMACKEREL_AUTH_TOKEN
   UNSET in `os.environ` raises a RuntimeError whose message names
   the variable. Reverting `_AUTH_TOKEN = os.environ[...]` to the
   previous `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` form would
   cause this test to FAIL — the import would silently succeed with
   `_AUTH_TOKEN == ""`.

2. Importing `app.auth` with SMACKEREL_AUTH_TOKEN set to an empty
   string succeeds and leaves `_AUTH_TOKEN == ""`. Empty is the
   SST-sanctioned dev-mode auth-bypass signal and MUST remain
   allowed; the production-vs-dev branching that converts empty +
   SMACKEREL_ENV=production to sys.exit(1) lives in
   `ml/app/main.py:_check_required_config`, not in `auth.py`.

3. Importing `app.auth` with SMACKEREL_AUTH_TOKEN set to a real
   non-empty token succeeds and `_AUTH_TOKEN` equals the value
   provided.
"""

import importlib
import os
import sys

import pytest


def _reload_auth_module():
    """Force re-import of `app.auth` so the module-import-time read
    happens against the current `os.environ` state. Returns the
    reloaded module object on success; the caller may also wrap the
    call in `pytest.raises(...)` to assert the failure path.
    """
    if "app.auth" in sys.modules:
        del sys.modules["app.auth"]
    return importlib.import_module("app.auth")


def test_module_import_raises_when_env_var_unset(monkeypatch):
    """HL-RESCAN-013 adversarial: SMACKEREL_AUTH_TOKEN UNSET must
    raise RuntimeError naming the variable.

    Reverting `ml/app/auth.py` line 22 from
    ``_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]`` back to
    ``_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")``
    would silently succeed and this test would FAIL — proving the
    test catches a regression of the Gate G028 fail-loud contract.
    """
    monkeypatch.delenv("SMACKEREL_AUTH_TOKEN", raising=False)
    assert "SMACKEREL_AUTH_TOKEN" not in os.environ, (
        "test setup error: SMACKEREL_AUTH_TOKEN must be unset before reload"
    )
    with pytest.raises(RuntimeError, match=r"SMACKEREL_AUTH_TOKEN"):
        _reload_auth_module()


def test_module_import_succeeds_with_empty_value(monkeypatch):
    """Empty value is the SST-sanctioned dev-mode auth-bypass signal
    and MUST remain allowed at module-import time.
    """
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")
    auth_mod = _reload_auth_module()
    assert auth_mod._AUTH_TOKEN == "", (
        f"empty SMACKEREL_AUTH_TOKEN should set _AUTH_TOKEN to '', got {auth_mod._AUTH_TOKEN!r}"
    )


def test_module_import_succeeds_with_real_value(monkeypatch):
    """Non-empty value is the production case. Module-import-time
    read MUST succeed and `_AUTH_TOKEN` MUST equal the value passed.
    """
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "test-secret-real-value")
    auth_mod = _reload_auth_module()
    assert auth_mod._AUTH_TOKEN == "test-secret-real-value", (
        f"_AUTH_TOKEN should equal env value 'test-secret-real-value', got {auth_mod._AUTH_TOKEN!r}"
    )
