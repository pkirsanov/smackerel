# Feature: 030 — Observability: Metrics & Tracing

## Problem Statement

Smackerel's operational visibility is limited to structured logging (659 slog calls across the codebase). There are no Prometheus metrics, no Grafana dashboards, and no distributed tracing. When a user reports "search is slow" or "my recipe wasn't processed," the only diagnostic tool is grep through container logs. This scored 6/10 in the system review and is a production readiness gap.

## Outcome Contract

**Intent:** The system exposes Prometheus metrics for key operations (ingestion latency, search P95, connector sync counts, pipeline throughput, LLM token usage) and optionally supports OpenTelemetry distributed tracing for request-level debugging.

**Success Signal:** An operator can query `http://localhost:8080/metrics` and see: artifacts ingested in the last hour, search request latency histogram, connector sync success/failure counts, and LLM token usage per domain. A trace ID in a log line can be followed through core → NATS → ML sidecar → core.

**Hard Constraints:**
- Metrics endpoint must be unauthenticated (standard Prometheus scrape pattern)
- Metrics must not degrade request latency by more than 1ms
- Tracing must be opt-in (disabled by default, enabled via config)
- No external service dependencies for core metrics (Prometheus scrapes, not pushes)
- Metric names must follow Prometheus naming conventions (`smackerel_*` prefix)

**Failure Condition:** If metrics exist but don't cover the hot paths (search, ingest, connectors), they're vanity metrics. If tracing is always-on and adds latency, it hurts the system it's meant to help.

## Goals

1. Add Prometheus `/metrics` endpoint to the Go core HTTP server
2. Instrument key operations: artifact ingestion, search, connector sync, digest generation, domain extraction
3. Add LLM usage metrics (tokens used, latency, errors by provider/model)
4. Add optional OpenTelemetry trace context propagation through NATS messages
5. Add Python ML sidecar metrics endpoint

## Non-Goals

- Grafana dashboard provisioning (operator-managed)
- Alertmanager rules (operator-configured)
- Application Performance Monitoring (APM) vendors
- Custom logging format changes (slog is sufficient)

## User Scenarios (Gherkin)

```gherkin
Scenario: Prometheus scrapes core metrics
  Given the smackerel-core container is running
  When Prometheus scrapes http://localhost:8080/metrics
  Then it receives metrics including smackerel_artifacts_ingested_total, smackerel_search_requests_total, smackerel_search_latency_seconds

Scenario: Connector sync metrics
  Given 15 connectors are configured
  When connectors run their sync cycles
  Then smackerel_connector_sync_total and smackerel_connector_sync_errors_total are incremented per connector

Scenario: LLM usage tracking
  Given the ML sidecar processes artifacts
  When LLM calls complete
  Then smackerel_llm_tokens_used_total is incremented with provider and model labels

Scenario: Distributed trace follows a request
  Given tracing is enabled in config
  When a user sends a capture request
  Then a trace spans core HTTP handler → NATS publish → ML processing → NATS response → DB write
```

## Acceptance Criteria

- [ ] GET /metrics returns Prometheus-format metrics from Go core
- [ ] Ingestion counter: `smackerel_artifacts_ingested_total{source, type}`
- [ ] Search histogram: `smackerel_search_latency_seconds{mode}`
- [ ] Connector gauge: `smackerel_connector_sync_total{connector, status}`
- [ ] LLM counter: `smackerel_llm_tokens_used_total{provider, model}`
- [ ] ML sidecar exposes /metrics with processing latency
- [ ] Trace context propagated through NATS message headers (opt-in)
- [ ] Metrics add < 1ms overhead to hot paths
