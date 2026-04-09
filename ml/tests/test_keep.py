"""Tests for Google Keep bridge and OCR pipeline."""

import asyncio  # noqa: I001
import base64
import os
from unittest.mock import MagicMock, patch

from app.keep_bridge import authenticate, handle_sync_request, serialize_note
from app.ocr import _ocr_cache, check_cache, handle_ocr_request, store_cache
from app.ocr import _validate_ollama_url


# --- keep_bridge tests ---


class TestKeepBridge:
    """Tests for the gkeepapi Python bridge."""

    def test_serialize_text_note(self):
        """Serialized text note has all required fields."""
        gnote = MagicMock()
        gnote.id = "note-123"
        gnote.title = "Test Note"
        gnote.text = "Hello world"
        gnote.pinned = True
        gnote.archived = False
        gnote.trashed = False
        gnote.color = "BLUE"
        gnote.labels.all.return_value = []
        gnote.collaborators.all.return_value = []
        gnote.items = []
        gnote.timestamps.updated = None
        gnote.timestamps.created = None

        result = serialize_note(gnote)
        assert result["note_id"] == "note-123"
        assert result["title"] == "Test Note"
        assert result["text_content"] == "Hello world"
        assert result["is_pinned"] is True
        assert result["is_archived"] is False
        assert result["labels"] == []

    def test_serialize_checklist_note(self):
        """Serialized checklist note includes list items."""
        item1 = MagicMock()
        item1.text = "Buy milk"
        item1.checked = True
        item2 = MagicMock()
        item2.text = "Buy eggs"
        item2.checked = False

        gnote = MagicMock()
        gnote.id = "checklist-1"
        gnote.title = "Shopping"
        gnote.text = ""
        gnote.pinned = False
        gnote.archived = False
        gnote.trashed = False
        gnote.color = None
        gnote.labels.all.return_value = []
        gnote.collaborators.all.return_value = []
        gnote.items = [item1, item2]
        gnote.timestamps.updated = None
        gnote.timestamps.created = None

        result = serialize_note(gnote)
        assert len(result["list_items"]) == 2
        assert result["list_items"][0]["text"] == "Buy milk"
        assert result["list_items"][0]["is_checked"] is True

    def test_auth_failure_returns_error_response(self):
        """Invalid credentials produce an error response."""
        import app.keep_bridge as bridge

        # Reset cached session
        bridge._keep_session = None
        bridge._session_email = None

        # No env vars set = should fail
        with patch.dict(os.environ, {"KEEP_GOOGLE_EMAIL": "", "KEEP_GOOGLE_APP_PASSWORD": ""}):
            result = asyncio.run(handle_sync_request({"cursor": ""}))
            assert result["status"] == "error"
            assert len(result["notes"]) == 0

    def test_session_caching(self):
        """Two calls reuse the cached session."""
        import time

        import app.keep_bridge as bridge

        bridge._keep_session = MagicMock()
        bridge._session_email = "test@example.com"
        bridge._session_authenticated_at = time.time()  # Mark session as freshly authenticated

        with patch.dict(
            os.environ,
            {
                "KEEP_GOOGLE_EMAIL": "test@example.com",
                "KEEP_GOOGLE_APP_PASSWORD": "pw",
            },
        ):
            session = authenticate()
            assert session is bridge._keep_session

        # Reset
        bridge._keep_session = None
        bridge._session_email = None
        bridge._session_authenticated_at = 0.0


# --- OCR tests ---


class TestOCR:
    """Tests for the OCR pipeline."""

    def test_cache_hit(self):
        """Known image hash returns cached text."""
        _ocr_cache["abc123"] = {"text": "Meeting room layout", "engine": "tesseract"}

        result = asyncio.run(handle_ocr_request({"image_hash": "abc123", "image_data": ""}))
        assert result["cached"] is True
        assert result["text"] == "Meeting room layout"

        # Cleanup
        _ocr_cache.clear()

    def test_cache_miss(self):
        """Unknown image hash runs OCR."""
        _ocr_cache.clear()

        result = asyncio.run(handle_ocr_request({"image_hash": "unknown", "image_data": ""}))
        assert result["cached"] is False

    def test_both_ocr_fail_returns_ok(self):
        """When both engines fail, status is still 'ok' with empty text."""
        _ocr_cache.clear()

        # Provide a small image data (1x1 pixel white PNG)
        tiny_png = base64.b64encode(
            b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01"
            b"\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde\x00"
            b"\x00\x00\x0cIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00"
            b"\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82"
        ).decode()

        with patch("app.ocr.extract_text_tesseract", return_value=""):
            result = asyncio.run(
                handle_ocr_request(
                    {
                        "image_hash": "test-fail",
                        "image_data": tiny_png,
                    }
                )
            )
            assert result["status"] == "ok"

    def test_store_and_check_cache(self):
        """Store and retrieve from cache."""
        _ocr_cache.clear()

        asyncio.run(store_cache("hash1", "extracted text", "tesseract"))
        text = asyncio.run(check_cache("hash1"))
        assert text == "extracted text"

        _ocr_cache.clear()

    @patch("app.ocr.extract_text_tesseract", return_value="short")
    @patch("app.ocr.extract_text_ollama", return_value="This is the full OCR text from Ollama")
    def test_ollama_fallback(self, mock_ollama, mock_tesseract):
        """Tesseract < 10 chars triggers Ollama fallback."""
        _ocr_cache.clear()

        tiny_png = base64.b64encode(b"fake image data").decode()

        with patch.dict(os.environ, {"OLLAMA_URL": "http://localhost:11434"}):
            result = asyncio.run(
                handle_ocr_request(
                    {
                        "image_hash": "fallback-test",
                        "image_data": tiny_png,
                    }
                )
            )
            assert result["ocr_engine"] == "ollama"
            assert "Ollama" in result["text"]

        _ocr_cache.clear()


# --- Security Tests ---


class TestSecurityOCR:
    """Security tests for the OCR pipeline."""

    def test_ssrf_javascript_scheme_rejected(self):
        """SSRF: javascript: scheme in OLLAMA_URL must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("javascript:alert(1)")

    def test_ssrf_file_scheme_rejected(self):
        """SSRF: file:// scheme in OLLAMA_URL must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("file:///etc/passwd")

    def test_ssrf_ftp_scheme_rejected(self):
        """SSRF: ftp:// scheme in OLLAMA_URL must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("ftp://evil.com/payload")

    def test_ssrf_gopher_scheme_rejected(self):
        """SSRF: gopher:// scheme in OLLAMA_URL must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("gopher://internal:25")

    def test_ssrf_empty_scheme_rejected(self):
        """SSRF: empty/missing scheme in OLLAMA_URL must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("no-scheme-at-all")

    def test_ssrf_no_hostname_rejected(self):
        """SSRF: URL with no hostname must be rejected."""
        import pytest

        with pytest.raises(ValueError, match="valid hostname"):
            _validate_ollama_url("http://")

    def test_valid_http_allowed(self):
        """Valid http URL passes validation."""
        assert _validate_ollama_url("http://localhost:11434") == "http://localhost:11434"

    def test_valid_https_allowed(self):
        """Valid https URL passes validation."""
        assert _validate_ollama_url("https://ollama.internal:443") == "https://ollama.internal:443"

    def test_image_size_limit(self):
        """Oversized base64 image data must be rejected."""
        _ocr_cache.clear()
        huge_data = "A" * (10 * 1024 * 1024 + 1)
        result = asyncio.run(
            handle_ocr_request({"image_hash": "huge", "image_data": huge_data})
        )
        assert result["status"] == "error"
        assert "too large" in result["error"]


class TestSecurityKeepBridge:
    """Security tests for the Keep bridge."""

    def test_error_response_does_not_leak_credentials(self):
        """Auth error NATS response must NOT contain raw exception details."""
        import app.keep_bridge as bridge

        bridge._keep_session = None
        bridge._session_email = None
        bridge._session_authenticated_at = 0.0

        with patch.dict(
            os.environ,
            {"KEEP_GOOGLE_EMAIL": "", "KEEP_GOOGLE_APP_PASSWORD": ""},
        ):
            result = asyncio.run(handle_sync_request({"cursor": ""}))
            assert result["status"] == "error"
            err_msg = result["error"]
            # Must be a sanitized message, not a raw exception string
            assert "KEEP_GOOGLE_APP_PASSWORD" not in err_msg
            assert "password" not in err_msg.lower() or err_msg == "gkeepapi authentication failed"

    def test_sync_retry_error_does_not_leak_internals(self):
        """Sync failure after retry must use sanitized error message."""
        import time

        import app.keep_bridge as bridge

        mock_keep = MagicMock()
        mock_keep.sync.side_effect = Exception("internal timeout at db:5432 connection refused")
        bridge._keep_session = mock_keep
        bridge._session_email = "test@example.com"
        bridge._session_authenticated_at = time.time()

        with patch.dict(
            os.environ,
            {"KEEP_GOOGLE_EMAIL": "test@example.com", "KEEP_GOOGLE_APP_PASSWORD": "secret123"},
        ):
            with patch.object(bridge, "authenticate", return_value=mock_keep):
                result = asyncio.run(handle_sync_request({"cursor": ""}))
                assert result["status"] == "error"
                err_msg = result["error"]
                # Must NOT contain internal infrastructure details
                assert "db:5432" not in err_msg
                assert "connection refused" not in err_msg

        bridge._keep_session = None
        bridge._session_email = None
        bridge._session_authenticated_at = 0.0
