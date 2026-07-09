"""HL-RESCAN-013 / Gate G028 — pre-seed SMACKEREL_AUTH_TOKEN before any
test module is collected by pytest.

The module-import-time fail-loud read in `ml/app/auth.py` raises a
RuntimeError when SMACKEREL_AUTH_TOKEN is UNSET in os.environ. Several
test modules (e.g. `test_main.py`, `test_embedder.py`) import from
`app.main`, which transitively imports `app.auth`, at module-collection
time — well before any test fixture has a chance to monkeypatch the
environment.

To keep the contract intact while still allowing pytest to be invoked
without an env-file context (the developer ergonomic case), this
conftest sets SMACKEREL_AUTH_TOKEN to an empty string IF AND ONLY IF
the variable is not already set. An empty value is the SST-sanctioned
dev-mode auth-bypass signal, so this preserves the same observable
behaviour as the previous `os.environ.get("SMACKEREL_AUTH_TOKEN", "")`
default for unrelated tests, without re-introducing the silent default
inside the production module itself.

The adversarial test in `test_auth_module_import_fail_loud.py` proves
the fail-loud contract by using `monkeypatch.delenv` to clear the
variable AFTER this seed has been applied.
"""

import os

os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")

# F2 (redteam LLM-enrichment cold-load) — the ML sidecar's ollama completions
# read ML_OLLAMA_KEEP_ALIVE fail-loud at CALL time (ml/app/ollama_keepalive.py).
# Several unit tests drive the ollama code path (test_processor / test_domain /
# test_synthesis), so seed a value here IFF unset — the same developer-ergonomic
# setdefault pattern used for SMACKEREL_AUTH_TOKEN above, and NOT a default in
# the production module. The fail-loud contract itself is proven adversarially
# in test_ollama_keepalive.py via monkeypatch.delenv.
os.environ.setdefault("ML_OLLAMA_KEEP_ALIVE", "30m")
