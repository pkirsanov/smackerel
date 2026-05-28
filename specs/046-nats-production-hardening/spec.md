# Feature: NATS Production Hardening

**Status:** Done (certified per state.json)

## Status

In Progress - planning packet created

## Review Findings

- STB-003: Python ML-sidecar NATS client reconnect behavior is not hardened for long-lived deployment runtime.
- STB-005: NATS server payload and storage limits are not product-contract guarded.
- STB-006: Per-stream storage caps are not enforced in code or configuration.

## Outcome Contract

**Intent:** Make NATS connectivity and storage behavior safe for always-on deployment operation.

**Success Signal:** The ML sidecar reconnects indefinitely without silent disconnect, NATS server limits are explicit and generated from product configuration, and JetStream streams have bounded `MaxBytes`/retention settings with tests proving caps are applied.

**Hard Constraints:**

- NATS settings must flow through the SST configuration pipeline.
- Stream caps must be explicit; unbounded streams are not acceptable.
- Tests must use disposable runtime state for integration scenarios.

**Failure Condition:** Smackerel can run with finite ML reconnect attempts, unbounded NATS payload/storage settings, or unbounded JetStream streams.

## Requirements

- **FR-046-001:** ML sidecar NATS client MUST configure indefinite reconnect behavior, including `max_reconnect_attempts=-1` or the equivalent library setting.
- **FR-046-002:** NATS server configuration MUST expose `max_payload`, `max_file_store`, and `max_mem_store` limits from SST.
- **FR-046-003:** Every Smackerel-created stream MUST set a bounded `MaxBytes` value.
- **FR-046-004:** Integration tests MUST prove reconnect behavior and stream/storage cap application.
- **FR-046-005:** Documentation MUST explain operational impact of the NATS caps.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-046-N01 ML sidecar survives NATS restart
  Given the ML sidecar is connected to NATS
  When NATS restarts during deployment operation
  Then the sidecar keeps reconnecting until NATS returns
  And embeddings and extraction workers resume without manual restart

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

## Product Principle Alignment

This spec supports Principle 3, Knowledge Breathes, by making message lifecycle storage bounded rather than permanent by accident. It supports Principle 6 by preventing noisy manual recovery after transient NATS interruptions.

## Non-Goals

- Replacing NATS JetStream with another bus.
- Changing message schemas.
- Implementing target adapter host-level NATS operations.
