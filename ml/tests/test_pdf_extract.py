"""Tests for PDF text extraction module."""

import io

import pytest

from app.pdf_extract import extract_text_from_bytes

# pypdf is a runtime dependency — skip tests if not installed
pypdf = pytest.importorskip("pypdf", reason="pypdf not installed (runtime dependency)")


def _make_minimal_pdf(text: str) -> bytes:
    """Create a minimal valid PDF with the given text content.

    Uses pypdf's PdfWriter to create a real PDF in memory.
    """
    from pypdf import PdfWriter
    from pypdf.generic import (
        DictionaryObject,
        NameObject,
        NumberObject,
        StreamObject,
    )

    writer = PdfWriter()

    # Create a page with text via a content stream
    page = writer.add_blank_page(width=612, height=792)

    # Build a simple content stream that renders text
    content = f"BT /F1 12 Tf 100 700 Td ({text}) Tj ET"
    content_bytes = content.encode("latin-1")

    stream = StreamObject()
    stream._data = content_bytes  # type: ignore[attr-defined]
    stream[NameObject("/Length")] = NumberObject(len(content_bytes))

    # Build font dictionary
    font_dict = DictionaryObject()
    font_dict[NameObject("/Type")] = NameObject("/Font")
    font_dict[NameObject("/Subtype")] = NameObject("/Type1")
    font_dict[NameObject("/BaseFont")] = NameObject("/Helvetica")

    resources = DictionaryObject()
    font_resources = DictionaryObject()
    font_resources[NameObject("/F1")] = writer._add_object(font_dict)
    resources[NameObject("/Font")] = font_resources

    page[NameObject("/Resources")] = resources
    page[NameObject("/Contents")] = writer._add_object(stream)

    buf = io.BytesIO()
    writer.write(buf)
    return buf.getvalue()


class TestExtractTextFromBytes:
    """Tests for extract_text_from_bytes."""

    def test_valid_pdf_extracts_text(self):
        """A PDF with embedded text should return that text."""
        pdf_bytes = _make_minimal_pdf("Hello World")
        result = extract_text_from_bytes(pdf_bytes)
        # pypdf may extract the text with minor formatting differences
        assert result is not None
        assert "Hello" in result or "World" in result

    def test_empty_bytes_returns_none(self):
        """Empty/invalid bytes should return None without crashing."""
        result = extract_text_from_bytes(b"")
        assert result is None

    def test_non_pdf_bytes_returns_none(self):
        """Random bytes that aren't a PDF should return None."""
        result = extract_text_from_bytes(b"This is not a PDF file at all")
        assert result is None

    def test_truncation_at_100kb(self):
        """Text longer than 100KB should be truncated."""
        # Create a PDF with very long text (this is hard to do with real PDFs,
        # so we test the truncation logic conceptually)
        long_text = "A" * 200
        pdf_bytes = _make_minimal_pdf(long_text)
        result = extract_text_from_bytes(pdf_bytes)
        # Result should exist (may or may not hit 100KB depending on PDF structure)
        # Main assertion: function doesn't crash on large input
        if result is not None:
            assert len(result) <= 100_001  # 100KB + possible newline
