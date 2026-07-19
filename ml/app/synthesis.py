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

from .metrics import synthesis_schema_repair_attempts_total
from .ollama_keepalive import (
    OllamaProfileConfigError,
    dispatch_litellm,
    resolve_domain_output_token_budget,
    resolve_ollama_request_profile,
)
from .ollama_thinking import apply_structured_extraction_thinking
from .receipt_detection import detect_receipt_content

logger = logging.getLogger("smackerel-ml.synthesis")

# Content char limit sent to LLM (design contract: 8000 chars)
MAX_CONTENT_CHARS = 8000


def resolve_synthesis_schema_repair_attempts() -> int:
    """Resolve the exact-one synthesis schema-repair budget from SST."""
    value = os.environ.get("ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS")
    if value is None or not value.strip():
        raise RuntimeError(
            "ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS is required "
            "(SST services.ml.synthesis_schema_repair_attempts)"
        )
    try:
        attempts = int(value)
    except ValueError:
        raise RuntimeError(
            "ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS expected=integer 1 category=invalid_schema_repair_budget"
        ) from None
    if attempts != 1:
        raise RuntimeError(
            "ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS expected=integer 1 category=invalid_schema_repair_budget"
        )
    return attempts


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


def classify_schema_validation_error(output: dict, schema: dict) -> str:
    """Return a content-free validator/path class for a terminal failure."""
    import jsonschema

    try:
        jsonschema.validate(instance=output, schema=schema)
        return "validator=none path=$"
    except jsonschema.ValidationError as error:
        path = "$"
        for part in error.absolute_path:
            path += f"[{part}]" if isinstance(part, int) else f".{part}"
        return f"validator={error.validator} path={path}"
    except jsonschema.SchemaError:
        return "validator=invalid_schema path=$"


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


def build_schema_repair_prompt(validation_error: str, schema: dict) -> str:
    """Build the corrective instruction without inventing semantic defaults."""
    return (
        "Your previous response was valid JSON but failed the required output schema.\n"
        f"Validation error: {validation_error}\n"
        "Return corrected JSON only. Preserve every supported semantic claim from the artifact; "
        "do not invent concepts, claims, entities, or relationships and do not substitute empty "
        "values for required semantic content.\n"
        f"Required schema:\n{json.dumps(schema, indent=2)}"
    )


async def _dispatch_synthesis_completion(
    completion_kwargs: dict,
    *,
    provider: str,
    model: str,
    profile,
    litellm_module,
    fallback_model: str,
) -> tuple[str, int, str]:
    response = await dispatch_litellm(
        completion_kwargs,
        provider=provider,
        model=model,
        profile=profile,
        litellm_module=litellm_module,
    )
    output_text = response.choices[0].message.content
    tokens_used = response.usage.total_tokens if response.usage else 0
    model_used = response.model or fallback_model
    return output_text, tokens_used, model_used


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
    trace_id = data.get("trace_id", "")

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
            "trace_id": trace_id,
        }

    # Build the LLM prompt
    prompt = build_synthesis_prompt(
        contract=contract,
        artifact_data=data,
        existing_concepts=data.get("existing_concepts", []),
        existing_entities=data.get("existing_entities", []),
    )

    temperature = contract.get("temperature", 0.3)
    # Spec 102 SCOPE-102-03 (BUG-026-006) — the prompt contract's token_budget
    # still wins when set; otherwise fall back to the SST-owned budget
    # (ML_DOMAIN_OUTPUT_TOKEN_BUDGET) instead of a hardcoded 2000 magic number.
    token_budget = contract.get("token_budget", resolve_domain_output_token_budget())
    resolve_synthesis_schema_repair_attempts()

    start = time.time()

    # Call LLM via litellm (consistent with existing ML sidecar pattern)
    try:
        import litellm

        llm_model = model
        if provider == "ollama":
            llm_model = f"ollama_chat/{model}"
            litellm.api_base = ollama_url

        completion_kwargs = {
            "model": llm_model,
            "messages": [
                {"role": "system", "content": contract.get("system_prompt", "")},
                {"role": "user", "content": prompt},
            ],
            "temperature": temperature,
            "max_tokens": token_budget,
            "api_key": api_key if provider != "ollama" else None,
            "response_format": {"type": "json_object"},
        }
        if provider == "ollama":
            completion_kwargs["api_base"] = ollama_url

        # BUG-026-007 (redteam F2, latency half) — disable qwen3 thinking on the
        # synthesis structured-JSON extraction call when SST says so via the
        # native top-level think=False (litellm forwards it to /api/chat on the
        # ollama_chat/ route above; the /no_think prompt token is ignored by
        # qwen3's template). No-op for non-ollama / when thinking stays on / on
        # non-qwen models.
        apply_structured_extraction_thinking(completion_kwargs, provider)
        profile = resolve_ollama_request_profile(model) if provider == "ollama" else None
        llm_output_text, tokens_used, model_used = await _dispatch_synthesis_completion(
            completion_kwargs,
            provider=provider,
            model=model,
            profile=profile,
            litellm_module=litellm,
            fallback_model=llm_model,
        )

    except OllamaProfileConfigError:
        raise
    except Exception as e:
        logger.error("LLM call failed for synthesis: %s", type(e).__name__)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"LLM call failed: {type(e).__name__}",
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model or "",
            "tokens_used": 0,
            "trace_id": trace_id,
        }

    # Parse LLM output as JSON
    try:
        llm_output = json.loads(llm_output_text)
    except (json.JSONDecodeError, TypeError) as e:
        logger.error("LLM returned invalid JSON: %s", type(e).__name__)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"LLM returned invalid JSON: {type(e).__name__}",
            "prompt_contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
            "model_used": model_used,
            "tokens_used": tokens_used,
            "trace_id": trace_id,
        }

    # Validate against extraction schema
    extraction_schema = contract.get("extraction_schema", {})
    valid, error_msg = validate_extraction(llm_output, extraction_schema)
    if not valid:
        logger.info(
            "Synthesis schema repair attempt class=schema_validation",
            extra={"repair_class": "schema_validation"},
        )
        synthesis_schema_repair_attempts_total.inc()
        repair_kwargs = dict(completion_kwargs)
        repair_kwargs["messages"] = [
            *completion_kwargs["messages"],
            {"role": "assistant", "content": llm_output_text},
            {
                "role": "user",
                "content": build_schema_repair_prompt(error_msg, extraction_schema),
            },
        ]

        try:
            repair_text, repair_tokens, model_used = await _dispatch_synthesis_completion(
                repair_kwargs,
                provider=provider,
                model=model,
                profile=profile,
                litellm_module=litellm,
                fallback_model=model_used,
            )
            tokens_used += repair_tokens
        except OllamaProfileConfigError:
            raise
        except Exception as e:
            logger.error("Synthesis schema repair LLM call failed: %s", type(e).__name__)
            return {
                "artifact_id": artifact_id,
                "success": False,
                "error": f"LLM schema repair failed: {type(e).__name__}",
                "prompt_contract_version": contract_version,
                "processing_time_ms": _elapsed_ms(start),
                "model_used": model_used,
                "tokens_used": tokens_used,
                "trace_id": trace_id,
            }

        try:
            llm_output = json.loads(repair_text)
        except (json.JSONDecodeError, TypeError) as e:
            logger.error("Synthesis schema repair returned invalid JSON: %s", type(e).__name__)
            return {
                "artifact_id": artifact_id,
                "success": False,
                "error": f"LLM schema repair returned invalid JSON: {type(e).__name__}",
                "prompt_contract_version": contract_version,
                "processing_time_ms": _elapsed_ms(start),
                "model_used": model_used,
                "tokens_used": tokens_used,
                "trace_id": trace_id,
            }

        valid, _ = validate_extraction(llm_output, extraction_schema)
        if not valid:
            repair_error_class = classify_schema_validation_error(llm_output, extraction_schema)
            logger.error(
                "Synthesis schema repair failed validation class=schema_validation",
                extra={"repair_class": "schema_validation"},
            )
            return {
                "artifact_id": artifact_id,
                "success": False,
                "error": f"Schema validation failed after repair: {repair_error_class}",
                "prompt_contract_version": contract_version,
                "processing_time_ms": _elapsed_ms(start),
                "model_used": model_used,
                "tokens_used": tokens_used,
                "trace_id": trace_id,
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
        "trace_id": trace_id,
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
            llm_model = f"ollama_chat/{model}"
            litellm.api_base = ollama_url

        crosssource_kwargs = {
            "model": llm_model,
            "messages": [{"role": "user", "content": "\n".join(prompt_parts)}],
            "temperature": contract.get("temperature", 0.3),
            "max_tokens": contract.get("token_budget", 500),
            "api_key": api_key if provider != "ollama" else None,
            "response_format": {"type": "json_object"},
        }
        if provider == "ollama":
            crosssource_kwargs["api_base"] = ollama_url

        # BUG-026-007 (redteam F2, latency half) — disable qwen3 thinking on the
        # cross-source structured-JSON extraction call too via native
        # think=False (litellm forwards it top-level on the ollama_chat/ route).
        # No-op for non-ollama / when thinking stays on / on non-qwen models.
        apply_structured_extraction_thinking(crosssource_kwargs, provider)
        profile = resolve_ollama_request_profile(model) if provider == "ollama" else None
        response = await dispatch_litellm(
            crosssource_kwargs,
            provider=provider,
            model=model,
            profile=profile,
            litellm_module=litellm,
        )

        output = json.loads(response.choices[0].message.content)
        model_used = response.model or llm_model

    except OllamaProfileConfigError:
        raise
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
