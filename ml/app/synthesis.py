"""Knowledge synthesis consumer for the ML sidecar.

Subscribes to `synthesis.extract` and `synthesis.crosssource` NATS subjects,
processes artifacts through LLM extraction using prompt contracts, validates
output against JSON Schema, and publishes results.
"""

import json
import logging
import os
import time

import yaml

from .receipt_detection import detect_receipt_content

logger = logging.getLogger("smackerel-ml.synthesis")

# Content char limit sent to LLM (design contract: 8000 chars)
MAX_CONTENT_CHARS = 8000


def _elapsed_ms(start: float) -> int:
    """Return elapsed time in milliseconds since start."""
    return int((time.time() - start) * 1000)


def load_prompt_contract(version: str) -> dict:
    """Load a prompt contract YAML file by version name.

    Requires PROMPT_CONTRACTS_DIR env var — fails loud if missing.
    Sanitizes version to prevent path traversal.
    """
    contracts_dir = os.environ["PROMPT_CONTRACTS_DIR"]
    if os.path.basename(version) != version or ".." in version:
        raise ValueError(f"Invalid prompt contract version: {version!r}")
    path = os.path.join(contracts_dir, f"{version}.yaml")
    if not os.path.isfile(path):
        raise FileNotFoundError(f"Prompt contract not found: {path}")
    with open(path) as f:
        return yaml.safe_load(f)


def validate_extraction(output: dict, schema: dict) -> tuple[bool, str]:
    """Validate LLM extraction output against the prompt contract's extraction_schema.

    Returns (valid, error_message). On success error_message is empty.
    Uses jsonschema for full JSON Schema validation.
    """
    import jsonschema

    try:
        jsonschema.validate(instance=output, schema=schema)
        return True, ""
    except jsonschema.ValidationError as e:
        return False, f"Schema validation failed: {e.message}"
    except jsonschema.SchemaError as e:
        return False, f"Invalid schema: {e.message}"


def truncate_content(text: str, max_chars: int = MAX_CONTENT_CHARS) -> str:
    """Truncate content to max_chars without splitting words."""
    if len(text) <= max_chars:
        return text
    truncated = text[:max_chars]
    last_space = truncated.rfind(" ")
    if last_space > max_chars * 0.8:
        return truncated[:last_space]
    return truncated


def build_synthesis_prompt(
    contract: dict,
    artifact_data: dict,
    existing_concepts: list,
    existing_entities: list,
) -> str:
    """Build the LLM prompt from the prompt contract and artifact data."""
    parts = []

    # System prompt from contract
    system_prompt = contract.get("system_prompt", "")
    if system_prompt:
        parts.append(system_prompt)

    # Existing knowledge context
    if existing_concepts:
        parts.append("\n--- EXISTING CONCEPT PAGES ---")
        for c in existing_concepts[:50]:
            parts.append(f"- {c.get('title', '')}: {c.get('summary', '')[:200]}")

    if existing_entities:
        parts.append("\n--- EXISTING ENTITY PROFILES ---")
        for e in existing_entities[:50]:
            parts.append(f"- {e.get('name', '')} ({e.get('type', '')})")

    # Artifact content
    parts.append("\n--- ARTIFACT TO ANALYZE ---")
    parts.append(f"Title: {artifact_data.get('title', 'Untitled')}")
    parts.append(f"Type: {artifact_data.get('content_type', 'unknown')}")
    parts.append(f"Source: {artifact_data.get('source_id', 'unknown')}")

    if artifact_data.get("summary"):
        parts.append(f"Summary: {artifact_data['summary']}")

    if artifact_data.get("key_ideas"):
        parts.append(f"Key Ideas: {', '.join(artifact_data['key_ideas'])}")

    if artifact_data.get("topics"):
        parts.append(f"Topics: {', '.join(artifact_data['topics'])}")

    content = artifact_data.get("content_raw", "")
    if content:
        content = truncate_content(content)
        parts.append(f"\nFull Content:\n{content}")

    # Output format instruction
    schema = contract.get("extraction_schema", {})
    parts.append(
        f"\n--- OUTPUT FORMAT ---\nReturn ONLY valid JSON matching this schema:\n{json.dumps(schema, indent=2)}"
    )

    return "\n".join(parts)


async def handle_extract(
    data: dict,
    provider: str,
    model: str,
    api_key: str,
    ollama_url: str,
) -> dict:
    """Handle a synthesis.extract request.

    Loads prompt contract, builds LLM prompt, calls LLM, validates output,
    and returns the extraction result.
    """
    artifact_id = data.get("artifact_id", "")
    contract_version = data.get("prompt_contract_version", "")

    # Pre-filter: check if content looks like a receipt (avoids expensive LLM
    # calls for receipt extraction on non-receipt content)
    content_raw = data.get("content_raw", "")
    content_type = data.get("content_type", "")
    source_id = data.get("source_id", "")
    is_receipt = detect_receipt_content(
        text=content_raw,
        content_type=content_type,
        source_id=source_id,
        sender=data.get("sender", ""),
        subject=data.get("subject", ""),
    )

    try:
        contract = load_prompt_contract(contract_version)
    except FileNotFoundError as e:
        logger.error("Prompt contract not found: %s", e)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": str(e),
            "prompt_contract_version": contract_version,
        }

    # Build the LLM prompt
    prompt = build_synthesis_prompt(
        contract=contract,
        artifact_data=data,
        existing_concepts=data.get("existing_concepts", []),
        existing_entities=data.get("existing_entities", []),
    )

    temperature = contract.get("temperature", 0.3)
    token_budget = contract.get("token_budget", 2000)

    start = time.time()

    # Call LLM via litellm (consistent with existing ML sidecar pattern)
    try:
        import litellm

        llm_model = model
        if provider == "ollama":
            llm_model = f"ollama/{model}"
            litellm.api_base = ollama_url

        response = await litellm.acompletion(
            model=llm_model,
            messages=[
                {"role": "system", "content": contract.get("system_prompt", "")},
                {"role": "user", "content": prompt},
            ],
            temperature=temperature,
            max_tokens=token_budget,
            api_key=api_key if provider != "ollama" else None,
            response_format={"type": "json_object"},
        )

        llm_output_text = response.choices[0].message.content
        tokens_used = response.usage.total_tokens if response.usage else 0
        model_used = response.model or llm_model

    except Exception as e:
        logger.error("LLM call failed for synthesis: %s", type(e).__name__)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"LLM call failed: {type(e).__name__}: {str(e)[:200]}",
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model or "",
            "tokens_used": 0,
        }

    # Parse LLM output as JSON
    try:
        llm_output = json.loads(llm_output_text)
    except json.JSONDecodeError as e:
        logger.error("LLM returned invalid JSON: %s", e)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"LLM returned invalid JSON: {e}",
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model_used,
            "tokens_used": tokens_used,
        }

    # Validate against extraction schema
    extraction_schema = contract.get("extraction_schema", {})
    valid, error_msg = validate_extraction(llm_output, extraction_schema)
    if not valid:
        logger.error("Extraction output failed schema validation: %s", error_msg)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"Schema validation failed: {error_msg}",
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model_used,
            "tokens_used": tokens_used,
        }

    # Apply validation rules (max counts from contract)
    validation_rules = contract.get("validation_rules", {})
    result = enforce_validation_rules(llm_output, validation_rules)

    return {
        "artifact_id": artifact_id,
        "success": True,
        "result": result,
        "is_receipt": is_receipt,
        "prompt_contract_version": contract_version,
        "processing_time_ms": _elapsed_ms(start),
        "model_used": model_used,
        "tokens_used": tokens_used,
    }


async def handle_crosssource(
    data: dict,
    provider: str,
    model: str,
    api_key: str,
    ollama_url: str,
) -> dict:
    """Handle a synthesis.crosssource request.

    Assesses whether artifacts from different sources sharing a concept
    represent a genuine cross-domain connection.
    """
    concept_id = data.get("concept_id", "")
    concept_title = data.get("concept_title", "")
    contract_version = data.get("prompt_contract_version", "")
    artifacts = data.get("artifacts", [])

    try:
        contract = load_prompt_contract(contract_version)
    except FileNotFoundError:
        return {
            "concept_id": concept_id,
            "has_genuine_connection": False,
            "insight_text": "",
            "confidence": 0.0,
            "artifact_ids": [],
            "prompt_contract_version": contract_version,
            "processing_time_ms": 0,
            "model_used": "",
        }

    prompt_parts = [
        contract.get("system_prompt", ""),
        f"\nConcept: {concept_title}",
        "\nArtifacts from different sources:",
    ]
    for art in artifacts:
        prompt_parts.append(f"- [{art.get('source_type', '')}] {art.get('title', '')}: {art.get('summary', '')[:300]}")
    prompt_parts.append(
        "\nAssess: Is there a genuine cross-source connection beyond surface-level keyword overlap?"
        '\nReturn JSON: {"has_genuine_connection": bool, "insight_text": str, "confidence": float}'
    )

    start = time.time()

    try:
        import litellm

        llm_model = model
        if provider == "ollama":
            llm_model = f"ollama/{model}"
            litellm.api_base = ollama_url

        response = await litellm.acompletion(
            model=llm_model,
            messages=[{"role": "user", "content": "\n".join(prompt_parts)}],
            temperature=contract.get("temperature", 0.3),
            max_tokens=contract.get("token_budget", 500),
            api_key=api_key if provider != "ollama" else None,
            response_format={"type": "json_object"},
        )

        output = json.loads(response.choices[0].message.content)
        model_used = response.model or llm_model

    except Exception as e:
        logger.error("Cross-source LLM call failed: %s", type(e).__name__)
        return {
            "concept_id": concept_id,
            "has_genuine_connection": False,
            "insight_text": "",
            "confidence": 0.0,
            "artifact_ids": [a.get("id", "") for a in artifacts],
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model or "",
        }

    return {
        "concept_id": concept_id,
        "has_genuine_connection": output.get("has_genuine_connection", False),
        "insight_text": output.get("insight_text", ""),
        "confidence": output.get("confidence", 0.0),
        "artifact_ids": [a.get("id", "") for a in artifacts],
        "prompt_contract_version": contract_version,
        "processing_time_ms": _elapsed_ms(start),
        "model_used": model_used,
    }


def enforce_validation_rules(output: dict, rules: dict) -> dict:
    """Trim extraction output to respect validation_rules max counts."""
    max_concepts = rules.get("max_concepts", 10)
    max_entities = rules.get("max_entities", 20)
    max_relationships = rules.get("max_relationships", 30)
    max_contradictions = rules.get("max_contradictions", 5)

    result = {
        "concepts": output.get("concepts", [])[:max_concepts],
        "entities": output.get("entities", [])[:max_entities],
        "relationships": output.get("relationships", [])[:max_relationships],
        "contradictions": output.get("contradictions", [])[:max_contradictions],
    }
    return result
