# Recipe: Observe Production (Live Telemetry Adapters)

**When to use:** You want to wire your production telemetry stack (alerts, SLO burn, error rate, deploy impact) into the Bubbles observability adapter contract and declare your repo's observability posture.

> **Consumer status:** this adapter layer is WIRED to live runtime consumers. Operate-plane signals are fetched read-only by `bubbles.stabilize` (incident diagnosis), `bubbles.upkeep` (the weekly `slo-review` task), and `bubbles.train` (promote/rollback gating); `bubbles.devops` owns the wiring execution. Wiring an adapter lights those consumers up for your stack. (`bubbles.retro` is not a telemetry consumer.)

---

## The shipped adapters

| Adapter | What it does | Required env |
|---------|--------------|--------------|
| `none` | `fetch-alerts` → `[]`; the three maps (`fetch-slo-burn` / `fetch-error-rate` / `fetch-deploy-impact`) → `{}`. Safe default. | — |
| `prometheus` | Queries Prometheus HTTP API | `PROMETHEUS_BASE_URL`, `PROMETHEUS_CURL_MAX_TIME`, and verb-specific `PROMETHEUS_QUERY_*` values (required; no defaults) |

Live in `bubbles/adapters/observability/<name>.sh`. The shipped contract is 4 verbs: `fetch-alerts`, `fetch-slo-burn`, `fetch-error-rate`, `fetch-deploy-impact`.

---

## Posture decision (wire vs opt-out)

Before wiring endpoints, declare a posture. It is a required decision, not optional tooling:

- **`wired`** — you have monitoring and intend to prove telemetry/SLOs. At least one validate-plane signal must be a real provider (all-`none` is rejected as *fake-wired*).
- **`opted-out`** — a legitimate, recorded, **expiring** choice. Set the full `optOut` block (`reasonCode`, `reason`, `declaredAt`, `revisitAfter`, `approvedBy`). Once `revisitAfter` passes, an opt-in reminder escalates — opting out is never "forever and forgotten".
- **undeclared** (no `posture:`) — the only nag state; `policy.undeclaredPosture` decides warn vs block.

The fastest way to decide is `/bubbles.setup focus: observability`, which discovers your stack and PROPOSES a posture for you to approve. Start from [`templates/observability.yaml.tmpl`](../../templates/observability.yaml.tmpl).

## Wire it in your repo

Open your `bubbles-project.yaml` (or `.github/bubbles-project.yaml`) and add the `observability` block UNDER the existing `traceContracts:` key:

```yaml
traceContracts:
  observability:
    schemaVersion: 1
    posture: wired
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

Adapter values are provider **names** only. Set the adapter's real env (Prometheus needs `PROMETHEUS_BASE_URL`, no default) per plane via `BUBBLES_OBS_VALIDATE_*` / `BUBBLES_OBS_OPERATE_*` in your deployment shell or deploy-overlay — never commit URLs/tokens here.

> **Clean cutover:** this `observability.endpoints` block REPLACES the old `traceContracts.liveTelemetryEndpoints` flat map. The legacy key had no consumer, so it is removed outright — there is no deprecation cycle. Map each legacy signal to an explicit `operate.<signal>` entry.

---

## Test the adapter directly

```bash
PROMETHEUS_BASE_URL=http://localhost:9090 \
PROMETHEUS_CURL_MAX_TIME=10 \
PROMETHEUS_QUERY_SLO_BURN='slo:burn_rate' \
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
5. Reference your adapter under `traceContracts.observability.endpoints.{validate,operate}` (per signal, each with a `profile`).

---

## Graceful degradation

When an adapter is set to `none` or returns exit 1, a telemetry-consuming agent skips the live-telemetry enrichment without failing. Nothing in Bubbles requires telemetry — it's enrichment, not infrastructure. The operate-plane consumers (`bubbles.stabilize`, `bubbles.upkeep`, `bubbles.train`) degrade gracefully to log/code diagnosis when a signal resolves to `none` or the adapter exits 1.

---

## Quote

> *"Decent."* — Bubbles
