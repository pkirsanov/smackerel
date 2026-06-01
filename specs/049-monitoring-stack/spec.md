# Feature: Monitoring Stack

**Status:** Done (certified per state.json)

> **Successor Notice (added 2026-05-31, analyst).**
> The monitoring stack contract (Prometheus scrape targets, dashboards,
> alert routing, retention) is unchanged. New assistant-side metrics
> introduced by
> [spec 064 — Open-Ended Knowledge Agent](../064-open-ended-knowledge-agent/spec.md)
> (refusal causes, cite-back verification counters, per-turn budgets)
> are exported through the existing pipeline declared here. Future
> intent-compiler metrics from
> [spec 068 — Structured Intent Compiler](../068-structured-intent-compiler/spec.md)
> (compiler error totals, action-class distribution, clarification
> rate) will follow the same contract. This spec stays `done`; the
> additions amend metric names only, not the monitoring contract.

## Status

In Progress — implementation

## Review Findings

- D-014: Monitoring stack readiness is not fully planned as a generic deployment contract.
- SEC-DEP-005: Metrics endpoints and dashboards need access-boundary clarity without embedding target-specific exposure rules.

## Outcome Contract

**Intent:** Provide a concrete, generic, SST-driven Prometheus + alert-rule
contract for Smackerel deployments so any target adapter can stand up a
monitoring stack against the running `smackerel-core` and `smackerel-ml`
metrics surfaces without hand-edited Prometheus configuration.

**Success Signal:** A target adapter can enable an optional `monitoring`
Compose profile and Prometheus comes up scraping
`smackerel-core:${CORE_CONTAINER_PORT}/metrics` and
`smackerel-ml:${ML_CONTAINER_PORT}/metrics` over the internal compose
network. Alert rules covering service unavailability, ingestion failure,
queue pressure, database pressure, ML starvation, and backup failure
evaluate against the existing `smackerel_*` metric surface (spec 030) and
spec-050 embedding-pool metrics. Dashboard inventory and ownership are
documented; target-specific Grafana provisioning and Alertmanager wiring
stay in the deploy-adapter overlay.

**Hard Constraints:**

- Product artifacts MUST NOT encode concrete host exposure decisions, real
  hostnames, real IPs, or real tailnet identifiers (per
  `.github/copilot-instructions.md` "No env-specific content").
- All runtime values flow through the SST pipeline at
  `config/smackerel.yaml` → `scripts/commands/config.sh` →
  `config/generated/*.env` + `config/generated/prometheus.yml`. Hidden
  defaults and `${VAR:-fallback}` syntax are FORBIDDEN.
- Prometheus runs behind a `monitoring` Compose profile so it does not
  cost dev resources unless explicitly enabled.
- Prometheus inherits spec 042 (tailnet-edge bind) + spec 045
  (resource/filesystem hardening) invariants: published host port uses
  `${HOST_BIND_ADDRESS:?...}` substitution in `deploy/compose.deploy.yml`,
  `read_only: true` with an explicit tmpfs allowlist, and explicit
  `deploy.resources.limits.cpus`/`memory` substitution.
- The `smackerel-core` and `smackerel-ml` `/metrics` endpoints stay bound
  to the existing per-service host-bind addresses (spec 042); the
  monitoring stack MUST reach them over the compose-internal network by
  service name, not via the host bind.
- Alert rules MUST reference existing metric names from
  `internal/metrics/` and `ml/app/metrics.py`; they MUST NOT introduce
  hypothetical metrics that aren't actually emitted.
- Alertmanager wiring, Grafana provisioning, paging integrations, and
  per-target reverse-proxy fronting stay in the deploy-adapter overlay
  repo; this spec does NOT ship them.

**Failure Condition:** Operators have to hand-write a Prometheus config
that names individual service hostnames or ports; alert rules silently
drift away from real metric names; the monitoring container regresses
any of the spec 042/045 deployment invariants.

## Requirements

- **FR-049-001:** Ship a generic Prometheus scrape configuration template
  (`config/prometheus/prometheus.yml.tmpl`) that renders, via the SST
  pipeline, into `config/generated/prometheus.yml` with scrape jobs for
  `smackerel-core` and `smackerel-ml` addressing them by compose service
  name plus their SST-bound container ports.
- **FR-049-002:** Document the dashboard inventory in
  `docs/Operations.md` (service health, ingestion throughput, NATS
  pressure, ML latency/embedding pool, Postgres pressure, alert delivery,
  backup status), naming the underlying `smackerel_*` metrics for each
  panel. Grafana JSON provisioning is explicitly an operator-adapter
  concern; this product spec ships the inventory and ownership boundary.
- **FR-049-003:** Ship a committed alert-rule file
  (`config/prometheus/alerts.yml`) containing at minimum one rule per
  failure class named in the user scenario: service unavailability,
  ingestion failure, NATS queue pressure, Postgres pressure, ML
  starvation/backpressure, and backup failure. Every rule MUST reference
  metric names that are actually emitted by the live runtime.
- **FR-049-004:** Document the metrics access boundary in
  `docs/Operations.md`: which `/metrics` endpoints are product-owned
  (`smackerel-core`, `smackerel-ml`), how Prometheus reaches them inside
  the compose network, and how host-level exposure (reverse proxy,
  Tailscale ACLs, firewall) stays in the deploy-adapter overlay.
- **FR-049-005:** Add static contract tests that prove:
  (a) the template `prometheus.yml.tmpl` declares both `smackerel-core`
  and `smackerel-ml` scrape jobs by service name with no embedded
  hostnames/IPs;
  (b) the `alerts.yml` file declares every alert class named in
  FR-049-003 and every referenced metric is actually emitted by the
  live runtime (cross-checked against `internal/metrics/` and
  `ml/app/metrics.py`);
  (c) the `prometheus` Compose service in `deploy/compose.deploy.yml`
  inherits the spec 042 fail-loud `${HOST_BIND_ADDRESS:?...}` host-port
  substitution, spec 045 `read_only` + tmpfs allowlist, and
  `deploy.resources.limits.cpus`/`memory` fail-loud
  `${PROMETHEUS_*_LIMIT:?...}` substitution;
  (d) the `smackerel-core` and `smackerel-ml` `/metrics` endpoints
  continue to be served only on the per-service host-bind addresses
  (no `0.0.0.0` broad-bind regression).

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-049-M01 Operator sees Smackerel metrics scraped
  Given the monitoring Compose profile is enabled
  When Prometheus starts and reloads its config
  Then it scrapes smackerel-core:${CORE_CONTAINER_PORT}/metrics
  And it scrapes smackerel-ml:${ML_CONTAINER_PORT}/metrics
  And scrape targets are addressed by compose service name, not by IP

Scenario: SCN-049-M02 Alerts cover deployment failure modes
  Given Prometheus has loaded config/prometheus/alerts.yml
  When the rule engine evaluates the rule groups
  Then a SmackerelCoreUnavailable rule exists and references `up`
    on the smackerel-core job
  And a SmackerelMLUnavailable rule exists and references `up`
    on the smackerel-ml job
  And a SmackerelIngestionStalled rule references
    `smackerel_artifacts_ingested_total`
  And a SmackerelNATSDeadLetterPressure rule references
    `smackerel_nats_deadletter_total`
  And a SmackerelDBPoolSaturated rule references
    `smackerel_db_connections_active`
  And a SmackerelMLEmbeddingStarvation rule references
    `smackerel_ml_embedding_rejected_total`
  And a SmackerelBackupStale rule exists for the backup failure class

Scenario: SCN-049-M03 Metrics endpoints remain inside the operator boundary
  Given the live deploy compose file is inspected
  When a static contract test walks every service's `ports:` block
  Then `smackerel-core` and `smackerel-ml` published ports start with the
    spec 042 `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
    fail-loud SST prefix
  And no service publishes its container port on `0.0.0.0` or via
    `network_mode: host`
  And Prometheus inherits the same fail-loud bind contract when its
    profile is enabled

Scenario: SCN-049-M04 Monitoring stack obeys spec 045 hardening
  Given the live deploy compose file is inspected
  When the spec 045 read-only + resource contract is evaluated
  Then `prometheus` declares `read_only: true` with only the documented
    tmpfs allowlist
  And `prometheus` declares
    `deploy.resources.limits.cpus`/`memory` using the fail-loud
    `${PROMETHEUS_CPU_LIMIT:?...}` / `${PROMETHEUS_MEMORY_LIMIT:?...}`
    SST substitution
```

## Product Principle Alignment

This spec supports Principle 6 (Invisible by Default, Felt Not Heard) by
keeping monitoring off the dev critical path (`monitoring` profile is
off by default) and only firing when an actionable failure mode is
detected. It supports Principle 8 (Trust Through Transparency) by
attaching named, source-linked metrics to every alert class so an
operator can always answer "what fired and why".

## Non-Goals

- Installing host reverse proxies, VPN/network overlays, or firewall
  rules from Smackerel source.
- Choosing a managed observability vendor (Datadog, NewRelic, etc.).
- Building a custom Smackerel monitoring UI.
- Shipping Grafana JSON dashboards. The inventory contract is the
  product-owned surface; Grafana provisioning is operator-adapter work.
- Wiring Alertmanager receivers (Telegram, PagerDuty, email). The alert
  rules are the product-owned surface; receivers stay with the operator.
