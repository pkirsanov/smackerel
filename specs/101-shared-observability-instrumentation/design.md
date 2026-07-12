# Design — Spec 101 Shared-Observability Instrumentation Contract

**Feature:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

## Current Truth (read-only audit, 2026-07-06)

A solution-blind audit of the smackerel repo established what already exists
BEFORE any code was written (per the "do not assume greenfield" mandate). The
scope-03 Implementation Plan assumed a greenfield product side; the audit proves
otherwise:

| Contract element | Current-truth in smackerel | Verdict |
|---|---|---|
| `/metrics` Prometheus endpoint | `internal/api/router.go` L62 `r.Handle("/metrics", metrics.Handler())` → `internal/metrics/metrics.go` `promhttp.Handler()` (rich `smackerel_*` metric set) | **ALREADY EXISTS** — reuse + cite |
| OTLP/gRPC span exporter | `internal/assistant/tracing/tracer.go` — real `otlptrace.New` + `otlptracegrpc.NewClient` + `sdktrace.NewTracerProvider` (BatchSpanProcessor, `service.name` resource), no-op fallback when disabled, fail-loud 5 s TCP boot probe in `cmd/core/wiring.go::initAssistantTracing`, shutdown flush in `cmd/core/shutdown.go`, tests in `cmd/core/wiring_otel_test.go` | **ALREADY EXISTS** — reuse, do NOT fork (knb FINDING-014-03-1) |
| otel-go SDK deps | `go.mod`: `otel v1.44.0`, `otlptrace`, `otlptracegrpc`, `sdk`, `trace` | **ALREADY PRESENT** |
| env-var contract | `internal/config/config.go` read `OTEL_ENABLED` + `OTEL_EXPORTER_ENDPOINT` (single); `OTEL_EXPORTER_ENDPOINT` was **declared-but-NOT-consumed** (0 Go consumers, grep-verified) | **WRONG NAMING** — migrate to knb 3-var (option (a)) |
| knb adapter injection | `<deployment-owner>/<product>/<target>/apply.sh` already injects `OTLP_TRACES_ENDPOINT` / `OTLP_LOGS_ENDPOINT` / `METRICS_SCRAPE_LABEL_PRODUCT` | **knb side DONE** — zero rework |
| container labels | `com.smackerel.component` + `com.smackerel.lifecycle` only | **GAP** — add `com.bubbles.product` + `com.bubbles.service` |

**Conclusion:** the genuine gaps are (1) env-var contract naming, (2) a fail-loud
service-tier reader of the 3 vars, (3) the `com.bubbles.*` discovery labels. The
exporter and `/metrics` are reused. This matches the knb scope-03 report's
ratified **option (a)** resolution and the WanderAide scope-05 accepted precedent
(a fail-loud config reader; live export deferred-to-flip).

## Design decisions

### D1 — Reconcile, do not duplicate (knb FINDING-014-03-1)

`internal/observability` is the service-tier **config CONTRACT** (fail-loud
reader), NOT a second exporter. Span export continues through the existing
`internal/assistant/tracing` OTLP/gRPC pipeline; metrics continue through the
existing `internal/api/router.go` `/metrics` handler. This preserves single-
source-of-truth and avoids forking the `done` spec-061 subsystem.

### D2 — Opt-in gate = `OTEL_ENABLED`, not presence-magic

The contract is validated fail-loud ONLY when `OTEL_ENABLED=true` (smackerel's
existing, ratified opt-in observability gate; `otel_enabled: false` by default).
This is NOT a fallback — it is an explicit boolean posture switch:

- **bundled / dev / test** (`OTEL_ENABLED=false`, endpoints empty): startup
  unaffected, contract inert (matches the existing "zero overhead when disabled"
  design and the assistant tracer's no-op path).
- **shared posture** (operator sets `otel_enabled: true`; knb adapter injects the
  3 real endpoints): the 3 vars MUST resolve non-empty or `initSharedObservability`
  aborts startup with a named error.
- **misconfigured shared** (enabled but a var empty): fail-loud abort — this is
  the scope-03 "missing values cause fail-loud startup" proof.

### D3 — Fail-loud, no defaults (smackerel NO-DEFAULTS SST / Gate G028)

- `internal/observability/shared.go::Config.Validate()` rejects unset / empty /
  whitespace-only on each of the 3 vars with a named, actionable error.
- The generator (`scripts/commands/config.sh`) uses `required_value` (fails on a
  MISSING SST key) — an improvement over the prior soft `yaml_get … || ""`.
  Empty-string VALUES remain the intended dev placeholder; the Go runtime is the
  fail-loud authority when `OTEL_ENABLED=true`.

### D4 — SST-sourced discovery labels

`com.bubbles.product: ${METRICS_SCRAPE_LABEL_PRODUCT}` sources the product name
from SST (the generator always emits `METRICS_SCRAPE_LABEL_PRODUCT=smackerel`);
`com.bubbles.service` is the per-container identity (matches the existing
`com.smackerel.component` value). Labels are additive — existing `com.smackerel.*`
labels (including the spec-082 nats `persistent` lifecycle contract) are
untouched.

## Architecture (target state)

```
config/smackerel.yaml (observability:)                 ← SST source of truth
   otel_enabled: false
   otlp_traces_endpoint: ""       (knb injects under shared posture)
   otlp_logs_endpoint: ""         (knb injects under shared posture)
   metrics_scrape_label_product: "smackerel"
        │  ./smackerel.sh config generate  (required_value; NO-DEFAULTS)
        ▼
config/generated/<env>.env
   OTEL_ENABLED / OTLP_TRACES_ENDPOINT / OTLP_LOGS_ENDPOINT / METRICS_SCRAPE_LABEL_PRODUCT
        │  env_file
        ▼
internal/config/config.go   (Config.OTLPTracesEndpoint / OTLPLogsEndpoint / MetricsScrapeLabelProduct)
        │
        ▼
cmd/core/services.go::SetupServices
   if cfg.OTELEnabled {
       observability.Config{…}.Validate()   ← FAIL-LOUD (spec 101 boot gate)
   }
        │  (span export unchanged — reuses …)
        ├─▶ internal/assistant/tracing/tracer.go     (OTLP/gRPC exporter — existing)
        └─▶ internal/api/router.go  /metrics          (promhttp.Handler() — existing)

docker-compose.yml + deploy/compose.deploy.yml
   labels: com.bubbles.product=${METRICS_SCRAPE_LABEL_PRODUCT}, com.bubbles.service=<svc>
        │  Prometheus docker_sd on <deploy-host> (shared stack) scopes by label
        ▼  [DEFERRED-to-flip — operator apply-shared-obs; no live mutation here]
```

## Rollback

All edits are additive or 1:1 field renames. `git revert` the spec-101 commit
restores the prior single `OTEL_EXPORTER_ENDPOINT` naming; the labels are purely
additive (removal restores the prior `com.smackerel.*`-only blocks). No data
migration, no host state touched.
