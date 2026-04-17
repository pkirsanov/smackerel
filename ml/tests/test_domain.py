"""Tests for the domain extraction handler."""

import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

# Ensure litellm mock is in place before importing app code
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

import asyncio

from app.domain import (  # isort: skip
    _build_system_prompt,
    _build_user_prompt,
    _domain_from_contract,
    _resolve_model,
    handle_domain_extract,
)


class TestDomainFromContract:
    def test_recipe_contract(self):
        assert _domain_from_contract("recipe-extraction-v1") == "recipe"

    def test_product_contract(self):
        assert _domain_from_contract("product-extraction-v1") == "product"

    def test_unknown(self):
        assert _domain_from_contract("unknown") == "unknown"

    def test_empty(self):
        assert _domain_from_contract("") == ""


class TestBuildSystemPrompt:
    def test_recipe_prompt(self):
        prompt = _build_system_prompt("recipe-extraction-v1", "recipe")
        assert "recipe extraction engine" in prompt.lower()
        assert "ingredients" in prompt.lower()

    def test_product_prompt(self):
        prompt = _build_system_prompt("product-extraction-v1", "product")
        assert "product extraction engine" in prompt.lower()
        assert "specs" in prompt.lower() or "specification" in prompt.lower()

    def test_unknown_domain(self):
        prompt = _build_system_prompt("travel-extraction-v1", "article")
        assert "travel" in prompt.lower()


class TestBuildUserPrompt:
    def test_with_all_fields(self):
        prompt = _build_user_prompt("My Recipe", "A great dish", "Ingredients: eggs, flour")
        assert "My Recipe" in prompt
        assert "A great dish" in prompt
        assert "eggs, flour" in prompt

    def test_with_content_only(self):
        prompt = _build_user_prompt("", "", "Just some content")
        assert "Just some content" in prompt


class TestHandleDomainExtract:
    def test_no_content_returns_failure(self):
        data = {
            "artifact_id": "art-001",
            "contract_version": "recipe-extraction-v1",
            "content_type": "recipe",
        }
        result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))
        assert result["success"] is False
        assert "no content" in result["error"]
        assert result["artifact_id"] == "art-001"

    def test_successful_extraction(self):
        recipe_json = json.dumps(
            {
                "domain": "recipe",
                "ingredients": [{"name": "eggs", "quantity": "4", "unit": ""}],
                "steps": [{"number": 1, "instruction": "Beat eggs"}],
            }
        )

        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.content = recipe_json
        mock_response.usage = MagicMock()
        mock_response.usage.total_tokens = 500

        with patch("app.domain.litellm.acompletion", new_callable=AsyncMock, return_value=mock_response):
            data = {
                "artifact_id": "art-001",
                "contract_version": "recipe-extraction-v1",
                "content_type": "recipe",
                "title": "Scrambled Eggs",
                "content_raw": "Beat 4 eggs and cook in a pan.",
            }
            result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))

        assert result["success"] is True
        assert result["domain_data"]["domain"] == "recipe"
        assert len(result["domain_data"]["ingredients"]) == 1
        assert result["tokens_used"] == 500

    def test_llm_returns_invalid_json_retries(self):
        mock_response_bad = MagicMock()
        mock_response_bad.choices = [MagicMock()]
        mock_response_bad.choices[0].message.content = "not json"
        mock_response_bad.usage = MagicMock()
        mock_response_bad.usage.total_tokens = 100

        mock_response_good = MagicMock()
        mock_response_good.choices = [MagicMock()]
        mock_response_good.choices[0].message.content = json.dumps(
            {
                "domain": "recipe",
                "ingredients": [],
                "steps": [],
            }
        )
        mock_response_good.usage = MagicMock()
        mock_response_good.usage.total_tokens = 200

        call_count = 0

        async def mock_acompletion(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                return mock_response_bad
            return mock_response_good

        with patch("app.domain.litellm.acompletion", side_effect=mock_acompletion):
            with patch("asyncio.sleep", new_callable=AsyncMock):
                data = {
                    "artifact_id": "art-002",
                    "contract_version": "recipe-extraction-v1",
                    "content_type": "recipe",
                    "content_raw": "Some recipe content",
                }
                result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))

        assert result["success"] is True
        assert call_count == 2

    def test_all_retries_exhausted(self):
        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.content = "not json"
        mock_response.usage = MagicMock()
        mock_response.usage.total_tokens = 50

        with patch("app.domain.litellm.acompletion", new_callable=AsyncMock, return_value=mock_response):
            with patch("asyncio.sleep", new_callable=AsyncMock):
                data = {
                    "artifact_id": "art-003",
                    "contract_version": "recipe-extraction-v1",
                    "content_type": "recipe",
                    "content_raw": "Some content",
                }
                result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))

        assert result["success"] is False
        assert "JSON parse error" in result["error"]

    def test_domain_auto_injected_when_missing(self):
        response_without_domain = json.dumps(
            {
                "ingredients": [{"name": "flour"}],
                "steps": [],
            }
        )

        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.content = response_without_domain
        mock_response.usage = MagicMock()
        mock_response.usage.total_tokens = 100

        with patch("app.domain.litellm.acompletion", new_callable=AsyncMock, return_value=mock_response):
            data = {
                "artifact_id": "art-004",
                "contract_version": "recipe-extraction-v1",
                "content_type": "recipe",
                "content_raw": "Mix flour and water",
            }
            result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))

        assert result["success"] is True
        assert result["domain_data"]["domain"] == "recipe"

    def test_product_extraction(self):
        product_json = json.dumps(
            {
                "domain": "product",
                "product_name": "Sony WH-1000XM5",
                "brand": "Sony",
                "price": {"amount": 349.99, "currency": "USD"},
                "specs": [{"name": "Weight", "value": "250g"}],
                "pros": ["Great ANC"],
                "cons": ["Expensive"],
            }
        )

        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.content = product_json
        mock_response.usage = MagicMock()
        mock_response.usage.total_tokens = 300

        with patch("app.domain.litellm.acompletion", new_callable=AsyncMock, return_value=mock_response):
            data = {
                "artifact_id": "art-005",
                "contract_version": "product-extraction-v1",
                "content_type": "product",
                "title": "Sony WH-1000XM5",
                "content_raw": "Sony WH-1000XM5 headphones, $349.99",
            }
            result = asyncio.run(handle_domain_extract(data, "openai", "gpt-4o", "key", ""))

        assert result["success"] is True
        assert result["domain_data"]["domain"] == "product"
        assert result["domain_data"]["brand"] == "Sony"
        assert result["domain_data"]["price"]["amount"] == 349.99


class TestResolveModel:
    def test_ollama_provider(self):
        assert _resolve_model("llama3", "ollama", "http://ollama:11434") == "ollama/llama3"

    def test_openai_provider(self):
        assert _resolve_model("gpt-4o", "openai", "") == "gpt-4o"

    def test_anthropic_provider(self):
        assert _resolve_model("claude-sonnet-4-20250514", "anthropic", "") == "anthropic/claude-sonnet-4-20250514"

    def test_other_provider(self):
        assert _resolve_model("gemini-pro", "google", "") == "google/gemini-pro"
