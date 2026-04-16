"""Tests for the knowledge synthesis consumer (ml/app/synthesis.py)."""

import json
import os
import tempfile

import pytest

from app.synthesis import (
    build_synthesis_prompt,
    enforce_validation_rules,
    load_prompt_contract,
    truncate_content,
    validate_extraction,
)

# --- Fixtures ---

SAMPLE_CONTRACT = {
    "version": "ingest-synthesis-v1",
    "type": "ingest-synthesis",
    "description": "Test contract",
    "system_prompt": "You are a knowledge synthesis engine.",
    "extraction_schema": {
        "type": "object",
        "required": ["concepts", "entities", "relationships"],
        "properties": {
            "concepts": {
                "type": "array",
                "items": {
                    "type": "object",
                    "required": ["name", "description", "claims"],
                    "properties": {
                        "name": {"type": "string", "maxLength": 100},
                        "description": {"type": "string"},
                        "claims": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "required": ["text"],
                                "properties": {
                                    "text": {"type": "string"},
                                    "confidence": {"type": "number"},
                                },
                            },
                        },
                        "is_new": {"type": "boolean"},
                    },
                },
            },
            "entities": {
                "type": "array",
                "items": {
                    "type": "object",
                    "required": ["name", "type", "context"],
                    "properties": {
                        "name": {"type": "string"},
                        "type": {"type": "string", "enum": ["person", "organization", "place"]},
                        "context": {"type": "string"},
                    },
                },
            },
            "relationships": {
                "type": "array",
                "items": {
                    "type": "object",
                    "required": ["source", "target", "type", "description"],
                    "properties": {
                        "source": {"type": "string"},
                        "target": {"type": "string"},
                        "type": {
                            "type": "string",
                            "enum": [
                                "CONCEPT_RELATES_TO",
                                "ENTITY_RELATES_TO_CONCEPT",
                                "SUPPORTS",
                                "CONTRADICTS",
                            ],
                        },
                        "description": {"type": "string"},
                    },
                },
            },
        },
    },
    "validation_rules": {
        "max_concepts": 10,
        "max_entities": 20,
        "max_relationships": 30,
        "max_contradictions": 5,
    },
    "token_budget": 2000,
    "temperature": 0.3,
}

VALID_EXTRACTION = {
    "concepts": [
        {
            "name": "Leadership",
            "description": "Organizational management",
            "claims": [{"text": "Servant leadership increases retention", "confidence": 0.85}],
            "is_new": True,
        }
    ],
    "entities": [{"name": "Sarah", "type": "person", "context": "Leadership consultant"}],
    "relationships": [
        {
            "source": "Leadership",
            "target": "Remote Work",
            "type": "CONCEPT_RELATES_TO",
            "description": "Both address team management",
        }
    ],
}


# --- T2-07: validate_extraction valid output → True ---


def test_validate_extraction_valid():
    """Valid extraction output passes schema validation."""
    schema = SAMPLE_CONTRACT["extraction_schema"]
    valid, error_msg = validate_extraction(VALID_EXTRACTION, schema)
    assert valid is True
    assert error_msg == ""


# --- T2-08: validate_extraction missing required field → False with error ---


def test_validate_extraction_missing_required_concepts():
    """Output missing required 'concepts' field fails validation."""
    schema = SAMPLE_CONTRACT["extraction_schema"]
    invalid_output = {
        "entities": [],
        "relationships": [],
    }
    valid, error_msg = validate_extraction(invalid_output, schema)
    assert valid is False
    assert "concepts" in error_msg.lower()


def test_validate_extraction_invalid_entity_type():
    """Entity with invalid type enum fails validation."""
    schema = SAMPLE_CONTRACT["extraction_schema"]
    invalid_output = {
        "concepts": [],
        "entities": [{"name": "Test", "type": "INVALID_TYPE", "context": "test"}],
        "relationships": [],
    }
    valid, error_msg = validate_extraction(invalid_output, schema)
    assert valid is False
    assert "INVALID_TYPE" in error_msg or "enum" in error_msg.lower()


def test_validate_extraction_missing_entity_name():
    """Entity missing required 'name' field fails validation."""
    schema = SAMPLE_CONTRACT["extraction_schema"]
    invalid_output = {
        "concepts": [],
        "entities": [{"type": "person", "context": "test"}],
        "relationships": [],
    }
    valid, error_msg = validate_extraction(invalid_output, schema)
    assert valid is False
    assert "name" in error_msg.lower()


def test_validate_extraction_invalid_relationship_type():
    """Relationship with invalid type enum fails validation."""
    schema = SAMPLE_CONTRACT["extraction_schema"]
    invalid_output = {
        "concepts": [],
        "entities": [],
        "relationships": [{"source": "A", "target": "B", "type": "INVALID", "description": "test"}],
    }
    valid, error_msg = validate_extraction(invalid_output, schema)
    assert valid is False


# --- T2-09: SynthesisConsumer builds prompt with existing concepts context ---


def test_build_synthesis_prompt_includes_system_prompt():
    """Prompt includes system_prompt from contract."""
    prompt = build_synthesis_prompt(
        contract=SAMPLE_CONTRACT,
        artifact_data={"title": "Test", "content_raw": "Some text"},
        existing_concepts=[],
        existing_entities=[],
    )
    assert "knowledge synthesis engine" in prompt.lower()


def test_build_synthesis_prompt_includes_existing_concepts():
    """Prompt includes existing concept pages as context."""
    existing = [
        {"title": "Leadership", "summary": "Management approach focused on influence"},
        {"title": "Remote Work", "summary": "Working outside traditional office"},
    ]
    prompt = build_synthesis_prompt(
        contract=SAMPLE_CONTRACT,
        artifact_data={"title": "Test Article", "content_raw": "Some content"},
        existing_concepts=existing,
        existing_entities=[],
    )
    assert "EXISTING CONCEPT PAGES" in prompt
    assert "Leadership" in prompt
    assert "Remote Work" in prompt


def test_build_synthesis_prompt_includes_existing_entities():
    """Prompt includes existing entity profiles as context."""
    existing = [{"name": "Sarah Chen", "type": "person"}]
    prompt = build_synthesis_prompt(
        contract=SAMPLE_CONTRACT,
        artifact_data={"title": "Test", "content_raw": "Content"},
        existing_concepts=[],
        existing_entities=existing,
    )
    assert "EXISTING ENTITY PROFILES" in prompt
    assert "Sarah Chen" in prompt


def test_build_synthesis_prompt_includes_artifact_content():
    """Prompt includes the artifact title, type, and content."""
    prompt = build_synthesis_prompt(
        contract=SAMPLE_CONTRACT,
        artifact_data={
            "title": "Modern Leadership Strategies",
            "content_type": "article",
            "source_id": "rss",
            "summary": "About servant leadership",
            "content_raw": "The full article about leadership...",
            "key_ideas": ["servant leadership", "team empowerment"],
            "topics": ["management", "leadership"],
        },
        existing_concepts=[],
        existing_entities=[],
    )
    assert "Modern Leadership Strategies" in prompt
    assert "article" in prompt
    assert "rss" in prompt
    assert "servant leadership" in prompt
    assert "management" in prompt


# --- Prompt contract loading ---


def test_load_prompt_contract_from_file():
    """Load a prompt contract from a temporary YAML file."""
    import yaml

    with tempfile.TemporaryDirectory() as tmpdir:
        contract_path = os.path.join(tmpdir, "test-v1.yaml")
        with open(contract_path, "w") as f:
            yaml.dump(SAMPLE_CONTRACT, f)

        os.environ["PROMPT_CONTRACTS_DIR"] = tmpdir
        try:
            contract = load_prompt_contract("test-v1")
            assert contract["version"] == "ingest-synthesis-v1"
            assert "system_prompt" in contract
        finally:
            del os.environ["PROMPT_CONTRACTS_DIR"]


def test_load_prompt_contract_not_found():
    """Missing contract file raises FileNotFoundError."""
    os.environ["PROMPT_CONTRACTS_DIR"] = "/nonexistent/path"
    try:
        with pytest.raises(FileNotFoundError):
            load_prompt_contract("nonexistent-v1")
    finally:
        del os.environ["PROMPT_CONTRACTS_DIR"]


# --- Content truncation ---


def test_truncate_content_short():
    """Short content is not truncated."""
    text = "Short text"
    assert truncate_content(text, 100) == text


def test_truncate_content_long():
    """Long content is truncated at word boundary."""
    text = "word " * 2000  # ~10000 chars
    result = truncate_content(text, 8000)
    assert len(result) <= 8000


# --- Validation rules enforcement ---


def test_enforce_validation_rules_trims_excess():
    """Validation rules trim arrays to max counts."""
    output = {
        "concepts": [{"name": f"Concept {i}"} for i in range(15)],
        "entities": [{"name": f"Entity {i}"} for i in range(25)],
        "relationships": [{"source": f"A{i}", "target": f"B{i}"} for i in range(35)],
        "contradictions": [{"concept": f"C{i}"} for i in range(10)],
    }
    rules = {
        "max_concepts": 10,
        "max_entities": 20,
        "max_relationships": 30,
        "max_contradictions": 5,
    }

    result = enforce_validation_rules(output, rules)
    assert len(result["concepts"]) == 10
    assert len(result["entities"]) == 20
    assert len(result["relationships"]) == 30
    assert len(result["contradictions"]) == 5


def test_enforce_validation_rules_no_trim_needed():
    """Validation rules don't trim when within limits."""
    output = {
        "concepts": [{"name": "A"}],
        "entities": [{"name": "B"}],
        "relationships": [],
        "contradictions": [],
    }
    rules = {"max_concepts": 10, "max_entities": 20, "max_relationships": 30, "max_contradictions": 5}
    result = enforce_validation_rules(output, rules)
    assert len(result["concepts"]) == 1
    assert len(result["entities"]) == 1


# --- T4-04: handle_crosssource genuine → correct response shape ---


def test_handle_crosssource_genuine_connection():
    """handle_crosssource returns has_genuine_connection=true with insight for genuine connections.

    Uses a mock LLM response to verify the response shape without a live model.
    """
    import asyncio
    from unittest.mock import AsyncMock, MagicMock, patch

    from app.synthesis import handle_crosssource

    mock_response = MagicMock()
    crosssource_json = json.dumps(
        {
            "has_genuine_connection": True,
            "insight_text": "Restaurant recommendation from email was later visited per Maps timeline",
            "confidence": 0.85,
        }
    )
    mock_response.choices = [MagicMock(message=MagicMock(content=crosssource_json))]
    mock_response.model = "ollama/llama3.2"

    # Write a temp cross-source contract
    import tempfile

    import yaml

    contract = {
        "version": "cross-source-connection-v1",
        "type": "cross-source-connection",
        "description": "Test cross-source contract",
        "system_prompt": "You are a cross-source connection assessor.",
        "extraction_schema": {
            "type": "object",
            "required": ["has_genuine_connection", "confidence"],
            "properties": {
                "has_genuine_connection": {"type": "boolean"},
                "insight_text": {"type": "string"},
                "confidence": {"type": "number", "minimum": 0, "maximum": 1},
            },
        },
        "validation_rules": {},
        "token_budget": 500,
        "temperature": 0.2,
    }

    with tempfile.TemporaryDirectory() as tmpdir:
        path = os.path.join(tmpdir, "cross-source-connection-v1.yaml")
        with open(path, "w") as f:
            yaml.dump(contract, f)

        os.environ["PROMPT_CONTRACTS_DIR"] = tmpdir
        try:
            import sys

            mock_litellm = MagicMock()
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)
            with patch.dict(sys.modules, {"litellm": mock_litellm}):
                result = asyncio.run(
                    handle_crosssource(
                        data={
                            "concept_id": "01JCONCEPT01",
                            "concept_title": "Italian Restaurants",
                            "artifacts": [
                                {
                                    "id": "01JART001",
                                    "title": "Email: Restaurant recommendation",
                                    "source_type": "email",
                                    "summary": "Great Italian place downtown",
                                },
                                {
                                    "id": "01JART002",
                                    "title": "Maps: Trattoria Roma",
                                    "source_type": "google-maps-timeline",
                                    "summary": "Visited Italian restaurant",
                                },
                            ],
                            "prompt_contract_version": "cross-source-connection-v1",
                        },
                        provider="ollama",
                        model="llama3.2",
                        api_key="",
                        ollama_url="http://localhost:11434",
                    )
                )
        finally:
            del os.environ["PROMPT_CONTRACTS_DIR"]

    assert result["concept_id"] == "01JCONCEPT01"
    assert result["has_genuine_connection"] is True
    assert result["confidence"] == 0.85
    assert "restaurant" in result["insight_text"].lower() or "recommendation" in result["insight_text"].lower()
    assert len(result["artifact_ids"]) == 2
    assert "01JART001" in result["artifact_ids"]
    assert "01JART002" in result["artifact_ids"]
    assert result["prompt_contract_version"] == "cross-source-connection-v1"
    assert result["processing_time_ms"] >= 0


# --- T4-05: handle_crosssource surface-level → has_genuine_connection=false ---


def test_handle_crosssource_surface_level():
    """handle_crosssource returns has_genuine_connection=false for surface overlap.

    Uses a mock LLM response to verify correct handling of negative assessments.
    """
    import asyncio
    from unittest.mock import AsyncMock, MagicMock, patch

    from app.synthesis import handle_crosssource

    mock_response = MagicMock()
    mock_response.choices = [
        MagicMock(
            message=MagicMock(content='{"has_genuine_connection": false, "insight_text": "", "confidence": 0.25}')
        )
    ]
    mock_response.model = "ollama/llama3.2"

    import tempfile

    import yaml

    contract = {
        "version": "cross-source-connection-v1",
        "type": "cross-source-connection",
        "description": "Test contract",
        "system_prompt": "Assess connections.",
        "extraction_schema": {
            "type": "object",
            "required": ["has_genuine_connection", "confidence"],
            "properties": {
                "has_genuine_connection": {"type": "boolean"},
                "insight_text": {"type": "string"},
                "confidence": {"type": "number"},
            },
        },
        "validation_rules": {},
        "token_budget": 500,
        "temperature": 0.2,
    }

    with tempfile.TemporaryDirectory() as tmpdir:
        path = os.path.join(tmpdir, "cross-source-connection-v1.yaml")
        with open(path, "w") as f:
            yaml.dump(contract, f)

        os.environ["PROMPT_CONTRACTS_DIR"] = tmpdir
        try:
            import sys

            mock_litellm = MagicMock()
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)
            with patch.dict(sys.modules, {"litellm": mock_litellm}):
                result = asyncio.run(
                    handle_crosssource(
                        data={
                            "concept_id": "01JCONCEPT02",
                            "concept_title": "Food",
                            "artifacts": [
                                {
                                    "id": "01JART003",
                                    "title": "Email mentioning food",
                                    "source_type": "email",
                                    "summary": "Lunch discussion",
                                },
                                {
                                    "id": "01JART004",
                                    "title": "Maps food court visit",
                                    "source_type": "google-maps-timeline",
                                    "summary": "Went to food court",
                                },
                            ],
                            "prompt_contract_version": "cross-source-connection-v1",
                        },
                        provider="ollama",
                        model="llama3.2",
                        api_key="",
                        ollama_url="http://localhost:11434",
                    )
                )
        finally:
            del os.environ["PROMPT_CONTRACTS_DIR"]

    assert result["concept_id"] == "01JCONCEPT02"
    assert result["has_genuine_connection"] is False
    assert result["confidence"] == 0.25
    assert result["insight_text"] == ""
    assert len(result["artifact_ids"]) == 2
