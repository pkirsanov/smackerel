"""Whisper transcription for voice notes."""

import logging
import os
import tempfile
from typing import Any

import httpx

logger = logging.getLogger("smackerel-ml.whisper")


async def transcribe_voice(voice_url: str, ollama_url: str | None = None) -> dict[str, Any]:
    """Transcribe a voice note URL using local Whisper or Ollama.

    Downloads the audio file and transcribes it. Falls back to a simple
    HTTP-based approach if Whisper is not available locally.
    """
    try:
        # Download the audio file
        async with httpx.AsyncClient(timeout=30) as client:
            resp = await client.get(voice_url)
            resp.raise_for_status()
            audio_data = resp.content

        # Write to temp file
        with tempfile.NamedTemporaryFile(suffix=".ogg", delete=False) as f:
            f.write(audio_data)
            temp_path = f.name

        try:
            # Try local whisper first
            transcript = await _whisper_local(temp_path)
            if transcript:
                return {"success": True, "text": transcript}

            # Fall back to Ollama if available
            if ollama_url:
                transcript = await _whisper_ollama(temp_path, ollama_url)
                if transcript:
                    return {"success": True, "text": transcript}

            return {"success": False, "error": "No whisper backend available"}

        finally:
            os.unlink(temp_path)

    except httpx.HTTPError as e:
        logger.error("Failed to download voice note: %s", e)
        return {"success": False, "error": f"Download failed: {e}"}
    except Exception as e:
        logger.error("Voice transcription failed: %s", e)
        return {"success": False, "error": str(e)}


async def _whisper_local(audio_path: str) -> str | None:
    """Attempt local Whisper transcription."""
    try:
        import whisper

        model = whisper.load_model("base")
        result = model.transcribe(audio_path)
        return result.get("text", "")
    except ImportError:
        logger.debug("whisper not installed, skipping local transcription")
        return None
    except Exception as e:
        logger.warning("Local whisper failed: %s", e)
        return None


async def _whisper_ollama(audio_path: str, ollama_url: str) -> str | None:
    """Attempt Whisper transcription via Ollama."""
    # Ollama doesn't natively support audio yet; placeholder for future
    logger.debug("Ollama whisper not yet implemented")
    return None
