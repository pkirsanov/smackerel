# Spec 064 — Open-Ended Knowledge Agent

**Status:** in_progress (planning bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Owner Directive (2026-05-30):** Extend the spec 061 assistant with an
LLM-driven agent loop that can answer open-domain questions by calling
tools (internal retrieval, web search, calculator, unit convert) while
preserving the capture-as-fallback invariant and the provenance-gate
contract.

**Depends On:** spec 061 (conversational assistant), spec 044
(per-user bearer auth), spec 054 (notification intelligence handler).
**Amends:** spec 061 (provenance gate Source taxonomy + canonical
refusal taxonomy), spec 020 (egress allowlist), spec 022 (resilience
circuit breaker), spec 048 (scenario manifest ordering), spec 049
(assistant metrics).
**Unblocks:** future provider implementations (Brave, Tavily),
follow-up specs that may add deep-fetch, multi-turn tool memoisation,
or expose AgentAnswer artifacts to the retrieval index. Unblocks spec
066 (legacy keyword surface retirement) — open-knowledge is the
terminal scenario that absorbs the NL equivalents of retired slash
commands.

**Related (added 2026-05-31, analyst).** Spec 065 (Generic
Micro-Tools) introduces `location_normalize`, `unit_convert`,
`entity_resolve`, `calculator` as scenario-agnostic primitives. The
v1 tool set declared in this spec's `policySnapshot.v1ToolSet`
(`internal_retrieval`, `web_search`, `unit_convert`, `calculator`)
MUST be sourced from the spec 065 registry once 065 ships;
implementation in this spec should not fork private copies of these
tools. Spec 067 (Intent-Driven Policy Enforcement) supplies the CI
guard that enforces the `principleAlignment` block this spec already
declares per scenario. Spec 068 (Structured Intent Compiler) inserts
the upstream NL -> `CompiledIntent` stage that this open-knowledge
agent consumes; open-knowledge should reason over compiled action
class, side-effect class, source policy, and normalized slots rather
than raw text alone.

---

## 1. Problem Statement

The spec 061 assistant is **closed-set**: every Telegram message is
matched against a deterministic scenario manifest (retrieval, weather,
notifications), and any message that does not match a scenario falls
through to capture-as-fallback. Users who ask an open-domain
question — "what's the conversion factor between pounds and kilograms?",
"what's the current median 1-bedroom rent in Toronto?", "what did I
save about the lease, and how does that compare to today's listings?" —
get the capture-only response: their prompt is saved as an `Idea`
artifact, with no attempt at a grounded answer.

The owner wants the assistant to **also** be able to answer such
open-ended questions while preserving every existing product invariant:
capture is never lost, every answer carries verifiable provenance, and
the LLM is never trusted to attest its own grounding.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (chat owner)** | Single human operator interacting through any spec 061 transport adapter (Telegram v1). | Ask open-domain factual, computational, or current-events questions. Receive sourced answers or an explicit refusal-with-capture. Never lose a prompt. | All spec 061 transport permissions; subject to the per-user monthly budget cap. |
| **Open-Knowledge Scenario** (new) | Terminal scenario registered second-to-last in the spec 048 manifest, immediately before `capture_as_fallback`. | Match any non-empty message not claimed by a deterministic scenario; run the agent loop; return either a verified answer or a canonical refusal. | Reads `assistant.open_knowledge.*` config; calls the Tool Registry; persists `WebSnippet` and `AgentAnswer` artifacts. |
| **Agent Loop** (new) | Bounded planner ↔ tool ↔ observation loop driven by the LLM bridge in tool-use mode. | Plan tool calls, observe results, finalise an answer whose every citation hash-matches a recorded tool result. Stay inside the SST-supplied iteration, token, and USD budgets. | Calls the LLM bridge; calls allowlisted tools via the Registry; charges the user's monthly budget. |
| **Tool Registry** (new capability foundation) | Pluggable surface for tools; v1 tools = `internal_retrieval`, `web_search`, `unit_convert`, `calculator`. | Provide a uniform `Tool` contract; gate availability by operator allowlist; surface tool descriptions to the planner. | Read-only on the request side; each tool holds its own scoped credentials. |
| **Web Search Provider** (new external dependency) | One of `searxng`, `brave`, `tavily`. Operator picks exactly one via SST. | Return ranked `{URL, Title, Snippet, FetchedAt, ContentHash}` results for a query. | Outbound HTTP to one allowlisted host only; no deep-fetch in v1. |
| **Cite-Back Verifier** (new, mandatory) | Mechanical, non-LLM checker that every final-answer citation hash-matches a per-turn tool result. | Reject fabricated sources before the provenance gate sees them. | Pure function over the per-turn `ToolResultStore`. |
| **Operator** | Owns SST config, allowlists, budgets, and provider credentials. | Enable/disable the open-knowledge scenario; pick a provider; set budgets; allowlist tools. | Edits `config/smackerel.yaml` `assistant.open_knowledge.*`; injects `provider_api_key` at apply time. |

---

## 3. Outcome Contract

**Intent:** A user can ask the Smackerel assistant an open-domain
question and receive a grounded answer with citations, OR an explicit
refusal-with-capture; the user never loses the prompt and never
receives a fabricated citation.

**Success Signal:**
- For a curated v1 evaluation set of ≥20 open-domain questions, the
  agent returns a verified, sourced answer for ≥70% of questions
  whose ground-truth answer is reachable through allowlisted tools
  AND a canonical refusal-with-capture for 100% of questions whose
  answer is not reachable.
- Every grounded answer's citations pass the cite-back verifier:
  every cited `(SourceRef, ContentHash)` is present in the per-turn
  `ToolResultStore`. The `fabricated_source_total` counter increments
  zero times across the evaluation run.
- Every refusal path persists an `Idea` artifact for the original
  prompt, identically to today's capture-only path.

**Hard Constraints:**
1. **Capture-as-fallback is inviolable.** Every open-knowledge turn
   creates an `Idea` artifact, regardless of whether the agent
   returned a grounded answer, a refusal, or panicked.
2. **Cite-back verification is mandatory and non-configurable.** The
   verifier runs before the spec 061 provenance gate. A fabricated
   citation produces the canonical refusal `fabricated-source-blocked`.
3. **All runtime values come from SST.** Allowlist, budgets, provider
   selection, provider endpoint, provider credentials, LLM model id,
   and iteration cap are required keys in
   `assistant.open_knowledge.*`; missing or empty values fail-loud at
   load time (smackerel NO-DEFAULTS policy).
4. **Single egress host per deployment.** Only the configured
   provider host joins the spec 020 allowlist. No deep-fetch in v1.
5. **QF boundary preserved (P10).** The tool allowlist structurally
   excludes any tool that initiates financial action; loader enforces.
6. **Phone-screen-fit answers (P7).** Body ≤ 4 lines + Sources block
   ≤ 5 lines on a typical 390 px viewport.

**Failure Condition:** If the agent cannot return a verified answer
within the per-turn budget, OR a tool returns a hard failure, OR the
cite-back verifier rejects the LLM's citations, the turn produces a
canonical refusal-with-capture and increments the matching cause
counter.

---

## 4. Product Principle Alignment

| Principle | Alignment | Evidence |
|-----------|-----------|----------|
| **P1 Observe First, Ask Second** | The agent runs only after a user explicitly asks. Capture-as-fallback persists every prompt regardless of agent outcome. | Hard Constraint 1; SCN-064-A04..A08. |
| **P2 Vague In, Precise Out** | The planner system prompt biases toward `internal_retrieval` first; vague queries that touch the user's corpus are answered from their own knowledge before the web. | Design §"Agent Loop / Planner contract". |
| **P3 Knowledge Breathes** | `WebSnippet` and `AgentAnswer` are first-class graph artifacts with declared lifecycle (`emerging → active`); web snippets dedup by `(URL, ContentHash)` so the graph grows without bloat. | Design §"Artifact Persistence + Lifecycle". |
| **P4 Source-Qualified Processing** | Every `Source` carries a typed `Kind` plus provider/artifact/computation metadata. The provenance gate refuses entries with missing fields. | Design §"Provenance Gate Amendment". |
| **P5 One Graph, Many Views** | New artifact kinds extend the existing knowledge graph; no parallel store. Tool traces attach to `AgentAnswer`, not a sidecar index. | Design §"Artifact Persistence + Lifecycle". |
| **P7 Small, Frequent, Actionable Output** | Telegram answer body capped at 4 lines + 3 citations; longer citation lists collapse behind an inline-keyboard expansion. | Hard Constraint 6; UX packet §1, §8. |
| **P8 Trust Through Transparency** | Cite-back verifier is mechanical and mandatory; fabricated citations are observable via `fabricated_source_total`; every refusal cause has a distinct label. | Hard Constraint 2; Design §"Cite-Back Verifier". |
| **P10 QF Companion Boundary** | Tool allowlist structurally excludes financial-action tools; loader rejects unknown names; no QF tool is registered in v1. | Hard Constraint 5; Design §"Security / QF boundary". |

---

## 5. Functional Requirements (BDD Scenarios)

All scenarios are tagged with stable IDs `SCN-064-A01..A08` for
later `scenario-manifest.json` mapping.

### SCN-064-A01 — General-knowledge web answer with citations

**Given** the `open_knowledge` scenario is enabled and the operator has
allowlisted `internal_retrieval` and `web_search`, AND the configured
provider is reachable,
**When** the user sends "what is the boiling point of water in Denver?",
**Then** the agent loop calls `web_search`, the cite-back verifier
confirms every citation hash-matches a returned snippet, the
provenance gate accepts the answer, and the user receives the answer
body plus a `📎 Sources` block listing the registrable domain(s) used.
The user's prompt is also persisted as an `Idea` artifact with
`Status="answered"` referencing the `AgentAnswer`.

### SCN-064-A02 — Unit/math conversion via deterministic tool

**Given** the operator has allowlisted `unit_convert`,
**When** the user sends "72 °F in °C",
**Then** the agent loop calls `unit_convert`, the answer body is the
single-line result `72 °F = 22.2 °C`, the `Sources` block shows
`[convert] 72 °F → °C`, no web egress occurs, and the user's prompt
is persisted as an `Idea` artifact with `Status="answered"`.

### SCN-064-A03 — Hybrid internal-graph + web answer

**Given** the user's graph contains a bookmark artifact relevant to
the query AND the operator has allowlisted both `internal_retrieval`
and `web_search`,
**When** the user asks a hybrid question that requires both their
saved knowledge and current external data,
**Then** the agent loop calls `internal_retrieval` first, then
`web_search`, the verifier accepts both source kinds, and the
response's Sources block shows `(1 graph · 1 web)` with one
`[graph]` citation and one `[web]` citation.

### SCN-064-A04 — Refusal-with-capture on per-turn budget exhaustion

**Given** the per-turn token or USD budget is exhausted before the
planner emits a `final` step,
**When** the agent loop terminates with no cite-back-verified
citations available,
**Then** the response is the canonical refusal
`That needs more reasoning than today's budget allows.` followed by
`💡 Saved as an idea for later.`, no `📎 Sources` block appears, the
`open_knowledge_refusal_total{cause="budget-exhausted-turn"}` counter
increments by 1, and the user's prompt is persisted as an `Idea`
artifact with `Status="saved-as-idea-only"`.

### SCN-064-A05 — Refusal-with-capture on tool failure

**Given** a tool the planner chose to call returns a hard error AND
no other tool path yields cite-back-verified citations,
**When** the agent loop terminates without a verified answer,
**Then** the response is the canonical refusal
`A tool I'd need for that isn't reachable right now.` followed by
the capture acknowledgement, the
`open_knowledge_refusal_total{cause="provider-unavailable"}` counter
increments by 1, and the prompt is persisted as an `Idea` artifact
with `Status="saved-as-idea-only"`.

### SCN-064-A06 — Refusal on fabricated source

**Given** the planner emits a `final` step citing a URL or hash that
does not appear in the per-turn `ToolResultStore`,
**When** the cite-back verifier runs before the provenance gate,
**Then** the verifier rejects the answer with cause
`fabricated-source-blocked`, the response is the canonical refusal
`I couldn't verify the sources for that answer.` followed by the
capture acknowledgement, the `fabricated_source_total` counter
increments by 1, and the prompt is persisted as an `Idea` artifact
with `Status="saved-as-idea-only"`.

### SCN-064-A07 — Operator disables `web_search`

**Given** the operator has removed `web_search` from
`assistant.open_knowledge.tool_allowlist`,
**When** a user sends a question that cannot be grounded from internal
artifacts or deterministic tools,
**Then** the agent loop runs with the restricted tool set, terminates
without verified citations, and the response is the canonical refusal
`I can only answer that from your own knowledge, and I don't have it.`
followed by the capture acknowledgement. The prompt is persisted as
an `Idea` artifact with `Status="saved-as-idea-only"`.

### SCN-064-A08 — Per-user monthly budget exceeded

**Given** the user's accumulated monthly USD spend has reached
`assistant.open_knowledge.per_user_monthly_budget_usd`,
**When** the user sends any new open-knowledge query,
**Then** the agent loop refuses BEFORE the first tool call with cause
`budget-exhausted-monthly`, the response is the canonical refusal
`That needs more reasoning than today's budget allows.` followed by
the capture acknowledgement, the
`open_knowledge_budget_exhausted_total{scope="month"}` counter
increments by 1, and the prompt is persisted as an `Idea` artifact
with `Status="saved-as-idea-only"`.

---

## 6. UI Wireframes

The Telegram answer surface is the only v1 user-facing rendering.
Wireframes and the full refusal taxonomy live in the UX packet for
spec 064 and are referenced from the design document; the shape
contract is:

- Body first, blank line, `📎 Sources (n graph · n web)` header, then
  numbered `[graph]` / `[web]` / `[convert]` / `[calc]` citations.
- Refusal shell: single-line refusal body, blank line,
  `💡 Saved as an idea for later.`
- `📎 Sources` block XOR `💡 Saved as an idea` line — never both,
  never neither (Hard Constraint 1 + Hard Constraint 2).

---

## 7. Out of Scope (v1)

- Deep-fetching arbitrary URLs returned by the provider (egress
  surface stays at one host).
- Multi-turn tool-result memoisation across turns.
- Surfacing `AgentAnswer` artifacts to the `internal_retrieval` index
  (avoids self-reinforcement loops).
- User-facing tool-selection UI.
- Additional provider implementations beyond `searxng` (Brave/Tavily
  return `ErrProviderNotConfigured` until follow-up scopes wire them).

---

## 8. Cross-Spec Amendments

| Spec | Amendment |
|------|-----------|
| 061  | Source taxonomy extended (`SourceWeb`, `SourceToolComputation`); canonical refusal taxonomy gains six new causes; cite-back verifier is gate-level for scenarios that opt in. Backward compatible. |
| 020  | Egress allowlist gains the configured provider host when `open_knowledge.enabled: true`. No other hosts in v1. |
| 022  | New circuit-breaker instance for the web_search provider (5 failures / 60 s open, 30 s half-open). |
| 048  | Scenario manifest places `open_knowledge` second-to-last, immediately before `capture_as_fallback`. |
| 049  | New metrics: `open_knowledge_tool_calls_total`, `open_knowledge_iterations_per_query`, `open_knowledge_budget_usd_total`, `open_knowledge_budget_exhausted_total`, `fabricated_source_total`, `open_knowledge_refusal_total`, `open_knowledge_provider_request_seconds`. |
