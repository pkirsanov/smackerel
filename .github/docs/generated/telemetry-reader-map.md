<!-- GENERATED:TELEMETRY_READER_MAP_START — run bash bubbles/scripts/generate-telemetry-reader-map.sh -->
# Telemetry Producer/Reader Map

GENERATED — do not edit by hand. Run `bash bubbles/scripts/generate-telemetry-reader-map.sh`.

Every telemetry store the framework declares or references, with the surface
that produces it and the surfaces that read it. A reported field whose store
has no producer is a measurement nobody takes; this table exists so that case
is visible instead of absent.

| Store | Producer | Readers | Value class | Declared by |
|---|---|---|---|---|
| `(derived) state.json executionHistory` | `state.json executionHistory` | — | derived | `derivedFromState` |
| `.specify/metrics/.gitignore` | `undeclared` | `bubbles/scripts/trust-metadata.sh` | direct | code only |
| `.specify/metrics/activity.jsonl` | — | — | unmeasured | `activityTracking` |
| `.specify/metrics/events.jsonl` | `undeclared` | `bubbles/scripts/generate-telemetry-reader-map-selftest.sh` | direct | `metrics` |
| `.specify/metrics/observations.jsonl` | `undeclared` | `bubbles/scripts/developer-profile.sh` | direct | code only |
| `.specify/runtime/.gitignore` | `undeclared` | `bubbles/scripts/runtime-lease-selftest.sh`<br>`bubbles/scripts/trust-metadata.sh` | direct | code only |
| `.specify/runtime/.locks` | `undeclared` | `bubbles/scripts/runtime-concurrency-selftest.sh` | direct | code only |
| `.specify/runtime/batch-promotion-override-ledger.jsonl` | `undeclared` | `bubbles/scripts/batch-promotion-lint.sh` | direct | code only |
| `.specify/runtime/code-search.tool` | `undeclared` | `bubbles/scripts/code-search.sh`<br>`bubbles/scripts/v5.2-selftest.sh` | direct | code only |
| `.specify/runtime/experience-recall` | `undeclared` | `bubbles/scripts/experience-recall-authority-selftest.sh`<br>`bubbles/scripts/experience-recall-cli-selftest.sh`<br>`bubbles/scripts/experience-recall-index-selftest.sh`<br>`bubbles/scripts/experience-recall-lifecycle-selftest.sh` | direct | code only |
| `.specify/runtime/framework-events.jsonl` | `undeclared` | `bubbles/scripts/bundle-cost-report-selftest.sh`<br>`bubbles/scripts/bundle-cost-report.sh`<br>`bubbles/scripts/experience-recall-cli-selftest.sh`<br>`bubbles/scripts/framework-health-evidence-lint-selftest.sh`<br>`bubbles/scripts/goal-fidelity-telemetry-selftest.sh`<br>`bubbles/scripts/goal-fidelity-telemetry.sh`<br>`bubbles/scripts/retro-framework-health-selftest.sh`<br>`bubbles/scripts/retro-framework-health.sh` | direct | code only |
| `.specify/runtime/gate-hits.jsonl` | `bubbles/scripts/gate-hit-log.sh` | `bubbles/scripts/cli.sh`<br>`bubbles/scripts/gate-hit-log-selftest.sh`<br>`bubbles/scripts/generate-telemetry-reader-map-selftest.sh` | direct | `gateTelemetry` |
| `.specify/runtime/observability` | `undeclared` | `bubbles/scripts/guards/tail-delegated-gates.sh`<br>`bubbles/scripts/observability-check.sh`<br>`bubbles/scripts/observability-slo-guard-selftest.sh`<br>`bubbles/scripts/observability-slo-guard.sh` | direct | code only |
| `.specify/runtime/resource-leases.json` | `undeclared` | `bubbles/scripts/closeout-report.sh`<br>`bubbles/scripts/design-experiment-guard.sh`<br>`bubbles/scripts/worktree-hygiene-guard-selftest.sh`<br>`bubbles/scripts/worktree-hygiene-report.sh`<br>`bubbles/scripts/worktree-reap.sh` | direct | code only |
| `.specify/runtime/tool-calls.jsonl` | `undeclared` | `bubbles/scripts/evidence-admission-hardening-selftest.sh`<br>`bubbles/scripts/evidence-tool-log-bridge-selftest.sh`<br>`bubbles/scripts/evidence-tool-log-bridge.sh`<br>`bubbles/scripts/observability-check.sh`<br>`bubbles/scripts/observability-slo-guard.sh`<br>`bubbles/scripts/session-cap-guard-selftest.sh`<br>`bubbles/scripts/session-cap-guard.sh`<br>`bubbles/scripts/state-transition-guard-selftest.sh`<br>`bubbles/scripts/state-transition-guard.sh`<br>`bubbles/scripts/tool-capture-shim-selftest.sh`<br>`bubbles/scripts/tool-capture-shim.sh`<br>`bubbles/scripts/tool-log.sh` | direct | code only |
| `.specify/runtime/workflow-runs.json` | `undeclared` | `bubbles/scripts/retro-framework-health-selftest.sh`<br>`bubbles/scripts/retro-framework-health.sh`<br>`bubbles/scripts/run-state-reaper-selftest.sh`<br>`bubbles/scripts/run-state-registry-selftest.sh` | direct | code only |

16 store(s): derived 1, direct 14, unmeasured 1.

`direct` — a script carries the store path and produces or reads it.
`derived` — the registry names a non-store source rather than a file.
`unmeasured` — declared but no surface references the path: nothing produces
or reads it, so any field reported from it is unbacked.
<!-- GENERATED:TELEMETRY_READER_MAP_END -->
