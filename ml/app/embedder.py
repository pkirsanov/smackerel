"""Embedding generation via sentence-transformers."""

import asyncio
import logging
import threading

logger = logging.getLogger("smackerel-ml.embedder")

# Global model holder — loaded once at import/startup
_model = None
_model_name = "all-MiniLM-L6-v2"

# Backpressure: track in-flight executor tasks to prevent thread leak
_pending_count = 0
_pending_lock = threading.Lock()
_MAX_PENDING = 3


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
    global _pending_count

    with _pending_lock:
        if _pending_count >= _MAX_PENDING:
            raise RuntimeError(
                f"embedding backpressure: {_pending_count} requests in flight, rejecting"
            )
        _pending_count += 1

    model = _load_model()
    loop = asyncio.get_event_loop()
    future = loop.run_in_executor(
        None, lambda: model.encode(text, normalize_embeddings=True).tolist()
    )
    try:
        return await asyncio.wait_for(future, timeout=10.0)
    except asyncio.TimeoutError:
        future.cancel()  # best-effort cancel
        raise
    finally:
        with _pending_lock:
            _pending_count -= 1


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
