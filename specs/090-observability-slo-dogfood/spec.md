# Spec 090 — Observability SLO Dogfood (core /api/health)

| Field | Value |
|-------|-------|
| Spec ID | 090-observability-slo-dogfood |
| Status | in_progress |
| Workflow mode | full-delivery |
| Release train | mvp |
| Created | 2026-06-14 |
| Closes | Bubbles IMP-001 SCOPE-9 / T9.4 (multi-repo observability propagation) |

## Summary

Smackerel declares observability posture `wired` in
`.github/bubbles-project.yaml`, but until this spec the project carried **no
`traceContracts.workflows` SLO registry** and **no captured
`.specify/runtime/observability/*.slo.json` evidence** — so the Bubbles G100
gate (`observability_slo_evidence_gate`) was a permanent no-op here.

This spec is the **second** downstream dogfood of the IMP-001 observability
pattern (the first was QuantitativeFinance spec 085). It graduates one real,
already-running workflow — the Go core `/api/health` liveness endpoint — into a
gated SLO contract, captures **genuine** telemetry under load, and proves the
Bubbles G100 gate enforces the captured evidence against the contract target.

It is the downstream-propagation proof for the framework observability work
(bubbles `improvements/IMP-001-observability-first-class.md`, SCOPE-9 / T9.4): a
**second** wired downstream repo that carries a real `slo:` link backed by
captured runtime evidence that a blocking gate asserts — demonstrating the
pattern generalises beyond the first repo.

## Release Train

This spec targets the **mvp** train (the current active `self-hosted` slot). It
introduces **no feature flags** (`flagsIntroduced: []`): the SLO registry entry
and capture tool are unconditional infrastructure, not flag-gated behavior, so
there is no default-off obligation on the `next` train. The work instruments the
core liveness endpoint that already runs on the mvp train today.

## Problem

| Symptom | Evidence |
|---------|----------|
| Posture `wired` but no SLO registry | `traceContracts.workflows` was absent in `.github/bubbles-project.yaml` |
| No captured runtime SLO evidence | `.specify/runtime/observability/` had no `*.slo.json` files |
| G100 was a permanent no-op for Smackerel | `observability-slo-guard.sh --repo-root .` reported "wired, but no instrumented scope declares an observabilityWorkflow with an slo: link; G100 no-op" |

## Goals

1. Add a real SLO target registry entry and a `workflow -> slo` link to
   `.github/bubbles-project.yaml` for the `core.health` workflow.
2. Provide a **real**, fail-loud capture tool that measures live per-request
   latency + status client-side and emits the G100 evidence file. No synthetic
   fallback; refuse to emit fabricated "healthy" evidence when the core is down.
3. Declare `observabilityWorkflow: core.health` in a scope Test Plan so G100
   transitions from no-op to ENFORCED for Smackerel.
4. Capture genuine telemetry against the live Go core and prove the captured
   evidence MEETS the contract target (G100 green).

## Non-Goals

- Instrumenting every Smackerel workflow. This is the first Smackerel dogfood
  slice; further workflows graduate incrementally via the same mechanism.
- Adding server-side prometheus exposition for the core. The dogfood measures
  latency client-side (curl), so it does not depend on a `/metrics` endpoint.
- Any change to the deploy adapter, knb overlay, or operate-plane wiring.

### Single-Capability Justification

This spec exercises exactly **one** observability capability — captured SLO
evidence asserted by the Bubbles G100 gate — for exactly **one** workflow
(`core.health`). The Smackerel observability contract is fronted by swappable
telemetry **adapters** (`none` / `prometheus`) at the framework layer, but this
spec does NOT introduce, fork, or generalise any adapter, provider, strategy, or
variant. It is a single concrete consumer of the existing framework capability.
No capability foundation, plugin surface, or second provider is being
established. Additional workflows reuse the same single capability by adding
another `slos:` entry — never a new abstraction.

## Functional Requirements

- **FR-090-1** The project config MUST declare
  `traceContracts.observability.slos.core.health` with `latencyP99Ms`,
  `errorRatePct`, and `availabilityPct` targets, and
  `traceContracts.workflows.core.health.slo: core.health`.
- **FR-090-2** A capture tool MUST measure live HTTP per-request latency and
  status and emit `.specify/runtime/observability/core.health.slo.json` in the
  G100 evidence schema (`workflow, slo, sampleWindow, source, target,
  observed{latencyP99Ms, errorRatePct, availabilityPct}`).
- **FR-090-3** The capture tool MUST fail loud (non-zero, no file that asserts
  health) when the target endpoint is unreachable — no fabricated evidence
  path, no `--skip/--force/--fake` bypass.
- **FR-090-4** A scope Test Plan MUST declare `observabilityWorkflow:
  core.health` so the Bubbles G100 gate enforces the captured evidence.
- **FR-090-5** The captured evidence MUST be produced by a real load run and
  MUST MEET the contract target (G100 exit 0).

## Acceptance Criteria

- `observability-slo-guard.sh --repo-root .` reports the `core.health` workflow
  ENFORCED and within target (G100 OK, exit 0) with the captured evidence
  present.
- The capture tool, run against a down endpoint, exits non-zero and emits no
  within-target evidence.
- Smackerel's config validity and PII gates remain green (this spec adds no
  hidden defaults and no PII).
