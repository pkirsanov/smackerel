---
description: How to author and wire telemetry adapters under bubbles/adapters/observability/ — uniform 4-verb contract, none default, prometheus reference, swappable per downstream repo. Also how to use the validate plane during live-category tests (integration/e2e/stress/load) to discover defects — error spans, latency outliers, fan-out/N+1 — and route findings to bug artifacts.
---

# bubbles-observability-adapter

Bubbles ships a uniform adapter contract for fetching live production telemetry (alerts, SLO burn, error rate, deploy impact). Mirrors the knb offsite-backup adapter pattern: declarative selection in `traceContracts.observability.endpoints`, swappable backends under `bubbles/adapters/observability/<name>.sh`, `none` as the safe default. Selection is split across two planes — `validate` (the ephemeral per-run test stack) and `operate` (prod, read-only).

> **Consumer status (read this first).** This adapter layer is WIRED to live runtime consumers. Operate-plane signals are fetched **read-only** by `bubbles.stabilize` (incident diagnosis — alerts/error-rate/deploy-impact fetched first), `bubbles.upkeep` (the weekly `slo-review` task — SLO burn + error rate), and `bubbles.train` (deploy-impact / SLO burn consulted before promote and as a rollback signal). `bubbles.devops` owns the wiring EXECUTION (authoring adapters, dashboards, alert rules); the three agents above are read-only CONSUMERS. The validate plane is exercised by instrumented feature scopes. (`bubbles.retro` is NOT a telemetry consumer — earlier drafts named it in error.)

## When to author a new adapter

- You run a telemetry stack (Sentry, Grafana Cloud, Datadog, New Relic, custom Prometheus federation) that the shipped `prometheus` adapter does not cover.
- You want Bubbles ops agents to read live signals from that stack. The operate-plane consumers (`bubbles.stabilize`, `bubbles.upkeep`, `bubbles.train`) invoke the `fetch-*` verbs at runtime through the plane resolver, so authoring the adapter immediately lights up incident diagnosis, the `slo-review` task, and promote/rollback gating for your stack.

## Contract (every adapter MUST implement all 4 verbs)

| Verb | Stdout | Exit 0 means | Exit 1 means |
|------|--------|--------------|--------------|
| `fetch-alerts` | JSON array of active alerts | Adapter reachable, no parse errors | Adapter unreachable / parse failure (NOT a framework failure) |
| `fetch-slo-burn` | JSON map service→burn-rate | (same) | (same) |
| `fetch-error-rate` | JSON map service→error-pct | (same) | (same) |
| `fetch-deploy-impact` | JSON map sha→regression-delta | (same) | (same) |

Unknown verbs MUST exit 1 with `unknown verb` on stderr. `-h` / `--help` MUST print a usage block and exit 0.

## File location

`bubbles/adapters/observability/<adapter-name>.sh` (chmod +x).

## Required structure

```bash
#!/usr/bin/env bash
set -euo pipefail
VERB="${1:-}"
# validate env / dependencies
case "$VERB" in
  fetch-alerts) ... ;;
  fetch-slo-burn) ... ;;
  fetch-error-rate) ... ;;
  fetch-deploy-impact) ... ;;
  -h|--help|"") usage; exit 0 ;;
  *) echo "[<name>][ERROR] unknown verb '$VERB'" >&2; exit 1 ;;
esac
```

The case statement is what `observability-adapter-lint.sh` greps for. If a verb is not listed verbatim, the lint fails.

## NO defaults for adapter inputs

Adapters that need env vars (URL, token, etc.) MUST refuse to run when the var is unset. NO `: "${VAR:=fallback}"`. Example: `prometheus.sh` requires `PROMETHEUS_BASE_URL` and exits 1 with an explicit error if it is missing.

This matches the framework-wide NO DEFAULTS policy. The operator (or knb adapter overlay) sets the env var explicitly per environment.

## Wiring

In your repo's `.github/bubbles-project.yaml`, under the `traceContracts.observability` block (start from `templates/observability.yaml.tmpl`):

```yaml
traceContracts:
  observability:
    endpoints:
      validate:                              # ephemeral per-run test stack
        sloBurn: { adapter: prometheus, profile: test }
        errorRate: { adapter: prometheus, profile: test }
        alerts: { adapter: none }
        deployImpact: { adapter: none }
      operate:                               # prod (read-only)
        alerts: { adapter: prometheus, profile: prod }
        sloBurn: { adapter: prometheus, profile: prod }
        errorRate: { adapter: prometheus, profile: prod }
        deployImpact: { adapter: none }      # mix-and-match is fine
```

A resolver (`bubbles/scripts/observability-endpoint-resolve.sh`) maps `(plane, signal) → { adapter, profile }`. Adapter values are provider **names** (never URLs/tokens); `profile` selects the env binding. The operate-plane consumers (`bubbles.stabilize`, `bubbles.upkeep`, `bubbles.train`) read this config through the resolver at runtime; the validate plane is read by instrumented feature scopes.

## Posture model

`traceContracts.observability.posture` is a tri-state decision every repo makes:

| Posture | Meaning |
|---------|---------|
| `wired` | Telemetry is wired; instrumented scopes must prove telemetry + SLOs. At least one non-`none` validate-plane signal is required (all-`none` is rejected as *fake-wired*). |
| `opted-out` | A legitimate, recorded, expiring choice. Requires a full `optOut` block. |
| *(absent)* | Undeclared — the only "nag" state. `policy.undeclaredPosture` decides warn vs block. |

## Two planes, one provider, a profile per plane

- **validate** → resolves to the ephemeral per-run test stack (`profile: test`). Feature scopes use the validate plane only; test telemetry stays `env=test*` (G115 protects prod).
- **operate** → resolves to prod (`profile: prod`), read-only, and only for deploy / train / upkeep / incident / release scopes.

Adapter names are provider names, never environment names — the `profile` selects env vars/endpoints, so there is no `prometheus-test` adapter file proliferation.

## Opt-out lifecycle

When `posture: opted-out`, the `optOut` block is REQUIRED and carries `reasonCode` (`no-runtime` | `pre-monitoring` | `external-monitoring-only`), `reason`, `declaredAt`, `revisitAfter`, and `approvedBy`. `revisitAfter` is the single source of truth for expiry — once an opt-out passes its `revisitAfter`, an opt-in reminder escalates. `reasonCode` only seeds the *default* `revisitAfter` that setup proposes. A `wired → opted-out` transition (monitoring decommissioned) is legitimate but never silent: it must set a fresh `decision` and a full `optOut`.

## Evidence-file convention

Captured telemetry lands under `.specify/runtime/observability/<workflow>.<signal>.{txt,json}` (trace/log evidence may stay line-oriented text; SLO evidence is normalized JSON). Two stores, non-duplicative:

- `.specify/runtime/tool-calls.jsonl` — provenance that the capture command ran (MCP `record_evidence`).
- `.specify/runtime/observability/<workflow>.<signal>.json` — the parsed metric artifact the SLO guard reads (an *output* of the captured run, never a second source of truth).

`.specify/runtime/` is gitignored.

## Acceptance heuristic — "3 AM reconstructibility"

For a wired, instrumented scope (a Test Plan row declaring `observabilityWorkflow`), the DoD item *"telemetry captured in integration/e2e"* (Gate G080) is satisfied only when the captured trace passes the **3 AM reconstructibility** question:

> *Could an on-call engineer, paged at 3 AM with nothing but this trace, reconstruct the full story of what the workflow did and where it broke — without reading the source?*

If the answer is no — spans are missing, attributes that name the actor / resource / outcome are absent, or the failure point is ambiguous — the instrumentation is insufficient and the DoD item stays `[ ]`, regardless of whether the SLO numbers happen to pass. This is the human acceptance bar behind G080; the `trace-contract-guard.sh` check is the mechanical floor beneath it, and the G100 SLO guard asserts the numeric target on top. See [`scope-workflow.md`](../../agents/bubbles_shared/scope-workflow.md) → "Observability DoD Injection (MUST-when-wired)".

## Trace-driven defect discovery (during live-category tests)

Capturing SLO numbers (G100) proves a workflow met its target; it does NOT surface the defects hiding inside a passing run. During every live-category test (`integration` / `e2e-api` / `e2e-ui` / `stress` / `load`) against a `wired` stack, actively MINE the validate-plane telemetry for defects, then file what you find. **A green test over a sick trace is an undiscovered bug, not a pass.**

### When to run

- Any `integration` / `e2e-api` / `e2e-ui` / `stress` / `load` run against a `posture: wired` stack (validate plane, `profile: test`, `env=test*`).
- NOT for `unit` / `ui-unit` (no real stack to trace), and NEVER against the operate/prod plane (read-only; off-limits to feature tests — G115).

### The four defect classes to scan for

Pull the exercised workflow's traces/metrics from the validate-plane stack and look for:

1. **Error / exception spans** — any span with `error=true`, a non-OK status, or a logged exception — *even when the top-level request returned success*. A swallowed or silently-retried error is a defect.
2. **Latency outliers** — spans that breach the workflow's `traceContracts.observability.slos` target, or that dominate the critical path when they should not (the slowest span is not the expected one).
3. **Fan-out / N+1** — the same downstream span (DB query, RPC, cache call) repeated per-item instead of batched; a span count that scales with input size.
4. **Retries / timeouts / missing spans** — silent retries, near-timeout spans, or a step that emits NO span (a blind spot that also fails the 3 AM bar above).

### What to do with a finding

- Treat it as a **defect, not noise**: file it through `bubbles.bug` (see `bubbles-bug-template`) with the captured trace as before-fix evidence. Do NOT defer it to "follow-up".
- Completion gating applies: under `bubbles-dod-validation` + `bubbles-anti-fabrication`, a scope cannot be marked done while a discovered error-span, SLO-breaching latency, or N+1 remains unfixed.
- Record the capture as evidence (`bubbles-evidence-capture`); it lands under `.specify/runtime/observability/<workflow>.<signal>.{txt,json}` (gitignored).

### Repo wiring this needs

The validate-plane stack must be reachable from the test (`prometheus` / `jaeger` / `tempo` on the ephemeral `profile: test` stack), plus a capture command the test or agent runs to deposit evidence — the repo's own observability-capture surface. Defect-mining reuses the same validate-plane access as SLO capture; it just queries the trace/span signals, not only the SLO aggregate.

### Works well with

- `bubbles-test-integrity` — the test-quality gates this method runs alongside (Step 8, live categories).
- `bubbles-evidence-capture` — the evidence shape for the captured trace/metric.
- `bubbles-bug-template` / `bubbles.bug` — where a discovered defect is filed.
- `bubbles-dod-validation` / `bubbles-anti-fabrication` — why an unfixed finding blocks done.

## Verification

```bash
bash bubbles/scripts/observability-adapter-lint.sh
# Output:
# [observability-adapter-lint] OK (2 adapter(s) validated)
```

If you add a third adapter (e.g. `sentry.sh`), the lint discovers it via the `*.sh` glob. No registry update needed.

## Reference adapters

| Adapter | Verbs | Env vars |
|---------|-------|----------|
| `none` | `fetch-alerts` returns `[]`; `fetch-slo-burn` / `fetch-error-rate` / `fetch-deploy-impact` return `{}` | — |
| `prometheus` | all 4 query Prometheus HTTP API (`fetch-alerts` normalized to a bare array) | `PROMETHEUS_BASE_URL`, `PROMETHEUS_CURL_MAX_TIME`, `PROMETHEUS_QUERY_SLO_BURN`, `PROMETHEUS_QUERY_ERROR_RATE`, `PROMETHEUS_QUERY_DEPLOY_IMPACT` (all required; no defaults), `PROMETHEUS_BEARER_TOKEN` (optional secret) |

> **Canonical per-verb shapes (shipped):** `fetch-alerts` → JSON **array** (`[]` when empty); `fetch-slo-burn` / `fetch-error-rate` / `fetch-deploy-impact` → JSON **map** (`{}` when empty). These shapes are defined in [`docs/guides/CONTROL_PLANE_SCHEMAS.md`](../../docs/guides/CONTROL_PLANE_SCHEMAS.md), enforced per-verb by `observability-adapter-lint.sh`, and already implemented in `none.sh` and `prometheus.sh` (the `fetch-alerts` array-vs-map split landed in IMP-001 SCOPE-3a). The earlier "all 4 return `{}`" behavior is superseded.

## Failure semantics

Adapter exit 1 is **not** a framework failure. It means "telemetry unavailable, proceed without enrichment". Consumers MUST gracefully degrade when an adapter returns non-zero or empty. The framework continues normally.

## Anti-patterns

| Don't | Why |
|-------|-----|
| Hardcode a default URL/token in the adapter | Violates NO DEFAULTS policy — operator sets env vars |
| Return non-JSON on success | Breaks consumer parsing |
| Exit 2+ on adapter unavailable | Reserved for framework-level errors |
| Skip the `-h` / `--help` block | Lint and human discovery both expect it |
| Edit `bubbles/scripts/*` to mention a specific adapter | Adapters discovered via filesystem glob; never hard-referenced |
