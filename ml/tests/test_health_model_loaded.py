"""Redteam F8 regression — ML /health must report the embedder's CURRENT
model-loaded state, not a stale import-time snapshot.

Root cause: ``ml/app/main.py`` did ``from .embedder import _model`` and then
``/health`` returned ``"model_loaded": _model is not None``. A ``from X import
name`` binds the IMPORTING module to the value at import time (``None``), and
that binding is NOT re-evaluated when ``embedder._model`` is later reassigned by
the lazy load in ``embedder._load_model()`` (invoked on the first
``generate_embedding()``). So ``/health`` reported ``model_loaded: false``
PERMANENTLY even though embeddings worked (``POST /embed`` → 200).

Fix: ``/health`` calls ``embedder.is_model_loaded()``, which reads the
module-global ``embedder._model`` at CALL time.

The ``…_tracks_embedder_not_stale_import`` and
``…_true_after_generate_embedding`` cases are ADVERSARIAL: they load the model
AFTER importing ``app.main`` and assert ``/health`` reflects it. Against the
pre-fix code (main's own ``_model`` frozen at ``None``) both FAIL; only a
/health that reads the embedder's live state passes.
"""

import asyncio

import app.embedder as embedder
from app.main import health


def test_is_model_loaded_reflects_current_state(monkeypatch):
    """The helper reads the live module global, so it flips with the model."""
    monkeypatch.setattr(embedder, "_model", None, raising=False)
    assert embedder.is_model_loaded() is False

    monkeypatch.setattr(embedder, "_model", object(), raising=False)
    assert embedder.is_model_loaded() is True


def test_health_model_loaded_tracks_embedder_not_stale_import(monkeypatch):
    """/health must follow embedder._model reassigned AFTER app.main imported."""
    # Before load: embedder._model is None → /health reports false.
    monkeypatch.setattr(embedder, "_model", None, raising=False)
    resp_before = asyncio.run(health())
    assert resp_before["model_loaded"] is False

    # Simulate the lazy load that generate_embedding() performs
    # (embedder._model := SentenceTransformer(...)). Against the pre-fix
    # ``from .embedder import _model``, main's local _model stays None here and
    # /health would STILL report false — this assertion is the regression guard.
    monkeypatch.setattr(embedder, "_model", object(), raising=False)
    resp_after = asyncio.run(health())
    assert resp_after["model_loaded"] is True, (
        "F8 stale-binding regression: /health must reflect embedder._model "
        "loaded at call time, not the import-time snapshot"
    )


def test_health_model_loaded_true_after_generate_embedding(monkeypatch):
    """Faithful end-to-end: drive the REAL generate_embedding() lazy-load path
    (with a stub model so no 90MB download) and assert /health flips to
    model_loaded:true afterwards — the exact user-observable contract."""
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "1")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "2")
    embedder._reset_for_tests()
    monkeypatch.setattr(embedder, "_model", None, raising=False)

    # Before any generate_embedding(): model not loaded.
    assert asyncio.run(health())["model_loaded"] is False

    class _StubVec:
        def tolist(self):
            return [0.0] * 384

    class _StubModel:
        def encode(self, text, normalize_embeddings=True):  # noqa: ARG002
            return _StubVec()

    # _load_model() is what generate_embedding() calls on first use; stub it so
    # the lazy load sets embedder._model exactly as the real path would, without
    # downloading sentence-transformers weights.
    def _fake_load():
        embedder._model = _StubModel()
        return embedder._model

    monkeypatch.setattr(embedder, "_load_model", _fake_load)

    vec = asyncio.run(embedder.generate_embedding("hello world"))
    assert len(vec) == 384

    # After a real generate_embedding(): /health MUST now report loaded.
    assert asyncio.run(health())["model_loaded"] is True

    embedder._reset_for_tests()
