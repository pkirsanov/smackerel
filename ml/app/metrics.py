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

# BUG-026-008 — content-free observability for the single corrective model call
# made after parsed synthesis JSON fails its prompt-contract schema. No labels
# are used, so model output, artifact content, identifiers, and validation text
# can never enter the metric surface or create unbounded cardinality.
synthesis_schema_repair_attempts_total = Counter(
    "smackerel_ml_synthesis_schema_repair_attempts_total",
    "Corrective synthesis model calls after parsed JSON fails schema validation",
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

# Spec 046 follow-up F4 (sweep round 13) — NATS consume-loop observability.
#
# nats_consume_fetch_errors_total counts non-timeout errors raised by the
# JetStream pull subscriber's fetch() call in nats_client._consume_loop.
# Idle-fetch timeouts are NOT counted (they are the normal poll cadence);
# only transport/stream/auth errors increment this counter. Non-zero values
# mean the sidecar is hitting reconnect storms, stream-deleted conditions,
# or auth failures that would otherwise be invisible behind the bare-except
# previously present in _consume_loop.
nats_consume_fetch_errors_total = Counter(
    "smackerel_ml_nats_consume_fetch_errors_total",
    "Non-timeout errors raised by JetStream pull-subscribe fetch() per subject (spec 046 follow-up F4)",
    ["subject"],
)

# Spec 081 FR-081-003 — Python sidecar dead-letter parity with Go.
#
# nats_deadletter_total counts messages routed to deadletter.<subject>
# from the ML sidecar's poison-pill branch (parity with the Go
# metrics.NATSDeadLetter counter exposed by internal/pipeline/*subscriber.go).
# Labelled by stream so operators can see which JetStream stream is
# producing poison messages.
nats_deadletter_total = Counter(
    "smackerel_ml_nats_deadletter_total",
    "Messages routed to deadletter.<subject> from the ML sidecar (spec 081 FR-081-003)",
    ["stream"],
)

# nats_deadletter_publish_failures_total counts publish-to-deadletter
# attempts that failed. On failure the consumer nak()s the original
# message (rather than term()) so JetStream redelivers and we retry the
# publish — see design §4 invariant 1 (publish-before-term).
nats_deadletter_publish_failures_total = Counter(
    "smackerel_ml_nats_deadletter_publish_failures_total",
    (
        "Publish attempts to deadletter.<subject> that failed; "
        "original msg was nak()ed for redelivery (spec 081 FR-081-003)"
    ),
    ["subject"],
)
