# PKT-022-A — RESPONSE (accepted as known drift)

- **Date:** 2026-06-02
- **Authorized by:** workflow owner (rescope decision)
- **Disposition:** Accepted as known drift; ownership transferred to downstream spec 022.

## Decision

The local web-provider `CircuitBreaker`, SST-bound thresholds
(`failure_threshold=5`, `open_window_seconds=60`,
`half_open_after_seconds=30`, all fail-loud per G028),
`TerminationToolUnavailable` mid-loop short-circuit,
`RefusalToolUnavailable` mapping, and the bounded-cardinality
`openknowledge_circuit_state{provider}` /
`openknowledge_circuit_trips_total{provider}` metrics have shipped
locally for SCOPE-16.

The three review questions raised in PKT-022-A are **accepted as known
drift** and transferred to spec 022 ownership:

1. v1 threshold alignment with the operational-resilience playbook
   (incl. `runtime.resilience.default_*` SST sub-block question).
2. Whether an openknowledge health-check endpoint contribution is
   required for operator dashboards.
3. Whether the budget-exhaustion refusal-with-capture handshake should
   be lifted into a cross-subsystem graceful-degradation pattern in
   spec 022.

These items are out-of-scope for spec 064 close-out and will be picked
up in a successor spec dedicated to operational resilience.

## Impact on spec 064

- Spec 064 SCOPE-16 transitions to `Done` with a known-drift
  annotation in its scope status.
- `state.json.transitionRequests[PKT-022-A].status` flips to `closed`.
- `state.json.reworkQueue[PKT-022-A-await].status` flips to `resolved`.

## Receiving-spec action

Spec 022 (or its successor) is the owner of the three playbook
questions listed above. The local circuit-breaker shipped in SCOPE-16
remains authoritative for spec 064 runtime resilience.
