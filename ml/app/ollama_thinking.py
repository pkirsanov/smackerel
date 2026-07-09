"""SST-owned qwen ``thinking`` control for the ML sidecar's structured-JSON
extraction path (BUG-026-007 — the latency half of redteam F2).

``qwen3:30b-a3b`` (the prod ``LLM_MODEL``) is a *thinking* model: by default it
emits a hidden ``<think>…</think>`` reasoning block BEFORE its answer. On the
structured-JSON *extraction* calls — where the output is a machine-parsed JSON
object and the prose reasoning is irrelevant — that block is pure latency. Live
on evo-x2 (both models warm, identical recipe task) it cost ~113s vs ~10s with
thinking disabled, at IDENTICAL 9/9-ingredient accuracy. 113s ≫ the 30s
``DOMAIN_EXTRACTION_TIMEOUT`` in ``domain.py``, so prod domain extraction was
silently timing out into the degraded fallback.

Mechanism — the qwen ``/no_think`` control token in the request messages, NOT a
top-level ``think=False`` param. Several in-scope calls (``nats_client``
search-rerank, ``drive_classify``) route through litellm's LEGACY ``ollama/``
(``/api/generate``) transform, which buries unknown top-level params under
``options`` where Ollama never sees them — the SAME trap ``ollama_keepalive.py``
documents for ``keep_alive``. A top-level ``think=False`` would therefore be a
silent no-op on those routes. ``/no_think`` is route-agnostic (it is prompt text
the model always sees), version-agnostic, a documented no-op on non-qwen models
(e.g. ``gemma3:4b`` in dev), and is ALREADY used in this repo by
``nats_client.py::_handle_generate_digest`` to suppress reasoning on the digest
path.
"""

import os
from typing import Any

# The qwen control token that suppresses the hidden reasoning block. A no-op on
# non-qwen Ollama models, so it is safe to inject on any ``LLM_MODEL``.
NO_THINK_DIRECTIVE = "/no_think"


def resolve_structured_extraction_thinking() -> bool:
    """Return whether qwen ``thinking`` stays ENABLED on the structured-JSON
    extraction path (``ML_STRUCTURED_EXTRACTION_THINKING``).

    Fail-loud per smackerel NO-DEFAULTS (Gate G028): there is NO hardcoded
    default. The value is emitted by ``scripts/commands/config.sh`` from
    ``services.ml.structured_extraction_thinking`` and validated at sidecar
    startup by ``ml/app/main.py::_check_required_config``; a missing / empty /
    invalid value raises here rather than silently substituting a fallback.

    Value semantics (mirrors the ``ML_PROCESSING_DEGRADED_FALLBACK_ENABLED``
    true|false contract):

    - ``"true"``  -> thinking ENABLED (leave the request unchanged).
    - ``"false"`` -> thinking DISABLED (inject ``/no_think``) — the fix's posture.
    - anything else / empty / unset -> ``RuntimeError`` (no guessing).
    """
    raw = os.environ.get("ML_STRUCTURED_EXTRACTION_THINKING", "").strip().lower()
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise RuntimeError(
        "ML_STRUCTURED_EXTRACTION_THINKING is required and must be exactly 'true' or "
        "'false' (SST services.ml.structured_extraction_thinking) — no default; the ml "
        "sidecar refuses to guess qwen's thinking mode on the structured-extraction path"
    )


def _inject_no_think(messages: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Return a COPY of ``messages`` with ``/no_think`` placed at the front of
    the message the model sees first.

    Placement: prefer the system message (applies to the whole turn); if there
    is none (the user-only calls — processor / crosssource / search-rerank /
    drive-classify), fall back to the first user message, matching the existing
    ``_handle_generate_digest`` ``/no_think``-as-first-line precedent. Idempotent:
    it never double-injects. The caller's message objects are not mutated.
    """
    adjusted = [dict(m) for m in messages]
    target = next((m for m in adjusted if m.get("role") == "system"), None)
    if target is None:
        target = next((m for m in adjusted if m.get("role") == "user"), None)
    if target is None:
        # Degenerate shape (no system, no user) — inject a system directive so
        # the token is never silently lost.
        adjusted.insert(0, {"role": "system", "content": NO_THINK_DIRECTIVE})
        return adjusted
    content = target.get("content") or ""
    if NO_THINK_DIRECTIVE not in content:
        target["content"] = f"{NO_THINK_DIRECTIVE}\n{content}" if content else NO_THINK_DIRECTIVE
    return adjusted


def apply_structured_extraction_thinking(
    messages: list[dict[str, Any]],
    provider: str,
) -> list[dict[str, Any]]:
    """Return ``messages`` adjusted for the SST structured-extraction thinking
    posture.

    - Non-ollama provider -> returned UNCHANGED (thinking-mode is an
      Ollama/qwen concept; hosted providers are untouched).
    - SST thinking ENABLED (``true``) -> returned UNCHANGED.
    - SST thinking DISABLED (``false``) + ollama -> ``/no_think`` injected so
      qwen3 skips its hidden reasoning block (~113s -> ~10s, inside the 30s
      domain budget). A no-op on non-qwen Ollama models.

    The SST resolver is consulted ONLY on the ollama path, so a hosted-provider
    deployment never reads ``ML_STRUCTURED_EXTRACTION_THINKING`` here (matching
    the ollama-conditional requirement in ``_check_required_config``).
    """
    if provider != "ollama":
        return messages
    if resolve_structured_extraction_thinking():
        return messages
    return _inject_no_think(messages)
