"""SST-owned qwen ``thinking`` control for the ML sidecar's structured-JSON
extraction path (BUG-026-007 — the latency half of redteam F2).

``qwen3:30b-a3b`` (the prod ``LLM_MODEL``) is a *thinking* model: by default it
emits a hidden ``<think>…</think>`` reasoning block BEFORE its answer. On the
structured-JSON *extraction* calls — where the output is a machine-parsed JSON
object and the prose reasoning is irrelevant — that block is pure latency. Live
on evo-x2 (shared daemon, warm qwen3) it cost >150s with thinking on vs ~1s of
compute with thinking disabled, at IDENTICAL accuracy. That is ≫ the 30s
``DOMAIN_EXTRACTION_TIMEOUT`` in ``domain.py`` (so domain extraction silently
timed out into the degraded fallback) and it prefixes the JSON callers with a
``<think>`` block that trips ``LLM returned invalid JSON`` — the F2 failure.

Mechanism — the NATIVE Ollama ``think`` request field, NOT the ``/no_think``
prompt-injection token the FIRST fix used. qwen3's Ollama chat template IGNORES
a ``/no_think`` string in the messages (measured live on evo-x2: ``/no_think``
-> thinking STILL on, >150s, invalid-JSON output); it honors ONLY the native
``think`` field on the ``/api/chat`` request body (native ``think=False`` ->
thinking OFF, ~1s compute, valid JSON).

litellm forwarding — verified against the sidecar-pinned ``litellm==1.84.0``
(``ml/requirements.txt``) with an empirical request-capture probe recorded in the
BUG-026-007 report: a TOP-LEVEL ``think=`` kwarg on ``litellm.acompletion`` is
placed at the Ollama request TOP LEVEL (``data["think"]``) by BOTH the
``ollama_chat/`` (``/api/chat``) and the legacy ``ollama/`` (``/api/generate``)
transforms — ``litellm/llms/ollama/{chat,completion}/transformation.py`` do
``think = optional_params.pop("think", None); if think is not None: data["think"]
= think``. (Unlike ``keep_alive``, which the legacy generate transform buries
under ``options``; and unlike ``reasoning_effort``, which litellm maps to
``think=True`` for ``"low"|"medium"|"high"`` — the wrong direction here.) The
in-scope callers therefore set the top-level ``think`` kwarg AND route Ollama
through the ``ollama_chat/`` prefix (for role fidelity, ``keep_alive`` parity,
and consistency with the other structured sites).
"""

import os
from typing import Any


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
    - ``"false"`` -> thinking DISABLED (send native ``think=False``) — the fix's
      posture.
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


def apply_structured_extraction_thinking(
    completion_kwargs: dict[str, Any],
    provider: str,
) -> dict[str, Any]:
    """Mutate ``completion_kwargs`` in place for the SST structured-extraction
    thinking posture and return it (for call-site chaining).

    - Non-ollama provider -> UNCHANGED (thinking-mode is an Ollama/qwen concept;
      hosted providers are untouched and the SST resolver is never consulted).
    - SST thinking ENABLED (``true``) -> UNCHANGED (qwen keeps its default
      thinking-on behavior; no ``think`` key is added).
    - SST thinking DISABLED (``false``) + ollama -> set the native
      ``completion_kwargs["think"] = False`` so qwen3 skips its hidden reasoning
      block (>150s -> ~1s compute, inside the 30s domain budget). A no-op on
      non-qwen Ollama models (they ignore the field).

    ``think=False`` is a TOP-LEVEL litellm kwarg: ``litellm==1.84.0`` forwards it
    to the Ollama request top level (``data["think"]``) for both the
    ``ollama_chat/`` and legacy ``ollama/`` transforms (verified — see the module
    docstring / the BUG-026-007 report probe). Callers route Ollama through
    ``ollama_chat/`` so system/user roles round-trip natively and ``keep_alive``
    also lands top-level.

    The SST resolver is consulted ONLY on the ollama path, so a hosted-provider
    deployment never reads ``ML_STRUCTURED_EXTRACTION_THINKING`` here (matching
    the ollama-conditional requirement in ``_check_required_config``).
    """
    if provider != "ollama":
        return completion_kwargs
    if resolve_structured_extraction_thinking():
        return completion_kwargs
    completion_kwargs["think"] = False
    return completion_kwargs
