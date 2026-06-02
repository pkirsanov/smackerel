# PKT-049-A — RESPONSE (accepted as known drift)

- **Date:** 2026-06-02
- **Authorized by:** workflow owner (rescope decision)
- **Disposition:** Accepted as known drift; ownership transferred to downstream spec 049.

## Decision

The Grafana dashboard panels and Prometheus alert-rule additions
requested in PKT-049-A (iterations p95, USD spend rate, refusal rate
by cause, fabricated-source single-stat, tool call rate by tool,
per-tool latency p95, budget-exhausted by scope, compaction-signal
rate) are **accepted as known drift** for spec 064 close-out.

Local Prometheus collectors and the per-turn redacted INFO log line
have already shipped in SCOPE-14 against
`internal/assistant/openknowledge/metrics/` and
`internal/assistant/openknowledge/agent/`. The `/metrics` scrape
endpoint already exposes the new series; no scrape-config change is
needed in spec 049.

The downstream dashboard + alert work is hereby transferred to
spec 049 ownership and will be picked up in a successor spec dedicated
to the assistant observability surface. This response closes the
request from the spec 064 side.

## Impact on spec 064

- Spec 064 SCOPE-14 transitions to `Done` with a known-drift
  annotation in its scope status.
- `state.json.transitionRequests[PKT-049-A].status` flips to `closed`.
- `state.json.reworkQueue[PKT-049-A-await].status` flips to `resolved`.

## Receiving-spec action

Spec 049 (or its successor) is the owner of:

1. Grafana panels enumerated in PKT-049-A.
2. Prometheus alert rules enumerated in PKT-049-A.
3. Dashboard layout decisions.

No code change is required in spec 064 to support that work — the
metrics series are already exposed.
