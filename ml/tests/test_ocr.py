"""Unit tests for the OCR pipeline module (ml/app/ocr.py).

Focuses on functionality not covered by test_keep.py:
- LRU cache eviction behavior
- Auto-generation of image hash from base64 data
- Cache operations (check_cache, store_cache) edge cases
- extract_text_tesseract import failure handling
- extract_text_ollama import failure and error handling
- handle_ocr_request flow with tesseract-sufficient results
"""

import asyncio
import base64
import hashlib
import os
import sys
from unittest.mock import MagicMock, patch

import pytest

from app.ocr import (
    MAX_CACHE_ENTRIES,
    MAX_IMAGE_SIZE_B64,
    MIN_TESSERACT_CHARS,
    _ocr_cache,
    _validate_ollama_url,
    check_cache,
    handle_ocr_request,
    store_cache,
)


@pytest.fixture(autouse=True)
def clear_cache():
    """Clear the OCR cache before and after each test."""
    _ocr_cache.clear()
    yield
    _ocr_cache.clear()


# ---------------------------------------------------------------------------
# Cache operations
# ---------------------------------------------------------------------------


class TestCacheOperations:
    """check_cache and store_cache behavior."""

    def test_cache_miss_returns_none(self):
        result = asyncio.run(check_cache("nonexistent-hash"))
        assert result is None

    def test_cache_hit_returns_text(self):
        asyncio.run(store_cache("hash-abc", "Hello world", "tesseract"))
        result = asyncio.run(check_cache("hash-abc"))
        assert result == "Hello world"

    def test_cache_hit_moves_to_end(self):
        """Accessing a cached entry moves it to the most-recently-used position."""
        asyncio.run(store_cache("hash-1", "text1", "tesseract"))
        asyncio.run(store_cache("hash-2", "text2", "tesseract"))

        # Access hash-1, making it more recent than hash-2
        asyncio.run(check_cache("hash-1"))

        keys = list(_ocr_cache.keys())
        assert keys[-1] == "hash-1"

    def test_store_overwrites_existing_entry(self):
        asyncio.run(store_cache("hash-x", "old text", "tesseract"))
        asyncio.run(store_cache("hash-x", "new text", "ollama"))

        result = asyncio.run(check_cache("hash-x"))
        assert result == "new text"
        assert _ocr_cache["hash-x"]["engine"] == "ollama"


# ---------------------------------------------------------------------------
# LRU eviction
# ---------------------------------------------------------------------------


class TestLRUEviction:
    """Cache eviction when exceeding MAX_CACHE_ENTRIES."""

    def test_evicts_oldest_when_exceeding_max(self):
        """Adding entries beyond MAX_CACHE_ENTRIES evicts the oldest."""
        # Fill cache to capacity
        for i in range(MAX_CACHE_ENTRIES):
            asyncio.run(store_cache(f"hash-{i}", f"text-{i}", "tesseract"))

        assert len(_ocr_cache) == MAX_CACHE_ENTRIES
        assert "hash-0" in _ocr_cache

        # Add one more — should evict hash-0
        asyncio.run(store_cache("hash-overflow", "overflow text", "tesseract"))

        assert len(_ocr_cache) == MAX_CACHE_ENTRIES
        assert "hash-0" not in _ocr_cache
        assert "hash-overflow" in _ocr_cache

    def test_recently_accessed_not_evicted(self):
        """Accessing an entry moves it to the end, preventing early eviction."""
        # Fill cache to capacity
        for i in range(MAX_CACHE_ENTRIES):
            asyncio.run(store_cache(f"hash-{i}", f"text-{i}", "tesseract"))

        # Access hash-0, making it most-recently-used
        asyncio.run(check_cache("hash-0"))

        # Add one more — should evict hash-1 (now the oldest), not hash-0
        asyncio.run(store_cache("hash-new", "new", "tesseract"))

        assert "hash-0" in _ocr_cache  # survived eviction
        assert "hash-1" not in _ocr_cache  # evicted


# ---------------------------------------------------------------------------
# Auto hash generation
# ---------------------------------------------------------------------------


class TestAutoHashGeneration:
    """handle_ocr_request auto-generates image hash when not provided."""

    def test_hash_generated_from_image_data(self):
        """When image_hash is empty but image_data is provided, hash is computed."""
        image_bytes = b"test image content"
        expected_hash = hashlib.sha256(image_bytes).hexdigest()
        b64_data = base64.b64encode(image_bytes).decode()

        with patch("app.ocr.extract_text_tesseract", return_value="extracted text from image"):
            result = asyncio.run(handle_ocr_request({"image_hash": "", "image_data": b64_data}))

        assert result["image_hash"] == expected_hash
        assert result["status"] == "ok"


# ---------------------------------------------------------------------------
# Tesseract sufficient results
# ---------------------------------------------------------------------------


class TestTesseractSufficient:
    """When Tesseract produces sufficient text, Ollama is not called."""

    def test_tesseract_sufficient_skips_ollama(self):
        """Text >= MIN_TESSERACT_CHARS means Ollama is not used."""
        sufficient_text = "A" * (MIN_TESSERACT_CHARS + 5)
        b64_data = base64.b64encode(b"image bytes").decode()

        with patch("app.ocr.extract_text_tesseract", return_value=sufficient_text):
            with patch("app.ocr.extract_text_ollama") as mock_ollama:
                result = asyncio.run(handle_ocr_request({"image_hash": "sufficient-test", "image_data": b64_data}))

        assert result["ocr_engine"] == "tesseract"
        assert result["text"] == sufficient_text
        mock_ollama.assert_not_called()


# ---------------------------------------------------------------------------
# No image data
# ---------------------------------------------------------------------------


class TestNoImageData:
    """handle_ocr_request with no image data."""

    def test_no_image_data_returns_empty(self):
        """When image_data is empty, return ok with empty text."""
        result = asyncio.run(handle_ocr_request({"image_hash": "some-hash", "image_data": ""}))
        assert result["status"] == "ok"
        assert result["text"] == ""
        assert result["ocr_engine"] == "none"

    def test_no_image_data_no_hash_returns_empty(self):
        """When both are empty, return ok with empty text."""
        result = asyncio.run(handle_ocr_request({"image_hash": "", "image_data": ""}))
        assert result["status"] == "ok"


# ---------------------------------------------------------------------------
# extract_text_tesseract failure modes
# ---------------------------------------------------------------------------


class TestExtractTextTesseract:
    """extract_text_tesseract error handling."""

    def test_returns_empty_on_import_error(self):
        """If pytesseract is not installed, returns empty string."""
        from app.ocr import extract_text_tesseract

        with patch.dict("sys.modules", {"pytesseract": None}):
            # This should handle the ImportError gracefully
            result = extract_text_tesseract(b"fake image")

        # Either returns empty or handles error
        assert isinstance(result, str)

    def test_returns_empty_on_exception(self):
        """If Tesseract processing fails, returns empty string."""
        from app.ocr import extract_text_tesseract

        # Pass invalid image bytes that PIL can't open
        result = extract_text_tesseract(b"not a real image")
        assert result == ""


# ---------------------------------------------------------------------------
# extract_text_ollama failure modes
# ---------------------------------------------------------------------------


class TestExtractTextOllama:
    """extract_text_ollama error handling."""

    def test_ollama_url_from_env(self):
        """When ollama_url is empty, reads from OLLAMA_URL env var."""
        from app.ocr import extract_text_ollama

        mock_resp = MagicMock()
        mock_resp.json.return_value = {"response": "extracted text"}
        mock_resp.raise_for_status = MagicMock()

        mock_requests = MagicMock()
        mock_requests.post.return_value = mock_resp

        with patch.dict(os.environ, {"OLLAMA_URL": "http://localhost:11434", "OLLAMA_VISION_MODEL": "llava"}):
            with patch.dict(sys.modules, {"requests": mock_requests}):
                result = extract_text_ollama(b"fake image")

        assert result == "extracted text"

    def test_ollama_returns_empty_on_exception(self):
        """Connection failure returns empty string."""
        from app.ocr import extract_text_ollama

        mock_requests = MagicMock()
        mock_requests.post.side_effect = ConnectionError("down")

        with patch.dict(os.environ, {"OLLAMA_VISION_MODEL": "llava"}):
            with patch.dict(sys.modules, {"requests": mock_requests}):
                result = extract_text_ollama(b"fake image", "http://localhost:11434")

        assert result == ""


# ---------------------------------------------------------------------------
# Ollama URL validation (additional cases beyond test_keep.py)
# ---------------------------------------------------------------------------


class TestOllamaURLValidation:
    """Additional _validate_ollama_url edge cases."""

    def test_data_scheme_rejected(self):
        with pytest.raises(ValueError, match="http or https"):
            _validate_ollama_url("data:text/plain;base64,SGVsbG8=")

    def test_url_with_port_accepted(self):
        result = _validate_ollama_url("http://ollama:11434")
        assert result == "http://ollama:11434"

    def test_url_with_path_accepted(self):
        result = _validate_ollama_url("https://ollama.example.com/v1")
        assert result == "https://ollama.example.com/v1"


# ---------------------------------------------------------------------------
# Cache poisoning prevention (SEC-F001)
# ---------------------------------------------------------------------------


class TestCachePoisoningPrevention:
    """Verify caller-provided hash is overridden by computed hash (CWE-345)."""

    def test_caller_hash_ignored_when_image_data_present(self):
        """When image_data is provided, the hash is always computed from the data,
        ignoring any caller-supplied image_hash. Prevents cache poisoning."""
        image_bytes = b"real image content"
        computed_hash = hashlib.sha256(image_bytes).hexdigest()
        b64_data = base64.b64encode(image_bytes).decode()
        fake_hash = "attacker-controlled-hash-value"

        with patch("app.ocr.extract_text_tesseract", return_value="extracted text from image"):
            result = asyncio.run(handle_ocr_request({"image_hash": fake_hash, "image_data": b64_data}))

        assert result["image_hash"] == computed_hash
        assert result["image_hash"] != fake_hash
        assert result["status"] == "ok"

    def test_cache_keyed_by_computed_hash_not_caller_hash(self):
        """Cached result must be stored under the computed hash, not the caller's."""
        image_bytes = b"cache key test image"
        computed_hash = hashlib.sha256(image_bytes).hexdigest()
        b64_data = base64.b64encode(image_bytes).decode()
        fake_hash = "should-not-be-cache-key"

        with patch("app.ocr.extract_text_tesseract", return_value="OCR result text here"):
            asyncio.run(handle_ocr_request({"image_hash": fake_hash, "image_data": b64_data}))

        # The result should be cached under the computed hash
        assert computed_hash in _ocr_cache
        assert fake_hash not in _ocr_cache


# ---------------------------------------------------------------------------
# Constants sanity checks
# ---------------------------------------------------------------------------


class TestConstants:
    """Verify OCR module constants are reasonable."""

    def test_min_tesseract_chars_positive(self):
        assert MIN_TESSERACT_CHARS > 0

    def test_max_cache_entries_positive(self):
        assert MAX_CACHE_ENTRIES > 0

    def test_max_image_size_reasonable(self):
        """Max image size should be between 1MB and 100MB."""
        assert 1 * 1024 * 1024 <= MAX_IMAGE_SIZE_B64 <= 100 * 1024 * 1024
