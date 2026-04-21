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

---

## Gaps-to-Doc Sweep (2026-04-21)

### Trigger: `bubbles.gaps` probe — implementation gaps against design/requirements

### Findings

| # | Category | Gap | Severity | Status |
|---|----------|-----|----------|--------|
| G1 | Metrics | `smackerel_capture_total` hardcoded `"api"` source label — spec requires per-source tracking (telegram, api, extension, pwa) | Medium | Fixed |
| G2 | Metrics | `smackerel_digest_generation_total` counter missing — spec goals and design architecture diagram both list digest generation as instrumented operation | Medium | Fixed |
| G3 | Trace | `PublishWithHeaders()` exists but is never called in pipeline code — trace context not injected into production NATS messages | Low | Documented (design-scoped deferral) |
| G4 | Trace | Python ML sidecar has no trace context extraction from NATS headers | Low | Documented (design-scoped deferral) |

### Remediation

**G1 — Capture source metric label (FIXED)**
- Added `X-Capture-Source` header support in `internal/api/capture.go`
- `captureSource()` validates against bounded set: `api`, `telegram`, `extension`, `pwa`
- Unknown/missing header defaults to `"api"` (backward-compatible)
- Telegram bot sets `X-Capture-Source: telegram` in `internal/telegram/bot.go:callCapture()`
- Added `TestCaptureSource` with 7 cases including injection prevention

**G2 — Digest generation metric (FIXED)**
- Added `smackerel_digest_generation_total{status}` counter in `internal/metrics/metrics.go`
- Status labels: `published` (NATS success), `fallback` (NATS failure, local generation), `quiet` (no content day)
- Instrumented in `internal/digest/generator.go:Generate()`
- Metric registered in `init()` and covered by `TestMetricsRegistered`

**G3/G4 — Trace propagation wiring (DOCUMENTED — not a gap against current design)**
- Scope 5 design explicitly positions trace propagation as a "foundation" with "Full OTEL SDK can be added later when collector is deployed"
- `TraceHeaders()` and `ExtractTraceID()` are implemented, tested, and work correctly
- `PublishWithHeaders()` is available in `internal/nats/client.go`
- Production wiring requires threading `OTELEnabled` config into all NATS publish sites and adding Python-side extraction — this is future work when OTEL collector infrastructure is deployed

### Verification

- `go test -count=1 ./internal/metrics/ ./internal/api/ ./internal/digest/` — all pass
- `./smackerel.sh lint` — 0 errors

---

## Regression-to-Doc Sweep (2026-04-21)

### Trigger: `bubbles.regression` probe — stochastic-quality-sweep R73

### Probe Scope

Checked for regressions across all 5 observability scopes since spec was marked done:

1. **Unit test suite**: `./smackerel.sh test unit` — 42 Go packages pass, 236 Python tests pass (0 failures)
2. **Lint**: `./smackerel.sh lint` — 0 errors
3. **Go metric callsite wiring** (8 metrics verified live in source):
   - `metrics.ArtifactsIngested` → `internal/pipeline/subscriber.go:238`
   - `metrics.SearchLatency` → `internal/api/search.go:171`
   - `metrics.ConnectorSync` → `internal/connector/supervisor.go:268,320`
   - `metrics.CaptureTotal` → `internal/api/capture.go:142`
   - `metrics.DomainExtraction` → `internal/pipeline/subscriber.go:585,589` + `domain_subscriber.go:155,183`
   - `metrics.NATSDeadLetter` → `internal/pipeline/subscriber.go:366`
   - `metrics.DBConnectionsActive` → `internal/db/postgres.go:81`
   - `metrics.DigestGeneration` → `internal/digest/generator.go:151,164,168`
4. **Route registration**: `metrics.Handler()` registered at `/metrics` in `internal/api/router.go:44` — unauthenticated
5. **ML sidecar endpoint**: `/metrics` endpoint in `ml/app/main.py:98` via `generate_latest()` — unauthenticated
6. **ML sidecar metric consumers**: `llm_tokens_used`, `processing_latency`, `sanitize_model` imported and called in `ml/app/nats_client.py:14,295,301`
7. **Trace utilities**: `trace.go` (`TraceHeaders`, `ExtractTraceID`) and `trace_test.go` (7 tests) present and passing
8. **Config pipeline**: `OTEL_ENABLED` in `config/smackerel.yaml:364`, generated into env via `config.sh:498`, consumed in `config.go:513`

### Findings

| # | Category | Finding | Severity |
|---|----------|---------|----------|
| — | — | No regressions detected | — |

### Verdict

**CLEAN.** All observability implementation remains intact. No metric definitions removed, no callsites disconnected, no test failures, no lint errors. Spec remains `done`.

---

## Improve-Existing Sweep (2026-04-21)

### Trigger: `bubbles.improve` probe — stochastic-quality-sweep child workflow

### Probe Scope

Analyzed all observability code for improvement opportunities across Go core metrics, ML sidecar metrics, trace propagation, and handler instrumentation patterns.

### Findings

| # | Category | Finding | Severity | Status |
|---|----------|---------|----------|--------|
| IMP-1 | Metrics | Intelligence handlers (8 endpoints) skip `IntelligenceLatency` histogram observation on error paths — error duration invisible to operators, violating RED (Rate/Error/Duration) methodology | Medium | Fixed |

### Remediation

**IMP-1 — Intelligence handler error-path latency (FIXED)**
- All 8 handlers in `internal/api/intelligence.go` used `metrics.IntelligenceLatency.WithLabelValues(...).Observe(...)` only on the success return path
- On error, `IntelligenceErrors` was incremented but latency was NOT recorded — operators could not distinguish fast failures (e.g., validation) from slow failures (e.g., timeouts)
- Fix: moved latency observation into a `defer` at the top of each handler, ensuring it fires for both success and error paths
- Affected handlers: `ExpertiseHandler`, `LearningPathsHandler`, `SubscriptionsHandler`, `SerendipityHandler`, `ContentFuelHandler`, `QuickReferencesHandler`, `MonthlyReportHandler`, `SeasonalPatternsHandler`

### Verification

- `./smackerel.sh test unit`: 42 Go packages pass (including `internal/api`), 236 Python tests pass — 0 failures

---

## DevOps-to-Doc Sweep (2026-04-21)

### Trigger: `bubbles.devops` probe — stochastic-quality-sweep child workflow

### Probe Scope

Systematic DevOps audit of observability infrastructure: build pipeline, Docker Compose wiring, config SST compliance, metrics endpoint exposure, deployment documentation, and operational runbook accuracy.

### Build Pipeline

| Check | Result |
|-------|--------|
| Go core builds with `prometheus/client_golang v1.23.2` | PASS |
| ML sidecar builds with `prometheus_client==0.21.0` | PASS |
| Both Dockerfiles: multi-stage, OCI labels, non-root user | PASS |
| `./smackerel.sh build` completes successfully | PASS |

### Config SST Compliance

| Check | Result |
|-------|--------|
| `observability.otel_enabled` defined in SST | PASS |
| `observability.otel_exporter_endpoint` defined in SST | PASS |
| Config generator emits `OTEL_ENABLED`, `OTEL_EXPORTER_ENDPOINT` to env | PASS |
| Go `config.go` reads `OTEL_ENABLED` from env | PASS |
| `./smackerel.sh check` — SST in sync, no drift | PASS |
| `./smackerel.sh check` — env_file drift guard OK | PASS |

### Docker Compose

| Check | Result |
|-------|--------|
| Core `/metrics` accessible on `:40001/metrics` (same port as app) | PASS |
| ML sidecar `/metrics` on `:40002/metrics` (same port as app) | PASS |
| Both services use `env_file:` for SST vars | PASS |
| Health checks defined for all services | PASS |
| Production overrides (`docker-compose.prod.yml`) with logging config | PASS |

### Tests

| Check | Result |
|-------|--------|
| `./smackerel.sh test unit --go` — 42 packages, 0 failures | PASS |
| `./smackerel.sh test unit --python` — 236 tests, 0 failures | PASS |
| `./smackerel.sh lint` — 0 errors | PASS |

### Documentation Findings

| # | Category | Gap | Severity | Status |
|---|----------|-----|----------|--------|
| D1 | Docs | Operations.md metrics table missing `smackerel_digest_generation_total{status}` | Low | Fixed |
| D2 | Docs | Operations.md missing ML sidecar metrics section (`smackerel_llm_tokens_used_total`, `smackerel_ml_processing_latency_seconds`) and endpoint URL | Medium | Fixed |
| D3 | Docs | Operations.md missing OTEL tracing enablement guidance | Low | Fixed |

### Remediation

**D1/D2/D3 — Operations.md updated** (`docs/Operations.md`):
- Split metrics section into "Go Core" and "ML Sidecar" subsections with respective endpoint URLs
- Added `smackerel_digest_generation_total{status}` to Go core metrics table
- Added `smackerel_capture_total` source label detail (telegram, api, extension, pwa)
- Added ML sidecar metrics table: `smackerel_llm_tokens_used_total`, `smackerel_ml_processing_latency_seconds`
- Added cardinality note for model labels
- Added "OpenTelemetry Tracing (Opt-in)" section with enablement steps

### Verdict

**CLEAN (code) + 3 doc-sync fixes applied.** Build pipeline, Docker Compose, config SST, and test infrastructure are all healthy. No code changes required. Operations documentation now matches the full implemented metrics surface.
