# Design: 071 IntentTrace Observability Surface

Owner: `bubbles.design`
Workflow mode: `product-to-planning`
Status ceiling for this pass: `specs_hardened`
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Specs 067 and 068 require an `IntentTrace`, but the runtime only has the broader assistant structured log, OTel span wrapper, `agent_traces` rows for executor runs, and assistant audit seams. Existing tracing in `internal/assistant/tracing/` stamps canonical assistant attributes, while `agent_traces` in migration 020 only exists after executor invocation and cannot represent clarify, refusal, sampled-out, or capture-only turns.

**Target State.** Every compiled turn writes one redacted `IntentTrace` or one sampled-out envelope, exports bounded metrics and OTel attributes, persists replayable redacted payload in Postgres, and feeds the spec 049 monitoring stack. Operators can inspect the Assistant Intents dashboard, enforce retention, and run `./smackerel.sh assistant replay-intent <trace_id>` without mutating conversation state or invoking side effects.

**Patterns to Follow.** Extend `internal/assistant/tracing/` rather than adding an unrelated tracing stack. Keep bodies and sensitive slots redacted like `Facade.Handle` logs. Reuse `agent_traces.trace_id` only as an optional join target, because many assistant decisions never reach the executor. Use fail-loud config loading like `internal/config/assistant.go` and dashboard inventory patterns from specs 030 and 049.

**Patterns to Avoid.** Do not depend on Loki/Grafana logs as the only replay store. Do not log raw text or slot values when source policy forbids persistence. Do not branch replay through side-effect tools. Do not make sampling silently drop total-turn accounting. Do not introduce config fallbacks for sampling, retention, or export targets.

**Resolved Decisions.** `IntentTrace` is persisted in a new Postgres table, emitted as one structured log family, and projected into OTel attributes. `slots_redaction_summary` is a typed map. Sampled-out turns store only the minimal envelope. Replay loads the Postgres trace row and runs compiler/router dry-run comparison. `user_id` is never exported raw; dashboards use `user_id_hash`.

**Open Questions.** The exact Grafana JSON is provisioned by the monitoring/deploy owner, but this design defines the required panels, metrics, and query fields. The operator must explicitly choose SST values for sampling ratio, retention days, and export targets; no runtime value is implied.

## Overview

This design makes `IntentTrace` the authoritative assistant turn observability contract introduced by spec 068 and consumed by spec 067, spec 049, and spec 074. The trace is not user-facing product content. It is an operator, privacy, policy, and replay surface.

Each compiled turn produces exactly one of:

| Record | When | Payload |
|--------|------|---------|
| `IntentTrace` | Turn selected for full trace capture | Redacted v1 payload with compiler, route, tool-call, status, refusal, and capture fields |
| `IntentTraceSampledOut` | Turn excluded by configured sampling | Minimal envelope with counters and no raw text or slot values |

Both records increment total-turn accounting so sampling cannot under-report usage.

## Architecture

```text
AssistantMessage
  -> spec 068 intent compiler
  -> trace builder applies source-policy redaction
  -> sampling decision
     -> full IntentTrace: persist + structured log + OTel attrs + metrics
     -> sampled-out envelope: persist minimal row + structured log + metrics
  -> facade routing / policy guard / capture-as-fallback
  -> optional agent_traces / artifact joins
```

Components:

| Component | Package / Surface | Responsibility |
|-----------|-------------------|----------------|
| Trace builder | `internal/assistant/intenttrace` | Build v1 payload, enforce redaction, validate schema before export |
| Trace store | `internal/assistant/intenttrace/postgres` | Persist replayable rows and enforce TTL sweep |
| Trace exporter | `internal/assistant/intenttrace/export` | Emit structured logs, OTel attributes, and Prometheus metrics |
| Replay command | `cmd/core` via `./smackerel.sh assistant replay-intent <trace_id>` | Load one row, dry-run compiler/router, compare route/tool calls |
| Dashboard inventory | spec 049 monitoring stack | Assistant Intents panel set and alerts |

The trace store is the replay source of truth. Logs and OTel are export views of that same validated payload.

## Capability Foundation

The reusable capability is `IntentTraceObservability`: a versioned trace contract with shared redaction, persistence, export, replay, retention, and dashboard policies.

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| `IntentTraceRecorder` | Accept one compiler turn and write exactly one trace/envelope | spec 068 compiler, spec 061 facade |
| `IntentTraceRedactor` | Convert raw compiler fields to source-policy-safe payload | recorder, replay, dashboard exports |
| `IntentTraceStore` | Persist, fetch by trace id, sweep by retention TTL | replay command, dashboard queries, retention job |
| `IntentTraceExporter` | Emit metrics, structured log, OTel attrs from validated payload | spec 030/049 observability stack |
| `IntentTraceReplay` | Dry-run compiler/router and compare decisions | CLI/devtools, scenario authors, privacy review |

Foundation-owned behavior:

- Exactly one trace or sampled-out envelope per compiled turn.
- Schema validation before persistence and export.
- Redaction before any payload leaves process memory.
- Retention sweeps driven by required SST keys.
- Replay is read-only and side-effect-blocked.

### Variation Axes

| Axis | Values | Foundation-Owned? |
|------|--------|-------------------|
| Export target | structured log, OTel, Prometheus/dashboard | Yes |
| Payload class | full trace, sampled-out envelope | Yes |
| Replay input | sampled full trace, trace not found, expired trace | Yes |
| Source policy | raw allowed, raw disallowed, slot-level sensitive | Yes |
| Consumer | dashboard, spec 067 guard, replay CLI, privacy review | Yes |

## Concrete Implementations

### IntentTrace Recorder

Package: `internal/assistant/intenttrace`.

The recorder is called once after spec 068 validates a compiled intent and before the facade executes side-effect-bearing work. The recorder receives compiler output, source policy, route result, tool-call summary, refusal/capture metadata, and response status through an append-only update object. It refuses to persist any payload that fails the v1 schema or redaction checks.

### Postgres Replay Store

Package: `internal/assistant/intenttrace/postgres`.

Postgres is chosen over Loki as the replay backend because replay needs deterministic lookup by trace id, retention enforcement, and structured fields even when the executor never wrote an `agent_traces` row.

### Export Adapters

Package: `internal/assistant/intenttrace/export`.

Exports are derived from the persisted payload. OTel spans carry bounded attributes only; the structured log carries redacted JSON; metrics use closed-vocabulary labels.

## Data Model

### `assistant_intent_traces`

```sql
CREATE TABLE IF NOT EXISTS assistant_intent_traces (
    trace_id                  TEXT PRIMARY KEY,
    schema_version            TEXT NOT NULL CHECK (schema_version IN ('v1')),
    turn_id                   TEXT NOT NULL,
    user_id_hash              TEXT NOT NULL,
    transport                 TEXT NOT NULL CHECK (transport IN ('telegram', 'whatsapp', 'web', 'mobile')),
    transport_message_id      TEXT NOT NULL,
    sampled                   BOOLEAN NOT NULL,
    sampled_out_reason        TEXT,
    action_class              TEXT NOT NULL,
    side_effect_class         TEXT NOT NULL,
    confidence                DOUBLE PRECISION,
    route_decision            TEXT,
    tool_calls                JSONB NOT NULL,
    final_response_status     TEXT NOT NULL,
    compiler_invoked          BOOLEAN NOT NULL,
    model_route               TEXT,
    seed                      TEXT,
    refusal_cause             TEXT,
    capture_cause             TEXT,
    idea_artifact_id          TEXT,
    agent_trace_id            TEXT REFERENCES agent_traces(trace_id) ON DELETE SET NULL,
    slots_redaction_summary   JSONB NOT NULL,
    redacted_payload          JSONB NOT NULL,
    emitted_at                TIMESTAMPTZ NOT NULL,
    expires_at                TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_intent_traces_turn
    ON assistant_intent_traces (turn_id);

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_dashboard
    ON assistant_intent_traces (emitted_at DESC, action_class, final_response_status);

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_refusal
    ON assistant_intent_traces (refusal_cause, emitted_at DESC)
    WHERE refusal_cause IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_expiry
    ON assistant_intent_traces (expires_at);
```

`redacted_payload` is the full replay payload. Full traces and sampled-out envelopes share the table so total-turn counts cannot diverge.

### `IntentTrace` v1 Payload

```json
{
  "schema_version": "v1",
  "trace_id": "01H...",
  "turn_id": "assistant-turn-id",
  "user_id_hash": "16-hex-prefix",
  "transport": "web",
  "transport_message_id": "client-id",
  "compiler_invoked": true,
  "action_class": "weather.lookup",
  "side_effect_class": "read",
  "confidence": 0.91,
  "route_decision": "scenarios/weather",
  "tool_calls": [{"name":"weather.lookup","arguments_redacted":true,"outcome":"ok"}],
  "final_response_status": "checking_weather",
  "refusal_cause": null,
  "capture_cause": null,
  "idea_artifact_id": null,
  "model_route": "intent-compiler-v1",
  "seed": "sst-or-request-seed",
  "slots_redaction_summary": {
    "raw_text": "absent",
    "slot_classes": {"location":"safe", "account":"redacted"},
    "redacted_count": 1
  }
}
```

Sampled-out payloads contain only `schema_version`, `trace_id`, `turn_id`, `user_id_hash` when allowed by policy, `transport`, `transport_message_id`, `action_class`, `side_effect_class`, `sampled=false`, and `sampled_out_reason`.

## API/Contracts

### CLI Contract

Command: `./smackerel.sh assistant replay-intent <trace_id>`.

Inputs:

| Argument | Validation |
|----------|------------|
| `trace_id` | Required; exact lookup in `assistant_intent_traces` |

Success output shape:

```json
{
  "trace_id": "01H...",
  "schema_version": "v1",
  "read_only": true,
  "original": {"route_decision":"scenarios/weather", "tool_calls":["weather.lookup"]},
  "dry_run": {"route_decision":"scenarios/weather", "tool_calls":["weather.lookup"]},
  "match": {"route_decision": true, "tool_calls": true},
  "side_effects_invoked": false
}
```

Error outputs:

| Condition | Exit | Code |
|-----------|------|------|
| Trace not found or expired | 2 | `intent_trace_not_found` |
| Trace is sampled-out only | 2 | `intent_trace_sampled_out` |
| Replay would require a side effect | 3 | `side_effects_blocked` |
| Schema validation fails | 4 | `intent_trace_schema_invalid` |

### OTel Attribute Contract

Every assistant span participating in a compiled turn carries bounded attributes:

| Attribute | Source |
|-----------|--------|
| `assistant.intent_trace_id` | recorder trace id |
| `assistant.intent.schema_version` | `v1` |
| `assistant.intent.action_class` | compiled intent |
| `assistant.intent.side_effect_class` | compiled intent |
| `assistant.intent.route_decision` | route result or empty string |
| `assistant.intent.refusal_cause` | closed vocabulary or empty string |
| `assistant.intent.capture_cause` | closed vocabulary or empty string |

Raw text, slot values, bearer tokens, and user ids are never OTel attributes.

## UI/UX

### Assistant Intents Dashboard

Surface: Grafana dashboard inventory owned by spec 049.

Required panels:

| Panel | Data |
|-------|------|
| Total turns | full trace plus sampled-out count |
| Top action classes | `action_class` over selected range |
| Clarification rate | `action_class=clarify` plus response status |
| Refusal causes | `refusal_cause` joined to spec 064 counter `cause` |
| Compiler errors | status/error metrics from recorder |
| Capture-as-fallback rate | `capture_cause` and `idea_artifact_id` population |
| Recent trace samples | trace id, action class, route, redaction state, status |

Dashboard errors are fail-loud. If an export target is unavailable, the panel names the unavailable source rather than rendering zeroes.

### Replay Result

The replay result is primarily CLI output with the same content model available to a future devtools panel. It shows original vs dry-run route/tool calls, redaction state, and an explicit `side_effects_invoked: false` field before detailed rows.

## Security/Compliance

- Redaction occurs before persistence, structured logging, metrics, and OTel export.
- `user_id_hash` uses the existing assistant tracing hash helper; no raw user id is exported.
- Slot summaries are typed counts/classes, not string sketches that could leak values.
- Replay dry-run cannot write `assistant_conversations`, artifacts, tool side effects, or external calls.
- `agent_trace_id` and `idea_artifact_id` are references only; the trace store does not copy artifact content.
- Retention sweep deletes expired trace rows from Postgres and emits a structured log with counts only.

## Configuration And Migrations

Required SST keys:

| Key | Validation |
|-----|------------|
| `assistant.intent_trace.sampling_ratio` | float, `0.0 <= value <= 1.0`, explicitly set |
| `assistant.intent_trace.retention_days` | integer, `>= 1`, explicitly set |
| `assistant.intent_trace.export_targets` | non-empty list of `structured_log`, `otel`, `prometheus` |
| `assistant.intent_trace.replay_enabled` | strict bool |
| `assistant.intent_trace.retention_sweep_interval` | positive Go duration |

Missing or malformed keys abort startup with an error that names the key. Sampling and retention never use hidden runtime values.

Migration adds `assistant_intent_traces` and indexes above. Down migration drops the table and indexes only when no dependent replay/reporting surface is active.

## Observability

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_intent_trace_total` | `record_type,action_class,side_effect_class` | full vs sampled-out trace count |
| `smackerel_assistant_intent_trace_export_total` | `target,outcome` | export success/failure |
| `smackerel_assistant_intent_trace_redaction_total` | `policy,outcome` | redaction decisions |
| `smackerel_assistant_intent_replay_total` | `outcome` | replay command results |
| `smackerel_assistant_intent_trace_retention_sweep_total` | `outcome` | TTL sweep results |

Structured logs use event names `assistant_intent_trace`, `assistant_intent_trace_sampled_out`, `assistant_intent_replay`, and `assistant_intent_trace_retention_sweep`. Bodies and slots remain redacted.

Failure handling:

- Schema-invalid trace: fail the turn before side effects and emit `intent_trace_schema_invalid`.
- Store unavailable: fail loud if replay/persistence is required by SST; do not silently log-only.
- Export target unavailable: persist row, increment export failure metric, and surface dashboard health.

## Testing Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-071-A01 | integration | `tests/integration/assistant/intent_trace_test.go` | one full trace per compiled turn |
| SCN-071-A02 | unit + integration | `internal/assistant/intenttrace/sampling_test.go` | sampled-out envelope still counts |
| SCN-071-A03 | unit | `internal/assistant/intenttrace/redaction_test.go` | sensitive slots absent, typed summary present |
| SCN-071-A04 | e2e-api | `tests/e2e/assistant/intent_replay_test.go` | replay route/tool calls match and no side effects occur |
| SCN-071-A05 | unit | `internal/config/assistant_intent_trace_test.go` | missing SST key fails startup |
| SCN-071-A06 | integration | `tests/integration/monitoring/assistant_intents_dashboard_test.go` | dashboard query fields are emitted metrics |
| SCN-071-A07 | integration | `tests/integration/assistant/refusal_trace_join_test.go` | refusal counter cause equals trace cause |
| SCN-071-A08 | integration | `tests/integration/policy/intent_bypass_guard_test.go` | bypass guard reads trace ancestors |
| SCN-071-A09 | integration | `tests/integration/assistant/intent_trace_retention_test.go` | expired rows swept and logged |
| SCN-071-A10 | unit | `internal/assistant/intenttrace/golden_contract_test.go` | schema field change requires version bump |

## Risks & Open Questions

| Risk | Mitigation |
|------|------------|
| Trace table grows too quickly | Required sampling ratio, retention TTL, expiry index, sweep metric |
| Redaction bug leaks sensitive slots | Central redactor, golden adversarial fixtures, no per-call redaction |
| Replay drifts from live route | Replay uses the same compiler/router dry-run with side effects blocked |
| Dashboard joins overfit labels | Refusal/capture cause vocabularies are shared with specs 064 and 074 |
| Operator expects raw text during review | Design intentionally exposes redaction summaries, not raw disallowed values |