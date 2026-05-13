# Scopes: Monitoring Stack

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Prometheus scrape config, SST wiring, metrics access boundary

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-049-M01 Operator sees Smackerel metrics scraped
  Given the monitoring Compose profile is enabled
  When Prometheus starts and reloads its config
  Then it scrapes smackerel-core:${CORE_CONTAINER_PORT}/metrics
  And it scrapes smackerel-ml:${ML_CONTAINER_PORT}/metrics
  And scrape targets are addressed by compose service name, not by IP

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
  And `prometheus` declares `deploy.resources.limits.cpus`/`memory`
    using the fail-loud `${PROMETHEUS_CPU_LIMIT:?...}` /
    `${PROMETHEUS_MEMORY_LIMIT:?...}` SST substitution
```

### Implementation Plan

1. Add `monitoring:` block + `deploy_resources.prometheus` +
   per-env `prometheus_host_port` / `prometheus_volume_name` to
   `config/smackerel.yaml`.
2. Extend `scripts/commands/config.sh` to read every new key with
   `required_value`, emit `PROMETHEUS_*` env vars into
   `config/generated/<env>.env`, render
   `config/prometheus/prometheus.yml.tmpl` →
   `config/generated/prometheus.yml` via `envsubst`, and include both
   `prometheus.yml` (rendered) and `alerts.yml` (static copy) in the
   deploy bundle.
3. Commit `config/prometheus/prometheus.yml.tmpl` template (scrape
   jobs for `smackerel-core` and `smackerel-ml` by compose service
   name; no IPs).
4. Wire the `prometheus` service into `docker-compose.yml` and
   `deploy/compose.deploy.yml` under the `monitoring` Compose profile,
   with spec-042 fail-loud bind, spec-045 read-only root +
   `deploy.resources.limits.cpus`/`memory` fail-loud substitution,
   `cap_drop: [ALL]`, `no-new-privileges`, named volume for the TSDB.
5. Extend `internal/deploy/compose_resource_contract_test.go`,
   `compose_filesystem_contract_test.go`, and `compose_contract_test.go`
   to include `prometheus` in their contract sets.
6. Add new contract tests:
   `internal/deploy/monitoring_scrape_contract_test.go` (T-049-001),
   `internal/deploy/monitoring_render_test.go` (T-049-002),
   `internal/deploy/monitoring_bind_contract_test.go` (T-049-003).
7. Document the metrics access boundary + monitoring profile activation
   in `docs/Operations.md` and `docs/Deployment.md`.

### Test Plan

| ID         | Test Type     | Location                                                | Scenario  | Assertion                                                                                                                  |
| ---------- | ------------- | ------------------------------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------- |
| T-049-001  | config-static | `internal/deploy/monitoring_scrape_contract_test.go`    | SCN-049-M01 | Template declares `smackerel-core` + `smackerel-ml` scrape jobs by service name; no literal IPs/hostnames; rule_files referenced. |
| T-049-002  | integration   | `internal/deploy/monitoring_render_test.go`             | SCN-049-M01 | Rendering the template against the test env's `<env>.env` produces a valid Prometheus config struct with both jobs.        |
| T-049-003  | config-static | `internal/deploy/monitoring_bind_contract_test.go`      | SCN-049-M03 | No service publishes a `/metrics`-serving port on `0.0.0.0` or `network_mode: host`; `prometheus` (when present) obeys the spec-042 bind contract. |
| T-049-007  | config-static | `internal/deploy/compose_resource_contract_test.go`     | SCN-049-M04 | `prometheus` is in `servicesUnderResourceContract` with fail-loud `${PROMETHEUS_*_LIMIT:?...}`.                              |
| T-049-008  | config-static | `internal/deploy/compose_filesystem_contract_test.go`   | SCN-049-M04 | `prometheus` is in `readOnlyAllowlist` with `[/tmp]` as the only allowed tmpfs path.                                         |
| T-049-009  | Regression E2E | `internal/deploy/...` full contract suite             | SCN-049-M01/M03/M04 | Scenario-specific regression: extending spec 030/042/045/046/050 contract sets with `prometheus` does not regress any prior adversarial sub-test; broader regression sweep across `go test ./internal/...` stays green. |

### Definition of Done

- [x] **SCN-049-M01:** `config/smackerel.yaml` has
      `monitoring.prometheus.*`, per-env `prometheus_host_port`,
      per-env `prometheus_volume_name`, so the SST supplies every
      value Prometheus needs to scrape `smackerel-core` and
      `smackerel-ml` by compose service name.
      (Evidence: `config/smackerel.yaml::monitoring.prometheus.*` +
      `environments.<env>.prometheus_host_port` +
      `environments.<env>.prometheus_volume_name`; verified by
      `./smackerel.sh config generate` succeeding in report.md)
- [x] **SCN-049-M01:** `scripts/commands/config.sh` extracts every
      monitoring var via `required_value` and renders
      `config/prometheus/prometheus.yml.tmpl` →
      `config/generated/prometheus.yml` via `envsubst`, so the
      Prometheus config that ends up in the container scrapes both
      jobs from the SST-resolved container ports.
      (Evidence: `scripts/commands/config.sh` envsubst block +
      `config/generated/prometheus.yml` rendered output captured in
      report.md::SST Pipeline Smoke)
- [x] **SCN-049-M01:** `config/prometheus/prometheus.yml.tmpl` is
      committed with both scrape jobs addressed by compose service
      name (`smackerel-core:${CORE_CONTAINER_PORT}` and
      `smackerel-ml:${ML_CONTAINER_PORT}`) — no literal IPs, no
      env-specific hostnames — and `rule_files` references
      `/etc/prometheus/alerts.yml`.
      (Evidence: `config/prometheus/prometheus.yml.tmpl`; locked by
      `internal/deploy/monitoring_scrape_contract_test.go::TestMonitoringScrapeContract_LiveTemplate` PASS)
- [x] **SCN-049-M01:** T-049-001 `TestMonitoringScrapeContract_LiveTemplate`
      passes; adversarial sub-tests reject a missing `smackerel-ml` job
      and a literal-IP target so the contract proves Prometheus
      genuinely scrapes both Smackerel `/metrics` endpoints.
      (Evidence: `internal/deploy/monitoring_scrape_contract_test.go::TestMonitoringScrapeContract_LiveTemplate`
      + `_AdversarialMissingMLJob` + `_AdversarialLiteralIP`
      + `_AdversarialMissingRuleFiles` + `_AdversarialStrayEnvVar` all PASS)
- [x] **SCN-049-M01:** T-049-002 `TestMonitoringRender_LiveTemplate`
      passes; adversarial sub-test rejects an unsubstituted
      `${ML_CONTAINER_PORT}` substitution variable, proving rendering produces
      a valid Prometheus config from the SST.
      (Evidence: `internal/deploy/monitoring_render_test.go::TestMonitoringRender_LiveTemplate`
      + `_AdversarialUnsubstitutedVar` + `_AdversarialInvalidYAML` all PASS)
- [x] **SCN-049-M03:** `deploy/compose.deploy.yml` `prometheus`
      service publishes its host port using the spec 042 fail-loud
      `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy
      adapter}:` prefix (Gate G028 NO-DEFAULTS) so exposure is an
      explicit deploy-adapter decision, not a wildcard bind.
      (Evidence: `deploy/compose.deploy.yml::services.prometheus.ports`;
      locked by `internal/deploy/compose_contract_test.go::TestComposeContract_LiveFile`
      `requiredPrometheusPrefix` check PASS)
- [x] **SCN-049-M03:** `docker-compose.yml` `prometheus` binds to
      `127.0.0.1` only on the dev stack so metrics never leak to
      another host NIC.
      (Evidence: `docker-compose.yml::services.prometheus.ports` line
      uses `127.0.0.1:${PROMETHEUS_HOST_PORT}:${PROMETHEUS_CONTAINER_PORT}`;
      locked by `internal/deploy/monitoring_bind_contract_test.go::TestMonitoringBindContract_LiveDevCompose` PASS)
- [x] **SCN-049-M03:** No service in either `docker-compose.yml`
      or `deploy/compose.deploy.yml` publishes a port on `0.0.0.0`
      or via `network_mode: host`; the spec 042 invariant is
      preserved for the operator-metrics boundary.
      (Evidence: `internal/deploy/monitoring_bind_contract_test.go::TestMonitoringBindContract_LiveDevCompose`
      + `_LiveDeployCompose` both PASS)
- [x] **SCN-049-M03:** T-049-003 `TestMonitoringBindContract` passes
      against both live compose files; adversarial sub-tests reject
      `0.0.0.0:9090:9090`, `[::]:9090:9090`, and the unqualified
      `9090:9090` form so no wildcard bind can ever land.
      (Evidence: `internal/deploy/monitoring_bind_contract_test.go::TestMonitoringBindContract_AdversarialIPv4Wildcard`
      + `_AdversarialIPv6Wildcard` + `_AdversarialUnqualifiedPort` all PASS)
- [x] **SCN-049-M03:** Extended `TestComposeContract_LiveFile`
      (spec 042) blocks regression to a literal-bind or
      default-fallback form on the prometheus port.
      (Evidence: `internal/deploy/compose_contract_test.go::requiredPrometheusPrefix`
      const + conditional check in `assertComposeContract`; PASS)
- [x] **SCN-049-M03:** `docs/Operations.md` "Metrics Access Boundary"
      section names the deploy adapter as owner of reverse-proxy,
      Alertmanager, Grafana, and `HOST_BIND_ADDRESS` selection, while
      the product repo owns `/metrics`, scrape config, and alert
      rules.
      (Evidence: `docs/Operations.md::### Metrics Access Boundary`;
      locked by `internal/deploy/monitoring_docs_contract_test.go::TestMonitoringDocsContract_LiveFile` PASS)
- [x] **SCN-049-M04 Monitoring stack obeys spec 045 hardening:**
      `deploy/compose.deploy.yml` `prometheus`
      declares `read_only: true` with `[/tmp]` as the only tmpfs
      allowlist entry, satisfying spec 045 FR-045-003 (read-only
      root for monitoring service).
      (Evidence: `deploy/compose.deploy.yml::services.prometheus.read_only`
      + `.tmpfs`; locked by `internal/deploy/compose_filesystem_contract_test.go::TestFilesystemContract_LiveFile`
      with `readOnlyAllowlist["prometheus"] = [/tmp]` PASS)
- [x] **SCN-049-M04:** `deploy/compose.deploy.yml` `prometheus`
      declares `deploy.resources.limits.cpus` and `.memory` using
      the fail-loud `${PROMETHEUS_CPU_LIMIT:?...}` and
      `${PROMETHEUS_MEMORY_LIMIT:?...}` substitution form, satisfying
      spec 045 FR-045-001 (resource envelope).
      (Evidence: `deploy/compose.deploy.yml::services.prometheus.deploy.resources.limits`;
      locked by `internal/deploy/compose_resource_contract_test.go::TestComposeResourceContract_LiveFile`
      with `prometheus` in `servicesUnderResourceContract` PASS)
- [x] **SCN-049-M04:** `deploy/compose.deploy.yml` `prometheus`
      drops all capabilities (`cap_drop: [ALL]`), runs as
      `user: nobody`, and sets `no-new-privileges:true`, so
      Prometheus inherits the same hardening posture as
      `smackerel-core` and `smackerel-ml`.
      (Evidence: `deploy/compose.deploy.yml::services.prometheus.cap_drop`
      + `.user` + `.security_opt`; verified via direct file inspection)
- [x] **SCN-049-M04:** Extended `TestComposeResourceContract_LiveFile`
      and `TestFilesystemContract_LiveFile` pass with `prometheus`
      added to the contract set; the spec 045 contracts now cover
      the monitoring stack.
      (Evidence: `go test ./internal/deploy/... -count=1` green;
      adversarial sub-tests `TestComposeResourceContract_AdversarialMissingCPU`
      + `_AdversarialDefaultFallback` etc. all PASS)
- [x] **SCN-049-M01 + SCN-049-M03 + SCN-049-M04:** `./smackerel.sh
      config generate` succeeds for dev and test; the resulting
      `config/generated/dev.env`, `config/generated/prometheus.yml`,
      and deploy bundle staging include all monitoring outputs with
      no missing required values.
      (Evidence: report.md::SST Pipeline Smoke shows
      `Generated config/generated/prometheus.yml` and emitted
      `PROMETHEUS_*` env vars)
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are committed and pass:
      the 5 new contract test files plus the 3 extended contract sets
      (compose, resource, filesystem) cover every Gherkin scenario
      (SCN-049-M01/M03/M04) with at least one adversarial sub-test per
      behavior, so any regression in scrape config, bind boundary, or
      hardening posture would fail CI.
      (Evidence: `go test ./internal/deploy/... -count=1` lists 5 new
      `TestMonitoring*` + extended `TestComposeContract` /
      `TestComposeResourceContract` / `TestFilesystemContract` PASS;
      report.md::Adversarial Regression Tests inventories 14 sub-tests)
- [x] Broader E2E regression suite passes after this scope's changes:
      `go test ./internal/...` for ALL packages stays green
      (no regressions in spec 030/042/045/046/050 contract tests, no
      regressions in any sibling package).
      (Evidence: report.md::Build + Vet + Format shows every
      `internal/...` package green; no `FAIL` lines emitted)

## Scope 2: Alert rules, dashboard inventory, monitoring operator docs

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
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
```

### Implementation Plan

1. Commit `config/prometheus/alerts.yml` with the eight rules named in
   the Gherkin (six failure classes named in FR-049-003 + an
   `AlertDeliveryFailing` covering the alert-pipeline-itself class +
   `BackupStale` for the backup class).
2. Add `internal/deploy/monitoring_alerts_contract_test.go` (T-049-004)
   that parses `alerts.yml` and asserts every required alert name
   exists, AND that every metric name referenced in an `expr:` field is
   actually emitted by the live runtime (cross-checked against
   `internal/metrics/*.go` and `ml/app/metrics.py`).
3. Add `internal/deploy/monitoring_docs_contract_test.go` (T-049-005)
   that parses `docs/Operations.md` and asserts the dashboard
   inventory headings + every alert name appear.
4. Update `docs/Operations.md` with the new "Monitoring Stack" section
   covering: dashboard inventory, alert runbooks, access boundary, and
   how to enable the `monitoring` Compose profile.
5. Update `docs/Deployment.md` with a section on enabling the
   `monitoring` profile and the operator-adapter responsibilities
   (Alertmanager, Grafana, reverse proxy).
6. Run artifact-lint, traceability-guard, and regression-baseline-guard
   for spec 049.

### Test Plan

| ID         | Test Type     | Location                                                  | Scenario  | Assertion                                                                                                                                            |
| ---------- | ------------- | --------------------------------------------------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| T-049-004  | config-static | `internal/deploy/monitoring_alerts_contract_test.go`      | SCN-049-M02 | All required alert names exist; every metric in an `expr:` is in the known-metrics set extracted from `internal/metrics/*.go` and `ml/app/metrics.py`. |
| T-049-005  | docs-static   | `internal/deploy/monitoring_docs_contract_test.go`        | SCN-049-M02 | `docs/Operations.md` contains the "Monitoring Stack" section, the dashboard inventory table, the access boundary heading, and every alert name.       |
| T-049-006  | artifact      | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack` | all | Artifact lint passes for this feature folder.                                                                                                        |
| T-049-010  | Regression E2E | `internal/deploy/monitoring_alerts_contract_test.go` + `internal/deploy/monitoring_docs_contract_test.go` | SCN-049-M02 | Scenario-specific regression: every required alert + every dashboard heading + every metric reference stays under contract; broader regression sweep across `go test ./internal/...` stays green. |

### Definition of Done

- [x] **SCN-049-M02:** `config/prometheus/alerts.yml` is committed
      with the eight required alert rules:
      `SmackerelCoreUnavailable` (references `up` on smackerel-core),
      `SmackerelMLUnavailable` (references `up` on smackerel-ml),
      `SmackerelIngestionStalled` (references
      `smackerel_artifacts_ingested_total`),
      `SmackerelNATSDeadLetterPressure` (references
      `smackerel_nats_deadletter_total`),
      `SmackerelDBPoolSaturated` (references
      `smackerel_db_connections_active`),
      `SmackerelMLEmbeddingStarvation` (references
      `smackerel_ml_embedding_rejected_total`),
      `SmackerelAlertDeliveryFailing` (references
      `smackerel_alert_delivery_failures_total`),
      `SmackerelBackupStale` (references the backup-class metric).
      (Evidence: `config/prometheus/alerts.yml`; locked by
      `internal/deploy/monitoring_alerts_contract_test.go::TestMonitoringAlertsContract_LiveFile`
      `requiredAlerts` check PASS)
- [x] **SCN-049-M02:** Every `expr:` field in `alerts.yml`
      references a metric name that is actually emitted by the live
      runtime — extracted from `internal/metrics/*.go` for the Go
      core or `ml/app/metrics.py` for the Python sidecar, or the
      Prometheus builtin `up` — so no fabricated metrics ever land.
      (Evidence: `internal/deploy/monitoring_alerts_contract_test.go::loadKnownEmittedMetrics`
      walks the runtime sources and `assertAlertsContract` matches
      every expr; live PASS extracts a 54-entry metric set)
- [x] **SCN-049-M02:** T-049-004 `TestMonitoringAlertsContract_LiveFile`
      passes; adversarial sub-test
      `TestMonitoringAlertsContract_AdversarialFabricatedMetric`
      rejects a non-existent metric reference; adversarial sub-test
      `TestMonitoringAlertsContract_AdversarialMissingRequiredAlert`
      rejects a file with a required alert silently dropped.
      (Evidence: `internal/deploy/monitoring_alerts_contract_test.go::TestMonitoringAlertsContract_LiveFile`
      + `_AdversarialFabricatedMetric` + `_AdversarialMissingRequiredAlert`
      + `_AdversarialEmptyExpr` all PASS)
- [x] **SCN-049-M02:** T-049-005
      `TestMonitoringDocsContract_LiveFile` passes; adversarial
      sub-tests reject a dropped `### Dashboard Inventory` heading
      and a new alert that lacks a runbook row in
      `docs/Operations.md`, so the operator-runbook surface stays
      complete.
      (Evidence: `internal/deploy/monitoring_docs_contract_test.go::TestMonitoringDocsContract_LiveFile`
      + `_AdversarialMissingHeading` + `_AdversarialMissingAlertMention` all PASS)
- [x] **SCN-049-M02:** `docs/Operations.md` contains a "Monitoring
      Stack" section with the dashboard inventory table (10
      dashboards naming their backing metrics), the alert runbook
      table (one row per alert mapping name + severity + firing
      action), and the explicit metrics access boundary table that
      assigns reverse-proxy/Alertmanager/Grafana to the deploy
      adapter and `/metrics`/scrape-config/alert-rules to the
      product.
      (Evidence: `docs/Operations.md::## Monitoring Stack` ->
      `### Dashboard Inventory`, `### Alert Runbook`,
      `### Metrics Access Boundary`; locked by
      `internal/deploy/monitoring_docs_contract_test.go::requiredOperationsHeadings`
      check PASS)
- [x] **SCN-049-M02:** `docs/Deployment.md` documents how to enable
      the `monitoring` Compose profile (the `--profile
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are committed and pass:
      `TestMonitoringAlertsContract_LiveFile` and
      `TestMonitoringDocsContract_LiveFile` each ship with adversarial
      sub-tests (fabricated metric, missing required alert, empty expr,
      missing heading, missing alert mention) so any regression in
      alert rules or operator docs would fail CI.
      (Evidence: `internal/deploy/monitoring_alerts_contract_test.go`
      + `internal/deploy/monitoring_docs_contract_test.go`; 7
      adversarial sub-tests catalogued in
      report.md::Adversarial Regression Tests; all PASS)
- [x] Broader E2E regression suite passes after this scope's changes:
      every previously-shipped contract test still passes after
      `prometheus` was added to `requiredAlerts`, `requiredOperationsHeadings`,
      and the live-file contract sets; no regression in any
      `internal/...` package.
      (Evidence: `go test ./internal/... -count=1` -> all packages
      green; no `FAIL` lines emitted) monitoring`
      flag), what artifacts this repo ships (template + alerts +
      service definition + docs), and what the deploy-adapter
      overlay owns (Alertmanager, Grafana, reverse-proxy fronting,
      `HOST_BIND_ADDRESS` selection).
      (Evidence: `docs/Deployment.md::## Monitoring Profile (Spec 049 — Optional)`
      section; direct file inspection)
- [x] **SCN-049-M02:** `artifact-lint` passes for
      `specs/049-monitoring-stack`; `traceability-guard` passes;
      `regression-baseline-guard` passes; the spec and design
      docs match this scope.
      (Evidence: `bash .github/bubbles/scripts/artifact-lint.sh
      specs/049-monitoring-stack` -> PASS;
      `bash .github/bubbles/scripts/traceability-guard.sh
      specs/049-monitoring-stack` -> RESULT: PASSED (0 warnings);
      `bash .github/bubbles/scripts/regression-baseline-guard.sh
      specs/049-monitoring-stack --verbose` -> PASSED)
