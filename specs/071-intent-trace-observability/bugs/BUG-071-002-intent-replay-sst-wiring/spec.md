# Specification: BUG-071-002 Intent replay SST wiring

## Expected Behavior

The five required `assistant.intent_trace` values MUST flow from `config/smackerel.yaml` through `scripts/commands/config.sh` into generated env and through aggregate `config.Load()` into `Config.Assistant.IntentTrace`. Replay-dependent tests MUST consume that explicit test-target capability. Missing, empty, malformed, or unknown values MUST fail loudly.

## Acceptance Criteria

1. The generator reads and emits sampling ratio, retention days, export targets, replay enabled, and retention sweep interval.
2. Aggregate assistant config loading invokes the existing intent-trace loader and returns its aggregate errors.
3. The test target explicitly carries `ASSISTANT_INTENT_TRACE_REPLAY_ENABLED=true` from SST.
4. Both replay E2E scenarios pass against the real disposable Postgres store.
5. Removing any required key breaks a unit/config contract and the replay scenario cannot silently succeed.
6. Production values remain operator-defined by SST; no test-only fallback or language default is introduced.

### Single-Capability Justification

- **Classification:** This is missing wiring inside the existing intent-trace capability, not a new configuration capability, replay provider, or runtime variant.
- **Existing foundation and reuse path:** The five `assistant.intent_trace` values originate in `config/smackerel.yaml`, flow through `scripts/commands/config.sh` into generated `ASSISTANT_INTENT_TRACE_*` values, and are loaded by `internal/config.loadAssistantConfig` through `loadIntentTraceConfig` into `Config.Assistant.IntentTrace`. Replay then consumes the same typed configuration and persisted intent-trace store.
- **Consumer set:** Core startup validation, intent recording and retention behavior, the `assistant replay-intent` command, and the replay integration/E2E scenarios all consume this one typed intent-trace configuration.
- **Why no new abstraction or provider registry is needed:** The typed `IntentTraceConfig` and aggregate assistant loader are already the single extension point. The repair connects two omitted edges in that path and adds no alternate source, store, replay engine, or provider that would justify a registry.

## Release Train

This bug targets the `mvp` train and introduces no feature flag.

## Test Isolation

Replay E2E uses the disposable `smackerel-test` Postgres database. Test-created trace rows use unique prefixes and the CLI tears down the backing store.

## Deployment Boundary

This packet changes generic product config compilation and runtime loading only. It does not touch deployment adapters, manifests, hosts, release-train bundles, or secrets.
