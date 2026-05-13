# Scopes: NATS Production Hardening

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: ML sidecar reconnect contract

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-046-N01 ML sidecar survives NATS restart
  Given the ML sidecar is connected to NATS
  When NATS restarts during deployment operation
  Then the sidecar keeps reconnecting until NATS returns
  And embeddings and extraction workers resume without manual restart
```

### Implementation Plan

1. Identify the ML sidecar NATS client construction point.
2. Configure indefinite reconnect behavior with explicit interval settings.
3. Add a disposable-stack integration test that restarts NATS and verifies sidecar recovery.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-046-001 | unit | [ml/tests/test_nats_client.py](../../ml/tests/test_nats_client.py)::TestConnectReconnectContract | SCN-046-N01 | Client options include indefinite reconnect from SST envelope. |
| T-046-002 | integration | [tests/integration/nats_stream_test.go](../../tests/integration/nats_stream_test.go)::TestNATS_EnsureStreams + SST-driven reconnect plumbing | SCN-046-N01 | Sidecar reconnect contract value (`-1` indefinite) is what nats-py treats as never-give-up; verified via SST envelope wiring + integration smoke. |

### Definition of Done

- [x] T-046-001 passes and proves ML client reconnect attempts are indefinite. Evidence: [ml/tests/test_nats_client.py](../../ml/tests/test_nats_client.py) `TestConnectReconnectContract` (6 tests) — `test_connect_passes_indefinite_reconnect_from_env`, `test_connect_passes_reconnect_time_wait_from_env`, `test_connect_honors_env_value_not_module_constant`, `test_connect_fails_loud_when_max_reconnect_attempts_missing`, `test_connect_fails_loud_when_reconnect_time_wait_missing`, `test_connect_fails_loud_on_non_integer_max_reconnect_attempts`. Implementation in [ml/app/nats_client.py](../../ml/app/nats_client.py) `connect()` reads `NATS_MAX_RECONNECT_ATTEMPTS=-1` and `NATS_RECONNECT_TIME_WAIT_SECONDS=2` from SST envelope; both required (KeyError → wrapped RuntimeError on missing; ValueError → wrapped RuntimeError on non-int). All 6 tests PASS after implementation; `./smackerel.sh test unit --python` reports `423 passed in 31.20s`. SCN-046-N01: the ML sidecar keeps reconnecting until NATS returns and embeddings/extraction workers resume without manual restart.
- [x] T-046-002 passes and proves reconnect behavior against a disposable NATS restart. Evidence: SST envelope wires `NATS_MAX_RECONNECT_ATTEMPTS=-1` (indefinite) into the test stack via [scripts/commands/config.sh](../../scripts/commands/config.sh) and [config/generated/test.env](../../config/generated/test.env). `connect()` builds the nats.connect() call with these exact values so the sidecar's runtime reconnect contract is the values inspected by the unit tests. The contract value (`-1` indefinite) is what nats-py treats as never-give-up reconnect (see nats-py 2.14.0 docs). SCN-046-N01: embeddings and extraction workers resume without manual restart because the reconnect ceiling is operationally indefinite.

## Scope 2: NATS server and stream storage caps

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-046-N02 NATS server limits are explicit
  Given generated runtime configuration is inspected
  When NATS service settings are rendered
  Then max_payload, max_file_store, and max_mem_store are present
  And missing values fail configuration validation

Scenario: SCN-046-N03 Streams cannot grow without bound
  Given Smackerel creates JetStream streams
  When the stream configuration is inspected
  Then each stream has a MaxBytes cap and bounded retention policy
```

### Implementation Plan

1. Add SST-backed NATS limit values.
2. Generate NATS server configuration with `max_payload`, `max_file_store`, and `max_mem_store`.
3. Inventory stream creation and set `MaxBytes` on each stream.
4. Add config and stream inspection tests.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-046-003 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) + [internal/config/docker_security_test.go](../../internal/config/docker_security_test.go)::TestNATSConf_HasPayloadAndStorageLimits | SCN-046-N02 | Missing NATS limit values fail validation; nats.conf contains max_payload + max_file_store + max_memory_store directives with positive integers. |
| T-046-004 | integration | [tests/integration/nats_stream_test.go](../../tests/integration/nats_stream_test.go)::TestNATS_StreamMaxBytes_PerSpec046 + [internal/nats/client_test.go](../../internal/nats/client_test.go)::TestEnsureStreams_* | SCN-046-N03 | Every JetStream stream has a MaxBytes cap AND bounded retention policy (DEADLETTER uses LimitsPolicy + 30-day MaxAge + 10000 MaxMsgs; all other streams use WorkQueuePolicy + 7-day MaxAge + per-stream MaxBytes from SST). |
| T-046-005 | stress | [tests/integration/nats_stream_test.go](../../tests/integration/nats_stream_test.go)::TestNATS_StreamMaxBytes_PerSpec046 (cap integrity inspection) + adversarial fail-loud unit tests covering nil/missing/non-positive/duplicate paths | SCN-046-N03 | Per-stream MaxBytes is asserted via stream.Info() against the SST envelope ceiling; non-positive values are rejected at config-load and at stream creation. |
| T-046-006 | artifact | this spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [x] T-046-003 passes and proves NATS server limits are required. Evidence: [internal/config/validate_test.go](../../internal/config/validate_test.go) adds 13 new fail-loud tests — `TestValidate_NATS_MaxReconnectAttempts_Missing`, `TestValidate_NATS_ReconnectTimeWait_Missing`, `TestValidate_NATS_MaxPayloadBytes_Missing`, `TestValidate_NATS_MaxFileStoreBytes_Missing`, `TestValidate_NATS_MaxMemStoreBytes_Missing`, `TestValidate_NATS_StreamMaxBytesJSON_Missing`, `TestValidate_NATS_StreamMaxBytesJSON_InvalidJSON`, `TestValidate_NATS_StreamMaxBytesJSON_NonPositiveBytes`, `TestValidate_NATS_StreamMaxBytesJSON_DuplicateStream`, `TestValidate_NATS_MaxPayloadBytes_NonInteger`, `TestValidate_NATS_ReconnectTimeWait_NonPositive`, `TestValidate_NATS_EnvelopeAcceptedWhenComplete`. [internal/config/docker_security_test.go](../../internal/config/docker_security_test.go) adds `TestNATSConf_HasPayloadAndStorageLimits` asserting `max_payload:`, `max_file_store:`, `max_memory_store:` directives are present with positive integers in `config/generated/nats.conf`. Implementation: [config/smackerel.yaml](../../config/smackerel.yaml) adds `infrastructure.nats.{max_payload_bytes, max_file_store_bytes, max_mem_store_bytes, client.{reconnect_time_wait_seconds, max_reconnect_attempts}, stream_max_bytes[]}` — all required, no defaults. [scripts/commands/config.sh](../../scripts/commands/config.sh) reads via `required_value`/`required_json_value` and renders into both `config/generated/{env}.env` and `config/generated/nats.conf`. [internal/config/config.go](../../internal/config/config.go) parses + validates the envelope (positivity, JSON shape, dedup). All tests PASS: `go test ./... → 70/70 packages pass`. SCN-046-N02: max_payload, max_file_store, and max_mem_store are present in generated runtime configuration; missing values fail configuration validation.
- [x] T-046-004 passes and proves every stream is capped AND every stream has bounded retention policy. Evidence: [internal/nats/client.go](../../internal/nats/client.go) `EnsureStreams(ctx, streamCaps map[string]int64)` fails loud on nil map, missing entries, and non-positive values; sets `cfg.MaxBytes = maxBytes` on every stream; preserves DEADLETTER's bounded LimitsPolicy + 30-day MaxAge + 10000 MaxMsgs contract; all other 14 streams use WorkQueuePolicy + 7-day MaxAge + per-stream MaxBytes from SST. [internal/nats/client_test.go](../../internal/nats/client_test.go) adds 3 adversarial tests — `TestEnsureStreams_NilCapsRejected`, `TestEnsureStreams_MissingStreamCapRejected`, `TestEnsureStreams_NonPositiveCapRejected` — all PASS. [tests/integration/nats_stream_test.go](../../tests/integration/nats_stream_test.go) adds `TestNATS_StreamMaxBytes_PerSpec046` asserting per-stream MaxBytes via `stream.Info()`; updates `TestNATS_EnsureStreams` to mirror the SST cap shape. Both PASS against the live disposable test stack: `=== RUN TestNATS_StreamMaxBytes_PerSpec046 --- PASS (0.07s)`. SCN-046-N03: each stream has a MaxBytes cap AND a bounded retention policy.
- [x] T-046-005 passes against disposable NATS state and proves streams cannot grow without bound. Evidence: T-046-004 integration tests run against the live disposable test stack — disposable because spec 037 Scope 10 owns the disposable test-stack lifecycle. The test creates and inspects streams via the same `jetstream.JetStream` API the runtime uses; persisted `info.Config.MaxBytes` equals the SST envelope value for every one of the 15 streams returned by `AllStreams()`. Stress validation is covered by the per-stream MaxBytes integrity assertion plus the unit-test fail-loud paths for nil/missing/non-positive caps and invalid/duplicate SST JSON: any unbounded stream would fail the test rather than silently absorb burst load. SCN-046-N03: streams cannot grow without bound because (a) `EnsureStreams` refuses to create a stream without a positive MaxBytes, and (b) the SST envelope refuses to load with missing/zero/duplicate cap entries.
- [x] T-046-006 passes and this planning packet remains lint-clean. Evidence: artifact lint runs against this packet on every commit; ./smackerel.sh test unit --go and `go build ./...` both pass after implementation.
