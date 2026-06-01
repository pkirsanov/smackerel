# Design — Open-Ended Knowledge Agent (Spec 064)

> Amends spec 061 (provenance gate Source taxonomy + canonical refusal
> taxonomy), spec 020 (egress allowlist), spec 022 (resilience
> circuit breaker), spec 048 (scenario manifest ordering), and spec 049
> (assistant metrics). New capability foundation: a Tool Registry plus a
> bounded agent loop with mandatory cite-back verification.

> **Design Successor Note (2026-05-31).** This design remains the
> authority for the `open_knowledge` terminal scenario, web-search provider
> boundary, budget tracking, cite-back verification, web snippet and agent
> answer artifacts, and capture-as-fallback behavior. Generic tools named
> here (`unit_convert`, `calculator`, plus `entity_resolve` and
> `location_normalize`) are now owned as cross-scenario primitives by
> [spec 065](../065-generic-micro-tools/design.md). Incoming open-ended
> user turns should arrive with a `CompiledIntent` from
> [spec 068](../068-structured-intent-compiler/design.md) before this
> scenario plans tool use.

## Design Brief

**Current State.** Spec 061's assistant pipeline routes every Telegram
query through a deterministic scenario manifest. Each scenario either
returns a grounded answer (sources from the internal graph) or refuses
through the provenance gate. There is no path for open-ended questions
that fall outside the scenario catalog; today they fall through to
capture-as-fallback only.

**Target State.** Add a new terminal scenario `open_knowledge` placed
LAST in the manifest (just before `capture_as_fallback`). It runs a
bounded agent loop over a pluggable Tool Registry
(`internal_retrieval`, `web_search`, `unit_convert`, `calculator`).
Every final answer is gated by the existing 061 provenance gate,
extended with a mechanical cite-back verifier: every citation must
hash-match a tool result captured in the same turn. Web snippets the
agent grounds on persist as first-class graph artifacts (P3 —
Knowledge Breathes). Capture-as-fallback remains inviolable; the
prompt is always persisted as an `Idea` artifact regardless of agent
outcome.

**Patterns to Follow.**
- Provenance gate enforcement: `internal/assistant/provenance/`
- Scenario manifest routing: `internal/assistant/facade.go` + skills
- Capability foundation split: `.github/skills/bubbles-capability-foundation-design/SKILL.md`
- LLM bridge contract: `ml/app/` (existing Python sidecar boundary)
- SST config: `config/smackerel.yaml` + `internal/config/` loaders
  (smackerel NO-DEFAULTS policy)
- Egress allowlist: spec 020

**Patterns to Avoid.**
- Single-provider hardcoding (use a Provider interface from day one).
- Unbounded LLM loops (bound iterations and per-turn token + USD).
- Implicit fallback chains (operator picks exactly one provider; if it
  fails, the circuit breaker trips and the turn refuses).
- Persisting tool traces in a parallel store (P5 — attach to the
  `AgentAnswer` artifact).
- Treating web snippets as ephemeral cache (P3 — they enrich the
  graph permanently).

**Resolved Decisions.**
- Capability foundation: `Tool` interface + `Registry`; v1 tools =
  `internal_retrieval`, `web_search`, `unit_convert`, `calculator`.
- Web search provider interface with three v1 implementations,
  operator picks exactly one: SearxNG (recommended for local-first),
  Brave, Tavily.
- Cite-back verification is a hard product invariant, NOT operator
  configurable.
- Web snippets persist as `Kind=WebSnippet` graph artifacts (no TTL).
- Agent answer persists as `Kind=AgentAnswer` derived artifact.
- Capture-as-fallback ALWAYS creates the `Idea` artifact, independent
  of agent outcome; the `Idea.Status` field reflects "answered" vs
  "saved-as-idea-only".
- v1 does NOT deep-fetch URLs; we ground on provider-returned snippets
  only to keep the egress surface to one host.

---

## Architecture Overview

```
Telegram message
   ↓
Scenario router (spec 048)
   ↓ (no scenario matched OR scenario delegates)
open_knowledge scenario   ← NEW
   ↓
AgentLoop
   ├── Planner (LLM, tool-use mode)
   ├── ToolRegistry
   │     ├── internal_retrieval  (wraps graph search)
   │     ├── web_search          (Provider interface)
   │     │     └─ SearxNG | Brave | Tavily  (operator picks one)
   │     ├── unit_convert        (deterministic, no egress)
   │     └── calculator          (deterministic, sandboxed)
   ├── BudgetTracker (tokens + USD, per-turn + per-user-monthly)
   └── ToolResultStore (per-turn, feeds cite-back verifier)
   ↓
Source assembler → CiteBackVerifier (hash-match every citation)
   ↓
Provenance gate (spec 061, amended) — accept | refuse
   ↓
Artifact persistence
   ├── WebSnippet artifacts        (each grounded snippet, P3 lifecycle)
   ├── AgentAnswer artifact        (derived, references inputs + trace)
   └── Idea artifact               (capture-as-fallback, ALWAYS created)
   ↓
Telegram response (per UX packet shapes)
```

Trust boundaries:
- The LLM is an untrusted planner; its outputs are validated
  mechanically (cite-back, schema, budget).
- Web snippets are untrusted content; they cannot re-issue tool calls
  (prompt boundary discipline at the LLM bridge).
- API keys live in SST and are never logged.

---

## Capability Foundation — Tool Registry

This satisfies DE4 (capability-foundation-design). Triggers: a reusable
capability with ≥ 2 concrete implementations
(`internal_retrieval`, `web_search`, `unit_convert`, `calculator`) and
a provider/adapter sub-axis on `web_search`.

### Tool interface (Go)

```go
package openknowledge

type Tool interface {
    Name() string
    Description() string
    ParamsSchema() json.RawMessage
    Execute(ctx context.Context, params json.RawMessage) (*ToolResult, error)
}

type ToolResult struct {
    Snippets    []Snippet
    Sources     []Source
    Computation *Computation
    Error       *ToolError
}

type Snippet struct {
    Text        string
    ContentHash string   // sha256 of canonicalized Text
    SourceRef   string   // index into Sources or artifact ID
}

type Source struct {
    Kind        SourceKind   // SourceArtifact | SourceWeb | SourceToolComputation
    Artifact    *ArtifactRef
    Web         *WebSource
    Computation *ComputationSource
}
```

### Registry

```go
type Registry interface {
    Register(t Tool) error
    Lookup(name string) (Tool, error)
    Enabled() []Tool   // operator-allowlisted subset, deterministic order
}
```

Construction reads `assistant.open_knowledge.tool_allowlist` from SST.
Any tool whose `Name()` is not in the allowlist is excluded from
`Lookup()` and `Enabled()`. A nil or empty allowlist denies every
tool — there is no implicit "allow all" mode.

Typed sentinels:
- `ErrUnknownTool` — name was never `Register`ed.
- `ErrDuplicateTool` — `Register` called twice for the same name.
- `ErrToolNotAllowed` — tool is `Register`ed but excluded by allowlist.

### Variation Axes

1. **Tool intent:** retrieval (`internal_retrieval`, `web_search`) vs
   deterministic computation (`unit_convert`, `calculator`).
2. **Egress profile:** zero-egress vs allowlisted-egress.
3. **Provider sub-axis (web_search only):** SearxNG vs Brave vs Tavily.

### Single-Implementation Justification

N/A — every axis has ≥ 2 implementations in v1.

---

## Agent Loop

### Planner contract

System prompt instructs the LLM to:
1. **Prefer `internal_retrieval` FIRST** when the query references
   entities, topics, or timeframes likely in the user's graph (P2 +
   competitive edge: the user's own corpus is the primary surface).
2. Call deterministic tools (`calculator`, `unit_convert`) whenever a
   numeric transformation is needed — never reason about arithmetic
   in prose.
3. Call `web_search` only when prior tool calls did not yield
   sufficient grounding.
4. Finalise with an answer where EVERY factual claim cites a
   `ToolResult.Source` from this turn.

### Loop structure

```
for iter := 0; iter < cfg.MaxIterations; iter++ {
    planStep := llm.Plan(history, registry.EnabledDescriptions())
    if planStep.Final != nil {
        return assemble(planStep.Final, toolResults)
    }
    if !budget.Allow(planStep.ToolCall) {
        return refuse("budget-exhausted")
    }
    tool, err := registry.Lookup(planStep.ToolCall.Name)
    if err != nil {
        return refuse("tool-not-allowed-or-unknown")
    }
    result, execErr := tool.Execute(ctx, planStep.ToolCall.Params)
    budget.Charge(result.TokensUsed, result.UsdCost)
    toolResults.Record(planStep.ToolCall, result, execErr)
    history.Append(planStep, result, execErr)
}
return refuse("iteration-cap-reached")
```

Hard invariants:
- `cfg.MaxIterations` comes from SST; no default.
- Per-turn token + USD budgets come from SST; charged after each tool
  call AND after each LLM planner step.
- Per-user monthly USD budget checked BEFORE the loop starts.
- A tool returning a structured `ToolError` does NOT terminate the
  loop; the planner sees the error and may try another path.

### LLM bridge extension

Go-side client location: `internal/assistant/openknowledge/llm/`
(scope-local to spec 064). Spec 061 design §10 + §11.3 forbid a
top-level `internal/assistant/llm/` package because it would
re-introduce a parallel spec 037 substrate; the open-knowledge agent's
LLM bridge therefore lives under the `openknowledge/` capability
subtree and is exercised by `TestLLMClient_*` plus the shared
parity fixture at
`internal/assistant/openknowledge/llm/testdata/chat_fixture.json`.

The Python ML sidecar (`ml/app/`) exposes a `/llm/plan` endpoint that
accepts:
```
{
  "model_id":       string,
  "messages":       [...],
  "tools":          [{"name", "description", "params_schema"}, ...],
  "max_tokens":     int,
  "turn_budget_usd": float
}
```
and returns:
```
{
  "step": {
    "tool_call": {"name", "params"} | null,
    "final":     {"text", "citations": [...]} | null
  },
  "tokens_used": int,
  "usd_cost":    float
}
```

Inline in this design (no separate ML-contract spec). The Go core
enforces budget; the sidecar reports usage but does NOT self-throttle.

Prompt boundary discipline: tool result text passed back to the
planner is wrapped in a `<tool_output id="...">` envelope; the
planner system prompt forbids treating envelope contents as
instructions.

### Failure modes

| Cause                       | Outcome                              |
|-----------------------------|--------------------------------------|
| Tool execute soft error     | Loop continues; planner sees error   |
| Tool execute hard panic     | Refuse + capture, error logged       |
| Budget exhausted pre-loop   | Refuse `budget-exhausted-monthly`    |
| Budget exhausted mid-loop   | If verified citations exist → finalise partial answer; else refuse `budget-exhausted-turn` |
| Iteration cap reached       | Refuse `iteration-cap-reached`       |
| LLM error                   | No retry within turn; refuse `llm-error` |
| Provider circuit open       | `web_search` returns soft error; loop continues; if all paths fail → refuse `provider-unavailable` |

All refusal paths trigger capture-as-fallback (`Idea` artifact
persisted).

---

## Source Assembly + Cite-Back Verifier

### Assembler

After the planner emits a `final` step with `citations: [{SourceRef,
ContentHash}]`, the assembler constructs the response `Sources[]` by
looking up each citation in the per-turn `ToolResultStore`.

### Verifier (mandatory, mechanical, non-LLM)

```go
func Verify(final FinalStep, store *ToolResultStore) error {
    for _, c := range final.Citations {
        snip := store.FindSnippet(c.SourceRef, c.ContentHash)
        if snip == nil {
            metrics.FabricatedSource.Inc()
            return ErrFabricatedSource{Ref: c.SourceRef, Hash: c.ContentHash}
        }
    }
    return nil
}
```

`FindSnippet` matches on `(SourceRef, ContentHash)` exactly. Any
citation whose hash does not appear in any recorded `ToolResult` is a
fabricated source — the gate refuses with `fabricated-source-blocked`
and the prompt is captured.

The verifier runs BEFORE the provenance gate. Cite-back failure is a
distinct refusal cause for observability (it is much more serious than
budget exhaustion).

---

## Provenance Gate Amendment (Spec 061)

`internal/assistant/provenance/`:

### Source taxonomy extension

```go
type SourceKind int
const (
    SourceArtifact SourceKind = iota + 1
    SourceWeb                            // NEW (spec 064)
    SourceToolComputation                // NEW (spec 064)
)
```

### Gate.Enforce semantics (amended)

Passes when ALL of:
- `len(Sources) > 0`
- Every `Source.Kind` is in
  `{SourceArtifact, SourceWeb, SourceToolComputation}`
- For `SourceWeb`: `URL != "" && ContentHash != "" && Provider != ""`
- For `SourceToolComputation`: `Tool != "" && Input != nil && Output != nil`
- Cite-back verifier returned nil

### CanonicalRefusalBody taxonomy (extended)

Adds:
- `fabricated-source-blocked`
- `budget-exhausted-monthly`
- `budget-exhausted-turn`
- `iteration-cap-reached`
- `llm-error`
- `provider-unavailable`
- `no-grounding-found`

### Backward compatibility

Existing 061 scenarios (retrieval, weather, notifications) emit only
`SourceArtifact`. The taxonomy extension is additive; their flows are
unchanged. The cite-back verifier is opt-in per scenario;
`open_knowledge` opts in.

---

## Artifact Persistence + Lifecycle (P3)

### WebSnippet artifact

```
Kind:        WebSnippet
Identity:    sha256(ContentHash + URL)        // dedup across turns
Fields:      URL, Title, Provider, FetchedAt, ContentHash, Snippet,
             SourceQuery
Lifecycle:   emerging → active (no TTL; ages via standard graph weighting)
Visibility:  user-visible (appears in graph)
```

### AgentAnswer artifact

```
Kind:        AgentAnswer
Identity:    sha256(turn_id)
Fields:      UserPrompt, AnswerText, Citations[], ToolTraceRef,
             Provider, ModelID, TokensUsed, UsdCost, IterationCount
References:  WebSnippet artifacts cited + internal artifacts cited +
             Computation source entries
Lifecycle:   active, marked LowPriority for graph weighting
Visibility:  user-visible
```

### ToolTrace (attached, not standalone)

```
Stored as:   AgentAnswer.ToolTraceRef → JSON blob in artifact store
Contents:    [{iter, tool, params (redacted), result_summary, tokens, usd}, ...]
Visibility:  operator-only
```

### Idea artifact (capture-as-fallback, ALWAYS)

```
Kind:        Idea
Created:     ALWAYS, regardless of agent outcome
Identity:    turn-scoped
Status:      "answered" if AgentAnswer succeeded
             "saved-as-idea-only" if refused
References:  AgentAnswer (when present)
```

Inviolable: the user's prompt is ALWAYS captured.

---

## Routing Rule — `open_knowledge` Scenario Placement (Spec 048)

Scenario manifest order:
```
1. (existing scenarios from 061 / 048 ...)
N-1. open_knowledge       ← NEW, second-to-last
N.   capture_as_fallback  ← last
```

Match rule: matches any non-empty user message that no prior scenario
matched. Behaviour:
- Run the agent loop.
- If the agent returns a verified answer → response includes the
  answer + `Idea` artifact with `Status="answered"`.
- If the agent refuses → response is the refusal + `Idea` with
  `Status="saved-as-idea-only"`.

`capture_as_fallback` remains as the no-op safety net: if
`assistant.open_knowledge.enabled: false`, the manifest skips
`open_knowledge` and falls through to pure capture.

---

## SST Config Block (NO-DEFAULTS)

Per `.github/skills/smackerel-no-defaults/SKILL.md`, every value below
MUST be explicitly set in `config/smackerel.yaml`; loaders MUST
fail-loud on missing or empty values.

```yaml
assistant:
  open_knowledge:
    enabled: true                          # required
    provider: searxng                      # required: searxng | brave | tavily
    provider_endpoint: ""                  # required: provider URL
    provider_api_key: ""                   # required (empty allowed for
                                           # searxng without auth; fail-loud
                                           # for brave/tavily)
    llm_model_id: ""                       # required
    max_iterations: 0                      # required, > 0
    per_query_token_budget: 0              # required, > 0
    per_query_usd_budget: 0.0              # required, >= 0
    monthly_budget_usd: 0.0                # required, >= 0 (must be explicit)
    per_user_monthly_budget_usd: 0.0       # required, >= 0
    tool_allowlist: []                     # required, non-empty
    web_snippet_cache_enabled: false       # required
```

Loader rules:
- `enabled: true` requires every other field to be valid (non-empty
  where applicable, positive where applicable, allowlist non-empty).
- `enabled: false` skips deep validation; scenario not registered.
- Per-provider validation: `brave` and `tavily` require non-empty
  `provider_api_key`; `searxng` allows empty.
- `tool_allowlist` MUST NOT contain any name not registered in code.
- `tool_allowlist` MUST NOT contain financial-action tools (P10 —
  structural guard).

### `cite_back_required` is NOT a config field

Cite-back verification is a hard product invariant enforced in code.
Operators cannot disable it. It lives outside the config block by
design.

---

## Security

### Egress allowlist (cross-spec to spec 020)

- The configured `provider_endpoint` host joins the allowlist when
  `open_knowledge.enabled: true`.
- v1: NO deep-fetch beyond provider responses.
- Any future deep-fetch must allowlist every fetched host or refuse.

### Prompt-injection mitigations

- Tool output wrapped in `<tool_output id="...">` envelopes; planner
  system prompt declares envelope contents as untrusted data.
- Provider snippets are HTML-stripped and length-capped before being
  passed to the planner.
- Tool call names in planner output are validated against the
  registry; unknown names are a hard refusal.

### Secret handling

- `provider_api_key` and `llm_model_id` credentials come from SST.
- Logs redact API keys; full prompts only at DEBUG with PII stripped
  per existing policy; provider response bodies log metadata only.

### QF boundary (P10)

The `tool_allowlist` structurally cannot include any tool that
initiates trade approval, mandate change, execution, or financial
advice. v1 registers no such tool; the loader is the long-term
guard.

---

## Observability (cross-spec to spec 049)

### Metrics

| Metric                                          | Labels             | Type      |
|-------------------------------------------------|--------------------|-----------|
| `open_knowledge_tool_calls_total`               | tool, outcome      | counter   |
| `open_knowledge_iterations_per_query`           | —                  | histogram |
| `open_knowledge_budget_usd_total`               | user               | counter   |
| `open_knowledge_budget_exhausted_total`         | scope (turn/month) | counter   |
| `fabricated_source_total`                       | —                  | counter   |
| `open_knowledge_refusal_total`                  | cause              | counter   |
| `open_knowledge_provider_request_seconds`       | provider, outcome  | histogram |

### Tracing

- Each tool round-trip is a span with redacted prompt + result
  summary.
- Each turn rolls up to a parent span with turn_id, user_id (hashed),
  `scenario=open_knowledge`.

---

## Failure Modes + Circuit Breaker (cross-spec to spec 022)

### Provider circuit breaker

- 5 failures in 60 s opens the breaker.
- Half-open after 30 s; one probe request.
- Open-state `web_search` calls return soft error
  `provider-unavailable`; planner sees this and may finalise from
  `internal_retrieval` results or refuse.

### LLM errors

- No in-turn retry. One transient LLM failure terminates the turn with
  `llm-error` + capture.
- Operator-visible metric increments; alerts wired per spec 049.

### Budget exhaustion mid-loop

- If cite-back-verified citations already exist from prior iterations,
  finalise an answer from what we have.
- Otherwise refuse with `budget-exhausted-turn`.

---

## Testing & Validation Strategy

| Behaviour                                      | Test type   | Notes |
|-----------------------------------------------|-------------|-------|
| Tool interface contract                       | unit        | per-tool table tests |
| Registry allowlist enforcement                | unit        | reject unknown, reject empty, deterministic Enabled() |
| AgentLoop iteration cap                       | unit        | mock planner returns infinite tool calls |
| Cite-back verifier hash matching              | unit        | adversarial: tamper one byte → reject |
| Provenance gate accepts new Source kinds      | unit        | extends 061 gate tests |
| Provider interface conformance                | unit        | SearxNG/Brave/Tavily share a contract test suite |
| Budget exhaustion paths                       | unit        | pre-loop, mid-loop, with/without prior citations |
| Capture-as-fallback always creates Idea       | integration | success and refusal paths |
| WebSnippet artifact dedup                     | integration | repeated query → same artifact ID |
| AgentAnswer references resolve                | integration | walk graph from AgentAnswer to sources |
| Egress allowlist enforcement                  | integration | non-allowlisted host attempt → blocked |
| Circuit breaker opens on provider failure     | integration | spec 022 harness |
| End-to-end Telegram open-ended question       | e2e-api     | live stack, real (test) provider |
| Cite-back fabrication rejection (adversarial) | e2e-api     | mock LLM returns fabricated citation → refusal + metric |

All live-stack tests run against the disposable test stack (test
environment isolation policy).

---

## Cross-Spec Amendments Summary

| Spec | Amendment |
|------|-----------|
| 061  | Source taxonomy extended; canonical refusal taxonomy extended; cite-back verifier gate-level for opt-in scenarios. Backward compatible. |
| 020  | Egress allowlist gains the configured provider host when `open_knowledge.enabled: true`. |
| 022  | New circuit-breaker instance for `web_search` provider. |
| 048  | Scenario manifest places `open_knowledge` second-to-last, before `capture_as_fallback`. |
| 049  | New metrics surface wired into the assistant dashboard. |
