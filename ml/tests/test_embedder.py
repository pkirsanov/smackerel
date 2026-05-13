"""Regression tests for embedding runtime behavior."""

import asyncio
import threading
import time
from pathlib import Path

import pytest

import app.embedder as embedder
import app.metrics as metrics


@pytest.fixture(autouse=True)
def _reset_embedder(monkeypatch):
    """Reset the embedder executor and provide sane spec 050 SST env defaults.

    Each test case still owns its env via monkeypatch — this fixture only
    establishes a valid baseline so a single env-clearing test does not
    leak state into the next one.
    """
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    embedder._reset_for_tests()
    yield
    embedder._reset_for_tests()


def test_generate_embedding_releases_pending_count_when_model_load_fails(monkeypatch):
    embedder._pending_count = 0

    def fail_load_model():
        raise RuntimeError("model metadata unavailable")

    monkeypatch.setattr(embedder, "_load_model", fail_load_model)

    with pytest.raises(RuntimeError, match="model metadata unavailable"):
        asyncio.run(embedder.generate_embedding("hello"))

    assert embedder._pending_count == 0


def test_ml_dockerfile_preserves_python_package_metadata():
    dockerfile = Path(__file__).resolve().parents[1] / "Dockerfile"
    for line in dockerfile.read_text().splitlines():
        assert not ("dist-info" in line and "rm -rf" in line)


def test_ml_dockerfile_provisions_writable_embedding_cache():
    dockerfile = Path(__file__).resolve().parents[1] / "Dockerfile"
    contents = dockerfile.read_text()

    assert "SentenceTransformer('all-MiniLM-L6-v2')" in contents
    assert "ENV HOME=/home/smackerel" in contents
    assert "ENV HF_HOME=/home/smackerel/.cache/huggingface" in contents
    assert (
        "ENV SENTENCE_TRANSFORMERS_HOME=/home/smackerel/.cache/sentence-transformers"
        in contents
    )
    assert (
        "COPY --from=builder --chown=smackerel:smackerel /opt/hf-cache /home/smackerel/.cache"
        in contents
    )


# Spec 050 FR-050-002 — bounded worker pool admission control.
#
# These tests build a fake "encode" that blocks on threading.Event so we can
# assert against the executor without loading the real sentence-transformer
# model (which would download hundreds of MB at test time).


class _BlockingModel:
    """Stand-in for SentenceTransformer that blocks encode() on an Event."""

    def __init__(
        self, release_event: threading.Event, started_event: threading.Event = None
    ):
        self.release_event = release_event
        self.started_event = started_event
        self.encode_calls = 0
        self._lock = threading.Lock()

    def encode(self, text, normalize_embeddings=True):
        with self._lock:
            self.encode_calls += 1
            started = self.started_event
        if started is not None:
            started.set()
        # Block until the test signals completion.
        self.release_event.wait(timeout=10.0)

        class _Vec:
            def tolist(self_inner):
                return [0.0] * 384

        return _Vec()


def test_spec050_bounded_executor_size_matches_ml_embedding_workers(monkeypatch):
    """The dedicated executor is constructed with ML_EMBEDDING_WORKERS slots."""
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "4")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "8")
    embedder._reset_for_tests()

    executor = embedder._ensure_executor()

    # ThreadPoolExecutor exposes _max_workers; this is a stable runtime attr.
    assert executor._max_workers == 4
    assert embedder._executor_workers == 4
    assert embedder._pending_queue_max == 8


def test_spec050_backpressure_rejects_at_queue_max(monkeypatch):
    """Once admitted count == queue_max, new requests MUST be rejected.

    Adversarial proof: removing the queue-cap check in generate_embedding
    makes this test fail because the fourth call returns a vector instead
    of raising.
    """
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    embedder._reset_for_tests()

    release = threading.Event()
    started = threading.Event()
    model = _BlockingModel(release, started)
    monkeypatch.setattr(embedder, "_load_model", lambda: model)

    async def fill_and_reject():
        # Spawn three concurrent embed tasks — they will all be admitted
        # because queue_max=3. The fourth one must be rejected.
        tasks = [
            asyncio.create_task(embedder.generate_embedding(f"text-{i}"))
            for i in range(3)
        ]
        # Wait for at least one encode to start so we know the executor is
        # busy; the rest are queued inside the executor.
        await asyncio.get_event_loop().run_in_executor(None, started.wait, 5.0)
        rejected = None
        try:
            await embedder.generate_embedding("rejected-text")
        except RuntimeError as exc:
            rejected = exc
        # Release the blocking model so the three admitted tasks can complete
        # and the executor can be torn down by the fixture.
        release.set()
        await asyncio.gather(*tasks)
        return rejected

    rejected = asyncio.run(fill_and_reject())
    assert rejected is not None, "expected RuntimeError once queue_max was reached"
    assert "backpressure" in str(rejected)
    assert "queue_max=3" in str(rejected)


def test_spec050_inflight_metric_tracks_admitted_count(monkeypatch):
    """smackerel_ml_embedding_inflight rises with admitted work, drops to 0 after.

    Adversarial proof: removing the embedding_inflight.set() calls makes
    this test fail because the gauge stays at 0 while work is in flight.
    """
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "1")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "2")
    embedder._reset_for_tests()

    release = threading.Event()
    started = threading.Event()
    model = _BlockingModel(release, started)
    monkeypatch.setattr(embedder, "_load_model", lambda: model)

    async def run_with_inflight_observation():
        task = asyncio.create_task(embedder.generate_embedding("text"))
        # Wait for the worker thread to actually start encode().
        await asyncio.get_event_loop().run_in_executor(None, started.wait, 5.0)
        observed_inflight = metrics.embedding_inflight._value.get()
        release.set()
        await task
        return observed_inflight

    inflight_during = asyncio.run(run_with_inflight_observation())
    inflight_after = metrics.embedding_inflight._value.get()

    assert (
        inflight_during >= 1
    ), f"expected inflight gauge >= 1 while encode was running, got {inflight_during}"
    assert (
        inflight_after == 0
    ), f"expected inflight gauge == 0 after task completion, got {inflight_after}"


def test_spec050_rejected_counter_increments_on_backpressure(monkeypatch):
    """smackerel_ml_embedding_rejected_total MUST increment when work is rejected.

    Adversarial proof: removing the embedding_rejected_total.inc() call
    makes this test fail because the counter stays flat under backpressure.
    """
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "1")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "1")
    embedder._reset_for_tests()

    release = threading.Event()
    started = threading.Event()
    model = _BlockingModel(release, started)
    monkeypatch.setattr(embedder, "_load_model", lambda: model)

    before = metrics.embedding_rejected_total._value.get()

    async def trigger_reject():
        admitted = asyncio.create_task(embedder.generate_embedding("admitted"))
        await asyncio.get_event_loop().run_in_executor(None, started.wait, 5.0)
        with pytest.raises(RuntimeError, match="backpressure"):
            await embedder.generate_embedding("rejected")
        release.set()
        await admitted

    asyncio.run(trigger_reject())

    after = metrics.embedding_rejected_total._value.get()
    assert (
        after == before + 1
    ), f"expected rejected counter to increment by 1, before={before} after={after}"


def test_spec050_health_handler_unblocked_by_busy_executor(monkeypatch):
    """FR-050-001 adversarial proof: /health returns within SLA while
    the embedding executor is fully saturated.

    The health coroutine is async-only and reads in-memory state, so it
    must NEVER enter the dedicated embedding executor. Saturating the
    executor MUST NOT slow /health.

    Adversarial proof: if a future change routes /health through the
    embedding executor (e.g. await loop.run_in_executor(embedder._executor, ...)),
    this test fails because the health call would wait on the blocking
    encode() and exceed the SLA budget.
    """
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "1")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "2")
    sla_ms = 500
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", str(sla_ms))
    embedder._reset_for_tests()

    release = threading.Event()
    started = threading.Event()
    model = _BlockingModel(release, started)
    monkeypatch.setattr(embedder, "_load_model", lambda: model)

    from app.main import health

    async def saturate_and_probe():
        # Saturate the executor with a long-running encode.
        embed_task = asyncio.create_task(embedder.generate_embedding("blocker"))
        # Wait for the worker thread to actually start encode().
        await asyncio.get_event_loop().run_in_executor(None, started.wait, 5.0)

        # Now probe /health 5 times while encode is blocked. Each probe
        # must complete well within the SLA budget. We use the median to
        # filter event-loop scheduling jitter on shared CI hardware.
        latencies_ms = []
        for _ in range(5):
            t0 = time.monotonic()
            response = await health()
            elapsed_ms = (time.monotonic() - t0) * 1000.0
            latencies_ms.append(elapsed_ms)
            assert "status" in response, f"unexpected health payload: {response}"

        release.set()
        await embed_task
        return latencies_ms

    latencies_ms = asyncio.run(saturate_and_probe())
    median = sorted(latencies_ms)[len(latencies_ms) // 2]

    assert median < sla_ms, (
        f"FR-050-003 SLA breach: median /health latency {median:.1f}ms "
        f">= configured budget {sla_ms}ms while embedding executor saturated. "
        f"All samples: {[f'{x:.1f}' for x in latencies_ms]}"
    )
    # All probes must complete within 5x the SLA even on a slow CI box;
    # this guards against an executor-routed regression.
    assert max(latencies_ms) < 5 * sla_ms, (
        f"FR-050-001 isolation breach: max /health latency {max(latencies_ms):.1f}ms "
        f"exceeded 5x SLA ({5*sla_ms}ms). Health probe was likely waiting "
        f"behind embedding executor."
    )


def test_spec050_workers_configured_metric_published(monkeypatch):
    """smackerel_ml_embedding_workers_configured MUST equal ML_EMBEDDING_WORKERS."""
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "3")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "6")
    embedder._reset_for_tests()

    embedder._ensure_executor()
    assert metrics.embedding_workers_configured._value.get() == 3
