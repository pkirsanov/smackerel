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

# Spec 102 SCOPE-102-03 — every Ollama request now resolves a positive num_ctx
# fail-loud from ML_MODEL_MEMORY_PROFILES_JSON. Seed the finite unit-test model
# inventory only when the repo CLI did not provide generated SST config. Tests
# for missing/malformed/duplicate/unprofiled data replace or delete this value
# explicitly, so the production no-default contract keeps adversarial coverage.
os.environ.setdefault(
    "ML_MODEL_MEMORY_PROFILES_JSON",
    "["
    '{"model":"m","num_ctx":4096},'
    '{"model":"test-model","num_ctx":4096},'
    '{"model":"gemma","num_ctx":4096},'
    '{"model":"llama3","num_ctx":4096},'
    '{"model":"llama3.2","num_ctx":4096},'
    '{"model":"gemma3:4b","num_ctx":8192},'
    '{"model":"gemma4:26b","num_ctx":8192},'
    '{"model":"llava","num_ctx":4096},'
    '{"model":"qwen2.5:0.5b-instruct","num_ctx":4096},'
    '{"model":"qwen3:30b-a3b","num_ctx":32768},'
    '{"model":"some-other-model:7b","num_ctx":4096},'
    '{"model":"deepseek-ocr:3b","num_ctx":4096}'
    "]",
)

# F2 (redteam LLM-enrichment cold-load) — the ML sidecar's ollama completions
# read ML_OLLAMA_KEEP_ALIVE fail-loud at CALL time (ml/app/ollama_keepalive.py).
# Several unit tests drive the ollama code path (test_processor / test_domain /
# test_synthesis), so seed a value here IFF unset — the same developer-ergonomic
# setdefault pattern used for SMACKEREL_AUTH_TOKEN above, and NOT a default in
# the production module. The fail-loud contract itself is proven adversarially
# in test_ollama_keepalive.py via monkeypatch.delenv.
os.environ.setdefault("ML_OLLAMA_KEEP_ALIVE", "30m")

# BUG-026-007 (redteam F2, latency half) — the ML sidecar's structured-JSON
# extraction completions read ML_STRUCTURED_EXTRACTION_THINKING fail-loud at CALL
# time (ml/app/ollama_thinking.py) on the ollama path. The same unit tests that
# drive the ollama code path (test_processor / test_domain / test_synthesis /
# test_card_categories / test_drive_classify) would otherwise raise, so seed the
# fix's default posture (thinking DISABLED) here IFF unset — the same
# developer-ergonomic setdefault pattern as above, and NOT a default in the
# production module. The fail-loud contract itself is proven adversarially in
# test_ollama_thinking.py via monkeypatch.delenv.
os.environ.setdefault("ML_STRUCTURED_EXTRACTION_THINKING", "false")

# Spec 102 SCOPE-102-03 (BUG-026-006) — the ML sidecar's structured-JSON domain/
# synthesis extraction completions read ML_DOMAIN_OUTPUT_TOKEN_BUDGET fail-loud
# at CALL time (ml/app/ollama_keepalive.py::resolve_domain_output_token_budget),
# used regardless of provider. The unit tests that drive process_content /
# handle_extract (test_processor / test_synthesis / test_ollama_keepalive) would
# otherwise raise, so seed the SST default here IFF unset — the same
# developer-ergonomic setdefault pattern as above, NOT a default in the
# production module. The fail-loud contract is proven adversarially in
# test_ollama_keepalive.py via monkeypatch.delenv.
os.environ.setdefault("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", "4096")
