"""Tests for ML sidecar startup auth-token validation (SCN-020-017, SCN-020-018, MIT-040-S-004).

MIT-040-S-004 (spec 040 SST hardening) revised the startup auth-token
contract to be SMACKEREL_ENV-conditional:

- SMACKEREL_ENV=production + empty SMACKEREL_AUTH_TOKEN → sys.exit(1)
- SMACKEREL_ENV=development|test + empty SMACKEREL_AUTH_TOKEN → WARN log,
  continue (preserves dev-mode bypass for single-tenant local stacks).
- Valid SMACKEREL_AUTH_TOKEN with any allowed environment value → no
  warn/exit (lifespan completes normally).

Tests below pin all three branches by exercising the FastAPI lifespan via
asyncio.run() and the unittest mock helpers (no pytest-asyncio needed).
"""

import asyncio
import logging
import os
from unittest.mock import MagicMock, patch

import pytest


class FakeNATSClient:
    def __init__(self):
        self.is_connected = True

    async def connect(self):
        self.is_connected = True

    async def subscribe_all(self):
        return None

    async def close(self):
        self.is_connected = False


def _run_lifespan(auth_token: str, environment: str, caplog) -> list[logging.LogRecord]:
    """Spin up the lifespan context manager under a controlled env and return log records."""
    env = {
        "NATS_URL": "nats://localhost:4222",
        "LLM_PROVIDER": "ollama",
        "LLM_MODEL": "llama3",
        "OLLAMA_URL": "http://localhost:11434",
        "ML_PROCESSING_DEGRADED_FALLBACK_ENABLED": "false",
        "SMACKEREL_AUTH_TOKEN": auth_token,
        "SMACKEREL_ENV": environment,
        # Spec 050 — ML sidecar health/worker isolation SST contract.
        # All three values are required for _check_required_config() to
        # succeed; tests in this module exercise the auth-token branch
        # only, so we pass valid defaults.
        "ML_EMBEDDING_WORKERS": "2",
        "ML_EMBEDDING_QUEUE_MAX": "3",
        "ML_HEALTH_LATENCY_SLA_MS": "500",
    }
    nats_mock = FakeNATSClient()

    with patch.dict(os.environ, env, clear=False):
        import importlib

        import app.main as main_mod

        importlib.reload(main_mod)

        # Patch NATSClient AFTER reload so the import binding is overridden
        with patch.object(main_mod, "NATSClient", return_value=nats_mock):

            async def _drive():
                mock_app = MagicMock()
                async with main_mod.lifespan(mock_app):
                    pass

            with caplog.at_level(logging.WARNING, logger="smackerel-ml"):
                loop = asyncio.new_event_loop()
                try:
                    loop.run_until_complete(_drive())
                finally:
                    loop.close()

    return list(caplog.records)


class TestMLStartupS004ProductionFailLoud:
    """MIT-040-S-004 — production env + empty SMACKEREL_AUTH_TOKEN → sys.exit(1).

    Adversarial proof: removing the production-mode auth_token branch in
    _check_required_config() makes this test fail (no SystemExit raised).
    """

    def test_exits_when_token_empty_in_production(self, caplog):
        with pytest.raises(SystemExit) as exc_info:
            _run_lifespan("", "production", caplog)

        assert (
            exc_info.value.code == 1
        ), f"Expected sys.exit(1) on empty SMACKEREL_AUTH_TOKEN in production, got code={exc_info.value.code!r}"

        error_records = [r for r in caplog.records if r.levelno >= logging.ERROR]
        joined = " ".join(r.message for r in error_records)
        assert (
            "SMACKEREL_AUTH_TOKEN" in joined
        ), f"Expected ERROR log naming SMACKEREL_AUTH_TOKEN, got: {[(r.levelname, r.message) for r in caplog.records]}"
        assert (
            "production" in joined
        ), f"Expected ERROR log mentioning production, got: {[(r.levelname, r.message) for r in caplog.records]}"


class TestMLStartupS004DevModeBypass:
    """MIT-040-S-004 — development env + empty SMACKEREL_AUTH_TOKEN → warn + continue.

    Adversarial proof: making AUTH_TOKEN unconditionally required (i.e.
    putting SMACKEREL_AUTH_TOKEN back into _check_required_config().keys)
    makes this test fail because the lifespan exits before the WARN log.
    """

    def test_warns_and_continues_when_token_empty_in_development(self, caplog):
        records = _run_lifespan("", "development", caplog)

        warning_records = [
            r
            for r in records
            if r.levelno == logging.WARNING and "SMACKEREL_AUTH_TOKEN" in r.message
        ]
        assert (
            warning_records
        ), f"Expected dev-mode bypass WARN log, got: {[(r.levelname, r.message) for r in records]}"


class TestMLStartupNoWarningWithToken:
    """SCN-020-018 — no auth warning when SMACKEREL_AUTH_TOKEN is configured.

    Test runs in development environment to exercise the
    warn-only-when-empty branch (when token is set, no auth warning is
    expected regardless of environment).
    """

    def test_no_warning_when_token_set(self, caplog):
        records = _run_lifespan("real-secret-token", "development", caplog)

        auth_warnings = [
            r for r in records if "SMACKEREL_AUTH_TOKEN is empty" in r.message
        ]
        assert (
            len(auth_warnings) == 0
        ), f"Expected no auth warning with token set, got: {[r.message for r in auth_warnings]}"
