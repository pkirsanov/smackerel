"""Embedding generation via sentence-transformers."""

import asyncio
import logging

logger = logging.getLogger("smackerel-ml.embedder")

# Global model holder — loaded once at import/startup
_model = None
_model_name = "all-MiniLM-L6-v2"


def _load_model():
    """Lazy-load the sentence-transformers model."""
    global _model
    if _model is None:
        from sentence_transformers import SentenceTransformer

        logger.info("Loading embedding model: %s", _model_name)
        _model = SentenceTransformer(_model_name)
        logger.info("Embedding model loaded (dim=%d)", _model.get_sentence_embedding_dimension())
    return _model


async def generate_embedding(text: str) -> list[float]:
    """Generate a 384-dimension embedding vector from text."""
    model = _load_model()
    loop = asyncio.get_event_loop()
    return await asyncio.wait_for(
        loop.run_in_executor(None, lambda: model.encode(text, normalize_embeddings=True).tolist()),
        timeout=10.0,
    )


async def generate_artifact_embedding(title: str, summary: str, key_ideas: list[str]) -> list[float]:
    """Generate embedding from artifact's title + summary + key ideas."""
    parts = [title]
    if summary:
        parts.append(summary)
    if key_ideas:
        parts.extend(key_ideas[:5])  # Limit to top 5 ideas

    combined = " ".join(parts)
    return await generate_embedding(combined)


def embedding_dimension() -> int:
    """Return the embedding dimension for the loaded model."""
    return 384  # all-MiniLM-L6-v2 fixed at 384
