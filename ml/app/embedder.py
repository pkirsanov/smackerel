"""Embedding generation via sentence-transformers."""

import logging
from typing import Any

import numpy as np

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


def generate_embedding(text: str) -> list[float]:
    """Generate a 384-dimension embedding vector from text."""
    model = _load_model()
    # Combine meaningful text for richer embedding
    embedding = model.encode(text, normalize_embeddings=True)
    return embedding.tolist()


def generate_artifact_embedding(title: str, summary: str, key_ideas: list[str]) -> list[float]:
    """Generate embedding from artifact's title + summary + key ideas."""
    parts = [title]
    if summary:
        parts.append(summary)
    if key_ideas:
        parts.extend(key_ideas[:5])  # Limit to top 5 ideas

    combined = " ".join(parts)
    return generate_embedding(combined)


def embedding_dimension() -> int:
    """Return the embedding dimension for the loaded model."""
    return 384  # all-MiniLM-L6-v2 fixed at 384
