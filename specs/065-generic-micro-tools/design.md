# Design: 065 Generic Micro-Tools

Owner: `bubbles.design`  
Workflow mode: `product-to-planning`  
Status ceiling for this pass: `specs_hardened`  
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 037 already owns the scenario loader, router, executor, tool registry, JSON Schema validation, and trace store. Spec 061 wraps that substrate in the assistant facade. Weather, recipes, lists, annotations, and `/find` still have pressure to add scenario-local parsing or prompt-side normalization.

**Target State.** Register four generic micro-tools through the existing spec 037 tool registry: `location_normalize`, `unit_convert`, `calculator`, and `entity_resolve`. Scenarios and the spec 068 compiler call these tools to normalize slots before domain tools execute. Every tool call is schema-bound, source-qualified, traceable, and configured through SST.

**Patterns to Follow.** Use [internal/agent/registry.go](../../internal/agent/registry.go), [internal/agent/executor.go](../../internal/agent/executor.go), existing package-local tool registration under `internal/agent/tools/`, [config/smackerel.yaml](../../config/smackerel.yaml), and existing agent trace tables. Provider-backed tools use explicit provider interfaces selected by required SST keys.

**Patterns to Avoid.** Do not add a second tool registry, scenario-specific branches inside generic tools, prompt-side location dictionaries, regex entity routing, hidden provider fallback chains, or config defaults. `calculator` must not evaluate identifiers, host functions, file paths, network expressions, or any financial action.

**Resolved Decisions.** Micro-tools live under `internal/agent/tools/microtools/` with primitive-oriented files or subpackages. `location_normalize` owns its own cache; the registry does not add shared caching. `entity_resolve` wraps graph/search primitives and returns entity candidates; it does not replace the retrieval scenario. `unit_convert` and `calculator` are deterministic no-egress read tools.

**Open Questions.** No design blockers. `/bubbles.plan` should choose whether all four tools land in one foundation scope or whether `location_normalize` lands first and the other tools layer after the foundation.

## Purpose And Scope

This design defines reusable normalization and computation primitives for the assistant. It does not retire commands, add transports, modify the spec 037 registry contract, or change the open-knowledge cite-back verifier.

## Architecture Overview

```text
CompiledIntent or scenario loop
  -> agent.Executor validates tool args
  -> micro-tool handler runs
  -> handler returns common envelope
  -> executor validates output and records agent_tool_calls
  -> scenario continues with normalized slots or clarification
```

The tools are ordinary spec 037 tools. They are only usable by scenarios that list them in `allowed_tools` and only when their SST config validates at startup.

## Capability Foundation

The reusable foundation is the micro-tool envelope layered on top of `agent.Tool`:

```go
type MicroToolEnvelope struct {
    SchemaVersion string               `json:"schema_version"`
    Status        string               `json:"status"` // resolved | ambiguous | failed
    Value         map[string]any       `json:"value,omitempty"`
    Candidates    []MicroToolCandidate `json:"candidates,omitempty"`
    Confidence    float64              `json:"confidence,omitempty"`
    Source        MicroToolSource      `json:"source"`
    Error         *MicroToolError      `json:"error,omitempty"`
}
```

Foundation policies:

- `resolved` requires schema-valid `value` plus source metadata.
- `ambiguous` requires ranked candidates and must route to clarification before a domain action executes.
- `failed` requires an explicit error code and must not silently choose another provider.
- Tool handlers never inspect scenario IDs or transport names.

### Variation Axes

| Axis | Values | Enforcement |
|------|--------|-------------|
| Provider protocol | local computation, graph read, external HTTP provider | tool-specific provider interface |
| Output shape | scalar conversion, canonical location, ranked entity, numeric result | per-tool JSON Schema |
| Ambiguity policy | resolved, ambiguous, failed | common envelope and facade clarification |
| Storage behavior | no persistence, in-process cache, graph read | no durable table in this spec |
| Security class | read, external | `agent.SideEffectClass` and scenario allowlist |

## Concrete Implementations

### `location_normalize`

Input:

```json
{
  "input": "palm springs ca",
  "country_hint": "US",
  "locale": "en-US",
  "max_candidates": 5
}
```

Resolved `value`:

```json
{
  "name": "Palm Springs",
  "country": "United States",
  "country_code": "US",
  "admin1": "California",
  "lat": 33.8303,
  "lon": -116.5453,
  "provider_id": "provider-specific-id"
}
```

V1 provider is Open-Meteo geocoding. Later providers implement the same interface and are selected by SST.

### `unit_convert`

Converts numeric units, including substance-aware kitchen conversions. Volume-to-mass conversions require `substance`; missing or ambiguous density data returns `ambiguous` or `failed`, not an invented result.

### `calculator`

Evaluates pure arithmetic with a dedicated parser. Allowed grammar is numeric literals, parentheses, unary signs, `+`, `-`, `*`, `/`, `%`, and explicitly enabled bounded exponentiation. Identifiers, function calls, imports, assignment, and non-finite values are rejected.

### `entity_resolve`

Resolves colloquial artifact/entity references against the authenticated user's graph. Resolution order is exact owned reference, recent context, graph relation, then vector retrieval. Close or low-confidence matches return `ambiguous`.

## Data Model

No new database tables are required. Existing `agent_tool_calls` and `agent_traces.tool_calls` hold tool-call evidence. In-process caches are disposable and keyed by provider, normalized input, and config version.

## API And Contracts

There is no public HTTP API. The contracts are the registered tool names, JSON Schemas, side-effect classes, and common envelope. Scenarios consume tools through `allowed_tools`; disabled or unknown tools fail scenario loading.

## Configuration

Required SST keys include:

| Key | Purpose |
|-----|---------|
| `assistant.tools.location_normalize.enabled` | strict bool |
| `assistant.tools.location_normalize.provider` | provider selector |
| `assistant.tools.location_normalize.timeout_ms` | per-call deadline |
| `assistant.tools.location_normalize.cache_ttl_seconds` | cache TTL |
| `assistant.tools.location_normalize.cache_max_entries` | cache bound |
| `assistant.tools.unit_convert.enabled` | strict bool |
| `assistant.tools.unit_convert.catalog_version` | conversion catalog version |
| `assistant.tools.calculator.enabled` | strict bool |
| `assistant.tools.calculator.max_expression_chars` | parser input cap |
| `assistant.tools.entity_resolve.enabled` | strict bool |
| `assistant.tools.entity_resolve.confidence_floor` | ambiguity floor |
| `assistant.tools.entity_resolve.timeout_ms` | per-call deadline |

Missing keys fail loud at config validation.

## Security And Compliance

- All tools are read-only or external-read in v1.
- `entity_resolve` scopes every query by authenticated user.
- Provider outputs are source-qualified and redacted before trace persistence.
- `calculator` never delegates to shell, Python `eval`, SQL, or network calls.
- QF financial actions are structurally excluded.

## Observability And Failure Handling

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_microtool_calls_total` | `tool,status` | call outcomes |
| `smackerel_assistant_microtool_latency_seconds` | `tool,provider,status` | latency |
| `smackerel_assistant_microtool_ambiguous_total` | `tool` | clarification pressure |
| `smackerel_assistant_microtool_provider_errors_total` | `tool,provider,cause` | provider failures |
| `smackerel_assistant_microtool_cache_total` | `tool,result` | cache hit/miss |

Provider errors produce schema-valid `failed` envelopes. Schema violations remain executor failures in existing traces. Missing config aborts startup.

## Testing And Validation Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-065-A01 | integration | `tests/integration/assistant/microtools_location_test.go` | `palm springs ca` resolves to California with provider attribution |
| SCN-065-A02 | integration | `tests/integration/assistant/microtools_location_test.go` | `sf` resolves to San Francisco |
| SCN-065-A03 | e2e-api | `tests/e2e/assistant/microtools_http_test.go` | ambiguous `springfield` produces clarification |
| SCN-065-A04 | unit + e2e-api | `internal/agent/tools/microtools/unit_test.go` and HTTP E2E | flour conversion returns numeric grams with source |
| SCN-065-A05 | unit | `internal/agent/tools/microtools/calculator_test.go` | arithmetic succeeds and forbidden identifiers reject |
| SCN-065-A06 | integration | `tests/integration/assistant/entity_resolve_test.go` | resolver returns ranked candidates and ambiguity below floor |
| SCN-065-A07 | unit | `internal/config/assistant_tools_test.go` | missing required tool config fails loud |

`/bubbles.plan` must add scenario-specific `Regression E2E` rows for every scenario.

## Alternatives And Tradeoffs

| Option | Decision | Rationale |
|--------|----------|-----------|
| Prompt-side normalization | Rejected | Repeats the weather brittleness pattern |
| Second tool registry | Rejected | Spec 037 already owns allowlist, schema, trace, and side-effect enforcement |
| Registry-level shared cache | Rejected | Tool-specific invalidation and privacy rules differ |
| New durable normalized-location table | Rejected for v1 | Tool traces and provider source are sufficient |

## Risks And Open Questions

| Risk | Mitigation |
|------|------------|
| Provider drift changes exact candidates | Assert stable shape and ambiguity behavior, not provider IDs |
| Conversion table implies false precision | Return `precision`, source, and exactness metadata |
| Entity resolver leaks cross-user artifacts | User-scoped store APIs and two-user isolation tests |
| Prompt authors keep adding normalization text | Spec 067 prompt and policy guards |
