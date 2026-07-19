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

## Complexity Tracking

None - simplest viable fix used.
