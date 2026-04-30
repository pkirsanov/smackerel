"""Scope 038 drive classification contract tests."""

import asyncio
import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

from app.drive_classify import classify_drive_file, validate_drive_classification_result


def _response(payload: dict) -> MagicMock:
    message = MagicMock()
    message.content = json.dumps(payload)
    choice = MagicMock()
    choice.message = message
    usage = MagicMock()
    usage.total_tokens = 57
    response = MagicMock()
    response.choices = [choice]
    response.usage = usage
    return response


def test_drive_classification_contract_requires_evidence_confidence_and_sensitivity():
    invalid_payloads = [
        {"classification": "recipe", "confidence": 0.9, "sensitivity": "none"},
        {"classification": "recipe", "evidence": ["ingredients"], "sensitivity": "none"},
        {"classification": "recipe", "confidence": 0.9, "evidence": ["ingredients"]},
    ]

    for payload in invalid_payloads:
        with pytest.raises(ValueError):
            validate_drive_classification_result(payload)


def test_drive_classification_contract_rejects_low_information_evidence():
    with pytest.raises(ValueError):
        validate_drive_classification_result(
            {
                "classification": "recipe",
                "topic": "Dinner",
                "audience": "personal",
                "sensitivity": "none",
                "confidence": 0.95,
                "evidence": ["file name"],
                "domain_routes": ["recipes"],
            }
        )


def test_classify_drive_file_returns_provider_neutral_metadata_with_evidence():
    llm_payload = {
        "classification": "recipe",
        "topic": "Dinner planning",
        "audience": "household",
        "sensitivity": "none",
        "confidence": 0.91,
        "evidence": ["ingredients include chickpeas", "folder context is Meal Plans"],
        "domain_routes": ["recipes", "meal_plan", "lists", "digest"],
        "action_items": ["Buy chickpeas"],
        "summary": "Dinner plan with chickpeas and parsley.",
    }

    with patch("app.drive_classify.litellm") as mock_litellm:
        mock_litellm.acompletion = AsyncMock(return_value=_response(llm_payload))
        result = asyncio.run(
            classify_drive_file(
                {
                    "artifact_id": "artifact-recipe",
                    "title": "dinner-plan.txt",
                    "mime_type": "text/plain",
                    "folder_path": "Meal Plans/April",
                    "extracted_text": "Chickpeas, parsley, lemon, tahini. Buy chickpeas.",
                    "folder_summary": {"topic": "Meal Plans", "audience": "household"},
                },
                provider="ollama",
                model="llama3",
                api_key="",
            )
        )

    assert result["success"] is True
    assert result["classification"] == "recipe"
    assert result["topic"] == "Dinner planning"
    assert result["sensitivity"] == "none"
    assert result["confidence"] == pytest.approx(0.91)
    assert "recipes" in result["domain_routes"]
    assert "provider_id" not in result
    assert result["evidence"] == llm_payload["evidence"]
