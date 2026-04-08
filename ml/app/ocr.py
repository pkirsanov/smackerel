"""OCR pipeline for Google Keep image notes.

Handles keep.ocr.request NATS messages. Uses Tesseract as primary OCR engine
with Ollama vision as fallback when Tesseract produces insufficient results.
Results are cached by image content hash (SHA-256).
"""

import hashlib
import io
import logging
import os
from typing import Optional

logger = logging.getLogger("smackerel-ml.ocr")

# In-memory OCR cache (production would use the ocr_cache DB table)
_ocr_cache: dict[str, dict] = {}

MIN_TESSERACT_CHARS = 10

# Maximum base64 image size: 10 MB base64 ≈ 7.5 MB decoded image
MAX_IMAGE_SIZE_B64 = 10 * 1024 * 1024

# Prevent PIL decompression bombs (25 megapixels)
try:
    from PIL import Image as _PILImage

    _PILImage.MAX_IMAGE_PIXELS = 25_000_000
except ImportError:
    pass


def extract_text_tesseract(image_bytes: bytes) -> str:
    """Extract text from image bytes using Tesseract OCR.

    Args:
        image_bytes: Raw image data.

    Returns:
        Extracted text string, empty if Tesseract fails.
    """
    try:
        import pytesseract
        from PIL import Image

        image = Image.open(io.BytesIO(image_bytes))
        text = pytesseract.image_to_string(image)
        return text.strip()
    except ImportError:
        logger.warning("pytesseract or Pillow not installed — skipping Tesseract OCR")
        return ""
    except Exception as exc:
        logger.warning("Tesseract OCR failed: %s", exc)
        return ""


def extract_text_ollama(image_bytes: bytes, ollama_url: str = "") -> str:
    """Extract text from image bytes using Ollama vision model.

    Args:
        image_bytes: Raw image data.
        ollama_url: URL of the Ollama API server.

    Returns:
        Extracted text string, empty if Ollama fails.
    """
    if not ollama_url:
        ollama_url = os.environ.get("OLLAMA_URL")
        if not ollama_url:
            ollama_url = "http://ollama:11434"  # Docker Compose service default

    try:
        import base64

        import requests

        b64_image = base64.b64encode(image_bytes).decode("utf-8")

        response = requests.post(
            f"{ollama_url}/api/generate",
            json={
                "model": os.environ.get("OLLAMA_VISION_MODEL") or "llava",
                "prompt": "Extract all visible text from this image. Return only the text content, no commentary.",
                "images": [b64_image],
                "stream": False,
            },
            timeout=60,
        )
        response.raise_for_status()
        result = response.json()
        return result.get("response", "").strip()
    except ImportError:
        logger.warning("requests not installed — skipping Ollama OCR")
        return ""
    except Exception as exc:
        logger.warning("Ollama vision OCR failed: %s", exc)
        return ""


async def check_cache(image_hash: str) -> Optional[str]:
    """Check if OCR result exists in cache.

    Args:
        image_hash: SHA-256 hash of the image.

    Returns:
        Cached extracted text, or None if not cached.
    """
    cached = _ocr_cache.get(image_hash)
    if cached:
        return cached["text"]
    return None


async def store_cache(image_hash: str, text: str, engine: str) -> None:
    """Store OCR result in cache.

    Args:
        image_hash: SHA-256 hash of the image.
        text: Extracted text.
        engine: OCR engine used ("tesseract" or "ollama").
    """
    _ocr_cache[image_hash] = {"text": text, "engine": engine}


async def handle_ocr_request(data: dict) -> dict:
    """Handle a keep.ocr.request NATS message.

    Args:
        data: Request payload with 'image_data' (base64) and 'image_hash' fields.

    Returns:
        Response dict with 'status', 'text', 'cached', and 'ocr_engine'.
    """
    import base64

    image_hash = data.get("image_hash", "")
    image_data_b64 = data.get("image_data", "")

    if image_data_b64 and len(image_data_b64) > MAX_IMAGE_SIZE_B64:
        return {
            "status": "error",
            "text": "",
            "cached": False,
            "ocr_engine": "none",
            "image_hash": image_hash,
            "error": f"image too large: {len(image_data_b64)} bytes base64 exceeds {MAX_IMAGE_SIZE_B64} limit",
        }

    if not image_hash and image_data_b64:
        raw = base64.b64decode(image_data_b64)
        image_hash = hashlib.sha256(raw).hexdigest()

    # Check cache first
    cached_text = await check_cache(image_hash)
    if cached_text is not None:
        logger.info("OCR cache hit for %s", image_hash[:16])
        return {
            "status": "ok",
            "text": cached_text,
            "cached": True,
            "ocr_engine": "cached",
            "image_hash": image_hash,
        }

    # Decode image
    if not image_data_b64:
        return {
            "status": "ok",
            "text": "",
            "cached": False,
            "ocr_engine": "none",
            "image_hash": image_hash,
        }

    image_bytes = base64.b64decode(image_data_b64)

    # Try Tesseract first
    text = extract_text_tesseract(image_bytes)
    engine = "tesseract"

    # If Tesseract produced insufficient text, try Ollama
    if len(text) < MIN_TESSERACT_CHARS:
        ollama_url = os.environ.get("OLLAMA_URL")
        if ollama_url:
            ollama_text = extract_text_ollama(image_bytes, ollama_url)
            if len(ollama_text) > len(text):
                text = ollama_text
                engine = "ollama"

    # Cache result
    await store_cache(image_hash, text, engine)

    logger.info(
        "OCR complete for %s: %d chars via %s",
        image_hash[:16],
        len(text),
        engine,
    )

    return {
        "status": "ok",
        "text": text,
        "cached": False,
        "ocr_engine": engine,
        "image_hash": image_hash,
    }
