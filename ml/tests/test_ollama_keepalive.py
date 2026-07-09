"""F2 (redteam LLM-enrichment cold-load) — Ollama keep_alive wiring tests.

Root cause: the domain/synthesis model unloads between sparse captures (Ollama's
default keep_alive = 5m), so each capture pays a 22-45s cold-load that blows the
30s domain-extraction budget → truncated / invalid JSON. The fix keeps the model
resident by passing ``keep_alive`` on the ml sidecar's Ollama completions.

These are UNIT tests. They prove the smackerel-side contract — that each Ollama
completion is composed with (a) the ``ollama_chat/`` (/api/chat) prefix and
(b) the SST-owned ``keep_alive`` window at the request TOP LEVEL — not the live
prod latency (which only the orchestrator's redeploy can confirm). ``keep_alive``
is honored by Ollama ONLY at the top level of the request body, and litellm
forwards it there for ``ollama_chat/`` but buries it under ``options`` for the
legacy ``ollama/`` (/api/generate) transform (verified vs litellm 1.59.8 +
1.84.0) — so each capture test asserts, ADVERSARIALLY, that the legacy
generate prefix is NOT used.
"""

import asyncio
import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import yaml

# The [dev] unit lane deliberately does NOT install the heavy real litellm, so
# app.domain / app.processor (which `import litellm` at module top) can only be
# imported once a stand-in module exists. Mirror the guard the sibling LLM test
# modules use so this file is self-sufficient when run in isolation, not merely
# when a sibling injected the stub first (collection-order independence).
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

# --------------------------------------------------------------------------
# SST fail-loud resolver (ml/app/ollama_keepalive.py)
# --------------------------------------------------------------------------


def test_resolve_returns_configured_window(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "45m")
    from app.ollama_keepalive import resolve_ollama_keep_alive

    assert resolve_ollama_keep_alive() == "45m"


def test_resolve_fail_loud_when_unset(monkeypatch):
    # ADVERSARIAL — no default; a missing window must raise, never silently
    # substitute a fallback (smackerel NO-DEFAULTS / Gate G028). Fails if a
    # default is ever added to resolve_ollama_keep_alive().
    monkeypatch.delenv("ML_OLLAMA_KEEP_ALIVE", raising=False)
    from app.ollama_keepalive import resolve_ollama_keep_alive

    with pytest.raises(RuntimeError):
        resolve_ollama_keep_alive()


def test_resolve_fail_loud_when_blank(monkeypatch):
    # A whitespace-only value is as good as unset — must still fail loud.
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "   ")
    from app.ollama_keepalive import resolve_ollama_keep_alive

    with pytest.raises(RuntimeError):
        resolve_ollama_keep_alive()


# --------------------------------------------------------------------------
# domain.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_domain_extract_passes_keepalive_via_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "45m")
    from app.domain import handle_domain_extract

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [
            MagicMock(message=MagicMock(content=json.dumps({"domain": "recipe", "ingredients": [], "steps": []})))
        ]
        resp.usage = MagicMock(total_tokens=12)
        return resp

    with patch("app.domain.litellm.acompletion", new_callable=AsyncMock) as mock_comp:
        mock_comp.side_effect = _capture
        data = {
            "artifact_id": "art-1",
            "contract_version": "recipe-extraction-v1",
            "content_type": "recipe",
            "content_raw": "Ingredients: flour. Instructions: bake.",
        }
        result = asyncio.run(handle_domain_extract(data, "ollama", "gemma4:26b", "", "http://ollama:11434"))

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "45m"
    assert captured["api_base"] == "http://ollama:11434"
    # ADVERSARIAL: the legacy ollama/ generate prefix (where litellm buries
    # keep_alive under `options`, making it a silent no-op) must NOT be used.
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# processor.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_process_content_passes_keepalive_via_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    from app.processor import process_content

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content=json.dumps({"artifact_type": "article", "title": "T"})))]
        resp.usage = MagicMock(total_tokens=20)
        return resp

    with patch("app.processor.litellm") as mock_litellm:
        mock_litellm.acompletion = AsyncMock(side_effect=_capture)
        result = asyncio.run(
            process_content(
                content="hello world",
                content_type="article",
                source_id="s1",
                processing_tier="standard",
                user_context="",
                model="gemma4:26b",
                api_key="",
                provider="ollama",
            )
        )

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# synthesis.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_synthesis_extract_passes_keepalive_via_ollama_chat(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    contract = {
        "version": "ingest-synthesis-v1",
        "type": "ingest-synthesis",
        "system_prompt": "You are a knowledge synthesis engine.",
        "extraction_schema": {"type": "object"},
        "validation_rules": {},
        "token_budget": 500,
        "temperature": 0.3,
    }
    (tmp_path / "ingest-synthesis-v1.yaml").write_text(yaml.dump(contract))
    monkeypatch.setenv("PROMPT_CONTRACTS_DIR", str(tmp_path))
    from app.synthesis import handle_extract

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content="{}"))]
        resp.usage = MagicMock(total_tokens=5)
        resp.model = "ollama_chat/gemma4:26b"
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        result = asyncio.run(
            handle_extract(
                {"artifact_id": "a1", "prompt_contract_version": "ingest-synthesis-v1", "content_raw": "hello"},
                "ollama",
                "gemma4:26b",
                "",
                "http://ollama:11434",
            )
        )

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# main.py — startup warmup (best-effort, non-fatal)
# --------------------------------------------------------------------------


def test_warmup_skipped_for_non_ollama_provider():
    # No litellm call at all for hosted providers — returns immediately.
    from app.main import _warmup_domain_model

    asyncio.run(_warmup_domain_model({"LLM_PROVIDER": "openai", "LLM_MODEL": "gpt-4o", "OLLAMA_URL": ""}))


def test_warmup_uses_ollama_chat_and_keepalive(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    from app.main import _warmup_domain_model
    import litellm

    captured: dict = {}

    async def _capture(**kwargs):
        captured.update(kwargs)
        return MagicMock()

    monkeypatch.setattr(litellm, "acompletion", _capture, raising=False)
    asyncio.run(
        _warmup_domain_model({"LLM_PROVIDER": "ollama", "LLM_MODEL": "gemma4:26b", "OLLAMA_URL": "http://ollama:11434"})
    )

    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


def test_warmup_is_non_fatal_when_ollama_unreachable(monkeypatch):
    # ADVERSARIAL — a warmup failure at boot (model not pulled, Ollama down)
    # MUST be swallowed; it must NEVER propagate and block sidecar startup.
    # Fails if the try/except around the warmup completion is removed.
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    from app.main import _warmup_domain_model
    import litellm

    async def _boom(**kwargs):
        raise ConnectionError("connection refused")

    monkeypatch.setattr(litellm, "acompletion", _boom, raising=False)
    # Absence of a raised exception here IS the assertion.
    asyncio.run(
        _warmup_domain_model({"LLM_PROVIDER": "ollama", "LLM_MODEL": "gemma4:26b", "OLLAMA_URL": "http://ollama:11434"})
    )
