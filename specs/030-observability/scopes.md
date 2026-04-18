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

**Status:** Not Started
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

- [ ] `internal/metrics/metrics.go` exists with all metric definitions
- [ ] GET /metrics returns valid Prometheus format
- [ ] Metrics endpoint is unauthenticated
- [ ] `./smackerel.sh test unit` passes

---

## Scope 2: Ingestion & Search Metrics

**Status:** Not Started
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

- [ ] Ingestion counter incremented per artifact
- [ ] Search latency histogram observed per request
- [ ] Domain extraction counter per schema/status
- [ ] Capture counter per source

---

## Scope 3: Connector Sync Metrics

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [ ] `smackerel_connector_sync_total{connector, status}` counter
- [ ] Connector names from allowlist (15 registered connectors)
- [ ] NATS dead letter counter: `smackerel_nats_deadletter_total{stream}`
- [ ] DB connection pool gauge: `smackerel_db_connections_active`

---

## Scope 4: ML Sidecar Metrics

**Status:** Not Started
**Priority:** P1
**Depends On:** None

### DoD

- [ ] `prometheus_client` added to ml/requirements.txt
- [ ] GET /metrics on ML sidecar returns Prometheus format
- [ ] `smackerel_llm_tokens_used_total{provider, model}` counter
- [ ] Processing latency histogram per operation type
- [ ] Label cardinality bounded (<10 model values)

---

## Scope 5: OpenTelemetry Trace Propagation

**Status:** Not Started
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

- [ ] `go.opentelemetry.io/otel` dependency added (opt-in)
- [ ] Trace context injected into NATS message headers
- [ ] Python sidecar extracts trace context from NATS headers
- [ ] Tracing disabled by default (zero overhead when off)
