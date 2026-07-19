# Spec 101 — Shared-Observability Instrumentation Contract (knb spec 014 scope 03)

**Status:** done
**Workflow mode:** full-delivery · **Status ceiling:** done
**Release train:** mvp
**Relates to:** [030-observability](../030-observability/spec.md) (existing Prometheus `/metrics` + W3C trace propagation this reconciles with), [061-conversational-assistant](../061-conversational-assistant/spec.md) (the existing OTLP/gRPC exporter this reuses, not forks), knb [014-shared-host-observability](../../../knb/specs/014-shared-host-observability/spec.md) scope 03 (the acceptance contract), knb spec-032 shared-services selector (the `observability: shared|bundled` posture that flips this on)

## Problem

<deploy-host> now runs a LIVE shared self-hosted observability stack (one Prometheus,
one Grafana, one Tempo, one Loki, one otel-collector) owned by the knb adapter
`shared/observability/self-hosted/`. knb spec 014 **scope 03** is the acceptance
contract that makes smackerel *flip-ready* for that stack: smackerel must
consume the three canonical instrumentation env vars the knb adapter injects
(`OTLP_TRACES_ENDPOINT`, `OTLP_LOGS_ENDPOINT`, `METRICS_SCRAPE_LABEL_PRODUCT`),
fail loud on a missing/empty value, expose Prometheus `/metrics`, and carry
`com.bubbles.product` + `com.bubbles.service` discovery labels so the shared
Prometheus `docker_sd` scopes smackerel by label.

**This is a RECONCILIATION, not greenfield.** A current-truth audit (design.md
§ Current Truth) found smackerel already ships most of scope 03:

- `/metrics` **already exists** — `internal/api/router.go` (`r.Handle("/metrics",
  metrics.Handler())`) + `internal/metrics` (`promhttp.Handler()`).
- A mature OTLP/gRPC span exporter **already exists** —
  `internal/assistant/tracing/tracer.go` (real `otlptracegrpc`, no-op fallback,
  fail-loud boot probe in `cmd/core/wiring.go`, shutdown flush, tests). knb
  `docs/Observability.md` explicitly recommends **reusing** it, not forking.
- The otel-go SDK is already in `go.mod`.

The genuine gaps are: (1) the **env-var contract naming** — smackerel had a
declared-but-**not-consumed** single `OTEL_EXPORTER_ENDPOINT` (zero Go consumers,
grep-verified), not the knb 3-var contract; (2) a **fail-loud service-tier
reader** of the 3 vars; (3) the **`com.bubbles.*` discovery labels** (smackerel
used only `com.smackerel.*`). The knb scope-03 report already ratified the fix:
operator decision **option (a)** — smackerel adopts the knb 3-var naming — with a
turnkey migration, and the knb adapter side needs **zero rework** (it already
injects exactly those 3 vars). Forking a second exporter is explicitly rejected
(knb FINDING-014-03-1: "no duplication / single source of truth").

## Goal

Make smackerel **instrumentation-complete and flip-ready** for the shared
observability stack by closing ONLY the genuine gaps, reusing the existing
exporter + `/metrics`, and adopting the knb canonical contract — without
forking a `done` subsystem, without any live <deploy-host> mutation, and without
breaking bundled/dev/test startup (the contract is inert unless `OTEL_ENABLED=true`).

## Capability Proportionality

### Single-Capability Justification

This spec delivers exactly ONE capability — the smackerel service-tier
consumption of the knb spec-014 shared-observability instrumentation contract
(the three canonical env vars + fail-loud validation + `com.bubbles.*` discovery
labels). It is deliberately NOT a capability foundation with multiple providers,
adapters, strategies, or variants: design decision D1 (knb FINDING-014-03-1)
REJECTS forking a second exporter and instead REUSES the single existing
OTLP/gRPC span exporter (`internal/assistant/tracing`) and the single existing
Prometheus `/metrics` handler (`internal/api/router.go`). There is one contract,
one fail-loud reader, one boot gate, and one label convention — no variation
axis and no second implementation. A capability-foundation /
concrete-implementations / variation-axes decomposition would be
over-engineering for a single-consumer reconciliation. The proportionality
trigger words in this spec ("adapter", "shared", "labels", "contract") describe
the knb-owned upstream and the reused subsystems, not new smackerel-side
variants introduced here.

## Requirements

- **FR-101-01** — smackerel adopts the knb spec-014 canonical env-var contract
  (`OTLP_TRACES_ENDPOINT`, `OTLP_LOGS_ENDPOINT`, `METRICS_SCRAPE_LABEL_PRODUCT`),
  REPLACING the prior declared-not-consumed single `OTEL_EXPORTER_ENDPOINT`
  across the config SST (`config/smackerel.yaml` → `scripts/commands/config.sh`
  generator → generated env → `internal/config/config.go`).
- **FR-101-02** — a service-tier `internal/observability` package reads and
  FAIL-LOUD validates the 3 vars: a missing OR empty/whitespace value returns a
  named error (no default, no fallback — smackerel NO-DEFAULTS SST).
- **FR-101-03** — the contract is wired into core boot gated on `OTEL_ENABLED`:
  when `true`, the 3 vars MUST resolve non-empty or startup aborts; when `false`
  (bundled/dev/test), the contract is inert and startup is unaffected.
- **FR-101-04** — every smackerel-owned container in BOTH `docker-compose.yml`
  and `deploy/compose.deploy.yml` carries `com.bubbles.product`
  (SST-sourced `${METRICS_SCRAPE_LABEL_PRODUCT}`) + `com.bubbles.service`
  (per-service identity), additively preserving the existing `com.smackerel.*`
  labels.
- **FR-101-05** — the existing Prometheus `/metrics` endpoint
  (`internal/api/router.go`) and the existing OTLP/gRPC span exporter
  (`internal/assistant/tracing`) are REUSED and cited, NOT duplicated
  (knb FINDING-014-03-1).
- **FR-101-06** — no real hostname, IP, or tailnet identifier is committed to
  the smackerel repo by this spec; only env-var NAMES + empty dev placeholders +
  generic in-cluster DNS example names (spec 014 FR-013 / AC-009).

## Scenarios

See [scenario-manifest.json](scenario-manifest.json). Three scenarios mirror the
knb scope-03 Gherkin:

- **SCN-101-A01** — smackerel reads the 3 instrumentation env vars fail-loud (SCOPE-01).
- **SCN-101-A02** — smackerel exposes `/metrics` with Prometheus exposition; the
  route + handler exist in-repo (SCOPE-01), and a LIVE 200 from the shared
  Prometheus is apply-gated (SCOPE-02).
- **SCN-101-A03** — every smackerel container carries `com.bubbles.product` +
  `com.bubbles.service` (SCOPE-01).
- **SCN-101-A04** — live `/metrics` 200 scraped by the shared Prometheus +
  OTLP spans in the shared Tempo (SCOPE-02) — **DEFERRED-to-flip**.

## Operational live-verification handoff (SCOPE-02, non-gating)

- **Live verification on the shared stack** (SCOPE-02) is a NON-GATING
  operational handoff to `bubbles.devops`, NOT a smackerel-repo code
  deliverable. The in-repo instrumentation contract (all six FRs, SCN-101-A01
  / A02 / A03) is complete and offline-proven; the live confirmation — a
  `/metrics` 200 scraped by the shared Prometheus with `product=smackerel`
  plus OTLP spans reaching the shared Tempo — is produced by an operator
  action on the knb side (set smackerel `sharedServices.observability: shared`
  in the knb adapter params + run `apply-shared-obs` on the deploy host).
  smackerel needs zero further code for it, and spec-101 certification does not
  gate on it.
- **knb adapter changes**: none required (already injects the 3 vars; option (a)).
