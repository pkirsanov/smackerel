"""Prometheus metrics definitions for smackerel-ml sidecar."""

from prometheus_client import Counter, Gauge, Histogram

# Known model values for bounded cardinality (plus "other" bucket)
_KNOWN_MODELS = frozenset(
    [
        "llama3.2",
        "llama3.1",
        "llama3",
        "mistral",
        "mixtral",
        "gpt-4",
        "gpt-4o",
        "gpt-3.5-turbo",
        "claude-3-opus",
        "claude-3-sonnet",
    ]
)


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

# Spec 050 FR-050-005 — embedding worker pool observability.
#
# embedding_workers_configured exposes the configured pool size
# (ML_EMBEDDING_WORKERS) as a Gauge so operators and alerting can verify the
# sidecar is actually running with the SST-bound concurrency limit. Set on
# first generate_embedding() call when the dedicated ThreadPoolExecutor is
# materialised.
#
# embedding_inflight tracks the current admitted-but-not-completed request
# count. It rises toward ML_EMBEDDING_QUEUE_MAX under load and drops back as
# encode() completes. Pair with embedding_rejected_total to see whether
# upstream callers are bouncing on backpressure.
#
# embedding_rejected_total counts requests rejected at the queue cap (i.e.
# inflight == queue_max at admission time). Non-zero values mean upstream
# callers are taking the fail-fast path so the Go core can fall back to text
# search (see internal/api/search.go::textSearch fallback).
embedding_workers_configured = Gauge(
    "smackerel_ml_embedding_workers_configured",
    "Configured size of the dedicated embedding ThreadPoolExecutor (spec 050 FR-050-002)",
)

embedding_inflight = Gauge(
    "smackerel_ml_embedding_inflight",
    "Current admitted-but-not-completed embedding requests (spec 050 FR-050-005)",
)

embedding_rejected_total = Counter(
    "smackerel_ml_embedding_rejected_total",
    "Total embedding requests rejected at the queue cap (spec 050 FR-050-002 backpressure)",
)
