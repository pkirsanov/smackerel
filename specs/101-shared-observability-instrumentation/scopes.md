# Scopes — Spec 101 Shared-Observability Instrumentation Contract

**Feature:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

> Two scopes. SCOPE-01 is the full in-repo instrumentation (contract naming +
> config SST + fail-loud reader + boot gate + `com.bubbles.*` labels + reuse of
> the existing `/metrics` + OTLP exporter) — fully authorable, offline-proven,
> and complete. SCOPE-02 is the LIVE flip confirmation on the shared stack
> (a `/metrics` 200 scraped by the shared Prometheus + OTLP spans in the shared
> Tempo) — an operator-apply action on the knb side, routed to `bubbles.devops`
> as a NON-GATING operational handoff (`followUpOwner: bubbles.devops`). The
> in-repo contract needs zero further code for it, so spec-101 certifies on
> SCOPE-01 + the contract (the accepted WanderAide scope-05 / knb scope-03
> precedent).

---

## Scope 1: SCOPE-01 — In-repo instrumentation contract (naming + fail-loud reader + labels)

**Status:** Done
**Scope-Kind:** contract-only
**Depends On:** —

Adopt the knb 3-var contract across the config SST, add the fail-loud
`internal/observability` reader + the `OTEL_ENABLED`-gated boot validation, add
the `com.bubbles.*` discovery labels to both compose files, and REUSE (cite, not
fork) the existing `/metrics` handler and OTLP/gRPC exporter.

### Gherkin Scenarios

```gherkin
Scenario: SCN-101-A01 — smackerel reads the 3 instrumentation env vars fail-loud
  Given smackerel core boots with OTEL_ENABLED=true
  When OTLP_TRACES_ENDPOINT / OTLP_LOGS_ENDPOINT / METRICS_SCRAPE_LABEL_PRODUCT
      are read from the generated config bundle
  Then observability.Config.Validate() rejects any missing OR empty/whitespace
      value with a named error (no default, no fallback)
  And all-non-empty succeeds
  And with OTEL_ENABLED=false the contract is inert and startup is unaffected

Scenario: SCN-101-A02 — smackerel exposes /metrics with Prometheus exposition (route reused)
  Given smackerel core is running
  When GET /metrics is issued
  Then the route is wired in-repo via internal/api/router.go
      r.Handle("/metrics", metrics.Handler()) → promhttp.Handler()
  And it returns 200 text/plain with process + go runtime metrics
  # LIVE 200 scraped by the shared Prometheus is the SCOPE-02 operator-apply handoff

Scenario: SCN-101-A03 — smackerel containers carry product + service labels
  Given docker-compose.yml and deploy/compose.deploy.yml
  When the shared-observability label contract is asserted
  Then every service with com.smackerel.component also carries a non-empty
      com.bubbles.product (SST-sourced ${METRICS_SCRAPE_LABEL_PRODUCT}) and
      com.bubbles.service
  And an adversarial mutation removing either label is rejected
```

### Implementation plan

1. `config/smackerel.yaml` (`observability:`): replace `otel_exporter_endpoint`
   with `otlp_traces_endpoint` / `otlp_logs_endpoint` (empty dev values) +
   `metrics_scrape_label_product: "smackerel"`.
2. `scripts/commands/config.sh`: read the 3 keys via `required_value`
   (NO-DEFAULTS) and emit them into the generated env; drop `OTEL_EXPORTER_ENDPOINT`.
3. `internal/config/config.go`: replace the `OTELExporterEndpoint` field + loader
   with `OTLPTracesEndpoint` / `OTLPLogsEndpoint` / `MetricsScrapeLabelProduct`.
4. `internal/observability/shared.go` (new): `Config` + `Validate()` + `FromEnv()`
   fail-loud reader + `internal/observability/shared_test.go`.
5. `cmd/core/services.go`: `OTEL_ENABLED`-gated `observability.Config.Validate()`
   boot gate (fail-loud) next to `initAssistantTracing`.
6. `docker-compose.yml` + `deploy/compose.deploy.yml`: add `com.bubbles.product` +
   `com.bubbles.service` labels to every smackerel service;
   `internal/deploy/shared_observability_labels_contract_test.go` (new) locks the invariant.
7. `docs/Operations.md` + `tests/e2e/assistant_regression_e2e_test.sh`: correct the
   obsoleted `OTEL_EXPORTER_ENDPOINT` references.

### Test Plan

| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| unit | unit | `internal/observability/shared_test.go` | SCN-101-A01 — fail-loud 3-var read: unset/empty/whitespace/partial → named error, all-present → success (9 cases) | `./smackerel.sh test unit --go --go-run 'Validate_Accepts\|Validate_Rejects\|FromLookup_\|Constants_MatchKnbCanonicalNames'` |
| unit | unit | `internal/deploy/shared_observability_labels_contract_test.go` | SCN-101-A03 — com.bubbles.product + com.bubbles.service on every smackerel service in both compose files + adversarial reject | `./smackerel.sh test unit --go --go-run 'SharedObservabilityLabels'` |
| unit | unit | `internal/api/health_test.go` | SCN-101-A02 — /metrics route serves Prometheus exposition via the reused promhttp handler (internal/api/router.go) | `./smackerel.sh test unit --go --go-run 'Metrics\|Health'` |

### Consumer Impact Sweep

This scope renames one config surface across three representations
(`internal/config.Config.OTELExporterEndpoint` field, the `OTEL_EXPORTER_ENDPOINT`
env var, and the `observability.otel_exporter_endpoint` SST key) → the knb
3-var contract. Affected consumer surfaces enumerated + reconciled: `config.go`
(field + loader), `config.sh` (read + emit), `smackerel.yaml` (SST key), the 3
generated env files (regenerated), `docs/Operations.md`, and the SCOPE-09 e2e
echo. The old top-level surface had ZERO runtime Go consumers
(declared-but-not-consumed — grep-verified: only doc comments remain). Full
enumeration table + evidence: [report.md](report.md#consumer-sweep).

### Definition of Done

- [x] **SCN-101-A01 — smackerel reads the 3 instrumentation env vars fail-loud** — `internal/observability` unit tests prove unset/empty/whitespace/partial → named error, all-present → success (9 cases). → Evidence: report.md
- [x] **SCN-101-A02 — smackerel exposes /metrics with Prometheus exposition (route reused)** — reused `internal/api/router.go` L62 + `internal/metrics` `promhttp.Handler()`; cited, not forked. → Evidence: report.md
- [x] **SCN-101-A03 — smackerel containers carry product + service labels** — every smackerel service block in both compose files carries `com.bubbles.product` + `com.bubbles.service`; labels contract test GREEN (dev 9/9, deploy 7/7) + adversarial reject. → Evidence: report.md
- [x] **T1 — `go test` on `internal/observability` exit 0** (9 tests PASS). → Evidence: report.md
- [x] **T2 — PII scan clean** — 0 real host/IP/tailnet tokens in added lines. → Evidence: report.md
- [x] **T3 — fail-loud startup (unit level)** — Validate rejects empty on each var + the `OTEL_ENABLED`-gated boot gate compiles. → Evidence: report.md
- [x] **T5 — labels grep/contract clean** — `internal/deploy` labels contract test GREEN. → Evidence: report.md
- [x] **Contract migration verified** — `config generate` exit 0; all 3 env files carry the 3 new vars, `OTEL_EXPORTER_ENDPOINT` gone. → Evidence: report.md
- [x] **Consumer impact sweep complete** — the config-field / env-var / SST-key rename is enumerated; all 6 live consumer surfaces (config.go field+loader, config.sh read+emit, smackerel.yaml SST key, the 3 generated env files, docs/Operations.md, the e2e echo) are reconciled; a post-migration grep confirms zero stale-reference to the old name across runtime code (0 runtime Go consumers; the 4 residual mentions are migration doc-comments), so zero stale first-party references remain. → Evidence: report.md

**Build Quality Gate:**

- [x] Zero warnings — gofmt clean (69 files unchanged), `go vet` "All checks passed!", `config generate` exit 0. → Evidence: report.md
- [x] No default values added for the OTLP vars (fail-loud; `required_value` + `Validate`). → Evidence: report.md
- [x] No duplication — existing exporter + `/metrics` reused, not forked (knb FINDING-014-03-1). → Evidence: report.md
- [x] Existing `com.smackerel.*` labels (incl. spec-082 nats persistent contract) untouched — additive only. → Evidence: report.md

---

## Scope 2: SCOPE-02 — Operational live-verification handoff to bubbles.devops (non-gating)

**Status:** Done
**Scope-Kind:** deploy-pointer
**Depends On:** SCOPE-01

The live shared-stack confirmation (a `/metrics` 200 scraped by the shared
Prometheus with `product=smackerel` visible, and OTLP spans landing in the
shared Tempo) is produced by an operator-apply action on the knb side, not by
smackerel-repo code. This scope's deliverable is the NON-GATING routing of that
operational confirmation to `bubbles.devops` (`followUpOwner: bubbles.devops`):
the in-repo contract (SCOPE-01) is complete, needs zero further smackerel code,
and spec-101 certification does not gate on the live scrape. This is the same
disposition accepted for WanderAide scope-05 / knb scope-03.

### Gherkin Scenarios

```gherkin
Scenario: SCN-101-A04 — live /metrics 200 in the shared Prometheus (operator-apply, routed to bubbles.devops)
  Given smackerel is flipped to sharedServices.observability: shared
  And the operator has run apply-shared-obs on the deploy host
  When the shared Prometheus docker_sd discovers smackerel-core by com.bubbles.product
  Then GET /metrics returns 200 and the product=smackerel scrape label is visible
  And OTLP spans from smackerel land in the shared Tempo
```

### Test Plan

| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| deploy-pointer | operational | `internal/api/health_test.go` | SCN-101-A04 — the in-repo /metrics-serving proof is `internal/api/health_test.go`; the live 200 scraped by the shared Prometheus + OTLP spans in the shared Tempo is an operator-apply confirmation routed NON-GATING to bubbles.devops | `operator runs apply-shared-obs on the deploy host, then confirms the shared Prometheus target set + curl --max-time 5 /metrics` |

### Definition of Done

- [x] **SCN-101-A04 live confirmation recorded as a bubbles.devops handoff** — the live `/metrics` 200 scraped by the shared Prometheus + OTLP spans in the shared Tempo is recorded as a NON-GATING `bubbles.devops` operational confirmation; the in-repo contract is complete and needs zero further smackerel code. → Evidence: report.md
- [x] **Exact unblock action named** — operator sets smackerel `sharedServices.observability: shared` in the knb adapter params + runs `apply-shared-obs` on the deploy host, then confirms the shared Prometheus scrapes `/metrics` (product=smackerel) and OTLP spans reach the shared Tempo. → Evidence: report.md
- [x] **Routing recorded (non-gating)** — state.json routes SCN-101-A04 to `bubbles.devops` via `followUpOwner`; spec-101 certification does not gate on the live scrape. → Evidence: report.md
