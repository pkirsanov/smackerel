# Scopes — Spec 101 Shared-Observability Instrumentation Contract

**Feature:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

> Two scopes. SCOPE-01 is the full in-repo instrumentation (contract naming +
> config SST + fail-loud reader + boot gate + `com.bubbles.*` labels + reuse of
> the existing `/metrics` + OTLP exporter) — fully authorable, offline-provable,
> and complete this session. SCOPE-02 is the LIVE flip verification on <deploy-host>
> (a `/metrics` 200 scraped by the shared Prometheus + OTLP spans in the shared
> Tempo) — operator/apply-gated and explicitly **DEFERRED-to-flip** (no live host
> mutation this session, per the acceptance constraints and the accepted
> WanderAide scope-05 / knb scope-03 precedent).

---

## Scope 1: SCOPE-01 — In-repo instrumentation contract (naming + fail-loud reader + labels)

**Status:** Done
**Scope-Kind:** product-repo (smackerel)
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
  # LIVE 200 scraped by the shared Prometheus is SCOPE-02 (deferred-to-flip)

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
   with `otlp_traces_endpoint` / `otlp_logs_endpoint` (empty placeholders) +
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

- [x] **SCN-101-A01 — fail-loud 3-var read** — `internal/observability` unit tests
      prove unset/empty/whitespace/partial → error, all-present → success. Evidence: [report.md](report.md#a01-fail-loud)
- [x] **SCN-101-A02 (route side) — /metrics exists** — reused `internal/api/router.go`
      L62 + `internal/metrics` `promhttp.Handler()`; cited, not forked. Evidence: [report.md](report.md#a02-metrics-route)
- [x] **SCN-101-A03 — com.bubbles.* labels** — labels contract test GREEN on both
      compose files (dev 9/9, deploy 7/7) + adversarial reject. Evidence: [report.md](report.md#a03-labels)
- [x] **T1 — `go test` on `internal/observability` exit 0** (9 tests PASS). Evidence: [report.md](report.md#unit-observability)
- [x] **T2 — PII scan clean** — 0 real host/IP/tailnet tokens in added lines. Evidence: [report.md](report.md#t2-pii)
- [x] **T3 — fail-loud startup (unit level)** — Validate rejects empty on each var + the `OTEL_ENABLED`-gated boot gate compiles. Evidence: [report.md](report.md#a01-fail-loud)
- [x] **T5 — labels grep/contract clean** — `internal/deploy` labels contract test GREEN. Evidence: [report.md](report.md#unit-labels)
- [x] **Contract migration verified** — `config generate` exit 0; all 3 env files carry the 3 new vars, `OTEL_EXPORTER_ENDPOINT` gone. Evidence: [report.md](report.md#config-generate)
- [x] **Consumer Impact Sweep** — config-field / env-var / SST-key rename enumerated; all 6 live consumer surfaces reconciled; 0 runtime Go consumers of the old field (grep-verified). Evidence: [report.md](report.md#consumer-sweep)

**Build Quality Gate:**

- [x] Zero warnings — gofmt clean (69 files unchanged), `go vet` "All checks passed!", `config generate` exit 0. Evidence: [report.md](report.md#quality)
- [x] No default values added for the OTLP vars (fail-loud; `required_value` + `Validate`). Evidence: [report.md](report.md#a01-fail-loud)
- [x] No duplication — existing exporter + `/metrics` reused, not forked (knb FINDING-014-03-1). Evidence: [design.md](design.md#d1--reconcile-do-not-duplicate-knb-finding-014-03-1)
- [x] Existing `com.smackerel.*` labels (incl. spec-082 nats persistent contract) untouched — additive only. Evidence: [report.md](report.md#a03-labels)

---

## Scope 2: SCOPE-02 — Live shared-stack verification (<deploy-host>)

**Status:** Blocked
**Scope-Kind:** live-host-verification (operator/apply-gated)
**Depends On:** SCOPE-01

Prove the flip works against the LIVE shared observability stack on <deploy-host>: a
`/metrics` 200 scraped by the shared Prometheus with the `product=smackerel`
label visible, and OTLP spans landing in the shared Tempo.

### Gherkin Scenarios

```gherkin
Scenario: SCN-101-A04 — live /metrics 200 in the shared Prometheus (deferred-to-flip)
  Given smackerel is flipped to sharedServices.observability: shared
  And the operator has run apply-shared-obs on <deploy-host>
  When the shared Prometheus docker_sd discovers smackerel-core by com.bubbles.product
  Then GET /metrics returns 200 and the product=smackerel scrape label is visible
  And OTLP spans from smackerel land in the shared Tempo
```

### Test Plan

| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| integration | integration | `internal/api/health_test.go` | SCN-101-A04 — live /metrics 200 scraped by the shared Prometheus + OTLP spans in the shared Tempo; DEFERRED-to-flip (the in-repo /metrics-serving proof is health_test.go; the live scrape is operator-gated) | `[DEFERRED-to-flip] operator runs apply-shared-obs on <deploy-host>, then verifies the shared Prometheus target set + curl --max-time 5 /metrics` |

### Definition of Done

- [ ] **T4 — live `/metrics` 200 scraped by the shared Prometheus** — **DEFERRED-to-flip.** Unblock: operator sets smackerel `sharedServices.observability: shared` in the knb adapter params + runs `apply-shared-obs` on <deploy-host>; this session performs NO live host mutation. Evidence: [report.md](report.md#deferred-live)
- [ ] **Live OTLP export lands in the shared Tempo** — **DEFERRED-to-flip.** Same unblock condition. Evidence: [report.md](report.md#deferred-live)
- [ ] **`product=smackerel` label visible in the shared Prometheus target set** — **DEFERRED-to-flip.** Same unblock condition. Evidence: [report.md](report.md#deferred-live)
