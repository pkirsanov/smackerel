# Design 090 — Observability SLO Dogfood (core /api/health)

## Context

This design propagates the Bubbles IMP-001 observability pattern (already
proven in QuantitativeFinance spec 085) into Smackerel as the **second** wired
downstream. It graduates the Go core `/api/health` liveness endpoint into a
gated SLO contract backed by genuinely captured runtime telemetry.

The mechanism is the Bubbles G100 gate
(`.github/bubbles/scripts/observability-slo-guard.sh`). G100 resolves like this:

1. Read `.github/bubbles-project.yaml` `traceContracts.observability.posture`.
   `wired` + `schemaVersion: 1` arms the gate.
2. Collect workflows under `traceContracts.workflows.*` that carry an `slo:`
   link.
3. ENFORCE **only if** a scope under `specs/` declares
   `observabilityWorkflow: <wf>` (whole-key anchored). Otherwise green no-op.
4. Resolve the target from `traceContracts.observability.slos.<sloKey>`.
5. Require `.specify/runtime/observability/<wf>.slo.json`; missing = BLOCK.
6. Assert `observed.latencyP99Ms <= target`, `observed.errorRatePct <=
   target`, `observed.availabilityPct >= target`. A contract-declared metric
   absent from `observed` is a BREACH, not a skip.

## Decision

### Workflow choice: `core.health`

The `/api/health` endpoint (`internal/api/router.go` — `r.Get("/health",
deps.HealthHandler)` mounted under the `/api` prefix) is the cleanest dogfood
target:

- It already exists and runs in the standard dev stack — no new server code.
  The compose `smackerel-core` healthcheck itself probes
  `http://localhost:${CORE_CONTAINER_PORT}/api/health`, so the container's
  reported health depends on this endpoint returning 2xx.
- It is the canonical liveness probe, so a tight SLO (p99 ≤ 200ms, err ≤ 0.1%,
  avail ≥ 99.9%) is meaningful and matches the QF `simulation.health`
  precedent.

### Target values

| Metric | Target | Rationale |
|--------|--------|-----------|
| `latencyP99Ms` | 200 | A liveness probe should answer well under 200ms p99 on the dev host. |
| `errorRatePct` | 0.1 | A healthy liveness probe approaches a 0% error floor; 0.1% leaves headroom for a single transient blip in a 600-request run. |
| `availabilityPct` | 99.9 | Mirror of the error budget (avail = 100 − err). |

### Capture approach: client-side measurement

The tool (`scripts/observability/capture-slo.sh`) issues N concurrent curl
requests, records each request's `%{time_total}` and `%{http_code}`, and
computes:

- `latencyP99Ms` = p99 nearest-rank over the responded requests, ms.
- `errorRatePct` = `100 * (#5xx + #connection-failures) / total`.
- `availabilityPct` = `100 − errorRatePct`.

Client-side measurement is chosen over scraping a `/metrics` endpoint because
the `/api/health` latency surface is exactly what curl timing captures
end-to-end, without requiring server-side histogram exposition.

### HTTP availability vs body sub-status (honest nuance)

The core `/api/health` returns HTTP **200** with a JSON body whose `status`
field may read `degraded` when optional inference subservices (e.g. a
not-yet-warmed model) are not fully ready. The SLO measured here is the **HTTP
liveness contract** — latency + HTTP-status availability — for which a 200
response is "available". The `degraded` body field is a deeper subservice
summary orthogonal to the liveness HTTP contract; it is recorded honestly in
`## Discovered Issues` rather than silently conflated with the SLO. Probing the
endpoint 20× returned 200 every time, and the container healthcheck (which
greps for a 2xx on `/api/health`) reports `healthy`.

### Fail-loud, no fabrication

A preflight probe runs ONE request first. If it returns a non-HTTP code
(connection refused → curl code `000`), the tool exits 1 and writes NO file.
This is the anti-fabrication guarantee: the tool can only ever emit evidence
from a real responding endpoint. There is no `--skip`, `--force`, or `--fake`
flag.

### Single-Implementation Justification

This design introduces exactly **one** capture implementation
(`scripts/observability/capture-slo.sh`) for exactly **one** workflow
(`core.health`). Although the tool consumes the Bubbles observability
contract — which itself is fronted by swappable telemetry **adapters**
(`none`/`prometheus`) at the framework layer — this spec does NOT add, fork, or
abstract any adapter, provider, strategy, or variant. It is a single concrete
consumer of an existing framework capability, mirroring the QF spec 085
implementation byte-for-byte except for the repo-specific endpoint, header text,
and CLI hint. A second implementation is explicitly excluded; further workflows
reuse this same single tool by passing a different `--workflow` / `--url`. No
capability foundation is being established here.

## Components

| Component | Path | Role |
|-----------|------|------|
| SLO registry | `.github/bubbles-project.yaml` `traceContracts.observability.slos.core.health` | Target values |
| Workflow link | `.github/bubbles-project.yaml` `traceContracts.workflows.core.health.slo` | Arms collection |
| Capture tool | `scripts/observability/capture-slo.sh` | Emits evidence |
| Evidence file | `.specify/runtime/observability/core.health.slo.json` | G100 input (gitignored runtime output) |
| Scope arming token | `specs/090-observability-slo-dogfood/scopes.md` `observabilityWorkflow: core.health` | Flips G100 no-op → ENFORCED |

## Alternatives Considered

- **Scrape a `/metrics` surface**: rejected — `/api/health` is the latency
  under test, and curl timing captures the real client experience.
- **Synthetic/simulated samples**: rejected — defeats the entire dogfood; the
  point is genuine captured telemetry, enforced by a real gate.
- **A new workflow with no live endpoint**: rejected — `/api/health` already
  runs, so the dogfood needs zero new server code.

## Discovered Issues

| Date | Issue | Severity | Disposition | Reference |
|------|-------|----------|-------------|-----------|
| 2026-06-14 | Smackerel `posture: wired` carried no SLO registry, so G100 was a permanent no-op here | Low | Resolved by this spec (adds `core.health` registry + workflow link + captured evidence + scope arming token) | This design, Components table; `.github/bubbles-project.yaml` |
| 2026-06-14 | Core `/api/health` returns HTTP 200 with a body `status: degraded` (`services: null`) when optional inference subservices are not warmed; the deeper subservice readiness is not surfaced as a distinct HTTP code | Low | Recorded honestly; NOT remediated here. The HTTP liveness SLO (200 + latency) is genuine and within target; a future spec may split a deeper readiness signal. This dogfood deliberately measures only the HTTP liveness contract. | `internal/api/router.go` HealthHandler; this report § Test Evidence DoD-5 |
