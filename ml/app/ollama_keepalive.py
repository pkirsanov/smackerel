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

import json
import os
import re
from dataclasses import dataclass
from typing import Any


class OllamaProfileConfigError(RuntimeError):
    """Raised when SST cannot produce a bounded Ollama inference request."""

    def __init__(self, category: str, detail: str):
        self.category = category
        super().__init__(f"{detail} category={category}")


@dataclass(frozen=True)
class OllamaRequestProfile:
    """Request-scoped Ollama settings resolved from the SST profile set."""

    model: str
    num_ctx: int
    keep_alive: str


_KEEP_ALIVE_INTEGER_RE = re.compile(r"^\d+$")
_KEEP_ALIVE_DURATION_RE = re.compile(r"^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$")
_KEEP_ALIVE_PART_RE = re.compile(r"(\d+(?:\.\d+)?)(?:ns|us|µs|ms|s|m|h)")


def _normalize_ollama_model(model: str) -> str:
    if not isinstance(model, str) or not model.strip():
        raise OllamaProfileConfigError(
            "invalid_model",
            "Ollama request profile key=model model=<redacted> expected=non-empty string",
        )
    normalized = model.strip()
    for prefix in ("ollama_chat/", "ollama/"):
        if normalized.casefold().startswith(prefix):
            normalized = normalized[len(prefix) :].strip()
            break
    if not normalized:
        raise OllamaProfileConfigError(
            "invalid_model",
            "Ollama request profile key=model model=<redacted> expected=non-empty string",
        )
    return normalized


def parse_ollama_model_profiles(raw: str) -> dict[str, int]:
    """Parse one SST profile document into normalized model-to-context entries."""
    try:
        decoded = json.loads(raw)
    except (json.JSONDecodeError, TypeError):
        raise OllamaProfileConfigError(
            "invalid_document",
            "ML_MODEL_MEMORY_PROFILES_JSON key=document expected=valid JSON profile list",
        ) from None
    if not isinstance(decoded, list) or not decoded:
        raise OllamaProfileConfigError(
            "invalid_document",
            "ML_MODEL_MEMORY_PROFILES_JSON key=document expected=non-empty JSON list",
        )

    profiles: dict[str, int] = {}
    for index, entry in enumerate(decoded):
        if not isinstance(entry, dict):
            raise OllamaProfileConfigError(
                "invalid_entry_type",
                f"ML_MODEL_MEMORY_PROFILES_JSON entry={index} expected=object",
            )
        try:
            model = _normalize_ollama_model(entry.get("model"))
        except OllamaProfileConfigError:
            raise OllamaProfileConfigError(
                "invalid_model",
                f"ML_MODEL_MEMORY_PROFILES_JSON entry={index} key=model model=<redacted> expected=non-empty string",
            ) from None
        key = model.casefold()
        if key in profiles:
            raise OllamaProfileConfigError(
                "duplicate_model",
                f"ML_MODEL_MEMORY_PROFILES_JSON entry={index} key=model model=<redacted> "
                "expected=unique normalized model",
            )

        num_ctx = entry.get("num_ctx")
        if isinstance(num_ctx, bool) or not isinstance(num_ctx, int) or num_ctx <= 0:
            raise OllamaProfileConfigError(
                "invalid_num_ctx",
                f"ML_MODEL_MEMORY_PROFILES_JSON entry={index} key=num_ctx model=<redacted> expected=positive integer",
            )
        profiles[key] = num_ctx
    return profiles


def load_ollama_model_profiles() -> dict[str, int]:
    """Load and strictly parse the required SST model profile document."""
    raw_value = os.environ.get("ML_MODEL_MEMORY_PROFILES_JSON")
    if raw_value is None or not raw_value.strip():
        raise OllamaProfileConfigError(
            "missing_document",
            "ML_MODEL_MEMORY_PROFILES_JSON key=document expected=required non-empty profile list",
        )
    return parse_ollama_model_profiles(raw_value)


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
    value = os.environ.get("ML_OLLAMA_KEEP_ALIVE")
    if value is None:
        raise OllamaProfileConfigError(
            "missing_keep_alive",
            "Ollama request profile key=ML_OLLAMA_KEEP_ALIVE expected=required positive duration or integer",
        )
    value = value.strip()
    if not value:
        raise OllamaProfileConfigError(
            "missing_keep_alive",
            "Ollama request profile key=ML_OLLAMA_KEEP_ALIVE expected=required positive duration or integer",
        )
    positive_integer = _KEEP_ALIVE_INTEGER_RE.fullmatch(value) and int(value) > 0
    positive_duration = _KEEP_ALIVE_DURATION_RE.fullmatch(value) and any(
        float(part.group(1)) > 0 for part in _KEEP_ALIVE_PART_RE.finditer(value)
    )
    if not positive_integer and not positive_duration:
        raise OllamaProfileConfigError(
            "invalid_keep_alive",
            "Ollama request profile key=ML_OLLAMA_KEEP_ALIVE expected=positive duration or integer",
        )
    return value


def resolve_ollama_num_ctx(model: str) -> int:
    """Return the required SST-owned per-model Ollama ``num_ctx``.

    Spec 102 SCOPE-102-03 — REPLACES the host-only ``ollama create <tag>
    PARAMETER num_ctx`` tag-overwrite hack (a host mutation outside SST, lost on
    any ``ollama pull`` / host rebuild). The per-model context window is now
    SST-driven (``config/smackerel.yaml`` ``services.ml.model_memory_profiles[].num_ctx``),
    emitted as ``ML_MODEL_MEMORY_PROFILES_JSON``, and threaded onto every ollama
    completion request via ``options.num_ctx`` so the cap is request-level and
    host-independent.

    Missing, malformed, duplicate, invalid, or unmatched profile data raises
    before a caller can perform network I/O. No production Ollama route is
    intentionally unprofiled, so the daemon default is never a valid fallback.
    """
    normalized = _normalize_ollama_model(model)
    profiles = load_ollama_model_profiles()
    try:
        return profiles[normalized.casefold()]
    except KeyError:
        raise OllamaProfileConfigError(
            "missing_model_profile",
            "ML_MODEL_MEMORY_PROFILES_JSON key=model model=<redacted> expected=exactly one matching profile entry",
        ) from None


def resolve_ollama_request_profile(model: str) -> OllamaRequestProfile:
    """Resolve one selected model to all required request-scoped settings."""
    normalized = _normalize_ollama_model(model)
    return OllamaRequestProfile(
        model=normalized,
        num_ctx=resolve_ollama_num_ctx(normalized),
        keep_alive=resolve_ollama_keep_alive(),
    )


def _apply_ollama_profile(payload: dict[str, Any], *, provider: str, model: str) -> dict[str, Any]:
    result = dict(payload)
    if provider.strip().casefold() != "ollama":
        return result

    profile = resolve_ollama_request_profile(model)
    existing_options = result.get("options")
    if existing_options is None:
        options: dict[str, Any] = {}
    elif isinstance(existing_options, dict):
        options = dict(existing_options)
    else:
        raise OllamaProfileConfigError(
            "invalid_options",
            "Ollama request profile key=options expected=object",
        )
    options["num_ctx"] = profile.num_ctx
    result["options"] = options
    result["keep_alive"] = profile.keep_alive
    return result


def apply_ollama_profile_to_litellm(kwargs: dict[str, Any], *, provider: str, model: str) -> dict[str, Any]:
    """Copy and profile LiteLLM kwargs without clobbering caller-owned fields."""
    return _apply_ollama_profile(kwargs, provider=provider, model=model)


def apply_ollama_profile_to_native_json(payload: dict[str, Any], *, provider: str, model: str) -> dict[str, Any]:
    """Copy and profile native Ollama JSON without clobbering caller options."""
    return _apply_ollama_profile(payload, provider=provider, model=model)


def _apply_resolved_ollama_profile(
    payload: dict[str, Any], *, profile: OllamaRequestProfile, model: str
) -> dict[str, Any]:
    if not isinstance(profile, OllamaRequestProfile):
        raise OllamaProfileConfigError(
            "missing_resolved_profile",
            "Ollama network dispatch key=profile model=<redacted> expected=resolved OllamaRequestProfile",
        )
    normalized_model = _normalize_ollama_model(model)
    if profile.model.casefold() != normalized_model.casefold():
        raise OllamaProfileConfigError(
            "profile_model_mismatch",
            "Ollama network dispatch key=profile model=<redacted> expected=selected model match",
        )
    result = dict(payload)
    payload_model = result.get("model")
    if not isinstance(payload_model, str) or not payload_model.strip():
        raise OllamaProfileConfigError(
            "invalid_dispatch_model",
            "Ollama network dispatch key=model model=<redacted> expected=non-empty string",
        )
    if _normalize_ollama_model(payload_model).casefold() != normalized_model.casefold():
        raise OllamaProfileConfigError(
            "profile_model_mismatch",
            "Ollama network dispatch key=model model=<redacted> expected=resolved profile model match",
        )
    existing_options = result.get("options")
    if existing_options is None:
        options: dict[str, Any] = {}
    elif isinstance(existing_options, dict):
        options = dict(existing_options)
    else:
        raise OllamaProfileConfigError(
            "invalid_options",
            "Ollama request profile key=options expected=object",
        )
    options["num_ctx"] = profile.num_ctx
    result["options"] = options
    result["keep_alive"] = profile.keep_alive
    return result


async def dispatch_litellm(
    kwargs: dict[str, Any],
    *,
    provider: str,
    model: str,
    profile: OllamaRequestProfile | None,
    completion_fn: Any | None = None,
    litellm_module: Any | None = None,
) -> Any:
    """Own the production LiteLLM network primitive for every provider.

    Ollama dispatch requires a previously resolved profile and applies it inside
    this function, so callers cannot discard a profiled copy and send the
    original payload. Hosted providers bypass Ollama profiling with
    ``profile=None`` while still sharing the single network boundary.
    """
    normalized_provider = provider.strip().casefold()
    if normalized_provider == "ollama":
        if profile is None:
            raise OllamaProfileConfigError(
                "missing_resolved_profile",
                "Ollama network dispatch key=profile model=<redacted> expected=resolved OllamaRequestProfile",
            )
        dispatch_kwargs = _apply_resolved_ollama_profile(kwargs, profile=profile, model=model)
    else:
        if profile is not None:
            raise OllamaProfileConfigError(
                "hosted_profile_forbidden",
                "Hosted network dispatch key=profile expected=none",
            )
        dispatch_kwargs = dict(kwargs)

    if completion_fn is None:
        if litellm_module is None:
            import litellm as litellm_module

        completion_fn = litellm_module.acompletion
    return await completion_fn(**dispatch_kwargs)


async def dispatch_ollama_native_json_async(
    url: str,
    payload: dict[str, Any],
    *,
    profile: OllamaRequestProfile,
    model: str,
    timeout: float,
    client_factory: Any | None = None,
) -> Any:
    """Own async native ``/api/generate`` dispatch with mandatory profiling."""
    dispatch_payload = _apply_resolved_ollama_profile(payload, profile=profile, model=model)
    if client_factory is None:
        import httpx

        client_factory = httpx.AsyncClient
    async with client_factory(timeout=timeout) as client:
        return await client.post(url, json=dispatch_payload)


def dispatch_ollama_native_json(
    url: str,
    payload: dict[str, Any],
    *,
    profile: OllamaRequestProfile,
    model: str,
    timeout: float,
    post_fn: Any | None = None,
) -> Any:
    """Own synchronous native ``/api/generate`` dispatch with mandatory profiling."""
    dispatch_payload = _apply_resolved_ollama_profile(payload, profile=profile, model=model)
    if post_fn is None:
        import requests

        post_fn = requests.post
    return post_fn(url, json=dispatch_payload, timeout=timeout)


def resolve_domain_output_token_budget() -> int:
    """Return the SST-owned output-token budget (``ML_DOMAIN_OUTPUT_TOKEN_BUDGET``).

    Spec 102 SCOPE-102-03 (BUG-026-006) — REPLACES the hardcoded ``2000`` magic
    number in the structured-JSON domain/synthesis extraction path. BUG-026-006
    root-caused malformed-JSON capture drops partly to an under-provisioned
    output budget truncating the JSON mid-object; making the budget SST-owned
    lets the operator size it against the model without a code change.

    Fail-loud per smackerel NO-DEFAULTS (Gate G028): there is NO hardcoded
    default. The value is emitted by ``scripts/commands/config.sh`` from
    ``services.ml.domain_output_token_budget``; a missing / empty / invalid
    value raises here rather than silently substituting a fallback. Callers use
    it as the SST fallback for a prompt contract that does not set its own
    ``token_budget``.
    """
    value = os.environ.get("ML_DOMAIN_OUTPUT_TOKEN_BUDGET")
    if value is None:
        raise RuntimeError(
            "ML_DOMAIN_OUTPUT_TOKEN_BUDGET is required (SST services.ml.domain_output_token_budget) — "
            "no default; the ml sidecar refuses to guess an output-token budget"
        )
    value = value.strip()
    if not value:
        raise RuntimeError(
            "ML_DOMAIN_OUTPUT_TOKEN_BUDGET is required (SST services.ml.domain_output_token_budget) — "
            "no default; the ml sidecar refuses to guess an output-token budget"
        )
    try:
        budget = int(value)
    except ValueError:
        raise RuntimeError(
            "ML_DOMAIN_OUTPUT_TOKEN_BUDGET key=value expected=positive integer category=invalid_output_budget"
        ) from None
    if budget < 1:
        raise RuntimeError(
            "ML_DOMAIN_OUTPUT_TOKEN_BUDGET key=value expected=integer >= 1 category=invalid_output_budget"
        )
    return budget
