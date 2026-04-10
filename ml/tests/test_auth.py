"""Tests for ML sidecar auth middleware."""

from unittest.mock import patch

import pytest
from fastapi import FastAPI, APIRouter, Depends
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
        resp = self.client.get(
            "/process", headers={"Authorization": "Bearer test-secret"}
        )
        assert resp.status_code == 200
        assert resp.json()["result"] == "ok"

    def test_accept_x_auth_token_header(self):
        """SCN-020-006: Valid X-Auth-Token header is accepted."""
        resp = self.client.get(
            "/process", headers={"X-Auth-Token": "test-secret"}
        )
        assert resp.status_code == 200

    def test_reject_wrong_token(self):
        """SCN-020-005: Wrong token returns 401."""
        resp = self.client.get(
            "/process", headers={"Authorization": "Bearer wrong-token"}
        )
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
