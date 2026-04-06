"""YouTube transcript fetcher using youtube-transcript-api."""

import logging
from typing import Any

logger = logging.getLogger("smackerel-ml.youtube")


async def fetch_transcript(video_id: str) -> dict[str, Any]:
    """Fetch transcript for a YouTube video.

    Returns dict with 'text' (full transcript), 'segments' (timed segments),
    and 'metadata' (video info).
    """
    try:
        from youtube_transcript_api import YouTubeTranscriptApi

        transcript_list = YouTubeTranscriptApi.list_transcripts(video_id)

        # Prefer manually created transcripts, fall back to auto-generated
        transcript = None
        try:
            transcript = transcript_list.find_manually_created_transcript(["en"])
        except Exception:
            try:
                transcript = transcript_list.find_generated_transcript(["en"])
            except Exception:
                # Try any available language
                for t in transcript_list:
                    transcript = t
                    break

        if transcript is None:
            return {"success": False, "error": "No transcript available"}

        segments = transcript.fetch()
        full_text = " ".join(seg.get("text", seg.get("snippet", "")) if isinstance(seg, dict) else str(seg) for seg in segments)

        return {
            "success": True,
            "text": full_text,
            "segments": segments[:100],  # Limit segments count
            "language": getattr(transcript, "language_code", "en"),
        }

    except Exception as e:
        logger.error("YouTube transcript fetch failed for %s: %s", video_id, e)
        return {"success": False, "error": str(e)}
