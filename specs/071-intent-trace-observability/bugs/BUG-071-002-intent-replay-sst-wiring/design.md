# Bug Fix Design: BUG-071-002

## Root Cause Analysis

### Investigation Summary

`config/smackerel.yaml` declares all five intent-trace keys and `internal/config/assistant_intent_trace.go` validates them. Search of `scripts/commands/config.sh` finds no parse or output block, and search of aggregate config loading finds no call to `loadIntentTraceConfig`. The generated `test.env` therefore has no replay key, while the replay CLI reads the zero-value boolean.

### Root Cause

The intent-trace capability has two missing integration edges: SST compiler emission and aggregate loader invocation.

### Impact Analysis

- Affected components: config compiler, assistant config loader, replay CLI
- Affected tests: two replay E2E scenarios
- Affected data: read-only replay cannot start; no rows are changed

## Fix Design

### Solution Approach

Mirror adjacent assistant config blocks in `config.sh`: read all five keys with required-value helpers and emit all five into generated env. Invoke `loadIntentTraceConfig` from aggregate assistant loading and fold its errors into the existing fail-loud result. Add integration contract coverage that deletes each edge independently and proves the aggregate path catches it.

### Alternative Approaches Considered

1. Set replay true directly in the test process - rejected because it bypasses SST and leaves production config ignored.
2. Default `ReplayEnabled` to true in Go - rejected because missing config would become silent behavior.
3. Remove the replay gate from the CLI - rejected because the capability gate is an intentional operator control.

### Single-Implementation Justification

- **Existing owning abstraction:** The repository SST pipeline owns this configuration from `config/smackerel.yaml`, through required-value emission in `scripts/commands/config.sh`, into aggregate `config.Load()` and `loadAssistantConfig`. `internal/config.loadIntentTraceConfig` validates the typed `Config.Assistant.IntentTrace` block fail-loud.
- **Concrete implementations:** The five concrete values are sampling ratio, retention days, export targets, replay enabled, and retention sweep interval. The `assistant replay-intent` command, intent recorder/exporter, and retention behavior all consume that same typed configuration; there is no alternate source or replay engine.
- **Current consumers:** Core startup validation, persisted intent tracing, export fan-out, retention sweeping, the replay CLI, and replay integration/E2E scenarios depend on the complete generated environment and aggregate loader invocation.
- **Bounded variation axes:** Operator values vary by SST target, and `export_targets` varies within the existing closed vocabulary of `structured_log`, `otel`, and `prometheus`. Replay enablement is an explicit boolean gate, not provider selection.
- **Extension path:** Another intent-trace setting must be added to the canonical YAML, required generator emission, typed config structure, aggregate loader validation, and its concrete consumer in one coherent path. Alternate config sources or fallback loaders are not extension points.
- **Foundation decision:** `IntentTraceConfig` and the aggregate assistant loader already form the reusable configuration boundary. This bug restores two omitted wiring edges; a provider registry or second configuration framework would bypass SST and add no supported implementation.

## Complexity Tracking

None - simplest viable fix used.
