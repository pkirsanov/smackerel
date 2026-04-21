# Execution Report: 030 — Observability

Links: [spec.md](spec.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 030 adds observability infrastructure: Prometheus metrics endpoints for Go core and Python ML sidecar, ingestion/search/connector/domain-extraction metrics instrumentation, and W3C trace propagation via NATS headers. All 5 scopes completed.

---

## Test-to-Doc Sweep (2026-04-21)

### Trigger: `bubbles.test` probe — coverage gaps, weak assertions, untested scenarios

### Findings

| # | Category | Gap | Severity |
|---|----------|-----|----------|
| F1 | Go unit | `ConnectorSync`, `DomainExtraction`, `NATSDeadLetter` counters had zero dedicated increment tests — 3 of 7 metrics untested for label behavior | Medium |
| F2 | Go unit | No round-trip test for `TraceHeaders()` → `ExtractTraceID()` and no edge-case tests for malformed traceparent with wrong part count | Medium |
| F3 | Python unit | Zero test coverage for ML sidecar metrics: no test for `/metrics` endpoint, `sanitize_model()`, `llm_tokens_used`, or `processing_latency` (Scope 4 had 0 tests) | High |

### Remediation

**F1 — Go counter tests added** (`internal/metrics/metrics_test.go`):
- `TestConnectorSyncCounter` — verifies `smackerel_connector_sync_total{connector,status}` with success/error labels
- `TestDomainExtractionCounter` — verifies `smackerel_domain_extraction_total{schema,status}` with published/error labels
- `TestNATSDeadLetterCounter` — verifies `smackerel_nats_deadletter_total{stream}` increment

**F2 — Trace round-trip and edge cases added** (`internal/metrics/trace_test.go`):
- `TestTraceRoundTrip` — inject via `TraceHeaders()` → extract via `ExtractTraceID()` → same ID
- `TestExtractTraceID_TooFewParts` — 3-part traceparent returns empty
- `TestExtractTraceID_TooManyParts` — 5-part traceparent returns empty

**F3 — Python metrics test suite created** (`ml/tests/test_metrics.py`):
- `TestSanitizeModel` — 10 known models pass through, unknown → "other", empty → "other", case-sensitive
- `TestLLMTokensUsedCounter` — increment with provider/model labels, independent label tracking
- `TestProcessingLatencyHistogram` — observation recording, accumulation
- `TestMetricsEndpoint` — `/metrics` returns 200, correct content type, contains `smackerel_llm_tokens_used_total` and `smackerel_ml_processing_latency_seconds`, unauthenticated access

### Verification

- `./smackerel.sh test unit`: All 39 Go packages pass (0 failures), 236 Python tests pass (0 failures)

---

## Scope Evidence

### Scope 1 — Prometheus Metrics Package
- `internal/metrics/metrics.go` defines 7 Prometheus metrics: `smackerel_artifacts_ingested_total`, `smackerel_capture_total`, `smackerel_search_latency_seconds`, `smackerel_domain_extraction_total`, `smackerel_connector_sync_total`, `smackerel_nats_deadletter_total`, `smackerel_db_connections_active`.
- Exposed at `/metrics` endpoint (unauthenticated, standard scrape pattern).

### Scope 2 — Ingestion & Capture Metrics
- Counter increments wired into capture handler and pipeline processing.

### Scope 3 — Search Latency Histogram
- Histogram records search latency with `mode` label.

### Scope 4 — Connector & NATS Metrics
- Connector sync counter tracks per-connector success/failure. NATS dead-letter counter per stream.

### Scope 5 — W3C Trace Propagation
- `internal/metrics/trace.go` provides `TraceHeaders()` and `ExtractTraceID()` for W3C `traceparent` header injection/extraction over NATS messages.
