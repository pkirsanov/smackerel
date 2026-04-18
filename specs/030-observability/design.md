# Design: 030 — Observability: Metrics & Tracing

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 18, 2026
> **Status:** Draft

---

## Overview

Adds Prometheus metrics endpoint to Go core and Python ML sidecar, instruments key operations, and optionally supports OpenTelemetry trace context propagation through NATS messages.

### Key Design Decisions

1. **`promhttp` for Go, `prometheus_client` for Python** — Standard libraries, minimal footprint
2. **Metrics endpoint unauthenticated at `/metrics`** — Standard Prometheus scrape pattern; protected by Docker network isolation (localhost binding)
3. **Bounded cardinality** — Connector names use allowlist; LLM models use a "known models" set with `other` bucket
4. **Tracing opt-in** — Disabled by default; enabled via `OTEL_ENABLED=true` in config
5. **NATS header propagation** — Trace context embedded in NATS message headers using W3C traceparent format

---

## Architecture

### Metrics Collection Points

```
┌──────────────────────────────────────────────────────┐
│                   Go Core Runtime                     │
│                                                      │
│  /metrics ─── promhttp.Handler()                     │
│                                                      │
│  Instrumented points:                                │
│  ├── api/ ── search latency histogram                │
│  ├── api/ ── capture counter                         │
│  ├── pipeline/ ── artifacts ingested counter         │
│  ├── pipeline/ ── domain extraction counter          │
│  ├── connector/ ── sync total/errors per connector   │
│  ├── digest/ ── generation counter                   │
│  ├── nats/ ── dead letter counter per stream         │
│  └── db/ ── connection pool gauge                    │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│                   Python ML Sidecar                   │
│                                                      │
│  /metrics ─── prometheus_client                       │
│                                                      │
│  Instrumented points:                                │
│  ├── processor ── processing latency histogram       │
│  ├── processor ── LLM tokens counter                 │
│  ├── domain ── extraction latency histogram          │
│  └── embedder ── embedding latency histogram         │
└──────────────────────────────────────────────────────┘
```

### Metric Definitions

```go
// internal/metrics/metrics.go
var (
    ArtifactsIngested = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "smackerel_artifacts_ingested_total",
            Help: "Total artifacts ingested by source and type",
        },
        []string{"source", "type"},
    )
    SearchLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "smackerel_search_latency_seconds",
            Help:    "Search request latency",
            Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5},
        },
        []string{"mode"},
    )
    ConnectorSync = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "smackerel_connector_sync_total",
            Help: "Connector sync attempts",
        },
        []string{"connector", "status"},
    )
    DomainExtraction = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "smackerel_domain_extraction_total",
            Help: "Domain extraction attempts",
        },
        []string{"schema", "status"},
    )
    NATSDeadLetter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "smackerel_nats_deadletter_total",
            Help: "Messages routed to dead letter",
        },
        []string{"stream"},
    )
    DBConnectionsActive = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "smackerel_db_connections_active",
            Help: "Active database connections",
        },
    )
    CaptureTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "smackerel_capture_total",
            Help: "Capture requests by source",
        },
        []string{"source"},
    )
)
```

### Cardinality Control

| Label | Source | Max Values | Strategy |
|-------|--------|------------|----------|
| `connector` | 15 registered connectors | 15 | Allowlist from registry |
| `source` | capture/ingestion source | ~6 | Enum: telegram, api, extension, pwa, connector, import |
| `type` | artifact_type | ~15 | Enum from processor |
| `schema` | domain extraction | ~5 | Registered contract versions |
| `provider` | LLM provider | ~4 | ollama, openai, anthropic, other |
| `model` | LLM model | ~10 | Known models + "other" bucket |
| `mode` | search mode | 4 | vector, text_fallback, time_range, knowledge |
| `stream` | NATS stream | 9 | Fixed stream names |
| **Total combinations** | | **<500** | Well under 1000 target |

### OpenTelemetry Integration (Opt-in)

```go
// Trace context in NATS headers
headers := nats.Header{}
if otel.Enabled() {
    propagator := otel.GetTextMapPropagator()
    propagator.Inject(ctx, propagation.HeaderCarrier(headers))
}
msg.Header = headers
```

Python consumer extracts:
```python
if os.getenv("OTEL_ENABLED") == "true":
    propagator = TraceContextTextMapPropagator()
    ctx = propagator.extract(carrier=msg.headers)
    with tracer.start_as_current_span("ml.process", context=ctx):
        ...
```

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Unit | Metric registration, counter increment, histogram observe | `internal/metrics/metrics_test.go` |
| Integration | `/metrics` endpoint returns valid Prometheus format | `./smackerel.sh test integration` |
| Manual | Prometheus scrape config + Grafana dashboard screenshot | Ops verification |

---

## Risks & Open Questions

| # | Risk | Mitigation |
|---|------|------------|
| 1 | Prometheus dependency adds binary size | `prometheus/client_golang` is ~5MB compiled — acceptable |
| 2 | Histogram buckets misaligned with actual latencies | Start with standard buckets, adjust after production observation |
| 3 | Trace context adds NATS message overhead | W3C traceparent header is 55 bytes — negligible |
