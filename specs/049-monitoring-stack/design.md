# Design: Monitoring Stack

## Current Truth

Smackerel emits a rich Prometheus metric surface today (spec 030 + spec
050):

- Go core registers metrics in `internal/metrics/` (artifacts ingested,
  search latency, connector sync, NATS dead-letter counters, DB pool
  gauge, intelligence latency, alert delivery, lists, drive
  observability, recommendations, auth metrics).
- ML sidecar registers metrics in `ml/app/metrics.py`
  (`smackerel_ml_processing_latency_seconds`,
  `smackerel_ml_embedding_workers_configured`,
  `smackerel_ml_embedding_inflight`,
  `smackerel_ml_embedding_rejected_total`,
  `smackerel_llm_tokens_used_total`).
- Both services serve `/metrics` (unauthenticated by design, per spec
  030 hard constraint and the Prometheus scrape convention).
- The Go core router (`internal/api/router.go`) excludes `/metrics`
  from the structured logger to keep scrape volume out of the
  application logs.
- `docker-compose.yml` (dev) and `deploy/compose.deploy.yml` (deploy)
  already host `postgres`, `nats`, `smackerel-core`, `smackerel-ml`,
  and `ollama`, each obeying the spec 042 tailnet-edge bind contract
  and the spec 045 read-only + resource hardening contract.

What does NOT exist yet:

- A Prometheus container that actually scrapes those endpoints.
- A committed Prometheus scrape configuration that survives target
  adapter generation.
- A committed alert-rule file that names actionable failure modes by
  the actual metric names emitted by the runtime.
- A documented dashboard inventory + metrics access boundary in
  `docs/Operations.md`.

This spec closes that gap as a generic, target-agnostic deployment
contract.

## Proposed Design

### Architecture Overview

```
                                   monitoring profile (opt-in)
                                   ─────────────────────────────
                                   │  prometheus container       │
                                   │  /etc/prometheus/           │
                                   │    prometheus.yml  (ro)     │
                                   │    alerts.yml      (ro)     │
                                   │  /prometheus       (volume) │
                                   │  /tmp              (tmpfs)  │
                                   ─────────────────────────────
                                       │  scrape (compose network, by service name)
                       ┌───────────────┴───────────────┐
                       ▼                               ▼
               smackerel-core:8080            smackerel-ml:8081
               /metrics                       /metrics
```

Prometheus runs only when the `monitoring` Compose profile is enabled.
It reaches both metric surfaces over the internal compose network by
service name — never by host IP or tailnet identifier. Operator-side
exposure (reverse proxy, Tailscale ACLs, firewall) stays with the
deploy-adapter overlay (per the "No env-specific content" rule).

### Prometheus Scrape Config Template

A committed template at `config/prometheus/prometheus.yml.tmpl` is
rendered, via `scripts/commands/config.sh` and `envsubst`, into
`config/generated/prometheus.yml` and into the deploy bundle as
`prometheus.yml`. Substituted env vars come from the SST source of truth
in `config/smackerel.yaml` (`monitoring.prometheus.*`). The template:

```yaml
# Auto-generated from config/prometheus/prometheus.yml.tmpl by
# scripts/commands/config.sh. Do NOT edit the generated file by hand.
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL_S}s
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL_S}s
  external_labels:
    deployment: smackerel

rule_files:
  - /etc/prometheus/alerts.yml

scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - smackerel-core:${CORE_CONTAINER_PORT}
        labels:
          component: core

  - job_name: smackerel-ml
    metrics_path: /metrics
    static_configs:
      - targets:
          - smackerel-ml:${ML_CONTAINER_PORT}
        labels:
          component: ml
```

Targets are addressed by Docker Compose service name (resolved by the
embedded DNS on the project network), not by IP, hostname, or tailnet
identifier — this is the property the static contract test enforces.

### Alert Rules

A committed file at `config/prometheus/alerts.yml` declares the six
failure classes named in SCN-049-M02:

| Alert name                         | Failure class            | Backing metric                                 |
| ---------------------------------- | ------------------------ | ---------------------------------------------- |
| `SmackerelCoreUnavailable`         | Service unavailability   | `up{job="smackerel-core"}`                     |
| `SmackerelMLUnavailable`           | Service unavailability   | `up{job="smackerel-ml"}`                       |
| `SmackerelIngestionStalled`        | Ingestion failure        | `smackerel_artifacts_ingested_total` rate      |
| `SmackerelNATSDeadLetterPressure`  | NATS queue pressure      | `smackerel_nats_deadletter_total` rate         |
| `SmackerelDBPoolSaturated`         | Database pressure        | `smackerel_db_connections_active` vs max conns |
| `SmackerelMLEmbeddingStarvation`   | ML starvation            | `smackerel_ml_embedding_rejected_total` rate   |
| `SmackerelAlertDeliveryFailing`    | Alert delivery failure   | `smackerel_alert_delivery_failures_total` rate |
| `SmackerelBackupStale`             | Backup failure           | `up{job="smackerel-core"}` + business rule\*   |

\* The backup-failure rule is intentionally conservative: it fires on a
prolonged absence of `smackerel_artifacts_ingested_total` increase
during business-hours window combined with `up` flapping. A target
adapter with a dedicated backup metric should swap in that metric; the
generic rule covers the case where the operator has not yet wired
backup-specific telemetry. The contract test only requires the alert
name to exist and the rule body to reference at least one metric that
is emitted today.

Every metric name in the rules is cross-checked against the live
`internal/metrics/*.go` and `ml/app/metrics.py` files by the
`monitoring_alerts_contract_test.go` test — adding a new alert that
references a hypothetical metric will fail the build.

### Dashboard Inventory

The product surface defines what dashboards MUST exist and which
metrics back them. Operator adapters provision the actual Grafana JSON.
The inventory lives in `docs/Operations.md` under the new "Monitoring
Stack" section (FR-049-002):

| Dashboard               | Purpose                              | Key metrics                                                                        |
| ----------------------- | ------------------------------------ | ---------------------------------------------------------------------------------- |
| Service Health          | up / down + alert summary            | `up`, alert rule states                                                            |
| Ingestion Throughput    | What flowed in over time             | `smackerel_artifacts_ingested_total`, `smackerel_capture_total`                    |
| NATS Pressure           | Queue and DLQ health                 | `smackerel_nats_deadletter_total`, NATS `/jsz` (manual)                            |
| ML Latency & Pool       | Sidecar saturation                   | `smackerel_ml_processing_latency_seconds`, `smackerel_ml_embedding_*`              |
| Postgres Pressure       | Pool saturation + query times        | `smackerel_db_connections_active`                                                  |
| Search Latency          | User-facing query times              | `smackerel_search_latency_seconds`                                                 |
| LLM Usage               | Token spend by provider/model        | `smackerel_llm_tokens_used_total`                                                  |
| Alert Delivery          | Telegram alert health                | `smackerel_alerts_delivered_total`, `smackerel_alert_delivery_failures_total`      |
| Connector Sync          | Per-connector sync health            | `smackerel_connector_sync_total`                                                   |
| Domain Extraction       | Schema-aware extraction success      | `smackerel_domain_extraction_total`, `smackerel_domain_extraction_duration_ms`     |

### Metrics Access Boundary

Captured in `docs/Operations.md` (FR-049-004). The product owns:

- `smackerel-core` `/metrics` (compose service name)
- `smackerel-ml` `/metrics` (compose service name)
- The scrape config + alert rules + dashboard inventory

The deploy adapter owns:

- Whether Prometheus is reachable from outside the host (reverse proxy,
  Tailscale ACLs, firewall).
- Grafana provisioning (datasources, dashboards JSON).
- Alertmanager wiring + receiver routes.

This boundary keeps real hostnames, real tailnet IPs, and real ACL
files out of the product repo (per `.github/copilot-instructions.md`
"No env-specific content").

### Compose Wiring

The `prometheus` service is added to **both** `docker-compose.yml`
(dev) and `deploy/compose.deploy.yml` (deploy bundle) under a
`monitoring` Compose profile so it is opt-in. The service obeys every
hardening invariant already in force:

- **Bind (spec 042 / Gate G028):** Dev compose binds `127.0.0.1:42005`
  literally (the spec-020-equivalent dev form is acceptable in
  `docker-compose.yml` per existing convention); deploy compose uses
  `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${PROMETHEUS_HOST_PORT}:${PROMETHEUS_CONTAINER_PORT}`
  fail-loud SST substitution.
- **Read-only root + tmpfs (spec 045 FR-045-003):** `read_only: true`
  with the explicit tmpfs allowlist `[/tmp]`. The TSDB lives on the
  named volume `prometheus-data` mounted at `/prometheus`.
- **Resource envelope (spec 045 FR-045-001):**
  `deploy.resources.limits.cpus` and `memory` use the fail-loud
  `${PROMETHEUS_CPU_LIMIT:?...}` / `${PROMETHEUS_MEMORY_LIMIT:?...}`
  SST form.
- **Image:** pinned via the SST key `monitoring.prometheus.image`
  exported as `PROMETHEUS_IMAGE` (no `latest`, no fallback default).
- **Security:** `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`.
- **Profile gating:** `profiles: [monitoring]` so the service does not
  start by default.

### SST Pipeline

New config block in `config/smackerel.yaml`:

```yaml
monitoring:
  prometheus:
    image: prom/prometheus:v2.55.1
    container_port: 9090
    scrape_interval_seconds: 15
    evaluation_interval_seconds: 15
    retention_days: 15

deploy_resources:
  prometheus:
    cpus: "1.0"
    memory: "512M"

environments:
  dev:
    prometheus_host_port: 42005
    prometheus_volume_name: smackerel-prometheus-data
  test:
    prometheus_host_port: 47005
    prometheus_volume_name: smackerel-test-prometheus-data
  home-lab:
    prometheus_host_port: 43005
    prometheus_volume_name: smackerel-home-lab-prometheus-data
```

`scripts/commands/config.sh` will:

1. Read every monitoring key with `required_value` (fail-loud on miss).
2. Emit `PROMETHEUS_*` env vars into `config/generated/<env>.env`.
3. Render `config/prometheus/prometheus.yml.tmpl` →
   `config/generated/prometheus.yml` via `envsubst`.
4. Include both `prometheus.yml` (rendered) and `alerts.yml` (static)
   in the deploy bundle alongside `app.env`, `nats.conf`, etc.

The bundle layout becomes:

```
./app.env
./nats.conf
./docker-compose.yml      (= deploy/compose.deploy.yml)
./nats_contract.json
./prometheus.yml          (rendered)
./alerts.yml              (static copy)
./prompt_contracts/...
./bundle-manifest.yaml
```

## Test Strategy

| Test ID    | Type           | File                                                          | Purpose                                                                                                                              |
| ---------- | -------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| T-049-001  | config-static  | `internal/deploy/monitoring_scrape_contract_test.go`          | Parse `config/prometheus/prometheus.yml.tmpl` and assert scrape jobs `smackerel-core` and `smackerel-ml` exist with service-name addressing. |
| T-049-002  | integration    | `internal/deploy/monitoring_render_test.go`                   | Render the template against the test env's env file and validate the result parses as a valid Prometheus config (struct + required fields). |
| T-049-003  | config-static  | `internal/deploy/monitoring_bind_contract_test.go`            | Walk every `ports:` entry in both compose files, assert no service publishes `/metrics` on `0.0.0.0`; assert prometheus bind contract. |
| T-049-004  | config-static  | `internal/deploy/monitoring_alerts_contract_test.go`          | Parse `config/prometheus/alerts.yml`, assert all required alert names exist, every referenced metric is in `internal/metrics/` or `ml/app/metrics.py`. |
| T-049-005  | docs-static    | `internal/deploy/monitoring_docs_contract_test.go`            | Assert `docs/Operations.md` contains the dashboard inventory + access boundary headings + every alert name from `alerts.yml`.       |
| T-049-006  | artifact       | (artifact lint via `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack`) | Lint-clean planning packet.                                                                                                          |

Plus extension of the existing spec 042/045 contract tests to include
the new `prometheus` service:

- `internal/deploy/compose_resource_contract_test.go` — extend
  `servicesUnderResourceContract` with prometheus.
- `internal/deploy/compose_filesystem_contract_test.go` — extend
  `readOnlyAllowlist` with prometheus.
- `internal/deploy/compose_contract_test.go` — extend the backend
  bind invariant to cover prometheus when present.

Each test ships at least one adversarial sub-test that proves the
contract function would FAIL on the most likely regression (per the
existing pattern).

## Risk Controls

- The `monitoring` profile is opt-in so dev and CI workflows do not pay
  the prometheus container cost unless explicitly enabled.
- Prometheus runs hardened (read_only + tmpfs allowlist + resource
  limits + cap_drop ALL + no-new-privileges) like every other service
  in the deploy compose.
- Static contract tests run as part of `./smackerel.sh test unit`
  (Go unit test set), so a regression that strips Prometheus from the
  deploy compose, embeds a real hostname in the scrape config, or
  references a non-existent metric in alerts fails the build before
  any deploy.
- Alert rules use rate-based triggers with sensible `for:` windows so a
  brief blip does not page; thresholds are documented in the alert
  body so operators can tune via overlay without touching the rule
  file.
- Adding a metric to an alert that the runtime does not emit is
  rejected at build time by `monitoring_alerts_contract_test.go`.

## Open Questions (resolved)

- **OQ-049-A:** Should Smackerel ship Grafana JSON dashboards?
  **Resolution:** No. The product ships the inventory + metric names;
  Grafana JSON belongs to the deploy adapter overlay so each adapter
  can pick its own variable/templating style without touching this
  repo.
- **OQ-049-B:** Should Smackerel ship Alertmanager wiring?
  **Resolution:** No. The product ships the alert rules + thresholds;
  receivers, route trees, silencing, and paging integrations belong to
  the deploy adapter overlay (because those names and tokens are
  per-operator).
- **OQ-049-C:** What happens when an alert references a metric that
  the runtime does not emit?
  **Resolution:** `monitoring_alerts_contract_test.go` fails the
  build. The test parses `internal/metrics/*.go` and
  `ml/app/metrics.py` for known metric names, then walks every
  `expr:` field in `alerts.yml`.
