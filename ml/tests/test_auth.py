"""Tests for ML sidecar auth middleware."""

from unittest.mock import patch

import pytest
from fastapi import APIRouter, Depends, FastAPI, HTTPException
from fastapi.testclient import TestClient


def _make_app(auth_token: str) -> FastAPI:
    """Create a test FastAPI app with the auth dependency."""
    with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": auth_token}):
        # Re-import to pick up patched env var
        import importlib

        import app.auth as auth_mod

        importlib.reload(auth_mod)

        test_app = FastAPI()

        @test_app.get("/health")
        async def health():
            return {"status": "up"}

        authed = APIRouter(dependencies=[Depends(auth_mod.verify_auth)])

        @authed.get("/process")
        async def process():
            return {"result": "ok"}

        test_app.include_router(authed)
        return test_app


class TestMLSidecarAuthWithToken:
    """SCN-020-005, SCN-020-006, SCN-020-007: Auth enforcement when token is configured."""

    @pytest.fixture(autouse=True)
    def setup(self):
        self.app = _make_app("test-secret")
        self.client = TestClient(self.app)

    def test_reject_unauthenticated_request(self):
        """SCN-020-005: Non-health endpoint without auth returns 401."""
        resp = self.client.get("/process")
        assert resp.status_code == 401
        assert resp.json()["detail"] == "Unauthorized"

    def test_accept_bearer_token(self):
        """SCN-020-006: Valid Bearer token is accepted."""
        resp = self.client.get("/process", headers={"Authorization": "Bearer test-secret"})
        assert resp.status_code == 200
        assert resp.json()["result"] == "ok"

    def test_accept_x_auth_token_header(self):
        """SCN-020-006: Valid X-Auth-Token header is accepted."""
        resp = self.client.get("/process", headers={"X-Auth-Token": "test-secret"})
        assert resp.status_code == 200

    def test_reject_wrong_token(self):
        """SCN-020-005: Wrong token returns 401."""
        resp = self.client.get("/process", headers={"Authorization": "Bearer wrong-token"})
        assert resp.status_code == 401

    def test_health_unauthenticated(self):
        """SCN-020-007: Health endpoint remains unauthenticated."""
        resp = self.client.get("/health")
        assert resp.status_code == 200
        assert resp.json()["status"] == "up"


class TestMLSidecarAuthDevMode:
    """SCN-020-008: Dev mode passthrough when token is empty."""

    @pytest.fixture(autouse=True)
    def setup(self):
        self.app = _make_app("")
        self.client = TestClient(self.app)

    def test_allow_unauthenticated_in_dev_mode(self):
        """SCN-020-008: Empty token allows all requests."""
        resp = self.client.get("/process")
        assert resp.status_code == 200
        assert resp.json()["result"] == "ok"

    def test_health_in_dev_mode(self):
        """SCN-020-008: Health still works in dev mode."""
        resp = self.client.get("/health")
        assert resp.status_code == 200


class TestMLSidecarAuthAdversarial:
    """GAP-020-R30-002: Non-ASCII tokens must get 401, not 500 (CWE-755)."""

    @pytest.fixture(autouse=True)
    def setup(self):
        self.app = _make_app("test-secret")
        self.client = TestClient(self.app)

    def test_non_ascii_bearer_returns_401(self):
        """Non-ASCII str in Bearer token must raise 401, not TypeError (CWE-755).

        Uvicorn decodes headers as Latin-1, so non-ASCII bytes arrive as
        extended Python str. hmac.compare_digest raises TypeError on non-ASCII
        str. The auth dependency must catch this and return 401.
        """
        import asyncio
        import importlib
        from unittest.mock import MagicMock

        import app.auth as auth_mod

        with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "test-secret"}):
            importlib.reload(auth_mod)

        # Build a mock Request with a non-ASCII Authorization header
        # (simulating what uvicorn delivers for Latin-1 encoded bytes)
        mock_request = MagicMock()
        mock_request.headers = {
            "authorization": "Bearer caf\u00e9-tok\u00ebn",
        }
        mock_request.method = "GET"
        mock_request.url.path = "/process"
        mock_request.client.host = "127.0.0.1"

        with pytest.raises(HTTPException) as exc_info:
            asyncio.get_event_loop().run_until_complete(auth_mod.verify_auth(mock_request))
        assert exc_info.value.status_code == 401

    def test_non_ascii_x_auth_token_returns_401(self):
        """Non-ASCII str in X-Auth-Token must raise 401, not TypeError."""
        import asyncio
        import importlib
        from unittest.mock import MagicMock

        import app.auth as auth_mod

        with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "test-secret"}):
            importlib.reload(auth_mod)

        mock_request = MagicMock()
        mock_request.headers = {
            "authorization": "",
            "x-auth-token": "\u00fc\u00f1\u00ee\u00e7\u00f8\u00f0\u00e9",
        }
        mock_request.method = "GET"
        mock_request.url.path = "/process"
        mock_request.client.host = "127.0.0.1"

        with pytest.raises(HTTPException) as exc_info:
            asyncio.get_event_loop().run_until_complete(auth_mod.verify_auth(mock_request))
        assert exc_info.value.status_code == 401

    def test_empty_bearer_prefix_returns_401(self):
        """'Bearer ' with no actual token value must return 401."""
        resp = self.client.get(
            "/process",
            headers={"Authorization": "Bearer "},
        )
        assert resp.status_code == 401
