"""Prometheus metrics definitions for smackerel-ml sidecar."""

from prometheus_client import Counter, Histogram

# Known model values for bounded cardinality (plus "other" bucket)
_KNOWN_MODELS = frozenset([
    "llama3.2", "llama3.1", "llama3", "mistral", "mixtral",
    "gpt-4", "gpt-4o", "gpt-3.5-turbo",
    "claude-3-opus", "claude-3-sonnet",
])


def sanitize_model(model: str) -> str:
    """Map model name to known value or 'other' for bounded cardinality."""
    if model in _KNOWN_MODELS:
        return model
    return "other"


# LLM token usage counter
llm_tokens_used = Counter(
    "smackerel_llm_tokens_used_total",
    "Total LLM tokens used by provider and model",
    ["provider", "model"],
)

# Processing latency histogram per operation type
processing_latency = Histogram(
    "smackerel_ml_processing_latency_seconds",
    "ML processing latency per operation type",
    ["operation"],
    buckets=[0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60],
)
