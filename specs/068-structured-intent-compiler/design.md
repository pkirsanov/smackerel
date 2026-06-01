# Design: 068 Structured Intent Compiler

Owner: `bubbles.design`  
Workflow mode: `product-to-planning`  
Status ceiling for this pass: `specs_hardened`  
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** The assistant facade currently builds `agent.IntentEnvelope` from raw text, optionally sets `ScenarioID` for slash shortcuts, and calls `agent.Router.Route`. Scenario execution later asks the LLM for tools. That preserves the spec 037 substrate but lets raw text and shortcut IDs decide user-facing behavior before structured interpretation.

**Target State.** Every user-facing natural-language turn produces a schema-valid `CompiledIntent` before routing, tool selection, side-effect execution, or response synthesis. The facade routes from compiled action and scenario hints, passes the compiled object into `IntentEnvelope.StructuredContext`, records it in assistant-turn audit, and refuses/clarifies/captures on compiler failure rather than routing raw text.

**Patterns to Follow.** Keep spec 037 router/executor/registry as the runtime substrate. Add compiler code under `internal/assistant/intent/`, not `internal/assistant/router`. Use `AssistantMessage` and `AssistantResponse`; transports must not parse domain intent. Persist trace data through artifacts/audit and `agent_traces` JSON payloads. Config lives under `assistant.intent_compiler.*` and fails loud.

**Patterns to Avoid.** Do not add a second router, rule engine, regex classifier, raw-text fallback after compiler failure, tool execution inside the compiler, transport-specific compiler branches, or hidden model/prompt defaults.

**Resolved Decisions.** `CompiledIntent` is embedded into `agent.IntentEnvelope.StructuredContext` rather than adding a typed field to spec 037 `IntentEnvelope`. `/ask`, `/weather`, and `/remind` produce synthetic compiled intents. `/help`, `/status`, `/reset`, `/digest`, `/recent`, and `/done` are explicit operational-command bypasses. Compiler traces persist in `assistant_turn` audit payloads and, when the executor runs, in `agent_traces.input_envelope`.

**Open Questions.** No blocking design questions. The exact model role and prompt contract are SST/planning choices constrained by this design.

## Purpose And Scope

This design owns the compiler contract between inbound user text and downstream assistant routing. It does not implement micro-tools, retire commands, add HTTP transport, or change domain behavior beyond structured request entry.

## Architecture Overview

```text
AssistantMessage
  -> operational-command carve-out
  -> intent.Compiler.Compile(raw text + conversation context)
  -> schema validation
  -> clarify/refuse/capture without router OR actionable route
  -> agent.IntentEnvelope{RawInput, StructuredContext{compiled_intent}}
  -> existing Router + Executor + tools
  -> AssistantResponse + audit trace
```

## Capability Foundation

The foundation is the `IntentCompiler` capability: one transport-neutral compiler interface, one versioned schema, and one trace contract used by every adapter.

```go
type Compiler interface {
    Compile(ctx context.Context, turn RawTurn) (CompiledIntent, CompilerTrace, error)
}

type RawTurn struct {
    UserID string
    Transport string
    TransportMessageID string
    Text string
    ConversationWindow []ContextTurn
    ReceivedAt time.Time
}
```

Policies:

- Compiler output is advisory until JSON Schema validation passes.
- The compiler does not call tools, mutate state, or choose transport rendering.
- Missing required slots become `action_class=clarify`.
- `side_effect_class` gates write and external-write execution.
- Raw text remains for trace/capture but does not drive behavior alone.

### Variation Axes

| Axis | Values | Enforcement |
|------|--------|-------------|
| Inbound source | Telegram, HTTP, scheduler/user-facing API, later adapters | adapters build `RawTurn` |
| Action class | answer, retrieve, external_lookup, internal_action, state_mutation, clarify, capture_only, refuse | schema enum |
| Side effect | none, read, write, external_read, external_write | confirmation/policy gate |
| Routing path | scenario hint, similarity tie-breaker, no-route clarify/capture/refuse | facade decision table |
| Trace state | operational bypass, compiler success, compiler failure | assistant turn payload |

## Concrete Implementations

### Go Compiler Package

Package: `internal/assistant/intent/`.

Responsibilities: render compiler request, call the ML sidecar compiler client, validate JSON, enforce confidence floors and vocabularies, and return typed compiler errors without routing raw text.

### ML Sidecar Compiler Contract

Internal service-to-service route: `POST /assistant/intent/compile`.

Request:

```json
{
  "schema_version": "v1",
  "model_id": "sst-resolved-model-id",
  "prompt_contract_version": "intent-compiler-v1",
  "raw_turn": {"text": "weather in palm springs ca tomorrow", "transport": "telegram"},
  "conversation_context": [],
  "response_schema": "compiled-intent-v1"
}
```

Response:

```json
{
  "schema_version": "v1",
  "compiled_intent": {},
  "provider": "ollama",
  "model": "sst-resolved-model-id",
  "tokens_prompt": 0,
  "tokens_completion": 0,
  "latency_ms": 0
}
```

The sidecar returns compiler JSON only; it does not execute tools.

## Data Model

### `CompiledIntent` Minimum Schema

```json
{
  "version": "v1",
  "language": "en",
  "user_goal": "string",
  "action_class": "answer|retrieve|external_lookup|internal_action|state_mutation|clarify|capture_only|refuse",
  "side_effect_class": "none|read|write|external_read|external_write",
  "scenario_hint": "string|null",
  "tool_hints": ["string"],
  "normalized_request": {},
  "slots": {},
  "missing_slots": ["string"],
  "confidence": 0.0,
  "clarification_prompt": "string|null",
  "safety_flags": ["string"],
  "source_policy": {"requires_citations": true, "allowed_source_kinds": ["graph", "tool", "web", "computation"]}
}
```

### Audit Persistence

```sql
ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS assistant_turn_payload JSONB;

CREATE INDEX IF NOT EXISTS idx_artifacts_assistant_turn_payload
    ON artifacts (artifact_type)
    WHERE artifact_type = 'assistant_turn' AND assistant_turn_payload IS NOT NULL;
```

`agent_traces.input_envelope.structured_context.compiled_intent` records the compiled intent when the executor runs.

## API And Contracts

No public endpoint is owned here. Internal contracts are `intent.Compiler`, `POST /assistant/intent/compile`, `CompiledIntent` v1, and `assistant_turn_payload`.

Facade decision table:

| Compiler Result | Facade Action |
|-----------------|---------------|
| `clarify` | clarification response, no router |
| `capture_only` | capture-as-fallback, no router |
| `refuse` | refusal response with capture policy |
| actionable with strong scenario hint | set `IntentEnvelope.ScenarioID` |
| actionable without strong hint | route by similarity using compiled goal/request |
| malformed/schema invalid | compiler error response, no router |

## Configuration

Required SST keys:

| Key | Purpose |
|-----|---------|
| `assistant.intent_compiler.enabled` | strict bool |
| `assistant.intent_compiler.model_role` | provider-routing role |
| `assistant.intent_compiler.prompt_contract_version` | prompt version |
| `assistant.intent_compiler.schema_version` | expected schema |
| `assistant.intent_compiler.timeout_ms` | request deadline |
| `assistant.intent_compiler.confidence_floor` | scenario hint floor |
| `assistant.intent_compiler.max_context_turns` | context bound |
| `assistant.intent_compiler.max_output_bytes` | output cap |
| `assistant.intent_compiler.retry_budget` | schema retry budget |

Missing keys fail startup.

## Security And Compliance

- Compiler input/output logs are redacted.
- Write and external-write intents require confirmation.
- Safety flags can force refuse or clarify, never grant permission.
- Transport adapters do not alter slots.
- QF financial-action requests are refused or routed only to non-actionable evidence surfaces.

## Observability And Failure Handling

Trace sequence: `raw_turn_received -> intent_compiled -> intent_validated -> route_selected -> tool_or_action_executed -> response_synthesized`.

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_intent_compiler_requests_total` | `outcome,action_class` | compiler results |
| `smackerel_assistant_intent_compiler_latency_seconds` | `provider,model,outcome` | compiler latency |
| `smackerel_assistant_intent_compiler_error_total` | `cause` | schema/provider/config failures |
| `smackerel_assistant_intent_bypass_total` | `reason` | operational bypass count |
| `smackerel_assistant_side_effect_blocked_total` | `side_effect_class,cause` | ungated writes blocked |

Compiler errors never silently reroute raw text.

## Testing And Validation Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-068-A01 | e2e-api | `tests/e2e/assistant/intent_compiler_http_test.go` | weather compiles before route |
| SCN-068-A02 | e2e-api | same | retrieval receives compiled context |
| SCN-068-A03 | e2e-api | `tests/e2e/assistant/intent_side_effect_test.go` | list write confirms before persistence |
| SCN-068-A04 | e2e-api | `tests/e2e/assistant/annotation_intent_test.go` | annotation slots come from compiled intent |
| SCN-068-A05 | e2e-api | `tests/e2e/assistant/intent_clarify_test.go` | ambiguity emits clarification |
| SCN-068-A06 | unit + e2e-api | `internal/assistant/intent/compiler_test.go` | malformed JSON blocks routing |
| SCN-068-A07 | unit | `internal/assistant/intent/bypass_test.go` | `/status` records bypass |
| SCN-068-A08 | policy guard | `tests/integration/policy/intent_bypass_guard_test.go` | raw route without compiled intent fails |
| SCN-068-A09 | unit/e2e-api | `internal/assistant/intent/side_effect_gate_test.go` | external-write cannot execute without confirmation |

## Alternatives And Tradeoffs

| Option | Decision | Rationale |
|--------|----------|-----------|
| Add typed field to `agent.IntentEnvelope` | Rejected for v1 | `StructuredContext` avoids unnecessary spec 037 churn |
| Raw-text route on compiler failure | Rejected | Violates owner-required path |
| Keep shortcuts as explicit ids only | Rejected | Non-uniform traces |
| Deterministic parser | Rejected | Recreates keyword/rule drift |

## Risks And Open Questions

| Risk | Mitigation |
|------|------------|
| Hallucinated slots or names | Schema and registry validation |
| Latency increase | Timeout, model-role tuning, metrics |
| Scenario hints bypass similarity | Confidence floor and policy guard |
| Sensitive slots in traces | Schema redaction and controlled trace views |
