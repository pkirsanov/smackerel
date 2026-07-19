"""Redteam F1 / BUG-050-002 regression — the ML sidecar half.

The ML sidecar ``GET /health`` computes ``"status": "degraded"`` (e.g. NATS
disconnected) but FastAPI serialised the plain dict with an UNCONDITIONAL HTTP
200, so every status-code consumer (the Docker ``HEALTHCHECK``, monitoring) was
blind to a degraded sidecar. That is the same masking BUG-050-002 fixes on the
Go ``/api/health`` surface, here COMPLETED on the ML ``/health`` surface that
spec 050 (ML Sidecar Health Isolation) actually owns.

Fix (mirrors the Go ``healthStrictRequested`` contract in
``internal/api/health.go``): an opt-in ``?strict=true|1|yes`` returns 503 when
the sidecar status is not ``"up"``. The DEFAULT ``/health`` (no param — exactly
what the Docker ``HEALTHCHECK``'s ``urllib.request.urlopen`` sends, which RAISES
on any non-2xx) is byte-for-byte unchanged and stays a plain 200 dict, so a
degraded-but-ALIVE sidecar is never restart-flapped.

``test_strict_degraded_returns_503`` is ADVERSARIAL: against the pre-fix handler
(unconditional 200) it FAILS; only a status-aware ``/health`` passes. The
``test_default_degraded_stays_200`` case is the NON-DESTABILIZATION invariant —
it protects the container liveness contract and would fail if a naive fix
flipped the default path to 503.

The ``TestClient(app)`` (no context-manager) pattern matches
``tests/test_metrics.py``: the NATS-connecting lifespan does NOT run, so
``nats_client`` stays ``None`` ⇒ status ``"degraded"`` by default.
"""

import asyncio

import pytest
from fastapi.responses import JSONResponse
from starlette.testclient import TestClient

import app.main as main
from app.main import app, health


class _ConnectedNATS:
    """Minimal stand-in for a connected NATSClient (only .is_connected is read
    by the /health handler)."""

    is_connected = True


@pytest.fixture
def client():
    # No context manager ⇒ the NATS-connecting startup lifespan does NOT run
    # (identical to tests/test_metrics.py), so app.main.nats_client stays None
    # and the aggregate status is "degraded" unless a test patches it.
    return TestClient(app)


def test_strict_degraded_returns_503(client, monkeypatch):
    """ADVERSARIAL (redteam F1): ?strict=true while the sidecar is degraded
    (NATS disconnected) MUST return HTTP 503. Against the pre-fix handler — which
    returned 200 unconditionally so degraded was invisible to every status-code
    consumer — this assertion FAILS. This is the regression guard."""
    monkeypatch.setattr(main, "nats_client", None, raising=False)
    resp = client.get("/health?strict=true")
    assert resp.status_code == 503, (
        "F1 masking regression: a degraded ML sidecar MUST surface 503 on the "
        f"opt-in ?strict path, got {resp.status_code}"
    )
    body = resp.json()
    assert body["status"] == "degraded"
    assert body["nats"] == "disconnected"


def test_strict_truthy_variants_return_503(client, monkeypatch):
    """?strict accepts 1|true|yes (case-insensitive) — parity with the Go
    healthStrictRequested contract — and all yield 503 when degraded."""
    monkeypatch.setattr(main, "nats_client", None, raising=False)
    for variant in ("1", "true", "TRUE", "Yes", "yes"):
        resp = client.get(f"/health?strict={variant}")
        assert resp.status_code == 503, (
            f"?strict={variant!r} must be treated as truthy ⇒ 503 when degraded, "
            f"got {resp.status_code}"
        )


def test_strict_healthy_returns_200(client, monkeypatch):
    """?strict=1 stays 200 when the sidecar is up (no false alarms on the
    operator/monitoring path)."""
    monkeypatch.setattr(main, "nats_client", _ConnectedNATS(), raising=False)
    resp = client.get("/health?strict=1")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "up"
    assert body["nats"] == "connected"


def test_default_degraded_stays_200(client, monkeypatch):
    """NON-DESTABILIZATION invariant: the DEFAULT /health (no ?strict — what the
    Docker HEALTHCHECK's urllib.request.urlopen sends, which RAISES on non-2xx)
    stays 200 even when degraded, so a degraded-but-alive sidecar is NOT marked
    unhealthy / restart-flapped. A naive 503-on-degraded fix would fail here."""
    monkeypatch.setattr(main, "nats_client", None, raising=False)
    resp = client.get("/health")
    assert resp.status_code == 200, (
        "liveness contract broken: default /health MUST stay 200 when degraded, "
        f"got {resp.status_code}"
    )
    assert resp.json()["status"] == "degraded"


def test_strict_falsey_and_absent_stay_200_when_degraded(client, monkeypatch):
    """A non-truthy ?strict value (e.g. false/0/garbage) is treated as the
    default liveness path and stays 200 even when degraded — only an explicit
    opt-in changes the status code."""
    monkeypatch.setattr(main, "nats_client", None, raising=False)
    for variant in ("", "false", "0", "no", "maybe"):
        resp = client.get(f"/health?strict={variant}")
        assert resp.status_code == 200, (
            f"?strict={variant!r} is not a truthy opt-in ⇒ must stay 200, "
            f"got {resp.status_code}"
        )
        assert resp.json()["status"] == "degraded"


def test_default_returns_plain_dict_not_response(monkeypatch):
    """Body-shape + backward-compat invariant: the default in-process call
    returns a subscriptable dict (BUG-050-001's regressions and test_main.py
    index health()["status"] / ["model_loaded"]), NOT a Response, so the default
    liveness path can never carry a non-200 status code."""
    monkeypatch.setattr(main, "nats_client", None, raising=False)
    resp = asyncio.run(health())
    assert isinstance(resp, dict)
    assert not isinstance(resp, JSONResponse)
    assert resp["status"] == "degraded"
    assert "model_loaded" in resp
