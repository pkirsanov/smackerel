# Recipe: Observe Production (Live Telemetry Adapters)

**When to use:** You want `bubbles.retro` and `bubbles.stabilize` to read live signals (alerts, SLO burn, error rate, deploy impact) from your production telemetry stack instead of operating blind.

---

## The shipped adapters

| Adapter | What it does | Required env |
|---------|--------------|--------------|
| `none` | Returns `{}` for every verb. Safe default. | — |
| `prometheus` | Queries Prometheus HTTP API | `PROMETHEUS_BASE_URL` (required) |

Live in `bubbles/adapters/observability/<name>.sh`. The shipped contract is 4 verbs: `fetch-alerts`, `fetch-slo-burn`, `fetch-error-rate`, `fetch-deploy-impact`.

---

## Wire it in your repo

Open your `bubbles-project.yaml` (or `.github/bubbles-project.yaml`) and add:

```yaml
traceContracts:
  liveTelemetryEndpoints:
    alerts: "prometheus"          # or "none" to disable
    slo-burn: "prometheus"
    error-rate: "prometheus"
    deploy-impact: "none"         # mix-and-match is fine
```

Set the adapter's env vars in your deployment shell or knb adapter overlay (Prometheus needs `PROMETHEUS_BASE_URL`, no default).

---

## Test the adapter directly

```bash
PROMETHEUS_BASE_URL=http://localhost:9090 \
  bash bubbles/adapters/observability/prometheus.sh fetch-alerts
```

Expected: JSON from `/api/v1/alerts`. Exit 0 on success; exit 1 if Prometheus is unreachable (which is informational, not a framework failure).

---

## Authoring a new adapter

See [skill: bubbles-observability-adapter](../../skills/bubbles-observability-adapter/SKILL.md).

Short version:
1. Create `bubbles/adapters/observability/<name>.sh`.
2. Implement all 4 verbs in a `case` statement.
3. Refuse to run when required env vars are missing (NO defaults).
4. Run `bash bubbles/scripts/observability-adapter-lint.sh` — should exit 0.
5. Reference your adapter in `traceContracts.liveTelemetryEndpoints`.

---

## Graceful degradation

When an adapter is set to `none` or returns exit 1, consumers (`bubbles.retro target: framework`, `bubbles.stabilize` incident-fastlane) skip the live-telemetry enrichment without failing. Nothing in Bubbles requires telemetry — it's enrichment, not infrastructure.

---

## Quote

> *"Decent."* — Bubbles
