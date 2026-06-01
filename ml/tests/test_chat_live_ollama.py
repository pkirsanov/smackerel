"""Spec 064 SCOPE-04 / PKT-WORKFLOW-A #1 — live Ollama smoke for /llm/chat.

Opt-in: requires OLLAMA_URL and a reachable Ollama instance. Skips
otherwise so CI without a live stack does not flake.

Run with: pytest -m live_ollama ml/tests/test_chat_live_ollama.py
"""

from __future__ import annotations

import os

import httpx
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.routes.chat import router
from app.schemas import ChatResponse, StopReason

pytestmark = pytest.mark.live_ollama


def _ollama_reachable(url: str) -> bool:
    try:
        r = httpx.get(url.rstrip("/") + "/api/tags", timeout=2.0)
        return r.status_code == 200
    except Exception:
        return False


@pytest.fixture(scope="module")
def client() -> TestClient:
    app = FastAPI()
    app.include_router(router)
    return TestClient(app)


def test_live_ollama_end_turn(client: TestClient) -> None:
    ollama_url = os.environ.get("OLLAMA_URL")
    if not ollama_url:
        pytest.skip("OLLAMA_URL not set; live Ollama smoke is opt-in")
    if not _ollama_reachable(ollama_url):
        pytest.skip(f"Ollama not reachable at {ollama_url}")

    model = os.environ.get("LLM_MODEL")
    if not model:
        pytest.skip("LLM_MODEL not set; cannot pick a model for live smoke")

    payload = {
        "model": model,
        "messages": [
            {"role": "user", "content": "Reply with the single word: ready."},
        ],
        "max_tokens": 32,
        "temperature": 0.0,
    }
    resp = client.post("/llm/chat", json=payload)
    assert resp.status_code == 200, resp.text
    body = ChatResponse.model_validate(resp.json())
    assert body.stop_reason in (StopReason.END_TURN, StopReason.TOOL_USE)
