"""Scope 038 drive extraction tests.

These tests pin the provider-neutral extraction contract before the Go
pipeline consumes it: every supported file type must produce searchable text,
and unprocessable files must surface an explicit state, reason, and action.
"""

import asyncio
import base64
import io
import zipfile

from app.drive_extract import handle_drive_extract_request


def _b64(data: bytes) -> str:
    return base64.b64encode(data).decode("ascii")


def _docx_bytes(text: str) -> bytes:
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as archive:
        archive.writestr(
            "word/document.xml",
            f"""<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>{text}</w:t></w:r></w:p></w:body>
</w:document>""",
        )
    return buffer.getvalue()


def test_drive_extract_routes_pdf_image_office_audio_and_text():
    fixtures = [
        {
            "file_id": "plain-note",
            "mime_type": "text/plain",
            "file_name": "farmers-market.txt",
            "file_bytes_b64": _b64(b"Farmers market list: tomatoes, basil, olive oil"),
            "expected_text": "tomatoes",
            "expected_route": "text",
        },
        {
            "file_id": "pdf-note",
            "mime_type": "application/pdf",
            "file_name": "pdf-with-text.pdf",
            "file_bytes_b64": _b64(b"%PDF-1.4\n1 0 obj\n(Recipe binder: risotto porcini stock)\nendobj\n%%EOF"),
            "expected_text": "risotto",
            "expected_route": "pdf_text",
        },
        {
            "file_id": "scanned-pdf",
            "mime_type": "application/pdf",
            "file_name": "scanned-receipt.pdf",
            "file_bytes_b64": _b64(b"%PDF-1.4\n/SMACKEREL_OCR_TEXT(Receipt total 42.13 grocery)\n%%EOF"),
            "expected_text": "42.13",
            "expected_route": "pdf_ocr",
        },
        {
            "file_id": "image-note",
            "mime_type": "image/svg+xml",
            "file_name": "whiteboard.svg",
            "file_bytes_b64": _b64(b"<svg><text>Launch action item: send catering quote</text></svg>"),
            "expected_text": "catering quote",
            "expected_route": "image_ocr",
        },
        {
            "file_id": "office-note",
            "mime_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            "file_name": "kitchen-notes.docx",
            "file_bytes_b64": _b64(_docx_bytes("Kitchen remodel measurements and cabinet list")),
            "expected_text": "cabinet list",
            "expected_route": "office_document",
        },
        {
            "file_id": "audio-note",
            "mime_type": "audio/webm",
            "file_name": "voice-memo.webm",
            "file_bytes_b64": _b64(b"WEBM\nTRANSCRIPT: Buy chickpeas, parsley, lemons for dinner\n"),
            "expected_text": "chickpeas",
            "expected_route": "audio_transcript",
        },
    ]

    for fixture in fixtures:
        result = asyncio.run(
            handle_drive_extract_request(
                {
                    "artifact_id": "artifact-" + fixture["file_id"],
                    "provider_file_id": fixture["file_id"],
                    "mime_type": fixture["mime_type"],
                    "file_name": fixture["file_name"],
                    "size_bytes": len(base64.b64decode(fixture["file_bytes_b64"])),
                    "file_bytes_b64": fixture["file_bytes_b64"],
                    "max_size_bytes": 1024 * 1024,
                }
            )
        )

        assert result["success"] is True
        assert result["extraction_state"] == "extracted"
        assert fixture["expected_text"] in result["text"]
        assert result["extraction_route"] == fixture["expected_route"]
        assert result["provider_file_id"] == fixture["file_id"]
        assert result["skip_reason"] == ""


def test_drive_extract_oversized_file_is_skipped_with_action_not_silent_success():
    result = asyncio.run(
        handle_drive_extract_request(
            {
                "artifact_id": "artifact-too-large",
                "provider_file_id": "too-large",
                "mime_type": "application/pdf",
                "file_name": "oversized-contract.pdf",
                "size_bytes": 2048,
                "file_bytes_b64": _b64(b"x" * 2048),
                "max_size_bytes": 64,
            }
        )
    )

    assert result["success"] is False
    assert result["extraction_state"] == "skipped"
    assert result["skip_reason"] == "file_too_large"
    assert result["recommended_action"]
    assert result["text"] == ""
