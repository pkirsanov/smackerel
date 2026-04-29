"""Regression tests for embedding runtime behavior."""

import asyncio
from pathlib import Path

import pytest

import app.embedder as embedder


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
    assert "ENV SENTENCE_TRANSFORMERS_HOME=/home/smackerel/.cache/sentence-transformers" in contents
    assert "COPY --from=builder --chown=smackerel:smackerel /opt/hf-cache /home/smackerel/.cache" in contents
