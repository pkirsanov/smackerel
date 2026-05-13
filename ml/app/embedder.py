"""Embedding generation via sentence-transformers.

Spec 050 — ML Sidecar Health Isolation
======================================

The embedder runs CPU-bound sentence-transformer encode() work on a
dedicated ``concurrent.futures.ThreadPoolExecutor`` whose size is bound
by ``ML_EMBEDDING_WORKERS`` (SST). The default asyncio executor stays
free for any other async-adjacent work, and the FastAPI ``/health``
endpoint never enters this executor at all (it is a pure async coroutine
that reads in-memory state), so a saturated embedding pool cannot
starve health probes (FR-050-001).

Backpressure: ``ML_EMBEDDING_QUEUE_MAX`` caps the in-flight (active +
queued) request count. Excess requests are rejected with a
``RuntimeError`` so the upstream NATS consumer / search fallback path
takes over rather than letting the executor queue grow without bound
(FR-050-002).

Observability: ``ml/app/metrics.py`` exposes
``smackerel_ml_embedding_inflight`` and
``smackerel_ml_embedding_rejected_total`` so operators and tests can see
queue pressure in real time (FR-050-005).
"""

import asyncio
import logging
import os
import threading
from concurrent.futures import ThreadPoolExecutor
from typing import Optional

from .metrics import (
    embedding_inflight,
    embedding_rejected_total,
    embedding_workers_configured,
)

logger = logging.getLogger("smackerel-ml.embedder")

# Global model holder — loaded once at import/startup
_model = None
_model_name = "all-MiniLM-L6-v2"

# Spec 050 — dedicated bounded executor. Lazily constructed on first
# generate_embedding() call so module import remains side-effect-free and
# the unit tests can monkeypatch ML_EMBEDDING_WORKERS before the pool is
# materialised. ``_executor_lock`` guards initialisation across threads;
# ``_executor`` reads after the one-time init are lock-free.
_executor: Optional[ThreadPoolExecutor] = None
_executor_lock = threading.Lock()
_executor_workers: int = 0

# Backpressure: ``_pending_count`` tracks in-flight (admitted but not
# completed) requests; ``_pending_lock`` serialises the
# admit/reject/decrement state transition. ``_pending_queue_max`` is
# bound from ``ML_EMBEDDING_QUEUE_MAX`` on first admission.
_pending_count = 0
_pending_lock = threading.Lock()
_pending_queue_max: int = 0


def _read_positive_int_env(key: str) -> int:
    """Read a required positive integer env var. Fail-loud SST.

    Raises ``RuntimeError`` (not ``SystemExit``) here so callers in
    request paths can convert it to a 5xx without aborting the entire
    sidecar. Startup-time validation in ``main.py::_check_required_config``
    is the authoritative fail-fast guard at process start; this helper is
    defense-in-depth for runtime drift (e.g. an operator clearing the env
    var via ``docker exec`` on a running container).
    """
    try:
        raw = os.environ[key]
    except KeyError as exc:
        raise RuntimeError(f"{key} is required (spec 050 SST contract)") from exc
    if not raw:
        raise RuntimeError(f"{key} is required (spec 050 SST contract)")
    try:
        value = int(raw)
    except ValueError as exc:
        raise RuntimeError(f"{key} must be a positive integer, got {raw!r}") from exc
    if value < 1:
        raise RuntimeError(f"{key} must be a positive integer, got {value}")
    return value


def _ensure_executor() -> ThreadPoolExecutor:
    """Return the dedicated embedding executor, constructing it on first use."""
    global _executor, _executor_workers, _pending_queue_max
    # Fast path — already initialised.
    if _executor is not None:
        return _executor
    with _executor_lock:
        if _executor is not None:
            return _executor
        workers = _read_positive_int_env("ML_EMBEDDING_WORKERS")
        queue_max = _read_positive_int_env("ML_EMBEDDING_QUEUE_MAX")
        if queue_max < workers:
            raise RuntimeError(
                f"ML_EMBEDDING_QUEUE_MAX ({queue_max}) must be >= "
                f"ML_EMBEDDING_WORKERS ({workers})"
            )
        executor = ThreadPoolExecutor(
            max_workers=workers,
            thread_name_prefix="smackerel-ml-embed",
        )
        _executor = executor
        _executor_workers = workers
        _pending_queue_max = queue_max
        embedding_workers_configured.set(workers)
        logger.info(
            "embedding executor initialised",
            extra={
                "workers": workers,
                "queue_max": queue_max,
            },
        )
        return executor


def _reset_for_tests() -> None:
    """Test-only hook to tear down the executor between cases.

    Importable as ``embedder._reset_for_tests`` from unit tests. Not part
    of the public sidecar API.
    """
    global _executor, _executor_workers, _pending_queue_max, _pending_count
    with _executor_lock:
        if _executor is not None:
            _executor.shutdown(wait=False, cancel_futures=True)
        _executor = None
        _executor_workers = 0
        _pending_queue_max = 0
    with _pending_lock:
        _pending_count = 0
    embedding_inflight.set(0)
    embedding_workers_configured.set(0)


def _load_model():
    """Lazy-load the sentence-transformers model."""
    global _model
    if _model is None:
        from sentence_transformers import SentenceTransformer

        logger.info("Loading embedding model: %s", _model_name)
        _model = SentenceTransformer(_model_name)
        logger.info(
            "Embedding model loaded (dim=%d)", _model.get_sentence_embedding_dimension()
        )
    return _model


async def generate_embedding(text: str) -> list[float]:
    """Generate a 384-dimension embedding vector from text.

    Admission control: rejects with ``RuntimeError`` when in-flight count
    already equals ``ML_EMBEDDING_QUEUE_MAX``. The executor itself is
    bounded by ``ML_EMBEDDING_WORKERS``; the queue cap guards against
    unbounded executor queueing and gives the upstream caller an
    immediate fail-fast signal so it can fall back rather than wait.
    """
    global _pending_count

    executor = _ensure_executor()

    with _pending_lock:
        if _pending_count >= _pending_queue_max:
            embedding_rejected_total.inc()
            raise RuntimeError(
                f"embedding backpressure: {_pending_count} requests in flight "
                f"(queue_max={_pending_queue_max}), rejecting"
            )
        _pending_count += 1
        admitted_count = _pending_count
    embedding_inflight.set(admitted_count)

    future = None
    try:
        model = _load_model()
        loop = asyncio.get_event_loop()
        future = loop.run_in_executor(
            executor,
            lambda: model.encode(text, normalize_embeddings=True).tolist(),
        )
        return await asyncio.wait_for(future, timeout=10.0)
    except asyncio.TimeoutError:
        if future is not None:
            future.cancel()  # best-effort cancel
        raise
    finally:
        with _pending_lock:
            _pending_count -= 1
            remaining = _pending_count
        embedding_inflight.set(remaining)


async def generate_artifact_embedding(
    title: str, summary: str, key_ideas: list[str]
) -> list[float]:
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
