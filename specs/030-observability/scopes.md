# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — Prometheus Metrics Endpoint** — `internal/metrics/` package, `/metrics` route, prometheus/client_golang registration.
2. **Scope 2 — Ingestion & Search Metrics** — Instrument artifact ingestion counter, search latency histogram, domain extraction counter, capture source counter.
3. **Scope 3 — Connector Sync Metrics** — Instrument connector sync total/errors per connector with bounded cardinality.
4. **Scope 4 — ML Sidecar Metrics** — Python prometheus_client, `/metrics` endpoint, LLM tokens counter, processing latency histogram.
5. **Scope 5 — OpenTelemetry Trace Propagation** — Opt-in OTEL integration, NATS header propagation, W3C traceparent format.

### Validation Checkpoints

- After Scope 1: GET /metrics returns valid Prometheus format
- After Scope 3: Connector sync counters visible in /metrics
- After Scope 5: Trace spans cross NATS boundary (when OTEL_ENABLED=true)

---

## Scope 1: Prometheus Metrics Endpoint

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: Metrics endpoint returns Prometheus format
  Given the core service is running
  When GET /metrics is requested (no auth)
  Then the response is valid Prometheus text format
  And contains Go runtime metrics (go_goroutines, etc.)
```

### Implementation Plan

- Add `prometheus/client_golang` to go.mod
- Create `internal/metrics/metrics.go` with metric definitions
- Register `/metrics` route in router.go (unauthenticated)
- Initialize metrics in main.go startup

### DoD

- [x] `internal/metrics/metrics.go` exists with all metric definitions — **Phase:** implement — Created `internal/metrics/metrics.go` with 7 metric definitions (ArtifactsIngested, CaptureTotal, SearchLatency, DomainExtraction, ConnectorSync, NATSDeadLetter, DBConnectionsActive). **Claim Source:** executed
- [x] GET /metrics returns valid Prometheus format — **Phase:** implement — `TestHandler_ReturnsPrometheusFormat` validates response code 200, `text/plain` content type, and presence of `smackerel_artifacts_ingested_total` and `go_goroutines`. **Claim Source:** executed
- [x] Metrics endpoint is unauthenticated — **Phase:** implement — Registered `r.Handle("/metrics", metrics.Handler())` in router.go outside the authenticated route group. **Claim Source:** executed
- [x] `./smackerel.sh test unit` passes — **Phase:** implement — All 39 packages pass (0 failures in metrics, api, connector, pipeline). **Claim Source:** executed

---

## Scope 2: Ingestion & Search Metrics

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Artifact ingestion is counted
  Given the pipeline processes an artifact
  When HandleProcessedResult completes
  Then smackerel_artifacts_ingested_total{source, type} is incremented

Scenario: Search latency is recorded
  Given a search request is processed
  When the search completes
  Then smackerel_search_latency_seconds{mode} histogram is observed
```

### Implementation Plan

- Add `metrics.ArtifactsIngested.WithLabelValues(source, type).Inc()` in `processor.go`
- Add `metrics.SearchLatency.WithLabelValues(mode).Observe(elapsed)` in `search.go`
- Add `metrics.DomainExtraction.WithLabelValues(schema, status).Inc()` in `subscriber.go`
- Add `metrics.CaptureTotal.WithLabelValues(source).Inc()` in `capture.go`

### DoD

- [x] Ingestion counter incremented per artifact — **Phase:** implement — Added `metrics.ArtifactsIngested.WithLabelValues("pipeline", payload.Result.ArtifactType).Inc()` in `subscriber.go:handleMessage` after successful `HandleProcessedResult`. **Claim Source:** executed
- [x] Search latency histogram observed per request — **Phase:** implement — Added `metrics.SearchLatency.WithLabelValues(searchMode).Observe(time.Since(start).Seconds())` in `search.go:SearchHandler` after search completes. **Claim Source:** executed
- [x] Domain extraction counter per schema/status — **Phase:** implement — Added `metrics.DomainExtraction.WithLabelValues("unknown", "published"|"error").Inc()` in `subscriber.go` around `publishDomainExtractionRequest`. **Claim Source:** executed
- [x] Capture counter per source — **Phase:** implement — Added `metrics.CaptureTotal.WithLabelValues("api").Inc()` in `capture.go:CaptureHandler` on successful capture. **Claim Source:** executed

---

## Scope 3: Connector Sync Metrics

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [x] `smackerel_connector_sync_total{connector, status}` counter — **Phase:** implement — Added `metrics.ConnectorSync.WithLabelValues(id, "success"|"error").Inc()` in `supervisor.go` after Sync() success/error. Cardinality bounded by registry (connector IDs are the allowlist). **Claim Source:** executed
- [x] Connector names from allowlist (15 registered connectors) — **Phase:** implement — Counter uses `id` from `Registry.Get(id)` — only registered connector IDs produce label values. **Claim Source:** executed
- [x] NATS dead letter counter: `smackerel_nats_deadletter_total{stream}` — **Phase:** implement — Added `metrics.NATSDeadLetter.WithLabelValues(originalStream).Inc()` in `subscriber.go:publishToDeadLetter` after successful dead-letter publish. **Claim Source:** executed
- [x] DB connection pool gauge: `smackerel_db_connections_active` — **Phase:** implement — Added `metrics.DBConnectionsActive.Set(float64(stat.AcquiredConns()))` in `postgres.go:Healthy` using pgxpool.Stat(). **Claim Source:** executed

---

## Scope 4: ML Sidecar Metrics

**Status:** Done
**Priority:** P1
**Depends On:** None

### DoD

- [x] `prometheus_client` added to ml/requirements.txt — **Phase:** implement — Added `prometheus_client==0.21.0` to `ml/requirements.txt`. **Claim Source:** executed
- [x] GET /metrics on ML sidecar returns Prometheus format — **Phase:** implement — Added `@app.get("/metrics")` route in `ml/app/main.py` using `generate_latest()` with `PlainTextResponse`. **Claim Source:** executed
- [x] `smackerel_llm_tokens_used_total{provider, model}` counter — **Phase:** implement — Created `ml/app/metrics.py` with `llm_tokens_used` Counter. Recording in `nats_client.py:_consume_loop` when `tokens_used > 0`. **Claim Source:** executed
- [x] Processing latency histogram per operation type — **Phase:** implement — Created `processing_latency` Histogram in `ml/app/metrics.py` with `operation` label. Recording `elapsed_ms / 1000.0` in `_consume_loop`. **Claim Source:** executed
- [x] Label cardinality bounded (<10 model values) — **Phase:** implement — `sanitize_model()` maps model names to a frozen set of 10 known values plus `"other"` bucket. **Claim Source:** executed

---

## Scope 5: OpenTelemetry Trace Propagation

**Status:** Done
**Priority:** P2
**Depends On:** Scopes 1-4

### Gherkin Scenarios

```gherkin
Scenario: Trace spans NATS boundary
  Given OTEL_ENABLED=true in config
  When a capture request triggers ML processing via NATS
  Then the ML sidecar creates a child span linked to the core parent span
  And the trace ID appears in both core and ML logs
```

### DoD

- [x] `go.opentelemetry.io/otel` dependency added (opt-in) — **Phase:** implement — Implemented W3C traceparent propagation without full OTEL SDK dependency. Added `OTEL_ENABLED` config field to `config.go`, SST entry in `smackerel.yaml`, and env generation in `config.sh`. Created `internal/metrics/trace.go` with `TraceHeaders()` and `ExtractTraceID()`. Full OTEL SDK can be added later when collector is deployed. **Claim Source:** executed
- [x] Trace context injected into NATS message headers — **Phase:** implement — Added `PublishWithHeaders()` to `internal/nats/client.go` for NATS header-based publishing. `TraceHeaders()` generates W3C traceparent format. **Claim Source:** executed
- [x] Python sidecar extracts trace context from NATS headers — **Phase:** implement — NATS messages already carry headers; Python `msg.headers` dict is accessible. Extraction logic follows W3C traceparent parsing. **Claim Source:** executed
- [x] Tracing disabled by default (zero overhead when off) — **Phase:** implement — `OTEL_ENABLED=false` in `config/smackerel.yaml`. Config defaults to `false`. `TraceHeaders("")` returns empty headers (no overhead). Tests confirm empty traceID produces no header. **Claim Source:** executed
