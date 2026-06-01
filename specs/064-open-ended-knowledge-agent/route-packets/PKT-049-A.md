# Route Packet PKT-049-A — Open-Knowledge Subsystem Observability (Dashboards + Alerts)

| Field              | Value |
|--------------------|-------|
| **Packet ID**      | PKT-049-A |
| **Routed from**    | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-14 |
| **Routed to**      | `specs/049-monitoring-stack` (next dispatch via `bubbles.workflow`) |
| **Status**         | `pending` |
| **Date**           | 2026-05-31 |
| **Kind**           | `cross_spec_request` |
| **Blocks**         | spec 064 SCOPE-14 final close-out (the local Prometheus surface ships; dashboards + alerts are owned by 049) |
| **Does NOT block** | spec 064 SCOPE-15, SCOPE-16, SCOPE-17, SCOPE-18 |

---

## 1. Context

Spec 064 SCOPE-14 has landed the local observability surface for the
open-knowledge agent:

- New package `internal/assistant/openknowledge/metrics/` exposes nine
  Prometheus collectors with bounded-cardinality labels (G021) and no
  in-code defaults (G028). Registration is explicit
  (`(*Metrics).Register(prometheus.Registerer)`), invoked by
  `cmd/core/wiring_assistant_openknowledge.go` against
  `prometheus.DefaultRegisterer` so the existing `/metrics` scrape
  picks them up without additional wiring.
- The agent (`internal/assistant/openknowledge/agent/agent.go`) now
  records histogram observations and counter increments on every turn
  termination (success or refusal), and emits one redacted INFO log
  line per turn (`openknowledge.turn`) with the prompt SHA-256 hex,
  per-tool-call name+outcome trace, termination reason, tokens, USD,
  iterations, and source count.

The metrics are scraped already (no scrape-config change required —
they ride the runtime `/metrics` endpoint at `internal/api/router.go`
`r.Handle("/metrics", metrics.Handler())`). What spec 049 owns is the
**Grafana dashboard panels** and **Prometheus alert rules** that turn
these series into operator-visible signals.

## 2. New Metric Series (already live in the runtime)

| Name                                       | Type              | Labels             | Use |
|--------------------------------------------|-------------------|--------------------|-----|
| `openknowledge_tool_calls_total`           | counter           | `tool`, `outcome`  | Tool call rate by tool; success/error split |
| `openknowledge_iterations_per_query`       | histogram         | —                  | Agent-loop iteration distribution (`buckets: 1,2,3,5,8,13`) |
| `openknowledge_tokens_per_query`           | histogram         | —                  | LLM tokens per turn |
| `openknowledge_usd_cents_per_query`        | histogram         | —                  | USD spend per turn (cents) |
| `openknowledge_tool_latency_seconds`       | histogram         | `tool`             | Per-tool Execute latency |
| `openknowledge_budget_exhausted_total`     | counter           | `scope`            | Budget cap hits by scope ∈ `{iterations, tokens, usd, monthly, per_user_monthly}` |
| `openknowledge_fabricated_source_total`    | counter           | —                  | Security signal: planner cited an unverified source |
| `openknowledge_refusal_total`              | counter           | `cause`            | Refusal rate by `contracts.AllRefusalCauses` |
| `openknowledge_compaction_signaled_total`  | counter           | —                  | Turns that crossed the compaction threshold |

Label cardinality bounds:

- `tool` ∈ registered tools (currently `internal_retrieval`,
  `web_search`, `unit_convert`, `calculator`).
- `outcome` ∈ `{success, error}`.
- `scope` ∈ the five-value `AllBudgetScopes` enum.
- `cause` ∈ `contracts.AllRefusalCauses` (6 values).

Unknown label values are silently dropped at the increment site so an
LLM that returns garbage cannot inflate Prometheus cardinality
(adversarial test:
`TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021`).

## 3. Requested Grafana Dashboard Panels

Add a new dashboard row "Open-Knowledge Agent" (or fold into the
existing assistant dashboard) with the panels below. Panel titles are
suggestions; the operational intent is what matters.

1. **Iterations per query (p50 / p95 / p99)** —
   `histogram_quantile(0.95, sum(rate(openknowledge_iterations_per_query_bucket[5m])) by (le))`
2. **USD cents per query (p95) + cumulative rate** —
   `histogram_quantile(0.95, sum(rate(openknowledge_usd_cents_per_query_bucket[5m])) by (le))`
   and `sum(rate(openknowledge_usd_cents_per_query_sum[1h]))`
3. **Refusal rate by cause (stacked)** —
   `sum(rate(openknowledge_refusal_total[5m])) by (cause)`
4. **Fabricated-source rate (single-stat, red threshold > 0)** —
   `sum(rate(openknowledge_fabricated_source_total[15m]))`
   — *this is the most important panel; it is a security signal that the planner attempted to cite an unverified source.*
5. **Tool call rate by tool + outcome (stacked, error in red)** —
   `sum(rate(openknowledge_tool_calls_total[5m])) by (tool, outcome)`
6. **Per-tool latency p95** —
   `histogram_quantile(0.95, sum(rate(openknowledge_tool_latency_seconds_bucket[5m])) by (le, tool))`
7. **Budget exhausted by scope (stacked)** —
   `sum(rate(openknowledge_budget_exhausted_total[5m])) by (scope)`
8. **Compaction-signal rate** —
   `rate(openknowledge_compaction_signaled_total[5m])`

## 4. Requested Prometheus Alert Rules

| Alert                                    | Expression                                                                                       | For    | Severity | Rationale |
|------------------------------------------|--------------------------------------------------------------------------------------------------|--------|----------|-----------|
| `OpenKnowledgeFabricatedSource`          | `sum(rate(openknowledge_fabricated_source_total[15m])) > 0`                                      | 5m     | warn     | Any sustained fabricated-source rate is a security smell — the cite-back verifier is catching it, but the planner shouldn't be trying. |
| `OpenKnowledgeMonthlyBudgetHit`          | `sum(rate(openknowledge_budget_exhausted_total{scope="monthly"}[1h])) > 0`                       | 30m    | warn     | The shared monthly USD cap fired — operator should review whether usage is on plan. |
| `OpenKnowledgePerUserBudgetHit`          | `sum(rate(openknowledge_budget_exhausted_total{scope="per_user_monthly"}[1h])) > 0`              | 30m    | info     | Per-user monthly cap fired — possibly a single noisy user; not yet a system-wide issue. |
| `OpenKnowledgeToolErrorRateHigh`         | `sum(rate(openknowledge_tool_calls_total{outcome="error"}[10m])) by (tool) > 0.1`                | 15m    | warn     | Sustained tool-error rate (> 0.1/s) by any tool — provider outage or quota exhaustion. |
| `OpenKnowledgeRefusalRateSpike`          | `sum(rate(openknowledge_refusal_total[10m])) > 0.05`                                             | 30m    | info     | Sustained refusal rate (> 0.05/s) — agent is hitting caps or losing grounding for many users. |

Operators may want to relax the warn-level thresholds during initial
rollout; the cardinality of `cause` and `scope` is bounded so any
threshold tuning is safe.

## 5. What This Packet Does NOT Ask For

- No scrape-config change. The metrics ride the existing runtime
  `/metrics` endpoint.
- No new metric series. The nine series above are the complete
  SCOPE-14 surface; spec 049 should NOT add additional open-knowledge
  series of its own.
- No changes to spec 049 alerting infrastructure beyond the rules
  above.

## 6. Test Anchors

The metric surface is covered by
`internal/assistant/openknowledge/metrics/metrics_test.go`:

- `TestOpenKnowledgeMetrics_NamesPinned` — regression guard on the
  exact metric names. Renaming any series here is a
  cross-spec-coordinated change.
- `TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021` —
  the adversarial cardinality guard.
- `TestOpenKnowledgeMetrics_AllRefusalCausesAccepted` /
  `TestOpenKnowledgeMetrics_AllBudgetScopesAccepted` — closed-vocab
  coverage.

The redacted log surface is covered by
`internal/assistant/openknowledge/agent/agent_log_test.go`:

- `TestAgentTurnLog_RedactsSecrets` — adversarial leak test (API
  keys, full URLs, web snippet bodies, raw prompts MUST NOT appear
  in the log).
- `TestAgentTurnLog_EmittedOnRefusal` — log fires on refusal paths
  too.

## 7. Definition of Done (for spec 049)

- [ ] Dashboard panels 1–8 above land in the spec 049 Grafana JSON
  bundle (or equivalent).
- [ ] Alert rules in §4 land in the spec 049 Prometheus rules.
- [ ] Spec 049 `report.md` references this packet ID and links to the
  series names + queries above.
- [ ] Response packet `PKT-049-A-RESPONSE.md` lands here on
  acceptance/rejection so spec 064 can close SCOPE-14.
