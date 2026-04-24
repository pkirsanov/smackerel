# Design: 037 LLM Scenario Agent & Tool Registry

> **Status:** Foundational design for the LLM-Agent + Tools capability
> committed to in [docs/smackerel.md §3.6](../../docs/smackerel.md) and the
> "Agent + Tool Development Discipline" section of
> [docs/Development.md](../../docs/Development.md). Specs
> [034 expense tracking](../034-expense-tracking/),
> [035 recipe enhancements](../035-recipe-enhancements/), and
> [036 meal planning](../036-meal-planning/) build on this surface — none of
> them re-implements intent routing, tool dispatch, schema validation, or
> trace recording.

---

## Design Brief

**Current State.** Domain reasoning today lives as Go code: ~50 hardcoded
vendor seed aliases, a 7-level expense classifier rule chain, ingredient
keyword maps, and 30+ regex branches in the Telegram intent dispatcher. LLM
use is confined to fixed extraction prompt contracts in
[config/prompt_contracts/](../../config/prompt_contracts/) executed by
[ml/app/synthesis.py](../../ml/app/synthesis.py); the LLM cannot call tools,
cannot replan, and cannot drive a multi-step interaction. There is no
cross-cutting trace store for LLM-driven outcomes.

**Target State.** A Go-owned scenario agent that, given an intent envelope,
selects a declarative scenario file, drives an LLM tool-calling loop through
the Python ML sidecar, validates every tool call against a server-side
allowlist and JSON Schema, persists a full audit trace to PostgreSQL, and
returns either a schema-valid output or a structured failure outcome to the
caller. Adding a new domain capability is a YAML drop plus, when needed, a
single `RegisterTool` call in the package that owns the data.

**Patterns to Follow.**
- Prompt contract YAML shape from
  [config/prompt_contracts/cross-source-connection-v1.yaml](../../config/prompt_contracts/cross-source-connection-v1.yaml)
  (`version`, `type`, `description`, `system_prompt`, schema block,
  `validation_rules`, `token_budget`, `temperature`, `model_preference`).
- NATS-mediated Go ↔ Python boundary as in
  [config/nats_contract.json](../../config/nats_contract.json) (one new
  stream `AGENT`, single source of truth, both sides verify constants).
- Generated config + zero-defaults from
  [config/smackerel.yaml](../../config/smackerel.yaml) →
  `config/generated/*.env` pipeline (no hardcoded ports, hosts, or model
  names anywhere in code).
- Per-package data ownership as in `internal/recipe/`,
  `internal/intelligence/`, `internal/knowledge/` — tools register from the
  package that owns the data they touch.
- litellm provider routing as in
  [ml/app/synthesis.py](../../ml/app/synthesis.py) (Ollama / hosted), and
  [docs/Development.md](../../docs/Development.md) Tool Conventions.

**Patterns to Avoid.**
- The Telegram regex/switch intent dispatcher in `internal/telegram/` —
  hardcoded keyword mapping is the exact anti-pattern this spec exists to
  remove. The agent's intent router replaces it; do not extend it.
- The multi-level expense classifier ladder in `internal/intelligence/` —
  business policy in Go branches; replace with scenario + read-only tools.
- Hardcoded vendor / alias / synonym seed lists in Go source — must be moved
  behind a `lookup_*` tool that consults PostgreSQL.
- Embedding business policy inside extraction prompt contracts (e.g.,
  "if vendor looks like a grocery store and amount > X..."). Policy belongs
  in scenario prompts; extraction contracts stay minimal and stable.
- Central registration tables (e.g., a single `tools.go` file enumerating
  every tool) — registration is decentralized via package `init()` only.

**Resolved Decisions.**
- Go owns: scenario loader, tool registry, allowlist enforcement, JSON
  Schema validation (in/out), execution loop, trace store, intent routing.
- Python owns: prompt rendering, LLM provider call, tool-call parsing from
  provider response, per-token streaming. Python is stateless; it never
  decides which tool to execute and never persists state.
- New NATS stream: `AGENT` with subjects `agent.invoke.*`,
  `agent.complete.*`, `agent.tool_call.*`, `agent.tool_result.*`.
- Trace store: PostgreSQL tables `agent_traces` (one row per invocation)
  and `agent_tool_calls` (one row per tool call), with `tool_calls jsonb[]`
  denormalized snapshot on the trace row for fast inspection.
- Intent routing: embedding similarity over `intent_examples` from each
  scenario, plus explicit `scenario_id` override, plus router-scenario
  fallback when similarity is below threshold. No regex, no switch, no
  hardcoded keyword tables.
- Side-effect classes: `read` | `write` | `external` declared on every tool
  and re-declared on every scenario allowlist entry. Mismatch = registration
  refusal at startup.
- Scenarios are versioned; in-flight invocations complete against the
  version that started them (BS-019).
- Existing extraction prompt contracts (recipe, receipt, product, ingestion-
  synthesis, cross-source-connection, query-augment, lint-audit, digest-
  assembly) are unchanged. Migration to scenarios is opt-in per contract.

**Open Questions.**
- Hot reload signal: SIGHUP vs. NATS control message vs. polling? Default
  to SIGHUP for v1; revisit if ops surfaces it.
- Embedding model for intent routing: reuse `runtime.embedding_model`
  (`nomic-embed-text`) or a dedicated smaller model? Default: reuse.
- Trace retention: 30 days hot in PostgreSQL, then prune; archival not in
  scope for v1.
- Per-scenario provider override: design supports it via
  `model_preference`; not exposed in v1 unless a scenario requires it.

---

## 1. Architecture Placement

```
┌─────────────────── Go Core ──────────────────────┐    ┌──── Python ML Sidecar ────┐
│                                                  │    │                            │
│  internal/agent/                                 │    │  ml/app/agent.py           │
│   ├ loader.go      ── scenario YAML parse + lint │    │   ├ render_prompt()        │
│   ├ registry.go    ── tool registry, RegisterTool│    │   ├ call_llm()  (litellm)  │
│   ├ router.go      ── intent → scenario          │    │   └ parse_tool_calls()     │
│   ├ executor.go    ── loop, validate, dispatch   │    │                            │
│   ├ tracer.go      ── persist agent_traces       │◄──►│  request/response only;    │
│   ├ schema.go      ── JSON Schema validate (in/out)   │  no state, no decisions    │
│   └ replay.go      ── deterministic replay       │    │                            │
│                                                  │    └────────────────────────────┘
│  internal/<domain>/tools.go (per package)        │              ▲
│   └ init() { agent.RegisterTool(...) }           │              │ NATS AGENT stream
│                                                  │              │
│  Surfaces:                                       │              ▼
│   internal/telegram/, internal/api/,             │    nats://nats:4222
│   internal/scheduler/, internal/pipeline/        │
│        └ hand intent envelope to executor        │
│                                                  │
│  PostgreSQL                                      │
│   ├ agent_traces       (one row per invocation)  │
│   └ agent_tool_calls   (one row per tool call)   │
└──────────────────────────────────────────────────┘
```

**Go core owns** (authoritative; never delegated):
- Scenario loading and load-time validation (allowlist references, schema
  self-test, id uniqueness, required fields).
- Tool registry with `RegisterTool(...)`.
- Allowlist enforcement before tool dispatch.
- JSON Schema validation of tool args (before dispatch) and tool returns
  (after dispatch).
- The execution loop (render → call → parse → validate → execute → loop).
- Limits: `max_loop_iterations`, per-invocation `timeout_ms`, schema-retry
  budget.
- Trace persistence (atomic per invocation; concurrent isolation via
  `trace_id`).
- Intent routing decision (which scenario fired, with what scores).
- Replay against fixtures.

**Python ML sidecar owns** (and only these):
- Rendering the scenario `system_prompt` with the input envelope.
- The LLM provider call via litellm (Ollama or hosted).
- Parsing the provider's tool-call format into a normalized
  `{name, arguments_json}` envelope returned to Go.
- Streaming token usage back to Go for the trace.

**NATS subjects** (added to `config/nats_contract.json` under stream
`AGENT`, pattern `agent.>`):

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `agent.invoke.request` | Go → Python | Start an LLM turn for a `trace_id` |
| `agent.invoke.response` | Python → Go | LLM returned a tool-call list or final output |
| `agent.tool_call.executed` | Go internal log | Tool dispatch + result (also persisted) |
| `agent.complete` | Go → caller | Final structured outcome envelope |

The Go executor is the only thing that decides whether to loop again; a
single LLM turn is one `agent.invoke.request` ↔ `agent.invoke.response`
pair.

---

## 2. Scenario Contract

Scenarios extend the existing prompt-contract YAML shape with one
discriminator (`type: scenario`) and the agent-specific fields. They live
in [config/prompt_contracts/](../../config/prompt_contracts/) (or a
`scenarios/` subdirectory; both are scanned).

### 2.1 YAML schema

```yaml
version: "expense-question-v1"        # snake-case id + -vN
type: "scenario"                       # discriminator (existing types unchanged)
id: "expense_question"                 # stable scenario id, snake_case, must equal slug of version sans -vN
description: "Answer a natural-language question about expenses"

intent_examples:                       # router uses these for embedding similarity
  - "how much did I spend on groceries last week"
  - "what were my biggest expenses this month"
  - "show me restaurant spend in Q1"

system_prompt: |
  You are the expense-question agent.
  Use the provided tools to answer questions about the user's expenses.
  ...

allowed_tools:                         # MUST be subset of registered tools
  - name: "search_expenses"
    side_effect_class: "read"          # MUST match tool registration; mismatch = load failure
  - name: "aggregate_amounts"
    side_effect_class: "read"
  - name: "format_currency"
    side_effect_class: "read"
  - name: "format_answer"
    side_effect_class: "read"

input_schema:                          # JSON Schema for the structured_context envelope
  type: object
  required: [user_tz, now]
  properties:
    user_tz: { type: string }
    now: { type: string, format: "date-time" }
    contact: { type: string, x-redact: true }   # x-redact ⇒ trace shows "***"

output_schema:                         # JSON Schema for the final response
  type: object
  required: [answer, sources]
  properties:
    answer: { type: string, maxLength: 2000 }
    sources: { type: array, items: { type: string } }

limits:
  max_loop_iterations: 8               # absolute cap on tool calls per invocation
  timeout_ms: 30000                    # wall-clock cap per invocation
  schema_retry_budget: 2               # how many output-schema retries before failure
  per_tool_timeout_ms: 5000            # default; tools may override

token_budget: 4000                     # passed to ML sidecar (existing field)
temperature: 0.2
model_preference: "default"            # "default" | "reasoning" | "fast" | "vision" | "ocr"

side_effect_class: "read"              # scenario-level: highest class across allowed_tools
                                       # MUST be ≥ max(allowed_tools[].side_effect_class)
```

### 2.2 Load-time validation rules

Loader runs all of these before a scenario is registered. Any failure ⇒
scenario is **rejected** (other scenarios continue to load); two scenarios
with the same `id` ⇒ **process refuses to start** (BS-011).

| Rule | Failure outcome |
|------|-----------------|
| All required top-level fields present (`id`, `version`, `type`, `system_prompt`, `allowed_tools`, `input_schema`, `output_schema`, `limits`, `side_effect_class`) | Reject scenario; structured error names file + missing field (BS-009) |
| `id` matches `^[a-z][a-z0-9_]*$` | Reject |
| `version` ends with `-v<N>` and the prefix slug equals `id` | Reject |
| Every `allowed_tools[].name` is in the registry | Reject; error names missing tool + scenario id (BS-010, A1) |
| Every `allowed_tools[].side_effect_class` equals the tool's registered class | Reject; error shows expected vs declared |
| `side_effect_class` ≥ max of `allowed_tools[].side_effect_class` (`read` < `write` < `external`) | Reject |
| `input_schema` and `output_schema` are valid JSON Schema (self-test compile) | Reject (BS-009 A4) |
| `output_schema` does not violate the redaction policy (no `x-redact: true` on required fields) | Reject |
| No two registered scenarios share an `id` | Process refuses to start (BS-011) |
| `limits.max_loop_iterations` in `[1, 32]`; `timeout_ms` in `[1000, 120000]`; `schema_retry_budget` in `[0, 5]` | Reject |
| `intent_examples` non-empty if scenario is reachable via routing (system-only scenarios may set `intent_examples: []` and require explicit `scenario_id`) | Reject if reachable + empty |

`content_hash = sha256(canonical_yaml)` is computed and stored alongside the
in-memory scenario record. The hash is recorded on every trace (§6) so
replay (UC-003) can detect that the scenario file changed since the trace.

### 2.3 Hot reload (BS-019)

On SIGHUP the loader re-scans the scenario directory and atomically swaps
the in-memory registry. In-flight invocations hold a reference to the
scenario record they started with; only new invocations see the new
version. The trace records the resolved `version` per invocation.

---

## 3. Tool Registry

### 3.1 Registration API

```go
// internal/agent/registry.go
package agent

type SideEffectClass string

const (
    SideEffectRead     SideEffectClass = "read"
    SideEffectWrite    SideEffectClass = "write"
    SideEffectExternal SideEffectClass = "external"
)

type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

type Tool struct {
    Name             string           // snake_case, globally unique
    Description      string           // one-line, used by LLM for tool selection
    InputSchema      json.RawMessage  // JSON Schema (Draft 2020-12)
    OutputSchema     json.RawMessage  // JSON Schema
    SideEffectClass  SideEffectClass
    OwningPackage    string           // for trace + ops attribution
    PerCallTimeoutMs int              // 0 = use scenario default
    Handler          ToolHandler
}

// RegisterTool is called from package init() in the package that owns the data.
// It panics on duplicate name (process refuses to start), malformed schema, or
// missing required fields. There is no central registration table.
func RegisterTool(t Tool)
```

Example call site:

```go
// internal/intelligence/tools.go
package intelligence

func init() {
    agent.RegisterTool(agent.Tool{
        Name:            "search_expenses",
        Description:     "Search expense records by date range, vendor, or category",
        InputSchema:     mustEmbed("schemas/search_expenses.in.json"),
        OutputSchema:    mustEmbed("schemas/search_expenses.out.json"),
        SideEffectClass: agent.SideEffectRead,
        OwningPackage:   "intelligence",
        Handler:         searchExpenses,
    })
}
```

### 3.2 Side-effect classes

| Class | Meaning | Allowlist behavior |
|-------|---------|--------------------|
| `read` | Pure read of local state (PostgreSQL, in-memory caches) | Allowed in any scenario that lists the name |
| `write` | Mutates local state | Allowed only when scenario-level `side_effect_class >= write` AND the tool is explicitly listed |
| `external` | Calls an external network service or non-deterministic source (LLM-as-tool, third-party API) | Allowed only when scenario-level `side_effect_class == external` AND the tool is explicitly listed |

A scenario whose declared class is lower than any allowed tool's class
fails to register (§2.2).

### 3.3 JSON Schema validation

- Schemas use JSON Schema Draft 2020-12 via the existing schema library
  already wired for prompt contracts.
- `InputSchema` is checked **before** the handler runs; failure ⇒ tool
  call rejected, structured error returned to LLM (BS-004).
- `OutputSchema` is checked **after** the handler returns; failure ⇒
  tool call result discarded, invocation fails with `tool-return-invalid`
  (BS-005). No downstream artifact is updated.
- Schemas may use `x-redact: true` on string fields; trace renders those
  values as `***` (§7).
- Schema compilation is done once at registration; mutating a registered
  tool's schema at runtime is impossible.

---

## 4. Intent Routing

Surfaces (Telegram, API, scheduler, pipeline) hand the executor a typed
envelope:

```go
type IntentEnvelope struct {
    Source            string          // "telegram" | "api" | "scheduler" | "pipeline"
    RawInput          string          // free-text intent (may be empty for system triggers)
    StructuredContext json.RawMessage // surface-specific structured context
    ScenarioID        string          // optional explicit override; bypasses similarity routing
    ConfidenceFloor   float32         // optional override of cfg default
}
```

### 4.1 Routing decision table

```
1. If envelope.ScenarioID != "":
     scenario := registry.ByID(envelope.ScenarioID)
     if scenario == nil:                       → outcome = unknown-intent
     route_reason = "explicit_scenario_id"
     proceed.

2. Else (similarity routing):
     embed envelope.RawInput once.
     for each registered scenario S:
         similarity[S] = max( cosine(embed, embed(example))
                              for example in S.intent_examples )
     ranked = sort scenarios by similarity desc
     top = ranked[0]
     if top.similarity >= cfg.routing.confidence_floor:
         scenario = top; route_reason = "similarity_match"; proceed.
     elif cfg.routing.fallback_scenario_id is set
          and registry.ByID(fallback) exists:
         scenario = registry.ByID(fallback); route_reason = "fallback_clarify"
         proceed (the fallback scenario's job is to ask a clarifying question
         or return a structured unknown-intent outcome).
     else:
         outcome = unknown-intent
         trace.routing.considered = ranked[:cfg.routing.consider_top_n]
```

Trace always records: every considered scenario id with its similarity
score, the chosen scenario id, and the route reason (BS-002, BS-014).

### 4.2 Configuration

```yaml
agent:
  routing:
    confidence_floor: 0.65       # REQUIRED; SST — no Go-side default
    consider_top_n: 5
    fallback_scenario_id: "clarify_intent"  # may be empty string
    embedding_model: ""          # empty ⇒ inherit runtime.embedding_model
```

### 4.3 Forbidden in any code touching routing

- `regexp.MustCompile` over user input for the purpose of intent
  classification.
- `switch input` / `if strings.Contains(input, "...")` chains for intent
  selection.
- Keyword maps (`map[string]ScenarioID`) used for routing.
- Hardcoded vendor/alias lists used to inflect routing.

The scenario linter (§12) statically rejects all four in any file under
`internal/agent/`, `internal/telegram/dispatch*`, `internal/api/intent*`,
and `internal/scheduler/`.

---

## 5. Execution Loop

### 5.1 Loop pseudocode

```
trace := tracer.Begin(scenario, envelope)
ctx, cancel := context.WithTimeout(parent, scenario.Limits.TimeoutMs)
defer cancel()

input_valid := schema.Validate(scenario.InputSchema, envelope.StructuredContext)
if !input_valid: return outcome(input-schema-violation), trace.End(...)

iter := 0
schema_retries := 0
turn_messages := []  // accumulating LLM conversation, includes tool results

for {
    iter++
    if iter > scenario.Limits.MaxLoopIterations:
        return outcome(loop-limit), trace.End(reason="max_iterations")

    // 1) Ask Python for next LLM turn.
    resp, err := nats.Request("agent.invoke.request", {
        trace_id, scenario_id, version, system_prompt, tool_defs, turn_messages,
        token_budget, temperature, model_preference, deadline = ctx.Deadline()
    })
    if ctx timeout: return outcome(timeout)
    if err != nil:  return outcome(provider-error)
    trace.RecordTurn(resp.tokens, resp.provider, resp.model)

    // 2) If the model returned a final answer (no tool calls), validate it.
    if len(resp.tool_calls) == 0:
        if schema.Validate(scenario.OutputSchema, resp.final):
            return outcome(ok, resp.final), trace.End(...)
        schema_retries++
        if schema_retries > scenario.Limits.SchemaRetryBudget:
            return outcome(schema-failure), trace.End(...)
        turn_messages.append(systemRetryMessage(schema_error))
        continue

    // 3) Process each tool call sequentially.
    for _, call := range resp.tool_calls {
        if !registry.Has(call.name):
            trace.RecordRejection(call, reason="unknown_tool")
            turn_messages.append(toolErrorMessage(call, "tool_not_found", available))
            continue
        if !scenario.Allows(call.name):
            trace.RecordRejection(call, reason="not_in_allowlist")
            turn_messages.append(toolErrorMessage(call, "tool_not_allowed", allowlist))
            continue
        if !schema.Validate(tool.InputSchema, call.arguments):
            trace.RecordRejection(call, reason="argument_schema_violation", err)
            turn_messages.append(toolErrorMessage(call, "argument_invalid", err))
            continue

        result, err := executeWithTimeout(tool, call.arguments, perToolDeadline)
        if err != nil:
            trace.RecordToolError(call, err)
            turn_messages.append(toolErrorMessage(call, "tool_error", err.Error()))
            continue
        if !schema.Validate(tool.OutputSchema, result):
            trace.RecordReturnInvalid(call, result, err)
            return outcome(tool-return-invalid), trace.End(...)

        trace.RecordToolCall(call, result, latency_ms)
        turn_messages.append(toolResultMessage(call, result))
    }
    // loop continues: next LLM turn sees the appended results
}
```

### 5.2 Limits and budgets

| Limit | Source | Default | Trigger outcome |
|-------|--------|---------|-----------------|
| `max_loop_iterations` | scenario | required | `loop-limit` (BS-008) |
| `timeout_ms` | scenario | required | `timeout` (BS-021) |
| `schema_retry_budget` | scenario | required | `schema-failure` (BS-007) |
| `per_tool_timeout_ms` | tool override or scenario | required | `tool-error` for that call only |

There are no Go-side fallback defaults; missing values fail load
validation (§2.2).

### 5.3 Hallucinated and disallowed tool handling

Both unknown tool names (BS-006) and disallowed-but-registered tool names
(BS-003) are rejected **before** any handler runs. Each rejection appends
a structured error message to `turn_messages` so the LLM sees, in its next
turn, exactly which tools are available and may retry. Rejections do not
consume the iteration budget — but each LLM turn does.

---

## 6. Trace Store

### 6.1 PostgreSQL schema

```sql
CREATE TABLE agent_traces (
    trace_id          TEXT PRIMARY KEY,                -- e.g., trace_<rfc3339>_<rand>
    scenario_id       TEXT NOT NULL,
    scenario_version  TEXT NOT NULL,
    scenario_hash     TEXT NOT NULL,                   -- sha256 of scenario YAML at invocation
    source            TEXT NOT NULL,                   -- "telegram" | "api" | "scheduler" | "pipeline"
    input_envelope    JSONB NOT NULL,                  -- redacted per scenario.input_schema x-redact
    routing           JSONB NOT NULL,                  -- {reason, considered:[{id,score}], chosen}
    tool_calls        JSONB NOT NULL DEFAULT '[]',     -- denormalized [{seq,name,args,result_summary,outcome,latency_ms}]
    final_output      JSONB,                           -- redacted; null on failure outcomes
    outcome           TEXT NOT NULL,                   -- "ok" | "unknown-intent" | "schema-failure" |
                                                      --   "loop-limit" | "timeout" | "tool-error" |
                                                      --   "tool-return-invalid" | "allowlist-violation" |
                                                      --   "hallucinated-tool" | "input-schema-violation" |
                                                      --   "provider-error"
    outcome_detail    JSONB,                           -- structured failure details
    provider          TEXT NOT NULL,                   -- "ollama" | "openai" | ...
    model             TEXT NOT NULL,
    tokens_prompt     INTEGER NOT NULL DEFAULT 0,
    tokens_completion INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_traces_started_at  ON agent_traces (started_at DESC);
CREATE INDEX idx_agent_traces_scenario    ON agent_traces (scenario_id, started_at DESC);
CREATE INDEX idx_agent_traces_outcome     ON agent_traces (outcome, started_at DESC);
CREATE INDEX idx_agent_traces_source      ON agent_traces (source, started_at DESC);

CREATE TABLE agent_tool_calls (
    trace_id          TEXT NOT NULL REFERENCES agent_traces(trace_id) ON DELETE CASCADE,
    seq               INTEGER NOT NULL,                -- 1-based per trace
    tool_name         TEXT NOT NULL,
    side_effect_class TEXT NOT NULL,
    arguments         JSONB NOT NULL,                  -- validated args, redacted
    result            JSONB,                           -- full result (redacted), null on rejection
    rejection_reason  TEXT,                            -- null on success
    error             TEXT,                            -- null on success
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (trace_id, seq)
);
```

The `tool_calls` JSONB array on the trace row is the fast-path snapshot
for list/detail UIs and exports. The normalized `agent_tool_calls` table is
the authoritative record for query and replay.

Concurrent invocations write only their own `trace_id`; isolation is
trivially guaranteed by the primary key (BS-018).

### 6.2 Replay mechanism (UC-003)

```
replay(trace_id, fixtures_dir):
    trace := db.LoadTrace(trace_id)
    scenario := registry.ByID(trace.scenario_id)
    if scenario == nil:                         → diff: scenario_missing
    if scenario.version != trace.scenario_version:
        if not in --allow-version-drift mode:   → diff: scenario_version_changed
    if scenario.content_hash != trace.scenario_hash and not --allow-content-drift:
        → diff: scenario_content_changed

    fakeRegistry := tools that read responses from fixtures_dir keyed by
                    (tool_name, sha256(canonicalize(arguments)))
    fakeProvider := returns recorded LLM responses from trace.turn_log

    new_trace := executor.Run(scenario, trace.input_envelope,
                              registry=fakeRegistry, provider=fakeProvider)

    diff := compare(trace.tool_calls, new_trace.tool_calls)
            ∪ compare(trace.final_output, new_trace.final_output)
    return diff   // empty = no behavior drift (UC-003 main flow)
```

Recording the per-turn LLM response (the messages exchanged with the
provider) is gated by config `agent.trace.record_llm_messages: true`;
disabled by default to keep trace size bounded, enabled in dev/test.

---

## 7. Security

### 7.1 Allowlist enforced server-side

Allowlist enforcement happens in `executor.go` step (3) of §5.1, in Go,
**before** any handler is dispatched and **independently** of any
instruction the LLM receives. The system prompt is a hint to the model;
it is not a security boundary. BS-020 (adversarial prompt) is satisfied
because the user-supplied "ignore instructions" text never reaches the
allowlist check — the LLM may emit a `delete_all_expenses` tool call, but
the executor refuses to dispatch it. The trace records the rejection.

### 7.2 Prompt injection defenses

- User-supplied content is always passed as a structured `user` message in
  `turn_messages`, never concatenated into the `system_prompt`.
- Tool results returned to the LLM are wrapped as
  `{"role": "tool", "tool_call_id": ..., "content": <json>}` envelopes,
  so user-controlled data inside a tool result cannot be confused with a
  scenario-level instruction.
- The scenario `system_prompt` may not be templated with raw input. If
  templating is needed, it is restricted to fields from the validated
  `structured_context`.
- Allowlist + schema validation make injected "please call tool X with
  args Y" instructions inert when X is not allowlisted or Y fails the
  schema.

### 7.3 Secret redaction in traces

- Schema fields marked `x-redact: true` are replaced with `"***"` before
  the trace row is written. Redaction happens in `tracer.go`; redaction
  is not a tool responsibility.
- Redaction applies to: `input_envelope`, every `agent_tool_calls.arguments`,
  every `agent_tool_calls.result`, and `final_output`.
- Replay (§6.2) supplies the redacted values out-of-band from
  `fixtures_dir`, so traces remain redaction-safe at rest while still
  being deterministically replayable in dev (BS-022).

### 7.4 Scenario integrity

- `content_hash` (sha256 of canonical YAML) is stamped on every trace.
- Replay surfaces scenario_content_changed diffs, so silent edits to a
  scenario file cannot mask a behavior change.
- The scenario linter in CI (§12) recomputes hashes and refuses commits
  that mutate `-v1` semantics without bumping `version`.

### 7.5 Concurrent isolation

- Each invocation has its own `trace_id`, its own `turn_messages` slice,
  and its own context.
- The executor holds no global mutable state per scenario. The registry
  is read-only after startup (and after hot reload swap).
- Trace writes are scoped by `trace_id`; the database guarantees no
  cross-invocation interleaving (BS-018).

---

## 8. Failure-Mode Map

Every adversarial business scenario from spec.md maps to a single, named
code path producing a single, structured outcome.

| BS | Code path | Outcome | User-visible result |
|----|-----------|---------|---------------------|
| BS-001 | loader scans dir; registers new file | n/a (load-time) | Scenario invokable; no other behavior changes |
| BS-002 | `router.go` similarity routing; trace records `considered` + `chosen` | `ok` | Correct scenario fires; trace shows scoring |
| BS-003 | executor §5.1 step (3) `scenario.Allows(name)` check | `allowlist-violation` recorded per call; final outcome may still be `ok` if LLM recovers | LLM gets `tool_not_allowed`; user gets the answer the legal tools could produce |
| BS-004 | executor §5.1 step (3) `schema.Validate(InputSchema, args)` | per-call rejection; final outcome `ok` after retry | LLM retries with valid args; user sees no error |
| BS-005 | executor §5.1 step (3) post-handler `schema.Validate(OutputSchema, result)` | `tool-return-invalid` (terminal) | Surface emits a structured failure (no malformed downstream write) |
| BS-006 | executor §5.1 step (3) `registry.Has(name)` check | per-call rejection; final outcome may be `ok` after retry | LLM retries with a real tool; user gets the answer |
| BS-007 | executor §5.1 step (2) output schema fail + `schema_retry_budget` exhaustion | `schema-failure` | Surface emits structured failure (e.g., Telegram "I couldn't structure that answer; please try again") |
| BS-008 | executor §5.1 step (1) `iter > MaxLoopIterations` | `loop-limit` | Surface emits structured failure naming the cap |
| BS-009 | loader rejects malformed file; logs structured error; continues | n/a (load-time) | Service starts; bad scenario absent; valid scenarios load |
| BS-010 | loader allowlist-references-registered-tool check | n/a (load-time) | Service starts; bad scenario absent |
| BS-011 | loader id-uniqueness check | n/a (load-time) | Service refuses to start |
| BS-012 | tracer.go writes one row per invocation including routing + tool_calls + final_output | (any outcome) | Operator UI reconstructs the decision path from the row alone |
| BS-013 | replay.go diff against scenario at HEAD | replay diff (not a runtime outcome) | Test passes / structured diff |
| BS-014 | router.go falls below `confidence_floor` and no fallback or fallback exhausts | `unknown-intent` | Surface chooses how to ask for clarification; agent never invents |
| BS-015 | executor §5.1 step (3) handler returns error | per-call `tool-error` recorded; final outcome depends on LLM recovery | LLM recovers or finalizes failure; trace shows both errors |
| BS-016 | NATS `agent.invoke.request` carries `model_preference`; ML sidecar resolves provider | `ok` | No scenario edit; trace records actual provider/model |
| BS-017 | loader side-effect-class-consistency check + executor allowlist check (§7.1) | per-call `allowlist-violation`; loader rejects misclassified scenarios | Write tool never executes from a read-only scenario |
| BS-018 | per-`trace_id` isolation; PK on `(trace_id, seq)` | n/a (data-shape) | Each trace shows only its own calls |
| BS-019 | hot reload swap; in-flight invocation holds prior scenario record | (any outcome) | Trace records the exact `version` that handled it |
| BS-020 | executor allowlist check is in Go, not delegated to LLM | per-call `allowlist-violation`; final may be `ok` | No write tool runs; trace shows the attempt |
| BS-021 | `context.WithTimeout(scenario.Limits.TimeoutMs)` | `timeout` | Surface emits structured failure; other invocations unaffected |
| BS-022 | tracer.go applies `x-redact` before persistence | n/a (data-shape) | Trace renders `***`; replay supplies values from fixtures |

---

## 9. Extension Points

### 9.1 Adding a new scenario (zero Go changes)

1. Drop a YAML file in `config/prompt_contracts/` (or a `scenarios/`
   subdirectory) following §2.1.
2. SIGHUP the running service (or restart).
3. Scenario is invokable by id and reachable via routing if
   `intent_examples` is non-empty.

No edits to: routing code, tool dispatch, registry, executor, surfaces.

### 9.2 Adding a new tool

1. In the package that owns the data the tool touches (e.g.,
   `internal/recipe/`), add a `tools.go` file (or extend the existing one).
2. In `init()`, call `agent.RegisterTool(...)` with name, description,
   schemas, side-effect class, owning package, handler.
3. Restart the service. The tool is now available to any scenario that
   allowlists it.

No edits to: a central tools registry file, the executor, the loader, or
any other package's `tools.go`.

### 9.3 Adding a new surface

A new surface (e.g., a CLI subcommand, a new API endpoint, a new
scheduler trigger) calls `agent.Executor.Run(ctx, IntentEnvelope{...})`
and handles the structured outcome. No agent-side edits required.

---

## 10. Migration

### 10.1 Existing prompt contracts coexist

The eight existing prompt contracts in
[config/prompt_contracts/](../../config/prompt_contracts/) declare
`type: "extraction"` (or similar non-`scenario` types). The loader skips
non-`scenario` types and the existing
[ml/app/synthesis.py](../../ml/app/synthesis.py) handlers continue to
process them via the `SYNTHESIS` and related NATS streams. No change to
those contracts is required by 037.

### 10.2 Opt-in convergence

When (and only when) a piece of existing functionality genuinely benefits
from tool-calling — e.g., expense classification needs to look up vendor
history before deciding — the maintainer:

1. Authors a new scenario in the prompt-contract directory.
2. Registers the necessary tools (which may wrap existing extraction
   helpers).
3. Switches the calling surface (e.g., the expense pipeline) from
   "synthesis-style request" to `agent.Executor.Run(...)`.
4. Leaves the old extraction contract in place until traffic has been
   verified on the new path. Then deletes it.

There is no big-bang migration. Each domain can move independently.

### 10.3 Specs 034 / 035 / 036 dependency

Specs 034 (expense tracking), 035 (recipe enhancements), and 036 (meal
planning) reference this design as the **only** supported way to
implement intent routing, classification chains, and multi-step domain
flows. Any scope plan in those specs that proposes a new regex router, a
new keyword categorization map, or a new hardcoded vendor list is a
violation of this design and must be reworked as a scenario + tool.

---

## 11. Configuration

A new `agent:` block is added to
[config/smackerel.yaml](../../config/smackerel.yaml). All values are
required (SST zero-defaults — no Go fallback). The
`./smackerel.sh config generate` pipeline materializes them into
`config/generated/dev.env` and `config/generated/test.env`.

```yaml
agent:
  scenario_dir: "config/prompt_contracts"   # REQUIRED; service refuses to start if missing
  scenario_glob: "*.yaml"                   # REQUIRED; loader scans matches under scenario_dir
  hot_reload: true                          # REQUIRED; SIGHUP rescans

  routing:
    confidence_floor: 0.65                  # REQUIRED
    consider_top_n: 5                       # REQUIRED
    fallback_scenario_id: ""                # REQUIRED ("" = no fallback)
    embedding_model: ""                     # REQUIRED ("" = inherit runtime.embedding_model)

  trace:
    retention_days: 30                      # REQUIRED
    record_llm_messages: false              # REQUIRED (true in dev/test, false in prod)
    redact_marker: "***"                    # REQUIRED

  defaults:
    # These are *failsafe ceilings*, not Go fallbacks. A scenario MUST declare
    # its own limits; these caps clamp scenario values that exceed them.
    max_loop_iterations_ceiling: 32         # REQUIRED
    timeout_ms_ceiling: 120000              # REQUIRED
    schema_retry_budget_ceiling: 5          # REQUIRED
    per_tool_timeout_ms_ceiling: 30000      # REQUIRED

  provider_routing:
    # Map model_preference → ml/app/agent.py provider/model. SST.
    default:   { provider: "ollama",    model: "gemma4:26b"     }
    reasoning: { provider: "ollama",    model: "deepseek-r1:32b" }
    fast:      { provider: "ollama",    model: "gpt-oss:20b"    }
    vision:    { provider: "ollama",    model: "gemma4:26b"     }
    ocr:       { provider: "ollama",    model: "deepseek-ocr:3b" }
```

NATS contract: `config/nats_contract.json` gains the `AGENT` stream
entry; both Go and Python NATS-constants tests must continue to pass.

Forbidden in code:
- `getEnv("AGENT_X", "fallback")` — must be `os.Getenv` + empty check +
  fatal.
- Hardcoded `0.65`, `30000`, `8`, etc. anywhere in `internal/agent/`.

---

## 12. Testing

### 12.1 Test pyramid

| Layer | Scope | Where | Notes |
|-------|-------|-------|-------|
| Unit (Go) | loader, registry, schema validate, executor with fake LLM, router | `internal/agent/*_test.go` | Fake LLM returns scripted tool-call sequences; no NATS, no network |
| Unit (Python) | prompt rendering, tool-call parsing, provider routing | `ml/tests/test_agent.py` | No live LLM; mock litellm `acompletion` |
| Replay snapshots | per-scenario fixtures + recorded traces | `tests/integration/agent/replay/<scenario_id>/` | One snapshot per BS-* that exercises the scenario; runs offline; updated only by explicit `--update` flag |
| Integration | live ML sidecar over NATS, fake LLM provider in sidecar | `tests/integration/agent/` | Verifies the AGENT NATS contract end-to-end; no real provider |
| E2E (live stack) | Telegram + scheduler + agent + Ollama | `tests/e2e/agent/` | Disposable test stack only; uses real Ollama with a small deterministic prompt |
| Stress | concurrent invocations | `tests/stress/agent/` | Asserts BS-018 isolation and per-invocation timeout under load |
| Scenario linter | static check of every YAML under `config/prompt_contracts/` | `cmd/scenario-lint/` (CI) | Implements the load-time rules of §2.2 plus forbidden-pattern checks of §4.3; runs in CI on every PR |

### 12.2 Required adversarial coverage

Every adversarial BS in §8 MUST have at least one test that **would fail
if the failure-mode handling were removed**. No bailouts (`if outcome ==
"ok": return`); each test asserts the specific structured outcome and the
specific trace shape produced by the bug-fix path. This is enforced by
[bubbles-test-integrity](../../.github/skills/bubbles-test-integrity/SKILL.md).

### 12.3 Live-stack authenticity

The E2E tier MUST NOT use `route()`, `intercept()`, `msw`, `nock`, or any
request mocker; if a test uses one, it is reclassified as integration or
unit per repo testing rules.

### 12.4 Forbidden-pattern guard

The scenario linter additionally greps the agent-touching packages for
the forbidden patterns of §4.3 and fails CI on any match. This catches
regressions where a developer reaches for a regex router instead of
adding a scenario.

---

## Open Questions

| # | Question | Default for v1 |
|---|----------|----------------|
| Q1 | Hot reload trigger — SIGHUP, NATS control message, or polling? | SIGHUP |
| Q2 | Reuse `runtime.embedding_model` for routing or a dedicated smaller model? | Reuse |
| Q3 | Trace retention beyond 30 days — archive to object storage? | Out of scope for v1; revisit at first capacity pressure |
| Q4 | Per-scenario provider override exposed in v1? | Schema-supported via `model_preference`; no scenario uses non-`default` until 034/035/036 require it |
| Q5 | Fallback scenario implementation — empty string or a `clarify_intent` scenario shipped with the agent? | Empty string in default config; surfaces (Telegram) handle `unknown-intent` themselves until a `clarify_intent` scenario is needed |
