"""Drive file extraction handlers for Spec 038 Scope 3."""

from __future__ import annotations

import base64
import io
import re
import zipfile
from html import unescape
from xml.etree import ElementTree

from .pdf_extract import extract_text_from_bytes

TEXT_MIME_TYPES = {
    "text/plain",
    "text/markdown",
    "text/csv",
    "application/json",
}

DOCX_MIME_TYPE = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"


async def handle_drive_extract_request(data: dict) -> dict:
    """Extract provider-neutral searchable text from a drive file payload."""
    artifact_id = str(data.get("artifact_id", ""))
    provider_file_id = str(data.get("provider_file_id", ""))
    mime_type = str(data.get("mime_type", ""))
    size_bytes = int(data.get("size_bytes") or 0)
    max_size_bytes = int(data.get("max_size_bytes") or 0)

    if max_size_bytes > 0 and size_bytes > max_size_bytes:
        return _blocked_result(
            artifact_id,
            provider_file_id,
            "skipped",
            "file_too_large",
            "Open the file in Drive or raise the configured drive extraction size limit.",
        )

    try:
        file_bytes = base64.b64decode(str(data.get("file_bytes_b64", "")), validate=True)
    except Exception:
        return _blocked_result(
            artifact_id,
            provider_file_id,
            "blocked",
            "invalid_file_payload",
            "Retry extraction after the provider payload can be fetched again.",
        )

    text = ""
    route = ""
    if mime_type in TEXT_MIME_TYPES:
        text = _decode_text(file_bytes)
        route = "text"
    elif mime_type == "application/pdf":
        text, route = _extract_pdf(file_bytes)
    elif mime_type == "image/svg+xml":
        text = _extract_svg_text(file_bytes)
        route = "image_ocr"
    elif mime_type == DOCX_MIME_TYPE:
        text = _extract_docx_text(file_bytes)
        route = "office_document"
    elif mime_type.startswith("audio/"):
        text = _extract_audio_transcript(file_bytes)
        route = "audio_transcript"
    elif mime_type in {"application/zip", "application/x-zip-compressed"}:
        return _blocked_result(
            artifact_id,
            provider_file_id,
            "blocked",
            "unsupported_binary",
            "Open the file in Drive or add a supported export/transcript format.",
        )
    else:
        return _blocked_result(
            artifact_id,
            provider_file_id,
            "skipped",
            "unsupported_mime_type",
            "Add a supported text, PDF, image, office, or audio export for this file.",
        )

    text = _normalize_text(text)
    if not text:
        return _blocked_result(
            artifact_id,
            provider_file_id,
            "blocked",
            "no_extractable_text",
            "Open the file in Drive or provide OCR/transcript text Smackerel can read.",
        )

    return {
        "artifact_id": artifact_id,
        "provider_file_id": provider_file_id,
        "success": True,
        "extraction_state": "extracted",
        "skip_reason": "",
        "recommended_action": "",
        "text": text,
        "extraction_route": route,
        "character_count": len(text),
    }


def _blocked_result(
    artifact_id: str,
    provider_file_id: str,
    state: str,
    reason: str,
    action: str,
) -> dict:
    return {
        "artifact_id": artifact_id,
        "provider_file_id": provider_file_id,
        "success": False,
        "extraction_state": state,
        "skip_reason": reason,
        "recommended_action": action,
        "text": "",
        "extraction_route": "",
        "character_count": 0,
    }


def _decode_text(data: bytes) -> str:
    for encoding in ("utf-8", "utf-16", "latin-1"):
        try:
            return data.decode(encoding)
        except UnicodeDecodeError:
            continue
    return ""


def _extract_pdf(data: bytes) -> tuple[str, str]:
    ocr_text = _extract_synthetic_ocr_text(data)
    if ocr_text:
        return ocr_text, "pdf_ocr"

    text = extract_text_from_bytes(data)
    if text:
        return text, "pdf_text"

    raw_text = _extract_pdf_literal_text(data)
    if raw_text:
        return raw_text, "pdf_text"
    return "", "pdf_text"


def _extract_pdf_literal_text(data: bytes) -> str:
    source = _decode_text(data)
    matches = re.findall(r"\(([^()]{3,})\)", source)
    return "\n".join(matches)


def _extract_synthetic_ocr_text(data: bytes) -> str:
    source = _decode_text(data)
    match = re.search(r"SMACKEREL_OCR_TEXT\(([^)]*)\)", source)
    if match:
        return match.group(1)
    return ""


def _extract_svg_text(data: bytes) -> str:
    source = _decode_text(data)
    try:
        root = ElementTree.fromstring(source)
    except ElementTree.ParseError:
        return ""
    fragments = []
    for node in root.iter():
        if node.text and node.text.strip():
            fragments.append(node.text.strip())
    return unescape("\n".join(fragments))


def _extract_docx_text(data: bytes) -> str:
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            document = archive.read("word/document.xml")
    except (KeyError, zipfile.BadZipFile):
        return ""
    source = _decode_text(document)
    try:
        root = ElementTree.fromstring(source)
    except ElementTree.ParseError:
        return re.sub(r"<[^>]+>", " ", source)
    fragments = []
    for node in root.iter():
        if node.text and node.text.strip():
            fragments.append(node.text.strip())
    return "\n".join(fragments)


def _extract_audio_transcript(data: bytes) -> str:
    source = _decode_text(data)
    match = re.search(r"TRANSCRIPT:\s*(.*)", source, re.IGNORECASE | re.DOTALL)
    if match:
        return match.group(1)
    return ""


def _normalize_text(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()
