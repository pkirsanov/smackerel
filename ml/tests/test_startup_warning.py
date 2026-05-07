"""Tests for ML sidecar startup auth token validation (SCN-020-017, SCN-020-018, MIT-040-S-004).

MIT-040-S-004 (spec 040 SST hardening) elevated the prior warn-on-empty behavior
to fail-loud: an empty or missing SMACKEREL_AUTH_TOKEN now causes the ML sidecar
to sys.exit(1) at startup via _check_required_config(), instead of logging a
WARNING and continuing in dev-mode-pass-through. The tests below pin both:
- empty token → SystemExit (no warning ever reached)
- valid token → lifespan completes without the legacy auth warning

We drive the async context manager via asyncio.run() to avoid requiring
pytest-asyncio as a dependency.
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


class TestMLStartupFailLoudEmptyToken:
    """SCN-020-017 / MIT-040-S-004: ML sidecar fails loud (sys.exit) when SMACKEREL_AUTH_TOKEN is empty.

    Behavior was hardened from warn-on-empty (spec 020) to fail-loud (spec 040
    MIT-040-S-004) per the SST zero-defaults policy in
    .github/copilot-instructions.md and .github/instructions/bubbles-config-sst.instructions.md.
    """

    def test_exits_when_token_empty(self, caplog):
        """When SMACKEREL_AUTH_TOKEN is empty, lifespan exits via _check_required_config()."""
        with pytest.raises(SystemExit) as exc_info:
            _run_lifespan("", caplog)

        assert exc_info.value.code == 1, (
            f"Expected sys.exit(1) on empty SMACKEREL_AUTH_TOKEN, got code={exc_info.value.code!r}"
        )

        error_records = [r for r in caplog.records if r.levelno >= logging.ERROR]
        assert any("SMACKEREL_AUTH_TOKEN" in r.message for r in error_records), (
            "Expected an ERROR log naming SMACKEREL_AUTH_TOKEN among missing required config, "
            f"got: {[(r.levelname, r.message) for r in caplog.records]}"
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
