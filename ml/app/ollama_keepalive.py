"""SST-owned Ollama ``keep_alive`` resolution for the ML sidecar (F2).

The domain / synthesis / universal-processing completions keep the multi-GB
domain model resident across an active capture session by passing ``keep_alive``
on every Ollama call. ``keep_alive`` is honored by Ollama ONLY when it lands at
the TOP LEVEL of the ``/api/chat`` | ``/api/generate`` request body; litellm
forwards it there for the ``ollama_chat/`` (``/api/chat``) prefix but buries it
under ``options`` for the legacy ``ollama/`` (``/api/generate``) transform, where
Ollama ignores it (verified against litellm 1.59.8 + 1.84.0). The callers
therefore use the ``ollama_chat/`` prefix and read the window from here.
"""

import os


def resolve_ollama_keep_alive() -> str:
    """Return the SST-owned Ollama ``keep_alive`` window (``ML_OLLAMA_KEEP_ALIVE``).

    Fail-loud per smackerel NO-DEFAULTS (Gate G028): there is NO hardcoded
    default. The value is emitted by ``scripts/commands/config.sh`` from
    ``services.ml.ollama_keep_alive`` and validated at sidecar startup by
    ``ml/app/main.py::_check_required_config``; a missing / empty value raises
    here rather than silently substituting a fallback.

    VRAM-envelope note: ``"30m"`` keeps the domain/synthesis model resident
    across a capture session so sparse captures skip the ~22-45s cold-load, but
    lets it unload after 30m idle instead of pinning it forever. Under the knb
    48G model envelope (gemma4:26b ~16 GB + qwen3:30b-a3b ~20 GB <= 48 G) a
    permanent pin (``-1``) would starve the qwen3 assistant, so a bounded window
    is intentional.

    This is the PER-REQUEST window the ml sidecar sends on each Ollama
    completion — distinct from the daemon-level ``OLLAMA_KEEP_ALIVE``
    (``infrastructure.ollama.keep_alive``), which sets smackerel's OWN dev
    ollama container default and feeds the Go core's VRAM-envelope guard. A
    per-request keep_alive overrides the daemon default for that model, so this
    value is what governs residency against the SHARED host ollama daemon in
    prod (where smackerel's own ollama container is off).
    """
    value = os.environ.get("ML_OLLAMA_KEEP_ALIVE", "").strip()
    if not value:
        raise RuntimeError(
            "ML_OLLAMA_KEEP_ALIVE is required (SST services.ml.ollama_keep_alive) — "
            "no default; the ml sidecar refuses to guess an Ollama keep_alive window"
        )
    return value
