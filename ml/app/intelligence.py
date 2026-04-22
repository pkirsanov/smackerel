"""Phase 5 intelligence handlers for the ML sidecar.

Handles NATS subjects:
- learning.classify — classify resource difficulty for learning paths
- content.analyze — generate writing angles from topic artifacts
- monthly.generate — produce LLM-enhanced monthly report text
- quickref.generate — compile quick references from source artifacts
- seasonal.analyze — detect seasonal patterns with LLM commentary
"""

import json
import logging
import time

logger = logging.getLogger("smackerel-ml.intelligence")

MAX_CONTENT_CHARS = 4000


def _elapsed_ms(start: float) -> int:
    return int((time.time() - start) * 1000)


def _truncate(text: str, max_chars: int = MAX_CONTENT_CHARS) -> str:
    if not text or len(text) <= max_chars:
        return text or ""
    truncated = text[:max_chars]
    last_space = truncated.rfind(" ")
    if last_space > max_chars * 0.8:
        return truncated[:last_space]
    return truncated


async def _call_llm(prompt: str, provider: str, model: str, api_key: str, ollama_url: str = "") -> str | None:
    """Call LLM with the given prompt. Returns response text or None on failure."""
    try:
        if provider == "ollama":
            import httpx
            url = ollama_url or "http://localhost:11434"
            async with httpx.AsyncClient(timeout=60.0) as client:
                resp = await client.post(
                    f"{url}/api/generate",
                    json={"model": model or "llama3", "prompt": prompt, "stream": False},
                )
                if resp.status_code == 200:
                    return resp.json().get("response", "")
        elif provider in ("openai", "azure"):
            import httpx
            headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
            base_url = "https://api.openai.com/v1" if provider == "openai" else api_key  # simplified
            async with httpx.AsyncClient(timeout=60.0) as client:
                resp = await client.post(
                    f"https://api.openai.com/v1/chat/completions",
                    headers=headers,
                    json={
                        "model": model or "gpt-4o-mini",
                        "messages": [{"role": "user", "content": prompt}],
                        "temperature": 0.3,
                    },
                )
                if resp.status_code == 200:
                    return resp.json()["choices"][0]["message"]["content"]
    except Exception as e:
        logger.warning("LLM call failed: %s", e)
    return None


async def handle_learning_classify(
    data: dict,
    provider: str | None,
    model: str | None,
    api_key: str | None,
    ollama_url: str | None = None,
) -> dict:
    """Classify resource difficulty for learning paths.

    Input: { artifact_id, title, summary, content_snippet, content_type }
    Output: { artifact_id, difficulty, key_takeaway, success }
    """
    start = time.time()
    artifact_id = data.get("artifact_id", "")
    title = data.get("title", "")
    summary = data.get("summary", "")
    content_snippet = _truncate(data.get("content_snippet", ""))

    # Try LLM classification
    if provider and api_key:
        prompt = (
            "Classify this educational resource's difficulty level.\n\n"
            f"Title: {title}\n"
            f"Summary: {summary}\n"
            f"Content snippet: {content_snippet}\n\n"
            "Return ONLY valid JSON with these fields:\n"
            '- "difficulty": one of "beginner", "intermediate", "advanced"\n'
            '- "key_takeaway": one sentence describing what this resource teaches\n\n'
            "JSON:"
        )
        result = await _call_llm(prompt, provider, model or "", api_key, ollama_url or "")
        if result:
            try:
                parsed = json.loads(result.strip().strip("`").strip())
                difficulty = parsed.get("difficulty", "intermediate")
                if difficulty not in ("beginner", "intermediate", "advanced"):
                    difficulty = "intermediate"
                return {
                    "artifact_id": artifact_id,
                    "difficulty": difficulty,
                    "key_takeaway": parsed.get("key_takeaway", ""),
                    "success": True,
                    "processing_time_ms": _elapsed_ms(start),
                }
            except (json.JSONDecodeError, KeyError):
                logger.warning("Failed to parse LLM difficulty classification")

    # Heuristic fallback
    lower = f"{title} {summary}".lower()
    difficulty = "intermediate"
    advanced_terms = ["advanced", "deep dive", "internals", "architecture", "performance", "optimization", "expert"]
    beginner_terms = ["introduction", "intro ", "beginner", "getting started", "101", "basics", "fundamentals", "tutorial"]

    for term in advanced_terms:
        if term in lower:
            difficulty = "advanced"
            break
    else:
        for term in beginner_terms:
            if term in lower:
                difficulty = "beginner"
                break

    return {
        "artifact_id": artifact_id,
        "difficulty": difficulty,
        "key_takeaway": "",
        "success": True,
        "processing_time_ms": _elapsed_ms(start),
    }


async def handle_content_analyze(
    data: dict,
    provider: str | None,
    model: str | None,
    api_key: str | None,
    ollama_url: str | None = None,
) -> dict:
    """Generate writing angles from topic artifacts.

    Input: { topic_id, topic_name, capture_count, source_diversity, supporting_ids }
    Output: { topic_id, angles: [{ title, uniqueness_rationale, format_suggestion }], success }
    """
    start = time.time()
    topic_id = data.get("topic_id", "")
    topic_name = data.get("topic_name", "")
    capture_count = data.get("capture_count", 0)
    source_diversity = data.get("source_diversity", 0)

    if provider and api_key:
        prompt = (
            f"You are a content strategist. A user has {capture_count} saved captures "
            f"about '{topic_name}' from {source_diversity} different sources.\n\n"
            "Generate 3-5 original writing angles. For each angle, explain why "
            "the user has a unique perspective others lack.\n\n"
            "Return ONLY valid JSON array with objects containing:\n"
            '- "title": angle title\n'
            '- "uniqueness_rationale": why this angle is unique to the user\n'
            '- "format_suggestion": "blog post", "detailed guide", "long-form essay", etc.\n\n'
            "JSON:"
        )
        result = await _call_llm(prompt, provider, model or "", api_key, ollama_url or "")
        if result:
            try:
                parsed = json.loads(result.strip().strip("`").strip())
                if isinstance(parsed, list):
                    return {
                        "topic_id": topic_id,
                        "angles": parsed[:5],
                        "success": True,
                        "processing_time_ms": _elapsed_ms(start),
                    }
            except (json.JSONDecodeError, KeyError):
                logger.warning("Failed to parse LLM content analysis")

    # Fallback: basic template angle
    fmt = "blog post"
    if capture_count > 100:
        fmt = "long-form essay"
    elif capture_count > 50:
        fmt = "detailed guide"

    return {
        "topic_id": topic_id,
        "angles": [{
            "title": f"Deep dive: {topic_name}",
            "uniqueness_rationale": f"{capture_count} captures from {source_diversity} sources",
            "format_suggestion": fmt,
        }],
        "success": True,
        "processing_time_ms": _elapsed_ms(start),
    }


async def handle_monthly_generate(
    data: dict,
    provider: str | None,
    model: str | None,
    api_key: str | None,
    ollama_url: str | None = None,
) -> dict:
    """Produce LLM-enhanced monthly report text.

    Input: Full MonthlyReport JSON with all assembled data sections.
    Output: { report_text, word_count, success }
    """
    start = time.time()

    if provider and api_key:
        # Build a structured summary for the LLM
        month = data.get("month", "")
        shifts = data.get("expertise_shifts", [])
        diet = data.get("information_diet", {})
        patterns = data.get("productivity_patterns", [])
        sub_sum = data.get("subscription_summary")
        seasonal = data.get("seasonal_patterns", [])

        prompt_parts = [
            "You are a personal knowledge analyst. Write a concise monthly self-knowledge "
            f"report for {month}. Keep it under 500 words, insightful and actionable.\n",
        ]

        if shifts:
            prompt_parts.append("Expertise shifts this month:")
            for s in shifts[:10]:
                prompt_parts.append(f"  - {s.get('topic_name', '')}: {s.get('direction', '')} "
                                    f"({s.get('previous_depth', 0)} → {s.get('current_depth', 0)})")

        if diet and diet.get("total", 0) > 0:
            prompt_parts.append(
                f"\nInformation diet: {diet.get('articles', 0)} articles, "
                f"{diet.get('videos', 0)} videos, {diet.get('emails', 0)} emails, "
                f"{diet.get('notes', 0)} notes"
            )

        if sub_sum and sub_sum.get("monthly_total", 0) > 0:
            prompt_parts.append(
                f"\nSubscriptions: ${sub_sum['monthly_total']:.2f}/month, "
                f"{len(sub_sum.get('active', []))} active"
            )

        if patterns:
            prompt_parts.append(f"\nPatterns: {', '.join(patterns[:5])}")

        if seasonal:
            prompt_parts.append("\nSeasonal insights:")
            for sp in seasonal[:3]:
                prompt_parts.append(f"  - {sp.get('observation', '')}")

        prompt_parts.append("\nWrite the report now:")

        result = await _call_llm("\n".join(prompt_parts), provider, model or "", api_key, ollama_url or "")
        if result:
            word_count = len(result.split())
            return {
                "report_text": result,
                "word_count": word_count,
                "success": True,
                "processing_time_ms": _elapsed_ms(start),
            }

    return {
        "report_text": "",
        "word_count": 0,
        "success": False,
        "processing_time_ms": _elapsed_ms(start),
    }


async def handle_quickref_generate(
    data: dict,
    provider: str | None,
    model: str | None,
    api_key: str | None,
    ollama_url: str | None = None,
) -> dict:
    """Compile quick reference from source artifacts.

    Input: { concept, source_artifacts: [{ id, title, summary }] }
    Output: { concept, content, source_artifact_ids: [], success }
    """
    start = time.time()
    concept = data.get("concept", "")
    sources = data.get("source_artifacts", [])
    source_ids = [s.get("id", "") for s in sources if s.get("id")]

    if provider and api_key and sources:
        source_text = "\n".join(
            f"- {s.get('title', 'Untitled')}: {_truncate(s.get('summary', ''), 500)}"
            for s in sources[:10]
        )
        prompt = (
            f"Create a concise quick reference for '{concept}' from these saved resources:\n\n"
            f"{source_text}\n\n"
            "Write a compact, scannable reference (bullet points, key facts, code examples "
            "if relevant). Keep it under 300 words. Output plain text only."
        )
        result = await _call_llm(prompt, provider, model or "", api_key, ollama_url or "")
        if result:
            return {
                "concept": concept,
                "content": result,
                "source_artifact_ids": source_ids,
                "success": True,
                "processing_time_ms": _elapsed_ms(start),
            }

    # Fallback: concatenate summaries
    content_parts = [f"Quick Reference: {concept}\n"]
    for s in sources[:10]:
        title = s.get("title", "Untitled")
        summary = s.get("summary", "")
        if summary:
            content_parts.append(f"• {title}: {_truncate(summary, 200)}")
    content = "\n".join(content_parts) if len(content_parts) > 1 else f"Quick reference for {concept}"

    return {
        "concept": concept,
        "content": content,
        "source_artifact_ids": source_ids,
        "success": True,
        "processing_time_ms": _elapsed_ms(start),
    }


async def handle_seasonal_analyze(
    data: dict,
    provider: str | None,
    model: str | None,
    api_key: str | None,
    ollama_url: str | None = None,
) -> dict:
    """Detect seasonal patterns with LLM commentary.

    Input: { month, patterns: [{ pattern, month, observation, actionable }] }
    Output: { patterns: [{ pattern, month, observation, actionable }], success }
    """
    start = time.time()
    patterns = data.get("patterns", [])
    month = data.get("month", "")

    if provider and api_key and patterns:
        pattern_text = "\n".join(
            f"- {p.get('observation', '')}" for p in patterns[:10]
        )
        prompt = (
            f"You are a personal analyst reviewing seasonal capture patterns for {month}.\n\n"
            f"Detected patterns:\n{pattern_text}\n\n"
            "For each pattern, write a brief, actionable insight. "
            "Return ONLY valid JSON array with objects containing:\n"
            '- "pattern": pattern type\n'
            '- "month": month name\n'
            '- "observation": human-readable insight\n'
            '- "actionable": true/false\n\n'
            "JSON:"
        )
        result = await _call_llm(prompt, provider, model or "", api_key, ollama_url or "")
        if result:
            try:
                parsed = json.loads(result.strip().strip("`").strip())
                if isinstance(parsed, list):
                    return {
                        "patterns": parsed,
                        "success": True,
                        "processing_time_ms": _elapsed_ms(start),
                    }
            except (json.JSONDecodeError, KeyError):
                logger.warning("Failed to parse LLM seasonal analysis")

    # Fallback: return input patterns as-is
    return {
        "patterns": patterns,
        "success": True,
        "processing_time_ms": _elapsed_ms(start),
    }
