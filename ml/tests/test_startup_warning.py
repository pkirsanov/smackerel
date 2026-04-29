"""Tests for ML sidecar startup auth warning (SCN-020-017, SCN-020-018).

These tests exercise the warning branch inside the lifespan function.
We drive the async context manager via asyncio.run() to avoid requiring
pytest-asyncio as a dependency.
"""

import asyncio
import logging
import os
from unittest.mock import AsyncMock, MagicMock, patch


def _run_lifespan(auth_token: str, caplog) -> list[logging.LogRecord]:
    """Spin up the lifespan context manager under controlled env and return log records."""
    env = {
        "NATS_URL": "nats://localhost:4222",
        "LLM_PROVIDER": "ollama",
        "LLM_MODEL": "llama3",
        "OLLAMA_URL": "http://localhost:11434",
        "ML_PROCESSING_DEGRADED_FALLBACK_ENABLED": "false",
        "SMACKEREL_AUTH_TOKEN": auth_token,
    }
    nats_mock = AsyncMock()
    nats_mock.is_connected = True

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


class TestMLStartupWarningEmptyToken:
    """SCN-020-017: ML sidecar logs WARNING when SMACKEREL_AUTH_TOKEN is empty."""

    def test_warns_when_token_empty(self, caplog):
        """When SMACKEREL_AUTH_TOKEN is empty, lifespan emits a WARNING."""
        records = _run_lifespan("", caplog)

        assert any("SMACKEREL_AUTH_TOKEN is empty" in r.message for r in records), (
            f"Expected auth warning in log, got: {[r.message for r in records]}"
        )


class TestMLStartupNoWarningWithToken:
    """SCN-020-018: No warning when SMACKEREL_AUTH_TOKEN is configured."""

    def test_no_warning_when_token_set(self, caplog):
        """When SMACKEREL_AUTH_TOKEN is non-empty, no auth warning is logged."""
        records = _run_lifespan("real-secret-token", caplog)

        auth_warnings = [r for r in records if "SMACKEREL_AUTH_TOKEN is empty" in r.message]
        assert len(auth_warnings) == 0, (
            f"Expected no auth warning with token set, got: {[r.message for r in auth_warnings]}"
        )
