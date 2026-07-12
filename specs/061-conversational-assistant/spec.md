# Spec 061 — Conversational Assistant (Transport-Agnostic)

**Status:** done (certified per state.json)
**Workflow Mode:** `full-delivery` (historical; certified done)
**Owner Directives:**
- 2026-05-28 — extend Smackerel from capture-only into a conversational
  assistant while preserving capture-as-fallback.
- 2026-05-28 (revision) — the assistant capability MUST be
  **transport-agnostic**. Telegram is the v1 reference adapter; WhatsApp,
  web chat, mobile in-app, and future transports MUST be able to consume
  the same backend capability via a transport-specific adapter only.
  Per `bubbles-capability-foundation-design`, the assistant is the
  foundation; transports are pluggable adapters.

**Depends On:** spec 044 (per-user bearer auth), spec 060 (scope claims),
existing `internal/telegram/bot.go` (v1 reference adapter integration
site), existing `ml/` sidecar (LLM bridge).
**Unblocks:** future per-transport adapter specs (WhatsApp, web chat,
mobile in-app), future skill specs (email, etc.).

**Architectural Position (added 2026-05-31, analyst).** This spec owns
the **closed-set scenario router + transport adapter + capability
facade**. It does NOT own:

- The **open-domain agent loop** (any NL request that no deterministic
  scenario claims) — owned by **spec 064 (Open-Ended Knowledge
  Agent)**, registered as a terminal scenario immediately before
  capture-as-fallback.
- The **structured-intent compiler** (LLM transforms every user NL
  turn into a schema-bound `CompiledIntent` before routing) — owned by
  **spec 068 (Structured Intent Compiler)**. Spec 068 amends this
  facade so the pipeline becomes: raw turn -> compiled intent -> route
  -> tool/action -> synthesized response.
- **Generic cross-scenario micro-tools** (`location_normalize`,
  `unit_convert`, `entity_resolve`, `calculator`) — owned by **spec
  065 (Generic Micro-Tools)**. Scenarios authored after 065 ships
  MUST compose these tools rather than embed normalization quirks in
  their `system_prompt`.
- **Legacy slash command retirement** (`/find`, `/rate`, `/list`,
  `/meal_plan`, etc. → NL-driven equivalents) — owned by **spec 066
  (Legacy Keyword Surface Retirement)**. After 066 ships, the
  Telegram BotCommands menu collapses to the operational set
  (`/help /status /reset /digest /recent /done`) plus the spec 061
  intent-aware shortcuts (`/ask /weather /remind`).
- **CI guards for the intent-driven mandate** (scenario-prompt size
  cap, `principleAlignment`-required, broadened NO-DEFAULTS sweep,
  forbidden-keyword-routing extended beyond the agent path) — owned
  by **spec 067 (Intent-Driven Policy Enforcement)**.
- The **HTTP transport adapter** that exposes `Facade.Handle` for
  end-to-end tests and non-Telegram frontends (web chat, Android,
  WhatsApp bridge) — owned by **spec 069 (Assistant HTTP
  Transport)**. The capability ⇄ transport contract this spec
  defined (`contracts.TransportAdapter`) is unchanged; 069 simply
  adds the second concrete adapter next to Telegram.

Spec 061 status remains `done`: it shipped what it specified. The
above successors *amend* its surface; they do not invalidate it.

---

## 1. Problem Statement

Today Smackerel's only conversational surface is the Telegram bot, and it
is **capture-only** for plain text. In
`internal/telegram/bot.go::handleMessage`, every non-command, non-special,
non-media plain-text message is routed to `handleTextCapture` →
`POST /api/capture` and saved as an `idea` artifact. There is no path for
a user — on Telegram or any other transport — to ask a question ("what's
the weather in Seattle?", "what did I save about X last week?") or to
request an action ("remind me at 6pm").

The owner wants Smackerel to **also** be a conversational assistant, and
wants that assistant to be reachable from any present or future frontend
— Telegram (v1), WhatsApp, web chat, mobile in-app — without forking the
backend logic per transport. The existing capture path is product-critical
(Principle 1: Observe First, Ask Second — passive ingestion is the
lifeblood of the knowledge graph) and MUST NOT regress on any transport.
Therefore the new behavior must:

1. Live in a **transport-agnostic capability layer** (intent router,
   skill registry, skills, conversational state, source attribution).
2. Be reachable through a thin **transport adapter** per frontend, which
   translates between transport-native messages and a canonical
   assistant message contract.
3. Layer on top of capture: classify intent, route to skills when
   confident, fall back to capture otherwise — identically on every
   transport.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (chat owner)** | Single human operator interacting via any supported frontend (Telegram in v1; WhatsApp, web chat, mobile in-app in future versions), authenticated per-transport against the same Smackerel user identity. | Capture passively (legacy); ask questions; trigger actions; trust the assistant to default to capture when intent is unclear. Get the same semantics regardless of which frontend they use. | All assistant skills enabled for their `user_id`; subject to per-skill scope claims (spec 060). |
| **Transport Adapter** (per frontend) | Thin layer that owns one transport (Telegram, WhatsApp, web SSE, mobile in-app, etc.). Translates inbound transport messages to a canonical `AssistantMessage` and renders the canonical `AssistantResponse` using transport-native widgets (Telegram inline keyboards, WhatsApp quick replies, web SSE+button cards, mobile native cards). v1 ships ONE reference adapter: Telegram (`internal/telegram/assistant_adapter/`). | Faithfully translate between transport-native and canonical contracts without altering semantics. Map status tokens, sources blocks, confirm cards, disambiguation prompts to the transport's native conventions. Authenticate the inbound user against Smackerel identity (e.g. spec 044 chat→user mapping for Telegram). | Reads/writes its own transport's API surface only; never writes to the knowledge graph directly; calls the capability layer via the in-process `Assistant` interface. |
| **Intent Router** (new, capability layer, transport-agnostic) | First-pass classifier that decides: is this a question, an action, or a capture? Identical behavior across transports. | Route to the right skill with high precision; defer to capture on uncertainty. | Reads canonical `AssistantMessage` text + recent conversational context; writes to skill registry; never writes to the knowledge graph directly. |
| **Skill Registry** (new, capability foundation, transport-agnostic) | Pluggable surface for assistant skills. v1 skills register handlers; new skills can be added without modifying the router or any adapter. | Provide a uniform contract: `intent → handler → canonical response + provenance`. | Read-only on the request side; each handler holds its own scoped credentials. |
| **LLM Bridge** (reused from `ml/` sidecar) | Existing FastAPI sidecar that fronts Ollama/external LLM providers. | Provide intent classification + answer synthesis with source attribution. | Already wired; no new infrastructure access. |
| **Operator** (deployer / SRE) | Owns the self-hosted deploy and rotates credentials. | Enable/disable the capability and individual skills via SST config; enable/disable individual transports via SST config; rotate API keys via secret store; observe per-skill AND per-transport metrics. | Edits `config/smackerel.yaml` `assistant.*` and `assistant.transports.*` blocks; injects per-skill and per-transport secrets at apply time. |

**Known / planned transport adapters.**

| Adapter | Status | Notes |
|---------|--------|-------|
| Telegram | v1 (reference adapter, ships with this spec) | Single bot identity; existing chat→user_id mapping from spec 044 Scope 03 is the only auth surface for this transport. |
| WhatsApp | future | Likely WhatsApp Business API + webhook adapter. |
| Web chat | future | Likely SSE+POST adapter served by core HTTP server. |
| Mobile in-app | future | Likely native client calling the same web-chat adapter or a dedicated mobile adapter. |

---

## 3. Outcome Contract

**Intent:** A user can ask the Smackerel Telegram bot a question or request
an action and get a useful, source-attributed answer — OR, when intent is
ambiguous, have the message captured exactly as today. The bot is
*additive*: it never silently swallows what would have been a capture.

**Success Signal:**
- For a curated v1 evaluation set of ≥30 messages (mix of questions,
  actions, captures, ambiguous), the bot's routing matches the labeled
  intent for ≥85% of cases AND the fallback rate to capture for the
  "capture" subset is 100% (zero false-positive skill triggers on plain
  notes).
- Every synthesized answer (retrieval Q&A, email summarization, weather)
  carries source attribution: artifact IDs for graph-sourced answers,
  external provider name + timestamp for tool-sourced answers (Principle
  8).

**Hard Constraints:**
1. **Capture-as-fallback is inviolable on every transport.** Any message
   that fails intent classification with confidence ≥
   `assistant.intent.min_confidence` (SST value, no default) MUST be
   routed to the existing capture path with no user-visible difference,
   regardless of which frontend the user is on.
2. **No silent skill execution.** Skills that have side effects
   (notifications scheduled, emails sent) MUST get explicit user
   confirmation before execution. The confirmation is rendered using
   each transport's native confirmation widget (Telegram inline
   keyboard, WhatsApp quick reply, web button card, etc.) but the
   underlying semantics are identical.
3. **Source attribution required.** No answer may be returned on any
   transport without either (a) artifact-ID citations to the knowledge
   graph or (b) named external provider + retrieval timestamp.
4. **Transport-agnostic capability surface.** The intent router, skill
   registry, skills, conversational state, and provenance enforcement
   live in a transport-agnostic capability layer. Adapters MUST NOT
   embed business logic; they only translate between transport-native
   messages and the canonical `AssistantMessage` / `AssistantResponse`
   contracts.
5. **Per-transport identity is owned by the adapter.** Each adapter is
   responsible for resolving the transport-specific identity (e.g. the
   existing Telegram chat→user_id mapping from spec 044 Scope 03 for
   the Telegram adapter) into a Smackerel `user_id`. The capability
   layer only sees `user_id`.
6. **Reuse the Spec 037 agent runtime; do NOT fork it.** Spec 037
   (`specs/037-llm-agent-tools/`, status `done`) already shipped the
   intent router (similarity + explicit-id override + confidence floor
   + fallback), the executor (loop, retry budget, tool allowlist,
   schema validation), the YAML-driven scenario registry with
   hot-reload, the NATS LLM driver, the PostgreSQL tracer, the REST
   `POST /v1/agent/invoke` handler, and a callable
   `internal/telegram/agent_bridge.go` (`AgentBridge.Handle`) that
   renders results via `internal/agent/userreply` honoring a 4-line
   Telegram budget. This spec's capability layer MUST consume that
   substrate. Equivalence:
   - "Skill" ≡ a Spec 037 `Scenario` (YAML in `AGENT_SCENARIO_DIR`)
     whose `intent_examples` and tool allowlist target a user-facing
     intent rather than a backend extraction job.
   - "Intent Router" ≡ the existing `agent.Router` (no new router).
   - "Skill Registry" ≡ the existing scenario loader + hot-reload.
   - The canonical `AssistantResponse` is a thin projection of
     `agent.InvocationResult` (Outcome, Final, ToolCalls, trace ref)
     PLUS the net-new fields this spec adds: structured
     `sources[]`, `confirmCard`, `disambiguationPrompt`,
     `errorCause`, `captureRoute`, `status` token.
   - Trace persistence and CLI (`smackerel agent traces|scenarios|
     tools`) are reused as-is; no parallel trace store.

   A parallel `internal/assistant/` runtime that re-implements router,
   registry, executor, or tracer is a **blocking failure** (foundation
   duplication). New code introduced by this spec is limited to:
   (a) thin facade types (`AssistantMessage` / `AssistantResponse`)
       and the `TransportAdapter` interface,
   (b) the provenance-enforcement gate (BS-007) wrapping
       `Runner.Invoke` results,
   (c) the three v1 user-facing scenarios (retrieval, weather,
       notifications) added as YAML scenarios + Go tool handlers in
       the existing tool registry,
   (d) the confirm/disambiguation/context primitives if they are
       genuinely absent from Spec 037 (verify per scope),
   (e) per-transport adapter packages (Telegram v1 in
       `internal/telegram/assistant_adapter/`).

**Failure Condition:**
- Capture regression on ANY transport: any plain-note message saved as
  something other than `idea` artifact during the v1 rollout window.
  This is a P0 rollback trigger.
- Hallucinated answer: an answer returned without attached provenance,
  or with provenance pointing to an artifact that does not exist.
- Adapter business-logic leak: an adapter that classifies intent,
  invokes a skill, mutates graph state, or otherwise reaches around
  the canonical capability interface. Adapters are translation-only.
- Substrate duplication: any new code in this spec that re-implements
  Spec 037's router, executor, scenario registry, tracer, or LLM
  driver instead of consuming them. This is a P0 architectural
  rollback trigger because it forks the agent foundation and breaks
  the capability-foundation doctrine.

---

## 3.1 Substrate Reuse — Spec 037 LLM Agent Runtime (FOUNDATION ALREADY SHIPPED)

Spec 037 (`specs/037-llm-agent-tools/`, status `done`, full-delivery)
shipped a complete LLM-scenario agent runtime that this spec's
capability layer MUST consume. This subsection makes the substrate
boundary explicit so design, plan, and implementation phases all
treat it as a hard constraint.

### 3.1.1 What Spec 037 Already Provides

Verified by reading the Spec 037 state.json execution history and the
on-disk packages as of 2026-05-28:

| Concern | Spec 037 artifact | Location |
|---------|-------------------|----------|
| Intent envelope | `agent.IntentEnvelope` (Source, RawInput, ConfidenceFloor override) | `internal/agent/router.go` |
| Intent router (similarity + explicit-id + floor + fallback) | `agent.Router` with `RoutingDecision` (Reason ∈ explicit_scenario_id, similarity_match, fallback_clarify, unknown_intent) | `internal/agent/router.go` |
| Scenario registry (YAML + hot-reload) | `agent.Bridge` (loader, registry, SIGHUP-triggered `Reload`) | `internal/agent/bridge.go`, loader in same package |
| Executor (loop, retry budget, allowlist, schema validation) | `agent.Executor` with the full `Outcome` enum (ok, allowlist-violation, hallucinated-tool, tool-error, tool-return-invalid, schema-failure, loop-limit, timeout, provider-error, input-schema-violation, unknown-intent) | `internal/agent/executor.go` |
| LLM driver (Go ↔ Python over NATS) | `agent.NATSLLMDriver` + `ml/app/agent.py::handle_invoke` | `internal/agent/nats_driver.go`, `ml/app/agent.py` |
| Trace persistence + replay | `agent.PostgresTracer`, migration `020_agent_traces.sql`, `smackerel agent replay/traces/scenarios/tools` CLI | `internal/agent/tracer.go`, `internal/agent/replay.go`, `cmd/core/cmd_agent.go` |
| Telegram bridge (CALLABLE; NOT wired into bot.go) | `telegram.AgentBridge.Handle(ctx, chatID, text)` rendering via `internal/agent/userreply` (≤4-line Telegram budget, trace ref appended) | `internal/telegram/agent_bridge.go`, `internal/agent/userreply/` |
| REST surface | `api.AgentInvokeHandler` on `POST /v1/agent/invoke` | wired in `cmd/core/wiring_agent.go` |
| Admin web UI | `/admin/agent/*` views (traces, scenarios, tools) | `internal/web/` |
| SST config | `agent.*` block with full NO-DEFAULTS contract; provider routing for `gemma3:4b` default+vision, `deepseek-r1:7b` reasoning, `deepseek-ocr:3b` OCR, `nomic-embed-text` embeddings | `config/smackerel.yaml`, `internal/agent/config.go` |

### 3.1.2 The Single Real Wiring Gap

`internal/telegram/bot.go::handleMessage` routes every non-command,
non-special, non-media plain-text message directly to
`handleTextCapture` (verified at line 534). It never invokes
`AgentBridge`. The bridge is constructed in `cmd/core/wiring_agent.go`
and attached only to the REST handler (`deps.AgentInvokeHandler`);
the `*telegram.Bot` struct does not hold a reference to it. The
"scope 10 work" comment near line 21 of `agent_bridge.go` was
aspirational — Spec 037 closed at scope 9 without that wiring.

Closing this single wiring gap is the irreducible minimum for the
end-to-end "user sends free-form text via Telegram → Ollama replies"
behavior described in the owner's 2026-05-28 directive. It belongs
in this spec's **SCOPE-05** (Telegram adapter v1 reference). Per
artifact-ownership rules (`bubbles-artifact-ownership-routing`),
Spec 037 cannot be reopened because it is in terminal `done` status.

### 3.1.3 Conceptual Mapping (Spec 061 ⇄ Spec 037)

| Spec 061 concept | Spec 037 substrate | New code needed |
|------------------|--------------------|------------------|
| `AssistantMessage` (inbound) | `agent.IntentEnvelope` | Thin facade adding `userID`, `transport`, `kind`, `confirmRef`, `disambiguationRef` |
| `AssistantResponse` (outbound) | `agent.InvocationResult` + `agent.RoutingDecision` | Thin facade adding `status` token, structured `sources[]`, `confirmCard`, `disambiguationPrompt`, `errorCause`, `captureRoute` |
| Skill | Spec 037 Scenario (YAML) with user-facing `intent_examples` | 3 v1 scenario YAMLs (retrieval, weather, notifications) + their tool handlers in the existing tool registry |
| Skill Registry | Spec 037 scenario loader + hot-reload | None — reuse as-is |
| Intent Router (3-band: high / borderline / low) | Spec 037 `agent.Router` (binary: above-floor / below-floor) | Borderline-band logic on TOP of `RoutingDecision`. Could be (a) capability-layer post-processing of `Router.Route()`, (b) a new `Outcome` value in Spec 037 (would require Spec 037 amendment — DO NOT do without owner ratification), or (c) a separate scenario whose final response carries a `disambiguationPrompt`. Recommend (a). |
| Capture-as-fallback | Today only the bot's default path; Spec 037 returns `OutcomeUnknownIntent` | Bot dispatch rule: on `OutcomeUnknownIntent` OR low-band routing, fall through to `handleTextCapture` |
| Conversational state (multi-turn, idle reset) | Not present in Spec 037 | Net-new capability-layer component (state-store schema per Spec 061 design.md §6 stands) |
| Confirm card (side-effect gate) | Not present in Spec 037 (its tools execute directly) | Net-new — implemented in the capability layer as a wrapper that holds the InvocationResult, surfaces a `confirmCard`, and only invokes the side-effecting tool after the confirm callback round-trip |
| Provenance enforcement (BS-007) | Spec 037 records every tool call in the trace but does not REJECT empty-sources synthesis | Net-new — capability-layer gate over `InvocationResult.Final` |
| Per-transport telemetry | Spec 037 has per-scenario metrics | Net-new `transport=<name>` label dimension; reuse existing Prometheus scrape |
| Trace persistence + replay | Spec 037 `PostgresTracer` + `agent_traces` table | None — reuse; tag traces with `transport` |
| Tool registry | Spec 037 already has retrieval-adjacent tools (e.g. `recommendation`) | Add weather + notifications tool handlers under `internal/agent/.../tools/` |
| Telegram adapter | Existing `AgentBridge` is 80% of the adapter | Wrap `AgentBridge` in `internal/telegram/assistant_adapter/`, add rendering for the net-new `AssistantResponse` fields (status token, sources block, confirm card, disambiguation prompt, error line) per UX §14.B.1 |

### 3.1.4 Forbidden Duplications

The following packages/types MUST NOT be re-created under
`internal/assistant/` (or any sibling) by Spec 061 implementation:

- A new "Router" type. Use `agent.Router`.
- A new "Executor" type. Use `agent.Executor`.
- A new scenario loader / hot-reload mechanism. Use `agent.Bridge`.
- A new trace persistence layer. Use `agent.PostgresTracer`.
- A new NATS LLM driver. Use `agent.NATSLLMDriver`.
- A new `IntentEnvelope` shape. `AssistantMessage` MUST embed or
  trivially convert to `agent.IntentEnvelope`.

Net-new packages that ARE expected:

- `internal/assistant/contracts/` — `AssistantMessage`,
  `AssistantResponse`, `TransportAdapter` interface, `Source`,
  `ConfirmCard`, `DisambiguationPrompt`.
- `internal/assistant/provenance/` — RequireProvenance gate (BS-007).
- `internal/assistant/context/` — per-`(user_id, transport)` rolling
  context with PostgreSQL backing per design §6.
- `internal/assistant/confirm/` — confirm-card state machine + audit
  artifact emission.
- `internal/telegram/assistant_adapter/` — Telegram v1 adapter.
- Three new scenario YAMLs in `AGENT_SCENARIO_DIR` + their tool Go
  handlers.

This subsection is binding on bubbles.design, bubbles.plan, and
bubbles.implement. Any departure requires owner ratification recorded
in `uservalidation.md`.

---

## 4. Use Cases

Use cases are written in transport-neutral language. Wherever a use
case says "any supported frontend", v1 will exercise it specifically via
the Telegram reference adapter; later versions exercise the same
capability via WhatsApp / web chat / mobile in-app with no change to the
capability layer.

### UC-001 — Ask a knowledge-graph question (any frontend)
- **Actor:** Human user.
- **Preconditions:** User has captured ≥1 artifact whose content is
  relevant; the assistant capability is online on at least one enabled
  transport; retrieval skill enabled.
- **Main Flow:**
  1. User sends via any supported frontend: "what did I save about
     Tailscale last month?"
  2. The transport adapter translates the inbound message to a
     canonical `AssistantMessage` and calls the capability layer.
  3. Intent router classifies as `retrieval.query` with confidence ≥
     threshold.
  4. Retrieval skill issues semantic search against existing pgvector
     store via existing `/api/search` (or its successor).
  5. Skill calls LLM bridge to synthesize an answer that cites the
     top-K artifact IDs.
  6. Capability layer returns a canonical `AssistantResponse` with body
     + provenance.
  7. The adapter renders the response using transport-native widgets
     (e.g. Telegram trailing `sources:` block per UX §13.4; WhatsApp
     quick replies for follow-ups; web SSE chunks).
- **Alternative Flows:**
  - Low confidence → capture-as-fallback (idea artifact is created).
  - Zero search hits → capability returns "nothing saved yet" response
    + offers to capture the question; adapter renders.
- **Postconditions:** Reply delivered with provenance; no graph mutation
  (search is read-only); telemetry incremented for skill
  `retrieval.query` and per-transport counter.

### UC-002 — Ask for current weather (any frontend)
- **Actor:** Human user.
- **Preconditions:** Weather skill enabled in `assistant.skills.weather`;
  provider API key present in secret store; user's default location
  configured or supplied inline.
- **Main Flow:**
  1. User sends via any supported frontend: "weather in Reykjavík
     tomorrow?"
  2. Adapter → canonical `AssistantMessage` → capability layer.
  3. Intent router classifies as `weather.query`.
  4. Weather skill calls the configured provider (e.g. open-meteo) and
     formats a short answer with the provider name + retrieval
     timestamp in the canonical response.
  5. Adapter renders the response.
- **Alternative Flows:**
  - Provider unreachable → capability returns canonical "provider
    unavailable" error response with `cause=unavailable`; adapter
    renders + captures the question.
- **Postconditions:** No graph mutation; telemetry incremented.

### UC-003 — Summarize unread email (any frontend; v2 skill)
- **Actor:** Human user.
- **Preconditions:** Email-read skill enabled (v2 separate spec); email
  connector credentials present; **only IMAP-readable inbox** is in v2
  scope.
- **Main Flow:**
  1. User sends via any supported frontend: "summarize my unread mail".
  2. Adapter → canonical `AssistantMessage` → capability layer.
  3. Intent router classifies as `email.summarize`.
  4. Email skill pulls unread headers + bodies via IMAP, calls LLM
     bridge for per-message TL;DR, returns a ranked digest in the
     canonical response, capped at N.
  5. Adapter renders the digest sized for the transport (Telegram
     phone-screen fit; web vertical scroll; etc. — Principle 7).
- **Alternative Flows:**
  - No unread mail → capability returns "inbox clear" canonical
    response.
  - IMAP failure → canonical error response + capture the user request.
- **Postconditions:** Email content is **not** persisted to the graph by
  this flow (per Principle 1 — observation, not action), but the user's
  *request* is logged via telemetry. Email artifacts may still flow
  into the graph via a separate email connector (not this spec).

### UC-004 — Schedule a notification / reminder (any frontend)
- **Actor:** Human user.
- **Preconditions:** Notifications skill enabled; scheduler service
  available (re-use spec 054 notification handler).
- **Main Flow:**
  1. User sends via any supported frontend: "remind me to take out the
     trash at 7pm".
  2. Adapter → canonical `AssistantMessage` → capability layer.
  3. Intent router classifies as `notification.schedule`.
  4. Skill extracts {when, what} and returns a canonical
     `AssistantResponse` containing a `confirmCard` payload.
  5. Adapter renders the confirm card with two confirm/cancel widgets
     using the transport's native confirmation mechanism (Telegram
     inline keyboard, WhatsApp quick reply, web button card, mobile
     native action sheet).
  6. On confirm callback (translated by the adapter back to a canonical
     confirm message), the skill registers the notification with the
     existing scheduler and returns a terminal confirmation response.
- **Alternative Flows:**
  - Time parsing ambiguous → skill returns a canonical disambiguation
    prompt (one clarifying question per Principle 6).
  - User cancels or confirm times out → request is captured as an
    `idea` (fallback).
- **Postconditions:** Scheduler record created; reminder will fire via
  spec 054 path; telemetry incremented (per skill AND per transport).

### UC-005 — Capture-as-fallback (regression guard, any frontend)
- **Actor:** Human user.
- **Preconditions:** Assistant capability online; at least one
  transport enabled.
- **Main Flow:**
  1. User sends via any supported frontend: "thought of the day:
     read 'Antifragile' again".
  2. Adapter → canonical `AssistantMessage` → capability layer.
  3. Intent router classifies confidence < threshold for every skill.
  4. Capability layer routes to `handleTextCapture` (or its
     transport-agnostic successor) exactly as today's capture path.
  5. Adapter renders the existing capture confirmation.
- **Postconditions:** `idea` artifact created in the graph (unchanged
  behavior).

### UC-006 — Telegram reference adapter integration (v1 reference)
- **Actor:** Human user on Telegram.
- **Preconditions:** Telegram adapter enabled in
  `assistant.transports.telegram.enabled`; existing chat→user_id
  mapping (spec 044 Scope 03) populated.
- **Main Flow:**
  1. Telegram update arrives at `internal/telegram/bot.go::handleMessage`.
  2. The Telegram adapter (in `internal/telegram/assistant_adapter/`)
     intercepts the plain-text branch BEFORE `handleTextCapture`,
     translates the Telegram update into a canonical
     `AssistantMessage` (resolving chat_id → user_id), and calls
     `Assistant.Handle(ctx, msg)`.
  3. Capability layer returns a canonical `AssistantResponse`.
  4. Adapter renders the response using Telegram-native widgets:
     trailing `sources:` block (UX §13.4), inline keyboard for
     confirm cards (UX §13.6), numbered choices for disambiguation
     (UX §13.3).
  5. Adapter sends the rendered message via existing `tgbotapi`.
- **Alternative Flows:**
  - Capability returns capture-route signal → adapter calls existing
    `handleTextCapture` unchanged (regression-safe path).
- **Postconditions:** Behavior on Telegram is identical to UC-001..005
  evaluated against Telegram; per-transport telemetry tagged
  `transport="telegram"`. This use case is the v1 acceptance bar; it
  proves the capability + adapter split works end-to-end on the
  reference transport.

---

## 5. Business Scenarios (BDD)

Scenarios are written in transport-neutral language ("any supported
frontend") so they apply identically to every adapter. One
Telegram-specific scenario (BS-010) anchors the v1 reference adapter
integration test.

### BS-001 — Plain note is captured (regression guard, any frontend)
```gherkin
Given the assistant capability is enabled
And at least one transport is enabled
And the intent router min confidence is 0.75
When the user sends "random thought: I should try sourdough" via any supported frontend
Then the message is routed to the existing capture path
And an `idea` artifact is created with the verbatim text
And no skill is invoked
And telemetry counter `assistant.fallback_to_capture` is incremented
And per-transport counter `assistant.fallback_to_capture{transport=<name>}` is incremented
```

### BS-002 — High-confidence retrieval question is answered with citations (any frontend)
```gherkin
Given the assistant capability is enabled
And at least one artifact about "Tailscale" exists in the user's graph
When the user sends "what did I save about Tailscale last month?" via any supported frontend
Then the retrieval skill is invoked
And the canonical AssistantResponse body contains a synthesized answer
And the canonical AssistantResponse provenance contains at least one artifact ID citation
And the adapter renders the citations using the transport's native source-attribution widget
And no graph mutation occurs
```

### BS-003 — Weather query returns provider-attributed answer (any frontend)
```gherkin
Given the weather skill is enabled
And a weather provider API key is present in the secret store
When the user sends "weather in Seattle today" via any supported frontend
Then the weather skill calls the configured provider
And the canonical AssistantResponse provenance contains the provider name and the retrieval timestamp
And no graph mutation occurs
```

### BS-004 — Notification requires explicit confirmation before scheduling (any frontend)
```gherkin
Given the notifications skill is enabled
When the user sends "remind me to call Mom at 6pm" via any supported frontend
Then the capability layer returns an AssistantResponse with a confirmCard payload
And no scheduler record exists yet
And the adapter renders the confirm card using the transport's native confirmation widget
When the user confirms via the transport's native confirmation widget
Then the capability layer receives a canonical confirm message
And a scheduler record is created for the parsed time
And the canonical AssistantResponse confirms the scheduled time
```

### BS-005 — Ambiguous intent falls back to capture (no silent skill execution, any frontend)
```gherkin
Given the assistant capability is enabled
And the intent router min confidence is 0.75
When the user sends "weather" via any supported frontend
And the router's top skill confidence is 0.40
Then the message is routed to the existing capture path
And no skill is invoked
```

### BS-006 — Weather provider outage falls back to capture (any frontend)
```gherkin
Given the weather skill is enabled
And the configured weather provider returns a 5xx error
When the user sends "weather in Seattle today" via any supported frontend
Then the capability layer returns a canonical error response with cause="unavailable"
And the adapter renders a brief "provider unavailable" message
And the request is captured as an idea
And telemetry counter `assistant.skill_error.weather` is incremented
```

### BS-007 — Synthesis without provenance is rejected (Principle 8 hard constraint, any frontend)
```gherkin
Given the retrieval skill is enabled
When the LLM bridge returns a synthesis with zero source citations
Then the capability layer does NOT return the synthesis to the adapter
And the canonical AssistantResponse body is "I don't have a sourced answer for that"
And the request is captured as an idea
And telemetry counter `assistant.provenance_violation` is incremented
```

### BS-008 — Disabled skill is not invoked even on a perfect intent match (any frontend)
```gherkin
Given the email skill is disabled in `assistant.skills.email.enabled`
When the user sends "summarize my unread mail" via any supported frontend
Then the email skill is NOT invoked
And the canonical AssistantResponse body indicates "email skill is disabled"
And the request is captured as an idea
```

### BS-009 — Missing required SST config aborts core startup (NO-DEFAULTS)
```gherkin
Given `config/smackerel.yaml` is missing `assistant.intent.min_confidence`
When the core binary starts
Then startup fails loud with a clear error naming the missing key
And no transport adapter binds its update loop
```

### BS-010 — Telegram reference adapter end-to-end (v1 acceptance)
```gherkin
Given the Telegram adapter is enabled in `assistant.transports.telegram.enabled`
And the existing Telegram chat→user_id mapping (spec 044 Scope 03) is populated
And the retrieval skill is enabled with at least one matching artifact
When a Telegram update arrives with text "what did I save about Tailscale last month?"
Then the Telegram adapter translates the update into a canonical AssistantMessage
And the capability layer returns an AssistantResponse with body + artifact-ID provenance
And the Telegram adapter renders the body followed by a trailing `sources:` block per UX §13.4
And the rendered reply is sent via existing tgbotapi without invoking any other adapter
And per-transport telemetry is tagged `transport="telegram"`
```

---

## 6. Transport Adapter Contract

Every transport adapter (Telegram in v1; WhatsApp, web chat, mobile
in-app in future versions) MUST conform to this contract. Adapters are
translation-only — they own ZERO business logic.

### 6.1 Canonical Message Contracts

The capability layer speaks ONLY in two canonical shapes. The full Go
types are owned by `bubbles.design`; the analyst-level contract is:

**`AssistantMessage`** (inbound, adapter → capability):

| Field | Description |
|-------|-------------|
| `userID` | Smackerel user identity resolved by the adapter from its transport-specific identity (e.g. Telegram chat_id via spec 044 mapping). |
| `transport` | Adapter name (`"telegram"`, `"whatsapp"`, `"web"`, `"mobile"`, …). Used for per-transport telemetry only. |
| `text` | Plain user message text. Adapters MUST strip transport-specific markup before sending. |
| `kind` | `"text"` (free-form) OR `"confirm"` (response to a previously-sent confirm card; carries the confirm reference) OR `"disambiguation"` (response to a disambiguation prompt). |
| `confirmRef` / `disambiguationRef` | Opaque identifiers issued by the capability layer in a previous response; the adapter echoes them back without interpretation. |
| `receivedAt` | Adapter-side timestamp. |

**`AssistantResponse`** (outbound, capability → adapter):

| Field | Description |
|-------|-------------|
| `status` | Closed-vocabulary status token from UX §13.2 (e.g. `"thinking"`, `"checking weather"`, `"reminder set"`, `"saved"`). Transport renders per its conventions (Telegram edits a message in place; web SSE streams; mobile native may use a spinner). |
| `body` | Plain answer text. Transport-neutral; no transport-specific markup. |
| `sources[]` | Ordered list of provenance entries. Each entry is either `{kind: "artifact", id, title, capturedAt}` or `{kind: "provider", name, retrievedAt}`. Adapter renders per UX §13.4 (Telegram trailing block; web inline links; mobile expandable card). |
| `confirmCard` | Optional. Present iff the response requires user confirmation for a side-effecting action. Contains `proposedAction`, opaque `payload`, `timeout`, and `confirmRef`. Adapter renders using the transport's native confirmation widget. |
| `disambiguationPrompt` | Optional. Present iff the router is in the borderline band. Contains `choices[]` (max 3, always includes `save as note`), `timeout`, and `disambiguationRef`. Adapter renders using numbered choices, quick replies, or the transport's native chooser. |
| `errorCause` | Optional. Closed-vocabulary cause from UX §13.7 (`unavailable`/`unauthorized`/`parser`/`disabled`/`rate-limited`). Adapter renders an error line in the transport's native style. |
| `captureRoute` | Boolean. When `true`, signals the adapter that the canonical handling decision is "this is a capture, not an answer"; the adapter MUST invoke its existing capture path (or the transport-agnostic capture service) and produce the existing capture confirmation in the transport's native style. |

### 6.2 Adapter Responsibilities

Every adapter MUST:

1. **Translate inbound** transport-native messages into `AssistantMessage`
   (resolving transport identity to `user_id`).
2. **Call** the capability layer's `Assistant.Handle(ctx, msg)` interface.
3. **Render outbound** `AssistantResponse` using the transport's native
   widgets, preserving semantics across all five rendered elements
   (status, body, sources, confirm, disambiguation).
4. **Translate confirm / disambiguation callbacks** from transport-native
   widget callbacks back into `AssistantMessage` (kind=`"confirm"` or
   `"disambiguation"`) so the capability layer is the sole decision
   point.
5. **Honor `captureRoute=true`** by invoking the existing capture path
   on that transport. (For Telegram v1 this is `handleTextCapture`.)
6. **Emit per-transport telemetry** tagged `transport=<name>` for the
   counters defined in design.md §11.
7. **Authenticate** the inbound user against Smackerel identity using
   the transport's own auth mechanism (spec 044 chat→user_id mapping
   for Telegram; future adapters define their own).

Every adapter MUST NOT:

1. Classify intent.
2. Invoke skills directly.
3. Mutate the knowledge graph.
4. Embed any skill-specific rendering logic (e.g. a Telegram-specific
   "weather" formatter).
5. Hold any per-skill secret (skill credentials live in the capability
   layer / secret store).
6. Implement its own conversational state or context window (the
   capability layer owns multi-turn context per UX §13.5).
7. Render any answer with empty `sources[]` (mirrors the
   `RequireProvenance` gate at the capability layer; an adapter that
   sees empty sources alongside a non-empty body MUST drop the message
   and log a violation — belt-and-braces).

### 6.3 Semantic Parity Across Transports

The per-transport rendering may differ (Telegram inline buttons vs.
WhatsApp quick replies vs. web SSE+button cards vs. mobile native
action sheets), but the SEMANTICS MUST be identical:

| Canonical element | Required semantic on every transport |
|-------------------|--------------------------------------|
| `status` | At most ONE status token per turn; closed vocabulary; lowercase; in-flight (`thinking…`) vs terminal (`saved`) distinguishable. |
| `body` | Plain text; no greetings; no chattiness; fits the transport's natural message size. |
| `sources[]` | Visible, scannable, attributable to artifact ID OR external provider + timestamp; max 5 entries with overflow indication. |
| `confirmCard` | Two terminal outcomes (confirm / cancel) plus timeout; timeout default ALWAYS captures; confirmation widget is the ONLY input (typed `yes`/`no` not accepted). |
| `disambiguationPrompt` | Max 3 choices; `save as note` always present; numeric/command shortcut OR transport widget accepted; non-matching reply within timeout is captured. |
| `errorCause` | Closed cause vocabulary; never expose stack traces; offer to capture as a follow-up. |
| `captureRoute=true` | Existing transport capture confirmation reproduced byte-for-byte where possible; no new UX. |

The per-transport rendering tables (e.g. Telegram inline-keyboard
format, WhatsApp quick-reply format) belong in `design.md` and
per-adapter docs — NOT in this spec.

### 6.4 SST Surface for Transports

A new `assistant.transports.*` SST block (NO-DEFAULTS, fail-loud)
gates each adapter independently. v1 ships only the Telegram adapter,
but the SST shape MUST accommodate future adapters:

```yaml
assistant:
  transports:
    telegram:
      enabled: ${ASSISTANT_TRANSPORT_TELEGRAM_ENABLED:?required}
    # whatsapp: ...   # future adapter
    # web:      ...   # future adapter
    # mobile:   ...   # future adapter
```

At least one transport MUST be enabled iff `assistant.enabled=true`;
otherwise startup fails loud. Disabling all transports while leaving
`assistant.enabled=true` is a configuration error.

---

## 7. UI Scenario Matrix (transport-aware)

The matrix is transport-aware: each row's expected outcome is described
in canonical terms (status token, body, sources, confirm card,
disambiguation prompt); each enabled transport renders that outcome
using its native widgets. v1 exercises the matrix only via the Telegram
reference adapter.

| Scenario | Actor | Entry Point | Steps | Expected Canonical Outcome | Telegram Rendering (v1 reference) |
|----------|-------|-------------|-------|----------------------------|------------------------------------|
| Plain capture (regression) | User | Send "random thought" via any frontend | 1 msg | `captureRoute=true` | Existing Telegram capture confirmation |
| Retrieval Q&A | User | Send "what did I save about X" via any frontend | 1 msg | `status=thinking`, `body=<answer>`, `sources=[artifact]` | Inline status edit → body + trailing `sources:` block per UX §13.4 |
| Weather query | User | Send "weather in X" via any frontend | 1 msg | `status=checking weather`, `body=<forecast>`, `sources=[provider]` | Inline status edit → body + trailing `sources:` block with provider + timestamp |
| Email digest (v2) | User | Send "summarize unread mail" via any frontend | 1 msg | `body=<ranked digest>`, `sources=[provider]` | Phone-screen-sized digest |
| Notification (confirm path) | User | Send "remind me to …" via any frontend | 2 msgs (request + confirm callback) | `confirmCard` → confirm callback → `status=reminder set` | Inline keyboard `[✅ schedule][❌ cancel]` → terminal status token |
| Notification (cancel path) | User | Send "remind me to …" then cancel | 2 msgs | `confirmCard` → cancel callback → `captureRoute=true` | Inline keyboard → capture confirmation |
| Ambiguous → capture fallback | User | Send "weather" alone via any frontend | 1 msg | `captureRoute=true` | Existing Telegram capture confirmation |
| Borderline → disambiguation | User | Send borderline-confidence msg via any frontend | 1 msg + reply | `disambiguationPrompt` → user picks → dispatch | Numbered list `1./2./3.` (Telegram has no native chooser; uses text + commands) |
| Skill disabled | User | Send query for disabled skill via any frontend | 1 msg | `errorCause=missing_scope` + `captureRoute=true` follow-up | Error line + capture confirmation |
| Provider outage | User | Send query when provider down | 1 msg + confirm | `errorCause=provider_unavailable` + offer-to-capture confirm card | Error line + inline keyboard `[yes][no]` |
| Notification (confirm-card timeout) | User | Send "remind me to …" then ignore | 1 msg + timeout | `confirmCard` → timeout (no callback) → `captureRoute=true` (discard action, capture original msg) | Inline keyboard disappears → capture confirmation |
| Borderline → disambiguation timeout | User | Send borderline-confidence msg, no reply | 1 msg + timeout | `disambiguationPrompt` → timeout → `captureRoute=true` | Numbered list → capture confirmation |
| Borderline → non-matching reply | User | Send borderline-confidence msg, then unrelated free text | 1 msg + 1 unrelated msg within timeout | `disambiguationPrompt` → fresh-input (no `disambiguationRef` echo) → reply treated as new capture or new intent classification | Numbered list → unrelated reply enters fresh classification path |

---

## 8. Competitive Analysis

| Capability | Smackerel (today) | Smackerel (after this spec) | Notion AI Bot | Telegram + ChatGPT bridges | Apple/Google Assistant |
|------------|-------------------|------------------------------|---------------|----------------------------|------------------------|
| Passive capture | ✅ all messages | ✅ preserved as fallback | ❌ | ❌ | ⚠️ partial |
| Retrieval Q&A over personal graph | ❌ | ✅ v1 | ✅ (Notion-only) | ❌ | ❌ |
| Source-attributed answers | n/a | ✅ enforced (Principle 8) | ⚠️ inconsistent | ❌ usually none | ❌ |
| Capture-as-fallback safety net | n/a | ✅ enforced | ❌ | ❌ | ❌ |
| Self-hosted, owner-controlled | ✅ | ✅ | ❌ | ❌ | ❌ |
| Explicit confirmation for side-effecting skills | n/a | ✅ | ⚠️ varies | ⚠️ varies | ⚠️ varies |

**Differentiation:** the safety net (capture-as-fallback) plus enforced
source attribution are the two structural choices no consumer competitor
makes. Both follow directly from Smackerel product principles.

---

## 9. Pressure-Tested v1 / v2 Skill Split

The owner directive lists four candidate skills. Not all are equally
ready. Honest assessment:

| Skill | Recommended phase | Effort (S/M/L) | Value | Dependencies | Rationale |
|-------|-------------------|----------------|-------|--------------|-----------|
| **Retrieval Q&A over the knowledge graph** | **v1** | M | High | Existing `/api/search` + LLM bridge in `ml/` sidecar | Lowest infra delta. Uses what is already running. Highest principle-alignment (Principles 2, 8). |
| **Weather (read-only external API)** | **v1** | S | Medium | One external provider + 1 secret in secret store | Cleanly contained. Good first non-graph skill to prove the skill-registry abstraction. |
| **Notifications / reminders** | **v1** | M | High | Existing spec 054 notification handler; confirmation UX | Re-uses an already-shipped foundation; the only new surface is the confirm/cancel flow. |
| **Email read + summarize (IMAP)** | **v2** (separate spec) | L | Medium | New email connector + IMAP creds + privacy review for "what counts as captured?" | Adds a brand-new connector surface, requires its own product principle pass (especially Principle 4 — Source-Qualified Processing), and the LLM bridge has to learn to summarize content it does not persist. Belongs in its own spec to avoid scope creep here. |

**v1 skill set (this spec):** retrieval Q&A, weather, notifications.
**v2 skill set (separate spec):** email read + summarize.

This split keeps v1 deployable behind the existing `assistant.enabled`
SST switch and avoids the email-connector design surface bleeding into
this spec.

---

## 10. Non-Goals

- **NOT removing capture.** Capture is the product floor (Principle 1).
- **NOT a marketplace.** No third-party skill submission, no
  GuestHost-style mediation.
- **NOT financial advice.** Even if QF integration later exists, this
  assistant never executes trades or gives financial recommendations.
- **NOT a new Telegram bot token / new chat binding for the v1
  reference adapter.** The Telegram adapter reuses the existing
  chat→user_id mapping from spec 044 Scope 03 as its sole auth
  surface.
- **NOT a generic chat LLM proxy.** Free-text questions that do not
  match a registered skill fall back to capture, not to a generic
  ChatGPT reply.
- **NOT writing email to the knowledge graph from this spec.** That
  belongs in the v2 email connector spec.
- **NOT transport-specific feature parity beyond the canonical
  capability set.** Each adapter ships what its transport natively
  supports for the canonical elements (status, body, sources,
  confirm, disambiguation, errorCause, captureRoute). An adapter is
  NOT required to invent transport-only features beyond that
  canonical surface, and the canonical surface itself MUST NOT
  expand to accommodate per-transport bells and whistles.
- **NOT shipping non-Telegram adapters in v1.** WhatsApp, web chat,
  and mobile in-app adapters are explicit future work, each in their
  own spec. v1 ships the capability layer + the Telegram reference
  adapter only.

---

## 11. Product Principle Alignment

Per `.github/instructions/product-principles.instructions.md`. Each
ratified principle must be addressed.

**Capability-first design note** (per
`.github/skills/bubbles-capability-foundation-design/SKILL.md`): this
spec is a foundation-introducing spec. It defines (a) a transport-agnostic
assistant capability (intent router, skill registry, three v1 skills,
conversational state, source attribution) and (b) a `TransportAdapter`
interface with one v1 reference implementation (Telegram). The
proportionality trigger applies on TWO axes simultaneously: multiple
skills sharing the `Skill` interface (≥2 implementations) AND multiple
transports sharing the `TransportAdapter` interface (1 in v1, but at
least 3 planned: WhatsApp / web / mobile). The foundation is
introduced now, not retrofitted later.

### Principle 1 — Observe First, Ask Second  (⚠️ explicit deviation)

This spec is a **partial, justified deviation**. The principle says
"infer at observation time; do not require user action". This spec
introduces a path where the user actively asks the bot for something.

**Justification:**
1. The capture path is **strictly preserved** as the fallback — passive
   ingestion is never blocked or replaced.
2. The new behavior is **additive**: the user can keep using the bot
   exactly as today (every plain note still becomes an `idea`).
3. The active-ask path covers operations that **cannot be inferred from
   capture alone** — weather and notifications are inherently
   user-triggered actions; retrieval Q&A is the read-side of the
   graph, which is meaningless without an explicit query.
4. The intent router's default behavior under uncertainty is to honor
   the principle (fall back to capture).

This deviation MUST be re-affirmed during owner ratification.

### Principle 2 — Vague In, Precise Out

Directly served by retrieval Q&A: the user asks an imprecise question;
the system returns a sourced, ranked answer derived from semantic search
+ LLM synthesis. The skill registry is designed so that every skill
returns precise, attributable output regardless of how vague the input
was.

### Principle 6 — Invisible By Default, Felt Not Heard

Honored as follows:
- The bot **never** initiates a conversation from the assistant path.
- The notification skill asks at most **one** clarifying question per
  request (Principle 6's actionability bar).
- Status-update prompts ("we processed X messages today") are
  explicitly forbidden by the non-goals.
- The bot remains silent on capture (no new chattiness) — capture
  confirmation messages are unchanged.

### Principle 8 — Trust Through Transparency

This is the load-bearing principle for this spec. It is enforced as a
**hard constraint** in §3:
- Every synthesized answer must cite source artifacts or an external
  provider + timestamp.
- BS-007 proves the system rejects un-sourced synthesis at runtime.
- The architecture (handler returns `{response, provenance}` tuple)
  makes provenance impossible to omit.

---

## 12. SST Configuration Contract (NO-DEFAULTS / fail-loud)

New `assistant.*` block in `config/smackerel.yaml`. Every value is
required; missing keys abort startup per spec `smackerel-no-defaults`.

```yaml
assistant:
  enabled: ${ASSISTANT_ENABLED:?ASSISTANT_ENABLED required}
  intent:
    min_confidence: ${ASSISTANT_INTENT_MIN_CONFIDENCE:?required, 0..1 float}
    classifier_model: ${ASSISTANT_INTENT_MODEL:?required, e.g. "llama3.1:8b"}
  skills:
    retrieval:
      enabled: ${ASSISTANT_SKILL_RETRIEVAL_ENABLED:?required}
      top_k: ${ASSISTANT_SKILL_RETRIEVAL_TOP_K:?required, int}
    weather:
      enabled: ${ASSISTANT_SKILL_WEATHER_ENABLED:?required}
      provider: ${ASSISTANT_SKILL_WEATHER_PROVIDER:?required, e.g. "open-meteo"}
      # Secret reference, not value. Resolved from secret store by deploy adapter.
      api_key_ref: ${ASSISTANT_SKILL_WEATHER_API_KEY_REF:?required, secret-store key name}
    notifications:
      enabled: ${ASSISTANT_SKILL_NOTIFICATIONS_ENABLED:?required}
      confirm_required: ${ASSISTANT_SKILL_NOTIFICATIONS_CONFIRM_REQUIRED:?required, bool}
```

**Forbidden shapes** (per `smackerel-no-defaults`):
- `${VAR:-default}` colon-dash fallback
- Literal API keys inline
- Implicit per-skill enable (every skill enable must be explicit)

The deploy adapter (in `knb/smackerel/<target>/`) is responsible for
populating `*_api_key_ref` indirection and ensuring secret-store values
exist before apply. If any secret-store ref is unresolved, startup MUST
fail loud (no skill falls back to "disabled" silently — see BS-008
which assumes explicit-disable).

---

## 13. Open Questions (for bubbles.design / owner)

1. **Intent classifier substrate.** Local Ollama model vs. a small
   purpose-trained classifier? Owner preference + cost/latency
   tradeoff needed. Spec assumes Ollama via existing `ml/` sidecar.
2. **Notification skill: re-use spec 054 path or a thinner wrapper?**
   Spec 054 is geared for system-generated notifications; user-scheduled
   reminders may need a small extension to the scheduler payload.
3. **Per-skill scopes via spec 060.** Should each skill require its own
   PASETO scope (e.g. `assistant.retrieval`, `assistant.weather`) or is
   `assistant.*` granularity enough for v1? Recommend per-skill for
   blast-radius containment but defer to bubbles.design.
4. **Provenance UX for retrieval answers.** Inline citations (`[1]
   [2]`) vs. trailing "Sources:" block? UX call for bubbles.ux.
   **RESOLVED 2026-05-28** by bubbles.ux §14.B.1: trailing
   `sources:` block on Telegram; structured `sources[]` on the
   capability surface.
5. **Substrate reconciliation against Spec 037 (BLOCKING on
   bubbles.design).** Surfaced 2026-05-28 by bubbles.analyst.
   `design.md` (authored 2026-05-28 by bubbles.design) proposes a
   parallel `internal/assistant/{router,registry,executor,tracer}`
   stack that re-implements machinery already shipped by Spec 037
   (`internal/agent/{router,bridge,executor,tracer,nats_driver}`).
   Hard Constraint 6 and §3.1 now make substrate reuse binding.
   bubbles.design MUST revise `design.md` so that:
   (a) the capability layer's intent router IS `agent.Router`
       (with an additional borderline-band post-processor for the
       three-band decision rule from UX §14.A.3);
   (b) the "skill registry" IS Spec 037's scenario loader (the
       three v1 skills ship as YAML scenarios under
       `AGENT_SCENARIO_DIR` plus tool handlers in the existing
       tool registry);
   (c) the executor + tracer + LLM driver are reused as-is;
   (d) `AssistantResponse` is constructed as a thin facade over
       `agent.InvocationResult` PLUS the net-new fields
       (`status`, structured `sources[]`, `confirmCard`,
       `disambiguationPrompt`, `errorCause`, `captureRoute`);
   (e) the module layout in design §10 collapses to the net-new
       packages enumerated in §3.1.4 (contracts, provenance,
       context, confirm, telegram adapter) PLUS three scenario
       YAMLs + tool handlers — no parallel `internal/assistant/
       router` / `executor` / `tracer` packages.
   Until this reconciliation lands, scopes.md SCOPE-03 (skill
   registry foundation) and SCOPE-04 (intent router) are
   structurally wrong about what they are building, and the
   capability-foundation-design proportionality satisfaction in
   §11 partially over-claims (foundation already exists; the
   spec is foundation-EXTENDING, not foundation-INTRODUCING on
   the agent-runtime axis — it IS foundation-introducing on the
   TransportAdapter axis).
6. **Borderline-band placement.** Spec 037's `Router` is binary
   (above/below floor). The three-band decision rule
   (high/borderline/low) introduced by UX §14.A.3 requires either
   (a) capability-layer post-processing of `RoutingDecision.TopScore`
   against a new `borderline_floor` SST key, (b) a Spec 037
   amendment to add a `BorderlineFloor` field and an extra
   `Outcome` value (would need Spec 037 reopen via separate bug),
   or (c) emitting the disambiguation prompt as the `Final` of a
   dedicated "clarify" scenario. Recommend (a) — purely additive,
   keeps Spec 037 terminal. Decision belongs to bubbles.design.
7. **Confirm-card primitive ownership.** Spec 037's executor
   invokes tools directly; there is no "propose, then execute
   after user confirmation" primitive. The notifications skill
   (BS-004) requires this. Net-new capability-layer component
   (`internal/assistant/confirm/`) — confirmed in §3.1.3. Design
   needs to specify the state machine: how does the capability
   layer pause a tool invocation, surface `confirmCard`, persist
   the pending action across the round-trip, and resume on
   confirm callback (vs. drop on cancel/timeout and capture the
   original message)?

---

## 14. UX & Interaction Model

> Owner: bubbles.ux. Revised 2026-05-28 to separate **transport-neutral
> interaction semantics** (§14.A) from **transport-specific rendering**
> (§14.B), per the capability-first doctrine in
> `bubbles-capability-foundation-design`. The capability layer is the
> source of truth for assistant behavior; each adapter renders that
> behavior using its transport's native primitives. The §6 canonical
> message contracts (`AssistantMessage`, `AssistantResponse`) bind both
> halves: §14.A defines what fields the capability emits and what they
> mean; §14.B defines how each transport paints them on the wire.
>
> Downstream agents (bubbles.design, bubbles.implement,
> bubbles.cinematic-designer) MUST treat the §14.A semantics as binding
> for the capability layer and §14.B.1 as binding for the v1 Telegram
> reference adapter. The §14.B.2/3/4 sketches are non-binding guidance
> for future adapters until each future adapter ships its own
> per-adapter UX spec.

### 14.A Transport-Neutral Interaction Semantics

Defines the canonical behavior the capability layer emits via
`AssistantResponse`. Every adapter MUST honor these semantics
identically. No adapter may add, remove, or reinterpret a field.

#### 14.A.1 Design Principles (capability layer)

1. **Capture is the floor.** Every uncertain message becomes an `idea`
   artifact. The capability layer NEVER silently invokes a skill on
   uncertainty (Principle 1 deviation justification, §11;
   capture-as-fallback is inviolable per
   `policySnapshot.captureAsFallback`).
2. **Terseness is contractual.** The capability layer emits at most ONE
   status token per turn and never emits greetings, preambles, or
   chattiness in `AssistantResponse.body` (Principle 6 — Invisible by
   Default).
3. **Provenance is non-negotiable.** Every non-empty `body` that
   represents a synthesized answer MUST be accompanied by a non-empty
   `sources[]`. Empty-sources synthesis is dropped server-side
   (BS-007). Adapters MUST refuse to render an `AssistantResponse`
   with a non-empty synthesis `body` and empty `sources[]`
   (belt-and-braces, §6.2). Error-line bodies and confirm/disambiguation
   prompt bodies are exempt because they are not synthesis.
4. **Predictable closed vocabularies.** Status tokens, error causes,
   and source kinds are closed sets (§14.A.2, §14.A.7, §14.A.4). The
   capability layer NEVER emits a value outside these sets.
5. **One ask, one answer.** The capability layer emits at most ONE
   `disambiguationPrompt` per turn. If the user's reply is still
   ambiguous, the next turn falls back to capture (no nested
   clarification trees).
6. **Phone-screen-sized output is a capability invariant.** Exact
   wrapping is transport-specific, but `body` length is bounded by
   `assistant.capability.body_max_chars` (SST) so every adapter has
   headroom to render `sources[]` and confirm/disambiguation widgets
   inside its transport's natural message size.

#### 14.A.2 Canonical Status Vocabulary

`AssistantResponse.status` is a single token drawn from a closed
vocabulary. The capability layer emits the token; the adapter chooses
how to paint it (text prefix, edited message, reaction emoji, SSE
event, mobile spinner — see §14.B per transport).

| Token | Meaning | In-flight / Terminal |
|-------|---------|----------------------|
| `thinking` | Retrieval or general LLM synthesis in progress. | in-flight |
| `checking_weather` | Weather skill calling external provider. | in-flight |
| `checking_email` | Email skill calling external provider (v2). | in-flight |
| `reminder_proposed` | Notification skill has emitted a `confirmCard`. | terminal-of-turn |
| `reminder_confirmed` | User confirmed and scheduler accepted. | terminal |
| `reminder_cancelled` | User cancelled OR confirm-card timeout discarded the action. | terminal |
| `saved_as_idea` | Capture path fired (capture-as-fallback or low-confidence). | terminal |
| `unavailable` | Skill failed; refine via `errorCause` (§14.A.7). | terminal |

Capability-layer rules:
- Exactly one status token per `AssistantResponse`.
- Tokens are snake_case, lowercase, and stable across releases.
- In-flight tokens MUST be followed by a terminal turn within
  `assistant.capability.status_max_duration` (SST) or the capability
  emits `status=unavailable` with `errorCause=internal_error`.
- Adapters MUST NOT invent rendering conventions that change meaning
  (e.g. an adapter MUST NOT collapse `reminder_proposed` and
  `reminder_confirmed` into one render — they are user-distinguishable).

#### 14.A.3 Capture-vs-Answer Disambiguation (three-band confidence)

The intent router emits a confidence score in `[0.0, 1.0]`. The
capability layer applies a **transport-neutral** three-band decision
rule (SST floors, no defaults):

| Band | Bounds | Capability decision | `AssistantResponse` shape |
|------|--------|---------------------|---------------------------|
| **High** | `confidence ≥ assistant.capability.intent.min_confidence` | Invoke skill. | `status=<skill-token>`, `body=<answer>`, `sources[]` populated. |
| **Borderline** | `assistant.capability.intent.disambiguate_floor ≤ confidence < min_confidence` | Ask once. | `disambiguationPrompt={choices[≤3], timeout, disambiguationRef}`; no skill invoked yet. |
| **Low** | `confidence < assistant.capability.intent.disambiguate_floor` | Capture silently. | `captureRoute=true`, `status=saved_as_idea`. |

**Default outcome is capture.** Borderline asks the user ONCE; if the
user does not reply within
`assistant.capability.intent.disambiguate_timeout`, the capability
layer captures the original message and emits a follow-up
`AssistantResponse` with `captureRoute=true` and
`status=saved_as_idea`.

Capability-layer rules:
- `disambiguationPrompt.choices[]` has length ≤3, ordered
  most-likely-skill first, and `{id: "save_as_note", label: "save as
  note"}` is ALWAYS the last entry.
- A non-matching inbound reply (`kind=text`, no `disambiguationRef`)
  within the timeout window is treated as a fresh capture OR a new
  intent classification (the user moved on), NOT as an answer to the
  prompt.
- `disambiguationRef` is opaque to the adapter; the adapter MUST echo
  it on the inbound `AssistantMessage` (`kind=disambiguation`).

The decision rule is transport-neutral. Telegram numbered buttons,
WhatsApp quick replies, web SSE pickers, and mobile native choosers
all yield the same capability behavior because they all produce the
same `disambiguationRef` echo.

#### 14.A.4 Source Attribution Semantics

`AssistantResponse.sources[]` is a **structured** ordered list. The
capability layer MUST NOT pre-format sources as a string; rendering is
the adapter's job.

```go
// Capability-layer contract (Go-style sketch; design.md owns canonical types)
type Source struct {
    ID    string     // stable identifier (artifact UUID OR provider name)
    Title string     // human label
    Kind  SourceKind // "artifact" | "external_provider"
    Ref   SourceRef  // discriminated union below
}

type SourceRef interface{ isSourceRef() }
type ArtifactRef struct {
    ArtifactID string
    CapturedAt time.Time
}
type ExternalProviderRef struct {
    ProviderName string
    RetrievedAt  time.Time
}
```

Capability-layer rules:
- `len(sources) ≤ assistant.capability.sources_max` (SST; recommend 5).
- Sources are ordered by relevance (most-relevant first).
- If more sources existed than the cap, the capability sets
  `sources_overflow_count` on the response; adapters MAY render an
  overflow indicator natively ("+N more", "show all", etc.).
- The capability layer NEVER emits a string-formatted sources block.
  Every adapter renders structured `Source` values to its transport's
  conventions (§14.B).

#### 14.A.5 Multi-Turn Thread Context

The capability layer maintains a per-`(user_id, transport)` rolling
context of the last `assistant.capability.context.window_turns`
interactions. Each entry: `{user_msg, skill_used, result_summary,
source_ids, ts}`.

**Conversational state key recommendation:** `(user_id, transport)`.
Rationale (for bubbles.design to ratify):
- A user MAY interact with Smackerel via Telegram and WhatsApp
  concurrently with genuinely independent threads. Keying on
  `user_id` alone would cross-pollute references ("that one") between
  transports the user mentally treats as separate sessions.
- Keying on `(user_id, transport)` keeps thread reference resolution
  local to one frontend while still allowing skill scopes, provenance,
  and audit to be `user_id`-scoped at the data layer.
- Cost: one extra dimension in the state-store key. Trivially
  acceptable.

**Fresh-thread rule:** Threads reset **implicitly** after
`assistant.capability.context.idle_timeout` (SST, no default; recommend
10 minutes). The capability layer does NOT impose a manual reset
command surface — transports MAY expose one as a convenience
(§14.B.1 documents Telegram's `/reset`), but the capability semantic
is idle-driven.

**Reference resolution (capability-layer):**
- Phrases like "that one", "the second one", "the X one" → resolved
  against the most recent turn's `source_ids` for
  `(user_id, transport)`.
- Numeric references (`open 2`) → resolved against the most recent
  reply's `sources[]` (1-indexed).
- Unresolvable reference → `status=unavailable`,
  `errorCause=slot_missing`,
  `body="cannot resolve reference. last result has <N> sources."`

**Persistence:**
- Thread context lives in process memory keyed by
  `(user_id, transport)`.
- Does NOT survive capability-layer restart. A restart is a clean
  thread boundary. Artifact citations themselves are persisted in the
  graph independently of this rolling window.

#### 14.A.6 Confirm Card Semantics

`AssistantResponse.confirmCard` MAY be present on side-effecting
turns (notifications v1; email send v2+; any future skill that mutates
state outside the graph) AND on error-recovery turns (offer-to-capture).

```go
// Capability-layer contract (sketch; design.md owns canonical types)
type ConfirmCard struct {
    ProposedAction string        // human-readable summary, e.g.
                                 // `schedule "call mom" at 2026-05-28 18:00`
    Payload        []byte        // opaque, capability-private; adapter echoes only
    Timeout        time.Duration // per-skill SST:
                                 //   assistant.capability.skills.<skill>.confirm_timeout
                                 // OR assistant.capability.error.capture_timeout
                                 // for error-recovery cards.
    ConfirmRef     string        // opaque ref; adapter echoes on inbound confirm
}
```

**Three terminal outcomes** (capability-layer state machine):

| User action | Capability behavior | Follow-up `AssistantResponse` |
|-------------|---------------------|-------------------------------|
| Confirm (positive callback) | Execute the side effect. | `status=reminder_confirmed` (or skill-specific terminal) for action cards; `status=saved_as_idea`, `captureRoute=true` for offer-to-capture cards. |
| Discard (negative callback) | Drop the action; route the original message to capture. | `status=reminder_cancelled` (for action cards) THEN `status=saved_as_idea`, `captureRoute=true` follow-up. |
| Timeout (no callback within `Timeout`) | Drop the action; route the original message to capture. | `status=saved_as_idea`, `captureRoute=true`. |

**Audit invariant:** For action cards, REGARDLESS of outcome, the
capability layer MUST create an `assistant_proposal` artifact
recording the parsed action (skill, slots, proposed time) — Principle
8 transparency. Adapters MUST NOT influence this; it is a
capability-layer write.

**No typed yes/no:** The capability layer accepts ONLY
`AssistantMessage{kind: "confirm", confirmRef: ...}`. Free-text
`yes`/`no` is NOT a confirm callback; it is fresh input and is
classified by the intent router like any other message. Adapters MUST
NOT translate typed `yes`/`no` into a confirm callback (avoids parser
ambiguity with the user's next free-text intent).

#### 14.A.7 Error Semantics

`AssistantResponse.errorCause` is a closed-vocabulary cause emitted by
the capability layer when `status=unavailable`. The adapter renders a
terse user-facing line in its transport-native style.

| Cause | Meaning |
|-------|---------|
| `provider_unavailable` | External provider returned 5xx, timeout, or DNS failure. |
| `missing_scope` | Caller lacks the per-skill scope (spec 060) OR the skill is `enabled: false` in SST. |
| `slot_missing` | Skill could not extract a required slot (time, location, reference). |
| `internal_error` | Capability-layer bug, panic, or in-flight status exceeded `status_max_duration`. |

Capability-layer rules:
- The body of an error response is bounded: `body = "<skill>: <human
  cause>"` (single line, no markdown gymnastics).
- Every error response carries an offer-to-capture `confirmCard` with
  timeout = `assistant.capability.error.capture_timeout` (SST). The
  default outcome on timeout is capture (yes).
- The capability layer NEVER emits a stack trace, HTTP status code,
  or internal error message in `body`.
- Server-side, every error path increments
  `assistant_capability_skill_error_total{skill, cause}` for ops
  dashboards (cardinality bounded by the closed cause set).

#### 14.A.8 Accessibility & Cognitive Load (transport-neutral)

The capability layer guarantees the following accessibility properties
in every `AssistantResponse`; adapters MUST preserve them when
rendering:

- **No emoji in `body` or `status`.** Adapters MAY add transport-native
  symbols on widget surfaces (e.g. Telegram inline-keyboard `✅`) but
  the canonical `body` and `status` strings are emoji-free.
- **Numbered choices** for `disambiguationPrompt.choices[]` (the
  capability assigns 1-indexed `choice.number`); adapters render the
  numbers visibly.
- **Predictable word order:** capability `body` for terminal states
  uses `<subject> <verb> <object>` (e.g. `reminder set for 2026-05-28
  18:00`, NOT `set reminder for ...`).
- **Verb tense:** in-flight statuses are present-progressive,
  terminal statuses are simple-past. The status vocabulary in §14.A.2
  is normative.
- **No ALL-CAPS** in capability strings.
- **ISO-like timestamps** in capability-emitted text
  (`2026-05-28 18:00 local`), never relative-only (`in 5 hours`),
  because the user may scroll up to verify long after the message
  was sent.
- **Screen-reader friendliness:** capability text MUST be readable
  aloud without rendered glyphs (no `✅` alone as a meaning carrier).

#### 14.A.9 Canonical UI Primitives Catalog (capability layer)

These are CAPABILITY-LAYER contracts. The renderable form lives in
adapter packages per §14.B.

| Primitive | Capability contract (sketch) | Consumers |
|-----------|------------------------------|-----------|
| **Status Token** | `AssistantResponse.status string` (closed vocab §14.A.2) | retrieval, weather, notifications, capture, error |
| **Sources Block** | `AssistantResponse.sources []Source` (structured, §14.A.4); `sources_overflow_count int` | retrieval, weather, future email |
| **Disambiguation Prompt** | `AssistantResponse.disambiguationPrompt {choices[≤3], timeout, disambiguationRef}` (§14.A.3) | intent router borderline band |
| **Confirm Card** | `AssistantResponse.confirmCard {proposedAction, payload, timeout, confirmRef}` (§14.A.6) | notifications, error-recovery offers, future side-effect skills |
| **Error Line** | `AssistantResponse.errorCause Cause`; `body "<skill>: <cause>"` (§14.A.7) | every skill failure path |
| **Reference Resolver** | capability-layer state keyed `(user_id, transport)` (§14.A.5); resolves "that one" / numeric refs to `sources[]` | multi-turn thread context across all skills |

UX9 / G094 satisfied: ≥5 primitives shared across ≥3 skills (retrieval,
weather, notifications) AND across ≥2 transports (Telegram v1 plus the
contract surface for future WhatsApp / web / mobile adapters).

### 14.B Transport-Specific Rendering

Each adapter renders the §14.A primitives using its transport's
native conventions. Only §14.B.1 (Telegram) is binding for v1;
§14.B.2/3/4 are non-binding sketches for future adapter specs.

#### 14.B.1 Telegram v1 Reference Adapter (BINDING)

Package: `internal/telegram/assistant_adapter/`. Rendering knobs SST'd
under `assistant.transports.telegram.*` (see §14.E).

| Canonical primitive | Telegram rendering |
|---------------------|--------------------|
| **Status Token** (`status`) | Rendered as the first line of the assistant message OR by editing a prior in-flight message in place. In-flight statuses (`thinking`, `checking_weather`, `checking_email`) are sent as `<status>…` (trailing ellipsis added by the adapter, not by the capability). Terminal statuses are sent without the ellipsis (`reminder set for 2026-05-28 19:00`, `saved: "<title>" (idea)`). At most one status line per message. |
| **Body** (`body`) | Plain text. Adapter chooses Telegram parse mode per `assistant.transports.telegram.markdown_mode` (SST: `plain` or `markdown_v2`). When `markdown_v2`, the adapter escapes per Telegram MarkdownV2 rules; the capability `body` is unmarked plain text either way. |
| **Sources Block** (`sources[]`) | Trailing block, separated from `body` by a blank line, preceded by literal `sources:`. Each entry rendered as `  <N>. <id-short> — <title> (<captured-date>)` for `artifact` kind OR `  <N>. <provider-name> — retrieved <RFC3339-UTC>` for `external_provider` kind. `<id-short>` = first 8 hex chars of the artifact UUID. `sources_overflow_count > 0` rendered as a trailing `  … +<N> more` line. |
| **Confirm Card** (`confirmCard`) | Inline keyboard with two buttons rendered from per-skill labels: `[✅ <positive-label>]` and `[❌ <negative-label>]`. Button callback data carries `confirmRef`. Typed `yes`/`no` NOT accepted (per §14.A.6). |
| **Disambiguation Prompt** (`disambiguationPrompt`) | Body line: `not sure what you meant. pick one:`. Numbered choices rendered one per line: `  <N>. <label>`. Footer: `reply with a number, /<shortcut1>, /<shortcut2>, or /save.` Choices MAY be reinforced with an inline keyboard `[1][2][3]` when ≤3 choices fit. Either a numeric text reply OR a button tap echoes `disambiguationRef`. |
| **Error Line** (`errorCause` + `body`) | Single line `<skill>: <cause>` where `<cause>` is the human form (`unavailable`, `missing location`, etc.). NEVER markdown, NEVER multi-line. Followed by the capability's offer-to-capture `confirmCard` rendered per the row above. |
| **`captureRoute=true`** | Adapter invokes the existing `handleTextCapture` path in `internal/telegram/bot.go` and produces the existing capture confirmation byte-for-byte. No new UX. |
| **Reset** | Idle reset is the capability semantic (§14.A.5). Telegram v1 ALSO exposes a `/reset` command as a manual escape hatch: on receipt, the adapter sends `AssistantMessage{kind: "reset"}` and the capability clears the per-`(user_id, telegram)` rolling context. `/reset` is documentation-only; users do not need to learn it. |

**Telegram character-budget rule:**
- Total rendered message (status line + body + blank + sources block)
  MUST fit in `assistant.transports.telegram.max_message_chars` (SST,
  recommend 3500) to leave room for Telegram quote-reply overhead
  inside the 4096-char hard limit.
- If `body` alone would exceed the budget, the adapter truncates
  `body` with a trailing `…` and keeps the full sources block.
  Provenance is more valuable than the last sentence.

**Telegram identity:** Per spec 044 Scope 03, the adapter resolves the
inbound `chat_id` to a Smackerel `user_id` before constructing
`AssistantMessage`. This is the v1 Telegram identity surface; future
adapters MUST define and document their own.

#### 14.B.2 WhatsApp Adapter (future — sketch only)

Future adapter spec will own the binding rendering. Non-binding
guidance:

- Render `confirmCard` as WhatsApp Business API **quick-reply buttons**
  (2 buttons, labels from capability).
- Render `disambiguationPrompt` as a WhatsApp **list message** when
  `len(choices) > 2`, else quick-reply buttons.
- Sources block: trailing numbered list as in Telegram, but with
  WhatsApp's larger per-message character budget (no MarkdownV2 escape
  concerns).
- Authenticate the inbound user via WhatsApp phone number → `user_id`
  mapping (out of scope for v1; future adapter spec owns).

#### 14.B.3 Web Chat Adapter (future — sketch only)

Future adapter spec will own the binding rendering. Non-binding
guidance:

- Stream `status` tokens via SSE events
  (`event: status\ndata: thinking`); render as a transient pill above
  the response bubble.
- Render `sources[]` as inline clickable citations linking to the
  artifact viewer for `artifact` kind, and as a hover tooltip showing
  retrieval timestamp for `external_provider` kind.
- Render `confirmCard` as inline buttons under the response bubble.
- Render `disambiguationPrompt` as a button row (≤3 choices fit
  inline on desktop and mobile web).
- Authenticate via existing browser session / bearer token (spec 044).

#### 14.B.4 Mobile In-App Adapter (future — sketch only)

Future adapter spec will own the binding rendering. Non-binding
guidance:

- Render `confirmCard` as a native iOS / Android action sheet OR an
  inline card under the message bubble.
- Push-notification fallback for `confirmCard` timeouts: if the user
  has backgrounded the app, send a silent reminder push ~15s before
  `confirmCard.Timeout` expires (mobile-adapter-private concern; does
  not change capability semantics).
- Render `sources[]` as an expandable native card below the response.
- Authenticate via mobile bearer token (spec 044) — likely the same
  surface as the web chat adapter.

### 14.C Sample Transcripts (Dual View)

Each transcript shows both the canonical `AssistantResponse` emitted
by the capability layer AND the v1 Telegram rendering. The dual view
proves the §14.A / §14.B separation: a future WhatsApp / web / mobile
adapter consuming the same canonical payload renders differently
without changing capability behavior.

#### Transcript 1 — Retrieval Q&A with citations (high confidence)

**User input** (any transport):

```
what did i save about tailscale last month?
```

**Capability emits** `AssistantResponse` (in-flight, then terminal turn):

```yaml
# turn 1 — in-flight
status: thinking
body: ""
sources: []
# turn 2 — terminal
status: thinking      # held; adapter edits in place
body: |
  you saved 3 notes on tailscale auth keys, ACL patterns, and CGNAT
  routing. main themes: tagged identity for headless nodes, and using
  ACL rules to scope service-to-service traffic.
sources:
  - kind: artifact
    id: a1b2c3d4-...
    title: "tailscale ACL tags for self-hosted"
    ref: { capturedAt: 2026-04-12T... }
  - kind: artifact
    id: f7e8d9c0-...
    title: "auth key rotation playbook"
    ref: { capturedAt: 2026-04-22T... }
  - kind: artifact
    id: 5a4b3c2d-...
    title: "CGNAT and SSH cert workflow"
    ref: { capturedAt: 2026-04-28T... }
sources_overflow_count: 0
```

**Telegram render** (Telegram adapter sends `thinking…`, then edits in place with the terminal payload):

```
─────────────────────────────────────────────
BOT:  thinking…
─────────────────────────────────────────────
BOT (edit): you saved 3 notes on tailscale auth keys, ACL
            patterns, and CGNAT routing. main themes: tagged
            identity for headless nodes, and using ACL rules
            to scope service-to-service traffic.

            sources:
              1. a1b2c3d4 — "tailscale ACL tags for self-hosted" (2026-04-12)
              2. f7e8d9c0 — "auth key rotation playbook" (2026-04-22)
              3. 5a4b3c2d — "CGNAT and SSH cert workflow" (2026-04-28)
─────────────────────────────────────────────
```

#### Transcript 2 — Weather (action request with provider attribution)

**User input:** `weather in seattle today`

**Capability emits (terminal turn):**

```yaml
status: checking_weather   # in-flight handled by prior edit; terminal payload below replaces
body: "seattle today: 14°c / 9°c, light rain in the afternoon, wind 12 km/h sw."
sources:
  - kind: external_provider
    id: open-meteo
    title: open-meteo
    ref: { providerName: open-meteo, retrievedAt: 2026-05-28T14:03:00Z }
sources_overflow_count: 0
```

**Telegram render:**

```
─────────────────────────────────────────────
BOT:  checking weather…
─────────────────────────────────────────────
BOT (edit): seattle today: 14°c / 9°c, light rain in the
            afternoon, wind 12 km/h sw.

            sources:
              1. open-meteo — retrieved 2026-05-28T14:03:00Z
─────────────────────────────────────────────
```

#### Transcript 3 — Reminder (confirm flow, happy path)

**User input:** `remind me to take out the trash at 7pm`

**Capability emits (turn 1):**

```yaml
status: reminder_proposed
body: 'schedule: "take out the trash" at 2026-05-28 19:00 local'
confirmCard:
  proposedAction: 'schedule "take out the trash" at 2026-05-28 19:00 local'
  payload: <opaque>
  timeout: 60s
  confirmRef: cnf_a1b2
```

**Telegram render (turn 1):**

```
─────────────────────────────────────────────
BOT: schedule: "take out the trash" at 2026-05-28 19:00 local
     confirm within 60s:
       [✅ schedule]  [❌ cancel]
─────────────────────────────────────────────
```

**User confirms** → inbound `AssistantMessage{kind: confirm,
confirmRef: cnf_a1b2, choice: positive}`.

**Capability emits (turn 2):**

```yaml
status: reminder_confirmed
body: "reminder set for 2026-05-28 19:00"
```

**Telegram render (turn 2):**

```
─────────────────────────────────────────────
BOT: reminder set for 2026-05-28 19:00
─────────────────────────────────────────────
```

#### Transcript 4 — Low-confidence message → silent capture

**User input:** `thought of the day: try sourdough again this weekend`

**Capability emits:**

```yaml
status: saved_as_idea
captureRoute: true
body: 'saved: "thought of the day: try sourdough again…" (idea)'
```

(no `sources[]`, no `disambiguationPrompt`, no `confirmCard` —
identical to today's capture-only behavior.)

**Telegram render:**

```
─────────────────────────────────────────────
BOT: saved: "thought of the day: try sourdough again…" (idea)
─────────────────────────────────────────────
```

#### Transcript 5 — Borderline → disambiguation → user picks → execute

**User input:** `weather`

**Capability emits (turn 1):**

```yaml
disambiguationPrompt:
  choices:
    - { number: 1, id: weather,      label: "weather lookup", shortcut: "/weather" }
    - { number: 2, id: ask,          label: "answer it",      shortcut: "/ask" }
    - { number: 3, id: save_as_note, label: "save as note",   shortcut: "/save" }
  timeout: 30s
  disambiguationRef: dmb_xyz
body: "not sure what you meant. pick one:"
```

**Telegram render (turn 1):**

```
─────────────────────────────────────────────
BOT: not sure what you meant. pick one:
       1. weather lookup
       2. answer it
       3. save as note
     reply with a number, /weather, /ask, or /save.
─────────────────────────────────────────────
```

**User replies `1`** → inbound `AssistantMessage{kind: disambiguation,
disambiguationRef: dmb_xyz, choice: 1}`. Skill fires but the slot is
missing:

**Capability emits (turn 2):**

```yaml
status: unavailable
errorCause: slot_missing
body: "weather: missing location"
disambiguationPrompt:
  choices:
    - { number: 1, id: save_as_note, label: "save as note", shortcut: "/save" }
  timeout: 30s
  disambiguationRef: dmb_xyz2
```

**Telegram render (turn 2):**

```
─────────────────────────────────────────────
BOT: weather: missing location
     reply with a city, or pick:
       1. save as note
─────────────────────────────────────────────
```

**User replies `seattle`** → inbound is fresh text within the
slot-resolution window; capability completes the missing slot.

**Capability emits (turn 3):**

```yaml
status: checking_weather
body: "seattle now: 12°c, overcast, wind 8 km/h w."
sources:
  - kind: external_provider
    id: open-meteo
    title: open-meteo
    ref: { providerName: open-meteo, retrievedAt: 2026-05-28T15:22:00Z }
```

**Telegram render (turn 3):**

```
─────────────────────────────────────────────
BOT: seattle now: 12°c, overcast, wind 8 km/h w.

     sources:
       1. open-meteo — retrieved 2026-05-28T15:22:00Z
─────────────────────────────────────────────
```

#### Transcript 6 — Weather provider outage → fallback capture (covers BS-006)

**User input:** `weather in reykjavik tomorrow`

**Capability emits (turn 1):**

```yaml
status: unavailable
errorCause: provider_unavailable
body: "weather: unavailable"
confirmCard:
  proposedAction: "save your question as a note"
  payload: <opaque, encodes original user_msg>
  timeout: 30s
  confirmRef: cnf_err_q9
```

**Telegram render (turn 1):**

```
─────────────────────────────────────────────
BOT: weather: unavailable
     save your question as a note?
       [yes]  [no]
─────────────────────────────────────────────
```

**User taps [yes]** → inbound `AssistantMessage{kind: confirm,
confirmRef: cnf_err_q9, choice: positive}`.

**Capability emits (turn 2):**

```yaml
status: saved_as_idea
captureRoute: true
body: 'saved: "weather in reykjavik tomorrow" (idea)'
```

**Telegram render (turn 2):**

```
─────────────────────────────────────────────
BOT: saved: "weather in reykjavik tomorrow" (idea)
─────────────────────────────────────────────
```

#### Transcript 7 — Reminder cancel path (covers BS-004 alt-flow)

**User input:** `remind me to call the dentist tomorrow at 9`

**Capability emits (turn 1):** same shape as Transcript 3 turn 1 with
proposed time `2026-05-29 09:00 local` and `confirmRef: cnf_p4q5`.

**Telegram render (turn 1):**

```
─────────────────────────────────────────────
BOT: schedule: "call the dentist" at 2026-05-29 09:00 local
     confirm within 60s:
       [✅ schedule]  [❌ cancel]
─────────────────────────────────────────────
```

**User taps [❌ cancel]** → inbound `AssistantMessage{kind: confirm,
confirmRef: cnf_p4q5, choice: negative}`.

**Capability emits (turn 2):**

```yaml
status: reminder_cancelled
body: ""              # cancelled has no body; follow-up turn carries capture
```

**Capability emits (turn 3, follow-up capture per §14.A.6 audit invariant):**

```yaml
status: saved_as_idea
captureRoute: true
body: 'saved: "call the dentist tomorrow at 9" (idea)'
```

**Telegram render (turn 2 + 3, collapsed into one user-visible message per Telegram convention):**

```
─────────────────────────────────────────────
BOT: saved: "call the dentist tomorrow at 9" (idea)
─────────────────────────────────────────────
```

### 14.D Decisions Recorded

1. **Source attribution rendering.** The CAPABILITY emits structured
   `sources[]`. Telegram v1 renders a trailing numbered block (§14.B.1
   row). Other adapters render natively (inline citations on web,
   expandable native cards on mobile, list messages on WhatsApp). This
   resolves §13 Open Question #4 at the capability layer (sources are
   structured) AND ratifies the Telegram trailing-block format as the
   v1 adapter rendering choice.

2. **Thread reset.** Idle timeout is the **capability** semantic
   (§14.A.5). Transports MAY surface a manual reset command; Telegram
   v1 exposes `/reset` as a non-mandatory escape hatch (§14.B.1 Reset
   row). The capability layer does NOT require any reset command
   surface.

3. **Three-band confidence floors are capability-layer config.** The
   SST keys `assistant.capability.intent.min_confidence` and
   `assistant.capability.intent.disambiguate_floor` are NOT
   per-transport. Confidence thresholds determine intent routing,
   which is a capability concern; per-transport floors would create
   user-visible semantic drift across frontends and is FORBIDDEN.

4. **Conversational state key.** Recommend keying on
   `(user_id, transport)` for thread context (§14.A.5).
   Bubbles.design ratifies during the full design pass.

5. **Typed yes/no is NOT a confirm callback** (§14.A.6). Adapters
   MUST NOT translate free-text `yes`/`no` into `kind: confirm`. The
   user's next free-text message remains classifiable by the intent
   router.

### 14.E SST Key Reclassification

The 6 SST keys introduced by the prior §13.11 are reclassified into
**capability-layer** keys (transport-neutral semantics) vs.
**transport-specific** keys (per-adapter rendering knobs). All keys
remain NO-DEFAULTS / fail-loud per `smackerel-no-defaults`.

**Capability-layer keys (under `assistant.capability.*`):**

| Key | Type | Section |
|-----|------|---------|
| `assistant.capability.intent.disambiguate_floor` | float `[0,1]`, must be `< min_confidence` | §14.A.3 |
| `assistant.capability.intent.disambiguate_timeout` | duration | §14.A.3 |
| `assistant.capability.context.window_turns` | int (recommend 4–6) | §14.A.5 |
| `assistant.capability.context.idle_timeout` | duration (recommend 10m) | §14.A.5 |
| `assistant.capability.skills.notifications.confirm_timeout` | duration (recommend 60s) | §14.A.6 |
| `assistant.capability.error.capture_timeout` | duration (recommend 30s) | §14.A.7 |
| `assistant.capability.sources_max` | int (recommend 5) — **new** | §14.A.4 |
| `assistant.capability.body_max_chars` | int — **new** | §14.A.1 |
| `assistant.capability.status_max_duration` | duration — **new** | §14.A.2 |

**Transport-specific keys (under `assistant.transports.telegram.*`):**

| Key | Type | Section |
|-----|------|---------|
| `assistant.transports.telegram.markdown_mode` | enum `plain` \| `markdown_v2` — **new** | §14.B.1 |
| `assistant.transports.telegram.max_message_chars` | int (recommend 3500) — **new** | §14.B.1 |

Bubbles.design owns the YAML schema and validation tests; UX specifies
the contract and the capability-vs-transport split.

The existing `assistant.intent.min_confidence` and
`assistant.intent.classifier_model` keys (from §12) MUST be moved
under `assistant.capability.intent.*` for naming consistency;
bubbles.design folds this rename into the same `assistant.capability.*`
block. The existing `assistant.transports.telegram.enabled` (§6.4) is
already transport-scoped.

### 14.F Anchors To Existing Scope Titles

No new scopes introduced. UX anchors to the 10 candidate scopes
already in `scopes.md`:

- §14.A.2–14.A.4 (status vocab, three-band disambiguation, structured
  sources) → primarily **SCOPE-04** (intent router + capture
  fallback, capability layer) and **SCOPE-06** (retrieval), with the
  `Source` contract shipping in **SCOPE-03** (skill registry +
  Provenance).
- §14.A.5 (thread context, `(user_id, transport)` key) → primarily
  **SCOPE-04** with foundation hooks in **SCOPE-02** (canonical
  message contracts).
- §14.A.6 (confirm card) → **SCOPE-08** (notifications); the
  primitive contract lives in **SCOPE-02** so it ships before any
  side-effect skill.
- §14.A.7 (error UX) → cross-cutting; contract in **SCOPE-02**, first
  exercised in **SCOPE-06** (retrieval can fail).
- §14.A.9 (capability primitives catalog) → **SCOPE-02** for the type
  surface; **SCOPE-04** for first use.
- §14.B.1 (Telegram adapter renderer) → **SCOPE-05** (Telegram v1
  reference adapter); shared adapter rendering helpers live under
  `internal/telegram/assistant_adapter/`.
- §14.E (new SST keys: capability `sources_max`, `body_max_chars`,
  `status_max_duration`; telegram `markdown_mode`,
  `max_message_chars`) → **SCOPE-01** (assistant SST: capability +
  adapter sub-block).

Bubbles.plan owns whether any of the above warrant splitting an
existing scope. UX makes no such recommendation.

---

## 15. References

- `.github/instructions/product-principles.instructions.md` — Principle
  enforcement (advisory until ratification; this spec treats them as
  binding for design purposes).
- `.github/skills/smackerel-no-defaults/SKILL.md` — SST policy applied
  to `assistant.*` block.
- `.github/skills/bubbles-capability-foundation-design/SKILL.md` —
  rationale for introducing a skill-registry foundation now rather than
  growing skills ad-hoc inside `bot.go`.
- `.github/skills/bubbles-spec-template-bdd/SKILL.md` — BDD scenario
  shape.
- `internal/telegram/bot.go` — current `handleMessage` + `handleTextCapture`
  routing.
- `ml/` sidecar — LLM bridge to be reused for intent classification +
  answer synthesis.
- spec 044 — per-user bearer auth, chat→user mapping.
- spec 060 — scope claims, candidate for per-skill scoping.
- spec 054 — notification intelligence handler, candidate re-use for
  reminders.
