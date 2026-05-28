"""Regression tests for BUG-007-003: keep_bridge silent exception swallow.

Verifies that serialize_note emits WARNING logs (instead of silently swallowing
exceptions) for each of the five gkeepapi attribute-access failure points:
labels.all, collaborators.all, items iteration, timestamps.updated,
timestamps.created. Fallback values and non-raising behavior must be preserved.
"""

import logging
from unittest.mock import MagicMock, PropertyMock

from app.keep_bridge import serialize_note

LOGGER_NAME = "smackerel-ml.keep-bridge"


def _make_baseline_gnote() -> MagicMock:
    """Build a gnote mock whose top-level fields succeed; sub-fields configurable."""
    gnote = MagicMock()
    gnote.id = "n1"
    gnote.title = "t"
    gnote.text = "x"
    gnote.pinned = False
    gnote.archived = False
    gnote.trashed = False
    gnote.color = "DEFAULT"
    # Sub-fields default to working values
    gnote.labels.all.return_value = []
    gnote.collaborators.all.return_value = []
    gnote.items = []
    gnote.timestamps.updated = None
    gnote.timestamps.created = None
    return gnote


def _warning_records(caplog) -> list[logging.LogRecord]:
    return [r for r in caplog.records if r.name == LOGGER_NAME and r.levelno == logging.WARNING]


class TestSerializeNoteSurfacesFailures:
    """BUG-007-003 regression: each attribute-access failure must emit a WARNING."""

    def test_labels_failure_logs_warning(self, caplog):
        gnote = _make_baseline_gnote()
        gnote.labels.all.side_effect = AttributeError("labels broken")
        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)
        warnings = _warning_records(caplog)
        assert len(warnings) == 1
        assert "labels" in warnings[0].getMessage()
        assert "AttributeError" in warnings[0].getMessage()
        assert result["labels"] == []

    def test_collaborators_failure_logs_warning(self, caplog):
        gnote = _make_baseline_gnote()
        gnote.collaborators.all.side_effect = RuntimeError("collab broken")
        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)
        warnings = _warning_records(caplog)
        assert len(warnings) == 1
        assert "collaborators" in warnings[0].getMessage()
        assert "RuntimeError" in warnings[0].getMessage()
        assert result["collaborators"] == []

    def test_items_failure_logs_warning(self, caplog):
        gnote = _make_baseline_gnote()
        # Make iteration over items raise
        type(gnote).items = PropertyMock(side_effect=TypeError("items broken"))
        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)
        warnings = _warning_records(caplog)
        assert len(warnings) == 1
        assert "list_items" in warnings[0].getMessage()
        assert "TypeError" in warnings[0].getMessage()
        assert result["list_items"] == []

    def test_timestamps_updated_failure_logs_warning(self, caplog):
        gnote = _make_baseline_gnote()

        class BadTS:
            @property
            def updated(self):
                raise AttributeError("upd broken")

            created = None

        gnote.timestamps = BadTS()
        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)
        warnings = _warning_records(caplog)
        assert len(warnings) == 1
        assert "timestamps.updated" in warnings[0].getMessage()
        assert "AttributeError" in warnings[0].getMessage()
        assert result["modified_usec"] == 0

    def test_timestamps_created_failure_logs_warning(self, caplog):
        gnote = _make_baseline_gnote()

        class BadTS:
            updated = None

            @property
            def created(self):
                raise AttributeError("crt broken")

        gnote.timestamps = BadTS()
        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)
        warnings = _warning_records(caplog)
        assert len(warnings) == 1
        assert "timestamps.created" in warnings[0].getMessage()
        assert "AttributeError" in warnings[0].getMessage()
        assert result["created_usec"] == 0

    def test_all_five_failures_emit_five_distinct_warnings(self, caplog):
        """Adversarial: removing any single logger.warning would drop count below 5."""
        gnote = _make_baseline_gnote()
        gnote.labels.all.side_effect = AttributeError("labels")
        gnote.collaborators.all.side_effect = AttributeError("collab")

        class BadItems:
            def __iter__(self):
                raise AttributeError("items")

        gnote.items = BadItems()

        class BadTS:
            @property
            def updated(self):
                raise AttributeError("upd")

            @property
            def created(self):
                raise AttributeError("crt")

        gnote.timestamps = BadTS()

        with caplog.at_level(logging.WARNING, logger=LOGGER_NAME):
            result = serialize_note(gnote)  # must not raise

        warnings = _warning_records(caplog)
        assert len(warnings) == 5, f"expected 5 WARNINGs, got {len(warnings)}: {[w.getMessage() for w in warnings]}"

        messages = [w.getMessage() for w in warnings]
        contexts = ["labels", "collaborators", "list_items", "timestamps.updated", "timestamps.created"]
        for ctx in contexts:
            assert any(ctx in m for m in messages), f"missing context {ctx} in {messages}"
        # Every warning must include the exception type
        for m in messages:
            assert "AttributeError" in m

        # Fallback shape preserved
        assert result["labels"] == []
        assert result["collaborators"] == []
        assert result["list_items"] == []
        assert result["modified_usec"] == 0
        assert result["created_usec"] == 0

    def test_serialize_note_does_not_raise_on_full_failure(self):
        gnote = _make_baseline_gnote()
        gnote.labels.all.side_effect = AttributeError("x")
        gnote.collaborators.all.side_effect = AttributeError("x")
        type(gnote).items = PropertyMock(side_effect=AttributeError("x"))

        class BadTS:
            @property
            def updated(self):
                raise AttributeError("x")

            @property
            def created(self):
                raise AttributeError("x")

        gnote.timestamps = BadTS()
        # Must not raise
        serialize_note(gnote)
