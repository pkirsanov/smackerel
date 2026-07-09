"""BUG-026-007 (redteam F2, latency half) — qwen3 thinking-disable wiring tests.

Root cause: qwen3:30b-a3b runs with thinking mode ON by default, adding a hidden
``<think>…</think>`` reasoning block (>150s live on evo-x2) before its JSON on the
structured-JSON extraction path — blowing the 30s DOMAIN_EXTRACTION_TIMEOUT and
prefixing callers with a ``<think>`` block that trips ``LLM returned invalid
JSON``. The fix sets the NATIVE Ollama ``think=False`` request field on the
extraction ``litellm.acompletion`` call when the SST switch
``ML_STRUCTURED_EXTRACTION_THINKING=false``, while leaving the agent reasoning
path unchanged.

These are UNIT tests. They prove the smackerel-side contract — that each in-scope
structured-JSON extraction call sends the native ``think=False`` kwarg to litellm
when SST disables thinking (and does NOT when SST enables it) — not the live prod
latency (which only the orchestrator's redeploy can confirm). The mechanism is
the native ``think`` field, NOT the ``/no_think`` prompt token the FIRST fix used
(qwen3's Ollama chat template ignores that string; measured live on evo-x2). A
top-level ``think=`` kwarg is forwarded to the Ollama request top level by
litellm 1.84.0 for BOTH the ``ollama_chat/`` and legacy ``ollama/`` transforms,
and the two formerly-legacy routes (search-rerank, drive-classify) are migrated
to the ``ollama_chat/`` prefix here for role fidelity + keep_alive parity.
"""

import asyncio
import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import yaml

# The [dev] unit lane deliberately does NOT install the heavy real litellm, so
# app.domain / app.processor / app.drive_classify (which `import litellm` at
# module top) can only be imported once a stand-in module exists. Mirror the
# guard the sibling LLM test modules use so this file is self-sufficient in
# isolation (collection-order independent). Include every exception name any
# in-scope handler imports.
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

# Enrich (do NOT guard on "not in sys.modules") the shared litellm.exceptions
# stand-in with EVERY exception name any in-scope handler imports. A sibling
# test module may have installed a litellm.exceptions mock first that only
# carries the domain/processor names — card_categories additionally does a lazy
# `from litellm.exceptions import APIConnectionError, APIError, Timeout`, so add
# any missing names without clobbering existing ones (collection-order safe).
_exc_mod = sys.modules.get("litellm.exceptions")
if _exc_mod is None:
    _exc_mod = types.ModuleType("litellm.exceptions")
    sys.modules["litellm.exceptions"] = _exc_mod
for _name in (
    "RateLimitError",
    "ServiceUnavailableError",
    "InternalServerError",
    "APIConnectionError",
    "APIError",
    "Timeout",
):
    if not hasattr(_exc_mod, _name):
        setattr(_exc_mod, _name, type(_name, (Exception,), {}))

from app.ollama_thinking import (  # noqa: E402  isort: skip
    apply_structured_extraction_thinking,
    resolve_structured_extraction_thinking,
)


def _think_disabled(kwargs: dict) -> bool:
    """True iff the completion kwargs carry the native ``think=False`` field
    (the mechanism qwen3's Ollama template actually honors)."""
    return kwargs.get("think") is False


# ==========================================================================
# resolver — ml/app/ollama_thinking.py::resolve_structured_extraction_thinking
# ==========================================================================


def test_resolve_returns_true_when_enabled(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "true")
    assert resolve_structured_extraction_thinking() is True


def test_resolve_returns_false_when_disabled(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    assert resolve_structured_extraction_thinking() is False


def test_resolve_is_case_insensitive(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "FALSE")
    assert resolve_structured_extraction_thinking() is False


def test_resolve_fail_loud_when_unset(monkeypatch):
    # ADVERSARIAL — no default; a missing switch must raise, never silently
    # substitute a fallback (smackerel NO-DEFAULTS / Gate G028). Fails if a
    # default is ever added to resolve_structured_extraction_thinking().
    monkeypatch.delenv("ML_STRUCTURED_EXTRACTION_THINKING", raising=False)
    with pytest.raises(RuntimeError):
        resolve_structured_extraction_thinking()


def test_resolve_fail_loud_when_blank(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "   ")
    with pytest.raises(RuntimeError):
        resolve_structured_extraction_thinking()


def test_resolve_fail_loud_when_invalid(monkeypatch):
    # ADVERSARIAL — any value other than true/false is a config error, not a
    # silent "assume thinking on/off".
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "maybe")
    with pytest.raises(RuntimeError):
        resolve_structured_extraction_thinking()


# ==========================================================================
# injector — apply_structured_extraction_thinking
# ==========================================================================


def test_apply_sets_native_think_false_when_disabled(monkeypatch):
    # ADVERSARIAL — the mechanism qwen3 actually honors is the native top-level
    # think=False kwarg (NOT a /no_think message token, which qwen3's Ollama
    # template ignores). Fails if the mutator stops setting think=False.
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    kwargs = {"model": "ollama_chat/qwen3:30b-a3b", "messages": [{"role": "user", "content": "x"}]}
    out = apply_structured_extraction_thinking(kwargs, "ollama")
    assert out is kwargs  # mutates in place and returns the same dict
    assert kwargs["think"] is False
    # It does NOT smuggle a /no_think token into the messages (the failed first fix).
    assert all("/no_think" not in (m.get("content") or "") for m in kwargs["messages"])


def test_apply_is_noop_when_thinking_enabled(monkeypatch):
    # ADVERSARIAL — with thinking ENABLED no think key is added; fails if the
    # disable is hard-wired on regardless of the SST value.
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "true")
    kwargs = {"model": "ollama_chat/qwen3:30b-a3b", "messages": [{"role": "user", "content": "x"}]}
    apply_structured_extraction_thinking(kwargs, "ollama")
    assert "think" not in kwargs


def test_apply_is_noop_for_non_ollama_provider(monkeypatch):
    # Hosted providers have no qwen thinking concept; the resolver must not even
    # be consulted (so a hosted deployment never needs the ollama-only switch),
    # and no think key is added.
    monkeypatch.delenv("ML_STRUCTURED_EXTRACTION_THINKING", raising=False)
    kwargs = {"model": "gpt-4", "messages": [{"role": "user", "content": "x"}]}
    apply_structured_extraction_thinking(kwargs, "openai")
    assert "think" not in kwargs


def test_apply_does_not_disturb_other_kwargs(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    kwargs = {
        "model": "ollama_chat/qwen3:30b-a3b",
        "messages": [{"role": "system", "content": "Extract JSON."}],
        "temperature": 0.1,
        "keep_alive": "30m",
    }
    apply_structured_extraction_thinking(kwargs, "ollama")
    # Only the think key is added; existing kwargs are untouched.
    assert kwargs["think"] is False
    assert kwargs["temperature"] == 0.1
    assert kwargs["keep_alive"] == "30m"
    assert kwargs["messages"][0]["content"] == "Extract JSON."


# ==========================================================================
# in-scope call sites — each MUST send native think=False when SST disables
# thinking (and NOT when SST enables it)
# ==========================================================================


def _domain_data() -> dict:
    return {
        "artifact_id": "art-1",
        "contract_version": "recipe-extraction-v1",
        "content_type": "recipe",
        "content_raw": "Ingredients: flour. Instructions: bake.",
    }


def test_domain_extract_disables_thinking_when_sst_false(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
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
        result = asyncio.run(handle_domain_extract(_domain_data(), "ollama", "qwen3:30b-a3b", "", "http://ollama:11434"))

    assert result["success"] is True
    # ADVERSARIAL: the proven 30s-budget path must disable thinking via the
    # native think=False kwarg. Fails if the mutator is reverted from domain.py.
    assert _think_disabled(captured), captured


def test_domain_extract_keeps_thinking_when_enabled(monkeypatch):
    # ADVERSARIAL — with SST=true the domain request must NOT carry think=False;
    # fails if the disable is hard-wired on.
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "true")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
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
        result = asyncio.run(handle_domain_extract(_domain_data(), "ollama", "qwen3:30b-a3b", "", "http://ollama:11434"))

    assert result["success"] is True
    assert "think" not in captured, captured


def test_process_content_disables_thinking_when_sst_false(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
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
                model="qwen3:30b-a3b",
                api_key="",
                provider="ollama",
            )
        )

    assert result["success"] is True
    assert _think_disabled(captured), captured


def _write_contract(tmp_path, monkeypatch) -> None:
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


def test_synthesis_extract_disables_thinking_when_sst_false(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    _write_contract(tmp_path, monkeypatch)
    from app.synthesis import handle_extract

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content="{}"))]
        resp.usage = MagicMock(total_tokens=5)
        resp.model = "ollama_chat/qwen3:30b-a3b"
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        result = asyncio.run(
            handle_extract(
                {"artifact_id": "a1", "prompt_contract_version": "ingest-synthesis-v1", "content_raw": "hello"},
                "ollama",
                "qwen3:30b-a3b",
                "",
                "http://ollama:11434",
            )
        )

    assert result["success"] is True
    assert _think_disabled(captured), captured


def test_synthesis_crosssource_disables_thinking_when_sst_false(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    _write_contract(tmp_path, monkeypatch)
    from app.synthesis import handle_crosssource

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [
            MagicMock(
                message=MagicMock(
                    content=json.dumps({"has_genuine_connection": True, "insight_text": "x", "confidence": 0.9})
                )
            )
        ]
        resp.usage = MagicMock(total_tokens=5)
        resp.model = "ollama_chat/qwen3:30b-a3b"
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        asyncio.run(
            handle_crosssource(
                {
                    "concept_id": "c1",
                    "concept_title": "T",
                    "prompt_contract_version": "ingest-synthesis-v1",
                    "artifacts": [{"source_type": "email", "title": "A", "summary": "s"}],
                },
                "ollama",
                "qwen3:30b-a3b",
                "",
                "http://ollama:11434",
            )
        )

    assert _think_disabled(captured), captured


def test_search_rerank_disables_thinking_and_uses_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    from app.nats_client import NATSClient

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [
            MagicMock(
                message=MagicMock(
                    content=json.dumps({"ranked": [{"index": 1, "relevance": "high", "explanation": "x"}]})
                )
            )
        ]
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    # _handle_search_rerank touches no self attributes; skip __init__ (which
    # would try to open a NATS connection) via __new__.
    client = NATSClient.__new__(NATSClient)
    data = {
        "query_id": "q1",
        "query": "chickpea recipes",
        "candidates": [{"id": "a1", "title": "Chickpea curry", "artifact_type": "recipe", "summary": "spicy"}],
    }
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        result = asyncio.run(client._handle_search_rerank(data, "ollama", "qwen3:30b-a3b", ""))

    assert "ranked" in result
    # ADVERSARIAL (route migration): this call was on the legacy ollama/
    # (/api/generate) transform, which buries keep_alive under `options`; it now
    # routes via ollama_chat/ so top-level params (keep_alive, think) reach
    # Ollama. Fails if the migration is reverted from _handle_search_rerank.
    assert captured["model"] == "ollama_chat/qwen3:30b-a3b", captured["model"]
    # ADVERSARIAL: and it disables qwen3 thinking via the native think=False
    # kwarg. Fails if the mutator is reverted.
    assert _think_disabled(captured), captured


def test_card_categories_disables_thinking_when_sst_false(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("LLM_MODEL", "qwen3:30b-a3b")
    from app.card_categories import ExtractCardCategoriesRequest, extract_card_categories

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [
            MagicMock(
                message=MagicMock(
                    content=json.dumps(
                        {
                            "card_id": "chase-freedom",
                            "period_label": "2026-Q3",
                            "period_start": "2026-07-01",
                            "period_end": "2026-09-30",
                            "categories": ["groceries"],
                            "spend_limit": 1500,
                            "activation_required": True,
                            "confidence": 0.9,
                            "source_evidence": "Q3 categories include groceries",
                        }
                    )
                )
            )
        ]
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    req = ExtractCardCategoriesRequest(
        card_id="chase-freedom", period_label="2026-Q3", page_text="Q3 categories include groceries"
    )
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        result = asyncio.run(extract_card_categories(req))

    assert result["card_id"] == "chase-freedom"
    assert captured["model"] == "ollama_chat/qwen3:30b-a3b", captured["model"]
    assert _think_disabled(captured), captured


def test_drive_classify_disables_thinking_and_uses_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    from app.drive_classify import classify_drive_file

    captured: dict = {}
    payload = {
        "classification": "recipe",
        "topic": "Dinner planning",
        "audience": "household",
        "sensitivity": "none",
        "confidence": 0.91,
        "evidence": ["ingredients include chickpeas", "folder context is Meal Plans"],
        "domain_routes": ["recipes", "meal_plan"],
        "action_items": ["Buy chickpeas"],
        "summary": "Dinner plan with chickpeas.",
    }

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content=json.dumps(payload)))]
        resp.usage = MagicMock(total_tokens=57)
        return resp

    with patch("app.drive_classify.litellm") as mock_litellm:
        mock_litellm.acompletion = AsyncMock(side_effect=_capture)
        result = asyncio.run(
            classify_drive_file(
                {
                    "artifact_id": "artifact-recipe",
                    "title": "dinner-plan.txt",
                    "mime_type": "text/plain",
                    "folder_path": "Meal Plans/April",
                    "extracted_text": "chickpeas, parsley",
                },
                provider="ollama",
                model="qwen3:30b-a3b",
                api_key="",
            )
        )

    assert result["success"] is True
    # ADVERSARIAL (route migration): drive-classify was on the legacy ollama/
    # (/api/generate) transform; it now routes via ollama_chat/. Fails if the
    # migration is reverted.
    assert captured["model"] == "ollama_chat/qwen3:30b-a3b", captured["model"]
    # ADVERSARIAL: and it disables qwen3 thinking via the native think=False
    # kwarg. Fails if the mutator is reverted.
    assert _think_disabled(captured), captured


# ==========================================================================
# scope boundary — the agent reasoning path is left thinking-ON
# ==========================================================================


def test_agent_path_keeps_thinking_even_when_disabled(monkeypatch):
    # ADVERSARIAL scope boundary — even with SST=false, agent.handle_invoke must
    # NOT disable thinking (the agent reasoning path keeps qwen3 thinking:
    # quality > latency). Fails if a future change wires the extraction
    # thinking-disable into the agent path.
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    monkeypatch.setenv("AGENT_PROVIDER_FAST_PROVIDER", "ollama")
    monkeypatch.setenv("AGENT_PROVIDER_FAST_MODEL", "qwen3:30b-a3b")
    from app.agent import handle_invoke

    captured: dict = {}

    async def _capture(**kwargs):
        captured.update(kwargs)
        return types.SimpleNamespace(
            choices=[types.SimpleNamespace(message=types.SimpleNamespace(tool_calls=None, content="ok"))],
            usage=types.SimpleNamespace(prompt_tokens=1, completion_tokens=1),
            model=kwargs.get("model", ""),
        )

    request = {
        "trace_id": "t1",
        "model_preference": "fast",
        "system_prompt": "You are a helpful assistant.",
        "turn_messages": [{"role": "user", "content": "hello"}],
    }
    env = asyncio.run(handle_invoke(request, completion_fn=_capture))

    assert "error" not in env, env
    assert captured.get("think") is not False, captured
