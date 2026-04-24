"""Spec 037 — Agent runtime configuration loader for the Python ML sidecar.

The Go core owns scenario routing, allowlist enforcement, and trace
persistence. The Python sidecar only needs the slice of agent config that
governs LLM dispatch (provider routing) and the trace-related toggles that
shape the per-turn payload it emits. All values flow from
config/smackerel.yaml via ./smackerel.sh config generate. There are no
Python-side fallback defaults — missing or empty AGENT_* environment
variables raise AgentConfigError so the sidecar refuses to start.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field

# Empty-string is explicitly allowed for these two opt-out fields per
# design §11 (no fallback scenario / inherit runtime embedding model). Every
# other AGENT_* variable must be present and non-empty.
_OPTIONAL_EMPTY = frozenset(
    {
        "AGENT_ROUTING_FALLBACK_SCENARIO_ID",
        "AGENT_ROUTING_EMBEDDING_MODEL",
    }
)

_REQUIRED_PROVIDER_ROUTES = ("default", "reasoning", "fast", "vision", "ocr")

_REQUIRED_NON_EMPTY = (
    "AGENT_SCENARIO_DIR",
    "AGENT_SCENARIO_GLOB",
    "AGENT_HOT_RELOAD",
    "AGENT_ROUTING_CONFIDENCE_FLOOR",
    "AGENT_ROUTING_CONSIDER_TOP_N",
    "AGENT_TRACE_RETENTION_DAYS",
    "AGENT_TRACE_RECORD_LLM_MESSAGES",
    "AGENT_TRACE_REDACT_MARKER",
    "AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING",
    "AGENT_DEFAULTS_TIMEOUT_MS_CEILING",
    "AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING",
    "AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING",
)


class AgentConfigError(RuntimeError):
    """Raised when AGENT_* environment is missing or malformed."""


@dataclass(frozen=True)
class ProviderRoute:
    provider: str
    model: str


@dataclass(frozen=True)
class AgentConfig:
    scenario_dir: str
    scenario_glob: str
    hot_reload: bool
    routing_confidence_floor: float
    routing_consider_top_n: int
    routing_fallback_scenario_id: str
    routing_embedding_model: str
    trace_retention_days: int
    trace_record_llm_messages: bool
    trace_redact_marker: str
    defaults_max_loop_iterations_ceiling: int
    defaults_timeout_ms_ceiling: int
    defaults_schema_retry_budget_ceiling: int
    defaults_per_tool_timeout_ms_ceiling: int
    provider_routing: dict[str, ProviderRoute] = field(default_factory=dict)


def _parse_bool(key: str, value: str, missing: list[str], bad: list[str]) -> bool:
    if value == "":
        missing.append(key)
        return False
    v = value.strip().lower()
    if v == "true":
        return True
    if v == "false":
        return False
    bad.append(f"{key} (must be true or false, got {value!r})")
    return False


def _parse_int(key: str, value: str, low: int, missing: list[str], bad: list[str]) -> int:
    if value == "":
        missing.append(key)
        return 0
    try:
        n = int(value)
    except ValueError:
        bad.append(f"{key} (must be an integer, got {value!r})")
        return 0
    if n < low:
        bad.append(f"{key} (must be >= {low}, got {n})")
        return 0
    return n


def _parse_float(key: str, value: str, low: float, high: float, missing: list[str], bad: list[str]) -> float:
    if value == "":
        missing.append(key)
        return 0.0
    try:
        f = float(value)
    except ValueError:
        bad.append(f"{key} (must be a float, got {value!r})")
        return 0.0
    if f < low or f > high:
        bad.append(f"{key} (must be in range [{low}, {high}], got {f})")
        return 0.0
    return f


def _require_string(key: str, missing: list[str]) -> str:
    if key not in os.environ:
        missing.append(key)
        return ""
    v = os.environ[key]
    if v == "":
        missing.append(key)
        return ""
    return v


def _allow_empty_string(key: str, missing: list[str]) -> str:
    """Returns the env value (possibly empty). Absent var is fatal."""
    if key not in os.environ:
        missing.append(key)
        return ""
    return os.environ[key]


def load_agent_config() -> AgentConfig:
    """Load AGENT_* config from the environment, fail-loud on any missing or malformed value.

    Collects every problem so the operator sees the full picture in one error.
    """
    missing: list[str] = []
    bad: list[str] = []

    scenario_dir = _require_string("AGENT_SCENARIO_DIR", missing)
    scenario_glob = _require_string("AGENT_SCENARIO_GLOB", missing)
    hot_reload = _parse_bool("AGENT_HOT_RELOAD", os.environ.get("AGENT_HOT_RELOAD", ""), missing, bad)
    confidence_floor = _parse_float(
        "AGENT_ROUTING_CONFIDENCE_FLOOR",
        os.environ.get("AGENT_ROUTING_CONFIDENCE_FLOOR", ""),
        0.0,
        1.0,
        missing,
        bad,
    )
    consider_top_n = _parse_int(
        "AGENT_ROUTING_CONSIDER_TOP_N",
        os.environ.get("AGENT_ROUTING_CONSIDER_TOP_N", ""),
        1,
        missing,
        bad,
    )
    fallback_scenario_id = _allow_empty_string("AGENT_ROUTING_FALLBACK_SCENARIO_ID", missing)
    embedding_model = _allow_empty_string("AGENT_ROUTING_EMBEDDING_MODEL", missing)
    retention_days = _parse_int(
        "AGENT_TRACE_RETENTION_DAYS",
        os.environ.get("AGENT_TRACE_RETENTION_DAYS", ""),
        1,
        missing,
        bad,
    )
    record_llm_messages = _parse_bool(
        "AGENT_TRACE_RECORD_LLM_MESSAGES",
        os.environ.get("AGENT_TRACE_RECORD_LLM_MESSAGES", ""),
        missing,
        bad,
    )
    redact_marker = _require_string("AGENT_TRACE_REDACT_MARKER", missing)
    max_loop_ceiling = _parse_int(
        "AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING",
        os.environ.get("AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING", ""),
        1,
        missing,
        bad,
    )
    timeout_ceiling = _parse_int(
        "AGENT_DEFAULTS_TIMEOUT_MS_CEILING",
        os.environ.get("AGENT_DEFAULTS_TIMEOUT_MS_CEILING", ""),
        1,
        missing,
        bad,
    )
    schema_retry_ceiling = _parse_int(
        "AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING",
        os.environ.get("AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING", ""),
        0,
        missing,
        bad,
    )
    per_tool_ceiling = _parse_int(
        "AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING",
        os.environ.get("AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING", ""),
        1,
        missing,
        bad,
    )

    provider_routing: dict[str, ProviderRoute] = {}
    for name in _REQUIRED_PROVIDER_ROUTES:
        upper = name.upper()
        provider = _require_string(f"AGENT_PROVIDER_{upper}_PROVIDER", missing)
        model = _require_string(f"AGENT_PROVIDER_{upper}_MODEL", missing)
        provider_routing[name] = ProviderRoute(provider=provider, model=model)

    if missing or bad:
        parts: list[str] = []
        if missing:
            parts.append("missing required agent configuration: " + ", ".join(missing))
        if bad:
            parts.append("invalid agent configuration: " + "; ".join(bad))
        raise AgentConfigError("; ".join(parts))

    return AgentConfig(
        scenario_dir=scenario_dir,
        scenario_glob=scenario_glob,
        hot_reload=hot_reload,
        routing_confidence_floor=confidence_floor,
        routing_consider_top_n=consider_top_n,
        routing_fallback_scenario_id=fallback_scenario_id,
        routing_embedding_model=embedding_model,
        trace_retention_days=retention_days,
        trace_record_llm_messages=record_llm_messages,
        trace_redact_marker=redact_marker,
        defaults_max_loop_iterations_ceiling=max_loop_ceiling,
        defaults_timeout_ms_ceiling=timeout_ceiling,
        defaults_schema_retry_budget_ceiling=schema_retry_ceiling,
        defaults_per_tool_timeout_ms_ceiling=per_tool_ceiling,
        provider_routing=provider_routing,
    )


# Public surface for adversarial tests so they can iterate every key without
# duplicating the loader's internal lists.
ALL_AGENT_ENV_KEYS: tuple[str, ...] = (
    "AGENT_SCENARIO_DIR",
    "AGENT_SCENARIO_GLOB",
    "AGENT_HOT_RELOAD",
    "AGENT_ROUTING_CONFIDENCE_FLOOR",
    "AGENT_ROUTING_CONSIDER_TOP_N",
    "AGENT_ROUTING_FALLBACK_SCENARIO_ID",
    "AGENT_ROUTING_EMBEDDING_MODEL",
    "AGENT_TRACE_RETENTION_DAYS",
    "AGENT_TRACE_RECORD_LLM_MESSAGES",
    "AGENT_TRACE_REDACT_MARKER",
    "AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING",
    "AGENT_DEFAULTS_TIMEOUT_MS_CEILING",
    "AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING",
    "AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING",
    "AGENT_PROVIDER_DEFAULT_PROVIDER",
    "AGENT_PROVIDER_DEFAULT_MODEL",
    "AGENT_PROVIDER_REASONING_PROVIDER",
    "AGENT_PROVIDER_REASONING_MODEL",
    "AGENT_PROVIDER_FAST_PROVIDER",
    "AGENT_PROVIDER_FAST_MODEL",
    "AGENT_PROVIDER_VISION_PROVIDER",
    "AGENT_PROVIDER_VISION_MODEL",
    "AGENT_PROVIDER_OCR_PROVIDER",
    "AGENT_PROVIDER_OCR_MODEL",
)

REQUIRED_NON_EMPTY_KEYS: tuple[str, ...] = _REQUIRED_NON_EMPTY + tuple(
    f"AGENT_PROVIDER_{name.upper()}_{slot}" for name in _REQUIRED_PROVIDER_ROUTES for slot in ("PROVIDER", "MODEL")
)
