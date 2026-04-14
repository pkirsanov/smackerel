"""PDF text extraction for the capture pipeline.

Handles PDF content from the general artifacts.process flow (GAP-003).
Downloads PDF from URL and extracts text using pypdf.
"""

import io
import logging
from typing import Optional

import httpx

from .url_validator import SSRFError, validate_fetch_url

logger = logging.getLogger("smackerel-ml.pdf")

# Maximum PDF download size: 50 MB
MAX_PDF_SIZE = 50 * 1024 * 1024


async def extract_pdf_text(url: str) -> Optional[str]:
    """Download a PDF from URL and extract its text content.

    Args:
        url: Public URL pointing to a PDF file.

    Returns:
        Extracted text, or None if extraction failed.
    """
    try:
        # Validate URL against SSRF (SEC-004-002)
        validate_fetch_url(url)

        # Download PDF
        async with httpx.AsyncClient(timeout=60, follow_redirects=True) as client:
            resp = await client.get(url)
            resp.raise_for_status()

            content_length = int(resp.headers.get("content-length", 0))
            if content_length > MAX_PDF_SIZE:
                logger.warning("PDF too large (%d bytes), skipping: %s", content_length, url)
                return None

            pdf_bytes = resp.content
            if len(pdf_bytes) > MAX_PDF_SIZE:
                logger.warning("PDF download exceeded size limit: %s", url)
                return None

        return extract_text_from_bytes(pdf_bytes)

    except SSRFError as e:
        logger.warning("PDF URL blocked by SSRF validation: %s — %s", url, e)
        return None
    except httpx.HTTPError as e:
        logger.error("Failed to download PDF from %s: %s", url, e)
        return None
    except Exception as e:
        logger.error("PDF extraction failed for %s: %s", url, e)
        return None


def extract_text_from_bytes(pdf_bytes: bytes) -> Optional[str]:
    """Extract text from PDF bytes using pypdf.

    Args:
        pdf_bytes: Raw PDF file content.

    Returns:
        Extracted text, or None if no text found.
    """
    try:
        from pypdf import PdfReader

        reader = PdfReader(io.BytesIO(pdf_bytes))
        pages_text = []
        for page in reader.pages:
            text = page.extract_text()
            if text:
                pages_text.append(text.strip())

        if not pages_text:
            logger.info("PDF has no extractable text (may be scanned/image-only)")
            return None

        combined = "\n\n".join(pages_text)
        # Truncate to 100KB to stay within LLM context limits
        if len(combined) > 100_000:
            combined = combined[:100_000]
            logger.info("PDF text truncated to 100KB")

        return combined

    except ImportError:
        logger.error("pypdf not installed — cannot extract PDF text")
        return None
    except Exception as e:
        logger.error("pypdf extraction error: %s", e)
        return None
